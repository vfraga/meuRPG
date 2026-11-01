package dbtest

import (
	"context"
	"database/sql"
	"fmt"
	"slices"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/PuraFome/meuRPG/backend/internal/platform/db"
	"github.com/PuraFome/meuRPG/backend/internal/platform/testenv"
	"github.com/PuraFome/meuRPG/backend/migrations"
)

// recorder is a testing.TB that keeps what the guard reports instead of
// failing the test that checks the guard.
type recorder struct {
	testing.TB
	mu      sync.Mutex
	errors  []string
	cleanup []func()
}

func (r *recorder) Errorf(format string, args ...any) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.errors = append(r.errors, fmt.Sprintf(format, args...))
}

func (r *recorder) Cleanup(f func()) { r.cleanup = append(r.cleanup, f) }

func (r *recorder) found() []string {
	r.mu.Lock()
	defer r.mu.Unlock()
	return append([]string(nil), r.errors...)
}

// TestGuardCatchesANestedAcquisition proves the guard every test pool carries
// (see guardPool): a read through the pool inside a db.InTx closure is
// reported with its stack, and a closure that uses only its transaction is not.
// The pool here has two connections, so the stack check is what is tested, not
// the one-connection deadlock (the second line of defense, which takes
// nestedAcquireTimeout). The nested read fails at once, canceled.
func TestGuardCatchesANestedAcquisition(t *testing.T) {
	rawURL := testenv.DatabaseURL(t)
	ctx := t.Context()

	rec := &recorder{TB: t}
	pool, err := db.NewPoolWith(ctx, rawURL, guardPool(rec, 2))
	if err != nil {
		t.Fatalf("NewPoolWith() error = %v", err)
	}
	defer pool.Close()
	if got := pool.Config().MaxConns; got != 2 {
		t.Fatalf("MaxConns = %d, want 2 (%s)", got, poolMaxConnsEnv)
	}

	// Only the transaction: nothing to report.
	if err := db.InTx(ctx, pool, func(tx pgx.Tx) error {
		var n int
		return tx.QueryRow(ctx, "SELECT 1").Scan(&n)
	}); err != nil {
		t.Fatalf("InTx() with only the transaction error = %v", err)
	}
	if got := rec.found(); len(got) != 0 {
		t.Fatalf("the guard reported a transaction that stays on its connection: %v", got)
	}

	// A read through the pool inside the closure: reported, and canceled.
	err = db.InTx(ctx, pool, func(tx pgx.Tx) error {
		var n int
		if err := tx.QueryRow(ctx, "SELECT 1").Scan(&n); err != nil {
			return err
		}
		return pool.QueryRow(ctx, "SELECT 2").Scan(&n) // the second connection
	})
	if err == nil {
		t.Fatal("InTx() with a nested read succeeded, want the read canceled")
	}
	got := rec.found()
	if len(got) != 1 || !strings.Contains(got[0], "nested pool acquisition") || !strings.Contains(got[0], "TestGuardCatchesANestedAcquisition") {
		t.Fatalf("the guard reported %q, want one nested pool acquisition that names this test in its stack", got)
	}

	// After the test ends, a late request must not touch the finished test.
	for _, f := range rec.cleanup {
		f()
	}
	_ = db.InTx(context.Background(), pool, func(_ pgx.Tx) error {
		return pool.QueryRow(context.Background(), "SELECT 3").Scan(new(int))
	})
	if got := rec.found(); len(got) != 1 {
		t.Errorf("a request after the test ended was reported again: %v", got)
	}
}

// TestPoolsHaveOneConnection pins the default size of a test pool.
func TestPoolsHaveOneConnection(t *testing.T) {
	rawURL := testenv.DatabaseURL(t)
	pool, err := db.NewPoolWith(t.Context(), rawURL, guardPool(&recorder{TB: t}, 1))
	if err != nil {
		t.Fatalf("NewPoolWith() error = %v", err)
	}
	defer pool.Close()
	if got := pool.Config().MaxConns; got != 1 {
		t.Errorf("MaxConns = %d, want 1: the guard needs a pool that deadlocks on a nested acquisition", got)
	}
}

