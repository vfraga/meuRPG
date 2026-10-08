package maps

import (
	"context"
	"errors"
	"net/http"
	"slices"
	"strings"
	"sync"
	"testing"
	"time"

	"connectrpc.com/connect"
	"github.com/jackc/pgx/v5"
	"google.golang.org/protobuf/proto"

	charactersv1 "github.com/PuraFome/meuRPG/backend/gen/meurpg/characters/v1"
	mapsv1 "github.com/PuraFome/meuRPG/backend/gen/meurpg/maps/v1"
	playv1 "github.com/PuraFome/meuRPG/backend/gen/meurpg/play/v1"
	rulesv1 "github.com/PuraFome/meuRPG/backend/gen/meurpg/rules/v1"
	"github.com/PuraFome/meuRPG/backend/internal/maps/mapsdb"
	"github.com/PuraFome/meuRPG/backend/internal/platform/db"
	"github.com/PuraFome/meuRPG/backend/internal/rules/grid"
)

// The fog of war on the server (MR-036, RN-10, Etapa 9, D6): what each player
// receives on a map with the fog on. The map is "A caverna do Vale Seco" of the
// design (design/etapa9/cave.py and cave-data.md): 24 x 16 squares, walls, the
// guard room's torch, four characters and three goblins. Every player's response
// is read as the app's JSON (protojson), never as a Go struct, so a field that
// leaks shows up whatever its name.

var caveWalls = []string{
	"########################",
	"########################",
	"################.......#",
	"################.......#",
	"################....q..#",
	"################.......#",
	"#......#########.......#",
	"...................h...#",
	"...................h...#",
	"#...::.#..######.......#",
	"#...::.#..##############",
	"######........##########",
	"######........##########",
	"######........##########",
	"######........##########",
	"########################",
}

var caveGrid = grid.Grid{Columns: 24, Rows: 16}

// at is a square's center, in basis points.
func at(col, row int) (x, y int32) {
	xBP, yBP := caveGrid.CenterOf(grid.Square{Col: col, Row: row})
	return int32(xBP), int32(yBP) //nolint:gosec // G115: at most 10000
}

// pc creates a player's character of a race (a level 1 wizard).
func (u *user) pc(campaignID, name, race string) *charactersv1.Character {
	u.h.t.Helper()
	sheet := &charactersv1.CharacterSheet{Content: &charactersv1.CharacterSheet_Full{Full: &charactersv1.FullSheet{
		BaseScores: &rulesv1.AbilityScores{Strength: 10, Dexterity: 14, Constitution: 12, Intelligence: 16, Wisdom: 10, Charisma: 8},
		RaceKey:    race,
		Classes:    []*charactersv1.ClassLevel{{ClassKey: "class:wizard", Level: 1}},
	}}}
	res, err := u.characters.CreateCharacter(u.h.t.Context(), connect.NewRequest(&charactersv1.CreateCharacterRequest{
		CampaignId: campaignID, Kind: charactersv1.CharacterKind_CHARACTER_KIND_PLAYER, Name: name, Sheet: sheet,
	}))
	if err != nil {
		u.h.t.Fatalf("CreateCharacter(%s) error = %v", name, err)
	}
	return res.Msg.GetCharacter()
}

// cave is the table of these tests.
type cave struct {
	h                          *harness
	master                     *user
	ana, caio, bia, dani       *user
	campaign, mapID, imageID   string
	pens, toren, brisa, salvia *charactersv1.Character
	goblin1, goblin2, captain  *charactersv1.Character
	hiddenGoblin               *charactersv1.Character
	entrance, guardhouse       *mapsv1.MapPoint
	chest, torch               *mapsv1.MapPoint
	probeMap                   string
}

// The squares of the table (cave.py: PARTY, GOBLINS).
var (
	sqPens, sqToren, sqBrisa, sqSalvia = grid.Square{Col: 5, Row: 8}, grid.Square{Col: 6, Row: 7}, grid.Square{Col: 4, Row: 7}, grid.Square{Col: 3, Row: 8}
	sqGoblin1, sqGoblin2, sqCaptain    = grid.Square{Col: 18, Row: 5}, grid.Square{Col: 20, Row: 7}, grid.Square{Col: 21, Row: 3}
)

// newCave builds the cave with the fog on, the four characters at their places,
// and a session open: Pensantus (gnome, darkvision 18 m), Toren (human), Brisa
// (halfling) and Sálvia (half-elf, darkvision 18 m); two visible goblins, a
// third hidden by the master, and the captain, in the guard room; the guard
// room's torch.
func newCave(t *testing.T, configure ...func(*Config)) *cave {
	t.Helper()
	return newCaveWith(t, true, configure...)
}

// newCaveWith is newCave; with visible false the map stays hidden from the
// players and is not the session's current map (the master is preparing it).
func newCaveWith(t *testing.T, visible bool, configure ...func(*Config)) *cave {
	t.Helper()
	h := newHarness(t, configure...)
	c := &cave{h: h, master: h.newUser("Mestre"), ana: h.newUser("Ana"), caio: h.newUser("Caio"), bia: h.newUser("Bia"), dani: h.newUser("Dani")}
	m := c.master
	c.campaign = h.newCampaign(m, c.ana, c.caio, c.bia, c.dani)
	c.imageID = m.mustUpload(c.campaign, "caverna.png", patternImage(t, 240, 160)).GetId()
	c.mapID = m.createMap(c.campaign, "A caverna do Vale Seco", c.imageID).GetId()
	if visible {
		m.setMapRevealed(c.campaign, c.mapID, true)
	}
	m.mustSetGrid(c.campaign, c.mapID, 24)

	var walls, rubble, crates, column [][2]int32
	for row, line := range caveWalls {
		for col, ch := range line {
			switch ch {
			case '#':
				walls = append(walls, [2]int32{int32(col), int32(row)}) //nolint:gosec // G115: a cave of 24 x 16
			case ':':
				rubble = append(rubble, [2]int32{int32(col), int32(row)}) //nolint:gosec // G115: a cave of 24 x 16
			case 'h':
				crates = append(crates, [2]int32{int32(col), int32(row)}) //nolint:gosec // G115: a cave of 24 x 16
			case 'q':
				column = append(column, [2]int32{int32(col), int32(row)}) //nolint:gosec // G115: a cave of 24 x 16
			}
		}
	}
	m.mustPaint(c.campaign, c.mapID, mapsv1.MapLayer_MAP_LAYER_WALL, 1, walls...)
	m.mustPaint(c.campaign, c.mapID, mapsv1.MapLayer_MAP_LAYER_DIFFICULT_TERRAIN, 1, rubble...)
	m.mustPaint(c.campaign, c.mapID, mapsv1.MapLayer_MAP_LAYER_COVER, 1, crates...)
	m.mustPaint(c.campaign, c.mapID, mapsv1.MapLayer_MAP_LAYER_COVER, 2, column...)

	point := func(kind mapsv1.MapPointKind, name string, col, row int, edit func(*mapsv1.CreateMapPointRequest)) *mapsv1.MapPoint {
		x, y := at(col, row)
		req := &mapsv1.CreateMapPointRequest{CampaignId: c.campaign, MapId: c.mapID, Kind: kind, Name: name, Description: "descrição de " + name, XBp: x, YBp: y}
		if edit != nil {
			edit(req)
		}
		return m.createPoint(req)
	}
	c.entrance = m.setPointRevealed(c.campaign, point(mapsv1.MapPointKind_MAP_POINT_KIND_SCENE, "Entrada da caverna", 2, 7, nil), true)
	c.guardhouse = m.setPointRevealed(c.campaign, point(mapsv1.MapPointKind_MAP_POINT_KIND_SCENE, "A guarita", 19, 3, nil), true)
	c.chest = m.setPointRevealed(c.campaign, point(mapsv1.MapPointKind_MAP_POINT_KIND_TREASURE, "Baú de moedas", 12, 13, func(r *mapsv1.CreateMapPointRequest) {
		r.TreasureValuePo = proto.Int32(250)
	}), true)
	c.torch = point(mapsv1.MapPointKind_MAP_POINT_KIND_LIGHT, "Tocha da guarita", 19, 4, func(r *mapsv1.CreateMapPointRequest) {
		r.Light = &mapsv1.LightSpec{PresetKey: "light:torch"}
	})

	if _, err := m.maps.SetMapFog(t.Context(), connect.NewRequest(&mapsv1.SetMapFogRequest{CampaignId: c.campaign, MapId: c.mapID, FogEnabled: new(true)})); err != nil {
		t.Fatalf("SetMapFog(on) error = %v", err)
	}
	m.start(c.campaign)
	if visible {
		if _, err := m.setCurrentMap(c.campaign, c.mapID); err != nil {
			t.Fatalf("SetCurrentMap() error = %v", err)
		}
	}

	c.pens = c.ana.pc(c.campaign, "Pensantus", "race:gnome")
	c.toren = c.caio.pc(c.campaign, "Toren", "race:human")
	c.brisa = c.bia.pc(c.campaign, "Brisa", "race:halfling")
	c.salvia = c.dani.pc(c.campaign, "Sálvia", "race:half-elf")
	npc := func(name string, sq grid.Square, visible bool) *charactersv1.Character {
		ch := m.createCharacter(c.campaign, charactersv1.CharacterKind_CHARACTER_KIND_MINION, name)
		m.placeAt(c.campaign, c.mapID, ch.GetId(), sq)
		if visible {
			m.setTokenHidden(c.campaign, c.mapID, ch.GetId(), false)
		}
		return ch
	}
	for _, p := range []struct {
		ch *charactersv1.Character
		sq grid.Square
	}{{c.pens, sqPens}, {c.toren, sqToren}, {c.brisa, sqBrisa}, {c.salvia, sqSalvia}} {
		m.placeAt(c.campaign, c.mapID, p.ch.GetId(), p.sq)
	}
	c.goblin1 = npc("Goblin 1", sqGoblin1, true)
	c.goblin2 = npc("Goblin 2", sqGoblin2, true)
	c.captain = npc("Capitão Goblin", sqCaptain, true)
	c.hiddenGoblin = npc("Goblin Emboscado", grid.Square{Col: 20, Row: 8}, false)
	c.probeMap = m.createMap(c.campaign, "Sonda", m.newImage(c.campaign)).GetId()
	return c
}

