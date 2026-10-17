package play

import (
	"context"
	"errors"
	"fmt"
	"math"
	"slices"
	"strings"

	"connectrpc.com/connect"
	"github.com/jackc/pgx/v5"

	playv1 "github.com/PuraFome/meuRPG/backend/gen/meurpg/play/v1"
	rulesv1 "github.com/PuraFome/meuRPG/backend/gen/meurpg/rules/v1"
	"github.com/PuraFome/meuRPG/backend/internal/authz"
	"github.com/PuraFome/meuRPG/backend/internal/platform/idem"
	"github.com/PuraFome/meuRPG/backend/internal/play/live"
	"github.com/PuraFome/meuRPG/backend/internal/play/playdb"
	"github.com/PuraFome/meuRPG/backend/internal/rules/combat"
	"github.com/PuraFome/meuRPG/backend/internal/rules/grid"
)

// Combat movement (MR-034, RN-21, Etapa 9, slice 9.6). The geometry is package
// rules/grid's (the circle, the cost of a straight line, walls, other
// creatures, jumps); this file asks it and keeps the combatant's numbers. The
// browser does no geometry: GetMoveOptions says where a combatant can go.

// TerrainSource gives a combat the layers of the map it runs on: walls,
// difficult terrain and cover. The maps module implements it
// (maps.Service.Terrain) and cmd/api connects it with SetTerrain, after both
// services exist. A nil source, and a map with no layers, are open floor.
type TerrainSource interface {
	// Terrain returns the map's grid and layers, or a `not_found` Connect error
	// when mapID is not a map of the campaign.
	Terrain(ctx context.Context, tx pgx.Tx, campaignID, mapID string) (grid.Terrain, error)
}

// DoorKeeper opens the doors a move walks into (MR-010, RN-26). The maps module
// implements it too (maps.Service.OpenDoors and DoorsChanged), and SetTerrain
// picks it up from the same value, so there is nothing more to wire.
type DoorKeeper interface {
	// OpenDoors opens, inside tx, the doors at the squares of the map that are
	// still closed, and returns the ones it opened. It raises the layers' revision
	// of the map.
	OpenDoors(ctx context.Context, tx pgx.Tx, campaignID, mapID string, squares []grid.Square) ([]grid.Square, error)
	// DoorsChanged tells the watchers, after the commit, that doors of the map
	// were opened.
	DoorsChanged(ctx context.Context, campaignID, mapID string)
}

// SetTerrain connects the source of the maps' layers. play and maps need each
// other, so cmd/api calls it once maps exists, before the server starts. A
// source that is also a DoorKeeper opens the doors the moves walk into; without
// one the doors stay as they are.
func (s *Service) SetTerrain(t TerrainSource) {
	s.terrain = t
	s.doors = doorKeeperOf(t)
}

// doorKeeperOf is the source as a DoorKeeper, nil when it is not one.
func doorKeeperOf(t TerrainSource) DoorKeeper {
	d, _ := t.(DoorKeeper)
	return d
}

// terrainOf is the terrain of the encounter's map. The grid is the one copied
// into the encounter when it started: layers sized for another grid (the map's
// grid changed meanwhile, which clears them and is refused while a combat runs)
// are not used, and a map the master deleted is open floor.
func (s *Service) terrainOf(ctx context.Context, tx pgx.Tx, campaignID string, enc playdb.Encounter) (grid.Terrain, error) {
	open := grid.Terrain{Grid: grid.Grid{Columns: int(enc.GridColumns), Rows: int(enc.GridRows)}}
	if s.terrain == nil || enc.MapID == nil {
		return open, nil
	}
	t, err := s.terrain.Terrain(ctx, tx, campaignID, *enc.MapID)
	if err != nil {
		if connect.CodeOf(err) == connect.CodeNotFound {
			return open, nil
		}
		return grid.Terrain{}, fmt.Errorf("read the map's terrain: %w", err)
	}
	layersFit := t.Validate() == nil
	if !layersFit || t.Grid != open.Grid {
		return open, nil
	}
	return t, nil
}

// The combatants' sizes and covers as the tables store them
// (combatants_size_valid, combatants_cover_mark_valid), and as the rules and the
// API know them.
var (
	sizeToGrid = map[string]grid.Size{
		"tiny": grid.SizeTiny, "small": grid.SizeSmall, "medium": grid.SizeMedium,
		"large": grid.SizeLarge, "huge": grid.SizeHuge, "gargantuan": grid.SizeGargantuan,
	}
	sizeToProto = map[string]rulesv1.CreatureSize{
		"tiny": rulesv1.CreatureSize_CREATURE_SIZE_TINY, "small": rulesv1.CreatureSize_CREATURE_SIZE_SMALL,
		"medium": rulesv1.CreatureSize_CREATURE_SIZE_MEDIUM, "large": rulesv1.CreatureSize_CREATURE_SIZE_LARGE,
		"huge": rulesv1.CreatureSize_CREATURE_SIZE_HUGE, "gargantuan": rulesv1.CreatureSize_CREATURE_SIZE_GARGANTUAN,
	}
	coverToGrid = map[string]grid.Cover{
		"none": grid.CoverNone, "half": grid.CoverHalf, "three_quarters": grid.CoverThreeQuarters, "total": grid.CoverTotal,
	}
)

// The two sides of a combat (combatants_side_valid).
const (
	sidePartyKey = "party"
	sideEnemyKey = "enemy"
)

// sizeKey is the size a link.Character carries, as the table stores it: a name
// the table does not know, and no name, are medium.
func sizeKey(name string) string {
	if _, ok := sizeToGrid[name]; ok {
		return name
	}
	return "medium"
}

// squareOfCombatant is the combatant's square; callers checked placed.
func squareOfCombatant(c playdb.Combatant) grid.Square {
	return grid.Square{Col: int(*c.GridCol), Row: int(*c.GridRow)}
}

// flies says whether the combatant moves on its fly speed: it has one and it is
// the better of its speeds, the one speedDFt gives. A creature that walks faster
// than it flies (walk 60, fly 10) walks.
func flies(c playdb.Combatant) bool { return c.SpeedFlyFt > 0 && c.SpeedFlyFt >= c.SpeedFt }

// moverOf says how a combatant moves: a creature that moves on its fly speed is
// a flier (difficult terrain costs it nothing). Swimming and climbing are out of
// scope.
func moverOf(c playdb.Combatant) grid.Mover {
	return grid.Mover{Flier: flies(c), Size: sizeToGrid[sizeKey(c.Size)]}
}

// noSpeed are the conditions that leave a creature with no speed (SRD 5.1):
// grappled and restrained set it to 0, and a paralyzed, petrified, stunned or
// unconscious creature cannot move at all. (Prone does not set the speed to 0: standing
// up costs half of it, see combat_standup.go.)
var noSpeed = []string{
	"condition:grappled", "condition:restrained", "condition:paralyzed", "condition:petrified", "condition:stunned", "condition:unconscious",
}

// speedDFt is the combatant's best speed for this turn, in tenths of a foot: its
// walking speed, or its fly speed when it has a better one, twice after the Dash
// action (RN-21).
func speedDFt(c playdb.Combatant) int {
	if slices.ContainsFunc(c.Conditions, func(k string) bool { return slices.Contains(noSpeed, k) }) {
		return 0
	}
	if c.EffectNoMove { // the lethargy of a Velocidade that ended (RN-22)
		return 0
	}
	speed := (int(max(c.SpeedFt, c.SpeedFlyFt)) + int(c.EffectSpeedAddFt)) * 10
	if c.Dashed {
		speed *= 2
	}
	// The speed the effects leave: doubled by Velocidade, halved or 0 by exhaustion (SRD, Conditions).
	if c.EffectSpeedPct > 0 {
		speed = speed * int(c.EffectSpeedPct) / 100
	}
	return speed
}

