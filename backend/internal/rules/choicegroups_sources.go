package rules

import (
	"fmt"
	"slices"
	"strconv"
	"strings"

	"github.com/PuraFome/meuRPG/backend/internal/rules/srd51"
)

// race adds the choices of the race and the subrace: a trait that offers options
// (the dragonborn's ancestry), a bonus cantrip (the high elf's) and the abilities
// a race lets the player raise (the half-elf's).
// The numbers the SRD gives: the Half-Elf's two +1s, the Book of Shadows' three cantrips, a Wizard's two signature spells.
const (
	halfElfBonusCount = 2
	tomeCantrips      = 3
	bookRitualPicks   = 2
	signatureSpells   = 2
)

func (cb *choiceBuilder) race() {
	x := cb.x
	if x.race == nil {
		return
	}
	cb.raceSource(x.race.Key, x.raceTraitKeys())
	if x.subrace != nil {
		cb.raceSource(x.subrace.Key, x.subrace.Traits)
	}
	if !x.replacesRaceBonuses() {
		cb.abilityChoice(x.race)
	}
}

// raceSource adds the choices of the race or the subrace key: a feat it grants,
// and those of its traits.
func (cb *choiceBuilder) raceSource(key string, traits []string) {
	g := cb.group(ChoiceOriginRace, key, 0, 0)
	cb.featChoice(g, key)
	for _, tk := range traits {
		cb.traitChoices(g, tk)
	}
}

// traitChoices adds the choices of one trait: its options, its cantrips and a
// feat it grants.
func (cb *choiceBuilder) traitChoices(g *groupBuild, tk string) {
	c := cb.c
	tr := c.traits[tk]
	if tr == nil {
		return
	}
	if len(tr.Options) > 0 {
		choose := 1
		if es := c.effectChoices(tk, "feature"); len(es) > 0 && es[0].Count > 0 {
			choose = es[0].Count
		}
		cb.optionChoice(g, tk, choose, tr.Options)
	}
	for _, e := range c.effectChoices(tk, "cantrip") {
		cb.cantripChoice(g, tk, e)
	}
	cb.featChoice(g, tk)
}

// abilityChoice is the race's "+1 to two abilities" (SRD 5.1, Half-Elf, Ability
// Score Increase): the abilities are picks, and Derive adds the points. A table
// race's sets of bonuses are picks too (racebonuspicks.go).
func (cb *choiceBuilder) abilityChoice(r *srd51.Race) {
	c := cb.c
	if sets := c.raceBonusSets(r.Key); len(sets) > 0 {
		cb.bonusChoices(r.Key, sets)
		return
	}
	if r.AbilityBonusChoices == nil {
		return
	}
	g := cb.group(ChoiceOriginRace, r.Key, 0, 0)
	key := AbilityChoiceKey(r.Key)
	ch := cb.addChoice(g, &Choice{Key: key, FeatureKey: r.Key, Kind: ChoiceKindAbilities, TitlePT: "+1 em duas habilidades", Picks: r.AbilityBonusChoices.Choose})
	if r.AbilityBonusChoices.Choose != halfElfBonusCount {
		ch.TitlePT = fmt.Sprintf("+1 em %d habilidades", r.AbilityBonusChoices.Choose)
	}
	for _, k := range r.AbilityBonusChoices.From {
		a, ok := ability(k)
		if !ok {
			continue
		}
		ch.Options = append(ch.Options, ChoiceOption{
			Key: abilityValue(a), Stored: ScopedChoice(key, abilityValue(a)), NamePT: c.namePT(string(a)), SummaryPT: "+1",
		})
	}
	cb.takeScoped(ch)
}

// AbilityChoiceKey is the key of a race's choice of abilities.
func AbilityChoiceKey(raceKey string) string { return raceKey + textSeparator + "abilities" }

// class adds the choices of a class and its subclass, level by level.
func (cb *choiceBuilder) class(oc ownedClass) {
	c := cb.c
	idx := oc.index + 1
	for lvl := 1; lvl <= oc.level; lvl++ {
		classFeatures, _ := c.levelFeatures(oc.key, nil, lvl)
		cb.features(oc, idx, ChoiceOriginClass, oc.key, lvl, classFeatures, "")
		if oc.subclass != nil {
			_, subFeatures := c.levelFeatures("", oc.subclass, lvl)
			cb.features(oc, idx, ChoiceOriginSubclass, oc.subclass.Key, lvl, subFeatures, oc.subclass.Key)
		}
	}
	cb.invocations(oc, idx)
}

