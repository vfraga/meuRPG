package play

import (
	"github.com/PuraFome/meuRPG/backend/internal/play/mountsave"
	"testing"

	"github.com/PuraFome/meuRPG/backend/internal/authz"
	"github.com/PuraFome/meuRPG/backend/internal/play/playdb"
)

// A second push (or a prone) while the rider's save is open moves the same save on: one roll covers both, and the window does not close
// as a trigger gone because the mount is not where the first push left it. An open window holds every action, so the second push is
// one of the same change.

// pushTwice asks the save of the push of the mount, moves the mount and its rider two squares on, and asks it again, in one change.
// The first ask can be preceded by the prone of the rider.
func (c *cave) pushTwice(t *testing.T, proneFirst bool) {
	t.Helper()
	m := authz.Membership{CampaignID: c.campaignID, UserID: c.master.id, Role: authz.RoleMaster}
	encounterID, ogro := c.get(t, c.master).GetId(), c.id(t, "Ogro")
	toren := c.id(t, "Toren") // read before the transaction: the test pool has one connection
	_, err := c.h.svc.write(t.Context(), combatWrite{m: m, key: newKey(), kind: eventConditionsSet, encounterID: encounterID}, func(cx *combatTx) (any, error) {
		if proneFirst {
			if err := c.h.svc.riderProne(t.Context(), cx, toren); err != nil {
				return nil, err
			}
		}
		for range 2 {
			if _, err := cx.tx.Exec(t.Context(), `UPDATE combatants SET grid_col = grid_col + 2 WHERE id = $1 OR mounted_on_id = $1`, ogro); err != nil {
				return nil, err
			}
			if err := c.h.svc.mountPushed(t.Context(), cx, ogro); err != nil {
				return nil, err
			}
		}
		return actionEvent{Round: cx.enc.Round}, nil
	})
	if err != nil {
		t.Fatalf("push the mount twice: %v", err)
	}
}

func TestMountedCombat_ASecondPushKeepsTheRidersSave(t *testing.T) {
	t.Parallel()
	c := newAreaCave(t)
	c.mountedWave(t)
	c.pushTwice(t, false)
	wantSquare(t, c, "Ogro", [2]int32{22, 7})
	wantSave(t, "after the second push", c.mountSaveOf(t, c.master), c.id(t, "Toren"), c.id(t, "Ogro"), causePushed)
	if n := len(c.openSaveWindows(t)); n != 1 {
		t.Errorf("open mount_save windows = %d, want 1: one save covers both pushes", n)
	}
	if c.mountedOn(t, "Toren") == nil {
		t.Error("the rider was thrown with no roll")
	}
}

func TestMountedCombat_TheUndoOfTwoPushesTakesTheSaveWithThem(t *testing.T) {
	t.Parallel()
	c := newAreaCave(t)
	c.mountedWave(t)
	c.wave(t)
	if c.mountSaveOf(t, c.master) == nil {
		t.Fatal("the push asked no save")
	}
	c.mustUndo(t, c.get(t, c.master))
	wantSquare(t, c, "Ogro", [2]int32{18, 7})
	if got := c.mountSaveOf(t, c.master); got != nil {
		t.Errorf("the save %v is still asked after the undo", got)
	}
	c.pushTwice(t, false)
	c.execSQL(t, `UPDATE combatants SET grid_col = 18 WHERE id = $1 OR mounted_on_id = $1`, c.id(t, "Ogro"))
	c.touchCombat(t)
	if got := c.mountSaveOf(t, c.master); got != nil {
		t.Errorf("the save %v is still asked with the mount back where it was", got)
	}
}

// A push on the rider that is prone already keeps the save when it stands up: the push is the cause now.
func TestMountedCombat_APushAfterAProneKeepsTheRidersSave(t *testing.T) {
	t.Parallel()
	c := newAreaCave(t)
	c.mountedWave(t)
	c.execSQL(t, `UPDATE combatants SET conditions = ARRAY['condition:prone'] WHERE id = $1`, c.id(t, "Toren"))
	c.pushTwice(t, true)
	c.execSQL(t, `UPDATE combatants SET conditions = '{}' WHERE id = $1`, c.id(t, "Toren"))
	c.touchCombat(t)
	if c.mountSaveOf(t, c.master) == nil {
		t.Error("the save of the push was lost when the prone went away")
	}
}

// A prone after the push keeps the window of the push, and the later move of the mount moves its square on.
func TestMountedCombat_AProneAfterAPushKeepsTheRidersSave(t *testing.T) {
	t.Parallel()
	c := newAreaCave(t)
	c.mountedWave(t)
	c.pushTwice(t, false)
	c.execSQL(t, `UPDATE combatants SET conditions = ARRAY['condition:prone'] WHERE id = $1`, c.id(t, "Toren"))
	c.askSave(t, "Toren", causeProne)
	c.execSQL(t, `UPDATE combatants SET conditions = '{}' WHERE id = $1`, c.id(t, "Toren"))
	c.touchCombat(t)
	if c.mountSaveOf(t, c.master) == nil {
		t.Error("the save of the push was lost when the prone went away")
	}
}

// The mount put back on the square of the earlier push (the undo of the last) keeps the window; any other square closes it.
func TestMountedCombat_TheSquaresOfTheEarlierPushesKeepTheSave(t *testing.T) {
	t.Parallel()
	one, two, three := int32(20), int32(22), int32(24)
	rider := playdb.Combatant{ID: "r", MountedOnID: new("m")}
	w := playdb.ReactionWindow{ReactorID: new("r")}
	tr := windowTrigger{Target: "m", Cause: mountsave.Key(causePushed), AtCol: two, AtRow: 7, Before: [][2]int32{{one, 7}}}
	for col, gone := range map[int32]bool{one: false, two: false, three: true, 18: true} {
		mount := playdb.Combatant{ID: "m", GridCol: &col, GridRow: new(int32(7))}
		if got := mountSaveGone(w, tr, map[string]playdb.Combatant{"r": rider, "m": mount}); got != gone {
			t.Errorf("mountSaveGone with the mount at column %d = %v, want %v", col, got, gone)
		}
	}
}
