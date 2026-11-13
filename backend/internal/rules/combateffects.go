package rules

import (
	"errors"
	"fmt"
	"io/fs"
	"slices"
	"strings"

	"github.com/PuraFome/meuRPG/backend/internal/rules/srd51"
)

// Effects that last in a combat (RN-22, SRD 5.1: "Conditions", "Duration",
// "Concentration", the spells Bless, Bane, Haste, Hold Person and Web, the rider
// of Vicious Mockery, and the Dodge action). effects/combat_effects.json is
// handwritten and checked by the loader: what each of those spells and actions
// puts on a creature, for how long, how it ends, and what the card tells the
// player. The play module reads
// it (Content.CombatSpellEffect, CombatEffect, ConditionInfo); the clock, the
// exhaustion table and the conditions' arithmetic are pure and live in
// rules/combat.

// The kinds of duration of a combat effect.
const (
	// EffectDurationRounds lasts a number of rounds, ending at the start of the
	// turn of its anchor (the caster's) that many rounds later: ten rounds for the
	// SRD's "1 minute", counted from the round of the casting.
	EffectDurationRounds = "rounds"
	// EffectDurationUntilStartOfTurnOf ends at the start of a combatant's next turn.
	EffectDurationUntilStartOfTurnOf = "until_start_of_turn_of"
	// EffectDurationUntilEndOfTurnOf ends at the end of a combatant's next turn.
	EffectDurationUntilEndOfTurnOf = "until_end_of_turn_of"
	// EffectDurationConcentration lasts while the caster keeps concentrating, with
	// no time limit of its own.
	EffectDurationConcentration = "concentration"
	// EffectDurationUntilDismissed lasts until the master ends it.
	EffectDurationUntilDismissed = "until_dismissed"
	// EffectDurationLongRest lasts until a long rest.
	EffectDurationLongRest = "long_rest"
)

// RoundsPerMinute is the SRD's length of a minute in rounds: a round is about
// 6 seconds ("Combat"), so a minute is 10 rounds.
const RoundsPerMinute = secondsPerMinute / SecondsPerRound

// The kinds of modifier an effect gives.
const (
	// ModifierRollDie adds (Sign +1) or subtracts (Sign -1) a die to the roll.
	ModifierRollDie = "roll_die"
	// ModifierACBonus adds Value to the armor class.
	ModifierACBonus = "ac_bonus"
	// ModifierSpeedMultiplier multiplies the speed by Value percent.
	ModifierSpeedMultiplier = "speed_multiplier"
	// ModifierSaveAdvantage gives advantage on the saving throws of Abilities.
	ModifierSaveAdvantage = "save_advantage"
	// ModifierExtraAction gives one more action each turn, limited to Allowed.
	ModifierExtraAction = "extra_action"
	// ModifierNoMove stops the creature from moving.
	ModifierNoMove = "no_move"
	// ModifierNoAction stops the creature from taking actions.
	ModifierNoAction = "no_action"
	// ModifierNoReaction stops the creature from taking reactions (the Open Hand
	// technique: no reactions until the end of the monk's next turn).
	ModifierNoReaction = "no_reaction"
	// ModifierCheckBonus adds a bonus to the checks of a skill (Pass without Trace: +10 to
	// Dexterity (Stealth)).
	ModifierCheckBonus = "check_bonus"
	// ModifierBaseAC makes the armor class of a creature that wears no armor at least Value
	// (Mage Armor: 13 + Dexterity, worked out for the target when it is cast).
	ModifierBaseAC = "base_ac"
	// ModifierSpeedAdd adds feet to the walking speed (Longstrider: +10 ft).
	ModifierSpeedAdd = "speed_add"
	// ModifierCheckAdvantage gives advantage on the ability checks of Abilities (Enhance Ability:
	// the caster picks the ability when casting, Choose says so).
	ModifierCheckAdvantage = "check_advantage"
	// ModifierConditionImmunity makes the creature immune to the Conditions (Heroism: frightened).
	ModifierConditionImmunity = "condition_immunity"
	// ModifierTurnTempHP gives temporary hit points at the start of each of the creature's turns
	// (Heroism: the caster's spellcasting modifier, worked out when it is cast: FromCaster).
	ModifierTurnTempHP = "turn_temp_hp"
	// ModifierAttackDisadvantage gives the creature's attack rolls disadvantage. With Once, its
	// next attack roll spends the effect, hit or miss (Vicious Mockery). It takes no other field.
	ModifierAttackDisadvantage = "attack_disadvantage"
	// ModifierRollBonus adds a flat Value to the rolls of AppliesTo (a +1 to attack rolls). It is not the die of
	// roll_die, and it is not check_bonus, which is for one skill.
	ModifierRollBonus = "roll_bonus"
	// ModifierTempHP gives Value temporary hit points when the effect lands; turn_temp_hp gives them each turn.
	ModifierTempHP = "temp_hp"
	// ModifierResistance gives the creature resistance, immunity or vulnerability (Kind) to the DamageTypes.
	ModifierResistance = "resistance"
	// ModifierAttackAdvantage gives the creature's attack rolls advantage.
	ModifierAttackAdvantage = "attack_advantage"
	// ModifierSaveDisadvantage gives disadvantage on the saving throws of Abilities.
	ModifierSaveDisadvantage = "save_disadvantage"
	// ModifierCheckDisadvantage gives disadvantage on the ability checks of Abilities.
	ModifierCheckDisadvantage = "check_disadvantage"
	// ModifierAura makes the effect an aura: it sits on its bearer and reaches, at the roll or the damage, the
	// creatures within RadiusFt of the bearer on the Affects side (self, allies, enemies). The other modifiers of
	// the effect are what the aura gives them. It has no value of its own; it is written by the code of a table
	// spell's or feature's aura, never chosen as a modifier.
	ModifierAura = "aura"
)