// features adds the choices of the features a class or subclass gains at one level.
func (cb *choiceBuilder) features(_ ownedClass, idx int, origin ChoiceOrigin, source string, level int, keys []string, subclass string) {
	c := cb.c
	if len(keys) == 0 {
		return
	}
	g := cb.group(origin, source, level, idx)
	for _, fk := range keys {
		if c.features[fk] == nil {
			continue
		}
		cb.featureChoices(g, fk, subclass)
		cb.featureSpecialChoice(g, fk)
	}
}

// featureChoices adds the choices every feature may ask: its options, its
// cantrips and a feat it grants.
func (cb *choiceBuilder) featureChoices(g *groupBuild, fk, subclass string) {
	c := cb.c
	for _, d := range c.featureGains([]string{fk}, subclass).choices {
		cb.optionChoice(g, d.feature, d.choose, d.options)
	}
	for _, e := range c.effectChoices(fk, "cantrip") {
		cb.cantripChoice(g, fk, e)
	}
	cb.featChoice(g, fk)
}

// featureSpecialChoice adds the choice of the SRD features that ask their own:
// the ranger's enemies and terrains, the warlock's arcanum and tome, the
// wizard's mastery and signature spells.
func (cb *choiceBuilder) featureSpecialChoice(g *groupBuild, fk string) {
	switch {
	case strings.HasPrefix(fk, "feature:favored-enemy-") && !isTableKey(fk):
		cb.favoredEnemy(g, fk)
	case strings.HasPrefix(fk, "feature:natural-explorer-") && !isTableKey(fk):
		cb.naturalExplorer(g, fk)
	case strings.HasPrefix(fk, "feature:mystic-arcanum-") && !isTableKey(fk):
		cb.arcanum(g, fk)
	case fk == "feature:spell-mastery":
		cb.spellMastery(g)
	case fk == "feature:signature-spell":
		cb.signatureSpells(g)
	case fk == pactBoonKey:
		if slices.Contains(cb.plain, "feature:pact-of-the-tome") {
			cb.tomeCantrips(g)
		}
	}
}

// invocations adds the warlock's Eldritch Invocations: one choice for all of them,
// as many as the class table says (SRD 5.1, Warlock), in the group of the level the
// last one came at.
func (cb *choiceBuilder) invocations(oc ownedClass, idx int) {
	c := cb.c
	inv := c.features[invocationsFeature]
	rows := c.classLevels[oc.key]
	if inv == nil || len(inv.Options) == 0 || oc.level < 1 || oc.level > len(rows) {
		return
	}
	known := invocationsKnown(rows[oc.level-1])
	if known < 1 {
		return
	}
	last := oc.level
	for last > 1 && invocationsKnown(rows[last-2]) == known {
		last--
	}
	g := cb.group(ChoiceOriginClass, oc.key, last, idx)
	cb.optionChoice(g, invocationsFeature, known, inv.Options)
	if slices.Contains(cb.plain, bookInvocationKey) {
		cb.bookRituals(g)
	}
}

// enemyTitles and terrainTitles are the titles of the 2nd and 3rd pick of a
// ranger feature that comes again at levels 6, 10 and 14.
var (
	enemyTitles   = []string{"Inimigo Favorito", "Segundo inimigo favorito", "Terceiro inimigo favorito"}
	terrainTitles = []string{"Explorador Natural", "Segundo terreno favorito", "Terceiro terreno favorito"}
)

// featureOrdinal reads the 1, 2 or 3 in "feature:favored-enemy-2-types".
func featureOrdinal(key, prefix string) int {
	rest := strings.TrimPrefix(key, prefix)
	n, _ := strconv.Atoi(rest[:1])
	return n
}

