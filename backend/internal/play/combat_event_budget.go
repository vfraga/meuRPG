package play

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
)

// session_events refuses a payload whose text in the database is over 16 KiB (migration
// 00200). Some lists an event keeps only so that the master's undo puts back more than
// the action itself needs: the lines it takes back with the action, what an effect that
// ended left behind, the Hunter's Marks a cast took off and the windows of a discarded
// hit. An action must never fail because of them, so an event over undoHelpersBudget
// drops them, one at a time, until it fits, and its undo then does what it did without
// them (the lines stay and stop the undo chain, the lethargy stays, the first target
// stays unmarked, the hit goes on to its roll).

// undoHelpersBudget is how many bytes of Go's encoding an event may take with its undo
// helpers. The database checks its own text of the JSON, which writes a space after each
// colon and comma (about 8% more for these payloads, measured on a cast's event), so this
// leaves the same share of the 16 KiB free as eventPayloadBudget leaves of the 4 KiB the
// table allowed first. It is not eventPayloadBudget: an event over 3,500 bytes is common
// and fits (a concentration that ends on four creatures keeps about 3,400 bytes of rows to
// put back), and dropping its helpers there would undo less than the table allows.
const undoHelpersBudget = 14000

// undoHelpers are those lists, in the order an event over the budget drops them: the
// lists of line IDs first, which only tidy the log and the undo chain, then the lists
// that put rows back.
var undoHelpers = []struct {
	name string
	drop func(*actionEvent) bool
}{
	{"reveal_lines", func(ev *actionEvent) bool { return dropList(&ev.RevealLines) }},
	{"follow_up_lines", func(ev *actionEvent) bool { return dropList(&ev.FollowUpLines) }},
	{"ended_lines", func(ev *actionEvent) bool { return dropList(&ev.EndedLines) }},
	{"follow_ups", func(ev *actionEvent) bool { return dropList(&ev.FollowUps) }},
	{"marks_before", func(ev *actionEvent) bool { return dropList(&ev.MarksBefore) }},
	{"closed_windows", func(ev *actionEvent) bool { return dropList(&ev.ClosedWindows) }},
}

// dropList empties a list and says whether it had anything.
func dropList[T any](l *[]T) bool {
	had := len(*l) > 0
	*l = nil
	return had
}

// trimUndoHelpers encodes the event and, while the encoding is over undoHelpersBudget,
// drops the next undo helper the event has (undoHelpers' order) and encodes it again. It
// returns the last encoding, which is over the budget only when the event is that large
// without any helper, and the names of the helpers it dropped.
func trimUndoHelpers(ev actionEvent) ([]byte, []string, error) {
	body, err := json.Marshal(ev)
	if err != nil {
		return nil, nil, fmt.Errorf("encode the event payload: %w", err)
	}
	var dropped []string
	for _, h := range undoHelpers {
		if len(body) <= undoHelpersBudget {
			break
		}
		if !h.drop(&ev) {
			continue
		}
		dropped = append(dropped, h.name)
		if body, err = json.Marshal(ev); err != nil {
			return nil, nil, fmt.Errorf("encode the event payload: %w", err)
		}
	}
	return body, dropped, nil
}

// fitBudget is the encoding of an event over undoHelpersBudget with the undo helpers it
// had to drop (trimUndoHelpers), and a warning that names them and the event's kind: IDs
// never go in the log.
func (c *combatTx) fitBudget(ctx context.Context, kind string, ev actionEvent) ([]byte, error) {
	body, dropped, err := trimUndoHelpers(ev)
	if err != nil || len(dropped) == 0 {
		return body, err
	}
	logger := slog.Default()
	if c.svc != nil {
		logger = c.svc.logger
	}
	logger.WarnContext(ctx, "play: an event over the payload budget dropped its undo helpers",
		"event_kind", kind, "dropped_fields", dropped, "payload_bytes", len(body))
	return body, nil
}
