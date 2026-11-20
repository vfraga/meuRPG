package rules

import (
	"fmt"
	"slices"
	"strings"
)

// A feat of the player's choice (MR-025). A race, subrace, background, class or
// subclass feature, a trait or a feat of the table may carry a `choice` effect
// with `choice: "feat"`: the character picks `count` feats, from the effect's
// `from` list or, without one, from every feat of the content. The engine offers
// it like any other choice (choicegroups.go) and the pick is stored the same way
// as the others: "<owner key>#feat=<feat key>" in Build.FeatureChoices, where
// the owner is the thing that grants it. The feat applies exactly like one in
// Build.Feats (its effects, its prerequisite, its place among the features) for
// as long as the owner is on the sheet, and it is never on the sheet twice.
//
// The table rule "Talentos" (feats_allowed) does not gate this: the table wrote
// the grant itself. A feat's ability increase is not applied through a choice:
// the player notes it in the extra ability bonuses.

// featChoiceSuffix ends the key of the choice a feat grant asks.
const featChoiceSuffix = textSeparator + "feat"

// FeatChoiceKey is the key of the choice of a feat that owner grants.
func FeatChoiceKey(owner string) string { return owner + featChoiceSuffix }

// choiceFeat is a feat picked through a choice, and the owner that granted it.
type choiceFeat struct {
	Feat, Owner string
	// At is the pick's place in Build.FeatureChoices.
	At int
}

// featChoiceSpec is what the feat choices of an owner ask in all: how many feats
// and the list they come from (empty: any feat).
func (c *content) featChoiceSpec(owner string) (picks int, from []string, ok bool) {
	return c.choiceSpec(owner, "feat")
}

// choiceSpec is what the `choice` effects of a kind that owner has ask in all: how many picks and the list they
// come from (empty: any).
func (c *content) choiceSpec(owner, kind string) (picks int, from []string, ok bool) {
	open := false
	for _, e := range c.effectChoices(owner, kind) {
		ok = true
		picks += max(e.Count, 1)
		if len(e.From) == 0 {
			open = true
			continue
		}
		for _, k := range e.From {
			if !slices.Contains(from, k) {
				from = append(from, k)
			}
		}
	}
	if open {
		from = nil
	}
	return picks, from, ok
}

// pickedChoiceFeats are the feats the picks of Build.FeatureChoices give: the ones
// whose owner is on the sheet (owned), that the owner's list offers, up to the
// number it grants, none the sheet already has. In the order of the picks.
func (c *content) pickedChoiceFeats(b Build, owned func(string) bool) []choiceFeat {
	var out []choiceFeat
	var count map[string]int
	for at, k := range b.FeatureChoices {
		owner, value, ok := featPickParts(k)
		if !ok || !owned(owner) || !c.featPickFits(b, out, count[owner], owner, value) {
			continue
		}
		if count == nil {
			count = map[string]int{}
		}
		count[owner]++
		out = append(out, choiceFeat{Feat: value, Owner: owner, At: at})
	}
	return out
}

// featPickParts reads a stored pick as the choice of a feat: the owner that
// grants it and the feat, and whether it is one.
func featPickParts(stored string) (owner, value string, ok bool) {
	choiceKey, value, ok := SplitScopedChoice(stored)
	if !ok || !strings.HasPrefix(value, "feat:") {
		return "", "", false
	}
	owner, isFeat := strings.CutSuffix(choiceKey, featChoiceSuffix)
	return owner, value, isFeat
}

// featPickFits says the owner grants a feat choice with room left (taken is
// how many of its picks are kept already), the feat exists and is on the
// owner's list, and neither the sheet nor an earlier pick has it.
func (c *content) featPickFits(b Build, kept []choiceFeat, taken int, owner, value string) bool {
	picks, from, granted := c.featChoiceSpec(owner)
	if !granted || taken >= picks || c.feats[value] == nil {
		return false
	}
	if len(from) > 0 && !slices.Contains(from, value) {
		return false
	}
	return !slices.Contains(b.Feats, value) && !slices.ContainsFunc(kept, func(cf choiceFeat) bool { return cf.Feat == value })
}

// ownedBy says whether a key is on a derived sheet: a feature, trait or feat it
// lists, or the race, subrace, background, class or subclass.
func ownedBy(b Build, d Derived) func(string) bool {
	return func(key string) bool {
		if key == "" {
			return false
		}
		if key == b.Race || key == b.Subrace || key == b.Background {
			return true
		}
		for _, cl := range b.Classes {
			if key == cl.Class || key == cl.Subclass {
				return true
			}
		}
		return slices.ContainsFunc(d.Features, func(f Feature) bool { return f.Key == key })
	}
}

