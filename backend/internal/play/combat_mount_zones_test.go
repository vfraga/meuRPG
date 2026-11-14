package play

import (
	"testing"

	mapsv1 "github.com/PuraFome/meuRPG/backend/gen/meurpg/maps/v1"
)

// What a rider meets when its mount moves, and when it mounts or dismounts, in the areas spells keep on the map (zones_hooks.go) and
// in the traps of the map: the rider is a creature of the square like any other, so it is caught with its mount. The fixture is the
// strip of zones_test.go: Toren casts Spirit Guardians at (5,7), a sphere of 15 ft that reaches column 8; the Ogro and the Goblin are
// the enemies, the Goblin the rider.

// TestMountedCombat_ARiderIsCaughtByTheAreaItsMountWalksInto: the mount walks into the aura of Spirit Guardians and its rider, who
// came along, takes the damage as the mount does.
func TestMountedCombat_ARiderIsCaughtByTheAreaItsMountWalksInto(t *testing.T) {
	t.Parallel()
	c := zoneCave(t)
	e := c.zoneFight(t)
	c.mustMove(t, c.master, "Ogro", 14, 7) // out of the area first
	c.sitOn(t, "Goblin", "Ogro")
	c.castSpiritGuardians(t, 3)
	c.passTo(t, e, "Ogro")
	c.execSQL(t, `UPDATE combatants SET speed_ft = 200 WHERE id = $1`, c.id(t, "Ogro"))
	c.h.roller.queue(5, 4, 4, 4, 5, 4, 4, 4)
	if _, err := c.move(t, c.master, "Ogro", 8, 7); err != nil {
		t.Fatalf("MoveCombatant() error = %v", err)
	}
	if cur, _, _ := c.hp(t, "Ogro"); cur >= 59 {
		t.Fatalf("the positive control failed: the ogre's hit points = %d, want it hit by the aura", cur)
	}
	if cur, _, _ := c.hp(t, "Goblin"); cur >= 7 {
		t.Errorf("the rider's hit points = %d, want it hit by the aura its mount walked into", cur)
	}
}

// TestMountedCombat_ADismountIntoAnAreaCatchesTheRider: the rider that gets off onto a square of the aura enters it, and the mount it
// leaves, which stays outside, is not hit.
func TestMountedCombat_ADismountIntoAnAreaCatchesTheRider(t *testing.T) {
	t.Parallel()
	c := zoneCave(t)
	e := c.zoneFight(t)
	c.mustMove(t, c.master, "Ogro", 9, 7) // outside the aura, which ends on column 8
	c.sitOn(t, "Goblin", "Ogro")
	c.castSpiritGuardians(t, 3)
	c.passTo(t, e, "Goblin")
	c.h.roller.queue(5, 4, 4, 4)
	if _, err := c.dismountAs(t, c.master, "Goblin", 8, 7); err != nil {
		t.Fatalf("DismountCombatant() error = %v", err)
	}
	if cur, _, _ := c.hp(t, "Goblin"); cur >= 7 {
		t.Errorf("the rider's hit points = %d, want it hit by the aura it got off into", cur)
	}
	if cur, _, _ := c.hp(t, "Ogro"); cur != 59 {
		t.Errorf("the mount's hit points = %d, want 59: it stayed outside", cur)
	}
}

// TestMountedCombat_ADismountOntoATrapFiresIt: a player's rider that gets off onto the square of an armed trap fires it, as a step
// into it does, and the undo takes the firing back before the dismount.
func TestMountedCombat_ADismountOntoATrapFiresIt(t *testing.T) {
	t.Parallel()
	r := newTrapRig(t)
	hole := r.trap(t, "Fosso Dourado", 8, 7, pit("1d6"))
	r.fight(t)
	r.steed(t)
	r.mustMount(t, r.caio, "Toren", "Ogro")
	r.nextTurnOf(t, "Toren")
	r.h.roller.queue(4)
	if _, err := r.dismountAs(t, r.caio, "Toren", 8, 7); err != nil {
		t.Fatalf("DismountCombatant() error = %v", err)
	}
	if got := r.point(t, r.master, hole.GetId()).GetTrap().GetState(); got != mapsv1.TrapState_TRAP_STATE_TRIGGERED {
		t.Fatalf("the trap after the dismount = %v, want triggered", got)
	}
	if damages := r.trapDamages(t); len(damages) != 1 || damages[0].GetAmount() != 4 || damages[0].GetCharacterName() != "Toren" {
		t.Errorf("the trap damages = %v, want 4 on Toren", damages)
	}
	r.undoLast(t)
	if got := r.point(t, r.master, hole.GetId()).GetTrap().GetState(); got != mapsv1.TrapState_TRAP_STATE_ARMED {
		t.Errorf("the trap after the first undo = %v, want armed", got)
	}
	r.undoLast(t)
	if got := r.rideRowOf(t, "Toren"); got.MountedOn != r.id(t, "Ogro") || got.Col != 7 {
		t.Errorf("Toren after the second undo = %+v, want on the Ogro again", got)
	}
}
