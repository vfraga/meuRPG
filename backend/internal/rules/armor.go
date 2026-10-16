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
// Shield is not counted: it lasts a moment and belongs to the combat. Mage Armor
// is, when the caller says the character has it (Build.MageArmor): it lasts
// hours and belongs to the game session.
func (x *deriver) armorClass() {
	c := x.c
	name := "Sem armadura"
	base := 10 + x.mods[DEX]
	if x.armor != nil {
		name = c.namePT(x.b.Armor)
		base = x.wornArmor(name)
	}
	base, name = x.effectBases(base, name)

	// Mage Armor (SRD 5.1): a creature that wears no armor has a base AC of 13 +
	// its Dexterity modifier, the best base winning like any other.
	if x.b.MageArmor && x.armor == nil {
		if v := mageArmorBase + x.mods[DEX]; v > base {
			base = v
			name = c.namePT(MageArmorSpell)
		}
	}

	ac := base
	if x.b.Shield {
		ac += 2
		name += " + escudo"
		if !x.armorProficient("shield", "equipment:shield") {
			x.issue(IssueArmorProficiency, "full.shield", "Sem proficiência em escudos.")
		}
	}
	x.d.ArmorClass = x.modifiers("ac", ac)
	x.d.ArmorClassDescription = name
	x.d.ArmorCategory = x.armorCategory
}

// wornArmor is the base AC of the worn armor (named name on the sheet), with
// the Dexterity it lets count, and records what the armor costs: an Issue
// without proficiency, a Hint for Stealth and one for a Strength the character
// lacks.
func (x *deriver) wornArmor(name string) int {
	a := x.armor
	base := a.BaseAC
	if a.DexBonus {
		dex := x.mods[DEX]
		if a.MaxDexBonus > 0 {
			dex = min(dex, a.MaxDexBonus)
		}
		base += dex
	}
	if !x.armorProficient(x.armorCategory, x.b.Armor) {
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
	return base
}

// effectBases applies the other bases the "ac.base" effects offer to base and
// its description name: the best one wins, and the description names the
// feature that gave it. Unarmored Defense is the exception: a character that
// has it from one class never gains it again from another (SRD 5.1,
// "Multiclassing", "Unarmored Defense"), so only the first of the features
// that read "Unarmored Defense" counts.
func (x *deriver) effectBases(base int, name string) (int, string) {
	unarmored := ""
	for _, a := range x.active {
		if !isBaseEffect(a, &unarmored) || !x.applies(a) {
			continue
		}
		v, ok := x.value(a)
		if !ok {
			continue
		}
		switch e := a.effect; {
		case e.Mode == "set", e.Mode == "max" && v > base:
			base = v
			name = x.c.namePT(a.owner)
		case e.Mode == "add":
			base += v
		}
	}
	return base, name
}

// isBaseEffect says whether a offers a base AC: an untagged "ac.base"
// modifier, and, among the Unarmored Defense features, only the first one met
// (unarmored holds its key once one is met).
func isBaseEffect(a activeEffect, unarmored *string) bool {
	e := a.effect
	if e.Type != "modifier" || e.Target != "ac.base" || len(e.Tags) > 0 {
		return false
	}
	if !unarmoredDefenseFeatures[a.owner] {
		return true
	}
	if *unarmored == "" {
		*unarmored = a.owner
	}
	return a.owner == *unarmored
}

// unarmoredDefenseFeatures are the class features called Unarmored Defense: the
// barbarian's and the monk's. A character has at most one of them.
var unarmoredDefenseFeatures = map[string]bool{
	"feature:barbarian-unarmored-defense": true,
	"feature:monk-unarmored-defense":      true,
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
