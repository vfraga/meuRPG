package play

import (
	"testing"

	"connectrpc.com/connect"

	charactersv1 "github.com/PuraFome/meuRPG/backend/gen/meurpg/characters/v1"
	playv1 "github.com/PuraFome/meuRPG/backend/gen/meurpg/play/v1"
)

// Review finding U3-2: a PC the master marked dead mid-combat makes every non-forced NPC move and GetMoveOptions fail with not_found (isDown error is unfiltered).
func TestReview3_DeadCharacterBreaksNPCMoves(t *testing.T) {
	t.Parallel()
	c := newCave(t)
	c.fight(t)

	if _, err := c.master.characters.MarkCharacterDead(t.Context(), connect.NewRequest(&charactersv1.MarkCharacterDeadRequest{
		CampaignId: c.campaignID, CharacterId: c.pens.GetId(),
	})); err != nil {
		t.Fatalf("MarkCharacterDead() error = %v", err)
	}

	// The NPC is on turn: only then does a move offer opportunity attacks.
	c.passTo(t, c.get(t, c.master), "Goblin 1")

	// A forced move still works (no reactors are computed).
	if _, err := c.move(t, c.master, "Goblin 1", 17, 5, func(r *playv1.MoveCombatantRequest) { r.Forced = true }); err != nil {
		t.Errorf("forced MoveCombatant(Goblin 1) error = %v, want nil", err)
	}
	if _, err := c.options(t, c.master, "Goblin 1"); err != nil {
		t.Errorf("GetMoveOptions(Goblin 1) error = %v, want nil", err)
	}
	if _, err := c.move(t, c.master, "Goblin 1", 16, 5); err != nil {
		t.Errorf("MoveCombatant(Goblin 1, not forced) error = %v, want nil", err)
	}
}
