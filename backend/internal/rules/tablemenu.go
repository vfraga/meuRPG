package rules

import (
	"fmt"
	"slices"
	"strings"
)

// The closed menu of what a table feature may do, as data (slice 10.3, MR-025,
// ADR-0018, section 4): the effect types and their fields, and every closed list
// a field takes (targets, modes, senses, recharges...) with Portuguese names, so
// the class editor never hard-codes a list. The validators (checkEffect and
// compileEffect) and this menu read the same lists; the tests prove each value
// the menu offers is accepted and that nothing else is.

// MenuType is one effect type of the menu.
type MenuType struct {
	// Type is the effect's `type`.
	Type string
	// NamePT and HintPT are the picker's label and its one-line explanation, in
	// Portuguese ("Só texto" has no type: a feature with no effects).
	NamePT, HintPT string
	// Fields are the fields the type reads, in the order the editor shows them;
	// every other field of the effect is refused.
	Fields []MenuField
}

// MenuField is a field of an effect type.
type MenuField struct {
	// Name is the proto field of TableEffect.
	Name string
	// Required says the effect is refused without it.
	Required bool
	// Kind is how the editor asks: "formula" (an Int formula), "condition" (a Bool
	// formula), "text", "number", "choice" (one of a closed list, named by List),
	// "choices" (several of it) or "tags".
	Kind string
	// List names the closed list of the menu the value comes from (a MenuList's
	// Name), empty when the value is free.
	List string
	// Min and Max bound a number; both zero when it is unbounded.
	Min, Max int
}

// MenuValue is a value of a closed list: the key the effect stores and the name
// in Portuguese. Hint is a short explanation where the name alone does not say.
type MenuValue struct {
	Key, NamePT, HintPT string
}

// MenuList is a closed list a field takes.
type MenuList struct {
	// Name is how fields point at it ("modifier_targets", "senses"...).
	Name   string
	Values []MenuValue
}

// MenuOptionSet is an SRD feature that offers options (a fighting style, an
// eldritch invocation): what a "choice" of kind "feature" may list in `from`.
type MenuOptionSet struct {
	// Key and NamePT name the offering feature; Choose is how many the SRD lets
	// the character pick.
	Key, NamePT string
	Choose      int
	Options     []MenuValue
}

// FormulaHelper is a function a formula may call.
type FormulaHelper struct {
	// Call is how it is written ("mod(\"int\")"), Returns what it gives ("number",
	// "text" or "yes/no") and HintPT says what it is for.
	Call, Returns, HintPT string
}

// EffectMenu is the whole menu.
type EffectMenu struct {
	Types []MenuType
	Lists []MenuList
	// OptionSets are the SRD features whose options a choice effect may offer.
	OptionSets []MenuOptionSet
	// Helpers are the functions a formula ("value", "when", "max", "prepared_max")
	// may use. ClassIndexes are the classes `classLevel(...)` takes: the SRD's and
	// the table's, by their key without "class:".
	Helpers      []FormulaHelper
	ClassIndexes []string
	// The limits the server enforces on features and effects.
	MaxFeaturesPerClass, MaxEffectsPerFeature, MaxTagsPerEffect int
	// ExtraAttackMin and ExtraAttackMax bound an extra_attack's count.
	ExtraAttackMin, ExtraAttackMax int
}

// Names of the closed lists of the menu.
const (
	ListModifierTargets   = "modifier_targets"
	ListModifierModes     = "modifier_modes"
	ListProficiencies     = "proficiency_targets"
	ListProficiencyLevels = "proficiency_levels"
	ListRollModes         = "roll_modes"
	ListRollTargets       = "roll_targets"
	ListSenses            = "senses"
	ListRecharges         = "recharges"
	ListEconomies         = "economies"
	ListChoiceKinds       = "choice_kinds"
	ListSkills            = "skills"
	ListLanguages         = "languages"
	ListTools             = "tools"
	ListTagPrefixes       = "tag_prefixes"
)

