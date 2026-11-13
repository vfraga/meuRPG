package play

import (
	"slices"
	"testing"

	"connectrpc.com/connect"

	playv1 "github.com/PuraFome/meuRPG/backend/gen/meurpg/play/v1"
	"github.com/PuraFome/meuRPG/backend/internal/platform/dbtest"
)

// Undoing a swap of gear in a combat: the swap is an event of its own, so the master's
// "undo the last action" takes it back first, with the economy and the sheet's equipped state
// from before it, and only then the action under it.

// lastEventID is the id the master's undo expects.
func (a *armed) lastEventID(t *testing.T, e *playv1.Encounter) string {
	t.Helper()
	id := a.log(t, a.master, e).GetUndoableEventId()
	if id == "" {
		t.Fatal("nothing to undo, want an event")
	}
	return id
}

func (a *armed) mustUndo(t *testing.T, e *playv1.Encounter) {
	t.Helper()
	if err := a.undo(t, a.master, e, a.lastEventID(t, e)); err != nil {
		t.Fatalf("UndoLastAction() error = %v", err)
	}
}

func (a *armed) hasCarried(t *testing.T, key string) bool {
	t.Helper()
	return slices.Contains(a.carriedOf(t, a.pens), key)
}

// attack, swap, undo, undo: the swap goes first and gives back the free interaction and the
// piece; the second undo takes the attack and its action.
func TestUndoTakesTheSwapBeforeTheActionUnderIt(t *testing.T) {
	t.Parallel()
	a, e := equipFight(t)
	e = a.passTo(t, e, "Reator")
	a.mustAttack(t, a.ana, e, "Reator", longswordKey, "Goblin", d20(15))
	a.wantTurn(t, "after the attack", false, true)
	a.mustSwap(t, a.ana, a.pens, daggerKey, false)
	a.wantTurn(t, "after the swap", true, true)
	if !a.hasCarried(t, daggerKey) {
		t.Fatal("the dagger is not stowed after the swap")
	}

	a.mustUndo(t, e)
	a.wantTurn(t, "after undoing the swap", false, true)
	if a.hasCarried(t, daggerKey) {
		t.Error("the dagger is still stowed after undoing the swap")
	}

	a.mustUndo(t, e)
	a.wantTurn(t, "after undoing the attack", false, false)
}

// Undoing an earlier event is not possible past the swap: the swap is undone first, and it
// does not give back what it spent when the attack under it is undone.
func TestUndoAnEarlierEventKeepsWhatTheSwapSpentUntilTheSwapIsUndone(t *testing.T) {
	t.Parallel()
	a, e := equipFight(t)
	e = a.passTo(t, e, "Reator")
	a.mustAttack(t, a.ana, e, "Reator", longswordKey, "Goblin", d20(15))
	first := a.lastEventID(t, e)
	a.mustSwap(t, a.ana, a.pens, daggerKey, false)
	if err := a.undo(t, a.master, e, first); err == nil {
		t.Fatal("UndoLastAction(the attack under the swap) succeeded, want it refused")
	}
	a.wantTurn(t, "after the refused undo", true, true)
	if !a.hasCarried(t, daggerKey) {
		t.Error("a refused undo gave the dagger back")
	}
}

// A second swap costs the action: undoing it gives the action back and leaves the first swap.
func TestUndoTheSecondSwapGivesTheActionBack(t *testing.T) {
	t.Parallel()
	a, e := equipFight(t)
	e = a.passTo(t, e, "Reator")
	a.mustSwap(t, a.ana, a.pens, longswordKey, false)
	a.mustSwap(t, a.ana, a.pens, daggerKey, false)
	a.wantTurn(t, "after two swaps", true, true)
	a.mustUndo(t, e)
	a.wantTurn(t, "after undoing the second", true, false)
	if a.hasCarried(t, daggerKey) || !a.hasCarried(t, longswordKey) {
		t.Errorf("carried = %v, want the longsword only", a.carriedOf(t, a.pens))
	}
	a.mustUndo(t, e)
	a.wantTurn(t, "after undoing the first", false, false)
	if got := a.carriedOf(t, a.pens); len(got) != 0 {
		t.Errorf("carried = %v, want nothing stowed", got)
	}
}

