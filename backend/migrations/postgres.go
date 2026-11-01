package migrations

import (
	"context"
	"database/sql"
	"fmt"
	"io/fs"
	"regexp"
	"strings"
	"testing/fstest"

	"github.com/pressly/goose/v3"
)

// The migrations are written for CockroachDB, and production applies them
// byte for byte, so they never change to please another database.
// The tests can also run on PostgreSQL.
// This file is the one place that adapts the files to it.
// NewTestProvider and NewAdaptedProvider read them with the CockroachDB-only
// parts cut or stood in for.
// Only tests call it.
//
// What PostgreSQL lacks, and what stands in for it:
//
//   - Row-level TTL (WITH (ttl_...) on CREATE TABLE, ALTER TABLE ... SET
//     (ttl_...)): the clause is cut.
//     PostgreSQL never deletes an expired row by itself, so a test that
//     needs the database to do it fails on PostgreSQL (docs/testing.md).
//   - The STRING type: a domain over TEXT.
//   - unique_rowid(): a function over a sequence.
//
// A new migration should not need an entry here; if it does, the SQL rule in
// docs/data/README.md says to write it dialect-neutral instead.
var (
	ttlOnCreate = regexp.MustCompile(`(?s)\)\s*WITH\s*\(\s*ttl_[^;]*?\)\s*;`)
	ttlOnAlter  = regexp.MustCompile(`(?s)ALTER TABLE \w+ SET \(\s*ttl_.*?\);`)
)

// postgresPrelude makes the types and functions of CockroachDB that the
// migrations use. It can run again on the same database, and from several
// processes at once: the advisory lock makes them take turns.
const postgresPrelude = `
BEGIN;
SELECT pg_advisory_xact_lock(7280001);
DO $$
BEGIN
	IF NOT EXISTS (SELECT 1 FROM pg_type WHERE typname = 'string') THEN
		CREATE DOMAIN string AS text;
	END IF;
END
$$;
CREATE SEQUENCE IF NOT EXISTS unique_rowid_seq;
CREATE OR REPLACE FUNCTION unique_rowid() RETURNS bigint LANGUAGE sql AS 'SELECT nextval(''unique_rowid_seq'')';
COMMIT;
`

// IsPostgres reports whether the server behind db is PostgreSQL rather than
// CockroachDB.
func IsPostgres(ctx context.Context, db *sql.DB) (bool, error) {
	var version string
	if err := db.QueryRowContext(ctx, `SELECT version()`).Scan(&version); err != nil {
		return false, err
	}
	return !strings.Contains(version, "CockroachDB"), nil
}

// PrepareTestDatabase makes what the migrations expect of a PostgreSQL
// database: the prelude above. It does nothing on CockroachDB.
func PrepareTestDatabase(ctx context.Context, db *sql.DB) error {
	postgres, err := IsPostgres(ctx, db)
	if err != nil || !postgres {
		return err
	}
	if _, err := db.ExecContext(ctx, postgresPrelude); err != nil {
		return fmt.Errorf("prepare the PostgreSQL database for the migrations: %w", err)
	}
	return nil
}

// NewTestProvider is NewProvider for a test database of either kind, made
// ready first (PrepareTestDatabase).
func NewTestProvider(ctx context.Context, db *sql.DB, opts ...goose.ProviderOption) (*goose.Provider, error) {
	if err := PrepareTestDatabase(ctx, db); err != nil {
		return nil, err
	}
	postgres, err := IsPostgres(ctx, db)
	if err != nil {
		return nil, err
	}
	return NewAdaptedProvider(db, postgres, opts...)
}

// NewAdaptedProvider is NewProvider, or on PostgreSQL the provider of the
// files without their TTL clauses. It does not talk to the database.
func NewAdaptedProvider(db *sql.DB, postgres bool, opts ...goose.ProviderOption) (*goose.Provider, error) {
	if !postgres {
		return NewProvider(db, opts...)
	}
	names, err := fs.Glob(files, "*.sql")
	if err != nil {
		return nil, err
	}
	adapted := fstest.MapFS{}
	for _, name := range names {
		body, err := files.ReadFile(name)
		if err != nil {
			return nil, err
		}
		body = ttlOnCreate.ReplaceAll(body, []byte(");"))
		body = ttlOnAlter.ReplaceAll(body, nil)
		adapted[name] = &fstest.MapFile{Data: body}
	}
	return goose.NewProvider(goose.DialectPostgres, db, adapted, opts...)
}

// DropDatabaseSQL drops a test database and what is in it, even while a
// connection is open: CockroachDB spells that CASCADE, PostgreSQL WITH (FORCE).
func DropDatabaseSQL(postgres bool, name string) string {
	if postgres {
		return "DROP DATABASE IF EXISTS " + name + " WITH (FORCE)"
	}
	return "DROP DATABASE IF EXISTS " + name + " CASCADE"
}
