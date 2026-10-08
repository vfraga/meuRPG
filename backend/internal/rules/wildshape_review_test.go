package rules

import "testing"

func wildShapeResource(t *testing.T, d Derived) Resource {
	t.Helper()
	for _, r := range d.Resources {
		if r.Key == "wild_shape" {
			return r
		}
	}
	t.Fatal("no wild_shape resource")
	return Resource{}
}

// TestRulesReviewCreatures_ArchdruidUnlimitedWildShape: SRD, Archdruid (20th level): "You
// can use your Wild Shape an unlimited number of times." The barbarian's unlimited Rage is
// modelled as Max 99 (rules.Resource.Max), so the 20th-level druid's Wild Shape must be too.
func TestRulesReviewCreatures_ArchdruidUnlimitedWildShape(t *testing.T) {
	t.Parallel()
	c := loadForTest(t)
	if got := wildShapeResource(t, Derive(salvia(19), c)).Max; got != 2 {
		t.Fatalf("level 19 Wild Shape max = %d, want 2", got)
	}
	if got := wildShapeResource(t, Derive(salvia(20), c)).Max; got < 99 {
		t.Errorf("level 20 (Archdruid) Wild Shape max = %d, want unlimited (99, like the barbarian's Rage)", got)
	}
}

// TestRulesReviewCreatures_BeastSpellsAt18: SRD, Beast Spells (18th level): the druid can
// perform somatic and verbal components in a beast shape, so keeps casting (no material
// components). Below 18 the spells are gone.
func TestRulesReviewCreatures_BeastSpellsAt18(t *testing.T) {
	t.Parallel()
	c := loadForTest(t)
	for _, level := range []int{17, 18} {
		d, err := c.WildShapeDerived(Derive(salvia(level), c), "monster:wolf")
		if err != nil {
			t.Fatal(err)
		}
		has := d.Spellcasting != nil && len(d.Spells) > 0
		if want := level >= 18; has != want {
			t.Errorf("druid %d in wolf form keeps spellcasting = %v, want %v (Spellcasting nil: %v, Spells: %d)", level, has, want, d.Spellcasting == nil, len(d.Spells))
		}
	}
}
