package formula

import (
	"strings"
	"testing"
)

// wizard3 is the environment of a 3rd-level wizard with INT 18 (Pensantus).
func wizard3() *Env {
	scores := map[string]int{"str": 12, "dex": 16, "con": 16, "int": 18, "wis": 13, "cha": 12}
	return &Env{
		Level: func() int { return 3 },
		ClassLevel: func(class string) int {
			if class == "wizard" {
				return 3
			}
			return 0
		},
		Mod:    func(a string) int { return scores[a]/2 - 5 },
		Score:  func(a string) int { return scores[a] },
		Prof:   func() int { return 2 },
		Armor:  func() string { return "none" },
		Shield: func() bool { return false },
	}
}

func newCompiler() *Compiler {
	return NewCompiler([]string{"barbarian", "monk", "sorcerer", "wizard"})
}

func TestIntFormulas(t *testing.T) {
	t.Parallel()
	tests := []struct {
		source string
		want   int
	}{
		// ADR-0008's example: Arcane Recovery, half the wizard level rounded up.
		{`ceil(classLevel("wizard") / 2)`, 2},
		{`floor(classLevel("wizard") / 2)`, 1},
		// "/" always returns a float in Expr; AsInt truncates it.
		{`classLevel("wizard") / 2`, 1},
		{`8 + prof() + mod("int")`, 14},
		{`prof() + mod("int")`, 6},
		{`max(1, mod("int") + classLevel("wizard"))`, 7},
		{`min(mod("dex"), 2)`, 2},
		{`10 + mod("dex") + mod("con")`, 16},
		{`score("str")`, 12},
		{`level()`, 3},
		{`classLevel("monk") >= 6 ? 15 : 10`, 10},
		{`-mod("str")`, -1},
		{`7 % 3`, 1},
		{`ceil(3)`, 3},
	}
	c := newCompiler()
	for _, tt := range tests {
		t.Run(tt.source, func(t *testing.T) {
			t.Parallel()
			p, err := c.Compile(tt.source, Int)
			if err != nil {
				t.Fatalf("Compile: %v", err)
			}
			got, err := p.Int(wizard3())
			if err != nil {
				t.Fatalf("Int: %v", err)
			}
			if got != tt.want {
				t.Errorf("= %d, want %d", got, tt.want)
			}
		})
	}
}

func TestBoolFormulas(t *testing.T) {
	t.Parallel()
	tests := []struct {
		source string
		want   bool
	}{
		{`armor() == "none"`, true},
		{`"none" == armor()`, true},
		{`armor() != "heavy"`, true},
		{`armor() == "none" and not shield()`, true},
		{`shield() || level() > 2`, true},
		{`!(classLevel("wizard") >= 3)`, false},
		{`true`, true},
	}
	c := newCompiler()
	for _, tt := range tests {
		t.Run(tt.source, func(t *testing.T) {
			t.Parallel()
			p, err := c.Compile(tt.source, Bool)
			if err != nil {
				t.Fatalf("Compile: %v", err)
			}
			got, err := p.Bool(wizard3())
			if err != nil {
				t.Fatalf("Bool: %v", err)
			}
			if got != tt.want {
				t.Errorf("= %v, want %v", got, tt.want)
			}
		})
	}
}

