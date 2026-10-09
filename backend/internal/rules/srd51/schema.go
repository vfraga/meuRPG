package srd51

import "encoding/json"

// This file is the format of the files in data/: the SRD 5.1 content that
// cmd/srdimport writes and package rules reads. It is our own normalized
// shape, not the 5e-database one: only the fields the engine uses, and
// every reference as a stable key with a kind prefix, such as
// "class:wizard", "feature:arcane-recovery" or "spell:fire-bolt". Text
// ("desc") is the SRD's English text, as it came.
//
// Every file is a JSON array sorted by key (levels.json by class, subclass
// and level), so a new snapshot shows as a small, readable diff.

// Manifest describes a snapshot: where it came from and the sha256 of every
// input and output file.
type Manifest struct {
	// SourceRepo and SourceCommit pin the 5e-srd-api commit the snapshot
	// was made from, and SourcePath is the folder of the JSON files in it.
	SourceRepo   string `json:"source_repo"`
	SourceCommit string `json:"source_commit"`
	SourcePath   string `json:"source_path"`
	// SnapshotVersion is "srd51@" plus the first 12 characters of
	// SourceCommit. The content version adds the effects revision to it.
	SnapshotVersion string     `json:"snapshot_version"`
	Inputs          []FileHash `json:"inputs"`
	Outputs         []FileHash `json:"outputs"`
}

// FileHash is a file name and the hex sha256 of its bytes.
type FileHash struct {
	Name   string `json:"name"`
	SHA256 string `json:"sha256"`
}

// AbilityScore is one of the six abilities. Its key is the bare index
// ("str"), like rules.Ability.
type AbilityScore struct {
	Key      string   `json:"key"`
	Name     string   `json:"name"`
	FullName string   `json:"full_name"`
	Desc     []string `json:"desc"`
}

// Skill is a skill and the ability it uses.
type Skill struct {
	Key     string   `json:"key"`
	Name    string   `json:"name"`
	Ability string   `json:"ability"`
	Desc    []string `json:"desc"`
}

// Choice is "choose N from these keys".
type Choice struct {
	Choose int      `json:"choose"`
	From   []string `json:"from"`
}

// Race is a race.
type Race struct {
	Key            string         `json:"key"`
	Name           string         `json:"name"`
	SpeedFt        int            `json:"speed_ft"`
	Size           string         `json:"size"`
	AbilityBonuses map[string]int `json:"ability_bonuses"`
	// AbilityBonusChoices is the half-elf's "+1 to two other abilities".
	AbilityBonusChoices *Choice `json:"ability_bonus_choices,omitempty"`
	// Languages are always known; LanguageChoices more are chosen.
	Languages       []string `json:"languages"`
	LanguageChoices int      `json:"language_choices,omitempty"`
	Traits          []string `json:"traits"`
	Subraces        []string `json:"subraces"`
}

// Subrace is a subrace of one race.
type Subrace struct {
	Key            string         `json:"key"`
	Name           string         `json:"name"`
	Race           string         `json:"race"`
	Desc           string         `json:"desc"`
	AbilityBonuses map[string]int `json:"ability_bonuses"`
	Traits         []string       `json:"traits"`
}

// Trait is a racial trait.
type Trait struct {
	Key      string   `json:"key"`
	Name     string   `json:"name"`
	Races    []string `json:"races"`
	Subraces []string `json:"subraces"`
	Desc     []string `json:"desc"`
	// Proficiencies are proficiency keys the trait grants.
	Proficiencies []string `json:"proficiencies"`
	// ProficiencyChoices is how many proficiencies the trait lets the
	// player choose (Skill Versatility: 2), and ProficiencyOptions from
	// which.
	ProficiencyChoices int      `json:"proficiency_choices,omitempty"`
	ProficiencyOptions []string `json:"proficiency_options,omitempty"`
	LanguageChoices    int      `json:"language_choices,omitempty"`
	// Parent is set on an option of another trait, such as one color of
	// Draconic Ancestry.
	Parent string `json:"parent,omitempty"`
	// Options are the keys of this trait's options, to choose one.
	Options []string `json:"options,omitempty"`
}