// placeAt places a token on a square's center.
func (u *user) placeAt(campaignID, mapID, characterID string, sq grid.Square) *mapsv1.MapToken {
	u.h.t.Helper()
	x, y := at(sq.Col, sq.Row)
	return u.placeToken(campaignID, mapID, characterID, x, y)
}

// vision calls GetMapVision as u; as is the "Ver como" character, if any.
func (u *user) vision(campaignID, mapID string, as ...string) (*mapsv1.GetMapVisionResponse, error) {
	req := &mapsv1.GetMapVisionRequest{CampaignId: campaignID, MapId: mapID}
	if len(as) > 0 {
		req.AsCharacterId = as[0]
	}
	res, err := u.maps.GetMapVision(u.h.t.Context(), connect.NewRequest(req))
	if err != nil {
		return nil, err
	}
	return res.Msg, nil
}

func (u *user) mustVision(campaignID, mapID string, as ...string) *mapsv1.GetMapVisionResponse {
	u.h.t.Helper()
	res, err := u.vision(campaignID, mapID, as...)
	if err != nil {
		u.h.t.Fatalf("GetMapVision() error = %v", err)
	}
	return res
}

// codes unpacks a view's squares: four bits each, the low half of the byte first
// (GetMapVisionResponse).
func codes(t *testing.T, res *mapsv1.GetMapVisionResponse) []byte {
	t.Helper()
	n := int(res.GetGridColumns() * res.GetGridRows())
	if len(res.GetStates()) != (n+1)/2 {
		t.Fatalf("states = %d bytes for %d squares, want %d", len(res.GetStates()), n, (n+1)/2)
	}
	out := make([]byte, n)
	for i := range out {
		out[i] = res.GetStates()[i/2] >> (4 * (i % 2)) & 0xf
	}
	return out
}

// picture draws a view as the oracle does: B bright, d dim, g seen in grey, # a
// wall seen, r remembered, @ the viewer's own square, a space unseen.
func picture(t *testing.T, res *mapsv1.GetMapVisionResponse, own grid.Square) []string {
	t.Helper()
	cs := codes(t, res)
	var rows []string
	for row := range int(res.GetGridRows()) {
		var b strings.Builder
		for col := range int(res.GetGridColumns()) {
			ch := byte(' ')
			switch cs[row*int(res.GetGridColumns())+col] {
			case 4:
				ch = 'B'
			case 3:
				ch = 'd'
			case 2:
				ch = 'g'
			case 1:
				ch = '#'
			case 5:
				ch = 'r'
			}
			if (grid.Square{Col: col, Row: row}) == own {
				ch = '@'
			}
			b.WriteByte(ch)
		}
		rows = append(rows, b.String())
	}
	return rows
}

func diffRows(t *testing.T, name string, got, want []string) {
	t.Helper()
	for row := range want {
		if row >= len(got) || got[row] != want[row] {
			g := ""
			if row < len(got) {
				g = got[row]
			}
			t.Errorf("%s: row %d\n got  %q\n want %q", name, row, g, want[row])
		}
	}
}

// bitOf reads a square of a one-bit layer.
func bitOf(b []byte, g grid.Grid, col, row int) bool {
	n := row*g.Columns + col
	return n/8 < len(b) && b[n/8]>>(n%8)&1 == 1
}

// sees lists the names of the tokens a player received.
func tokenNames(res *mapsv1.GetMapResponse) []string {
	var out []string
	for _, tok := range res.GetTokens() {
		out = append(out, tok.GetName())
	}
	slices.Sort(out)
	return out
}

