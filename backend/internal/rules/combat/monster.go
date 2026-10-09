package combat

import (
	"errors"
	"slices"
	"strconv"

	"github.com/PuraFome/meuRPG/backend/internal/rules"
)

// What a monster spends of its stat block in a combat (SRD 5.1, "Monsters": actions;
// "Legendary Creatures"), as pure functions over MonsterState. The play module stores
// the state as JSON on the combatant and calls these; nothing here reads a database or
// rolls a die (the caller passes the d6 of a recharge).

// The reasons an action cannot be used.
var (
	// ErrMonsterRecharging: the action waits for its recharge.
	ErrMonsterRecharging = errors.New("combat: the action is recharging")
	// ErrNoMonsterUses: the uses of the action are spent.
	ErrNoMonsterUses = errors.New("combat: no uses left")
	// ErrNoLegendaryActions: fewer legendary actions are left than the option costs.
	ErrNoLegendaryActions = errors.New("combat: not enough legendary actions left")
	// ErrNoLegendaryResistance: Legendary Resistance is spent.
	ErrNoLegendaryResistance = errors.New("combat: no Legendary Resistance left")
	// ErrNotARoutine: the Multiattack action has no such routine.
	ErrNotARoutine = errors.New("combat: not a routine of the Multiattack action")
)

// MonsterState is what a monster has spent. The zero value is a monster with everything
// available.
type MonsterState struct {
	// Recharging are the keys of the actions that wait for their recharge.
	Recharging []string `json:"recharging,omitempty"`
	// Used are the uses spent of the actions with a number of uses (x/day, a rest) and of
	// the innate spells, by action key or spell key (a group that shares its uses has the
	// key of InnateGroupKey).
	Used map[string]int `json:"used,omitempty"`
	// LegendaryUsed are the legendary actions taken since the creature's last turn began.
	LegendaryUsed int `json:"legendary_used,omitempty"`
	// ResistanceUsed are the uses of Legendary Resistance spent.
	ResistanceUsed int `json:"resistance_used,omitempty"`
	// SlotsUsed[l-1] are the spell slots of level l spent.
	SlotsUsed []int `json:"slots_used,omitempty"`
	// Multiattack is the routine of this turn and what is left of it.
	Multiattack *MultiattackState `json:"multiattack,omitempty"`
	// RechargeRolls are the d6 the server rolled at the start of the creature's last turn, one for
	// each action that waited for its recharge: the master reads them (and the log keeps them).
	RechargeRolls []RechargeFace `json:"recharge_rolls,omitempty"`
	// Offer is the chance to take a legendary action that the end of another creature's turn
	// opened: it closes when one is taken, when the master lets it pass and when a turn begins.
	Offer *LegendaryOffer `json:"offer,omitempty"`
	// Prompts are the failed saving throws that wait for the master's word on Legendary
	// Resistance, the most recent last; at most MaxPrompts.
	Prompts []ResistPrompt `json:"prompts,omitempty"`
}

// MaxPrompts is how many failed saves wait for an answer at once (the oldest is let go).
const MaxPrompts = 3

// RechargeFace is the d6 of one action's recharge roll.
type RechargeFace struct {
	Action string `json:"action"`
	Face   int    `json:"face"`
}

// LegendaryOffer is an open chance to take a legendary action, after the turn of After (a
// combatant ID).
type LegendaryOffer struct {
	After string `json:"after"`
}

// ResistPrompt is a saving throw the creature failed that the master may turn into a success
// with Legendary Resistance: the cast it was made for, who cast it, the roll and the damage of the
// cast that is still to be rolled (the prompt ends when it is).
type ResistPrompt struct {
	Cast    string   `json:"cast"`
	Spell   string   `json:"spell"`
	Caster  string   `json:"caster,omitempty"`
	Ability string   `json:"ability"`
	DC      int      `json:"dc"`
	D20     int      `json:"d20"`
	Bonus   int      `json:"bonus"`
	Total   int      `json:"total"`
	Pending []string `json:"pending,omitempty"`
}

// AddPrompt writes down a failed saving throw to answer, dropping the oldest beyond MaxPrompts.
func (s *MonsterState) AddPrompt(p ResistPrompt) {
	s.Prompts = append(s.Prompts, p)
	if len(s.Prompts) > MaxPrompts {
		s.Prompts = s.Prompts[len(s.Prompts)-MaxPrompts:]
	}
}

// DropPrompt forgets the prompt of a cast.
func (s *MonsterState) DropPrompt(cast string) {
	s.Prompts = slices.DeleteFunc(s.Prompts, func(p ResistPrompt) bool { return p.Cast == cast })
}

