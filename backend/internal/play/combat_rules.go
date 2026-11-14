package play

import (
	"cmp"
	"slices"
	"strconv"
	"strings"
	"unicode/utf8"

	playv1 "github.com/PuraFome/meuRPG/backend/gen/meurpg/play/v1"
	"github.com/PuraFome/meuRPG/backend/internal/play/link"
	"github.com/PuraFome/meuRPG/backend/internal/play/playdb"
)

// The combat's own arithmetic: the turn order, whose turn comes next, the
// grid squares and the state words. Pure functions, no database, so each rule
// has a plain unit test (MR-013, RN-19, RN-20, RN-21).

// The database's values for a combat's status and a combatant's kind
// (encounters_status_valid, combatants_kind_valid).
const (
	statusSetup  = "setup"
	statusActive = "active"
	statusEnded  = "ended"

	kindPlayer = "player"
	kindNPC    = "npc"
	// kindCreature is a creature of a player's character (MR-037): a familiar,
	// summoned animals, a creature the master gave. Its character_id is the
	// owner's and its user_id the owner's player.
	kindCreature = "creature"
)

// holdsHP says whether the combatant keeps its hit points on the combatant row
// (an NPC and a creature) instead of in a character's vitals (a player's
// character).
func holdsHP(c playdb.Combatant) bool { return c.Kind == kindNPC || c.Kind == kindCreature }

// inParty says whether the combatant is on the party's side: a player's
// character or one of its creatures, whom a player plays.
func inParty(c playdb.Combatant) bool { return c.Kind != kindNPC }

// isCreature says whether the combatant is a creature of a character.
func isCreature(c playdb.Combatant) bool { return c.Kind == kindCreature }

// Limits of a combat. Tests cannot go beyond the CHECKs of the tables, which
// say the same.
const (
	// maxCombatants is how many combatants a combat holds: a table never
	// plays more, and the unpaginated answer stays small.
	maxCombatants = 40
	// maxNPCCopies is how many copies of one NPC a participant may ask for.
	maxNPCCopies = 10
	// maxLabelLength is the longest combatant label, in characters.
	maxLabelLength = 40
	// maxEncounterName is the longest combat name, in characters.
	maxEncounterName = 80
)

// orderCombatants returns the combatants in turn order (RN-19): the highest
// initiative total first, then the higher bonus, and, among those still tied,
// the place they already hold (order_index), which is where the master's
// decision lives (SetInitiativeOrder). Combatants without an initiative yet
// come last, in the place they hold. A controlled mount that carries a rider sits
// right after it. It does not change cs.
func orderCombatants(cs []playdb.Combatant) []playdb.Combatant {
	out := slices.Clone(cs)
	slices.SortStableFunc(out, func(a, b playdb.Combatant) int {
		switch {
		case a.Initiative == nil && b.Initiative == nil:
			return cmp.Compare(a.OrderIndex, b.OrderIndex)
		case a.Initiative == nil:
			return 1
		case b.Initiative == nil:
			return -1
		}
		return cmp.Or(
			cmp.Compare(*b.Initiative, *a.Initiative),
			cmp.Compare(b.InitiativeBonus, a.InitiativeBonus),
			cmp.Compare(a.OrderIndex, b.OrderIndex),
		)
	})
	return mountsAfterRiders(out)
}

// sameTie says whether two combatants are tied: the same total and the same
// bonus, which is when the master decides who goes first.
func sameTie(a, b playdb.Combatant) bool {
	return a.Initiative != nil && b.Initiative != nil && *a.Initiative == *b.Initiative && a.InitiativeBonus == b.InitiativeBonus
}

// unresolvedTies returns the IDs of the combatants that are tied with another
// and whose group the master has not ordered yet (tie_ordered).
func unresolvedTies(cs []playdb.Combatant) map[string]bool {
	out := map[string]bool{}
	for i, a := range cs {
		for _, b := range cs[i+1:] {
			if sameTie(a, b) && !joinedPair(a, b) && (!a.TieOrdered || !b.TieOrdered) { // a mount has its rider's total: no tie to order
				out[a.ID], out[b.ID] = true, true
			}
		}
	}
	return out
}

