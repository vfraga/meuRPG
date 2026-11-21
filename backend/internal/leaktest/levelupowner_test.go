package leaktest

import (
	"bytes"
	"strings"
	"testing"

	"github.com/PuraFome/meuRPG/backend/gen/meurpg/characters/v1/charactersv1connect"

	campaignsv1 "github.com/PuraFome/meuRPG/backend/gen/meurpg/campaigns/v1"
	charactersv1 "github.com/PuraFome/meuRPG/backend/gen/meurpg/characters/v1"
	rulesv1 "github.com/PuraFome/meuRPG/backend/gen/meurpg/rules/v1"
)

// A pin, not a guard row: the closed-choice answer comes before the sources of a level are built, so the options a player asks
// for a multiclass class the table switched off or archived name neither the class nor its skill source.
func TestTheSkillSourcesOfAMulticlassOwnerTheTableKeepsOutReachNoPlayer(t *testing.T) {
	p := newChoicesProbe(t)
	w, ctx := p.w, t.Context()
	must(w.master.campaigns.SetCampaignXpMode(ctx, rq(&campaignsv1.SetCampaignXpModeRequest{CampaignId: w.campaign, XpMode: campaignsv1.XpMode_XP_MODE_ENEMIES})))
	const canary = "Zorvak4321"
	const extra = "skill:performance"
	own := func(name string) *rulesv1.TableClass {
		return tableClass(name, &rulesv1.TableFeature{
			NamePt: name + " canto", DescPt: []string{"Texto de teste."},
			Effects: []*rulesv1.TableEffect{{Type: "choice", Choice: "skill", Count: 1, From: []string{extra}}},
		})
	}
	options := func(who *person, id, class string) reply {
		return who.call(charactersv1connect.CharacterServiceGetLevelUpOptionsProcedure, &charactersv1.GetLevelUpOptionsRequest{CampaignId: w.campaign, CharacterId: id, ClassKey: class})
	}
	key := p.createClass(own(canary + " fora"))
	s := sheet("class:wizard", 2)
	s.GetFull().ExperiencePoints = 6500
	apart := must(w.ana.characters.CreateCharacter(ctx, rq(&charactersv1.CreateCharacterRequest{
		CampaignId: w.campaign, Kind: charactersv1.CharacterKind_CHARACTER_KIND_PLAYER, Name: "Perita", Sheet: s,
	}))).GetCharacter()
	levelUp := func(class string, skills ...string) reply {
		return w.ana.call(charactersv1connect.CharacterServiceLevelUpCharacterProcedure, &charactersv1.LevelUpCharacterRequest{
			CampaignId: w.campaign, CharacterId: apart.GetId(), Revision: apart.GetRevision(), Choices: &charactersv1.LevelUpChoices{
				ClassKey: class, SkillProficiencyKeys: skills,
				HitPoints: &charactersv1.LevelUpHitPoints{Method: charactersv1.LevelUpHitPointsMethod_LEVEL_UP_HIT_POINTS_METHOD_AVERAGE},
			},
		})
	}
	for _, how := range []string{"switched off", "archived"} {
		if how == "archived" {
			must(w.master.table.SetOptionSwitches(ctx, rq(&rulesv1.SetOptionSwitchesRequest{CampaignId: w.campaign, Switches: []*rulesv1.OptionSwitch{{Key: key, Off: false}}})))
			must(w.master.table.ArchiveTableEntry(ctx, rq(&rulesv1.ArchiveTableEntryRequest{CampaignId: w.campaign, Key: key})))
		} else {
			must(w.master.table.SetOptionSwitches(ctx, rq(&rulesv1.SetOptionSwitchesRequest{CampaignId: w.campaign, Switches: []*rulesv1.OptionSwitch{{Key: key, Off: true}}})))
		}
		// A class the table keeps out is refused on the options and on the level-up as main refuses it, whatever the pick: the
		// skill its list holds, one it does not, and none. The answer names the class the caller sent and no skill.
		reason := map[string]string{"switched off": "SWITCHED_OFF_CHOICE", "archived": "ARCHIVED_CHOICE"}[how]
		refused := func(what string, got reply) {
			t.Helper()
			body := string(got.body)
			if got.ok() || got.status != 400 || !strings.Contains(body, reason) || strings.Contains(body, extra) {
				t.Errorf("%s: %s is answered %d %s, not the refusal %s of the class", how, what, got.status, shorten(got.body), reason)
			}
		}
		refused("the options", options(w.ana, apart.GetId(), key))
		none := levelUp(key)
		refused("the level-up with no pick", none)
		for name, picks := range map[string][]string{"a pick the list holds": {extra}, "a pick the list lacks": {"skill:stealth"}} {
			got := levelUp(key, picks...)
			refused("the level-up with "+name, got)
			if !bytes.Equal(got.body, none.body) {
				t.Errorf("%s: the level-up with %s is answered %s, with no pick %s", how, name, shorten(got.body), shorten(none.body))
			}
		}
	}
}