// RN-10: a player receives only what their character sees. Every response of
// Ana's (Pensantus), Caio's (Toren), Bia's (Brisa) and Dani's (Sálvia) is read as
// JSON: no image id, URL or thumbnail, no NPC on a square they do not see, no
// point outside what they see or remember, no light, no wall far from them, and
// the counts are of what they receive.
func TestRN10_FogPlayersReceiveOnlyWhatTheirCharacterSees(t *testing.T) {
	t.Parallel()
	c := newCave(t)
	ctx := t.Context()

	forbiddenEverywhere := []string{
		c.imageID, "/images/", "thumbnailUrl",
		c.goblin1.GetId(), "Goblin 1", c.captain.GetId(), "Capitão", c.hiddenGoblin.GetId(), "Emboscado",
		c.guardhouse.GetId(), "A guarita", c.chest.GetId(), "Baú de moedas", c.torch.GetId(), "Tocha da guarita",
	}
	for _, p := range []struct {
		who  *user
		name string
		// what the player may receive: Goblin 2 is in the corridor's light for all
		// of them; the entrance is seen only with darkvision.
		tokens []string
		points []string
	}{
		{c.ana, "Ana", []string{"Brisa", "Goblin 2", "Pensantus", "Sálvia", "Toren"}, []string{c.entrance.GetId()}},
		{c.caio, "Caio", []string{"Brisa", "Goblin 2", "Pensantus", "Sálvia", "Toren"}, nil},
		{c.bia, "Bia", []string{"Brisa", "Goblin 2", "Pensantus", "Sálvia", "Toren"}, nil},
		{c.dani, "Dani", []string{"Brisa", "Goblin 2", "Pensantus", "Sálvia", "Toren"}, []string{c.entrance.GetId()}},
	} {
		got := p.who.mustGetMap(c.campaign, c.mapID)
		forbidden := slices.Clone(forbiddenEverywhere)
		if len(p.points) == 0 {
			forbidden = append(forbidden, c.entrance.GetId(), "Entrada da caverna")
		}
		responses := map[string]string{"GetMap": asJSON(t, got)}
		responses["ListMaps"] = asJSON(t, &mapsv1.ListMapsResponse{Maps: p.who.listMaps(c.campaign)})
		layers := p.who.mustLayers(c.campaign, c.mapID)
		responses["GetMapLayers"] = asJSON(t, layers)
		responses["GetMapVision"] = asJSON(t, p.who.mustVision(c.campaign, c.mapID))
		live, err := p.who.play.GetLiveSession(ctx, connect.NewRequest(&playv1.GetLiveSessionRequest{CampaignId: c.campaign}))
		if err != nil {
			t.Fatal(err)
		}
		responses["GetLiveSession"] = asJSON(t, live.Msg)
		for call, body := range responses {
			// The one path to an image a player gets is their own tiles (9.5, tiles_test.go).
			body = strings.ReplaceAll(body, TilesPath+c.mapID+"/tiles/", "")
			for _, bad := range forbidden {
				if strings.Contains(body, bad) {
					t.Errorf("%s: %s's %s has %q:\n%s", t.Name(), p.name, call, bad, body)
				}
			}
		}
		if !got.GetMap().GetImageWithheld() || got.GetMap().GetImage().GetId() != "" || got.GetMap().GetImage().GetUrl() != "" ||
			got.GetMap().GetImage().GetWidth() != 240 {
			t.Errorf("%s's map image = %v, want withheld, only its size", p.name, got.GetMap().GetImage())
		}
		if names := tokenNames(got); !slices.Equal(names, p.tokens) {
			t.Errorf("%s's tokens = %v, want %v", p.name, names, p.tokens)
		}
		if ids := pointIDs(got); !slices.Equal(ids, p.points) || int(got.GetMap().GetPointCount()) != len(p.points) {
			t.Errorf("%s's points = %v (count %d), want %v", p.name, ids, got.GetMap().GetPointCount(), p.points)
		}
		if listed := p.who.listMaps(c.campaign)[0]; int(listed.GetPointCount()) != len(p.points) || !listed.GetImageWithheld() {
			t.Errorf("%s's ListMaps entry: count %d, withheld %v; want %d, withheld", p.name, listed.GetPointCount(), listed.GetImageWithheld(), len(p.points))
		}
		// The layers: never the light; no wall that is far from what they see.
		if len(layers.GetLight()) != 0 || !layers.GetFogWithheld() {
			t.Errorf("%s's layers: %d light bytes, filtered %v; want none, filtered", p.name, len(layers.GetLight()), layers.GetFogWithheld())
		}
		if bitOf(layers.GetWall(), caveGrid, 0, 0) || bitOf(layers.GetWall(), caveGrid, 23, 15) || bitOf(layers.GetWall(), caveGrid, 16, 2) {
			t.Errorf("%s's walls hold a wall far from anything they see", p.name)
		}
		if bitOf(layers.GetCover(), caveGrid, 20, 4) || len(layers.GetCover()) != 0 && grid.LayerSize(caveGrid) < 0 {
			t.Errorf("%s's cover has the column of the guard room, which nobody sees", p.name)
		}
	}

	// Ana sees the walls around the entrance cave, and the rubble where she looks.
	layers := c.ana.mustLayers(c.campaign, c.mapID)
	if !bitOf(layers.GetWall(), caveGrid, 7, 6) || !bitOf(layers.GetWall(), caveGrid, 0, 5) || !bitOf(layers.GetDifficultTerrain(), caveGrid, 4, 9) {
		t.Errorf("Ana does not read the walls and the rubble of the room she sees")
	}
	// Toren, in the dark with no darkvision, does not read the rubble she sees.
	if bitOf(c.caio.mustLayers(c.campaign, c.mapID).GetDifficultTerrain(), caveGrid, 4, 9) {
		t.Errorf("Caio reads the rubble of a room he does not see")
	}

	// The image route refuses a player the image of a fog map; the master gets it.
	for _, path := range []string{"/images/" + c.imageID, "/images/" + c.imageID + "/thumb"} {
		if res := c.ana.get(path); res.status != http.StatusNotFound {
			t.Errorf("Ana GET %s = %d, want 404", path, res.status)
		}
		if res := c.master.get(path); res.status != http.StatusOK {
			t.Errorf("the master GET %s = %d, want 200", path, res.status)
		}
	}

	// The master's reads do not change: everything, with the light and the image.
	master := c.master.mustGetMap(c.campaign, c.mapID)
	if master.GetMap().GetImage().GetId() != c.imageID || master.GetMap().GetImageWithheld() || len(master.GetTokens()) != 8 || master.GetMap().GetPointCount() != 4 {
		t.Errorf("the master's map = %v, want the whole map", master)
	}
	if len(c.master.mustLayers(c.campaign, c.mapID).GetLight()) != 0 {
		t.Errorf("no light was painted, but the master's layer has bytes")
	}
}

// The views match the oracle (cave.py, cave-data.md): Pensantus's, Toren's,
// Brisa's and Sálvia's states, in the dark, and each with Toren's torch.
func TestMR036_FogViewsMatchTheCaveOracle(t *testing.T) {
	t.Parallel()
	c := newCave(t)
	viewers := []struct {
		who *user
		ch  *charactersv1.Character
		own grid.Square
		key string
	}{
		{c.ana, c.pens, sqPens, "pensantus"},
		{c.caio, c.toren, sqToren, "toren"},
		{c.bia, c.brisa, sqBrisa, "brisa"},
		{c.dani, c.salvia, sqSalvia, "salvia"},
	}
	for _, v := range viewers {
		res := v.who.mustVision(c.campaign, c.mapID)
		if !res.GetCharacterOnMap() || !res.GetFogEnabled() || res.GetGroupVision() {
			t.Errorf("%s: character on map %v, fog %v, group %v; want on the map, fog, no group", v.key, res.GetCharacterOnMap(), res.GetFogEnabled(), res.GetGroupVision())
		}
		diffRows(t, v.key, picture(t, res, v.own), caveOracle[v.key])
	}

	// Toren lights his torch: his view and everyone else's change as the oracle's.
	if _, err := c.caio.maps.SetCarriedLight(t.Context(), connect.NewRequest(&mapsv1.SetCarriedLightRequest{
		CampaignId: c.campaign, MapId: c.mapID, CharacterId: c.toren.GetId(), LightKey: "light:torch",
	})); err != nil {
		t.Fatal(err)
	}
	for _, v := range viewers {
		diffRows(t, v.key+" with the torch", picture(t, v.who.mustVision(c.campaign, c.mapID), v.own), caveOracle[v.key+"-tocha"])
	}
	// What Ana receives of the torch is only its light, not who carries it.
	if body := asJSON(t, c.ana.mustGetMap(c.campaign, c.mapID)); strings.Contains(body, "light:torch") {
		t.Errorf("Ana's map names Toren's light: %s", body)
	}
}

// "Visão do grupo": every player sees the union of what the four characters see.
func TestMR036_GroupVisionIsTheUnion(t *testing.T) {
	t.Parallel()
	c := newCave(t)
	viewers := []struct {
		who *user
		key string
	}{{c.ana, "pensantus"}, {c.caio, "toren"}, {c.bia, "brisa"}, {c.dani, "salvia"}}
	var states [][]byte
	for _, v := range viewers {
		states = append(states, codes(t, v.who.mustVision(c.campaign, c.mapID)))
	}
	// "Visão do grupo": everyone sees the union of the four.
	if _, err := c.master.maps.SetMapFog(t.Context(), connect.NewRequest(&mapsv1.SetMapFogRequest{CampaignId: c.campaign, MapId: c.mapID, GroupVision: new(true)})); err != nil {
		t.Fatal(err)
	}
	for _, v := range viewers {
		res := v.who.mustVision(c.campaign, c.mapID)
		got := codes(t, res)
		for n := range got {
			want := byte(0)
			for _, s := range states {
				want = max(want, s[n])
			}
			if got[n] != want {
				t.Fatalf("%s with the group's vision: square %d = %d, want %d (the union)", v.key, n, got[n], want)
			}
		}
		if !res.GetGroupVision() {
			t.Errorf("%s: group_vision = false with the switch on", v.key)
		}
	}
	if _, err := c.master.maps.SetMapFog(t.Context(), connect.NewRequest(&mapsv1.SetMapFogRequest{CampaignId: c.campaign, MapId: c.mapID, GroupVision: new(false)})); err != nil {
		t.Fatal(err)
	}
}

