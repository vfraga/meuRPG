package rules

import (
	"strings"
	"testing"
)

// findResourceBySource returns the first resource granted by a feature whose
// key starts with prefix.
func findResourceBySource(d Derived, prefix string) (Resource, bool) {
	for _, r := range d.Resources {
		if strings.HasPrefix(r.Source, prefix) {
			return r, true
		}
	}
	return Resource{}, false
}

// SRD 5.1 Fighter table: Indomitable has 1 use at level 9, 2 at 13 and 3 at
// 17, regained on a long rest. Nothing before level 9.
func TestRulesReviewRests_IndomitableUses(t *testing.T) {
	c := loadForTest(t)
	for _, tc := range []struct{ level, want int }{{8, 0}, {9, 1}, {13, 2}, {17, 3}, {20, 3}} {
		d := Derive(standard("class:fighter", tc.level), c)
		t.Logf("fighter %d resources: %+v", tc.level, d.Resources)
		r, ok := findResourceBySource(d, "feature:indomitable")
		if tc.want == 0 {
			if ok {
				t.Errorf("level %d: Indomitable should not be tracked yet, got %+v", tc.level, r)
			}
			continue
		}
		if !ok {
			t.Errorf("level %d: the fighter should track Indomitable with %d use(s), but no resource has a feature:indomitable source", tc.level, tc.want)
			continue
		}
		if r.Max != tc.want || r.Recharge != RechargeLongRest {
			t.Errorf("level %d: Indomitable want %d uses on a long rest, got Max=%d Recharge=%q", tc.level, tc.want, r.Max, r.Recharge)
		}
	}
}

// Other SRD features with a stated number of uses between rests.
func TestRulesReviewRests_OtherLimitedUseFeatures(t *testing.T) {
	c := loadForTest(t)
	for _, tc := range []struct {
		name, class, subclass string
		level                 int
		source                string
		recharges             []string
	}{
		{"monk wholeness of body", "class:monk", "", 6, "feature:wholeness-of-body", []string{RechargeLongRest}},
		{"paladin cleansing touch", "class:paladin", "", 14, "feature:cleansing-touch", []string{RechargeLongRest}},
		{"rogue stroke of luck", "class:rogue", "", 20, "feature:stroke-of-luck", []string{RechargeShortRest}},
		{"warlock dark one's own luck", "class:warlock", "subclass:fiend", 6, "feature:dark-ones-own-luck", []string{RechargeShortRest}},
		{"warlock hurl through hell", "class:warlock", "subclass:fiend", 14, "feature:hurl-through-hell", []string{RechargeLongRest}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			b := standard(tc.class, tc.level)
			b.Classes[0].Subclass = tc.subclass
			d := Derive(b, c)
			t.Logf("resources: %+v", d.Resources)
			r, ok := findResourceBySource(d, tc.source)
			if !ok {
				t.Fatalf("%s is a once-per-rest feature in the SRD but no resource has source %s", tc.name, tc.source)
			}
			if r.Max < 1 {
				t.Errorf("Max = %d, want at least 1", r.Max)
			}
			okR := false
			for _, w := range tc.recharges {
				okR = okR || r.Recharge == w
			}
			if !okR {
				t.Errorf("Recharge = %q, want one of %v", r.Recharge, tc.recharges)
			}
		})
	}
}
