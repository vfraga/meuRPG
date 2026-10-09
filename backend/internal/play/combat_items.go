package play

import (
	"context"
	"errors"
	"fmt"
	"math"

	"connectrpc.com/connect"

	playv1 "github.com/PuraFome/meuRPG/backend/gen/meurpg/play/v1"
	"github.com/PuraFome/meuRPG/backend/internal/authz"
	"github.com/PuraFome/meuRPG/backend/internal/platform/dice"
	"github.com/PuraFome/meuRPG/backend/internal/platform/idem"
	"github.com/PuraFome/meuRPG/backend/internal/play/link"
	"github.com/PuraFome/meuRPG/backend/internal/play/playdb"
)

// Items used in a combat (SRD 5.1 "Use an Object", "Potions"): every use is the combatant's
// action. The characters module owns the inventory; this file settles the dice, the hit
// points and the action economy, and asks the roster to describe the item and to change the
// inventory in the same transaction.

// itemReach is how close a potion has to be to be given to a creature to drink (a hand's
// reach: 5 ft).
const itemReach = 5

// scrollCheckBase is the DC of a scroll's ability check before the spell's level (SRD 5.1
// "Spell Scroll").
const scrollCheckBase = 10

// UseItem implements playv1connect.CombatServiceHandler.
//
//nolint:gocognit,gocyclo // one use of an item in a combat: the checks and the effect of each kind of use
func (s *Service) UseItem(
	ctx context.Context,
	req *connect.Request[playv1.UseItemRequest],
) (*connect.Response[playv1.UseItemResponse], error) {
	m, err := authz.RequireCampaignMember(ctx, req.Msg.GetCampaignId())
	if err != nil {
		return nil, err
	}
	key, err := parseKey(req.Msg.GetIdempotencyKey())
	if err != nil {
		return nil, err
	}
	encID, err := parseCombatID(req.Msg.GetEncounterId(), "encounter")
	if err != nil {
		return nil, err
	}
	combID, err := parseCombatID(req.Msg.GetCombatantId(), "combatant")
	if err != nil {
		return nil, err
	}
	itemID, err := parseCombatID(req.Msg.GetItemId(), "item")
	if err != nil {
		return nil, err
	}
	use := req.Msg.GetUse()
	if use <= playv1.CombatItemUse_COMBAT_ITEM_USE_UNSPECIFIED || use > playv1.CombatItemUse_COMBAT_ITEM_USE_UNEQUIP_SHIELD {
		return nil, connect.NewError(connect.CodeInvalidArgument, errors.New("use must name what is done with the item"))
	}
	var targetID string
	if use == playv1.CombatItemUse_COMBAT_ITEM_USE_GIVE_TO_DRINK {
		if targetID, err = parseCombatID(req.Msg.GetTargetCombatantId(), "combatant"); err != nil {
			return nil, err
		}
	} else if req.Msg.GetTargetCombatantId() != "" {
		return nil, connect.NewError(connect.CodeInvalidArgument, errors.New("target_combatant_id is for giving a potion to drink"))
	}
	var in rollInput
	var rolled bool
	switch roll := req.Msg.GetRoll().(type) {
	case *playv1.UseItemRequest_RollInApp:
		if !roll.RollInApp {
			return nil, connect.NewError(connect.CodeInvalidArgument, errors.New("roll_in_app must be true"))
		}
		in.inApp, rolled = true, true
	case *playv1.UseItemRequest_TypedSum:
		in.typed, rolled = int(roll.TypedSum), true
	}
	v := viewerOf(m)

	var made actionEvent
	var out itemUseOutcome
	var vitals []*playv1.CharacterVitals
	res, err := s.write(ctx, combatWrite{m: m, key: key, hash: idem.Hash(req.Msg), kind: eventActionTaken, encounterID: encID}, func(c *combatTx) (any, error) {
		vitals, out = nil, itemUseOutcome{}
		cs, err := c.q.ListCombatants(ctx, c.enc.ID)
		if err != nil {
			return nil, fmt.Errorf("list the combatants: %w", err)
		}
		v = c.viewer(m, cs)
		who, err := findCombatant(cs, combID, v)
		if err != nil {
			return nil, err
		}
		if err := v.mayAct(who); err != nil {
			return nil, err
		}
		if err := s.mustActNow(ctx, c, who); err != nil {
			return nil, err
		}
		if isCreature(who) {
			return nil, errEncounter(playv1.EncounterBlockedReason_ENCOUNTER_BLOCKED_REASON_ITEM_NOT_USABLE, "a creature has no inventory")
		}
		if who.ActionUsed && !v.master {
			return nil, errEncounter(playv1.EncounterBlockedReason_ENCOUNTER_BLOCKED_REASON_ACTION_USED, "the action of this turn is used")
		}
		item, err := s.roster.ItemForUse(ctx, c.tx, m.CampaignID, who.CharacterID, itemID)
		if err != nil {
			return nil, err
		}
		// An item nobody has identified is not used in a fight, not even by the master: what
		// the log would call it would give it away (RN-10).
		if item.Unidentified || item.Kind == "" {
			return nil, errEncounter(playv1.EncounterBlockedReason_ENCOUNTER_BLOCKED_REASON_ITEM_NOT_USABLE, "this item cannot be used now")
		}
		run, err := breakRun(ctx, c, who)
		if err != nil {
			return nil, err
		}
		made = actionEvent{
			Round: c.enc.Round, Secret: who.Hidden, Actor: who.ID, Key: item.Key, RunBefore: run,
			ActionBefore: who.ActionUsed, BonusBefore: who.BonusActionUsed, ReactionBefore: who.ReactionUsed, DashedBefore: who.Dashed,
			AttacksBefore: who.AttacksMade, DisengagedBefore: who.Disengaged, SurgedBefore: who.ActionSurged,
			AttackKeyBefore: deref(who.ActionAttackKey), FlurryBefore: who.BonusAttacksLeft,
		}
		st := &itemState{ctx: ctx, s: s, c: c, m: m, v: v, who: who, cs: cs, item: item, in: in, rolled: rolled, made: &made, out: &out, vitals: &vitals}
		switch use {
		case playv1.CombatItemUse_COMBAT_ITEM_USE_DRINK:
			err = st.drink(who)
		case playv1.CombatItemUse_COMBAT_ITEM_USE_GIVE_TO_DRINK:
			var target playdb.Combatant
			if target, err = findCombatant(cs, targetID, v); err == nil {
				err = st.give(target)
			}
		case playv1.CombatItemUse_COMBAT_ITEM_USE_READ_SCROLL:
			err = st.read()
		case playv1.CombatItemUse_COMBAT_ITEM_USE_CHARGES:
			err = st.charges(int(req.Msg.GetCharges()))
		default:
			err = st.shield(use == playv1.CombatItemUse_COMBAT_ITEM_USE_EQUIP_SHIELD)
		}
		if err != nil {
			return nil, err
		}
		if err := c.q.SetCombatantEconomy(ctx, playdb.SetCombatantEconomyParams{
			ID: who.ID, ActionUsed: true, BonusActionUsed: who.BonusActionUsed, ReactionUsed: who.ReactionUsed, Dashed: who.Dashed,
		}); err != nil {
			return nil, fmt.Errorf("spend the action: %w", err)
		}
		if c.enc, err = c.q.TouchEncounter(ctx, c.enc.ID); err != nil {
			return nil, fmt.Errorf("touch the encounter: %w", err)
		}
		c.characterID = &who.CharacterID
		return made, nil
	})
	if err != nil {
		return nil, s.dbError(ctx, "use an item", err)
	}
	ev, err := resultEvent(res, made)
	if err != nil {
		return nil, s.dbError(ctx, "read the item use", err)
	}
	enc, err := s.finish(ctx, m, res, func(ctx context.Context, d *encounterData) {
		s.publishEncounterChanged(ctx, m.CampaignID, d.enc)
		s.publishLogChanged(ctx, m.CampaignID, d.enc.ID, !ev.Secret)
		for _, vit := range vitals {
			s.publishVitals(m.CampaignID, vit)
		}
		s.publishInventoryOf(m.CampaignID, ev.Actor, d)
	})
	if err != nil {
		return nil, err
	}
	resp := &playv1.UseItemResponse{Encounter: enc, SpellKey: out.spell, SpellLevel: out.level, LastChargeD20: out.lastD20, Destroyed: out.destroyed}
	if ev.Heal {
		resp.Roll = diceRoll(ev.DiceCount, ev.DiceSides, ev.Faces, ev.Modifier, ev.Total, ev.Physical)
		resp.Healed = &ev.Amount
	}
	if out.check != nil {
		resp.AbilityCheck, resp.AbilityCheckPassed = out.check, out.checkPassed
	}
	return connect.NewResponse(resp), nil
}

