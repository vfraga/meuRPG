package names

import "testing"

// Finding U1-08
func TestReview1_InvisibleChars(t *testing.T) {
	cases := map[string]string{
		"only ZWSP":        "​",
		"only ZWNJ ZWJ":    "‌‍",
		"only WJ":          "⁠",
		"only BOM":         "\uFEFF",
		"only hangul fill": "ㅤ",
		"U+2028 inside":    "ab cd",
		"U+2029 inside":    "ab cd",
	}
	for name, in := range cases {
		t.Run(name, func(t *testing.T) {
			got, err := Clean(in, 40)
			if err == nil {
				t.Errorf("Clean(%q) accepted, returned %q; want an error", in, got)
			}
		})
	}
	t.Run("NFC", func(t *testing.T) {
		got, err := Clean("é", 40)
		if err == nil && got != "é" {
			t.Errorf("Clean did not normalise to NFC: %q", got)
		}
	})
}
