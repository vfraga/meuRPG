package rules

import (
	"slices"
	"strings"
	"testing"
)

// firstFitPlacement is the placement that gives each pick the first source with room that allows it and never moves a
// pick already placed; the tests below show it leaves out picks that matchPicks places.
func firstFitPlacement(sources []pickSource, picks []string) []int {
	room := make([]int, len(sources))
	for i, s := range sources {
		room[i] = s.Count
	}
	out := make([]int, len(picks))
	for p, k := range picks {
		out[p] = -1
		for i, s := range sources {
			if room[i] > 0 && s.allows(k) {
				room[i]--
				out[p] = i
				break
			}
		}
	}
	return out
}

func TestMatchPicks(t *testing.T) {
	t.Parallel()
	for _, tc := range []struct {
		name    string
		sources []pickSource
		picks   []string
		want    []int
	}{
		{"no picks", []pickSource{{Count: 1, Any: true}}, nil, []int{}},
		{"no sources", nil, []string{"a"}, []int{-1}},
		{"one list takes both", []pickSource{{Count: 2, From: []string{"a", "b"}}}, []string{"a", "b"}, []int{0, 0}},
		{"a pick no list allows", []pickSource{{Count: 2, From: []string{"a"}}}, []string{"a", "z"}, []int{0, -1}},
		{"a full source leaves the later pick out", []pickSource{{Count: 1, From: []string{"a", "b"}}}, []string{"a", "b"}, []int{0, -1}},
		{"an open source takes any key", []pickSource{{Count: 1, Any: true}}, []string{"q"}, []int{0}},
		{"the earlier pick moves to make room", []pickSource{{Count: 1, From: []string{"a", "b"}}, {Count: 1, From: []string{"a"}}}, []string{"a", "b"}, []int{1, 0}},
		{"a source of two overlapping another", []pickSource{{Count: 2, From: []string{"a", "b", "c"}}, {Count: 1, From: []string{"a"}}}, []string{"a", "b", "c"}, []int{1, 0, 0}},
		{"a chain of moves", []pickSource{{Count: 1, From: []string{"a", "b"}}, {Count: 1, From: []string{"b", "c"}}, {Count: 1, From: []string{"c"}}}, []string{"a", "b", "c"}, []int{0, 1, 2}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			if got := matchPicks(tc.sources, tc.picks); !slices.Equal(got, tc.want) {
				t.Errorf("matchPicks = %v, want %v", got, tc.want)
			}
		})
	}
}

// TestMatchingPlacesWhatFirstFitLeavesOut: two sources whose lists overlap. First-fit gives the shared key to the first
// source and has no room for the key only that source allows; the matching moves the shared key to the other source.
func TestMatchingPlacesWhatFirstFitLeavesOut(t *testing.T) {
	t.Parallel()
	sources := []pickSource{{Owner: "x", Count: 1, From: []string{"skill:a", "skill:b"}}, {Owner: "y", Count: 1, From: []string{"skill:a"}}}
	picks := []string{"skill:a", "skill:b"}
	if got := firstFitPlacement(sources, picks); !slices.Contains(got, -1) {
		t.Fatalf("first-fit = %v, the case is meant to leave a pick out", got)
	}
	if left := unplacedPicks(sources, picks); len(left) != 0 {
		t.Errorf("unplacedPicks = %v, want none", left)
	}
}

// fitSheet is a fighter of a table race whose trait lets the player choose one skill from skill:arcana alone. The fighter
// chooses two of its list; the third pick (the trait's) is the one given.
func fitSheet(t *testing.T, trait string) (*Content, Build) {
	t.Helper()
	race := TableRace{
		Key: "race:perito@mesa", NamePT: "Perito", Revision: 1, Size: "Medium", SpeedFt: 30,
		Traits: []TableFeature{{Key: "trait:faro@mesa", NamePT: "Faro", Effects: []Effect{{Type: "choice", Choice: "skill", Count: 1, From: []string{"skill:arcana"}}}}},
	}
	c := withOverlay(t, Overlay{Revision: 1, Races: []TableRace{race}})
	b := sweepBase(t, c, "class:fighter", "")
	b.Race = race.Key
	b.SkillProficiencies = append(b.SkillProficiencies, trait)
	return c, b
}

