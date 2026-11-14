package play

import (
	"context"
	"errors"
	"fmt"
	"math"

	"connectrpc.com/connect"

	playv1 "github.com/PuraFome/meuRPG/backend/gen/meurpg/play/v1"
	"github.com/PuraFome/meuRPG/backend/internal/play/playdb"
	"github.com/PuraFome/meuRPG/backend/internal/rules/grid"
)

// A teleport spell on a map (SRD 5.1, Misty Step): the caster's player picks the square the caster lands on, in
// the cast itself. The server checks the square before anything is spent, and the cast then puts the caster on
// it: no movement is spent, no opportunity attack is offered, the master's cover mark goes, and the undo puts the
// caster back. Without a square the cast changes nothing on the map and the master moves the token.

// mistyStepReachFt is how far Misty Step takes the caster, in feet.
const mistyStepReachFt = 30

// teleportReachFt is how far each teleport spell takes the caster, in feet.
var teleportReachFt = map[string]int{"spell:misty-step": mistyStepReachFt}

// teleportEvent is where a teleport put the caster and where it stood.
type teleportEvent struct {
	From *moveState `json:"from,omitempty"`
	Col  int32      `json:"col,omitempty"`
	Row  int32      `json:"row,omitempty"`
}

// errUnavailable is the refusal of a square a player may not use. It is the same for a wall, a creature (seen
// or not) and a square out of sight, so it never tells a player what they do not see (RN-10).
func errUnavailable() error {
	return errEncounter(playv1.EncounterBlockedReason_ENCOUNTER_BLOCKED_REASON_DESTINATION_UNAVAILABLE, "that square is not available")
}

// checkTeleport checks the square a teleport spell was given, before anything is spent.
func (f *castFlow) checkTeleport(context.Context) error {
	dest := f.req.GetDestination()
	reach, teleports := teleportReachFt[f.call.spellKey]
	if dest == nil {
		return f.requireDestination(teleports)
	}
	switch {
	case !teleports:
		return connect.NewError(connect.CodeInvalidArgument, errors.New("destination is for a teleport spell"))
	case unseenMountOf(f.cs, f.v, f.caster) == nil && f.caster.MountedOnID != nil:
		return mountedError(f.caster.ID) // a rider does not teleport alone: it gets off first
	case isTheatre(f.c.enc):
		return connect.NewError(connect.CodeInvalidArgument, errors.New("this combat has no map: there is no square to teleport to"))
	case !inGrid(f.c.enc, dest.GetCol(), dest.GetRow()):
		return connect.NewError(connect.CodeInvalidArgument, errors.New("destination must be a square of the grid"))
	case !placed(f.caster):
		return errEncounter(playv1.EncounterBlockedReason_ENCOUNTER_BLOCKED_REASON_NOT_PLACED, "the caster is not on the map yet")
	}
	to := grid.Square{Col: int(dest.GetCol()), Row: int(dest.GetRow())}
	if grid.RangeFt(squareOfCombatant(f.caster), to) > reach {
		return errEncounter(playv1.EncounterBlockedReason_ENCOUNTER_BLOCKED_REASON_TELEPORT_TOO_FAR, "that square is farther than the spell reaches")
	}
	if err := f.squareFree(to); err != nil {
		return err
	}
	if unseenMountOf(f.cs, f.v, f.caster) != nil {
		return errUnavailable() // the rider of a mount the caller does not see is refused as for a square it may not use, after the checks of a caster on foot
	}
	f.landing = &to
	return nil
}

// requireDestination refuses a player's cast of a teleport spell on a map that came without a square: only the
// master may leave it out (and then moves the token himself).
func (f *castFlow) requireDestination(teleports bool) error {
	if !teleports || f.v.master || isTheatre(f.c.enc) {
		return nil
	}
	return errEncounter(playv1.EncounterBlockedReason_ENCOUNTER_BLOCKED_REASON_DESTINATION_REQUIRED, "pick the square to teleport to")
}

// squareFree refuses a square with a wall, a creature or, for a player, out of the caster's sight. The master is
// told which; a player is only told it is not available.
func (f *castFlow) squareFree(to grid.Square) error {
	wall := f.terrain.Walls.Has(to)
	taken := occupiedBy(f.cs, f.caster, to)
	switch {
	case f.v.master && wall:
		return errWall()
	case f.v.master && taken:
		return errEncounter(playv1.EncounterBlockedReason_ENCOUNTER_BLOCKED_REASON_SQUARE_OCCUPIED, "another combatant is on that square")
	case wall || taken || (!f.v.master && f.v.sight != nil && !f.v.sight.sight.Sees(f.v.userID, to)):
		return errUnavailable()
	}
	return nil
}

