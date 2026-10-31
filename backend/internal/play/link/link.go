// Package link holds the small plain types that cross the borders between
// the play module and the modules it reads while it runs a combat: the
// characters module (who can fight, with which numbers) and the maps module
// (the grid, the battle point, the tokens).
//
// Modules never import each other's code (docs/architecture/README.md), and the
// numbers a combat needs are not API messages, so the types both sides
// agree on live here, in a package that imports nothing of the project.
// Package play declares the interfaces (CombatRoster, MapKeeper) that use
// them; cmd/api connects the real services.
package link

import (
	"errors"
	"slices"
	"time"
)

// What a Wild Shape change refuses with (MR-037): the characters module returns
// them, the play module turns them into the typed refusal the app reads.
var (
	// ErrBeastNotAllowed: the beast is not one the character's Wild Shape allows
	// (its level, the fly and swim limits, not a beast), or it has no Wild Shape.
	ErrBeastNotAllowed = errors.New("the beast is not one the character's Wild Shape allows")
	// ErrAlreadyInWildShape: the character is in a beast form already.
	ErrAlreadyInWildShape = errors.New("the character is in a beast form already")
)

// Character is what a combat needs to know about a character that fights:
// who it is and the numbers that start a combatant (MR-013). Numbers come
// from the sheet, derived by the rules module for a full sheet, and read
// as they are for a basic one.
type Character struct {
	ID   string
	Name string
	// Player is true for a player's character, false for an NPC.
	Player bool
	// PlayerUserID is the account that plays a player's character. Empty for
	// an NPC, and for a character whose player deleted their account (RN-16).
	PlayerUserID string
	// Reserved is true for a character the master gave back to the reserve (MR-049):
	// nobody owns it and no player may see it. Only SessionCharacters sets it; the
	// other reads leave a reserved character out.
	Reserved bool
	// InitiativeBonus is added to the d20 for the initiative.
	InitiativeBonus int
	// SpeedFt is the walking speed, in feet (5 ft per square of the grid).
	SpeedFt int
	// HitPointsMax is the maximum hit points. A combatant takes it only for
	// an NPC: a player character's hit points are in its vitals.
	HitPointsMax int
	// XPValue is the XP an NPC gives when defeated (its sheet's xp_value), 0
	// for a player's character (MR-016).
	XPValue int
	// PortraitImageID is the gallery image of an NPC's portrait (MR-031), ""
	// for none and for a player's character.
	PortraitImageID string
	// Size is the creature's size, one of "tiny", "small", "medium", "large",
	// "huge" or "gargantuan": a character's race size, or a basic sheet's Tamanho.
	// "" means medium (MR-034, RN-21).
	Size string
	// SpeedFlyFt is the fly speed in feet, 0 for none.
	SpeedFlyFt int
	// JumpLongDFt and JumpHighDFt are the long and high jump with a running start,
	// in tenths of a foot (rules/combat.JumpLimits); 0 for a sheet with no
	// Strength. Standing, each is half.
	JumpLongDFt, JumpHighDFt int
	// JumpLongBonusDFt is the extra distance of the running long jump that the
	// Thief's Second-Story Work gives (rules/combat.Jumps.LongRunningBonus).
	JumpLongBonusDFt int
	// CombatOnly marks the NPC the app keeps for a creature's monsters (RN-29):
	// never a participant, a stage NPC or a token.
	CombatOnly bool
	// CastsInBeastForm says the character keeps its spells in a beast shape
	// (a druid of level 18, Beast Spells); every other character has none there.
	CastsInBeastForm bool
	// ArmorUntrained says the character wears armor or a shield it is not proficient with,
	// and so cannot cast spells (rules.ArmorUntrained).
	ArmorUntrained bool
	// MonsterKey and ChallengeRating are the SRD creature an NPC was made from
	// ("monster:bandit") and its ND ("1/8"), "" for an NPC the master typed: the
	// master's view of a monster.
	MonsterKey, ChallengeRating string
}

// PartyMember is a player's character as an encounter's budget reads it (MR-043):
// its name and its total level.
type PartyMember struct {
	ID, Name string
	Level    int
}

// Grid is a map's battle grid: squares of 1.5 m (5 ft) across the image's
// width and down its height. The zero Grid means the map has none.
type Grid struct {
	Columns int32
	Rows    int32
}

// OK says whether the map has a grid.
func (g Grid) OK() bool { return g.Columns > 0 && g.Rows > 0 }

// BattlePoint is a battle point of a map: where its fight happens.
type BattlePoint struct {
	// MapID is the map the point is on.
	MapID string
	// TargetMapID is the map of the fight, when the master chose one; empty
	// when the fight happens on the session's current map.
	TargetMapID string
	// XBP and YBP are where the point is on its map, in basis points of the
	// image's width and height (0 to 10000).
	XBP, YBP int32
}

