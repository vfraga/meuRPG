package rules

import (
	"fmt"
	"io/fs"
	"slices"
	"strconv"
	"strings"
)

// Action economies, as in the grant_action effect.
const (
	EconomyAction      = "action"
	EconomyBonusAction = "bonus_action"
	EconomyReaction    = "reaction"
	EconomyFree        = "free"
	EconomyMovement    = "movement"
)

// Resource recharges, as in the resource effect.
const (
	RechargeShortRest = "short_rest"
	RechargeLongRest  = "long_rest"
	RechargeDawn      = "dawn"
	RechargeNone      = "none"
	// RechargeInitiative gives back one use when the owner rolls initiative, only
	// when none is left.
	RechargeInitiative = "initiative"
	// RechargeRestoreOneShort gives back one use on a short rest and all of them on
	// a long rest; RechargeRestoreOneLong gives back one use on a long rest and none
	// on a short one.
	RechargeRestoreOneShort = "restore_one_on_short_rest"
	RechargeRestoreOneLong  = "restore_one_on_long_rest"
)

// Resource is a use-limited feature at the character's level: Second Wind
// (1 use), Ki points, Rage. The session spends and recovers it (RN-02).
type Resource struct {
	// Key is the effect's resource name, such as "second_wind".
	Key string
	// NamePT is the Portuguese name ("Retomar o Fôlego").
	NamePT string
	// Max is the number of uses. Unlimited uses (a barbarian's Rage at 20, a druid's Wild Shape at 20) are 99; the screens say "ilimitado".
	Max int
	// Recharge is a Recharge* constant.
	Recharge string
	// Source is the feature or trait key that gives it.
	Source string
}

// Action is something a character does on their turn: a standard action
// (Dash) or one a feature grants (Second Wind).
type Action struct {
	// Key is the feature key for a feature action ("feature:second-wind"),
	// or "standard:dash" for a standard one.
	Key    string
	NamePT string
	// Economy is an Economy* constant.
	Economy string
	// Resource is the key of the Resource each use spends, or "".
	Resource string
	// Source is the feature or trait key that grants it, "" for standard
	// actions.
	Source string
	// Standard is the key of the standard action a feature action performs
	// ("standard:dash" for Cunning Action's Dash), whose effect on the turn it
	// has; "" for any other action.
	Standard string
	// State is the key of the state the action starts or ends (the action's own Key); ""
	// for any other action. StateActive says the state is on, so the action ends it: free,
	// and without the resource that starting it spends.
	State       string `json:",omitempty"`
	StateActive bool   `json:",omitempty"`
	// Cost is how many uses of Resource each use of the action spends, from the
	// grant_action effect's cost (1 when the effect has none); CostMax, when above
	// Cost, is the most the player may spend on one use, and the use asks how many.
	// Both are 0 for an action with no Resource. See actionspend.go.
	Cost    int `json:",omitempty"`
	CostMax int `json:",omitempty"`
	// GivesSlot is the level of the spell slot each use creates from the uses it spends (0: none);
	// SpendsSlot says each use spends a spell slot of the level the player picks and heals by it:
	// the Heal at level 1 and HealPerPoint for each level above. See actionslots.go.
	GivesSlot  int  `json:",omitempty"`
	SpendsSlot bool `json:",omitempty"`
	// Result is what a use of the action gives: hit points regained, temporary hit points.
	Result ActionResult `json:",omitzero"`
}

// featureStandards are the features whose action is a standard action taken
// with another economy: Cunning Action (a rogue's bonus action to Dash,
// Disengage or Hide), the monk's Step of the Wind (Disengage or Dash) and Patient
// Defense (Dodge). Each choice is an action of its own, keyed "<feature>:<choice>",
// so that taking it has the effect of the standard action (a Dash doubles the
// movement, a Disengage ends the opportunity attacks). A feature with a single
// choice keeps its own key.
var featureStandards = map[string][]string{
	"feature:cunning-action":   {"standard:dash", "standard:disengage", "standard:hide"},
	"feature:step-of-the-wind": {"standard:disengage", "standard:dash"},
	"feature:patient-defense":  {"standard:dodge"},
}

// standardAction is one entry of effects/standard_actions.json: the actions
// every character has, which the SRD only says in prose.
type standardAction struct {
	Key     string `json:"key"`
	NamePT  string `json:"name_pt"`
	Economy string `json:"economy"`
}

