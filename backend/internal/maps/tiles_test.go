package maps

import (
	"bytes"
	"encoding/binary"
	"errors"
	"fmt"
	"hash/crc32"
	"image"
	"image/color"
	"image/jpeg"
	"image/png"
	"io"
	"net/http"
	"os"
	"runtime"
	"slices"
	"strconv"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"connectrpc.com/connect"

	mapsv1 "github.com/PuraFome/meuRPG/backend/gen/meurpg/maps/v1"
	"github.com/PuraFome/meuRPG/backend/internal/platform/dbtest"
	"github.com/PuraFome/meuRPG/backend/internal/platform/ratelimit"
	"github.com/PuraFome/meuRPG/backend/internal/rules/grid"
)

// The fog's image as tiles per player (MR-036, RN-10, D6; ADR-0016). The cave's
// image is 240 x 160 pixels for a grid of 24 x 16 squares: 10 pixels a square, so
// the working copy is the image itself and every pixel is known.

// patternAt is the color of the test image at a pixel: unique enough that a
// pixel of the wrong square, or a blurred one, shows.
func patternAt(x, y int) color.NRGBA {
	return color.NRGBA{R: uint8(x), G: uint8(y), B: uint8(x*7 + y*13), A: 255} //nolint:gosec // G115: a test pattern, wrapping is the point
}

// patternImage is a w x h PNG of patternAt.
func patternImage(t testing.TB, w, h int) []byte {
	t.Helper()
	img := image.NewNRGBA(image.Rect(0, 0, w, h))
	for y := range h {
		for x := range w {
			img.SetNRGBA(x, y, patternAt(x, y))
		}
	}
	var buf bytes.Buffer
	if err := png.Encode(&buf, img); err != nil {
		t.Fatal(err)
	}
	return buf.Bytes()
}

// tileURL is the path of a tile as GetMapVision says to build it.
func tileURL(res *mapsv1.GetMapVisionResponse, tile *mapsv1.MapTile) string {
	return res.GetTilesPath() + strconv.Itoa(int(tile.GetTx())) + "/" + strconv.Itoa(int(tile.GetTy())) + "?r=" + strconv.Itoa(int(tile.GetRevision()))
}

func decodeTile(t *testing.T, r httpResult) image.Image {
	t.Helper()
	if r.status != http.StatusOK {
		t.Fatalf("tile status = %d, body %s; want 200", r.status, r.body)
	}
	if ct := r.header.Get("Content-Type"); ct != "image/png" {
		t.Fatalf("tile Content-Type = %q", ct)
	}
	img, err := png.Decode(bytes.NewReader(r.body))
	if err != nil {
		t.Fatalf("a tile is not a PNG: %v", err)
	}
	return img
}

// checkTile asserts what RN-10 asks of a tile: every pixel of a square the viewer
// does not know is opaque black, and a pixel of a known square is the image's
// unless it is within a pixel of an unknown square (the belt). It returns how many
// squares of each kind the tile has.
func checkTile(t *testing.T, who string, img image.Image, res *mapsv1.GetMapVisionResponse, tile *mapsv1.MapTile) (shown, black int) {
	t.Helper()
	return checkTileAt(t, who, img, res, tile, 10)
}

// checkTileAt is checkTile for a map whose squares are sq pixels wide (the cave's are
// 10; calibrated to 2, they are 5).
func checkTileAt(t *testing.T, who string, img image.Image, res *mapsv1.GetMapVisionResponse, tile *mapsv1.MapTile, sq int) (shown, black int) {
	t.Helper()
	g := grid.Grid{Columns: int(res.GetGridColumns()), Rows: int(res.GetGridRows())}
	pv := codes(t, res)
	c0, r0 := int(tile.GetTx())*tileSquares, int(tile.GetTy())*tileSquares
	wantW, wantH := min(g.Columns-c0, tileSquares)*sq, min(g.Rows-r0, tileSquares)*sq
	if b := img.Bounds(); b.Dx() != wantW || b.Dy() != wantH {
		t.Fatalf("%s: tile (%d,%d) is %dx%d px, want %dx%d", who, tile.GetTx(), tile.GetTy(), b.Dx(), b.Dy(), wantW, wantH)
	}
	knownAt := func(px, py int) bool { // a pixel of the whole map; outside it is known
		col, row := px/sq, py/sq
		if px < 0 || py < 0 || col >= g.Columns || row >= g.Rows {
			return true
		}
		return pv[row*g.Columns+col] != 0
	}
	for y := range wantH {
		for x := range wantW {
			px, py := c0*sq+x, r0*sq+y
			col, row := px/sq, py/sq
			got := color.NRGBAModel.Convert(img.At(x, y)).(color.NRGBA)
			if !knownAt(px, py) {
				if got != (color.NRGBA{A: 255}) {
					t.Fatalf("%s: pixel (%d,%d) of an unseen square (%d,%d) = %v, want opaque black: the tile carries what the player has not seen", who, x, y, col, row, got)
				}
				continue
			}
			edge := false
			for dy := -1; dy <= 1; dy++ {
				for dx := -1; dx <= 1; dx++ {
					edge = edge || !knownAt(px+dx, py+dy)
				}
			}
			if want := patternAt(px, py); !edge && got != want {
				t.Fatalf("%s: pixel (%d,%d) of a seen square (%d,%d) = %v, want the image's %v", who, x, y, col, row, got, want)
			}
		}
	}
	for dr := range min(g.Rows-r0, tileSquares) {
		for dc := range min(g.Columns-c0, tileSquares) {
			if pv[(r0+dr)*g.Columns+c0+dc] != 0 {
				shown++
			} else {
				black++
			}
		}
	}
	return shown, black
}

