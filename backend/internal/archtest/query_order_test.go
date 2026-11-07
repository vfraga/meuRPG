package archtest

import (
	_ "embed"
	"fmt"
	"os"
	"regexp"
	"slices"
	"strings"
	"testing"
)

// queryOrderExceptions lists the :many queries that still have no ORDER BY.
// It can only shrink.
//
//go:embed query_order_exceptions.txt
var queryOrderExceptions string

var (
	manyHeader = regexp.MustCompile(`(?m)^--\s*name:\s*(\w+)\s+:many\b`)
	anyHeader  = regexp.MustCompile(`(?m)^--\s*name:\s*\w+`)
	orderBy    = regexp.MustCompile(`(?i)\bORDER\s+BY\b`)
)

// TestManyQueriesHaveOrderBy fails for a :many query with no ORDER BY in its outermost statement.
// It reads the query files that sqlc.yaml lists.
// A query that query_order_exceptions.txt lists is allowed.
// A listed query that has an ORDER BY now fails.
func TestManyQueriesHaveOrderBy(t *testing.T) {
	matches := sqlcQueryFiles(t)
	found := map[string]string{}
	many := 0
	for _, m := range matches {
		src, err := os.ReadFile(backendRoot + "/" + m)
		if err != nil {
			t.Fatal(err)
		}
		module := moduleOfQueryFile(m)
		n, unordered := unorderedManyQueries(string(src))
		many += n
		for _, name := range unordered {
			key := module + "/" + name
			found[key] = fmt.Sprintf("%s is a :many query with no ORDER BY in its outermost statement: add an ORDER BY with a stable key "+
				"(the primary key, or created_at then the key). An UPDATE/DELETE ... RETURNING :many wraps in "+
				"WITH changed AS (...) SELECT ... FROM changed ORDER BY .... "+
				"Why: the tests run on PostgreSQL, whose insertion order hides what CockroachDB's order exposes (docs/data/README.md)", key)
		}
	}
	failProblems(t, compareExceptions("query_order_exceptions.txt", parseExceptions(queryOrderExceptions), found))
	t.Logf("%d :many queries in %d sqlc query files checked; %d known exceptions", many, len(matches), len(found))
}

// unorderedManyQueries returns how many :many queries the file holds and the
// names of those with no ORDER BY at parenthesis depth 0.
func unorderedManyQueries(sql string) (int, []string) {
	heads := anyHeader.FindAllStringIndex(sql, -1)
	count := 0
	var out []string
	for i, h := range heads {
		end := len(sql)
		if i+1 < len(heads) {
			end = heads[i+1][0]
		}
		block := sql[h[0]:end]
		m := manyHeader.FindStringSubmatch(block)
		if m == nil {
			continue
		}
		count++
		if !hasOuterOrderBy(block) {
			out = append(out, m[1])
		}
	}
	return count, out
}

// hasOuterOrderBy reports whether the SQL has an ORDER BY outside every
// parenthesis, comments and string literals left out.
func hasOuterOrderBy(sql string) bool {
	return orderBy.MatchString(depthZero(stripCommentsAndStrings(sql)))
}

// stripCommentsAndStrings blanks "-- line" comments, "/* block */" comments
// and 'string literals' (two quotes in a row are an escaped quote) in one pass, so an apostrophe
// in a comment or "--" in a string cannot confuse the other.
func stripCommentsAndStrings(sql string) string {
	var b strings.Builder
	for i := 0; i < len(sql); {
		switch {
		case strings.HasPrefix(sql[i:], "--"):
			for i < len(sql) && sql[i] != '\n' {
				i++
			}
			b.WriteByte(' ')
		case strings.HasPrefix(sql[i:], "/*"):
			j := strings.Index(sql[i+2:], "*/")
			if j < 0 {
				i = len(sql)
			} else {
				i += j + 4
			}
			b.WriteByte(' ')
		case sql[i] == '\'':
			i++
			for i < len(sql) {
				if sql[i] == '\'' {
					if i+1 < len(sql) && sql[i+1] == '\'' {
						i += 2
						continue
					}
					i++
					break
				}
				i++
			}
			b.WriteString("''")
		default:
			b.WriteByte(sql[i])
			i++
		}
	}
	return b.String()
}

