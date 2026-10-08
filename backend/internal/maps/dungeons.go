package maps

import (
	"bytes"
	"context"
	"crypto/rand"
	"encoding/binary"
	"errors"
	"fmt"
	"slices"
	"sync"
	"time"
	"uuid"

	"connectrpc.com/connect"
	"github.com/jackc/pgx/v5"
	"google.golang.org/protobuf/encoding/protojson"
	"google.golang.org/protobuf/types/known/timestamppb"

	mapsv1 "github.com/PuraFome/meuRPG/backend/gen/meurpg/maps/v1"
	playv1 "github.com/PuraFome/meuRPG/backend/gen/meurpg/play/v1"
	"github.com/PuraFome/meuRPG/backend/internal/authz"
	"github.com/PuraFome/meuRPG/backend/internal/maps/dungeonimg"
	"github.com/PuraFome/meuRPG/backend/internal/maps/images"
	"github.com/PuraFome/meuRPG/backend/internal/maps/mapsdb"
	"github.com/PuraFome/meuRPG/backend/internal/platform/db"
	"github.com/PuraFome/meuRPG/backend/internal/platform/idem"
	"github.com/PuraFome/meuRPG/backend/internal/platform/ratelimit"
	"github.com/PuraFome/meuRPG/backend/internal/rules/dungeon"
	"github.com/PuraFome/meuRPG/backend/internal/rules/grid"
)

// DungeonService (MR-010, RN-26, Etapa 10, slice 10.6d): a generated dungeon
// becomes a campaign map. The generator is package rules/dungeon (pure); this
// file is what the server does around it: validate and run it under a deadline,
// render the flat image (package dungeonimg) and store it as the gallery stores an
// upload (re-encoded, no metadata, ADR-0012), fill the map's layers, and keep the
// dungeon's record (generated_dungeons) for the master's rooms list and for
// "Redesenhar".
//
// Every method is the master's, and a player gets `not_found` from each of them
// (requireDungeonMaster): the rooms list, the options and seed, and a secret
// door are the master's alone (RN-10). Nothing here reaches the live stream
// except the map's own `map_changed`, which says nothing about a dungeon.

// dungeonDeadline is how long the generator may run (spec 6): a result that is
// not ready by then is `deadline_exceeded`. Generation takes single-digit
// milliseconds for every size, 10 ms or so for 199 x 399.
const dungeonDeadline = 2 * time.Second

// dungeonGenerators is how many generations run at once: the preview is asked on
// every change of an option, and each takes a core while it runs.
const dungeonGenerators = 4

// The stairs' names: submap points without a target, which read as stairs (the
// MAP-LANGUAGE-E10 stair badge, up or down). A stair that leads somewhere is the
// master's to link.
const (
	stairsUpName   = "Escada para cima"
	stairsDownName = "Escada para baixo"
)

// newDungeonLimiter limits the creation and the redrawing of dungeons per
// campaign, the heavy writes of this service (a render of up to 32 megapixels and
// an image in the gallery): a burst of 4, then one every 15 seconds, and 20 at once
// for the whole server. In memory, as every limiter of the server.
func newDungeonLimiter() *ratelimit.Limiter {
	return ratelimit.New(ratelimit.Config{
		PerClient:  ratelimit.Rate{Burst: 4, Every: 15 * time.Second},
		Global:     ratelimit.Rate{Burst: 20, Every: time.Second},
		MaxClients: 1024,
	})
}

// campaignSlots lets each campaign hold one slot at a time (the preview's).
type campaignSlots struct {
	mu   sync.Mutex
	busy map[string]bool
}

func (c *campaignSlots) take(id string) bool {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.busy[id] {
		return false
	}
	if c.busy == nil {
		c.busy = map[string]bool{}
	}
	c.busy[id] = true
	return true
}

func (c *campaignSlots) release(id string) {
	c.mu.Lock()
	defer c.mu.Unlock()
	delete(c.busy, id)
}

// requireDungeonMaster is the check at the top of every handler: the campaign's
// master, and `not_found` for anyone else, a player included.
func requireDungeonMaster(ctx context.Context, campaignID string) (authz.Membership, error) {
	m, err := authz.RequireCampaignMember(ctx, campaignID)
	if err != nil {
		return authz.Membership{}, err
	}
	if m.Role != authz.RoleMaster {
		return authz.Membership{}, errMapNotFound()
	}
	return m, nil
}

// errTooManyDungeons is `resource_exhausted` for a campaign that creates or
// redraws dungeons too fast.
func errTooManyDungeons() error {
	return connect.NewError(connect.CodeResourceExhausted, errors.New("too many dungeons were created or redrawn a moment ago; wait a little"))
}

// PreviewDungeon implements mapsv1connect.DungeonServiceHandler.
func (s *Service) PreviewDungeon(
	ctx context.Context,
	req *connect.Request[mapsv1.PreviewDungeonRequest],
) (*connect.Response[mapsv1.PreviewDungeonResponse], error) {
	m, err := requireDungeonMaster(ctx, req.Msg.GetCampaignId())
	if err != nil {
		return nil, err
	}
	// One preview at a time per campaign, on top of the global gate: the page asks
	// again with the latest options, so a second one at once is turned away.
	if !s.previews.take(m.CampaignID) {
		return nil, connect.NewError(connect.CodeResourceExhausted, errors.New("a preview of this campaign is already running"))
	}
	defer s.previews.release(m.CampaignID)
	seed, err := seedOf(req.Msg.Seed)
	if err != nil {
		return nil, s.dbError(ctx, "draw a seed", err)
	}
	d, err := s.generateDungeon(ctx, req.Msg.GetOptions(), seed)
	if err != nil {
		return nil, err
	}
	open, _ := dungeon.WallsMask(d)
	layer := grid.NewLayer(grid.Grid{Columns: d.Width, Rows: d.Height})
	for i, o := range open {
		if o {
			layer.Set(i%d.Width, i/d.Width, true)
		}
	}
	res := &mapsv1.PreviewDungeonResponse{
		Seed: seed, Options: dungeonOptionsToProto(d.Options), GeneratorVersion: int32(d.Version), //nolint:gosec // G115: a small version number
		Width: int32(d.Width), Height: int32(d.Height), Open: layer.Encode(), //nolint:gosec // G115: at most 199 x 399
	}
	res.Doors, res.Stairs = dungeonDoorsProto(d), dungeonStairsProto(d)
	for _, r := range d.Rooms {
		res.Rooms = append(res.Rooms, &mapsv1.DungeonRoomRect{Id: int32(r.ID), Floor: rectOf(r)}) //nolint:gosec // G115: at most 500 rooms
	}
	return connect.NewResponse(res), nil
}

