// Package dbtest gives integration tests a real, freshly migrated database,
// CockroachDB or PostgreSQL.
// Only tests import it.
//
// The tests need MEURPG_TEST_DATABASE_URL; without it they skip (or fail, with
// MEURPG_REQUIRE_DB=1, which CI sets), so `go test ./...` works anywhere.
// The package asks the server which database it is.
// To run the tests, start a throwaway database (`make db-test-start`) and run:
//
//	MEURPG_TEST_DATABASE_URL='postgresql://root@localhost:5433/postgres?sslmode=disable' make test-postgres
//
// Migrating a new database for every test is too slow on CockroachDB, where
// each schema change is a job.
// So the migrations run once per set of migration files, into a template
// database named after their fingerprint (meurpg_tpl_<hash>).
// A test database is a copy of the template:
//   - On PostgreSQL, a native CREATE DATABASE ... TEMPLATE.
//   - On CockroachDB, which has no native copy, the template's DDL is replayed
//     and its reference tables and goose's table are copied row by row.
//
// A copy still costs much more than emptying a database, so a package makes
// only as many databases as it runs tests at once and reuses them, emptying
// one with DELETE.
// The schema is the one the migrations build; the migrations package still
// tests the migrations themselves, from scratch.
//
// Every test process shares the template: the first one to create it
// migrates it, and the others wait until it has every migration.
// A template left half migrated (a run killed midway) is dropped and built again.
// Templates of older migration sets stay on the test server; they are empty
// and safe to drop.
package dbtest

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"net/url"
	"os"
	"regexp"
	"runtime"
	"slices"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgxpool"
	_ "github.com/jackc/pgx/v5/stdlib" // the "pgx" driver for database/sql, which goose needs
	"github.com/pressly/goose/v3"

	"github.com/PuraFome/meuRPG/backend/internal/platform/db"
	"github.com/PuraFome/meuRPG/backend/internal/platform/testenv"
	"github.com/PuraFome/meuRPG/backend/migrations"
)

// EnvVar names the variable with the test server's connection string.
const EnvVar = testenv.DatabaseEnv

// counter makes database names unique even when two tests start in the same
// nanosecond.
var counter atomic.Int64

// The template this process clones, built once.
var (
	templateOnce sync.Once
	template     templateDB
	errTemplate  error
)

// The databases this process made, and the ones free for the next test: a
// test that ends gives its database back, and the next test with the same
// prefix empties it (DELETE, a few milliseconds) instead of building a new
// one (CREATE DATABASE and every table, seconds on CockroachDB).
var (
	freeMu sync.Mutex
	free   = map[string][]string{}
	made   []string
)

// templateDB is a migrated database and the statements that copy it.
type templateDB struct {
	name string
	// tables are the template's tables, each after the ones it references;
	// emptying a database deletes them in reverse.
	tables []string
	// ddl creates the template's sequences and tables, each table after the
	// ones it references.
	ddl []string
	// reference are the tables (in the order of tables) that the migrations
	// leave with rows: data the game reads and never writes, copied with its
	// rows, and left alone when a database is emptied. Read from the migrated
	// template, so a migration that seeds another table needs no change here.
	reference []string
	// postgres says the server is PostgreSQL: the copy is CREATE DATABASE ...
	// TEMPLATE and ddl is empty (postgres.go).
	postgres bool
}

// Enabled reports whether a test database is configured.
func Enabled() bool { return os.Getenv(EnvVar) != "" }

// NewPool gives the test an empty database on the test server with every
// migration's schema and returns a pool connected to it. The database is
// the test's alone while it runs, so tests can run in parallel without
// seeing each other's rows; when the test ends it goes back to the package,
// and the next test with the same prefix empties it and uses it again (the
// package's TestMain drops them all at the end, see Main). prefix names the
// databases, to tell packages apart in CockroachDB's console.
//
// Without MEURPG_TEST_DATABASE_URL, NewPool skips the test (it fails the test
// when MEURPG_REQUIRE_DB=1, which CI sets: see package testenv).
//
// The pool has one connection (see guardPool). A test that races transactions
// needs them to overlap: it asks for a bigger pool with NewPoolConns.
func NewPool(t testing.TB, prefix string) *pgxpool.Pool {
	t.Helper()
	wantedMu.Lock()
	conns, ok := wanted[t]
	if !pooled[t] {
		pooled[t] = true
		t.Cleanup(func() {
			wantedMu.Lock()
			delete(pooled, t)
			wantedMu.Unlock()
		})
	}
	wantedMu.Unlock()
	if !ok {
		conns = 1
	}
	return NewPoolConns(t, prefix, conns)
}

