package rules

import (
	"slices"
	"strconv"

	"github.com/PuraFome/meuRPG/backend/internal/rules/formula"
)

// RollDamage is, besides the d20 rolls of combateffects.go, a roll a reroll covers (its
// `rolls`): a die of weapon or spell damage.
const RollDamage = "damage"

// rollApplies are the rolls a reroll covers: RollAppliesAttack, RollAppliesCheck and
// RollAppliesSave (the d20 rolls) and RollDamage (a die of weapon or spell damage).
var rollApplies = []string{RollAppliesAttack, RollAppliesCheck, RollAppliesSave, RollDamage}

// The bounds of the numbers of the roll effects.
const (
	// MinCriticalRange and MaxCriticalRange bound the lowest natural d20 that is a critical hit.
	MinCriticalRange = 15
	MaxCriticalRange = 20
	// MinRerollAtMost and MaxRerollAtMost bound a reroll's at_most: a die showing it or lower may be rolled again.
	MinRerollAtMost = 1
	MaxRerollAtMost = 20
	// MinRollMinimum and MaxRollMinimum bound a roll_minimum's value: a die that shows less counts as that
	// value. 1 would change nothing, and 20 is the top face of the d20 (a damage die smaller than the value
	// counts as its own size when the minimum is applied).
	MinRollMinimum = 2
	MaxRollMinimum = 20
)

// The scopes of the critical range (the applies of a critical_range modifier): the weapon attacks (the
// default), the spell attacks, or both.
const (
	CriticalScopeWeapon = "weapon"
	CriticalScopeSpell  = "spell"
	CriticalScopeBoth   = "both"
)

// criticalScopes are the scopes a critical_range modifier takes.
var criticalScopes = []string{CriticalScopeWeapon, CriticalScopeSpell, CriticalScopeBoth}

// CriticalRangeTarget is the modifier target that widens the critical range: the lowest natural d20 that
// is a critical hit. Only the mode set is meaningful, and only the deriver reads it (Derived.CriticalRange).
const CriticalRangeTarget = "critical_range"

// The modifier targets of the sheet's passives, the creature's melee reach and the hit points a hit die gives at least.
const (
	PassivePerceptionTarget    = "passive.perception"
	PassiveInvestigationTarget = "passive.investigation"
	PassiveInsightTarget       = "passive.insight"
	// ReachMeleeTarget is the creature's melee reach in feet, 5 without a rule; MaxWeaponReachFt bounds it.
	ReachMeleeTarget = "reach.melee"
	// HitDieMinimumTarget is the least a hit die heals on a short rest, 0 to MaxHitDieMinimum.
	HitDieMinimumTarget = "hitdie.minimum"
	MaxHitDieMinimum    = 40
	// CarryCapacityTarget is the carrying capacity in pounds, 15 times the Strength score without a rule;
	// MaxCarryCapacityLb bounds it.
	CarryCapacityTarget = "carry.capacity"
	MaxCarryCapacityLb  = 10000
	// DamageReductionWeaponTarget is the flat amount taken off the damage of each weapon attack that hits
	// the character, and DamageReductionNonmagicalWeaponTarget the same for the attacks that are not magical.
	// MaxDamageReduction bounds each source.
	DamageReductionWeaponTarget           = "damage.reduction.weapon"
	DamageReductionNonmagicalWeaponTarget = "damage.reduction.nonmagical_weapon"
	MaxDamageReduction                    = 100
)

// DamageReduction is the flat damage reduction one feature gives: its key, the amount, and whether it
// covers only the attacks that are not magical.
type DamageReduction struct {
	Source     string
	Amount     int
	Nonmagical bool
}

// RerollRule is a reroll effect the character has: a die of the covered rolls that shows AtMost or lower may
// be rolled again, and the new roll must be used (the halfling's Lucky trait). Each reroll spends Cost uses of Resource
// when it names one.
type RerollRule struct {
	// Applies are the rolls it covers (Roll*).
	Applies []string
	// AtMost is the highest face that may be rolled again.
	AtMost int
	// Resource is the resource each reroll spends, "" for a free one, and Cost how many uses.
	Resource string
	Cost     int
	// Source is the feature (or option) key that gives it.
	Source string
}

// RollMinimum is a roll_minimum effect the character has: a die of the covered rolls that shows less than Value
// counts as Value (the d20 of an attack, a check or a save; a die of weapon or spell damage).
type RollMinimum struct {
	// Applies are the rolls it covers (Roll*).
	Applies []string
	// Value is the lowest the die counts as, from MinRollMinimum to MaxRollMinimum.
	Value int
	// DamageType, a "damage-type:" key, limits a damage minimum to the dice of that type (the Elemental Adept
	// shape); "" is every die. Only a minimum that covers damage alone has it.
	DamageType string
	// Source is the feature (or option) key that gives it.
	Source string
}

