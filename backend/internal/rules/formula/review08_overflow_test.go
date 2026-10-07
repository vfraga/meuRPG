// Finding U8-4 (see review/unit-08-rules-content.md).
package formula

import (
	"strings"
	"testing"
)

// TestReview08_IntegerOverflowEscapesMaxResult: 1000 = 2^3*5^3, so 22 factors
// of 1000 wrap to 0 in 64-bit int arithmetic and slip under MaxResult.
func TestReview08_IntegerOverflowEscapesMaxResult(t *testing.T) {
	t.Parallel()
	product := strings.TrimSuffix(strings.Repeat("1000*", 22), "*")
	c := newCompiler()
	for _, source := range []string{
		product,
		product + " + 5",
		product + " + level()",
	} {
		p, err := c.Compile(source, Int)
		if err != nil {
			continue // refused at compile time: fine
		}
		n, err := p.Int(wizard3())
		if err == nil {
			t.Errorf("%q (len %d) returned %d with no error; an overflowing product must be refused",
				source[len(source)-12:], len(source), n)
		}
	}
}
