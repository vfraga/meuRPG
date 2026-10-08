package maps

import (
	"bytes"
	"container/list"
	"context"
	"errors"
	"fmt"
	"image"
	"image/color"
	"image/jpeg"
	"image/png"
	"io"
	"net/http"
	"strconv"
	"strings"
	"sync"
	"time"

	"connectrpc.com/connect"
	"github.com/jackc/pgx/v5"
	xdraw "golang.org/x/image/draw"

	mapsv1 "github.com/PuraFome/meuRPG/backend/gen/meurpg/maps/v1"
	"github.com/PuraFome/meuRPG/backend/internal/authz"
	"github.com/PuraFome/meuRPG/backend/internal/maps/images"
	"github.com/PuraFome/meuRPG/backend/internal/platform/blob"
	"github.com/PuraFome/meuRPG/backend/internal/platform/ratelimit"
	"github.com/PuraFome/meuRPG/backend/internal/platform/slowclient"
	"github.com/PuraFome/meuRPG/backend/internal/rules/grid"
)

// The image of a fog map, as tiles per player (MR-036, RN-10, Etapa 9, D6, ADR-0016).
//
// A player on a map with the fog on never receives the image: the server cuts it
// into tiles of 16 x 16 squares and draws into each one only the squares that
// player sees or remembers. Every other pixel of the tile is solid black and
// opaque (the "Não visto" of the design), and the PNG is written from the pixels
// alone (no metadata). A tile with no known square does not exist. The browser
// draws the per-square shading (seen now, grey, remembered) from GetMapVision, so
// moving re-renders nothing until a new square is seen.
//
// What is guaranteed is this, and tiles_test.go proves it (TestRN10_TileOracle):
// two images that differ only inside squares a player does not know give
// byte-identical tiles for that player. To get there:
//   - The working copy is shrunk square by square, so a pixel of a square is
//     built from that square's own source pixels, never from a neighbour's.
//   - A PNG tile also blacks out one pixel on the known side of every edge with
//     an unknown square (a belt: the shrink already keeps squares apart).
//   - A JPEG tile blacks out every 16 x 16 block (an MCU of the 4:2:0 re-encode:
//     the unit that shares chroma and whose 8 x 8 DCT blocks spread ringing) that
//     touches an unknown square. So on a JPEG map the fog's edge is darker by up
//     to one block (16 source pixels), and a map with very small squares loses
//     more of each known square.
//
// The server has 512 MiB and one CPU, so the work is bounded (numbers in
// CONTRIBUTING, "As medidas dos tiles da névoa"):
//   - one working copy of the image per map and grid, decoded once and capped at
//     workingMaxSide pixels on the long side, in an LRU of workingCopies maps;
//   - one render at a time (tileRenderer.gate): a request waits renderWait, then
//     gets 503 with Retry-After; a user's cache misses are also rate limited;
//   - a cache of the finished tiles, bounded by its bytes. Its key is the image,
//     the grid, the tile and the known squares around it, never a user: two
//     players with the same view of a tile share it;
//   - each player's tiles (which exist, and their revisions) kept in memory until
//     what the player sees changes, so a tile request does not rebuild the view.

const (
	// tileSquares is the side of a tile, in squares: 16 x 16, 256 squares.
	tileSquares = 16
	// workingMaxSide caps the working copy's long side in pixels: 2.048 x 1.365
	// is about 11 MB of RGBA, a 4.096 x 2.048 image is shrunk by half.
	workingMaxSide = 2048
	// workingCopies is how many maps keep a working copy: the one in play and the
	// one just left. It is a limit of the whole server: a third map in play at the
	// same time makes the copies take turns, and each turn decodes again.
	workingCopies = 2
	// tileCacheBytes bounds the finished tiles kept in memory (the PNG bytes).
	tileCacheBytes = 32 << 20
	// decodeLimit is the most memory the decode of a stored image may need (the
	// decoded pixels, by their color model); a larger one (a 16-bit PNG of more
	// than 24 megapixels, kept from before uploads were stored as 8 bits) is
	// refused with a 503 instead of risking the server.
	decodeLimit = 192 << 20
	// renderWait is how long a request waits for its turn before the 503.
	renderWait = 8 * time.Second
	// retryAfterSeconds is the Retry-After of that 503.
	retryAfterSeconds = 2
	// mcu is the side, in source pixels, of a JPEG block that shares chroma
	// (4:2:0), and jpegReach how far a block's content reaches into its
	// neighbors when padded by the shrink's one pixel.
	mcu       = 16
	jpegReach = mcu + 2
	// viewsTTL is how long a player's tiles are kept without a write telling
	// the server to drop them (a belt: every write that changes what a player
	// sees drops them).
	viewsTTL = 30 * time.Second
	// maxViews bounds the kept views: a table has a handful of players.
	maxViews = 256
)