// CreateDungeonMap implements mapsv1connect.DungeonServiceHandler.
func (s *Service) CreateDungeonMap(
	ctx context.Context,
	req *connect.Request[mapsv1.CreateDungeonMapRequest],
) (*connect.Response[mapsv1.CreateDungeonMapResponse], error) {
	m, err := requireDungeonMaster(ctx, req.Msg.GetCampaignId())
	if err != nil {
		return nil, err
	}
	if s.blobs == nil {
		return nil, errImagesOff()
	}
	name, err := cleanName("name", req.Msg.GetName())
	if err != nil {
		return nil, err
	}
	// The key is unique in the campaign, and kept with a hash of the whole request. A retry is
	// answered before the dungeon is drawn again (the render is the costly part): with the first
	// call's map, seed and room count, and with no other map and no count against the limit.
	scopedKey, requestHash, err := keyOf(m.CampaignID, req.Msg.GetIdempotencyKey(), req.Msg)
	if err != nil {
		return nil, err
	}
	if scopedKey != nil {
		prior, err := s.queries.GetMapByCreateKey(ctx, scopedKey)
		switch {
		case err == nil:
			if err := idem.SameRequest(prior.CreateHash, requestHash); err != nil {
				return nil, err
			}
			return s.dungeonReplay(ctx, m, prior)
		case !errors.Is(err, pgx.ErrNoRows):
			return nil, s.dbError(ctx, "find the dungeon of the key", err)
		}
	}
	seed, err := seedOf(req.Msg.Seed)
	if err != nil {
		return nil, s.dbError(ctx, "draw a seed", err)
	}
	// The cheap refusals come first: a bad option never costs a render.
	d, err := s.generateDungeon(ctx, req.Msg.GetOptions(), seed)
	if err != nil {
		return nil, err
	}
	// Whether there is room comes before the render and before a token of the
	// dungeon limit: a campaign at its limits is refused without costing the
	// server a drawing, or the table a try. The transaction below decides for
	// good; this only refuses what it would refuse.
	if err := s.checkDungeonRoom(ctx, m.CampaignID, true, freedRoom{}); err != nil {
		return nil, err
	}
	if ok, _ := s.dungeonLimit.Allow(m.CampaignID); !ok {
		return nil, errTooManyDungeons()
	}
	// Valid from here: the map is made even if the caller goes away (the page may be
	// closed meanwhile; "se você sair, continua sendo criado").
	ctx = context.WithoutCancel(ctx)

	g := grid.Grid{Columns: d.Width, Rows: d.Height}
	layers := dungeonLayers(d, g)
	solid := solidOf(d.Width, d.Height, layers.walls, layers.doors)
	res, err := s.drawDungeon(ctx, solid, d.Width, d.Height, 0, 0)
	if err != nil {
		return nil, err
	}
	rooms, err := marshalRooms(d)
	if err != nil {
		return nil, s.dbError(ctx, "keep the dungeon's rooms", err)
	}
	options, err := protojson.Marshal(dungeonOptionsToProto(d.Options))
	if err != nil {
		return nil, s.dbError(ctx, "keep the dungeon's options", err)
	}

	imageID := uuid.New().String()
	if err := s.putImageFiles(ctx, m.CampaignID, imageID, res); err != nil {
		return nil, err
	}
	var created mapsdb.Map
	replayed := false
	err = db.InTx(ctx, s.pool, func(tx pgx.Tx) error {
		q := s.queries.WithTx(tx)
		var err error
		// The first lookup was outside the transaction: this one is what holds when two calls with the
		// same key race (the second finds the first's map, and its own files are deleted below).
		created, replayed, err = idem.Create(ctx, scopedKey, requestHash, q.GetMapByCreateKey,
			func(row mapsdb.Map) *string { return row.CreateHash },
			func() (mapsdb.Map, error) {
				count, err := q.CountMaps(ctx, m.CampaignID)
				if err != nil {
					return created, fmt.Errorf("count maps: %w", err)
				}
				if count >= s.maxMaps {
					return created, connect.NewError(connect.CodeResourceExhausted, fmt.Errorf("the campaign already has %d maps", s.maxMaps))
				}
				if _, err := s.insertGeneratedImage(ctx, q, m, imageID, nameWithSuffix(name, dungeonImageSuffix), res, freedRoom{}); err != nil {
					return created, err
				}
				row, err := q.InsertMap(ctx, mapsdb.InsertMapParams{
					CampaignID: m.CampaignID, Name: name, ImageID: imageID, CreateKey: scopedKey, CreateHash: requestHash, Now: s.now(),
				})
				if errors.Is(err, pgx.ErrNoRows) {
					return row, err // another call with the key won the race: its map is ours
				}
				if err != nil {
					return row, fmt.Errorf("insert map: %w", err)
				}
				// The grid first, then the fog: the fog needs a grid. Fog on, base light
				// "Claro" (MR-010, decided 06/10/2026): a secret room stays dark to a player
				// until someone sees it, whatever the image shows.
				cols := int32(d.Width) //nolint:gosec // G115: at most 199
				if _, err := q.SetMapGrid(ctx, mapsdb.SetMapGridParams{CampaignID: m.CampaignID, ID: row.ID, GridColumns: &cols, GridFactor: 1, Now: s.now()}); err != nil {
					return row, fmt.Errorf("set the grid: %w", err)
				}
				fog, light := true, baseLightToDB[mapsv1.LightLevel_LIGHT_LEVEL_BRIGHT]
				if _, err := q.SetMapFog(ctx, mapsdb.SetMapFogParams{CampaignID: m.CampaignID, ID: row.ID, FogEnabled: &fog, BaseLight: &light, Now: s.now()}); err != nil {
					return row, fmt.Errorf("set the fog: %w", err)
				}
				if err := q.UpsertMapLayers(ctx, mapsdb.UpsertMapLayersParams{
					MapID: row.ID, Walls: nilIfBlank(layers.walls.Encode()), Doors: nilIfBlank(layers.doors.Encode()), Now: s.now(),
				}); err != nil {
					return row, fmt.Errorf("write the layers: %w", err)
				}
				if _, err := q.BumpMapLayersRevision(ctx, row.ID); err != nil {
					return row, fmt.Errorf("bump the layers' revision: %w", err)
				}
				for _, st := range d.Stairs {
					if err := s.insertStairs(ctx, q, row.ID, g, st); err != nil {
						return row, err
					}
				}
				seedBits := int64(seed) //nolint:gosec // G115: the same 64 bits, kept as an INT8
				return row, q.InsertGeneratedDungeon(ctx, mapsdb.InsertGeneratedDungeonParams{
					MapID: row.ID, GeneratorVersion: int32(d.Version), Seed: seedBits, //nolint:gosec // G115: a small version number
					Width: int32(d.Width), Height: int32(d.Height), //nolint:gosec // G115: at most 199 x 399
					Options: options, Cells: packCells(d.Kinds), Rooms: rooms, ImageID: &imageID, CreatedAt: s.now(),
				})
			})
		return err
	})
	if err != nil || replayed {
		s.deleteFiles(ctx, m.CampaignID, imageID)
	}
	if err != nil {
		return nil, s.dbError(ctx, "create a dungeon map", err)
	}
	if replayed {
		return s.dungeonReplay(ctx, m, created)
	}
	// A new map is hidden: only the master hears about it.
	s.publishMapChanged(m.CampaignID, created.ID, false)
	out, err := s.masterMap(ctx, m, created.ID)
	if err != nil {
		return nil, err
	}
	return connect.NewResponse(&mapsv1.CreateDungeonMapResponse{Map: out, Seed: seed, RoomCount: int32(len(d.Rooms))}), nil //nolint:gosec // G115: at most 500 rooms
}

