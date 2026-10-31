package play

import (
	"math"

	"github.com/PuraFome/meuRPG/backend/internal/play/link"
	"github.com/PuraFome/meuRPG/backend/internal/play/playdb"
	"github.com/PuraFome/meuRPG/backend/internal/rules/grid"
)

// How far an attack or a spell reaches on the grid.

// meleeReachFt is the reach of a melee attack that says none: 5 ft.
const meleeReachFt = 5

// reachFt is how far an attack reaches: its range, its long range, or 5 ft
// for a melee attack that says none.
func reachFt(rangeFt, longRangeFt int32) int32 {
	return max(rangeFt, longRangeFt, meleeReachFt)
}

// attackMeleeReach is the melee reach of an attack: 5 ft, or the longer reach a weapon rule
// gives its line.
func attackMeleeReach(a link.Attack) int {
	return max(meleeReachFt, a.ReachFt)
}

// attackReach is how far the attack reaches for a player: its range, or for an
// opportunity attack the melee reach, whatever range the weapon has when thrown.
func attackReach(a link.Attack, asReaction bool) int32 {
	if asReaction {
		return clamp32(attackMeleeReach(a), 0, math.MaxInt32)
	}
	return max(reachFt(clamp32(a.RangeFt, 0, math.MaxInt32), clamp32(a.LongRangeFt, 0, math.MaxInt32)), clamp32(a.ReachFt, 0, math.MaxInt32))
}

// distanceFt is the distance between two combatants on the grid for an attack's
// reach or a spell's range: the squares between the centers, rounded down, times
// 5 ft (RN-21, grid.RangeFt), so a diagonal neighbor is 5 ft away; false when
// either has no square.
func distanceFt(a, b playdb.Combatant) (int32, bool) {
	if !placed(a) || !placed(b) {
		return 0, false
	}
	return clamp32(grid.RangeFt(squareOfCombatant(a), squareOfCombatant(b)), 0, math.MaxInt32), true
}
