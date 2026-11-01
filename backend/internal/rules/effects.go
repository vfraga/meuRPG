package rules

import (
	"fmt"
	"reflect"
	"slices"
	"strings"

	"github.com/PuraFome/meuRPG/backend/internal/rules/formula"
)

// Effect is one structured effect of a feature, trait or background, as
// written by hand in srd51/effects/*.json (ADR-0008, "efeitos
// estruturados"). The effect types are closed: load refuses any other.
//
// Only some fields apply to each type; load checks that the ones a type
// needs are there. Formulas (Value, When, PreparedMax, Max) are compiled
// once, when the content loads.
type Effect struct {
	// Type is one of the effect types below.
	Type string `json:"type"`

	// modifier: changes a derived number. Target is one of the targets in
	// modifierTargets (or "skill.<skill>", "save.<ability>",
	// "damage.spell.<spell>"); Mode is add, max or set; Value is an Int
	// formula.
	Target string `json:"target,omitempty"`
	Mode   string `json:"mode,omitempty"`
	Value  string `json:"value,omitempty"`

	// When is an optional Bool formula: the effect applies only while it
	// is true (Unarmored Defense: `armor() == "none"`).
	When string `json:"when,omitempty"`

	// Tags make an effect situational ("against:magic",
	// "about:magic-items"). The engine never applies a tagged effect: it
	// shows it as a Hint, and the master decides (ADR-0008).
	Tags []string `json:"tags,omitempty"`

	// proficiency: grants a proficiency. Proficiency is a skill key, "skill:*"
	// (every skill), "save.<ability>", "initiative", or a proficiency key
	// such as "proficiency:heavy-armor". Level is half, full (the default)
	// or expertise.
	Proficiency string `json:"proficiency,omitempty"`
	Level       string `json:"level,omitempty"`

	// roll_mode: Roll is advantage or disadvantage on Targets (such as
	// "save.int" or "skill.stealth"), usually with Tags.
	Roll    string   `json:"roll,omitempty"`
	Targets []string `json:"targets,omitempty"`

	// sense: Sense is darkvision, blindsight, tremorsense or truesight,
	// with RangeFt.
	Sense   string `json:"sense,omitempty"`
	RangeFt int    `json:"range_ft,omitempty"`

	// spellcasting: how a class casts. Progression is full, half or pact;
	// PreparedMax is an Int formula for classes that prepare; Spellbook
	// says the class prepares from a spellbook (the wizard) instead of
	// from its whole list.
	Ability     string `json:"ability,omitempty"`
	Progression string `json:"progression,omitempty"`
	Prepares    bool   `json:"prepares,omitempty"`
	PreparedMax string `json:"prepared_max,omitempty"`
	Spellbook   bool   `json:"spellbook,omitempty"`
	Ritual      bool   `json:"ritual,omitempty"`

	// resource: a use-limited resource. Max is an Int formula; Recharge is
	// short_rest, long_rest, dawn, none, initiative, restore_one_on_short_rest or
	// restore_one_on_long_rest. It becomes Derived.Resources; the
	// session counts the uses (RN-02).
	// RechargeIf is an optional Bool formula: while it holds, the resource
	// recharges as RechargeThen instead of Recharge (Bardic Inspiration comes
	// back on a short rest too from bard level 5). Both or neither.
	Resource     string `json:"resource,omitempty"`
	Max          string `json:"max,omitempty"`
	Recharge     string `json:"recharge,omitempty"`
	RechargeIf   string `json:"recharge_if,omitempty"`
	RechargeThen string `json:"recharge_then,omitempty"`

	// Cap is, for a modifier on a score ("score.str"), the highest value the
	// effect may lift the score to (24 for Primal Champion); a score already
	// above it is left as it is. It also raises the ceiling over which the
	// sheet reports a score above the normal maximum.
	Cap int `json:"cap,omitempty"`

	// choice: something the player chooses. Choice is one of choiceKinds,
	// Count how many, From the keys (or a class key, for a spell list). The
	// same Count is, for extra_attack, how many attacks the Attack action
	// makes (2 at level 5); the highest of a character's effects wins, and a
	// character without one makes 1.
	Choice string   `json:"choice,omitempty"`
	Count  int      `json:"count,omitempty"`
	From   []string `json:"from,omitempty"`

	// grant_action: an action for "Sua vez" (MR-014). Economy is action,
	// bonus_action, reaction, free or movement. It becomes Derived.Actions
	// (package combat reads it); the standard actions are in
	// effects/standard_actions.json.
	Economy string `json:"economy,omitempty"`

	// superiority_die: a die a table option adds to a roll, spent from a resource
	// (a Battle Master style maneuver). Resource is the resource it spends, Value the
	// sides of the die (an Int formula), Applies where the die goes (maneuverApplies),
	// Economy what it costs when it costs one (bonus_action or reaction; empty is
	// free) and Ability, for reduce_melee_damage, the modifier added to the die. The
	// text_pt is the rider the master reads.
	// ranged_attack uses Applies too: RangedAttackSpell or RangedAttackWeapon. So does
	// ambush (AmbushUnactedAdvantage or AmbushSurprisedCritical) and
	// extra_damage (ExtraDamageLargerTarget or ExtraDamageAnyTarget), with Value the damage
	// added to a weapon hit (an Int formula) and Resource, when set, the uses it spends.
	Applies string `json:"applies,omitempty"`

	// ranged_attack (rangedattack.go): what the character's attacks at a distance get,
	// for a table feat (the SRD has none). IgnoreCover takes the AC bonus of half and
	// three-quarters cover away from the targets of those attacks; RangeMultiplier
	// ("spell" only, 2 or 3) multiplies the range of a spell that needs an attack roll;
	// NoLongRangeDisadvantage ("weapon" only) drops the disadvantage of attacking
	// beyond the normal range.
	IgnoreCover             bool `json:"ignore_cover,omitempty"`
	RangeMultiplier         int  `json:"range_multiplier,omitempty"`
	NoLongRangeDisadvantage bool `json:"no_long_range_disadvantage,omitempty"`

	// resistance: resistance (Kind "resist", the default), immunity ("immune") or
	// vulnerability ("vulnerable") to the damage types it names: every damage type but
	// Except (a "damage-type:" key), or the DamageTypes ("damage-type:" keys, or one of
	// the words DamageTypeAll and DamageTypeNonmagicalBPS). Except and DamageTypes
	// exclude each other and one of them is required. While is ResistanceAlways ("") or
	// ResistanceWhileRage ("rage"), or the name of a state of the entry. It takes no When, because While is its condition.
	// Content.Resistances lists it, so play applies it with the other resistances (the
	// same type from two sources counts once).
	//
	// While is the state gate (gate.go): the other types in gatedEffectTypes
	// (roll_mode, extra_damage, weapon_rule, pseudo_weapon) read it too, and hold
	// only while the named state of their entry is on.
	Except      string   `json:"except,omitempty"`
	While       string   `json:"while,omitempty"`
	DamageTypes []string `json:"damage_types,omitempty"`
	Kind        string   `json:"kind,omitempty"`

	// condition_immunity: the creature takes none of the Conditions ("condition:" keys of
	// the 15 conditions). roll_mode may also name an AgainstCondition: its advantage or
	// disadvantage holds on the saving throws against being that condition.
	Conditions       []string `json:"conditions,omitempty"`
	AgainstCondition string   `json:"against_condition,omitempty"`

	// extra_damage (extradamage.go): the dice it rolls, Dice an Int formula for how many
	// and DiceSides (4, 6, 8, 10 or 12) their sides, beside the fixed Value; DamageType a
	// "damage-type:" key (empty is the weapon's own); Weapon one of ExtraDamageAnyWeapon,
	// ExtraDamageMeleeWeapon and ExtraDamageRangedWeapon; Frequency one of
	// ExtraDamageOncePerTurn and ExtraDamageEachHit; NoCritDoubling keeps a critical hit
	// from doubling the dice.
	Dice           string `json:"dice,omitempty"`
	DiceSides      int    `json:"dice_sides,omitempty"`
	DamageType     string `json:"damage_type,omitempty"`
	Weapon         string `json:"weapon,omitempty"`
	Frequency      string `json:"frequency,omitempty"`
	NoCritDoubling bool   `json:"no_crit_doubling,omitempty"`

	// spell_modifier (spellmodifier.go): AppliesTo which spells ("damage_spell", "cantrip", an SRD
	// school key or damage type key) and SpellChange what it does (SpellChange*). From, Value (the
	// fixed damage), DamageType and Count are the change's own.
	AppliesTo   string `json:"applies_to,omitempty"`
	SpellChange string `json:"spell_change,omitempty"`

	// state (state.go): a temporary state a feature switches on for a while (a stance, a
	// form). StateName names it within the entry (a-z, 0-9 and _, like a resource);
	// DurationRounds is an Int formula for how many rounds it lasts (empty or 0: until
	// it ends); EndsOn lists the ways it ends besides its duration (StateEnds*);
	// Resource, the one it spends on start, and Economy, what starting it costs,
	// are the fields grant_action has.
	StateName      string   `json:"state_name,omitempty"`
	DurationRounds string   `json:"duration_rounds,omitempty"`
	EndsOn         []string `json:"ends_on,omitempty"`

	// weapon_rule and pseudo_weapon (weaponrule.go). weapon_rule changes the weapon
	// lines Weapon chooses (Weapon* filters; WeaponName for WeaponNamed): AttackAbility
	// and DamageAbility offer another ability (the best of the options applies),
	// AttackBonus and DamageBonus add, ReachFt widens a melee reach. pseudo_weapon gives
	// an attack line of its own: AttackName, Weapon (WeaponMelee or WeaponRanged),
	// DamageDice (plain dice, "1d6"), DamageType (a "damage-type:" key), Ability, ReachFt
	// for a melee one and RangeFt (above) for a ranged one.
	WeaponName    string `json:"weapon_name,omitempty"`
	AttackAbility string `json:"attack_ability,omitempty"`
	DamageAbility string `json:"damage_ability,omitempty"`
	AttackBonus   int    `json:"attack_bonus,omitempty"`
	DamageBonus   int    `json:"damage_bonus,omitempty"`
	ReachFt       int    `json:"reach_ft,omitempty"`
	AttackName    string `json:"attack_name,omitempty"`
	DamageDice    string `json:"damage_dice,omitempty"`

	// proficiency, level "expertise" on a skill: OnlyProficient doubles the skill
	// only where the character has its proficiency from another source when the
	// sheet is worked out; the effect never gives the proficiency itself.
	OnlyProficient bool `json:"only_proficient,omitempty"`

	// wild_shape: the beasts a druid may turn into (wildshape.go). MaxCR is
	// the highest challenge rating, such as "1/4", and NoFly and NoSwim leave
	// out the beasts with a fly or a swim speed. The druid's features of
	// levels 2, 4 and 8 each carry one; the most permissive one the
	// character has wins.
	// MaxCRFrom is an Int formula for the highest whole challenge rating, from the
	// character's level (such as "floor(level() / 3)"); it replaces MaxCR. FormCost is
	// an Int formula for the uses of the Wild Shape resource one change of form costs
	// (empty is 1), and MagicalAttacks makes the beast form's attacks count as magical.
	// The character takes the highest rating, the lowest cost, and magical attacks from
	// any effect.
	MaxCR          string `json:"max_cr,omitempty"`
	MaxCRFrom      string `json:"max_cr_from,omitempty"`
	FormCost       string `json:"form_cost,omitempty"`
	MagicalAttacks bool   `json:"magical_attacks,omitempty"`
	NoFly          bool   `json:"no_fly,omitempty"`
	NoSwim         bool   `json:"no_swim,omitempty"`

	// grant_action and spell slots (actionslots.go): GivesSlot is an Int formula for the level
	// (1 to 9) of the slot the action creates from the uses it spends, and SpendsSlot makes
	// the action spend a spell slot of the level the player picks and heal by it.
	// slot_recovery: Resource is the resource each recovery spends, RecoverMaxLevel an Int
	// formula for the highest level of a slot it takes back and RecoverLevels one for the
	// combined level of the slots.
	GivesSlot       string `json:"gives_slot,omitempty"`
	SpendsSlot      bool   `json:"spends_slot,omitempty"`
	RecoverMaxLevel string `json:"recover_max_level,omitempty"`
	RecoverLevels   string `json:"recover_levels,omitempty"`

	// song_of_rest: the die of the bard's Song of Rest (6, 8, 10 or 12 sides),
	// which the character's short rest adds to the hit points the party regains
	// with hit dice (songofrest.go). Each tier of the feature carries one; the
	// largest the character has counts.
	Sides int `json:"sides,omitempty"`

	// replaces: the feature this one takes over from, a lower tier of the same
	// feature (Channel Divinity twice between rests replaces once). The
	// sheet lists only the newest tier; the older one's effects still apply.
	Replaces string `json:"replaces,omitempty"`

	// grant_action (actions.go): Cost is how many uses of Resource each use spends (an Int
	// formula; empty is 1) and CostMax, when set, the most the player may spend on one use.
	// ValuePerPoint is what each point above Cost adds to the result (dice, or a flat
	// formula); Heal is dice, HealPercentMax (on_event only) a share of the maximum, TempHP a
	// formula. The same Heal, TempHP and (for the ability modifier) Ability serve on_event.
	Cost          string `json:"cost,omitempty"`
	CostMax       string `json:"cost_max,omitempty"`
	ValuePerPoint string `json:"value_per_point,omitempty"`
	Heal          string `json:"heal,omitempty"`
	TempHP        string `json:"temp_hp,omitempty"`

	// on_event (hooks.go): Event is one of the Event* constants, the hook runs for the
	// owner alone. Restore is the resource whose uses it gives back (Value, an Int
	// formula, how many; empty is 1) and HealPercentMax a share of the maximum hit
	// points healed instead of Heal.
	Event          string `json:"event,omitempty"`
	Restore        string `json:"restore,omitempty"`
	HealPercentMax int    `json:"heal_percent_max,omitempty"`

	// handler: the name of a Go function for what data cannot say, from
	// handlers.
	Handler string `json:"handler,omitempty"`

	// granted_spell (grantedspells.go): a spell the entry gives the character, apart from
	// the class's lists. Spell is its key (SRD, or a table spell of the same table);
	// FromLevel an Int formula, the level from which it is granted (1 when empty):
	// the class level for a class or subclass feature, the character level for
	// anything else. Uses is GrantedAtWill or the name of a resource of the entry that
	// every cast spends one of. AlsoWithSlot lets the character cast it with a spell
	// slot too. Ability is the casting ability of the grant (GrantedAbilities, or
	// GrantedAbilityChoice: the player picks one). RitualOnly limits the grant to the
	// ritual way; CountsAsKnown puts the spell among the known ones too.
	Spell         string `json:"spell,omitempty"`
	FromLevel     string `json:"from_level,omitempty"`
	Uses          string `json:"uses,omitempty"`
	AlsoWithSlot  bool   `json:"also_with_slot,omitempty"`
	RitualOnly    bool   `json:"ritual_only,omitempty"`
	CountsAsKnown bool   `json:"counts_as_known,omitempty"`

	// Spells are spells the effect grants, which then count as on the
	// character's list (Infernal Legacy's thaumaturgy).
	Spells []string `json:"spells,omitempty"`

	// TextPT is an optional short Portuguese sentence for the sheet, in
	// our own words. A note with a text becomes a Hint. In the text,
	// {value} is replaced by Value's result with its sign ("+8") and {n}
	// without it ("13"); a note may carry a Value just for that (a save
	// DC, a number of uses).
	TextPT string `json:"text_pt,omitempty"`

	// Compiled formulas, filled at load.
	value, when, preparedMax, max, rechargeIf, dice, durationRounds, fromLevel, cost, costMax, tempHP, perPointFlat, maxCRFrom, formCost, givesSlot, recoverMaxLevel, recoverLevels *formula.Program

	// What the overlay resolves for the state gate (gate.go), on the copy of the effect
	// that is compiled: entry is the table entry the effect belongs to; stateKey is,
	// for a state, its key on the sheet; gateKey is the key of the state While names;
	// whenStates maps the names the `when` formula reads with active() to those keys.
	entry, stateKey, gateKey string
	whenStates               map[string]string
}

