package grid

import "slices"

// The area of a spell (SRD 5.1, "Areas of Effect"). A shape is drawn on the grid
// from its point of origin, and a square is inside it when the square's center
// is (a creature standing on a square is in the area when its square is).
//
//   - A sphere and a cylinder are centered on a point, which is a square the
//     caster chooses, and the origin is inside it. A cylinder's height does not
//     matter on a flat map, so it is a circle like the sphere's. The radius is
//     the shape's size.
//   - A cone and a line begin at the caster, who picks a direction; the caster's
//     own square is not inside (the SRD's origin is inside only if the caster
//     says so). A cone is as wide at a point as that point is far from the origin
//     (SRD): at two squares it is two squares wide. A line is as long as the
//     size and as wide as the width (5 ft unless a spell says another).
//   - A cube has its origin on a face; from the caster it touches the caster's
//     square by a face (a straight direction) or by a corner (a diagonal one), and
//     it is size × size.
//
// The directions are the eight a grid has (each 45 degrees), not an angle: the
// SRD does not say how to lay a cone or a line on squares, so a table needs one
// way that is the same on every device. Distances are exact (the squares are
// counted between centers, a diagonal is longer), unlike the range of a spell,
// which RangeFt rounds down (RN-21), so that a 4-square sphere does not grow by a
// square.

// Shape is the form of an area, as the spell data names it.
type Shape string

// The five forms of an area of effect.
const (
	ShapeSphere   Shape = "sphere"
	ShapeCylinder Shape = "cylinder"
	ShapeCone     Shape = "cone"
	ShapeLine     Shape = "line"
	ShapeCube     Shape = "cube"
)

// Direction is one of the eight directions a cone, a line or a cube points:
// Dx is -1 (west), 0 or 1 (east), Dy is -1 (north), 0 or 1 (south), and not both
// are 0.
type Direction struct {
	Dx, Dy int
}

// Valid says whether it is one of the eight directions.
func (d Direction) Valid() bool {
	return d.Dx >= -1 && d.Dx <= 1 && d.Dy >= -1 && d.Dy <= 1 && (d.Dx != 0 || d.Dy != 0)
}

// diagonal says the direction runs between two axes.
func (d Direction) diagonal() bool { return d.Dx != 0 && d.Dy != 0 }

// Toward is the direction, among the eight, that a square lies in as seen from
// another: the one with the smallest angle to the straight line between their
// centers. It is false when they are the same square. The cut between a straight
// direction and a diagonal one is at 22.5 degrees, worked out with integers.
func Toward(from, to Square) (Direction, bool) {
	dc, dr := to.Col-from.Col, to.Row-from.Row
	if dc == 0 && dr == 0 {
		return Direction{}, false
	}
	// tan(22.5°) is 0.4142...: the axis wins while the other component is at most
	// 5/12 of it (0.4167, a hair over), which keeps the math in integers.
	ac, ar := abs(dc), abs(dr)
	switch {
	case 12*ar <= 5*ac:
		return Direction{Dx: sign(dc)}, true
	case 12*ac <= 5*ar:
		return Direction{Dy: sign(dr)}, true
	}
	return Direction{Dx: sign(dc), Dy: sign(dr)}, true
}

func abs(n int) int {
	if n < 0 {
		return -n
	}
	return n
}

func sign(n int) int {
	switch {
	case n < 0:
		return -1
	case n > 0:
		return 1
	}
	return 0
}

// Area is the form and the size of a spell's area.
type Area struct {
	Shape Shape
	// SizeFt is the sphere's and the cylinder's radius, the cone's and the line's
	// length and the cube's side, in feet, a multiple of 5.
	SizeFt int
	// WidthFt is a line's width; 0 is 5 ft.
	WidthFt int
}

// FromPoint is the squares of a sphere or a cylinder centered on the origin
// square, the origin included, before the walls are looked at. It is nothing for
// any other shape.
func (a Area) FromPoint(origin Square) []Square {
	if a.Shape != ShapeSphere && a.Shape != ShapeCylinder {
		return nil
	}
	n := a.SizeFt / FeetPerSquare
	var out []Square
	for dr := -n; dr <= n; dr++ {
		for dc := -n; dc <= n; dc++ {
			if dc*dc+dr*dr <= n*n {
				out = append(out, Square{Col: origin.Col + dc, Row: origin.Row + dr})
			}
		}
	}
	return out
}

// FromCaster is the squares of a cone, a line or a cube that comes out of the
// caster's square in a direction, the caster's own square left out, before the
// walls are looked at. It is nothing for a sphere or a cylinder, and for a
// direction that is not one of the eight.
func (a Area) FromCaster(caster Square, d Direction) []Square {
	if !d.Valid() {
		return nil
	}
	n := a.SizeFt / FeetPerSquare
	switch a.Shape {
	case ShapeCone, ShapeLine:
		return a.ray(caster, d, n)
	case ShapeCube:
		return cube(caster, d, n)
	}
	return nil
}

// ray is the squares of a cone or a line. With the axis along the direction, a is
// how far a square's center is along it from the caster and p how far to the side;
// both are scaled by the square root of 2 for a diagonal (as A and P), so that
// everything compares as integers: a straight direction has A = a and P = p, a
// diagonal one A = a·√2 and P = p·√2.
func (a Area) ray(caster Square, d Direction, n int) []Square {
	w := max(a.WidthFt/FeetPerSquare, 1)
	var out []Square
	for dr := -n; dr <= n; dr++ {
		for dc := -n; dc <= n; dc++ {
			along := d.Dx*dc + d.Dy*dr // A
			side := d.Dy*dc - d.Dx*dr  // P
			if along <= 0 || !a.within(d, along, side, n, w) {
				continue
			}
			out = append(out, Square{Col: caster.Col + dc, Row: caster.Row + dr})
		}
	}
	return out
}