func TestRN10_TilesCarryOnlyWhatThePlayerSaw(t *testing.T) {
	t.Parallel()
	c := newCave(t)
	ana, dani := c.ana.mustVision(c.campaign, c.mapID), c.dani.mustVision(c.campaign, c.mapID)
	if ana.GetTileSquares() != tileSquares || ana.GetTilesPath() != TilesPath+c.mapID+"/tiles/" {
		t.Fatalf("tile_squares = %d, tiles_path = %q", ana.GetTileSquares(), ana.GetTilesPath())
	}
	if len(ana.GetTiles()) == 0 {
		t.Fatal("Ana sees part of the cave and has no tile")
	}
	partial := false
	key := func(tile *mapsv1.MapTile) string { return fmt.Sprint(tile.GetTx(), ",", tile.GetTy()) }
	bodies := map[string][]byte{}
	for _, tile := range ana.GetTiles() {
		r := c.ana.get(tileURL(ana, tile))
		shown, black := checkTile(t, "Ana", decodeTile(t, r), ana, tile)
		if shown == 0 {
			t.Errorf("tile (%d,%d) is in the index with no known square", tile.GetTx(), tile.GetTy())
		}
		partial = partial || black > 0
		bodies[key(tile)] = r.body
	}
	if !partial {
		t.Fatal("every tile Ana got was fully known: the test would not catch a leak")
	}
	// A tile outside the index is a 404 (here every tile has a square she knows: the
	// guard room's torch lights part of the second one).
	listed := map[[2]int32]bool{}
	for _, tile := range ana.GetTiles() {
		listed[[2]int32{tile.GetTx(), tile.GetTy()}] = true
	}
	for ty := range int32(1) {
		for tx := range int32(2) {
			if r := c.ana.get(ana.GetTilesPath() + fmt.Sprint(tx, "/", ty)); listed[[2]int32{tx, ty}] != (r.status == http.StatusOK) {
				t.Errorf("tile %d/%d: listed %v, status %d", tx, ty, listed[[2]int32{tx, ty}], r.status)
			}
		}
	}
	// Outside the grid, or not numbers: 404 as well.
	for _, p := range []string{"0/5", "7/0", "-1/0", "a/0", "0/"} {
		if r := c.ana.get(ana.GetTilesPath() + p); r.status != http.StatusNotFound {
			t.Errorf("tile %s: status %d, want 404", p, r.status)
		}
	}

	// Another player's tiles are built from that player's memory.
	differs := false
	for _, tile := range dani.GetTiles() {
		r := c.dani.get(tileURL(dani, tile))
		checkTile(t, "Sálvia", decodeTile(t, r), dani, tile)
		if !bytes.Equal(r.body, bodies[key(tile)]) {
			differs = true
		}
	}
	if !differs && !slices.Equal(codes(t, ana), codes(t, dani)) {
		t.Error("two players with different views got identical tiles")
	}

	// Walking on makes the tile gain pixels, never lose them: the revision changes.
	c.master.placeAt(c.campaign, c.mapID, c.pens.GetId(), grid.Square{Col: 12, Row: 7})
	after := c.ana.mustVision(c.campaign, c.mapID)
	if after.GetTiles()[0].GetRevision() == ana.GetTiles()[0].GetRevision() && slices.Equal(codes(t, after), codes(t, ana)) {
		t.Fatal("Pensantus moved and nothing changed: the test does not move her far enough")
	}
	for _, tile := range after.GetTiles() {
		checkTile(t, "Ana after walking", decodeTile(t, c.ana.get(tileURL(after, tile))), after, tile)
	}

	// The master gets the whole image, as before, and no tiles; a player never does.
	if res := c.master.mustVision(c.campaign, c.mapID); len(res.GetTiles()) != 0 || res.GetTilesPath() != "" {
		t.Errorf("the master got tiles: %v", res)
	}
	if r := c.master.get(ana.GetTilesPath() + "0/0"); r.status != http.StatusNotFound {
		t.Errorf("the master asked for a tile: status %d, want 404 (the master reads the image)", r.status)
	}
	full := c.master.get(ImagesPath + c.imageID)
	if full.status != http.StatusOK || len(full.body) == 0 {
		t.Fatalf("the master's image: status %d", full.status)
	}
	if r := c.ana.get(ImagesPath + c.imageID); r.status != http.StatusNotFound {
		t.Errorf("a player asked for the fog map's image: status %d, want 404", r.status)
	}
	// "Ver como": the master reads the tile as that player does.
	asAna := c.master.get(ana.GetTilesPath() + "0/0?as=" + c.pens.GetId())
	want := c.ana.mustVision(c.campaign, c.mapID)
	if tile := want.GetTiles()[0]; tile.GetTx() == 0 && tile.GetTy() == 0 {
		if own := c.ana.get(tileURL(want, tile)); !bytes.Equal(own.body, asAna.body) {
			t.Error("the master's \"Ver como\" tile differs from the player's own")
		}
	}
	if r := c.ana.get(ana.GetTilesPath() + "0/0?as=" + c.salvia.GetId()); r.status != http.StatusNotFound {
		t.Errorf("a player sent as=: status %d, want 404", r.status)
	}
	// The pixels outside the mask are never in the bytes: no tile is the full image's.
	if bytes.Contains(full.body, bodies[key(&mapsv1.MapTile{})]) {
		t.Error("a tile is a slice of the stored file")
	}
}

func TestRN10_APlayerWhoSawNothingGetsNoTile(t *testing.T) {
	t.Parallel()
	c := newCave(t)
	eva := c.h.newUser("Eva")
	c.h.join(c.master, c.campaign, false, eva)
	eva.pc(c.campaign, "Eva", "race:human") // a character, with no token on the map
	res := eva.mustVision(c.campaign, c.mapID)
	if len(res.GetTiles()) != 0 || res.GetCharacterOnMap() {
		t.Fatalf("a player with no token got tiles %v (on map %v)", res.GetTiles(), res.GetCharacterOnMap())
	}
	if r := eva.get(res.GetTilesPath() + "0/0"); r.status != http.StatusNotFound {
		t.Errorf("tile 0/0 of a player who saw nothing: status %d, want 404", r.status)
	}
}

