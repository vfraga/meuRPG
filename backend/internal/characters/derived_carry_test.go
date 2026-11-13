package characters

import (
	"testing"

	"github.com/PuraFome/meuRPG/backend/internal/rules"
)

// The carrying capacity of the derived sheet reaches the proto the screens read. No database is needed.
func TestDerivedToProtoCarriesTheCarryCapacity(t *testing.T) {
	t.Parallel()
	if got := derivedToProto(rules.Derived{CarryCapacityLb: 225}).GetCarryCapacityLb(); got != 225 {
		t.Errorf("carry_capacity_lb = %d, want 225", got)
	}
}
