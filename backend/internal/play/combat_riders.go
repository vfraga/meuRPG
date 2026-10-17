package play

import (
	"context"
	"errors"
	"fmt"
	"math"
	"slices"

	"connectrpc.com/connect"

	playv1 "github.com/PuraFome/meuRPG/backend/gen/meurpg/play/v1"
	"github.com/PuraFome/meuRPG/backend/internal/authz"
	"github.com/PuraFome/meuRPG/backend/internal/platform/idem"
	"github.com/PuraFome/meuRPG/backend/internal/play/link"
	"github.com/PuraFome/meuRPG/backend/internal/play/playdb"
	"github.com/PuraFome/meuRPG/backend/internal/rules"
	"github.com/PuraFome/meuRPG/backend/internal/rules/combat"
)

// The monk's riders on a hit (SRD 5.1, Monk).
//
// Open Hand technique (Way of the Open Hand, level 3): whenever the monk hits with one of the
// attacks Flurry of Blows grants, it may impose one effect on the target: a Dexterity save or
// be knocked prone, a Strength save or be pushed up to 15 ft, or no reactions until the end of
// the monk's next turn. Stunning Strike (level 5): on a melee weapon hit, 1 ki point; the
// target makes a Constitution save or is stunned until the end of the monk's next turn. Both
// use the ki save DC. A hit that qualifies opens one offer (hit_riders) per rider, open until
// answered or the round ends. The server rolls the target's save, as it does for a spell.
// The push is the master's to place: the app does not move the token (forced move).

const (
	riderOpenHand = "open_hand"
	riderStunning = "stunning_strike"

	choiceProne       = "prone"
	choicePush        = "push"
	choiceNoReactions = "no_reactions"
	choiceStun        = "stun"
	choiceDecline     = "decline"

	effectStunningStrike = "effect:stunning-strike"
	effectNoReactions    = "effect:open-hand-no-reactions"

	riderSaved   = "saved"
	riderFailed  = "failed"
	riderApplied = "applied"
	riderDecl    = "declined"
)

// riderEvent is the line of an answered rider.
type riderEvent struct {
	Choice string `json:"choice"`
	Result string `json:"result"`
	D20    int32  `json:"d20,omitempty"`
	Total  int32  `json:"total,omitempty"`
	DC     int32  `json:"dc,omitempty"`
}

// ridersFor says which riders a hit offers: Open Hand for a Flurry of Blows hit, Stunning
// Strike for a melee weapon hit; only a player's character with the feature.
func ridersFor(attacker playdb.Combatant, attack link.Attack, sheet link.Sheet, flurry bool) []string {
	if attacker.Kind != kindPlayer || attack.Spell || !attack.Melee || attack.Save || sheet.Traits.KiSaveDC <= 0 {
		return nil
	}
	var out []string
	if flurry && sheet.Traits.OpenHand {
		out = append(out, riderOpenHand)
	}
	if sheet.Traits.StunningStrike {
		out = append(out, riderStunning)
	}
	return out
}

// offerRiders opens the offers of a hit.
func (s *Service) offerRiders(ctx context.Context, c *combatTx, attacker, target playdb.Combatant, attack link.Attack, sheet link.Sheet, flurry bool) error {
	if target.Side == attacker.Side {
		return nil
	}
	for _, kind := range ridersFor(attacker, attack, sheet, flurry) {
		if _, err := c.q.InsertHitRider(ctx, playdb.InsertHitRiderParams{
			EncounterID: c.enc.ID, AttackerID: attacker.ID, TargetID: target.ID, Kind: kind, Round: c.enc.Round, CreatedAt: c.now,
		}); err != nil {
			return fmt.Errorf("offer the rider: %w", err)
		}
	}
	return nil
}

func riderKindProto(kind string) playv1.HitRiderKind {
	if kind == riderOpenHand {
		return playv1.HitRiderKind_HIT_RIDER_KIND_OPEN_HAND
	}
	return playv1.HitRiderKind_HIT_RIDER_KIND_STUNNING_STRIKE
}

// hitRidersView builds the open offers the viewer reads: the master's all, a player's the
// ones of their own character, on a target they see (RN-10, RN-20).
func (d *encounterData) hitRidersView(v combatViewer, vitals map[string]*playv1.CharacterVitals) []*playv1.HitRiderOffer {
	var out []*playv1.HitRiderOffer
	for _, r := range d.riders {
		if r.Used || r.Round != d.enc.Round {
			continue
		}
		a, okA := combatantByID(d.cs, r.AttackerID)
		t, okT := combatantByID(d.cs, r.TargetID)
		// Only the master and the attacker's own player see the offer, and only on a target they see.
		mayRead := v.master || v.owns(a)
		if !okA || !okT || a.Defeated || !mayRead || !v.sees(t) {
			continue
		}
		offer := &playv1.HitRiderOffer{Id: r.ID, Kind: riderKindProto(r.Kind), AttackerId: r.AttackerID, TargetId: r.TargetID}
		for _, res := range vitals[a.CharacterID].GetResources() {
			if res.GetKey() == resKi {
				offer.KiLeft = max(res.GetTotal()-res.GetUsed(), 0)
			}
		}
		out = append(out, offer)
	}
	return out
}

