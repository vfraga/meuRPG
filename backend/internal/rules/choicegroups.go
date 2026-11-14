package rules

import (
	"cmp"
	"fmt"
	"slices"
	"strings"
)

// The choices a character makes (PM-05): which ones the sheet asks for, how many
// picks each takes, what is picked, what each option asks for, and what is still
// open. The engine reads a Build (a draft or a stored sheet) and answers the same
// way for the creation step, the locked sheet's completion page and the level-up's
// catch-up, so the browser never decides a rule: it only shows the answer.
//
// What Build.FeatureChoices holds is written in choicekeys.go.

// ChoiceOrigin says where a group of choices comes from.
type ChoiceOrigin int

// The origins of a group, in the order the step shows them.
const (
	ChoiceOriginRace ChoiceOrigin = iota + 1
	ChoiceOriginClass
	ChoiceOriginSubclass
	// ChoiceOriginBackground is the background, and ChoiceOriginFeat a feat the
	// sheet has: a choice that a background's or a feat's effect asks (a feat of
	// the player's choice).
	ChoiceOriginBackground
	ChoiceOriginFeat
)

// ChoiceKind says how a choice is made.
type ChoiceKind int

// The kinds of choice.
const (
	// ChoiceKindOptions picks options of a feature or trait (a fighting style, an
	// invocation) or values of a fixed list (a terrain).
	ChoiceKindOptions ChoiceKind = iota + 1
	// ChoiceKindAbilities picks abilities (the half-elf's +1 to two).
	ChoiceKindAbilities
	// ChoiceKindEnemy picks the type of a favored enemy, or the humanoid
	// option with two races written as text.
	ChoiceKindEnemy
	// ChoiceKindLanguage picks a language, or none.
	ChoiceKindLanguage
	// ChoiceKindSpells picks spells (a bonus cantrip, the Pact of the Tome's, an
	// arcanum).
	ChoiceKindSpells
)

// OptionPrerequisite is one thing an option asks for.
type OptionPrerequisite struct {
	// Kind is PrerequisiteLevel, PrerequisiteSpell or PrerequisiteFeature, or
	// PrerequisiteTaken for an option another choice already took.
	Kind string
	// Key is the spell or feature required, empty for a level; Level is the class
	// level required (0 for the others).
	Key   string
	Level int
	Met   bool
	// NamePT names the spell or the feature required, empty for a level.
	NamePT string
	// ReasonPT says what is missing, in Portuguese; empty when Met.
	ReasonPT string
}

// PrerequisiteTaken is the kind of the "already chosen" rule: an option can be
// taken once (a fighting style, a terrain, a favored enemy).
const PrerequisiteTaken = "taken"

// CircleSpell is a spell a Land druid's terrain gives, with the druid level.
type CircleSpell struct {
	Level   int
	Key     string
	NamePT  string
	Reached bool
}

// ChoiceOption is one option of a choice.
type ChoiceOption struct {
	// Key is the option's own key; Stored is what Build.FeatureChoices holds for it.
	Key, Stored string
	NamePT      string
	// SummaryPT is the rule in one line, in our own words.
	SummaryPT     string
	Prerequisites []OptionPrerequisite
	// ReasonPT is why the option cannot be taken now, in one sentence; empty when it
	// can.
	ReasonPT string
	// CircleSpells are the spells a terrain gives (the Land druid's).
	CircleSpells []CircleSpell
	// SpellLevel, SchoolPT and ClassesPT describe a spell option.
	SpellLevel int
	SchoolPT   string
	ClassesPT  []string
	// NeedsText says the option takes free text (the humanoid favored enemy).
	NeedsText bool
}

// Blocked says the option cannot be taken now.
func (o ChoiceOption) Blocked() bool { return o.ReasonPT != "" }

