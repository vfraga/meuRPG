package play

import (
	"testing"

	"connectrpc.com/connect"

	playv1 "github.com/PuraFome/meuRPG/backend/gen/meurpg/play/v1"
)

// TestTheDieOfAnAnswerIsTheRollersOwn: the copies of an NPC share a character, and each holds a die of its own: the
// answer to one copy's attack uses that copy's die, not another's.
func TestTheDieOfAnAnswerIsTheRollersOwn(t *testing.T) {
	t.Parallel()
	a := newArmed(t)
	e := a.start(t, plan{
		npcs:     []*playv1.Participant{{CharacterId: a.goblin.GetId(), Count: 2}},
		npcRolls: []int{18, 18},
		players:  map[string]int32{"Toren": 5, "Pensantus": 5, "Brisa": 5},
		reveal:   []string{"Goblin 1", "Goblin 2"},
		at:       map[string][2]int32{"Toren": {3, 3}, "Goblin 1": {4, 3}, "Goblin 2": {3, 4}, "Pensantus": {10, 3}, "Brisa": {8, 8}},
	})
	for label, sides := range map[string]int{"Goblin 1": 6, "Goblin 2": 12} {
		a.giveCombatantDie(t, label, e)
		if _, err := a.h.pool.Exec(t.Context(), `UPDATE combatants SET inspiration_sides = $2 WHERE id = $1`, a.id(t, label), sides); err != nil {
			t.Fatalf("the die of %s: %v", label, err)
		}
	}
	first := a.mustAttack(t, a.master, e, "Goblin 1", sword, "Toren", d20(8))
	second := a.mustAttack(t, a.master, e, "Goblin 2", sword, "Toren", d20(9))
	if first.GetInspirationOffer().GetDie().GetSides() != 6 || second.GetInspirationOffer().GetDie().GetSides() != 12 {
		t.Fatalf("the dice offered = d%d and d%d, want d6 and d12", first.GetInspirationOffer().GetDie().GetSides(), second.GetInspirationOffer().GetDie().GetSides())
	}
	// A 10 is a face of the second copy's d12, not of the first one's d6.
	typed10 := func(r *playv1.AnswerBardicInspirationRequest) {
		r.Use, r.Roll = true, &playv1.AnswerBardicInspirationRequest_TypedFace{TypedFace: 10}
	}
	res, err := a.answer(t, a.master, e, second.GetInspirationOffer().GetHoldId(), typed10)
	if err != nil {
		t.Fatalf("the second copy's answer with a 10: error = %v", err)
	}
	if r := res.GetAttack().GetRoll(); len(r.GetBonusDice()) != 1 || r.GetBonusDice()[0].GetFace() != 10 || r.GetBonusDice()[0].GetSides() != 12 {
		t.Errorf("the second copy's roll = %v, want its d12's 10 added", r)
	}
	var sides *int32
	if err := a.h.pool.QueryRow(t.Context(), `SELECT inspiration_sides FROM combatants WHERE id = $1`, a.id(t, "Goblin 1")).Scan(&sides); err != nil || sides == nil || *sides != 6 {
		t.Errorf("the first copy's die = %v (%v), want its d6 still held", sides, err)
	}
	_, err = a.answer(t, a.master, e, first.GetInspirationOffer().GetHoldId(), typed10)
	wantCode(t, "the first copy's answer with a 10", err, connect.CodeInvalidArgument)
}
