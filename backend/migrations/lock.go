package migrations

import (
	"context"
	"crypto/rand"
	"database/sql"
	"encoding/hex"
	"errors"
	"fmt"
	"log/slog"
	"time"
)

// A lease lock for `migrate up` and `migrate down` (audit D-08).
//
// Why: goose has no lock for CockroachDB (its PostgreSQL lock needs advisory
// locks, which CockroachDB does not have), and CockroachDB DDL is not atomic
// (see the package comment). Two runs at once, say two deploys, could
// interleave the statements of a migration. So a run takes a lease first:
//
//   - The lease is the one row of migration_lock. Taking it is one INSERT ...
//     ON CONFLICT DO UPDATE ... WHERE expired statement, which succeeds for
//     exactly one runner.
//   - The lease expires. A run that crashed leaves a row that the next run may
//     take over once expires_at has passed, so nobody is blocked for ever.
//   - A live run renews the lease every ttl/3 (the heartbeat). If it cannot (it
//     lost the lease), the context it works with is canceled, so goose stops
//     between statements instead of racing the new owner.
//   - Every time is the database's now(), never a runner's clock, so skew
//     between runners does not matter. (The run's own deadline is a local one:
//     it stops renewing, and cancels, before the lease can end.)
//
// Canceling stops goose between statements, not a statement already sent: the
// schema-change job CockroachDB started for it goes on in the background, and
// no lock can hold that back.
//
// The table is created here, before goose, and is not a numbered migration: the
// lock must exist before the first migration runs, and a migration that created
// it would run without it.

// LockOptions tune the lease. The zero value gives the production defaults.
type LockOptions struct {
	// TTL is how long a lease lasts without a heartbeat (default 60 s). It must
	// be longer than the longest pause of a healthy run, and short enough that a
	// crashed run does not hold up a deploy for long.
	TTL time.Duration
	// Wait is how long to wait for another run to finish (default 10 minutes).
	Wait time.Duration
	// Poll is how often to try again while waiting (default 1 s).
	Poll time.Duration
	// Logger says who holds the lease while waiting. Optional.
	Logger *slog.Logger
}

func (o LockOptions) withDefaults() LockOptions {
	if o.TTL <= 0 {
		o.TTL = 60 * time.Second
	}
	if o.Wait <= 0 {
		o.Wait = 10 * time.Minute
	}
	if o.Poll <= 0 {
		o.Poll = time.Second
	}
	if o.Logger == nil {
		o.Logger = slog.New(slog.DiscardHandler)
	}
	return o
}

// ErrLockTimeout says another run held the lock for the whole wait.
var ErrLockTimeout = errors.New("another migrate run holds the migration lock")

// AcquireLock takes the migration lock, waiting for another run to finish. It
// returns a context to run the migrations with, which is canceled if the lease
// is lost, and a release function to call when done (it is safe to call twice).
func AcquireLock(ctx context.Context, db *sql.DB, opt LockOptions) (context.Context, func(), error) {
	opt = opt.withDefaults()
	if err := createLockTable(ctx, db); err != nil {
		return nil, nil, err
	}
	holder, err := newHolderID()
	if err != nil {
		return nil, nil, err
	}

	var leaseFrom time.Time
	deadline := time.Now().Add(opt.Wait)
	for {
		// Taken before the statement: the lease the database grants ends no
		// earlier than this plus the TTL, whatever the statement's latency.
		attempt := time.Now()
		got, err := tryAcquire(ctx, db, holder, opt.TTL)
		if err != nil {
			return nil, nil, err
		}
		if got {
			leaseFrom = attempt
			break
		}
		if time.Now().After(deadline) {
			return nil, nil, fmt.Errorf("%w (waited %s); if no run is going, the lease ends on its own within %s",
				ErrLockTimeout, opt.Wait, opt.TTL)
		}
		opt.Logger.InfoContext(ctx, "another migrate run holds the lock; waiting", "poll", opt.Poll.String())
		select {
		case <-ctx.Done():
			return nil, nil, fmt.Errorf("wait for the migration lock: %w", ctx.Err())
		case <-time.After(opt.Poll):
		}
	}

	runCtx, cancel := context.WithCancelCause(ctx)
	stopped := make(chan struct{})
	go heartbeat(runCtx, cancel, db, holder, opt, leaseFrom, stopped)

	var released bool
	release := func() {
		if released {
			return
		}
		released = true
		cancel(nil)
		<-stopped
		// ctx may be canceled already (Ctrl+C): give the release its own time.
		rctx, rcancel := context.WithTimeout(context.WithoutCancel(ctx), 10*time.Second)
		defer rcancel()
		if _, err := db.ExecContext(rctx, `DELETE FROM migration_lock WHERE id = 1 AND holder = $1`, holder); err != nil {
			// Not fatal: the lease expires on its own.
			opt.Logger.WarnContext(ctx, "cannot release the migration lock; it expires on its own", "ttl", opt.TTL.String(), "error", err)
		}
	}
	return runCtx, release, nil
}

