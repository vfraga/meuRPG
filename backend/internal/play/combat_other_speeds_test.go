package play

import (
	"testing"

	rulesv1 "github.com/PuraFome/meuRPG/backend/gen/meurpg/rules/v1"
)

// The other speeds of a character (#567): a table's speed.swim, speed.climb and speed.burrow modifiers reach the
// combatant when it joins, like the fly speed, for whoever reads that speed. The grid does not spend them: the
// movement left stays the walking speed's.
func TestTheOtherSpeedsOfASheetReachTheCombatant(t *testing.T) {
	t.Parallel()
	a := fxTable(t, fxTrait("Nadador da Mesa",
		&rulesv1.TableEffect{Type: "modifier", Target: "speed.swim", Mode: "set", Value: "speedwalk()"},
		&rulesv1.TableEffect{Type: "modifier", Target: "speed.climb", Mode: "set", Value: "15"},
		&rulesv1.TableEffect{Type: "modifier", Target: "speed.burrow", Mode: "set", Value: "10", When: "level() > 10"},
	))
	e := fxFight(t, a)
	for name, u := range map[string]*user{"master": a.master, "owner": a.caio} {
		who := byLabel(t, a.get(t, u), "Ragna")
		if who.GetSpeedSwimFt() != 30 || who.GetSpeedClimbFt() != 15 || who.GetSpeedBurrowFt() != 0 {
			t.Errorf("%s reads swim %d, climb %d, burrow %d; want 30, 15, 0 (the burrow waits for level 11)", name, who.GetSpeedSwimFt(), who.GetSpeedClimbFt(), who.GetSpeedBurrowFt())
		}
		if who.GetSpeedFt() != 30 || who.GetSpeedFlyFt() != 0 {
			t.Errorf("%s reads walk %d, fly %d; want 30, 0", name, who.GetSpeedFt(), who.GetSpeedFlyFt())
		}
	}
	// Another player reads no speed of Ragna, the walking one included: the same audience as speed_fly_ft.
	other := byLabel(t, a.get(t, a.ana), "Ragna")
	if other.GetSpeedFt() != 0 || other.GetSpeedFlyFt() != 0 || other.GetSpeedSwimFt() != 0 || other.GetSpeedClimbFt() != 0 || other.GetSpeedBurrowFt() != 0 {
		t.Errorf("another player reads speeds %d/%d/%d/%d/%d, want none", other.GetSpeedFt(), other.GetSpeedFlyFt(), other.GetSpeedSwimFt(), other.GetSpeedClimbFt(), other.GetSpeedBurrowFt())
	}
	// Nothing spends them: the movement the turn has is the walking speed's.
	if got := byLabel(t, e, "Ragna").GetSpeedDft(); got != 300 {
		t.Errorf("speed_dft = %d, want 300 (the walking speed)", got)
	}
	// The row keeps them for the master's other reads (a form, a creature) to copy.
	var swim, climb, burrow int32
	if err := a.h.pool.QueryRow(t.Context(), `SELECT speed_swim_ft, speed_climb_ft, speed_burrow_ft FROM combatants WHERE id = $1`, a.id(t, "Ragna")).Scan(&swim, &climb, &burrow); err != nil || swim != 30 || climb != 15 || burrow != 0 {
		t.Errorf("stored swim %d, climb %d, burrow %d (%v); want 30, 15, 0", swim, climb, burrow, err)
	}
}

// A Wild Shape form carries its own other speeds to the combatant and the druid's own come back when it ends.
func TestAWildShapeFormCarriesItsOtherSpeedsToTheCombatant(t *testing.T) {
	t.Parallel()
	s := newShapers(t)
	s.fight(t)
	res := s.mustAssume(t, s.bia, s.bri, "monster:crocodile")
	if got := byLabel(t, res.GetEncounter(), "Sálvia"); got.GetSpeedSwimFt() != 30 || got.GetSpeedFt() != 20 {
		t.Errorf("the crocodile form reads walk %d, swim %d; want 20 and 30", got.GetSpeedFt(), got.GetSpeedSwimFt())
	}
	left := s.mustLeave(t, s.bia, s.bri)
	if got := byLabel(t, left.GetEncounter(), "Sálvia"); got.GetSpeedSwimFt() != 0 || got.GetSpeedFt() != 30 {
		t.Errorf("after the form she reads walk %d, swim %d; want her own 30 and 0", got.GetSpeedFt(), got.GetSpeedSwimFt())
	}
}
