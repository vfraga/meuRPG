package play

import (
	"context"
	"errors"
	"fmt"

	"connectrpc.com/connect"

	playv1 "github.com/PuraFome/meuRPG/backend/gen/meurpg/play/v1"
	"github.com/PuraFome/meuRPG/backend/internal/platform/idem"
	"github.com/PuraFome/meuRPG/backend/internal/play/playdb"
	"github.com/PuraFome/meuRPG/backend/internal/rules/grid"
)

// Mounted combat (SRD 5.1, "Mounted Combat"). The rider and its mount stay two combatants: the rider row names the mount it sits
// on (mounted_on_id), the mount row says whether it is a willing mount and how it obeys (mount_kind).
//
// The kinds of a mount: "" is not a mount, "controlled" obeys its rider and "independent" keeps its own turn.
const (
	mountKindNone        = ""
	mountKindControlled  = "controlled"
	mountKindIndependent = "independent"
)

// mountKindOf is the kind the request names, and false for a word that is not one of the three.
func mountKindOf(kind string) (string, bool) {
	switch kind {
	case mountKindNone, mountKindControlled, mountKindIndependent:
		return kind, true
	}
	return "", false
}

// SetCombatantMount implements playv1connect.CombatServiceHandler.
func (s *Service) SetCombatantMount(
	ctx context.Context,
	req *connect.Request[playv1.SetCombatantMountRequest],
) (*connect.Response[playv1.SetCombatantMountResponse], error) {
	call, err := readMasterTargetCall(ctx, req.Msg.GetCampaignId(), req.Msg.GetIdempotencyKey(), req.Msg.GetEncounterId(), req.Msg.GetCombatantId())
	if err != nil {
		return nil, err
	}
	kind, ok := mountKindOf(req.Msg.GetKind())
	if !ok {
		return nil, connect.NewError(connect.CodeInvalidArgument, errors.New(`kind must be "controlled", "independent" or ""`))
	}
	m := call.m
	res, err := s.write(ctx, combatWrite{m: m, key: call.key, hash: idem.Hash(req.Msg), kind: eventMountKindSet, encounterID: call.encID}, func(c *combatTx) (any, error) {
		return s.mountKindIn(ctx, c, call, kind)
	})
	if err != nil {
		return nil, s.dbError(ctx, "mark a combatant as a mount", err)
	}
	out, err := s.finish(ctx, m, res, s.changedFor(m.CampaignID, call.combID))
	if err != nil {
		return nil, err
	}
	return connect.NewResponse(&playv1.SetCombatantMountResponse{Encounter: out}), nil
}

// mountKindIn is the change of SetCombatantMount: the master marks a combatant as a willing mount, or takes the mark off. A
// player's character is never a mount, a rider is not marked, and the mark stays on a mount that carries a rider.
func (s *Service) mountKindIn(ctx context.Context, c *combatTx, call masterTargetCall, kind string) (any, error) {
	if err := notEnded(c.enc); err != nil {
		return nil, err
	}
	target, err := s.masterTargetOf(ctx, c, call)
	if err != nil {
		return nil, err
	}
	if target.Kind == kindPlayer {
		return nil, connect.NewError(connect.CodeInvalidArgument, errors.New("a player's character is not a mount"))
	}
	if target.MountedOnID != nil {
		return nil, errEncounter(playv1.EncounterBlockedReason_ENCOUNTER_BLOCKED_REASON_ALREADY_MOUNTED, "a combatant that rides is not a mount: it gets off first",
			func(b *playv1.EncounterBlocked) { b.CombatantIds = []string{target.ID} })
	}
	if kind == mountKindNone {
		if err := mustCarryNobody(ctx, c, target.ID); err != nil {
			return nil, err
		}
	}
	if err := c.q.SetCombatantMountKind(ctx, playdb.SetCombatantMountKindParams{ID: target.ID, MountKind: kind}); err != nil {
		return nil, fmt.Errorf("set the mount kind: %w", err)
	}
	if err := settleMounts(ctx, c); err != nil { // a mount that carries a rider takes its initiative or gives it back with its kind
		return nil, err
	}
	if err := touch(ctx, c); err != nil {
		return nil, err
	}
	c.characterID = &target.CharacterID
	return actionEvent{Round: c.enc.Round, Secret: target.Hidden, Actor: target.ID, Key: kind}, nil
}