// dungeonReplay answers a CreateDungeonMap whose key already made its map: the first call's
// map, seed and room count, read back from the stored dungeon. It announces nothing (the first
// call did).
func (s *Service) dungeonReplay(ctx context.Context, m authz.Membership, prior mapsdb.Map) (*connect.Response[mapsv1.CreateDungeonMapResponse], error) {
	rec, err := s.dungeonOf(ctx, s.queries, m.CampaignID, prior.ID)
	if err != nil {
		return nil, s.dbError(ctx, "read the dungeon of the key", err)
	}
	stored := &mapsv1.GetDungeonRoomsResponse{}
	if err := protojson.Unmarshal(rec.Rooms, stored); err != nil {
		return nil, s.dbError(ctx, "read the dungeon of the key", fmt.Errorf("decode the stored rooms: %w", err))
	}
	out, err := s.masterMap(ctx, m, prior.ID)
	if err != nil {
		return nil, err
	}
	return connect.NewResponse(&mapsv1.CreateDungeonMapResponse{
		Map: out, Seed: uint64(rec.Seed), RoomCount: int32(len(stored.GetRooms())), //nolint:gosec // G115: the same 64 bits; at most 500 rooms
	}), nil
}

// GetDungeonRooms implements mapsv1connect.DungeonServiceHandler.
func (s *Service) GetDungeonRooms(
	ctx context.Context,
	req *connect.Request[mapsv1.GetDungeonRoomsRequest],
) (*connect.Response[mapsv1.GetDungeonRoomsResponse], error) {
	m, err := requireDungeonMaster(ctx, req.Msg.GetCampaignId())
	if err != nil {
		return nil, err
	}
	mapID, ok := parseID(req.Msg.GetMapId())
	if !ok {
		return nil, errMapNotFound()
	}
	rec, err := s.dungeonOf(ctx, s.queries, m.CampaignID, mapID)
	if err != nil {
		return nil, s.dbError(ctx, "read a dungeon's rooms", err)
	}
	res := &mapsv1.GetDungeonRoomsResponse{}
	if err := protojson.Unmarshal(rec.Rooms, res); err != nil {
		return nil, s.dbError(ctx, "read a dungeon's rooms", fmt.Errorf("decode the stored rooms: %w", err))
	}
	res.Options = &mapsv1.DungeonOptions{}
	if err := protojson.Unmarshal(rec.Options, res.Options); err != nil {
		return nil, s.dbError(ctx, "read a dungeon's options", fmt.Errorf("decode the stored options: %w", err))
	}
	row, err := s.campaignMap(ctx, s.queries, m.CampaignID, mapID)
	if err != nil {
		return nil, s.dbError(ctx, "read the dungeon's map", err)
	}
	res.ImageIsGenerated = isGeneratedImage(rec, row)
	// The scenes in each room: the map's scene points whose square is on its floor.
	points, err := s.queries.ListMapPoints(ctx, mapID)
	if err != nil {
		return nil, s.dbError(ctx, "list the map's scenes", err)
	}
	g := grid.Grid{Columns: int(rec.Width), Rows: int(rec.Height)}
	for _, p := range points {
		if p.Kind != kindToDB[mapsv1.MapPointKind_MAP_POINT_KIND_SCENE] {
			continue
		}
		sq := g.SquareOf(int(p.XBp), int(p.YBp))
		for _, r := range res.Rooms {
			f := r.GetFloor()
			if int32(sq.Col) >= f.GetX() && int32(sq.Col) < f.GetX()+f.GetWidth() && int32(sq.Row) >= f.GetY() && int32(sq.Row) < f.GetY()+f.GetHeight() { //nolint:gosec // G115: inside the grid
				r.ScenePointIds = append(r.ScenePointIds, p.ID)
			}
		}
	}
	res.Seed = uint64(rec.Seed) //nolint:gosec // G115: the same 64 bits
	res.GeneratorVersion, res.Width, res.Height, res.CreatedAt = rec.GeneratorVersion, rec.Width, rec.Height, timestamppb.New(rec.CreatedAt)
	return connect.NewResponse(res), nil
}

// PlaceDungeonScene implements mapsv1connect.DungeonServiceHandler. The scene is
// made by CreateMapPoint, the one path every point of interest goes through, so
// its limits, its hidden start and its answer are the ones a hand-made point has.
func (s *Service) PlaceDungeonScene(
	ctx context.Context,
	req *connect.Request[mapsv1.PlaceDungeonSceneRequest],
) (*connect.Response[mapsv1.PlaceDungeonSceneResponse], error) {
	m, err := requireDungeonMaster(ctx, req.Msg.GetCampaignId())
	if err != nil {
		return nil, err
	}
	mapID, ok := parseID(req.Msg.GetMapId())
	if !ok {
		return nil, errMapNotFound()
	}
	rec, err := s.dungeonOf(ctx, s.queries, m.CampaignID, mapID)
	if err != nil {
		return nil, s.dbError(ctx, "read a dungeon's rooms", err)
	}
	stored := &mapsv1.GetDungeonRoomsResponse{}
	if err := protojson.Unmarshal(rec.Rooms, stored); err != nil {
		return nil, s.dbError(ctx, "read a dungeon's rooms", fmt.Errorf("decode the stored rooms: %w", err))
	}
	var room *mapsv1.DungeonRoom
	for _, r := range stored.GetRooms() {
		if r.GetId() == req.Msg.GetRoomId() {
			room = r
		}
	}
	if room == nil {
		return nil, errMapNotFound()
	}
	x, y := grid.Grid{Columns: int(rec.Width), Rows: int(rec.Height)}.CenterOf(grid.Square{Col: int(room.GetCenterCol()), Row: int(room.GetCenterRow())})
	res, err := s.CreateMapPoint(ctx, connect.NewRequest(&mapsv1.CreateMapPointRequest{
		CampaignId: m.CampaignID, MapId: mapID, Kind: mapsv1.MapPointKind_MAP_POINT_KIND_SCENE,
		Name: fmt.Sprintf("Sala %d", room.GetId()), XBp: int32(x), YBp: int32(y), //nolint:gosec // G115: 0 to 10000
		IdempotencyKey: req.Msg.GetIdempotencyKey(),
	}))
	if err != nil {
		return nil, err
	}
	return connect.NewResponse(&mapsv1.PlaceDungeonSceneResponse{Point: res.Msg.GetPoint()}), nil
}