// within is the test of a cone or a line for a square at A and P (see ray): not
// farther than the length n, and inside the cone's width at that distance or the
// line's strip. The cone includes its edge; the strip of a line with an even width
// has no center, and takes the extra square on the side of its right hand (the
// SRD does not say).
func (a Area) within(d Direction, along, side, n, w int) bool {
	if d.diagonal() {
		if along*along > 2*n*n {
			return false
		}
	} else if along > n {
		return false
	}
	if a.Shape == ShapeCone {
		return 2*abs(side) <= along
	}
	if d.diagonal() {
		if side >= 0 {
			return 2*side*side <= w*w
		}
		return 2*side*side < w*w
	}
	return -w < 2*side && 2*side <= w
}

// cube is the n × n squares that touch the caster's square by a face (a straight
// direction) or by a corner (a diagonal one). On the other axis of a straight
// direction the cube is centered on the caster's row or column; with an even side
// the extra square is on the larger side.
func cube(caster Square, d Direction, n int) []Square {
	span := func(dir int) (lo, hi int) {
		switch {
		case dir > 0:
			return 1, n
		case dir < 0:
			return -n, -1
		}
		lo = -((n - 1) / 2)
		return lo, lo + n - 1
	}
	c0, c1 := span(d.Dx)
	r0, r1 := span(d.Dy)
	out := make([]Square, 0, n*n)
	for dr := r0; dr <= r1; dr++ {
		for dc := c0; dc <= c1; dc++ {
			out = append(out, Square{Col: caster.Col + dc, Row: caster.Row + dr})
		}
	}
	return out
}

// OpenArea is the part of an area that its origin reaches (SRD, "Areas of
// Effect": a location is outside the area if no unblocked straight line runs
// from the origin to it, and only total cover blocks). squares are the shape's
// squares; the ones off the grid, the walls and the ones behind a wall are left
// out, in row-major order. The origin's own square is in when the shape has it.
//
// With corners, the area is the part of the shape connected to the origin through
// open squares instead (a spell that "spreads around corners", Fireball): the
// fire goes around a corner a straight line cannot, but not between two walls that
// touch at a corner, and not through a wall.
func (t Terrain) OpenArea(origin Square, squares []Square, corners bool) []Square {
	if !t.Grid.Contains(origin) || t.wall(origin) {
		return nil
	}
	inside := make(map[Square]bool, len(squares))
	for _, s := range squares {
		if t.Grid.Contains(s) && !t.wall(s) {
			inside[s] = true
		}
	}
	var out []Square
	if corners {
		out = t.spread(origin, inside)
	} else {
		for s := range inside {
			if s == origin || t.CoverBetween(origin, s, nil) != CoverTotal {
				out = append(out, s)
			}
		}
	}
	slices.SortFunc(out, func(a, b Square) int {
		if a.Row != b.Row {
			return a.Row - b.Row
		}
		return a.Col - b.Col
	})
	return out
}

// spread is the squares of the set connected to the origin by steps between
// neighbors (eight ways, none squeezing between two walls).
func (t Terrain) spread(origin Square, inside map[Square]bool) []Square {
	if !inside[origin] {
		return nil
	}
	seen := map[Square]bool{origin: true}
	queue := []Square{origin}
	for len(queue) > 0 {
		cur := queue[0]
		queue = append(queue[1:], t.spreadStep(cur, inside, seen)...)
	}
	out := make([]Square, 0, len(seen))
	for s := range seen {
		out = append(out, s)
	}
	return out
}

// spreadStep is the neighbors of cur the spread reaches for the first time,
// in order (row by row), marked as seen.
func (t Terrain) spreadStep(cur Square, inside, seen map[Square]bool) []Square {
	var next []Square
	for dr := -1; dr <= 1; dr++ {
		for dc := -1; dc <= 1; dc++ {
			n := Square{Col: cur.Col + dc, Row: cur.Row + dr}
			if (dc == 0 && dr == 0) || !inside[n] || seen[n] || t.cornerSqueeze(cur, dc, dr) {
				continue
			}
			seen[n] = true
			next = append(next, n)
		}
	}
	return next
}

// cornerSqueeze says whether the diagonal step from cur squeezes between two
// walls that touch at a corner.
func (t Terrain) cornerSqueeze(cur Square, dc, dr int) bool {
	return dc != 0 && dr != 0 && t.wall(Square{Col: cur.Col + dc, Row: cur.Row}) && t.wall(Square{Col: cur.Col, Row: cur.Row + dr})
}

// NearSide is the point of origin of an area its caster places at a point they
// cannot see because a wall is in the way (SRD, "Targets"): it comes into being on
// the near side of the obstruction, the last square the straight line from the
// caster reaches before the wall. A point with a clear line stays as it is.
func (t Terrain) NearSide(caster, point Square) Square {
	prev := caster
	for _, s := range Line(caster, point) {
		if (s.Corner && t.wall(s.SideA) && t.wall(s.SideB)) || t.wall(s.Square) {
			return prev
		}
		prev = s.Square
	}
	return point
}