func fitIssues(d Derived) []Issue {
	var out []Issue
	for _, is := range d.Issues {
		if is.Code == IssuePickFit {
			out = append(out, is)
		}
	}
	return out
}

// TestFitIssueIsTheMastersAndNoChoiceProblem pins the path that decides it: the issue comes only from deriver.readsFit (the
// master's derivation of table content), and ChoiceSet.ChoiceProblems, which the save and the level-up read, is built from
// the choices alone and never from the issues of Derive. TestUnfitStoredSheetLevelsUp pins the level-up.
func TestFitIssueIsTheMastersAndNoChoiceProblem(t *testing.T) {
	t.Parallel()
	c, fits := fitSheet(t, "skill:arcana")
	if is := fitIssues(Derive(fits, c)); len(is) != 0 {
		t.Fatalf("a sheet that fits has fit issues: %v", is)
	}
	_, b := fitSheet(t, "skill:history") // the trait's pick is a skill only the trait could give: history is in nobody's list
	master := Derive(b, c)
	is := fitIssues(master)
	if len(is) != 1 || is[0].Field != "full.skill_proficiency_keys[2]" {
		t.Fatalf("the master's fit issues = %v, want one on the third skill", is)
	}
	if got := fitIssues(Derive(b, c.ForPlayer())); len(got) != 0 {
		t.Errorf("a player's derivation has fit issues: %v", got)
	}
	set := c.Choices(b)
	if problems := set.ChoiceProblems(true); len(problems) != 0 {
		t.Errorf("the sheet's choice problems = %v, want none", problems)
	}
	if lerr := checkChoiceProblems(set, set, nil); lerr != nil {
		t.Errorf("the level-up refuses the sheet for the fit: %v", lerr)
	}
}

// TestFitIssueMatchesOverlappingSources: the sheet fits only when history, which the class and the trait both list, goes to
// the trait; first-fit gives it to the class and has no room for perception.
func TestFitIssueMatchesOverlappingSources(t *testing.T) {
	t.Parallel()
	race := TableRace{
		Key: "race:perito@mesa", NamePT: "Perito", Revision: 1, Size: "Medium", SpeedFt: 30,
		Traits: []TableFeature{{Key: "trait:faro@mesa", NamePT: "Faro", Effects: []Effect{{Type: "choice", Choice: "skill", Count: 1, From: []string{"skill:history"}}}}},
	}
	c := withOverlay(t, Overlay{Revision: 1, Races: []TableRace{race}})
	b := sweepBase(t, c, "class:fighter", "")
	b.Race = race.Key
	b.SkillProficiencies = []string{"skill:history", "skill:athletics", "skill:perception"}
	if got := firstFitPlacement([]pickSource{
		{Owner: "class:fighter", Count: 2, From: c.c.classes["class:fighter"].SkillChoices.From},
		{Owner: "trait:faro@mesa", Count: 1, From: []string{"skill:history"}},
	}, b.SkillProficiencies); !slices.Contains(got, -1) {
		t.Fatalf("first-fit = %v, the case is meant to leave a pick out", got)
	}
	if is := fitIssues(Derive(b, c)); len(is) != 0 {
		t.Errorf("the sheet fits by moving a pick between sources, fit issues = %v", is)
	}
}

