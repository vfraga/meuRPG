package archtest

import (
	_ "embed"
	"fmt"
	"go/ast"
	"go/parser"
	"go/token"
	"io/fs"
	"os"
	"strings"
	"testing"
)

// goStatementExceptions lists the functions that may still start a bare goroutine.
// An entry names the function, not each go statement in it, so a second one in a listed function is not seen.
// It can only shrink.
//
//go:embed go_statement_exceptions.txt
var goStatementExceptions string

// safegoDir is the one package that may contain a go statement.
const safegoDir = "internal/platform/safego"

// TestNoBareGoStatements fails for a go statement outside internal/platform/safego.
// It reads the non-test Go files under internal, cmd and migrations.
// A function that go_statement_exceptions.txt lists is allowed.
// A listed function that no longer has a go statement fails.
func TestNoBareGoStatements(t *testing.T) {
	root := os.DirFS(backendRoot)
	found := map[string]string{}
	files := 0
	for _, top := range []string{"internal", "cmd", "migrations"} {
		err := walkGoFiles(root, top, func(path string) error {
			if isTestFile(path) {
				return nil
			}
			files++
			src, err := fs.ReadFile(root, path)
			if err != nil {
				return err
			}
			keys, err := goStatementKeys(path, src)
			if err != nil {
				return err
			}
			for _, k := range keys {
				found[k] = fmt.Sprintf("%s starts a bare goroutine: use safego.Go(logger, \"what\", fn, onPanic...) "+
					"(internal/platform/safego), because a panic in a bare goroutine ends the process "+
					"(or, when the goroutine recovers inline, list the function in go_statement_exceptions.txt)", k)
			}
			return nil
		})
		if err != nil {
			t.Fatalf("reading %s: %v", top, err)
		}
	}
	if files == 0 {
		t.Fatal("found no Go file: is backendRoot right?")
	}
	failProblems(t, compareExceptions("go_statement_exceptions.txt", parseExceptions(goStatementExceptions), found))
	t.Logf("%d non-test Go files checked for go statements; %d known exceptions", files, len(found))
}

// skipDirName reports whether the go tool ignores a directory of this name.
func skipDirName(name string) bool {
	return name == "testdata" || strings.HasPrefix(name, ".") || strings.HasPrefix(name, "_")
}

// isTestFile reports whether a path is a _test.go file.
func isTestFile(path string) bool { return strings.HasSuffix(path, "_test.go") }

// walkGoFiles calls fn for every .go file under dir.
// It skips the directories the go tool ignores.
func walkGoFiles(root fs.FS, dir string, fn func(path string) error) error {
	return fs.WalkDir(root, dir, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() {
			if skipDirName(d.Name()) {
				return fs.SkipDir
			}
			return nil
		}
		if !strings.HasSuffix(path, ".go") {
			return nil
		}
		return fn(path)
	})
}

// goStatementKeys returns the "file:function" key of every go statement in one file.
// A key appears once, in source order.
// Files of internal/platform/safego have none.
func goStatementKeys(path string, src []byte) ([]string, error) {
	if strings.HasPrefix(path, safegoDir+"/") {
		return nil, nil
	}
	f, err := parser.ParseFile(token.NewFileSet(), path, src, parser.SkipObjectResolution)
	if err != nil {
		return nil, err
	}
	var keys []string
	add := func(fn string) {
		k := path + ":" + fn
		for _, have := range keys {
			if have == k {
				return
			}
		}
		keys = append(keys, k)
	}
	for _, decl := range f.Decls {
		switch d := decl.(type) {
		case *ast.FuncDecl:
			if hasGo(d) {
				add(funcName(d))
			}
		case *ast.GenDecl:
			for _, spec := range d.Specs {
				if vs, ok := spec.(*ast.ValueSpec); ok && hasGo(vs) {
					name := "init"
					if len(vs.Names) > 0 && vs.Names[0].Name != "_" {
						name = vs.Names[0].Name
					}
					add(name)
				}
			}
		}
	}
	return keys, nil
}

// hasGo reports whether the node contains a go statement.
func hasGo(n ast.Node) bool {
	found := false
	ast.Inspect(n, func(n ast.Node) bool {
		if _, ok := n.(*ast.GoStmt); ok {
			found = true
		}
		return !found
	})
	return found
}

// funcName is a function's name with its receiver: "F", "(*T).M", "(T).M".
func funcName(d *ast.FuncDecl) string {
	if d.Recv == nil || len(d.Recv.List) == 0 {
		return d.Name.Name
	}
	typ := d.Recv.List[0].Type
	star := ""
	if s, ok := typ.(*ast.StarExpr); ok {
		star, typ = "*", s.X
	}
	switch e := typ.(type) { // strip type parameters
	case *ast.IndexExpr:
		typ = e.X
	case *ast.IndexListExpr:
		typ = e.X
	}
	name := "?"
	if id, ok := typ.(*ast.Ident); ok {
		name = id.Name
	}
	return "(" + star + name + ")." + d.Name.Name
}

func TestSkipRules(t *testing.T) {
	for name, want := range map[string]bool{"testdata": true, ".git": true, "_old": true, "gen": false, "internal": false, "safego": false} {
		if got := skipDirName(name); got != want {
			t.Errorf("skipDirName(%q) = %v, want %v", name, got, want)
		}
	}
	if !isTestFile("a/b_test.go") || isTestFile("a/b.go") || isTestFile("a/test.go") {
		t.Error("isTestFile is wrong")
	}
}

func TestGoStatementKeys(t *testing.T) {
	cases := []struct {
		name, path, src string
		want            []string
	}{
		{"function", "internal/x/a.go", "package x\nfunc F() { go func() {}() }", []string{"internal/x/a.go:F"}},
		{"pointer method", "internal/x/a.go", "package x\ntype S struct{}\nfunc (s *S) Run() { go s.Run() }", []string{"internal/x/a.go:(*S).Run"}},
		{"value method, nested literal", "internal/x/a.go", "package x\ntype S struct{}\nfunc (S) Run() { f := func() { go g() }; f() }\nfunc g() {}", []string{"internal/x/a.go:(S).Run"}},
		{"generic receiver", "internal/x/a.go", "package x\ntype S[T any] struct{}\nfunc (s *S[T]) Run() { go s.Run() }", []string{"internal/x/a.go:(*S).Run"}},
		{"package-level literal", "cmd/x/a.go", "package x\nvar start = func() { go start() }", []string{"cmd/x/a.go:start"}},
		{"one key per function", "internal/x/a.go", "package x\nfunc F() { go F(); go F() }", []string{"internal/x/a.go:F"}},
		{"method value call", "internal/x/a.go", "package x\ntype S struct{}\nfunc (s S) f() {}\nfunc F(x S) { go x.f() }", []string{"internal/x/a.go:F"}},
		{"grouped var", "internal/x/a.go", "package x\nvar (\n\ta = 1\n\tstart = func() { go start() }\n)", []string{"internal/x/a.go:start"}},
		{"inside safego", "internal/platform/safego/safego.go", "package safego\nfunc Go() { go func() {}() }", nil},
		{"no go statement", "internal/x/a.go", "package x\nfunc F() { F() }", nil},
	}
	for _, c := range cases {
		got, err := goStatementKeys(c.path, []byte(c.src))
		if err != nil {
			t.Fatalf("%s: %v", c.name, err)
		}
		if fmt.Sprint(got) != fmt.Sprint(c.want) {
			t.Errorf("%s: got %v, want %v", c.name, got, c.want)
		}
	}
}
