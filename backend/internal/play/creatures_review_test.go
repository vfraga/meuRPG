package play

import (
	"testing"

	playv1 "github.com/PuraFome/meuRPG/backend/gen/meurpg/play/v1"
	rulesv1 "github.com/PuraFome/meuRPG/backend/gen/meurpg/rules/v1"
)

// Review of the creatures rules (SRD "Damage Resistance and Vulnerability"). They need
// the database (MEURPG_TEST_DATABASE_URL).

const poisonSpray = "spell:poison-spray"

// skeletonFight is a theatre combat with a Skeleton (13 hit points, vulnerable to
// bludgeoning, immune to poison) added by the master. Toren (a fighter with a mace) and
// Pensantus (a wizard with Poison Spray) are first in the order.
func skeletonFight(t *testing.T) (*armed, *playv1.Encounter, string) {
	t.Helper()
	scores := &rulesv1.AbilityScores{Strength: 16, Dexterity: 13, Constitution: 14, Intelligence: 16, Wisdom: 10, Charisma: 8}
	a := newArmedWith(t, func(a *armed) {
		a.toren = a.caio.hero(t, a.campaignID, "Toren", "class:fighter", "race:human", 2, scores, []string{maceKey}, nil)
		a.pens = a.ana.hero(t, a.campaignID, "Pensantus", "class:wizard", "race:gnome", 1, scores, nil, []string{poisonSpray})
		a.bri = a.bia.hero(t, a.campaignID, "Brisa", "class:fighter", "race:human", 2, scores, []string{rapier}, nil)
	})
	e := a.monsterSetup(t, true)
	a.h.roller.queue(1)
	res := a.mustAddMonsters(t, e, func(r *playv1.AddMonstersRequest) {
		r.CreatureKey = "monster:skeleton"
		r.Count = 1
		r.Hidden = new(false)
	})
	label := ""
	for _, c := range res.GetEncounter().GetCombatants() {
		if c.GetId() == res.GetCombatantIds()[0] {
			label = c.GetLabel()
		}
	}
	if label == "" {
		t.Fatal("the skeleton is not in the combat")
	}
	e = a.begin(t, a.get(t, a.master))
	if c := byLabel(t, e, label); c.GetHitPointsCurrent() != 13 {
		t.Fatalf("%s has %d hit points, want 13", label, c.GetHitPointsCurrent())
	}
	return a, e, label
}

// TestRulesReviewCreatures_MonsterVulnerabilityIgnored: a mace hit (bludgeoning) on a
// Skeleton (vulnerable to bludgeoning) must do double damage: 5 rolled, 10 taken.
func TestRulesReviewCreatures_MonsterVulnerabilityIgnored(t *testing.T) {
	t.Parallel()
	a, e, skel := skeletonFight(t)
	key := a.mustOptions(t, a.caio, e, "Toren").GetOptions().GetAttacks()[0].GetAttack().GetKey()
	hit := a.mustAttack(t, a.caio, e, "Toren", key, skel, d20(15))
	pending := hit.GetPendingDamage().GetId()
	dmg := a.mustDamage(t, a.caio, e, pending, typedDamage(3)).GetPendingDamage()
	if dmg.GetDamageTypeKey() != "damage-type:bludgeoning" || dmg.GetAmount() < 1 {
		t.Fatalf("pending damage = %v, want bludgeoning", dmg)
	}
	hp, _, _ := a.hp(t, skel)
	if want := 13 - 2*dmg.GetAmount(); hp != max(want, 0) {
		t.Errorf("Skeleton has %d hit points after %d bludgeoning damage, want %d (vulnerable: double); the damage is applied in full", hp, dmg.GetAmount(), max(want, 0))
	}
}

// TestRulesReviewCreatures_MonsterPoisonImmunityIgnored: Poison Spray on a Skeleton
// (immune to poison) must do no damage, and the Skeleton must not lose hit points.
func TestRulesReviewCreatures_MonsterPoisonImmunityIgnored(t *testing.T) {
	t.Parallel()
	a, e, skel := skeletonFight(t)
	e = a.passTo(t, e, "Pensantus")
	// The skeleton fails its save (d20 = 1 queued), damage dice are 2d12.
	a.h.roller.queue(1, 6, 6)
	res, err := a.cast(t, a.ana, e, "Pensantus", poisonSpray, nil, a.at(t, skel), noCastRoll)
	if err != nil {
		t.Fatalf("CastSpell(Poison Spray) error = %v", err)
	}
	if len(res.GetCast().GetPendingDamages()) != 1 {
		t.Fatalf("cast = %v, want one pending poison damage", res.GetCast())
	}
	dmg := a.mustDamage(t, a.ana, e, res.GetCast().GetPendingDamages()[0].GetId(), typedDamage(6)).GetPendingDamage()
	t.Logf("poison damage rolled/applied: type %s amount %d status %v", dmg.GetDamageTypeKey(), dmg.GetAmount(), dmg.GetStatus())
	if hp, _, _ := a.hp(t, skel); hp != 13 {
		t.Errorf("Skeleton has %d hit points after Poison Spray, want 13 (immune to poison); damage is applied in full", hp)
	}
}
