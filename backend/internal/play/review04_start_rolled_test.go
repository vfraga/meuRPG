package play

import (
	"testing"

	"connectrpc.com/connect"

	playv1 "github.com/PuraFome/meuRPG/backend/gen/meurpg/play/v1"
)

// Finding U4-1: StartEncounter with rolled monster hit points writes ONE combatants_added event listing every
// monster (id, hit points, dice, each die face); session_events.payload is capped at 4096 bytes, so a legal
// start of 36 rolled bandits (4 players + 36 = 40) fails with `internal` instead of succeeding.
func TestReview4_StartWithRolledMonstersOverflowsEventPayload(t *testing.T) {
	t.Parallel()
	m := newMirathel(t)
	const n = 36 // 4 players + 36 = 40: the server's own TOO_MANY check accepts it (see TestMR043_TheFortyCountsThePartysCreatures)
	_, err := m.master.combat.StartEncounter(t.Context(), connect.NewRequest(&playv1.StartEncounterRequest{
		CampaignId: m.campaignID, IdempotencyKey: newKey(), Name: "x", Monsters: groups(bandit, n),
		MonsterHitPoints: playv1.MonsterHitPoints_MONSTER_HIT_POINTS_ROLLED,
	}))
	if err != nil {
		t.Errorf("StartEncounter(%d rolled bandits, 4 players = 40 combatants) error = %v; want success (within the limit of 40)", n, err)
	}
}
