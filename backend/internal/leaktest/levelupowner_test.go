package leaktest

import (
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
	for _, how := range []string{"switched off", "archived"} {
		for _, k := range []string{key} {
			if how == "archived" {
				must(w.master.table.SetOptionSwitches(ctx, rq(&rulesv1.SetOptionSwitchesRequest{CampaignId: w.campaign, Switches: []*rulesv1.OptionSwitch{{Key: k, Off: false}}})))
				must(w.master.table.ArchiveTableEntry(ctx, rq(&rulesv1.ArchiveTableEntryRequest{CampaignId: w.campaign, Key: k})))
			} else {
				must(w.master.table.SetOptionSwitches(ctx, rq(&rulesv1.SetOptionSwitchesRequest{CampaignId: w.campaign, Switches: []*rulesv1.OptionSwitch{{Key: k, Off: true}}})))
			}
		}
		if body := string(options(w.ana, apart.GetId(), key).body); strings.Contains(body, canary) || strings.Contains(body, extra) {
			t.Errorf("%s: Ana's options for a class she does not hold name it: %s", how, shorten([]byte(body)))
		}
	}
}
