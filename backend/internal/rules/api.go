package rules

import (
	"slices"
	"strings"
)

// This file is the public surface of package rules: the types other modules
// build and read, and the four entry points (LoadSRD, Validate, Derive and
// the Content methods). It changes only by adding things, so the characters
// module can depend on it.

// Ability is one of the six ability scores, by its SRD index.
type Ability string

// The six abilities.
const (
	STR Ability = "str"
	DEX Ability = "dex"
	CON Ability = "con"
	INT Ability = "int"
	WIS Ability = "wis"
	CHA Ability = "cha"
)

// AllAbilities returns the six abilities in the order of the official sheet.
func AllAbilities() []Ability {
	return []Ability{STR, DEX, CON, INT, WIS, CHA}
}

// Limits that Validate enforces on a Build. The characters module shows the
// same numbers in its error messages.
const (
	// MinScore and MaxScore bound a base ability score (the 5e maximum for
	// any creature is 30).
	MinScore = 1
	MaxScore = 30
	// MaxNormalScore is the highest score an Ability Score Improvement reaches;
	// only a feature that says so (Primal Champion) takes a score higher.
	MaxNormalScore = 20
	// MaxLevel is the highest total character level.
	MaxLevel = 20
	// MaxManualBonus bounds each of Build.ExtraAbilityBonuses, in both
	// directions (-10..+10): ability score improvements (+2 up to five
	// times) and the increases a race lets the player choose go there.
	MaxManualBonus = 10
	// MaxCustomNameLength bounds the free-text names in a Build
	// (ClassLevel.CustomSubclassName, Build.CustomBackgroundName), in
	// characters.
	MaxCustomNameLength = 40
	// CustomBackgroundSkillCount is how many skills a custom background
	// grants. It mirrors the two skills of every SRD background.
	CustomBackgroundSkillCount = 2
	// CustomBackgroundKey and CustomBackgroundFeatureKey are what a custom
	// background's feature says it comes from and is on the sheet
	// (Feature.Source and Feature.Key). They are no content: nothing resolves
	// them, and a table key always ends in "@mesa".
	CustomBackgroundKey        = "background:custom"
	CustomBackgroundFeatureKey = "background-feature:custom"
	// CustomBackgroundProficiencyCount is how many tools or languages a custom
	// background grants in total, in any mix: SRD 5.1 "Customizing a Background"
	// (two skills, two tool proficiencies or languages, a feature, equipment;
	// question 82, 05/10/2026).
	CustomBackgroundProficiencyCount = 2
	// MaxCustomFeatureTextLength and MaxCustomEquipmentLength bound the free text
	// of a custom background's feature and equipment, in characters.
	MaxCustomFeatureTextLength = 1000
	MaxCustomEquipmentLength   = 500
	// MaxListLength bounds the lists of keys that have no limit of their
	// own (Build.FeatureChoices), so a write can never make Derive do
	// unbounded work.
	MaxListLength = 100
	// Limits of each list, the same as the FullSheet proto's.
	MaxClasses        = 12
	MaxSkillKeys      = 18 // SkillProficiencies, Expertise, and at most 2 custom background skills
	MaxWeapons        = 20
	MaxCantrips       = 30
	MaxKnownSpells    = 200
	MaxPreparedSpells = 100
	// MaxHitPointRolls is one roll per level after the first, and MaxRoll
	// the largest hit die (d12). A roll above its own class's die is only
	// an Issue.
	MaxHitPointRolls = MaxLevel - 1
	MaxRoll          = 12
)