// The sides an aura reaches, from its bearer's side.
const (
	AuraAffectsSelf    = "self"
	AuraAffectsAllies  = "allies"
	AuraAffectsEnemies = "enemies"
)

// The radius of an aura, in feet: 5 to 120, a multiple of 5 (a square of the map).
const (
	MinAuraRadiusFt = 5
	MaxAuraRadiusFt = 120
)

// AuraAffects are the sides an aura may reach.
var AuraAffects = []string{AuraAffectsSelf, AuraAffectsAllies, AuraAffectsEnemies}

// EffectGoaded is the effect a failed goad puts on the target: disadvantage on its attack rolls against
// anyone but the attacker, until the end of the attacker's next turn (effects/combat_effects.json).
const EffectGoaded = "effect:goaded"

// The rolls a ModifierRollDie applies to.
const (
	RollAppliesAttack = "attack"
	RollAppliesSave   = "save"
	RollAppliesCheck  = "check"
)

// The audience values of an effect's visibility: who of the players reads it.
const (
	// EffectVisibilityPublic effects show to every player (the label in the order,
	// the condition on the combatant); the other kind shows to the target's player
	// and the master only.
	EffectVisibilityPublic = "public"
	EffectVisibilityOwner  = "owner"
)

// maxPlayerLabel is the length of the free label the master may put on an effect.
const maxPlayerLabel = 30

// EffectModifier is one change an effect makes to a creature's numbers.
type EffectModifier struct {
	Kind string `json:"kind"`
	// Die and Sign are for ModifierRollDie: a d4 added (+1) or subtracted (-1);
	// AppliesTo says to which rolls.
	Die       int      `json:"die,omitempty"`
	Sign      int      `json:"sign,omitempty"`
	AppliesTo []string `json:"applies_to,omitempty"`
	// RadiusFt and Affects are for ModifierAura: how far from the bearer, and whom (AuraAffects).
	RadiusFt int    `json:"radius_ft,omitempty"`
	Affects  string `json:"affects,omitempty"`
	// Value is the armor class bonus, or the speed multiplier in percent.
	Value int `json:"value,omitempty"`
	// Skill is the skill key of ModifierCheckBonus ("skill:stealth").
	Skill string `json:"skill,omitempty"`
	// Abilities are the ability keys of ModifierSaveAdvantage and ModifierCheckAdvantage.
	Abilities []string `json:"abilities,omitempty"`
	// ExceptSource, on ModifierAttackDisadvantage, spares the effect's source: the disadvantage holds
	// against every target but the creature that put the effect on (a goad).
	ExceptSource bool `json:"except_source,omitempty"`
	// Once says the die is spent on the first roll it is added to, and the effect ends with it
	// (Guidance, Resistance); on ModifierAttackDisadvantage, that the next attack roll spends it
	// (Vicious Mockery: the effect stays, spent, until its clock ends it).
	Once bool `json:"once,omitempty"`
	// Choose says the caster picks the ability when casting (Enhance Ability).
	Choose bool `json:"choose,omitempty"`
	// Conditions are the condition keys of ModifierConditionImmunity.
	Conditions []string `json:"conditions,omitempty"`
	// FromCaster says Value is the caster's spellcasting modifier, set when the spell is cast.
	FromCaster bool `json:"from_caster,omitempty"`
	// Allowed are the standard actions an extra action may be ("attack",
	// "dash"...), and MaxWeaponAttacks how many weapon attacks that action makes.
	Allowed          []string `json:"allowed,omitempty"`
	MaxWeaponAttacks int      `json:"max_weapon_attacks,omitempty"`
	// ResistanceKind is a ResistanceKind* constant (empty is resist) and DamageTypes the damage type keys, or
	// DamageTypeAll or DamageTypeNonmagicalBPS alone; both are for ModifierResistance. They are not named
	// Kind because that field is the modifier's own kind.
	ResistanceKind string   `json:"resistance_kind,omitempty"`
	DamageTypes    []string `json:"damage_types,omitempty"`
	// Claim and Unclaimed are kept by the engine on a ModifierTempHP row: the pool this effect set (what its end may
	// take back) and that it set none. Value stays the spell's own number. Neither is set for a row without them,
	// which claims Value.
	Claim     int  `json:"claim,omitempty"`
	Unclaimed bool `json:"unclaimed,omitempty"`
}

// EffectDuration is how long an effect lasts.
type EffectDuration struct {
	Kind   string `json:"kind"`
	Rounds int    `json:"rounds,omitempty"`
}

// EffectSave is the saving throw an effect asks again: at the end of the
// target's turn (Hold Person) or at the start of it (Web). The DC is the
// caster's, kept on the effect when it is cast.
type EffectSave struct {
	Ability string `json:"ability"`
	// OnPass is what a passed save does: "end" ends the effect on the target. A
	// start-of-turn save that fails adds OnFail, an effect key.
	OnPass string `json:"on_pass,omitempty"`
	OnFail string `json:"on_fail,omitempty"`
	// OnDamage says the save is asked again each time the target takes damage, with
	// advantage (Hideous Laughter).
	OnDamage bool `json:"on_damage,omitempty"`
}

