package migrations

import (
	"context"
	"database/sql"
	"fmt"
	"go/ast"
	"go/parser"
	"go/token"
	"io/fs"
	"net/url"
	"path/filepath"
	"slices"
	"strconv"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	_ "github.com/jackc/pgx/v5/stdlib"

	"github.com/PuraFome/meuRPG/backend/internal/platform/testenv"
)

// TestMigrationsUpDownUp applies every migration to a brand-new database,
// rolls them all back, and applies them again. That proves each Down really
// undoes its Up, and that Up is safe to repeat.
//
// It needs MEURPG_TEST_DATABASE_URL (see internal/platform/db for how to
// start a local CockroachDB) and creates, then drops, its own database.
func TestMigrationsUpDownUp(t *testing.T) {
	t.Parallel()
	db := freshDatabase(t)
	ctx := t.Context()

	provider, err := NewProvider(db)
	if err != nil {
		t.Fatalf("NewProvider() error = %v", err)
	}

	if _, err := provider.Up(ctx); err != nil {
		t.Fatalf("Up() error = %v", err)
	}
	// Whatever the migrations create today; the checks below don't need a
	// hard-coded list, so adding a migration never requires editing this test.
	created := tables(t, db)

	// Running Up again with nothing pending must be a no-op.
	if results, err := provider.Up(ctx); err != nil || len(results) != 0 {
		t.Fatalf("second Up() = %d results, %v; want 0, nil", len(results), err)
	}

	if _, err := provider.DownTo(ctx, 0); err != nil {
		t.Fatalf("DownTo(0) error = %v", err)
	}
	if got := tables(t, db); len(got) != 0 {
		t.Errorf("tables after Down = %v, want none", got)
	}

	if _, err := provider.Up(ctx); err != nil {
		t.Fatalf("Up() after Down error = %v", err)
	}
	if got := tables(t, db); !slices.Equal(got, created) {
		t.Errorf("tables after Down then Up = %v, want %v", got, created)
	}
	version, err := provider.GetDBVersion(ctx)
	if err != nil {
		t.Fatalf("GetDBVersion() error = %v", err)
	}
	if sources := provider.ListSources(); version != sources[len(sources)-1].Version {
		t.Errorf("database version = %d, want the latest migration %d", version, sources[len(sources)-1].Version)
	}
}

// freshCounter numbers the databases freshDatabase creates.
var freshCounter atomic.Int64

// freshDatabase creates an empty database on the test server and returns a
// connection to it. The database is dropped when the test ends.
func freshDatabase(t *testing.T) *sql.DB {
	t.Helper()

	rawURL := testenv.DatabaseURL(t)

	admin := open(t, rawURL)
	// The clock alone is not unique: on macOS it ticks in microseconds, and two
	// parallel tests can read the same value. The counter makes each name
	// unique in the process, as dbtest does.
	name := fmt.Sprintf("meurpg_migrations_test_%d_%d", time.Now().UnixNano(), freshCounter.Add(1))
	exec(t, admin, "CREATE DATABASE "+name)
	t.Cleanup(func() { exec(t, admin, "DROP DATABASE IF EXISTS "+name+" CASCADE") })

	u, err := url.Parse(rawURL)
	if err != nil {
		t.Fatalf("parse MEURPG_TEST_DATABASE_URL: %v", err)
	}
	u.Path = "/" + name
	return open(t, u.String())
}

func open(t *testing.T, dsn string) *sql.DB {
	t.Helper()
	db, err := sql.Open("pgx", dsn)
	if err != nil {
		t.Fatalf("open database: %v", err)
	}
	t.Cleanup(func() { _ = db.Close() })
	return db
}

func exec(t *testing.T, db *sql.DB, query string) {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	if _, err := db.ExecContext(ctx, query); err != nil {
		t.Fatalf("exec %q: %v", query, err)
	}
}

