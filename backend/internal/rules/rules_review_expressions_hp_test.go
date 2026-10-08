package rules

import "testing"

// SRD 5.1 "Finesse": with a finesse weapon you choose Strength or Dexterity
// for both the attack and damage rolls. A dart is ranged and finesse.
func TestRulesReviewExpressions_DartUsesStrength(t *testing.T) {
	t.Parallel()
	c := loadForTest(t)
	b := standard("class:fighter", 1)
	b.BaseScores = map[Ability]int{STR: 18, DEX: 10, CON: 13, INT: 12, WIS: 10, CHA: 8}
	b.Weapons = []string{"equipment:dart"}
	d := Derive(b, c)
	var got *Attack
	for i := range d.Attacks {
		if d.Attacks[i].Key == "equipment:dart" {
			got = &d.Attacks[i]
		}
	}
	if got == nil {
		t.Fatal("no dart attack")
	}
	str := abilityOf(d, STR).Modifier
	if got.Ability != STR || got.AttackBonus != str+2 {
		t.Errorf("dart ability=%v attack=%+d, want STR and %+d (STR %+d + prof 2); damage %q", got.Ability, got.AttackBonus, str+2, str, got.Damage)
	}
}
