package rules

import (
	"errors"
	"os"
	"runtime"
	"strconv"
	"strings"
	"testing"
	"time"
)

// budgetOverlay is the worst an overlay may be and still pass every limit: 300
// entries, 6,000 effects (20 in each of 300 features) and 500 distinct formulas
// (the other effects repeat them).
func budgetOverlay() Overlay {
	_, _, bg := tableMisc()
	o := Overlay{Revision: 3}
	n := 0
	for i := range MaxOverlayEffects / MaxFeatureEffects {
		b := bg
		b.Key = "background:antecedente-" + strconv.Itoa(i) + tableSuffix
		fx := make([]Effect, 0, MaxFeatureEffects)
		for range MaxFeatureEffects {
			// 500 distinct texts, each repeated twelve times over the overlay; the
			// formulas use every part of the grammar so each compile has work to do.
			fx = append(fx, Effect{Type: "modifier", Target: "ac", Mode: "add", Value: `max(1, mod("str") + ` + strconv.Itoa(n%MaxOverlayFormulas+1) + ")"})
			n++
		}
		b.Feature = TableFeature{Key: "background-feature:antecedente-" + strconv.Itoa(i) + tableSuffix, NamePT: "F", Effects: fx}
		o.Backgrounds = append(o.Backgrounds, b)
	}
	return o
}

// TestWithAtTheBudgets: an overlay at every budget (6,000 effects, 500 distinct
// formulas) is accepted and stays cheap: under 100 ms and under 8 MB kept. Without
// the budgets, 300 classes of 60 features of 20 effects took 24 s and kept 673 MB.
func TestWithAtTheBudgets(t *testing.T) {
	srd := loadForTest(t)
	o := budgetOverlay()
	best := time.Hour
	var kept *Content
	heap := func() uint64 {
		runtime.GC()
		runtime.GC()
		var m runtime.MemStats
		runtime.ReadMemStats(&m)
		return m.HeapAlloc
	}
	before := heap()
	for range 5 {
		start := time.Now()
		c, err := srd.With(o)
		if err != nil {
			t.Fatal(err)
		}
		best = min(best, time.Since(start))
		kept = c
	}
	after := heap()
	retained := float64(after-before) / 1e6
	t.Logf("at the budgets: %v best of 5, %.2f MB kept (one content)", best, retained)
	// The time is held only when asked (MEURPG_MEASURE=1, on a quiet machine,
	// without -race): on a busy one, or in CI, a clock check fails for nothing.
	if os.Getenv("MEURPG_MEASURE") == "1" && !raceEnabled && best > 100*time.Millisecond {
		t.Errorf("With at the budgets took %v, want under 100 ms", best)
	}
	if retained > 8 {
		t.Errorf("a content at the budgets keeps %.2f MB, want under 8", retained)
	}
	runtime.KeepAlive(kept)
}

// breakFormula replaces every effect whose formula is text with one that does
// not compile.
func breakFormula(o *Overlay, text string) {
	for i := range o.Backgrounds {
		fx := o.Backgrounds[i].Feature.Effects
		for j := range fx {
			if fx[j].Value == text {
				fx[j].Value = "max(1,"
			}
		}
	}
}

// TestBudgetRefusalsHappenBeforeAnyCompile: the 6,001st effect and the 501st
// distinct formula are refused with the limit named, before anything is
// compiled. Every copy of one formula is broken: had With compiled anything
// first, it would have answered with that formula's error, not the limit.
// (No clock: a timing check fails on a busy machine.)
func TestBudgetRefusalsHappenBeforeAnyCompile(t *testing.T) {
	t.Parallel()
	srd := loadForTest(t)
	const first = `max(1, mod("str") + 1)` // the first effect's formula in budgetOverlay

	moreEffects := budgetOverlay()
	breakFormula(&moreEffects, first)
	bg := moreEffects.Backgrounds[0]
	bg.Key = "background:um-a-mais" + tableSuffix
	bg.Feature = TableFeature{Key: "background-feature:um-a-mais" + tableSuffix, NamePT: "F", Effects: []Effect{{Type: "note"}}}
	moreEffects.Backgrounds = append(moreEffects.Backgrounds, bg)

	moreFormulas := budgetOverlay()
	breakFormula(&moreFormulas, first)
	moreFormulas.Backgrounds[0].Feature.Effects[1].Value = "max(2, 3)"

	for name, tc := range map[string]struct {
		o    Overlay
		want string
	}{
		"effects":  {moreEffects, "more than 6000 effects"},
		"formulas": {moreFormulas, "more than 500 distinct formulas"},
	} {
		_, err := srd.With(tc.o)
		var oe *OverlayError
		if !errors.As(err, &oe) || oe.Reason != ReasonLimit || !strings.Contains(oe.Message, tc.want) {
			t.Errorf("%s: err = %v, want the limit %q before any compile", name, err, tc.want)
		}
	}
}
