package play

import (
	"math"
	"slices"

	playv1 "github.com/PuraFome/meuRPG/backend/gen/meurpg/play/v1"
	"github.com/PuraFome/meuRPG/backend/internal/play/link"
	"github.com/PuraFome/meuRPG/backend/internal/rules/grid"
)

// A wall (Wall of Fire, SRD 5.1) is a zone whose squares are a line or a ring laid from the point of origin.
// They are not a shape of the spell's area.
// A creature is inside the wall when its square is one of them.

// The largest a wall is, in feet: a straight wall's length and a ring's diameter.
const (
	wallLineFt = 60
	wallRingFt = 20
)

// wallReachFt is how far from the damaging side the wall hurts.
const wallReachFt = 10

// wallSpec is a wall as the caster laid it.
type wallSpec struct {
	// Ring says the wall is a ring around the point of origin; otherwise it is a straight wall from it.
	Ring bool `json:"ring,omitempty"`
	// Dir is the way a straight wall runs from the point of origin; unset for a ring.
	Dir grid.Direction `json:"dir"`
	// LengthFt is the straight wall's length or the ring's diameter, never 0.
	LengthFt int `json:"length_ft"`
	// Side is the side that deals the damage: left or right of a straight wall, inner or outer of a ring.
	Side playv1.WallSide `json:"side"`
}

// wallSideNames are the names the zone row keeps for the sides (wallOfRow reads them back).
var wallSideNames = map[playv1.WallSide]string{
	playv1.WallSide_WALL_SIDE_LEFT:  "left",
	playv1.WallSide_WALL_SIDE_RIGHT: "right",
	playv1.WallSide_WALL_SIDE_INNER: "inner",
	playv1.WallSide_WALL_SIDE_OUTER: "outer",
}

// needsWall says the spell lays a wall: the cast must say its shape and its damaging side.
func needsWall(sp link.Spell) bool {
	d, ok := zoneDefOfSpell(sp)
	return ok && d.kind == zoneWall
}

// wallFor reads the wall of a cast or a preview on a map.
// A spell that lays a wall must have one, laid from a point of origin.
// No other spell takes one.
func wallFor(sp link.Spell, w *playv1.SpellWall, origin *playv1.SpellOrigin) (*wallSpec, error) {
	switch {
	case !needsWall(sp):
		if w != nil {
			return nil, badArea("wall is for a spell that lays a wall")
		}
		return nil, nil
	case w == nil:
		return nil, errWallPlacementRequired()
	case origin == nil:
		return nil, badArea("a wall is laid from the point of origin: set origin")
	}
	spec, err := readWall(w)
	if err != nil {
		return nil, err
	}
	return &spec, nil
}

// errWallPlacementRequired is the refusal of a cast that lays a wall and comes without one.
func errWallPlacementRequired() error {
	return errEncounter(playv1.EncounterBlockedReason_ENCOUNTER_BLOCKED_REASON_WALL_PLACEMENT_REQUIRED,
		"this spell lays a wall: set wall with its shape, size and damaging side")
}

// readWall checks a wall the request sends and fills in its full size.
// A straight wall runs in one of the eight directions and has a left or a right side.
// A ring has no direction and an inner or an outer side.
func readWall(w *playv1.SpellWall) (wallSpec, error) {
	side := w.GetSide()
	spec := wallSpec{Ring: w.GetRing(), Side: side}
	size, sideOK := wallLineFt, side == playv1.WallSide_WALL_SIDE_LEFT || side == playv1.WallSide_WALL_SIDE_RIGHT
	if w.GetRing() {
		size, sideOK = wallRingFt, side == playv1.WallSide_WALL_SIDE_INNER || side == playv1.WallSide_WALL_SIDE_OUTER
		if w.GetDirection() != nil {
			return wallSpec{}, badArea("a ringed wall has no direction")
		}
	} else {
		if w.GetDirection() == nil {
			return wallSpec{}, badArea("a straight wall needs its direction")
		}
		spec.Dir = grid.Direction{Dx: int(w.GetDirection().GetDx()), Dy: int(w.GetDirection().GetDy())}
		if !spec.Dir.Valid() {
			return wallSpec{}, badArea("the direction must be one of the eight: dx and dy from -1 to 1, not both 0")
		}
	}
	if !sideOK {
		return wallSpec{}, badArea("the side must be LEFT or RIGHT for a straight wall and INNER or OUTER for a ring")
	}
	length, err := wallLength(int(w.GetLengthFt()), size)
	spec.LengthFt = length
	return spec, err
}