// menuTypes is the menu's types, in the picker's order, with the copy in
// Portuguese, in our own words. Only the types of overlayEffectTypes appear.
var menuTypes = []MenuType{
	{Type: "modifier", NamePT: "Modificador", HintPT: "Soma, define ou limita um número da ficha: CA, PV, deslocamento, ataque, dano, uma perícia.", Fields: []MenuField{
		{Name: "target", Required: true, Kind: "choice", List: ListModifierTargets},
		{Name: "mode", Required: true, Kind: "choice", List: ListModifierModes},
		{Name: "value", Required: true, Kind: "formula"},
		{Name: "when", Kind: "condition"},
		{Name: "tags", Kind: "tags", List: ListTagPrefixes},
		{Name: "text_pt", Kind: "text"},
	}},
	{Type: "proficiency", NamePT: "Proficiência", HintPT: "Dá proficiência numa perícia, num teste de resistência, numa ferramenta, arma ou armadura, ou na iniciativa.", Fields: []MenuField{
		{Name: "proficiency", Required: true, Kind: "choice", List: ListProficiencies},
		{Name: "level", Kind: "choice", List: ListProficiencyLevels},
		{Name: "when", Kind: "condition"},
		{Name: "text_pt", Kind: "text"},
	}},
	{Type: "resource", NamePT: "Recurso", HintPT: "Um número de usos que a ficha conta e que volta num descanso.", Fields: []MenuField{
		{Name: "resource", Required: true, Kind: "text"},
		{Name: "max", Required: true, Kind: "formula"},
		{Name: "recharge", Required: true, Kind: "choice", List: ListRecharges},
		{Name: "when", Kind: "condition"},
		{Name: "text_pt", Kind: "text"},
	}},
	{Type: "sense", NamePT: "Sentido", HintPT: "Dá visão no escuro ou outro sentido, com o alcance em pés.", Fields: []MenuField{
		{Name: "sense", Required: true, Kind: "choice", List: ListSenses},
		{Name: "range_ft", Required: true, Kind: "number", Min: 1},
		{Name: "when", Kind: "condition"},
		{Name: "text_pt", Kind: "text"},
	}},
	{Type: "roll_mode", NamePT: "Vantagem ou desvantagem", HintPT: "Vantagem ou desvantagem em certas rolagens; com uma situação (tags), o app só lembra e o mestre decide.", Fields: []MenuField{
		{Name: "roll", Required: true, Kind: "choice", List: ListRollModes},
		{Name: "targets", Required: true, Kind: "choices", List: ListRollTargets},
		{Name: "tags", Kind: "tags", List: ListTagPrefixes},
		{Name: "when", Kind: "condition"},
		{Name: "text_pt", Kind: "text"},
	}},
	{Type: "grant_action", NamePT: "Ação", HintPT: "Uma ação que o personagem passa a ter na vez dele: ação, ação bônus, reação, livre ou movimento.", Fields: []MenuField{
		{Name: "economy", Required: true, Kind: "choice", List: ListEconomies},
		{Name: "when", Kind: "condition"},
		{Name: "tags", Kind: "tags", List: ListTagPrefixes},
		{Name: "text_pt", Kind: "text"},
	}},
	{Type: "extra_attack", NamePT: "Ataque extra", HintPT: "Quantos ataques a ação Atacar faz; vale o maior que o personagem tiver.", Fields: []MenuField{
		{Name: "count", Required: true, Kind: "number", Min: 2, Max: 4},
		{Name: "when", Kind: "condition"},
		{Name: "text_pt", Kind: "text"},
	}},
	{Type: "choice", NamePT: "Escolha", HintPT: "Algo que o jogador escolhe: perícias, especialização, truques, magias, idiomas, ferramentas ou uma opção de uma lista do SRD (como um estilo de luta).", Fields: []MenuField{
		{Name: "choice", Required: true, Kind: "choice", List: ListChoiceKinds},
		{Name: "count", Required: true, Kind: "number", Min: 1, Max: MaxChoiceCount},
		{Name: "from", Kind: "choices"},
		{Name: "when", Kind: "condition"},
		{Name: "text_pt", Kind: "text"},
	}},
	{Type: "note", NamePT: "Nota e magia concedida", HintPT: "Um lembrete na ficha, com um número se quiser, ou magias que a característica concede (um truque, uma magia por dia).", Fields: []MenuField{
		{Name: "text_pt", Kind: "text"},
		{Name: "value", Kind: "formula"},
		{Name: "spells", Kind: "choices"},
		{Name: "tags", Kind: "tags", List: ListTagPrefixes},
		{Name: "when", Kind: "condition"},
	}},
}

