package play

import (
	"context"
	"testing"
	"time"

	"connectrpc.com/connect"
	"google.golang.org/protobuf/proto"

	playv1 "github.com/PuraFome/meuRPG/backend/gen/meurpg/play/v1"
	"github.com/PuraFome/meuRPG/backend/internal/play/live"
)

// The TV tab's stream. The tests that open a stream need the database (MEURPG_TEST_DATABASE_URL).

// Tests that use harness.screenLeft run one stream rig each and never share it.

// screenClock stands in for the pause between two `changed` events: a test fires it by hand.
type screenClock struct{ waits chan chan time.Time }

func newScreenClock() *screenClock { return &screenClock{waits: make(chan chan time.Time, 64)} }

func (c *screenClock) after(time.Duration) <-chan time.Time {
	ch := make(chan time.Time, 1)
	c.waits <- ch
	return ch
}

// fire ends the wait a stream started, and fails if none starts.
func (c *screenClock) fire(t *testing.T) {
	t.Helper()
	select {
	case ch := <-c.waits:
		ch <- time.Now()
	case <-time.After(waitLimit):
		t.Fatal("no stream started a wait for a `changed`")
	}
}

// tvWatcher reads one WatchTableScreen stream in the background.
type tvWatcher struct {
	events chan *playv1.WatchTableScreenResponse
	done   chan error
	cancel context.CancelFunc
	// left delivers once for each TV stream of the instance that was uncounted.
	left <-chan struct{}
}

func (a *armed) watchScreen(t *testing.T, u *user) *tvWatcher {
	t.Helper()
	ctx, cancel := context.WithCancel(t.Context())
	t.Cleanup(cancel)
	w := &tvWatcher{events: make(chan *playv1.WatchTableScreenResponse, 64), done: make(chan error, 1), cancel: cancel, left: a.h.screenLeft}
	stream, err := u.screen.WatchTableScreen(ctx, connect.NewRequest(&playv1.WatchTableScreenRequest{CampaignId: a.campaignID}))
	if err != nil {
		t.Fatalf("WatchTableScreen() error = %v", err)
	}
	go func() {
		defer close(w.events)
		for stream.Receive() {
			w.events <- stream.Msg()
		}
		w.done <- stream.Err()
		_ = stream.Close()
	}()
	return w
}

// next returns the next event, which must be one a TV stream may send, and with nothing set when it is a `changed`.
func (w *tvWatcher) next(t *testing.T) *playv1.WatchTableScreenResponse {
	t.Helper()
	select {
	case ev, ok := <-w.events:
		if !ok {
			t.Fatalf("the stream ended (%v), want another event", <-w.done)
		}
		if ev.GetReady() == nil && ev.GetHeartbeat() == nil && ev.GetChanged() == nil && ev.GetSessionEnded() == nil {
			t.Fatalf("event %v is none of ready, heartbeat, changed and session_ended", ev)
		}
		if proto.Size(ev) != 2 { // the oneof's tag and an empty message's length
			t.Fatalf("event %v carries something: %d bytes, want the bare tag and an empty message", ev, proto.Size(ev))
		}
		return ev
	case <-time.After(waitLimit):
		t.Fatal("no event within the time limit")
	}
	return nil
}

func (w *tvWatcher) ready(t *testing.T) {
	t.Helper()
	if ev := w.next(t); ev.GetReady() == nil {
		t.Fatalf("first event = %v, want ready", ev)
	}
}

func (w *tvWatcher) changed(t *testing.T) {
	t.Helper()
	if ev := w.next(t); ev.GetChanged() == nil {
		t.Fatalf("event = %v, want changed", ev)
	}
}

// end waits for the stream to end and returns its error.
func (w *tvWatcher) end(t *testing.T) error {
	t.Helper()
	timeout := time.After(waitLimit)
	for {
		select {
		case ev, ok := <-w.events:
			if ok && ev.GetHeartbeat() == nil && ev.GetSessionEnded() == nil {
				t.Fatalf("event %v, want the stream to end", ev)
			}
			if !ok {
				return <-w.done
			}
		case <-timeout:
			t.Fatal("the stream did not end within the time limit")
		}
	}
}

