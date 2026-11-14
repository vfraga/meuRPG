package maps

import (
	"context"
	"errors"

	"connectrpc.com/connect"
	"github.com/jackc/pgx/v5"

	mapsv1 "github.com/PuraFome/meuRPG/backend/gen/meurpg/maps/v1"
	playv1 "github.com/PuraFome/meuRPG/backend/gen/meurpg/play/v1"
	"github.com/PuraFome/meuRPG/backend/internal/authz"
	"github.com/PuraFome/meuRPG/backend/internal/maps/mapsdb"
	"github.com/PuraFome/meuRPG/backend/internal/platform/sessiontell"
	"github.com/PuraFome/meuRPG/backend/internal/platform/uid"
	"github.com/PuraFome/meuRPG/backend/internal/rules/grid"
)

// The reads and writes of the fog of war that players meet (MR-036, RN-10): the
// player's view of a map (GetMapVision), "Ver como", the master's "Esquecer o
// que foi visto", and the pieces GetMap, ListMaps and GetMapLayers use to filter
// what a player receives. What is seen is worked out in fog.go.

// visionKey is the key under which `vision_changed` hints of a map are
// coalesced in a stream.
func visionKey(mapID string) string { return "vision:" + mapID }

// visionChangedEvent is the hint that tells one player to read the map's view
// again. It carries the map, which the player sees, and nothing of what changed.
// It is also sent to the master, whose light warning reads the same scene.
func visionChangedEvent(mapID string) *playv1.WatchGameSessionResponse {
	return &playv1.WatchGameSessionResponse{Event: &playv1.WatchGameSessionResponse_VisionChanged_{
		VisionChanged: &playv1.WatchGameSessionResponse_VisionChanged{MapId: mapID},
	}}
}

// readerOf is the viewer of a read: the caller, or, for the master who sends
// as_character_id, the player of that character ("Ver como"): the same rules the
// player gets, on any map. A player sending it is refused, before anything else
// is looked up.
func (s *Service) readerOf(ctx context.Context, m authz.Membership, asCharacterID string) (viewer, error) {
	v, err := s.viewerOf(ctx, m)
	if err != nil {
		return viewer{}, err
	}
	if asCharacterID == "" {
		return v, nil
	}
	if !v.master {
		return viewer{}, connect.NewError(connect.CodePermissionDenied, errors.New("only the master may read as a character"))
	}
	id, ok := uid.Canonical(asCharacterID)
	if !ok {
		return viewer{}, errCharacterNotFound()
	}
	party, err := s.characters.PartyVision(ctx, nil, m.CampaignID)
	if err != nil {
		return viewer{}, s.dbError(ctx, "read the party", err)
	}
	for _, member := range party {
		if member.CharacterID != id || member.Dead { // a dead character is no one to look as
			continue
		}
		as := viewer{userID: member.UserID, currentMap: v.currentMap, preview: true}
		if as.traps, err = s.knownTraps(ctx, m.CampaignID, member.UserID); err != nil {
			return viewer{}, s.dbError(ctx, "read the traps a player knows", err)
		}
		return as, nil
	}
	return viewer{}, errCharacterNotFound()
}

// foggedFor says whether what the viewer receives of the map is filtered by
// their view: the fog is on and they are a player (or the master "Ver como").
func foggedFor(v viewer, r mapsdb.ListMapDetailsRow) bool {
	return !v.master && r.FogEnabled && r.GridColumns != nil
}

// parentKnows is the viewer's test for a submap point of a parent map: on a map
// with the fog on, the parent is named only when the point is on a square the
// player sees or remembers. The views are worked out once for each parent.
func (s *Service) parentKnows(ctx context.Context, cm campaignMaps, v viewer) func(mapsdb.ListSubmapLinksRow) bool {
	views := map[string]*playerView{}
	return func(l mapsdb.ListSubmapLinksRow) bool {
		parent, ok := cm.byID[l.MapID]
		if !ok || !foggedFor(v, parent) {
			return true
		}
		pv, done := views[parent.ID]
		if !done {
			var err error
			if pv, _, _, err = s.fogViewOf(ctx, parent, v); err != nil {
				s.logger.ErrorContext(ctx, "maps: cannot work out what a player sees of a parent map", "error", err)
				pv = nil // the parent stays unnamed
			}
			views[parent.ID] = pv
		}
		return pv != nil && pv.known(pv.g.SquareOf(int(l.XBp), int(l.YBp)))
	}
}

