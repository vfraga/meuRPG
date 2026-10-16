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
	"github.com/PuraFome/meuRPG/backend/internal/platform/dice"
	"github.com/PuraFome/meuRPG/backend/internal/play/link"
	"github.com/PuraFome/meuRPG/backend/internal/play/playdb"
	"github.com/PuraFome/meuRPG/backend/internal/rules"
	"github.com/PuraFome/meuRPG/backend/internal/rules/reaction"
)

// The table maneuvers (the superiority_die effect, rules/maneuvers.go) that are not the
// damage extras of combat_parts.go: the die of a grapple a hit makes possible, and the
// reduction of a melee hit's damage.

// maneuverOf finds the character's maneuver of that key and kind.
func maneuverOf(sheet link.Sheet, key, applies string) (link.Maneuver, bool) {
	i := slices.IndexFunc(sheet.Traits.Maneuvers, func(m link.Maneuver) bool { return m.Key == key && m.Applies == applies })
	if i < 0 {
		return link.Maneuver{}, false
	}
	return sheet.Traits.Maneuvers[i], true
}

// markMeleeHit records that the attacker hit with a melee weapon attack this turn, which a
// maneuver that starts a grapple needs. The attack's event keeps the mark it replaces, so
// that the master's undo of the hit puts that one back.
func markMeleeHit(ctx context.Context, c *combatTx, attacker playdb.Combatant, attack link.Attack, ev *actionEvent) error {
	if attacker.Kind != kindPlayer || !attack.Weapon || !attack.Melee {
		return nil
	}
	key := turnKey(c.enc)
	if err := c.q.SetCombatantMeleeHitTurn(ctx, playdb.SetCombatantMeleeHitTurnParams{ID: attacker.ID, MeleeHitTurn: &key}); err != nil {
		return fmt.Errorf("note the melee hit: %w", err)
	}
	ev.MeleeHit, ev.MeleeHitBefore = true, deref(attacker.MeleeHitTurn)
	return nil
}

// grappleManeuverRefusal says why a maneuver cannot start a grapple now, nil when it can:
// no bonus action, no melee hit this turn, no use. The master is not held to the economy.
func (s *Service) grappleManeuverRefusal(c *combatTx, who playdb.Combatant, usesLeft int32, master bool) error {
	switch {
	case who.BonusActionUsed && !master:
		return errEncounter(playv1.EncounterBlockedReason_ENCOUNTER_BLOCKED_REASON_BONUS_ACTION_USED, "the bonus action of this turn is used")
	case deref(who.MeleeHitTurn) != turnKey(c.enc):
		return errEncounter(playv1.EncounterBlockedReason_ENCOUNTER_BLOCKED_REASON_NO_MELEE_HIT, "no melee weapon hit this turn")
	case usesLeft <= 0:
		return errEncounter(playv1.EncounterBlockedReason_ENCOUNTER_BLOCKED_REASON_NO_USES, "no uses left")
	}
	return nil
}

// useGrappleManeuver checks and spends what a grapple started by a maneuver costs (the
// bonus action and one use) and returns the maneuver.
func (s *Service) useGrappleManeuver(ctx context.Context, c *combatTx, campaignID string, who playdb.Combatant, sheet link.Sheet, key string, master bool) (link.Maneuver, error) {
	if who.Kind != kindPlayer {
		return link.Maneuver{}, connect.NewError(connect.CodeInvalidArgument, errors.New("only a player's character uses a maneuver"))
	}
	m, ok := maneuverOf(sheet, key, rules.ManeuverGrapple)
	if !ok {
		return link.Maneuver{}, connect.NewError(connect.CodeInvalidArgument, errors.New("the character has no such grapple maneuver"))
	}
	vit, err := s.vitals.GetVitalsTx(ctx, c.tx, campaignID, who.CharacterID)
	if err != nil {
		return link.Maneuver{}, err
	}
	if err := s.grappleManeuverRefusal(c, who, usesLeftOf(vit)[m.Resource], master); err != nil {
		return link.Maneuver{}, err
	}
	if _, err := s.spendResource(ctx, c, who.CharacterID, m.Resource, 1); err != nil {
		return link.Maneuver{}, err
	}
	if err := c.q.SetCombatantEconomy(ctx, playdb.SetCombatantEconomyParams{
		ID: who.ID, ActionUsed: who.ActionUsed, BonusActionUsed: true, ReactionUsed: who.ReactionUsed, Dashed: who.Dashed,
	}); err != nil {
		return link.Maneuver{}, fmt.Errorf("spend the bonus action: %w", err)
	}
	return m, nil
}