// Choice is one thing the sheet asks the player to pick.
type Choice struct {
	// Key identifies the choice on the sheet: the feature or trait it answers (with
	// "#<part>" when a feature asks for several things, "<spell-mastery>#1").
	Key string
	// FeatureKey is the feature, trait or race it belongs to.
	FeatureKey string
	Kind       ChoiceKind
	// TitlePT names the choice; PartPT the part of a feature that asks for several
	// ("Tipo de inimigo", "Idioma que falam"); LabelPT is how a sentence about
	// what is missing calls it ("Estilo de Luta (Guerreiro, nível 1)").
	TitlePT, PartPT, LabelPT string
	// HintPT is a line of help under the title.
	HintPT string
	// Level is the class level the choice came at (0 for a race's).
	Level int
	// Picks is how many selections the choice takes.
	Picks int
	// Picked are the option keys taken, in the order of the options.
	Picked []string
	// Texts are the free texts of the humanoid favored enemy, in order; empty for
	// the others.
	Texts   []string
	Options []ChoiceOption
	// ResultPT says what the picks give, with the numbers of the draft (the breath
	// weapon with its DC, the abilities' new scores); empty when nothing is picked.
	ResultPT string
	// Unmet are the picked options whose prerequisites are not met, and Overflow the
	// picks past what the choice takes or that it does not offer.
	Unmet, Overflow []string
	// Advisory says the choice only warns while it is open: the languages and the tools a race, a trait, a
	// background or a feature lets the player pick. It is pending (the step marks it, the locked sheet and the master's
	// list count it) but no problem is raised for it and a level-up does not ask it, as before the picks were here.
	Advisory bool

	family string
	// plain says the picks are options written as their own keys (a fighting style),
	// which the level-up's feature choices carry; the others are scoped picks.
	plain bool
	// legacy counts the selections a sheet made before they were asked here (a
	// bonus cantrip in the cantrip list) without saying which ones.
	legacy int
	// viaCantrips says the picks are cantrips the cantrip list carries too (a bonus
	// cantrip): the guided level-up asks for them with the cantrips.
	viaCantrips bool
}

// Done is how many of the choice's selections are made (never more than Picks). A pick of an
// option another choice took is a repeat, not a selection made.
func (ch Choice) Done() int {
	n := min(len(ch.Picked)-ch.repeats()+ch.legacy, ch.Picks)
	if n > 0 && ch.needsTexts() && !ch.textsComplete() {
		return n - 1
	}
	return n
}

// repeats is how many of the picks are of an option that another choice (of the same
// pool or family) took.
func (ch Choice) repeats() int {
	n := 0
	for _, o := range ch.Options {
		if slices.Contains(ch.Picked, o.Key) && o.taken() {
			n++
		}
	}
	return n
}

// Missing is how many selections are still to make.
func (ch Choice) Missing() int { return ch.Picks - ch.Done() }

func (ch Choice) needsTexts() bool {
	for _, o := range ch.Options {
		if o.NeedsText && slices.Contains(ch.Picked, o.Key) {
			return true
		}
	}
	return false
}

func (ch Choice) textsComplete() bool {
	if len(ch.Texts) < humanoidRaceSlots {
		return false
	}
	for _, t := range ch.Texts {
		if strings.TrimSpace(t) == "" {
			return false
		}
	}
	return true
}

// humanoidRaceSlots is how many races the humanoid favored enemy names (SRD 5.1,
// Ranger, Favored Enemy: "two races of humanoid").
const humanoidRaceSlots = 2

// ChoiceGroup is the choices one source asks at one level.
type ChoiceGroup struct {
	Origin ChoiceOrigin
	// SourceKey is the race, subrace, class or subclass key; SourceNamePT its name.
	// ClassNamePT is the class's name for a class or a subclass ("Patrulheiro" for
	// the Hunter), empty for a race.
	SourceKey, SourceNamePT, ClassNamePT string
	// Level is the class level of the group (0 for a race).
	Level   int
	Choices []Choice
}

// ChoiceSet is every choice a Build asks, and the picks nothing asks for.
type ChoiceSet struct {
	Groups []ChoiceGroup
	// NotOffered are the stored picks that no choice of this character offers.
	NotOffered []string
}

// Counts is how many selections are made and how many the sheet asks: the
// "Escolhas feitas: N de M" of every screen.
func (s ChoiceSet) Counts() (done, total int) {
	for _, g := range s.Groups {
		for _, ch := range g.Choices {
			done += ch.Done()
			total += ch.Picks
		}
	}
	return done, total
}

// PendingChoice is a choice with selections still to make.
type PendingChoice struct {
	GroupSourceKey string
	Level          int
	ChoiceKey      string
	LabelPT        string
	Missing        int
}

