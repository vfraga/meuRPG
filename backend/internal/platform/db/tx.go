package db

import (
	"context"
	"fmt"
	"time"

	"github.com/cockroachdb/cockroach-go/v2/crdb"
	crdbpgx "github.com/cockroachdb/cockroach-go/v2/crdb/crdbpgxv5"
	"github.com/jackc/pgx/v5"
)

// TxStarter is anything that can begin a transaction. Both *pgxpool.Pool and
// *pgx.Conn satisfy it, and so can a fake in tests.
type TxStarter = crdbpgx.Conn

// defaultRetryPolicy waits 10ms, 20ms, 40ms... (capped at 500ms) between
// attempts and gives up after 8 retries (about 1.6s of waiting in total).
// Backing off, instead of retrying immediately, gives the transaction we
// collided with time to finish.
var defaultRetryPolicy crdb.RetryPolicy = &crdb.ExpBackoffRetryPolicy{
	RetryLimit: 8,
	BaseDelay:  10 * time.Millisecond,
	MaxDelay:   500 * time.Millisecond,
}

// InTx runs fn inside a transaction and commits it when fn returns nil.
//
// Why retries: CockroachDB always runs transactions with SERIALIZABLE
// isolation. When two transactions touch the same rows concurrently, it may
// abort one of them with SQLSTATE 40001 ("restart transaction") instead of
// letting them produce an inconsistent result. The fix is simply to run the
// transaction again, and InTx does that for you, using the official
// cockroach-go helper.
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
	ctx = crdb.WithRetryPolicy(ctx, policy)
	return crdbpgx.ExecuteTx(ctx, conn, pgx.TxOptions{}, fn)
}

// ReadTx runs fn inside one read-only transaction, so every statement of fn
// sees the same snapshot of the database. Use it for a read RPC that runs two
// or more queries and puts their results in one response: as separate
// autocommit statements, each picks its own timestamp, and a write committed
// between them would give a view that mixes two states (audit D-03).
//
// Why not AS OF SYSTEM TIME follower_read_timestamp(): it reads data a few
// seconds old, and a live game must show what the table just did (a player who
// moved a token must not see it back where it was). A read-only SERIALIZABLE
// transaction in CockroachDB reads at its start time and sees every write
// committed before it, and it never blocks writers. It can still be asked to
// restart (40001), so it goes through the same retry helper as InTx: fn may
// run more than once, and must follow the same rules (only tx, %w on errors,
// no side effects). Inside fn use only the transaction's queries
// (queries.WithTx(tx)), never the pool: that is a second connection.
func ReadTx(ctx context.Context, conn TxStarter, fn func(pgx.Tx) error) error {
	return readTx(ctx, conn, defaultRetryPolicy, fn)
}

func readTx(ctx context.Context, conn TxStarter, policy crdb.RetryPolicy, fn func(pgx.Tx) error) error {
	ctx = crdb.WithRetryPolicy(ctx, policy)
	// The READ ONLY option of BEGIN is not enough: cockroach-go restarts after
	// a 40001 with ROLLBACK and a plain BEGIN, so the retry would run read-write.
	// SET TRANSACTION as the first statement of every attempt keeps it read-only.
	return crdbpgx.ExecuteTx(ctx, conn, pgx.TxOptions{AccessMode: pgx.ReadOnly}, func(tx pgx.Tx) error {
		if _, err := tx.Exec(ctx, "SET TRANSACTION READ ONLY"); err != nil {
			return fmt.Errorf("set the transaction read only: %w", err)
		}
		return fn(tx)
	})
}
