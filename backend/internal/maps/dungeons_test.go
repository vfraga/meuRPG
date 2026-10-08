package maps

import (
	"bytes"
	"errors"
	"fmt"
	"image"
	"image/color"
	"image/png"
	"log/slog"
	"net/http"
	"os"
	"slices"
	"strconv"
	"strings"
	"testing"
	"time"

	"connectrpc.com/connect"
	"github.com/jackc/pgx/v5"
	"google.golang.org/protobuf/proto"

	mapsv1 "github.com/PuraFome/meuRPG/backend/gen/meurpg/maps/v1"
	"github.com/PuraFome/meuRPG/backend/internal/maps/dungeonimg"
	"github.com/PuraFome/meuRPG/backend/internal/maps/mapsdb"
	"github.com/PuraFome/meuRPG/backend/internal/platform/blob"
	"github.com/PuraFome/meuRPG/backend/internal/platform/db"
	"github.com/PuraFome/meuRPG/backend/internal/platform/ratelimit"
	"github.com/PuraFome/meuRPG/backend/internal/rules/dungeon"
	"github.com/PuraFome/meuRPG/backend/internal/rules/grid"
)

// A generated dungeon becomes a campaign map (MR-010, RN-26, RN-10; slice 10.6d).
// The generator itself is tested in package rules/dungeon; these tests prove what
// the server builds from its answer: the map, its layers, the image, the record,
// the rooms list, "Redesenhar", and that a player never reads any of it.

// dungeonTable is the table of these tests: Mirathel's master, Ana (Pensantus) and
// Caio, with no map yet.
type dungeonTable struct {
	h                 *harness
	master, ana, caio *user
	campaign          string
	pens              string // Pensantus's ID
}

func newDungeonTable(t *testing.T, configure ...func(*Config)) *dungeonTable {
	t.Helper()
	h := newHarness(t, configure...)
	// A test creates many dungeons at once: the limiter has its own test.
	h.svc.dungeonLimit = ratelimit.New(ratelimit.Config{
		PerClient: ratelimit.Rate{Burst: 1000, Every: time.Millisecond}, Global: ratelimit.Rate{Burst: 1000, Every: time.Millisecond}, MaxClients: 16,
	})
	d := &dungeonTable{h: h, master: h.newUser("Mestre"), ana: h.newUser("Ana"), caio: h.newUser("Caio")}
	d.campaign = h.newCampaign(d.master, d.ana, d.caio)
	d.pens = d.ana.pc(d.campaign, "Pensantus", "race:gnome").GetId()
	return d
}

// testDungeonOptions are the options of the tests: 41 x 31 squares, many doors of
// every kind, two stairs.
func testDungeonOptions() *mapsv1.DungeonOptions {
	return &mapsv1.DungeonOptions{
		Width: proto.Int32(41), Height: proto.Int32(31), DoorMix: mapsv1.DungeonDoorMix_DUNGEON_DOOR_MIX_PARANOID,
		DoorDensity: proto.Int32(150), Stairs: proto.Int32(2),
	}
}

// testDungeonSeed is the first seed whose dungeon, with testDungeonOptions, has a
// secret door, a locked or barred one, a closed one, a trapped one and two stairs:
// the generator is deterministic, so it is always the same.
func testDungeonSeed(t *testing.T) (uint64, *dungeon.Dungeon) {
	t.Helper()
	for seed := uint64(1); seed < 2000; seed++ {
		opts, err := dungeonOptionsFromProto(testDungeonOptions(), seed)
		if err != nil {
			t.Fatal(err)
		}
		d, err := dungeon.Generate(opts)
		if err != nil {
			t.Fatalf("Generate(%d) error = %v", seed, err)
		}
		kinds, trapped := map[dungeon.DoorKind]int{}, 0
		for _, dr := range d.Doors {
			kinds[dr.Kind]++
			if dr.Trapped {
				trapped++
			}
		}
		if kinds[dungeon.DoorSecret] > 0 && kinds[dungeon.DoorLocked] > 0 && kinds[dungeon.DoorBarred] > 0 && kinds[dungeon.DoorClosed] > 0 &&
			kinds[dungeon.DoorArchway] > 0 && trapped > 0 && d.StairsPlaced == 2 && len(d.Rooms) >= 4 {
			return seed, d
		}
	}
	t.Fatal("no seed below 2000 gives a dungeon with every kind of door")
	return 0, nil
}

func (u *user) tryCreateDungeon(campaignID, name string, opts *mapsv1.DungeonOptions, seed *uint64) (*mapsv1.CreateDungeonMapResponse, error) {
	res, err := u.dungeons.CreateDungeonMap(u.h.t.Context(), connect.NewRequest(&mapsv1.CreateDungeonMapRequest{CampaignId: campaignID, Name: name, Options: opts, Seed: seed}))
	if err != nil {
		return nil, err
	}
	return res.Msg, nil
}

func (u *user) createDungeon(campaignID, name string, opts *mapsv1.DungeonOptions, seed uint64) *mapsv1.CreateDungeonMapResponse {
	u.h.t.Helper()
	res, err := u.tryCreateDungeon(campaignID, name, opts, &seed)
	if err != nil {
		u.h.t.Fatalf("CreateDungeonMap() error = %v", err)
	}
	return res
}

func (u *user) previewDungeon(campaignID string, opts *mapsv1.DungeonOptions, seed *uint64) (*mapsv1.PreviewDungeonResponse, error) {
	res, err := u.dungeons.PreviewDungeon(u.h.t.Context(), connect.NewRequest(&mapsv1.PreviewDungeonRequest{CampaignId: campaignID, Options: opts, Seed: seed}))
	if err != nil {
		return nil, err
	}
	return res.Msg, nil
}

func (u *user) dungeonRooms(campaignID, mapID string) (*mapsv1.GetDungeonRoomsResponse, error) {
	res, err := u.dungeons.GetDungeonRooms(u.h.t.Context(), connect.NewRequest(&mapsv1.GetDungeonRoomsRequest{CampaignId: campaignID, MapId: mapID}))
	if err != nil {
		return nil, err
	}
	return res.Msg, nil
}

func (u *user) redraw(campaignID, mapID string) (*mapsv1.RedrawDungeonMapResponse, error) {
	res, err := u.dungeons.RedrawDungeonMap(u.h.t.Context(), connect.NewRequest(&mapsv1.RedrawDungeonMapRequest{CampaignId: campaignID, MapId: mapID}))
	if err != nil {
		return nil, err
	}
	return res.Msg, nil
}

func (u *user) placeScene(campaignID, mapID string, room int32) (*mapsv1.PlaceDungeonSceneResponse, error) {
	res, err := u.dungeons.PlaceDungeonScene(u.h.t.Context(), connect.NewRequest(&mapsv1.PlaceDungeonSceneRequest{CampaignId: campaignID, MapId: mapID, RoomId: room}))
	if err != nil {
		return nil, err
	}
	return res.Msg, nil
}

// layersOf reads a map's layers as the master, for a dungeon's grid.
func (d *dungeonTable) layersOf(mapID string, g grid.Grid) (walls *grid.Layer, doors *grid.DoorLayer, res *mapsv1.GetMapLayersResponse) {
	d.h.t.Helper()
	res = d.master.mustLayers(d.campaign, mapID)
	wb, db := res.GetWall(), res.GetDoors()
	if len(wb) == 0 {
		wb = make([]byte, grid.LayerSize(g))
	}
	if len(db) == 0 {
		db = make([]byte, grid.DoorLayerSize(g))
	}
	var err error
	if walls, err = grid.DecodeLayer(g, wb); err != nil {
		d.h.t.Fatalf("decode the walls: %v", err)
	}
	if doors, err = grid.DecodeDoorLayer(g, db); err != nil {
		d.h.t.Fatalf("decode the doors: %v", err)
	}
	return walls, doors, res
}

// imageOf fetches and decodes the map's image as the master.
func (d *dungeonTable) imageOf(m *mapsv1.Map) image.Image {
	d.h.t.Helper()
	res := d.master.get(m.GetImage().GetUrl())
	if res.status != http.StatusOK {
		d.h.t.Fatalf("GET %s: status %d", m.GetImage().GetUrl(), res.status)
	}
	img, err := png.Decode(bytes.NewReader(res.body))
	if err != nil {
		d.h.t.Fatalf("decode the map's image: %v", err)
	}
	return img
}

// pixel is the color at the middle of a square of an image of p pixels a square.
func pixel(img image.Image, p, col, row int) color.Color {
	return img.At(col*p+p/2, row*p+p/2)
}

func sameColor(a, b color.Color) bool {
	ar, ag, ab, aa := a.RGBA()
	br, bg, bb, ba := b.RGBA()
	return ar == br && ag == bg && ab == bb && aa == ba
}

// wallColors are the colors a wall square can have in the image: every color
// of a square the dungeon surely leaves solid (the corner).
func wallColors(img image.Image, p int) []color.Color {
	var out []color.Color
	for y := range p {
		for x := range p {
			c := img.At(x, y)
			if !slices.ContainsFunc(out, func(o color.Color) bool { return sameColor(o, c) }) {
				out = append(out, c)
			}
		}
	}
	return out
}

func isWall(img image.Image, p, col, row int, walls []color.Color) bool {
	c := pixel(img, p, col, row)
	return slices.ContainsFunc(walls, func(w color.Color) bool { return sameColor(w, c) })
}

