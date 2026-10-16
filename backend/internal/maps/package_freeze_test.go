package maps

import (
	"archive/zip"
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"strings"
	"testing"
	"time"

	"google.golang.org/protobuf/encoding/protojson"
	"google.golang.org/protobuf/proto"

	"github.com/jackc/pgx/v5"

	pkgv1 "github.com/PuraFome/meuRPG/backend/gen/meurpg/campaignpackage/v1"
	mapsv1 "github.com/PuraFome/meuRPG/backend/gen/meurpg/maps/v1"
	"github.com/PuraFome/meuRPG/backend/internal/campaignpackage"
	"github.com/PuraFome/meuRPG/backend/internal/platform/db"
	"github.com/PuraFome/meuRPG/backend/internal/rules/grid"
)

// Characterization of the maps module's share of the campaign package (MR-050):
// how a package's points, maps and images are checked before they are written
// (every problem with its kind, name and reason, in the order the checks run)
// and what is staged from the good ones. Written on the code before it was split
// into phases; the goldens in testdata must not change in the refactor.

// Fixed UUIDs of the package's own ids.
const (
	pkgMapA    = "11111111-1111-4111-8111-111111111111"
	pkgMapB    = "22222222-2222-4222-8222-222222222222"
	pkgMapC    = "77777777-7777-4777-8777-777777777777"
	pkgMapGone = "33333333-3333-4333-8333-333333333333"
	pkgPoint1  = "44444444-4444-4444-8444-444444444441"
	pkgPoint2  = "44444444-4444-4444-8444-444444444442"
	pkgClue1   = "55555555-5555-4555-8555-555555555551"
	pkgClue2   = "55555555-5555-4555-8555-555555555552"
	pkgImage1  = "66666666-6666-4666-8666-666666666661"
	pkgImage2  = "66666666-6666-4666-8666-666666666662"
	pkgImage3  = "66666666-6666-4666-8666-666666666663"
)

// problemsOf renders what an import found wrong, one line each, and the
// references that never got defined.
func problemsOf(in *campaignpackage.Import) string {
	in.IDs.Unresolved(&in.Problems)
	var lines []string
	for _, p := range in.Problems.List {
		lines = append(lines, fmt.Sprintf("problem kind=%s name=%q reason=%s limit=%d", p.GetKind(), p.GetName(), p.GetReason(), p.GetLimit()))
	}
	if len(lines) == 0 {
		return "no problems"
	}
	return strings.Join(lines, "\n")
}

func orNil[T any](p *T) string {
	if p == nil {
		return "nil"
	}
	return fmt.Sprint(*p)
}

// renderStagedPoint renders what a staged point holds.
func renderStagedPoint(pt *stagedPoint) string {
	if pt == nil {
		return "point nil"
	}
	var b strings.Builder
	fmt.Fprintf(&b, "point kind=%s name=%q desc=%q hooks=%q showDC=%v x=%d y=%d revealed=%v createdAt=%d target=%v stairs=%s",
		pt.kind, pt.name, pt.desc, pt.hooks, pt.showDC, pt.x, pt.y, pt.revealed, pt.createdAt.UnixMicro()%1000, pt.target != nil, orNil(pt.stairs))
	fmt.Fprintf(&b, " trap=%v trapState=%s value=%s light=%v/%s/%d/%d", pt.spec.trap != nil, orNil(pt.spec.trapSt), orNil(pt.spec.value),
		pt.spec.hasLight, orNil(pt.spec.light.preset), pt.spec.light.bright, pt.spec.light.dimmed)
	for _, a := range pt.actions {
		fmt.Fprintf(&b, "\n  action key=%s name=%q dc=%s attempts=%d", a.key, a.name, orNil(a.dc), a.maxAttempts)
	}
	for _, c := range pt.clues {
		fmt.Fprintf(&b, "\n  clue text=%q", c.text)
	}
	return b.String()
}

func newStageImport() *campaignpackage.Import {
	return campaignpackage.NewImport(nil, "campaign-new", "user-new", false, nil)
}

// pointCase is a point of a package and the staging it gets.
type pointCase struct {
	name string
	edit func(pp *pkgv1.PackagePoint)
	// prepare runs on the import before the point is staged.
	prepare func(in *campaignpackage.Import)
}

func goodPoint() *pkgv1.PackagePoint {
	return &pkgv1.PackagePoint{
		Id: pkgPoint1, Kind: mapsv1.MapPointKind_MAP_POINT_KIND_SCENE, Name: "  A ponte  ", Description: "Uma ponte\nde corda.", XBp: 100, YBp: 200, Revealed: true,
	}
}

