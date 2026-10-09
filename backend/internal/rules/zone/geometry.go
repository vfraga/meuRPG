package zone

import (
	"slices"

	"github.com/PuraFome/meuRPG/backend/internal/rules/grid"
)

// Shape returns the squares a zone of the spec covers before the walls of the map
// are looked at: the shape set on the point (or beginning at it, running in a
// direction, for a wall), sizeFt big. The caller cuts it with the line of effect
// (grid.Terrain.OpenArea). A shape the placement cannot make, or a size below a
// square, gives nothing.
//
// The Moonbeam cylinder of 5 ft radius is the point and the four squares next to it
// (a square is inside when its center is within the radius, SRD "Areas of Effect"
// laid on squares by package grid); the app keeps that reading on purpose.
func Shape(spec Spec, sizeFt int, origin grid.Square, dir *grid.Direction) []grid.Square {
	switch spec.Shape {
	case grid.ShapeSphere, grid.ShapeCylinder:
		return grid.Area{Shape: spec.Shape, SizeFt: sizeFt}.FromPoint(origin)
	case grid.ShapeCube:
		return grid.CubeAt(origin, sizeFt)
	case grid.ShapeWall:
		if dir == nil {
			return nil
		}
		return grid.WallFrom(origin, *dir, sizeFt)
	case grid.ShapeRing:
		return grid.RingAt(origin, sizeFt)
	}
	return nil
}

// Contains says whether the square is one of the cells.
func Contains(cells []grid.Square, sq grid.Square) bool { return slices.Contains(cells, sq) }

// Overlap says how a creature's footprint (the squares it occupies) lies in the
// zone: any of them inside, and all of them inside. A creature larger than a square
// is in the zone when any of its squares is, and entirely inside when all are. An
// empty footprint is in nothing.
func Overlap(cells, footprint []grid.Square) (any, all bool) {
	if len(footprint) == 0 {
		return false, false
	}
	all = true
	for _, sq := range footprint {
		if slices.Contains(cells, sq) {
			any = true
		} else {
			all = false
		}
	}
	return any, all
}

// Side is the side of a wall: A is the left of the direction it runs, B the right (seen
// from above, with rows counting down); for a ring, A is the inside and B the outside.
type Side string

// The sides of a wall.
const (
	SideNone Side = ""
	SideA    Side = "a"
	SideB    Side = "b"
)

// Valid says whether it is a side or none.
func (s Side) Valid() bool { return s == SideNone || s == SideA || s == SideB }

// Wall is a wall of a zone: the squares it covers and how it is laid, enough to say
// which side of it a square is on.
type Wall struct {
	Cells []grid.Square
	// Dir is the direction a straight wall runs in, nil for a ring.
	Dir *grid.Direction
	// Center is a ring's center.
	Center grid.Square
	// Radius is the ring's radius in squares (the diameter's half).
	Radius int
	Ring   bool
}

// SideOf says which side of the wall a square lies on, and false for a square on the
// wall's line (neither side). For a straight wall the side is the sign of the cross
// product of the direction with the vector from the nearest wall square to the square:
// a square straight ahead or behind has none.
func (w Wall) SideOf(sq grid.Square) (Side, bool) {
	if w.Ring {
		// The hollow is what RingAt leaves out: the squares within a square less than the
		// loop's radius of the center.
		dc, dr := sq.Col-w.Center.Col, sq.Row-w.Center.Row
		inner := w.Radius - 1
		switch d2 := dc*dc + dr*dr; {
		case slices.Contains(w.Cells, sq):
			return SideNone, false
		case d2 <= inner*inner:
			return SideA, true
		default:
			return SideB, true
		}
	}
	if w.Dir == nil {
		return SideNone, false
	}
	near, ok := grid.Nearest(w.Cells, sq)
	if !ok {
		return SideNone, false
	}
	dc, dr := sq.Col-near.Col, sq.Row-near.Row
	// A is the left of the direction: with rows counting down, left of (dx, dy) is
	// (dy, -dx).
	switch cross := w.Dir.Dx*dr - w.Dir.Dy*dc; {
	case cross < 0:
		return SideA, true
	case cross > 0:
		return SideB, true
	}
	return SideNone, false
}

// Within says whether the square is within reachFt feet of the damaging side of the
// wall, or inside the wall itself: where Wall of Fire burns a creature that ends its
// turn ("within 10 feet of that side or inside the wall"). The distance is the one a
// spell's range uses (grid.RangeFt, squares between the centers).
func (w Wall) Within(sq grid.Square, side Side, reachFt int) bool {
	if slices.Contains(w.Cells, sq) {
		return true
	}
	if side == SideNone {
		return false
	}
	if s, ok := w.SideOf(sq); !ok || s != side {
		return false
	}
	near, ok := grid.Nearest(w.Cells, sq)
	return ok && grid.RangeFt(near, sq) <= reachFt
}

// Blocks says whether the straight line between two squares passes through a cell of
// a sight-blocking zone, or either end is in one: nobody sees into, out of or through
// it. A creature sees itself, so a line from a square to itself is never blocked.
func Blocks(blocking func(grid.Square) bool, from, to grid.Square) bool {
	if from == to {
		return false
	}
	if blocking(from) || blocking(to) {
		return true
	}
	for _, step := range grid.Line(from, to) {
		if step.Square != to && blocking(step.Square) {
			return true
		}
	}
	return false
}

// Crossed counts the squares a straight move from one square to another enters
// whose cells are in the zone, the start not counted (the move's own steps: the
// zone's damage for every 5 ft travelled in it, Spike Growth).
func Crossed(cells []grid.Square, from, to grid.Square) int {
	n := 0
	for _, step := range grid.Line(from, to) {
		if slices.Contains(cells, step.Square) {
			n++
		}
	}
	return n
}