// The Portuguese names of the small closed lists, in our own words.
var (
	modifierTargetNames = map[string]string{
		"ac.base": "Classe de Armadura base", "ac": "Classe de Armadura", "hp.max": "Pontos de vida máximos",
		"speed.walk": "Deslocamento", "initiative": "Iniciativa",
		"attack.weapon.melee": "Ataque com arma corpo a corpo", "attack.weapon.ranged": "Ataque com arma à distância",
		"damage.weapon.melee": "Dano de arma corpo a corpo", "damage.weapon.ranged": "Dano de arma à distância",
	}
	modeNames   = map[string][2]string{"add": {"Somar", "Soma o valor ao número."}, "max": {"Pelo menos", "O número não fica abaixo do valor."}, "set": {"Definir", "O número passa a ser o valor."}}
	levelNames  = map[string]string{"half": "Metade", "full": "Completa", "expertise": "Especialização"}
	rollNames   = map[string]string{"advantage": "Vantagem", "disadvantage": "Desvantagem"}
	senseNames  = map[string]string{"darkvision": "Visão no escuro", "blindsight": "Visão às cegas", "tremorsense": "Sentido sísmico", "truesight": "Visão verdadeira"}
	rechargeNms = map[string]string{"short_rest": "Descanso curto", "long_rest": "Descanso longo", "dawn": "Ao amanhecer", "none": "Não volta sozinho"}
	economyNms  = map[string]string{"action": "Ação", "bonus_action": "Ação bônus", "reaction": "Reação", "free": "Livre", "movement": "Movimento"}
	choiceNames = map[string]string{
		"skill": "Perícias", "expertise": "Especialização", "cantrip": "Truques", "spell": "Magias",
		"language": "Idiomas", "tool": "Ferramentas", "feature": "Uma opção de uma lista do SRD",
	}
	rollTargetNames = map[string]string{
		"attack": "Ataques", "death_save": "Testes contra a morte", "initiative": "Iniciativa",
		"check.all": "Todos os testes de habilidade", "save.all": "Todos os testes de resistência",
	}
	tagPrefixes = []MenuValue{
		{"against:", "Contra…", "Só vale contra o que a etiqueta diz (against:magic); o app lembra e o mestre decide."},
		{"about:", "Sobre…", "Vale sobre um assunto (about:magic-items); o app lembra e o mestre decide."},
	}
)

