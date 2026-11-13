package combat

import (
	"slices"
	"strings"

	"github.com/PuraFome/meuRPG/backend/internal/rules"
)

// The steps that take a damage from what was rolled to what lands (SRD 5.1,
// "Damage Resistance and Vulnerability"): resistance and vulnerability apply
// after every other modifier, resistance first, so a damage halved by
// resistance and then doubled by vulnerability is the rolled number rounded
// down to an even half; multiple instances of resistance or vulnerability to the
// same type count as one; immunity takes the damage to 0. A resistance or immunity
// can be limited to attacks that are not magical (the stat blocks' "from nonmagical
// attacks"): it covers a damage only when the damage comes from an attack and the
// attack is not magical. The stat blocks' "from nonmagical weapons that aren't silvered"
// (or adamantine) is the same, and the attack's weapon must not be of that material either.

// The kinds of a Step.
const (
	StepResistance    = "resistance"
	StepVulnerability = "vulnerability"
	StepImmunity      = "immunity"
	// StepReduction is a flat amount taken once off a weapon attack's damage, before resistance and
	// vulnerability (those two apply last, after every other adjustment).
	StepReduction = "reduction"
)

// QualifierNonmagical limits a Modifier to the damage of attacks that are not magical.
const QualifierNonmagical = "nonmagical"

// QualifierNonmagicalUnlessSilvered limits a Modifier to the damage of nonmagical attacks
// that are not made with a silvered weapon.
const QualifierNonmagicalUnlessSilvered = "nonmagical-unless-silvered"

// QualifierNonmagicalUnlessAdamantine limits a Modifier to the damage of nonmagical attacks
// that are not made with an adamantine weapon.
const QualifierNonmagicalUnlessAdamantine = "nonmagical-unless-adamantine"

// Modifier is a resistance, vulnerability or immunity to some damage types, from
// one source (a race, a feature, a state, a stat block).
type Modifier struct {
	// Kind is a Step* constant.
	Kind string
	// Source is its key: "race:tiefling", "feature:rage", "monster:skeleton".
	Source string
	// DamageTypes are the damage type keys it covers; rules.DamageTypeAll covers every type.
	DamageTypes []string
	// Qualifier limits it: "" covers every damage of its types, QualifierNonmagical only
	// the damage of a nonmagical attack, and the two unless-qualifiers only that of a
	// nonmagical attack whose weapon is not silvered (or adamantine).
	Qualifier string `json:",omitempty"`
	// Hidden says the source is an effect the master keeps from the players: a player reads no name for it.
	Hidden bool `json:"hidden,omitempty"`
	// Amount is what a StepReduction takes off the damage; it covers every damage type of a weapon
	// attack (and, with QualifierNonmagical, only a weapon attack that is not magical).
	Amount int `json:",omitempty"`
}

// Origin says where a damage comes from, for the modifiers that depend on it. The zero
// Origin is a damage that is no attack: a trap, a hazard, a fall, the damage of an
// ongoing effect.
type Origin struct {
	// Attack says the damage is that of an attack (a weapon, a spell attack, the damage
	// of a spell).
	Attack bool `json:"attack,omitempty"`
	// Weapon says the attack is made with a weapon (or an unarmed strike), not with a spell: the
	// damage reductions cover only these.
	Weapon bool `json:"weapon,omitempty"`
	// Magical says the attack is magical: a spell, a magic weapon, a creature's attack
	// with the trait "Magic Weapons", an attack the sheet marks.
	Magical bool `json:"magical,omitempty"`
	// Silvered and Adamantine say the attack's weapon is of that material.
	Silvered   bool `json:"silvered,omitempty"`
	Adamantine bool `json:"adamantine,omitempty"`
}

// Covers says the modifier applies to a damage of this type from this origin.
func (m Modifier) Covers(damageType string, from Origin) bool {
	if m.Kind == StepReduction {
		return m.coversReduction(from)
	}
	if !slices.Contains(m.DamageTypes, damageType) && !slices.Contains(m.DamageTypes, rules.DamageTypeAll) {
		return false
	}
	switch m.Qualifier {
	case "":
		return true
	case QualifierNonmagical:
		return from.Attack && !from.Magical
	case QualifierNonmagicalUnlessSilvered:
		return from.Attack && !from.Magical && !from.Silvered
	case QualifierNonmagicalUnlessAdamantine:
		return from.Attack && !from.Magical && !from.Adamantine
	}
	return false
}

// coversReduction says a StepReduction applies to this origin: it covers every damage type of a
// weapon attack (only one that is not magical, with QualifierNonmagical).
func (m Modifier) coversReduction(from Origin) bool {
	if !from.Attack || !from.Weapon {
		return false
	}
	return m.Qualifier == "" || (m.Qualifier == QualifierNonmagical && !from.Magical)
}

