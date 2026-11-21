package leaktest

import (
	"slices"
	"strconv"
	"strings"
	"testing"

	campaignsv1 "github.com/PuraFome/meuRPG/backend/gen/meurpg/campaigns/v1"
	charactersv1 "github.com/PuraFome/meuRPG/backend/gen/meurpg/characters/v1"
	"github.com/PuraFome/meuRPG/backend/gen/meurpg/characters/v1/charactersv1connect"
	rulesv1 "github.com/PuraFome/meuRPG/backend/gen/meurpg/rules/v1"
	"github.com/PuraFome/meuRPG/backend/internal/rules"
)

// A class feature the master archived is no source for a player: every answer a player gets is the one they would get if the
// feature did not exist, with a skill its list holds and with one it does not. The master, who reads what is archived, keeps
// the list.
func TestAnArchivedClassFeatureIsNoSourceOfAPickForAPlayer(t *testing.T) {
	p := newChoicesProbe(t)
	w, ctx := p.w, t.Context()
	must(w.master.campaigns.SetCampaignXpMode(ctx, rq(&campaignsv1.SetCampaignXpModeRequest{CampaignId: w.campaign, XpMode: campaignsv1.XpMode_XP_MODE_ENEMIES})))
	const inside, outside = "skill:performance", "skill:stealth"
	held := func(who *person, xp int32) *charactersv1.Character {
		s := sheet("class:wizard", 1)
		s.GetFull().ExperiencePoints = xp
		return must(who.characters.CreateCharacter(ctx, rq(&charactersv1.CreateCharacterRequest{
			CampaignId: w.campaign, Kind: charactersv1.CharacterKind_CHARACTER_KIND_PLAYER, Name: "Perita", Sheet: s,
		}))).GetCharacter()
	}
	rising := held(w.ana, 300)
	stored := rising
	draft := func(skills ...string) *charactersv1.CharacterSheet {
		s := sheet("class:wizard", 2)
		s.GetFull().SkillProficiencyKeys = skills
		return s
	}
	// lists are the skills the player's own choices allow, for the draft of the stored sheet.
	lists := func(who *person, out map[string]string) map[string]string {
		r := who.call(charactersv1connect.CharacterServicePreviewChoicesProcedure, &charactersv1.PreviewChoicesRequest{
			CampaignId: w.campaign, CharacterId: stored.GetId(), Kind: charactersv1.CharacterKind_CHARACTER_KIND_PLAYER, Sheet: draft(),
		})
		if !r.ok() {
			t.Fatalf("PreviewChoices: HTTP %d %s", r.status, shorten(r.body))
		}
		c := choicesOf(t, r)
		out["skills_allowed"] = strings.Join(c.GetSkillsAllowed().GetKeys(), ",")
		out["expertise_allowed"] = strings.Join(c.GetExpertiseAllowed().GetKeys(), ",")
		return out
	}
	levelUp := func(skill string) reply {
		return w.ana.call(charactersv1connect.CharacterServiceLevelUpCharacterProcedure, &charactersv1.LevelUpCharacterRequest{
			CampaignId: w.campaign, CharacterId: rising.GetId(), Revision: rising.GetRevision(), Choices: &charactersv1.LevelUpChoices{
				ClassKey: "class:wizard", SubclassKey: "subclass:evocation", KnownSpellKeys: []string{"spell:magic-missile", "spell:shield"}, SkillProficiencyKeys: []string{skill},
				HitPoints: &charactersv1.LevelUpHitPoints{Method: charactersv1.LevelUpHitPointsMethod_LEVEL_UP_HIT_POINTS_METHOD_AVERAGE},
			},
		})
	}
	// answers are the replies of the player to every call that takes a pick, for each skill, and the lists of the choices.
	answers := func(who *person, key string) map[string]string {
		out := map[string]string{}
		put := func(name string, r reply) {
			if key != "" {
				r.body = []byte(strings.ReplaceAll(string(r.body), key, "K"))
			}
			out[name] = string(r.body) + " HTTP " + strconv.Itoa(r.status/100)
		}
		for _, skill := range []string{inside, outside} {
			put("create "+skill, w.caio.call(charactersv1connect.CharacterServiceCreateCharacterProcedure, &charactersv1.CreateCharacterRequest{
				CampaignId: w.campaign, Kind: charactersv1.CharacterKind_CHARACTER_KIND_PLAYER, Name: "Nova", Sheet: draft(skill),
			}))
			put("update "+skill, who.call(charactersv1connect.CharacterServiceUpdateCharacterProcedure, &charactersv1.UpdateCharacterRequest{
				CampaignId: w.campaign, CharacterId: stored.GetId(), Revision: stored.GetRevision(), Name: stored.GetName(), Sheet: draft(skill),
			}))
			put("preview "+skill, who.call(charactersv1connect.CharacterServicePreviewCharacterProcedure, &charactersv1.PreviewCharacterRequest{
				CampaignId: w.campaign, Kind: charactersv1.CharacterKind_CHARACTER_KIND_PLAYER, Sheet: draft(skill),
			}))
		}
		put("options", who.call(charactersv1connect.CharacterServiceGetLevelUpOptionsProcedure, &charactersv1.GetLevelUpOptionsRequest{CampaignId: w.campaign, CharacterId: rising.GetId(), ClassKey: "class:wizard"}))
		return out
	}
	bareStealth, barePerformance := levelUp(outside), levelUp(inside)
	without := lists(w.ana, answers(w.ana, ""))
	if without["create "+inside] == without["create "+outside] && strings.Contains(without["create "+inside], "HTTP 2") {
		t.Fatalf("without the feature the draft with %s is answered %s: the row measures nothing", inside, without["create "+inside])
	}
	feature := must(w.master.table.CreateTableEntry(ctx, rq(&rulesv1.CreateTableEntryRequest{
		CampaignId: w.campaign, Body: &rulesv1.CreateTableEntryRequest_TableClassFeature{TableClassFeature: &rulesv1.TableClassFeature{
			NamePt: "Canto arquivado", DescPt: []string{"Texto de teste."}, ClassKey: "class:wizard", Level: 2,
			Effects: []*rulesv1.TableEffect{{Type: "choice", Choice: "skill", Count: 1, From: []string{inside}}},
		}},
	}))).GetEntry()
	// The positive control: while the feature is live Ana and the master read its list.
	for _, who := range []struct {
		name string
		p    *person
	}{{"Ana", w.ana}, {"the master", w.master}} {
		if got := lists(who.p, map[string]string{})["skills_allowed"]; !slices.Contains(strings.Split(got, ","), inside) {
			t.Fatalf("%s: before the archive skills_allowed is %q, want %s listed (the positive control)", who.name, got, inside)
		}
	}
	must(w.master.table.ArchiveTableEntry(ctx, rq(&rulesv1.ArchiveTableEntryRequest{CampaignId: w.campaign, Key: feature.GetKey()})))
	with := lists(w.ana, answers(w.ana, feature.GetKey()))
	for name, want := range without {
		if name == "options" {
			continue // main counts the feature's pick in the level (skillChoices), so only its sources are compared, below
		}
		if with[name] != want {
			t.Errorf("Ana, %s, with the archived feature: %s\nwithout the feature: %s", name, with[name], want)
		}
	}
	// The options keep main's count of the level's picks; the feature is named as a source of none of them.
	if body := with["options"]; strings.Contains(body, "skillSources") || strings.Contains(body, inside) {
		t.Errorf("Ana's options list the archived feature as a source of skills: %s", body)
	}
	// A level-up is counted by the level itself, as on main: the pick the archived feature lists goes through, any other is
	// refused as it is without the feature. Last, because it moves the character.
	if got, want := levelUp(outside), bareStealth; strings.ReplaceAll(string(got.body), feature.GetKey(), "K") != string(want.body) {
		t.Errorf("Ana, level-up %s, with the archived feature: %s\nwithout the feature: %s", outside, shorten(got.body), shorten(want.body))
	}
	if barePerformance.ok() {
		t.Fatalf("without the feature the level-up with %s is accepted: the row measures nothing", inside)
	}
	if got := levelUp(inside); !got.ok() {
		t.Errorf("Ana, level-up %s, with the archived feature: %s %d, want what main answers (accepted)", inside, shorten(got.body), got.status)
	}
	if got := lists(w.master, map[string]string{})["skills_allowed"]; !slices.Contains(strings.Split(got, ","), inside) {
		t.Errorf("the master reads what is archived: skills_allowed is %q, want %s listed", got, inside)
	}
}