// favoredEnemy adds a favored enemy (SRD 5.1, Ranger, Favored Enemy): a type of
// creature or two humanoid races, and a language the enemy speaks, if any.
func (cb *choiceBuilder) favoredEnemy(g *groupBuild, fk string) {
	c := cb.c
	if c.choices == nil {
		return
	}
	n := min(max(featureOrdinal(fk, "feature:favored-enemy-"), 1), len(enemyTitles))
	title := enemyTitles[n-1]
	typeChoice := cb.addChoice(g, &Choice{
		Key: fk, FeatureKey: fk, Kind: ChoiceKindEnemy, TitlePT: title, PartPT: "Tipo de inimigo", Picks: 1, family: "enemy",
		HintPT: "Vantagem em testes de Sabedoria (Sobrevivência) para rastrear e de Inteligência para lembrar informações sobre essas criaturas. O idioma é um dos que esses inimigos falam, se falarem algum.",
	})
	types := slices.Clone(c.choices.enemyTypes)
	slices.SortFunc(types, func(a, b string) int { return strings.Compare(foldPT(c.namePT(a)), foldPT(c.namePT(b))) })
	for _, k := range types {
		typeChoice.Options = append(typeChoice.Options, ChoiceOption{Key: k, Stored: ScopedChoice(fk, k), NamePT: c.namePT(k)})
	}
	h := c.choices.enemyHumanoid
	typeChoice.Options = append(typeChoice.Options, ChoiceOption{
		Key: h, Stored: ScopedChoice(fk, h), NamePT: "Duas raças de humanoides…", NeedsText: true,
		SummaryPT: "Por exemplo, gnolls e orcs: escreva o nome de cada raça.",
	})
	cb.takeScoped(typeChoice)
	for i := 1; i <= humanoidRaceSlots; i++ {
		typeChoice.Texts = append(typeChoice.Texts, strings.TrimSpace(cb.b.FeatureChoiceText[ChoiceTextKey(fk, i)]))
	}
	if !typeChoice.needsTexts() {
		typeChoice.Texts = nil
	}

	langKey := fk + textSeparator + "language"
	lang := cb.addChoice(g, &Choice{Key: langKey, FeatureKey: fk, Kind: ChoiceKindLanguage, TitlePT: title, PartPT: "Idioma que falam", Picks: 1})
	langs := make([]string, 0, len(c.languages))
	for k := range c.languages {
		langs = append(langs, k)
	}
	slices.SortFunc(langs, func(a, b string) int { return strings.Compare(foldPT(c.namePT(a)), foldPT(c.namePT(b))) })
	for _, k := range langs {
		lang.Options = append(lang.Options, ChoiceOption{Key: k, Stored: ScopedChoice(langKey, k), NamePT: c.namePT(k)})
	}
	lang.Options = append(lang.Options, ChoiceOption{Key: LanguageNone, Stored: ScopedChoice(langKey, LanguageNone), NamePT: "Nenhum"})
	cb.takeScoped(lang)
}

// naturalExplorer adds a favored terrain (SRD 5.1, Ranger, Natural Explorer).
func (cb *choiceBuilder) naturalExplorer(g *groupBuild, fk string) {
	c := cb.c
	if c.choices == nil {
		return
	}
	n := min(max(featureOrdinal(fk, "feature:natural-explorer-"), 1), len(terrainTitles))
	ch := cb.addChoice(g, &Choice{
		Key: fk, FeatureKey: fk, Kind: ChoiceKindOptions, TitlePT: terrainTitles[n-1], Picks: 1, family: "terrain",
		HintPT: "Nesse terreno, seu bônus de proficiência dobra em testes de Inteligência e Sabedoria que usem uma perícia sua, e a viagem fica mais fácil.",
	})
	for _, k := range c.choices.terrains {
		ch.Options = append(ch.Options, ChoiceOption{Key: k, Stored: ScopedChoice(fk, k), NamePT: c.namePT(k)})
	}
	cb.takeScoped(ch)
}

// cantripChoice is a feature or trait that lets the character know one more cantrip
// from a class list (the high elf's wizard cantrip, the Land druid's druid cantrip).
// The pick is a spell the sheet knows besides the class's own numbers.
func (cb *choiceBuilder) cantripChoice(g *groupBuild, fk string, e *Effect) {
	c := cb.c
	var keys []string
	for _, entry := range c.spellEntries {
		if entry.Level == 0 && !cb.retiredPick(fk, entry) && slices.ContainsFunc(e.From, func(list string) bool { return c.onList(c.spells[entry.Key], list) }) {
			keys = append(keys, entry.Key)
		}
	}
	title := choiceTitle(c, fk)
	if len(e.From) == 1 && fk == "feature:bonus-cantrip" {
		title = "Truque adicional"
	}
	ch := cb.addChoice(g, &Choice{Key: fk, FeatureKey: fk, Kind: ChoiceKindSpells, TitlePT: title, Picks: max(e.Count, 1), viaCantrips: true})
	if len(e.From) == 1 {
		ch.HintPT = fmt.Sprintf("Um truque da lista de %s. Ele não conta nos truques da classe.", c.namePT(e.From[0]))
	}
	cb.spellOptions(ch, keys)
	cb.takeScoped(ch)
	cb.legacyCantrips(ch)
}

