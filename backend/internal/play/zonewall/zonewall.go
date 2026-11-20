// Package zonewall holds the geometry of a wall of fire (Wall of Fire, SRD 5.1) that needs no store, no transport and no
// Service: the squares a wall takes from its point of origin and the squares its damaging side reaches. play reads the
// request, keeps the zone row and calls here.
package zonewall

import (
	"slices"

	playv1 "github.com/PuraFome/meuRPG/backend/gen/meurpg/play/v1"
	"github.com/PuraFome/meuRPG/backend/internal/rules/grid"
)

// A wall (Wall of Fire, SRD 5.1) is a zone whose squares are a line or a ring laid from the point of origin.
// They are not a shape of the spell's area.
// A creature is inside the wall when its square is one of them.

// LineFt and RingFt are the largest a wall is, in feet: a straight wall's length and a ring's diameter.
const (
	LineFt = 60
	RingFt = 20
)

// ReachFt is how far from the damaging side the wall hurts.
const ReachFt = 10

// Spec is a wall as the caster laid it.
type Spec struct {
	// Ring says the wall is a ring around the point of origin; otherwise it is a straight wall from it.
	Ring bool `json:"ring,omitempty"`
	// Dir is the way a straight wall runs from the point of origin; unset for a ring.
	Dir grid.Direction `json:"dir"`
	// LengthFt is the straight wall's length or the ring's diameter, never 0.
	LengthFt int `json:"length_ft"`
	// Side is the side that deals the damage: left or right of a straight wall, inner or outer of a ring.
	Side playv1.WallSide `json:"side"`
}

// sideNames are the names the zone row keeps for the sides (FromRow reads them back).
var sideNames = map[playv1.WallSide]string{
	playv1.WallSide_WALL_SIDE_LEFT:  "left",
	playv1.WallSide_WALL_SIDE_RIGHT: "right",
	playv1.WallSide_WALL_SIDE_INNER: "inner",
	playv1.WallSide_WALL_SIDE_OUTER: "outer",
}

// SideName is the name the zone row keeps for a side of the wall.
func SideName(s playv1.WallSide) string { return sideNames[s] }

// FromRow rebuilds the wall a zone row keeps; nil for a zone that is no wall.
func FromRow(side string, dx, dy int16, lengthFt int32) *Spec {
	w := &Spec{Dir: grid.Direction{Dx: int(dx), Dy: int(dy)}, LengthFt: int(lengthFt)}
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
func (w Spec) ringFt() int {
	return (w.LengthFt/2 + grid.FeetPerSquare - 1) / grid.FeetPerSquare * grid.FeetPerSquare
}

// Shape is the shape and the size the zone view tells: a line of the wall's length, or a sphere of the ring's radius.
func (w Spec) Shape() (grid.Shape, int) {
	if w.Ring {
		return grid.ShapeSphere, w.ringFt()
	}
	return grid.ShapeLine, w.LengthFt
}

// Squares are the squares of the wall laid from the point of origin, before the terrain is looked at.
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
func (w Spec) Squares(origin grid.Square) []grid.Square {
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

func (w Spec) ringSquares(origin grid.Square) []grid.Square {
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

// Laid is the squares of the wall on the map: the ones off the map and the walls of the map are left out and, in a
// straight wall, the first of them ends it. The ring is cut like any area, by the line of effect from its center.
func (w Spec) Laid(t grid.Terrain, origin grid.Square) []grid.Square {
	raw := w.Squares(origin)
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

// Reach is the squares of the wall and the squares within 10 feet of it that lie on the damaging side.
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
func (w Spec) Reach(origin grid.Square, wall []grid.Square) map[grid.Square]bool {
	out := make(map[grid.Square]bool, len(wall)*3)
	span := ReachFt / grid.FeetPerSquare
	for _, sq := range wall {
		out[sq] = true
		for dr := -span; dr <= span; dr++ {
			for dc := -span; dc <= span; dc++ {
				near := grid.Square{Col: sq.Col + dc, Row: sq.Row + dr}
				if grid.RangeFt(sq, near) <= ReachFt && w.onDamagingSide(origin, near) {
					out[near] = true
				}
			}
		}
	}
	return out
}

// onDamagingSide says whether the square lies on the side of the wall that deals damage.
func (w Spec) onDamagingSide(origin, sq grid.Square) bool {
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

// Burning is the squares of the damaging side the wall's own squares do not take, row by row from the top, for a wall laid as
// these squares from the origin.
func (w Spec) Burning(origin grid.Square, wall []grid.Square) []grid.Square {
	own := make(map[grid.Square]bool, len(wall))
	for _, sq := range wall {
		own[sq] = true
	}
	var out []grid.Square
	for sq := range w.Reach(origin, wall) {
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