// The pool sizes tests asked for with PoolSize, and the tests that already got a
// pool from NewPool (a PoolSize after that would change nothing).
var (
	wantedMu sync.Mutex
	wanted   = map[testing.TB]int32{}
	pooled   = map[testing.TB]bool{}
)

// PoolSize makes the pool this test gets from NewPool (through any harness) have
// conns connections instead of one. Call it first in a test that races
// transactions, before the harness is built, with conns at least the number of
// racers: on one connection they would run one after the other and prove
// nothing. The guard against nested acquisitions stays on. It works with
// t.Parallel, which the process-wide MEURPG_TEST_POOL_MAX_CONNS does not (t.Setenv
// forbids it).
func PoolSize(t testing.TB, conns int32) {
	t.Helper()
	wantedMu.Lock()
	late := pooled[t]
	wanted[t] = conns
	wantedMu.Unlock()
	if late {
		t.Fatal("dbtest.PoolSize came after the test's pool was made (the harness): it changes nothing, and the racers run one by one; call it first")
	}
	t.Cleanup(func() {
		wantedMu.Lock()
		delete(wanted, t)
		wantedMu.Unlock()
	})
}

// Barrier lines up the racers of a test so they really overlap. NewBarrier(n) is made
// before the goroutines are started, and each of the n calls Wait first: none goes on
// until all n have reached it. Starting them and closing a channel right after does
// not do that, since a goroutine may begin its call before the last one has even
// been scheduled.
type Barrier struct {
	arrived sync.WaitGroup
	open    chan struct{}
}

// NewBarrier returns a Barrier for n racers.
func NewBarrier(n int) *Barrier {
	b := &Barrier{open: make(chan struct{})}
	b.arrived.Add(n)
	go func() {
		b.arrived.Wait()
		close(b.open)
	}()
	return b
}

// Wait blocks until every racer has called it, and then lets them all go.
func (b *Barrier) Wait() {
	b.arrived.Done()
	<-b.open
}

// NewPoolConns is NewPool with a pool of conns connections, for a test that
// races transactions (ten people accepting the last use of an invite): with
// one connection they would run one after the other and prove nothing. Pick
// conns at least as big as the number of racers. The guard against nested
// acquisitions stays on at any size; MEURPG_TEST_POOL_MAX_CONNS, which is
// process-wide, only changes the default of NewPool.
func NewPoolConns(t testing.TB, prefix string, conns int32) *pgxpool.Pool {
	t.Helper()
	rawURL := testenv.DatabaseURL(t)

	templateOnce.Do(func() { template, errTemplate = buildTemplate(rawURL) })
	if errTemplate != nil {
		t.Fatalf("build the test database template: %v", errTemplate)
	}

	name, reused := take(prefix)
	if !reused {
		name = fmt.Sprintf("%s_%d_%d", prefix, time.Now().UnixNano(), counter.Add(1))
	}
	dsn, err := withDatabase(rawURL, name)
	if err != nil {
		t.Fatalf("parse %s: %v", EnvVar, err)
	}
	if reused {
		conn := open(t, dsn)
		if _, err := conn.ExecContext(t.Context(), template.emptyAll()); err != nil {
			t.Fatalf("empty the reused test database %s: %v", name, err)
		}
		return poolFor(t, prefix, name, dsn, conns)
	}
	admin := open(t, rawURL)
	if template.postgres {
		freeMu.Lock()
		made = append(made, name)
		freeMu.Unlock()
		if err := copyPostgresTemplate(t.Context(), admin, template.name, name); err != nil {
			t.Fatalf("copy the template: %v", err)
		}
		return poolFor(t, prefix, name, dsn, conns)
	}
	mustExec(t, admin, "CREATE DATABASE "+name)
	freeMu.Lock()
	made = append(made, name)
	freeMu.Unlock()
	copyCockroachTemplate(t, open(t, dsn))

	return poolFor(t, prefix, name, dsn, conns)
}