// MultiattackState is a Multiattack routine and the attacks of it still to be made, by
// step (the order of the routine).
type MultiattackState struct {
	Action  string `json:"action"`
	Routine int    `json:"routine"`
	Left    []int  `json:"left"`
}

// InnateGroupKey is the key of the uses a group of innate spells shares ("3/day: enlarge/
// reduce, tongues"): the spellcasting trait's key and the group's place in it.
func InnateGroupKey(traitKey string, group int) string {
	return traitKey + "@" + strconv.Itoa(group)
}

// Clone copies the state, so a change never touches the one it was read from.
func (s MonsterState) Clone() MonsterState {
	s.Recharging = slices.Clone(s.Recharging)
	if s.Used != nil {
		used := make(map[string]int, len(s.Used))
		for k, v := range s.Used {
			used[k] = v
		}
		s.Used = used
	}
	s.SlotsUsed = slices.Clone(s.SlotsUsed)
	s.RechargeRolls = slices.Clone(s.RechargeRolls)
	s.Prompts = slices.Clone(s.Prompts)
	if s.Offer != nil {
		o := *s.Offer
		s.Offer = &o
	}
	if s.Multiattack != nil {
		m := *s.Multiattack
		m.Left = slices.Clone(m.Left)
		s.Multiattack = &m
	}
	return s
}

// IsRecharging says whether the action waits for its recharge.
func (s MonsterState) IsRecharging(key string) bool { return slices.Contains(s.Recharging, key) }

// UsesLeft is how many uses an action with a number of uses has left; -1 for an action
// with no such limit.
func (s MonsterState) UsesLeft(usage rules.ActionUsage, key string) int {
	switch usage.Kind {
	case rules.UsagePerDay:
		return max(usage.Uses-s.Used[key], 0)
	case rules.UsageRest:
		return max(1-s.Used[key], 0)
	}
	return -1
}

// CanUse says whether an action can be used now as far as its limit goes: the error is
// ErrMonsterRecharging or ErrNoMonsterUses.
func (s MonsterState) CanUse(usage rules.ActionUsage, key string) error {
	switch usage.Kind {
	case rules.UsageRecharge:
		if s.IsRecharging(key) {
			return ErrMonsterRecharging
		}
	case rules.UsagePerDay, rules.UsageRest:
		if s.UsesLeft(usage, key) == 0 {
			return ErrNoMonsterUses
		}
	}
	return nil
}

// Spend uses an action: a recharging action starts to wait for its recharge, an action
// with a number of uses loses one. It changes the state only when it succeeds.
func (s *MonsterState) Spend(usage rules.ActionUsage, key string) error {
	if err := s.CanUse(usage, key); err != nil {
		return err
	}
	switch usage.Kind {
	case rules.UsageRecharge:
		s.Recharging = append(s.Recharging, key)
	case rules.UsagePerDay, rules.UsageRest:
		if s.Used == nil {
			s.Used = map[string]int{}
		}
		s.Used[key]++
	}
	return nil
}

// Refund undoes a Spend (the master's "Desfazer").
func (s *MonsterState) Refund(usage rules.ActionUsage, key string) {
	switch usage.Kind {
	case rules.UsageRecharge:
		s.Recharging = slices.DeleteFunc(s.Recharging, func(k string) bool { return k == key })
	case rules.UsagePerDay, rules.UsageRest:
		if s.Used[key] > 1 {
			s.Used[key]--
		} else {
			delete(s.Used, key)
		}
	}
}

// Recharged says whether a d6 brings a recharging action back: at the action's lowest
// face or more ("Recharge 5-6" on a 5 or a 6).
func Recharged(usage rules.ActionUsage, face int) bool {
	return usage.Kind == rules.UsageRecharge && face >= usage.RechargeMin
}

// RechargeRoll is a recharge roll at the start of a turn.
type RechargeRoll struct {
	Action     string
	Face       int
	Min        int
	Recharged  bool
	WasWaiting bool
}

// StartTurn works out the start of the monster's own turn: each action that waits for its
// recharge gets a d6 (roll gives its face) and comes back on its minimum or more, the
// legendary actions are regained (SRD 5.1: "regains its spent legendary actions at the
// start of its turn") and the Multiattack routine of the last turn is forgotten. It returns
// the rolls in the order of the stat block's actions.
func (s *MonsterState) StartTurn(plan *rules.MonsterPlan, roll func() int) []RechargeRoll {
	var out []RechargeRoll
	s.RechargeRolls, s.Offer = nil, nil
	for _, a := range plan.Actions {
		if a.Usage.Kind != rules.UsageRecharge || !s.IsRecharging(a.Key) {
			continue
		}
		r := RechargeRoll{Action: a.Key, Face: roll(), Min: a.Usage.RechargeMin, WasWaiting: true}
		s.RechargeRolls = append(s.RechargeRolls, RechargeFace{Action: a.Key, Face: r.Face})
		if r.Recharged = Recharged(a.Usage, r.Face); r.Recharged {
			s.Refund(a.Usage, a.Key)
		}
		out = append(out, r)
	}
	s.LegendaryUsed = 0
	s.Multiattack = nil
	return out
}

