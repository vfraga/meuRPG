package db

import (
	"context"
	"errors"
	"fmt"
	"slices"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/PuraFome/meuRPG/backend/internal/platform/testenv"
)

// testPool connects to the database in MEURPG_TEST_DATABASE_URL, or skips the
// test when the variable is unset, so `go test ./...` works without Docker.
//
// Start a throwaway CockroachDB with:
//
//	docker run -d --rm --name crdb -p 26257:26257 cockroachdb/cockroach:v26.2.8 start-single-node --insecure
//	export MEURPG_TEST_DATABASE_URL='postgresql://root@localhost:26257/defaultdb?sslmode=disable'
func testPool(t *testing.T) *pgxpool.Pool {
	t.Helper()

	url := testenv.DatabaseURL(t)

	pool, err := NewPool(t.Context(), url)
	if err != nil {
		t.Fatalf("NewPool() error = %v", err)
	}
	t.Cleanup(pool.Close)
	return pool
}

func TestPoolPing(t *testing.T) {
	t.Parallel()
	pool := testPool(t)

	if err := pool.Ping(t.Context()); err != nil {
		t.Fatalf("Ping() error = %v", err)
	}

	var appName string
	if err := pool.QueryRow(t.Context(), "SHOW application_name").Scan(&appName); err != nil {
		t.Fatalf("SHOW application_name: %v", err)
	}
	if appName != applicationName {
		t.Errorf("application_name = %q, want %q", appName, applicationName)
	}
}

// TestInTxRetriesRealConflict provokes a genuine serialization failure and
// checks that InTx recovers from it:
//
//  1. The transaction reads a counter.
//  2. Meanwhile, another connection increments the same counter and commits.
//  3. The transaction then writes a value based on its (now stale) read.
//
// Under SERIALIZABLE isolation CockroachDB cannot commit step 3, so it
// aborts with SQLSTATE 40001. InTx runs fn again, the second attempt reads
// the fresh value, and nothing is lost.
func TestInTxRetriesRealConflict(t *testing.T) {
	t.Parallel()
	pool := testPool(t)
	ctx := t.Context()

	table := pgx.Identifier{fmt.Sprintf("retry_test_%d", time.Now().UnixNano())}.Sanitize()
	mustExec(t, pool, "CREATE TABLE "+table+" (id INT PRIMARY KEY, n INT NOT NULL)")
	t.Cleanup(func() {
		// t.Context() is already canceled during cleanup.
		mustExec(t, pool, "DROP TABLE IF EXISTS "+table)
	})
	mustExec(t, pool, "INSERT INTO "+table+" (id, n) VALUES (1, 0)")

	attempts := 0
	err := InTx(ctx, pool, func(tx pgx.Tx) error {
		attempts++

		var n int
		if err := tx.QueryRow(ctx, "SELECT n FROM "+table+" WHERE id = 1").Scan(&n); err != nil {
			return fmt.Errorf("read counter: %w", err)
		}

		if attempts == 1 {
			// A concurrent writer, outside our transaction.
			if _, err := pool.Exec(ctx, "UPDATE "+table+" SET n = n + 1 WHERE id = 1"); err != nil {
				return fmt.Errorf("concurrent update: %w", err)
			}
		}

		if _, err := tx.Exec(ctx, "UPDATE "+table+" SET n = $1 WHERE id = 1", n+10); err != nil {
			return fmt.Errorf("write counter: %w", err)
		}
		return nil
	})
	if err != nil {
		t.Fatalf("InTx() error = %v", err)
	}

	t.Logf("fn ran %d times", attempts)
	if attempts < 2 {
		t.Errorf("fn ran %d time(s), want at least 2 (the first attempt must hit a 40001)", attempts)
	}

	var n int
	if err := pool.QueryRow(ctx, "SELECT n FROM "+table+" WHERE id = 1").Scan(&n); err != nil {
		t.Fatalf("read final counter: %v", err)
	}
	// 1 from the concurrent writer + 10 from our transaction. A lost update
	// would leave 10.
	if n != 11 {
		t.Errorf("counter = %d, want 11", n)
	}
}

func mustExec(t *testing.T, pool *pgxpool.Pool, sql string) {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	if _, err := pool.Exec(ctx, sql); err != nil {
		t.Fatalf("exec %q: %v", sql, err)
	}
}