// MR-010: the preview is a pure answer. The same options and seed give the same
// layout, a missing seed is drawn and returned, the options come back as used, the
// layout is the generator's, and nothing is stored.
func TestMR010_PreviewIsDeterministicAndStoresNothing(t *testing.T) {
	t.Parallel()
	d := newDungeonTable(t)
	m := d.master
	seed, want := testDungeonSeed(t)

	a, err := m.previewDungeon(d.campaign, testDungeonOptions(), &seed)
	if err != nil {
		t.Fatalf("PreviewDungeon() error = %v", err)
	}
	b, err := m.previewDungeon(d.campaign, testDungeonOptions(), &seed)
	if err != nil {
		t.Fatalf("PreviewDungeon() again error = %v", err)
	}
	if !proto.Equal(a, b) {
		t.Error("the same options and seed gave two different previews")
	}
	if a.GetSeed() != seed || a.GetWidth() != 41 || a.GetHeight() != 31 || a.GetGeneratorVersion() != dungeon.Version {
		t.Errorf("preview = seed %d, %d x %d, version %d", a.GetSeed(), a.GetWidth(), a.GetHeight(), a.GetGeneratorVersion())
	}
	if len(a.GetDoors()) != len(want.Doors) || len(a.GetStairs()) != 2 || len(a.GetRooms()) != len(want.Rooms) {
		t.Errorf("preview has %d doors, %d stairs, %d rooms; the generator made %d, 2, %d", len(a.GetDoors()), len(a.GetStairs()), len(a.GetRooms()), len(want.Doors), len(want.Rooms))
	}
	open, _ := dungeon.WallsMask(want)
	g := grid.Grid{Columns: 41, Rows: 31}
	layer, err := grid.DecodeLayer(g, a.GetOpen())
	if err != nil {
		t.Fatalf("decode the open squares: %v", err)
	}
	for i, o := range open {
		if layer.Get(i%41, i/41) != o {
			t.Fatalf("open square %d = %v, want %v", i, !o, o)
		}
	}
	if !a.GetStairs()[0].GetUp() || a.GetStairs()[1].GetUp() {
		t.Error("the first stair is up and the second down")
	}
	// The options come back as used: the defaults filled in.
	if a.GetOptions().GetRoomSideMax() != 11 || a.GetOptions().GetMask() != mapsv1.DungeonMask_DUNGEON_MASK_NONE || a.GetOptions().GetAspectLimit() != 3.0 {
		t.Errorf("options used = %v, want the generator's defaults where none was sent", a.GetOptions())
	}

	// No seed: the server draws one, and the next call another; an empty message
	// is the default dungeon, 51 x 51; an even size goes down to the odd one.
	first, err := m.previewDungeon(d.campaign, &mapsv1.DungeonOptions{}, nil)
	if err != nil {
		t.Fatalf("PreviewDungeon(no options) error = %v", err)
	}
	second, _ := m.previewDungeon(d.campaign, &mapsv1.DungeonOptions{}, nil)
	// No options message at all is the same default dungeon (it used to panic).
	seeded := first.GetSeed()
	none, err := m.previewDungeon(d.campaign, nil, &seeded)
	if err != nil || !proto.Equal(none, first) {
		t.Errorf("PreviewDungeon(nil options) = %v, %v; want the default dungeon of seed %d", none, err, seeded)
	}
	if first.GetWidth() != 51 || first.GetHeight() != 51 || first.GetSeed() == second.GetSeed() {
		t.Errorf("default previews: %d x %d, seeds %d and %d; want 51 x 51 and two seeds", first.GetWidth(), first.GetHeight(), first.GetSeed(), second.GetSeed())
	}
	zero := uint64(0)
	if z, err := m.previewDungeon(d.campaign, &mapsv1.DungeonOptions{Width: proto.Int32(40), Height: proto.Int32(30)}, &zero); err != nil || z.GetSeed() != 0 || z.GetWidth() != 39 || z.GetHeight() != 29 {
		t.Errorf("seed 0 and even sizes: %v, %v; want seed 0 kept and 39 x 29", z, err)
	}

	// Nothing was stored: no map, no image.
	if got := len(m.listMaps(d.campaign)); got != 0 {
		t.Errorf("a preview left %d maps", got)
	}
	if got := len(m.list(d.campaign).GetImages()); got != 0 {
		t.Errorf("a preview left %d gallery images", got)
	}
	if files := d.h.storedFiles(); len(files) != 0 {
		t.Errorf("a preview left files: %v", files)
	}
}

// An option out of range is refused naming the option (the generator's *OptionError),
// and the same holds for a creation, which stores nothing.
func TestMR010_OptionsOutOfRangeAreRefusedByName(t *testing.T) {
	t.Parallel()
	d := newDungeonTable(t)
	m := d.master
	refused := func(err error) (string, bool) {
		ce, ok := errors.AsType[*connect.Error](err)
		if !ok || ce.Code() != connect.CodeInvalidArgument {
			return "", false
		}
		for _, det := range ce.Details() {
			if v, err := det.Value(); err == nil {
				if r, ok := v.(*mapsv1.DungeonOptionRefused); ok {
					return r.GetOption(), true
				}
			}
		}
		return "", false
	}
	for name, c := range map[string]struct {
		opts   *mapsv1.DungeonOptions
		option string
	}{
		"width under 15":      {&mapsv1.DungeonOptions{Width: proto.Int32(14)}, "width"},
		"width over 199":      {&mapsv1.DungeonOptions{Width: proto.Int32(200)}, "width"},
		"height over 399":     {&mapsv1.DungeonOptions{Height: proto.Int32(400)}, "height"},
		"room side min 2":     {&mapsv1.DungeonOptions{RoomSideMin: proto.Int32(2)}, "room_side_min"},
		"room side max 33":    {&mapsv1.DungeonOptions{RoomSideMax: proto.Int32(33)}, "room_side_max"},
		"aspect 7":            {&mapsv1.DungeonOptions{AspectLimit: proto.Float64(7)}, "aspect_limit"},
		"room density 5":      {&mapsv1.DungeonOptions{RoomDensity: proto.Int32(5)}, "room_density"},
		"door density 300":    {&mapsv1.DungeonOptions{DoorDensity: proto.Int32(300)}, "door_density"},
		"deadends 101":        {&mapsv1.DungeonOptions{DeadendRemoval: proto.Int32(101)}, "deadend_removal"},
		"9 stairs":            {&mapsv1.DungeonOptions{Stairs: proto.Int32(9)}, "stairs"},
		"loops -1":            {&mapsv1.DungeonOptions{ExtraLoops: proto.Int32(-1)}, "extra_loops"},
		"hole 10 on a donut":  {&mapsv1.DungeonOptions{Mask: mapsv1.DungeonMask_DUNGEON_MASK_DONUT, MaskHole: proto.Int32(10)}, "mask_hole"},
		"unknown mask":        {&mapsv1.DungeonOptions{Mask: mapsv1.DungeonMask(99)}, "mask"},
		"unknown placement":   {&mapsv1.DungeonOptions{Placement: mapsv1.DungeonPlacement(99)}, "placement"},
		"unknown style":       {&mapsv1.DungeonOptions{CorridorStyle: mapsv1.DungeonCorridorStyle(99)}, "corridor_style"},
		"unknown door mix":    {&mapsv1.DungeonOptions{DoorMix: mapsv1.DungeonDoorMix(99)}, "door_mix"},
		"max below min sides": {&mapsv1.DungeonOptions{RoomSideMin: proto.Int32(9), RoomSideMax: proto.Int32(5)}, "room_side_max"},
	} {
		_, err := m.previewDungeon(d.campaign, c.opts, nil)
		if option, ok := refused(err); !ok || option != c.option {
			t.Errorf("preview with %s: error = %v, option %q; want invalid_argument naming %q", name, err, option, c.option)
		}
		_, err = m.tryCreateDungeon(d.campaign, "Masmorra", c.opts, nil)
		if option, ok := refused(err); !ok || option != c.option {
			t.Errorf("create with %s: error = %v, option %q; want invalid_argument naming %q", name, err, option, c.option)
		}
	}
	// The name has CreateMap's rules.
	_, err := m.tryCreateDungeon(d.campaign, "  ", testDungeonOptions(), nil)
	wantCode(t, "create with a blank name", err, connect.CodeInvalidArgument)
	if len(m.listMaps(d.campaign)) != 0 || len(d.h.storedFiles()) != 0 {
		t.Error("a refused creation left a map or a file")
	}
}

