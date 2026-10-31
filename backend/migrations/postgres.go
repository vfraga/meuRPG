package migrations

import (
	"database/sql"
	"io/fs"
	"regexp"
	"testing/fstest"

	"github.com/pressly/goose/v3"
)

// EXPERIMENT (exp/postgres-tests): the migrations as PostgreSQL can run them.
// Nothing in production uses this.
//
// PostgreSQL lacks three things the files use: row-level TTL (a WITH clause on
// CREATE TABLE, or ALTER TABLE ... SET), unique_rowid().
// The function is made by PostgresPrelude before the first
// migration. The TTL clauses are cut from the text goose reads.
var (
	ttlOnCreate = regexp.MustCompile(`(?s)\)\s*WITH\s*\(\s*ttl_[^;]*?\)\s*;`)
	ttlOnAlter  = regexp.MustCompile(`(?s)ALTER TABLE \w+ SET \(\s*ttl_.*?\);`)
)

// PostgresPrelude is what PostgreSQL needs before the migrations run.
const PostgresPrelude = `
CREATE SEQUENCE unique_rowid_seq;
CREATE FUNCTION unique_rowid() RETURNS bigint LANGUAGE sql AS 'SELECT nextval(''unique_rowid_seq'')';
`

// NewPostgresProvider is NewProvider over the files with the TTL clauses removed.
func NewPostgresProvider(db *sql.DB, opts ...goose.ProviderOption) (*goose.Provider, error) {
	names, err := fs.Glob(files, "*.sql")
	if err != nil {
		return nil, err
	}
	out := fstest.MapFS{}
	for _, name := range names {
		body, err := files.ReadFile(name)
		if err != nil {
			return nil, err
		}
		body = ttlOnCreate.ReplaceAll(body, []byte(");"))
		body = ttlOnAlter.ReplaceAll(body, nil)
		out[name] = &fstest.MapFile{Data: body}
	}
	return goose.NewProvider(goose.DialectPostgres, db, out, opts...)
}
