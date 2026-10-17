package play

import (
	"context"
	"errors"
	"fmt"
	"math"
	"slices"

	"connectrpc.com/connect"

	playv1 "github.com/PuraFome/meuRPG/backend/gen/meurpg/play/v1"
	rulesv1 "github.com/PuraFome/meuRPG/backend/gen/meurpg/rules/v1"
	"github.com/PuraFome/meuRPG/backend/internal/authz"
	"github.com/PuraFome/meuRPG/backend/internal/play/playdb"
	"github.com/PuraFome/meuRPG/backend/internal/rules/reaction"
)

// Counterspell (SRD 5.1, Counterspell): a creature within 60 feet that the reactor
// sees casts a spell. The reactor spends its reaction and a slot of the 3rd level
// or above; a spell whose level is no more than the slot's fails and does nothing,
// a higher one asks the reactor an ability check with its spellcasting ability, DC
// 10 + the spell's level. The cast waits for the windows before anything of it
// happens (a held action), and the slot of a countered spell is spent all the same:
// casting it already expended it. The prompt never names the spell or its level.

// spendReaction marks the combatant's reaction as spent.
func (s *Service) spendReaction(ctx context.Context, c *combatTx, who playdb.Combatant) error {
	if err := c.q.SetCombatantEconomy(ctx, playdb.SetCombatantEconomyParams{
		ID: who.ID, ActionUsed: who.ActionUsed, BonusActionUsed: who.BonusActionUsed, ReactionUsed: true, Dashed: who.Dashed,
	}); err != nil {
		return fmt.Errorf("spend the reaction: %w", err)
	}
	return nil
}

// reactorSees says whether a reactor sees the combatant that triggered a window and
// is within the 60 feet of the reactions (18 m): a hidden or invisible one is seen
// by no player (RN-10), a blind reactor sees nothing, and on a map with the fog each
// reactor sees with its own eyes.
func (s *Service) reactorSees(ctx context.Context, c *combatTx, cs []playdb.Combatant, reactor, actor playdb.Combatant) (dist int32, ok bool, err error) {
	if eyesCannotSee(reactor, actor) {
		return 0, false, nil
	}
	if !isTheatre(c.enc) {
		d, placedBoth := distanceFt(reactor, actor)
		if placedBoth && d > reaction.RangeFt {
			return 0, false, nil
		}
		dist = d
	}
	sees, err := s.fogSees(ctx, c, cs, reactor, actor)
	if err != nil {
		return 0, false, err
	}
	return dist, sees, nil
}

// eyesCannotSee says a blind reactor sees nothing, a hidden NPC is seen by no player (RN-10) and an
// invisible actor is seen by no one.
func eyesCannotSee(reactor, actor playdb.Combatant) bool {
	if slices.Contains(reactor.Conditions, "condition:blinded") || (actor.Kind == kindNPC && actor.Hidden && reactor.Kind == kindPlayer) {
		return true
	}
	return slices.Contains(actor.Conditions, "condition:invisible")
}

// fogSees says whether the reactor sees the actor on a map with the fog: a player with its own eyes, an
// NPC with the senses of its sheet. Without the fog, or with a combatant that has no square, it does.
func (s *Service) fogSees(ctx context.Context, c *combatTx, cs []playdb.Combatant, reactor, actor playdb.Combatant) (bool, error) {
	if c.sight == nil {
		return true, nil
	}
	if reactor.Kind == kindPlayer && reactor.UserID != nil {
		v := c.viewer(authz.Membership{CampaignID: c.session.CampaignID, UserID: *reactor.UserID, Role: authz.RolePlayer}, cs)
		return v.sees(actor), nil
	}
	if reactor.Kind != kindPlayer && placed(reactor) && placed(actor) {
		sheet, err := s.sheetOf(ctx, c.tx, c.session.CampaignID, reactor)
		if err != nil {
			return false, err
		}
		return c.sight.reactorSees(reactor, sheet, squareOfCombatant(actor)), nil
	}
	return true, nil
}

