package rules

import (
	"slices"
	"strconv"

	"github.com/PuraFome/meuRPG/backend/internal/rules/formula"
)

// The limits of the dice a table heal rolls.
const (
	maxHealDice  = 20
	minHealSides = 2
	maxHealSides = 100
)

// strayCosts refuses the fields of the costed action and of the hook on a type that
// does not read them: cost and heal go with grant_action and on_event, cost_max and
// value_per_point with grant_action, event, restore and heal_percent_max with on_event.
func (x effectCompiler) strayCosts() error {
	e := x.e
	action, hook := e.Type == "grant_action", e.Type == "on_event"
	switch {
	case !action && !hook && (e.Heal != "" || e.TempHP != "" || (e.Cost != "" && e.Type != "spell_modifier")):
		return x.fail("cost, heal and temp_hp belong to grant_action and on_event (cost to spell_modifier too)")
	case !action && (e.CostMax != "" || e.ValuePerPoint != ""):
		return x.fail("cost_max and value_per_point belong to grant_action")
	case !hook && (e.Event != "" || e.Restore != "" || e.HealPercentMax != 0):
		return x.fail("event, restore and heal_percent_max belong to on_event")
	}
	return nil
}

// grantAction checks an action the character takes: its economy, what it costs and
// the hit points it gives back.
func (x effectCompiler) grantAction() error {
	e := x.e
	if !slices.Contains(economies, e.Economy) {
		return x.fail("unknown economy %q", e.Economy)
	}
	if err := x.checkCosts(); err != nil {
		return err
	}
	if err := x.checkSpendRange(); err != nil {
		return err
	}
	if err := x.checkHealAndTemp(); err != nil {
		return err
	}
	if err := x.checkPerPoint(); err != nil {
		return err
	}
	return x.checkSlotAction()
}

// checkCosts compiles the cost and the cost_max of an action or a hook: they name
// the resource they spend, and a literal is a whole number of at least 1.
func (x effectCompiler) checkCosts() error {
	e := x.e
	for _, f := range []struct{ name, src string }{{"cost", e.Cost}, {"cost_max", e.CostMax}} {
		if f.src != "" && e.Resource == "" {
			return x.fail("%s needs the resource it spends", f.name)
		}
		if n, err := strconv.Atoi(f.src); err == nil && n < 1 {
			return x.fail("%s must be at least 1", f.name)
		}
	}
	var err error
	if e.cost, err = x.compile(e.Cost, formula.Int); err != nil {
		return err
	}
	e.costMax, err = x.compile(e.CostMax, formula.Int)
	return err
}

// checkSpendRange refuses a cost_max that is below the cost when both are numbers.
func (x effectCompiler) checkSpendRange() error {
	least, errLeast := strconv.Atoi(x.e.Cost)
	if x.e.Cost == "" {
		least, errLeast = 1, nil
	}
	most, errMost := strconv.Atoi(x.e.CostMax)
	if errLeast == nil && errMost == nil && most < least {
		return x.fail("cost_max is below cost")
	}
	return nil
}

// checkHealAndTemp checks the heal (dice, at least one die, the ability modifier in
// ability) and compiles the temporary hit points.
func (x effectCompiler) checkHealAndTemp() error {
	e := x.e
	if e.Heal != "" {
		if _, ok := parseHealDice(e.Heal); !ok {
			return x.fail("heal is dice such as 1d6 or 2d4+2, with at least one die")
		}
	}
	if e.Ability != "" {
		if _, ok := abilityIndex[Ability(e.Ability)]; !ok {
			return x.fail("unknown ability %q", e.Ability)
		}
		if e.Heal == "" {
			return x.fail("ability goes with a heal")
		}
	}
	var err error
	e.tempHP, err = x.compile(e.TempHP, formula.Int)
	return err
}

// parseHealDice reads the dice of a heal: one or more dice, no "+ MOD" (the ability
// field adds the modifier).
func parseHealDice(s string) (DiceFormula, bool) {
	d, ok := ParseDice(s)
	if !ok || d.AddsModifier || d.Count < 1 || d.Count > maxHealDice || d.Sides < minHealSides || d.Sides > maxHealSides {
		return DiceFormula{}, false
	}
	return d, true
}

// checkPerPoint checks value_per_point: it needs a range to choose from and a result
// to scale. Dice scale the heal, with its sides; a flat formula scales the temporary
// hit points, or the heal when there are none.
func (x effectCompiler) checkPerPoint() error {
	e := x.e
	if e.ValuePerPoint == "" {
		return nil
	}
	if e.CostMax == "" && !e.SpendsSlot {
		return x.fail("value_per_point needs cost_max (or spends_slot): nothing is chosen, nothing to scale")
	}
	if e.Heal == "" && e.TempHP == "" {
		return x.fail("value_per_point needs a heal or temp_hp to scale")
	}
	if per, ok := ParseDice(e.ValuePerPoint); ok && per.Count > 0 {
		heal, healOK := parseHealDice(e.Heal)
		if !healOK || per.AddsModifier || per.Sides != heal.Sides || per.Count > maxHealDice {
			return x.fail("value_per_point as dice needs a heal with the same sides")
		}
		return nil
	}
	var err error
	e.perPointFlat, err = x.compile(e.ValuePerPoint, formula.Int)
	return err
}

// onEvent checks a hook of the owner: the event, what it costs, and its result.
func (x effectCompiler) onEvent() error {
	e := x.e
	if !slices.Contains(eventKinds, e.Event) {
		return x.fail("unknown event %q", e.Event)
	}
	if err := x.checkHookResult(); err != nil {
		return err
	}
	if e.CostMax != "" || e.ValuePerPoint != "" {
		return x.fail("a hook has a fixed cost")
	}
	if err := x.checkCosts(); err != nil {
		return err
	}
	if err := x.checkHealAndTemp(); err != nil {
		return err
	}
	var err error
	e.value, err = x.compile(e.Value, formula.Int)
	return err
}

// checkHookResult checks what a hook gives: a share of the maximum is 1 to 100 and
// not alongside dice, there is at least one result, and a count goes with a restore.
func (x effectCompiler) checkHookResult() error {
	e := x.e
	switch {
	case e.HealPercentMax < 0 || e.HealPercentMax > MaxHealPercent:
		return x.fail("heal_percent_max is 1 to %d", MaxHealPercent)
	case e.HealPercentMax > 0 && e.Heal != "":
		return x.fail("heal_percent_max and heal are not both set")
	case e.Heal == "" && e.HealPercentMax == 0 && e.TempHP == "" && e.Restore == "":
		return x.fail("an on_event needs a result: heal, heal_percent_max, temp_hp or restore")
	case e.Value != "" && e.Restore == "":
		return x.fail("value goes with restore")
	}
	return nil
}