// movementLeftDFt is the movement the combatant has left this turn, in tenths of
// a foot.
func movementLeftDFt(c playdb.Combatant) int {
	return max(speedDFt(c)-int(c.MovementUsedDft), 0)
}

// movementLeftFt is movementLeftDFt in feet, rounded down: the field the shipped
// web reads.
func movementLeftFt(c playdb.Combatant) int { return movementLeftDFt(c) / 10 }

// placed says whether the combatant has a square on the grid.
func placed(c playdb.Combatant) bool { return c.GridCol != nil && c.GridRow != nil }

// occupantsFor says who stands where, as far as the mover's move goes: every
// other combatant on the map that is not defeated, hostile when its side is the
// other one, with the largest size when several share a square. A player plans
// on the combatants they see (RN-10); with the fog of war (slice 9.4) it will be
// the ones the mover sees, and the real move is then cut short where something
// unseen is in the way. The master sees everyone.
func occupantsFor(cs []playdb.Combatant, mover playdb.Combatant, v combatViewer) grid.OccupantMap {
	out := grid.OccupantMap{}
	for _, o := range cs {
		if o.ID == mover.ID || o.Defeated || !placed(o) || !v.sees(o) {
			continue
		}
		sq := squareOfCombatant(o)
		size := sizeToGrid[sizeKey(o.Size)]
		cur, ok := out[sq]
		if !ok || size > cur.Size {
			cur.Size = size
		}
		cur.Hostile = cur.Hostile || o.Side != mover.Side
		out[sq] = cur
	}
	return out
}

// moveStop is the failed_precondition for a straight move that cannot be made.
func moveStop(mv grid.Move, occ grid.Occupants, to grid.Square) error {
	if mv.By == grid.StopOccupied {
		if _, here := occ.At(to); here {
			return errEncounter(playv1.EncounterBlockedReason_ENCOUNTER_BLOCKED_REASON_SQUARE_OCCUPIED, "another combatant is on that square")
		}
		return errEncounter(playv1.EncounterBlockedReason_ENCOUNTER_BLOCKED_REASON_ENEMY_IN_THE_WAY, "an enemy is in the way")
	}
	return errEncounter(playv1.EncounterBlockedReason_ENCOUNTER_BLOCKED_REASON_MOVE_BLOCKED, "a wall, a column or a squeeze is in the way")
}

// tooFar is the TOO_FAR error: the move costs missing tenths of a foot more than
// the movement left (or the jump is that much longer than its limit).
func tooFar(missingDFt int, edit ...func(*playv1.EncounterBlocked)) error {
	return errEncounter(playv1.EncounterBlockedReason_ENCOUNTER_BLOCKED_REASON_TOO_FAR, "that is beyond your movement",
		append([]func(*playv1.EncounterBlocked){func(b *playv1.EncounterBlocked) {
			b.MissingFt = clamp32((missingDFt+9)/10, 0, math.MaxInt32)
			b.MissingDft = clamp32(missingDFt, 0, math.MaxInt32)
		}}, edit...)...)
}

// moveState is where a combatant stood, and what it had walked, before a move:
// what the undo puts back.
type moveState struct {
	Placed  bool   `json:"placed,omitempty"`
	Col     int32  `json:"col,omitempty"`
	Row     int32  `json:"row,omitempty"`
	UsedDFt int32  `json:"used_dft,omitempty"`
	LastDFt int32  `json:"last_dft,omitempty"`
	Cover   string `json:"cover,omitempty"`
}

func moveStateOf(c playdb.Combatant) *moveState {
	st := &moveState{UsedDFt: c.MovementUsedDft, LastDFt: c.LastMoveDft, Cover: c.CoverMark}
	if placed(c) {
		st.Placed, st.Col, st.Row = true, *c.GridCol, *c.GridRow
	}
	return st
}

// moveRequest is a MoveCombatant call, read and checked.
type moveRequest struct {
	m             authz.Membership
	key           string
	encID, combID string
	col, row      int32
	jump          playv1.JumpKind
	height        int32
	// forced (a teleport, a token put right) and place (the first square of a
	// combatant that has none) are the master's.
	forced, place bool
}

// walking says whether the move is a walk that pays its way: not a jump, and not
// the master's forced one.
func (r moveRequest) walking() bool {
	return !r.forced && r.jump == playv1.JumpKind_JUMP_KIND_UNSPECIFIED
}

// readMoveRequest authorizes the caller and reads what the call asks.
func readMoveRequest(ctx context.Context, msg *playv1.MoveCombatantRequest) (moveRequest, error) {
	m, err := authz.RequireCampaignMember(ctx, msg.GetCampaignId())
	if err != nil {
		return moveRequest{}, err
	}
	key, err := parseKey(msg.GetIdempotencyKey())
	if err != nil {
		return moveRequest{}, err
	}
	encID, err := parseCombatID(msg.GetEncounterId(), "encounter")
	if err != nil {
		return moveRequest{}, err
	}
	combID, err := parseCombatID(msg.GetCombatantId(), "combatant")
	if err != nil {
		return moveRequest{}, err
	}
	r := moveRequest{
		m: m, key: key, encID: encID, combID: combID, col: msg.GetCol(), row: msg.GetRow(),
		jump: msg.GetJump(), height: msg.GetJumpHeightDft(), forced: msg.GetForced(), place: msg.GetPlace(),
	}
	return r, r.validate()
}

// validate refuses what no state of the combat could make right: a jump that is
// not one, and what only the master may ask.
func (r moveRequest) validate() error {
	switch r.jump {
	case playv1.JumpKind_JUMP_KIND_UNSPECIFIED, playv1.JumpKind_JUMP_KIND_LONG:
	case playv1.JumpKind_JUMP_KIND_HIGH:
		if r.height < 1 || r.height > maxJumpDFt {
			return connect.NewError(connect.CodeInvalidArgument, fmt.Errorf("jump_height_dft must be 1 to %d", maxJumpDFt))
		}
	default:
		return connect.NewError(connect.CodeInvalidArgument, errors.New("jump must be a kind of jump"))
	}
	master := viewerOf(r.m).master
	// A forced move (a teleport, a token put right) is the master's, and never
	// provokes.
	if r.forced && !master {
		return connect.NewError(connect.CodePermissionDenied, errors.New("only the master may force a move"))
	}
	if r.place && !master {
		return connect.NewError(connect.CodePermissionDenied, errors.New("only the master may place a combatant"))
	}
	if r.place && r.jump != playv1.JumpKind_JUMP_KIND_UNSPECIFIED {
		return connect.NewError(connect.CodeInvalidArgument, errors.New("a placement is not a jump"))
	}
	return nil
}

// A drag and a crawl each make a foot of path cost this many.
const slowedFoot = 2

// moveAttempt is one run of the move's transaction: what it read, what it worked
// out and what it made, which the publishers read after the commit. The closure
// of the write runs again on a retry, and every run starts from a new one, so
// nothing of a failed attempt stays.
type moveAttempt struct {
	req     moveRequest
	c       *combatTx
	v       combatViewer
	cs      []playdb.Combatant
	target  playdb.Combatant
	terrain grid.Terrain
	onTurn  bool
	// dragged is the creature the target holds and drags, if it does; factor is
	// what a foot of path costs after the drag and the crawl.
	dragged    playdb.Combatant
	dragging   bool
	dragHalves bool
	factor     int
	// col and row are the square asked for, or the one a placement chose.
	col, row int32
	// th fires the traps the move enters.
	th *trapHook
	// from is where the target stood, and what it had walked, before the move.
	from *moveState
	// to is where the move ends; cost is what it spends, length how far it goes
	// and last the running start it leaves, all in tenths of a foot.
	to                 grid.Square
	cost, length, last int
	// the row the move writes.
	newCol, newRow int32
	squareChanged  bool
	coverMark      string
	used           int
	// made is the move's event.
	made actionEvent
	// what the publishers read after the commit.
	moved        playdb.Combatant
	logged       bool     // the move is a line of the combat log
	offered      []string // the reactors the move offered an attack
	markCleared  bool     // the move took the master's cover mark off
	opened       []grid.Square
	draggedMoved *playdb.Combatant // the creature the mover dragged along, where it stands now
	draggedFrom  *grid.Square      // and where it stood
	draggedID    string            // the creature dragged along: it provokes nothing against its grappler
}

