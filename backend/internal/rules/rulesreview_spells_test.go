package rules

import (
	"fmt"
	"testing"
)

// rollsAt prints the rolls of a spell at a slot and character level.
func rrsRolls(t *testing.T, c *Content, key string, slot, char int) []DamageRoll {
	t.Helper()
	d, ok := c.SpellDetails(key)
	if !ok {
		t.Fatalf("no details for %s", key)
	}
	return d.DamageAt(slot, char)
}

// SRD 5.1, Eldritch Blast: one beam, two at 5th level, three at 11th, four at
// 17th; each beam is its own attack roll and deals 1d10 force.
func TestRulesReviewSpells_EldritchBlastBeams(t *testing.T) {
	c := loadForTest(t)
	want := map[int]int{1: 1, 5: 2, 11: 3, 17: 4}
	sheet := map[int]string{}
	for _, lv := range []int{1, 5, 11, 17} {
		b := standard("class:warlock", lv)
		b.Cantrips = []string{"spell:eldritch-blast"}
		d := Derive(b, c)
		a, ok := attackOf(d, "spell:eldritch-blast")
		if !ok {
			t.Fatalf("level %d: no eldritch blast attack on the sheet", lv)
		}
		a.AttackBonus, a.SaveDC, a.Proficient = 0, 0, false // the to-hit grows with the level on its own; the beams are what is asked
		sheet[lv] = fmt.Sprintf("%+v", a)
		t.Logf("level %d: Damage=%q DamageDice=%+v Notes=%q", lv, a.Damage, a.DamageDice, a.Notes)
	}
	// The sheet must tell the beam count: level 17 (4 beams) cannot look like level 1 (1 beam).
	for _, lv := range []int{5, 11, 17} {
		if sheet[lv] == sheet[1] {
			t.Errorf("Eldritch Blast at level %d: SRD says %d beams (each its own attack + 1d10 force), the sheet (ignoring to-hit) is identical to level 1, one attack of 1d10, no beam count: %s",
				lv, want[lv], sheet[lv])
		}
	}
}

// SRD 5.1 "At Higher Levels" for spells whose app table has only the base slot.
func TestRulesReviewSpells_HigherSlotScaling(t *testing.T) {
	c := loadForTest(t)
	tests := []struct {
		key, dtype string
		slot       int
		want       string
	}{
		{"spell:disintegrate", "damage-type:force", 7, "13d6 + 40"},
		{"spell:freezing-sphere", "damage-type:cold", 7, "11d6"},
		{"spell:phantasmal-killer", "damage-type:psychic", 5, "5d10"},
		{"spell:wall-of-fire", "damage-type:fire", 5, "6d8"},
		{"spell:flame-strike", "damage-type:fire", 6, "5d6"},
		{"spell:flame-strike", "damage-type:radiant", 6, "5d6"},
	}
	for _, tc := range tests {
		t.Run(tc.key+"@"+fmt.Sprint(tc.slot)+"/"+tc.dtype, func(t *testing.T) {
			var got *DamageRoll
			for _, r := range rrsRolls(t, c, tc.key, tc.slot, 20) {
				if r.Type == tc.dtype {
					r := r
					got = &r
				}
			}
			if got == nil {
				t.Fatalf("%s slot %d: SRD says %s %s, the app has no %s roll", tc.key, tc.slot, tc.want, tc.dtype, tc.dtype)
			}
			if got.Raw != tc.want || !got.Parsed {
				t.Errorf("%s slot %d: SRD says %s, the app gives %q (parsed=%v)", tc.key, tc.slot, tc.want, got.Raw, got.Parsed)
			}
		})
	}
}

// SRD 5.1, Spirit Guardians: 3d8 radiant or necrotic, +1d8 per slot above 3rd.
func TestRulesReviewSpells_SpiritGuardiansNoDamage(t *testing.T) {
	c := loadForTest(t)
	for _, tc := range []struct {
		slot, char int
		want       string
	}{{3, 5, "3d8"}, {5, 9, "5d8"}} {
		rolls := rrsRolls(t, c, "spell:spirit-guardians", tc.slot, tc.char)
		if len(rolls) == 0 {
			t.Errorf("Spirit Guardians slot %d: SRD says %s, the app returns no damage roll at all", tc.slot, tc.want)
			continue
		}
		if rolls[0].Raw != tc.want || !rolls[0].Parsed {
			t.Errorf("Spirit Guardians slot %d: SRD says %s, the app gives %q", tc.slot, tc.want, rolls[0].Raw)
		}
	}
}

// SRD 5.1, Life Domain Spells table: 7th level is death ward AND guardian of faith.
func TestRulesReviewSpells_LifeDomainSpells(t *testing.T) {
	c := loadForTest(t)
	b := standard("class:cleric", 7)
	b.Classes[0].Subclass = "subclass:life"
	d := Derive(b, c)
	for _, s := range d.Spells {
		if s.Spell.Key == "spell:guardian-of-faith" {
			if !s.Prepared {
				t.Errorf("guardian of faith is on the sheet but not Prepared: %+v", s)
			}
			return
		}
	}
	t.Errorf("Life Domain cleric 7: SRD lists death ward and guardian of faith at 7th, the app has only death ward (guardian of faith missing from Derived.Spells)")
}
