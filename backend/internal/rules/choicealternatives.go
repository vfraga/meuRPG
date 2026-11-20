package rules

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"slices"
	"strings"
)

// A choice of alternatives is a "one of these" that mixes kinds: a skill or a language, a skill or a tool. It is a `choice`
// effect of kind "alternatives" whose `alternatives` list one kind each (a skill, a language or a tool), each with its own
// keys (a tool alternative may name a filter of proficiency kinds instead). `count` is how many picks the player makes in
// all, from the union of the alternatives. SRD 5.1 has no such choice; it is how a table's feature leaves some of what it
// grants to the player.
//
// A pick is written "<owner key>#alt:<signature>=<value>" in Build.FeatureChoices, the value carrying its kind
// ("skill:...", "language:..." or "proficiency:..."), and the signature is worked out from the count and the alternatives
// (not their order), as the groups of a background do. So a pick follows its choice when the master reorders it, and
// stops counting when the master edits it: the choice then asks for its picks again.
//
// A skill picked here is a skill the sheet has outside the skills of the class and the other sources (it is no pick of
// theirs, so it takes none of their places), a language joins the languages and a tool goes through the tool matching with
// the owner's other tool picks.

const (
	// choiceAlternatives is the kind of a choice effect that offers alternatives.
	choiceAlternatives = "alternatives"
	// MaxChoiceAlternatives bounds the alternatives of a choice: one of each kind.
	MaxChoiceAlternatives = 3
	minChoiceAlternatives = 2

	altFromAttr = ".from"

	altSkill    = "skill"
	altLanguage = "language"
	altTool     = "tool"

	altKeyPart = textSeparator + "alt:"
	// familyAlternativePick makes each pick of a choice of alternatives another one.
	familyAlternativePick = "alternative-pick"
)

// alternativeKinds are the kinds an alternative may have, in the order the menu lists them.
var alternativeKinds = []string{altSkill, altLanguage, altTool}

// alternativeKindNames are the Portuguese names of the kinds.
var alternativeKindNames = map[string]string{altSkill: "Perícia", altLanguage: "Idioma", altTool: "Ferramenta"}

// ChoiceAlternative is one kind of pick a choice of alternatives offers.
type ChoiceAlternative struct {
	// Kind is "skill", "language" or "tool".
	Kind string `json:"kind"`
	// From are the skill, language or proficiency keys offered.
	From []string `json:"from,omitempty"`
	// Filter, on a tool alternative, offers every proficiency of its kinds in place of From.
	Filter *ChoiceFilter `json:"filter,omitempty"`
}

// signature is what ties a stored pick to the choice: the same for the same count and alternatives in any order.
func alternativesSignature(e *Effect) string {
	parts := make([]string, 0, len(e.Alternatives))
	for _, a := range e.Alternatives {
		from := slices.Clone(a.From)
		slices.Sort(from)
		var kinds []string
		if a.Filter != nil {
			kinds = slices.Clone(a.Filter.ProficiencyKinds)
			slices.Sort(kinds)
		}
		parts = append(parts, fmt.Sprintf("%d:%s|%s|%s", len(a.Kind), a.Kind, lengthPrefixed(from), lengthPrefixed(kinds)))
	}
	slices.Sort(parts)
	sum := sha256.Sum256(fmt.Appendf(nil, "%d|%s", e.Count, lengthPrefixed(parts)))
	return hex.EncodeToString(sum[:4])
}

// lengthPrefixed writes every value with its length first, so no text can pass for a separator.
func lengthPrefixed(values []string) string {
	var b strings.Builder
	for _, v := range values {
		fmt.Fprintf(&b, "%d:%s,", len(v), v)
	}
	return b.String()
}

// AlternativesKey is the key of the choice a choice of alternatives of owner asks, and of the picks that answer it.
func AlternativesKey(owner string, e *Effect) string {
	return owner + altKeyPart + alternativesSignature(e)
}

// alternativeOptions are the keys one alternative offers, each once: the ones it lists that its kind may name, or the
// proficiencies of its filter's kinds.
func (c *content) alternativeOptions(a ChoiceAlternative) []string {
	switch {
	case a.Kind == altTool && a.Filter != nil:
		return c.proficiencyKeysOfKinds(a.Filter.ProficiencyKinds...)
	case a.Kind == altTool:
		return c.toolsOffered(a.From, false)
	}
	var out []string
	for _, k := range a.From {
		if c.alternativeKeyOK(a.Kind, k) && !slices.Contains(out, k) {
			out = append(out, k)
		}
	}
	return out
}

