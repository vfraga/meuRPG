package characters

import (
	"slices"
	"testing"

	charactersv1 "github.com/PuraFome/meuRPG/backend/gen/meurpg/characters/v1"
	"github.com/PuraFome/meuRPG/backend/internal/rules"
)

// The conditions a stat block is immune to ride on the traits of a creature and of an NPC made from
// it (MR-042, RN-29), where the combat reads them when it gives a condition. The Black Pudding is
// immune to prone and charmed, not to poisoned (SRD 5.1).
func TestStatBlockConditionImmunitiesReachTheTraits(t *testing.T) {
	t.Parallel()
	srd, err := rules.LoadSRD()
	if err != nil {
		t.Fatalf("LoadSRD: %v", err)
	}
	const pudding = "monster:black-pudding"
	creature, ok := creatureDerived(srd, pudding, "full")
	if !ok {
		t.Fatalf("creatureDerived(%s) not found", pudding)
	}
	npc := basicDerived(srd, &charactersv1.BasicSheet{MonsterKey: pudding})
	for name, d := range map[string]rules.Derived{"creature": creature, "NPC": npc} {
		var held []string
		for _, i := range traitsOf(srd, d).ConditionImmunities {
			held = append(held, i.Conditions...)
		}
		if !slices.Contains(held, "condition:prone") || !slices.Contains(held, "condition:charmed") {
			t.Errorf("%s: condition immunities = %v, want prone and charmed", name, held)
		}
		if slices.Contains(held, "condition:poisoned") { // the control: what the stat block does not list is not held
			t.Errorf("%s: condition immunities = %v, want no poisoned", name, held)
		}
	}
	goblin := basicDerived(srd, &charactersv1.BasicSheet{MonsterKey: "monster:goblin"})
	if got := traitsOf(srd, goblin).ConditionImmunities; len(got) != 0 {
		t.Errorf("a goblin holds condition immunities %v, want none", got)
	}
}
