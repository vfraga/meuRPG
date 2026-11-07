package maps

import (
	"bytes"
	"encoding/json"
	"net/http"
	"slices"
	"strconv"
	"strings"
	"testing"

	"connectrpc.com/connect"
	"google.golang.org/protobuf/proto"

	mapsv1 "github.com/PuraFome/meuRPG/backend/gen/meurpg/maps/v1"
	"github.com/PuraFome/meuRPG/backend/internal/authz"
	"github.com/PuraFome/meuRPG/backend/internal/play/link"
	"github.com/PuraFome/meuRPG/backend/internal/rules/grid"
)

// The map on the table screen (RN-10): the TV reads the map as a player with no character does,
// through the code of the player's GetMap and GetMapVision. These tests need the database
// (MEURPG_TEST_DATABASE_URL).

// shown and refused are the gates a test hands the maps module: the play module's rule, decided by its caller.
func shown(bool, bool) bool   { return true }
func refused(bool, bool) bool { return false }

// tvMap reads the map as the table screen does.
func (c *cave) tvMap(t *testing.T, may link.MapGate) link.TableScreenMap {
	t.Helper()
	sm := NewSessionMaps(c.h.pool)
	sm.SetService(c.h.svc)
	out, err := sm.TableScreenMap(t.Context(), c.campaign, c.mapID, may)
	if err != nil {
		t.Fatalf("TableScreenMap() error = %v", err)
	}
	return out
}

func (c *cave) groupVision(t *testing.T, on bool) {
	t.Helper()
	if _, err := c.master.maps.SetMapFog(t.Context(), connect.NewRequest(&mapsv1.SetMapFogRequest{CampaignId: c.campaign, MapId: c.mapID, GroupVision: new(on)})); err != nil {
		t.Fatalf("SetMapFog(group vision) error = %v", err)
	}
}

// tvText is everything the TV reads of a map, as the app's JSON.
func tvText(t *testing.T, m link.TableScreenMap) string {
	t.Helper()
	text := asJSON(t, m.Map)
	if m.Vision != nil {
		text += asJSON(t, m.Vision)
	}
	return text
}

// canaries are the things of the master that no player reads, each next to the party where the group sees.
type canaries struct {
	names, ids []string
	// numbers are canaries that are numbers (a treasure's value). They are searched as numbers, in the number fields
	// of the reply, never as digits in its text: a UUID, a timestamp or a revision holds any digits by chance.
	numbers []float64
}

// leaksIn lists what the reply (the app's JSON) holds of the canaries: a name or an id as text, a number as a JSON
// number. ids limits the ids searched.
func (c canaries) leaksIn(body string, ids []string) []string {
	var out []string
	for _, w := range append(slices.Clone(c.names), ids...) {
		if strings.Contains(body, w) {
			out = append(out, w)
		}
	}
	// The reply is the map and its vision, two documents in a row.
	dec := json.NewDecoder(strings.NewReader(body))
	for dec.More() {
		var doc any
		if dec.Decode(&doc) != nil {
			break
		}
		out = append(out, c.numbersIn(doc)...)
	}
	return out
}

// numbersIn lists the canary numbers among the JSON numbers of the document.
func (c canaries) numbersIn(v any) []string {
	var out []string
	switch v := v.(type) {
	case float64:
		if slices.Contains(c.numbers, v) {
			out = append(out, strconv.FormatFloat(v, 'f', -1, 64))
		}
	case []any:
		for _, e := range v {
			out = append(out, c.numbersIn(e)...)
		}
	case map[string]any:
		for _, e := range v {
			out = append(out, c.numbersIn(e)...)
		}
	}
	return out
}

