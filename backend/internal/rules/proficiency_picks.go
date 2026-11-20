package rules

import (
	"fmt"
	"slices"
	"strings"
)

// The language and tool picks. A race, a trait or a background that lets the player pick languages
// (`language_choices`: the half-elf's extra language, the Acolyte's two), and a trait, background, feature or
// feat with a `choice` effect of kind tool (the dwarf's artisan's tools, a table background's one type of
// artisan's tools), are choices of the "Escolhas" step like a feat is. A pick is written
// "<owner key>#language=<language key>" or "<owner key>#tool=<proficiency key>" in Build.FeatureChoices, where
// the owner is the race, trait, background or feature that grants it. The language joins the sheet's languages
// and the tool its proficiencies for as long as the owner is on the sheet; a language the character already
// speaks and a tool it already has are no option.
//
// The sheet's free fields (Build.Languages, Build.ToolProficiencies) stay for what the master adds. A sheet
// written before the picks existed answered them in those fields, so what is written there still answers the
// picks the character is owed, after the picks made here (the way a sheet's bonus cantrip once did).

const (
	// languagePickSuffix and toolPickSuffix end the key of the choice a language or a tool pick answers.
	languagePickSuffix = textSeparator + "language"
	toolPickSuffix     = textSeparator + "tool"

	// familyLanguagePick and familyToolPick make each pick another one: no language or tool is picked twice.
	familyLanguagePick = "language-pick"
	familyToolPick     = "tool-pick"
)

// LanguagePickKey is the key of the choice of the languages that owner lets the player pick.
func LanguagePickKey(owner string) string { return owner + languagePickSuffix }

// ToolPickKey is the key of the choice of the tool proficiencies that owner lets the player pick.
func ToolPickKey(owner string) string { return owner + toolPickSuffix }

// languageGrant is how many languages a race, a trait or a background lets the player pick.
func (c *content) languageGrant(owner string) int {
	switch {
	case c.races[owner] != nil:
		return c.races[owner].LanguageChoices
	case c.traits[owner] != nil:
		return c.traits[owner].LanguageChoices
	case c.backgrounds[owner] != nil:
		return c.backgrounds[owner].LanguageChoices
	}
	return 0
}

// isToolKey says a proficiency key is a tool the sheet can list.
func (c *content) isToolKey(key string) bool {
	p := c.proficiencies[key]
	return p != nil && p.Kind == "tool"
}

// proficiencyPick is a language or a tool picked through a choice, and the owner that granted it.
type proficiencyPick struct{ Value, Owner string }

// pickedProficiencies are the languages and the tools the picks of Build.FeatureChoices give: the ones whose owner
// is on the sheet (owned) and grants a pick with room left, that the owner offers, none picked before. In the order
// of the picks.
func (c *content) pickedProficiencies(b Build, owned func(string) bool) (languages, tools []proficiencyPick) {
	taken := map[string]int{}
	for _, stored := range b.FeatureChoices {
		choiceKey, value, ok := SplitScopedChoice(stored)
		if !ok {
			continue
		}
		if owner, isLanguage := strings.CutSuffix(choiceKey, languagePickSuffix); isLanguage {
			if owned(owner) && taken[choiceKey] < c.languageGrant(owner) && c.languages[value] != nil && !hasPick(languages, value) {
				taken[choiceKey]++
				languages = append(languages, proficiencyPick{Value: value, Owner: owner})
			}
		} else if owner, isTool := strings.CutSuffix(choiceKey, toolPickSuffix); isTool {
			if owned(owner) && c.toolPickFits(owner, value, taken[choiceKey]) && !hasPick(tools, value) {
				taken[choiceKey]++
				tools = append(tools, proficiencyPick{Value: value, Owner: owner})
			}
		}
	}
	return languages, tools
}

func hasPick(picks []proficiencyPick, value string) bool {
	return slices.ContainsFunc(picks, func(p proficiencyPick) bool { return p.Value == value })
}

// toolPickFits says the owner grants a tool pick with room left (taken are the ones made already) and offers the tool.
func (c *content) toolPickFits(owner, tool string, taken int) bool {
	picks, from, open, ok := c.toolSpec(owner)
	if !ok || taken >= picks {
		return false
	}
	return slices.Contains(c.toolsOffered(from, open), tool)
}

// proficiencyPicks reads the language and tool picks of the sheet, once the effects are collected.
func (x *deriver) proficiencyPicks() {
	onSheet := ownedBy(x.b, *x.d)
	x.pickedLanguages, x.pickedTools = x.c.pickedProficiencies(x.b, func(k string) bool { return x.owned[k] || onSheet(k) })
}

