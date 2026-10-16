package play

import (
	"encoding/json"
	"errors"
	"fmt"
	"regexp"
	"slices"
	"strings"
	"testing"

	"connectrpc.com/connect"
	"google.golang.org/protobuf/encoding/protojson"

	playv1 "github.com/PuraFome/meuRPG/backend/gen/meurpg/play/v1"
)

// The characterization of MoveCombatant. These tests freeze what the call does
// today, branch by branch, before it is split into smaller functions: the square,
// the movement used, the running start, the cover mark, the flags of the answer,
// the exact payload of the logged event and the code, the reason and the message
// of every refusal. Where the behavior looks odd it is frozen as it is, with a
// comment saying so; a change here is a change of behavior, not of structure.

// moveRow is what the database keeps of a combatant's move.
type moveRow struct {
	Placed          bool
	Col, Row        int32
	UsedFt, UsedDft int32
	LastDft         int32
	CoverMark       string
}

func (r moveRow) String() string {
	if !r.Placed {
		return fmt.Sprintf("(off the map), %d ft, %d dft, run %d, cover %q", r.UsedFt, r.UsedDft, r.LastDft, r.CoverMark)
	}
	return fmt.Sprintf("(%d, %d), %d ft, %d dft, run %d, cover %q", r.Col, r.Row, r.UsedFt, r.UsedDft, r.LastDft, r.CoverMark)
}

// moveRowOf reads the move columns of a combatant, as the master sees its label.
func (a *armed) moveRowOf(t *testing.T, label string) moveRow {
	t.Helper()
	var r moveRow
	var col, row *int32
	err := a.h.pool.QueryRow(t.Context(),
		`SELECT grid_col, grid_row, movement_used_ft, movement_used_dft, last_move_dft, cover_mark FROM combatants WHERE id = $1`, a.id(t, label),
	).Scan(&col, &row, &r.UsedFt, &r.UsedDft, &r.LastDft, &r.CoverMark)
	if err != nil {
		t.Fatalf("read the move columns of %s: %v", label, err)
	}
	if col != nil && row != nil {
		r.Placed, r.Col, r.Row = true, *col, *row
	}
	return r
}

// moveKindCount is how many events of a kind the session has.
func (a *armed) moveKindCount(t *testing.T, kind string) int {
	t.Helper()
	var n int
	if err := a.h.pool.QueryRow(t.Context(), `SELECT count(*) FROM session_events WHERE kind = $1`, kind).Scan(&n); err != nil {
		t.Fatalf("count %s events: %v", kind, err)
	}
	return n
}

var offerIDs = regexp.MustCompile(`"move_id": "[0-9a-f-]{36}"`)

// lastMovePayload is the payload of the newest event of a kind, as the database
// prints it, with the ids of the combat's combatants and users replaced by their
// labels so that the text is the same in every run.
func (a *armed) lastMovePayload(t *testing.T, kind string) string {
	t.Helper()
	var payload string
	if err := a.h.pool.QueryRow(t.Context(), `SELECT payload::STRING FROM session_events WHERE kind = $1 ORDER BY seq DESC LIMIT 1`, kind).Scan(&payload); err != nil {
		t.Fatalf("read the newest %s event: %v", kind, err)
	}
	for _, c := range a.get(t, a.master).GetCombatants() {
		payload = strings.ReplaceAll(payload, c.GetId(), "<"+c.GetLabel()+">")
	}
	payload = offerIDs.ReplaceAllString(payload, `"move_id": "<offers>"`) // a new UUID in every run
	for name, u := range map[string]*user{"master": a.master, "caio": a.caio, "ana": a.ana, "bia": a.bia} {
		payload = strings.ReplaceAll(payload, u.id, "<user:"+name+">")
	}
	return payload
}

// frozenMove is MoveCombatant as u, with the request edited by edit.
func (a *armed) frozenMove(t *testing.T, u *user, label string, col, row int32, edit ...func(*playv1.MoveCombatantRequest)) (*playv1.MoveCombatantResponse, error) {
	t.Helper()
	req := &playv1.MoveCombatantRequest{
		CampaignId: a.campaignID, EncounterId: a.get(t, a.master).GetId(), CombatantId: a.id(t, label), IdempotencyKey: newKey(), Col: col, Row: row,
	}
	for _, e := range edit {
		e(req)
	}
	res, err := u.combat.MoveCombatant(t.Context(), connect.NewRequest(req))
	if err != nil {
		return nil, err
	}
	return res.Msg, nil
}

// refusal is the whole of a refused call: the code, the message and the details
// (EncounterBlocked or ContestBlocked, as JSON), which are what the app reads.
type refusal struct {
	code   connect.Code
	msg    string
	detail string // "" when the error carries none
}

