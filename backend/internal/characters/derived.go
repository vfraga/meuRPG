package characters

import (
	"math"
	"strings"

	rulesv1 "github.com/PuraFome/meuRPG/backend/gen/meurpg/rules/v1"
	"github.com/PuraFome/meuRPG/backend/internal/rules"
)

// This file turns package rules' Go types into the rules.v1 messages: the
// derived sheet (GetCharacter) and the catalog (ListContent). It only copies
// and renames; every number comes from package rules.

var abilityToProto = map[rules.Ability]rulesv1.Ability{
	rules.STR: rulesv1.Ability_ABILITY_STRENGTH,
	rules.DEX: rulesv1.Ability_ABILITY_DEXTERITY,
	rules.CON: rulesv1.Ability_ABILITY_CONSTITUTION,
	rules.INT: rulesv1.Ability_ABILITY_INTELLIGENCE,
	rules.WIS: rulesv1.Ability_ABILITY_WISDOM,
	rules.CHA: rulesv1.Ability_ABILITY_CHARISMA,
}

var proficiencyToProto = map[rules.ProficiencyLevel]rulesv1.ProficiencyLevel{
	rules.ProficiencyNone:      rulesv1.ProficiencyLevel_PROFICIENCY_LEVEL_NONE,
	rules.ProficiencyHalf:      rulesv1.ProficiencyLevel_PROFICIENCY_LEVEL_HALF,
	rules.ProficiencyFull:      rulesv1.ProficiencyLevel_PROFICIENCY_LEVEL_PROFICIENT,
	rules.ProficiencyExpertise: rulesv1.ProficiencyLevel_PROFICIENCY_LEVEL_EXPERTISE,
}

var attackKindToProto = map[string]rulesv1.AttackKind{
	"weapon": rulesv1.AttackKind_ATTACK_KIND_WEAPON,
	"spell":  rulesv1.AttackKind_ATTACK_KIND_SPELL,
}

