package play

import (
	"testing"

	"connectrpc.com/connect"
	"google.golang.org/protobuf/proto"

	playv1 "github.com/PuraFome/meuRPG/backend/gen/meurpg/play/v1"
)

// Review finding U3-1: MoveCombatant lets a player's character at 0 HP (down) walk on their turn; it skips the down gate.
func TestReview3_DownPlayerMoves(t *testing.T) {
	c := newCave(t)
	c.fight(t) // Toren is first on turn
	if _, err := c.master.play.AdjustCharacterVitals(t.Context(), connect.NewRequest(&playv1.AdjustCharacterVitalsRequest{
		CampaignId: c.campaignID, CharacterId: c.toren.GetId(), IdempotencyKey: newKey(), HitPointsCurrent: proto.Int32(0),
	})); err != nil {
		t.Fatalf("AdjustCharacterVitals() error = %v", err)
	}
	_, err := c.move(t, c.caio, "Toren", 7, 7)
	wantBlockedBy(t, "a down player's MoveCombatant", err, blockedDown)
}
