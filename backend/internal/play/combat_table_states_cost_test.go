package play

import (
	"context"
	"runtime"
	"strings"
	"sync/atomic"
	"testing"

	"github.com/jackc/pgx/v5"

	playv1 "github.com/PuraFome/meuRPG/backend/gen/meurpg/play/v1"
)

// vitalsReads is the vitals keeper of the harness, counting the vitals that Service.statesNow reads
// inside a transaction: each one derives the character's whole sheet.
type vitalsReads struct {
	VitalsKeeper
	reads atomic.Int64
}

func (v *vitalsReads) GetVitalsTx(ctx context.Context, tx pgx.Tx, campaignID, characterID string) (*playv1.CharacterVitals, error) {
	if calledBy("(*Service).statesNow") {
		v.reads.Add(1)
	}
	return v.VitalsKeeper.GetVitalsTx(ctx, tx, campaignID, characterID)
}

// calledBy says a function whose name ends with name is on the stack.
func calledBy(name string) bool {
	pcs := make([]uintptr, 32)
	frames := runtime.CallersFrames(pcs[:runtime.Callers(2, pcs)])
	for {
		f, more := frames.Next()
		if strings.HasSuffix(f.Function, name) {
			return true
		}
		if !more {
			return false
		}
	}
}

// The states of a turn start (the clock, the ends) are read from the stored list before a character's sheet
// is derived: a character with no state stored costs no sheet to them.
func TestTheTurnStartDerivesNoSheetForACharacterWithoutStates(t *testing.T) {
	t.Parallel()
	r := newStanceRig(t, stanceEffects("10", "dismiss"))
	reads := &r.a.h.vitals.reads
	before := reads.Load()
	r.a.passTo(t, r.a.get(t, r.a.master), "Goblin")
	r.a.passTo(t, r.a.get(t, r.a.master), "Toren") // a new round: the turns of Toren, Pensantus and Brisa start
	if got := reads.Load() - before; got != 0 {
		t.Errorf("a round of turns with no state on had the states read %d times, want 0", got)
	}
	r.mustToggle(t, stanceName, false)
	before = reads.Load()
	r.a.passTo(t, r.a.get(t, r.a.master), "Goblin")
	r.a.passTo(t, r.a.get(t, r.a.master), "Toren")
	if got := reads.Load() - before; got == 0 {
		t.Error("a round of turns with the stance on had no states read: its rounds were not counted")
	}
	if v := r.a.vitals(t, r.a.toren); len(v.GetActiveStates()) != 1 || v.GetActiveStates()[0].GetRoundsLeft() != 9 {
		t.Errorf("states after a round = %v, want the stance with 9 rounds left", v.GetActiveStates())
	}
}

// Mage Armor's armor class, worked out outside the combat, counts the states that are on, as the sheet does.
func TestMageArmorArmorClassCountsTheStatesThatAreOn(t *testing.T) {
	t.Parallel()
	r := newStanceRig(t, stanceEffects("10", "dismiss"))
	mageArmorAC := func() int {
		t.Helper()
		tx, err := r.a.h.pool.Begin(t.Context())
		if err != nil {
			t.Fatalf("begin: %v", err)
		}
		defer func() { _ = tx.Rollback(t.Context()) }()
		got, err := r.a.h.chars.MageArmorAC(t.Context(), tx, r.a.campaignID, r.a.toren.GetId())
		if err != nil {
			t.Fatalf("MageArmorAC() error = %v", err)
		}
		if !got.Applies || got.Wears {
			t.Fatalf("MageArmorAC() = %+v, want it to apply to a character that wears no armor", got)
		}
		return got.AC
	}
	off := mageArmorAC()
	r.mustToggle(t, stanceName, false) // +2 AC while the stance is on
	if on := mageArmorAC(); on != off+2 {
		t.Errorf("Mage Armor AC with the stance on = %d, want %d (%d without it, +2)", on, off+2, off)
	}
}
