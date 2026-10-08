package rules

import (
	"slices"
	"testing"
)

// SRD 5.1 "Life Domain Spells": 7th level grants death ward AND guardian of faith.
func TestRulesReviewProgression_LifeDomainSpells(t *testing.T) {
	c := loadForTest(t)
	b := clericBuild(7)
	b.SpellsPrepared = []string{"spell:cure-wounds", "spell:bless"}
	d := Derive(b, c)

	prepared := map[string]bool{}
	for _, s := range d.Spells {
		if s.Prepared {
			prepared[s.Spell.Key] = true
		}
	}
	for _, want := range []string{"spell:death-ward", "spell:guardian-of-faith"} {
		if !prepared[want] {
			t.Errorf("level-7 Life cleric should have %s always prepared (SRD 5.1 Life Domain Spells, 7th)", want)
		}
	}

	var got []string
	for _, s := range c.c.subclasses["subclass:life"].Spells {
		if s.ClassLevel == 7 {
			got = append(got, s.Spell)
		}
	}
	slices.Sort(got)
	want := []string{"spell:death-ward", "spell:guardian-of-faith"}
	if !slices.Equal(got, want) {
		t.Errorf("subclass:life class_level 7 spells = %v, want %v", got, want)
	}
}