// hideThings puts a hidden point, a trap, a treasure, a secret door and a hidden NPC in the squares the party
// sees. The master's own GetMap is the positive control.
func (c *cave) hideThings(t *testing.T) canaries {
	t.Helper()
	m := c.master
	point := func(kind mapsv1.MapPointKind, name string, col, row int, edit func(*mapsv1.CreateMapPointRequest)) *mapsv1.MapPoint {
		x, y := at(col, row)
		req := &mapsv1.CreateMapPointRequest{CampaignId: c.campaign, MapId: c.mapID, Kind: kind, Name: name, Description: "descrição de " + name, XBp: x, YBp: y}
		if edit != nil {
			edit(req)
		}
		return m.createPoint(req)
	}
	hidden := point(mapsv1.MapPointKind_MAP_POINT_KIND_SCENE, "CanarioPontoOculto", 4, 8, nil)
	trap := point(mapsv1.MapPointKind_MAP_POINT_KIND_TRAP, "CanarioArmadilha", 5, 7, func(r *mapsv1.CreateMapPointRequest) { r.Trap = testTrap() })
	treasure := point(mapsv1.MapPointKind_MAP_POINT_KIND_TREASURE, "CanarioTesouro", 6, 8, func(r *mapsv1.CreateMapPointRequest) { r.TreasureValuePo = proto.Int32(4321) })
	m.mustPaint(c.campaign, c.mapID, mapsv1.MapLayer_MAP_LAYER_DOORS, int32(mapsv1.DoorState_DOOR_STATE_SECRET), [2]int32{7, 7})
	// The master's GetMap has them all.
	master := asJSON(t, m.mustGetMap(c.campaign, c.mapID))
	for _, p := range []*mapsv1.MapPoint{hidden, trap, treasure} {
		if !strings.Contains(master, p.GetName()) || !strings.Contains(master, p.GetId()) {
			t.Fatalf("the master's GetMap does not carry %s: the control is void", p.GetName())
		}
	}
	if !strings.Contains(master, "Goblin Emboscado") {
		t.Fatal("the master's GetMap does not carry the hidden NPC: the control is void")
	}
	return canaries{
		names:   []string{"CanarioPontoOculto", "CanarioArmadilha", "CanarioTesouro", "Goblin Emboscado", "TRAP", "descrição de Canario"},
		numbers: []float64{4321},
		ids:     []string{hidden.GetId(), trap.GetId(), treasure.GetId(), c.hiddenGoblin.GetId(), c.goblin1.GetId(), c.captain.GetId()},
	}
}

func TestTableScreenMapHasOnlyWhatPlayersRead(t *testing.T) {
	t.Parallel()
	c := newCave(t)
	c.groupVision(t, true)
	hid := c.hideThings(t)
	tv := c.tvMap(t, shown)
	body := tvText(t, tv)
	for _, w := range hid.leaksIn(body, hid.ids) {
		t.Errorf("the TV's map has %q, which no player reads: %s", w, body)
	}
	for _, w := range []string{c.imageID, "thumbnailUrl", "Tocha da guarita", "light:torch", "Goblin 1", "Capitão Goblin"} {
		if strings.Contains(body, w) {
			t.Errorf("the TV's map has %q, which no player reads: %s", w, body)
		}
	}
	// It is what a player receives under "Visão do grupo": the same tokens, and only points a player reads.
	player := c.ana.mustGetMap(c.campaign, c.mapID)
	if got, want := tokenNames(tv.Map), tokenNames(player); !slices.Equal(got, want) {
		t.Errorf("the TV's tokens = %v, want a player's %v", got, want)
	}
	allowed := map[string]bool{}
	for _, p := range c.master.mustGetMap(c.campaign, c.mapID).GetPoints() {
		allowed[p.GetId()] = p.GetRevealed()
	}
	for _, p := range tv.Map.GetPoints() {
		if !allowed[p.GetId()] {
			t.Errorf("the TV's map has the point %q, which is not revealed", p.GetName())
		}
	}
	if !tv.Map.GetMap().GetImageWithheld() || tv.Map.GetMap().GetImage().GetId() != "" || tv.Hidden {
		t.Errorf("the TV's map = withheld %v, image %v, hidden %v; want a fog map with no image", tv.Map.GetMap().GetImageWithheld(), tv.Map.GetMap().GetImage(), tv.Hidden)
	}
	if tv.Vision.GetCharacterOnMap() != true || tv.Revision == 0 {
		t.Errorf("vision on map %v, revision %d; want the group on the map and a revision", tv.Vision.GetCharacterOnMap(), tv.Revision)
	}
	// Every NPC token the master hid, and the NPCs no character sees, are absent; the party is there.
	if names := tokenNames(tv.Map); slices.Contains(names, "Goblin Emboscado") || !slices.Contains(names, "Pensantus") {
		t.Errorf("the TV's tokens = %v", names)
	}
}