// Build is what the player chose: the part of the sheet that the rules read.
// Every choice is a stable content key, never free text: "race:gnome",
// "subrace:rock-gnome", "class:wizard", "subclass:evocation",
// "background:acolyte", "skill:arcana", "equipment:leather-armor",
// "spell:fire-bolt". Derived numbers are never part of a Build; Derive
// computes them on every read (ADR-0008).
type Build struct {
	// BaseScores are the scores before any bonus, 1..30 each. All six are
	// required.
	BaseScores map[Ability]int
	// Race is required; Subrace is optional and must belong to Race.
	Race    string
	Subrace string
	// Classes holds one entry per class, in the order they were taken; the
	// first one is the starting class (saving throws, level-1 hit points).
	// The UI offers one class in Etapa 4, but the engine handles more.
	Classes []ClassLevel
	// Background is an SRD background key. Leave it empty for a custom
	// background: CustomBackgroundName plus CustomBackgroundSkills (the
	// table's own background, such as a Sage, in its own words). With
	// neither, the sheet shows an Issue.
	Background             string
	CustomBackgroundName   string
	CustomBackgroundSkills []string
	// The rest of a custom background (SRD 5.1 "Customizing a Background"):
	// CustomBackgroundProficiencies are two tool proficiency keys
	// ("proficiency:thieves-tools") or language keys ("language:elvish") in any
	// mix, CustomBackgroundFeatureName and CustomBackgroundFeature the feature
	// the player writes (a name and a text), CustomBackgroundEquipment the
	// equipment as text. Derive applies the first three; the equipment is shown.
	CustomBackgroundProficiencies []string
	CustomBackgroundFeatureName   string
	CustomBackgroundFeature       string
	CustomBackgroundEquipment     string
	// SkillProficiencies are the skills the player chose (from the class,
	// or from anywhere the table allows). Background and race skills are
	// added by Derive and need not be repeated here.
	SkillProficiencies []string
	// Expertise doubles the proficiency bonus of these skills (Rogue,
	// Bard). Derive flags an expertise the build is not entitled to as an
	// Issue, never as an error.
	Expertise []string
	// ExtraAbilityBonuses are manual bonuses (-10..+10): the increases a
	// race lets the player choose (the half-elf's two +1), an Ability Score
	// Improvement or a magic item. The sheet shows them as "manual".
	ExtraAbilityBonuses map[Ability]int
	// HitPoints says how levels after the first add hit points.
	HitPoints HitPoints
	// Armor is the worn body armor, or empty. Shield says whether a shield
	// is carried.
	Armor  string
	Shield bool
	// Weapons are the weapons carried, for the sheet's attacks.
	Weapons []string
	// Cantrips are level-0 spells. SpellsKnown is the list of spells the
	// character knows (the wizard's spellbook, the sorcerer's known
	// spells). SpellsPrepared are the prepared ones, for classes that
	// prepare.
	Cantrips       []string
	SpellsKnown    []string
	SpellsPrepared []string
	// FeatureChoices are the options chosen for features and traits that
	// offer them: a fighting style ("feature:fighter-fighting-style-defense"),
	// a dragon ancestry ("trait:draconic-ancestry-red"), an eldritch
	// invocation. Derive applies an option only while the build has the
	// feature or trait it belongs to.
	FeatureChoices []string
	// Items are the inventory's lines, in the sheet's order. Only equipped items
	// change the numbers: a worn armor replaces Armor, a shield sets Shield and
	// an equipped weapon is one more attack. Alignment is AlignmentGood,
	// AlignmentEvil or "", for the attunement restrictions.
	Items     []Item
	Alignment string
}

// ClassLevel is one class of a Build and its level in that class.
type ClassLevel struct {
	// Class is a class key, such as "class:wizard".
	Class string
	// Subclass is an SRD subclass key of this class, or empty.
	Subclass string
	// CustomSubclassName names a subclass that is not in the SRD (the
	// table's own content). It is shown as text and has no effects. At
	// most one of Subclass and CustomSubclassName is set.
	CustomSubclassName string
	// Level is 1..20.
	Level int
}

// HitPointMethod says how levels after the first add hit points.
type HitPointMethod int

const (
	// HitPointsFixed takes the hit die's fixed average (half the die plus
	// one) for every level after the first. It is the zero value.
	HitPointsFixed HitPointMethod = iota
	// HitPointsRolled uses the player's rolls, from HitPoints.Rolls.
	HitPointsRolled
)

// HitPoints says how the maximum hit points grow. The first character level
// always takes the starting class's hit die maximum.
type HitPoints struct {
	Method HitPointMethod
	// Rolls are the hit die results for character levels 2, 3, ..., in
	// the order of Build.Classes (all levels of the first class, then the
	// next class). Validate takes 1..12; a roll above the die of its level,
	// or a missing roll, uses the fixed average and becomes an Issue.
	Rolls []int
}

// Content is the loaded rules content: the SRD 5.1 snapshot merged with the
// hand-written effects, with every formula already compiled. It is
// immutable after LoadSRD and safe for concurrent use.
type Content struct {
	c *content
}

// LoadSRD loads the embedded SRD 5.1 snapshot and effects, checks them and
// compiles every formula. Call it once at startup and fail fast on error:
// an error here is a bug in the embedded data, never a user's input.
func LoadSRD() (*Content, error) {
	return loadSRD()
}

// Version identifies the content, "srd51@<commit12>+fx.<n>", with
// "+mesa.<revision>" added by With for a campaign's own content. It changes
// whenever the SRD snapshot, the effects or the table's content change.
func (c *Content) Version() string {
	return c.c.version
}

// Archived says whether the table retired a content key: sheets that have it
// still resolve, but it is not a new choice (ADR-0018). Always false for the SRD.
func (c *Content) Archived(key string) bool {
	return c.c.archived[key]
}

// Catalog lists what the editor can offer, with Portuguese names. The
// returned slices are shared and must not be modified.
func (c *Content) Catalog() Catalog {
	return c.c.catalog
}

