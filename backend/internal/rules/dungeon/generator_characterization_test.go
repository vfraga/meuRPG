package dungeon

import (
	"crypto/sha256"
	"encoding/binary"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"hash"
	"math"
	"os"
	"slices"
	"strings"
	"testing"
)

// The characterization of the generator: every phase's working state, every
// test probe and every rendering, over many seeds and options, recorded in
// testdata/generator_golden.json. It freezes the generator byte for byte (the
// state after each stage, not only the result), so the phases can be split
// into small functions without moving a single draw. The generation cases are
// hashes (a level is too big to read); the white-box cases, for branches no
// generated level reaches, are readable.
//
// One branch has no case: in extraLoops, a corridor-pair candidate whose
// square is no longer rock when its turn comes. Each such square is listed
// once and only its own candidate opens it (a threshold candidate opens a
// door square, which has a room on one side and so is never a corridor-pair
// square), so the skip cannot happen.
const generatorGoldenPath = "testdata/generator_golden.json"

// genCharCombos is how many option combinations the generation cases cover
// (comboFor, the same varied sets as the property tests, from another start),
// in buckets of genCharBucket.
const (
	genCharCombos  = 2000
	genCharBucket  = 100
	genCharStartAt = 300000

	genCharDenseSeeds  = 200
	genCharDenseBucket = 50
)

// genHashState writes the working state of a generation into h.
func genHashState(h hash.Hash, stage string, g *gen) {
	fmt.Fprintf(h, "stage %s\n", stage)
	genWriteCells(h, g.kind)
	genWriteCells(h, g.rid)
	genWriteFlags(h, g.blocked)
	genWriteFlags(h, g.lane)
	genWriteFlags(h, g.protected)
	genWriteFlags(h, g.joined)
	genWriteCells(h, g.doorKind)
	genWriteFlags(h, g.doorTrap)
	fmt.Fprintf(h, "any %v epoch %d passes %d kindRNG %v\n", g.anyBlocked, g.epoch, g.passes, g.kindRNG.s)
	for _, r := range g.rooms {
		fmt.Fprintf(h, "room %d %d %d %d %d %v %d %d\n", r.id, r.x, r.y, r.w, r.h, r.discarded, r.centerX, r.centerY)
	}
	for id, r := range g.roomByID {
		if r == nil {
			fmt.Fprintf(h, "nil %d\n", id)
		}
	}
	pairs := make([]uint32, 0, len(g.directPairs))
	for k := range g.directPairs {
		pairs = append(pairs, k)
	}
	slices.Sort(pairs)
	fmt.Fprintf(h, "pairs %v lanes %v discarded %v stairs %+v\n", pairs, g.lanes, g.discarded, g.stairs)
}

// genWriteCells writes a grid of small numbers into h, two bytes a square.
func genWriteCells[T ~uint8 | ~uint16](h hash.Hash, cells []T) {
	b := make([]byte, 0, 2*len(cells))
	for _, c := range cells {
		b = binary.LittleEndian.AppendUint16(b, uint16(c))
	}
	h.Write(b)
}

// genWriteFlags writes a grid of flags into h, one byte a square.
func genWriteFlags(h hash.Hash, flags []bool) {
	b := make([]byte, len(flags))
	for i, f := range flags {
		if f {
			b[i] = 1
		}
	}
	h.Write(b)
}

// genHashResult writes a result and every rendering of it into h.
func genHashResult(h hash.Hash, d *Dungeon, err error) {
	if err != nil {
		fmt.Fprintf(h, "error %s\n", err)
		return
	}
	// The grids by hand (JSON through reflection is slow under -race), the
	// rest as JSON.
	genWriteCells(h, d.Kinds)
	genWriteCells(h, d.RoomIDs)
	genWriteCells(h, d.CorridorIDs)
	genWriteFlags(h, d.Mask)
	rest := *d
	rest.Kinds, rest.RoomIDs, rest.CorridorIDs, rest.Mask = nil, nil, nil, nil
	b, jerr := json.Marshal(rest)
	if jerr != nil {
		panic(jerr)
	}
	h.Write(b)
	h.Write([]byte(ASCII(d)))
	open, wall := WallsMask(d)
	genWriteFlags(h, open)
	genWriteFlags(h, wall)
	p, perr := json.Marshal(PromptOf(d))
	if perr != nil {
		panic(perr)
	}
	h.Write(p)
	fmt.Fprintf(h, "%+v\n", Markers(d))
}

