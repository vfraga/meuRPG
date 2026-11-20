package play

import (
	"maps"
	"reflect"
	"slices"
	"testing"
)

// guardReplay restores combatTx with a shallow copy: a refused replay gets back its values and the header of every slice,
// but not what the replay wrote into a map, into an element of a slice, or through a pointer.
// Every field of a kind that holds such state is named here with how a refused replay restores it.
// A new field of that kind: decide how a refused replay restores it, make guardReplay do it, and add it here.
var replayRestoredBy = map[string]string{
	"tx":            "not changed by a replay: nothing assigns it after openTx, and the savepoint is the guard's own sp",
	"q":             "not changed by a replay: nothing assigns it, and it holds no state of its own",
	"hints":         "marked by guardReplay and rewound by undo (hints.rewind): a replay only appends to it and sets its flags",
	"characterID":   "a replay assigns a new pointer and never writes through it: the copy and writeAmbient put the old one back",
	"meta":          "applyMetamagic assigns a new castMeta (combat_cast.go) and nothing writes into the old one",
	"sculpted":      "castFlow assigns a new map (combat_cast.go) and nothing writes into the old one",
	"svc":           "not changed by a replay: the service's own state (its seams, hub and caches) is not written by a combat change",
	"told":          "append only during a replay: the restored header hides what it added",
	"sight":         "its maps memoize a read of what the replay does not write (combat sight, maps/fogcombat.go): a refusal leaves what a fresh read gives",
	"unseated":      "append only during a replay (keepUnseated sets it nil): the restored header gives the riders back",
	"stamped":       "writeAmbient puts it back; insertEventID assigns the address of a new event and nothing writes through it",
	"lines":         "append only during a replay: the restored header hides what it added",
	"hpBefore":      "assigned by snapshotHP in write, before the replay: a replay only reads it",
	"endedEffects":  "append only during a replay: the restored header hides what it added",
	"endedLines":    "append only during a replay: the restored header hides what it added",
	"followUps":     "append only during a replay: the restored header hides what it added",
	"followUpLines": "append only during a replay: the restored header hides what it added",
	"tempCleared":   "append only during a replay: the restored header hides what it added",
	"zoneOut":       "append only during a replay (the flush sets it nil): the restored header gives the lines back",
	"hash":          "not changed by a replay: only read",
	"reactionNotes": "append only during a replay: the restored header hides what it added",
	"landed":        "append only during a replay: the restored header hides what it added",
	"replay":        "the replayState of this release, dropped with it: a replay writes its after (cutRoll), and nothing reads it once refused",
	"pushed":        "append only during a replay (settleReplay adds to it after the guard is kept): the restored header hides what it added",
	"tellIDs":       "append only during a replay (tellActorVitals sorts it in place, but only after the guard is kept)",
	"answered":      "append only during a replay: the restored header hides what it added",
	"session":       "a struct value: a replay never writes through its pointers or into its slices",
	"enc":           "a struct value: a replay assigns a whole new Encounter and never writes through its pointers or into SunlightSquares",
	"now":           "a time.Time value: nothing writes through its location",
	"rules":         "a struct value: a replay never writes into Reminders",
}

// A field of combatTx that a shallow copy does not give back whole must say how a refused replay restores it.
func TestEveryReferenceFieldOfCombatTxSaysHowAReplayRestoresIt(t *testing.T) {
	typ := reflect.TypeFor[combatTx]()
	shared := []reflect.Kind{reflect.Map, reflect.Slice, reflect.Pointer, reflect.Interface, reflect.Func, reflect.Chan}
	holds := map[string]bool{}
	for f := range typ.Fields() {
		if !holdsReference(f.Type, shared) {
			continue
		}
		holds[f.Name] = true
		if replayRestoredBy[f.Name] == "" {
			t.Errorf("combatTx.%s is (or holds) a %s, which the shallow copy in guardReplay shares with the replay: decide how a refused replay restores it (clone it in guardReplay, or show that the replay only appends to it or never writes it) and add it to replayRestoredBy in replay_guard_fields_test.go", f.Name, f.Type.Kind())
		}
	}
	for _, name := range slices.Sorted(maps.Keys(replayRestoredBy)) {
		if !holds[name] {
			t.Errorf("replayRestoredBy names %q, which is no longer a field of combatTx that is or holds a map, slice, pointer, interface, func or chan: take it out", name)
		}
	}
}

// holdsReference says whether a value of t shares state with its copy: t is one of the shared kinds, or a struct or array
// that holds one.
func holdsReference(t reflect.Type, shared []reflect.Kind) bool {
	switch {
	case slices.Contains(shared, t.Kind()):
		return true
	case t.Kind() == reflect.Array:
		return holdsReference(t.Elem(), shared)
	case t.Kind() == reflect.Struct:
		for f := range t.Fields() {
			if holdsReference(f.Type, shared) {
				return true
			}
		}
	}
	return false
}
