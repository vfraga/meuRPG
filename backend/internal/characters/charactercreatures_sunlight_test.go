package characters

import (
	"testing"

	charactersv1 "github.com/PuraFome/meuRPG/backend/gen/meurpg/characters/v1"
	"github.com/PuraFome/meuRPG/backend/internal/rules"
)

// A stat block's Sunlight Sensitivity and Sunlight Weakness reach the traits of a creature and of an NPC made from it,
// each as the trait it is: the drow has the Sensitivity, the shadow the Weakness, the goblin neither.
func TestSunlightTraitsReachTheTraits(t *testing.T) {
	t.Parallel()
	srd, err := rules.LoadSRD()
	if err != nil {
		t.Fatalf("LoadSRD: %v", err)
	}
	for key, want := range map[string][2]bool{
		"monster:drow":   {true, false},
		"monster:kobold": {true, false},
		"monster:shadow": {false, true},
		"monster:goblin": {false, false},
	} {
		creature, ok := creatureDerived(srd, key, "full")
		if !ok {
			t.Fatalf("creatureDerived(%s) not found", key)
		}
		npc := basicDerived(srd, &charactersv1.BasicSheet{MonsterKey: key})
		for who, d := range map[string]rules.Derived{"creature": creature, "NPC": npc} {
			got := traitsOf(srd, d)
			if got.SunlightSensitivity != want[0] || got.SunlightWeakness != want[1] {
				t.Errorf("%s %s: Sensitivity, Weakness = %v, %v, want %v, %v", key, who, got.SunlightSensitivity, got.SunlightWeakness, want[0], want[1])
			}
		}
	}
}