// genTrace generates with every stage and probe recorded into h. tamper, when
// not nil, runs after the state of each stage is recorded.
func genTrace(h hash.Hash, o Options, tamper func(stage string, g *gen)) (*Dungeon, error) {
	d, err := generate(o, func(stage string, g *gen) {
		if stage == "grid" {
			g.onLater = func(n int) { fmt.Fprintf(h, "later %d\n", n) }
			g.onDeadEndDraw = func(node int, ok bool) { fmt.Fprintf(h, "draw %d %v\n", node, ok) }
		}
		genHashState(h, stage, g)
		if tamper != nil {
			tamper(stage, g)
		}
	})
	genHashResult(h, d, err)
	return d, err
}

func genHex(h hash.Hash) string { return hex.EncodeToString(h.Sum(nil)) }

// genCharGenerationCases hashes the generation cases: the combinations in
// buckets, the golden options one by one and the worst cases on the biggest
// grid.
func genCharGenerationCases(t *testing.T, got map[string]string) {
	t.Helper()
	for start := 0; start < genCharCombos; start += genCharBucket {
		h := sha256.New()
		for k := start; k < start+genCharBucket; k++ {
			o := comboFor(uint64(genCharStartAt + k))
			o.ExtraLoops = []int{0, 30, 100}[k%3]
			o.SelfCheck = k%2 == 0
			fmt.Fprintf(h, "combo %d\n", k)
			_, _ = genTrace(h, o, nil)
		}
		got[fmt.Sprintf("combos-%04d", start)] = genHex(h)
	}
	// Dense levels of small rooms with few doors: the groups that link through
	// a carved path, on a later pass, or not at all.
	for start := uint64(1); start <= genCharDenseSeeds; start += genCharDenseBucket {
		h := sha256.New()
		for seed := start; seed < start+genCharDenseBucket; seed++ {
			o := opts(seed, 41, 41)
			o.RoomDensity, o.RoomSideMin, o.RoomSideMax, o.DoorDensity = 200, 3, 5, 25
			o.ExtraLoops = int(seed % 2 * 50)
			_, _ = genTrace(h, o, nil)
		}
		got[fmt.Sprintf("dense-%03d", start)] = genHex(h)
	}
	for _, c := range goldenCases() {
		h := sha256.New()
		_, _ = genTrace(h, c.Opts(), nil)
		got["golden-"+c.Name] = genHex(h)
	}
	for name, f := range worstCases() {
		h := sha256.New()
		for seed := uint64(1); seed <= 2; seed++ {
			o := opts(seed, 199, 399)
			f(&o)
			_, _ = genTrace(h, o, nil)
		}
		got["worst-"+name] = genHex(h)
	}
}

// genDoorlessRoom finds the options of a level with at least three stairs,
// one of them in a room corner, and that room's ID: the
// fixture that takes every door off one room before the final phase.
func genDoorlessRoom(t *testing.T) (Options, int) {
	t.Helper()
	for seed := uint64(1); seed <= 500; seed++ {
		o := opts(seed, 31, 31)
		o.DeadendRemoval, o.Stairs = 100, 8
		o.Placement, o.RoomDensity = PlacementTiled, 200
		d, err := Generate(o)
		if err != nil || len(d.Rooms) < 3 || len(d.Stairs) < 3 {
			continue
		}
		for _, s := range d.Stairs {
			if s.InRoom != 0 {
				return o, s.InRoom
			}
		}
	}
	t.Fatal("no level with a room stair")
	return Options{}, 0
}