// EffectTrigger is damage an effect deals to a creature, with no saving throw: at the start of
// its turn when the effect's clock does not end at the end of a turn (the Web that burns), and as
// the effect ends at the end of the creature's turn when it does (Acid Arrow's later damage).
type EffectTrigger struct {
	Dice        string `json:"dice"`
	DamageType  string `json:"damage_type"`
	MaxTriggers int    `json:"max_triggers"`
	// PerSlotLevel is the dice a slot level above the spell's own adds to Dice (same die).
	PerSlotLevel string `json:"per_slot_level,omitempty"`
}

// DiceAt is the trigger's dice for a spell cast levelsAbove slot levels above its own.
func (t EffectTrigger) DiceAt(levelsAbove int) string {
	base, ok := ParseDice(t.Dice)
	extra, extraOK := ParseDice(t.PerSlotLevel)
	if !ok || !extraOK || levelsAbove < 1 || base.Sides != extra.Sides {
		return t.Dice
	}
	return fmt.Sprintf("%dd%d", base.Count+extra.Count*levelsAbove, base.Sides)
}

// EffectDef is what an effect of the catalog is: a spell's, an action's or one
// the app adds (the lethargy a Haste leaves, the web caught in fire).
type EffectDef struct {
	// Key is the effect's key: a spell's ("spell:bless") or "effect:<name>".
	Key string `json:"-"`
	// NamePT is its Portuguese name (effects/names_pt.json).
	NamePT string `json:"-"`
	// Kind is the source: "spell", "feature" or "master".
	Kind string `json:"kind,omitempty"`
	// Duration is the effect's own. A spell's comes from the spell (Content.SpellDetails), except
	// the rider of an instantaneous spell: it has a clock of its own on its target's turn (Vicious
	// Mockery: until the end of the target's next turn), riderDurations.
	Duration *EffectDuration `json:"duration,omitempty"`
	// Concentration is read from the spell; it is not in the file.
	Concentration bool `json:"-"`
	// Conditions are the condition keys it gives its targets.
	Conditions []string         `json:"conditions,omitempty"`
	Modifiers  []EffectModifier `json:"modifiers,omitempty"`
	// EndSave is the save at the end of the target's turn; StartSave the one at the
	// start of it.
	EndSave   *EffectSave    `json:"end_save,omitempty"`
	StartSave *EffectSave    `json:"start_save,omitempty"`
	Trigger   *EffectTrigger `json:"turn_trigger,omitempty"`
	// Applies says which targets get the effect when a spell is cast: "all" (Bless,
	// Haste), "failed_save" (Bane, Hold Person: the ones that failed the save the
	// spell asks) or "hit" (Acid Arrow: the ones the spell attack hit).
	Applies string `json:"applies,omitempty"`
	// TargetType limits the targets to a creature type ("humanoid": Hold Person).
	TargetType string `json:"target_type,omitempty"`
	// OnEnd is the effect that follows when this one ends (Haste's lethargy).
	OnEnd string `json:"on_end,omitempty"`
	// EndsEffectsOf are the effect keys that end on the target when this one ends
	// (the web burns away, and nobody is held by it).
	EndsEffectsOf []string `json:"ends_effects_of,omitempty"`
	// BreakFree is the ability of the check that frees a creature the effect holds
	// (Web: a Strength check against the caster's DC, as an action).
	BreakFree string `json:"break_free,omitempty"`
	// PlayerLabelPT is the label the other players read on the target when the
	// effect is visible ("Preso numa teia"); empty: the condition's name.
	PlayerLabelPT string `json:"player_label_pt,omitempty"`
	// Visibility is the default audience: EffectVisibilityPublic or ...Owner.
	Visibility string `json:"visibility,omitempty"`
	// TagsPT are the short labels of the card ("+1d4 em ataques e resistências"),
	// and ChangesPT the lines of "O que isso muda".
	TagsPT    []string `json:"tags_pt,omitempty"`
	ChangesPT []string `json:"changes_pt,omitempty"`
}

// ConditionInfo is what a condition shows to the table: who reads it and what
// it does to the creature, in our words (SRD 5.1, "Conditions").
type ConditionInfo struct {
	Key string `json:"-"`
	// Visibility is EffectVisibilityPublic (the table sees it on the creature) or
	// EffectVisibilityOwner (only the creature's player and the master).
	Visibility string `json:"visibility"`
	// ChangesPT are the lines "O que isso muda" of the creature's own card.
	ChangesPT []string `json:"changes_pt"`
}

type combatEffectsFile struct {
	Source     string                   `json:"source"`
	Conditions map[string]ConditionInfo `json:"conditions"`
	Spells     map[string]*EffectDef    `json:"spells"`
	Effects    map[string]*EffectDef    `json:"effects"`
}

// combatEffects is the loaded file.
type combatEffects struct {
	conditions map[string]ConditionInfo
	spells     map[string]*EffectDef
	effects    map[string]*EffectDef
}

