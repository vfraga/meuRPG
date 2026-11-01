package db

import (
	"context"
	"errors"
	"fmt"
	"runtime"
	"slices"
	"testing"

	"github.com/cockroachdb/cockroach-go/v2/crdb"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
)

// These tests exercise the retry loop without a database: fakeConn hands out
// a fakeTx that records the calls the loop makes. The integration tests in
// db_integration_test.go cover the same behavior against real CockroachDB.

// fakeTx implements pgx.Tx. Embedding the interface satisfies the methods we
// don't override; calling one of those panics, which is what we want in a
// test because it means the helper did something unexpected.
type fakeTx struct {
	pgx.Tx

	committed  bool
	rolledBack bool
	commits    int
	rollbacks  int
	// commitErrs are the answers of the successive COMMITs (nil when it runs out).
	commitErrs []error
}

func (f *fakeTx) Commit(context.Context) error {
	f.commits++
	var err error
	if len(f.commitErrs) > 0 {
		err, f.commitErrs = f.commitErrs[0], f.commitErrs[1:]
	}
	f.committed = err == nil
	return err
}

func (f *fakeTx) Rollback(context.Context) error {
	f.rolledBack = true
	f.rollbacks++
	return nil
}

type fakeConn struct {
	tx *fakeTx
	// opts are the options of each BEGIN.
	opts []pgx.TxOptions
}

func (c *fakeConn) Begin(context.Context) (pgx.Tx, error) { return c.tx, nil }

func (c *fakeConn) BeginTx(_ context.Context, opts pgx.TxOptions) (pgx.Tx, error) {
	c.opts = append(c.opts, opts)
	return c.tx, nil
}

// errSerializationFailure is the error pgx returns when CockroachDB aborts a
// transaction that must be retried.
var errSerializationFailure = &pgconn.PgError{Code: "40001", Message: "restart transaction"}

func TestInTxRetriesSerializationFailures(t *testing.T) {
	t.Parallel()

	conn := &fakeConn{tx: &fakeTx{}}
	attempts := 0

	err := InTx(t.Context(), conn, func(pgx.Tx) error {
		attempts++
		if attempts < 3 {
			// Wrapped on purpose: the helper must look through %w.
			return fmt.Errorf("update character: %w", errSerializationFailure)
		}
		return nil
	})
	if err != nil {
		t.Fatalf("InTx() error = %v, want nil", err)
	}

	if attempts != 3 {
		t.Errorf("fn ran %d times, want 3", attempts)
	}
	// Each retry is a new transaction: two rolled back, the third committed.
	if len(conn.opts) != 3 || conn.tx.rollbacks != 2 || conn.tx.commits != 1 || !conn.tx.committed {
		t.Errorf("begins = %d, rollbacks = %d, commits = %d; want 3, 2, 1", len(conn.opts), conn.tx.rollbacks, conn.tx.commits)
	}
}

// PostgreSQL reports a serialization failure at COMMIT: it is retried, and
// the transaction is not reported as saved.
func TestInTxRetriesAFailureAtCommit(t *testing.T) {
	t.Parallel()

	conn := &fakeConn{tx: &fakeTx{commitErrs: []error{errSerializationFailure, &pgconn.PgError{Code: "40P01"}}}}
	attempts := 0

	err := InTx(t.Context(), conn, func(pgx.Tx) error {
		attempts++
		return nil
	})
	if err != nil {
		t.Fatalf("InTx() error = %v, want nil", err)
	}
	if attempts != 3 || conn.tx.commits != 3 || !conn.tx.committed {
		t.Errorf("attempts = %d, commits = %d, committed = %v; want 3, 3, true", attempts, conn.tx.commits, conn.tx.committed)
	}
}

// A COMMIT that keeps failing with 40001 ends as an error, never as success.
func TestInTxDoesNotReportALostCommitAsSuccess(t *testing.T) {
	t.Parallel()

	failures := []error{errSerializationFailure, errSerializationFailure, errSerializationFailure}
	conn := &fakeConn{tx: &fakeTx{commitErrs: failures}}
	policy := &crdb.LimitBackoffRetryPolicy{RetryLimit: 2}

	err := inTx(t.Context(), conn, policy, func(pgx.Tx) error { return nil })

	if _, ok := errors.AsType[*crdb.MaxRetriesExceededError](err); !ok {
		t.Fatalf("InTx() error = %v, want *crdb.MaxRetriesExceededError", err)
	}
}

