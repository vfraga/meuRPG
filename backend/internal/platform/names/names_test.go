package names

import (
	"errors"
	"strings"
	"testing"
)

func TestClean(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name    string
		input   string
		want    string
		wantErr bool
	}{
		{"plain", "Mirathel", "Mirathel", false},
		{"trims spaces at both ends", "  Mirathel \u00a0", "Mirathel", false},
		{"keeps inner spaces", "A Maldição de Strahd", "A Maldição de Strahd", false},
		{"emoji", "Mesa de quinta 🐉", "Mesa de quinta 🐉", false},
		{"exactly the limit", strings.Repeat("é", 20), strings.Repeat("é", 20), false},
		{"empty", "", "", true},
		{"only spaces", " \t\n ", "", true},
		{"one character over", strings.Repeat("é", 21), "", true},
		{"line break", "Mira\nthel", "", true},
		{"tab", "Mira\tthel", "", true},
		{"NUL", "Mira\x00thel", "", true},
		{"right-to-left override", "Mira\u202ethel", "", true},
		{"invalid UTF-8", "Mira\xffthel", "", true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			got, err := Clean(tt.input, 20)
			if (err != nil) != tt.wantErr {
				t.Fatalf("Clean(%q) error = %v, wantErr %v", tt.input, err, tt.wantErr)
			}
			if got != tt.want {
				t.Errorf("Clean(%q) = %q, want %q", tt.input, got, tt.want)
			}
		})
	}

	if _, err := Clean("   ", 10); !errors.Is(err, ErrEmpty) {
		t.Errorf("Clean(spaces) error = %v, want ErrEmpty", err)
	}
}

func TestCleanText(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name    string
		input   string
		want    string
		wantErr bool
	}{
		{"empty is fine", "", "", false},
		{"only spaces become empty", " \t\n ", "", false},
		{"keeps line breaks and tabs inside", "Nasceu em\n\tMirathel.", "Nasceu em\n\tMirathel.", false},
		{"trims blank lines at both ends", "\n\nNasceu.\n\n", "Nasceu.", false},
		{"Windows line breaks", "linha 1\r\nlinha 2", "linha 1\nlinha 2", false},
		{"old Mac line breaks", "linha 1\rlinha 2", "linha 1\nlinha 2", false},
		{"exactly the limit", strings.Repeat("é", 20), strings.Repeat("é", 20), false},
		{"one character over", strings.Repeat("é", 21), "", true},
		{"NUL", "Mira\x00thel", "", true},
		{"escape", "Mira\x1bthel", "", true},
		{"right-to-left override", "Mira\u202ethel", "", true},
		{"invalid UTF-8", "Mira\xffthel", "", true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			got, err := CleanText(tt.input, 20)
			if (err != nil) != tt.wantErr {
				t.Fatalf("CleanText(%q) error = %v, wantErr %v", tt.input, err, tt.wantErr)
			}
			if got != tt.want {
				t.Errorf("CleanText(%q) = %q, want %q", tt.input, got, tt.want)
			}
		})
	}
}

func TestCleanRefusesInvisibleCharacters(t *testing.T) {
	t.Parallel()
	cases := map[string]string{
		"only ZWSP":        "\u200b",
		"only ZWNJ ZWJ":    "\u200c\u200d",
		"only WJ":          "\u2060",
		"only BOM":         "\uFEFF",
		"only hangul fill": "\u3164",
		"ZWSP inside":      "ab\u200bcd",
		"U+2028 inside":    "ab\u2028cd",
		"U+2029 inside":    "ab\u2029cd",
	}
	for name, in := range cases {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			if got, err := Clean(in, 40); err == nil {
				t.Errorf("Clean(%q) accepted, returned %q; want an error", in, got)
			}
			if got, err := CleanText(in, 40); err == nil && strings.ContainsAny(got, "\u2028\u2029\u200b\u2060\uFEFF\u3164") {
				t.Errorf("CleanText(%q) kept an invisible character: %q", in, got)
			}
		})
	}
	t.Run("only invisible is no visible character", func(t *testing.T) {
		t.Parallel()
		if _, err := Clean("\u200d\u200c", 40); !errors.Is(err, ErrNoVisible) {
			t.Errorf("error = %v, want ErrNoVisible", err)
		}
	})
	t.Run("emoji sequence with a joiner is kept", func(t *testing.T) {
		t.Parallel()
		in := "Família 👨\u200d👩\u200d👧"
		if got, err := Clean(in, 40); err != nil || got != in {
			t.Errorf("Clean(%q) = %q, %v; want it kept", in, got, err)
		}
	})
}

func TestCleanNormalizesToNFC(t *testing.T) {
	t.Parallel()
	const decomposed, composed = "e\u0301", "é"
	if got, err := Clean(decomposed, 40); err != nil || got != composed {
		t.Errorf("Clean(decomposed) = %q, %v; want %q", got, err, composed)
	}
	if got, err := CleanText("a\n"+decomposed, 40); err != nil || got != "a\n"+composed {
		t.Errorf("CleanText(decomposed) = %q, %v; want NFC", got, err)
	}
	// The limit counts the normalized text, the same unit the database checks.
	if _, err := Clean(strings.Repeat(decomposed, 20), 20); err != nil {
		t.Errorf("20 composed characters refused by a limit of 20: %v", err)
	}
}