// Class is a class.
type Class struct {
	Key          string   `json:"key"`
	Name         string   `json:"name"`
	HitDie       int      `json:"hit_die"`
	SavingThrows []string `json:"saving_throws"`
	// SkillChoices is the class's choice of skills at level 1.
	SkillChoices Choice `json:"skill_choices"`
	// Proficiencies are the proficiency keys of a character who starts in
	// this class, saving throws left out.
	Proficiencies []string          `json:"proficiencies"`
	Multiclass    Multiclass        `json:"multiclass"`
	Spellcasting  *ClassSpellcaster `json:"spellcasting,omitempty"`
	Subclasses    []string          `json:"subclasses"`
	// SubclassLevel is the first class level with a subclass feature.
	SubclassLevel int `json:"subclass_level"`
}

// Multiclass is what taking this class as a second class needs and gives.
type Multiclass struct {
	// Minimums must all be met; with AnyOf set, one of AnyOf is enough
	// instead (the fighter: STR 13 or DEX 13).
	Minimums map[string]int `json:"minimums,omitempty"`
	AnyOf    map[string]int `json:"any_of,omitempty"`
	// Proficiencies and SkillChoices are what a multiclass character gets.
	Proficiencies []string `json:"proficiencies"`
	SkillChoices  *Choice  `json:"skill_choices,omitempty"`
}

// ClassSpellcaster says when a class starts casting and with which
// ability.
type ClassSpellcaster struct {
	Level   int    `json:"level"`
	Ability string `json:"ability"`
}

// Level is one row of a class table, or of a subclass table when Subclass
// is set.
type Level struct {
	Class     string   `json:"class"`
	Subclass  string   `json:"subclass,omitempty"`
	Level     int      `json:"level"`
	ProfBonus int      `json:"prof_bonus,omitempty"`
	Features  []string `json:"features"`
	// Spellcasting is the row's casting columns, for casting classes.
	Spellcasting *LevelSpellcasting `json:"spellcasting,omitempty"`
	// ClassSpecific and SubclassSpecific are the class table's other
	// columns (rage count, ki points, sneak attack dice...), as they came.
	ClassSpecific    json.RawMessage `json:"class_specific,omitempty"`
	SubclassSpecific json.RawMessage `json:"subclass_specific,omitempty"`
}

// LevelSpellcasting is the casting columns of a class table row. Slots[0]
// is the number of 1st-level slots; for the warlock these are pact slots.
type LevelSpellcasting struct {
	CantripsKnown int    `json:"cantrips_known"`
	SpellsKnown   int    `json:"spells_known"`
	Slots         [9]int `json:"slots"`
}

// Subclass is a subclass of one class.
type Subclass struct {
	Key    string   `json:"key"`
	Name   string   `json:"name"`
	Class  string   `json:"class"`
	Flavor string   `json:"flavor"`
	Desc   []string `json:"desc"`
	// Spells are the subclass's always-available spells (domain, oath,
	// circle and patron spells).
	Spells []SubclassSpell `json:"spells,omitempty"`
	// ExpandedList says Spells only join the class's spell list, to choose from
	// when the class learns a spell, instead of being always prepared. It is set
	// by effects/corrections.json, never read from data/.
	ExpandedList bool `json:"-"`
}

// SubclassSpell is a spell a subclass gives at a class level, sometimes
// only with a chosen feature (the druid's land).
type SubclassSpell struct {
	Spell        string   `json:"spell"`
	ClassLevel   int      `json:"class_level"`
	WithFeatures []string `json:"with_features,omitempty"`
}

// Feature is a class or subclass feature.
type Feature struct {
	Key      string   `json:"key"`
	Name     string   `json:"name"`
	Class    string   `json:"class"`
	Subclass string   `json:"subclass,omitempty"`
	Level    int      `json:"level"`
	Desc     []string `json:"desc"`
	// Parent is set on an option of another feature, such as one fighting
	// style.
	Parent string `json:"parent,omitempty"`
	// Options are the keys of the options to choose OptionsChoose of.
	Options       []string `json:"options,omitempty"`
	OptionsChoose int      `json:"options_choose,omitempty"`
	// ExpertiseChoices is how many skills the feature doubles.
	ExpertiseChoices int `json:"expertise_choices,omitempty"`
}

// Background is a background.
type Background struct {
	Key             string            `json:"key"`
	Name            string            `json:"name"`
	Skills          []string          `json:"skills"`
	Proficiencies   []string          `json:"proficiencies,omitempty"`
	LanguageChoices int               `json:"language_choices,omitempty"`
	Feature         BackgroundFeature `json:"feature"`
}

// BackgroundFeature is a background's feature.
type BackgroundFeature struct {
	Key  string   `json:"key"`
	Name string   `json:"name"`
	Desc []string `json:"desc"`
}

