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
	"github.com/PuraFome/meuRPG/backend/internal/play/playdb"
	"github.com/PuraFome/meuRPG/backend/internal/rules/combat"
)

// The fallen (RN-03, MR-014, Etapa 6, slice 6.4b). A player's character at 0 hit
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
func deathOutcomeOf(face int) string {
	switch {
	case face == 20:
		return deathRevived
	case face == 1:
		return deathCritical
	case face >= 10:
		return deathSuccess
	}
	return deathFailure
}

// RollDeathSave implements playv1connect.CombatServiceHandler.
func (s *Service) RollDeathSave(
	ctx context.Context,
	req *connect.Request[playv1.RollDeathSaveRequest],
) (*connect.Response[playv1.RollDeathSaveResponse], error) {
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
	var in rollInput
	switch roll := req.Msg.GetRoll().(type) {
	case *playv1.RollDeathSaveRequest_RollInApp:
		if !roll.RollInApp {
			return nil, connect.NewError(connect.CodeInvalidArgument, errors.New("roll_in_app must be true"))
		}
		in.inApp = true
	case *playv1.RollDeathSaveRequest_D20Face:
		in.typed = int(roll.D20Face)
		if in.typed < 1 || in.typed > 20 {
			return nil, connect.NewError(connect.CodeInvalidArgument, errors.New("d20_face must be 1 to 20"))
		}
	default:
		return nil, connect.NewError(connect.CodeInvalidArgument, errors.New("set roll_in_app or d20_face"))
	}
	v := viewerOf(m)

	var made actionEvent
	var vitals *playv1.CharacterVitals // after a natural 20
	res, err := s.write(ctx, combatWrite{m: m, key: key, hash: idem.Hash(req.Msg), kind: eventDeathSaveRolled, encounterID: encID}, func(c *combatTx) (any, error) {
		vitals = nil
		cs, err := c.q.ListCombatants(ctx, c.enc.ID)
		if err != nil {
			return nil, fmt.Errorf("list the combatants: %w", err)
		}
		v = c.viewer(m, cs) // the fog: an NPC the player does not see is not found
		who, err := findCombatant(cs, combID, v)
		if err != nil {
			return nil, err
		}
		if err := v.mayAct(who); err != nil {
			return nil, err
		}
		if who.Kind != kindPlayer {
			return nil, connect.NewError(connect.CodeInvalidArgument, errors.New("only a player's character rolls death saves"))
		}
		if err := notEnded(c.enc); err != nil {
			return nil, err
		}
		switch {
		case c.enc.Status != statusActive:
			return nil, errEncounter(playv1.EncounterBlockedReason_ENCOUNTER_BLOCKED_REASON_NOT_ACTIVE, "the combat is not running")
		case !actsNow(c.enc, who):
			return nil, errEncounter(playv1.EncounterBlockedReason_ENCOUNTER_BLOCKED_REASON_NOT_YOUR_TURN, "it is not this combatant's turn")
		}
		now, err := s.vitals.GetVitalsTx(ctx, c.tx, c.session.CampaignID, who.CharacterID)
		if err != nil {
			return nil, err
		}
		if !deathSaveDue(who, now) {
			return nil, errEncounter(playv1.EncounterBlockedReason_ENCOUNTER_BLOCKED_REASON_DEATH_SAVE_NOT_DUE, "there is no death save to roll now")
		}
		if !v.master {
			if err := s.mustRollThisWay(ctx, c.tx, m, in); err != nil {
				return nil, err
			}
		}
		face, roll, err := s.d20(in, 0)
		if err != nil {
			return nil, err
		}
		r := combat.DeathSave(face, int(who.DeathSuccesses), int(who.DeathFailures))
		made = actionEvent{
			Round: c.enc.Round, Actor: who.ID, D20: clamp32(face, 1, 20), Physical: roll.Physical, DeathOutcome: deathOutcomeOf(face),
			DeathBefore: deathOf(who), DeathHidden: c.rules.DeathSavesHidden,
		}
		after := deathState{Successes: clamp32(r.Successes, 0, 3), Failures: clamp32(r.Failures, 0, 3), Rolled: true}
		if r.Outcome == combat.DeathSaveRevived {
			// A natural 20: 1 hit point, through the vitals, and the counts reset.
			hp := clamp32(r.HP, 0, math.MaxInt32)
			before, healed, err := s.vitalsOf(ctx, c, who.CharacterID, &playv1.AdjustCharacterVitalsRequest{HitPointsCurrent: &hp})
			if err != nil {
				return nil, err
			}
			vitals = healed
			made.Before, made.After = new(hpStateOf(before)), new(hpStateOf(healed))
			after = deathState{Rolled: true}
		}
		made.Death = &after
		if err := c.q.SetCombatantDeathSaves(ctx, playdb.SetCombatantDeathSavesParams{
			ID: who.ID, DeathSuccesses: after.Successes, DeathFailures: after.Failures, DeathSaveRolled: true, Defeated: who.Defeated,
		}); err != nil {
			return nil, fmt.Errorf("save the death save: %w", err)
		}
		if c.enc, err = c.q.TouchEncounter(ctx, c.enc.ID); err != nil {
			return nil, fmt.Errorf("touch the encounter: %w", err)
		}
		c.characterID = &who.CharacterID
		return made, nil
	})
	if err != nil {
		return nil, s.dbError(ctx, "roll a death save", err)
	}
	ev, err := resultEvent(res, made)
	if err != nil {
		return nil, s.dbError(ctx, "read the death save", err)
	}
	out, err := s.finish(ctx, m, res, func(ctx context.Context, d *encounterData) {
		s.publishEncounterChanged(ctx, m.CampaignID, d.enc)
		s.publishLogChanged(ctx, m.CampaignID, d.enc.ID, true) // a player is never hidden
		s.publishVitals(m.CampaignID, vitals)
	})
	if err != nil {
		return nil, err
	}
	return connect.NewResponse(&playv1.RollDeathSaveResponse{Encounter: out, DeathSave: deathSaveProto(ev, v)}), nil
}

