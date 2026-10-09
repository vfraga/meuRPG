package zone

import (
	"slices"
	"testing"

	"github.com/PuraFome/meuRPG/backend/internal/rules/grid"
)

func sq(col, row int) grid.Square { return grid.Square{Col: col, Row: row} }

func mustSpec(t *testing.T, key string) Spec {
	t.Helper()
	s, ok := Lookup(key)
	if !ok {
		t.Fatalf("Lookup(%s) not found", key)
	}
	return s
}

func TestEverySpellOfTheCatalogMakesItsShape(t *testing.T) {
	t.Parallel()
	east := grid.Direction{Dx: 1}
	cases := []struct {
		key   string
		slot  int
		count int
	}{
		{"spell:fog-cloud", 1, 49},        // radius 20 ft = 4 squares
		{"spell:fog-cloud", 2, 197},       // 40 ft: 20 ft more for each slot level above the 1st
		{"spell:web", 2, 16},              // a 20-foot cube is 4 × 4
		{"spell:wall-of-fire", 4, 12},     // 60 ft long
		{"spell:spike-growth", 2, 49},     // 20-foot radius
		{"spell:darkness", 2, 29},         // 15-foot radius
		{"spell:stinking-cloud", 3, 49},   // 20-foot radius
		{"spell:cloudkill", 5, 49},        // 20-foot radius
		{"spell:silence", 2, 49},          // 20-foot radius
		{"spell:spirit-guardians", 3, 29}, // 15 ft around the caster
		{"spell:moonbeam", 2, 5},          // the point and the four next to it, on purpose
		{"spell:grease", 1, 4},            // a 10-foot square is 2 × 2
		{"spell:entangle", 1, 16},         // a 20-foot square is 4 × 4
	}
	for _, tc := range cases {
		spec := mustSpec(t, tc.key)
		got := Shape(spec, spec.Sized(tc.slot), sq(20, 20), &east)
		if len(got) != tc.count {
			t.Errorf("%s at slot %d covers %d squares, want %d", tc.key, tc.slot, len(got), tc.count)
		}
		if len(slices.Compact(grid.Sorted(got))) != len(got) {
			t.Errorf("%s repeats a square", tc.key)
		}
	}
}

func TestACubeSetAtAPointPutsTheExtraRowAndColumnOnTheLargerSide(t *testing.T) {
	t.Parallel()
	got := grid.Sorted(grid.CubeAt(sq(10, 10), 10))
	want := []grid.Square{sq(10, 10), sq(11, 10), sq(10, 11), sq(11, 11)}
	if !slices.Equal(got, want) {
		t.Errorf("a 10-foot square at (10,10) = %v, want %v", got, want)
	}
	got = grid.Sorted(grid.CubeAt(sq(10, 10), 20))
	if got[0] != sq(9, 9) || got[len(got)-1] != sq(12, 12) {
		t.Errorf("a 20-foot cube at (10,10) runs %v to %v, want (9,9) to (12,12)", got[0], got[len(got)-1])
	}
	if odd := grid.CubeAt(sq(10, 10), 15); len(odd) != 9 || !slices.Contains(odd, sq(10, 10)) || !slices.Contains(odd, sq(9, 9)) || !slices.Contains(odd, sq(11, 11)) {
		t.Errorf("a 15-foot cube is not 3 × 3 around the point: %v", odd)
	}
}

func TestTheRingIsAClosedLoopAroundAHollow(t *testing.T) {
	t.Parallel()
	ring := grid.RingAt(sq(10, 10), 20)
	if len(ring) != 8 {
		t.Fatalf("a ring of 20 ft diameter has %d squares, want 8", len(ring))
	}
	for _, in := range []grid.Square{sq(10, 10), sq(11, 10), sq(10, 9)} {
		if slices.Contains(ring, in) {
			t.Errorf("%v is in the hollow but the ring covers it", in)
		}
	}
	for _, on := range []grid.Square{sq(12, 10), sq(8, 10), sq(10, 12), sq(10, 8), sq(11, 11), sq(9, 9)} {
		if !slices.Contains(ring, on) {
			t.Errorf("%v is on the loop but the ring misses it", on)
		}
	}
}

