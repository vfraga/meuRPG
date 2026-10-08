package maps

import "testing"
import rulesv1 "github.com/PuraFome/meuRPG/backend/gen/meurpg/rules/v1"

type namedRules struct{ noRules }

func (namedRules) NamePT(string) string { return "Perfurante" }

// SRD 5.1 "Damage Severity by Level": a deadly trap at levels 17-20 is 24d10,
// and the app's own table (effects/traps.json) lists it.
func TestRulesReviewExpressions_TrapDamage24d10(t *testing.T) {
	t.Parallel()
	s := &Service{rules: namedRules{}}
	_, err := s.cleanDamage("damage[0]", &rulesv1.TrapDamage{Dice: "24d10", DamageTypeKey: "damage-type:piercing"})
	if err != nil {
		t.Errorf("cleanDamage(24d10) = %v, want it accepted", err)
	}
}
