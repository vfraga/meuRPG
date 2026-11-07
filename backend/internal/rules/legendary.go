package rules

import (
	"cmp"
	"regexp"
	"strconv"
	"strings"
)

// Legendary actions, Legendary Resistance and monster reactions: the
// stat block's words read into what a combat can offer the master. Pure
// functions over a Creature; the play package keeps only what was spent.

// LegendaryActionsPerRound is how many legendary actions a creature can take
// between turns. SRD 5.1, "Legendary Actions": a legendary creature can take 3
// legendary actions, choosing from the options of its stat block, one at the end
// of another creature's turn; it regains its spent actions at the start of its
// turn. Every SRD stat block that has legendary actions follows this rule, and
// the data does not repeat the number.
const LegendaryActionsPerRound = 3

// Kinds of a legendary option: an attack resolved by the attack flow, a move
// resolved by movement, and text the master resolves by hand.
const (
	LegendaryKindAttack = "attack"
	LegendaryKindMove   = "move"
	LegendaryKindText   = "text"
)

// Kinds of a monster reaction: Parry changes the armor class, any other is text.
const (
	ReactionKindParry = "parry"
	ReactionKindText  = "text"
)

// legendaryResistanceTrait is the name of the SRD trait.
const legendaryResistanceTrait = "Legendary Resistance"

// LegendaryOption is one of the stat block's legendary actions.
type LegendaryOption struct {
	// Key is a stable slug of the name ("tail-attack"), unique in the stat block.
	Key string
	// Name is the SRD's name without the "(Costs N Actions)" suffix.
	Name string
	// NamePT is its Portuguese name; the option's Name when the content has none.
	NamePT string
	// Cost is how many of the creature's legendary actions it takes, 1 to 3.
	Cost int
	// Text is the SRD's text of the option, in English.
	Text string
	Kind string
	// ActionKey is, for an attack, the key of the stat block action it makes
	// (ActionPlan.Key); empty for the other kinds.
	ActionKey string
}

// MoveSpeedPercent is how much of its speed a legendary option of kind move lets the creature walk:
// 50 when the text says "half its speed" (the tarrasque), else 100 ("up to its speed").
func (o LegendaryOption) MoveSpeedPercent() int {
	if strings.Contains(o.Text, "half its speed") {
		return HalfSpeedPercent
	}
	return FullSpeedPercent
}

// The parts of the speed a legendary move walks, in percent.
const (
	HalfSpeedPercent = 50
	FullSpeedPercent = 100
)

// MonsterReaction is one of the stat block's reactions.
type MonsterReaction struct {
	Key  string
	Name string
	Text string
	Kind string
	// ACBonus is what Parry adds to the armor class; 0 for any other reaction.
	ACBonus int
}

// LegendaryProfile is what a stat block says about legendary actions, Legendary
// Resistance and reactions. The zero value is a creature with none.
type LegendaryProfile struct {
	// PerRound is LegendaryActionsPerRound when the stat block has options, else 0.
	PerRound int
	Options  []LegendaryOption
	// ResistancePerDay is the uses of Legendary Resistance a day, 0 when it has none.
	ResistancePerDay int
	Reactions        []MonsterReaction
}

// IsLegendary says the stat block has legendary actions or Legendary Resistance.
func (p LegendaryProfile) IsLegendary() bool { return p.PerRound > 0 || p.ResistancePerDay > 0 }

var (
	costRe     = regexp.MustCompile(`\s*\(Costs (\d+) Actions?\)\s*$`)
	parryACRe  = regexp.MustCompile(`adds (\d+) to its AC`)
	makesOneRe = regexp.MustCompile(`makes one (\w+) attack( or )?`)
)

// LegendaryProfile reads the stat block of a creature key; false when the key is
// not a creature of the content.
func (c *Content) LegendaryProfile(key string) (LegendaryProfile, bool) {
	cr, ok := c.CreatureByKey(key)
	if !ok {
		return LegendaryProfile{}, false
	}
	return ReadLegendaryProfile(cr), true
}