// TestSeededTablesAreTheOnesTheMigrationsFill: the tables a migration fills are
// found in the migrated template, not listed by hand, and a test database has
// their rows (and empties everything else).
func TestSeededTablesAreTheOnesTheMigrationsFill(t *testing.T) {
	rawURL := testenv.DatabaseURL(t)
	ctx := t.Context()

	scratch, err := sql.Open("pgx", rawURL)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = scratch.Close() }()
	name := fmt.Sprintf("meurpg_dbtest_seeded_%d", time.Now().UnixNano())
	if _, err := scratch.ExecContext(ctx, "CREATE DATABASE "+name); err != nil {
		t.Fatalf("create the scratch database: %v", err)
	}
	t.Cleanup(func() {
		_, _ = scratch.ExecContext(context.WithoutCancel(ctx), migrations.DropDatabaseSQL(template.postgres, name))
	})
	dsn, err := withDatabase(rawURL, name)
	if err != nil {
		t.Fatal(err)
	}
	conn, err := sql.Open("pgx", dsn)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = conn.Close() }()
	for _, stmt := range []string{
		`CREATE TABLE seeded (id INT PRIMARY KEY)`,
		`CREATE TABLE seeded_too (id INT PRIMARY KEY)`,
		`CREATE TABLE written_by_the_game (id INT PRIMARY KEY)`,
		`CREATE TABLE goose_db_version (id INT PRIMARY KEY)`,
		`INSERT INTO seeded VALUES (1)`,
		`INSERT INTO seeded_too VALUES (1)`,
		`INSERT INTO goose_db_version VALUES (1)`,
	} {
		if _, err := conn.ExecContext(ctx, stmt); err != nil {
			t.Fatalf("%s: %v", stmt, err)
		}
	}
	got, err := seededTables(ctx, conn, []string{"goose_db_version", "seeded", "seeded_too", "written_by_the_game"})
	if err != nil {
		t.Fatalf("seededTables() error = %v", err)
	}
	if want := []string{"seeded", "seeded_too"}; !slices.Equal(got, want) {
		t.Errorf("seededTables() = %v, want %v", got, want)
	}

	// The real template: its seeded tables are in every test database, with rows.
	pool := NewPool(t, "meurpg_dbtest_seeded")
	if len(template.reference) == 0 {
		t.Fatal("the migrated template has no seeded table; session_event_kinds should be one")
	}
	for _, table := range template.reference {
		var n int
		if err := pool.QueryRow(ctx, "SELECT count(*) FROM "+pgx.Identifier{table}.Sanitize()).Scan(&n); err != nil {
			t.Fatalf("count %s: %v", table, err)
		}
		if n == 0 {
			t.Errorf("the test database has no rows in %s, which the migrations fill", table)
		}
	}
	if !slices.Contains(template.reference, "session_event_kinds") {
		t.Errorf("reference tables = %v, want session_event_kinds among them", template.reference)
	}
}

// TestBarrierHoldsEveryRacerUntilTheLastArrives: no racer goes on from Wait before all
// of them have reached it, so the calls after it really start together.
func TestBarrierHoldsEveryRacerUntilTheLastArrives(t *testing.T) {
	const racers = 16
	var arrived, early atomic.Int32
	var wg sync.WaitGroup
	b := NewBarrier(racers)
	for range racers {
		wg.Go(func() {
			arrived.Add(1)
			b.Wait()
			if arrived.Load() != racers {
				early.Add(1)
			}
		})
	}
	wg.Wait()
	if early.Load() != 0 {
		t.Errorf("%d racers went on before the last one arrived", early.Load())
	}
}

// failNow is a testing.TB whose Fatal panics instead of ending the test that checks it.
type failNow struct{ testing.TB }

func (failNow) Helper()        {}
func (failNow) Cleanup(func()) {}
func (failNow) Fatal(...any)   { panic("fatal") }

// TestPoolSizeAfterThePoolIsMadeFails: a PoolSize that comes after the pool was made
// would change nothing and leave the racers running one by one, so it fails the test.
func TestPoolSizeAfterThePoolIsMadeFails(t *testing.T) {
	rec := failNow{t}
	wantedMu.Lock()
	pooled[rec] = true
	wantedMu.Unlock()
	defer func() {
		wantedMu.Lock()
		delete(pooled, rec)
		delete(wanted, rec)
		wantedMu.Unlock()
		if recover() == nil {
			t.Error("PoolSize() after the pool was made did not fail the test")
		}
	}()
	PoolSize(rec, 4)
}
