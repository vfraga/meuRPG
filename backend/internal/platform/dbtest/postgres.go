package dbtest

// The parts of the kit that differ on PostgreSQL. CockroachDB's template is
// copied statement by statement (SHOW CREATE, and INSERT ... SELECT across
// databases for the reference rows), none of which PostgreSQL has; it copies a
// whole database with CREATE DATABASE ... TEMPLATE, rows included, in a few
// hundredths of a second. The server decides which path runs: see
// migrations.IsPostgres.

import (
	"context"
	"database/sql"
	"fmt"
	"strings"
	"time"
)

// postgresTemplate reads what emptying a database needs from the migrated
// PostgreSQL template: its tables, each after the ones it references, and the
// ones the migrations fill. Nothing else is read: the copy is the server's.
func postgresTemplate(ctx context.Context, tpl *sql.DB, name string) (templateDB, error) {
	tables, err := names(ctx, tpl, `SELECT table_name FROM information_schema.tables WHERE table_schema = 'public' AND table_type = 'BASE TABLE' ORDER BY 1`)
	if err != nil {
		return templateDB{}, err
	}
	rows, err := tpl.QueryContext(ctx, `
		SELECT c.conrelid::regclass::text, r.relname
		FROM pg_constraint c JOIN pg_class r ON r.oid = c.confrelid
		WHERE c.contype = 'f' AND c.connamespace = 'public'::regnamespace`)
	if err != nil {
		return templateDB{}, fmt.Errorf("list the template's foreign keys: %w", err)
	}
	defer func() { _ = rows.Close() }()
	// orderByReferences reads "REFERENCES public.x" in a table's statement.
	create := make(map[string]string, len(tables))
	for rows.Next() {
		var child, parent string
		if err := rows.Scan(&child, &parent); err != nil {
			return templateDB{}, err
		}
		child = strings.Trim(child, `"`)
		create[child] += " REFERENCES public." + parent
	}
	if err := rows.Err(); err != nil {
		return templateDB{}, err
	}
	order, err := orderByReferences(tables, create)
	if err != nil {
		return templateDB{}, err
	}
	reference, err := seededTables(ctx, tpl, order)
	if err != nil {
		return templateDB{}, err
	}
	return templateDB{name: name, tables: order, reference: reference, postgres: true}, nil
}

// copyPostgresTemplate makes the database name from the template. PostgreSQL
// refuses while another session is connected to the template (55006: another
// test process, still waiting for it to be migrated, or copying it), so it
// tries again until that session is gone.
func copyPostgresTemplate(ctx context.Context, admin *sql.DB, template, name string) error {
	for {
		_, err := admin.ExecContext(ctx, "CREATE DATABASE "+name+" TEMPLATE "+template)
		if !isCode(err, "55006") && !isCode(err, "40P01") { // object_in_use, deadlock_detected
			return err
		}
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-time.After(20 * time.Millisecond):
		}
	}
}
