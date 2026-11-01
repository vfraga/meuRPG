package rules

import (
	"fmt"
	"regexp"
	"slices"
	"strconv"
	"strings"

	"github.com/PuraFome/meuRPG/backend/internal/rules/srd51"
)

// The actions of a stat block as a combat reads them (SRD 5.1, "Monsters":
// actions). The data (data/monsters.json) has the attack bonus, the damage
// parts and the saving throw of an action; what it leaves in the text (the
// damage a failed save deals to the target of a hit, the condition a failed
// save gives, a grapple's escape DC, the area) is read here from the SRD's own
// words, by patterns checked against the 334 creatures (monsterplan_test.go).
// Whatever the patterns cannot read reliably stays in Text, which the master
// always reads. Traits, reactions, legendary actions and spellcasting are not
// planned here.

// How far the engine reads an action (ActionPlan.Class).
const (
	// ActionStructured: every damage part, saving throw and condition the text
	// states is in the plan; the text adds only what the table rules on (a
	// duration, a size, where something moves).
	ActionStructured = "structured"
	// ActionPartial: the plan has the attack, the damage or the saving throw the
	// data states, but the text has a number or a condition the plan does not
	// hold: the master reads the text for it.
	ActionPartial = "partial"
	// ActionText: nothing to roll; the text is a reminder for the master.
	ActionText = "text"
)

// The kind of an action (ActionPlan.Kind).
const (
	// ActionKindAttack makes an attack roll.
	ActionKindAttack = "attack"
	// ActionKindSave asks the creatures it affects for a saving throw.
	ActionKindSave = "save"
	// ActionKindMultiattack is the Multiattack action: a routine of other actions.
	ActionKindMultiattack = "multiattack"
	// ActionKindOther has no roll (the text is the reminder).
	ActionKindOther = "other"
)

// How often an action may be used (ActionUsage.Kind). The plan only reads the
// limit; nothing enforces it yet.
const (
	UsageAtWill = "at_will"
	// UsageRecharge: usable again on a d6 of RechargeMin or more.
	UsageRecharge = "recharge"
	// UsagePerDay: Uses times a day.
	UsagePerDay = "per_day"
	// UsageRest: once, until a short or long rest.
	UsageRest = "rest"
)

// The shapes of an ActionArea.
const (
	AreaCone     = "cone"
	AreaLine     = "line"
	AreaSphere   = "sphere"
	AreaCube     = "cube"
	AreaCylinder = "cylinder"
	// AreaRadius is everything within SizeFt of the creature.
	AreaRadius = "radius"
)

// ActionUsage is the limit of an action.
type ActionUsage struct {
	Kind        string
	RechargeMin int
	Uses        int
	// Text is the SRD's words ("Recharge 5-6", "3/day"), or "".
	Text string
}

// ActionDamage is a damage part of an action.
type ActionDamage struct {
	Dice DiceFormula
	// DiceText is the dice as the rest of the content writes them ("2d8",
	// "1d6+3", or a flat "5").
	DiceText   string
	TypeKey    string
	TypeNamePT string
}

// ActionAttack is the attack roll of an action.
type ActionAttack struct {
	Bonus int
	// Melee is a melee attack; Spell a spell attack (not a weapon's).
	Melee, Spell bool
	// ReachFt is a melee attack's reach; RangeFt and LongRangeFt a ranged
	// attack's (or a thrown weapon's) normal and long range.
	ReachFt, RangeFt, LongRangeFt int
}

