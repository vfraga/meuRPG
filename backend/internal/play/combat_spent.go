package play

import (
	"context"
	"encoding/json"
	"fmt"
	"strconv"
	"strings"

	"github.com/PuraFome/meuRPG/backend/internal/play/playdb"
)

// What a creature of a stat block has spent, kept on the combatant in
// combatants.creature_spent as JSON {"<key>": N}: only what was spent, a key that
// is absent is 0. The keys:
//
//   - "legendary_actions": legendary actions taken since the creature's last turn;
//     ResetCombatantTurn clears it when the creature's own turn starts.
//   - "legendary_resistance": uses of Legendary Resistance spent. A monster starts every combat
//     with none spent; a persistent NPC keeps them until the table's long rest.
//   - "use:<action key>": uses spent of an action with uses a day, or 1 once an action that
//     recharges after a rest was used.
//   - "recharge:<action key>": 1 while a Recharge action is spent.
//
// Every key this file does not know is kept as it is.
//
// A persistent NPC (a character that is not combat_only) keeps "legendary_resistance" and the "use:" keys until the table's long rest,
// in npc_spell_use.creature_spent: bumpCreatureSpent follows them there. The other keys end with the combat.
//
// Only what was spent is stored: a combatant starts with '{}' and an absent key is 0. The monster's
// reaction stays in combatants.reaction_used (one per round, RN-31).
const (
	spentLegendaryActions    = "legendary_actions"
	spentLegendaryResistance = "legendary_resistance"
	// spentUsePrefix and spentRechargePrefix start the keys of the limited actions of a stat block.
	spentUsePrefix      = "use:"
	spentRechargePrefix = "recharge:"
)

// creatureSpentLimit is the most bytes the column's text has (the table's CHECK).
const creatureSpentLimit = 4096

// errCreatureSpentFull is the refusal of a spend that would take the column over its limit.
var errCreatureSpentFull = fmt.Errorf("what the creature spent does not fit in %d bytes", creatureSpentLimit)

// spentValues is the column as stored: every key with its value as written, so a key
// this package does not know comes back out of a change as it went in, whatever its value is.
type spentValues map[string]json.RawMessage

// count is what was spent under key: 0 when it is absent or not a whole number.
func (v spentValues) count(key string) int {
	var n int
	_ = json.Unmarshal(v[key], &n)
	return n
}

// creatureSpentOf reads what a combatant spent for the view: the whole numbers, every key
// of them, the ones this package does not know too.
func creatureSpentOf(raw []byte) map[string]int {
	out := map[string]int{}
	for k, v := range parseSpent(raw) {
		var n int
		if json.Unmarshal(v, &n) == nil {
			out[k] = n
		}
	}
	return out
}

// parseSpent reads the column's JSON as stored.
func parseSpent(raw []byte) spentValues {
	out := spentValues{}
	if len(raw) > 0 {
		_ = json.Unmarshal(raw, &out) // a value this package wrote
	}
	return out
}

// bumpCreatureSpent adds delta to what the combatant spent under key, never below 0,
// and returns what it spent now. A key that reaches 0 is dropped (only what was
// spent is kept), and every other key stays as it is. A result over 4 KiB is refused
// with errCreatureSpentFull and nothing changes.
func bumpCreatureSpent(ctx context.Context, q *playdb.Queries, id, key string, delta int) (map[string]int, error) {
	spent, err := readCreatureSpent(ctx, q, id)
	if err != nil {
		return nil, err
	}
	var cur int
	_ = json.Unmarshal(spent[key], &cur) // a missing or odd value counts as 0
	if next := max(cur+delta, 0); next > 0 {
		spent[key] = json.RawMessage(strconv.Itoa(next))
	} else {
		delete(spent, key)
	}
	if err := writeCreatureSpent(ctx, q, id, spent); err != nil {
		return nil, err
	}
	if keptUntilLongRest(key) {
		if err := q.SyncNPCCreatureSpent(ctx, playdb.SyncNPCCreatureSpentParams{ID: id, Key: key}); err != nil {
			return nil, fmt.Errorf("count what the NPC spent: %w", err)
		}
	}
	body, _ := json.Marshal(spent)
	return creatureSpentOf(body), nil
}

// keptUntilLongRest says a key of creature_spent lasts past the combat for a hand-made NPC: the uses a day
// and Legendary Resistance come back on a long rest, a Recharge action and the legendary actions do not.
func keptUntilLongRest(key string) bool {
	return key == spentLegendaryResistance || strings.HasPrefix(key, spentUsePrefix)
}

// readCreatureSpent reads the column now.
func readCreatureSpent(ctx context.Context, q *playdb.Queries, id string) (spentValues, error) {
	raw, err := q.GetCombatantCreatureSpent(ctx, id)
	if err != nil {
		return nil, fmt.Errorf("read what the creature spent: %w", err)
	}
	return parseSpent(raw), nil
}

// writeCreatureSpent saves spent, refusing what the column cannot hold.
func writeCreatureSpent(ctx context.Context, q *playdb.Queries, id string, spent spentValues) error {
	body, err := json.Marshal(spent)
	if err != nil {
		return fmt.Errorf("encode what the creature spent: %w", err)
	}
	if n := jsonbTextLen(body); n > creatureSpentLimit {
		return fmt.Errorf("%w: %d bytes", errCreatureSpentFull, n)
	}
	if err := q.SetCombatantCreatureSpent(ctx, playdb.SetCombatantCreatureSpentParams{ID: id, CreatureSpent: body}); err != nil {
		return fmt.Errorf("save what the creature spent: %w", err)
	}
	return nil
}

// jsonbTextLen is the length of the text form of compact JSON that the database checks:
// JSONB writes ": " and ", " where compact JSON writes ":" and ",", one byte more for
// each one outside a string.
func jsonbTextLen(compact []byte) int {
	n, inString, escaped := len(compact), false, false
	for _, b := range compact {
		switch {
		case escaped:
			escaped = false
		case inString && b == '\\':
			escaped = true
		case b == '"':
			inString = !inString
		case !inString && (b == ',' || b == ':'):
			n++
		}
	}
	return n
}

// usesLeft is what is left of an allowance of per after spent was used, never below 0.
func usesLeft(per, spent int) int { return max(per-spent, 0) }
