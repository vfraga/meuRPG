package rules

import (
	"errors"
	"strings"
	"testing"
)

// Finding U8-1 (see review/unit-08-rules-content.md).

// badChoiceOverlay is a table class whose level 2 feature carries one choice
// effect e.
func badChoiceOverlay(t testing.TB, c *Content, e Effect) (Overlay, string) {
	t.Helper()
	tc := genClass(c, genKinds[0])
	tc.Levels[1].Features = []TableFeature{tf("gen-none-bad", "Ruim", e)}
	return Overlay{Revision: 1, Classes: []TableClass{tc}}, tc.Key
}

// With must refuse a choice that asks for more than can be picked.
func TestReview08_ChoiceCountBeyondOptions(t *testing.T) {
	two := []string{"feature:fighter-fighting-style-defense", "feature:fighter-fighting-style-dueling"}
	cases := map[string]Effect{
		"feature 5 of 2":  {Type: "choice", Choice: "feature", Count: 5, From: two},
		"skill 1<<30":     {Type: "choice", Choice: "skill", Count: 1 << 30},
		"expertise 1<<30": {Type: "choice", Choice: "expertise", Count: 1 << 30},
		"cantrip 1<<30":   {Type: "choice", Choice: "cantrip", Count: 1 << 30},
	}
	for name, e := range cases {
		t.Run(name, func(t *testing.T) {
			base := loadForTest(t)
			o, _ := badChoiceOverlay(t, base, e)
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
}

// The consequence: With accepts it, and then no level-up to level 2 is valid.
func TestReview08_ChoiceCountBeyondOptionsBlocksLevelUp(t *testing.T) {
	base := loadForTest(t)
	two := []string{"feature:fighter-fighting-style-defense", "feature:fighter-fighting-style-dueling"}
	o, classKey := badChoiceOverlay(t, base, Effect{Type: "choice", Choice: "feature", Count: 5, From: two})
	c, err := base.With(o)
	if err != nil {
		t.Skipf("With already refuses it (fixed): %v", err)
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
		t.Fatalf("no valid level-up: Choose=%d with %d options; ApplyLevelUp: %v", fc.Choose, len(keys), err)
	}
	if err := CheckLevelUp(b, after, c); err != nil {
		t.Fatalf("no valid level-up: Choose=%d with %d options (all picked): %v", fc.Choose, len(keys), err)
	}
}
