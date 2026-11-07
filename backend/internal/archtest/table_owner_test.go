package archtest

import (
	_ "embed"
	"fmt"
	"os"
	"regexp"
	"slices"
	"sort"
	"strings"
	"testing"
)

// tableOwnerExceptions lists the queries that still touch another module's table.
// It can only shrink.
//
//go:embed table_owner_exceptions.txt
var tableOwnerExceptions string

// dataReadme is docs/data/README.md, seen from this package's directory.
const dataReadme = "../../../docs/data/README.md"

// ownerless are the tables that belong to no module (docs/data/README.md).
var ownerless = []string{"goose_db_version", "migration_lock"}

var (
	ownerRow     = regexp.MustCompile("^\\|\\s*`(\\w+)`(?:\\s*\\([^)]*\\))?\\s*\\|[^|]*\\|(.*)\\|\\s*$")
	backtickName = regexp.MustCompile("`(\\w+)`")
	queryHeader  = regexp.MustCompile(`(?m)^--\s*name:\s*(\w+)`)
	cteWith      = regexp.MustCompile(`(?i)\bWITH\s+(?:RECURSIVE\s+)?(\w+)\s*(?:\([^)]*\))?\s+AS\b`)
	cteNext      = regexp.MustCompile(`(?i),\s*(\w+)\s*(?:\([^)]*\))?\s+AS\s+(?:NOT\s+)?(?:MATERIALIZED\s*)?\(`)
	sqlToken     = regexp.MustCompile(`[A-Za-z_][A-Za-z0-9_.$]*|\$\d+|\S`)
)

// fromInsideFunction are the functions whose arguments contain a FROM that is not a table:
// EXTRACT(x FROM col), TRIM(BOTH ' ' FROM col), SUBSTRING(s FROM n), OVERLAY(s PLACING r FROM n).
var fromInsideFunction = []string{"extract", "trim", "substring", "overlay", "position"}

// endsTableList are the words that end a FROM/USING list of tables.
var endsTableList = []string{
	"where", "group", "order", "limit", "offset", "returning", "set", "union",
	"intersect", "except", "having", "window", "for", "fetch", "select", "values",
}

// TestQueriesTouchOnlyOwnTables fails when a query names a table that another module owns.
// It reads the query files that sqlc.yaml lists.
// The owner is the module in docs/data/README.md, "Modules and tables".
// A query that table_owner_exceptions.txt lists is allowed.
// A listed query that no longer does it fails.
// A table that no module lists fails too.
func TestQueriesTouchOnlyOwnTables(t *testing.T) {
	owners := readTableOwners(t)
	files := sqlcQueryFiles(t)
	found := map[string]string{}
	var unlisted []string
	queries := 0
	for _, f := range files {
		src, err := os.ReadFile(backendRoot + "/" + f)
		if err != nil {
			t.Fatal(err)
		}
		qs := scanQueries(moduleOfQueryFile(f), string(src), owners)
		queries += qs.count
		for k, msg := range qs.foreign {
			found[k] = msg
		}
		unlisted = append(unlisted, qs.unlisted...)
	}
	slices.Sort(unlisted)
	for _, u := range slices.Compact(unlisted) {
		t.Error(u)
	}
	failProblems(t, compareExceptions("table_owner_exceptions.txt", parseExceptions(tableOwnerExceptions), found))
	t.Logf("%d queries in %d sqlc query files checked against %d tables of %d modules; %d known exceptions",
		queries, len(files), len(owners), countModules(owners), len(found))
}

func countModules(owners map[string]string) int {
	seen := map[string]bool{}
	for _, m := range owners {
		seen[m] = true
	}
	return len(seen)
}

// readTableOwners parses the table of docs/data/README.md, "Modules and tables".
// The result maps a table name to its owning module.
func readTableOwners(t *testing.T) map[string]string {
	t.Helper()
	md, err := os.ReadFile(dataReadme)
	if err != nil {
		t.Fatalf("reading %s: %v", dataReadme, err)
	}
	owners, err := parseTableOwners(string(md))
	if err != nil {
		t.Fatalf("%s: %v", dataReadme, err)
	}
	if len(owners) == 0 {
		t.Fatalf("%s has no table rows under \"Modules and tables\"", dataReadme)
	}
	return owners
}

