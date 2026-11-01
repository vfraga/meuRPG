package rules

import (
	"errors"
	"fmt"
	"slices"

	"github.com/PuraFome/meuRPG/backend/internal/rules/formula"
)

// ErrSpend is the base of the errors of a spend outside what an action allows.
var ErrSpend = errors.New("spend")

// ActionResult is what one use of a grant_action gives, at its base cost. Spending
// more points (Action.CostMax) adds the per-point parts. Heal and TempHP follow the
// rules of the SRD spells (combat.ApplyHeal, combat.ApplyTempHP): the heal never
// passes the maximum, a dead creature is not healed and temporary hit points do not
// stack.
type ActionResult struct {
	// Heal is the dice healed, the ability modifier already in Bonus; nil when the
	// action heals nothing.
	Heal *DiceFormula `json:",omitempty"`
	// TempHP is the temporary hit points given; 0 when there are none.
	TempHP int `json:",omitempty"`
	// HealPerPoint is what each point spent above the base cost adds to Heal (the
	// same sides), TempHPPerPoint what it adds to TempHP.
	HealPerPoint   DiceFormula `json:",omitzero"`
	TempHPPerPoint int         `json:",omitempty"`
}

// Outcome is the result of one use that spent some points.
type Outcome struct {
	// Heal is the dice to roll, nil when there is no heal.
	Heal *DiceFormula
	// TempHP is the temporary hit points to give, 0 for none.
	TempHP int
}

// UnitCost is how many uses one use of the action spends at least: 0 for an
// action with no resource, otherwise the cost (1 when it has none).
func (a Action) UnitCost() int {
	if a.SpendsSlot {
		return 1 // the lowest slot level: the heal grows from there
	}
	if a.Resource == "" {
		return 0
	}
	return max(a.Cost, 1)
}

// ChoosesSpend says the player picks how many points to spend (CostMax above the
// cost).
func (a Action) ChoosesSpend() bool {
	return a.Resource != "" && a.CostMax > a.UnitCost()
}

// SpendRange is the least and the most the player may spend with left uses of the
// resource left; ok is false when fewer than the cost are left (the action is off).
func (a Action) SpendRange(left int) (lo, hi int, ok bool) {
	lo = a.UnitCost()
	if left < lo {
		return lo, lo, false
	}
	hi = lo
	if a.ChoosesSpend() {
		hi = min(a.CostMax, left)
	}
	return lo, hi, true
}

// Spend validates the number the player picked (0 takes the least) against the
// range and returns what to spend; ErrSpend otherwise. The server calls it with the
// uses left read in the same transaction as the use, never trusting the client with
// more than the number picked.
func (a Action) Spend(chosen, left int) (int, error) {
	lo, hi, ok := a.SpendRange(left)
	switch {
	case !ok:
		return 0, fmt.Errorf("%w: %d left, the action costs %d", ErrSpend, left, lo)
	case chosen == 0:
		return lo, nil
	case chosen < lo || chosen > hi:
		return 0, fmt.Errorf("%w: %d is outside %d to %d", ErrSpend, chosen, lo, hi)
	}
	return chosen, nil
}

// Outcome is what spending spent points gives: the base result plus the per-point
// part for each point above the cost. The heal's dice are those of the base with the
// per-point dice added to the count (same sides, checked when the effect is written).
func (a Action) Outcome(spent int) Outcome {
	extra := max(spent-a.UnitCost(), 0)
	r := a.Result
	out := Outcome{TempHP: r.TempHP}
	if r.TempHP > 0 || r.TempHPPerPoint > 0 {
		out.TempHP = r.TempHP + extra*r.TempHPPerPoint
	}
	if r.Heal != nil {
		h := *r.Heal
		h.Count += extra * r.HealPerPoint.Count
		h.Bonus += extra * r.HealPerPoint.Bonus
		out.Heal = &h
	}
	return out
}

// costAndResult fills the cost, the range and the result of an action from its
// grant_action effect, for the character's level and abilities. A formula that fails
// is an Issue and leaves the part out; a cost below 1 counts as 1, and a cost_max that
// is not above the cost, or that the resource can never reach, leaves the cost fixed.
func (x *deriver) costAndResult(act *Action, a activeEffect) {
	e := a.effect
	if act.Resource == "" {
		x.actionResult(act, a)
		return
	}
	if e.cost != nil {
		if n, ok := x.evalInt(e.cost, a); ok {
			act.Cost = max(n, 1)
		}
	}
	if e.costMax != nil {
		if n, ok := x.evalInt(e.costMax, a); ok {
			act.CostMax = n
		}
	}
	if i := slicesIndexResource(x.d.Resources, act.Resource); i >= 0 {
		act.CostMax = min(act.CostMax, x.d.Resources[i].Max)
	}
	if act.CostMax <= act.UnitCost() {
		act.CostMax = 0
	}
	x.actionResult(act, a)
}

// actionResult fills the heal and the temporary hit points of an action.
func (x *deriver) actionResult(act *Action, a activeEffect) {
	e := a.effect
	r := &act.Result
	if e.Heal != "" {
		if d, ok := parseHealDice(e.Heal); ok {
			d.Bonus += x.mods[Ability(e.Ability)]
			r.Heal = &d
		}
	}
	if e.tempHP != nil {
		if n, ok := x.evalInt(e.tempHP, a); ok {
			r.TempHP = max(n, 0)
		}
	}
	if e.ValuePerPoint == "" || (act.CostMax == 0 && !act.SpendsSlot) {
		return
	}
	if per, ok := ParseDice(e.ValuePerPoint); ok && per.Count > 0 {
		r.HealPerPoint = per
		return
	}
	n, ok := x.evalInt(e.perPointFlat, a)
	switch {
	case !ok:
	case e.TempHP != "":
		r.TempHPPerPoint = max(n, 0)
	case r.Heal != nil:
		r.HealPerPoint = DiceFormula{Bonus: n}
	}
}

// evalInt runs an Int formula of an effect; a failure is an Issue.
func (x *deriver) evalInt(p *formula.Program, a activeEffect) (int, bool) {
	n, err := p.Int(x.env)
	if err != nil {
		x.issue(IssueFormula, "", "Um efeito de %s foi ignorado: a fórmula falhou.", x.c.namePT(a.owner))
		return 0, false
	}
	return n, true
}

func slicesIndexResource(rs []Resource, key string) int {
	return slices.IndexFunc(rs, func(r Resource) bool { return r.Key == key })
}
