package play

import (
	"context"
	"fmt"
	"math"
	"slices"

	"connectrpc.com/connect"

	playv1 "github.com/PuraFome/meuRPG/backend/gen/meurpg/play/v1"
	"github.com/PuraFome/meuRPG/backend/internal/authz"
	"github.com/PuraFome/meuRPG/backend/internal/platform/idem"
	"github.com/PuraFome/meuRPG/backend/internal/play/playdb"
)

// Standing up from Prone (SRD 5.1, "Being Prone" and "Movement and Position"): it costs an
// amount of movement equal to half the creature's speed, and a creature cannot stand up
// without that much movement left, for example when its speed is 0. While Prone, every foot
// of movement costs 1 extra foot (crawling): see crawls.

// The log line of a combatant standing up, an event of combatant_moved (so the master's
// undo puts the movement back) that carries it in Contest.Line.
const contestLineStoodUp = "stood_up"

// The log lines this file adds to the contests' table: contestLineProto lives in the
// contests' file, which another change owns, so the lines register themselves here.
func init() {
	contestLineProto[contestLineStoodUp] = playv1.ContestLogLine_CONTEST_LOG_LINE_STOOD_UP
	contestLineProto[contestLineHideRevealed] = playv1.ContestLogLine_CONTEST_LOG_LINE_HIDE_REVEALED
}

// isProne says whether the combatant carries the Prone condition.
func isProne(c playdb.Combatant) bool { return slices.Contains(c.Conditions, condProne) }

// crawls says the combatant moves by crawling: Prone, on foot. A creature that flies and
// is better in the air than on the ground (the speed it moves on) is not crawling.
func crawls(c playdb.Combatant) bool { return isProne(c) && !flies(c) }

// standUpCostDFt is what standing up costs the combatant, in tenths of a foot: half its
// speed, rounded down, and 0 when it is not Prone or has no speed (the call is then
// refused). The Dash does not count: it adds movement, not speed (SRD 5.1, "Dash").
func standUpCostDFt(c playdb.Combatant) int {
	if !isProne(c) {
		return 0
	}
	c.Dashed = false
	return speedDFt(c) / standUpSpeedDivisor
}

// standUpSpeedDivisor: standing up costs half the speed (SRD 5.1, "Being Prone").
const standUpSpeedDivisor = 2

// standUpCall is what StandUp reads from its request, checked.
type standUpCall struct {
	m           authz.Membership
	key         string
	encounterID string
	combatantID string
}

// StandUp implements playv1connect.CombatServiceHandler.
func (s *Service) StandUp(
	ctx context.Context,
	req *connect.Request[playv1.StandUpRequest],
) (*connect.Response[playv1.StandUpResponse], error) {
	m, key, encID, err := contestCaller(ctx, req.Msg.GetCampaignId(), req.Msg.GetIdempotencyKey(), req.Msg.GetEncounterId())
	if err != nil {
		return nil, err
	}
	call := standUpCall{m: m, key: key, encounterID: encID}
	if call.combatantID, err = parseCombatID(req.Msg.GetCombatantId(), "combatant"); err != nil {
		return nil, err
	}

	var hidden bool
	res, err := s.write(ctx, combatWrite{m: m, key: key, hash: idem.Hash(req.Msg), kind: eventCombatantMoved, encounterID: encID}, func(c *combatTx) (any, error) {
		return s.standUpWrite(ctx, c, call, &hidden)
	})
	if err != nil {
		return nil, s.dbError(ctx, "stand up", err)
	}
	out, err := s.finish(ctx, m, res, func(ctx context.Context, d *encounterData) {
		s.publishEncounterChanged(ctx, m.CampaignID, d.enc)
		s.publishLogChanged(ctx, m.CampaignID, d.enc.ID, !hidden)
	})
	if err != nil {
		return nil, err
	}
	return connect.NewResponse(&playv1.StandUpResponse{Encounter: out, MovementLeftDft: standUpMovementLeft(out, call.combatantID)}), nil
}

// standUpWrite is the transaction of StandUp: who may, what it costs, the movement spent and
// the Prone condition taken off.
func (s *Service) standUpWrite(ctx context.Context, c *combatTx, call standUpCall, hidden *bool) (any, error) {
	if err := notEnded(c.enc); err != nil {
		return nil, err
	}
	cs, err := listCombatants(ctx, c)
	if err != nil {
		return nil, err
	}
	v := c.viewer(call.m, cs)
	target, err := findCombatant(cs, call.combatantID, v)
	if err != nil {
		return nil, err
	}
	if err := v.mayAct(target); err != nil {
		return nil, err
	}
	if err := s.standUpGates(ctx, c, call.m, v, target); err != nil {
		return nil, err
	}
	cost, err := standUpCostOf(target)
	if err != nil {
		return nil, err
	}
	if err := spendStandUp(ctx, c, target, cost); err != nil {
		return nil, err
	}
	if err := setConditionsOf(ctx, c, target, withoutCondition(target.Conditions, condProne)); err != nil {
		return nil, err
	}
	if err := touchEncounter(ctx, c); err != nil {
		return nil, err
	}
	c.characterID = &target.CharacterID
	*hidden = target.Hidden
	return standUpEvent(c.enc, target, cost), nil
}

