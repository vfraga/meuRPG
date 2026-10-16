package grid_test

import (
	"crypto/sha256"
	"encoding/hex"
	"flag"
	"fmt"
	"hash"
	"io"
	"math"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/PuraFome/meuRPG/backend/internal/rules/grid"
)

var updateSweep = flag.Bool("update", false, "rewrite testdata/*.sha256 from the current code")

// sweepMap is a 10 x 8 map with one of everything a move, a sight line or an
// area reads: # wall, ~ difficult, h half cover, H three-quarters cover, and the
// doors o open, c closed, l locked, b barred, s secret. The walls at (3, 3) and
// (2, 4) touch at a corner, so a line can squeeze between them.
var sweepMap = []string{
	"..........",
	".#..~~..H.",
	".#.c..l...",
	"...#b.~...",
	"..#.s..o..",
	".~~..h..#.",
	"....#.....",
	"......c...",
}

func sweepTerrain() grid.Terrain {
	g := grid.Grid{Columns: len(sweepMap[0]), Rows: len(sweepMap)}
	t := grid.Terrain{Grid: g, Walls: grid.NewLayer(g), Difficult: grid.NewLayer(g), Cover: grid.NewCoverLayer(g), Doors: grid.NewDoorLayer(g)}
	doors := map[rune]grid.Door{'o': grid.DoorOpen, 'c': grid.DoorClosed, 'l': grid.DoorLocked, 'b': grid.DoorBarred, 's': grid.DoorSecret}
	for row, line := range sweepMap {
		for col, ch := range line {
			switch ch {
			case '#':
				t.Walls.Set(col, row, true)
			case '~':
				t.Difficult.Set(col, row, true)
			case 'h':
				t.Cover.Set(col, row, grid.CoverHalf)
			case 'H':
				t.Cover.Set(col, row, grid.CoverThreeQuarters)
			default:
				if d, ok := doors[ch]; ok {
					t.Doors.Set(col, row, d)
				}
			}
		}
	}
	return t
}

// sweepOccupants stand on the map: a friend, hostile creatures of four sizes and
// a Tiny friend, so every pass and every end rule comes up.
var sweepOccupants = grid.OccupantMap{
	{Col: 5, Row: 0}: {Size: grid.SizeMedium},
	{Col: 7, Row: 6}: {Size: grid.SizeMedium, Hostile: true},
	{Col: 2, Row: 7}: {Size: grid.SizeTiny, Hostile: true},
	{Col: 8, Row: 3}: {Size: grid.SizeTiny},
	{Col: 0, Row: 4}: {Size: grid.SizeHuge, Hostile: true},
	{Col: 6, Row: 5}: {Size: grid.SizeSmall, Hostile: true},
}

// sweepTrap is a trap's area: one square held by a friend, one a destination
// can be, one behind a barred door.
var sweepTrap = grid.Squares{{Col: 4, Row: 1}, {Col: 5, Row: 3}, {Col: 9, Row: 7}, {Col: 5, Row: 0}}

var sweepMovers = []grid.Mover{
	{Size: grid.SizeMedium},
	{Flier: true, Size: grid.SizeMedium},
	{Size: grid.SizeTiny},
	{Size: grid.SizeGargantuan},
}

// sweepBudgets are the movement left for MoveUntil: none, a little, some, and
// no limit (the master).
var sweepBudgets = []int{0, 40, 150, math.MaxInt}

// checkSweep compares the digest of what write prints with the one in
// testdata/<name>.sha256 (the full text is far too long to keep).
func checkSweep(t *testing.T, name string, write func(w io.Writer)) {
	t.Helper()
	h := sha256.New()
	lines := &countingWriter{h: h}
	write(lines)
	got := fmt.Sprintf("%s %d\n", hex.EncodeToString(h.Sum(nil)), lines.lines)
	path := filepath.Join("testdata", name+".sha256")
	if *updateSweep {
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
		t.Errorf("the %s sweep changed: digest %q, want %q", name, strings.TrimSpace(got), strings.TrimSpace(string(want)))
	}
}

type countingWriter struct {
	h     hash.Hash
	lines int
}

