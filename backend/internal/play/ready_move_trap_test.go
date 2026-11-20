package play

import (
	"testing"
)

// A readied move that walks into a trap writes the move and, after it, the trap's firing. Two undos take them back, the
// firing first and then the move, and the second puts the use back: the row waits, its window is open, the reaction is free.
func TestAReadiedMoveThatWalksIntoATrapIsUndoneInTwoSteps(t *testing.T) {
	t.Parallel()
	r := newTrapRig(t)
	r.trap(t, "Fosso Dourado", 9, 7, pit("1d6"))
	e := r.fight(t)
	r.mustReady(t, r.caio, r.readyMoveRequest(t, e))
	e = r.mustEndTurn(t, r.master, e)
	r.mustFire(t, e, "Toren")

	r.h.roller.queue(4)
	if res, err := r.moveResult(t, r.caio, "Toren", 12, 7); err != nil || !res.GetStoppedEarly() {
		t.Fatalf("the readied move into the trap = %v, %v; want it stopped on the trap", res, err)
	}
	if n := r.eventCount(t, "trap_triggered"); n != 1 || r.readiedStatus(t, "Toren") != readiedFired || r.readiedOpen(t, "Toren") {
		t.Fatalf("after the move: trap firings %d, row %q, window open %v", n, r.readiedStatus(t, "Toren"), r.readiedOpen(t, "Toren"))
	}

	r.undoLast(t) // the firing
	if got := r.who(t, r.caio, "Toren"); got.GetCol() != 9 || r.readiedStatus(t, "Toren") != readiedFired {
		t.Errorf("after the first undo: Toren at (%d, %d), row %q; want the move kept on (9, 7) and used", got.GetCol(), got.GetRow(), r.readiedStatus(t, "Toren"))
	}
	r.undoLast(t) // the move, and the use with it
	if got := r.who(t, r.caio, "Toren"); got.GetCol() != 6 || got.GetRow() != 7 || got.GetMovementUsedDft() != 0 {
		t.Errorf("after the second undo Toren is %v, want back on (6, 7) with nothing walked", got)
	}
	r.untouched(t, e, "the undo of the trap's firing and of the move")
}
