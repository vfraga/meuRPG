package play

import (
	"testing"

	"connectrpc.com/connect"

	charactersv1 "github.com/PuraFome/meuRPG/backend/gen/meurpg/characters/v1"
	playv1 "github.com/PuraFome/meuRPG/backend/gen/meurpg/play/v1"
	rulesv1 "github.com/PuraFome/meuRPG/backend/gen/meurpg/rules/v1"
)

// A table feature changes Wild Shape (MR-025, MR-037): a Druid 5 of the table's subclass takes
// beasts of challenge rating 1 (the SRD allows 1/2 at this level) and changing form costs no use.
// These tests need the database (MEURPG_TEST_DATABASE_URL).

const direWolfKey = "monster:dire-wolf"

func newWildTable(t *testing.T, effects ...*rulesv1.TableEffect) (*armed, *charactersv1.Character) {
	t.Helper()
	var druid *charactersv1.Character
	a := newArmedWith(t, func(a *armed) {
		res, err := a.master.table.CreateTableEntry(t.Context(), connect.NewRequest(&rulesv1.CreateTableEntryRequest{
			CampaignId: a.campaignID, Body: &rulesv1.CreateTableEntryRequest_TableSubclass{TableSubclass: &rulesv1.TableSubclass{
				NamePt: "Círculo das Formas", ClassKey: "class:druid", DescPt: []string{"Formas de teste."},
				Levels: []*rulesv1.TableSubclassLevel{{Level: 2, Features: []*rulesv1.TableFeature{{NamePt: "Forma Maior", DescPt: []string{"Mais fácil."}, Effects: effects}}}},
			}},
		}))
		if err != nil {
			t.Fatalf("CreateTableEntry() error = %v", err)
		}
		druid = a.bia.heroWith(t, a.campaignID, "Sálvia", classLevel("class:druid", 5, res.Msg.GetEntry().GetKey()),
			&rulesv1.AbilityScores{Strength: 10, Dexterity: 12, Constitution: 14, Intelligence: 10, Wisdom: 16, Charisma: 8}, nil, nil, nil)
		a.bri = druid
		a.toren = a.caio.hero(t, a.campaignID, "Toren", "class:fighter", "race:human", 2,
			&rulesv1.AbilityScores{Strength: 16, Dexterity: 13, Constitution: 14, Intelligence: 10, Wisdom: 10, Charisma: 8}, []string{battleaxe}, nil)
		a.pens = a.ana.hero(t, a.campaignID, "Pensantus", "class:wizard", "race:gnome", 1,
			&rulesv1.AbilityScores{Strength: 10, Dexterity: 14, Constitution: 12, Intelligence: 16, Wisdom: 10, Charisma: 8}, nil, []string{fireBolt})
	})
	return a, druid
}

func TestMR025_ATableFeatureRaisesTheWildShapeRatingAndMakesItFree(t *testing.T) {
	t.Parallel()
	a, druid := newWildTable(t, &rulesv1.TableEffect{Type: "wild_shape", MaxCr: "1", FormCost: "0"})

	// The list and the check agree: the dire wolf (CR 1) is allowed.
	list, err := a.bia.characters.ListWildShapeForms(t.Context(), connect.NewRequest(&charactersv1.ListWildShapeFormsRequest{CampaignId: a.campaignID, CharacterId: druid.GetId()}))
	if err != nil || list.Msg.GetMaxCr() != "1" {
		t.Fatalf("ListWildShapeForms() = %v, %v; want the limit CR 1", list.Msg, err)
	}
	a.mustAssume(t, a.bia, druid, direWolfKey)
	if v := a.vitals(t, druid); v.GetWildShape().GetBeastKey() != direWolfKey || used(v, wildShapeResource) != 0 {
		t.Fatalf("after the change: form %v, %d uses spent; want the dire wolf and no use", v.GetWildShape(), used(v, wildShapeResource))
	}
	a.mustLeave(t, a.bia, druid)

	// A beast above the rating is still refused.
	_, err = a.assume(t, a.bia, druid, "monster:giant-crocodile")
	wantEncounterBlocked(t, err, playv1.EncounterBlockedReason_ENCOUNTER_BLOCKED_REASON_WILD_SHAPE_BEAST_NOT_ALLOWED)
}

func TestMR025_ACheaperWildShapeUndoesWithoutGivingBackAUse(t *testing.T) {
	t.Parallel()
	a, druid := newWildTable(t, &rulesv1.TableEffect{Type: "wild_shape", MaxCr: "1", FormCost: "0"})
	a.start(t, plan{
		npcs: []*playv1.Participant{{CharacterId: a.goblin.GetId()}}, npcRolls: []int{1},
		players: map[string]int32{"Sálvia": 20, "Toren": 10, "Pensantus": 5},
		reveal:  []string{"Goblin"},
		at:      map[string][2]int32{"Sálvia": {6, 5}, "Goblin": {7, 5}, "Toren": {2, 2}, "Pensantus": {12, 2}},
	})
	a.mustAssume(t, a.bia, druid, direWolfKey)
	if n := used(a.vitals(t, druid), wildShapeResource); n != 0 {
		t.Fatalf("the change spent %d uses, want none", n)
	}
	a.undoLast(t, a.get(t, a.master))
	v := a.vitals(t, druid)
	if v.GetWildShape() != nil || used(v, wildShapeResource) != 0 {
		t.Errorf("after the undo: form %v, %d uses spent; want none of either", v.GetWildShape(), used(v, wildShapeResource))
	}
}