// parseTableOwners fails for a table listed under two modules.
func parseTableOwners(md string) (map[string]string, error) {
	owners := map[string]string{}
	in := false
	for line := range strings.Lines(md) {
		line = strings.TrimRight(line, "\n")
		if strings.HasPrefix(line, "## ") {
			in = strings.TrimSpace(strings.TrimPrefix(line, "## ")) == "Modules and tables"
			continue
		}
		if !in {
			continue
		}
		if m := ownerRow.FindStringSubmatch(line); m != nil {
			for _, tb := range backtickName.FindAllStringSubmatch(m[2], -1) {
				if prev, dup := owners[tb[1]]; dup && prev != m[1] {
					return nil, fmt.Errorf("the table %q is listed under two modules (%s and %s): a table has one owner", tb[1], prev, m[1])
				}
				owners[tb[1]] = m[1]
			}
		}
	}
	return owners, nil
}

// queryScan is what one query file holds.
type queryScan struct {
	count    int
	foreign  map[string]string // "module/Query" -> failure message
	unlisted []string          // failure messages: a table no module lists
}

// scanQueries checks every "-- name:" block of one module's query file.
func scanQueries(module, sql string, owners map[string]string) queryScan {
	qs := queryScan{foreign: map[string]string{}}
	heads := queryHeader.FindAllStringSubmatchIndex(sql, -1)
	for i, h := range heads {
		end := len(sql)
		if i+1 < len(heads) {
			end = heads[i+1][0]
		}
		key := module + "/" + sql[h[2]:h[3]]
		qs.count++
		foreign := map[string]string{} // table -> owner
		for _, tb := range tablesOf(sql[h[1]:end]) {
			owner, ok := owners[tb]
			switch {
			case slices.Contains(ownerless, tb):
			case !ok:
				qs.unlisted = append(qs.unlisted, fmt.Sprintf("%s reads the table %q, which no module lists in docs/data/README.md "+
					"(\"Modules and tables\"): add it to its module's row there", key, tb))
			case owner != module:
				foreign[tb] = owner
			}
		}
		if len(foreign) > 0 {
			var parts []string
			for tb, owner := range foreign {
				parts = append(parts, tb+" ("+owner+")")
			}
			sort.Strings(parts)
			qs.foreign[key] = fmt.Sprintf("%s touches tables of another module: %s. "+
				"Read another module's data through its link interface (docs/architecture/overview.md), "+
				"never by joining its tables "+
				"(or, for a sweep that must, list the query in table_owner_exceptions.txt)", key, strings.Join(parts, ", "))
		}
	}
	return qs
}

// frame is one open parenthesis while tablesOf walks a query.
type frame struct {
	owner string // the word before the parenthesis
	list  bool   // a FROM/USING list is open: a comma starts another table
}

