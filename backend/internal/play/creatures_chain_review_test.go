package play

import (
	"testing"

	"connectrpc.com/connect"

	charactersv1 "github.com/PuraFome/meuRPG/backend/gen/meurpg/characters/v1"
	playv1 "github.com/PuraFome/meuRPG/backend/gen/meurpg/play/v1"
	rulesv1 "github.com/PuraFome/meuRPG/backend/gen/meurpg/rules/v1"
)

// TestRulesReviewCreatures_ChainFamiliarReactionNeedsForgoneAttack: SRD Pact of the Chain: "when you
// take the Attack action, you can forgo one of your own attacks to allow your familiar to
// make one attack of its own with its reaction." Here it is Toren's turn: the warlock
// (Brisa) took no Attack action and forwent nothing, yet the imp attacks with its reaction.
// Needs the database (MEURPG_TEST_DATABASE_URL).
func TestRulesReviewCreatures_ChainFamiliarReactionNeedsForgoneAttack(t *testing.T) {
	t.Parallel()
	a := newArmedWith(t, func(a *armed) {
		scores := &rulesv1.AbilityScores{Strength: 10, Dexterity: 14, Constitution: 14, Intelligence: 10, Wisdom: 10, Charisma: 16}
		a.toren = a.caio.hero(t, a.campaignID, "Toren", "class:fighter", "race:human", 2, scores, []string{battleaxe}, nil)
		a.pens = a.ana.hero(t, a.campaignID, "Pensantus", "class:wizard", "race:gnome", 1, scores, nil, []string{fireBolt})
		sheet := &charactersv1.CharacterSheet{Content: &charactersv1.CharacterSheet_Full{Full: &charactersv1.FullSheet{
			BaseScores: scores, RaceKey: "race:human", Classes: []*charactersv1.ClassLevel{{ClassKey: "class:warlock", Level: 3}},
			FeatureChoiceKeys: []string{"feature:pact-of-the-chain"},
		}}}
		res, err := a.bia.characters.CreateCharacter(t.Context(), connect.NewRequest(&charactersv1.CreateCharacterRequest{
			CampaignId: a.campaignID, Kind: charactersv1.CharacterKind_CHARACTER_KIND_PLAYER, Name: "Brisa", Sheet: sheet,
		}))
		if err != nil {
			t.Fatalf("CreateCharacter(Brisa) error = %v", err)
		}
		a.bri = res.Msg.GetCharacter()
	})
	// The pact gives the ritual with no spell on the sheet; a wizard's form still works.
	a.mustCastSummon(t, a.bia, a.bri, findFamiliar, nil, 0, []string{"monster:imp"}, "Fagulha")
	list := a.mustCreatures(t, a.bia, a.bri)
	if len(list) != 1 || list[0].GetAttack() != 2 || list[0].GetMonsterNamePt() == "" {
		t.Fatalf("creatures = %v, want the imp, which attacks with its reaction", list)
	}

	e := a.start(t, plan{
		npcs:     []*playv1.Participant{{CharacterId: a.goblin.GetId()}},
		npcRolls: []int{3},
		players:  map[string]int32{"Toren": 18, "Pensantus": 10, "Brisa": 5, "Fagulha": 2},
		reveal:   []string{"Goblin"},
		at:       map[string][2]int32{"Toren": {3, 3}, "Goblin": {4, 3}, "Pensantus": {10, 3}, "Brisa": {8, 8}, "Fagulha": {5, 3}},
	})
	key := a.mustOptions(t, a.bia, e, "Fagulha").GetOptions().GetAttacks()[0].GetAttack().GetKey()
	if _, err := a.attackAs(t, a.bia, e, "Fagulha", key, "Goblin", d20(15), true); err == nil {
		t.Error("the Pact of the Chain imp attacked with its reaction on Toren's turn, with no Attack action of its warlock to forgo an attack from (SRD)")
	}
}
