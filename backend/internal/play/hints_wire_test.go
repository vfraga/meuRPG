package play

import (
	"testing"
	"time"

	"connectrpc.com/connect"
	"github.com/jackc/pgx/v5"
	"google.golang.org/protobuf/proto"

	playv1 "github.com/PuraFome/meuRPG/backend/gen/meurpg/play/v1"
	"github.com/PuraFome/meuRPG/backend/internal/play/live"
	"github.com/PuraFome/meuRPG/backend/internal/play/playdb"
)

// The stream carries the session's hint number: on Ready, on every heartbeat (read from the database), and on the events a
// counted write sends.
func TestWatchStampsReadyHeartbeatAndEventsWithTheHintNumber(t *testing.T) {
	t.Parallel()
	h := newHarness(t, LiveConfig{Heartbeat: 20 * time.Millisecond})
	master := h.newUser("Mestre")
	campaign := h.newCampaign(master, "Mirathel")
	session := master.start(t, campaign).GetGameSession()
	setSeq := func(n int) {
		t.Helper()
		if _, err := h.pool.Exec(t.Context(), `UPDATE game_sessions SET hint_seq = $2 WHERE id = $1`, session.GetId(), n); err != nil {
			t.Fatal(err)
		}
	}
	setSeq(7)

	stream := master.watch(t, campaign)
	ev := stream.next(t)
	if ev.GetReady() == nil || ev.GetSessionSeq() != 7 || ev.GetReady().GetSessionSeq() != 7 {
		t.Fatalf("first event = %v, want ready with the number 7 on both", ev)
	}

	// The number moves in the database; a heartbeat carries the new one (no sleep: the next heartbeats are read until it does).
	setSeq(9)
	for {
		ev := stream.next(t)
		hb := ev.GetHeartbeat()
		if hb == nil {
			t.Fatalf("event = %v, want only heartbeats", ev)
		}
		if hb.GetSessionSeq() != ev.GetSessionSeq() {
			t.Fatalf("heartbeat number %d, stamp %d: want the same", hb.GetSessionSeq(), ev.GetSessionSeq())
		}
		if hb.GetSessionSeq() == 9 {
			break
		}
		if hb.GetSessionSeq() != 7 {
			t.Fatalf("heartbeat number = %d, want 7 or 9", hb.GetSessionSeq())
		}
	}

	// A counted write stamps its event with the number it left; an event sent without a write has none.
	err := h.svc.tx(t.Context(), session.GetId(), func(_ pgx.Tx, hints *hints) error {
		hints.tell(campaign, live.Event{
			Audience: live.Audience{Master: true},
			Message:  &playv1.WatchGameSessionResponse{Event: &playv1.WatchGameSessionResponse_ContentChanged_{ContentChanged: &playv1.WatchGameSessionResponse_ContentChanged{}}},
		})
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	h.svc.PublishContentChanged(campaign)
	var stamped, plain []int64
	for len(stamped)+len(plain) < 2 {
		ev := stream.next(t)
		switch {
		case ev.GetHeartbeat() != nil:
		case len(stamped) == 0:
			stamped = append(stamped, ev.GetSessionSeq())
		default:
			plain = append(plain, ev.GetSessionSeq())
		}
	}
	if stamped[0] != 10 || plain[0] != 0 {
		t.Errorf("the counted event carried %d (want 10) and the uncounted one %d (want 0)", stamped[0], plain[0])
	}
}

// ListCombatLog with since_seq: the new and changed lines only, the highest number to ask from next, and the whole log again
// after an undo.
func TestListCombatLogSinceSeq(t *testing.T) {
	t.Parallel()
	a := newArmed(t)
	e := a.threeAndAGoblin(t)
	a.h.roller.queue(15, 3)
	hit := a.mustAttack(t, a.caio, e, "Toren", battleaxe, "Goblin", inAppRoll)

	first := a.log(t, a.master, e)
	entries := func(l *playv1.ListCombatLogResponse) (out []*playv1.CombatLogEntry) {
		for _, r := range l.GetRounds() {
			out = append(out, r.GetEntries()...)
		}
		return out
	}
	if !first.GetFull() || first.GetLastSeq() == 0 || len(entries(first)) == 0 {
		t.Fatalf("the whole log: full = %v, last_seq = %d, %d entries; want full, a number and entries", first.GetFull(), first.GetLastSeq(), len(entries(first)))
	}
	for _, en := range entries(first) {
		if en.GetSeq() == 0 || en.GetSeq() > first.GetLastSeq() {
			t.Errorf("entry %s has seq %d, want 1 to %d", en.GetId(), en.GetSeq(), first.GetLastSeq())
		}
	}
	since := func(u *user, n int64) *playv1.ListCombatLogResponse {
		t.Helper()
		res, err := u.combat.ListCombatLog(t.Context(), connect.NewRequest(&playv1.ListCombatLogRequest{CampaignId: a.campaignID, EncounterId: e.GetId(), SinceSeq: n}))
		if err != nil {
			t.Fatalf("ListCombatLog(since %d) error = %v", n, err)
		}
		return res.Msg
	}

	// Nothing new: no entries, the same number, not full. (The entry the master can undo is always sent.)
	playerFirst := a.log(t, a.caio, e)
	again := since(a.caio, playerFirst.GetLastSeq())
	if len(entries(again)) != 0 || again.GetFull() || again.GetLastSeq() != playerFirst.GetLastSeq() || playerFirst.GetLastSeq() == 0 {
		t.Errorf("nothing new, as a player: %d entries, full = %v, last_seq = %d; want 0, false, %d (not 0)", len(entries(again)), again.GetFull(), again.GetLastSeq(), playerFirst.GetLastSeq())
	}
	// As the master, the undoable entry is always sent: its flag moves with the log's end.
	masterAgain := since(a.master, first.GetLastSeq())
	if got := entries(masterAgain); len(got) != 1 || !got[0].GetUndoable() || masterAgain.GetFull() {
		t.Errorf("nothing new, as the master: entries = %v, full = %v; want only the undoable entry", got, masterAgain.GetFull())
	}

	// The damage joins the attack's entry: it comes again with a higher number, alone.
	a.mustDamage(t, a.caio, e, hit.GetPendingDamage().GetId(), inAppDamage)
	delta := since(a.caio, playerFirst.GetLastSeq())
	got := entries(delta)
	if delta.GetFull() || len(got) != 1 || got[0].GetKind() != playv1.CombatLogKind_COMBAT_LOG_KIND_ATTACK || got[0].GetSeq() != 0 || delta.GetLastSeq() <= playerFirst.GetLastSeq() {
		t.Fatalf("after the damage: full = %v, entries = %v, last_seq = %d; want only the attack's entry with no seq of its own (a player's entries carry none) and a higher last_seq", delta.GetFull(), got, delta.GetLastSeq())
	}
	if all := a.log(t, a.caio, e); len(entries(all)) < 2 {
		t.Errorf("the whole log has %d entries, want the delta to be a part of more", len(entries(all)))
	}

	// An undo takes lines back: the answer is the whole log again.
	l := a.log(t, a.master, e)
	if err := a.undo(t, a.master, e, l.GetUndoableEventId()); err != nil {
		t.Fatal(err)
	}
	after := since(a.master, l.GetLastSeq())
	if !after.GetFull() || after.GetLastSeq() <= l.GetLastSeq() {
		t.Errorf("after an undo: full = %v, last_seq = %d (was %d); want full and a higher number", after.GetFull(), after.GetLastSeq(), l.GetLastSeq())
	}
	if len(entries(after)) != len(entries(a.log(t, a.master, e))) {
		t.Errorf("the full answer has %d entries, the whole log %d", len(entries(after)), len(entries(a.log(t, a.master, e))))
	}
}

// An undo tells the streams the log was rewritten (full); an ordinary change does not.
func TestCombatLogChangedIsFullAfterAnUndo(t *testing.T) {
	t.Parallel()
	a := newArmed(t)
	e := a.threeAndAGoblin(t)
	stream := a.master.watch(t, a.campaignID)
	stream.ready(t)
	playerStream := a.caio.watch(t, a.campaignID)
	playerStream.ready(t)
	a.h.roller.queue(15, 3)
	a.mustAttack(t, a.caio, e, "Toren", battleaxe, "Goblin", inAppRoll)
	logChange := func() *playv1.WatchGameSessionResponse_CombatLogChanged {
		t.Helper()
		for {
			if c := stream.next(t).GetCombatLogChanged(); c != nil {
				return c
			}
		}
	}
	playerLogChange := func() *playv1.WatchGameSessionResponse_CombatLogChanged {
		t.Helper()
		for {
			if c := playerStream.next(t).GetCombatLogChanged(); c != nil {
				return c
			}
		}
	}
	if c := logChange(); c.GetFull() {
		t.Error("an attack told the streams the log was rewritten")
	}
	ordinary := playerLogChange()
	if err := a.undo(t, a.master, e, a.log(t, a.master, e).GetUndoableEventId()); err != nil {
		t.Fatal(err)
	}
	if c := logChange(); !c.GetFull() {
		t.Error("an undo did not tell the master's stream the log was rewritten")
	}
	if c := playerLogChange(); !proto.Equal(c, ordinary) {
		t.Errorf("a player heard %v after the undo, %v after the attack; want the same", c, ordinary)
	}
}

// Writes that change how the entries the app holds are seen, without a new event on those entries, make the answer the whole log.
func TestListCombatLogSinceSeqIsFullWhenTheViewMayHaveChanged(t *testing.T) {
	t.Parallel()
	a := newArmed(t)
	e := a.threeAndAGoblin(t)
	a.h.roller.queue(15, 3)
	a.mustAttack(t, a.caio, e, "Toren", battleaxe, "Goblin", inAppRoll)
	player := func() *playv1.ListCombatLogResponse { return a.log(t, a.caio, e) }
	sinceAfter := func(base *playv1.ListCombatLogResponse, write func()) *playv1.ListCombatLogResponse {
		t.Helper()
		write()
		res, err := a.caio.combat.ListCombatLog(t.Context(), connect.NewRequest(&playv1.ListCombatLogRequest{CampaignId: a.campaignID, EncounterId: e.GetId(), SinceSeq: base.GetLastSeq()}))
		if err != nil {
			t.Fatal(err)
		}
		return res.Msg
	}
	cases := []struct {
		name  string
		write func()
		full  *bool // nil: either, as long as the answer rebuilds the whole log
	}{
		{"a master-only hit point correction adds no change to old lines", func() {
			if _, err := a.adjustHP(t, e, "Goblin", damageHP(1)); err != nil {
				t.Fatal(err)
			}
		}, new(false)},
		{"the Goblin is hidden", func() { hideAgain(t, a, e, "Goblin") }, new(true)},
		// The player's number fell below the Goblin's lines when they went, so the line comes as a new one: no need for the whole log.
		{"the Goblin is shown", func() { showNPC(t, a, e, "Goblin") }, nil},
		{"a roll mode request is answered", func() {
			req, err := a.rmRequestRaw(t, a.caio, e, func(r *playv1.RequestRollModeRequest) { r.AttackKey = battleaxe })
			if err != nil {
				t.Fatal(err)
			}
			if err := a.answerMode(t, a.master, e, req.GetRequest().GetId(), playv1.RollMode_ROLL_MODE_ADVANTAGE); err != nil {
				t.Fatal(err)
			}
		}, new(true)},
	}
	for _, tc := range cases {
		base := player()
		got := sinceAfter(base, tc.write)
		if tc.full != nil && got.GetFull() != *tc.full {
			t.Errorf("%s: full = %v, want %v", tc.name, got.GetFull(), *tc.full)
		}
		if held, want := rebuilt(base, got), player(); !sameLines(held, want) {
			t.Errorf("%s: the app's log after the answer has %d lines, the whole log %d", tc.name, len(held), len(entriesOf(want)))
		}
	}
}

// What a player cannot see does not move the player's answer: a master-only write, or an undo of one, leaves the answer to
// the same since_seq as it was (entries, last_seq, full).
func TestListCombatLogSinceSeqOfAPlayerIgnoresWhatTheyCannotSee(t *testing.T) {
	t.Parallel()
	a := newArmed(t)
	e := a.threeAndAGoblin(t)
	a.h.roller.queue(15, 3)
	a.mustAttack(t, a.caio, e, "Toren", battleaxe, "Goblin", inAppRoll)
	base := a.log(t, a.caio, e)
	ask := func() *playv1.ListCombatLogResponse {
		res, err := a.caio.combat.ListCombatLog(t.Context(), connect.NewRequest(&playv1.ListCombatLogRequest{CampaignId: a.campaignID, EncounterId: e.GetId(), SinceSeq: base.GetLastSeq()}))
		if err != nil {
			t.Fatal(err)
		}
		return res.Msg
	}
	before := ask()
	if _, err := a.adjustHP(t, e, "Goblin", damageHP(1)); err != nil { // the master's own line
		t.Fatal(err)
	}
	if after := ask(); !proto.Equal(before, after) {
		t.Errorf("a master-only write changed the player's answer:\n before %v\n after  %v", before, after)
	}
	if err := a.undo(t, a.master, e, a.log(t, a.master, e).GetUndoableEventId()); err != nil { // and its undo
		t.Fatal(err)
	}
	if after := ask(); !proto.Equal(before, after) {
		t.Errorf("a master-only undo changed the player's answer:\n before %v\n after  %v", before, after)
	}
}

// A retry of an undo (the same key) tells the streams the log was rewritten, as the first call did.
func TestCombatLogChangedIsFullOnARetriedUndo(t *testing.T) {
	t.Parallel()
	a := newArmed(t)
	e := a.threeAndAGoblin(t)
	a.h.roller.queue(15, 3)
	a.mustAttack(t, a.caio, e, "Toren", battleaxe, "Goblin", inAppRoll)
	stream := a.master.watch(t, a.campaignID)
	stream.ready(t)
	req := &playv1.UndoLastActionRequest{CampaignId: a.campaignID, EncounterId: e.GetId(), ExpectedEventId: a.log(t, a.master, e).GetUndoableEventId(), IdempotencyKey: newKey()}
	for i := range 2 {
		if _, err := a.master.combat.UndoLastAction(t.Context(), connect.NewRequest(req)); err != nil {
			t.Fatalf("UndoLastAction(call %d) error = %v", i+1, err)
		}
		for {
			if c := stream.next(t).GetCombatLogChanged(); c != nil {
				if !c.GetFull() {
					t.Errorf("call %d: combat_log_changed.full = false, want true", i+1)
				}
				break
			}
		}
	}
}

func entriesOf(l *playv1.ListCombatLogResponse) (out []*playv1.CombatLogEntry) {
	for _, r := range l.GetRounds() {
		out = append(out, r.GetEntries()...)
	}
	return out
}

// rebuilt is what an app holding base has after it applies the answer: all of it when full, else the new entries by id.
func rebuilt(base, answer *playv1.ListCombatLogResponse) map[string]int64 {
	held := map[string]int64{}
	if !answer.GetFull() {
		for _, e := range entriesOf(base) {
			held[e.GetId()] = e.GetSeq()
		}
	}
	for _, e := range entriesOf(answer) {
		held[e.GetId()] = e.GetSeq()
	}
	return held
}

func sameLines(held map[string]int64, whole *playv1.ListCombatLogResponse) bool {
	want := entriesOf(whole)
	if len(held) != len(want) {
		return false
	}
	for _, e := range want {
		if seq, ok := held[e.GetId()]; !ok || seq != e.GetSeq() {
			return false
		}
	}
	return true
}

// A player's answer to a since_seq does not move on a toggle of an NPC that is in no line they hold.
func TestListCombatLogSinceSeqOfAPlayerIgnoresAToggleOfAnNPCTheyNeverSaw(t *testing.T) {
	t.Parallel()
	a := newArmed(t)
	e := a.threeAndAGoblin(t)
	hideAgain(t, a, e, "Goblin") // before any line names it
	base := a.log(t, a.caio, e)
	ask := func() *playv1.ListCombatLogResponse {
		res, err := a.caio.combat.ListCombatLog(t.Context(), connect.NewRequest(&playv1.ListCombatLogRequest{CampaignId: a.campaignID, EncounterId: e.GetId(), SinceSeq: base.GetLastSeq()}))
		if err != nil {
			t.Fatal(err)
		}
		return res.Msg
	}
	before := ask()
	if before.GetFull() || base.GetLastSeq() == 0 {
		t.Fatalf("the starting answer: full = %v, last_seq = %d; want a delta from a held line", before.GetFull(), base.GetLastSeq())
	}
	showNPC(t, a, e, "Goblin")
	if after := ask(); !proto.Equal(before, after) {
		t.Errorf("showing an NPC in no held line changed the answer:\n before %v\n after  %v", before, after)
	}
	hideAgain(t, a, e, "Goblin")
	if after := ask(); !proto.Equal(before, after) {
		t.Errorf("hiding an NPC in no held line changed the answer:\n before %v\n after  %v", before, after)
	}
}

// The synthetic cases of the player's rule: events built by hand, as the database holds them.
func TestSinceSeqOfForAPlayerHearsOnlyWhatTheyMaySee(t *testing.T) {
	t.Parallel()
	player := combatViewer{userID: "ana"}
	row := func(id string, seq int32, kind, payload string) playdb.ListEncounterEventsRow {
		return playdb.ListEncounterEventsRow{ID: id, Seq: seq, Kind: kind, Payload: []byte(payload)}
	}
	held := func(seq int64, actor string) *logEntry { // a line the player holds: seq 5, naming the actor
		return &logEntry{seq: seq, ev: actionEvent{Actor: actor}}
	}
	cs := []playdb.Combatant{{ID: "c-gob", CreatureID: new("cr-1")}}
	events := func(extra ...playdb.ListEncounterEventsRow) []playdb.ListEncounterEventsRow {
		return append([]playdb.ListEncounterEventsRow{
			row("e-seen", 4, eventAttackRolled, `{"combatant_id":"c-toren","round":1}`),
			row("e-fogged", 5, eventAttackRolled, `{"combatant_id":"c-gob","fogged":true,"seen_by":["bia"]}`),
			row("e-secret", 6, eventAttackRolled, `{"combatant_id":"c-gob","secret":true}`),
		}, extra...)
	}
	cases := []struct {
		name    string
		extra   []playdb.ListEncounterEventsRow
		entries []*logEntry
		master  bool
		want    int64 // 0: the whole log
	}{
		{"monsters added hidden", []playdb.ListEncounterEventsRow{row("m1", 7, eventCombatantsAdded, `{"round":1,"monsters":{"count":1,"hidden":true}}`)}, nil, false, 5},
		{"monsters added hidden, for the master", []playdb.ListEncounterEventsRow{row("m1", 7, eventCombatantsAdded, `{"round":1,"monsters":{"count":1,"hidden":true}}`)}, nil, true, 0},
		{"monsters added in the open", []playdb.ListEncounterEventsRow{row("m1", 7, eventCombatantsAdded, `{"round":1,"monsters":{"count":1}}`)}, nil, false, 0},
		{"combatants added, all hidden", []playdb.ListEncounterEventsRow{row("a1", 7, eventCombatantsAdded, `{"added":2,"hidden_added":2}`)}, nil, false, 5},
		{"combatants added, one in the open", []playdb.ListEncounterEventsRow{row("a1", 7, eventCombatantsAdded, `{"added":2,"hidden_added":1}`)}, nil, false, 0},
		{"combatants added by an older server", []playdb.ListEncounterEventsRow{row("a1", 7, eventCombatantsAdded, `{"added":1}`)}, nil, false, 0},
		{"an undo of a fogged action the player never saw", []playdb.ListEncounterEventsRow{row("u1", 7, eventActionUndone, `{"undone_id":"e-fogged"}`)}, nil, false, 5},
		{"the master hears of that undo", []playdb.ListEncounterEventsRow{row("u1", 7, eventActionUndone, `{"undone_id":"e-fogged"}`)}, nil, true, 0},
		{"an undo of a secret action", []playdb.ListEncounterEventsRow{row("u1", 7, eventActionUndone, `{"undone_id":"e-secret"}`)}, nil, false, 5},
		{"an undo of an action the player saw", []playdb.ListEncounterEventsRow{row("u1", 7, eventActionUndone, `{"undone_id":"e-seen"}`)}, nil, false, 0},
		{"a summon with a secret line", []playdb.ListEncounterEventsRow{row("s1", 7, eventCreatureSummoned, `{"combatant_id":"c-gob","secret":true,"creature_ids":["cr-1"]}`)}, []*logEntry{held(4, "c-gob")}, false, 5},
		{"a dismissal of a creature in no held line", []playdb.ListEncounterEventsRow{row("d1", 7, eventCreatureDismissed, `{"creature_ids":["cr-1"]}`)}, []*logEntry{held(4, "c-toren")}, false, 5},
		{"a dismissal of a creature in a held line", []playdb.ListEncounterEventsRow{row("d1", 7, eventCreatureDismissed, `{"creature_ids":["cr-1"]}`)}, []*logEntry{held(4, "c-gob")}, false, 0},
		{"a toggle of a combatant in a master-only line", []playdb.ListEncounterEventsRow{row("h1", 7, eventCombatantHiddenSet, `{"combatant_id":"c-gob"}`)}, []*logEntry{{seq: 4, masterOnly: true, ev: actionEvent{Actor: "c-gob"}}}, false, 5},
		{"a firing that only adds a hidden NPC to an earlier one", []playdb.ListEncounterEventsRow{row("t1", 7, eventTrapTriggered, `{"trap":{"extends_id":"f","caught":[{"target_id":"c-h","hidden":true}]}}`)}, nil, false, 5},
		{"that addition, for the master", []playdb.ListEncounterEventsRow{row("t1", 7, eventTrapTriggered, `{"trap":{"extends_id":"f","caught":[{"target_id":"c-h","hidden":true}]}}`)}, nil, true, 0},
		{"an addition with a creature the player is given", []playdb.ListEncounterEventsRow{row("t1", 7, eventTrapTriggered, `{"trap":{"extends_id":"f","caught":[{"target_id":"c-h","hidden":true},{"target_id":"c-p"}]}}`)}, nil, false, 0},
		{"a toggle of a creature a held firing caught and the player is given", []playdb.ListEncounterEventsRow{row("h1", 7, eventCombatantHiddenSet, `{"combatant_id":"c-gob"}`)}, []*logEntry{{seq: 4, ev: actionEvent{Trap: &trapFireEvent{Caught: []trapCaughtEvent{{Target: "c-gob"}}}}}}, false, 0},
		{"a toggle of a creature hidden when the held firing caught it", []playdb.ListEncounterEventsRow{row("h1", 7, eventCombatantHiddenSet, `{"combatant_id":"c-gob"}`)}, []*logEntry{{seq: 4, ev: actionEvent{Trap: &trapFireEvent{Caught: []trapCaughtEvent{{Target: "c-gob", Hidden: true}}}}}}, false, 5},
		{"a toggle of a creature the held firing caught out of this player's sight", []playdb.ListEncounterEventsRow{row("h1", 7, eventCombatantHiddenSet, `{"combatant_id":"c-gob"}`)}, []*logEntry{{seq: 4, ev: actionEvent{Trap: &trapFireEvent{Caught: []trapCaughtEvent{{Target: "c-gob", Fogged: true, SeenBy: []string{"bia"}}}}}}}, false, 5},
		{"a toggle of a creature the held firing caught in this player's sight", []playdb.ListEncounterEventsRow{row("h1", 7, eventCombatantHiddenSet, `{"combatant_id":"c-gob"}`)}, []*logEntry{{seq: 4, ev: actionEvent{Trap: &trapFireEvent{Caught: []trapCaughtEvent{{Target: "c-gob", Fogged: true, SeenBy: []string{"ana"}}}}}}}, false, 0},
		{"a secret roll mode answer", []playdb.ListEncounterEventsRow{row("r1", 7, eventRollModeAnswered, `{"secret":true}`)}, nil, false, 5},
		{"a roll mode answer the player may see", []playdb.ListEncounterEventsRow{row("r1", 7, eventRollModeAnswered, `{}`)}, nil, false, 0},
		{"an event no one can read", []playdb.ListEncounterEventsRow{row("x1", 7, eventRollModeAnswered, `not json`)}, nil, false, 5},
		{"an unreadable event, for the master", []playdb.ListEncounterEventsRow{row("x1", 7, eventRollModeAnswered, `not json`)}, nil, true, 0},
	}
	for _, c := range cases {
		v := player
		v.master = c.master
		if got := sinceSeqOf(5, v, events(c.extra...), c.entries, cs); got != c.want {
			t.Errorf("%s: since = %d, want %d", c.name, got, c.want)
		}
	}
}

// An entry's number is the highest among the events it shows, and a player's counts the events they hear of: a Secret event or a
// fogged one they did not see that joins a line they hold moves nothing they read (a legendary resistance spent on a save they see).
// The entry's own event always counts (a reactor's player is given the line of a reaction the table is not). The master counts all.
func TestStampSeqsOfAPlayerCountsOnlyTheEventsTheyHearOf(t *testing.T) {
	t.Parallel()
	row := func(id string, seq int32, payload string) playdb.ListEncounterEventsRow {
		return playdb.ListEncounterEventsRow{ID: id, Seq: seq, Kind: eventAttackRolled, Payload: []byte(payload)}
	}
	events := []playdb.ListEncounterEventsRow{
		row("own", 10, `{"combatant_id":"c-a"}`),
		row("secret", 11, `{"combatant_id":"c-a","secret":true}`),
		row("fogged", 12, `{"combatant_id":"c-a","fogged":true,"seen_by":["bia"]}`),
		row("seen-fog", 13, `{"combatant_id":"c-a","fogged":true,"seen_by":["ana"]}`),
		row("later", 14, `{"combatant_id":"c-a"}`),
		row("unreadable", 15, `not json`),
		row("secret-own", 20, `{"combatant_id":"c-b","secret":true}`),
		// firings that add creatures to an earlier one: of a hidden NPC alone, of an NPC the fog showed to bia alone, of a visible one
		row("add-hidden", 30, `{"trap":{"extends_id":"f","caught":[{"target_id":"c-h","hidden":true}]}}`),
		row("add-fogged", 31, `{"trap":{"extends_id":"f","caught":[{"target_id":"c-f","fogged":true,"seen_by":["bia"]}]}}`),
		row("add-mixed", 32, `{"trap":{"extends_id":"f","caught":[{"target_id":"c-h","hidden":true},{"target_id":"c-p"}]}}`),
		row("fire-hidden", 33, `{"trap":{"caught":[{"target_id":"c-h","hidden":true}]}}`),
	}
	cases := []struct {
		name         string
		hosts        []string
		player, boss int64
	}{
		{"a secret host and a fogged-unseen host are not counted", []string{"own", "secret", "fogged"}, 10, 12},
		{"a visible later host counts, past the hidden ones", []string{"own", "secret", "fogged", "later"}, 14, 14},
		{"a fogged host the player saw counts", []string{"own", "secret", "seen-fog"}, 13, 13},
		{"a host that cannot be read is not counted", []string{"own", "unreadable"}, 10, 15},
		{"the entry's own event counts even when it is secret", []string{"secret-own", "secret"}, 20, 20},
		{"a host that is no event is not counted", []string{"own", "gone"}, 10, 10},
		{"creatures only the master is given, added to a firing, are not counted", []string{"own", "add-hidden", "add-fogged"}, 10, 31},
		{"an addition with a creature the player is given is counted", []string{"own", "add-hidden", "add-mixed"}, 32, 32},
		{"a firing of a hidden NPC alone is not an addition: it counts", []string{"own", "fire-hidden"}, 33, 33},
	}
	for _, c := range cases {
		for who, v := range map[string]combatViewer{"player": {userID: "ana"}, "master": {userID: "m", master: true}} {
			entries := []*logEntry{{hosts: c.hosts}}
			stampSeqs(entries, events, v)
			want := c.player
			if who == "master" {
				want = c.boss
			}
			if entries[0].seq != want {
				t.Errorf("%s, for the %s: seq = %d, want %d", c.name, who, entries[0].seq, want)
			}
		}
	}
}

// deltaSafeKinds cannot drift from the log builders: a kind on it is one a builder handles, or one that writes no line.
func TestDeltaSafeKindsAreKindsOfTheLogBuilders(t *testing.T) {
	t.Parallel()
	noLine := map[string]bool{eventEncounterStarted: true, eventInitiativeSubmitted: true, eventInitiativeOrderSet: true, eventTurnEnded: true}
	for kind := range deltaSafeKinds {
		_, handled := logHandlers[kind]
		switch {
		case !handled && !noLine[kind]:
			t.Errorf("%s is delta-safe but no log builder handles it and it is not a kind that writes no line", kind)
		case handled && noLine[kind]:
			t.Errorf("%s is listed as writing no line, but a log builder handles it", kind)
		}
	}
	for kind := range noLine {
		if !deltaSafeKinds[kind] {
			t.Errorf("%s is listed as writing no line but is not delta-safe", kind)
		}
	}
}

// The hint a player hears is the same whatever the master undid: full is the master's copy alone.
func TestCombatLogChangedHintOfAPlayerNeverCarriesFull(t *testing.T) {
	t.Parallel()
	hub := live.New(0)
	s := &Service{hub: hub}
	master, err := hub.Subscribe("camp", live.Subscriber{UserID: "m", Master: true})
	if err != nil {
		t.Fatal(err)
	}
	defer master.Close()
	player, err := hub.Subscribe("camp", live.Subscriber{UserID: "p"})
	if err != nil {
		t.Fatal(err)
	}
	defer player.Close()
	heard := func(sub *live.Subscription) *playv1.WatchGameSessionResponse {
		t.Helper()
		select {
		case ev := <-sub.Events():
			sub.Taken(ev)
			return ev.Message
		default:
			t.Fatal("no hint")
			return nil
		}
	}
	s.publishLogChange(t.Context(), "camp", "enc", true, false)
	ordinary := heard(player)
	heard(master)
	s.publishLogChange(t.Context(), "camp", "enc", true, true) // an undo, of whatever the player could or could not see
	if got := heard(player); !proto.Equal(got, ordinary) {
		t.Errorf("a player heard %v after an undo, %v after an ordinary change; want the same", got, ordinary)
	}
	if !heard(master).GetCombatLogChanged().GetFull() {
		t.Error("the master did not hear full after an undo")
	}
}

// A visible event that makes no line for the player (a roll mode request taken back) asks for the whole log once: the answer's
// last_seq reaches it, so the next read from that number is a plain, empty delta.
func TestListCombatLogSinceSeqOfAPlayerIsNotFullTwiceForALinelessEvent(t *testing.T) {
	t.Parallel()
	a := newArmed(t)
	e := a.threeAndAGoblin(t)
	ask := func(since int64) *playv1.ListCombatLogResponse {
		res, err := a.caio.combat.ListCombatLog(t.Context(), connect.NewRequest(&playv1.ListCombatLogRequest{CampaignId: a.campaignID, EncounterId: e.GetId(), SinceSeq: since}))
		if err != nil {
			t.Fatal(err)
		}
		return res.Msg
	}
	base := ask(0)
	req, err := a.rmRequestRaw(t, a.caio, e, func(r *playv1.RequestRollModeRequest) { r.AttackKey = battleaxe })
	if err != nil {
		t.Fatal(err)
	}
	if err := a.rmCancel(t, a.caio, e, req.GetRequest().GetId()); err != nil {
		t.Fatal(err)
	}
	first := ask(base.GetLastSeq())
	if !first.GetFull() || first.GetLastSeq() <= base.GetLastSeq() {
		t.Fatalf("after the request and its cancel: full = %v, last_seq = %d (was %d); want full and a higher number", first.GetFull(), first.GetLastSeq(), base.GetLastSeq())
	}
	second := ask(first.GetLastSeq())
	if second.GetFull() || len(entriesOf(second)) != 0 || second.GetLastSeq() != first.GetLastSeq() {
		t.Errorf("a second read from last_seq: full = %v, %d entries, last_seq = %d; want a plain empty delta at %d", second.GetFull(), len(entriesOf(second)), second.GetLastSeq(), first.GetLastSeq())
	}
}

// A roll mode request answered while the target is hidden is a secret event: the player's answer to the same since_seq is what it
// was before the answer, and the master's moves.
func TestListCombatLogSinceSeqOfAPlayerIgnoresASecretRollModeAnswer(t *testing.T) {
	t.Parallel()
	a := newArmed(t)
	e := a.threeAndAGoblin(t)
	req, err := a.rmRequestRaw(t, a.caio, e, func(r *playv1.RequestRollModeRequest) { r.AttackKey = battleaxe })
	if err != nil {
		t.Fatal(err)
	}
	hideAgain(t, a, e, "Goblin") // the target goes hidden before the master answers
	ask := func(u *user, since int64) *playv1.ListCombatLogResponse {
		res, err := u.combat.ListCombatLog(t.Context(), connect.NewRequest(&playv1.ListCombatLogRequest{CampaignId: a.campaignID, EncounterId: e.GetId(), SinceSeq: since}))
		if err != nil {
			t.Fatal(err)
		}
		return res.Msg
	}
	playerSince, masterSince := a.log(t, a.caio, e).GetLastSeq(), a.log(t, a.master, e).GetLastSeq()
	playerBefore, masterBefore := ask(a.caio, playerSince), ask(a.master, masterSince)
	if err := a.answerMode(t, a.master, e, req.GetRequest().GetId(), playv1.RollMode_ROLL_MODE_ADVANTAGE); err != nil {
		t.Fatal(err)
	}
	if after := ask(a.caio, playerSince); !proto.Equal(playerBefore, after) {
		t.Errorf("a secret roll mode answer changed the player's answer:\n before %v\n after  %v", playerBefore, after)
	}
	if after := ask(a.master, masterSince); proto.Equal(masterBefore, after) {
		t.Error("the master's answer did not move after the answer: the control is gone")
	}
}