// TilesPath is the base of the tile route: GET TilesPath+{map}/tiles/{tx}/{ty}.
// It lives under /images so the static server and the dev proxy already treat it as the API.
const TilesPath = "/images/maps/"

// tileSource is what the tiles of one map's image depend on, besides the player.
type tileSource struct {
	mapID, campaignID, imageID string
	g                          grid.Grid
	width, height              int  // the stored image's pixels
	jpeg                       bool // it is a JPEG (the fog's edge is a block wide)
}

func tileSourceOf(mapID, campaignID, imageID, contentType string, columns *int32, factor, width, height int32) tileSource {
	return tileSource{
		mapID: mapID, campaignID: campaignID, imageID: imageID, g: gridOf(columns, factor, width, height),
		width: int(width), height: int(height), jpeg: contentType == "image/jpeg",
	}
}

// ring is how many squares around a tile its pixels depend on: one for a PNG (the
// belt), and for a JPEG as many as it takes to cover the blocks that touch it.
func (t tileSource) ring() int {
	if !t.jpeg {
		return 1
	}
	side := max(1, min(t.width/max(1, t.g.Columns), t.height/max(1, t.g.Rows)))
	return min((jpegReach+side-1)/side, jpegReach)
}

// tileGrid is how many tiles wide and high a grid is.
func tileGrid(g grid.Grid) (cols, rows int) {
	return (g.Columns + tileSquares - 1) / tileSquares, (g.Rows + tileSquares - 1) / tileSquares
}

// nbhd says which squares of a window around a tile the viewer knows: the tile's
// 16 x 16 and a ring of squares around it. A square outside the map is known (there
// is nothing there to leak), one outside the window is not.
type nbhd struct {
	cols, rows int
	c0, r0     int // the window's first square
	side       int // squares on each side of the window
	bits       string
}

func (n nbhd) known(c, r int) bool {
	if c < 0 || r < 0 || c >= n.cols || r >= n.rows {
		return true
	}
	dc, dr := c-n.c0, r-n.r0
	if dc < 0 || dr < 0 || dc >= n.side || dr >= n.side {
		return false
	}
	i := dr*n.side + dc
	return n.bits[i/8]&(1<<(i%8)) != 0
}

func newNbhd(g grid.Grid, tx, ty, ring int, known func(c, r int) bool) nbhd {
	n := nbhd{cols: g.Columns, rows: g.Rows, c0: tx*tileSquares - ring, r0: ty*tileSquares - ring, side: tileSquares + 2*ring}
	bits := make([]byte, (n.side*n.side+7)/8)
	for dr := range n.side {
		for dc := range n.side {
			if known(n.c0+dc, n.r0+dr) {
				i := dr*n.side + dc
				bits[i/8] |= 1 << (i % 8)
			}
		}
	}
	n.bits = string(bits)
	return n
}

// tileInfo is one tile a viewer has: the squares around it that they know, and its revision.
type tileInfo struct {
	tx, ty   int
	nb       nbhd
	revision int32
}

// viewTiles is the tiles a viewer has on a map, for one image and grid.
type viewTiles struct {
	imageID string
	g       grid.Grid
	tiles   []tileInfo
	byXY    map[[2]int]int
	at      time.Time
}

func (v *viewTiles) find(tx, ty int) (tileInfo, bool) {
	i, ok := v.byXY[[2]int{tx, ty}]
	if !ok {
		return tileInfo{}, false
	}
	return v.tiles[i], true
}

// tileRevision is the number the app compares (the `r` of the URL and the ETag):
// the known squares around the tile, the image and the grid, so a new image or
// grid is a new tile, and a neighbor that became known (a JPEG block it cleared)
// is too.
func tileRevision(imageID string, g grid.Grid, tx, ty int, nb nbhd) int32 {
	return hashOf([]byte(imageID), []byte(strconv.Itoa(g.Columns)+"x"+strconv.Itoa(g.Rows)+":"+strconv.Itoa(tx)+","+strconv.Itoa(ty)), []byte(nb.bits))
}

