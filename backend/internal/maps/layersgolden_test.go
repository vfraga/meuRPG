package maps

import (
	"bytes"
	"encoding/json"
	"errors"
	"regexp"
	"strconv"
	"testing"

	"connectrpc.com/connect"
	"github.com/jackc/pgx/v5"
	"google.golang.org/protobuf/encoding/protojson"
	"google.golang.org/protobuf/proto"

	charactersv1 "github.com/PuraFome/meuRPG/backend/gen/meurpg/characters/v1"
	mapsv1 "github.com/PuraFome/meuRPG/backend/gen/meurpg/maps/v1"
	playv1 "github.com/PuraFome/meuRPG/backend/gen/meurpg/play/v1"
	"github.com/PuraFome/meuRPG/backend/internal/platform/db"
	"github.com/PuraFome/meuRPG/backend/internal/rules/grid"
)

// Characterization of what each viewer reads of a map and its layers (RN-10, MR-034,
// MR-036, RN-26): GetMap, ListMaps and GetMapLayers for the master, the master "Ver
// como", two players and a member with no character, on a map without fog and on the
// cave with the fog on, with the doors in every state, the lights, the traps (secret,
// revealed to one character, fired, revealed), the treasures (hidden, shown, found)
// and the submap points that name the parents. Each answer is a golden file
// (testdata/layers_*.golden.json, ids replaced by names and the times by <time>); run
// with -update-golden to write them again after a change of behavior that was
// meant. The refusals of the calls that write the layers are frozen with their exact
// words in TestLayersFrozen_Refusals.

var (
	layersIDPattern   = regexp.MustCompile(`[0-9a-f]{8}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{12}`)
	layersTimePattern = regexp.MustCompile(`"\d{4}-\d\d-\d\dT[0-9:.]+Z"`)
)

// layersGolden names the ids of a table and compares answers with their files.
type layersGolden struct {
	table  string
	labels map[string]string
	extra  map[string]string // ids met in the answers that the table did not name, in order of appearance
}

func newLayersGolden(table string) *layersGolden {
	return &layersGolden{table: table, labels: map[string]string{}, extra: map[string]string{}}
}

func (g *layersGolden) name(id, label string) { g.labels[id] = label }

// scrub replaces the ids by their names and the times by <time>.
func (g *layersGolden) scrub(raw []byte) []byte {
	raw = layersTimePattern.ReplaceAll(raw, []byte(`"<time>"`))
	return layersIDPattern.ReplaceAllFunc(raw, func(id []byte) []byte {
		if label, ok := g.labels[string(id)]; ok {
			return []byte(label)
		}
		label, ok := g.extra[string(id)]
		if !ok {
			label = "id" + strconv.Itoa(len(g.extra)+1)
			g.extra[string(id)] = label
		}
		return []byte(label)
	})
}

// check compares what the parts say with the viewer's file.
func (g *layersGolden) check(t *testing.T, viewer string, parts map[string]proto.Message, errs map[string]error) {
	t.Helper()
	tree := map[string]any{}
	for name, msg := range parts {
		raw, err := protojson.Marshal(msg)
		if err != nil {
			t.Fatal(err)
		}
		var v any
		if err := json.Unmarshal(raw, &v); err != nil {
			t.Fatal(err)
		}
		tree[name] = v
	}
	for name, err := range errs {
		tree[name] = "error: " + layersShapeOf(err)
	}
	raw, err := json.Marshal(tree) // the keys come out sorted
	if err != nil {
		t.Fatal(err)
	}
	var out bytes.Buffer
	if err := json.Indent(&out, g.scrub(raw), "", "  "); err != nil {
		t.Fatal(err)
	}
	out.WriteByte('\n')
	checkGolden(t, "layers_"+g.table+"_"+viewer+".golden.json", out.Bytes())
}

// layersShapeOf is what a refusal says: the code, the message and the reasons of its details.
func layersShapeOf(err error) string {
	if err == nil {
		return "<nil>"
	}
	out := connect.CodeOf(err).String() + "|" + err.Error()
	if ce, ok := errors.AsType[*connect.Error](err); ok {
		for _, d := range ce.Details() {
			if v, err := d.Value(); err == nil {
				if b, ok := v.(*mapsv1.MapBlocked); ok {
					out += "|" + b.GetReason().String()
				}
			}
		}
	}
	return out
}

// layersReader is one viewer of the golden tables.
type layersReader struct {
	name string
	u    *user
	as   string // the character the master reads as
}

// read collects what the viewer reads of the maps and the layers of the table.
func (g *layersGolden) read(t *testing.T, campaign string, r layersReader, mapIDs map[string]string) {
	t.Helper()
	ctx := t.Context()
	parts := map[string]proto.Message{}
	errs := map[string]error{}
	list, err := r.u.maps.ListMaps(ctx, connect.NewRequest(&mapsv1.ListMapsRequest{CampaignId: campaign}))
	if err != nil {
		t.Fatalf("ListMaps(%s) error = %v", r.name, err)
	}
	parts["listMaps"] = list.Msg
	for label, id := range mapIDs {
		got, err := r.u.maps.GetMap(ctx, connect.NewRequest(&mapsv1.GetMapRequest{CampaignId: campaign, MapId: id, AsCharacterId: r.as}))
		if err != nil {
			errs["getMap_"+label] = err
		} else {
			parts["getMap_"+label] = got.Msg
		}
		layers, err := r.u.maps.GetMapLayers(ctx, connect.NewRequest(&mapsv1.GetMapLayersRequest{CampaignId: campaign, MapId: id, AsCharacterId: r.as}))
		if err != nil {
			errs["layers_"+label] = err
		} else {
			parts["layers_"+label] = layers.Msg
		}
	}
	g.check(t, r.name, parts, errs)
}