// "Ver como": the master gets exactly what that character's player gets, from
// the same calls; a player may not ask, and an NPC is not a character to see as.
func TestMR036_SeeAsAPlayersCharacter(t *testing.T) {
	t.Parallel()
	c := newCave(t)
	ctx := t.Context()
	for _, p := range []struct {
		who *user
		ch  *charactersv1.Character
	}{{c.ana, c.pens}, {c.caio, c.toren}} {
		as := p.ch.GetId()
		playerMap, masterMap := p.who.mustGetMap(c.campaign, c.mapID), c.master.mustGetMapAs(c.campaign, c.mapID, as)
		if asJSON(t, playerMap) != asJSON(t, masterMap) {
			t.Errorf("GetMap as %s differs from the player's:\n%s\n%s", p.ch.GetName(), asJSON(t, masterMap), asJSON(t, playerMap))
		}
		playerLayers := p.who.mustLayers(c.campaign, c.mapID)
		masterLayers, err := c.master.maps.GetMapLayers(ctx, connect.NewRequest(&mapsv1.GetMapLayersRequest{CampaignId: c.campaign, MapId: c.mapID, AsCharacterId: as}))
		if err != nil || asJSON(t, playerLayers) != asJSON(t, masterLayers.Msg) {
			t.Errorf("GetMapLayers as %s differs from the player's (%v)", p.ch.GetName(), err)
		}
		if asJSON(t, p.who.mustVision(c.campaign, c.mapID)) != asJSON(t, c.master.mustVision(c.campaign, c.mapID, as)) {
			t.Errorf("GetMapVision as %s differs from the player's", p.ch.GetName())
		}
	}
	// Without it, the master sees every square.
	all := c.master.mustVision(c.campaign, c.mapID)
	for n, code := range codes(t, all) {
		if code != 4 {
			t.Fatalf("the master's square %d = %d, want 4 (everything seen)", n, code)
		}
	}
	// A player may not send it, on any of the three calls; an NPC is not a character.
	_, err := c.ana.maps.GetMap(ctx, connect.NewRequest(&mapsv1.GetMapRequest{CampaignId: c.campaign, MapId: c.mapID, AsCharacterId: c.toren.GetId()}))
	wantCode(t, "a player's GetMap as a character", err, connect.CodePermissionDenied)
	_, err = c.ana.maps.GetMapLayers(ctx, connect.NewRequest(&mapsv1.GetMapLayersRequest{CampaignId: c.campaign, MapId: c.mapID, AsCharacterId: c.toren.GetId()}))
	wantCode(t, "a player's GetMapLayers as a character", err, connect.CodePermissionDenied)
	_, err = c.ana.vision(c.campaign, c.mapID, c.pens.GetId())
	wantCode(t, "a player's GetMapVision as their own character", err, connect.CodePermissionDenied)
	_, err = c.master.vision(c.campaign, c.mapID, c.goblin2.GetId())
	wantCode(t, "GetMapVision as an NPC", err, connect.CodeNotFound)
	_, err = c.master.vision(c.campaign, c.mapID, "not-a-uuid")
	wantCode(t, "GetMapVision as an ID that is not a UUID", err, connect.CodeNotFound)
}

func (u *user) mustGetMapAs(campaignID, mapID, characterID string) *mapsv1.GetMapResponse {
	u.h.t.Helper()
	res, err := u.maps.GetMap(u.h.t.Context(), connect.NewRequest(&mapsv1.GetMapRequest{CampaignId: campaignID, MapId: mapID, AsCharacterId: characterID}))
	if err != nil {
		u.h.t.Fatalf("GetMap(as %s) error = %v", characterID, err)
	}
	return res.Msg
}

// memoryRows is how many players' memories of the map are stored.
func (c *cave) memoryRows() int {
	c.h.t.Helper()
	var n int
	if err := c.h.pool.QueryRow(c.h.t.Context(), `SELECT count(*) FROM map_vision_memory WHERE map_id = $1`, c.mapID).Scan(&n); err != nil {
		c.h.t.Fatal(err)
	}
	return n
}

// D6: what was seen stays, darkened and without creatures; "Esquecer o que foi
// visto" clears it, for the master only; a new grid clears it too.
func TestMR036_WhatWasSeenStays(t *testing.T) {
	t.Parallel()
	c := newCave(t)
	m := c.master
	remembered := func(res *mapsv1.GetMapVisionResponse) int {
		n := 0
		for _, code := range codes(t, res) {
			if code == 5 {
				n++
			}
		}
		return n
	}
	if got := remembered(c.ana.mustVision(c.campaign, c.mapID)); got != 0 {
		t.Fatalf("Ana remembers %d squares before she moved", got)
	}
	before := codes(t, c.ana.mustVision(c.campaign, c.mapID))

	// A goblin stands in the entrance cave, in a square Pensantus sees now.
	lurker := m.createCharacter(c.campaign, charactersv1.CharacterKind_CHARACTER_KIND_MINION, "Goblin Furtivo")
	m.placeAt(c.campaign, c.mapID, lurker.GetId(), grid.Square{Col: 3, Row: 10})
	m.setTokenHidden(c.campaign, c.mapID, lurker.GetId(), false)
	if names := tokenNames(c.ana.mustGetMap(c.campaign, c.mapID)); !slices.Contains(names, "Goblin Furtivo") {
		t.Fatalf("Ana does not see the goblin standing in her room: %v", names)
	}

	// Pensantus walks down the corridor: the room is behind a wall now.
	m.placeAt(c.campaign, c.mapID, c.pens.GetId(), grid.Square{Col: 15, Row: 7})
	res := c.ana.mustVision(c.campaign, c.mapID)
	cs := codes(t, res)
	room := 10*24 + 3 // (3, 10)
	if before[room] == 0 || cs[room] != 5 {
		t.Fatalf("the square (3, 10) was %d and is %d, want seen, then remembered (5)", before[room], cs[room])
	}
	// Remembered is every square she saw and does not see now, and nothing else.
	for n := range cs {
		if cs[n] == 5 && before[n] == 0 {
			t.Fatalf("square %d is remembered and was never seen", n)
		}
		if cs[n] == 0 && before[n] != 0 && !slices.Contains([]byte{0}, cs[n]) {
			t.Fatalf("square %d was seen and is forgotten", n)
		}
	}
	// A creature is never remembered: the goblin of the remembered room is not sent,
	// but the points of the remembered part stay listed.
	got := c.ana.mustGetMap(c.campaign, c.mapID)
	if names := tokenNames(got); slices.Contains(names, "Goblin Furtivo") {
		t.Errorf("Ana still receives the goblin of a room she remembers: %v", names)
	}
	// The entrance, behind her now, stays listed as remembered; the guardhouse she
	// looks at from the corridor is seen now, and the chest was never seen.
	if ids := pointIDs(got); !slices.Equal(ids, []string{c.entrance.GetId(), c.guardhouse.GetId()}) {
		t.Errorf("Ana's points = %v, want the entrance (remembered) and the guardhouse (seen)", ids)
	} else if !got.GetPoints()[0].GetRemembered() || got.GetPoints()[1].GetRemembered() {
		t.Errorf("remembered flags = %v, %v; want the entrance remembered and the guardhouse seen now", got.GetPoints()[0].GetRemembered(), got.GetPoints()[1].GetRemembered())
	}
	if layers := c.ana.mustLayers(c.campaign, c.mapID); !bitOf(layers.GetWall(), caveGrid, 7, 6) || !bitOf(layers.GetDifficultTerrain(), caveGrid, 4, 9) {
		t.Errorf("Ana lost the walls and the rubble of the room she remembers")
	}
	if got := remembered(res); got == 0 {
		t.Fatal("Ana remembers nothing")
	}

	// A character that is not on the map sees nothing new, only what is remembered.
	if _, err := m.maps.RemoveMapToken(t.Context(), connect.NewRequest(&mapsv1.RemoveMapTokenRequest{CampaignId: c.campaign, MapId: c.mapID, CharacterId: c.pens.GetId()})); err != nil {
		t.Fatal(err)
	}
	away := c.ana.mustVision(c.campaign, c.mapID)
	if away.GetCharacterOnMap() {
		t.Error("character_on_map is true for a character with no token on the map")
	}
	for n, code := range codes(t, away) {
		if code != 0 && code != 5 {
			t.Fatalf("with no token, square %d = %d, want unseen or remembered", n, code)
		}
	}
	if remembered(away) <= remembered(res) {
		t.Errorf("remembered squares fell from %d to %d when the token left", remembered(res), remembered(away))
	}
	m.placeAt(c.campaign, c.mapID, c.pens.GetId(), sqPens)

	// "Esquecer o que foi visto": the master only. Afterwards only what is seen now.
	_, err := c.ana.maps.ForgetMapVision(t.Context(), connect.NewRequest(&mapsv1.ForgetMapVisionRequest{CampaignId: c.campaign, MapId: c.mapID}))
	wantCode(t, "a player forgets the memory", err, connect.CodePermissionDenied)
	if remembered(c.ana.mustVision(c.campaign, c.mapID)) == 0 {
		t.Fatal("a refused call changed the memory")
	}
	m.placeAt(c.campaign, c.mapID, c.pens.GetId(), grid.Square{Col: 15, Row: 7})
	if _, err := m.maps.ForgetMapVision(t.Context(), connect.NewRequest(&mapsv1.ForgetMapVisionRequest{CampaignId: c.campaign, MapId: c.mapID})); err != nil {
		t.Fatal(err)
	}
	for n, code := range codes(t, c.ana.mustVision(c.campaign, c.mapID)) {
		if code == 5 {
			t.Fatalf("square %d is still remembered after the master cleared the memory", n)
		}
	}
	if ids := pointIDs(c.ana.mustGetMap(c.campaign, c.mapID)); !slices.Equal(ids, []string{c.guardhouse.GetId()}) {
		t.Errorf("Ana's points after the memory was cleared = %v, want only what she sees now (the guardhouse)", ids)
	}

	// A new grid clears the memory of every player.
	if c.memoryRows() == 0 {
		t.Fatal("no memory is stored")
	}
	if _, err := m.setGrid(c.campaign, c.mapID, 25); err != nil {
		t.Fatal(err)
	}
	if n := c.memoryRows(); n != 0 && remembered(c.ana.mustVision(c.campaign, c.mapID)) != 0 {
		t.Errorf("after a new grid, %d memories are stored and Ana remembers squares", n)
	}
}

