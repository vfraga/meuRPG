package grid_test

import (
	"testing"

	"github.com/PuraFome/meuRPG/backend/internal/rules/grid"
)

func sq(c, r int) grid.Square { return grid.Square{Col: c, Row: r} }

func TestCreaturesOnTheWay(t *testing.T) {
	t.Parallel()
	tr := cave()
	from, to := sq(10, 7), sq(13, 7)
	friend := func(s grid.Size) grid.OccupantMap { return grid.OccupantMap{sq(12, 7): {Size: s}} }
	foe := func(s grid.Size) grid.OccupantMap { return grid.OccupantMap{sq(12, 7): {Size: s, Hostile: true}} }
	medium := grid.Mover{Size: grid.SizeMedium}

	// A creature that is not hostile can be passed, and its square costs 5 ft more.
	if mv := tr.Move(from, to, friend(grid.SizeMedium), medium); mv.Blocked || mv.CostDFt != 200 {
		t.Errorf("passing a friend: %+v, want cost 200", mv)
	}
	// An unset size counts as Medium.
	if mv := tr.Move(from, to, friend(0), grid.Mover{}); mv.Blocked || mv.CostDFt != 200 {
		t.Errorf("an unset size: %+v", mv)
	}
	// A hostile creature can't be passed, unless the two are two sizes apart.
	cases := []struct {
		mover, foe grid.Size
		passes     bool
	}{
		{grid.SizeMedium, grid.SizeMedium, false},
		{grid.SizeMedium, grid.SizeLarge, false},
		{grid.SizeMedium, grid.SizeSmall, false},
		{grid.SizeMedium, grid.SizeHuge, true}, // two larger
		{grid.SizeMedium, grid.SizeTiny, true}, // two smaller
		{grid.SizeLarge, grid.SizeSmall, true},
		{grid.SizeSmall, grid.SizeLarge, true},
		{grid.SizeTiny, grid.SizeTiny, false},
		{grid.SizeGargantuan, grid.SizeHuge, false},
	}
	for _, c := range cases {
		mv := tr.Move(from, to, foe(c.foe), grid.Mover{Size: c.mover})
		if mv.Blocked == c.passes {
			t.Errorf("mover %d past a hostile %d: blocked=%v, want %v", c.mover, c.foe, mv.Blocked, !c.passes)
		}
		if c.passes && mv.CostDFt != 200 {
			t.Errorf("passing a hostile creature costs as difficult terrain, got %d", mv.CostDFt)
		}
		if !c.passes && mv.By != grid.StopOccupied {
			t.Errorf("blocked by a creature must say so, got %q", mv.By)
		}
	}

	// No move ends in another creature's square, even a friend's or one it could pass.
	if mv := tr.Move(from, to, grid.OccupantMap{to: {Size: grid.SizeMedium}}, medium); !mv.Blocked || mv.By != grid.StopOccupied {
		t.Errorf("ending in a friend's square: %+v", mv)
	}
	if mv := tr.Move(from, to, grid.OccupantMap{to: {Size: grid.SizeTiny, Hostile: true}}, medium); !mv.Blocked {
		t.Errorf("ending in a hostile Tiny square: %+v", mv)
	}
	// Except a Tiny creature in a square that holds only Tiny creatures.
	tiny := grid.Mover{Size: grid.SizeTiny}
	if mv := tr.Move(from, to, grid.OccupantMap{to: {Size: grid.SizeTiny}}, tiny); mv.Blocked || mv.CostDFt != 200 {
		t.Errorf("a Tiny creature sharing with a Tiny friend: %+v", mv)
	}
	if mv := tr.Move(from, to, grid.OccupantMap{to: {Size: grid.SizeSmall}}, tiny); !mv.Blocked {
		t.Errorf("a Tiny creature may not share with a Small one: %+v", mv)
	}
	if mv := tr.Move(from, to, grid.OccupantMap{to: {Size: grid.SizeTiny, Hostile: true}}, tiny); !mv.Blocked {
		t.Errorf("a Tiny creature may not share with a hostile Tiny one (it can't even pass): %+v", mv)
	}
	// Reach leaves those squares out, and lists a Tiny creature's shared square.
	occ := grid.OccupantMap{sq(11, 7): {Size: grid.SizeTiny}}
	if has(tr.Reach(from, 100, occ, medium), sq(11, 7)) {
		t.Error("Reach must not list a square the mover may not end in")
	}
	if !has(tr.Reach(from, 100, occ, tiny), sq(11, 7)) {
		t.Error("Reach must list a square a Tiny creature may share")
	}
}

