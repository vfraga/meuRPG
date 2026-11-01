package play

import (
	"context"
	"errors"
	"fmt"
	"math"

	"connectrpc.com/connect"

	playv1 "github.com/PuraFome/meuRPG/backend/gen/meurpg/play/v1"
	"github.com/PuraFome/meuRPG/backend/internal/authz"
	"github.com/PuraFome/meuRPG/backend/internal/platform/idem"
	"github.com/PuraFome/meuRPG/backend/internal/play/link"
	"github.com/PuraFome/meuRPG/backend/internal/play/playdb"
	"github.com/PuraFome/meuRPG/backend/internal/rules/combat"
)

// The fallen (RN-03, MR-014). A player's character at 0 hit
// points is "Caído": its turn starts with a death save due, and the turn waits
// for it. 10 or more is a success, less a failure, a natural 1 two failures, a
// natural 20 brings it back with 1 hit point. Three successes make it stable;
// three failures make it dying, and the master confirms the death: the engine
// never kills a character by itself. The counts live on the combatant (they stay
// when the combat ends, for the summary) and any healing above 0 resets them
// (changeVitals).

// The death save outcomes as an event keeps them.
const (
	deathSuccess  = "success"
	deathFailure  = "failure"
	deathCritical = "critical_failure"
	deathRevived  = "revived"
)

var deathOutcomeToProto = map[string]playv1.DeathSaveOutcome{
	deathSuccess:  playv1.DeathSaveOutcome_DEATH_SAVE_OUTCOME_SUCCESS,
	deathFailure:  playv1.DeathSaveOutcome_DEATH_SAVE_OUTCOME_FAILURE,
	deathCritical: playv1.DeathSaveOutcome_DEATH_SAVE_OUTCOME_CRITICAL_FAILURE,
	deathRevived:  playv1.DeathSaveOutcome_DEATH_SAVE_OUTCOME_REVIVED,
}

// deathOutcomeOf names what a d20 does to a death save.
func deathOutcomeOf(face, bonus int) string {
	switch {
	case face == 20:
		return deathRevived
	case face == 1:
		return deathCritical
	case face+bonus >= combat.DeathSaveDC:
		return deathSuccess
	}
	return deathFailure
}

// deathSaveCall is what RollDeathSave reads from its request, checked.
type deathSaveCall struct {
	m           authz.Membership
	key         string
	encounterID string
	combatantID string
	in          rollInput
	msg         *playv1.RollDeathSaveRequest
}

// deathSaveOutcome is what the write of a death save leaves for the answer and the
// publishers after the commit.
type deathSaveOutcome struct {
	made   actionEvent
	vitals *playv1.CharacterVitals // after a natural 20
	viewer combatViewer
	held   *playv1.HeldRollOffer // the d20 was kept for a question: nothing is written
	// hookName is the name of the feature whose hook ran on this save.
	hookName string
}

// RollDeathSave implements playv1connect.CombatServiceHandler.
func (s *Service) RollDeathSave(
	ctx context.Context,
	req *connect.Request[playv1.RollDeathSaveRequest],
) (*connect.Response[playv1.RollDeathSaveResponse], error) {
	call, err := readDeathSaveRequest(ctx, req.Msg)
	if err != nil {
		return nil, err
	}
	out := deathSaveOutcome{viewer: viewerOf(call.m)}
	res, err := s.write(ctx, combatWrite{m: call.m, key: call.key, hash: idem.Hash(req.Msg), kind: eventDeathSaveRolled, encounterID: call.encounterID}, func(c *combatTx) (any, error) {
		out.vitals, out.held, out.hookName = nil, nil, ""
		return s.deathSaveWrite(ctx, c, call, &out)
	})
	if err != nil {
		return nil, s.dbError(ctx, "roll a death save", err)
	}
	if out.held != nil { // nothing was written: the question is the whole answer
		enc, err := s.finish(ctx, call.m, res, s.changed(call.m.CampaignID))
		if err != nil {
			return nil, err
		}
		return connect.NewResponse(&playv1.RollDeathSaveResponse{Encounter: enc, HeldRollOffer: out.held}), nil
	}
	ev, err := resultEvent(res, out.made)
	if err != nil {
		return nil, s.dbError(ctx, "read the death save", err)
	}
	enc, err := s.finish(ctx, call.m, res, func(ctx context.Context, d *encounterData) {
		s.publishEncounterChanged(ctx, call.m.CampaignID, d.enc)
		s.publishLogChanged(ctx, call.m.CampaignID, d.enc.ID, true) // a player is never hidden
		s.publishVitals(call.m.CampaignID, out.vitals)
	})
	if err != nil {
		return nil, err
	}
	return connect.NewResponse(&playv1.RollDeathSaveResponse{Encounter: enc, DeathSave: deathSaveProto(ev, viewerOf(heldViewer(ctx, call.m)), out.hookName)}), nil
}

