package play

import (
	"testing"

	"connectrpc.com/connect"

	playv1 "github.com/PuraFome/meuRPG/backend/gen/meurpg/play/v1"
	rulesv1 "github.com/PuraFome/meuRPG/backend/gen/meurpg/rules/v1"
)

// Review finding U3-13: reach weapons (glaive etc.) keep normal_range_ft 5, so a 10 ft attack is refused as out of range.
func TestReview3_ReachWeaponHitsAt10Feet(t *testing.T) {
	a := newArmedWith(t, func(a *armed) {
		a.toren = a.caio.hero(t, a.campaignID, "Toren", "class:fighter", "race:human", 2,
			&rulesv1.AbilityScores{Strength: 15, Dexterity: 13, Constitution: 14, Intelligence: 10, Wisdom: 10, Charisma: 8},
			[]string{"equipment:glaive"}, nil)
	})
	goblin := a.master.sizedNPC(t, a.campaignID, "Goblin", 7, 12, rulesv1.CreatureSize_CREATURE_SIZE_SMALL)
	a.mapID = a.h.newMapOf(a.campaignID, 24, 1200, 800)
	if _, err := a.master.play.SetCurrentMap(t.Context(), connect.NewRequest(&playv1.SetCurrentMapRequest{CampaignId: a.campaignID, MapId: a.mapID})); err != nil {
		t.Fatalf("SetCurrentMap() error = %v", err)
	}
	e := a.start(t, plan{
		npcs:     []*playv1.Participant{{CharacterId: goblin.GetId()}},
		npcRolls: []int{2},
		players:  map[string]int32{"Toren": 18},
		reveal:   []string{"Goblin"},
		at:       map[string][2]int32{"Toren": {3, 3}, "Goblin": {5, 3}},
	})
	if _, err := a.attack(t, a.caio, e, "Toren", "equipment:glaive", "Goblin", d20(12)); err != nil {
		t.Errorf("glaive (Reach, 10 ft) attack on a goblin two squares away = %v, want it allowed", err)
	}
}
