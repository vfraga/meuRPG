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
	"strings"
	"testing"
)

// hintTxExceptions lists the functions of internal/play that tell the streams inside the closure of a transaction.
// An entry names the function, not each call in it. It can only shrink.
//
//go:embed txhints_exceptions.txt
var hintTxExceptions string

// bumpFunnel is the one function that may write game_sessions.hint_seq: Service.tx takes the bump in the transaction of the write.
const bumpFunnel = "internal/play/hints.go:(*Service).txOn"

// TestHintsAreNotSentInsideATransaction fails for a call of send or tellMembers inside a function literal that is an argument
// of Service.tx or db.InTx in a non-test file of internal/play: it would tell the streams before the commit, and again on a
// retry. A closure collects with h.tell and h.touch; Service.tx sends after the commit.
// BumpHintSeq may be called only by Service.txOn (the body of Service.tx), and no string literal of a non-test file may name hint_seq (a raw
// UPDATE would move the number outside the funnel; the queries live in queries.sql and the generated playdb package).
//
// Limits, by name and by file, as the other rules here: a closure given by a literal or by an identifier assigned a literal in
// the same function is read; a closure that is a method value, a parameter or a package-level function is not. A helper that the
// closure calls and that itself calls send is not seen (only the closure's own calls are). Only the files directly in
// internal/play are read.
func TestHintsAreNotSentInsideATransaction(t *testing.T) {
	root := os.DirFS(backendRoot)
	const dir = "internal/play"
	entries, err := fs.ReadDir(root, dir)
	if err != nil {
		t.Fatalf("reading %s: %v", dir, err)
	}
	inTx, bumps := map[string]string{}, map[string]string{}
	files := 0
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
		in, bump, err := txHintKeys(p, src)
		if err != nil {
			t.Fatalf("parsing %s: %v", p, err)
		}
		for _, k := range in {
			inTx[k] = fmt.Sprintf("%s tells the streams (send or tellMembers) inside a transaction's closure: collect with h.tell in Service.tx, "+
				"or send after db.InTx returns, or list the function in txhints_exceptions.txt", k)
		}
		for _, k := range bump {
			bumps[k] = fmt.Sprintf("%s calls BumpHintSeq: only %s may", k, bumpFunnel)
		}
	}
	if files == 0 {
		t.Fatal("found no Go file: is backendRoot right?")
	}
	failProblems(t, compareExceptions("txhints_exceptions.txt", parseExceptions(hintTxExceptions), inTx))
	for _, p := range bumps {
		t.Error(p)
	}
	t.Logf("%d non-test Go files checked; %d known exceptions", files, len(inTx))
}

// txHintKeys returns the "file:function" keys of a file's functions that call send or tellMembers inside a transaction's
// closure, and of those that call BumpHintSeq (other than the funnel).
func txHintKeys(p string, src []byte) (inTx, bump []string, err error) {
	f, err := parser.ParseFile(token.NewFileSet(), p, src, parser.SkipObjectResolution)
	if err != nil {
		return nil, nil, err
	}
	add := func(keys *[]string, fn string) {
		if k := p + ":" + fn; k != bumpFunnel && !slices.Contains(*keys, k) {
			*keys = append(*keys, k)
		}
	}
	scan := func(n ast.Node, name string) {
		lits := funcLiterals(n)
		ast.Inspect(n, func(n ast.Node) bool {
			if lit, ok := n.(*ast.BasicLit); ok && lit.Kind == token.STRING && strings.Contains(lit.Value, "hint_seq") {
				if p+":"+name != bumpFunnel {
					add(&bump, name)
				}
			}
			call, ok := n.(*ast.CallExpr)
			if !ok {
				return true
			}
			if isSelector(call.Fun, "BumpHintSeq") {
				if k := p + ":" + name; k != bumpFunnel {
					add(&bump, name)
				}
			}
			if !isTxCall(call) {
				return true
			}
			for _, arg := range call.Args {
				if id, ok := arg.(*ast.Ident); ok && lits[id.Name] != nil {
					arg = lits[id.Name]
				}
				if lit, ok := arg.(*ast.FuncLit); ok && callsTell(lit) {
					add(&inTx, name)
				}
			}
			return true
		})
	}
	for _, decl := range f.Decls {
		switch d := decl.(type) {
		case *ast.FuncDecl:
			scan(d, funcName(d))
		case *ast.GenDecl:
			for _, spec := range d.Specs {
				if vs, ok := spec.(*ast.ValueSpec); ok {
					name := "init"
					if len(vs.Names) > 0 && vs.Names[0].Name != "_" {
						name = vs.Names[0].Name
					}
					scan(vs, name)
				}
			}
		}
	}
	return inTx, bump, nil
}

// funcLiterals maps the names assigned a function literal anywhere in n (name := func..., name = func..., var name = func...)
// to the literal. A name assigned twice keeps the last literal.
func funcLiterals(n ast.Node) map[string]*ast.FuncLit {
	out := map[string]*ast.FuncLit{}
	ast.Inspect(n, func(n ast.Node) bool {
		switch n := n.(type) {
		case *ast.AssignStmt:
			if len(n.Lhs) == len(n.Rhs) {
				for i, l := range n.Lhs {
					if id, ok := l.(*ast.Ident); ok {
						if lit, ok := n.Rhs[i].(*ast.FuncLit); ok {
							out[id.Name] = lit
						}
					}
				}
			}
		case *ast.ValueSpec:
			if len(n.Names) == len(n.Values) {
				for i, id := range n.Names {
					if lit, ok := n.Values[i].(*ast.FuncLit); ok {
						out[id.Name] = lit
					}
				}
			}
		}
		return true
	})
	return out
}

