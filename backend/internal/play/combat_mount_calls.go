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
	"github.com/PuraFome/meuRPG/backend/internal/platform/idem"
	"github.com/PuraFome/meuRPG/backend/internal/play/playdb"
	"github.com/PuraFome/meuRPG/backend/internal/rules/grid"
)

// Mounting and dismounting (SRD 5.1, "Mounting and Dismounting"): a rider that is within 5 ft of a mount at least one size larger gets on
// it, or off it onto a free square next to it, at the cost of half its speed. It does so once in a move, and it does not Dash.

// mountReach is how far the mount may be, in feet: the rider is next to it, a diagonal square included.
const mountReach = 5

// mountNo is why a rider cannot mount or dismount now: what a write answers with, and what the turn options show.
type mountNo struct {
	reason  playv1.EncounterBlockedReason
	refusal rulesv1.MountRefusal
	msg     string
	ids     []string
	missing int // tenths of a foot, for the cost
}

// err is the failed_precondition of the refusal.
func (n *mountNo) err() error {
	name := func(b *playv1.EncounterBlocked) { b.CombatantIds = n.ids }
	if n.reason == playv1.EncounterBlockedReason_ENCOUNTER_BLOCKED_REASON_TOO_FAR {
		return tooFar(n.missing, name)
	}
	return errEncounter(n.reason, n.msg, name)
}

// willingMount says whether the viewer may mount the combatant (ruling of the mounts): the master mounts anything; a player mounts a
// creature of their own and a combatant of the party the master marked as a mount. A player's character is nobody's mount.
func willingMount(v combatViewer, mount playdb.Combatant) bool {
	switch {
	case mount.Kind == kindPlayer:
		return false
	case v.master:
		return true
	}
	return v.owns(mount) || (mount.MountKind != mountKindNone && mount.Side == sidePartyKey)
}

// mountVerdict says why the rider cannot get on the mount now, or nil. standing is the surface the rider stands on. The order is the
// order of the refusals.
func mountVerdict(cs []playdb.Combatant, rider, mount playdb.Combatant, standing grid.Surface) *mountNo {
	no := func(reason playv1.EncounterBlockedReason, refusal rulesv1.MountRefusal, msg string, ids ...string) *mountNo {
		return &mountNo{reason: reason, refusal: refusal, msg: msg, ids: ids}
	}
	switch {
	case rider.MountedOnID != nil:
		return no(playv1.EncounterBlockedReason_ENCOUNTER_BLOCKED_REASON_ALREADY_MOUNTED, rulesv1.MountRefusal_MOUNT_REFUSAL_ALREADY_MOUNTED, "the rider rides already: it gets off first", rider.ID)
	case riderOf(cs, rider.ID) != nil: // the carry is one level: a mount that carries a rider does not ride
		return no(playv1.EncounterBlockedReason_ENCOUNTER_BLOCKED_REASON_ALREADY_MOUNTED, rulesv1.MountRefusal_MOUNT_REFUSAL_ALREADY_MOUNTED, "a mount that carries a rider does not ride: the rider gets off first", rider.ID)
	case mount.MountedOnID != nil:
		return no(playv1.EncounterBlockedReason_ENCOUNTER_BLOCKED_REASON_ALREADY_MOUNTED, rulesv1.MountRefusal_MOUNT_REFUSAL_ALREADY_MOUNTED, "a combatant that rides is not a mount", mount.ID)
	case riderOf(cs, mount.ID) != nil:
		return no(playv1.EncounterBlockedReason_ENCOUNTER_BLOCKED_REASON_MOUNT_TAKEN, rulesv1.MountRefusal_MOUNT_REFUSAL_TAKEN, "the mount carries a rider", mount.ID)
	case sizeToGrid[sizeKey(mount.Size)] <= sizeToGrid[sizeKey(rider.Size)]:
		return no(playv1.EncounterBlockedReason_ENCOUNTER_BLOCKED_REASON_MOUNT_TOO_SMALL, rulesv1.MountRefusal_MOUNT_REFUSAL_TOO_SMALL, "a mount is at least one size larger than its rider", mount.ID)
	case !placed(rider) || !placed(mount) || grid.RangeFt(squareOfCombatant(rider), squareOfCombatant(mount)) > mountReach:
		return no(playv1.EncounterBlockedReason_ENCOUNTER_BLOCKED_REASON_MOUNT_TOO_FAR, rulesv1.MountRefusal_MOUNT_REFUSAL_TOO_FAR, "the mount is not within 5 ft", mount.ID)
	case rider.MountMoveUsed:
		return no(playv1.EncounterBlockedReason_ENCOUNTER_BLOCKED_REASON_MOUNT_MOVE_USED, rulesv1.MountRefusal_MOUNT_REFUSAL_MOVE_USED, "the rider mounted or dismounted in this move already", rider.ID)
	}
	return rideCost(rider, standing)
}

