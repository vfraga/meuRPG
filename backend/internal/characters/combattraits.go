package characters

import (
	"slices"
	"strings"

	"github.com/PuraFome/meuRPG/backend/internal/play/link"
	"github.com/PuraFome/meuRPG/backend/internal/rules"
	"github.com/PuraFome/meuRPG/backend/internal/rules/combat"
)

// The features a combat reads from a sheet while it rolls attacks and damage
// (advantage and disadvantage, the extras of a hit, Rage, the resistances). The
// rules engine does the numbers; this file only reads which features the derived
// sheet has.

// greatWeaponFighting are the keys of the fighting style, which the fighter, the
// paladin and the ranger spell apart.
var greatWeaponFighting = []string{
	"feature:fighter-fighting-style-great-weapon-fighting", "feature:fighting-style-great-weapon-fighting",
}

// packTacticsSuffix ends the key of a monster's Pack Tactics trait
// ("monster:wolf#pack-tactics").
const packTacticsSuffix = "#pack-tactics"

// sunlightSensitivitySuffix and sunlightWeaknessSuffix end the keys of a monster's Sunlight Sensitivity and
// Sunlight Weakness traits ("monster:drow#sunlight-sensitivity", "monster:shadow#sunlight-weakness").
const (
	sunlightSensitivitySuffix = "#sunlight-sensitivity"
	sunlightWeaknessSuffix    = "#sunlight-weakness"
)

// magicResistanceSuffix ends the key of a monster's Magic Resistance trait.
const magicResistanceSuffix = "#magic-resistance"

// evasionFeatures are the keys of Evasion for a character (the monk's and the rogue's, and the
// ranger's Superior Hunter's Defense choice); evasionSuffix ends a monster's trait
// ("monster:adult-blue-dragon#evasion").
var evasionFeatures = []string{"feature:monk-evasion", "feature:rogue-evasion", "feature:superior-hunters-defense-evasion"}

const evasionSuffix = "#evasion"

// creatureTypePrefix starts the key of the pseudo feature that carries a monster's
// type ("creature-type:undead"): a combat reads it with Traits.CreatureType (Divine
// Smite does extra damage to an undead or a fiend).
const creatureTypePrefix = "creature-type:"

// withCreatureType adds the SRD type of the monster to its features.
func withCreatureType(content *rules.Content, monsterKey string, features []rules.Feature) []rules.Feature {
	c, ok := content.CreatureByKey(monsterKey)
	if !ok || c.Type == "" {
		return features
	}
	return append(slices.Clone(features), rules.Feature{Key: creatureTypePrefix + strings.ToLower(c.Type), Name: c.Type, Source: monsterKey})
}

// taggedSavesOf are the advantages on saving throws the sheet's hints give against one kind of source
// ("against:frightened"): the racial ones, which a combat applies when it knows what the save is against.
func taggedSavesOf(hints []rules.Hint) []link.TaggedSave {
	var out []link.TaggedSave
	for _, h := range hints {
		if t, ok := taggedSaveOf(h); ok {
			out = append(out, t)
		}
	}
	return out
}

// taggedSaveOf is the tagged save a hint gives, when it gives one: an advantage against a kind of source the combat knows
// ("against:frightened"), or the advantage or disadvantage of a table's roll mode against a condition; on saving
// throws only.
func taggedSaveOf(h rules.Hint) (link.TaggedSave, bool) {
	against, ok := againstOf(h.Tags)
	if h.AgainstCondition != "" {
		against, ok = h.AgainstCondition, len(h.Tags) == 0
	}
	if !ok || (h.Mode != "advantage" && (h.Mode != "disadvantage" || h.AgainstCondition == "")) {
		return link.TaggedSave{}, false
	}
	if len(h.Targets) > 0 && slices.ContainsFunc(h.Targets, func(x string) bool { return !strings.HasPrefix(x, "save.") }) {
		return link.TaggedSave{}, false // not only saving throws
	}
	t := link.TaggedSave{Source: h.Source, Against: against, Disadvantage: h.Mode == "disadvantage"}
	for _, target := range h.Targets {
		if ability, ok := strings.CutPrefix(target, "save."); ok && ability != "all" {
			t.Abilities = append(t.Abilities, ability)
		}
	}
	return t, true
}

// againstOf is the kind of source the tags say the advantage holds against, when it is one the combat knows.
func againstOf(tags []string) (string, bool) {
	if len(tags) != 1 {
		return "", false
	}
	kind, ok := strings.CutPrefix(tags[0], "against:")
	return kind, ok && slices.Contains([]string{combat.AgainstMagic, combat.AgainstFrightened, combat.AgainstCharmed, combat.AgainstPoison}, kind)
}

