package rules

import (
	"encoding/json"
	"fmt"
	"slices"
	"strconv"
	"strings"

	"github.com/PuraFome/meuRPG/backend/internal/rules/srd51"
)

// UnarmedStrikeKey is the attack line every character has, whatever they carry.
const UnarmedStrikeKey = "attack:unarmed-strike"

// attacks computes one line per carried weapon, one per attack cantrip and,
// last, the unarmed strike.
//
// A weapon attacks with STR, or DEX if it is ranged; a finesse weapon takes
// the better of the two, melee or ranged, and so does a monk weapon while
// Martial Arts applies (no armor and no shield). The proficiency bonus is
// added with proficiency in the weapon or its category. The damage adds the
// same ability modifier. "attack.weapon.*" and "damage.weapon.*" effects add
// on top (the Archery style).
//
// The unarmed strike is always proficient and deals 1 + STR bludgeoning; with
// Martial Arts it rolls the monk die and takes the better of DEX and STR.
//
// A cantrip that deals damage uses the damage for the character's total
// level (Fire Bolt: 1d10, then 2d10 at level 5) and the spell attack or
// save DC of a class that has it on its list.
func (x *deriver) attacks() {
	// Martial Arts holds only while the monk wears no armor and no shield.
	martialArts := x.hasHandler("monk.martial_arts") && x.armorCategory == "none" && !x.b.Shield
	martialDie := x.martialArtsDie()
	for i, key := range x.b.Weapons {
		x.weaponAttack(i, key, martialArts, martialDie)
	}
	for _, key := range x.b.Cantrips {
		x.cantripAttack(key)
	}
	x.pseudoWeapons()
	// Last, so the weapons and cantrips stay the first lines of the sheet.
	x.unarmedStrike(martialArts, martialDie)
}

// weaponAttack adds the line of the i-th carried weapon, or an Issue when the
// key is not a weapon of the content.
func (x *deriver) weaponAttack(i int, key string, martialArts bool, martialDie int) {
	c := x.c
	eq, ok := c.equipment[key]
	if !ok || eq.Weapon == nil {
		x.issue(IssueUnknownKey, fmt.Sprintf("full.weapon_keys[%d]", i), "A arma escolhida não existe no conteúdo %s.", c.version)
		return
	}
	w := eq.Weapon
	kind := w.Range // "melee" or "ranged"
	has := func(property string) bool { return slices.Contains(w.Properties, "weapon-property:"+property) }
	monkWeapon := martialArts && has("monk")
	// The weapon rules offer other abilities: the best of the options attacks, and the best hurts.
	t := x.tuneWeapon(x.weaponAbility(kind, has("finesse") || monkWeapon), weaponFacts{
		name: foldPT(strings.TrimSpace(c.namePT(key))), melee: kind == "melee", finesse: has("finesse"),
	})
	if has("heavy") {
		x.heavyWeaponHint(key, kind)
	}
	proficient := x.weaponProficient(key, w.Category)
	bonus := x.mods[t.attackAbility]
	if proficient {
		bonus += x.prof
	}
	dmg := x.modifiers("damage.weapon."+kind, x.mods[t.damageAbility]) + t.damageBonus
	oneHanded, note := dmg, ""
	if kind == "melee" && !has("two-handed") {
		oneHanded, note = x.oneHandedDamage(dmg)
	}
	a := Attack{
		Key: key, Name: eq.Name, NamePT: c.namePT(key), Kind: "weapon", Ability: t.attackAbility,
		AttackBonus: x.modifiers("attack.weapon."+kind, bonus) + t.attackBonus, Proficient: proficient,
		DamageType: w.DamageType, DamageTypeNamePT: c.namePT(w.DamageType),
		Melee: kind == "melee", MartialArts: monkWeapon, AbilityMod: x.mods[t.damageAbility],
		Light:        kind == "melee" && has("light"),
		Finesse:      has("finesse"),
		TwoHanded:    has("two-handed"),
		DamageNotePT: note,
	}
	monkDie := 0
	if monkWeapon {
		monkDie = martialDie
	}
	if w.Damage != "" {
		a.Damage = withModifier(monkDice(w.Damage, monkDie), oneHanded)
	}
	if w.TwoHandedDamage != "" {
		a.VersatileDamage = withModifier(monkDice(w.TwoHandedDamage, monkDie), dmg)
	}
	a.RangeFt, a.LongRangeFt = weaponRange(w)
	if a.Melee && t.reachFt > 0 {
		a.ReachFt = t.reachFt // the weapon keeps its own range: a reach is not a throwing range
	}
	a.DamageDice, _ = ParseDice(a.Damage)
	a.VersatileDice, _ = ParseDice(a.VersatileDamage)
	x.d.Attacks = append(x.d.Attacks, a)
}

