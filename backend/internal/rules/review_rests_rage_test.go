package rules

import "testing"

// A level 20 barbarian has unlimited rages (SRD 5.1, Barbarian table). The app
// stores 99 uses (actions.go: "A barbarian's unlimited rage is 99"), so the
// sheet shows a counter of 99 uses. No document names this as a decision.
func TestRulesReviewRests_UnlimitedRageIsNotACounterOf99(t *testing.T) {
	c, err := LoadSRD()
	if err != nil {
		t.Fatal(err)
	}
	b := pensantus()
	b.Race, b.Subrace = "race:human", ""
	b.Classes = []ClassLevel{{Class: "class:barbarian", Level: 20}}
	d := Derive(b, c)
	for _, r := range d.Resources {
		if r.Key == "rage" && r.Max == 99 {
			t.Errorf("barbarian 20: rage is a counter of %d uses; the SRD says unlimited, so the sheet should not show a number of uses", r.Max)
		}
	}
}