// The closed sets of the effect schema.
var (
	effectTypes = []string{
		"modifier", "proficiency", "roll_mode", "sense", "spellcasting",
		"resource", "choice", "grant_action", "extra_attack", "note", "handler", "wild_shape", "beast_spells", "replaces", "slot_recovery",
		"ability_increase", "superiority_die", "song_of_rest", "ranged_attack", "extra_damage", "ambush", "resistance",
		"state", "weapon_rule", "pseudo_weapon", "breathes_water", "condition_immunity", "granted_spell", "on_event", "spell_modifier",
	}
	modifierTargets = []string{
		"ac.base", "ac", "hp.max", "speed.walk", "speed.fly", "speed.swim", "speed.climb", "speed.burrow", "initiative",
		"attack.weapon.melee", "attack.weapon.ranged",
		"damage.weapon.melee", "damage.weapon.ranged",
	}
	modifierModes     = []string{"add", "max", "set"}
	proficiencyLevels = []string{"half", "full", "expertise"}
	rollModes         = []string{"advantage", "disadvantage"}
	senses            = []string{"darkvision", "blindsight", "tremorsense", "truesight"}
	progressions      = []string{"full", "half", "third", "pact"}
	recharges         = []string{RechargeShortRest, RechargeLongRest, RechargeDawn, RechargeNone, RechargeInitiative, RechargeRestoreOneShort, RechargeRestoreOneLong}
	choiceKinds       = []string{
		"skill", "expertise", "cantrip", "spell", "subclass", "feature",
		"language", "tool", "ability_score_improvement", "feat",
	}
	economies = []string{"action", "bonus_action", "reaction", "free", "movement"}
	// maneuverApplies is where a superiority die goes: the damage of a weapon attack
	// after a hit, an attack roll, the Athletics check of a grapple, or a reduction
	// of the damage of a melee attack that hits the character (a reaction).
	maneuverApplies = []string{ManeuverDamage, ManeuverAttack, ManeuverGrapple, ManeuverReduceMelee, ManeuverGoad, ManeuverCheck, ManeuverSave, ManeuverInitiative}
	// handlers are the Go functions an effect may name. Content (and, later,
	// the table's homebrew) can only point at these; it never brings code.
	handlers = []string{
		"monk.martial_arts",      // attacks.go: monk weapons use STR or DEX
		"wizard.arcane_recovery", // recover slots on a short rest
		"wizard.sculpt_spells",   // protect allies from evocations
		"monk.open_hand",         // play: the Open Hand technique rider on a Flurry of Blows hit
		"monk.stunning_strike",   // play: the Stunning Strike rider on a melee hit
	}
)

