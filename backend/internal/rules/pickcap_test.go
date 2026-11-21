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

// TestAListCoveredByAutomaticSkillsLetsThePlayerChooseAnother: SRD 5.1, a character who would gain the same proficiency
// from two sources chooses a different proficiency of the same kind. A trait whose whole list is given automatically (the
// acolyte's Insight) is a free choice of any other skill, in the asked number, in the lists and in the refusal.
func TestAListCoveredByAutomaticSkillsLetsThePlayerChooseAnother(t *testing.T) {
	t.Parallel()
	traits := []TableFeature{capTrait("trait:faro@mesa", "skill", 1, "skill:insight")}
	c, none := capSheet(t, traits, nil, nil)
	if got := missingOf(Derive(none, c), OpenChoiceSkills); got != 1 {
		t.Errorf("open skills before the extra pick = %d, want 1: the trait's Insight comes from the background", got)
	}
	_, held := capSheet(t, traits, []string{"skill:stealth"}, nil)
	for name, view := range map[string]*Content{"master": c, "player": c.ForPlayer()} {
		if d := Derive(held, view); missingOf(d, OpenChoiceSkills) != 0 || len(countIssues(d)) != 0 || len(fitIssues(d)) != 0 {
			t.Errorf("%s: open = %d, issues = %v, fit = %v", name, missingOf(d, OpenChoiceSkills), countIssues(d), fitIssues(d))
		}
		if field, key, found := view.UnplacedNewPick(&none, held); found {
			t.Errorf("%s: refused %q at %q: the covered list gives a free choice", name, key, field)
		}
	}
	if allowed, _ := c.PickLists(held); !allowed.Any {
		t.Errorf("skills allowed = %+v, want any skill: the covered list's choice takes one", allowed)
	}
	_, over := capSheet(t, traits, []string{"skill:stealth", "skill:arcana"}, nil)
	if _, key, found := c.ForPlayer().UnplacedNewPick(&none, over); !found || key == "" {
		t.Error("a pick past the covered list's one choice is refused")
	}
}

// TestALevelAsksForTheSkillOfAFeatItMakesValid: a sheet holds a feat whose level the character has not reached; the feat
// gives nothing yet (it is lost), so the level that reaches it asks for the skill the feat then lets the player choose, as
// the sheet after the level reads it.
func TestALevelAsksForTheSkillOfAFeatItMakesValid(t *testing.T) {
	t.Parallel()
	feat := TableFeat{
		TableEntry: TableEntry{Key: "feat:estudioso" + tableSuffix, NamePT: "Estudioso"}, DescPT: []string{"Texto."},
		Prerequisite: FeatPrerequisite{Level: 3},
		Effects:      []Effect{{Type: "choice", Choice: "skill", Count: 1}},
	}
	c := withOverlay(t, Overlay{Revision: 1, Feats: []TableFeat{feat}})
	b := sweepUp(t, c, sweepBase(t, c, "class:fighter", ""), "class:fighter", "", 2)
	b.Feats = []string{feat.Key}
	if is := Derive(b, c).Issues; !slices.ContainsFunc(is, func(i Issue) bool { return i.Code == IssueFeatPrerequisite }) {
		t.Fatalf("the feat is not lost at level 2: %v", is)
	}
	o, err := LevelUpOptions(b, "class:fighter", c)
	if err != nil {
		t.Fatal(err)
	}
	if o.SkillChoices != 1 {
		t.Errorf("the level asks %d skills, want 1: the feat is valid at level 3 and gives a choice", o.SkillChoices)
	}
}

// TestAStoredAutomaticSkillThatBecomesAPickIsNew: the stored sheet holds a skill its background gives; a save that changes
// the background to one that does not give it makes that skill a pick, a new one the lists must have room for.
func TestAStoredAutomaticSkillThatBecomesAPickIsNew(t *testing.T) {
	t.Parallel()
	c := loadForTest(t)
	stored := sweepBase(t, c, "class:fighter", "")
	stored.SkillProficiencies = append(stored.SkillProficiencies, "skill:religion") // the acolyte gives it; it is no pick of the stored sheet
	if _, _, found := c.UnplacedNewPick(&stored, stored); found {
		t.Fatal("the stored sheet is refused against itself")
	}
	now := stored.clone()
	now.Background = "background:soldier"
	field, key, found := c.UnplacedNewPick(&stored, now)
	if !found || key != "skill:religion" || field != "full.skill_proficiency_keys[2]" {
		t.Errorf("got %q %q %v, want religion refused at [2]: it is a pick now and no list has room for it", field, key, found)
	}
	later := now.clone()
	later.Background = "background:acolyte"
	if field, key, found := c.UnplacedNewPick(&stored, later); found {
		t.Errorf("the background that still gives it refused %q at %q", key, field)
	}
}
