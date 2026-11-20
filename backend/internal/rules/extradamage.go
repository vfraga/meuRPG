package rules

import (
	"cmp"
	"fmt"
	"slices"

	"github.com/PuraFome/meuRPG/backend/internal/rules/formula"
)

// What an extra_damage effect asks of the target (its `applies`).
const (
	// ExtraDamageAnyTarget adds the damage to a weapon hit on any creature.
	ExtraDamageAnyTarget = "any_target"
	// ExtraDamageLargerTarget adds it only when the target is of a larger size than the attacker.
	ExtraDamageLargerTarget = "larger_target"
)

// Which weapon attacks an extra_damage goes with (its `weapon`; empty is any).
const (
	ExtraDamageAnyWeapon    = "any"
	ExtraDamageMeleeWeapon  = "melee"
	ExtraDamageRangedWeapon = "ranged"
)

// How often an extra_damage may be added (its `frequency`; empty is once per turn).
const (
	ExtraDamageOncePerTurn = "once_per_turn"
	ExtraDamageEachHit     = "each_hit"
)

var (
	extraDamageApplies     = []string{ExtraDamageAnyTarget, ExtraDamageLargerTarget}
	extraDamageWeapons     = []string{ExtraDamageAnyWeapon, ExtraDamageMeleeWeapon, ExtraDamageRangedWeapon}
	extraDamageFrequencies = []string{ExtraDamageOncePerTurn, ExtraDamageEachHit}
	// extraDamageSides are the dice an extra_damage may roll.
	extraDamageSides = []int{4, 6, 8, 10, 12}
)

// MinExtraDamageSides and MaxExtraDamageSides bound the die of an extra_damage on the menu; only the
// dice of extraDamageSides are accepted between them.
const (
	MinExtraDamageSides = 4
	MaxExtraDamageSides = 12
)

// Bounds of the damage an extra_damage adds: a formula that gives anything else is ignored.
const (
	minExtraDamage = 1
	maxExtraDamage = 999
	// maxExtraDice is the most dice an extra_damage rolls.
	maxExtraDice = 20
)

// ExtraDamage is an extra_damage effect of a feature the character has: damage added to a
// weapon hit, at the player's choice, when the target is what Applies says. The engine is
// generic; the table names it.
type ExtraDamage struct {
	// Key is the feature that gives it, NamePT its name.
	Key, NamePT string
	// Amount is the fixed damage, worked out for the character; never doubled by a critical
	// hit. It is 0 when the effect only rolls dice.
	Amount int
	// Dice is the dice it rolls (Count d Sides), zero when the effect adds a fixed number only.
	Dice DiceFormula
	// NoCritDoubling says a critical hit does not double the dice.
	NoCritDoubling bool
	// DamageType is the "damage-type:" key of the damage; empty is the weapon's own type.
	DamageType string
	// Weapon is one of the ExtraDamage*Weapon constants and Frequency one of ExtraDamageOncePerTurn
	// and ExtraDamageEachHit, both always set.
	Weapon, Frequency string
	// Applies is one of the ExtraDamage* target constants.
	Applies string
	// Resource is the resource each use spends, one use each; empty when it only has the
	// frequency limit.
	Resource string
	// TextPT is the line the master reads.
	TextPT string
	// Line is the key of the one attack line it goes with, LinePact for the pact weapon, empty for every line
	// the Weapon filter chooses (LineMatches).
	Line string
}

// extraDamage checks the fields of an extra_damage effect.
func (x effectCompiler) extraDamage() error {
	e := x.e
	if !slices.Contains(extraDamageApplies, e.Applies) {
		return x.fail("unknown applies %q", e.Applies)
	}
	if e.Value == "" && e.Dice == "" {
		return x.fail("value is required: the damage added")
	}
	if _, msg := extraDamageShape(e); msg != "" {
		return x.fail("%s", msg)
	}
	if e.DamageType != "" && !x.c.isLiveDamageType(e.DamageType) {
		return x.fail("damage_type %q is not a damage type", e.DamageType)
	}
	var err error
	if e.value, err = x.compile(e.Value, formula.Int); err != nil {
		return err
	}
	if e.dice, err = x.compile(e.Dice, formula.Int); err != nil {
		return err
	}
	return x.line()
}