// buildTiles lists the tiles that exist for the view: those with a known square.
func buildTiles(p *playerView, src tileSource) *viewTiles {
	cols, rows := tileGrid(p.g)
	ring := src.ring()
	out := &viewTiles{imageID: src.imageID, g: p.g, byXY: map[[2]int]int{}, at: time.Now()}
	known := func(c, r int) bool { return p.known(grid.Square{Col: c, Row: r}) }
	W, H := workingSize(max(1, src.width), max(1, src.height))
	for ty := range rows {
		for tx := range cols {
			if x0, x1, y0, y1 := tilePixels(p.g, W, H, tx, ty); x0 == x1 || y0 == y1 {
				continue // no pixel to draw: nothing to list
			}
			seen := false
			for dr := 0; dr < tileSquares && !seen; dr++ {
				for dc := 0; dc < tileSquares && !seen; dc++ {
					seen = known(tx*tileSquares+dc, ty*tileSquares+dr)
				}
			}
			if !seen {
				continue
			}
			nb := newNbhd(p.g, tx, ty, ring, known)
			out.byXY[[2]int{tx, ty}] = len(out.tiles)
			out.tiles = append(out.tiles, tileInfo{tx: tx, ty: ty, nb: nb, revision: tileRevision(src.imageID, p.g, tx, ty, nb)})
		}
	}
	return out
}

// proto is the index GetMapVision sends.
func (v *viewTiles) proto() []*mapsv1.MapTile {
	out := make([]*mapsv1.MapTile, 0, len(v.tiles))
	for _, t := range v.tiles {
		out = append(out, &mapsv1.MapTile{Tx: int32(t.tx), Ty: int32(t.ty), Revision: t.revision}) //nolint:gosec // G115: at most 13 x 25 tiles
	}
	return out
}

// ---- the working copy ----

// workingCopy is a map's image decoded once and shrunk (per square) to at most
// workingMaxSide.
type workingCopy struct {
	mapID, imageID string
	cols, rows     int
	img            *image.RGBA
	srcW, srcH     int // the stored image's size
}

// workingSet is the LRU of working copies.
type workingSet struct {
	mu    sync.Mutex
	items []*workingCopy // most recently used first
}

func (w *workingSet) get(mapID, imageID string, g grid.Grid) *workingCopy {
	w.mu.Lock()
	defer w.mu.Unlock()
	for i, c := range w.items {
		if c.mapID == mapID && c.imageID == imageID && c.cols == g.Columns && c.rows == g.Rows {
			copy(w.items[1:i+1], w.items[:i])
			w.items[0] = c
			return c
		}
	}
	return nil
}

// put adds a copy (replacing the same map's older one) and drops the least
// recently used beyond workingCopies.
func (w *workingSet) put(c *workingCopy) {
	w.mu.Lock()
	defer w.mu.Unlock()
	kept := []*workingCopy{c}
	for _, o := range w.items {
		if o.mapID != c.mapID && len(kept) < workingCopies {
			kept = append(kept, o)
		}
	}
	w.items = kept
}

func (w *workingSet) forget(mapID string) {
	w.mu.Lock()
	defer w.mu.Unlock()
	kept := w.items[:0]
	for _, c := range w.items {
		if c.mapID != mapID {
			kept = append(kept, c)
		}
	}
	w.items = kept
}

func (w *workingSet) maps() []string {
	w.mu.Lock()
	defer w.mu.Unlock()
	out := make([]string, 0, len(w.items))
	for _, c := range w.items {
		out = append(out, c.mapID)
	}
	return out
}

// pixelAt is the first pixel of square i (a column or a row) when n squares
// span size pixels: the squares share the pixels evenly, so the tile and the
// shading the browser draws agree to the pixel.
func pixelAt(i, n, size int) int { return i * size / n }

// squareOf is the square (of n across size pixels) that holds pixel px: the
// inverse of pixelAt.
func squareOf(px, n, size int) int {
	return min(max(((px+1)*n+size-1)/size-1, 0), n-1)
}

// workingSize is the size of the working copy of a sw x sh image: the image's, or
// shrunk so its long side is at most workingMaxSide.
func workingSize(sw, sh int) (w, h int) {
	if long := max(sw, sh); long > workingMaxSide {
		return max(1, sw*workingMaxSide/long), max(1, sh*workingMaxSide/long)
	}
	return sw, sh
}

// tilePixels is the pixel rectangle of tile (tx, ty) in a width x height working copy. A grid
// finer than the image's pixels leaves some tiles with none.
func tilePixels(g grid.Grid, width, height, tx, ty int) (x0, x1, y0, y1 int) {
	c0, c1 := tx*tileSquares, min((tx+1)*tileSquares, g.Columns)
	r0, r1 := ty*tileSquares, min((ty+1)*tileSquares, g.Rows)
	return pixelAt(c0, g.Columns, width), pixelAt(c1, g.Columns, width), pixelAt(r0, g.Rows, height), pixelAt(r1, g.Rows, height)
}

// errTooBig is the refusal of a stored image whose decode would need more than decodeLimit.
var errTooBig = errors.New("the stored image needs too much memory to decode")

