package rules

import "testing"

// TestRulesReviewMap_HeavyArmorStrengthSpeed checks the SRD 5.1 armor rule:
// wearing armor whose Strength column is above the character's Strength score
// reduces speed by 10 feet (dwarves are not slowed).
func TestRulesReviewMap_HeavyArmorStrengthSpeed(t *testing.T) {
	t.Parallel()
	c := loadForTest(t)

	tests := []struct {
		name    string
		race    string
		subrace string
		armor   string
		str     int // final Strength score wanted
		bonus   int // racial STR bonus, to pick the base score
		want    int
	}{
		{"human Str 10 chain mail (needs 13)", "race:human", "", "equipment:chain-mail", 10, 1, 20},
		{"human Str 13 chain mail (meets 13)", "race:human", "", "equipment:chain-mail", 13, 1, 30},
		{"human Str 14 splint (needs 15)", "race:human", "", "equipment:splint", 14, 1, 20},
		{"human Str 15 splint (meets 15)", "race:human", "", "equipment:splint", 15, 1, 30},
		{"human Str 14 plate (needs 15)", "race:human", "", "equipment:plate", 14, 1, 20},
		{"human Str 15 plate (meets 15)", "race:human", "", "equipment:plate", 15, 1, 30},
		{"human Str 8 leather (no requirement)", "race:human", "", "equipment:leather", 8, 1, 30},
		{"hill dwarf Str 8 chain mail (not slowed)", "race:dwarf", "subrace:hill-dwarf", "equipment:chain-mail", 8, 0, 25},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			b := standard("class:fighter", 1)
			b.Race, b.Subrace = tt.race, tt.subrace
			b.BaseScores[STR] = tt.str - tt.bonus
			b.Armor = tt.armor
			d := Derive(b, c)
			if got := abilityOf(d, STR).Score; got != tt.str {
				t.Fatalf("%s: setup error: final STR = %d, want %d", tt.name, got, tt.str)
			}
			if d.SpeedWalkFt != tt.want {
				t.Errorf("%s: app speed = %d ft, SRD speed = %d ft", tt.name, d.SpeedWalkFt, tt.want)
			}
		})
	}
}