var (
	effectKinds     = []string{"spell", "feature", "master"}
	effectApplies   = []string{"all", "failed_save", "hit"}
	effectDurations = []string{
		EffectDurationRounds, EffectDurationUntilStartOfTurnOf, EffectDurationUntilEndOfTurnOf,
		EffectDurationConcentration, EffectDurationUntilDismissed, EffectDurationLongRest,
	}
	modifierKinds = []string{ModifierRollDie, ModifierACBonus, ModifierSpeedMultiplier, ModifierSaveAdvantage, ModifierExtraAction, ModifierNoMove, ModifierNoAction, ModifierNoReaction, ModifierCheckBonus, ModifierSpeedAdd, ModifierBaseAC, ModifierCheckAdvantage, ModifierConditionImmunity, ModifierTurnTempHP, ModifierAttackDisadvantage, ModifierRollBonus, ModifierTempHP, ModifierResistance, ModifierAttackAdvantage, ModifierSaveDisadvantage, ModifierCheckDisadvantage, ModifierAura}
	saveAbilities = []string{"str", "dex", "con", "int", "wis", "cha"}
	// extraActions are the standard actions an extra action may be: the keys of
	// effects/standard_actions.json.
	extraActions = []string{"attack", "dash", "disengage", "hide", "use-an-object"}
	// riderDurations are the durations a spell's entry may give itself: the clock of a rider that
	// outlasts an instantaneous spell, on its target's next turn (Vicious Mockery: until the end of
	// it). Any other spell's effect lasts as long as the spell does.
	riderDurations = []string{EffectDurationUntilStartOfTurnOf, EffectDurationUntilEndOfTurnOf}
)

// combatEffectsName is the file loadCombatEffects reads.
const combatEffectsName = "effects/combat_effects.json"

// loadCombatEffects reads and checks effects/combat_effects.json.
func (c *content) loadCombatEffects(fsys fs.FS) error {
	const name = combatEffectsName
	var f combatEffectsFile
	if err := readJSON(fsys, name, &f); err != nil {
		return err
	}
	if strings.TrimSpace(f.Source) == "" {
		return fmt.Errorf("%s: it needs a source line", name)
	}
	out := &combatEffects{conditions: map[string]ConditionInfo{}, spells: map[string]*EffectDef{}, effects: map[string]*EffectDef{}}
	steps := []func(f *combatEffectsFile, out *combatEffects) error{
		c.loadEffectConditions, c.loadEffectSpells, c.loadEffectEntries, c.checkEffectNames, c.checkEffectDefs,
	}
	for _, step := range steps {
		if err := step(&f, out); err != nil {
			return err
		}
	}
	c.combatEffects = out
	return nil
}

// loadEffectConditions keeps the file's conditions, and refuses one that is
// not the SRD's, one of no audience or lines, and a missing one.
func (c *content) loadEffectConditions(f *combatEffectsFile, out *combatEffects) error {
	const name = combatEffectsName
	for key, info := range f.Conditions {
		if _, ok := c.named[key]; !ok {
			return fmt.Errorf("%s: %q is not an SRD condition", name, key)
		}
		if info.Visibility != EffectVisibilityPublic && info.Visibility != EffectVisibilityOwner {
			return fmt.Errorf("%s: %s: visibility must be %q or %q", name, key, EffectVisibilityPublic, EffectVisibilityOwner)
		}
		if len(info.ChangesPT) == 0 {
			return fmt.Errorf("%s: %s needs its lines in changes_pt", name, key)
		}
		info.Key = key
		out.conditions[key] = info
	}
	for key := range c.named {
		if strings.HasPrefix(key, "condition:") && out.conditions[key].Key == "" {
			return fmt.Errorf("%s: the condition %s is missing", name, key)
		}
	}
	return nil
}

// loadEffectSpells keeps the spells' effects, with what the spell says of
// itself.
func (c *content) loadEffectSpells(f *combatEffectsFile, out *combatEffects) error {
	const name = combatEffectsName
	for key, def := range f.Spells {
		sp, ok := c.spells[key]
		if !ok {
			return fmt.Errorf("%s: %q is not an SRD spell", name, key)
		}
		def.Key, def.Kind, def.NamePT = key, "spell", c.namesPT[key]
		def.Concentration = sp.Concentration
		if def.Duration != nil {
			if err := checkRiderDuration(sp, def.Duration); err != nil {
				return fmt.Errorf("%s: %s: %w", name, key, err)
			}
		}
		out.spells[key] = def
	}
	return nil
}

// loadEffectEntries keeps the effects that are not spells, with their
// Portuguese names; a kind left out is "feature".
func (c *content) loadEffectEntries(f *combatEffectsFile, out *combatEffects) error {
	const name = combatEffectsName
	for key, def := range f.Effects {
		if !strings.HasPrefix(key, "effect:") {
			return fmt.Errorf("%s: effect key %q must start with \"effect:\"", name, key)
		}
		def.Key, def.NamePT = key, c.namesPT[key]
		if def.NamePT == "" {
			return fmt.Errorf("%s: %s has no Portuguese name in effects/names_pt.json", name, key)
		}
		if def.Kind == "" {
			def.Kind = "feature"
		}
		out.effects[key] = def
	}
	return nil
}

