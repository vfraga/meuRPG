package play

import (
	"testing"

	playv1 "github.com/PuraFome/meuRPG/backend/gen/meurpg/play/v1"
)

// Review finding U3-15: an opportunity offer survives its reactor changing to the mover's side, and can still be answered with an attack on its new friend.
func TestReview3_AnOfferIsDroppedWhenItsReactorChangesSide(t *testing.T) {
	t.Parallel()
	c := newCave(t)
	c.oppFight(t)
	c.leaveGoblin(t)
	offers := c.offersOf(t, c.master)
	if len(offers) != 1 {
		t.Fatalf("offers after leaving the reach = %v, want 1", offers)
	}
	offer := offers[0]

	c.side(t, "Goblin 1", playv1.CombatantSide_COMBATANT_SIDE_PARTY)

	if got := c.offersOf(t, c.master); len(got) != 0 {
		t.Errorf("after Goblin 1 joined the party the offer on Toren still waits: %v", got)
	}
	if _, err := c.offerAttack(t, c.master, "Goblin 1", sword, "Toren", offer.GetId(), d20(15)); err == nil {
		t.Errorf("an ally made an opportunity attack on its friend Toren")
	}
}
