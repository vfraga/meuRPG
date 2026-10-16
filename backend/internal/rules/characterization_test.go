package rules

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
)

// The characterization tests of the loaders and the engine pieces split in E5
// record, for each corrupted input, the exact text the code answers, in
// testdata/golden/<name>.json. The loaders refuse in a fixed order and with
// fixed words; the files freeze both, so a split into smaller functions
// cannot change what a table master reads. Rewrite them with -update, only for
// a deliberate change, and review the diff.

// goldenText compares got with testdata/golden/<name>.json (a JSON object of
// case name to text), or rewrites the file under -update.
func goldenText(t *testing.T, name string, got map[string]string) {
	t.Helper()
	path := filepath.Join("testdata", "golden", name+".json")
	if *update {
		b, err := json.MarshalIndent(got, "", "  ")
		if err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(path, append(b, '\n'), 0o600); err != nil {
			t.Fatal(err)
		}
		return
	}
	raw, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("%v (run with -update to create it)", err)
	}
	var want map[string]string
	if err := json.Unmarshal(raw, &want); err != nil {
		t.Fatal(err)
	}
	for k, v := range got {
		if w, ok := want[k]; !ok || w != v {
			t.Errorf("%s: %s:\n got  %q\n want %q", name, k, v, w)
		}
	}
	for k := range want {
		if _, ok := got[k]; !ok {
			t.Errorf("%s: the golden has %q and the test does not", name, k)
		}
	}
}

// errText is the text of an error, or the word accepted.
func errText(err error) string {
	if err == nil {
		return "accepted"
	}
	return err.Error()
}
