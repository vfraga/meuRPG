package characters

import (
	"testing"

	rulesv1 "github.com/PuraFome/meuRPG/backend/gen/meurpg/rules/v1"
)

// The line of a weapon_rule or extra_damage (TableEffect.line) reaches the engine and is checked at its field.

func TestTableEffectsOfReadsTheLine(t *testing.T) {
	t.Parallel()
	got := tableEffectsOf([]*rulesv1.TableEffect{
		{Type: "weapon_rule", Weapon: "any", AttackBonus: 1, Line: "pact"},
		{Type: "extra_damage", Applies: "any_target", Value: "1", Line: "Lançador"},
	})
	if got[0].Line != "pact" || got[1].Line != "Lançador" {
		t.Errorf("lines = %q, %q, want pact and Lançador", got[0].Line, got[1].Line)
	}
}

func TestALineThatNamesNothingIsRefusedAtItsField(t *testing.T) {
	t.Parallel()
	h := newHarness(t)
	master := h.newUser("Samuel")
	campaign := h.newCampaign(master, "Mirathel")
	for name, e := range map[string]*rulesv1.TableEffect{
		"a weapon rule":   {Type: "weapon_rule", Weapon: "any", AttackBonus: 1, Line: "Sem Linha"},
		"an extra damage": {Type: "extra_damage", Applies: "any_target", Value: "1", Line: "Sem Linha"},
	} {
		body := testClass("Outra")
		body.Levels[0].Features[0].Effects = []*rulesv1.TableEffect{e}
		vs := master.refused(t, campaign, body)
		if len(vs) != 1 || vs[0].GetField() != "table_class.levels[0].features[0].effects[0].line" || vs[0].GetReason() != "dangling_reference" {
			t.Errorf("%s: violations %v, want line refused (dangling_reference)", name, vs)
		}
	}
}
