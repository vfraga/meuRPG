package play

import (
	"context"
	"errors"
	"fmt"
	"maps"
	"slices"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgxpool"

	playv1 "github.com/PuraFome/meuRPG/backend/gen/meurpg/play/v1"
	"github.com/PuraFome/meuRPG/backend/internal/platform/db"
	"github.com/PuraFome/meuRPG/backend/internal/play/live"
	"github.com/PuraFome/meuRPG/backend/internal/play/playdb"
)

// hintTable is a session with one open stream (the master's) and a scratch table whose rows stand for "the write".
type hintTable struct {
	h          *harness
	campaignID string
	sessionID  string
	master     *live.Subscription
}

func newHintTable(t *testing.T) *hintTable {
	t.Helper()
	h := newHarness(t)
	master := h.newUser("Mestra")
	campaignID := h.newCampaign(master, "Hints")
	sessionID := master.start(t, campaignID).GetGameSession().GetId()
	ctx := t.Context()
	if _, err := h.pool.Exec(ctx, `CREATE TABLE IF NOT EXISTS hint_probe (id INT PRIMARY KEY, n INT NOT NULL DEFAULT 0)`); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _, _ = h.pool.Exec(context.WithoutCancel(ctx), `DROP TABLE IF EXISTS hint_probe`) })
	sub, err := h.svc.hub.Subscribe(campaignID, live.Subscriber{UserID: master.id, Master: true})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(sub.Close)
	return &hintTable{h: h, campaignID: campaignID, sessionID: sessionID, master: sub}
}

// event is a hint whose Coalesce names it, so a test can tell which one arrived.
func (w *hintTable) event(name string, everyone bool) live.Event {
	return live.Event{
		Audience: live.Audience{Master: true, Everyone: everyone},
		Message:  &playv1.WatchGameSessionResponse{Event: &playv1.WatchGameSessionResponse_ContentChanged_{ContentChanged: &playv1.WatchGameSessionResponse_ContentChanged{}}},
		Coalesce: name,
	}
}

// heard drains the master's stream and returns the names of the events in it.
func (w *hintTable) heard() []string {
	var out []string
	for {
		select {
		case ev := <-w.master.Events():
			w.master.Taken(ev)
			out = append(out, ev.Coalesce)
		default:
			return out
		}
	}
}

func (w *hintTable) seq(t *testing.T) int64 {
	t.Helper()
	n, err := w.h.svc.queries.GetHintSeq(t.Context(), w.sessionID)
	if err != nil {
		t.Fatal(err)
	}
	return n
}

// probe reads the scratch row id: -1 when it does not exist.
func (w *hintTable) probe(t *testing.T, id int) int {
	t.Helper()
	var n int
	err := w.h.pool.QueryRow(t.Context(), `SELECT n FROM hint_probe WHERE id = $1`, id).Scan(&n)
	if errors.Is(err, pgx.ErrNoRows) {
		return -1
	}
	if err != nil {
		t.Fatal(err)
	}
	return n
}

func (w *hintTable) write(ctx context.Context, tx pgx.Tx, id int) error {
	_, err := tx.Exec(ctx, `INSERT INTO hint_probe (id, n) VALUES ($1, 1) ON CONFLICT (id) DO UPDATE SET n = hint_probe.n + 1`, id)
	return err
}

func TestTxResetsTheCollectorOnEveryAttempt(t *testing.T) {
	w := newHintTable(t)
	ctx := t.Context()
	attempts := 0
	var first, second live.Event
	err := w.h.svc.tx(ctx, w.sessionID, func(tx pgx.Tx, h *hints) error {
		attempts++
		if err := w.write(ctx, tx, 1); err != nil {
			return err
		}
		if attempts == 1 {
			first = w.event("first-attempt", true)
			h.tell(w.campaignID, first)
			h.touch()
			return &pgconn.PgError{Code: "40001", Message: "restart transaction"}
		}
		second = w.event("second-attempt", true)
		h.tell(w.campaignID, second)
		return nil
	})
	defer func() {
		if first.Message.GetSessionSeq() != 0 || second.Message.GetSessionSeq() != 1 {
			t.Errorf("stamps: first attempt's event %d, second's %d; want 0 and 1", first.Message.GetSessionSeq(), second.Message.GetSessionSeq())
		}
	}()
	if err != nil {
		t.Fatalf("tx() error = %v", err)
	}
	if attempts != 2 {
		t.Fatalf("fn ran %d times, want 2 (one retry)", attempts)
	}
	if got := w.heard(); !slices.Equal(got, []string{"second-attempt"}) {
		t.Errorf("the streams heard %q, want only the second attempt's event, once", got)
	}
	if got := w.seq(t); got != 1 {
		t.Errorf("hint_seq = %d after a retried write, want 1", got)
	}
}