// derivedToProto turns rules.Derived into the DerivedSheet the app shows.
func derivedToProto(d rules.Derived) *rulesv1.DerivedSheet {
	out := &rulesv1.DerivedSheet{
		ContentVersion:        d.ContentVersion,
		RaceNamePt:            d.RaceNamePT,
		SubraceNamePt:         d.SubraceNamePT,
		BackgroundNamePt:      d.BackgroundNamePT,
		BackgroundEquipmentPt: d.BackgroundEquipmentPT,
		TotalLevel:            i32(d.TotalLevel),
		ProficiencyBonus:      i32(d.ProficiencyBonus),
		PassivePerception:     i32(d.PassivePerception),
		PassiveInvestigation:  i32(d.PassiveInvestigation),
		PassiveInsight:        i32(d.PassiveInsight),
		Initiative:            i32(d.Initiative),
		NextLevelXp:           i32(d.NextLevelXP),
		ArmorClass:            i32(d.ArmorClass),
		ArmorClassDescription: d.ArmorClassDescription,
		HitPointsMax:          i32(max(d.HitPointsMax, 1)),
		SpeedWalkFt:           i32(d.SpeedWalkFt),
		SpeedFlyFt:            i32(d.SpeedFlyFt),
		SpeedSwimFt:           i32(d.SpeedSwimFt),
		SpeedClimbFt:          i32(d.SpeedClimbFt),
		SpeedBurrowFt:         i32(d.SpeedBurrowFt),
		Hover:                 d.Hover,
		Proficiencies:         &rulesv1.Proficiencies{},
	}
	for _, c := range d.Classes {
		out.Classes = append(out.Classes, &rulesv1.DerivedClass{
			ClassKey:       c.ClassKey,
			NamePt:         c.NamePT,
			Level:          i32(c.Level),
			SubclassNamePt: c.SubclassNamePT,
		})
	}
	for _, a := range d.Abilities {
		out.Abilities = append(out.Abilities, &rulesv1.DerivedAbility{
			Ability:  abilityToProto[a.Ability],
			NamePt:   a.NamePT,
			Base:     i32(a.Base),
			Bonus:    i32(a.Bonus),
			Score:    i32(a.Score),
			Modifier: i32(a.Modifier),
		})
	}
	for _, st := range d.SavingThrows {
		out.SavingThrows = append(out.SavingThrows, &rulesv1.SavingThrow{
			Ability:    abilityToProto[st.Ability],
			NamePt:     st.NamePT,
			Proficient: st.Proficient,
			Bonus:      i32(st.Bonus),
		})
	}
	for _, sk := range d.Skills {
		out.Skills = append(out.Skills, &rulesv1.DerivedSkill{
			Key:         sk.Key,
			NamePt:      sk.NamePT,
			Ability:     abilityToProto[sk.Ability],
			Proficiency: proficiencyToProto[sk.Proficiency],
			Bonus:       i32(sk.Bonus),
		})
	}
	for _, hd := range d.HitDice {
		out.HitDice = append(out.HitDice, &rulesv1.HitDice{Faces: i32(hd.Die), Count: i32(hd.Count)})
	}
	for _, se := range d.Senses {
		// The proto's key is what grants the sense, as the Hint's source_key.
		out.Senses = append(out.Senses, &rulesv1.Sense{Key: se.Source, NamePt: se.NamePT, RangeFt: i32(se.RangeFt), Sense: se.Key})
	}
	for _, sc := range d.Spellcasting {
		p := &rulesv1.Spellcasting{
			ClassKey:      sc.Class,
			ClassNamePt:   sc.ClassNamePT,
			Ability:       abilityToProto[sc.Ability],
			SaveDc:        i32(sc.SaveDC),
			AttackBonus:   i32(sc.AttackBonus),
			CantripsKnown: i32(sc.CantripsKnown),
		}
		// Only one of the two limits applies, and the other stays 0.
		if sc.PreparesSpells {
			p.PreparedMax = i32(sc.PreparedMax)
		} else {
			p.SpellsKnown = i32(sc.SpellsKnownMax)
		}
		out.Spellcasting = append(out.Spellcasting, p)
	}
	for i, n := range d.SpellSlots {
		if n > 0 {
			out.SpellSlots = append(out.SpellSlots, &rulesv1.SpellSlots{Level: i32(i + 1), Count: i32(n)})
		}
	}
	if d.PactMagic != nil {
		out.PactMagic = &rulesv1.PactMagic{SlotLevel: i32(d.PactMagic.SlotLevel), Count: i32(d.PactMagic.Slots)}
	}
	for _, sp := range d.Spells {
		out.Spells = append(out.Spells, &rulesv1.CharacterSpell{Spell: spellToProto(sp.Spell), Prepared: sp.Prepared})
	}
	for _, a := range d.Attacks {
		out.Attacks = append(out.Attacks, attackToProto(a))
	}
	for _, r := range d.Resources {
		out.Resources = append(out.Resources, &rulesv1.Resource{
			Key: r.Key, NamePt: r.NamePT, Max: i32(r.Max), Recharge: rechargeToProto[r.Recharge], SourceKey: r.Source,
		})
	}
	for _, a := range d.SaveActions {
		out.SaveActions = append(out.SaveActions, &rulesv1.SaveAction{
			Key: a.Key, Name: a.Name, Ability: abilityToProto[a.Ability], Dc: i32(a.DC),
			OnSuccess: saveSuccessToProto[a.OnSuccess], Usage: a.Usage, Text: a.Text,
		})
	}
	for _, a := range d.Actions {
		out.Actions = append(out.Actions, actionToProto(a))
	}
	for _, a := range d.StandardActions {
		out.StandardActions = append(out.StandardActions, actionToProto(a))
	}
	for _, f := range d.Features {
		out.Features = append(out.Features, &rulesv1.Feature{
			Key:         f.Key,
			Name:        f.Name,
			NamePt:      f.NamePT,
			SourcePt:    f.SourcePT,
			Description: strings.Join(f.Description, "\n\n"),
		})
	}
	for _, l := range d.Languages {
		out.Languages = append(out.Languages, l.NamePT)
	}
	for _, p := range d.Proficiencies {
		switch p.Kind {
		case "armor":
			out.Proficiencies.Armor = append(out.Proficiencies.Armor, p.NamePT)
		case "weapon":
			out.Proficiencies.Weapons = append(out.Proficiencies.Weapons, p.NamePT)
		case "tool", "other":
			// The SRD files the thieves' tools, the herbalism kit, the navigator's tools, the
			// poisoner's kit and the vehicles under "other": they are tools too (saving
			// throws and skills never get here, and are shown elsewhere on the sheet).
			out.Proficiencies.Tools = append(out.Proficiencies.Tools, p.NamePT)
		}
	}
	for _, h := range d.Hints {
		out.Hints = append(out.Hints, &rulesv1.Hint{SourceKey: h.Source, Text: h.TextPT})
	}
	for _, is := range d.Issues {
		out.Issues = append(out.Issues, &rulesv1.Issue{Code: is.Code, Field: is.Field, Message: is.Message})
	}
	return out
}

