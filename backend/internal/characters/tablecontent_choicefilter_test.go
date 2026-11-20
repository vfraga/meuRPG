package characters

import (
	"slices"
	"testing"

	rulesv1 "github.com/PuraFome/meuRPG/backend/gen/meurpg/rules/v1"
)

// A choice's filter and the feature it offers the options of reach the engine's effect.
func TestTableEffectsOfReadsTheFilterAndTheFeatureOfAChoice(t *testing.T) {
	t.Parallel()
	got := tableEffectsOf([]*rulesv1.TableEffect{
		{Type: "choice", Choice: "tool", Count: 1, Filter: &rulesv1.TableChoiceFilter{ProficiencyKinds: []string{"weapon", "other"}}},
		{Type: "choice", Choice: "feature", Count: 2, FromFeature: "feature:posturas@mesa"},
		{Type: "choice", Choice: "tool", Count: 1},
	})
	if f := got[0].Filter; f == nil || !slices.Equal(f.ProficiencyKinds, []string{"weapon", "other"}) {
		t.Errorf("filter = %+v, want the weapon and other kinds", f)
	}
	if got[1].FromFeature != "feature:posturas@mesa" || got[1].Filter != nil {
		t.Errorf("effect = %+v, want the feature and no filter", got[1])
	}
	if got[2].Filter != nil || got[2].FromFeature != "" {
		t.Errorf("effect = %+v, want neither", got[2])
	}
}
