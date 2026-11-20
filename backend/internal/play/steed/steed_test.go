package steed

import (
	"testing"

	"github.com/PuraFome/meuRPG/backend/internal/rules"
)

func TestTooSimple(t *testing.T) {
	t.Parallel()
	intScore := func(base int) []rules.AbilityScore {
		return []rules.AbilityScore{{Ability: rules.STR, Base: 3}, {Ability: rules.INT, Base: base}}
	}
	for _, tc := range []struct {
		name      string
		score     int
		abilities []rules.AbilityScore
		want      bool
	}{
		{"the page says 2", 0, intScore(2), true},
		{"the page says 4", 0, intScore(4), true},
		{"the page says 5", 0, intScore(5), false},
		{"a steed's floor of 6 beats the page's 2", 6, intScore(2), false},
		{"a known score of 3", 3, intScore(10), true},
		{"no score anywhere", 0, nil, false},
	} {
		if got := TooSimple(tc.score, tc.abilities); got != tc.want {
			t.Errorf("%s: TooSimple() = %v, want %v", tc.name, got, tc.want)
		}
	}
}

func TestNoteFor(t *testing.T) {
	t.Parallel()
	if NoteFor(true) != TargetNote || NoteFor(false) != TableNote || TargetNote == TableNote {
		t.Error("the note of a cast does not follow whether the effect reached the steed")
	}
	// The app writes "o mestre" in lower case in a sentence.
	if want := "O corcel também é alvo da magia: o mestre aplica o efeito a ele."; NoteFor(false) != want {
		t.Errorf("NoteFor(false) = %q, want %q", NoteFor(false), want)
	}
}