// Pending lists the choices with selections left to make, in the order of the
// groups.
func (s ChoiceSet) Pending() []PendingChoice {
	var out []PendingChoice
	for _, g := range s.Groups {
		for _, ch := range g.Choices {
			if m := ch.Missing(); m > 0 {
				out = append(out, PendingChoice{GroupSourceKey: g.SourceKey, Level: g.Level, ChoiceKey: ch.Key, LabelPT: ch.LabelPT, Missing: m})
			}
		}
	}
	return out
}

// PendingCount is how many selections are left across the sheet.
func (s ChoiceSet) PendingCount() int {
	n := 0
	for _, p := range s.Pending() {
		n += p.Missing
	}
	return n
}

// Choices answers every choice the Build asks. A Build with unknown keys still
// answers: what it cannot place is in NotOffered.
func (c *Content) Choices(b Build) ChoiceSet { return c.c.choiceSet(b) }

// choiceKeysField is the sheet field the picks live in.
const choiceKeysField = "full.feature_choice_keys"

// classWarlock is the Warlock, whose pact, invocations and patron the choices know by key.
const classWarlock = "class:warlock"

// ChoiceProblem is a pick the rules refuse.
type ChoiceProblem struct {
	// Code is ChoiceProblemMissing, ChoiceProblemPrerequisite or ChoiceProblemNotOffered.
	Code string
	// ChoiceKey and Field name the choice; Picked and Required count its selections.
	ChoiceKey, Field string
	LabelPT          string
	Picked, Required int
	OptionKey        string
}

// The codes of a ChoiceProblem.
const (
	ChoiceProblemMissing      = "missing"
	ChoiceProblemPrerequisite = "prerequisite"
	ChoiceProblemNotOffered   = "not_offered"
)

// ChoiceProblems lists what the rules refuse in the picks of a Build: a prerequisite
// that is not met, a pick the sheet does not offer or past the number it takes,
// and (with missing set) a choice left open. They come in that order, so the first
// is the most specific.
func (s ChoiceSet) ChoiceProblems(missing bool) []ChoiceProblem {
	out := s.choiceProblems(nil, unmetProblems)
	for _, k := range s.NotOffered {
		out = append(out, ChoiceProblem{Code: ChoiceProblemNotOffered, Field: choiceKeysField, OptionKey: k})
	}
	out = s.choiceProblems(out, overflowProblems)
	if missing {
		out = s.choiceProblems(out, missingProblem)
	}
	return out
}

// choiceProblems appends the problems problemsOf finds in each choice of the
// set, group by group.
func (s ChoiceSet) choiceProblems(out []ChoiceProblem, problemsOf func([]ChoiceProblem, Choice) []ChoiceProblem) []ChoiceProblem {
	for _, g := range s.Groups {
		for _, ch := range g.Choices {
			out = problemsOf(out, ch)
		}
	}
	return out
}

// unmetProblems are the picks of a choice whose prerequisite is not met.
func unmetProblems(out []ChoiceProblem, ch Choice) []ChoiceProblem {
	for _, k := range ch.Unmet {
		out = append(out, ChoiceProblem{Code: ChoiceProblemPrerequisite, ChoiceKey: ch.Key, Field: choiceKeysField, LabelPT: ch.LabelPT, Picked: len(ch.Picked), Required: ch.Picks, OptionKey: k})
	}
	return out
}

// overflowProblems are the picks past the number a choice takes.
func overflowProblems(out []ChoiceProblem, ch Choice) []ChoiceProblem {
	for _, k := range ch.Overflow {
		out = append(out, ChoiceProblem{Code: ChoiceProblemNotOffered, ChoiceKey: ch.Key, Field: choiceKeysField, LabelPT: ch.LabelPT, Picked: len(ch.Picked), Required: ch.Picks, OptionKey: k})
	}
	return out
}

// missingProblem is a choice left open.
func missingProblem(out []ChoiceProblem, ch Choice) []ChoiceProblem {
	if ch.Missing() > 0 && !ch.Advisory {
		out = append(out, ChoiceProblem{Code: ChoiceProblemMissing, ChoiceKey: ch.Key, Field: choiceKeysField, LabelPT: ch.LabelPT, Picked: ch.Done(), Required: ch.Picks})
	}
	return out
}

// groupBuild is a group being built: its choices are pointers until the end, so
// the pools can write the picks into them.
type groupBuild struct {
	ChoiceGroup
	choices []*Choice
	sort    [3]int
}

