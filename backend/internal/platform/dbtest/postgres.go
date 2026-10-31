package dbtest

// EXPERIMENT (exp/postgres-tests): the parts of the kit that differ on
// PostgreSQL. CockroachDB's SHOW CREATE and its cross-database INSERT ...
// SELECT do not exist there. Two ways to copy the template:
//
//   - native (default): CREATE DATABASE ... TEMPLATE, one statement; the rows of
//     the reference tables come with it.
//   - dump: the template-and-DELETE design as it is on CockroachDB: the
//     template's schema (pg_dump --schema-only) is replayed into a new database
//     and the reference rows are copied with COPY. MEURPG_TEST_PG_CLONE=dump.
//
// MEURPG_TEST_PG_REUSE=0 makes a test drop its database instead of handing it
// to the next test (only sensible with the native copy).

import (
	"bytes"
	"context"
	"database/sql"
	"fmt"
	"os"
	"os/exec"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"
)

const (
	cloneEnv = "MEURPG_TEST_PG_CLONE"
	reuseEnv = "MEURPG_TEST_PG_REUSE"
)

func pgNative() bool { return os.Getenv(cloneEnv) != "dump" }
func pgReuse() bool  { return os.Getenv(reuseEnv) != "0" }

// isPostgres reports whether the server is PostgreSQL rather than CockroachDB.
func isPostgres(ctx context.Context, admin *sql.DB) (bool, error) {
	var v string
	if err := admin.QueryRowContext(ctx, `SELECT version()`).Scan(&v); err != nil {
		return false, err
	}
	return !strings.Contains(v, "CockroachDB"), nil
}

// pgTemplateInfo reads what the copy needs from the migrated PostgreSQL template.
func pgTemplateInfo(ctx context.Context, tpl *sql.DB, dsn string) (templateDB, error) {
	tables, err := names(ctx, tpl, `SELECT table_name FROM information_schema.tables WHERE table_schema = 'public' AND table_type = 'BASE TABLE' ORDER BY 1`)
	if err != nil {
		return templateDB{}, err
	}
	// foreign keys, as the "REFERENCES public.x" text orderByReferences reads
	rows, err := tpl.QueryContext(ctx, `
		SELECT c.conrelid::regclass::text, r.relname
		FROM pg_constraint c JOIN pg_class r ON r.oid = c.confrelid
		WHERE c.contype = 'f' AND c.connamespace = 'public'::regnamespace`)
	if err != nil {
		return templateDB{}, err
	}
	create := map[string]string{}
	for rows.Next() {
		var child, parent string
		if err := rows.Scan(&child, &parent); err != nil {
			_ = rows.Close()
			return templateDB{}, err
		}
		create[strings.Trim(child, `"`)] += " REFERENCES public." + parent
	}
	_ = rows.Close()
	order, err := orderByReferences(tables, create)
	if err != nil {
		return templateDB{}, err
	}
	reference, err := seededTables(ctx, tpl, order)
	if err != nil {
		return templateDB{}, err
	}
	out := templateDB{ddl: nil, tables: order, reference: reference, pg: true}
	if pgNative() {
		return out, nil
	}
	dump, err := exec.CommandContext(ctx, "pg_dump", "--schema-only", "--no-owner", "--no-privileges", "--dbname="+dsn).Output()
	if err != nil {
		return templateDB{}, fmt.Errorf("pg_dump the template: %w", err)
	}
	var b strings.Builder
	for _, line := range strings.Split(string(dump), "\n") {
		if strings.HasPrefix(line, `\`) || strings.Contains(line, "set_config('search_path'") {
			continue // psql meta-commands and the empty search_path
		}
		b.WriteString(line + "\n")
	}
	out.pgDump = b.String()
	// The rows of the reference tables and goose's own, read once.
	out.refData = map[string][]byte{}
	conn, err := pgx.Connect(ctx, dsn)
	if err != nil {
		return templateDB{}, err
	}
	defer func() { _ = conn.Close(ctx) }()
	for _, table := range append(append([]string{}, reference...), "goose_db_version") {
		var buf bytes.Buffer
		if _, err := conn.PgConn().CopyTo(ctx, &buf, "COPY "+pgx.Identifier{"public", table}.Sanitize()+" TO STDOUT"); err != nil {
			return templateDB{}, fmt.Errorf("copy out %s: %w", table, err)
		}
		out.refData[table] = buf.Bytes()
	}
	return out, nil
}

// pgClone makes the database name from the template, either way.
func pgClone(ctx context.Context, admin *sql.DB, tpl templateDB, name, dsn string) error {
	if pgNative() {
		return pgCreateFromTemplate(ctx, admin, tpl.name, name)
	}
	if _, err := admin.ExecContext(ctx, "CREATE DATABASE "+name); err != nil {
		return err
	}
	conn, err := pgx.Connect(ctx, dsn)
	if err != nil {
		return err
	}
	defer func() { _ = conn.Close(ctx) }()
	if _, err := conn.Exec(ctx, tpl.pgDump); err != nil {
		return fmt.Errorf("replay the template's schema: %w", err)
	}
	for table, data := range tpl.refData {
		if _, err := conn.PgConn().CopyFrom(ctx, bytes.NewReader(data), "COPY "+pgx.Identifier{"public", table}.Sanitize()+" FROM STDIN"); err != nil {
			return fmt.Errorf("copy in %s: %w", table, err)
		}
	}
	_, err = conn.Exec(ctx, `SELECT setval('goose_db_version_id_seq', (SELECT max(id) FROM goose_db_version))`)
	return err
}

// pgCreateFromTemplate retries while another session is connected to the
// template (55006), which another test process waiting for it may be.
func pgCreateFromTemplate(ctx context.Context, admin *sql.DB, template, name string) error {
	for {
		_, err := admin.ExecContext(ctx, "CREATE DATABASE "+name+" TEMPLATE "+template)
		if !isCode(err, "55006") && !isCode(err, "40P01") {
			return err
		}
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-time.After(20 * time.Millisecond):
		}
	}
}
