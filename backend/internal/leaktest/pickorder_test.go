package leaktest

import (
	"strings"
	"testing"

	campaignsv1 "github.com/PuraFome/meuRPG/backend/gen/meurpg/campaigns/v1"
	charactersv1 "github.com/PuraFome/meuRPG/backend/gen/meurpg/characters/v1"
	"github.com/PuraFome/meuRPG/backend/gen/meurpg/characters/v1/charactersv1connect"
	rulesv1 "github.com/PuraFome/meuRPG/backend/gen/meurpg/rules/v1"
)

// The order of the refusals on a new pick: a player whose draft names an entry the master switched off or archived, that the
// stored sheet does not hold, gets the answer of that entry (the sentence of the closed choice and its key) whatever skill the
// draft also picks, one the entry's own source allows or one no source allows. The expected answer is the one the call gives
// without the skill pick.
func TestTheClosedChoiceIsAnsweredBeforeTheFitOfANewPick(t *testing.T) {
	p := newChoicesProbe(t)
	w, ctx := p.w, t.Context()
	const inside, outside = "skill:performance", "skill:stealth"
	must(w.master.campaigns.SetCampaignXpMode(ctx, rq(&campaignsv1.SetCampaignXpModeRequest{CampaignId: w.campaign, XpMode: campaignsv1.XpMode_XP_MODE_ENEMIES})))
	rl := must(w.master.campaigns.GetTableRules(ctx, rq(&campaignsv1.GetTableRulesRequest{CampaignId: w.campaign}))).GetRules()
	rl.FeatsAllowed = true
	must(w.master.campaigns.SetTableRules(ctx, rq(&campaignsv1.SetTableRulesRequest{CampaignId: w.campaign, Rules: rl})))
	pick := &rulesv1.TableEffect{Type: "choice", Choice: "skill", Count: 1, From: []string{inside}}
	feature := func() *rulesv1.TableFeature {
		return &rulesv1.TableFeature{NamePt: "Canto", DescPt: []string{"Texto de teste."}, Effects: []*rulesv1.TableEffect{pick}}
	}
	create := func(body *rulesv1.CreateTableEntryRequest) *rulesv1.TableEntry {
		body.CampaignId = w.campaign
		res, err := w.master.table.CreateTableEntry(ctx, rq(body))
		if err != nil {
			t.Fatalf("create: %v", err)
		}
		return res.Msg.GetEntry()
	}
	namedRace := func(name string, from ...string) *rulesv1.TableRace {
		r := fitRace(from...)
		r.NamePt = name
		return r
	}
	type kind struct {
		name  string
		entry *rulesv1.TableEntry
		put   func(s *charactersv1.FullSheet, key string)
	}
	kinds := []kind{
		{"a class", create(&rulesv1.CreateTableEntryRequest{Body: &rulesv1.CreateTableEntryRequest_TableClass{TableClass: tableClass("Menestrel", feature())}}),
			func(s *charactersv1.FullSheet, key string) { s.Classes[0].ClassKey = key }},
		{"a race", create(&rulesv1.CreateTableEntryRequest{Body: &rulesv1.CreateTableEntryRequest_TableRace{TableRace: namedRace("Perito A", inside)}}),
			func(s *charactersv1.FullSheet, key string) { s.RaceKey = key }},
		{"a background", create(&rulesv1.CreateTableEntryRequest{Body: &rulesv1.CreateTableEntryRequest_TableBackground{TableBackground: &rulesv1.TableBackground{NamePt: "Artista", Skills: []string{"skill:insight", inside}, Feature: feature()}}}),
			func(s *charactersv1.FullSheet, key string) {
				s.Background = &charactersv1.FullSheet_BackgroundKey{BackgroundKey: key}
			}},
		{"a feat", create(&rulesv1.CreateTableEntryRequest{Body: &rulesv1.CreateTableEntryRequest_TableFeat{TableFeat: &rulesv1.TableFeat{NamePt: "Dom", DescPt: []string{"Texto de teste."}, Effects: []*rulesv1.TableEffect{pick}}}}),
			func(s *charactersv1.FullSheet, key string) { s.FeatKeys = []string{key} }},
	}
	// Each kind has an entry for each way to close: the first copy is switched off, the second archived.
	type closed struct {
		kind   kind
		how    string
		key    string
		reason string
		text   string
	}
	var all []closed
	for _, k := range kinds {
		off := k.entry
		dup := func() *rulesv1.TableEntry {
			switch k.name {
			case "a class":
				return create(&rulesv1.CreateTableEntryRequest{Body: &rulesv1.CreateTableEntryRequest_TableClass{TableClass: tableClass("Menestrel B", feature())}})
			case "a race":
				return create(&rulesv1.CreateTableEntryRequest{Body: &rulesv1.CreateTableEntryRequest_TableRace{TableRace: namedRace("Perito B", inside)}})
			case "a background":
				return create(&rulesv1.CreateTableEntryRequest{Body: &rulesv1.CreateTableEntryRequest_TableBackground{TableBackground: &rulesv1.TableBackground{NamePt: "Artista B", Skills: []string{"skill:insight", inside}, Feature: feature()}}})
			}
			return create(&rulesv1.CreateTableEntryRequest{Body: &rulesv1.CreateTableEntryRequest_TableFeat{TableFeat: &rulesv1.TableFeat{NamePt: "Dom B", DescPt: []string{"Texto de teste."}, Effects: []*rulesv1.TableEffect{pick}}}})
		}()
		must(w.master.table.SetOptionSwitches(ctx, rq(&rulesv1.SetOptionSwitchesRequest{CampaignId: w.campaign, Switches: []*rulesv1.OptionSwitch{{Key: off.GetKey(), Off: true}}})))
		must(w.master.table.ArchiveTableEntry(ctx, rq(&rulesv1.ArchiveTableEntryRequest{CampaignId: w.campaign, Key: dup.GetKey()})))
		all = append(all,
			closed{k, "switched off", off.GetKey(), "SWITCHED_OFF_CONTENT", "the master switched off an option this sheet picks: it is not a new choice for players"},
			closed{k, "archived", dup.GetKey(), "ARCHIVED_CONTENT", "the master archived a table entry this sheet picks: it is not a new choice any more"})
	}
	draft := func(c closed, skills ...string) *charactersv1.CharacterSheet {
		s := sheet("class:wizard", 1)
		s.GetFull().SkillProficiencyKeys = skills
		c.kind.put(s.GetFull(), c.key)
		return s
	}
	held := must(w.ana.characters.CreateCharacter(ctx, rq(&charactersv1.CreateCharacterRequest{
		CampaignId: w.campaign, Kind: charactersv1.CharacterKind_CHARACTER_KIND_PLAYER, Name: "Anã", Sheet: sheet("class:wizard", 1),
	}))).GetCharacter()
	calls := map[string]func(who *person, s *charactersv1.CharacterSheet) reply{
		"create": func(who *person, s *charactersv1.CharacterSheet) reply {
			return who.call(charactersv1connect.CharacterServiceCreateCharacterProcedure, &charactersv1.CreateCharacterRequest{
				CampaignId: w.campaign, Kind: charactersv1.CharacterKind_CHARACTER_KIND_PLAYER, Name: "Nova", Sheet: s,
			})
		},
		"preview": func(who *person, s *charactersv1.CharacterSheet) reply {
			return who.call(charactersv1connect.CharacterServicePreviewCharacterProcedure, &charactersv1.PreviewCharacterRequest{
				CampaignId: w.campaign, Kind: charactersv1.CharacterKind_CHARACTER_KIND_PLAYER, Sheet: s,
			})
		},
		"preview of the held": func(who *person, s *charactersv1.CharacterSheet) reply {
			return who.call(charactersv1connect.CharacterServicePreviewCharacterProcedure, &charactersv1.PreviewCharacterRequest{
				CampaignId: w.campaign, CharacterId: held.GetId(), Kind: charactersv1.CharacterKind_CHARACTER_KIND_PLAYER, Sheet: s,
			})
		},
		"update": func(who *person, s *charactersv1.CharacterSheet) reply {
			return who.call(charactersv1connect.CharacterServiceUpdateCharacterProcedure, &charactersv1.UpdateCharacterRequest{
				CampaignId: w.campaign, CharacterId: held.GetId(), Revision: held.GetRevision(), Name: held.GetName(), Sheet: s,
			})
		},
		"preview of the choices": func(who *person, s *charactersv1.CharacterSheet) reply {
			return who.call(charactersv1connect.CharacterServicePreviewChoicesProcedure, &charactersv1.PreviewChoicesRequest{
				CampaignId: w.campaign, CharacterId: held.GetId(), Kind: charactersv1.CharacterKind_CHARACTER_KIND_PLAYER, Sheet: s,
			})
		},
	}
	for _, c := range all {
		for call, send := range calls {
			who := w.ana
			if call == "create" {
				who = w.caio
			}
			want := send(who, draft(c))
			body := string(want.body)
			if want.ok() || !strings.Contains(body, c.text) || !strings.Contains(body, c.reason) || !strings.Contains(body, c.key) {
				t.Errorf("%s %s, %s, no skill: HTTP %d %s, want the closed choice of %s", c.kind.name, c.how, call, want.status, shorten(want.body), c.reason)
				continue
			}
			for _, skill := range []string{inside, outside} {
				got := send(who, draft(c, skill))
				if got.status != want.status || keyless(got.body, c.key) != keyless(want.body, c.key) {
					t.Errorf("%s %s, %s, with %s: HTTP %d %s\nwithout the skill: HTTP %d %s", c.kind.name, c.how, call, skill, got.status, shorten(got.body), want.status, shorten(want.body))
				}
			}
		}
	}
}
