package rules

import "testing"

// SRD 5.1 Fighter, Extra Attack: 2 attacks at level 5, 3 at 11, 4 at 20.
func TestRulesReviewCombat_FighterExtraAttack(t *testing.T) {
	c := loadForTest(t)
	scores := map[Ability]int{STR: 16, DEX: 12, CON: 14, INT: 10, WIS: 10, CHA: 8}
	want := func(l int) int {
		switch {
		case l >= 20:
			return 4
		case l >= 11:
			return 3
		case l >= 5:
			return 2
		}
		return 1
	}
	for l := 1; l <= 20; l++ {
		d := Derive(Build{BaseScores: scores, Race: "race:human",
			Classes: []ClassLevel{{Class: "class:fighter", Level: l}}}, c)
		if d.AttacksPerAction != want(l) {
			t.Errorf("fighter %d: AttacksPerAction = %d, SRD says %d", l, d.AttacksPerAction, want(l))
		}
	}
	d := Derive(Build{BaseScores: scores, Race: "race:human",
		Classes: []ClassLevel{{Class: "class:fighter", Level: 11}, {Class: "class:barbarian", Level: 5}}}, c)
	if d.AttacksPerAction != 3 {
		t.Errorf("fighter 11 / barbarian 5: AttacksPerAction = %d, want 3", d.AttacksPerAction)
	}
}