func stagePointGolden(t *testing.T, part *mapsPart) string {
	t.Helper()
	kind := func(k mapsv1.MapPointKind) func(pp *pkgv1.PackagePoint) {
		return func(pp *pkgv1.PackagePoint) { pp.Kind = k }
	}
	submap, battle, treasure, trapKind, light := mapsv1.MapPointKind_MAP_POINT_KIND_SUBMAP, mapsv1.MapPointKind_MAP_POINT_KIND_BATTLE,
		mapsv1.MapPointKind_MAP_POINT_KIND_TREASURE, mapsv1.MapPointKind_MAP_POINT_KIND_TRAP, mapsv1.MapPointKind_MAP_POINT_KIND_LIGHT
	with := func(k mapsv1.MapPointKind, more func(pp *pkgv1.PackagePoint)) func(pp *pkgv1.PackagePoint) {
		return func(pp *pkgv1.PackagePoint) { pp.Kind = k; more(pp) }
	}
	manyActions := func(n int) []*pkgv1.PackageAction {
		out := make([]*pkgv1.PackageAction, n)
		for i := range out {
			out[i] = &pkgv1.PackageAction{Key: "skill:arcana"}
		}
		return out
	}
	cases := []pointCase{
		{name: "a good scene"},
		{name: "a scene with actions and clues", edit: func(pp *pkgv1.PackagePoint) {
			pp.Hooks = "  ganchos  "
			pp.ShowDc = true
			pp.Actions = []*pkgv1.PackageAction{
				{Key: "skill:investigation", Name: " Procurar ", Dc: 15, MaxAttempts: 2},
				{Key: "save:wis"},
			}
			pp.Clues = []*pkgv1.PackageClue{{Id: pkgClue1, Text: " pista um "}, {Id: pkgClue2, Text: "pista dois"}}
		}},
		{name: "an empty name", edit: func(pp *pkgv1.PackagePoint) { pp.Name = "  " }},
		{name: "a name of 81 characters", edit: func(pp *pkgv1.PackagePoint) { pp.Name = strings.Repeat("a", 81) }},
		{name: "a name with a line break", edit: func(pp *pkgv1.PackagePoint) { pp.Name = "a\nb" }},
		{name: "a kind that does not exist", edit: kind(mapsv1.MapPointKind_MAP_POINT_KIND_UNSPECIFIED)},
		{name: "an id that is not a UUID", edit: func(pp *pkgv1.PackagePoint) { pp.Id = "x" }},
		{name: "an id used twice", prepare: func(in *campaignpackage.Import) { in.IDs.Define(nsPoint, pkgPoint1) }},
		{name: "a description of 2001 characters", edit: func(pp *pkgv1.PackagePoint) { pp.Description = strings.Repeat("d", 2001) }},
		{name: "hooks of 4001 characters", edit: func(pp *pkgv1.PackagePoint) { pp.Hooks = strings.Repeat("h", 4001) }},
		{name: "a position of 10001", edit: func(pp *pkgv1.PackagePoint) { pp.XBp = 10001 }},
		{name: "a negative position", edit: func(pp *pkgv1.PackagePoint) { pp.YBp = -1 }},
		{name: "hooks on a battle point", edit: with(battle, func(pp *pkgv1.PackagePoint) { pp.Hooks = "x" })},
		{name: "a DC shown by a battle point", edit: with(battle, func(pp *pkgv1.PackagePoint) { pp.ShowDc = true })},
		{name: "a target on a scene", edit: func(pp *pkgv1.PackagePoint) { pp.TargetMapId = pkgMapB }},
		{name: "a submap to its own map", edit: with(submap, func(pp *pkgv1.PackagePoint) { pp.TargetMapId = pkgMapA })},
		{name: "a submap to another map", edit: with(submap, func(pp *pkgv1.PackagePoint) { pp.TargetMapId = pkgMapB })},
		{
			name: "a submap to a map the package defines", edit: with(submap, func(pp *pkgv1.PackagePoint) { pp.TargetMapId = pkgMapB }),
			prepare: func(in *campaignpackage.Import) { in.IDs.Define(nsMap, pkgMapB) },
		},
		{name: "a battle to another map", edit: with(battle, func(pp *pkgv1.PackagePoint) { pp.TargetMapId = pkgMapGone })},
		{name: "a submap with stairs up", edit: with(submap, func(pp *pkgv1.PackagePoint) { pp.Stairs = "up" })},
		{name: "a submap with stairs down", edit: with(submap, func(pp *pkgv1.PackagePoint) { pp.Stairs = "down" })},
		{name: "a submap with stairs sideways", edit: with(submap, func(pp *pkgv1.PackagePoint) { pp.Stairs = "sideways" })},
		{name: "stairs on a scene", edit: func(pp *pkgv1.PackagePoint) { pp.Stairs = "up" }},
		{name: "a trap point", edit: with(trapKind, func(pp *pkgv1.PackagePoint) { pp.Trap = testTrap() })},
		{name: "a trap point left triggered", edit: with(trapKind, func(pp *pkgv1.PackagePoint) {
			pp.Trap = testTrap()
			pp.Trap.State = mapsv1.TrapState_TRAP_STATE_TRIGGERED
		})},
		{name: "a trap point with no trap", edit: kind(trapKind)},
		{name: "a trap with an unknown damage type", edit: with(trapKind, func(pp *pkgv1.PackagePoint) {
			pp.Trap = testTrap()
			pp.Trap.Effect.Damage[0].DamageTypeKey = "damage-type:nope"
		})},
		{name: "a trap with an unknown condition", edit: with(trapKind, func(pp *pkgv1.PackagePoint) {
			pp.Trap = testTrap()
			pp.Trap.Effect.Conditions[0].ConditionKey = "condition:nope"
		})},
		{name: "a trap with a DC of 99", edit: with(trapKind, func(pp *pkgv1.PackagePoint) {
			pp.Trap = testTrap()
			pp.Trap.NoticeDc = 99
		})},
		{name: "a trap on a scene", edit: func(pp *pkgv1.PackagePoint) { pp.Trap = testTrap() }},
		{name: "a treasure of 250", edit: with(treasure, func(pp *pkgv1.PackagePoint) { pp.TreasureValuePo = proto.Int32(250) })},
		{name: "a treasure with no value", edit: kind(treasure)},
		{name: "a treasure of a negative value", edit: with(treasure, func(pp *pkgv1.PackagePoint) { pp.TreasureValuePo = proto.Int32(-5) })},
		{name: "a treasure value on a scene", edit: func(pp *pkgv1.PackagePoint) { pp.TreasureValuePo = proto.Int32(5) }},
		{name: "a torch", edit: with(light, func(pp *pkgv1.PackagePoint) { pp.Light = &mapsv1.LightSpec{PresetKey: "light:torch"} })},
		{name: "a light of its own radii", edit: with(light, func(pp *pkgv1.PackagePoint) { pp.Light = &mapsv1.LightSpec{BrightFt: 10, DimFt: 20} })},
		{name: "a light of an unknown preset", edit: with(light, func(pp *pkgv1.PackagePoint) { pp.Light = &mapsv1.LightSpec{PresetKey: "light:nope"} })},
		{name: "a light with no spec", edit: kind(light)},
		{name: "a light on a scene", edit: func(pp *pkgv1.PackagePoint) { pp.Light = &mapsv1.LightSpec{PresetKey: "light:torch"} }},
		{name: "actions on a battle point", edit: with(battle, func(pp *pkgv1.PackagePoint) { pp.Actions = manyActions(1) })},
		{name: "clues on a battle point", edit: with(battle, func(pp *pkgv1.PackagePoint) { pp.Clues = []*pkgv1.PackageClue{{Id: pkgClue1, Text: "x"}} })},
		{name: "21 actions", edit: func(pp *pkgv1.PackagePoint) { pp.Actions = manyActions(21) }},
		{name: "20 actions", edit: func(pp *pkgv1.PackagePoint) { pp.Actions = manyActions(20) }},
		{name: "31 clues", edit: func(pp *pkgv1.PackagePoint) {
			for range 31 {
				pp.Clues = append(pp.Clues, &pkgv1.PackageClue{Id: newKey(), Text: "x"})
			}
		}},
		{name: "an action of an unknown check", edit: func(pp *pkgv1.PackagePoint) { pp.Actions = []*pkgv1.PackageAction{{Key: "attack:sword"}} }},
		{name: "an action with a DC of 31", edit: func(pp *pkgv1.PackagePoint) { pp.Actions = []*pkgv1.PackageAction{{Key: "skill:arcana", Dc: 31}} }},
		{name: "an action with a name of 61", edit: func(pp *pkgv1.PackagePoint) {
			pp.Actions = []*pkgv1.PackageAction{{Key: "skill:arcana", Name: strings.Repeat("n", 61)}}
		}},
		{name: "an action with 6 attempts", edit: func(pp *pkgv1.PackagePoint) {
			pp.Actions = []*pkgv1.PackageAction{{Key: "skill:arcana", MaxAttempts: 6}}
		}},
		{name: "a clue of an empty text", edit: func(pp *pkgv1.PackagePoint) { pp.Clues = []*pkgv1.PackageClue{{Id: pkgClue1, Text: " "}} }},
		{name: "a clue of 501 characters", edit: func(pp *pkgv1.PackagePoint) {
			pp.Clues = []*pkgv1.PackageClue{{Id: pkgClue1, Text: strings.Repeat("c", 501)}}
		}},
		{name: "a clue with an id that is not a UUID", edit: func(pp *pkgv1.PackagePoint) { pp.Clues = []*pkgv1.PackageClue{{Id: "x", Text: "x"}} }},
		{name: "a clue id twice", edit: func(pp *pkgv1.PackagePoint) {
			pp.Clues = []*pkgv1.PackageClue{{Id: pkgClue1, Text: "a"}, {Id: pkgClue1, Text: "b"}}
		}},
		{name: "a battle point", edit: kind(battle)},
	}
	var out strings.Builder
	for _, c := range cases {
		in := newStageImport()
		if c.prepare != nil {
			c.prepare(in)
		}
		pp := goodPoint()
		if c.edit != nil {
			c.edit(pp)
		}
		pm := &pkgv1.PackageMap{Id: pkgMapA}
		sm := &stagedMap{id: "new-map-id"}
		pt := part.stagePoint(in, pm, sm, pp, time.UnixMicro(1_700_000_000_123_456))
		fmt.Fprintf(&out, "== %s\n%s\n%s\ncounts scenes=%d battles=%d treasures=%d\n\n", c.name, renderStagedPoint(pt), problemsOf(in), in.Counts.Scenes, in.Counts.BattlePoints, in.Counts.TreasurePoints)
	}
	return out.String()
}