// tables lists the application's tables, leaving out goose's own.
func tables(t *testing.T, db *sql.DB) []string {
	t.Helper()
	rows, err := db.QueryContext(t.Context(), `
		SELECT table_name FROM information_schema.tables
		WHERE table_schema = 'public' AND table_type = 'BASE TABLE'
		  AND table_name <> 'goose_db_version'
		ORDER BY table_name`)
	if err != nil {
		t.Fatalf("list tables: %v", err)
	}
	defer func() { _ = rows.Close() }()

	var names []string
	for rows.Next() {
		var name string
		if err := rows.Scan(&name); err != nil {
			t.Fatalf("scan table name: %v", err)
		}
		names = append(names, name)
	}
	if err := rows.Err(); err != nil {
		t.Fatalf("list tables: %v", err)
	}
	return names
}

// TestMigrationsAreSafeToRerun runs every migration's Up a second time over
// a migrated database, as happens when a migration fails halfway and is run
// again (CockroachDB commits before each DDL statement, see migrations.go).
// The second run must succeed and leave the schema exactly as it was: no
// "already exists" error and no duplicated constraint.
func TestMigrationsAreSafeToRerun(t *testing.T) {
	t.Parallel()
	db := freshDatabase(t)
	ctx := t.Context()

	provider, err := NewProvider(db)
	if err != nil {
		t.Fatalf("NewProvider() error = %v", err)
	}
	if _, err := provider.Up(ctx); err != nil {
		t.Fatalf("Up() error = %v", err)
	}
	before := schema(t, db)

	// Make goose forget every migration, so Up applies them all again.
	exec(t, db, "DELETE FROM goose_db_version WHERE version_id > 0")
	if _, err := provider.Up(ctx); err != nil {
		t.Fatalf("second Up() error = %v", err)
	}
	if after := schema(t, db); after != before {
		t.Errorf("schema changed when the migrations ran again.\nbefore:\n%s\nafter:\n%s", before, after)
	}
}

// schema returns the CREATE statements of every table, goose's excluded.
func schema(t *testing.T, db *sql.DB) string {
	t.Helper()
	var b strings.Builder
	for _, table := range tables(t, db) {
		var name, create string
		if err := db.QueryRowContext(t.Context(), "SHOW CREATE TABLE "+table).Scan(&name, &create); err != nil {
			t.Fatalf("SHOW CREATE TABLE %s: %v", table, err)
		}
		b.WriteString(create + "\n")
	}
	return b.String()
}