// rideCost is the refusal of a rider that cannot pay for a mount or a dismount: its speed is 0, or less movement is left than half of it.
func rideCost(rider playdb.Combatant, standing grid.Surface) *mountNo {
	base := rider
	base.Dashed = false
	cost := mountCostDFt(rider, standing)
	left := movementLeftAtDFt(rider, standing)
	if speedOnDFt(base, standing) > 0 && cost <= left {
		return nil
	}
	return &mountNo{reason: playv1.EncounterBlockedReason_ENCOUNTER_BLOCKED_REASON_TOO_FAR, refusal: rulesv1.MountRefusal_MOUNT_REFUSAL_NO_MOVEMENT, missing: max(cost-left, 0)}
}

// landingSquares are the squares a rider may get off onto: the neighbors of the mount that hold a creature of no kind, that a
// creature can stand on and that the rider can step to.
func landingSquares(t grid.Terrain, mount grid.Square, occ grid.OccupantMap, rider grid.Mover) []grid.Square {
	var out []grid.Square
	for dc := -1; dc <= 1; dc++ {
		for dr := -1; dr <= 1; dr++ {
			sq := grid.Square{Col: mount.Col + dc, Row: mount.Row + dr}
			if sq == mount || !t.Standable(sq) {
				continue
			}
			if _, taken := occ.At(sq); taken || t.Move(mount, sq, occ, rider).Blocked {
				continue
			}
			out = append(out, sq)
		}
	}
	return out
}

// dismountVerdict says why the rider cannot get off now, or nil. to is the square it asked for, or nil to ask only whether it could get off.
func dismountVerdict(rider playdb.Combatant, standing grid.Surface, landing []grid.Square, to *grid.Square) *mountNo {
	switch {
	case rider.MountMoveUsed:
		return &mountNo{reason: playv1.EncounterBlockedReason_ENCOUNTER_BLOCKED_REASON_MOUNT_MOVE_USED, refusal: rulesv1.MountRefusal_MOUNT_REFUSAL_MOVE_USED, msg: "the rider mounted or dismounted in this move already", ids: []string{rider.ID}}
	case to == nil && len(landing) == 0, to != nil && !slices.Contains(landing, *to):
		return &mountNo{reason: playv1.EncounterBlockedReason_ENCOUNTER_BLOCKED_REASON_NO_ROOM_TO_DISMOUNT, refusal: rulesv1.MountRefusal_MOUNT_REFUSAL_NO_ROOM, msg: "there is no free square for the rider next to the mount"}
	}
	return rideCost(rider, standing)
}

// rideCall is what MountCombatant and DismountCombatant read from their requests, checked.
type rideCall struct {
	m                authz.Membership
	key, encounterID string
	riderID, mountID string
	col, row         int32
	dismount         bool
}

// rideDone is what a ride's transaction made, for the publishers after the commit.
type rideDone struct {
	rider, mount playdb.Combatant // as they stand after the change
	from         *grid.Square     // where the rider stood
	secret       bool
	hook         *trapHook // the trap the ride fired, for the streams
	stopped      bool      // the dismount stopped on a square a creature the caller does not see holds: nobody moved
}

