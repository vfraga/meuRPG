package play

import (
	"context"
	"slices"
	"testing"
	"time"

	"connectrpc.com/connect"

	playv1 "github.com/PuraFome/meuRPG/backend/gen/meurpg/play/v1"
	"github.com/PuraFome/meuRPG/backend/internal/platform/dbtest"
	"github.com/PuraFome/meuRPG/backend/internal/play/live"
)

// Ending the session (#735 part 2, 4d) goes through Service.tx: the session's row is locked first, its number moves once with the end,
// and every stream hears the end with it. A repeat of the call, on a session ended already, changes and tells nothing.

const kSessionEnded = "*playv1.WatchGameSessionResponse_SessionEnded_"

func (a *armed) endCall(t *testing.T, sessionID string) func() error {
	t.Helper()
	return func() error {
		_, err := a.master.play.EndGameSession(t.Context(), connect.NewRequest(&playv1.EndGameSessionRequest{CampaignId: a.campaignID, GameSessionId: sessionID}))
		return err
	}
}

func TestEndGameSessionHints(t *testing.T) {
	t.Parallel()
	a := newArmed(t)
	g := a.stageHints(t)
	heard := g.first(a.endCall(t, g.session), true)
	heardKinds(t, heard, map[string][]string{"master": {kSessionEnded}, "Ana": {kSessionEnded}, "Caio": {kSessionEnded}, "Bia": {kSessionEnded}})
}

func TestEndGameSessionAgainMovesNothing(t *testing.T) {
	t.Parallel()
	a := newArmed(t)
	g := a.stageHints(t)
	do(t, a.endCall(t, g.session))
	g.drain()
	before := g.seq()
	do(t, a.endCall(t, g.session))
	if got := g.seq(); got != before {
		t.Errorf("hint_seq = %d, want %d: the session was ended already", got, before)
	}
	for name, evs := range g.drain() {
		if len(evs) != 0 {
			t.Errorf("%s heard %v of a session ended before", name, eventKinds(evs))
		}
	}
}

func TestEndGameSessionRefusedTellsNothing(t *testing.T) {
	t.Parallel()
	a := newArmed(t)
	g := a.stageHints(t)
	g.refused(a.endCall(t, newKey())) // no such session
}

// A master ends only the sessions of his own campaign: the id of another campaign's session answers as an unknown one, and its
// row is never locked (a transaction holding it does not make the call wait), written or numbered.
func TestEndGameSessionOfAnotherCampaignAnswersAsUnknownAndLocksNothing(t *testing.T) {
	t.Parallel()
	dbtest.PoolSize(t, 6) // the test holds a connection of its own
	a := newArmed(t)
	otto := a.h.newUser("Otto")
	other := a.h.newCampaign(otto, "Outra mesa")
	foreign := otto.start(t, other).GetGameSession()

	unknown := a.endCall(t, newKey())()
	hold, err := a.h.pool.Begin(t.Context())
	if err != nil {
		t.Fatalf("Begin() error = %v", err)
	}
	t.Cleanup(func() { _ = hold.Rollback(context.Background()) })
	var locked string
	if err := hold.QueryRow(t.Context(), `SELECT id FROM game_sessions WHERE id = $1 FOR UPDATE`, foreign.GetId()).Scan(&locked); err != nil {
		t.Fatalf("lock the foreign session: %v", err)
	}
	answer := make(chan error, 1)
	go func() { answer <- a.endCall(t, foreign.GetId())() }()
	var got error
	select {
	case got = <-answer:
	case <-time.After(20 * time.Second): // the test's ceiling, not a delay: a call that locked the foreign row would wait for it
		t.Fatal("EndGameSession waited for the row of another campaign's session")
	}
	if connect.CodeOf(got) != connect.CodeOf(unknown) || got.Error() != unknown.Error() {
		t.Errorf("EndGameSession(foreign id) = %v, want the answer of an unknown id: %v", got, unknown)
	}
	if err := hold.Rollback(t.Context()); err != nil {
		t.Fatalf("Rollback() error = %v", err)
	}
	if n := a.h.hintSeq(t, foreign.GetId()); n != 0 {
		t.Errorf("the foreign session's hint_seq = %d, want 0", n)
	}
	if live := otto.liveSession(t, other).GetGameSession(); live.GetEndedAt() != nil {
		t.Errorf("the foreign session was ended")
	}
}

// A write that read the open session and lost it to the end before its lock is refused with no open session, and moves nothing.
func TestAWriteRacingTheEndOfTheSessionIsRefusedAndMovesNothing(t *testing.T) {
	t.Parallel()
	a := newArmed(t)
	g := a.stageHints(t)
	var afterEnd int64
	a.h.svc.afterSessionRead = func(string) {
		a.h.svc.afterSessionRead = nil
		do(t, a.endCall(t, g.session))
		afterEnd = g.seq()
		g.drain() // what the end told
	}
	err := a.sayCall(a.master, "", "Tarde demais.")()
	wantBlocked(t, "SendSessionMessage() on a session that ended", err, playv1.GameSessionBlockedReason_GAME_SESSION_BLOCKED_REASON_NO_OPEN_SESSION)
	if got := g.seq(); got != afterEnd {
		t.Errorf("hint_seq = %d after the refused line, want %d", got, afterEnd)
	}
	for name, evs := range g.drain() {
		if len(evs) != 0 {
			t.Errorf("%s heard %v of a refused line", name, eventKinds(evs))
		}
	}
}

