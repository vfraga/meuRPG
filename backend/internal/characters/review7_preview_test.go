package characters

import (
	"strings"
	"testing"

	"connectrpc.com/connect"

	charactersv1 "github.com/PuraFome/meuRPG/backend/gen/meurpg/characters/v1"
	rulesv1 "github.com/PuraFome/meuRPG/backend/gen/meurpg/rules/v1"
)

// Finding U7-4 (review/unit-07-characters.md): PreviewLevelUp only drops `after`
// when the refusal is ARCHIVED_CHOICE or SWITCHED_OFF_CHOICE, but the plan sets those
// only when no earlier refusal exists. With an earlier one (here HIT_POINT_ROLL_MISSING:
// ROLLED_IN_APP with no kept roll) the player still gets the sheet built with the
// switched-off subclass (RN-23: what is off never appears to players).
func TestReview7_PreviewDoesNotLeakSwitchedOffChoice(t *testing.T) {
	t.Parallel()
	h := newHarness(t)
	h.svc.SetLevelUps(xpLevelUps{})
	master, player := h.newUser("Samuel"), h.newUser("Dona")
	campaign := h.newCampaign(master, "Mirathel", player)
	sub := master.addEntry(t, campaign, testSubclass("Caminho do Vento", "class:fighter"))
	toren := &charactersv1.CharacterSheet{Content: &charactersv1.CharacterSheet_Full{Full: &charactersv1.FullSheet{
		BaseScores:        &rulesv1.AbilityScores{Strength: 15, Dexterity: 12, Constitution: 14, Intelligence: 8, Wisdom: 10, Charisma: 10},
		RaceKey:           "race:human",
		Background:        &charactersv1.FullSheet_BackgroundKey{BackgroundKey: "background:acolyte"},
		Classes:           []*charactersv1.ClassLevel{{ClassKey: "class:fighter", Level: 2}},
		ArmorKey:          "equipment:chain-mail",
		WeaponKeys:        []string{"equipment:longsword"},
		FeatureChoiceKeys: []string{"feature:fighter-fighting-style-defense"},
		ExperiencePoints:  900,
	}}}
	pc := player.create(t, campaign, charactersv1.CharacterKind_CHARACTER_KIND_PLAYER, "Toren", toren)
	h.lockSheets(campaign)
	master.setOff(t, campaign, true, sub.GetKey())

	preview := func(method charactersv1.LevelUpHitPointsMethod) *charactersv1.PreviewLevelUpResponse {
		pv, err := player.api.PreviewLevelUp(t.Context(), connect.NewRequest(&charactersv1.PreviewLevelUpRequest{
			CampaignId: campaign, CharacterId: pc.GetId(),
			Choices: &charactersv1.LevelUpChoices{
				ClassKey: "class:fighter", SubclassKey: sub.GetKey(),
				HitPoints: &charactersv1.LevelUpHitPoints{Method: method},
			},
		}))
		if err != nil {
			t.Fatalf("PreviewLevelUp() error = %v", err)
		}
		return pv.Msg
	}

	// Control: no earlier refusal, correctly suppressed.
	if r := preview(charactersv1.LevelUpHitPointsMethod_LEVEL_UP_HIT_POINTS_METHOD_AVERAGE); r.GetAfter() != nil {
		t.Errorf("control (average): after = %v, want none", r.GetAfter())
	}
	// Earlier refusal: in-app roll never made.
	r := preview(charactersv1.LevelUpHitPointsMethod_LEVEL_UP_HIT_POINTS_METHOD_ROLLED_IN_APP)
	t.Logf("refusal = %v", r.GetRefusal())
	if r.GetAfter() != nil {
		js := jsonOf(t, r.GetAfter())
		t.Errorf("player's preview of an off subclass with refusal %v carries `after` (mentions subclass key: %v, name: %v): %.600s",
			r.GetRefusal().GetReason(), strings.Contains(js, sub.GetKey()), strings.Contains(js, "Caminho do Vento"), js)
	}
}