func TestTxAnErrorSendsNothingAndKeepsTheNumber(t *testing.T) {
	w := newHintTable(t)
	ctx := t.Context()
	boom := errors.New("boom")
	err := w.h.svc.tx(ctx, w.sessionID, func(tx pgx.Tx, h *hints) error {
		if err := w.write(ctx, tx, 1); err != nil {
			return err
		}
		h.tell(w.campaignID, w.event("lost", true))
		h.touch()
		return boom
	})
	if !errors.Is(err, boom) {
		t.Fatalf("tx() error = %v, want boom", err)
	}
	if got := w.heard(); len(got) != 0 {
		t.Errorf("the streams heard %q after a failed write", got)
	}
	if got := w.seq(t); got != 0 {
		t.Errorf("hint_seq = %d after a failed write, want 0", got)
	}
	if got := w.probe(t, 1); got != -1 {
		t.Errorf("the failed write left its row (n = %d)", got)
	}
}

// The number is saved with the write: when the bump finds no row (the closure deleted the session after writing), the
// write is rolled back with it and nothing is sent.
func TestTxTheNumberAndTheWriteAreSavedTogether(t *testing.T) {
	w := newHintTable(t)
	ctx := t.Context()
	ran := false
	err := w.h.svc.tx(ctx, w.sessionID, func(tx pgx.Tx, h *hints) error {
		ran = true
		if err := w.write(ctx, tx, 1); err != nil {
			return err
		}
		if _, err := tx.Exec(ctx, `DELETE FROM game_sessions WHERE id = $1`, w.sessionID); err != nil {
			return err
		}
		h.tell(w.campaignID, w.event("lost", true))
		return nil
	})
	if !ran {
		t.Fatal("the closure did not run")
	}
	if !errors.Is(err, pgx.ErrNoRows) {
		t.Fatalf("tx() error = %v, want the bump's no rows", err)
	}
	if got := w.probe(t, 1); got != -1 {
		t.Errorf("the write stayed (n = %d) although its number could not be bumped", got)
	}
	var sessions int
	if err := w.h.pool.QueryRow(ctx, `SELECT count(*) FROM game_sessions WHERE id = $1`, w.sessionID).Scan(&sessions); err != nil || sessions != 1 {
		t.Errorf("the deleted session is back? count = %d, err = %v; want 1 (rolled back)", sessions, err)
	}
	if got := w.heard(); len(got) != 0 {
		t.Errorf("the streams heard %q", got)
	}
}

// failingCommit begins real transactions whose COMMIT fails without an answer from the database, as a broken connection does.
type failingCommit struct {
	db.TxStarter
	begun int
}

func (f *failingCommit) BeginTx(ctx context.Context, o pgx.TxOptions) (pgx.Tx, error) {
	tx, err := f.TxStarter.BeginTx(ctx, o)
	f.begun++
	return lostCommit{tx}, err
}

type lostCommit struct{ pgx.Tx }

func (l lostCommit) Commit(ctx context.Context) error {
	_ = l.Rollback(ctx)
	return errors.New("connection reset by peer")
}

// A COMMIT whose outcome is unknown is not retried and tells nothing: the hints would be about a write that may not exist.
func TestTxAnAmbiguousCommitIsNotRetriedAndSendsNothing(t *testing.T) {
	w := newHintTable(t)
	ctx := t.Context()
	starter := &failingCommit{TxStarter: w.h.pool}
	attempts := 0
	err := w.h.svc.txOn(ctx, starter, w.sessionID, func(tx pgx.Tx, h *hints) error {
		attempts++
		h.tell(w.campaignID, w.event("unsure", true))
		return w.write(ctx, tx, 1)
	})
	if _, ok := errors.AsType[*db.AmbiguousCommitError](err); !ok {
		t.Fatalf("tx() error = %v, want *db.AmbiguousCommitError", err)
	}
	if attempts != 1 || starter.begun != 1 {
		t.Errorf("fn ran %d times in %d transactions, want 1 and 1 (no retry)", attempts, starter.begun)
	}
	if got := w.heard(); len(got) != 0 {
		t.Errorf("the streams heard %q after a commit of unknown outcome", got)
	}
}