// renderStagedMap renders what a staged map holds (the new ids only as set or not).
func renderStagedMap(in *campaignpackage.Import, sm *stagedMap) string {
	if sm == nil {
		return "map nil"
	}
	var b strings.Builder
	imageID, imageDefined := in.IDs.Get(nsImage, pkgImage1)
	fmt.Fprintf(&b, "map oldID=%s idSet=%v name=%q image=%v columns=%s factor=%d revealed=%v fog=%v onGrid=%v group=%v light=%s grid=%v doors=%v createdAt=%d",
		sm.oldID, sm.id != "", sm.name, imageDefined && sm.imageID == imageID, orNil(sm.columns), sm.factor, sm.revealed, sm.fog, sm.onGrid, sm.group, sm.baseLight,
		sm.grid, sm.doors != nil, sm.createdAt.UnixMicro()%1000)
	for i, l := range sm.layers {
		fmt.Fprintf(&b, " layer%d=%d", i, len(l))
	}
	for _, pt := range sm.points {
		fmt.Fprintf(&b, "\n  %s", renderStagedPoint(pt))
	}
	return b.String()
}

// layerBytes are the five painted layers of g with one square set in each.
func layerBytes(g grid.Grid) (terrain, walls, cover, light, doors []byte) {
	l := grid.NewLayer(g)
	l.Set(1, 1, true)
	c := grid.NewCoverLayer(g)
	li := grid.NewLightLayer(g)
	d := grid.NewDoorLayer(g)
	d.Set(2, 2, grid.DoorClosed)
	return l.Encode(), l.Encode(), c.Encode(), li.Encode(), d.Encode()
}

