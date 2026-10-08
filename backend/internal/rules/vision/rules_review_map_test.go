package vision_test

import (
	"testing"

	"github.com/PuraFome/meuRPG/backend/internal/rules/grid"
	"github.com/PuraFome/meuRPG/backend/internal/rules/vision"
)

func TestRulesReviewMap_DimOnlyLightSourceSquare(t *testing.T) {
	t.Parallel()
	g := grid.Grid{Columns: 11, Rows: 11}
	at := grid.Square{Col: 5, Row: 5}
	sq := func(dc int) grid.Square { return grid.Square{Col: 5 + dc, Row: 5} }

	// SRD: Dancing Lights / Faerie Fire shed dim light only, in a 10-foot radius.
	dimOnly := compile(t, vision.Scene{Grid: g, Base: grid.Dark, Sources: []vision.Source{{At: at, BrightFt: 0, DimFt: 10}}})
	for _, c := range []struct {
		name string
		sq   grid.Square
		want grid.Light
	}{
		{"own square", sq(0), grid.Dim},
		{"5 ft", sq(1), grid.Dim},
		{"10 ft", sq(2), grid.Dim},
		{"15 ft", sq(3), grid.Dark},
	} {
		if got := dimOnly.LightAt(c.sq); got != c.want {
			t.Errorf("dim-only source, %s: LightAt = %v, want %v", c.name, got, c.want)
		}
	}
	viewer := vision.Viewer{At: sq(2)}
	if got := dimOnly.See(viewer).At(at); got != vision.SeenDim {
		t.Errorf("dim-only source: viewer 2 squares away sees the source square as %v, want SeenDim", got)
	}

	// Control: 5 ft bright + 5 ft dim.
	ctl := compile(t, vision.Scene{Grid: g, Base: grid.Dark, Sources: []vision.Source{{At: at, BrightFt: 5, DimFt: 5}}})
	for _, c := range []struct {
		name string
		sq   grid.Square
		want grid.Light
	}{
		{"own square", sq(0), grid.Bright},
		{"5 ft", sq(1), grid.Bright},
		{"-5 ft", sq(-1), grid.Bright},
		{"10 ft", sq(2), grid.Dim},
		{"15 ft", sq(3), grid.Dark},
	} {
		if got := ctl.LightAt(c.sq); got != c.want {
			t.Errorf("control source, %s: LightAt = %v, want %v", c.name, got, c.want)
		}
	}
}