// legacyCantrips counts a sheet written before cantrips were picked here: it knows
// more cantrips than its class's number, and the surplus is the feature's.
func (cb *choiceBuilder) legacyCantrips(ch *Choice) {
	if len(ch.Picked) >= ch.Picks {
		return
	}
	d := cb.derive()
	allowed := 0
	for _, sc := range d.Spellcasting {
		allowed += sc.CantripsKnown
	}
	own := 0
	for _, k := range cb.b.Cantrips {
		if !cb.grantedCantrip(k) {
			own++
		}
	}
	surplus := max(own-allowed, 0) - cb.legacyTaken
	if surplus <= 0 {
		return
	}
	n := min(surplus, ch.Picks-len(ch.Picked))
	cb.legacyTaken += n
	// The surplus cantrips stay in Build.Cantrips; the choice is done without saying
	// which ones they are.
	ch.legacy = n
}

// grantedCantrip says whether a cantrip comes from a pick of a choice (and so
// counts against none of the class's numbers).
func (cb *choiceBuilder) grantedCantrip(key string) bool {
	for choiceKey, values := range cb.scoped {
		_ = choiceKey
		if slices.Contains(values, key) {
			return true
		}
	}
	return false
}

// spellOptions fills a choice's options with spells.
func (cb *choiceBuilder) spellOptions(ch *Choice, keys []string) {
	c := cb.c
	seen := map[string]bool{}
	entries := make([]SpellEntry, 0, len(keys))
	for _, k := range keys {
		if e, ok := c.spellEntries[k]; ok && !seen[k] {
			seen[k] = true
			entries = append(entries, e)
		}
	}
	slices.SortFunc(entries, func(a, b SpellEntry) int { return strings.Compare(foldPT(a.NamePT), foldPT(b.NamePT)) })
	for _, e := range entries {
		o := ChoiceOption{Key: e.Key, Stored: ScopedChoice(ch.Key, e.Key), NamePT: e.NamePT, SpellLevel: e.Level, SchoolPT: e.SchoolNamePT}
		for _, cl := range e.Classes {
			if _, ok := c.classes[cl]; ok && !isTableKey(cl) {
				o.ClassesPT = append(o.ClassesPT, c.namePT(cl))
			}
		}
		slices.Sort(o.ClassesPT)
		ch.Options = append(ch.Options, o)
	}
}

// tomeCantrips are the Pact of the Tome's three cantrips, from any class (SRD 5.1,
// Warlock, Pact of the Tome).
func (cb *choiceBuilder) tomeCantrips(g *groupBuild) {
	c := cb.c
	key := "feature:pact-of-the-tome" + textSeparator + "cantrips"
	ch := cb.addChoice(g, &Choice{
		Key: key, FeatureKey: "feature:pact-of-the-tome", Kind: ChoiceKindSpells, TitlePT: "Truques do Tomo", Picks: tomeCantrips,
		HintPT: "Livro das Sombras: 3 truques de qualquer classe, conjurados à vontade e fora da contagem de truques conhecidos.",
	})
	var keys []string
	for _, e := range c.spellEntries {
		if e.Level == 0 && !cb.retiredPick(key, e) {
			keys = append(keys, e.Key)
		}
	}
	cb.spellOptions(ch, keys)
	cb.takeScoped(ch)
}

// bookRituals are the two rituals of the Book of Ancient Secrets: 1st-level spells with
// the ritual tag, from any class (SRD 5.1, Warlock, Eldritch Invocations).
func (cb *choiceBuilder) bookRituals(g *groupBuild) {
	ch := cb.addChoice(g, &Choice{
		Key: bookRitualsKey, FeatureKey: bookInvocationKey, Kind: ChoiceKindSpells, TitlePT: "Rituais do Livro", Picks: bookRitualPicks,
		HintPT: "Livro das Sombras: 2 magias de 1º nível com a etiqueta de ritual, de qualquer classe. Só se conjuram como ritual, com o Livro das Sombras em mãos, e não contam nas magias conhecidas.",
	})
	var keys []string
	for _, e := range cb.c.spellEntries {
		if e.Level == 1 && e.Ritual && !cb.retiredPick(bookRitualsKey, e) {
			keys = append(keys, e.Key)
		}
	}
	cb.spellOptions(ch, keys)
	cb.takeScoped(ch)
}