// riderChoiceOf checks that the choice belongs to the rider.
func riderChoiceOf(kind string, in playv1.HitRiderChoice) (string, error) {
	var choice string
	switch in {
	case playv1.HitRiderChoice_HIT_RIDER_CHOICE_PRONE:
		choice = choiceProne
	case playv1.HitRiderChoice_HIT_RIDER_CHOICE_PUSH:
		choice = choicePush
	case playv1.HitRiderChoice_HIT_RIDER_CHOICE_NO_REACTIONS:
		choice = choiceNoReactions
	case playv1.HitRiderChoice_HIT_RIDER_CHOICE_STUN:
		choice = choiceStun
	case playv1.HitRiderChoice_HIT_RIDER_CHOICE_DECLINE:
		return choiceDecline, nil
	}
	ok := (kind == riderOpenHand && slices.Contains([]string{choiceProne, choicePush, choiceNoReactions}, choice)) ||
		(kind == riderStunning && choice == choiceStun)
	if !ok {
		return "", connect.NewError(connect.CodeInvalidArgument, errors.New("the choice is not one this offer has"))
	}
	return choice, nil
}

// riderSaveAbility is the ability of the target's save for a choice ("" for none).
func riderSaveAbility(choice string) string {
	switch choice {
	case choiceProne:
		return "dex"
	case choicePush:
		return "str"
	case choiceStun:
		return "con"
	}
	return ""
}

// UseHitRider implements playv1connect.CombatServiceHandler.
func (s *Service) UseHitRider(
	ctx context.Context,
	req *connect.Request[playv1.UseHitRiderRequest],
) (*connect.Response[playv1.UseHitRiderResponse], error) {
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
	riderID, err := parseCombatID(req.Msg.GetRiderId(), "rider")
	if err != nil {
		return nil, err
	}
	res, err := s.write(ctx, combatWrite{m: m, key: key, hash: idem.Hash(req.Msg), kind: eventHitRider, encounterID: encID}, func(c *combatTx) (any, error) {
		return s.answerRider(ctx, c, m, riderID, req.Msg.GetChoice())
	})
	if err != nil {
		return nil, s.dbError(ctx, "answer a rider", err)
	}
	out, err := s.finish(ctx, m, res, func(ctx context.Context, d *encounterData) {
		s.publishEncounterChanged(ctx, m.CampaignID, d.enc)
		s.publishLogChanged(ctx, m.CampaignID, d.enc.ID, true)
	})
	if err != nil {
		return nil, err
	}
	return connect.NewResponse(&playv1.UseHitRiderResponse{Encounter: out}), nil
}

// answerRider answers an offer: the choice is checked, applied (a decline spends nothing) and the
// offer closed.
func (s *Service) answerRider(ctx context.Context, c *combatTx, m authz.Membership, riderID string, in playv1.HitRiderChoice) (actionEvent, error) {
	cs, err := c.q.ListCombatants(ctx, c.enc.ID)
	if err != nil {
		return actionEvent{}, fmt.Errorf("list the combatants: %w", err)
	}
	row, attacker, target, err := s.riderOffer(ctx, c, m, cs, riderID)
	if err != nil {
		return actionEvent{}, err
	}
	choice, err := riderChoiceOf(row.Kind, in)
	if err != nil {
		return actionEvent{}, err
	}
	ev := actionEvent{Round: c.enc.Round, Secret: secretOf(attacker, target), Actor: attacker.ID, Target: target.ID, Key: row.Kind}
	re := &riderEvent{Choice: choice, Result: riderDecl}
	ev.Rider = re
	if choice != choiceDecline {
		if err := s.applyChoice(ctx, c, m, cs, row.Kind, attacker, target, choice, re); err != nil {
			return actionEvent{}, err
		}
	}
	return s.closeRider(ctx, c, row, attacker, choice, ev)
}