// playerPlan is what a player plans and walks on: the terrain they know, the
// creatures they see and the creatures there are.
type playerPlan struct {
	terrain grid.Terrain
	seen    grid.OccupantMap
	all     grid.OccupantMap
	mover   grid.Mover
	origin  grid.Square
	leftDFt int
}

// MoveCombatant implements playv1connect.CombatServiceHandler.
func (s *Service) MoveCombatant(
	ctx context.Context,
	req *connect.Request[playv1.MoveCombatantRequest],
) (*connect.Response[playv1.MoveCombatantResponse], error) {
	mv, err := readMoveRequest(ctx, req.Msg)
	if err != nil {
		return nil, err
	}
	var a moveAttempt
	res, err := s.write(ctx, combatWrite{m: mv.m, key: mv.key, hash: idem.Hash(req.Msg), kind: eventCombatantMoved, altKind: eventTrapTriggered, encounterID: mv.encID}, func(c *combatTx) (any, error) {
		a = moveAttempt{req: mv, c: c, factor: 1}
		return s.moveInTx(ctx, &a)
	})
	if err != nil {
		return nil, s.dbError(ctx, "move a combatant", err)
	}
	out, err := s.finish(ctx, mv.m, res, func(ctx context.Context, d *encounterData) { s.publishMove(ctx, mv.m.CampaignID, d, &a) })
	if err != nil {
		return nil, err
	}
	ev, err := resultEvent(res, a.made)
	if err != nil {
		return nil, s.dbError(ctx, "read the move", err)
	}
	return connect.NewResponse(&playv1.MoveCombatantResponse{Encounter: out, StoppedEarly: ev.StoppedEarly, Provoked: ev.MoveID != "", LockedDoor: ev.LockedDoor}), nil
}

// moveInTx is the move's transaction: it finds who moves and where (resolve),
// works out the path (plan), writes the move and what it brings with it (write)
// and fills the event the log keeps (describe).
func (s *Service) moveInTx(ctx context.Context, a *moveAttempt) (any, error) {
	if err := s.resolveMove(ctx, a); err != nil {
		return nil, err
	}
	if err := s.planMove(ctx, a); err != nil {
		return nil, err
	}
	if err := s.writeMove(ctx, a); err != nil {
		return nil, err
	}
	a.describeMove()
	return a.th.finish(ctx, a.c, a.made, a.cs, a.moved)
}

// resolveMove loads the combatants, picks the square and the terrain, and checks
// that the caller may move the combatant now.
func (s *Service) resolveMove(ctx context.Context, a *moveAttempt) error {
	if err := s.loadMover(ctx, a); err != nil {
		return err
	}
	if err := s.chooseSquare(ctx, a); err != nil {
		return err
	}
	if err := s.readTerrain(ctx, a); err != nil {
		return err
	}
	return s.mayMove(ctx, a)
}

// loadMover finds the combatant in the combat as the caller sees it, and what a
// foot of its path costs: a grappler drags the creature it holds, at half speed
// (combat_drag.go).
func (s *Service) loadMover(ctx context.Context, a *moveAttempt) error {
	if err := notEnded(a.c.enc); err != nil {
		return err
	}
	if isTheatre(a.c.enc) {
		return errNeedsAMap() // no squares: a move, a jump and a placement are the master's word (SpendMovement)
	}
	cs, err := a.c.q.ListCombatants(ctx, a.c.enc.ID)
	if err != nil {
		return fmt.Errorf("list the combatants: %w", err)
	}
	a.cs = cs
	a.v = a.c.viewer(a.req.m, cs) // the fog: an NPC the player does not see is not found
	if a.target, err = findCombatant(cs, a.req.combID, a.v); err != nil {
		return err
	}
	if err := a.v.mayAct(a.target); err != nil {
		return err
	}
	if a.dragged, a.dragging, err = s.draggedBy(ctx, a.c.q, cs, a.target); err != nil {
		return err
	}
	a.setFactor()
	return nil
}

// setFactor works out what a foot of path costs.
func (a *moveAttempt) setFactor() {
	if a.dragging && a.req.walking() && combat.DragHalvesSpeed(a.target.Size, a.dragged.Size) {
		a.factor, a.dragHalves = slowedFoot, true
	}
	// Crawling (SRD 5.1, "Being Prone"): every foot of movement costs 1 extra foot, so a
	// prone creature pays twice (a drag halves the speed on top of it). See combat_standup.go.
	if a.req.walking() && crawls(a.target) {
		a.factor *= slowedFoot
	}
}

// chooseSquare takes the square the request names, or picks one for a placement.
func (s *Service) chooseSquare(ctx context.Context, a *moveAttempt) error {
	a.col, a.row = a.req.col, a.req.row // the closure runs again on a retry: a placement picks its square afresh
	if err := s.pickSquare(ctx, a); err != nil {
		return err
	}
	if a.req.jump != playv1.JumpKind_JUMP_KIND_UNSPECIFIED && !placed(a.target) {
		return errEncounter(playv1.EncounterBlockedReason_ENCOUNTER_BLOCKED_REASON_NOT_PLACED, "a jump starts from a square of the map")
	}
	return nil
}

// pickSquare is the placement's choice, or the check that the square is on the grid.
func (s *Service) pickSquare(ctx context.Context, a *moveAttempt) error {
	switch {
	case a.req.place && placed(a.target):
		return errAlreadyPlaced
	case a.req.place:
		p, err := s.placementFor(ctx, a.c, a.cs)
		if err != nil {
			return err
		}
		sq, ok := p.next()
		if !ok {
			return errNoRoom()
		}
		a.col, a.row = clamp32(sq.Col, 0, math.MaxInt32), clamp32(sq.Row, 0, math.MaxInt32)
	case a.req.jump != playv1.JumpKind_JUMP_KIND_HIGH && !inGrid(a.c.enc, a.col, a.row):
		return connect.NewError(connect.CodeInvalidArgument, errors.New("col and row must be a square of the grid"))
	}
	return nil
}

// readTerrain reads the map's layers and refuses the master a wall to stand in.
func (s *Service) readTerrain(ctx context.Context, a *moveAttempt) error {
	terrain, err := s.terrainOf(ctx, a.c.tx, a.req.m.CampaignID, a.c.enc)
	if err != nil {
		return err
	}
	a.terrain = terrain
	if a.v.master && a.req.jump != playv1.JumpKind_JUMP_KIND_HIGH && terrain.Walls.Has(grid.Square{Col: int(a.col), Row: int(a.row)}) {
		return errWall() // nobody stands in a wall, and a creature in one is seen by no player
	}
	return nil
}

// mayMove checks that a player may move the combatant now. The master moves
// anyone at any time.
func (s *Service) mayMove(ctx context.Context, a *moveAttempt) error {
	a.onTurn = actsNow(a.c.enc, a.target)
	if a.v.master {
		return nil
	}
	// RN-21: a player walks only on their own turn.
	switch {
	case a.c.enc.Status != statusActive:
		return errEncounter(playv1.EncounterBlockedReason_ENCOUNTER_BLOCKED_REASON_NOT_ACTIVE, "the combat is not running")
	case !a.onTurn: // not in the turn that is running, or its part ended
		return errEncounter(playv1.EncounterBlockedReason_ENCOUNTER_BLOCKED_REASON_NOT_YOUR_TURN, "it is not your turn")
	case !placed(a.target):
		return errEncounter(playv1.EncounterBlockedReason_ENCOUNTER_BLOCKED_REASON_NOT_PLACED, "you are not on the map yet; ask the master")
	}
	if err := s.mustNotBeDown(ctx, a.c.tx, a.req.m.CampaignID, a.target); err != nil {
		return err
	}
	// A surprised creature does not move on its first turn (SRD 5.1, "Surprise").
	return s.mustNotBeSurprised(ctx, a.c, a.target)
}