// Authorization: the tile's is the map's (the matrix row of the image routes).
func TestRN10_TileAuthorization(t *testing.T) {
	t.Parallel()
	c := newCave(t)
	good := c.ana.mustVision(c.campaign, c.mapID)
	path := tileURL(good, good.GetTiles()[0])
	if r := c.ana.get(path); r.status != http.StatusOK {
		t.Fatalf("Ana's own tile: status %d", r.status)
	}
	outsider := c.h.newUser("Fora")
	pending := c.h.newUser("Pendente")
	c.h.join(c.master, c.campaign, true, pending)
	for name, u := range map[string]*user{"a non-member": outsider, "a pending member": pending} {
		if r := u.get(path); r.status != http.StatusNotFound {
			t.Errorf("%s: status %d, want 404", name, r.status)
		}
	}
	if r := c.h.anonymous().get(path); r.status != http.StatusUnauthorized {
		t.Errorf("without a session: status %d, want 401", r.status)
	}
	if r := c.ana.get(TilesPath + "6f1c7a52-3b5e-4c55-9d0b-2a51f0c1e0ff/tiles/0/0"); r.status != http.StatusNotFound {
		t.Errorf("a map that does not exist: status %d, want 404", r.status)
	}
	// The master hides the map: a player who knew the URL gets 404 (RN-10).
	c.master.setMapRevealed(c.campaign, c.mapID, false)
	if _, err := c.master.setCurrentMap(c.campaign, c.probeMap); err != nil {
		t.Fatal(err)
	}
	if r := c.ana.get(path); r.status != http.StatusNotFound {
		t.Errorf("a hidden map's tile: status %d, want 404", r.status)
	}
	c.master.setMapRevealed(c.campaign, c.mapID, true)
	if r := c.ana.get(path); r.status != http.StatusOK {
		t.Errorf("a revealed map's tile: status %d, want 200", r.status)
	}
	// With the fog off there are no tiles: the player reads the image.
	if _, err := c.master.maps.SetMapFog(t.Context(), connect.NewRequest(&mapsv1.SetMapFogRequest{CampaignId: c.campaign, MapId: c.mapID, FogEnabled: new(false)})); err != nil {
		t.Fatal(err)
	}
	if r := c.ana.get(path); r.status != http.StatusNotFound {
		t.Errorf("a map without the fog: status %d, want 404", r.status)
	}
	if res := c.ana.mustVision(c.campaign, c.mapID); len(res.GetTiles()) != 0 {
		t.Errorf("a map without the fog lists tiles: %v", res.GetTiles())
	}
}

func TestRN10_TileCaching(t *testing.T) {
	t.Parallel()
	c := newCave(t)
	res := c.ana.mustVision(c.campaign, c.mapID)
	path := tileURL(res, res.GetTiles()[0])
	first := c.ana.get(path)
	if cc := first.header.Get("Cache-Control"); cc != "private, no-cache" || first.header.Get("Vary") != "Cookie" || first.header.Get("ETag") == "" {
		t.Errorf("headers: Cache-Control %q, Vary %q, ETag %q", cc, first.header.Get("Vary"), first.header.Get("ETag"))
	}
	if r := c.ana.get(path, "If-None-Match", first.header.Get("ETag")); r.status != http.StatusNotModified || len(r.body) != 0 {
		t.Errorf("If-None-Match: status %d, %d bytes, want 304", r.status, len(r.body))
	}
	// Another player's ETag is not Ana's tile: a 304 needs the same mask.
	if r := c.dani.get(path, "If-None-Match", first.header.Get("ETag")); r.status == http.StatusNotModified && !slices.Equal(c.dani.mustVision(c.campaign, c.mapID).GetStates(), res.GetStates()) {
		t.Error("a tile was reported unchanged to a player whose mask differs")
	}
	// The same tile again is the cache's: one render for both asks.
	before := c.h.svc.tiles.decodes
	c.ana.get(path)
	if c.h.svc.tiles.decodes != before {
		t.Error("a repeated tile decoded the image again")
	}
}

// Invalidation (D6): a new image, a new grid and "Esquecer" drop the working copy
// and the tiles of the map.
func TestRN10_TilesAreInvalidated(t *testing.T) {
	t.Parallel()
	c := newCave(t)
	m := c.master
	tr := c.h.svc.tiles
	fetch := func() *mapsv1.GetMapVisionResponse {
		t.Helper()
		res := c.ana.mustVision(c.campaign, c.mapID)
		for _, tile := range res.GetTiles() {
			c.ana.get(tileURL(res, tile))
		}
		return res
	}
	filled := func(what string) {
		t.Helper()
		if n, _ := tr.cache.size(); n == 0 || !slices.Contains(tr.working.maps(), c.mapID) {
			t.Fatalf("%s: nothing cached (%d tiles, copies of %v)", what, n, tr.working.maps())
		}
	}
	empty := func(what string) {
		t.Helper()
		if n, _ := tr.cache.size(); n != 0 || slices.Contains(tr.working.maps(), c.mapID) {
			t.Fatalf("%s: %d tiles and copies of %v are still cached", what, n, tr.working.maps())
		}
	}

	fetch()
	filled("first")
	// Forgetting the memory.
	if _, err := m.maps.ForgetMapVision(t.Context(), connect.NewRequest(&mapsv1.ForgetMapVisionRequest{CampaignId: c.campaign, MapId: c.mapID})); err != nil {
		t.Fatal(err)
	}
	empty("after forgetting what was seen")
	fetch()
	filled("second")

	// A new grid.
	if _, err := m.setGrid(c.campaign, c.mapID, 20); err != nil {
		t.Fatal(err)
	}
	empty("after a new grid")
	if g := c.ana.mustVision(c.campaign, c.mapID); g.GetGridColumns() != 20 {
		t.Fatalf("grid = %d columns", g.GetGridColumns())
	}

	// A new image: other bytes, the same grid; a fight cannot be running.
	img := m.mustUpload(c.campaign, "outra.png", patternImage(t, 200, 200)).GetId()
	m.mustSetGrid(c.campaign, c.mapID, 24)
	fetch()
	filled("before the new image")
	cur := m.mustGetMap(c.campaign, c.mapID).GetMap()
	if _, err := m.maps.UpdateMap(t.Context(), connect.NewRequest(&mapsv1.UpdateMapRequest{CampaignId: c.campaign, MapId: c.mapID, Revision: cur.GetRevision(), ImageId: new(img)})); err != nil {
		t.Fatalf("UpdateMap(image) error = %v", err)
	}
	empty("after a new image")
}