// armedWithClock is an armed table whose TV streams wait on a clock the test fires.
func armedWithClock(t *testing.T) (*armed, *screenClock) {
	t.Helper()
	a := newArmed(t)
	clock := newScreenClock()
	a.h.svc.live.ScreenAfter = clock.after
	a.h.screenLeft = make(chan struct{}, 16)
	a.h.svc.screens.afterLeft = func() { a.h.screenLeft <- struct{}{} }
	return a, clock
}

func TestWatchTableScreenIsTheMastersAndNeedsASession(t *testing.T) {
	t.Parallel()
	a := newArmed(t)
	wantCode(t, "WatchTableScreen as a player", a.watchScreen(t, a.caio).end(t), connect.CodePermissionDenied)
	wantCode(t, "WatchTableScreen as a stranger", a.watchScreen(t, a.h.newUser("Intruso")).end(t), connect.CodeNotFound)
	if n := a.h.svc.hub.Count(a.campaignID); n != 0 {
		t.Errorf("hub subscriptions after refused streams = %d, want 0", n)
	}
	a.master.end(t, a.master.liveSession(t, a.campaignID).GetGameSession())
	wantBlocked(t, "WatchTableScreen", a.watchScreen(t, a.master).end(t), playv1.GameSessionBlockedReason_GAME_SESSION_BLOCKED_REASON_NO_OPEN_SESSION)
}

// A move of a hidden combatant is a change the master's stream hears: the TV stream turns it into an empty
// `changed`, and what the tab then reads is the same as before it.
func TestWatchTableScreenHintsCarryNothingAndAHiddenMoveShowsNothing(t *testing.T) {
	t.Parallel()
	a, clock := armedWithClock(t)
	e, _ := a.fightWithAnOgre(t)
	tv := a.watchScreen(t, a.master)
	tv.ready(t)
	before := a.mustGetScreen(t)

	if _, err := a.adjustHP(t, e, "Ogro Oculto", damageHP(100)); err != nil {
		t.Fatalf("AdjustCombatantHitPoints(hidden ogre) error = %v", err)
	}
	clock.fire(t)
	tv.changed(t)
	if after := a.mustGetScreen(t); !proto.Equal(before, after) {
		t.Errorf("the table screen after the hidden ogre's wound differs:\nbefore %v\nafter  %v", text(before), text(after))
	}

	// The session ends: session_ended, then the stream ends without an error.
	a.master.end(t, a.master.liveSession(t, a.campaignID).GetGameSession())
	for {
		ev := tv.next(t)
		if ev.GetSessionEnded() != nil {
			break
		}
		if ev.GetChanged() == nil {
			t.Fatalf("event %v before session_ended", ev)
		}
	}
	if err := tv.end(t); err != nil {
		t.Errorf("stream after session_ended: error = %v, want a clean end", err)
	}
}

func TestWatchTableScreenCoalescesABurst(t *testing.T) {
	t.Parallel()
	a, clock := armedWithClock(t)
	tv := a.watchScreen(t, a.master)
	tv.ready(t)
	const burst = 12
	for range burst {
		a.publishMasterEvent()
	}
	var first chan time.Time
	select {
	case first = <-clock.waits:
	case <-time.After(waitLimit):
		t.Fatal("no wait started for the burst")
	}
	// While that wait is open, no event starts another, and nothing is sent. No seam tells the test that the stream took
	// every event of the burst off its queue, so this check can pass for lack of time but never fails wrongly; the exact
	// count of waits for a burst is proved without the stream, on the gate itself, by TestChangedGateWaitsOncePerBurst.
	if n := len(clock.waits); n != 0 {
		t.Errorf("waits started by a burst of %d events = %d more, want none", burst, n)
	}
	select {
	case ev := <-tv.events:
		t.Fatalf("event %v before the wait ended", ev)
	default:
	}
	first <- time.Now()
	tv.changed(t)
	// The trailing edge (an event after a send is followed by its own `changed`) is TestChangedGateWaitsOncePerBurst's.
}