// Every tx takes the session row's lock before its closure runs, so the writes of one session take turns; a tx that told
// nothing takes it too but writes nothing.
func TestTxTakesTheSessionLockFirstAndWritesNothingWithoutHints(t *testing.T) {
	w := newHintTable(t)
	conn, err := pgx.ConnectConfig(t.Context(), w.h.pool.Config().ConnConfig)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = conn.Close(context.WithoutCancel(t.Context())) }()
	holder, err := conn.Begin(t.Context())
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = holder.Rollback(context.WithoutCancel(t.Context())) }()
	if _, err := holder.Exec(t.Context(), `SELECT id FROM game_sessions WHERE id = $1 FOR UPDATE`, w.sessionID); err != nil {
		t.Fatal(err)
	}
	var released atomic.Bool
	ran := make(chan bool, 1)
	done := make(chan error, 1)
	go func() {
		done <- w.h.svc.tx(t.Context(), w.sessionID, func(tx pgx.Tx, _ *hints) error {
			ran <- released.Load() // true only if the closure started after the holder let go
			return w.write(t.Context(), tx, 1)
		})
	}()
	// The one wait of the test: the closure must NOT have started while the lock is held elsewhere.
	select {
	case <-ran:
		t.Fatal("the closure ran while another transaction held the session row's lock")
	case <-time.After(300 * time.Millisecond):
	}
	released.Store(true)
	if err := holder.Commit(t.Context()); err != nil {
		t.Fatal(err)
	}
	if err := <-done; err != nil {
		t.Fatalf("tx() after the lock was released: %v", err)
	}
	if !<-ran {
		t.Error("the closure started before the lock was released")
	}
	if got := w.probe(t, 1); got != 1 {
		t.Errorf("the write did not happen (n = %d)", got)
	}
	if got := w.heard(); len(got) != 0 {
		t.Errorf("the streams heard %q", got)
	}
	if got := w.seq(t); got != 0 {
		t.Errorf("hint_seq = %d after a tx that told nothing, want 0", got)
	}
}

func TestTxTouchBumpsOnceAndSendsNothing(t *testing.T) {
	w := newHintTable(t)
	ctx := t.Context()
	err := w.h.svc.tx(ctx, w.sessionID, func(_ pgx.Tx, h *hints) error {
		h.touch()
		h.touch()
		return nil
	})
	if err != nil {
		t.Fatalf("tx() error = %v", err)
	}
	if got := w.seq(t); got != 1 {
		t.Errorf("hint_seq = %d after a touch, want 1", got)
	}
	if got := w.heard(); len(got) != 0 {
		t.Errorf("a touch sent %q", got)
	}
}

// The number counts transactions, not events and not what they say.
func TestTxBumpsOncePerTransactionWhateverItTold(t *testing.T) {
	w := newHintTable(t)
	ctx := t.Context()
	var got *hints
	err := w.h.svc.tx(ctx, w.sessionID, func(_ pgx.Tx, h *hints) error {
		got = h
		h.tell(w.campaignID, w.event("public", true))
		h.tell(w.campaignID, w.event("master-only", false))
		return nil
	})
	if err != nil {
		t.Fatalf("tx() error = %v", err)
	}
	if got := w.heard(); !slices.Equal(got, []string{"public", "master-only"}) {
		t.Errorf("the streams heard %q, want both events in the order told", got)
	}
	if got := w.seq(t); got != 1 {
		t.Errorf("hint_seq = %d after two events in one tx, want 1", got)
	}
	if got.seq != 1 {
		t.Errorf("the collector holds number %d, want 1", got.seq)
	}
	// One master-only event bumps the same as one public event.
	err = w.h.svc.tx(ctx, w.sessionID, func(_ pgx.Tx, h *hints) error {
		h.tell(w.campaignID, w.event("secret", false))
		return nil
	})
	if err != nil {
		t.Fatalf("tx() error = %v", err)
	}
	if got := w.seq(t); got != 2 {
		t.Errorf("hint_seq = %d, want 2", got)
	}
}