// vision_changed (D6): sent to the players whose view changed, never with a
// square; an NPC moving in the dark sends a player nothing; a player character
// moving is no secret.
func TestMR036_VisionChangedReachesTheRightPlayers(t *testing.T) {
	t.Parallel()
	c := newCave(t)
	m := c.master
	ana, caio, bia := c.ana.watch(c.campaign), c.caio.watch(c.campaign), c.bia.watch(c.campaign)
	watchers := map[string]*watcher{"Ana": ana, "Caio": caio, "Bia": bia}
	drain := func() map[string][]*playv1.WatchGameSessionResponse {
		m.setMapRevealed(c.campaign, c.probeMap, !m.mustGetMap(c.campaign, c.probeMap).GetMap().GetRevealed())
		out := map[string][]*playv1.WatchGameSessionResponse{}
		for name, w := range watchers {
			out[name] = w.drain(c.probeMap)
		}
		return out
	}
	visionOnly := func(name string, evs []*playv1.WatchGameSessionResponse) {
		t.Helper()
		if len(evs) != 1 || evs[0].GetVisionChanged().GetMapId() != c.mapID {
			t.Errorf("%s got %v, want one vision_changed for the map", name, evs)
		}
	}
	for name, evs := range drain() { // what the setup left in the streams
		_ = name
		_ = evs
	}

	// A goblin moves between two squares nobody sees: nobody is told.
	m.placeAt(c.campaign, c.mapID, c.goblin1.GetId(), grid.Square{Col: 18, Row: 6})
	for name, evs := range drain() {
		if len(evs) != 0 {
			t.Errorf("%s got %v for a goblin moving in the dark, want nothing", name, evs)
		}
	}
	// A goblin moves where all three see it: vision_changed, never token_moved
	// (which would carry the square).
	m.placeAt(c.campaign, c.mapID, c.goblin2.GetId(), grid.Square{Col: 21, Row: 7})
	for name, evs := range drain() {
		visionOnly(name, evs)
	}
	// A goblin the master keeps hidden moves in plain view: nobody is told.
	m.placeAt(c.campaign, c.mapID, c.hiddenGoblin.GetId(), grid.Square{Col: 21, Row: 8})
	for name, evs := range drain() {
		if len(evs) != 0 {
			t.Errorf("%s got %v for a hidden goblin moving in view, want nothing", name, evs)
		}
	}
	// Toren moves: his token is no secret (token_moved to all), and only he sees
	// anything new (vision_changed to Caio).
	x, y := at(7, 7)
	m.placeToken(c.campaign, c.mapID, c.toren.GetId(), x, y)
	for name, evs := range drain() {
		moved := evs[0].GetTokenMoved()
		if len(evs) == 0 || moved.GetCharacterId() != c.toren.GetId() || moved.GetXBp() != x || moved.GetYBp() != y {
			t.Errorf("%s got %v, want Toren's token_moved first", name, evs)
			continue
		}
		if name == "Caio" {
			visionOnly(name, evs[1:])
		} else if len(evs) != 1 {
			t.Errorf("%s got %v after Toren moved, want his token_moved only", name, evs)
		}
	}
	// A change to a point on a square nobody sees reaches nobody; one on the
	// entrance reaches the player who sees it (Ana, with darkvision).
	if _, err := m.maps.UpdateMapPoint(t.Context(), connect.NewRequest(&mapsv1.UpdateMapPointRequest{
		CampaignId: c.campaign, MapId: c.mapID, PointId: c.guardhouse.GetId(), Description: new("outra"),
	})); err != nil {
		t.Fatal(err)
	}
	for name, got := range drain() {
		if len(got) != 0 {
			t.Errorf("%s got %v for a point in the guard room, want nothing", name, got)
		}
	}
	if _, err := m.maps.UpdateMapPoint(t.Context(), connect.NewRequest(&mapsv1.UpdateMapPointRequest{
		CampaignId: c.campaign, MapId: c.mapID, PointId: c.entrance.GetId(), Description: new("outra"),
	})); err != nil {
		t.Fatal(err)
	}
	for name, got := range drain() {
		switch {
		case name == "Ana" && (len(got) != 1 || got[0].GetMapChanged().GetMapId() != c.mapID):
			t.Errorf("Ana got %v for a point she sees, want map_changed", got)
		case name != "Ana" && len(got) != 0:
			t.Errorf("%s got %v for a point they do not see, want nothing", name, got)
		}
	}
	// Toren lights a torch: everyone near his light sees more.
	if _, err := c.caio.maps.SetCarriedLight(t.Context(), connect.NewRequest(&mapsv1.SetCarriedLightRequest{
		CampaignId: c.campaign, MapId: c.mapID, CharacterId: c.toren.GetId(), LightKey: "light:torch",
	})); err != nil {
		t.Fatal(err)
	}
	evs := drain()
	for name, got := range evs {
		var vision []*playv1.WatchGameSessionResponse
		for _, ev := range got {
			if ev.GetVisionChanged() != nil {
				vision = append(vision, ev)
			}
		}
		if len(vision) != 1 {
			t.Errorf("%s got %v for Toren's torch, want one vision_changed", name, got)
		}
	}
	// The master forgets: whoever remembered something hears of it.
	m.placeAt(c.campaign, c.mapID, c.pens.GetId(), grid.Square{Col: 15, Row: 7})
	drain()
	if _, err := m.maps.ForgetMapVision(t.Context(), connect.NewRequest(&mapsv1.ForgetMapVisionRequest{CampaignId: c.campaign, MapId: c.mapID})); err != nil {
		t.Fatal(err)
	}
	for name, got := range drain() {
		if name == "Ana" {
			visionOnly(name, got)
		}
	}
}

