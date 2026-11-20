package rules

import (
	"fmt"
	"slices"
	"strconv"

	"github.com/PuraFome/meuRPG/backend/internal/rules/srd51"
)

// FeatureSwapLevels says at which class levels the picks of a table feature's option pool may be swapped, one pick
// for another option of the pool, as part of the level-up. The zero value is never. Levels and EveryLevel are not set
// together.
type FeatureSwapLevels struct {
	// Levels are the class levels (1 to MaxLevel, each once) at which a swap is allowed.
	Levels []int
	// EveryLevel allows a swap at every class level from the feature's own level on.
	EveryLevel bool
}

// IsZero says the feature never allows a swap.
func (s FeatureSwapLevels) IsZero() bool { return len(s.Levels) == 0 && !s.EveryLevel }

// allows says the swap is allowed at class level level, for a feature that is at class level from.
func (s FeatureSwapLevels) allows(level, from int) bool {
	if s.EveryLevel {
		return level >= from
	}
	return slices.Contains(s.Levels, level)
}

// addSwapLevels checks the swap levels of the class or subclass feature f, which writeFeature has put in the clone, and keeps
// them. Only a feature that has options may allow a swap.
func (b *overlayBuilder) addSwapLevels(f *TableFeature, path string) error {
	s := f.SwapLevels
	if s.IsZero() {
		return nil
	}
	at := path + ".swap_levels"
	if s.EveryLevel && len(s.Levels) > 0 {
		return ovErr(f.Key, "the swap levels are a list or every level, not both").at(at+".every_level", ReasonValue)
	}
	for i, n := range s.Levels {
		switch {
		case n < 1 || n > MaxLevel:
			return ovErr(f.Key, "a swap level is 1 to %d", MaxLevel).at(fmt.Sprintf("%s.levels[%d]", at, i), ReasonValue)
		case slices.Contains(s.Levels[:i], n):
			return ovErr(f.Key, "swap level %d is listed twice", n).at(fmt.Sprintf("%s.levels[%d]", at, i), ReasonValue)
		}
	}
	if slices.ContainsFunc(f.Effects, func(e Effect) bool { return e.FromFeature != "" }) {
		return ovErr(f.Key, "a feature that offers the options of another feature cannot allow swaps: drop the swap levels or give it options of its own").at(at, ReasonValue)
	}
	if len(b.n.features[f.Key].Options) == 0 {
		return ovErr(f.Key, "only a feature with options can allow a swap").at(at, ReasonValue)
	}
	b.n.featureSwaps[f.Key] = FeatureSwapLevels{Levels: slices.Clone(s.Levels), EveryLevel: s.EveryLevel}
	return nil
}

// ownedFeature is a feature of a class or subclass and the class level it is at.
type ownedFeature struct {
	key   string
	level int
}

// ownedFeatures are the features of the class, and of the subclass sub when it is not nil, from level 1 to level.
func (c *content) ownedFeatures(classKey string, sub *srd51.Subclass, level int) (class, subclass []ownedFeature) {
	for lvl := 1; lvl <= level; lvl++ {
		classKeys, subKeys := c.levelFeatures(classKey, sub, lvl)
		for _, k := range classKeys {
			class = append(class, ownedFeature{key: k, level: lvl})
		}
		for _, k := range subKeys {
			subclass = append(subclass, ownedFeature{key: k, level: lvl})
		}
	}
	return class, subclass
}

// featureSwapGroups are the groups of picks the class may swap one in when it gains class level newLevel, by what the
// table's features say (FeatureSwapLevels): one for each feature of the class, and of the subclass sub, that the
// character has by that level and that allows a swap at it, whatever the table's swap rule.
func (c *content) featureSwapGroups(b Build, classKey string, sub *srd51.Subclass, newLevel int, newClass bool) []SwapGroup {
	if newClass || len(c.featureSwaps) == 0 {
		return nil
	}
	class, subclass := c.ownedFeatures(classKey, sub, newLevel)
	return slices.Concat(c.entrySwapGroups(b, class, newLevel), c.entrySwapGroups(b, subclass, newLevel))
}

// entrySwapGroups are the groups of the features one entry (a class or a subclass) owns. The pool of a feature is its options
// and those of the entry's other features that share an option with it, as the choices of the sheet pool them
// (groupPools).
func (c *content) entrySwapGroups(b Build, owned []ownedFeature, newLevel int) []SwapGroup {
	entries := map[string]*optionEntry{}
	var all []*optionEntry
	for _, o := range owned {
		if f := c.features[o.key]; f != nil && len(f.Options) > 0 {
			entries[o.key] = &optionEntry{options: f.Options}
			all = append(all, entries[o.key])
		}
	}
	var out []SwapGroup
	for _, o := range owned {
		if e := entries[o.key]; e != nil && c.featureSwaps[o.key].allows(newLevel, o.level) {
			if g, ok := c.featureSwapGroup(b, o.key, poolOptionsOf(all, e)); ok {
				out = append(out, g)
			}
		}
	}
	return out
}

