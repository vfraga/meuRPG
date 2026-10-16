package formula

import (
	"flag"
	"fmt"
	"math"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/expr-lang/expr/ast"
)

var updateFormulaGolden = flag.Bool("update", false, "rewrite testdata/compile.golden from the current code")

// compileCases reach every branch of check and reach through Compile: each
// node kind, each refusal (the first one found wins, so the order of the
// checks shows), and products that could wrap on each side of each operator.
var compileCases = []string{
	`1000`, `1001`, `-1001`, `1000.5`, `999.5`, `true`, `"heavy"`, `-mod("str")`, `+3`, `!true`, `not shield()`,
	`1 + 2`, `5 % 2`, `3 / 2`, `1 < 2`, `1 <= 2 and 2 >= 1`, `true || false`, `1 ** 2`, `1 in 2`,
	`armor() == "heavy"`, `"heavy" == armor()`, `armor() != "none"`, `armor() == "mithral"`,
	`level() == "x"`, `"x" == "y"`, `armor(1) == "none"`, `shield() == "none"`, `1 == 1`, `level() != 1001`, `1001 == level()`,
	`true ? 1 : 2`, `1001 ? 1 : 2`, `true ? 1001 : 2`, `true ? 1 : 1001`, `level() > 3 ? "a" : 2`,
	`a.b`, `x`, `[1]`, `{"a": 1}`, `nil`, `let x = 1; x`, `1; 2`, `$env`,
	`mod("str")`, `mod(str)`, `mod("xyz")`, `score("cha")`, `classLevel("wizard")`, `classLevel("bard")`, `classLevel(1)`,
	`level(1)`, `prof()`, `armor()`, `shield(1)`, `foo()`, `(level)()`, `floor(level() / 2)`, `ceil(1001)`, `floor()`,
	`min(1)`, `max(1, 2)`, `min(1, 1001)`, `max("a", 1)`,
	`1000 * 1000 * 1000`, `1000 * 1000 * 1000 * 1000`, `1000 * 1000 * 1000 + 1000 * 1000 * 1000 + 1000 * 1000 * 1000`,
	`1000 * 1000 * 1000 * 3 / 2`, `1 + 1000 * 1000 * 1000 * 1000`, `1000 * 1000 * 1000 * 1000 - 1`, `-(1000 * 1000 * 1000 * 1000)`,
	`(1000 * 1000 * 1000 * 1000) % 7`, `7 % (1000 * 1000 * 1000 * 1000)`, `1000 * 1000 * 1000 * 1000 == 1`, `1000 * 1000 * 1000 * 1000 > 1 and true`,
	`true ? 1000 * 1000 * 1000 * 1000 : 1`, `true ? 1 : 1000 * 1000 * 1000 * 1000`, `1000 * 1000 * 1000 * 1000 > 1 ? 1 : 2`,
	`max(1000 * 1000 * 1000 * 1000, 1)`, `min(1, 1000 * 1000 * 1000 * 1000)`, `floor(1000.0 * 1000 * 1000 * 1000)`, `ceil(1000 * 1000 * 1000 * 3)`,
	`level() * 1000 * 1000 * 1000`, `level() * 1000 * 1000 * 1000 * 3`, `mod("str") * 1000 * 1000`, `prof() * prof() * prof() * prof()`,
	`true ? level() * 1000 * 1000 : 1000 * 1000 * 1000`, `999.9 * 999.9 * 999.9 * 999.9`,
}