// ReadLegendaryProfile reads a stat block into its LegendaryProfile.
func ReadLegendaryProfile(cr Creature) LegendaryProfile {
	var p LegendaryProfile
	for _, t := range cr.Traits {
		if t.Name == legendaryResistanceTrait {
			p.ResistancePerDay = perDay(t.Usage)
		}
	}
	if len(cr.LegendaryActions) > 0 {
		p.PerRound = LegendaryActionsPerRound
	}
	for _, a := range cr.LegendaryActions {
		p.Options = append(p.Options, legendaryOption(a, cr.Actions))
	}
	for _, r := range cr.Reactions {
		p.Reactions = append(p.Reactions, monsterReaction(r))
	}
	return p
}

// perDay reads the uses of a "N/day" usage; 0 when it is not one.
func perDay(usage string) int {
	m := perDayRe.FindStringSubmatch(usage)
	if m == nil {
		return 0
	}
	n, _ := strconv.Atoi(m[1])
	return n
}

// legendaryOption reads one legendary action: its cost from the name's suffix and
// its kind from the name and the text.
func legendaryOption(a CreatureAbility, actions []CreatureAction) LegendaryOption {
	opt := LegendaryOption{Cost: 1, Text: a.Text, Kind: LegendaryKindText}
	opt.Name = strings.TrimSpace(costRe.ReplaceAllString(a.Name, ""))
	opt.NamePT = cmp.Or(a.NamePT, opt.Name)
	if m := costRe.FindStringSubmatch(a.Name); m != nil {
		opt.Cost, _ = strconv.Atoi(m[1])
	}
	opt.Key = slugOf(opt.Name)
	if opt.Name == "Move" {
		opt.Kind = LegendaryKindMove
		return opt
	}
	if act := attackActionOf(opt, actions); act != nil && act.Plan != nil {
		opt.Kind, opt.ActionKey = LegendaryKindAttack, act.Plan.Key
	}
	return opt
}

// attackActionOf finds the attack-roll action of the stat block an option makes:
// the one named like the option ("Bite"; "Claw Attack" is the Claw), or else the
// one its text names ("makes one tail attack"), unless the text offers another choice
// ("makes one bite attack or uses its Swallow"). nil when none, which makes the
// option text the master resolves (a choice between attacks, a spell, a gaze).
func attackActionOf(opt LegendaryOption, actions []CreatureAction) *CreatureAction {
	if act := attackActionNamed(actions, strings.TrimSuffix(strings.ToLower(opt.Name), " attack")); act != nil {
		return act
	}
	if strings.Contains(opt.Name, " or ") || opt.Name == "Attack" {
		return nil
	}
	if m := makesOneRe.FindStringSubmatch(opt.Text); len(m) == 3 && m[2] == "" {
		return attackActionNamed(actions, m[1])
	}
	return nil
}

// attackActionNamed finds the action with an attack roll whose name is name.
func attackActionNamed(actions []CreatureAction, name string) *CreatureAction {
	for i := range actions {
		if actions[i].HasAttack && strings.EqualFold(actions[i].Name, name) {
			return &actions[i]
		}
	}
	return nil
}

// monsterReaction reads one reaction: Parry adds a number to the armor class.
func monsterReaction(r CreatureAbility) MonsterReaction {
	out := MonsterReaction{Key: slugOf(r.Name), Name: r.Name, Text: r.Text, Kind: ReactionKindText}
	if m := parryACRe.FindStringSubmatch(r.Text); m != nil && r.Name == "Parry" {
		out.Kind = ReactionKindParry
		out.ACBonus, _ = strconv.Atoi(m[1])
	}
	return out
}

// legendaryNameKey is the names_pt.json key of a legendary action: "legendary:" and the slug of
// its name without the "(Costs N Actions)" suffix.
func legendaryNameKey(name string) string {
	return "legendary:" + slugOf(costRe.ReplaceAllString(name, ""))
}