// mustCarryNobody refuses with MOUNT_TAKEN when a combatant sits on the mount.
func mustCarryNobody(ctx context.Context, c *combatTx, mountID string) error {
	cs, err := listCombatants(ctx, c)
	if err != nil {
		return err
	}
	if riderOf(cs, mountID) != nil {
		return errEncounter(playv1.EncounterBlockedReason_ENCOUNTER_BLOCKED_REASON_MOUNT_TAKEN, "the mount carries a rider: the rider gets off first",
			func(b *playv1.EncounterBlocked) { b.CombatantIds = []string{mountID} })
	}
	return nil
}

// riderOf is the combatant that sits on the mount, or nil.
func riderOf(cs []playdb.Combatant, mountID string) *playdb.Combatant {
	for i := range cs {
		if cs[i].MountedOnID != nil && *cs[i].MountedOnID == mountID {
			return &cs[i]
		}
	}
	return nil
}

// mountFor puts on out, the copy of c that the viewer reads, what the viewer may know of the mount and the rider (RN-10, RN-20).
// The link goes to a viewer who sees both rows, so a hidden mount leaves its rider plain and a hidden rider leaves its mount
// without one. The mark is the master's and the player's who controls the combatant: it says who the master made a mount of.
// cs is every combatant of the combat, in whatever order.
func mountFor(out *playv1.Combatant, c playdb.Combatant, cs []playdb.Combatant, v combatViewer) {
	if v.master || v.owns(c) {
		out.MountKind = c.MountKind
	}
	if c.MountedOnID != nil {
		for _, other := range cs {
			if other.ID == *c.MountedOnID && v.sees(other) {
				out.MountedOnId = other.ID
			}
		}
	}
	if rider := riderOf(cs, c.ID); rider != nil && v.sees(*rider) {
		out.RiderId = rider.ID
	}
}

// ridingOn is the mount the combatant sits on among cs, or nil when it rides nothing.
func ridingOn(cs []playdb.Combatant, c playdb.Combatant) *playdb.Combatant {
	if c.MountedOnID == nil {
		return nil
	}
	for i := range cs {
		if cs[i].ID == *c.MountedOnID {
			return &cs[i]
		}
	}
	return nil
}

// samePair says whether a rides b or b rides a: the two are one occupant of their square, and neither is in the other's way.
func samePair(a, b playdb.Combatant) bool {
	return (a.MountedOnID != nil && *a.MountedOnID == b.ID) || (b.MountedOnID != nil && *b.MountedOnID == a.ID)
}

// carryRider puts the rider of the mount on the mount's square. Every write that changes the square of a combatant calls it for
// that combatant, after the square is written: the pair never stands on two squares.
func carryRider(ctx context.Context, q *playdb.Queries, mountID string) error {
	if err := q.CarryRiders(ctx, mountID); err != nil {
		return fmt.Errorf("carry the rider: %w", err)
	}
	return nil
}

// unseat is a rider that a dismissal took off its mount, as the event keeps it.
type unseat struct {
	Rider string `json:"rider"`
	Mount string `json:"mount"`
}

// setRiderLink writes the mount the rider sits on (nil: none) and leaves the rest of the row as it is.
func setRiderLink(ctx context.Context, q *playdb.Queries, rider playdb.Combatant, on *string) error {
	if err := q.SetCombatantRide(ctx, playdb.SetCombatantRideParams{
		ID: rider.ID, MountedOnID: on, GridCol: rider.GridCol, GridRow: rider.GridRow,
		MovementUsedFt: rider.MovementUsedFt, MovementUsedDft: rider.MovementUsedDft, MountMoveUsed: rider.MountMoveUsed, CoverMark: rider.CoverMark,
	}); err != nil {
		return fmt.Errorf("write the rider's link: %w", err)
	}
	return nil
}

// seatRiders puts the riders a dismissal took off their mounts back on them, for the mounts and riders that are in cs again and
// where the rider still rides nothing.
func seatRiders(ctx context.Context, q *playdb.Queries, cs []playdb.Combatant, unseated []unseat) error {
	for _, u := range unseated {
		var rider, mount *playdb.Combatant
		for i := range cs {
			switch cs[i].ID {
			case u.Rider:
				rider = &cs[i]
			case u.Mount:
				mount = &cs[i]
			}
		}
		if rider == nil || mount == nil || rider.MountedOnID != nil {
			continue
		}
		if err := setRiderLink(ctx, q, *rider, &mount.ID); err != nil {
			return err
		}
	}
	return nil
}

// moveStateIn is moveStateOf for a combatant among cs: a mount keeps its rider's cover mark too.
func moveStateIn(cs []playdb.Combatant, c playdb.Combatant) *moveState {
	st := moveStateOf(c)
	if rider := riderOf(cs, c.ID); rider != nil {
		st.RiderID, st.RiderCover = rider.ID, rider.CoverMark
	}
	return st
}

