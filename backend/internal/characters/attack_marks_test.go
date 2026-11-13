package characters

import (
	"testing"

	"github.com/PuraFome/meuRPG/backend/internal/rules"
)

// An attack's marks (silvered, adamantine, magical) reach the wire as the engine derived them, from whichever source
// gave them: the proto fields are a copy for the screens, never a second rule.
func TestAttackProtoCarriesTheDerivedMarks(t *testing.T) {
	for _, tc := range []struct {
		name string
		in   rules.Attack
		want [3]bool // silvered, adamantine, magical
	}{
		{"plain steel", rules.Attack{Key: "equipment:longsword"}, [3]bool{}},
		{"silvered", rules.Attack{Key: "equipment:dagger", Silvered: true}, [3]bool{true, false, false}},
		{"adamantine and magical", rules.Attack{Key: "equipment:mace", Adamantine: true, Magical: true}, [3]bool{false, true, true}},
		{"all three", rules.Attack{Key: "equipment:shortsword", Silvered: true, Adamantine: true, Magical: true}, [3]bool{true, true, true}},
	} {
		got := attackToProto(tc.in)
		if [3]bool{got.GetSilvered(), got.GetAdamantine(), got.GetMagical()} != tc.want {
			t.Errorf("%s: got silvered=%v adamantine=%v magical=%v, want %v", tc.name, got.GetSilvered(), got.GetAdamantine(), got.GetMagical(), tc.want)
		}
	}
}