// extraDamageShape names the field of an extra_damage's closed values that is wrong and why
// (both "" when they are right): the die, the weapon and the frequency.
func extraDamageShape(e *Effect) (field, msg string) {
	switch {
	case e.Dice == "" && e.DiceSides != 0:
		return "dice", "dice_sides needs dice: the number of dice"
	case e.Dice != "" && e.DiceSides == 0:
		return "dice_sides", "dice needs dice_sides: the sides of the die"
	case e.DiceSides != 0 && !slices.Contains(extraDamageSides, e.DiceSides):
		return "dice_sides", fmt.Sprintf("dice_sides %d is not a die of the menu (4, 6, 8, 10 or 12)", e.DiceSides)
	case e.NoCritDoubling && e.Dice == "":
		return "no_crit_doubling", "no_crit_doubling needs dice"
	case e.Weapon != "" && !slices.Contains(extraDamageWeapons, e.Weapon):
		return "weapon", fmt.Sprintf("unknown weapon %q", e.Weapon)
	case e.Frequency != "" && !slices.Contains(extraDamageFrequencies, e.Frequency):
		return "frequency", fmt.Sprintf("unknown frequency %q", e.Frequency)
	}
	return "", ""
}

// extraDamages fills Derived.ExtraDamages from the extra_damage effects the character has.
// A resource the character does not have at this level leaves the effect out, as for a maneuver.
func (x *deriver) extraDamages() {
	for _, a := range x.active {
		e := a.effect
		if e.Type != "extra_damage" || !x.applies(a) {
			continue
		}
		if e.Resource != "" && !slices.ContainsFunc(x.d.Resources, func(r Resource) bool { return r.Key == e.Resource }) {
			continue
		}
		flat, dice, ok := x.extraDamageAmounts(e)
		if !ok {
			x.issue(IssueFormula, "", "Um efeito de %s foi ignorado: o dano extra não é válido.", x.c.namePT(a.owner))
			continue
		}
		x.d.ExtraDamages = append(x.d.ExtraDamages, ExtraDamage{
			Key: a.owner, NamePT: x.c.namePT(a.owner), Amount: flat, Dice: dice, NoCritDoubling: e.NoCritDoubling, DamageType: e.DamageType,
			Weapon: cmp.Or(e.Weapon, ExtraDamageAnyWeapon), Frequency: cmp.Or(e.Frequency, ExtraDamageOncePerTurn),
			Applies: e.Applies, Resource: e.Resource, TextPT: e.TextPT, Line: e.Line,
		})
	}
}

// extraDamageAmounts works out the fixed damage and the dice of an extra_damage for the
// character; ok is false when the formulas give nothing the table can use.
func (x *deriver) extraDamageAmounts(e *Effect) (flat int, dice DiceFormula, ok bool) {
	if e.value != nil {
		n, err := e.value.Int(x.env)
		if err != nil || n < 0 || n > maxExtraDamage {
			return 0, DiceFormula{}, false
		}
		flat = n
	}
	if e.dice != nil {
		n, err := e.dice.Int(x.env)
		if err != nil || n < 1 || n > maxExtraDice {
			return 0, DiceFormula{}, false
		}
		dice = DiceFormula{Count: n, Sides: e.DiceSides}
	}
	return flat, dice, flat >= minExtraDamage || dice.Count > 0
}

// checkExtraDamage checks an extra_damage of the table's content: what it asks of the target, the
// damage it adds and the resource it spends, which an entry or its class defines.
func (b *overlayBuilder) checkExtraDamage(site effectSite, e *Effect) error {
	if !slices.Contains(extraDamageApplies, e.Applies) {
		return site.fail(".applies", ReasonValue, "applies %q is not on the table's menu", e.Applies)
	}
	if e.Value == "" && e.Dice == "" {
		return site.fail(".value", ReasonValue, "the damage added needs a value or dice")
	}
	if field, msg := extraDamageShape(e); field != "" {
		return site.fail("."+field, ReasonValue, "%s", msg)
	}
	if e.DamageType != "" && !b.base.isLiveDamageType(e.DamageType) {
		return site.fail(".damage_type", ReasonReference, "damage_type %q is not a damage type", e.DamageType)
	}
	if e.Resource != "" && !b.resourceDefined(site.owner, e.Resource) {
		return site.fail(".resource", ReasonReference, "the extra damage spends %q, which no resource effect of this entry or its class defines", e.Resource)
	}
	return b.checkLine(site, e)
}

func extraDamageValues() []MenuValue {
	out := make([]MenuValue, 0, len(extraDamageApplies))
	for _, k := range extraDamageApplies {
		out = append(out, MenuValue{Key: k, NamePT: extraDamageNms[k][0], HintPT: extraDamageNms[k][1]})
	}
	return out
}

func extraDamageWeaponValues() []MenuValue {
	return namedMenu(extraDamageWeapons, func(k string) string { return extraDamageWeaponNms[k] })
}

func extraDamageFrequencyValues() []MenuValue {
	return namedMenu(extraDamageFrequencies, func(k string) string { return extraDamageFrequencyNms[k] })
}
