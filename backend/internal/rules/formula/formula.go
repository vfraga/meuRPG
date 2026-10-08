// Package formula compiles and runs the small arithmetic formulas that rules
// content uses, such as "8 + prof() + mod(\"int\")" or
// "ceil(classLevel(\"wizard\") / 2)" (ADR-0008).
//
// Formulas run on Expr (github.com/expr-lang/expr), locked down so that a
// formula can only do arithmetic over the character:
//
//   - The environment is the Env struct: a closed set of pure functions
//     (level, classLevel, mod, score, prof, armor, shield). Nothing in it
//     touches a database, the network, the clock or randomness, and an
//     unknown name is a compile error.
//   - Every Expr builtin is disabled. Only floor, ceil, min and max come
//     back, as our own two-line functions with typed signatures.
//   - Before compiling, the syntax tree is checked against a whitelist:
//     numbers, booleans, arithmetic, comparisons, and/or/not, the ternary,
//     and calls to the functions above with literal arguments from known
//     sets. Ranges ("1..1000000" allocates even without builtins), member
//     access, pipes, "in", "matches", "??", "let", arrays, maps and
//     closures are refused.
//   - A formula has at most MaxSourceBytes bytes and MaxNodes nodes. It is
//     compiled once, when content loads, never while serving a request.
//   - Running a formula never panics: any run-time failure (a division by
//     zero in "%", an out-of-range result) comes back as an error, and the
//     engine turns it into an Issue on the sheet.
package formula

import (
	"errors"
	"fmt"
	"math"
	"slices"

	"github.com/expr-lang/expr"
	"github.com/expr-lang/expr/ast"
	"github.com/expr-lang/expr/conf"
	"github.com/expr-lang/expr/file"
	"github.com/expr-lang/expr/parser"
	"github.com/expr-lang/expr/parser/lexer"
	"github.com/expr-lang/expr/vm"
)

// Limits on a formula.
const (
	// MaxSourceBytes bounds the formula's text. SRD formulas are one short
	// line; the longest we have (Unarmored Movement) is about 150 bytes.
	MaxSourceBytes = 256
	// MaxNodes bounds the syntax tree, checked by Expr itself.
	MaxNodes = 64
	// MaxLiteral bounds every number written in a formula, and MaxResult
	// every number a formula returns. No 5e number comes near either, and
	// the bounds keep a formula from overflowing into nonsense.
	MaxLiteral = 1000
	MaxResult  = 10000
	// maxIntermediate bounds what any step of a formula can reach, worked out
	// from the literals and the most each function returns. Integer arithmetic
	// wraps in 64 bits, so a long product could slip under MaxResult; a formula
	// whose steps could pass this bound is refused when it is written.
	maxIntermediate = 1 << 31
)

// Env is what a formula can see. The engine fills the functions for one
// character; formulas call them, as in mod("int") or level().
type Env struct {
	// Level is the total character level.
	Level func() int `expr:"level"`
	// ClassLevel is the level in one class, by its SRD index ("wizard"), or
	// 0 when the character has no level in it.
	ClassLevel func(class string) int `expr:"classLevel"`
	// Mod is an ability modifier and Score an ability score, by the
	// ability's index ("str" ... "cha").
	Mod   func(ability string) int `expr:"mod"`
	Score func(ability string) int `expr:"score"`
	// Prof is the proficiency bonus.
	Prof func() int `expr:"prof"`
	// Armor is the category of the worn body armor: "none", "light",
	// "medium" or "heavy". Shield says whether a shield is carried.
	Armor  func() string `expr:"armor"`
	Shield func() bool   `expr:"shield"`
}

// Abilities are the arguments mod and score accept.
var abilities = []string{"str", "dex", "con", "int", "wis", "cha"}

// ArmorCategories are the values armor() returns, and the only strings a
// formula may compare it with.
var armorCategories = []string{"none", "light", "medium", "heavy"}

// Kind is the type a formula must return.
type Kind int

// The two kinds of formula.
const (
	// Int formulas compute a number: a modifier's value, a resource's
	// maximum, the number of prepared spells.
	Int Kind = iota
	// Bool formulas are conditions, such as `armor() == "none"`.
	Bool
)

// Program is a compiled formula.
type Program struct {
	source  string
	kind    Kind
	program *vm.Program
}

// Source returns the formula's text.
func (p *Program) Source() string { return p.source }

// Compiler compiles formulas for one set of content. It knows the class
// indexes that classLevel accepts.
type Compiler struct {
	classes []string
}