// TestMigrationsDownWorksOnTheatreRows: the Down of the combat modes (00143 to
// 00145) runs on a database that holds what they allow: a combat without a grid,
// and an opportunity offer the master made by hand, one withdrawn and one with a
// square. The combat without a grid and the offers without a square are deleted,
// a withdrawn offer becomes a skipped one, and the combat on a grid stays.
func TestMigrationsDownWorksOnTheatreRows(t *testing.T) {
	t.Parallel()
	db := freshDatabase(t)
	ctx := t.Context()
	provider, err := NewProvider(db)
	if err != nil {
		t.Fatalf("NewProvider() error = %v", err)
	}
	if _, err := provider.Up(ctx); err != nil {
		t.Fatalf("Up() error = %v", err)
	}
	scan := func(query string, dest ...any) {
		t.Helper()
		if err := db.QueryRowContext(ctx, query).Scan(dest...); err != nil {
			t.Fatalf("%s: %v", query, err)
		}
	}
	var user, campaign, session, character string
	scan(`INSERT INTO users DEFAULT VALUES RETURNING id`, &user)
	scan(fmt.Sprintf(`INSERT INTO campaigns (name, xp_mode, created_by) VALUES ('Mirathel', 'enemies', '%s') RETURNING id`, user), &campaign)
	scan(fmt.Sprintf(`INSERT INTO game_sessions (campaign_id, session_number, started_at) VALUES ('%s', 1, now()) RETURNING id`, campaign), &session)
	scan(fmt.Sprintf(`INSERT INTO characters (campaign_id, kind, name, sheet, master_user_id, created_at, updated_at)
		VALUES ('%s', 'minion', 'Goblin', '{}', '%s', now(), now()) RETURNING id`, campaign, user), &character)
	var theatre, grid string
	scan(fmt.Sprintf(`INSERT INTO encounters (game_session_id, name, status, grid_columns, grid_rows, created_at, mode)
		VALUES ('%s', 'Sem mapa', 'ended', 0, 0, now(), 'theatre') RETURNING id`, session), &theatre)
	scan(fmt.Sprintf(`INSERT INTO encounters (game_session_id, name, status, grid_columns, grid_rows, created_at, mode)
		VALUES ('%s', 'No mapa', 'ended', 20, 10, now(), 'grid') RETURNING id`, session), &grid)
	combatant := func(encounter, label string) string {
		var id string
		scan(fmt.Sprintf(`INSERT INTO combatants (encounter_id, character_id, label, kind, hidden, initiative_bonus, order_index, speed_ft, created_at, hp_current, hp_max, hp_temp)
			VALUES ('%s', '%s', '%s', 'npc', false, 0, 0, 30, now(), 7, 7, 0) RETURNING id`, encounter, character, label), &id)
		return id
	}
	a, b := combatant(grid, "Goblin 1"), combatant(grid, "Goblin 2")
	ta, tb := combatant(theatre, "Goblin 1"), combatant(theatre, "Goblin 2")
	exec(t, db, fmt.Sprintf(`INSERT INTO opportunity_offers (encounter_id, move_id, mover_id, reactor_id, left_col, left_row, state, created_at) VALUES
		('%[1]s', gen_random_uuid(), '%[2]s', '%[3]s', 4, 5, 'pending', now()),
		('%[1]s', gen_random_uuid(), '%[3]s', '%[2]s', 4, 5, 'withdrawn', now()),
		('%[4]s', gen_random_uuid(), '%[5]s', '%[6]s', NULL, NULL, 'pending', now()),
		('%[4]s', gen_random_uuid(), '%[6]s', '%[5]s', NULL, NULL, 'withdrawn', now())`, grid, a, b, theatre, ta, tb))

	if _, err := provider.DownTo(ctx, 142); err != nil {
		t.Fatalf("DownTo(142) error = %v", err)
	}
	var encounters, offers int
	scan(`SELECT count(*) FROM encounters`, &encounters)
	scan(`SELECT count(*) FROM opportunity_offers`, &offers)
	if encounters != 1 || offers != 2 {
		t.Errorf("after Down: %d encounters and %d offers, want the combat on a grid and its two offers", encounters, offers)
	}
	var skipped int
	scan(`SELECT count(*) FROM opportunity_offers WHERE state = 'skipped'`, &skipped)
	if skipped != 1 {
		t.Errorf("after Down: %d skipped offers, want the withdrawn one turned into one", skipped)
	}
	if _, err := provider.Up(ctx); err != nil {
		t.Fatalf("Up() after Down error = %v", err)
	}
	scan(`SELECT count(*) FROM encounters WHERE mode = 'grid'`, &encounters)
	if encounters != 1 {
		t.Errorf("after Down then Up: %d combats on a grid, want 1", encounters)
	}
}

