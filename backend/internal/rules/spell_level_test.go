package rules

import "testing"

func TestMaxSpellLevelFromSlots(t *testing.T) {
	tests := []struct {
		name  string
		slots [9]int
		want  int
	}{
		{"no slots", [9]int{}, 0},
		{"one first-circle slot", [9]int{2}, 1},
		{"up to third circle", [9]int{4, 3, 2}, 3},
		{"warlock pact slots live at one circle", [9]int{0, 0, 2}, 3},
		{"all nine circles", [9]int{4, 3, 3, 3, 3, 2, 2, 1, 1}, 9},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := MaxSpellLevelFromSlots(tt.slots); got != tt.want {
				t.Errorf("MaxSpellLevelFromSlots(%v) = %d, want %d", tt.slots, got, tt.want)
			}
		})
	}
}

// TestCatalogMaxSpellLevelByLevel reads the real SRD table: the browser
// relies on these numbers to list only the spells a character can take.
func TestCatalogMaxSpellLevelByLevel(t *testing.T) {
	c, err := LoadSRD()
	if err != nil {
		t.Fatal(err)
	}
	byClass := map[string][]int{}
	for _, cl := range c.Catalog().Classes {
		byClass[cl.Key] = cl.MaxSpellLevelByLevel
	}
	tests := []struct {
		class string
		level int
		want  int
	}{
		{"class:bard", 1, 1},
		{"class:bard", 3, 2},
		{"class:wizard", 5, 3},
		{"class:paladin", 1, 0},
		{"class:paladin", 2, 1},
		{"class:warlock", 3, 2},
		{"class:warlock", 11, 5},
		{"class:wizard", 20, 9},
	}
	for _, tt := range tests {
		got := byClass[tt.class]
		if len(got) != MaxLevel {
			t.Fatalf("%s: %d entries, want %d", tt.class, len(got), MaxLevel)
		}
		if got[tt.level-1] != tt.want {
			t.Errorf("%s level %d = %d, want %d", tt.class, tt.level, got[tt.level-1], tt.want)
		}
	}
	if got := byClass["class:fighter"]; got != nil {
		t.Errorf("fighter = %v, want none", got)
	}
}

// TestClassSpellListsFollowTheSRD: the spell lists of the bard, the cleric and the
// druid are the SRD 5.1's (Spellcasting chapter, Spell Lists), not the snapshot's.
func TestClassSpellListsFollowTheSRD(t *testing.T) {
	t.Parallel()
	c := loadForTest(t).c
	for _, tc := range []struct {
		class, spell string
		want         bool
	}{
		{"class:bard", "spell:faerie-fire", true},
		{"class:cleric", "spell:divination", true},
		{"class:cleric", "spell:arcane-eye", false},
		{"class:druid", "spell:meld-into-stone", true},
		{"class:druid", "spell:create-food-and-water", false},
		{"class:druid", "spell:divination", false},
		// What was right stays right.
		{"class:cleric", "spell:create-food-and-water", true},
		{"class:wizard", "spell:arcane-eye", true},
	} {
		s, ok := c.spells[tc.spell]
		if !ok {
			t.Fatalf("no spell %s", tc.spell)
		}
		if got := c.onList(s, tc.class); got != tc.want {
			t.Errorf("%s on the list of %s = %v, want %v", tc.spell, tc.class, got, tc.want)
		}
	}
}
