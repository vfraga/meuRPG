package rules

import (
	"slices"
	"strings"
)

// The spell list of the players' "Magias" page (MR-045): the SRD's spells and the
// table's together, found by name and filtered by class, circle and school. It is
// pure, so the filters have tests of their own; the characters module only adds
// who may see what.

// SpellFilter is what ListSpells filters by. Every field is optional and they add
// up: a spell passes all of them.
type SpellFilter struct {
	// Query is part of the Portuguese or English name, ignoring case and accents.
	Query string
	// Class is a class key (its list: the spells that name it, or the list it
	// reuses) or the key of a third caster's subclass (the list it casts from).
	Class string
	// Levels are the circles to list (0 is a cantrip); empty for all.
	Levels []int
	// Schools are school keys; empty for all.
	Schools []string
	// Ritual keeps the spells that can be cast as a ritual.
	Ritual bool
	// Learnable, when not nil, keeps the spells one of these lists has, up to the
	// highest circle of that list: what a character can learn.
	Learnable []SpellAccess
	// Hidden says whether an entry is left out (the table's archived spells for a
	// player; the switches of "Opções para os jogadores" later). Nil leaves none.
	Hidden func(key string) bool
}

// SpellAccess is a spell list a character reads and the highest circle it casts
// from it (0 when only cantrips).
type SpellAccess struct {
	// List is the class key whose list it is.
	List     string
	MaxLevel int
}

// SpellListOf is the class whose spell list the key names: the class itself, or
// the class a third caster's subclass casts from. It is false for a key that has no
// list (a class that never casts and reuses none, a subclass that does not cast, a
// key that is not a class).
func (c *Content) SpellListOf(key string) (string, bool) {
	if s, ok := c.c.subCasting[key]; ok {
		return s.list, s.list != ""
	}
	if _, ok := c.c.classes[key]; !ok {
		return "", false
	}
	// A class has a list when it casts (its own) or reuses one (a table class).
	if _, casts := c.c.casting[key]; casts || c.c.listFrom[key] != "" {
		return key, true
	}
	return "", false
}

// ListSpells lists the catalog's spells that pass the filter, sorted by
// Portuguese name (as Catalog().Spells is). The entries are shared: do not modify
// them.
func (c *Content) ListSpells(f SpellFilter) []SpellEntry {
	query := foldPT(strings.TrimSpace(f.Query))
	list := ""
	if f.Class != "" {
		l, ok := c.SpellListOf(f.Class)
		if !ok {
			return nil
		}
		list = l
	}
	var out []SpellEntry
	for _, e := range c.c.catalog.Spells {
		if spellPasses(f, e, query, list) {
			out = append(out, e)
		}
	}
	return out
}

// spellPasses says whether a spell passes every field of the filter; query
// is the folded query, and list the class list ("" for any).
func spellPasses(f SpellFilter, e SpellEntry, query, list string) bool {
	switch {
	case f.Hidden != nil && f.Hidden(e.Key):
		return false
	case query != "" && !strings.Contains(foldPT(e.NamePT), query) && !strings.Contains(foldPT(e.Name), query):
		return false
	case list != "" && !slices.Contains(e.Classes, list):
		return false
	case !spellIsOfKind(f, e):
		return false
	}
	return f.Learnable == nil || slices.ContainsFunc(f.Learnable, func(a SpellAccess) bool {
		return e.Level <= a.MaxLevel && slices.Contains(e.Classes, a.List)
	})
}

// spellIsOfKind says whether a spell is of one of the circles and schools of the filter, and a ritual when it asks for one.
func spellIsOfKind(f SpellFilter, e SpellEntry) bool {
	return (len(f.Levels) == 0 || slices.Contains(f.Levels, e.Level)) &&
		(len(f.Schools) == 0 || slices.Contains(f.Schools, e.School)) && (!f.Ritual || e.Ritual)
}