// protections are the damage resistances, immunities and vulnerabilities the sheet's features give, and the conditions
// they make it immune to.
func protections(content *rules.Content, d rules.Derived) ([]link.Resistance, []link.ConditionImmunity) {
	var resistances []link.Resistance
	for _, r := range content.Resistances(d) {
		resistances = append(resistances, link.Resistance{Source: r.Source, NamePT: r.NamePT, DamageTypes: r.DamageTypes, While: r.While, Kind: r.Kind, Nonmagical: r.Nonmagical})
	}
	var immunities []link.ConditionImmunity
	for _, c := range content.ConditionImmunities(d) {
		immunities = append(immunities, link.ConditionImmunity{Source: c.Source, Conditions: c.Conditions})
	}
	return resistances, immunities
}

// traitsOf reads the combat traits of a derived sheet or stat block.
func traitsOf(content *rules.Content, d rules.Derived) link.Traits {
	t := link.Traits{
		BarbarianLevel: rules.LevelIn(d, "class:barbarian"),
		RogueLevel:     rules.LevelIn(d, "class:rogue"),
		Rage:           rules.HasFeature(d, "feature:rage"),
		RecklessAttack: rules.HasFeature(d, "feature:reckless-attack"),
		DangerSense:    rules.HasFeature(d, "feature:danger-sense"),
		Frenzy:         rules.HasFeature(d, "feature:frenzy"),
		SculptSpells:   rules.HasFeature(d, "feature:sculpt-spells"),
		DivineSmite:    rules.HasFeature(d, "feature:divine-smite"),
		ColossusSlayer: rules.HasFeature(d, "feature:hunters-prey-colossus-slayer"),
		HeavyArmor:     d.ArmorCategory == "heavy",
		HuntersMark: slices.ContainsFunc(d.Spells, func(s rules.CharacterSpell) bool {
			return s.Spell.Key == "spell:hunters-mark"
		}),
	}
	// The armor's "Stealth: Disadvantage" is a hint on the sheet; the Hide roll reads it here.
	t.StealthDisadvantage = slices.ContainsFunc(d.Hints, func(h rules.Hint) bool {
		return h.Target == "skill:stealth" && h.Mode == "disadvantage"
	})
	t.ArmorUntrained = d.ArmorUntrained.Any()
	t.TaggedSaves = taggedSavesOf(d.Hints)
	t.ImprovedDivineSmite = rules.HasFeature(d, "feature:improved-divine-smite")
	t.OpenHand = rules.HasFeature(d, "feature:open-hand-technique")
	t.StunningStrike = rules.HasFeature(d, "feature:stunning-strike")
	if rules.LevelIn(d, "class:monk") > 0 {
		for _, a := range d.Abilities {
			if a.Ability == rules.WIS {
				t.KiSaveDC = 8 + d.ProficiencyBonus + a.Modifier // the ki save DC (SRD 5.1, Monk)
			}
		}
	}
	if rules.HasFeature(d, "feature:sneak-attack") {
		t.SneakAttackDice = combat.SneakAttackDice(t.RogueLevel)
	}
	t.Evasion = slices.ContainsFunc(d.Features, func(f rules.Feature) bool {
		return slices.Contains(evasionFeatures, f.Key) || strings.HasSuffix(f.Key, evasionSuffix)
	})
	t.GreatWeaponFighting = slices.ContainsFunc(greatWeaponFighting, func(k string) bool { return rules.HasFeature(d, k) })
	t.PackTactics = slices.ContainsFunc(d.Features, func(f rules.Feature) bool { return strings.HasSuffix(f.Key, packTacticsSuffix) })
	t.MagicResistance = slices.ContainsFunc(d.Features, func(f rules.Feature) bool { return strings.HasSuffix(f.Key, magicResistanceSuffix) })
	t.SunlightSensitivity = slices.ContainsFunc(d.Features, func(f rules.Feature) bool { return strings.HasSuffix(f.Key, sunlightSensitivitySuffix) })
	t.SunlightWeakness = slices.ContainsFunc(d.Features, func(f rules.Feature) bool { return strings.HasSuffix(f.Key, sunlightWeaknessSuffix) })
	// The halfling's Lucky is a race trait, so every halfling has it, whichever its subrace.
	t.Lucky = rules.HasFeature(d, link.LuckyTrait)
	for _, f := range d.Features {
		if typ, ok := strings.CutPrefix(f.Key, creatureTypePrefix); ok {
			t.CreatureType = typ
		}
	}
	t.Resistances, t.ConditionImmunities = protections(content, d)
	for _, r := range d.DamageReductions {
		t.DamageReductions = append(t.DamageReductions, link.DamageReduction{Source: r.Source, Amount: r.Amount, Nonmagical: r.Nonmagical})
	}
	t.SpellAttackIgnoresCover, t.RangedWeaponIgnoresCover = d.SpellAttackIgnoresCover, d.RangedWeaponIgnoresCover
	t.RangedWeaponNoLongRangeDisadvantage = d.RangedWeaponNoLongRangeDisadvantage
	t.UnactedAdvantage, t.SurprisedCritical = d.UnactedAdvantage, d.SurprisedCritical
	t.Maneuvers, t.ExtraDamages, t.SpellModifiers = tableTraitsOf(d)
	t.RerollRules = rerollRulesOf(d)
	t.Flags = slices.Clone(d.Flags)
	t.RollMinimums, t.SpellCriticalRange = rollMinimumsOf(d), spellCriticalRangeOf(d)
	return t
}