// MountCombatant implements playv1connect.CombatServiceHandler.
func (s *Service) MountCombatant(
	ctx context.Context,
	req *connect.Request[playv1.MountCombatantRequest],
) (*connect.Response[playv1.MountCombatantResponse], error) {
	m, key, encID, err := memberCall(ctx, req.Msg.GetCampaignId(), req.Msg.GetIdempotencyKey(), req.Msg.GetEncounterId())
	if err != nil {
		return nil, err
	}
	call := rideCall{m: m, key: key, encounterID: encID}
	if call.riderID, err = parseCombatID(req.Msg.GetRiderId(), "combatant"); err != nil {
		return nil, err
	}
	if call.mountID, err = parseCombatID(req.Msg.GetMountId(), "combatant"); err != nil {
		return nil, err
	}
	out, _, err := s.ride(ctx, call, idem.Hash(req.Msg), eventCombatantMounted)
	if err != nil {
		return nil, err
	}
	return connect.NewResponse(&playv1.MountCombatantResponse{Encounter: out, MovementLeftDft: standUpMovementLeft(out, call.riderID)}), nil
}

// DismountCombatant implements playv1connect.CombatServiceHandler.
func (s *Service) DismountCombatant(
	ctx context.Context,
	req *connect.Request[playv1.DismountCombatantRequest],
) (*connect.Response[playv1.DismountCombatantResponse], error) {
	m, key, encID, err := memberCall(ctx, req.Msg.GetCampaignId(), req.Msg.GetIdempotencyKey(), req.Msg.GetEncounterId())
	if err != nil {
		return nil, err
	}
	call := rideCall{m: m, key: key, encounterID: encID, dismount: true, col: req.Msg.GetCol(), row: req.Msg.GetRow()}
	if call.riderID, err = parseCombatID(req.Msg.GetRiderId(), "combatant"); err != nil {
		return nil, err
	}
	out, stopped, err := s.ride(ctx, call, idem.Hash(req.Msg), eventCombatantDismounted)
	if err != nil {
		return nil, err
	}
	return connect.NewResponse(&playv1.DismountCombatantResponse{Encounter: out, MovementLeftDft: standUpMovementLeft(out, call.riderID), StoppedEarly: stopped}), nil
}

// ride is the write of a mount or a dismount, with the answer the viewer reads and whether a dismount stopped early.
func (s *Service) ride(ctx context.Context, call rideCall, hash *string, kind string) (*playv1.Encounter, bool, error) {
	var done rideDone
	res, err := s.write(ctx, combatWrite{m: call.m, key: call.key, hash: hash, kind: kind, encounterID: call.encounterID}, func(c *combatTx) (any, error) {
		done = rideDone{}
		return s.rideWrite(ctx, c, call, &done)
	})
	if err != nil {
		return nil, false, s.dbError(ctx, "mount or dismount", err)
	}
	out, err := s.finish(ctx, call.m, res, func(ctx context.Context, d *encounterData) {
		s.publishRide(ctx, call.m.CampaignID, d, &done)
	})
	if err != nil {
		return nil, false, err
	}
	ev, err := resultEvent(res, actionEvent{StoppedEarly: done.stopped}) // a replay of the key reads it from the event
	if err != nil {
		return nil, false, s.dbError(ctx, "read the ride", err)
	}
	return out, ev.StoppedEarly, nil
}

// publishRide tells the streams that a rider mounted or dismounted: its square changed, and the log has a line unless a hidden
// combatant is in it.
func (s *Service) publishRide(ctx context.Context, campaignID string, d *encounterData, done *rideDone) {
	if done.stopped { // nobody moved and nothing was said: the table's copy only changed in its revision
		s.publishEncounterChangedFor(ctx, campaignID, d, done.rider.ID)
		return
	}
	s.publishCombatantMoved(ctx, campaignID, d.enc, done.rider, done.from)
	s.positionChanged(ctx, campaignID, d.enc, done.rider, done.from)
	s.publishEncounterChangedFor(ctx, campaignID, d, done.rider.ID, done.mount.ID)
	s.publishLogChanged(ctx, campaignID, d.enc.ID, !done.secret)
	done.hook.after(ctx, campaignID, d.enc)
}