// checkEffectNames refuses a Portuguese effect name with no effect behind it.
func (c *content) checkEffectNames(_ *combatEffectsFile, out *combatEffects) error {
	for _, k := range c.namesWithPrefix("effect:") {
		if out.effects[k] == nil {
			return fmt.Errorf("effects/names_pt.json names %q, which is not in combat_effects.json", k)
		}
	}
	return nil
}

// checkEffectDefs checks every definition, spells first, in key order.
func (c *content) checkEffectDefs(_ *combatEffectsFile, out *combatEffects) error {
	for _, def := range allDefs(out) {
		if err := c.checkEffectDef(combatEffectsName, def, out); err != nil {
			return err
		}
	}
	return nil
}

// checkRiderDuration refuses a duration on a spell's entry, but for the rider of an instantaneous
// spell, whose clock is its target's next turn (riderDurations).
func checkRiderDuration(sp *srd51.Spell, d *EffectDuration) error {
	if parseDuration(sp.Duration, sp.Concentration).Kind != DurationInstantaneous {
		return errors.New("a spell's duration comes from the spell, leave duration out (only the rider of an instantaneous spell has its own)")
	}
	if !slices.Contains(riderDurations, d.Kind) {
		return fmt.Errorf("the rider of an instantaneous spell lasts until_start_of_turn_of or until_end_of_turn_of, not %q", d.Kind)
	}
	return nil
}

// allDefs lists the effects of the file, spells first, in key order.
func allDefs(f *combatEffects) []*EffectDef {
	var keys []string
	for k := range f.spells {
		keys = append(keys, k)
	}
	slices.Sort(keys)
	var effectKeys []string
	for k := range f.effects {
		effectKeys = append(effectKeys, k)
	}
	slices.Sort(effectKeys)
	var out []*EffectDef
	for _, k := range keys {
		out = append(out, f.spells[k])
	}
	for _, k := range effectKeys {
		out = append(out, f.effects[k])
	}
	return out
}

// effectFail builds the error of a definition's field, prefixed with the file
// and the effect's key.
type effectFail func(format string, a ...any) error

// effectDefCheck is one step of checkEffectDef: one field, or a group of
// related fields, of a definition.
type effectDefCheck func(c *content, d *EffectDef, all *combatEffects, bad effectFail) error

// effectDefChecks are checkEffectDef's steps, in the order the first refusal
// is chosen.
var effectDefChecks = []effectDefCheck{
	checkEffectHeader,
	checkEffectConditions,
	checkEffectModifiers,
	checkEffectSaves,
	checkEffectTrigger,
	checkEffectFollowers,
	checkEffectFooter,
}

// checkEffectDef refuses what an effect of the file may not say.
func (c *content) checkEffectDef(file string, d *EffectDef, all *combatEffects) error {
	bad := func(format string, a ...any) error {
		return fmt.Errorf("%s: %s: %s", file, d.Key, fmt.Sprintf(format, a...))
	}
	for _, check := range effectDefChecks {
		if err := check(c, d, all, bad); err != nil {
			return err
		}
	}
	return nil
}

// checkEffectHeader checks the closed values: kind, duration, applies,
// visibility and target type.
func checkEffectHeader(_ *content, d *EffectDef, _ *combatEffects, bad effectFail) error {
	if !slices.Contains(effectKinds, d.Kind) {
		return bad("kind must be spell, feature or master")
	}
	if err := checkEffectDuration(d.Duration, bad); err != nil {
		return err
	}
	if d.Applies != "" && !slices.Contains(effectApplies, d.Applies) {
		return bad("applies must be all, failed_save or hit")
	}
	if d.Visibility != "" && d.Visibility != EffectVisibilityPublic && d.Visibility != EffectVisibilityOwner {
		return bad("unknown visibility %q", d.Visibility)
	}
	if d.TargetType != "" && d.TargetType != "humanoid" {
		return bad("the only target type limit is humanoid")
	}
	return nil
}

// checkEffectDuration refuses an unknown kind, and rounds on any kind but
// the rounds one (or that one without them).
func checkEffectDuration(dur *EffectDuration, bad effectFail) error {
	if dur == nil {
		return nil
	}
	if !slices.Contains(effectDurations, dur.Kind) {
		return bad("unknown duration kind %q", dur.Kind)
	}
	if (dur.Kind == EffectDurationRounds) != (dur.Rounds > 0) {
		return bad("rounds go with the rounds duration, and only with it")
	}
	return nil
}

func checkEffectConditions(_ *content, d *EffectDef, all *combatEffects, bad effectFail) error {
	for _, k := range d.Conditions {
		if _, ok := all.conditions[k]; !ok {
			return bad("%q is not an SRD condition", k)
		}
	}
	return nil
}

func checkEffectModifiers(c *content, d *EffectDef, _ *combatEffects, bad effectFail) error {
	for i, m := range d.Modifiers {
		if err := checkModifier(m); err != nil {
			return bad("modifiers[%d]: %v", i, err)
		}
		if err := c.checkModifierContent(m); err != nil {
			return bad("modifiers[%d]: %v", i, err)
		}
	}
	return nil
}