// refusalOf reads an error the way the app does.
func refusalOf(t *testing.T, err error) refusal {
	t.Helper()
	var cerr *connect.Error
	if !errors.As(err, &cerr) {
		t.Fatalf("error %v is not a connect error", err)
	}
	got := refusal{code: cerr.Code(), msg: cerr.Message()}
	for _, d := range cerr.Details() {
		msg, derr := d.Value()
		if derr != nil {
			t.Fatalf("read the error detail: %v", derr)
		}
		raw, merr := protojson.Marshal(msg)
		if merr != nil {
			t.Fatalf("marshal the error detail: %v", merr)
		}
		var canon any
		if jerr := json.Unmarshal(raw, &canon); jerr != nil { // protojson adds random spaces: canonical text, sorted keys
			t.Fatalf("unmarshal the error detail: %v", jerr)
		}
		text, _ := json.Marshal(canon)
		got.detail += d.Type() + string(text)
	}
	return got
}

// blockedBy is the refusal of an EncounterBlocked with the given fields of its JSON.
func blockedBy(reason, msg string, fields ...string) refusal {
	return refusal{code: connect.CodeFailedPrecondition, msg: msg, detail: "meurpg.play.v1.EncounterBlocked" + jsonFields(append([]string{`"reason":"ENCOUNTER_BLOCKED_REASON_` + reason + `"`}, fields...))}
}

// contestBlockedBy is the same for a ContestBlocked.
func contestBlockedBy(reason, msg string, fields ...string) refusal {
	return refusal{code: connect.CodeFailedPrecondition, msg: msg, detail: "meurpg.play.v1.ContestBlocked" + jsonFields(append([]string{`"reason":"CONTEST_BLOCKED_REASON_` + reason + `"`}, fields...))}
}

// jsonFields is the object protojson prints for fields in the order given, which
// is the one canonical text uses: sorted by name.
func jsonFields(fields []string) string {
	sorted := slices.Clone(fields)
	slices.Sort(sorted)
	return "{" + strings.Join(sorted, ",") + "}"
}

// invalid is a refusal that carries only a code and a message.
func refuse(code connect.Code, msg string) refusal { return refusal{code: code, msg: msg} }

func (a *armed) frozenWantRefusal(t *testing.T, err error, want refusal) {
	t.Helper()
	if err == nil {
		t.Fatalf("the call worked, want %v %q %s", want.code, want.msg, want.detail)
	}
	if got := refusalOf(t, err); got != want {
		t.Errorf("refusal =\n  %v %q\n  %s\nwant\n  %v %q\n  %s", got.code, got.msg, got.detail, want.code, want.msg, want.detail)
	}
}

// moveWant is what a move that worked leaves behind.
type moveWant struct {
	col, row         int32  // where the mover ends
	usedDft, lastDft int32  // the movement walked, the running start
	cover            string // the mover's cover mark, "" when it does not matter
	stopped, offered bool   // the answer's stopped_early and provoked
	locked           bool   // the answer's locked_door
	event            string // the payload of the combatant_moved event
	events           int    // the combatant_moved events written (1 by default)
}

// frozenWantMoved checks the answer of a move, the mover's row and the logged event.
func (a *armed) frozenWantMoved(t *testing.T, res *playv1.MoveCombatantResponse, label string, before int, want moveWant) {
	t.Helper()
	if res.GetStoppedEarly() != want.stopped || res.GetProvoked() != want.offered || res.GetLockedDoor() != want.locked {
		t.Errorf("answer = stopped %v provoked %v locked %v; want %v %v %v", res.GetStoppedEarly(), res.GetProvoked(), res.GetLockedDoor(), want.stopped, want.offered, want.locked)
	}
	row := a.moveRowOf(t, label)
	if !row.Placed || row.Col != want.col || row.Row != want.row || row.UsedDft != want.usedDft || row.LastDft != want.lastDft || row.UsedFt != want.usedDft/10 {
		t.Errorf("%s = %v; want (%d, %d), %d dft, run %d", label, row, want.col, want.row, want.usedDft, want.lastDft)
	}
	if want.cover != "" && row.CoverMark != want.cover {
		t.Errorf("%s cover mark = %q, want %q", label, row.CoverMark, want.cover)
	}
	events := want.events
	if events == 0 {
		events = 1
	}
	if got := a.moveKindCount(t, "combatant_moved") - before; got != events {
		t.Errorf("combatant_moved events written = %d, want %d", got, events)
	}
	if got := a.lastMovePayload(t, "combatant_moved"); got != want.event {
		t.Errorf("the combatant_moved payload =\n  %s\nwant\n  %s", got, want.event)
	}
}

// frozenCheck moves label as u and checks the whole result: a refusal changes
// nothing and writes nothing, and a move leaves what want says.
func (a *armed) frozenCheck(t *testing.T, u *user, label string, col, row int32, edit []func(*playv1.MoveCombatantRequest), refused *refusal, want moveWant) {
	t.Helper()
	before, moved, events := a.moveRowOf(t, label), a.moveKindCount(t, "combatant_moved"), a.events(t)
	res, err := a.frozenMove(t, u, label, col, row, edit...)
	if refused != nil {
		a.frozenWantRefusal(t, err, *refused)
		if got := a.moveRowOf(t, label); got != before || a.events(t) != events {
			t.Errorf("a refused move changed %s: %v, %d events; was %v, %d", label, got, a.events(t), before, events)
		}
		return
	}
	if err != nil {
		t.Fatalf("MoveCombatant(%s to %d,%d) error = %v", label, col, row, err)
	}
	a.frozenWantMoved(t, res, label, moved, want)
}