// alternativeKeyOK says the key is one an alternative of the kind may name.
func (c *content) alternativeKeyOK(kind, key string) bool {
	switch kind {
	case altSkill:
		return c.skills[key] != nil
	case altLanguage:
		return c.languages[key] != nil
	case altTool:
		return c.isToolPickKey(key)
	}
	return false
}

// alternativeValues are the keys a choice of alternatives offers in all, each once, in the order of the alternatives.
func (c *content) alternativeValues(e *Effect) []string {
	var out []string
	for _, a := range e.Alternatives {
		for _, k := range c.alternativeOptions(a) {
			if !slices.Contains(out, k) {
				out = append(out, k)
			}
		}
	}
	return out
}

// alternativeToolOptions are the tools the choice's tool alternatives offer.
func (c *content) alternativeToolOptions(e *Effect) []string {
	var out []string
	for _, a := range e.Alternatives {
		if a.Kind == altTool {
			out = append(out, c.alternativeOptions(a)...)
		}
	}
	return slices.Compact(slices.Sorted(slices.Values(out)))
}

// alternativeChoices are the choices of alternatives that owner has.
func (c *content) alternativeChoices(owner string) []*Effect {
	var out []*Effect
	seen := map[string]bool{}
	for _, e := range c.effectChoices(owner, choiceAlternatives) {
		if sig := alternativesSignature(e); !seen[sig] {
			seen[sig] = true
			out = append(out, e)
		}
	}
	return out
}

// alternativeByKey finds the choice of alternatives of owner that the choice key names.
func (c *content) alternativeByKey(owner, choiceKey string) *Effect {
	for _, e := range c.alternativeChoices(owner) {
		if AlternativesKey(owner, e) == choiceKey {
			return e
		}
	}
	return nil
}

// alternativeValueOK says a stored pick of a choice of alternatives is one the sheet may carry: for an owner that has
// the choice, a value the alternatives offer, or any value of a kind the content has when the master has edited the
// choice since (its signature matches none now: the pick counts for nothing, and the choice asks again).
func (c *content) alternativeValueOK(choiceKey, value string) bool {
	owner, _, _ := strings.Cut(choiceKey, altKeyPart)
	if e := c.alternativeByKey(owner, choiceKey); e != nil {
		return slices.Contains(c.alternativeValues(e), value)
	}
	if !c.choiceOwnerKnown(owner) {
		return false
	}
	return c.skills[value] != nil || c.languages[value] != nil || c.isToolPickKey(value)
}

// altPick is a pick of a choice of alternatives that counts.
type altPick struct {
	Owner, Key, Value string
	Effect            *Effect
}

// pickedAlternatives are the picks of the sheet's choices of alternatives that count: the owner is on the sheet, the choice
// offers the value, none before it was the same, and the choice takes no more than its count.
func (c *content) pickedAlternatives(b Build, owned func(string) bool) []altPick {
	var out []altPick
	taken := map[string]int{}
	for _, stored := range b.FeatureChoices {
		choiceKey, value, ok := SplitScopedChoice(stored)
		owner, _, isAlt := strings.Cut(choiceKey, altKeyPart)
		if !ok || !isAlt || !owned(owner) {
			continue
		}
		e := c.alternativeByKey(owner, choiceKey)
		if e == nil || taken[choiceKey] >= e.Count || !slices.Contains(c.alternativeValues(e), value) ||
			slices.ContainsFunc(out, func(p altPick) bool { return p.Value == value }) {
			continue
		}
		taken[choiceKey]++
		out = append(out, altPick{Owner: owner, Key: choiceKey, Value: value, Effect: e})
	}
	return out
}

// addAlternativePicks adds to the language and tool picks the ones the choices of alternatives took.
func (c *content) addAlternativePicks(b Build, owned func(string) bool, languages, tools []proficiencyPick) (outLanguages, outTools []proficiencyPick) {
	for _, p := range c.pickedAlternatives(b, owned) {
		switch {
		case altPickOfKind(p.Value, altLanguage) && !hasPick(languages, p.Value):
			languages = append(languages, proficiencyPick{Value: p.Value, Owner: p.Owner, Alt: true})
		case altPickOfKind(p.Value, altTool) && !hasPick(tools, p.Value):
			tools = append(tools, proficiencyPick{Value: p.Value, Owner: p.Owner, Alt: true})
		}
	}
	return languages, tools
}