// ---- the render budget: pure tests, no database ----

func TestTileBudget_WorkingCopyIsCappedAt2048(t *testing.T) {
	t.Parallel()
	for _, tc := range []struct{ w, h, wantW, wantH int }{
		{4096, 2048, 2048, 1024},
		{2048, 4096, 1024, 2048},
		{2000, 1000, 2000, 1000}, // under the cap: untouched
	} {
		g := grid.Grid{Columns: 40, Rows: grid.RowsFor(40, tc.w, tc.h)}
		wc, sw, sh, err := decodeWorkingCopy(patternImage(t, tc.w, tc.h), g)
		if err != nil {
			t.Fatal(err)
		}
		if b := wc.Bounds(); b.Dx() != tc.wantW || b.Dy() != tc.wantH || sw != tc.w || sh != tc.h {
			t.Errorf("%dx%d decoded to %dx%d (source %dx%d), want %dx%d", tc.w, tc.h, b.Dx(), b.Dy(), sw, sh, tc.wantW, tc.wantH)
		}
	}
}

// The decode is bounded by the header, before any pixel is read.
func TestTileBudget_ADecodeOverTheLimitIsRefused(t *testing.T) {
	t.Parallel()
	var buf bytes.Buffer
	img := image.NewRGBA64(image.Rect(0, 0, 4096, 8192)) // 33 megapixels of 8 bytes: 268 MB decoded
	if err := (&png.Encoder{CompressionLevel: png.BestSpeed}).Encode(&buf, img); err != nil {
		t.Fatal(err)
	}
	if _, _, _, err := decodeWorkingCopy(buf.Bytes(), grid.Grid{Columns: 64, Rows: 128}); !errors.Is(err, errTooBig) {
		t.Errorf("a 16-bit PNG of 33 megapixels: error = %v, want errTooBig", err)
	}
	if _, _, _, err := decodeWorkingCopy([]byte("GIF89a"), grid.Grid{Columns: 1, Rows: 1}); err == nil {
		t.Error("a GIF was decoded")
	}
}

// A gray PNG with a tRNS chunk decodes to NRGBA64 when it has 16 bits, though its header
// names gray at 2 bytes a pixel: the budget counts the chunk.
func TestTileBudget_AGrayPNGWithATransparentColorIsPricedAsDecoded(t *testing.T) {
	t.Parallel()
	const w, h = 4096, 8192 // 33 megapixels: 67 MB as gray16, 268 MB as the NRGBA64 it decodes to
	var plain bytes.Buffer
	if err := (&png.Encoder{CompressionLevel: png.BestSpeed}).Encode(&plain, image.NewGray16(image.Rect(0, 0, w, h))); err != nil {
		t.Fatal(err)
	}
	g := grid.Grid{Columns: 64, Rows: 128}
	// The same file with a tRNS chunk (the transparent gray value) after the header.
	data := plain.Bytes()
	const afterHeader = 8 + 4 + 4 + 13 + 4 // signature, IHDR's length, type, data and CRC
	chunk := append(binary.BigEndian.AppendUint32(nil, 2), "tRNS\x00\x00"...)
	chunk = binary.BigEndian.AppendUint32(chunk, crc32.ChecksumIEEE(chunk[4:]))
	withKey := append(append(append([]byte{}, data[:afterHeader]...), chunk...), data[afterHeader:]...)
	if cfg, err := png.DecodeConfig(bytes.NewReader(withKey)); err != nil || cfg.ColorModel != color.Gray16Model {
		t.Fatalf("the fixture's header = %v, %v; want gray16", cfg.ColorModel, err)
	}
	if _, _, _, err := decodeWorkingCopy(withKey, g); !errors.Is(err, errTooBig) {
		t.Errorf("a 16-bit gray PNG with a tRNS chunk of 33 megapixels: error = %v, want errTooBig", err)
	}
	// The control: without the chunk the same image is gray16 and fits.
	if _, _, _, err := decodeWorkingCopy(data, g); err != nil {
		t.Errorf("the same image without the chunk: error = %v, want it decoded", err)
	}
}

func TestTileBudget_TheLRUKeepsTwoMaps(t *testing.T) {
	t.Parallel()
	var w workingSet
	for _, id := range []string{"a", "b", "c"} {
		w.put(&workingCopy{mapID: id, imageID: "i", cols: 1, rows: 1})
		if id == "b" {
			w.get("a", "i", grid.Grid{Columns: 1, Rows: 1}) // a is the most recently used
		}
	}
	if got := w.maps(); !slices.Equal(got, []string{"c", "a"}) {
		t.Errorf("the LRU holds %v, want [c a]: the third map evicts the least recently used", got)
	}
	w.put(&workingCopy{mapID: "c", imageID: "j", cols: 1, rows: 1}) // the same map's new image replaces its copy
	if got := w.maps(); len(got) != 2 || got[0] != "c" {
		t.Errorf("after a new image the LRU holds %v", got)
	}
	if w.get("c", "i", grid.Grid{Columns: 1, Rows: 1}) != nil {
		t.Error("the old image's working copy is still served")
	}
}

func TestTileBudget_TheTileCacheIsBounded(t *testing.T) {
	t.Parallel()
	c := tileCache{max: 1000}
	for i := range 50 {
		c.put("m", tileKey{tx: i}, make([]byte, 100))
		if _, n := c.size(); n > 1000 {
			t.Fatalf("the cache holds %d bytes, over its 1000", n)
		}
	}
	if e, n := c.size(); e != 10 || n != 1000 {
		t.Errorf("cache = %d tiles, %d bytes, want 10 and 1000", e, n)
	}
	if c.get(tileKey{tx: 0}) != nil || c.get(tileKey{tx: 49}) == nil {
		t.Error("the least recently used tile did not go first")
	}
	c.put("m", tileKey{tx: 99}, make([]byte, 5000)) // larger than the whole cache: not kept
	if c.get(tileKey{tx: 99}) != nil {
		t.Error("a tile larger than the cache was kept")
	}
	c.forget("m")
	if e, n := c.size(); e != 0 || n != 0 {
		t.Errorf("after forget: %d tiles, %d bytes", e, n)
	}
}