// deathSaveProto builds the DeathSave an event tells. "Dying" is the master's:
// a player only sees the counts.
func deathSaveProto(ev actionEvent, v combatViewer) *playv1.DeathSave {
	after := ev.Death
	if after == nil {
		after = &deathState{}
	}
	return &playv1.DeathSave{
		Roll:      diceRoll(1, 20, []int32{ev.D20}, 0, ev.D20, ev.Physical),
		Outcome:   deathOutcomeToProto[ev.DeathOutcome],
		Successes: after.Successes, Failures: after.Failures,
		Stable: after.Successes >= 3, Dying: v.master && after.Failures >= 3,
	}
}

// ConfirmDeath implements playv1connect.CombatServiceHandler.
func (s *Service) ConfirmDeath(
	ctx context.Context,
	req *connect.Request[playv1.ConfirmDeathRequest],
) (*connect.Response[playv1.ConfirmDeathResponse], error) {
	m, err := authz.RequireCampaignRole(ctx, req.Msg.GetCampaignId(), authz.RoleMaster)
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

	var turnPassed bool
	res, err := s.write(ctx, combatWrite{m: m, key: key, hash: idem.Hash(req.Msg), kind: eventDeathConfirmed, encounterID: encID}, func(c *combatTx) (any, error) {
		turnPassed = false
		if err := notEnded(c.enc); err != nil {
			return nil, err
		}
		cs, err := c.q.ListCombatants(ctx, c.enc.ID)
		if err != nil {
			return nil, fmt.Errorf("list the combatants: %w", err)
		}
		who, err := findCombatant(cs, combID, viewerOf(m))
		if err != nil {
			return nil, err
		}
		if who.Kind != kindPlayer {
			return nil, connect.NewError(connect.CodeInvalidArgument, errors.New("only a player's character dies: an NPC is defeated"))
		}
		if who.Defeated || who.DeathFailures < 3 {
			return nil, errEncounter(playv1.EncounterBlockedReason_ENCOUNTER_BLOCKED_REASON_NOT_DYING, "the character has not failed three death saves, or is dead already")
		}
		// The character is dead in the characters module, as MarkCharacterDead
		// leaves it: its player may create another (RN-03).
		if err := s.roster.MarkDead(ctx, c.tx, m.CampaignID, who.CharacterID, c.now); err != nil {
			return nil, err
		}
		if err := c.q.SetCombatantDeathSaves(ctx, playdb.SetCombatantDeathSavesParams{
			ID: who.ID, DeathSuccesses: who.DeathSuccesses, DeathFailures: who.DeathFailures, DeathSaveRolled: who.DeathSaveRolled, Defeated: true,
		}); err != nil {
			return nil, fmt.Errorf("take the character out of the order: %w", err)
		}
		// Out of the order: it leaves the turn first, and the turn passes when
		// nobody who acts is left in its group.
		if c.enc.Status == statusActive {
			if _, err := leaveTurn(ctx, c, cs, who); err != nil {
				return nil, err
			}
			turnPassed = inTurn(c.enc, who)
		} else if c.enc, err = c.q.TouchEncounter(ctx, c.enc.ID); err != nil {
			return nil, fmt.Errorf("touch the encounter: %w", err)
		}
		c.characterID = &who.CharacterID
		return actionEvent{Round: c.enc.Round, Actor: who.ID, DeathBefore: deathOf(who), Death: &deathState{Successes: who.DeathSuccesses, Failures: who.DeathFailures, Dead: true}}, nil
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

// failuresWhileDown adds the death save failures a damage causes to a character
// at 0 hit points (RN-03): one, two for a critical hit. A stable character that
// takes damage starts over (the SRD: it must make death saves again), so its
// successes reset first. It returns the state before and after, and the
// failures added; nothing changes for a damage of 0.
func (s *Service) failuresWhileDown(ctx context.Context, c *combatTx, target playdb.Combatant, critical bool, amount int32) (before, after *deathState, added int32, err error) {
	before = deathOf(target)
	if amount <= 0 {
		return before, before, 0, nil
	}
	successes := int(target.DeathSuccesses)
	if successes >= 3 {
		successes = 0
	}
	n := combat.DamageWhileDown(critical)
	r := combat.AddFailures(successes, int(target.DeathFailures), n)
	after = &deathState{Successes: clamp32(r.Successes, 0, 3), Failures: clamp32(r.Failures, 0, 3), Rolled: target.DeathSaveRolled, Dead: target.Defeated}
	if err := c.q.SetCombatantDeathSaves(ctx, playdb.SetCombatantDeathSavesParams{
		ID: target.ID, DeathSuccesses: after.Successes, DeathFailures: after.Failures, DeathSaveRolled: after.Rolled, Defeated: after.Dead,
	}); err != nil {
		return nil, nil, 0, fmt.Errorf("add the death save failures: %w", err)
	}
	return before, after, clamp32(n, 0, math.MaxInt32), nil
}
