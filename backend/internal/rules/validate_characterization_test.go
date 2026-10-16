package rules

import (
	"fmt"
	"strings"
	"testing"
)

// validateCases corrupts Pensantus one invariant at a time (and a few pairs, to
// freeze which refusal comes first). Each Build is made fresh by the mutation.
func validateCases() map[string]func(b *Build) {
	long := func(n int) string { return strings.Repeat("x", n) }
	keys := func(prefix string, n int) []string {
		out := make([]string, n)
		for i := range out {
			out[i] = fmt.Sprintf("%s%d", prefix, i)
		}
		return out
	}
	cases := map[string]func(b *Build){
		"ok: Pensantus":                     func(_ *Build) {},
		"ok: choice text with a separator":  func(b *Build) { b.FeatureChoiceText = map[string]string{"a" + textSeparator + "b": "text"} },
		"ok: armor and shield":              func(b *Build) { b.Armor = "equipment:chain-mail"; b.Shield = true },
		"ok: rolled hit points":             func(b *Build) { b.HitPoints = HitPoints{Method: HitPointsRolled, Rolls: []int{1, MaxRoll}} },
		"ok: known background":              func(b *Build) { knownBackground(b) },
		"ok: extra bonus at the limit":      func(b *Build) { b.ExtraAbilityBonuses = map[Ability]int{STR: MaxManualBonus} },
		"ok: expertise":                     func(b *Build) { b.Expertise = []string{"skill:arcana"} },
		"bad: score missing":                func(b *Build) { delete(b.BaseScores, WIS) },
		"bad: score too low":                func(b *Build) { b.BaseScores[STR] = MinScore - 1 },
		"bad: score too high":               func(b *Build) { b.BaseScores[CHA] = MaxScore + 1 },
		"bad: unknown ability score":        func(b *Build) { b.BaseScores[Ability("nope")] = 10 },
		"bad: unknown ability bonus":        func(b *Build) { b.ExtraAbilityBonuses = map[Ability]int{Ability("nope"): 1} },
		"bad: bonus too high":               func(b *Build) { b.ExtraAbilityBonuses = map[Ability]int{DEX: MaxManualBonus + 1} },
		"bad: bonus too low":                func(b *Build) { b.ExtraAbilityBonuses = map[Ability]int{CON: -MaxManualBonus - 1} },
		"bad: race empty":                   func(b *Build) { b.Race = "" },
		"bad: race unknown":                 func(b *Build) { b.Race = "race:nope" },
		"bad: subrace unknown":              func(b *Build) { b.Subrace = "subrace:nope" },
		"bad: subrace of another race":      func(b *Build) { b.Subrace = "subrace:hill-dwarf" },
		"bad: no classes":                   func(b *Build) { b.Classes = nil },
		"bad: too many classes":             func(b *Build) { b.Classes = make([]ClassLevel, MaxClasses+1) },
		"bad: class unknown":                func(b *Build) { b.Classes[0].Class = "class:nope" },
		"bad: class repeated":               func(b *Build) { b.Classes = append(b.Classes, ClassLevel{Class: "class:wizard", Level: 1}) },
		"bad: level 0":                      func(b *Build) { b.Classes[0].Level = 0 },
		"bad: level above max":              func(b *Build) { b.Classes[0].Level = MaxLevel + 1 },
		"bad: subclass and custom subclass": func(b *Build) { b.Classes[0].CustomSubclassName = "Meu" },
		"bad: subclass unknown":             func(b *Build) { b.Classes[0].Subclass = "subclass:nope" },
		"bad: subclass of another class":    func(b *Build) { b.Classes[0].Subclass = "subclass:champion" },
		"bad: custom subclass too long": func(b *Build) {
			b.Classes[0].Subclass = ""
			b.Classes[0].CustomSubclassName = long(MaxCustomNameLength + 1)
		},
		"bad: custom subclass with spaces":   func(b *Build) { b.Classes[0].Subclass = ""; b.Classes[0].CustomSubclassName = " Meu " },
		"bad: second class level 2 unknown":  func(b *Build) { b.Classes = append(b.Classes, ClassLevel{Class: "class:nope", Level: 1}) },
		"bad: second class level out":        func(b *Build) { b.Classes = append(b.Classes, ClassLevel{Class: "class:fighter", Level: 0}) },
		"bad: levels add up over max":        func(b *Build) { b.Classes = append(b.Classes, ClassLevel{Class: "class:fighter", Level: MaxLevel}) },
		"bad: background unknown":            func(b *Build) { b.Background = "background:nope" },
		"bad: background with custom name":   func(b *Build) { knownBackground(b); b.CustomBackgroundName = "Meu" },
		"bad: background with custom skills": func(b *Build) { knownBackground(b); b.CustomBackgroundSkills = []string{"skill:arcana"} },
		"bad: background with custom profs": func(b *Build) {
			knownBackground(b)
			b.CustomBackgroundProficiencies = []string{"language:elvish"}
		},
		"bad: background with feature name":   func(b *Build) { knownBackground(b); b.CustomBackgroundFeatureName = "Meu" },
		"bad: background with feature text":   func(b *Build) { knownBackground(b); b.CustomBackgroundFeature = "texto" },
		"bad: background with equipment":      func(b *Build) { knownBackground(b); b.CustomBackgroundEquipment = "tralha" },
		"bad: custom background name long":    func(b *Build) { b.CustomBackgroundName = long(MaxCustomNameLength + 1) },
		"bad: custom background name spaces":  func(b *Build) { b.CustomBackgroundName = "Sábio " },
		"bad: custom skills too many":         func(b *Build) { b.CustomBackgroundSkills = keys("skill:", CustomBackgroundSkillCount+1) },
		"bad: custom skill repeated":          func(b *Build) { b.CustomBackgroundSkills = []string{"skill:arcana", "skill:arcana"} },
		"bad: custom skill unknown":           func(b *Build) { b.CustomBackgroundSkills = []string{"skill:arcana", "skill:nope"} },
		"bad: custom proficiency unknown":     func(b *Build) { b.CustomBackgroundProficiencies = []string{"language:elvish", "language:nope"} },
		"bad: custom proficiency of a weapon": func(b *Build) { b.CustomBackgroundProficiencies = []string{"equipment:quarterstaff"} },
		"bad: custom proficiency repeated":    func(b *Build) { b.CustomBackgroundProficiencies = []string{"language:elvish", "language:elvish"} },
		"bad: custom proficiency too many": func(b *Build) {
			b.CustomBackgroundProficiencies = keys("language:", CustomBackgroundProficiencyCount+1)
		},
		"bad: custom feature name long":          func(b *Build) { b.CustomBackgroundFeatureName = long(MaxCustomNameLength + 1) },
		"bad: custom feature name spaces":        func(b *Build) { b.CustomBackgroundFeatureName = " x" },
		"bad: custom feature text long":          func(b *Build) { b.CustomBackgroundFeature = long(MaxCustomFeatureTextLength + 1) },
		"bad: custom equipment long":             func(b *Build) { b.CustomBackgroundEquipment = long(MaxCustomEquipmentLength + 1) },
		"ok: custom feature text at the limit":   func(b *Build) { b.CustomBackgroundFeature = long(MaxCustomFeatureTextLength) },
		"bad: skills too many":                   func(b *Build) { b.SkillProficiencies = keys("skill:", MaxSkillKeys+1) },
		"bad: skill unknown":                     func(b *Build) { b.SkillProficiencies = []string{"skill:nope"} },
		"bad: skill repeated":                    func(b *Build) { b.SkillProficiencies = []string{"skill:arcana", "skill:arcana"} },
		"bad: expertise unknown":                 func(b *Build) { b.Expertise = []string{"skill:arcana", "skill:nope"} },
		"bad: expertise repeated":                func(b *Build) { b.Expertise = []string{"skill:arcana", "skill:arcana"} },
		"bad: hit point method empty":            func(b *Build) { b.HitPoints.Method = HitPointMethod(-1) },
		"bad: hit point method unknown":          func(b *Build) { b.HitPoints.Method = HitPointMethod(7) },
		"bad: too many rolls":                    func(b *Build) { b.HitPoints.Rolls = make([]int, MaxHitPointRolls+1) },
		"bad: roll 0":                            func(b *Build) { b.HitPoints = HitPoints{Method: HitPointsRolled, Rolls: []int{3, 0}} },
		"bad: roll above max":                    func(b *Build) { b.HitPoints = HitPoints{Method: HitPointsRolled, Rolls: []int{MaxRoll + 1}} },
		"bad: armor unknown":                     func(b *Build) { b.Armor = "equipment:nope" },
		"bad: armor is a shield":                 func(b *Build) { b.Armor = "equipment:shield" },
		"bad: armor is a weapon":                 func(b *Build) { b.Armor = "equipment:quarterstaff" },
		"bad: weapon unknown":                    func(b *Build) { b.Weapons = []string{"equipment:nope"} },
		"bad: weapon is armor":                   func(b *Build) { b.Weapons = []string{"equipment:chain-mail"} },
		"bad: weapons too many":                  func(b *Build) { b.Weapons = keys("equipment:", MaxWeapons+1) },
		"bad: weapon repeated":                   func(b *Build) { b.Weapons = []string{"equipment:dagger", "equipment:dagger"} },
		"bad: cantrip is leveled":                func(b *Build) { b.Cantrips = []string{"spell:shield"} },
		"bad: cantrips too many":                 func(b *Build) { b.Cantrips = keys("spell:", MaxCantrips+1) },
		"bad: known spell is a cantrip":          func(b *Build) { b.SpellsKnown = []string{"spell:fire-bolt"} },
		"bad: known spells too many":             func(b *Build) { b.SpellsKnown = keys("spell:", MaxKnownSpells+1) },
		"bad: prepared spell unknown":            func(b *Build) { b.SpellsPrepared = []string{"spell:nope"} },
		"bad: prepared spells too many":          func(b *Build) { b.SpellsPrepared = keys("spell:", MaxPreparedSpells+1) },
		"bad: choice texts too many":             func(b *Build) { b.FeatureChoiceText = manyTexts(MaxChoiceTexts + 1) },
		"bad: choice text key without separator": func(b *Build) { b.FeatureChoiceText = map[string]string{"nokey": "x"} },
		"bad: choice text too long": func(b *Build) {
			b.FeatureChoiceText = map[string]string{"a" + textSeparator + "b": long(MaxChoiceTextLength + 1)}
		},
		"bad: choice key unknown":   func(b *Build) { b.FeatureChoices = []string{"feature:nope"} },
		"bad: choice keys too many": func(b *Build) { b.FeatureChoices = keys("feature:", MaxListLength+1) },
		"bad: choice key repeated": func(b *Build) {
			b.FeatureChoices = []string{"feature:fighter-fighting-style-defense", "feature:fighter-fighting-style-defense"}
		},
		"bad: feat unknown":   func(b *Build) { b.Feats = []string{"feat:nope"} },
		"bad: feats too many": func(b *Build) { b.Feats = keys("feat:", MaxListLength+1) },
		"bad: feat repeated":  func(b *Build) { b.Feats = []string{"feat:grappler", "feat:grappler"} },
		// Pairs: the earlier check in validate's order must win.
		"order: score before race":       func(b *Build) { b.BaseScores[STR] = 0; b.Race = "" },
		"order: race before classes":     func(b *Build) { b.Race = "race:nope"; b.Classes = nil },
		"order: classes before bg":       func(b *Build) { b.Classes = nil; b.Background = "background:nope" },
		"order: level before subclass":   func(b *Build) { b.Classes[0].Level = 0; b.Classes[0].Subclass = "subclass:nope" },
		"order: subclass before name":    func(b *Build) { b.Classes[0].Subclass = "subclass:nope"; b.Classes[0].CustomSubclassName = " " },
		"order: skills before expertise": func(b *Build) { b.SkillProficiencies = []string{"skill:nope"}; b.Expertise = []string{"skill:nope"} },
		"order: rolls before armor":      func(b *Build) { b.HitPoints.Rolls = make([]int, MaxHitPointRolls+1); b.Armor = "equipment:nope" },
		"order: weapons before cantrips": func(b *Build) { b.Weapons = []string{"equipment:nope"}; b.Cantrips = []string{"spell:nope"} },
		"order: known before prepared":   func(b *Build) { b.SpellsKnown = []string{"spell:nope"}; b.SpellsPrepared = []string{"spell:nope"} },
		"order: texts before choices": func(b *Build) {
			b.FeatureChoiceText = map[string]string{"x": "y"}
			b.FeatureChoices = []string{"feature:nope"}
		},
		"order: choices before feats": func(b *Build) { b.FeatureChoices = []string{"feature:nope"}; b.Feats = []string{"feat:nope"} },
	}
	return cases
}

// knownBackground swaps the custom background for a table one.
func knownBackground(b *Build) {
	b.Background = "background:acolyte"
	b.dropCustomBackground()
}

func manyTexts(n int) map[string]string {
	out := make(map[string]string, n)
	for i := range n {
		out[fmt.Sprintf("k%d%s%d", i, textSeparator, i)] = "x"
	}
	return out
}

// TestValidateCharacterization freezes every refusal of validate, its field
// and its text, and the order in which they fire.
func TestValidateCharacterization(t *testing.T) {
	t.Parallel()
	c := loadForTest(t)
	got := map[string]string{}
	for name, mutate := range validateCases() {
		b := pensantus()
		b.HitPoints = HitPoints{Method: HitPointsFixed}
		mutate(&b)
		err := Validate(b, c)
		got[name] = errText(err)
		if strings.HasPrefix(name, "ok:") != (err == nil) {
			t.Errorf("%s: %v", name, err)
		}
	}
	goldenText(t, "validate-refusals", got)
}
