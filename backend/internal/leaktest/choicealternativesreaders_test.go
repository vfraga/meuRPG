package leaktest

import (
	"strings"
	"testing"

	campaignsv1 "github.com/PuraFome/meuRPG/backend/gen/meurpg/campaigns/v1"
	charactersv1 "github.com/PuraFome/meuRPG/backend/gen/meurpg/characters/v1"
	"github.com/PuraFome/meuRPG/backend/gen/meurpg/characters/v1/charactersv1connect"
	rulesv1 "github.com/PuraFome/meuRPG/backend/gen/meurpg/rules/v1"
	"github.com/PuraFome/meuRPG/backend/gen/meurpg/rules/v1/rulesv1connect"
	"github.com/PuraFome/meuRPG/backend/internal/rules"
)

// A class that carries a choice of alternatives (a skill or a language, at level 1 and at level 2) that the table switches
// off afterwards: the character that holds it keeps reading it (RN-23), the master reads it too, and nobody else reads the
// class, its features or the key of the choice, in the character, the list of the characters, the level-up options, the
// level-up preview, the choices of the sheet and the entries the table lists.
func TestAChoiceOfAlternativesOfAClassTheTableSwitchedOffIsReadByItsHolderAndTheMasterAlone(t *testing.T) {
	const name = "Zorvak4321"
	p := newChoicesProbe(t)
	w, ctx := p.w, t.Context()
	must(w.master.campaigns.SetCampaignXpMode(ctx, rq(&campaignsv1.SetCampaignXpModeRequest{CampaignId: w.campaign, XpMode: campaignsv1.XpMode_XP_MODE_ENEMIES})))
	effect := func(skills ...string) []*rulesv1.TableEffect {
		return []*rulesv1.TableEffect{{Type: "choice", Choice: "alternatives", Count: 1, Alternatives: []*rulesv1.ChoiceAlternative{
			{Kind: "skill", From: skills},
			{Kind: "language", From: []string{"language:elvish", "language:dwarvish"}},
		}}}
	}
	class := tableClass(name, &rulesv1.TableFeature{NamePt: name + " primeira", DescPt: []string{"Texto."}, Effects: effect("skill:stealth", "skill:persuasion")})
	class.Levels[1].Features = []*rulesv1.TableFeature{{NamePt: name + " segunda", DescPt: []string{"Texto."}, Effects: effect("skill:insight", "skill:deception")}}
	made := p.createEntry(t, class)
	classKey := made.GetKey()
	first := made.GetTableClass().GetLevels()[0].GetFeatures()[0].GetKey()
	second := made.GetTableClass().GetLevels()[1].GetFeatures()[0].GetKey()
	firstKey := rules.AlternativesKey(first, &rules.Effect{Count: 1, Alternatives: []rules.ChoiceAlternative{
		{Kind: "skill", From: []string{"skill:stealth", "skill:persuasion"}}, {Kind: "language", From: []string{"language:elvish", "language:dwarvish"}},
	}})
	secondKey := rules.AlternativesKey(second, &rules.Effect{Count: 1, Alternatives: []rules.ChoiceAlternative{
		{Kind: "skill", From: []string{"skill:insight", "skill:deception"}}, {Kind: "language", From: []string{"language:elvish", "language:dwarvish"}},
	}})
	hidden := []string{classKey, first, second, firstKey[len(first):], secondKey[len(second):], name} // what only a holder and the master may read
	pick := firstKey + "=skill:stealth"
	held := func() *charactersv1.CharacterSheet {
		s := sheet(classKey, 1, pick)
		s.GetFull().ExperiencePoints = 6500 // enough for the next level, in a campaign that counts XP
		return s
	}
	holder := must(w.ana.characters.CreateCharacter(ctx, rq(&charactersv1.CreateCharacterRequest{
		CampaignId: w.campaign, Kind: charactersv1.CharacterKind_CHARACTER_KIND_PLAYER, Name: "Cantora", Sheet: held(),
	}))).GetCharacter()
	must(w.master.table.SetOptionSwitches(ctx, rq(&rulesv1.SetOptionSwitchesRequest{CampaignId: w.campaign, Switches: []*rulesv1.OptionSwitch{{Key: classKey, Off: true}}})))

	reads := map[string]func(who reader) reply{
		"the character": func(who reader) reply {
			return who.person.call(charactersv1connect.CharacterServiceGetCharacterProcedure, &charactersv1.GetCharacterRequest{CampaignId: w.campaign, CharacterId: holder.GetId()})
		},
		"the characters list": func(who reader) reply {
			return who.person.call(charactersv1connect.CharacterServiceListCharactersProcedure, &charactersv1.ListCharactersRequest{CampaignId: w.campaign})
		},
		"the level-up options": func(who reader) reply {
			return who.person.call(charactersv1connect.CharacterServiceGetLevelUpOptionsProcedure, &charactersv1.GetLevelUpOptionsRequest{CampaignId: w.campaign, CharacterId: holder.GetId()})
		},
		"the level-up preview": func(who reader) reply {
			return who.person.call(charactersv1connect.CharacterServicePreviewLevelUpProcedure, &charactersv1.PreviewLevelUpRequest{
				CampaignId: w.campaign, CharacterId: holder.GetId(), Choices: &charactersv1.LevelUpChoices{
					ClassKey: classKey, HitPoints: &charactersv1.LevelUpHitPoints{Method: charactersv1.LevelUpHitPointsMethod_LEVEL_UP_HIT_POINTS_METHOD_AVERAGE},
				},
			})
		},
		"the choices of the sheet": func(who reader) reply {
			return who.person.call(charactersv1connect.CharacterServicePreviewChoicesProcedure, &charactersv1.PreviewChoicesRequest{
				CampaignId: w.campaign, CharacterId: holder.GetId(), Kind: who.kind, Sheet: held(),
			})
		},
		"the entries the table lists": func(who reader) reply {
			return who.person.call(rulesv1connect.TableContentServiceListTableEntriesProcedure, &rulesv1.ListTableEntriesRequest{CampaignId: w.campaign})
		},
	}
	for what, read := range reads {
		for _, who := range p.readers() {
			if !who.member {
				continue
			}
			body := string(read(who).body)
			mayRead := who.person == w.master || (who.person == w.ana && what != "the entries the table lists")
			for _, needle := range hidden {
				if strings.Contains(body, needle) && !mayRead {
					t.Errorf("%s reads %s: it holds %q of a class the table switched off", who.name, what, needle)
				}
			}
		}
	}
	// The holder reads her pick on her sheet, and both read that the level-up asks the choice of the next level.
	for _, who := range []reader{{person: w.ana, kind: charactersv1.CharacterKind_CHARACTER_KIND_PLAYER}, {person: w.master, kind: charactersv1.CharacterKind_CHARACTER_KIND_ENEMY}} {
		if r := reads["the choices of the sheet"](who); !strings.Contains(string(r.body), firstKey) {
			t.Errorf("a reader who may read the class reads no choice of alternatives in the choices of the sheet: %s", shorten(r.body))
		}
		if r := reads["the level-up options"](who); !r.ok() {
			t.Errorf("the level-up options: HTTP %d %s", r.status, shorten(r.body))
		}
		if r := reads["the level-up preview"](who); !r.ok() || !strings.Contains(string(r.body), "LEVEL_UP_REFUSAL_REASON_FEATURE_CHOICE") {
			t.Errorf("the level-up preview does not ask the choice of the next level: HTTP %d %s", r.status, shorten(r.body))
		}
	}
}