// MR-010, the story's third criterion: a generated dungeon becomes a map of the
// campaign, hidden, with the grid, the fog and the base light, the walls and the
// doors layers cell by cell as the generator says, the stairs as submap points, a
// flat image of floors and walls only, and the dungeon's record.
func TestMR010_ADungeonBecomesAMap(t *testing.T) {
	t.Parallel()
	d := newDungeonTable(t)
	m := d.master
	seed, want := testDungeonSeed(t)
	res := m.createDungeon(d.campaign, "Masmorra de Vesna", testDungeonOptions(), seed)
	created := res.GetMap()
	if res.GetSeed() != seed || res.GetRoomCount() != int32(len(want.Rooms)) { //nolint:gosec // G115: at most 500 rooms
		t.Errorf("answer = seed %d, %d rooms; want %d and %d", res.GetSeed(), res.GetRoomCount(), seed, len(want.Rooms))
	}

	// The map: hidden, grid = the dungeon's width, rows to match, fog on, base light bright.
	if created.GetName() != "Masmorra de Vesna" || created.GetRevealed() || created.GetGridColumns() != 41 || created.GetGridRows() != 31 ||
		!created.GetFogEnabled() || created.GetBaseLight() != mapsv1.LightLevel_LIGHT_LEVEL_BRIGHT || created.GetGroupVision() {
		t.Errorf("map = %v", created)
	}
	g := grid.Grid{Columns: 41, Rows: 31}
	if created.GetImage().GetWidth() != 41*24 || created.GetImage().GetHeight() != 31*24 {
		t.Errorf("image = %d x %d pixels, want %d x %d (24 a square)", created.GetImage().GetWidth(), created.GetImage().GetHeight(), 41*24, 31*24)
	}
	if got := d.master.listMaps(d.campaign); len(got) != 1 || got[0].GetId() != created.GetId() {
		t.Errorf("the campaign's maps = %v", got)
	}

	// The layers, cell by cell.
	// The walls layer covers every square that is not open: the walls and all the rock.
	wantOpen, _ := dungeon.WallsMask(want)
	walls, doors, layers := d.layersOf(created.GetId(), g)
	if layers.GetGridColumns() != 41 || layers.GetGridRows() != 31 || layers.GetLayersRevision() < 1 {
		t.Errorf("layers = %d x %d, revision %d", layers.GetGridColumns(), layers.GetGridRows(), layers.GetLayersRevision())
	}
	doorAt := map[grid.Square]dungeon.Door{}
	for _, dr := range want.Doors {
		doorAt[grid.Square{Col: dr.X, Row: dr.Y}] = dr
	}
	wantState := map[dungeon.DoorKind]grid.Door{
		dungeon.DoorArchway: grid.DoorNone, dungeon.DoorClosed: grid.DoorClosed, dungeon.DoorLocked: grid.DoorLocked,
		dungeon.DoorBarred: grid.DoorBarred, dungeon.DoorSecret: grid.DoorSecret,
	}
	for row := range 31 {
		for col := range 41 {
			if got := walls.Get(col, row); got == wantOpen[row*41+col] {
				t.Fatalf("wall (%d, %d) = %v, want %v (every square that is not open)", col, row, got, !got)
			}
			wantDoor := grid.DoorNone
			if dr, ok := doorAt[grid.Square{Col: col, Row: row}]; ok {
				wantDoor = wantState[dr.Kind]
			}
			if got := doors.Get(col, row); got != wantDoor {
				t.Fatalf("door (%d, %d) = %d, want %d", col, row, got, wantDoor)
			}
		}
	}
	// A door with a trap is the same door in the layer, and the map has no trap point.
	trapped := 0
	for _, dr := range want.Doors {
		if dr.Trapped {
			trapped++
			if dr.Kind == dungeon.DoorArchway {
				t.Error("an archway has a trap")
			}
		}
	}
	if trapped == 0 {
		t.Fatal("the fixture has no trapped door")
	}

	// The stairs: submap points, no target, revealed like a door, named by direction, at the middle of their squares.
	points := m.mustGetMap(d.campaign, created.GetId()).GetPoints()
	if len(points) != 2 || created.GetPointCount() != 2 {
		t.Fatalf("points = %d (map says %d), want the 2 stairs", len(points), created.GetPointCount())
	}
	for i, p := range points {
		st := want.Stairs[i]
		wantName, x, y := stairsDownName, 0, 0
		if st.Kind == dungeon.StairUp {
			wantName = stairsUpName
		}
		x, y = g.CenterOf(grid.Square{Col: st.X, Row: st.Y})
		wantDir := mapsv1.StairDirection_STAIR_DIRECTION_DOWN
		if st.Kind == dungeon.StairUp {
			wantDir = mapsv1.StairDirection_STAIR_DIRECTION_UP
		}
		if p.GetStairs() != wantDir {
			t.Errorf("stair %d has stairs = %v, want %v", i, p.GetStairs(), wantDir)
		}
		if p.GetKind() != mapsv1.MapPointKind_MAP_POINT_KIND_SUBMAP || p.GetName() != wantName || p.GetTargetMap() != nil || !p.GetRevealed() ||
			p.GetXBp() != int32(x) || p.GetYBp() != int32(y) { //nolint:gosec // G115: 0 to 10000
			t.Errorf("stair %d = %v, want a revealed submap %q at (%d, %d)", i, p, wantName, x, y)
		}
	}

	// The image: one hidden gallery image of the campaign, floors and walls only.
	imgs := m.list(d.campaign).GetImages()
	if len(imgs) != 1 || imgs[0].GetId() != created.GetImage().GetId() || imgs[0].GetContentType() != "image/png" || imgs[0].GetName() != "Masmorra de Vesna (masmorra)" {
		t.Fatalf("gallery = %v", imgs)
	}
	img := d.imageOf(created)
	const p = 24
	walled := wallColors(img, p) // the corner square is solid
	room := want.Rooms[0]
	cx, cy := room.Center()
	floor := pixel(img, p, cx, cy)
	if isWall(img, p, cx, cy, walled) {
		t.Fatal("the middle of a room is drawn as wall")
	}
	for _, dr := range want.Doors {
		switch dr.Kind {
		case dungeon.DoorSecret:
			if !isWall(img, p, dr.X, dr.Y, walled) {
				t.Errorf("the secret door at (%d, %d) is not drawn as wall", dr.X, dr.Y)
			}
		default: // a passage, a closed, locked or barred door: floor, with no mark of a door
			if !sameColor(pixel(img, p, dr.X, dr.Y), floor) {
				t.Errorf("the %v door at (%d, %d) is not drawn as floor", dr.Kind, dr.X, dr.Y)
			}
		}
	}
	for row := range 31 {
		for col := range 41 {
			if want.Kinds[row*41+col] == dungeon.KindRock && !isWall(img, p, col, row, walled) {
				t.Fatalf("rock square (%d, %d) is not wall", col, row)
			}
			if want.Kinds[row*41+col] == dungeon.KindRoom && !sameColor(pixel(img, p, col, row), floor) {
				t.Fatalf("room square (%d, %d) is not floor", col, row)
			}
		}
	}
	// A player can't fetch it (the map is hidden), and nothing in the stored file
	// but pixels: a PNG with no text chunk.
	if r := d.ana.get(created.GetImage().GetUrl()); r.status != http.StatusNotFound {
		t.Errorf("a player fetched the hidden map's image: status %d", r.status)
	}

	// The record: the options as used, the seed, the version and the cells.
	rec, err := d.rec(created.GetId())
	if err != nil {
		t.Fatalf("read the dungeon's row: %v", err)
	}
	if uint64(rec.Seed) != seed || rec.GeneratorVersion != dungeon.Version || rec.Width != 41 || rec.Height != 31 { //nolint:gosec // G115: the same bits
		t.Errorf("row = seed %d, version %d, %d x %d", rec.Seed, rec.GeneratorVersion, rec.Width, rec.Height)
	}
	if kinds := unpackCells(rec.Cells, 41*31); !slices.Equal(kinds, want.Kinds) {
		t.Error("the stored cells are not the generator's")
	}
	rooms, err := m.dungeonRooms(d.campaign, created.GetId())
	if err != nil {
		t.Fatalf("GetDungeonRooms() error = %v", err)
	}
	if rooms.GetSeed() != seed || rooms.GetGeneratorVersion() != dungeon.Version || rooms.GetWidth() != 41 || rooms.GetHeight() != 31 || len(rooms.GetRooms()) != len(want.Rooms) ||
		rooms.GetOptions().GetDoorMix() != mapsv1.DungeonDoorMix_DUNGEON_DOOR_MIX_PARANOID || rooms.GetCreatedAt() == nil {
		t.Errorf("rooms = seed %d, version %d, %d rooms, options %v", rooms.GetSeed(), rooms.GetGeneratorVersion(), len(rooms.GetRooms()), rooms.GetOptions())
	}
	if !rooms.GetImageIsGenerated() || len(rooms.GetDoors()) != len(want.Doors) || len(rooms.GetStairs()) != 2 ||
		int(rooms.GetEntrance().GetX()) != want.Entrance.X || int(rooms.GetEntrance().GetY()) != want.Entrance.Y || rooms.GetEntrance().GetOnStairs() != want.Entrance.OnStairs {
		t.Errorf("rooms list: generated %v, %d doors (want %d), %d stairs, entrance %v", rooms.GetImageIsGenerated(), len(rooms.GetDoors()), len(want.Doors), len(rooms.GetStairs()), rooms.GetEntrance())
	}
	exits, trappedExits := 0, 0
	for i, r := range rooms.GetRooms() {
		w := want.Rooms[i]
		cx, cy := w.Center()
		if int(r.GetId()) != w.ID || int(r.GetFloor().GetX()) != w.X || int(r.GetFloor().GetY()) != w.Y || int(r.GetFloor().GetWidth()) != w.Width ||
			int(r.GetFloor().GetHeight()) != w.Height || int(r.GetCenterCol()) != cx || int(r.GetCenterRow()) != cy || len(r.GetExits()) != len(w.Exits) {
			t.Errorf("room %d = %v, want %+v", i, r, w)
		}
		for j, e := range r.GetExits() {
			exits++
			if e.GetTrapped() {
				trappedExits++
			}
			if e.GetKind() != doorKindToProto[w.Exits[j].Kind] || e.GetTrapped() != w.Exits[j].Trapped || int(e.GetOtherRoomId()) != w.Exits[j].Other {
				t.Errorf("room %d exit %d = %v, want %+v", w.ID, j, e, w.Exits[j])
			}
		}
	}
	if exits == 0 || trappedExits == 0 {
		t.Errorf("the rooms list has %d exits, %d with a trap; want the trapped doors listed", exits, trappedExits)
	}
}

// rec reads the dungeon's row straight from the table.
func (d *dungeonTable) rec(mapID string) (struct {
	Seed             int64
	GeneratorVersion int32
	Width, Height    int32
	Cells            []byte
}, error,
) {
	var r struct {
		Seed             int64
		GeneratorVersion int32
		Width, Height    int32
		Cells            []byte
	}
	err := d.h.pool.QueryRow(d.h.t.Context(), `SELECT seed, generator_version, width, height, cells FROM generated_dungeons WHERE map_id = $1`, mapID).
		Scan(&r.Seed, &r.GeneratorVersion, &r.Width, &r.Height, &r.Cells)
	return r, err
}

