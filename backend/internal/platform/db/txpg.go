package db

// EXPERIMENT (exp/postgres-tests): the transaction loop for PostgreSQL.
//
// cockroach-go ends a transaction with RELEASE SAVEPOINT cockroach_restart and
// then COMMIT, and it throws the COMMIT's error away: on CockroachDB the
// RELEASE is where a serialization failure shows. On PostgreSQL it shows at
// COMMIT, so the helper would report success for a transaction that was not
// saved. This loop commits itself, reads the error, and starts every attempt
// with the same BEGIN options.

import (
	"context"
	"errors"
	"sync/atomic"
	"time"

	"github.com/cockroachdb/cockroach-go/v2/crdb"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
)

var postgresRetries atomic.Bool

// UsePostgresRetries switches InTx and ReadTx to the loop above.
func UsePostgresRetries(on bool) { postgresRetries.Store(on) }

func retryable(err error) bool {
	pgErr, ok := errors.AsType[*pgconn.PgError](err)
	return ok && (pgErr.Code == "40001" || pgErr.Code == "40P01")
}

// runAttempt is the frame dbtest's nested-acquisition guard looks for.
//
//go:noinline
func runAttempt(tx pgx.Tx, fn func(pgx.Tx) error) error { return fn(tx) }

func inTxLoop(ctx context.Context, conn TxStarter, opts pgx.TxOptions, policy crdb.RetryPolicy, fn func(pgx.Tx) error) error {
	retry := policy.NewRetry()
	for {
		tx, err := conn.BeginTx(ctx, opts)
		if err != nil {
			return err
		}
		err = runAttempt(tx, fn)
		if err == nil {
			err = tx.Commit(ctx)
		} else {
			_ = tx.Rollback(ctx)
		}
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
			return ctx.Err()
		}
	}
}