// paintDoors puts a door in every state, one on a wall and one where there is
// nothing, so that a player's read of the doors and the walls differs from the master's.
func paintDoors(m *user, campaign, mapID string) {
	m.h.t.Helper()
	for state, cells := range map[mapsv1.DoorState][][2]int32{
		mapsv1.DoorState_DOOR_STATE_OPEN:   {{1, 4}},
		mapsv1.DoorState_DOOR_STATE_CLOSED: {{2, 4}},
		mapsv1.DoorState_DOOR_STATE_LOCKED: {{3, 4}},
		mapsv1.DoorState_DOOR_STATE_BARRED: {{4, 4}},
		mapsv1.DoorState_DOOR_STATE_SECRET: {{5, 4}, {3, 3}},
	} {
		m.mustPaint(campaign, mapID, mapsv1.MapLayer_MAP_LAYER_DOORS, int32(state), cells...)
	}
}

// layersSubmap puts a revealed or hidden submap point on mapID that leads to target.
func layersSubmap(m *user, campaign, mapID, target, name string, x, y int32, revealed bool) *mapsv1.MapPoint {
	m.h.t.Helper()
	p := m.createPoint(&mapsv1.CreateMapPointRequest{
		CampaignId: campaign, MapId: mapID, Kind: mapsv1.MapPointKind_MAP_POINT_KIND_SUBMAP, Name: name, XBp: x, YBp: y, TargetMapId: target,
	})
	if revealed {
		p = m.setPointRevealed(campaign, p, true)
	}
	return p
}

func TestLayersGolden_WhatEachViewerReadsWithoutFog(t *testing.T) {
	t.Parallel()
	s := newScenes(t, true)
	m := s.master
	g := newLayersGolden("mirathel")
	visitor := s.h.newUser("Visitante")
	s.h.join(m, s.campaign, false, visitor)

	m.mustSetGrid(s.campaign, s.mapID, 20)
	m.mustPaint(s.campaign, s.mapID, mapsv1.MapLayer_MAP_LAYER_WALL, 1, [2]int32{1, 1}, [2]int32{3, 3}, [2]int32{6, 6})
	m.mustPaint(s.campaign, s.mapID, mapsv1.MapLayer_MAP_LAYER_DIFFICULT_TERRAIN, 1, [2]int32{2, 2})
	m.mustPaint(s.campaign, s.mapID, mapsv1.MapLayer_MAP_LAYER_COVER, 1, [2]int32{3, 1})
	m.mustPaint(s.campaign, s.mapID, mapsv1.MapLayer_MAP_LAYER_COVER, 2, [2]int32{3, 2})
	m.mustPaint(s.campaign, s.mapID, mapsv1.MapLayer_MAP_LAYER_LIGHT, 3, [2]int32{4, 4})
	m.mustPaint(s.campaign, s.mapID, mapsv1.MapLayer_MAP_LAYER_LIGHT, 1, [2]int32{5, 5})
	paintDoors(m, s.campaign, s.mapID)
	m.placeToken(s.campaign, s.mapID, s.pens.GetId(), 3000, 3000)
	m.placeToken(s.campaign, s.mapID, s.other.GetId(), 4000, 4000)
	for _, c := range []struct{ character, key string }{{s.other.GetId(), "light:torch"}, {s.pens.GetId(), "light:hooded-lantern"}} {
		if _, err := m.maps.SetCarriedLight(t.Context(), connect.NewRequest(&mapsv1.SetCarriedLightRequest{CampaignId: s.campaign, MapId: s.mapID, CharacterId: c.character, LightKey: c.key})); err != nil {
			t.Fatal(err)
		}
	}
	if _, err := m.maps.SetMapFog(t.Context(), connect.NewRequest(&mapsv1.SetMapFogRequest{CampaignId: s.campaign, MapId: s.mapID, BaseLight: mapsv1.LightLevel_LIGHT_LEVEL_DIM.Enum()})); err != nil {
		t.Fatal(err)
	}

	trapA := s.newTrap("ARMADILHA-SECRETA-A", 1234, 2345)
	trapB := s.newTrap("ARMADILHA-SECRETA-B", 3456, 4567)
	trapC := s.newTrap("ARMADILHA-DISPARADA-C", 5678, 6789)
	trapD := s.newTrap("ARMADILHA-REVELADA-D", 7890, 8901)
	light := s.newLight("LUZ-SECRETA", "light:torch")
	hidden := s.newTreasure("TESOURO-ESCONDIDO", "CONTEUDO-ESCONDIDO", 7777, 1111, 9111)
	shown := s.newTreasure("TESOURO-REVELADO", "CONTEUDO-REVELADO", 8888, 2222, 9222)
	found := s.newTreasure("TESOURO-ACHADO", "CONTEUDO-ACHADO", 9999, 3333, 9333)
	m.setPointRevealed(s.campaign, trapD, true)
	m.setPointRevealed(s.campaign, shown, true)
	if _, err := m.maps.MarkTreasureFound(t.Context(), connect.NewRequest(&mapsv1.MarkTreasureFoundRequest{
		CampaignId: s.campaign, MapId: s.mapID, PointId: found.GetId(), CharacterIds: []string{s.pens.GetId()},
	})); err != nil {
		t.Fatalf("MarkTreasureFound() error = %v", err)
	}
	if _, err := m.maps.RevealTrap(t.Context(), connect.NewRequest(&mapsv1.RevealTrapRequest{
		CampaignId: s.campaign, MapId: s.mapID, PointId: trapB.GetId(), CharacterIds: []string{s.other.GetId()},
	})); err != nil {
		t.Fatalf("RevealTrap() error = %v", err)
	}
	spec := testTrap()
	spec.State = mapsv1.TrapState_TRAP_STATE_TRIGGERED
	if _, err := m.updatePoint(&mapsv1.UpdateMapPointRequest{CampaignId: s.campaign, MapId: s.mapID, PointId: trapC.GetId(), Trap: spec}); err != nil {
		t.Fatal(err)
	}

	// The maps around it: a tower and a crypt (hidden), with submap points to and from.
	tower := m.createMap(s.campaign, "Torre", m.newImage(s.campaign))
	m.setMapRevealed(s.campaign, tower.GetId(), true)
	crypt := m.createMap(s.campaign, "Cripta", m.newImage(s.campaign))
	lonely := m.createMap(s.campaign, "Sem grade", m.newImage(s.campaign))
	m.setMapRevealed(s.campaign, lonely.GetId(), true)
	toTower := layersSubmap(m, s.campaign, s.mapID, tower.GetId(), "Para a torre", 6000, 1000, true)
	toCrypt := layersSubmap(m, s.campaign, s.mapID, crypt.GetId(), "Para a cripta", 6500, 1500, true)
	secretStairs := layersSubmap(m, s.campaign, s.mapID, tower.GetId(), "Escada secreta", 6800, 1800, false)
	fromTower := layersSubmap(m, s.campaign, tower.GetId(), s.mapID, "De volta", 1000, 1000, true)
	fromCrypt := layersSubmap(m, s.campaign, crypt.GetId(), s.mapID, "Saida da cripta", 2000, 2000, true)
	fromLonely := layersSubmap(m, s.campaign, lonely.GetId(), s.mapID, "Porta fechada", 2000, 2000, false)

	for id, label := range map[string]string{
		s.campaign: "campaign", s.mapID: "mirathel", tower.GetId(): "tower", crypt.GetId(): "crypt", lonely.GetId(): "lonely",
		s.pens.GetId(): "pens", s.other.GetId(): "toren", s.point.GetId(): "cart",
		trapA.GetId(): "trapA", trapB.GetId(): "trapB", trapC.GetId(): "trapC", trapD.GetId(): "trapD", light.GetId(): "light",
		hidden.GetId(): "treasureHidden", shown.GetId(): "treasureShown", found.GetId(): "treasureFound",
		toTower.GetId(): "toTower", toCrypt.GetId(): "toCrypt", secretStairs.GetId(): "secretStairs",
		fromTower.GetId(): "fromTower", fromCrypt.GetId(): "fromCrypt", fromLonely.GetId(): "fromLonely",
	} {
		g.name(id, label)
	}
	maps := map[string]string{"mirathel": s.mapID, "tower": tower.GetId(), "crypt": crypt.GetId(), "lonely": lonely.GetId()}
	for _, r := range []layersReader{
		{"master", m, ""}, {"master_as_pens", m, s.pens.GetId()}, {"ana", s.ana, ""}, {"caio", s.caio, ""}, {"visitor", visitor, ""},
	} {
		g.read(t, s.campaign, r, maps)
	}
}