// genSealRoom turns every door on a room's ring into rock, as if dead-end
// removal had cut them all: the case finish's step 2 exists for.
func genSealRoom(g *gen, id int) {
	rm := g.roomByID[id]
	for side := North; side <= West; side++ {
		for p := range rm.sideLen(side) {
			t := rm.thresholdAt(side, p)
			if di := g.idx(t.doorX, t.doorY); g.kind[di] == KindDoor {
				g.kind[di] = KindRock
			}
		}
	}
}

// genCharFinishCases records what finish does with a room that lost every
// door: an error under SelfCheck, otherwise the room is erased, its stairs go
// and the first two stairs are up and down again.
func genCharFinishCases(t *testing.T, got map[string]string) {
	t.Helper()
	o, id := genDoorlessRoom(t)
	seal := func(stage string, g *gen) {
		if stage == "deadends" {
			genSealRoom(g, id)
		}
	}
	for _, self := range []bool{true, false} {
		o.SelfCheck = self
		h := sha256.New()
		d, err := genTrace(h, o, seal)
		name := fmt.Sprintf("finish-sealed-room-selfcheck-%v", self)
		if err != nil {
			got[name] = err.Error()
			continue
		}
		got[name] = fmt.Sprintf("seed %d room %d: discarded %v, stairs %+v, entrance %+v, rooms %d, %s",
			o.Seed, id, d.DiscardedRooms, d.Stairs, d.Entrance, len(d.Rooms), genHex(h))
	}
	// Sealing the first room moves the entrance when there are no stairs.
	o.Stairs, o.SelfCheck = 0, false
	h := sha256.New()
	d, err := genTrace(h, o, func(stage string, g *gen) {
		if stage == "deadends" {
			genSealRoom(g, g.rooms[0].id)
		}
	})
	if err != nil {
		t.Fatal(err)
	}
	got["finish-sealed-first-room-no-stairs"] = fmt.Sprintf("discarded %v, entrance %+v, %s", d.DiscardedRooms, d.Entrance, genHex(h))
}

// genBareGen is an empty 15 by 15 working state, for the white-box cases.
func genBareGen(t *testing.T) *gen {
	t.Helper()
	o, err := normalize(opts(1, 15, 15))
	if err != nil {
		t.Fatal(err)
	}
	return newGen(o)
}

// genCharWhiteBoxCases covers the branches no generated level reaches: a
// threshold whose beyond is the same room, a stair site at the grid's edge or
// beside a door, and an open square on the border for the component labels
// and the renderings.
func genCharWhiteBoxCases(t *testing.T, got map[string]string) {
	t.Helper()
	g := genBareGen(t)
	a := g.addRoom(1, 1, 3, 3)
	same := &room{id: a.id, x: 5, y: 1, w: 3, h: 3}
	_, ok := g.candidateOK(same, West, 0)
	got["candidate-beyond-is-the-same-room"] = fmt.Sprint(ok)

	g = genBareGen(t)
	g.kind[g.idx(5, 0)], g.kind[g.idx(5, 1)] = KindCorridor, KindCorridor
	b, ok := g.stairSite(g.idx(5, 0))
	got["stair-site-on-the-top-edge"] = fmt.Sprint(b, ok)
	g.kind[g.idx(7, 7)], g.kind[g.idx(7, 8)], g.kind[g.idx(8, 7)] = KindCorridor, KindCorridor, KindDoor
	b, ok = g.stairSite(g.idx(7, 7))
	got["stair-site-beside-a-door"] = fmt.Sprint(b, ok)
	g.kind[g.idx(3, 9)], g.kind[g.idx(3, 10)], g.kind[g.idx(3, 8)] = KindCorridor, KindCorridor, KindCorridor
	b, ok = g.stairSite(g.idx(3, 9))
	got["stair-site-two-corridors"] = fmt.Sprint(b, ok)

	g = genBareGen(t)
	for _, p := range [][2]int{{0, 3}, {1, 3}, {14, 14}, {13, 14}, {6, 6}} {
		g.kind[g.idx(p[0], p[1])] = KindCorridor
	}
	c := g.labelComponents()
	var comps []string
	for id := range int32(len(c.starts) - 1) { //nolint:gosec // G115: a handful of components
		comps = append(comps, fmt.Sprint(c.squares(id)))
	}
	got["components-on-the-border"] = strings.Join(comps, " ")

	// The hand-built row of TestGroupLinksAfterAnotherIsDiscarded: the front
	// group is discarded, then the other links through the freed squares.
	g = genGroupRow(t)
	h := sha256.New()
	g.onLater = func(n int) { fmt.Fprintf(h, "later %d\n", n) }
	g.connect()
	genHashState(h, "connect", g)
	got["connect-links-after-a-discard"] = fmt.Sprintf("discarded %v, %s", g.discarded, genHex(h))

	d := &Dungeon{Width: 4, Height: 3, Kinds: make([]Kind, 12), RoomIDs: make([]uint16, 12), CorridorIDs: make([]uint16, 12), Mask: make([]bool, 12)}
	for i, k := range []Kind{KindCorridor, KindCorridor, KindRock, KindCorridor, KindRock, KindRock, KindRock, KindCorridor, KindCorridor, KindRock, KindRoom, KindCorridor} {
		d.Kinds[i] = k
		d.Mask[i] = i != 6
		if k == KindCorridor {
			d.CorridorIDs[i] = uint16(i + 1)
		}
	}
	d.Entrance = Entrance{X: 2, Y: 2}
	conns, nets := connectionsAndNetworks(d)
	got["networks-on-the-border"] = fmt.Sprintf("%+v %+v", conns, nets)
	open, wall := WallsMask(d)
	got["render-on-the-border"] = fmt.Sprintf("%q %v %v", ASCII(d), open, wall)
}

