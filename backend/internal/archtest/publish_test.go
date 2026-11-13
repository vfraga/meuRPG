package archtest

import (
	_ "embed"
	"fmt"
	"go/ast"
	"go/parser"
	"go/token"
	"io/fs"
	"os"
	"path"
	"slices"
	"testing"
)

// publishExceptions lists the functions that may still call Publish outside the funnel functions.
// An entry names the function, not each call in it, so a second call in a listed function is not seen.
// It can only shrink.
//
//go:embed publish_exceptions.txt
var publishExceptions string

// publishFunnels maps each package that tells the live streams to the one function that may call Publish.
// The play module has Service.send, the maps module Service.publish.
// Only the files directly in the package are read: its subpackages are other packages.
var publishFunnels = map[string]string{
	"internal/play": "internal/play/hints.go:(*Service).send",
	"internal/maps": "internal/maps/visibility.go:(*Service).publish",
}

// TestPublishOnlyThroughTheFunnel fails for a use of Publish (a call or a method value), on any value, in a non-test file of
// internal/play or internal/maps other than its funnel function.
// A function that publish_exceptions.txt lists is allowed.
// A listed function that no longer calls Publish fails.
func TestPublishOnlyThroughTheFunnel(t *testing.T) {
	root := os.DirFS(backendRoot)
	found := map[string]string{}
	files := 0
	for dir, funnel := range publishFunnels {
		entries, err := fs.ReadDir(root, dir)
		if err != nil {
			t.Fatalf("reading %s: %v", dir, err)
		}
		for _, e := range entries {
			p := path.Join(dir, e.Name())
			if e.IsDir() || path.Ext(p) != ".go" || isTestFile(p) {
				continue
			}
			files++
			src, err := fs.ReadFile(root, p)
			if err != nil {
				t.Fatalf("reading %s: %v", p, err)
			}
			keys, err := publishKeys(p, src, funnel)
			if err != nil {
				t.Fatalf("parsing %s: %v", p, err)
			}
			for _, k := range keys {
				found[k] = fmt.Sprintf("%s uses Publish: tell the streams through %s "+
					"(Service.send in play, Service.publish in maps), or list the function in publish_exceptions.txt", k, funnel)
			}
		}
	}
	if files == 0 {
		t.Fatal("found no Go file: is backendRoot right?")
	}
	failProblems(t, compareExceptions("publish_exceptions.txt", parseExceptions(publishExceptions), found))
	t.Logf("%d non-test Go files checked for Publish calls; %d known exceptions", files, len(found))
}

// publishKeys returns the "file:function" key of every function in one file that calls a method or
// function named Publish. A key appears once, in source order. The funnel function has none.
func publishKeys(p string, src []byte, funnel string) ([]string, error) {
	f, err := parser.ParseFile(token.NewFileSet(), p, src, parser.SkipObjectResolution)
	if err != nil {
		return nil, err
	}
	var keys []string
	add := func(fn string) {
		if k := p + ":" + fn; k != funnel && !slices.Contains(keys, k) {
			keys = append(keys, k)
		}
	}
	for _, decl := range f.Decls {
		switch d := decl.(type) {
		case *ast.FuncDecl:
			if callsPublish(d) {
				add(funcName(d))
			}
		case *ast.GenDecl:
			for _, spec := range d.Specs {
				if vs, ok := spec.(*ast.ValueSpec); ok && callsPublish(vs) {
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

// callsPublish reports whether the node contains a selector named Publish: a call, or a method value such as
// pub := s.hub.Publish. A method declaration is not a selector.
func callsPublish(n ast.Node) bool {
	found := false
	ast.Inspect(n, func(n ast.Node) bool {
		if sel, ok := n.(*ast.SelectorExpr); ok && sel.Sel.Name == "Publish" {
			found = true
		}
		return !found
	})
	return found
}

func TestPublishKeys(t *testing.T) {
	const fun = "internal/x/hints.go:(*S).send"
	cases := []struct {
		name, path, src string
		want            []string
	}{
		{"hub call", "internal/x/a.go", "package x\ntype S struct{ hub H }\nfunc (s *S) F() { s.hub.Publish(\"c\", 1) }", []string{"internal/x/a.go:(*S).F"}},
		{"method on the service", "internal/x/a.go", "package x\nfunc F(s S) { s.Publish(\"c\", true, nil) }", []string{"internal/x/a.go:F"}},
		{"method value", "internal/x/a.go", "package x\nfunc F(s S) { pub := s.hub.Publish; pub() }", []string{"internal/x/a.go:F"}},
		{"inside a literal", "internal/x/a.go", "package x\nfunc F(s S) { go func() { s.live.Publish() }() }", []string{"internal/x/a.go:F"}},
		{"package-level literal", "internal/x/a.go", "package x\nvar tell = func(s S) { s.Publish() }", []string{"internal/x/a.go:tell"}},
		{"one key per function", "internal/x/a.go", "package x\nfunc F(s S) { s.Publish(); s.Publish() }", []string{"internal/x/a.go:F"}},
		{"funnel function", "internal/x/hints.go", "package x\ntype S struct{}\nfunc (s *S) send() { s.hub.Publish() }", nil},
		{"another function in the funnel file", "internal/x/hints.go", "package x\ntype S struct{}\nfunc (s *S) other() { s.hub.Publish() }", []string{"internal/x/hints.go:(*S).other"}},
		{"declaring a Publish method is not a call", "internal/x/a.go", "package x\nfunc (s S) Publish() {}\nfunc F(s S) { s.PublishToUsers() }", nil},
		{"no call", "internal/x/a.go", "package x\nfunc F(s S) { s.send() }", nil},
	}
	for _, c := range cases {
		got, err := publishKeys(c.path, []byte(c.src), fun)
		if err != nil {
			t.Fatalf("%s: %v", c.name, err)
		}
		if !slices.Equal(got, c.want) {
			t.Errorf("%s: got %q, want %q", c.name, got, c.want)
		}
	}
}
