package play

import (
	"context"
	"errors"
	"fmt"

	"connectrpc.com/connect"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"

	playv1 "github.com/PuraFome/meuRPG/backend/gen/meurpg/play/v1"
	"github.com/PuraFome/meuRPG/backend/internal/authz"
	"github.com/PuraFome/meuRPG/backend/internal/platform/uid"
	"github.com/PuraFome/meuRPG/backend/internal/play/consumable"
	"github.com/PuraFome/meuRPG/backend/internal/play/playdb"
	"github.com/PuraFome/meuRPG/backend/internal/rules"
	"github.com/PuraFome/meuRPG/backend/internal/rules/combat"
)

// A consumable used in a running combat (SRD 5.1, "Potions" and "Other Activity on Your Turn"): drinking or giving a potion, every
// potion, one that is only text included (an oil is none: see rules.Content.IsPotion), is the user's action, taken on its turn
// like any action (the master takes it for the character, bound by the same turn and action). The receiver of a gift is within 5
// feet of the giver (a player's character is never hidden from the party). A heal goes through the combat's own heal, and a
// resistance is a lasting effect of the receiver's combatant that lasts the hour in rounds.

// consumableFight is the combat a use happens in: its transaction, the combatants, and the user's and the receiver's combatant,
// nil for a character that is not a combatant of it.
type consumableFight struct {
	c           *combatTx
	cs          []playdb.Combatant
	giver, recv *playdb.Combatant
}

func errNotAdjacent() error {
	return errEncounter(playv1.EncounterBlockedReason_ENCOUNTER_BLOCKED_REASON_TARGET_NOT_ADJACENT, "the receiver is not within 5 feet of the giver, or does not fight in the combat")
}

func errItemGone() error {
	return errEncounter(playv1.EncounterBlockedReason_ENCOUNTER_BLOCKED_REASON_ITEM_GONE, "the item is no longer on the sheet")
}

// giftReach is the distance to a receiver the giver may hand a potion to, and whether it may: within 5 feet (a combat with no
// map has no distance). A receiver is a player's character, and a player's combatant is never hidden, so no creature hidden from
// the giver can be one.
func giftReach(enc playdb.Encounter, giver, recv playdb.Combatant) (int32, bool) {
	switch {
	case recv.ID == giver.ID, isTheatre(enc):
		return 0, true
	}
	d, both := distanceFt(giver, recv)
	return d, both && d <= combat.MeleeReachFt
}

// openFight opens the combat the use happens in, or nil when it is none of the user's or the receiver's. Only a potion (SRD 5.1,
// "Potions", every one, whether or not the app applies it) is an action: in a running combat it needs the user's turn and an action
// not yet spent, a receiver within 5 feet, and it spends the action; a user or a receiver that is the only one of the two in the
// combat, a receiver out of reach, a turn that is not the user's and an action already used refuse, with nothing written. A combat
// in setup has no turn, and any other consumable (a scroll, an oil) is spent and logged on any turn: neither costs a turn or an
// action. A receiver that is dead (three failed death saves) is refused whatever the item.
func (s *Service) openFight(ctx context.Context, tx pgx.Tx, q *playdb.Queries, h *hints, session playdb.GameSession, use consumableCall, potion bool) (*consumableFight, error) {
	enc, cs, found, err := openCombatants(ctx, q, session.ID)
	if err != nil || !found {
		return nil, err
	}
	c, err := s.openTx(ctx, combatTx{tx: tx, q: q, session: session, enc: enc, now: s.now(), hints: h, actorUserID: use.m.UserID, svc: s})
	if err != nil {
		return nil, err
	}
	giver, recv := wearerIn(cs, use.charText), wearerIn(cs, use.target)
	if giver == nil && (recv == nil || !potion) { // the combat has no part in it: no effect on a combatant and no line of the user's, but a dead receiver is refused
		return nil, s.refuseTheDeadReceiver(ctx, c, recv)
	}
	acts := potion && enc.Status == statusActive
	if acts && (giver == nil || recv == nil) {
		return nil, errNotAdjacent()
	}
	f := &consumableFight{c: c, cs: cs, giver: giver, recv: recv}
	if err := s.gateConsumable(ctx, f, acts); err != nil {
		return nil, err
	}
	return f, nil
}

// gateConsumable is what a use needs of the combat: for a potion in a running combat (acts) the user's turn, a receiver within reach
// and the action; whatever the item, a receiver that is not dead.
func (s *Service) gateConsumable(ctx context.Context, f *consumableFight, acts bool) error {
	if acts {
		if err := s.mustActNow(ctx, f.c, *f.giver); err != nil {
			return err
		}
		if _, ok := giftReach(f.c.enc, *f.giver, *f.recv); !ok {
			return errNotAdjacent()
		}
	}
	if err := s.refuseTheDeadReceiver(ctx, f.c, f.recv); err != nil {
		return err
	}
	if acts {
		return s.payConsumableAction(ctx, f.c, *f.giver)
	}
	return nil
}

