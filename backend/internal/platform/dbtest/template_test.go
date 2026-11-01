package dbtest

import (
	"context"
	"database/sql"
	"fmt"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/PuraFome/meuRPG/backend/internal/platform/testenv"
	"github.com/PuraFome/meuRPG/backend/migrations"
)

// TestMain drops the databases this package's own tests take from NewPool.
func TestMain(m *testing.M) { Main(m) }

// scratchDatabase creates an empty database with these statements run in it,
// and returns a connection to it. It is dropped when the test ends.
func scratchDatabase(t *testing.T, label string, stmts ...string) *sql.DB {
	t.Helper()
	rawURL := testenv.DatabaseURL(t)
	admin := open(t, rawURL)
	name := fmt.Sprintf("meurpg_dbtest_%s_%d", label, time.Now().UnixNano())
	mustExec(t, admin, "CREATE DATABASE "+name)
	t.Cleanup(func() { mustExec(t, admin, migrations.DropDatabaseSQL(template.postgres, name)) })
	dsn, err := withDatabase(rawURL, name)
	if err != nil {
		t.Fatal(err)
	}
	conn := open(t, dsn)
	for _, stmt := range stmts {
		mustExec(t, conn, stmt)
	}
	return conn
}

// TestSchemaDDLOrdersTablesAfterTheirReferences checks schemaDDL: the
// sequences first, then the tables in rounds, each round taking (in name
// order) the tables whose references are all created; a table that points
// at itself is ready at once.
func TestSchemaDDLOrdersTablesAfterTheirReferences(t *testing.T) {
	t.Parallel()
	conn := scratchDatabase(t, "ddl",
		`CREATE SEQUENCE s_b`,
		`CREATE SEQUENCE s_a`,
		`CREATE TABLE z_root (id INT PRIMARY KEY)`,
		`CREATE TABLE b_parent (id INT PRIMARY KEY, root INT REFERENCES z_root (id))`,
		`CREATE TABLE c_child (id INT PRIMARY KEY, parent INT REFERENCES b_parent (id), root INT REFERENCES z_root (id))`,
		`CREATE TABLE a_self (id INT PRIMARY KEY, up INT REFERENCES a_self (id))`,
		`CREATE VIEW v_roots AS SELECT id FROM z_root`,
	)
	ddl, order, err := schemaDDL(t.Context(), conn)
	if err != nil {
		t.Fatalf("schemaDDL() error = %v", err)
	}
	wantOrder := []string{"a_self", "z_root", "b_parent", "c_child"}
	if !slices.Equal(order, wantOrder) {
		t.Errorf("order = %v, want %v", order, wantOrder)
	}
	wantPrefixes := []string{
		"CREATE SEQUENCE public.s_a ",
		"CREATE SEQUENCE public.s_b ",
		"CREATE TABLE public.a_self (",
		"CREATE TABLE public.z_root (",
		"CREATE TABLE public.b_parent (",
		"CREATE TABLE public.c_child (",
	}
	if len(ddl) != len(wantPrefixes) {
		t.Fatalf("ddl has %d statements, want %d:\n%s", len(ddl), len(wantPrefixes), strings.Join(ddl, "\n"))
	}
	for i, prefix := range wantPrefixes {
		if !strings.HasPrefix(ddl[i], prefix) {
			t.Errorf("ddl[%d] = %q, want it to start with %q", i, ddl[i], prefix)
		}
	}
	if !strings.Contains(ddl[5], "REFERENCES public.b_parent(id)") || !strings.Contains(ddl[5], "REFERENCES public.z_root(id)") {
		t.Errorf("the foreign keys are not inline: %s", ddl[5])
	}
}

// TestSchemaDDLRefusesACycle checks the error for foreign keys in a cycle:
// it names, in name order, every table left once no round makes progress
// (the tables that only depend on the cycle, too).
func TestSchemaDDLRefusesACycle(t *testing.T) {
	t.Parallel()
	conn := scratchDatabase(t, "cycle",
		`CREATE TABLE free (id INT PRIMARY KEY)`,
		`CREATE TABLE x_cyc (id INT PRIMARY KEY, y INT)`,
		`CREATE TABLE y_cyc (id INT PRIMARY KEY, x INT REFERENCES x_cyc (id))`,
		`ALTER TABLE x_cyc ADD CONSTRAINT x_y FOREIGN KEY (y) REFERENCES y_cyc (id)`,
		`CREATE TABLE after_cyc (id INT PRIMARY KEY, x INT REFERENCES x_cyc (id))`,
	)
	ddl, order, err := schemaDDL(t.Context(), conn)
	if want := "foreign keys in a cycle among after_cyc, x_cyc, y_cyc"; err == nil || err.Error() != want {
		t.Errorf("schemaDDL() error = %v, want %q", err, want)
	}
	if ddl != nil || order != nil {
		t.Errorf("schemaDDL() = %v, %v with its error", ddl, order)
	}
}