// decodeWorkingCopy decodes the stored image (a PNG or a JPEG, by their names, never
// whatever decoder is registered) and shrinks it so its long side is at most
// workingMaxSide, one square at a time for the grid g: a square of the copy is
// built from that square's source pixels alone. The stored image is already
// clean (re-encoded from its pixels), and the working copy is only pixels.
func decodeWorkingCopy(data []byte, g grid.Grid) (*image.RGBA, int, int, error) {
	var (
		cfg    image.Config
		err    error
		isJPEG = bytes.HasPrefix(data, []byte("\xff\xd8\xff"))
	)
	switch {
	case isJPEG:
		cfg, err = jpeg.DecodeConfig(bytes.NewReader(data))
	case bytes.HasPrefix(data, []byte("\x89PNG\r\n\x1a\n")):
		cfg, err = png.DecodeConfig(bytes.NewReader(data))
	default:
		err = errors.New("not a PNG or a JPEG")
	}
	if err != nil {
		return nil, 0, 0, fmt.Errorf("read the map's image header: %w", err)
	}
	if cfg.Width < 1 || cfg.Height < 1 {
		return nil, 0, 0, errors.New("the map's image is empty")
	}
	per := 4.0 // NRGBA, RGBA, 8-bit
	switch {
	case isJPEG:
		per = 1.5
	case cfg.ColorModel == color.RGBA64Model || cfg.ColorModel == color.NRGBA64Model:
		per = 8
	case cfg.ColorModel == color.Gray16Model:
		per = 2
		if images.HasTransparencyChunk(data) {
			per = 8 // png.Decode returns NRGBA64 for it
		}
	}
	if len(data) > 28 && !isJPEG && data[28] == 1 {
		per *= 2 // interlaced: each pass has its own image before they merge
	}
	if int64(float64(cfg.Width)*float64(cfg.Height)*per) > decodeLimit {
		return nil, 0, 0, errTooBig
	}
	var src image.Image
	if isJPEG {
		src, err = jpeg.Decode(bytes.NewReader(data))
	} else {
		src, err = png.Decode(bytes.NewReader(data))
	}
	if err != nil {
		return nil, 0, 0, fmt.Errorf("decode the map's image: %w", err)
	}
	b := src.Bounds()
	sw, sh := b.Dx(), b.Dy()
	w, h := workingSize(sw, sh)
	dst := image.NewRGBA(image.Rect(0, 0, w, h))
	if w == sw && h == sh {
		xdraw.Draw(dst, dst.Bounds(), src, b.Min, xdraw.Src)
		return dst, sw, sh, nil
	}
	if g.Columns < 1 || g.Rows < 1 {
		return nil, 0, 0, errors.New("the map has no grid")
	}
	// Square by square: Scale reads only inside the source rectangle it is given,
	// so a pixel of one square never takes a neighbour's color.
	for r := range g.Rows {
		dy0, dy1 := pixelAt(r, g.Rows, h), pixelAt(r+1, g.Rows, h)
		sy0, sy1 := pixelAt(r, g.Rows, sh), pixelAt(r+1, g.Rows, sh)
		for c := range g.Columns {
			dx0, dx1 := pixelAt(c, g.Columns, w), pixelAt(c+1, g.Columns, w)
			sx0, sx1 := pixelAt(c, g.Columns, sw), pixelAt(c+1, g.Columns, sw)
			if dx0 == dx1 || dy0 == dy1 || sx0 == sx1 || sy0 == sy1 {
				continue
			}
			xdraw.ApproxBiLinear.Scale(dst, image.Rect(dx0, dy0, dx1, dy1), src,
				image.Rect(b.Min.X+sx0, b.Min.Y+sy0, b.Min.X+sx1, b.Min.Y+sy1), xdraw.Src, nil)
		}
	}
	return dst, sw, sh, nil
}

// ---- the tile cache ----

type tileKey struct {
	imageID string
	cols    int
	rows    int
	tx, ty  int
	nb      string // the known squares around the tile
}

type tileEntry struct {
	key    tileKey
	mapID  string
	png    []byte
	nbytes int
}

// tileCache is an LRU of finished tiles bounded by their bytes.
type tileCache struct {
	mu      sync.Mutex
	max     int
	bytes   int
	order   *list.List // front is the most recently used
	entries map[tileKey]*list.Element
}

func (c *tileCache) get(k tileKey) []byte {
	c.mu.Lock()
	defer c.mu.Unlock()
	if el, ok := c.entries[k]; ok {
		c.order.MoveToFront(el)
		return el.Value.(*tileEntry).png
	}
	return nil
}