// TestToolEffectsOfOneOwnerWithDifferentPools: two tool choices of one owner, each with its own list, place the owner's
// picks by the same matching whatever the order of the picks.
func TestToolEffectsOfOneOwnerWithDifferentPools(t *testing.T) {
	t.Parallel()
	const thieves, herbs, smith = "proficiency:thieves-tools", "proficiency:herbalism-kit", "proficiency:smiths-tools"
	o := filterBackgroundWith(Effect{Type: "choice", Choice: "tool", Count: 1, From: []string{thieves, herbs}})
	o.Backgrounds[0].Feature.Effects = append(o.Backgrounds[0].Feature.Effects, Effect{Type: "choice", Choice: "tool", Count: 1, From: []string{thieves}})
	c := withOverlay(t, o)
	owner := "background-feature:rede-do-cais" + tableSuffix
	for _, picks := range [][]string{{thieves, herbs}, {herbs, thieves}} {
		var in []proficiencyPick
		for _, k := range picks {
			in = append(in, proficiencyPick{Value: k, Owner: owner})
		}
		placed, unplaced := c.c.placeToolPicks(in)
		if len(placed) != 2 || len(unplaced) != 0 {
			t.Errorf("picks %v: placed %v, unplaced %v", picks, placed, unplaced)
		}
	}
	placed, unplaced := c.c.placeToolPicks([]proficiencyPick{{Value: herbs, Owner: owner}, {Value: smith, Owner: owner}})
	if len(placed) != 1 || len(unplaced) != 1 || unplaced[0].Value != smith {
		t.Errorf("a tool no choice lists: placed %v, unplaced %v", placed, unplaced)
	}
}

const sourceSub = "subclass:mestre-do-saber" + tableSuffix

// sourceLevelUpFixture is a fighter at level 2 and a table subclass whose level-3 feature lets the player choose one skill
// from arcana and history.
func sourceLevelUpFixture(t *testing.T) (*Content, Build, LevelUpChoices) {
	t.Helper()
	c := withOverlay(t, Overlay{Revision: 1, Subclasses: []TableSubclass{{
		TableEntry: TableEntry{Key: sourceSub, NamePT: "Mestre do saber"}, Class: "class:fighter",
		Levels: []TableSubclassLevel{{Level: 3, Features: []TableFeature{{
			Key: "feature:estudo@mesa", NamePT: "Estudo", DescPT: []string{"Uma perícia à escolha."},
			Effects: []Effect{{Type: "choice", Choice: "skill", Count: 1, From: []string{"skill:arcana", "skill:history"}}},
		}}}},
	}}})
	b := sweepUp(t, c, sweepBase(t, c, "class:fighter", ""), "class:fighter", sourceSub, 2)
	return c, b, satisfy(t, c, b, "class:fighter", sourceSub)
}

func TestLevelUpSkillsAreMatchedToTheLevelsSources(t *testing.T) {
	t.Parallel()
	c, before, ch := sourceLevelUpFixture(t)
	o, err := LevelUpOptions(before, "class:fighter", c)
	if err != nil {
		t.Fatal(err)
	}
	var found bool
	for _, s := range o.Subclasses {
		if s.Key == sourceSub {
			found = len(s.SkillSources) == 1 && s.SkillSources[0].Owner == "feature:estudo@mesa" && s.SkillSources[0].NamePT == "Estudo" &&
				s.SkillSources[0].Count == 1 && slices.Equal(s.SkillSources[0].From, []string{"skill:arcana", "skill:history"})
		}
	}
	if !found {
		t.Errorf("the subclass's skill sources = %+v", o.Subclasses)
	}
	for skill, ok := range map[string]bool{"skill:arcana": true, "skill:history": true, "skill:stealth": false} {
		ch.SkillProficiencies = []string{skill}
		after, err := ApplyLevelUp(before, ch, c)
		if err != nil {
			t.Fatalf("%s: ApplyLevelUp: %v", skill, err)
		}
		err = CheckLevelUpOf(before, after, ch, c)
		if ok && err != nil {
			t.Errorf("%s: refused: %v", skill, err)
		}
		if !ok {
			wantRefusal(t, err, LevelUpReasonSkills, "full.skill_proficiency_keys")
		}
	}
}

