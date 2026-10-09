package grid

import "slices"

// The shapes a spell leaves on the map for its whole duration (SRD 5.1, "Areas of
// Effect"): a sphere or a cylinder centered on a point is Area.FromPoint, and these
// are the other ones, laid on squares with the same rule: a square is inside when
// its center is.
//
//   - A cube set at a point (Web, Grease, Entangle: "a 20-foot cube from that point",
//     "a 10-foot square centered on a point") is size × size squares around the
//     point. With an odd side the point is the middle square; with an even side the
//     point is one of the four middle squares and the extra row and column fall on
//     the larger side, the same rule a cube that comes out of a caster uses on the
//     axis it is centered on (cube).
//   - A wall (Wall of Fire, "up to 60 feet long") begins at the point and runs in one
//     of the eight directions, one square wide.
//   - A ring (the same wall, "a ringed wall up to 20 feet in diameter") is the closed
//     loop of squares around the point, at 10 ft from its center.

// ShapeRing is a wall bent into a circle around a point.
const ShapeRing Shape = "ring"

// ShapeWall is a straight wall of one square width that begins at a point and runs
// in a direction.
const ShapeWall Shape = "wall"

// CubeAt is the squares of a cube of sizeFt on a side set at the point, before the
// walls are looked at.
func CubeAt(origin Square, sizeFt int) []Square {
	n := sizeFt / FeetPerSquare
	if n < 1 {
		return nil
	}
	lo := -((n - 1) / 2)
	out := make([]Square, 0, n*n)
	for dr := lo; dr < lo+n; dr++ {
		for dc := lo; dc < lo+n; dc++ {
			out = append(out, Square{Col: origin.Col + dc, Row: origin.Row + dr})
		}
	}
	return out
}

// WallFrom is the squares of a wall of lengthFt that begins on the origin square and
// runs in the direction, the origin included, before the walls of the map are looked
// at. A diagonal wall steps from corner to corner, one square for each 5 ft.
func WallFrom(origin Square, d Direction, lengthFt int) []Square {
	n := lengthFt / FeetPerSquare
	if n < 1 || !d.Valid() {
		return nil
	}
	out := make([]Square, 0, n)
	for i := 0; i < n; i++ {
		out = append(out, Square{Col: origin.Col + d.Dx*i, Row: origin.Row + d.Dy*i})
	}
	return out
}

// RingAt is the squares of a ringed wall of diameterFt around the center square: the
// squares whose centers are within the radius, without the ones within 1 square of
// the center, which are the hollow. A diameter of 20 ft makes the loop of 8 squares
// at 2 squares from the center (orthogonal) and 1 (diagonal).
func RingAt(center Square, diameterFt int) []Square {
	r := diameterFt / FeetPerSquare / 2
	if r < 2 {
		return nil
	}
	var out []Square
	for dr := -r; dr <= r; dr++ {
		for dc := -r; dc <= r; dc++ {
			d2 := dc*dc + dr*dr
			if d2 <= r*r && d2 > (r-1)*(r-1) {
				out = append(out, Square{Col: center.Col + dc, Row: center.Row + dr})
			}
		}
	}
	return out
}

// Nearest is the square of the list closest to the target (the distance RangeSquares
// gives), the first in the list when several tie; false for an empty list.
func Nearest(squares []Square, to Square) (Square, bool) {
	best, bestD, ok := Square{}, 0, false
	for _, s := range squares {
		if d := RangeSquares(s, to); !ok || d < bestD {
			best, bestD, ok = s, d, true
		}
	}
	return best, ok
}

// Sorted returns the squares in row-major order without repeats, the order every
// stored area uses.
func Sorted(squares []Square) []Square {
	out := slices.Clone(squares)
	slices.SortFunc(out, func(a, b Square) int {
		if a.Row != b.Row {
			return a.Row - b.Row
		}
		return a.Col - b.Col
	})
	return slices.Compact(out)
}
