package play

import (
	"slices"
	"testing"

	playv1 "github.com/PuraFome/meuRPG/backend/gen/meurpg/play/v1"
)

// The undo of the release of a held spell (the CastSpell of its readier's open window) gives back everything the release did
// and nothing else: the area it laid, its concentration as the held spell had it, the reaction, the row waiting with its window
// open and its wait clean, and the slots as they were (the release spends none, and the Ready's stay spent).

// heldCase is a spell held in a cave of its own, and how to release it and to count what the release laid.
type heldCase struct {
	name, spell string
	slot        int32
}

// fight opens the cave's fight with the readier on turn.
func (h heldCase) fight(t *testing.T) (*cave, *playv1.Encounter) {
	t.Helper()
	switch h.name {
	case "Web":
		c := zoneCave(t)
		e := c.passTo(t, c.zoneFight(t), "Pensantus")
		return c, e
	case "Wall of Fire":
		c := wallCave(t)
		e := c.passTo(t, c.zoneFight(t), "Pensantus")
		return c, e
	}
	c := newAreaCave(t)
	e := c.areaFight(t)
	return c, e
}

// release is the readier's CastSpell with the point (and the wall) the spell asks.
func (h heldCase) release(t *testing.T, c *cave) {
	t.Helper()
	switch h.name {
	case "Web":
		c.mustCastArea(t, c.ana, "Pensantus", webKey, nil, at(9, 7), nil)
	case "Wall of Fire":
		c.h.roller.queue(5, 5, 5, 5, 5, 5)
		c.mustCastArea(t, c.ana, "Pensantus", wallOfFireKey, nil, at(7, 8), nil, northWall(playv1.WallSide_WALL_SIDE_LEFT))
	default:
		c.mustCastArea(t, c.ana, "Pensantus", sleepSpell, nil, at(20, 7), nil, poolSum(20))
	}
}

// laid counts what the release leaves on the map: the zones of the spell, or the creatures it put to sleep.
func (h heldCase) laid(t *testing.T, c *cave) int {
	t.Helper()
	e := c.get(t, c.master)
	if h.name != "Sleep" {
		return len(zonesOf(e, h.spell))
	}
	n := 0
	for _, cb := range e.GetCombatants() {
		if slices.Contains(cb.GetConditions(), "condition:unconscious") {
			n++
		}
	}
	return n
}

// storedConcentration is the concentration the combatant's row holds, as the table keeps it: not what a read tells (a held spell
// reads as its key to its owner and the master).
func (a *armed) storedConcentration(t *testing.T, label string) string {
	t.Helper()
	var got *string
	if err := a.h.pool.QueryRow(t.Context(), `SELECT concentration_spell FROM combatants WHERE label = $1`, label).Scan(&got); err != nil {
		t.Fatalf("read the stored concentration of %s: %v", label, err)
	}
	if got == nil {
		return ""
	}
	return *got
}

func TestTheUndoOfAReleasedHeldSpellHoldsItAgain(t *testing.T) {
	t.Parallel()
	for _, tt := range []heldCase{{"Web", webKey, 2}, {"Wall of Fire", wallOfFireKey, 4}, {"Sleep", sleepSpell, 1}} {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			c, e := tt.fight(t)
			c.mustReady(t, c.ana, c.readySpell(t, e, "Pensantus", tt.spell, tt.slot, ""))
			e = c.mustEndTurn(t, c.master, e)
			w := c.mustFire(t, e, "Pensantus")
			if err := c.markRolling(t, c.ana, e, w.GetId()); err != nil { // the wait is on when the release comes
				t.Fatalf("the mark: %v", err)
			}
			held := byLabel(t, c.get(t, c.master), "Pensantus").GetConcentrationSpell()
			levels := []int32{1, 2, 3, 4, 5}
			if got := c.storedConcentration(t, "Pensantus"); got != concentrationReadied {
				t.Fatalf("the stored concentration while the spell is held = %q, want the sentinel %q", got, concentrationReadied)
			}
			slots, laid := c.slotsOf(t, "Pensantus", levels...), tt.laid(t, c)
			tt.release(t, c)
			if got := tt.laid(t, c); got <= laid || c.readiedStatus(t, "Pensantus") != readiedFired { // the check can fail: the release did lay something
				t.Fatalf("the release laid %d (was %d), row %q; want more and fired", got, laid, c.readiedStatus(t, "Pensantus"))
			}
			c.mustUndo(t, e)
			pens := byLabel(t, c.get(t, c.master), "Pensantus")
			win := c.windowOf(t, c.master, kindReadied)
			if got := tt.laid(t, c); got != laid {
				t.Errorf("after the undo %d of what the release laid is left, want %d", got, laid)
			}
			if pens.GetConcentrationSpell() != held || pens.GetReactionUsed() || c.readiedStatus(t, "Pensantus") != readiedWaiting || !slices.Equal(c.slotsOf(t, "Pensantus", levels...), slots) {
				t.Errorf("after the undo: concentration %q (want %q), reaction used %v, row %q, slots %v (want %v); want the held spell as it was",
					pens.GetConcentrationSpell(), held, pens.GetReactionUsed(), c.readiedStatus(t, "Pensantus"), c.slotsOf(t, "Pensantus", levels...), slots)
			}
			if got := c.storedConcentration(t, "Pensantus"); got != concentrationReadied {
				t.Errorf("the stored concentration after the undo = %q, want the sentinel %q", got, concentrationReadied)
			}
			if win == nil || win.GetReactorId() != pens.GetId() || win.GetRolling() || win.GetWaitBeganAt() != nil || win.GetWaitedMs() != 0 {
				t.Errorf("after the undo the READIED window reads %v; want it open with a clean wait", win)
			}
		})
	}
}
