package formula

import "testing"

// SRD 5.1 "Ability Scores and Modifiers": subtract 10, divide by 2, round down.
func TestRulesReviewExpressions_DivisionRoundsDown(t *testing.T) {
	t.Parallel()
	c := newCompiler()
	for _, tt := range []struct {
		source string
		want   int
	}{
		{`(score("str") - 10) / 2`, -2}, // str 12 in wizard3 -> +1; overridden below
		{`-3 / 2`, -2},
	} {
		p, err := c.Compile(tt.source, Int)
		if err != nil {
			t.Fatalf("Compile(%q): %v", tt.source, err)
		}
		env := wizard3()
		env.Score = func(string) int { return 7 }
		env.Mod = func(string) int { return -2 }
		got, err := p.Int(env)
		if err != nil {
			t.Fatalf("Int: %v", err)
		}
		if got != tt.want {
			t.Errorf("%s = %d, want %d (round down)", tt.source, got, tt.want)
		}
	}
}