func TestLayersGolden_WhatEachViewerReadsWithFog(t *testing.T) {
	t.Parallel()
	c := newCave(t)
	m := c.master
	g := newLayersGolden("cave")
	visitor := c.h.newUser("Visitante")
	c.h.join(m, c.campaign, false, visitor)

	// The doors sit in the guard room's wall and the corridor, in every state; the light layer
	// has a bright spot in the corridor and a dark one in the guard room.
	for state, cells := range map[mapsv1.DoorState][][2]int32{
		mapsv1.DoorState_DOOR_STATE_OPEN:   {{7, 7}},
		mapsv1.DoorState_DOOR_STATE_CLOSED: {{10, 8}},
		mapsv1.DoorState_DOOR_STATE_LOCKED: {{12, 8}},
		mapsv1.DoorState_DOOR_STATE_BARRED: {{14, 8}},
		mapsv1.DoorState_DOOR_STATE_SECRET: {{15, 7}, {16, 6}, {7, 6}},
	} {
		m.mustPaint(c.campaign, c.mapID, mapsv1.MapLayer_MAP_LAYER_DOORS, int32(state), cells...)
	}
	m.mustPaint(c.campaign, c.mapID, mapsv1.MapLayer_MAP_LAYER_LIGHT, 3, [2]int32{8, 7}, [2]int32{9, 7})
	m.mustPaint(c.campaign, c.mapID, mapsv1.MapLayer_MAP_LAYER_LIGHT, 1, [2]int32{20, 4})
	if _, err := m.maps.SetCarriedLight(t.Context(), connect.NewRequest(&mapsv1.SetCarriedLightRequest{CampaignId: c.campaign, MapId: c.mapID, CharacterId: c.toren.GetId(), LightKey: "light:torch"})); err != nil {
		t.Fatal(err)
	}
	crypt := m.createMap(c.campaign, "A cripta", m.newImage(c.campaign))
	m.setMapRevealed(c.campaign, crypt.GetId(), true)
	seenPoint := layersSubmap(m, c.campaign, crypt.GetId(), c.mapID, "Para a caverna", 100, 100, true)
	overPoint := layersSubmap(m, c.campaign, c.mapID, crypt.GetId(), "Para a cripta", 2000, 4000, true)

	for id, label := range map[string]string{
		c.campaign: "campaign", c.mapID: "cave", crypt.GetId(): "crypt", c.probeMap: "probe",
		c.pens.GetId(): "pens", c.toren.GetId(): "toren", c.brisa.GetId(): "brisa", c.salvia.GetId(): "salvia",
		c.goblin1.GetId(): "goblin1", c.goblin2.GetId(): "goblin2", c.captain.GetId(): "captain", c.hiddenGoblin.GetId(): "hiddenGoblin",
		c.entrance.GetId(): "entrance", c.guardhouse.GetId(): "guardhouse", c.chest.GetId(): "chest", c.torch.GetId(): "torch",
		seenPoint.GetId(): "fromCrypt", overPoint.GetId(): "toCrypt", c.imageID: "caveImage",
	} {
		g.name(id, label)
	}
	maps := map[string]string{"cave": c.mapID, "crypt": crypt.GetId(), "probe": c.probeMap}
	master, asPens, asToren := layersReader{"master", m, ""}, layersReader{"master_as_pens", m, c.pens.GetId()}, layersReader{"master_as_toren", m, c.toren.GetId()}
	ana, caio, visitorReads := layersReader{"ana", c.ana, ""}, layersReader{"caio", c.caio, ""}, layersReader{"visitor", visitor, ""}
	// Each state reads the layers again for the viewers it can change: the fog for all of them
	// (the two characters the master reads as, and the two players, whose own lights differ),
	// the party's vision for the master, a player and the member with no character, the fog
	// off for the master, one character and one player.
	phase := func(prefix string, viewers ...layersReader) {
		t.Helper()
		for _, r := range viewers {
			r.name = prefix + r.name
			g.read(t, c.campaign, r, maps)
		}
	}
	phase("fog_", master, asPens, asToren, ana, caio, visitorReads)

	// The party's vision, then the fog off: the same layers read again.
	if _, err := m.maps.SetMapFog(t.Context(), connect.NewRequest(&mapsv1.SetMapFogRequest{CampaignId: c.campaign, MapId: c.mapID, GroupVision: new(true)})); err != nil {
		t.Fatal(err)
	}
	phase("group_", master, ana, visitorReads)
	if _, err := m.maps.SetMapFog(t.Context(), connect.NewRequest(&mapsv1.SetMapFogRequest{CampaignId: c.campaign, MapId: c.mapID, FogEnabled: new(false)})); err != nil {
		t.Fatal(err)
	}
	phase("nofog_", master, asPens, ana)
}