// NamePT returns the Portuguese name of any content key ("class:wizard" is
// "Mago"), falling back to the SRD's English name, or "" for an unknown key.
func (c *Content) NamePT(key string) string {
	return c.c.namePT(key)
}

// Conditions returns the SRD's conditions ("condition:poisoned", "Envenenado"),
// sorted by key. The combat lets the master mark them as labels (RN-22).
func (c *Content) Conditions() []NamedKey {
	var out []NamedKey
	for key := range c.c.named {
		if strings.HasPrefix(key, "condition:") {
			out = append(out, NamedKey{Key: key, NamePT: c.c.namePT(key)})
		}
	}
	slices.SortFunc(out, func(a, b NamedKey) int { return strings.Compare(a.Key, b.Key) })
	return out
}

// StandardActions returns the actions every creature has (Attack, Dash...),
// in the order of effects/standard_actions.json. A basic sheet, which is not
// derived, takes them from here, so the combat offers the same ten to
// everyone. The caller gets a copy.
func (c *Content) StandardActions() []Action {
	return slices.Clone(c.c.standardActions)
}

// Summary names a Build's race and classes for a list row, without the
// work of Derive.
func (c *Content) Summary(b Build) Summary {
	return summarize(b, c.c)
}

// Summary is what a list of characters shows.
type Summary struct {
	// RaceNamePT is the subrace's name when there is one ("Gnomo das
	// Rochas"), else the race's; empty for an unknown race.
	RaceNamePT string
	// ClassSummaryPT is each known class and its level, such as "Mago 3"
	// or "Guerreiro 2 / Mago 1".
	ClassSummaryPT string
	TotalLevel     int
}

// Validate checks the structure of a Build before it is written: scores
// 1..30, levels 1..20, keys that exist and belong together, list sizes. It
// returns a *ValidationError naming the field (never echoing the value), or
// nil. Choices that are only unusual (a spell off the class list, too many
// skills) are not errors: Derive reports them as Issues, because the app is
// an assistant, not a judge.
func Validate(b Build, c *Content) error {
	return validate(b, c.c)
}

// Derive computes the sheet's numbers from a Build. It never fails, so a
// stored sheet always opens: unknown keys, missing data or a formula that
// breaks become Issues and the rest is still computed.
func Derive(b Build, c *Content) Derived {
	return derive(b, c.c)
}

// ValidationError is what Validate returns. Field is the path of the
// sheet field, with the CharacterSheet proto's field names, such as
// "full.base_scores.intelligence" or "full.classes[0].level".
type ValidationError struct {
	Field   string
	Message string
}

func (e *ValidationError) Error() string {
	return e.Field + ": " + e.Message
}

// Catalog lists the content the editor offers. Every entry has a stable
// Key, the SRD's English Name and our Portuguese NamePT.
type Catalog struct {
	ContentVersion string
	// Attribution is the exact CC-BY-4.0 notice the SRD 5.1 requires.
	Attribution string
	// Abilities are the six abilities, in AllAbilities order. Every other
	// list is sorted by NamePT.
	Abilities   []AbilityEntry
	Races       []RaceEntry
	Subraces    []SubraceEntry
	Classes     []ClassEntry
	Subclasses  []SubclassEntry
	Backgrounds []BackgroundEntry
	Skills      []SkillEntry
	Armor       []ArmorEntry
	Weapons     []WeaponEntry
	Spells      []SpellEntry
	// ChallengeRatings are the SRD's 34 ratings with their XP, in order.
	ChallengeRatings []ChallengeRating
	// Languages, Proficiencies and DamageTypes name the SRD's keys a table entry
	// points at, sorted by Portuguese name (the master's effect menu carries the
	// same names; a player has no menu).
	Languages, Proficiencies, DamageTypes []NamedEntry
}

// NamedEntry is a key and its Portuguese name.
type NamedEntry struct {
	Key, NamePT string
	// Kind is what the key names: "tool", "armor", "weapon", "skill",
	// "saving-throw", "language" or "other" (the SRD's proficiency kinds, and a
	// language); empty for a damage type.
	Kind string
}

// AbilityEntry names an ability.
type AbilityEntry struct {
	Ability Ability
	// Name is the English name ("Intelligence"), NamePT the Portuguese one
	// ("Inteligência") and AbbreviationPT its short form ("INT").
	Name, NamePT, AbbreviationPT string
}

// Spell preparation, as in ClassEntry.SpellPreparation.
const (
	// PreparationKnown: the class knows a fixed number of spells (bard,
	// ranger, sorcerer, warlock) and uses Build.SpellsKnown.
	PreparationKnown = "known"
	// PreparationPrepared: the class prepares from its whole list (cleric,
	// druid, paladin) and uses Build.SpellsPrepared.
	PreparationPrepared = "prepared"
	// PreparationSpellbook: the class prepares from its spellbook (wizard)
	// and uses both lists: SpellsKnown is the spellbook.
	PreparationSpellbook = "spellbook"
)