// counterspellWindows lists the windows a cast opens: one for each combatant of the
// other side that sees the caster, has the reaction and a slot of the 3rd level or
// above, in the initiative order. In "Sempre" a player's spell on an enemy that no
// one can counter still asks the master's check.
func (s *Service) counterspellWindows(ctx context.Context, c *combatTx, cs []playdb.Combatant, caster playdb.Combatant, spellKey string, level int32, targs []playdb.Combatant) ([]windowSpec, error) {
	var specs []windowSpec
	for _, r := range cs {
		if r.ID == caster.ID || r.Side == caster.Side || r.Defeated {
			continue
		}
		k, ok, err := s.kitOf(ctx, c.tx, c.session.CampaignID, r)
		if err != nil {
			return nil, err
		}
		if !ok || !k.canReact() || len(k.slotsFor(spellCounterspell)) == 0 {
			continue
		}
		dist, sees, err := s.reactorSees(ctx, c, cs, r, caster)
		if err != nil {
			return nil, err
		}
		if !sees {
			continue
		}
		specs = append(specs, windowSpec{
			kind: reaction.CounterspellKind, reactor: &r,
			trigger: windowTrigger{Actor: caster.ID, Key: spellKey, Spell: spellKey, Level: level, Distance: dist, Roll: "cast"},
		})
	}
	if len(specs) == 0 && c.rules.EnemyReactionsAlways && caster.Kind == kindPlayer && slices.ContainsFunc(targs, func(t playdb.Combatant) bool { return playerAgainstEnemy(caster, t) }) {
		specs = append(specs, windowSpec{kind: reaction.MasterCheck, trigger: windowTrigger{Actor: caster.ID, Key: spellKey, Spell: spellKey, Level: level, Roll: "cast"}})
	}
	return specs, nil
}

// holdCast holds a cast that a Counterspell or the master's check can change: the
// request is kept and the windows opened, and nothing of the cast happens until they
// are answered (the replay in releaseHold). It returns the event that stands for
// the held cast, or false when nothing waits.
func (s *Service) holdCast(ctx context.Context, c *combatTx, m authz.Membership, req *playv1.CastSpellRequest, cs []playdb.Combatant, caster playdb.Combatant, spellKey string, level int32, targs []playdb.Combatant) (actionEvent, bool, error) {
	if c.replay != nil {
		return actionEvent{}, false, nil
	}
	specs, err := s.counterspellWindows(ctx, c, cs, caster, spellKey, level, targs)
	if err != nil || len(specs) == 0 {
		return actionEvent{}, false, err
	}
	ev := actionEvent{Round: c.enc.Round, Secret: caster.Hidden, Actor: caster.ID, Key: spellKey}
	for _, t := range targs {
		ev.Secret = ev.Secret || t.Hidden
	}
	if ev, err = s.holdAction(ctx, c, m, "cast", caster, req, holdData{}, specs, ev); err != nil {
		return ev, false, err
	}
	if err := touchEncounter(ctx, c); err != nil {
		return ev, false, err
	}
	c.characterID = &caster.CharacterID
	return ev, true, nil
}

// counterspellPrompt builds the prompt of a Counterspell window: the caster the reactor
// sees and how far, and its slots; never the spell or its level. For the master it also
// says what each slot does ("anula sem teste" or the check).
func (wv *windowView) counterspellPrompt(w playdb.ReactionWindow, reactor *playdb.Combatant, out *playv1.ReactionWindow) error {
	t := windowTriggerOf(w)
	p := &playv1.CounterspellPrompt{}
	if a, ok := wv.byID[t.Actor]; ok && (wv.v.master || wv.v.sees(a)) {
		p.CasterLabel = a.Label
	}
	if t.Distance > 0 {
		p.DistanceFt = &t.Distance
	}
	if reactor != nil {
		if err := wv.counterspellSlots(*reactor, t, p, out); err != nil {
			return err
		}
	}
	out.Prompt = &playv1.ReactionWindow_Counterspell{Counterspell: p}
	return nil
}

// counterspellSlots gives the prompt the free slots of the reactor and, to the master, what each does.
func (wv *windowView) counterspellSlots(reactor playdb.Combatant, t windowTrigger, p *playv1.CounterspellPrompt, out *playv1.ReactionWindow) error {
	k, err := wv.kitFor(&reactor)
	if err != nil {
		return err
	}
	p.SlotOptions = k.slotsFor(spellCounterspell)
	if wv.v.master && out.Trigger != nil {
		for _, sl := range p.SlotOptions {
			auto, dc := reaction.Counterspell(int(sl.GetLevel()), int(t.Level))
			out.Trigger.SlotEffects = append(out.Trigger.SlotEffects, &playv1.SlotEffect{
				Slot: sl, NoCheck: auto, CheckDc: clamp32(dc, 0, math.MaxInt32), CheckBonus: clamp32(k.st.CastingMod, math.MinInt32, math.MaxInt32),
			})
		}
	}
	return nil
}