// visiblePoints is the points a player receives of a map: the ones they would
// without the fog (RN-10, and the traps, treasures and lights rules), and, on a fog
// map, only those with a square they see now or remember.
func (s *Service) visiblePoints(points []mapsdb.MapPoint, v viewer, pv *playerView) []mapsdb.MapPoint {
	var out []mapsdb.MapPoint
	for _, p := range points {
		if v.seesPoint(p) && (pv == nil || s.pointKnown(pv, p)) {
			out = append(out, p)
		}
	}
	return out
}

// fogViewOf reads what the viewer knows of a fog map: loads its points and
// tokens, and builds the scene.
func (s *Service) fogViewOf(ctx context.Context, r mapsdb.ListMapDetailsRow, v viewer) (*playerView, []mapsdb.MapPoint, []mapsdb.MapToken, error) {
	points, err := s.queries.ListMapPoints(ctx, r.ID)
	if err != nil {
		return nil, nil, nil, err
	}
	tokens, err := s.queries.ListMapTokens(ctx, r.ID)
	if err != nil {
		return nil, nil, nil, err
	}
	pv, err := s.playerViewOf(ctx, fogInputOfDetails(r), points, tokens, v.userID)
	if err != nil {
		return nil, nil, nil, err
	}
	return pv, points, tokens, nil
}

// filterLayers is a player's copy of the layers: the walls, the difficult terrain,
// the cover, the doors, the squares nobody passes and the surfaces, of the squares they see now or remember (a wall next to
// a seen square is one of them). The doors are as a player knows them: a locked
// door reads closed, and a secret one is a wall there and no door (RN-26). The
// light is never theirs. With the base light "Claro" it is every square's (the
// whole floor plan); creatures and points do not come from here.
func filterLayers(set layerSet, pv *playerView) layerSet {
	out := layerSet{terrain: grid.NewLayer(pv.g), walls: grid.NewLayer(pv.g), cover: grid.NewCoverLayer(pv.g), doors: grid.NewDoorLayer(pv.g), noPass: grid.NewLayer(pv.g), surface: grid.NewSurfaceLayer(pv.g)}
	for row := range pv.g.Rows {
		for col := range pv.g.Columns {
			if !pv.terrainKnown(grid.Square{Col: col, Row: row}) {
				continue
			}
			out.terrain.Set(col, row, set.terrain.Get(col, row))
			door, secretWall := set.doors.Get(col, row).AsPlayerKnows()
			out.walls.Set(col, row, set.walls.Get(col, row) || secretWall)
			out.cover.Set(col, row, set.cover.Get(col, row))
			out.doors.Set(col, row, door)
			out.noPass.Set(col, row, set.noPass.Get(col, row))
			out.surface.Set(col, row, set.surface.Get(col, row))
		}
	}
	return out
}

// GetMapVision implements mapsv1connect.MapServiceHandler.
func (s *Service) GetMapVision(
	ctx context.Context,
	req *connect.Request[mapsv1.GetMapVisionRequest],
) (*connect.Response[mapsv1.GetMapVisionResponse], error) {
	m, err := authz.RequireCampaignMember(ctx, req.Msg.GetCampaignId())
	if err != nil {
		return nil, err
	}
	mapID, ok := uid.Canonical(req.Msg.GetMapId())
	if !ok {
		return nil, errMapNotFound()
	}
	gen := s.tiles.views.generation(mapID)
	v, row, pv, err := s.visionOf(ctx, m, mapID, req.Msg.GetAsCharacterId())
	if err != nil {
		return nil, err
	}
	return connect.NewResponse(s.visionResponse(mapID, gen, tileViewerOf(v, row.CampaignID, req.Msg.GetAsCharacterId()), v, row, pv)), nil
}

