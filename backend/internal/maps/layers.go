package maps

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"log/slog"
	"slices"
	"sync"
	"time"

	"connectrpc.com/connect"
	"github.com/jackc/pgx/v5"

	mapsv1 "github.com/PuraFome/meuRPG/backend/gen/meurpg/maps/v1"
	"github.com/PuraFome/meuRPG/backend/internal/authz"
	"github.com/PuraFome/meuRPG/backend/internal/maps/mapsdb"
	"github.com/PuraFome/meuRPG/backend/internal/platform/db"
	"github.com/PuraFome/meuRPG/backend/internal/platform/safego"
	"github.com/PuraFome/meuRPG/backend/internal/platform/uid"
	"github.com/PuraFome/meuRPG/backend/internal/rules/grid"
)

// The painted layers of a map and its fog of war settings (MR-034, MR-036).
//
// Five layers sit on a map's grid: difficult terrain and walls (one bit a
// square), cover (two bits: none, half, three-quarters), light (two bits: not
// painted, dark, dim, bright) and doors (four bits: none, open, closed, locked,
// barred, secret; RN-26). Package rules/grid owns the byte layout; this file
// stores the bytes (table map_layers, one row a map, and a layer with nothing
// painted is NULL) and decides who reads what:
//
//   - the master reads all five, as painted;
//   - a player reads the walls, the difficult terrain, the cover and the doors of
//     a map they see, because those are things you can see, but never the light,
//     and the doors as they know them: a locked door reads closed (they learn it
//     is locked by trying) and a secret door is no door but a wall (the walls
//     layer they get has one there, the doors layer has nothing);
//   - while the map has the fog of war on, a player reads only the squares
//     their character sees now or remembers, and the walls next to a seen square
//     (fog.go says which; GetMapLayers answers fog_withheld, "this is a filtered
//     view"), and the master reads them as a player's character does when they
//     ask "Ver como".
//
// What clears them: a new grid (other columns) and a new image, because a layer
// is sized by the grid. clearLayers is the one place that does it, and where the
// players' memory of the map (map_vision_memory) is forgotten too, since a bitmap
// only fits the grid it was made on.

// maxPaintSquares is how many squares one PaintMapCells may carry. Painting is
// dragged, so the app sends small batches; this bounds what one call costs.
const maxPaintSquares = 400

// defaultLayerHintEvery is how often a map's layers may send their `map_changed`
// (Service.layerHintEvery): a drag paints dozens of batches a second, and each
// hint makes every watcher read the layers again.
const defaultLayerHintEvery = time.Second

// errMapBlocked is MapService's `failed_precondition`, with the MapBlocked
// detail that tells the app why.
func errMapBlocked(reason mapsv1.MapBlockedReason, msg string) error {
	err := connect.NewError(connect.CodeFailedPrecondition, errors.New(msg))
	if detail, detailErr := connect.NewErrorDetail(&mapsv1.MapBlocked{Reason: reason}); detailErr == nil {
		err.AddDetail(detail)
	}
	return err
}

func errNoGrid() error {
	return errMapBlocked(mapsv1.MapBlockedReason_MAP_BLOCKED_REASON_NO_GRID, "the map has no grid")
}

// gridOf is the grid the rules use for a map row with its image's size: the
// zero Grid when the map has none. columns is the engine's (grid_columns, the
// drawn ones times the factor, MR-025), and the rows are the drawn rows times the
// factor, so a square of the drawing is always a whole block of rules squares.
func gridOf(columns *int32, factor, width, height int32) grid.Grid {
	if columns == nil {
		return grid.Grid{}
	}
	f := max(int(factor), 1)
	return grid.Grid{Columns: int(*columns), Rows: grid.RowsFor(int(*columns)/f, int(width), int(height)) * f}
}

// layerSet is a map's six layers, decoded.
type layerSet struct {
	terrain *grid.Layer
	walls   *grid.Layer
	cover   *grid.CoverLayer
	light   *grid.LightLayer
	doors   *grid.DoorLayer
	// noPass are the squares nobody walks into, that block no sight.
	noPass *grid.Layer
}