// tablesOf returns the table names one query reads or writes.
// CTE aliases, function calls and keywords are left out.
func tablesOf(body string) []string {
	body = strings.ReplaceAll(stripCommentsAndStrings(body), `"`, "")
	ctes := map[string]bool{}
	for _, m := range cteWith.FindAllStringSubmatch(body, -1) {
		ctes[strings.ToLower(m[1])] = true
	}
	for _, m := range cteNext.FindAllStringSubmatch(body, -1) {
		ctes[strings.ToLower(m[1])] = true
	}
	toks := sqlToken.FindAllString(body, -1)
	for i := range toks {
		toks[i] = strings.ToLower(toks[i])
	}
	var out []string
	add := func(name string) {
		if i := strings.LastIndex(name, "."); i >= 0 {
			name = name[i+1:]
		}
		if name == "" || !isIdentStart(name[0]) || ctes[name] ||
			slices.Contains([]string{"set", "of", "skip", "nowait", "select", "values"}, name) || slices.Contains(out, name) {
			return
		}
		out = append(out, name)
	}
	// ref reads the table reference that starts at toks[i], after any ONLY/LATERAL,
	// and returns the index of the token after the name.
	// A name followed by "(" is a function call when wantCall is false.
	ref := func(i int, intoOrUpdate bool) int {
		for i < len(toks) && (toks[i] == "only" || toks[i] == "lateral") {
			i++
		}
		if i >= len(toks) {
			return i
		}
		name := toks[i]
		if !isIdentStart(name[0]) {
			return i // a subselect or a parameter
		}
		if i+1 < len(toks) && toks[i+1] == "(" && !intoOrUpdate {
			return i + 1 // a function call, not a table
		}
		add(name)
		return i + 1
	}
	stack := []frame{{}}
	for i := 0; i < len(toks); i++ {
		tok := toks[i]
		top := &stack[len(stack)-1]
		switch {
		case tok == "(":
			owner := ""
			if i > 0 {
				owner = toks[i-1]
			}
			stack = append(stack, frame{owner: owner})
		case tok == ")":
			if len(stack) > 1 {
				stack = stack[:len(stack)-1]
			}
		case tok == "," && top.list:
			i = ref(i+1, false) - 1
		case slices.Contains(endsTableList, tok):
			top.list = false
		case tok == "from" || tok == "join" || tok == "using" || tok == "into" || tok == "update":
			if slices.Contains(fromInsideFunction, top.owner) || i > 0 && toks[i-1] == "distinct" {
				continue
			}
			if tok == "update" && i > 0 && toks[i-1] == "do" { // ON CONFLICT DO UPDATE SET
				continue
			}
			i = ref(i+1, tok == "into" || tok == "update") - 1
			top.list = tok == "from" || tok == "using"
		}
	}
	slices.Sort(out)
	return out
}

func TestTablesOf(t *testing.T) {
	cases := []struct {
		name, sql string
		want      []string
	}{
		{"join", "SELECT * FROM characters c JOIN map_points p ON p.id = c.id", []string{"characters", "map_points"}},
		{"insert and update", "INSERT INTO a (x) VALUES ($1) ON CONFLICT (x) DO UPDATE SET x = 1", []string{"a"}},
		{"insert without space", "INSERT INTO a(x) VALUES ($1)", []string{"a"}},
		{"cte alias ignored", "WITH recent AS (SELECT id FROM maps), other AS (SELECT 1) SELECT * FROM recent JOIN other ON true", []string{"maps"}},
		{"function ignored", "SELECT * FROM unnest($1::uuid[]) AS u(id) JOIN jsonb_object_keys(x) k ON true", nil},
		{"function with a space", "SELECT * FROM unnest ($1::uuid[]) AS u(id)", nil},
		{"delete using", "DELETE FROM a USING b WHERE a.id = b.id", []string{"a", "b"}},
		{"for update of", "SELECT 1 FROM a FOR UPDATE OF a SKIP LOCKED", []string{"a"}},
		{"is distinct from", "SELECT 1 FROM a WHERE x IS DISTINCT FROM y", []string{"a"}},
		{"is not distinct from", "SELECT 1 FROM a WHERE x IS NOT DISTINCT FROM y", []string{"a"}},
		{"comment and string", "SELECT 'from nowhere' FROM a -- join b\n", []string{"a"}},
		{"dashes inside a string", "SELECT '--' FROM a JOIN b ON true", []string{"a", "b"}},
		{"block comment", "SELECT 1 /* FROM hidden */ FROM a", []string{"a"}},
		{"subselect", "SELECT * FROM (SELECT * FROM a) s", []string{"a"}},
		{"comma list", "SELECT 1 FROM a, b WHERE true", []string{"a", "b"}},
		{"comma list with aliases and newlines", "SELECT 1\nFROM a x,\n     b AS y,\n     c\nWHERE true", []string{"a", "b", "c"}},
		{"comma list after a function", "SELECT 1 FROM unnest($1) AS u(id), b", []string{"b"}},
		{"comma list with a subselect", "SELECT 1 FROM (SELECT 1 FROM a) s, b", []string{"a", "b"}},
		{"list ends at where", "SELECT 1 FROM a WHERE x IN (1, 2), b", []string{"a"}},
		{"select list commas are not tables", "SELECT x, y, z FROM a ORDER BY x, y", []string{"a"}},
		{"update only", "UPDATE ONLY a SET x = 1", []string{"a"}},
		{"from only", "DELETE FROM ONLY a WHERE true", []string{"a"}},
		{"quoted name", `SELECT 1 FROM "b" JOIN public."c" ON true`, []string{"b", "c"}},
		{"extract from", "SELECT EXTRACT(EPOCH FROM created_at) FROM a", []string{"a"}},
		{"trim from", "SELECT TRIM(BOTH ' ' FROM name) FROM a", []string{"a"}},
		{"substring from", "SELECT SUBSTRING(name FROM 2) FROM a", []string{"a"}},
		{"overlay from", "SELECT OVERLAY(name PLACING 'x' FROM 2) FROM a", []string{"a"}},
		{"position in", "SELECT POSITION('a' IN name) FROM a", []string{"a"}},
		{"from after a function call", "SELECT EXTRACT(EPOCH FROM t) FROM b", []string{"b"}},
	}
	for _, c := range cases {
		if got := tablesOf(c.sql); !slices.Equal(got, c.want) {
			t.Errorf("%s: got %v, want %v", c.name, got, c.want)
		}
	}
}