// ridersOf reads who a ride is about and checks that the caller may make it now: the combatants, the viewer, the rider, the mount it
// gets on or off, and the surface the rider stands on.
func (s *Service) ridersOf(ctx context.Context, c *combatTx, call rideCall) (cs []playdb.Combatant, v combatViewer, rider, mount playdb.Combatant, standing grid.Surface, err error) {
	if err := notEnded(c.enc); err != nil {
		return nil, v, rider, mount, standing, err
	}
	if isTheatre(c.enc) {
		return nil, v, rider, mount, standing, errNeedsAMap() // no squares: nobody is next to a mount
	}
	if cs, err = listCombatants(ctx, c); err != nil {
		return nil, v, rider, mount, standing, err
	}
	v = c.viewer(call.m, cs)
	if rider, err = findCombatant(cs, call.riderID, v); err != nil {
		return nil, v, rider, mount, standing, err
	}
	if err := v.mayAct(rider); err != nil {
		return nil, v, rider, mount, standing, err
	}
	if mount, err = rideMountOf(cs, v, call, rider); err != nil {
		return nil, v, rider, mount, standing, err
	}
	if err := s.rideGates(ctx, c, call.m, v, rider); err != nil {
		return nil, v, rider, mount, standing, err
	}
	if standing, err = s.standingSurface(ctx, c.tx, c.enc, combatViewer{master: true}, rider); err != nil { // the real surface: the server decides
		return nil, v, rider, mount, standing, fmt.Errorf("read the surface the rider stands on: %w", err)
	}
	return cs, v, rider, mount, standing, nil
}

// rideWrite is the transaction of a mount or a dismount: who may, where the rider goes, what it costs, and the event the log and the
// undo keep.
func (s *Service) rideWrite(ctx context.Context, c *combatTx, call rideCall, done *rideDone) (any, error) {
	cs, v, rider, mount, standing, err := s.ridersOf(ctx, c, call)
	if err != nil {
		return nil, err
	}
	to, stopped, err := s.rideSquare(ctx, c, call, cs, v, rider, mount, standing)
	if err != nil || stopped {
		return s.dismountStopped(ctx, c, err, rider, mount, done)
	}
	cost := mountCostDFt(rider, standing)
	after, err := rideTo(ctx, c, rider, mount, to, cost, call.dismount)
	if err != nil {
		return nil, err
	}
	joined, err := rideInitiative(ctx, c, cs, rider, mount)
	if err != nil {
		return nil, err
	}
	if err := touch(ctx, c); err != nil {
		return nil, err
	}
	c.characterID = &rider.CharacterID
	*done = rideDone{rider: after, mount: mount, from: squareOfState(moveStateOf(rider)), secret: rider.Hidden || mount.Hidden}
	made := rideEvent(c.enc, rider, mount, to, cost, done.secret)
	made.Join = joined
	// The rider steps onto another square of the same map as a move does: the areas spells keep on it and the traps look at it.
	if err := s.zonesAfterSteps(ctx, c, zoneStep{who: rider.ID, from: done.from, path: []grid.Square{to}, travels: true}); err != nil {
		return nil, err
	}
	return s.fireTrapOnRide(ctx, c, call, cs, rider, after, to, made, done)
}

// fireTrapOnRide fires the trap on the square the rider ended on, as a move into it does: the ride's own event is written now and
// the firing is the change's own (so a retry finds it under the idempotency key, and an undo takes the firing back first, then the
// ride).
func (s *Service) fireTrapOnRide(ctx context.Context, c *combatTx, call rideCall, cs []playdb.Combatant, before, after playdb.Combatant, to grid.Square, made actionEvent, done *rideDone) (any, error) {
	th, err := s.newTrapHook(ctx, c.tx, call.m.CampaignID, c.enc, before)
	if err != nil {
		return nil, err
	}
	th.landed(to)
	ev, err := th.fire(ctx, c, cs, after)
	if err != nil || ev == nil {
		return made, err
	}
	kind := eventCombatantMounted
	if call.dismount {
		kind = eventCombatantDismounted
	}
	if err := insertEvent(ctx, c, kind, &c.actorUserID, nil, made); err != nil {
		return nil, err
	}
	th.fired, done.hook = ev, th
	c.kind = eventTrapTriggered
	return actionEvent{Round: c.enc.Round, Actor: after.ID, Trap: ev, OnTurn: made.OnTurn}, nil
}

