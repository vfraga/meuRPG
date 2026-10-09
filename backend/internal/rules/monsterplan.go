package rules

import (
	"fmt"
	"regexp"
	"slices"
	"strconv"
	"strings"

	"github.com/PuraFome/meuRPG/backend/internal/rules/srd51"
)

// The stat block as a combat reads it (SRD 5.1, "Monsters": the creature
// statistics): MonsterPlan turns every action, trait, reaction and legendary
// action of a creature into what the server can roll, spend and show. The data
// (data/monsters.json) has the attack bonus, the damage parts and the save of an
// action; what it leaves in the text (the damage a failed save deals to the
// target of a hit, the condition a failed save gives, a grapple's escape DC, the
// area, the spells) is read here, from the SRD's own words, by patterns that are
// checked against the 334 creatures (monsterplan_test.go). Whatever the patterns
// cannot read reliably stays in Text, which the master always reads.

// How far the engine reads an action (ActionPlan.Class).
const (
	// ActionStructured: every number, damage part, saving throw and condition the
	// text states is in the plan; the text adds only what the table rules on
	// (a duration, a size, where something moves).
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

// How often an action may be used (ActionUsage.Kind).
const (
	UsageAtWill = "at_will"
	// UsageRecharge: after it is used, the server rolls a d6 at the start of each of the
	// creature's turns; at RechargeMin or more it can be used again.
	UsageRecharge = "recharge"
	// UsagePerDay: Uses times in the creature's day. A combat counts the uses of the
	// combat (the master gives the day back by hand).
	UsagePerDay = "per_day"
	// UsageRest: once, until a short or long rest.
	UsageRest = "rest"
)

// ActionUsage is the limit of an action.
type ActionUsage struct {
	Kind        string
	RechargeMin int
	Uses        int
	// Text is the SRD's words ("Recharge 5-6", "3/day").
	Text string
}

// ActionDamage is a damage part of an action.
type ActionDamage struct {
	Dice       DiceFormula
	TypeKey    string
	TypeNamePT string
}

// ActionAttack is the attack roll of an action.
type ActionAttack struct {
	Bonus int
	// Melee is a melee attack; Spell a spell attack (not a weapon's).
	Melee, Spell bool
	// ReachFt is a melee attack's reach; RangeFt and LongRangeFt a ranged attack's
	// (or a thrown weapon's) normal and long range.
	ReachFt, RangeFt, LongRangeFt int
}

// ActionSave is a saving throw an action asks for.
type ActionSave struct {
	Ability Ability
	DC      int
	// OnSuccess is "none", "half" or "other" (what a creature that saves still
	// takes): half of Damage, or the text's own effect.
	OnSuccess string
	// Damage is what the save governs: all of it for a creature that fails it,
	// OnSuccess says what a creature that saves takes. Empty when the save only
	// gives a condition.
	Damage []ActionDamage
	// ConditionKey is the condition a failed save gives ("condition:paralyzed"), or "".
	ConditionKey string
	// Duration is how long the condition lasts, in the SRD's words ("1 minute", "24 hours"), or
	// "" when the text gives another end (a round, a trigger); RepeatSave says the target repeats
	// the saving throw at the end of each of its turns. The app keeps the label until the master
	// takes it off; the duration is his to count (conditions with a duration are another unit's).
	Duration   string
	RepeatSave bool
	// OnHit says the save is the rider of an attack: it is asked of the target
	// once the attack hits. Without it the action asks it directly of the
	// creatures it affects.
	OnHit bool
}

// ActionHitCondition is a condition an attack gives its target when it hits,
// with no saving throw: a grapple (escape DC).
type ActionHitCondition struct {
	ConditionKey string
	EscapeDC     int
	// MaxSize is the biggest size the target may have ("Large"), or "".
	MaxSize string
}

// ActionArea is the area an action affects, as the SRD words it.
type ActionArea struct {
	// Shape is "cone", "line", "sphere", "cube", "cylinder" or "radius" (everything
	// within SizeFt of the creature).
	Shape string
	// SizeFt is the length of a cone or a line, the edge of a cube, the radius of a
	// sphere or a radius, and WidthFt a line's width.
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
	// Text is the SRD's words for a count that is not a number ("Number of Heads").
	Text string
}

// ActionPlan is one action of a stat block, as a combat uses it.
type ActionPlan struct {
	// Key is "<creature key>#<slug of the name>", the key of its Attack when it has
	// an attack roll, "monster:ghoul#claws".
	Key    string
	Name   string
	NamePT string
	Kind   string
	Class  string
	Usage  ActionUsage
	// Attack is set for an action with an attack roll; Damage is what a hit deals
	// (every part, in order). An action that asks a saving throw directly has its
	// damage in Save.Damage.
	Attack       *ActionAttack
	Damage       []ActionDamage
	Save         *ActionSave
	HitCondition *ActionHitCondition
	Area         *ActionArea
	// Routines are the Multiattack action's routines: the creature makes one of
	// them.
	Routines [][]MultiattackStep
	// Text is the SRD's text of the action, in English.
	Text string
}

// TraitPlan is a trait or a reaction: a name, the SRD's text, and the limit.
type TraitPlan struct {
	Key  string
	Name string
	// NamePT is the Portuguese name when the content has one.
	NamePT string
	// EngineReads says the engine applies the trait (effects/monster_traits.json); without it the
	// trait is a reminder the app does not apply.
	EngineReads bool
	Usage       ActionUsage
	Text        string
}

// LegendaryOption is one legendary action.
type LegendaryOption struct {
	Key    string
	Name   string
	NamePT string
	// Cost is how many of the creature's legendary actions the option spends.
	Cost int
	// ActionKey is the action of the stat block the option makes ("The dragon makes
	// a tail attack"), or "".
	ActionKey string
	// Action is the option itself when it has a roll of its own (Wing Attack's
	// saving throw): the same plan as an action.
	Action *ActionPlan
	Text   string
}

// LegendaryPlan is a creature's legendary actions (SRD 5.1, "Legendary Creatures").
type LegendaryPlan struct {
	// PerRound is how many legendary actions the creature may take between two of
	// its turns.
	PerRound int
	Options  []LegendaryOption
}

// SpellRef is a spell of a creature's spellcasting.
type SpellRef struct {
	Key    string
	Name   string
	NamePT string
	Level  int
	// Note is the SRD's remark ("self only", "any humanoid form"), or "".
	Note string
}

// SpellcastingLevel is the spells of one spell level of a creature that casts with
// slots, and the slots it has.
type SpellcastingLevel struct {
	Level  int
	Slots  int
	Spells []SpellRef
}

// InnateGroup is the spells a creature casts Uses times a day.
type InnateGroup struct {
	Uses int
	// Each says every spell has the uses ("3/day each"); without it the uses are
	// shared by the whole group ("3/day: enlarge/reduce, tongues").
	Each   bool
	Spells []SpellRef
}

// SpellcastingPlan is a creature's Spellcasting or Innate Spellcasting trait.
type SpellcastingPlan struct {
	// Key is "<creature key>#spellcasting" or "#innate-spellcasting".
	Key string
	// Innate says it is the Innate Spellcasting trait (no slots: at will, or a
	// number of times a day); otherwise it casts with slots.
	Innate  bool
	Name    string
	Ability Ability
	SaveDC  int
	// AttackBonus is the spell attack bonus; 0 when the SRD does not state one.
	AttackBonus int
	// CasterLevel is the spellcaster level of a creature that casts with slots.
	CasterLevel int
	// ClassKey is the class whose list the spells are from ("class:wizard"), or "".
	ClassKey string
	// AtWill are the spells cast without limit (the cantrips of a creature that
	// casts with slots, the "At will" line of an innate caster).
	AtWill []SpellRef
	// Levels are the spells by spell level (slots).
	Levels []SpellcastingLevel
	// Innate lists the spells with a number of uses a day.
	PerDay []InnateGroup
	// Text is the SRD's text of the trait.
	Text string
}

// MonsterPlan is everything a combat reads of a stat block beyond the Derived.
type MonsterPlan struct {
	Key     string
	Actions []ActionPlan
	// Traits are the special abilities that are not spellcasting or Legendary
	// Resistance; Reactions the reactions.
	Traits    []TraitPlan
	Reactions []TraitPlan
	// LegendaryResistance is how many times a day the creature may turn a failed
	// saving throw into a success; 0 when it has none.
	LegendaryResistance int
	Legendary           *LegendaryPlan
	Spellcasting        []SpellcastingPlan
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

// The conditions the patterns look for. exhaustion is a level, not a condition
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
	// failedSaveAnchorRe finds where the damage of a failed save starts: the words
	// that come right before it in the SRD's stat blocks.
	failedSaveAnchorRe = regexp.MustCompile(
		`(?:taking |takes |take |deals |suffers )`)
	rechargeRe = regexp.MustCompile(`^Recharge (\d)(?:-(\d))?$`)
	perDayRe   = regexp.MustCompile(`^(\d+)/[Dd]ay`)
	escapeRe   = regexp.MustCompile(`grappled \(escape DC (\d+)\)`)
	sizeRe     = regexp.MustCompile(`(Tiny|Small|Medium|Large|Huge|Gargantuan) or smaller`)
	costRe     = regexp.MustCompile(` ?\(Costs (\d+) Actions?\)$`)
	areaRe     = regexp.MustCompile(`(\d+)-foot(?:-radius)? (cone|line|sphere|cube|cylinder|radius)`)
	lineWidth  = regexp.MustCompile(`line that is (\d+) feet wide`)
	withinRe   = regexp.MustCompile(`within (\d+) (?:feet|ft\.)`)
	// halfDamageRe is how the SRD says a creature that saves takes half.
	halfDamageRe = regexp.MustCompile(`half as much damage|half the (?:\w+ )?damage`)
	// everyCreatureRe tells an area from a single target.
	everyCreatureRe = regexp.MustCompile(`(?i)\b(?:each|every|any) (?:living )?(?:creature|humanoid)`)
	atkHeadRe       = regexp.MustCompile(`^(Melee|Ranged|Melee or Ranged) (Weapon|Spell) Attack:`)
	// twoHandedRe is the other damage of a weapon used with two hands: the data keeps the
	// one-handed damage, and the text tells the table the other.
	twoHandedRe = regexp.MustCompile(`or \d+ \([0-9d +\-]+\) [a-z]+ damage if used with two hands`)
)

// sizeOrder ranks the sizes, for "Large or smaller".
var sizeOrder = []string{"Tiny", "Small", "Medium", "Large", "Huge", "Gargantuan"}

// SizeAtMost says whether size is the same as or smaller than largest.
func SizeAtMost(size, largest string) bool {
	a, b := slices.Index(sizeOrder, size), slices.Index(sizeOrder, largest)
	return a >= 0 && b >= 0 && a <= b
}

// parseUsage reads the SRD's limit of an action.
func parseUsage(text string) ActionUsage {
	u := ActionUsage{Kind: UsageAtWill, Text: text}
	switch {
	case text == "":
	case strings.HasPrefix(text, "Recharges after"):
		u.Kind = UsageRest
	default:
		if m := rechargeRe.FindStringSubmatch(text); m != nil {
			u.Kind = UsageRecharge
			u.RechargeMin, _ = strconv.Atoi(m[1])
		} else if m := perDayRe.FindStringSubmatch(text); m != nil {
			u.Kind = UsagePerDay
			u.Uses, _ = strconv.Atoi(m[1])
		}
	}
	return u
}

// parseDamageChain reads "N (dice) type damage" and the parts that follow it,
// joined by "plus": the dice of the text, not the average N.
func (c *content) parseDamageChain(s string) []ActionDamage {
	var out []ActionDamage
	for {
		s = strings.TrimSpace(s)
		if m := damageChainRe.FindStringSubmatch(s); m != nil {
			f, ok := ParseDice(m[2])
			key := "damage-type:" + m[3]
			if _, named := c.named[key]; !ok || !named {
				return out
			}
			out = append(out, ActionDamage{Dice: f, TypeKey: key, TypeNamePT: c.namePT(key)})
			s = s[len(m[0]):]
		} else if m := flatDamageRe.FindStringSubmatch(s); m != nil {
			n, _ := strconv.Atoi(m[1])
			key := "damage-type:" + m[2]
			if _, named := c.named[key]; !named {
				return out
			}
			out = append(out, ActionDamage{Dice: DiceFormula{Bonus: n}, TypeKey: key, TypeNamePT: c.namePT(key)})
			s = s[len(m[0]):]
		} else {
			return out
		}
		rest, ok := strings.CutPrefix(s, " plus ")
		if !ok {
			return out
		}
		s = rest
	}
}

// failedSaveDamage reads the damage a failed saving throw deals when the text states
// it: in the sentences that speak of a saving throw ("taking 9 (2d8) poison damage on a
// failed save", "or take 14 (3d6 + 4) bludgeoning damage", "On a failure, a target takes
// 15 (3d8 + 2) bludgeoning damage").
func (c *content) failedSaveDamage(text string) []ActionDamage {
	for _, sentence := range sentences(text) {
		if !strings.Contains(sentence, "saving throw") && !strings.Contains(sentence, "On a failure") && !strings.Contains(sentence, "On a failed save") {
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

// sentences splits an action's text into its sentences, keeping the numbers'
// decimal points and "ft." together.
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

// failedSaveCondition is the condition a failed saving throw gives, when the text says it
// in the usual words: "must succeed on a DC 14 Wisdom saving throw or be charmed", "or
// become frightened for 1 minute", "or fall unconscious", "is frightened", "knocked prone".
func failedSaveCondition(text string) string {
	for _, sentence := range sentences(text) {
		if !strings.Contains(sentence, "saving throw") && !strings.Contains(sentence, "On a failure") && !strings.Contains(sentence, "On a failed save") {
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

var (
	durationRe   = regexp.MustCompile(`\bfor (\d+ (?:minute|hour)s?)\b`)
	repeatSaveRe = regexp.MustCompile(`repeat the saving throw (?:at the end of|on) each of its turns`)
)

// conditionDuration reads how long the condition of a failed save lasts and whether the target
// repeats the saving throw each turn, from the SRD's words ("or become frightened for 1 minute",
// "can repeat the saving throw at the end of each of its turns").
func conditionDuration(text string) (duration string, repeat bool) {
	if m := durationRe.FindStringSubmatch(text); m != nil {
		duration = m[1]
	}
	return duration, repeatSaveRe.MatchString(text)
}

// areaOf reads the area of an action that affects every creature in it: "60-foot cone",
// "a 90-foot line that is 10 feet wide", "each creature within 10 ft". An action that
// picks one target ("one creature within 60 ft.") has no area.
func areaOf(text string) *ActionArea {
	if !everyCreatureRe.MatchString(text) {
		return nil
	}
	if m := areaRe.FindStringSubmatch(text); m != nil {
		a := &ActionArea{Shape: m[2]}
		a.SizeFt, _ = strconv.Atoi(m[1])
		if w := lineWidth.FindStringSubmatch(text); a.Shape == "line" && w != nil {
			a.WidthFt, _ = strconv.Atoi(w[1])
		}
		return a
	}
	if m := withinRe.FindStringSubmatch(text); m != nil {
		a := &ActionArea{Shape: "radius"}
		a.SizeFt, _ = strconv.Atoi(m[1])
		return a
	}
	return nil
}

// rawAction is the data an ActionPlan is built from: an action of the stat block, or a
// legendary action (which has nothing but a name and a text).
type rawAction struct {
	name, desc, usage string
	hasAttack         bool
	attackBonus       int
	damage            []srd51.MonsterDamage
	save              *srd51.MonsterSave
}

// planAction builds the plan of one action.
func (c *content) planAction(m *srd51.Monster, a rawAction) ActionPlan {
	p := ActionPlan{
		Key: m.Key + "#" + slugOf(a.name), Name: a.name, NamePT: c.namesPT["attack:"+slugOf(a.name)],
		Kind: ActionKindOther, Class: ActionText, Usage: parseUsage(a.usage), Text: a.desc,
	}
	dataDamage := func() []ActionDamage {
		var out []ActionDamage
		for _, d := range a.damage {
			f, _ := ParseDice(d.Dice)
			out = append(out, ActionDamage{Dice: f, TypeKey: d.DamageType, TypeNamePT: c.namePT(d.DamageType)})
		}
		return out
	}
	if a.hasAttack && atkHeadRe.MatchString(a.desc) {
		p.Kind = ActionKindAttack
		at := &ActionAttack{Bonus: a.attackBonus, Melee: strings.HasPrefix(a.desc, "Melee"), Spell: strings.Contains(a.desc, "Spell Attack")}
		if mm := reachRe.FindStringSubmatch(a.desc); mm != nil {
			at.ReachFt, _ = strconv.Atoi(mm[1])
		}
		if mm := rangeRe.FindStringSubmatch(a.desc); mm != nil {
			at.RangeFt, _ = strconv.Atoi(mm[1])
			at.LongRangeFt, _ = strconv.Atoi(mm[2])
		}
		p.Attack = at
		p.Damage = dataDamage()
	}
	hasSave := a.save != nil
	if hasSave {
		s := &ActionSave{Ability: Ability(a.save.Ability), DC: a.save.DC, OnSuccess: a.save.OnSuccess, OnHit: p.Attack != nil}
		s.ConditionKey = failedSaveCondition(a.desc)
		s.Duration, s.RepeatSave = conditionDuration(a.desc)
		if p.Attack == nil {
			p.Kind = ActionKindSave
			s.Damage = dataDamage()
		}
		// The damage the data leaves out: a bite's poison that a failed save deals.
		if len(s.Damage) == 0 {
			s.Damage = c.failedSaveDamage(a.desc)
		}
		// The data lists the damage of a failed save with the damage of the hit for some
		// attacks (the assassin's poison); it is rolled once, with the save.
		if p.Attack != nil {
			p.Damage = slices.DeleteFunc(p.Damage, func(d ActionDamage) bool {
				return slices.ContainsFunc(s.Damage, func(o ActionDamage) bool { return o.TypeKey == d.TypeKey && o.Dice == d.Dice })
			})
		}
		p.Save = s
	}
	if p.Attack != nil {
		if m := escapeRe.FindStringSubmatch(a.desc); m != nil && !strings.Contains(a.desc, "Instead of dealing damage") {
			h := &ActionHitCondition{ConditionKey: "condition:grappled"}
			h.EscapeDC, _ = strconv.Atoi(m[1])
			if sz := sizeRe.FindStringSubmatch(a.desc); sz != nil {
				h.MaxSize = sz[1]
			}
			p.HitCondition = h
		}
	}
	if p.Kind == ActionKindSave {
		p.Area = areaOf(a.desc)
	}
	p.Class = planClass(p, a)
	return p
}

// planClass says how far the plan holds what the text states: the damage parts, the
// saving throws and the conditions it mentions are all in the plan.
func planClass(p ActionPlan, a rawAction) string {
	if p.Kind == ActionKindOther {
		return ActionText
	}
	damage, dcs := len(p.Damage), 0
	conds := map[string]bool{}
	if p.Save != nil {
		damage += len(p.Save.Damage)
		dcs++
		if p.Save.ConditionKey != "" {
			conds[strings.TrimPrefix(p.Save.ConditionKey, "condition:")] = true
		}
	}
	if p.HitCondition != nil {
		dcs++ // the escape DC
		conds[strings.TrimPrefix(p.HitCondition.ConditionKey, "condition:")] = true
	}
	for _, w := range mentionedConditions(a.desc) {
		if !conds[w] {
			return ActionPartial
		}
	}
	mentions := len(damageMentionRe.FindAllString(a.desc, -1)) - len(twoHandedRe.FindAllString(a.desc, -1))
	if damage != mentions || dcs != len(dcMentionRe.FindAllString(a.desc, -1)) {
		return ActionPartial
	}
	return ActionStructured
}

// buildPlans builds the plan of every creature, after the corrections.
func (c *content) buildPlans() error {
	c.monsterPlans = make(map[string]*MonsterPlan, len(c.monsters))
	for _, key := range sortedKeys(c.monsters) {
		plan, err := c.planMonster(c.monsters[key])
		if err != nil {
			return fmt.Errorf("data/monsters.json: %s: %w", key, err)
		}
		c.monsterPlans[key] = plan
	}
	return nil
}

func (c *content) planMonster(m *srd51.Monster) (*MonsterPlan, error) {
	plan := &MonsterPlan{Key: m.Key}
	for _, a := range m.Actions {
		ap := c.planAction(m, rawAction{
			name: a.Name, desc: a.Desc, usage: a.Usage, hasAttack: a.HasAttack, attackBonus: a.AttackBonus, damage: a.Damage, save: a.Save,
		})
		if len(a.Multiattack) > 0 {
			ap.Kind, ap.Class = ActionKindMultiattack, ActionStructured
			for _, routine := range a.Multiattack {
				var steps []MultiattackStep
				for _, x := range routine {
					step := MultiattackStep{Name: x.Name, Count: x.Count, Kind: x.Kind, Text: x.Text}
					if i := slices.IndexFunc(m.Actions, func(o srd51.MonsterAction) bool { return o.Name == x.Name }); i >= 0 {
						step.ActionKey = m.Key + "#" + slugOf(x.Name)
					}
					steps = append(steps, step)
				}
				ap.Routines = append(ap.Routines, steps)
			}
		}
		plan.Actions = append(plan.Actions, ap)
	}
	for _, a := range m.SpecialAbilities {
		t := TraitPlan{Key: m.Key + "#" + slugOf(a.Name), Name: a.Name, NamePT: c.namesPT["attack:"+slugOf(a.Name)], Usage: parseUsage(a.Usage), Text: a.Desc, EngineReads: c.engineTraits[a.Name]}
		switch {
		case strings.HasPrefix(a.Name, "Legendary Resistance"):
			plan.LegendaryResistance = max(t.Usage.Uses, 1)
		case a.Name == "Spellcasting" || a.Name == "Innate Spellcasting":
			sc, err := c.planSpellcasting(m, a)
			if err != nil {
				return nil, err
			}
			plan.Spellcasting = append(plan.Spellcasting, sc)
		default:
			plan.Traits = append(plan.Traits, t)
		}
	}
	for _, a := range m.Reactions {
		plan.Reactions = append(plan.Reactions, TraitPlan{Key: m.Key + "#" + slugOf(a.Name), Name: a.Name, NamePT: c.namesPT["attack:"+slugOf(a.Name)], Usage: parseUsage(a.Usage), Text: a.Desc})
	}
	if len(m.LegendaryActions) > 0 {
		lp := &LegendaryPlan{PerRound: c.legendaryPerRound[m.Key]}
		for _, a := range m.LegendaryActions {
			opt := LegendaryOption{Key: m.Key + "#legendary-" + slugOf(costRe.ReplaceAllString(a.Name, "")), Cost: 1, Text: a.Desc}
			opt.Name = costRe.ReplaceAllString(a.Name, "")
			opt.NamePT = c.namesPT["attack:"+slugOf(opt.Name)]
			if cm := costRe.FindStringSubmatch(a.Name); cm != nil {
				opt.Cost, _ = strconv.Atoi(cm[1])
			}
			opt.ActionKey = legendaryTarget(m, a.Desc)
			// A legendary action with a saving throw of its own: the data keeps only its
			// text, so it is read as the actions' texts are.
			if sv := legendarySave(a.Desc); sv != nil {
				own := c.planAction(m, rawAction{name: opt.Name, desc: a.Desc})
				own.Key, own.Kind, own.Save, own.Area = opt.Key, ActionKindSave, c.legendarySavePlan(sv, a.Desc), areaOf(a.Desc)
				own.Class = planClass(own, rawAction{name: opt.Name, desc: a.Desc})
				opt.Action = &own
			}
			lp.Options = append(lp.Options, opt)
		}
		plan.Legendary = lp
	}
	return plan, nil
}

var (
	legendarySaveRe  = regexp.MustCompile(`DC (\d+) (Strength|Dexterity|Constitution|Intelligence|Wisdom|Charisma) saving throw`)
	legendaryMakesRe = regexp.MustCompile(`^The [a-z ]+? (?:makes (?:a|one) ([a-z ]+?) attack|uses its ([A-Za-z ]+?))\.`)
)

var abilityByWord = map[string]Ability{
	"Strength": STR, "Dexterity": DEX, "Constitution": CON, "Intelligence": INT, "Wisdom": WIS, "Charisma": CHA,
}

// legendarySave reads the DC and the ability of a legendary action's saving throw.
func legendarySave(text string) *srd51.MonsterSave {
	m := legendarySaveRe.FindStringSubmatch(text)
	if m == nil {
		return nil
	}
	dc, _ := strconv.Atoi(m[1])
	return &srd51.MonsterSave{Ability: string(abilityByWord[m[2]]), DC: dc, OnSuccess: "none"}
}

func (c *content) legendarySavePlan(sv *srd51.MonsterSave, text string) *ActionSave {
	s := &ActionSave{Ability: Ability(sv.Ability), DC: sv.DC, OnSuccess: "none", ConditionKey: failedSaveCondition(text)}
	s.Duration, s.RepeatSave = conditionDuration(text)
	s.Damage = c.failedSaveDamage(text)
	if len(s.Damage) == 0 {
		// "must succeed on a DC 19 Dexterity saving throw or take 13 (2d6 + 6) bludgeoning damage".
		s.Damage = c.damageAfterOr(text)
	}
	if len(s.Damage) > 0 && halfDamageRe.MatchString(text) {
		s.OnSuccess = "half"
	}
	return s
}

// damageAfterOr reads "or take N (dice) type damage" after a saving throw.
func (c *content) damageAfterOr(text string) []ActionDamage {
	_, after, ok := strings.Cut(text, "saving throw or take ")
	if !ok {
		return nil
	}
	return c.parseDamageChain(after)
}

// legendaryTarget finds the action of the stat block a legendary action makes ("The dragon
// makes a tail attack", "The lich uses its Paralyzing Touch"): the action called that, or
// whose name starts with the words. "" when there is none or the words are not clear.
func legendaryTarget(m *srd51.Monster, text string) string {
	mm := legendaryMakesRe.FindStringSubmatch(text)
	if mm == nil {
		return ""
	}
	want := strings.ToLower(strings.TrimSpace(mm[1] + mm[2]))
	var found []string
	for _, a := range m.Actions {
		name := strings.ToLower(a.Name)
		if name == want || strings.HasPrefix(name, want+" ") || (mm[1] != "" && name == strings.TrimSuffix(want, " attack")) {
			found = append(found, m.Key+"#"+slugOf(a.Name))
		}
	}
	if len(found) == 1 {
		return found[0]
	}
	return ""
}
