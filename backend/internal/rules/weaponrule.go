package rules

import (
	"slices"
	"strconv"
	"strings"
	"unicode/utf8"
)

// The weapon_rule and pseudo_weapon effects.
//
// weapon_rule changes the weapon lines of the sheet that its filter chooses: another
// ability to attack or to hurt with (the best of the options applies, as a finesse
// weapon takes the better of Strength and Dexterity), a bonus to the attack or to the
// damage, a longer melee reach. It is read at the one place the sheet builds a weapon
// line (attacks.go), so the sheet, the attack roll and the damage roll agree.
//
// pseudo_weapon gives the sheet an attack line of its own (a built-in ranged attack
// that uses Intelligence, 1d6 damage): never an item, only a line in the attack list,
// proficient, which the weapon rules change like any weapon.

// The filters of a weapon_rule, and the kinds of a pseudo_weapon (the first two).
const (
	WeaponAny    = "any"
	WeaponMelee  = "melee"
	WeaponRanged = "ranged"
	// WeaponFinesse chooses the weapons with the finesse property.
	WeaponFinesse = "finesse"
	// WeaponMagic chooses the weapon lines the sheet marks as magical: a weapon the sheet's
	// weapon_marks mark magical and the unarmed strike of a monk with Ki-Empowered Strikes.
	WeaponMagic = "magic"
	// WeaponNamed chooses the weapon whose Portuguese name is WeaponName, ignoring case
	// and accents.
	WeaponNamed = "named"
)

var (
	weaponFilters = []string{WeaponAny, WeaponMelee, WeaponRanged, WeaponFinesse, WeaponMagic, WeaponNamed}
	weaponKinds   = []string{WeaponMelee, WeaponRanged}
	// pseudoWeaponSides are the dice a pseudo-weapon may roll: the weapon dice.
	pseudoWeaponSides = []int{4, 6, 8, 10, 12}
)

// Bounds of the weapon effects.
const (
	// MaxWeaponBonus and MaxWeaponPenalty bound the bonus (and the penalty) a weapon rule adds
	// to the attack roll or to the damage.
	MaxWeaponBonus   = 10
	MaxWeaponPenalty = 5
	// MaxWeaponReachFt bounds a melee reach, and MaxWeaponRangeFt a pseudo-weapon's range.
	MaxWeaponReachFt = 60
	MaxWeaponRangeFt = 300
	// maxPseudoDice is the most dice a pseudo-weapon rolls.
	maxPseudoDice = 10
	// maxPseudoNameRunes and maxWeaponNameRunes bound the names the effects carry.
	maxPseudoNameRunes = 40
	maxWeaponNameRunes = 60
)

// weaponProblem is what is wrong with a weapon effect: the field the editor points at,
// the reason code and a sentence.
type weaponProblem struct{ attr, reason, msg string }

func (p weaponProblem) found() bool { return p.attr != "" }

func problem(attr, reason, msg string) weaponProblem { return weaponProblem{attr, reason, msg} }

// checkWeaponRule says what is wrong with a weapon_rule, or the zero problem.
func checkWeaponRule(e *Effect) weaponProblem {
	switch {
	case !slices.Contains(weaponFilters, e.Weapon):
		return problem(".weapon", ReasonValue, "weapon is one of "+strings.Join(weaponFilters, ", "))
	case len(e.Tags) > 0:
		return problem(".tags", ReasonValue, "a weapon rule has no tags")
	}
	if p := checkWeaponName(e); p.found() {
		return p
	}
	if p := checkWeaponNumbers(e); p.found() {
		return p
	}
	if e.AttackAbility == "" && e.DamageAbility == "" && e.AttackBonus == 0 && e.DamageBonus == 0 && e.ReachFt == 0 && !e.Magical && !e.Silvered && !e.Adamantine {
		return problem(".type", ReasonValue, "a weapon rule changes nothing: give it an ability, a bonus, a reach or a mark")
	}
	return weaponProblem{}
}

// checkWeaponName checks the name a "named" filter carries: required for it, and for
// no other.
func checkWeaponName(e *Effect) weaponProblem {
	name := strings.TrimSpace(e.WeaponName)
	switch {
	case e.Weapon != WeaponNamed && e.WeaponName != "":
		return problem(".weapon_name", ReasonValue, "only the named filter takes a weapon name")
	case e.Weapon == WeaponNamed && name == "":
		return problem(".weapon_name", ReasonValue, "the named filter needs the weapon's name")
	case utf8.RuneCountInString(e.WeaponName) > maxWeaponNameRunes:
		return problem(".weapon_name", ReasonLimit, "a weapon name has at most "+strconv.Itoa(maxWeaponNameRunes)+" characters")
	}
	return weaponProblem{}
}