// listedNames adds to the sheet's list the names the player typed in a free field, once each: a name the sheet
// shows already (a language the race gives, a tool a pick gave) is not repeated.
func listedNames[T any](list []T, typed []string, name func(T) string, add func(string)) {
	seen := map[string]bool{}
	for _, item := range list {
		seen[foldPT(name(item))] = true
	}
	for _, text := range typed {
		text = strings.TrimSpace(text)
		if text == "" || seen[foldPT(text)] {
			continue
		}
		seen[foldPT(text)] = true
		add(text)
	}
}

// checkProficiencyPicks warns of the tool and language picks the sheet has not made: the tools a trait lets the
// player pick (the dwarf's), and the languages the race, its traits and the background let the player pick. The
// picks are made in the "Escolhas" step; what the player typed in the free fields answers what is left.
func (x *deriver) checkProficiencyPicks() {
	if len(x.classes) == 0 {
		return
	}
	tools, languages := 0, 0
	if x.race != nil {
		languages += x.race.LanguageChoices
	}
	for _, t := range x.ownedTraits() {
		languages += t.LanguageChoices
	}
	if bg, ok := x.c.backgrounds[x.b.Background]; ok {
		languages += bg.LanguageChoices
	}
	tools = x.toolPicksAsked()
	if n := tools - len(x.pickedTools) - len(x.b.ToolProficiencies); n > 0 {
		x.openProficiencyPick(OpenChoiceTools, n, "ferramenta", "ferramentas")
	}
	x.checkBackgroundGroupPicks()
	if n := languages - len(x.pickedLanguages) - len(x.b.Languages); n > 0 {
		x.openProficiencyPick(OpenChoiceLanguages, n, "idioma", "idiomas")
	}
}

// toolPicksAsked is how many tool picks the sheet's choices of tools ask in all, each choice asking what
// requiredToolPicks says, as the choice does.
func (x *deriver) toolPicksAsked() int {
	had := map[string]bool{}
	for _, k := range x.grantedProficiencies() {
		had[k] = true
	}
	total := 0
	var seen []string
	for _, a := range x.active {
		if a.effect.Type != "choice" || a.effect.Choice != "tool" || a.effect.DailyChoice || slices.Contains(seen, a.owner) {
			continue
		}
		seen = append(seen, a.owner)
		picks, from, open, ok := x.c.toolSpec(a.owner)
		if !ok {
			continue
		}
		var held []string
		for _, p := range x.pickedTools {
			if p.Owner == a.owner {
				held = append(held, p.Value)
			}
		}
		total += requiredToolPicks(picks, x.c.toolsOffered(from, open), had, held)
	}
	return total
}

// requiredToolPicks is requiredPicks for a choice of tools: the proficiencies it offers that the sheet does not have
// are the picks to make, and the picks it holds are made.
func requiredToolPicks(picks int, offered []string, had map[string]bool, held []string) int {
	var pickable, made []string
	for _, k := range offered {
		switch {
		case slices.Contains(held, k):
			made = append(made, k)
		case !had[k]:
			pickable = append(pickable, k)
		}
	}
	return requiredPicks(picks, pickable, made)
}

// openProficiencyPick records a language or tool pick the sheet has not made: an open choice, and an issue that sends the
// player to the step that makes it.
func (x *deriver) openProficiencyPick(kind string, missing int, one, many string) {
	x.d.OpenChoices = append(x.d.OpenChoices, OpenChoice{Kind: kind, Missing: missing})
	noun := many
	if missing == 1 {
		noun = one
	}
	x.issue(IssueMissing, choiceKeysField, "Falta escolher %d %s: faça a escolha no passo “Escolhas”.", missing, noun)
}

// proficiencyChoices adds the language and tool choices that owner asks (and lists in g).
func (cb *choiceBuilder) proficiencyChoices(g *groupBuild, owner string) {
	if owner == "" {
		return
	}
	cb.languageChoice(g, owner)
	cb.toolChoice(g, owner)
}