func TestScanQueries(t *testing.T) {
	owners, err := parseTableOwners("## Modules and tables\n\n" +
		"| Module | Page | Tables |\n| --- | --- | --- |\n" +
		"| `maps` | [maps.md](maps.md) | `maps`, `map_points` |\n" +
		"| `play` (combat) | [combat.md](combat.md) | `encounters` |\n" +
		"| `play` | [play.md](play.md) | `game_sessions` |\n\n## Next\n\n| `other` | x | `ignored` |\n")
	if err != nil || owners["map_points"] != "maps" || owners["encounters"] != "play" || owners["ignored"] != "" {
		t.Fatalf("parseTableOwners = %v, %v", owners, err)
	}
	sql := "-- name: Own :one\nSELECT * FROM encounters;\n" +
		"-- name: Joins :many\n-- a comment\nSELECT * FROM encounters e JOIN map_points p ON p.id = e.id;\n" +
		"-- name: Alias :many\nWITH map_points AS (SELECT 1) SELECT * FROM map_points;\n" +
		"-- name: Unknown :one\nSELECT * FROM mystery_table;\n" +
		"-- name: Goose :one\nSELECT * FROM goose_db_version;\n"
	qs := scanQueries("play", sql, owners)
	if qs.count != 5 {
		t.Errorf("count = %d, want 5", qs.count)
	}
	if len(qs.foreign) != 1 || !strings.Contains(qs.foreign["play/Joins"], "map_points (maps)") {
		t.Errorf("foreign = %v, want only play/Joins naming map_points (maps)", qs.foreign)
	}
	if len(qs.unlisted) != 1 || !strings.Contains(qs.unlisted[0], "mystery_table") {
		t.Errorf("unlisted = %v, want only mystery_table", qs.unlisted)
	}
}

func TestParseTableOwnersRefusesTwoOwners(t *testing.T) {
	md := "## Modules and tables\n\n| Module | Page | Tables |\n| --- | --- | --- |\n" +
		"| `maps` | [maps.md](maps.md) | `shared`, `a` |\n| `play` | [play.md](play.md) | `shared` |\n"
	if _, err := parseTableOwners(md); err == nil || !strings.Contains(err.Error(), "shared") {
		t.Errorf("err = %v, want a refusal naming shared", err)
	}
}

// isIdentStart reports whether c can start a lower-case SQL identifier.
func isIdentStart(c byte) bool {
	return c == '_' || c >= 'a' && c <= 'z'
}