// appliedBy is ownedBy for what a prerequisite asks: an entry the sheet lists but has lost does not count, as it gives
// nothing (an option that needs a lost option is lost too).
func appliedBy(b Build, d Derived) func(string) bool {
	owns := ownedBy(b, d)
	return func(key string) bool {
		return owns(key) && !slices.ContainsFunc(d.Issues, func(is Issue) bool { return is.Lost != nil && is.Lost.Key == key })
	}
}

// choiceFeats adds to the sheet the feats its picks give, after the feats of
// Build.Feats. A feat whose prerequisite is no longer met is listed and not applied.
func (x *deriver) choiceFeats(owned map[string]bool, add func(string), feature func(key, name, source string, level int, desc []string)) {
	c := x.c
	for _, cf := range c.pickedChoiceFeats(x.b, func(k string) bool { return owned[k] }) {
		f := c.feats[cf.Feat]
		if x.inactiveFeats[cf.Feat] {
			x.lostIssue(IssueFeatPrerequisite, fmt.Sprintf("%s[%d]", choiceKeysField, cf.At), x.lost[cf.Feat])
		} else {
			add(cf.Feat)
		}
		feature(cf.Feat, f.Name, cf.Feat, 0, f.Desc)
		source := c.namePT(cf.Owner)
		for _, of := range x.d.Features {
			if of.Key == cf.Owner && of.SourcePT != "" {
				source = of.SourcePT
				break
			}
		}
		x.d.Features[len(x.d.Features)-1].SourcePT = "Talento · " + source
	}
}

// background adds the choices of the background: a feat it grants.
func (cb *choiceBuilder) background() {
	bg, ok := cb.c.backgrounds[cb.b.Background]
	if !ok {
		return
	}
	g := cb.group(ChoiceOriginBackground, bg.Key, 0, 0)
	cb.featChoice(g, bg.Key)
	cb.featChoice(g, bg.Feature.Key)
	cb.backgroundGroups(g, bg.Key)
}

// featOwners adds the choices the feats of the sheet ask (a feat that grants another).
func (cb *choiceBuilder) featOwners() {
	for _, key := range cb.b.Feats {
		if cb.c.feats[key] == nil {
			continue
		}
		cb.featChoice(cb.group(ChoiceOriginFeat, key, 0, 0), key)
	}
	// A feat taken through a choice asks its own choices too.
	if !slices.ContainsFunc(cb.b.FeatureChoices, func(k string) bool { _, _, ok := featPickParts(k); return ok }) {
		return
	}
	for _, cf := range cb.c.pickedChoiceFeats(cb.b, ownedBy(cb.b, *cb.derive())) {
		cb.featChoice(cb.group(ChoiceOriginFeat, cf.Feat, 0, 0), cf.Feat)
	}
}

// featChoice adds the choice of a feat that owner grants, if any: the options are
// the feats of the list (or of the content), each with whether the character
// qualifies. A feat the sheet already has is not an option; one the table retired
// or switched off stays only for whoever already picked it.
func (cb *choiceBuilder) featChoice(g *groupBuild, owner string) {
	c := cb.c
	cb.grantAbilityChoice(g, owner) // every owner of a choice passes here
	cb.proficiencyChoices(g, owner)
	cb.featPickChoices(g, owner)
	picks, from, ok := c.featChoiceSpec(owner)
	if !ok || owner == "" {
		return
	}
	key := FeatChoiceKey(owner)
	ch := cb.addChoice(g, &Choice{
		Key: key, FeatureKey: owner, Kind: ChoiceKindOptions, TitlePT: "Talento", Picks: picks, family: "feat",
		HintPT: "Um talento à escolha. Um talento pede o que o texto dele diz; os que o personagem ainda não cumpre aparecem indisponíveis.",
	})
	// addChoice wrote the singular title; several picks take the plural.
	if picks > 1 {
		ch.TitlePT = "Talentos"
		ch.HintPT = fmt.Sprintf("%d talentos à escolha. Um talento pede o que o texto dele diz; os que o personagem ainda não cumpre aparecem indisponíveis.", picks)
	}
	d := cb.derive()
	pickedNow := cb.scoped[key]
	for _, e := range (&Content{c: c}).Feats() {
		if cb.offersFeat(e, from, pickedNow) {
			ch.Options = append(ch.Options, cb.featOption(e, key, *d))
		}
	}
	cb.takeScoped(ch)
	unmeetBlockedPicks(ch)
}