// One render at a time, and a 503 with Retry-After for the one that waits too long.
func TestTileBudget_OneRenderAtATime(t *testing.T) {
	t.Parallel()
	dbtest.PoolSize(t, 8) // the racers must overlap: one connection would run them one by one
	c := newCave(t)
	res := c.ana.mustVision(c.campaign, c.mapID)
	tr := c.h.svc.tiles

	var running, peak, renders atomic.Int32
	tr.onRender = func() {
		n := running.Add(1)
		for {
			if p := peak.Load(); n <= p || peak.CompareAndSwap(p, n) {
				break
			}
		}
		renders.Add(1)
		time.Sleep(30 * time.Millisecond)
		running.Add(-1)
	}
	// Several players ask for several tiles at the same time: different masks, so different renders.
	var wg sync.WaitGroup
	for _, u := range []*user{c.ana, c.caio, c.bia, c.dani} {
		view := u.mustVision(c.campaign, c.mapID)
		for _, tile := range view.GetTiles() {
			wg.Go(func() {
				if r := u.get(tileURL(view, tile)); r.status != http.StatusOK {
					t.Errorf("tile status %d", r.status)
				}
			})
		}
	}
	wg.Wait()
	if renders.Load() < 2 {
		t.Fatalf("only %d renders happened: the test proves nothing", renders.Load())
	}
	if peak.Load() != 1 {
		t.Errorf("%d tiles were rendered at the same time, want 1", peak.Load())
	}

	// The gate is taken and the wait is short: 503 with Retry-After.
	tr.cache.forget(c.mapID)
	tr.wait = 50 * time.Millisecond
	tr.gate <- struct{}{}
	r := c.ana.get(tileURL(res, res.GetTiles()[0]))
	<-tr.gate
	if r.status != http.StatusServiceUnavailable || r.header.Get("Retry-After") == "" || r.header.Get("Cache-Control") != "no-store" {
		t.Errorf("a busy server: status %d, Retry-After %q, Cache-Control %q; want 503 with Retry-After", r.status, r.header.Get("Retry-After"), r.header.Get("Cache-Control"))
	}
	if eb := r.errorBody(t); eb.Reason != "BUSY" {
		t.Errorf("reason = %q", eb.Reason)
	}
	// And it works again afterwards.
	tr.wait = renderWait
	tr.onRender = nil
	if r := c.ana.get(tileURL(res, res.GetTiles()[0])); r.status != http.StatusOK {
		t.Errorf("after the wait: status %d", r.status)
	}
}

// Only the tiles with a known square exist: of a 40 x 40 grid (3 x 3 tiles), a
// view that knows one square of the first tile and one of the last has those two.
func TestRN10_TheIndexListsOnlyTilesWithAKnownSquare(t *testing.T) {
	t.Parallel()
	g := grid.Grid{Columns: 40, Rows: 40}
	src := tileSource{imageID: "image", g: g, width: 400, height: 400}
	pv := &playerView{g: g, codes: make([]byte, g.Squares())}
	pv.codes[3*g.Columns+4] = stateRemembered
	pv.codes[39*g.Columns+39] = byte(4)
	tiles := buildTiles(pv, src).tiles
	if len(tiles) != 2 || tiles[0].tx != 0 || tiles[0].ty != 0 || tiles[1].tx != 2 || tiles[1].ty != 2 {
		t.Fatalf("tiles = %v, want (0,0) and (2,2)", tiles)
	}
	more := &playerView{g: g, codes: slices.Clone(pv.codes)}
	more.codes[3*g.Columns+5] = stateRemembered
	if buildTiles(more, src).tiles[0].revision == tiles[0].revision {
		t.Error("a tile that gained a square kept its revision")
	}
	other := src
	other.imageID = "other"
	if buildTiles(pv, other).tiles[1].revision == tiles[1].revision {
		t.Error("a new image kept the tile's revision")
	}
}

// ---- the oracle ----

// oracleSquare is the square's pixel rectangle in a w x h image, as the tiles divide it.
func oracleSquare(g grid.Grid, w, h, c, r int) image.Rectangle {
	return image.Rect(pixelAt(c, g.Columns, w), pixelAt(r, g.Rows, h), pixelAt(c+1, g.Columns, w), pixelAt(r+1, g.Rows, h))
}

// oracleImage is a w x h image: a textured gradient in the known squares and, in
// the unknown ones, a flat color (dark in one image, bright in the other).
func oracleImage(g grid.Grid, w, h int, known func(c, r int) bool, unknown color.NRGBA) *image.NRGBA {
	img := image.NewNRGBA(image.Rect(0, 0, w, h))
	for y := range h {
		for x := range w {
			img.SetNRGBA(x, y, color.NRGBA{R: uint8(x*255/w) ^ uint8(y&7*9), G: uint8(y * 255 / h), B: uint8((x ^ y) & 63 * 4), A: 255}) //nolint:gosec // G115: a test texture
		}
	}
	for r := range g.Rows {
		for c := range g.Columns {
			if !known(c, r) {
				rect := oracleSquare(g, w, h, c, r)
				for y := rect.Min.Y; y < rect.Max.Y; y++ {
					for x := rect.Min.X; x < rect.Max.X; x++ {
						img.SetNRGBA(x, y, unknown)
					}
				}
			}
		}
	}
	return img
}

func encodeOracle(t testing.TB, img image.Image, asJPEG bool) []byte {
	t.Helper()
	var buf bytes.Buffer
	var err error
	if asJPEG {
		err = jpeg.Encode(&buf, img, &jpeg.Options{Quality: 88}) // as images.Process stores a JPEG: 4:2:0, q88
	} else {
		err = (&png.Encoder{CompressionLevel: png.BestSpeed}).Encode(&buf, img)
	}
	if err != nil {
		t.Fatal(err)
	}
	return buf.Bytes()
}