var rechargeToProto = map[string]rulesv1.Recharge{
	rules.RechargeShortRest: rulesv1.Recharge_RECHARGE_SHORT_REST,
	rules.RechargeLongRest:  rulesv1.Recharge_RECHARGE_LONG_REST,
	rules.RechargeDawn:      rulesv1.Recharge_RECHARGE_DAWN,
	rules.RechargeNone:      rulesv1.Recharge_RECHARGE_NONE,
}

var economyToProto = map[string]rulesv1.ActionEconomy{
	rules.EconomyAction:      rulesv1.ActionEconomy_ACTION_ECONOMY_ACTION,
	rules.EconomyBonusAction: rulesv1.ActionEconomy_ACTION_ECONOMY_BONUS_ACTION,
	rules.EconomyReaction:    rulesv1.ActionEconomy_ACTION_ECONOMY_REACTION,
	rules.EconomyFree:        rulesv1.ActionEconomy_ACTION_ECONOMY_FREE,
	rules.EconomyMovement:    rulesv1.ActionEconomy_ACTION_ECONOMY_MOVEMENT,
}

func attackToProto(a rules.Attack) *rulesv1.Attack {
	return &rulesv1.Attack{
		DamageDice:          diceToProto(a.DamageDice),
		VersatileDamageDice: diceToProto(a.VersatileDice),
		DamageTypeKey:       a.DamageType,
		Key:                 a.Key,
		Name:                a.Name,
		NamePt:              a.NamePT,
		AttackBonus:         i32(a.AttackBonus),
		Damage:              a.Damage,
		DamageTypePt:        a.DamageTypeNamePT,
		Kind:                attackKindToProto[a.Kind],
		SaveDc:              i32(a.SaveDC),
		SaveAbility:         abilityToProto[a.SaveAbility],
		VersatileDamage:     a.VersatileDamage,
		RangeFt:             i32(a.RangeFt),
		LongRangeFt:         i32(a.LongRangeFt),
		Melee:               a.Melee,
		Notes:               a.Notes,
	}
}

func actionToProto(a rules.Action) *rulesv1.Action {
	return &rulesv1.Action{Key: a.Key, NamePt: a.NamePT, Economy: economyToProto[a.Economy], ResourceKey: a.Resource, SourceKey: a.Source}
}

// diceToProto is nil for a weapon without damage dice.
func diceToProto(f rules.DiceFormula) *rulesv1.DiceFormula {
	if f == (rules.DiceFormula{}) {
		return nil
	}
	return &rulesv1.DiceFormula{Count: i32(f.Count), Sides: i32(f.Sides), Bonus: i32(f.Bonus), AddsModifier: f.AddsModifier}
}

func spellToProto(s rules.SpellEntry) *rulesv1.Spell {
	return &rulesv1.Spell{
		Key:           s.Key,
		Name:          s.Name,
		NamePt:        s.NamePT,
		Level:         i32(s.Level),
		SchoolKey:     s.School,
		SchoolNamePt:  s.SchoolNamePT,
		ClassKeys:     s.Classes,
		Ritual:        s.Ritual,
		Concentration: s.Concentration,
		Archived:      s.Archived,
		Off:           s.Off,
	}
}

// int32s converts a list of small numbers for the API (see i32).
func int32s(in []int) []int32 {
	var out []int32
	for _, n := range in {
		out = append(out, i32(n))
	}
	return out
}