// The limits of the effect schema.
const (
	// minExtraAttacks and maxExtraAttacks bound the attacks an extra_attack makes.
	minExtraAttacks = 2
	maxExtraAttacks = 4
)

// effectCompiler checks and compiles one effect.
type effectCompiler struct {
	c *content
	// key is the feature, trait or background the effect belongs to.
	key string
	e   *Effect
}

func (x effectCompiler) fail(format string, args ...any) error {
	return fmt.Errorf("effect %s (%s): %s", x.key, x.e.Type, fmt.Sprintf(format, args...))
}

// compile compiles a formula of the effect; an empty one is no program.
func (x effectCompiler) compile(src string, kind formula.Kind) (*formula.Program, error) {
	if src == "" {
		return nil, nil
	}
	p, err := x.c.compileFormula(src, kind)
	if err != nil {
		return nil, x.fail("formula %q: %v", src, err)
	}
	return p, nil
}

// takesOnly says the effect sets nothing but the fields strip takes off a copy
// of it (compiled formulas included).
func (x effectCompiler) takesOnly(strip func(other *Effect)) bool {
	other := *x.e
	// What the overlay resolves for the state gate (gate.go) is not a field the author set.
	other.entry, other.stateKey, other.gateKey, other.whenStates = "", "", "", nil
	strip(&other)
	return reflect.DeepEqual(other, Effect{})
}