// copyCockroachTemplate copies the template's schema and rows into the empty database conn is open on.
func copyCockroachTemplate(t testing.TB, conn *sql.DB) {
	t.Helper()
	for _, stmt := range template.ddl {
		if _, err := conn.ExecContext(t.Context(), stmt); err != nil {
			t.Fatalf("copy the template's schema: %v\n%s", err, stmt)
		}
	}
	// The reference tables keep their rows: a migration fills them, and no test
	// writes to them, so a copy of the schema alone would leave them empty.
	for _, table := range template.reference {
		if _, err := conn.ExecContext(t.Context(), fmt.Sprintf(
			`INSERT INTO %s SELECT * FROM %s.public.%s`, table, template.name, table)); err != nil {
			t.Fatalf("copy the reference table %s: %v", table, err)
		}
	}
	// goose's own table too, so the database says which migrations it has.
	if _, err := conn.ExecContext(t.Context(), fmt.Sprintf(
		`INSERT INTO goose_db_version SELECT * FROM %s.public.goose_db_version`, template.name)); err != nil {
		t.Fatalf("copy the migration versions: %v", err)
	}
	if _, err := conn.ExecContext(t.Context(),
		`SELECT setval('goose_db_version_id_seq', (SELECT max(id) FROM goose_db_version))`); err != nil {
		t.Fatalf("move the migration version sequence: %v", err)
	}
}

// Main runs a package's tests and then drops the databases this process
// made. Every package that uses NewPool calls it from its TestMain:
//
//	func TestMain(m *testing.M) { dbtest.Main(m) }
//
// Without it the databases stay on the test server after the run (harmless,
// but they pile up; they can be dropped by hand, see CONTRIBUTING.md).
func Main(m *testing.M) {
	code := m.Run()
	if url := os.Getenv(EnvVar); url != "" && len(made) > 0 {
		dropAll(url)
	}
	os.Exit(code)
}

// dropAll drops every database this process made, in one statement each, and
// gives up quietly on a server too busy to answer: the run is over.
func dropAll(rawURL string) {
	if errTemplate != nil {
		return // no template, so no database was made, and the dialect is unknown
	}
	conn, err := sql.Open("pgx", rawURL)
	if err != nil {
		return
	}
	defer func() { _ = conn.Close() }()
	for _, name := range made {
		ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
		_, _ = conn.ExecContext(ctx, migrations.DropDatabaseSQL(template.postgres, name))
		cancel()
	}
}

// poolFor connects to a test database and gives it back when the test ends:
// the pool closes first (cleanups run last-in, first-out), then the database
// is free for the next test with this prefix.
func poolFor(t testing.TB, prefix, name, dsn string, conns int32) *pgxpool.Pool {
	t.Helper()
	t.Cleanup(func() {
		freeMu.Lock()
		free[prefix] = append(free[prefix], name)
		freeMu.Unlock()
	})
	pool, err := db.NewPoolWith(t.Context(), dsn, guardPool(t, conns))
	if err != nil {
		t.Fatalf("NewPool() error = %v", err)
	}
	t.Cleanup(pool.Close)
	return pool
}

// Guard against nested pool acquisition: a request that takes a second pool
// connection while its transaction holds the first.
//
// A db.InTx closure that reads through the pool instead of the transaction
// (a cross-module call with no tx, a pool-bound s.queries) takes a second
// connection while the transaction keeps the first. With the pool's small
// size and a few concurrent requests, every connection ends up held by a
// transaction waiting for another one: a deadlock that lasts until the
// requests' contexts end (PR #120: CombatService.MoveCombatant waited 176 s).
// Alone, in a test, the same code works, which is why it slipped through.
// Two guards, on every test pool:
//
//   - The pool tracer looks at the stack of each Acquire. One made from inside
//     a db.InTx closure is a bug whatever the pool's size: the test fails at
//     once with the stack, which names the path, and the Acquire itself is
//     canceled (errNestedAcquire), so nothing waits.
//   - The pool has ONE connection (MaxConns = 1; NewPoolConns asks for more),
//     so a nested acquisition that the stack check cannot see (another goroutine
//     started by the closure) deadlocks, and after nestedAcquireTimeout a panic
//     prints the waiting goroutine's stack. That panic ends the whole package's
//     test binary, not just the test.
//
// MEURPG_TEST_POOL_MAX_CONNS overrides the size, for a test that really needs
// several connections at once (or to list every violation in one run instead
// of stopping at the first deadlock).
const poolMaxConnsEnv = "MEURPG_TEST_POOL_MAX_CONNS"