func abilityScores(m map[rules.Ability]int) *rulesv1.AbilityScores {
	return &rulesv1.AbilityScores{
		Strength:     i32(m[rules.STR]),
		Dexterity:    i32(m[rules.DEX]),
		Constitution: i32(m[rules.CON]),
		Intelligence: i32(m[rules.INT]),
		Wisdom:       i32(m[rules.WIS]),
		Charisma:     i32(m[rules.CHA]),
	}
}

var (
	preparationToProto = map[string]rulesv1.SpellPreparation{
		rules.PreparationKnown:     rulesv1.SpellPreparation_SPELL_PREPARATION_KNOWN,
		rules.PreparationPrepared:  rulesv1.SpellPreparation_SPELL_PREPARATION_PREPARED,
		rules.PreparationSpellbook: rulesv1.SpellPreparation_SPELL_PREPARATION_SPELLBOOK,
	}
	armorCategoryToProto = map[string]rulesv1.ArmorCategory{
		"light":  rulesv1.ArmorCategory_ARMOR_CATEGORY_LIGHT,
		"medium": rulesv1.ArmorCategory_ARMOR_CATEGORY_MEDIUM,
		"heavy":  rulesv1.ArmorCategory_ARMOR_CATEGORY_HEAVY,
	}
	weaponCategoryToProto = map[string]rulesv1.WeaponCategory{
		"simple":  rulesv1.WeaponCategory_WEAPON_CATEGORY_SIMPLE,
		"martial": rulesv1.WeaponCategory_WEAPON_CATEGORY_MARTIAL,
	}
)