func stageMapGolden(t *testing.T, part *mapsPart) string {
	t.Helper()
	imgs := &stagedImages{byOld: map[string]*stagedImage{pkgImage1: {width: 400, height: 300}}}
	g := grid.Grid{Columns: 20, Rows: 15}
	terrain, walls, cover, light, doors := layerBytes(g)
	goodMap := func() *pkgv1.PackageMap {
		return &pkgv1.PackageMap{
			Id: pkgMapA, Name: " Vila ", ImageId: pkgImage1, GridColumns: 20, GridFactor: 1, Revealed: true, FogEnabled: true, FogOnFirstGrid: true,
			GroupVision: true, BaseLight: "dim",
			Layers: &pkgv1.PackageLayers{DifficultTerrain: terrain, Walls: walls, Cover: cover, Light: light, Doors: doors},
			Points: []*pkgv1.PackagePoint{goodPoint(), {Id: pkgPoint2, Kind: mapsv1.MapPointKind_MAP_POINT_KIND_BATTLE, Name: "Emboscada", XBp: 5, YBp: 5}},
		}
	}
	noGrid := func(pm *pkgv1.PackageMap) {
		pm.GridColumns, pm.GridFactor, pm.FogEnabled, pm.Layers = 0, 1, false, nil
	}
	cases := []struct {
		name    string
		edit    func(pm *pkgv1.PackageMap)
		noImage bool // the package's image is not staged
		prepare func(in *campaignpackage.Import)
	}{
		{name: "a good map"},
		{name: "a good map whose image was refused", noImage: true},
		{name: "a good map with a wide layers set", edit: func(pm *pkgv1.PackageMap) { pm.Layers.Doors = nil }},
		{name: "a map with no grid", edit: noGrid},
		{name: "a map with no layers", edit: func(pm *pkgv1.PackageMap) { pm.Layers = nil }},
		{name: "an empty name", edit: func(pm *pkgv1.PackageMap) { pm.Name = " " }},
		{name: "a name of 81 characters", edit: func(pm *pkgv1.PackageMap) { pm.Name = strings.Repeat("m", 81) }},
		{name: "an id that is not a UUID", edit: func(pm *pkgv1.PackageMap) { pm.Id = "x" }},
		{name: "an id used twice", prepare: func(in *campaignpackage.Import) { in.IDs.Define(nsMap, pkgMapA) }},
		{name: "a light of pitch", edit: func(pm *pkgv1.PackageMap) { pm.BaseLight = "pitch" }},
		{name: "no light", edit: func(pm *pkgv1.PackageMap) { pm.BaseLight = "" }},
		{name: "bright light", edit: func(pm *pkgv1.PackageMap) { pm.BaseLight = "bright" }},
		{name: "dark light", edit: func(pm *pkgv1.PackageMap) { pm.BaseLight = "dark" }},
		{name: "no grid with a scale", edit: func(pm *pkgv1.PackageMap) { noGrid(pm); pm.GridFactor = 2 }},
		{name: "no grid with a factor of zero", edit: func(pm *pkgv1.PackageMap) { noGrid(pm); pm.GridFactor = 0 }},
		{name: "no grid with fog", edit: func(pm *pkgv1.PackageMap) { noGrid(pm); pm.FogEnabled = true }},
		{name: "no grid with a wall", edit: func(pm *pkgv1.PackageMap) { noGrid(pm); pm.Layers = &pkgv1.PackageLayers{Walls: walls} }},
		{name: "no grid with empty layers", edit: func(pm *pkgv1.PackageMap) { noGrid(pm); pm.Layers = &pkgv1.PackageLayers{} }},
		{name: "a factor of zero", edit: func(pm *pkgv1.PackageMap) { pm.GridFactor = 0 }},
		{name: "a factor over the most", edit: func(pm *pkgv1.PackageMap) { pm.GridFactor = grid.MaxFactor + 1 }},
		{name: "columns that the factor does not divide", edit: func(pm *pkgv1.PackageMap) { pm.GridColumns, pm.GridFactor = 21, 2 }},
		{name: "3 drawn columns", edit: func(pm *pkgv1.PackageMap) { pm.GridColumns = 3 }},
		{name: "201 drawn columns", edit: func(pm *pkgv1.PackageMap) { pm.GridColumns = 201 }},
		{name: "a scaled grid", edit: func(pm *pkgv1.PackageMap) {
			pm.GridColumns, pm.GridFactor = 40, 2
			pm.Layers, pm.FogEnabled = nil, false
		}},
		{name: "an engine grid over the limits", edit: func(pm *pkgv1.PackageMap) {
			pm.GridColumns, pm.GridFactor = 400, 4
			pm.Layers = nil
		}},
		{
			name: "an image with no size", edit: func(pm *pkgv1.PackageMap) { pm.Layers = nil },
			prepare: func(*campaignpackage.Import) {},
		},
		{name: "a layer of another grid", edit: func(pm *pkgv1.PackageMap) { pm.Layers.Walls = []byte{1, 2, 3} }},
		{name: "a cover of another grid", edit: func(pm *pkgv1.PackageMap) { pm.Layers.Cover = []byte{9} }},
		{name: "a light layer of another grid", edit: func(pm *pkgv1.PackageMap) { pm.Layers.Light = []byte{9} }},
		{name: "a door layer of another grid", edit: func(pm *pkgv1.PackageMap) { pm.Layers.Doors = []byte{9} }},
		{name: "a terrain layer of another grid", edit: func(pm *pkgv1.PackageMap) { pm.Layers.DifficultTerrain = []byte{9} }},
		{name: "a bad point", edit: func(pm *pkgv1.PackageMap) { pm.Points[0].Name = "" }},
		{name: "two bad points", edit: func(pm *pkgv1.PackageMap) { pm.Points[0].Name, pm.Points[1].XBp = "", -1 }},
		{name: "no points", edit: func(pm *pkgv1.PackageMap) { pm.Points = nil }},
	}
	var out strings.Builder
	for _, c := range cases {
		in := newStageImport()
		in.IDs.Define(nsImage, pkgImage1)
		if c.prepare != nil {
			c.prepare(in)
		}
		pm := goodMap()
		if c.edit != nil {
			c.edit(pm)
		}
		set := imgs
		switch {
		case c.noImage:
			set = &stagedImages{byOld: map[string]*stagedImage{}}
		case c.name == "an image with no size":
			set = &stagedImages{byOld: map[string]*stagedImage{pkgImage1: {}}}
		}
		sm := part.stageMap(in, pm, set, time.UnixMicro(1_700_000_000_123_456))
		fmt.Fprintf(&out, "== %s\n%s\n%s\ncounts scenes=%d battles=%d\n\n", c.name, renderStagedMap(in, sm), problemsOf(in), in.Counts.Scenes, in.Counts.BattlePoints)
	}
	return out.String()
}