func has(rs []grid.Reachable, s grid.Square) bool {
	for _, r := range rs {
		if r.Square == s {
			return true
		}
	}
	return false
}

func TestAFlierIgnoresRubbleButNotWalls(t *testing.T) {
	t.Parallel()
	tr := cave()
	from, to := salvia, sq(6, 10) // 33,03 ft on foot: three squares of rubble
	if mv := tr.Move(from, to, nil, grid.Mover{}); mv.CostDFt != 330 {
		t.Fatalf("on foot: %d", mv.CostDFt)
	}
	if mv := tr.Move(from, to, nil, grid.Mover{Flier: true}); mv.Blocked || mv.CostDFt != 180 {
		t.Errorf("a flier pays only the length, got %+v", mv)
	}
	if mv := tr.Move(sq(10, 8), sq(9, 10), nil, grid.Mover{Flier: true}); !mv.Blocked {
		t.Error("walls stop a flier too")
	}
	// The reach of a flier is the whole circle: a 30 ft circle has 69 squares to
	// walk on in the open and the rubble does not shrink it.
	foot := tr.Reach(sq(2, 9), 100, nil, grid.Mover{})
	air := tr.Reach(sq(2, 9), 100, nil, grid.Mover{Flier: true})
	if len(air) <= len(foot) {
		t.Errorf("the rubble must shrink the reach on foot: %d squares on foot, %d flying", len(foot), len(air))
	}
}

func TestMoveCostIsInTenthsOfAFoot(t *testing.T) {
	t.Parallel()
	tr := grid.Terrain{Grid: grid.Grid{Columns: 20, Rows: 20}}
	cases := []struct {
		to   grid.Square
		want int
	}{
		{sq(1, 0), 50}, {sq(1, 1), 71}, {sq(2, 1), 112}, {sq(2, 2), 141}, {sq(3, 4), 250}, {sq(6, 0), 300}, {sq(5, 5), 354},
	}
	for _, c := range cases {
		if mv := tr.Move(sq(0, 0), c.to, nil, grid.Mover{}); mv.CostDFt != c.want {
			t.Errorf("(0,0) to %v costs %d, want %d", c.to, mv.CostDFt, c.want)
		}
	}
	// 30 ft of movement: the circle of 6 squares, 4 squares diagonally (28,3 ft)
	// is in, 5 (35,4 ft) is not.
	reach := tr.Reach(sq(10, 10), 300, nil, grid.Mover{})
	if !has(reach, sq(14, 14)) || has(reach, sq(15, 15)) || !has(reach, sq(16, 10)) || has(reach, sq(17, 10)) {
		t.Error("the reach of 30 ft is the circle of 6 squares")
	}
	if got := tr.Reach(sq(10, 10), 0, nil, grid.Mover{}); got != nil {
		t.Errorf("no movement left reaches nothing, got %v", got)
	}
}

func TestJumpIgnoresDifficultTerrain(t *testing.T) {
	t.Parallel()
	tr := cave()
	from, to := salvia, sq(6, 10)
	if mv := tr.Jump(from, to, nil, grid.Mover{}); mv.Blocked || mv.CostDFt != 180 {
		t.Errorf("a jump over the rubble costs the length only, got %+v", mv)
	}
	if mv := tr.Jump(sq(10, 8), sq(9, 10), nil, grid.Mover{}); !mv.Blocked {
		t.Error("a jump can't cross a wall")
	}
	if mv := tr.Jump(sq(19, 4), sq(21, 4), nil, grid.Mover{}); !mv.Blocked {
		t.Error("a jump can't cross a column")
	}
	// It clears the creatures on the way but needs a free square to land in.
	occ := grid.OccupantMap{sq(4, 9): {Size: grid.SizeMedium, Hostile: true}, sq(6, 10): {Size: grid.SizeMedium}}
	if mv := tr.Jump(from, to, occ, grid.Mover{}); !mv.Blocked {
		t.Error("a jump can't land in a creature's square")
	}
	delete(occ, sq(6, 10))
	if mv := tr.Jump(from, to, occ, grid.Mover{}); mv.Blocked || mv.CostDFt != 180 {
		t.Errorf("a jump clears a hostile creature, got %+v", mv)
	}
}