// wallLength is the length a request asks for: 0 is the full size, anything else a multiple of 5 up to the size.
func wallLength(asked, size int) (int, error) {
	if asked == 0 {
		return size, nil
	}
	if asked < grid.FeetPerSquare || asked > size || asked%grid.FeetPerSquare != 0 {
		return 0, badArea("length_ft must be a multiple of 5 up to the spell's size (60 for a straight wall, 20 for a ring), or 0 for the full size")
	}
	return asked, nil
}

// wallOfRow rebuilds the wall a zone row keeps; nil for a zone that is no wall.
func wallOfRow(side string, dx, dy int16, lengthFt int32) *wallSpec {
	w := &wallSpec{Dir: grid.Direction{Dx: int(dx), Dy: int(dy)}, LengthFt: int(lengthFt)}
	switch side {
	case "left":
		w.Side = playv1.WallSide_WALL_SIDE_LEFT
	case "right":
		w.Side = playv1.WallSide_WALL_SIDE_RIGHT
	case "inner":
		w.Side, w.Ring = playv1.WallSide_WALL_SIDE_INNER, true
	case "outer":
		w.Side, w.Ring = playv1.WallSide_WALL_SIDE_OUTER, true
	default:
		return nil
	}
	return w
}

// ringFt is the distance of the squares of a ring from the point of origin, in feet.
// It is the radius taken up to the next multiple of 5.
func (w wallSpec) ringFt() int {
	return (w.LengthFt/2 + grid.FeetPerSquare - 1) / grid.FeetPerSquare * grid.FeetPerSquare
}

// shape is the shape and the size the zone view tells: a line of the wall's length, or a sphere of the ring's radius.
func (w wallSpec) shape() (grid.Shape, int) {
	if w.Ring {
		return grid.ShapeSphere, w.ringFt()
	}
	return grid.ShapeLine, w.LengthFt
}

// squares are the squares of the wall laid from the point of origin, before the terrain is looked at.
//
// A straight wall starts on the point of origin.
// It takes one square at a time in its direction.
// A square is in while its distance from the point of origin, plus the square itself, is at most the length.
// The distance is the map's own, so a wall of 60 feet has 12 squares along a row and 9 along a diagonal.
//
// A ring is the squares at the radius from the point of origin.
// The radius is half the diameter, taken up to the next multiple of 5.
// A diameter of 20 feet is the squares 10 feet out.
// A diameter of 10 feet is the 8 squares around the point of origin.
func (w wallSpec) squares(origin grid.Square) []grid.Square {
	if w.Ring {
		return w.ringSquares(origin)
	}
	var out []grid.Square
	if !w.Dir.Valid() {
		return out
	}
	for k := 0; k <= w.LengthFt/grid.FeetPerSquare; k++ {
		sq := grid.Square{Col: origin.Col + k*w.Dir.Dx, Row: origin.Row + k*w.Dir.Dy}
		if grid.RangeFt(origin, sq)+grid.FeetPerSquare > w.LengthFt {
			break
		}
		out = append(out, sq)
	}
	return out
}

func (w wallSpec) ringSquares(origin grid.Square) []grid.Square {
	out := []grid.Square{}
	reach := w.ringFt() / grid.FeetPerSquare
	for dr := -reach; dr <= reach; dr++ {
		for dc := -reach; dc <= reach; dc++ {
			sq := grid.Square{Col: origin.Col + dc, Row: origin.Row + dr}
			if d := grid.RangeFt(origin, sq); d <= w.ringFt() && d > w.ringFt()-grid.FeetPerSquare {
				out = append(out, sq)
			}
		}
	}
	return out
}

