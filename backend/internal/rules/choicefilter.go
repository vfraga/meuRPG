package rules

import (
	"slices"
	"strings"
)

// What a `choice` effect may offer besides a list of keys: every proficiency of some kinds (a choice of tools with a
// filter), or the options of another feature (a choice of features with from_feature).

// The proficiency kinds a filter names, and the order the menu lists them in.
const (
	filterKindWeapon = "weapon"
	filterKindTool   = "tool"
	filterKindOther  = "other"
)

// filterKinds are the proficiency kinds a filter may name.
var filterKinds = []string{filterKindWeapon, filterKindTool, filterKindOther}

// ChoiceFilter says which entries a choice offers by what they are: the kinds of proficiency of a choice of tools, and
// the class lists, circles and schools of a choice of cantrips or of spells.
type ChoiceFilter struct {
	ProficiencyKinds []string `json:"proficiency_kinds,omitempty"`
	// ClassLists are the class keys whose spell lists the spells come from (a spell on any of them passes). SpellLevels
	// are the circles offered, Schools the school keys ("school:evocation") and Ritual keeps the spells that can be cast as
	// a ritual. A field left empty (false) keeps every spell.
	ClassLists  []string `json:"class_lists,omitempty"`
	SpellLevels []int    `json:"spell_levels,omitempty"`
	Schools     []string `json:"schools,omitempty"`
	Ritual      bool     `json:"ritual,omitempty"`
}

// hasSpellFields says the filter names something of spells.
func (f *ChoiceFilter) hasSpellFields() bool {
	return len(f.ClassLists) > 0 || len(f.SpellLevels) > 0 || len(f.Schools) > 0 || f.Ritual
}

// proficiencyKindNames are the Portuguese names of the kinds a filter names.
var proficiencyKindNames = map[string]string{
	filterKindWeapon: "Armas",
	filterKindTool:   "Ferramentas",
	filterKindOther:  "Veículos e kits",
}

// menuProficienciesOfKind are the proficiencies of one kind, by Portuguese name: what a filter of that kind offers.
func (c *content) menuProficienciesOfKind(kind string) []MenuValue {
	var out []MenuValue
	for _, k := range c.proficiencyKeysOfKinds(kind) {
		out = append(out, MenuValue{Key: k, NamePT: c.proficiencyNamePT(k)})
	}
	sortMenuByName(out)
	return out
}

// proficiencyKeysOfKinds are the keys of the content's proficiencies of the kinds, sorted.
func (c *content) proficiencyKeysOfKinds(kinds ...string) []string {
	var out []string
	for k, p := range c.proficiencies {
		if slices.Contains(kinds, p.Kind) {
			out = append(out, k)
		}
	}
	slices.Sort(out)
	return out
}

// checkChoiceFilter checks the filter of a choice: only a choice of tools, cantrips or spells has one, it replaces the
// list, the set, the feature and the daily pick, and it holds the fields of its kind of choice.
func checkChoiceFilter(site effectSite, e *Effect) error {
	f := e.Filter
	if f == nil {
		return nil
	}
	switch {
	case e.Choice != "tool" && e.Choice != "cantrip" && e.Choice != "spell":
		return site.fail(".filter", ReasonValue, "only a choice of tools, cantrips or spells has a filter")
	case len(e.From) > 0 || e.Set != "" || e.FromFeature != "":
		return site.fail(".filter", ReasonValue, "a choice has a filter or a list of options, not both")
	case e.DailyChoice:
		return site.fail(".filter", ReasonValue, "a daily choice has a list, not a filter")
	case e.Choice == "tool":
		return checkToolFilter(site, f)
	}
	return checkSpellFilter(site, e.Choice, f)
}

// checkToolFilter checks the filter of a choice of tools: at least one kind of proficiency, each once, and no field of spells.
func checkToolFilter(site effectSite, f *ChoiceFilter) error {
	switch {
	case f.hasSpellFields():
		return site.fail(".filter", ReasonValue, "a choice of tools has a filter of proficiency kinds, not of spells")
	case len(f.ProficiencyKinds) == 0:
		return site.fail(".filter.proficiency_kinds", ReasonValue, "a filter names at least one kind of proficiency")
	}
	for i, k := range f.ProficiencyKinds {
		switch {
		case !slices.Contains(filterKinds, k):
			return site.fail(".filter.proficiency_kinds", ReasonValue, "kind %q is not on the filter's menu (weapon, tool or other)", k)
		case slices.Contains(f.ProficiencyKinds[:i], k):
			return site.fail(".filter.proficiency_kinds", ReasonValue, "kind %q is listed twice", k)
		}
	}
	return nil
}

