package rules

import (
	"errors"
	"strings"
	"testing"
)

// firstViolation runs With and returns the Reason and Message of the first violation.
func firstViolation(t *testing.T, srd *Content, o Overlay) (reason, msg string) {
	t.Helper()
	_, err := srd.With(o)
	if err == nil {
		return "", ""
	}
	var oe *OverlayError
	if !errors.As(err, &oe) {
		t.Fatalf("error is not an *OverlayError: %v", err)
	}
	v := oe.Violations()
	t.Logf("violations: %d; first: field=%q reason=%q msg=%q", len(v), v[0].Field, v[0].Reason, v[0].Message)
	return v[0].Reason, v[0].Message
}

func overlayGenClass(t *testing.T, o *Overlay) *TableClass {
	t.Helper()
	for i := range o.Classes {
		if o.Classes[i].Key == "class:gen-none"+tableSuffix {
			return &o.Classes[i]
		}
	}
	t.Fatal("no class gen-none")
	return nil
}

// A name or a paragraph over its length limit must carry the documented
// ReasonName / ReasonText, not the generic ReasonLimit.
func TestEntryReasonOfALongNameOrParagraph(t *testing.T) {
	t.Parallel()
	srd := loadForTest(t)
	cases := []struct {
		name string
		edit func(o *Overlay, c *TableClass)
		want string
	}{
		{"class name of 81", func(_ *Overlay, c *TableClass) { c.NamePT = strings.Repeat("a", 81) }, ReasonName},
		{"feature name of 81", func(_ *Overlay, c *TableClass) {
			c.Levels[0].Features[0].NamePT = strings.Repeat("a", 81)
		}, ReasonName},
		{"feature paragraph of 4001", func(_ *Overlay, c *TableClass) {
			c.Levels[0].Features[0].DescPT = []string{strings.Repeat("a", 4001)}
		}, ReasonText},
		{"feature with too many paragraphs", func(_ *Overlay, c *TableClass) {
			c.Levels[0].Features[0].DescPT = make([]string, maxTextParagraphs+1)
		}, ReasonLimit},
		{"spell paragraph of 4001", func(o *Overlay, _ *TableClass) { o.Spells[0].DescPT = []string{strings.Repeat("a", 4001)} }, ReasonText},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			o := fullOverlay(t, srd)
			tc.edit(&o, overlayGenClass(t, &o))
			got, msg := firstViolation(t, srd, o)
			if got != tc.want {
				t.Errorf("Reason = %q (message %q), want %q", got, msg, tc.want)
			}
		})
	}
}

// A name is one line: line and paragraph separators and invisible format
// characters (text direction, zero width) are refused like control characters.
func TestEntryNameRefusesSeparatorsAndInvisibleCharacters(t *testing.T) {
	t.Parallel()
	srd := loadForTest(t)
	chars := []struct{ name, s string }{
		{"U+2028", "a\u2028b"},
		{"U+2029", "a\u2029b"},
		{"U+200B", "a\u200bb"},
		{"U+202E", "Espada\u202eodnoB"},
		{"U+2066", "a\u2066b"},
	}
	targets := []struct {
		name string
		set  func(o *Overlay, s string)
	}{
		{"class", func(o *Overlay, s string) { overlayGenClass(t, o).NamePT = s }},
		{"feature", func(o *Overlay, s string) { overlayGenClass(t, o).Levels[0].Features[0].NamePT = s }},
		{"spell", func(o *Overlay, s string) { o.Spells[0].NamePT = s }},
	}
	for _, ch := range chars {
		for _, tg := range targets {
			t.Run(ch.name+"/"+tg.name, func(t *testing.T) {
				t.Parallel()
				o := fullOverlay(t, srd)
				tg.set(&o, ch.s)
				if got, _ := firstViolation(t, srd, o); got != ReasonName {
					t.Errorf("%s name %q: Reason = %q, want %q", tg.name, ch.s, got, ReasonName)
				}
			})
		}
	}
	t.Run("a joiner in an emoji name is fine", func(t *testing.T) {
		t.Parallel()
		o := fullOverlay(t, srd)
		o.Spells[0].NamePT = "Família 👨‍👩‍👧"
		if got, msg := firstViolation(t, srd, o); got != "" {
			t.Errorf("Reason = %q (%q), want the name accepted", got, msg)
		}
	})
}