func TestAWallBurnsOnTheSideTheCasterChose(t *testing.T) {
	t.Parallel()
	north := grid.Direction{Dy: -1}
	w := Wall{Cells: grid.WallFrom(sq(10, 20), north, 60), Dir: &north}
	// A wall running north: the left of its direction is west (side A), the right is east.
	if s, ok := w.SideOf(sq(8, 15)); !ok || s != SideA {
		t.Errorf("west of a wall running north = %v %v, want A", s, ok)
	}
	if s, ok := w.SideOf(sq(12, 15)); !ok || s != SideB {
		t.Errorf("east of a wall running north = %v %v, want B", s, ok)
	}
	if _, ok := w.SideOf(sq(10, 5)); ok {
		t.Errorf("a square ahead of the wall on its line has no side")
	}
	tests := []struct {
		sq   grid.Square
		side Side
		want bool
	}{
		{sq(10, 15), SideB, true},  // inside the wall
		{sq(12, 15), SideB, true},  // 10 ft east, the damaging side
		{sq(13, 15), SideB, false}, // 15 ft east is out of reach
		{sq(8, 15), SideB, false},  // 10 ft west, the other side deals no damage
		{sq(8, 15), SideA, true},
		{sq(12, 15), SideNone, false}, // no side chosen: only the wall itself burns
	}
	for _, tc := range tests {
		if got := w.Within(tc.sq, tc.side, 10); got != tc.want {
			t.Errorf("Within(%v, %q) = %v, want %v", tc.sq, tc.side, got, tc.want)
		}
	}
	ring := Wall{Cells: grid.RingAt(sq(10, 10), 20), Center: sq(10, 10), Radius: 2, Ring: true}
	if s, ok := ring.SideOf(sq(10, 10)); !ok || s != SideA {
		t.Errorf("the ring's hollow = %v %v, want A (inside)", s, ok)
	}
	if s, ok := ring.SideOf(sq(14, 10)); !ok || s != SideB {
		t.Errorf("outside the ring = %v %v, want B", s, ok)
	}
}

func TestNobodySeesIntoOutOfOrThroughAHeavilyObscuredZone(t *testing.T) {
	t.Parallel()
	cells := grid.CubeAt(sq(10, 10), 20)
	blocking := func(s grid.Square) bool { return slices.Contains(cells, s) }
	cases := []struct {
		name     string
		from, to grid.Square
		want     bool
	}{
		{"both outside, the zone between them", sq(2, 10), sq(20, 10), true},
		{"both outside, clear of the zone", sq(2, 2), sq(20, 2), false},
		{"from outside into it", sq(2, 10), sq(10, 10), true},
		{"from inside out of it", sq(10, 10), sq(2, 10), true},
		{"both inside, two creatures", sq(9, 9), sq(12, 12), true},
		{"a creature sees itself", sq(10, 10), sq(10, 10), false},
	}
	for _, tc := range cases {
		if got := Blocks(blocking, tc.from, tc.to); got != tc.want {
			t.Errorf("%s: Blocks = %v, want %v", tc.name, got, tc.want)
		}
	}
}

func TestACreatureLargerThanASquareIsInTheZoneWhenAnySquareIs(t *testing.T) {
	t.Parallel()
	cells := grid.CubeAt(sq(10, 10), 10)
	large := []grid.Square{sq(11, 11), sq(12, 11), sq(11, 12), sq(12, 12)}
	any, all := Overlap(cells, large)
	if !any || all {
		t.Errorf("a Large creature half in the zone: any = %v all = %v, want true false", any, all)
	}
	if any, all := Overlap(cells, []grid.Square{sq(10, 10)}); !any || !all {
		t.Errorf("a creature wholly in the zone: any = %v all = %v", any, all)
	}
	if any, all := Overlap(cells, nil); any || all {
		t.Errorf("no footprint is in nothing")
	}
}

