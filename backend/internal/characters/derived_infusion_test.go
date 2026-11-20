package characters

import (
	"testing"

	"github.com/PuraFome/meuRPG/backend/internal/rules"
)

const (
	infusionGumeKey = "feature:class-engenhista-de-li-6d59b9--gume-vivo@mesa"
	infusionSword   = "equipment:longsword"
)

// An infusion record of the fixture's class gives its numbers and its name through Derive, and the name reaches the
// attack, the item modifiers and the gear entry on the wire.
func TestTheInfusionNameReachesTheAttackTheModifiersAndTheGearEntry(t *testing.T) {
	t.Parallel()
	c := infusionContent(t)
	b := rules.Build{
		BaseScores: map[rules.Ability]int{rules.STR: 15, rules.DEX: 14, rules.CON: 13, rules.INT: 12, rules.WIS: 10, rules.CHA: 8},
		Race:       "race:human", Background: "background:acolyte",
		Classes:        []rules.ClassLevel{{Class: infusionClassKey, Level: 10}},
		Weapons:        []string{infusionSword},
		FeatureChoices: []string{infusionGumeKey},
		Infusions: []rules.InfusionRecord{{
			ID: "inf-1", InfusionKey: infusionGumeKey, ItemKind: rules.InfusionItemGear, ItemKey: infusionSword,
		}},
	}
	d := rules.Derive(b, c)
	if len(d.InfusionHosts) != 1 || d.InfusionHosts[0].Limit != 4 || d.InfusionHosts[0].Used != 1 || d.InfusionHosts[0].ClassLevel != 10 {
		t.Fatalf("hosts = %+v, want limit 4 and used 1 at class level 10", d.InfusionHosts)
	}
	sheet := derivedToProto(d)
	var found bool
	for _, a := range sheet.GetAttacks() {
		if a.GetKey() == infusionSword {
			found = true
			if a.GetInfusionNamePt() != "Gume Vivo" || a.GetMagicBonus() != 2 {
				t.Errorf("attack = name %q, magic bonus %d, want Gume Vivo and 2", a.GetInfusionNamePt(), a.GetMagicBonus())
			}
		} else if a.GetInfusionNamePt() != "" {
			t.Errorf("%s names an infusion", a.GetKey())
		}
	}
	if !found {
		t.Fatal("no attack of the sword")
	}
	if len(sheet.GetItemModifiers()) != 2 {
		t.Fatalf("modifiers = %v, want attack and damage", sheet.GetItemModifiers())
	}
	for _, m := range sheet.GetItemModifiers() {
		if m.GetInfusionNamePt() != "Gume Vivo" || m.GetInfusionId() != "inf-1" || m.GetValue() != 2 || !m.GetCounts() {
			t.Errorf("modifier = %v", m)
		}
	}
	gear := gearToProto(rules.DeriveGear(b, c, d)[0])
	if gear.GetInfusionNamePt() != "Gume Vivo" {
		t.Errorf("gear entry names %q", gear.GetInfusionNamePt())
	}
}