// refuseTheDeadReceiver refuses a receiver that is a combatant and dead (three failed death saves): no item reaches it, whether or not
// the user fights in the combat. A nil receiver is no combatant of it, and nothing is refused.
func (s *Service) refuseTheDeadReceiver(ctx context.Context, c *combatTx, recv *playdb.Combatant) error {
	if recv == nil {
		return nil
	}
	return s.refuseTheDead(ctx, c, []playdb.Combatant{*recv})
}

// payConsumableAction spends the action of the turn for a potion. Activating a potion is an action, not the object interaction
// of Use an Object (SRD 5.1, "Potions"), so the extra action of Haste does not pay for it: with the action used the use is refused.
func (s *Service) payConsumableAction(ctx context.Context, c *combatTx, who playdb.Combatant) error {
	if who.ActionUsed {
		return errEncounter(playv1.EncounterBlockedReason_ENCOUNTER_BLOCKED_REASON_ACTION_USED, "the action of this turn is used")
	}
	if err := c.q.SetCombatantEconomy(ctx, playdb.SetCombatantEconomyParams{
		ID: who.ID, ActionUsed: true, BonusActionUsed: who.BonusActionUsed, ReactionUsed: who.ReactionUsed, Dashed: who.Dashed,
	}); err != nil {
		return fmt.Errorf("spend the action: %w", err)
	}
	return nil
}

// receives says the combat has the receiver as a combatant: the effect of the potion is the combatant's. False for no fight.
func (f *consumableFight) receives() bool {
	return f != nil && f.recv != nil
}

// name the combatants in the event: the user's combatant, and the receiver's when it is another. The user is a combatant.
func (f *consumableFight) name(ev *consumableUsedEvent) {
	ev.Round, ev.Actor = f.c.enc.Round, f.giver.ID
	if f.recv != nil && f.recv.ID != f.giver.ID {
		ev.TargetCombatant = f.recv.ID
	}
}

// finish bumps the combat's revision and tells the streams of the combat and of its log once the transaction commits.
func (f *consumableFight) finish(ctx context.Context, s *Service, h *hints) error {
	if err := touch(ctx, f.c); err != nil {
		return err
	}
	campaignID, enc := f.c.session.CampaignID, f.c.enc
	h.after(func(ctx context.Context, say sayFn) {
		s.tellEncounterChanged(ctx, say, campaignID, enc)
		s.sayLogChange(ctx, say, campaignID, enc.ID, true, false)
	})
	return nil
}

// heal heals the receiver's combatant with the combat's heal and says the hit points it regained.
func (f *consumableFight) heal(ctx context.Context, s *Service, total int32) (int32, error) {
	hit, _, err := s.healCombatant(ctx, f.c, *f.recv, total)
	return hit.Amount, err
}

// resist puts the potion's resistance on the receiver's combatant for the hour, in rounds; a second potion of the same kind
// ends the first and starts the hour again.
func (f *consumableFight) resist(ctx context.Context, s *Service, potion rules.PotionEffect, key string) error {
	group := uuid.New().String()
	if err := s.dropSameSpell(ctx, f.c, f.cs, f.recv.ID, key, group); err != nil {
		return err
	}
	_, err := s.addEffects(ctx, f.c, f.cs, effectSpec{
		key: key, sourceKind: sourceKindItem, group: group, targets: []playdb.Combatant{*f.recv}, audience: rules.EffectVisibilityOwner,
		dur:       durationSpec{Kind: rules.EffectDurationRounds, Rounds: clamp32(potion.Rounds, 0, 1<<20), Anchor: f.recv.ID},
		modifiers: []rules.EffectModifier{consumable.Resistance(potion)},
	})
	return err
}

// ListConsumableTargets implements playv1connect.ChargesServiceHandler: who the character may give the consumable to.
func (s *Service) ListConsumableTargets(ctx context.Context, req *connect.Request[playv1.ListConsumableTargetsRequest]) (*connect.Response[playv1.ListConsumableTargetsResponse], error) {
	m, err := authz.RequireCampaignMember(ctx, req.Msg.GetCampaignId())
	if err != nil {
		return nil, err
	}
	charID, ok := uid.Canonical(req.Msg.GetCharacterId())
	if !ok {
		return nil, errCharacterNotFound()
	}
	if _, err := s.requireItemCharacter(ctx, nil, m, charID); err != nil {
		return nil, err
	}
	potion, err := s.entryIsAPotion(ctx, m.CampaignID, charID, req.Msg.GetEntryId())
	if err != nil {
		return nil, err
	}
	rows, err := s.consumableTargets(ctx, m.CampaignID, charID, potion, m.Role == authz.RoleMaster)
	if err != nil {
		return nil, s.dbError(ctx, "list the consumable's targets", err)
	}
	return connect.NewResponse(&playv1.ListConsumableTargetsResponse{Targets: rows}), nil
}