// RN-10: a player, and anyone who is not a member, gets `not_found` from every method of
// the service, before the map is revealed and after. What the player reads of the
// map after it is revealed comes through the fog, with no secret door, no seed and no
// room.
func TestRN10_PlayersReadNothingOfADungeon(t *testing.T) {
	t.Parallel()
	d := newDungeonTable(t)
	m := d.master
	seed, want := testDungeonSeed(t)
	created := m.createDungeon(d.campaign, "Masmorra de Vesna", testDungeonOptions(), seed).GetMap()
	g := grid.Grid{Columns: 41, Rows: 31}
	stranger := d.h.newUser("Estranho")

	check := func(when string) {
		t.Helper()
		for name, u := range map[string]*user{"a player": d.ana, "another player": d.caio, "a stranger": stranger} {
			_, err := u.previewDungeon(d.campaign, testDungeonOptions(), &seed)
			wantCode(t, when+": "+name+" previews", err, connect.CodeNotFound)
			_, err = u.tryCreateDungeon(d.campaign, "Outra", testDungeonOptions(), &seed)
			wantCode(t, when+": "+name+" creates", err, connect.CodeNotFound)
			_, err = u.dungeonRooms(d.campaign, created.GetId())
			wantCode(t, when+": "+name+" reads the rooms", err, connect.CodeNotFound)
			_, err = u.placeScene(d.campaign, created.GetId(), 1)
			wantCode(t, when+": "+name+" places a scene", err, connect.CodeNotFound)
			_, err = u.redraw(d.campaign, created.GetId())
			wantCode(t, when+": "+name+" redraws", err, connect.CodeNotFound)
		}
		if got := len(m.listMaps(d.campaign)); got != 1 {
			t.Errorf("%s: %d maps, want the one", when, got)
		}
		if got := len(m.mustGetMap(d.campaign, created.GetId()).GetPoints()); got != 2 {
			t.Errorf("%s: the stairs were %d points after the players' tries", when, got)
		}
	}
	check("hidden")
	if _, err := d.ana.getMap(d.campaign, created.GetId()); connect.CodeOf(err) != connect.CodeNotFound {
		t.Errorf("a player read the hidden map: %v", err)
	}
	if got := d.ana.listMaps(d.campaign); len(got) != 0 {
		t.Errorf("a player lists %d maps before the reveal", len(got))
	}
	_, err := d.ana.layers(d.campaign, created.GetId())
	wantCode(t, "a player reads the hidden map's layers", err, connect.CodeNotFound)

	// Revealed: the map comes through the fog, and nothing of the dungeon.
	m.setMapRevealed(d.campaign, created.GetId(), true)
	entrance := grid.Square{Col: want.Entrance.X, Row: want.Entrance.Y}
	d.stand(created.GetId(), g, d.pens, entrance)
	check("revealed")

	// The stairs are born revealed, so with the fog a player sees one only where they see its
	// square: standing on the entrance (the first, up stair), only that one. The master-only flag
	// that says the map is a generated dungeon is never true for a player.
	if !want.Entrance.OnStairs {
		t.Skip("the fixture's entrance is not on the stairs")
	}
	seen, err := d.ana.getMap(d.campaign, created.GetId())
	if err != nil {
		t.Fatalf("a player reads the revealed map: %v", err)
	}
	var names []string
	for _, p := range seen.GetPoints() {
		names = append(names, p.GetName())
	}
	if len(seen.GetPoints()) == 1 && seen.GetPoints()[0].GetStairs() != mapsv1.StairDirection_STAIR_DIRECTION_UP {
		t.Errorf("the stair a player sees has stairs = %v, want UP", seen.GetPoints()[0].GetStairs())
	}
	if !slices.Equal(names, []string{stairsUpName}) {
		t.Errorf("the player sees the points %q, want only %q (the stair on a seen square)", names, stairsUpName)
	}
	if seen.GetMap().GetGeneratedDungeon() {
		t.Error("a player got generated_dungeon = true")
	}
	if !m.mustGetMap(d.campaign, created.GetId()).GetMap().GetGeneratedDungeon() {
		t.Error("the master's generated dungeon has generated_dungeon = false")
	}
	for _, mp := range d.ana.listMaps(d.campaign) {
		if mp.GetGeneratedDungeon() {
			t.Error("a player's map list says generated_dungeon")
		}
	}

	secret := map[grid.Square]bool{}
	for _, dr := range want.Doors {
		if dr.Kind == dungeon.DoorSecret {
			secret[grid.Square{Col: dr.X, Row: dr.Y}] = true
		}
	}
	// A second dungeon with a seed that cannot be mistaken for another number, revealed too.
	second := m.createDungeon(d.campaign, "Outra masmorra", &mapsv1.DungeonOptions{Width: proto.Int32(21), Height: proto.Int32(21)}, 4815162342).GetMap()
	m.setMapRevealed(d.campaign, second.GetId(), true)
	// Everything the player can read of the map, as the app reads it: JSON.
	var all []string
	got := d.ana.mustGetMap(d.campaign, created.GetId())
	all = append(all, asJSON(t, got))
	for _, mp := range d.ana.listMaps(d.campaign) {
		all = append(all, asJSON(t, mp))
	}
	layers := d.ana.mustLayers(d.campaign, created.GetId())
	all = append(all, asJSON(t, asApp(t, layers)))
	all = append(all, asJSON(t, d.ana.mustVision(d.campaign, created.GetId())))
	if !got.GetMap().GetImageWithheld() || got.GetMap().GetImage().GetId() != "" || got.GetMap().GetImage().GetUrl() != "" {
		t.Errorf("a player's map has its image: %v", got.GetMap().GetImage())
	}
	if got.GetMap().GetBaseLight() != mapsv1.LightLevel_LIGHT_LEVEL_UNSPECIFIED || len(got.GetPoints()) != 1 {
		t.Errorf("a player's map shows the master's base light, or not just the one stair on the square they see: %v", got)
	}
	joined := strings.Join(all, "\n")
	// The field goes with the point: the stair on the seen square carries it, and nothing says there is another.
	if !strings.Contains(joined, "STAIR_DIRECTION_UP") || strings.Contains(joined, "STAIR_DIRECTION_DOWN") {
		t.Errorf("a player's answers carry the stair direction of other points than the seen stair:\n%s", joined)
	}
	for _, secretWord := range []string{"4815162342", "room", "Sala", "seed", "options", "generator", "(masmorra)"} {
		if strings.Contains(joined, secretWord) {
			t.Errorf("a player's answers mention %q", secretWord)
		}
	}
	// The fog's tiles are cut from the stored image (a PNG with a palette): the player
	// gets the one their character stands in, and it decodes.
	vision := d.ana.mustVision(d.campaign, created.GetId())
	if len(vision.GetTiles()) == 0 {
		t.Fatal("the player is told of no tile")
	}
	r := d.ana.get(tileURL(vision, vision.GetTiles()[0]))
	if _, err := png.Decode(bytes.NewReader(r.body)); r.status != http.StatusOK || err != nil {
		t.Errorf("the player's tile: status %d, decode %v", r.status, err)
	}
	// The layers the player gets (the fog's filtered view): no nibble says locked or secret, and
	// a secret door's square is a wall wherever the player can see it.
	if !layers.GetFogWithheld() {
		t.Error("a player's layers are not the fog's filtered view")
	}
	pd := doorsOf(t, g, asApp(t, layers))
	for n := range g.Squares() {
		if v := layers.GetDoors(); len(v) > 0 {
			if nib := v[n/2] >> (4 * (n % 2)) & 15; nib == byte(grid.DoorLocked) || nib == byte(grid.DoorSecret) {
				t.Fatalf("a player's doors hold %d at square %d", nib, n)
			}
		}
	}
	for sq := range secret {
		if pd.Get(sq.Col, sq.Row) != grid.DoorNone {
			t.Errorf("a player's doors layer has the secret door (%d, %d)", sq.Col, sq.Row)
		}
	}

	// The whole map, as the master reads it for the player's character ("Ver como"):
	// every secret door is no door there, and where the character sees it, a wall.
	as, err := m.maps.GetMapLayers(t.Context(), connect.NewRequest(&mapsv1.GetMapLayersRequest{CampaignId: d.campaign, MapId: created.GetId(), AsCharacterId: d.pens}))
	if err != nil {
		t.Fatalf("GetMapLayers(as) error = %v", err)
	}
	asDoors := doorsOf(t, g, as.Msg)
	for sq := range secret {
		if asDoors.Get(sq.Col, sq.Row) != grid.DoorNone {
			t.Errorf("seen as the player, the secret door (%d, %d) is in the doors layer", sq.Col, sq.Row)
		}
	}
}