// EffectMenu gives the closed menu for this content: the table's classes are in
// ClassIndexes, and everything else is the SRD's.
func (c *Content) EffectMenu() EffectMenu {
	x := c.c
	m := EffectMenu{
		Types:               slices.Clone(menuTypes),
		MaxFeaturesPerClass: MaxTableFeatures, MaxEffectsPerFeature: MaxFeatureEffects, MaxTagsPerEffect: maxEffectList,
		ExtraAttackMin: 2, ExtraAttackMax: 4,
	}
	abilityName := func(a Ability) string { return x.namePT(string(a)) }

	var targets, rolls, profs []MenuValue
	for _, k := range modifierTargets {
		targets = append(targets, MenuValue{Key: k, NamePT: modifierTargetNames[k]})
	}
	for _, k := range x.skillOrder {
		targets = append(targets, MenuValue{Key: "skill." + strings.TrimPrefix(k, "skill:"), NamePT: "Perícia: " + x.namePT(k)})
	}
	for _, a := range AllAbilities() {
		targets = append(targets, MenuValue{Key: "save." + string(a), NamePT: "Teste de resistência de " + abilityName(a)})
	}
	for _, k := range []string{"attack", "death_save", "initiative", "check.all", "save.all"} {
		rolls = append(rolls, MenuValue{Key: k, NamePT: rollTargetNames[k]})
	}
	for _, a := range AllAbilities() {
		rolls = append(rolls, MenuValue{Key: "check." + string(a), NamePT: "Testes de " + abilityName(a)})
	}
	for _, k := range x.skillOrder {
		rolls = append(rolls, MenuValue{Key: "skill." + strings.TrimPrefix(k, "skill:"), NamePT: "Perícia: " + x.namePT(k)})
	}
	for _, a := range AllAbilities() {
		rolls = append(rolls, MenuValue{Key: "save." + string(a), NamePT: "Teste de resistência de " + abilityName(a)})
	}
	profs = append(profs, MenuValue{Key: "skill:*", NamePT: "Todas as perícias"}, MenuValue{Key: "initiative", NamePT: "Iniciativa"})
	var skills, languages, tools []MenuValue
	for _, k := range x.skillOrder {
		v := MenuValue{Key: k, NamePT: x.namePT(k)}
		skills = append(skills, v)
		profs = append(profs, v)
	}
	for _, a := range AllAbilities() {
		profs = append(profs, MenuValue{Key: "save." + string(a), NamePT: "Teste de resistência de " + abilityName(a)})
	}
	for _, k := range sortedKeys(x.proficiencies) {
		switch x.proficiencies[k].Kind {
		case "armor", "weapon":
			profs = append(profs, MenuValue{Key: k, NamePT: x.proficiencyNamePT(k)})
		case "tool", "other":
			v := MenuValue{Key: k, NamePT: x.proficiencyNamePT(k)}
			profs = append(profs, v)
			tools = append(tools, v)
		}
	}
	for _, k := range sortedKeys(x.languages) {
		languages = append(languages, MenuValue{Key: k, NamePT: x.namePT(k)})
	}
	sortByName := func(vs []MenuValue) {
		slices.SortStableFunc(vs, func(a, b MenuValue) int { return strings.Compare(strings.ToLower(a.NamePT), strings.ToLower(b.NamePT)) })
	}
	sortByName(skills)
	sortByName(languages)
	sortByName(tools)

	named := func(keys []string, name func(string) string) []MenuValue {
		out := make([]MenuValue, 0, len(keys))
		for _, k := range keys {
			out = append(out, MenuValue{Key: k, NamePT: name(k)})
		}
		return out
	}
	var modes []MenuValue
	for _, k := range modifierModes {
		modes = append(modes, MenuValue{Key: k, NamePT: modeNames[k][0], HintPT: modeNames[k][1]})
	}
	m.Lists = []MenuList{
		{Name: ListModifierTargets, Values: targets},
		{Name: ListModifierModes, Values: modes},
		{Name: ListProficiencies, Values: profs},
		{Name: ListProficiencyLevels, Values: named(proficiencyLevels, func(k string) string { return levelNames[k] })},
		{Name: ListRollModes, Values: named(rollModes, func(k string) string { return rollNames[k] })},
		{Name: ListRollTargets, Values: rolls},
		{Name: ListSenses, Values: named(senses, func(k string) string { return senseNames[k] })},
		{Name: ListRecharges, Values: named(recharges, func(k string) string { return rechargeNms[k] })},
		{Name: ListEconomies, Values: named(economies, func(k string) string { return economyNms[k] })},
		{Name: ListChoiceKinds, Values: named(overlayChoiceKinds, func(k string) string { return choiceNames[k] })},
		{Name: ListSkills, Values: skills},
		{Name: ListLanguages, Values: languages},
		{Name: ListTools, Values: tools},
		{Name: ListTagPrefixes, Values: tagPrefixes},
	}

	// The option sets: SRD features that offer options. A set with no option
	// names (none today) is left out.
	for _, k := range sortedKeys(x.features) {
		f := x.features[k]
		if len(f.Options) == 0 {
			continue
		}
		set := MenuOptionSet{Key: k, NamePT: x.namePT(k), Choose: f.OptionsChoose}
		for _, o := range f.Options {
			set.Options = append(set.Options, MenuValue{Key: o, NamePT: x.namePT(o)})
		}
		m.OptionSets = append(m.OptionSets, set)
	}
	// Two sets with one name (the Fighter's and the Paladin's fighting styles) say
	// whose they are, and, when the class repeats it too (the Sorcerer's three
	// Metamagic), at which level.
	names := map[string]int{}
	for _, set := range m.OptionSets {
		names[set.NamePT]++
	}
	withClass := map[string]int{}
	for _, set := range m.OptionSets {
		if names[set.NamePT] > 1 {
			withClass[set.NamePT+" ("+x.namePT(x.features[set.Key].Class)+")"]++
		}
	}
	for i, set := range m.OptionSets {
		if names[set.NamePT] < 2 {
			continue
		}
		f := x.features[set.Key]
		label := set.NamePT + " (" + x.namePT(f.Class)
		if withClass[label+")"] > 1 {
			label += fmt.Sprintf(", nível %d", f.Level)
		}
		m.OptionSets[i].NamePT = label + ")"
	}
	slices.SortStableFunc(m.OptionSets, func(a, b MenuOptionSet) int { return strings.Compare(a.NamePT, b.NamePT) })

	m.Helpers = []FormulaHelper{
		{Call: "level()", Returns: "number", HintPT: "O nível total do personagem."},
		{Call: `classLevel("<classe>")`, Returns: "number", HintPT: "O nível numa classe, pelo índice dela (sem o \"class:\"); 0 se o personagem não a tem."},
		{Call: `mod("<habilidade>")`, Returns: "number", HintPT: `O modificador de uma habilidade: "str", "dex", "con", "int", "wis" ou "cha".`},
		{Call: `score("<habilidade>")`, Returns: "number", HintPT: "O valor de uma habilidade, com os bônus."},
		{Call: "prof()", Returns: "number", HintPT: "O bônus de proficiência."},
		{Call: "armor()", Returns: "text", HintPT: `A armadura vestida: "none", "light", "medium" ou "heavy".`},
		{Call: "shield()", Returns: "yes/no", HintPT: "Se o personagem carrega um escudo."},
		{Call: "floor(x)", Returns: "number", HintPT: "Arredonda para baixo."},
		{Call: "ceil(x)", Returns: "number", HintPT: "Arredonda para cima."},
		{Call: "min(a, b)", Returns: "number", HintPT: "O menor dos dois."},
		{Call: "max(a, b)", Returns: "number", HintPT: "O maior dos dois."},
	}
	for k := range x.classes {
		m.ClassIndexes = append(m.ClassIndexes, strings.TrimPrefix(k, "class:"))
	}
	slices.Sort(m.ClassIndexes)
	return m
}