// checkModifierContent checks what a modifier names that only the content knows: the damage types of a
// resistance, the conditions of an immunity and the skill of a check bonus. checkModifier has already
// checked its shape.
func (c *content) checkModifierContent(m EffectModifier) error {
	switch m.Kind {
	case ModifierResistance:
		if msg := c.checkDamageTypes(&Effect{DamageTypes: m.DamageTypes}); msg != "" {
			return modifierField("damage_types", "resistance %s", msg)
		}
	case ModifierConditionImmunity:
		for j, k := range m.Conditions {
			if !c.isCondition(k) {
				return modifierField(fmt.Sprintf("conditions[%d]", j), "%q is not an SRD condition", k)
			}
		}
	case ModifierCheckBonus:
		if c.skills[m.Skill] == nil {
			return modifierField("skill", "%q is not an SRD skill", m.Skill)
		}
	}
	return nil
}

// checkEffectSaves checks the end save, then the start save.
func checkEffectSaves(_ *content, d *EffectDef, all *combatEffects, bad effectFail) error {
	for _, s := range []*EffectSave{d.EndSave, d.StartSave} {
		if err := checkEffectSave(s, all, bad); err != nil {
			return err
		}
	}
	return nil
}

func checkEffectSave(s *EffectSave, all *combatEffects, bad effectFail) error {
	if s == nil {
		return nil
	}
	if !slices.Contains(saveAbilities, s.Ability) {
		return bad("a save needs an ability")
	}
	if s.OnPass != "" && s.OnPass != "end" {
		return bad("on_pass can only be end")
	}
	if s.OnFail != "" {
		if _, ok := all.effects[s.OnFail]; !ok {
			return bad("on_fail names %q, which is not an effect of the file", s.OnFail)
		}
	}
	return nil
}

func checkEffectTrigger(c *content, d *EffectDef, _ *combatEffects, bad effectFail) error {
	t := d.Trigger
	if t == nil {
		return nil
	}
	f, ok := ParseDice(t.Dice)
	if !ok || f.Count < 1 || f.AddsModifier {
		return bad("turn_trigger.dice %q is not a dice formula", t.Dice)
	}
	if !strings.HasPrefix(t.DamageType, "damage-type:") || c.named[t.DamageType] == nil {
		return bad("turn_trigger.damage_type %q is not an SRD damage type", t.DamageType)
	}
	if t.MaxTriggers < 1 {
		return bad("turn_trigger.max_triggers must be at least 1")
	}
	if t.PerSlotLevel != "" {
		if more, ok := ParseDice(t.PerSlotLevel); !ok || more.Count < 1 || more.AddsModifier || more.Sides != f.Sides {
			return bad("turn_trigger.per_slot_level %q is not dice of the same die as %q", t.PerSlotLevel, t.Dice)
		}
	}
	return nil
}

// checkEffectFollowers checks on_end, then ends_effects_of: each names an
// effect or a spell of the file.
func checkEffectFollowers(_ *content, d *EffectDef, all *combatEffects, bad effectFail) error {
	for _, k := range append([]string{d.OnEnd}, d.EndsEffectsOf...) {
		if k == "" {
			continue
		}
		if _, ok := all.effects[k]; !ok && all.spells[k] == nil {
			return bad("%q is not an effect of the file", k)
		}
	}
	return nil
}

// checkEffectFooter checks break_free and the label's length.
func checkEffectFooter(_ *content, d *EffectDef, _ *combatEffects, bad effectFail) error {
	if d.BreakFree != "" && !slices.Contains(saveAbilities, d.BreakFree) {
		return bad("break_free needs an ability")
	}
	if len([]rune(d.PlayerLabelPT)) > maxPlayerLabel {
		return bad("player_label_pt is over %d characters", maxPlayerLabel)
	}
	return nil
}

// The bounds of a modifier's numbers.
const (
	// minModifierDie is the smallest die a roll_die adds: a d2.
	minModifierDie = 2
	// maxModifierDie is the largest, a d100.
	maxModifierDie = 100
	// maxACBonus, maxSpeedAdd and maxCheckBonus bound the flat numbers of ac_bonus, speed_add (feet) and
	// check_bonus (either sign).
	maxACBonus    = 20
	maxSpeedAdd   = 120
	maxCheckBonus = 20
	// maxBaseAC is the highest armor class a base_ac may set.
	maxBaseAC = 60
	// maxSpeedPercent is the highest speed_multiplier, in percent.
	maxSpeedPercent = 400
)

// modifierChecks are the checks of each kind of modifier that has fields; a
// kind missing here takes none.
var modifierChecks = map[string]func(m EffectModifier) error{
	ModifierRollDie:           checkRollDie,
	ModifierCheckBonus:        checkCheckBonus,
	ModifierBaseAC:            checkBaseAC,
	ModifierSpeedAdd:          checkSpeedAdd,
	ModifierACBonus:           checkACBonus,
	ModifierSpeedMultiplier:   checkSpeedMultiplier,
	ModifierSaveAdvantage:     checkSaveAdvantage,
	ModifierCheckAdvantage:    checkCheckAdvantage,
	ModifierConditionImmunity: checkConditionImmunity,
	ModifierTurnTempHP:        checkTurnTempHP,
	ModifierExtraAction:       checkExtraAction,
	ModifierRollBonus:         checkRollBonus,
	ModifierTempHP:            checkTempHP,
	ModifierResistance:        checkResistanceModifier,
	ModifierSaveDisadvantage:  checkAbilityDisadvantage,
	ModifierCheckDisadvantage: checkAbilityDisadvantage,
	ModifierAura:              checkAuraModifier,
}