// armorTrainingOf is the armor training that counts for a combatant: the sheet's own for a player's
// character, and none for anyone else. An NPC or a creature with a stat block is proficient with what
// it wears (SRD 5.1, "Armor"), and a druid in a beast form fights as the beast. This is the one place
// that says whose armor can spoil a roll; combat.ArmorSpoilsAbility says which rolls.
func armorTrainingOf(d rules.Derived, kind string, beast *string) rules.ArmorUntrained {
	if kind != kindPlayer || (beast != nil && *beast != "") {
		return rules.ArmorUntrained{}
	}
	return d.ArmorUntrained
}

// tableTraitsOf are the maneuvers, extra damages and cast options the character's table features give.
func tableTraitsOf(d rules.Derived) (maneuvers []link.Maneuver, extras []link.ExtraDamage, options []link.SpellModifier) {
	for _, m := range d.Maneuvers {
		maneuvers = append(maneuvers, link.Maneuver{
			Key: m.Key, NamePT: m.NamePT, Resource: m.Resource, Applies: m.Applies, Economy: m.Economy,
			Ability: m.Ability, TextPT: m.TextPT, Sides: m.Sides, SaveDC: m.SaveDC, Targets: m.Targets,
		})
	}
	for _, x := range d.ExtraDamages {
		extras = append(extras, link.ExtraDamage{
			Key: x.Key, NamePT: x.NamePT, Applies: x.Applies, Resource: x.Resource, TextPT: x.TextPT, Amount: x.Amount,
			DiceCount: x.Dice.Count, DiceSides: x.Dice.Sides, NoCritDoubling: x.NoCritDoubling, DamageType: x.DamageType,
			Weapon: x.Weapon, Frequency: x.Frequency,
		})
	}
	for _, m := range d.SpellModifiers {
		options = append(options, link.SpellModifier{
			Key: m.Key, NamePT: m.NamePT, AppliesTo: m.AppliesTo, Change: m.Change, BonusType: m.BonusType, Resource: m.Resource,
			TextPT: m.TextPT, From: m.From, Bonus: m.Bonus, ExtraDice: m.ExtraDice, Cost: m.Cost,
		})
	}
	return maneuvers, extras, options
}

// rerollRulesOf are the rerolls of a die the character's table features give.
func rerollRulesOf(d rules.Derived) (rerolls []link.RerollRule) {
	for _, r := range d.Rerolls {
		rerolls = append(rerolls, link.RerollRule{Applies: r.Applies, AtMost: r.AtMost, Resource: r.Resource, Cost: r.Cost, Source: r.Source})
	}
	return rerolls
}

// rollMinimumsOf are the lowest values a die counts as that the character's table features give.
func rollMinimumsOf(d rules.Derived) (minimums []link.RollMinimum) {
	for _, m := range d.RollMinimums {
		minimums = append(minimums, link.RollMinimum{Applies: m.Applies, Value: m.Value, DamageType: m.DamageType, Source: m.Source})
	}
	return minimums
}

// spellCriticalRangeOf is the character's critical range for a spell attack roll; 0 (the default, 20) when
// nothing lowers it, like Sheet.CriticalRange of a basic sheet.
func spellCriticalRangeOf(d rules.Derived) int {
	if d.SpellCriticalRange >= rules.MaxCriticalRange {
		return 0
	}
	return d.SpellCriticalRange
}