// publishMasterEvent publishes an event of the master's audience that carries nothing the TV may read.
func (a *armed) publishMasterEvent() {
	a.h.svc.hub.Publish(a.campaignID, live.Event{Audience: live.Audience{Master: true}, Message: &playv1.WatchGameSessionResponse{Event: &playv1.WatchGameSessionResponse_Heartbeat_{Heartbeat: &playv1.WatchGameSessionResponse_Heartbeat{}}}})
}

func TestChangedGateWaitsOncePerBurst(t *testing.T) {
	t.Parallel()
	clock := newScreenClock()
	g := changedGate{after: clock.after, every: time.Second}
	for range 10 {
		g.hit()
	}
	if len(clock.waits) != 1 {
		t.Fatalf("waits after a burst = %d, want 1", len(clock.waits))
	}
	if g.due == nil {
		t.Fatal("no `changed` is due after events")
	}
	(<-clock.waits) <- time.Now()
	<-g.due
	g.sent()
	if g.due != nil {
		t.Error("a `changed` is due after it was sent")
	}
	g.hit() // an event after the send starts a new wait: the last event is never left without a `changed`
	if len(clock.waits) != 1 || g.due == nil {
		t.Errorf("waits after an event that follows a send = %d, want 1", len(clock.waits))
	}
}

func TestWatchTableScreenPresenceReachesTheMastersStreamAlone(t *testing.T) {
	t.Parallel()
	a, clock := armedWithClock(t)
	master, ana := a.master.watch(t, a.campaignID), a.ana.watch(t, a.campaignID)
	master.ready(t)
	ana.ready(t)
	if p := a.mustGetScreen(t).GetPresence(); p.GetOpen() || p.GetLostAt() != nil {
		t.Errorf("presence before any TV = %v, want closed and no loss", p)
	}
	heard := func() []*playv1.WatchGameSessionResponse_TableScreenChanged {
		a.master.markCurrentMap(t, a.campaignID, a.mapID, ana, master)
		var out []*playv1.WatchGameSessionResponse_TableScreenChanged
		for _, ev := range master.beforeMarker(t) {
			if c := ev.GetTableScreenChanged(); c != nil {
				out = append(out, c)
			}
		}
		for _, ev := range ana.beforeMarker(t) {
			if ev.GetTableScreenChanged() != nil {
				t.Error("a player's stream got table_screen_changed")
			}
		}
		return out
	}

	first, second := a.watchScreen(t, a.master), a.watchScreen(t, a.master)
	first.ready(t)
	second.ready(t)
	got := heard()
	if len(got) != 1 || !got[0].GetPresenceKnown() || !got[0].GetOpen() || got[0].GetLostAt() != nil {
		t.Fatalf("two TV tabs connected: the master heard %v, want one open event", got)
	}
	if p := a.mustGetScreen(t).GetPresence(); !p.GetOpen() || p.GetLostAt() != nil {
		t.Errorf("presence with a TV = %v, want open", p)
	}
	if _, err := a.setScreen(a.master, 0, false, kMap); err != nil {
		t.Fatalf("SetTableScreen() error = %v", err)
	}
	clock.fire(t) // the choice reaches both TV streams as a `changed`
	clock.fire(t)
	first.changed(t)
	second.changed(t)

	// The first tab leaves: nothing is published; the last one leaving is.
	first.leave(t)
	if got := heard(); len(got) != 1 || got[0].GetPresenceKnown() || got[0].GetChoiceRevision() != 1 {
		t.Fatalf("one of two TV tabs left: the master heard %v, want only the choice event", got)
	}
	second.leave(t)
	got = heard()
	if len(got) != 1 || !got[0].GetPresenceKnown() || got[0].GetOpen() || got[0].GetLostAt() == nil || got[0].GetChoiceRevision() != 1 {
		t.Fatalf("the last TV tab left: the master heard %v, want closed with lost_at at revision 1", got)
	}
	if p := a.mustGetScreen(t).GetPresence(); p.GetOpen() || !p.GetLostAt().AsTime().Equal(got[0].GetLostAt().AsTime()) {
		t.Errorf("presence after the loss = %v, want closed with the lost_at the master heard", p)
	}
}