// LegendaryLeft is how many legendary actions the creature has left until its next turn.
func (s MonsterState) LegendaryLeft(plan *rules.MonsterPlan) int {
	if plan.Legendary == nil {
		return 0
	}
	return max(plan.Legendary.PerRound-s.LegendaryUsed, 0)
}

// ResistanceLeft is how many uses of Legendary Resistance are left.
func (s MonsterState) ResistanceLeft(plan *rules.MonsterPlan) int {
	return max(plan.LegendaryResistance-s.ResistanceUsed, 0)
}

// SpendLegendary takes a legendary action of the given cost.
func (s *MonsterState) SpendLegendary(plan *rules.MonsterPlan, cost int) error {
	if cost < 1 || s.LegendaryLeft(plan) < cost {
		return ErrNoLegendaryActions
	}
	s.LegendaryUsed += cost
	return nil
}

// SpendResistance uses one Legendary Resistance.
func (s *MonsterState) SpendResistance(plan *rules.MonsterPlan) error {
	if s.ResistanceLeft(plan) < 1 {
		return ErrNoLegendaryResistance
	}
	s.ResistanceUsed++
	return nil
}

// incapacitating are the conditions that leave a creature unable to take actions
// (SRD 5.1, Conditions: incapacitated, and paralyzed, petrified, stunned and unconscious,
// which include it).
var incapacitating = []string{
	"condition:incapacitated", "condition:paralyzed", "condition:petrified", "condition:stunned", "condition:unconscious",
}

// CanAct says whether the conditions leave the creature able to take actions.
func CanAct(conditions []string) bool {
	return !slices.ContainsFunc(conditions, func(c string) bool { return slices.Contains(incapacitating, c) })
}

// StartMultiattack writes down the routine of a Multiattack action the creature uses.
func (s *MonsterState) StartMultiattack(a rules.ActionPlan, routine int) error {
	if routine < 0 || routine >= len(a.Routines) {
		return ErrNotARoutine
	}
	left := make([]int, len(a.Routines[routine]))
	for i, step := range a.Routines[routine] {
		left[i] = step.Count
	}
	s.Multiattack = &MultiattackState{Action: a.Key, Routine: routine, Left: left}
	return nil
}

// CountAttack takes one attack off the routine of the turn when it is one of its steps.
// It reports whether it was: an attack outside the routine (or after the routine is
// finished) is still the master's to make.
func (s *MonsterState) CountAttack(plan *rules.MonsterPlan, actionKey string) bool {
	m := s.Multiattack
	if m == nil {
		return false
	}
	a, ok := plan.Action(m.Action)
	if !ok || m.Routine >= len(a.Routines) {
		return false
	}
	for i, step := range a.Routines[m.Routine] {
		if step.ActionKey == actionKey && i < len(m.Left) && m.Left[i] > 0 {
			m.Left[i]--
			return true
		}
	}
	return false
}

// SlotLeft is how many slots of a spell level (1 to 9) are left, given the slots the creature has.
func (s MonsterState) SlotLeft(level, total int) int {
	if level < 1 || level > 9 {
		return 0
	}
	used := 0
	if level <= len(s.SlotsUsed) {
		used = s.SlotsUsed[level-1]
	}
	return max(total-used, 0)
}

// SpendSlot spends a spell slot of the level.
func (s *MonsterState) SpendSlot(level int) {
	for len(s.SlotsUsed) < level {
		s.SlotsUsed = append(s.SlotsUsed, 0)
	}
	s.SlotsUsed[level-1]++
}

// RefundSlot gives a slot back (an undo).
func (s *MonsterState) RefundSlot(level int) {
	if level >= 1 && level <= len(s.SlotsUsed) && s.SlotsUsed[level-1] > 0 {
		s.SlotsUsed[level-1]--
	}
}

// SlotsUsedArray is SlotsUsed as the 9 levels Usage reads.
func (s MonsterState) SlotsUsedArray() [9]int {
	var out [9]int
	copy(out[:], s.SlotsUsed)
	return out
}