// loadStandardActions reads effects/standard_actions.json.
func (c *content) loadStandardActions(fsys fs.FS) error {
	var f struct {
		Actions []standardAction `json:"actions"`
	}
	if err := readJSON(fsys, "effects/standard_actions.json", &f); err != nil {
		return err
	}
	seen := map[string]bool{}
	for _, a := range f.Actions {
		switch {
		case a.Key == "" || a.NamePT == "":
			return fmt.Errorf("effects/standard_actions.json: every action needs a key and a name_pt")
		case seen[a.Key]:
			return fmt.Errorf("effects/standard_actions.json: duplicate key %q", a.Key)
		case !slices.Contains(economies, a.Economy):
			return fmt.Errorf("effects/standard_actions.json: %s: unknown economy %q", a.Key, a.Economy)
		}
		seen[a.Key] = true
		c.standardActions = append(c.standardActions, Action{Key: "standard:" + a.Key, NamePT: a.NamePT, Economy: a.Economy})
	}
	return nil
}

// resourcesAndActions fills Derived.Resources and Derived.Actions from the
// resource and grant_action effects of what the character has,
// StandardActions from the hand-written list, and AttacksPerAction from the
// extra_attack effects.
func (x *deriver) resourcesAndActions() {
	x.d.StandardActions = slices.Clone(x.c.standardActions)
	x.d.AttacksPerAction = 1
	x.criticalAndStyles()
	x.wildShapeRule()
	x.slotRecoveries()
	x.extraAttacks()
	// Two classes may share a resource name (Channel Divinity of the cleric and
	// the paladin); the session counts them as one pool, which has the larger
	// Max: a second class gives new effects but no extra use (SRD Multiclassing).
	at := map[string]int{}
	for _, a := range x.active {
		if a.effect.Type != "resource" || !x.applies(a) {
			continue
		}
		if r, ok := x.resource(a); ok {
			x.addResource(at, r)
		}
	}
	x.maneuvers()
	x.extraDamages()
	x.spellModifiers()
	x.ambushes()
	for _, a := range x.active {
		if a.effect.Type != "grant_action" || !x.applies(a) {
			continue
		}
		if x.hasManeuver(a.owner) {
			continue // the option is a die on a roll; its action would spend the resource twice
		}
		x.grantAction(a)
	}
	x.states()
	x.hooks()
}

// extraAttacks sets AttacksPerAction from the extra_attack effects: the
// highest count wins.
func (x *deriver) extraAttacks() {
	for _, a := range x.active {
		if a.effect.Type == "extra_attack" && x.applies(a) {
			x.d.AttacksPerAction = max(x.d.AttacksPerAction, a.effect.Count)
		}
	}
}

// resource is the Resource a resource effect gives at the character's level,
// and false when it gives none: no use at this level, or a formula that failed
// (an Issue is recorded).
func (x *deriver) resource(a activeEffect) (Resource, bool) {
	e := a.effect
	n, err := e.max.Int(x.env)
	if err != nil {
		x.issue(IssueFormula, "", "Um efeito de %s foi ignorado: a fórmula falhou.", x.c.namePT(a.owner))
		return Resource{}, false
	}
	if n <= 0 {
		return Resource{}, false
	}
	recharge := e.Recharge
	if e.rechargeIf != nil {
		switch ok, err := e.rechargeIf.Bool(x.env); {
		case err != nil:
			x.issue(IssueFormula, "", "Um efeito de %s foi ignorado: a condição falhou.", x.c.namePT(a.owner))
			return Resource{}, false
		case ok:
			recharge = e.RechargeThen
		}
	}
	name := x.c.namesPT["resource:"+e.Resource]
	if name == "" {
		name = x.c.namePT(a.owner)
	}
	return Resource{Key: e.Resource, NamePT: name, Max: n, Recharge: recharge, Source: a.owner}, true
}

// addResource adds r to Derived.Resources, or, when a resource of the same key
// is there (at holds where), keeps the one with the larger Max.
func (x *deriver) addResource(at map[string]int, r Resource) {
	if i, shared := at[r.Key]; shared {
		if r.Max > x.d.Resources[i].Max {
			x.d.Resources[i] = r
		}
		return
	}
	at[r.Key] = len(x.d.Resources)
	x.d.Resources = append(x.d.Resources, r)
}

