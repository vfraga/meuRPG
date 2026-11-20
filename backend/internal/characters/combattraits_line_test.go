package characters

import (
	"testing"

	"github.com/PuraFome/meuRPG/backend/internal/rules"
)

// The line of an extra_damage and the pact mark of a weapon line reach the combat as they are.
func TestTheLineFactsReachTheCombat(t *testing.T) {
	t.Parallel()
	d := rules.Derived{ExtraDamages: []rules.ExtraDamage{{Key: "feature:a@mesa", Weapon: rules.ExtraDamageAnyWeapon, Line: rules.LinePact}}}
	_, extras, _ := tableTraitsOf(d)
	if len(extras) != 1 || extras[0].Line != rules.LinePact {
		t.Errorf("extras = %+v, want one with the pact line", extras)
	}
	a := combatAttack(rules.Attack{Key: "equipment:rapier", Kind: "weapon", Pact: true})
	if !a.Pact || a.Key != "equipment:rapier" {
		t.Errorf("combatAttack = %+v, want the pact mark and the key", a)
	}
	if combatAttack(rules.Attack{Key: "equipment:dagger", Kind: "weapon"}).Pact {
		t.Error("an unmarked line is the pact weapon")
	}
}