// offersFeat says a feat is an option of the choice: on its list (if it has
// one), not on the sheet already, and live or picked already.
func (cb *choiceBuilder) offersFeat(e FeatEntry, from, pickedNow []string) bool {
	if len(from) > 0 && !slices.Contains(from, e.Key) {
		return false
	}
	if slices.Contains(cb.b.Feats, e.Key) {
		return false
	}
	if slices.Contains(pickedNow, e.Key) {
		return true
	}
	// A player is not offered a feat asking only for races they cannot read: they cannot meet it.
	return !e.Archived && !e.Off && !cb.c.asksOnlyHiddenRaces(e.Prerequisite, cb.b)
}

// featOption is a feat as an option of the choice key, blocked with its reason
// when the sheet d does not meet its prerequisite.
func (cb *choiceBuilder) featOption(e FeatEntry, key string, d Derived) ChoiceOption {
	c := cb.c
	o := ChoiceOption{Key: e.Key, Stored: ScopedChoice(key, e.Key), NamePT: e.NamePT, SummaryPT: featSummaryPT(c, e, cb.b)}
	unmet := c.unmetPrerequisite(e, cb.b, d)
	if len(unmet) == 0 {
		return o
	}
	var reasons []string
	for _, u := range unmet {
		reasons = append(reasons, c.featUnmetPT(u, cb.b))
	}
	o.ReasonPT = "Você ainda não cumpre o pré-requisito: " + strings.Join(reasons, " ")
	o.Prerequisites = append(o.Prerequisites, OptionPrerequisite{Kind: PrerequisiteFeature, Met: false, ReasonPT: o.ReasonPT})
	return o
}

// unmeetBlockedPicks marks unmet each pick whose option is blocked.
func unmeetBlockedPicks(ch *Choice) {
	for _, k := range ch.Picked {
		for _, o := range ch.Options {
			if o.Key == k && o.Blocked() {
				ch.Unmet = append(ch.Unmet, k)
			}
		}
	}
}

// featSummaryPT is the feat's text as a choice card shows it, with what it asks first.
func featSummaryPT(c *content, e FeatEntry, b Build) string {
	text := e.DescPT
	if len(text) == 0 {
		text = e.Desc
	}
	var parts []string
	if asks := c.featAsksPT(e.Prerequisite, b); asks != "" {
		parts = append(parts, "Pede: "+asks+".")
	}
	parts = append(parts, text...)
	return strings.Join(parts, " ")
}

// featAsksPT is what a feat asks of a character, in a phrase ("Força 13 e nível 4 ou mais").
// In a player's view it names no hidden race or feature.
func (c *content) featAsksPT(p FeatPrerequisite, b Build) string {
	out := c.abilityAsksPT(p)
	if p.Proficiency != "" {
		out = append(out, "proficiência em "+c.namePT(p.Proficiency))
	}
	if p.Spellcasting {
		out = append(out, "conseguir conjurar magias")
	}
	if ask := c.raceAskPT(p, b); ask != "" {
		out = append(out, ask)
	}
	if p.Size != "" {
		out = append(out, "ter tamanho "+strings.ToLower(sizeNamePT[p.Size]))
	}
	if p.LacksFeature != "" {
		if c.hiddenTo(b, p.LacksFeature) {
			out = append(out, hiddenAskPT)
		} else {
			out = append(out, "não ter "+c.namePT(p.LacksFeature))
		}
	}
	if p.Level > 0 {
		out = append(out, fmt.Sprintf("nível %d ou mais", p.Level))
	}
	out = append(out, c.knowledgeAsksPT(p, b)...)
	return strings.Join(out, " e ")
}

// knowledgeAsksPT are the class level, the Pact Boon, the spell and the options a prerequisite asks. A key a player may
// not read is not named.
func (c *content) knowledgeAsksPT(p FeatPrerequisite, b Build) []string {
	var out []string
	if p.ClassKey != "" {
		if c.hiddenTo(b, p.ClassKey) {
			out = append(out, hiddenAskPT)
		} else {
			out = append(out, fmt.Sprintf("nível %d de %s", p.ClassLevel, c.namePT(p.ClassKey)))
		}
	}
	name := func(prefix, key string) {
		if c.hiddenTo(b, key) {
			out = append(out, hiddenAskPT)
		} else {
			out = append(out, prefix+optionLabel(c.namePT(key)))
		}
	}
	if p.RequiresPact != "" {
		name("a Dádiva ", p.RequiresPact)
	}
	if p.RequiresSpell != "" {
		name("conhecer ", p.RequiresSpell)
	}
	for _, k := range p.RequiresOptions {
		name("ter escolhido ", k)
	}
	return out
}