// answerCounterspell answers a Counterspell window.
func (s *Service) answerCounterspell(ctx context.Context, c *combatTx, w playdb.ReactionWindow, reactor playdb.Combatant, use bool, req *playv1.AnswerReactionRequest, got *answered) error {
	t := windowTriggerOf(w)
	re := &reactionEvent{Kind: string(reaction.CounterspellKind), Window: w.ID, ByMaster: c.master, Spell: t.Spell, Level: t.Level}
	got.ev = actionEvent{Round: c.enc.Round, Secret: reactor.Hidden, Actor: reactor.ID, Target: t.Actor, Reaction: re}
	got.result = &playv1.ReactionResult{Kind: playv1.ReactionKind_REACTION_KIND_COUNTERSPELL, ByMaster: c.master, Used: use}
	if !use {
		_, err := s.closeWindow(ctx, c, w, windowAnswered, reaction.ReasonNone, windowOutcome{ByMaster: c.master}, "")
		return err
	}
	k, slot, err := s.counterspellSlotOf(ctx, c, reactor, req)
	if err != nil {
		return err
	}
	check, err := s.counterspellCheck(ctx, c, req, reactor, k, slot, t, re)
	if err != nil {
		return err
	}
	vitals, err := s.spendReactionSlot(ctx, c, reactor, slot)
	if err != nil {
		return err
	}
	if vitals != nil {
		got.vitals = append(got.vitals, vitals)
	}
	if err := s.spendReaction(ctx, c, reactor); err != nil {
		return err
	}
	re.Used, re.Slot, re.Countered = true, &slot, check.countered
	got.ev.Slot, got.ev.ReactionBefore = &slot, reactor.ReactionUsed
	if _, err := s.closeWindow(ctx, c, w, windowAnswered, reaction.ReasonNone, windowOutcome{ByMaster: c.master, Used: true, Slot: &slot, Countered: check.countered}, ""); err != nil {
		return err
	}
	c.lastAnswered = reaction.CounterspellKind
	if check.countered {
		// The spell is gone: the other reactors have nothing left to counter.
		if err := s.closeGroup(ctx, c, w, reaction.ReasonTriggerGone); err != nil {
			return err
		}
	}
	names := s.namesAt(ctx, c)
	got.result.Result = &playv1.ReactionResult_Counterspell{Counterspell: &playv1.CounterspellResult{
		Countered: check.countered, SpellKey: t.Spell, SpellNamePt: names(t.Spell), SpellLevel: t.Level, Check: check.roll, CheckDc: clamp32(check.dc, 0, math.MaxInt32),
	}}
	c.characterID = &reactor.CharacterID
	return nil
}

// counterspellSlotOf is the reactor's kit and the slot its answer spends: the reaction must be free,
// and a free slot of the 3rd level or above must be the one chosen.
func (s *Service) counterspellSlotOf(ctx context.Context, c *combatTx, reactor playdb.Combatant, req *playv1.AnswerReactionRequest) (kit, slotRef, error) {
	if reactor.ReactionUsed {
		return kit{}, slotRef{}, errEncounter(playv1.EncounterBlockedReason_ENCOUNTER_BLOCKED_REASON_REACTION_USED, "the reaction is already used")
	}
	k, ok, err := s.kitOf(ctx, c.tx, c.session.CampaignID, reactor)
	if err != nil {
		return kit{}, slotRef{}, err
	}
	var free []*rulesv1.SlotChoice
	if ok && k.canReact() {
		free = k.slotsFor(spellCounterspell)
	}
	if len(free) == 0 {
		return kit{}, slotRef{}, errEncounter(playv1.EncounterBlockedReason_ENCOUNTER_BLOCKED_REASON_NO_SLOT, "there is no free spell slot for Contramágica",
			func(b *playv1.EncounterBlocked) { b.MinLevel = reaction.CounterspellLevel })
	}
	slot, err := pickSlot(req.GetSlot(), free)
	return k, slot, err
}

