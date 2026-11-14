package leaktest

import (
	"testing"
	"time"

	"google.golang.org/protobuf/proto"

	charactersv1 "github.com/PuraFome/meuRPG/backend/gen/meurpg/characters/v1"
	playv1 "github.com/PuraFome/meuRPG/backend/gen/meurpg/play/v1"
)

// The writers of the scene, the stage, the shown image, the conversation, the group check and the free dice (#735 part 2, 4d) tell
// through Service.tx. This row is the leak check of that move. A scene is open to Ana alone and an image is shown to her alone (the
// setup, before the streams open); then the master makes seven writes only Ana is to read: an NPC on the stage, an NPC hidden on it,
// the audience of the image changed, another image shown to her alone, a line for her alone, a group check asked in the scene and a
// secret roll. Caio is out of all of it. (The scene's own writes are checked on the stream in sceneaudience_test.go.) His stream hears no event at all, its heartbeat reads a
// number that moved once for each write and holds nothing else, and nothing a canary holds is on it; Ana and the master hear the
// writes (the controls). Two worlds, whose canaries differ, leave Caio the same.

type playHiddenRead struct {
	start, end int64
	caio       []string // every event Caio heard apart from the stream's own (ready, heartbeats, the end)
	ana        map[string]bool
	master     map[string]bool
}

const playHiddenWrites = 7

func playHiddenWorld(t *testing.T) playHiddenRead {
	t.Helper()
	w := newWorld(t)
	ps := w.buildPartialScene()
	im, im2 := w.newShown("hint-image", w.ana), w.newShown("hint-image-2", w.ana)
	other, _ := w.npc("hint-stage-2", &charactersv1.BasicSheet{HitPointsMax: 9, ArmorClass: 11, SpeedFt: 30})
	ctx := t.Context()
	if open := must(w.master.contests.GetGroupCheck(ctx, rq(&playv1.GetGroupCheckRequest{CampaignId: w.campaign}))).GetGroupCheck(); open.GetOpen() {
		must(w.master.contests.CloseGroupCheck(ctx, rq(&playv1.CloseGroupCheckRequest{CampaignId: w.campaign, IdempotencyKey: newKey(), GroupCheckId: open.GetId()})))
	}
	w.openSceneTo(ps.point.GetId(), w.ana)
	w.showImage(im, false, &playv1.ImageAudience{Kind: kindScenes})
	w.hints.Settle(t)
	watchers := map[*person]*streamWatcher{w.master: w.watchStream(w.master), w.ana: w.watchStream(w.ana), w.caio: w.watchStream(w.caio)}
	out := playHiddenRead{start: watchers[w.caio].number(), ana: map[string]bool{}, master: map[string]bool{}}

	must(w.master.play.PutOnStage(ctx, rq(&playv1.PutOnStageRequest{CampaignId: w.campaign, CharacterId: ps.npc})))
	must(w.master.play.PutOnStage(ctx, rq(&playv1.PutOnStageRequest{CampaignId: w.campaign, CharacterId: other.GetId(), Hidden: true})))
	w.setImageTo(chosen(w.ana))
	w.showImage(im2, false, chosen(w.ana))
	w.say(w.master, speakNarrator, w.secrets.marker("hint-line", w.ana), func(r *playv1.SendSessionMessageRequest) { r.ReaderUserIds = []string{w.ana.id} })
	must(w.master.contests.RequestGroupCheck(ctx, rq(&playv1.RequestGroupCheckRequest{CampaignId: w.campaign, IdempotencyKey: newKey(), SkillKey: "skill:stealth", Dc: 17})))
	must(w.master.play.RollFreeDice(ctx, rq(&playv1.RollFreeDiceRequest{CampaignId: w.campaign, IdempotencyKey: newKey(), Sides: 20, Count: 1, Secret: true})))

	want := out.start + playHiddenWrites
	for _, sw := range watchers {
		out.end = sw.beatAfter(t, 1, func(got int64) bool { return got == want })
	}
	// Ending the session moves the number once too: a heartbeat after the call reads that number, so the heartbeats are checked up to
	// the event each stream had when the call was made.
	beforeEnd := map[*person]int{}
	for p, sw := range watchers {
		beforeEnd[p] = sw.count()
	}
	must(w.master.play.EndGameSession(ctx, rq(&playv1.EndGameSessionRequest{CampaignId: w.campaign, GameSessionId: w.session})))
	for p, sw := range watchers {
		select {
		case <-sw.done:
		case <-time.After(30 * time.Second):
			t.Fatalf("%s's stream did not end", p.name)
		}
		for i, ev := range sw.since(1) {
			c := eventCase(ev)
			switch c {
			case "heartbeat":
				n := ev.GetHeartbeat().GetSessionSeq()
				if i+1 < beforeEnd[p] && (n < out.start || n > want) {
					t.Errorf("%s: a heartbeat says %d, want a number from %d to %d", p.name, n, out.start, want)
				}
				if p == w.caio && !proto.Equal(ev, &playv1.WatchGameSessionResponse{SessionSeq: n, Event: &playv1.WatchGameSessionResponse_Heartbeat_{Heartbeat: &playv1.WatchGameSessionResponse_Heartbeat{SessionSeq: n}}}) {
					t.Errorf("Caio's heartbeat holds more than the number: %s", mustJSON(ev))
				}
				continue
			case "session_ended":
				continue
			}
			switch p {
			case w.caio:
				out.caio = append(out.caio, eventLine(c, ev))
				for _, f := range w.inspect(p, reply{status: 200, body: mustJSON(ev), msg: ev}, nil) {
					t.Errorf("Caio's stream: %s: %s", c, f)
				}
			case w.ana:
				out.ana[c] = true
			default:
				out.master[c] = true
			}
		}
	}
	return out
}

func TestWritesForSomePlayersMoveTheNumberOfThePlayersOutsideAndTellThemNothing(t *testing.T) {
	a := playHiddenWorld(t)
	b := playHiddenWorld(t)
	for i, r := range []playHiddenRead{a, b} {
		if r.end != r.start+playHiddenWrites {
			t.Errorf("world %d: the number went from %d to %d, want one bump for each of the %d writes", i, r.start, r.end, playHiddenWrites)
		}
		if len(r.caio) != 0 {
			t.Errorf("world %d: Caio, who is out of all of it, heard %v", i, r.caio)
		}
		for _, kind := range []string{"stage_changed", "shown_image_changed", "session_message_added", "group_check_changed"} {
			if !r.ana[kind] {
				t.Errorf("world %d: Ana heard no %s: the control is gone", i, kind)
			}
		}
		if !r.master["stage_changed"] || !r.master["shown_image_changed"] {
			t.Errorf("world %d: the master heard %v, want the stage and the image: the control is gone", i, r.master)
		}
		if r.master["free_roll_rolled"] || r.ana["free_roll_rolled"] {
			t.Errorf("world %d: a secret roll was told", i)
		}
	}
	if a.start != b.start || a.end != b.end {
		t.Errorf("two worlds whose canaries differ moved the number %d to %d and %d to %d: it must not depend on the content", a.start, a.end, b.start, b.end)
	}
}