// TestRN10_TileOracle is the acceptance test of the leak: two images that differ
// only inside the squares a player does not know must give byte-identical tiles for
// that player, whatever the format, the size (shrunk or not, by a whole factor or
// not) and the grid (even or not).
func TestRN10_TileOracle(t *testing.T) {
	t.Parallel()
	for _, tc := range []struct {
		name    string
		w, h    int
		columns int
		jpeg    bool
		stripe  int // an unknown column and row every stripe squares
	}{
		{"PNG 1:1", 240, 160, 24, false, 9},
		{"PNG 1:1, uneven grid", 250, 170, 24, false, 9},
		{"PNG 4096 px, a whole factor", 4096, 2048, 64, false, 9},
		{"PNG 4096 px, an uneven grid", 4096, 2500, 50, false, 9},
		{"PNG 3000 px, not a whole factor", 3000, 2000, 37, false, 9},
		{"JPEG 1:1", 240, 160, 24, true, 9},
		{"JPEG 1:1, uneven grid", 250, 170, 24, true, 9},
		{"JPEG 1:1, tiny squares", 400, 240, 100, true, 25},
		{"JPEG 4096 px", 4096, 2048, 64, true, 9},
		{"JPEG 3001 px, uneven grid", 3001, 1999, 41, true, 9},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			g := grid.Grid{Columns: tc.columns, Rows: grid.RowsFor(tc.columns, tc.w, tc.h)}
			known := func(c, r int) bool { return c%tc.stripe != tc.stripe/2 && r%tc.stripe != tc.stripe/2 }
			src := tileSource{g: g, width: tc.w, height: tc.h, jpeg: tc.jpeg}
			a, b := oracleImage(g, tc.w, tc.h, known, color.NRGBA{A: 255}), oracleImage(g, tc.w, tc.h, known, color.NRGBA{R: 255, G: 255, B: 255, A: 255})
			wcA, swA, shA, err := decodeWorkingCopy(encodeOracle(t, a, tc.jpeg), g)
			if err != nil {
				t.Fatal(err)
			}
			wcB, _, _, err := decodeWorkingCopy(encodeOracle(t, b, tc.jpeg), g)
			if err != nil {
				t.Fatal(err)
			}
			if bytes.Equal(wcA.Pix, wcB.Pix) {
				t.Fatal("the two images give the same working copy: the test would prove nothing")
			}
			copyA := &workingCopy{img: wcA, srcW: swA, srcH: shA}
			copyB := &workingCopy{img: wcB, srcW: swA, srcH: shA}
			cols, rows := tileGrid(g)
			shownPixels := 0
			for ty := range rows {
				for tx := range cols {
					nb := newNbhd(g, tx, ty, src.ring(), known)
					pa, err := renderTile(copyA, g, tx, ty, nb, tc.jpeg)
					if err != nil {
						t.Fatal(err)
					}
					pb, err := renderTile(copyB, g, tx, ty, nb, tc.jpeg)
					if err != nil {
						t.Fatal(err)
					}
					if !bytes.Equal(pa, pb) {
						t.Fatalf("tile (%d,%d): the tiles of two images that differ only in unknown squares differ: something of an unknown square is in the tile", tx, ty)
					}
					img, err := png.Decode(bytes.NewReader(pa))
					if err != nil {
						t.Fatal(err)
					}
					for y := range img.Bounds().Dy() {
						for x := range img.Bounds().Dx() {
							if r, g, b, _ := img.At(x, y).RGBA(); r|g|b != 0 {
								shownPixels++
							}
						}
					}
				}
			}
			if shownPixels == 0 {
				t.Fatal("every tile is black: the test would prove nothing")
			}
		})
	}
}

// The edge of a JPEG map is darker by up to a block, and nothing else changes: a
// square far from every unknown one shows its pixels exactly.
func TestRN10_JPEGKeepsTheInteriorOfAKnownArea(t *testing.T) {
	t.Parallel()
	g := grid.Grid{Columns: 24, Rows: 16}
	known := func(c, _ int) bool { return c < 12 } // the left half
	img := oracleImage(g, 240, 160, func(_, _ int) bool { return true }, color.NRGBA{})
	data := encodeOracle(t, img, true)
	wc, sw, sh, err := decodeWorkingCopy(data, g)
	if err != nil {
		t.Fatal(err)
	}
	copyOf := &workingCopy{img: wc, srcW: sw, srcH: sh}
	src := tileSource{g: g, width: 240, height: 160, jpeg: true}
	out, err := renderTile(copyOf, g, 0, 0, newNbhd(g, 0, 0, src.ring(), known), true)
	if err != nil {
		t.Fatal(err)
	}
	tile, err := png.Decode(bytes.NewReader(out))
	if err != nil {
		t.Fatal(err)
	}
	for y := range 160 {
		for x := range 160 {
			got := color.NRGBAModel.Convert(tile.At(x, y)).(color.NRGBA)
			want := color.NRGBAModel.Convert(wc.At(x, y)).(color.NRGBA)
			switch {
			case x < 110 && got != want: // the blocks clear of the unknown half (the last pixel before the edge is one the shrink widens over)
				t.Fatalf("pixel (%d,%d) in the interior is %v, want %v", x, y, got, want)
			case x >= 120 && got != (color.NRGBA{A: 255}):
				t.Fatalf("pixel (%d,%d) of an unknown square is %v", x, y, got)
			}
		}
	}
}

// ---- the viewers' tiles and the limit ----