// alternatives checks the alternatives of the effect being compiled, and says whether the effect is a choice of
// alternatives (or carries some), which the other checks of a choice do not apply to.
func (x effectCompiler) alternatives() (bool, error) {
	e := x.e
	if attr, _, msg := x.c.alternativeProblem(e); attr != "" {
		return true, x.fail("%s", msg)
	}
	return e.Choice == choiceAlternatives, nil
}

// altPickOfKind says the value is of the kind (a skill, a language or a tool).
func altPickOfKind(value, kind string) bool {
	switch kind {
	case altSkill:
		return strings.HasPrefix(value, "skill:")
	case altLanguage:
		return strings.HasPrefix(value, "language:")
	}
	return strings.HasPrefix(value, "proficiency:")
}

// alternativePicksOfKind are the values the picks of the kind hold.
func alternativePicksOfKind(picks []altPick, kind string) []string {
	var out []string
	for _, p := range picks {
		if altPickOfKind(p.Value, kind) {
			out = append(out, p.Value)
		}
	}
	return out
}

// alternativeToolSources are the lists the owner's choices of alternatives offer tools from, one source each.
func (c *content) alternativeToolSources(owner string) []pickSource {
	var out []pickSource
	for _, e := range c.alternativeChoices(owner) {
		if tools := c.alternativeToolOptions(e); len(tools) > 0 {
			out = append(out, pickSource{Owner: owner, Count: e.Count, From: tools})
		}
	}
	return out
}

// heldApart says the sheet has the skill, the language or the tool the value names from a source that is not a choice of
// alternatives: a pick of this kind is one the character would have twice.
func (x *deriver) heldApart(value string) bool {
	switch {
	case strings.HasPrefix(value, "skill:"):
		return x.skillHeldApart(value)
	case strings.HasPrefix(value, "language:"):
		return x.languageHeldApart(value)
	}
	return x.proficientApart[value] || hasPick(withoutAlternatives(x.pickedTools), value) || x.groupToolHeld(value)
}

// skillHeldApart says the automatic skills, the player's own list or an effect that gives at least a full proficiency with no
// condition give the skill.
func (x *deriver) skillHeldApart(skill string) bool {
	if slices.Contains(x.automaticSkillKeysApart(), skill) || slices.Contains(x.b.SkillProficiencies, skill) {
		return true
	}
	return slices.ContainsFunc(x.active, func(a activeEffect) bool {
		e := a.effect
		return e.Type == "proficiency" && e.Proficiency == skill && len(e.Tags) == 0 && x.applies(a)
	})
}

// groupToolHeld says a tool group of the background gave the tool.
func (x *deriver) groupToolHeld(tool string) bool {
	return slices.ContainsFunc(x.groupPicks(), func(p groupPick) bool {
		return p.Group.Kind == GroupTool && slices.Contains(p.Values, tool)
	})
}

// languageHeldApart says the race, a plain pick or the custom background gives the language.
func (x *deriver) languageHeldApart(language string) bool {
	if x.race != nil && slices.Contains(x.race.Languages, language) {
		return true
	}
	if x.b.Background == "" && slices.Contains(x.b.CustomBackgroundProficiencies, language) {
		return true
	}
	return hasPick(withoutAlternatives(x.pickedLanguages), language)
}

// withoutDuplicateAlternatives drops the picks of the choices of alternatives that repeat what another source gives.
func (x *deriver) withoutDuplicateAlternatives(picks []altPick) []altPick {
	return slices.DeleteFunc(picks, func(p altPick) bool { return x.heldApart(p.Value) })
}

// dropDuplicateAlternativePicks takes out of the language and tool picks the ones a choice of alternatives took that another
// source gives the sheet too.
func (x *deriver) dropDuplicateAlternativePicks() {
	repeated := func(p proficiencyPick) bool { return p.Alt && x.heldApart(p.Value) }
	x.pickedLanguages = slices.DeleteFunc(x.pickedLanguages, repeated)
	x.pickedTools = slices.DeleteFunc(x.pickedTools, repeated)
	x.unplacedTools = slices.DeleteFunc(x.unplacedTools, repeated)
}

// alternativeRequired is requiredPicks for a choice of alternatives: the keys it offers that the sheet does not have
// from another source are the picks to make, and the picks it holds (held) are made. It is the one number the screen
// and the sheet's issue read.
func (x *deriver) alternativeRequired(e *Effect, held []string) int {
	var pickable []string
	for _, v := range x.c.alternativeValues(e) {
		if !slices.Contains(held, v) && !x.heldApart(v) {
			pickable = append(pickable, v)
		}
	}
	return requiredPicks(e.Count, pickable, held)
}