// NewCompiler returns a Compiler that accepts classLevel of these class
// indexes ("wizard", ...).
func NewCompiler(classes []string) *Compiler {
	return &Compiler{classes: slices.Clone(classes)}
}

// Compile checks and compiles a formula of the given kind. The error names
// what is wrong, never with more than the formula's own text.
func (c *Compiler) Compile(source string, kind Kind) (*Program, error) {
	if len(source) > MaxSourceBytes {
		return nil, fmt.Errorf("formula is %d bytes, the limit is %d", len(source), MaxSourceBytes)
	}
	if err := checkTokens(source); err != nil {
		return nil, err
	}
	opts := c.options(kind)

	// Parse with the same configuration Compile will use, so that floor,
	// ceil, min and max parse as calls to our functions, then check the
	// tree before Expr ever compiles it.
	config := conf.CreateNew()
	for _, opt := range opts {
		opt(config)
	}
	for name := range config.Disabled {
		delete(config.Builtins, name)
	}
	tree, err := parser.ParseWithConfig(source, config)
	if err != nil {
		return nil, fmt.Errorf("formula does not parse: %w", err)
	}
	if err := c.check(tree.Node); err != nil {
		return nil, err
	}
	if _, err := reach(tree.Node); err != nil {
		return nil, err
	}

	program, err := expr.Compile(source, opts...)
	if err != nil {
		return nil, fmt.Errorf("formula does not compile: %w", err)
	}
	return &Program{source: source, kind: kind, program: program}, nil
}

func (c *Compiler) options(kind Kind) []expr.Option {
	opts := []expr.Option{
		expr.Env(Env{}),
		expr.DisableAllBuiltins(),
		expr.MaxNodes(MaxNodes),
		expr.Function("floor", roundFunc(math.Floor), new(func(float64) int), new(func(int) int)),
		expr.Function("ceil", roundFunc(math.Ceil), new(func(float64) int), new(func(int) int)),
		expr.Function("min", pickFunc(func(a, b int) bool { return a < b }), new(func(int, int) int)),
		expr.Function("max", pickFunc(func(a, b int) bool { return a > b }), new(func(int, int) int)),
	}
	if kind == Bool {
		return append(opts, expr.AsBool())
	}
	return append(opts, expr.AsInt())
}

// roundFunc is floor or ceil: one number in, an int out. It refuses NaN and
// infinities (a float division by zero), which have no int value.
func roundFunc(round func(float64) float64) func(params ...any) (any, error) {
	return func(params ...any) (any, error) {
		if len(params) != 1 {
			return nil, errors.New("takes one number")
		}
		var x float64
		switch v := params[0].(type) {
		case int:
			return v, nil
		case float64:
			x = v
		default:
			return nil, errors.New("takes a number")
		}
		if math.IsNaN(x) || math.IsInf(x, 0) || math.Abs(x) > MaxResult {
			return nil, errors.New("number out of range")
		}
		return int(round(x)), nil
	}
}

// pickFunc is min or max of two ints. It never recurses into its arguments,
// unlike the builtins that CVE-2025-68156 was about.
func pickFunc(better func(a, b int) bool) func(params ...any) (any, error) {
	return func(params ...any) (any, error) {
		if len(params) != 2 {
			return nil, errors.New("takes two numbers")
		}
		a, okA := params[0].(int)
		b, okB := params[1].(int)
		if !okA || !okB {
			return nil, errors.New("takes two whole numbers")
		}
		if better(b, a) {
			return b, nil
		}
		return a, nil
	}
}

// The operators a formula may use.
var (
	unaryOperators  = []string{"-", "+", "not", "!"}
	binaryOperators = []string{
		"+", "-", "*", "/", "%",
		"==", "!=", "<", "<=", ">", ">=",
		"and", "or", "&&", "||",
	}
)

// checkTokens refuses operators and brackets outside the whitelist. Most of
// them would also fail the tree check below, but a pipe ("x | f()") leaves
// no trace in the tree, so this is the only place that sees it.
func checkTokens(source string) error {
	tokens, err := lexer.Lex(file.NewSource(source))
	if err != nil {
		return fmt.Errorf("formula does not parse: %w", err)
	}
	for _, t := range tokens {
		switch t.Kind {
		case lexer.Operator:
			if !slices.Contains(unaryOperators, t.Value) && !slices.Contains(binaryOperators, t.Value) &&
				t.Value != "?" && t.Value != ":" && t.Value != "," {
				return fmt.Errorf("operator %q is not allowed", t.Value)
			}
		case lexer.Bracket:
			if t.Value != "(" && t.Value != ")" {
				return fmt.Errorf("%q is not allowed", t.Value)
			}
		case lexer.Bytes:
			return errors.New("byte strings are not allowed")
		}
	}
	return nil
}