// TestFormulaWhitelist feeds every forbidden construct to Compile. Each must
// be refused before it can run.
func TestFormulaWhitelist(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name, source string
		kind         Kind
	}{
		{"range allocates", `len(1..1000000)`, Int},
		{"range alone", `1..10`, Int},
		{"range sum", `sum(1..100)`, Int},
		{"matches", `armor() matches "h.*"`, Bool},
		{"in", `"int" in ["int"]`, Bool},
		{"contains", `armor() contains "e"`, Bool},
		{"startsWith", `armor() startsWith "h"`, Bool},
		{"member access", `Env.Level`, Int},
		{"method call", `armor().Len()`, Int},
		{"index", `[1, 2][0]`, Int},
		{"pipe", `level() | max(1)`, Int},
		{"nil coalescing", `nil ?? 1`, Int},
		{"let", `let x = 1; x`, Int},
		{"sequence", `1; 2`, Int},
		{"closure", `map([1, 2], # * 2)[0]`, Int},
		{"filter", `len(filter([1], # > 0))`, Int},
		{"array literal", `[1]`, Int},
		{"map literal", `{"a": 1}.a`, Int},
		{"nil", `nil`, Int},
		{"builtin len", `len("abc")`, Int},
		{"builtin now", `now().Year()`, Int},
		{"builtin abs", `abs(-1)`, Int},
		{"power", `2 ** 10`, Int},
		{"caret power", `2 ^ 10`, Int},
		{"unknown function", `rand()`, Int},
		{"unknown identifier", `level`, Int},
		{"env field without call", `prof`, Int},
		{"ability not literal", `mod(armor())`, Int},
		{"unknown ability", `mod("luck")`, Int},
		{"unknown class", `classLevel("artificer")`, Int},
		{"class key instead of index", `classLevel("class:wizard")`, Int},
		{"too many arguments", `level(1)`, Int},
		{"too few arguments", `max(1)`, Int},
		{"string outside comparison", `"int"`, Bool},
		{"string compared with non-armor", `mod("int") == "4"`, Bool},
		{"two strings", `"a" == "b"`, Bool},
		{"unknown armor category", `armor() == "plate"`, Bool},
		{"huge literal", `level() * 1000000`, Int},
		{"too long", "1" + strings.Repeat(" + 1", 100), Int},
		{"too many nodes", strings.TrimSuffix(strings.Repeat("level()+", 40), "+"), Int},
		{"wrong result type for Int", `armor() == "none"`, Int},
		{"wrong result type for Bool", `level()`, Bool},
		{"bytes literal", `b"abc"`, Int},
		{"slice", `"abc"[0:1]`, Int},
		{"optional chaining", `Env?.Level`, Int},
		{"if else with range", `if true { 1..3 } else { 1 }`, Int},
		{"parse error", `(1 +`, Int},
	}
	c := newCompiler()
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			if _, err := c.Compile(tt.source, tt.kind); err == nil {
				t.Errorf("Compile(%q) = nil error, want refusal", tt.source)
			}
		})
	}
}

// TestRunTimeFailuresAreErrors: what can only fail while running returns an
// error, never a panic.
func TestRunTimeFailuresAreErrors(t *testing.T) {
	t.Parallel()
	c := newCompiler()
	tests := []string{
		`level() % (classLevel("monk"))`,      // integer division by zero
		`floor(level() / classLevel("monk"))`, // float division by zero: +Inf
		`ceil(mod("str") / (level() - 3))`,    // 1 / 0
	}
	for _, source := range tests {
		t.Run(source, func(t *testing.T) {
			t.Parallel()
			p, err := c.Compile(source, Int)
			if err != nil {
				t.Fatalf("Compile: %v", err)
			}
			if got, err := p.Int(wizard3()); err == nil {
				t.Errorf("Int = %d, want an error", got)
			}
		})
	}

	t.Run("nil environment", func(t *testing.T) {
		t.Parallel()
		p, err := c.Compile(`level()`, Int)
		if err != nil {
			t.Fatal(err)
		}
		if _, err := p.Int(nil); err == nil {
			t.Error("want an error")
		}
		if _, err := p.Int(&Env{}); err == nil {
			t.Error("empty Env: want an error")
		}
		if _, err := p.Bool(wizard3()); err == nil {
			t.Error("Bool on an Int formula: want an error")
		}
	})
}

// A product of literals wraps in 64-bit integer arithmetic (1000 is 2^3*5^3,
// so 22 factors of it come to 0), which would slip under MaxResult: a formula
// whose steps could grow that far is refused when it is compiled.
func TestProductsThatCouldOverflowAreRefused(t *testing.T) {
	t.Parallel()
	product := strings.TrimSuffix(strings.Repeat("1000*", 22), "*")
	c := newCompiler()
	for _, source := range []string{
		product,
		product + " + 5",
		product + " + level()",
		"1000 * 1000 * 1000 * 1000",
		"level() * 1000 * 1000 * 1000",
	} {
		if _, err := c.Compile(source, Int); err == nil {
			t.Errorf("%q (%d bytes) was accepted; a product that could overflow must be refused", source[max(0, len(source)-12):], len(source))
		}
	}
	// The multiplications the rules content really writes stay accepted.
	for source, want := range map[string]int{
		"5 * classLevel(\"wizard\")":      15,
		"1000 * level() * 2":              6000,
		"8 + prof() * mod(\"int\") * 100": 808,
		"(level() + 4) * (prof() + 1)":    21,
	} {
		p, err := c.Compile(source, Int)
		if err != nil {
			t.Errorf("%q was refused: %v", source, err)
			continue
		}
		if got, err := p.Int(wizard3()); err != nil || got != want {
			t.Errorf("%q = %d, %v; want %d", source, got, err, want)
		}
	}
}