// layersCall runs a call and returns its refusal's words.
type layersCall struct {
	name string
	run  func() error
	want string
}

func checkLayersCalls(t *testing.T, calls []layersCall) {
	t.Helper()
	for _, c := range calls {
		if got := layersShapeOf(c.run()); got != c.want {
			t.Errorf("%s: error = %s, want %s", c.name, got, c.want)
		}
	}
}

// TestLayersFrozen_Refusals: every refusal of the calls that paint, set the fog, carry a
// light and read the layers, in the order the checks run, with the exact words.
func TestLayersFrozen_Refusals(t *testing.T) {
	t.Parallel()
	s := newScenes(t, true)
	m := s.master
	ctx := t.Context()
	outsider := s.h.newUser("Intruso")
	hiddenMap := m.createMap(s.campaign, "Escondido", m.newImage(s.campaign)).GetId()
	foreign := newKey()
	paintCall := func(u *user, campaign, mapID string, layer mapsv1.MapLayer, value int32, cells ...[2]int32) func() error {
		return func() error {
			_, err := u.maps.PaintMapCells(ctx, connect.NewRequest(&mapsv1.PaintMapCellsRequest{
				CampaignId: campaign, MapId: mapID, Layer: layer, Value: value, Squares: squares(cells...),
			}))
			return err
		}
	}
	fogCall := func(u *user, mapID string, edit func(*mapsv1.SetMapFogRequest)) func() error {
		return func() error {
			req := &mapsv1.SetMapFogRequest{CampaignId: s.campaign, MapId: mapID}
			edit(req)
			_, err := u.maps.SetMapFog(ctx, connect.NewRequest(req))
			return err
		}
	}
	lightCall := func(u *user, mapID, character, key string) func() error {
		return func() error {
			_, err := u.maps.SetCarriedLight(ctx, connect.NewRequest(&mapsv1.SetCarriedLightRequest{CampaignId: s.campaign, MapId: mapID, CharacterId: character, LightKey: key}))
			return err
		}
	}
	layersCallOf := func(u *user, campaign, mapID, as string) func() error {
		return func() error {
			_, err := u.maps.GetMapLayers(ctx, connect.NewRequest(&mapsv1.GetMapLayersRequest{CampaignId: campaign, MapId: mapID, AsCharacterId: as}))
			return err
		}
	}
	const (
		notFoundMap = "not_found|not_found: map not found"
		denied      = "permission_denied|permission_denied: only the campaign's master can do this"
		noGrid      = "failed_precondition|failed_precondition: the map has no grid|MAP_BLOCKED_REASON_NO_GRID"
	)
	wall := mapsv1.MapLayer_MAP_LAYER_WALL
	one := [2]int32{0, 0}

	// Before the map has a grid.
	checkLayersCalls(t, []layersCall{
		{"paint: a player", paintCall(s.ana, s.campaign, s.mapID, wall, 1, one), denied},
		{"paint: an outsider", paintCall(outsider, s.campaign, s.mapID, wall, 1, one), "not_found|not_found: campaign not found"},
		{"paint: a map that is not an id", paintCall(m, s.campaign, "x", wall, 1, one), notFoundMap},
		{"paint: the id is checked before the layer", paintCall(m, s.campaign, "x", mapsv1.MapLayer_MAP_LAYER_UNSPECIFIED, 9), notFoundMap},
		{"paint: no layer", paintCall(m, s.campaign, s.mapID, mapsv1.MapLayer_MAP_LAYER_UNSPECIFIED, 1, one), "invalid_argument|invalid_argument: layer is required"},
		{"paint: the layer is checked before the squares", paintCall(m, s.campaign, s.mapID, wall, 2), "invalid_argument|invalid_argument: value must be 0 to 1 for this layer"},
		{"paint: cover value", paintCall(m, s.campaign, s.mapID, mapsv1.MapLayer_MAP_LAYER_COVER, 3, one), "invalid_argument|invalid_argument: value must be 0 to 2 for this layer"},
		{"paint: light value", paintCall(m, s.campaign, s.mapID, mapsv1.MapLayer_MAP_LAYER_LIGHT, -1, one), "invalid_argument|invalid_argument: value must be 0 to 3 for this layer"},
		{"paint: door value", paintCall(m, s.campaign, s.mapID, mapsv1.MapLayer_MAP_LAYER_DOORS, 6, one), "invalid_argument|invalid_argument: value must be 0 to 5 for this layer"},
		{"paint: no squares", paintCall(m, s.campaign, s.mapID, wall, 1), "invalid_argument|invalid_argument: squares must have 1 to 400 squares"},
		{"paint: another campaign's map", paintCall(m, s.campaign, foreign, wall, 1, one), notFoundMap},
		{"paint: no grid", paintCall(m, s.campaign, s.mapID, wall, 1, one), noGrid},
		{"fog: a player", fogCall(s.ana, s.mapID, func(r *mapsv1.SetMapFogRequest) { r.FogEnabled = new(true) }), denied},
		{"fog: not an id", fogCall(m, "x", func(r *mapsv1.SetMapFogRequest) { r.FogEnabled = new(true) }), notFoundMap},
		{"fog: the id comes before nothing to change", fogCall(m, "x", func(*mapsv1.SetMapFogRequest) {}), notFoundMap},
		{"fog: nothing", fogCall(m, s.mapID, func(*mapsv1.SetMapFogRequest) {}), "invalid_argument|invalid_argument: nothing to change"},
		{"fog: base light unspecified", fogCall(m, s.mapID, func(r *mapsv1.SetMapFogRequest) { r.BaseLight = mapsv1.LightLevel_LIGHT_LEVEL_UNSPECIFIED.Enum() }), "invalid_argument|invalid_argument: base_light must be DARK, DIM or BRIGHT"},
		{"fog: base light checked before the map", fogCall(m, foreign, func(r *mapsv1.SetMapFogRequest) { r.BaseLight = mapsv1.LightLevel_LIGHT_LEVEL_UNSPECIFIED.Enum() }), "invalid_argument|invalid_argument: base_light must be DARK, DIM or BRIGHT"},
		{"fog: another campaign's map", fogCall(m, foreign, func(r *mapsv1.SetMapFogRequest) { r.GroupVision = new(true) }), notFoundMap},
		{"fog: on, no grid", fogCall(m, s.mapID, func(r *mapsv1.SetMapFogRequest) { r.FogEnabled = new(true) }), noGrid},
		{"fog: off, no grid, nothing happens", fogCall(m, s.mapID, func(r *mapsv1.SetMapFogRequest) { r.FogEnabled = new(false) }), "<nil>"},
		{"carry: not a member", lightCall(outsider, s.mapID, s.pens.GetId(), "light:torch"), "not_found|not_found: campaign not found"},
		{"carry: not an id", lightCall(s.ana, "x", s.pens.GetId(), "light:torch"), notFoundMap},
		{"carry: the map id comes before the key", lightCall(s.ana, "x", s.pens.GetId(), "light:nope"), notFoundMap},
		{"carry: a key that is no preset", lightCall(s.ana, foreign, s.pens.GetId(), "light:nope"), "invalid_argument|invalid_argument: light_key is not a light preset"},
		{"carry: the key comes before the character", lightCall(s.ana, s.mapID, newKey(), "light:nope"), "invalid_argument|invalid_argument: light_key is not a light preset"},
		{"carry: a character that is not there", lightCall(s.ana, s.mapID, newKey(), "light:torch"), "not_found|not_found: character not found"},
		{"carry: not an id of a character", lightCall(s.ana, s.mapID, "x", "light:torch"), "not_found|not_found: character not found"},
		{"carry: another player's character", lightCall(s.ana, s.mapID, s.other.GetId(), "light:torch"), "permission_denied|permission_denied: only the character's player or the master may do this"},
		{"carry: no token", lightCall(s.ana, s.mapID, s.pens.GetId(), "light:torch"), "not_found|not_found: token not found"},
		{"carry: the master, no token", lightCall(m, s.mapID, s.pens.GetId(), ""), "not_found|not_found: token not found"},
		{"carry: a hidden map to a player", lightCall(s.ana, hiddenMap, s.pens.GetId(), "light:torch"), notFoundMap},
		{"carry: the map of another campaign to the master", lightCall(m, foreign, s.pens.GetId(), "light:torch"), notFoundMap},
		{"layers: an outsider", layersCallOf(outsider, s.campaign, s.mapID, ""), "not_found|not_found: campaign not found"},
		{"layers: not an id", layersCallOf(s.ana, s.campaign, "x", ""), notFoundMap},
		{"layers: a player reads as a character", layersCallOf(s.ana, s.campaign, s.mapID, s.pens.GetId()), "permission_denied|permission_denied: only the master may read as a character"},
		{"layers: the master reads as nobody", layersCallOf(m, s.campaign, s.mapID, newKey()), "not_found|not_found: character not found"},
		{"layers: the master reads as a bad id", layersCallOf(m, s.campaign, s.mapID, "x"), "not_found|not_found: character not found"},
		{"layers: the id comes before the reader", layersCallOf(s.ana, s.campaign, "x", s.pens.GetId()), notFoundMap},
		{"layers: a map of another campaign", layersCallOf(m, s.campaign, foreign, ""), notFoundMap},
		{"layers: a hidden map to a player", layersCallOf(s.ana, s.campaign, hiddenMap, ""), notFoundMap},
		{"layers: a hidden map to the master", layersCallOf(m, s.campaign, hiddenMap, ""), "<nil>"},
		{"layers: no grid", layersCallOf(s.ana, s.campaign, s.mapID, ""), "<nil>"},
	})

	// A map with no grid reads as no layers, and the master's revision adds the light's.
	for name, u := range map[string]*user{"master": m, "player": s.ana} {
		if res := u.mustLayers(s.campaign, s.mapID); res.GetGridColumns() != 0 || res.GetGridRows() != 0 || len(res.GetWall()) != 0 || res.GetFogWithheld() {
			t.Errorf("%s reads %v of a map with no grid, want nothing", name, res)
		}
	}

	// With a grid: the squares are checked against it, and nothing is painted by a refused batch.
	m.mustSetGrid(s.campaign, s.mapID, 20)
	checkLayersCalls(t, []layersCall{
		{"paint: after the last column", paintCall(m, s.campaign, s.mapID, wall, 1, one, [2]int32{20, 0}), "invalid_argument|invalid_argument: squares[1] is outside the map's grid of 20 x 15 squares"},
		{"paint: after the last row", paintCall(m, s.campaign, s.mapID, wall, 1, [2]int32{0, 15}), "invalid_argument|invalid_argument: squares[0] is outside the map's grid of 20 x 15 squares"},
		{"paint: before the first", paintCall(m, s.campaign, s.mapID, wall, 1, [2]int32{0, -1}), "invalid_argument|invalid_argument: squares[0] is outside the map's grid of 20 x 15 squares"},
		{"fog: on, with a grid", fogCall(m, s.mapID, func(r *mapsv1.SetMapFogRequest) { r.FogEnabled = new(true) }), "<nil>"},
		{"carry: a player's own hidden token on a fog map is found (D6), and a character with no token is not", lightCall(s.ana, s.mapID, s.pens.GetId(), "light:torch"), "not_found|not_found: token not found"},
	})
	if d := decode(t, m.mustLayers(s.campaign, s.mapID)); d.walls.Count() != 0 {
		t.Errorf("a refused batch painted %d walls", d.walls.Count())
	}
}

