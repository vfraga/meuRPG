package maps

import (
	"bytes"
	"flag"
	"os"
	"path/filepath"
	"testing"
)

var updateGolden = flag.Bool("update-golden", false, "write the golden files of testdata again instead of comparing with them")

// checkGolden compares got with the golden file testdata/name. With -update-golden it
// writes the file instead: review the diff before committing it.
func checkGolden(t *testing.T, name string, got []byte) {
	t.Helper()
	path := filepath.Join("testdata", name)
	if *updateGolden {
		if err := os.MkdirAll("testdata", 0o750); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(path, got, 0o600); err != nil {
			t.Fatal(err)
		}
		return
	}
	want, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read the golden file: %v (write it with -update-golden)", err)
	}
	if !bytes.Equal(got, want) {
		t.Errorf("differs from %s:\n--- got\n%s\n--- want\n%s", path, got, want)
	}
}
