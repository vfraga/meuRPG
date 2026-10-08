package characters

import (
	"testing"

	"github.com/PuraFome/meuRPG/backend/internal/rules"
)

// The sheet the client reads never carries a maximum below 1 hit point, so a
// reader that treats 0 as "no hit points" cannot be handed one.
func TestDerivedSheetMaxHitPointsIsAtLeastOne(t *testing.T) {
	t.Parallel()
	for _, hp := range []int{-990, 0} {
		if got := derivedToProto(rules.Derived{HitPointsMax: hp}).GetHitPointsMax(); got != 1 {
			t.Errorf("DerivedSheet.HitPointsMax = %d for Derived.HitPointsMax = %d, want 1", got, hp)
		}
	}
	if got := derivedToProto(rules.Derived{HitPointsMax: 24}).GetHitPointsMax(); got != 24 {
		t.Errorf("DerivedSheet.HitPointsMax = %d for 24, want 24", got)
	}
}