func (c *tileCache) put(mapID string, k tileKey, png []byte) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.entries == nil {
		c.entries, c.order = map[tileKey]*list.Element{}, list.New()
	}
	if _, ok := c.entries[k]; ok || len(png) > c.max {
		return
	}
	c.entries[k] = c.order.PushFront(&tileEntry{key: k, mapID: mapID, png: png, nbytes: len(png)})
	c.bytes += len(png)
	for c.bytes > c.max {
		c.drop(c.order.Back())
	}
}

func (c *tileCache) drop(el *list.Element) {
	e := c.order.Remove(el).(*tileEntry)
	delete(c.entries, e.key)
	c.bytes -= e.nbytes
}

func (c *tileCache) forget(mapID string) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.order == nil {
		return
	}
	for el := c.order.Front(); el != nil; {
		next := el.Next()
		if el.Value.(*tileEntry).mapID == mapID {
			c.drop(el)
		}
		el = next
	}
}

func (c *tileCache) size() (entries, bytes int) {
	c.mu.Lock()
	defer c.mu.Unlock()
	return len(c.entries), c.bytes
}

// ---- the viewers' tiles ----

type viewKey struct{ mapID, viewer string }

// viewSet keeps each viewer's tiles per map, so a tile request (even a cache hit
// or a 304) does not rebuild the player's view: a dozen reads and the rules'
// derivation of every party member. A write that changes what anyone sees drops a
// map's views (refreshVision), and so do the grid, the image, the memory's
// clearing and a deleted map. gen makes a view that was being built while a write
// dropped them not be kept.
type viewSet struct {
	mu      sync.Mutex
	gen     map[string]uint64
	entries map[viewKey]*viewTiles
}

func (v *viewSet) generation(mapID string) uint64 {
	v.mu.Lock()
	defer v.mu.Unlock()
	return v.gen[mapID]
}

func (v *viewSet) get(mapID, viewer string, src tileSource) *viewTiles {
	v.mu.Lock()
	defer v.mu.Unlock()
	t := v.entries[viewKey{mapID, viewer}]
	if t == nil || t.imageID != src.imageID || t.g != src.g || time.Since(t.at) > viewsTTL {
		return nil
	}
	return t
}

func (v *viewSet) put(mapID, viewer string, gen uint64, t *viewTiles) {
	v.mu.Lock()
	defer v.mu.Unlock()
	if v.gen[mapID] != gen {
		return
	}
	if v.entries == nil || len(v.entries) >= maxViews {
		v.entries = map[viewKey]*viewTiles{}
	}
	v.entries[viewKey{mapID, viewer}] = t
}

func (v *viewSet) drop(mapID string) {
	v.mu.Lock()
	defer v.mu.Unlock()
	if v.gen == nil {
		v.gen = map[string]uint64{}
	}
	v.gen[mapID]++
	for k := range v.entries {
		if k.mapID == mapID {
			delete(v.entries, k)
		}
	}
}

func (v *viewSet) size() int {
	v.mu.Lock()
	defer v.mu.Unlock()
	return len(v.entries)
}

// tileRenderer is the render budget: the working copies, the finished tiles, the
// viewers' tiles and the one-at-a-time gate.
type tileRenderer struct {
	working workingSet
	cache   tileCache
	views   viewSet
	gate    chan struct{}
	wait    time.Duration
	limiter *ratelimit.Limiter
	decodes int // working copies decoded so far (tests read it)
	// onRender, when set, runs inside the gate before each render (tests watch the gate with it).
	onRender func()
}

func newTileRenderer() *tileRenderer {
	r := &tileRenderer{gate: make(chan struct{}, 1), wait: renderWait}
	r.cache.max = tileCacheBytes
	r.cache.order = list.New()
	r.cache.entries = map[tileKey]*list.Element{}
	// A user's cache misses: a table opening a map asks for a few tiles at a time,
	// so 60 at once and 10 a second is far above a person and below a script.
	r.limiter = ratelimit.New(ratelimit.Config{
		PerClient:  ratelimit.Rate{Burst: 60, Every: 100 * time.Millisecond},
		Global:     ratelimit.Rate{Burst: 300, Every: 20 * time.Millisecond},
		MaxClients: 1024,
	})
	return r
}

// forget drops everything of a map: its working copy, its tiles and its viewers'
// tiles (a new image, a new grid, a cleared memory, a deleted map).
func (r *tileRenderer) forget(mapID string) {
	r.working.forget(mapID)
	r.cache.forget(mapID)
	r.views.drop(mapID)
}

// errBusy is the 503 for a request that waited renderWait for its turn.
func errBusy() *httpError {
	return &httpError{status: http.StatusServiceUnavailable, code: connect.CodeUnavailable, reason: "BUSY", message: "the server is rendering other tiles, try again", retryAfter: retryAfterSeconds}
}