// pkgEntry is an entry of a package built by the tests.
type pkgEntry struct {
	kind pkgv1.PackageEntryKind
	path string
	data []byte
}

// openTestPackage zips the entries with a manifest that tells the truth about
// them and opens the result as the server does.
func openTestPackage(t *testing.T, entries ...pkgEntry) *campaignpackage.Package {
	t.Helper()
	var buf bytes.Buffer
	zw := zip.NewWriter(&buf)
	manifest := &pkgv1.PackageManifest{FormatVersion: campaignpackage.FormatVersion}
	for _, e := range entries {
		sum := sha256.Sum256(e.data)
		manifest.Entries = append(manifest.Entries, &pkgv1.PackageEntry{Path: e.path, Kind: e.kind, Sha256: hex.EncodeToString(sum[:]), Size: int64(len(e.data))})
		w, err := zw.Create(e.path)
		if err != nil {
			t.Fatal(err)
		}
		if _, err := w.Write(e.data); err != nil {
			t.Fatal(err)
		}
	}
	w, err := zw.Create(campaignpackage.ManifestName)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := w.Write(jsonOf(t, manifest)); err != nil {
		t.Fatal(err)
	}
	if err := zw.Close(); err != nil {
		t.Fatal(err)
	}
	pkg, problems := campaignpackage.Open(bytes.NewReader(buf.Bytes()), int64(buf.Len()))
	if len(problems) > 0 {
		t.Fatalf("campaignpackage.Open() problems = %v", problems)
	}
	return pkg
}

func jsonOf(t *testing.T, m proto.Message) []byte {
	t.Helper()
	b, err := protojson.Marshal(m)
	if err != nil {
		t.Fatal(err)
	}
	// protojson adds a random space on purpose, so no output depends on its
	// exact bytes: compact and indent it again to get the same text every run.
	var compact, indented bytes.Buffer
	if err := json.Compact(&compact, b); err != nil {
		t.Fatal(err)
	}
	if err := json.Indent(&indented, compact.Bytes(), "", "  "); err != nil {
		t.Fatal(err)
	}
	return indented.Bytes()
}

const (
	imageKind = pkgv1.PackageEntryKind_PACKAGE_ENTRY_KIND_IMAGE_FILE
	listKind  = pkgv1.PackageEntryKind_PACKAGE_ENTRY_KIND_IMAGES
	mapKind   = pkgv1.PackageEntryKind_PACKAGE_ENTRY_KIND_MAP
)

// stagedImagesOf renders what imagesPart.Stage staged.
func stagedImagesOf(in *campaignpackage.Import) string {
	staged, _ := in.Get(stashImages).(*stagedImages)
	var b strings.Builder
	if staged != nil {
		for _, si := range staged.list {
			// The first image is made "now": the others are older by its distance.
			fmt.Fprintf(&b, "image old=%s name=%q type=%s %dx%d size>0=%v generated=%v kind=%q parent=%s copy=%s olderBy=%dms\n",
				si.oldID, si.name, si.contentType, si.width, si.height, si.size > 0, si.generated, si.generatedKind, si.parentOld, si.copyOld,
				staged.list[0].createdAt.Sub(si.createdAt)/time.Millisecond)
		}
	}
	c := in.Counts
	fmt.Fprintf(&b, "counts images=%d bytes>0=%v maps=%d scenes=%d\n", c.Images, c.ImageBytes > 0, c.Maps, c.Scenes)
	return b.String()
}