// TestLayersFrozen_PaintingCountsAndRevisions: what PaintMapCells answers (the squares that
// changed and the revision the caller reads), and the revisions the other readers see, for
// each layer; a calibrated map paints a whole door and square by square the rest.
func TestLayersFrozen_PaintingCountsAndRevisions(t *testing.T) {
	t.Parallel()
	s := newScenes(t, true)
	m := s.master
	m.mustSetCalibration(s.campaign, s.mapID, 8, 2) // 16 x 12 squares, doors in blocks of 2 x 2
	gr := grid.Grid{Columns: 16, Rows: 12}
	rev := func(u *user) int32 { return u.mustLayers(s.campaign, s.mapID).GetLayersRevision() }
	base := rev(m)
	playerBase := rev(s.ana)

	type step struct {
		name         string
		layer        mapsv1.MapLayer
		value        int32
		cells        [][2]int32
		changed      int32
		masterMoves  int32
		playerMoves  int32
		wantDoorsAt  map[[2]int]grid.Door
		wantWallsAt  [][2]int
		wantLightsAt map[[2]int]grid.Light
	}
	steps := []step{
		{name: "a wall", layer: mapsv1.MapLayer_MAP_LAYER_WALL, value: 1, cells: [][2]int32{{1, 1}, {3, 3}, {1, 1}}, changed: 2, masterMoves: 1, playerMoves: 1, wantWallsAt: [][2]int{{1, 1}, {3, 3}}},
		{name: "the same wall again", layer: mapsv1.MapLayer_MAP_LAYER_WALL, value: 1, cells: [][2]int32{{1, 1}}, changed: 0},
		{name: "a light", layer: mapsv1.MapLayer_MAP_LAYER_LIGHT, value: 3, cells: [][2]int32{{5, 5}, {5, 6}}, changed: 2, masterMoves: 1, wantLightsAt: map[[2]int]grid.Light{{5, 5}: grid.Bright, {5, 6}: grid.Bright}},
		{
			name: "a closed door, one square paints its block of four", layer: mapsv1.MapLayer_MAP_LAYER_DOORS, value: int32(mapsv1.DoorState_DOOR_STATE_CLOSED), cells: [][2]int32{{3, 3}}, changed: 4, masterMoves: 1, playerMoves: 1,
			wantDoorsAt: map[[2]int]grid.Door{{2, 2}: grid.DoorClosed, {3, 2}: grid.DoorClosed, {2, 3}: grid.DoorClosed, {3, 3}: grid.DoorClosed, {4, 4}: grid.DoorNone},
		},
		{
			name: "locking it changes nothing a player reads", layer: mapsv1.MapLayer_MAP_LAYER_DOORS, value: int32(mapsv1.DoorState_DOOR_STATE_LOCKED), cells: [][2]int32{{2, 2}}, changed: 4, masterMoves: 1,
			wantDoorsAt: map[[2]int]grid.Door{{2, 2}: grid.DoorLocked, {3, 3}: grid.DoorLocked},
		},
		{
			name: "a secret door over a wall: the wall stays a wall", layer: mapsv1.MapLayer_MAP_LAYER_DOORS, value: int32(mapsv1.DoorState_DOOR_STATE_SECRET), cells: [][2]int32{{0, 0}}, changed: 4, masterMoves: 1, playerMoves: 1,
			wantDoorsAt: map[[2]int]grid.Door{{0, 0}: grid.DoorSecret, {1, 1}: grid.DoorSecret},
		},
		{
			name: "a door erased", layer: mapsv1.MapLayer_MAP_LAYER_DOORS, value: 0, cells: [][2]int32{{2, 3}}, changed: 4, masterMoves: 1, playerMoves: 1,
			wantDoorsAt: map[[2]int]grid.Door{{2, 2}: grid.DoorNone, {3, 3}: grid.DoorNone},
		},
		{name: "terrain is square by square", layer: mapsv1.MapLayer_MAP_LAYER_DIFFICULT_TERRAIN, value: 1, cells: [][2]int32{{7, 7}}, changed: 1, masterMoves: 1, playerMoves: 1},
		{name: "cover is square by square", layer: mapsv1.MapLayer_MAP_LAYER_COVER, value: 2, cells: [][2]int32{{8, 8}, {9, 8}}, changed: 2, masterMoves: 1, playerMoves: 1},
	}
	for _, st := range steps {
		got := m.mustPaint(s.campaign, s.mapID, st.layer, st.value, st.cells...)
		base += st.masterMoves
		playerBase += st.playerMoves
		if got.GetChanged() != st.changed || got.GetLayersRevision() != base {
			t.Errorf("%s: answered changed %d, revision %d; want %d and %d", st.name, got.GetChanged(), got.GetLayersRevision(), st.changed, base)
		}
		if r := rev(s.ana); r != playerBase {
			t.Errorf("%s: the player reads revision %d, want %d", st.name, r, playerBase)
		}
		res := m.mustLayers(s.campaign, s.mapID)
		if res.GetGridColumns() != 16 || res.GetGridRows() != 12 || res.GetLayersRevision() != base {
			t.Errorf("%s: the master reads %d x %d at revision %d", st.name, res.GetGridColumns(), res.GetGridRows(), res.GetLayersRevision())
		}
		d := decodeOn(t, gr, res)
		for at, want := range st.wantDoorsAt {
			if d.doors.Get(at[0], at[1]) != want {
				t.Errorf("%s: door at %v = %d, want %d", st.name, at, d.doors.Get(at[0], at[1]), want)
			}
		}
		for _, at := range st.wantWallsAt {
			if !d.walls.Get(at[0], at[1]) {
				t.Errorf("%s: no wall at %v", st.name, at)
			}
		}
		for at, want := range st.wantLightsAt {
			if d.light.Get(at[0], at[1]) != want {
				t.Errorf("%s: light at %v = %d, want %d", st.name, at, d.light.Get(at[0], at[1]), want)
			}
		}
	}
	// What a player reads of it at the end: the secret door is a wall, and no light.
	p := decodeOn(t, gr, s.ana.mustLayers(s.campaign, s.mapID))
	if !p.walls.Get(0, 0) || !p.walls.Get(1, 1) || p.doors.Get(0, 0) != grid.DoorNone || len(s.ana.mustLayers(s.campaign, s.mapID).GetLight()) != 0 {
		t.Errorf("the player reads the secret door as door %d, wall %v", p.doors.Get(0, 0), p.walls.Get(0, 0))
	}
}

