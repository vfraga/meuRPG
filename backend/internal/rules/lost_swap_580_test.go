package rules

import (
	"slices"
	"testing"
)

// Preflight #580, round 2: a lost prerequisite is the same issue when the pick that holds it moves. A swap at the
// level-up takes the old pick out of the list and appends the new one, so the picks after it move down by one.

func lostIDOf(t *testing.T, d Derived, code string) string {
	t.Helper()
	return lostIssue(t, d, code).ID()
}

func swapStyle() LevelUpChoices {
	return LevelUpChoices{
		Class: "class:fighter", AbilityIncrease: map[Ability]int{DEX: 2}, HitPoints: LevelUpHitPoints{Average: true},
		Swap: PickSwap{Group: SwapGroupFightStyle, From: styleDefense, To: styleDueling},
	}
}

func TestALostOptionStaysTheSameIssueWhenAnEarlierPickIsSwapped(t *testing.T) {
	t.Parallel()
	c := lostLevelUpContent(t)
	before := postureBuild(3, styleDefense, postureBull, postureHeron, postureCrow)
	dBefore := Derive(before, c)
	if !hasIssueOn(dBefore, IssueOptionPrerequisite, "full.feature_choice_keys[1]") {
		t.Fatalf("issues before = %+v, want the Bull lost at pick 1", dBefore.Issues)
	}
	after, err := ApplyLevelUp(before, swapStyle(), c)
	if err == nil {
		err = CheckLevelUp(before, after, c)
	}
	if err != nil {
		t.Fatalf("the level-up that swaps a pick before the lost option was refused: %v", err)
	}
	if at := slices.Index(after.FeatureChoices, postureBull); at != 0 {
		t.Fatalf("the Bull is at pick %d after the swap, want 0: the case tests nothing (%v)", at, after.FeatureChoices)
	}
	if b, a := lostIDOf(t, dBefore, IssueOptionPrerequisite), lostIDOf(t, Derive(after, c), IssueOptionPrerequisite); b != a {
		t.Errorf("the identity moved with the pick: %q then %q", b, a)
	}
}

func TestALostFeatTakenThroughAChoiceStaysTheSameIssueWhenAnEarlierPickIsSwapped(t *testing.T) {
	t.Parallel()
	c, race := featChoiceTable(t)
	before := featChoiceBuild(race, punhoFeat)
	before.Classes[0].Level, before.Classes[0].Subclass = 3, "subclass:champion"
	before.BaseScores = map[Ability]int{STR: 8, DEX: 14, CON: 13, INT: 12, WIS: 10, CHA: 15}
	before.SkillProficiencies = []string{"skill:acrobatics", "skill:perception"}
	dBefore := Derive(before, c)
	if is := lostIssue(t, dBefore, IssueFeatPrerequisite); is.Field != "full.feature_choice_keys[1]" {
		t.Fatalf("the lost feat is at %q, want pick 1", is.Field)
	}
	after, err := ApplyLevelUp(before, swapStyle(), c)
	if err == nil {
		err = CheckLevelUp(before, after, c)
	}
	if err != nil {
		t.Fatalf("the level-up that swaps a pick before the lost feat was refused: %v", err)
	}
	dAfter := Derive(after, c)
	if is := lostIssue(t, dAfter, IssueFeatPrerequisite); is.Field != "full.feature_choice_keys[0]" {
		t.Fatalf("the lost feat is at %q after the swap, want pick 0: the case tests nothing", is.Field)
	}
	if b, a := lostIDOf(t, dBefore, IssueFeatPrerequisite), lostIDOf(t, dAfter, IssueFeatPrerequisite); b != a {
		t.Errorf("the identity moved with the pick: %q then %q", b, a)
	}
}