// loadLayers decodes the stored layers for the grid. A layer that is NULL or does
// not fit the grid (bytes of another grid, which clearLayers prevents) reads as
// nothing painted.
func loadLayers(row mapsdb.MapLayer, g grid.Grid) layerSet {
	set := layerSet{terrain: grid.NewLayer(g), walls: grid.NewLayer(g), cover: grid.NewCoverLayer(g), light: grid.NewLightLayer(g), doors: grid.NewDoorLayer(g), noPass: grid.NewLayer(g)}
	if l, err := grid.DecodeLayer(g, row.DifficultTerrain); err == nil {
		set.terrain = l
	}
	if l, err := grid.DecodeLayer(g, row.Walls); err == nil {
		set.walls = l
	}
	if l, err := grid.DecodeCoverLayer(g, row.Cover); err == nil {
		set.cover = l
	}
	if l, err := grid.DecodeLightLayer(g, row.Light); err == nil {
		set.light = l
	}
	if l, err := grid.DecodeDoorLayer(g, row.Doors); err == nil {
		set.doors = l
	}
	if l, err := grid.DecodeLayer(g, row.NoPass); err == nil {
		set.noPass = l
	}
	return set
}

// forPlayers is the layers as a player who has not found out more reads them:
// the doors as they know them, and the secret ones as walls (RN-26). The light
// is theirs never, so it stays out too.
func (set layerSet) forPlayers() layerSet {
	walls, doors := set.doors.ForPlayers(set.walls)
	return layerSet{terrain: set.terrain, walls: walls, cover: set.cover, doors: doors, noPass: set.noPass}
}

// nilIfBlank stores a layer with nothing painted as NULL, so "empty" has one
// shape.
func nilIfBlank(b []byte) []byte {
	for _, x := range b {
		if x != 0 {
			return b
		}
	}
	return nil
}

// PaintMapCells implements mapsv1connect.MapServiceHandler.
func (s *Service) PaintMapCells(
	ctx context.Context,
	req *connect.Request[mapsv1.PaintMapCellsRequest],
) (*connect.Response[mapsv1.PaintMapCellsResponse], error) {
	m, err := authz.RequireCampaignRole(ctx, req.Msg.GetCampaignId(), authz.RoleMaster)
	if err != nil {
		return nil, err
	}
	mapID, ok := uid.Canonical(req.Msg.GetMapId())
	if !ok {
		return nil, errMapNotFound()
	}
	layer, value := req.Msg.GetLayer(), req.Msg.GetValue()
	if err := checkPaint(layer, value); err != nil {
		return nil, err
	}
	squares := req.Msg.GetSquares()
	if err := checkPaintSquares(len(squares)); err != nil {
		return nil, err
	}

	batch := paintBatch{campaignID: m.CampaignID, mapID: mapID, layer: layer, value: value, squares: squares}
	var done paintDone
	err = db.InTx(ctx, s.pool, func(tx pgx.Tx) error {
		var err error
		done, err = s.paintTx(ctx, s.queries.WithTx(tx), batch)
		return err
	})
	if err != nil {
		return nil, s.dbError(ctx, "paint a map's squares", err)
	}
	if done.changed > 0 {
		s.layersChanged(ctx, m.CampaignID, done.mapRow, done.playersToo, layer == mapsv1.MapLayer_MAP_LAYER_LIGHT)
	}
	return connect.NewResponse(&mapsv1.PaintMapCellsResponse{LayersRevision: done.revision, Changed: done.changed}), nil
}

// paintBatch is one PaintMapCells, once it passed the checks that need no read.
type paintBatch struct {
	campaignID string
	mapID      string
	layer      mapsv1.MapLayer
	value      int32
	squares    []*mapsv1.MapSquare
}

// paintDone is what a batch of paint did: the map's row (as it was locked), how many
// squares changed, the revision the master reads after it, and whether what changed is
// something the players read.
type paintDone struct {
	mapRow     mapsdb.Map
	revision   int32
	changed    int32
	playersToo bool
}