// effectCheckers check what each effect type needs, after the checks every
// effect goes through.
var effectCheckers = map[string]func(x effectCompiler) error{
	"ranged_attack":      effectCompiler.rangedAttack,
	"resistance":         effectCompiler.resistance,
	"condition_immunity": effectCompiler.conditionImmunity,
	"state":              effectCompiler.state,
	"weapon_rule":        effectCompiler.weaponRule,
	"pseudo_weapon":      effectCompiler.pseudoWeapon,
	"breathes_water":     effectCompiler.breathesWater,
	"granted_spell":      effectCompiler.grantedSpell,
	"replaces":           effectCompiler.replaces,
	"modifier":           effectCompiler.modifier,
	"proficiency":        effectCompiler.proficiency,
	"roll_mode":          effectCompiler.rollMode,
	"sense":              effectCompiler.sense,
	"spellcasting":       effectCompiler.spellcasting,
	"resource":           effectCompiler.resource,
	"choice":             effectCompiler.choice,
	"ability_increase":   effectCompiler.abilityIncrease,
	"grant_action":       effectCompiler.grantAction,
	"on_event":           effectCompiler.onEvent,
	"superiority_die":    effectCompiler.superiorityDie,
	"extra_damage":       effectCompiler.extraDamage,
	"spell_modifier":     effectCompiler.spellModifier,
	"ambush":             effectCompiler.ambush,
	"extra_attack":       effectCompiler.extraAttack,
	"wild_shape":         effectCompiler.wildShape,
	"slot_recovery":      effectCompiler.slotRecovery,
	"song_of_rest":       effectCompiler.songOfRest,
	"beast_spells":       effectCompiler.beastSpells,
	"handler":            effectCompiler.handler,
	"note":               effectCompiler.note,
}

// compileEffect checks one effect against the schema and compiles its
// formulas. key is the feature, trait or background it belongs to.
func (c *content) compileEffect(key string, e *Effect) error {
	x := effectCompiler{c: c, key: key, e: e}
	if !slices.Contains(effectTypes, e.Type) {
		return x.fail("unknown effect type")
	}
	var err error
	if e.when, err = x.compileWhen(e.When); err != nil {
		return err
	}
	for _, tag := range e.Tags {
		if tag == "" || strings.TrimSpace(tag) != tag {
			return x.fail("empty tag")
		}
	}
	if err := x.strayFields(); err != nil {
		return err
	}
	if err := x.speedwalkOnlyForOtherSpeeds(); err != nil {
		return err
	}
	if check, ok := effectCheckers[e.Type]; ok {
		if err := check(x); err != nil {
			return err
		}
	}
	for _, s := range e.Spells {
		if _, ok := c.spells[s]; !ok {
			return x.fail("unknown spell %q", s)
		}
	}
	return nil
}

