package rules

import (
	"slices"
	"strings"

	"github.com/PuraFome/meuRPG/backend/internal/rules/formula"
)

// The state effect: a temporary state a table feature switches on for a while (a stance
// for 1 minute, a form that lasts until a long rest). It names itself within its entry,
// lasts a number of rounds (or until it ends), ends in the ways it lists, and may spend
// a use of a resource when it starts. Starting and ending it is an action on the sheet,
// like a granted action; the effects of the same entry that carry `while` or read
// active() hold only while it is on (gate.go).
//
// The sheet's numbers come from Derive; which states are on is the game session's
// (Build.ActiveStates), and so is the clock: the rounds left, the rests and the
// incapacity that end a state are the play module's.

// How a state ends, besides its duration. "dismiss" is the player's own choice (the
// action of the sheet turns into "Encerrar"); a rest ends the states that say so
// (a short rest ends the "short_rest" ones, a long rest those and the "long_rest" ones);
// "incapacitated" ends it when the character is at 0 hit points or has a condition
// that incapacitates.
const (
	StateEndsDismiss       = "dismiss"
	StateEndsIncapacitated = "incapacitated"
	StateEndsShortRest     = "short_rest"
	StateEndsLongRest      = "long_rest"
)

var (
	// stateEnds is the closed set of EndsOn.
	stateEnds = []string{StateEndsDismiss, StateEndsIncapacitated, StateEndsShortRest, StateEndsLongRest}
	// stateEconomies are what starting a state may cost.
	stateEconomies = []string{EconomyAction, EconomyBonusAction, EconomyFree}
)

// Bounds of a state.
const (
	// MaxEntryStates is the most states one table entry defines.
	MaxEntryStates = 8
	// MaxStateRounds bounds a state's duration: 24 hours of 6-second rounds.
	MaxStateRounds = 14400
)

// State is a state effect of a feature the character has, as the sheet lists it.
type State struct {
	// Key identifies the state: its feature's key, "#" and its name
	// ("feature:postura@mesa#postura"). It is what Build.ActiveStates holds and what the
	// action that starts and ends it is keyed by.
	Key string
	// Name is the name the entry's effects read it by.
	Name string
	// NamePT is its feature's name.
	NamePT string
	// Source is the feature, trait, background feature or feat that defines it.
	Source string
	// DurationRounds is how many rounds it lasts; 0 is until it ends.
	DurationRounds int
	// EndsOn are the StateEnds* that end it besides the duration.
	EndsOn []string
	// Resource is the resource each start spends one use of; "" for none.
	Resource string
	// Economy is what starting it costs (an Economy* constant).
	Economy string
	// TextPT is the effect's text.
	TextPT string
	// Active says it is on.
	Active bool
}

// StateKey is the key of the state called name that the feature owner defines.
func StateKey(owner, name string) string { return owner + "#" + name }

// StateOn says whether the sheet has the state with this key and it is on.
func (d Derived) StateOn(key string) bool {
	return slices.ContainsFunc(d.States, func(s State) bool { return s.Key == key && s.Active })
}

// state checks what a state effect needs and compiles its duration.
func (x effectCompiler) state() error {
	e := x.e
	if !formula.ValidStateName(e.StateName) {
		return x.fail("a state needs a state_name of a-z, 0-9 and _ (1 to %d characters)", formula.MaxStateNameBytes)
	}
	if !slices.Contains(stateEconomies, e.Economy) {
		return x.fail("unknown economy %q: a state costs an action, a bonus action or nothing", e.Economy)
	}
	for i, end := range e.EndsOn {
		if !slices.Contains(stateEnds, end) || slices.Contains(e.EndsOn[:i], end) {
			return x.fail("unknown or repeated ends_on %q", end)
		}
	}
	e.stateKey = StateKey(x.key, e.StateName)
	var err error
	e.durationRounds, err = x.compile(e.DurationRounds, formula.Int)
	return err
}

// declareStates notes the states the effects of a feature define, in its entry: a gate
// of any feature of the entry may name them.
func (b *overlayBuilder) declareStates(owner, entry string, effects []Effect) {
	for i := range effects {
		e := &effects[i]
		if e.Type != "state" || !formula.ValidStateName(e.StateName) {
			continue
		}
		if b.stateKeys == nil {
			b.stateKeys, b.stateCount = map[string]map[string]string{}, map[string]map[string]int{}
		}
		if b.stateKeys[entry] == nil {
			b.stateKeys[entry], b.stateCount[entry] = map[string]string{}, map[string]int{}
		}
		b.stateCount[entry][e.StateName]++
		if b.stateKeys[entry][e.StateName] == "" {
			b.stateKeys[entry][e.StateName] = StateKey(owner, e.StateName)
		}
	}
}