// paintTx paints the batch inside the transaction of q. The map's row lock makes
// two batches of paint take turns: each reads the layers the other left.
func (s *Service) paintTx(ctx context.Context, q *mapsdb.Queries, b paintBatch) (paintDone, error) {
	done := paintDone{playersToo: b.layer != mapsv1.MapLayer_MAP_LAYER_LIGHT}
	var err error
	if done.mapRow, err = lockMap(ctx, q, b.campaignID, b.mapID); err != nil {
		return done, err
	}
	if done.mapRow.GridColumns == nil {
		return done, errNoGrid()
	}
	g, err := paintGrid(ctx, q, b)
	if err != nil {
		return done, err
	}
	stored, err := q.GetMapLayers(ctx, b.mapID)
	if err != nil && !errors.Is(err, pgx.ErrNoRows) {
		return done, fmt.Errorf("read the layers: %w", err)
	}
	set := loadLayers(stored, g)
	asPlayersRead := playersRead(set)
	done.changed = paintSquares(set, b, max(int(done.mapRow.GridFactor), 1))
	// Locking or unlocking a door, or painting a secret door where a wall is, changes
	// nothing a player reads (a locked door reads closed, a secret one is a wall):
	// like the light, it stays out of the number they see and out of their hints (RN-10).
	done.playersToo = b.layer != mapsv1.MapLayer_MAP_LAYER_LIGHT &&
		(b.layer != mapsv1.MapLayer_MAP_LAYER_DOORS || !bytes.Equal(asPlayersRead, playersRead(set)))
	done.revision = done.mapRow.LayersRevision + done.mapRow.LightRevision
	if done.changed == 0 {
		return done, nil // painting what is already there changes nothing
	}
	if err := storeLayers(ctx, q, b.mapID, set, s.now()); err != nil {
		return done, err
	}
	err = bumpPaintRevision(ctx, q, b.mapID, &done)
	return done, err
}

// paintGrid reads the map's grid and checks that every square of the batch is on it.
func paintGrid(ctx context.Context, q *mapsdb.Queries, b paintBatch) (grid.Grid, error) {
	size, err := q.GetMapGrid(ctx, mapsdb.GetMapGridParams{CampaignID: b.campaignID, ID: b.mapID})
	if err != nil {
		return grid.Grid{}, fmt.Errorf("read the map's grid: %w", err)
	}
	g := gridOf(size.GridColumns, size.GridFactor, size.ImageWidth, size.ImageHeight)
	for i, sq := range b.squares {
		if !g.Contains(grid.Square{Col: int(sq.GetCol()), Row: int(sq.GetRow())}) {
			return grid.Grid{}, badSpec("squares[%d] is outside the map's grid of %d x %d squares", i, g.Columns, g.Rows)
		}
	}
	return g, nil
}

// paintSquares paints the batch on the layers and says how many squares changed.
// A door is a whole square of the drawing (MR-025): on a calibrated map painting one
// square of it paints its block of factor x factor squares, so locking, unlocking or
// revealing is one action. The other layers are painted square by square.
func paintSquares(set layerSet, b paintBatch, factor int) int32 {
	f := 1
	if b.layer == mapsv1.MapLayer_MAP_LAYER_DOORS {
		f = factor
	}
	var changed int32
	for _, sq := range b.squares {
		bc, br := int(sq.GetCol())/f*f, int(sq.GetRow())/f*f
		for r := br; r < br+f; r++ {
			for c := bc; c < bc+f; c++ {
				if paint(set, b.layer, c, r, b.value) {
					changed++
				}
			}
		}
	}
	return changed
}

// storeLayers writes the six layers of a map, a layer with nothing painted as NULL.
func storeLayers(ctx context.Context, q *mapsdb.Queries, mapID string, set layerSet, now time.Time) error {
	err := q.UpsertMapLayers(ctx, mapsdb.UpsertMapLayersParams{
		MapID: mapID, DifficultTerrain: nilIfBlank(set.terrain.Encode()), Walls: nilIfBlank(set.walls.Encode()),
		Cover: nilIfBlank(set.cover.Encode()), Light: nilIfBlank(set.light.Encode()), Doors: nilIfBlank(set.doors.Encode()), NoPass: nilIfBlank(set.noPass.Encode()), Now: now,
	})
	if err != nil {
		return fmt.Errorf("write the layers: %w", err)
	}
	return nil
}

