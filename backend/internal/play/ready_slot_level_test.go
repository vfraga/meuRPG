package play

import (
	"testing"

	playv1 "github.com/PuraFome/meuRPG/backend/gen/meurpg/play/v1"
)

// A held spell's row carries the slot level it was held with, to the readier's player and the master; a held cantrip
// carries 0, and another player reads no row.
func TestTheReadiedRowCarriesTheSlotLevelOfTheHeldSpell(t *testing.T) {
	t.Parallel()
	a := newCasters(t)
	e := a.castersFight(t, 1)
	a.mustReady(t, a.ana, a.readySpell(t, e, "Pensantus", magicMissileSpell, 2, ""))
	for who, u := range map[string]*user{"the readier's player": a.ana, "the master": a.master} {
		if got := byLabel(t, a.get(t, u), "Pensantus").GetReadied().GetSlotLevel(); got != 2 {
			t.Errorf("%s reads slot_level %d, want 2", who, got)
		}
	}
	if got := byLabel(t, a.get(t, a.caio), "Pensantus").GetReadied(); got != nil {
		t.Errorf("another player reads the row %v", got)
	}
	e = a.passTo(t, e, "Brisa")
	a.mustReady(t, a.bia, a.readySpell(t, e, "Brisa", sacredFlame, 0, "Goblin"))
	if row := byLabel(t, a.get(t, a.bia), "Brisa").GetReadied(); row.GetKind() != playv1.ReadiedKind_READIED_KIND_SPELL || row.GetSlotLevel() != 0 {
		t.Errorf("a held cantrip's row = %v, want a spell with slot_level 0", row)
	}
}