// planMove sets the move up, checks that nothing holds the turn and works out
// where the move ends and what it costs.
func (s *Service) planMove(ctx context.Context, a *moveAttempt) error {
	if err := s.startMove(ctx, a); err != nil {
		return err
	}
	if !a.v.master || a.onTurn { // the turn's move waits for the master's answer; his own moves of a token off turn do not
		if err := s.mustNotHold(ctx, a.c); err != nil {
			return err
		}
	}
	if a.v.master {
		return a.planMasterWalk()
	}
	if err := s.mustNotWait(ctx, a.c, a.target); err != nil {
		return err
	}
	return s.planPlayerMove(ctx, a)
}

// startMove opens the move's event and the trap hook, and takes the move to be
// free and as long as the line to the square asked.
func (s *Service) startMove(ctx context.Context, a *moveAttempt) error {
	th, err := s.newTrapHook(ctx, a.c.tx, a.req.m.CampaignID, a.c.enc, a.target)
	if err != nil {
		return err
	}
	a.th = th
	a.from = moveStateOf(a.target)
	a.made = actionEvent{Round: a.c.enc.Round, Secret: a.target.Hidden, Actor: a.target.ID, OnTurn: a.onTurn, From: a.from}
	a.to = grid.Square{Col: int(a.col), Row: int(a.row)}
	if a.req.jump == playv1.JumpKind_JUMP_KIND_HIGH {
		a.to = squareOfCombatant(a.target) // a high jump moves nobody
	}
	a.cost, a.length, a.last = 0, 0, int(a.target.LastMoveDft) // the master's move leaves the running start alone
	if placed(a.target) && a.req.jump != playv1.JumpKind_JUMP_KIND_HIGH {
		a.length = grid.LengthDFt(squareOfCombatant(a.target), a.to)
	}
	return nil
}

// planPlayerMove works out the move of a player. They plan on what they see
// (RN-10); the real move runs against everyone, and stops before a creature they
// do not see, without saying what it was (D1): the answer only says it was cut
// short.
func (s *Service) planPlayerMove(ctx context.Context, a *moveAttempt) error {
	known, err := a.c.sight.knownTerrain(ctx, a.c.tx, a.v) // the mover's is cached by sightForWrite; any other is read in this transaction
	if err != nil {
		return err
	}
	p := playerPlan{
		terrain: planOn(a.v, a.terrain, known), // the walls and rubble the player knows; the rest is floor to them
		seen:    occupantsFor(a.cs, a.target, a.v),
		all:     occupantsFor(a.cs, a.target, combatViewer{master: true}),
		mover:   moverOf(a.target),
		origin:  squareOfCombatant(a.target),
		leftDFt: movementLeftDFt(a.target) / a.factor, // the path that fits: a drag costs twice its length
	}
	switch a.req.jump {
	case playv1.JumpKind_JUMP_KIND_LONG:
		return a.planLongJump(p)
	case playv1.JumpKind_JUMP_KIND_HIGH:
		return a.planHighJump(p)
	}
	return a.planWalk(p)
}

// planWalk is a player's straight move: refused when a wall or a creature they
// know of is in the way or the path costs more than they have, and cut short by
// what they do not know of.
func (a *moveAttempt) planWalk(p playerPlan) error {
	mv := p.terrain.Move(p.origin, a.to, p.seen, p.mover)
	if mv.Blocked {
		return moveStop(mv, p.seen, a.to)
	}
	if mv.CostDFt > p.leftDFt {
		return tooFar(mv.CostDFt - p.leftDFt)
	}
	st := a.terrain.MoveUntil(p.origin, a.to, p.all, p.mover, a.th.stops(), p.leftDFt)
	a.made.StoppedEarly = st.Reason != grid.StopNone && (st.Reason != grid.StopMarked || st.Reached != a.to) // a trap's area is not "in the way" when it is where the mover went
	if err := a.noteLock(p.terrain, st); err != nil {
		return err
	}
	a.opened = st.Opens
	a.th.marked(st.Marked, st.MarkedAt)
	a.to, a.cost = st.Reached, st.CostDFt
	a.length = grid.LengthDFt(p.origin, a.to)
	a.last = runAfterWalk(a.last, a.length, p.mover.Flier)
	return nil
}

// noteLock tells a locked door the player knows of from a creature in the way.
func (a *moveAttempt) noteLock(plan grid.Terrain, st grid.Stopped) error {
	if st.Reason != grid.StopLocked || !doorKnown(plan, st.LockedAt) {
		return nil
	}
	// The player learns the door is locked by trying it (RN-26). A door they
	// do not see (the fog) is "something in the way" like any other.
	a.made.StoppedEarly, a.made.LockedDoor = false, true
	if len(st.Entered) == 0 {
		return errEncounter(playv1.EncounterBlockedReason_ENCOUNTER_BLOCKED_REASON_DOOR_LOCKED, "the door is locked")
	}
	return nil
}

// runAfterWalk is the running start after a walk of length: consecutive moves on
// foot add up to it (10 ft just before); a flier's move is no run.
func runAfterWalk(last, length int, flier bool) int {
	switch {
	case length == 0:
		return last
	case flier:
		return 0
	}
	return last + length
}

// planLongJump is a player's long jump: within the limit of the jump, over no
// wall, onto no creature and paid for by the movement left.
func (a *moveAttempt) planLongJump(p playerPlan) error {
	if a.length == 0 {
		return connect.NewError(connect.CodeInvalidArgument, errors.New("a jump needs another square"))
	}
	running := combat.HasRunningStart(int(a.target.LastMoveDft))
	limit := longJumpDFt(a.target, running)
	if a.length > limit {
		return tooFar(a.length-limit, jumpLimit(limit, running))
	}
	mv := p.terrain.Jump(p.origin, a.to, p.seen, p.mover)
	if mv.Blocked {
		return moveStop(mv, p.seen, a.to)
	}
	if mv.CostDFt > p.leftDFt {
		return tooFar(mv.CostDFt - p.leftDFt)
	}
	if a.terrain.Jump(p.origin, a.to, p.all, p.mover).Blocked { // a creature it does not see holds the landing
		a.made.StoppedEarly, a.to, a.length = true, p.origin, 0
		return nil
	}
	a.cost, a.last = mv.CostDFt, 0
	a.th.landed(a.to) // a jump fires a trap only where it lands (D3, D5)
	return nil
}

// planHighJump is a player's high jump: nobody moves, and the height is paid in
// movement.
func (a *moveAttempt) planHighJump(p playerPlan) error {
	height := int(a.req.height)
	running := combat.HasRunningStart(int(a.target.LastMoveDft))
	limit := highJumpDFt(a.target, running)
	if height > limit {
		return tooFar(height-limit, jumpLimit(limit, running))
	}
	if height > p.leftDFt {
		return tooFar(height - p.leftDFt)
	}
	a.cost, a.last = height, 0
	return nil
}

// planMasterWalk is the master's move: free, and no wall stops it, but the doors
// do what they do for everyone (RN-26): it opens the closed ones on its line, and
// a locked, barred or secret one stops it before. The master unlocks by painting.
func (a *moveAttempt) planMasterWalk() error {
	if !a.req.walking() || !placed(a.target) || a.terrain.Doors.Count() == 0 {
		return nil
	}
	origin := squareOfCombatant(a.target)
	st := grid.Terrain{Grid: a.terrain.Grid, Doors: a.terrain.Doors}.MoveUntil(origin, a.to, nil, moverOf(a.target), nil, math.MaxInt)
	if st.Reason == grid.StopLocked && len(st.Entered) == 0 {
		return errEncounter(playv1.EncounterBlockedReason_ENCOUNTER_BLOCKED_REASON_DOOR_LOCKED, "the door is locked")
	}
	a.made.StoppedEarly, a.made.LockedDoor = st.Reason != grid.StopNone, st.Reason == grid.StopLocked
	a.to, a.opened = st.Reached, st.Opens
	a.length = grid.LengthDFt(origin, a.to)
	return nil
}

