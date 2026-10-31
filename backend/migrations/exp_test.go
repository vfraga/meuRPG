package migrations

// EXPERIMENT (exp/postgres-tests): the test helpers pick the dialect of the server.

import (
	"context"
	"database/sql"
	"strings"
	"testing"

	"github.com/pressly/goose/v3"
)

func isPG(t testing.TB, db *sql.DB) bool {
	t.Helper()
	var v string
	if err := db.QueryRowContext(context.Background(), `SELECT version()`).Scan(&v); err != nil {
		t.Fatal(err)
	}
	return !strings.Contains(v, "CockroachDB")
}

// testProvider is NewProvider, or the PostgreSQL one with its prelude run.
func testProvider(t testing.TB, db *sql.DB, opts ...goose.ProviderOption) (*goose.Provider, error) {
	t.Helper()
	if !isPG(t, db) {
		return NewProvider(db, opts...)
	}
	if _, err := db.ExecContext(t.Context(), `CREATE SEQUENCE IF NOT EXISTS unique_rowid_seq`); err != nil {
		return nil, err
	}
	if _, err := db.ExecContext(t.Context(), `CREATE OR REPLACE FUNCTION unique_rowid() RETURNS bigint LANGUAGE sql AS 'SELECT nextval(''unique_rowid_seq'')'`); err != nil {
		return nil, err
	}
	return NewPostgresProvider(db, opts...)
}

func dropSQL(t testing.TB, admin *sql.DB, name string) string {
	if isPG(t, admin) {
		return "DROP DATABASE IF EXISTS " + name + " WITH (FORCE)"
	}
	return "DROP DATABASE IF EXISTS " + name + " CASCADE"
}