func TestInTxCommitFailures(t *testing.T) {
	t.Parallel()

	answered := &pgconn.PgError{Code: "23505", Message: "duplicate key"}
	broken := errors.New("write: connection reset")
	unknown := &pgconn.PgError{Code: "40003", Message: "statement completion unknown"}
	tests := []struct {
		name      string
		commitErr error
		ambiguous bool
	}{
		{"the database answered with a constraint error", answered, false},
		{"the connection broke", broken, true},
		{"CockroachDB does not know", unknown, true},
		{"PostgreSQL answered COMMIT with ROLLBACK", pgx.ErrTxCommitRollback, false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			conn := &fakeConn{tx: &fakeTx{commitErrs: []error{tt.commitErr}}}
			attempts := 0
			err := InTx(t.Context(), conn, func(pgx.Tx) error { attempts++; return nil })
			if !errors.Is(err, tt.commitErr) || attempts != 1 {
				t.Fatalf("InTx() error = %v after %d attempts, want %v after 1", err, attempts, tt.commitErr)
			}
			_, ambiguous := errors.AsType[*AmbiguousCommitError](err)
			if ambiguous != tt.ambiguous {
				t.Errorf("ambiguous = %v, want %v", ambiguous, tt.ambiguous)
			}
		})
	}
}

// Every attempt of a read-only transaction begins READ ONLY, so a retry
// cannot turn into a read-write transaction.
func TestReadTxBeginsReadOnlyOnEveryAttempt(t *testing.T) {
	t.Parallel()

	conn := &fakeConn{tx: &fakeTx{}}
	attempts := 0
	err := ReadTx(t.Context(), conn, func(pgx.Tx) error {
		attempts++
		if attempts < 3 {
			return fmt.Errorf("read: %w", errSerializationFailure)
		}
		return nil
	})
	if err != nil {
		t.Fatalf("ReadTx() error = %v", err)
	}
	if len(conn.opts) != 3 {
		t.Fatalf("began %d transactions, want 3", len(conn.opts))
	}
	for i, o := range conn.opts {
		if o.IsoLevel != pgx.Serializable {
			t.Errorf("attempt %d began with isolation %q, want serializable", i+1, o.IsoLevel)
		}
		if o.AccessMode != pgx.ReadOnly {
			t.Errorf("attempt %d began with access mode %q, want read only", i+1, o.AccessMode)
		}
	}
}

// A test that fails inside the closure (runtime.Goexit) still rolls back.
func TestInTxRollsBackWhenFnExits(t *testing.T) {
	t.Parallel()

	conn := &fakeConn{tx: &fakeTx{}}
	done := make(chan struct{})
	go func() {
		defer close(done)
		_ = InTx(context.Background(), conn, func(pgx.Tx) error {
			runtime.Goexit()
			return nil
		})
	}()
	<-done
	if !conn.tx.rolledBack || conn.tx.committed {
		t.Errorf("committed = %v, rolledBack = %v; want rolled back only", conn.tx.committed, conn.tx.rolledBack)
	}
}

func TestInTxDoesNotRetryOtherErrors(t *testing.T) {
	t.Parallel()

	conn := &fakeConn{tx: &fakeTx{}}
	errNotFound := errors.New("character not found")
	attempts := 0

	err := InTx(t.Context(), conn, func(pgx.Tx) error {
		attempts++
		return errNotFound
	})

	if !errors.Is(err, errNotFound) {
		t.Fatalf("InTx() error = %v, want %v", err, errNotFound)
	}
	if attempts != 1 {
		t.Errorf("fn ran %d times, want 1", attempts)
	}
	if !conn.tx.rolledBack || conn.tx.committed {
		t.Errorf("committed = %v, rolledBack = %v; want rolled back only", conn.tx.committed, conn.tx.rolledBack)
	}
}

func TestInTxGivesUpAfterRetryLimit(t *testing.T) {
	t.Parallel()

	conn := &fakeConn{tx: &fakeTx{}}
	policy := &crdb.LimitBackoffRetryPolicy{RetryLimit: 2} // no delay, keeps the test fast
	attempts := 0

	err := inTx(t.Context(), conn, policy, func(pgx.Tx) error {
		attempts++
		return errSerializationFailure
	})

	if _, ok := errors.AsType[*crdb.MaxRetriesExceededError](err); !ok {
		t.Fatalf("InTx() error = %v, want *crdb.MaxRetriesExceededError", err)
	}
	if !errors.Is(err, errSerializationFailure) {
		t.Errorf("InTx() error = %v, want it to wrap the original 40001", err)
	}
	if attempts != 3 { // the first try + 2 retries
		t.Errorf("fn ran %d times, want 3", attempts)
	}
	if !conn.tx.rolledBack {
		t.Error("transaction was not rolled back")
	}
}

func TestDefaultRetryPolicyBacksOff(t *testing.T) {
	t.Parallel()

	next := defaultRetryPolicy.NewRetry()
	var delays []int64
	for {
		delay, err := next(errSerializationFailure)
		if err != nil {
			break
		}
		delays = append(delays, delay.Milliseconds())
	}

	want := []int64{10, 20, 40, 80, 160, 320, 500, 500}
	if !slices.Equal(delays, want) {
		t.Errorf("retry delays (ms) = %v, want %v", delays, want)
	}
}
