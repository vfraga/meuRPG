package leaktest

import (
	"slices"
	"strings"
	"testing"

	campaignsv1 "github.com/PuraFome/meuRPG/backend/gen/meurpg/campaigns/v1"
	charactersv1 "github.com/PuraFome/meuRPG/backend/gen/meurpg/characters/v1"
	"github.com/PuraFome/meuRPG/backend/gen/meurpg/characters/v1/charactersv1connect"
	rulesv1 "github.com/PuraFome/meuRPG/backend/gen/meurpg/rules/v1"
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
			out[name] = shorten(r.body) + " HTTP " + string(rune('0'+r.status/100))
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
		if with[name] != want {
			t.Errorf("Ana, %s, with the archived feature: %s\nwithout the feature: %s", name, with[name], want)
		}
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