// RaceEntry is a race in the Catalog.
type RaceEntry struct {
	Key, Name, NamePT string
	SpeedFt           int
	Size              string
	AbilityBonuses    map[Ability]int
	// Subraces are the keys of this race's subraces.
	Subraces []string
	// ChoiceBonuses are the ability bonuses the player places on abilities of
	// their choice (a table race's "+2 and +1 to your choice"), largest first;
	// nil for an SRD race (the half-elf's choice is in the SRD data).
	ChoiceBonuses []int
	// Archived says the table has retired it: sheets that have it keep it,
	// but it is not offered as a new choice. Every Archived below is the same.
	Archived bool
	// Off says the master switched it off for the players (RN-23): they never
	// receive it, and a sheet that has it keeps it. Every Off below is the same.
	Off bool
}

// SubraceEntry is a subrace in the Catalog.
type SubraceEntry struct {
	Key, Name, NamePT string
	// Race is the key of the parent race.
	Race           string
	AbilityBonuses map[Ability]int
	Archived       bool
	Off            bool
}

// ClassEntry is a class in the Catalog.
type ClassEntry struct {
	Key, Name, NamePT string
	// HitDie is the die size: 6 for d6.
	HitDie       int
	SavingThrows []Ability
	// SkillChoices is how many skills the class lets the player choose,
	// from SkillOptions.
	SkillChoices int
	SkillOptions []string
	// SpellcastingAbility is empty for classes that do not cast at level 1
	// or later (fighter, rogue and barbarian have no SRD caster subclass).
	SpellcastingAbility Ability
	// PreparesSpells is true for classes that prepare from a list (cleric,
	// druid, paladin, wizard) and false for classes that know spells.
	PreparesSpells bool
	// SpellcastingLevel is the class level at which spellcasting starts (1,
	// or 2 for paladin and ranger), or 0.
	SpellcastingLevel int
	// SpellPreparation is PreparationKnown, PreparationPrepared or
	// PreparationSpellbook, or empty for a class that never casts.
	SpellPreparation string
	// SubclassLevel is the class level at which the subclass is chosen.
	SubclassLevel int
	// MaxSpellLevelByLevel[n-1] is the highest spell level the class can
	// cast at class level n (index 0 is level 1, index 19 is level 20), or 0
	// when it has no leveled spells yet. Nil for a class that never casts.
	MaxSpellLevelByLevel []int
	// Subclasses are the keys of this class's subclasses, the table's
	// included.
	Subclasses []string
	Archived   bool
	Off        bool
	// SpellListFrom is the class whose spell list a table class reuses, or empty
	// (its own list: the spells that name it).
	SpellListFrom string
}

// SubclassEntry is a subclass in the Catalog.
type SubclassEntry struct {
	Key, Name, NamePT string
	// Class is the key of the parent class.
	Class    string
	Archived bool
	Off      bool
	// AlwaysPrepared are the spells it always has prepared from a class level
	// (without the ones that depend on a feature's choice).
	AlwaysPrepared []SubclassSpellRef
	// Casting is set for a subclass that casts on its own (a third caster, the
	// table's only): the web editor and the server read the numbers from it.
	Casting *SubclassCasting
}

// SubclassSpellRef is a spell a subclass always prepares from a class level.
type SubclassSpellRef struct {
	Spell      string
	ClassLevel int
}

// SubclassCasting is how a third caster's subclass casts.
type SubclassCasting struct {
	// Kind is CastingThird.
	Kind    string
	Ability Ability
	// Preparation is PreparationKnown or PreparationPrepared.
	Preparation string
	// SpellList is the class whose list it casts from, and StartLevel the class
	// level casting starts at.
	SpellList  string
	StartLevel int
	// MaxSpellLevelByLevel[n-1] is the highest spell level at class level n (0
	// before it casts), as ClassEntry's.
	MaxSpellLevelByLevel []int
}

// BackgroundEntry is a background in the Catalog.
type BackgroundEntry struct {
	Key, Name, NamePT  string
	SkillProficiencies []string
	// EquipmentPT is a table background's equipment text; empty for the SRD's.
	EquipmentPT string
	Archived    bool
	Off         bool
}

// SkillEntry is a skill in the Catalog.
type SkillEntry struct {
	Key, Name, NamePT string
	Ability           Ability
}

// ArmorEntry is a body armor in the Catalog. The shield is not listed: it
// is Build.Shield.
type ArmorEntry struct {
	Key, Name, NamePT string
	// Category is "light", "medium" or "heavy".
	Category string
	BaseAC   int
	// DexBonus says whether the dexterity modifier is added, and
	// MaxDexBonus caps it (0 means no cap).
	DexBonus            bool
	MaxDexBonus         int
	StrMinimum          int
	StealthDisadvantage bool
}

