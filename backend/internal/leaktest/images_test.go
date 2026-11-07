package leaktest

import (
	"fmt"
	"net/http"
	"slices"
	"strings"
	"testing"

	"connectrpc.com/connect"

	mapsv1 "github.com/PuraFome/meuRPG/backend/gen/meurpg/maps/v1"
)

// anonymous is someone with no session at all: the routes must refuse them too.
func (w *world) anonymous() *person {
	return w.personWith("anônimo", "")
}

// TestImagesAndTiles checks the download routes: a picture is served only to who may see
// it now, and a hidden one is "404", the same as a picture that does not exist (RN-10). The
// positive controls are the pictures the master showed, left or put on the stage, and the
// tiles of what each character sees.
func checkImagesAndTiles(t *testing.T, w *world) {
	t.Helper()
	players := w.players()
	anon := w.anonymous()

	type pic struct {
		name   string
		id     string
		allow  []*person // besides the master, who may always fetch it
		thumbs bool
	}
	table := []pic{
		{"the shown image", w.imgShown, []*person{w.ana, w.caio}, true},
		{"an image left with the players", w.imgLeft, []*person{w.ana, w.caio}, true},
		{"the portrait of an NPC on the stage", w.imgStage, []*person{w.ana, w.caio}, true},
		{"the picture on a line for Ana alone (RN-36)", w.imgLine, []*person{w.ana}, true},
		{"the portrait of an NPC hidden on the stage", w.imgHiddenStage, nil, true},
		{"the portrait of an NPC out of the scene", w.imgPortrait, nil, true},
		{"an image nobody was shown", w.imgUnshown, nil, true},
		{"an image attached to a scene, never shown", w.imgSceneA, nil, true},
		{"a generated image not shown", w.imgGenerated, nil, true},
		{"the image of a hidden map", w.imgHiddenMap, nil, true},
		{"the image of a hidden dungeon", w.imgDungeon, nil, true},
		{"the image of the fog map (players get tiles)", w.imgMap, nil, true},
	}
	for _, c := range table {
		paths := []string{"/images/" + c.id}
		if c.thumbs {
			paths = append(paths, "/images/"+c.id+"/thumb")
		}
		for _, path := range paths {
			if status, body := w.master.get(path); status != http.StatusOK {
				t.Errorf("%s, %s: the master got HTTP %d %s, want 200 (the positive control)", c.name, path, status, shorten(body))
			}
			for _, p := range append(players, anon) {
				want := http.StatusNotFound
				if slicesContains(c.allow, p) {
					want = http.StatusOK
				}
				status, body := p.get(path)
				switch {
				case want == http.StatusOK && status != http.StatusOK:
					t.Errorf("%s, %s: %s got HTTP %d, want 200: the picture is theirs (the control)", c.name, path, p.name, status)
				case want == http.StatusNotFound && status == http.StatusOK:
					t.Errorf("%s, %s: %s got the picture, want 404 (RN-10)", c.name, path, p.name)
				case want == http.StatusNotFound && status != http.StatusNotFound && status != http.StatusUnauthorized:
					t.Errorf("%s, %s: %s got HTTP %d, want 404 like a picture that does not exist", c.name, path, p.name, status)
				}
				for _, f := range w.inspect(p, reply{status: status, body: body}, nil) {
					t.Errorf("%s, %s: %s: %s", c.name, path, p.name, f)
				}
			}
		}
	}

	// The tiles of the fog map: a player fetches exactly the pieces their character has
	// seen or remembers, and a piece without a square they know does not exist.
	vis := map[*person]*mapsv1.GetMapVisionResponse{w.ana: w.vision(w.ana, w.fogMap), w.caio: w.vision(w.caio, w.fogMap)}
	for _, p := range []*person{w.ana, w.caio} {
		if len(vis[p].GetTiles()) == 0 {
			t.Fatalf("%s has no tile to fetch: the fixture's fog map is not what the test assumes", p.name)
		}
	}
	if sameTiles(vis[w.ana], vis[w.caio]) {
		t.Fatalf("Ana and Caio see the same tiles: the fixture cannot tell a tile that is theirs from one that is not")
	}
	for _, p := range players {
		v := vis[p]
		per := 16 // the fixture's tile size: TestFixtureShape checks that the server says the same
		if v != nil && v.GetTileSquares() > 0 {
			per = int(v.GetTileSquares())
		}
		base := "/images/maps/" + w.fogMap + "/tiles/"
		for ty := 0; ty*per < 16; ty++ {
			for tx := 0; tx*per < caveColumns; tx++ {
				known := false
				if v != nil {
					for _, tile := range v.GetTiles() {
						known = known || (int(tile.GetTx()) == tx && int(tile.GetTy()) == ty)
					}
				}
				status, _ := p.get(fmt.Sprintf("%s%d/%d", base, tx, ty))
				switch {
				case known && status != http.StatusOK:
					t.Errorf("tile %d,%d: %s got HTTP %d, want 200: their character has seen it", tx, ty, p.name, status)
				case !known && status == http.StatusOK:
					t.Errorf("tile %d,%d: %s got a tile that holds no square their character knows (RN-10)", tx, ty, p.name)
				}
			}
		}
	}
	for _, p := range append(players, anon) {
		for _, other := range []string{w.hiddenMap, w.dungeonMap} {
			path := strings.Replace("/images/maps/"+w.fogMap+"/tiles/0/0", w.fogMap, other, 1)
			if status, _ := p.get(path); status == http.StatusOK {
				t.Errorf("%s got a tile of a map that is not theirs (%s)", p.name, path)
			}
		}
	}
	// A tile of the fog map for someone with no character that sees nothing: the answer must not differ.
	for _, p := range []*person{w.pending, w.stranger, anon} {
		if status, _ := p.get("/images/maps/" + w.fogMap + "/tiles/0/0"); status == http.StatusOK {
			t.Errorf("%s got a tile of the fog map", p.name)
		}
	}
	// The TV's tiles (`as=tv`, the master's, for the table screen): "Visão do grupo" is off in the fixture, so the TV has
	// no tile of the fog map. Nobody gets one, the master neither: every answer is the one of a tile that does not exist.
	// (With "Visão do grupo" on the master gets 200 and the others the same 404: TestRN10_TheTVOfAFogMapReadsOnlyWhatTheGroupSees.)
	known := vis[w.ana].GetTiles()[0]
	checkTVTiles(t, w, "group vision off", "/images/maps/"+w.fogMap+"/tiles/", known, false)
	_ = connect.CodeNotFound
}

func slicesContains(ps []*person, p *person) bool {
	return slices.Contains(ps, p)
}

func sameTiles(a, b *mapsv1.GetMapVisionResponse) bool {
	if len(a.GetTiles()) != len(b.GetTiles()) {
		return false
	}
	for i := range a.GetTiles() {
		if a.GetTiles()[i].GetTx() != b.GetTiles()[i].GetTx() || a.GetTiles()[i].GetTy() != b.GetTiles()[i].GetTy() {
			return false
		}
	}
	return true
}