// MR-010: "Pôr uma cena nesta sala" makes the Etapa 8 scene point at the room's
// middle, named "Sala N", through the same path as CreateMapPoint.
func TestMR010_APlacedSceneIsAnRPSceneInTheRoom(t *testing.T) {
	t.Parallel()
	d := newDungeonTable(t)
	m := d.master
	seed, want := testDungeonSeed(t)
	created := m.createDungeon(d.campaign, "Masmorra de Vesna", testDungeonOptions(), seed).GetMap()
	g := grid.Grid{Columns: 41, Rows: 31}
	room := want.Rooms[2]

	res, err := m.placeScene(d.campaign, created.GetId(), int32(room.ID)) //nolint:gosec // G115: at most 500 rooms
	if err != nil {
		t.Fatalf("PlaceDungeonScene() error = %v", err)
	}
	cx, cy := room.Center()
	x, y := g.CenterOf(grid.Square{Col: cx, Row: cy})
	p := res.GetPoint()
	if p.GetKind() != mapsv1.MapPointKind_MAP_POINT_KIND_SCENE || p.GetName() != "Sala "+strconv.Itoa(room.ID) || p.GetXBp() != int32(x) || p.GetYBp() != int32(y) || p.GetRevealed() { //nolint:gosec // G115: 0 to 10000
		t.Errorf("scene = %v, want a hidden scene \"Sala %d\" at (%d, %d)", p, room.ID, x, y)
	}
	points := m.mustGetMap(d.campaign, created.GetId()).GetPoints()
	if len(points) != 3 {
		t.Errorf("the map has %d points, want the 2 stairs and the scene", len(points))
	}
	rooms, err := m.dungeonRooms(d.campaign, created.GetId())
	if err != nil {
		t.Fatal(err)
	}
	for _, r := range rooms.GetRooms() {
		has := slices.Contains(r.GetScenePointIds(), p.GetId())
		if has != (int(r.GetId()) == room.ID) {
			t.Errorf("room %d lists the scene = %v", r.GetId(), has)
		}
	}
	// A scene point works as any: the master can give it actions.
	if a := m.addAction(d.campaign, created.GetId(), p.GetId(), "skill:perception", "Olhar a sala", 12); a.GetId() == "" {
		t.Error("the scene takes no action")
	}
	// Again: another point.
	again, err := m.placeScene(d.campaign, created.GetId(), int32(room.ID)) //nolint:gosec // G115: at most 500 rooms
	if err != nil || again.GetPoint().GetId() == p.GetId() {
		t.Errorf("a second scene for the room = %v, %v; want a new point", again.GetPoint(), err)
	}
	// A room the dungeon does not have, and a map that is not a dungeon.
	_, err = m.placeScene(d.campaign, created.GetId(), 9999)
	wantCode(t, "a room that does not exist", err, connect.CodeNotFound)
	plain := m.createMap(d.campaign, "Comum", m.newImage(d.campaign))
	_, err = m.placeScene(d.campaign, plain.GetId(), 1)
	wantCode(t, "a plain map", err, connect.CodeNotFound)
	_, err = m.dungeonRooms(d.campaign, plain.GetId())
	wantCode(t, "the rooms of a plain map", err, connect.CodeNotFound)
	_, err = m.redraw(d.campaign, plain.GetId())
	wantCode(t, "redraw a plain map", err, connect.CodeNotFound)
	_, err = m.dungeonRooms(d.campaign, "not-a-uuid")
	wantCode(t, "the rooms of a bad ID", err, connect.CodeNotFound)
}

// MR-010, the story's fourth criterion: "Redesenhar" draws the image again from the
// walls and doors the map has now, at the same size, and does not clear the layers
// nor what the players remember.
func TestMR010_RedrawShowsTheMastersEditsAndKeepsTheLayers(t *testing.T) {
	t.Parallel()
	d := newDungeonTable(t)
	m := d.master
	seed, want := testDungeonSeed(t)
	created := m.createDungeon(d.campaign, "Masmorra de Vesna", testDungeonOptions(), seed).GetMap()
	g := grid.Grid{Columns: 41, Rows: 31}
	const p = 24
	m.setMapRevealed(d.campaign, created.GetId(), true)
	entrance := grid.Square{Col: want.Entrance.X, Row: want.Entrance.Y}
	d.stand(created.GetId(), g, d.pens, entrance)
	scene, err := m.placeScene(d.campaign, created.GetId(), int32(want.Rooms[0].ID)) //nolint:gosec // G115: at most 500 rooms
	if err != nil {
		t.Fatal(err)
	}
	before := d.imageOf(created)
	walled := wallColors(before, p)

	// The player has been there: the memory is stored.
	seen := d.ana.mustVision(d.campaign, created.GetId())
	rows := func() int {
		var n int
		if err := d.h.pool.QueryRow(t.Context(), `SELECT count(*) FROM map_vision_memory WHERE map_id = $1`, created.GetId()).Scan(&n); err != nil {
			t.Fatal(err)
		}
		return n
	}
	memory := rows()
	if memory == 0 {
		t.Fatal("the player's memory of the map was not stored")
	}

	// The master's edits: a wall in the middle of a room, a wall cleared from the ring
	// of another (a hole), a secret door revealed.
	room := want.Rooms[1]
	wx, wy := room.Center()
	hole := grid.Square{Col: room.X - 1, Row: room.Y + 1} // the room's west wall (if it is not a door, it is a wall)
	var secretDoor, archway *dungeon.Door
	for i := range want.Doors {
		switch want.Doors[i].Kind {
		case dungeon.DoorSecret:
			if secretDoor == nil {
				secretDoor = &want.Doors[i]
			}
		case dungeon.DoorArchway:
			if archway == nil {
				archway = &want.Doors[i]
			}
		}
	}
	_, wantWalls := dungeon.WallsMask(want)
	if !wantWalls[hole.Row*41+hole.Col] {
		t.Skipf("the fixture's room %d has no wall at %v to clear", room.ID, hole)
	}
	m.mustPaint(d.campaign, created.GetId(), mapsv1.MapLayer_MAP_LAYER_WALL, 1, [2]int32{int32(wx), int32(wy)}) //nolint:gosec // G115: inside the grid
	m.mustPaint(d.campaign, created.GetId(), mapsv1.MapLayer_MAP_LAYER_WALL, 0, [2]int32{int32(hole.Col), int32(hole.Row)})
	m.mustPaint(d.campaign, created.GetId(), doors, int32(mapsv1.DoorState_DOOR_STATE_CLOSED), [2]int32{int32(secretDoor.X), int32(secretDoor.Y)}) //nolint:gosec // G115: inside the grid
	if !isWall(before, p, secretDoor.X, secretDoor.Y, walled) {
		t.Fatal("the secret door was not wall in the first image")
	}
	oldWalls, oldDoors, oldLayers := d.layersOf(created.GetId(), g)
	pointsBefore := m.mustGetMap(d.campaign, created.GetId())
	oldImage := created.GetImage().GetId()
	oldRevision := m.mustGetMap(d.campaign, created.GetId()).GetMap().GetRevision()

	res, err := m.redraw(d.campaign, created.GetId())
	if err != nil {
		t.Fatalf("RedrawDungeonMap() error = %v", err)
	}
	after := res.GetMap()
	if after.GetImage().GetId() == oldImage || after.GetImage().GetWidth() != 41*p || after.GetImage().GetHeight() != 31*p || after.GetRevision() != oldRevision+1 {
		t.Errorf("redrawn map = image %s (was %s), %d x %d, revision %d (was %d)", after.GetImage().GetId(), oldImage, after.GetImage().GetWidth(), after.GetImage().GetHeight(), after.GetRevision(), oldRevision)
	}
	if !after.GetFogEnabled() || after.GetRevealed() != true || after.GetGridColumns() != 41 || after.GetBaseLight() != mapsv1.LightLevel_LIGHT_LEVEL_BRIGHT {
		t.Errorf("the map's settings changed: %v", after)
	}

	// The image shows the edits.
	img := d.imageOf(after)
	if !isWall(img, p, wx, wy, walled) {
		t.Errorf("the wall painted at (%d, %d) is not wall in the new image", wx, wy)
	}
	floor := pixel(img, p, want.Rooms[0].X, want.Rooms[0].Y)
	if !sameColor(pixel(img, p, hole.Col, hole.Row), floor) {
		t.Errorf("the wall cleared at %v is not floor in the new image", hole)
	}
	if !sameColor(pixel(img, p, secretDoor.X, secretDoor.Y), floor) {
		t.Errorf("the revealed door at (%d, %d) is not floor in the new image", secretDoor.X, secretDoor.Y)
	}
	if archway != nil && !sameColor(pixel(img, p, archway.X, archway.Y), floor) {
		t.Error("a passage is not floor")
	}
	// Interior rock is still rock.
	rock := -1
	for i, k := range want.Kinds {
		col, row := i%41, i/41
		if k == dungeon.KindRock && !wantWalls[i] && col > 1 && row > 1 && col < 39 && row < 29 {
			rock = i
			break
		}
	}
	if rock >= 0 && !isWall(img, p, rock%41, rock/41, walled) {
		t.Errorf("rock square %d is not wall in the new image", rock)
	}

	// The layers, the points, the token, the revision and the players' memory are what they were.
	newWalls, newDoors, newLayers := d.layersOf(created.GetId(), g)
	if !bytes.Equal(newWalls.Encode(), oldWalls.Encode()) || !bytes.Equal(newDoors.Encode(), oldDoors.Encode()) || newLayers.GetLayersRevision() != oldLayers.GetLayersRevision() {
		t.Error("Redesenhar changed the layers or their revision")
	}
	if newWalls.Get(wx, wy) != true || newWalls.Get(hole.Col, hole.Row) != false || newDoors.Get(secretDoor.X, secretDoor.Y) != grid.DoorClosed {
		t.Error("the master's edits are not in the layers")
	}
	pointsAfter := m.mustGetMap(d.campaign, created.GetId())
	if len(pointsAfter.GetPoints()) != len(pointsBefore.GetPoints()) || len(pointsAfter.GetTokens()) != 1 ||
		!slices.ContainsFunc(pointsAfter.GetPoints(), func(pt *mapsv1.MapPoint) bool { return pt.GetId() == scene.GetPoint().GetId() }) {
		t.Errorf("points/tokens changed: %d points (was %d), %d tokens", len(pointsAfter.GetPoints()), len(pointsBefore.GetPoints()), len(pointsAfter.GetTokens()))
	}
	if rows() != memory {
		t.Errorf("the players' memory went from %d rows to %d", memory, rows())
	}
	if again := d.ana.mustVision(d.campaign, created.GetId()); !bytes.Equal(again.GetStates(), seen.GetStates()) && countRemembered(codes(t, again)) < countRemembered(codes(t, seen)) {
		t.Error("the player remembers less of the map after Redesenhar")
	}

	// The old image is gone from the gallery and from the blob store; the new one is the only one.
	imgs := m.list(d.campaign).GetImages()
	if len(imgs) != 1 || imgs[0].GetId() != after.GetImage().GetId() {
		t.Errorf("gallery after Redesenhar = %v, want only the new image", imgs)
	}
	for _, f := range d.h.storedFiles() {
		if strings.Contains(f, oldImage) {
			t.Errorf("the old image's file %s is still stored", f)
		}
	}
	// A second Redesenhar with no change gives the same pixels.
	res2, err := m.redraw(d.campaign, created.GetId())
	if err != nil {
		t.Fatalf("a second RedrawDungeonMap() error = %v", err)
	}
	if !equalImages(img, d.imageOf(res2.GetMap())) {
		t.Error("redrawing twice gave two different images")
	}
}

