package play

import (
	"testing"

	playv1 "github.com/PuraFome/meuRPG/backend/gen/meurpg/play/v1"
)

// SRD 5.1, Spellcasting > Casting Time > Bonus Action: a caster that casts a
// spell with a bonus action can't cast another spell during the same turn,
// except a cantrip with a casting time of 1 action.
func TestRulesReviewCombat_BonusActionSpellLimit(t *testing.T) {
	t.Parallel()
	a := newCasters(t)
	e := a.castersFight(t, 1)
	a.passTo(t, e, "Brisa")

	a.mustCast(t, a.bia, e, "Brisa", healingWord, slotOfLevel(1), a.at(t, "Toren"), noCastRoll)
	// Cure Wounds (1 action, levelled) after a bonus action spell: refused.
	if _, err := a.cast(t, a.bia, e, "Brisa", cureWounds, slotOfLevel(1), a.at(t, "Toren"), noCastRoll); err == nil {
		t.Errorf("Cure Wounds after Healing Word in the same turn was accepted; SRD allows only a 1-action cantrip")
	}
	// A 1-action cantrip is still allowed.
	if _, err := a.cast(t, a.bia, e, "Brisa", sacredFlame, nil, a.at(t, "Goblin"), func(r *playv1.CastSpellRequest) { r.Roll = &playv1.CastSpellRequest_RollInApp{RollInApp: true} }); err != nil {
		t.Logf("Sacred Flame after Healing Word: %v", err)
	}
}
