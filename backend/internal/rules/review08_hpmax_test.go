// Finding U8-2 (see review/unit-08-rules-content.md).
package rules

import "testing"

// TestReview08_HitPointsMaxCanGoBelowOne: a table feature with an "hp.max"
// modifier that is negative (or sets 0) leaves the sheet with max HP < 1.
func TestReview08_HitPointsMaxCanGoBelowOne(t *testing.T) {
	t.Parallel()
	srd := loadForTest(t)

	derive := func(t *testing.T, e Effect) Derived {
		t.Helper()
		tc := genClass(srd, genKinds[0])
		tc.Levels[0].Features = append(tc.Levels[0].Features, tf("review08", "Review08", e))
		c, err := srd.With(Overlay{Classes: []TableClass{tc}})
		if err != nil {
			t.Fatalf("With refused the effect: %v", err)
		}
		b := sweepBase(t, c, tc.Key, "")
		return Derive(b, c)
	}

	t.Run("add -1000", func(t *testing.T) {
		d := derive(t, Effect{Type: "modifier", Target: "hp.max", Mode: "add", Value: "-1000"})
		if d.HitPointsMax < 1 {
			t.Errorf("HitPointsMax = %d, want >= 1", d.HitPointsMax)
		}
	})
	t.Run("set 0", func(t *testing.T) {
		d := derive(t, Effect{Type: "modifier", Target: "hp.max", Mode: "set", Value: "0"})
		if d.HitPointsMax < 1 {
			t.Errorf("HitPointsMax = %d, want >= 1", d.HitPointsMax)
		}
	})
	t.Run("resource max -3", func(t *testing.T) {
		d := derive(t, Effect{Type: "resource", Resource: "neg", Max: "-3", Recharge: "long_rest"})
		for _, r := range d.Resources {
			if r.Max < 1 {
				t.Errorf("resource %s has Max %d", r.Key, r.Max)
			}
		}
		t.Logf("resources: %+v", d.Resources)
	})
}
