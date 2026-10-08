package rules

import (
	"strings"
	"testing"
)

// TestRulesReviewCombat_UnarmedStrike checks SRD 5.1 unarmed strikes: any
// creature is proficient, deals 1+STR bludgeoning; a Monk uses DEX and the
// Martial Arts die.
func TestRulesReviewCombat_UnarmedStrike(t *testing.T) {
	t.Parallel()
	c := loadForTest(t)

	find := func(d Derived) (Attack, bool) {
		for _, a := range d.Attacks {
			if strings.Contains(strings.ToLower(a.Key), "unarmed") || strings.Contains(strings.ToLower(a.Name), "unarmed") {
				return a, true
			}
		}
		return Attack{}, false
	}

	cases := []struct {
		name   string
		build  Build
		bonus  int
		damage string
	}{
		{"monk1 dex16", Build{
			BaseScores: map[Ability]int{"str": 10, "dex": 16, "con": 10, "int": 10, "wis": 10, "cha": 10},
			Race:       "race:human",
			Classes:    []ClassLevel{{Class: "class:monk", Level: 1}},
		}, 5, "1d4+3"},
		{"wizard1 str10", Build{
			BaseScores: map[Ability]int{"str": 10, "dex": 10, "con": 10, "int": 10, "wis": 10, "cha": 10},
			Race:       "race:human",
			Classes:    []ClassLevel{{Class: "class:wizard", Level: 1}},
		}, 2, "1"},
	}
	for _, tc := range cases {
		d := Derive(tc.build, c)
		a, ok := find(d)
		if !ok {
			t.Errorf("%s: no unarmed strike attack line (attacks=%d)", tc.name, len(d.Attacks))
			continue
		}
		if a.AttackBonus != tc.bonus {
			t.Errorf("%s: AttackBonus = %d, want %d", tc.name, a.AttackBonus, tc.bonus)
		}
		if a.Damage != tc.damage {
			t.Errorf("%s: Damage = %q, want %q", tc.name, a.Damage, tc.damage)
		}
		if a.DamageType != "damage-type:bludgeoning" {
			t.Errorf("%s: DamageType = %q", tc.name, a.DamageType)
		}
		if !a.Melee {
			t.Errorf("%s: Melee = false", tc.name)
		}
	}
}
