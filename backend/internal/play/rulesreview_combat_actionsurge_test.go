package play

import (
	"testing"

	playv1 "github.com/PuraFome/meuRPG/backend/gen/meurpg/play/v1"
	rulesv1 "github.com/PuraFome/meuRPG/backend/gen/meurpg/rules/v1"
)

// TestRulesReviewCombat_ActionSurgeOncePerTurn: SRD 5.1 Fighter, Action Surge:
// at level 17 the fighter can use it twice before a rest, but only once on the
// same turn.
func TestRulesReviewCombat_ActionSurgeOncePerTurn(t *testing.T) {
	t.Parallel()
	a := newArmedWith(t, func(a *armed) {
		a.toren = a.caio.hero(t, a.campaignID, "Toren", "class:fighter", "race:human", 17,
			&rulesv1.AbilityScores{Strength: 16, Dexterity: 13, Constitution: 14, Intelligence: 10, Wisdom: 10, Charisma: 8}, []string{battleaxe}, nil)
		a.pens = a.ana.caster(t, a.campaignID, "Pensantus", "class:wizard", "race:gnome", 3,
			&rulesv1.AbilityScores{Strength: 10, Dexterity: 14, Constitution: 12, Intelligence: 16, Wisdom: 10, Charisma: 8}, nil, []string{fireBolt},
			[]string{magicMissileSpell}, []string{magicMissileSpell})
		a.bri = a.bia.caster(t, a.campaignID, "Brisa", "class:cleric", "race:human", 3,
			&rulesv1.AbilityScores{Strength: 10, Dexterity: 16, Constitution: 14, Intelligence: 10, Wisdom: 16, Charisma: 8}, []string{maceKey}, []string{sacredFlame}, nil,
			[]string{cureWounds})
	})
	e := a.castersFight(t, 1)
	a.mustEndTurn(t, a.ana, e) // Toren's turn

	if _, err := a.feature(t, a.caio, e, "Toren", actionSurgeKey, noTakeRoll); err != nil {
		t.Fatalf("first Action Surge error = %v, want it allowed", err)
	}
	if used, total := resourceUsed(a.vitals(t, a.toren), "action_surge"); used != 1 || total != 2 {
		t.Fatalf("action_surge used = %d of %d, want 1 of 2 at level 17", used, total)
	}
	if _, err := a.feature(t, a.caio, e, "Toren", actionSurgeKey, noTakeRoll); err == nil {
		t.Errorf("second Action Surge on the SAME turn was accepted; SRD allows only one use per turn")
	}

	// Control: a later turn may use the second charge.
	if used, _ := resourceUsed(a.vitals(t, a.toren), "action_surge"); used == 2 {
		// the buggy second use already spent it; reset to isolate the per-turn rule
		a.correct(t, a.toren, func(r *playv1.AdjustCharacterVitalsRequest) {
			r.ResourcesUsed = []*playv1.ResourceUsed{{Key: "action_surge", Used: 1}}
		})
	}
	a.passTo(t, e, "Pensantus")
	a.passTo(t, e, "Toren")
	if _, err := a.feature(t, a.caio, e, "Toren", actionSurgeKey, noTakeRoll); err != nil {
		t.Errorf("Action Surge on a later turn error = %v, want it allowed", err)
	}
}