func TestLevelSkillSourcesOfANewClassListTheMulticlassPick(t *testing.T) {
	t.Parallel()
	c := loadForTest(t)
	b := sweepBase(t, c, "class:fighter", "")
	b.BaseScores = copyScores(map[Ability]int{STR: 14, DEX: 14, CON: 14, INT: 14, WIS: 14, CHA: 14})
	o, err := LevelUpOptions(b, "class:ranger", c)
	if err != nil {
		t.Fatal(err)
	}
	if len(o.SkillSources) != 1 || o.SkillSources[0].Owner != "class:ranger" || o.SkillSources[0].Count != 1 || slices.Contains(o.SkillSources[0].From, "skill:acrobatics") || len(o.SkillSources[0].From) != 8 {
		t.Errorf("sources = %+v, want the ranger's list of eight", o.SkillSources)
	}
}

func TestUnplacedNewPick(t *testing.T) {
	t.Parallel()
	c, fits := fitSheet(t, "skill:arcana")
	_, unfit := fitSheet(t, "skill:history") // the trait's pick is in nobody's list
	t.Run("a new pick no source takes is refused at its place", func(t *testing.T) {
		t.Parallel()
		field, key, found := c.UnplacedNewPick(&fits, unfit)
		if !found || field != "full.skill_proficiency_keys[2]" || key != "skill:history" {
			t.Errorf("got %q %q %v", field, key, found)
		}
		if _, _, found := c.UnplacedNewPick(nil, unfit); !found {
			t.Error("a new character's pick is new too")
		}
	})
	t.Run("a held pick that does not fit stays allowed", func(t *testing.T) {
		t.Parallel()
		if field, key, found := c.UnplacedNewPick(&unfit, unfit); found {
			t.Errorf("refused the held pick %q at %q", key, field)
		}
		later := unfit.clone()
		later.Weapons = append(later.Weapons, "equipment:longsword") // another field changes; the picks stay
		if _, _, found := c.UnplacedNewPick(&unfit, later); found {
			t.Error("refused a sheet whose picks are held")
		}
	})
	t.Run("a new pick that fits by moving a held pick is accepted", func(t *testing.T) {
		t.Parallel()
		race := TableRace{
			Key: "race:perito@mesa", NamePT: "Perito", Revision: 1, Size: "Medium", SpeedFt: 30,
			Traits: []TableFeature{{Key: "trait:faro@mesa", NamePT: "Faro", Effects: []Effect{{Type: "choice", Choice: "skill", Count: 1, From: []string{"skill:history"}}}}},
		}
		cc := withOverlay(t, Overlay{Revision: 1, Races: []TableRace{race}})
		held := sweepBase(t, cc, "class:fighter", "")
		held.Race = race.Key
		held.SkillProficiencies = []string{"skill:history", "skill:athletics"}
		now := held.clone()
		now.SkillProficiencies = []string{"skill:history", "skill:athletics", "skill:perception"}
		if field, key, found := cc.UnplacedNewPick(&held, now); found {
			t.Errorf("refused %q at %q: perception fits when history moves to the trait", key, field)
		}
		now.SkillProficiencies = []string{"skill:history", "skill:athletics", "skill:stealth"}
		if _, key, found := cc.UnplacedNewPick(&held, now); !found || key != "skill:stealth" {
			t.Errorf("stealth is in no list, got %q %v", key, found)
		}
	})
	t.Run("a new expertise no source takes is refused", func(t *testing.T) {
		t.Parallel()
		race := TableRace{
			Key: "race:perito@mesa", NamePT: "Perito", Revision: 1, Size: "Medium", SpeedFt: 30,
			Traits: []TableFeature{{Key: "trait:faro@mesa", NamePT: "Faro", Effects: []Effect{{Type: "choice", Choice: "expertise", Count: 1, From: []string{"skill:athletics"}}}}},
		}
		ec := withOverlay(t, Overlay{Revision: 1, Races: []TableRace{race}})
		held := sweepBase(t, ec, "class:fighter", "")
		held.Race = race.Key
		now := held.clone()
		now.Expertise = []string{"skill:athletics"}
		if field, key, found := ec.UnplacedNewPick(&held, now); found {
			t.Errorf("refused %q at %q", key, field)
		}
		now.Expertise = []string{"skill:acrobatics"}
		field, key, found := ec.UnplacedNewPick(&held, now)
		if !found || field != "full.expertise_skill_keys[0]" || key != "skill:acrobatics" {
			t.Errorf("got %q %q %v", field, key, found)
		}
	})
	t.Run("a tool no choice of the owner lists", func(t *testing.T) {
		t.Parallel()
		const thieves, herbs, smith = "proficiency:thieves-tools", "proficiency:herbalism-kit", "proficiency:smiths-tools"
		o := filterBackgroundWith(Effect{Type: "choice", Choice: "tool", Count: 1, From: []string{thieves, herbs}})
		o.Backgrounds[0].Feature.Effects = append(o.Backgrounds[0].Feature.Effects, Effect{Type: "choice", Choice: "tool", Count: 1, From: []string{thieves}})
		tc := withOverlay(t, o)
		key := filterPickKey()
		held := filterBuild()
		held.FeatureChoices = []string{ScopedChoice(key, thieves)}
		now := held.clone()
		now.FeatureChoices = []string{ScopedChoice(key, thieves), ScopedChoice(key, herbs)}
		if _, _, found := tc.UnplacedNewPick(&held, now); found {
			t.Error("herbalism fits when thieves moves to the second choice")
		}
		now.FeatureChoices = []string{ScopedChoice(key, thieves), ScopedChoice(key, smith)}
		field, k, found := tc.UnplacedNewPick(&held, now)
		if !found || k != smith || field != "full.feature_choice_keys[1]" {
			t.Errorf("got %q %q %v", field, k, found)
		}
	})
}

