package leaktest

import (
	"strings"
	"testing"

	"google.golang.org/protobuf/encoding/protojson"

	campaignsv1 "github.com/PuraFome/meuRPG/backend/gen/meurpg/campaigns/v1"
	charactersv1 "github.com/PuraFome/meuRPG/backend/gen/meurpg/characters/v1"
	"github.com/PuraFome/meuRPG/backend/gen/meurpg/characters/v1/charactersv1connect"
	rulesv1 "github.com/PuraFome/meuRPG/backend/gen/meurpg/rules/v1"
)

// fitRace is a table race whose trait lets the player choose one skill from a list.
func fitRace(from ...string) *rulesv1.TableRace {
	return &rulesv1.TableRace{
		NamePt: "Perito", Size: "Medium", SpeedFt: 30, AbilityBonuses: &rulesv1.AbilityScores{Constitution: 1},
		Traits: []*rulesv1.TableFeature{{NamePt: "Faro", Effects: []*rulesv1.TableEffect{{Type: "choice", Choice: "skill", Count: 1, From: from}}}},
	}
}

func fitSheet(class, race string, skills ...string) *charactersv1.CharacterSheet {
	s := sheet(class, 1)
	s.GetFull().RaceKey = race
	s.GetFull().SkillProficiencyKeys = skills
	return s
}

// A stored sheet whose picks the table's lists no longer fit is read with the issue by the master alone, on every read that
// carries a sheet's issues; its owner still saves it untouched and levels it up.
func TestOnlyTheMasterReadsTheFitIssueOfAStoredSheet(t *testing.T) {
	p := newChoicesProbe(t)
	w, ctx := p.w, t.Context()
	must(w.master.campaigns.SetCampaignXpMode(ctx, rq(&campaignsv1.SetCampaignXpModeRequest{CampaignId: w.campaign, XpMode: campaignsv1.XpMode_XP_MODE_ENEMIES})))
	class := p.createClass(tableClass("Escola dos Rios"))
	race := must(w.master.table.CreateTableEntry(ctx, rq(&rulesv1.CreateTableEntryRequest{CampaignId: w.campaign, Body: &rulesv1.CreateTableEntryRequest_TableRace{TableRace: fitRace("skill:stealth", "skill:perception")}}))).GetEntry()
	stored := fitSheet(class, race.GetKey(), "skill:arcana", "skill:history", "skill:stealth")
	stored.GetFull().ExperiencePoints = 6500
	pc := must(w.ana.characters.CreateCharacter(ctx, rq(&charactersv1.CreateCharacterRequest{
		CampaignId: w.campaign, Kind: charactersv1.CharacterKind_CHARACTER_KIND_PLAYER, Name: "Perita", Sheet: stored,
	}))).GetCharacter()
	// The table narrows the list: the held pick is in no list now.
	must(w.master.table.UpdateTableEntry(ctx, rq(&rulesv1.UpdateTableEntryRequest{CampaignId: w.campaign, Key: race.GetKey(), ExpectedRevision: race.GetRevision(),
		Body: &rulesv1.UpdateTableEntryRequest_TableRace{TableRace: fitRace("skill:perception")}})))

	levelUp := &charactersv1.PreviewLevelUpRequest{CampaignId: w.campaign, CharacterId: pc.GetId(), Choices: &charactersv1.LevelUpChoices{
		ClassKey: class, HitPoints: &charactersv1.LevelUpHitPoints{Method: charactersv1.LevelUpHitPointsMethod_LEVEL_UP_HIT_POINTS_METHOD_AVERAGE},
	}}
	reads := map[string]func(who reader) reply{
		"the character": func(who reader) reply {
			return who.person.call(charactersv1connect.CharacterServiceGetCharacterProcedure, &charactersv1.GetCharacterRequest{CampaignId: w.campaign, CharacterId: pc.GetId()})
		},
		"the characters list": func(who reader) reply {
			return who.person.call(charactersv1connect.CharacterServiceListCharactersProcedure, &charactersv1.ListCharactersRequest{CampaignId: w.campaign})
		},
		"the preview of the sheet": func(who reader) reply {
			return who.person.call(charactersv1connect.CharacterServicePreviewCharacterProcedure, &charactersv1.PreviewCharacterRequest{CampaignId: w.campaign, CharacterId: pc.GetId(), Kind: who.kind, Sheet: stored})
		},
		"the level-up options": func(who reader) reply {
			return who.person.call(charactersv1connect.CharacterServiceGetLevelUpOptionsProcedure, &charactersv1.GetLevelUpOptionsRequest{CampaignId: w.campaign, CharacterId: pc.GetId()})
		},
		"the level-up preview": func(who reader) reply {
			return who.person.call(charactersv1connect.CharacterServicePreviewLevelUpProcedure, levelUp)
		},
	}
	const marker = "não cabe em nenhuma das listas"
	masterReadsIt := false
	for what, read := range reads {
		for _, who := range p.readers() {
			if !who.member {
				continue
			}
			body := string(read(who).body)
			has := strings.Contains(body, marker) || strings.Contains(body, "pick_fit")
			switch {
			case who.person == w.master:
				masterReadsIt = masterReadsIt || has
			case has:
				i := strings.Index(body, "pick_fit")
				if i < 0 {
					i = strings.Index(body, marker)
				}
				t.Errorf("%s reads %s: it carries the fit issue: ...%s", who.name, what, body[max(i-200, 0):min(i+300, len(body))])
			}
		}
	}
	if !masterReadsIt {
		t.Error("the master reads the fit issue on none of the reads")
	}

	// The owner saves the sheet untouched under another name and levels it up.
	got := must(w.ana.characters.GetCharacter(ctx, rq(&charactersv1.GetCharacterRequest{CampaignId: w.campaign, CharacterId: pc.GetId()}))).GetCharacter()
	saved := must(w.ana.characters.UpdateCharacter(ctx, rq(&charactersv1.UpdateCharacterRequest{
		CampaignId: w.campaign, CharacterId: pc.GetId(), Revision: got.GetRevision(), Name: "Perita do Rio", Sheet: got.GetSheet(),
	}))).GetCharacter()
	if _, err := w.ana.characters.LevelUpCharacter(ctx, rq(&charactersv1.LevelUpCharacterRequest{
		CampaignId: w.campaign, CharacterId: pc.GetId(), Revision: saved.GetRevision(), Choices: levelUp.GetChoices(),
	})); err != nil {
		t.Fatalf("the owner's level-up of the sheet: %v", err)
	}
}