// readDeathSaveRequest reads the request in the order the answers are given: who may call,
// the key, the encounter, the combatant, then the roll.
func readDeathSaveRequest(ctx context.Context, req *playv1.RollDeathSaveRequest) (deathSaveCall, error) {
	m, err := heldMember(ctx, req.GetCampaignId()) // a held roll's answer runs this again as the roller
	if err != nil {
		return deathSaveCall{}, err
	}
	key, encID, err := parseCall(req.GetIdempotencyKey(), req.GetEncounterId())
	if err != nil {
		return deathSaveCall{}, err
	}
	call := deathSaveCall{m: m, key: key, encounterID: encID, msg: req}
	if call.combatantID, err = parseCombatID(req.GetCombatantId(), "combatant"); err != nil {
		return deathSaveCall{}, err
	}
	switch roll := req.GetRoll().(type) {
	case *playv1.RollDeathSaveRequest_RollInApp:
		if !roll.RollInApp {
			return deathSaveCall{}, connect.NewError(connect.CodeInvalidArgument, errors.New("roll_in_app must be true"))
		}
		call.in.inApp = true
	case *playv1.RollDeathSaveRequest_D20Face:
		call.in.typed = int(roll.D20Face)
		if call.in.typed < 1 || call.in.typed > 20 {
			return deathSaveCall{}, connect.NewError(connect.CodeInvalidArgument, errors.New("d20_face must be 1 to 20"))
		}
	default:
		return deathSaveCall{}, connect.NewError(connect.CodeInvalidArgument, errors.New("set roll_in_app or d20_face"))
	}
	return call, nil
}

// deathSaveWrite is the transaction of RollDeathSave: who rolls, the roll, the counts and, on
// a natural 20, the hit point the character comes back with.
func (s *Service) deathSaveWrite(ctx context.Context, c *combatTx, call deathSaveCall, out *deathSaveOutcome) (any, error) {
	cs, err := listCombatants(ctx, c)
	if err != nil {
		return nil, err
	}
	out.viewer = c.viewer(call.m, cs) // the fog: an NPC the player does not see is not found
	who, err := findCombatant(cs, call.combatantID, out.viewer)
	if err != nil {
		return nil, err
	}
	if err := s.deathSaveGates(ctx, c, call, out.viewer, who); err != nil {
		return nil, err
	}
	sheet, err := s.sheetOf(ctx, c.tx, c.session.CampaignID, who)
	if err != nil {
		return nil, err
	}
	// A character that holds a Bardic Inspiration die, or a halfling whose d20 shows a 1, has the d20 kept until the
	// player answers; the answer brings it back with the die added (SRD 5.1).
	// Indomitable (held_indomitable.go) rerolls a death save that fails by the roll. The DC is the fixed 10, and the
	// counts are the owner's even at a table that hides them from the others, so the question may say it failed.
	gate, err := s.heldGate(ctx, s.inspirationGate(c), rollSpec{
		Kind: heldDeathSave, Key: call.key, Request: call.msg, Roller: attackRoller(call.m, who, c.enc.Round), In: call.in,
		Mode: combat.ModeNormal, Lucky: sheet.Traits.Lucky, MayAsk: playedBy(who, call.m),
		Save: &saveRule{Death: true},
	})
	if err != nil {
		return nil, err
	}
	if gate.Held != nil {
		_, _, out.held = s.outsideOffers(ctx, c.tx, gate.Held)
		return nil, nil
	}
	roll := gate.Roll
	face, bonus := roll.Face(), roll.Total-roll.Face()
	r := combat.DeathSaveWith(face, bonus, int(who.DeathSuccesses), int(who.DeathFailures))
	out.made = actionEvent{
		Round: c.enc.Round, Actor: who.ID, D20: clamp32(face, 1, 20), Modifier: clamp32(bonus, 0, math.MaxInt32), Total: clamp32(roll.Total, 0, math.MaxInt32),
		Physical: roll.Physical, DeathOutcome: deathOutcomeOf(face, bonus), DeathBefore: deathOf(who), DeathHidden: c.rules.DeathSavesHidden,
		Lucky: gate.Lucky, Indomitable: gate.Indomitable,
	}
	if b := gate.Bonus; b != nil { // the die the player added: the log shows it and the undo gives it back
		out.made.Res = &resourceEvent{Kind: resBardicUse, Sides: b.Sides, Face: b.Face, FromID: deref(who.InspirationFrom), ExpiresRound: derefInt32(who.InspirationExpiresRound)}
		if u := gate.Used; u != nil { // the die was spent by an earlier answer: the row kept who gave it
			out.made.Res.FromID, out.made.Res.ExpiresRound = u.From, u.ExpiresRound
		}
	}
	after, err := s.deathCounts(ctx, c, who, sheet, r, out)
	if err != nil {
		return nil, err
	}
	out.made.Death = &after
	if err := c.q.SetCombatantDeathSaves(ctx, playdb.SetCombatantDeathSavesParams{
		ID: who.ID, DeathSuccesses: after.Successes, DeathFailures: after.Failures, DeathSaveRolled: true, Defeated: who.Defeated,
	}); err != nil {
		return nil, fmt.Errorf("save the death save: %w", err)
	}
	if err := touch(ctx, c); err != nil {
		return nil, err
	}
	c.characterID = &who.CharacterID
	return out.made, nil
}