// grantAction adds the actions of a grant_action effect, each route once.
func (x *deriver) grantAction(a activeEffect) {
	act := Action{Key: a.owner, NamePT: x.c.namePT(a.owner), Economy: a.effect.Economy, Source: a.owner}
	// The resource of the same feature, when it has one at this level. The
	// action takes the resource's name: the SRD names some features per
	// level ("Surto de Ação (1 uso)"), and the screen shows the uses apart.
	if i := slices.IndexFunc(x.d.Resources, func(r Resource) bool { return r.Source == a.owner }); i >= 0 {
		act.Resource = x.d.Resources[i].Key
		act.NamePT = x.d.Resources[i].NamePT
	}
	// A feature action may spend a resource of another feature (the monk's
	// actions spend ki).
	if spent := a.effect.Resource; spent != "" {
		if !slices.ContainsFunc(x.d.Resources, func(r Resource) bool { return r.Key == spent }) {
			return // the resource is not there: no ki, no ki action
		}
		act.Resource = spent
	}
	if !x.slotAction(&act, a) {
		return
	}
	x.costAndResult(&act, a)
	for _, route := range x.routes(act, a.effect.TextPT) {
		if !slices.ContainsFunc(x.d.Actions, func(o Action) bool { return o.Key == route.Key && o.Economy == route.Economy }) {
			x.d.Actions = append(x.d.Actions, route)
		}
	}
}

// routes is the action as the character takes it: the action itself, or one
// action for each standard action the feature performs (featureStandards).
func (x *deriver) routes(act Action, text string) []Action {
	standards := featureStandards[act.Key]
	if len(standards) == 0 && act.Economy == EconomyBonusAction && strings.HasSuffix(act.Key, tableContentSuffix) {
		standards = standardsNamedIn(text)
	}
	if len(standards) == 0 {
		return []Action{act}
	}
	var out []Action
	for _, key := range standards {
		route := act
		route.Standard = key
		if len(standards) > 1 {
			route.Key = act.Key + ":" + strings.TrimPrefix(key, "standard:")
		}
		if i := slices.IndexFunc(x.c.standardActions, func(a Action) bool { return a.Key == key }); i >= 0 && len(standards) > 1 {
			route.NamePT = act.NamePT + ": " + x.c.standardActions[i].NamePT
		}
		out = append(out, route)
	}
	return out
}

// tableContentSuffix ends the key of everything the master's table content defines.
const tableContentSuffix = "@mesa"

// standardsNamedIn lists the standard actions the Portuguese text of a table
// content's bonus action says it performs ("Desengajar ou Esconder como ação
// bônus", the goblin's Fuga Ágil), in a fixed order: the
// content has no field for it, as the SRD's Cunning Action has in featureStandards.
// Without one of the three words the action stays a plain bonus action.
func standardsNamedIn(text string) []string {
	t := strings.ToLower(text)
	var out []string
	for _, w := range []struct{ word, key string }{
		{"dispar", "standard:dash"}, {"desengaj", "standard:disengage"}, {"esconde", "standard:hide"},
	} {
		if strings.Contains(t, w.word) {
			out = append(out, w.key)
		}
	}
	return out
}

// Features that change the critical range or the bonus action attack.
const (
	improvedCriticalFeature = "feature:improved-critical"
	superiorCriticalFeature = "feature:superior-critical"
	// reliableTalentFeature is the rogue's Reliable Talent (SRD 5.1, Rogue,
	// level 11).
	reliableTalentFeature = "feature:reliable-talent"
	// savageAttacksTrait is the half-orc's Savage Attacks (SRD 5.1, Half-Orc).
	savageAttacksTrait = "trait:savage-attacks"
)

// brutalCriticalDice is how many extra weapon damage dice each Brutal Critical
// feature of the barbarian adds to a melee critical hit (SRD 5.1, Barbarian:
// one at level 9, two at 13, three at 17). A character has them all; the
// highest counts.
var brutalCriticalDice = map[string]int{
	"feature:brutal-critical-1-die":  1,
	"feature:brutal-critical-2-dice": 2, //nolint:mnd // the SRD table: two extra dice at level 13
	"feature:brutal-critical-3-dice": 3, //nolint:mnd // the SRD table: three extra dice at level 17
}

// twoWeaponFightingStyles are the fighting styles that keep the ability
// modifier on the damage of the bonus action attack.
var twoWeaponFightingStyles = []string{
	"feature:fighter-fighting-style-two-weapon-fighting",
	"feature:ranger-fighting-style-two-weapon-fighting",
}

