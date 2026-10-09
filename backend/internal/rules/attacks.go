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
	c := x.c
	// Martial Arts holds only while the monk wears no armor and no shield.
	martialArts := x.hasHandler("monk.martial_arts") && x.armorCategory == "none" && !x.b.Shield
	martialDie := x.martialArtsDie()
	for i, key := range x.b.Weapons {
		eq, ok := c.equipment[key]
		if !ok || eq.Weapon == nil {
			x.issue(IssueUnknownKey, fmt.Sprintf("full.weapon_keys[%d]", i), "A arma escolhida não existe no conteúdo %s.", c.version)
			continue
		}
		w := eq.Weapon
		kind := w.Range // "melee" or "ranged"
		ab := STR
		if kind == "ranged" {
			ab = DEX
		}
		monkWeapon := martialArts && slices.Contains(w.Properties, "weapon-property:monk")
		if slices.Contains(w.Properties, "weapon-property:finesse") || monkWeapon {
			ab = STR
			if x.mods[DEX] > x.mods[STR] {
				ab = DEX
			}
		}
		if slices.Contains(w.Properties, "weapon-property:heavy") && x.race != nil && x.race.Size == "Small" {
			x.d.Hints = append(x.d.Hints, Hint{
				Source: key, Target: "attack.weapon." + kind, Targets: []string{"attack.weapon." + kind}, Mode: "disadvantage",
				TextPT: fmt.Sprintf("Desvantagem nas jogadas de ataque com %s: arma pesada para criaturas Pequenas.", strings.ToLower(c.namePT(key))),
			})
		}
		proficient := x.weaponProficient(key, w.Category)
		bonus := x.mods[ab]
		if proficient {
			bonus += x.prof
		}
		dmg := x.modifiers("damage.weapon."+kind, x.mods[ab])
		oneHanded := dmg
		if kind == "melee" && len(x.b.Weapons) == 1 && !slices.Contains(w.Properties, "weapon-property:two-handed") {
			oneHanded += x.oneMeleeWeaponBonus()
		}
		dice := w.Damage
		if monkWeapon && martialDie > 0 {
			dice = biggerDie(dice, martialDie)
		}
		a := Attack{
			Key: key, Name: eq.Name, NamePT: c.namePT(key), Kind: "weapon", Ability: ab,
			AttackBonus: x.modifiers("attack.weapon."+kind, bonus), Proficient: proficient,
			DamageType: w.DamageType, DamageTypeNamePT: c.namePT(w.DamageType),
			Melee: kind == "melee", MartialArts: monkWeapon, AbilityMod: x.mods[ab],
			Light: kind == "melee" && slices.Contains(w.Properties, "weapon-property:light"),
		}
		if dice != "" {
			a.Damage = withModifier(dice, oneHanded)
		}
		if w.TwoHandedDamage != "" {
			two := w.TwoHandedDamage
			if monkWeapon && martialDie > 0 {
				two = biggerDie(two, martialDie)
			}
			a.VersatileDamage = withModifier(two, dmg)
		}
		switch {
		case w.ThrowNormalFt > 0:
			a.RangeFt, a.LongRangeFt = w.ThrowNormalFt, w.ThrowLongFt
		default:
			a.RangeFt, a.LongRangeFt = w.NormalRangeFt, w.LongRangeFt
		}
		a.DamageDice, _ = ParseDice(a.Damage)
		a.VersatileDice, _ = ParseDice(a.VersatileDamage)
		x.d.Attacks = append(x.d.Attacks, a)
	}

	for _, key := range x.b.Cantrips {
		s, ok := c.spells[key]
		if !ok || s.Level != 0 || len(s.Damage) == 0 || len(s.Damage[0].AtCharacterLevel) == 0 {
			continue
		}
		sc := x.casterFor(s)
		if sc == nil {
			continue
		}
		dmg := x.modifiers("damage.spell."+strings.TrimPrefix(key, "spell:"), 0)
		dice := damageAt(s.Damage[0].AtCharacterLevel, x.d.TotalLevel)
		a := Attack{
			Key: key, Name: s.Name, NamePT: c.namePT(key), Kind: "spell", Ability: sc.Ability,
			Damage: withModifier(dice, dmg), SpellDice: dice,
			DamageType: s.Damage[0].DamageType, DamageTypeNamePT: c.namePT(s.Damage[0].DamageType),
			Proficient: true, RangeFt: feet(s.Range), Beams: beamsAt(key, x.d.TotalLevel),
		}
		switch {
		case s.AttackType != "":
			a.AttackBonus = sc.AttackBonus
		case s.SaveAbility != "":
			a.SaveDC = sc.SaveDC
			a.SaveAbility = Ability(s.SaveAbility)
		}
		a.DamageDice, _ = ParseDice(a.Damage)
		x.d.Attacks = append(x.d.Attacks, a)
	}

	// Last, so the weapons and cantrips stay the first lines of the sheet.
	x.unarmedStrike(martialArts, martialDie)
}

// wieldingOneMeleeWeapon is the tag of a damage bonus that holds while the
// character wields one melee weapon in one hand and no other weapon (the
// Dueling fighting style, SRD 5.1).
const wieldingOneMeleeWeapon = "wielding:one-melee-weapon"

// oneMeleeWeaponBonus is the sum of the damage bonuses that need one melee
// weapon wielded in one hand and no other weapon. The sheet lists the weapons
// carried, not the ones in hand, so the caller grants it only to the sole
// weapon of the sheet, in the damage it rolls in one hand; with other weapons
// carried, the bonus stays a Hint for the master to apply. An applied effect is
// recorded so effectHints does not repeat it.
func (x *deriver) oneMeleeWeaponBonus() int {
	total := 0
	for _, a := range x.active {
		e := a.effect
		if e.Type != "modifier" || e.Target != "damage.weapon.melee" || e.Mode != "add" || !slices.Equal(e.Tags, []string{wieldingOneMeleeWeapon}) || !x.applies(a) {
			continue
		}
		if v, ok := x.value(a); ok {
			total += v
			x.appliedTagged[e] = true
		}
	}
	return total
}

// unarmedReachFt is the reach of an unarmed strike.
const unarmedReachFt = 5

// unarmedStrike adds the unarmed strike line: STR (DEX too for a monk with
// Martial Arts), proficient, 1 + modifier bludgeoning, or the monk die plus
// modifier.
func (x *deriver) unarmedStrike(martialArts bool, martialDie int) {
	ab := STR
	if martialArts && x.mods[DEX] > x.mods[STR] {
		ab = DEX
	}
	mod := x.mods[ab]
	a := Attack{
		Key: UnarmedStrikeKey, Name: "Unarmed Strike", NamePT: x.c.namePT(UnarmedStrikeKey), Kind: "weapon", Ability: ab,
		AttackBonus: mod + x.prof, Proficient: true,
		DamageType: "damage-type:bludgeoning", DamageTypeNamePT: x.c.namePT("damage-type:bludgeoning"),
		Melee: true, RangeFt: unarmedReachFt, MartialArts: martialArts, AbilityMod: mod,
	}
	if martialArts && martialDie > 0 {
		a.Damage = withModifier("1d"+strconv.Itoa(martialDie), mod)
	} else {
		a.Damage = strconv.Itoa(max(1+mod, 0))
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
