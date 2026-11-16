package rules

import (
	"fmt"
	"slices"
	"strings"
)

// The pool of a choice of cantrips or of spells that offers the spells of some class lists (a filter, or the classes a
// choice of spells lists in from): worked out for the reader, so a player's pool never holds what the table keeps from
// them.

// SpellsOfChoiceFilter lists the spells of the filter's class lists, circles, schools and rituals, sorted by Portuguese
// name, each once. A spell passes when any of the lists has it; an empty SpellLevels, Schools or false Ritual keeps
// every spell, and a filter with no class list has none. A spell or a class list the table retired is left out for every
// reader, and so is any key hidden(key) says: nil leaves none (the master reads what is switched off), and
// Content.Hidden is the test of a player. The entries are shared: do not modify them.
func (c *Content) SpellsOfChoiceFilter(f ChoiceFilter, hidden func(key string) bool) []SpellEntry {
	return c.c.spellsOfFilter(f, func(key string) bool {
		return c.c.archived[key] || (hidden != nil && hidden(key))
	})
}

// spellsOfFilter is SpellsOfChoiceFilter where out says every key that is left out, the retired included.
func (c *content) spellsOfFilter(f ChoiceFilter, out func(key string) bool) []SpellEntry {
	var lists []SpellAccess
	for _, l := range f.ClassLists {
		if out == nil || !out(l) {
			lists = append(lists, SpellAccess{List: l, MaxLevel: highestSpellLevel})
		}
	}
	if len(lists) == 0 {
		return nil
	}
	return (&Content{c: c}).ListSpells(SpellFilter{Levels: f.SpellLevels, Schools: f.Schools, Ritual: f.Ritual, Learnable: lists, Hidden: out})
}

// spellPoolOf is the filter a choice of cantrips or of spells reads: its own filter, or the classes it lists, with the
// circles it allows when it names none (the cantrips for a choice of cantrips, circles 1 to 9 for one of spells).
func spellPoolOf(e *Effect) ChoiceFilter {
	var f ChoiceFilter
	if e.Filter != nil {
		f = *e.Filter
	} else {
		f.ClassLists = e.From
	}
	if len(f.SpellLevels) == 0 {
		f.SpellLevels = []int{0}
		if e.Choice == "spell" {
			f.SpellLevels = []int{1, 2, 3, 4, 5, 6, 7, 8, 9}
		}
	}
	return f
}

// poolKeys are the keys of the spells a filter offers the reader of this content, and the picks the sheet holds that the
// pool keeps out of the reader's offer all the same: the choice lists them (the sheet keeps them), and the set says
// they are kept out, so a new pick of one is refused as one that names nothing.
func (cb *choiceBuilder) poolKeys(choiceKey string, f ChoiceFilter) []string {
	c := cb.c
	reader := func(key string) bool { return c.archived[key] || (c.player && c.hidden(key)) }
	var keys []string
	for _, e := range c.spellsOfFilter(f, reader) {
		keys = append(keys, e.Key)
	}
	for _, e := range c.spellsOfFilter(f, nil) {
		if slices.Contains(cb.scoped[choiceKey], e.Key) && !slices.Contains(keys, e.Key) {
			keys = append(keys, e.Key)
			cb.keptOut = append(cb.keptOut, ScopedChoice(choiceKey, e.Key))
		}
	}
	return keys
}

// spellChoice is a feature or trait that lets the character know one more spell of some class lists, circles 1 to 9
// unless a filter says others. The pick is a spell the sheet knows besides the class's own numbers. A choice that
// names no class list asks nothing.
func (cb *choiceBuilder) spellChoice(g *groupBuild, fk string, e *Effect) {
	f := spellPoolOf(e)
	if len(f.ClassLists) == 0 {
		return
	}
	cb.narrowedSpellChoice(g, fk, e, f, false)
}

// narrowedSpellChoice adds a choice of spells whose pool is the reader's: it takes no more picks than the pool offers,
// and an empty pool only warns.
func (cb *choiceBuilder) narrowedSpellChoice(g *groupBuild, fk string, e *Effect, f ChoiceFilter, viaCantrips bool) *Choice {
	keys := cb.poolKeys(fk, f)
	ch := cb.addChoice(g, &Choice{
		Key: fk, FeatureKey: fk, Kind: ChoiceKindSpells, TitlePT: choiceTitle(cb.c, fk), Picks: min(max(e.Count, 1), len(keys)),
		Advisory: len(keys) == 0, viaCantrips: viaCantrips, Narrowed: true, HintPT: cb.spellPoolHint(e, f),
	})
	cb.spellOptions(ch, keys)
	cb.takeScoped(ch)
	return ch
}

// spellPoolHint is the line under the title of a choice of spells: the lists it takes them from (a list the reader may
// not read is not named).
func (cb *choiceBuilder) spellPoolHint(e *Effect, f ChoiceFilter) string {
	var names []string
	for _, l := range f.ClassLists {
		if n := cb.c.namePT(l); n != "" && (!cb.c.player || !cb.c.hidden(l)) {
			names = append(names, n)
		}
	}
	if len(names) == 0 {
		return ""
	}
	if e.Choice == "cantrip" {
		return fmt.Sprintf("Um truque da lista de %s. Ele não conta nos truques da classe.", strings.Join(names, ", "))
	}
	return fmt.Sprintf("Uma magia da lista de %s. Ela não conta nas magias da classe.", strings.Join(names, ", "))
}
