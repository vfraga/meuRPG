package archtest

import (
	"fmt"
	"slices"
	"strings"
	"testing"
)

// parseExceptions reads an exception list, one entry per line.
// Blank lines and lines that start with # are skipped.
// An entry may be followed by " # why".
// The entries come back in file order.
func parseExceptions(text string) []string {
	var out []string
	for line := range strings.Lines(text) {
		line = strings.TrimSpace(line)
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		if i := strings.Index(line, " #"); i >= 0 {
			line = strings.TrimSpace(line[:i])
		}
		out = append(out, line)
	}
	return out
}

// compareExceptions is the shared rule of the convention tests: an exception list can only shrink.
// The found map holds each offender's key and the message that says what to use instead.
// The listed slice holds the file's entries.
// An offender that the file does not list fails with its message.
// An entry that is no longer an offender fails with "remove it".
// It returns the problems, sorted.
func compareExceptions(file string, listed []string, found map[string]string) []string {
	var problems []string
	if !slices.IsSorted(listed) {
		problems = append(problems, fmt.Sprintf("%s is not sorted: keep its entries in sorted order", file))
	}
	for i := 1; i < len(listed); i++ {
		if listed[i] == listed[i-1] {
			problems = append(problems, fmt.Sprintf("%s lists %q twice: remove one", file, listed[i]))
		}
	}
	for key, msg := range found {
		if !slices.Contains(listed, key) {
			problems = append(problems, msg)
		}
	}
	for _, key := range listed {
		if _, ok := found[key]; !ok {
			problems = append(problems, fmt.Sprintf("%s lists %q, which is not an offender any more: remove it from %s", file, key, file))
		}
	}
	slices.Sort(problems)
	return problems
}

// failProblems reports every problem as a test error.
func failProblems(t *testing.T, problems []string) {
	t.Helper()
	for _, p := range problems {
		t.Error(p)
	}
}

func TestCompareExceptions(t *testing.T) {
	found := map[string]string{"a.go:F": "a.go:F is new", "b.go:G": "b.go:G is new"}
	cases := []struct {
		name   string
		listed []string
		want   []string
	}{
		{"exact", []string{"a.go:F", "b.go:G"}, nil},
		{"new offender", []string{"a.go:F"}, []string{"b.go:G is new"}},
		{"stale entry", []string{"a.go:F", "b.go:G", "c.go:H"}, []string{`x.txt lists "c.go:H", which is not an offender any more: remove it from x.txt`}},
		{"unsorted", []string{"b.go:G", "a.go:F"}, []string{"x.txt is not sorted: keep its entries in sorted order"}},
		{"duplicate", []string{"a.go:F", "a.go:F", "b.go:G"}, []string{`x.txt lists "a.go:F" twice: remove one`}},
	}
	for _, c := range cases {
		if got := compareExceptions("x.txt", c.listed, found); !slices.Equal(got, c.want) {
			t.Errorf("%s: got %q, want %q", c.name, got, c.want)
		}
	}
	if got := parseExceptions("# c\n\nb # why\na\n"); !slices.Equal(got, []string{"b", "a"}) {
		t.Errorf("parseExceptions = %q", got)
	}
}
