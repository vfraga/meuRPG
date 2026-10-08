package rules

import "testing"

func TestRulesReviewCombat_MonkVersatileDie(t *testing.T) {
	c := loadForTest(t)
	cases := []struct {
		level          int
		dmg, versatile string
	}{
		{11, "1d8+5", "1d8+5"},
		{17, "1d10+5", "1d10+5"},
	}
	for _, tc := range cases {
		d := Derive(Build{
			BaseScores: map[Ability]int{STR: 10, DEX: 20, CON: 14, INT: 10, WIS: 14, CHA: 10},
			Race:       "race:human",
			Classes:    []ClassLevel{{Class: "class:monk", Level: tc.level}},
			Weapons:    []string{"equipment:quarterstaff"},
		}, c)
		var found bool
		for _, a := range d.Attacks {
			if a.Key != "equipment:quarterstaff" {
				continue
			}
			found = true
			if a.Damage != tc.dmg {
				t.Errorf("monk %d Damage = %q, want %q", tc.level, a.Damage, tc.dmg)
			}
			if a.VersatileDamage != tc.versatile {
				t.Errorf("monk %d VersatileDamage = %q, want %q", tc.level, a.VersatileDamage, tc.versatile)
			}
		}
		if !found {
			t.Fatalf("monk %d: quarterstaff attack missing", tc.level)
		}
	}
}