// errTooManyTiles is the 429 for a user who asks for too many tiles that are not cached.
func errTooManyTiles(after time.Duration) *httpError {
	return &httpError{status: http.StatusTooManyRequests, code: connect.CodeResourceExhausted, reason: "RATE_LIMITED", message: "too many tiles asked for, try again", retryAfter: max(1, int((after+time.Second-1)/time.Second))}
}

// tile returns the PNG of the tile for a viewer: from the cache, or rendered
// now, one at a time.
func (s *Service) tile(ctx context.Context, userID string, src tileSource, t tileInfo) ([]byte, error) {
	r := s.tiles
	key := tileKey{imageID: src.imageID, cols: src.g.Columns, rows: src.g.Rows, tx: t.tx, ty: t.ty, nb: t.nb.bits}
	if png := r.cache.get(key); png != nil {
		return png, nil
	}
	if ok, after := r.limiter.Allow(userID); !ok {
		return nil, errTooManyTiles(after)
	}
	timer := time.NewTimer(r.wait)
	defer timer.Stop()
	select {
	case r.gate <- struct{}{}:
	case <-timer.C:
		return nil, errBusy()
	case <-ctx.Done():
		return nil, ctx.Err()
	}
	defer func() { <-r.gate }()
	// Another request may have rendered it while this one waited.
	if png := r.cache.get(key); png != nil {
		return png, nil
	}
	if r.onRender != nil {
		r.onRender()
	}
	wc := r.working.get(src.mapID, src.imageID, src.g)
	if wc == nil {
		var err error
		if wc, err = s.loadWorkingCopy(ctx, src); err != nil {
			return nil, err
		}
		r.working.put(wc)
		r.decodes++
	}
	png, err := renderTile(wc, src.g, t.tx, t.ty, t.nb, src.jpeg)
	if err != nil {
		return nil, err
	}
	r.cache.put(src.mapID, key, png)
	return png, nil
}

// loadWorkingCopy reads the stored image and decodes it. The decode is the
// heaviest thing the server does, so it also takes the upload's slot: an upload
// and a first tile never decode at the same time.
func (s *Service) loadWorkingCopy(ctx context.Context, src tileSource) (*workingCopy, error) {
	select {
	case s.processing <- struct{}{}:
	case <-ctx.Done():
		return nil, ctx.Err()
	}
	defer func() { <-s.processing }()
	imageKey, _ := blobKeys(src.campaignID, src.imageID)
	obj, err := s.blobs.Open(ctx, imageKey)
	if err != nil {
		if errors.Is(err, blob.ErrNotFound) {
			s.logger.ErrorContext(ctx, "maps: an image's file is missing")
			return nil, errMapNotFound()
		}
		return nil, errStorage()
	}
	defer func() { _ = obj.Close() }()
	data, err := io.ReadAll(io.LimitReader(obj.Content, 11<<20))
	if err != nil {
		return nil, errStorage()
	}
	img, sw, sh, err := decodeWorkingCopy(data, src.g)
	if err != nil {
		s.logger.ErrorContext(ctx, "maps: cannot decode an image for its tiles", "error", err)
		return nil, errStorage()
	}
	return &workingCopy{mapID: src.mapID, imageID: src.imageID, cols: src.g.Columns, rows: src.g.Rows, img: img, srcW: sw, srcH: sh}, nil
}

