package play

import (
	"slices"
	"testing"

	"github.com/PuraFome/meuRPG/backend/internal/play/infusion"
	"github.com/PuraFome/meuRPG/backend/internal/play/playdb"
	"github.com/PuraFome/meuRPG/backend/internal/rules"
)

// These tests need no database: they freeze which events lastAction walks past
// (the ones an undo never closes the chain on), which ones close the chain, and
// which one it hands to the undo.

// chainRow is one row of the session's latest events for the chain tests.
type chainRow struct {
	id, kind string
	enc      string // the combat of the row; "" for none
	seq      int32
	ev       actionEvent
	raw      []byte // a payload that is not an event, when set
}

func chainRows(t *testing.T, rows ...chainRow) []playdb.ListRecentSessionEventsRow {
	t.Helper()
	out := make([]playdb.ListRecentSessionEventsRow, len(rows))
	for i, r := range rows {
		row := playdb.ListRecentSessionEventsRow{ID: r.id, Kind: r.kind, Seq: r.seq, Payload: r.raw}
		if r.raw == nil {
			row.Payload = payload(t, r.ev)
		}
		if r.enc != "" {
			enc := r.enc
			row.EncounterID = &enc
		}
		out[i] = row
	}
	return out
}

// TestLastActionChain freezes every rule of the walk, newest event first: what is
// passed over, what closes the chain, and what is taken.
func TestLastActionChain(t *testing.T) {
	t.Parallel()
	const enc = "enc"
	badJSON := []byte("{not json")
	// The action an undo would take back, under the event being tried.
	under := chainRow{id: "act", kind: eventAttackRolled, enc: enc, seq: 1}
	tests := []struct {
		name string
		rows []chainRow // newest first
		want string     // "" for nothing to undo
	}{
		{"a summoned creature is passed over", []chainRow{{id: "x", kind: eventCreatureSummoned, enc: enc}, under}, "act"},
		{"a dismissed creature is passed over", []chainRow{{id: "x", kind: eventCreatureDismissed, enc: enc}, under}, "act"},
		{"a concentration that ended at 0 hit points is passed over", []chainRow{{id: "x", kind: eventDamageApplied, enc: enc, ev: actionEvent{Reaction: &reactionEvent{Kind: concentrationEnd}}}, under}, "act"},
		{"a table state that ended at 0 hit points is passed over", []chainRow{{id: "x", kind: eventStateChanged, enc: enc, ev: actionEvent{StateKind: stateTable, StateReason: stateEndIncapacitated}}, under}, "act"},
		{"a table state the master ended is passed over", []chainRow{{id: "x", kind: eventStateChanged, enc: enc, ev: actionEvent{StateKind: stateTable, StateReason: stateEndMaster}}, under}, "act"},
		{"a table state that ran out of rounds closes the chain", []chainRow{{id: "x", kind: eventStateChanged, enc: enc, ev: actionEvent{StateKind: stateTable, StateReason: stateEndDuration}}, under}, ""},
		{"a reaction of another kind is not passed over", []chainRow{{id: "x", kind: eventReactionUsed, enc: enc, ev: actionEvent{Reaction: &reactionEvent{Kind: "shield"}}}, under}, "x"},
		{"an opportunity offer of a move is passed over", []chainRow{{id: "x", kind: eventOpportunityOffered, enc: enc}, under}, "act"},
		{"an opportunity offer made by hand closes the chain", []chainRow{{id: "x", kind: eventOpportunityOffered, enc: enc, ev: actionEvent{ByHand: true}}, under}, ""},
		{"an opportunity offer with a broken payload is passed over", []chainRow{{id: "x", kind: eventOpportunityOffered, enc: enc, raw: badJSON}, under}, "act"},
		{"a door opened is passed over", []chainRow{{id: "x", kind: eventDoorOpened, enc: enc}, under}, "act"},
		{"a hider that was found is passed over", []chainRow{{id: "x", kind: eventHideEnded, enc: enc}, under}, "act"},
		{"a puzzle line is passed over", []chainRow{{id: "x", kind: eventPuzzleShown}, {id: "y", kind: eventPuzzleReset}, under}, "act"},
		{"a favor line is passed over", []chainRow{{id: "x", kind: eventFavorGranted}, {id: "y", kind: eventFavorPassed}, under}, "act"},
		{"an infusion line closes the chain", []chainRow{{id: "x", kind: infusion.KindInfused}, under}, ""},
		{"an infusion moved closes the chain", []chainRow{{id: "x", kind: infusion.KindMoved}, under}, ""},
		{"an infusion ended closes the chain", []chainRow{{id: "x", kind: infusion.KindEnded}, under}, ""},
		{"the clue of a solve is passed over", []chainRow{{id: "s", kind: eventPuzzleSolved, seq: 6}, {id: "x", kind: eventClueRevealed, seq: 5}, under}, "act"},
		{"a clue with no solve after it closes the chain", []chainRow{{id: "x", kind: eventClueRevealed, seq: 5}, under}, ""},
		{"a trap noticed is passed over", []chainRow{{id: "x", kind: eventTrapNoticed}, under}, "act"},
		{"a trap search of another combat is passed over", []chainRow{{id: "x", kind: eventTrapSearched, enc: "other"}, under}, "act"},
		{"a trap disarmed with no combat is passed over", []chainRow{{id: "x", kind: eventTrapDisarmed}, under}, "act"},
		{"a trap damage settled with no combat is passed over", []chainRow{{id: "x", kind: eventDamageApplied}, under}, "act"},
		{"a trap damage discarded with no combat is passed over", []chainRow{{id: "x", kind: eventDamageDiscarded}, under}, "act"},
		{"a trap search of this combat closes the chain", []chainRow{{id: "x", kind: eventTrapSearched, enc: enc}, under}, ""},
		{"a wild shape that ended by the beast's fall is passed over", []chainRow{{id: "x", kind: eventWildShapeEnded, enc: enc, ev: actionEvent{Reason: endedByDamage}}, under}, "act"},
		{"a wild shape that ended at 0 hit points is passed over", []chainRow{{id: "x", kind: eventWildShapeEnded, enc: enc, ev: actionEvent{Reason: endedAtZero}}, under}, "act"},
		{"a wild shape that ended by the player's hand is an action to undo", []chainRow{{id: "x", kind: eventWildShapeEnded, enc: enc, ev: actionEvent{Reason: endedByLeaving}}, under}, "x"},
		{"a wild shape with a broken payload closes the chain", []chainRow{{id: "x", kind: eventWildShapeEnded, enc: enc, raw: badJSON}, under}, ""},
		{"a familiar sight that started is an action to undo", []chainRow{{id: "x", kind: eventFamiliarSight, enc: enc, ev: actionEvent{Sight: "start"}}, under}, "x"},
		{"a familiar sight that ended closes the chain", []chainRow{{id: "x", kind: eventFamiliarSight, enc: enc, ev: actionEvent{Sight: "end"}}, under}, ""},
		{"a familiar sight with a broken payload closes the chain", []chainRow{{id: "x", kind: eventFamiliarSight, enc: enc, raw: badJSON}, under}, ""},
		{"a held action closes the chain", []chainRow{{id: "x", kind: eventReactionUsed, enc: enc, ev: actionEvent{Reaction: &reactionEvent{Hold: true}}}, under}, ""},
		{"a trap fall that waited for a Feather Fall closes the chain", []chainRow{{id: "x", kind: eventTrapTriggered, enc: enc, ev: actionEvent{Trap: &trapFireEvent{Held: true}}}, under}, ""},
		{"a trap firing of this combat is an action to undo", []chainRow{{id: "x", kind: eventTrapTriggered, enc: enc, ev: actionEvent{Trap: &trapFireEvent{}}}, under}, "x"},
		{"a trap firing of another combat is passed over", []chainRow{{id: "x", kind: eventTrapTriggered, enc: "other", ev: actionEvent{Trap: &trapFireEvent{}}}, under}, "act"},
		{"a Revivify cast closes the chain", []chainRow{{id: "x", kind: eventSpellCast, enc: enc, ev: actionEvent{FxKind: rules.SpellKindRevive}}, under}, ""},
		{"another cast is an action to undo", []chainRow{{id: "x", kind: eventSpellCast, enc: enc}, under}, "x"},
		{"the decision on a hide attempt closes the chain", []chainRow{{id: "x", kind: eventHideResolved, enc: enc, ev: actionEvent{Contest: &contestEvent{Line: "hide_found"}}}, under}, ""},
		{"a hide resolved with no contest closes the chain", []chainRow{{id: "x", kind: eventHideResolved, enc: enc}, under}, ""},
		{"the reveal of a hider is an action to undo", []chainRow{{id: "x", kind: eventHideResolved, enc: enc, ev: actionEvent{Contest: &contestEvent{Line: contestLineHideRevealed}}}, under}, "x"},
		{"a move that knows where it came from is an action to undo", []chainRow{{id: "x", kind: eventCombatantMoved, enc: enc, ev: actionEvent{From: &moveState{Placed: true}}}, under}, "x"},
		{"a move that does not say where it came from closes the chain", []chainRow{{id: "x", kind: eventCombatantMoved, enc: enc}, under}, ""},
		{"a move with a broken payload closes the chain", []chainRow{{id: "x", kind: eventCombatantMoved, enc: enc, raw: badJSON}, under}, ""},
		{"an undoable kind of another combat closes the chain", []chainRow{{id: "x", kind: eventAttackRolled, enc: "other"}, under}, ""},
		{"an undoable kind with no combat closes the chain", []chainRow{{id: "x", kind: eventAttackRolled}, under}, ""},
		{"a kind an undo cannot take back closes the chain", []chainRow{{id: "x", kind: eventTurnEnded, enc: enc}, under}, ""},
		{"an undo and its target are passed over, with the older parts it took", []chainRow{
			{id: "u", kind: eventActionUndone, enc: enc, ev: actionEvent{Undone: "x", UndoneAlso: []string{"y"}}},
			{id: "x", kind: eventDamageRolled, enc: enc},
			{id: "y", kind: eventDamageRolled, enc: enc},
			under,
		}, "act"},
		{"an undo with a broken payload takes nothing back", []chainRow{{id: "u", kind: eventActionUndone, enc: enc, raw: badJSON}, under}, "act"},
		{"a broken payload of an undoable action is still taken", []chainRow{{id: "x", kind: eventAttackRolled, enc: enc, raw: badJSON}, under}, "x"},
		{"nothing in the list", nil, ""},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			got, ok := lastAction(chainRows(t, tt.rows...), enc)
			switch {
			case tt.want == "" && ok:
				t.Errorf("lastAction() = %q, want none", got.ID)
			case tt.want != "" && (!ok || got.ID != tt.want):
				t.Errorf("lastAction() = %q (%v), want %q", got.ID, ok, tt.want)
			}
		})
	}
}