// nestedAcquireTimeout is how long an Acquire may wait before it is called a
// deadlock. It is the second line of defense, for the nested acquisition that
// the stack check cannot see, so it is long: on a one-connection pool the
// requests of a test queue for the connection, and under the race detector and
// a busy test server a queue of them waits whole seconds (a request builds its
// answer from dozens of queries).
const nestedAcquireTimeout = 90 * time.Second

// inTxClosure is the frame of the function db.InTx calls with the
// transaction: its fn runs under it, and nothing else does.
//
// If db renames that function, the guard goes blind and
// TestGuardCatchesANestedAcquisition (it needs the database, so it runs in
// CI's go-db job) fails.
const inTxClosure = "platform/db.runAttempt"

// errNestedAcquire is the cause of the canceled Acquire of a nested read.
var errNestedAcquire = errors.New("dbtest: nested pool acquisition inside a db.InTx closure")

// guardPool is the tweak NewPoolWith applies to every test pool.
func guardPool(t testing.TB, conns int32) func(*pgxpool.Config) {
	t.Helper()
	return func(cfg *pgxpool.Config) {
		cfg.MaxConns = conns
		if v := os.Getenv(poolMaxConnsEnv); v != "" && conns == 1 {
			var n int32
			if _, err := fmt.Sscan(v, &n); err == nil && n > 0 {
				cfg.MaxConns = n
			}
		}
		w := &acquireWatch{t: t}
		t.Cleanup(w.finish)
		cfg.ConnConfig.Tracer = w
	}
}

type acquireWatchKey struct{}

// acquireWatch implements pgxpool.AcquireTracer (and pgx.QueryTracer, which pgx
// requires of the tracer, without tracing any query).
type acquireWatch struct {
	t testing.TB
	// mu guards done: a request that outlives the test must not touch t.
	mu   sync.Mutex
	done bool
}

func (w *acquireWatch) finish() {
	w.mu.Lock()
	w.done = true
	w.mu.Unlock()
}

type acquireWait struct{ timer *time.Timer }

func (w *acquireWatch) TraceAcquireStart(ctx context.Context, pool *pgxpool.Pool, _ pgxpool.TraceAcquireStartData) context.Context {
	// Only the program counters are taken.
	// Each is looked up in a cache that says whether it lies in the transaction's closure.
	// The stack is formatted only when it is flagged or the backstop timer fires.
	var buf [maxStackDepth]uintptr
	pcs := buf[:runtime.Callers(2, buf[:])]
	if slices.ContainsFunc(pcs, inTxClosureAt) {
		w.mu.Lock()
		if !w.done {
			w.t.Errorf("nested pool acquisition: a query went through the pool inside a db.InTx closure, "+
				"which takes a second connection while the transaction holds the first and deadlocks under load "+
				"(inside a transaction use only queries.WithTx(tx), or a function that takes the pgx.Tx). The stack:\n%s", formatStack(slices.Clone(pcs)))
		}
		w.mu.Unlock()
		// Fail the Acquire at once: nothing waits for a connection the
		// transaction would never give back.
		ctx, cancel := context.WithCancelCause(ctx)
		cancel(errNestedAcquire)
		return ctx
	}
	stack := slices.Clone(pcs)
	wait := &acquireWait{}
	wait.timer = time.AfterFunc(nestedAcquireTimeout, func() {
		panic(fmt.Sprintf("dbtest: no pool connection for %v (pool of %d): a read through the pool "+
			"inside a db.InTx closure, probably. The goroutine that waits:\n%s",
			nestedAcquireTimeout, pool.Config().MaxConns, formatStack(stack)))
	})
	return context.WithValue(ctx, acquireWatchKey{}, wait)
}

// maxStackDepth is how many frames of an acquirer's stack are kept.
const maxStackDepth = 256

// inTxPCs remembers, for each program counter seen on an acquirer's stack,
// whether it lies in the transaction's closure.
// The stacks of a package's acquirers repeat the same counters, so each is resolved once.
var inTxPCs sync.Map // uintptr -> bool