// rideMountOf is the mount of the ride: the one the rider sits on for a dismount, the one the request names for a mount.
func rideMountOf(cs []playdb.Combatant, v combatViewer, call rideCall, rider playdb.Combatant) (playdb.Combatant, error) {
	if !call.dismount {
		return findCombatant(cs, call.mountID, v)
	}
	on := ridingOn(cs, rider)
	if on == nil {
		return playdb.Combatant{}, connect.NewError(connect.CodeInvalidArgument, errors.New("the combatant rides nothing"))
	}
	return *on, nil
}

// rideSquare is the square the rider ends on, after the checks of the ride: the mount's for a mount, the landing for a dismount.
// stopped says the dismount ends before it starts: a creature the caller does not see holds the square (see dismountSquare).
func (s *Service) rideSquare(ctx context.Context, c *combatTx, call rideCall, cs []playdb.Combatant, v combatViewer, rider, mount playdb.Combatant, standing grid.Surface) (to grid.Square, stopped bool, err error) {
	if call.dismount {
		return s.dismountSquare(ctx, c, call, cs, v, rider, mount, standing)
	}
	if err := s.mayBeMounted(v, rider, mount, cs, standing); err != nil {
		return grid.Square{}, false, err
	}
	return squareOfCombatant(mount), false, nil
}

// rideGates refuses the rider that may not act now: out of its turn, down, surprised, unable to act, or the turn waits.
func (s *Service) rideGates(ctx context.Context, c *combatTx, m authz.Membership, v combatViewer, rider playdb.Combatant) error {
	if err := s.standUpGates(ctx, c, m, v, rider); err != nil {
		return err
	}
	if code := cannotActCode(rider); code != rulesv1.DisabledReasonCode_DISABLED_REASON_CODE_UNSPECIFIED {
		return gateError(code)
	}
	return nil
}

// mayBeMounted checks the rules of the mount: the mount is one the viewer may mount, and the refusals of mountVerdict.
func (s *Service) mayBeMounted(v combatViewer, rider, mount playdb.Combatant, cs []playdb.Combatant, standing grid.Surface) error {
	if mount.ID == rider.ID {
		return connect.NewError(connect.CodeInvalidArgument, errors.New("a combatant does not mount itself"))
	}
	if rider.MountedOnID == nil && (mount.Defeated || !willingMount(v, mount)) || hiddenPair(cs, v, rider, mount) { // a rider that rides is refused first, whatever it asked
		return errEncounter(playv1.EncounterBlockedReason_ENCOUNTER_BLOCKED_REASON_NOT_A_MOUNT, "that combatant is not a willing mount", func(b *playv1.EncounterBlocked) { b.CombatantIds = []string{mount.ID} })
	}
	if no := mountVerdict(cs, rider, mount, standing); no != nil {
		return no.err()
	}
	return nil
}

// hiddenPair says whether the viewer is kept from a combatant it does not see in the pair of the rider or the mount: a rider that sits
// on the mount, a mount that carries the rider or that rides another. The refusals that would name it are answered as for a combatant
// that is no mount, and the options do not list the mount (RN-10). The master sees every row, so it is never kept from one.
func hiddenPair(cs []playdb.Combatant, v combatViewer, rider, mount playdb.Combatant) bool {
	unseen := func(c *playdb.Combatant) bool { return c != nil && !v.sees(*c) }
	return unseen(riderOf(cs, mount.ID)) || unseen(riderOf(cs, rider.ID)) || unseen(ridingOn(cs, mount)) || unseen(ridingOn(cs, rider))
}

