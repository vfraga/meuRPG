package rules

import (
	"fmt"
	"slices"
	"strings"
)

// Where a superiority die goes (the `applies` of a superiority_die effect).
const (
	// ManeuverDamage adds the die to the damage of a weapon attack that hit.
	ManeuverDamage = "damage"
	// ManeuverAttack adds the die to an attack roll.
	ManeuverAttack = "attack"
	// ManeuverGrapple adds the die to the Athletics check of a grapple the character starts.
	ManeuverGrapple = "grapple"
	// ManeuverReduceMelee reduces the damage of a melee attack that hit the character
	// by the die plus an ability modifier; it costs the reaction.
	ManeuverReduceMelee = "reduce_melee_damage"
	// ManeuverGoad adds the die to the damage of a weapon attack that hit, like ManeuverDamage, and
	// asks the target for a Wisdom saving throw against the maneuver's DC (8 + proficiency + the
	// modifier of Ability): on a failure it has disadvantage on its attack rolls against creatures
	// other than the attacker until the end of the attacker's next turn (EffectGoaded).
	ManeuverGoad = "goad"
	// ManeuverCheck adds the die to an ability check of the character (a scene check, a search, a contest...),
	// all of them or, with `targets`, the skills and abilities it lists.
	ManeuverCheck = "check"
	// ManeuverSave adds the die to a saving throw of the character, any or, with `ability`, that ability's.
	ManeuverSave = "save"
	// ManeuverInitiative adds the die to the initiative roll.
	ManeuverInitiative = "initiative"
)

// Bounds of a superiority die's sides: a formula that gives anything else is ignored.
const (
	minManeuverSides = 2
	maxManeuverSides = 100
)

// Maneuver is a superiority_die effect of a table option the character has: a die
// from a resource that goes on a roll. The engine is generic; the table names it.
type Maneuver struct {
	// Key is the feature (option) key that gives it, NamePT its name.
	Key, NamePT string
	// Resource is the resource each use spends, one use each.
	Resource string
	// Sides is the die, worked out for the character's level.
	Sides int
	// Applies is one of the Maneuver* constants.
	Applies string
	// Economy is bonus_action or reaction when using it costs one; empty when it does not.
	Economy string
	// Targets are, for ManeuverCheck, the skills ("skill.stealth") and abilities ("check.dex") the die goes
	// on; empty is every ability check.
	Targets []string
	// Ability is, for ManeuverReduceMelee, the ability whose modifier is added, and for ManeuverGoad
	// the one whose modifier is in SaveDC.
	Ability string
	// SaveDC is the DC of the target's saving throw against a goad (8 + proficiency + the modifier of
	// Ability); 0 for any other maneuver.
	SaveDC int
	// TextPT is the rider the master reads ("o alvo faz um teste de Sabedoria...").
	TextPT string
}

// maneuvers fills Derived.Maneuvers from the superiority_die effects the character
// has. The resource must be one the character has at this level, as for an action.
func (x *deriver) maneuvers() {
	for _, a := range x.active {
		e := a.effect
		if e.Type != "superiority_die" || !x.applies(a) {
			continue
		}
		if !slices.ContainsFunc(x.d.Resources, func(r Resource) bool { return r.Key == e.Resource }) {
			continue
		}
		sides, err := e.value.Int(x.env)
		if err != nil || sides < minManeuverSides || sides > maxManeuverSides {
			x.issue(IssueFormula, "", "Um efeito de %s foi ignorado: o dado não é válido.", x.c.namePT(a.owner))
			continue
		}
		x.d.Maneuvers = append(x.d.Maneuvers, Maneuver{
			Key: a.owner, NamePT: x.c.namePT(a.owner), Resource: e.Resource, Sides: sides,
			Applies: e.Applies, Economy: e.Economy, Ability: e.Ability, TextPT: e.TextPT, SaveDC: x.goadDC(e), Targets: slices.Clone(e.Targets),
		})
	}
}

// rollDieShape names the field of a superiority die that goes on a roll of the character (a check, a save or the
// initiative) that is wrong and why (both "" when right). The keys of `targets` are the content's to check.
func rollDieShape(e *Effect) (field, msg string) {
	switch {
	case e.Economy != "":
		return "economy", "a die added to a roll costs no action"
	case e.Applies != ManeuverCheck && len(e.Targets) > 0:
		return "targets", "only a die added to ability checks lists targets"
	case e.Applies != ManeuverSave && e.Ability != "":
		return "ability", "only a die added to a saving throw names an ability"
	case e.Applies == ManeuverSave && e.Ability != "":
		if _, ok := abilityIndex[Ability(e.Ability)]; !ok {
			return "ability", fmt.Sprintf("ability %q is not an ability", e.Ability)
		}
	}
	return "", ""
}

// rollDieTargetsOK says every target of a die on ability checks is a skill ("skill.stealth") or an ability
// ("check.dex") of the content.
func (c *content) rollDieTargetsOK(targets []string) (string, bool) {
	for _, t := range targets {
		known := strings.HasPrefix(t, "skill.") || strings.HasPrefix(t, "check.")
		if t == "check.all" || !known || !c.validRollTarget(t) {
			return t, false
		}
	}
	return "", true
}

// isRollDie says the superiority die is checked as a die on a roll of the character: a check, a save or the initiative.
func isRollDie(applies string) bool {
	return applies == ManeuverCheck || applies == ManeuverSave || applies == ManeuverInitiative
}

// IsRollDie says the superiority die goes on a roll of the character the player names (RollDie): a check, a save, the
// initiative or a weapon attack. An attack die is checked as a die added to an attack, which takes no targets, ability or
// action.
func IsRollDie(applies string) bool {
	return isRollDie(applies) || applies == ManeuverAttack
}

// hasManeuver says the feature (an option) has a superiority_die effect that applies: the die
// is spent where it is added to a roll, so a grant_action of the same feature must not
// spend the same resource a second time.
func (x *deriver) hasManeuver(owner string) bool {
	return slices.ContainsFunc(x.active, func(a activeEffect) bool {
		return a.owner == owner && a.effect.Type == "superiority_die" && x.applies(a)
	})
}

// goadDC is the DC of a goad: 8 plus the proficiency bonus and the modifier of the effect's ability;
// 0 for a maneuver that asks no saving throw.
func (x *deriver) goadDC(e *Effect) int {
	if e.Applies != ManeuverGoad {
		return 0
	}
	return x.saveDC(e.Ability, e.DCAbilityAlt)
}

// goadAltProblem says what is wrong with the second ability of a goad's DC ("" when right): only a goad has one, it
// is an ability and not the first.
func goadAltProblem(e *Effect) string {
	switch {
	case e.DCAbilityAlt == "":
		return ""
	case e.Applies != ManeuverGoad:
		return "dc_ability_alt is for the DC of a goad"
	case !isAbilityKey(e.DCAbilityAlt):
		return fmt.Sprintf("dc_ability_alt %q is not an ability", e.DCAbilityAlt)
	case e.DCAbilityAlt == e.Ability:
		return "dc_ability_alt is a second ability: it cannot be the ability"
	}
	return ""
}

// goadBaseDC is the 8 of a maneuver's save DC.
const goadBaseDC = 8
