package play

import (
	"testing"

	"github.com/PuraFome/meuRPG/backend/internal/play/link"
	"github.com/PuraFome/meuRPG/backend/internal/play/playdb"
	"github.com/PuraFome/meuRPG/backend/internal/rules/combat"
	"github.com/PuraFome/meuRPG/backend/internal/rules/grid"
)

// A thrown melee weapon against a target within the attacker's melee reach is a melee attack; beyond the
// reach, up to its thrown range, it is a ranged attack made by throwing it (SRD 5.1, "Ranged Attacks").
// A weapon rule can lengthen the reach of the attack line (link.Attack.ReachFt).

// spear is a spear as the sheet derives it: thrown 20/60 ft, with the reach a weapon rule gives it, if any.
func spear(reachFt int) link.Attack {
	return link.Attack{Key: "weapon:spear", Name: "Lança", Weapon: true, Melee: true, Ability: "str", RangeFt: 20, LongRangeFt: 60, ReachFt: reachFt}
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

// A weapon rule's reach is the reach of an opportunity attack too (SRD 5.1: it happens when a creature
// leaves the attacker's reach): a spear with 10 ft provokes when the target leaves 10 ft, as a glaive does.
func TestAThrownWeaponWithALongReachProvokesWhereItLeavesTheReach(t *testing.T) {
	t.Parallel()
	glaive := link.Attack{Key: "weapon:glaive", Weapon: true, Melee: true, RangeFt: 10}
	for _, tc := range []struct {
		name   string
		attack link.Attack
		reach  int
	}{
		{"a spear with a 10 ft reach", spear(10), 10},
		{"a spear with no rule", spear(0), 5},
		{"a glaive", glaive, 10},
	} {
		if got := meleeReachOf(link.Sheet{Attacks: []link.Attack{tc.attack}}); got != tc.reach {
			t.Errorf("%s: meleeReachOf = %d, want %d", tc.name, got, tc.reach)
		}
	}
	reach := meleeReachOf(link.Sheet{Attacks: []link.Attack{spear(10)}})
	from, to := grid.Square{Col: 2, Row: 0}, grid.Square{Col: 3, Row: 0} // 10 ft to 15 ft from the spear at 0,0
	zero := int32(0)
	cands := []candidate{{who: playdb.Combatant{ID: "r", GridCol: &zero, GridRow: &zero}, reachFt: reach}}
	if got := provokedBy(cands, from, to); len(got) != 1 {
		t.Errorf("a target leaving 10 ft of a 10 ft spear provokes %d, want 1", len(got))
	}
	if got := attackReach(spear(10), true); got != 10 {
		t.Errorf("attackReach of the opportunity attack = %d, want 10", got)
	}
	if got := attackReach(spear(0), true); got != 5 {
		t.Errorf("attackReach of the opportunity attack with no rule = %d, want 5", got)
	}
	// The opportunity attack is melee at the reach, not back at 5 ft.
	f := &attackFlow{attack: spear(10), call: attackCall{asReaction: true}}
	if s := f.modeShape(); s.Thrown || s.RangeFt != 10 || s.MeleeReachFt != 10 || s.rangedAt(10, true) {
		t.Errorf("the opportunity attack's shape = %+v, want melee with a reach of 10", s)
	}
}

// One rule decides "thrown at range": the attack's shape. A spell attack is never a thrown weapon.
func TestThrownAtRangeFollowsTheShapeOfTheAttack(t *testing.T) {
	t.Parallel()
	at := func(col int32) playdb.Combatant {
		row := int32(0)
		return playdb.Combatant{GridCol: &col, GridRow: &row}
	}
	spell := link.Attack{Key: "spell:test", Spell: true, Melee: true, RangeFt: 20, LongRangeFt: 60}
	if thrownAtRange(playdb.Encounter{}, spell, at(0), at(3), false) {
		t.Error("a spell attack is a thrown weapon at 15 ft")
	}
	if !thrownAtRange(playdb.Encounter{}, spear(0), at(0), at(3), false) {
		t.Error("a spear is not thrown at 15 ft")
	}
}