// toolFitFixture is a background whose feature has two tool choices: one of thieves' tools or the herbalism kit, one of the
// smith's tools. The sheet holds thieves' tools and the herbalism kit: the matching takes one of them and leaves the other.
func toolFitFixture(t *testing.T) (*Content, Build, string) {
	t.Helper()
	const thieves, herbs, smith = "proficiency:thieves-tools", "proficiency:herbalism-kit", "proficiency:smiths-tools"
	o := filterBackgroundWith(Effect{Type: "choice", Choice: "tool", Count: 1, From: []string{thieves, herbs}})
	o.Backgrounds[0].Feature.Effects = append(o.Backgrounds[0].Feature.Effects, Effect{Type: "choice", Choice: "tool", Count: 1, From: []string{smith}})
	c := withOverlay(t, o)
	b := filterBuild()
	b.FeatureChoices = []string{ScopedChoice(filterPickKey(), thieves), ScopedChoice(filterPickKey(), herbs)}
	return c, b, filterPickKey()
}

func missingTools(d Derived) int {
	n := 0
	for _, oc := range d.OpenChoices {
		if oc.Kind == OpenChoiceTools {
			n += oc.Missing
		}
	}
	return n
}

// TestHeldToolPickTheMatchingLeavesOutStillCounts: a stored sheet that does not fit plays. A held tool pick that the
// placement leaves out counts as held, as the choice counts it, and still gives its proficiency; the master's fit issue
// is the only trace.
func TestHeldToolPickTheMatchingLeavesOutStillCounts(t *testing.T) {
	t.Parallel()
	c, b, key := toolFitFixture(t)
	d := Derive(b, c)
	if n := missingTools(d); n != 0 {
		t.Errorf("Derive asks for %d more tools, the choice holds both", n)
	}
	for _, is := range d.Issues {
		if is.Code == IssueMissing && strings.Contains(is.Message, "ferramenta") {
			t.Errorf("missing-tools issue: %s", is.Message)
		}
	}
	for _, p := range []string{"proficiency:thieves-tools", "proficiency:herbalism-kit"} {
		if !slices.ContainsFunc(d.Proficiencies, func(x Proficiency) bool { return x.Key == p }) {
			t.Errorf("the held pick %s gives no proficiency", p)
		}
	}
	for _, g := range c.Choices(b).Groups {
		for _, ch := range g.Choices {
			if ch.Key == key && (ch.Picks != 2 || ch.Missing() != 0) {
				t.Errorf("the choice holds %d of %d", ch.Done(), ch.Picks)
			}
		}
	}
	if is := fitIssues(d); len(is) != 1 {
		t.Errorf("the master's fit issues = %v, want one", is)
	}
	if is := fitIssues(Derive(b, c.ForPlayer())); len(is) != 0 {
		t.Errorf("a player's fit issues = %v", is)
	}
}