// The shield is the action: undoing its doffing gives the action and the shield back.
func TestUndoTheShieldGivesTheActionAndTheShieldBack(t *testing.T) {
	t.Parallel()
	a, e := equipFight(t)
	e = a.passTo(t, e, "Reator")
	ac := func() int32 { return byLabel(t, a.get(t, a.master), "Reator").GetArmorClass() }
	if got := ac(); got != 18 {
		t.Fatalf("armor class with the shield = %d, want 18", got)
	}
	a.mustSwap(t, a.ana, a.pens, shieldItem, false)
	a.wantTurn(t, "after doffing", false, true)
	if got := ac(); got != 16 {
		t.Fatalf("armor class without the shield = %d, want 16", got)
	}
	a.mustUndo(t, e)
	a.wantTurn(t, "after the undo", false, false)
	if got := ac(); got != 18 {
		t.Errorf("armor class after the undo = %d, want 18: the derived gear is read again", got)
	}
	if a.hasCarried(t, shieldItem) {
		t.Error("the shield is still stowed")
	}
	// the action is back: the shield can be doffed again.
	a.mustSwap(t, a.ana, a.pens, shieldItem, false)
}

// Haste's extra action paid for the swap: undoing gives it back, not the action.
func TestUndoTheSwapPaidByTheExtraActionGivesItBack(t *testing.T) {
	t.Parallel()
	a, e := equipFight(t)
	e = a.passTo(t, e, "Reator")
	a.mustAddEffect(t, e, "spell:haste", []string{"Reator"}, a.rounds(t, 3, "Reator"))
	a.mustSwap(t, a.ana, a.pens, shieldItem, false)
	a.mustSwap(t, a.ana, a.pens, longswordKey, false)
	a.mustSwap(t, a.ana, a.pens, daggerKey, false)
	a.mustUndo(t, e)
	var extra bool
	if err := a.h.pool.QueryRow(t.Context(), `SELECT extra_action_used FROM combatants WHERE id = $1`, a.id(t, "Reator")).Scan(&extra); err != nil || extra {
		t.Fatalf("extra action used = %v (%v), want it given back", extra, err)
	}
	a.wantTurn(t, "after the undo", true, true)
}

// The master's own swap enters the undo too.
func TestUndoTheMastersSwap(t *testing.T) {
	t.Parallel()
	a, e := equipFight(t)
	e = a.passTo(t, e, "Reator")
	a.mustSwap(t, a.master, a.pens, shieldItem, false)
	a.mustUndo(t, e)
	if a.hasCarried(t, shieldItem) {
		t.Error("the shield is still stowed")
	}
}

// The sheet no longer lists the piece the swap touched: the undo is refused and writes nothing.
func TestUndoIsRefusedWhenTheSheetLostThePiece(t *testing.T) {
	t.Parallel()
	a, e := equipFight(t)
	e = a.passTo(t, e, "Reator")
	a.mustSwap(t, a.ana, a.pens, daggerKey, false)
	// the dagger leaves the sheet: the owner edits the weapons list.
	if _, err := a.h.pool.Exec(t.Context(), `UPDATE characters SET sheet = jsonb_set(sheet, '{full,weapon_keys}', (sheet #> '{full,weapon_keys}') - 1) WHERE id = $1 AND sheet #>> '{full,weapon_keys,1}' = $2`, a.pens.GetId(), daggerKey); err != nil {
		t.Fatalf("remove the dagger from the sheet: %v", err)
	}
	before := a.carriedOf(t, a.pens)
	err := a.undo(t, a.master, e, a.lastEventID(t, e))
	if connect.CodeOf(err) != connect.CodeFailedPrecondition {
		t.Fatalf("UndoLastAction() error = %v, want failed precondition", err)
	}
	a.wantTurn(t, "after the refused undo", true, false)
	if got := a.carriedOf(t, a.pens); !slices.Equal(got, before) {
		t.Errorf("a refused undo changed the sheet: %v -> %v", before, got)
	}
}