// TokenPosition is where a character's token stands on a map, in basis
// points of the image's width and height (0 to 10000).
type TokenPosition struct {
	// CharacterID is the character whose token it is; empty for a creature's.
	CharacterID string
	// CreatureID is the creature whose token it is (MR-037); empty for a
	// character's own token.
	CreatureID string
	XBP, YBP   int32
}

// Sheet is what the actions of a combat need from a character's sheet: the
// armor class an attack must reach, the attacks it can make and the standard
// actions it has (MR-012, MR-014). The characters module fills it from the
// derived full sheet, or from the structured attacks of a basic one.
type Sheet struct {
	// ArmorClass is the number a d20 plus the attack bonus must reach. It
	// never goes to a player (RN-20).
	ArmorClass int
	// Attacks are the weapon attacks and the attack cantrips, in sheet order.
	Attacks []Attack
	// Actions are the standard actions every creature has.
	Actions []Action
	// AttacksPerAction is how many attacks the Attack action makes: 1, or more
	// with Extra Attack.
	AttacksPerAction int
	// CriticalRange is the lowest natural d20 that is a critical hit with a
	// weapon attack: 20, 19 with Improved Critical, 18 with Superior Critical.
	// 0 (a basic sheet) is 20.
	CriticalRange int
	// BrutalCriticalDice is how many weapon damage dice a critical hit with a melee
	// weapon attack rolls on top of the doubled ones (the barbarian's Brutal
	// Critical: 1, 2 or 3 by level); 0 without it.
	BrutalCriticalDice int
	// SavageAttacks says the half-orc's Savage Attacks apply: a critical hit with a
	// melee weapon attack rolls one more of the weapon's damage dice.
	SavageAttacks bool
	// Metamagic are the Metamagic options the sorcerer knows (feature keys), ChaMod
	// is its Charisma modifier, and BardicDie the size of the die its Bardic
	// Inspiration gives, 0 for a character with none.
	Metamagic []string
	ChaMod    int
	BardicDie int
	// TwoWeaponFighting says the character has the fighting style.
	TwoWeaponFighting bool
	// FeatureActions are the actions the sheet's class and race features grant
	// (rules.Derived.Actions): their key, name, economy and resource.
	FeatureActions []FeatureAction
	// FighterLevel is what Retomar o fôlego adds to its d10: the character's
	// fighter level, 0 for anyone who is not a fighter.
	FighterLevel int
	// MonkLevel is the monk's level, 0 for a character with none: Slow Fall takes
	// five times it off a fall.
	MonkLevel int
	// ClericLevel is the cleric's level, 0 for a character with none: Preserve Life restores five times it,
	// and ChannelDC is the save DC of Turn Undead (8 + proficiency bonus + Wisdom modifier).
	ClericLevel, ChannelDC int
	// Senses are the special senses the sheet or stat block gives (darkvision...),
	// which an NPC sees with on a map with the fog of war (MR-036). The zero value
	// is plain sight.
	Senses Senses
	// Traits are the features the rolls and the damage of a combat read.
	Traits Traits
	// Hooks are the character's on_event effects (a failed death save, dropping to 0 hit points).
	Hooks []Hook
}

// Maneuver is a die a table option adds to a roll, spent from a resource (the
// engine's superiority_die effect). Applies, Economy and Ability are the effect's.
type Maneuver struct {
	Key, NamePT, Resource, Applies, Economy, Ability, TextPT string
	Sides, SaveDC                                            int
	// Targets are the skills ("skill.stealth") and abilities ("check.dex") a die on ability checks goes on;
	// empty is every ability check.
	Targets []string
}

// ExtraDamage is damage a table feature adds to a weapon hit (the engine's extra_damage effect):
// Applies says what it asks of the target ("any_target" or "larger_target"), Resource the uses it
// spends ("" for none), Amount the fixed damage (never doubled by a critical hit) and DiceCount d
// DiceSides the dice (0 for none; a critical hit doubles them unless NoCritDoubling). DamageType is
// the damage's own type, empty for the weapon's. Weapon ("any", "melee" or "ranged") says which weapon
// attacks it goes with and Frequency ("once_per_turn" or "each_hit") how often.
type ExtraDamage struct {
	Key, NamePT, Applies, Resource, TextPT string
	DamageType, Weapon, Frequency          string
	Amount, DiceCount, DiceSides           int
	NoCritDoubling                         bool
}

// SpellModifier is a cast option a table feature offers (the engine's spell_modifier effect): AppliesTo
// says which spells it fits ("damage_spell", "cantrip", a school key or a damage type key) and Change what
// it does ("damage_type_swap", "bonus_damage" or "extra_dice_per_slot_over"). From are the types a swap is
// among, Bonus and BonusType the fixed damage and its type (empty is the spell's), ExtraDice the dice per
// slot level above the spell's own, and Resource the resource each use spends ("" for none).
type SpellModifier struct {
	Key, NamePT, AppliesTo, Change, BonusType, Resource, TextPT string
	From                                                        []string
	Bonus, ExtraDice, Cost                                      int
}

