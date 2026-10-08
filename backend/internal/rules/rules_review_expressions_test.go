package rules

import (
	"strconv"
	"strings"
	"testing"
)

// TestRulesReviewExpressions_DisintegrateUpcast: SRD 5.1 Disintegrate says
// "the damage increases by 3d6 for each slot level above 6th".
func TestRulesReviewExpressions_DisintegrateUpcast(t *testing.T) {
	c := loadForTest(t)
	d, ok := c.SpellDetails("spell:disintegrate")
	if !ok {
		t.Fatal("spell:disintegrate missing")
	}
	want := map[int]string{6: "10d6 + 40", 7: "13d6 + 40", 8: "16d6 + 40", 9: "19d6 + 40"}
	for slot := 6; slot <= 9; slot++ {
		rolls := d.DamageAt(slot, 20)
		if len(rolls) != 1 {
			t.Fatalf("slot %d: %d damage rolls, want 1", slot, len(rolls))
		}
		if got := rolls[0].Raw; got != want[slot] {
			t.Errorf("Disintegrate at slot %d: app = %q, SRD = %q (+3d6 per slot level above 6th)", slot, got, want[slot])
		}
	}
}

// TestRulesReviewExpressions_UpcastDiceSweep compares, for spells whose SRD
// higher_level text adds a fixed dice step per slot level, the dice the app
// serves (DamageAt, HealAt) with base + step*(slot-level).
func TestRulesReviewExpressions_UpcastDiceSweep(t *testing.T) {
	c := loadForTest(t)
	// spell -> {damage type suffix, base dice count, sides, dice added per slot level above the spell's own}
	cases := []struct {
		key        string
		level      int
		base, step int
		sides      int
		bonus      string
	}{
		{"disintegrate", 6, 10, 3, 6, " + 40"},
		{"freezing-sphere", 6, 10, 1, 6, ""},
		{"phantasmal-killer", 4, 4, 1, 10, ""},
		{"wall-of-fire", 4, 5, 1, 8, ""},
	}
	for _, tc := range cases {
		d, ok := c.SpellDetails("spell:" + tc.key)
		if !ok {
			t.Errorf("%s missing", tc.key)
			continue
		}
		for slot := tc.level + 1; slot <= 9; slot++ {
			want := strconv.Itoa(tc.base+tc.step*(slot-tc.level)) + "d" + strconv.Itoa(tc.sides) + tc.bonus
			rolls := d.DamageAt(slot, 20)
			got := ""
			if len(rolls) > 0 {
				got = rolls[0].Raw
			}
			if !strings.EqualFold(got, want) {
				t.Errorf("%s at slot %d: app = %q, SRD = %q", tc.key, slot, got, want)
			}
		}
	}
}