// standUpGates refuses a stand up out of the combatant's turn, or while it is down,
// surprised, or the turn waits for the master or for a held action.
func (s *Service) standUpGates(ctx context.Context, c *combatTx, m authz.Membership, v combatViewer, target playdb.Combatant) error {
	if c.enc.Status != statusActive {
		return errEncounter(playv1.EncounterBlockedReason_ENCOUNTER_BLOCKED_REASON_NOT_ACTIVE, "the combat is not running")
	}
	// Movement is a turn's: the master stands a creature up on its turn too.
	if !actsNow(c.enc, target) {
		return errEncounter(playv1.EncounterBlockedReason_ENCOUNTER_BLOCKED_REASON_NOT_YOUR_TURN, "it is not your turn")
	}
	if !v.master {
		if err := s.mustNotBeDown(ctx, c.tx, m.CampaignID, target); err != nil {
			return err
		}
		if err := s.mustNotBeSurprised(ctx, c, target); err != nil {
			return err
		}
	}
	if err := s.mustNotHold(ctx, c); err != nil {
		return err
	}
	return s.mustNotWait(ctx, c, target)
}

// standUpCostOf says what standing up costs the combatant, in tenths of a foot, or why it
// cannot: it is not Prone, it has no speed, or it has not enough movement left.
func standUpCostOf(target playdb.Combatant) (int, error) {
	if !isProne(target) {
		return 0, errEncounter(playv1.EncounterBlockedReason_ENCOUNTER_BLOCKED_REASON_NOT_PRONE, "the combatant is not prone")
	}
	cost := standUpCostDFt(target)
	base := target
	base.Dashed = false
	if speedDFt(base) == 0 {
		return 0, errEncounter(playv1.EncounterBlockedReason_ENCOUNTER_BLOCKED_REASON_CANNOT_STAND_UP, "the combatant cannot stand up: its speed is 0")
	}
	if left := movementLeftDFt(target); cost > left {
		return 0, tooFar(cost - left)
	}
	return cost, nil
}

// spendStandUp adds the cost of standing up to the movement the combatant used.
func spendStandUp(ctx context.Context, c *combatTx, target playdb.Combatant, cost int) error {
	used := int(target.MovementUsedDft) + cost
	if err := c.q.SetCombatantMove(ctx, playdb.SetCombatantMoveParams{
		ID: target.ID, GridCol: target.GridCol, GridRow: target.GridRow,
		MovementUsedFt: clamp32(used/10, 0, math.MaxInt32), MovementUsedDft: clamp32(used, 0, math.MaxInt32),
		LastMoveDft: target.LastMoveDft, CoverMark: target.CoverMark, // nobody changed square
	}); err != nil {
		return fmt.Errorf("spend the movement: %w", err)
	}
	return nil
}

// standUpEvent is the combatant_moved event of a stand up, as SpendMovement writes it: the
// master's undo puts the movement back from From, and the conditions from CondBefore. Col and
// Row are where it stands, so the fog's reading of an undone move finds no change.
func standUpEvent(enc playdb.Encounter, target playdb.Combatant, cost int) actionEvent {
	made := actionEvent{
		Round: enc.Round, Secret: target.Hidden, Actor: target.ID, OnTurn: true, From: moveStateOf(target),
		CostDFt: clamp32(cost, 0, math.MaxInt32), CostFt: clamp32(cost/10, 0, math.MaxInt32),
		CondSet: true, CondBefore: slices.Clone(target.Conditions),
		Contest: &contestEvent{Line: contestLineStoodUp},
	}
	if placed(target) {
		made.Col, made.Row = *target.GridCol, *target.GridRow
	}
	return made
}

// standUpMovementLeft reads the movement the combatant has left from the combat as the viewer reads
// it; zero when it is not there.
func standUpMovementLeft(out *playv1.Encounter, combatantID string) int32 {
	i := slices.IndexFunc(out.GetCombatants(), func(c *playv1.Combatant) bool { return c.GetId() == combatantID })
	if i < 0 {
		return 0
	}
	return out.GetCombatants()[i].GetMovementLeftDft()
}