// publishInventoryOf tells the master and the combatant's player that the inventory changed.
func (s *Service) publishInventoryOf(campaignID, combatantID string, d *encounterData) {
	for _, c := range d.cs {
		if c.ID == combatantID && c.CharacterID != "" {
			s.PublishInventoryChanged(campaignID, c.CharacterID, deref(c.UserID))
			return
		}
	}
}

// itemUseOutcome is what a use hands back besides the event.
type itemUseOutcome struct {
	spell       string
	level       int32
	check       *playv1.DiceRoll
	checkPassed bool
	lastD20     int32
	destroyed   bool
}

// itemState is one use of an item in progress.
type itemState struct {
	ctx    context.Context
	s      *Service
	c      *combatTx
	m      authz.Membership
	v      combatViewer
	who    playdb.Combatant
	cs     []playdb.Combatant
	item   link.UsableItem
	in     rollInput
	rolled bool
	made   *actionEvent
	out    *itemUseOutcome
	vitals *[]*playv1.CharacterVitals
}

func notUsable() error {
	return errEncounter(playv1.EncounterBlockedReason_ENCOUNTER_BLOCKED_REASON_ITEM_NOT_USABLE, "this item cannot be used that way")
}

func (st *itemState) apply(w link.ItemUseWrite) error {
	w.ItemID = st.item.ID
	return st.s.roster.ApplyItemUse(st.ctx, st.c.tx, st.m.CampaignID, st.who.CharacterID, w)
}