// The encounter revision moves with the undo, so the web rereads the gear.
func TestUndoTheSwapBumpsTheEncounterRevision(t *testing.T) {
	t.Parallel()
	a, e := equipFight(t)
	e = a.passTo(t, e, "Reator")
	a.mustSwap(t, a.ana, a.pens, daggerKey, false)
	rev := func() int64 {
		var r int64
		if err := a.h.pool.QueryRow(t.Context(), `SELECT revision FROM encounters WHERE id = $1`, e.GetId()).Scan(&r); err != nil {
			t.Fatalf("read the revision: %v", err)
		}
		return r
	}
	before := rev()
	a.mustUndo(t, e)
	if after := rev(); after <= before {
		t.Errorf("revision %d -> %d, want it to move", before, after)
	}
}

// The undo of a swap and a SetEquipped of the same character cross in the database: the undo takes the
// session and then the character (RestoreEquipped), SetEquipped the character and then the session. Whichever
// the database aborts (40P01 on PostgreSQL, 40001 on CockroachDB), db.InTx runs it again, so nothing reaches the
// caller. Both are the real calls, run at the same moment many times; after each pair the result is one of the two
// serial orders and no other: SetEquipped always succeeds, and the undo either ran first (it applied once: the
// dagger is back, and the master's change came after it) or came second and was refused for what it is, a stale
// "last action" (aborted, nothing written). Nothing reads a PostgreSQL view, so it runs on CockroachDB too.
func TestUndoOfASwapCrossingASetEquippedTakesTurns(t *testing.T) {
	t.Parallel()
	dbtest.PoolSize(t, 2) // one connection would run the two calls one after the other and prove nothing
	a, e := equipFight(t)
	e = a.passTo(t, e, "Reator")
	// A piece is put to the other state by asking for the state it is not in.
	toggle := func(key string) error {
		_, err := a.swap(a.master, a.pens, key, a.hasCarried(t, key))
		return err
	}
	undone, refused := 0, 0
	for round := range 60 {
		daggerBefore, swordBefore := a.hasCarried(t, daggerKey), a.hasCarried(t, longswordKey)
		if err := toggle(daggerKey); err != nil {
			t.Fatalf("round %d: the swap error = %v", round, err)
		}
		id := a.lastEventID(t, e)
		start := dbtest.NewBarrier(2)
		undoErr, equipErr := make(chan error, 1), make(chan error, 1)
		go func() { start.Wait(); undoErr <- a.undo(t, a.master, e, id) }()
		go func() { start.Wait(); equipErr <- toggle(longswordKey) }()
		uerr, eerr := <-undoErr, <-equipErr
		if eerr != nil {
			t.Fatalf("round %d: SetEquipped error = %v, want none", round, eerr)
		}
		daggerAfter, swordAfter := a.hasCarried(t, daggerKey), a.hasCarried(t, longswordKey)
		switch {
		case uerr == nil:
			undone++
			if daggerAfter != daggerBefore {
				t.Fatalf("round %d: the undo succeeded and the dagger is not back (carried %v, was %v)", round, daggerAfter, daggerBefore)
			}
		case connect.CodeOf(uerr) == connect.CodeAborted:
			refused++
			if daggerAfter == daggerBefore {
				t.Fatalf("round %d: the undo was refused and the swap is gone (carried %v)", round, daggerAfter)
			}
		default:
			t.Fatalf("round %d: UndoLastAction() error = %v, want it applied or refused as stale", round, uerr)
		}
		if swordAfter == swordBefore {
			t.Fatalf("round %d: the master's change to the longsword was lost (carried %v)", round, swordAfter)
		}
		a.wantTurn(t, "after the pair", false, false)
		// Back to where the round began: the next one starts from the same sheet.
		if swordAfter != swordBefore {
			if err := toggle(longswordKey); err != nil {
				t.Fatalf("round %d: put the longsword back: %v", round, err)
			}
		}
		if daggerAfter != daggerBefore {
			if err := toggle(daggerKey); err != nil {
				t.Fatalf("round %d: put the dagger back: %v", round, err)
			}
		}
	}
	t.Logf("%d undos applied, %d refused as stale", undone, refused)
}