func (c *countingWriter) Write(p []byte) (int, error) {
	c.lines += strings.Count(string(p), "\n")
	return c.h.Write(p)
}

func allSquares(g grid.Grid) []grid.Square {
	var out []grid.Square
	for row := range g.Rows {
		for col := range g.Columns {
			out = append(out, grid.Square{Col: col, Row: row})
		}
	}
	return out
}

// TestMoveSweepCharacterization freezes Move, Jump and MoveUntil for every
// pair of squares of sweepMap, every mover, with and without the creatures,
// and MoveUntil for every budget, with and without the trap.
func TestMoveSweepCharacterization(t *testing.T) {
	t.Parallel()
	tr := sweepTerrain()
	squares := allSquares(tr.Grid)
	checkSweep(t, "move_sweep", func(w io.Writer) {
		for _, from := range squares {
			for _, to := range squares {
				for mi, m := range sweepMovers {
					for _, occ := range []grid.Occupants{nil, sweepOccupants} {
						fmt.Fprintf(w, "%v>%v m%d occ=%v move=%+v jump=%+v\n", from, to, mi, occ != nil, tr.Move(from, to, occ, m), tr.Jump(from, to, occ, m))
						for _, left := range sweepBudgets {
							for _, trap := range []grid.Set{nil, sweepTrap} {
								fmt.Fprintf(w, " left=%d trap=%v until=%+v\n", left, trap != nil, tr.MoveUntil(from, to, occ, m, trap, left))
							}
						}
					}
				}
			}
		}
	})
}

// TestMoveOffTheGridCharacterization: a start or a destination off the grid,
// or a grid that is not valid, never leaves the start, and says a wall stopped it.
func TestMoveOffTheGridCharacterization(t *testing.T) {
	t.Parallel()
	tr := sweepTerrain()
	in, out := grid.Square{Col: 1, Row: 0}, grid.Square{Col: -1, Row: 0}
	for _, c := range []struct {
		name     string
		tr       grid.Terrain
		from, to grid.Square
	}{
		{"from off the grid", tr, out, in},
		{"to off the grid", tr, in, out},
		{"invalid grid", grid.Terrain{}, in, in},
	} {
		got := c.tr.MoveUntil(c.from, c.to, nil, grid.Mover{}, nil, math.MaxInt)
		want := grid.Stopped{Reached: c.from, Reason: grid.StopWall}
		if fmt.Sprintf("%+v", got) != fmt.Sprintf("%+v", want) {
			t.Errorf("%s: MoveUntil = %+v, want %+v", c.name, got, want)
		}
		if mv := c.tr.Move(c.from, c.to, nil, grid.Mover{}); !mv.Blocked || mv.By != grid.StopWall || mv.Entered != nil {
			t.Errorf("%s: Move = %+v, want blocked by a wall", c.name, mv)
		}
		if mv := c.tr.Jump(c.from, c.to, nil, grid.Mover{}); !mv.Blocked || mv.By != grid.StopWall {
			t.Errorf("%s: Jump = %+v, want blocked by a wall", c.name, mv)
		}
	}
}

// layerPicture draws a layer on its own grid, one | after each row.
func layerPicture(l *grid.Layer) string {
	if l == nil {
		return "<nil>"
	}
	g := l.Grid()
	var b strings.Builder
	for row := range g.Rows {
		for col := range g.Columns {
			if l.Get(col, row) {
				b.WriteByte('#')
			} else {
				b.WriteByte('.')
			}
		}
		b.WriteByte('|')
	}
	return b.String()
}