// WeaponEntry is a weapon in the Catalog.
type WeaponEntry struct {
	Key, Name, NamePT string
	// Category is "simple" or "martial"; Range is "melee" or "ranged".
	Category string
	Range    string
	// Damage is the dice, such as "1d8", and DamageType a key such as
	// "damage-type:slashing". TwoHandedDamage is set for versatile weapons.
	Damage          string
	DamageType      string
	TwoHandedDamage string
	// Properties are keys such as "weapon-property:finesse".
	Properties []string
	// NormalRangeFt and LongRangeFt are the range of ranged and thrown
	// weapons, 0 otherwise.
	NormalRangeFt int
	LongRangeFt   int
}

// SpellEntry is a spell in the Catalog.
type SpellEntry struct {
	Key, Name, NamePT string
	// Level is 0 for a cantrip.
	Level int
	// School is a key such as "school:evocation", and SchoolNamePT its
	// Portuguese name.
	School        string
	SchoolNamePT  string
	Classes       []string
	Ritual        bool
	Concentration bool
	// CastingTime is here, and not only in SpellDetails, because "Sua
	// vez" groups a character's spells by it (package combat).
	CastingTime CastingTime
	// Archived says the table retired the spell (see RaceEntry.Archived); the
	// tag keeps it out of the sheet's JSON, which the golden pins.
	Archived bool `json:",omitempty"`
	// Off says the master switched the spell off for the players (see
	// RaceEntry.Off); kept out of the sheet's JSON the same way.
	Off bool `json:",omitempty"`
}

// ProficiencyLevel says how much of the proficiency bonus a roll adds.
type ProficiencyLevel int

const (
	// ProficiencyNone adds nothing.
	ProficiencyNone ProficiencyLevel = iota
	// ProficiencyHalf adds half, rounded down (Jack of All Trades).
	ProficiencyHalf
	// ProficiencyFull adds the proficiency bonus.
	ProficiencyFull
	// ProficiencyExpertise adds twice the proficiency bonus.
	ProficiencyExpertise
)

// Derived is the computed sheet: everything the official character sheet
// shows that follows from the Build. The characters module maps it to
// rules.v1.DerivedSheet.
type Derived struct {
	// ContentVersion is Content.Version() at the time of Derive.
	ContentVersion string
	// RaceNamePT and SubraceNamePT name the race and subrace, and
	// BackgroundNamePT the SRD background or the custom one's name as
	// typed. Each is empty when the Build has none (or an unknown key).
	RaceNamePT       string
	SubraceNamePT    string
	BackgroundNamePT string
	// BackgroundEquipmentPT is the background's equipment as text: a table
	// background's (RN-23) or a custom one's. Empty for an SRD background, which
	// the SRD data does not carry.
	BackgroundEquipmentPT string
	// Classes are the Build's known classes, in Build order, with names.
	Classes []DerivedClass
	// Abilities has the six abilities, in AllAbilities order.
	Abilities []AbilityScore
	// TotalLevel is the sum of all class levels.
	TotalLevel       int
	ProficiencyBonus int
	// SavingThrows has the six saves, in AllAbilities order.
	SavingThrows []SavingThrow
	// Skills has all 18 skills, sorted by NamePT.
	Skills               []Skill
	PassivePerception    int
	PassiveInvestigation int
	PassiveInsight       int
	Initiative           int
	// ArmorClass is the best AC with the worn armor and shield, and
	// ArmorClassDescription says in Portuguese how it was computed, such as
	// "Sem armadura" or "Armadura de couro + escudo".
	ArmorClass            int
	ArmorClassDescription string
	HitPointsMax          int
	// HitPointsFromEffects is HitPointsMax minus what the hit dice and the
	// Constitution modifier give (with the floor of one per level): what the
	// "hp.max" effects add. It is negative for an effect that subtracts and 0
	// for a creature or a beast form.
	HitPointsFromEffects int
	// HitDice has one entry per die size, largest first.
	HitDice     []HitDice
	SpeedWalkFt int
	// The other speeds, in feet, 0 for none: a character has none today, a
	// creature (or a druid in Wild Shape) may fly, swim, climb or burrow.
	// Hover says a flying creature hovers.
	SpeedFlyFt, SpeedSwimFt, SpeedClimbFt, SpeedBurrowFt int
	Hover                                                bool
	Senses                                               []Sense
	// NextLevelXP is the XP that reaches TotalLevel+1, or 0 at level 20.
	// The sheet compares it with the XP the player has to show "Pode subir
	// de nível" (RN-12).
	NextLevelXP int
	// Spellcasting has one entry per casting class, in Build order.
	Spellcasting []Spellcasting
	// SpellSlots are the slots per spell level, index 0 is the 1st level;
	// it always has 9 entries. Warlock pact slots are in PactMagic.
	SpellSlots []int
	// PactMagic is nil unless the build has warlock levels.
	PactMagic *PactMagic
	// Spells are the character's cantrips and spells (the Build's lists
	// plus the subclass's always-prepared spells), cantrips first, then by
	// level and NamePT.
	Spells []CharacterSpell
	// Attacks are the carried weapons (Kind "weapon") and the cantrips
	// that deal damage (Kind "spell").
	Attacks []Attack
	// Resources are the use-limited features the character has, and Actions
	// the actions features grant (Second Wind is a bonus action that spends
	// the "second_wind" resource). StandardActions are the ones everyone
	// has (Attack, Dash...). All three feed package combat's TurnOptions.
	Resources       []Resource
	Actions         []Action
	StandardActions []Action
	// AttacksPerAction is how many attacks the Attack action makes: 1, or
	// the extra_attack effect's count (Extra Attack: 2 at level 5), or a
	// creature's Multiattack count.
	AttacksPerAction int
	// CriticalRange is the lowest natural d20 that is a critical hit with a
	// weapon attack: 20, or 19 with Improved Critical and 18 with Superior
	// Critical.
	CriticalRange int
	// TwoWeaponFighting says the character has the fighting style: the damage
	// of the bonus action attack keeps the ability modifier.
	TwoWeaponFighting bool
	// SaveActions are a creature's actions that ask for a saving throw
	// (a breath, a bite that knocks prone), with their DC. Empty for a
	// character.
	SaveActions []SaveAction
	// Features are the class, subclass, race and background features and
	// traits up to the character's level.
	Features []Feature
	// Languages are the languages the race grants, such as
	// {"language:common", "Comum"}. The ones the player chooses are free
	// text on the sheet.
	Languages     []NamedKey
	Proficiencies []Proficiency
	// Hints are situational bonuses the engine shows but does not apply
	// (ADR-0008: "vantagem se a fonte for mágica").
	Hints []Hint
	// ItemResistances are the damage types the equipped items give resistance to.
	ItemResistances []ItemResistance
	// ItemModifiers are the numbers the equipped items change, one line each, for the
	// sheet's "por causa de".
	ItemModifiers []ItemModifier
	// Issues are problems found while deriving: unknown keys, unusual
	// choices, a broken formula. They never stop the sheet from opening.
	Issues []Issue
}