// laid is the squares of the wall on the map: the ones off the map and the walls of the map are left out and, in a
// straight wall, the first of them ends it. The ring is cut like any area, by the line of effect from its center.
func (w wallSpec) laid(t grid.Terrain, origin grid.Square) []grid.Square {
	raw := w.squares(origin)
	open := t.OpenArea(origin, raw, false)
	if w.Ring {
		return open
	}
	for i, sq := range raw {
		if !slices.Contains(open, sq) {
			return raw[:i]
		}
	}
	return raw
}

// reach is the squares of the wall and the squares within 10 feet of it that lie on the damaging side.
//
// Of a straight wall the side is the sign of a cross product.
// It is the direction of the wall crossed with the way from the point of origin to the square.
// Rows grow downward on the map, so a wall that runs east (dx 1, dy 0) has its left to the north.
// The squares of the row above it have a negative cross product, which is LEFT.
// The squares below it have a positive one, which is RIGHT.
// A square level with the wall, beyond its ends, is on neither side.
//
// Of a ring, INNER is the squares closer to the point of origin than the ring.
// OUTER is the squares farther.
func (w wallSpec) reach(origin grid.Square, wall []grid.Square) map[grid.Square]bool {
	out := make(map[grid.Square]bool, len(wall)*3)
	span := wallReachFt / grid.FeetPerSquare
	for _, sq := range wall {
		out[sq] = true
		for dr := -span; dr <= span; dr++ {
			for dc := -span; dc <= span; dc++ {
				near := grid.Square{Col: sq.Col + dc, Row: sq.Row + dr}
				if grid.RangeFt(sq, near) <= wallReachFt && w.onDamagingSide(origin, near) {
					out[near] = true
				}
			}
		}
	}
	return out
}

// onDamagingSide says whether the square lies on the side of the wall that deals damage.
func (w wallSpec) onDamagingSide(origin, sq grid.Square) bool {
	if w.Ring {
		inside := grid.RangeFt(origin, sq) < w.ringFt()
		return inside == (w.Side == playv1.WallSide_WALL_SIDE_INNER)
	}
	cross := w.Dir.Dx*(sq.Row-origin.Row) - w.Dir.Dy*(sq.Col-origin.Col)
	if w.Side == playv1.WallSide_WALL_SIDE_LEFT {
		return cross < 0
	}
	return cross > 0
}

// burning is the squares of the damaging side the wall's own squares do not take, row by row from the top.
func (z *zone) burning() []grid.Square {
	if z.wall == nil {
		return nil
	}
	return z.wall.burning(z.origin(), z.squares)
}

// burning is burning for a wall laid as these squares from the origin.
func (w wallSpec) burning(origin grid.Square, wall []grid.Square) []grid.Square {
	own := make(map[grid.Square]bool, len(wall))
	for _, sq := range wall {
		own[sq] = true
	}
	var out []grid.Square
	for sq := range w.reach(origin, wall) {
		if !own[sq] {
			out = append(out, sq)
		}
	}
	slices.SortFunc(out, func(a, b grid.Square) int {
		if a.Row != b.Row {
			return a.Row - b.Row
		}
		return a.Col - b.Col
	})
	return out
}

// hurtsAtEnd says whether the square is one the damaging side of the wall reaches.
func (z *zone) hurtsAtEnd(sq grid.Square) bool { return z.reach[sq] }

// wallStored are the columns of a zone row that keep the wall.
type wallStored struct {
	side     string
	dx, dy   int16
	lengthFt int32
}

// storedWall is the wall as the zone row keeps it; the zero value for a zone that is no wall.
func storedWall(w *wallSpec) wallStored {
	if w == nil {
		return wallStored{}
	}
	return wallStored{
		side: wallSideNames[w.Side], dx: int16(w.Dir.Dx), dy: int16(w.Dir.Dy), //nolint:gosec // -1 to 1
		lengthFt: clamp32(w.LengthFt, 0, math.MaxInt32),
	}
}