// checkWeaponNumbers checks the abilities, the bonuses and the reach of a weapon rule.
func checkWeaponNumbers(e *Effect) weaponProblem {
	for _, f := range []struct{ attr, value string }{{".attack_ability", e.AttackAbility}, {".damage_ability", e.DamageAbility}} {
		if _, ok := abilityIndex[Ability(f.value)]; f.value != "" && !ok {
			return problem(f.attr, ReasonValue, "unknown ability "+strconv.Quote(f.value))
		}
	}
	for _, f := range []struct {
		attr string
		n    int
	}{{".attack_bonus", e.AttackBonus}, {".damage_bonus", e.DamageBonus}} {
		if f.n > MaxWeaponBonus || f.n < -MaxWeaponPenalty {
			return problem(f.attr, ReasonLimit, "a bonus goes from -"+strconv.Itoa(MaxWeaponPenalty)+" to "+strconv.Itoa(MaxWeaponBonus))
		}
	}
	return checkReach(e, e.Weapon == WeaponRanged)
}

// checkReach checks a melee reach: on the 5 ft grid, within the limit, and not for a
// ranged weapon.
func checkReach(e *Effect, ranged bool) weaponProblem {
	switch {
	case e.ReachFt == 0:
		return weaponProblem{}
	case ranged:
		return problem(".reach_ft", ReasonValue, "a reach belongs to melee weapons")
	case e.ReachFt < 0 || e.ReachFt > MaxWeaponReachFt || e.ReachFt%feetStep != 0:
		return problem(".reach_ft", ReasonLimit, "a reach is 5 to "+strconv.Itoa(MaxWeaponReachFt)+" feet, in steps of 5")
	}
	return weaponProblem{}
}

// checkPseudoWeapon says what is wrong with a pseudo_weapon, or the zero problem.
// damageTypeOK says whether a key is a damage type of the content.
func checkPseudoWeapon(e *Effect, damageTypeOK func(string) bool) weaponProblem {
	name := strings.TrimSpace(e.AttackName)
	_, abilityOK := abilityIndex[Ability(e.Ability)]
	switch {
	case name == "" || utf8.RuneCountInString(e.AttackName) > maxPseudoNameRunes:
		return problem(".attack_name", ReasonValue, "an attack has a name of 1 to "+strconv.Itoa(maxPseudoNameRunes)+" characters")
	case !slices.Contains(weaponKinds, e.Weapon):
		return problem(".weapon", ReasonValue, "weapon is melee or ranged")
	case !pseudoDiceOK(e.DamageDice):
		return problem(".damage_dice", ReasonValue, "damage_dice is plain dice such as 1d6, up to "+strconv.Itoa(maxPseudoDice)+" dice of d4, d6, d8, d10 or d12")
	case !damageTypeOK(e.DamageType):
		return problem(".damage_type", ReasonReference, "damage_type is not a damage type")
	case !abilityOK:
		return problem(".ability", ReasonValue, "the attack needs an ability")
	case len(e.Tags) > 0:
		return problem(".tags", ReasonValue, "a pseudo-weapon has no tags")
	}
	return checkPseudoReach(e)
}

// checkPseudoReach checks the reach of a melee pseudo-weapon or the range of a ranged
// one: each has one, and neither has the other's.
func checkPseudoReach(e *Effect) weaponProblem {
	if e.Weapon == WeaponMelee {
		if e.RangeFt != 0 {
			return problem(".range_ft", ReasonValue, "a melee weapon has a reach, not a range")
		}
		return checkReach(e, false)
	}
	switch {
	case e.ReachFt != 0:
		return problem(".reach_ft", ReasonValue, "a reach belongs to melee weapons")
	case e.RangeFt <= 0 || e.RangeFt > MaxWeaponRangeFt || e.RangeFt%feetStep != 0:
		return problem(".range_ft", ReasonLimit, "a ranged weapon has a range of 5 to "+strconv.Itoa(MaxWeaponRangeFt)+" feet, in steps of 5")
	}
	return weaponProblem{}
}

// pseudoDiceOK says the text is plain dice a pseudo-weapon may roll.
func pseudoDiceOK(s string) bool {
	f, ok := plainDice(s)
	return ok && f.Count <= maxPseudoDice && slices.Contains(pseudoWeaponSides, f.Sides)
}

// weaponRule is the compile-time check of a weapon_rule: the table's menu has checked
// it at its fields already (checkEffect), so this only guards the SRD load.
func (x effectCompiler) weaponRule() error {
	if p := checkWeaponRule(x.e); p.found() {
		return x.fail("%s", p.msg)
	}
	return nil
}

// pseudoWeapon is the compile-time check of a pseudo_weapon.
func (x effectCompiler) pseudoWeapon() error {
	if p := checkPseudoWeapon(x.e, x.c.isLiveDamageType); p.found() {
		return x.fail("%s", p.msg)
	}
	return nil
}

// weaponFacts is what the filters of a weapon rule ask of a weapon line.
type weaponFacts struct {
	// name is the line's Portuguese name, folded (foldPT).
	name           string
	melee, finesse bool
	magical        bool
}

// matchesWeapon says whether the weapon rule chooses the line.
func (e *Effect) matchesWeapon(f weaponFacts) bool {
	switch e.Weapon {
	case WeaponAny:
		return true
	case WeaponMelee:
		return f.melee
	case WeaponRanged:
		return !f.melee
	case WeaponFinesse:
		return f.finesse
	case WeaponMagic:
		return f.magical
	case WeaponNamed:
		return foldPT(strings.TrimSpace(e.WeaponName)) == f.name
	}
	return false
}