// criticalAndStyles fills Derived.CriticalRange (20, 19 with Improved
// Critical, 18 with Superior Critical), Derived.TwoWeaponFighting,
// Derived.BrutalCriticalDice, Derived.SavageAttacks and Derived.ReliableTalent from the features the
// character has.
func (x *deriver) criticalAndStyles() {
	x.d.CriticalRange = 20
	for _, a := range x.active {
		if !x.applies(a) {
			continue
		}
		if a.effect.Type == "song_of_rest" {
			x.d.SongOfRestSides = max(x.d.SongOfRestSides, a.effect.Sides)
		}
		switch {
		case a.owner == superiorCriticalFeature:
			x.d.CriticalRange = min(x.d.CriticalRange, 18)
		case a.owner == improvedCriticalFeature:
			x.d.CriticalRange = min(x.d.CriticalRange, 19)
		case a.owner == reliableTalentFeature:
			x.d.ReliableTalent = true
		case a.owner == savageAttacksTrait:
			x.d.SavageAttacks = true
		case brutalCriticalDice[a.owner] > 0:
			x.d.BrutalCriticalDice = max(x.d.BrutalCriticalDice, brutalCriticalDice[a.owner])
		case slices.Contains(twoWeaponFightingStyles, a.owner):
			x.d.TwoWeaponFighting = true
		}
	}
}

// wildShapeRule fills Derived.WildShape from the wild_shape effects the character has: the
// highest challenge rating (the effect that gives it brings its no_fly and no_swim; on a tie
// the one with fewer limits), the lowest cost in uses and magical attacks if any effect gives them.
func (x *deriver) wildShapeRule() {
	var rule *WildShapeRule
	var from *Effect
	best := -1
	for _, a := range x.active {
		if a.effect.Type != "wild_shape" || !x.applies(a) {
			continue
		}
		if rule == nil {
			rule = &WildShapeRule{Cost: 1}
		}
		e := a.effect
		rule.Cost = min(rule.Cost, x.wildShapeCost(a))
		rule.Magical = rule.Magical || e.MagicalAttacks
		eighths, ok := x.wildShapeRating(a)
		if !ok {
			continue
		}
		if eighths > best || (eighths == best && limits(e) < limits(from)) {
			best, from = eighths, e
			rule.MaxCR, rule.NoFly, rule.NoSwim = crString(eighths), e.NoFly, e.NoSwim
		}
	}
	if rule != nil && best >= 0 {
		x.d.WildShape = rule
	}
}

// wildShapeCost is the uses one change of form costs under the effect: its form_cost (at least 0), or the
// standard cost of 1 when it sets none (or its formula failed).
func (x *deriver) wildShapeCost(a activeEffect) int {
	if a.effect.FormCost == "" {
		return 1
	}
	n, ok := x.evalInt(a.effect.formCost, a)
	if !ok {
		return 1
	}
	return max(n, 0)
}

// wildShapeRating is the highest challenge rating the effect allows, in eighths; ok is false when it sets none.
func (x *deriver) wildShapeRating(a activeEffect) (eighths int, ok bool) {
	e := a.effect
	switch {
	case e.MaxCR != "":
		return crEighths(e.MaxCR)
	case e.MaxCRFrom != "":
		n, good := x.evalInt(e.maxCRFrom, a)
		return eighthsPerRating * min(max(n, 0), maxChallengeRating), good
	}
	return 0, false
}

const (
	// maxChallengeRating is the highest challenge rating of a creature.
	maxChallengeRating = 30
	// eighthsPerRating is how many eighths make one challenge rating.
	eighthsPerRating = 8
)

// noLimitsKnown ranks an effect that is not there after one with any limit, in limits.
const noLimitsKnown = 3

// limits counts the restrictions a wild_shape effect puts on the forms.
func limits(e *Effect) int {
	if e == nil {
		return noLimitsKnown
	}
	n := 0
	if e.NoFly {
		n++
	}
	if e.NoSwim {
		n++
	}
	return n
}

// crFractions are the challenge ratings under 1, by their eighths.
var crFractions = map[int]string{1: "1/8", 2: "1/4", 4: "1/2"}

// crString is a challenge rating in eighths as the content writes it: "0", "1/8", "1/4", "1/2", "1"...
func crString(eighths int) string {
	if s, ok := crFractions[eighths]; ok {
		return s
	}
	return strconv.Itoa(eighths / 8)
}