// The skill sources of a level and the refusal of a pick name nothing the table keeps out of the reader's offer: a subclass
// switched off for the players reaches no player's skill sources, by key or by name, and the sentence of a refused pick holds
// only the pick's own name.
func TestTheSkillSourcesOfALevelNameNothingTheTableKeepsOut(t *testing.T) {
	p := newChoicesProbe(t)
	w, ctx := p.w, t.Context()
	const canary = "Zorvak4321"
	class := p.createClass(tableClass("Escola dos Rios"))
	sub := must(w.master.table.CreateTableEntry(ctx, rq(&rulesv1.CreateTableEntryRequest{CampaignId: w.campaign, Body: &rulesv1.CreateTableEntryRequest_TableSubclass{TableSubclass: &rulesv1.TableSubclass{
		NamePt: canary, ClassKey: class,
		Levels: []*rulesv1.TableSubclassLevel{{Level: 3, Features: []*rulesv1.TableFeature{{
			NamePt: canary + " estudo", DescPt: []string{"Texto de teste."},
			Effects: []*rulesv1.TableEffect{{Type: "choice", Choice: "skill", Count: 1, From: []string{"skill:stealth"}}},
		}}}},
	}}}))).GetEntry()
	s := fitSheet(class, "race:human", "skill:arcana", "skill:history")
	s.GetFull().Classes[0].Level = 2
	s.GetFull().ExperiencePoints = 6500
	pc := must(w.ana.characters.CreateCharacter(ctx, rq(&charactersv1.CreateCharacterRequest{
		CampaignId: w.campaign, Kind: charactersv1.CharacterKind_CHARACTER_KIND_PLAYER, Name: "Perita", Sheet: s,
	}))).GetCharacter()
	must(w.master.table.SetOptionSwitches(ctx, rq(&rulesv1.SetOptionSwitchesRequest{CampaignId: w.campaign, Switches: []*rulesv1.OptionSwitch{{Key: sub.GetKey(), Off: true}}})))

	options := func(who reader) reply {
		return who.person.call(charactersv1connect.CharacterServiceGetLevelUpOptionsProcedure, &charactersv1.GetLevelUpOptionsRequest{CampaignId: w.campaign, CharacterId: pc.GetId()})
	}
	for _, who := range p.readers() {
		if !who.member {
			continue
		}
		r := options(who)
		if !r.ok() {
			continue
		}
		var res charactersv1.GetLevelUpOptionsResponse
		if err := protojson.Unmarshal(r.body, &res); err != nil {
			t.Fatalf("%s: %v", who.name, err)
		}
		text := protojson.Format(res.GetOptions())
		named := strings.Contains(text, sub.GetKey()) || strings.Contains(text, canary)
		if who.person == w.master {
			if !named || len(res.GetOptions().GetSubclasses()) == 0 || len(res.GetOptions().GetSubclasses()[0].GetSkillSources()) != 1 {
				t.Errorf("the master reads %s, want the subclass with its skill source", shorten(r.body))
			}
			src := res.GetOptions().GetSubclasses()[0].GetSkillSources()[0]
			if src.GetNamePt() != canary+" estudo" || src.GetCount() != 1 || len(src.GetFrom()) != 1 || src.GetFrom()[0].GetKey() != "skill:stealth" || src.GetFrom()[0].GetNamePt() != "Furtividade" {
				t.Errorf("the master's skill source = %v", src)
			}
			continue
		}
		if named {
			t.Errorf("%s reads the level-up options: they name the subclass the table keeps out: %s", who.name, shorten(r.body))
		}
	}

	// A pick no list allows is refused with the pick's own name alone.
	got := must(w.ana.characters.GetCharacter(ctx, rq(&charactersv1.GetCharacterRequest{CampaignId: w.campaign, CharacterId: pc.GetId()}))).GetCharacter()
	bad := got.GetSheet()
	bad.GetFull().SkillProficiencyKeys = append(bad.GetFull().SkillProficiencyKeys, "skill:stealth")
	r := w.ana.call(charactersv1connect.CharacterServiceUpdateCharacterProcedure, &charactersv1.UpdateCharacterRequest{
		CampaignId: w.campaign, CharacterId: pc.GetId(), Revision: got.GetRevision(), Name: got.GetName(), Sheet: bad,
	})
	body := string(r.body)
	if r.ok() || !strings.Contains(body, "Furtividade") || strings.Contains(body, canary) || strings.Contains(body, sub.GetKey()) {
		t.Errorf("the refusal of the pick: HTTP %d %s", r.status, shorten(r.body))
	}
}