// RN-10, D6: a map's image is never one a player can fetch. An image the fog map
// shares with a map without fog is copied when the fog goes on; the other map
// keeps the old one.
func TestRN10_FogMapImageIsCopiedWhenShared(t *testing.T) {
	t.Parallel()
	h := newHarness(t)
	master, ana := h.newUser("Mestre"), h.newUser("Ana")
	campaign := h.newCampaign(master, ana)
	shared := master.newImage(campaign)
	a, b := master.createMap(campaign, "Com névoa", shared), master.createMap(campaign, "Sem névoa", shared)
	master.setMapRevealed(campaign, a.GetId(), true)
	master.setMapRevealed(campaign, b.GetId(), true)
	master.mustSetGrid(campaign, a.GetId(), 12)
	if res := ana.get("/images/" + shared); res.status != http.StatusOK {
		t.Fatalf("before the fog, Ana GET the shared image = %d, want 200", res.status)
	}
	before := len(master.list(campaign).GetImages())

	res, err := master.maps.SetMapFog(t.Context(), connect.NewRequest(&mapsv1.SetMapFogRequest{CampaignId: campaign, MapId: a.GetId(), FogEnabled: new(true)}))
	if err != nil {
		t.Fatal(err)
	}
	copyID := res.Msg.GetMap().GetImage().GetId()
	if copyID == shared || copyID == "" {
		t.Fatalf("the fog map's image = %q, want a copy of %q", copyID, shared)
	}
	if got := master.getMapImage(campaign, b.GetId()); got != shared {
		t.Errorf("the map without fog has image %q, want %q kept", got, shared)
	}
	if after := len(master.list(campaign).GetImages()); after != before+1 {
		t.Errorf("the gallery has %d images, want %d (the copy)", after, before+1)
	}
	if res := ana.get("/images/" + shared); res.status != http.StatusOK {
		t.Errorf("Ana GET the other map's image = %d, want 200", res.status)
	}
	for _, path := range []string{"/images/" + copyID, "/images/" + copyID + "/thumb"} {
		if res := ana.get(path); res.status != http.StatusNotFound {
			t.Errorf("Ana GET the fog map's image %s = %d, want 404", path, res.status)
		}
		if res := master.get(path); res.status != http.StatusOK {
			t.Errorf("the master GET the copy %s = %d, want 200", path, res.status)
		}
	}
	// The copy has the same bytes.
	originalKey, _ := blobKeys(campaign, shared)
	copyKey, _ := blobKeys(campaign, copyID)
	_, original := h.storedFile(originalKey)
	_, copied := h.storedFile(copyKey)
	if string(original) != string(copied) {
		t.Error("the copy's bytes differ from the image's")
	}
	// An image the fog map has to itself is not copied.
	solo := master.createMap(campaign, "Sozinho", master.newImage(campaign))
	master.setMapRevealed(campaign, solo.GetId(), true)
	master.mustSetGrid(campaign, solo.GetId(), 12)
	soloImage := master.getMapImage(campaign, solo.GetId())
	res, err = master.maps.SetMapFog(t.Context(), connect.NewRequest(&mapsv1.SetMapFogRequest{CampaignId: campaign, MapId: solo.GetId(), FogEnabled: new(true)}))
	if err != nil || res.Msg.GetMap().GetImage().GetId() != soloImage {
		t.Errorf("a fog map with its own image: image %q, error %v; want %q kept", res.Msg.GetMap().GetImage().GetId(), err, soloImage)
	}
	if res := ana.get("/images/" + soloImage); res.status != http.StatusNotFound {
		t.Errorf("Ana GET a fog map's own image = %d, want 404", res.status)
	}
}

func (u *user) getMapImage(campaignID, mapID string) string {
	u.h.t.Helper()
	return u.mustGetMap(campaignID, mapID).GetMap().GetImage().GetId()
}

// The fog needs a grid: its views and "Esquecer" on a map without one are refused
// as the layers are, and a map without fog gives everyone every square.
func TestMR036_VisionOfAMapWithoutFog(t *testing.T) {
	t.Parallel()
	s := newScenes(t, false)
	_, err := s.ana.vision(s.campaign, s.mapID)
	wantMapBlocked(t, "GetMapVision on a map without a grid", err, mapsv1.MapBlockedReason_MAP_BLOCKED_REASON_NO_GRID)
	s.master.mustSetGrid(s.campaign, s.mapID, 8)
	res := s.ana.mustVision(s.campaign, s.mapID)
	if res.GetFogEnabled() || !res.GetCharacterOnMap() {
		t.Errorf("a map without fog: fog %v, on map %v; want no fog, on the map", res.GetFogEnabled(), res.GetCharacterOnMap())
	}
	for n, code := range codes(t, res) {
		if code != 4 {
			t.Fatalf("a map without fog: square %d = %d, want 4", n, code)
		}
	}
	if got := s.ana.mustGetMap(s.campaign, s.mapID); got.GetMap().GetImageWithheld() || got.GetMap().GetImage().GetId() == "" {
		t.Errorf("a map without fog withholds its image: %v", got.GetMap().GetImage())
	}
	hidden := s.master.createMap(s.campaign, "Escondido", s.master.newImage(s.campaign))
	s.master.mustSetGrid(s.campaign, hidden.GetId(), 8)
	_, err = s.ana.vision(s.campaign, hidden.GetId())
	wantCode(t, "GetMapVision of a hidden map", err, connect.CodeNotFound)
}

var _ context.Context

// TestFogGetMapTiming measures the whole filtered GetMap, database included, for
// the cave and for a 60 x 40 map with six players and three lights (the numbers
// are in CONTRIBUTING, "As medidas da névoa"; run it with -v). It asserts nothing:
// a time limit would only make it fail on a busy machine.
func TestFogGetMapTiming(t *testing.T) {
	t.Parallel()
	cave := newCave(t)

	h := newHarness(t)
	master := h.newUser("Mestre")
	var players []*user
	for _, name := range []string{"Ana", "Caio", "Bia", "Dani", "Edu", "Fabi"} {
		players = append(players, h.newUser(name))
	}
	campaign := h.newCampaign(master, players...)
	big := master.createMap(campaign, "A grande sala", master.mustUpload(campaign, "sala.png", pngImage(t, 600, 400)).GetId())
	master.setMapRevealed(campaign, big.GetId(), true)
	master.mustSetGrid(campaign, big.GetId(), 60)
	var pillars [][2]int32
	for col := int32(6); col < 54; col += 6 {
		for row := int32(4); row < 36; row += 6 {
			pillars = append(pillars, [2]int32{col, row})
		}
	}
	master.mustPaint(campaign, big.GetId(), mapsv1.MapLayer_MAP_LAYER_WALL, 1, pillars...)
	g := grid.Grid{Columns: 60, Rows: 40}
	place := func(characterID string, col, row int) {
		x, y := g.CenterOf(grid.Square{Col: col, Row: row})
		master.placeToken(campaign, big.GetId(), characterID, int32(x), int32(y)) //nolint:gosec // G115: at most 10000
	}
	for i, p := range players {
		place(p.pc(campaign, "Herói "+p.id[:4], "race:gnome").GetId(), 3+9*i, 3+5*i)
	}
	for i, at := range [][2]int{{10, 10}, {30, 20}, {50, 30}} {
		x, y := g.CenterOf(grid.Square{Col: at[0], Row: at[1]})
		master.createPoint(&mapsv1.CreateMapPointRequest{
			CampaignId: campaign, MapId: big.GetId(), Kind: mapsv1.MapPointKind_MAP_POINT_KIND_LIGHT, Name: "Luz " + string(rune('A'+i)),
			Light: &mapsv1.LightSpec{PresetKey: "light:torch"}, XBp: int32(x), YBp: int32(y), //nolint:gosec // G115: at most 10000
		})
	}
	if _, err := master.maps.SetMapFog(t.Context(), connect.NewRequest(&mapsv1.SetMapFogRequest{CampaignId: campaign, MapId: big.GetId(), FogEnabled: new(true)})); err != nil {
		t.Fatal(err)
	}

	measure := func(name string, u *user, campaignID, mapID string) {
		u.mustGetMap(campaignID, mapID) // the first read compiles the scene
		const reads = 50
		start := time.Now()
		for range reads {
			u.mustGetMap(campaignID, mapID)
		}
		avg := time.Since(start) / reads
		t.Logf("%s: GetMap for a player, %v on average over %d reads (race detector and test database included)", name, avg, reads)
	}
	measure("the cave", cave.ana, cave.campaign, cave.mapID)
	measure("60 x 40, six players, three lights", players[0], campaign, big.GetId())
}