// ActionSave is a saving throw an action asks for.
type ActionSave struct {
	Ability Ability
	DC      int
	// OnSuccess is "none", "half" or "other": what a creature that saves still
	// takes of Damage (half of it, or the text's own effect).
	OnSuccess string
	// Damage is what the save governs: all of it for a creature that fails.
	// Empty when the save only gives a condition.
	Damage []ActionDamage
	// ConditionKey is the condition a failed save gives ("condition:paralyzed"),
	// or "", and ConditionNamePT its Portuguese name.
	ConditionKey, ConditionNamePT string
	// Duration is how long the condition lasts, in the SRD's words ("1 minute"),
	// or "" when the text gives another end. RepeatSave says the target repeats
	// the saving throw at the end of each of its turns. Both are notes for the
	// master; nothing counts the time.
	Duration   string
	RepeatSave bool
	// OnHit says the save is the rider of an attack: asked of the target once the
	// attack hits. Without it the action asks it of the creatures it affects.
	OnHit bool
}

// ActionHitCondition is a condition an attack gives its target when it hits,
// with no saving throw: a grapple (escape DC).
type ActionHitCondition struct {
	ConditionKey, ConditionNamePT string
	EscapeDC                      int
	// MaxSize is the biggest size the target may have ("Large"), or "".
	MaxSize string
}

// ActionArea is the area an action affects, as the SRD words it.
type ActionArea struct {
	// Shape is one of the Area* constants.
	Shape string
	// SizeFt is the length of a cone or a line, the edge of a cube, the radius of
	// a sphere, cylinder or radius; WidthFt is a line's width.
	SizeFt, WidthFt int
}

// MultiattackStep is one attack of a Multiattack routine.
type MultiattackStep struct {
	// ActionKey is the stat block action the step makes; "" for a step with no
	// action of its own (the creature's Spellcasting).
	ActionKey string
	Name      string
	Count     int
	// Kind is "melee", "ranged", "ability" or "magic" (the data's).
	Kind string
	// Text is the SRD's words for a count that is not a number, or "".
	Text string
}

// ActionPlan is one action of a stat block, as a combat uses it.
type ActionPlan struct {
	// Key is "<creature key>#<slug of the name>", the key of its Attack when it
	// has an attack roll, as in "monster:ghoul#claws".
	Key    string
	Name   string
	NamePT string
	Kind   string
	Class  string
	Usage  ActionUsage
	// Attack is set for an action with an attack roll; Damage is what a hit
	// deals (every part, in order). An action that asks a saving throw directly
	// has its damage in Save.Damage.
	Attack       *ActionAttack
	Damage       []ActionDamage
	Save         *ActionSave
	HitCondition *ActionHitCondition
	Area         *ActionArea
	// Routines are the Multiattack action's routines: the creature makes one.
	Routines [][]MultiattackStep
	// Text is the SRD's text of the action, in English.
	Text string
}

// MonsterPlan is everything a combat reads of a stat block's actions beyond
// the Derived.
type MonsterPlan struct {
	Key     string
	Actions []ActionPlan
}

// Action finds an action by key.
func (p *MonsterPlan) Action(key string) (ActionPlan, bool) {
	i := slices.IndexFunc(p.Actions, func(a ActionPlan) bool { return a.Key == key })
	if i < 0 {
		return ActionPlan{}, false
	}
	return p.Actions[i], true
}

// MonsterPlan returns the plan of a creature, and whether the key is a creature.
func (c *Content) MonsterPlan(key string) (*MonsterPlan, bool) {
	p, ok := c.c.monsterPlans[key]
	return p, ok
}

// MonsterActionPlan returns one action of a creature by the key
// "<creature key>#<slug>" (the key of its Attack and of its SaveAction), and
// whether there is such an action.
func (c *Content) MonsterActionPlan(actionKey string) (ActionPlan, bool) {
	creature, _, ok := strings.Cut(actionKey, "#")
	if !ok {
		return ActionPlan{}, false
	}
	p, ok := c.c.monsterPlans[creature]
	if !ok {
		return ActionPlan{}, false
	}
	return p.Action(actionKey)
}

// The conditions the patterns look for. Exhaustion is a level, not a condition
// a hit gives, and stays in the text.
var planConditions = []string{
	"blinded", "charmed", "deafened", "frightened", "grappled", "incapacitated", "invisible", "paralyzed",
	"petrified", "poisoned", "prone", "restrained", "stunned", "unconscious",
}