// compileWhen compiles the condition of an effect: the one formula that may read a
// state with active().
func (x effectCompiler) compileWhen(src string) (*formula.Program, error) {
	if src == "" {
		return nil, nil
	}
	p, err := x.c.compileGated(src)
	if err != nil {
		return nil, x.fail("formula %q: %v", src, err)
	}
	return p, nil
}

// strayFields refuses the fields of one type on an effect of another.
func (x effectCompiler) strayFields() error {
	e := x.e
	if err := x.strayShapeAndSlots(); err != nil {
		return err
	}
	if e.Type != "song_of_rest" && e.Sides != 0 {
		return x.fail("sides belongs to song_of_rest")
	}
	if e.Type != "replaces" && e.Replaces != "" {
		return x.fail("replaces belongs to the replaces effect")
	}
	if err := x.strayOwnFields(); err != nil {
		return err
	}
	if err := x.strayStateAndWeapon(); err != nil {
		return err
	}
	if err := x.strayExtraDamage(); err != nil {
		return err
	}
	if err := x.strayConditions(); err != nil {
		return err
	}
	if err := x.strayCosts(); err != nil {
		return err
	}
	return x.straySpellAndRangeFields()
}

// strayShapeAndSlots refuses the fields of wild_shape, of the slots of grant_action
// and of slot_recovery on an effect of another type.
func (x effectCompiler) strayShapeAndSlots() error {
	e := x.e
	if e.Type != "wild_shape" && (e.MaxCR != "" || e.MaxCRFrom != "" || e.FormCost != "" || e.MagicalAttacks || e.NoFly || e.NoSwim) {
		return x.fail("max_cr, max_cr_from, form_cost, magical_attacks, no_fly and no_swim belong to wild_shape")
	}
	if e.Type != "grant_action" && (e.GivesSlot != "" || e.SpendsSlot) {
		return x.fail("gives_slot and spends_slot belong to grant_action")
	}
	if e.Type != "slot_recovery" && (e.RecoverMaxLevel != "" || e.RecoverLevels != "") {
		return x.fail("recover_max_level and recover_levels belong to slot_recovery")
	}
	return nil
}

// straySpellAndRangeFields refuses the fields of granted_spell and of
// ranged_attack on an effect of another type.
func (x effectCompiler) straySpellAndRangeFields() error {
	e := x.e
	if e.Type != "granted_spell" && (e.Spell != "" || e.FromLevel != "" || e.Uses != "" || e.AlsoWithSlot || e.RitualOnly || e.CountsAsKnown) {
		return x.fail("spell, from_level, uses, also_with_slot, ritual_only and counts_as_known belong to granted_spell")
	}
	if e.Type != "ranged_attack" && (e.IgnoreCover || e.RangeMultiplier != 0 || e.NoLongRangeDisadvantage) {
		return x.fail("ignore_cover, range_multiplier and no_long_range_disadvantage belong to ranged_attack")
	}
	return nil
}

// strayExtraDamage refuses the fields of extra_damage and spell_modifier on an effect of another type.
func (x effectCompiler) strayExtraDamage() error {
	e := x.e
	if e.Type != "extra_damage" && (e.Dice != "" || e.DiceSides != 0 || e.Frequency != "" || e.NoCritDoubling) {
		return x.fail("dice, dice_sides, frequency and no_crit_doubling belong to extra_damage")
	}
	if !slices.Contains(damageTypeEffects, e.Type) && e.DamageType != "" {
		return x.fail("damage_type belongs to extra_damage, spell_modifier and pseudo_weapon")
	}
	if !slices.Contains(weaponEffects, e.Type) && e.Weapon != "" {
		return x.fail("weapon belongs to extra_damage, weapon_rule and pseudo_weapon")
	}
	if e.Type != "spell_modifier" && (e.AppliesTo != "" || e.SpellChange != "") {
		return x.fail("applies_to and spell_change belong to spell_modifier")
	}
	return nil
}

// strayConditions refuses conditions on an effect that is no condition_immunity and against_condition on one that
// is no roll_mode.
func (x effectCompiler) strayConditions() error {
	switch {
	case x.e.Type != "condition_immunity" && len(x.e.Conditions) > 0:
		return x.fail("conditions belong to condition_immunity")
	case x.e.Type != "roll_mode" && x.e.AgainstCondition != "":
		return x.fail("against_condition belongs to roll_mode")
	}
	return nil
}

// strayOwnFields refuses the fields of resistance on an effect of another type (while
// is also read by the effects of gatedEffectTypes), and only_proficient on one that is
// not a proficiency.
func (x effectCompiler) strayOwnFields() error {
	if x.e.Type != "resistance" && (x.e.Except != "" || len(x.e.DamageTypes) > 0 || x.e.Kind != "") {
		return x.fail("except, damage_types and kind belong to resistance")
	}
	if x.e.While != "" && !slices.Contains(gatedEffectTypes, x.e.Type) {
		return x.fail("while belongs to the effects that hold only while a state is on")
	}
	if x.e.Type != "proficiency" && x.e.OnlyProficient {
		return x.fail("only_proficient belongs to proficiency")
	}
	return nil
}

