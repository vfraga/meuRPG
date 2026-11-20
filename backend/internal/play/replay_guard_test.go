package play

import (
	"context"
	"errors"
	"slices"
	"testing"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"

	"github.com/PuraFome/meuRPG/backend/internal/play/live"
	"github.com/PuraFome/meuRPG/backend/internal/play/playdb"
)

// The savepoint of a replay (releaseHold, guardReplay) undoes a refusal and nothing else.

// A serialization failure raised inside the replay is no refusal: settleReplay hands it back untouched, the savepoint is not rolled back
// by it, and the transaction loop runs the whole change again (SQLSTATE 40001 on CockroachDB, 40P01 on PostgreSQL both reach it the same way).
func TestASerializationFailureInAReplayIsNotSwallowedByItsSavepoint(t *testing.T) {
	w := newHintTable(t)
	ctx := t.Context()
	attempts := 0
	var first error
	err := w.h.svc.tx(ctx, w.sessionID, func(tx pgx.Tx, h *hints) error {
		attempts++
		c := &combatTx{tx: tx, q: playdb.New(tx), hints: h}
		g, err := guardReplay(ctx, c)
		if err != nil {
			return err
		}
		if err := w.write(ctx, tx, 1); err != nil {
			return err
		}
		if attempts == 1 {
			first = w.h.svc.settleReplay(ctx, c, playdb.ReactionHold{}, nil, &replayState{}, g, &pgconn.PgError{Code: "40001", Message: "restart transaction"})
			return first
		}
		return g.keep(ctx)
	})
	var pg *pgconn.PgError
	if !errors.As(first, &pg) || pg.Code != "40001" {
		t.Fatalf("settleReplay(40001) = %v, want the error untouched", first)
	}
	if err != nil || attempts != 2 {
		t.Fatalf("tx() = %v after %d attempt(s), want the retry (2 attempts) and success", err, attempts)
	}
	if got := w.probe(t, 1); got != 1 {
		t.Errorf("the probe row has n = %d, want 1: only the second attempt's write stands", got)
	}
}

// What a replay queued in memory goes with it when it is refused: the hints, the log lines and the other queues, and the rows it wrote.
func TestARefusedReplayTakesWhatItQueuedWithIt(t *testing.T) {
	w := newHintTable(t)
	ctx := t.Context()
	err := w.h.svc.tx(ctx, w.sessionID, func(tx pgx.Tx, h *hints) error {
		c := &combatTx{tx: tx, q: playdb.New(tx), hints: h, tellIDs: []string{"a"}, replay: &replayState{}}
		h.tell(w.campaignID, w.event("before", true))
		g, err := guardReplay(ctx, c)
		if err != nil {
			return err
		}
		if err := w.write(ctx, tx, 2); err != nil {
			return err
		}
		h.tell(w.campaignID, w.event("replayed", true))
		h.touch()
		h.after(func(context.Context, func(string, live.Event)) {})
		c.lines = append(c.lines, actionEvent{})
		c.tellIDs = append(c.tellIDs, "b")
		c.pushed = append(c.pushed, &forcedMove{})
		c.castsChanged = castsPublic
		if err := g.undo(ctx, c); err != nil {
			return err
		}
		if len(c.lines) != 0 || len(c.pushed) != 0 || !slices.Equal(c.tellIDs, []string{"a"}) || c.castsChanged != 0 || c.replay == nil {
			t.Errorf("the change after the undo = lines %d, pushed %d, tellIDs %v, casts %d, replay %v; want what it was", len(c.lines), len(c.pushed), c.tellIDs, c.castsChanged, c.replay)
		}
		if len(h.events) != 1 || len(h.afters) != 0 || h.touched {
			t.Errorf("the hints after the undo = %d event(s), %d step(s), touched %v; want the one told before", len(h.events), len(h.afters), h.touched)
		}
		return w.write(ctx, tx, 1)
	})
	if err != nil {
		t.Fatalf("tx() error = %v", err)
	}
	if got := w.probe(t, 2); got != -1 {
		t.Errorf("the refused replay's row stands (n = %d)", got)
	}
	if got := w.heard(); !slices.Equal(got, []string{"before"}) {
		t.Errorf("the streams heard %q, want only what was told before the replay", got)
	}
}
