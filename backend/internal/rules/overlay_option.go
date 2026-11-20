package rules

import (
	"fmt"
	"slices"
	"strings"

	"github.com/PuraFome/meuRPG/backend/internal/rules/srd51"
)

// The table's options for the SRD's option sets (MR-025): an Eldritch Invocation, a
// Metamagic option, a fighting style, a Pact Boon... written by the master as an entry
// "option:<set>:<slug>@mesa". The option joins the Options of every SRD feature that
// offers the set, so the choice engine deals with it like an SRD option: the picks are
// shared in the pool of the set, the option is a feature once picked (its effects
// apply while the character owns an offering feature), and what it asks for is listed
// on it as prerequisites.

// optionSetDef is an option set a table option may join: its short name (the one in the
// key), the Portuguese name and the SRD features that offer it.
type optionSetDef struct {
	Name, NamePT, HintPT string
	Offerers             []string
}

// optionSetDefs are the sets, in the menu's order. A set whose options are data the engine
// reads by name (the Land druid's terrain, the draconic ancestry) is not here.
var optionSetDefs = []optionSetDef{
	{"invocation", "Invocação mística", "As invocações do bruxo.", []string{invocationsFeature}},
	{"pact_boon", "Dádiva do pacto", "A dádiva que o bruxo escolhe no 3º nível.", []string{pactBoonKey}},
	{"metamagic", "Metamagia", "As opções de Metamagia do feiticeiro.", []string{"feature:metamagic-1", "feature:metamagic-2", "feature:metamagic-3"}},
	{"fighting_style", "Estilo de luta", "Os estilos de luta do guerreiro, do paladino e do patrulheiro.", []string{
		"feature:fighter-fighting-style", "feature:additional-fighting-style", "feature:paladin-fighting-style", "feature:ranger-fighting-style",
	}},
	{"hunters_prey", "Presa do Caçador", "As opções de Presa do Caçador do patrulheiro.", []string{"feature:hunters-prey"}},
	{"defensive_tactics", "Táticas defensivas", "As opções de Táticas Defensivas do patrulheiro.", []string{"feature:defensive-tactics"}},
	{"multiattack", "Ataque múltiplo", "As opções de Ataque Múltiplo do patrulheiro.", []string{"feature:multiattack"}},
	{"superior_hunters_defense", "Defesa superior do caçador", "As opções de Defesa Superior do Caçador do patrulheiro.", []string{"feature:superior-hunters-defense"}},
	{"table_feature", "Opção de característica da mesa", "As opções de uma característica de classe ou subclasse da mesa (as manobras de uma subclasse).", nil},
}

// optionSetOf is the set with this name.
func optionSetOf(name string) (optionSetDef, bool) {
	i := slices.IndexFunc(optionSetDefs, func(d optionSetDef) bool { return d.Name == name })
	if i < 0 {
		return optionSetDef{}, false
	}
	return optionSetDefs[i], true
}

// OptionKeyPrefix starts the key of a table option.
const OptionKeyPrefix = "option:"

// featureKeyPrefix starts the key of a class, subclass or racial feature.
const (
	featureKeyPrefix = "feature:"
	traitKeyPrefix   = "trait:"
)

// pactBoonKey is the warlock's Pact Boon, the parent of its options.
const pactBoonKey = "feature:pact-boon"

// MaxOptionRequires is the most options one option may require.
const MaxOptionRequires = 10

// TableOption is an option of one of the SRD's option sets. Key is
// "option:<set>:<slug>@mesa" and Set is the same set the key names.
type TableOption struct {
	TableEntry
	DescPT []string
	Set    string
	// OfferedBy is the table feature whose options this one joins; only for the set "table_feature".
	OfferedBy    string
	Prerequisite TableOptionPrerequisite
	Effects      []Effect
	// Infusion is set when the option is an infusion: only an option of the set "table_feature" whose feature has an
	// InfusionRule.
	Infusion *TableInfusion
}