func (x effectCompiler) rangedAttack() error {
	if _, msg := checkRangedAttack(x.e); msg != "" {
		return x.fail("%s", msg)
	}
	return nil
}

// resistance checks what a resistance covers (the damage types it names, or every
// type but the one it leaves out), its kind and when it holds.
func (x effectCompiler) resistance() error {
	e := x.e
	if e.Except != "" && len(e.DamageTypes) > 0 {
		return x.fail("except and damage_types exclude each other")
	}
	if msg := x.c.checkDamageTypes(e); msg != "" {
		return x.fail("%s", msg)
	}
	if !slices.Contains(ResistanceKinds, e.Kind) {
		return x.fail("kind %q is not resist, immune or vulnerable", e.Kind)
	}
	if e.While != ResistanceAlways && e.While != ResistanceWhileRage && !formula.ValidStateName(e.While) {
		return x.fail("while must be empty, %q or the name of a state", ResistanceWhileRage)
	}
	if e.When != "" {
		return x.fail("a resistance has no when: while is its condition")
	}
	if len(e.Tags) > 0 {
		return x.fail("a resistance has no tags: while is its condition")
	}
	return nil
}

// conditionImmunity checks the conditions the character takes none of.
func (x effectCompiler) conditionImmunity() error {
	e := x.e
	if len(e.Conditions) == 0 {
		return x.fail("conditions are required")
	}
	for i, k := range e.Conditions {
		if !x.c.isCondition(k) {
			return x.fail("%q is not a condition", k)
		}
		if slices.Contains(e.Conditions[:i], k) {
			return x.fail("the condition %q is listed twice in conditions", k)
		}
	}
	return nil
}

func (x effectCompiler) replaces() error {
	if !x.takesOnly(func(o *Effect) { o.Type, o.Replaces = "", "" }) {
		return x.fail("replaces takes the feature key only")
	}
	if _, ok := x.c.features[x.e.Replaces]; !ok {
		return x.fail("replaces needs the key of a feature")
	}
	if x.e.Replaces == x.key {
		return x.fail("a feature cannot replace itself")
	}
	return nil
}

func (x effectCompiler) modifier() error {
	e := x.e
	if !x.c.validModifierTarget(e.Target) {
		return x.fail("unknown target %q", e.Target)
	}
	if !slices.Contains(modifierModes, e.Mode) {
		return x.fail("unknown mode %q", e.Mode)
	}
	if e.Value == "" {
		return x.fail("value is required")
	}
	if !scoreCapFits(e) {
		return x.fail("a score modifier is an add with a cap above %d, and nothing else has a cap", MaxNormalScore)
	}
	var err error
	e.value, err = x.compile(e.Value, formula.Int)
	return err
}

// scoreCapFits: a modifier of a score is an add with a cap between the normal
// maximum and the highest score, and no other modifier has a cap.
func scoreCapFits(e *Effect) bool {
	_, isScore := strings.CutPrefix(e.Target, "score.")
	if isScore != (e.Cap != 0) {
		return false
	}
	return !isScore || (e.Mode == "add" && e.Cap > MaxNormalScore && e.Cap <= MaxScore)
}

func (x effectCompiler) proficiency() error {
	e := x.e
	if !x.c.validProficiencyTarget(e.Proficiency) {
		return x.fail("unknown proficiency %q", e.Proficiency)
	}
	if e.Level != "" && !slices.Contains(proficiencyLevels, e.Level) {
		return x.fail("unknown level %q", e.Level)
	}
	if msg := checkOnlyProficient(e); msg != "" {
		return x.fail("%s", msg)
	}
	return nil
}

// checkOnlyProficient says what is wrong with the only_proficient switch of a
// proficiency effect, or "": it doubles skills, so it goes with the level
// "expertise" and a skill (or "skill:*").
func checkOnlyProficient(e *Effect) string {
	if !e.OnlyProficient {
		return ""
	}
	if e.Level != "expertise" || !strings.HasPrefix(e.Proficiency, "skill:") {
		return "only_proficient goes with the level expertise on a skill"
	}
	return ""
}

func (x effectCompiler) rollMode() error {
	e := x.e
	if !slices.Contains(rollModes, e.Roll) {
		return x.fail("unknown roll %q", e.Roll)
	}
	if len(e.Targets) == 0 {
		return x.fail("targets are required")
	}
	for _, t := range e.Targets {
		if !x.c.validRollTarget(t) {
			return x.fail("unknown target %q", t)
		}
	}
	if e.AgainstCondition != "" {
		return x.againstCondition()
	}
	return nil
}

// againstCondition checks a roll_mode that holds on the saving throws against one condition.
func (x effectCompiler) againstCondition() error {
	e := x.e
	if !x.c.isCondition(e.AgainstCondition) {
		return x.fail("against_condition %q is not a condition", e.AgainstCondition)
	}
	for _, t := range e.Targets {
		if !strings.HasPrefix(t, "save.") {
			return x.fail("against_condition holds on saving throws: target %q is not one", t)
		}
	}
	if len(e.Tags) > 0 {
		return x.fail("against_condition takes no tags: the condition is the situation")
	}
	return nil
}

func (x effectCompiler) sense() error {
	if !slices.Contains(senses, x.e.Sense) || x.e.RangeFt <= 0 {
		return x.fail("sense needs a known sense and a range")
	}
	return nil
}

func (x effectCompiler) spellcasting() error {
	e := x.e
	if _, ok := abilityIndex[Ability(e.Ability)]; !ok {
		return x.fail("unknown ability %q", e.Ability)
	}
	if !slices.Contains(progressions, e.Progression) {
		return x.fail("unknown progression %q", e.Progression)
	}
	if e.Prepares != (e.PreparedMax != "") || (e.Spellbook && !e.Prepares) {
		return x.fail("prepared_max is required exactly when prepares is true, and a spellbook prepares")
	}
	var err error
	e.preparedMax, err = x.compile(e.PreparedMax, formula.Int)
	return err
}