// checkState checks a state effect of the table's content, at its fields.
func (b *overlayBuilder) checkState(site effectSite, entry string, e *Effect) error {
	if !formula.ValidStateName(e.StateName) {
		return site.fail(".state_name", ReasonValue, "a state name has 1 to %d characters of a-z, 0-9 and _", formula.MaxStateNameBytes)
	}
	if e.StateName == ResistanceWhileRage {
		return site.fail(".state_name", ReasonValue, "%q is the barbarian's rage, not a name for a state", ResistanceWhileRage)
	}
	if b.stateCount[entry][e.StateName] > 1 {
		return site.fail(".state_name", ReasonValue, "two states of this entry are named %q", e.StateName)
	}
	if len(b.stateKeys[entry]) > MaxEntryStates {
		return site.fail(".state_name", ReasonLimit, "an entry has at most %d states", MaxEntryStates)
	}
	if !slices.Contains(stateEconomies, e.Economy) {
		return site.fail(".economy", ReasonValue, "starting a state costs an action, a bonus action or nothing")
	}
	if err := checkStateEnds(site, e); err != nil {
		return err
	}
	if e.Resource != "" && !b.resourceDefined(site.owner, e.Resource) {
		return site.fail(".resource", ReasonReference, "the state spends %q, which no resource effect of this entry or its class defines", e.Resource)
	}
	if len(e.Tags) > 0 {
		return site.fail(".tags", ReasonValue, "a state has no tags")
	}
	return nil
}

// checkStateEnds checks the ways a state ends: the closed set, once each, and at
// least one way (a duration, or an entry of ends_on).
func checkStateEnds(site effectSite, e *Effect) error {
	for i, end := range e.EndsOn {
		if !slices.Contains(stateEnds, end) {
			return site.fail(".ends_on", ReasonValue, "ends_on %q is not one of %s", end, strings.Join(stateEnds, ", "))
		}
		if slices.Contains(e.EndsOn[:i], end) {
			return site.fail(".ends_on", ReasonValue, "ends_on lists %q twice", end)
		}
	}
	if e.DurationRounds == "" && len(e.EndsOn) == 0 {
		return site.fail(".ends_on", ReasonValue, "a state needs an end: a duration in rounds or at least one way to end")
	}
	return nil
}

// states fills Derived.States and the actions that start and end them, from the state
// effects the character has. A state is listed when its condition holds and, if it
// spends a resource, the character has that resource at this level; one that is on
// is always listed, so it can be ended.
func (x *deriver) states() {
	for _, a := range x.active {
		e := a.effect
		if e.Type != "state" {
			continue
		}
		on := x.stateOn(e.stateKey)
		if !on && (!x.applies(a) || (e.Resource != "" && !x.hasResource(e.Resource))) {
			continue
		}
		s := State{
			Key: e.stateKey, Name: e.StateName, NamePT: x.c.namePT(a.owner), Source: a.owner, DurationRounds: x.stateRounds(a),
			EndsOn: slices.Clone(e.EndsOn), Resource: e.Resource, Economy: e.Economy, TextPT: e.TextPT, Active: on,
		}
		x.d.States = append(x.d.States, s)
		if act, ok := stateAction(s); ok {
			x.d.Actions = append(x.d.Actions, act)
		}
	}
}

// stateRounds is the duration of a state in rounds: its formula, from 0 (until it ends)
// to MaxStateRounds.
func (x *deriver) stateRounds(a activeEffect) int {
	if a.effect.durationRounds == nil {
		return 0
	}
	n, err := a.effect.durationRounds.Int(x.env)
	if err != nil {
		x.issue(IssueFormula, "", "A duração de um efeito de %s não pôde ser calculada: o estado dura até acabar.", x.c.namePT(a.owner))
		return 0
	}
	return min(max(n, 0), MaxStateRounds)
}

// stateAction is the card of a state: it starts it with the economy and the resource
// its effect says, and, while it is on, ends it for free (when the player may dismiss
// it). Both are the same action, keyed by the state.
func stateAction(s State) (Action, bool) {
	act := Action{Key: s.Key, NamePT: s.NamePT, Economy: s.Economy, Resource: s.Resource, Source: s.Source, State: s.Key}
	if !s.Active {
		return act, true
	}
	act.Economy, act.Resource, act.StateActive = EconomyFree, "", true
	return act, slices.Contains(s.EndsOn, StateEndsDismiss)
}

// strayStateAndWeapon refuses the fields of the state, weapon_rule and pseudo_weapon
// effects on an effect of another type.
func (x effectCompiler) strayStateAndWeapon() error {
	e := x.e
	groups := []struct {
		types []string
		set   bool
		names string
	}{
		{[]string{"state"}, e.StateName != "" || e.DurationRounds != "" || len(e.EndsOn) > 0, "state_name, duration_rounds and ends_on belong to state"},
		{
			[]string{"weapon_rule"},
			e.WeaponName != "" || e.AttackAbility != "" || e.DamageAbility != "" || e.AttackBonus != 0 || e.DamageBonus != 0,
			"weapon_name, attack_ability, damage_ability, attack_bonus and damage_bonus belong to weapon_rule",
		},
		{[]string{"pseudo_weapon"}, e.AttackName != "" || e.DamageDice != "", "attack_name and damage_dice belong to pseudo_weapon"},
		{[]string{"weapon_rule", "pseudo_weapon"}, e.ReachFt != 0, "reach_ft belongs to weapon_rule and pseudo_weapon"},
	}
	for _, g := range groups {
		if g.set && !slices.Contains(g.types, e.Type) {
			return x.fail("%s", g.names)
		}
	}
	return nil
}