// LuckyTrait is the content key of the halfling's Lucky (SRD 5.1, Halfling): the sheet that
// has it sets Traits.Lucky, and a d20 it rolled again names it as the source of the reroll.
const LuckyTrait = "trait:lucky"

// TaggedSave is an advantage (or a disadvantage, Disadvantage) on saving throws against one kind of source ("magic",
// "frightened", "charmed", "poison", or a condition key such as "condition:frightened" for a table's roll mode against
// a condition), as the sheet's trait gives it: Source is the key of the trait, Abilities the saves it covers (all when
// empty).
type TaggedSave struct {
	Source, Against string
	Abilities       []string
	Disadvantage    bool `json:",omitempty"`
}

// Traits are what a sheet has that changes how its owner rolls and what its
// damage carries: the barbarian's Rage, Reckless Attack and Danger Sense, the
// rogue's Sneak Attack, the paladin's Divine Smite, the ranger's Hunter's Mark and
// Colossus Slayer, the fighting style that rerolls low damage dice, a monster's
// Pack Tactics, the halfling's Lucky, the armor that cancels Rage and the damage
// resistances.
type Traits struct {
	// BarbarianLevel and RogueLevel are the class levels, 0 without the class.
	BarbarianLevel, RogueLevel int
	// Rage, RecklessAttack and DangerSense are the barbarian's features, and Frenzy
	// the Berserker's level 3 feature.
	Rage, RecklessAttack, DangerSense, Frenzy bool
	// SculptSpells says the sheet has the evocation wizard's Sculpt Spells.
	SculptSpells bool
	// SneakAttackDice is the dice of Sneak Attack, 0 without the feature.
	SneakAttackDice int
	// DivineSmite, ImprovedDivineSmite and ColossusSlayer are the features of those
	// names, and HuntersMark says the character has the spell.
	DivineSmite, ImprovedDivineSmite, ColossusSlayer, HuntersMark bool
	// GreatWeaponFighting is the fighting style.
	GreatWeaponFighting bool
	// OpenHand and StunningStrike are the monk's Open Hand Technique (a rider on a
	// Flurry of Blows hit) and Stunning Strike (a rider on a melee hit, 1 ki point);
	// KiSaveDC is the monk's ki save DC (8 + proficiency + Wisdom modifier), 0 for
	// anyone who is not a monk.
	OpenHand, StunningStrike bool
	KiSaveDC                 int
	// PackTactics is a monster's trait.
	PackTactics bool
	// Evasion says the sheet has Evasion: the monk's, the rogue's, the ranger's Superior
	// Hunter's Defense choice, or a stat block's trait (combat.HalfSaveShare).
	Evasion bool
	// Lucky is the halfling's trait (LuckyTrait): a 1 on the d20 of an attack roll or a saving
	// throw may be rolled again, and the new roll stands. The player is asked before the result.
	Lucky bool
	// TaggedSaves are the racial advantages on saving throws that hold against one kind of source (Brave, Fey
	// Ancestry, Gnome Cunning, Dwarven Resilience).
	TaggedSaves []TaggedSave
	// HeavyArmor says the armor worn is heavy: Rage gives none of its benefits then.
	HeavyArmor bool
	// StealthDisadvantage says the armor worn gives disadvantage on Dexterity (Stealth).
	StealthDisadvantage bool
	// ArmorUntrained says the character wears armor or a shield it is not proficient with.
	ArmorUntrained bool
	// Resistances are the damage resistances, immunities and vulnerabilities the features and traits give.
	Resistances []Resistance
	// ConditionImmunities are the conditions the features and traits make the sheet immune to.
	ConditionImmunities []ConditionImmunity `json:",omitempty"`
	// Maneuvers are the dice the table's options add to a roll (superiority_die effects).
	Maneuvers []Maneuver
	// ExtraDamages are the damage the table's features add to a weapon hit (extra_damage effects).
	ExtraDamages []ExtraDamage
	// SpellModifiers are the cast options the table's features offer (spell_modifier effects).
	SpellModifiers []SpellModifier
	// CreatureType is the SRD type of a monster ("undead", "fiend", "beast"); empty
	// for a character.
	CreatureType string
	// SpellAttackIgnoresCover says half and three-quarters cover give the target no AC
	// bonus against the character's ranged spell attacks, and RangedWeaponIgnoresCover
	// the same against its ranged weapon attacks (a thrown weapon used at range too);
	// RangedWeaponNoLongRangeDisadvantage says those weapon attacks have no disadvantage
	// beyond the normal range. A table feat's ranged_attack effect gives them
	// (rules.Derived); total cover still blocks. The spell range multiplier is already
	// in the ranges (Spell.RangeFt, Attack.RangeFt of a cantrip).
	SpellAttackIgnoresCover, RangedWeaponIgnoresCover, RangedWeaponNoLongRangeDisadvantage bool
	// UnactedAdvantage and SurprisedCritical are the keys of the table features (ambush effects)
	// that give advantage against a creature that has not had its turn yet and make a hit on a
	// surprised creature critical; empty for none.
	UnactedAdvantage, SurprisedCritical string
}