// dismountSquare is the square the rider lands on, or the refusal. The viewer plans on the creatures it sees, as the options offer
// the squares, so a square that only a creature it does not see holds is a square it may choose; the real landing runs on everyone
// and, as a move does (movement.md), stops before that creature: stopped is true, nothing is spent and no refusal says why (RN-10).
func (s *Service) dismountSquare(ctx context.Context, c *combatTx, call rideCall, cs []playdb.Combatant, v combatViewer, rider, mount playdb.Combatant, standing grid.Surface) (to grid.Square, stopped bool, err error) {
	to = grid.Square{Col: int(call.col), Row: int(call.row)}
	terrain, err := s.terrainOf(ctx, c.tx, call.m.CampaignID, c.enc)
	if err != nil {
		return to, false, err
	}
	mover, err := s.moverWith(ctx, c.tx, call.m.CampaignID, rider)
	if err != nil {
		return to, false, err
	}
	at := squareOfCombatant(mount)
	planned := landingSquares(terrain, at, occupantsFor(cs, rider, v), mover)
	if no := dismountVerdict(rider, standing, planned, &to); no != nil {
		return to, false, no.err()
	}
	actual := landingSquares(terrain, at, occupantsFor(cs, rider, combatViewer{master: true}), mover)
	return to, !slices.Contains(actual, to), nil
}

// dismountStopped is the answer of a dismount that did not get to the landing: the refusal, or the write of a dismount a hidden creature stopped: the rider stays on the mount, nothing is spent and the move
// of the mount's ride is not used. It leaves the event of a move that goes nowhere, which the log skips as it skips any such move.
func (s *Service) dismountStopped(ctx context.Context, c *combatTx, refused error, rider, mount playdb.Combatant, done *rideDone) (any, error) {
	if refused != nil {
		return nil, refused
	}
	if err := touch(ctx, c); err != nil {
		return nil, err
	}
	c.kind = eventCombatantMoved
	c.characterID = &rider.CharacterID
	*done = rideDone{rider: rider, mount: mount, stopped: true}
	return actionEvent{
		Round: c.enc.Round, Actor: rider.ID, OnTurn: true, StoppedEarly: true, From: moveStateOf(rider),
		Col: clamp32(squareOfCombatant(rider).Col, 0, math.MaxInt32), Row: clamp32(squareOfCombatant(rider).Row, 0, math.MaxInt32),
	}, nil
}

// rideTo writes the ride: the link, the rider's square, the movement it cost and the mark of the move. It answers the rider as it stands.
func rideTo(ctx context.Context, c *combatTx, rider, mount playdb.Combatant, to grid.Square, cost int, dismount bool) (playdb.Combatant, error) {
	used := int(rider.MovementUsedDft) + cost
	col, row := clamp32(to.Col, 0, math.MaxInt32), clamp32(to.Row, 0, math.MaxInt32)
	var on *string
	if !dismount {
		on = &mount.ID
	}
	cover := rider.CoverMark
	if placed(rider) && to != squareOfCombatant(rider) {
		cover = "none"
	}
	p := playdb.SetCombatantRideParams{
		ID: rider.ID, MountedOnID: on, GridCol: &col, GridRow: &row,
		MovementUsedFt: clamp32(used/10, 0, math.MaxInt32), MovementUsedDft: clamp32(used, 0, math.MaxInt32), MountMoveUsed: true, CoverMark: cover,
	}
	if err := c.q.SetCombatantRide(ctx, p); err != nil {
		return rider, fmt.Errorf("write the ride: %w", err)
	}
	rider.MountedOnID, rider.GridCol, rider.GridRow = p.MountedOnID, p.GridCol, p.GridRow
	rider.MovementUsedFt, rider.MovementUsedDft, rider.MountMoveUsed, rider.CoverMark = p.MovementUsedFt, p.MovementUsedDft, true, cover
	return rider, nil
}

// rideEvent is the event of a mount or a dismount: the rider is the actor and the mount the target. The master's undo puts the rider
// back from From, and the line of the log is a master's alone when either of them is hidden (RN-10).
func rideEvent(enc playdb.Encounter, rider, mount playdb.Combatant, to grid.Square, cost int, secret bool) actionEvent {
	return actionEvent{
		Round: enc.Round, Secret: secret, Actor: rider.ID, Target: mount.ID, OnTurn: true, From: moveStateOf(rider),
		CostDFt: clamp32(cost, 0, math.MaxInt32), CostFt: clamp32(cost/10, 0, math.MaxInt32),
		Col: clamp32(to.Col, 0, math.MaxInt32), Row: clamp32(to.Row, 0, math.MaxInt32),
	}
}

