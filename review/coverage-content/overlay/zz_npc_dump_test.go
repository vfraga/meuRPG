package characters

import (
	"encoding/json"
	"os"
	"sort"
	"testing"

	"github.com/PuraFome/meuRPG/backend/internal/rules"
)

// Coverage inventory (not part of the repo): what "Criar NPC" keeps of each of the 334 stat blocks.
func TestCoverageDumpNpcSheets(t *testing.T) {
	c, err := rules.LoadSRD()
	if err != nil {
		t.Fatal(err)
	}
	type row struct {
		Key             string   `json:"key"`
		Attacks         []string `json:"attacks_on_sheet"`
		StatBlockAttack int      `json:"attack_actions"`
		PerAction       int      `json:"attacks_per_action"`
		DescriptionLine bool     `json:"extra_damage_in_description"`
		Err             string   `json:"err,omitempty"`
	}
	var keys []string
	list, _ := c.ListCreatures(rules.CreatureFilter{})
	for _, e := range list {
		keys = append(keys, e.Key)
	}
	sort.Strings(keys)
	var out []row
	for _, k := range keys {
		cr, _ := c.CreatureByKey(k)
		d, _ := c.MonsterDerived(k)
		r := row{Key: k, PerAction: d.AttacksPerAction}
		for _, a := range cr.Actions {
			if a.HasAttack {
				r.StatBlockAttack++
			}
		}
		b, err := npcSheetFromCreature(c, k)
		if err != nil {
			r.Err = err.Error()
		} else {
			for _, a := range b.GetAttacks() {
				r.Attacks = append(r.Attacks, a.GetName())
			}
			r.DescriptionLine = len(b.GetDescription()) > len("Baseado em "+cr.NamePT+" (SRD 5.1).")
		}
		out = append(out, r)
	}
	bt, _ := json.MarshalIndent(out, "", " ")
	if err := os.WriteFile(os.Getenv("COV_OUT"), bt, 0o644); err != nil {
		t.Fatal(err)
	}
}
