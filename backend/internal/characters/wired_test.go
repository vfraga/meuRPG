package characters

import (
	"strings"
	"testing"
)

// A service whose Set... calls were never made says which ones: nil collaborators
// quietly turn features off (a nil fog source is "no fog of war"), so cmd/api
// refuses to start with them (platform/wiring).
func TestCheckWiredNamesWhatIsMissing(t *testing.T) {
	t.Parallel()
	err := (&Service{}).CheckWired()
	if err == nil {
		t.Fatal("CheckWired() on an unwired service = nil, want an error")
	}
	for _, setter := range []string{"SetGallery", "SetLive", "SetCreatureHost", "SetReviewHost", "SetLevelUps", "SetSessionHost"} {
		if !strings.Contains(err.Error(), setter) {
			t.Errorf("CheckWired() = %q, want it to name %s", err, setter)
		}
	}
}
