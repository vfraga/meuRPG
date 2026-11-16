package leaktest

import (
	"bytes"
	"testing"
	"time"

	"google.golang.org/protobuf/proto"

	playv1 "github.com/PuraFome/meuRPG/backend/gen/meurpg/play/v1"
	"github.com/PuraFome/meuRPG/backend/gen/meurpg/play/v1/playv1connect"
)

// The Inspiration the master grants (RN-10, RN-43): who has it is the master's and the holder's player's to read. Another
// player reads neither who has it nor that it was granted: not in GetFavor, not in the combat log, not in the stream, and the
// session's hint number steps by one for them whoever the master picks, so it tells them nothing.
//
// The holders are the characters of two players who joined late, Dani's and Edu's, named so that no other text of the fixture
// holds the name; Caio is the third player. The positive controls are the master's and the holders' own reads, and the hints
// the master's and the holders' streams hear.

// grantFavor is the master's GrantFavor.
func (w *world) grantFavor(characterID string) {
	must(w.master.favor.GrantFavor(w.t.Context(), rq(&playv1.GrantFavorRequest{CampaignId: w.campaign, CharacterId: characterID, IdempotencyKey: newKey()})))
}

// hintNumber is the session's hint number as a stream says it: the first heartbeat sent after now.
func hintNumber(sw *streamWatcher) int64 {
	return hintNumberOtherThan(sw, -1)
}

// hintNumberOtherThan is the first heartbeat's number, sent after now, that is not old: a heartbeat the stream sent before the
// write committed says the old number, so it is passed over. When none says another number before the deadline, it is old.
func hintNumberOtherThan(sw *streamWatcher, old int64) int64 {
	seen := sw.count()
	deadline := time.After(20 * time.Second)
	for {
		events := sw.since(seen)
		seen += len(events)
		for _, ev := range events {
			if hb := ev.GetHeartbeat(); hb != nil && hb.GetSessionSeq() != old {
				return hb.GetSessionSeq()
			}
		}
		select {
		case <-sw.more:
		case <-deadline:
			return old
		}
	}
}

