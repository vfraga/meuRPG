package play

import (
	"testing"
)

// Review finding U3-7: undoing a move made in the dark is told to every player (revision +1, encounter_changed and combat_log_changed).
func TestReview3_UndoOfAnUnseenMoveIsNotToldToPlayers(t *testing.T) {
	t.Parallel()
	f := newFogCave(t)
	f.fight(t)
	f.stage(t)
	// The Capitão goes two squares further into the dark: no player sees either square.
	f.mustMove(t, f.master, "Capitão Goblin", 17, 7)
	f.mustMove(t, f.master, "Capitão Goblin", 18, 7)

	players := map[string]*user{"Toren's player": f.caio, "Pensantus's player": f.ana, "Brisa's player": f.bia}
	before := map[string]int32{}
	for name, u := range players {
		before[name] = f.get(t, u).GetRevision()
	}
	s := f.watchAll(t)
	f.undoLast(t)
	for name, u := range players {
		if after := f.get(t, u).GetRevision(); after != before[name] {
			t.Errorf("%s: revision went %d -> %d after the undo of a move nobody saw, want unchanged", name, before[name], after)
		}
	}
	f.markEnd(t, 2)

	for name, w := range map[string]*watcher{"Toren's player": s.caio, "Pensantus's player": s.ana, "Brisa's player": s.bia} {
		if got := f.collect(t, w, 2); len(got) != 0 {
			t.Errorf("%s's stream got %v for the undo of a move in the dark, want nothing", name, kinds(got))
		}
	}
}
