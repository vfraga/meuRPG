package characters

import (
	"testing"

	"github.com/PuraFome/meuRPG/backend/internal/rules"
)

// The melee reach a weapon rule gives an attack line reaches the combat (link.Attack.ReachFt), so a
// thrown weapon is a melee attack up to that reach. No database is needed.
func TestCombatAttackCarriesTheMeleeReachOfTheLine(t *testing.T) {
	t.Parallel()
	spear := rules.Attack{Key: "weapon:spear", Kind: "weapon", Melee: true, RangeFt: 20, LongRangeFt: 60, ReachFt: 10}
	if got := combatAttack(spear).ReachFt; got != 10 {
		t.Errorf("combatAttack ReachFt = %d, want 10", got)
	}
	spear.ReachFt = 0
	if got := combatAttack(spear).ReachFt; got != 0 {
		t.Errorf("without a rule, combatAttack ReachFt = %d, want 0", got)
	}
}