func TestFavorReachesOnlyTheMasterAndTheOwner(t *testing.T) {
	w := newWorld(t)
	defer func() {
		if r := recover(); r != nil {
			t.Fatalf("the test: %v", r)
		}
	}()
	m, caio := w.master, w.caio
	dani, edu := w.newPerson("Dani"), w.newPerson("Edu")
	w.join(false, dani, edu)
	// The puzzles on screen are the table's, and Dani and Edu sit at it as Ana and Caio do.
	for _, role := range []string{"shown", "riddle-shown", "split"} {
		if pz := w.puzzles[role]; pz != nil {
			w.secrets.allowNeedle(pz.GetId(), w.ana, w.caio, dani, edu)
		}
	}
	first := w.pc(dani, "Zorvak4321", "race:human")
	second := w.pc(edu, "Nymrix8765", "race:human")
	canaries := map[*person][]*canary{}
	for owner, ch := range map[*person]interface {
		GetName() string
		GetId() string
	}{dani: first, edu: second} {
		for _, text := range []string{ch.GetName(), ch.GetId()} {
			c := &canary{needle: text, kind: "favor-holder", readers: names([]*person{owner})}
			w.secrets.add(c)
			canaries[owner] = append(canaries[owner], c)
		}
	}

	getFavor := read{
		procedure: playv1connect.FavorServiceGetFavorProcedure, allow: members,
		req: func(w *world) proto.Message { return &playv1.GetFavorRequest{CampaignId: w.campaign} },
	}
	combatLog := read{
		procedure: playv1connect.CombatServiceListCombatLogProcedure, allow: members,
		req: func(w *world) proto.Message {
			return &playv1.ListCombatLogRequest{CampaignId: w.campaign, EncounterId: w.encounter.GetId()}
		},
	}
	answers := newAnswers()
	// What Caio reads of the two: it must be the same before the grants and after each one. The bytes can be compared: neither
	// answer has a clock or a revision, and the log's last_seq does not move on a write its reader may not see.
	caioReads := func() [][]byte {
		var out [][]byte
		for _, r := range []read{getFavor, combatLog} {
			reply := caio.call(r.procedure, r.request(w, caio))
			answers.keep(caio, reply)
			if !reply.ok() {
				t.Fatalf("Caio's %s was refused: HTTP %d %s", r.name(), reply.status, reply.body)
			}
			for _, f := range w.inspect(caio, reply, nil) {
				t.Errorf("Caio's %s: %s\n\tanswer: %s", r.name(), f, shorten(reply.body))
			}
			out = append(out, reply.body)
		}
		return out
	}
	sameReads := func(when string, want, have [][]byte) {
		t.Helper()
		for i := range want {
			if !bytes.Equal(want[i], have[i]) {
				t.Errorf("Caio's read %d is not the same %s:\n\tbefore: %s\n\tnow:    %s", i, when, shorten(want[i]), shorten(have[i]))
			}
		}
	}

	watchers := map[*person]*streamWatcher{m: w.watchStream(m), dani: w.watchStream(dani), edu: w.watchStream(edu), caio: w.watchStream(caio), w.ana: w.watchStream(w.ana)}
	before, reads := hintNumber(watchers[caio]), caioReads()
	anaBefore := hintNumber(watchers[w.ana])

	// The first grant, in the middle of the combat the fixture runs.
	w.grantFavor(first.GetId())
	afterFirst := hintNumberOtherThan(watchers[caio], before)
	if afterFirst != before+1 {
		t.Errorf("Caio's hint number went from %d to %d with the first grant, want one step", before, afterFirst)
	}
	if anaAfterFirst := hintNumberOtherThan(watchers[w.ana], anaBefore); anaAfterFirst != anaBefore+1 {
		t.Errorf("Ana's hint number went from %d to %d with the first grant, want one step", anaBefore, anaAfterFirst)
	}
	sameReads("after the first grant", reads, caioReads())

	// The second, to another character: the same for Caio, so the number does not say which one was picked.
	w.grantFavor(second.GetId())
	afterSecond := hintNumberOtherThan(watchers[caio], afterFirst)
	if afterSecond != afterFirst+1 {
		t.Errorf("Caio's hint number went from %d to %d with the second grant, want one step", afterFirst, afterSecond)
	}
	if anaAfterSecond := hintNumberOtherThan(watchers[w.ana], anaBefore+1); anaAfterSecond != anaBefore+2 {
		t.Errorf("Ana's hint number went from %d to %d with the second grant, want one step", anaBefore+1, anaAfterSecond)
	}
	sameReads("after the second grant", reads, caioReads())

	// The reads of everyone: the master reads both holders and each holder's player reads their own (the positive controls),
	// the others read none.
	w.runRead(t, getFavor, answers)
	w.runRead(t, combatLog, answers)
	for _, p := range []*person{dani, edu} {
		reply := p.call(getFavor.procedure, getFavor.request(w, p))
		answers.keep(p, reply)
		if !reply.ok() {
			t.Fatalf("%s was refused GetFavor: HTTP %d %s", p.name, reply.status, reply.body)
		}
		for _, f := range w.inspect(p, reply, nil) {
			t.Errorf("%s: %s\n\tanswer: %s", p.name, f, shorten(reply.body))
		}
	}
	for owner, list := range canaries {
		for _, c := range list {
			for _, p := range []*person{m, owner} {
				if !answers.holds(p, c) {
					t.Errorf("%s reads %s in no answer: the control that the read works is missing", p.name, c)
				}
			}
			for _, p := range []*person{w.ana, caio, w.pending, w.stranger} {
				if answers.holds(p, c) {
					t.Errorf("%s reads %s", p.name, c)
				}
			}
		}
	}

	// The session's end flushes the streams: the master heard both grants and each holder's player the grant to their own
	// character (the vitals hints); Ana and Caio heard none.
	must(m.combat.EndEncounter(t.Context(), rq(&playv1.EndEncounterRequest{CampaignId: w.campaign, EncounterId: w.encounter.GetId(), IdempotencyKey: newKey()})))
	must(m.play.EndGameSession(t.Context(), rq(&playv1.EndGameSessionRequest{CampaignId: w.campaign, GameSessionId: w.session})))
	for p, sw := range watchers {
		select {
		case <-sw.done:
		case <-time.After(30 * time.Second):
			t.Fatalf("%s's stream did not end with the session", p.name)
		}
		hints := map[string]bool{}
		for _, ev := range sw.since(0) {
			if v := ev.GetVitalsChanged().GetVitals(); v != nil {
				hints[v.GetCharacterId()] = true
			}
			if p == m {
				continue
			}
			body := mustJSON(ev)
			for _, f := range w.inspect(p, reply{status: 200, body: body, msg: ev}, nil) {
				t.Errorf("%s: event %s: %s\n\tevent: %s", p.name, eventCase(ev), f, shorten(body))
			}
		}
		want := map[*person][2]bool{m: {true, true}, dani: {true, false}, edu: {false, true}, caio: {false, false}, w.ana: {false, false}}[p]
		if heard := [2]bool{hints[first.GetId()], hints[second.GetId()]}; heard != want {
			t.Errorf("%s's stream heard the vitals hints of (first, second) = %v, want %v", p.name, heard, want)
		}
	}

	// After the session: the summary every member reads has no line of the grants.
	summary := read{procedure: playv1connect.PlayServiceGetSessionSummaryProcedure, allow: members, req: func(w *world) proto.Message {
		return &playv1.GetSessionSummaryRequest{CampaignId: w.campaign, GameSessionId: w.session}
	}}
	w.runRead(t, summary, answers)
}

// The master's grant to a character that is in the combat (a combatant, Toren) is a line of its log, there so that the master can undo it
// as the combat's last action (RN-43): the line is his alone, so the holder's own player (Caio) and the other players read nothing of it,
// and they read the log as they read it before.
func TestFavorGrantLineInACombatIsTheMastersAlone(t *testing.T) {
	w := newWorld(t)
	defer func() {
		if r := recover(); r != nil {
			t.Fatalf("the test: %v", r)
		}
	}()
	line := &canary{needle: "O mestre deu a Inspiração a", kind: "favor-grant-line", readers: names(nil)}
	w.secrets.add(line)
	combatLog := read{
		procedure: playv1connect.CombatServiceListCombatLogProcedure, allow: members,
		req: func(w *world) proto.Message {
			return &playv1.ListCombatLogRequest{CampaignId: w.campaign, EncounterId: w.encounter.GetId()}
		},
	}
	answers := newAnswers()
	w.grantFavor(w.toren.GetId())
	w.runRead(t, combatLog, answers)
	if !answers.holds(w.master, line) {
		t.Errorf("the master reads %s in no answer: the control that the line is there is missing", line)
	}
	for _, p := range []*person{w.caio, w.ana, w.pending, w.stranger} {
		if answers.holds(p, line) {
			t.Errorf("%s reads %s", p.name, line)
		}
	}
}
