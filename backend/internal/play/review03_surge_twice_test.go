package play

import (
	"testing"

	playv1 "github.com/PuraFome/meuRPG/backend/gen/meurpg/play/v1"
	rulesv1 "github.com/PuraFome/meuRPG/backend/gen/meurpg/rules/v1"
)

// Review finding U3-8: a level-17 Fighter can use Action Surge twice in the same turn (the SRD allows one use per turn).
func TestReview3_ActionSurgeOnlyOncePerTurn(t *testing.T) {
	t.Parallel()
	a := newArmedWith(t, func(a *armed) {
		a.toren = a.caio.hero(t, a.campaignID, "Toren", "class:fighter", "race:human", 17,
			&rulesv1.AbilityScores{Strength: 16, Dexterity: 13, Constitution: 14, Intelligence: 10, Wisdom: 10, Charisma: 8}, []string{battleaxe}, nil)
		a.pens = a.ana.hero(t, a.campaignID, "Pensantus", "class:wizard", "race:gnome", 1,
			&rulesv1.AbilityScores{Strength: 10, Dexterity: 14, Constitution: 12, Intelligence: 16, Wisdom: 10, Charisma: 8}, nil, []string{fireBolt})
		a.bri = a.bia.hero(t, a.campaignID, "Brisa", "class:fighter", "race:human", 2,
			&rulesv1.AbilityScores{Strength: 10, Dexterity: 16, Constitution: 14, Intelligence: 10, Wisdom: 10, Charisma: 8}, []string{rapier}, nil)
	})
	e := a.start(t, plan{
		npcs: []*playv1.Participant{{CharacterId: a.goblin.GetId()}}, npcRolls: []int{1}, reveal: []string{"Goblin"},
		at:      map[string][2]int32{"Pensantus": {5, 5}, "Toren": {6, 5}, "Brisa": {5, 6}, "Goblin": {7, 5}},
		players: map[string]int32{"Toren": 20, "Pensantus": 15, "Brisa": 10},
	})

	o := a.mustOptions(t, a.caio, e, "Toren")
	if surge := featureOption(o, actionSurgeKey); surge == nil || surge.GetUsesLeft() != 2 {
		t.Fatalf("Surto de ação = %v, want 2 uses at fighter level 17", surge)
	}
	a.mustAttack(t, a.caio, e, "Toren", battleaxe, "Goblin", d20(3))
	if _, err := a.feature(t, a.caio, e, "Toren", actionSurgeKey, noTakeRoll); err != nil {
		t.Fatalf("first Surto de ação error = %v", err)
	}
	a.mustAttack(t, a.caio, e, "Toren", battleaxe, "Goblin", d20(3))
	if _, err := a.feature(t, a.caio, e, "Toren", actionSurgeKey, noTakeRoll); err == nil {
		t.Errorf("second Surto de ação in the same turn accepted, want it refused (SRD: only one use per turn)")
	}
}