func TestMoveUntilTheFog(t *testing.T) {
	t.Parallel()
	// The player sees a plain corridor; a wall they don't see is at (9,8), and
	// rubble they don't see at (7,8). The move is planned on what is seen and
	// run on what is there.
	g := grid.Grid{Columns: 20, Rows: 12}
	seen := grid.Terrain{Grid: g}
	truth := grid.Terrain{Grid: g, Walls: grid.NewLayer(g), Difficult: grid.NewLayer(g)}
	truth.Walls.Set(9, 8, true)
	truth.Difficult.Set(7, 8, true)
	from, to := sq(5, 8), sq(12, 8)

	planned := seen.Move(from, to, nil, grid.Mover{})
	if planned.Blocked || planned.CostDFt != 350 {
		t.Fatalf("the plan on what is seen: %+v", planned)
	}
	got := truth.MoveUntil(from, to, nil, grid.Mover{}, nil, 100000)
	if got.Reached != sq(8, 8) || got.Reason != grid.StopWall {
		t.Fatalf("reached %v for %q, want (8,8) for a wall", got.Reached, got.Reason)
	}
	// 3 squares (15 ft) plus 5 ft for the rubble it did enter: the real cost up to there.
	if got.CostDFt != 200 || !equalSquares(got.Entered, []grid.Square{{6, 8}, {7, 8}, {8, 8}}) {
		t.Errorf("cost %d entered %v, want 200 and 3 squares", got.CostDFt, got.Entered)
	}
	// A wall right next to the start: it doesn't leave.
	truth.Walls.Set(6, 8, true)
	if got := truth.MoveUntil(from, to, nil, grid.Mover{}, nil, 100000); got.Reached != from || got.CostDFt != 0 || got.Reason != grid.StopWall || len(got.Entered) != 0 {
		t.Errorf("blocked at once: %+v", got)
	}
	// With no obstacle the move is the plan.
	open := grid.Terrain{Grid: g}
	if got := open.MoveUntil(from, to, nil, grid.Mover{}, nil, 100000); got.Reached != to || got.CostDFt != 350 || got.Reason != grid.StopNone {
		t.Errorf("a free move: %+v", got)
	}
}

func TestMoveUntilATrapArea(t *testing.T) {
	t.Parallel()
	g := grid.Grid{Columns: 20, Rows: 12}
	tr := grid.Terrain{Grid: g}
	// The Fosso escondido of the cave: a 2 x 2 area at (11,7)..(12,8).
	area := grid.Squares{sq(11, 7), sq(12, 7), sq(11, 8), sq(12, 8)}
	got := tr.MoveUntil(sq(8, 7), sq(15, 7), nil, grid.Mover{}, area, 100000)
	if got.Reached != sq(11, 7) || got.Reason != grid.StopMarked || got.CostDFt != 150 {
		t.Errorf("the move stops at the first square of the area: %+v", got)
	}
	// Ending in the area is a stop in it too, so one check fires the trap.
	got = tr.MoveUntil(sq(8, 7), sq(11, 7), nil, grid.Mover{}, area, 100000)
	if got.Reached != sq(11, 7) || got.Reason != grid.StopMarked {
		t.Errorf("a move that ends in the area: %+v", got)
	}
	// The next square of the area stops it too: a mover that already stands in
	// the area and does not want that leaves its own square out of the set.
	got = tr.MoveUntil(sq(11, 7), sq(15, 7), nil, grid.Mover{}, area, 100000)
	if got.Reached != sq(12, 7) || got.Reason != grid.StopMarked {
		t.Errorf("a move out of the area: %+v", got)
	}
	// Away from the area nothing stops it.
	if got = tr.MoveUntil(sq(8, 3), sq(15, 3), nil, grid.Mover{}, area, 100000); got.Reached != sq(15, 3) || got.Reason != grid.StopNone {
		t.Errorf("a move that misses the area: %+v", got)
	}
}