// languageChoice is the pick of the languages a race, a trait or a background gives: any language of the content,
// but the ones the character speaks already and the ones another pick took.
func (cb *choiceBuilder) languageChoice(g *groupBuild, owner string) {
	c := cb.c
	picks := c.languageGrant(owner)
	if picks <= 0 {
		return
	}
	key := LanguagePickKey(owner)
	title, hint := "Idioma", "Um idioma à escolha, entre os do jogo."
	if picks > 1 {
		title, hint = "Idiomas", fmt.Sprintf("%d idiomas à escolha, entre os do jogo.", picks)
	}
	ch := cb.addChoice(g, &Choice{Key: key, FeatureKey: owner, Kind: ChoiceKindOptions, TitlePT: title, HintPT: hint + " Os que o personagem já fala não entram.", Picks: picks, family: familyLanguagePick, Advisory: true})
	keys := make([]string, 0, len(c.languages))
	for k := range c.languages {
		keys = append(keys, k)
	}
	slices.SortFunc(keys, func(a, b string) int { return strings.Compare(foldPT(c.namePT(a)), foldPT(c.namePT(b))) })
	spoken := cb.spokenLanguages()
	for _, k := range keys {
		o := ChoiceOption{Key: k, Stored: ScopedChoice(key, k), NamePT: c.namePT(k)}
		if spoken[k] {
			blockOption(&o, "Você já fala este idioma.")
		}
		ch.Options = append(ch.Options, o)
	}
	cb.takeScoped(ch)
	unmeetBlockedPicks(ch)
	cb.answerWithTyped(ch, len(cb.b.Languages), &cb.typedLanguages)
}

// toolChoice is the pick of the tool proficiencies a `choice` effect of kind tool offers: the ones it lists or its filter names (any
// tool without a list), but the ones the character has already and the ones another pick took.
func (cb *choiceBuilder) toolChoice(g *groupBuild, owner string) {
	c := cb.c
	picks, from, open, ok := c.toolSpec(owner)
	if !ok {
		return
	}
	key := ToolPickKey(owner)
	title, hint := "Ferramenta", "Uma proficiência com ferramenta à escolha."
	if picks > 1 {
		title, hint = "Ferramentas", fmt.Sprintf("%d proficiências com ferramentas à escolha.", picks)
	}
	from = c.toolsOffered(from, open)
	slices.SortFunc(from, func(a, b string) int {
		return strings.Compare(foldPT(c.proficiencyNamePT(a)), foldPT(c.proficiencyNamePT(b)))
	})
	had := cb.toolsHad()
	var options []ChoiceOption
	for _, k := range from {
		o := ChoiceOption{Key: k, Stored: ScopedChoice(key, k), NamePT: c.proficiencyNamePT(k)}
		if had[k] {
			blockOption(&o, "Você já tem esta proficiência.")
		}
		options = append(options, o)
	}
	// The choice asks for no more than the sheet can make: the proficiencies it does not have, and the picks it holds.
	ch := cb.addChoice(g, &Choice{Key: key, FeatureKey: owner, Kind: ChoiceKindOptions, TitlePT: title, HintPT: hint + " As que o personagem já tem não entram.", Picks: requiredToolPicks(picks, from, had, cb.scoped[key]), family: familyToolPick, Advisory: true, Options: options})
	cb.takeScoped(ch)
	unmeetBlockedPicks(ch)
	cb.answerWithTyped(ch, len(cb.b.ToolProficiencies), &cb.typedTools)
}

// blockOption makes an option one that cannot be taken, and says why.
func blockOption(o *ChoiceOption, reason string) {
	o.Prerequisites = append(o.Prerequisites, takenPrerequisite(reason))
	o.ReasonPT = prerequisitesReason(o.Prerequisites)
}

// answerWithTyped lets what the player typed in the free field of a sheet that predates the picks answer the picks
// left: used counts the typed entries the choices before this one took.
func (cb *choiceBuilder) answerWithTyped(ch *Choice, typed int, used *int) {
	n := min(max(typed-*used, 0), ch.Picks-len(ch.Picked))
	*used += n
	ch.legacy = n
}

// spokenLanguages are the languages the character speaks without a pick: the race's and a custom background's.
func (cb *choiceBuilder) spokenLanguages() map[string]bool {
	out := map[string]bool{}
	if r := cb.x.race; r != nil {
		for _, l := range r.Languages {
			out[l] = true
		}
	}
	if cb.b.Background == "" {
		for _, k := range cb.b.CustomBackgroundProficiencies {
			out[k] = true
		}
	}
	return out
}

// toolsHad are the tool proficiencies the character has without a pick: the class's, the race's and the
// background's.
func (cb *choiceBuilder) toolsHad() map[string]bool {
	out := map[string]bool{}
	for _, k := range cb.x.grantedProficiencies() {
		out[k] = true
	}
	return out
}
