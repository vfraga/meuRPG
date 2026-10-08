package rules

import (
	"errors"
	"strings"
	"testing"
)

// choiceOverlay is a table class whose level 2 feature carries one choice
// effect e.
func choiceOverlay(t testing.TB, c *Content, e Effect) (Overlay, string) {
	t.Helper()
	tc := genClass(c, genKinds[0])
	tc.Levels[1].Features = []TableFeature{tf("gen-none-pick", "Escolha", e)}
	return Overlay{Revision: 1, Classes: []TableClass{tc}}, tc.Key
}

// With must refuse a choice that asks for more than can be picked.
func TestChoiceCountBeyondWhatCanBePickedIsRefused(t *testing.T) {
	two := []string{"feature:fighter-fighting-style-defense", "feature:fighter-fighting-style-dueling"}
	cases := map[string]Effect{
		"feature 5 of 2":             {Type: "choice", Choice: "feature", Count: 5, From: two},
		"skill past the maximum":     {Type: "choice", Choice: "skill", Count: MaxChoiceCount + 1},
		"expertise past the maximum": {Type: "choice", Choice: "expertise", Count: MaxChoiceCount + 1},
		"cantrip past the maximum":   {Type: "choice", Choice: "cantrip", Count: MaxChoiceCount + 1},
	}
	for name, e := range cases {
		t.Run(name, func(t *testing.T) {
			base := loadForTest(t)
			o, _ := choiceOverlay(t, base, e)
			_, err := base.With(o)
			if err == nil {
				t.Fatalf("With accepted a choice of count %d", e.Count)
			}
			var oe *OverlayError
			if !errors.As(err, &oe) {
				t.Fatalf("want *OverlayError, got %T: %v", err, err)
			}
			found := false
			for _, v := range oe.Violations() {
				if strings.HasSuffix(v.Field, ".count") {
					found = true
				}
			}
			if !found {
				t.Fatalf("no violation on a .count field: %v", err)
			}
		})
	}
	t.Run("control: the largest counts stay accepted", func(t *testing.T) {
		for name, e := range map[string]Effect{
			"feature 2 of 2": {Type: "choice", Choice: "feature", Count: 2, From: two},
			"skill maximum":  {Type: "choice", Choice: "skill", Count: MaxChoiceCount},
		} {
			base := loadForTest(t)
			o, _ := choiceOverlay(t, base, e)
			if _, err := base.With(o); err != nil {
				t.Errorf("%s was refused: %v", name, err)
			}
		}
	})
}

// A choice of features that asks for every option it lists still gives a valid
// level-up (the count equals the options), and one more than that is refused.
func TestChoiceOfAllItsOptionsLevelsUp(t *testing.T) {
	base := loadForTest(t)
	two := []string{"feature:fighter-fighting-style-defense", "feature:fighter-fighting-style-dueling"}
	o, classKey := choiceOverlay(t, base, Effect{Type: "choice", Choice: "feature", Count: 2, From: two})
	c, err := base.With(o)
	if err != nil {
		t.Fatalf("With refused a choice of 2 of 2: %v", err)
	}
	b := sweepBase(t, c, classKey, "")
	opts, err := LevelUpOptions(b, classKey, c)
	if err != nil {
		t.Fatalf("LevelUpOptions: %v", err)
	}
	if len(opts.FeatureChoices) != 1 {
		t.Fatalf("want 1 feature choice, got %+v", opts.FeatureChoices)
	}
	fc := opts.FeatureChoices[0]
	var keys []string
	for _, op := range fc.Options {
		keys = append(keys, op.Key)
	}
	ch := LevelUpChoices{Class: classKey, HitPoints: LevelUpHitPoints{Average: true}, FeatureChoices: keys}
	after, err := ApplyLevelUp(b, ch, c)
	if err != nil {
		t.Fatalf("level-up: Choose=%d with %d options; ApplyLevelUp: %v", fc.Choose, len(keys), err)
	}
	if err := CheckLevelUp(b, after, c); err != nil {
		t.Fatalf("level-up: Choose=%d with %d options (all picked): %v", fc.Choose, len(keys), err)
	}
}