// RedrawDungeonMap implements mapsv1connect.DungeonServiceHandler.
func (s *Service) RedrawDungeonMap(
	ctx context.Context,
	req *connect.Request[mapsv1.RedrawDungeonMapRequest],
) (*connect.Response[mapsv1.RedrawDungeonMapResponse], error) {
	m, err := requireDungeonMaster(ctx, req.Msg.GetCampaignId())
	if err != nil {
		return nil, err
	}
	if s.blobs == nil {
		return nil, errImagesOff()
	}
	mapID, ok := parseID(req.Msg.GetMapId())
	if !ok {
		return nil, errMapNotFound()
	}
	// What the new image is drawn from: the map, its layers and the dungeon's cells,
	// read together. The transaction below reads them again and checks they are the same.
	rec, err := s.dungeonOf(ctx, s.queries, m.CampaignID, mapID)
	if err != nil {
		return nil, s.dbError(ctx, "read a dungeon", err)
	}
	row, err := s.campaignMap(ctx, s.queries, m.CampaignID, mapID)
	if err != nil {
		return nil, s.dbError(ctx, "read a map", err)
	}
	if row.GridColumns == nil {
		return nil, errNoGrid()
	}
	// Only the generator's own image is ever replaced, and deleted: when the master put
	// another image on the map, the map is no longer the dungeon's.
	if !isGeneratedImage(rec, row) {
		return nil, errImageChanged()
	}
	old, err := s.queries.GetGalleryImageInCampaign(ctx, mapsdb.GetGalleryImageInCampaignParams{CampaignID: m.CampaignID, ID: row.ImageID})
	if err != nil {
		return nil, s.dbError(ctx, "read the map's image", err)
	}
	// The grid is read the way the map reads it (gridOf), and must still be the dungeon's,
	// on an image of the size drawn for it. A dungeon is drawn one square of the image per
	// square of the rules (calibration factor 1, RN-25): a map calibrated since is no
	// longer drawn the dungeon's way.
	g := gridOf(row.GridColumns, row.GridFactor, old.Width, old.Height)
	wantW, wantH := dungeonimg.Size(int(rec.Width), int(rec.Height))
	if row.GridFactor != 1 || g.Columns != int(rec.Width) || g.Rows != int(rec.Height) || int(old.Width) != wantW || int(old.Height) != wantH {
		return nil, errImageChanged()
	}
	stored, err := s.queries.GetMapLayers(ctx, mapID)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, errImageChanged() // the layers were cleared (a new image or grid does that)
	}
	if err != nil {
		return nil, s.dbError(ctx, "read the map's layers", err)
	}
	// The old image goes with the replacement unless something else uses it. The
	// transaction asks again what it can (another map, an image left with the players,
	// the shown image); what it cannot read (the stage, a portrait) is asked here.
	used, err := s.imageUsedElsewhere(ctx, m.CampaignID, mapID, old.ID)
	if err != nil {
		return nil, s.dbError(ctx, "read whether the image is used elsewhere", err)
	}
	// As in CreateDungeonMap: a full gallery is refused before the render, unless the
	// old image leaves with the redraw and the gallery does not grow.
	var freed freedRoom
	if !used {
		freed = freedRoom{images: 1, bytes: int64(old.ByteSize)}
	}
	if err := s.checkDungeonRoom(ctx, m.CampaignID, false, freed); err != nil {
		return nil, err
	}
	if ok, _ := s.dungeonLimit.Allow(m.CampaignID); !ok {
		return nil, errTooManyDungeons()
	}
	current, err := s.currentMap(ctx, m.CampaignID)
	if err != nil {
		return nil, err
	}
	set := loadLayers(stored, g)
	solid := solidOf(g.Columns, g.Rows, set.walls, set.doors)
	res, err := s.drawDungeon(ctx, solid, g.Columns, g.Rows, int(old.Width), int(old.Height))
	if err != nil {
		return nil, err
	}
	newID := uuid.New().String()
	if err := s.putImageFiles(ctx, m.CampaignID, newID, res); err != nil {
		return nil, err
	}
	if s.beforeRedrawTx != nil {
		s.beforeRedrawTx() // tests change the map here, between the render and the transaction
	}
	var updated mapsdb.Map
	deleted, portraits := false, int64(0)
	err = db.InTx(ctx, s.pool, func(tx pgx.Tx) error {
		q := s.queries.WithTx(tx)
		deleted, portraits = false, 0
		locked, err := q.GetMapForUpdate(ctx, mapsdb.GetMapForUpdateParams{CampaignID: m.CampaignID, ID: mapID})
		if errors.Is(err, pgx.ErrNoRows) {
			return errMapNotFound()
		}
		if err != nil {
			return fmt.Errorf("find map: %w", err)
		}
		again, err := s.dungeonOf(ctx, q, m.CampaignID, mapID)
		if err != nil {
			return err
		}
		if !isGeneratedImage(again, locked) || locked.ImageID != row.ImageID || locked.GridFactor != row.GridFactor || int32PtrValue(locked.GridColumns) != int32PtrValue(row.GridColumns) {
			return errImageChanged()
		}
		now, err := q.GetMapLayers(ctx, mapID)
		if errors.Is(err, pgx.ErrNoRows) {
			return errImageChanged()
		}
		if err != nil {
			return fmt.Errorf("read the layers: %w", err)
		}
		// What the image depends on, not the layers' revision: a door a character opens
		// during play bumps the revision and changes nothing the image shows (an open door
		// is floor, like a closed one). A wall, or a secret door, that changed would show.
		nowSet := loadLayers(now, g)
		if !slices.Equal(solid, solidOf(g.Columns, g.Rows, nowSet.walls, nowSet.doors)) {
			return errStaleMap()
		}
		// Whether the old image goes with the redraw, asked before the new one is added:
		// it frees its place in the gallery, so a full gallery does not grow.
		elsewhere, err := q.ImageIsUsedElsewhere(ctx, mapsdb.ImageIsUsedElsewhereParams{CampaignID: m.CampaignID, ImageID: old.ID, MapID: mapID})
		if err != nil {
			return fmt.Errorf("read whether the image is used elsewhere: %w", err)
		}
		// The screen is asked again here: the master may have shown the old image
		// since the first read, and deleting it would blank the screen. (A portrait
		// that raced in loses the image, as in DeleteGalleryImage.)
		shown := false
		if !used && !elsewhere {
			if shown, err = s.live.ImageShown(ctx, tx, m.CampaignID, old.ID); err != nil {
				return fmt.Errorf("read whether the image is shown: %w", err)
			}
		}
		remove := !used && !elsewhere && !shown
		var freed freedRoom
		if remove {
			freed = freedRoom{images: 1, bytes: int64(old.ByteSize)}
		}
		fresh, err := s.insertGeneratedImage(ctx, q, m, newID, nameWithSuffix(row.Name, dungeonImageSuffix), res, freed)
		if err != nil {
			return err
		}
		if updated, err = s.replaceMapImageKeepingLayers(ctx, q, locked, old, fresh); err != nil {
			return err
		}
		if err := q.SetGeneratedDungeonImage(ctx, mapsdb.SetGeneratedDungeonImageParams{MapID: mapID, ImageID: &newID}); err != nil {
			return fmt.Errorf("record the dungeon's new image: %w", err)
		}
		if !remove {
			return nil
		}
		// As DeleteGalleryImage does: a portrait that raced in loses the image.
		if portraits, err = s.characters.ClearPortraits(ctx, tx, m.CampaignID, old.ID); err != nil {
			return fmt.Errorf("clear the portraits: %w", err)
		}
		if _, err := q.DeleteGalleryImage(ctx, mapsdb.DeleteGalleryImageParams{CampaignID: m.CampaignID, ID: old.ID}); err != nil {
			return fmt.Errorf("delete the old image: %w", err)
		}
		deleted = true
		return nil
	})
	if err != nil {
		s.deleteFiles(ctx, m.CampaignID, newID)
		return nil, s.dbError(ctx, "redraw a dungeon map", err)
	}
	if deleted {
		s.deleteFiles(ctx, m.CampaignID, old.ID)
	}
	if portraits > 0 {
		s.live.Publish(m.CampaignID, true, &playv1.WatchGameSessionResponse{Event: &playv1.WatchGameSessionResponse_StageChanged_{
			StageChanged: &playv1.WatchGameSessionResponse_StageChanged{},
		}})
	}
	s.tiles.forget(mapID) // the fog's tiles are cut from the image
	s.publishMapChanged(m.CampaignID, mapID, playersSee(mapID, updated.RevealedAt, current))
	out, err := s.masterMap(ctx, m, mapID)
	if err != nil {
		return nil, err
	}
	return connect.NewResponse(&mapsv1.RedrawDungeonMapResponse{Map: out}), nil
}