// stageImagesGolden stages the images of crafted packages, a good one and a
// list of bad ones, with the problems each gets.
func stageImagesGolden(t *testing.T, part *imagesPart) string {
	t.Helper()
	ctx := t.Context()
	png1 := patternImage(t, 400, 300)
	entry := func(path string, data []byte) pkgEntry { return pkgEntry{imageKind, path, data} }
	list := func(imgs ...*pkgv1.PackageImage) pkgEntry {
		return pkgEntry{listKind, campaignpackage.ImagesEntry, jsonOf(t, &pkgv1.PackageImages{Images: imgs})}
	}
	img := func(id, name, file string) *pkgv1.PackageImage {
		return &pkgv1.PackageImage{Id: id, Name: name, File: file}
	}
	cases := []struct {
		name    string
		entries []pkgEntry
		limits  func(s *Service) // changes the service's limits for this case
	}{
		{name: "no images", entries: nil},
		{name: "files but no list", entries: []pkgEntry{entry("images/1", png1)}},
		{name: "a good list of three", entries: []pkgEntry{
			list(img(pkgImage1, " Mapa ", "images/1"),
				&pkgv1.PackageImage{Id: pkgImage2, Name: "Cena", File: "images/2", Generated: true, GeneratedKind: "scene", ParentImageId: pkgImage1},
				&pkgv1.PackageImage{Id: pkgImage3, Name: "Cópia", File: "images/3", CopyOfImageId: pkgImage1}),
			entry("images/1", png1), entry("images/2", patternImage(t, 40, 30)), entry("images/3", patternImage(t, 50, 20)),
		}},
		{name: "two lists", entries: []pkgEntry{
			list(), {listKind, "other.json", []byte("{}")},
		}},
		{name: "a list under another name", entries: []pkgEntry{{listKind, "lista.json", []byte("{}")}}},
		{name: "a list that is not JSON", entries: []pkgEntry{{listKind, campaignpackage.ImagesEntry, []byte("{")}}},
		{name: "a list with a field the server does not know", entries: []pkgEntry{{listKind, campaignpackage.ImagesEntry, []byte(`{"images":[],"owner":"x"}`)}}},
		{name: "more images than the gallery holds", limits: func(s *Service) { s.maxImages = 2 }, entries: []pkgEntry{
			list(img(pkgImage1, "a", "images/1"), img(pkgImage2, "b", "images/2"), img(pkgImage3, "c", "images/3")),
			entry("images/1", png1), entry("images/2", png1), entry("images/3", png1),
		}},
		{name: "exactly the most images", limits: func(s *Service) { s.maxImages = 2 }, entries: []pkgEntry{
			list(img(pkgImage1, "a", "images/1"), img(pkgImage2, "b", "images/2")), entry("images/1", png1), entry("images/2", png1),
		}},
		{name: "a name that is empty", entries: []pkgEntry{list(img(pkgImage1, " ", "images/1")), entry("images/1", png1)}},
		{name: "an id that is not a UUID", entries: []pkgEntry{list(img("x", "a", "images/1")), entry("images/1", png1)}},
		{name: "an id twice", entries: []pkgEntry{list(img(pkgImage1, "a", "images/1"), img(pkgImage1, "b", "images/2")), entry("images/1", png1), entry("images/2", png1)}},
		{name: "a kind that does not exist", entries: []pkgEntry{
			list(&pkgv1.PackageImage{Id: pkgImage1, Name: "a", File: "images/1", Generated: true, GeneratedKind: "magic"}), entry("images/1", png1),
		}},
		{name: "a kind on an image that is not generated", entries: []pkgEntry{
			list(&pkgv1.PackageImage{Id: pkgImage1, Name: "a", File: "images/1", GeneratedKind: "scene"}), entry("images/1", png1),
		}},
		{name: "a generated image with no kind", entries: []pkgEntry{
			list(&pkgv1.PackageImage{Id: pkgImage1, Name: "a", File: "images/1", Generated: true}), entry("images/1", png1),
		}},
		{name: "a file the package lacks", entries: []pkgEntry{list(img(pkgImage1, "a", "images/9")), entry("images/1", png1)}},
		{name: "a file that is not an image file", entries: []pkgEntry{list(img(pkgImage1, "a", "maps/1.json")), {mapKind, "maps/1.json", []byte("{}")}}},
		{name: "a file two images use", entries: []pkgEntry{list(img(pkgImage1, "a", "images/1"), img(pkgImage2, "b", "images/1")), entry("images/1", png1)}},
		{name: "a file that is not an image", entries: []pkgEntry{list(img(pkgImage1, "a", "images/1")), entry("images/1", []byte("not an image"))}},
		{name: "a file nobody names", entries: []pkgEntry{list(img(pkgImage1, "a", "images/1")), entry("images/1", png1), entry("images/2", png1)}},
		{name: "a gallery bigger than its bytes", limits: func(s *Service) { s.maxBytes = 100 }, entries: []pkgEntry{list(img(pkgImage1, "a", "images/1")), entry("images/1", png1)}},
	}
	var golden strings.Builder
	for _, c := range cases {
		s := part.s
		limits := [2]int32{s.maxImages, s.maxBytes}
		if c.limits != nil {
			c.limits(s)
		}
		in := campaignpackage.NewImport(openTestPackage(t, c.entries...), "campaign-new", "user-new", false, nil)
		err := part.Stage(ctx, in)
		s.maxImages, s.maxBytes = limits[0], limits[1]
		fmt.Fprintf(&golden, "== %s\nerr=%v\n%s%s\n\n", c.name, err, stagedImagesOf(in), problemsOf(in))
	}
	return golden.String()
}