// createLockTable creates migration_lock if it is not there. Two runners may
// do it at once; CockroachDB can answer one of them with an error although the
// table now exists, so a failure is checked against the table before it counts.
func createLockTable(ctx context.Context, db *sql.DB) error {
	const ddl = `CREATE TABLE IF NOT EXISTS migration_lock (
		id INT4 NOT NULL PRIMARY KEY,
		holder TEXT NOT NULL,
		expires_at TIMESTAMPTZ NOT NULL,
		CONSTRAINT migration_lock_one_row CHECK (id = 1)
	)`
	err := withRetries(ctx, func() error {
		_, err := db.ExecContext(ctx, ddl)
		return err
	})
	if err != nil {
		return fmt.Errorf("create the migration lock table: %w", err)
	}
	return nil
}

// withRetries runs fn up to 5 times, a short wait apart: CREATE TABLE IF NOT
// EXISTS racing another runner's can fail once and work the next time.
func withRetries(ctx context.Context, fn func() error) error {
	var err error
	for range 5 {
		if err = fn(); err == nil {
			return nil
		}
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-time.After(200 * time.Millisecond):
		}
	}
	return err
}

// tryAcquire takes the lease if the row is free or expired. The INSERT creates
// the row when there is none; otherwise ON CONFLICT takes it over only when it
// has expired. RETURNING gives a row only when this holder won.
func tryAcquire(ctx context.Context, db *sql.DB, holder string, ttl time.Duration) (bool, error) {
	var won string
	err := db.QueryRowContext(ctx, `
		INSERT INTO migration_lock (id, holder, expires_at)
		VALUES (1, $1, now() + $2::INTERVAL)
		ON CONFLICT (id) DO UPDATE
		   SET holder = excluded.holder, expires_at = excluded.expires_at
		 WHERE migration_lock.expires_at < now()
		RETURNING holder`, holder, intervalOf(ttl)).Scan(&won)
	if errors.Is(err, sql.ErrNoRows) {
		return false, nil // someone else holds a live lease
	}
	if err != nil {
		return false, fmt.Errorf("take the migration lock: %w", err)
	}
	return won == holder, nil
}

// heartbeat renews the lease every ttl/3 until ctx ends. When a renewal finds
// the row gone or owned by someone else, or has failed for so long that the
// next one could not finish before the lease ends, it cancels the run: the
// migrations stop before another run can take the lease over, rather than race
// it.
//
// Each renewal has ttl/6 to answer, so a database that stops answering (a
// frozen connection never fails by itself) is noticed at the next tick instead
// of holding the heartbeat in the call. leaseFrom, and later the start of each
// renewal that worked, is a time before the database set the new expiry, so
// leaseFrom+ttl is when the lease ends at the earliest.
func heartbeat(ctx context.Context, cancel context.CancelCauseFunc, db *sql.DB, holder string, opt LockOptions, leaseFrom time.Time, stopped chan<- struct{}) {
	defer close(stopped)
	tick := time.NewTicker(opt.TTL / 3)
	defer tick.Stop()
	lastOK := leaseFrom
	for {
		select {
		case <-ctx.Done():
			return
		case <-tick.C:
		}
		attempt := time.Now()
		rctx, rcancel := context.WithTimeout(ctx, opt.TTL/6)
		res, err := db.ExecContext(rctx, `UPDATE migration_lock SET expires_at = now() + $2::INTERVAL WHERE id = 1 AND holder = $1`, holder, intervalOf(opt.TTL))
		rcancel()
		if ctx.Err() != nil {
			return
		}
		if err == nil {
			if n, _ := res.RowsAffected(); n == 1 {
				lastOK = attempt
				continue
			}
			cancel(errors.New("the migration lock was lost: another run took it over"))
			return
		}
		opt.Logger.WarnContext(ctx, "cannot renew the migration lock", "error", err)
		// The next renewal is a tick away: if that is the lease's end or later,
		// this was the last chance to keep it.
		if time.Since(lastOK)+opt.TTL/3 >= opt.TTL {
			cancel(fmt.Errorf("the migration lock could not be renewed for %s: %w", time.Since(lastOK).Round(time.Millisecond), err))
			return
		}
	}
}

// intervalOf writes a duration as a SQL interval, in whole milliseconds.
func intervalOf(d time.Duration) string {
	return fmt.Sprintf("%d milliseconds", d.Milliseconds())
}

func newHolderID() (string, error) {
	var b [8]byte
	if _, err := rand.Read(b[:]); err != nil {
		return "", fmt.Errorf("make a lock holder id: %w", err)
	}
	return hex.EncodeToString(b[:]), nil
}
