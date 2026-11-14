package play

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"net"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"connectrpc.com/connect"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	playv1 "github.com/PuraFome/meuRPG/backend/gen/meurpg/play/v1"
	"github.com/PuraFome/meuRPG/backend/internal/authz"
	"github.com/PuraFome/meuRPG/backend/internal/platform/db"
	"github.com/PuraFome/meuRPG/backend/internal/platform/dbtest"
	"github.com/PuraFome/meuRPG/backend/internal/play/playdb"
)

// Combat-level pins of the hint funnel (#735): what Service.write does with the session's number when the commit's
// outcome is unknown, when the session it read without a lock is gone before it takes the lock, and when the master ends
// the session at the same time. They need the database (MEURPG_TEST_DATABASE_URL).

// keyedEvents counts the session events written under key.
func (x *coreFixture) keyedEvents(t *testing.T, sessionID, key string) int {
	t.Helper()
	var n int
	if err := x.h.pool.QueryRow(t.Context(), `SELECT count(*) FROM session_events WHERE game_session_id = $1 AND idempotency_key = $2`, sessionID, key).Scan(&n); err != nil {
		t.Fatalf("count the events of the key: %v", err)
	}
	return n
}

// lossyCommit is a net.Conn whose next COMMIT (a simple query "commit") gets no answer, as a broken connection gives none:
// either the COMMIT never leaves the client (the transaction is rolled back) or it reaches CockroachDB, which saves it,
// and the answer is lost.
type lossyCommit struct {
	net.Conn
	arm    *atomic.Int32 // 0 off, 1 lose the commit before it is sent, 2 lose its answer
	lostAn bool          // the commit was sent: the next read gets no answer
}

func (c *lossyCommit) Write(b []byte) (int, error) {
	if len(b) > 6 && b[0] == 'Q' && strings.EqualFold(string(bytes.TrimRight(b[5:], "\x00")), "commit") {
		switch c.arm.Swap(0) {
		case 1:
			_ = c.Close()
			return 0, net.ErrClosed
		case 2:
			n, err := c.Conn.Write(b)
			c.lostAn = err == nil
			return n, err
		}
	}
	return c.Conn.Write(b)
}

func (c *lossyCommit) Read(b []byte) (int, error) {
	if c.lostAn {
		// The server answers the commit only once it has run it: wait for that, then lose the answer.
		_, _ = c.Conn.Read(b)
		_ = c.Close()
		return 0, net.ErrClosed
	}
	return c.Conn.Read(b)
}

// useLossyPool makes the service's transactions go through connections that can lose a COMMIT, and returns the switch.
func useLossyPool(t *testing.T, h *harness) *atomic.Int32 {
	t.Helper()
	arm := &atomic.Int32{}
	cfg := h.pool.Config().Copy()
	cfg.MaxConns = 2
	dial := cfg.ConnConfig.DialFunc
	if dial == nil {
		dial = (&net.Dialer{}).DialContext
	}
	cfg.ConnConfig.DialFunc = func(ctx context.Context, network, addr string) (net.Conn, error) {
		conn, err := dial(ctx, network, addr)
		if err != nil {
			return nil, err
		}
		return &lossyCommit{Conn: conn, arm: arm}, nil
	}
	pool, err := pgxpool.NewWithConfig(t.Context(), cfg)
	if err != nil {
		t.Fatal(err)
	}
	was := h.svc.pool
	h.svc.pool = pool
	t.Cleanup(func() { h.svc.pool = was; pool.Close() })
	return arm
}