// isGeneratedImage says whether the map's image is the one the generator drew for it.
func isGeneratedImage(rec mapsdb.GeneratedDungeon, row mapsdb.Map) bool {
	return rec.ImageID != nil && *rec.ImageID == row.ImageID
}

// errImageChanged is `failed_precondition` IMAGE_CHANGED: the map is no longer the
// generated dungeon's.
func errImageChanged() error {
	return errMapBlocked(mapsv1.MapBlockedReason_MAP_BLOCKED_REASON_IMAGE_CHANGED,
		"the map is no longer the generated dungeon's: its image, size, grid or layers changed")
}

// replaceMapImageKeepingLayers makes fresh the map's image without clearing
// anything: the painted layers, the points, the tokens and what the players
// remember all stay. It is the one operation that does it: every other new image
// goes through UpdateMap, which clears the layers because they are sized by the
// grid. This one is only safe because it checks that nothing the layers depend on
// changes: the old image is the map's current one, the new image is of the same
// campaign and has the old one's size in pixels, and the map has a grid (so its
// columns and rows are the same). "Redesenhar" and the textured map (10.8) use it,
// inside a transaction that holds the map's row lock (locked comes from
// GetMapForUpdate). A map's revision still goes up, as for any new image.
func (s *Service) replaceMapImageKeepingLayers(ctx context.Context, q *mapsdb.Queries, locked mapsdb.Map, old, fresh mapsdb.GalleryImage) (mapsdb.Map, error) {
	if locked.GridColumns == nil {
		return mapsdb.Map{}, errNoGrid()
	}
	if locked.ImageID != old.ID {
		return mapsdb.Map{}, errImageChanged()
	}
	if fresh.CampaignID != locked.CampaignID || old.CampaignID != locked.CampaignID {
		return mapsdb.Map{}, errNotAGalleryImage()
	}
	if fresh.Width != old.Width || fresh.Height != old.Height {
		return mapsdb.Map{}, errMapBlocked(mapsv1.MapBlockedReason_MAP_BLOCKED_REASON_IMAGE_CHANGED,
			"the new image must have the old one's size in pixels, so the layers still fit")
	}
	updated, err := q.SetMapImageOnly(ctx, mapsdb.SetMapImageOnlyParams{CampaignID: locked.CampaignID, ID: locked.ID, ImageID: fresh.ID, Now: s.now()})
	if err != nil {
		return mapsdb.Map{}, fmt.Errorf("give the map its new image: %w", err)
	}
	return updated, nil
}

// dungeonImageSuffix names the gallery image of a generated map.
const dungeonImageSuffix = " (masmorra)"

// nameWithSuffix is name and then suffix, within 80 characters.
func nameWithSuffix(name, suffix string) string { return copyNameWith(name, suffix) }

// dungeonOf finds the map's dungeon record, or `not_found`.
func (s *Service) dungeonOf(ctx context.Context, q *mapsdb.Queries, campaignID, mapID string) (mapsdb.GeneratedDungeon, error) {
	rec, err := q.GetGeneratedDungeon(ctx, mapsdb.GetGeneratedDungeonParams{CampaignID: campaignID, MapID: mapID})
	if errors.Is(err, pgx.ErrNoRows) {
		return mapsdb.GeneratedDungeon{}, errMapNotFound()
	}
	if err != nil {
		return mapsdb.GeneratedDungeon{}, fmt.Errorf("find the dungeon: %w", err)
	}
	return rec, nil
}

// imageUsedElsewhere says whether an image of a map is also used another way: the
// background of another map, an image left with the players, the one the session
// shows, a portrait or the stage's. The same questions the fog's copy asks.
func (s *Service) imageUsedElsewhere(ctx context.Context, campaignID, mapID, imageID string) (bool, error) {
	inPortrait, err := s.characters.PortraitInUse(ctx, campaignID, imageID)
	if err != nil {
		return false, fmt.Errorf("read whether the image is a portrait: %w", err)
	}
	used, err := s.queries.ImageIsUsedElsewhere(ctx, mapsdb.ImageIsUsedElsewhereParams{CampaignID: campaignID, ImageID: imageID, MapID: mapID})
	if err != nil {
		return false, fmt.Errorf("read whether the image is used elsewhere: %w", err)
	}
	if used || inPortrait {
		return true, nil
	}
	if _, shown, err := s.live.OnScreen(ctx, campaignID); err != nil {
		return false, fmt.Errorf("read the shown image: %w", err)
	} else if shown == imageID {
		return true, nil
	}
	onStage, err := s.live.ImageOnStage(ctx, campaignID, imageID)
	if err != nil {
		return false, fmt.Errorf("read the stage: %w", err)
	}
	return onStage, nil
}

// drawDungeon renders the floor plan as a PNG, one image at a time (the
// processing gate that uploads use, so the server never holds two big images). w
// and h are the image's size in pixels, or 0 for the size dungeonimg picks for
// the grid.
func (s *Service) drawDungeon(ctx context.Context, solid []bool, cols, rows, w, h int) (*images.Result, error) {
	select {
	case s.processing <- struct{}{}:
	case <-ctx.Done():
		return nil, ctx.Err()
	}
	defer func() { <-s.processing }()
	if w == 0 || h == 0 {
		w, h = dungeonimg.Size(cols, rows)
	}
	img, err := dungeonimg.Render(dungeonimg.Floorplan{Cols: cols, Rows: rows, Solid: solid}, w, h)
	if err != nil {
		return nil, fmt.Errorf("render the dungeon: %w", err)
	}
	res, err := images.Encode(img)
	if err != nil {
		return nil, fmt.Errorf("encode the dungeon's image: %w", err)
	}
	return res, nil
}