func TestMoveUntilCreaturesItCannotSeeOrPass(t *testing.T) {
	t.Parallel()
	g := grid.Grid{Columns: 20, Rows: 12}
	tr := grid.Terrain{Grid: g}
	from, to := sq(5, 5), sq(10, 5)
	medium := grid.Mover{Size: grid.SizeMedium}
	// A hostile creature in the way stops it before.
	foe := grid.OccupantMap{sq(8, 5): {Size: grid.SizeMedium, Hostile: true}}
	if got := tr.MoveUntil(from, to, foe, medium, nil, 100000); got.Reached != sq(7, 5) || got.Reason != grid.StopOccupied || got.CostDFt != 100 {
		t.Errorf("a hostile creature in the way: %+v", got)
	}
	// A friend is passed, at 5 ft more.
	friend := grid.OccupantMap{sq(8, 5): {Size: grid.SizeMedium}}
	if got := tr.MoveUntil(from, to, friend, medium, nil, 100000); got.Reached != to || got.CostDFt != 300 || got.Reason != grid.StopNone {
		t.Errorf("a friend in the way: %+v", got)
	}
	// A friend on the destination: it stops on the last square before.
	end := grid.OccupantMap{to: {Size: grid.SizeMedium}}
	if got := tr.MoveUntil(from, to, end, medium, nil, 100000); got.Reached != sq(9, 5) || got.Reason != grid.StopOccupied || got.CostDFt != 200 {
		t.Errorf("a creature on the destination: %+v", got)
	}
	// Two in a row at the end: it backs up past both.
	two := grid.OccupantMap{to: {Size: grid.SizeMedium}, sq(9, 5): {Size: grid.SizeMedium}}
	if got := tr.MoveUntil(from, to, two, medium, nil, 100000); got.Reached != sq(8, 5) {
		t.Errorf("two creatures at the end: %+v", got)
	}
}

func TestCoverBetweenAgainstTheCave(t *testing.T) {
	t.Parallel()
	tr := cave()
	others := func(a, b grid.Square) grid.Squares {
		var out grid.Squares
		for _, s := range []grid.Square{toren, pensantus, brisa, salvia, goblins[0], goblins[1], goblins[2]} {
			if s != a && s != b {
				out = append(out, s)
			}
		}
		return out
	}
	// The numbers of cave.py's `cover` command (cave-data.md).
	cases := []struct {
		a, b grid.Square
		want grid.Cover
	}{
		{sq(14, 7), sq(20, 7), grid.CoverHalf},          // across the crates
		{sq(14, 7), sq(21, 3), grid.CoverTotal},         // a wall in the way
		{sq(14, 7), sq(18, 5), grid.CoverTotal},         // a wall in the way
		{sq(14, 8), sq(20, 7), grid.CoverHalf},          // the crates
		{sq(14, 8), sq(21, 3), grid.CoverThreeQuarters}, // the column
		{sq(14, 8), sq(18, 5), grid.CoverNone},
		{sq(16, 7), sq(20, 7), grid.CoverHalf},
		{sq(16, 7), sq(21, 3), grid.CoverThreeQuarters},
		{sq(16, 7), sq(18, 5), grid.CoverNone},
		{sq(16, 8), sq(20, 7), grid.CoverHalf},
		{sq(16, 8), sq(21, 3), grid.CoverThreeQuarters},
		{sq(16, 8), sq(18, 5), grid.CoverNone},
	}
	for _, c := range cases {
		if got := tr.CoverBetween(c.a, c.b, others(c.a, c.b)); got != c.want {
			t.Errorf("cover of %v against %v = %d, want %d", c.b, c.a, got, c.want)
		}
	}
	// The ends do not count: a target standing on crates is not covered by them.
	if got := tr.CoverBetween(sq(17, 7), sq(19, 7), nil); got != grid.CoverNone {
		t.Errorf("the target's own square gives nothing, got %d", got)
	}
	if got := tr.CoverBetween(sq(19, 7), sq(21, 7), nil); got != grid.CoverNone {
		t.Errorf("the attacker's own square gives nothing, got %d", got)
	}
	// A creature on the line gives half; the best degree applies, they don't add.
	if got := tr.CoverBetween(sq(14, 8), sq(20, 7), grid.Squares{sq(16, 8)}); got != grid.CoverHalf {
		t.Errorf("a creature gives half, got %d", got)
	}
	if got := tr.CoverBetween(sq(14, 8), sq(21, 3), grid.Squares{sq(16, 7)}); got != grid.CoverThreeQuarters {
		t.Errorf("a creature and a column do not add, got %d", got)
	}
	// Neighbors are never covered.
	if got := tr.CoverBetween(sq(10, 7), sq(11, 8), nil); got != grid.CoverNone {
		t.Errorf("a diagonal neighbor has no cover, got %d", got)
	}
}