// Resistance is a damage resistance of a feature or trait: its key and name, the
// damage types it halves ("damage-type:fire") and when ("" always, "rage").
type Resistance struct {
	Source, NamePT string
	DamageTypes    []string
	While          string
	// Kind is rules.ResistanceKindResist, ResistanceKindImmune or ResistanceKindVulnerable, and Nonmagical
	// limits it to the damage of attacks that are not magical.
	Kind       string `json:",omitempty"`
	Nonmagical bool   `json:",omitempty"`
}

// ConditionImmunity is a condition the sheet takes none of, with the trait that says so.
type ConditionImmunity struct {
	Source     string
	Conditions []string
}

// Senses are a creature's special senses, as a range in feet (0 for none).
type Senses struct {
	DarkvisionFt, BlindsightFt, TruesightFt int
}

// Attack is one attack of a sheet, with real dice.
type Attack struct {
	// Key identifies it in the sheet: a weapon or spell key, "basic:0" for the
	// first attack of a basic sheet.
	Key string
	// Name is its Portuguese name.
	Name string
	// Save says it asks for a saving throw instead of an attack roll, which
	// the spells slice handles.
	Save bool
	// Spell says it is a cantrip, cast with the whole action: Extra Attack
	// belongs to weapon attacks only.
	Spell bool
	// ToHit is added to the d20.
	ToHit int
	// DiceCount d DiceSides plus DiceBonus is the damage. DiceCount 0 is a flat
	// number.
	DiceCount, DiceSides, DiceBonus int
	// DamageType is a content key such as "damage-type:slashing".
	DamageType string
	// RangeFt is the reach or normal range, LongRangeFt the long range, both
	// in feet; 0 when the sheet says none (a melee attack reaches 5 ft).
	RangeFt, LongRangeFt int
	// Melee says it is a melee weapon, thrown or not: the only kind an
	// opportunity attack can use, with the melee reach.
	Melee bool
	// ReachFt is the melee reach a weapon rule, a pseudo-weapon or a creature's stat
	// block gives the attack line, in feet; 0 when none does (a melee attack reaches 5 ft). A melee weapon with a
	// throwing range is a melee attack up to this reach and a throw beyond it.
	ReachFt int
	// Beams is how many attack rolls the cantrip makes in one action (Eldritch
	// Blast: 2, 3 and 4 beams from character levels 5, 11 and 17); 0 or 1 for
	// anything else.
	Beams int
	// Light is a light melee weapon. Unarmed is the unarmed strike, and
	// MartialArts says the Martial Arts strike goes with the attack. AbilityMod
	// is the ability modifier inside DiceBonus. The bonus action attacks read
	// them.
	Light, Unarmed, MartialArts bool
	AbilityMod                  int
	// Ability is the ability the attack uses ("str", "dex"...), Finesse and TwoHanded
	// the weapon's properties, and Weapon says it is a weapon attack (an unarmed
	// strike too) and not a spell.
	Ability            string
	Finesse, TwoHanded bool
	Weapon             bool
	// Magical says the attack's damage is magical (a spell; a creature's weapon attack
	// with the trait "Magic Weapons"; any attack the sheet marks, such as an unarmed
	// strike): the resistances and immunities to nonmagical attacks do not cover it.
	Magical bool
}

// Action is a standard action: its key ("standard:dash") and Portuguese name.
type Action struct {
	Key, Name string
}

// FeatureAction is an action a feature grants ("feature:second-wind").
type FeatureAction struct {
	Key, Name string
	// Economy is "action", "bonus_action", "reaction", "free" or "movement".
	Economy string
	// Resource is the key of the resource each use spends, or "".
	Resource string
	// Pool says the resource is a pool of points (Cura pelas mãos), not a count
	// of uses: using the action spends no point.
	Pool bool
	// Standard is the standard action it performs ("standard:dash" for Cunning
	// Action's Dash), or "".
	Standard string
	// Cost is the uses each use spends (0: one) and CostMax the most the player may choose to
	// spend (0: the cost is fixed); the table's content sets them.
	Cost, CostMax int
	// Heal is the dice the action heals, nil for none, HealPerPoint what each point above the
	// cost adds to it; TempHP is the temporary hit points it gives and TempHPPerPoint what each
	// point above the cost adds.
	Heal                   *Dice
	HealPerPoint           Dice
	TempHP, TempHPPerPoint int
	// State is the key of the state the action starts or ends (rules.Action.State), or "";
	// StateActive says the state is on, so taking the action ends it.
	State       string `json:",omitempty"`
	StateActive bool   `json:",omitempty"`
	// StateRounds is how many rounds the state lasts once started, 0 until it ends.
	StateRounds int `json:",omitempty"`
}