// The number does not depend on how much a transaction told: one event with a long body and one with a short one add 1 each.
func TestTxBumpsOnceWhateverTheSizeOfWhatItTold(t *testing.T) {
	w := newHintTable(t)
	ctx := t.Context()
	for i, body := range []string{"x", strings.Repeat("a long hidden body ", 4000)} {
		err := w.h.svc.tx(ctx, w.sessionID, func(_ pgx.Tx, h *hints) error {
			h.tell(w.campaignID, live.Event{
				Audience: live.Audience{Master: true},
				Message: &playv1.WatchGameSessionResponse{Event: &playv1.WatchGameSessionResponse_CombatLogChanged_{
					CombatLogChanged: &playv1.WatchGameSessionResponse_CombatLogChanged{EncounterId: body},
				}},
			})
			return nil
		})
		if err != nil {
			t.Fatalf("tx() error = %v", err)
		}
		if got := w.seq(t); got != int64(i+1) {
			t.Errorf("hint_seq = %d after a body of %d bytes, want %d", got, len(body), i+1)
		}
	}
}

// Four players and the master write one session at the same instant, each its own row, each telling an event, so every
// write adds 1 to the session's number. It measures the cost of that row: the attempts of the closure beyond one per
// write are the retries CockroachDB asked for. The control does the same writes telling nothing. The second pair reads
// the session row before it writes, as a handler that resolves the open session does. Service.tx takes the row's lock
// first in every row, so the control still locks it; what it leaves out is the bump. The last row does the first one's
// writes from a single writer, for the time of one write alone. The log gives the median and the 95th percentile of one
// write (the whole of tx, retries included). Read with
// go test -run TestHintSeqContention -v -count=N.
func TestHintSeqContentionOnCockroachDB(t *testing.T) {
	w := newHintTable(t)
	ctx := t.Context()
	// A statement only CockroachDB has: the contention of its serializable transactions is what is measured.
	var version string
	if err := w.h.pool.QueryRow(ctx, `SHOW CLUSTER SETTING version`).Scan(&version); err != nil {
		t.Fatalf("this test measures CockroachDB: %v", err)
	}
	const perWriter = 100
	// The test pool has one connection, which would make the writers take turns: a pool of their own over the same database.
	cfg := w.h.pool.Config().Copy()
	cfg.MaxConns = 6
	pool, err := pgxpool.NewWithConfig(ctx, cfg)
	if err != nil {
		t.Fatal(err)
	}
	defer pool.Close()
	svc := &Service{pool: pool, queries: playdb.New(pool), hub: w.h.svc.hub}
	run := func(name string, writers int, tell, readsSession bool) {
		var attempts atomic.Int64
		var failures atomic.Int64
		var wg sync.WaitGroup
		var mu sync.Mutex
		var took []time.Duration
		before := w.seq(t)
		for id := range writers {
			wg.Go(func() {
				for range perWriter {
					start := time.Now()
					err := svc.tx(ctx, w.sessionID, func(tx pgx.Tx, h *hints) error {
						attempts.Add(1)
						if readsSession { // most handlers read the open session first
							if _, err := svc.queries.WithTx(tx).GetHintSeq(ctx, w.sessionID); err != nil {
								return err
							}
						}
						if err := w.write(ctx, tx, id); err != nil {
							return err
						}
						if tell {
							h.tell(w.campaignID, w.event("write", true))
						}
						return nil
					})
					mu.Lock()
					took = append(took, time.Since(start))
					mu.Unlock()
					if err != nil {
						failures.Add(1)
						t.Errorf("%s: writer %d: a write failed after its retries: %v", name, id, err)
					}
				}
			})
		}
		wg.Wait()
		writes := int64(writers * perWriter)
		retries := attempts.Load() - writes
		want := before
		if tell {
			want += writes
		}
		if got := w.seq(t); got != want {
			t.Errorf("%s: hint_seq = %d, want %d", name, got, want)
		}
		slices.Sort(took)
		t.Logf("%s: %d writers, %d writes, %d failed, %d retries = %.1f per hundred writes, one write: median %.1f ms, p95 %.1f ms", name, writers, writes, failures.Load(), retries, float64(retries)*100/float64(writes),
			float64(took[len(took)/2].Microseconds())/1000, float64(took[len(took)*95/100].Microseconds())/1000)
		w.heard()
	}
	run("with the bump", 5, true, false)
	run("control, no bump", 5, false, false)
	run("with the bump, session row read first", 5, true, true)
	run("control, no bump, session row read first", 5, false, true)
	run("one writer, with the bump", 1, true, false)
}

