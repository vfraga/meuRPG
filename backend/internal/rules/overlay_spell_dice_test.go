package rules

import (
	"errors"
	"fmt"
	"slices"
	"strings"
	"testing"

	"github.com/PuraFome/meuRPG/backend/internal/platform/dice"
)

// The dice of a table spell's damage or healing
// must be a die that exists at a table (the SRD uses d4..d12; the dice package
// accepts 4, 6, 8, 10, 12, 20 and 100). With checks only the count today, so a
// 1d200 spell silently rolls as 1d100 (play clamps the sides to 100) and a d7
// cannot be rolled with real dice.
func TestTableSpellDiceAreDiceThatExist(t *testing.T) {
	t.Parallel()
	base := loadForTest(t)

	// refused requires With to refuse the spell with a violation whose Field
	// ends in one of the wanted suffixes.
	refused := func(t *testing.T, ts TableSpell, suffixes ...string) {
		t.Helper()
		o := fullOverlay(t, base)
		o.Spells = []TableSpell{ts}
		_, err := base.With(o)
		if err == nil {
			t.Fatalf("With accepted a spell with an impossible die: damage=%+v heal=%+v", ts.Damage, ts.Heal)
		}
		var oe *OverlayError
		if !errors.As(err, &oe) {
			t.Fatalf("error is %T, want *OverlayError: %v", err, err)
		}
		for _, v := range oe.Violations() {
			for _, s := range suffixes {
				if strings.HasSuffix(v.Field, s) {
					return
				}
			}
		}
		t.Fatalf("no violation ends in %v: %v", suffixes, err)
	}
	damage := func(dice, perSlot string) TableSpell {
		ts := tableTestSpells()[0] // bolt, level 1
		ts.Damage = []TableSpellDamage{{Type: "damage-type:fire", Dice: dice, PerSlotLevel: perSlot}}
		return ts
	}

	for _, tc := range []struct{ name, dice string }{
		{"101 sides (play clamps to 100)", "1d101"},
		{"200 sides", "1d200"},
		{"2000000000 sides (Count*Sides overflows int32)", "30d2000000000"},
	} {
		t.Run("hard/damage/"+tc.name, func(t *testing.T) {
			t.Parallel()
			refused(t, damage(tc.dice, ""), ".dice")
		})
	}
	t.Run("hard/per_slot_level", func(t *testing.T) {
		t.Parallel()
		refused(t, damage("1d101", "1d101"), ".dice", ".per_slot_level")
	})
	t.Run("hard/heal", func(t *testing.T) {
		t.Parallel()
		ts := tableTestSpells()[3]
		ts.Heal = &TableSpellHeal{Dice: "1d2000000000"}
		refused(t, ts, ".heal.dice")
	})
	t.Run("hard/cantrip per_tier", func(t *testing.T) {
		t.Parallel()
		ts := tableTestSpells()[4]
		ts.Damage = []TableSpellDamage{{Type: "damage-type:lightning", Dice: "1d101", PerTier: "1d101"}}
		refused(t, ts, ".dice", ".per_tier")
	})
	// Softer: not a real table die, but below the dice package's cap.
	t.Run("soft/d7", func(t *testing.T) {
		t.Parallel()
		refused(t, damage("1d7", ""), ".dice")
	})
	t.Run("soft/d1", func(t *testing.T) {
		t.Parallel()
		refused(t, damage("1d1", ""), ".dice")
	})
	// Control: real table dice stay accepted.
	t.Run("control/real dice", func(t *testing.T) {
		t.Parallel()
		for _, d := range []string{"1d4", "1d6", "1d8", "1d10", "1d12", "1d20", "1d100"} {
			o := fullOverlay(t, base)
			o.Spells = []TableSpell{damage(d, "")}
			if _, err := base.With(o); err != nil {
				t.Errorf("%s refused: %v", d, err)
			}
		}
	})
}

// tableDiceSides must stay the list of dice the dice package rolls.
func TestTableDiceSidesAreTheDicePackages(t *testing.T) {
	t.Parallel()
	for sides := 1; sides <= 101; sides++ {
		_, err := dice.Parse(fmt.Sprintf("1d%d", sides))
		if rolls := err == nil; rolls != slices.Contains(tableDiceSides, sides) {
			t.Errorf("d%d: the dice package rolls it = %v, tableDiceSides has it = %v", sides, rolls, !rolls)
		}
	}
}