// TestSchemaDDLErrors checks the error of a catalog that cannot be read.
func TestSchemaDDLErrors(t *testing.T) {
	t.Parallel()
	conn := scratchDatabase(t, "closed")
	_ = conn.Close()
	_, _, err := schemaDDL(t.Context(), conn)
	if want := "list the template's objects: sql: database is closed"; err == nil || err.Error() != want {
		t.Errorf("schemaDDL() error = %v, want %q", err, want)
	}
}

// TestBuildTemplateErrors checks buildTemplate's refusals: a URL that does
// not parse, and a server that does not answer.
func TestBuildTemplateErrors(t *testing.T) {
	t.Parallel()
	if _, err := buildTemplate("postgresql://[::1"); err == nil || err.Error() != `parse "postgresql://[::1": missing ']' in host` {
		t.Errorf("buildTemplate(bad URL) error = %v", err)
	}
	_, err := buildTemplate("postgresql://root@127.0.0.1:1/none?sslmode=disable&connect_timeout=5")
	want := "create the template: failed to connect to `user=root database=none`: 127.0.0.1:1 (127.0.0.1): dial error: dial tcp 127.0.0.1:1: connect: connection refused"
	if err == nil || err.Error() != want {
		t.Errorf("buildTemplate(unreachable) error = %v, want %q", err, want)
	}
}

// TestBuildTemplateFindsTheMigratedTemplate checks the path of every process
// but the first: the template exists (CREATE DATABASE says 42P04) and is
// complete, so it is read, not migrated again, and gives what the first
// process got.
func TestBuildTemplateFindsTheMigratedTemplate(t *testing.T) {
	t.Parallel()
	rawURL := testenv.DatabaseURL(t)
	first, err := buildTemplate(rawURL)
	if err != nil {
		t.Fatalf("buildTemplate() error = %v", err)
	}
	again, err := buildTemplate(rawURL)
	if err != nil {
		t.Fatalf("second buildTemplate() error = %v", err)
	}
	fingerprint, err := migrations.Fingerprint()
	if err != nil {
		t.Fatal(err)
	}
	if again.name != "meurpg_tpl_"+fingerprint || again.name != first.name {
		t.Errorf("template name = %q, want meurpg_tpl_%s", again.name, fingerprint)
	}
	if !slices.Equal(again.ddl, first.ddl) || !slices.Equal(again.tables, first.tables) || !slices.Equal(again.reference, first.reference) {
		t.Error("the second build read another template")
	}
	if !slices.Contains(again.tables, "goose_db_version") || !slices.Contains(again.reference, "session_event_kinds") || slices.Contains(again.reference, "goose_db_version") {
		t.Errorf("tables %v, reference %v", again.tables, again.reference)
	}
	if again.postgres {
		if len(again.ddl) != 0 {
			t.Errorf("a PostgreSQL template keeps no DDL (it is copied natively), got %d statements", len(again.ddl))
		}
	} else if len(again.ddl) == 0 || !strings.HasPrefix(again.ddl[0], "CREATE SEQUENCE public.goose_db_version_id_seq ") {
		t.Errorf("ddl = %q, want goose's sequence first", again.ddl)
	}
}

// TestGooseIDsOfACopiedDatabase documents a known defect, not fixed: the template's
// DDL creates goose's identity sequence (goose_db_version_id_seq) as a
// sequence of its own, before goose_db_version, whose identity column then
// makes another one. NewPool's setval moves the copy, not the sequence the
// table uses, so the next row goose inserts in a test database (after a
// down, say) collides with a copied id. No test runs goose on a test
// database today.
func TestGooseIDsOfACopiedDatabase(t *testing.T) {
	t.Parallel()
	pool := NewPool(t, "meurpg_dbtest_goose")
	ctx, cancel := context.WithTimeout(t.Context(), time.Minute)
	defer cancel()
	var used string
	if err := pool.QueryRow(ctx, `SELECT pg_get_serial_sequence('goose_db_version', 'id')`).Scan(&used); err != nil {
		t.Fatal(err)
	}
	var version string
	if err := pool.QueryRow(ctx, `SELECT version()`).Scan(&version); err != nil {
		t.Fatal(err)
	}
	// A PostgreSQL copy is native, so the table keeps the sequence it was made with; the DDL of
	// CockroachDB's copy is where the defect is.
	if postgres := !strings.Contains(version, "CockroachDB"); postgres != (used == "public.goose_db_version_id_seq") {
		t.Errorf("goose_db_version uses %q on %q: the finding changed, update this test", used, version)
	}
}