func gearLine(res *playv1.ListCombatLogResponse) *playv1.CombatLogEntry {
	for _, r := range res.GetRounds() {
		for _, en := range r.GetEntries() {
			if en.GetKind() == playv1.CombatLogKind_COMBAT_LOG_KIND_GEAR_SWAPPED {
				return en
			}
		}
	}
	return nil
}

// The line of a swap in the combat log: the master and the owner read the piece, the way it went and
// what it cost; a player who does not own the character reads nothing. A master's swap costs nothing
// and the line says the master made it.
func TestTheLogHasTheSwapForTheMasterAndTheOwnerAlone(t *testing.T) {
	t.Parallel()
	a, e := equipFight(t)
	e = a.passTo(t, e, "Reator")
	a.mustSwap(t, a.ana, a.pens, longswordKey, false)
	a.mustSwap(t, a.ana, a.pens, daggerKey, false)
	for i, u := range []*user{a.master, a.ana} {
		line := gearLine(a.log(t, u, e))
		if line == nil {
			t.Fatalf("a gear line is missing from the log of reader %d", i)
		}
		g := line.GetGearSwapped()
		if g.GetNamePt() == "" || g.GetKind() != "weapon" || g.GetEquipped() || g.GetCost() != "action" {
			t.Errorf("reader %d reads %v, want the dagger stowed for the action", i, g)
		}
	}
	if line := gearLine(a.log(t, a.bia, e)); line != nil {
		t.Errorf("another player reads %v", line)
	}
	a.mustUndo(t, e)
	for i, u := range []*user{a.master, a.ana} {
		line := gearLine(a.log(t, u, e))
		if line == nil || line.GetGearSwapped().GetCost() != "free" {
			t.Errorf("reader %d reads %v after undoing the second, want the first (free)", i, line)
		}
	}
	a.mustUndo(t, e)
	if line := gearLine(a.log(t, a.master, e)); line != nil {
		t.Errorf("the master reads %v after undoing both", line)
	}

	a.mustSwap(t, a.master, a.pens, shieldItem, false)
	line := gearLine(a.log(t, a.ana, e))
	if line == nil || line.GetGearSwapped().GetCost() != "" || !line.GetByMaster() {
		t.Errorf("the owner reads %v for the master's swap, want a free line by the master", line)
	}
}

// A player's armor swap is refused in a combat; the master's is allowed and enters the undo: the undo puts the armor
// back on the sheet and the armor class with it, and the line says "armor".
func TestUndoTheMastersArmorSwap(t *testing.T) {
	t.Parallel()
	a, e := equipFight(t)
	e = a.passTo(t, e, "Reator")
	ac := func() int32 { return byLabel(t, a.get(t, a.master), "Reator").GetArmorClass() }
	if got := ac(); got != 18 {
		t.Fatalf("armor class in chain mail with the shield = %d, want 18", got)
	}
	a.mustSwap(t, a.master, a.pens, chainMailKey, false)
	if got := ac(); got != 14 {
		t.Fatalf("armor class with the armor stowed = %d, want 14", got)
	}
	if line := gearLine(a.log(t, a.master, e)); line == nil || line.GetGearSwapped().GetKind() != "armor" {
		t.Errorf("the master reads %v, want a gear line of kind armor", line)
	}
	a.mustUndo(t, e)
	if a.hasCarried(t, chainMailKey) {
		t.Error("the armor is still stowed after the undo")
	}
	if got := ac(); got != 18 {
		t.Errorf("armor class after the undo = %d, want 18", got)
	}
	a.wantTurn(t, "after the undo", false, false)
}