// Proficiency is something a character can be proficient in.
type Proficiency struct {
	Key  string `json:"key"`
	Name string `json:"name"`
	// Kind is "armor", "weapon", "tool", "skill", "saving-throw" or "other".
	Kind string `json:"kind"`
	// Refs are what it covers: an equipment key, an equipment category
	// such as "equipment-category:light-armor", a skill key, or an ability.
	Refs []string `json:"refs"`
}

// Equipment is an armor, a weapon or a tool.
type Equipment struct {
	Key  string `json:"key"`
	Name string `json:"name"`
	// Kind is "armor", "weapon", "tool" or "gear".
	Kind   string  `json:"kind"`
	Armor  *Armor  `json:"armor,omitempty"`
	Weapon *Weapon `json:"weapon,omitempty"`
	Gear   *Gear   `json:"gear,omitempty"`
}

// Gear is the part of an Equipment that is adventuring gear: the ordinary goods
// that are neither weapon, armor nor tool (a rope, a backpack, an arrow).
type Gear struct {
	// Ammunition says it is ammunition: arrows, bolts, sling bullets and
	// blowgun needles.
	Ammunition bool `json:"ammunition,omitempty"`
	// PackQuantity is how many pieces one purchase holds (20 arrows), or 0 for
	// a single piece.
	PackQuantity int `json:"pack_quantity,omitempty"`
}

// Armor is the armor part of an Equipment.
type Armor struct {
	// Category is "light", "medium", "heavy" or "shield".
	Category            string `json:"category"`
	BaseAC              int    `json:"base_ac"`
	DexBonus            bool   `json:"dex_bonus"`
	MaxDexBonus         int    `json:"max_dex_bonus,omitempty"`
	StrMinimum          int    `json:"str_minimum,omitempty"`
	StealthDisadvantage bool   `json:"stealth_disadvantage,omitempty"`
}

// Weapon is the weapon part of an Equipment.
type Weapon struct {
	// Category is "simple" or "martial"; Range is "melee" or "ranged".
	Category        string   `json:"category"`
	Range           string   `json:"range"`
	Damage          string   `json:"damage,omitempty"`
	DamageType      string   `json:"damage_type,omitempty"`
	TwoHandedDamage string   `json:"two_handed_damage,omitempty"`
	Properties      []string `json:"properties"`
	// NormalRangeFt is the reach (5 or 10) of a melee weapon, or the
	// normal range of a ranged one; LongRangeFt the long range.
	NormalRangeFt int `json:"normal_range_ft,omitempty"`
	LongRangeFt   int `json:"long_range_ft,omitempty"`
	// ThrowNormalFt and ThrowLongFt are set for thrown weapons.
	ThrowNormalFt int `json:"throw_normal_ft,omitempty"`
	ThrowLongFt   int `json:"throw_long_ft,omitempty"`
}

// Spell is a spell.
type Spell struct {
	Key           string   `json:"key"`
	Name          string   `json:"name"`
	Level         int      `json:"level"`
	School        string   `json:"school"`
	Classes       []string `json:"classes"`
	Subclasses    []string `json:"subclasses,omitempty"`
	Ritual        bool     `json:"ritual"`
	Concentration bool     `json:"concentration"`
	CastingTime   string   `json:"casting_time"`
	Range         string   `json:"range"`
	Duration      string   `json:"duration"`
	Components    []string `json:"components"`
	Material      string   `json:"material,omitempty"`
	// AttackType is "melee" or "ranged" for spell attacks. SaveAbility is
	// set for spells that ask for a saving throw.
	AttackType  string `json:"attack_type,omitempty"`
	SaveAbility string `json:"save_ability,omitempty"`
	SaveSuccess string `json:"save_success,omitempty"`
	// Damage is one entry per damage type (Ice Storm has two).
	Damage []SpellDamage `json:"damage,omitempty"`
	// DamageChoice, set by effects/corrections.json and never read from data/,
	// says what the caster picks among the damage types: "scale" (all of them
	// are dealt, and the higher-slot dice go to the chosen one) or "alternative"
	// (only the chosen one is dealt). Empty: every type is dealt as listed.
	DamageChoice    string            `json:"-"`
	HealAtSlotLevel map[string]string `json:"heal_at_slot_level,omitempty"`
	// AreaType and AreaSizeFt are the 5e-database's structured area_of_effect:
	// "cone", "cube", "cylinder", "line" or "sphere", and its size in feet (the
	// cone's and line's length, the cube's side, the cylinder's and sphere's
	// radius). Empty for a spell that has none (a creature, a point, the caster).
	AreaType    string   `json:"area_type,omitempty"`
	AreaSizeFt  int      `json:"area_size_ft,omitempty"`
	Desc        []string `json:"desc"`
	HigherLevel []string `json:"higher_level,omitempty"`
}

