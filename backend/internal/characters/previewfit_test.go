package characters

import (
	"slices"
	"testing"

	"connectrpc.com/connect"

	charactersv1 "github.com/PuraFome/meuRPG/backend/gen/meurpg/characters/v1"
)

// A draft whose picks no longer fit (the class changed under them): the preview of the choices answers it with the
// lists of the draft as it stands, and the preview of the sheet and the save refuse the pick.

func offListSheet(u *unfitTable) *charactersv1.CharacterSheet {
	return u.sheet("skill:stealth", "skill:survival") // the gnome wizard's class list holds neither
}

func TestPreviewChoicesAnswersADraftWhosePicksNoListAllows(t *testing.T) {
	t.Parallel()
	u := newUnfitTable(t, "skill:nature")
	res := u.player.allowed(t, u.campaign, offListSheet(u))
	got := res.GetSkillsAllowed()
	if got == nil || got.GetAny() || !slices.Contains(got.GetKeys(), "skill:arcana") || slices.Contains(got.GetKeys(), "skill:stealth") {
		t.Errorf("skills_allowed = %v, want the wizard's list and the trait's, without stealth", got)
	}
}

func TestPreviewChoicesAnswersAnEditedDraftWhosePicksNoListAllows(t *testing.T) {
	t.Parallel()
	u := newUnfitTable(t, "skill:nature")
	pc := u.player.create(t, u.campaign, charactersv1.CharacterKind_CHARACTER_KIND_PLAYER, "Pensantus", u.sheet("skill:investigation", "skill:insight", "skill:nature"))
	res, err := u.player.api.PreviewChoices(t.Context(), connect.NewRequest(&charactersv1.PreviewChoicesRequest{
		CampaignId: u.campaign, CharacterId: pc.GetId(), Kind: charactersv1.CharacterKind_CHARACTER_KIND_PLAYER, Sheet: offListSheet(u),
	}))
	if err != nil {
		t.Fatalf("PreviewChoices() of a draft with picks no list allows: %v", err)
	}
	if got := res.Msg.GetSkillsAllowed(); got == nil || slices.Contains(got.GetKeys(), "skill:stealth") {
		t.Errorf("skills_allowed = %v, want a list without stealth", got)
	}
}

func TestPreviewCharacterAndTheSaveStillRefuseThePick(t *testing.T) {
	t.Parallel()
	u := newUnfitTable(t, "skill:nature")
	_, err := u.player.preview(t, u.campaign, "", charactersv1.CharacterKind_CHARACTER_KIND_PLAYER, offListSheet(u))
	wantUnfit(t, err, "sheet.full.skill_proficiency_keys[0]", "Furtividade")
	_, err = u.player.api.CreateCharacter(t.Context(), connect.NewRequest(&charactersv1.CreateCharacterRequest{
		CampaignId: u.campaign, Kind: charactersv1.CharacterKind_CHARACTER_KIND_PLAYER, Name: "Pensantus", Sheet: offListSheet(u),
	}))
	wantUnfit(t, err, "sheet.full.skill_proficiency_keys[0]", "Furtividade")
}