func TestLeavesReach(t *testing.T) {
	t.Parallel()
	reactor := sq(10, 10)
	cases := []struct {
		name     string
		from, to grid.Square
		reachFt  int
		want     grid.Reaction
	}{
		{"away from a neighbor", sq(11, 10), sq(15, 10), 5, grid.Reaction{Leaves: true, LastInReach: sq(11, 10)}},
		{"diagonal neighbor away", sq(11, 11), sq(14, 14), 5, grid.Reaction{Leaves: true, LastInReach: sq(11, 11)}},
		{"starts outside and stays outside", sq(13, 10), sq(16, 10), 5, grid.Reaction{}},
		{"ends inside", sq(12, 10), sq(11, 10), 5, grid.Reaction{}},
		{"around to another neighbor", sq(11, 10), sq(11, 11), 5, grid.Reaction{}},
		{"walks in and stops", sq(14, 10), sq(11, 10), 5, grid.Reaction{}},
		// Reach 10 ft: two squares. From two away to six away.
		{"a reach of 10 ft", sq(12, 10), sq(16, 10), 10, grid.Reaction{Leaves: true, LastInReach: sq(12, 10)}},
		{"from the middle of a 10 ft reach", sq(11, 10), sq(15, 10), 10, grid.Reaction{Leaves: true, LastInReach: sq(12, 10)}},
		{"a 4 squares diagonal is 25 ft away", sq(11, 11), sq(14, 14), 25, grid.Reaction{}},
		{"from the reactor's own square", sq(10, 10), sq(14, 10), 5, grid.Reaction{Leaves: true, LastInReach: sq(11, 10)}},
	}
	for _, c := range cases {
		if got := grid.LeavesReach(c.from, c.to, reactor, c.reachFt); got != c.want {
			t.Errorf("%s: %+v, want %+v", c.name, got, c.want)
		}
	}
}

func TestReachIsTheSameEveryTime(t *testing.T) {
	t.Parallel()
	tr := cave()
	a := tr.Reach(sq(6, 7), 300, partyBut(sq(6, 7)), grid.Mover{})
	b := tr.Reach(sq(6, 7), 300, partyBut(sq(6, 7)), grid.Mover{})
	if len(a) != len(b) {
		t.Fatal("Reach is not deterministic")
	}
	for i := range a {
		if a[i] != b[i] {
			t.Fatal("Reach is not deterministic")
		}
		if i > 0 && (a[i].Square.Row < a[i-1].Square.Row || (a[i].Square.Row == a[i-1].Square.Row && a[i].Square.Col <= a[i-1].Square.Col)) {
			t.Fatal("Reach is not in reading order")
		}
	}
}

// TestLeavesReachAgreesWithWalkingTheWholeLine: LeavesReach skips a reactor the
// move's box cannot touch; the answer is the one the whole line gives.
func TestLeavesReachAgreesWithWalkingTheWholeLine(t *testing.T) {
	t.Parallel()
	walk := func(from, to, reactor grid.Square, reachFt int) grid.Reaction {
		inside := func(s grid.Square) bool { return grid.RangeFt(reactor, s) <= reachFt }
		if inside(to) {
			return grid.Reaction{}
		}
		var last grid.Square
		found := inside(from)
		if found {
			last = from
		}
		for _, s := range grid.Line(from, to) {
			if inside(s.Square) {
				last, found = s.Square, true
			}
		}
		return grid.Reaction{Leaves: found, LastInReach: last}
	}
	leaving := 0
	for fc := 0; fc < 9; fc += 2 {
		for fr := 0; fr < 9; fr += 3 {
			for tc := 0; tc < 9; tc += 2 {
				for tr := 0; tr < 9; tr += 3 {
					for rc := range 9 {
						for rr := range 9 {
							for _, reach := range []int{5, 10, 15} {
								from, to, reactor := sq(fc, fr), sq(tc, tr), sq(rc, rr)
								want := walk(from, to, reactor, reach)
								if got := grid.LeavesReach(from, to, reactor, reach); got != want {
									t.Fatalf("LeavesReach(%v, %v, %v, %d) = %+v, want %+v", from, to, reactor, reach, got, want)
								}
								if want.Leaves {
									leaving++
								}
							}
						}
					}
				}
			}
		}
	}
	if leaving == 0 {
		t.Fatal("no move left a reach: the comparison proves nothing")
	}
}