// Most class lists and schools a filter names, and the circle of the highest spell.
const (
	maxFilterNames    = 8
	highestSpellLevel = 9
)

// checkSpellFilter checks the filter of a choice of cantrips or of spells: no kind of proficiency, at least one class
// list (each once), circles of the choice's kind (each once) and each school once. That the keys exist is the builder's.
func checkSpellFilter(site effectSite, kind string, f *ChoiceFilter) error {
	switch {
	case len(f.ProficiencyKinds) > 0:
		return site.fail(".filter.proficiency_kinds", ReasonValue, "only a choice of tools has a filter of proficiency kinds")
	case len(f.ClassLists) == 0:
		return site.fail(".filter.class_lists", ReasonValue, "a filter of spells names at least one class list")
	}
	if err := checkFilterNames(site, ".filter.class_lists", f.ClassLists); err != nil {
		return err
	}
	if err := checkFilterNames(site, ".filter.schools", f.Schools); err != nil {
		return err
	}
	return checkFilterLevels(site, kind, f.SpellLevels)
}

// checkFilterNames checks the keys of a field of a filter: at most maxFilterNames, each once.
func checkFilterNames(site effectSite, field string, keys []string) error {
	if len(keys) > maxFilterNames {
		return site.fail(field, ReasonLimit, "a filter names at most %d", maxFilterNames)
	}
	for i, k := range keys {
		if slices.Contains(keys[:i], k) {
			return site.fail(field, ReasonValue, "%q is listed twice", k)
		}
	}
	return nil
}

// checkFilterLevels checks the circles of a filter: a choice of cantrips takes the cantrips (0), a choice of spells the
// circles 1 to 9, each once.
func checkFilterLevels(site effectSite, kind string, levels []int) error {
	low, high := 1, highestSpellLevel
	if kind == "cantrip" {
		low, high = 0, 0
	}
	for i, n := range levels {
		switch {
		case n < low || n > high:
			return site.fail(".filter.spell_levels", ReasonValue, "circle %d is not one a choice of %ss takes (%d to %d)", n, kind, low, high)
		case slices.Contains(levels[:i], n):
			return site.fail(".filter.spell_levels", ReasonValue, "circle %d is listed twice", n)
		}
	}
	return nil
}

// checkFilterKeys checks the keys of a filter of spells against the content: each class list is a class of the base or of
// the table, each school one of the SRD's.
func (b *overlayBuilder) checkFilterKeys(site effectSite, e *Effect) error {
	f := e.Filter
	if f == nil || e.Choice == "tool" {
		return nil
	}
	for _, k := range f.ClassLists {
		if !b.isClass(k) {
			return site.fail(".filter.class_lists", ReasonReference, "class list %q is not a class of the content", k)
		}
	}
	for _, k := range f.Schools {
		if sch, ok := b.base.named[k]; !ok || sch == nil || !strings.HasPrefix(k, "school:") {
			return site.fail(".filter.schools", ReasonReference, "school %q is not in the SRD", k)
		}
	}
	return nil
}

// isToolPickKey says a proficiency key is one a tool choice's own list may name: a tool, a vehicle or a kit, or a weapon.
func (c *content) isToolPickKey(key string) bool {
	p := c.proficiencies[key]
	return p != nil && slices.Contains(filterKinds, p.Kind)
}

// toolSpec is what the choices of tools that owner has ask in all: how many picks, the proficiencies they list (a
// filter lists the ones of its kinds), and whether one of them lists none, which offers every tool.
func (c *content) toolSpec(owner string) (picks int, from []string, open, ok bool) {
	for _, e := range c.effectChoices(owner, "tool") {
		ok = true
		picks += max(e.Count, 1)
		switch {
		case e.Filter != nil:
			from = append(from, c.proficiencyKeysOfKinds(e.Filter.ProficiencyKinds...)...)
		case len(e.From) == 0:
			open = true
		default:
			from = append(from, e.From...)
		}
	}
	return picks, from, open, ok
}

// toolsOffered are the proficiencies a tool choice offers: the listed ones that a tool choice may name and, when it
// is open, every tool. Each once.
func (c *content) toolsOffered(from []string, open bool) []string {
	var out []string
	for _, k := range from {
		if c.isToolPickKey(k) && !slices.Contains(out, k) {
			out = append(out, k)
		}
	}
	if open {
		for _, k := range c.proficiencyKeysOfKinds(filterKindTool) {
			if !slices.Contains(out, k) {
				out = append(out, k)
			}
		}
	}
	return out
}