// TableOptionPrerequisite is what a table option asks of the character; every one set must
// hold. The zero value asks nothing.
type TableOptionPrerequisite struct {
	// ClassKey and ClassLevel: a level in one class (1 to MaxLevel); both or neither.
	ClassKey   string
	ClassLevel int
	// RequiresOptions are options (the SRD's or the table's) the character has picked.
	RequiresOptions []string
	// RequiresPact is the Pact Boon the character has.
	RequiresPact string
	// RequiresSpell is a spell the character knows.
	RequiresSpell string
}

// IsZero says the option asks nothing.
func (p TableOptionPrerequisite) IsZero() bool {
	return p.ClassKey == "" && p.ClassLevel == 0 && len(p.RequiresOptions) == 0 && p.RequiresPact == "" && p.RequiresSpell == ""
}

// optionKeySet is the set a table option key names: "option:fighting_style:x@mesa".
func optionKeySet(key string) (string, bool) {
	rest, ok := strings.CutPrefix(key, OptionKeyPrefix)
	if !ok {
		return "", false
	}
	set, _, ok := strings.Cut(rest, ":")
	return set, ok
}

// checkOptionKey checks "option:<set>:<slug>@mesa": a known set and a slug as any key's.
func checkOptionKey(key string) error {
	set, ok := optionKeySet(key)
	if !ok {
		return ovErr(key, "the key must start with %q and name a set", OptionKeyPrefix)
	}
	if _, known := optionSetOf(set); !known {
		return ovErr(key, "the key names an unknown option set %q", set).at("key", ReasonKey)
	}
	return checkTableKey(OptionKeyPrefix+set+":", key)
}

// addOption registers a table option: a feature of the clone that the SRD features of its set
// offer, with its effects queued with the others.
func (b *overlayBuilder) addOption(o *TableOption, path string) error {
	n := b.n
	key := o.Key
	var c entryErrors
	def, known := optionSetOf(o.Set)
	switch keySet, _ := optionKeySet(key); {
	case !known:
		c.at(key, path, ".set", ReasonValue, "unknown option set %q", o.Set)
	case keySet != o.Set:
		c.at(key, path, ".set", ReasonValue, "the set is the one in the key (%q)", keySet)
	}
	b.checkOfferedBy(&c, o, path)
	checkTextAt(&c, key, path, ".desc_pt", o.DescPT)
	b.checkOptionPrerequisite(&c, o, path)
	b.addInfusion(&c, o.Key, o.OfferedBy, path, o.Infusion)
	for i := range o.Effects {
		if o.Effects[i].Type == "choice" {
			c.at(key, path, fmt.Sprintf(".effects[%d].type", i), ReasonEffect, "an option does not ask for another choice")
		}
	}
	if len(o.Effects) > MaxFeatureEffects {
		c.at(key, path, ".effects", ReasonLimit, "%d effects; the limit is %d per feature", len(o.Effects), MaxFeatureEffects)
	}
	if len(o.Effects) > 0 {
		// An option of the set "table_feature" is read as an inline option of its feature is:
		// it spends the resources and reads the states of the feature's entry and class.
		home := featureKind{owner: key}
		if o.Set == tableFeatureSet {
			home = featureKind{owner: b.featureEntry[o.OfferedBy], class: b.featureClass[o.OfferedBy]}
		}
		b.pending = append(b.pending, pendingEffects{owner: key, entry: home.owner, effects: o.Effects, path: path, strict: b.strict[key]})
		b.know(key, home)
		b.declareResources(home, o.Effects)
		b.declareStates(key, home.owner, o.Effects)
	}
	if err := c.err(); err != nil {
		return err
	}
	b.register(o.TableEntry)
	b.offerOption(o, def)
	n.optionSet[key] = o.Set
	if !o.Prerequisite.IsZero() {
		n.optionPrereq[key] = o.Prerequisite
	}
	return nil
}