// deathCounts works out the counts after the save: the roll's, or a natural 20's return with 1 hit point, or the start
// over of a failed save a hook of the character answered by standing it up. The hook runs before the failure is added, so
// the one that stands the character up keeps the third failure from killing it.
func (s *Service) deathCounts(ctx context.Context, c *combatTx, who playdb.Combatant, sheet link.Sheet, r combat.DeathSaveResult, out *deathSaveOutcome) (deathState, error) {
	after := deathState{Successes: clamp32(r.Successes, 0, 3), Failures: clamp32(r.Failures, 0, 3), Rolled: true}
	switch {
	case r.Outcome == combat.DeathSaveRevived:
		return s.reviveOnNaturalTwenty(ctx, c, who, r, out)
	case failedDeathSave(out.made.DeathOutcome):
		stood, err := s.deathSaveHook(ctx, c, who, sheet, out)
		if err != nil {
			return after, err
		}
		if stood {
			after = deathState{Rolled: true}
		}
	}
	return after, nil
}

// deathSaveGates checks that the combatant may roll a death save now: its player or the
// master, a player's character, in a combat that runs, on its turn, with a save due, and in
// the way the table rolls.
func (s *Service) deathSaveGates(ctx context.Context, c *combatTx, call deathSaveCall, v combatViewer, who playdb.Combatant) error {
	if err := v.mayAct(who); err != nil {
		return err
	}
	if who.Kind != kindPlayer {
		return connect.NewError(connect.CodeInvalidArgument, errors.New("only a player's character rolls death saves"))
	}
	if err := notEnded(c.enc); err != nil {
		return err
	}
	switch {
	case c.enc.Status != statusActive:
		return errEncounter(playv1.EncounterBlockedReason_ENCOUNTER_BLOCKED_REASON_NOT_ACTIVE, "the combat is not running")
	case !actsNow(c.enc, who):
		return errEncounter(playv1.EncounterBlockedReason_ENCOUNTER_BLOCKED_REASON_NOT_YOUR_TURN, "it is not this combatant's turn")
	}
	now, err := s.vitals.GetVitalsTx(ctx, c.tx, c.session.CampaignID, who.CharacterID)
	if err != nil {
		return err
	}
	if !deathSaveDue(who, now) {
		return errEncounter(playv1.EncounterBlockedReason_ENCOUNTER_BLOCKED_REASON_DEATH_SAVE_NOT_DUE, "there is no death save to roll now")
	}
	if !v.master {
		return s.mustRollThisWay(ctx, c.tx, call.m, call.in)
	}
	return nil
}

