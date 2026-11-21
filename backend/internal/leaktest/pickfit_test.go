package leaktest

import (
	"strings"
	"testing"

	"google.golang.org/protobuf/encoding/protojson"

	campaignsv1 "github.com/PuraFome/meuRPG/backend/gen/meurpg/campaigns/v1"
	charactersv1 "github.com/PuraFome/meuRPG/backend/gen/meurpg/characters/v1"
	"github.com/PuraFome/meuRPG/backend/gen/meurpg/characters/v1/charactersv1connect"
	rulesv1 "github.com/PuraFome/meuRPG/backend/gen/meurpg/rules/v1"
	"github.com/PuraFome/meuRPG/backend/internal/rules"
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
	narrowed := &rulesv1.UpdateTableEntryRequest{
		CampaignId: w.campaign, Key: race.GetKey(), ExpectedRevision: race.GetRevision(),
		Body: &rulesv1.UpdateTableEntryRequest_TableRace{TableRace: fitRace("skill:perception")},
	}
	must(w.master.table.UpdateTableEntry(ctx, rq(narrowed)))

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
	for what, read := range reads {
		listsNoIssues := what == "the characters list" || what == "the level-up options"
		masterReadsIt := false
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
		if !masterReadsIt && !listsNoIssues {
			t.Errorf("the master reads the fit issue on %s: no", what)
		}
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
	must(w.master.campaigns.SetCampaignXpMode(ctx, rq(&campaignsv1.SetCampaignXpModeRequest{CampaignId: w.campaign, XpMode: campaignsv1.XpMode_XP_MODE_ENEMIES})))
	own := tableClass("Escola dos Rios")
	own.Levels[2].Features = []*rulesv1.TableFeature{{
		NamePt: "Estudo do rio", DescPt: []string{"Texto de teste."},
		Effects: []*rulesv1.TableEffect{{Type: "choice", Choice: "skill", Count: 1, From: []string{"skill:history"}}},
	}}
	class := p.createClass(own)
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

	for _, who := range p.readers() {
		if !who.member {
			continue
		}
		r := who.person.call(charactersv1connect.CharacterServiceGetLevelUpOptionsProcedure, &charactersv1.GetLevelUpOptionsRequest{CampaignId: w.campaign, CharacterId: pc.GetId()})
		if who.person != w.master && who.person != w.ana {
			continue // another reader gets no level-up of this character
		}
		if !r.ok() {
			t.Fatalf("%s: HTTP %d %s", who.name, r.status, shorten(r.body))
		}
		var res charactersv1.GetLevelUpOptionsResponse
		if err := protojson.Unmarshal(r.body, &res); err != nil {
			t.Fatalf("%s: %v", who.name, err)
		}
		opts := res.GetOptions()
		if len(opts.GetSkillSources()) != 1 || opts.GetSkillSources()[0].GetNamePt() != "Estudo do rio" || opts.GetSkillSources()[0].GetFrom()[0].GetKey() != "skill:history" {
			t.Errorf("%s: the level's skill sources = %v, want the class feature's", who.name, opts.GetSkillSources())
		}
		text := protojson.Format(opts)
		named := strings.Contains(text, sub.GetKey()) || strings.Contains(text, canary)
		if who.person == w.master {
			if !named || len(opts.GetSubclasses()) == 0 || len(opts.GetSubclasses()[0].GetSkillSources()) != 1 {
				t.Fatalf("the master reads %s, want the subclass with its skill source", shorten(r.body))
			}
			src := opts.GetSubclasses()[0].GetSkillSources()[0]
			if src.GetNamePt() != canary+" estudo" || src.GetCount() != 1 || len(src.GetFrom()) != 1 || src.GetFrom()[0].GetKey() != "skill:stealth" || src.GetFrom()[0].GetNamePt() != "Furtividade" {
				t.Errorf("the master's skill source = %v", src)
			}
			continue
		}
		if named {
			t.Errorf("%s: the options list the entry %q: %s", who.name, canary, shorten(r.body))
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

// A tool pick whose value is the key of an entry the table keeps out is answered as the closed entry it is, as main answers it,
// on create, update and preview, and the answer does not name the entry. A tool pick of a language, which the sheet takes as a
// proficiency but no tool choice offers, is answered as a key that does not exist. (A skill or an expertise of an entry's key is
// an unknown key on main too, for any entry, so no row of it can fail without this branch.)
func TestAPickOfAnEntryKeyIsAnsweredAsAKeyThatDoesNotExist(t *testing.T) {
	p := newChoicesProbe(t)
	w, ctx := p.w, t.Context()
	const canary = "Zorvak4321"
	class := p.createClass(tableClass("Escola dos Rios"))
	feat := func(name string) *rulesv1.TableEntry {
		return must(w.master.table.CreateTableEntry(ctx, rq(&rulesv1.CreateTableEntryRequest{CampaignId: w.campaign, Body: &rulesv1.CreateTableEntryRequest_TableFeat{TableFeat: &rulesv1.TableFeat{
			NamePt: name, DescPt: []string{"Texto de teste."},
		}}}))).GetEntry()
	}
	off, archived := feat(canary+" dádiva"), feat(canary+" bênção")
	must(w.master.table.SetOptionSwitches(ctx, rq(&rulesv1.SetOptionSwitchesRequest{CampaignId: w.campaign, Switches: []*rulesv1.OptionSwitch{{Key: off.GetKey(), Off: true}}})))
	must(w.master.table.ArchiveTableEntry(ctx, rq(&rulesv1.ArchiveTableEntryRequest{CampaignId: w.campaign, Key: archived.GetKey()})))
	const absent = "feat:nao-existe-zz@mesa"

	base := func() *charactersv1.CharacterSheet {
		s := sheet(class, 1)
		s.GetFull().RaceKey, s.GetFull().SubraceKey = "race:dwarf", "subrace:hill-dwarf"
		return s
	}
	toolPick := func(key string) *charactersv1.CharacterSheet {
		s := base()
		s.GetFull().FeatureChoiceKeys = []string{rules.ScopedChoice(rules.ToolPickKey("trait:tool-proficiency"), key)}
		return s
	}
	held := must(w.ana.characters.CreateCharacter(ctx, rq(&charactersv1.CreateCharacterRequest{
		CampaignId: w.campaign, Kind: charactersv1.CharacterKind_CHARACTER_KIND_PLAYER, Name: "Anã", Sheet: base(),
	}))).GetCharacter()
	callsOf := func(build func(key string) *charactersv1.CharacterSheet) map[string]func(who *person, key string) reply {
		return map[string]func(who *person, key string) reply{
			"create": func(who *person, key string) reply {
				return who.call(charactersv1connect.CharacterServiceCreateCharacterProcedure, &charactersv1.CreateCharacterRequest{
					CampaignId: w.campaign, Kind: charactersv1.CharacterKind_CHARACTER_KIND_PLAYER, Name: "Nova", Sheet: build(key),
				})
			},
			"preview": func(who *person, key string) reply {
				return who.call(charactersv1connect.CharacterServicePreviewCharacterProcedure, &charactersv1.PreviewCharacterRequest{
					CampaignId: w.campaign, Kind: charactersv1.CharacterKind_CHARACTER_KIND_PLAYER, Sheet: build(key),
				})
			},
			"update": func(who *person, key string) reply {
				return who.call(charactersv1connect.CharacterServiceUpdateCharacterProcedure, &charactersv1.UpdateCharacterRequest{
					CampaignId: w.campaign, CharacterId: held.GetId(), Revision: held.GetRevision(), Name: held.GetName(), Sheet: build(key),
				})
			},
		}
	}
	asker := func(call string) *person {
		if call == "create" {
			return w.caio // Ana has her character already
		}
		return w.ana
	}
	for call, send := range callsOf(toolPick) {
		for closing, entry := range map[string]*rulesv1.TableEntry{"CHARACTER_BLOCKED_REASON_SWITCHED_OFF_CONTENT": off, "CHARACTER_BLOCKED_REASON_ARCHIVED_CONTENT": archived} {
			named := send(asker(call), entry.GetKey())
			if strings.Contains(string(named.body), canary) {
				t.Errorf("a tool pick, %s: the answer names the entry: %s", call, shorten(named.body))
			}
			// The key is a pick of the sheet that the table keeps out: the answer is the closed choice's, whatever the picks.
			if named.status != 400 || !strings.Contains(string(named.body), closing) || !strings.Contains(string(named.body), entry.GetKey()) {
				t.Errorf("a tool pick, %s: the entry's key: HTTP %d %s, want the refusal of a closed choice %s", call, named.status, shorten(named.body), closing)
			}
		}
	}
	// A language is a proficiency the sheet takes, but no tool choice offers it: it is answered as a key that does not exist.
	const language = "language:elvish"
	for call, send := range callsOf(toolPick) {
		named, none := send(asker(call), language), send(asker(call), absent)
		if none.ok() || !sameAnswer(named, language, none, absent) {
			t.Errorf("a tool pick of a language, %s: HTTP %d %s\na key that does not exist: HTTP %d %s", call, named.status, shorten(named.body), none.status, shorten(none.body))
		}
	}
}
