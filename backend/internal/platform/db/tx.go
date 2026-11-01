package db

import (
	"context"
	"errors"
	"time"

	"github.com/cockroachdb/cockroach-go/v2/crdb"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
)

// TxStarter is anything that can begin a transaction. Both *pgxpool.Pool and
// *pgx.Conn satisfy it, and so can a fake in tests.
type TxStarter interface {
	Begin(context.Context) (pgx.Tx, error)
	BeginTx(context.Context, pgx.TxOptions) (pgx.Tx, error)
}

// defaultRetryPolicy is cockroach-go's policy type: it counts the attempts and
// ends them with *crdb.MaxRetriesExceededError.
// It waits 10ms, 20ms, 40ms... (capped at 500ms) between attempts.
// It gives up after 8 retries (about 1.6s of waiting in total).
// Backing off, instead of retrying immediately, gives the transaction we
// collided with time to finish.
var defaultRetryPolicy crdb.RetryPolicy = &crdb.ExpBackoffRetryPolicy{
	RetryLimit: 8,
	BaseDelay:  10 * time.Millisecond,
	MaxDelay:   500 * time.Millisecond,
}

// InTx runs fn inside a transaction and commits it when fn returns nil.
//
// Why retries: every transaction begins with ISOLATION LEVEL SERIALIZABLE,
// which is all CockroachDB has and is asked for explicitly on PostgreSQL.
// When two transactions touch the same rows concurrently, the
// database may abort one of them with SQLSTATE 40001 ("restart transaction"),
// or 40P01 on a deadlock, instead of letting them produce an inconsistent
// result. The fix is simply to run the transaction again, and InTx does that
// for you.
//
// Every attempt is a new transaction that begins with the same options, and
// the COMMIT's own error is read: CockroachDB reports a serialization failure
// while the statements run or at COMMIT, PostgreSQL mostly at COMMIT, and
// both are retried. A COMMIT that fails without an answer from the server
// (the connection broke) may or may not have been saved: InTx returns
// *AmbiguousCommitError and does not retry.
//
// Rules for fn, because it may run more than once:
//   - Talk to the database only through tx.
//   - No side effects outside the database (sending e-mails, calling other
//     APIs, publishing events). Do those after InTx returns.
//   - When adding context to an error, wrap it with %w
//     (fmt.Errorf("insert character: %w", err)); otherwise InTx cannot see
//     the 40001 code and will not retry.
//
// Any other error rolls the transaction back and is returned as is.
func InTx(ctx context.Context, conn TxStarter, fn func(pgx.Tx) error) error {
	return inTx(ctx, conn, defaultRetryPolicy, fn)
}

// inTx is InTx with a configurable retry policy, so tests can use a policy
// without delays.
func inTx(ctx context.Context, conn TxStarter, policy crdb.RetryPolicy, fn func(pgx.Tx) error) error {
	return runTx(ctx, conn, pgx.TxOptions{}, policy, fn)
}

// ReadTx runs fn inside one read-only transaction, so every statement of fn
// sees the same snapshot of the database. Use it for a read RPC that runs two
// or more queries and puts their results in one response: as separate
// autocommit statements, each picks its own timestamp, and a write committed
// between them would give a view that mixes two states.
//
// Why not AS OF SYSTEM TIME follower_read_timestamp(): it reads data a few
// seconds old, and a live game must show what the table just did (a player who
// moved a token must not see it back where it was). A read-only SERIALIZABLE
// transaction in CockroachDB reads at its start time and sees every write
// committed before it, and it never blocks writers. It can still be asked to
// restart (40001), so it goes through the same retry loop as InTx: fn may
// run more than once, and must follow the same rules (only tx, %w on errors,
// no side effects). Inside fn use only the transaction's queries
// (queries.WithTx(tx)), never the pool: that is a second connection.
func ReadTx(ctx context.Context, conn TxStarter, fn func(pgx.Tx) error) error {
	return readTx(ctx, conn, defaultRetryPolicy, fn)
}

func readTx(ctx context.Context, conn TxStarter, policy crdb.RetryPolicy, fn func(pgx.Tx) error) error {
	return runTx(ctx, conn, pgx.TxOptions{AccessMode: pgx.ReadOnly}, policy, fn)
}

// Postgres and CockroachDB agree on these SQLSTATEs.
const (
	sqlstateSerialization = "40001"
	sqlstateDeadlock      = "40P01"
	sqlstateUnknownCommit = "40003" // CockroachDB: statement completion unknown
)

// AmbiguousCommitError is a COMMIT that failed without an answer from the
// database: the transaction may or may not have been saved.
type AmbiguousCommitError struct{ cause error }

func (e *AmbiguousCommitError) Error() string {
	return "the commit's outcome is unknown: " + e.cause.Error()
}

func (e *AmbiguousCommitError) Unwrap() error { return e.cause }

func retryable(err error) bool {
	pgErr, ok := errors.AsType[*pgconn.PgError](err)
	return ok && (pgErr.Code == sqlstateSerialization || pgErr.Code == sqlstateDeadlock)
}

// runTx runs fn in a new transaction with opts, again after each retryable
// failure, until the policy gives up.
func runTx(ctx context.Context, conn TxStarter, opts pgx.TxOptions, policy crdb.RetryPolicy, fn func(pgx.Tx) error) error {
	// Asked for in every BEGIN, so no server default can weaken it: the code
	// relies on SERIALIZABLE (a count checked and then written, say).
	opts.IsoLevel = pgx.Serializable
	retry := policy.NewRetry()
	for {
		err := attempt(ctx, conn, opts, fn)
		if err == nil || !retryable(err) {
			return err
		}
		delay, retryErr := retry(err)
		if retryErr != nil {
			return retryErr
		}
		select {
		case <-time.After(delay):
		case <-ctx.Done():
			return errors.Join(ctx.Err(), err)
		}
	}
}

// attempt runs fn once in a transaction of its own and commits it. A panic,
// or a test's t.Fatal (runtime.Goexit), in fn still rolls the transaction
// back, so the connection goes back to the pool.
func attempt(ctx context.Context, conn TxStarter, opts pgx.TxOptions, fn func(pgx.Tx) error) error {
	tx, err := conn.BeginTx(ctx, opts)
	if err != nil {
		return err
	}
	finished := false
	defer func() {
		if !finished {
			rollback(ctx, tx)
		}
	}()
	if err := runAttempt(tx, fn); err != nil {
		finished = true
		rollback(ctx, tx)
		return err
	}
	finished = true
	err = tx.Commit(ctx)
	if err == nil || retryable(err) {
		return err
	}
	if pgErr, ok := errors.AsType[*pgconn.PgError](err); ok && pgErr.Code != sqlstateUnknownCommit {
		return err // the database answered: the transaction was not saved
	}
	if errors.Is(err, pgx.ErrTxCommitRollback) || errors.Is(err, pgx.ErrTxClosed) {
		return err // PostgreSQL answered COMMIT with ROLLBACK: the transaction was not saved
	}
	return &AmbiguousCommitError{cause: err}
}

// runAttempt is the frame dbtest's nested-acquisition guard looks for: fn
// runs under it, and nothing else does.
//
//go:noinline
func runAttempt(tx pgx.Tx, fn func(pgx.Tx) error) error { return fn(tx) }

// rollback ends tx even when ctx is already canceled, but never waits on a hung connection for long.
func rollback(ctx context.Context, tx pgx.Tx) {
	ctx, cancel := context.WithTimeout(context.WithoutCancel(ctx), 5*time.Second)
	defer cancel()
	_ = tx.Rollback(ctx)
}