// arcanum is a Mystic Arcanum (SRD 5.1, Warlock): one spell of the arcanum's level
// from the warlock's list, cast once per long rest.
func (cb *choiceBuilder) arcanum(g *groupBuild, fk string) {
	c := cb.c
	rest := strings.TrimPrefix(fk, "feature:mystic-arcanum-")
	level, err := strconv.Atoi(rest[:1])
	if err != nil {
		return
	}
	ch := cb.addChoice(g, &Choice{
		Key: fk, FeatureKey: fk, Kind: ChoiceKindSpells, TitlePT: "Arcana Mística", PartPT: fmt.Sprintf("Magia de %dº nível", level), Picks: 1,
		HintPT: "Conjurada uma vez por descanso longo, sem gastar espaço de magia.",
	})
	var keys []string
	for _, e := range c.spellEntries {
		if e.Level == level && !cb.retiredPick(fk, e) && cb.x.onWarlockList(e) {
			keys = append(keys, e.Key)
		}
	}
	cb.spellOptions(ch, keys)
	cb.takeScoped(ch)
}

// onWarlockList says whether a spell is on the warlock's list for this character:
// the class's own or the patron's expanded list.
func (x *deriver) onWarlockList(e SpellEntry) bool {
	if slices.Contains(e.Classes, classWarlock) {
		return true
	}
	for _, oc := range x.classes {
		if oc.key != classWarlock || oc.subclass == nil || !oc.subclass.ExpandedList {
			continue
		}
		if s := x.c.spells[e.Key]; s != nil && slices.Contains(s.Subclasses, oc.subclass.Key) {
			return true
		}
	}
	return false
}

// spellMastery is the wizard's Spell Mastery (SRD 5.1, Wizard): a 1st-level and a
// 2nd-level spell of the spellbook.
func (cb *choiceBuilder) spellMastery(g *groupBuild) {
	for level := 1; level <= 2; level++ {
		key := fmt.Sprintf("feature:spell-mastery%s%d", textSeparator, level)
		ch := cb.addChoice(g, &Choice{
			Key: key, FeatureKey: "feature:spell-mastery", Kind: ChoiceKindSpells, TitlePT: "Dominar Magia", PartPT: fmt.Sprintf("Magia de %dº nível", level), Picks: 1,
			HintPT: "Uma magia do grimório, conjurada no nível mais baixo sem gastar espaço de magia.",
		})
		cb.spellOptions(ch, cb.spellbookSpells(level))
		cb.takeScoped(ch)
	}
}

// signatureSpells are the wizard's Signature Spells (SRD 5.1, Wizard): two 3rd-level
// spells of the spellbook.
func (cb *choiceBuilder) signatureSpells(g *groupBuild) {
	ch := cb.addChoice(g, &Choice{
		Key: "feature:signature-spell", FeatureKey: "feature:signature-spell", Kind: ChoiceKindSpells, TitlePT: "Assinatura Mágica", Picks: signatureSpells,
		HintPT: "Duas magias de 3º nível do grimório, cada uma conjurada uma vez por descanso curto ou longo, sem gastar espaço de magia.",
	})
	cb.spellOptions(ch, cb.spellbookSpells(3))
	cb.takeScoped(ch)
}

// spellbookSpells are the wizard spells of a level the sheet knows.
func (cb *choiceBuilder) spellbookSpells(level int) []string {
	var out []string
	for _, k := range cb.b.SpellsKnown {
		if s := cb.c.spells[k]; s != nil && s.Level == level && cb.c.onList(s, "class:wizard") {
			out = append(out, k)
		}
	}
	return out
}

// takeScoped writes the scoped picks of a choice into it: the ones its options
// offer, up to the number it takes, in the order of the options; the rest overflow.
func (cb *choiceBuilder) takeScoped(ch *Choice) {
	index := map[string]int{}
	for i, o := range ch.Options {
		index[o.Key] = i
	}
	var picked []string
	for _, v := range cb.scoped[ch.Key] {
		if _, ok := index[v]; !ok {
			continue
		}
		cb.used[ScopedChoice(ch.Key, v)] = true
		if len(picked) < ch.Picks {
			picked = append(picked, v)
		} else {
			ch.Overflow = append(ch.Overflow, v)
		}
	}
	slices.SortFunc(picked, func(a, b string) int { return index[a] - index[b] })
	ch.Picked = picked
}
