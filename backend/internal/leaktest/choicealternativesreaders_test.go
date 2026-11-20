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
// off afterwards: the character that holds it keeps reading it (RN-23), and the master reads it too. A player who does not
// hold it reads nothing of it in the entries the table lists or in the level-up options of a character of her own (the
// menu of classes to take), and a sheet of hers that names the class is answered as a sheet that names a class that does not
// exist, in the choices preview and in the update. Each of those rows has its positive control (the holder and the master
// read the class there), so it fails when the table does not switch the class off.
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
	pick := firstKey + "=skill:stealth"                                                               // what the holder stores
	held := func() *charactersv1.CharacterSheet {
		s := sheet(classKey, 1, pick)
		s.GetFull().ExperiencePoints = 6500 // enough for the next level, in a campaign that counts XP
		return s
	}
	holder := must(w.ana.characters.CreateCharacter(ctx, rq(&charactersv1.CreateCharacterRequest{
		CampaignId: w.campaign, Kind: charactersv1.CharacterKind_CHARACTER_KIND_PLAYER, Name: "Cantora", Sheet: held(),
	}))).GetCharacter()
	base := p.createClass(tableClass("Guardião Base"))
	ownSheet := func() *charactersv1.CharacterSheet {
		s := sheet(base, 1)
		s.GetFull().ExperiencePoints = 6500
		return s
	}
	own := must(w.caio.characters.CreateCharacter(ctx, rq(&charactersv1.CreateCharacterRequest{
		CampaignId: w.campaign, Kind: charactersv1.CharacterKind_CHARACTER_KIND_PLAYER, Name: "Cantor", Sheet: ownSheet(),
	}))).GetCharacter()
	switchOff(t, p, classKey)

	// What only the holder and the master read: the character itself and the level-up that asks the choice of the next level.
	for _, who := range []reader{{person: w.ana, kind: charactersv1.CharacterKind_CHARACTER_KIND_PLAYER}, {person: w.master, kind: charactersv1.CharacterKind_CHARACTER_KIND_ENEMY}} {
		got := who.person.call(charactersv1connect.CharacterServiceGetCharacterProcedure, &charactersv1.GetCharacterRequest{CampaignId: w.campaign, CharacterId: holder.GetId()})
		if !got.ok() || !strings.Contains(string(got.body), classKey) || !strings.Contains(string(got.body), firstKey) {
			t.Errorf("the character, for a reader who may read the class: HTTP %d, it does not carry the class and its pick: %s", got.status, shorten(got.body))
		}
		up := who.person.call(charactersv1connect.CharacterServicePreviewLevelUpProcedure, &charactersv1.PreviewLevelUpRequest{
			CampaignId: w.campaign, CharacterId: holder.GetId(), Choices: &charactersv1.LevelUpChoices{
				ClassKey: classKey, HitPoints: &charactersv1.LevelUpHitPoints{Method: charactersv1.LevelUpHitPointsMethod_LEVEL_UP_HIT_POINTS_METHOD_AVERAGE},
			},
		})
		if !up.ok() || !strings.Contains(string(up.body), "LEVEL_UP_REFUSAL_REASON_FEATURE_CHOICE") {
			t.Errorf("the level-up preview does not ask the choice of the next level: HTTP %d %s", up.status, shorten(up.body))
		}
	}

	// A player reads none of it where a reader of her own is asked: the entries the table lists, and the menu of classes in the
	// level-up options of her own character. The master reads the class there.
	menu := func(who reader) reply {
		return who.person.call(charactersv1connect.CharacterServiceGetLevelUpOptionsProcedure, &charactersv1.GetLevelUpOptionsRequest{CampaignId: w.campaign, CharacterId: own.GetId()})
	}
	list := func(who reader) reply {
		return who.person.call(rulesv1connect.TableContentServiceListTableEntriesProcedure, &rulesv1.ListTableEntriesRequest{CampaignId: w.campaign})
	}
	for what, read := range map[string]func(who reader) reply{"the level-up options of a character of her own": menu, "the entries the table lists": list} {
		for _, who := range p.readers() {
			if !who.member {
				continue
			}
			r := read(who)
			if who.person == w.master {
				if !strings.Contains(string(r.body), classKey) {
					t.Errorf("the master reads %s without the class: HTTP %d %s", what, r.status, shorten(r.body))
				}
				continue
			}
			if what == "the level-up options of a character of her own" && who.person != w.caio {
				continue // the character is Caio's: the others are not answered with it
			}
			for _, needle := range hidden {
				if strings.Contains(string(r.body), needle) {
					t.Errorf("%s reads %s: it holds %q of a class the table switched off", who.name, what, needle)
				}
			}
		}
	}
	if r := menu(reader{person: w.caio}); !r.ok() {
		t.Errorf("the level-up options of Caio's character: HTTP %d %s", r.status, shorten(r.body))
	}

	// A sheet of her own that names the class is refused with the answer for switched off content, that names the class by the key
	// she sent and by nothing else. For someone outside the campaign it is the answer for a campaign that does not exist.
	naming := map[string]func(who reader) reply{
		"the choices preview": func(who reader) reply { return p.preview(who, sheet(classKey, 1, firstKey+"=skill:stealth")) },
		"the update of her character": func(who reader) reply {
			s := ownSheet()
			s.GetFull().Classes = []*charactersv1.ClassLevel{{ClassKey: classKey, Level: 1}}
			return who.person.call(charactersv1connect.CharacterServiceUpdateCharacterProcedure, &charactersv1.UpdateCharacterRequest{
				CampaignId: w.campaign, CharacterId: own.GetId(), Revision: own.GetRevision(), Name: own.GetName(), Sheet: s,
			})
		},
	}
	for what, call := range naming {
		for _, who := range p.readers() {
			if who.person == w.master || (what == "the update of her character" && who.person != w.caio) {
				continue
			}
			r := call(who)
			switch {
			case !who.member && r.status != 404:
				t.Errorf("%s, %s: HTTP %d, want the answer for a campaign that does not exist", what, who.name, r.status)
			case who.member && (r.ok() || !strings.Contains(string(r.body), "CHARACTER_BLOCKED_REASON_SWITCHED_OFF_CONTENT")):
				t.Errorf("%s, %s: HTTP %d %s; want the refusal for switched off content", what, who.name, r.status, shorten(r.body))
			}
			if strings.Contains(string(r.body), name) {
				t.Errorf("%s, %s: the answer names %s", what, who.name, name)
			}
		}
	}
	if r := naming["the choices preview"](reader{person: w.master, kind: charactersv1.CharacterKind_CHARACTER_KIND_ENEMY}); !r.ok() {
		t.Errorf("the master's choices preview of the class: HTTP %d %s", r.status, shorten(r.body))
	}
}

// switchOff switches the entry off for the players.
func switchOff(t *testing.T, p *choicesProbe, key string) {
	t.Helper()
	must(p.w.master.table.SetOptionSwitches(t.Context(), rq(&rulesv1.SetOptionSwitchesRequest{CampaignId: p.w.campaign, Switches: []*rulesv1.OptionSwitch{{Key: key, Off: true}}})))
}
