package migrations

import (
	"io/fs"
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"testing"
)

// lastCockroachOnlyMigration is the newest migration written before the tests
// ran on PostgreSQL. The ones up to it may use what only CockroachDB accepts
// (postgres.go adapts them); a newer one, and every query the application
// sends, must run on both.
const lastCockroachOnlyMigration = 288

// cockroachOnly are the spellings that only CockroachDB accepts.
var cockroachOnly = []struct {
	re   *regexp.Regexp
	what string
}{
	{regexp.MustCompile(`(?i)\bSTRING\b`), "the STRING type (write TEXT)"},
	{regexp.MustCompile(`(?i)\bunique_rowid\s*\(`), "unique_rowid() (write a sequence or an identity column)"},
	{regexp.MustCompile(`(?i)\bttl_[a-z_]+`), "row-level TTL (sweep the rows in the application)"},
	{regexp.MustCompile(`(?i)\bcrdb_internal\b`), "crdb_internal"},
	{regexp.MustCompile(`(?i)\bSHOW\s+CREATE\b`), "SHOW CREATE"},
	{regexp.MustCompile(`(?i)\bAS\s+OF\s+SYSTEM\s+TIME\b`), "AS OF SYSTEM TIME"},
	{regexp.MustCompile(`(?i)\bUSING\s+HASH\b`), "USING HASH"},
	{regexp.MustCompile(`(?i)\bSTORING\b`), "STORING (write INCLUDE)"},
	{regexp.MustCompile(`(?i)\bSET\s+TRANSACTION\s+PRIORITY\b`), "SET TRANSACTION PRIORITY"},
	{regexp.MustCompile(`(?i)\bBYTES\b`), "the BYTES type (write BYTEA)"},
	{regexp.MustCompile(`(?i)\bUPSERT\b`), "UPSERT (write INSERT ... ON CONFLICT)"},
	{regexp.MustCompile(`(?i)\bFAMILY\b`), "column families"},
	{regexp.MustCompile(`(?i)\bINVERTED\s+INDEX\b`), "INVERTED INDEX (write USING GIN)"},
	{regexp.MustCompile(`(?i)\bCONFIGURE\s+ZONE\b`), "CONFIGURE ZONE"},
	{regexp.MustCompile(`(?i)\bNOT\s+VISIBLE\b`), "NOT VISIBLE columns"},
}

// stripComments cuts the -- comments of a SQL text, but not a -- inside a string literal.
func stripComments(sql string) string {
	var out strings.Builder
	inLiteral := false
	for i := 0; i < len(sql); i++ {
		c := sql[i]
		switch {
		case c == '\'':
			inLiteral = !inLiteral
		case !inLiteral && c == '-' && i+1 < len(sql) && sql[i+1] == '-':
			for i < len(sql) && sql[i] != '\n' {
				i++
			}
			if i < len(sql) {
				out.WriteByte('\n')
			}
			continue
		}
		out.WriteByte(c)
	}
	return out.String()
}

// dialectProblems names what a SQL text uses that only CockroachDB accepts.
func dialectProblems(sql string) []string {
	sql = stripComments(sql)
	var out []string
	for _, c := range cockroachOnly {
		if c.re.MatchString(sql) {
			out = append(out, c.what)
		}
	}
	return out
}

func TestDialectProblemsFindsWhatOnlyCockroachDBAccepts(t *testing.T) {
	t.Parallel()
	for sql, want := range map[string]int{
		"CREATE TABLE t (a STRING)":                              1,
		"CREATE TABLE t (a TEXT) WITH (ttl_expire_after = '1h')": 1,
		"SELECT unique_rowid()":                                  1,
		"SELECT 1 -- STRING is cut in a comment":                 0,
		"CREATE TABLE t (a TEXT, b BIGINT)":                      0,
		"SELECT x FROM t AS OF SYSTEM TIME '-1s'":                1,
		"SELECT '--', 1 FROM t AS OF SYSTEM TIME '-1s'":          1,
		"CREATE TABLE t (a BYTES)":                               1,
		"UPSERT INTO t VALUES (1)":                               1,
		"CREATE INVERTED INDEX i ON t (a)":                       1,
		"ALTER TABLE t ALTER COLUMN a SET NOT VISIBLE":           1,
		"SELECT 1 -- FAMILY\nSELECT 2":                           0,
	} {
		if got := dialectProblems(sql); len(got) != want {
			t.Errorf("dialectProblems(%q) = %v, want %d", sql, got, want)
		}
	}
}

func TestNewMigrationsAndQueryFilesAvoidCockroachDBOnlySQL(t *testing.T) {
	t.Parallel()
	check := func(path string, body []byte) {
		for _, p := range dialectProblems(string(body)) {
			t.Errorf("%s uses %s, which only CockroachDB accepts; write SQL both databases run (docs/data/README.md)", path, p)
		}
	}
	entries, err := fs.ReadDir(files, ".")
	if err != nil {
		t.Fatal(err)
	}
	checked, highest := 0, 0
	for _, e := range entries {
		n, err := strconv.Atoi(strings.SplitN(e.Name(), "_", 2)[0])
		if err != nil {
			continue
		}
		highest = max(highest, n)
		if n <= lastCockroachOnlyMigration {
			continue
		}
		body, err := fs.ReadFile(files, e.Name())
		if err != nil {
			t.Fatal(err)
		}
		check(e.Name(), body)
		checked++
	}
	if highest < lastCockroachOnlyMigration {
		t.Fatalf("the highest migration number read is %d, want at least %d: the scan does not see the migration files", highest, lastCockroachOnlyMigration)
	}
	queries, err := filepath.Glob(filepath.Join("..", "internal", "*", "queries.sql"))
	if err != nil || len(queries) == 0 {
		t.Fatalf("find the queries.sql files: %v (found %d)", err, len(queries))
	}
	for _, q := range queries {
		body, err := fs.ReadFile(os.DirFS(".."), filepath.ToSlash(strings.TrimPrefix(q, ".."+string(filepath.Separator))))
		if err != nil {
			t.Fatal(err)
		}
		check(q, body)
	}
	t.Logf("checked %d new migrations and %d query files", checked, len(queries))
}
