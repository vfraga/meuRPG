package play

import (
	"testing"

	"connectrpc.com/connect"

	playv1 "github.com/PuraFome/meuRPG/backend/gen/meurpg/play/v1"
)

// Review finding U3-3: a member of a joint turn who dies or is defeated keeps turn_state 'acting', so the turn never passes when the others end their parts.
func TestReview3_ConfirmedDeathInJointTurnBlocksThePass(t *testing.T) {
	t.Parallel()
	a, e := jointFight(t, false)
	// Toren (second of the Brisa,Toren group) is down with three failures; the master confirms the death.
	a.correct(t, a.toren, hpIs(0))
	a.execSQL(t, `UPDATE combatants SET death_failures = 3 WHERE id = $1`, a.id(t, "Toren"))
	if _, err := a.master.combat.ConfirmDeath(t.Context(), connect.NewRequest(&playv1.ConfirmDeathRequest{
		CampaignId: a.campaignID, EncounterId: e.GetId(), CombatantId: a.id(t, "Toren"), IdempotencyKey: newKey(),
	})); err != nil {
		t.Fatalf("ConfirmDeath() error = %v", err)
	}
	// Brisa, the only living member, ends her part: the turn must pass to the Capitão.
	got := a.mustEndPart(t, a.bia, e, "Brisa")
	if group(got) != "Capitão Goblin" {
		t.Errorf("after the last living member ended: group %q, current %q; want the turn passed to Capitão Goblin", group(got), current(got))
	}
}

// Review finding U3-3: a member of a joint turn who dies or is defeated keeps turn_state 'acting', so the turn never passes when the others end their parts.
func TestReview3_DefeatedNPCInJointTurnBlocksThePass(t *testing.T) {
	t.Parallel()
	a := newArmed(t)
	e := a.start(t, plan{
		npcs: []*playv1.Participant{
			{CharacterId: a.capitao.GetId(), Hidden: new(false)},
			{CharacterId: a.goblin.GetId(), Count: 2},
		},
		npcRolls: []int{16, 12, 12},
		players:  map[string]int32{"Brisa": 13, "Toren": 17, "Pensantus": 12}, // Toren 19, Brisa 16 = Capitão 16, Pensantus 14
		reveal:   []string{"Goblin 1", "Goblin 2"},
		at: map[string][2]int32{
			"Brisa": {9, 4}, "Toren": {5, 5}, "Capitão Goblin": {10, 5}, "Pensantus": {8, 2}, "Goblin 1": {6, 5}, "Goblin 2": {14, 2},
		},
	})
	e = a.mustEndPart(t, a.master, e, "Toren")
	if group(e) != "Brisa,Capitão Goblin" {
		t.Fatalf("group = %q, want Brisa,Capitão Goblin", group(e))
	}
	if _, err := a.adjustHP(t, e, "Capitão Goblin", damageHP(1000)); err != nil {
		t.Fatalf("AdjustCombatantHitPoints() error = %v", err)
	}
	got := a.mustEndPart(t, a.bia, e, "Brisa")
	if group(got) != "Pensantus" {
		t.Errorf("after the last living member ended: group %q, current %q; want the turn passed to Pensantus", group(got), current(got))
	}
}
