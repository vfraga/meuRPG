package play

import (
	"testing"

	"github.com/PuraFome/meuRPG/backend/internal/rules"
)

// The master's line under an attack option: the action's name, attack bonus, damage and reach in
// meters (LegendaryOption.attack_detail_pt). Text and move options have none.
func TestLegendaryAttackDetails(t *testing.T) {
	t.Parallel()
	c, err := rules.LoadSRD()
	if err != nil {
		t.Fatalf("rules.LoadSRD() error = %v", err)
	}
	for _, tc := range []struct{ creature, option, want string }{
		{"monster:adult-red-dragon", "tail-attack", "Cauda: +14 · 2d8 + 8 de concussão · alcance 4,5 m"},
		{"monster:androsphinx", "claw-attack", "Garra: +12 · 2d10 + 6 de cortante · alcance 1,5 m"},
		{"monster:adult-red-dragon", "detect", ""},
		{"monster:adult-red-dragon", "wing-attack", ""},
		{"monster:vampire-bat", "move", ""},
	} {
		p, ok := c.LegendaryProfile(tc.creature)
		plan, pok := c.MonsterPlan(tc.creature)
		if !ok || !pok {
			t.Fatalf("%s: no profile or plan", tc.creature)
		}
		if got := legendaryAttackDetails(p, plan)[tc.option]; got != tc.want {
			t.Errorf("%s %s = %q, want %q", tc.creature, tc.option, got, tc.want)
		}
	}
}