// A combat change whose COMMIT has an unknown outcome is not retried by the funnel and tells nothing; the caller's retry under
// the same key finds the change saved (and answers with what was stored, with no second bump) or not saved (and makes it once).
// Either way the session's number moved once, the change was written once, and the events heard carry that one number.
func TestCombatWriteAfterAnAmbiguousCommitMakesTheChangeOnceWithOneNumber(t *testing.T) {
	t.Parallel()
	for _, tc := range []struct {
		name  string
		mode  int32
		saved bool
	}{
		{"the commit never reached the database", 1, false},
		{"the commit was saved and its answer lost", 2, true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			x := newCoreFixture(t)
			sid := x.sessionID(t)
			master := listen(t, x.h, x.campaignID, x.master.id, true)
			master.heard(t, x.h) // what the setup left in the stream
			arm := useLossyPool(t, x.h)
			base := x.h.hintSeq(t, sid)
			key := newKey()

			arm.Store(tc.mode)
			_, err := x.writeThatTells(t, key)
			if _, ok := errors.AsType[*db.AmbiguousCommitError](err); !ok {
				t.Fatalf("write() error = %v, want *db.AmbiguousCommitError", err)
			}
			if arm.Load() != 0 {
				t.Fatal("the commit was never lost: the test would prove nothing")
			}
			if got := master.heard(t, x.h); len(got) != 0 {
				t.Errorf("the master heard %v of a change whose commit has an unknown outcome", eventKinds(got))
			}
			wantSaved := 0
			if tc.saved {
				wantSaved = 1
			}
			if got := x.keyedEvents(t, sid, key); got != wantSaved {
				t.Fatalf("events under the key after the lost commit = %d, want %d", got, wantSaved)
			}
			if got, want := x.h.hintSeq(t, sid), base+int64(wantSaved); got != want {
				t.Fatalf("hint_seq after the lost commit = %d, want %d", got, want)
			}

			res, err := x.writeThatTells(t, key) // the caller's retry
			if err != nil {
				t.Fatalf("the retry: write() error = %v", err)
			}
			if res.repeated != tc.saved {
				t.Errorf("the retry repeated = %v, want %v (it finds the stored change only if the commit was saved)", res.repeated, tc.saved)
			}
			if got := x.keyedEvents(t, sid, key); got != 1 {
				t.Errorf("events under the key after the retry = %d, want 1", got)
			}
			if got, want := x.h.hintSeq(t, sid), base+1; got != want {
				t.Errorf("hint_seq after the retry = %d, want %d: one change, one number", got, want)
			}
			heard := master.heard(t, x.h)
			if len(heard) == 0 {
				t.Fatal("the retry told the master nothing")
			}
			for _, ev := range heard {
				if ev.GetSessionSeq() != base+1 {
					t.Errorf("%T carries number %d, want the stored %d", ev.GetEvent(), ev.GetSessionSeq(), base+1)
				}
			}
		})
	}
}

// afterOpenSessionRead makes the unlocked read of the open session in Service.writeOnce (the one Service.tx's lock then
// has to find still true) end with a callback: the test ends or replaces the session there.
type afterOpenSessionRead struct {
	playdb.DBTX
	armed atomic.Bool
	fn    func()
}

func (a *afterOpenSessionRead) QueryRow(ctx context.Context, sql string, args ...any) pgx.Row {
	row := a.DBTX.QueryRow(ctx, sql, args...)
	if strings.Contains(sql, "-- name: GetOpenGameSession :one") {
		return hookedRow{Row: row, a: a}
	}
	return row
}

type hookedRow struct {
	pgx.Row
	a *afterOpenSessionRead
}

func (r hookedRow) Scan(dest ...any) error {
	err := r.Row.Scan(dest...)
	if err == nil && r.a.armed.CompareAndSwap(true, false) {
		r.a.fn() // the row is read and its connection is free
	}
	return err
}