// reviveOnNaturalTwenty brings the character back with the hit point of a natural 20, through
// the vitals, and resets the counts. It returns the counts after.
func (s *Service) reviveOnNaturalTwenty(ctx context.Context, c *combatTx, who playdb.Combatant, r combat.DeathSaveResult, out *deathSaveOutcome) (deathState, error) {
	hp := clamp32(r.HP, 0, math.MaxInt32)
	before, healed, err := s.vitalsOf(ctx, c, who.CharacterID, &playv1.AdjustCharacterVitalsRequest{HitPointsCurrent: &hp})
	if err != nil {
		return deathState{}, err
	}
	out.vitals = healed
	out.made.Before, out.made.After = new(hpStateOf(before)), new(hpStateOf(healed))
	return deathState{Rolled: true}, nil
}

// deathRoll is the d20 of a death save as an event tells it: with the Bardic Inspiration die as the modifier and the
// d20 the halfling's Lucky rolled again.
func deathRoll(ev actionEvent) *playv1.DiceRoll {
	total := ev.Total
	if total == 0 {
		total = ev.D20
	}
	roll := diceRoll(1, 20, []int32{ev.D20}, ev.Modifier, total, ev.Physical)
	roll.Rerolled = ev.rerolls()
	return roll
}

// deathSaveProto builds the DeathSave an event tells. "Dying" is the master's:
// a player only sees the counts.
func deathSaveProto(ev actionEvent, v combatViewer, hookName string) *playv1.DeathSave {
	after := ev.Death
	if after == nil {
		after = &deathState{}
	}
	return &playv1.DeathSave{
		Roll:      deathRoll(ev),
		BonusDice: bonusDiceOf(ev),
		Outcome:   deathOutcomeToProto[ev.DeathOutcome],
		Successes: after.Successes, Failures: after.Failures,
		Stable: after.Successes >= 3, Dying: v.master && after.Failures >= 3,
		Hook: hookProto(ev.Hook, hookName),
	}
}

// ConfirmDeath implements playv1connect.CombatServiceHandler.
func (s *Service) ConfirmDeath(
	ctx context.Context,
	req *connect.Request[playv1.ConfirmDeathRequest],
) (*connect.Response[playv1.ConfirmDeathResponse], error) {
	m, key, encID, err := masterCall(ctx, req.Msg.GetCampaignId(), req.Msg.GetIdempotencyKey(), req.Msg.GetEncounterId())
	if err != nil {
		return nil, err
	}
	combID, err := parseCombatID(req.Msg.GetCombatantId(), "combatant")
	if err != nil {
		return nil, err
	}

	var turnPassed bool
	res, err := s.write(ctx, combatWrite{m: m, key: key, hash: idem.Hash(req.Msg), kind: eventDeathConfirmed, encounterID: encID}, func(c *combatTx) (any, error) {
		turnPassed = false
		return s.confirmDeathWrite(ctx, c, m, combID, &turnPassed)
	})
	if err != nil {
		return nil, s.dbError(ctx, "confirm a death", err)
	}
	out, err := s.finish(ctx, m, res, func(ctx context.Context, d *encounterData) {
		s.publishEncounterChanged(ctx, m.CampaignID, d.enc)
		s.publishLogChanged(ctx, m.CampaignID, d.enc.ID, true)
		if turnPassed {
			s.publishTurnChanged(ctx, m.CampaignID, d)
		}
	})
	if err != nil {
		return nil, err
	}
	return connect.NewResponse(&playv1.ConfirmDeathResponse{Encounter: out}), nil
}

