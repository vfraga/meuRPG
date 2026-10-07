package play

import (
	"testing"

	"connectrpc.com/connect"
)

// Review finding U3-14: undoing a feature action whose resource the sheet no longer has is an internal error that blocks the whole undo chain.
func TestReview3_UndoAfterTheSheetLostTheResourceIsNotAnInternalError(t *testing.T) {
	t.Parallel()
	a := newCasters(t)
	e := a.castersFight(t, 1)
	a.mustEndTurn(t, a.ana, e) // Toren's turn
	if _, err := a.feature(t, a.caio, e, "Toren", actionSurgeKey, noTakeRoll); err != nil {
		t.Fatalf("TakeAction(action surge) error = %v", err)
	}
	if _, err := a.h.pool.Exec(t.Context(),
		`UPDATE characters SET sheet = jsonb_set(sheet, '{full,classes,0,level}', '1') WHERE id = $1`, a.toren.GetId()); err != nil {
		t.Fatalf("lower the level: %v", err)
	}
	err := a.undo(t, a.master, e, a.log(t, a.master, e).GetUndoableEventId())
	if err != nil && connect.CodeOf(err) == connect.CodeInternal {
		t.Fatalf("UndoLastAction() = %v, want a handled outcome, not an internal error", err)
	}
}
