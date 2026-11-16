package rules

import (
	"slices"
	"testing"
)

// Preflight #253, A1: a lost prerequisite never blocks a level-up. Its sentence says what the character has ("está
// no 3"), which changes with the level; the issue is the same one, told apart by its code, field and entry.

const (
	featKeyLevel8 = "feat:nivel-8" + tableSuffix
	featKeyStr20  = "feat:forca-20" + tableSuffix
)

func lostLevelUpContent(t *testing.T) *Content {
	t.Helper()
	o := gatedPostureOverlay()
	feat := func(key, name string, p FeatPrerequisite) TableFeat {
		return TableFeat{TableEntry: TableEntry{Key: key, NamePT: name}, DescPT: []string{"Texto."}, Prerequisite: p}
	}
	o.Feats = []TableFeat{
		feat(featKeyLevel8, "Veterano", FeatPrerequisite{Level: 8}),
		feat(featKeyStr20, "Colosso", FeatPrerequisite{Minimums: map[Ability]int{STR: 20}}),
	}
	return withOverlay(t, o)
}

func hasIssueOn(d Derived, code, field string) bool {
	return slices.ContainsFunc(d.Issues, func(i Issue) bool { return i.Code == code && i.Field == field })
}

func TestALostPrerequisiteDoesNotBlockALevelUpThatStaysShortOfIt(t *testing.T) {
	t.Parallel()
	c := lostLevelUpContent(t)
	increase := LevelUpChoices{Class: "class:fighter", AbilityIncrease: map[Ability]int{STR: 2}, HitPoints: LevelUpHitPoints{Average: true}}
	cases := []struct {
		name  string
		edit  func(*Build)
		code  string
		field string
	}{
		{"an option that asks for a class level", func(*Build) {}, IssueOptionPrerequisite, "full.feature_choice_keys[0]"},
		{"a feat that asks for a total level", func(b *Build) { b.Feats = []string{featKeyLevel8} }, IssueFeatPrerequisite, "full.feat_keys[0]"},
		{"a feat that asks for a score the increase stays short of", func(b *Build) { b.Feats = []string{featKeyStr20} }, IssueFeatPrerequisite, "full.feat_keys[0]"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			before := postureBuild(3, postureBull, postureHeron, postureCrow, "feature:fighter-fighting-style-defense")
			tc.edit(&before)
			d3 := Derive(before, c)
			if !hasIssueOn(d3, tc.code, tc.field) {
				t.Fatalf("issues before = %+v, want %s at %s", d3.Issues, tc.code, tc.field)
			}
			after, err := ApplyLevelUp(before, increase, c)
			if err == nil {
				err = CheckLevelUp(before, after, c)
			}
			if err != nil {
				t.Fatalf("the level-up of a sheet with a lost prerequisite was refused: %v", err)
			}
			d4 := Derive(after, c)
			if !hasIssueOn(d4, tc.code, tc.field) {
				t.Errorf("issues after = %+v, want %s still at %s", d4.Issues, tc.code, tc.field)
			}
			var m3, m4, id3, id4 string
			for _, is := range d3.Issues {
				if is.Code == tc.code {
					m3, id3 = is.Message, is.ID()
				}
			}
			for _, is := range d4.Issues {
				if is.Code == tc.code {
					m4, id4 = is.Message, is.ID()
				}
			}
			if m3 == m4 {
				t.Errorf("the sentence did not change with the level (%q): the case tests nothing", m3)
			}
			if id3 != id4 || id3 == "" {
				t.Errorf("the issue's identity changed with its sentence: %q then %q", id3, id4)
			}
		})
	}
}

// The level-up that reaches the prerequisite goes through and the issue is gone.
func TestTheLevelUpThatReachesTheLostPrerequisiteClearsTheIssue(t *testing.T) {
	t.Parallel()
	c := lostLevelUpContent(t)
	l4, err := ApplyLevelUp(postureBuild(3, postureBull, postureHeron, postureCrow, "feature:fighter-fighting-style-defense"), LevelUpChoices{Class: "class:fighter", AbilityIncrease: map[Ability]int{STR: 2}, HitPoints: LevelUpHitPoints{Average: true}}, c)
	if err != nil {
		t.Fatal(err)
	}
	l5, err := ApplyLevelUp(l4, LevelUpChoices{Class: "class:fighter", HitPoints: LevelUpHitPoints{Average: true}}, c)
	if err == nil {
		err = CheckLevelUp(l4, l5, c)
	}
	if err != nil {
		t.Fatalf("the level-up to the level the option asks was refused: %v", err)
	}
	if d := Derive(l5, c); slices.ContainsFunc(d.Issues, func(i Issue) bool { return i.Code == IssueOptionPrerequisite }) {
		t.Errorf("the issue stays at the level the option asks: %+v", d.Issues)
	}
}

// An issue with a sentence only is told apart by it, as before; a lost prerequisite by its entry.
func TestAnIssuesIdentity(t *testing.T) {
	t.Parallel()
	plain := Issue{Code: "c", Field: "f", Message: "m"}
	if plain.ID() != "c|f|m" {
		t.Errorf("plain ID = %q", plain.ID())
	}
	lost := Issue{Code: IssueFeatPrerequisite, Field: "full.feat_keys[0]", Message: "X pede…", Lost: &LostPrerequisite{Key: "feat:x@mesa", EntryPT: "X"}}
	if got, want := lost.ID(), IssueFeatPrerequisite+"|full.feat_keys|lost:feat:x@mesa"; got != want {
		t.Errorf("lost ID = %q, want %q", got, want)
	}
}