// featureSwapGroup is the group of the feature key over its pool: the sheet's picks inside the pool and the options of the
// pool it does not have, leaving out the ones the master archived or switched off. ok is false when there is nothing to give up
// or nothing to take.
func (c *content) featureSwapGroup(b Build, key string, pool []string) (SwapGroup, bool) {
	g := SwapGroup{Group: key, NamePT: c.namePT(key), Picks: c.keysNamed(b.FeatureChoices, pool)}
	for _, k := range pool {
		if !slices.Contains(b.FeatureChoices, k) && !c.archived[k] && !c.off[k] {
			g.Pool = append(g.Pool, NamedKey{Key: k, NamePT: c.namePT(k)})
		}
	}
	return g, len(g.Picks) > 0 && len(g.Pool) > 0
}

// AllowsFeatureSwaps says the offer lets every swap be made: at most one for each feature group, the given-up options
// all different and the taken ones all different, each given up from the group's picks and taken from its pool.
func (o LevelUpOffer) AllowsFeatureSwaps(swaps []PickSwap) bool {
	groups, from, to := map[string]bool{}, map[string]bool{}, map[string]bool{}
	for _, s := range swaps {
		if groups[s.Group] || from[s.From] || to[s.To] || !groupOffers(o.FeatureSwaps, s) {
			return false
		}
		groups[s.Group], from[s.From], to[s.To] = true, true, true
	}
	return true
}

// applyFeatureSwap gives up the option s.From for s.To, which takes its place among the sheet's picks: the choices deal the
// picks in order, so the new option stays with the choice that had the old one. A swap of an option that is not a pick changes
// nothing (the offer never lets one through).
func applyFeatureSwap(after *Build, s PickSwap) {
	if i := slices.Index(after.FeatureChoices, s.From); i >= 0 {
		after.FeatureChoices[i] = s.To
	}
}

// unmatchedRemovals are the removed picks that no feature group takes in: a group takes in at most one pick of its own, and a
// pick goes to one group only.
func unmatchedRemovals(gone []string, groups []SwapGroup) []string {
	owner := map[string]int{}
	var take func(g int, seen map[string]bool) bool
	take = func(g int, seen map[string]bool) bool {
		for _, n := range groups[g].Picks {
			if !slices.Contains(gone, n.Key) || seen[n.Key] {
				continue
			}
			seen[n.Key] = true
			if prev, taken := owner[n.Key]; !taken || take(prev, seen) {
				owner[n.Key] = g
				return true
			}
		}
		return false
	}
	for g := range groups {
		take(g, map[string]bool{})
	}
	return slices.DeleteFunc(slices.Clone(gone), func(k string) bool { _, ok := owner[k]; return ok })
}

// withSwapLine is the text of the table feature key, which comes from source (a class or a subclass) at class level level,
// with the line that says at which levels one of its options can be swapped; the text of a feature that allows no swap is
// the master's own. The line is markdown: its lead is bold.
func (c *content) withSwapLine(key, source string, level int, desc []string) []string {
	s, ok := c.featureSwaps[key]
	if !ok {
		return desc
	}
	return slices.Concat(desc, []string{swapLinePT(s, c.namePT(c.classOfSource(source)), level)})
}

// classOfSource is the class of a class or subclass key.
func (c *content) classOfSource(source string) string {
	if sub, ok := c.subclasses[source]; ok {
		return sub.Class
	}
	return source
}

// swapLinePT says, in Portuguese, when a player may swap an option of a feature of the class named class that is at class
// level from.
func swapLinePT(s FeatureSwapLevels, class string, from int) string {
	var when string
	switch levels := slices.Sorted(slices.Values(s.Levels)); {
	case s.EveryLevel:
		when = fmt.Sprintf("em todo nível de %s a partir do nível %d", class, from)
	case len(levels) == 1:
		when = fmt.Sprintf("no nível %d de %s", levels[0], class)
	default:
		names := make([]string, len(levels))
		for i, n := range levels {
			names[i] = strconv.Itoa(n)
		}
		when = fmt.Sprintf("nos níveis %s de %s", joinList(names), class)
	}
	return "**Troca:** " + when + " você pode trocar uma opção desta característica por outra."
}