// Step is one change to a damage: what it was before, and after, and where it came from.
type Step struct {
	// Kind is a Step* constant.
	Kind string
	// Sources are the keys of every source counted in this step: several sources of
	// the same kind for one damage type count once, and all are named.
	Sources    []string
	DamageType string
	Before     int
	After      int
	// Qualifier is the Qualifier of the last covering Modifier of the step, so it names the
	// kind of damage the step holds for when the sources share one. It is "" when any
	// covering Modifier is plain (covers every damage).
	Qualifier string
}

// Adjust takes a damage of one type through the modifiers that cover it. ignored
// holds the source keys the master told the app to leave out; from says where the
// damage comes from. It returns the damage that lands and the steps that changed it; a
// modifier that changes nothing is no step.
func Adjust(amount int, damageType string, mods []Modifier, ignored []string, from Origin) (int, []Step) {
	amount = max(amount, 0)
	if damageType == "" || amount == 0 {
		return amount, nil
	}
	covering := func(kind string) (sources []string, qualifier string) {
		plain := false
		for _, m := range mods {
			if m.Kind != kind || !m.Covers(damageType, from) || slices.Contains(ignored, m.Source) || slices.Contains(sources, m.Source) {
				continue
			}
			sources = append(sources, m.Source)
			plain = plain || m.Qualifier == ""
			qualifier = m.Qualifier
		}
		if plain {
			qualifier = ""
		}
		return sources, qualifier
	}
	amount, steps, spent := reduceFirst(amount, damageType, mods, ignored, from)
	if spent {
		return 0, steps
	}
	if src, q := covering(StepImmunity); len(src) > 0 {
		return 0, append(steps, Step{Kind: StepImmunity, Sources: src, DamageType: damageType, Before: amount, After: 0, Qualifier: q})
	}
	if src, q := covering(StepResistance); len(src) > 0 {
		after := amount / 2 // rounded down (Player's Handbook, "Rounding Down")
		steps = append(steps, Step{Kind: StepResistance, Sources: src, DamageType: damageType, Before: amount, After: after, Qualifier: q})
		amount = after
	}
	if src, q := covering(StepVulnerability); len(src) > 0 {
		after := amount * 2
		steps = append(steps, Step{Kind: StepVulnerability, Sources: src, DamageType: damageType, Before: amount, After: after, Qualifier: q})
		amount = after
	}
	return amount, steps
}

// reduceFirst takes the flat reductions off the damage before every other adjustment. It returns
// the damage left, the step (none when nothing is taken) and whether the damage is spent (0).
func reduceFirst(amount int, damageType string, mods []Modifier, ignored []string, from Origin) (int, []Step, bool) {
	step, ok := reduction(amount, damageType, mods, ignored, from)
	if !ok {
		return amount, nil, false
	}
	return step.After, []Step{step}, step.After == 0
}

// reduction is the step of the flat reductions on the damage of one type.
func reduction(amount int, damageType string, mods []Modifier, ignored []string, from Origin) (Step, bool) {
	step, after, ok := Reduce(map[string]int{damageType: amount}, mods, ignored, from)
	if !ok {
		return Step{}, false
	}
	step.DamageType, step.After = damageType, after[damageType]
	return step, true
}

// Reduce takes the flat reductions that cover the attack off its damage once: every covering
// amount adds (two entries of one source add too), each source is named once, and the total is
// taken from the types largest first (ties by the type key) until it is spent or the damage is
// 0. The step is of the whole attack, so it has no damage type. It reports false when nothing is
// taken.
func Reduce(byType map[string]int, mods []Modifier, ignored []string, from Origin) (Step, map[string]int, bool) {
	step := Step{Kind: StepReduction}
	reduce, plain := 0, false
	for _, m := range mods {
		if m.Kind != StepReduction || m.Amount <= 0 || !m.Covers("", from) || slices.Contains(ignored, m.Source) {
			continue
		}
		reduce += m.Amount
		if !slices.Contains(step.Sources, m.Source) {
			step.Sources = append(step.Sources, m.Source)
		}
		plain = plain || m.Qualifier == ""
		step.Qualifier = m.Qualifier
	}
	if plain {
		step.Qualifier = ""
	}
	types := make([]string, 0, len(byType))
	for t, n := range byType {
		step.Before += max(n, 0)
		types = append(types, t)
	}
	if reduce == 0 || step.Before == 0 {
		return Step{}, nil, false
	}
	slices.SortFunc(types, func(a, b string) int {
		if byType[a] != byType[b] {
			return byType[b] - byType[a]
		}
		return strings.Compare(a, b)
	})
	after := make(map[string]int, len(byType))
	for _, t := range types {
		taken := min(reduce, max(byType[t], 0))
		after[t] = max(byType[t], 0) - taken
		reduce -= taken
		step.After += after[t]
	}
	return step, after, true
}