// TestUnfitStoredSheetLevelsUp: a stored sheet whose picks do not fit still levels up. The level adds a source, so the pick
// the fit issue names changes; the issue never blocks the level, and the new skill is refused only when no source of the
// level takes it.
func TestUnfitStoredSheetLevelsUp(t *testing.T) {
	t.Parallel()
	race := TableRace{
		Key: "race:perito@mesa", NamePT: "Perito", Revision: 1, Size: "Medium", SpeedFt: 30,
		Traits: []TableFeature{{Key: "trait:faro@mesa", NamePT: "Faro", Effects: []Effect{{Type: "choice", Choice: "skill", Count: 1, From: []string{"skill:medicine"}}}}},
	}
	c := withOverlay(t, Overlay{Revision: 1, Races: []TableRace{race}, Subclasses: []TableSubclass{{
		TableEntry: TableEntry{Key: sourceSub, NamePT: "Mestre do saber"}, Class: "class:fighter",
		Levels: []TableSubclassLevel{{Level: 3, Features: []TableFeature{{
			Key: "feature:estudo@mesa", NamePT: "Estudo", DescPT: []string{"Uma perícia à escolha."},
			Effects: []Effect{{Type: "choice", Choice: "skill", Count: 1, From: []string{"skill:stealth", "skill:arcana"}}},
		}}}},
	}}})
	before := sweepUp(t, c, sweepBase(t, c, "class:fighter", ""), "class:fighter", sourceSub, 2)
	before.Race = race.Key
	before.SkillProficiencies = []string{"skill:acrobatics", "skill:animal-handling", "skill:stealth"}
	if is := fitIssues(Derive(before, c)); len(is) != 1 {
		t.Fatalf("the stored sheet's fit issues = %v, want one", is)
	}
	ch := satisfy(t, c, before, "class:fighter", sourceSub)
	ch.SkillProficiencies = []string{"skill:arcana"}
	after, err := ApplyLevelUp(before, ch, c)
	if err != nil {
		t.Fatal(err)
	}
	if err := CheckLevelUpOf(before, after, ch, c); err != nil {
		t.Errorf("the level up is refused: %v", err)
	}
	// The held pick the stored sheet could not place does not take the slot of the level's new source from the new pick.
	if field, key, found := c.UnplacedNewPick(&before, after); found {
		t.Errorf("the new pick is refused: %s %s", field, key)
	}
	ch.SkillProficiencies = []string{"skill:history"}
	after, err = ApplyLevelUp(before, ch, c)
	if err != nil {
		t.Fatal(err)
	}
	wantRefusal(t, CheckLevelUpOf(before, after, ch, c), LevelUpReasonSkills, "full.skill_proficiency_keys")
}

// TestToolFitIssue: the tool fit issue is the master's, comes once the owner's picks are as many as its choices ask, and
// names the tool by its name.
func TestToolFitIssue(t *testing.T) {
	t.Parallel()
	c, b, _ := toolFitFixture(t)
	is := fitIssues(Derive(b, c))
	if len(is) != 1 || is[0].Field != choiceKeysField || !strings.Contains(is[0].Message, "ferramenta") {
		t.Fatalf("fit issues = %v, want one about a tool", is)
	}
	short := b.clone()
	short.FeatureChoices = short.FeatureChoices[:1] // one pick of two asked: the count is not right, so no fit issue
	if is := fitIssues(Derive(short, c)); len(is) != 0 {
		t.Errorf("a sheet short of picks has fit issues: %v", is)
	}
	if is := fitIssues(Derive(b, c.ForPlayer())); len(is) != 0 {
		t.Errorf("a player's fit issues = %v", is)
	}
}