// catalogToProto turns the rules catalog into ListContent's answer.
func catalogToProto(c rules.Catalog) *rulesv1.Content {
	out := &rulesv1.Content{ContentVersion: c.ContentVersion, Attribution: c.Attribution}
	for _, a := range c.Abilities {
		out.Abilities = append(out.Abilities, &rulesv1.AbilityInfo{
			Ability: abilityToProto[a.Ability], Name: a.Name, NamePt: a.NamePT, AbbreviationPt: a.AbbreviationPT,
		})
	}
	for _, r := range c.Races {
		out.Races = append(out.Races, &rulesv1.Race{
			Key: r.Key, Name: r.Name, NamePt: r.NamePT, SpeedFt: i32(r.SpeedFt), AbilityBonuses: abilityScores(r.AbilityBonuses),
			Archived: r.Archived, Off: r.Off, ChoiceBonuses: int32s(r.ChoiceBonuses),
		})
	}
	for _, s := range c.Subraces {
		out.Subraces = append(out.Subraces, &rulesv1.Subrace{
			Key: s.Key, Name: s.Name, NamePt: s.NamePT, RaceKey: s.Race, AbilityBonuses: abilityScores(s.AbilityBonuses),
			Archived: s.Archived, Off: s.Off,
		})
	}
	for _, cl := range c.Classes {
		p := &rulesv1.CharacterClass{
			Key:           cl.Key,
			Name:          cl.Name,
			NamePt:        cl.NamePT,
			HitDie:        i32(cl.HitDie),
			SkillChoice:   &rulesv1.SkillChoice{Count: i32(cl.SkillChoices), SkillKeys: cl.SkillOptions},
			SubclassLevel: i32(cl.SubclassLevel),
			Archived:      cl.Archived,
			Off:           cl.Off,
		}
		for _, a := range cl.SavingThrows {
			p.SavingThrows = append(p.SavingThrows, abilityToProto[a])
		}
		if cl.SpellcastingAbility != "" {
			p.Spellcasting = &rulesv1.ClassSpellcasting{
				Ability:      abilityToProto[cl.SpellcastingAbility],
				FirstLevel:   i32(cl.SpellcastingLevel),
				Preparation:  preparationToProto[cl.SpellPreparation],
				ListClassKey: cl.SpellListFrom,
			}
			for _, n := range cl.MaxSpellLevelByLevel {
				p.Spellcasting.MaxSpellLevelByLevel = append(p.Spellcasting.MaxSpellLevelByLevel, i32(n))
			}
		}
		out.Classes = append(out.Classes, p)
	}
	for _, s := range c.Subclasses {
		p := &rulesv1.Subclass{Key: s.Key, Name: s.Name, NamePt: s.NamePT, ClassKey: s.Class, Archived: s.Archived, Off: s.Off}
		if c := s.Casting; c != nil {
			p.Spellcasting = &rulesv1.SubclassSpellcasting{
				Ability: abilityToProto[c.Ability], Preparation: preparationToProto[c.Preparation],
				ListClassKey: c.SpellList, FirstLevel: i32(c.StartLevel), MaxSpellLevelByLevel: int32s(c.MaxSpellLevelByLevel),
			}
		}
		for _, ap := range s.AlwaysPrepared {
			p.AlwaysPrepared = append(p.AlwaysPrepared, &rulesv1.SubclassAlwaysPrepared{SpellKey: ap.Spell, ClassLevel: i32(ap.ClassLevel)})
		}
		out.Subclasses = append(out.Subclasses, p)
	}
	for _, b := range c.Backgrounds {
		out.Backgrounds = append(out.Backgrounds, &rulesv1.Background{
			Key: b.Key, Name: b.Name, NamePt: b.NamePT, SkillKeys: b.SkillProficiencies, Archived: b.Archived, Off: b.Off, EquipmentPt: b.EquipmentPT,
		})
	}
	for _, s := range c.Skills {
		out.Skills = append(out.Skills, &rulesv1.Skill{Key: s.Key, Name: s.Name, NamePt: s.NamePT, Ability: abilityToProto[s.Ability]})
	}
	for _, a := range c.Armor {
		out.Armor = append(out.Armor, &rulesv1.Armor{Key: a.Key, Name: a.Name, NamePt: a.NamePT, Category: armorCategoryToProto[a.Category]})
	}
	for _, w := range c.Weapons {
		out.Weapons = append(out.Weapons, &rulesv1.Weapon{
			Key: w.Key, Name: w.Name, NamePt: w.NamePT, Category: weaponCategoryToProto[w.Category], Ranged: w.Range == "ranged",
		})
	}
	for _, s := range c.Spells {
		out.Spells = append(out.Spells, spellToProto(s))
	}
	for _, r := range c.ChallengeRatings {
		out.ChallengeRatings = append(out.ChallengeRatings, &rulesv1.ChallengeRating{Rating: r.Rating, Xp: i32(r.XP)})
	}
	named := func(in []rules.NamedEntry) []*rulesv1.NamedKey {
		var list []*rulesv1.NamedKey
		for _, n := range in {
			list = append(list, &rulesv1.NamedKey{Key: n.Key, NamePt: n.NamePT, Kind: namedKeyKindToProto[n.Kind]})
		}
		return list
	}
	out.Languages, out.Proficiencies, out.DamageTypes = named(c.Languages), named(c.Proficiencies), named(c.DamageTypes)
	return out
}

// namedKeyKindToProto maps NamedEntry.Kind (the SRD's proficiency kinds and "language").
var namedKeyKindToProto = map[string]rulesv1.NamedKeyKind{
	"tool":         rulesv1.NamedKeyKind_NAMED_KEY_KIND_TOOL,
	"armor":        rulesv1.NamedKeyKind_NAMED_KEY_KIND_ARMOR,
	"weapon":       rulesv1.NamedKeyKind_NAMED_KEY_KIND_WEAPON,
	"skill":        rulesv1.NamedKeyKind_NAMED_KEY_KIND_SKILL,
	"saving-throw": rulesv1.NamedKeyKind_NAMED_KEY_KIND_SAVING_THROW,
	"language":     rulesv1.NamedKeyKind_NAMED_KEY_KIND_LANGUAGE,
	"other":        rulesv1.NamedKeyKind_NAMED_KEY_KIND_OTHER,
}

// i32 converts a number from package rules for the API. Those numbers are
// small (the SRD's tables, and sheets that passed rules.Validate), so the
// clamp never changes a real value; it keeps the conversion provably safe.
func i32(n int) int32 {
	if n > math.MaxInt32 {
		return math.MaxInt32
	}
	if n < math.MinInt32 {
		return math.MinInt32
	}
	return int32(n)
}