// With the fog on and "Visão do grupo" off, the gate refuses the map: nothing of it is read. A gate that
// lets it through still gives the TV no square, because without a group the table sees and remembers nothing.
func TestTableScreenMapWithoutGroupVisionIsHiddenAndBlack(t *testing.T) {
	t.Parallel()
	c := newCave(t)
	hid := c.hideThings(t)
	got := c.tvMap(t, refused)
	if !got.Hidden || got.Map != nil || got.Vision != nil || got.Revision != 0 {
		t.Errorf("refused map = %+v, want hidden with nothing else", got)
	}
	open := c.tvMap(t, shown)
	for n, code := range codes(t, open.Vision) {
		if code != 0 {
			t.Fatalf("square %d = %d without group vision, want unseen", n, code)
		}
	}
	body := tvText(t, open)
	for _, w := range hid.leaksIn(body, hid.ids) {
		t.Errorf("the TV's map has %q: %s", w, body)
	}
}

// The tiles of the fog map for the TV: squares the group does not see are black. The master asks for them
// with `as=tv`; nobody else does.
func TestTableScreenTilesAreBlackWhereTheGroupDoesNotSee(t *testing.T) {
	t.Parallel()
	c := newCave(t)
	c.groupVision(t, true)
	tv := c.tvMap(t, shown)
	if len(tv.Vision.GetTiles()) == 0 {
		t.Fatal("the TV has no tile for a map the group sees part of")
	}
	partial := false
	for _, tile := range tv.Vision.GetTiles() {
		r := c.master.get(tileURL(tv.Vision, tile) + "&as=tv")
		shownSquares, black := checkTile(t, "the TV", decodeTile(t, r), tv.Vision, tile)
		partial = partial || (shownSquares > 0 && black > 0)
	}
	if !partial {
		t.Error("no tile mixes seen and unseen squares: the test does not tell black from drawn")
	}
	if _, ok := c.h.svc.tiles.views.entries[viewKey{c.mapID, "tv:" + c.campaign}]; !ok {
		t.Error("the TV's tiles are not kept under tv:<campaign>")
	}
}

// `as=tv` is the master's: a player gets the 404 of a tile that does not exist, byte for byte.
func TestTableScreenTilesAreRefusedToPlayersAsAMissingTile(t *testing.T) {
	t.Parallel()
	c := newCave(t)
	c.groupVision(t, true)
	tile := c.tvMap(t, shown).Vision.GetTiles()[0]
	path := TilesPath + c.mapID + "/tiles/" + itoa(tile.GetTx()) + "/" + itoa(tile.GetTy())
	missing := c.ana.get(TilesPath + c.mapID + "/tiles/99/99")
	if missing.status != http.StatusNotFound {
		t.Fatalf("a missing tile = %d, want 404", missing.status)
	}
	for _, who := range []struct {
		name string
		u    *user
	}{{"a player", c.ana}, {"another player", c.caio}, {"a stranger", c.h.newUser("Intruso")}, {"nobody", c.h.anonymous()}} {
		got := who.u.get(path + "?as=tv")
		if who.name == "nobody" {
			if got.status != http.StatusUnauthorized {
				t.Errorf("%s asking for the TV's tile = %d, want 401", who.name, got.status)
			}
			continue
		}
		if got.status != missing.status || !bytes.Equal(got.body, missing.body) {
			t.Errorf("%s asking for the TV's tile = %d %q, want the missing tile's %d %q", who.name, got.status, got.body, missing.status, missing.body)
		}
	}
	// The master without `as` is refused too: the master reads tiles only as someone.
	if got := c.master.get(path); got.status != http.StatusNotFound {
		t.Errorf("the master asking for a tile with no `as` = %d, want 404", got.status)
	}
}