var (
	conditionWordRe = regexp.MustCompile(`\b(` + strings.Join(planConditions, "|") + `)\b`)
	damageMentionRe = regexp.MustCompile(`\b\d+ \([0-9d +\-]+\) [a-z]+ damage|\b\d+ (?:bludgeoning|piercing|slashing|acid|cold|fire|force|lightning|necrotic|poison|psychic|radiant|thunder) damage`)
	dcMentionRe     = regexp.MustCompile(`\bDC \d+`)
	damageChainRe   = regexp.MustCompile(`^(\d+) \(([0-9d +\-]+)\) ([a-z]+) damage`)
	flatDamageRe    = regexp.MustCompile(`^(\d+) ([a-z]+) damage`)
	// failedSaveAnchorRe finds where the damage of a failed save starts: the
	// words that come right before it in the SRD's stat blocks.
	failedSaveAnchorRe = regexp.MustCompile(`(?:taking |takes |take |deals |suffers )`)
	rechargeRe         = regexp.MustCompile(`^Recharge (\d)(?:-(\d))?$`)
	perDayRe           = regexp.MustCompile(`^(\d+)/[Dd]ay`)
	escapeRe           = regexp.MustCompile(`grappled \(escape DC (\d+)\)`)
	sizeRe             = regexp.MustCompile(`(Tiny|Small|Medium|Large|Huge|Gargantuan) or smaller`)
	areaRe             = regexp.MustCompile(`(\d+)-foot(?:-radius)? (cone|line|sphere|cube|cylinder|radius)`)
	lineWidthRe        = regexp.MustCompile(`line that is (\d+) (?:feet|ft\.) wide`)
	withinRe           = regexp.MustCompile(`within (\d+) (?:feet|ft\.)`)
	// everyCreatureRe tells an area from a single target.
	everyCreatureRe = regexp.MustCompile(`(?i)\b(?:each|every|any) (?:living )?(?:creature|humanoid)`)
	atkHeadRe       = regexp.MustCompile(`^(Melee|Ranged|Melee or Ranged) (Weapon|Spell) Attack:`)
	// twoHandedRe is the other damage of a weapon used with two hands: the data
	// keeps the one-handed damage and the text tells the table the other.
	twoHandedRe  = regexp.MustCompile(`or \d+ \([0-9d +\-]+\) [a-z]+ damage if used with two hands`)
	durationRe   = regexp.MustCompile(`\bfor (\d+ (?:minute|hour)s?)\b`)
	repeatSaveRe = regexp.MustCompile(`repeat the saving throw (?:at the end of|on) each of its turns`)
)

// parseUsage reads the SRD's limit of an action.
func parseUsage(text string) ActionUsage {
	u := ActionUsage{Kind: UsageAtWill, Text: text}
	if strings.HasPrefix(text, "Recharges after") {
		u.Kind = UsageRest
	} else if m := rechargeRe.FindStringSubmatch(text); m != nil {
		u.Kind = UsageRecharge
		u.RechargeMin, _ = strconv.Atoi(m[1])
	} else if m := perDayRe.FindStringSubmatch(text); m != nil {
		u.Kind = UsagePerDay
		u.Uses, _ = strconv.Atoi(m[1])
	}
	return u
}

// parseDamageChain reads "N (dice) type damage" and the parts that follow it,
// joined by "plus": the dice of the text, not the average N.
func (c *content) parseDamageChain(s string) []ActionDamage {
	var out []ActionDamage
	for {
		part, rest, ok := c.nextDamagePart(strings.TrimSpace(s))
		if !ok {
			return out
		}
		out = append(out, part)
		s, ok = strings.CutPrefix(rest, " plus ")
		if !ok {
			return out
		}
	}
}