// bumpPaintRevision raises the counter a batch of paint moves and sets the revision the
// master reads. The light is the master's alone: it has its own counter, so the number
// players see does not tell them when the master paints it. A change the players cannot
// see counts there too.
func bumpPaintRevision(ctx context.Context, q *mapsdb.Queries, mapID string, done *paintDone) error {
	if !done.playersToo {
		light, err := q.BumpMapLightRevision(ctx, mapID)
		if err != nil {
			return fmt.Errorf("bump the light's revision: %w", err)
		}
		done.revision = done.mapRow.LayersRevision + light
		return nil
	}
	layers, err := q.BumpMapLayersRevision(ctx, mapID)
	if err != nil {
		return fmt.Errorf("bump the layers' revision: %w", err)
	}
	done.revision = layers + done.mapRow.LightRevision
	return nil
}

// playersRead is what the layers a player reads would say if the player read them
// whole: the walls and the doors as they know them (locked reads closed, a secret
// door is a wall). Two states with the same bytes look the same to every player.
func playersRead(set layerSet) []byte {
	p := set.forPlayers()
	return slices.Concat(p.walls.Encode(), p.doors.Encode(), p.noPass.Encode())
}

// checkPaintSquares checks the number of squares of one PaintMapCells.
func checkPaintSquares(n int) error {
	if n == 0 || n > maxPaintSquares {
		return badSpec("squares must have 1 to %d squares", maxPaintSquares)
	}
	return nil
}

// checkPaint checks that a value is one the layer holds.
func checkPaint(layer mapsv1.MapLayer, value int32) error {
	var highest int32
	switch layer {
	case mapsv1.MapLayer_MAP_LAYER_DIFFICULT_TERRAIN, mapsv1.MapLayer_MAP_LAYER_WALL, mapsv1.MapLayer_MAP_LAYER_NO_PASS:
		highest = 1
	case mapsv1.MapLayer_MAP_LAYER_COVER:
		highest = int32(grid.CoverThreeQuarters)
	case mapsv1.MapLayer_MAP_LAYER_LIGHT:
		highest = int32(grid.Bright)
	case mapsv1.MapLayer_MAP_LAYER_DOORS:
		highest = int32(grid.DoorSecret)
	default:
		return badSpec("layer is required")
	}
	if value < 0 || value > highest {
		return badSpec("value must be 0 to %d for this layer", highest)
	}
	return nil
}

// paint sets one square of a layer, and says whether its value changed.
func paint(set layerSet, layer mapsv1.MapLayer, col, row int, value int32) bool {
	switch layer {
	case mapsv1.MapLayer_MAP_LAYER_DIFFICULT_TERRAIN:
		return paintBit(set.terrain, col, row, value == 1)
	case mapsv1.MapLayer_MAP_LAYER_WALL:
		return paintBit(set.walls, col, row, value == 1)
	case mapsv1.MapLayer_MAP_LAYER_NO_PASS:
		return paintBit(set.noPass, col, row, value == 1)
	case mapsv1.MapLayer_MAP_LAYER_COVER:
		cover := grid.Cover(value) //nolint:gosec // G115: checkPaint kept it 0 to 2
		was := set.cover.Get(col, row)
		set.cover.Set(col, row, cover)
		return was != cover
	case mapsv1.MapLayer_MAP_LAYER_DOORS:
		door := grid.Door(value) //nolint:gosec // G115: checkPaint kept it 0 to 5
		was := set.doors.Get(col, row)
		set.doors.Set(col, row, door)
		return was != door
	default: // light
		light := grid.Light(value) //nolint:gosec // G115: checkPaint kept it 0 to 3
		was := set.light.Get(col, row)
		set.light.Set(col, row, light)
		return was != light
	}
}

func paintBit(l *grid.Layer, col, row int, on bool) bool {
	was := l.Get(col, row)
	l.Set(col, row, on)
	return was != on
}