// checkDungeonRoom refuses, before anything is drawn, a dungeon the transaction
// would refuse: a campaign at its map limit (only for a new map) or whose
// gallery has no room for one more image. It reads through the pool, so it
// runs outside the transaction, and it is only a first look: the transaction
// counts again under its own snapshot.
func (s *Service) checkDungeonRoom(ctx context.Context, campaignID string, newMap bool, freed freedRoom) error {
	if newMap {
		count, err := s.queries.CountMaps(ctx, campaignID)
		if err != nil {
			return s.dbError(ctx, "count maps", err)
		}
		if count >= s.maxMaps {
			return connect.NewError(connect.CodeResourceExhausted, fmt.Errorf("the campaign already has %d maps", s.maxMaps))
		}
	}
	usage, err := s.queries.GetGalleryUsage(ctx, campaignID)
	if err != nil {
		return s.dbError(ctx, "read the gallery usage", err)
	}
	if usage.ImageCount-freed.images >= s.maxImages || usage.ByteCount-freed.bytes >= int64(s.maxBytes) {
		return connect.NewError(connect.CodeResourceExhausted, errors.New("the campaign's gallery is full"))
	}
	return nil
}

// freedRoom is the room in the gallery an operation gives back in the same step that
// adds an image: a redraw whose old image goes away.
type freedRoom struct {
	images int32
	bytes  int64
}

// putImageFiles writes the image's two files, as an upload does before its row.
func (s *Service) putImageFiles(ctx context.Context, campaignID, imageID string, res *images.Result) error {
	imageKey, thumbnailKey := blobKeys(campaignID, imageID)
	for _, f := range []struct {
		key     string
		content []byte
	}{{imageKey, res.Data}, {thumbnailKey, res.Thumbnail}} {
		if err := s.blobs.Put(ctx, f.key, res.ContentType, bytes.NewReader(f.content)); err != nil {
			s.deleteFiles(ctx, campaignID, imageID)
			s.logger.ErrorContext(ctx, "maps: cannot store a generated image file", "error", err)
			return connect.NewError(connect.CodeUnavailable, errors.New("cannot store the image right now, please try again"))
		}
	}
	return nil
}

// insertGeneratedImage adds the image's row to the campaign's gallery inside the
// caller's transaction, checking the quota the way the upload does (less the room
// freed in the same step).
func (s *Service) insertGeneratedImage(ctx context.Context, q *mapsdb.Queries, m authz.Membership, id, name string, res *images.Result, freed freedRoom) (mapsdb.GalleryImage, error) {
	usage, err := q.GetGalleryUsage(ctx, m.CampaignID)
	if err != nil {
		return mapsdb.GalleryImage{}, fmt.Errorf("read the gallery usage: %w", err)
	}
	size := int32(len(res.Data)) //nolint:gosec // G115: at most images.MaxBytes
	if usage.ImageCount-freed.images >= s.maxImages || usage.ByteCount-freed.bytes+int64(size) > int64(s.maxBytes) {
		return mapsdb.GalleryImage{}, connect.NewError(connect.CodeResourceExhausted, errors.New("the campaign's gallery is full"))
	}
	row, err := q.InsertGalleryImage(ctx, mapsdb.InsertGalleryImageParams{
		ID: id, CampaignID: m.CampaignID, UploadedBy: &m.UserID, Name: name, ContentType: res.ContentType,
		Width: int32(res.Width), Height: int32(res.Height), ByteSize: size, CreatedAt: s.now(), //nolint:gosec // G115: at most images.MaxSide
	})
	if err != nil {
		return mapsdb.GalleryImage{}, fmt.Errorf("insert the generated image: %w", err)
	}
	return row, nil
}

// insertStairs adds a stair of the dungeon as a submap point with no target.
func (s *Service) insertStairs(ctx context.Context, q *mapsdb.Queries, mapID string, g grid.Grid, st dungeon.Stair) error {
	name, dir := stairsDownName, "down"
	if st.Kind == dungeon.StairUp {
		name, dir = stairsUpName, "up"
	}
	x, y := g.CenterOf(grid.Square{Col: st.X, Row: st.Y})
	// A stair is a visible feature like a door: born revealed. With the fog on, a player
	// sees it only where they see its square (MAP-LANGUAGE-E10).
	now := s.now()
	_, err := q.InsertMapPoint(ctx, mapsdb.InsertMapPointParams{
		MapID: mapID, Kind: kindToDB[mapsv1.MapPointKind_MAP_POINT_KIND_SUBMAP], Name: name,
		XBp: int32(x), YBp: int32(y), Now: now, RevealedAt: &now, Stairs: &dir, //nolint:gosec // G115: 0 to 10000
	})
	if err != nil {
		return fmt.Errorf("insert a stair: %w", err)
	}
	return nil
}

// generateDungeon validates the options and runs the generator under the
// deadline. The generator checks every option before it draws anything, so a bad
// one costs nothing; a refused option is `invalid_argument` with the
// DungeonOptionRefused detail. The generator cannot be interrupted, so it runs on
// its own goroutine: when the deadline passes the call answers
// `deadline_exceeded` and the goroutine finishes alone (the gate keeps their number
// down).
func (s *Service) generateDungeon(ctx context.Context, in *mapsv1.DungeonOptions, seed uint64) (*dungeon.Dungeon, error) {
	opts, err := dungeonOptionsFromProto(in, seed)
	if err != nil {
		return nil, s.generatorError(ctx, err)
	}
	timer := time.NewTimer(dungeonDeadline)
	defer timer.Stop()
	select {
	case s.dungeonGate <- struct{}{}:
	case <-timer.C:
		return nil, connect.NewError(connect.CodeDeadlineExceeded, errors.New("the server is busy generating dungeons, try again"))
	case <-ctx.Done():
		return nil, ctx.Err()
	}
	type result struct {
		d   *dungeon.Dungeon
		err error
	}
	done := make(chan result, 1)
	go func() {
		defer func() { <-s.dungeonGate }()
		defer func() {
			if r := recover(); r != nil {
				done <- result{err: fmt.Errorf("the generator panicked: %v", r)}
			}
		}()
		d, err := dungeon.Generate(opts)
		done <- result{d, err}
	}()
	select {
	case r := <-done:
		return r.d, s.generatorError(ctx, r.err)
	case <-timer.C:
		return nil, connect.NewError(connect.CodeDeadlineExceeded, errors.New("the dungeon took too long to generate"))
	case <-ctx.Done():
		return nil, ctx.Err()
	}
}

// generatorError turns an error of the generator into the answer.
func (s *Service) generatorError(ctx context.Context, err error) error {
	if err == nil {
		return nil
	}
	refused := func(option string, cause error) error {
		e := connect.NewError(connect.CodeInvalidArgument, cause)
		if detail, detailErr := connect.NewErrorDetail(&mapsv1.DungeonOptionRefused{Option: option}); detailErr == nil {
			e.AddDetail(detail)
		}
		return e
	}
	if oe, ok := errors.AsType[*dungeon.OptionError](err); ok {
		return refused(oe.Option, oe)
	}
	if errors.Is(err, dungeon.ErrNoSpace) {
		return refused("", err)
	}
	s.logger.ErrorContext(ctx, "maps: the dungeon generator failed", "error", err)
	return connect.NewError(connect.CodeInternal, errors.New("cannot generate the dungeon"))
}