// inTxClosureAt reports whether the frame at pc is the transaction's closure.
// runtime.Callers returns a program counter for each frame, the outer frames of inlined calls included.
// So one counter is one frame.
func inTxClosureAt(pc uintptr) bool {
	if v, ok := inTxPCs.Load(pc); ok {
		hit, _ := v.(bool)
		return hit
	}
	f, _ := runtime.CallersFrames([]uintptr{pc}).Next()
	hit := strings.Contains(f.Function, inTxClosure)
	inTxPCs.Store(pc, hit)
	return hit
}

// formatStack prints the frames of a captured stack, one function and one
// file:line each.
func formatStack(pcs []uintptr) string {
	var b strings.Builder
	frames := runtime.CallersFrames(pcs)
	for {
		f, more := frames.Next()
		fmt.Fprintf(&b, "\t%s\n\t\t%s:%d\n", f.Function, f.File, f.Line)
		if !more {
			return b.String()
		}
	}
}

func (*acquireWatch) TraceAcquireEnd(ctx context.Context, _ *pgxpool.Pool, _ pgxpool.TraceAcquireEndData) {
	if w, ok := ctx.Value(acquireWatchKey{}).(*acquireWait); ok {
		w.timer.Stop()
	}
}

func (*acquireWatch) TraceQueryStart(ctx context.Context, _ *pgx.Conn, _ pgx.TraceQueryStartData) context.Context {
	return ctx
}
func (*acquireWatch) TraceQueryEnd(context.Context, *pgx.Conn, pgx.TraceQueryEndData) {}

// SideConnection returns a pool of its own on the database of pool, for a test
// that must hold a transaction open while the service under test works: the
// service's pool has one connection (see guardPool), so a transaction begun on
// it would leave the service with none. Begin on the side pool (side.Begin):
// do not wrap service calls in db.InTx(ctx, side, ...), the guard would flag
// them as nested. The side pool is closed when the test
// ends and is not guarded: it is the test's, not the service's.
func SideConnection(t testing.TB, pool *pgxpool.Pool) *pgxpool.Pool {
	t.Helper()
	side, err := pgxpool.New(t.Context(), pool.Config().ConnString())
	if err != nil {
		t.Fatalf("open a side connection: %v", err)
	}
	t.Cleanup(side.Close)
	return side
}

// take returns a free database with this prefix, if there is one.
func take(prefix string) (string, bool) {
	freeMu.Lock()
	defer freeMu.Unlock()
	names := free[prefix]
	if len(names) == 0 {
		return "", false
	}
	name := names[len(names)-1]
	free[prefix] = names[:len(names)-1]
	return name, true
}

// emptyAll deletes every row of every table, children before parents.
// On PostgreSQL it runs as READ COMMITTED: the server tracks serializable transactions across all
// its databases, and when many test binaries run at once it summarizes the old ones and cancels a
// DELETE of the whole database ("conflict out to old committed transaction", 40001). A cleanup
// that nobody else reads needs no serializability.
func (tpl templateDB) emptyAll() string {
	var b strings.Builder
	if tpl.postgres {
		b.WriteString("BEGIN ISOLATION LEVEL READ COMMITTED;")
	}
	for _, v := range slices.Backward(tpl.tables) {
		if v == "goose_db_version" || slices.Contains(tpl.reference, v) {
			continue // the migrations it records, and the reference data, stay
		}
		b.WriteString("DELETE FROM " + pgx.Identifier{v}.Sanitize() + ";")
	}
	if tpl.postgres {
		b.WriteString("COMMIT;")
	}
	return b.String()
}

// errStalled says a template has stopped getting migrations: whoever was
// migrating it is gone.
var errStalled = errors.New("the template stopped getting migrations")