// layersChanged tells the watchers that a map's layers changed. playersToo says
// the players read what changed, and seeingChanged that what the fog shows changes
// even if they do not read it (the light). The master
// always hears; the players only when they may read the layers (a map they see,
// no fog) and the layer is one they read (never the light). On a fog map the
// walls and the light decide what the players see: what each one sees, and
// remembers, is worked out again, and `vision_changed` goes to the players it
// changed (refreshVision); terrain and cover are theirs only on squares they
// know, and the hint of the walls and the light covers the squares they see.
func (s *Service) layersChanged(ctx context.Context, campaignID string, mapRow mapsdb.Map, playersToo, seeingChanged bool) {
	if fogged(mapRow) && !playersToo && !seeingChanged {
		s.layerHints.fire(mapRow.ID, s.layerHintEvery, false, func(bool) { s.publishMapChanged(campaignID, mapRow.ID, false) })
		return // only the master's own view of the layers changed
	}
	if fogged(mapRow) {
		s.layerHints.fire(mapRow.ID, s.layerHintEvery, false, func(bool) {
			s.publishMapChanged(campaignID, mapRow.ID, false)
			s.refreshVision(ctx, campaignID, mapRow.ID)
		})
		return
	}
	players := false
	if playersToo && !mapRow.FogEnabled {
		current, err := s.currentMap(ctx, campaignID)
		if err != nil {
			s.logger.ErrorContext(ctx, "maps: read the current map", "error", err)
		}
		players = playersSee(mapRow.ID, mapRow.RevealedAt, current)
	}
	s.layerHints.fire(mapRow.ID, s.layerHintEvery, players, func(players bool) {
		s.publishMapChanged(campaignID, mapRow.ID, players)
	})
}

// hintGate lets a key send at most one hint every interval: the first goes at
// once, and the ones that follow within the interval merge into a single hint
// sent when it ends, so the last change is never left unannounced.
type hintGate struct {
	mu     sync.Mutex
	keys   map[string]*gateState
	logger *slog.Logger // for a panic in a send that runs on the timer's own goroutine
	// now and afterFunc are the clock and the timer; nil means time.Now and
	// time.AfterFunc. A test replaces them to drive the interval by hand.
	now       func() time.Time
	afterFunc func(d time.Duration, f func()) *time.Timer
}

func (g *hintGate) clock() time.Time {
	if g.now != nil {
		return g.now()
	}
	return time.Now()
}

type gateState struct {
	last    time.Time
	timer   *time.Timer
	players bool // a hint is waiting, and whether it is for the players too
}

func (g *hintGate) fire(key string, every time.Duration, players bool, send func(players bool)) {
	g.mu.Lock()
	if g.keys == nil {
		g.keys = map[string]*gateState{}
	}
	st := g.keys[key]
	if st == nil {
		st = &gateState{}
		g.keys[key] = st
		g.forget(key, every) // the table stays small: idle keys go
	}
	if st.timer != nil { // a hint is already waiting: merge into it
		st.players = st.players || players
		g.mu.Unlock()
		return
	}
	if wait := every - g.clock().Sub(st.last); wait > 0 {
		st.players = players
		after := time.AfterFunc
		if g.afterFunc != nil {
			after = g.afterFunc
		}
		st.timer = after(wait, func() {
			defer safego.Recover(g.logger, "map hint")
			g.mu.Lock()
			p := st.players
			st.timer, st.players, st.last = nil, false, g.clock()
			g.mu.Unlock()
			send(p)
		})
		g.mu.Unlock()
		return
	}
	st.last = g.clock()
	g.mu.Unlock()
	send(players)
}

// forget drops the keys that have been idle for a minute, so the table does not
// grow with every map ever painted. The caller holds the lock.
func (g *hintGate) forget(except string, every time.Duration) {
	if len(g.keys) < 256 {
		return
	}
	for k, st := range g.keys {
		if k != except && st.timer == nil && g.clock().Sub(st.last) > max(time.Minute, every) {
			delete(g.keys, k)
		}
	}
}