// writeMove writes the move, the creature it drags, the opportunity offers it
// makes and the doors it opens, in the order the undo takes them back.
func (s *Service) writeMove(ctx context.Context, a *moveAttempt) error {
	a.settleMove()
	if err := s.saveMove(ctx, a); err != nil {
		return err
	}
	if err := s.dragAlong(ctx, a); err != nil {
		return err
	}
	if err := s.offerAttacks(ctx, a); err != nil {
		return err
	}
	a.setMoved()
	a.c.characterID = &a.target.CharacterID
	var err error
	a.opened, err = s.openDoors(ctx, a.c, a.target, a.opened, a.from)
	return err
}

// settleMove works out the row the move leaves: the square, the cover mark and
// the movement walked.
func (a *moveAttempt) settleMove() {
	a.newCol, a.newRow = clamp32(a.to.Col, 0, math.MaxInt32), clamp32(a.to.Row, 0, math.MaxInt32)
	// The master's cover mark clears when the combatant changes square, not
	// when nobody moved (a high jump, a move to where it stands).
	a.squareChanged = !placed(a.target) || a.to != squareOfCombatant(a.target)
	a.coverMark = a.target.CoverMark
	if a.squareChanged {
		a.markCleared = a.coverMark != "" && a.coverMark != "none"
		a.coverMark = "none"
	}
	a.used = int(a.target.MovementUsedDft) + a.cost*a.factor
}

// saveMove writes the combatant's row and touches the encounter.
func (s *Service) saveMove(ctx context.Context, a *moveAttempt) error {
	if err := a.c.q.SetCombatantMove(ctx, playdb.SetCombatantMoveParams{
		ID: a.target.ID, GridCol: &a.newCol, GridRow: &a.newRow,
		MovementUsedFt: clamp32(a.used/10, 0, math.MaxInt32), MovementUsedDft: clamp32(a.used, 0, math.MaxInt32), LastMoveDft: clamp32(a.last, 0, math.MaxInt32), CoverMark: a.coverMark,
	}); err != nil {
		return fmt.Errorf("move the combatant: %w", err)
	}
	if err := touchEncounter(ctx, a.c); err != nil {
		return err
	}
	return nil
}

// dragAlong moves the creature the mover holds to the square behind its path.
func (s *Service) dragAlong(ctx context.Context, a *moveAttempt) error {
	if !a.dragging || !a.req.walking() || !a.squareChanged || !placed(a.target) {
		return nil
	}
	from := squareOfCombatant(a.dragged)
	moved, err := s.dragTo(ctx, a.c, a.cs, a.target, a.dragged, dragSquare(a.terrain, squareOfCombatant(a.target), a.to, moverOf(a.target)))
	if err != nil {
		return err
	}
	a.draggedMoved, a.draggedFrom = &moved, &from
	a.draggedID = a.dragged.ID
	a.made.Dragged, a.made.DraggedFrom = a.dragged.ID, moveStateOf(a.dragged)
	return nil
}

// offerAttacks makes the opportunity attacks (D1b): a move on foot or a long jump
// (it spends movement), on the mover's turn, that leaves an enemy's reach lands,
// and the enemy is offered an attack. A high jump moves nobody; a placement out of
// turn, a forced move and a move that stays put offer nothing.
func (s *Service) offerAttacks(ctx context.Context, a *moveAttempt) error {
	if a.req.jump == playv1.JumpKind_JUMP_KIND_HIGH || a.req.forced || !a.onTurn || !placed(a.target) || !a.squareChanged {
		return nil
	}
	moveID, offered, err := s.offerOpportunities(ctx, a.c, a.cs, a.target, squareOfCombatant(a.target), a.to, a.req.jump == playv1.JumpKind_JUMP_KIND_LONG, a.draggedID)
	if err != nil {
		return err
	}
	a.offered, a.made.MoveID = offered, moveID
	return nil
}

// setMoved is the mover as it stands after the move.
func (a *moveAttempt) setMoved() {
	a.moved = a.target
	a.moved.GridCol, a.moved.GridRow, a.moved.MovementUsedFt, a.moved.MovementUsedDft = &a.newCol, &a.newRow, clamp32(a.used/10, 0, math.MaxInt32), clamp32(a.used, 0, math.MaxInt32)
	a.moved.LastMoveDft, a.moved.CoverMark = clamp32(a.last, 0, math.MaxInt32), a.coverMark
}

// describeMove fills the event the log keeps. The log tells how far a combatant
// went on its turn: the line between where it was and where it goes, whoever
// moves it.
func (a *moveAttempt) describeMove() {
	made := &a.made
	made.Col, made.Row = a.newCol, a.newRow
	made.CostDFt, made.CostFt = clamp32(a.cost, 0, math.MaxInt32), clamp32(a.cost/10, 0, math.MaxInt32)
	made.DistanceDFt, made.DistanceFt = clamp32(a.length, 0, math.MaxInt32), clamp32(a.length/10, 0, math.MaxInt32)
	if a.req.jump == playv1.JumpKind_JUMP_KIND_UNSPECIFIED && a.cost > 0 { // what the walk spent: the path's cost, doubled by a drag and by a crawl
		made.SpentDFt, made.Dragging, made.Crawling = clamp32(a.cost*a.factor, 0, math.MaxInt32), a.dragHalves, crawls(a.target) && !a.req.forced
	}
	a.describeJump()
	a.logged = a.onTurn && (a.length > 0 || made.Jump == jumpHigh)
	if a.v.master && a.squareChanged {
		a.th.landed(a.to) // the master's move is not a walk: only where it ends counts
	}
}

// describeJump names the jump in the event.
func (a *moveAttempt) describeJump() {
	switch a.req.jump {
	case playv1.JumpKind_JUMP_KIND_LONG:
		a.made.Jump, a.made.JumpRunning = jumpLong, combat.HasRunningStart(int(a.target.LastMoveDft))
		// SRD: landing in difficult terrain asks for a DC 10 Acrobatics check or
		// the jumper falls prone. The app rolls nothing: the master's log says it.
		a.made.LandingDifficult = !moverOf(a.target).Flier && a.terrain.Difficult.Has(a.to)
	case playv1.JumpKind_JUMP_KIND_HIGH:
		a.made.Jump, a.made.HeightDFt = jumpHigh, a.req.height
	}
}

// publishMove tells the streams about the move, after the commit.
func (s *Service) publishMove(ctx context.Context, campaignID string, d *encounterData, a *moveAttempt) {
	s.publishCombatantMoved(ctx, campaignID, d.enc, a.moved, squareOfState(a.made.From))
	if a.draggedMoved != nil { // the creature it dragged moved too
		s.publishCombatantMoved(ctx, campaignID, d.enc, *a.draggedMoved, a.draggedFrom)
		s.positionChanged(ctx, campaignID, d.enc, *a.draggedMoved, a.draggedFrom)
	}
	if a.made.MoveID != "" { // the offers are in the combat, not in the move's hint
		s.publishOffersMade(campaignID, d, a.moved, a.offered)
	}
	if a.markCleared { // the mark is on the combatant, not in the move's hint: who saw it reads again
		s.publishCoverCleared(ctx, campaignID, d.enc, a.moved)
	}
	s.positionChanged(ctx, campaignID, d.enc, a.moved, squareOfState(a.made.From)) // the fog remembers what it showed
	s.publishDoorsOpened(ctx, campaignID, d.enc, a)
	a.th.after(ctx, campaignID, d.enc)
	s.noticeAfterMove(ctx, campaignID, d.enc, a.moved) // a trap near where the mover ended may be noticed (MR-035)
}

