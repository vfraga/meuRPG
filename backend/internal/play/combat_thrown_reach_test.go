package play

import (
	"testing"

	"github.com/PuraFome/meuRPG/backend/internal/play/link"
	"github.com/PuraFome/meuRPG/backend/internal/play/playdb"
	"github.com/PuraFome/meuRPG/backend/internal/rules/combat"
)

// A thrown melee weapon against a target within the attacker's melee reach is a melee attack; beyond the
// reach, up to its thrown range, it is a ranged attack made by throwing it (SRD 5.1, "Ranged Attacks").
// A weapon rule can lengthen the reach of the attack line (link.Attack.ReachFt).

// spear is a spear as the sheet derives it: thrown 20/60 ft, with the reach a weapon rule gives it, if any.
func spear(reachFt int) link.Attack {
	return link.Attack{Key: "weapon:spear", Name: "Lança", Weapon: true, Melee: true, Ability: "str", RangeFt: max(20, reachFt), LongRangeFt: 60, ReachFt: reachFt}
}

func TestAThrownWeaponIsMeleeUpToTheReachOfItsAttackLine(t *testing.T) {
	t.Parallel()
	for _, tc := range []struct {
		name   string
		attack link.Attack
		distFt int32
		want   bool // thrown, so ranged
	}{
		{"10 ft reach, target at 5 ft", spear(10), 5, false},
		{"10 ft reach, target at 10 ft", spear(10), 10, false},
		{"10 ft reach, target at 15 ft", spear(10), 15, true},
		{"no rule, target at 5 ft", spear(0), 5, false},
		{"no rule, target at 10 ft", spear(0), 10, true},
		{"20 ft reach, target at 20 ft", spear(20), 20, false},
		{"20 ft reach, target at 25 ft", spear(20), 25, true},
	} {
		shape := shapeOfAttack(tc.attack)
		if !shape.Thrown {
			t.Errorf("%s: the spear is not read as a thrown weapon", tc.name)
		}
		if got := shape.rangedAt(tc.distFt, true); got != tc.want {
			t.Errorf("%s: rangedAt = %v, want %v", tc.name, got, tc.want)
		}
	}
}

func TestAThrownWeaponReachDecidesTheHitFacts(t *testing.T) {
	t.Parallel()
	at := func(id string, col int32) playdb.Combatant {
		row := int32(0)
		return playdb.Combatant{ID: id, GridCol: &col, GridRow: &row}
	}
	for _, tc := range []struct {
		name   string
		attack link.Attack
		col    int32 // the target's square: 5 ft each
		want   bool
	}{
		{"10 ft reach, target at 10 ft", spear(10), 2, false},
		{"10 ft reach, target at 15 ft", spear(10), 3, true},
		{"no rule, target at 5 ft", spear(0), 1, false},
		{"no rule, target at 10 ft", spear(0), 2, true},
		{"20 ft reach, target at 20 ft", spear(20), 4, false},
		{"20 ft reach, target at 25 ft", spear(20), 5, true},
	} {
		if got := thrownAtRange(playdb.Encounter{}, tc.attack, at("a", 0), at("t", tc.col), false); got != tc.want {
			t.Errorf("%s: thrownAtRange = %v, want %v", tc.name, got, tc.want)
		}
	}
}

func TestAThrownWeaponWithALongReachIsRolledAsMeleeInTheScene(t *testing.T) {
	t.Parallel()
	at := func(id string, col int32, side string) playdb.Combatant {
		row := int32(0)
		return playdb.Combatant{ID: id, Side: side, GridCol: &col, GridRow: &row}
	}
	attacker, target, foe := at("a", 0, "party"), at("t", 0, "foes"), at("f", 1, "foes")
	for _, tc := range []struct {
		name       string
		attack     link.Attack
		col        int32
		wantRanged bool
	}{
		{"10 ft reach, target at 10 ft", spear(10), 2, false},
		{"10 ft reach, target at 15 ft", spear(10), 3, true},
		{"no rule, target at 10 ft", spear(0), 2, true},
	} {
		tg := target
		tg.GridCol = &tc.col
		f := modeFacts{cs: []playdb.Combatant{attacker, tg, foe}}
		s := f.scene(attacker, tg, link.Traits{}, shapeOfAttack(tc.attack), true, nil)
		if s.Ranged != tc.wantRanged {
			t.Errorf("%s: scene.Ranged = %v, want %v", tc.name, s.Ranged, tc.wantRanged)
		}
		if s.HostileNearAttacker != tc.wantRanged {
			t.Errorf("%s: the enemy within 5 ft counts = %v, want %v (only a throw is disadvantaged by it)", tc.name, s.HostileNearAttacker, tc.wantRanged)
		}
	}
}

// Without a map the distance is unknown. A thrown weapon with the usual reach is in melee, so a hit on a
// paralyzed target is a critical hit; one with a longer reach has no known "within 5 ft", like any reach
// weapon, and is not.
func TestAThrownWeaponWithALongReachHasNoKnownDistanceWithoutAMap(t *testing.T) {
	t.Parallel()
	attacker := playdb.Combatant{ID: "a", Side: "party"}
	target := playdb.Combatant{ID: "t", Side: "foes", Conditions: []string{"condition:paralyzed"}}
	for _, tc := range []struct {
		name   string
		attack link.Attack
		want   bool
	}{
		{"no rule", spear(0), true},
		{"10 ft reach", spear(10), false},
	} {
		s := modeFacts{cs: []playdb.Combatant{attacker, target}, theatre: true}.scene(attacker, target, link.Traits{}, shapeOfAttack(tc.attack), true, nil)
		if s.Ranged {
			t.Errorf("%s: a thrown weapon without a map is ranged", tc.name)
		}
		if got := combat.AttackMode(s).CriticalOnHit; got != tc.want {
			t.Errorf("%s: critical on hit = %v, want %v", tc.name, got, tc.want)
		}
	}
}