func countRemembered(c []byte) int {
	n := 0
	for _, v := range c {
		if v != 0 {
			n++
		}
	}
	return n
}

func equalImages(a, b image.Image) bool {
	if a.Bounds() != b.Bounds() {
		return false
	}
	for y := a.Bounds().Min.Y; y < a.Bounds().Max.Y; y++ {
		for x := a.Bounds().Min.X; x < a.Bounds().Max.X; x++ {
			if !sameColor(a.At(x, y), b.At(x, y)) {
				return false
			}
		}
	}
	return true
}

// A redraw never changes the size: when the map's image or grid no longer fits the
// dungeon (the master changed them, which cleared the layers), it is refused. And
// the keep-layers operation guards the size itself.
func TestMR010_RedrawNeverChangesTheSize(t *testing.T) {
	t.Parallel()
	d := newDungeonTable(t)
	m := d.master
	seed, _ := testDungeonSeed(t)
	created := m.createDungeon(d.campaign, "Masmorra de Vesna", testDungeonOptions(), seed).GetMap()

	// The keep-layers operation: another size is refused, the same size passes.
	other := m.mustUpload(d.campaign, "outra.png", pngImage(t, 400, 300))
	same := m.mustUpload(d.campaign, "mesma.png", pngImage(t, 41*24, 31*24))
	err := db.InTx(t.Context(), d.h.pool, func(tx pgx.Tx) error {
		q := d.h.svc.queries.WithTx(tx)
		locked, err := q.GetMapForUpdate(t.Context(), mapsdb.GetMapForUpdateParams{CampaignID: d.campaign, ID: created.GetId()})
		if err != nil {
			return err
		}
		old, err := q.GetGalleryImageInCampaign(t.Context(), mapsdb.GetGalleryImageInCampaignParams{CampaignID: d.campaign, ID: locked.ImageID})
		if err != nil {
			return err
		}
		bad, err := q.GetGalleryImageInCampaign(t.Context(), mapsdb.GetGalleryImageInCampaignParams{CampaignID: d.campaign, ID: other.GetId()})
		if err != nil {
			return err
		}
		if _, err := d.h.svc.replaceMapImageKeepingLayers(t.Context(), q, locked, old, bad); connect.CodeOf(err) != connect.CodeFailedPrecondition {
			t.Errorf("an image of another size: error = %v, want failed_precondition", err)
		}
		fine, err := q.GetGalleryImageInCampaign(t.Context(), mapsdb.GetGalleryImageInCampaignParams{CampaignID: d.campaign, ID: same.GetId()})
		if err != nil {
			return err
		}
		// The old image must be the map's own current one.
		if _, err := d.h.svc.replaceMapImageKeepingLayers(t.Context(), q, locked, bad, fine); connect.CodeOf(err) != connect.CodeFailedPrecondition {
			t.Errorf("an old image that is not the map's: error = %v, want failed_precondition", err)
		}
		if _, err := d.h.svc.replaceMapImageKeepingLayers(t.Context(), q, locked, old, fine); err != nil {
			t.Errorf("an image of the same size: error = %v", err)
		}
		return errors.New("roll back") // leave the map as it was
	})
	if err == nil || err.Error() != "roll back" {
		t.Fatalf("the transaction ended with %v", err)
	}

	// A new image through UpdateMap clears the layers and changes the size: no more redrawing.
	rev := m.mustGetMap(d.campaign, created.GetId()).GetMap().GetRevision()
	if _, err := m.maps.UpdateMap(t.Context(), connect.NewRequest(&mapsv1.UpdateMapRequest{CampaignId: d.campaign, MapId: created.GetId(), Revision: rev, ImageId: new(other.GetId())})); err != nil {
		t.Fatalf("UpdateMap(image) error = %v", err)
	}
	_, err = m.redraw(d.campaign, created.GetId())
	wantMapBlocked(t, "redraw after a new image", err, mapsv1.MapBlockedReason_MAP_BLOCKED_REASON_IMAGE_CHANGED)
	// The rooms list is the stored one, still there.
	if _, err := m.dungeonRooms(d.campaign, created.GetId()); err != nil {
		t.Errorf("the rooms list after a new image: %v", err)
	}
	// No grid: refused for that.
	m.mustSetGrid(d.campaign, created.GetId(), 0)
	_, err = m.redraw(d.campaign, created.GetId())
	wantMapBlocked(t, "redraw a map with no grid", err, mapsv1.MapBlockedReason_MAP_BLOCKED_REASON_NO_GRID)
}

// A failed creation leaves nothing: no map, no gallery image, no file. The map limit
// is checked before the dungeon is drawn and again inside the transaction.
func TestMR010_AFailedCreationLeavesNothingBehind(t *testing.T) {
	t.Parallel()
	d := newDungeonTable(t, func(c *Config) { c.MaxMaps = 1 })
	m := d.master
	seed, _ := testDungeonSeed(t)
	m.createDungeon(d.campaign, "Primeira", testDungeonOptions(), seed)
	files := d.h.storedFiles()
	_, err := m.tryCreateDungeon(d.campaign, "Segunda", testDungeonOptions(), &seed)
	wantCode(t, "a second map over the limit", err, connect.CodeResourceExhausted)
	if got := d.h.storedFiles(); !slices.Equal(got, files) {
		t.Errorf("files after the refusal = %v, want %v", got, files)
	}
	if len(m.listMaps(d.campaign)) != 1 || len(m.list(d.campaign).GetImages()) != 1 {
		t.Error("the refused creation left a map or an image")
	}
	var n int
	if err := d.h.pool.QueryRow(t.Context(), `SELECT count(*) FROM generated_dungeons`).Scan(&n); err != nil || n != 1 {
		t.Errorf("%d dungeon rows (%v), want 1", n, err)
	}

	// A full gallery refuses too, and cleans up.
	full := newDungeonTable(t, func(c *Config) { c.MaxImages = 1 })
	full.master.newImage(full.campaign)
	files = full.h.storedFiles()
	_, err = full.master.tryCreateDungeon(full.campaign, "Cheia", testDungeonOptions(), &seed)
	wantCode(t, "a full gallery", err, connect.CodeResourceExhausted)
	if got := full.h.storedFiles(); !slices.Equal(got, files) {
		t.Errorf("files after a full gallery = %v, want %v", got, files)
	}
}

// Creating and redrawing are rate limited per campaign: a burst, then a slow refill.
func TestMR010_CreatingAndRedrawingAreRateLimited(t *testing.T) {
	t.Parallel()
	h := newHarness(t)
	master := h.newUser("Mestre")
	campaign := h.newCampaign(master)
	opts := &mapsv1.DungeonOptions{Width: proto.Int32(21), Height: proto.Int32(21)}
	seed := uint64(7)
	var made *mapsv1.Map
	for i := range 4 {
		made = master.createDungeon(campaign, "Pequena "+strconv.Itoa(i), opts, seed).GetMap()
	}
	_, err := master.tryCreateDungeon(campaign, "Quinta", opts, &seed)
	wantCode(t, "the fifth creation in a burst", err, connect.CodeResourceExhausted)
	_, err = master.redraw(campaign, made.GetId())
	wantCode(t, "a redraw after the burst", err, connect.CodeResourceExhausted)
	// A preview is not limited.
	if _, err := master.previewDungeon(campaign, opts, &seed); err != nil {
		t.Errorf("a preview after the burst: %v", err)
	}
	// Another campaign has its own bucket.
	other := h.newUser("Outro mestre")
	otherCampaign := h.newCampaign(other)
	if _, err := other.tryCreateDungeon(otherCampaign, "Outra", opts, &seed); err != nil {
		t.Errorf("another campaign's creation: %v", err)
	}
}

// Timing of the whole creation (generate, render, encode, store, the rows), for the
// biggest dungeons the page offers and the API's largest. Run with
// MEURPG_MEASURE=1 and -v, without -race.
func TestDungeonMapCreationTiming(t *testing.T) {
	if os.Getenv("MEURPG_MEASURE") == "" {
		t.Skip("set MEURPG_MEASURE=1 to measure")
	}
	t.Parallel()
	d := newDungeonTable(t)
	m := d.master
	for _, size := range [][2]int32{{121, 121}, {199, 399}} {
		opts := &mapsv1.DungeonOptions{Width: new(size[0]), Height: new(size[1])}
		seed := uint64(11)
		start := time.Now()
		res := m.createDungeon(d.campaign, "Grande "+strconv.Itoa(int(size[0])), opts, seed)
		took := time.Since(start)
		img := res.GetMap().GetImage()
		stored, thumb := 0, 0
		for _, f := range d.h.storedFiles() {
			if strings.Contains(f, img.GetId()) {
				_, body := d.h.storedFile(f)
				if strings.HasSuffix(f, ".thumb") {
					thumb = len(body)
				} else {
					stored = len(body)
				}
			}
		}
		pre := time.Now()
		if _, err := m.previewDungeon(d.campaign, opts, &seed); err != nil {
			t.Fatal(err)
		}
		t.Logf("%d x %d: create %v (image %d x %d px, %d KiB, thumbnail %d KiB, %d rooms); preview %v",
			size[0], size[1], took.Round(time.Millisecond), img.GetWidth(), img.GetHeight(), stored/1024, thumb/1024, res.GetRoomCount(), time.Since(pre).Round(time.Millisecond))
		if img.GetWidth() > dungeonimg.MaxSide || img.GetHeight() > dungeonimg.MaxSide {
			t.Errorf("image of %d x %d pixels is over %d", img.GetWidth(), img.GetHeight(), dungeonimg.MaxSide)
		}
	}
}

