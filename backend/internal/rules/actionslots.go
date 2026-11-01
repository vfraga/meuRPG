package rules

import (
	"fmt"
	"strconv"

	"github.com/PuraFome/meuRPG/backend/internal/rules/formula"
)

// The spell slots a table's effects move (MR-025): an action that turns the uses of a resource into a
// slot (gives_slot), an action that spends a slot to heal (spends_slot), and the recovery of expended
// slots after a rest (slot_recovery, the shape of the SRD's Arcane Recovery and Natural Recovery).

// checkSlotAction checks gives_slot and spends_slot on a grant_action. gives_slot spends the uses of a
// resource and gives a slot: it takes nothing that heals or scales. spends_slot spends a slot instead of
// a resource and needs the heal it scales by level; the player picks the level, so it takes no cost.
func (x effectCompiler) checkSlotAction() error {
	e := x.e
	switch {
	case e.GivesSlot == "" && !e.SpendsSlot:
		return nil
	case e.GivesSlot != "" && e.SpendsSlot:
		return x.fail("gives_slot and spends_slot are not both set")
	case e.GivesSlot != "":
		return x.checkGivesSlot()
	}
	return x.checkSpendsSlot()
}

func (x effectCompiler) checkGivesSlot() error {
	e := x.e
	if e.Resource == "" {
		return x.fail("gives_slot needs the resource whose uses it turns into a slot")
	}
	if e.CostMax != "" || e.Heal != "" || e.TempHP != "" || e.ValuePerPoint != "" {
		return x.fail("gives_slot takes no cost_max, heal, temp_hp or value_per_point")
	}
	if err := x.slotLevelLiteral(e.GivesSlot, "gives_slot"); err != nil {
		return err
	}
	var err error
	e.givesSlot, err = x.compile(e.GivesSlot, formula.Int)
	return err
}

func (x effectCompiler) checkSpendsSlot() error {
	e := x.e
	switch {
	case e.Resource != "" || e.Cost != "" || e.CostMax != "":
		return x.fail("spends_slot spends a spell slot: it takes no resource, cost or cost_max")
	case e.Heal == "":
		return x.fail("spends_slot needs a heal")
	case e.TempHP != "":
		return x.fail("spends_slot takes no temp_hp")
	}
	return nil
}

// slotLevelLiteral refuses a literal that is not a spell level.
func (x effectCompiler) slotLevelLiteral(src, name string) error {
	if n, err := strconv.Atoi(src); err == nil && (n < 1 || n > MaxSlotLevel) {
		return x.fail("%s is a spell level, 1 to %d", name, MaxSlotLevel)
	}
	return nil
}

// slotRecovery checks the effect that takes expended slots back after a rest.
func (x effectCompiler) slotRecovery() error {
	if !x.takesOnly(func(o *Effect) {
		o.Type, o.Resource, o.RecoverMaxLevel, o.RecoverLevels, o.When, o.TextPT = "", "", "", "", "", ""
		o.recoverMaxLevel, o.recoverLevels, o.when = nil, nil, nil
	}) {
		return x.fail("slot_recovery takes resource, recover_max_level, recover_levels, when and text_pt only")
	}
	e := x.e
	if e.Resource == "" || e.RecoverMaxLevel == "" || e.RecoverLevels == "" {
		return x.fail("slot_recovery needs a resource, a recover_max_level and recover_levels")
	}
	if err := x.slotLevelLiteral(e.RecoverMaxLevel, "recover_max_level"); err != nil {
		return err
	}
	if n, err := strconv.Atoi(e.RecoverLevels); err == nil && n < 1 {
		return x.fail("recover_levels must be at least 1")
	}
	var err error
	if e.recoverMaxLevel, err = x.compile(e.RecoverMaxLevel, formula.Int); err != nil {
		return err
	}
	e.recoverLevels, err = x.compile(e.RecoverLevels, formula.Int)
	return err
}

// TableSlotRecovery is a table feature's way of taking expended spell slots back (a slot_recovery effect), for
// the character's level: the resource a recovery spends a use of, the highest level of a slot it takes back
// and the combined level of the slots it takes back. The SRD's own Arcane Recovery is not one.
type TableSlotRecovery struct {
	// Source is the key of the feature that has it.
	Source   string
	Resource string
	// MaxLevel is the highest slot level, Levels the combined level (a level 2 slot counts 2).
	MaxLevel int
	Levels   int
}

// Check checks the slots a recovery takes back, by spell level and count: each level within MaxLevel and the
// combined level within Levels. It returns the combined level.
func (r TableSlotRecovery) Check(slots map[int]int) (int, error) {
	total := 0
	for level, count := range slots {
		if level < 1 || level > r.MaxLevel {
			return 0, errSlotRecovery("a slot of level %d cannot be recovered", level)
		}
		if count < 1 {
			return 0, errSlotRecovery("the count of the level %d slots must be at least 1", level)
		}
		total += level * count
	}
	if total > r.Levels {
		return total, errSlotRecovery("the slots add up to level %d and the most is %d", total, r.Levels)
	}
	return total, nil
}

// slotRecoveries fills Derived.TableRecoveries from the slot_recovery effects the character has. The
// limits of two effects do not merge: each is its own entry, and a feature key reads the first.
func (x *deriver) slotRecoveries() {
	for _, a := range x.active {
		if a.effect.Type != "slot_recovery" || !x.applies(a) {
			continue
		}
		maxLevel, ok := x.evalInt(a.effect.recoverMaxLevel, a)
		levels, ok2 := x.evalInt(a.effect.recoverLevels, a)
		if !ok || !ok2 {
			continue
		}
		x.d.TableRecoveries = append(x.d.TableRecoveries, TableSlotRecovery{
			Source: a.owner, Resource: a.effect.Resource, MaxLevel: min(max(maxLevel, 0), MaxSlotLevel), Levels: max(levels, 0),
		})
	}
}

// slotAction sets what a grant_action does with spell slots on the action: the slot it gives (its
// level, for the character) or the slot it spends. A spends_slot action spends no resource, even the one
// of its own feature. It returns false when the action cannot be: its level formula failed.
func (x *deriver) slotAction(act *Action, a activeEffect) bool {
	e := a.effect
	switch {
	case e.SpendsSlot:
		act.Resource, act.SpendsSlot = "", true
	case e.givesSlot != nil:
		n, ok := x.evalInt(e.givesSlot, a)
		if !ok {
			return false
		}
		if n < 1 || n > MaxSlotLevel {
			return false // a level outside 1 to 9 gives no slot: no action
		}
		act.GivesSlot = n
	}
	return true
}

// errSlotRecovery is a refusal of the slots a recovery takes back; it wraps ErrSlotRecovery, the base
// of the recovery refusals.
func errSlotRecovery(format string, args ...any) error {
	return fmt.Errorf("%w: "+format, append([]any{ErrSlotRecovery}, args...)...)
}