// give hands a potion to another combatant within reach to drink.
func (st *itemState) give(target playdb.Combatant) error {
	if target.ID == st.who.ID {
		return connect.NewError(connect.CodeInvalidArgument, errors.New("the target must be another combatant: drink it yourself"))
	}
	if target.Kind != kindPlayer || isCreature(target) {
		return errEncounter(playv1.EncounterBlockedReason_ENCOUNTER_BLOCKED_REASON_ITEM_NOT_USABLE, "only a player's character drinks a potion it is given")
	}
	if !st.v.master && !isTheatre(st.c.enc) {
		if !placed(st.who) || !placed(target) {
			return errEncounter(playv1.EncounterBlockedReason_ENCOUNTER_BLOCKED_REASON_NOT_PLACED, "both must be on the map")
		}
		if dist, _ := distanceFt(st.who, target); dist > itemReach {
			return errEncounter(playv1.EncounterBlockedReason_ENCOUNTER_BLOCKED_REASON_TARGET_OUT_OF_REACH, "the target is beyond reach",
				func(b *playv1.EncounterBlocked) { b.MissingFt = dist - itemReach })
		}
	}
	return st.drink(target)
}

// drink makes the target drink a potion: it heals and gives temporary hit points (SRD 5.1:
// temporary hit points do not add up).
func (st *itemState) drink(target playdb.Combatant) error {
	if st.item.Kind != "potion" {
		return notUsable()
	}
	st.made.Target = target.ID
	if st.item.HealDice != "" {
		if !st.rolled {
			return connect.NewError(connect.CodeInvalidArgument, errors.New("set roll_in_app or typed_sum for the potion's dice"))
		}
		if !st.v.master {
			if err := st.s.mustRollThisWay(st.ctx, st.c.tx, st.m, st.in); err != nil {
				return err
			}
		}
		expr, err := dice.Parse(st.item.HealDice)
		if err != nil {
			return fmt.Errorf("read the potion's dice: %w", err)
		}
		var roll dice.Result
		if st.in.inApp {
			roll, err = dice.Roll(st.s.roller, expr)
		} else {
			roll, err = dice.Physical(expr, st.in.typed)
		}
		if err != nil {
			if st.in.inApp {
				return fmt.Errorf("roll the potion: %w", err)
			}
			return connect.NewError(connect.CodeInvalidArgument, errors.New("typed_sum must be the sum of the potion's dice"))
		}
		hit, vit, err := st.s.healCombatant(st.ctx, st.c, target, clamp32(max(roll.Total, 0), 0, math.MaxInt32))
		if err != nil {
			return err
		}
		st.made.Heal, st.made.DiceCount, st.made.DiceSides, st.made.Faces = true, clamp32(expr.Count, 0, math.MaxInt32), clamp32(expr.Sides, 0, math.MaxInt32), faces32(roll.Faces)
		st.made.Modifier, st.made.Total, st.made.Physical = clamp32(expr.Modifier, math.MinInt32, math.MaxInt32), clamp32(roll.Total, 0, math.MaxInt32), roll.Physical
		st.made.Amount, st.made.Before, st.made.After, st.made.DeathBefore = hit.Amount, hit.Before, hit.After, hit.DeathBefore
		if vit != nil {
			*st.vitals = append(*st.vitals, vit)
		}
	}
	if st.item.TempHP > 0 {
		t, err := st.s.readFxTarget(st.ctx, st.c, target)
		if err != nil {
			return err
		}
		var h castHit
		vit, err := st.s.giveTempHP(st.ctx, st.c, t, st.item.TempHP, &h)
		if err != nil {
			return err
		}
		if vit != nil {
			*st.vitals = append(*st.vitals, vit)
		}
		if !st.made.Heal {
			st.made.Heal, st.made.Amount, st.made.After = true, 0, &hpState{Temp: clamp32(max(st.item.TempHP, t.temp), 0, math.MaxInt32)}
		}
	}
	return st.apply(link.ItemUseWrite{Op: "consume"})
}

