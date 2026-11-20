package rules

import "slices"

// A pick source is where the sheet's chosen skills, expertise or tools come from: a class, a trait, a feature or an effect
// that lets the player choose Count of a list. The sheet's picks are one flat list; each one is placed in a source that
// allows it, and no source takes more than its Count.

// pickSource is one list the player chooses from. A source with Any set (a choice that names no list) allows every key.
type pickSource struct {
	Owner string
	Count int
	From  []string
	Any   bool
}

// allows says the source lets the player choose key.
func (s pickSource) allows(key string) bool {
	return s.Any || slices.Contains(s.From, key)
}

// matchPicks places the picks in the sources: the result holds, for each pick, the index of the source that takes it, or -1
// when the placement leaves it out. The placement is a maximum matching of picks to slots (each source has Count slots), so a
// pick is left out only when no placement takes it with the others; the earlier picks stay placed in preference to the later.
func matchPicks(sources []pickSource, picks []string) []int {
	var slotSource []int
	for i, s := range sources {
		for range s.Count {
			slotSource = append(slotSource, i)
		}
	}
	slotPick := make([]int, len(slotSource))
	for i := range slotPick {
		slotPick[i] = -1
	}
	for p := range picks {
		place(sources, picks, slotSource, slotPick, p, make([]bool, len(slotSource)))
	}
	out := make([]int, len(picks))
	for i := range out {
		out[i] = -1
	}
	for slot, p := range slotPick {
		if p >= 0 {
			out[p] = slotSource[slot]
		}
	}
	return out
}

// place finds a slot for pick p, moving the picks that hold the slots it needs to other slots when they can go (an
// augmenting path); tried marks the slots the search has visited.
func place(sources []pickSource, picks []string, slotSource, slotPick []int, p int, tried []bool) bool {
	for slot, si := range slotSource {
		if tried[slot] || !sources[si].allows(picks[p]) {
			continue
		}
		tried[slot] = true
		if slotPick[slot] < 0 || place(sources, picks, slotSource, slotPick, slotPick[slot], tried) {
			slotPick[slot] = p
			return true
		}
	}
	return false
}

// unplacedPicks are the picks that no placement in the sources takes, in the order of the picks.
func unplacedPicks(sources []pickSource, picks []string) []string {
	var out []string
	for i, source := range matchPicks(sources, picks) {
		if source < 0 {
			out = append(out, picks[i])
		}
	}
	return out
}