// Hook is an on_event effect of the character (rules.EventHook): what the hook of an event gives,
// and the resource it spends. Source is the key of the feature that has it and SourceNamePT its name.
type Hook struct {
	Event, Source, SourceNamePT string
	// Heal is the dice regained, HealPercentMax the share of the maximum, TempHP the temporary hit points.
	Heal           *Dice
	HealPercentMax int
	TempHP         int
	// Restore is the resource whose uses come back and RestoreCount how many.
	Restore      string
	RestoreCount int
	// Spends is the resource the hook costs and SpendCount how many uses: it runs only with that many left.
	Spends     string
	SpendCount int
}

// Dice is a roll: Count d Sides plus Bonus (Count 0 is a flat number), with the
// damage type for a damage.
type Dice struct {
	Count, Sides, Bonus int
	// DamageType is a key such as "damage-type:fire"; empty for a heal.
	DamageType string
}

// Spell is what a cast needs from a spell and the caster's sheet, resolved at
// the slot level the spell is cast with (MR-014).
type Spell struct {
	// Nonmagical says the effect is a creature's action from its stat block, not a spell: its saving throw
	// is not against magic (Magic Resistance, Gnome Cunning do not apply).
	Nonmagical bool
	Key, Name  string
	// Level is the spell's own level, 0 for a cantrip, and School its school's key ("school:evocation").
	Level  int
	School string
	// Economy is "action", "bonus_action" or "reaction"; "" for a casting time
	// too long for a fight.
	Economy       string
	Concentration bool
	// Verbal says the spell has a verbal component: the noise gives a hidden caster's
	// position away.
	Verbal bool
	// RangeKind is "self", "touch", "ranged", "sight", "unlimited" or "special",
	// and RangeFt the distance of a ranged spell as this caster casts it: a spell
	// that needs an attack roll reaches farther when a table feat multiplies its
	// range (rules.Derived.SpellRangeFt).
	RangeKind string
	RangeFt   int
	// AttackType is "melee", "ranged" or "": a spell attack, with ToHit added to
	// the d20.
	AttackType string
	ToHit      int
	// SaveAbility is the ability of the saving throw ("dex"), "" when there is
	// none; SaveOnSuccess is "half", "none" or "other"; SaveDC the caster's DC.
	SaveAbility   string
	SaveOnSuccess string
	SaveDC        int
	// CasterDC is the caster's spell save DC, whether or not the spell asks for a saving
	// throw: the DC of the saves an effect that lasts asks later (Web, Hold Person).
	CasterDC int
	// Damages are the spell's damage at the slot level, one part for each damage
	// type it deals (Ice Storm has two), empty when it has none the engine can
	// roll; Heal is its healing with the caster's spellcasting modifier already in
	// Bonus, nil when it does not heal.
	Damages []Dice
	Heal    *Dice
	// DamageChoice says what the caster picks among the damage types: "scale"
	// (every part is dealt, the higher-slot dice go to the picked type),
	// "alternative" (only the picked type is dealt) or "" (nothing to pick).
	// DamageTypes are the types the caster may pick, and Damages already follow
	// the pick the spell was resolved with.
	DamageChoice string
	DamageTypes  []string
	// Area says the spell hits every creature in an area: any number of targets.
	// ExtraTargetPerLevel says it takes one more target for each slot level
	// above its own.
	Area                bool
	ExtraTargetPerLevel bool
	// Evocation says the spell is of the school of evocation, and Sculpts that the
	// caster has Sculpt Spells (SRD 5.1, School of Evocation): together they let the
	// cast spare 1 + the cast level creatures.
	Evocation, Sculpts bool
	// TargetCount is how many targets the spell takes at its own level (0 when the
	// spell says only Area or nothing: the old rules apply), and TargetPerLevel
	// how many more it takes for each slot level above its own. A table spell
	// says both itself ("três criaturas, uma a mais por nível"); an SRD spell's
	// come from the same fields of rules.SpellTarget.
	TargetCount    int
	TargetPerLevel int
	// CasterOnly says there is nobody to pick: the spell reaches the caster alone,
	// or no creature at all (a point, an object, a place), whatever its range.
	CasterOnly bool
	// HP is set for a spell that reads hit points (Sono, Palavra de Poder...):
	// Damage and Heal are nil for it, and the cast applies HP instead.
	HP *HPEffect
	// Summon says the spell brings creatures (Convocar Familiar, Animar os
	// Mortos, Conjurar Animais; MR-037): the cast takes a choice of creatures
	// instead of targets.
	Summon bool
	// IgnoresCover says the spell's saving throw gets no benefit from cover (Chama
	// Sagrada, SRD 5.1); the total-cover targeting refusal stays.
	IgnoresCover bool
	// HalfOnMiss says a spell attack that misses still deals half its damage (Acid Arrow).
	HalfOnMiss bool
	// AreaShape is the form of the spell's area ("sphere", "cylinder", "cone", "line"
	// or "cube"), AreaSizeFt its radius (sphere, cylinder), length (cone, line) or
	// side (cube) in feet, and AreaWidthFt a line's width: empty and 0 for a spell
	// that is not an area with a shape. SpreadsAroundCorners says the area is the
	// part of the shape connected to its origin, not what the origin sees.
	AreaShape            string
	AreaSizeFt           int
	AreaWidthFt          int
	SpreadsAroundCorners bool
	// Zone is the area a table spell keeps on the map while it lasts: nil for an SRD spell (the combat knows
	// the SRD's four) and for a table spell that keeps none.
	Zone *ZoneRule
	// ZoneLaid says this cast lays the area of the spell on the map as a zone (the request chose a point): the
	// area is then laid out as the zone says. Set by the cast, never by the roster.
	ZoneLaid bool
}