// attachMountOptions puts on the turn options what the combatant can do about mounts, as the viewer may know it: the mounts it sees,
// and, when it rides, how it gets off. Nothing out of a grid combat, for a combatant with no square and for a defeated one. terrain is
// what the viewer knows of the map, so a player is not told of a wall they have not seen.
func (s *Service) attachMountOptions(ctx context.Context, campaignID string, d *encounterData, v combatViewer, who playdb.Combatant, terrain grid.Terrain, opts *rulesv1.TurnOptions) error {
	if opts == nil || isTheatre(d.enc) || !placed(who) || who.Defeated {
		return nil
	}
	standing, err := s.standingSurface(ctx, nil, d.enc, v, who)
	if err != nil {
		return s.dbError(ctx, "read the surface the combatant stands on", err)
	}
	out := &rulesv1.MountOptions{}
	opts.Mount = out
	if who.MountedOnID == nil {
		out.Mounts = mountChoices(d.cs, v, who, standing)
		return nil
	}
	mount := ridingOn(d.cs, who)
	if mount == nil || !v.sees(*mount) { // a hidden mount is not on offer to a player (RN-10)
		return nil
	}
	mover, err := s.moverWith(ctx, nil, campaignID, who)
	if err != nil {
		return err
	}
	dismountOptions(out, d.cs, v, who, *mount, terrain, mover, standing)
	return nil
}

// dismountOptions fills what a rider can do to get off: the mount, the cost, why it cannot now and the squares to land on.
func dismountOptions(out *rulesv1.MountOptions, cs []playdb.Combatant, v combatViewer, rider, mount playdb.Combatant, terrain grid.Terrain, mover grid.Mover, standing grid.Surface) {
	landing := landingSquares(terrain, squareOfCombatant(mount), occupantsFor(cs, rider, v), mover)
	out.MountedOnId = mount.ID
	out.DismountCostDft = clamp32(mountCostDFt(rider, standing), 0, math.MaxInt32)
	if no := dismountVerdict(rider, standing, landing, nil); no != nil {
		out.DismountRefusal, out.MissingDft = no.refusal, clamp32(no.missing, 0, math.MaxInt32)
	}
	for _, sq := range landing {
		out.Landing = append(out.Landing, &rulesv1.MountSquare{Col: clamp32(sq.Col, 0, math.MaxInt32), Row: clamp32(sq.Row, 0, math.MaxInt32)})
	}
}

// listedMount says whether the options list the mount: a player's are the ones willingMount lets them mount; the master may mount any
// combatant, so only the marked ones and the ones next to the rider are listed.
func listedMount(v combatViewer, rider, mount playdb.Combatant) bool {
	return !v.master || mount.MountKind != mountKindNone || (placed(rider) && grid.RangeFt(squareOfCombatant(rider), squareOfCombatant(mount)) <= mountReach)
}

// mountChoices are the mounts the viewer sees that its rider could get on, each with the cost and why not.
func mountChoices(cs []playdb.Combatant, v combatViewer, rider playdb.Combatant, standing grid.Surface) []*rulesv1.MountChoice {
	var out []*rulesv1.MountChoice
	for _, mount := range cs {
		if mount.ID == rider.ID || mount.Defeated || !placed(mount) || !v.sees(mount) || !willingMount(v, mount) || !listedMount(v, rider, mount) || hiddenPair(cs, v, rider, mount) {
			continue
		}
		choice := &rulesv1.MountChoice{CombatantId: mount.ID, CostDft: clamp32(mountCostDFt(rider, standing), 0, math.MaxInt32)}
		if no := mountVerdict(cs, rider, mount, standing); no != nil {
			choice.Refusal, choice.MissingDft = no.refusal, clamp32(no.missing, 0, math.MaxInt32)
		}
		out = append(out, choice)
	}
	return out
}