// TestSessionTimeouts checks that the database really got the timeouts (they
// travel as startup parameters), and that they bite: a statement that runs
// past the limit is canceled with 57014, and an idle transaction is closed.
func TestSessionTimeouts(t *testing.T) {
	t.Parallel()
	url := testenv.DatabaseURL(t)
	sep := "?"
	if strings.Contains(url, "?") {
		sep = "&"
	}
	// Short limits in the URL: the test must not wait 30 seconds, and it proves
	// at the same time that the URL's value wins over the default.
	pool, err := NewPool(t.Context(), url+sep+"statement_timeout=300&idle_in_transaction_session_timeout=400")
	if err != nil {
		t.Fatalf("NewPool() error = %v", err)
	}
	t.Cleanup(pool.Close)

	if got := settingMillis(t, pool, "statement_timeout"); got != 300 {
		t.Errorf("statement_timeout = %d ms, want 300 ms (the URL's value)", got)
	}

	_, err = pool.Exec(t.Context(), "SELECT pg_sleep(5)")
	if pgErr, ok := errors.AsType[*pgconn.PgError](err); !ok || pgErr.Code != "57014" {
		t.Errorf("a statement past the limit: error = %v, want SQLSTATE 57014", err)
	}

	tx, err := pool.Begin(t.Context())
	if err != nil {
		t.Fatalf("Begin() error = %v", err)
	}
	if _, err := tx.Exec(t.Context(), "SELECT 1"); err != nil {
		t.Fatalf("first statement: %v", err)
	}
	time.Sleep(1200 * time.Millisecond)
	if _, err := tx.Exec(t.Context(), "SELECT 1"); err == nil {
		t.Error("a transaction idle past the limit still works; want it closed by the database")
	}
	_ = tx.Rollback(t.Context())
}

// settingMillis reads a timeout setting in milliseconds. SHOW prints a bare number of milliseconds on CockroachDB
// ("30000") and a number with a unit on PostgreSQL ("30s", "1min", "300ms").
func settingMillis(t *testing.T, pool *pgxpool.Pool, name string) int64 {
	t.Helper()
	var text string
	if err := pool.QueryRow(t.Context(), "SHOW "+name).Scan(&text); err != nil {
		t.Fatalf("SHOW %s: %v", name, err)
	}
	if n, err := strconv.ParseInt(text, 10, 64); err == nil {
		return n
	}
	d, err := time.ParseDuration(strings.Replace(text, "min", "m", 1))
	if err != nil {
		t.Fatalf("SHOW %s = %q: not a duration", name, text)
	}
	return d.Milliseconds()
}

// TestDefaultSessionTimeouts: with nothing in the URL, the pool asks for 30 s
// and 60 s.
func TestDefaultSessionTimeouts(t *testing.T) {
	t.Parallel()
	pool := testPool(t)
	for name, want := range map[string]int64{"statement_timeout": 30000, "idle_in_transaction_session_timeout": 60000} {
		if got := settingMillis(t, pool, name); got != want {
			t.Errorf("%s = %d ms, want %d ms", name, got, want)
		}
	}
}

// TestReadTxSeesOneSnapshot: two reads in a ReadTx agree even when another
// connection commits a write between them, while the same two reads as
// autocommit statements disagree. A write inside
// a ReadTx is refused.
func TestReadTxSeesOneSnapshot(t *testing.T) {
	t.Parallel()
	pool := testPool(t)
	side := testPool(t) // its own connection: the writer that commits in between
	ctx := t.Context()

	table := pgx.Identifier{fmt.Sprintf("readtx_test_%d", time.Now().UnixNano())}.Sanitize()
	mustExec(t, pool, "CREATE TABLE "+table+" (id INT PRIMARY KEY, n INT NOT NULL)")
	t.Cleanup(func() { mustExec(t, pool, "DROP TABLE IF EXISTS "+table) })
	mustExec(t, pool, "INSERT INTO "+table+" (id, n) VALUES (1, 1)")

	read := func(q interface {
		QueryRow(context.Context, string, ...any) pgx.Row
	},
	) int {
		var n int
		if err := q.QueryRow(ctx, "SELECT n FROM "+table+" WHERE id = 1").Scan(&n); err != nil {
			t.Fatalf("read n: %v", err)
		}
		return n
	}

	var first, second int
	err := ReadTx(ctx, pool, func(tx pgx.Tx) error {
		first = read(tx)
		mustExec(t, side, "UPDATE "+table+" SET n = 2 WHERE id = 1") // commits now
		second = read(tx)
		return nil
	})
	if err != nil {
		t.Fatalf("ReadTx() error = %v", err)
	}
	if first != 1 || second != 1 {
		t.Errorf("reads inside ReadTx = %d, %d; want 1, 1 (one snapshot)", first, second)
	}
	if got := read(pool); got != 2 {
		t.Errorf("a read after the commit = %d, want 2 (ReadTx is not stale reading)", got)
	}

	// Contrast: autocommit reads each take their own timestamp.
	a := read(pool)
	mustExec(t, side, "UPDATE "+table+" SET n = 3 WHERE id = 1")
	if b := read(pool); a == b {
		t.Errorf("autocommit reads = %d, %d; expected them to differ", a, b)
	}

	err = ReadTx(ctx, pool, func(tx pgx.Tx) error {
		_, err := tx.Exec(ctx, "UPDATE "+table+" SET n = 9 WHERE id = 1")
		return err
	})
	if err == nil {
		t.Error("a write inside ReadTx succeeded, want an error (read-only transaction)")
	}
}