// rollsShape says what is wrong with the list of rolls of a roll effect: the field and why, both "" when
// right. It is the shared check of the engine's load and of the table's menu.
func rollsShape(e *Effect) (field, msg string) {
	if len(e.Tags) > 0 {
		return "tags", "a " + e.Type + " has no tags: the engine never applies a tagged effect, and these have no hint to show"
	}
	if len(e.Rolls) == 0 {
		return "rolls", "rolls are required: attack, check, save or damage"
	}
	for i, r := range e.Rolls {
		if !slices.Contains(rollApplies, r) {
			return "rolls", "rolls " + strconv.Quote(r) + " is not on the menu: attack, check, save or damage"
		}
		if slices.Contains(e.Rolls[:i], r) {
			return "rolls", "rolls lists " + strconv.Quote(r) + " twice"
		}
	}
	return "", ""
}

// rerollShape says what is wrong with a reroll effect's own fields (the field and why), "" when right. The
// resource's existence and the cost formula are checked by the callers.
func rerollShape(e *Effect) (field, msg string) {
	if f, m := rollsShape(e); f != "" {
		return f, m
	}
	switch {
	case e.AtMost < MinRerollAtMost || e.AtMost > MaxRerollAtMost:
		return "at_most", "at_most is a face from 1 to 20"
	case e.Value != "":
		return "value", "a reroll has no value"
	case e.Cost != "" && e.Resource == "":
		return "cost", "cost needs the resource it spends"
	}
	if n, err := strconv.Atoi(e.Cost); err == nil && n < 1 {
		return "cost", "cost must be at least 1"
	}
	return "", ""
}

// rollMinimumShape says what is wrong with a roll_minimum effect's own fields (the field and why), "" when right:
// the rolls, and a literal value from 2 to 20. It has no tags (a tagged one would be a hint the engine never
// applies), no resource and no cost.
func rollMinimumShape(e *Effect) (field, msg string) {
	if f, m := rollsShape(e); f != "" {
		return f, m
	}
	n, err := strconv.Atoi(e.Value)
	switch {
	case err != nil || n < MinRollMinimum || n > MaxRollMinimum:
		return "value", "value is the lowest the die counts as: a number from 2 to 20"
	case e.DamageType != "" && !slices.Equal(e.Rolls, []string{RollDamage}):
		return "damage_type", "damage_type limits a minimum that covers damage alone: rolls must be only damage"
	case e.AtMost != 0:
		return "at_most", "a roll_minimum has no at_most"
	case e.Resource != "":
		return "resource", "a roll_minimum spends no resource"
	case e.Cost != "":
		return "cost", "a roll_minimum has no cost"
	}
	return "", ""
}

// criticalRangeShape says what is wrong with a modifier of the critical range: only the mode set, a
// literal value from 15 to 20 (a formula is clamped when it is worked out) and a known scope (applies).
func criticalRangeShape(e *Effect) (field, msg string) {
	if e.Applies != "" && !slices.Contains(criticalScopes, e.Applies) {
		return "applies", "unknown scope " + strconv.Quote(e.Applies) + " for critical_range: weapon, spell or both"
	}
	if e.Mode != "" && e.Mode != "set" {
		return "mode", "unknown mode " + strconv.Quote(e.Mode) + " for critical_range: only set"
	}
	if n, err := strconv.Atoi(e.Value); err == nil && (n < MinCriticalRange || n > MaxCriticalRange) {
		return "value", "critical_range is a natural d20 from 15 to 20"
	}
	return "", ""
}

// reroll checks a reroll effect and compiles its cost.
func (x effectCompiler) reroll() error {
	if field, msg := rerollShape(x.e); field != "" {
		return x.fail("%s", msg)
	}
	var err error
	x.e.cost, err = x.compile(x.e.Cost, formula.Int)
	return err
}

// checkReroll checks a reroll of the table's content at its own fields.
func (b *overlayBuilder) checkReroll(site effectSite, e *Effect) error {
	if e.Resource != "" && !b.resourceDefined(site.owner, e.Resource) {
		return site.fail(".resource", ReasonReference, "the reroll spends %q, which no resource effect of this entry or its class defines", e.Resource)
	}
	if field, msg := rerollShape(e); field != "" {
		return site.fail("."+field, ReasonValue, "%s", msg)
	}
	return nil
}

// rollMinimum checks a roll_minimum effect.
func (x effectCompiler) rollMinimum() error {
	if field, msg := rollMinimumShape(x.e); field != "" {
		return x.fail("%s", msg)
	}
	if t := x.e.DamageType; t != "" && !x.c.isLiveDamageType(t) {
		return x.fail("damage_type %q is not a damage type", t)
	}
	return nil
}

// checkRollMinimum checks a roll_minimum of the table's content at its own fields.
func (b *overlayBuilder) checkRollMinimum(site effectSite, e *Effect) error {
	if field, msg := rollMinimumShape(e); field != "" {
		return site.fail("."+field, ReasonValue, "%s", msg)
	}
	if t := e.DamageType; t != "" && !b.base.isLiveDamageType(t) {
		return site.fail(".damage_type", ReasonReference, "damage_type %q is not a damage type", t)
	}
	return nil
}