func (x effectCompiler) resource() error {
	e := x.e
	if e.Resource == "" || e.Max == "" || !slices.Contains(recharges, e.Recharge) {
		return x.fail("resource needs a name, a max and a known recharge")
	}
	var err error
	if e.max, err = x.compile(e.Max, formula.Int); err != nil {
		return err
	}
	if (e.RechargeIf == "") != (e.RechargeThen == "") || (e.RechargeThen != "" && !slices.Contains(recharges, e.RechargeThen)) {
		return x.fail("recharge_if and recharge_then go together, and recharge_then is a known recharge")
	}
	e.rechargeIf, err = x.compile(e.RechargeIf, formula.Bool)
	return err
}

func (x effectCompiler) choice() error {
	e := x.e
	if !slices.Contains(choiceKinds, e.Choice) || e.Count < 0 {
		return x.fail("unknown choice %q", e.Choice)
	}
	for _, k := range e.From {
		if !x.c.exists(k) {
			return x.fail("unknown key %q in from", k)
		}
	}
	return nil
}

// abilityIncrease: the player picks Count different abilities of From and adds
// Value to each; the sheet keeps the result as manual ability bonuses, so Derive
// reads nothing here. Only a feat has it (the table's menu).
func (x effectCompiler) abilityIncrease() error {
	if attr, msg := checkAbilityIncrease(x.e); attr != "" {
		return x.fail("%s", msg)
	}
	return nil
}

func (x effectCompiler) superiorityDie() error {
	e := x.e
	if e.Resource == "" {
		return x.fail("a superiority die names the resource it spends")
	}
	if !slices.Contains(maneuverApplies, e.Applies) {
		return x.fail("unknown applies %q", e.Applies)
	}
	if e.Value == "" {
		return x.fail("value is required: the sides of the die")
	}
	var err error
	if e.value, err = x.compile(e.Value, formula.Int); err != nil {
		return err
	}
	if err := x.strayTargets(); err != nil {
		return err
	}
	return x.maneuverRider()
}

// maneuverRider checks the ability and the economy a superiority die takes,
// which depend on where the die goes.
func (x effectCompiler) maneuverRider() error {
	e := x.e
	switch e.Applies {
	case ManeuverGoad:
		if _, ok := abilityIndex[Ability(e.Ability)]; !ok {
			return x.fail("goad needs the ability whose modifier is in the DC")
		}
		if e.Economy != "" {
			return x.fail("goad costs no action")
		}
	case ManeuverReduceMelee:
		if _, ok := abilityIndex[Ability(e.Ability)]; !ok {
			return x.fail("reduce_melee_damage needs the ability whose modifier is added")
		}
		if e.Economy != EconomyReaction {
			return x.fail("reduce_melee_damage costs a reaction")
		}
	case ManeuverGrapple:
		if e.Economy != "" && e.Economy != EconomyBonusAction {
			return x.fail("a grapple maneuver costs a bonus action or nothing")
		}
	case ManeuverCheck, ManeuverSave, ManeuverInitiative:
		return x.rollDie()
	default:
		if e.Ability != "" || e.Economy != "" {
			return x.fail("%s takes no ability and no economy", e.Applies)
		}
	}
	return nil
}

// strayTargets refuses the targets of a superiority die that is not on ability checks.
func (x effectCompiler) strayTargets() error {
	if x.e.Applies != ManeuverCheck && len(x.e.Targets) > 0 {
		return x.fail("only a die added to ability checks lists targets")
	}
	return nil
}

// rollDie checks a superiority die that goes on a roll of the character.
func (x effectCompiler) rollDie() error {
	if _, msg := rollDieShape(x.e); msg != "" {
		return x.fail("%s", msg)
	}
	if t, ok := x.c.rollDieTargetsOK(x.e.Targets); !ok {
		return x.fail("unknown target %q: a die on ability checks lists skills and abilities", t)
	}
	return nil
}

func (x effectCompiler) extraAttack() error {
	if x.e.Count < minExtraAttacks || x.e.Count > maxExtraAttacks {
		return x.fail("extra_attack needs a count of 2 to 4 attacks")
	}
	return nil
}

func (x effectCompiler) wildShape() error {
	if !x.takesOnly(func(o *Effect) {
		o.Type, o.MaxCR, o.MaxCRFrom, o.FormCost, o.MagicalAttacks, o.NoFly, o.NoSwim = "", "", "", "", false, false, false
		o.maxCRFrom, o.formCost = nil, nil
	}) {
		return x.fail("wild_shape takes max_cr, max_cr_from, form_cost, magical_attacks, no_fly and no_swim only")
	}
	e := x.e
	if e.MaxCR != "" && e.MaxCRFrom != "" {
		return x.fail("wild_shape takes max_cr or max_cr_from, not both")
	}
	if e.MaxCR == "" && e.MaxCRFrom == "" && e.FormCost == "" && !e.MagicalAttacks {
		return x.fail("wild_shape needs a max_cr, a max_cr_from, a form_cost or magical_attacks")
	}
	if e.MaxCR != "" {
		if _, ok := crEighths(e.MaxCR); !ok {
			return x.fail("wild_shape needs a max_cr that is a challenge rating, such as \"1/4\"")
		}
	}
	if e.MaxCR == "" && e.MaxCRFrom == "" && (e.NoFly || e.NoSwim) {
		return x.fail("no_fly and no_swim go with a max_cr or a max_cr_from")
	}
	var err error
	if e.maxCRFrom, err = x.compile(e.MaxCRFrom, formula.Int); err != nil {
		return err
	}
	e.formCost, err = x.compile(e.FormCost, formula.Int)
	return err
}