// TestMigrationsBackfillAChainOfEditsOfAnyDepth: 00167 marks a textured map and every edit below it as showing the
// whole map, however long the chain (RN-10: the app never shows such an image with one tap), and leaves a scene art
// and its edits alone. It also runs again without changing anything.
func TestMigrationsBackfillAChainOfEditsOfAnyDepth(t *testing.T) {
	t.Parallel()
	db := freshDatabase(t)
	ctx := t.Context()
	provider, err := NewProvider(db)
	if err != nil {
		t.Fatalf("NewProvider() error = %v", err)
	}
	if _, err := provider.UpTo(ctx, 166); err != nil {
		t.Fatalf("UpTo(166) error = %v", err)
	}
	scan := func(query string, dest ...any) {
		t.Helper()
		if err := db.QueryRowContext(ctx, query).Scan(dest...); err != nil {
			t.Fatalf("%s: %v", query, err)
		}
	}
	var user, campaign string
	scan(`INSERT INTO users DEFAULT VALUES RETURNING id`, &user)
	scan(fmt.Sprintf(`INSERT INTO campaigns (name, xp_mode, created_by) VALUES ('Mirathel', 'enemies', '%s') RETURNING id`, user), &campaign)
	image := func(parent string) string {
		var id string
		p := "NULL"
		if parent != "" {
			p = "'" + parent + "'"
		}
		scan(fmt.Sprintf(`INSERT INTO gallery_images (id, campaign_id, name, content_type, width, height, byte_size, created_at, generated, parent_image_id)
			VALUES (gen_random_uuid(), '%s', 'x', 'image/png', 10, 10, 10, now(), true, %s) RETURNING id`, campaign, p), &id)
		return id
	}
	n := 0
	request := func(kind, imageID string) {
		n++
		exec(t, db, fmt.Sprintf(`INSERT INTO image_requests (id, campaign_id, idempotency_key, kind, prompt, style, aspect_ratio, model, reference_ids, character_ids,
			number, quota_month, status, reason, refunded, image_id, created_at)
			VALUES (gen_random_uuid(), '%s', 'k%d', '%s', 'p', '', '16:9', 'm', '{}', '{}', %d, '2026-10', 'done', '', false, '%s', now())`, campaign, n, kind, n, imageID))
	}
	texture := image("")
	request("textured_map", texture)
	chain := []string{texture}
	for range 5 {
		chain = append(chain, image(chain[len(chain)-1]))
	}
	scene := image("")
	request("map_scene", scene)
	sceneEdit := image(scene)

	if _, err := provider.UpTo(ctx, 167); err != nil {
		t.Fatalf("UpTo(167) error = %v", err)
	}
	marked := func(id string) string {
		var kind string
		scan(fmt.Sprintf(`SELECT generated_kind FROM gallery_images WHERE id = '%s'`, id), &kind)
		return kind
	}
	for i, id := range chain {
		if got := marked(id); got != "textured_map" {
			t.Errorf("image %d of the chain of a textured map is %q, want textured_map", i, got)
		}
	}
	for name, id := range map[string]string{"the scene art": scene, "its edit": sceneEdit} {
		if got := marked(id); got != "" {
			t.Errorf("%s is %q, want it left alone", name, got)
		}
	}
	// Down does nothing to the data, and Up again changes nothing.
	if _, err := provider.DownTo(ctx, 166); err != nil {
		t.Fatalf("DownTo(166) error = %v", err)
	}
	if _, err := provider.UpTo(ctx, 167); err != nil {
		t.Fatalf("UpTo(167) again error = %v", err)
	}
	if got := marked(chain[len(chain)-1]); got != "textured_map" {
		t.Errorf("the deepest edit after a second run is %q", got)
	}
}

// TestEveryEventKindOfTheCodeIsInTheTable: (it also checks that the kind is limited
// by a foreign key and no longer by the CHECK of D-02.) A session event the Go code can write
// has a row in session_event_kinds, or its INSERT would fail the foreign key. The
// kinds are the `event...` string constants of internal/ (play, maps, progression);
// a new one without its migration fails here, not at a table.
func TestEveryEventKindOfTheCodeIsInTheTable(t *testing.T) {
	t.Parallel()
	db := freshDatabase(t)
	provider, err := NewProvider(db)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := provider.Up(t.Context()); err != nil {
		t.Fatalf("Up() error = %v", err)
	}

	var create string
	var name string
	if err := db.QueryRowContext(t.Context(), `SHOW CREATE TABLE session_events`).Scan(&name, &create); err != nil {
		t.Fatal(err)
	}
	if strings.Contains(create, "session_events_kind_valid") {
		t.Error("session_events still has the kind CHECK")
	}
	if !strings.Contains(create, "session_events_kind_fkey FOREIGN KEY (kind) REFERENCES public.session_event_kinds(kind)") {
		t.Errorf("session_events has no foreign key on kind:\n%s", create)
	}

	inTable := map[string]bool{}
	rows, err := db.QueryContext(t.Context(), `SELECT kind FROM session_event_kinds`)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = rows.Close() }()
	for rows.Next() {
		var k string
		if err := rows.Scan(&k); err != nil {
			t.Fatal(err)
		}
		inTable[k] = true
	}
	if err := rows.Err(); err != nil {
		t.Fatal(err)
	}

	kinds := eventKindsInCode(t, "../internal")
	if len(kinds) < 50 {
		t.Fatalf("found only %d event kinds in the code; the scan is broken", len(kinds))
	}
	for kind, where := range kinds {
		if !inTable[kind] {
			t.Errorf("event kind %q (%s) is not in session_event_kinds: add an INSERT in a new migration", kind, where)
		}
	}
	// And the other way: a row nobody writes is a stale list.
	for kind := range inTable {
		if _, ok := kinds[kind]; !ok {
			t.Errorf("session_event_kinds has %q, which no event... constant in the code writes", kind)
		}
	}
}