// rollManeuverDie rolls the die of a maneuver in the app, or reads the face typed for a
// physical die (1 to its sides).
func (s *Service) rollManeuverDie(m link.Maneuver, inApp bool, typed int32) (int32, error) {
	if inApp {
		res, err := dice.Roll(s.roller, dice.Expr{Count: 1, Sides: m.Sides})
		if err != nil {
			return 0, fmt.Errorf("roll the maneuver die: %w", err)
		}
		return clamp32(res.Total, 1, 100), nil
	}
	if typed < 1 || int(typed) > m.Sides {
		return 0, connect.NewError(connect.CodeInvalidArgument, fmt.Errorf("maneuver_face must be 1 to %d", m.Sides))
	}
	return typed, nil
}

// addManeuverDie puts the die on the roll: it counts in the total and the view names it.
func addManeuverDie(r *contestRoll, m link.Maneuver, face int32) {
	r.Total += face
	r.ManeuverKey, r.ManeuverName, r.ManeuverSides, r.ManeuverFace = m.Key, m.NamePT, clamp32(m.Sides, 0, 100), face
}

// grappleManeuverOptions lists, for the options of a grapple, the character's grapple
// maneuvers with whether each can be used now and why not. gate is why the combatant cannot
// act now (nil when it can).
func (s *Service) grappleManeuverOptions(ctx context.Context, campaignID string, enc playdb.Encounter, who playdb.Combatant, sheet link.Sheet, gate *rulesv1.DisabledReason, master bool) ([]*playv1.ManeuverGrappleOption, error) {
	if who.Kind != kindPlayer {
		return nil, nil
	}
	var out []*playv1.ManeuverGrappleOption
	var left map[string]int32
	for _, m := range sheet.Traits.Maneuvers {
		if m.Applies != rules.ManeuverGrapple {
			continue
		}
		if left == nil {
			vit, err := s.vitals.GetVitals(ctx, campaignID, who.CharacterID)
			if err != nil {
				return nil, err
			}
			left = usesLeftOf(vit)
		}
		opt := &playv1.ManeuverGrappleOption{Key: m.Key, NamePt: m.NamePT, Sides: clamp32(m.Sides, 0, 100), UsesLeft: left[m.Resource], Enabled: true}
		switch {
		case gate != nil:
			opt.Enabled, opt.Reason = false, gate
		case who.BonusActionUsed && !master:
			opt.Enabled, opt.Reason = false, &rulesv1.DisabledReason{Code: rulesv1.DisabledReasonCode_DISABLED_REASON_CODE_BONUS_ACTION_USED}
		case left[m.Resource] <= 0:
			opt.Enabled, opt.Reason = false, &rulesv1.DisabledReason{Code: rulesv1.DisabledReasonCode_DISABLED_REASON_CODE_NO_USES}
		case deref(who.MeleeHitTurn) != turnKey(enc):
			opt.Enabled, opt.NoMeleeHit = false, true
		}
		out = append(out, opt)
	}
	return out, nil
}

// reduceOptionsOf lists the maneuvers a reactor can use now on a melee hit: its own, with a
// use left (the reaction itself is checked by canReact).
func reduceOptionsOf(k kit) []link.ReduceManeuver {
	if k.who.Kind != kindPlayer {
		return nil
	}
	return slices.DeleteFunc(slices.Clone(k.st.Reduce), func(o link.ReduceManeuver) bool { return k.resourceLeft(o.Resource) <= 0 })
}

