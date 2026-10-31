package characters

import (
	"testing"

	"github.com/PuraFome/meuRPG/backend/internal/rules"
)

// The attack of the sheet promises a range_ft of at least its melee reach, so the screens and the list of
// targets that read range_ft as the reach keep working when a weapon rule lengthens the reach and the line
// keeps the weapon's own range. No database is needed.
func TestAttackToProtoRangeIsAtLeastTheMeleeReach(t *testing.T) {
	t.Parallel()
	for _, tc := range []struct {
		name        string
		in          rules.Attack
		want, reach int32
	}{
		{"a longsword with a 10 ft rule", rules.Attack{Melee: true, RangeFt: 0, ReachFt: 10}, 10, 10},
		{"a longsword with no rule", rules.Attack{Melee: true}, 0, 0},
		{"a dagger thrown 20/60 with a 30 ft rule", rules.Attack{Melee: true, RangeFt: 20, LongRangeFt: 60, ReachFt: 30}, 30, 30},
		{"a dagger thrown 20/60 with a 10 ft rule", rules.Attack{Melee: true, RangeFt: 20, LongRangeFt: 60, ReachFt: 10}, 20, 10},
	} {
		p := attackToProto(tc.in)
		if p.GetRangeFt() != tc.want || p.GetReachFt() != tc.reach {
			t.Errorf("%s: range_ft %d, reach_ft %d, want %d and %d", tc.name, p.GetRangeFt(), p.GetReachFt(), tc.want, tc.reach)
		}
	}
}