// numbers drains the master's stream and returns the session number each event carries, by name.
func (w *hintTable) numbers() map[string]int64 {
	out := map[string]int64{}
	for {
		select {
		case ev := <-w.master.Events():
			w.master.Taken(ev)
			out[ev.Coalesce] = ev.Message.GetSessionSeq()
		default:
			return out
		}
	}
}

// A replay sends what the closure told, stamped with the number as it stands, and does not move it.
func TestTxReplaySendsWithoutMovingTheNumber(t *testing.T) {
	w := newHintTable(t)
	ctx := t.Context()
	if err := w.h.svc.tx(ctx, w.sessionID, func(_ pgx.Tx, h *hints) error { h.touch(); return nil }); err != nil {
		t.Fatalf("tx() error = %v", err)
	}
	err := w.h.svc.tx(ctx, w.sessionID, func(_ pgx.Tx, h *hints) error {
		h.replay()
		h.touch() // a replay wrote nothing: touching does not move the number
		h.tell(w.campaignID, w.event("told", true))
		h.after(func(_ context.Context, say func(string, live.Event)) { say(w.campaignID, w.event("after", false)) })
		return nil
	})
	if err != nil {
		t.Fatalf("tx() error = %v", err)
	}
	if got := w.seq(t); got != 1 {
		t.Errorf("hint_seq = %d after a replay, want 1: it wrote nothing", got)
	}
	if got, want := w.numbers(), map[string]int64{"told": 1, "after": 1}; !maps.Equal(got, want) {
		t.Errorf("the replay's events carry %v, want %v", got, want)
	}
	// A replay that tells nothing sends nothing and moves nothing.
	if err := w.h.svc.tx(ctx, w.sessionID, func(_ pgx.Tx, h *hints) error { h.replay(); return nil }); err != nil {
		t.Fatalf("tx() error = %v", err)
	}
	if got, heard := w.seq(t), w.heard(); got != 1 || len(heard) != 0 {
		t.Errorf("an empty replay left hint_seq %d and sent %q, want 1 and nothing", got, heard)
	}
}

// The steps of after run once the transaction committed, after the told events, in order, stamped with the new number; a
// failed transaction runs none, and a retried attempt keeps the steps of its last attempt only.
func TestTxAfterRunsAfterTheCommitInOrder(t *testing.T) {
	w := newHintTable(t)
	ctx := t.Context()
	var order []string
	attempts := 0
	err := w.h.svc.tx(ctx, w.sessionID, func(tx pgx.Tx, h *hints) error {
		attempts++
		round := attempts
		h.tell(w.campaignID, w.event("told", true))
		h.after(func(_ context.Context, say func(string, live.Event)) {
			order = append(order, fmt.Sprintf("first of attempt %d", round))
			say(w.campaignID, w.event("first", true))
		})
		h.after(func(context.Context, func(string, live.Event)) {
			order = append(order, fmt.Sprintf("second of attempt %d", round))
		})
		if round == 1 {
			return &pgconn.PgError{Code: "40001"} // the database asks for a retry
		}
		return w.write(ctx, tx, 1)
	})
	if err != nil {
		t.Fatalf("tx() error = %v", err)
	}
	if want := []string{"first of attempt 2", "second of attempt 2"}; !slices.Equal(order, want) {
		t.Errorf("the steps ran %q, want %q", order, want)
	}
	if got, want := w.numbers(), map[string]int64{"told": 1, "first": 1}; !maps.Equal(got, want) {
		t.Errorf("the events carry %v, want %v", got, want)
	}
	ran := false
	err = w.h.svc.tx(ctx, w.sessionID, func(_ pgx.Tx, h *hints) error {
		h.after(func(context.Context, func(string, live.Event)) { ran = true })
		return errors.New("refused")
	})
	if err == nil || ran {
		t.Errorf("a failed transaction returned %v and ran its step = %v, want an error and no step", err, ran)
	}
}