func TestLeavesReachByPassingThrough(t *testing.T) {
	t.Parallel()
	// SRD: if you leave a hostile creature's reach during your move, you provoke,
	// even when you did not start in it. The reactor stands at (5,5), reach 5 ft.
	got := grid.LeavesReach(sq(3, 4), sq(7, 4), sq(5, 5), 5)
	if !got.Leaves || got.LastInReach != sq(6, 4) {
		t.Errorf("passing through the reach: %+v, want Leaves and (6,4)", got)
	}
	// A line that goes by without touching the reach does not.
	if got := grid.LeavesReach(sq(3, 2), sq(7, 2), sq(5, 5), 5); got.Leaves {
		t.Errorf("a move that never enters the reach: %+v", got)
	}
	// Walking in and stopping inside does not.
	if got := grid.LeavesReach(sq(2, 5), sq(6, 5), sq(5, 5), 5); got.Leaves {
		t.Errorf("a move that ends inside the reach: %+v", got)
	}
}

func TestDifficultTerrainDoesNotStack(t *testing.T) {
	t.Parallel()
	tr := cave()
	// Rubble at (4,9) and (5,9); an ally stands on (4,9). The move from (3,9) to
	// (5,9) enters two squares of difficult terrain: 100 + 50 + 50 = 200, and the
	// ally on the first does not add another 50 (SRD: even if several things in a
	// space count as difficult terrain).
	occ := grid.OccupantMap{sq(4, 9): {Size: grid.SizeMedium}}
	if mv := tr.Move(sq(3, 9), sq(5, 9), occ, grid.Mover{}); mv.Blocked || mv.CostDFt != 200 {
		t.Errorf("an ally on rubble: %+v, want 200", mv)
	}
	// A flier still pays for the creature's space, but not for the rubble under it.
	if mv := tr.Move(sq(3, 9), sq(5, 9), occ, grid.Mover{Flier: true}); mv.CostDFt != 150 {
		t.Errorf("a flier past an ally on rubble: %+v, want 150", mv)
	}
	if got := tr.MoveUntil(sq(3, 9), sq(5, 9), occ, grid.Mover{}, nil, 10000); got.CostDFt != 200 {
		t.Errorf("MoveUntil costs %d, want 200", got.CostDFt)
	}
}

func TestMoveUntilStopsWhereMovementRunsOut(t *testing.T) {
	t.Parallel()
	g := grid.Grid{Columns: 20, Rows: 12}
	seen := grid.Terrain{Grid: g}
	truth := grid.Terrain{Grid: g, Difficult: grid.NewLayer(g)}
	truth.Difficult.Set(7, 8, true)
	truth.Difficult.Set(8, 8, true)
	from, to := sq(5, 8), sq(11, 8)
	// The plan over what is seen: 6 squares, 300 dft, exactly the movement left.
	if mv := seen.Move(from, to, nil, grid.Mover{}); mv.Blocked || mv.CostDFt != 300 {
		t.Fatalf("the plan: %+v", mv)
	}
	// Two squares of rubble nobody saw: the real move costs 400, but only 300 is left.
	got := truth.MoveUntil(from, to, nil, grid.Mover{}, nil, 300)
	if got.Reason != grid.StopMovement || got.CostDFt > 300 || got.Reached != sq(9, 8) || got.CostDFt != 300 {
		t.Errorf("a move over hidden rubble: %+v, want it to stop on (9,8) at 300", got)
	}
	// With the movement to spare it goes through.
	if got := truth.MoveUntil(from, to, nil, grid.Mover{}, nil, 400); got.Reached != to || got.CostDFt != 400 || got.Reason != grid.StopNone {
		t.Errorf("with enough movement: %+v", got)
	}
	if got := truth.MoveUntil(from, to, nil, grid.Mover{}, nil, 0); got.Reached != from || got.Reason != grid.StopMovement {
		t.Errorf("with none left it does not move: %+v", got)
	}
}

