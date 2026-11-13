package rules

import (
	"slices"
	"strconv"
	"strings"
	"unicode/utf8"
)

// The natural_attack effect: an attack line of its own that is the creature's unarmed strike by
// another name (a bite, claws, horns). It is melee only, proficient, and the weapon rules and the
// melee reach change it like any melee weapon line; it is not the Unarmed Strike line, so what
// names that line (Martial Arts, Flurry of Blows, Ki-Empowered Strikes) does not read it. Savage
// Attacks and Brutal Critical apply to a natural line as to any melee weapon line: it has its own
// die and is not the unarmed strike key.

// NaturalAbilityBest is the ability of a natural attack that takes the better of Strength and
// Dexterity.
const NaturalAbilityBest = "best"

// naturalAbilities are the abilities a natural attack may name.
var naturalAbilities = []string{string(STR), string(DEX), NaturalAbilityBest}

// checkNaturalAttack says what is wrong with a natural_attack, or the zero problem.
// damageTypeOK says whether a key is a damage type of the content.
func checkNaturalAttack(e *Effect, damageTypeOK func(string) bool) weaponProblem {
	name := strings.TrimSpace(e.AttackName)
	switch {
	case name == "" || utf8.RuneCountInString(e.AttackName) > maxPseudoNameRunes:
		return problem(".attack_name", ReasonValue, "an attack has a name of 1 to "+strconv.Itoa(maxPseudoNameRunes)+" characters")
	case !pseudoDiceOK(e.DamageDice):
		return problem(".damage_dice", ReasonValue, "damage_dice is plain dice such as 1d6, up to "+strconv.Itoa(maxPseudoDice)+" dice of d4, d6, d8, d10 or d12")
	case !damageTypeOK(e.DamageType):
		return problem(".damage_type", ReasonReference, "damage_type is not a damage type")
	case !slices.Contains(naturalAbilities, e.Ability):
		return problem(".ability", ReasonValue, "ability is "+strings.Join(naturalAbilities, ", "))
	case len(e.Tags) > 0:
		return problem(".tags", ReasonValue, "a natural attack has no tags")
	}
	return checkReach(e, false)
}

// naturalAttack is the compile-time check of a natural_attack.
func (x effectCompiler) naturalAttack() error {
	if p := checkNaturalAttack(x.e, x.c.isLiveDamageType); p.found() {
		return x.fail("%s", p.msg)
	}
	return nil
}

// checkNaturalAttackEffect checks a natural_attack of the table's content at its fields.
func (b *overlayBuilder) checkNaturalAttackEffect(site effectSite, e *Effect) error {
	if p := checkNaturalAttack(e, b.base.isLiveDamageType); p.found() {
		return site.fail(p.attr, p.reason, "%s", p.msg)
	}
	return nil
}

// naturalAttacks adds the attack lines the natural_attack effects give, in the order of the
// effects, after the pseudo-weapons.
func (x *deriver) naturalAttacks() {
	nth := map[string]int{}
	for _, a := range x.active {
		if a.effect.Type != "natural_attack" {
			continue
		}
		// The count takes every effect of the feature, gated off or not, so a line keeps its key while another switches.
		nth[a.owner]++
		if x.applies(a) {
			x.naturalAttack(a, nth[a.owner])
		}
	}
}

// naturalAttack adds the line of one natural_attack effect. n counts the effects of its feature,
// from 1, to tell the lines apart.
func (x *deriver) naturalAttack(a activeEffect, n int) {
	e := a.effect
	ab := Ability(e.Ability)
	if e.Ability == NaturalAbilityBest {
		ab = x.betterAbility(STR, DEX)
	}
	t := x.tuneWeapon(ab, weaponFacts{name: foldPT(strings.TrimSpace(e.AttackName)), melee: true})
	line := Attack{
		Key: NaturalAttackKey(a.owner, n), Name: e.AttackName, NamePT: e.AttackName, Kind: "weapon", Ability: t.attackAbility,
		AttackBonus: x.mods[t.attackAbility] + x.prof + t.attackBonus, Proficient: true,
		DamageType: e.DamageType, DamageTypeNamePT: x.c.namePT(e.DamageType),
		Melee: true, AbilityMod: x.mods[t.damageAbility],
		Damage: withModifier(strings.TrimSpace(e.DamageDice), x.mods[t.damageAbility]+t.damageBonus),
	}
	line.RangeFt = max(unarmedReachFt, e.ReachFt, t.reachFt)
	if line.RangeFt > unarmedReachFt {
		line.ReachFt = line.RangeFt
	}
	line.DamageDice, _ = ParseDice(line.Damage)
	x.d.Attacks = append(x.d.Attacks, line)
}

// NaturalAttackKey is the key of the attack line the n-th (from 1) natural_attack effect of the
// feature gives.
func NaturalAttackKey(owner string, n int) string {
	if n <= 1 {
		return "natural:" + owner
	}
	return "natural:" + owner + "#" + strconv.Itoa(n)
}