// carryRiderBack is carryRider for an undo: the rider goes back with its mount and gets its cover mark back, which the carry cleared.
func carryRiderBack(ctx context.Context, q *playdb.Queries, mountID string, from *moveState) error {
	if err := carryRider(ctx, q, mountID); err != nil {
		return err
	}
	if from == nil || from.RiderID == "" || from.RiderCover == "" || from.RiderCover == "none" {
		return nil
	}
	if err := q.SetCombatantCoverMark(ctx, playdb.SetCombatantCoverMarkParams{ID: from.RiderID, CoverMark: from.RiderCover}); err != nil {
		return fmt.Errorf("put back the rider's cover mark: %w", err)
	}
	return nil
}

// drives says whether the viewer's player moves the combatant for its rider: a controlled mount obeys the rider, whose player the
// viewer is.
func (v combatViewer) drives(cs []playdb.Combatant, c playdb.Combatant) bool {
	if c.MountKind != mountKindControlled {
		return false
	}
	rider := riderOf(cs, c.ID)
	return rider != nil && v.owns(*rider)
}

// mayMove checks that the viewer may move the combatant: the master, its player, or the player of the rider of a controlled mount.
func (v combatViewer) mayMove(cs []playdb.Combatant, c playdb.Combatant) error {
	if v.drives(cs, c) {
		return nil
	}
	return v.mayAct(c)
}

// mountedFor is the refusal of a rider's own move or teleport, as the viewer may be told: MOUNTED, unless the viewer does not see the
// mount (it is hidden): then it is the refusal the viewer gets for something in the way, and says nothing of what the rider sits on.
func mountedFor(v combatViewer, cs []playdb.Combatant, rider playdb.Combatant, otherwise error) error {
	if mount := ridingOn(cs, rider); mount != nil && !v.sees(*mount) {
		return otherwise
	}
	return mountedError(rider.ID)
}

// mountedError is the MOUNTED refusal: a rider is moved by its mount, not on its own.
func mountedError(riderID string) error {
	return errEncounter(playv1.EncounterBlockedReason_ENCOUNTER_BLOCKED_REASON_MOUNTED, "a rider moves with its mount: it gets off first",
		func(b *playv1.EncounterBlocked) { b.CombatantIds = []string{riderID} })
}

// publishRideAlong tells the streams and the fog that the rider of a mount moved with it, after the commit: it moved from the same
// square as its mount did.
func (s *Service) publishRideAlong(ctx context.Context, campaignID string, d *encounterData, mount playdb.Combatant, from *grid.Square) {
	rider := riderOf(d.cs, mount.ID)
	if rider == nil || !placed(*rider) {
		return
	}
	s.publishCombatantMoved(ctx, campaignID, d.enc, *rider, from)
	s.positionChanged(ctx, campaignID, d.enc, *rider, from)
}

// mountBack takes a mount back: the rider is on foot where it stood, with the movement it had used, and its move is free again.
func (u *undoer) mountBack(ctx context.Context, ev actionEvent) error {
	return u.rideBack(ctx, ev, nil)
}

// dismountBack takes a dismount back: the rider is on its mount again, on the mount's square, with the movement it had used.
func (u *undoer) dismountBack(ctx context.Context, ev actionEvent) error {
	return u.rideBack(ctx, ev, &ev.Target)
}

// rideBack puts the rider's link, square, movement and cover mark back as the event kept them; on is the mount it sat on, or nil. The
// move of the turn was free, or the ride could not have been made.
func (u *undoer) rideBack(ctx context.Context, ev actionEvent, on *string) error {
	who, ok := u.find(ev.Actor)
	if ev.From == nil {
		return errEncounter(playv1.EncounterBlockedReason_ENCOUNTER_BLOCKED_REASON_NOTHING_TO_UNDO, "this ride cannot be undone")
	}
	if !ok {
		return nil
	}
	var col, row *int32
	if ev.From.Placed {
		col, row = &ev.From.Col, &ev.From.Row
	}
	if err := u.c.q.SetCombatantRide(ctx, playdb.SetCombatantRideParams{
		ID: who.ID, MountedOnID: on, GridCol: col, GridRow: row,
		MovementUsedFt: ev.From.UsedDFt / undoDFtPerFt, MovementUsedDft: ev.From.UsedDFt, MountMoveUsed: false, CoverMark: coverOrNone(ev.From.Cover),
	}); err != nil {
		return fmt.Errorf("put back the ride: %w", err)
	}
	return u.joinBack(ctx, ev.Join)
}
