package play

import (
	"encoding/json"
	"fmt"
	"slices"
	"testing"
	"time"
)

// budgetIDs are n distinct UUIDs, the size an ID takes in a payload.
func budgetIDs(prefix, n int) []string {
	out := make([]string, n)
	for i := range out {
		out[i] = fmt.Sprintf("%08x-0000-4000-8000-%012x", prefix, i)
	}
	return out
}

// withHelpers is a cast's event with each undo helper list n long (0: none).
func withHelpers(reveal, followLines, ended, follow, marks, windows int) actionEvent {
	ev := actionEvent{Round: 2, Actor: budgetIDs(9, 1)[0], Target: budgetIDs(9, 2)[1], Key: "spell:fireball", Revealed: budgetIDs(8, 2)}
	if reveal > 0 {
		ev.RevealLines = budgetIDs(1, reveal)
	}
	if followLines > 0 {
		ev.FollowUpLines = budgetIDs(2, followLines)
	}
	if ended > 0 {
		ev.EndedLines = budgetIDs(3, ended)
	}
	targets := budgetIDs(5, follow)
	for i, id := range budgetIDs(4, follow) {
		ev.FollowUps = append(ev.FollowUps, effectRef{ID: id, Target: targets[i]})
	}
	for _, id := range budgetIDs(6, marks) {
		ev.MarksBefore = append(ev.MarksBefore, markState{ID: id, Source: ev.Actor, Target: ev.Target, Round: 1, CreatedAt: time.Date(2026, 10, 16, 12, 0, 0, 0, time.UTC)})
	}
	if windows > 0 {
		ev.ClosedWindows = budgetIDs(7, windows)
	}
	return ev
}

// encodedWithout is the event's encoding with the named undo helpers dropped.
func encodedWithout(t *testing.T, ev actionEvent, names []string) []byte {
	t.Helper()
	for _, h := range undoHelpers {
		if slices.Contains(names, h.name) {
			h.drop(&ev)
		}
	}
	body, err := json.Marshal(ev)
	if err != nil {
		t.Fatalf("encode the event: %v", err)
	}
	return body
}

// TestTrimUndoHelpers: an event over the payload budget drops its undo helpers in the
// fixed order (the line lists, then follow_ups, marks_before and closed_windows), only the
// ones it has and only as many as it takes to fit, and keeps the rest of the event as it
// was; an event under the budget is written whole.
func TestTrimUndoHelpers(t *testing.T) {
	t.Parallel()
	order := []string{"reveal_lines", "follow_up_lines", "ended_lines", "follow_ups", "marks_before", "closed_windows"}
	var names []string
	for _, h := range undoHelpers {
		names = append(names, h.name)
	}
	if !slices.Equal(names, order) {
		t.Fatalf("the undo helpers are dropped in the order %v, want %v", names, order)
	}

	// How many entries make each kind of list over the budget alone: an ID takes 39 bytes,
	// an effectRef about 94 and a markState about 190.
	ids, refs, marks := undoHelpersBudget/39+10, undoHelpersBudget/94+10, undoHelpersBudget/190+10
	for _, tc := range []struct {
		name string
		ev   actionEvent
		want []string // nil: worked out as the shortest prefix of what the event has that fits
	}{
		{"under the budget", withHelpers(2, 1, 1, 1, 1, 1), []string{}},
		{"every list over the budget alone", withHelpers(ids, ids, ids, refs, marks, ids), order},
		{"the line lists are enough", withHelpers(ids/3, ids/3, ids/3, 2, 1, 1), nil},
		{"only the last helper", withHelpers(0, 0, 0, 0, 0, ids), []string{"closed_windows"}},
		{"the lists that put rows back", withHelpers(0, 0, 0, refs, marks, ids), []string{"follow_ups", "marks_before", "closed_windows"}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			body, dropped, err := trimUndoHelpers(tc.ev)
			if err != nil {
				t.Fatalf("trimUndoHelpers() error = %v", err)
			}
			if len(body) > undoHelpersBudget {
				t.Fatalf("the trimmed event takes %d bytes, over the budget of %d", len(body), undoHelpersBudget)
			}
			if !slices.Equal(body, encodedWithout(t, tc.ev, dropped)) {
				t.Error("the trimmed event is not the event without the dropped helpers")
			}
			if tc.want != nil && !slices.Equal(dropped, tc.want) {
				t.Errorf("dropped %v, want %v", dropped, tc.want)
			}
			// In order: what was dropped is the first helpers the event has, and one fewer
			// would not fit.
			var had []string
			for _, h := range undoHelpers {
				if probe := tc.ev; h.drop(&probe) {
					had = append(had, h.name)
				}
			}
			if !slices.Equal(dropped, had[:len(dropped)]) {
				t.Errorf("dropped %v, want the first ones of %v in that order", dropped, had)
			}
			if n := len(dropped); n > 0 && len(encodedWithout(t, tc.ev, dropped[:n-1])) <= undoHelpersBudget {
				t.Errorf("dropped %v, but %v already fit", dropped, dropped[:n-1])
			}
			var back actionEvent
			if err := json.Unmarshal(body, &back); err != nil {
				t.Fatalf("decode the trimmed event: %v", err)
			}
			if back.Actor != tc.ev.Actor || back.Key != tc.ev.Key || !slices.Equal(back.Revealed, tc.ev.Revealed) {
				t.Errorf("the trimmed event lost what is not an undo helper: %+v", back)
			}
		})
	}
}