// otherSpeedTargets are the modifier targets whose formulas may read speedwalk().
var otherSpeedTargets = []string{"speed.fly", "speed.swim", "speed.climb", "speed.burrow"}

// speedwalkOnlyForOtherSpeeds keeps speedwalk() to the other speeds' modifiers:
// the walking speed is worked out first, so anything else would read 0.
func (x effectCompiler) speedwalkOnlyForOtherSpeeds() error {
	e := x.e
	reads := strings.Contains(e.Value, "speedwalk") || strings.Contains(e.When, "speedwalk")
	if reads && (e.Type != "modifier" || !slices.Contains(otherSpeedTargets, e.Target)) {
		return x.fail("speedwalk() belongs to the modifiers of the fly, swim, climb and burrow speeds")
	}
	return nil
}

// breathesWater checks the effect that lets the character breathe underwater.
func (x effectCompiler) breathesWater() error {
	if !x.takesOnly(func(o *Effect) { o.Type, o.When, o.TextPT = "", "", ""; o.when = nil }) {
		return x.fail("breathes_water takes no fields but when and text_pt")
	}
	return nil
}

func (x effectCompiler) songOfRest() error {
	if !x.takesOnly(func(o *Effect) { o.Type, o.Sides = "", 0 }) {
		return x.fail("song_of_rest takes sides only")
	}
	if !slices.Contains(songOfRestDice, x.e.Sides) {
		return x.fail("song_of_rest needs sides of 6, 8, 10 or 12")
	}
	return nil
}

func (x effectCompiler) beastSpells() error {
	if !x.takesOnly(func(o *Effect) { o.Type = "" }) {
		return x.fail("beast_spells takes no fields")
	}
	return nil
}

func (x effectCompiler) handler() error {
	if !slices.Contains(handlers, x.e.Handler) {
		return x.fail("unknown handler %q", x.e.Handler)
	}
	return nil
}

func (x effectCompiler) note() error {
	e := x.e
	var err error
	if e.value, err = x.compile(e.Value, formula.Int); err != nil {
		return err
	}
	if e.Value != "" && e.TextPT == "" {
		return x.fail("a value needs a text_pt to show it in")
	}
	return nil
}

// programKey identifies a compiled formula: its text and its kind.
type programKey struct {
	src   string
	kind  formula.Kind
	gated bool
}

// compileFormula compiles one formula. While a table layer is being added
// (With), programs memoizes by text: a program is immutable, and a table repeats
// the same few formulas ("prof()", "1") in many features, so each is compiled
// once. The memo is dropped when With is done; the SRD load does not use it.
func (c *content) compileFormula(src string, kind formula.Kind) (*formula.Program, error) {
	return c.compileWith(c.compiler, src, kind, false)
}

// compileGated compiles the condition of an effect, which may read a state with
// active() (see formula.Compiler.WithStates).
func (c *content) compileGated(src string) (*formula.Program, error) {
	return c.compileWith(c.compiler.WithStates(), src, formula.Bool, true)
}

// compileWith is compileFormula with the compiler chosen; gated tells the memo
// which of the two programs a text means.
func (c *content) compileWith(compiler *formula.Compiler, src string, kind formula.Kind, gated bool) (*formula.Program, error) {
	if c.programs == nil {
		return compiler.Compile(src, kind)
	}
	key := programKey{src, kind, gated}
	if p, ok := c.programs[key]; ok {
		return p, nil
	}
	p, err := compiler.Compile(src, kind)
	if err == nil {
		c.programs[key] = p
	}
	return p, err
}

func (c *content) validModifierTarget(t string) bool {
	if slices.Contains(modifierTargets, t) {
		return true
	}
	if skill, ok := strings.CutPrefix(t, "skill."); ok {
		_, found := c.skills["skill:"+skill]
		return found
	}
	if ab, ok := strings.CutPrefix(t, "save."); ok {
		_, found := abilityIndex[Ability(ab)] // save.all adds to every saving throw
		return found || ab == "all"
	}
	if ab, ok := strings.CutPrefix(t, "score."); ok {
		_, found := abilityIndex[Ability(ab)]
		return found
	}
	if spell, ok := strings.CutPrefix(t, "damage.spell."); ok {
		_, found := c.spells["spell:"+spell]
		return found
	}
	return false
}

func (c *content) validProficiencyTarget(t string) bool {
	switch {
	case t == "skill:*", t == "initiative":
		return true
	case strings.HasPrefix(t, "skill:"):
		_, ok := c.skills[t]
		return ok
	case strings.HasPrefix(t, "save."):
		_, ok := abilityIndex[Ability(strings.TrimPrefix(t, "save."))]
		return ok
	case strings.HasPrefix(t, "proficiency:"):
		_, ok := c.proficiencies[t]
		return ok
	}
	return false
}

func (c *content) validRollTarget(t string) bool {
	switch t {
	case "attack", "death_save", "initiative", "check.all", "save.all":
		return true
	}
	if ab, ok := strings.CutPrefix(t, "check."); ok {
		_, found := abilityIndex[Ability(ab)]
		return found
	}
	return c.validModifierTarget(t) && (strings.HasPrefix(t, "skill.") || strings.HasPrefix(t, "save."))
}

// The effect types that read the fields they share: weapon (extra_damage's filter, the
// weapon lines a weapon_rule chooses, the kind of a pseudo_weapon) and damage_type.
var (
	weaponEffects     = []string{"extra_damage", "weapon_rule", "pseudo_weapon"}
	damageTypeEffects = []string{"extra_damage", "spell_modifier", "pseudo_weapon"}
)