// BenchmarkDungeonImage is the creation's own work with no database: the floor plan,
// the render, the PNG and its thumbnail, and the two files in the blob store.
// `go test -run '^$' -bench DungeonImage -benchmem ./internal/maps` (no -race).
func BenchmarkDungeonImage(b *testing.B) {
	blobs, err := blob.NewFS(b.TempDir())
	if err != nil {
		b.Fatal(err)
	}
	b.Cleanup(func() { _ = blobs.Close() })
	s := &Service{processing: make(chan struct{}, 1), blobs: blobs, logger: slog.New(slog.DiscardHandler)}
	for _, size := range [][2]int{{121, 121}, {199, 399}} {
		o := dungeon.DefaultOptions(11)
		o.Width, o.Height = size[0], size[1]
		d, err := dungeon.Generate(o)
		if err != nil {
			b.Fatal(err)
		}
		b.Run(fmt.Sprintf("%dx%d", size[0], size[1]), func(b *testing.B) {
			g := grid.Grid{Columns: d.Width, Rows: d.Height}
			for i := 0; b.Loop(); i++ {
				layers := dungeonLayers(d, g)
				solid := solidOf(d.Width, d.Height, layers.walls, layers.doors)
				res, err := s.drawDungeon(b.Context(), solid, d.Width, d.Height, 0, 0)
				if err != nil {
					b.Fatal(err)
				}
				if err := s.putImageFiles(b.Context(), "campanha", "imagem"+strconv.Itoa(i%2), res); err != nil {
					b.Fatal(err)
				}
				b.SetBytes(int64(len(res.Data)))
			}
		})
	}
}

// stand puts a character's token on the middle of a square of the dungeon's grid.
func (d *dungeonTable) stand(mapID string, g grid.Grid, characterID string, sq grid.Square) {
	d.h.t.Helper()
	x, y := g.CenterOf(sq)
	d.master.placeToken(d.campaign, mapID, characterID, int32(x), int32(y)) //nolint:gosec // G115: 0 to 10000
}

func TestCampaignSlotsAllowOneAtATime(t *testing.T) {
	t.Parallel()
	var c campaignSlots
	if !c.take("a") || c.take("a") || !c.take("b") {
		t.Fatal("a campaign takes one slot at a time, and others are free")
	}
	c.release("a")
	if !c.take("a") {
		t.Error("a released slot is free again")
	}
}

// A map that is no longer the dungeon's is never redrawn, and nothing the master put
// there is deleted: a same-size image they uploaded (which cleared the layers), or a
// grid taken off and put back (which cleared them too). The layers would be empty and
// every wall would turn to floor.
func TestMR010_RedrawRefusesAMapThatIsNoLongerTheDungeons(t *testing.T) {
	t.Parallel()
	d := newDungeonTable(t)
	m := d.master
	seed, _ := testDungeonSeed(t)
	const p = 24

	// 1. A same-size upload through UpdateMap.
	a := m.createDungeon(d.campaign, "Com upload", testDungeonOptions(), seed).GetMap()
	own := m.mustUpload(d.campaign, "meu.png", pngImage(t, 41*p, 31*p))
	if _, err := m.maps.UpdateMap(t.Context(), connect.NewRequest(&mapsv1.UpdateMapRequest{CampaignId: d.campaign, MapId: a.GetId(), Revision: a.GetRevision(), ImageId: new(own.GetId())})); err != nil {
		t.Fatalf("UpdateMap(image) error = %v", err)
	}
	_, err := m.redraw(d.campaign, a.GetId())
	wantMapBlocked(t, "redraw after a same-size upload", err, mapsv1.MapBlockedReason_MAP_BLOCKED_REASON_IMAGE_CHANGED)
	ids := map[string]bool{}
	for _, img := range m.list(d.campaign).GetImages() {
		ids[img.GetId()] = true
	}
	if !ids[own.GetId()] || !ids[a.GetImage().GetId()] {
		t.Errorf("the gallery lost an image: %v", ids)
	}
	if rooms, err := m.dungeonRooms(d.campaign, a.GetId()); err != nil || rooms.GetImageIsGenerated() {
		t.Errorf("rooms after an upload: %v, %v; want image_is_generated false", rooms.GetImageIsGenerated(), err)
	}

	// 2. A grid round trip: 41 columns, none, 41 again. The image is the generated one, the layers are gone.
	b := m.createDungeon(d.campaign, "Com grade", testDungeonOptions(), seed).GetMap()
	m.mustSetGrid(d.campaign, b.GetId(), 0)
	m.mustSetGrid(d.campaign, b.GetId(), 41)
	_, err = m.redraw(d.campaign, b.GetId())
	wantMapBlocked(t, "redraw after a grid round trip", err, mapsv1.MapBlockedReason_MAP_BLOCKED_REASON_IMAGE_CHANGED)
	if got := m.mustGetMap(d.campaign, b.GetId()).GetMap().GetImage().GetId(); got != b.GetImage().GetId() {
		t.Errorf("the map's image changed to %s", got)
	}
	if rooms, err := m.dungeonRooms(d.campaign, b.GetId()); err != nil || !rooms.GetImageIsGenerated() {
		t.Errorf("rooms after a grid round trip: %v, %v; want image_is_generated true", rooms.GetImageIsGenerated(), err)
	}
}

// "Redesenhar" deletes the old image unless something else uses it: another map, an
// image left with the players, the one the session shows, or an NPC's portrait.
func TestMR010_RedrawKeepsAnOldImageSomethingElseUses(t *testing.T) {
	t.Parallel()
	d := newDungeonTable(t)
	m := d.master
	seed, _ := testDungeonSeed(t)
	m.start(d.campaign)
	exec := func(sql string, args ...any) {
		t.Helper()
		if _, err := d.h.pool.Exec(t.Context(), sql, args...); err != nil {
			t.Fatalf("%s: %v", sql, err)
		}
	}
	for name, use := range map[string]func(oldImage string){
		"another map": func(img string) {
			other := m.createMap(d.campaign, "Outro", m.newImage(d.campaign))
			exec(`UPDATE maps SET image_id = $1 WHERE id = $2`, img, other.GetId())
		},
		"left with the players": func(img string) {
			exec(`INSERT INTO campaign_left_images (campaign_id, image_id, left_at) VALUES ($1, $2, now())`, d.campaign, img)
		},
		"shown in the session": func(img string) {
			exec(`UPDATE game_sessions SET shown_image_id = $1 WHERE campaign_id = $2`, img, d.campaign)
		},
		"a portrait": func(img string) {
			npc := m.createNPC(d.campaign, "Vesna", "")
			exec(`UPDATE characters SET sheet = jsonb_set(sheet, '{basic,portrait_image_id}', to_jsonb($1::text)) WHERE id = $2`, img, npc.GetId())
		},
	} {
		created := m.createDungeon(d.campaign, "Masmorra "+name, testDungeonOptions(), seed).GetMap()
		oldImage := created.GetImage().GetId()
		use(oldImage)
		res, err := m.redraw(d.campaign, created.GetId())
		if err != nil {
			t.Fatalf("%s: RedrawDungeonMap() error = %v", name, err)
		}
		if res.GetMap().GetImage().GetId() == oldImage {
			t.Errorf("%s: the map kept its old image", name)
		}
		kept := false
		for _, img := range m.list(d.campaign).GetImages() {
			kept = kept || img.GetId() == oldImage
		}
		if !kept {
			t.Errorf("%s: the old image was deleted from the gallery", name)
		}
		if rooms, err := m.dungeonRooms(d.campaign, created.GetId()); err != nil || !rooms.GetImageIsGenerated() {
			t.Errorf("%s: rooms after the redraw: %v, %v", name, rooms.GetImageIsGenerated(), err)
		}
	}
}

// The window between a redraw's render and its transaction: a door a character opens
// there (the layers' revision goes up, the image does not change) must not abort it,
// and a wall painted there, which the new image would not show, must.
func TestMR010_RedrawToleratesADoorOpenedWhileItDraws(t *testing.T) {
	t.Parallel()
	d := newDungeonTable(t)
	m := d.master
	seed, want := testDungeonSeed(t)
	created := m.createDungeon(d.campaign, "Masmorra de Vesna", testDungeonOptions(), seed).GetMap()
	var closed dungeon.Door
	for _, dr := range want.Doors {
		if dr.Kind == dungeon.DoorClosed {
			closed = dr
			break
		}
	}
	revision := func() int32 { return m.mustGetMap(d.campaign, created.GetId()).GetMap().GetLayersRevision() }
	during := func(change func()) {
		d.h.svc.beforeRedrawTx = func() { d.h.svc.beforeRedrawTx = nil; change() }
	}

	before := revision()
	during(func() {
		m.mustPaint(d.campaign, created.GetId(), doors, int32(mapsv1.DoorState_DOOR_STATE_OPEN), [2]int32{int32(closed.X), int32(closed.Y)}) //nolint:gosec // G115: inside the grid
	})
	if _, err := m.redraw(d.campaign, created.GetId()); err != nil {
		t.Fatalf("a redraw with a door opened meanwhile: %v", err)
	}
	if revision() <= before {
		t.Error("the door did not bump the layers' revision, so the test proves nothing")
	}

	room := want.Rooms[0]
	cx, cy := room.Center()
	during(func() {
		m.mustPaint(d.campaign, created.GetId(), mapsv1.MapLayer_MAP_LAYER_WALL, 1, [2]int32{int32(cx), int32(cy)}) //nolint:gosec // G115: inside the grid
	})
	_, err := m.redraw(d.campaign, created.GetId())
	wantCode(t, "a redraw with a wall painted meanwhile", err, connect.CodeAborted)
	if _, err := m.redraw(d.campaign, created.GetId()); err != nil {
		t.Errorf("the redraw after trying again: %v", err)
	}
}