// SpellDamage is one damage type of a spell. AtCharacterLevel (cantrips) and
// AtSlotLevel map a level, as text, to dice such as "2d10".
type SpellDamage struct {
	DamageType       string            `json:"damage_type,omitempty"`
	AtCharacterLevel map[string]string `json:"at_character_level,omitempty"`
	AtSlotLevel      map[string]string `json:"at_slot_level,omitempty"`
}

// Language is a language.
type Language struct {
	Key    string `json:"key"`
	Name   string `json:"name"`
	Type   string `json:"type"`
	Script string `json:"script,omitempty"`
}

// Named is a small SRD list entry: a damage type, a magic school, a weapon
// property or a condition.
type Named struct {
	Key  string   `json:"key"`
	Name string   `json:"name"`
	Desc []string `json:"desc"`
}

// Monster is a creature's stat block (5e-SRD-Monsters.json), for the
// creatures a character can have: a familiar, a summoned beast, a Wild Shape
// form or one the master gives (MR-037). Its key is "monster:<index>", such as
// "monster:wolf". Text is the SRD's English, as for the spells; speeds,
// senses and ranges are in feet.
type Monster struct {
	Key  string `json:"key"`
	Name string `json:"name"`
	// Size is "Tiny", "Small", "Medium", "Large", "Huge" or "Gargantuan".
	Size string `json:"size"`
	// Type is the SRD's creature type ("beast", "undead"...), Subtype its
	// parenthesised tag ("goblinoid"), and Alignment the SRD's text.
	Type      string `json:"type"`
	Subtype   string `json:"subtype,omitempty"`
	Alignment string `json:"alignment"`

	// ArmorClass is the stat block's first armor class, ArmorClassType what
	// it comes from ("natural", "armor", "dex", "spell", "condition") and
	// ArmorClassDesc the SRD's note ("armor scraps"), when there is one.
	// When the SRD lists several, ArmorClass is the one worn: the "armor"
	// entry over the "natural" or "dex" one (the azer's 17 with a shield, not its
	// natural 15). ArmorClassItems are the equipment keys of that armor
	// ("equipment:shield") and ArmorClassAlts the other ways to have an armor
	// class (the mage's 15 with Mage Armor, the ankheg's 11 while prone).
	ArmorClass      int            `json:"armor_class"`
	ArmorClassType  string         `json:"armor_class_type"`
	ArmorClassDesc  string         `json:"armor_class_desc,omitempty"`
	ArmorClassItems []string       `json:"armor_class_items,omitempty"`
	ArmorClassAlts  []MonsterACAlt `json:"armor_class_alts,omitempty"`
	// HitPoints is the average, HitDice the dice ("2d8") and HitPointsRoll the
	// dice with the bonus ("2d8+2").
	HitPoints     int    `json:"hit_points"`
	HitDice       string `json:"hit_dice"`
	HitPointsRoll string `json:"hit_points_roll"`

	// Speeds in feet. A creature that has none of a speed omits it. Hover
	// says a flying creature hovers.
	Speed MonsterSpeed `json:"speed"`

	// The six ability scores.
	Str int `json:"str"`
	Dex int `json:"dex"`
	Con int `json:"con"`
	Int int `json:"int"`
	Wis int `json:"wis"`
	Cha int `json:"cha"`
	// Saves are the stat block's saving throw bonuses, by ability index
	// ("dex"), and Skills its skill bonuses, by skill key ("skill:stealth").
	// Both are the total bonus, not the proficiency.
	Saves  map[string]int `json:"saves,omitempty"`
	Skills map[string]int `json:"skills,omitempty"`

	// Vulnerabilities, Resistances and Immunities are damage the creature
	// takes double, half or none of. ConditionImmunities are condition keys.
	Vulnerabilities     []MonsterDamageMod `json:"vulnerabilities,omitempty"`
	Resistances         []MonsterDamageMod `json:"resistances,omitempty"`
	Immunities          []MonsterDamageMod `json:"immunities,omitempty"`
	ConditionImmunities []string           `json:"condition_immunities,omitempty"`

	// Senses, in feet, and the passive Perception.
	Darkvision        int `json:"darkvision,omitempty"`
	Blindsight        int `json:"blindsight,omitempty"`
	Tremorsense       int `json:"tremorsense,omitempty"`
	Truesight         int `json:"truesight,omitempty"`
	PassivePerception int `json:"passive_perception"`
	// Languages is the SRD's text, such as "Common, Goblin".
	Languages string `json:"languages,omitempty"`

	// ChallengeRating is "0", "1/8", "1/4", "1/2", "1" ... "30".
	ChallengeRating  string `json:"challenge_rating"`
	XP               int    `json:"xp"`
	ProficiencyBonus int    `json:"proficiency_bonus"`

	SpecialAbilities []MonsterAbility `json:"special_abilities,omitempty"`
	Actions          []MonsterAction  `json:"actions,omitempty"`
	Reactions        []MonsterAbility `json:"reactions,omitempty"`
	// LegendaryActions keep only name and text.
	LegendaryActions []MonsterAbility `json:"legendary_actions,omitempty"`
}