// nextDamagePart reads one damage part at the start of s and returns what
// follows it.
func (c *content) nextDamagePart(s string) (ActionDamage, string, bool) {
	var text, typ string
	var n int
	if m := damageChainRe.FindStringSubmatch(s); m != nil {
		text, typ, n = strings.ReplaceAll(m[2], " ", ""), m[3], len(m[0])
	} else if m := flatDamageRe.FindStringSubmatch(s); m != nil {
		text, typ, n = m[1], m[2], len(m[0])
	} else {
		return ActionDamage{}, "", false
	}
	dice, ok := ParseDice(text)
	key := "damage-type:" + typ
	if !ok || !c.isDamageTypeKey(key) {
		return ActionDamage{}, "", false
	}
	return ActionDamage{Dice: dice, DiceText: text, TypeKey: key, TypeNamePT: c.namePT(key)}, s[n:], true
}

// speaksOfSave says the sentence is about a saving throw and its failure.
func speaksOfSave(sentence string) bool {
	return strings.Contains(sentence, "saving throw") || strings.Contains(sentence, "On a failure") || strings.Contains(sentence, "On a failed save")
}

// failedSaveDamage reads the damage a failed saving throw deals when the text
// states it, in the sentences that speak of a saving throw ("taking 9 (2d8)
// poison damage on a failed save", "or take 14 (3d6 + 4) bludgeoning damage").
func (c *content) failedSaveDamage(text string) []ActionDamage {
	for _, sentence := range sentences(text) {
		if !speaksOfSave(sentence) {
			continue
		}
		for _, at := range failedSaveAnchorRe.FindAllStringIndex(sentence, -1) {
			if parts := c.parseDamageChain(sentence[at[1]:]); len(parts) > 0 {
				return parts
			}
		}
	}
	return nil
}

// sentences splits an action's text into its sentences, keeping "ft." together.
func sentences(text string) []string {
	text = strings.ReplaceAll(text, "ft.", "ft")
	var out []string
	for _, line := range strings.Split(text, "\n") {
		for _, s := range strings.Split(line, ". ") {
			if s = strings.TrimSpace(s); s != "" {
				out = append(out, s)
			}
		}
	}
	return out
}

// mentionedConditions lists the conditions the text names, once each.
func mentionedConditions(text string) []string {
	var out []string
	for _, m := range conditionWordRe.FindAllString(text, -1) {
		if !slices.Contains(out, m) {
			out = append(out, m)
		}
	}
	return out
}

// failedSaveCondition is the condition a failed saving throw gives, when the
// text says it in the usual words ("must succeed on a DC 14 Wisdom saving throw
// or be charmed", "or become frightened for 1 minute", "or fall unconscious").
func failedSaveCondition(text string) string {
	for _, sentence := range sentences(text) {
		if !speaksOfSave(sentence) {
			continue
		}
		_, after, ok := strings.Cut(sentence, "saving throw")
		if !ok {
			after = sentence
		}
		if m := conditionWordRe.FindString(after); m != "" {
			return "condition:" + m
		}
	}
	return ""
}

// conditionDuration reads how long the condition of a failed save lasts and
// whether the target repeats the saving throw each turn.
func conditionDuration(text string) (duration string, repeat bool) {
	if m := durationRe.FindStringSubmatch(text); m != nil {
		duration = m[1]
	}
	return duration, repeatSaveRe.MatchString(text)
}

// areaOf reads the area of an action that affects every creature in it: "60-foot
// cone", "a 90-foot line that is 10 feet wide", "each creature within 10 ft". An
// action that picks one target has no area.
func areaOf(text string) *ActionArea {
	if !everyCreatureRe.MatchString(text) {
		return nil
	}
	if m := areaRe.FindStringSubmatch(text); m != nil {
		a := &ActionArea{Shape: m[2]}
		a.SizeFt, _ = strconv.Atoi(m[1])
		if w := lineWidthRe.FindStringSubmatch(text); a.Shape == AreaLine && w != nil {
			a.WidthFt, _ = strconv.Atoi(w[1])
		}
		return a
	}
	if m := withinRe.FindStringSubmatch(text); m != nil {
		a := &ActionArea{Shape: AreaRadius}
		a.SizeFt, _ = strconv.Atoi(m[1])
		return a
	}
	return nil
}