// With the fog off the map goes to the TV as a player gets it, image included; the image route serves the
// same file to the master and to a player, so the TV page needs no other URL; there are no tiles.
func TestTableScreenMapWithoutFogIsTheWholePlayerMap(t *testing.T) {
	t.Parallel()
	c := newCave(t)
	hid := c.hideThings(t)
	if _, err := c.master.maps.SetMapFog(t.Context(), connect.NewRequest(&mapsv1.SetMapFogRequest{CampaignId: c.campaign, MapId: c.mapID, FogEnabled: new(false)})); err != nil {
		t.Fatalf("SetMapFog(off) error = %v", err)
	}
	tv := c.tvMap(t, shown)
	player := c.ana.mustGetMap(c.campaign, c.mapID)
	if tv.Hidden || tv.Vision.GetFogEnabled() {
		t.Fatalf("hidden %v, fog %v; want a map with no fog", tv.Hidden, tv.Vision.GetFogEnabled())
	}
	if tv.Map.GetMap().GetImage().GetUrl() != player.GetMap().GetImage().GetUrl() || tv.Map.GetMap().GetImage().GetUrl() == "" || tv.Map.GetMap().GetImageWithheld() {
		t.Errorf("image = %v, want a player's %v", tv.Map.GetMap().GetImage(), player.GetMap().GetImage())
	}
	if got, want := tokenNames(tv.Map), tokenNames(player); !slices.Equal(got, want) {
		t.Errorf("tokens = %v, want a player's %v", got, want)
	}
	if got, want := len(tv.Map.GetPoints()), len(player.GetPoints()); got != want {
		t.Errorf("%d points, want a player's %d", got, want)
	}
	for n, code := range codes(t, tv.Vision) {
		if code != 4 {
			t.Fatalf("square %d = %d, want 4: every square is seen on a map with no fog", n, code)
		}
	}
	if len(tv.Vision.GetTiles()) != 0 || tv.Vision.GetTilesPath() != "" {
		t.Errorf("tiles on a map with no fog: %v", tv.Vision.GetTilesPath())
	}
	body := tvText(t, tv)
	for _, w := range hid.leaksIn(body, hid.ids[:3]) {
		t.Errorf("the TV's map has %q: %s", w, body)
	}
	// The image route serves the file to the master and to a player alike; only the cache headers differ.
	url := tv.Map.GetMap().GetImage().GetUrl()
	masterGot, playerGot := c.master.get(url), c.ana.get(url)
	if masterGot.status != http.StatusOK || playerGot.status != http.StatusOK || !bytes.Equal(masterGot.body, playerGot.body) {
		t.Errorf("image: master %d, player %d, same bytes %v; want the same file for both", masterGot.status, playerGot.status, bytes.Equal(masterGot.body, playerGot.body))
	}
}

// The TV's revision is its own: it does not move for what only the master sees, and it moves for what the TV
// shows.
func TestTableScreenRevisionIgnoresWhatOnlyTheMasterSees(t *testing.T) {
	t.Parallel()
	c := newCave(t)
	c.groupVision(t, true)
	c.hideThings(t)
	same := c.tvMap(t, shown)
	x, y := at(4, 8)
	c.master.createPoint(&mapsv1.CreateMapPointRequest{CampaignId: c.campaign, MapId: c.mapID, Kind: mapsv1.MapPointKind_MAP_POINT_KIND_SCENE, Name: "Outro oculto", XBp: x, YBp: y})
	after := c.tvMap(t, shown)
	if after.Revision != same.Revision {
		t.Errorf("the TV's revision moved from %d to %d for a hidden point", same.Revision, after.Revision)
	}
	// A point the master reveals is news for the TV.
	revealed := c.master.createPoint(&mapsv1.CreateMapPointRequest{CampaignId: c.campaign, MapId: c.mapID, Kind: mapsv1.MapPointKind_MAP_POINT_KIND_SCENE, Name: "Visível", XBp: x, YBp: y})
	c.master.setPointRevealed(c.campaign, revealed, true)
	if now := c.tvMap(t, shown); now.Revision == after.Revision {
		t.Error("the TV's revision did not move when a point was revealed")
	}
}