var caveOracle = map[string][]string{
	"pensantus": {
		"                        ",
		"                        ",
		"                        ",
		"                        ",
		"                        ",
		"########               #",
		"#gggggg#########   BBBB#",
		"gggggggggggggggBBBBBBBd#",
		"ggggg@ggggggggBBBBdBddd#",
		"#gggggg#  ######       #",
		"#gggggg#                ",
		"######g                 ",
		"     #gg                ",
		"     #ggg               ",
		"     # gg               ",
		"      ####              ",
	},
	"toren": {
		"                        ",
		"                        ",
		"                        ",
		"                        ",
		"                        ",
		"                        ",
		"       #      ##       #",
		"      @        ddBBBBBd#",
		"              dddddBddd#",
		"             ###   dddd#",
		"                  ######",
		"                        ",
		"                        ",
		"                        ",
		"                        ",
		"                        ",
	},
	"brisa": {
		"                        ",
		"                        ",
		"                        ",
		"                        ",
		"                        ",
		"                        ",
		"              ##       #",
		"    @          ddBBBBBd#",
		"              dddddBddd#",
		"             ###    ddd#",
		"                   #####",
		"                        ",
		"                        ",
		"                        ",
		"                        ",
		"                        ",
	},
	"salvia": {
		"                        ",
		"                        ",
		"                        ",
		"                        ",
		"                        ",
		"########               #",
		"#gggggg#########    BBB#",
		"gggggggggggggggddBBBBBd#",
		"ggg@ggggggggggBBdddBddd#",
		"#gggggg#  ######       #",
		"#gggggg#                ",
		"######gg                ",
		"     # gg               ",
		"        ggg             ",
		"         ggg            ",
		"        #####           ",
	},
	"salvia-lobo": {
		"                        ",
		"                        ",
		"                        ",
		"                        ",
		"                        ",
		"                       #",
		"              ##    BBB#",
		"               ddBBBBBd#",
		"   @          dddddBddd#",
		"             ###       #",
		"                        ",
		"                        ",
		"                        ",
		"                        ",
		"                        ",
		"                        ",
	},
	"toren-tocha": {
		"                        ",
		"                        ",
		"                        ",
		"                        ",
		"                        ",
		"########                ",
		"#ddBBBB#########       #",
		"ddBBBB@BBBBddddddBBBBBd#",
		"dddBBBBBBBdddddddddBddd#",
		"#ddBBBB#BB######   dddd#",
		"#dddBBB# d##      ######",
		"######B   d             ",
		"     #d    d            ",
		"     #d                 ",
		"     #dd                ",
		"     ####               ",
	},
	"brisa-tocha": {
		"                        ",
		"                        ",
		"                        ",
		"                        ",
		"                        ",
		"########                ",
		"#ddBBBB#########       #",
		"ddBB@BBBBBBddddddBBBBBd#",
		"dddBBBBBBBdddddddddBddd#",
		"#ddBBBB# B######    ddd#",
		"#dddBBB#  #        #####",
		"######B                 ",
		"     #                  ",
		"                        ",
		"       d                ",
		"      ###               ",
	},
	"pensantus-tocha": {
		"                        ",
		"                        ",
		"                        ",
		"                        ",
		"                        ",
		"########               #",
		"#BBBBBB#########   BBBB#",
		"BBBBBBBBBBBBBBBBBBBBBBd#",
		"BBBBB@BBBBBBBBBBBBdBddd#",
		"#BBBBBB#  ######       #",
		"#BBBBBB#                ",
		"######B                 ",
		"     #Bg                ",
		"     #Bgg               ",
		"     # Bg               ",
		"      ####              ",
	},
	"salvia-tocha": {
		"                        ",
		"                        ",
		"                        ",
		"                        ",
		"                        ",
		"########               #",
		"#BBBBBB#########    BBB#",
		"BBBBBBBBBBBBBBBddBBBBBd#",
		"BBB@BBBBBBBBBBBBdddBddd#",
		"#BBBBBB#  ######       #",
		"#BBBBBB#                ",
		"######Bg                ",
		"     # gg               ",
		"        ggg             ",
		"         ggg            ",
		"        #####           ",
	},
}

// Showing a fog map's image makes no copy while there is no open session, and showing the same image again (to turn "keep" on) shows the copy already made: no new gallery image, no event.
func TestShowingAFogMapImageAgainReusesItsCopy(t *testing.T) {
	t.Parallel()
	h := newHarness(t)
	master, ana := h.newUser("Mestre"), h.newUser("Ana")
	campaign := h.newCampaign(master, ana)
	x := master.newImage(campaign)
	fog := master.createMap(campaign, "Com névoa", x)
	master.mustSetGrid(campaign, fog.GetId(), 12)
	master.setMapRevealed(campaign, fog.GetId(), true)
	if _, err := master.maps.SetMapFog(t.Context(), connect.NewRequest(&mapsv1.SetMapFogRequest{CampaignId: campaign, MapId: fog.GetId(), FogEnabled: new(true)})); err != nil {
		t.Fatal(err)
	}
	count := func() int { return len(master.list(campaign).GetImages()) }
	show := func(keep bool) (*playv1.SetShownImageResponse, error) {
		res, err := master.play.SetShownImage(t.Context(), connect.NewRequest(&playv1.SetShownImageRequest{CampaignId: campaign, ImageId: x, Keep: keep}))
		if err != nil {
			return nil, err
		}
		return res.Msg, nil
	}
	left := func() int {
		res, err := ana.play.ListLeftImages(t.Context(), connect.NewRequest(&playv1.ListLeftImagesRequest{CampaignId: campaign}))
		if err != nil {
			t.Fatal(err)
		}
		return len(res.Msg.GetImages())
	}

	// No open session.
	g0 := count()
	for range 2 {
		_, err := show(false)
		wantBlocked(t, "SetShownImage without a session", err, playv1.GameSessionBlockedReason_GAME_SESSION_BLOCKED_REASON_NO_OPEN_SESSION)
	}
	g1 := count()
	t.Logf("gallery before=%d after two refused calls=%d", g0, g1)
	if g1 != g0 {
		t.Errorf("gallery grew from %d to %d images after refused calls (orphan copies)", g0, g1)
	}

	// A session is open: the same image again, to turn keep on.
	master.start(campaign)
	pw := ana.watch(campaign)
	first, err := show(false)
	if err != nil {
		t.Fatal(err)
	}
	firstID := first.GetShownImage().GetId()
	if ev := pw.next().GetShownImageChanged(); ev.GetImage().GetId() != firstID {
		t.Fatalf("first event = %v, want the shown image %s", ev, firstID)
	}
	g2 := count()
	second, err := show(true)
	if err != nil {
		t.Fatal(err)
	}
	secondID := second.GetShownImage().GetId()
	g3 := count()
	t.Logf("first shown=%s second shown=%s gallery before=%d after=%d left images (player)=%d", firstID, secondID, g2, g3, left())
	if secondID != firstID {
		t.Errorf("calling again with the same image shows %s, want the same %s", secondID, firstID)
	}
	if g3 != g2 {
		t.Errorf("gallery grew from %d to %d on the keep call", g2, g3)
	}
	select {
	case ev := <-pw.events:
		if ev.GetHeartbeat() == nil {
			t.Errorf("the player received %v on the keep call, want nothing", ev)
		}
	case <-time.After(700 * time.Millisecond):
	}
	// Stop showing: the kept image is left, exactly one.
	if _, err := master.play.SetShownImage(t.Context(), connect.NewRequest(&playv1.SetShownImageRequest{CampaignId: campaign})); err != nil {
		t.Fatal(err)
	}
	if n := left(); n != 1 {
		t.Errorf("left images = %d, want 1", n)
	}
}

// The copy of a fog map's image is made inside the show transaction: a refused show leaves no gallery image and no file, and showing the same image twice adds nothing.
func TestShowingAFogMapImageLeavesNoStrayCopy(t *testing.T) {
	t.Parallel()
	h := newHarness(t)
	master := h.newUser("Mestre")
	campaign := h.newCampaign(master)
	x := master.newImage(campaign)
	fog := master.createMap(campaign, "Com névoa", x)
	master.mustSetGrid(campaign, fog.GetId(), 12)
	if _, err := master.maps.SetMapFog(t.Context(), connect.NewRequest(&mapsv1.SetMapFogRequest{CampaignId: campaign, MapId: fog.GetId(), FogEnabled: new(true)})); err != nil {
		t.Fatal(err)
	}
	fogImage := master.getMapImage(campaign, fog.GetId())
	show := func() error {
		_, err := master.play.SetShownImage(t.Context(), connect.NewRequest(&playv1.SetShownImageRequest{CampaignId: campaign, ImageId: fogImage}))
		return err
	}
	counts := func() (int, int) { return len(master.list(campaign).GetImages()), len(h.storedFiles()) }

	// No open session: it fails and leaves nothing behind.
	imgs0, files0 := counts()
	err := show()
	t.Logf("error = %v", err)
	if err == nil {
		t.Fatal("SetShownImage without an open session succeeded, want an error")
	}
	imgs1, files1 := counts()
	t.Logf("images %d -> %d, files %d -> %d", imgs0, imgs1, files0, files1)
	if imgs1 != imgs0 || files1 != files0 {
		t.Errorf("failed show left images %d -> %d and files %d -> %d, want no growth", imgs0, imgs1, files0, files1)
	}

	// An open session, the same image twice: the second show grows nothing.
	master.start(campaign)
	imgs2, files2 := counts()
	if err := show(); err != nil {
		t.Fatal(err)
	}
	imgs3, files3 := counts()
	if err := show(); err != nil {
		t.Fatal(err)
	}
	imgs4, files4 := counts()
	t.Logf("images %d -> %d -> %d, files %d -> %d -> %d", imgs2, imgs3, imgs4, files2, files3, files4)
	if imgs4 != imgs3 || files4 != files3 {
		t.Errorf("showing the same fog image again grew images %d -> %d and files %d -> %d, want no growth", imgs3, imgs4, files3, files4)
	}
}