// clearLayers deletes a map's painted layers inside the transaction and bumps
// their revision when there were any: its grid's columns or its image changed,
// and a layer only makes sense for the grid it was painted on (D2). The players'
// memory of the map goes too: it is a bitmap of that grid.
func clearLayers(ctx context.Context, q *mapsdb.Queries, mapID string) error {
	if err := clearVisionMemory(ctx, q, mapID); err != nil {
		return err
	}
	n, err := q.DeleteMapLayers(ctx, mapID)
	if err != nil {
		return fmt.Errorf("clear the layers: %w", err)
	}
	if n > 0 {
		if _, err := q.BumpMapLayersRevision(ctx, mapID); err != nil {
			return fmt.Errorf("bump the layers' revision: %w", err)
		}
	}
	return nil
}

// clearVisionMemory makes every player forget what they saw of the map: the map's
// vision epoch goes up (what was remembered under an older one reads as empty and
// is never written back) and the old rows go.
func clearVisionMemory(ctx context.Context, q *mapsdb.Queries, mapID string) error {
	if _, err := q.ClearMapVisionMemory(ctx, mapID); err != nil {
		return fmt.Errorf("forget what the players saw: %w", err)
	}
	if _, err := q.DeleteMapVisionMemory(ctx, mapID); err != nil {
		return fmt.Errorf("forget what the players saw: %w", err)
	}
	return nil
}

// refuseWhileCombat refuses with COMBAT_RUNNING when a combat runs on the map:
// changing its grid or image would clear the layers the fight stands on.
func (s *Service) refuseWhileCombat(ctx context.Context, tx pgx.Tx, campaignID, mapID string) error {
	running, err := s.combats.CombatRunsOnMap(ctx, tx, campaignID, mapID)
	if err != nil {
		return fmt.Errorf("read whether a combat runs on the map: %w", err)
	}
	if running {
		return errMapBlocked(mapsv1.MapBlockedReason_MAP_BLOCKED_REASON_COMBAT_RUNNING,
			"a combat is running on this map: it cannot be changed or deleted")
	}
	return nil
}

// GetMapLayers implements mapsv1connect.MapServiceHandler.
func (s *Service) GetMapLayers(
	ctx context.Context,
	req *connect.Request[mapsv1.GetMapLayersRequest],
) (*connect.Response[mapsv1.GetMapLayersResponse], error) {
	m, err := authz.RequireCampaignMember(ctx, req.Msg.GetCampaignId())
	if err != nil {
		return nil, err
	}
	mapID, ok := uid.Canonical(req.Msg.GetMapId())
	if !ok {
		return nil, errMapNotFound()
	}
	v, err := s.readerOf(ctx, m, req.Msg.GetAsCharacterId())
	if err != nil {
		return nil, err
	}
	row, stored, err := s.readLayersSnapshot(ctx, m.CampaignID, mapID, v)
	if err != nil {
		return nil, s.dbError(ctx, "read a map", err)
	}
	g := gridOf(row.GridColumns, row.GridFactor, row.ImageWidth, row.ImageHeight)
	res := &mapsv1.GetMapLayersResponse{GridColumns: int32(g.Columns), GridRows: int32(g.Rows), LayersRevision: row.LayersRevision} //nolint:gosec // G115: a grid is at most 200 x 400
	if v.master {
		res.LayersRevision += row.LightRevision
	}
	if !g.Valid() {
		return connect.NewResponse(res), nil
	}
	set := loadLayers(stored, g)
	if foggedFor(v, row) {
		if err := s.fillFoggedLayers(ctx, res, set, row, v); err != nil {
			return nil, err
		}
		return connect.NewResponse(res), nil
	}
	if v.master {
		res.Light = nilIfBlank(set.light.Encode())
	} else {
		set = set.forPlayers() // a locked door reads closed and a secret one is a wall (RN-26)
	}
	fillLayers(res, set)
	return connect.NewResponse(res), nil
}

