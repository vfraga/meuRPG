package rules

import (
	"fmt"
	"slices"
	"strings"
)

// armorClass computes the AC with the worn armor and shield:
//
//   - no armor: 10 + DEX;
//   - light armor: base + DEX; medium: base + DEX up to +2 (the armor's
//     max_dex_bonus); heavy: base;
//   - "ac.base" effects offer other bases, such as Unarmored Defense
//     (10 + DEX + CON while wearing no armor), and the best one wins;
//   - a shield adds its +2, and "ac" effects add on top (Defense style).
//
// Spells such as Mage Armor or Shield are not counted: they last a while and
// belong to the game session (Etapa 6).
func (x *deriver) armorClass() {
	c := x.c
	name := "Sem armadura"
	if x.armor != nil {
		name = c.namePT(x.b.Armor)
		if x.armorItem != nil {
			name = x.armorItem.name
		}
	}

	base := 10 + x.mods[DEX]
	if a := x.armor; a != nil {
		base = a.BaseAC
		if a.DexBonus {
			dex := x.mods[DEX]
			if a.MaxDexBonus > 0 {
				dex = min(dex, a.MaxDexBonus)
			}
			base += dex
		}
		base += x.armorItemBonus()
		if !x.armorProficient(x.armorCategory, x.b.Armor) && !x.armorItemProficient() {
			x.issue(IssueArmorProficiency, "full.armor_key", "Sem proficiência em %s: desvantagem em testes, ataques e testes de resistência de FOR e DES, e não conjura magias.", strings.ToLower(name))
		}
		if a.StealthDisadvantage {
			x.d.Hints = append(x.d.Hints, Hint{
				Source: x.b.Armor, Target: "skill:stealth", Targets: []string{"skill:stealth"}, Mode: "disadvantage",
				TextPT: fmt.Sprintf("Desvantagem em Furtividade com %s.", strings.ToLower(name)),
			})
		}
		if a.StrMinimum > 0 && x.scores[STR] < a.StrMinimum {
			x.d.Hints = append(x.d.Hints, Hint{
				Source: x.b.Armor, Target: "speed.walk", Targets: []string{"speed.walk"}, Mode: "note",
				TextPT: fmt.Sprintf("%s pede FOR %d: com menos, o deslocamento cai 10 pés (anões não perdem).", name, a.StrMinimum),
			})
		}
	}

	// Other bases from effects: the best one wins, and the description
	// names the feature that gave it.
	for _, a := range x.active {
		e := a.effect
		if e.Type != "modifier" || e.Target != "ac.base" || len(e.Tags) > 0 || !x.applies(a) {
			continue
		}
		v, ok := x.value(a)
		if !ok {
			continue
		}
		switch {
		case e.Mode == "set", e.Mode == "max" && v > base:
			base = v
			name = c.namePT(a.owner)
		case e.Mode == "add":
			base += v
		}
	}

	ac := base
	if x.b.Shield {
		ac += 2 + x.shieldItemBonus()
		name += " + escudo"
		if x.shieldItem != nil && x.shieldItem.def != nil {
			name = strings.TrimSuffix(name, " + escudo") + " + " + x.shieldItem.name
		}
		if !x.armorProficient("shield", "equipment:shield") {
			x.issue(IssueArmorProficiency, "full.shield", "Sem proficiência em escudos.")
		}
	}
	x.d.ArmorClass = x.modifiers("ac", ac)
	x.d.ArmorClassDescription = name
}

// resolveArmor finds the worn armor. It runs before any effect, because
// conditions such as `armor() == "none"` read it.
func (x *deriver) resolveArmor() {
	c := x.c
	x.armorCategory = "none"
	if x.b.Armor == "" {
		return
	}
	eq, ok := c.equipment[x.b.Armor]
	switch {
	case !ok || eq.Armor == nil:
		x.issue(IssueUnknownKey, "full.armor_key", "A armadura escolhida não existe no conteúdo %s.", c.version)
	case eq.Armor.Category == "shield":
		x.issue(IssueUnknownKey, "full.armor_key", "O escudo é marcado à parte, não como armadura.")
	default:
		x.armor = eq.Armor
		x.armorCategory = eq.Armor.Category
	}
}

// armorProficient says whether the character is proficient in a category
// ("light", "shield"...) or in that very armor.
func (x *deriver) armorProficient(category, armorKey string) bool {
	want := []string{"equipment:" + strings.TrimPrefix(armorKey, "equipment:")}
	switch category {
	case "light", "medium", "heavy":
		want = append(want, "equipment-category:"+category+"-armor", "equipment-category:armor")
	case "shield":
		want = append(want, "equipment-category:shields")
	}
	for key := range x.proficient {
		for _, ref := range x.c.proficiencies[key].Refs {
			if slices.Contains(want, ref) {
				return true
			}
		}
	}
	return false
}

// weaponProficient says whether the character is proficient with a weapon,
// by its category ("simple", "martial") or by name.
func (x *deriver) weaponProficient(key, category string) bool {
	want := []string{key, "equipment-category:" + category + "-weapons"}
	for p := range x.proficient {
		for _, ref := range x.c.proficiencies[p].Refs {
			if slices.Contains(want, ref) {
				return true
			}
		}
	}
	return false
}