// An image kept with the players outlives the session: the end leaves it with the players who saw it and tells them it ended.
func TestEndGameSessionLeavesTheKeptImageWithItsViewers(t *testing.T) {
	t.Parallel()
	at := newSceneTable(t)
	image := at.image(t)
	do(t, at.showCall(image, true, imageAudience(kindChosen, at.ana)))
	g := at.sceneHints(t)
	heard := g.first(func() error {
		_, err := at.master.play.EndGameSession(t.Context(), connect.NewRequest(&playv1.EndGameSessionRequest{CampaignId: at.campaignID, GameSessionId: g.session}))
		return err
	}, true)
	heardKinds(t, heard, map[string][]string{"master": {kSessionEnded}, "Ana": {kSessionEnded}, "Caio": {kSessionEnded}, "Bia": {kSessionEnded}})
	if got := at.leftFor(t, at.ana); !slices.Contains(got, image) {
		t.Errorf("Ana's left images = %v, want the kept one", got)
	}
	if got := at.leftFor(t, at.caio); slices.Contains(got, image) {
		t.Errorf("Caio's left images = %v, want none: he never saw it", got)
	}
}

// The fog reads again before the streams end: a combat that ended with its session took a cloud off the map, and the maps module is
// told (VisionChanged) before session_ended is sent, which closes every stream.
func TestEndGameSessionTellsTheFogBeforeTheStreamsEnd(t *testing.T) {
	t.Parallel()
	c := obscureCave(t)
	c.passTo(t, c.zoneFight(t), "Pensantus")
	c.mustCastArea(t, c.ana, "Pensantus", fogCloudKey, slotOfLevel(1), at(7, 7), nil)
	g := newHintRig(t, c.h, c.campaignID, c.master, map[string]*user{"Ana": c.ana, "Caio": c.caio})
	fogAtEnd := int32(-1)
	sent := c.h.svc.dropSend.Load()
	c.h.svc.setDropSend(func(campaignID string, ev live.Event) bool {
		if ev.Message.GetSessionEnded() != nil && fogAtEnd < 0 {
			fogAtEnd = c.h.vision.n.Load()
		}
		return (*sent)(campaignID, ev)
	})
	fog, before := c.h.vision.n.Load(), g.seq()
	do(t, func() error {
		_, err := c.master.play.EndGameSession(t.Context(), connect.NewRequest(&playv1.EndGameSessionRequest{CampaignId: c.campaignID, GameSessionId: g.session}))
		return err
	})
	if got := g.seq(); got != before+1 {
		t.Errorf("hint_seq = %d, want %d: one bump for the end", got, before+1)
	}
	if fogAtEnd != fog+1 {
		t.Errorf("the fog had been told %d times when session_ended went out, want %d: the cloud's map is read again first", fogAtEnd-fog, 1)
	}
	for name, evs := range g.drain() {
		kinds := eventKinds(evs)
		if !slices.Contains(kinds, kSessionEnded) {
			t.Errorf("%s never heard the session end: %v", name, kinds)
		}
	}
}

// The lock that finds the open session (GetOpenGameSessionForUpdate, which reads every column) and the first statement of the funnel
// (LockSessionForHints) lock one row: a write that takes the session through the first and another that
// takes it through the second take turns. CockroachDB locks the key of the index a scan reads, so a lock
// found through the open-session index would not stop the funnel's lock by id.
func TestOpenSessionLockStopsTheFunnelsLock(t *testing.T) {
	t.Parallel()
	dbtest.PoolSize(t, 6)
	a := newArmed(t)
	ctx := t.Context()
	sessionID := a.master.liveSession(t, a.campaignID).GetGameSession().GetId()

	first, err := a.h.pool.Begin(ctx)
	if err != nil {
		t.Fatalf("Begin() error = %v", err)
	}
	t.Cleanup(func() { _ = first.Rollback(context.Background()) })
	if _, err := a.h.svc.queries.WithTx(first).GetOpenGameSessionForUpdate(ctx, a.campaignID); err != nil {
		t.Fatalf("GetOpenGameSessionForUpdate() error = %v", err)
	}
	second, err := dbtest.SideConnection(t, a.h.pool).Begin(ctx)
	if err != nil {
		t.Fatalf("Begin() error = %v", err)
	}
	t.Cleanup(func() { _ = second.Rollback(context.Background()) })
	secondID, err := dbtest.BackendID(ctx, second)
	if err != nil {
		t.Fatalf("BackendID() error = %v", err)
	}
	locked := make(chan error, 1)
	go func() {
		_, err := a.h.svc.queries.WithTx(second).LockSessionForHints(ctx, sessionID)
		locked <- err
	}()
	probe := dbtest.SideConnection(t, a.h.pool)
	deadline := time.Now().Add(20 * time.Second) // the test's ceiling, not a delay
	for {
		n, err := dbtest.WaitingStatements(ctx, probe, `%FROM game_sessions%FOR UPDATE%`, secondID)
		if err != nil {
			t.Fatalf("WaitingStatements() error = %v", err)
		}
		if n > 0 {
			break
		}
		select {
		case err := <-locked:
			t.Fatalf("LockSessionForHints() returned (%v) while the open-session lock was held: the two lock different keys", err)
		default:
		}
		if time.Now().After(deadline) {
			t.Fatal("the funnel's lock never waited")
		}
		time.Sleep(10 * time.Millisecond)
	}
	if err := first.Rollback(ctx); err != nil {
		t.Fatalf("Rollback() error = %v", err)
	}
	if err := <-locked; err != nil {
		t.Errorf("LockSessionForHints() after the first lock ended = %v", err)
	}
}