// TestAQueuedRequestWaitsAtMostAsLongAsTheConnectionIsHeld: with every
// connection busy a request queues for one, and the wait ends when the holder
// lets go; a holder stuck in one statement is cut off by statement_timeout, so
// the queue is never longer than that limit plus the holder's other work. A
// single-connection pool and a 500 ms limit stand for the 10 connections and
// 30 s of production.
func TestAQueuedRequestWaitsAtMostAsLongAsTheConnectionIsHeld(t *testing.T) {
	t.Parallel()
	url := testenv.DatabaseURL(t)
	sep := "?"
	if strings.Contains(url, "?") {
		sep = "&"
	}
	pool, err := NewPoolWith(t.Context(), url+sep+"statement_timeout=500", func(cfg *pgxpool.Config) { cfg.MaxConns = 1 })
	if err != nil {
		t.Fatalf("NewPoolWith() error = %v", err)
	}
	t.Cleanup(pool.Close)

	holding := make(chan struct{})
	holderErr := make(chan error, 1)
	go func() {
		holderErr <- InTx(t.Context(), pool, func(tx pgx.Tx) error {
			select {
			case <-holding:
			default:
				close(holding)
			}
			_, err := tx.Exec(t.Context(), "SELECT pg_sleep(30)")
			return err
		})
	}()
	<-holding

	start := time.Now()
	err = InTx(t.Context(), pool, func(tx pgx.Tx) error {
		_, err := tx.Exec(t.Context(), "SELECT 1")
		return err
	})
	waited := time.Since(start)
	if err != nil {
		t.Fatalf("the queued transaction: error = %v, want it served once the holder is cut off", err)
	}
	if waited > 10*time.Second {
		t.Errorf("the queued transaction waited %v, want about the statement timeout (500 ms)", waited)
	}
	if pgErr, ok := errors.AsType[*pgconn.PgError](<-holderErr); !ok || pgErr.Code != "57014" {
		t.Errorf("the holder was not canceled by statement_timeout (57014)")
	}
}