// optionEntry is a choice whose options are feature or trait keys, waiting for the
// picks of its pool.
type optionEntry struct {
	choice  *Choice
	choose  int
	options []string
	level   int
	order   int
}

type choiceBuilder struct {
	c *content
	b Build
	x *deriver

	plain  []string
	scoped map[string][]string
	used   map[string]bool

	groups  map[string]*groupBuild
	all     []*Choice
	entries []*optionEntry
	order   int

	derived *Derived
	// typedLanguages and typedTools count the entries of the free fields that earlier choices took as answers.
	typedLanguages, typedTools int
	// legacyTaken counts the surplus cantrips choices already claimed.
	legacyTaken int
}

func (c *content) choiceSet(b Build) ChoiceSet {
	x := &deriver{b: b, c: c, d: &Derived{}, proficient: map[string]bool{}, conditions: map[*Effect]bool{}}
	x.resolve()
	cb := &choiceBuilder{c: c, b: b, x: x, scoped: map[string][]string{}, used: map[string]bool{}, groups: map[string]*groupBuild{}}
	for _, k := range b.FeatureChoices {
		if choiceKey, value, ok := SplitScopedChoice(k); ok {
			if !slices.Contains(cb.scoped[choiceKey], value) {
				cb.scoped[choiceKey] = append(cb.scoped[choiceKey], value)
			}
			continue
		}
		if !slices.Contains(cb.plain, k) {
			cb.plain = append(cb.plain, k)
		}
	}
	cb.race()
	cb.background()
	cb.featOwners()
	for _, oc := range x.classes {
		cb.class(oc)
	}
	cb.assignPools()
	return cb.finish()
}

// derive is Derive of the Build, once, for the numbers a few choices read.
func (cb *choiceBuilder) derive() *Derived {
	if cb.derived == nil {
		d := derive(cb.b, cb.c)
		cb.derived = &d
	}
	return cb.derived
}

// group finds or makes the group of a source at a level. classIndex orders the
// groups: the race first, then each class in the sheet's order, a class's own
// features before its subclass's.
func (cb *choiceBuilder) group(origin ChoiceOrigin, source string, level, classIndex int) *groupBuild {
	key := fmt.Sprintf("%d|%s|%d", origin, source, level)
	if g, ok := cb.groups[key]; ok {
		return g
	}
	g := &groupBuild{ChoiceGroup: ChoiceGroup{Origin: origin, SourceKey: source, SourceNamePT: cb.c.namePT(source), Level: level}}
	if classIndex > 0 && classIndex <= len(cb.x.classes) {
		g.ClassNamePT = cb.c.namePT(cb.x.classes[classIndex-1].key)
	}
	g.sort = [3]int{classIndex, level, int(origin)}
	cb.groups[key] = g
	return g
}

func (cb *choiceBuilder) addChoice(g *groupBuild, ch *Choice) *Choice {
	ch.Level = g.Level
	ch.LabelPT = cb.labelOf(g, ch)
	g.choices = append(g.choices, ch)
	cb.all = append(cb.all, ch)
	return ch
}

// labelOf is how a sentence about a missing choice calls it: "Estilo de Luta
// (Guerreiro, nível 1)", "Ancestral Dracônico (Draconato)".
func (cb *choiceBuilder) labelOf(g *groupBuild, ch *Choice) string {
	title := ch.TitlePT
	if ch.PartPT != "" {
		title += ": " + strings.ToLower(ch.PartPT[:1]) + ch.PartPT[1:]
	}
	if g.Origin == ChoiceOriginRace || g.Origin == ChoiceOriginBackground || g.Origin == ChoiceOriginFeat {
		return fmt.Sprintf("%s (%s)", title, g.SourceNamePT)
	}
	return fmt.Sprintf("%s (%s, nível %d)", title, g.SourceNamePT, g.Level)
}

// optionChoice makes a choice of feature or trait options and queues it for the
// picks of its pool.
func (cb *choiceBuilder) optionChoice(g *groupBuild, featureKey string, choose int, options []string) *optionEntry {
	title := choiceTitle(cb.c, featureKey)
	ch := cb.addChoice(g, &Choice{Key: featureKey, FeatureKey: featureKey, Kind: ChoiceKindOptions, TitlePT: title, Picks: choose, plain: true})
	cb.order++
	e := &optionEntry{choice: ch, choose: choose, options: options, level: g.Level, order: cb.order}
	cb.entries = append(cb.entries, e)
	return e
}

