package play

import (
	"context"
	"errors"
	"fmt"
	"sync"
	"testing"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"

	playv1 "github.com/PuraFome/meuRPG/backend/gen/meurpg/play/v1"
	"github.com/PuraFome/meuRPG/backend/internal/platform/dbtest"
	"github.com/PuraFome/meuRPG/backend/internal/rules/combat"
)

// A spend racing the master's answer of the refund prompt (RN-43: one row per character). The prompt's roll left Toren with nothing, and
// the master gives it back while his player spends again. Either the spend came first (it finds nothing: HAS_NONE, and the refund
// returns the one row) or the refund did (the spend takes the returned row: none left). Never two rows, never the Inspiração returned twice.
//
// How the two are ordered: Service.tx locks the session's row first, so the transactions take turns. But a SERIALIZABLE transaction takes
// its snapshot at its first statement, which is that lock: a spend that waited for the answer holds a snapshot from before the answer, and
// when it deletes the row the answer inserted the database refuses it with 40001 (could not serialize access due to concurrent delete). That
// is no failure of the product: the closure of every call returns the error, Service.tx runs the closure again on a fresh snapshot, and the
// second run reads the row gone (HAS_NONE). So the test spends through that retry (tryFavorLocked) and asserts the end state; any error that
// is not the product's answer fails with the side that returned it and its SQLSTATE (failureOf), and the test retries nothing of its own.
func TestASpendAndTheAnswerOfARefundPromptRace(t *testing.T) {
	t.Parallel()
	dbtest.PoolSize(t, 8)
	for range 4 {
		f, p := refundFight(t)
		const spenders = 4
		spent := make([]spendTry, spenders)
		var ansErr error
		var start, wg sync.WaitGroup
		start.Add(1)
		wg.Go(func() { start.Wait(); _, ansErr = f.answerRefund(f.master, p.GetId(), true, newKey()) })
		for i := range spenders {
			wg.Go(func() { start.Wait(); spent[i] = f.tryFavorLocked(f.ask(combat.ModeNormal)) })
		}
		start.Done()
		wg.Wait()
		if ansErr != nil {
			t.Fatalf("%s", failureOf("the master's answer", ansErr))
		}
		won := spendsThatWon(t, spent)
		if won > 1 {
			t.Errorf("%d spends took the one returned Inspiração, want at most 1", won)
		}
		if rows := f.rows(t); rows != 1-won {
			t.Errorf("%d rows after %d spends, want %d: the refund returns one and a spend takes it", rows, won, 1-won)
		}
		if got := f.eventsOf(t, eventFavorRefunded); got != 1 {
			t.Errorf("%d refunded events, want 1", got)
		}
		if status := f.promptStatus(t, p.GetId()); status != "returned" {
			t.Errorf("the prompt is %q, want returned: the master's answer is the one that always wins its own prompt", status)
		}
	}
}

// An accept racing the master's undo of the grant that the pass is about (the grant is the combat's last action, the ask changed nothing of
// the log). Either the accept won (the Inspiração moved to Pensantus, and the undo, whose last action is now the pass, no longer matches)
// or the undo did (Toren's row is gone, the ask is canceled, the accept reads it closed). Never a row moved by an undone grant.
func TestAnAcceptAndTheUndoOfTheGrantRace(t *testing.T) {
	t.Parallel()
	dbtest.PoolSize(t, 8)
	for range 4 {
		f := newFavorFight(t)
		f.give(t, f.toren)
		p := f.mustAsk(t, f.toren.GetId(), f.pens.GetId())
		line := f.undoableLine(t).GetId()
		var accErr, undoErr error
		var start, wg sync.WaitGroup
		start.Add(1)
		wg.Go(func() { start.Wait(); _, accErr = f.answer(p.GetId(), true) })
		wg.Go(func() { start.Wait(); undoErr = f.undo(t, f.master, f.e, line) })
		start.Done()
		wg.Wait()
		if rows := f.rows(t); rows > 1 {
			t.Fatalf("%d rows, want at most one", rows)
		}
		switch {
		case accErr == nil && undoErr != nil:
			if !f.has(t, f.pens.GetId()) || f.has(t, f.toren.GetId()) {
				t.Errorf("the accept won but Toren has it %v and Pensantus %v", f.has(t, f.toren.GetId()), f.has(t, f.pens.GetId()))
			}
		case accErr != nil && undoErr == nil:
			wantFavorBlocked(t, "the accept that lost", accErr, blockedPassClosed)
			if f.rows(t) != 0 || f.passStatus(t, p.GetId()) != "canceled" {
				t.Errorf("the undo won but there are %d rows and the ask is %q, want none and canceled", f.rows(t), f.passStatus(t, p.GetId()))
			}
		default:
			t.Fatalf("accept error = %v and undo error = %v, want exactly one to win", accErr, undoErr)
		}
	}
}

// spendsThatWon counts the spends that took the Inspiração; any other answer must be HAS_NONE.
func spendsThatWon(t *testing.T, spent []spendTry) int {
	t.Helper()
	won := 0
	for i, s := range spent {
		switch {
		case s.use.Spent && s.err == nil:
			won++
		case !isBlockedBy(s.err, playv1.FavorBlockedReason_FAVOR_BLOCKED_REASON_HAS_NONE):
			t.Errorf("%s, want spent or HAS_NONE (the spend = %+v)", failureOf(fmt.Sprintf("spend %d", i), s.err), s.use)
		}
	}
	return won
}

// tryFavorLocked is tryFavor on the transaction every call of the service runs in (Service.tx), and with the closure of those calls: it takes
// the spend and returns its error, as the closure of RollAttack or of a check does, so Service.tx runs it again when the database refuses it
// with 40001 and returns any other error as it is (the product's answer, such as HAS_NONE). The spend of the attempt that settled is what it
// reports. Nothing is repeated by the test itself.
func (f *favorParty) tryFavorLocked(a favorAsk) spendTry {
	var out spendTry
	out.err = f.h.svc.tx(context.Background(), f.sessionID, func(tx pgx.Tx, _ *hints) error {
		use, err := f.h.svc.useFavor(context.Background(), f.h.svc.outsideGate(tx, f.h.svc.queries.WithTx(tx), nil), a)
		out.use = use
		return err
	})
	return out
}

// failureOf says which side of the race returned the error and its SQLSTATE (or that it has none), so that a failure read in a CI log
// names the side and the database's own code: a serialization failure (40001), a deadlock (40P01) or the service's own refusal.
func failureOf(side string, err error) string {
	if err == nil {
		return side + " returned no error"
	}
	var pgErr *pgconn.PgError
	if errors.As(err, &pgErr) {
		return fmt.Sprintf("%s returned an error with SQLSTATE %s: %v", side, pgErr.Code, err)
	}
	return fmt.Sprintf("%s returned an error with no SQLSTATE: %v", side, err)
}