// check walks the tree and refuses anything outside the whitelist.
func (c *Compiler) check(node ast.Node) error {
	switch n := node.(type) {
	case *ast.IntegerNode:
		if n.Value > MaxLiteral || n.Value < -MaxLiteral {
			return fmt.Errorf("number %d is out of range (at most %d)", n.Value, MaxLiteral)
		}
		return nil
	case *ast.FloatNode:
		if math.IsNaN(n.Value) || math.Abs(n.Value) > MaxLiteral {
			return errors.New("number is out of range")
		}
		return nil
	case *ast.BoolNode:
		return nil
	case *ast.StringNode:
		// Strings only make sense as a function's argument or compared with
		// armor(); both are checked where they appear.
		return errors.New("a text value can only be an argument of mod, score or classLevel, or compared with armor()")
	case *ast.UnaryNode:
		if !slices.Contains(unaryOperators, n.Operator) {
			return fmt.Errorf("operator %q is not allowed", n.Operator)
		}
		return c.check(n.Node)
	case *ast.BinaryNode:
		if !slices.Contains(binaryOperators, n.Operator) {
			return fmt.Errorf("operator %q is not allowed", n.Operator)
		}
		if n.Operator == "==" || n.Operator == "!=" {
			if ok, err := checkArmorComparison(n); ok || err != nil {
				return err
			}
		}
		if err := c.check(n.Left); err != nil {
			return err
		}
		return c.check(n.Right)
	case *ast.ConditionalNode:
		for _, part := range []ast.Node{n.Cond, n.Exp1, n.Exp2} {
			if err := c.check(part); err != nil {
				return err
			}
		}
		return nil
	case *ast.CallNode:
		return c.checkCall(n)
	default:
		return fmt.Errorf("%s is not allowed in a formula", describe(node))
	}
}

// reach is the largest magnitude a checked node can take: a literal is itself,
// a call is at most MaxLiteral (no function returns more in a 5e sheet), and an
// operator combines its operands'. It refuses a node that could pass
// maxIntermediate, so no step of the arithmetic can wrap.
func reach(node ast.Node) (float64, error) {
	var out float64
	switch n := node.(type) {
	case *ast.IntegerNode:
		out = math.Abs(float64(n.Value))
	case *ast.FloatNode:
		out = math.Abs(n.Value)
	case *ast.UnaryNode:
		x, err := reach(n.Node)
		if err != nil {
			return 0, err
		}
		out = x
	case *ast.BinaryNode:
		l, err := reach(n.Left)
		if err != nil {
			return 0, err
		}
		r, err := reach(n.Right)
		if err != nil {
			return 0, err
		}
		switch n.Operator {
		case "+", "-":
			out = l + r
		case "*":
			out = l * r
		case "/", "%":
			out = l
		default: // a comparison or a logical operator gives true or false
			out = 1
		}
	case *ast.ConditionalNode:
		for _, part := range []ast.Node{n.Exp1, n.Exp2} {
			x, err := reach(part)
			if err != nil {
				return 0, err
			}
			out = max(out, x)
		}
		if _, err := reach(n.Cond); err != nil {
			return 0, err
		}
	case *ast.CallNode:
		out = MaxLiteral
		if id, ok := n.Callee.(*ast.IdentifierNode); ok && (id.Value == "floor" || id.Value == "ceil" || id.Value == "min" || id.Value == "max") {
			for _, arg := range n.Arguments {
				x, err := reach(arg)
				if err != nil {
					return 0, err
				}
				out = max(out, x)
			}
		}
	}
	if out > maxIntermediate {
		return 0, errors.New("a step of the formula could grow past the limit; use smaller numbers")
	}
	return out, nil
}