// A tile request does not rebuild the view: the player's tiles stay in memory
// until a write changes what they see, and a write drops them.
func TestRN10_TheViewIsKeptBetweenTileRequests(t *testing.T) {
	t.Parallel()
	c := newCave(t)
	res := c.ana.mustVision(c.campaign, c.mapID) // reads the view and keeps its tiles
	if n := c.h.svc.tiles.views.size(); n == 0 {
		t.Fatal("GetMapVision kept no tiles")
	}
	path := tileURL(res, res.GetTiles()[0])
	first := c.ana.get(path)
	etag := first.header.Get("ETag")
	for range 3 {
		if r := c.ana.get(path, "If-None-Match", etag); r.status != http.StatusNotModified {
			t.Fatalf("status %d, want 304", r.status)
		}
	}
	// A move changes what she sees: the write drops the kept tiles, and the next request sees the new view.
	c.master.placeAt(c.campaign, c.mapID, c.pens.GetId(), grid.Square{Col: 12, Row: 7})
	if n := c.h.svc.tiles.views.size(); n != 0 {
		t.Errorf("%d kept views after a write that changed what the players see", n)
	}
	after := c.ana.mustVision(c.campaign, c.mapID)
	for _, tile := range after.GetTiles() {
		checkTile(t, "Ana after the move", decodeTile(t, c.ana.get(tileURL(after, tile))), after, tile)
	}
}

// A user's cache misses are rate limited: the 429 comes with Retry-After, and a
// tile that is cached costs nothing.
func TestRN10_TileMissesAreRateLimitedPerUser(t *testing.T) {
	t.Parallel()
	c := newCave(t)
	res := c.ana.mustVision(c.campaign, c.mapID)
	c.h.svc.tiles.limiter = ratelimit.New(ratelimit.Config{
		PerClient: ratelimit.Rate{Burst: 1, Every: time.Hour}, Global: ratelimit.Rate{Burst: 100, Every: time.Millisecond}, MaxClients: 8,
	})
	first, second := res.GetTiles()[0], res.GetTiles()[len(res.GetTiles())-1]
	if first.GetTx() == second.GetTx() && first.GetTy() == second.GetTy() {
		t.Skip("one tile")
	}
	if r := c.ana.get(tileURL(res, first)); r.status != http.StatusOK {
		t.Fatalf("the first miss: status %d", r.status)
	}
	if r := c.ana.get(tileURL(res, first)); r.status != http.StatusOK {
		t.Errorf("a cached tile was limited: status %d", r.status)
	}
	r := c.ana.get(tileURL(res, second))
	if r.status != http.StatusTooManyRequests || r.header.Get("Retry-After") == "" {
		t.Errorf("the second miss: status %d, Retry-After %q, want 429 with Retry-After", r.status, r.header.Get("Retry-After"))
	}
	if r := c.caio.get(tileURL(c.caio.mustVision(c.campaign, c.mapID), second)); r.status == http.StatusTooManyRequests {
		t.Error("another user was limited by Ana's misses")
	}
}

// ---- benchmarks ----

func fullKnown(g grid.Grid, tx, ty int) nbhd {
	return newNbhd(g, tx, ty, 1, func(int, int) bool { return true })
}

// BenchmarkTileCold is a tile of a 4.096 x 2.048 image on its first request: the
// decode, the shrink to the working copy and the render.
func BenchmarkTileCold(b *testing.B) {
	data := patternImage(b, 4096, 2048)
	g := grid.Grid{Columns: 64, Rows: 32}
	b.ReportAllocs()
	for b.Loop() {
		wc, sw, sh, err := decodeWorkingCopy(data, g)
		if err != nil {
			b.Fatal(err)
		}
		if _, err := renderTile(&workingCopy{img: wc, srcW: sw, srcH: sh}, g, 0, 0, fullKnown(g, 0, 0), false); err != nil {
			b.Fatal(err)
		}
	}
}

// BenchmarkTileWarm is one tile from the working copy that is already there (a
// fully seen tile: the most pixels to draw and to compress).
func BenchmarkTileWarm(b *testing.B) {
	g := grid.Grid{Columns: 64, Rows: 32}
	wc, sw, sh, err := decodeWorkingCopy(patternImage(b, 4096, 2048), g)
	if err != nil {
		b.Fatal(err)
	}
	copyOf := &workingCopy{img: wc, srcW: sw, srcH: sh}
	nb := fullKnown(g, 1, 1)
	b.ReportAllocs()
	for b.Loop() {
		if _, err := renderTile(copyOf, g, 1, 1, nb, false); err != nil {
			b.Fatal(err)
		}
	}
}

// BenchmarkTileWarmJPEG is the same for a JPEG map (the blocks are checked too).
func BenchmarkTileWarmJPEG(b *testing.B) {
	g := grid.Grid{Columns: 64, Rows: 32}
	img := oracleImage(g, 4096, 2048, func(int, int) bool { return true }, color.NRGBA{})
	wc, sw, sh, err := decodeWorkingCopy(encodeOracle(b, img, true), g)
	if err != nil {
		b.Fatal(err)
	}
	copyOf := &workingCopy{img: wc, srcW: sw, srcH: sh}
	nb := newNbhd(g, 1, 1, 18, func(int, int) bool { return true })
	b.ReportAllocs()
	for b.Loop() {
		if _, err := renderTile(copyOf, g, 1, 1, nb, true); err != nil {
			b.Fatal(err)
		}
	}
}

