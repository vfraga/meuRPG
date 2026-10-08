package rules

import "testing"

func TestRulesReviewRests_BardicInspirationRecharge(t *testing.T) {
	c, err := LoadSRD()
	if err != nil {
		t.Fatal(err)
	}
	recharge := func(level int) string {
		b := Build{
			BaseScores: map[Ability]int{STR: 8, DEX: 14, CON: 12, INT: 10, WIS: 10, CHA: 16},
			Race:       "race:human",
			Classes:    []ClassLevel{{Class: "class:bard", Level: level}},
		}
		for _, r := range Derive(b, c).Resources {
			if r.Key == "bardic_inspiration" {
				return r.Recharge
			}
		}
		t.Fatalf("bard level %d: no bardic_inspiration resource", level)
		return ""
	}
	if got := recharge(1); got != RechargeLongRest {
		t.Errorf("bard 1: Bardic Inspiration recharge = %q, want %q", got, RechargeLongRest)
	}
	// SRD 5.1, Bard, Font of Inspiration (level 5): uses also return on a short rest.
	if got := recharge(5); got != RechargeShortRest {
		t.Errorf("bard 5: Bardic Inspiration recharge = %q, want %q (SRD 5.1 Font of Inspiration: from level 5 uses regained on a short or long rest)", got, RechargeShortRest)
	}
}