// A tool pick of the owner an archived class feature is answers a player as a tool pick of an owner that does not exist,
// whether the tool is on the feature's list or not, on every call that takes a pick. The master, who reads the feature,
// is not asked here.
func TestAToolPickOfAnArchivedClassFeatureIsAnsweredAsOneOfNoOwner(t *testing.T) {
	p := newChoicesProbe(t)
	w, ctx := p.w, t.Context()
	const inside, outside, absent = "proficiency:smiths-tools", "proficiency:glaives", "feature:nenhum--dono@mesa"
	feature := must(w.master.table.CreateTableEntry(ctx, rq(&rulesv1.CreateTableEntryRequest{
		CampaignId: w.campaign, Body: &rulesv1.CreateTableEntryRequest_TableClassFeature{TableClassFeature: &rulesv1.TableClassFeature{
			NamePt: "Oficio arquivado", DescPt: []string{"Texto de teste."}, ClassKey: "class:wizard", Level: 1,
			Effects: []*rulesv1.TableEffect{{Type: "choice", Choice: "tool", Count: 1, From: []string{inside, "proficiency:thieves-tools"}}}, // two tools: one alone would be picked for the sheet
		}},
	}))).GetEntry()
	// The stored sheet holds the feature's other tool, as its owner: a pick the stored sheet holds stays, so the draft keeps it and adds the new one.
	const heldTool = "proficiency:thieves-tools"
	heldPick := rules.ScopedChoice(rules.ToolPickKey(feature.GetKey()), heldTool)
	held := must(w.ana.characters.CreateCharacter(ctx, rq(&charactersv1.CreateCharacterRequest{
		CampaignId: w.campaign, Kind: charactersv1.CharacterKind_CHARACTER_KIND_PLAYER, Name: "Perita", Sheet: sheet("class:wizard", 1, heldPick),
	}))).GetCharacter()
	must(w.master.table.ArchiveTableEntry(ctx, rq(&rulesv1.ArchiveTableEntryRequest{CampaignId: w.campaign, Key: feature.GetKey()})))
	draft := func(owner, tool string) *charactersv1.CharacterSheet {
		s := sheet("class:wizard", 1)
		s.GetFull().FeatureChoiceKeys = []string{rules.ScopedChoice(rules.ToolPickKey(owner), tool)}
		return s
	}
	// The stored sheet's own pick goes with the new one on an update.
	withHeld := func(s *charactersv1.CharacterSheet) *charactersv1.CharacterSheet {
		s.GetFull().FeatureChoiceKeys = append([]string{heldPick}, s.GetFull().GetFeatureChoiceKeys()...)
		return s
	}
	calls := map[string]func(who *person, owner, tool string) reply{
		"create": func(who *person, owner, tool string) reply {
			return who.call(charactersv1connect.CharacterServiceCreateCharacterProcedure, &charactersv1.CreateCharacterRequest{
				CampaignId: w.campaign, Kind: charactersv1.CharacterKind_CHARACTER_KIND_PLAYER, Name: "Nova", Sheet: draft(owner, tool),
			})
		},
		"preview": func(who *person, owner, tool string) reply {
			return who.call(charactersv1connect.CharacterServicePreviewCharacterProcedure, &charactersv1.PreviewCharacterRequest{
				CampaignId: w.campaign, Kind: charactersv1.CharacterKind_CHARACTER_KIND_PLAYER, Sheet: draft(owner, tool),
			})
		},
		"update": func(who *person, owner, tool string) reply {
			return who.call(charactersv1connect.CharacterServiceUpdateCharacterProcedure, &charactersv1.UpdateCharacterRequest{
				CampaignId: w.campaign, CharacterId: held.GetId(), Revision: held.GetRevision(), Name: held.GetName(), Sheet: withHeld(draft(owner, tool)),
			})
		},
	}
	for call, send := range calls {
		who := w.ana
		if call == "create" {
			who = w.caio
		}
		for _, tool := range []string{inside, outside} {
			named, none := send(who, feature.GetKey(), tool), send(who, absent, tool)
			if none.ok() {
				t.Errorf("%s, %s: accepted (archived %d, absent %d): the row measures nothing", call, tool, named.status, none.status)
			}
			if !sameAnswer(named, feature.GetKey(), none, absent) {
				t.Errorf("%s, %s: the archived owner: HTTP %d %s\nan owner that does not exist: HTTP %d %s", call, tool, named.status, shorten(named.body), none.status, shorten(none.body))
			}
		}
	}
}