// riderOffer finds the open offer the caller may answer, with the monk and the target it is about:
// a player is not told whether an offer exists (RN-10).
func (s *Service) riderOffer(ctx context.Context, c *combatTx, m authz.Membership, cs []playdb.Combatant, riderID string) (playdb.HitRider, playdb.Combatant, playdb.Combatant, error) {
	v := c.viewer(m, cs)
	row, err := c.q.GetHitRider(ctx, playdb.GetHitRiderParams{EncounterID: c.enc.ID, ID: riderID})
	if err != nil {
		if !v.master {
			return row, playdb.Combatant{}, playdb.Combatant{}, connect.NewError(connect.CodePermissionDenied, errors.New("only the monk's player or the master may answer"))
		}
		return row, playdb.Combatant{}, playdb.Combatant{}, connect.NewError(connect.CodeNotFound, errors.New("offer not found"))
	}
	attacker, okA := combatantByID(cs, row.AttackerID)
	target, okT := combatantByID(cs, row.TargetID)
	if !okA || !okT {
		return row, attacker, target, connect.NewError(connect.CodeNotFound, errors.New("offer not found"))
	}
	if _, err := findCombatant(cs, attacker.ID, v); err != nil {
		return row, attacker, target, err
	}
	if err := v.mayAct(attacker); err != nil {
		return row, attacker, target, err
	}
	if row.Used || row.Round != c.enc.Round {
		return row, attacker, target, errEncounter(playv1.EncounterBlockedReason_ENCOUNTER_BLOCKED_REASON_UNSPECIFIED, "the offer was answered or its round is over")
	}
	return row, attacker, target, nil
}

// applyChoice checks that the monk still has the feature the offer is for and applies the choice.
func (s *Service) applyChoice(ctx context.Context, c *combatTx, m authz.Membership, cs []playdb.Combatant, kind string, attacker, target playdb.Combatant, choice string, re *riderEvent) error {
	sheet, err := s.sheetOf(ctx, c.tx, m.CampaignID, attacker)
	if err != nil {
		return err
	}
	if sheet.Traits.KiSaveDC <= 0 || (kind == riderOpenHand && !sheet.Traits.OpenHand) || (kind == riderStunning && !sheet.Traits.StunningStrike) {
		return errEncounter(playv1.EncounterBlockedReason_ENCOUNTER_BLOCKED_REASON_UNSPECIFIED, "the combatant does not have this feature")
	}
	return s.applyRider(ctx, c, m, cs, attacker, target, choice, int32(sheet.Traits.KiSaveDC), re) //nolint:gosec // a DC
}

// closeRider marks the offer used with the choice and touches the combat.
func (s *Service) closeRider(ctx context.Context, c *combatTx, row playdb.HitRider, attacker playdb.Combatant, choice string, ev actionEvent) (actionEvent, error) {
	if err := c.q.UseHitRider(ctx, playdb.UseHitRiderParams{EncounterID: c.enc.ID, ID: row.ID, Choice: &choice}); err != nil {
		return actionEvent{}, fmt.Errorf("close the offer: %w", err)
	}
	if err := touchEncounter(ctx, c); err != nil {
		return actionEvent{}, err
	}
	c.characterID = &attacker.CharacterID
	return ev, nil
}

// applyRider spends the ki of a Stunning Strike, rolls the target's save and applies what a
// failure does.
func (s *Service) applyRider(ctx context.Context, c *combatTx, m authz.Membership, cs []playdb.Combatant, attacker, target playdb.Combatant, choice string, dc int32, re *riderEvent) error {
	if choice == choiceStun {
		if err := s.spendStunKi(ctx, c, m, attacker); err != nil {
			return err
		}
	}
	re.DC = dc
	landed, err := s.rollRiderOutcome(ctx, c, m, target, choice, dc, re)
	if err != nil || !landed {
		return err
	}
	return s.riderConsequence(ctx, c, cs, attacker, target, choice, re)
}

// spendStunKi spends the ki point a Stunning Strike costs.
func (s *Service) spendStunKi(ctx context.Context, c *combatTx, m authz.Membership, attacker playdb.Combatant) error {
	k, ok, err := s.kitOf(ctx, c.tx, m.CampaignID, attacker)
	if err != nil {
		return err
	}
	if !ok || k.resourceLeft(resKi) < 1 {
		return errEncounter(playv1.EncounterBlockedReason_ENCOUNTER_BLOCKED_REASON_UNSPECIFIED, "no ki point left")
	}
	_, err = s.spendReactionResource(ctx, c, attacker, resKi)
	return err
}

// rollRiderOutcome sets what the rider's result says and tells whether the choice lands: a choice
// with no save lands, one with a save lands only when the target fails it.
func (s *Service) rollRiderOutcome(ctx context.Context, c *combatTx, m authz.Membership, target playdb.Combatant, choice string, dc int32, re *riderEvent) (bool, error) {
	ability := riderSaveAbility(choice)
	if ability == "" {
		re.Result = riderApplied
		return true, nil
	}
	saved, err := s.rollRiderSave(ctx, c, m, target, ability, dc, re)
	if err != nil {
		return false, err
	}
	if saved {
		re.Result = riderSaved
		return false, nil
	}
	re.Result = riderFailed
	return true, nil
}

