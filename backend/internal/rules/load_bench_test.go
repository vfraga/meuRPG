package rules

import "testing"

// BenchmarkLoadSRD measures loading the whole SRD content: the loaders and the
// corrections this package splits (effects, spell effects, traps, corrections)
// all run in it, so a refactor of one shows here.
func BenchmarkLoadSRD(b *testing.B) {
	b.ReportAllocs()
	for b.Loop() {
		if _, err := LoadSRD(); err != nil {
			b.Fatal(err)
		}
	}
}