// TestSightWallsCharacterization freezes SightWalls (the function and the
// method) for no doors, doors of another grid, doors that block nothing, and
// doors that do, with and without walls; and whether it hands back the walls
// themselves or a new layer.
func TestSightWallsCharacterization(t *testing.T) {
	t.Parallel()
	tr := sweepTerrain()
	other := grid.NewDoorLayer(grid.Grid{Columns: 3, Rows: 3})
	other.Set(1, 1, grid.DoorClosed)
	openOnly := grid.NewDoorLayer(tr.Grid)
	openOnly.Set(2, 2, grid.DoorOpen)
	openOnly.Set(3, 2, grid.DoorBarred)
	for _, c := range []struct {
		name     string
		walls    *grid.Layer
		doors    *grid.DoorLayer
		want     string
		sameAsIn bool
	}{
		{"no doors", tr.Walls, nil, "..........|.#........|.#........|...#......|..#.......|........#.|....#.....|..........|", true},
		{"no doors, no walls", nil, nil, "<nil>", true},
		{"doors of another grid", tr.Walls, other, "..........|.#........|.#........|...#......|..#.......|........#.|....#.....|..........|", true},
		// With no walls to compare with, doors of another grid are read on their
		// own grid: the result is sized for the doors, not for the map.
		{"doors of another grid, no walls", nil, other, "...|.#.|...|", false},
		{"doors that block nothing", tr.Walls, openOnly, "..........|.#........|.#........|...#......|..#.......|........#.|....#.....|..........|", true},
		{"doors that block nothing, no walls", nil, openOnly, "<nil>", true},
		{"blocking doors", tr.Walls, tr.Doors, "..........|.#........|.#.#..#...|...#......|..#.#.....|........#.|....#.....|......#...|", false},
		{"blocking doors, no walls", nil, tr.Doors, "..........|..........|...#..#...|..........|....#.....|..........|..........|......#...|", false},
	} {
		got := grid.SightWalls(c.walls, c.doors)
		if p := layerPicture(got); p != c.want {
			t.Errorf("%s: SightWalls = %s, want %s", c.name, p, c.want)
		}
		if (got == c.walls) != c.sameAsIn {
			t.Errorf("%s: returns the walls themselves = %v, want %v", c.name, got == c.walls, c.sameAsIn)
		}
	}
	if p := layerPicture(tr.SightWalls()); p != "..........|.#........|.#.#..#...|...#......|..#.#.....|........#.|....#.....|......#...|" {
		t.Errorf("Terrain.SightWalls = %s", p)
	}
	// The arguments are not changed.
	if p := layerPicture(tr.Walls); p != "..........|.#........|.#........|...#......|..#.......|........#.|....#.....|..........|" {
		t.Errorf("the walls changed: %s", p)
	}
}

// TestOpenAreaSweepCharacterization freezes OpenArea (and so spread) from
// every square of sweepMap, for a disc of radius 3 with and without the
// origin, spreading around corners or not.
func TestOpenAreaSweepCharacterization(t *testing.T) {
	t.Parallel()
	tr := sweepTerrain()
	squares := allSquares(tr.Grid)
	checkSweep(t, "openarea_sweep", func(w io.Writer) {
		for _, origin := range squares {
			var disc, ring []grid.Square
			for dr := -3; dr <= 3; dr++ {
				for dc := -3; dc <= 3; dc++ {
					if dc*dc+dr*dr > 9 {
						continue
					}
					s := grid.Square{Col: origin.Col + dc, Row: origin.Row + dr}
					disc = append(disc, s)
					if s != origin {
						ring = append(ring, s)
					}
				}
			}
			for _, corners := range []bool{false, true} {
				fmt.Fprintf(w, "%v corners=%v disc=%v ring=%v\n", origin, corners, tr.OpenArea(origin, disc, corners), tr.OpenArea(origin, ring, corners))
			}
		}
	})
}

// TestSpreadWithoutTheOriginCharacterization: spreading around corners from an
// origin that is not in the shape gives nothing, though the shape has open
// squares next to it.
func TestSpreadWithoutTheOriginCharacterization(t *testing.T) {
	t.Parallel()
	tr := sweepTerrain()
	origin := grid.Square{Col: 6, Row: 1}
	near := []grid.Square{{Col: 5, Row: 1}, {Col: 7, Row: 1}, {Col: 6, Row: 0}}
	if got := tr.OpenArea(origin, near, true); got != nil {
		t.Errorf("OpenArea around corners without the origin = %v, want nil", got)
	}
	if got := tr.OpenArea(origin, near, false); len(got) != len(near) {
		t.Errorf("OpenArea in straight lines without the origin = %v, want the three squares", got)
	}
}