// publishCoverCleared tells who saw the combatant that its cover mark is gone.
func (s *Service) publishCoverCleared(ctx context.Context, campaignID string, enc playdb.Encounter, moved playdb.Combatant) {
	if moved.Hidden {
		s.hub.Publish(campaignID, live.Event{Audience: live.Audience{Master: true}, Message: encounterChangedMessage(enc)})
		return
	}
	s.publishEncounterChanged(ctx, campaignID, enc)
}

// publishDoorsOpened tells the watchers of the map that its layers changed, and
// of the log that the move, or the doors it opened, are a line of it.
func (s *Service) publishDoorsOpened(ctx context.Context, campaignID string, enc playdb.Encounter, a *moveAttempt) {
	if len(a.opened) > 0 && s.doors != nil && enc.MapID != nil { // the map's layers changed, and what the players see with them
		s.doors.DoorsChanged(ctx, campaignID, *enc.MapID)
	}
	if a.logged || len(a.opened) > 0 {
		s.publishLogChanged(ctx, campaignID, enc.ID, !a.moved.Hidden)
	}
}

// doorKnown says whether the terrain a player plans on has a door on the square:
// they see it, or remember it (a door in the dark is not theirs to know).
func doorKnown(plan grid.Terrain, sq grid.Square) bool {
	return plan.Doors.At(sq) != grid.DoorNone
}

// openDoors opens, inside the move's transaction, the closed doors the mover
// walked into (RN-26): the doors layer changes through the maps module with the
// move's transaction, and a door_opened event is written for each door that
// really opened, before the move's own event. The event is the door's: the
// master's undo of the move puts the mover back but leaves the door open, as a
// door opened stays opened. It returns the doors it opened.
func (s *Service) openDoors(ctx context.Context, c *combatTx, mover playdb.Combatant, squares []grid.Square, from *moveState) ([]grid.Square, error) {
	if len(squares) == 0 || s.doors == nil || c.enc.MapID == nil {
		return nil, nil
	}
	opened, err := s.doors.OpenDoors(ctx, c.tx, c.session.CampaignID, *c.enc.MapID, squares)
	if err != nil {
		return nil, fmt.Errorf("open the doors on the way: %w", err)
	}
	for _, sq := range opened {
		if err := insertEvent(ctx, c, eventDoorOpened, &c.actorUserID, nil, actionEvent{
			Round: c.enc.Round, Secret: mover.Hidden, Actor: mover.ID,
			Col: clamp32(sq.Col, 0, math.MaxInt32), Row: clamp32(sq.Row, 0, math.MaxInt32), From: from,
		}); err != nil {
			return nil, err
		}
	}
	return opened, nil
}

// The two jumps as a move event keeps them.
const (
	jumpLong = "long"
	jumpHigh = "high"
)

// maxJumpDFt is the largest height a request may ask for: 100 ft, far above any
// sheet's limit, so a typo cannot overflow the arithmetic.
const maxJumpDFt = 1000

// longJumpDFt and highJumpDFt are the combatant's limits for a jump, in tenths
// of a foot, with or without a running start. They were copied from the sheet
// when the combatant joined (the standing jump is half of the running one, without
// the Thief's bonus), doubled for the turn the monk used Step of the Wind.
func longJumpDFt(c playdb.Combatant, running bool) int {
	return combat.FromRunning(int(c.JumpLongDft), int(c.JumpLongBonusDft), int(c.JumpHighDft)).WithStepOfTheWind(c.JumpDoubled).Long(running)
}

func highJumpDFt(c playdb.Combatant, running bool) int {
	return combat.FromRunning(int(c.JumpLongDft), int(c.JumpLongBonusDft), int(c.JumpHighDft)).WithStepOfTheWind(c.JumpDoubled).High(running)
}

// jumpLimit fills the limit that applied in a TOO_FAR of a jump.
func jumpLimit(limit int, running bool) func(*playv1.EncounterBlocked) {
	return func(b *playv1.EncounterBlocked) {
		b.JumpLimitDft, b.JumpRunningStart = clamp32(limit, 0, math.MaxInt32), running
	}
}

// breakRun ends the running start of a combatant that acts (an action, an attack,
// a spell): the moves on foot add up only when nothing comes between them. It
// returns the run it broke, for the event's undo.
func breakRun(ctx context.Context, c *combatTx, who playdb.Combatant) (int32, error) {
	if who.LastMoveDft == 0 {
		return 0, nil
	}
	if err := c.q.SetCombatantRun(ctx, playdb.SetCombatantRunParams{ID: who.ID, LastMoveDft: 0}); err != nil {
		return 0, fmt.Errorf("break the running start: %w", err)
	}
	return who.LastMoveDft, nil
}

// markDashed records the Dash action: the combatant's speed counts twice for
// the rest of the turn (RN-21). TakeAction calls it, inside its own
// transaction, when the action is spent; it is here because it is movement's
// rule.
func markDashed(ctx context.Context, q *playdb.Queries, combatantID string) error {
	if err := q.MarkCombatantDashed(ctx, combatantID); err != nil {
		return fmt.Errorf("mark the dash: %w", err)
	}
	return nil
}

// GetMoveOptions implements playv1connect.CombatServiceHandler.
func (s *Service) GetMoveOptions(
	ctx context.Context,
	req *connect.Request[playv1.GetMoveOptionsRequest],
) (*connect.Response[playv1.GetMoveOptionsResponse], error) {
	m, err := authz.RequireCampaignMember(ctx, req.Msg.GetCampaignId())
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
	_, d, err := s.readEncounter(ctx, m.CampaignID, encID)
	if err != nil {
		return nil, err
	}
	v, sight, err := s.viewerWith(ctx, m, d.enc, d.cs)
	if err != nil {
		return nil, s.dbError(ctx, "work out what the player sees", err)
	}
	who, sight, err := s.moverToPlan(ctx, m, d, v, sight, combID)
	if err != nil {
		return nil, err
	}
	if isTheatre(d.enc) {
		// Without a map there is nowhere to go: a valid, empty answer, with what
		// SpendMovement can still spend.
		return connect.NewResponse(&playv1.GetMoveOptionsResponse{MovementLeftDft: clamp32(movementLeftDFt(who), 0, math.MaxInt32), Flier: flies(who)}), nil
	}
	jump := req.Msg.GetJump()
	out, err := s.moveOptionsOn(ctx, m, d, v, sight, who, jump)
	if err != nil {
		return nil, err
	}
	if err := s.markProvokes(ctx, m, d.enc, d.cs, who, v, sight, out); err != nil {
		return nil, s.dbError(ctx, "work out the opportunity attacks", err)
	}
	if !v.master {
		s.markKnownTraps(ctx, m.CampaignID, d.enc, who, out, jump == playv1.JumpKind_JUMP_KIND_LONG) // the warning before stepping into a trap the character knows
	}
	return connect.NewResponse(out), nil
}

// moverToPlan finds the combatant whose moves are asked, which the caller may act for, and
// checks that the combat is one it can move in now: not ended and, for a player, running, on
// the combatant's turn and with the character on its feet. It returns the fog of the master's
// view too.
func (s *Service) moverToPlan(ctx context.Context, m authz.Membership, d *encounterData, v combatViewer, sight *fogSight, combID string) (playdb.Combatant, *fogSight, error) {
	enc := d.enc
	who, err := findCombatant(d.cs, combID, v)
	if err != nil {
		return who, sight, err
	}
	if err := v.mayAct(who); err != nil {
		return who, sight, err
	}
	if sight == nil && v.master { // the master's warning follows the same fog rule as the real offers
		if sight, err = s.fogSightOf(ctx, m.CampaignID, enc); err != nil {
			return who, sight, s.dbError(ctx, "work out what the players see", err)
		}
	}
	if err := notEnded(enc); err != nil {
		return who, sight, err
	}
	if v.master {
		return who, sight, nil
	}
	switch {
	case enc.Status != statusActive:
		return who, sight, errEncounter(playv1.EncounterBlockedReason_ENCOUNTER_BLOCKED_REASON_NOT_ACTIVE, "the combat is not running")
	case !actsNow(enc, who):
		return who, sight, errEncounter(playv1.EncounterBlockedReason_ENCOUNTER_BLOCKED_REASON_NOT_YOUR_TURN, "it is not your turn")
	}
	return who, sight, s.mustNotBeDown(ctx, nil, m.CampaignID, who)
}