// MonsterACAlt is another armor class a creature has: Value with a spell
// ("spell:mage-armor") or while in a condition ("condition:prone").
type MonsterACAlt struct {
	Value     int    `json:"value"`
	Spell     string `json:"spell,omitempty"`
	Condition string `json:"condition,omitempty"`
}

// MonsterSpeed is a creature's speeds in feet.
type MonsterSpeed struct {
	Walk   int  `json:"walk,omitempty"`
	Fly    int  `json:"fly,omitempty"`
	Swim   int  `json:"swim,omitempty"`
	Climb  int  `json:"climb,omitempty"`
	Burrow int  `json:"burrow,omitempty"`
	Hover  bool `json:"hover,omitempty"`
}

// MonsterDamageMod is one entry of a vulnerability, resistance or immunity
// list. Types are the damage type keys it names ("damage-type:acid"); Note is
// the rest of the SRD's text ("from nonmagical weapons"), or the whole text
// when it names no damage type ("damage from spells").
type MonsterDamageMod struct {
	Types []string `json:"types,omitempty"`
	Note  string   `json:"note,omitempty"`
}

// MonsterAbility is a trait, a reaction or a legendary action: a name and
// its text. Usage is the SRD's limit, such as "3/day" or "Recharge 5-6".
type MonsterAbility struct {
	Name  string `json:"name"`
	Desc  string `json:"desc"`
	Usage string `json:"usage,omitempty"`
}

// MonsterAction is an action of a stat block.
type MonsterAction struct {
	Name  string `json:"name"`
	Desc  string `json:"desc"`
	Usage string `json:"usage,omitempty"`
	// AttackBonus is the to-hit of an attack action, 0 with HasAttack false
	// for any other. HasAttack distinguishes "+0" from "not an attack".
	HasAttack   bool `json:"has_attack,omitempty"`
	AttackBonus int  `json:"attack_bonus,omitempty"`
	// Damage are the damage parts in the SRD's order. When the SRD offers a
	// choice (a longsword's one or two hands), the first option is kept; the
	// text has the rest.
	Damage []MonsterDamage `json:"damage,omitempty"`
	// Save is the saving throw the action asks for, when it does.
	Save *MonsterSave `json:"save,omitempty"`
	// Multiattack, for the Multiattack action, are its routines: each is a
	// list of attacks and counts, and the creature makes one of them. A
	// stat block with one fixed routine has one.
	Multiattack [][]MonsterAttackCount `json:"multiattack,omitempty"`
}

// MonsterDamage is one damage part: "2d4+2" and "damage-type:piercing".
type MonsterDamage struct {
	Dice       string `json:"dice"`
	DamageType string `json:"damage_type"`
}

// MonsterSave is a saving throw an action asks for: the ability index
// ("con"), the DC and what a success does ("none", "half" or "other").
type MonsterSave struct {
	Ability   string `json:"ability"`
	DC        int    `json:"dc"`
	OnSuccess string `json:"on_success"`
}

// MonsterAttackCount is "Bite x1" in a Multiattack: the name of an action of
// the same stat block, how many times, and what it is ("melee", "ranged",
// "ability" or "magic").
//
// Count is always the number the engine uses (at least 1). When the SRD's count
// is not a number, Text keeps its words for the master: the hydra's "Number of
// Heads" (Count 5, its five heads) and the violet fungus's "1d4" (Count 1).
type MonsterAttackCount struct {
	Name  string `json:"name"`
	Count int    `json:"count"`
	Kind  string `json:"kind"`
	Text  string `json:"count_text,omitempty"`
}