// TestLayersFrozen_FogSettingsAnswerAndHints: SetMapFog answers the master's map after the
// change, tells the players only what they read (the switch and the party's vision, never the
// base light) and only while they see the map; the master hears of every call.
func TestLayersFrozen_FogSettingsAnswerAndHints(t *testing.T) {
	t.Parallel()
	s := newScenes(t, true)
	m := s.master
	m.mustSetGrid(s.campaign, s.mapID, 20)
	hiddenMap := m.createMap(s.campaign, "Escondido", m.newImage(s.campaign))
	m.mustSetGrid(s.campaign, hiddenMap.GetId(), 20)
	anaWatch, masterWatch := s.ana.watch(s.campaign), m.watch(s.campaign)
	bright, dim := mapsv1.LightLevel_LIGHT_LEVEL_BRIGHT.Enum(), mapsv1.LightLevel_LIGHT_LEVEL_DIM.Enum()
	for _, st := range []struct {
		name     string
		mapID    string
		edit     func(*mapsv1.SetMapFogRequest)
		fog      bool
		group    bool
		base     mapsv1.LightLevel
		anaHears bool
	}{
		{"the base light is the master's", s.mapID, func(r *mapsv1.SetMapFogRequest) { r.BaseLight = bright }, false, false, mapsv1.LightLevel_LIGHT_LEVEL_BRIGHT, false},
		{"the party's vision is read", s.mapID, func(r *mapsv1.SetMapFogRequest) { r.GroupVision = new(true) }, false, true, mapsv1.LightLevel_LIGHT_LEVEL_BRIGHT, true},
		{"the switch is read", s.mapID, func(r *mapsv1.SetMapFogRequest) { r.FogEnabled = new(true) }, true, true, mapsv1.LightLevel_LIGHT_LEVEL_BRIGHT, true},
		{"the same switch is no news", s.mapID, func(r *mapsv1.SetMapFogRequest) { r.FogEnabled = new(true) }, true, true, mapsv1.LightLevel_LIGHT_LEVEL_BRIGHT, false},
		{"all three at once", s.mapID, func(r *mapsv1.SetMapFogRequest) {
			r.FogEnabled, r.GroupVision, r.BaseLight = new(false), new(false), dim
		}, false, false, mapsv1.LightLevel_LIGHT_LEVEL_DIM, true},
		{"a map the players do not see", hiddenMap.GetId(), func(r *mapsv1.SetMapFogRequest) { r.FogEnabled = new(true); r.GroupVision = new(true) }, true, true, mapsv1.LightLevel_LIGHT_LEVEL_DARK, false},
	} {
		req := &mapsv1.SetMapFogRequest{CampaignId: s.campaign, MapId: st.mapID}
		st.edit(req)
		res, err := m.maps.SetMapFog(t.Context(), connect.NewRequest(req))
		if err != nil {
			t.Fatalf("%s: SetMapFog() error = %v", st.name, err)
		}
		got := res.Msg.GetMap()
		if got.GetId() != st.mapID || got.GetFogEnabled() != st.fog || got.GetGroupVision() != st.group || got.GetBaseLight() != st.base {
			t.Errorf("%s: answered %v, want fog %v, group %v, base light %v", st.name, got, st.fog, st.group, st.base)
		}
		probe := m.createMap(s.campaign, "Sonda "+st.name, m.newImage(s.campaign)).GetId()
		s.probe(probe)
		// A probe's own events may trail the drain by one: only the map of the call counts.
		var masterEvents []*playv1.WatchGameSessionResponse
		for _, ev := range masterWatch.drain(probe) {
			if ev.GetMapChanged().GetMapId() == st.mapID {
				masterEvents = append(masterEvents, ev)
			}
		}
		anaEvents := anaWatch.drain(probe)
		if len(masterEvents) != 1 || masterEvents[0].GetMapChanged().GetMapId() != st.mapID {
			t.Errorf("%s: the master heard %v, want one map_changed", st.name, masterEvents)
		}
		if heard := len(anaEvents) == 1 && anaEvents[0].GetMapChanged().GetMapId() == st.mapID; heard != st.anaHears || len(anaEvents) > 1 || (!st.anaHears && len(anaEvents) != 0) {
			t.Errorf("%s: Ana heard %v, want hears %v", st.name, anaEvents, st.anaHears)
		}
	}
}

