package play

import (
	"testing"
)

// Review finding U3-6: moving a master-hidden NPC inside a player's sight still sends that player vision_changed and raises their revision.
func TestReview3_HiddenNPCMoveInSightIsNotToldToPlayers(t *testing.T) {
	t.Parallel()
	f := newFogCave(t)
	f.fight(t)
	f.stage(t)
	f.hide(t, "Goblin 2")
	for _, sq := range [][2]int{{8, 8}, {9, 8}} {
		if !f.seesSquare(t, f.caio, sq[0], sq[1]) {
			t.Fatalf("precondition: Toren's player does not see %v", sq)
		}
	}
	before := f.get(t, f.caio).GetRevision()
	s := f.watchAll(t)
	f.mustMove(t, f.master, "Goblin 2", 9, 8)
	f.markEnd(t, 3)

	t.Run("stream", func(t *testing.T) {
		if got := f.collect(t, s.caio, 3); len(got) != 0 {
			t.Errorf("Toren's player got %v for a hidden NPC's move, want nothing", kinds(got))
		}
	})
	t.Run("revision", func(t *testing.T) {
		// Brisa's marker move is public, so it adds exactly one visible event.
		if after := f.get(t, f.caio).GetRevision(); after != before+1 {
			t.Errorf("Toren's player revision = %d, want %d (the hidden move must not count)", after, before+1)
		}
	})
}