// HPEffect is what a spell that reads hit points does at the slot level, from
// the rules' effects/spells.json (rules.SpellEffect). Kind is the rules'
// SpellKind: "hp_pool", "hp_threshold", "zero_hp_target", "flat_heal", "temp_hp"
// or "max_hp".
type HPEffect struct {
	Kind string
	// Pool is the dice of an hp_pool spell, with the extra dice of the slot
	// level already in Count.
	Pool Dice
	// Condition is the key given to each creature affected; empty for none.
	Condition string
	// Threshold is the most hit points a creature may have, and Dies says the
	// spell kills it instead (an hp_threshold spell).
	Threshold int
	Dies      bool
	// Heal is a flat_heal's healing at the slot level, and Ends the conditions
	// it ends.
	Heal int
	Ends []string
	// Amount is what a temp_hp spell adds to its Pool dice (the spell's dice are
	// in Pool, with no bonus) and what a max_hp spell raises the maximum by, at
	// the slot level.
	Amount int
}

// Save is a creature's saving throw: the bonus added to the d20, and whether
// the sheet has one at all. A basic-sheet NPC has none: Known is false and the
// Bonus 0.
type Save struct {
	Bonus int
	Known bool
}

// ReactionStats is what a combatant's sheet or stat block says about its
// reactions (PM-04): the spells it can cast and its slots, the numbers of each
// reaction and the features that give them. A player's character spends from its
// vitals; for anyone else the slots are the sheet's or the stat block's total and
// the combat counts what a fight spent.
type ReactionStats struct {
	// Spells are the keys of the spells it can cast now: every cantrip, the spells
	// it knows or has prepared, the always-prepared ones and, for a stat block, the
	// ones its Spellcasting trait lists.
	Spells []string
	// SlotsTotal are its slots by spell level (index 0 the 1st), PactLevel and
	// PactSlots the warlock's pact slots.
	SlotsTotal           [9]int
	PactLevel, PactSlots int
	// CastingMod is the spellcasting ability modifier (what a Counterspell check
	// adds), SaveDC the spell save DC.
	CastingMod, SaveDC int
	// InfernalLegacy says Hellish Rebuke can be cast through the tiefling's trait
	// (level 3), once a long rest, as a 2nd-level spell; its DC is the Charisma one.
	InfernalLegacy bool
	LegacyDC       int
	// UncannyDodge says the combatant has the rogue's feature.
	UncannyDodge bool
	// Deflect says it has Deflect Missiles; MonkLevel and DexMod are its numbers.
	Deflect           bool
	MonkLevel, DexMod int
	// Reduce are the table maneuvers that reduce the damage of a melee attack that hit
	// (superiority_die, applies reduce_melee_damage), each with the ability modifier it adds.
	Reduce []ReduceManeuver
	// Proficiency is the proficiency bonus (the Deflect Missiles throw back adds it).
	Proficiency int
	// Protection says it has the Protection fighting style and wields a shield (a
	// two-handed weapon on the sheet takes the shield out of its hand).
	Protection bool
	// CuttingWords says it has the bard's feature, with its bard level (the die)
	// and the bard's "Perguntar" setting: "all", "attacks" (the default) or "never".
	CuttingWords bool
	BardLevel    int
	CuttingAsk   string
	// CharmImmune says the stat block is immune to being charmed (Cutting Words
	// does nothing to it).
	CharmImmune bool
	// StatBlock says the combatant is a creature's stat block: its slots come from
	// the trait and the combat counts them.
	StatBlock bool
	// ResourceMax are the uses its features give at its level, by resource key
	// ("bardic_inspiration", "ki", "infernal_legacy"): the combat counts what an NPC
	// spent; a player's character spends from its vitals.
	ResourceMax map[string]int
}

// ReduceManeuver is a table maneuver that takes a die plus an ability modifier off the damage
// of a melee hit, as a reaction.
type ReduceManeuver struct {
	Key, NamePT, Resource string
	Sides, Mod            int
}

// Named is a content key with its Portuguese name.
type Named struct {
	Key, NamePT string
}