func isSelector(e ast.Expr, name string) bool {
	sel, ok := e.(*ast.SelectorExpr)
	return ok && sel.Sel.Name == name
}

// isTxCall reports whether call is x.tx(...), x.txOn(...) or db.InTx(...).
func isTxCall(call *ast.CallExpr) bool {
	if isSelector(call.Fun, "tx") || isSelector(call.Fun, "txOn") {
		return true
	}
	sel, ok := call.Fun.(*ast.SelectorExpr)
	if !ok || sel.Sel.Name != "InTx" {
		return false
	}
	id, ok := sel.X.(*ast.Ident)
	return ok && id.Name == "db"
}

// callsTell reports whether the literal, nested literals included, calls send or tellMembers.
func callsTell(lit *ast.FuncLit) bool {
	found := false
	ast.Inspect(lit, func(n ast.Node) bool {
		if call, ok := n.(*ast.CallExpr); ok && (isSelector(call.Fun, "send") || isSelector(call.Fun, "tellMembers")) {
			found = true
		}
		return !found
	})
	return found
}

func TestTxHintKeys(t *testing.T) {
	cases := []struct {
		name, path, src  string
		wantIn, wantBump []string
	}{
		{"send in s.tx", "internal/play/a.go", "package play\nfunc (s *S) F() { s.tx(c, id, func(tx T, h *H) error { s.send(c, e); return nil }) }", []string{"internal/play/a.go:(*S).F"}, nil},
		{"tellMembers in db.InTx", "internal/play/a.go", "package play\nfunc F(s S) { db.InTx(c, p, func(tx T) error { s.tellMembers(c, true, m); return nil }) }", []string{"internal/play/a.go:F"}, nil},
		{"nested literal", "internal/play/a.go", "package play\nfunc F(s S) { s.tx(c, id, func(tx T, h *H) error { go func() { s.send(c, e) }(); return nil }) }", []string{"internal/play/a.go:F"}, nil},
		{"send after the transaction", "internal/play/a.go", "package play\nfunc F(s S) { s.tx(c, id, func(tx T, h *H) error { return nil }); s.send(c, e) }", nil, nil},
		{"tell and touch are allowed", "internal/play/a.go", "package play\nfunc F(s S) { s.tx(c, id, func(tx T, h *H) error { h.tell(c, e); h.touch(); return nil }) }", nil, nil},
		{"a literal that is not an argument of a tx", "internal/play/a.go", "package play\nfunc F(s S) { f := func() { s.send(c, e) }; f() }", nil, nil},
		{"another InTx", "internal/play/a.go", "package play\nfunc F(s S) { other.InTx(c, p, func(tx T) error { s.send(c, e); return nil }) }", nil, nil},
		{"package-level literal", "internal/play/a.go", "package play\nvar run = func(s S) { s.tx(c, id, func(tx T, h *H) error { s.send(c, e); return nil }) }", []string{"internal/play/a.go:run"}, nil},
		{"one key per function", "internal/play/a.go", "package play\nfunc F(s S) { s.tx(c, id, func(tx T, h *H) error { s.send(c, e); s.send(c, e); return nil }) }", []string{"internal/play/a.go:F"}, nil},
		{"named closure", "internal/play/a.go", "package play\nfunc F(s S) { run := func(tx T, h *H) error { s.send(c, e); return nil }; s.tx(c, id, run) }", []string{"internal/play/a.go:F"}, nil},
		{"named closure that only collects", "internal/play/a.go", "package play\nfunc F(s S) { run := func(tx T, h *H) error { h.tell(c, e); return nil }; s.tx(c, id, run) }", nil, nil},
		{"named closure declared with var", "internal/play/a.go", "package play\nfunc F(s S) { var run = func(tx T) error { s.tellMembers(c, true, m); return nil }; db.InTx(c, p, run) }", []string{"internal/play/a.go:F"}, nil},
		{"a closure that is a parameter is not read", "internal/play/a.go", "package play\nfunc F(s S, run func(T, *H) error) { s.tx(c, id, run) }", nil, nil},
		{"a helper that sends is not seen", "internal/play/a.go", "package play\nfunc F(s S) { s.tx(c, id, func(tx T, h *H) error { return s.helper() }) }\nfunc (s S) helper() error { s.send(c, e); return nil }", nil, nil},
		{"raw SQL naming hint_seq", "internal/play/a.go", "package play\nfunc F(tx T) { tx.Exec(c, `UPDATE game_sessions SET hint_seq = 5`) }", nil, []string{"internal/play/a.go:F"}},
		{"hint_seq in a comment is fine", "internal/play/a.go", "package play\n// hint_seq\nfunc F() {}", nil, nil},
		{"BumpHintSeq elsewhere", "internal/play/a.go", "package play\nfunc F(q Q) { q.BumpHintSeq(c, id) }", nil, []string{"internal/play/a.go:F"}},
		{"BumpHintSeq in the funnel", "internal/play/hints.go", "package play\nfunc (s *Service) txOn() { q.BumpHintSeq(c, id) }", nil, nil},
		{"BumpHintSeq elsewhere in the funnel file", "internal/play/hints.go", "package play\nfunc (s *Service) other() { q.BumpHintSeq(c, id) }", nil, []string{"internal/play/hints.go:(*Service).other"}},
	}
	for _, c := range cases {
		in, bump, err := txHintKeys(c.path, []byte(c.src))
		if err != nil {
			t.Fatalf("%s: %v", c.name, err)
		}
		if !slices.Equal(in, c.wantIn) || !slices.Equal(bump, c.wantBump) {
			t.Errorf("%s: got in-tx %q bump %q, want %q %q", c.name, in, bump, c.wantIn, c.wantBump)
		}
	}
}
