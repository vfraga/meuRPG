package rules

import (
	"maps"
	"slices"
	"strings"

	"github.com/PuraFome/meuRPG/backend/internal/rules/srd51"
)

// A choice of features that offers the options of another feature (from_feature): the options are the named feature's
// own list (the SRD's or a table entry's), shared as one pool with every choice that offers the same options. The pool is
// worked out for the sheet that reads it: an option the table retired or switched off with its entry is no choice for a
// sheet that does not hold that entry, and a pick that names one reads as a key the content does not have.

// checkFromFeature checks a choice that offers the options of a feature and gives the compiled choice the list: the
// feature is a class or subclass feature, the key names a feature of the content that has options of its own, and the
// choice asks for no more than it lists.
func (b *overlayBuilder) checkFromFeature(site effectSite, e *Effect) error {
	switch {
	case b.optionEntry[site.owner] != "":
		return site.fail(".type", ReasonEffect, "an option does not ask for another choice")
	case e.Choice != "feature":
		return site.fail(".from_feature", ReasonValue, "only a choice of features offers the options of a feature")
	case !strings.HasPrefix(site.owner, featureKeyPrefix):
		return site.fail(".from_feature", ReasonValue, "only a class or subclass feature offers the options of another feature")
	case len(e.From) > 0 || e.Set != "":
		return site.fail(".from_feature", ReasonValue, "a choice offers a list, a set, a filter or the options of a feature, not several")
	}
	if b.featureChoices(site.owner) > 1 {
		return site.fail(".from_feature", ReasonValue, "a feature that offers the options of another feature asks for no other choice of features")
	}
	src, err := b.fromFeatureSource(site, e.FromFeature)
	if err != nil {
		return err
	}
	if e.Count > len(src.Options) {
		return site.fail(".count", ReasonValue, "a choice cannot ask for more than the %d options of %q", len(src.Options), e.FromFeature)
	}
	e.From = slices.Clone(src.Options)
	return nil
}

// featureChoices counts the choices of features that the owner makes, whatever their order.
func (b *overlayBuilder) featureChoices(owner string) int {
	n := 0
	for _, p := range b.pending {
		if p.owner != owner {
			continue
		}
		for i := range p.effects {
			if isFeatureChoice(&p.effects[i]) {
				n++
			}
		}
	}
	return n
}

// fromFeatureSource is the feature a choice offers the options of, when it is one that may be offered.
func (b *overlayBuilder) fromFeatureSource(site effectSite, key string) (*srd51.Feature, error) {
	f := b.n.features[key]
	switch {
	case f == nil || !strings.HasPrefix(key, featureKeyPrefix):
		return nil, site.fail(".from_feature", ReasonReference, "%q is not a feature of the content", key)
	case f.Parent != "":
		return nil, site.fail(".from_feature", ReasonReference, "%q is an option, not a feature with options", key)
	case key == site.owner:
		return nil, site.fail(".from_feature", ReasonValue, "a feature offers its own options with no list: leave from_feature empty")
	case key == invocationsFeature:
		return nil, site.fail(".from_feature", ReasonValue, "the eldritch invocations are dealt by the class table: they are no list to offer")
	case b.n.fromFeature[key] != "":
		return nil, site.fail(".from_feature", ReasonReference, "%q offers another feature's options, which is no list to offer", key)
	case len(f.Options) == 0:
		return nil, site.fail(".from_feature", ReasonReference, "%q has no options", key)
	}
	return f, nil
}

// offerFromFeatures makes each feature whose choice offers the options of another feature the offerer of them, once
// every entry exists: the feature's options are the named feature's, and the content remembers which feature each of
// those options belongs to. A choice that names something else is left alone: compiling it says what is wrong.
func (b *overlayBuilder) offerFromFeatures() {
	n := b.n
	asks := map[string]Effect{}
	for _, p := range b.pending {
		for i := range p.effects {
			if e := p.effects[i]; isFeatureChoice(&e) && e.FromFeature != "" {
				asks[p.owner] = e
				n.fromFeature[p.owner] = e.FromFeature
			}
		}
	}
	for _, owner := range slices.Sorted(maps.Keys(asks)) {
		e := asks[owner]
		f, src := n.features[owner], n.features[e.FromFeature]
		if f == nil || src == nil || len(src.Options) == 0 || src.Parent != "" || n.fromFeature[e.FromFeature] != "" || len(f.Options) > 0 {
			continue
		}
		f.Options, f.OptionsChoose = slices.Clone(src.Options), e.Count
		for _, o := range src.Options {
			n.askedBy[o] = append(n.askedBy[o], owner)
		}
	}
}