// TestHeldToolPicksOverTheCountAreNotCounted: the background offers one tool from thieves' tools or the herbalism kit and a
// race trait one smith's tool; the sheet holds thieves' tools and the herbalism kit and nothing for the trait. The herbalism
// kit is over the background's count, so it is neither counted nor proficient, and the trait's pick is still owed: the
// choice and Derive ask the same one.
func TestHeldToolPicksOverTheCountAreNotCounted(t *testing.T) {
	t.Parallel()
	const thieves, herbs, smith = "proficiency:thieves-tools", "proficiency:herbalism-kit", "proficiency:smiths-tools"
	o := filterBackgroundWith(Effect{Type: "choice", Choice: "tool", Count: 1, From: []string{thieves, herbs}})
	o.Races = []TableRace{{
		Key: "race:perito@mesa", NamePT: "Perito", Revision: 1, Size: "Medium", SpeedFt: 30,
		Traits: []TableFeature{{Key: "trait:faro@mesa", NamePT: "Faro", Effects: []Effect{{Type: "choice", Choice: "tool", Count: 1, From: []string{smith}}}}},
	}}
	c := withOverlay(t, o)
	b := filterBuild()
	b.Race = "race:perito@mesa"
	b.FeatureChoices = []string{ScopedChoice(filterPickKey(), thieves), ScopedChoice(filterPickKey(), herbs)}
	for name, view := range map[string]*Content{"master": c, "player": c.ForPlayer()} {
		d := Derive(b, view)
		missing := 0
		for _, g := range view.Choices(b).Groups {
			for _, ch := range g.Choices {
				if strings.HasSuffix(ch.Key, toolPickSuffix) {
					missing += ch.Missing()
				}
			}
		}
		if got := missingTools(d); got != 1 || missing != 1 {
			t.Errorf("%s: Derive misses %d tools, the choices %d, want 1 and 1", name, got, missing)
		}
		has := func(k string) bool {
			return slices.ContainsFunc(d.Proficiencies, func(p Proficiency) bool { return p.Key == k })
		}
		if !has(thieves) || has(herbs) {
			t.Errorf("%s: thieves proficient %v, herbs proficient %v", name, has(thieves), has(herbs))
		}
	}
}

// TestLevelUpExpertiseIsMatchedToTheLevelsSources: a new expertise outside the list of the level's choice is refused by the
// level's own check, at the expertise list.
func TestLevelUpExpertiseIsMatchedToTheLevelsSources(t *testing.T) {
	t.Parallel()
	c := withOverlay(t, Overlay{Revision: 1, Subclasses: []TableSubclass{{
		TableEntry: TableEntry{Key: sourceSub, NamePT: "Mestre do saber"}, Class: "class:fighter",
		Levels: []TableSubclassLevel{{Level: 3, Features: []TableFeature{{
			Key: "feature:estudo@mesa", NamePT: "Estudo", DescPT: []string{"Uma especialização à escolha."},
			Effects: []Effect{{Type: "choice", Choice: "expertise", Count: 1, From: []string{"skill:acrobatics"}}},
		}}}},
	}}})
	before := sweepUp(t, c, sweepBase(t, c, "class:fighter", ""), "class:fighter", sourceSub, 2)
	ch := satisfy(t, c, before, "class:fighter", sourceSub)
	for skill, ok := range map[string]bool{"skill:acrobatics": true, "skill:animal-handling": false} {
		ch.Expertise = []string{skill}
		after, err := ApplyLevelUp(before, ch, c)
		if err != nil {
			t.Fatalf("%s: ApplyLevelUp: %v", skill, err)
		}
		err = CheckLevelUpOf(before, after, ch, c)
		if ok && err != nil {
			t.Errorf("%s: refused: %v", skill, err)
		}
		if !ok {
			wantRefusal(t, err, LevelUpReasonExpertise, "full.expertise_skill_keys")
		}
	}
}