// Racers that each count the rows and insert one while the count is under a
// cap end with exactly the cap, and every racer that was told "saved" has a
// row. SERIALIZABLE makes the late ones fail, and InTx retries them: a
// failure the database reports at COMMIT must not be reported as success.
func TestInTxLosesNoWriteUnderContention(t *testing.T) {
	t.Parallel()
	url := testenv.DatabaseURL(t)
	const racers, limit = 12, 4
	pool, err := NewPoolWith(t.Context(), url, func(cfg *pgxpool.Config) { cfg.MaxConns = racers })
	if err != nil {
		t.Fatalf("NewPoolWith() error = %v", err)
	}
	t.Cleanup(pool.Close)
	ctx := t.Context()
	table := fmt.Sprintf("cap_test_%d", time.Now().UnixNano())
	mustExec(t, pool, "CREATE TABLE "+table+" (id INT PRIMARY KEY)")
	t.Cleanup(func() { _, _ = pool.Exec(context.Background(), "DROP TABLE IF EXISTS "+table) })

	var (
		wg    sync.WaitGroup
		mu    sync.Mutex
		saved []int
	)
	for id := range racers {
		wg.Go(func() {
			err := InTx(ctx, pool, func(tx pgx.Tx) error {
				var n int
				if err := tx.QueryRow(ctx, "SELECT count(*) FROM "+table).Scan(&n); err != nil {
					return fmt.Errorf("count: %w", err)
				}
				if n >= limit {
					return errCapReached
				}
				if _, err := tx.Exec(ctx, "INSERT INTO "+table+" VALUES ($1)", id); err != nil {
					return fmt.Errorf("insert: %w", err)
				}
				return nil
			})
			if err == nil {
				mu.Lock()
				saved = append(saved, id)
				mu.Unlock()
			} else if !errors.Is(err, errCapReached) {
				t.Errorf("racer %d: %v", id, err)
			}
		})
	}
	wg.Wait()

	rows, err := pool.Query(ctx, "SELECT id FROM "+table+" ORDER BY id")
	if err != nil {
		t.Fatal(err)
	}
	stored, err := pgx.CollectRows(rows, pgx.RowTo[int])
	if err != nil {
		t.Fatal(err)
	}
	slices.Sort(saved)
	if len(stored) != limit || !slices.Equal(saved, stored) {
		t.Errorf("rows saved = %v, racers told \"saved\" = %v; want the same %d", stored, saved, limit)
	}
}

var errCapReached = errors.New("cap reached")

// A 40001 starts the transaction again; the new BEGIN must still be READ ONLY,
// so the retry refuses writes.
func TestReadTxStaysReadOnlyAfterRetry(t *testing.T) {
	pool := testPool(t)
	ctx := t.Context()
	table := fmt.Sprintf("readtx_retry_%d", time.Now().UnixNano())
	if _, err := pool.Exec(ctx, "CREATE TABLE "+table+" (id INT PRIMARY KEY, n INT)"); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _, _ = pool.Exec(context.Background(), "DROP TABLE "+table) })

	attempts := 0
	var writeErr error
	err := ReadTx(ctx, pool, func(tx pgx.Tx) error {
		attempts++
		if attempts == 1 {
			return &pgconn.PgError{Code: "40001", Message: "forced retry"}
		}
		_, writeErr = tx.Exec(ctx, "INSERT INTO "+table+" VALUES (1, 1)")
		return writeErr
	})
	if attempts < 2 {
		t.Fatalf("retry did not happen (attempts=%d)", attempts)
	}
	if err == nil || writeErr == nil {
		t.Errorf("write inside ReadTx on retry attempt succeeded; want read-only error")
	}
	var c int
	if e := pool.QueryRow(ctx, "SELECT count(*) FROM "+table).Scan(&c); e != nil {
		t.Fatal(e)
	}
	if c != 0 {
		t.Errorf("rows committed by ReadTx = %d, want 0", c)
	}
}

// TestACanceledCallFreesItsConnectionAtOnce checks against the real database
// that the server honors the cancel request the pool sends (issue 710). If it
// did not, a canceled call would hold its only connection until the
// cancelDeadlineFallback.
func TestACanceledCallFreesItsConnectionAtOnce(t *testing.T) {
	t.Parallel()
	url := testenv.DatabaseURL(t)

	pool, err := NewPoolWith(t.Context(), url, func(cfg *pgxpool.Config) { cfg.MaxConns = 1 })
	if err != nil {
		t.Fatalf("NewPoolWith() error = %v", err)
	}
	t.Cleanup(pool.Close)

	ctx, cancel := context.WithTimeout(t.Context(), 100*time.Millisecond)
	defer cancel()
	start := time.Now()
	_, err = pool.Exec(ctx, "SELECT pg_sleep(10)")
	took := time.Since(start)
	if err == nil {
		t.Fatal("pg_sleep(10) returned without an error")
	}
	if !errors.Is(err, context.DeadlineExceeded) && !strings.Contains(err.Error(), "cancel") {
		t.Errorf("error = %v, want a cancellation", err)
	}
	if took >= 2*time.Second {
		t.Fatalf("the canceled call took %v, want well under the %v fallback", took, cancelDeadlineFallback)
	}

	start = time.Now()
	var one int
	if err := pool.QueryRow(t.Context(), "SELECT 1").Scan(&one); err != nil {
		t.Fatalf("the call after the cancel failed: %v", err)
	}
	if next := time.Since(start); next >= 2*time.Second {
		t.Errorf("the call after the cancel took %v, want it at once", next)
	}
}