// moveOptionsOn works out the squares the combatant on the map can go to, or land on with
// a long jump.
func (s *Service) moveOptionsOn(ctx context.Context, m authz.Membership, d *encounterData, v combatViewer, sight *fogSight, who playdb.Combatant, jump playv1.JumpKind) (*playv1.GetMoveOptionsResponse, error) {
	if !placed(who) {
		return nil, errEncounter(playv1.EncounterBlockedReason_ENCOUNTER_BLOCKED_REASON_NOT_PLACED, "the combatant is not on the map yet")
	}
	terrain, err := s.terrainOf(ctx, nil, m.CampaignID, d.enc)
	if err != nil {
		return nil, s.dbError(ctx, "read the terrain", err)
	}
	// A player's reach is worked out on what they know: the squares they do not see
	// are floor, so the reach may offer one that is really a wall, and the move is
	// then cut short (D1). No refusal names a wall or a creature they do not see.
	known, err := sight.knownTerrain(ctx, nil, v)
	if err != nil {
		return nil, s.dbError(ctx, "read the terrain the player knows", err)
	}
	switch jump {
	case playv1.JumpKind_JUMP_KIND_UNSPECIFIED:
	case playv1.JumpKind_JUMP_KIND_LONG:
		return jumpOptions(planOn(v, terrain, known), who, occupantsFor(d.cs, who, v)), nil
	default:
		return nil, connect.NewError(connect.CodeInvalidArgument, errors.New("jump must be unset or LONG: a high jump moves nobody"))
	}
	out, err := s.moveOptionsWithDrag(ctx, planOn(v, terrain, known), d.cs, who, v)
	if err != nil {
		return nil, s.dbError(ctx, "work out the drag", err)
	}
	return out, nil
}

// jumpOptions works out where the combatant can land with a long jump (SRD 5.1,
// "Special Types of Movement": each foot cleared costs a foot of movement; the
// distance is the Strength score in feet with a running start, half of it without):
// the squares within the jump's limit that nothing in the way stops (a wall, a door
// that is not open, a creature holding the landing) and that the movement left
// pays for. The running start is the one the combatant really has (what it walked
// just before), as MoveCombatant uses it: the preview never offers a square the move
// would refuse. Its cost is the length of the line.
func jumpOptions(t grid.Terrain, who playdb.Combatant, occ grid.OccupantMap) *playv1.GetMoveOptionsResponse {
	left, from, mover := movementLeftDFt(who), squareOfCombatant(who), moverOf(who)
	out := &playv1.GetMoveOptionsResponse{MovementLeftDft: clamp32(left, 0, math.MaxInt32), Flier: mover.Flier}
	limit := min(longJumpDFt(who, combat.HasRunningStart(int(who.LastMoveDft))), left)
	if limit <= 0 {
		return out
	}
	radius := limit/grid.DFtPerSquare + 1
	for row := max(from.Row-radius, 0); row <= min(from.Row+radius, t.Grid.Rows-1); row++ {
		for col := max(from.Col-radius, 0); col <= min(from.Col+radius, t.Grid.Columns-1); col++ {
			sq := grid.Square{Col: col, Row: row}
			if sq == from || grid.LengthDFt(from, sq) > limit {
				continue
			}
			if mv := t.Jump(from, sq, occ, mover); !mv.Blocked && mv.CostDFt <= left {
				out.Reachable = append(out.Reachable, &playv1.ReachableSquare{Col: clamp32(col, 0, math.MaxInt32), Row: clamp32(row, 0, math.MaxInt32), CostDft: clamp32(mv.CostDFt, 0, math.MaxInt32)})
			}
		}
	}
	return out
}

// moveOptions works out where the combatant can go in one straight move: the
// squares it reaches with what is left of its movement and the squares inside
// that circle that it cannot go to, with the reason. occ is who the mover plans
// around (with the fog of war: the ones it sees).
func moveOptions(t grid.Terrain, who playdb.Combatant, occ grid.OccupantMap) *playv1.GetMoveOptionsResponse {
	left, from, mover := movementLeftDFt(who), squareOfCombatant(who), moverOf(who)
	out := &playv1.GetMoveOptionsResponse{MovementLeftDft: clamp32(left, 0, math.MaxInt32), Flier: mover.Flier}
	reach := t.Reach(from, left, occ, mover)
	reachable := make(map[grid.Square]bool, len(reach))
	for _, r := range reach {
		reachable[r.Square] = true
		out.Reachable = append(out.Reachable, &playv1.ReachableSquare{Col: clamp32(r.Square.Col, 0, math.MaxInt32), Row: clamp32(r.Square.Row, 0, math.MaxInt32), CostDft: clamp32(r.CostDFt, 0, math.MaxInt32)})
	}
	if left <= 0 {
		return out
	}
	out.Refused = refusedSquares(t, from, left, reachable, occ, mover)
	return out
}

// refusedSquares lists the squares inside the circle of the movement left that are not
// reachable, and why not. They come in reading order too, like the reachable ones.
func refusedSquares(t grid.Terrain, from grid.Square, left int, reachable map[grid.Square]bool, occ grid.OccupantMap, mover grid.Mover) []*playv1.RefusedSquare {
	var refused []*playv1.RefusedSquare
	radius := left/grid.DFtPerSquare + 1
	for row := max(from.Row-radius, 0); row <= min(from.Row+radius, t.Grid.Rows-1); row++ {
		for col := max(from.Col-radius, 0); col <= min(from.Col+radius, t.Grid.Columns-1); col++ {
			sq := grid.Square{Col: col, Row: row}
			if sq == from || reachable[sq] || grid.LengthDFt(from, sq) > left {
				continue
			}
			reason := moveRefusalOf(t.Move(from, sq, occ, mover), occ, sq)
			refused = append(refused, &playv1.RefusedSquare{Col: clamp32(col, 0, math.MaxInt32), Row: clamp32(row, 0, math.MaxInt32), Reason: reason})
		}
	}
	return refused
}

// moveRefusalOf is why a move to the square cannot be made: too costly unless something
// stops it, and then an enemy, an occupant or a wall.
func moveRefusalOf(mv grid.Move, occ grid.OccupantMap, sq grid.Square) playv1.MoveRefusal {
	switch {
	case mv.Blocked && mv.By == grid.StopOccupied:
		if _, here := occ.At(sq); here {
			return playv1.MoveRefusal_MOVE_REFUSAL_OCCUPIED
		}
		return playv1.MoveRefusal_MOVE_REFUSAL_ENEMY
	case mv.Blocked:
		return playv1.MoveRefusal_MOVE_REFUSAL_WALL
	}
	return playv1.MoveRefusal_MOVE_REFUSAL_TOO_COSTLY
}

// masterTargetCall is what the master's marks on a combatant read from their requests: who
// asks, the key and the combat and combatant named.
type masterTargetCall struct {
	m                  authz.Membership
	key, encID, combID string
}

