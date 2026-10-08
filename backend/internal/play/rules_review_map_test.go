package play

import "testing"

// TestRulesReviewMap_TurnOptionsMovementWithGrappled: a grappled or restrained
// creature has speed 0 (SRD 5.1), so GetTurnOptions and the combatant's own view
// must both say 0 movement left.
func TestRulesReviewMap_TurnOptionsMovementWithGrappled(t *testing.T) {
	t.Parallel()
	for _, key := range []string{"condition:grappled", "condition:restrained"} {
		a := newArmed(t)
		e := a.theatreThree(t)
		if _, err := a.conditions(t, a.master, e, "Toren", []string{key}, true, false); err != nil {
			t.Fatalf("SetCombatantConditions(%s) error = %v", key, err)
		}
		view := a.theatreCombatant(t, a.caio, "Toren")
		opts := a.mustOptions(t, a.caio, e, "Toren")
		m := opts.GetOptions().GetEconomy().GetMovement()
		if view.GetMovementLeftDft() != 0 || m.GetLeftDft() != 0 || view.GetMovementLeftDft() != m.GetLeftDft() ||
			view.GetMovementLeftFt() != m.GetLeftFt() {
			t.Errorf("%s: combatant movement_left_dft=%d movement_left_ft=%d; GetTurnOptions economy.movement left_dft=%d left_ft=%d speed_ft=%d; want all 0",
				key, view.GetMovementLeftDft(), view.GetMovementLeftFt(), m.GetLeftDft(), m.GetLeftFt(), m.GetSpeedFt())
		}
	}
}
