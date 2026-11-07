package characters

import (
	"reflect"
	"testing"

	"github.com/PuraFome/meuRPG/backend/internal/rules"
)

// The sheet copies the roll minimums with the key and the Portuguese name of their feature, in the order of the effects.
func TestDerivedToProtoCopiesRollMinimums(t *testing.T) {
	t.Parallel()
	d := rules.Derived{
		Features: []rules.Feature{{Key: "feature:a@mesa", NamePT: "Piso A"}, {Key: "feature:b@mesa", NamePT: "Piso B"}},
		RollMinimums: []rules.RollMinimum{
			{Applies: []string{"damage"}, Value: 2, Source: "feature:b@mesa"},
			{Applies: []string{"attack", "save"}, Value: 10, Source: "feature:a@mesa"},
		},
	}
	got := derivedToProto(d).GetRollMinimums()
	if len(got) != 2 {
		t.Fatalf("roll_minimums = %v; want 2", got)
	}
	if got[0].GetFeatureKey() != "feature:b@mesa" || got[0].GetFeatureName() != "Piso B" || got[0].GetValue() != 2 || !reflect.DeepEqual(got[0].GetApplies(), []string{"damage"}) {
		t.Errorf("first = %v; want the damage 2 of Piso B", got[0])
	}
	if got[1].GetFeatureKey() != "feature:a@mesa" || got[1].GetFeatureName() != "Piso A" || got[1].GetValue() != 10 || !reflect.DeepEqual(got[1].GetApplies(), []string{"attack", "save"}) {
		t.Errorf("second = %v; want the attack and save 10 of Piso A", got[1])
	}
}

// A minimum whose source is not in the feature list is not sent: no key, no name.
func TestDerivedToProtoDropsRollMinimumOfUnlistedSource(t *testing.T) {
	t.Parallel()
	d := rules.Derived{
		Features: []rules.Feature{{Key: "feature:a@mesa", NamePT: "Piso A"}},
		RollMinimums: []rules.RollMinimum{
			{Applies: []string{"check"}, Value: 9, Source: "feature:oculto@mesa"},
			{Applies: []string{"check"}, Value: 8, Source: "feature:a@mesa"},
		},
	}
	got := derivedToProto(d).GetRollMinimums()
	if len(got) != 1 || got[0].GetFeatureKey() != "feature:a@mesa" {
		t.Fatalf("roll_minimums = %v; want only the one of the listed feature", got)
	}
	if got := derivedToProto(rules.Derived{RollMinimums: d.RollMinimums}).GetRollMinimums(); len(got) != 0 {
		t.Errorf("roll_minimums without features = %v; want none", got)
	}
}