// importedOf renders the rows an import wrote for the campaign: the images
// with their parents and copies, by name.
func importedOf(t *testing.T, h *harness, campaignID string) string {
	t.Helper()
	rows, err := h.pool.Query(t.Context(), `SELECT g.name, g.content_type, g.width, g.height, g.generated, g.generated_kind,
		COALESCE(p.name, '-'), COALESCE(c.name, '-')
		FROM gallery_images g
		LEFT JOIN gallery_images p ON p.id = g.parent_image_id
		LEFT JOIN gallery_images c ON c.id = g.copy_of_image_id
		WHERE g.campaign_id = $1 ORDER BY g.created_at DESC, g.name`, campaignID)
	if err != nil {
		t.Fatalf("read the imported images: %v", err)
	}
	defer rows.Close()
	var b strings.Builder
	for rows.Next() {
		var name, ctype, kind, parent, copyOf string
		var w, hgt int
		var generated bool
		if err := rows.Scan(&name, &ctype, &w, &hgt, &generated, &kind, &parent, &copyOf); err != nil {
			t.Fatal(err)
		}
		fmt.Fprintf(&b, "image %q %s %dx%d generated=%v kind=%q parent=%s copy=%s\n", name, ctype, w, hgt, generated, kind, parent, copyOf)
	}
	if err := rows.Err(); err != nil {
		t.Fatal(err)
	}
	return b.String()
}

// packageFixture is a package with three images and two maps: the first with
// one point of each kind, its layers, and a scene with actions and clues; the
// second with no grid.
func packageFixture(t *testing.T) []pkgEntry {
	t.Helper()
	g := grid.Grid{Columns: 20, Rows: 15}
	terrain, walls, cover, light, doors := layerBytes(g)
	trap := testTrap()
	mapOne := &pkgv1.PackageMap{
		Id: pkgMapA, Name: "Vila", ImageId: pkgImage1, GridColumns: 20, GridFactor: 1, Revealed: true, FogEnabled: true, FogOnFirstGrid: true,
		GroupVision: true, BaseLight: "dim",
		Layers: &pkgv1.PackageLayers{DifficultTerrain: terrain, Walls: walls, Cover: cover, Light: light, Doors: doors},
		Points: []*pkgv1.PackagePoint{
			{
				Id: pkgPoint1, Kind: mapsv1.MapPointKind_MAP_POINT_KIND_SCENE, Name: "A ponte", Description: "Uma ponte.", Hooks: "ganchos", ShowDc: true, XBp: 100, YBp: 200, Revealed: true,
				Actions: []*pkgv1.PackageAction{{Key: "skill:investigation", Name: "Procurar", Dc: 15, MaxAttempts: 2}, {Key: "save:wis"}},
				Clues:   []*pkgv1.PackageClue{{Id: pkgClue1, Text: "pista um"}, {Id: pkgClue2, Text: "pista dois"}},
			},
			{Id: "44444444-4444-4444-8444-444444444443", Kind: mapsv1.MapPointKind_MAP_POINT_KIND_SUBMAP, Name: "Torre", XBp: 300, YBp: 300, TargetMapId: pkgMapB, Stairs: "up"},
			{Id: "44444444-4444-4444-8444-444444444444", Kind: mapsv1.MapPointKind_MAP_POINT_KIND_BATTLE, Name: "Emboscada", XBp: 400, YBp: 400, TargetMapId: pkgMapB},
			{Id: "44444444-4444-4444-8444-444444444445", Kind: mapsv1.MapPointKind_MAP_POINT_KIND_TREASURE, Name: "Baú", XBp: 500, YBp: 500, TreasureValuePo: proto.Int32(250)},
			{Id: "44444444-4444-4444-8444-444444444446", Kind: mapsv1.MapPointKind_MAP_POINT_KIND_TRAP, Name: "Fosso", XBp: 600, YBp: 600, Trap: trap},
			{Id: "44444444-4444-4444-8444-444444444447", Kind: mapsv1.MapPointKind_MAP_POINT_KIND_LIGHT, Name: "Tocha", XBp: 700, YBp: 700, Light: &mapsv1.LightSpec{PresetKey: "light:torch"}},
		},
	}
	mapTwo := &pkgv1.PackageMap{Id: pkgMapB, Name: "Subsolo", ImageId: pkgImage2, GridFactor: 1, BaseLight: "dark"}
	mapThree := &pkgv1.PackageMap{Id: pkgMapC, Name: "Grade vazia", ImageId: pkgImage1, GridColumns: 20, GridFactor: 1, BaseLight: "bright"}
	return []pkgEntry{
		{listKind, campaignpackage.ImagesEntry, jsonOf(t, &pkgv1.PackageImages{Images: []*pkgv1.PackageImage{
			{Id: pkgImage1, Name: "Mapa", File: "images/1"},
			{Id: pkgImage2, Name: "Cena", File: "images/2", Generated: true, GeneratedKind: "scene", ParentImageId: pkgImage1},
			{Id: pkgImage3, Name: "Cópia", File: "images/3", CopyOfImageId: pkgImage1},
		}})},
		{imageKind, "images/1", patternImage(t, 400, 300)},
		{imageKind, "images/2", patternImage(t, 40, 30)},
		{imageKind, "images/3", patternImage(t, 50, 20)},
		{mapKind, "maps/1.json", jsonOf(t, mapOne)},
		{mapKind, "maps/2.json", jsonOf(t, mapTwo)},
		{mapKind, "maps/3.json", jsonOf(t, mapThree)},
	}
}

