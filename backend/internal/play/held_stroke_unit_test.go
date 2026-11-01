package play

import (
	"errors"
	"testing"

	"github.com/PuraFome/meuRPG/backend/internal/play/playdb"
)

// strokeRule.decide is the whole of "does the server ask, and may it say the roll failed".
func TestStrokeRuleDecide(t *testing.T) {
	t.Parallel()
	dc15 := checkStrokeRule(15, false)
	hidden := checkStrokeRule(15, true)
	cases := []struct {
		name              string
		rule              *strokeRule
		face, total       int
		wantAsk, wantTold bool
	}{
		{"a failed check asks and says so", dc15, 5, 8, true, true},
		{"a passed check is not asked", dc15, 10, 15, false, false},
		{"a hidden DC asks on a pass and says nothing", hidden, 10, 15, true, false},
		{"a hidden DC asks on a failure and says nothing", hidden, 5, 8, true, false},
		{"a natural 20 is never asked, hidden or not", hidden, 20, 20, false, false},
		{"a natural 20 on a check with the DC shown is not asked", dc15, 20, 22, false, false},
		{"an opposed check asks on every roll", opposedStrokeRule(), 7, 9, true, false},
	}
	for _, c := range cases {
		ask, told := c.rule.decide(c.face, c.total)
		if ask != c.wantAsk || told != c.wantTold {
			t.Errorf("%s: decide(%d, %d) = %v, %v; want %v, %v", c.name, c.face, c.total, ask, told, c.wantAsk, c.wantTold)
		}
	}
}

// A check with no DC has nothing to fail: no rule at all.
func TestAStrokeRuleNeedsADC(t *testing.T) {
	t.Parallel()
	if checkStrokeRule(0, false) != nil || checkStrokeRule(-1, true) != nil {
		t.Error("a check with no DC has a Stroke of Luck rule")
	}
	if groupStrokeRule(playdb.GroupCheck{}) != nil {
		t.Error("a group check with no DC has a Stroke of Luck rule")
	}
	dc := int32(12)
	if r := groupStrokeRule(playdb.GroupCheck{Dc: &dc, ShowDc: true}); r == nil || r.Blind {
		t.Errorf("a group check with a DC shown = %v, want a rule that tells", r)
	}
	if r := groupStrokeRule(playdb.GroupCheck{Dc: &dc}); r == nil || !r.Blind {
		t.Errorf("a group check with a DC hidden = %v, want a blind rule", r)
	}
}

// An attack misses against the armor class read lazily; a natural 1 misses whatever the bonus, and the attack flag marks
// the use as "the miss became a hit".
func TestAnAttackRuleMissesAgainstTheArmorClass(t *testing.T) {
	t.Parallel()
	r := attackStrokeRule(func() (int, error) { return 15, nil }, 20, false)
	if err := r.Prepare(); err != nil {
		t.Fatalf("Prepare() error = %v", err)
	}
	for _, c := range []struct {
		face, total int
		ask, told   bool
	}{
		{5, 12, true, true},   // 12 < 15
		{5, 15, false, false}, // reaches the armor class
		{1, 30, true, true},   // a natural 1 misses
		{20, 20, false, false},
	} {
		if ask, told := r.decide(c.face, c.total); ask != c.ask || told != c.told {
			t.Errorf("decide(%d, %d) = %v, %v; want %v, %v", c.face, c.total, ask, told, c.ask, c.told)
		}
	}
	failing := attackStrokeRule(func() (int, error) { return 0, errors.New("no armor") }, 20, false)
	if failing.Prepare() == nil {
		t.Error("Prepare() hid the error of the armor class")
	}
	if u := (&strokeUse{Index: 0, From: 6, To: 6}); !u.attack() || len(u.proto()) != 1 || u.proto()[0].GetSourceKey() != strokeSourceKey {
		t.Errorf("an attack's use = %v, want a miss turned into a hit marked as Stroke of Luck", u.proto())
	}
	if u := (&strokeUse{From: 6, To: 20}); u.attack() {
		t.Error("a check's use reads as an attack's")
	}
	var none *strokeUse
	if none.attack() || none.proto() != nil {
		t.Error("no use reads as one")
	}
}