// renderTile draws tile (tx, ty): opaque black, then the working copy's pixels of
// every square the viewer knows, except those that could carry something of a
// square they do not (the edges described at the top of the file). Nothing else of
// the working copy is read.
func renderTile(wc *workingCopy, g grid.Grid, tx, ty int, nb nbhd, jpegImage bool) ([]byte, error) {
	src := wc.img
	W, H := src.Bounds().Dx(), src.Bounds().Dy()
	c0, c1 := tx*tileSquares, min((tx+1)*tileSquares, g.Columns)
	r0, r1 := ty*tileSquares, min((ty+1)*tileSquares, g.Rows)
	if c0 >= c1 || r0 >= r1 {
		return nil, errors.New("the tile is outside the grid")
	}
	x0, x1, y0, y1 := tilePixels(g, W, H, tx, ty)
	w, h := x1-x0, y1-y0
	if w == 0 || h == 0 {
		return nil, errMapNotFound() // the index never lists it
	}

	// Which pixel may be shown. First: its square is known, and so are the squares
	// one pixel around it (the belt; for a JPEG the blocks below take over).
	colSq := make([]int, w+2) // the square of pixel x0-1+i
	rowSq := make([]int, h+2)
	for i := range colSq {
		colSq[i] = squareOf(min(max(x0-1+i, 0), W-1), g.Columns, W)
	}
	for i := range rowSq {
		rowSq[i] = squareOf(min(max(y0-1+i, 0), H-1), g.Rows, H)
	}
	knownPx := make([]bool, (w+2)*(h+2))
	for j := range h + 2 {
		for i := range w + 2 {
			knownPx[j*(w+2)+i] = nb.known(colSq[i], rowSq[j])
		}
	}
	show := make([]bool, w*h)
	for j := range h {
		for i := range w {
			ok := true
			for dj := 0; dj < 3 && ok; dj++ {
				for di := 0; di < 3 && ok; di++ {
					ok = knownPx[(j+dj)*(w+2)+i+di]
				}
			}
			show[j*w+i] = ok
		}
	}

	if jpegImage {
		blackenBlocks(show, wc, g, nb, x0, y0, w, h)
	}

	dst := image.NewRGBA(image.Rect(0, 0, w, h))
	for i := 3; i < len(dst.Pix); i += 4 {
		dst.Pix[i] = 0xff
	}
	for j := range h {
		for i := range w {
			if show[j*w+i] {
				copy(dst.Pix[j*dst.Stride+i*4:j*dst.Stride+i*4+4], src.Pix[(y0+j)*src.Stride+(x0+i)*4:])
			}
		}
	}
	var buf bytes.Buffer
	enc := png.Encoder{CompressionLevel: png.BestSpeed}
	if err := enc.Encode(&buf, dst); err != nil {
		return nil, fmt.Errorf("encode a tile: %w", err)
	}
	// The cache counts what it keeps: a copy of exactly the bytes, not the buffer's spare room.
	return bytes.Clone(buf.Bytes()), nil
}

// blackenBlocks clears, in show, every pixel whose source footprint (widened by
// the shrink's pixel) lies in a 16 x 16 source block that touches a square the
// viewer does not know.
func blackenBlocks(show []bool, wc *workingCopy, g grid.Grid, nb nbhd, x0, y0, w, h int) {
	W, H := wc.img.Bounds().Dx(), wc.img.Bounds().Dy()
	sw, sh := wc.srcW, wc.srcH
	// The blocks of the tile's source footprint, and whether each touches an unknown square.
	span := func(first, count, size, ssize int) (lo, hi []int) {
		lo, hi = make([]int, count), make([]int, count)
		for i := range count {
			a := (first + i) * ssize / size
			b := ((first+i+1)*ssize+size-1)/size - 1
			lo[i], hi[i] = max(a-1, 0)/mcu, min(max(b, a)+1, ssize-1)/mcu
		}
		return lo, hi
	}
	xlo, xhi := span(x0, w, W, sw)
	ylo, yhi := span(y0, h, H, sh)
	bx0, bx1, by0, by1 := xlo[0], xhi[w-1], ylo[0], yhi[h-1]
	bad := make([]bool, (bx1-bx0+1)*(by1-by0+1))
	for by := by0; by <= by1; by++ {
		for bx := bx0; bx <= bx1; bx++ {
			ca, cb := squareOf(bx*mcu, g.Columns, sw), squareOf(min((bx+1)*mcu-1, sw-1), g.Columns, sw)
			ra, rb := squareOf(by*mcu, g.Rows, sh), squareOf(min((by+1)*mcu-1, sh-1), g.Rows, sh)
			isBad := false
			for r := ra; r <= rb && !isBad; r++ {
				for c := ca; c <= cb && !isBad; c++ {
					isBad = !nb.known(c, r)
				}
			}
			bad[(by-by0)*(bx1-bx0+1)+bx-bx0] = isBad
		}
	}
	for j := range h {
		for i := range w {
			if !show[j*w+i] {
				continue
			}
			for by := ylo[j]; by <= yhi[j] && show[j*w+i]; by++ {
				for bx := xlo[i]; bx <= xhi[i]; bx++ {
					if bad[(by-by0)*(bx1-bx0+1)+bx-bx0] {
						show[j*w+i] = false
						break
					}
				}
			}
		}
	}
}

// ---- the route ----

// handleTile serves GET /images/maps/{map}/tiles/{tx}/{ty}: the tile of a fog
// map's image for the caller (RN-10).
//   - Only a player (or the master reading `as` a player's character) gets tiles:
//     the master reads the whole image from /images/{id}, as today.
//   - The authorization is the map's: a map the player does not see, a pending
//     member, a non-member, a map without the fog, a tile outside the grid and a
//     tile with no square the viewer knows are all the same 404.
//   - `r` is only a cache key for the browser. The tile is always built from what
//     the viewer knows now, and the ETag says which squares that is.
//   - Cache-Control is `private, no-cache`, as a player's copy of any image: the
//     browser asks again every time, gets a cheap 304 while the tile is the same,
//     and a 404 once the player loses the map. Vary: Cookie keeps two people of one
//     browser from sharing a tile.
//
// Each request checks the session, the membership and whether the player sees the
// map (three reads); the player's tiles come from memory (viewSet) until a write
// changes what they see.
func (s *Service) handleTile(w http.ResponseWriter, r *http.Request) {
	if err := s.serveTile(w, r); err != nil {
		s.writeError(w, r, err)
	}
}

