package play

import (
	"testing"

	playv1 "github.com/PuraFome/meuRPG/backend/gen/meurpg/play/v1"
	rulesv1 "github.com/PuraFome/meuRPG/backend/gen/meurpg/rules/v1"
)

// TestRulesReviewCreatures_BeastSpellsRefusedAt18: SRD Beast Spells (druid 18): the
// druid can cast druid spells in beast shape (no material components). A level 18
// druid in wolf form is nevertheless refused with WILD_SHAPE_NO_SPELLS.
// Needs the database (MEURPG_TEST_DATABASE_URL).
func TestRulesReviewCreatures_BeastSpellsRefusedAt18(t *testing.T) {
	t.Parallel()
	a := newArmedWith(t, func(a *armed) {
		scores := &rulesv1.AbilityScores{Strength: 10, Dexterity: 14, Constitution: 14, Intelligence: 10, Wisdom: 18, Charisma: 8}
		a.toren = a.caio.hero(t, a.campaignID, "Toren", "class:fighter", "race:human", 2, scores, []string{battleaxe}, nil)
		a.pens = a.ana.hero(t, a.campaignID, "Pensantus", "class:wizard", "race:gnome", 1, scores, nil, []string{fireBolt})
		a.bri = a.bia.caster(t, a.campaignID, "Sálvia", "class:druid", "race:half-elf", 18, scores, nil, []string{"spell:produce-flame"}, nil, nil)
	})
	e := a.start(t, plan{
		npcs: []*playv1.Participant{{CharacterId: a.goblin.GetId()}}, npcRolls: []int{1},
		players: map[string]int32{"Sálvia": 20, "Toren": 10, "Pensantus": 5},
		reveal:  []string{"Goblin"},
		at:      map[string][2]int32{"Sálvia": {6, 5}, "Goblin": {7, 5}, "Toren": {2, 2}, "Pensantus": {12, 2}},
	})
	res := a.mustAssume(t, a.bia, a.bri, wolfKey)
	e = res.GetEncounter() // the refusal comes before the action is checked
	if _, err := a.cast(t, a.bia, e, "Sálvia", "spell:produce-flame", nil, a.at(t, "Goblin"), func(r *playv1.CastSpellRequest) {
		r.Roll = &playv1.CastSpellRequest_D20Face{D20Face: 10}
	}); err != nil {
		if b := encounterBlocked(t, err); b.GetReason() == playv1.EncounterBlockedReason_ENCOUNTER_BLOCKED_REASON_WILD_SHAPE_NO_SPELLS {
			t.Errorf("a level 18 druid in wolf form is refused: %v (SRD Beast Spells: she may cast)", err)
		} else {
			t.Logf("refused for another reason: %v (reason %v)", err, b.GetReason())
		}
	}
}