// visionResponse is the view of the squares the viewer receives: the states, and on a fog map the tiles. The
// tiles are kept under tileViewer, so the tile requests that follow need no view of their own. It is the one
// code path of GetMapVision and of the table screen's vision.
func (s *Service) visionResponse(mapID string, gen uint64, tileViewer string, v viewer, row mapsdb.ListMapDetailsRow, pv *playerView) *mapsv1.GetMapVisionResponse {
	out := &mapsv1.GetMapVisionResponse{
		GridColumns: int32(pv.g.Columns), GridRows: int32(pv.g.Rows), //nolint:gosec // G115: a grid is at most 200 x 400
		States: pv.pack(), Revision: pv.revision(), CharacterOnMap: pv.onMap,
		FogEnabled: row.FogEnabled, GroupVision: row.GroupVision,
	}
	if foggedFor(v, row) {
		// A read never marks the view as told (s.seen): only refreshVision, which
		// sends the hint, does. The page reads its view as soon as `token_moved`
		// arrives, and that read can come before the move's refresh; had it counted,
		// the refresh would find nothing new and send no `vision_changed`, and the
		// page, which reads the tokens and points again only on that hint, would keep
		// showing a token that went out of sight (RN-10).
		if s.blobs != nil {
			src := tileSourceOf(row.ID, row.CampaignID, row.ImageID, row.ImageContentType, row.GridColumns, row.GridFactor, row.ImageWidth, row.ImageHeight)
			vt := buildTiles(pv, src)
			s.tiles.views.put(mapID, tileViewer, gen, vt)
			out.TilesPath, out.TileSquares, out.Tiles = TilesPath+mapID+"/tiles/", tileSquares, vt.proto()
		}
	}
	return out
}

// visionOf reads what the caller may know of a map's squares: the viewer, the
// map's row and the view. The master (and a map without the fog) sees every
// square; a player, or the master "Ver como" a character, only what that player
// sees or remembers. A hidden map is not found to a player (RN-10).
func (s *Service) visionOf(ctx context.Context, m authz.Membership, mapID, asCharacterID string) (viewer, mapsdb.ListMapDetailsRow, *playerView, error) {
	v, err := s.readerOf(ctx, m, asCharacterID)
	if err != nil {
		return viewer{}, mapsdb.ListMapDetailsRow{}, nil, err
	}
	row, pv, err := s.visionFor(ctx, m.CampaignID, mapID, v)
	return v, row, pv, err
}

// visionFor is visionOf for a viewer already made.
func (s *Service) visionFor(ctx context.Context, campaignID, mapID string, v viewer) (mapsdb.ListMapDetailsRow, *playerView, error) {
	cm, err := s.loadMaps(ctx, campaignID)
	if err != nil {
		return mapsdb.ListMapDetailsRow{}, nil, s.dbError(ctx, "read a map", err)
	}
	row, ok := cm.byID[mapID]
	if !ok || !v.seesMap(row.ID, row.RevealedAt) {
		return row, nil, errMapNotFound() // a hidden map is not found to a player (RN-10)
	}
	g := gridOf(row.GridColumns, row.GridFactor, row.ImageWidth, row.ImageHeight)
	if !g.Valid() {
		return row, nil, errNoGrid()
	}
	pv := everything(g) // the master, and a map without the fog: every square seen
	if foggedFor(v, row) {
		if pv, _, _, err = s.fogViewOf(ctx, row, v); err != nil {
			return row, nil, s.dbError(ctx, "work out what a player sees", err)
		}
	}
	return row, pv, nil
}

// ForgetMapVision implements mapsv1connect.MapServiceHandler.
func (s *Service) ForgetMapVision(
	ctx context.Context,
	req *connect.Request[mapsv1.ForgetMapVisionRequest],
) (*connect.Response[mapsv1.ForgetMapVisionResponse], error) {
	m, err := authz.RequireCampaignRole(ctx, req.Msg.GetCampaignId(), authz.RoleMaster)
	if err != nil {
		return nil, err
	}
	mapID, ok := uid.Canonical(req.Msg.GetMapId())
	if !ok {
		return nil, errMapNotFound()
	}
	// The epoch goes up with the clear, so a refresh that is already running cannot
	// write the old memory back (rememberFor).
	err = s.inSession(ctx, m.CampaignID, func(tx pgx.Tx, t sessiontell.Tell) error {
		q := s.queries.WithTx(tx)
		if _, err := s.campaignMap(ctx, q, m.CampaignID, mapID); err != nil {
			return err
		}
		if err := clearVisionMemory(ctx, q, mapID); err != nil {
			return err
		}
		s.announce(t, func(ctx context.Context) {
			// What the players see now is theirs again, from this moment.
			s.tiles.forget(mapID) // every player's mask changes: the tiles start over
			s.refreshVision(ctx, m.CampaignID, mapID)
		})
		return nil
	})
	if err != nil {
		return nil, s.dbError(ctx, "forget what the players saw", err)
	}
	return connect.NewResponse(&mapsv1.ForgetMapVisionResponse{}), nil
}