// readLayersSnapshot reads the map the viewer asks for and its stored layers. The
// revision in the answer and the layers it counts are one snapshot: read
// apart, a paint between them gives layers newer than their revision, and the app would
// not ask again.
func (s *Service) readLayersSnapshot(ctx context.Context, campaignID, mapID string, v viewer) (mapsdb.ListMapDetailsRow, mapsdb.MapLayer, error) {
	var (
		row    mapsdb.ListMapDetailsRow
		stored mapsdb.MapLayer
	)
	err := db.ReadTx(ctx, s.pool, func(tx pgx.Tx) error {
		q := s.queries.WithTx(tx)
		cm, err := loadMapsWith(ctx, q, campaignID)
		if err != nil {
			return err
		}
		var ok bool
		if row, ok = cm.byID[mapID]; !ok || !v.seesMap(row.ID, row.RevealedAt) {
			return errMapNotFound() // a hidden map is not found to a player (RN-10)
		}
		if !gridOf(row.GridColumns, row.GridFactor, row.ImageWidth, row.ImageHeight).Valid() {
			return nil // no grid, no layers to read
		}
		if stored, err = q.GetMapLayers(ctx, mapID); err != nil && !errors.Is(err, pgx.ErrNoRows) {
			return fmt.Errorf("read a map's layers: %w", err)
		}
		return nil
	})
	return row, stored, err
}

// fillLayers puts the four layers a player reads in the answer; a layer with nothing
// painted is left out.
func fillLayers(res *mapsv1.GetMapLayersResponse, set layerSet) {
	res.DifficultTerrain, res.Wall, res.Cover, res.Doors = nilIfBlank(set.terrain.Encode()), nilIfBlank(set.walls.Encode()), nilIfBlank(set.cover.Encode()), nilIfBlank(set.doors.Encode())
	res.NoPass = nilIfBlank(set.noPass.Encode())
}

// fillFoggedLayers fills the answer of a player on a fog map: only the squares their
// character sees or remembers (RN-10). The map's counter is withheld: it would tell when
// the master paints what they do not see; the revision is a number of what they receive.
func (s *Service) fillFoggedLayers(ctx context.Context, res *mapsv1.GetMapLayersResponse, set layerSet, row mapsdb.ListMapDetailsRow, v viewer) error {
	pv, _, _, err := s.fogViewOf(ctx, row, v)
	if err != nil {
		return s.dbError(ctx, "work out what a player sees", err)
	}
	fillLayers(res, filterLayers(set, pv))
	res.LayersRevision = layersHash([][]byte{res.DifficultTerrain, res.Wall, res.Cover, res.Doors}, res.NoPass)
	res.FogWithheld = true
	return nil
}

// SetMapFog implements mapsv1connect.MapServiceHandler.
func (s *Service) SetMapFog(
	ctx context.Context,
	req *connect.Request[mapsv1.SetMapFogRequest],
) (*connect.Response[mapsv1.SetMapFogResponse], error) {
	m, err := authz.RequireCampaignRole(ctx, req.Msg.GetCampaignId(), authz.RoleMaster)
	if err != nil {
		return nil, err
	}
	mapID, ok := uid.Canonical(req.Msg.GetMapId())
	if !ok {
		return nil, errMapNotFound()
	}
	msg := req.Msg
	baseLight, err := fogBaseLight(msg)
	if err != nil {
		return nil, err
	}
	current, err := s.currentMap(ctx, m.CampaignID)
	if err != nil {
		return nil, err
	}
	imageCopy, err := s.fogImageCopy(ctx, m.CampaignID, mapID, msg)
	if err != nil {
		return nil, err
	}
	before, after, copied, err := s.writeFog(ctx, m, mapID, msg, baseLight, imageCopy)
	s.discardFogCopy(ctx, m.CampaignID, imageCopy, err != nil || !copied)
	if err != nil {
		return nil, s.dbError(ctx, "set a map's fog", err)
	}
	// The players hear of what they read (the switch, "Visão do grupo"), not of the
	// base light, which is the master's.
	playersHear := before.FogEnabled != after.FogEnabled || before.GroupVision != after.GroupVision
	s.publishMapChanged(m.CampaignID, mapID, playersHear && playersSee(mapID, after.RevealedAt, current))
	// What everyone sees, and remembers, is worked out again (the base light is the
	// master's, but it moves what the squares look like).
	if after.FogEnabled {
		s.refreshVision(ctx, m.CampaignID, mapID)
	}
	out, err := s.masterMap(ctx, m, mapID)
	if err != nil {
		return nil, err
	}
	return connect.NewResponse(&mapsv1.SetMapFogResponse{Map: out}), nil
}

