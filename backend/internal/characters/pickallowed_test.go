package characters

import (
	"slices"
	"testing"

	"connectrpc.com/connect"

	charactersv1 "github.com/PuraFome/meuRPG/backend/gen/meurpg/characters/v1"
)

// The lists the editor cuts its skill picker to come from PreviewChoices.

func (u *user) allowed(t *testing.T, campaign string, s *charactersv1.CharacterSheet) *charactersv1.PreviewChoicesResponse {
	t.Helper()
	return u.allowedAs(t, campaign, charactersv1.CharacterKind_CHARACTER_KIND_PLAYER, s)
}

func (u *user) allowedAs(t *testing.T, campaign string, kind charactersv1.CharacterKind, s *charactersv1.CharacterSheet) *charactersv1.PreviewChoicesResponse {
	t.Helper()
	res, err := u.api.PreviewChoices(t.Context(), connect.NewRequest(&charactersv1.PreviewChoicesRequest{
		CampaignId: campaign, Kind: kind, Sheet: s,
	}))
	if err != nil {
		t.Fatalf("PreviewChoices() error = %v", err)
	}
	return res.Msg
}

func TestAWizardsSkillListIsTheClassList(t *testing.T) {
	t.Parallel()
	u := newUnfitTable(t, "skill:survival")
	s := pensantusSheet() // a gnome wizard: no trait offers skills
	res := u.player.allowed(t, u.campaign, s)
	got := res.GetSkillsAllowed()
	if got == nil || got.GetAny() {
		t.Fatalf("skills_allowed = %v, want a list", got)
	}
	for _, k := range []string{"skill:arcana", "skill:history", "skill:insight", "skill:investigation", "skill:medicine", "skill:religion"} {
		if !slices.Contains(got.GetKeys(), k) {
			t.Errorf("skills_allowed lacks %s: %v", k, got.GetKeys())
		}
	}
	for _, k := range []string{"skill:stealth", "skill:survival", "skill:athletics"} {
		if slices.Contains(got.GetKeys(), k) {
			t.Errorf("skills_allowed holds %s, which no source of a gnome wizard lists", k)
		}
	}
	if res.GetExpertiseAllowed() != nil {
		t.Errorf("expertise_allowed = %v, want unset: no source offers expertise", res.GetExpertiseAllowed())
	}
}

func TestATableTraitListAddsItsKeys(t *testing.T) {
	t.Parallel()
	u := newUnfitTable(t, "skill:survival")
	res := u.player.allowed(t, u.campaign, u.sheet("skill:investigation", "skill:insight"))
	keys := res.GetSkillsAllowed().GetKeys()
	if !slices.Contains(keys, "skill:survival") || !slices.Contains(keys, "skill:arcana") || res.GetSkillsAllowed().GetAny() {
		t.Errorf("skills_allowed = %v, want the class's keys and skill:survival", res.GetSkillsAllowed())
	}
	if n := len(slices.Compact(slices.Sorted(slices.Values(keys)))); n != len(keys) {
		t.Errorf("skills_allowed repeats a key: %v", keys)
	}
}

func TestASourceThatNamesNoListAllowsAnySkill(t *testing.T) {
	t.Parallel()
	u := newUnfitTable(t) // the trait names no list
	got := u.player.allowed(t, u.campaign, u.sheet("skill:investigation", "skill:insight")).GetSkillsAllowed()
	if !got.GetAny() || len(got.GetKeys()) != 0 {
		t.Errorf("skills_allowed = %v, want any with no keys", got)
	}
}

func TestExpertiseListIsSetOnlyWhenEverySourceListsSkills(t *testing.T) {
	t.Parallel()
	u := newUnfitTable(t, "skill:nature")
	u.race = u.master.updateEntry(t, u.campaign, u.race, expertiseTraitRace("skill:athletics", "skill:nature"))
	got := u.player.allowed(t, u.campaign, u.sheet("skill:investigation", "skill:insight", "skill:nature")).GetExpertiseAllowed()
	if got == nil || got.GetAny() || !slices.Equal(got.GetKeys(), []string{"skill:athletics", "skill:nature"}) {
		t.Errorf("expertise_allowed = %v, want the trait's list", got)
	}
	// The rogue's Expertise takes any skill the character is proficient in: no cut.
	rogue := u.sheet("skill:investigation", "skill:insight", "skill:nature")
	rogue.GetFull().Classes = []*charactersv1.ClassLevel{{ClassKey: "class:rogue", Level: 1}}
	if got := u.player.allowed(t, u.campaign, rogue).GetExpertiseAllowed(); got != nil {
		t.Errorf("expertise_allowed = %v on a sheet with a source that takes any skill, want unset", got)
	}
}

func TestTheMasterAndAPlayerReadTheSameLists(t *testing.T) {
	t.Parallel()
	u := newUnfitTable(t, "skill:survival")
	s := u.sheet("skill:investigation", "skill:insight", "skill:survival")
	p, m := u.player.allowed(t, u.campaign, s), u.master.allowedAs(t, u.campaign, charactersv1.CharacterKind_CHARACTER_KIND_ENEMY, s)
	if !slices.Equal(p.GetSkillsAllowed().GetKeys(), m.GetSkillsAllowed().GetKeys()) || p.GetSkillsAllowed().GetAny() != m.GetSkillsAllowed().GetAny() {
		t.Errorf("player %v, master %v", p.GetSkillsAllowed(), m.GetSkillsAllowed())
	}
}