// TestUnplacedNewListNamesAPickTheRefusalIsTrueFor: with more new picks than room the named pick is one the sentence is
// true for. A pick no source lists is named before a pick that only competes for room, and with one pick over the room
// the named pick is one whose removal makes the rest fit.
func TestUnplacedNewListNamesAPickTheRefusalIsTrueFor(t *testing.T) {
	t.Parallel()
	known := func(string) bool { return true }
	t.Run("a pick on no list is named before one that competes for room", func(t *testing.T) {
		t.Parallel()
		sources := []pickSource{{Owner: "x", Count: 1, From: []string{"skill:a", "skill:b"}}}
		_, key, found := unplacedNewList("full.skill_proficiency_keys", sources, []string{"skill:a", "skill:b", "skill:z"}, nil, nil, known)
		if !found || key != "skill:z" {
			t.Errorf("named %q (found %v), want skill:z: no placement takes it and removing a or b leaves it out", key, found)
		}
	})
	t.Run("one pick over the room: the named pick's removal makes the rest fit", func(t *testing.T) {
		t.Parallel()
		// Any pick named here leaves the rest placed, so this holds on a version that names the first in sheet order too.
		sources := []pickSource{{Owner: "x", Count: 1, From: []string{"skill:a", "skill:b"}}, {Owner: "y", Count: 1, From: []string{"skill:c"}}}
		list := []string{"skill:a", "skill:b", "skill:c"}
		field, key, found := unplacedNewList("full.skill_proficiency_keys", sources, list, nil, nil, known)
		if !found {
			t.Fatal("no pick named")
		}
		rest := slices.DeleteFunc(slices.Clone(list), func(k string) bool { return k == key })
		if left := unplacedPicks(sources, rest); len(left) != 0 {
			t.Errorf("named %q at %q: the rest %v still leaves %v out", key, field, rest, left)
		}
	})
	t.Run("a held pick that fits is never the one named", func(t *testing.T) {
		t.Parallel()
		sources := []pickSource{{Owner: "x", Count: 1, From: []string{"skill:a", "skill:b", "skill:c"}}}
		_, key, found := unplacedNewList("full.skill_proficiency_keys", sources, []string{"skill:a", "skill:b", "skill:c"}, []string{"skill:a"}, nil, known)
		if !found || key == "skill:a" {
			t.Errorf("named %q (found %v), want a new pick", key, found)
		}
	})
}

// TestUnplacedNewToolNamesAPickTheRefusalIsTrueFor is the tool case: the tool no choice of its owner lists is named
// before one that only competes for room, wherever it stands in the sheet.
func TestUnplacedNewToolNamesAPickTheRefusalIsTrueFor(t *testing.T) {
	t.Parallel()
	const thieves, herbs, smith = "proficiency:thieves-tools", "proficiency:herbalism-kit", "proficiency:smiths-tools"
	o := filterBackgroundWith(Effect{Type: "choice", Choice: "tool", Count: 1, From: []string{thieves, herbs}})
	c := withOverlay(t, o)
	key := filterPickKey()
	held := filterBuild()
	now := held.clone()
	now.FeatureChoices = []string{ScopedChoice(key, thieves), ScopedChoice(key, herbs), ScopedChoice(key, smith)}
	field, k, found := c.UnplacedNewPick(&held, now)
	if !found || k != smith || field != "full.feature_choice_keys[2]" {
		t.Errorf("got %q %q %v, want the smith's tools at [2]: thieves and herbs compete for one place, smith's is on no list", field, k, found)
	}
}