// eventKindsInCode returns every string constant named event<Something> in the
// non-test Go files under dir, with where it is.
func eventKindsInCode(t *testing.T, dir string) map[string]string {
	t.Helper()
	out := map[string]string{}
	fset := token.NewFileSet()
	err := filepath.WalkDir(dir, func(path string, d fs.DirEntry, err error) error {
		if err != nil || d.IsDir() || !strings.HasSuffix(path, ".go") || strings.HasSuffix(path, "_test.go") {
			return err
		}
		file, err := parser.ParseFile(fset, path, nil, 0)
		if err != nil {
			return err
		}
		for _, decl := range file.Decls {
			gen, ok := decl.(*ast.GenDecl)
			if !ok || gen.Tok != token.CONST {
				continue
			}
			for _, spec := range gen.Specs {
				vs := spec.(*ast.ValueSpec)
				for i, name := range vs.Names {
					if !strings.HasPrefix(name.Name, "event") || i >= len(vs.Values) {
						continue
					}
					lit, ok := vs.Values[i].(*ast.BasicLit)
					if !ok || lit.Kind != token.STRING {
						continue
					}
					if v, err := strconv.Unquote(lit.Value); err == nil {
						out[v] = filepath.ToSlash(path) + " " + name.Name
					}
				}
			}
		}
		return nil
	})
	if err != nil {
		t.Fatalf("scan the code for event kinds: %v", err)
	}
	return out
}

// A foreign key with ON DELETE CASCADE or SET NULL looks the child rows up by
// the key when the parent goes. On the big tables (the session history, the
// damage and the combatants of every combat) that lookup needs an index, or
// deleting a character, an account or a combatant scans the whole table.
func TestCascadeForeignKeysOfBigTablesAreIndexed(t *testing.T) {
	sdb := freshDatabase(t)
	provider, err := NewProvider(sdb)
	if err != nil {
		t.Fatalf("NewProvider: %v", err)
	}
	if _, err := provider.Up(t.Context()); err != nil {
		t.Fatalf("Up: %v", err)
	}
	run := func(q string) []string {
		t.Helper()
		rows, err := sdb.QueryContext(t.Context(), q)
		if err != nil {
			t.Fatalf("%s: %v", q, err)
		}
		defer rows.Close()
		var out []string
		for rows.Next() {
			var s string
			if err := rows.Scan(&s); err != nil {
				t.Fatalf("scan: %v", err)
			}
			out = append(out, s)
		}
		if err := rows.Err(); err != nil {
			t.Fatalf("rows: %v", err)
		}
		return out
	}
	bigTables := []string{"session_events", "pending_damages", "combatants"}

	rows := run(`
		SELECT c.conrelid::regclass::string || '.' || a.attname
		FROM pg_constraint c
		JOIN pg_attribute a ON a.attrelid = c.conrelid AND a.attnum = c.conkey[1]
		WHERE c.contype = 'f' AND c.confdeltype IN ('c','n','d')
		  AND NOT EXISTS (
		    SELECT 1 FROM pg_index i
		    WHERE i.indrelid = c.conrelid AND i.indkey[0] = c.conkey[1])
		ORDER BY 1`)
	for _, r := range rows {
		if table, _, _ := strings.Cut(r, "."); slices.Contains(bigTables, table) {
			t.Errorf("%s is a cascade or SET NULL foreign key of a big table without an index", r)
		}
	}
}