// TestLayersFrozen_OpenDoors: the doors a move opens, in the transaction of the move: the
// refusals, a map with no grid or nothing painted, a calibrated map (one square comes back for
// a block), repeated squares and squares outside the grid.
func TestLayersFrozen_OpenDoors(t *testing.T) {
	t.Parallel()
	s := newScenes(t, true)
	m := s.master
	sq := func(c, r int) grid.Square { return grid.Square{Col: c, Row: r} }
	open := func(campaign, mapID string, sqs ...grid.Square) ([]grid.Square, error) {
		var opened []grid.Square
		err := db.InTx(t.Context(), s.h.pool, func(tx pgx.Tx) error {
			var err error
			opened, err = s.h.svc.OpenDoors(t.Context(), tx, campaign, mapID, sqs)
			return err
		})
		return opened, err
	}
	for name, c := range map[string]struct{ campaign, mapID string }{
		"not an id": {s.campaign, "x"}, "no such map": {s.campaign, newKey()}, "another campaign": {newKey(), s.mapID},
	} {
		if opened, err := open(c.campaign, c.mapID, sq(1, 1)); layersShapeOf(err) != "not_found|not_found: map not found" || opened != nil {
			t.Errorf("%s: %v, %v; want map not found", name, opened, err)
		}
	}
	// No grid, a grid with nothing painted: nothing to open, no error.
	if opened, err := open(s.campaign, s.mapID, sq(1, 1)); opened != nil || err != nil {
		t.Errorf("a map with no grid: %v, %v", opened, err)
	}
	m.mustSetCalibration(s.campaign, s.mapID, 8, 2)
	if opened, err := open(s.campaign, s.mapID, sq(1, 1)); opened != nil || err != nil {
		t.Errorf("a grid with nothing painted: %v, %v", opened, err)
	}
	closed := int32(mapsv1.DoorState_DOOR_STATE_CLOSED)
	m.mustPaint(s.campaign, s.mapID, mapsv1.MapLayer_MAP_LAYER_DOORS, closed, [2]int32{2, 2}, [2]int32{6, 6})
	m.mustPaint(s.campaign, s.mapID, mapsv1.MapLayer_MAP_LAYER_DOORS, int32(mapsv1.DoorState_DOOR_STATE_LOCKED), [2]int32{10, 10})
	before := m.mustLayers(s.campaign, s.mapID).GetLayersRevision()

	// A square of a block opens the four; the second square of the same block finds it open.
	// Squares off the grid and squares with no door are left out.
	opened, err := open(s.campaign, s.mapID, sq(3, 3), sq(2, 2), sq(99, 99), sq(-1, 0), sq(10, 10), sq(0, 0), sq(7, 7))
	if err != nil || len(opened) != 2 || opened[0] != sq(3, 3) || opened[1] != sq(7, 7) {
		t.Fatalf("OpenDoors() = %v, %v; want the squares (3,3) and (7,7), one for each block", opened, err)
	}
	d := decodeOn(t, grid.Grid{Columns: 16, Rows: 12}, m.mustLayers(s.campaign, s.mapID)).doors
	for _, at := range [][2]int{{2, 2}, {3, 2}, {2, 3}, {3, 3}, {6, 6}, {7, 7}} {
		if d.Get(at[0], at[1]) != grid.DoorOpen {
			t.Errorf("door at %v = %d, want open", at, d.Get(at[0], at[1]))
		}
	}
	if d.Get(10, 10) != grid.DoorLocked {
		t.Errorf("the locked door became %d", d.Get(10, 10))
	}
	if after := m.mustLayers(s.campaign, s.mapID).GetLayersRevision(); after != before+1 {
		t.Errorf("revision %d after opening two blocks, want %d", after, before+1)
	}
}