func (s *Service) serveTile(w http.ResponseWriter, r *http.Request) error {
	ctx := r.Context()
	if _, err := authz.RequireSignedIn(ctx); err != nil {
		return err
	}
	mapID, ok := parseID(r.PathValue("map"))
	tx, errX := strconv.Atoi(r.PathValue("tx"))
	ty, errY := strconv.Atoi(r.PathValue("ty"))
	if !ok || errX != nil || errY != nil || tx < 0 || ty < 0 {
		return errMapNotFound()
	}
	info, err := s.queries.GetMapTileInfo(ctx, mapID)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return errMapNotFound()
		}
		return s.dbError(ctx, "find a map", err)
	}
	m, err := authz.RequireCampaignMember(ctx, info.CampaignID)
	if err != nil {
		if connect.CodeOf(err) == connect.CodeUnavailable {
			return err
		}
		return errMapNotFound()
	}
	as := r.URL.Query().Get("as")
	master := m.Role == authz.RoleMaster
	if master != (as != "") { // a player sends no `as`, and the master gets tiles only as a player
		return errMapNotFound()
	}
	src := tileSourceOf(info.ID, info.CampaignID, info.ImageID, info.ImageContentType, info.GridColumns, info.GridFactor, info.ImageWidth, info.ImageHeight)
	if !info.FogEnabled || !src.g.Valid() {
		return errMapNotFound()
	}
	if !master { // the master "Ver como" sees every map; a player, the ones the players see
		current, err := s.currentMap(ctx, info.CampaignID)
		if err != nil {
			return err
		}
		if !playersSee(info.ID, info.RevealedAt, current) {
			return errMapNotFound()
		}
	}
	viewer := "u:" + m.UserID
	if master {
		viewer = "as:" + as
	}
	vt := s.tiles.views.get(mapID, viewer, src)
	if vt == nil {
		gen := s.tiles.views.generation(mapID)
		v, row, pv, err := s.visionOf(ctx, m, mapID, as)
		if err != nil {
			if connect.CodeOf(err) == connect.CodeUnavailable || connect.CodeOf(err) == connect.CodeInternal {
				return err
			}
			return errMapNotFound()
		}
		if !foggedFor(v, row) {
			return errMapNotFound()
		}
		src = tileSourceOf(row.ID, row.CampaignID, row.ImageID, row.ImageContentType, row.GridColumns, row.GridFactor, row.ImageWidth, row.ImageHeight)
		vt = buildTiles(pv, src)
		s.tiles.views.put(mapID, viewer, gen, vt)
	}
	t, ok := vt.find(tx, ty)
	if !ok {
		return errMapNotFound()
	}
	etag := `"t` + strconv.Itoa(int(t.revision)) + `"`
	header := w.Header()
	header.Set("Cache-Control", "private, no-cache")
	header.Set("Vary", "Cookie")
	header.Set("ETag", etag)
	header.Set("X-Content-Type-Options", "nosniff")
	header.Set("Content-Security-Policy", "default-src 'none'")
	header.Set("Cross-Origin-Resource-Policy", "same-origin")
	if etagMatches(r.Header.Values("If-None-Match"), etag) {
		w.WriteHeader(http.StatusNotModified)
		return nil
	}
	if s.blobs == nil {
		return errImagesOff()
	}
	png, err := s.tile(ctx, m.UserID, src, t)
	if err != nil {
		return err
	}
	header.Set("Content-Type", "image/png")
	defer slowclient.WriteBody(w, downloadWriteTimeout)()
	http.ServeContent(w, r, "", time.Time{}, bytes.NewReader(png))
	return nil
}

// etagMatches reports whether an If-None-Match header (RFC 9110, 13.1.2)
// matches etag. The header is a list of entity tags, each optionally weak
// ("W/"), or "*", and may be split over several header lines. For
// If-None-Match the comparison is weak: W/"x" matches "x".
func etagMatches(values []string, etag string) bool {
	for _, value := range values {
		for candidate := range strings.SplitSeq(value, ",") {
			candidate = strings.TrimSpace(candidate)
			if candidate == "*" || strings.TrimPrefix(candidate, "W/") == etag {
				return true
			}
		}
	}
	return false
}
