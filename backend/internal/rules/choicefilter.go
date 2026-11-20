package rules

import "slices"

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

// ChoiceFilter says which entries a choice offers by what they are. It holds the kinds of proficiency of a choice of
// tools today; the fields for the spells of a choice of spells join it later.
type ChoiceFilter struct {
	ProficiencyKinds []string `json:"proficiency_kinds,omitempty"`
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

// checkChoiceFilter checks the filter of a choice: only a choice of tools has one, it replaces the list, the set, the
// feature and the daily pick, and it names each kind of proficiency it offers once.
func checkChoiceFilter(site effectSite, e *Effect) error {
	f := e.Filter
	if f == nil {
		return nil
	}
	switch {
	case e.Choice != "tool":
		return site.fail(".filter", ReasonValue, "only a choice of tools has a filter")
	case len(e.From) > 0 || e.Set != "" || e.FromFeature != "":
		return site.fail(".filter", ReasonValue, "a choice has a filter or a list of options, not both")
	case e.DailyChoice:
		return site.fail(".filter", ReasonValue, "a daily choice has a list, not a filter")
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