// DerivedClass is one of the character's classes, with names.
type DerivedClass struct {
	// ClassKey and NamePT name the class ("class:wizard", "Mago").
	ClassKey, NamePT string
	Level            int
	// SubclassNamePT is the SRD subclass's name or the custom subclass's
	// name as typed, or empty.
	SubclassNamePT string
}

// CharacterSpell is a spell on the sheet.
type CharacterSpell struct {
	Spell SpellEntry
	// Prepared says whether it can be cast today: every cantrip, every
	// spell of a class that knows its spells, the prepared list and the
	// subclass's always-prepared spells.
	Prepared bool
}

// NamedKey is a content key with its Portuguese name.
type NamedKey struct {
	Key, NamePT string
}

// AbilityScore is one ability on the sheet.
type AbilityScore struct {
	Ability Ability
	NamePT  string
	// Base is the Build's score. RaceBonus comes from the race and the
	// subrace, ManualBonus from Build.ExtraAbilityBonuses, and Bonus is
	// their sum.
	Base        int
	RaceBonus   int
	ManualBonus int
	Bonus       int
	// Score is Base + Bonus, and Modifier is floor((Score - 10) / 2).
	Score    int
	Modifier int
}

// SavingThrow is one saving throw on the sheet.
type SavingThrow struct {
	Ability    Ability
	NamePT     string
	Proficient bool
	Bonus      int
}

// Skill is one skill on the sheet.
type Skill struct {
	Key, NamePT string
	Ability     Ability
	Proficiency ProficiencyLevel
	Bonus       int
}

// HitDice is a pool of hit dice of one size: Count d Die.
type HitDice struct {
	Die   int
	Count int
}

// Sense is a special sense, such as darkvision.
type Sense struct {
	// Key is "darkvision", "blindsight", "tremorsense" or "truesight".
	Key, NamePT string
	RangeFt     int
	// Source is the key of what grants it, such as "trait:darkvision".
	Source string
}

