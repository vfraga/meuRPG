package db

import (
	"context"
	"errors"
	"fmt"
	"strings"
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
//	docker run -d --rm --name crdb -p 26257:26257 cockroachdb/cockroach:v26.2.7 start-single-node --insecure
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

	var got string
	if err := pool.QueryRow(t.Context(), "SHOW statement_timeout").Scan(&got); err != nil {
		t.Fatalf("SHOW statement_timeout: %v", err)
	}
	if got != "300" {
		t.Errorf("statement_timeout = %q, want 300 ms (the URL's value)", got)
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

// TestDefaultSessionTimeouts: with nothing in the URL, the pool asks for 30 s
// and 60 s (SHOW prints milliseconds).
func TestDefaultSessionTimeouts(t *testing.T) {
	t.Parallel()
	pool := testPool(t)
	for name, want := range map[string]string{"statement_timeout": "30000", "idle_in_transaction_session_timeout": "60000"} {
		var got string
		if err := pool.QueryRow(t.Context(), "SHOW "+name).Scan(&got); err != nil {
			t.Fatalf("SHOW %s: %v", name, err)
		}
		if got != want {
			t.Errorf("%s = %q, want %q", name, got, want)
		}
	}
}

// TestReadTxSeesOneSnapshot: two reads in a ReadTx agree even when another
// connection commits a write between them, while the same two reads as
// autocommit statements disagree (the torn read of audit D-03). A write inside
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

// A 40001 restarts the transaction with a plain BEGIN; the retry must still
// refuse writes.
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
