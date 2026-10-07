package play

import (
	"testing"

	"connectrpc.com/connect"

	playv1 "github.com/PuraFome/meuRPG/backend/gen/meurpg/play/v1"
)

// Review finding U3-10: write() matches a retried idempotency key by kind and actor only, so the same key for another request of that kind answers success and applies nothing.
func TestReview3_SameKeyForAnotherRequestIsRefused(t *testing.T) {
	t.Run("SubmitInitiative", func(t *testing.T) {
		f := newFight(t)
		e := f.startFight(t, 1)
		key := newKey()
		submit := func(label string, face int32) error {
			_, err := f.master.combat.SubmitInitiative(t.Context(), connect.NewRequest(&playv1.SubmitInitiativeRequest{
				CampaignId: f.campaignID, EncounterId: e.GetId(), CombatantId: byLabel(t, e, label).GetId(), IdempotencyKey: key,
				Roll: &playv1.SubmitInitiativeRequest_D20Face{D20Face: face},
			}))
			return err
		}
		if err := submit("Toren", 15); err != nil {
			t.Fatalf("first SubmitInitiative error = %v", err)
		}
		wantCode(t, "SubmitInitiative(same key, another combatant)", submit("Pensantus", 9), connect.CodeInvalidArgument)
	})
	t.Run("MoveCombatant", func(t *testing.T) {
		c := newCave(t)
		e := c.fight(t)
		key := newKey()
		move := func(label string, col, row int32) error {
			_, err := c.master.combat.MoveCombatant(t.Context(), connect.NewRequest(&playv1.MoveCombatantRequest{
				CampaignId: c.campaignID, EncounterId: e.GetId(), CombatantId: c.id(t, label), IdempotencyKey: key, Col: col, Row: row, Forced: true,
			}))
			return err
		}
		if err := move("Toren", 7, 7); err != nil {
			t.Fatalf("first MoveCombatant error = %v", err)
		}
		wantCode(t, "MoveCombatant(same key, another square)", move("Toren", 9, 7), connect.CodeInvalidArgument)
	})
}