// offerOption writes the option as a feature and adds it to the options of every feature
// that offers its set (the ones the content has). The offering features are copied before
// they change: the SRD's are shared.
func (b *overlayBuilder) offerOption(o *TableOption, def optionSetDef) {
	n := b.n
	var offerers []string
	candidates := def.Offerers
	if o.Set == tableFeatureSet {
		candidates = []string{o.OfferedBy}
	}
	for _, k := range candidates {
		if n.features[k] != nil {
			offerers = append(offerers, k)
		}
	}
	if len(offerers) == 0 {
		return
	}
	first := n.features[offerers[0]]
	n.features[o.Key] = &srd51.Feature{Key: o.Key, Name: o.NamePT, Class: first.Class, Parent: offerers[0], Desc: slices.Clone(o.DescPT)}
	if b.optionOffers == nil {
		b.optionOffers = map[string]bool{}
	}
	for _, k := range offerers {
		if !b.optionOffers[k] {
			cp := *n.features[k]
			cp.Options = slices.Clone(cp.Options)
			n.features[k] = &cp
			b.optionOffers[k] = true
		}
		n.features[k].Options = append(n.features[k].Options, o.Key)
		n.offeredBy[o.Key] = append(n.offeredBy[o.Key], k)
	}
}

// tableFeatureSet is the option set of the options of a table feature.
const tableFeatureSet = "table_feature"

// checkOfferedBy checks `offered_by`: the feature of the set "table_feature", a table class or subclass
// feature that has an option list of its own, and nothing for any other set.
func (b *overlayBuilder) checkOfferedBy(c *entryErrors, o *TableOption, path string) {
	if o.Set != tableFeatureSet {
		if o.OfferedBy != "" {
			c.at(o.Key, path, ".offered_by", ReasonValue, "only the set %q has an offering feature", tableFeatureSet)
		}
		return
	}
	f := b.n.features[o.OfferedBy]
	switch {
	case o.OfferedBy == "":
		c.at(o.Key, path, ".offered_by", ReasonReference, "the set %q needs the feature that offers the option", tableFeatureSet)
	case !strings.HasPrefix(o.OfferedBy, featureKeyPrefix) || !strings.HasSuffix(o.OfferedBy, tableSuffix) || f == nil:
		c.at(o.Key, path, ".offered_by", ReasonReference, "%q is not a class or subclass feature of the table", o.OfferedBy)
	case len(f.Options) == 0:
		c.at(o.Key, path, ".offered_by", ReasonReference, "%q has no choice of options to join", o.OfferedBy)
	}
}

// checkOptionPrerequisite checks what the option asks for, each at its own field.
func (b *overlayBuilder) checkOptionPrerequisite(c *entryErrors, o *TableOption, path string) {
	b.checkPrerequisiteFields(c, o.Key, path, o.Prerequisite)
}

// checkPrerequisiteFields checks a prerequisite of the entry key at path (a table option's, an inline option's or a
// feat's), each field at its own path.
func (b *overlayBuilder) checkPrerequisiteFields(c *entryErrors, key, path string, p TableOptionPrerequisite) {
	if (p.ClassKey == "") != (p.ClassLevel == 0) {
		c.at(key, path, ".prerequisite.class_level", ReasonValue, "a class and a level go together")
	}
	if p.ClassKey != "" && !b.isClass(p.ClassKey) {
		c.at(key, path, ".prerequisite.class_level.class_key", ReasonReference, "the class %q does not exist", p.ClassKey)
	}
	if p.ClassLevel < 0 || p.ClassLevel > MaxLevel {
		c.at(key, path, ".prerequisite.class_level.level", ReasonValue, "the level is 1 to %d", MaxLevel)
	}
	if len(p.RequiresOptions) > MaxOptionRequires {
		c.at(key, path, ".prerequisite.requires_option_keys", ReasonLimit, "%d options; the limit is %d", len(p.RequiresOptions), MaxOptionRequires)
	}
	for i, k := range p.RequiresOptions {
		field := fmt.Sprintf(".prerequisite.requires_option_keys[%d]", i)
		if k == key {
			c.at(key, path, field, ReasonValue, "an option cannot require itself")
			continue
		}
		b.optionRequires = append(b.optionRequires, optionRequire{owner: key, path: path, field: field, option: k})
	}
	if p.RequiresPact != "" && !b.isPact(p.RequiresPact) {
		c.at(key, path, ".prerequisite.requires_pact", ReasonReference, "%q is not a Pact Boon", p.RequiresPact)
	}
	if p.RequiresSpell != "" && !b.isSpell(p.RequiresSpell) {
		c.at(key, path, ".prerequisite.requires_spell_key", ReasonReference, "the spell %q does not exist", p.RequiresSpell)
	}
}

