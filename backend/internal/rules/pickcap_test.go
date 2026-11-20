package rules

import (
	"slices"
	"testing"
)

// A source that asks for more picks than its lists can supply asks for as many as can be made.

func capTrait(key, kind string, count int, from ...string) TableFeature {
	return TableFeature{Key: key, NamePT: "Faro", Effects: []Effect{{Type: "choice", Choice: kind, Count: count, From: from}}}
}

// capSheet is a fighter of a table race with these traits and the skills and expertise held on top of the class's two.
func capSheet(t *testing.T, traits []TableFeature, skills, expertise []string) (*Content, Build) {
	t.Helper()
	race := TableRace{Key: "race:perito@mesa", NamePT: "Perito", Revision: 1, Size: "Medium", SpeedFt: 30, Traits: traits}
	c := withOverlay(t, Overlay{Revision: 1, Races: []TableRace{race}})
	b := sweepBase(t, c, "class:fighter", "")
	b.Race = race.Key
	b.SkillProficiencies = append(b.SkillProficiencies, skills...)
	b.Expertise = expertise
	return c, b
}

func missingOf(d Derived, kind string) int {
	n := 0
	for _, o := range d.OpenChoices {
		if o.Kind == kind {
			n += o.Missing
		}
	}
	return n
}

func countIssues(d Derived) []Issue {
	var out []Issue
	for _, is := range d.Issues {
		if is.Code == IssueSkillCount || is.Code == IssueExpertise || is.Code == IssueMissing {
			out = append(out, is)
		}
	}
	return out
}

func TestACountPastWhatTheListCanSupplyIsCompleteWithEveryKeyOfTheList(t *testing.T) {
	t.Parallel()
	c, b := capSheet(t, []TableFeature{capTrait("trait:faro@mesa", "skill", 2, "skill:arcana")}, []string{"skill:arcana"}, nil)
	for name, view := range map[string]*Content{"master": c, "player": c.ForPlayer()} {
		d := Derive(b, view)
		if got := missingOf(d, OpenChoiceSkills); got != 0 {
			t.Errorf("%s: open skills = %d, want none", name, got)
		}
		if is := countIssues(d); len(is) != 0 {
			t.Errorf("%s: count issues = %v", name, is)
		}
	}
	_, without := capSheet(t, []TableFeature{capTrait("trait:faro@mesa", "skill", 2, "skill:arcana")}, nil, nil)
	if got := missingOf(Derive(without, c), OpenChoiceSkills); got != 1 {
		t.Errorf("open skills before the pick = %d, want 1", got)
	}
}

func TestSourcesWithTheSameListAskForWhatTheListHolds(t *testing.T) {
	t.Parallel()
	traits := []TableFeature{
		capTrait("trait:um@mesa", "skill", 2, "skill:arcana", "skill:nature"),
		capTrait("trait:dois@mesa", "skill", 2, "skill:arcana", "skill:nature"),
	}
	c, none := capSheet(t, traits, nil, nil)
	if got := missingOf(Derive(none, c), OpenChoiceSkills); got != 2 {
		t.Errorf("open skills = %d, want 2", got)
	}
	_, held := capSheet(t, traits, []string{"skill:arcana", "skill:nature"}, nil)
	if d := Derive(held, c); missingOf(d, OpenChoiceSkills) != 0 || len(countIssues(d)) != 0 {
		t.Errorf("both keys held: open = %d, issues = %v", missingOf(d, OpenChoiceSkills), countIssues(d))
	}
}

func TestAnExpertiseCountPastWhatTheListCanSupplyIsComplete(t *testing.T) {
	t.Parallel()
	traits := []TableFeature{capTrait("trait:faro@mesa", "expertise", 2, "skill:arcana"), capTrait("trait:olho@mesa", "skill", 1, "skill:arcana")}
	c, b := capSheet(t, traits, []string{"skill:arcana"}, []string{"skill:arcana"})
	for name, view := range map[string]*Content{"master": c, "player": c.ForPlayer()} {
		d := Derive(b, view)
		if got := missingOf(d, OpenChoiceExpertise); got != 0 {
			t.Errorf("%s: open expertise = %d, want none", name, got)
		}
		if is := countIssues(d); len(is) != 0 {
			t.Errorf("%s: count issues = %v", name, is)
		}
	}
	_, without := capSheet(t, traits, []string{"skill:arcana"}, nil)
	if got := missingOf(Derive(without, c), OpenChoiceExpertise); got != 1 {
		t.Errorf("open expertise before the pick = %d, want 1", got)
	}
}