func TestSpikeGrowthCountsTheSquaresAMoveEntersInTheZone(t *testing.T) {
	t.Parallel()
	spec := mustSpec(t, "spell:spike-growth")
	cells := Shape(spec, spec.Sized(2), sq(20, 20), nil)
	if got := Crossed(cells, sq(10, 20), sq(20, 20)); got != 5 {
		t.Errorf("a move from outside to the center enters %d squares of the zone, want 5 (x = 16..20)", got)
	}
	if got := Crossed(cells, sq(18, 20), sq(22, 20)); got != 4 {
		t.Errorf("a move within the zone enters %d squares, want 4", got)
	}
	if got := Crossed(cells, sq(2, 2), sq(8, 2)); got != 0 {
		t.Errorf("a move far from the zone enters %d squares, want 0", got)
	}
}

func TestEveryTriggerAndRuleOfTheCatalogIsKnown(t *testing.T) {
	t.Parallel()
	for _, key := range Keys() {
		spec := mustSpec(t, key)
		for _, tr := range spec.Triggers {
			if !tr.Kind.Valid() {
				t.Errorf("%s has an unknown trigger %q", key, tr.Kind)
			}
			if tr.Save != "" && tr.OnSuccess == "" {
				t.Errorf("%s trigger %s has a save and no success rule", key, tr.Kind)
			}
		}
		for _, r := range spec.Rules {
			if !r.Valid() {
				t.Errorf("%s has an unknown rule %q", key, r)
			}
		}
		if !spec.Obscurity.Valid() {
			t.Errorf("%s has an unknown obscurity %q", key, spec.Obscurity)
		}
	}
}

func TestGreaseHasNoConcentrationAndTheOthersDo(t *testing.T) {
	t.Parallel()
	for _, key := range Keys() {
		spec := mustSpec(t, key)
		if want := key != "spell:grease"; spec.Concentration != want {
			t.Errorf("%s concentration = %v, want %v (SRD: Grease lasts 1 minute with no concentration)", key, spec.Concentration, want)
		}
	}
}

func TestOnlySilenceHasNoVisibleForm(t *testing.T) {
	t.Parallel()
	for _, key := range Keys() {
		if spec := mustSpec(t, key); spec.Visible == (key == "spell:silence") {
			t.Errorf("%s visible = %v", key, spec.Visible)
		}
	}
}

func TestWebAndGreaseHaveNoFirstTimeOnATurnLimit(t *testing.T) {
	t.Parallel()
	for _, key := range []string{"spell:web", "spell:grease"} {
		spec := mustSpec(t, key)
		if _, limited := spec.Trigger(OnEnterFirstTimeOnATurn); limited {
			t.Errorf("%s limits entering to the first time on a turn; its text has no such limit", key)
		}
		if _, each := spec.Trigger(OnEnter); !each {
			t.Errorf("%s has no on_enter trigger", key)
		}
	}
}

func TestWindsDisperseWhatTheSRDSays(t *testing.T) {
	t.Parallel()
	stinking := mustSpec(t, "spell:stinking-cloud")
	if stinking.Dispersal[WindModerate] != 4 || stinking.Dispersal[WindStrong] != 1 {
		t.Errorf("stinking cloud dispersal = %v, want 4 rounds (moderate) and 1 (strong)", stinking.Dispersal)
	}
	if _, ok := mustSpec(t, "spell:cloudkill").Dispersal[WindModerate]; ok {
		t.Errorf("cloudkill is dispersed by a strong wind only")
	}
	if _, ok := mustSpec(t, "spell:cloudkill").Dispersal[WindStrong]; !ok {
		t.Errorf("a strong wind disperses cloudkill")
	}
	if len(mustSpec(t, "spell:darkness").Dispersal) != 0 {
		t.Errorf("wind does not disperse Darkness")
	}
}
