package leaktest

import (
	"testing"

	campaignsv1 "github.com/PuraFome/meuRPG/backend/gen/meurpg/campaigns/v1"
	charactersv1 "github.com/PuraFome/meuRPG/backend/gen/meurpg/characters/v1"
	rulesv1 "github.com/PuraFome/meuRPG/backend/gen/meurpg/rules/v1"
)

// A level up whose new skill is on no list of the sheet is refused for a player, and the master's preview of it is accepted:
// the level asks one skill from a list, and the pick is on no list. Only the owning player takes the level, so the master's
// answer is the preview's.
func TestOnlyAPlayerIsRefusedALevelUpSkillNoListAllows(t *testing.T) {
	p := newChoicesProbe(t)
	w, ctx := p.w, t.Context()
	must(w.master.campaigns.SetCampaignXpMode(ctx, rq(&campaignsv1.SetCampaignXpModeRequest{CampaignId: w.campaign, XpMode: campaignsv1.XpMode_XP_MODE_ENEMIES})))
	own := tableClass("Escola dos Rios")
	own.Levels[1].Features = []*rulesv1.TableFeature{{
		NamePt: "Estudo do rio", DescPt: []string{"Texto de teste."},
		Effects: []*rulesv1.TableEffect{{Type: "choice", Choice: "skill", Count: 1, From: []string{"skill:stealth"}}},
	}}
	class := p.createClass(own)
	s := sheet(class, 1)
	s.GetFull().SkillProficiencyKeys = []string{"skill:arcana", "skill:history"}
	s.GetFull().ExperiencePoints = 6500
	pc := must(w.ana.characters.CreateCharacter(ctx, rq(&charactersv1.CreateCharacterRequest{
		CampaignId: w.campaign, Kind: charactersv1.CharacterKind_CHARACTER_KIND_PLAYER, Name: "Perita", Sheet: s,
	}))).GetCharacter()
	choices := &charactersv1.LevelUpChoices{
		ClassKey: class, SkillProficiencyKeys: []string{unlistedSkill},
		HitPoints: &charactersv1.LevelUpHitPoints{Method: charactersv1.LevelUpHitPointsMethod_LEVEL_UP_HIT_POINTS_METHOD_AVERAGE},
	}
	preview := func(who *person) *charactersv1.PreviewLevelUpResponse {
		return must(who.characters.PreviewLevelUp(ctx, rq(&charactersv1.PreviewLevelUpRequest{CampaignId: w.campaign, CharacterId: pc.GetId(), Choices: choices})))
	}
	if got := preview(w.ana); got.GetRefusal().GetReason() != charactersv1.LevelUpRefusalReason_LEVEL_UP_REFUSAL_REASON_SKILLS {
		t.Errorf("the player's preview: refusal %v, want the refusal of the skill", got.GetRefusal())
	}
	if _, err := w.ana.characters.LevelUpCharacter(ctx, rq(&charactersv1.LevelUpCharacterRequest{
		CampaignId: w.campaign, CharacterId: pc.GetId(), Revision: pc.GetRevision(), Choices: choices,
	})); err == nil {
		t.Error("the player's level up with a skill no list allows is accepted")
	}
	if got := preview(w.master); got.GetRefusal() != nil || got.GetAfter() == nil {
		t.Fatalf("the master's preview: refusal %v, sheet after %v, want the level accepted", got.GetRefusal(), got.GetAfter() != nil)
	}
}
