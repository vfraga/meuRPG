package rules

import (
	"fmt"
	"maps"
	"slices"
	"testing"
)

// TestLevelUpSweepTable is the quality gate of ADR-0018 (section 12): the
// sweep of TestLevelUpSweep over classes the table wrote. Every generated table
// class (none, full, half and pact casters, which prepare or know their spells),
// with each of its subclasses, and the third-caster subclasses of two SRD
// classes, go from level 1 to 20, each step satisfying exactly what
// LevelUpOptions asks for. No step may fail CheckLevelUp, Validate or leave an
// Issue, and none may panic.
func TestLevelUpSweepTable(t *testing.T) {
	t.Parallel()
	srd := loadForTest(t)
	c := withOverlay(t, genOverlay(t, srd))
	n := 0
	for _, classKey := range sortedKeys(c.c.classes) {
		for _, subKey := range c.c.classes[classKey].Subclasses {
			if !isTableKey(classKey) && !isTableKey(subKey) {
				continue
			}
			n++
			t.Run(classKey+"/"+subKey, func(t *testing.T) {
				t.Parallel()
				b := sweepBase(t, c, classKey, subKey)
				b = sweepUp(t, c, b, classKey, subKey, MaxLevel)
				if got := Derive(b, c).TotalLevel; got != MaxLevel {
					t.Errorf("total level = %d", got)
				}
			})
		}
	}
	if want := len(genKinds)*2 - 1 + 2; n != want { // two subclasses of each class that casts, one of the class that does not, and the two third casters
		t.Errorf("swept %d class/subclass pairs, want %d", n, want)
	}
}

// TestLevelUpSweepTableMulticlass runs the sweep with a table class and an SRD
// class together, in both orders: the table class takes the levels after the SRD
// class reached level 5, and the SRD class joins a table class that reached
// level 5. The multiclass spellcaster rule (full, half and third casters, with
// the table's) must leave no Issue either.
func TestLevelUpSweepTableMulticlass(t *testing.T) {
	t.Parallel()
	srd := loadForTest(t)
	c := withOverlay(t, genOverlay(t, srd))
	strong := map[Ability]int{STR: 14, DEX: 14, CON: 14, INT: 14, WIS: 14, CHA: 14}
	for _, gk := range genKinds {
		tableKey := "class:gen-" + gk.name + tableSuffix
		tableSub := "subclass:gen-" + gk.name + "-a" + tableSuffix
		for _, srdKey := range []string{"class:fighter", "class:wizard", "class:ranger"} {
			srdSub := srd.c.classes[srdKey].Subclasses[0]
			t.Run(fmt.Sprintf("%s after %s", tableKey, srdKey), func(t *testing.T) {
				t.Parallel()
				b := sweepBase(t, c, srdKey, srdSub)
				b.BaseScores = copyScores(strong)
				b = sweepUp(t, c, b, srdKey, srdSub, 5)
				b = addClass(t, c, b, tableKey, 1)
				sweepUp(t, c, b, tableKey, tableSub, MaxLevel)
			})
			t.Run(fmt.Sprintf("%s before %s", tableKey, srdKey), func(t *testing.T) {
				t.Parallel()
				b := sweepBase(t, c, tableKey, tableSub)
				b.BaseScores = copyScores(strong)
				b = sweepUp(t, c, b, tableKey, tableSub, 5)
				b = addClass(t, c, b, srdKey, 1)
				sweepUp(t, c, b, tableKey, tableSub, 14)
			})
		}
	}
	for _, third := range []struct{ class, sub string }{
		{"class:fighter", "subclass:cavaleiro-runico" + tableSuffix},
		{"class:rogue", "subclass:trapaceiro-mistico" + tableSuffix},
	} {
		for _, other := range []string{"class:wizard", "class:paladin", "class:warlock"} {
			t.Run(third.sub+" with "+other, func(t *testing.T) {
				t.Parallel()
				b := sweepBase(t, c, third.class, third.sub)
				b.BaseScores = copyScores(strong)
				b = sweepUp(t, c, b, third.class, third.sub, 6)
				lvl := 1
				if other == "class:paladin" {
					lvl = 2 // casts from level 2
				}
				b = addClass(t, c, b, other, lvl)
				b = sweepUp(t, c, b, third.class, third.sub, MaxLevel)
				if d := Derive(b, c); len(d.Spellcasting) != 2 {
					t.Errorf("casters = %d, want 2", len(d.Spellcasting))
				}
			})
		}
	}
}

func copyScores(m map[Ability]int) map[Ability]int {
	out := make(map[Ability]int, len(m))
	maps.Copy(out, m)
	return out
}

// nextSkillFrom is the first skill of the list the character is not proficient in; the list is the one the class lets a
// multiclass character choose from (an empty list allows every skill).
func nextSkillFrom(t *testing.T, c *Content, b Build, from []string) string {
	t.Helper()
	have := map[string]bool{}
	for _, s := range slices.Concat(b.SkillProficiencies, c.c.backgrounds[b.Background].Skills) {
		have[s] = true
	}
	for _, s := range c.c.skillOrder {
		if !have[s] && (len(from) == 0 || slices.Contains(from, s)) {
			return s
		}
	}
	t.Fatalf("no skill of %v is left for the character to choose", from)
	return ""
}

// addClass is the master's multiclassing in the editor: a new class at a level,
// with the skills it gives a multiclass character and the spells it asks for.
func addClass(t *testing.T, c *Content, b Build, classKey string, level int) Build {
	t.Helper()
	b.Classes = append(b.Classes, ClassLevel{Class: classKey, Level: level})
	if mc := c.c.classes[classKey].Multiclass.SkillChoices; mc != nil {
		for range mc.Choose {
			b.SkillProficiencies = append(b.SkillProficiencies, nextSkillFrom(t, c, b, mc.From))
		}
	}
	return fillSpells(c, b, classKey, 0)
}
