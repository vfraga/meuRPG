// Finding U8-2 (see review/unit-08-rules-content.md).
package characters

import (
	"testing"

	"github.com/PuraFome/meuRPG/backend/internal/rules"
)

// TestReview08_DerivedSheetCarriesNegativeMaxHP: derivedToProto copies the raw
// maximum hit points to the client, with no floor.
func TestReview08_DerivedSheetCarriesNegativeMaxHP(t *testing.T) {
	t.Parallel()
	for _, hp := range []int{-990, 0} {
		got := derivedToProto(rules.Derived{HitPointsMax: hp}).GetHitPointsMax()
		if got < 1 {
			t.Errorf("DerivedSheet.HitPointsMax = %d for Derived.HitPointsMax = %d, want >= 1", got, hp)
		}
	}
}