// checkCall allows a call to one of our functions, with the right number
// of arguments; text arguments must be literals from the function's set.
func (c *Compiler) checkCall(n *ast.CallNode) error {
	callee, ok := n.Callee.(*ast.IdentifierNode)
	if !ok {
		return errors.New("only direct calls to the formula functions are allowed")
	}
	var literals []string // non-nil: the function takes one text argument from this set
	arity := 0
	switch callee.Value {
	case "level", "prof", "armor", "shield":
	case "mod", "score":
		literals, arity = abilities, 1
	case "classLevel":
		literals, arity = c.classes, 1
	case "floor", "ceil":
		arity = 1
	case "min", "max":
		arity = 2
	default:
		return fmt.Errorf("function %q does not exist in formulas", callee.Value)
	}
	if len(n.Arguments) != arity {
		return fmt.Errorf("%s takes %d argument(s), got %d", callee.Value, arity, len(n.Arguments))
	}
	if literals != nil {
		s, ok := n.Arguments[0].(*ast.StringNode)
		if !ok {
			return fmt.Errorf("the argument of %s must be written as text, such as %s(%q)", callee.Value, callee.Value, literals[0])
		}
		if !slices.Contains(literals, s.Value) {
			return fmt.Errorf("%s does not accept %q", callee.Value, s.Value)
		}
		return nil
	}
	for _, arg := range n.Arguments {
		if err := c.check(arg); err != nil {
			return err
		}
	}
	return nil
}

// checkArmorComparison allows `armor() == "heavy"` (either side) with a
// known category. It reports ok=false when the comparison is not with
// armor(), so the caller checks it as ordinary arithmetic.
func checkArmorComparison(n *ast.BinaryNode) (bool, error) {
	left, right := n.Left, n.Right
	if _, isString := left.(*ast.StringNode); isString {
		left, right = right, left
	}
	s, isString := right.(*ast.StringNode)
	if !isString {
		return false, nil
	}
	call, isCall := left.(*ast.CallNode)
	if !isCall || len(call.Arguments) != 0 {
		return true, errors.New("text can only be compared with armor()")
	}
	if id, ok := call.Callee.(*ast.IdentifierNode); !ok || id.Value != "armor" {
		return true, errors.New("text can only be compared with armor()")
	}
	if !slices.Contains(armorCategories, s.Value) {
		return true, fmt.Errorf("armor() is never %q", s.Value)
	}
	return true, nil
}

func describe(node ast.Node) string {
	switch n := node.(type) {
	case *ast.IdentifierNode:
		return fmt.Sprintf("the name %q (call it, as in %s())", n.Value, n.Value)
	case *ast.BuiltinNode:
		return fmt.Sprintf("the builtin %q", n.Name)
	case *ast.MemberNode, *ast.ChainNode:
		return "member access"
	case *ast.SliceNode:
		return "slicing"
	case *ast.ArrayNode:
		return "an array"
	case *ast.MapNode, *ast.PairNode:
		return "a map"
	case *ast.VariableDeclaratorNode:
		return "let"
	case *ast.SequenceNode:
		return "a sequence (;)"
	case *ast.PredicateNode, *ast.PointerNode:
		return "a closure"
	case *ast.NilNode:
		return "nil"
	default:
		return fmt.Sprintf("%T", node)
	}
}

// Int runs an Int formula.
func (p *Program) Int(env *Env) (int, error) {
	if p.kind != Int {
		return 0, errors.New("formula is a condition, not a number")
	}
	out, err := p.run(env)
	if err != nil {
		return 0, err
	}
	n, ok := out.(int)
	if !ok {
		return 0, fmt.Errorf("formula returned %T, not a number", out)
	}
	if n > MaxResult || n < -MaxResult {
		return 0, fmt.Errorf("formula returned %d, out of range", n)
	}
	return n, nil
}

// Bool runs a Bool formula.
func (p *Program) Bool(env *Env) (bool, error) {
	if p.kind != Bool {
		return false, errors.New("formula is a number, not a condition")
	}
	out, err := p.run(env)
	if err != nil {
		return false, err
	}
	b, ok := out.(bool)
	if !ok {
		return false, fmt.Errorf("formula returned %T, not true or false", out)
	}
	return b, nil
}

// run executes the program and turns any panic into an error. Expr already
// recovers inside its VM; this is the second belt, because a panic here
// would take down a request that only wanted to read a sheet.
func (p *Program) run(env *Env) (out any, err error) {
	defer func() {
		if r := recover(); r != nil {
			out, err = nil, fmt.Errorf("formula failed: %v", r)
		}
	}()
	if env == nil {
		return nil, errors.New("formula needs an environment")
	}
	out, err = expr.Run(p.program, *env)
	if err != nil {
		return nil, fmt.Errorf("formula failed: %w", err)
	}
	return out, nil
}