func TestASourceWithRoomAsksForItsFullCount(t *testing.T) {
	t.Parallel()
	c, b := capSheet(t, []TableFeature{capTrait("trait:faro@mesa", "skill", 2, "skill:arcana", "skill:nature", "skill:medicine")}, nil, nil)
	if got := missingOf(Derive(b, c), OpenChoiceSkills); got != 2 {
		t.Errorf("open skills = %d, want 2", got)
	}
	c, b = capSheet(t, []TableFeature{capTrait("trait:faro@mesa", "expertise", 2, "skill:arcana", "skill:nature")}, []string{"skill:arcana", "skill:nature"}, nil)
	if got := missingOf(Derive(b, c), OpenChoiceExpertise); got != 2 {
		t.Errorf("open expertise = %d, want 2", got)
	}
}

func TestACappedCountIsTheSizeOfTheLargestMatching(t *testing.T) {
	t.Parallel()
	pickable := []string{"a", "b", "c"}
	for name, tc := range map[string]struct {
		sources []pickSource
		want    int
	}{
		"overlapping lists":  {[]pickSource{{Count: 2, From: []string{"a", "b"}}, {Count: 2, From: []string{"a", "b"}}}, 2},
		"a list past count":  {[]pickSource{{Count: 3, From: []string{"a"}}}, 1},
		"a list that is any": {[]pickSource{{Count: 5, Any: true}}, 3},
		"a normal source":    {[]pickSource{{Count: 2, From: []string{"a", "b", "c"}}}, 2},
	} {
		if got := placedCount(tc.sources, pickable); got != tc.want {
			t.Errorf("%s: matching = %d, want %d", name, got, tc.want)
		}
	}
}

func capLevelUpFixture(t *testing.T, kind string) (*Content, Build) {
	t.Helper()
	c := withOverlay(t, Overlay{Revision: 1, Subclasses: []TableSubclass{{
		TableEntry: TableEntry{Key: sourceSub, NamePT: "Mestre do saber"}, Class: "class:fighter",
		Levels: []TableSubclassLevel{{Level: 3, Features: []TableFeature{{
			Key: "feature:estudo@mesa", NamePT: "Estudo", DescPT: []string{"Duas à escolha."},
			Effects: []Effect{{Type: "choice", Choice: kind, Count: 2, From: []string{"skill:arcana"}}},
		}}}},
	}}})
	before := sweepUp(t, c, sweepBase(t, c, "class:fighter", ""), "class:fighter", sourceSub, 2)
	if kind == "expertise" {
		before.SkillProficiencies = slices.Concat([]string{"skill:arcana"}, before.SkillProficiencies[1:]) // expertise asks for proficiency
	}
	return c, before
}

func TestTheLevelAsksForWhatTheListCanSupply(t *testing.T) {
	t.Parallel()
	for _, kind := range []string{"skill", "expertise"} {
		c, before := capLevelUpFixture(t, kind)
		o, err := LevelUpOptions(before, "class:fighter", c)
		if err != nil {
			t.Fatal(err)
		}
		i := slices.IndexFunc(o.Subclasses, func(s LevelUpSubclass) bool { return s.Key == sourceSub })
		if i < 0 {
			t.Fatalf("%s: subclass missing", kind)
		}
		got := o.Subclasses[i].SkillChoices
		if kind == "expertise" {
			got = o.Subclasses[i].ExpertiseChoices
		}
		if got != 1 {
			t.Errorf("%s: the level asks %d, want 1", kind, got)
		}
		ch := satisfy(t, c, before, "class:fighter", sourceSub)
		if kind == "expertise" {
			ch.Expertise = []string{"skill:arcana"}
		}
		after, err := ApplyLevelUp(before, ch, c)
		if err != nil {
			t.Fatalf("%s: ApplyLevelUp: %v", kind, err)
		}
		if err := CheckLevelUpOf(before, after, ch, c); err != nil {
			t.Errorf("%s: the level with one pick is refused: %v", kind, err)
		}
	}
}

// The level asks what the sheet after it may pick, less what the sheet holds: a held key can move to the new source.

func subclassWithFeature(effects ...Effect) TableSubclass {
	return TableSubclass{
		TableEntry: TableEntry{Key: sourceSub, NamePT: "Mestre do saber"}, Class: "class:fighter",
		Levels: []TableSubclassLevel{{Level: 3, Features: []TableFeature{{Key: "feature:estudo@mesa", NamePT: "Estudo", DescPT: []string{"Uma à escolha."}, Effects: effects}}}},
	}
}

func subclassAsked(t *testing.T, c *Content, b Build) (skills, expertise int) {
	t.Helper()
	o, err := LevelUpOptions(b, "class:fighter", c)
	if err != nil {
		t.Fatal(err)
	}
	i := slices.IndexFunc(o.Subclasses, func(s LevelUpSubclass) bool { return s.Key == sourceSub })
	if i < 0 {
		t.Fatal("subclass missing")
	}
	return o.SkillChoices + o.Subclasses[i].SkillChoices, o.ExpertiseChoices + o.Subclasses[i].ExpertiseChoices
}