// occupiedBy says whether a combatant other than who, standing, is on the square: a hidden one too.
func occupiedBy(cs []playdb.Combatant, who playdb.Combatant, sq grid.Square) bool {
	for _, o := range cs {
		if o.ID != who.ID && !samePair(o, who) && !o.Defeated && placed(o) && squareOfCombatant(o) == sq {
			return true
		}
	}
	return false
}

// teleport puts the caster on the square the cast chose.
func (f *castFlow) teleport(ctx context.Context) error {
	if f.landing == nil {
		return nil
	}
	col, row := clamp32(f.landing.Col, 0, math.MaxInt32), clamp32(f.landing.Row, 0, math.MaxInt32)
	if err := f.c.q.SetCombatantPlace(ctx, playdb.SetCombatantPlaceParams{ID: f.caster.ID, GridCol: &col, GridRow: &row, CoverMark: "none"}); err != nil {
		return fmt.Errorf("teleport the caster: %w", err)
	}
	if err := carryRider(ctx, f.c.q, f.caster.ID); err != nil {
		return err
	}
	f.out.made.Teleport = &teleportEvent{From: moveStateIn(f.cs, f.caster), Col: col, Row: row}
	moved := f.caster
	moved.GridCol, moved.GridRow, moved.CoverMark = &col, &row, "none"
	f.landed = &moved
	from := squareOfCombatant(f.caster)
	// The landing comes into the areas spells keep on the map as a step does (zones_hooks.go).
	return f.s.zonesAfterSteps(ctx, f.c, zoneStep{who: f.caster.ID, from: &from, path: []grid.Square{*f.landing}})
}

// fireTrapOnLanding fires the trap on the square the caster landed on, as a move into it does: the cast's own
// event is written now and the firing is the change's own (so a retry finds it under the idempotency key and an
// undo takes the firing back first, then the cast). The answer is still the cast's: the firing carries its fields.
func (f *castFlow) fireTrapOnLanding(ctx context.Context, made actionEvent) (any, error) {
	if f.landed == nil {
		return made, nil
	}
	th, err := f.s.newTrapHook(ctx, f.c.tx, f.m.CampaignID, f.c.enc, f.caster)
	if err != nil {
		return nil, err
	}
	th.landed(*f.landing)
	ev, err := th.fire(ctx, f.c, f.cs, *f.landed)
	if err != nil || ev == nil {
		return made, err
	}
	if err := insertEvent(ctx, f.c, eventSpellCast, &f.c.actorUserID, nil, made); err != nil {
		return nil, err
	}
	th.fired = ev
	f.out.hook = th
	f.c.kind = eventTrapTriggered
	f.out.made.Trap = ev
	return f.out.made, nil
}

// castTeleportBack puts the caster back where it stood before a teleport.
func (u *undoer) castTeleportBack(ctx context.Context, who playdb.Combatant, ev actionEvent) error {
	if ev.Teleport == nil || ev.Teleport.From == nil {
		return nil
	}
	from := ev.Teleport.From
	var col, row *int32
	if from.Placed {
		col, row = &from.Col, &from.Row
	}
	if err := u.c.q.SetCombatantPlace(ctx, playdb.SetCombatantPlaceParams{ID: who.ID, GridCol: col, GridRow: row, CoverMark: coverOrNone(from.Cover)}); err != nil {
		return fmt.Errorf("put back the teleport: %w", err)
	}
	return carryRiderBack(ctx, u.c.q, who.ID, from)
}

// publishTeleport tells the streams and the fog that the caster landed, after the commit.
func (s *Service) publishTeleport(ctx context.Context, campaignID string, d *encounterData, ev actionEvent) {
	if ev.Teleport == nil {
		return
	}
	moved, ok := combatantByID(d.cs, ev.Actor)
	if !ok {
		return
	}
	s.publishCombatantMoved(ctx, campaignID, d.enc, moved, squareOfState(ev.Teleport.From))
	if from := ev.Teleport.From; from != nil && from.Cover != "" && from.Cover != "none" { // the mark is on the combatant: who saw it reads again
		s.publishCoverCleared(ctx, campaignID, d.enc, moved)
	}
	s.positionChanged(ctx, campaignID, d.enc, moved, squareOfState(ev.Teleport.From))
	s.publishRideAlong(ctx, campaignID, d, moved, squareOfState(ev.Teleport.From))
	s.noticeAfterMove(ctx, campaignID, d.enc, moved) // a trap near where the caster landed may be noticed, as after a move
}

// unseenMountOf is the mount a rider sits on when the viewer does not see it, or nil: a rider that is not on one, or whose mount the
// viewer sees.
func unseenMountOf(cs []playdb.Combatant, v combatViewer, rider playdb.Combatant) *playdb.Combatant {
	if mount := ridingOn(cs, rider); mount != nil && !v.sees(*mount) {
		return mount
	}
	return nil
}