// A class feature the master archived is no source of expertise for a player either: the answer to a new expertise is the
// one it gets when the feature does not exist, with a skill on its list and with one off it.
func TestAnArchivedClassFeatureIsNoSourceOfExpertiseForAPlayer(t *testing.T) {
	p := newChoicesProbe(t)
	w, ctx := p.w, t.Context()
	const inside, outside = "skill:arcana", "skill:history"
	draft := func(expertise string) *charactersv1.CharacterSheet {
		s := sheet("class:wizard", 1)
		s.GetFull().SkillProficiencyKeys = []string{inside, outside}
		s.GetFull().ExpertiseSkillKeys = []string{expertise}
		return s
	}
	plain := func() *charactersv1.CharacterSheet {
		s := draft(inside)
		s.GetFull().ExpertiseSkillKeys = nil
		return s
	}
	held := must(w.ana.characters.CreateCharacter(ctx, rq(&charactersv1.CreateCharacterRequest{
		CampaignId: w.campaign, Kind: charactersv1.CharacterKind_CHARACTER_KIND_PLAYER, Name: "Perita", Sheet: plain(),
	}))).GetCharacter()
	answers := func(key string) map[string]string {
		out := map[string]string{}
		for _, skill := range []string{inside, outside} {
			for name, r := range map[string]reply{
				"create": w.caio.call(charactersv1connect.CharacterServiceCreateCharacterProcedure, &charactersv1.CreateCharacterRequest{
					CampaignId: w.campaign, Kind: charactersv1.CharacterKind_CHARACTER_KIND_PLAYER, Name: "Nova", Sheet: draft(skill),
				}),
				"preview": w.ana.call(charactersv1connect.CharacterServicePreviewCharacterProcedure, &charactersv1.PreviewCharacterRequest{
					CampaignId: w.campaign, Kind: charactersv1.CharacterKind_CHARACTER_KIND_PLAYER, Sheet: draft(skill),
				}),
				"update": w.ana.call(charactersv1connect.CharacterServiceUpdateCharacterProcedure, &charactersv1.UpdateCharacterRequest{
					CampaignId: w.campaign, CharacterId: held.GetId(), Revision: held.GetRevision(), Name: held.GetName(), Sheet: draft(skill),
				}),
			} {
				if key != "" {
					r.body = []byte(strings.ReplaceAll(string(r.body), key, "K"))
				}
				out[name+" "+skill] = string(r.body) + " HTTP " + strconv.Itoa(r.status/100)
			}
		}
		return out
	}
	without := answers("")
	if strings.Contains(without["create "+outside], "HTTP 2") || !strings.Contains(without["create "+outside], "HTTP 4") {
		t.Fatalf("without the feature expertise on %s is answered %s: the row measures nothing", outside, without["create "+outside])
	}
	feature := must(w.master.table.CreateTableEntry(ctx, rq(&rulesv1.CreateTableEntryRequest{
		CampaignId: w.campaign, Body: &rulesv1.CreateTableEntryRequest_TableClassFeature{TableClassFeature: &rulesv1.TableClassFeature{
			NamePt: "Mestria arquivada", DescPt: []string{"Texto de teste."}, ClassKey: "class:wizard", Level: 1,
			Effects: []*rulesv1.TableEffect{{Type: "choice", Choice: "expertise", Count: 1, From: []string{inside}}},
		}},
	}))).GetEntry()
	must(w.master.table.ArchiveTableEntry(ctx, rq(&rulesv1.ArchiveTableEntryRequest{CampaignId: w.campaign, Key: feature.GetKey()})))
	with := answers(feature.GetKey())
	for name, want := range without {
		if with[name] != want {
			t.Errorf("Ana, %s, with the archived feature: %s\nwithout the feature: %s", name, with[name], want)
		}
	}
}