// checkModifier checks the part of a modifier the engine's load does not tell apart: the critical range, the
// only target that reads applies (its scope).
func (b *overlayBuilder) checkModifier(site effectSite, e *Effect) error {
	if e.Target != CriticalRangeTarget {
		if e.Applies != "" {
			return site.fail(".applies", ReasonValue, "applies (the scope) belongs to the %s target", CriticalRangeTarget)
		}
		return nil
	}
	if field, msg := criticalRangeShape(e); field != "" {
		return site.fail("."+field, ReasonValue, "%s", msg)
	}
	return nil
}

// rollEffects fills Derived.Rerolls and Derived.RollMinimums from the effects the character has, in the order of the effects.
func (x *deriver) rollEffects() {
	for _, a := range x.active {
		e := a.effect
		switch {
		case len(e.Tags) > 0 || !x.applies(a):
		case e.Type == "roll_minimum":
			x.d.RollMinimums = append(x.d.RollMinimums, RollMinimum{Applies: slices.Clone(e.Rolls), Value: literalValue(e.Value), DamageType: e.DamageType, Source: a.owner})
		case e.Type == "reroll":
			if cost, ok := x.rerollCost(a); ok {
				x.d.Rerolls = append(x.d.Rerolls, RerollRule{
					Applies: slices.Clone(e.Rolls), AtMost: e.AtMost, Resource: e.Resource, Cost: cost, Source: a.owner,
				})
			}
		}
	}
}

// rerollCost is the uses a reroll spends (1 by default), and false when the reroll is dropped: the resource
// it spends does not exist at this level, the cost formula failed or gave less than 1, or the cost is more than the resource has.
func (x *deriver) rerollCost(a activeEffect) (int, bool) {
	e := a.effect
	if e.Resource == "" {
		return 1, true
	}
	i := slicesIndexResource(x.d.Resources, e.Resource)
	if i < 0 {
		return 0, false
	}
	n := 1
	if e.cost != nil {
		var ok bool
		if n, ok = x.evalInt(e.cost, a); !ok || n < 1 {
			return 0, false
		}
	}
	return n, n <= x.d.Resources[i].Max // a cost above the uses the resource has could never be paid
}

// literalValue is the number of a literal value the shape check has accepted.
func literalValue(s string) int {
	n, _ := strconv.Atoi(s)
	return n
}

// criticalRangeFrom lowers the critical range by a modifier of the critical_range target: the lowest of the
// range so far and the effect's value, clamped to what the menu allows. The scope (applies) says which range:
// the weapon's (Derived.CriticalRange, the default), the spell's (Derived.SpellCriticalRange) or both.
// A tagged modifier is a hint only.
func (x *deriver) criticalRangeFrom(a activeEffect) {
	e := a.effect
	if e.Type != "modifier" || e.Target != CriticalRangeTarget || len(e.Tags) > 0 {
		return
	}
	v, ok := x.value(a)
	if !ok {
		return
	}
	v = min(max(v, MinCriticalRange), MaxCriticalRange)
	if e.Applies != CriticalScopeSpell {
		x.d.CriticalRange = min(x.d.CriticalRange, v)
	}
	if e.Applies == CriticalScopeSpell || e.Applies == CriticalScopeBoth {
		x.d.SpellCriticalRange = min(x.d.SpellCriticalRange, v)
	}
}

// criticalScopeValues is the menu's list of the scopes of the critical range.
func criticalScopeValues() []MenuValue {
	return []MenuValue{
		{Key: CriticalScopeWeapon, NamePT: "Ataques com arma", HintPT: "Só os ataques com arma (o padrão)."},
		{Key: CriticalScopeSpell, NamePT: "Ataques com magia", HintPT: "Só as jogadas de ataque de magia."},
		{Key: CriticalScopeBoth, NamePT: "Armas e magias", HintPT: "Os ataques com arma e as jogadas de ataque de magia."},
	}
}

// rollAppliesValues is the menu's list of rolls.
func rollAppliesValues() []MenuValue {
	return namedMenu(rollApplies, func(k string) string { return rollAppliesNms[k] })
}

var rollAppliesNms = map[string]string{
	RollAppliesAttack: "Jogada de ataque (o d20)", RollAppliesCheck: "Teste de habilidade (o d20)",
	RollAppliesSave: "Teste de resistência (o d20)", RollDamage: "Dado de dano (de arma ou magia)",
}

// strayRollFields refuses the fields of reroll and roll_minimum on an effect of another type.
func (x effectCompiler) strayRollFields() error {
	e := x.e
	if e.Type != "reroll" && e.Type != "roll_minimum" && len(e.Rolls) > 0 {
		return x.fail("rolls belongs to reroll and roll_minimum")
	}
	if e.Type != "reroll" && e.AtMost != 0 {
		return x.fail("at_most belongs to reroll")
	}
	return nil
}
