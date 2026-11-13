package play

import (
	"context"
	"errors"
	"fmt"
	"slices"

	"connectrpc.com/connect"

	playv1 "github.com/PuraFome/meuRPG/backend/gen/meurpg/play/v1"
	"github.com/PuraFome/meuRPG/backend/internal/authz"
	"github.com/PuraFome/meuRPG/backend/internal/platform/conv"
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
// The push moves the target's token away from the monk (combat_forced.go).

const (
	riderOpenHand = "open_hand"
	riderStunning = "stunning_strike"
	// riderCreature is the rider of a creature's attack (combat_creature_rider.go).
	riderCreature = "creature"

	choiceProne       = "prone"
	choicePush        = "push"
	choiceNoReactions = "no_reactions"
	choiceStun        = "stun"
	choiceDecline     = "decline"
	choiceApply       = "apply"

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
	// countedRoll is the number the d20 counted as when a minimum raised it.
	countedRoll
	// Pushed is the forced move of a push that landed (combat_forced.go).
	Pushed *forcedMove `json:"pushed,omitempty"`
	// A creature's rider: the action, the condition it gave (Given says it was given), the escape DC of a
	// grapple (the master's alone), the stat block's words for how long it lasts, whether the save repeats and
	// the damage the save governs, opened as pending damage.
	ActionKey     string `json:"action_key,omitempty"`
	ActionName    string `json:"action_name,omitempty"`
	ConditionKey  string `json:"condition_key,omitempty"`
	ConditionName string `json:"condition_name,omitempty"`
	Given         bool   `json:"given,omitempty"`
	EscapeDC      int32  `json:"escape_dc,omitempty"`
	Duration      string `json:"duration,omitempty"`
	RepeatSave    bool   `json:"repeat_save,omitempty"`
	// Pending and More are the pending damages the save governs (the first damage type, then the others): the log
	// hosts the damage lines that follow on this event, as a cast hosts those of its targets.
	Pending string   `json:"pending_id,omitempty"`
	More    []string `json:"more_pending_ids,omitempty"`
	// Resisted says the target failed the save and the master made it a success with Legendary Resistance
	// (Result is then saved, D20 and Total the roll that failed), and ResistLeft the uses it has left: the master's
	// alone (combat_legendary_hold.go).
	Resisted   bool  `json:"resisted,omitempty"`
	ResistLeft int32 `json:"resist_left,omitempty"`
	// save is the target's save as it was rolled, kept for the hold of a creature with Legendary Resistance; it is
	// never stored with the event.
	save *saveRoll
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
	switch kind {
	case riderOpenHand:
		return playv1.HitRiderKind_HIT_RIDER_KIND_OPEN_HAND
	case riderCreature:
		return playv1.HitRiderKind_HIT_RIDER_KIND_CREATURE
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
		// Only the master and the attacker's own player see the offer, and only on a target they see; a
		// creature's rider is the master's alone.
		mayRead := v.master || (v.owns(a) && r.Kind != riderCreature)
		if !okA || !okT || a.Defeated || !mayRead || !v.sees(t) {
			continue
		}
		offer := &playv1.HitRiderOffer{Id: r.ID, Kind: riderKindProto(r.Kind), AttackerId: r.AttackerID, TargetId: r.TargetID, ActionKey: conv.OrZero(r.ActionKey)}
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
	case playv1.HitRiderChoice_HIT_RIDER_CHOICE_APPLY:
		choice = choiceApply
	case playv1.HitRiderChoice_HIT_RIDER_CHOICE_DECLINE:
		return choiceDecline, nil
	}
	ok := (kind == riderCreature && choice == choiceApply) || (kind == riderOpenHand && slices.Contains([]string{choiceProne, choicePush, choiceNoReactions}, choice)) ||
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
	m, _, _, err := memberCall(ctx, req.Msg.GetCampaignId(), req.Msg.GetIdempotencyKey(), req.Msg.GetEncounterId())
	if err != nil {
		return nil, err
	}
	return s.useHitRider(ctx, m, req.Msg)
}

// useHitRider is UseHitRider for a member already known: the replay of a rider held for the master's Legendary
// Resistance runs it again as the one who answered (combat_reaction_hold.go).
func (s *Service) useHitRider(ctx context.Context, m authz.Membership, msg *playv1.UseHitRiderRequest) (*connect.Response[playv1.UseHitRiderResponse], error) {
	key, encID, err := parseCall(msg.GetIdempotencyKey(), msg.GetEncounterId())
	if err != nil {
		return nil, err
	}
	riderID, err := parseCombatID(msg.GetRiderId(), "rider")
	if err != nil {
		return nil, err
	}
	res, err := s.write(ctx, combatWrite{m: m, key: key, hash: idem.Hash(msg), kind: eventHitRider, encounterID: encID}, func(c *combatTx) (any, error) {
		return s.answerRider(ctx, c, m, msg, riderID)
	})
	if err != nil {
		return nil, s.dbError(ctx, "answer a rider", err)
	}
	out, err := s.finish(ctx, m, res, func(ctx context.Context, d *encounterData) {
		s.publishEncounterChanged(ctx, m.CampaignID, d.enc)
		s.publishLogChanged(ctx, m.CampaignID, d.enc.ID, res.event == nil || !res.event.Secret)
		if res.event != nil && res.event.Rider != nil {
			s.publishPushed(ctx, m.CampaignID, d, []*forcedMove{res.event.Rider.Pushed})
		}
	})
	if err != nil {
		return nil, err
	}
	return connect.NewResponse(&playv1.UseHitRiderResponse{Encounter: out}), nil
}

// answerRider answers an offer: the choice is checked, applied (a decline spends nothing) and the
// offer closed.
// A choice that rolls the save of a creature with Legendary Resistance and a use left waits for the master, as a cast
// does (combat_legendary_hold.go): the answer is run under a savepoint, the roll is kept, the writes are rolled back and
// the request is held in a window of the master's alone. The offer stays open until the replay closes it.
func (s *Service) answerRider(ctx context.Context, c *combatTx, m authz.Membership, msg *playv1.UseHitRiderRequest, riderID string) (actionEvent, error) {
	cs, err := listCombatants(ctx, c)
	if err != nil {
		return actionEvent{}, err
	}
	row, attacker, target, err := s.riderOffer(ctx, c, m, cs, riderID)
	if err != nil {
		return actionEvent{}, err
	}
	if err := s.riderHeld(ctx, c); err != nil {
		return actionEvent{}, err
	}
	choice, err := riderChoiceOf(row.Kind, msg.GetChoice())
	if err != nil {
		return actionEvent{}, err
	}
	guard, err := s.guardRider(ctx, c, row, target, choice)
	if err != nil {
		return actionEvent{}, err
	}
	ev := actionEvent{Round: c.enc.Round, Secret: secretOf(attacker, target), Actor: attacker.ID, Target: target.ID, Key: row.Kind}
	re := &riderEvent{Choice: choice, Result: riderDecl}
	if err := s.applyAnswer(ctx, c, m, cs, row, attacker, target, choice, re); err != nil {
		guard.abort(ctx, c)
		return actionEvent{}, err
	}
	if held, ok, err := s.holdRider(ctx, c, m, guard, msg, row, attacker, target, re, ev); err != nil || ok {
		if err != nil {
			guard.abort(ctx, c)
		}
		return held, err
	}
	ev.Rider = re
	ev, err = s.closeRider(ctx, c, row, attacker, choice, ev)
	if err != nil {
		guard.abort(ctx, c)
		return actionEvent{}, err
	}
	if err := guard.release(ctx); err != nil {
		return actionEvent{}, err
	}
	return ev, nil
}

// applyAnswer applies the choice of an offer: a creature's rider, a monk's rider, or nothing for a decline.
func (s *Service) applyAnswer(ctx context.Context, c *combatTx, m authz.Membership, cs []playdb.Combatant, row playdb.HitRider, attacker, target playdb.Combatant, choice string, re *riderEvent) error {
	if row.Kind == riderCreature {
		return s.answerCreatureRider(ctx, c, m, row, attacker, target, choice, re)
	}
	if choice == choiceDecline {
		return nil
	}
	return s.applyChoice(ctx, c, m, cs, row.Kind, attacker, target, choice, re)
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
	if row.Kind == riderCreature && !v.master { // a creature's rider is the master's: a player is told nothing of an NPC's attack (RN-10)
		return row, playdb.Combatant{}, playdb.Combatant{}, connect.NewError(connect.CodePermissionDenied, errors.New("only the monk's player or the master may answer"))
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
	if err := touch(ctx, c); err != nil {
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
	_, err = s.spendResourceOf(ctx, c, attacker, resKi)
	return err
}

// riderAgainst is what the saving throw of a rider's choice is against: the condition the choice puts on a
// target that fails it.
func riderAgainst(choice string) []string {
	switch choice {
	case choiceProne:
		return []string{condProne}
	case choiceStun:
		return []string{conditionStunned}
	}
	return nil
}

// rollRiderOutcome sets what the rider's result says and tells whether the choice lands: a choice
// with no save lands, one with a save lands only when the target fails it.
func (s *Service) rollRiderOutcome(ctx context.Context, c *combatTx, m authz.Membership, target playdb.Combatant, choice string, dc int32, re *riderEvent) (bool, error) {
	ability := riderSaveAbility(choice)
	if ability == "" {
		re.Result = riderApplied
		return true, nil
	}
	saved, err := s.rollRiderSave(ctx, c, m, target, ability, riderAgainst(choice), dc, re)
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
		if immune, err := c.immuneToCondition(ctx, target, condProne); err != nil || immune {
			return err
		}
		if !slices.Contains(target.Conditions, condProne) && len(target.Conditions) < maxConditions {
			return setConditionsOf(ctx, c, target, withCondition(target.Conditions, condProne))
		}
	case choicePush:
		return s.pushByRider(ctx, c, cs, attacker, target, re)
	case choiceNoReactions:
		return s.riderEffect(ctx, c, cs, attacker, target, effectNoReactions)
	case choiceStun:
		states, err := s.readStates(ctx, c.tx, c.enc.ID)
		if err != nil {
			return err
		}
		sheetImmune, err := c.immuneToCondition(ctx, target, conditionStunned)
		if err != nil {
			return err
		}
		if sheetImmune || combat.ImmuneTo(effectModsOf(states, target.ID), conditionStunned) {
			re.Result = riderSaved
			return nil
		}
		return s.riderEffect(ctx, c, cs, attacker, target, effectStunningStrike)
	}
	return nil
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
// The replay of a rider held for the master's Legendary Resistance lands the roll the hold kept, never a new one, and
// turns the failed save into a success when the master chose to (combat_legendary_rider.go).
func (s *Service) rollRiderSave(ctx context.Context, c *combatTx, m authz.Membership, target playdb.Combatant, ability string, against []string, dc int32, re *riderEvent) (bool, error) {
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
	sun, err := s.sunlightOf(ctx, c.q, c.enc)
	if err != nil {
		return false, err
	}
	creature := sun.facts(creatureFacts(target, states, sheet.Traits), target)
	if combat.AutoFailsSave(creature, ability) {
		re.save = &saveRoll{DC: dc, Auto: true}
		return s.resistRiderSave(ctx, c, target, re, false)
	}
	mode := combat.Resolve(combat.SaveMode(combat.SaveScene{Creature: creature, Ability: ability, EffectVisible: true, Against: against}))
	var d20 d20Roll
	if kept := c.replay.keptSave(target.ID); kept != nil {
		dice, ok := keptDice(kept, save.Bonus, mode, nil)
		if !ok {
			return false, errKeptDiceMismatch(target.ID)
		}
		d20 = dice.d20
	} else if d20, err = s.d20With(rollInput{inApp: true}, save.Bonus, mode); err != nil {
		return false, err
	}
	d20 = floorD20(d20, save.Bonus, floorFor(sheet.Traits, rules.RollAppliesSave, false))
	re.D20, re.Total, re.countedRoll = clamp32(d20.Face(), 1, 20), conv.Int32(d20.Total), countedOf(d20)
	saved := combat.SaveSucceeded(d20.Total, int(dc))
	re.save = &saveRoll{
		D20: re.D20, Bonus: conv.Int32(save.Bonus), Total: re.Total, DC: dc, Saved: saved,
		countedRoll: countedOf(d20), D20B: clamp32(d20.otherFace(), 0, 20), Counted: clamp32(d20.Index, 0, 1), RollMode: modeKey(mode),
	}
	return s.resistRiderSave(ctx, c, target, re, saved)
}

// riderLogProto is the line of an answered rider: the target's roll for the master and, when
// the target is a player's character, its player (RN-20: the monk's player reads how the save
// ended, never an NPC's roll); the DC for the master, and for that player when the table shows
// the DC of a save (showDC).
func riderLogProto(r *riderEvent, numbers, showDC bool) *playv1.CombatLogHitRider {
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
	case choiceApply:
		out.Choice = playv1.HitRiderChoice_HIT_RIDER_CHOICE_APPLY
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
	out.PushedFt, out.PushNoMap, out.PushStopped = pushedFt(r.Pushed), pushedNoMap(r.Pushed), pushedStopped(r.Pushed)
	if r.ActionKey != "" {
		out.ActionKey, out.ActionNamePt, out.DurationText, out.RepeatSave = r.ActionKey, r.ActionName, r.Duration, r.RepeatSave
		if r.Given {
			out.ConditionKey, out.ConditionNamePt = r.ConditionKey, r.ConditionName
		}
		if showDC { // the stat block's numbers are the master's alone
			out.EscapeDc = r.EscapeDC
		}
	}
	if numbers && r.D20 > 0 {
		riderNumbersProto(out, r, showDC)
	}
	return out
}

// riderNumbersProto sets the roll of the save on the line: the d20, the total, what the die counted as and the DC.
func riderNumbersProto(out *playv1.CombatLogHitRider, r *riderEvent, showDC bool) {
	out.Numbers, out.D20, out.SaveTotal = true, r.D20, r.Total
	if r.CountedAs != 0 {
		out.TreatedAs, out.TreatedAsSource = new(r.CountedAs), legacySource(r.CountedBy)
	}
	if showDC { // the DC is the master's, and the target's player's when the table shows it
		out.Dc = r.DC
	}
}

// creatureRiderForPlayer takes from a creature rider's line what a player may not read: the stat block's key,
// and the condition and its duration unless it was given to a target the player owns or it has an outward sign.
func creatureRiderForPlayer(out *playv1.CombatLogHitRider, owns bool, r *riderEvent) {
	out.ActionKey = ""
	if !r.Given || (!owns && len(withoutOwnerOnly([]string{r.ConditionKey})) == 0) {
		out.ConditionKey, out.ConditionNamePt, out.DurationText, out.RepeatSave = "", "", "", false
	}
}

// goadTarget is what a goading maneuver does once its die is counted in the damage: the target
// makes a Wisdom save against the maneuver's DC and, on a failure, has disadvantage against
// everyone but the attacker until the end of the attacker's next turn (effect:goaded).
func (s *Service) goadTarget(ctx context.Context, c *combatTx, m authz.Membership, cs []playdb.Combatant, attacker, target playdb.Combatant, pr *partsRoll) error {
	var goads []string
	for i, r := range pr.rolls {
		if r.Counted && pr.parts[i].Kind == partExtra && pr.parts[i].Resource != "" {
			goads = append(goads, r.Key)
		}
	}
	if len(goads) == 0 {
		return nil
	}
	sheet, err := s.sheetOf(ctx, c.tx, m.CampaignID, attacker)
	if err != nil {
		return err
	}
	for _, mv := range sheet.Traits.Maneuvers {
		if mv.Applies != rules.ManeuverGoad || !slices.Contains(goads, mv.Key) {
			continue
		}
		var re riderEvent
		saved, err := s.rollRiderSave(ctx, c, m, target, goadSaveAbility, nil, conv.Int32(mv.SaveDC), &re)
		if err != nil || saved {
			return err
		}
		return s.riderEffect(ctx, c, cs, attacker, target, rules.EffectGoaded)
	}
	return nil
}

const goadSaveAbility = "wis"