// The session writeOnce read (with no lock) ended, or was replaced by another, before the funnel took its lock: stillOpen
// refuses with NO_OPEN_SESSION, the closure never runs, nothing is written and nothing is told, and neither the dead session's
// number nor the new one's moves.
func TestCombatWriteRefusesWhenTheSessionItReadIsGoneBeforeTheLock(t *testing.T) {
	t.Parallel()
	const noSession = "failed_precondition|failed_precondition: the campaign has no open game session|GAME_SESSION_BLOCKED_REASON_NO_OPEN_SESSION"
	for _, replaced := range []bool{false, true} {
		name := "the session ended"
		if replaced {
			name = "the session ended and another began"
		}
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			x := newCoreFixture(t)
			dead := x.sessionID(t)
			master := listen(t, x.h, x.campaignID, x.master.id, true)
			var fresh string
			var deadSeq, freshSeq int64
			hook := &afterOpenSessionRead{DBTX: x.h.pool}
			hook.fn = func() {
				x.endSession(t)
				if replaced {
					fresh = x.master.start(t, x.campaignID).GetGameSession().GetId()
					freshSeq = x.h.hintSeq(t, fresh)
				}
				deadSeq = x.h.hintSeq(t, dead)
				master.heard(t, x.h) // the end's own events
			}
			was := x.h.svc.queries
			x.h.svc.queries = playdb.New(hook)
			t.Cleanup(func() { x.h.svc.queries = was; x.h.svc.afterSightRead = nil })
			x.h.svc.afterSightRead = func(string) { hook.armed.Store(true) } // the next read of the open session is writeOnce's

			key := newKey()
			ran := false
			m := authz.Membership{UserID: x.master.id, CampaignID: x.campaignID, Role: authz.RoleMaster}
			_, err := x.h.svc.write(t.Context(), combatWrite{m: m, key: key, kind: eventCombatantHiddenSet, encounterID: x.e.GetId()}, func(c *combatTx) (any, error) {
				ran = true
				c.castsChanged = castsPublic
				return map[string]any{"encounter_id": x.e.GetId()}, nil
			})
			if hook.armed.Load() {
				t.Fatal("writeOnce never read the open session: the test would prove nothing")
			}
			if got := shapeOf(err); got != noSession {
				t.Errorf("write() = %s, want %s", got, noSession)
			}
			if ran {
				t.Error("the change's closure ran on a session that was gone")
			}
			if got := x.keyedEvents(t, dead, key); got != 0 {
				t.Errorf("the change left %d events on the dead session", got)
			}
			if got := x.h.hintSeq(t, dead); got != deadSeq {
				t.Errorf("the dead session's number = %d, want %d: a refused change bumps nothing", got, deadSeq)
			}
			if replaced {
				if got := x.h.hintSeq(t, fresh); got != freshSeq {
					t.Errorf("the new session's number = %d, want %d: the change was about the old one", got, freshSeq)
				}
				if got := x.keyedEvents(t, fresh, key); got != 0 {
					t.Errorf("the change left %d events on the new session", got)
				}
			}
			if got := master.heard(t, x.h); len(got) != 0 {
				t.Errorf("the master heard %v of a refused change", eventKinds(got))
			}
		})
	}
}

// The master ends the session while a combat change is being made. The two take the session's row in turn (the change's
// funnel and EndGameSession both lock it first), so there is no deadlock and the outcome is one of two, each consistent: the
// change committed first, with its number and its event, and the end followed; or the end came first and the change is
// refused with NO_OPEN_SESSION and left nothing. The first order is made on purpose and then the race is run.
func TestCombatWriteAndEndGameSessionRaceWithoutDeadlockAndStayConsistent(t *testing.T) {
	t.Parallel()
	const noSession = "failed_precondition|failed_precondition: the campaign has no open game session|GAME_SESSION_BLOCKED_REASON_NO_OPEN_SESSION"
	end := func(x *coreFixture, sid string) error { return endSessionAs(t, x, sid) }
	t.Run("the change commits before the end", func(t *testing.T) {
		t.Parallel()
		x := newCoreFixture(t)
		sid := x.sessionID(t)
		base := x.h.hintSeq(t, sid)
		key := newKey()
		if _, err := x.writeThatTells(t, key); err != nil {
			t.Fatalf("write() error = %v", err)
		}
		if err := end(x, sid); err != nil {
			t.Fatalf("EndGameSession() error = %v", err)
		}
		if got := x.keyedEvents(t, sid, key); got != 1 {
			t.Errorf("events under the key = %d, want 1", got)
		}
		// The change moved the number once, and ending the session is a write too: it moves it once more.
		if got := x.h.hintSeq(t, sid); got != base+2 {
			t.Errorf("hint_seq = %d, want %d (the change's bump and the end's)", got, base+2)
		}
	})
	t.Run("the end comes before the change", func(t *testing.T) {
		t.Parallel()
		x := newCoreFixture(t)
		sid := x.sessionID(t)
		if err := end(x, sid); err != nil {
			t.Fatalf("EndGameSession() error = %v", err)
		}
		base := x.h.hintSeq(t, sid)
		key := newKey()
		_, err := x.writeThatTells(t, key)
		if got := shapeOf(err); got != noSession {
			t.Errorf("write() after the end = %s, want %s", got, noSession)
		}
		if got := x.keyedEvents(t, sid, key); got != 0 {
			t.Errorf("events under the key = %d, want 0", got)
		}
		if got := x.h.hintSeq(t, sid); got != base {
			t.Errorf("hint_seq = %d, want %d: a refused change bumps nothing", got, base)
		}
	})
}