// weaponAbility is the ability a weapon attacks with: STR, or DEX if it is
// ranged; the better of the two when either may be used (finesse, or a monk
// weapon under Martial Arts).
func (x *deriver) weaponAbility(kind string, either bool) Ability {
	if either {
		if x.mods[DEX] > x.mods[STR] {
			return DEX
		}
		return STR
	}
	if kind == "ranged" {
		return DEX
	}
	return STR
}

// heavyWeaponHint reminds a Small character of the disadvantage with a heavy
// weapon.
func (x *deriver) heavyWeaponHint(key, kind string) {
	if x.d.Size != "Small" {
		return
	}
	x.d.Hints = append(x.d.Hints, Hint{
		Source: key, Target: "attack.weapon." + kind, Targets: []string{"attack.weapon." + kind}, Mode: "disadvantage",
		TextPT: fmt.Sprintf("Desvantagem nas jogadas de ataque com %s: arma pesada para criaturas Pequenas.", strings.ToLower(x.c.namePT(key))),
	})
}

// oneHandedDamage is the damage modifier of a melee weapon held in one hand,
// with the bonuses that need no other weapon (oneMeleeWeaponBonus), and the
// note that says so on the line ("" without such a bonus).
func (x *deriver) oneHandedDamage(dmg int) (int, string) {
	bonus, source := x.oneMeleeWeaponBonus()
	if source == "" {
		return dmg, ""
	}
	return dmg + bonus, fmt.Sprintf("Inclui %s de %s (sem outra arma na mão)", signed(bonus), x.c.namePT(source))
}

// monkDice is a weapon's damage dice with the Martial Arts die when it is
// bigger; monkDie is 0 when Martial Arts does not apply to the weapon.
func monkDice(dice string, monkDie int) string {
	if monkDie > 0 {
		return biggerDie(dice, monkDie)
	}
	return dice
}

// weaponRange is a weapon's normal and long range: the thrown range when it
// has one, else the ranged one (0 for a melee weapon).
func weaponRange(w *srd51.Weapon) (normal, long int) {
	if w.ThrowNormalFt > 0 {
		return w.ThrowNormalFt, w.ThrowLongFt
	}
	return w.NormalRangeFt, w.LongRangeFt
}