// TestFiringBeforeListsTheOlderPartsOfAFiring: a firing written as several events
// is read back newest first, ending with the event that holds the firing itself.
func TestFiringBeforeListsTheOlderPartsOfAFiring(t *testing.T) {
	t.Parallel()
	const enc = "enc"
	part := actionEvent{Trap: &trapFireEvent{Part: true}}
	head := actionEvent{Trap: &trapFireEvent{}}
	tests := []struct {
		name string
		rows []chainRow // newest first; the first is the last action
		want []string
	}{
		{"a firing that is not a part", []chainRow{{id: "a", kind: eventTrapTriggered, enc: enc, ev: head}, {id: "b", kind: eventTrapTriggered, enc: enc, ev: part}}, nil},
		{"two parts and the head", []chainRow{
			{id: "a", kind: eventTrapTriggered, enc: enc, ev: part},
			{id: "b", kind: eventTrapTriggered, enc: enc, ev: part},
			{id: "c", kind: eventTrapTriggered, enc: enc, ev: head},
			{id: "d", kind: eventTrapTriggered, enc: enc, ev: head},
		}, []string{"b", "c"}},
		{"a part with another event under it", []chainRow{
			{id: "a", kind: eventTrapTriggered, enc: enc, ev: part}, {id: "b", kind: eventAttackRolled, enc: enc, ev: head},
		}, nil},
		{"a part whose older event has no trap", []chainRow{
			{id: "a", kind: eventTrapTriggered, enc: enc, ev: part}, {id: "b", kind: eventTrapTriggered, enc: enc},
		}, nil},
		{"a part whose older event is broken", []chainRow{
			{id: "a", kind: eventTrapTriggered, enc: enc, ev: part}, {id: "b", kind: eventTrapTriggered, enc: enc, raw: []byte("{")},
		}, nil},
		{"a last action that has no trap", []chainRow{{id: "a", kind: eventTrapTriggered, enc: enc}}, nil},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			recent := chainRows(t, tt.rows...)
			var ids []string
			for _, e := range firingBefore(recent, recent[0]) {
				ids = append(ids, e.ID)
			}
			if !slices.Equal(ids, tt.want) {
				t.Errorf("firingBefore() = %v, want %v", ids, tt.want)
			}
		})
	}
	// A last event the list does not hold has no firing to complete.
	recent := chainRows(t, chainRow{id: "a", kind: eventTrapTriggered, enc: enc, ev: part})
	if got := firingBefore(recent, playdb.ListRecentSessionEventsRow{ID: "gone", Payload: recent[0].Payload}); got != nil {
		t.Errorf("firingBefore(a row outside the list) = %v, want none", got)
	}
}