// checkModifier checks one modifier's fields against its kind.
func checkModifier(m EffectModifier) error {
	if !slices.Contains(modifierKinds, m.Kind) {
		return modifierField("kind", "unknown kind %q", m.Kind)
	}
	if check, ok := modifierChecks[m.Kind]; ok {
		return check(m)
	}
	return nil
}

// modifierFieldError is a refusal of one field of a modifier, so a caller that reports by field (a table
// spell's effect) names it.
type modifierFieldError struct{ field, msg string }

func (e *modifierFieldError) Error() string { return e.msg }

func modifierField(field, format string, a ...any) error {
	return &modifierFieldError{field: field, msg: fmt.Sprintf(format, a...)}
}

// The bounds of a roll_bonus and a temp_hp.
const (
	maxRollBonus = 20
	maxTempHP    = 1000
)

// checkAuraModifier checks the radius and the side of an aura.
func checkAuraModifier(m EffectModifier) error {
	if m.RadiusFt < MinAuraRadiusFt || m.RadiusFt > MaxAuraRadiusFt || m.RadiusFt%5 != 0 {
		return modifierField("radius_ft", "an aura's radius is from %d to %d feet, in steps of 5", MinAuraRadiusFt, MaxAuraRadiusFt)
	}
	if !slices.Contains(AuraAffects, m.Affects) {
		return modifierField("affects", "an aura reaches self, allies or enemies")
	}
	return nil
}

func checkRollBonus(m EffectModifier) error {
	if m.Value == 0 || m.Value < -maxRollBonus || m.Value > maxRollBonus {
		return modifierField("value", "roll_bonus needs a value from 1 to 20, or -20 to -1")
	}
	if len(m.AppliesTo) == 0 {
		return modifierField("applies_to", "roll_bonus needs applies_to")
	}
	return checkAppliesTo(m.AppliesTo)
}

func checkTempHP(m EffectModifier) error {
	if m.Value < 1 || m.Value > maxTempHP {
		return modifierField("value", "temp_hp needs a value from 1 to 1000")
	}
	return nil
}

// checkResistanceModifier checks the kind of a resistance, that it names damage types and that
// DamageTypeNonmagicalBPS stands alone. Whether the keys are damage types, and the rest of DamageTypeAll's
// rule, is checkModifierContent's, which has the content.
func checkResistanceModifier(m EffectModifier) error {
	if !slices.Contains(ResistanceKinds, m.ResistanceKind) {
		return modifierField("resistance_kind", "resistance_kind is resist, immune or vulnerable")
	}
	if len(m.DamageTypes) == 0 {
		return modifierField("damage_types", "resistance needs damage_types")
	}
	if slices.Contains(m.DamageTypes, DamageTypeNonmagicalBPS) && len(m.DamageTypes) > 1 {
		return modifierField("damage_types", "damage_types: %q cannot be mixed with other damage types", DamageTypeNonmagicalBPS)
	}
	return nil
}

// checkAbilityDisadvantage is the check of save_disadvantage and check_disadvantage: the abilities it holds for.
func checkAbilityDisadvantage(m EffectModifier) error {
	if len(m.Abilities) == 0 {
		return modifierField("abilities", "%s needs abilities", m.Kind)
	}
	return checkModifierAbilities(m.Abilities)
}

func checkRollDie(m EffectModifier) error {
	if m.Die > maxModifierDie {
		return modifierField("die", "roll_die is at most a d%d", maxModifierDie)
	}
	if m.Die < minModifierDie || (m.Sign != 1 && m.Sign != -1) || len(m.AppliesTo) == 0 {
		return modifierField("die", "roll_die needs die, sign (1 or -1) and applies_to")
	}
	return checkAppliesTo(m.AppliesTo)
}

// checkAppliesTo refuses a roll that is not attack, save or check, and a repeated one.
func checkAppliesTo(applies []string) error {
	for i, a := range applies {
		switch {
		case a != RollAppliesAttack && a != RollAppliesSave && a != RollAppliesCheck:
			return modifierField("applies_to", "applies_to %q: only attack, save and check", a)
		case slices.Contains(applies[:i], a):
			return modifierField("applies_to", "applies_to %q is listed twice", a)
		}
	}
	return nil
}

func checkCheckBonus(m EffectModifier) error {
	if m.Skill == "" || m.Value == 0 {
		return modifierField("skill", "check_bonus needs a skill and a value")
	}
	if m.Value < -maxCheckBonus || m.Value > maxCheckBonus {
		return modifierField("value", "check_bonus is from -20 to 20")
	}
	return nil
}

func checkBaseAC(m EffectModifier) error {
	if m.Value < 1 || m.Value > maxBaseAC {
		return fmt.Errorf("base_ac needs an armor class from 1 to 60")
	}
	return nil
}

func checkSpeedAdd(m EffectModifier) error {
	if m.Value > maxSpeedAdd {
		return modifierField("value", "speed_add is at most %d feet", maxSpeedAdd)
	}
	if m.Value < 1 {
		return modifierField("value", "speed_add needs a value in feet")
	}
	return nil
}

func checkACBonus(m EffectModifier) error {
	if m.Value > maxACBonus {
		return modifierField("value", "ac_bonus is at most %d", maxACBonus)
	}
	if m.Value < 1 {
		return modifierField("value", "ac_bonus needs a value")
	}
	return nil
}