// genGroupRow is the hand-built row of three rooms of
// TestGroupLinksAfterAnotherIsDiscarded, before the connectivity phase.
func genGroupRow(t *testing.T) *gen {
	t.Helper()
	o, err := normalize(opts(1, 19, 15))
	if err != nil {
		t.Fatal(err)
	}
	g := newGen(o)
	for y := range g.h {
		for x := range g.w {
			g.blocked[g.idx(x, y)] = y <= 3 || y >= 9 || x >= 17
		}
	}
	g.anyBlocked = true
	g.blocked[g.idx(7, 7)] = true
	g.addRoom(1, 5, 3, 3)
	g.addRoom(9, 5, 3, 3)
	g.addRoom(13, 5, 3, 3)
	for _, sq := range [][2]int{{4, 5}, {8, 5}} {
		g.kind[g.idx(sq[0], sq[1])] = KindDoor
	}
	for _, sq := range [][2]int{{5, 5}, {7, 5}} {
		g.kind[g.idx(sq[0], sq[1])] = KindCorridor
		g.lane[g.idx(sq[0], sq[1])] = true
	}
	g.directPairs[pairKey(2, 3)] = struct{}{}
	return g
}

// genCharNormalizeCases records normalize's answer (the error, or the
// normalised sizes) for every branch, and the order of its checks: with two
// bad options, the first in that order is the one reported.
func genCharNormalizeCases(got map[string]string) {
	cm := func(w, h, n int) *CustomMask { return &CustomMask{Width: w, Height: h, On: make([]bool, n)} }
	cases := map[string]func(o *Options){
		"defaults":               func(*Options) {},
		"even-sizes":             func(o *Options) { o.Width, o.Height, o.RoomSideMin, o.RoomSideMax = 50, 24, 4, 10 },
		"sides-14-14":            func(o *Options) { o.RoomSideMin, o.RoomSideMax = 14, 14 },
		"version-low":            func(o *Options) { o.Version = 0 },
		"version-and-width":      func(o *Options) { o.Version, o.Width = 2, 1 },
		"width-low":              func(o *Options) { o.Width = 14 },
		"width-and-height":       func(o *Options) { o.Width, o.Height = 200, 400 },
		"height-high":            func(o *Options) { o.Height = 400 },
		"height-and-mask":        func(o *Options) { o.Height, o.Mask = 3, "blob" },
		"mask-unknown":           func(o *Options) { o.Mask = "blob" },
		"mask-and-sides":         func(o *Options) { o.Mask, o.RoomSideMin = "blob", 1 },
		"donut-hole-low":         func(o *Options) { o.Mask, o.MaskHole = MaskDonut, 19 },
		"donut-hole-high":        func(o *Options) { o.Mask, o.MaskHole = MaskDonut, 61 },
		"hole-ignored-elsewhere": func(o *Options) { o.Mask, o.MaskHole = MaskPlus, 99 },
		"custom-nil":             func(o *Options) { o.Mask = MaskCustom },
		"custom-small":           func(o *Options) { o.Mask, o.CustomMask = MaskCustom, cm(3, 2, 6) },
		"custom-large":           func(o *Options) { o.Mask, o.CustomMask, o.Width = MaskCustom, cm(50, 3, 150), 50 },
		"custom-cells":           func(o *Options) { o.Mask, o.CustomMask = MaskCustom, cm(3, 3, 8) },
		"custom-ok":              func(o *Options) { o.Mask, o.CustomMask = MaskCustom, cm(3, 3, 9) },
		"side-min-low":           func(o *Options) { o.RoomSideMin = 2 },
		"side-max-below-min":     func(o *Options) { o.RoomSideMin, o.RoomSideMax = 9, 7 },
		"side-max-and-aspect":    func(o *Options) { o.RoomSideMax, o.AspectLimit = 32, 0 },
		"aspect-nan":             func(o *Options) { o.AspectLimit = math.NaN() },
		"aspect-and-density":     func(o *Options) { o.AspectLimit, o.RoomDensity = 7, 0 },
		"density-high":           func(o *Options) { o.RoomDensity = 201 },
		"density-and-placement":  func(o *Options) { o.RoomDensity, o.Placement = 5, "x" },
		"placement-and-style":    func(o *Options) { o.Placement, o.CorridorStyle = "x", "" },
		"style-and-doors":        func(o *Options) { o.CorridorStyle, o.DoorDensity = "y", 1 },
		"doors-and-mix":          func(o *Options) { o.DoorDensity, o.DoorMix = 201, "none" },
		"mix-and-deadends":       func(o *Options) { o.DoorMix, o.DeadendRemoval = "none", -1 },
		"deadends-and-stairs":    func(o *Options) { o.DeadendRemoval, o.Stairs = 101, 9 },
		"stairs-and-loops":       func(o *Options) { o.Stairs, o.ExtraLoops = -1, 101 },
		"loops-low":              func(o *Options) { o.ExtraLoops = -1 },
	}
	for name, f := range cases {
		o := DefaultOptions(1)
		f(&o)
		n, err := normalize(o)
		if err != nil {
			var oe *OptionError
			got["normalize-"+name] = fmt.Sprintf("%v (option error %v, invalid %v)", err, errors.As(err, &oe), errors.Is(err, ErrInvalidOption))
			continue
		}
		got["normalize-"+name] = fmt.Sprintf("%dx%d sides %d..%d", n.Width, n.Height, n.RoomSideMin, n.RoomSideMax)
	}
}

func TestGoldenGeneratorCharacterization(t *testing.T) {
	if testing.Short() {
		t.Skip("the characterization runs 2000 generations")
	}
	got := map[string]string{}
	genCharGenerationCases(t, got)
	genCharFinishCases(t, got)
	genCharWhiteBoxCases(t, got)
	genCharNormalizeCases(got)
	if *update {
		b, err := json.MarshalIndent(got, "", "  ")
		if err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(generatorGoldenPath, append(b, '\n'), 0o600); err != nil {
			t.Fatal(err)
		}
		return
	}
	raw, err := os.ReadFile(generatorGoldenPath)
	if err != nil {
		t.Fatal(err)
	}
	var want map[string]string
	if err := json.Unmarshal(raw, &want); err != nil {
		t.Fatal(err)
	}
	for name, v := range got {
		if w, ok := want[name]; !ok || w != v {
			t.Errorf("%s:\n got  %q\n want %q", name, v, w)
		}
	}
	for name := range want {
		if _, ok := got[name]; !ok {
			t.Errorf("golden has %q, the test does not", name)
		}
	}
}