// alternativeHeld are the values of the picks of the choice with this key.
func alternativeHeld(picks []altPick, key string) []string {
	var out []string
	for _, p := range picks {
		if p.Key == key {
			out = append(out, p.Value)
		}
	}
	return out
}

// checkAlternativePicks warns of the picks the sheet's choices of alternatives have not had.
func (x *deriver) checkAlternativePicks() {
	missing := 0
	seen := map[string]bool{}
	for _, a := range x.active {
		e := a.effect
		if e.Type != "choice" || e.Choice != choiceAlternatives || e.DailyChoice || seen[AlternativesKey(a.owner, e)] {
			continue
		}
		key := AlternativesKey(a.owner, e)
		seen[key] = true
		held := alternativeHeld(x.altPicks, key)
		missing += max(x.alternativeRequired(e, held)-len(held), 0)
	}
	if missing > 0 {
		x.issue(IssueMissing, choiceKeysField, "Falta escolher %d: faça a escolha no passo “Escolhas”.", missing)
	}
}

// alternativesTitle is how a choice of alternatives is called: "Perícia, idioma ou ferramenta".
func alternativesTitle(e *Effect) string {
	var names []string
	for i, a := range e.Alternatives {
		n := alternativeKindNames[a.Kind]
		if i > 0 {
			n = strings.ToLower(n)
		}
		names = append(names, n)
	}
	if len(names) <= 1 {
		return strings.Join(names, "")
	}
	return strings.Join(names[:len(names)-1], ", ") + " ou " + names[len(names)-1]
}

// alternativeOptionName is how an option of a choice of alternatives is named.
func (c *content) alternativeOptionName(value string) string {
	if strings.HasPrefix(value, "proficiency:") {
		return c.proficiencyNamePT(value)
	}
	return c.namePT(value)
}

// alternativeChoiceOptions are the options of a choice of alternatives, each marked with its kind, and the ones among them the
// sheet holds as picks of this choice. An option the character has already is blocked.
func (cb *choiceBuilder) alternativeChoiceOptions(e *Effect, key string) (options []ChoiceOption, held []string) {
	for _, a := range e.Alternatives {
		opts := cb.c.alternativeOptions(a)
		slices.SortFunc(opts, func(x, y string) int {
			return strings.Compare(foldPT(cb.c.alternativeOptionName(x)), foldPT(cb.c.alternativeOptionName(y)))
		})
		for _, k := range opts {
			if slices.ContainsFunc(options, func(o ChoiceOption) bool { return o.Key == k }) {
				continue
			}
			o := ChoiceOption{Key: k, Stored: ScopedChoice(key, k), NamePT: cb.c.alternativeOptionName(k), SummaryPT: alternativeKindNames[a.Kind] + "."}
			if slices.Contains(cb.scoped[key], k) {
				held = append(held, k)
			}
			if cb.deriver().heldApart(k) {
				blockOption(&o, "Você já tem isto.")
			}
			options = append(options, o)
		}
	}
	return options, held
}

// alternativeChoices adds the choices of alternatives that owner asks (and lists in g): one choice whose options are the
// alternatives' keys, each with its kind, but the ones the character has already and the ones another pick took.
func (cb *choiceBuilder) alternativeChoices(g *groupBuild, owner string) {
	for _, e := range cb.c.alternativeChoices(owner) {
		key := AlternativesKey(owner, e)
		options, held := cb.alternativeChoiceOptions(e, key)
		hint := "Uma escolha entre tipos diferentes. O que o personagem já tem não entra."
		if e.Count > 1 {
			hint = fmt.Sprintf("%d escolhas entre tipos diferentes. O que o personagem já tem não entra.", e.Count)
		}
		ch := cb.addChoice(g, &Choice{Key: key, FeatureKey: owner, Kind: ChoiceKindOptions, TitlePT: alternativesTitle(e), HintPT: hint, Picks: cb.deriver().alternativeRequired(e, held), family: familyAlternativePick, Options: options})
		cb.takeScoped(ch)
		unmeetBlockedPicks(ch)
	}
}

// checkAlternativesTwice refuses a second choice of alternatives of an owner that is the first one over again: they would
// share one key and one set of picks.
func (b *overlayBuilder) checkAlternativesTwice(c *entryErrors, owner, at string, e *Effect, seen map[string]bool) bool {
	if e.Type != "choice" || e.Choice != choiceAlternatives {
		return true
	}
	sig := alternativesSignature(e)
	if seen[sig] {
		c.add(ovErr(owner, "a feature has this choice of alternatives already: two of them would share their picks").at(at+".alternatives", ReasonValue))
		return false
	}
	seen[sig] = true
	return true
}

