package combat

import "testing"

// Sunlight Sensitivity and Sunlight Weakness change a roll only while the creature is in sunlight, and each
// one only the rolls its text names: the Sensitivity the attack rolls and the Wisdom (Perception) checks that
// rely on sight, the Weakness the attack rolls, every ability check and the saving throws.
func TestSunlightTraitsGiveDisadvantageOnlyInSunlight(t *testing.T) {
	sens := Creature{SunlightSensitivity: true}
	weak := Creature{SunlightWeakness: true}
	lit := func(c Creature) Creature { c.Sunlit = true; return c }
	tests := []struct {
		name string
		got  []Source
		want []string
	}{
		{"a Sensitivity in sunlight attacks with disadvantage", AttackMode(AttackScene{Attacker: lit(sens)}).Sources, []string{SourceSunlightAttack}},
		{"a Weakness in sunlight attacks with disadvantage", AttackMode(AttackScene{Attacker: lit(weak)}).Sources, []string{SourceSunlightAttack}},
		{"a Sensitivity in the shade attacks normally", AttackMode(AttackScene{Attacker: sens}).Sources, nil},
		{"a creature with no trait is not spoiled by the sun", AttackMode(AttackScene{Attacker: Creature{Sunlit: true}}).Sources, nil},
		{"the sun spoils the attacker's roll, not the roll against it", AttackMode(AttackScene{Target: lit(sens)}).Sources, nil},
		{"a Sensitivity in sunlight: Perception", SaveMode(SaveScene{Creature: lit(sens), Ability: "wis", Check: true, Perception: true}), []string{SourceSunlightPerception}},
		{"a Sensitivity in sunlight: another check is normal", SaveMode(SaveScene{Creature: lit(sens), Ability: "str", Check: true}), nil},
		{"a Sensitivity in sunlight: a Wisdom check that is not Perception is normal", SaveMode(SaveScene{Creature: lit(sens), Ability: "wis", Check: true}), nil},
		{"a Sensitivity in sunlight: a save is normal", SaveMode(SaveScene{Creature: lit(sens), Ability: "wis"}), nil},
		{"a Sensitivity in the shade: Perception is normal", SaveMode(SaveScene{Creature: sens, Ability: "wis", Check: true, Perception: true}), nil},
		{"a Weakness in sunlight: every check", SaveMode(SaveScene{Creature: lit(weak), Ability: "str", Check: true}), []string{SourceSunlightPerception}},
		{"a Weakness in sunlight: Perception too", SaveMode(SaveScene{Creature: lit(weak), Ability: "wis", Check: true, Perception: true}), []string{SourceSunlightPerception}},
		{"a Weakness in sunlight: every save", SaveMode(SaveScene{Creature: lit(weak), Ability: "dex"}), []string{SourceSunlightSave}},
		{"a Weakness in the shade: a save is normal", SaveMode(SaveScene{Creature: weak, Ability: "dex"}), nil},
		{"a Weakness in the shade: a check is normal", SaveMode(SaveScene{Creature: weak, Ability: "dex", Check: true}), nil},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			if got := kinds(tc.got); !equalKinds(got, tc.want) {
				t.Errorf("sources = %v, want %v", got, tc.want)
			}
			for _, s := range tc.got {
				if s.Effect != ModeDisadvantage {
					t.Errorf("%s is %v, want disadvantage", s.Kind, s.Effect)
				}
			}
		})
	}
}

func equalKinds(a, b []string) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}

// The sun's disadvantage is one more source: an advantage from another circumstance cancels it (SRD 5.1,
// "Advantage and Disadvantage"), and a second disadvantage does not stack.
func TestSunlightDisadvantageCancelsWithAnAdvantageAndDoesNotStack(t *testing.T) {
	spoiled := Creature{Sunlit: true, SunlightSensitivity: true}
	if m := Resolve(AttackMode(AttackScene{Attacker: spoiled}).Sources); m != ModeDisadvantage {
		t.Errorf("mode = %v, want disadvantage", m)
	}
	prone := AttackScene{Attacker: spoiled, Target: Creature{Conditions: []string{conditionStunned}}}
	if m := Resolve(AttackMode(prone).Sources); m != ModeNormal {
		t.Errorf("against a stunned target the sun's disadvantage is canceled: mode = %v, want normal", m)
	}
	poisoned := spoiled
	poisoned.Conditions = []string{conditionPoisoned}
	if m := Resolve(AttackMode(AttackScene{Attacker: poisoned}).Sources); m != ModeDisadvantage {
		t.Errorf("poisoned in sunlight: mode = %v, want disadvantage", m)
	}
}