// Spellcasting is what one casting class gives.
type Spellcasting struct {
	// Class is the class key, and ClassNamePT its Portuguese name.
	Class       string
	ClassNamePT string
	Ability     Ability
	SaveDC      int
	AttackBonus int
	// CantripsKnown is the class table's number of cantrips.
	CantripsKnown int
	// PreparesSpells tells which of the next two numbers applies:
	// PreparedMax for classes that prepare, SpellsKnownMax for the others.
	PreparesSpells bool
	PreparedMax    int
	SpellsKnownMax int
	// MaxSpellLevel is the highest spell level this class alone could cast
	// (0 when it only knows cantrips).
	MaxSpellLevel int
	// Ritual says whether the class casts rituals.
	Ritual bool
	// SpellList is the class whose spell list this caster reads: the class
	// itself, or the class a third caster's subclass casts from (a table
	// subclass of the Fighter that casts from the wizard's list says
	// "class:wizard"). A spell is on it when the spell's classes (SpellEntry.Classes)
	// name SpellList; combat picks the caster of a spell with it, not with Class.
	SpellList string
}

// PactMagic is the warlock's pact slots, apart from the other slots.
type PactMagic struct {
	SlotLevel int
	Slots     int
}

// Attack is one line of the sheet's attacks: a weapon or an attack cantrip.
type Attack struct {
	// Key is the weapon or spell key; Name is the SRD's English name and
	// NamePT ours.
	Key, Name, NamePT string
	// Kind is "weapon" or "spell".
	Kind    string
	Ability Ability
	// AttackBonus is added to the d20. For a spell that asks for a saving
	// throw instead, it is 0 and SaveDC and SaveAbility are set.
	AttackBonus int
	SaveDC      int
	SaveAbility Ability
	// Damage is dice plus modifier, such as "1d8+3", and DamageType a key
	// such as "damage-type:slashing". VersatileDamage is the two-handed
	// damage of a versatile weapon.
	Damage           string
	DamageType       string
	DamageTypeNamePT string
	VersatileDamage  string
	// DamageDice and VersatileDice are Damage and VersatileDamage as
	// numbers, for the combat functions. Both are zero when the weapon has
	// no damage dice.
	DamageDice    DiceFormula
	VersatileDice DiceFormula
	Proficient    bool
	// RangeFt is the reach or normal range, LongRangeFt the long range.
	RangeFt     int
	LongRangeFt int
	// Melee is true for a weapon the SRD lists as a melee weapon, thrown or not
	// (a dagger, a spear, a handaxe), and false for a ranged weapon and a spell:
	// an opportunity attack needs a melee weapon.
	Melee bool
	// Beams is how many attack rolls the attack makes in one action, each with
	// its own damage roll: 4 for Eldritch Blast at character level 17. It is 1
	// for every other attack, and 0 for a weapon.
	Beams int
	// SpellDice is the plain dice a cantrip rolls at the character's level ("3d8"),
	// without the damage modifier; empty for a weapon and for a creature's attack.
	SpellDice string
	// Light is a melee weapon with the light property: the only kind of
	// Two-Weapon Fighting (the attack and the bonus action attack).
	Light bool
	// MartialArts says the Martial Arts bonus action unarmed strike goes with
	// this attack: the unarmed strike, and a monk weapon, while the monk wears
	// no armor and no shield.
	MartialArts bool
	// AbilityMod is the ability modifier inside Damage, which the bonus action
	// attack of Two-Weapon Fighting leaves out when it is positive.
	AbilityMod int
	// Notes is the rest of a creature's action, as the SRD wrote it (in
	// English): the damage parts after the first, a saving throw, a rider.
	// The engine rolls the to-hit and the first damage part; the master reads
	// the rest here. Empty for a character's weapon or cantrip.
	Notes string
	// Ammunition is the ammunition the weapon fires ("equipment:arrow"), or "" for
	// a weapon that needs none. AmmunitionItem is the inventory line the next shot
	// spends a piece of, or "" when the character carries no counted ammunition of
	// the kind (a sheet from before the inventory fires without limit). AmmunitionOut
	// says the character has stacks of it but no piece left.
	Ammunition, AmmunitionItem string
	AmmunitionOut              bool
}

// SaveAction is a creature's action that asks for a saving throw: a dragon's
// breath, or the Strength save of a wolf's bite.
type SaveAction struct {
	// Key is "<creature key>#<action>", such as "monster:wolf#bite", the
	// same key as the Attack of an action that also attacks.
	Key, Name string
	Ability   Ability
	DC        int
	// OnSuccess is "none", "half" or "other".
	OnSuccess string
	// Usage is the SRD's limit ("Recharge 5-6", "3/day"), or "".
	Usage string
	// Text is the SRD's text of the action, in English.
	Text string
}

// Feature is a class, subclass, race, subrace or background feature or
// trait that the character has.
type Feature struct {
	// Key is the feature or trait key, such as "feature:arcane-recovery"
	// or "trait:darkvision".
	Key, Name, NamePT string
	// Source is the key that grants it (a class, subclass, race, subrace
	// or background), and Level the class level at which it comes (0 for
	// race and background traits). SourcePT says it in Portuguese, such as
	// "Mago 1" or "Gnomo".
	Source   string
	Level    int
	SourcePT string
	// Description is the SRD text, in English (ADR-0008).
	Description []string
}