// TestCompileCharacterization freezes what Compile says of every case, as a
// number and as a condition, in testdata/compile.golden.
func TestCompileCharacterization(t *testing.T) {
	t.Parallel()
	c := newCompiler()
	var b strings.Builder
	for _, src := range compileCases {
		for _, kind := range []Kind{Int, Bool} {
			p, err := c.Compile(src, kind)
			fmt.Fprintf(&b, "%s\tkind=%d\t", src, kind)
			switch {
			case err != nil:
				fmt.Fprintf(&b, "refused: %v\n", err)
			case kind == Int:
				n, runErr := p.Int(wizard3())
				fmt.Fprintf(&b, "ok: %d %v\n", n, runErr)
			default:
				v, runErr := p.Bool(wizard3())
				fmt.Fprintf(&b, "ok: %v %v\n", v, runErr)
			}
		}
	}
	got := b.String()
	path := filepath.Join("testdata", "compile.golden")
	if *updateFormulaGolden {
		if err := os.MkdirAll("testdata", 0o750); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(path, []byte(got), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	want, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("%v (run with -update to create it)", err)
	}
	if got != string(want) {
		t.Errorf("Compile differs from %s; if the change is right, run with -update and review the diff.\ngot:\n%s", path, got)
	}
}

// TestCheckAndReachOnBuiltTreesCharacterization reaches what the parser
// never hands over: operators outside the whitelist in the tree, a NaN
// literal, and the nodes reach does not know (worth 0).
func TestCheckAndReachOnBuiltTreesCharacterization(t *testing.T) {
	t.Parallel()
	c := newCompiler()
	one := &ast.IntegerNode{Value: 1}
	for _, tc := range []struct {
		name  string
		node  ast.Node
		check string
		reach string
	}{
		{"a unary operator outside the whitelist", &ast.UnaryNode{Operator: "~", Node: one}, `operator "~" is not allowed`, "1 <nil>"},
		{"a binary operator outside the whitelist", &ast.BinaryNode{Operator: "**", Left: one, Right: one}, `operator "**" is not allowed`, "1 <nil>"},
		{"a NaN literal", &ast.FloatNode{Value: math.NaN()}, "number is out of range", "NaN <nil>"},
		{"a negative float", &ast.FloatNode{Value: -2.5}, "<nil>", "2.5 <nil>"},
		{"a bool", &ast.BoolNode{Value: true}, "<nil>", "0 <nil>"},
		{"a string", &ast.StringNode{Value: "x"}, "a text value can only be an argument of mod, score or classLevel, or compared with armor()", "0 <nil>"},
		{"a call of a call", &ast.CallNode{Callee: &ast.CallNode{Callee: &ast.IdentifierNode{Value: "level"}}}, "only direct calls to the formula functions are allowed", "1000 <nil>"},
		{"a builtin", &ast.BuiltinNode{Name: "len"}, `the builtin "len" is not allowed in a formula`, "0 <nil>"},
		{"a slice", &ast.SliceNode{Node: one}, "slicing is not allowed in a formula", "0 <nil>"},
		{"a closure", &ast.PredicateNode{Node: one}, "a closure is not allowed in a formula", "0 <nil>"},
		{"a pointer", &ast.PointerNode{}, "a closure is not allowed in a formula", "0 <nil>"},
		{"a chain", &ast.ChainNode{Node: one}, "member access is not allowed in a formula", "0 <nil>"},
		{"a pair", &ast.PairNode{Key: one, Value: one}, "a map is not allowed in a formula", "0 <nil>"},
		{"a constant", &ast.ConstantNode{Value: 1}, "*ast.ConstantNode is not allowed in a formula", "0 <nil>"},
		{"a huge float", &ast.FloatNode{Value: 3e9}, "number is out of range", "0 a step of the formula could grow past the limit; use smaller numbers"},
		{"a huge integer", &ast.IntegerNode{Value: -3e9}, "number -3000000000 is out of range (at most 1000)", "0 a step of the formula could grow past the limit; use smaller numbers"},
	} {
		if got := fmt.Sprint(c.check(tc.node)); got != tc.check {
			t.Errorf("%s: check = %q, want %q", tc.name, got, tc.check)
		}
		x, err := reach(tc.node)
		if got := fmt.Sprint(x, " ", err); got != tc.reach {
			t.Errorf("%s: reach = %q, want %q", tc.name, got, tc.reach)
		}
	}
}
