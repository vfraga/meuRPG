package characters

import (
	"testing"

	"github.com/PuraFome/meuRPG/backend/internal/rules"
)

// The feature the sheet is told with names the key of the feature that offers it, not only its name.
func TestAFeatureIsSentWithTheKeyOfTheFeatureThatOffersIt(t *testing.T) {
	t.Parallel()
	got := featureToProto(rules.Feature{Key: "feature:x", OptionOfPT: "Estilo de Luta", OptionOfKey: "feature:paladin-fighting-style"})
	if got.GetOptionOfKey() != "feature:paladin-fighting-style" || got.GetOptionOfPt() != "Estilo de Luta" {
		t.Errorf("option of = %q / %q", got.GetOptionOfKey(), got.GetOptionOfPt())
	}
}
