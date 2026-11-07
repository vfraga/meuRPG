package archtest

import (
	"fmt"
	"go/ast"
	"go/parser"
	"go/token"
	"io/fs"
	"os"
	"strconv"
	"strings"
	"testing"
	"testing/fstest"
)

// A canary a test hides in the data and then searches for in a whole answer, error or log text must not be made of
// digits alone: a random id, a timestamp or any other number in the text holds any four digits, and the search matches
// it now and then (run 42337730750: "4217" inside a combatant's id). A text canary has a letter outside a-f ("Zorvak4321"),
// so it never matches inside a UUID, a timestamp or a number; a number canary is searched as a number, in the number
// fields of the decoded message (leaktest's inspector, `w.secrets.number`), or in text bounded by a non-word character.
// A search for a bare-digit string that is a positive check (the 4096 of a limit's own message) says why on its line:
// `// canary-ok: <why>`.

// textSearchFuncs are the functions of strings and bytes that look for a piece of text inside another.
var textSearchFuncs = map[string]bool{
	"Contains": true, "ContainsAny": true, "Count": true, "HasPrefix": true, "HasSuffix": true,
	"Index": true, "IndexAny": true, "LastIndex": true,
}

// bareDigitSearches lists, as "file:line", the searches of a _test.go file under fsys that look for a string of four or more
// digits and nothing else, without a `canary-ok:` on the line.
func bareDigitSearches(fsys fs.FS) ([]string, error) {
	var found []string
	err := fs.WalkDir(fsys, ".", func(p string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() {
			if d.Name() == "testdata" || d.Name() == "gen" || d.Name() == "node_modules" {
				return fs.SkipDir
			}
			return nil
		}
		if !strings.HasSuffix(p, "_test.go") {
			return nil
		}
		src, err := fs.ReadFile(fsys, p)
		if err != nil {
			return err
		}
		fset := token.NewFileSet()
		file, err := parser.ParseFile(fset, p, src, 0)
		if err != nil {
			return fmt.Errorf("parse %s: %w", p, err)
		}
		lines := strings.Split(string(src), "\n")
		ast.Inspect(file, func(n ast.Node) bool {
			call, ok := n.(*ast.CallExpr)
			if !ok || !isTextSearch(call) {
				return true
			}
			for _, a := range call.Args {
				if !isBareDigits(a) {
					continue
				}
				line := fset.Position(call.Pos()).Line
				if !strings.Contains(lines[line-1], "canary-ok:") {
					found = append(found, fmt.Sprintf("%s:%d", p, line))
				}
			}
			return true
		})
		return nil
	})
	return found, err
}

// isTextSearch says the call is strings.Contains and its kin, or the same of bytes.
func isTextSearch(call *ast.CallExpr) bool {
	sel, ok := call.Fun.(*ast.SelectorExpr)
	if !ok || !textSearchFuncs[sel.Sel.Name] {
		return false
	}
	pkg, ok := sel.X.(*ast.Ident)
	return ok && (pkg.Name == "strings" || pkg.Name == "bytes")
}

// isBareDigits says the argument is a string literal of four or more digits, alone or as []byte("...").
func isBareDigits(e ast.Expr) bool {
	if conv, ok := e.(*ast.CallExpr); ok && len(conv.Args) == 1 {
		if _, isArray := conv.Fun.(*ast.ArrayType); isArray {
			e = conv.Args[0]
		}
	}
	lit, ok := e.(*ast.BasicLit)
	if !ok || lit.Kind != token.STRING {
		return false
	}
	s, err := strconv.Unquote(lit.Value)
	if err != nil || len(s) < 4 {
		return false
	}
	return strings.Trim(s, "0123456789") == ""
}

// TestNoTestSearchesTextForABareDigitCanary reads every _test.go file of the backend.
func TestNoTestSearchesTextForABareDigitCanary(t *testing.T) {
	found, err := bareDigitSearches(os.DirFS(backendRoot))
	if err != nil {
		t.Fatalf("reading the backend's tests: %v", err)
	}
	for _, f := range found {
		t.Errorf("%s searches a text for a string of digits alone: a random id or a timestamp holds it now and then. Use a text canary with a letter outside a-f (\"Zorvak4321\"), or search a number as a number (leaktest: w.secrets.number); a positive check says why with `// canary-ok: <why>` on the line", f)
	}
}

// TestTheBareDigitRuleSeesTheSearchesItMustAndSkipsTheRest proves the rule can fail and does not fail for what is safe.
func TestTheBareDigitRuleSeesTheSearchesItMustAndSkipsTheRest(t *testing.T) {
	file := func(body string) *fstest.MapFile {
		return &fstest.MapFile{Data: []byte("package p\n\nimport (\n\t\"bytes\"\n\t\"strings\"\n)\n\nfunc f(s string, b []byte) {\n" + body + "\n}\n")}
	}
	fsys := fstest.MapFS{
		"a/found_string_test.go":     file("\t_ = strings.Contains(s, \"4217\")"),
		"a/found_bytes_test.go":      file("\t_ = bytes.Contains(b, []byte(\"4217\"))"),
		"a/found_index_test.go":      file("\t_ = strings.Index(s, \"123456\")"),
		"a/letter_test.go":           file("\t_ = strings.Contains(s, \"Zorvak4321\")"),
		"a/short_test.go":            file("\t_ = strings.Contains(s, \"503\")"),
		"a/said_test.go":             file("\t_ = strings.Contains(s, \"4096\") // canary-ok: the limit's own message"),
		"a/not_a_search_test.go":     file("\t_ = strings.Repeat(\"4217\", 2)"),
		"a/production.go":            file("\t_ = strings.Contains(s, \"4217\")"),
		"a/testdata/fixture_test.go": file("\t_ = strings.Contains(s, \"4217\")"),
	}
	got, err := bareDigitSearches(fsys)
	if err != nil {
		t.Fatalf("bareDigitSearches() error = %v", err)
	}
	want := []string{"a/found_bytes_test.go:9", "a/found_index_test.go:9", "a/found_string_test.go:9"}
	if strings.Join(got, ",") != strings.Join(want, ",") {
		t.Errorf("bareDigitSearches() = %v, want %v", got, want)
	}
}