// TestTileMemory measures what a first tile costs in memory and time, for the
// numbers in CONTRIBUTING ("As medidas dos tiles da névoa"). It runs only with
// MEURPG_MEASURE=1, and without -race (the race detector multiplies both):
//
//	MEURPG_MEASURE=1 go test -run TestTileMemory -v ./internal/maps
func TestTileMemory(t *testing.T) {
	if os.Getenv("MEURPG_MEASURE") == "" {
		t.Skip("set MEURPG_MEASURE=1 to measure")
	}
	for _, tc := range []struct {
		name string
		w, h int
		cols int
	}{{"the cave, 240 x 160", 240, 160, 24}, {"an upload, 4.096 x 2.048", 4096, 2048, 64}, {"the largest upload, 8.192 x 4.096", 8192, 4096, 128}} {
		data := patternImage(t, tc.w, tc.h)
		g := grid.Grid{Columns: tc.cols, Rows: grid.RowsFor(tc.cols, tc.w, tc.h)}
		runtime.GC()
		var base runtime.MemStats
		runtime.ReadMemStats(&base)
		var peak uint64
		stop, done := make(chan struct{}), make(chan struct{})
		go func() {
			defer close(done)
			var m runtime.MemStats
			for {
				select {
				case <-stop:
					return
				case <-time.After(time.Millisecond):
					runtime.ReadMemStats(&m)
					peak = max(peak, m.HeapAlloc)
				}
			}
		}()
		start := time.Now()
		wc, sw, sh, err := decodeWorkingCopy(data, g)
		if err != nil {
			t.Fatal(err)
		}
		decoded := time.Since(start)
		start = time.Now()
		tile, err := renderTile(&workingCopy{img: wc, srcW: sw, srcH: sh}, g, 0, 0, fullKnown(g, 0, 0), false)
		if err != nil {
			t.Fatal(err)
		}
		rendered := time.Since(start)
		close(stop)
		<-done
		runtime.GC()
		var after runtime.MemStats
		runtime.ReadMemStats(&after)
		t.Logf("%s: file %d kB; working copy %d x %d = %.1f MB; first tile: decode+shrink %v, render %v, %d kB PNG; heap peak %.0f MB over the %.0f MB before, %.0f MB kept",
			tc.name, len(data)/1000, wc.Bounds().Dx(), wc.Bounds().Dy(), float64(len(wc.Pix))/1e6, decoded.Round(time.Millisecond), rendered.Round(time.Millisecond), len(tile)/1000,
			float64(peak)/1e6, float64(base.HeapAlloc)/1e6, float64(after.HeapAlloc)/1e6)
		runtime.KeepAlive(wc)
	}
}

// TestTileRequestTiming measures a request for a cached tile and a 304 through
// the whole route (MEURPG_MEASURE=1, without -race).
func TestTileRequestTiming(t *testing.T) {
	if os.Getenv("MEURPG_MEASURE") == "" {
		t.Skip("set MEURPG_MEASURE=1 to measure")
	}
	c := newCave(t)
	res := c.ana.mustVision(c.campaign, c.mapID)
	path := tileURL(res, res.GetTiles()[0])
	first := c.ana.get(path)
	etag := first.header.Get("ETag")
	const n = 200
	start := time.Now()
	for range n {
		c.ana.get(path)
	}
	hit := time.Since(start) / n
	start = time.Now()
	for range n {
		c.ana.get(path, "If-None-Match", etag)
	}
	t.Logf("a cached tile through the route: %v; a 304: %v (the harness's own HTTP round trip included)", hit, time.Since(start)/n)
}

// A grid finer than the image's pixels leaves tiles with no pixel at all: the index does not list them, and every tile it lists is a PNG.
func TestATinyImageWithAFineGridListsOnlyTilesWithPixels(t *testing.T) {
	var jbuf bytes.Buffer
	if err := jpeg.Encode(&jbuf, solid(10, 10, color.NRGBA{R: 200, G: 120, B: 40, A: 255}), nil); err != nil {
		t.Fatal(err)
	}
	var pbuf bytes.Buffer
	if err := png.Encode(&pbuf, solid(10, 10, color.NRGBA{R: 30, G: 90, B: 160, A: 255})); err != nil {
		t.Fatal(err)
	}
	for _, tc := range []struct {
		name, file string
		data       []byte
	}{{"jpeg", "tiny.jpg", jbuf.Bytes()}, {"png", "tiny.png", pbuf.Bytes()}} {
		t.Run(tc.name, func(t *testing.T) {
			h := newHarness(t)
			master, ana := h.newUser("Mestre"), h.newUser("Ana")
			campaign := h.newCampaign(master, ana)
			imageID := master.mustUpload(campaign, tc.file, tc.data).GetId()
			mapID := master.createMap(campaign, "Minúsculo", imageID).GetId()
			master.setMapRevealed(campaign, mapID, true)
			m := master.mustSetGrid(campaign, mapID, 200)
			if _, err := master.maps.SetMapFog(t.Context(), connect.NewRequest(&mapsv1.SetMapFogRequest{CampaignId: campaign, MapId: mapID, FogEnabled: new(true)})); err != nil {
				t.Fatalf("SetMapFog(on) error = %v", err)
			}
			master.start(campaign)
			if _, err := master.setCurrentMap(campaign, mapID); err != nil {
				t.Fatalf("SetCurrentMap() error = %v", err)
			}
			hero := ana.pc(campaign, "Pensantus", "race:gnome")
			g := grid.Grid{Columns: int(m.GetGridColumns()), Rows: int(m.GetGridRows())}
			// The square that holds the image's first pixel (squareOf): the 20 squares of a row of the grid share it.
			x, y := g.CenterOf(grid.Square{Col: 19, Row: 19})
			master.placeToken(campaign, mapID, hero.GetId(), int32(x), int32(y)) //nolint:gosec // G115: at most 10000
			res := ana.mustVision(campaign, mapID)
			if len(res.GetTiles()) == 0 {
				t.Fatal("the player has no tile at all")
			}
			t.Logf("grid %dx%d, %d tiles listed", g.Columns, g.Rows, len(res.GetTiles()))
			for _, tile := range res.GetTiles() {
				req, err := http.NewRequestWithContext(t.Context(), http.MethodGet, h.server.URL+tileURL(res, tile), nil)
				if err != nil {
					t.Fatal(err)
				}
				req.Header.Set(testUserHeader, ana.id)
				resp, err := h.server.Client().Do(req)
				if err != nil {
					t.Errorf("tile (%d,%d): request failed (server panic?): %v", tile.GetTx(), tile.GetTy(), err)
					continue
				}
				body, _ := io.ReadAll(resp.Body)
				_ = resp.Body.Close()
				if resp.StatusCode != http.StatusOK {
					t.Errorf("tile (%d,%d): status %d, body %s; want 200", tile.GetTx(), tile.GetTy(), resp.StatusCode, body)
					continue
				}
				if _, err := png.Decode(bytes.NewReader(body)); err != nil {
					t.Errorf("tile (%d,%d): not a PNG: %v", tile.GetTx(), tile.GetTy(), err)
				}
			}
		})
	}
}