// abilityAsksPT are the ability scores a prerequisite asks: all the minimums, then one of the any-of scores.
func (c *content) abilityAsksPT(p FeatPrerequisite) []string {
	var out, anyOf []string
	for _, a := range AllAbilities() {
		if least := p.Minimums[a]; least > 0 {
			out = append(out, fmt.Sprintf("%s %d", c.namePT(string(a)), least))
		}
	}
	for _, a := range AllAbilities() {
		if least := p.AnyOf[a]; least > 0 {
			anyOf = append(anyOf, fmt.Sprintf("%s %d", c.namePT(string(a)), least))
		}
	}
	if len(anyOf) > 0 {
		out = append(out, strings.Join(anyOf, " ou "))
	}
	return out
}

// raceAskPT is the race part of a prerequisite ("ser Elfo ou Anão"), empty when it asks for none; the races
// the view may not name are left out, and a text that names nothing stands for them when none is left.
func (c *content) raceAskPT(p FeatPrerequisite, b Build) string {
	asked := prerequisiteRaces(p)
	if len(asked) == 0 {
		return ""
	}
	if races := c.shownRaces(b, asked); len(races) > 0 {
		return "ser " + c.racesPT(races)
	}
	return hiddenAskPT
}

// racesPT are the races or subraces in Portuguese: "A", "A ou B", "A, B ou C".
func (c *content) racesPT(keys []string) string {
	names := make([]string, len(keys))
	for i, k := range keys {
		names[i] = c.namePT(k)
	}
	last := len(names) - 1
	if last <= 0 {
		return strings.Join(names, "")
	}
	return strings.Join(names[:last], ", ") + " ou " + names[last]
}

// hiddenAskPT and hiddenUnmetPT stand for a condition of a feat that a player may not read; they name nothing.
const (
	hiddenAskPT   = "uma condição que a mesa não mostra"
	hiddenUnmetPT = "Pede uma condição que a mesa não mostra."
)

// hiddenTo says the view is a player's and the key is one they never receive, which the sheet b does not have.
func (c *content) hiddenTo(b Build, key string) bool {
	return c.player && c.hidden(key) && !c.sheetHas(b, key)
}

// shownRaces are the races or subraces of keys that the view may name: all of them, except in a
// player's view the hidden ones that are not the race or subrace of the sheet b.
func (c *content) shownRaces(b Build, keys []string) []string {
	if !c.player {
		return keys
	}
	return slices.DeleteFunc(slices.Clone(keys), func(k string) bool { return c.hiddenTo(b, k) })
}

// asksOnlyHiddenRaces says the view is a player's and the prerequisite asks for races, none of which
// the player may read: the player's sheet cannot meet it, and the text must not read as no requirement.
func (c *content) asksOnlyHiddenRaces(p FeatPrerequisite, b Build) bool {
	asked := prerequisiteRaces(p)
	return c.player && len(asked) > 0 && len(c.shownRaces(b, asked)) == 0
}

// featUnmetPT says in a sentence what a character lacks for one condition of a feat.
func (c *content) featUnmetPT(u FeatUnmet, b Build) string {
	switch u.Kind {
	case FeatUnmetAbilityMinimum, FeatUnmetAbilityAnyOf:
		var parts []string
		for _, am := range u.Abilities {
			parts = append(parts, fmt.Sprintf("%s %d", c.namePT(string(am.Ability)), am.Minimum))
		}
		if u.Kind == FeatUnmetAbilityAnyOf {
			return "Precisa de " + strings.Join(parts, " ou ") + "."
		}
		return "Precisa de " + strings.Join(parts, " e ") + "."
	case FeatUnmetProficiency:
		return "Precisa de proficiência em " + c.namePT(u.Key) + "."
	case FeatUnmetSpellcasting:
		return "Precisa conseguir conjurar magias."
	case FeatUnmetRace:
		asked := u.Keys
		if u.Key != "" {
			asked = []string{u.Key}
		}
		if shown := c.shownRaces(b, asked); len(shown) > 0 {
			return "Só para " + c.racesPT(shown) + "."
		}
		return hiddenUnmetPT
	case FeatUnmetSize:
		return "Só para tamanho " + strings.ToLower(sizeNamePT[u.Key]) + "."
	case FeatUnmetLacksFeature:
		if c.hiddenTo(b, u.Key) {
			return hiddenUnmetPT
		}
		return "Não pode ter " + c.namePT(u.Key) + "."
	case FeatUnmetLevel:
		return fmt.Sprintf("Precisa de nível %d ou mais.", u.Value)
	case FeatUnmetClassLevel, FeatUnmetPact, FeatUnmetSpell, FeatUnmetOption:
		return c.knowledgeUnmetPT(u, b)
	}
	return ""
}