// Proficiency is an armor, weapon or tool proficiency.
type Proficiency struct {
	// Kind is "armor", "weapon", "tool" or "other".
	Kind        string
	Key, NamePT string
}

// Hint is a situational bonus or roll mode: the sheet shows it, and the
// master or the action decides when it applies.
type Hint struct {
	// Source is the feature or trait key that gives it.
	Source string
	// Target is what it affects, such as "save.int" or "skill:history".
	// When a hint covers several targets (Gnome Cunning: INT, WIS and CHA
	// saves), Target is the first and Targets lists them all.
	Target  string
	Targets []string
	// Mode is "advantage", "disadvantage", "bonus" or "note"; Value is set
	// for "bonus" (the total, such as History +8 for Artificer's Lore).
	Mode  string
	Value int
	// Tags are the conditions, such as "against:magic".
	Tags []string
	// TextPT is a ready-to-show sentence in Portuguese.
	TextPT string
}

// Issue is a problem Derive found. Code is stable (see the Issue* codes),
// Field points at the sheet field when it applies, as a path with the
// CharacterSheet proto's field names ("full.classes[0].subclass_key"), and
// Message is Portuguese text for the sheet.
//
// Keys are the table's content keys (RN-23, "...@mesa") the issue depends on:
// the entry at Field, and the entries whose numbers the problem is about (the
// classes for a skill count, the race for a bonus to place). The server blames a
// change of a table entry only for the issues that list its key.
type Issue struct {
	Code    string
	Field   string
	Message string
	Keys    []string `json:",omitempty"`
	// ChangeMessage is the sentence for "A classe mudou" (RN-23): the same problem
	// as a change of the table's entry reads, without the entry's name ("agora dá 2
	// perícias no nível 1; esta ficha tem 3."; see ChangeSubject). Only an issue tied to a table entry
	// (Keys) has one, and only for the common changes of a class: the skill count,
	// the cantrips, the known and the prepared spells, the level of the subclass,
	// the multiclass prerequisite and an option that is no longer offered. The
	// others tell it with Message.
	ChangeMessage string `json:",omitempty"`
	// ChangeSubject is the class or subclass key ChangeMessage is about ("agora dá
	// 2 perícias..." is what that entry gives). The sentence names it only when it
	// is the entry that changed; for any other it is told without a name. Empty
	// when ChangeMessage is not about one entry.
	ChangeSubject string `json:",omitempty"`
}

// Issue codes.
const (
	// IssueUnknownKey: a key in the Build is not in the content (for
	// example, a newer snapshot removed it).
	IssueUnknownKey = "unknown_key"
	// IssueSkillCount: more or fewer chosen skills than the class allows.
	IssueSkillCount = "skill_count"
	// IssueExpertise: expertise in a skill without proficiency, or more
	// expertise than the features give.
	IssueExpertise = "expertise"
	// IssueChoiceCount: more options picked for a feature than it allows
	// (two fighting styles at Fighter 1), or the same option picked twice.
	IssueChoiceCount = "choice_count"
	// IssueSpellNotOnList: a spell that is not on any of the build's class
	// lists.
	IssueSpellNotOnList = "spell_not_on_list"
	// IssueSpellCount: more cantrips, known or prepared spells than the
	// class allows.
	IssueSpellCount = "spell_count"
	// IssueSpellLevel: a spell above the highest level the build can cast.
	IssueSpellLevel = "spell_level"
	// IssueSubclassLevel: a subclass chosen before the class level that
	// grants it.
	IssueSubclassLevel = "subclass_level"
	// IssueMulticlass: a multiclass prerequisite is not met.
	IssueMulticlass = "multiclass_prerequisite"
	// IssueHitPointRolls: fewer or more hit point rolls than levels.
	IssueHitPointRolls = "hit_point_rolls"
	// IssueArmorProficiency: armor or a shield without the proficiency.
	IssueArmorProficiency = "armor_proficiency"
	// IssueScoreAbove20: a score above 20 without a manual bonus.
	IssueScoreAbove20 = "score_above_20"
	// IssueFormula: an effect's formula failed at run time and was
	// skipped.
	IssueFormula = "formula"
	// IssueMissing: something the sheet needs is missing (no race, no
	// class).
	IssueMissing = "missing"
	// IssueLevel: the total level is above 20; Derive computes level 20.
	IssueLevel = "level"
	// IssueRaceBonus: a table race lets the player place ability bonuses of
	// their choice (Catalog's RaceEntry.ChoiceBonuses) and the sheet's manual
	// bonuses do not cover them.
	IssueRaceBonus = "race_bonus"
)
