package rules

import "testing"

// A source whose list holds a skill the sheet has by another means (a background) takes another skill in its place (SRD 5.1).
func TestAnAutomaticSkillOnASourceListLetsAnotherSkillBePicked(t *testing.T) {
	t.Parallel()
	traits := []TableFeature{capTrait("trait:faro@mesa", "skill", 1, "skill:insight")} // the acolyte has insight already
	c, held := capSheet(t, traits, nil, nil)
	if got := missingOf(Derive(held, c), OpenChoiceSkills); got != 1 {
		t.Errorf("open skills before the pick = %d, want 1", got)
	}
	_, now := capSheet(t, traits, []string{"skill:arcana"}, nil)
	for name, view := range map[string]*Content{"master": c, "player": c.ForPlayer()} {
		d := Derive(now, view)
		if got := missingOf(d, OpenChoiceSkills); got != 0 {
			t.Errorf("%s: open skills = %d, want none", name, got)
		}
		if is := countIssues(d); len(is) != 0 {
			t.Errorf("%s: count issues = %v", name, is)
		}
		if field, key, found := view.UnplacedNewPick(&held, now); found {
			t.Errorf("%s: refused %q at %q", name, key, field)
		}
	}
}

// The pick the refusal names is one the sources cannot place: the sheet without it is accepted.
func TestTheRefusedPickIsOneTheSourcesHaveNoRoomFor(t *testing.T) {
	t.Parallel()
	traits := []TableFeature{capTrait("trait:faro@mesa", "skill", 1, "skill:arcana", "skill:nature")}
	c, held := capSheet(t, traits, nil, nil)
	for _, order := range [][]string{{"skill:arcana", "skill:nature"}, {"skill:nature", "skill:arcana"}} {
		_, now := capSheet(t, traits, order, nil)
		field, key, found := c.UnplacedNewPick(&held, now)
		if !found {
			t.Fatalf("%v: two picks for one slot are accepted", order)
		}
		without := now.clone()
		without.SkillProficiencies = nil
		for _, k := range now.SkillProficiencies {
			if k != key {
				without.SkillProficiencies = append(without.SkillProficiencies, k)
			}
		}
		if f, k, found := c.UnplacedNewPick(&held, without); found {
			t.Errorf("%v: refused %q at %q (first refused %q at %q)", order, k, f, key, field)
		}
	}
}

// A skill the stored sheet had by a background is no held pick: when the background changes, the skill is a new pick.
func TestASkillTheStoredSheetHadByItsBackgroundIsANewPickWhenTheBackgroundChanges(t *testing.T) {
	t.Parallel()
	c := loadForTest(t)
	held := sweepBase(t, c, "class:fighter", "") // the acolyte: insight and religion
	held.SkillProficiencies = append(held.SkillProficiencies, "skill:religion")
	now := held.clone()
	now.Background = ""
	field, key, found := c.UnplacedNewPick(&held, now)
	if !found || key != "skill:religion" {
		t.Errorf("got %q %q %v, want the skill the background no longer gives", field, key, found)
	}
	if _, _, found := c.UnplacedNewPick(&held, held); found {
		t.Error("the unchanged sheet is refused")
	}
}

// The level asks the same of a sheet that holds a feat it no longer meets as of one without the feat.
func TestTheLevelAsksTheSameWithALostFeat(t *testing.T) {
	t.Parallel()
	const feat = "feat:estudo" + tableSuffix
	c := withOverlay(t, Overlay{
		Revision: 1,
		Feats: []TableFeat{{
			TableEntry: TableEntry{Key: feat, NamePT: "Estudo"}, DescPT: []string{"Texto."},
			Prerequisite: FeatPrerequisite{ClassKey: "class:fighter", ClassLevel: 5},
			Effects:      []Effect{{Type: "choice", Choice: "skill", Count: 1, From: []string{"skill:athletics"}}},
		}},
		Subclasses: []TableSubclass{subclassWithFeature(Effect{Type: "choice", Choice: "skill", Count: 1, From: []string{"skill:athletics"}})},
	})
	plain := sweepUp(t, c, sweepBase(t, c, "class:fighter", ""), "class:fighter", sourceSub, 2)
	plain.SkillProficiencies = []string{"skill:acrobatics", "skill:perception"}
	lost := plain.clone()
	lost.Feats = []string{feat}
	want, _ := subclassAsked(t, c, plain)
	if got, _ := subclassAsked(t, c, lost); got != want {
		t.Errorf("the level asks %d skills with a lost feat, %d without it", got, want)
	}
}