// fogMapImage makes a map with the fog on and returns its (fog) image's ID.
func (u *user) fogMapImage(campaignID, name string) string {
	u.h.t.Helper()
	m := u.createMap(campaignID, name, u.newImage(campaignID))
	u.mustSetGrid(campaignID, m.GetId(), 12)
	if _, err := u.maps.SetMapFog(u.h.t.Context(), connect.NewRequest(&mapsv1.SetMapFogRequest{CampaignId: campaignID, MapId: m.GetId(), FogEnabled: new(true)})); err != nil {
		u.h.t.Fatal(err)
	}
	return u.getMapImage(campaignID, m.GetId())
}

// A fog map's image is never an NPC's portrait as it is: the portrait is a copy of its
// own, and the copy already made is the one every later portrait of that image gets, in
// a new NPC and in a saved sheet alike.
func TestAFogMapImageAsPortraitReusesItsCopy(t *testing.T) {
	t.Parallel()
	h := newHarness(t)
	master := h.newUser("Mestre")
	campaign := h.newCampaign(master)
	fogImage := master.fogMapImage(campaign, "Com névoa")
	before := len(master.list(campaign).GetImages())
	first := master.createNPC(campaign, "Vigia", fogImage)
	second := master.createNPC(campaign, "Guarda", "")
	res, err := master.characters.UpdateCharacter(t.Context(), connect.NewRequest(&charactersv1.UpdateCharacterRequest{
		CampaignId: campaign, CharacterId: second.GetId(), Revision: second.GetRevision(), Name: "Guarda",
		Sheet: &charactersv1.CharacterSheet{Content: &charactersv1.CharacterSheet_Basic{Basic: &charactersv1.BasicSheet{
			HitPointsMax: 7, ArmorClass: 12, SpeedFt: 30, ChallengeRating: "2", PortraitImageId: fogImage,
		}}},
	}))
	if err != nil {
		t.Fatalf("UpdateCharacter() error = %v", err)
	}
	portrait := func(c *charactersv1.Character) string { return c.GetSheet().GetBasic().GetPortraitImageId() }
	if portrait(first) == fogImage || portrait(res.Msg.GetCharacter()) == fogImage {
		t.Fatalf("a portrait is the fog map's own image: %q, %q", portrait(first), portrait(res.Msg.GetCharacter()))
	}
	if portrait(first) != portrait(res.Msg.GetCharacter()) {
		t.Errorf("the portraits are %q and %q, want the one copy", portrait(first), portrait(res.Msg.GetCharacter()))
	}
	if got := len(master.list(campaign).GetImages()); got != before+1 {
		t.Errorf("the gallery has %d images, want %d (one copy)", got, before+1)
	}
}

// Two shows of the same fog image at once both made their copy before the first
// committed: the second finds the first's row inside its transaction, uses it and
// deletes its own files, so the gallery gets one copy.
func TestTwoCopiesOfAFogMapImageMadeAtOnceLeaveOne(t *testing.T) {
	t.Parallel()
	h := newHarness(t)
	master := h.newUser("Mestre")
	campaign := h.newCampaign(master)
	fogImage := master.fogMapImage(campaign, "Com névoa")
	sm := NewSessionMaps(h.pool)
	sm.SetService(h.svc)
	var copies []ShownCopy
	var shown []string
	for range 2 {
		img, c, err := sm.PrepareShow(t.Context(), campaign, fogImage)
		if err != nil || c == nil {
			t.Fatalf("PrepareShow() = %v, %v, %v; want a copy to make", img, c, err)
		}
		copies, shown = append(copies, c), append(shown, img.GetId())
	}
	if shown[0] == shown[1] {
		t.Fatal("both prepared the same copy, so the test proves nothing")
	}
	before, files := len(master.list(campaign).GetImages()), len(h.storedFiles())
	var ids []string
	for i, c := range copies {
		var id string
		var created bool
		if err := db.InTx(t.Context(), h.pool, func(tx pgx.Tx) (err error) {
			id, created, err = c.Insert(t.Context(), tx)
			return err
		}); err != nil {
			t.Fatalf("Insert() of copy %d error = %v", i, err)
		}
		if !created {
			c.Discard(t.Context())
		}
		ids = append(ids, id)
	}
	if ids[0] != shown[0] || ids[1] != shown[0] {
		t.Errorf("the copies used %v, want both to use the first, %s", ids, shown[0])
	}
	if got := len(master.list(campaign).GetImages()); got != before+1 {
		t.Errorf("the gallery has %d images, want %d (one copy)", got, before+1)
	}
	if got := len(h.storedFiles()); got != files-2 {
		t.Errorf("the store has %d files, want %d (the second copy's were deleted)", got, files-2)
	}
}

// blockLayers is a mapsdb.DBTX whose first read of map_layers waits: it
// signals started, then holds until release is closed or the statement's ctx
// ends, in which case the read fails like a canceled one.
type blockLayers struct {
	mapsdb.DBTX
	once    sync.Once
	started chan struct{}
	release chan struct{}
}

func (b *blockLayers) QueryRow(ctx context.Context, sql string, args ...any) pgx.Row {
	if strings.Contains(sql, "FROM map_layers") {
		first := false
		b.once.Do(func() { first = true })
		if first {
			close(b.started)
			select {
			case <-b.release:
			case <-ctx.Done():
				return errRow{ctx.Err()}
			}
		}
	}
	return b.DBTX.QueryRow(ctx, sql, args...)
}

type errRow struct{ err error }

func (r errRow) Scan(...any) error { return r.err }

func TestSceneCompileSurvivesTheFirstAskerHangingUp(t *testing.T) {
	c := newCave(t)
	svc := c.h.svc
	ctx := t.Context()
	in, _, ok, err := svc.fogRow(ctx, c.campaign, c.mapID)
	if err != nil || !ok {
		t.Fatalf("fogRow: ok=%v err=%v", ok, err)
	}
	points, err := svc.queries.ListMapPoints(ctx, c.mapID)
	if err != nil {
		t.Fatal(err)
	}
	tokens, err := svc.queries.ListMapTokens(ctx, c.mapID)
	if err != nil {
		t.Fatal(err)
	}
	svc.lits.forget(c.mapID)
	bl := &blockLayers{DBTX: c.h.pool, started: make(chan struct{}), release: make(chan struct{})}
	svc.queries = mapsdb.New(bl)

	ctxA, cancelA := context.WithCancel(ctx)
	defer cancelA()
	var wg sync.WaitGroup
	wg.Add(2)
	go func() {
		defer wg.Done()
		_, _ = svc.newSight(ctxA, nil, in, points, tokens)
	}()
	select {
	case <-bl.started:
	case <-time.After(10 * time.Second):
		t.Fatal("first compile never started")
	}
	var errB error
	go func() {
		defer wg.Done()
		_, errB = svc.newSight(ctx, nil, in, points, tokens)
	}()
	time.Sleep(300 * time.Millisecond) // B joins the compile A started
	cancelA()
	close(bl.release)
	wg.Wait()
	if errB != nil {
		t.Fatalf("a waiter with a live context got %v (canceled=%v); want a valid scene", errB, errors.Is(errB, context.Canceled))
	}
}