// endSessionAs ends the session as the master and returns the call's error (no t.Fatal, so a goroutine may call it).
func endSessionAs(t *testing.T, x *coreFixture, sid string) error {
	t.Helper()
	_, err := x.master.play.EndGameSession(t.Context(), connect.NewRequest(&playv1.EndGameSessionRequest{CampaignId: x.campaignID, GameSessionId: sid}))
	return err
}

// The same two calls at the same instant, on a pool of several connections, a few times over: whichever wins, the end
// succeeds, the change either committed with its event and exactly one bump or was refused with NO_OPEN_SESSION and left
// nothing, and both calls return (no deadlock).
func TestCombatWriteRacingEndGameSessionIsConsistentWhicheverWins(t *testing.T) {
	t.Parallel()
	const noSession = "failed_precondition|failed_precondition: the campaign has no open game session|GAME_SESSION_BLOCKED_REASON_NO_OPEN_SESSION"
	for round := range 4 {
		t.Run(fmt.Sprintf("round %d", round+1), func(t *testing.T) {
			t.Parallel()
			dbtest.PoolSize(t, 6) // before the harness: on one connection the racers would take turns and prove nothing
			x := newCoreFixture(t)
			sid := x.sessionID(t)
			base := x.h.hintSeq(t, sid)
			key := newKey()
			m := authz.Membership{UserID: x.master.id, CampaignID: x.campaignID, Role: authz.RoleMaster}
			var writeErr, endErr error
			var wg sync.WaitGroup
			start := make(chan struct{})
			done := make(chan struct{})
			wg.Go(func() {
				<-start
				_, writeErr = x.h.svc.write(t.Context(), combatWrite{m: m, key: key, kind: eventCombatantHiddenSet, encounterID: x.e.GetId()}, func(c *combatTx) (any, error) {
					c.castsChanged = castsPublic
					return map[string]any{"encounter_id": x.e.GetId()}, nil
				})
			})
			wg.Go(func() {
				<-start
				endErr = endSessionAs(t, x, sid)
			})
			go func() { wg.Wait(); close(done) }()
			close(start)
			select {
			case <-done:
			case <-time.After(10 * waitLimit):
				t.Fatal("the change and the end of the session did not both return: a deadlock")
			}
			if endErr != nil {
				t.Errorf("EndGameSession() error = %v", endErr)
			}
			// The end moves the number once, whichever wins; the change moves it once more only when it committed.
			wantEvents, wantSeq := 1, base+2
			if writeErr != nil {
				if got := shapeOf(writeErr); got != noSession {
					t.Fatalf("write() = %s, want success or %s", got, noSession)
				}
				wantEvents, wantSeq = 0, base+1
			}
			if got := x.keyedEvents(t, sid, key); got != wantEvents {
				t.Errorf("events under the key = %d, want %d (the change's error: %v)", got, wantEvents, writeErr)
			}
			if got := x.h.hintSeq(t, sid); got != wantSeq {
				t.Errorf("hint_seq = %d, want %d (the change's error: %v)", got, wantSeq, writeErr)
			}
		})
	}
}