// withOrder puts the given IDs, which are a tie group (the same total and
// bonus), in the new order, inside the places the group holds together in
// cs, which is already in turn order. It returns the new list.
func withOrder(cs []playdb.Combatant, ids []string) []playdb.Combatant {
	out := slices.Clone(cs)
	byID := make(map[string]playdb.Combatant, len(cs))
	var places []int
	for i, c := range cs {
		byID[c.ID] = c
		if slices.Contains(ids, c.ID) {
			places = append(places, i)
		}
	}
	for n, place := range places {
		c := byID[ids[n]]
		c.TieOrdered = true
		out[place] = c
	}
	return out
}

// stateOf is the word that says how hurt an NPC or a creature is (RN-20):
// "Derrotado" when it is out; "Muito ferido" at half of its hit points or
// fewer; "Ferido" when hurt; "Ileso" otherwise. A player's character has none.
func stateOf(c playdb.Combatant) playv1.CombatantState {
	switch {
	case !holdsHP(c) || c.HpMax == nil || c.HpCurrent == nil:
		return playv1.CombatantState_COMBATANT_STATE_UNSPECIFIED
	case c.Defeated || *c.HpCurrent <= 0:
		return playv1.CombatantState_COMBATANT_STATE_DEFEATED
	case *c.HpCurrent*2 <= *c.HpMax:
		return playv1.CombatantState_COMBATANT_STATE_BADLY_HURT
	case *c.HpCurrent < *c.HpMax:
		return playv1.CombatantState_COMBATANT_STATE_HURT
	}
	return playv1.CombatantState_COMBATANT_STATE_UNHURT
}

// squareOf is the grid square a token's position falls in. The position is in
// basis points of the image's width and height; the grid is the squares of
// 1.5 m across the width and the rows down the height. A position on an edge
// belongs to the last square.
func squareOf(g link.Grid, xBP, yBP int32) (col, row int32) {
	col = min(xBP*g.Columns/10000, g.Columns-1)
	row = min(yBP*g.Rows/10000, g.Rows-1)
	return max(col, 0), max(row, 0)
}

// centerOf is the position, in basis points, of the middle of a square: where
// a token goes when a combat ends.
func centerOf(g link.Grid, col, row int32) (xBP, yBP int32) {
	return (2*col + 1) * 10000 / (2 * g.Columns), (2*row + 1) * 10000 / (2 * g.Rows)
}

// inGrid says whether a square is inside the encounter's grid.
func inGrid(e playdb.Encounter, col, row int32) bool {
	return col >= 0 && col < e.GridColumns && row >= 0 && row < e.GridRows
}

// isNumberedCopy says whether label is base and a number, "Goblin 2": another
// creature whose name only begins with base ("Goblin Boss") is not a copy.
func isNumberedCopy(label, base string) bool {
	n, ok := strings.CutPrefix(label, base+" ")
	if !ok || n == "" {
		return false
	}
	_, err := strconv.ParseUint(n, 10, 32)
	return err == nil
}

// copyLabels gives the labels of count copies of an NPC called name: the
// plain name for a single one that stands alone, "Goblin 1", "Goblin 2"...
// otherwise, continuing after the labels the combat already has (taken) so
// two never share one. It adds what it gives to taken. A long name is cut to
// leave room for the number.
func copyLabels(name string, count int, taken map[string]bool) []string {
	base := name
	if utf8.RuneCountInString(base) > maxLabelLength-4 {
		base = string([]rune(base)[:maxLabelLength-4])
	}
	// A single copy keeps the plain name only when no other copy is there.
	others := false
	for label := range taken {
		if label == base || isNumberedCopy(label, base) {
			others = true
		}
	}
	if count == 1 && !others {
		taken[base] = true
		return []string{base}
	}
	n := 1
	if taken[base] {
		n = 2 // "Goblin" is there already: the next one is "Goblin 2"
	}
	out := make([]string, 0, count)
	for ; len(out) < count; n++ {
		if label := base + " " + strconv.Itoa(n); !taken[label] {
			out = append(out, label)
			taken[label] = true
		}
	}
	return out
}