// seedOf is the seed the request gave, or a new random one.
func seedOf(given *uint64) (uint64, error) {
	if given != nil {
		return *given, nil
	}
	var b [8]byte
	if _, err := rand.Read(b[:]); err != nil {
		return 0, fmt.Errorf("draw a seed: %w", err)
	}
	return binary.BigEndian.Uint64(b[:]), nil
}

// dungeonLayers are the walls and doors layers of a generated dungeon. The walls
// cover every square that is not open: the walls and all the rock behind them, so
// one wall mark covers the whole mass, the image's hatch agrees with the editor's, and
// no token can be dropped on rock. The doors follow
// the markers: a passage ("archway") is floor and puts nothing in the layer;
// closed, locked, barred and secret doors put their own state; a door with a trap
// is the same door (RN-26: the trap is a note in the rooms list). The squares of
// the layers are the dungeon's own, as the map's grid has the dungeon's width.
func dungeonLayers(d *dungeon.Dungeon, g grid.Grid) layerSet {
	open, _ := dungeon.WallsMask(d)
	set := layerSet{walls: grid.NewLayer(g), doors: grid.NewDoorLayer(g)}
	for i, o := range open {
		if !o {
			set.walls.Set(i%d.Width, i/d.Width, true)
		}
	}
	for _, mk := range dungeon.Markers(d) {
		if mk.Type != "door" {
			continue
		}
		if state, ok := doorStateOf[mk.Door]; ok {
			set.doors.Set(mk.X, mk.Y, state)
		}
	}
	return set
}

// doorStateOf is the doors layer's state of each generated kind; a passage has
// none.
var doorStateOf = map[dungeon.DoorKind]grid.Door{
	dungeon.DoorClosed: grid.DoorClosed,
	dungeon.DoorBarred: grid.DoorBarred,
	dungeon.DoorLocked: grid.DoorLocked,
	dungeon.DoorSecret: grid.DoorSecret,
}

// solidOf is the floor plan of the image, from a map's walls and doors layers: a pure
// function of them, which is why "Redesenhar" shows the master's edits and creation
// shows the dungeon. A square is solid when the walls layer says so (it covers every
// wall and all the rock) or when it holds a secret door (drawn as wall, RN-10: the
// image never gives it away); any other square, a door's included, is floor.
func solidOf(w, h int, walls *grid.Layer, doors *grid.DoorLayer) []bool {
	solid := make([]bool, w*h)
	for i := range solid {
		col, row := i%w, i/w
		solid[i] = walls.Get(col, row) || doors.Get(col, row) == grid.DoorSecret
	}
	return solid
}

// packCells keeps the dungeon's kinds, two bits a square (see migration 00130).
func packCells(kinds []dungeon.Kind) []byte {
	out := make([]byte, (len(kinds)+3)/4)
	for i, k := range kinds {
		out[i/4] |= byte(k&3) << (2 * (i % 4))
	}
	return out
}

// unpackCells is packCells' reverse for n squares.
func unpackCells(b []byte, n int) []dungeon.Kind {
	out := make([]dungeon.Kind, n)
	for i := range out {
		out[i] = dungeon.Kind(b[i/4] >> (2 * (i % 4)) & 3)
	}
	return out
}

// marshalRooms is the rooms as the table keeps them: a GetDungeonRoomsResponse
// with only its rooms, in JSON.
func marshalRooms(d *dungeon.Dungeon) ([]byte, error) {
	res := &mapsv1.GetDungeonRoomsResponse{
		Doors: dungeonDoorsProto(d), Stairs: dungeonStairsProto(d),
		Entrance: &mapsv1.DungeonEntrance{X: int32(d.Entrance.X), Y: int32(d.Entrance.Y), OnStairs: d.Entrance.OnStairs}, //nolint:gosec // G115: inside the grid
	}
	for _, r := range d.Rooms {
		cx, cy := r.Center()
		room := &mapsv1.DungeonRoom{Id: int32(r.ID), Floor: rectOf(r), CenterCol: int32(cx), CenterRow: int32(cy)} //nolint:gosec // G115: inside the grid
		for _, e := range r.Exits {
			room.Exits = append(room.Exits, &mapsv1.DungeonExit{
				Side: sideToProto[e.Side], X: int32(e.X), Y: int32(e.Y), Kind: doorKindToProto[e.Kind], Trapped: e.Trapped, OtherRoomId: int32(e.Other), //nolint:gosec // G115: inside the grid
			})
		}
		res.Rooms = append(res.Rooms, room)
	}
	return protojson.Marshal(res)
}

func rectOf(r dungeon.Room) *mapsv1.DungeonRect {
	return &mapsv1.DungeonRect{X: int32(r.X), Y: int32(r.Y), Width: int32(r.Width), Height: int32(r.Height)} //nolint:gosec // G115: inside the grid
}

// The generator's enums and the API's.
var (
	doorKindToProto = map[dungeon.DoorKind]mapsv1.DungeonDoorKind{
		dungeon.DoorArchway: mapsv1.DungeonDoorKind_DUNGEON_DOOR_KIND_ARCHWAY,
		dungeon.DoorClosed:  mapsv1.DungeonDoorKind_DUNGEON_DOOR_KIND_CLOSED,
		dungeon.DoorBarred:  mapsv1.DungeonDoorKind_DUNGEON_DOOR_KIND_BARRED,
		dungeon.DoorLocked:  mapsv1.DungeonDoorKind_DUNGEON_DOOR_KIND_LOCKED,
		dungeon.DoorSecret:  mapsv1.DungeonDoorKind_DUNGEON_DOOR_KIND_SECRET,
	}
	sideToProto = map[dungeon.Side]mapsv1.DungeonSide{
		dungeon.North: mapsv1.DungeonSide_DUNGEON_SIDE_NORTH, dungeon.East: mapsv1.DungeonSide_DUNGEON_SIDE_EAST,
		dungeon.South: mapsv1.DungeonSide_DUNGEON_SIDE_SOUTH, dungeon.West: mapsv1.DungeonSide_DUNGEON_SIDE_WEST,
	}
	axisToProto = map[dungeon.Axis]mapsv1.DungeonAxis{
		dungeon.HorizontalWall: mapsv1.DungeonAxis_DUNGEON_AXIS_HORIZONTAL_WALL,
		dungeon.VerticalWall:   mapsv1.DungeonAxis_DUNGEON_AXIS_VERTICAL_WALL,
	}
	maskFromProto = map[mapsv1.DungeonMask]dungeon.Mask{
		mapsv1.DungeonMask_DUNGEON_MASK_NONE: dungeon.MaskNone, mapsv1.DungeonMask_DUNGEON_MASK_DONUT: dungeon.MaskDonut,
		mapsv1.DungeonMask_DUNGEON_MASK_PLUS: dungeon.MaskPlus, mapsv1.DungeonMask_DUNGEON_MASK_L_SHAPE: dungeon.MaskLShape,
		mapsv1.DungeonMask_DUNGEON_MASK_ELLIPSE: dungeon.MaskEllipse, mapsv1.DungeonMask_DUNGEON_MASK_DIAMOND: dungeon.MaskDiamond,
	}
	placementFromProto = map[mapsv1.DungeonPlacement]dungeon.Placement{
		mapsv1.DungeonPlacement_DUNGEON_PLACEMENT_SPREAD: dungeon.PlacementSpread,
		mapsv1.DungeonPlacement_DUNGEON_PLACEMENT_TILED:  dungeon.PlacementTiled,
	}
	styleFromProto = map[mapsv1.DungeonCorridorStyle]dungeon.CorridorStyle{
		mapsv1.DungeonCorridorStyle_DUNGEON_CORRIDOR_STYLE_TWISTY:     dungeon.StyleTwisty,
		mapsv1.DungeonCorridorStyle_DUNGEON_CORRIDOR_STYLE_MEANDERING: dungeon.StyleMeandering,
		mapsv1.DungeonCorridorStyle_DUNGEON_CORRIDOR_STYLE_LONG_RUNS:  dungeon.StyleLongRuns,
	}
	mixFromProto = map[mapsv1.DungeonDoorMix]dungeon.DoorMix{
		mapsv1.DungeonDoorMix_DUNGEON_DOOR_MIX_OPEN: dungeon.MixOpen, mapsv1.DungeonDoorMix_DUNGEON_DOOR_MIX_TYPICAL: dungeon.MixTypical,
		mapsv1.DungeonDoorMix_DUNGEON_DOOR_MIX_SECURED: dungeon.MixSecured, mapsv1.DungeonDoorMix_DUNGEON_DOOR_MIX_PARANOID: dungeon.MixParanoid,
	}
)

