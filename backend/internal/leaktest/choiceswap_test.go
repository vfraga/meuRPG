package leaktest

import (
	"strings"
	"testing"

	campaignsv1 "github.com/PuraFome/meuRPG/backend/gen/meurpg/campaigns/v1"
	charactersv1 "github.com/PuraFome/meuRPG/backend/gen/meurpg/characters/v1"
	"github.com/PuraFome/meuRPG/backend/gen/meurpg/characters/v1/charactersv1connect"
	rulesv1 "github.com/PuraFome/meuRPG/backend/gen/meurpg/rules/v1"
)

// The swap of the rule "Trocar uma escolha no aumento de atributo" takes its pool from the options that the features
// offering a pick have; a choice that offers another feature's options (from_feature) holds picks of that pool. An option the
// table switched off is no swap for a player, answered as a key that does not exist, and the pool a player reads names
// nothing of it; the master may swap to it, as it may pick it.
func TestASwapToAnOptionTheTableSwitchedOffIsAnsweredAsAKeyThatDoesNotExist(t *testing.T) {
	p := newChoicesProbe(t)
	w, ctx := p.w, t.Context()
	must(w.master.campaigns.SetCampaignXpMode(ctx, rq(&campaignsv1.SetCampaignXpModeRequest{CampaignId: w.campaign, XpMode: campaignsv1.XpMode_XP_MODE_ENEMIES})))
	rules := must(w.master.campaigns.GetTableRules(ctx, rq(&campaignsv1.GetTableRulesRequest{CampaignId: w.campaign}))).GetRules()
	rules.PickSwapAllowed = true
	must(w.master.campaigns.SetTableRules(ctx, rq(&campaignsv1.SetTableRulesRequest{CampaignId: w.campaign, Rules: rules})))

	const style, held = "feature:fighter-fighting-style", "feature:fighter-fighting-style-archery"
	guard := must(w.master.table.CreateTableEntry(ctx, rq(&rulesv1.CreateTableEntryRequest{
		CampaignId: w.campaign, Body: &rulesv1.CreateTableEntryRequest_TableOption{TableOption: &rulesv1.TableOption{
			NamePt: "Guarda Firme", DescPt: []string{"Texto de teste."}, Set: "fighting_style",
		}},
	}))).GetEntry().GetKey()
	asking := tableClass("Cantor das Marés")
	asking.SubclassLevel = 5
	asking.Levels[1].Features = []*rulesv1.TableFeature{{
		NamePt: "Eco das marés", DescPt: []string{"Texto de teste."},
		Effects: []*rulesv1.TableEffect{{Type: "choice", Choice: "feature", Count: 1, FromFeature: style}},
	}}
	class := p.createEntry(t, asking).GetKey()
	draft := func(picks ...string) *charactersv1.CharacterSheet {
		s := sheet(class, 3, picks...)
		s.GetFull().ExperiencePoints = 6500
		return s
	}
	holder := must(w.ana.characters.CreateCharacter(ctx, rq(&charactersv1.CreateCharacterRequest{
		CampaignId: w.campaign, Kind: charactersv1.CharacterKind_CHARACTER_KIND_PLAYER, Name: "Cantora", Sheet: draft(held),
	}))).GetCharacter()
	swap := func(who reader, to string) reply {
		return who.person.call(charactersv1connect.CharacterServicePreviewLevelUpProcedure, &charactersv1.PreviewLevelUpRequest{
			CampaignId: w.campaign, CharacterId: holder.GetId(), Choices: &charactersv1.LevelUpChoices{
				ClassKey: class, AbilityIncrease: &rulesv1.AbilityScores{Strength: 2},
				HitPoints: &charactersv1.LevelUpHitPoints{Method: charactersv1.LevelUpHitPointsMethod_LEVEL_UP_HIT_POINTS_METHOD_AVERAGE},
				SwapPick:  &charactersv1.LevelUpPickSwap{Group: "fighting_style", FromKey: held, ToKey: to},
			},
		})
	}
	pool := func() reply {
		return w.ana.call(charactersv1connect.CharacterServiceGetLevelUpOptionsProcedure, &charactersv1.GetLevelUpOptionsRequest{CampaignId: w.campaign, CharacterId: holder.GetId()})
	}
	if r := pool(); !r.ok() || !strings.Contains(string(r.body), guard) {
		t.Fatalf("before the table switches the option off: HTTP %d, want the swap pool to name %s: %s", r.status, guard, shorten(r.body))
	}
	if r := swap(p.readers()[1], guard); !r.ok() {
		t.Fatalf("the swap to %s before it is switched off: HTTP %d %s", guard, r.status, shorten(r.body))
	}

	must(w.master.table.SetOptionSwitches(ctx, rq(&rulesv1.SetOptionSwitchesRequest{CampaignId: w.campaign, Switches: []*rulesv1.OptionSwitch{{Key: guard, Off: true}}})))

	if r := pool(); !r.ok() || strings.Contains(string(r.body), guard) || strings.Contains(string(r.body), "Guarda Firme") {
		t.Errorf("the pool a player reads: HTTP %d, want it to name nothing of the option: %s", r.status, shorten(r.body))
	}
	for _, who := range p.readers() {
		if !who.member {
			continue
		}
		named, absent := swap(who, guard), swap(who, absentKey)
		if who.person == w.master {
			if !named.ok() {
				t.Errorf("%s: the swap to %s: HTTP %d %s; want it taken", who.name, guard, named.status, shorten(named.body))
			}
			continue
		}
		if !sameAnswer(named, guard, absent, absentKey) {
			t.Errorf("%s: the swap to %s: HTTP %d %s\nthe swap to a key that does not exist: HTTP %d %s", who.name, guard, named.status, shorten(named.body), absent.status, shorten(absent.body))
		}
	}
}