// weaponTuning is what the weapon rules do to one weapon line.
type weaponTuning struct {
	// attackAbility and damageAbility are the best of the line's own ability and the ones
	// the rules offer.
	attackAbility, damageAbility Ability
	attackBonus, damageBonus     int
	// reachFt is the longest melee reach a rule gives, 0 when none is longer than the
	// usual 5 ft.
	reachFt int
}

// tuneWeapon works out what the rules that choose the line do to it. ability is the
// ability the line attacks and hurts with before the rules.
func (x *deriver) tuneWeapon(ability Ability, f weaponFacts) weaponTuning {
	t := weaponTuning{attackAbility: ability, damageAbility: ability}
	for _, a := range x.active {
		e := a.effect
		if e.Type != "weapon_rule" || !e.matchesWeapon(f) || !x.applies(a) {
			continue
		}
		if e.AttackAbility != "" {
			t.attackAbility = x.betterAbility(t.attackAbility, Ability(e.AttackAbility))
		}
		if e.DamageAbility != "" {
			t.damageAbility = x.betterAbility(t.damageAbility, Ability(e.DamageAbility))
		}
		t.attackBonus += e.AttackBonus
		t.damageBonus += e.DamageBonus
		if f.melee && e.ReachFt > unarmedReachFt {
			t.reachFt = max(t.reachFt, e.ReachFt)
		}
	}
	// The creature's own melee reach (the modifier target): a weapon with a longer reach keeps it.
	if f.melee {
		if reach := min(max(x.modifiers(ReachMeleeTarget, unarmedReachFt), unarmedReachFt), MaxWeaponReachFt); reach > unarmedReachFt {
			t.reachFt = max(t.reachFt, reach)
		}
	}
	return t
}

// betterAbility is the ability with the higher modifier; a tie keeps the first.
func (x *deriver) betterAbility(a, b Ability) Ability {
	if x.mods[b] > x.mods[a] {
		return b
	}
	return a
}

// pseudoWeapons adds the attack lines the pseudo_weapon effects give, in the order of
// the effects, after the carried weapons and the cantrips.
func (x *deriver) pseudoWeapons() {
	nth := map[string]int{}
	for _, a := range x.active {
		if a.effect.Type != "pseudo_weapon" {
			continue
		}
		// The count takes every effect of the feature, gated off or not, so a line keeps its key while another switches.
		nth[a.owner]++
		if x.applies(a) {
			x.pseudoWeapon(a, nth[a.owner])
		}
	}
}

// pseudoWeapon adds the line of one pseudo_weapon effect. n counts the effects of its
// feature, from 1, to tell the lines apart.
func (x *deriver) pseudoWeapon(a activeEffect, n int) {
	e := a.effect
	melee := e.Weapon == WeaponMelee
	ab := Ability(e.Ability)
	t := x.tuneWeapon(ab, weaponFacts{name: foldPT(strings.TrimSpace(e.AttackName)), melee: melee})
	prof := x.prof // a pseudo-weapon counts as a weapon the character is proficient with
	line := Attack{
		Key: PseudoWeaponKey(a.owner, n), Name: e.AttackName, NamePT: e.AttackName, Kind: "weapon", Ability: t.attackAbility,
		AttackBonus: x.mods[t.attackAbility] + prof + t.attackBonus, Proficient: true,
		DamageType: e.DamageType, DamageTypeNamePT: x.c.namePT(e.DamageType),
		Melee: melee, AbilityMod: x.mods[t.damageAbility], RangeFt: e.RangeFt,
	}
	if _, ok := plainDice(e.DamageDice); ok {
		line.Damage = withModifier(strings.TrimSpace(e.DamageDice), x.mods[t.damageAbility]+t.damageBonus)
	}
	if melee {
		line.RangeFt = max(unarmedReachFt, e.ReachFt, t.reachFt)
		if line.RangeFt > unarmedReachFt {
			line.ReachFt = line.RangeFt
		}
	}
	line.DamageDice, _ = ParseDice(line.Damage)
	x.d.Attacks = append(x.d.Attacks, line)
}

// PseudoWeaponKey is the key of the attack line the n-th (from 1) pseudo_weapon effect
// of the feature gives.
func PseudoWeaponKey(owner string, n int) string {
	if n <= 1 {
		return "pseudo:" + owner
	}
	return "pseudo:" + owner + "#" + strconv.Itoa(n)
}

// checkWeaponEffects checks the two weapon effects of the table's content at their fields.
func (b *overlayBuilder) checkWeaponEffects(site effectSite, e *Effect) error {
	var p weaponProblem
	switch e.Type {
	case "weapon_rule":
		p = checkWeaponRule(e)
	case "pseudo_weapon":
		p = checkPseudoWeapon(e, b.base.isLiveDamageType)
	}
	if p.found() {
		return site.fail(p.attr, p.reason, "%s", p.msg)
	}
	return nil
}