// The presence messages skip the TV streams: a stream's own coming and going would otherwise start a
// `changed`. Delivery to the hub is synchronous, so the TV stream's queue is read right after.
func TestPresenceDoesNotReachTVStreams(t *testing.T) {
	t.Parallel()
	a := newArmed(t)
	svc, id := a.h.svc, a.master.liveSession(t, a.campaignID).GetGameSession().GetId()
	screen, err := svc.hub.Subscribe(a.campaignID, live.Subscriber{UserID: "tv:probe", Master: true, Screen: true})
	if err != nil {
		t.Fatal(err)
	}
	session, err := svc.hub.Subscribe(a.campaignID, live.Subscriber{UserID: "probe", Master: true})
	if err != nil {
		t.Fatal(err)
	}
	svc.screenJoined(a.campaignID, id, 0) // 0 -> 1
	svc.screenJoined(a.campaignID, id, 0) // 1 -> 2: publishes nothing
	svc.screenLeft(t.Context(), a.campaignID, id, 0)
	svc.screenLeft(t.Context(), a.campaignID, id, 0) // 1 -> 0
	if got := len(screen.Events()); got != 0 {
		t.Errorf("the TV stream was sent %d presence messages, want none", got)
	}
	if got := len(session.Events()); got != 2 {
		t.Errorf("the master's session stream was sent %d presence messages, want 2 (opened, lost)", got)
	}
}

// A session that ends leaves nothing lost, and the next session starts with no TV and no loss.
func TestWatchTableScreenForgetsAnEndedSession(t *testing.T) {
	t.Parallel()
	a, _ := armedWithClock(t)
	tv := a.watchScreen(t, a.master)
	tv.ready(t)
	session := a.master.liveSession(t, a.campaignID).GetGameSession()
	a.master.end(t, session)
	if err := tv.end(t); err != nil {
		t.Fatalf("stream after the session ended: error = %v", err)
	}
	tv.wasUncounted(t)
	a.h.svc.screens.mu.Lock()
	kept := len(a.h.svc.screens.by)
	a.h.svc.screens.mu.Unlock()
	if kept != 0 {
		t.Errorf("counts kept for an ended session = %d, want none", kept)
	}
	a.master.start(t, a.campaignID)
	if p := a.mustGetScreen(t).GetPresence(); p.GetOpen() || p.GetLostAt() != nil {
		t.Errorf("presence of the new session = %v, want closed and no loss", p)
	}
}

func TestWatchTableScreenCapReplacesTheOldestTVStreamOnly(t *testing.T) {
	t.Parallel()
	a, _ := armedWithClock(t)
	a.h.svc.hub.SetMaxPerUser(2)
	own := []*watcher{a.master.watch(t, a.campaignID), a.master.watch(t, a.campaignID)}
	for _, w := range own {
		w.ready(t)
	}
	oldest, middle := a.watchScreen(t, a.master), a.watchScreen(t, a.master)
	oldest.ready(t)
	middle.ready(t)
	newest := a.watchScreen(t, a.master)
	newest.ready(t)
	wantCode(t, "the oldest TV stream", oldest.end(t), connect.CodeUnavailable)
	oldest.wasUncounted(t)
	// The master's own session streams are still there: they hear a marker.
	a.master.markCurrentMap(t, a.campaignID, a.mapID, own...)
	for _, w := range own {
		w.beforeMarker(t)
	}
	if n := a.h.svc.hub.Count(a.campaignID); n != 4 {
		t.Errorf("hub subscriptions = %d, want 2 session streams and 2 TV streams", n)
	}
	if p := a.mustGetScreen(t).GetPresence(); !p.GetOpen() {
		t.Errorf("presence with two TV streams left = %v, want open", p)
	}
}

// leave closes the tab's stream and waits until the instance has uncounted it.
func (w *tvWatcher) leave(t *testing.T) {
	t.Helper()
	w.cancel()
	w.wasUncounted(t)
}

// wasUncounted waits until a TV stream of the instance has been uncounted.
func (w *tvWatcher) wasUncounted(t *testing.T) {
	t.Helper()
	select {
	case <-w.left:
	case <-time.After(waitLimit):
		t.Fatal("no TV stream was uncounted")
	}
}