// read reads a spell scroll (SRD 5.1 "Spell Scroll"): the spell must be on the reader's class
// list; one above the reader's highest slot takes an ability check, DC 10 + the spell's level.
func (st *itemState) read() error {
	it := st.item
	if it.Kind != "scroll" || it.Spell == "" {
		return notUsable()
	}
	if !it.Readable {
		return errEncounter(playv1.EncounterBlockedReason_ENCOUNTER_BLOCKED_REASON_SCROLL_UNREADABLE, "the scroll's spell is on none of the reader's class lists")
	}
	st.out.spell, st.out.level = it.Spell, clamp32(it.SpellLevel, 0, math.MaxInt32)
	if it.TooHigh {
		if !st.rolled {
			return connect.NewError(connect.CodeInvalidArgument, errors.New("set roll_in_app or typed_sum (the d20) for the scroll's ability check"))
		}
		face, roll, err := st.s.d20(st.in, it.CheckMod)
		if err != nil {
			return err
		}
		dc := scrollCheckBase + it.SpellLevel
		st.made.D20, st.made.Modifier, st.made.Total = clamp32(face, 1, 20), clamp32(it.CheckMod, math.MinInt32, math.MaxInt32), clamp32(roll.Total, math.MinInt32, math.MaxInt32)
		st.made.Physical, st.made.Outcome = roll.Physical, outcomeMiss
		passed := roll.Total >= dc
		if passed {
			st.made.Outcome = outcomeHit
		}
		st.out.check, st.out.checkPassed = diceRoll(1, 20, []int32{clamp32(face, 0, math.MaxInt32)}, clamp32(it.CheckMod, math.MinInt32, math.MaxInt32), clamp32(roll.Total, math.MinInt32, math.MaxInt32), roll.Physical), passed
	}
	st.made.Key = it.Spell // the log names the spell that was read
	return st.apply(link.ItemUseWrite{Op: "consume"})
}

// charges spends charges of an item; the last one risks it when the item says so.
func (st *itemState) charges(n int) error {
	it := st.item
	if it.Kind != "charges" {
		return notUsable()
	}
	if n < 1 || n > it.ChargesLeft {
		return errEncounter(playv1.EncounterBlockedReason_ENCOUNTER_BLOCKED_REASON_ITEM_NO_CHARGES, "the item has not that many charges left")
	}
	destroyed := false
	if n == it.ChargesLeft && it.DestroyOnEmpty {
		if !st.rolled {
			return connect.NewError(connect.CodeInvalidArgument, errors.New("set roll_in_app or typed_sum (the d20) for the last charge"))
		}
		face, roll, err := st.s.d20(st.in, 0)
		if err != nil {
			return err
		}
		st.out.lastD20, destroyed = clamp32(face, 1, 20), face == 1
		st.out.destroyed = destroyed
		st.made.D20, st.made.Physical = st.out.lastD20, roll.Physical
	}
	st.made.Amount = clamp32(n, 0, math.MaxInt32)
	return st.apply(link.ItemUseWrite{Op: "spend", Charges: n, Destroyed: destroyed})
}

// shield wears or takes off a shield, which is an action (SRD 5.1 "Armor": one action).
func (st *itemState) shield(on bool) error {
	if st.item.Kind != "shield" || st.item.Equipped == on {
		return notUsable()
	}
	op := "unequip"
	if on {
		op = "equip"
	}
	return st.apply(link.ItemUseWrite{Op: op})
}
