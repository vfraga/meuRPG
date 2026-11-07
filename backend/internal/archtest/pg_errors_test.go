package archtest

import (
	_ "embed"
	"fmt"
	"go/ast"
	"go/parser"
	"go/token"
	"io/fs"
	"os"
	"regexp"
	"strconv"
	"strings"
	"testing"
)

// pgErrorsExceptions lists the files in identity and authz that still read a database error's code.
// It can only shrink.
//
//go:embed pg_errors_exceptions.txt
var pgErrorsExceptions string

// pgErrorModules are the modules that translate database errors only through the common helper.
// Their sqlc packages are generated and skipped.
var (
	pgErrorModules = []string{"internal/identity", "internal/authz"}
	pgErrorSkip    = []string{"internal/identity/identitydb", "internal/authz/authzdb"}
)

var sqlStateLiteral = regexp.MustCompile(`^\d{5}$`)

// TestIdentityAuthzTranslateDBErrorsViaHelper fails when identity or authz reads a database error's code itself.
// It reads the non-test files of internal/identity and internal/authz.
// The code is read by an import of pgconn or pgerrcode, a use of PgError or SQLState, or a SQLSTATE-looking literal.
// A file that pg_errors_exceptions.txt lists is allowed.
// A listed file that no longer does it fails.
func TestIdentityAuthzTranslateDBErrorsViaHelper(t *testing.T) {
	root := os.DirFS(backendRoot)
	found := map[string]string{}
	files := 0
	for _, dir := range pgErrorModules {
		err := walkGoFiles(root, dir, func(path string) error {
			if skipPgErrorFile(path) {
				return nil
			}
			files++
			src, err := fs.ReadFile(root, path)
			if err != nil {
				return err
			}
			what, err := pgErrorUses(path, src)
			if err != nil {
				return err
			}
			if len(what) > 0 {
				found[path] = fmt.Sprintf("%s reads a database error's code itself (%s): use db.IsUniqueViolation "+
					"(internal/platform/db) for a unique violation and rpcerr (internal/platform/rpcerr) for the RPC error; "+
					"errors.Is(err, pgx.ErrNoRows) is fine", path, strings.Join(what, ", "))
			}
			return nil
		})
		if err != nil {
			t.Fatalf("reading %s: %v", dir, err)
		}
	}
	if files == 0 {
		t.Fatal("found no Go file: is backendRoot right?")
	}
	failProblems(t, compareExceptions("pg_errors_exceptions.txt", parseExceptions(pgErrorsExceptions), found))
	t.Logf("%d non-test Go files of identity and authz checked for pgconn, PgError, SQLState and SQLSTATE literals; %d known exceptions", files, len(found))
}

// skipPgErrorFile reports whether a path is out of scope: a test file or generated sqlc code.
func skipPgErrorFile(path string) bool {
	if isTestFile(path) {
		return true
	}
	for _, skip := range pgErrorSkip {
		if strings.HasPrefix(path, skip+"/") {
			return true
		}
	}
	return false
}

// pgErrorUses returns what in one file reads a database error's code.
// That is the pgconn and pgerrcode imports, the names PgError and SQLState, and a 5-digit string literal.
func pgErrorUses(path string, src []byte) ([]string, error) {
	f, err := parser.ParseFile(token.NewFileSet(), path, src, parser.SkipObjectResolution)
	if err != nil {
		return nil, err
	}
	seen := map[string]bool{}
	var what []string
	add := func(s string) {
		if !seen[s] {
			seen[s] = true
			what = append(what, s)
		}
	}
	for _, imp := range f.Imports {
		p, err := strconv.Unquote(imp.Path.Value)
		switch {
		case err != nil:
		case p == "github.com/jackc/pgx/v5/pgconn":
			add("imports pgconn")
		case p == "github.com/jackc/pgerrcode":
			add("imports pgerrcode")
		}
	}
	ast.Inspect(f, func(n ast.Node) bool {
		switch n := n.(type) {
		case *ast.Ident:
			if n.Name == "PgError" || n.Name == "SQLState" {
				add("uses " + n.Name)
			}
		case *ast.BasicLit:
			if n.Kind != token.STRING {
				break
			}
			if v, err := strconv.Unquote(n.Value); err == nil && sqlStateLiteral.MatchString(v) {
				add("a 5-character SQLSTATE-looking literal " + n.Value)
			}
		}
		return true
	})
	return what, nil
}

func TestPgErrorUses(t *testing.T) {
	cases := []struct {
		name, src string
		want      string
	}{
		{"pgconn import", "package x\nimport \"github.com/jackc/pgx/v5/pgconn\"\nvar _ = pgconn.Timeout", "[imports pgconn]"},
		{"PgError", "package x\nvar e *struct{ PgError int }", "[uses PgError]"},
		{"SQLState compare", "package x\nfunc f(e interface{ SQLState() string }) bool { return e.SQLState() == \"23505\" }", "[uses SQLState a 5-character SQLSTATE-looking literal \"23505\"]"},
		{"raw string literal", "package x\nvar c = `23505`", "[a 5-character SQLSTATE-looking literal `23505`]"},
		{"pgerrcode import", "package x\nimport \"github.com/jackc/pgerrcode\"\nvar _ = pgerrcode.UniqueViolation", "[imports pgerrcode]"},
		{"pgconn alias import", "package x\nimport pg \"github.com/jackc/pgx/v5/pgconn\"\nvar _ pg.PgError", "[imports pgconn uses PgError]"},
		{"pgconn dot import", "package x\nimport . \"github.com/jackc/pgx/v5/pgconn\"", "[imports pgconn]"},
		{"ErrNoRows is fine", "package x\nimport (\"errors\"; \"github.com/jackc/pgx/v5\")\nvar _ = errors.Is(nil, pgx.ErrNoRows)", "[]"},
		{"other number string", "package x\nvar s = \"123456\"; var t = \"12\"", "[]"},
	}
	for _, c := range cases {
		got, err := pgErrorUses("internal/identity/a.go", []byte(c.src))
		if err != nil || fmt.Sprint(got) != c.want {
			t.Errorf("%s: got %v (err %v), want %s", c.name, got, err, c.want)
		}
	}
}

func TestSkipPgErrorFile(t *testing.T) {
	for path, want := range map[string]bool{
		"internal/identity/a_test.go":            true,
		"internal/identity/identitydb/db.go":     true,
		"internal/authz/authzdb/db.go":           true,
		"internal/identity/store.go":             false,
		"internal/identity/oidctest/provider.go": false,
		"internal/authz/authz.go":                false,
	} {
		if got := skipPgErrorFile(path); got != want {
			t.Errorf("skipPgErrorFile(%q) = %v, want %v", path, got, want)
		}
	}
}