// riderConsequence is what a choice does to a target that did not resist it.
func (s *Service) riderConsequence(ctx context.Context, c *combatTx, cs []playdb.Combatant, attacker, target playdb.Combatant, choice string, re *riderEvent) error {
	switch choice {
	case choiceProne:
		if !slices.Contains(target.Conditions, condProne) && len(target.Conditions) < maxConditions {
			return setConditionsOf(ctx, c, target, withCondition(target.Conditions, condProne))
		}
	case choiceNoReactions:
		return s.riderEffect(ctx, c, cs, attacker, target, effectNoReactions)
	case choiceStun:
		states, err := s.readStates(ctx, c.tx, c.enc.ID)
		if err != nil {
			return err
		}
		if combat.ImmuneTo(effectModsOf(states, target.ID), "condition:stunned") {
			re.Result = riderSaved
			return nil
		}
		return s.riderEffect(ctx, c, cs, attacker, target, effectStunningStrike)
	}
	return nil // the push: the master places the token
}

// riderEffect puts a lasting effect on the target that ends with the monk's next turn.
func (s *Service) riderEffect(ctx context.Context, c *combatTx, cs []playdb.Combatant, attacker, target playdb.Combatant, key string) error {
	content, err := s.contentOf(ctx, c)
	if err != nil {
		return err
	}
	def, ok := content.CombatEffect(key)
	if !ok {
		return fmt.Errorf("the effect %s is not in the catalog", key)
	}
	rows, err := s.addEffects(ctx, c, cs, effectSpec{
		key: key, sourceKind: "feature", def: def, caster: &attacker, targets: []playdb.Combatant{target},
		dur: durationSpec{Kind: rules.EffectDurationUntilEndOfTurnOf, Anchor: attacker.ID},
	})
	if err != nil {
		return err
	}
	return s.addedEvent(ctx, c, cs, rows, key)
}

// rollRiderSave has the server roll the target's saving throw, as a spell's save is rolled.
func (s *Service) rollRiderSave(ctx context.Context, c *combatTx, m authz.Membership, target playdb.Combatant, ability string, dc int32, re *riderEvent) (bool, error) {
	save, err := s.saveOf(ctx, c.tx, m.CampaignID, target, ability)
	if err != nil {
		return false, err
	}
	sheet, err := s.sheetOf(ctx, c.tx, m.CampaignID, target)
	if err != nil {
		return false, err
	}
	states, err := s.readStates(ctx, c.tx, c.enc.ID)
	if err != nil {
		return false, err
	}
	creature := creatureFacts(target, states, sheet.Traits)
	if combat.AutoFailsSave(creature, ability) {
		return false, nil
	}
	mode := combat.Resolve(combat.SaveMode(combat.SaveScene{Creature: creature, Ability: ability, EffectVisible: true}))
	d20, err := s.d20With(rollInput{inApp: true}, save.Bonus, mode)
	if err != nil {
		return false, err
	}
	re.D20, re.Total = clamp32(d20.Face(), 1, 20), clamp32(d20.Total, math.MinInt32, math.MaxInt32)
	return combat.SaveSucceeded(d20.Total, int(dc)), nil
}

// riderLogProto is the line of an answered rider: the target's roll and the DC only for the
// master and, when the target is a player's character, its player (RN-20: the monk's player
// reads how the save ended, never an NPC's roll).
func riderLogProto(r *riderEvent, numbers bool) *playv1.CombatLogHitRider {
	out := &playv1.CombatLogHitRider{}
	switch r.Choice {
	case choiceProne:
		out.Choice = playv1.HitRiderChoice_HIT_RIDER_CHOICE_PRONE
	case choicePush:
		out.Choice = playv1.HitRiderChoice_HIT_RIDER_CHOICE_PUSH
	case choiceNoReactions:
		out.Choice = playv1.HitRiderChoice_HIT_RIDER_CHOICE_NO_REACTIONS
	case choiceStun:
		out.Choice = playv1.HitRiderChoice_HIT_RIDER_CHOICE_STUN
	default:
		out.Choice = playv1.HitRiderChoice_HIT_RIDER_CHOICE_DECLINE
	}
	switch r.Result {
	case riderSaved:
		out.Result = playv1.HitRiderLogResult_HIT_RIDER_LOG_RESULT_SAVED
	case riderFailed:
		out.Result = playv1.HitRiderLogResult_HIT_RIDER_LOG_RESULT_FAILED
	case riderApplied:
		out.Result = playv1.HitRiderLogResult_HIT_RIDER_LOG_RESULT_APPLIED
	default:
		out.Result = playv1.HitRiderLogResult_HIT_RIDER_LOG_RESULT_DECLINED
	}
	if numbers && r.D20 > 0 {
		out.Numbers, out.D20, out.SaveTotal, out.Dc = true, r.D20, r.Total, r.DC
	}
	return out
}