func checkSpeedMultiplier(m EffectModifier) error {
	if m.Value < 0 || m.Value > maxSpeedPercent {
		return fmt.Errorf("speed_multiplier is a percent from 0 to 400")
	}
	return nil
}

func checkSaveAdvantage(m EffectModifier) error {
	if len(m.Abilities) == 0 {
		return fmt.Errorf("save_advantage needs abilities")
	}
	return checkModifierAbilities(m.Abilities)
}

func checkCheckAdvantage(m EffectModifier) error {
	if len(m.Abilities) == 0 && !m.Choose {
		return fmt.Errorf("check_advantage needs abilities, or choose")
	}
	return checkModifierAbilities(m.Abilities)
}

// checkModifierAbilities refuses an ability key that is not one of the six.
func checkModifierAbilities(abilities []string) error {
	for i, a := range abilities {
		switch {
		case !slices.Contains(saveAbilities, a):
			return modifierField("abilities", "unknown ability %q", a)
		case slices.Contains(abilities[:i], a):
			return modifierField("abilities", "ability %q is listed twice", a)
		}
	}
	return nil
}

func checkConditionImmunity(m EffectModifier) error {
	if len(m.Conditions) == 0 {
		return fmt.Errorf("condition_immunity needs conditions")
	}
	return nil
}

func checkTurnTempHP(m EffectModifier) error {
	if !m.FromCaster && m.Value < 1 {
		return modifierField("value", "turn_temp_hp needs a value, or from_caster")
	}
	if m.Value > maxTempHP {
		return modifierField("value", "turn_temp_hp is at most %d", maxTempHP)
	}
	return nil
}

func checkExtraAction(m EffectModifier) error {
	if len(m.Allowed) == 0 || m.MaxWeaponAttacks < 1 {
		return fmt.Errorf("extra_action needs allowed and max_weapon_attacks")
	}
	for _, a := range m.Allowed {
		if !slices.Contains(extraActions, a) {
			return fmt.Errorf("%q is not a standard action", a)
		}
	}
	return nil
}

// CombatSpellEffect returns what a spell puts on its targets in a combat
// (Bênção, Perdição, Velocidade, Imobilizar Pessoa, Teia, Zombaria Viciosa), and
// whether it has such an effect. Its Duration is nil when the effect lasts as the
// spell does (SpellEffectRounds, the concentration), and set for the rider of an
// instantaneous spell, whose clock is its target's next turn (Zombaria Viciosa:
// until the end of it). The result is shared and must not be modified.
func (c *Content) CombatSpellEffect(spellKey string) (*EffectDef, bool) {
	if d, ok := c.c.tableSpellEffects[spellKey]; ok {
		// A table spell that is archived or switched off no longer puts anything on anyone.
		hidden := c.c.archived[spellKey] || c.c.off[spellKey]
		return d, !hidden
	}
	if c.c.combatEffects == nil {
		return nil, false
	}
	d, ok := c.c.combatEffects.spells[spellKey]
	return d, ok
}

// CombatEffect returns an effect of the catalog that is not a spell ("effect:dodging",
// "effect:lethargy"...). The result is shared and must not be modified.
func (c *Content) CombatEffect(key string) (*EffectDef, bool) {
	if c.c.combatEffects == nil {
		return nil, false
	}
	d, ok := c.c.combatEffects.effects[key]
	return d, ok
}

// CombatEffectKeys lists the keys of the catalog's non-spell effects, sorted.
func (c *Content) CombatEffectKeys() []string {
	if c.c.combatEffects == nil {
		return nil
	}
	keys := make([]string, 0, len(c.c.combatEffects.effects))
	for k := range c.c.combatEffects.effects {
		keys = append(keys, k)
	}
	slices.Sort(keys)
	return keys
}

// ConditionInfo says who reads a condition and what it does to its creature.
func (c *Content) ConditionInfo(key string) (ConditionInfo, bool) {
	if c.c.combatEffects == nil {
		return ConditionInfo{}, false
	}
	i, ok := c.c.combatEffects.conditions[key]
	return i, ok
}

// SpellEffectRounds is how many rounds a spell's effect lasts in a combat, from
// the spell's own duration: ten for each minute; 0 for a spell that lasts an
// hour or more (the app does not count that time: the effect lasts until the
// master ends it or the concentration does), and for one that has no timed
// duration.
func (c *Content) SpellEffectRounds(spellKey string) int {
	d, ok := c.c.spellDetails[spellKey]
	if !ok {
		return 0
	}
	seconds, timed := d.Duration.Seconds()
	if !timed {
		return 0
	}
	return seconds / SecondsPerRound
}

// SpellEffectSeconds is the game time a spell's effect lasts, in seconds: the spell's own
// duration, for the effect given to a character outside a combat. false for a spell with no
// timed duration.
func (c *Content) SpellEffectSeconds(spellKey string) (int, bool) {
	d, ok := c.c.spellDetails[spellKey]
	if !ok {
		return 0, false
	}
	return d.Duration.Seconds()
}

// ConditionKeys lists the keys of the conditions the file describes (the SRD's 15), sorted.
func (c *Content) ConditionKeys() []string {
	if c.c.combatEffects == nil {
		return nil
	}
	keys := make([]string, 0, len(c.c.combatEffects.conditions))
	for k := range c.c.combatEffects.conditions {
		keys = append(keys, k)
	}
	slices.Sort(keys)
	return keys
}