func TestAHeldSkillThatMovesToTheNewSourceFreesAPick(t *testing.T) {
	t.Parallel()
	c := withOverlay(t, Overlay{Revision: 1, Subclasses: []TableSubclass{subclassWithFeature(Effect{Type: "choice", Choice: "skill", Count: 1, From: []string{"skill:athletics"}})}})
	before := sweepUp(t, c, sweepBase(t, c, "class:fighter", ""), "class:fighter", sourceSub, 2)
	before.SkillProficiencies = []string{"skill:athletics", "skill:acrobatics"}
	for name, view := range map[string]*Content{"master": c, "player": c.ForPlayer()} {
		if got, _ := subclassAsked(t, view, before); got != 1 {
			t.Errorf("%s: the level asks %d skills, want 1", name, got)
		}
	}
	ch := satisfy(t, c, before, "class:fighter", sourceSub)
	ch.SkillProficiencies = nil
	if err := CheckLevelUpOf(before, mustApply(t, c, before, ch), ch, c); err == nil {
		t.Error("the level with no skill is accepted")
	}
	ch.SkillProficiencies = []string{"skill:survival"}
	after := mustApply(t, c, before, ch)
	if err := CheckLevelUpOf(before, after, ch, c); err != nil {
		t.Fatalf("the level with the freed pick is refused: %v", err)
	}
	if got := missingOf(Derive(after, c), OpenChoiceSkills); got != 0 {
		t.Errorf("open skills after the level = %d, want none", got)
	}
}

func TestAHeldExpertiseThatMovesToTheNewSourceFreesAPick(t *testing.T) {
	t.Parallel()
	race := TableRace{
		Key: "race:perito@mesa", NamePT: "Perito", Revision: 1, Size: "Medium", SpeedFt: 30,
		Traits: []TableFeature{capTrait("trait:faro@mesa", "expertise", 1, "skill:athletics", "skill:acrobatics")},
	}
	c := withOverlay(t, Overlay{Revision: 1, Races: []TableRace{race}, Subclasses: []TableSubclass{subclassWithFeature(Effect{Type: "choice", Choice: "expertise", Count: 1, From: []string{"skill:athletics"}})}})
	before := sweepUp(t, c, sweepBase(t, c, "class:fighter", ""), "class:fighter", sourceSub, 2)
	before.Race = race.Key
	before.SkillProficiencies = []string{"skill:athletics", "skill:acrobatics"}
	before.Expertise = []string{"skill:athletics"}
	for name, view := range map[string]*Content{"master": c, "player": c.ForPlayer()} {
		if _, got := subclassAsked(t, view, before); got != 1 {
			t.Errorf("%s: the level asks %d expertise, want 1", name, got)
		}
	}
	ch := satisfy(t, c, before, "class:fighter", sourceSub)
	ch.Expertise = []string{"skill:acrobatics"}
	after := mustApply(t, c, before, ch)
	if err := CheckLevelUpOf(before, after, ch, c); err != nil {
		t.Fatalf("the level with the freed expertise is refused: %v", err)
	}
	if got := missingOf(Derive(after, c), OpenChoiceExpertise); got != 0 {
		t.Errorf("open expertise after the level = %d, want none", got)
	}
}

func TestAMulticlassListOverlappingTheLevelsFeatureAsksForWhatCanBePicked(t *testing.T) {
	t.Parallel()
	srd := loadForTest(t)
	class := genClass(srd, genKind{name: "ambos"})
	class.MulticlassSkillChoose, class.SkillFrom = 1, []string{"skill:arcana", "skill:nature"}
	class.Minimums = map[Ability]int{CON: 11}
	class.Levels[0].Features = append(class.Levels[0].Features, TableFeature{
		Key: "feature:estudo-um@mesa", NamePT: "Estudo", DescPT: []string{"Uma perícia."},
		Effects: []Effect{{Type: "choice", Choice: "skill", Count: 1, From: []string{"skill:arcana"}}},
	})
	c := withOverlayOn(t, srd, Overlay{Revision: 1, Classes: []TableClass{class}, Subclasses: genSubclasses(class)})
	b := wizardWith(map[Ability]int{STR: 10, DEX: 10, CON: 12, INT: 13, WIS: 10, CHA: 10})
	b.SkillProficiencies = []string{"skill:history", "skill:investigation", "skill:nature"}
	for name, view := range map[string]*Content{"master": c, "player": c.ForPlayer()} {
		o, err := LevelUpOptions(b, class.Key, view)
		if err != nil {
			t.Fatalf("%s: %v", name, err)
		}
		if o.Multiclass == nil || o.Multiclass.Skills == nil || o.Multiclass.Skills.Choose+o.SkillChoices != 1 {
			t.Errorf("%s: the level asks %+v and %d, want one skill in all", name, o.Multiclass, o.SkillChoices)
		}
	}
	ch := satisfy(t, c, b, class.Key, "")
	ch.SkillProficiencies = []string{"skill:arcana"}
	after := mustApply(t, c, b, ch)
	if err := CheckLevelUpOf(b, after, ch, c); err != nil {
		t.Fatalf("the level with arcana is refused: %v", err)
	}
}
