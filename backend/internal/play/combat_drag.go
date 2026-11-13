package play

import (
	"context"
	"fmt"
	"math"

	playv1 "github.com/PuraFome/meuRPG/backend/gen/meurpg/play/v1"
	"github.com/PuraFome/meuRPG/backend/internal/play/playdb"
	"github.com/PuraFome/meuRPG/backend/internal/rules/combat"
	"github.com/PuraFome/meuRPG/backend/internal/rules/grid"
)

// Moving a grappled creature (SRD 5.1, "Moving a Grappled Creature"): when the grappler
// moves it drags the creature it holds along, and its speed is halved, unless the creature is
// two or more sizes smaller. A move that drags costs twice its length, which is the half
// speed: with 9 m of speed, 4.5 m of path fit. The dragged creature ends on the square the
// grappler left just before the last square of its path (the grappler's own starting square for
// a one-square move), which the server works out; when that square is held by another creature
// the move is refused (NO_ROOM_TO_DRAG). The dragged creature spends nothing and provokes
// nothing: it is moved without its own movement, action or reaction.

// draggedBy is the creature a grappler holds, when the hold still holds.
func (s *Service) draggedBy(ctx context.Context, q *playdb.Queries, cs []playdb.Combatant, grappler playdb.Combatant) (playdb.Combatant, bool, error) {
	holds, err := q.ListHolds(ctx, grappler.EncounterID)
	if err != nil {
		return playdb.Combatant{}, false, fmt.Errorf("list the holds: %w", err)
	}
	for _, h := range holds {
		if h.GrapplerID != grappler.ID {
			continue
		}
		if g, ok := combatantByID(cs, h.GrappledID); ok && isGrappled(g) && !g.Defeated && placed(g) {
			return g, true, nil
		}
	}
	return playdb.Combatant{}, false, nil
}

// dragSquare is the square a dragged creature ends on for a move from origin to to: the
// one before the last square the line enters, or the origin for a one-square move.
func dragSquare(terrain grid.Terrain, origin, to grid.Square, mover grid.Mover) grid.Square {
	entered := terrain.Move(origin, to, grid.OccupantMap{}, mover).Entered
	if len(entered) > 1 {
		return entered[len(entered)-2]
	}
	return origin
}

// dragRoom says whether the square a dragged creature would end on is free: no other creature
// is there, as far as the viewer knows (the whole table for the real move).
func dragRoom(cs []playdb.Combatant, grappler, dragged playdb.Combatant, sq grid.Square, v *combatViewer) bool {
	for _, o := range cs {
		if o.ID == grappler.ID || o.ID == dragged.ID || o.Defeated || !placed(o) || squareOfCombatant(o) != sq {
			continue
		}
		if v == nil || v.sees(o) {
			return false
		}
	}
	return true
}

// dragRoomError is the refusal of a drag onto a taken square, nil when there is
// room. When every creature on the square is unseen by the viewer it is the walk's
// generic refusal, so the drag does not tell a player that something unseen
// stands there (RN-10).
func dragRoomError(cs []playdb.Combatant, grappler, dragged playdb.Combatant, sq grid.Square, v combatViewer) error {
	if dragRoom(cs, grappler, dragged, sq, nil) {
		return nil
	}
	if dragRoom(cs, grappler, dragged, sq, &v) {
		return errMoveBlocked()
	}
	return errContest(playv1.ContestBlockedReason_CONTEST_BLOCKED_REASON_NO_ROOM_TO_DRAG, "there is no room to leave the creature")
}

// dragTo moves the creature a grappler drags to the square behind its path, in the move's
// transaction, and returns it as it stands now. The master's cover mark clears, as for any
// move; the movement, the action and the reaction of the dragged creature stay as they were.
func (s *Service) dragTo(ctx context.Context, c *combatTx, v combatViewer, cs []playdb.Combatant, grappler, dragged playdb.Combatant, sq grid.Square) (playdb.Combatant, error) {
	if err := dragRoomError(cs, grappler, dragged, sq, v); err != nil {
		return dragged, err
	}
	col, row := clamp32(sq.Col, 0, math.MaxInt32), clamp32(sq.Row, 0, math.MaxInt32)
	if err := c.q.SetCombatantPlace(ctx, playdb.SetCombatantPlaceParams{ID: dragged.ID, GridCol: &col, GridRow: &row, CoverMark: "none"}); err != nil {
		return dragged, fmt.Errorf("drag the creature: %w", err)
	}
	dragged.GridCol, dragged.GridRow, dragged.CoverMark = &col, &row, "none"
	return dragged, nil
}

// moveOptionsWithDrag is moveOptions for a combatant that may drag the creature it holds: the
// movement left is the path that fits at half speed, and each square says where the dragged
// creature would end (a square with no room for it is refused). A combatant that drags nobody
// gets moveOptions as it is.
func (s *Service) moveOptionsWithDrag(ctx context.Context, plan grid.Terrain, cs []playdb.Combatant, who playdb.Combatant, mover grid.Mover, v combatViewer) (*playv1.GetMoveOptionsResponse, error) {
	occ := occupantsFor(cs, who, v)
	dragged, dragging, err := s.draggedBy(ctx, s.queries, cs, who)
	// moveOptions reads the movement left: the path that fits is what is left divided by
	// what the move multiplies the cost by. Crawling doubles it (combat_standup.go), and so
	// does a drag at half speed.
	div := 1
	if crawls(who) {
		div *= 2
	}
	if err != nil || !dragging {
		return moveOptions(plan, probeWithLeftOver(who, div), mover, occ), err
	}
	halves := combat.DragHalvesSpeed(who.Size, dragged.Size)
	if halves {
		div *= 2
	}
	probe := probeWithLeftOver(who, div)
	out := moveOptions(plan, probe, mover, occ)
	if !v.master && !v.sees(dragged) {
		return out, nil // the halving holds, but what it drags is not theirs to know
	}
	out.DraggingCombatantId, out.DraggingHalved = dragged.ID, halves
	from := squareOfCombatant(who)
	reachable := out.Reachable[:0]
	for _, r := range out.Reachable {
		to := grid.Square{Col: int(r.Col), Row: int(r.Row)}
		sq := dragSquare(plan, from, to, mover)
		if !dragRoom(cs, who, dragged, sq, &v) {
			out.Refused = append(out.Refused, &playv1.RefusedSquare{Col: r.Col, Row: r.Row, Reason: playv1.MoveRefusal_MOVE_REFUSAL_NO_ROOM_TO_DRAG})
			continue
		}
		r.DraggedTo = &playv1.DraggedSquare{Col: clamp32(sq.Col, 0, math.MaxInt32), Row: clamp32(sq.Row, 0, math.MaxInt32)}
		reachable = append(reachable, r)
	}
	out.Reachable = reachable
	return out, nil
}

// probeWithLeftOver is the combatant with only the 1/div of its movement left that a move
// that costs div times as much can pay for, for moveOptions to read.
func probeWithLeftOver(who playdb.Combatant, div int) playdb.Combatant {
	if div > 1 {
		who.MovementUsedDft = clamp32(max(speedDFt(who)-movementLeftDFt(who)/div, 0), 0, math.MaxInt32)
	}
	return who
}