// TestLayersFrozen_CarriedLightAnswers: the token SetCarriedLight answers to each caller, and the
// order of the checks that find the token: a hidden token is no token to a player on a map with no
// fog, and is there on a fog map.
func TestLayersFrozen_CarriedLightAnswers(t *testing.T) {
	t.Parallel()
	s := newScenes(t, true)
	m := s.master
	m.mustSetGrid(s.campaign, s.mapID, 20)
	m.placeToken(s.campaign, s.mapID, s.pens.GetId(), 3000, 3000)
	carry := func(u *user, key string) (*mapsv1.MapToken, error) {
		res, err := u.maps.SetCarriedLight(t.Context(), connect.NewRequest(&mapsv1.SetCarriedLightRequest{CampaignId: s.campaign, MapId: s.mapID, CharacterId: s.pens.GetId(), LightKey: key}))
		if err != nil {
			return nil, err
		}
		return res.Msg.GetToken(), nil
	}
	m.setTokenHidden(s.campaign, s.mapID, s.pens.GetId(), true)
	if tok, err := carry(s.ana, "light:torch"); layersShapeOf(err) != "not_found|not_found: token not found" || tok != nil {
		t.Errorf("a player's light on a hidden token with no fog: %v, %v", tok, err)
	}
	tok, err := carry(m, "light:torch")
	if err != nil || tok.GetCarriedLight() != "light:torch" || !tok.GetHidden() || tok.GetMine() || tok.GetName() != s.pens.GetName() {
		t.Errorf("the master's light on a hidden token = %v, %v; want the key, hidden, not his", tok, err)
	}
	if _, err := m.maps.SetMapFog(t.Context(), connect.NewRequest(&mapsv1.SetMapFogRequest{CampaignId: s.campaign, MapId: s.mapID, FogEnabled: new(true)})); err != nil {
		t.Fatal(err)
	}
	tok, err = carry(s.ana, "light:hooded-lantern")
	if err != nil || tok.GetCarriedLight() != "light:hooded-lantern" || !tok.GetHidden() || !tok.GetMine() || tok.GetCharacterId() != s.pens.GetId() || tok.GetKind() != charactersv1.CharacterKind_CHARACTER_KIND_PLAYER {
		t.Errorf("Ana's light on her hidden token on a fog map = %v, %v", tok, err)
	}
	if tok, err = carry(s.ana, ""); err != nil || tok.GetCarriedLight() != "" {
		t.Errorf("putting it out = %v, %v", tok, err)
	}
}