// The TV's viewer is a player: never the master, no trap known, nothing owned.
func TestTableScreenViewerIsNeverTheMaster(t *testing.T) {
	t.Parallel()
	v := newTVViewer("current")
	if v.master || !v.tv || v.userID != "" || v.preview || len(v.traps) != 0 || v.parentKnows != nil {
		t.Errorf("TV viewer = %+v, want a player with no account, no trap and no preview", v)
	}
	if !v.seesMap("current", nil) || v.seesMap("hidden", nil) {
		t.Error("the TV sees the current map and no hidden one, as a player does")
	}
	for _, role := range []authz.Role{authz.RoleMaster, authz.RolePlayer} {
		if got := newViewer(authz.Membership{Role: role}, "").tv; got {
			t.Errorf("a %v viewer is the TV", role)
		}
	}
	if k := tileViewerOf(v, "c1", ""); k != "tv:c1" {
		t.Errorf("tile key = %q, want tv:c1", k)
	}
}

// On a map with no fog, a stroke of the master's on a layer (a secret door) does not move the TV's revision:
// a player reads the secret door as a wall, and the counter of the layers is not the TV's. A point the
// master reveals still does.
func TestTableScreenRevisionIgnoresLayerStrokesOnAMapWithoutFog(t *testing.T) {
	t.Parallel()
	c := newCave(t)
	if _, err := c.master.maps.SetMapFog(t.Context(), connect.NewRequest(&mapsv1.SetMapFogRequest{CampaignId: c.campaign, MapId: c.mapID, FogEnabled: new(false)})); err != nil {
		t.Fatalf("SetMapFog(off) error = %v", err)
	}
	before := c.tvMap(t, shown)
	if before.Map.GetMap().GetLayersRevision() != 0 {
		t.Errorf("layers_revision = %d on the TV, want 0", before.Map.GetMap().GetLayersRevision())
	}
	c.master.mustPaint(c.campaign, c.mapID, mapsv1.MapLayer_MAP_LAYER_DOORS, int32(mapsv1.DoorState_DOOR_STATE_SECRET), [2]int32{8, 7})
	if after := c.tvMap(t, shown); after.Revision != before.Revision {
		t.Errorf("the TV's revision moved from %d to %d for a secret door", before.Revision, after.Revision)
	}
	x, y := at(4, 8)
	p := c.master.createPoint(&mapsv1.CreateMapPointRequest{CampaignId: c.campaign, MapId: c.mapID, Kind: mapsv1.MapPointKind_MAP_POINT_KIND_SCENE, Name: "Visível", XBp: x, YBp: y})
	c.master.setPointRevealed(c.campaign, p, true)
	if now := c.tvMap(t, shown); now.Revision == before.Revision {
		t.Error("the TV's revision did not move when a point was revealed")
	}
}

// squaresOf lists the squares a view says are remembered (code 5).
func squaresOf(t *testing.T, res *mapsv1.GetMapVisionResponse, code byte) map[int]bool {
	t.Helper()
	out := map[int]bool{}
	for n, got := range codes(t, res) {
		if got == code {
			out[n] = true
		}
	}
	return out
}