// readMasterTargetCall reads and checks the master's call, in the order the refusals have
// always come.
func readMasterTargetCall(ctx context.Context, campaignID, rawKey, encounterID, combatantID string) (masterTargetCall, error) {
	m, err := authz.RequireCampaignRole(ctx, campaignID, authz.RoleMaster)
	if err != nil {
		return masterTargetCall{}, err
	}
	key, err := parseKey(rawKey)
	if err != nil {
		return masterTargetCall{}, err
	}
	encID, err := parseCombatID(encounterID, "encounter")
	if err != nil {
		return masterTargetCall{}, err
	}
	combID, err := parseCombatID(combatantID, "combatant")
	if err != nil {
		return masterTargetCall{}, err
	}
	return masterTargetCall{m: m, key: key, encID: encID, combID: combID}, nil
}

// sideKeyOf is the side as the table stores it, and false for UNSPECIFIED (not a side).
func sideKeyOf(side playv1.CombatantSide) (string, bool) {
	switch side {
	case playv1.CombatantSide_COMBATANT_SIDE_PARTY:
		return sidePartyKey, true
	case playv1.CombatantSide_COMBATANT_SIDE_ENEMY:
		return sideEnemyKey, true
	}
	return "", false
}

// SetCombatantSide implements playv1connect.CombatServiceHandler.
func (s *Service) SetCombatantSide(
	ctx context.Context,
	req *connect.Request[playv1.SetCombatantSideRequest],
) (*connect.Response[playv1.SetCombatantSideResponse], error) {
	call, err := readMasterTargetCall(ctx, req.Msg.GetCampaignId(), req.Msg.GetIdempotencyKey(), req.Msg.GetEncounterId(), req.Msg.GetCombatantId())
	if err != nil {
		return nil, err
	}
	side, ok := sideKeyOf(req.Msg.GetSide())
	if !ok {
		return nil, connect.NewError(connect.CodeInvalidArgument, errors.New("side must be PARTY or ENEMY"))
	}
	m := call.m
	res, err := s.write(ctx, combatWrite{m: m, key: call.key, hash: idem.Hash(req.Msg), kind: eventSideSet, encounterID: call.encID}, func(c *combatTx) (any, error) {
		return s.sideIn(ctx, c, call, side)
	})
	if err != nil {
		return nil, s.dbError(ctx, "set a combatant's side", err)
	}
	out, err := s.finish(ctx, m, res, s.changedFor(m.CampaignID, call.combID))
	if err != nil {
		return nil, err
	}
	return connect.NewResponse(&playv1.SetCombatantSideResponse{Encounter: out}), nil
}

// sideIn is the change of SetCombatantSide: an NPC changes sides, and the offers that wait
// between what are now allies are passed over.
func (s *Service) sideIn(ctx context.Context, c *combatTx, call masterTargetCall, side string) (any, error) {
	if err := notEnded(c.enc); err != nil {
		return nil, err
	}
	target, err := s.masterTargetOf(ctx, c, call)
	if err != nil {
		return nil, err
	}
	if target.Kind != kindNPC {
		return nil, connect.NewError(connect.CodeInvalidArgument, errors.New("a player's character is always on the party's side"))
	}
	if err := c.q.SetCombatantSide(ctx, playdb.SetCombatantSideParams{ID: target.ID, Side: side}); err != nil {
		return nil, fmt.Errorf("set the side: %w", err)
	}
	// An offer waits for a hostile reactor: one that is now an ally of the
	// mover (or the mover, now an ally of the reactor) no longer attacks.
	now := s.now()
	if _, err := c.q.SkipPendingOpportunityOffersBetweenAllies(ctx, playdb.SkipPendingOpportunityOffersBetweenAlliesParams{
		EncounterID: c.enc.ID, MoverID: target.ID, AnsweredAt: &now,
	}); err != nil {
		return nil, fmt.Errorf("pass over the offers between allies: %w", err)
	}
	if err := touchEncounter(ctx, c); err != nil {
		return nil, err
	}
	c.characterID = &target.CharacterID
	return actionEvent{Round: c.enc.Round, Secret: target.Hidden, Actor: target.ID, Side: side, SideBefore: target.Side}, nil
}

// masterTargetOf finds the combatant the master's call names, as the master sees it.
func (s *Service) masterTargetOf(ctx context.Context, c *combatTx, call masterTargetCall) (playdb.Combatant, error) {
	cs, err := c.q.ListCombatants(ctx, c.enc.ID)
	if err != nil {
		return playdb.Combatant{}, fmt.Errorf("list the combatants: %w", err)
	}
	return findCombatant(cs, call.combID, viewerOf(call.m))
}

// SetCombatantCover implements playv1connect.CombatServiceHandler.
func (s *Service) SetCombatantCover(
	ctx context.Context,
	req *connect.Request[playv1.SetCombatantCoverRequest],
) (*connect.Response[playv1.SetCombatantCoverResponse], error) {
	call, err := readMasterTargetCall(ctx, req.Msg.GetCampaignId(), req.Msg.GetIdempotencyKey(), req.Msg.GetEncounterId(), req.Msg.GetCombatantId())
	if err != nil {
		return nil, err
	}
	mark, ok := coverMarkKey(req.Msg.GetCover())
	if !ok {
		return nil, connect.NewError(connect.CodeInvalidArgument, errors.New("cover must be NONE, HALF, THREE_QUARTERS or TOTAL"))
	}
	m := call.m
	res, err := s.write(ctx, combatWrite{m: m, key: call.key, hash: idem.Hash(req.Msg), kind: eventCoverSet, encounterID: call.encID}, func(c *combatTx) (any, error) {
		return s.coverIn(ctx, c, call, mark)
	})
	if err != nil {
		return nil, s.dbError(ctx, "mark a combatant's cover", err)
	}
	out, err := s.finish(ctx, m, res, s.changedFor(m.CampaignID, call.combID))
	if err != nil {
		return nil, err
	}
	return connect.NewResponse(&playv1.SetCombatantCoverResponse{Encounter: out}), nil
}

// coverIn is the change of SetCombatantCover: the master's cover mark on a combatant.
func (s *Service) coverIn(ctx context.Context, c *combatTx, call masterTargetCall, mark string) (any, error) {
	if err := notEnded(c.enc); err != nil {
		return nil, err
	}
	target, err := s.masterTargetOf(ctx, c, call)
	if err != nil {
		return nil, err
	}
	if err := c.q.SetCombatantCoverMark(ctx, playdb.SetCombatantCoverMarkParams{ID: target.ID, CoverMark: mark}); err != nil {
		return nil, fmt.Errorf("set the cover: %w", err)
	}
	if err := touchEncounter(ctx, c); err != nil {
		return nil, err
	}
	c.characterID = &target.CharacterID
	return actionEvent{Round: c.enc.Round, Secret: target.Hidden, Actor: target.ID, CoverMark: mark, CoverMarkBefore: target.CoverMark}, nil
}

// coverMarkKey is a cover as the table stores the master's mark, and false for
// UNSPECIFIED (not a cover).
func coverMarkKey(c playv1.CoverDegree) (string, bool) {
	switch c {
	case playv1.CoverDegree_COVER_DEGREE_NONE:
		return "none", true
	case playv1.CoverDegree_COVER_DEGREE_HALF:
		return "half", true
	case playv1.CoverDegree_COVER_DEGREE_THREE_QUARTERS:
		return "three_quarters", true
	case playv1.CoverDegree_COVER_DEGREE_TOTAL:
		return "total", true
	}
	return "", false
}

// sideProto is a combatant's side as the API says it.
func sideProto(side string) playv1.CombatantSide {
	if side == sidePartyKey {
		return playv1.CombatantSide_COMBATANT_SIDE_PARTY
	}
	return playv1.CombatantSide_COMBATANT_SIDE_ENEMY
}

// sideOfKind is the side a combatant joins on: the players' characters are the
// party, the NPCs the enemy (the master marks one "Aliado" later).
func sideOfKind(kind string) string {
	if kind == kindPlayer {
		return sidePartyKey
	}
	return sideEnemyKey
}

// sizeProto is a combatant's size as the API says it.
func sizeProto(size string) rulesv1.CreatureSize { return sizeToProto[sizeKey(strings.ToLower(size))] }