// buildTemplate makes sure the template for these migrations exists and is
// complete, and reads the statements that copy it.
func buildTemplate(rawURL string) (templateDB, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Minute)
	defer cancel()

	fingerprint, err := migrations.Fingerprint()
	if err != nil {
		return templateDB{}, fmt.Errorf("fingerprint the migrations: %w", err)
	}
	name := "meurpg_tpl_" + fingerprint
	dsn, err := withDatabase(rawURL, name)
	if err != nil {
		return templateDB{}, err
	}
	admin, err := sql.Open("pgx", rawURL)
	if err != nil {
		return templateDB{}, err
	}
	defer func() { _ = admin.Close() }()
	tpl, err := sql.Open("pgx", dsn)
	if err != nil {
		return templateDB{}, err
	}
	defer func() { _ = tpl.Close() }()
	postgres, err := migrations.IsPostgres(ctx, admin)
	if err != nil {
		return templateDB{}, fmt.Errorf("create the template: %w", err)
	}
	provider, err := migrations.NewAdaptedProvider(tpl, postgres)
	if err != nil {
		return templateDB{}, err
	}
	if err := ensureMigrated(ctx, admin, tpl, provider, name, postgres); err != nil {
		return templateDB{}, err
	}
	if postgres {
		return postgresTemplate(ctx, tpl, name)
	}

	ddl, tables, err := schemaDDL(ctx, tpl)
	if err != nil {
		return templateDB{}, err
	}
	routines, err := routineDDL(ctx, tpl)
	if err != nil {
		return templateDB{}, err
	}
	ddl = append(ddl, routines...)
	reference, err := seededTables(ctx, tpl, tables)
	if err != nil {
		return templateDB{}, err
	}
	return templateDB{name: name, ddl: ddl, tables: tables, reference: reference}, nil
}

// ensureMigrated returns once the template name has every migration: made
// and migrated here, or by another process. A template whose migrations
// stalled is dropped and made again.
func ensureMigrated(ctx context.Context, admin, tpl *sql.DB, provider *goose.Provider, name string, postgres bool) error {
	sources := provider.ListSources()
	latest := sources[len(sources)-1].Version
	for {
		again, err := createOrAwait(ctx, admin, tpl, provider, name, latest, postgres)
		if !again {
			return err
		}
	}
}

// createOrAwait makes and migrates the template, or, when another process
// made it, waits until it has the latest migration. again says the template
// was stalled and is dropped: try once more.
func createOrAwait(ctx context.Context, admin, tpl *sql.DB, provider *goose.Provider, name string, latest int64, postgres bool) (again bool, _ error) {
	_, err := admin.ExecContext(ctx, "CREATE DATABASE "+name)
	if err == nil {
		// This process creates it, so it makes it ready and migrates it.
		if err := migrations.PrepareTestDatabase(ctx, tpl); err != nil {
			return false, err
		}
		if _, err := provider.Up(ctx); err != nil {
			return false, fmt.Errorf("migrate the template: %w", err)
		}
		return false, nil
	}
	// Two processes that create the same database at the same instant: PostgreSQL answers the second with
	// 42P04 (duplicate_database) or, from the unique index of its catalog, 23505.
	if !isCode(err, "42P04") && (!postgres || !isCode(err, "23505")) {
		return false, fmt.Errorf("create the template: %w", err)
	}
	// Another process made it: wait until it is complete.
	err = waitForVersion(ctx, tpl, latest)
	if !errors.Is(err, errStalled) {
		return false, err
	}
	if _, err := admin.ExecContext(ctx, migrations.DropDatabaseSQL(postgres, name)); err != nil {
		return false, fmt.Errorf("drop a stalled template: %w", err)
	}
	return true, nil
}

// seededTables are the tables of the freshly migrated template that have rows:
// nothing but the migrations has written to it. goose's own table is left out,
// it is copied apart.
func seededTables(ctx context.Context, tpl *sql.DB, tables []string) ([]string, error) {
	var seeded []string
	for _, table := range tables {
		if table == "goose_db_version" {
			continue
		}
		var has bool
		query := "SELECT EXISTS (SELECT 1 FROM " + pgx.Identifier{"public", table}.Sanitize() + ")"
		if err := tpl.QueryRowContext(ctx, query).Scan(&has); err != nil {
			return nil, fmt.Errorf("look for the rows of the template's table %s: %w", table, err)
		}
		if has {
			seeded = append(seeded, table)
		}
	}
	return seeded, nil
}