// dungeonOptionsFromProto starts from the generator's defaults and sets what the
// request set. Nothing is checked here: Generate refuses what is out of range,
// naming the option. The seed and the algorithm version are the server's.
func dungeonOptionsFromProto(in *mapsv1.DungeonOptions, seed uint64) (dungeon.Options, error) {
	o := dungeon.DefaultOptions(seed)
	// A request without options is the default dungeon, like an empty message (every
	// option is optional); reading a field of the nil message would panic.
	if in == nil {
		in = &mapsv1.DungeonOptions{}
	}
	set := func(dst *int, v *int32) {
		if v != nil {
			*dst = int(*v)
		}
	}
	set(&o.Width, in.Width)
	set(&o.Height, in.Height)
	set(&o.MaskHole, in.MaskHole)
	set(&o.RoomSideMin, in.RoomSideMin)
	set(&o.RoomSideMax, in.RoomSideMax)
	set(&o.RoomDensity, in.RoomDensity)
	set(&o.DoorDensity, in.DoorDensity)
	set(&o.DeadendRemoval, in.DeadendRemoval)
	set(&o.Stairs, in.Stairs)
	set(&o.ExtraLoops, in.ExtraLoops)
	if in.AspectLimit != nil {
		o.AspectLimit = in.GetAspectLimit()
	}
	// An enum that is not set takes the default; one with a value this server does
	// not know is refused by name, never turned into the default silently.
	if v, ok := maskFromProto[in.GetMask()]; ok {
		o.Mask = v
	} else if in.GetMask() != mapsv1.DungeonMask_DUNGEON_MASK_UNSPECIFIED {
		return o, &dungeon.OptionError{Option: "mask", Reason: "unknown value"}
	}
	if v, ok := placementFromProto[in.GetPlacement()]; ok {
		o.Placement = v
	} else if in.GetPlacement() != mapsv1.DungeonPlacement_DUNGEON_PLACEMENT_UNSPECIFIED {
		return o, &dungeon.OptionError{Option: "placement", Reason: "unknown value"}
	}
	if v, ok := styleFromProto[in.GetCorridorStyle()]; ok {
		o.CorridorStyle = v
	} else if in.GetCorridorStyle() != mapsv1.DungeonCorridorStyle_DUNGEON_CORRIDOR_STYLE_UNSPECIFIED {
		return o, &dungeon.OptionError{Option: "corridor_style", Reason: "unknown value"}
	}
	if v, ok := mixFromProto[in.GetDoorMix()]; ok {
		o.DoorMix = v
	} else if in.GetDoorMix() != mapsv1.DungeonDoorMix_DUNGEON_DOOR_MIX_UNSPECIFIED {
		return o, &dungeon.OptionError{Option: "door_mix", Reason: "unknown value"}
	}
	return o, nil
}

// dungeonOptionsToProto is the options as the generator used them, every field set.
func dungeonOptionsToProto(o dungeon.Options) *mapsv1.DungeonOptions {
	i := func(v int) *int32 { n := int32(v); return &n } //nolint:gosec // G115: every option is a small number
	out := &mapsv1.DungeonOptions{
		Width: i(o.Width), Height: i(o.Height), MaskHole: i(o.MaskHole), RoomSideMin: i(o.RoomSideMin), RoomSideMax: i(o.RoomSideMax),
		AspectLimit: &o.AspectLimit, RoomDensity: i(o.RoomDensity), DoorDensity: i(o.DoorDensity),
		DeadendRemoval: i(o.DeadendRemoval), Stairs: i(o.Stairs), ExtraLoops: i(o.ExtraLoops),
	}
	for k, v := range maskFromProto {
		if v == o.Mask {
			out.Mask = k
		}
	}
	for k, v := range placementFromProto {
		if v == o.Placement {
			out.Placement = k
		}
	}
	for k, v := range styleFromProto {
		if v == o.CorridorStyle {
			out.CorridorStyle = k
		}
	}
	for k, v := range mixFromProto {
		if v == o.DoorMix {
			out.DoorMix = k
		}
	}
	return out
}

// dungeonDoorsProto is every door (passages included) with its true kind.
func dungeonDoorsProto(d *dungeon.Dungeon) []*mapsv1.DungeonDoor {
	out := make([]*mapsv1.DungeonDoor, 0, len(d.Doors))
	for _, dr := range d.Doors {
		out = append(out, &mapsv1.DungeonDoor{
			X: int32(dr.X), Y: int32(dr.Y), Kind: doorKindToProto[dr.Kind], Trapped: dr.Trapped, Axis: axisToProto[dr.Axis], //nolint:gosec // G115: inside the grid
		})
	}
	return out
}

// dungeonStairsProto is every stair, the first one up.
func dungeonStairsProto(d *dungeon.Dungeon) []*mapsv1.DungeonStair {
	out := make([]*mapsv1.DungeonStair, 0, len(d.Stairs))
	for _, st := range d.Stairs {
		out = append(out, &mapsv1.DungeonStair{X: int32(st.X), Y: int32(st.Y), Up: st.Kind == dungeon.StairUp, Facing: sideToProto[st.Facing]}) //nolint:gosec // G115: inside the grid
	}
	return out
}
