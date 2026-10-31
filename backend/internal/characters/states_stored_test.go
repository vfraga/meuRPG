package characters

import (
	"reflect"
	"testing"

	playv1 "github.com/PuraFome/meuRPG/backend/gen/meurpg/play/v1"
	"github.com/PuraFome/meuRPG/backend/internal/rules"
)

// SetActiveStates turns the states it is given into the column's entries directly: the same entries the
// column holds, with no round left below 0. No database is needed.
func TestStoredFromIsWhatTheColumnHoldsAndShowsTheStatesTheContentDefines(t *testing.T) {
	t.Parallel()
	states := []*playv1.ActiveState{{Key: "a#postura", RoundsLeft: 4}, {Key: "a#forma", RoundsLeft: -3}, {Key: "a#gone"}}
	stored := storedFrom(states)
	if want := []storedState{{Key: "a#postura", RoundsLeft: 4}, {Key: "a#forma"}, {Key: "a#gone"}}; !reflect.DeepEqual(stored, want) {
		t.Errorf("storedFrom = %+v, want %+v", stored, want)
	}
	if got := parseStoredStates(storedOf(states)); !reflect.DeepEqual(got, stored) {
		t.Errorf("the column reads back %+v, want the entries %+v", got, stored)
	}
	defs := []rules.State{{Key: "a#postura", DurationRounds: 10}, {Key: "a#forma", DurationRounds: 10}}
	shown := activeStatesProto(stored, defs)
	if len(shown) != 2 || shown[0].GetKey() != "a#postura" || shown[0].GetRoundsLeft() != 4 || shown[1].GetRoundsLeft() != 0 {
		t.Errorf("activeStatesProto = %v, want the two states the content defines, with 4 and 0 rounds left", shown)
	}
}
