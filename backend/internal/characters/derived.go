package characters

import (
	"math"
	"strings"

	charactersv1 "github.com/PuraFome/meuRPG/backend/gen/meurpg/characters/v1"
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

// mapToProto converts each entry of a list in order; an empty list stays nil, as the
// appends it replaces left it.
func mapToProto[In, Out any](in []In, convert func(In) Out) []Out {
	var out []Out
	for _, v := range in {
		out = append(out, convert(v))
	}
	return out
}

// derivedToProto turns rules.Derived into the DerivedSheet the app shows.
func derivedToProto(d rules.Derived) *rulesv1.DerivedSheet {
	out := derivedNumbersToProto(d)
	out.Classes = mapToProto(d.Classes, derivedClassToProto)
	out.Abilities = mapToProto(d.Abilities, derivedAbilityToProto)
	out.SavingThrows = mapToProto(d.SavingThrows, savingThrowToProto)
	out.Skills = mapToProto(d.Skills, derivedSkillToProto)
	out.HitDice = mapToProto(d.HitDice, derivedHitDiceToProto)
	out.Senses = mapToProto(d.Senses, senseToProto)
	out.Spellcasting = mapToProto(d.Spellcasting, spellcastingToProto)
	out.SpellSlots = derivedSlotsToProto(d.SpellSlots)
	if d.PactMagic != nil {
		out.PactMagic = &rulesv1.PactMagic{SlotLevel: i32(d.PactMagic.SlotLevel), Count: i32(d.PactMagic.Slots)}
	}
	out.Spells = mapToProto(d.Spells, characterSpellToProto)
	out.Attacks = mapToProto(d.Attacks, attackToProto)
	out.Resources = mapToProto(d.Resources, resourceToProto)
	out.SaveActions = mapToProto(d.SaveActions, saveActionToProto)
	out.Actions = mapToProto(d.Actions, actionToProto)
	out.StandardActions = mapToProto(d.StandardActions, actionToProto)
	out.Features = mapToProto(d.Features, featureToProto)
	out.BreathWeapon = breathWeaponToProto(d.BreathWeapon)
	out.Resistances = mapToProto(d.Resistances, resistanceToProto)
	out.Languages = mapToProto(d.Languages, func(l rules.NamedKey) string { return l.NamePT })
	out.Proficiencies = proficienciesToProto(d.Proficiencies)
	out.Hints = mapToProto(d.Hints, func(h rules.Hint) *rulesv1.Hint { return &rulesv1.Hint{SourceKey: h.Source, Text: h.TextPT} })
	out.Issues = mapToProto(d.Issues, func(is rules.Issue) *rulesv1.Issue {
		return &rulesv1.Issue{Code: is.Code, Field: is.Field, Message: is.Message}
	})
	return out
}

// derivedNumbersToProto is the part of the DerivedSheet that is not a list: the names,
// the numbers and the speeds. The hit points are at least 1.
func derivedNumbersToProto(d rules.Derived) *rulesv1.DerivedSheet {
	return &rulesv1.DerivedSheet{
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
		HitPointsFromEffects:  i32(d.HitPointsFromEffects),
		SpeedWalkFt:           i32(d.SpeedWalkFt),
		SpeedFlyFt:            i32(d.SpeedFlyFt),
		SpeedSwimFt:           i32(d.SpeedSwimFt),
		SpeedClimbFt:          i32(d.SpeedClimbFt),
		SpeedBurrowFt:         i32(d.SpeedBurrowFt),
		Hover:                 d.Hover,
	}
}

func derivedClassToProto(c rules.DerivedClass) *rulesv1.DerivedClass {
	return &rulesv1.DerivedClass{ClassKey: c.ClassKey, NamePt: c.NamePT, Level: i32(c.Level), SubclassNamePt: c.SubclassNamePT}
}

func derivedAbilityToProto(a rules.AbilityScore) *rulesv1.DerivedAbility {
	return &rulesv1.DerivedAbility{
		Ability: abilityToProto[a.Ability], NamePt: a.NamePT, Base: i32(a.Base), Bonus: i32(a.Bonus), Score: i32(a.Score), Modifier: i32(a.Modifier),
	}
}

func savingThrowToProto(st rules.SavingThrow) *rulesv1.SavingThrow {
	return &rulesv1.SavingThrow{Ability: abilityToProto[st.Ability], NamePt: st.NamePT, Proficient: st.Proficient, Bonus: i32(st.Bonus)}
}

func derivedSkillToProto(sk rules.Skill) *rulesv1.DerivedSkill {
	return &rulesv1.DerivedSkill{
		Key: sk.Key, NamePt: sk.NamePT, Ability: abilityToProto[sk.Ability], Proficiency: proficiencyToProto[sk.Proficiency], Bonus: i32(sk.Bonus),
	}
}

func derivedHitDiceToProto(hd rules.HitDice) *rulesv1.HitDice {
	return &rulesv1.HitDice{Faces: i32(hd.Die), Count: i32(hd.Count)}
}

// senseToProto names the sense by what grants it: the proto's key is the source, as the
// Hint's source_key.
func senseToProto(se rules.Sense) *rulesv1.Sense {
	return &rulesv1.Sense{Key: se.Source, NamePt: se.NamePT, RangeFt: i32(se.RangeFt), Sense: se.Key}
}

// spellcastingToProto sets only the limit that applies to the class (prepared or
// known); the other stays 0.
func spellcastingToProto(sc rules.Spellcasting) *rulesv1.Spellcasting {
	p := &rulesv1.Spellcasting{
		ClassKey:      sc.Class,
		ClassNamePt:   sc.ClassNamePT,
		Ability:       abilityToProto[sc.Ability],
		SaveDc:        i32(sc.SaveDC),
		AttackBonus:   i32(sc.AttackBonus),
		CantripsKnown: i32(sc.CantripsKnown),
		Spellbook:     sc.Spellbook,
	}
	if sc.PreparesSpells {
		p.PreparedMax = i32(sc.PreparedMax)
	} else {
		p.SpellsKnown = i32(sc.SpellsKnownMax)
	}
	return p
}

// derivedSlotsToProto lists the circles that have slots; index 0 of the engine's list is
// the 1st circle.
func derivedSlotsToProto(slots []int) []*rulesv1.SpellSlots {
	var out []*rulesv1.SpellSlots
	for i, n := range slots {
		if n > 0 {
			out = append(out, &rulesv1.SpellSlots{Level: i32(i + 1), Count: i32(n)})
		}
	}
	return out
}

func characterSpellToProto(sp rules.CharacterSpell) *rulesv1.CharacterSpell {
	return &rulesv1.CharacterSpell{Spell: spellToProto(sp.Spell), Prepared: sp.Prepared}
}

func resourceToProto(r rules.Resource) *rulesv1.Resource {
	return &rulesv1.Resource{Key: r.Key, NamePt: r.NamePT, Max: i32(r.Max), Recharge: rechargeToProto[r.Recharge], SourceKey: r.Source}
}

func saveActionToProto(a rules.SaveAction) *rulesv1.SaveAction {
	return &rulesv1.SaveAction{
		Key: a.Key, Name: a.Name, Ability: abilityToProto[a.Ability], Dc: i32(a.DC),
		OnSuccess: saveSuccessToProto[a.OnSuccess], Usage: a.Usage, Text: a.Text,
	}
}

// featureToProto joins the paragraphs of the descriptions with a blank line.
func featureToProto(f rules.Feature) *rulesv1.Feature {
	return &rulesv1.Feature{
		Key:                  f.Key,
		Name:                 f.Name,
		NamePt:               f.NamePT,
		SourcePt:             f.SourcePT,
		Description:          strings.Join(f.Description, "\n\n"),
		SummaryPt:            f.SummaryPT,
		DescriptionPt:        strings.Join(f.DescriptionPT, "\n\n"),
		DescriptionPtMissing: f.DescriptionPTMissing,
		DescriptionPtOnly:    f.DescriptionPTOnly,
	}
}

// breathWeaponToProto is nil for a sheet without a breath weapon.
func breathWeaponToProto(bw *rules.BreathWeapon) *rulesv1.BreathWeapon {
	if bw == nil {
		return nil
	}
	return &rulesv1.BreathWeapon{
		AncestryKey: bw.AncestryKey, DamageType: bw.DamageType, DamageTypeNamePt: bw.DamageTypePT, Dice: bw.Dice,
		Shape: breathShapeToProto[bw.Shape], SizeFt: i32(bw.SizeFt), WidthFt: i32(bw.WidthFt),
		SaveAbility: abilityToProto[bw.SaveAbility], Dc: i32(bw.DC), TextPt: bw.Text,
	}
}

func resistanceToProto(r rules.ChoiceResistance) *rulesv1.Resistance {
	return &rulesv1.Resistance{DamageType: r.DamageType, DamageTypeNamePt: r.DamageTypePT, SourceKey: r.SourceKey}
}

// proficienciesToProto sorts the proficiencies by kind; the message is there even when
// it is empty.
func proficienciesToProto(list []rules.Proficiency) *rulesv1.Proficiencies {
	out := &rulesv1.Proficiencies{}
	for _, p := range list {
		switch p.Kind {
		case "armor":
			out.Armor = append(out.Armor, p.NamePT)
		case "weapon":
			out.Weapons = append(out.Weapons, p.NamePT)
		case "tool", "other":
			// The SRD files the thieves' tools, the herbalism kit, the navigator's tools, the
			// poisoner's kit and the vehicles under "other": they are tools too (saving
			// throws and skills never get here, and are shown elsewhere on the sheet).
			out.Tools = append(out.Tools, p.NamePT)
		}
	}
	return out
}

var breathShapeToProto = map[string]rulesv1.BreathShape{
	rules.BreathLine: rulesv1.BreathShape_BREATH_SHAPE_LINE,
	rules.BreathCone: rulesv1.BreathShape_BREATH_SHAPE_CONE,
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
		Beams:               i32(a.Beams),
		DamageNotePt:        a.DamageNotePT,
		SpellDice:           a.SpellDice,
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
		Reaction:      s.CastingTime.Unit == rules.CastReaction,
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
	return &rulesv1.Content{
		ContentVersion:   c.ContentVersion,
		Attribution:      c.Attribution,
		Abilities:        mapToProto(c.Abilities, abilityInfoToProto),
		Races:            mapToProto(c.Races, raceToProto),
		Subraces:         mapToProto(c.Subraces, subraceToProto),
		Classes:          mapToProto(c.Classes, classToProto),
		Subclasses:       mapToProto(c.Subclasses, subclassToProto),
		Backgrounds:      mapToProto(c.Backgrounds, backgroundToProto),
		Skills:           mapToProto(c.Skills, skillToProto),
		Armor:            mapToProto(c.Armor, armorToProto),
		Weapons:          mapToProto(c.Weapons, weaponToProto),
		Spells:           mapToProto(c.Spells, spellToProto),
		ChallengeRatings: mapToProto(c.ChallengeRatings, challengeRatingToProto),
		LevelXp:          int32s(c.LevelXP),
		Languages:        mapToProto(c.Languages, namedKeyToProto),
		Proficiencies:    mapToProto(c.Proficiencies, namedKeyToProto),
		DamageTypes:      mapToProto(c.DamageTypes, namedKeyToProto),
	}
}

func abilityInfoToProto(a rules.AbilityEntry) *rulesv1.AbilityInfo {
	return &rulesv1.AbilityInfo{Ability: abilityToProto[a.Ability], Name: a.Name, NamePt: a.NamePT, AbbreviationPt: a.AbbreviationPT}
}

func raceToProto(r rules.RaceEntry) *rulesv1.Race {
	return &rulesv1.Race{
		Key: r.Key, Name: r.Name, NamePt: r.NamePT, SpeedFt: i32(r.SpeedFt), AbilityBonuses: abilityScores(r.AbilityBonuses),
		Archived: r.Archived, Off: r.Off, ChoiceBonuses: int32s(r.ChoiceBonuses), SkillKeys: r.SkillProficiencies,
	}
}

func subraceToProto(s rules.SubraceEntry) *rulesv1.Subrace {
	return &rulesv1.Subrace{
		Key: s.Key, Name: s.Name, NamePt: s.NamePT, RaceKey: s.Race, AbilityBonuses: abilityScores(s.AbilityBonuses),
		Archived: s.Archived, Off: s.Off, SkillKeys: s.SkillProficiencies,
	}
}

func classToProto(cl rules.ClassEntry) *rulesv1.CharacterClass {
	return &rulesv1.CharacterClass{
		Key:           cl.Key,
		Name:          cl.Name,
		NamePt:        cl.NamePT,
		HitDie:        i32(cl.HitDie),
		SkillChoice:   &rulesv1.SkillChoice{Count: i32(cl.SkillChoices), SkillKeys: cl.SkillOptions},
		SubclassLevel: i32(cl.SubclassLevel),
		Archived:      cl.Archived,
		Off:           cl.Off,
		SavingThrows:  mapToProto(cl.SavingThrows, func(a rules.Ability) rulesv1.Ability { return abilityToProto[a] }),
		Spellcasting:  classSpellcastingToProto(cl),
	}
}

// classSpellcastingToProto is nil for a class that casts nothing (no ability).
func classSpellcastingToProto(cl rules.ClassEntry) *rulesv1.ClassSpellcasting {
	if cl.SpellcastingAbility == "" {
		return nil
	}
	return &rulesv1.ClassSpellcasting{
		Ability:              abilityToProto[cl.SpellcastingAbility],
		FirstLevel:           i32(cl.SpellcastingLevel),
		Preparation:          preparationToProto[cl.SpellPreparation],
		ListClassKey:         cl.SpellListFrom,
		MaxSpellLevelByLevel: int32s(cl.MaxSpellLevelByLevel),
	}
}

func subclassToProto(s rules.SubclassEntry) *rulesv1.Subclass {
	p := &rulesv1.Subclass{Key: s.Key, Name: s.Name, NamePt: s.NamePT, ClassKey: s.Class, Archived: s.Archived, Off: s.Off}
	if c := s.Casting; c != nil {
		p.Spellcasting = &rulesv1.SubclassSpellcasting{
			Ability: abilityToProto[c.Ability], Preparation: preparationToProto[c.Preparation],
			ListClassKey: c.SpellList, FirstLevel: i32(c.StartLevel), MaxSpellLevelByLevel: int32s(c.MaxSpellLevelByLevel),
		}
	}
	p.AlwaysPrepared = mapToProto(s.AlwaysPrepared, func(ap rules.SubclassSpellRef) *rulesv1.SubclassAlwaysPrepared {
		return &rulesv1.SubclassAlwaysPrepared{SpellKey: ap.Spell, ClassLevel: i32(ap.ClassLevel)}
	})
	return p
}

func backgroundToProto(b rules.BackgroundEntry) *rulesv1.Background {
	return &rulesv1.Background{
		Key: b.Key, Name: b.Name, NamePt: b.NamePT, SkillKeys: b.SkillProficiencies, Archived: b.Archived, Off: b.Off, EquipmentPt: b.EquipmentPT,
	}
}

func skillToProto(s rules.SkillEntry) *rulesv1.Skill {
	return &rulesv1.Skill{Key: s.Key, Name: s.Name, NamePt: s.NamePT, Ability: abilityToProto[s.Ability]}
}

func armorToProto(a rules.ArmorEntry) *rulesv1.Armor {
	return &rulesv1.Armor{Key: a.Key, Name: a.Name, NamePt: a.NamePT, Category: armorCategoryToProto[a.Category]}
}

func weaponToProto(w rules.WeaponEntry) *rulesv1.Weapon {
	return &rulesv1.Weapon{Key: w.Key, Name: w.Name, NamePt: w.NamePT, Category: weaponCategoryToProto[w.Category], Ranged: w.Range == "ranged"}
}

func challengeRatingToProto(r rules.ChallengeRating) *rulesv1.ChallengeRating {
	return &rulesv1.ChallengeRating{Rating: r.Rating, Xp: i32(r.XP)}
}

func namedKeyToProto(n rules.NamedEntry) *rulesv1.NamedKey {
	return &rulesv1.NamedKey{Key: n.Key, NamePt: n.NamePT, Kind: namedKeyKindToProto[n.Kind]}
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

// openChoiceKinds maps the engine's kinds of open choice to the API's.
var openChoiceKinds = map[string]charactersv1.OpenChoiceKind{
	rules.OpenChoiceSkills:         charactersv1.OpenChoiceKind_OPEN_CHOICE_KIND_SKILLS,
	rules.OpenChoiceCantrips:       charactersv1.OpenChoiceKind_OPEN_CHOICE_KIND_CANTRIPS,
	rules.OpenChoiceSpellsKnown:    charactersv1.OpenChoiceKind_OPEN_CHOICE_KIND_SPELLS_KNOWN,
	rules.OpenChoiceSpellsPrepared: charactersv1.OpenChoiceKind_OPEN_CHOICE_KIND_SPELLS_PREPARED,
}

// openChoices are the choices a derived sheet still lacks, for the master's
// list.
func openChoices(d rules.Derived) []*charactersv1.OpenChoice {
	var out []*charactersv1.OpenChoice
	for _, o := range d.OpenChoices {
		out = append(out, &charactersv1.OpenChoice{Kind: openChoiceKinds[o.Kind], Missing: i32(o.Missing)})
	}
	return out
}