// fogBaseLight checks that a SetMapFog changes something and that its base light, if
// any, is one the database holds; it returns the database's word for it (nil for none).
func fogBaseLight(msg *mapsv1.SetMapFogRequest) (*string, error) {
	if msg.FogEnabled == nil && msg.BaseLight == nil && msg.GroupVision == nil {
		return nil, badSpec("nothing to change")
	}
	if msg.BaseLight == nil {
		return nil, nil
	}
	word, ok := baseLightToDB[msg.GetBaseLight()]
	if !ok {
		return nil, badSpec("base_light must be DARK, DIM or BRIGHT")
	}
	return &word, nil
}

// fogImageCopy prepares the copy of the map's image that turning the fog on needs: it
// keeps the raw image from the players (RN-10), so an image that is also used another
// way is copied first, and the map gets the copy. It is nil when the call does not turn
// the fog on, or the map already has it.
func (s *Service) fogImageCopy(ctx context.Context, campaignID, mapID string, msg *mapsv1.SetMapFogRequest) (*fogCopy, error) {
	if !msg.GetFogEnabled() {
		return nil, nil
	}
	imageCopy, err := s.fogCopyOf(ctx, campaignID, mapID, func(m mapsdb.Map) bool { return !m.FogEnabled })
	if err != nil {
		return nil, s.dbError(ctx, "copy the map's image for the fog", err)
	}
	return imageCopy, nil
}

// writeFog sets the fog's settings in one transaction and returns the map's row before
// and after, and whether the map got the copy of its image.
func (s *Service) writeFog(ctx context.Context, m authz.Membership, mapID string, msg *mapsv1.SetMapFogRequest, baseLight *string, imageCopy *fogCopy) (before, after mapsdb.Map, copied bool, err error) {
	err = db.InTx(ctx, s.pool, func(tx pgx.Tx) error {
		q := s.queries.WithTx(tx)
		var err error
		copied = false
		if before, err = lockMap(ctx, q, m.CampaignID, mapID); err != nil {
			return err
		}
		if msg.GetFogEnabled() && before.GridColumns == nil {
			return errNoGrid() // the fog is made of squares
		}
		if copied, err = s.useFogCopy(ctx, q, m, before, imageCopy); err != nil {
			return err
		}
		after, err = q.SetMapFog(ctx, mapsdb.SetMapFogParams{
			CampaignID: m.CampaignID, ID: mapID, FogEnabled: msg.FogEnabled, BaseLight: baseLight, GroupVision: msg.GroupVision, Now: s.now(),
		})
		if err != nil {
			return fmt.Errorf("set the fog's settings: %w", err)
		}
		return nil
	})
	return before, after, copied, err
}

// useFogCopy gives the map the copy of its image, and says whether it did. The copy is
// used only if the map still has the image it was made from and the fog is still off.
func (s *Service) useFogCopy(ctx context.Context, q *mapsdb.Queries, m authz.Membership, before mapsdb.Map, imageCopy *fogCopy) (bool, error) {
	if imageCopy == nil || before.FogEnabled || before.ImageID != imageCopy.source.ID {
		return false, nil
	}
	if err := imageCopy.giveMapItsCopy(ctx, q, s, m.CampaignID, m.UserID, before.ID); err != nil {
		return false, err
	}
	return true, nil
}

// The database's base lights (maps_base_light_valid) and the API's.
var (
	baseLightToDB = map[mapsv1.LightLevel]string{
		mapsv1.LightLevel_LIGHT_LEVEL_DARK:   "dark",
		mapsv1.LightLevel_LIGHT_LEVEL_DIM:    "dim",
		mapsv1.LightLevel_LIGHT_LEVEL_BRIGHT: "bright",
	}
	baseLightFromDB = map[string]mapsv1.LightLevel{
		"dark":   mapsv1.LightLevel_LIGHT_LEVEL_DARK,
		"dim":    mapsv1.LightLevel_LIGHT_LEVEL_DIM,
		"bright": mapsv1.LightLevel_LIGHT_LEVEL_BRIGHT,
	}
)
