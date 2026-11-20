package zonewall

import (
	"slices"
	"testing"

	playv1 "github.com/PuraFome/meuRPG/backend/gen/meurpg/play/v1"
	"github.com/PuraFome/meuRPG/backend/internal/rules/grid"
)

func wsq(col, row int) grid.Square { return grid.Square{Col: col, Row: row} }

func TestWallStraightSquaresFollowTheMapsDistance(t *testing.T) {
	t.Parallel()
	for _, tt := range []struct {
		name   string
		dir    grid.Direction
		length int
		want   int
	}{
		{"orthogonal 60 ft", grid.Direction{Dx: 1}, 60, 12},
		// The map rounds a distance down, so the diagonal wall of 60 ft is 9 squares: fewer than the 12 of a row.
		{"diagonal 60 ft", grid.Direction{Dx: 1, Dy: 1}, 60, 9},
		{"25 ft", grid.Direction{Dy: -1}, 25, 5},
		{"5 ft", grid.Direction{Dx: -1}, 5, 1},
	} {
		w := Spec{Dir: tt.dir, LengthFt: tt.length, Side: playv1.WallSide_WALL_SIDE_LEFT}
		got := w.Squares(wsq(30, 30))
		if len(got) != tt.want {
			t.Errorf("%s: %d squares, want %d", tt.name, len(got), tt.want)
		}
		if got[0] != wsq(30, 30) {
			t.Errorf("%s: first square %v, want the point of origin", tt.name, got[0])
		}
	}
}

func TestWallRingSquares(t *testing.T) {
	t.Parallel()
	origin := wsq(10, 10)
	for _, tt := range []struct {
		diameter, want, atFt int
	}{{20, 16, 10}, {10, 8, 5}, {5, 8, 5}} {
		w := Spec{Ring: true, LengthFt: tt.diameter, Side: playv1.WallSide_WALL_SIDE_INNER}
		got := w.Squares(origin)
		if len(got) != tt.want {
			t.Errorf("diameter %d: %d squares, want %d", tt.diameter, len(got), tt.want)
		}
		for _, s := range got {
			if grid.RangeFt(origin, s) != tt.atFt {
				t.Errorf("diameter %d: %v is %d ft out, want %d", tt.diameter, s, grid.RangeFt(origin, s), tt.atFt)
			}
		}
	}
}

func TestWallIsCutByTheTerrain(t *testing.T) {
	t.Parallel()
	g := grid.Grid{Columns: 20, Rows: 20}
	terrain := grid.Terrain{Grid: g, Walls: grid.NewLayer(g)}
	terrain.Walls.Set(13, 5, true)
	w := Spec{Dir: grid.Direction{Dx: 1}, LengthFt: 60, Side: playv1.WallSide_WALL_SIDE_LEFT}
	got := w.Laid(terrain, wsq(10, 5))
	if want := []grid.Square{wsq(10, 5), wsq(11, 5), wsq(12, 5)}; !slices.Equal(got, want) {
		t.Errorf("laid = %v, want %v: a wall of the map ends the wall", got, want)
	}
}

func wallReachOf(w Spec, origin grid.Square) map[grid.Square]bool {
	return w.Reach(origin, w.Squares(origin))
}

// For a wall that runs east the left is north (rows grow downward), within 10 ft, and the wall itself hurts from both.
func TestWallReachOfAStraightWallIsTheDamagingSide(t *testing.T) {
	t.Parallel()
	origin := wsq(10, 10)
	left := wallReachOf(Spec{Dir: grid.Direction{Dx: 1}, LengthFt: 15, Side: playv1.WallSide_WALL_SIDE_LEFT}, origin)
	right := wallReachOf(Spec{Dir: grid.Direction{Dx: 1}, LengthFt: 15, Side: playv1.WallSide_WALL_SIDE_RIGHT}, origin)
	for _, s := range []grid.Square{wsq(10, 10), wsq(11, 10), wsq(12, 10)} {
		if !left[s] || !right[s] {
			t.Errorf("%v is a square of the wall: it hurts from both sides", s)
		}
	}
	for _, c := range []struct {
		s           grid.Square
		left, right bool
	}{
		{wsq(11, 9), true, false},
		{wsq(11, 8), true, false},
		{wsq(11, 7), false, false},
		{wsq(11, 11), false, true},
		{wsq(11, 12), false, true},
		{wsq(11, 13), false, false},
		{wsq(13, 9), true, false},
		{wsq(14, 10), false, false},
		{wsq(9, 10), false, false},
	} {
		if left[c.s] != c.left || right[c.s] != c.right {
			t.Errorf("%v: left %v right %v, want %v and %v", c.s, left[c.s], right[c.s], c.left, c.right)
		}
	}
	north := wallReachOf(Spec{Dir: grid.Direction{Dy: -1}, LengthFt: 15, Side: playv1.WallSide_WALL_SIDE_LEFT}, origin)
	if !north[wsq(9, 9)] || north[wsq(11, 9)] {
		t.Error("walking north, the left is west")
	}
}

func TestWallReachOfARingIsInsideOrOutside(t *testing.T) {
	t.Parallel()
	origin := wsq(10, 10)
	inner := wallReachOf(Spec{Ring: true, LengthFt: 20, Side: playv1.WallSide_WALL_SIDE_INNER}, origin)
	outer := wallReachOf(Spec{Ring: true, LengthFt: 20, Side: playv1.WallSide_WALL_SIDE_OUTER}, origin)
	for _, s := range []grid.Square{wsq(12, 10), wsq(10, 8), wsq(12, 12)} {
		if !inner[s] || !outer[s] {
			t.Errorf("%v is a square of the ring: it hurts from both sides", s)
		}
	}
	for _, c := range []struct {
		s            grid.Square
		inner, outer bool
	}{
		{wsq(10, 10), true, false},
		{wsq(11, 10), true, false},
		{wsq(11, 11), true, false},
		{wsq(13, 10), false, true},
		{wsq(14, 10), false, true},
		{wsq(15, 10), false, false},
	} {
		if inner[c.s] != c.inner || outer[c.s] != c.outer {
			t.Errorf("%v: inner %v outer %v, want %v and %v", c.s, inner[c.s], outer[c.s], c.inner, c.outer)
		}
	}
}

func TestAWallWithNoDirectionLaysNothingInTheGeometry(t *testing.T) {
	t.Parallel()
	if got := (Spec{LengthFt: 60}).Squares(wsq(5, 5)); len(got) != 0 {
		t.Errorf("squares = %v, want none for a wall that runs nowhere", got)
	}
}