// The list of the skills expertise may go on, which PreviewChoices returns, holds no skill of an archived class feature for a
// player: the master reads it, Ana's list is the one she reads when the feature does not exist.
func TestAnArchivedClassFeatureAddsNoExpertiseKeyToAPlayersListForADraft(t *testing.T) {
	p := newChoicesProbe(t)
	w, ctx := p.w, t.Context()
	const inside = "skill:arcana"
	draft := func() *charactersv1.CharacterSheet {
		s := sheet("class:wizard", 1)
		s.GetFull().SkillProficiencyKeys = []string{inside}
		return s
	}
	held := must(w.ana.characters.CreateCharacter(ctx, rq(&charactersv1.CreateCharacterRequest{
		CampaignId: w.campaign, Kind: charactersv1.CharacterKind_CHARACTER_KIND_PLAYER, Name: "Perita", Sheet: draft(),
	}))).GetCharacter()
	listed := func(who *person) string {
		r := who.call(charactersv1connect.CharacterServicePreviewChoicesProcedure, &charactersv1.PreviewChoicesRequest{
			CampaignId: w.campaign, CharacterId: held.GetId(), Kind: charactersv1.CharacterKind_CHARACTER_KIND_PLAYER, Sheet: draft(),
		})
		if !r.ok() {
			t.Fatalf("PreviewChoices: HTTP %d %s", r.status, shorten(r.body))
		}
		return strings.Join(choicesOf(t, r).GetExpertiseAllowed().GetKeys(), ",")
	}
	without := listed(w.ana)
	feature := must(w.master.table.CreateTableEntry(ctx, rq(&rulesv1.CreateTableEntryRequest{
		CampaignId: w.campaign, Body: &rulesv1.CreateTableEntryRequest_TableClassFeature{TableClassFeature: &rulesv1.TableClassFeature{
			NamePt: "Mestria arquivada", DescPt: []string{"Texto de teste."}, ClassKey: "class:wizard", Level: 1,
			Effects: []*rulesv1.TableEffect{{Type: "choice", Choice: "expertise", Count: 1, From: []string{inside}}},
		}},
	}))).GetEntry()
	for name, who := range map[string]*person{"the master": w.master, "Ana": w.ana} {
		if got := listed(who); !strings.Contains(got, inside) {
			t.Fatalf("%s: before the archive the expertise list is %q, without %s: the positive control", name, got, inside)
		}
	}
	must(w.master.table.ArchiveTableEntry(ctx, rq(&rulesv1.ArchiveTableEntryRequest{CampaignId: w.campaign, Key: feature.GetKey()})))
	if got := listed(w.master); !strings.Contains(got, inside) {
		t.Errorf("the master reads what is archived, and the expertise list is %q", got)
	}
	if got := listed(w.ana); got != without {
		t.Errorf("Ana's expertise list with the archived feature is %q, without the feature %q", got, without)
	}
}