// waitForVersion waits until the template has the latest migration. It
// returns errStalled when no migration lands for a minute and a half (one
// takes a few seconds), so a run killed while migrating does not leave the
// next ones waiting forever.
func waitForVersion(ctx context.Context, tpl *sql.DB, latest int64) error {
	last, lastChange := int64(-1), time.Now()
	for {
		var version int64
		err := tpl.QueryRowContext(ctx, `SELECT coalesce(max(version_id), 0) FROM goose_db_version`).Scan(&version)
		if isCode(err, "42P01") { // undefined_table: goose has not started yet
			err, version = nil, 0
		}
		if err != nil {
			return fmt.Errorf("read the template's version: %w", err)
		}
		if version >= latest {
			return nil
		}
		if version != last {
			last, lastChange = version, time.Now()
		} else if time.Since(lastChange) > 90*time.Second {
			return errStalled
		}
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-time.After(500 * time.Millisecond):
		}
	}
}

// references finds the tables a CREATE TABLE statement points at.
var references = regexp.MustCompile(`REFERENCES public\.(\w+)`)

// schemaDDL returns the statements that recreate the template's sequences
// and tables, with their indexes, CHECKs, foreign keys and options inline,
// as CockroachDB writes them (SHOW CREATE). A table comes after the ones it
// references, so every foreign key finds its table: creating a table with
// its foreign keys inline is quick, where adding them afterwards, as a
// migration does, costs a schema change job each.
func schemaDDL(ctx context.Context, tpl *sql.DB) ([]string, []string, error) {
	sequences, err := names(ctx, tpl, `SELECT sequence_name FROM information_schema.sequences WHERE sequence_schema = 'public' ORDER BY 1`)
	if err != nil {
		return nil, nil, err
	}
	tables, err := names(ctx, tpl, `SELECT table_name FROM information_schema.tables WHERE table_schema = 'public' AND table_type = 'BASE TABLE' ORDER BY 1`)
	if err != nil {
		return nil, nil, err
	}

	var ddl []string
	for _, s := range sequences {
		stmt, err := showCreate(ctx, tpl, "SEQUENCE", s)
		if err != nil {
			return nil, nil, err
		}
		ddl = append(ddl, stmt)
	}
	create := make(map[string]string, len(tables))
	for _, table := range tables {
		if create[table], err = showCreate(ctx, tpl, "TABLE", table); err != nil {
			return nil, nil, err
		}
	}
	order, err := orderByReferences(tables, create)
	if err != nil {
		return nil, nil, err
	}
	for _, table := range order {
		ddl = append(ddl, create[table])
	}
	return ddl, order, nil
}

// routineDDL returns the statements that recreate the template's functions, in the
// order they were made (a function may call one made before it), and then its triggers,
// which call them. CockroachDB writes the template's own name into a trigger's table
// (SHOW CREATE), which the copy drops, so the statements read the database they run in.
// They run after the tables: a trigger needs its table.
func routineDDL(ctx context.Context, tpl *sql.DB) ([]string, error) {
	var database string
	if err := tpl.QueryRowContext(ctx, `SELECT current_database()`).Scan(&database); err != nil {
		return nil, fmt.Errorf("name the template: %w", err)
	}
	functions, err := names(ctx, tpl, `
		SELECT p.proname FROM pg_proc p JOIN pg_namespace n ON n.oid = p.pronamespace
		WHERE n.nspname = 'public' ORDER BY p.oid`)
	if err != nil {
		return nil, err
	}
	var ddl []string
	for _, f := range functions {
		stmt, err := showCreateRoutine(ctx, tpl, "FUNCTION", pgx.Identifier{"public", f}.Sanitize(), database)
		if err != nil {
			return nil, err
		}
		ddl = append(ddl, stmt)
	}
	triggers, err := triggerTargets(ctx, tpl)
	if err != nil {
		return nil, err
	}
	for _, target := range triggers {
		stmt, err := showCreateRoutine(ctx, tpl, "TRIGGER", target, database)
		if err != nil {
			return nil, err
		}
		ddl = append(ddl, stmt)
	}
	return ddl, nil
}

// triggerTargets lists the template's triggers as SHOW CREATE TRIGGER wants them:
// "name ON table", quoted.
func triggerTargets(ctx context.Context, tpl *sql.DB) ([]string, error) {
	rows, err := tpl.QueryContext(ctx, `
		SELECT t.tgname, c.relname FROM pg_trigger t JOIN pg_class c ON c.oid = t.tgrelid
		WHERE NOT t.tgisinternal ORDER BY t.tgname`)
	if err != nil {
		return nil, fmt.Errorf("list the template's triggers: %w", err)
	}
	defer func() { _ = rows.Close() }()
	var out []string
	for rows.Next() {
		var name, table string
		if err := rows.Scan(&name, &table); err != nil {
			return nil, err
		}
		out = append(out, pgx.Identifier{name}.Sanitize()+" ON "+pgx.Identifier{"public", table}.Sanitize())
	}
	return out, rows.Err()
}