// alternativeProblem says what is wrong with the alternatives of a choice effect: the attribute, the reason and the
// sentence, with the attribute empty when the choice is sound. c is the content the keys are read in.
func (c *content) alternativeProblem(e *Effect) (attr, reason, msg string) {
	if e.Choice != choiceAlternatives {
		if len(e.Alternatives) > 0 {
			return ".alternatives", ReasonValue, "only a choice of alternatives has alternatives"
		}
		return "", "", ""
	}
	switch {
	case len(e.From) > 0 || e.Set != "" || e.Filter != nil || e.FromFeature != "" || e.DailyChoice || e.Name != "":
		return ".alternatives", ReasonValue, "a choice of alternatives lists its options in the alternatives, not in from, set, filter, from_feature or a daily choice"
	case len(e.Alternatives) < minChoiceAlternatives:
		return ".alternatives", ReasonValue, "a choice of alternatives offers at least two kinds"
	case len(e.Alternatives) > MaxChoiceAlternatives:
		return ".alternatives", ReasonLimit, fmt.Sprintf("a choice of alternatives has at most %d alternatives", MaxChoiceAlternatives)
	}
	for i, a := range e.Alternatives {
		if attr, reason, msg := c.alternativeFault(e.Alternatives[:i], a); attr != "" {
			return fmt.Sprintf(".alternatives[%d]%s", i, attr), reason, msg
		}
	}
	if n := len(c.alternativeValues(e)); e.Count > n {
		return ".count", ReasonValue, fmt.Sprintf("a choice cannot ask for more than the %d it offers", n)
	}
	return "", "", ""
}

// alternativeFault is what is wrong with one alternative, after the ones before it, at its own attribute.
func (c *content) alternativeFault(before []ChoiceAlternative, a ChoiceAlternative) (attr, reason, msg string) {
	switch {
	case !slices.Contains(alternativeKinds, a.Kind):
		return ".kind", ReasonValue, fmt.Sprintf("kind %q is not on the menu of an alternative (skill, language or tool)", a.Kind)
	case slices.ContainsFunc(before, func(o ChoiceAlternative) bool { return o.Kind == a.Kind }):
		return ".kind", ReasonValue, fmt.Sprintf("kind %q is listed twice", a.Kind)
	case a.Filter != nil && a.Kind != altTool:
		return ".filter", ReasonValue, "only a tool alternative has a filter"
	case a.Filter != nil && len(a.From) > 0:
		return ".filter", ReasonValue, "an alternative has a filter or a list of options, not both"
	case a.Filter == nil && len(a.From) == 0:
		return altFromAttr, ReasonValue, "an alternative lists the options to choose from"
	case a.Filter != nil:
		return filterFault(a.Filter)
	}
	for i, k := range a.From {
		switch {
		case slices.Contains(a.From[:i], k):
			return altFromAttr, ReasonValue, fmt.Sprintf("%q is listed twice", k)
		case !c.alternativeKeyOK(a.Kind, k):
			return altFromAttr, ReasonReference, fmt.Sprintf("%q is not a %s of the SRD that an alternative offers", k, a.Kind)
		}
	}
	return "", "", ""
}

// filterFault is what is wrong with the filter of a tool alternative.
func filterFault(f *ChoiceFilter) (attr, reason, msg string) {
	if len(f.ProficiencyKinds) == 0 {
		return ".filter.proficiency_kinds", ReasonValue, "a filter names at least one kind of proficiency"
	}
	for i, k := range f.ProficiencyKinds {
		switch {
		case !slices.Contains(filterKinds, k):
			return ".filter.proficiency_kinds", ReasonValue, fmt.Sprintf("kind %q is not on the filter's menu (weapon, tool or other)", k)
		case slices.Contains(f.ProficiencyKinds[:i], k):
			return ".filter.proficiency_kinds", ReasonValue, fmt.Sprintf("kind %q is listed twice", k)
		}
	}
	return "", "", ""
}

// checkAlternativesOwner refuses a choice of alternatives on an option: an option does not ask for another choice.
func (b *overlayBuilder) checkAlternativesOwner(site effectSite) error {
	if b.optionEntry[site.owner] != "" {
		return site.fail(".type", ReasonEffect, "an option does not ask for another choice")
	}
	return nil
}