// groupMemoryOf: the TV remembers the squares any member of the party remembers, in the map's current epoch.
// Ana saw a room and Caio another with no group vision; the TV, once the group has its vision, remembers both.
func TestTableScreenRemembersTheUnionOfThePartysMemoryOfTheCurrentEpochIncludingTheDeadAndNotWhoLeft(t *testing.T) {
	t.Parallel()
	c := newCave(t)
	ctx := t.Context()
	g := caveGrid
	bits := func(sqs ...grid.Square) []byte {
		l := grid.NewLayer(g)
		for _, sq := range sqs {
			l.Set(sq.Col, sq.Row, true)
		}
		return l.Encode()
	}
	var epoch int32
	if err := c.h.pool.QueryRow(ctx, `SELECT vision_epoch FROM maps WHERE id = $1`, c.mapID).Scan(&epoch); err != nil {
		t.Fatalf("read the epoch: %v", err)
	}
	userOf := func(u *user) string { return u.id }
	roomA, roomB, roomOld, roomLeft := grid.Square{Col: 20, Row: 12}, grid.Square{Col: 21, Row: 12}, grid.Square{Col: 22, Row: 12}, grid.Square{Col: 23, Row: 12}
	put := func(user string, ep int32, sq grid.Square) {
		if _, err := c.h.pool.Exec(ctx, `INSERT INTO map_vision_memory (map_id, user_id, seen, epoch, updated_at) VALUES ($1, $2, $3, $4, now())
			ON CONFLICT (map_id, user_id) DO UPDATE SET seen = excluded.seen, epoch = excluded.epoch`, c.mapID, user, bits(sq), ep); err != nil {
			t.Fatalf("write a memory: %v", err)
		}
	}
	put(userOf(c.ana), epoch, roomA)
	put(userOf(c.caio), epoch, roomB)
	put(userOf(c.bia), epoch-1, roomOld) // an older epoch reads as empty
	put(userOf(c.dani), epoch, roomLeft) // Sálvia's player has a memory, then leaves
	// A member whose player is gone is not in the party; a dead character's player keeps their memory.
	if _, err := c.h.pool.Exec(ctx, `UPDATE characters SET player_user_id = NULL WHERE id = $1`, c.salvia.GetId()); err != nil {
		t.Fatalf("remove Sálvia's player: %v", err)
	}
	if _, err := c.h.pool.Exec(ctx, `UPDATE characters SET status = 'dead', died_at = now() WHERE id = $1`, c.toren.GetId()); err != nil {
		t.Fatalf("kill Toren: %v", err)
	}
	c.groupVision(t, true)
	tv := squaresOf(t, c.tvMap(t, shown).Vision, 5)
	at := func(sq grid.Square) int { return sq.Row*g.Columns + sq.Col }
	if !tv[at(roomA)] || !tv[at(roomB)] {
		t.Errorf("the TV remembers %v, want Ana's room %d and Caio's room %d", tv, at(roomA), at(roomB))
	}
	if tv[at(roomOld)] {
		t.Error("the TV remembers a square of an older epoch")
	}
	if tv[at(roomLeft)] {
		t.Error("the TV remembers a square of a member who left the party")
	}
}

// With the fog on and no group vision the TV has no tile: the route answers as for a missing tile.
func TestTableScreenTilesAreRefusedWithoutGroupVision(t *testing.T) {
	t.Parallel()
	c := newCave(t)
	c.groupVision(t, true)
	tile := c.tvMap(t, shown).Vision.GetTiles()[0]
	path := TilesPath + c.mapID + "/tiles/" + itoa(tile.GetTx()) + "/" + itoa(tile.GetTy()) + "?as=tv"
	if got := c.master.get(path); got.status != http.StatusOK {
		t.Fatalf("the TV's tile with group vision = %d, want 200", got.status)
	}
	c.groupVision(t, false)
	missing := c.master.get(TilesPath + c.mapID + "/tiles/99/99?as=tv")
	got := c.master.get(path)
	if got.status != http.StatusNotFound || got.status != missing.status || !bytes.Equal(got.body, missing.body) {
		t.Errorf("the TV's tile without group vision = %d %q, want the missing tile's %d %q", got.status, got.body, missing.status, missing.body)
	}
}

// A random UUID or timestamp holds a canary's digits now and then; the search is by field, so it never trips on one.
func TestCanariesAreSearchedAsNumbersNotAsDigitsInText(t *testing.T) {
	t.Parallel()
	c := canaries{names: []string{"CanarioTesouro"}, numbers: []float64{4321}}
	// The old search: the number as text, in the whole reply.
	old := func(body string) bool { return strings.Contains(body, "4321") } // canary-ok: the old search, kept to show the case trips it
	clean := `{"image":{"id":"1d07fab1-fc5a-4967-83f9-13443215479a"},"revision":"20264321","at":"2026-11-04T13:43:21Z"}`
	if !old(clean) {
		t.Fatal("the crafted reply does not hold the digits: the case proves nothing")
	}
	if got := c.leaksIn(clean, nil); len(got) != 0 {
		t.Errorf("a reply with no canary leaks %v", got)
	}
	for _, leaky := range []string{`{"treasureValuePo":4321}`, `{"a":[{"b":4321}]}{"c":1}`, `{"x":"CanarioTesouro"}`} {
		if got := c.leaksIn(leaky, nil); len(got) == 0 {
			t.Errorf("a reply with a canary (%s) leaks nothing: the control is void", leaky)
		}
	}
}
