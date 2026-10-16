package encounter

import (
	"crypto/sha256"
	"encoding/hex"
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

var updateEncounterSweep = flag.Bool("update", false, "rewrite testdata/generate_sweep.sha256 from the current code")

// TestGenerateSweepCharacterization freezes Generate, the very encounter of
// each seed, over budgets from below the cheapest creature to far above the
// dearest, each type, each cap and the default and tight limits, on the test
// pool and on a pool of one kind (the group of the leader's own kind). The
// text is far too long to keep: its digest is in testdata.
func TestGenerateSweepCharacterization(t *testing.T) {
	t.Parallel()
	const seeds = 12
	pools := map[string][]Creature{
		"pool": pool,
		"orcs": {{"monster:orc", 4, 100, "humanoid"}},
	}
	limits := [][2]int{{0, 0}, {2, 3}, {1, 24}, {10, 2}}
	var b strings.Builder
	for _, name := range []string{"pool", "orcs"} {
		for _, budget := range []int{0, 10, 25, 60, 100, 230, 400, 1250, 1875, 2600, 6000, 20000} {
			for _, kind := range []string{"", "humanoid", "beast", "undead"} {
				for _, maxCR := range []int{0, 4, 16, 56, 240} {
					for _, l := range limits {
						for seed := range uint32(seeds) {
							o := Options{Budget: budget, MaxCR: maxCR, Type: kind, Seed: seed, MaxPerKind: l[0], MaxTotal: l[1]}
							r, err := Generate(pools[name], o)
							fmt.Fprintf(&b, "%s %+v %v %d %v\n", name, o, r.Entries, r.XP, err)
						}
					}
				}
			}
		}
	}
	sum := sha256.Sum256([]byte(b.String()))
	got := fmt.Sprintf("%s %d\n", hex.EncodeToString(sum[:]), strings.Count(b.String(), "\n"))
	path := filepath.Join("testdata", "generate_sweep.sha256")
	if *updateEncounterSweep {
		if err := os.MkdirAll("testdata", 0o750); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(path, []byte(got), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	want, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("%v (run with -update to create it)", err)
	}
	if got != string(want) {
		t.Errorf("the Generate sweep changed: digest %q, want %q", strings.TrimSpace(got), strings.TrimSpace(string(want)))
	}
}