// cantripAttack adds the line of a cantrip that deals damage and that a class
// of the character casts; any other cantrip adds nothing.
func (x *deriver) cantripAttack(key string) {
	c := x.c
	s, ok := c.spells[key]
	if !ok || s.Level != 0 || len(s.Damage) == 0 || len(s.Damage[0].AtCharacterLevel) == 0 {
		return
	}
	sc := x.casterFor(s)
	if sc == nil {
		return
	}
	dmg := x.modifiers("damage.spell."+strings.TrimPrefix(key, "spell:"), 0)
	dice := damageAt(s.Damage[0].AtCharacterLevel, x.d.TotalLevel)
	a := Attack{
		Key: key, Name: s.Name, NamePT: c.namePT(key), Kind: "spell", Ability: sc.Ability, Magical: true,
		Damage: withModifier(dice, dmg), SpellDice: dice,
		DamageType: s.Damage[0].DamageType, DamageTypeNamePT: c.namePT(s.Damage[0].DamageType),
		Proficient: true, RangeFt: feet(s.Range), Beams: beamsAt(key, x.d.TotalLevel),
	}
	switch {
	case s.AttackType != "":
		a.AttackBonus = sc.AttackBonus
		// A ranged_attack effect may multiply the range of a spell that needs an
		// attack roll (a range in feet only: feet gives 0 for touch and self).
		a.RangeFt = x.d.SpellRangeFt(s.AttackType, RangeRanged, a.RangeFt)
	case s.SaveAbility != "":
		a.SaveDC = sc.SaveDC
		a.SaveAbility = Ability(s.SaveAbility)
	}
	a.DamageDice, _ = ParseDice(a.Damage)
	x.d.Attacks = append(x.d.Attacks, a)
}

// wieldingOneMeleeWeapon is the tag of a damage bonus that holds while the
// character wields one melee weapon in one hand and no other weapon (the
// Dueling fighting style, SRD 5.1).
const wieldingOneMeleeWeapon = "wielding:one-melee-weapon"

// oneMeleeWeaponBonus is the sum of the damage bonuses that need a melee weapon
// wielded in one hand and no other weapon, and the feature that gives the first
// of them ("" when there is none). The sheet lists the weapons carried, not the
// ones in hand, so the caller grants it to every one-handed melee weapon and
// writes the condition on the attack line, for the table to check. An applied
// effect is recorded so effectHints does not repeat it.
func (x *deriver) oneMeleeWeaponBonus() (total int, source string) {
	for _, a := range x.active {
		e := a.effect
		if e.Type != "modifier" || e.Target != "damage.weapon.melee" || e.Mode != "add" || !slices.Equal(e.Tags, []string{wieldingOneMeleeWeapon}) || !x.applies(a) {
			continue
		}
		if v, ok := x.value(a); ok {
			total += v
			x.appliedTagged[e] = true
			if source == "" {
				source = a.owner
			}
		}
	}
	return total, source
}

// unarmedReachFt is the reach of an unarmed strike.
const unarmedReachFt = 5

// kiEmpoweredStrikes is the monk feature (level 6) that makes the unarmed strike magical.
const kiEmpoweredStrikes = "feature:ki-empowered-strikes"

// unarmedStrike adds the unarmed strike line: STR (DEX too for a monk with
// Martial Arts), proficient, 1 + modifier bludgeoning, or the monk die plus
// modifier. With Ki-Empowered Strikes it counts as magical for the resistances
// and immunities to nonmagical attacks, and for nothing else.
func (x *deriver) unarmedStrike(martialArts bool, martialDie int) {
	ab := STR
	if martialArts && x.mods[DEX] > x.mods[STR] {
		ab = DEX
	}
	magical := HasFeature(*x.d, kiEmpoweredStrikes)
	namePT := x.c.namePT(UnarmedStrikeKey)
	t := x.tuneWeapon(ab, weaponFacts{name: foldPT(strings.TrimSpace(namePT)), melee: true, magical: magical})
	mod := x.mods[t.damageAbility]
	a := Attack{
		Key: UnarmedStrikeKey, Name: "Unarmed Strike", NamePT: namePT, Kind: "weapon", Ability: t.attackAbility,
		AttackBonus: x.mods[t.attackAbility] + x.prof + t.attackBonus, Proficient: true,
		DamageType: "damage-type:bludgeoning", DamageTypeNamePT: x.c.namePT("damage-type:bludgeoning"),
		Melee: true, RangeFt: max(unarmedReachFt, t.reachFt), MartialArts: martialArts, AbilityMod: mod,
		Magical: magical,
	}
	if t.reachFt > 0 {
		a.ReachFt = t.reachFt
	}
	if martialArts && martialDie > 0 {
		a.Damage = withModifier("1d"+strconv.Itoa(martialDie), mod+t.damageBonus)
	} else {
		a.Damage = strconv.Itoa(max(1+mod+t.damageBonus, 0))
	}
	a.DamageDice, _ = ParseDice(a.Damage)
	x.d.Attacks = append(x.d.Attacks, a)
}

