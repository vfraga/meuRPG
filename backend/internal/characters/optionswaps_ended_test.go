package characters

import (
	"slices"
	"testing"

	"github.com/PuraFome/meuRPG/backend/internal/rules"
)

// What a given-up option defined is ended with it, unless another source of the new sheet still defines the same key.
func TestAnOptionGivenUpEndsOnlyTheStatesAndResourcesNothingElseDefines(t *testing.T) {
	t.Parallel()
	before := rules.Derived{
		Resources: []rules.Resource{{Key: "garca", Source: "feature:heron"}, {Key: "tiro", Source: "feature:heron"}},
		States:    []rules.State{{Key: "feature:heron#garca", Source: "feature:heron"}, {Key: "feature:shared#guarda", Source: "feature:heron"}},
	}
	for name, tc := range map[string]struct {
		after rules.Derived
		want  []string
	}{
		"no other source": {after: rules.Derived{}, want: []string{"feature:heron#garca", "feature:shared#guarda"}},
		"a kept feature defines the state key": {
			after: rules.Derived{States: []rules.State{{Key: "feature:shared#guarda", Source: "feature:kept"}}},
			want:  []string{"feature:heron#garca"},
		},
	} {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			_, states := endedByGivingUp(before, tc.after, []string{"feature:heron"})
			if !slices.Equal(states, tc.want) {
				t.Errorf("ended states = %v, want %v", states, tc.want)
			}
		})
	}
}