// dataDamage is the damage parts the data states for an action.
func (c *content) dataDamage(a srd51.MonsterAction) ([]ActionDamage, error) {
	var out []ActionDamage
	for _, d := range a.Damage {
		f, ok := ParseDice(d.Dice)
		if !ok {
			return nil, fmt.Errorf("%s: %q are not dice", a.Name, d.Dice)
		}
		out = append(out, ActionDamage{Dice: f, DiceText: d.Dice, TypeKey: d.DamageType, TypeNamePT: c.namePT(d.DamageType)})
	}
	return out, nil
}

// planAttack reads the attack roll of an action.
func planAttack(a srd51.MonsterAction) *ActionAttack {
	at := &ActionAttack{Bonus: a.AttackBonus, Melee: strings.HasPrefix(a.Desc, "Melee"), Spell: strings.Contains(a.Desc, "Spell Attack")}
	if mm := reachRe.FindStringSubmatch(a.Desc); mm != nil {
		at.ReachFt, _ = strconv.Atoi(mm[1])
	}
	if mm := rangeRe.FindStringSubmatch(a.Desc); mm != nil {
		at.RangeFt, _ = strconv.Atoi(mm[1])
		at.LongRangeFt, _ = strconv.Atoi(mm[2])
	}
	return at
}

// planSave reads the saving throw of an action; damage is what the data lists
// for it. An action with no attack roll asks the save directly, so the damage is
// the save's. For an attack, the data lists the damage of a failed save with the
// hit's for some attacks (the assassin's poison); it is rolled once, with the
// save.
func (c *content) planSave(a srd51.MonsterAction, p *ActionPlan, damage []ActionDamage) {
	s := &ActionSave{Ability: Ability(a.Save.Ability), DC: a.Save.DC, OnSuccess: a.Save.OnSuccess, OnHit: p.Attack != nil}
	s.ConditionKey = failedSaveCondition(a.Desc)
	if s.ConditionKey != "" {
		s.ConditionNamePT = c.namePT(s.ConditionKey)
	}
	s.Duration, s.RepeatSave = conditionDuration(a.Desc)
	if p.Attack == nil {
		p.Kind = ActionKindSave
		s.Damage = damage
	}
	// The damage the data leaves out: a bite's poison that a failed save deals.
	if len(s.Damage) == 0 {
		s.Damage = c.failedSaveDamage(a.Desc)
	}
	if p.Attack != nil {
		p.Damage = slices.DeleteFunc(p.Damage, func(d ActionDamage) bool {
			return slices.ContainsFunc(s.Damage, func(o ActionDamage) bool { return o.TypeKey == d.TypeKey && o.Dice == d.Dice })
		})
	}
	p.Save = s
}

// planHitCondition reads a grapple an attack gives on a hit.
func (c *content) planHitCondition(a srd51.MonsterAction) *ActionHitCondition {
	m := escapeRe.FindStringSubmatch(a.Desc)
	if m == nil || strings.Contains(a.Desc, "Instead of dealing damage") {
		return nil
	}
	h := &ActionHitCondition{ConditionKey: "condition:grappled", ConditionNamePT: c.namePT("condition:grappled")}
	h.EscapeDC, _ = strconv.Atoi(m[1])
	if sz := sizeRe.FindStringSubmatch(a.Desc); sz != nil {
		h.MaxSize = sz[1]
	}
	return h
}