// showCreateRoutine asks CockroachDB how to create a function or trigger (target is
// its quoted name, and for a trigger also the table), without the template's name.
func showCreateRoutine(ctx context.Context, conn *sql.DB, kind, target, database string) (string, error) {
	var object, stmt string
	if err := conn.QueryRowContext(ctx, "SHOW CREATE "+kind+" "+target).Scan(&object, &stmt); err != nil {
		return "", fmt.Errorf("show create %s %s: %w", kind, target, err)
	}
	return strings.ReplaceAll(stmt, database+".public.", "public."), nil
}

// orderByReferences orders tables so that each comes after the tables its
// CREATE statement (create) references, in rounds over tables in their
// order; a table found ready counts as created for the rest of its round.
func orderByReferences(tables []string, create map[string]string) ([]string, error) {
	var order []string
	done := map[string]bool{}
	for len(done) < len(tables) {
		var progress bool
		order, progress = addReadyTables(tables, create, done, order)
		if !progress {
			left := slices.DeleteFunc(slices.Clone(tables), func(t string) bool { return done[t] })
			return nil, fmt.Errorf("foreign keys in a cycle among %s", strings.Join(left, ", "))
		}
	}
	return order, nil
}

// addReadyTables is one round: it marks done, and appends to order, every
// table not done yet whose references are all done.
func addReadyTables(tables []string, create map[string]string, done map[string]bool, order []string) (_ []string, progress bool) {
	for _, table := range tables {
		if done[table] || !referencesDone(create[table], table, done) {
			continue
		}
		done[table], progress = true, true
		order = append(order, table)
	}
	return order, progress
}

// referencesDone reports whether every table stmt references, other than
// table itself, is done.
func referencesDone(stmt, table string, done map[string]bool) bool {
	for _, m := range references.FindAllStringSubmatch(stmt, -1) {
		if ref := m[1]; ref != table && !done[ref] {
			return false
		}
	}
	return true
}

func names(ctx context.Context, conn *sql.DB, query string) ([]string, error) {
	rows, err := conn.QueryContext(ctx, query)
	if err != nil {
		return nil, fmt.Errorf("list the template's objects: %w", err)
	}
	defer func() { _ = rows.Close() }()
	var out []string
	for rows.Next() {
		var n string
		if err := rows.Scan(&n); err != nil {
			return nil, err
		}
		out = append(out, n)
	}
	return out, rows.Err()
}

// showCreate asks CockroachDB how to create the template's sequence or table
// name. The name comes from the template's own catalog; it is quoted anyway.
func showCreate(ctx context.Context, conn *sql.DB, kind, name string) (string, error) {
	var object, stmt string
	query := "SHOW CREATE " + kind + " " + pgx.Identifier{"public", name}.Sanitize()
	if err := conn.QueryRowContext(ctx, query).Scan(&object, &stmt); err != nil {
		return "", fmt.Errorf("show create %s %s: %w", kind, name, err)
	}
	return stmt, nil
}

func isCode(err error, code string) bool {
	pgErr, ok := errors.AsType[*pgconn.PgError](err)
	return ok && pgErr.Code == code
}

// withDatabase points a connection string at another database.
func withDatabase(rawURL, name string) (string, error) {
	u, err := url.Parse(rawURL)
	if err != nil {
		return "", err
	}
	u.Path = "/" + name
	return u.String(), nil
}

func open(t testing.TB, dsn string) *sql.DB {
	t.Helper()
	conn, err := sql.Open("pgx", dsn)
	if err != nil {
		t.Fatalf("open database: %v", err)
	}
	t.Cleanup(func() { _ = conn.Close() })
	return conn
}

func mustExec(t testing.TB, conn *sql.DB, query string) {
	t.Helper()
	// Not t.Context(): cleanups run after it is canceled.
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	if _, err := conn.ExecContext(ctx, query); err != nil {
		t.Fatalf("exec %q: %v", query, err)
	}
}