// choiceTitles are the names the step gives a choice when the feature's own name
// would say less (the terrain, the bonus cantrip).
var choiceTitles = map[string]string{
	"feature:circle-of-the-land": "Terreno do Círculo",
	"feature:bonus-cantrip":      "Truque adicional",
}

func choiceTitle(c *content, featureKey string) string {
	if t, ok := choiceTitles[featureKey]; ok {
		return t
	}
	return c.namePT(featureKey)
}

// optionLabel is an option's name without the name of its feature: "Estilo de Luta:
// Arquearia" is "Arquearia" in a list under "Estilo de Luta".
func optionLabel(name string) string {
	if _, after, ok := strings.Cut(name, ": "); ok {
		return after
	}
	return name
}

// effectChoices lists the "choice" effects of a kind that a key has.
func (c *content) effectChoices(key, kind string) []*Effect {
	var out []*Effect
	for _, e := range c.effects[key] {
		if e.Type == "choice" && e.Choice == kind && !e.DailyChoice { // a daily choice is answered each day (SetDailyChoice), not in the sheet's picks
			out = append(out, e)
		}
	}
	return out
}

func sortedGroups(m map[string]*groupBuild) []*groupBuild {
	out := make([]*groupBuild, 0, len(m))
	for _, g := range m {
		out = append(out, g)
	}
	slices.SortFunc(out, func(a, b *groupBuild) int {
		for i := range a.sort {
			if c := cmp.Compare(a.sort[i], b.sort[i]); c != 0 {
				return c
			}
		}
		return strings.Compare(a.SourceKey, b.SourceKey)
	})
	return out
}

// finish writes the groups out, and the picks nothing took into NotOffered.
func (cb *choiceBuilder) finish() ChoiceSet {
	cb.markFamilies()
	var set ChoiceSet
	for _, g := range sortedGroups(cb.groups) {
		out := g.ChoiceGroup
		for _, ch := range g.choices {
			out.Choices = append(out.Choices, *ch)
		}
		if len(out.Choices) > 0 {
			set.Groups = append(set.Groups, out)
		}
	}
	for _, k := range cb.plain {
		if !cb.used[k] {
			set.NotOffered = append(set.NotOffered, k)
		}
	}
	for _, k := range cb.b.FeatureChoices {
		if choiceKey, value, ok := SplitScopedChoice(k); ok && !cb.used[ScopedChoice(choiceKey, value)] {
			set.NotOffered = append(set.NotOffered, k)
		}
	}
	slices.Sort(set.NotOffered)
	set.NotOffered = slices.Compact(set.NotOffered)
	cb.results(&set)
	return set
}

// markFamilies blocks, in each choice of a family, the values an earlier choice of the
// same family took: each new favored enemy and each new terrain is another one (SRD
// 5.1, Ranger). A pick that repeats one is unmet.
func (cb *choiceBuilder) markFamilies() {
	taken := map[string][]string{}
	for _, ch := range cb.all {
		if ch.family == "" {
			continue
		}
		blockTaken(ch, taken[ch.family])
		for _, k := range ch.Picked {
			if !slices.Contains(taken[ch.family], k) {
				taken[ch.family] = append(taken[ch.family], k)
			}
		}
	}
}

// blockTaken blocks the options of a choice that an earlier choice of its family
// took, and marks unmet the picks among them.
func blockTaken(ch *Choice, taken []string) {
	for i := range ch.Options {
		o := &ch.Options[i]
		if !slices.Contains(taken, o.Key) {
			continue
		}
		o.Prerequisites = append(o.Prerequisites, takenPrerequisite("Você já escolheu esta opção."))
		o.ReasonPT = prerequisitesReason(o.Prerequisites)
		if slices.Contains(ch.Picked, o.Key) {
			ch.Unmet = append(ch.Unmet, o.Key)
		}
	}
}

// taken says an earlier pick of the sheet already holds the option: a prerequisite of the
// "taken" kind that is not met.
func (o ChoiceOption) taken() bool {
	for _, p := range o.Prerequisites {
		if p.Kind == PrerequisiteTaken && !p.Met {
			return true
		}
	}
	return false
}