// entryIsAPotion says whether the entry of the character's sheet is a potion (rules.Content.IsPotion), the one consumable that is
// an action. ITEM_GONE for an entry that is no longer there, invalid_argument for one that is no consumable.
func (s *Service) entryIsAPotion(ctx context.Context, campaignID, characterID, entryID string) (bool, error) {
	var potion bool
	err := pgx.BeginFunc(ctx, s.pool, func(tx pgx.Tx) error {
		item, err := s.vitals.ConsumableItem(ctx, tx, campaignID, characterID, entryID)
		if err != nil {
			return err
		}
		content, err := s.roster.RulesContent(ctx, tx, campaignID)
		if err != nil {
			return err
		}
		potion = content.IsPotion(item.GetKey())
		return nil
	})
	if connect.CodeOf(err) == connect.CodeNotFound {
		return false, errItemGone()
	}
	return potion, err
}

// consumableTargets are the receivers the use of the item would accept from the character, as the viewer reads them (master). A
// potion in a running combat is an action within reach: from a combatant it goes to the player's characters it reaches, and from a
// character that does not fight to none that does. Any other case has no distance and no turn: the party's characters not dead.
func (s *Service) consumableTargets(ctx context.Context, campaignID, characterID string, potion, master bool) ([]*playv1.ConsumableTarget, error) {
	enc, cs, found := playdb.Encounter{}, []playdb.Combatant(nil), false
	open, err := s.queries.GetOpenGameSession(ctx, campaignID)
	if err == nil {
		enc, cs, found, err = openCombatants(ctx, s.queries, open.ID)
	}
	if err != nil && !errors.Is(err, pgx.ErrNoRows) {
		return nil, err
	}
	fight := potion && found && enc.Status == statusActive // the potion is an action in a running combat
	if giver := wearerIn(cs, characterID); fight && giver != nil {
		return s.fightTargets(ctx, campaignID, enc, cs, *giver, master)
	}
	party, err := s.roster.CombatParty(ctx, nil, campaignID)
	if err != nil {
		return nil, err
	}
	var out []*playv1.ConsumableTarget
	for _, p := range party {
		if p.ID == characterID {
			continue
		}
		if ok, err := s.partyMayReceive(ctx, campaignID, wearerIn(cs, p.ID), fight, master); err != nil {
			return nil, err
		} else if ok {
			out = append(out, consumable.Target(p.ID, p.Name, 0, playv1.ConsumableTargetState_CONSUMABLE_TARGET_STATE_UNSPECIFIED))
		}
	}
	return out, nil
}

// partyMayReceive says whether a character of the party that is not the user's combatant may be given the item: one that fights is
// out of reach of a user outside the combat when the potion is an action there (fight), and is left out when it is dead as read.
func (s *Service) partyMayReceive(ctx context.Context, campaignID string, fights *playdb.Combatant, fight, master bool) (bool, error) {
	if fights == nil {
		return true, nil
	}
	listed, _, err := s.receiverStanding(ctx, campaignID, *fights, master)
	return listed && !fight, err
}

// receiverStanding says whether a combatant is on a list of receivers and whether it is down, as the viewer reads it.
func (s *Service) receiverStanding(ctx context.Context, campaignID string, c playdb.Combatant, master bool) (listed, down bool, err error) {
	v, err := s.vitals.GetVitals(ctx, campaignID, c.CharacterID)
	if err != nil && connect.CodeOf(err) != connect.CodeNotFound {
		return false, false, err
	}
	listed, down = consumable.Standing(c.Defeated || err != nil, c.DeathFailures, master, v) // an error left: its character is marked dead
	return listed, down, nil
}

// fightTargets are the player's characters of the combat that the giver reaches.
func (s *Service) fightTargets(ctx context.Context, campaignID string, enc playdb.Encounter, cs []playdb.Combatant, giver playdb.Combatant, master bool) ([]*playv1.ConsumableTarget, error) {
	var out []*playv1.ConsumableTarget
	for _, c := range cs {
		if c.ID == giver.ID || c.Kind != kindPlayer || c.Dismissed {
			continue
		}
		d, ok := giftReach(enc, giver, c)
		if !ok {
			continue
		}
		listed, down, err := s.receiverStanding(ctx, campaignID, c, master)
		if err != nil {
			return nil, err
		}
		if listed {
			out = append(out, consumable.Target(c.CharacterID, c.Label, d, consumable.FightState(down)))
		}
	}
	return out, nil
}