// knowledgeUnmetPT says what a character lacks of a class level, a Pact Boon, a spell or an option a feat asks for.
func (c *content) knowledgeUnmetPT(u FeatUnmet, b Build) string {
	if c.hiddenTo(b, u.Key) {
		return hiddenUnmetPT
	}
	if u.Kind == FeatUnmetClassLevel {
		return fmt.Sprintf("Precisa de nível %d de %s.", u.Value, c.namePT(u.Key))
	}
	prefix := map[string]string{FeatUnmetPact: "Precisa da Dádiva do Pacto ", FeatUnmetSpell: "Precisa conhecer a magia ", FeatUnmetOption: "Precisa ter escolhido "}[u.Kind]
	return prefix + optionLabel(c.namePT(u.Key)) + "."
}

// featPickChoices adds the choices a feat of the table asks besides the feats it grants:
// its options, as a class feature's has them (a choice of features, or an option set),
// and the named picks of an ability or a damage type.
func (cb *choiceBuilder) featPickChoices(g *groupBuild, owner string) {
	c := cb.c
	f := c.feats[owner]
	if f == nil {
		return
	}
	if options := c.featOptionKeys(f); len(options) > 0 && f.OptionsChoose > 0 {
		cb.optionChoice(g, owner, f.OptionsChoose, options)
	}
	for _, e := range c.namedPickChoices(owner) {
		cb.namedPickChoice(g, owner, e)
	}
}

// namedPickChoice is the choice of one named pick: the abilities or the damage types the choice offers
// (all of them without a list), stored as "<feat key>#<name>=<value>".
func (cb *choiceBuilder) namedPickChoice(g *groupBuild, owner string, e *Effect) {
	c := cb.c
	key := PickChoiceKey(owner, e.Name)
	ch := &Choice{Key: key, FeatureKey: owner, Kind: ChoiceKindOptions, Picks: max(e.Count, 1)}
	var values []string
	if e.Choice == pickAbility {
		ch.TitlePT, ch.HintPT = c.pickTitle(owner, e, "Habilidade", "ª"), "A habilidade que o talento "+c.namePT(owner)+" usa."
		for _, a := range AllAbilities() {
			if len(e.From) == 0 || slices.Contains(e.From, string(a)) {
				values = append(values, abilityValue(a))
			}
		}
	} else {
		ch.TitlePT, ch.HintPT = c.pickTitle(owner, e, "Tipo de dano", "º"), "O tipo de dano que o talento "+c.namePT(owner)+" usa."
		for _, d := range c.damageTypeEntries() {
			if len(e.From) == 0 || slices.Contains(e.From, d.Key) {
				values = append(values, d.Key)
			}
		}
	}
	for _, v := range values {
		name := c.namePT(v)
		if e.Choice == pickAbility {
			name = c.namePT(strings.TrimPrefix(v, abilityPrefix))
		}
		ch.Options = append(ch.Options, ChoiceOption{Key: v, Stored: ScopedChoice(key, v), NamePT: name})
	}
	cb.addChoice(g, ch)
	cb.takeScoped(ch)
}

// pickTitle is the title of a named pick: the kind's name, numbered in the order of the feat's effects when
// the feat has two or more picks of that kind ("Habilidade (1ª)"). It never shows the master's own name.
func (c *content) pickTitle(owner string, e *Effect, title, ordinal string) string {
	n, at := 0, 0
	for _, o := range c.namedPickChoices(owner) {
		if o.Choice != e.Choice {
			continue
		}
		n++
		if o == e {
			at = n
		}
	}
	if n == 1 { // a single pick of a kind keeps the plain title
		return title
	}
	return fmt.Sprintf("%s (%d%s)", title, at, ordinal)
}

// damageTypeEntries are the damage types of the content, sorted by Portuguese name.
func (c *content) damageTypeEntries() []NamedEntry {
	_, _, damageTypes := c.namedEntries()
	return damageTypes
}