// depthZero keeps the text outside every parenthesis and blanks the rest.
func depthZero(sql string) string {
	var b strings.Builder
	depth := 0
	for i := range len(sql) {
		switch c := sql[i]; {
		case c == '(':
			depth++
			b.WriteByte(' ')
		case c == ')':
			depth = max(depth-1, 0)
			b.WriteByte(' ')
		case depth == 0:
			b.WriteByte(c)
		}
	}
	return b.String()
}

func TestUnorderedManyQueries(t *testing.T) {
	cases := []struct {
		name, sql string
		many      int
		want      []string
	}{
		{"with order by", "-- name: A :many\nSELECT * FROM t ORDER BY id;\n", 1, nil},
		{"without order by", "-- name: A :many\nSELECT * FROM t;\n", 1, []string{"A"}},
		{"order by in a subselect", "-- name: A :many\nSELECT * FROM (SELECT * FROM t ORDER BY id LIMIT 1) s;\n", 1, []string{"A"}},
		{"order by in a window", "-- name: A :many\nSELECT row_number() OVER (ORDER BY id) FROM t;\n", 1, []string{"A"}},
		{"order by in an aggregate", "-- name: A :many\nSELECT string_agg(x, ',' ORDER BY x) FROM t GROUP BY id;\n", 1, []string{"A"}},
		{"cte-wrapped dml", "-- name: A :many\nWITH changed AS (UPDATE t SET x = 1 RETURNING *)\nSELECT * FROM changed ORDER BY id;\n", 1, nil},
		{"cte without final order by", "-- name: A :many\nWITH x AS (SELECT * FROM t ORDER BY id) SELECT * FROM x;\n", 1, []string{"A"}},
		{"union ordered after the last branch", "-- name: A :many\nSELECT id FROM a UNION ALL SELECT id FROM b ORDER BY id;\n", 1, nil},
		{"union with parenthesized branches", "-- name: A :many\n(SELECT id FROM a ORDER BY id) UNION ALL (SELECT id FROM b ORDER BY id);\n", 1, []string{"A"}},
		{"order by in a comment", "-- name: A :many\nSELECT * FROM t; -- ORDER BY id\n", 1, []string{"A"}},
		{"order by in a block comment", "-- name: A :many\nSELECT * FROM t /* ORDER BY id */;\n", 1, []string{"A"}},
		{"order by in a string", "-- name: A :many\nSELECT 'ORDER BY id' FROM t;\n", 1, []string{"A"}},
		{"apostrophe in a comment", "-- name: A :many\n-- don't forget\nSELECT * FROM t ORDER BY id;\n", 1, nil},
		{"dashes in a string", "-- name: A :many\nSELECT '--' FROM t ORDER BY id;\n", 1, nil},
		{"one and exec ignored", "-- name: A :one\nSELECT * FROM t;\n-- name: B :exec\nDELETE FROM t;\n", 0, nil},
		{"marker with trailing text", "-- name: A :many -- the list\nSELECT * FROM t;\n", 1, []string{"A"}},
		{"marker needs a word end", "-- name: A :manyx\nSELECT * FROM t;\n", 0, nil},
		{"only the unordered one", "-- name: A :many\nSELECT * FROM t ORDER BY id;\n-- name: B :many\nSELECT * FROM t;\n", 2, []string{"B"}},
		{"lower case", "-- name: A :many\nselect * from t order\n  by id;\n", 1, nil},
	}
	for _, c := range cases {
		many, got := unorderedManyQueries(c.sql)
		if many != c.many || !slices.Equal(got, c.want) {
			t.Errorf("%s: got %d, %v; want %d, %v", c.name, many, got, c.many, c.want)
		}
	}
}
