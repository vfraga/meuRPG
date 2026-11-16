package rules

import (
	"slices"
	"testing"
)

// Preflight #253, A4: a lost feat taken through a choice is named at its pick, "full.feature_choice_keys[n]", so the
// sheet marks the feat's row; the service's row says the changed notice reads the feat's key from it.
func TestALostFeatTakenThroughAChoiceIsNamedAtItsPick(t *testing.T) {
	t.Parallel()
	c, race := featChoiceTable(t)
	b := featChoiceBuild(race, punhoFeat)
	b.BaseScores = map[Ability]int{STR: 8, DEX: 14, CON: 13, INT: 12, WIS: 10, CHA: 15}
	at := slices.Index(b.FeatureChoices, ScopedChoice(FeatChoiceKey(talentoTrait), punhoFeat))
	if at != 1 {
		t.Fatalf("the pick is at %d, want 1 (after the fighting style): %v", at, b.FeatureChoices)
	}
	is := lostIssue(t, Derive(b, c), IssueFeatPrerequisite)
	if want := "full.feature_choice_keys[1]"; is.Field != want {
		t.Errorf("field = %q, want %q", is.Field, want)
	}
	if want := IssueFeatPrerequisite + "|full.feature_choice_keys|lost:" + punhoFeat; is.ID() != want {
		t.Errorf("ID = %q, want %q", is.ID(), want)
	}
	if !KnownIssues([]string{IssueFeatPrerequisite + "|full.feature_choice_keys|O personagem não cumpre mais o pré-requisito de Punho: Força 15."}, []Issue{is})[0] {
		t.Error("the identity stored before the pick had an index is not read as this issue")
	}
}
