package leaktest

import (
	"errors"
	"slices"
	"testing"

	"connectrpc.com/connect"

	campaignsv1 "github.com/PuraFome/meuRPG/backend/gen/meurpg/campaigns/v1"
	charactersv1 "github.com/PuraFome/meuRPG/backend/gen/meurpg/characters/v1"
	rulesv1 "github.com/PuraFome/meuRPG/backend/gen/meurpg/rules/v1"
)

// A level up whose new skill or expertise is on no list of the sheet is refused for a player, and the master's preview of it
// is accepted and reads the fit issue: the level asks one pick from a list, and the pick is on no list. Only the owning
// player takes the level, so the master's answer is the preview's.
func TestOnlyAPlayerIsRefusedALevelUpPickNoListAllows(t *testing.T) {
	for _, tc := range []struct {
		name   string
		kind   string
		reason charactersv1.LevelUpRefusalReason
		picks  func(*charactersv1.LevelUpChoices)
	}{
		{"skill", "skill", charactersv1.LevelUpRefusalReason_LEVEL_UP_REFUSAL_REASON_SKILLS, func(c *charactersv1.LevelUpChoices) { c.SkillProficiencyKeys = []string{unlistedSkill} }},
		{"expertise", "expertise", charactersv1.LevelUpRefusalReason_LEVEL_UP_REFUSAL_REASON_EXPERTISE, func(c *charactersv1.LevelUpChoices) { c.ExpertiseSkillKeys = []string{"skill:history"} }},
	} {
		t.Run(tc.name, func(t *testing.T) {
			p := newChoicesProbe(t)
			w, ctx := p.w, t.Context()
			must(w.master.campaigns.SetCampaignXpMode(ctx, rq(&campaignsv1.SetCampaignXpModeRequest{CampaignId: w.campaign, XpMode: campaignsv1.XpMode_XP_MODE_ENEMIES})))
			own := tableClass("Escola dos Rios")
			own.Levels[1].Features = []*rulesv1.TableFeature{{
				NamePt: "Estudo do rio", DescPt: []string{"Texto de teste."},
				Effects: []*rulesv1.TableEffect{{Type: "choice", Choice: tc.kind, Count: 1, From: []string{"skill:stealth"}}},
			}}
			class := p.createClass(own)
			s := sheet(class, 1)
			s.GetFull().SkillProficiencyKeys = []string{"skill:arcana", "skill:history"}
			s.GetFull().ExperiencePoints = 6500
			pc := must(w.ana.characters.CreateCharacter(ctx, rq(&charactersv1.CreateCharacterRequest{
				CampaignId: w.campaign, Kind: charactersv1.CharacterKind_CHARACTER_KIND_PLAYER, Name: "Perita", Sheet: s,
			}))).GetCharacter()
			choices := &charactersv1.LevelUpChoices{
				ClassKey:  class,
				HitPoints: &charactersv1.LevelUpHitPoints{Method: charactersv1.LevelUpHitPointsMethod_LEVEL_UP_HIT_POINTS_METHOD_AVERAGE},
			}
			tc.picks(choices)
			preview := func(who *person) *charactersv1.PreviewLevelUpResponse {
				return must(who.characters.PreviewLevelUp(ctx, rq(&charactersv1.PreviewLevelUpRequest{CampaignId: w.campaign, CharacterId: pc.GetId(), Choices: choices})))
			}
			if got := preview(w.ana); got.GetRefusal().GetReason() != tc.reason {
				t.Errorf("the player's preview: refusal %v, want %v", got.GetRefusal(), tc.reason)
			}
			_, err := w.ana.characters.LevelUpCharacter(ctx, rq(&charactersv1.LevelUpCharacterRequest{
				CampaignId: w.campaign, CharacterId: pc.GetId(), Revision: pc.GetRevision(), Choices: choices,
			}))
			var refusal *charactersv1.LevelUpRefusal
			if ce, ok := errors.AsType[*connect.Error](err); ok {
				for _, d := range ce.Details() {
					if v, derr := d.Value(); derr == nil {
						if r, ok := v.(*charactersv1.LevelUpRefusal); ok {
							refusal = r
						}
					}
				}
			}
			if refusal.GetReason() != tc.reason {
				t.Errorf("the player's level up: %v with refusal %v, want %v", err, refusal, tc.reason)
			}
			got := preview(w.master)
			if got.GetRefusal() != nil || got.GetAfter() == nil {
				t.Fatalf("the master's preview: refusal %v, sheet after %v, want the level accepted", got.GetRefusal(), got.GetAfter() != nil)
			}
			if !slices.ContainsFunc(got.GetAfter().GetIssues(), func(is *rulesv1.Issue) bool { return is.GetCode() == "pick_fit" }) {
				t.Errorf("the master's preview reads %v, want the pick_fit issue", got.GetAfter().GetIssues())
			}
		})
	}
}
