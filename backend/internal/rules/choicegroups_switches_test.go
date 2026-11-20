package rules

import (
	"fmt"
	"slices"
	"testing"
)

// The table's switches apply to every spell choice: the book's rituals, the Pact of the Tome's cantrips, a Mystic
// Arcanum and a bonus cantrip. A sheet that holds a pick keeps it, and the master reads it all the same.

// spellChoiceRows are the spell choices of a sheet, each with a spell that is on its list.
func spellChoiceRows() []struct {
	name, choiceKey, spell string
	build                  func(picks ...string) Build
} {
	warlock := func(level int, picks ...string) Build {
		return choiceBuild("race:human", "", ClassLevel{Class: "class:warlock", Subclass: "subclass:fiend", Level: level}, picks...)
	}
	return []struct {
		name, choiceKey, spell string
		build                  func(picks ...string) Build
	}{
		{"the book's rituals", bookKey, "spell:detect-magic", func(p ...string) Build {
			return warlock(5, append([]string{"feature:pact-of-the-tome", bookInvocation}, p...)...)
		}},
		{"the Tome's cantrips", "feature:pact-of-the-tome#cantrips", "spell:guidance", func(p ...string) Build {
			return warlock(3, append([]string{"feature:pact-of-the-tome"}, p...)...)
		}},
		{"a Mystic Arcanum", "feature:mystic-arcanum-6th-level", "spell:eyebite", func(p ...string) Build {
			return warlock(11, p...)
		}},
		{"a bonus cantrip", "trait:high-elf-cantrip", "spell:fire-bolt", func(p ...string) Build {
			return choiceBuild("race:elf", "subrace:high-elf", ClassLevel{Class: "class:wizard", Level: 1}, p...)
		}},
	}
}

func TestTheSwitchesApplyToEverySpellChoice(t *testing.T) {
	t.Parallel()
	for _, row := range spellChoiceRows() {
		t.Run(row.name, func(t *testing.T) {
			t.Parallel()
			c := withOverlayOn(t, loadForTest(t), Overlay{Revision: 1, Off: []string{row.spell}})
			offered := func(c *Content, b Build) bool {
				return slices.Contains(optionKeys(choiceOf(t, c.Choices(b), row.choiceKey)), row.spell)
			}
			if !offered(c, row.build()) {
				t.Fatalf("the master is not offered %s", row.spell)
			}
			if offered(c.ForPlayer(), row.build()) {
				t.Errorf("a player is offered %s, which the master switched off", row.spell)
			}
			held := row.build(ScopedChoice(row.choiceKey, row.spell))
			ch := choiceOf(t, c.ForPlayer().Choices(held), row.choiceKey)
			if !slices.Contains(ch.Picked, row.spell) || slices.Contains(c.ForPlayer().Choices(held).NotOffered, ScopedChoice(row.choiceKey, row.spell)) {
				t.Errorf("a sheet that holds %s: picked %v, not offered %v; want the pick kept", row.spell, ch.Picked, c.ForPlayer().Choices(held).NotOffered)
			}
			if len(optionKeys(choiceOf(t, c.ForPlayer().Choices(row.build()), row.choiceKey))) == 0 {
				t.Error("a player is offered nothing: only the switched-off spell may go")
			}
		})
	}
}

// The spell of a pick is a key of the sheet, like a known spell.
func TestTheSpellOfAPickIsAKeyOfTheSheet(t *testing.T) {
	t.Parallel()
	for _, row := range spellChoiceRows() {
		t.Run(row.name, func(t *testing.T) {
			t.Parallel()
			c := withOverlayOn(t, loadForTest(t), Overlay{Revision: 1, Off: []string{row.spell}})
			b := row.build(ScopedChoice(row.choiceKey, row.spell))
			if got := c.OffKeys(b); !slices.Contains(got, row.spell) {
				t.Errorf("OffKeys = %v, want %s", got, row.spell)
			}
			var found bool
			for i, kf := range BuildKeys(b) {
				if kf.Key == row.spell {
					found = true
					if want := fmt.Sprintf("full.feature_choice_keys[%d]", slices.Index(b.FeatureChoices, ScopedChoice(row.choiceKey, row.spell))); kf.Field != want {
						t.Errorf("BuildKeys[%d] = %+v, want the field %s", i, kf, want)
					}
				}
			}
			if !found {
				t.Errorf("BuildKeys has no %s", row.spell)
			}
		})
	}
}