// meleeWeaponAttack says an attack of the attacker's sheet is a melee weapon attack: not a
// ranged weapon and not a spell.
func (s *Service) meleeWeaponAttack(ctx context.Context, c *combatTx, attacker playdb.Combatant, key string) (bool, error) {
	sheet, err := s.sheetOf(ctx, c.tx, c.session.CampaignID, attacker)
	if connect.CodeOf(err) == connect.CodeNotFound {
		return false, nil
	}
	if err != nil {
		return false, err
	}
	i := slices.IndexFunc(sheet.Attacks, func(a link.Attack) bool { return a.Key == key })
	return i >= 0 && sheet.Attacks[i].Melee && !sheet.Attacks[i].Spell, nil
}

// answerManeuverReduce: the damage of a melee hit drops by the maneuver's die plus its ability
// modifier, for the reaction and one use of the maneuver's resource.
func (s *Service) answerManeuverReduce(ctx context.Context, c *combatTx, w playdb.ReactionWindow, reactor playdb.Combatant, use bool, req *playv1.AnswerReactionRequest, got *answered) error {
	if !use {
		return s.passWindow(ctx, c, w, reactor, got)
	}
	k, err := s.reactorKit(ctx, c, reactor)
	if err != nil {
		return err
	}
	i := slices.IndexFunc(k.st.Reduce, func(o link.ReduceManeuver) bool { return o.Key == req.GetManeuverKey() })
	if i < 0 {
		return connect.NewError(connect.CodeInvalidArgument, errors.New("maneuver_key must be one of the prompt's options"))
	}
	opt := k.st.Reduce[i]
	if k.resourceLeft(opt.Resource) <= 0 {
		return errEncounter(playv1.EncounterBlockedReason_ENCOUNTER_BLOCKED_REASON_NO_USES, "no uses left")
	}
	in, err := s.reactionRoll(ctx, c, req, opt.Sides, reactor)
	if err != nil {
		return err
	}
	face, r, err := s.dieRoll(in, opt.Sides)
	if err != nil {
		return err
	}
	t := windowTriggerOf(w)
	reduction := clamp32(max(face+opt.Mod, 0), 0, math.MaxInt32)
	left := clamp32(max(int(t.Damage)-int(reduction), 0), 0, math.MaxInt32)
	if err := s.spendReaction(ctx, c, reactor); err != nil {
		return err
	}
	if _, err := s.spendResource(ctx, c, reactor.CharacterID, opt.Resource, 1); err != nil {
		return err
	}
	roll := diceRoll(1, clamp32(opt.Sides, 0, 100), []int32{clamp32(face, 1, 100)}, clamp32(opt.Mod, math.MinInt32, math.MaxInt32), reduction, r.Physical)
	got.ev = actionEvent{
		Round: c.enc.Round, Secret: reactor.Hidden, Actor: reactor.ID, Target: t.Actor, ReactionBefore: reactor.ReactionUsed,
		ManeuverSpent: []string{opt.Resource},
		Reaction: &reactionEvent{
			Kind: w.Kind, Window: w.ID, Used: true, ByMaster: c.master, Before: t.Damage, After: left,
			Die: clamp32(face, 1, 100), DieSides: clamp32(opt.Sides, 0, 100), ManeuverName: opt.NamePT, Resource: opt.Resource,
		},
	}
	got.result = &playv1.ReactionResult{
		Kind: playv1.ReactionKind_REACTION_KIND_MANEUVER_REDUCE, ByMaster: c.master, Used: true,
		Result: &playv1.ReactionResult_ManeuverReduce{ManeuverReduce: &playv1.ManeuverReduceResult{
			ManeuverKey: opt.Key, NamePt: opt.NamePT, Reduction: roll, DamageBefore: t.Damage, DamageAfter: left,
		}},
	}
	c.lastAnswered = reaction.ManeuverReduceKind
	c.characterID = &reactor.CharacterID
	_, err = s.closeWindow(ctx, c, w, windowAnswered, reaction.ReasonNone, windowOutcome{ByMaster: c.master, Used: true, Reduction: reduction, Before: t.Damage, After: left}, reaction.ManeuverReduceKind)
	return err
}