// Turn is what a combatant used in the current turn, for working out what it
// can still do.
type Turn struct {
	ActionUsed, BonusActionUsed, ReactionUsed bool
	// AttacksMade is how many attacks the Attack action made this turn.
	AttacksMade int
	// AttackKey is the attack the Attack action made last this turn ("" if none)
	// and FlurryLeft the unarmed strikes of Flurry of Blows left: what the bonus
	// action attacks read.
	AttackKey  string
	FlurryLeft int
	// FrenzyReady says the combatant is in a frenzied rage that began in an earlier
	// turn (the Berserker's Frenzy): a melee weapon attack is its bonus action.
	FrenzyReady bool
	// Dashed says the Dash action doubled the speed.
	Dashed bool
	// ActionSurged says Action Surge was used this turn (once per turn, whatever
	// the uses left).
	ActionSurged bool
	// SpellCast and BonusSpellCast say which kinds of spell were cast this turn
	// (see combat.TurnState).
	SpellCast, BonusSpellCast bool
	// SpeedFt is the combatant's best speed in a combat (walking, or flying when
	// it can), and MovementUsedFt the feet walked this turn, rounded down.
	SpeedFt, MovementUsedFt int
	// MovementUsedDFt is the movement used this turn in tenths of a foot, which is
	// what the server charges (RN-21); LastMoveDFt the length of the last move on
	// foot this turn, the running start of a jump; Disengaged says the Disengage
	// action was taken.
	MovementUsedDFt, LastMoveDFt int
	// JumpLongDFt and JumpHighDFt are the combatant's jump limits with a running
	// start, in tenths of a foot, as kept on the combatant (copied from the sheet
	// when it joined, like its speed): the turn options show exactly what the
	// server enforces. Standing, each is half.
	JumpLongDFt, JumpHighDFt int
	// JumpLongBonusDFt is what a long jump with a running start adds (Second-Story
	// Work), and JumpDoubled says Step of the Wind doubled the jump distance this turn.
	JumpLongBonusDFt int
	JumpDoubled      bool
	Disengaged       bool
}

// Scene is an RP scene (MR-015): a SCENE point of a map as the session opens
// it, with the master's actions on it. The DCs are in it, so it never goes to
// a player as it is.
type Scene struct {
	PointID string
	// Name and Description are the point's: the players read both in the open
	// scene.
	Name, Description string
	// Actions are in the order the master put them.
	Actions []SceneAction
	// Hooks is the master's private "Ganchos e anotações" (MR-029), and Clues
	// the clues the master prepared. They never go to a player (RN-20).
	Hooks string
	Clues []SceneClue
	// Images are the gallery images attached to the scene (MR-015), in the
	// master's order. They never go to a player (RN-10).
	Images []SceneImage
	// ShowDC is the master's "Mostrar a CD aos jogadores" switch: when it is
	// on, a player gets each DC and the pass or fail of their own rolls.
	ShowDC bool
}

// ImageAudience is who an image left with the players was left with (MR-028, RN-10): the whole
// table, or exactly the players UserIDs. The master always sees every image.
type ImageAudience struct {
	Everyone bool
	UserIDs  []string
}

// Union is the widest of the two: everyone if either is, else the players of both, without repeats.
func (a ImageAudience) Union(b ImageAudience) ImageAudience {
	if a.Everyone || b.Everyone {
		return ImageAudience{Everyone: true}
	}
	out := ImageAudience{UserIDs: slices.Clone(a.UserIDs)}
	for _, u := range b.UserIDs {
		if !slices.Contains(out.UserIDs, u) {
			out.UserIDs = append(out.UserIDs, u)
		}
	}
	return out
}

// SceneImage is a gallery image attached to a scene.
type SceneImage struct {
	ID, Name string
	// ShowsWholeMap is GalleryImage.shows_whole_map.
	ShowsWholeMap bool
}

// SceneClue is a clue of a scene, with who has it.
type SceneClue struct {
	ID, Text string
	// RevealedTo are the players it was revealed to, oldest first.
	RevealedTo []ClueRecipient
}

// ClueRecipient is a player's character a clue was revealed to.
type ClueRecipient struct {
	CharacterID string
	RevealedAt  time.Time
}

// SceneAction is one check of a scene.
type SceneAction struct {
	ID string
	// Key is "skill:investigation", "ability:str" or "save:wis".
	Key string
	// Name is the master's name for it, "" for none.
	Name string
	// DC is the difficulty class, 0 for none.
	DC int
	// MaxAttempts is how many times each player's character may roll it while
	// the scene is open: 1 to 5, and 0 means unlimited.
	MaxAttempts int
	// Hidden says the master hid the action from the players: package play offers it to the master
	// alone and refuses a player's roll on it as on an action that does not exist (RN-10).
	Hidden bool
}

// SceneOption is a scene check with one character's numbers on it (the rules
// engine's SceneOption).
type SceneOption struct {
	// Known says the sheet has this check; a basic sheet has no skills, so its
	// character has no numbers for a scene.
	Known bool
	// CheckName is the check's name in Portuguese: "Investigação".
	CheckName string
	// Bonus is added to the d20.
	Bonus int
	// Passive is the character's passive value, when HasPassive.
	Passive    int
	HasPassive bool
	// ReliableTalent says a d20 of this check counts as at least 10 (the
	// character has Reliable Talent and the check adds the proficiency bonus).
	ReliableTalent bool
	// Lucky says the character is a halfling (LuckyTrait): a 1 on the d20 of
	// this check (or saving throw) may be rolled again, and the new roll
	// stands. Its player is asked before the result.
	Lucky bool
}

