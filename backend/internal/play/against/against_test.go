package against

import (
	"slices"
	"testing"

	rulesv1 "github.com/PuraFome/meuRPG/backend/gen/meurpg/rules/v1"
	"github.com/PuraFome/meuRPG/backend/internal/play/link"
	"github.com/PuraFome/meuRPG/backend/internal/rules"
)

// What a saving throw is against, from its source: the racial advantages that hold against one kind of source apply
// to these.
func TestWhatASaveIsAgainst(t *testing.T) {
	sorted := func(l []string) []string { s := slices.Clone(l); slices.Sort(s); return s }
	tests := []struct {
		name string
		got  []string
		want []string
	}{
		{"a spell is magic", Spell(link.Spell{Key: "spell:sacred-flame"}), []string{"magic"}},
		{"a spell that deals poison damage", Spell(link.Spell{Key: "spell:poison-spray", Damages: []link.Dice{{DamageType: "damage-type:poison"}}}), []string{"magic", "poison"}},
		{"a spell that charms", Spell(link.Spell{Key: "spell:charm-person"}), []string{"charmed", "magic"}},
		{"a spell that frightens", Spell(link.Spell{Key: "spell:fear"}), []string{"frightened", "magic"}},
		{"an effect of a spell", Effect("spell:hold-person", &rules.EffectDef{Conditions: []string{"condition:paralyzed"}}), []string{"magic"}},
		{"an effect that gives fear", Effect("effect:dread", &rules.EffectDef{Conditions: []string{"condition:frightened"}}), []string{"frightened"}},
		{"an effect a creature action gives is no magic", Effect("monster:ghoul#claws", &rules.EffectDef{Conditions: []string{"condition:frightened"}}), []string{"frightened"}},
		{"an effect with no source known", Effect("effect:web-restrained", nil), nil},
		{"a trap's poison", Trap(&rulesv1.TrapEffect{Damage: []*rulesv1.TrapDamage{{DamageTypeKey: "damage-type:poison"}}}), []string{"poison"}},
		{"a trap that poisons", Trap(&rulesv1.TrapEffect{Conditions: []*rulesv1.TrapCondition{{ConditionKey: "condition:poisoned"}}}), []string{"poison"}},
		{"a trap of fire", Trap(&rulesv1.TrapEffect{Damage: []*rulesv1.TrapDamage{{DamageTypeKey: "damage-type:fire"}}}), nil},
	}
	for _, tt := range tests {
		if got := sorted(tt.got); !slices.Equal(got, sorted(tt.want)) {
			t.Errorf("%s: against %v, want %v", tt.name, got, tt.want)
		}
	}
}
