package rules

import (
	"slices"
	"strings"
)

// The groups of picks an Ability Score Improvement level may swap, when the table plays
// with the rule "Trocar uma escolha no aumento de atributo". Each names one of the
// choice groups the guided level-up knows: the cantrips of the class, or one of the
// option sets (the SRD's and the table's options of the set make the pool).
const (
	SwapGroupCantrip    = "cantrip"
	SwapGroupFightStyle = "fighting_style"
	SwapGroupMetamagic  = "metamagic"
	SwapGroupInvocation = "invocation"
	SwapGroupPactBoon   = "pact_boon"
)

var swapGroupNames = map[string]string{
	SwapGroupCantrip:    "Truque",
	SwapGroupFightStyle: "Estilo de luta",
	SwapGroupMetamagic:  "Metamagia",
	SwapGroupInvocation: "Invocação mística",
	SwapGroupPactBoon:   "Dádiva do pacto",
}

// swapGroupOrder is the order the groups are offered in.
var swapGroupOrder = []string{SwapGroupCantrip, SwapGroupFightStyle, SwapGroupMetamagic, SwapGroupInvocation, SwapGroupPactBoon}

// SwapGroup is one group a level may swap one pick in: the picks the character has (what
// it may give up) and the pool it may take from, without the picks it already has.
type SwapGroup struct {
	Group  string
	NamePT string
	Picks  []NamedKey
	Pool   []NamedKey
}

// PickSwap is the swap the player asks for: From, a pick of the group, becomes To.
type PickSwap struct {
	Group, From, To string
}

// IsZero says nothing is swapped.
func (s PickSwap) IsZero() bool { return s == PickSwap{} }

// setOptions are the options of an option set that the features offering the picks in
// have: the SRD's and the table's.
func (c *content) setOptions(set string, picks []string) []string {
	def, ok := optionSetOf(set)
	if !ok {
		return nil
	}
	var out []string
	for _, f := range def.Offerers {
		feat := c.features[f]
		if feat == nil || !slices.ContainsFunc(feat.Options, func(k string) bool { return slices.Contains(picks, k) }) {
			continue
		}
		for _, k := range feat.Options {
			if !slices.Contains(out, k) {
				out = append(out, k)
			}
		}
	}
	return out
}

// swapGroups lists the groups of the sheet before a level up with something to swap.
func (c *content) swapGroups(b Build, classKey string) []SwapGroup {
	var out []SwapGroup
	for _, name := range swapGroupOrder {
		g := SwapGroup{Group: name, NamePT: swapGroupNames[name]}
		if name == SwapGroupCantrip {
			c.fillCantripGroup(&g, b, classKey)
		} else {
			c.fillChoiceGroup(&g, b)
		}
		if len(g.Picks) > 0 && len(g.Pool) > 0 {
			out = append(out, g)
		}
	}
	return out
}

// fillCantripGroup fills the cantrip group: the known cantrips and the class's others.
func (c *content) fillCantripGroup(g *SwapGroup, b Build, classKey string) {
	g.Picks = c.keysNamed(b.Cantrips, nil)
	for _, k := range c.sortedSpellKeys() {
		s := c.spells[k]
		if s.Level == 0 && c.onList(s, classKey) && !slices.Contains(b.Cantrips, k) {
			g.Pool = append(g.Pool, NamedKey{Key: k, NamePT: c.namePT(k)})
		}
	}
}

// fillChoiceGroup fills a group of feature choices. The pool is what the features that
// offered the picks offer: a fighter's styles, not the paladin's.
func (c *content) fillChoiceGroup(g *SwapGroup, b Build) {
	opts := c.setOptions(g.Group, b.FeatureChoices)
	g.Picks = c.keysNamed(b.FeatureChoices, opts)
	if len(g.Picks) > 0 {
		g.Pool = c.keysNamed(opts, nil)
		g.Pool = slices.DeleteFunc(g.Pool, func(n NamedKey) bool { return slices.Contains(b.FeatureChoices, n.Key) })
	}
}

// keysNamed names the keys, only the ones in among when it is not nil.
func (c *content) keysNamed(keys, among []string) []NamedKey {
	var out []NamedKey
	for _, k := range keys {
		if among != nil && !slices.Contains(among, k) {
			continue
		}
		out = append(out, NamedKey{Key: k, NamePT: c.namePT(k)})
	}
	return out
}

func (c *content) sortedSpellKeys() []string { return sortedKeys(c.spells) }

// applySwap gives up the pick and takes the other, in the lists the group lives in.
func applySwap(after *Build, s PickSwap) {
	if s.IsZero() {
		return
	}
	list := &after.FeatureChoices
	if s.Group == SwapGroupCantrip {
		list = &after.Cantrips
	}
	*list = slices.DeleteFunc(*list, func(k string) bool { return k == s.From || ownPick(s.From, k) })
	*list = append(*list, s.To)
}

// ownPick says the stored pick k is a pick of the choice that belongs to the option `from`
// (the two rituals of the Book of Ancient Secrets), which goes away with it.
func ownPick(from, k string) bool {
	return strings.HasPrefix(k, from+textSeparator)
}

// ownedByAnyOf says the stored pick k belongs to the choice of one of the options in keys.
func ownedByAnyOf(keys []string, k string) bool {
	choiceKey, _, ok := SplitScopedChoice(k)
	if !ok {
		return false
	}
	owner, _, _ := strings.Cut(choiceKey, textSeparator)
	return slices.Contains(keys, owner)
}

// swapOf finds the swap the new sheet shows: a group of the offer in which the sheet gave
// up one pick (To is the pool option the sheet gained, "" when it gained none). found is
// false when no pick of an offered group was given up.
func swapOf(before, after Build, offer LevelUpOffer) (swap PickSwap, found bool) {
	for _, g := range offer.PickSwaps {
		if swap, found = swapInGroup(before, after, g); found {
			return swap, true
		}
	}
	return PickSwap{}, false
}

// swapInGroup finds the swap in one group: the pick the sheet gave up and the pool option it
// took. found is false when no pick was given up, or one was lost with nothing in its place
// (which is a swap only for a cantrip).
func swapInGroup(before, after Build, g SwapGroup) (swap PickSwap, found bool) {
	was, now := before.FeatureChoices, after.FeatureChoices
	if g.Group == SwapGroupCantrip {
		was, now = before.Cantrips, after.Cantrips
	}
	var gone string
	for _, n := range g.Picks {
		if !slices.Contains(now, n.Key) {
			gone = n.Key
		}
	}
	if gone == "" {
		return PickSwap{}, false
	}
	swap = PickSwap{Group: g.Group, From: gone}
	for _, n := range g.Pool {
		if slices.Contains(now, n.Key) && !slices.Contains(was, n.Key) {
			swap.To = n.Key
		}
	}
	return swap, swap.To != "" || g.Group == SwapGroupCantrip
}

// Offers says the offer lets the swap be made: the group is offered, from is one of its
// picks and to is in its pool.
func (o LevelUpOffer) Offers(s PickSwap) bool { return groupOffers(o.PickSwaps, s) }

// groupOffers says the swap is one of the groups: its group is there, from is one of its picks and to is in its pool.
func groupOffers(groups []SwapGroup, s PickSwap) bool {
	for _, g := range groups {
		if g.Group == s.Group {
			return slices.ContainsFunc(g.Picks, func(n NamedKey) bool { return n.Key == s.From }) &&
				slices.ContainsFunc(g.Pool, func(n NamedKey) bool { return n.Key == s.To })
		}
	}
	return false
}