// planAction builds the plan of one action.
func (c *content) planAction(m *srd51.Monster, a srd51.MonsterAction) (ActionPlan, error) {
	p := ActionPlan{
		Key: m.Key + "#" + slugOf(a.Name), Name: a.Name, NamePT: c.namesPT["attack:"+slugOf(a.Name)],
		Kind: ActionKindOther, Class: ActionText, Usage: parseUsage(a.Usage), Text: a.Desc,
	}
	damage, err := c.dataDamage(a)
	if err != nil {
		return p, err
	}
	if a.HasAttack && atkHeadRe.MatchString(a.Desc) {
		p.Kind, p.Attack, p.Damage = ActionKindAttack, planAttack(a), damage
		p.HitCondition = c.planHitCondition(a)
	}
	if a.Save != nil {
		c.planSave(a, &p, damage)
	}
	if p.Kind == ActionKindSave {
		p.Area = areaOf(a.Desc)
	}
	if len(a.Multiattack) > 0 {
		p.Kind, p.Routines = ActionKindMultiattack, planRoutines(m, a)
	}
	p.Class = planClass(p, a.Desc)
	return p, nil
}

// planRoutines reads the routines of a Multiattack, each step with the action
// of the stat block it makes.
func planRoutines(m *srd51.Monster, a srd51.MonsterAction) [][]MultiattackStep {
	var routines [][]MultiattackStep
	for _, routine := range a.Multiattack {
		var steps []MultiattackStep
		for _, x := range routine {
			step := MultiattackStep{Name: x.Name, Count: x.Count, Kind: x.Kind, Text: x.Text}
			if slices.ContainsFunc(m.Actions, func(o srd51.MonsterAction) bool { return o.Name == x.Name }) {
				step.ActionKey = m.Key + "#" + slugOf(x.Name)
			}
			steps = append(steps, step)
		}
		routines = append(routines, steps)
	}
	return routines
}

// planClass says how far the plan holds what the text states: the damage parts,
// the saving throws and the conditions it mentions are all in the plan.
func planClass(p ActionPlan, desc string) string {
	switch p.Kind {
	case ActionKindOther:
		return ActionText
	case ActionKindMultiattack:
		return ActionStructured
	}
	damage, dcs, conds := len(p.Damage), 0, map[string]bool{}
	if p.Save != nil {
		damage += len(p.Save.Damage)
		dcs++
		conds[strings.TrimPrefix(p.Save.ConditionKey, "condition:")] = p.Save.ConditionKey != ""
		if p.Save.OnSuccess == "other" { // the text's own effect on a success is not in the plan
			return ActionPartial
		}
	}
	if p.HitCondition != nil {
		dcs++ // the escape DC
		conds[strings.TrimPrefix(p.HitCondition.ConditionKey, "condition:")] = true
	}
	for _, w := range mentionedConditions(desc) {
		if !conds[w] {
			return ActionPartial
		}
	}
	mentions := len(damageMentionRe.FindAllString(desc, -1)) - len(twoHandedRe.FindAllString(desc, -1))
	if damage != mentions || dcs != len(dcMentionRe.FindAllString(desc, -1)) {
		return ActionPartial
	}
	return ActionStructured
}

// buildPlans builds the plan of every creature, after the corrections. It
// refuses a creature whose action cannot be read.
func (c *content) buildPlans() error {
	c.monsterPlans = make(map[string]*MonsterPlan, len(c.monsters))
	for _, key := range sortedKeys(c.monsters) {
		plan, err := c.planOf(c.monsters[key])
		if err != nil {
			return fmt.Errorf("data/monsters.json: %s: %w", key, err)
		}
		c.monsterPlans[key] = plan
	}
	return nil
}

// planOf builds the plan of one creature's actions.
func (c *content) planOf(m *srd51.Monster) (*MonsterPlan, error) {
	plan := &MonsterPlan{Key: m.Key}
	seen := map[string]bool{}
	for _, a := range m.Actions {
		ap, err := c.planAction(m, a)
		if err != nil {
			return nil, err
		}
		if seen[ap.Key] {
			return nil, fmt.Errorf("two actions have the key %s", ap.Key)
		}
		seen[ap.Key] = true
		plan.Actions = append(plan.Actions, ap)
	}
	return plan, nil
}
