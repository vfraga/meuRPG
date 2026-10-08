package rules

import (
	"fmt"
	"testing"
)

// TestRulesReviewExpressions_CultFanaticHitPoints: SRD 5.1 Cult Fanatic has 33
// hit points (6d8 + 6; Constitution 12 = +1 per die).
func TestRulesReviewExpressions_CultFanaticHitPoints(t *testing.T) {
	t.Parallel()
	c := loadForTest(t)
	cr, ok := c.CreatureByKey("monster:cult-fanatic")
	if !ok {
		t.Fatal("monster:cult-fanatic not found")
	}
	if cr.HitPoints != 33 || cr.HitPointsRoll != "6d8+6" {
		t.Errorf("Cult Fanatic: app serves hit_points=%d roll=%q; SRD 5.1 says 33 and %q (6d8 + 6)", cr.HitPoints, cr.HitPointsRoll, "6d8+6")
	}
}

// TestRulesReviewExpressions_CreatureHitPointsFormula: every creature's hit
// points are the average of its dice plus (dice x Constitution modifier).
func TestRulesReviewExpressions_CreatureHitPointsFormula(t *testing.T) {
	t.Parallel()
	c := loadForTest(t)
	list, err := c.ListCreatures(CreatureFilter{})
	if err != nil {
		t.Fatal(err)
	}
	if len(list) < 300 {
		t.Fatalf("only %d creatures listed", len(list))
	}
	for _, e := range list {
		cr, _ := c.CreatureByKey(e.Key)
		var n, s, b int
		var sign string
		var got int
		if k, _ := fmt.Sscanf(cr.HitPointsRoll, "%dd%d%1s%d", &n, &s, &sign, &b); k == 2 {
			sign, b = "+", 0
		} else if k != 4 {
			t.Errorf("%s: unparsable roll %q", e.Key, cr.HitPointsRoll)
			continue
		}
		if sign == "-" {
			b = -b
		}
		con := 10
		for _, a := range cr.Abilities {
			if a.Ability == CON {
				con = a.Base
			}
		}
		mod := (con - 10) / 2
		if con < 10 && (con-10)%2 != 0 {
			mod = (con - 11) / 2
		}
		got = n*(s+1)/2 + b
		if cr.HitPoints != got || b != n*mod {
			t.Errorf("%s: hit_points=%d roll=%q con=%d; want hit_points=%d (floor avg+bonus) and bonus %d (dice x con mod)", e.Key, cr.HitPoints, cr.HitPointsRoll, con, got, n*mod)
		}
	}
}
