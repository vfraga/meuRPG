package rules

import (
	"errors"
	"strings"
	"testing"
)

// reviewReasons runs With and returns the Reason and Message of the first violation.
func reviewReasons(t *testing.T, srd *Content, o Overlay) (reason, msg string) {
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

func reviewClass(t *testing.T, o *Overlay) *TableClass {
	t.Helper()
	for i := range o.Classes {
		if o.Classes[i].Key == "class:gen-none"+tableSuffix {
			return &o.Classes[i]
		}
	}
	t.Fatal("no class gen-none")
	return nil
}

// Finding U8-5 (see review/unit-08-rules-content.md).
// A name or a paragraph over its length limit must carry the documented
// ReasonName / ReasonText, not the generic ReasonLimit.
func TestReview08_ReasonCodeOfALongName(t *testing.T) {
	t.Parallel()
	srd := loadForTest(t)
	cases := []struct {
		name string
		edit func(t *testing.T, o *Overlay)
		want string
	}{
		{"class name of 81", func(t *testing.T, o *Overlay) { reviewClass(t, o).NamePT = strings.Repeat("a", 81) }, ReasonName},
		{"feature name of 81", func(t *testing.T, o *Overlay) {
			reviewClass(t, o).Levels[0].Features[0].NamePT = strings.Repeat("a", 81)
		}, ReasonName},
		{"feature paragraph of 4001", func(t *testing.T, o *Overlay) {
			reviewClass(t, o).Levels[0].Features[0].DescPT = []string{strings.Repeat("a", 4001)}
		}, ReasonText},
		{"spell paragraph of 4001", func(t *testing.T, o *Overlay) { o.Spells[0].DescPT = []string{strings.Repeat("a", 4001)} }, ReasonText},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			o := fullOverlay(t, srd)
			tc.edit(t, &o)
			got, msg := reviewReasons(t, srd, o)
			if got != tc.want {
				t.Errorf("Reason = %q (message %q), want %q", got, msg, tc.want)
			}
		})
	}
}

// Finding U8-6 (see review/unit-08-rules-content.md).
// A name is "one line": line/paragraph separators and invisible format
// characters (bidi overrides, zero-width) must be refused like control characters.
func TestReview08_NamesWithSeparatorsAndBidiControls(t *testing.T) {
	t.Parallel()
	srd := loadForTest(t)
	chars := []struct{ name, s string }{
		{"U+2028", "a b"},
		{"U+2029", "a b"},
		{"U+200B", "a​b"},
		{"U+202E", "Espada‮odnoB"},
		{"U+2066", "a⁦b"},
	}
	targets := []struct {
		name string
		set  func(t *testing.T, o *Overlay, s string)
	}{
		{"class", func(t *testing.T, o *Overlay, s string) { reviewClass(t, o).NamePT = s }},
		{"feature", func(t *testing.T, o *Overlay, s string) { reviewClass(t, o).Levels[0].Features[0].NamePT = s }},
		{"spell", func(t *testing.T, o *Overlay, s string) { o.Spells[0].NamePT = s }},
	}
	for _, ch := range chars {
		for _, tg := range targets {
			t.Run(ch.name+"/"+tg.name, func(t *testing.T) {
				o := fullOverlay(t, srd)
				tg.set(t, &o, ch.s)
				if _, err := srd.With(o); err == nil {
					t.Errorf("%s name %q was accepted", tg.name, ch.s)
				}
			})
		}
	}
	// What the sibling free-text checks allow (reported, not asserted).
	t.Run("report siblings", func(t *testing.T) {
		o := fullOverlay(t, srd)
		reviewClass(t, &o).Levels[0].Features[0].DescPT = []string{"a\x00b‮\n\tc"}
		_, err := srd.With(o)
		t.Logf("paragraph with NUL, U+202E, LF, TAB accepted: %v (err=%v)", err == nil, err)
		o2 := fullOverlay(t, srd)
		o2.Spells[0].CastingTime = TableCastingTime{Unit: CastReaction, Amount: 1, TriggerPT: "se a b‮"}
		_, err = srd.With(o2)
		t.Logf("reaction trigger with U+2028/U+202E accepted: %v (err=%v)", err == nil, err)
		o3 := fullOverlay(t, srd)
		o3.Spells[0].CastingTime = TableCastingTime{Unit: CastReaction, Amount: 1, TriggerPT: "se a\nb"}
		_, err = srd.With(o3)
		t.Logf("reaction trigger with LF accepted: %v (err=%v)", err == nil, err)
	})
}