func TestATrapSquareHeldByAnAllyStillFires(t *testing.T) {
	t.Parallel()
	g := grid.Grid{Columns: 20, Rows: 12}
	tr := grid.Terrain{Grid: g}
	area := grid.Squares{sq(11, 7)}
	occ := grid.OccupantMap{sq(11, 7): {Size: grid.SizeMedium}}
	got := tr.MoveUntil(sq(8, 7), sq(14, 7), occ, grid.Mover{Size: grid.SizeMedium}, area, 10000)
	if !got.Marked || got.MarkedAt != sq(11, 7) {
		t.Errorf("the trap fires even though an ally holds the square: %+v", got)
	}
	if got.Reached != sq(10, 7) || got.Reason != grid.StopMarked {
		t.Errorf("the mover ends on the last square it may end in: %+v", got)
	}
	// Nothing marked when the line misses the area.
	if got := tr.MoveUntil(sq(8, 3), sq(14, 3), occ, grid.Mover{}, area, 10000); got.Marked {
		t.Errorf("a move that misses the trap marked it: %+v", got)
	}
}

func TestStandableIsFloorWithoutAShutDoor(t *testing.T) {
	t.Parallel()
	g := grid.Grid{Columns: 8, Rows: 2}
	tr := grid.Terrain{Grid: g, Walls: grid.NewLayer(g), Cover: grid.NewCoverLayer(g), Doors: grid.NewDoorLayer(g)}
	tr.Walls.Set(1, 0, true)
	tr.Cover.Set(2, 0, grid.CoverThreeQuarters)
	tr.Cover.Set(3, 0, grid.CoverHalf)
	for col, door := range map[int]grid.Door{4: grid.DoorClosed, 5: grid.DoorLocked, 6: grid.DoorBarred, 7: grid.DoorSecret} {
		tr.Doors.Set(col, 1, door)
	}
	tr.Doors.Set(0, 1, grid.DoorOpen)
	for _, c := range []struct {
		sq   grid.Square
		want bool
	}{
		{sq(0, 0), true},
		{sq(1, 0), false},
		{sq(2, 0), false},
		{sq(3, 0), true},
		{sq(0, 1), true},
		{sq(4, 1), false},
		{sq(5, 1), false},
		{sq(6, 1), false},
		{sq(7, 1), false},
		{sq(-1, 0), false},
		{sq(0, 2), false},
	} {
		if got := tr.Standable(c.sq); got != c.want {
			t.Errorf("Standable(%v) = %v, want %v", c.sq, got, c.want)
		}
	}
	if !(grid.Terrain{Grid: g}).Standable(sq(3, 1)) {
		t.Error("a map with no layers is floor")
	}
}

func TestAMoverThatIgnoresMapDifficultTerrainStillPaysForASpellsArea(t *testing.T) {
	t.Parallel()
	g := grid.Grid{Columns: 12, Rows: 3}
	tr := grid.Terrain{Grid: g, Difficult: grid.NewLayer(g), ZoneDifficult: grid.NewLayer(g)}
	tr.Difficult.Set(2, 1, true)     // painted on the map
	tr.ZoneDifficult.Set(5, 1, true) // made by a spell
	tr.ZoneDifficult.Set(8, 1, true)
	tr.Difficult.Set(8, 1, true) // both on one square: still one extra, and the spell's holds
	cost := func(m grid.Mover, to int) int { return tr.Move(sq(to-1, 1), sq(to, 1), nil, m).CostDFt }
	walker, ignorer, flier := grid.Mover{Size: grid.SizeMedium}, grid.Mover{Size: grid.SizeMedium, IgnoresMapDifficult: true}, grid.Mover{Size: grid.SizeMedium, Flier: true}
	for _, tc := range []struct {
		name         string
		to           int
		walk, ignore int
	}{
		{"a square painted on the map", 2, 100, 50},
		{"a square of a spell's area", 5, 100, 100},
		{"a square that is both", 8, 100, 100},
		{"plain floor", 3, 50, 50},
	} {
		if got, got2 := cost(walker, tc.to), cost(ignorer, tc.to); got != tc.walk || got2 != tc.ignore {
			t.Errorf("%s: a walker pays %d and one that ignores the map's pays %d, want %d and %d", tc.name, got, got2, tc.walk, tc.ignore)
		}
		if got := cost(flier, tc.to); got != 50 {
			t.Errorf("%s: a flier pays %d, want 50", tc.name, got)
		}
	}
	if bad := (grid.Terrain{Grid: g, ZoneDifficult: grid.NewLayer(grid.Grid{Columns: 3, Rows: 3})}); bad.Validate() == nil {
		t.Error("Validate() of a spell layer sized for another grid = nil, want an error")
	}
}