// counterspellOutcome is whether the spell is countered, the DC it was up against and the ability check
// that decided it (nil when the slot was enough).
type counterspellOutcome struct {
	countered bool
	dc        int
	roll      *playv1.DiceRoll
}

// counterspellCheck decides the counter: a slot at least as high as the spell counters it; above the
// slot it is an ability check with the spellcasting ability, which the event keeps.
func (s *Service) counterspellCheck(ctx context.Context, c *combatTx, req *playv1.AnswerReactionRequest, reactor playdb.Combatant, k kit, slot slotRef, t windowTrigger, re *reactionEvent) (counterspellOutcome, error) {
	auto, dc := reaction.Counterspell(int(slot.Level), int(t.Level))
	out := counterspellOutcome{countered: auto, dc: dc}
	if auto {
		return out, nil
	}
	// The spell is above the slot: an ability check with the spellcasting ability.
	in, err := s.reactionRoll(ctx, c, req, 20, reactor)
	if err != nil {
		return out, err
	}
	face, roll, err := s.d20(in, k.st.CastingMod)
	if err != nil {
		return out, err
	}
	out.countered = roll.Total >= dc
	re.Check, re.CheckDC = clamp32(roll.Total, math.MinInt32, math.MaxInt32), clamp32(dc, 0, math.MaxInt32)
	out.roll = diceRoll(1, 20, []int32{clamp32(face, 1, 20)}, clamp32(k.st.CastingMod, math.MinInt32, math.MaxInt32), re.Check, roll.Physical)
	return out, nil
}

// closeGroup closes the other open windows of a window's group by themselves.
func (s *Service) closeGroup(ctx context.Context, c *combatTx, w playdb.ReactionWindow, reason reaction.Reason) error {
	all, err := c.q.ListReactionWindowsOfGroup(ctx, playdb.ListReactionWindowsOfGroupParams{EncounterID: c.enc.ID, GroupID: w.GroupID})
	if err != nil {
		return fmt.Errorf("list the windows of a trigger: %w", err)
	}
	for _, o := range all {
		if o.ID == w.ID || o.Status != windowOpen {
			continue
		}
		if _, err := s.closeWindow(ctx, c, o, windowClosed, reason, windowOutcome{}, ""); err != nil {
			return err
		}
	}
	return nil
}

// reactionRoll reads how a reaction's die is rolled: in the app or typed from a
// physical die, as the campaign's dice setting allows a player (RN-18). REACTION_NEEDS_ROLL
// when the answer sent none.
func (s *Service) reactionRoll(ctx context.Context, c *combatTx, req *playv1.AnswerReactionRequest, sides int, reactor playdb.Combatant) (rollInput, error) {
	var in rollInput
	switch r := req.GetRoll().(type) {
	case *playv1.AnswerReactionRequest_RollInApp:
		if !r.RollInApp {
			return in, connect.NewError(connect.CodeInvalidArgument, errors.New("roll_in_app must be true"))
		}
		in.inApp = true
	case *playv1.AnswerReactionRequest_Typed:
		if r.Typed < 1 || int(r.Typed) > sides {
			return in, connect.NewError(connect.CodeInvalidArgument, fmt.Errorf("typed must be 1 to %d", sides))
		}
		in.typed = int(r.Typed)
	default:
		return in, errEncounter(playv1.EncounterBlockedReason_ENCOUNTER_BLOCKED_REASON_REACTION_NEEDS_ROLL, "this answer needs a roll")
	}
	if !c.master && reactor.Kind == kindPlayer {
		m := authz.Membership{CampaignID: c.session.CampaignID, UserID: c.actorUserID, Role: authz.RolePlayer}
		if err := s.mustRollThisWay(ctx, c.tx, m, in); err != nil {
			return in, err
		}
	}
	return in, nil
}

// namesAt is the content's Portuguese names, read inside the change's transaction.
func (s *Service) namesAt(ctx context.Context, c *combatTx) func(key string) string {
	namer, err := s.namerFor(ctx, c.tx, c.session.CampaignID)
	if err != nil {
		return func(string) string { return "" }
	}
	return namer
}