// The window between a redraw's reads and its transaction: the master shows the old
// image to the players there. The redraw must keep the image, as it does when the image
// was already shown, instead of deleting it from under the screen.
func TestMR010_RedrawKeepsAnOldImageShownWhileItDraws(t *testing.T) {
	t.Parallel()
	d := newDungeonTable(t)
	m := d.master
	seed, _ := testDungeonSeed(t)
	m.start(d.campaign)
	created := m.createDungeon(d.campaign, "Masmorra de Vesna", testDungeonOptions(), seed).GetMap()
	oldImage := created.GetImage().GetId()
	d.h.svc.beforeRedrawTx = func() {
		d.h.svc.beforeRedrawTx = nil
		if _, err := d.h.pool.Exec(t.Context(), `UPDATE game_sessions SET shown_image_id = $1 WHERE campaign_id = $2`, oldImage, d.campaign); err != nil {
			t.Errorf("show the image: %v", err)
		}
	}
	if _, err := m.redraw(d.campaign, created.GetId()); err != nil {
		t.Fatalf("RedrawDungeonMap() error = %v", err)
	}
	var shown *string
	if err := d.h.pool.QueryRow(t.Context(), `SELECT shown_image_id::text FROM game_sessions WHERE campaign_id = $1`, d.campaign).Scan(&shown); err != nil {
		t.Fatal(err)
	}
	if shown == nil || *shown != oldImage {
		t.Errorf("the screen shows %v after the redraw, want the old image %s kept", shown, oldImage)
	}
}

// A calibration that keeps the grid's squares but changes the factor (13 drawn squares
// of 3 for 39 of 1), done while the redraw draws, leaves a map that is no longer drawn
// the dungeon's way (RN-25): the redraw is refused, as when the factor was already not 1.
func TestMR010_RedrawRefusesAFactorChangedWhileItDraws(t *testing.T) {
	t.Parallel()
	d := newDungeonTable(t)
	m := d.master
	opts := testDungeonOptions()
	opts.Width, opts.Height = proto.Int32(39), proto.Int32(27)
	seed, _ := testDungeonSeed(t)
	created := m.createDungeon(d.campaign, "Masmorra de lado múltiplo", opts, seed).GetMap()
	d.h.svc.beforeRedrawTx = func() {
		d.h.svc.beforeRedrawTx = nil
		if _, err := m.setCalibration(d.campaign, created.GetId(), 13, 3); err != nil {
			t.Errorf("calibrate: %v", err)
		}
	}
	_, err := m.redraw(d.campaign, created.GetId())
	wantMapBlocked(t, "a redraw with the factor changed meanwhile", err, mapsv1.MapBlockedReason_MAP_BLOCKED_REASON_IMAGE_CHANGED)
}

// The walls layer of a generated dungeon covers every square that is not open (the walls
// and all the rock behind them), so one wall mark covers the mass and no token can be put on
// rock; the image's plan is solid on exactly those squares, and nowhere else but a secret door.
func TestMR010_TheWallsLayerCoversAllRock(t *testing.T) {
	t.Parallel()
	_, d := testDungeonSeed(t)
	g := grid.Grid{Columns: d.Width, Rows: d.Height}
	layers := dungeonLayers(d, g)
	open, _ := dungeon.WallsMask(d)
	rock := 0
	for i, o := range open {
		col, row := i%d.Width, i/d.Width
		if layers.walls.Get(col, row) == o {
			t.Fatalf("square (%d, %d): wall = %v, open = %v: the layer must be every square that is not open", col, row, !o, o)
		}
		if !o {
			rock++
		}
	}
	solid := solidOf(d.Width, d.Height, layers.walls, layers.doors)
	for i := range solid {
		col, row := i%d.Width, i/d.Width
		if want := layers.walls.Get(col, row) || layers.doors.Get(col, row) == grid.DoorSecret; solid[i] != want {
			t.Fatalf("plan (%d, %d) = %v, want %v", col, row, solid[i], want)
		}
	}
	if rock == 0 {
		t.Fatal("the fixture has no rock")
	}
}

// A request without the options message takes every default, like an empty
// message, and never panics (CreateDungeonMap and PreviewDungeon both read it).
func TestDungeonOptionsFromProtoWithoutOptionsIsTheDefault(t *testing.T) {
	got, err := dungeonOptionsFromProto(nil, 7)
	if err != nil {
		t.Fatalf("dungeonOptionsFromProto(nil) error = %v", err)
	}
	empty, err := dungeonOptionsFromProto(&mapsv1.DungeonOptions{}, 7)
	if err != nil {
		t.Fatalf("dungeonOptionsFromProto(empty) error = %v", err)
	}
	if got != empty || got != dungeon.DefaultOptions(7) {
		t.Errorf("nil options = %+v, want the defaults %+v", got, dungeon.DefaultOptions(7))
	}
}

// A campaign at its map limit or with a full gallery is refused before the
// dungeon is drawn: the test holds the server's one processing slot, so a
// refusal decided before the render answers at once, and one decided after it
// would block waiting for the slot.
func TestDungeonLimitsAreCheckedBeforeRendering(t *testing.T) {
	t.Parallel()
	cases := map[string]func(*Config){
		"map cap":       func(c *Config) { c.MaxMaps = 1 },
		"gallery quota": func(c *Config) { c.MaxImages = 1 },
	}
	for name, configure := range cases {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			d := newDungeonTable(t, configure)
			seed, _ := testDungeonSeed(t)
			if name == "map cap" {
				d.master.createDungeon(d.campaign, "Primeira", testDungeonOptions(), seed)
			} else {
				d.master.newImage(d.campaign)
			}
			// Another request holds the server's one image-processing slot.
			d.h.svc.processing <- struct{}{}
			released := false
			release := func() {
				if !released {
					released = true
					<-d.h.svc.processing
				}
			}
			defer release()

			type result struct{ err error }
			done := make(chan result, 1)
			go func() {
				_, err := d.master.tryCreateDungeon(d.campaign, "Segunda", testDungeonOptions(), &seed)
				done <- result{err}
			}()
			select {
			case r := <-done:
				wantCode(t, "over the limit", r.err, connect.CodeResourceExhausted)
				// The refused try did not spend a token of the dungeon limit.
				if ok, _ := d.h.svc.dungeonLimit.Allow(d.campaign); !ok {
					t.Error("the refused creation spent a token of the dungeon limit")
				}
			case <-time.After(2 * time.Second):
				release()
				r := <-done
				t.Errorf("the refusal (%v) waited for the processing slot: the dungeon is rendered before the %s is checked", r.err, name)
			}
		})
	}
}

// A redraw whose old image goes away with it does not grow the gallery, so a full
// gallery allows it; the control is the same gallery with the old image kept (left with
// the players), where the redraw would add an image and is refused.
func TestRedrawInAFullGalleryIsAllowedWhenTheOldImageGoes(t *testing.T) {
	t.Parallel()
	d := newDungeonTable(t, func(c *Config) { c.MaxImages = 2 })
	seed, _ := testDungeonSeed(t)
	made := d.master.createDungeon(d.campaign, "Primeira", testDungeonOptions(), seed)
	d.master.newImage(d.campaign) // the dungeon's image and this one fill the gallery
	old := made.GetMap().GetImage().GetId()
	d.master.start(d.campaign)
	if _, err := d.h.pool.Exec(t.Context(), `INSERT INTO campaign_left_images (campaign_id, image_id, left_at) VALUES ($1, $2, now())`, d.campaign, old); err != nil {
		t.Fatal(err)
	}
	_, err := d.master.redraw(d.campaign, made.GetMap().GetId())
	wantCode(t, "a redraw that would grow a full gallery", err, connect.CodeResourceExhausted)

	if _, err := d.h.pool.Exec(t.Context(), `DELETE FROM campaign_left_images WHERE campaign_id = $1`, d.campaign); err != nil {
		t.Fatal(err)
	}
	res, err := d.master.redraw(d.campaign, made.GetMap().GetId())
	if err != nil {
		t.Fatalf("a redraw whose old image goes away, in a full gallery: %v", err)
	}
	if res.GetMap().GetImage().GetId() == old {
		t.Error("the map kept its old image")
	}
	if n := len(d.master.list(d.campaign).GetImages()); n != 2 {
		t.Errorf("the gallery has %d images after the redraw, want 2", n)
	}
}

// A redraw that would grow a full gallery is refused before the image is drawn, too.
func TestRedrawIsRefusedBeforeRenderingWhenTheGalleryIsFull(t *testing.T) {
	t.Parallel()
	d := newDungeonTable(t, func(c *Config) { c.MaxImages = 2 })
	seed, _ := testDungeonSeed(t)
	made := d.master.createDungeon(d.campaign, "Primeira", testDungeonOptions(), seed)
	d.master.newImage(d.campaign) // the dungeon's image and this one fill the gallery
	d.master.start(d.campaign)
	// The old image stays (left with the players), so the redraw would add an image.
	if _, err := d.h.pool.Exec(t.Context(), `INSERT INTO campaign_left_images (campaign_id, image_id, left_at) VALUES ($1, $2, now())`, d.campaign, made.GetMap().GetImage().GetId()); err != nil {
		t.Fatal(err)
	}

	d.h.svc.processing <- struct{}{}
	released := false
	release := func() {
		if !released {
			released = true
			<-d.h.svc.processing
		}
	}
	defer release()

	done := make(chan error, 1)
	go func() {
		_, err := d.master.redraw(d.campaign, made.GetMap().GetId())
		done <- err
	}()
	select {
	case err := <-done:
		wantCode(t, "redraw with a full gallery", err, connect.CodeResourceExhausted)
	case <-time.After(2 * time.Second):
		release()
		err := <-done
		t.Errorf("the refusal (%v) waited for the processing slot: the dungeon is drawn before the gallery is checked", err)
	}
}