// offerers are the features whose choice lists the option: the ones it belongs to, and the ones that ask for it.
func (c *content) offerers(key string) []string {
	return append(slices.Clone(c.offeredBy[key]), c.askedBy[key]...)
}

// optionFilter says, for the pool of the choice that owner makes, which options the table keeps out of a sheet's offer: an
// option that is retired, one that is switched off (for a player) and, for a player, the options of a feature whose
// entry is switched off while the sheet holds none of it; an entry that is retired closes its feature for everyone.
// Only a choice that offers the options of another feature has a filter: every other pool is as it was. The filter
// says nothing of what the sheet holds, which keepsPick does.
func (c *content) optionFilter(b Build, owner string) func(option string) bool {
	src := c.fromFeature[owner]
	if src == "" {
		return nil
	}
	closed := c.sourceClosed(b, src)
	return func(o string) bool { return c.archived[o] || (c.player && c.off[o]) || closed }
}

// sourceClosed says the feature whose options a choice offers is out of the sheet's reach: its entry is retired, or
// (for a player) switched off, and the sheet holds none of it.
func (c *content) sourceClosed(b Build, source string) bool {
	view := c
	if !c.player {
		x := *c
		x.off = nil // the master reads what is switched off
		view = &x
	}
	return view.hidden(source) && !slices.ContainsFunc(view.entryOwners(source, 0), func(owner string) bool { return c.sheetHolds(b, owner) })
}

// visibleOptions are the options the pool of the choice that owner makes offers a sheet: the list without what the table
// keeps out of it. A pick the sheet already holds stays.
func (c *content) visibleOptions(b Build, owner string, options []string) []string {
	out := c.optionFilter(b, owner)
	if out == nil {
		return options
	}
	return slices.DeleteFunc(slices.Clone(options), func(o string) bool { return out(o) && !slices.Contains(b.FeatureChoices, o) })
}

// keptOutPicks are the picks of the sheet that the pool of the choice that owner makes keeps out, and the sheet holds all
// the same.
func (c *content) keptOutPicks(b Build, owner string, options []string) []string {
	out := c.optionFilter(b, owner)
	if out == nil {
		return nil
	}
	return slices.DeleteFunc(slices.Clone(options), func(o string) bool { return !out(o) || !slices.Contains(b.FeatureChoices, o) })
}

// requiredPicks is how many picks a choice asks of a sheet, the one number every check of completeness reads, for a
// player and for the master alike: the count the choice asks, or fewer when a player has fewer picks to make today. A
// player may make the picks the sheet already holds (held) and one for each option a player may pick now that the sheet
// does not hold (pickable). The master may still pick an entry that is only switched off, but is never asked to.
func requiredPicks(count int, pickable, held []string) int {
	n := len(held)
	for _, o := range pickable {
		if !slices.Contains(held, o) {
			n++
		}
	}
	return min(count, n)
}

// optionShares splits the pool that the choice owner makes lists (options) by what a player may do with it: the options
// the table keeps out of a player's view are no pick to make, so they are in neither list; pickable are the ones a player
// may pick now and held the ones the sheet already holds. filtered says the table keeps something out of this choice's
// pool; a pool the table keeps nothing out of asks for its count.
func (c *content) optionShares(b Build, owner string, options []string) (pickable, held []string, filtered bool) {
	out := c.asPlayer().optionFilter(b, owner)
	for _, o := range options {
		switch {
		case slices.Contains(b.FeatureChoices, o):
			held = append(held, o)
		case out == nil || !out(o):
			pickable = append(pickable, o)
		}
	}
	return pickable, held, out != nil
}

// requiredOptionPicks is requiredPicks for the choice that owner makes alone, whose pool lists options. Choices that
// share a pool ask for the picks of the pool in all, dealt in the order of the picks (pool.dealRequired).
func (c *content) requiredOptionPicks(b Build, owner string, count int, options []string) int {
	pickable, held, filtered := c.optionShares(b, owner, options)
	if !filtered {
		return count
	}
	return requiredPicks(count, pickable, held)
}