// Creature is a creature of a player's character (MR-037) as a
// combat reads it: an SRD creature the table named, with its own hit points.
// The stat block is rules content; what a combat needs of it is copied in.
type Creature struct {
	ID string
	// CharacterID is the owner, and OwnerUserID the owner's player ("" when the
	// player left).
	CharacterID, OwnerUserID string
	// MonsterKey is the SRD creature, "monster:wolf", and Name what the table
	// calls it (1 to 40 characters).
	MonsterKey, Name string
	// Source is "familiar", "animate_dead", "conjure_animals" or "master".
	Source string
	// Attack is "none", "reaction" or "full" (rules.SummonAttack*).
	Attack string
	// GroupID is shared by the creatures of one casting.
	GroupID string
	// DependsOnConcentration says it lasts only while the caster concentrates.
	DependsOnConcentration bool
	// HitPointsCurrent and HitPointsMax are its own.
	HitPointsCurrent, HitPointsMax int
	// InitiativeBonus is its Dexterity modifier and SpeedFt its best speed, in
	// feet: what a combatant copies when it joins.
	InitiativeBonus, SpeedFt int
	// SpeedFlyFt is its fly speed, 0 for none; Size its size ("tiny" to
	// "gargantuan"); JumpLongDFt and JumpHighDFt its jump limits with a running
	// start, in tenths of a foot (rules/combat.JumpLimits of its Strength). What a
	// combatant copies when it joins, as a character's.
	SpeedFlyFt  int
	Size        string
	JumpLongDFt int
	JumpHighDFt int
}

// CreatureSpec is one creature a casting makes: the SRD creature, the name the
// player gave it ("" for the default) and what it may do on its own.
type CreatureSpec struct {
	MonsterKey, Name, Attack string
}

// Summon is a casting to record: the creatures a spell made for a character.
type Summon struct {
	CampaignID, CharacterID string
	// Source is "familiar", "animate_dead" or "conjure_animals".
	Source string
	// GroupID is the id the creatures of the casting share.
	GroupID string
	// Concentration says the creatures last only while the caster concentrates.
	Concentration bool
	Creatures     []CreatureSpec
}

// SummonResult is what recording a casting did: the creatures it made, in
// order, and the ones it dismissed (a new familiar replaces the old one).
type SummonResult struct {
	Created, Replaced []Creature
}

// SummonedForm is a creature a casting may make, as the rules check it.
type SummonedForm struct {
	MonsterKey, Attack string
}

// SummonSpell is what a summoning spell asks and what the character has of
// it, from the spell's data and the sheet.
type SummonSpell struct {
	// Known says the spell is on the character's sheet (a wizard's spellbook
	// counts), Prepared that it can be cast with a slot today, and CanRitual
	// that the character may cast it as a ritual: the spell is a ritual and the
	// character's class casts rituals.
	Known, Prepared, CanRitual bool
	// Ritual, Concentration and CastingUnit come from the spell: Find Familiar
	// is a ritual that takes 1 hour, Conjure Animals takes 1 action and needs
	// concentration. CastingUnit is a rules.Cast* constant.
	Ritual, Concentration bool
	CastingUnit           string
	// Level is the spell's own circle.
	Level int
	// Creatures are the creatures the choice made, with what each may do.
	Creatures []SummonedForm
}

// CreatureState is where a combat leaves a creature: its hit points and whether
// it is out of the fight (0 hit points).
type CreatureState struct {
	ID       string
	HP       int
	Defeated bool
}

// CreatureChanges says what keeping creatures in step with a combat did.
type CreatureChanges struct {
	// Dismissed are the creatures that were defeated and are dismissed now;
	// Revived the ones that were dismissed as defeated and an undo healed.
	Dismissed, Revived []Creature
}

// MonsterHitPoints are the hit points of an SRD creature: its average, and the
// dice it rolls them with ("2d8+2" is 2, 8, 2). DiceCount is 0 when the
// creature's dice are not known, and then the average is all there is.
type MonsterHitPoints struct {
	Average                         int
	DiceCount, DiceSides, DiceBonus int
}

// DeadCharacter is a player's character the master marked dead: a target Revivify may
// reach outside a combat, and the master's "Reviver".
type DeadCharacter struct {
	ID   string
	Name string
	// PlayerUserID is the account that plays it, empty when the player left (RN-16).
	PlayerUserID string
	// RevivifyBlocked is the master's switch "Revivificar não funciona nesta morte".
	RevivifyBlocked bool
}

// ZoneRule is the area a table spell keeps on the map: Trigger is "start_of_turn", "first_entry" or "both", and
// FollowsCaster says the area is centered on the caster and goes where they go.
type ZoneRule struct {
	Trigger       string
	FollowsCaster bool
}