// beamsAt is how many attack rolls a cantrip makes in one action at a character
// level: Eldritch Blast fires one beam, and two, three and four from levels 5,
// 11 and 17. Every other cantrip makes one.
func beamsAt(key string, level int) int {
	if key != "spell:eldritch-blast" {
		return 1
	}
	beams := 1
	for _, step := range eldritchBlastBeams {
		if level >= step.level {
			beams = step.beams
		}
	}
	return beams
}

// eldritchBlastBeams is the beams Eldritch Blast fires from each character level.
var eldritchBlastBeams = []struct{ level, beams int }{{5, 2}, {11, 3}, {17, 4}}

// casterFor picks the Spellcasting of a class that has the spell on its
// list, or the first one.
func (x *deriver) casterFor(s *srd51.Spell) *Spellcasting {
	for i := range x.d.Spellcasting {
		if x.c.onList(s, x.d.Spellcasting[i].SpellList) {
			return &x.d.Spellcasting[i]
		}
	}
	if len(x.d.Spellcasting) > 0 {
		return &x.d.Spellcasting[0]
	}
	return nil
}

// martialArtsDie is the monk's Martial Arts die at its level (4 for d4), or
// 0, from the class table's martial_arts column.
func (x *deriver) martialArtsDie() int {
	for _, oc := range x.classes {
		raw := x.c.classLevels[oc.key][oc.level-1].ClassSpecific
		if len(raw) == 0 {
			continue
		}
		var cs struct {
			MartialArts *struct {
				DiceValue int `json:"dice_value"`
			} `json:"martial_arts"`
		}
		if json.Unmarshal(raw, &cs) == nil && cs.MartialArts != nil {
			return cs.MartialArts.DiceValue
		}
	}
	return 0
}

// biggerDie returns "1d<die>" when it beats the weapon's "1d<n>".
func biggerDie(dice string, die int) string {
	count, faces, ok := parseDice(dice)
	if !ok || count != 1 || faces >= die {
		return dice
	}
	return "1d" + strconv.Itoa(die)
}

func parseDice(dice string) (count, faces int, ok bool) {
	n, f, found := strings.Cut(strings.TrimSpace(dice), "d")
	if !found {
		return 0, 0, false
	}
	count, err1 := strconv.Atoi(n)
	faces, err2 := strconv.Atoi(f)
	return count, faces, err1 == nil && err2 == nil
}

// withModifier writes dice and a modifier as the sheet shows them: "1d6+1",
// "1d4-1", or just "1d10".
func withModifier(dice string, mod int) string {
	switch {
	case mod > 0:
		return fmt.Sprintf("%s+%d", dice, mod)
	case mod < 0:
		return fmt.Sprintf("%s%d", dice, mod)
	}
	return dice
}

// damageAt picks the damage for a character level from a cantrip's table,
// whose keys are the levels where it grows ("1", "5", "11", "17").
func damageAt(table map[string]string, level int) string {
	best, bestLevel := "", 0
	for k, v := range table {
		l, err := strconv.Atoi(k)
		if err != nil || l > level || l < bestLevel {
			continue
		}
		best, bestLevel = v, l
	}
	return best
}

// feet reads the number of an SRD range such as "120 feet"; "Touch", "Self"
// and the like give 0.
func feet(r string) int {
	n, _, _ := strings.Cut(r, " ")
	v, err := strconv.Atoi(n)
	if err != nil {
		return 0
	}
	return v
}