// stageApplyExportGolden stages a package's images and maps, writes them in a
// transaction as the importer does, and exports the maps back, with the ids of
// the package put back in the place of the new ones.
func stageApplyExportGolden(t *testing.T, s *scenes) string {
	t.Helper()
	ctx := t.Context()
	svc := s.h.svc
	images, mapsP := &imagesPart{s: svc}, &mapsPart{s: svc}
	in := campaignpackage.NewImport(openTestPackage(t, packageFixture(t)...), s.campaign, s.master.id, false, nil)
	var b strings.Builder
	for _, stage := range []func(context.Context, *campaignpackage.Import) error{images.Stage, mapsP.Stage} {
		if err := stage(ctx, in); err != nil {
			t.Fatalf("Stage() error = %v", err)
		}
	}
	fmt.Fprintf(&b, "%s%s\n", stagedImagesOf(in), problemsOf(in))
	fmt.Fprintf(&b, "facts maps=%v kinds=%v names=%v\n", in.Facts.PointMap != nil, len(in.Facts.PointKind), len(in.Facts.PointName))
	for _, id := range []string{pkgPoint1, "44444444-4444-4444-8444-444444444443", "44444444-4444-4444-8444-444444444445"} {
		fmt.Fprintf(&b, "fact %s map=%s kind=%s name=%s\n", id, in.Facts.PointMap[id], in.Facts.PointKind[id], in.Facts.PointName[id])
	}
	fmt.Fprintf(&b, "door at (2,2) of map A=%v, (3,3)=%v, of map B=%v, of an unknown map=%v, outside the grid=%v\n",
		in.Facts.DoorAt(pkgMapA, 2, 2), in.Facts.DoorAt(pkgMapA, 3, 3), in.Facts.DoorAt(pkgMapB, 2, 2), in.Facts.DoorAt(pkgMapGone, 2, 2), in.Facts.DoorAt(pkgMapA, 99, 99))

	err := db.InTx(ctx, s.h.pool, func(tx pgx.Tx) error {
		if err := images.Apply(ctx, tx, in); err != nil {
			return err
		}
		return mapsP.Apply(ctx, tx, in)
	})
	if err != nil {
		t.Fatalf("Apply() error = %v", err)
	}
	// A part with nothing staged writes nothing.
	if err := db.InTx(ctx, s.h.pool, func(tx pgx.Tx) error { return mapsP.Apply(ctx, tx, newStageImport()) }); err != nil {
		t.Fatalf("Apply() with nothing staged error = %v", err)
	}
	b.WriteString(importedOf(t, s.h, s.campaign))

	// Export the imported maps and put the package's ids back.
	var pairs []string
	for _, ns := range []string{nsImage, nsMap, nsPoint, nsClue} {
		for _, old := range []string{
			pkgImage1, pkgImage2, pkgImage3, pkgMapA, pkgMapB, pkgMapC, pkgPoint1, pkgPoint2, pkgClue1, pkgClue2,
			"44444444-4444-4444-8444-444444444443", "44444444-4444-4444-8444-444444444444", "44444444-4444-4444-8444-444444444445",
			"44444444-4444-4444-8444-444444444446", "44444444-4444-4444-8444-444444444447",
		} {
			if id, ok := in.IDs.Get(ns, old); ok {
				pairs = append(pairs, id, old)
			}
		}
	}
	back := strings.NewReplacer(pairs...)
	rows, err := svc.queries.ListMapsOfCampaign(ctx, s.campaign)
	if err != nil {
		t.Fatalf("ListMapsOfCampaign() error = %v", err)
	}
	for _, m := range rows {
		if m.ID == s.mapID {
			continue
		}
		pm, err := mapsP.exportMap(ctx, svc.queries, m)
		if err != nil {
			t.Fatalf("exportMap() error = %v", err)
		}
		fmt.Fprintf(&b, "export %s\n", back.Replace(string(jsonOf(t, pm))))
	}
	return b.String()
}

// TestPackageStagingIsFrozen freezes what the maps module's parts of a campaign
// package do: every problem of a point, a map and an image with its kind, name
// and reason, and a package staged, written and exported again.
func TestPackageStagingIsFrozen(t *testing.T) {
	t.Parallel()
	s := newScenes(t, false)
	checkGolden(t, "package_stage_point.golden.txt", []byte(stagePointGolden(t, &mapsPart{s: s.h.svc})))
	checkGolden(t, "package_stage_map.golden.txt", []byte(stageMapGolden(t, &mapsPart{s: s.h.svc})))
	checkGolden(t, "package_stage_images.golden.txt", []byte(stageImagesGolden(t, &imagesPart{s: s.h.svc})))
	checkGolden(t, "package_apply_export.golden.txt", []byte(stageApplyExportGolden(t, s)))
}
