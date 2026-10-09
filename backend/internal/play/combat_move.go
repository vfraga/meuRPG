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
// unconscious creature cannot move at all. (Prone, which makes standing up cost
// half the speed, is not modeled: the app has no action for standing up.)
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
	speed := int(max(c.SpeedFt, c.SpeedFlyFt)) * 10
	if c.Dashed {
		speed *= 2
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

// MoveCombatant implements playv1connect.CombatServiceHandler.
func (s *Service) MoveCombatant(
	ctx context.Context,
	req *connect.Request[playv1.MoveCombatantRequest],
) (*connect.Response[playv1.MoveCombatantResponse], error) {
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
	col, row := req.Msg.GetCol(), req.Msg.GetRow()
	jump := req.Msg.GetJump()
	height := req.Msg.GetJumpHeightDft()
	switch jump {
	case playv1.JumpKind_JUMP_KIND_UNSPECIFIED, playv1.JumpKind_JUMP_KIND_LONG:
	case playv1.JumpKind_JUMP_KIND_HIGH:
		if height < 1 || height > maxJumpDFt {
			return nil, connect.NewError(connect.CodeInvalidArgument, fmt.Errorf("jump_height_dft must be 1 to %d", maxJumpDFt))
		}
	default:
		return nil, connect.NewError(connect.CodeInvalidArgument, errors.New("jump must be a kind of jump"))
	}
	v := viewerOf(m)
	// A forced move (a teleport, a token put right) is the master's, and never
	// provokes.
	forced := req.Msg.GetForced()
	if forced && !v.master {
		return nil, connect.NewError(connect.CodePermissionDenied, errors.New("only the master may force a move"))
	}

	var moved playdb.Combatant
	var made actionEvent
	var stoppedEarly bool    // a creature the player does not see cut the move short
	var logged bool          // the move is a line of the combat log
	var offered []string     // the reactors the move offered an attack
	var markCleared bool     // the move took the master's cover mark off
	var th *trapHook         // the traps the move may fire (combat_traps.go, MR-035)
	var moveID string        // the id of the opportunity offers the move made, if any
	var opened []grid.Square // the closed doors the move opened (RN-26)
	var lockedDoor bool      // a locked door stopped the move (a player only learns of one they know)
	res, err := s.write(ctx, combatWrite{m: m, key: key, hash: idem.Hash(req.Msg), kind: eventCombatantMoved, altKind: eventTrapTriggered, encounterID: encID}, func(c *combatTx) (any, error) {
		logged, stoppedEarly, moveID, opened, lockedDoor = false, false, "", nil, false
		if err := notEnded(c.enc); err != nil {
			return nil, err
		}
		if isTheatre(c.enc) {
			return nil, errNeedsAMap() // no squares: a move, a jump and a placement are the master's word (SpendMovement)
		}
		cs, err := c.q.ListCombatants(ctx, c.enc.ID)
		if err != nil {
			return nil, fmt.Errorf("list the combatants: %w", err)
		}
		v = c.viewer(m, cs) // the fog: an NPC the player does not see is not found
		target, err := findCombatant(cs, combID, v)
		if err != nil {
			return nil, err
		}
		if err := v.mayAct(target); err != nil {
			return nil, err
		}
		if jump != playv1.JumpKind_JUMP_KIND_HIGH && !inGrid(c.enc, col, row) {
			return nil, connect.NewError(connect.CodeInvalidArgument, errors.New("col and row must be a square of the grid"))
		}
		if jump != playv1.JumpKind_JUMP_KIND_UNSPECIFIED && !placed(target) {
			return nil, errEncounter(playv1.EncounterBlockedReason_ENCOUNTER_BLOCKED_REASON_NOT_PLACED, "a jump starts from a square of the map")
		}
		terrain, err := s.terrainOf(ctx, c.tx, m.CampaignID, c.enc)
		if err != nil {
			return nil, err
		}
		ground := terrain
		terrain = withZones(ground, c.zones, target, false) // the ground the zones leave: the real move costs all of it
		onTurn := actsNow(c.enc, target)
		if !v.master {
			// RN-21: a player walks only on their own turn.
			switch {
			case c.enc.Status != statusActive:
				return nil, errEncounter(playv1.EncounterBlockedReason_ENCOUNTER_BLOCKED_REASON_NOT_ACTIVE, "the combat is not running")
			case !onTurn: // not in the turn that is running, or its part ended
				return nil, errEncounter(playv1.EncounterBlockedReason_ENCOUNTER_BLOCKED_REASON_NOT_YOUR_TURN, "it is not your turn")
			case !placed(target):
				return nil, errEncounter(playv1.EncounterBlockedReason_ENCOUNTER_BLOCKED_REASON_NOT_PLACED, "you are not on the map yet; ask the master")
			}
			if err := s.mustNotBeDown(ctx, c.tx, m.CampaignID, target); err != nil {
				return nil, err
			}
		}

		if th, err = s.newTrapHook(ctx, c.tx, m.CampaignID, c.enc, target); err != nil {
			return nil, err
		}
		from := moveStateOf(target)
		made = actionEvent{Round: c.enc.Round, Secret: target.Hidden, Actor: target.ID, OnTurn: onTurn, From: from}
		offered, markCleared = nil, false
		to := grid.Square{Col: int(col), Row: int(row)}
		if jump == playv1.JumpKind_JUMP_KIND_HIGH {
			to = squareOfCombatant(target) // a high jump moves nobody
		}
		cost, length, last := 0, 0, int(target.LastMoveDft) // the master's move leaves the running start alone
		if placed(target) && jump != playv1.JumpKind_JUMP_KIND_HIGH {
			length = grid.LengthDFt(squareOfCombatant(target), to)
		}

		if !v.master || actsNow(c.enc, target) { // the turn's move waits for the master's answer; his own moves of a token off turn do not
			if err := s.mustNotHold(ctx, c); err != nil {
				return nil, err
			}
		}
		if !v.master {
			if err := s.mustNotWait(ctx, c, target); err != nil {
				return nil, err
			}
			// The player plans on what they see (RN-10); the real move runs against
			// everyone, and stops before a creature they do not see, without saying
			// what it was (D1): the answer only says it was cut short.
			occ := occupantsFor(cs, target, v)
			all := occupantsFor(cs, target, combatViewer{master: true})
			known, err := c.sight.knownTerrain(ctx, c.tx, v) // the mover's is cached by sightForWrite; any other is read in this transaction
			if err != nil {
				return nil, err
			}
			plan := withZones(planOn(v, ground, known), c.zones, target, true) // the walls and rubble the player knows, and the zones their creature knows; the rest is floor to them
			mover, origin := moverOf(target), squareOfCombatant(target)
			left := movementLeftDFt(target)
			switch jump {
			case playv1.JumpKind_JUMP_KIND_UNSPECIFIED:
				mv := plan.Move(origin, to, occ, mover)
				if mv.Blocked {
					return nil, moveStop(mv, occ, to)
				}
				if mv.CostDFt > left {
					return nil, tooFar(mv.CostDFt - left)
				}
				st := terrain.MoveUntil(origin, to, all, mover, th.stops(), left)
				stoppedEarly = st.Reason != grid.StopNone && (st.Reason != grid.StopMarked || st.Reached != to) // a trap's area is not "in the way" when it is where the mover went
				if st.Reason == grid.StopLocked && doorKnown(plan, st.LockedAt) {
					// The player learns the door is locked by trying it (RN-26). A door they
					// do not see (the fog) is "something in the way" like any other.
					stoppedEarly, lockedDoor = false, true
					if len(st.Entered) == 0 {
						return nil, errEncounter(playv1.EncounterBlockedReason_ENCOUNTER_BLOCKED_REASON_DOOR_LOCKED, "the door is locked")
					}
				}
				opened = st.Opens
				th.marked(st.Marked, st.MarkedAt)
				to, cost = st.Reached, st.CostDFt
				length = grid.LengthDFt(origin, to)
				// Consecutive moves on foot add up to the running start (10 ft just
				// before); a flier's move is no run.
				switch {
				case length == 0:
				case mover.Flier:
					last = 0
				default:
					last += length
				}
			case playv1.JumpKind_JUMP_KIND_LONG:
				if length == 0 {
					return nil, connect.NewError(connect.CodeInvalidArgument, errors.New("a jump needs another square"))
				}
				running := combat.HasRunningStart(int(target.LastMoveDft))
				limit := longJumpDFt(target, running)
				if length > limit {
					return nil, tooFar(length-limit, jumpLimit(limit, running))
				}
				mv := plan.Jump(origin, to, occ, mover)
				if mv.Blocked {
					return nil, moveStop(mv, occ, to)
				}
				if mv.CostDFt > left {
					return nil, tooFar(mv.CostDFt - left)
				}
				if terrain.Jump(origin, to, all, mover).Blocked { // a creature it does not see holds the landing
					stoppedEarly, to, length = true, origin, 0
				} else {
					cost, last = mv.CostDFt, 0
					th.landed(to) // a jump fires a trap only where it lands (D3, D5)
				}
			case playv1.JumpKind_JUMP_KIND_HIGH:
				running := combat.HasRunningStart(int(target.LastMoveDft))
				limit := highJumpDFt(target, running)
				if int(height) > limit {
					return nil, tooFar(int(height)-limit, jumpLimit(limit, running))
				}
				if int(height) > left {
					return nil, tooFar(int(height) - left)
				}
				cost, last = int(height), 0
			}
		}
		if v.master && !forced && jump == playv1.JumpKind_JUMP_KIND_UNSPECIFIED && placed(target) && terrain.Doors.Count() > 0 {
			// The master's move is free and no wall stops it, but the doors do what they
			// do for everyone (RN-26): it opens the closed ones on its line, and a locked,
			// barred or secret one stops it before. The master unlocks by painting.
			origin := squareOfCombatant(target)
			st := grid.Terrain{Grid: terrain.Grid, Doors: terrain.Doors}.MoveUntil(origin, to, nil, moverOf(target), nil, math.MaxInt)
			if st.Reason == grid.StopLocked && len(st.Entered) == 0 {
				return nil, errEncounter(playv1.EncounterBlockedReason_ENCOUNTER_BLOCKED_REASON_DOOR_LOCKED, "the door is locked")
			}
			stoppedEarly, lockedDoor = st.Reason != grid.StopNone, st.Reason == grid.StopLocked
			to, opened = st.Reached, st.Opens
			length = grid.LengthDFt(origin, to)
		}
		newCol, newRow := clamp32(to.Col, 0, math.MaxInt32), clamp32(to.Row, 0, math.MaxInt32)
		// The master's cover mark clears when the combatant changes square, not
		// when nobody moved (a high jump, a move to where it stands).
		squareChanged := !placed(target) || to != squareOfCombatant(target)
		coverMark := target.CoverMark
		if squareChanged {
			markCleared = coverMark != "" && coverMark != "none"
			coverMark = "none"
		}
		made.StoppedEarly, made.LockedDoor = stoppedEarly, lockedDoor

		used := int(target.MovementUsedDft) + cost
		if err := c.q.SetCombatantMove(ctx, playdb.SetCombatantMoveParams{
			ID: target.ID, GridCol: &newCol, GridRow: &newRow,
			MovementUsedFt: clamp32(used/10, 0, math.MaxInt32), MovementUsedDft: clamp32(used, 0, math.MaxInt32), LastMoveDft: clamp32(last, 0, math.MaxInt32), CoverMark: coverMark,
		}); err != nil {
			return nil, fmt.Errorf("move the combatant: %w", err)
		}
		if c.enc, err = c.q.TouchEncounter(ctx, c.enc.ID); err != nil {
			return nil, fmt.Errorf("touch the encounter: %w", err)
		}
		// Opportunity attacks (D1b): a move on foot or a long jump (it spends
		// movement), on the mover's turn, that leaves an enemy's reach lands, and the
		// enemy is offered an attack. A high jump moves nobody; a placement out of
		// turn, a forced move and a move that stays put offer nothing.
		if jump != playv1.JumpKind_JUMP_KIND_HIGH && !forced && onTurn && placed(target) && squareChanged {
			if moveID, offered, err = s.offerOpportunities(ctx, c, cs, target, squareOfCombatant(target), to); err != nil {
				return nil, err
			}
			made.MoveID = moveID
		}
		moved = target
		moved.GridCol, moved.GridRow, moved.MovementUsedFt, moved.MovementUsedDft = &newCol, &newRow, clamp32(used/10, 0, math.MaxInt32), clamp32(used, 0, math.MaxInt32)
		moved.LastMoveDft, moved.CoverMark = clamp32(last, 0, math.MaxInt32), coverMark
		c.characterID = &target.CharacterID
		if opened, err = s.openDoors(ctx, c, target, opened, from); err != nil {
			return nil, err
		}

		// The log tells how far a combatant went on its turn: the line between
		// where it was and where it goes, whoever moves it.
		made.Col, made.Row = newCol, newRow
		made.CostDFt, made.CostFt = clamp32(cost, 0, math.MaxInt32), clamp32(cost/10, 0, math.MaxInt32)
		made.DistanceDFt, made.DistanceFt = clamp32(length, 0, math.MaxInt32), clamp32(length/10, 0, math.MaxInt32)
		switch jump {
		case playv1.JumpKind_JUMP_KIND_LONG:
			made.Jump = jumpLong
			// SRD: landing in difficult terrain asks for a DC 10 Acrobatics check or
			// the jumper falls prone. The app rolls nothing: the master's log says it.
			made.LandingDifficult = !moverOf(target).Flier && terrain.Difficult.Has(to)
		case playv1.JumpKind_JUMP_KIND_HIGH:
			made.Jump, made.HeightDFt = jumpHigh, height
		}
		logged = onTurn && (length > 0 || made.Jump == jumpHigh)
		if v.master && squareChanged {
			th.landed(to) // the master's move is not a walk: only where it ends counts
		}
		// A creature that enters a zone, or travels in one that hurts by the distance, is caught by it;
		// a zone its caster carries goes with it (zones_engine.go).
		if squareChanged && jump != playv1.JumpKind_JUMP_KIND_HIGH && placed(moved) {
			if err := s.zonesAfterMove(ctx, c, cs, moved, squareOfState(from), ground); err != nil {
				return nil, err
			}
		}
		return th.finish(ctx, c, made, cs, moved)
	})
	if err != nil {
		return nil, s.dbError(ctx, "move a combatant", err)
	}
	out, err := s.finish(ctx, m, res, func(ctx context.Context, d *encounterData) {
		s.publishCombatantMoved(ctx, m.CampaignID, d.enc, moved, squareOfState(made.From))
		if made.MoveID != "" { // the offers are in the combat, not in the move's hint
			s.publishOffersMade(m.CampaignID, d, moved, offered)
		}
		if markCleared { // the mark is on the combatant, not in the move's hint: who saw it reads again
			if moved.Hidden {
				s.hub.Publish(m.CampaignID, live.Event{Audience: live.Audience{Master: true}, Message: encounterChangedMessage(d.enc)})
			} else {
				s.publishEncounterChanged(ctx, m.CampaignID, d.enc)
			}
		}
		s.positionChanged(ctx, m.CampaignID, d.enc, moved, squareOfState(made.From)) // the fog remembers what it showed
		if len(opened) > 0 && s.doors != nil && d.enc.MapID != nil {                 // the map's layers changed, and what the players see with them
			s.doors.DoorsChanged(ctx, m.CampaignID, *d.enc.MapID)
		}
		if logged || len(opened) > 0 {
			s.publishLogChanged(ctx, m.CampaignID, d.enc.ID, !moved.Hidden)
		}
		th.after(ctx, m.CampaignID, d.enc)
		s.noticeAfterMove(ctx, m.CampaignID, d.enc, moved) // a trap near where the mover ended may be noticed (MR-035)
	})
	if err != nil {
		return nil, err
	}
	ev, err := resultEvent(res, made)
	if err != nil {
		return nil, s.dbError(ctx, "read the move", err)
	}
	return connect.NewResponse(&playv1.MoveCombatantResponse{Encounter: out, StoppedEarly: ev.StoppedEarly, Provoked: ev.MoveID != "", LockedDoor: ev.LockedDoor}), nil
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
// when the combatant joined (the standing jump is half of the running one).
func longJumpDFt(c playdb.Combatant, running bool) int {
	return combat.Jumps{LongRunning: int(c.JumpLongDft), LongStanding: int(c.JumpLongDft) / 2}.Long(running)
}

func highJumpDFt(c playdb.Combatant, running bool) int {
	return combat.Jumps{HighRunning: int(c.JumpHighDft), HighStanding: int(c.JumpHighDft) / 2}.High(running)
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
	enc := d.enc
	v, sight, err := s.viewerWith(ctx, m, enc, d.cs)
	if err != nil {
		return nil, s.dbError(ctx, "work out what the player sees", err)
	}
	who, err := findCombatant(d.cs, combID, v)
	if err != nil {
		return nil, err
	}
	if err := v.mayAct(who); err != nil {
		return nil, err
	}
	if sight == nil && v.master { // the master's warning follows the same fog rule as the real offers
		if sight, err = s.fogSightOf(ctx, m.CampaignID, enc); err != nil {
			return nil, s.dbError(ctx, "work out what the players see", err)
		}
	}
	if err := notEnded(enc); err != nil {
		return nil, err
	}
	if !v.master {
		switch {
		case enc.Status != statusActive:
			return nil, errEncounter(playv1.EncounterBlockedReason_ENCOUNTER_BLOCKED_REASON_NOT_ACTIVE, "the combat is not running")
		case !actsNow(enc, who):
			return nil, errEncounter(playv1.EncounterBlockedReason_ENCOUNTER_BLOCKED_REASON_NOT_YOUR_TURN, "it is not your turn")
		}
		if err := s.mustNotBeDown(ctx, nil, m.CampaignID, who); err != nil {
			return nil, err
		}
	}
	if isTheatre(enc) {
		// Without a map there is nowhere to go: a valid, empty answer, with what
		// SpendMovement can still spend.
		return connect.NewResponse(&playv1.GetMoveOptionsResponse{MovementLeftDft: clamp32(movementLeftDFt(who), 0, math.MaxInt32), Flier: flies(who)}), nil
	}
	if !placed(who) {
		return nil, errEncounter(playv1.EncounterBlockedReason_ENCOUNTER_BLOCKED_REASON_NOT_PLACED, "the combatant is not on the map yet")
	}
	terrain, err := s.terrainOf(ctx, nil, m.CampaignID, enc)
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
	zones := zoneStatesOf(d.zones)
	out := moveOptions(withZones(planOn(v, terrain, known), zones, who, !v.master), who, occupantsFor(d.cs, who, v))
	zoneMoveOptions(out, zones, who, terrain)
	if err := s.markProvokes(ctx, m, enc, d.cs, who, v, sight, out); err != nil {
		return nil, s.dbError(ctx, "work out the opportunity attacks", err)
	}
	if !v.master {
		s.markKnownTraps(ctx, m.CampaignID, enc, who, out) // the warning before stepping into a trap the character knows
	}
	return connect.NewResponse(out), nil
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
	// Inside the circle, the squares that are not reachable: why not. They come
	// in reading order too, like the reachable ones.
	radius := left/grid.DFtPerSquare + 1
	for row := max(from.Row-radius, 0); row <= min(from.Row+radius, t.Grid.Rows-1); row++ {
		for col := max(from.Col-radius, 0); col <= min(from.Col+radius, t.Grid.Columns-1); col++ {
			sq := grid.Square{Col: col, Row: row}
			if sq == from || reachable[sq] || grid.LengthDFt(from, sq) > left {
				continue
			}
			mv := t.Move(from, sq, occ, mover)
			reason := playv1.MoveRefusal_MOVE_REFUSAL_TOO_COSTLY
			switch {
			case mv.Blocked && mv.By == grid.StopOccupied:
				reason = playv1.MoveRefusal_MOVE_REFUSAL_ENEMY
				if _, here := occ.At(sq); here {
					reason = playv1.MoveRefusal_MOVE_REFUSAL_OCCUPIED
				}
			case mv.Blocked:
				reason = playv1.MoveRefusal_MOVE_REFUSAL_WALL
			}
			out.Refused = append(out.Refused, &playv1.RefusedSquare{Col: clamp32(col, 0, math.MaxInt32), Row: clamp32(row, 0, math.MaxInt32), Reason: reason})
		}
	}
	return out
}

// SetCombatantSide implements playv1connect.CombatServiceHandler.
func (s *Service) SetCombatantSide(
	ctx context.Context,
	req *connect.Request[playv1.SetCombatantSideRequest],
) (*connect.Response[playv1.SetCombatantSideResponse], error) {
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
	var side string
	switch req.Msg.GetSide() {
	case playv1.CombatantSide_COMBATANT_SIDE_PARTY:
		side = sidePartyKey
	case playv1.CombatantSide_COMBATANT_SIDE_ENEMY:
		side = sideEnemyKey
	default:
		return nil, connect.NewError(connect.CodeInvalidArgument, errors.New("side must be PARTY or ENEMY"))
	}

	res, err := s.write(ctx, combatWrite{m: m, key: key, hash: idem.Hash(req.Msg), kind: eventSideSet, encounterID: encID}, func(c *combatTx) (any, error) {
		if err := notEnded(c.enc); err != nil {
			return nil, err
		}
		cs, err := c.q.ListCombatants(ctx, c.enc.ID)
		if err != nil {
			return nil, fmt.Errorf("list the combatants: %w", err)
		}
		target, err := findCombatant(cs, combID, viewerOf(m))
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
		if c.enc, err = c.q.TouchEncounter(ctx, c.enc.ID); err != nil {
			return nil, fmt.Errorf("touch the encounter: %w", err)
		}
		c.characterID = &target.CharacterID
		return actionEvent{Round: c.enc.Round, Secret: target.Hidden, Actor: target.ID, Side: side, SideBefore: target.Side}, nil
	})
	if err != nil {
		return nil, s.dbError(ctx, "set a combatant's side", err)
	}
	out, err := s.finish(ctx, m, res, s.changedFor(m.CampaignID, combID))
	if err != nil {
		return nil, err
	}
	return connect.NewResponse(&playv1.SetCombatantSideResponse{Encounter: out}), nil
}

// SetCombatantCover implements playv1connect.CombatServiceHandler.
func (s *Service) SetCombatantCover(
	ctx context.Context,
	req *connect.Request[playv1.SetCombatantCoverRequest],
) (*connect.Response[playv1.SetCombatantCoverResponse], error) {
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
	mark, ok := coverMarkKey(req.Msg.GetCover())
	if !ok {
		return nil, connect.NewError(connect.CodeInvalidArgument, errors.New("cover must be NONE, HALF, THREE_QUARTERS or TOTAL"))
	}

	res, err := s.write(ctx, combatWrite{m: m, key: key, hash: idem.Hash(req.Msg), kind: eventCoverSet, encounterID: encID}, func(c *combatTx) (any, error) {
		if err := notEnded(c.enc); err != nil {
			return nil, err
		}
		cs, err := c.q.ListCombatants(ctx, c.enc.ID)
		if err != nil {
			return nil, fmt.Errorf("list the combatants: %w", err)
		}
		target, err := findCombatant(cs, combID, viewerOf(m))
		if err != nil {
			return nil, err
		}
		if err := c.q.SetCombatantCoverMark(ctx, playdb.SetCombatantCoverMarkParams{ID: target.ID, CoverMark: mark}); err != nil {
			return nil, fmt.Errorf("set the cover: %w", err)
		}
		if c.enc, err = c.q.TouchEncounter(ctx, c.enc.ID); err != nil {
			return nil, fmt.Errorf("touch the encounter: %w", err)
		}
		c.characterID = &target.CharacterID
		return actionEvent{Round: c.enc.Round, Secret: target.Hidden, Actor: target.ID, CoverMark: mark, CoverMarkBefore: target.CoverMark}, nil
	})
	if err != nil {
		return nil, s.dbError(ctx, "mark a combatant's cover", err)
	}
	out, err := s.finish(ctx, m, res, s.changedFor(m.CampaignID, combID))
	if err != nil {
		return nil, err
	}
	return connect.NewResponse(&playv1.SetCombatantCoverResponse{Encounter: out}), nil
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