// confirmDeathWrite is the transaction of ConfirmDeath: the character must have failed three
// death saves, its effects end, it dies in the characters module and leaves the order.
func (s *Service) confirmDeathWrite(ctx context.Context, c *combatTx, m authz.Membership, combID string, turnPassed *bool) (any, error) {
	if err := notEnded(c.enc); err != nil {
		return nil, err
	}
	cs, err := listCombatants(ctx, c)
	if err != nil {
		return nil, err
	}
	who, err := findCombatant(cs, combID, viewerOf(m))
	if err != nil {
		return nil, err
	}
	if err := mustBeDying(who); err != nil {
		return nil, err
	}
	// Death ends what is on the character and the concentration it held (SRD 5.1, Dropping
	// to 0 Hit Points; Duration): the effects go, and living again brings back none of them.
	if err := s.endEffectsOfTheDead(ctx, c, cs, who); err != nil {
		return nil, err
	}
	// The character is dead in the characters module, as MarkCharacterDead
	// leaves it: its player may create another (RN-03).
	if err := s.roster.MarkDeadInCombat(ctx, c.tx, m.CampaignID, who.CharacterID, c.now, c.enc.Round, c.enc.ID); err != nil {
		return nil, err
	}
	if err := c.q.SetCombatantDeathSaves(ctx, playdb.SetCombatantDeathSavesParams{
		ID: who.ID, DeathSuccesses: who.DeathSuccesses, DeathFailures: who.DeathFailures, DeathSaveRolled: who.DeathSaveRolled, Defeated: true,
	}); err != nil {
		return nil, fmt.Errorf("take the character out of the order: %w", err)
	}
	if err := takeOutOfTheTurn(ctx, c, cs, who, turnPassed); err != nil {
		return nil, err
	}
	c.characterID = &who.CharacterID
	return actionEvent{Round: c.enc.Round, Actor: who.ID, DeathBefore: deathOf(who), Death: &deathState{Successes: who.DeathSuccesses, Failures: who.DeathFailures, Dead: true}}, nil
}

// mustBeDying refuses a death for anything but a player's character that failed three death
// saves and is not dead yet.
func mustBeDying(who playdb.Combatant) error {
	if who.Kind != kindPlayer {
		return connect.NewError(connect.CodeInvalidArgument, errors.New("only a player's character dies: an NPC is defeated"))
	}
	if who.Defeated || who.DeathFailures < 3 {
		return errEncounter(playv1.EncounterBlockedReason_ENCOUNTER_BLOCKED_REASON_NOT_DYING, "the character has not failed three death saves, or is dead already")
	}
	return nil
}

// takeOutOfTheTurn takes the dead out of the order: in a running combat it leaves the turn
// first, and the turn passes when nobody who acts is left in its group; before the combat
// runs the combat is only touched.
func takeOutOfTheTurn(ctx context.Context, c *combatTx, cs []playdb.Combatant, who playdb.Combatant, turnPassed *bool) error {
	if c.enc.Status != statusActive {
		return touch(ctx, c)
	}
	if _, err := leaveTurn(ctx, c, cs, who); err != nil {
		return err
	}
	*turnPassed = inTurn(c.enc, who)
	return nil
}

// failuresWhileDown adds the death save failures a damage causes to a character
// at 0 hit points (RN-03): one, two for a critical hit. A stable character that
// takes damage starts over (the SRD: it must make death saves again), so its
// both counts reset first. It returns the state before and after, and the
// failures added; nothing changes for a damage of 0.
func (s *Service) failuresWhileDown(ctx context.Context, c *combatTx, target playdb.Combatant, critical bool, amount int32) (before, after *deathState, added int32, err error) {
	before = deathOf(target)
	if amount <= 0 {
		return before, before, 0, nil
	}
	successes, failures := int(target.DeathSuccesses), int(target.DeathFailures)
	if successes >= 3 {
		successes, failures = 0, 0 // stable: both counts start over
	}
	n := combat.DamageWhileDown(critical)
	r := combat.AddFailures(successes, failures, n)
	after = &deathState{Successes: clamp32(r.Successes, 0, 3), Failures: clamp32(r.Failures, 0, 3), Rolled: target.DeathSaveRolled, Dead: target.Defeated}
	if err := c.q.SetCombatantDeathSaves(ctx, playdb.SetCombatantDeathSavesParams{
		ID: target.ID, DeathSuccesses: after.Successes, DeathFailures: after.Failures, DeathSaveRolled: after.Rolled, Defeated: after.Dead,
	}); err != nil {
		return nil, nil, 0, fmt.Errorf("add the death save failures: %w", err)
	}
	return before, after, clamp32(n, 0, math.MaxInt32), nil
}