// isOption says the key is an option of an SRD option set or a table option of the overlay.
func (b *overlayBuilder) isOption(key string) bool {
	if _, ok := b.optionEntry[key]; ok {
		return true // an option of a table feature or feat
	}
	if strings.HasPrefix(key, OptionKeyPrefix) {
		return b.entries[key]
	}
	if f := b.base.features[key]; f != nil && f.Parent != "" {
		return slices.ContainsFunc(optionSetDefs, func(d optionSetDef) bool { return slices.Contains(d.Offerers, f.Parent) })
	}
	return false
}

// isPact says the key is a Pact Boon: the SRD's, or a table option of the set "pact_boon".
func (b *overlayBuilder) isPact(key string) bool {
	if set, ok := optionKeySet(key); ok {
		return set == "pact_boon" && b.entries[key]
	}
	f := b.base.features[key]
	return f != nil && f.Parent == pactBoonKey
}

// OptionEntry is a table option as the content lists it.
type OptionEntry struct {
	Key, NamePT, Set string
	Archived, Off    bool
}

// Options lists the table's options, sorted by set and Portuguese name.
func (c *Content) Options() []OptionEntry {
	x := c.c
	out := make([]OptionEntry, 0, len(x.optionSet))
	for _, k := range sortedKeys(x.optionSet) {
		out = append(out, OptionEntry{Key: k, NamePT: x.namePT(k), Set: x.optionSet[k], Archived: x.archived[k], Off: x.off[k]})
	}
	slices.SortStableFunc(out, func(a, b OptionEntry) int {
		if a.Set != b.Set {
			return strings.Compare(a.Set, b.Set)
		}
		return comparePT(a.NamePT, b.NamePT)
	})
	return out
}

// optionRequire is an option a prerequisite asks for, at the field of its owner.
type optionRequire struct{ owner, path, field, option string }

// checkOptionRequires judges the options the prerequisites ask for once every entry is built, so an option may ask for
// one of a feature that comes later in the overlay: each must exist, and no two may wait for each other (they would stay
// blocked forever).
func (b *overlayBuilder) checkOptionRequires() error {
	var c entryErrors
	waits := map[string][]string{}
	for _, r := range b.optionRequires {
		if !b.isOption(r.option) {
			c.at(r.owner, r.path, r.field, ReasonReference, "the option %q does not exist", r.option)
			continue
		}
		waits[r.owner] = append(waits[r.owner], r.option)
	}
	for _, r := range b.optionRequires {
		if slices.Contains(waits[r.owner], r.option) && waitsFor(waits, r.option, r.owner, map[string]bool{}) {
			c.at(r.owner, r.path, r.field, ReasonValue, "%q and %q wait for each other: neither could ever be picked", r.owner, r.option)
		}
	}
	return c.err()
}

// waitsFor says whether from, through the options it asks for, asks for target.
func waitsFor(waits map[string][]string, from, target string, seen map[string]bool) bool {
	if seen[from] {
		return false
	}
	seen[from] = true
	for _, k := range waits[from] {
		if k == target || waitsFor(waits, k, target, seen) {
			return true
		}
	}
	return false
}
