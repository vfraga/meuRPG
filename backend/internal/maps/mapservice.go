package maps

import (
	"context"
	"errors"
	"fmt"
	"time"
	"uuid"

	"connectrpc.com/connect"
	"github.com/jackc/pgx/v5"
	"google.golang.org/protobuf/proto"

	charactersv1 "github.com/PuraFome/meuRPG/backend/gen/meurpg/characters/v1"
	mapsv1 "github.com/PuraFome/meuRPG/backend/gen/meurpg/maps/v1"
	"github.com/PuraFome/meuRPG/backend/internal/authz"
	"github.com/PuraFome/meuRPG/backend/internal/maps/mapsdb"
	"github.com/PuraFome/meuRPG/backend/internal/platform/db"
	"github.com/PuraFome/meuRPG/backend/internal/platform/idem"
	"github.com/PuraFome/meuRPG/backend/internal/platform/names"
	"github.com/PuraFome/meuRPG/backend/internal/rules/grid"
)

// MapService (MR-008, MR-009, MR-012). Every handler starts with one
// explicit check: authz.RequireCampaignMember for the reads, which any
// member may call and which filter what a player sees (visibility.go), and
// authz.RequireCampaignRole(master) for everything that changes a map. The
// check's error is already the right Connect error.
//
// A write reads the session's current map first (it decides whether the
// players see the map), changes the rows in one transaction, and only after
// the commit tells the watching members (visibility.go, "The live events").

// Limits that keep the unpaginated lists small (a proposal, like the
// gallery's quota). Tests set smaller ones through Config.
const (
	// DefaultMaxMaps is how many maps a campaign may have.
	DefaultMaxMaps = 200
	// DefaultMaxPointsPerMap is how many points of interest a map may have.
	DefaultMaxPointsPerMap = 200
)

// maxDescriptionLength is the longest point description, in characters
// (map_points_description_length).
const maxDescriptionLength = 2000

// maxHooksLength is the longest "Ganchos e anotações" of a scene point, in
// characters (map_points_hooks_length).
const maxHooksLength = 4000

// maxPosition is the largest position, in basis points of the image's width
// or height (map_points_position_valid, map_tokens_position_valid).
const maxPosition = 10000

// The database's point kinds (map_points_kind_valid) and the API's.
var (
	kindToDB = map[mapsv1.MapPointKind]string{
		mapsv1.MapPointKind_MAP_POINT_KIND_BATTLE:   "battle",
		mapsv1.MapPointKind_MAP_POINT_KIND_SUBMAP:   "submap",
		mapsv1.MapPointKind_MAP_POINT_KIND_SCENE:    "scene",
		mapsv1.MapPointKind_MAP_POINT_KIND_TRAP:     "trap",
		mapsv1.MapPointKind_MAP_POINT_KIND_TREASURE: "treasure",
		mapsv1.MapPointKind_MAP_POINT_KIND_LIGHT:    "light",
	}
	kindFromDB = map[string]mapsv1.MapPointKind{
		"battle":   mapsv1.MapPointKind_MAP_POINT_KIND_BATTLE,
		"submap":   mapsv1.MapPointKind_MAP_POINT_KIND_SUBMAP,
		"scene":    mapsv1.MapPointKind_MAP_POINT_KIND_SCENE,
		"trap":     mapsv1.MapPointKind_MAP_POINT_KIND_TRAP,
		"treasure": mapsv1.MapPointKind_MAP_POINT_KIND_TREASURE,
		"light":    mapsv1.MapPointKind_MAP_POINT_KIND_LIGHT,
	}
)

// ListMaps implements mapsv1connect.MapServiceHandler.
func (s *Service) ListMaps(
	ctx context.Context,
	req *connect.Request[mapsv1.ListMapsRequest],
) (*connect.Response[mapsv1.ListMapsResponse], error) {
	m, err := authz.RequireCampaignMember(ctx, req.Msg.GetCampaignId())
	if err != nil {
		return nil, err
	}
	v, err := s.viewerOf(ctx, m)
	if err != nil {
		return nil, err
	}
	cm, err := s.loadMaps(ctx, m.CampaignID)
	if err != nil {
		return nil, s.dbError(ctx, "list maps", err)
	}
	if !v.master {
		v.parentKnows = s.parentKnows(ctx, cm, v)
	}
	ctx = withPartyMemo(ctx) // the party's senses are read once for all the fog maps
	res := &mapsv1.ListMapsResponse{}
	for _, r := range cm.rows {
		if !v.seesMap(r.ID, r.RevealedAt) {
			continue
		}
		out := cm.mapToProto(r, v)
		if foggedFor(v, r) {
			// What a player counts on a fog map is what they receive of it (RN-10).
			pv, points, _, err := s.fogViewOf(ctx, r, v)
			if err != nil {
				return nil, s.dbError(ctx, "work out what a player sees", err)
			}
			key := countKey{mapID: r.ID, userID: v.userID, revision: pv.revision(), points: pointsSig(points), traps: trapsSig(v.traps)}
			n, ok := s.counts.get(key)
			if !ok {
				n = int32(len(s.visiblePoints(points, v, pv))) //nolint:gosec // G115: a map has at most 200 points
				s.counts.put(key, n)
			}
			out.PointCount = n
		}
		res.Maps = append(res.Maps, out)
	}
	return connect.NewResponse(res), nil
}

// GetMap implements mapsv1connect.MapServiceHandler.
func (s *Service) GetMap(
	ctx context.Context,
	req *connect.Request[mapsv1.GetMapRequest],
) (*connect.Response[mapsv1.GetMapResponse], error) {
	m, err := authz.RequireCampaignMember(ctx, req.Msg.GetCampaignId())
	if err != nil {
		return nil, err
	}
	mapID, ok := parseID(req.Msg.GetMapId())
	if !ok {
		return nil, errMapNotFound()
	}
	v, err := s.readerOf(ctx, m, req.Msg.GetAsCharacterId())
	if err != nil {
		return nil, err
	}
	// The map's rows, its points and its tokens are one snapshot: three
	// autocommit reads could show a token moved on a revision that does not
	// count the move (audit D-03). A hidden map is "not found" to a player
	// exactly as a map that does not exist (RN-10), and its points are not read.
	var (
		cm     campaignMaps
		row    mapsdb.ListMapDetailsRow
		points []mapsdb.MapPoint
		tokens []mapsdb.MapToken
	)
	err = db.ReadTx(ctx, s.pool, func(tx pgx.Tx) error {
		q := s.queries.WithTx(tx)
		var err error
		if cm, err = loadMapsWith(ctx, q, m.CampaignID); err != nil {
			return err
		}
		var ok bool
		if row, ok = cm.byID[mapID]; !ok || !v.seesMap(row.ID, row.RevealedAt) {
			return errMapNotFound()
		}
		if points, err = q.ListMapPoints(ctx, mapID); err != nil {
			return fmt.Errorf("list a map's points: %w", err)
		}
		if tokens, err = q.ListMapTokens(ctx, mapID); err != nil {
			return fmt.Errorf("list a map's tokens: %w", err)
		}
		return nil
	})
	if err != nil {
		return nil, s.dbError(ctx, "get a map", err)
	}
	if !v.master {
		v.parentKnows = s.parentKnows(ctx, cm, v)
	}
	res := &mapsv1.GetMapResponse{Map: cm.mapToProto(row, v)}

	// On a fog map a player receives only what their character sees now or
	// remembers (RN-10): pv says which squares those are.
	var pv *playerView
	if foggedFor(v, row) {
		if pv, err = s.playerViewOf(ctx, fogInputOfDetails(row), points, tokens, v.userID); err != nil {
			return nil, s.dbError(ctx, "work out what a player sees", err)
		}
	}
	for _, p := range s.visiblePoints(points, v, pv) {
		out := s.pointToProto(cm, p, v)
		out.Remembered = pv != nil && !s.pointSeenNow(pv, p)
		res.Points = append(res.Points, out)
	}
	if pv != nil {
		res.Map.PointCount = int32(len(res.Points)) //nolint:gosec // G115: a map has at most 200 points
	}
	if err := s.attachPointDetails(ctx, m.CampaignID, mapID, res.Points, v.master); err != nil {
		return nil, s.dbError(ctx, "list a map's traps and treasures", err)
	}
	if err := s.attachActions(ctx, mapID, res.Points, v.master); err != nil {
		return nil, s.dbError(ctx, "list a map's scene actions", err)
	}
	if err := s.attachClues(ctx, m.CampaignID, mapID, res.Points, v.master); err != nil {
		return nil, s.dbError(ctx, "list a map's scene clues", err)
	}

	byCharacter := make(map[string]mapsdb.MapToken, len(tokens))
	ids := make([]string, 0, len(tokens))
	for _, t := range tokens {
		// On a fog map the kind decides (a player character is always shown), so
		// the characters are asked for before the tokens are filtered.
		if pv != nil || v.seesToken(t) {
			byCharacter[t.CharacterID] = t
			ids = append(ids, t.CharacterID)
		}
	}
	// The characters come back in their own order (players' first), and
	// only the living ones: a dead character's token stays on the map, but
	// is not listed.
	characters, err := s.characters.MapCharacters(ctx, nil, m.CampaignID, ids)
	if err != nil {
		return nil, s.dbError(ctx, "read the characters on a map", err)
	}
	for _, c := range characters {
		t, ok := byCharacter[c.GetId()]
		if !ok {
			continue
		}
		if pv != nil {
			player := c.GetKind() == charactersv1.CharacterKind_CHARACTER_KIND_PLAYER
			if !tokenSeen(pv, t, player) || (!player && t.Hidden) {
				continue
			}
			t.Hidden = false // a player's character is never hidden from the party (D6)
		}
		res.Tokens = append(res.Tokens, tokenToProto(t, c, v))
	}
	// The creatures' tokens come after the characters': party tokens, never kept
	// from a player, fog or not (MR-037).
	creatureTokens, err := s.creatureTokensOf(ctx, m.CampaignID, mapID, v)
	if err != nil {
		return nil, s.dbError(ctx, "list a map's creature tokens", err)
	}
	res.Tokens = append(res.Tokens, creatureTokens...)
	return connect.NewResponse(res), nil
}

// CreateMap implements mapsv1connect.MapServiceHandler.
func (s *Service) CreateMap(
	ctx context.Context,
	req *connect.Request[mapsv1.CreateMapRequest],
) (*connect.Response[mapsv1.CreateMapResponse], error) {
	m, err := authz.RequireCampaignRole(ctx, req.Msg.GetCampaignId(), authz.RoleMaster)
	if err != nil {
		return nil, err
	}
	name, err := cleanName("name", req.Msg.GetName())
	if err != nil {
		return nil, err
	}
	imageID, ok := parseID(req.Msg.GetImageId())
	if !ok {
		return nil, errNotAGalleryImage()
	}

	// A new map has no grid, so it cannot have fog yet: when the table's rules want
	// the fog on new maps (RN-24; none by default), the map is marked, and the fog
	// comes on with its first grid (SetMapGrid).
	fogOn := false
	if s.defaults != nil {
		if fogOn, err = s.defaults.FogOnNewMaps(ctx, nil, m.CampaignID); err != nil {
			return nil, s.dbError(ctx, "read the table's rules", err)
		}
	}
	reuse, err := s.prepareReuseCopy(ctx, m.CampaignID, imageID)
	if err != nil {
		return nil, s.dbError(ctx, "copy the image of a fog map", err)
	}
	// The key is unique in the campaign, and kept with a hash of the whole request: a retry
	// returns the first map only when it is the same request.
	scopedKey, requestHash, err := keyOf(m.CampaignID, req.Msg.GetIdempotencyKey(), req.Msg)
	if err != nil {
		return nil, err
	}
	var created mapsdb.Map
	used, replayed := false, false
	err = db.InTx(ctx, s.pool, func(tx pgx.Tx) error {
		q := s.queries.WithTx(tx)
		used = false
		var err error
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
				// The map's image is local to the run: the copy's gallery row exists
				// only inside this attempt, and a retry checks the original again.
				mapImageID := imageID
				if err := checkImage(ctx, q, m.CampaignID, mapImageID); err != nil {
					return created, err
				}
				if reuse != nil {
					if err := reuse.insert(ctx, q, s, m.CampaignID, m.UserID); err != nil {
						return created, err
					}
					mapImageID, used = reuse.id, true
				}
				row, err := q.InsertMap(ctx, mapsdb.InsertMapParams{
					CampaignID: m.CampaignID, Name: name, ImageID: mapImageID, FogOnFirstGrid: fogOn,
					CreateKey: scopedKey, CreateHash: requestHash, Now: s.now(),
				})
				if err != nil && !errors.Is(err, pgx.ErrNoRows) {
					return row, fmt.Errorf("insert map: %w", err)
				}
				return row, err
			})
		return err
	})
	if reuse != nil && (err != nil || !used) {
		s.deleteFiles(ctx, m.CampaignID, reuse.id)
	}
	if err != nil {
		return nil, s.dbError(ctx, "create a map", err)
	}
	// A new map is hidden: only the master hears about it. A retry announces nothing: the
	// first call did.
	if !replayed {
		s.publishMapChanged(m.CampaignID, created.ID, false)
	}
	out, err := s.masterMap(ctx, m, created.ID)
	if err != nil {
		return nil, err
	}
	return connect.NewResponse(&mapsv1.CreateMapResponse{Map: out}), nil
}

// UpdateMap implements mapsv1connect.MapServiceHandler.
func (s *Service) UpdateMap(
	ctx context.Context,
	req *connect.Request[mapsv1.UpdateMapRequest],
) (*connect.Response[mapsv1.UpdateMapResponse], error) {
	m, err := authz.RequireCampaignRole(ctx, req.Msg.GetCampaignId(), authz.RoleMaster)
	if err != nil {
		return nil, err
	}
	mapID, ok := parseID(req.Msg.GetMapId())
	if !ok {
		return nil, errMapNotFound()
	}
	if req.Msg.Name == nil && req.Msg.ImageId == nil {
		return nil, connect.NewError(connect.CodeInvalidArgument, errors.New("set name, image_id or both"))
	}
	if req.Msg.GetRevision() < 1 {
		return nil, connect.NewError(connect.CodeInvalidArgument, errors.New("revision must be at least 1"))
	}
	var name, imageID *string
	if req.Msg.Name != nil {
		clean, err := cleanName("name", req.Msg.GetName())
		if err != nil {
			return nil, err
		}
		name = &clean
	}
	if req.Msg.ImageId != nil {
		id, ok := parseID(req.Msg.GetImageId())
		if !ok {
			return nil, errNotAGalleryImage()
		}
		imageID = &id
	}
	current, err := s.currentMap(ctx, m.CampaignID)
	if err != nil {
		return nil, err
	}
	// A map with the fog on never has an image a player can fetch (RN-10): a new
	// image that is also used another way is copied, and the map gets the copy.
	var imageCopy *fogCopy
	if imageID != nil {
		if old, err := s.queries.GetMap(ctx, mapsdb.GetMapParams{CampaignID: m.CampaignID, ID: mapID}); err == nil && *imageID != old.ImageID {
			if old.FogEnabled {
				imageCopy, err = s.prepareFogCopy(ctx, m.CampaignID, mapID, *imageID)
			} else {
				// A map without fog never takes a fog map's own image either.
				imageCopy, err = s.prepareReuseCopy(ctx, m.CampaignID, *imageID)
			}
			if err != nil {
				return nil, s.dbError(ctx, "copy the map's image", err)
			}
		}
	}
	var updated mapsdb.Map
	cleared, copied := false, false
	err = db.InTx(ctx, s.pool, func(tx pgx.Tx) error {
		q := s.queries.WithTx(tx)
		cleared, copied = false, false
		row, err := q.GetMapForUpdate(ctx, mapsdb.GetMapForUpdateParams{CampaignID: m.CampaignID, ID: mapID})
		if errors.Is(err, pgx.ErrNoRows) {
			return errMapNotFound()
		}
		if err != nil {
			return fmt.Errorf("find map: %w", err)
		}
		if row.Revision != req.Msg.GetRevision() {
			return errStaleMap()
		}
		params := mapsdb.UpdateMapParams{CampaignID: m.CampaignID, ID: mapID, Name: row.Name, ImageID: row.ImageID, Revision: row.Revision, Now: s.now()}
		if name != nil {
			params.Name = *name
		}
		if imageID != nil {
			img, err := q.GetGalleryImageInCampaign(ctx, mapsdb.GetGalleryImageInCampaignParams{CampaignID: m.CampaignID, ID: *imageID})
			if errors.Is(err, pgx.ErrNoRows) {
				return errNotAGalleryImage()
			}
			if err != nil {
				return fmt.Errorf("find image: %w", err)
			}
			// The rows follow the image's proportions: a taller image on a calibrated
			// map could pass the rules' 400 rows (MR-025), which no layer, combat or
			// CHECK accepts. The master picks another image or recalibrates first.
			if row.GridColumns != nil && *imageID != row.ImageID {
				if _, err := grid.EngineGrid(int(*row.GridColumns/row.GridFactor), int(row.GridFactor), int(img.Width), int(img.Height)); err != nil {
					return connect.NewError(connect.CodeInvalidArgument, fmt.Errorf(
						"this image would make the map's grid pass the limit of %d x %d squares: lower the calibration first", grid.MaxColumns, grid.MaxRows))
				}
			}
			params.ImageID = *imageID
			if imageCopy != nil && *imageID == imageCopy.source.ID {
				if err := imageCopy.insert(ctx, q, s, m.CampaignID, m.UserID); err != nil {
					return err
				}
				params.ImageID, copied = imageCopy.id, true
			}
		}
		// A new image clears the painted layers (D2), which a fight standing on
		// them cannot lose; the same image again clears nothing. Decided from the
		// row locked here, so a combat that starts meanwhile is seen.
		imageChanged := params.ImageID != row.ImageID
		if imageChanged {
			if err := s.refuseWhileCombat(ctx, tx, m.CampaignID, mapID); err != nil {
				return err
			}
		}
		updated, err = q.UpdateMap(ctx, params)
		if errors.Is(err, pgx.ErrNoRows) {
			return errStaleMap() // never, under the lock above: a second guard
		}
		if err != nil {
			return fmt.Errorf("update map: %w", err)
		}
		if imageChanged {
			cleared = true
			return clearLayers(ctx, q, mapID)
		}
		return nil
	})
	if imageCopy != nil && (err != nil || !copied) {
		s.deleteFiles(ctx, m.CampaignID, imageCopy.id) // made for nothing
	}
	if err != nil {
		return nil, s.dbError(ctx, "update a map", err)
	}
	s.publishMapChanged(m.CampaignID, mapID, playersSee(mapID, updated.RevealedAt, current))
	if cleared {
		s.tiles.forget(mapID)
		s.refreshVision(ctx, m.CampaignID, mapID) // the players' memory went with the layers
	}
	out, err := s.masterMap(ctx, m, mapID)
	if err != nil {
		return nil, err
	}
	return connect.NewResponse(&mapsv1.UpdateMapResponse{Map: out}), nil
}

// DeleteMap implements mapsv1connect.MapServiceHandler.
func (s *Service) DeleteMap(
	ctx context.Context,
	req *connect.Request[mapsv1.DeleteMapRequest],
) (*connect.Response[mapsv1.DeleteMapResponse], error) {
	m, err := authz.RequireCampaignRole(ctx, req.Msg.GetCampaignId(), authz.RoleMaster)
	if err != nil {
		return nil, err
	}
	mapID, ok := parseID(req.Msg.GetMapId())
	if !ok {
		return nil, errMapNotFound()
	}
	current, err := s.currentMap(ctx, m.CampaignID)
	if err != nil {
		return nil, err
	}
	// The links to this map, before they go, to tell the maps they leave.
	before, err := s.loadMaps(ctx, m.CampaignID)
	if err != nil {
		return nil, s.dbError(ctx, "delete a map", err)
	}
	openScene, err := s.live.OpenScenePoint(ctx, m.CampaignID) // a point of this map may be the open scene
	if err != nil {
		return nil, s.dbError(ctx, "read the open scene", err)
	}

	var deleted mapsdb.Map
	err = db.InTx(ctx, s.pool, func(tx pgx.Tx) error {
		q := s.queries.WithTx(tx)
		if _, err := s.campaignMap(ctx, q, m.CampaignID, mapID); err != nil {
			return err
		}
		// The map a combat that has not ended runs on cannot go: the fight stands on
		// its layers, and the fog of war's filter on the combat's map link.
		if err := s.refuseWhileCombat(ctx, tx, m.CampaignID, mapID); err != nil {
			return err
		}
		// A map holding a treasure that was found or turned into XP cannot go:
		// the treasure is part of the session's record (unmark it first).
		locks, err := q.GetMapTreasureLocks(ctx, mapID)
		if err != nil {
			return fmt.Errorf("read the map's treasures: %w", err)
		}
		if locks.Converted > 0 {
			return errTreasureConverted()
		}
		if locks.Found > 0 {
			return errTreasureFound()
		}
		deleted, err = q.DeleteMap(ctx, mapsdb.DeleteMapParams{CampaignID: m.CampaignID, ID: mapID})
		if errors.Is(err, pgx.ErrNoRows) {
			return errMapNotFound()
		}
		if err != nil {
			return fmt.Errorf("delete map: %w", err)
		}
		return nil
	})
	if err != nil {
		return nil, s.dbError(ctx, "delete a map", err)
	}
	s.lits.forget(mapID)
	s.tiles.forget(mapID)
	s.seen.forget(mapID)
	seen := playersSee(mapID, deleted.RevealedAt, current)
	s.publishMapChanged(m.CampaignID, mapID, seen)
	s.publishParentsChanged(m.CampaignID, mapID, before, current, seen, false)
	if mapID == current {
		// The foreign key unset the session's current map.
		s.publishCurrentMapCleared(m.CampaignID)
	}
	if openScene != "" {
		// The map's points went with it: if the scene was one of them, the
		// foreign key closed it.
		if still, err := s.live.OpenScenePoint(ctx, m.CampaignID); err == nil && still == "" {
			s.publishSceneChanged(m.CampaignID)
		}
	}
	return connect.NewResponse(&mapsv1.DeleteMapResponse{}), nil
}

// SetMapRevealed implements mapsv1connect.MapServiceHandler.
func (s *Service) SetMapRevealed(
	ctx context.Context,
	req *connect.Request[mapsv1.SetMapRevealedRequest],
) (*connect.Response[mapsv1.SetMapRevealedResponse], error) {
	m, err := authz.RequireCampaignRole(ctx, req.Msg.GetCampaignId(), authz.RoleMaster)
	if err != nil {
		return nil, err
	}
	mapID, ok := parseID(req.Msg.GetMapId())
	if !ok {
		return nil, errMapNotFound()
	}
	current, err := s.currentMap(ctx, m.CampaignID)
	if err != nil {
		return nil, err
	}

	var before, after mapsdb.Map
	err = db.InTx(ctx, s.pool, func(tx pgx.Tx) error {
		q := s.queries.WithTx(tx)
		var err error
		before, err = q.GetMapForUpdate(ctx, mapsdb.GetMapForUpdateParams{CampaignID: m.CampaignID, ID: mapID})
		if errors.Is(err, pgx.ErrNoRows) {
			return errMapNotFound()
		}
		if err != nil {
			return fmt.Errorf("find map: %w", err)
		}
		after, err = q.SetMapRevealed(ctx, mapsdb.SetMapRevealedParams{CampaignID: m.CampaignID, ID: mapID, Revealed: req.Msg.GetRevealed(), Now: s.now()})
		if err != nil {
			return fmt.Errorf("set map revealed: %w", err)
		}
		return nil
	})
	if err != nil {
		return nil, s.dbError(ctx, "reveal or hide a map", err)
	}
	seenBefore, seenAfter := playersSee(mapID, before.RevealedAt, current), playersSee(mapID, after.RevealedAt, current)
	s.publishMapChanged(m.CampaignID, mapID, seenBefore || seenAfter)
	if seenAfter && !seenBefore {
		s.refreshVision(ctx, m.CampaignID, mapID) // the first view the players have of a fog map is recorded now
	}
	if cm, err := s.loadMaps(ctx, m.CampaignID); err == nil {
		s.publishParentsChanged(m.CampaignID, mapID, cm, current, seenBefore, seenAfter)
	} else {
		// The change is made; only a hint to other maps is lost, and the
		// app reads the maps again after any reconnection.
		s.logger.ErrorContext(ctx, "maps: cannot tell the parent maps about a revealed map", "error", err)
	}
	out, err := s.masterMap(ctx, m, mapID)
	if err != nil {
		return nil, err
	}
	return connect.NewResponse(&mapsv1.SetMapRevealedResponse{Map: out}), nil
}

// The battle grid's limits (MR-013): how many squares of the drawing fit across
// the image's width. The maps_grid_columns_valid CHECK says the same of the
// rules' columns, which are at most 200 as well (the calibration multiplies the
// drawn columns, and rules/grid.EngineGrid refuses what passes the limits).
const (
	minGridColumns = 4
	maxGridColumns = 200
)

// SetMapGrid implements mapsv1connect.MapServiceHandler. calibration.go says
// what it does to the painted layers.
func (s *Service) SetMapGrid(
	ctx context.Context,
	req *connect.Request[mapsv1.SetMapGridRequest],
) (*connect.Response[mapsv1.SetMapGridResponse], error) {
	m, err := authz.RequireCampaignRole(ctx, req.Msg.GetCampaignId(), authz.RoleMaster)
	if err != nil {
		return nil, err
	}
	mapID, ok := parseID(req.Msg.GetMapId())
	if !ok {
		return nil, errMapNotFound()
	}
	drawn, factor := req.Msg.GetColumns(), req.Msg.GetSquareFactor()
	if drawn != 0 && (drawn < minGridColumns || drawn > maxGridColumns) {
		return nil, connect.NewError(connect.CodeInvalidArgument,
			fmt.Errorf("columns must be 0 (no grid) or %d to %d", minGridColumns, maxGridColumns))
	}
	if factor < 0 || factor > grid.MaxFactor || (drawn == 0 && factor > 1) {
		return nil, connect.NewError(connect.CodeInvalidArgument,
			fmt.Errorf("square_factor must be 1 to %d (0 reads as 1), and only with columns", grid.MaxFactor))
	}
	factor = max(factor, 1)
	current, err := s.currentMap(ctx, m.CampaignID)
	if err != nil {
		return nil, err
	}
	// The table's fog rule meets the map's first grid (RN-24): the fog comes on, and
	// the raw image never reaches the players (RN-10), so an image that serves
	// another use is copied first, as SetMapFog does.
	var imageCopy *fogCopy
	if drawn != 0 {
		if pre, err := s.queries.GetMap(ctx, mapsdb.GetMapParams{CampaignID: m.CampaignID, ID: mapID}); err == nil && pre.FogOnFirstGrid && pre.GridColumns == nil {
			if imageCopy, err = s.prepareFogCopy(ctx, m.CampaignID, mapID, pre.ImageID); err != nil {
				return nil, s.dbError(ctx, "copy the map's image for the fog", err)
			}
		}
	}
	var updated mapsdb.Map
	changed, copied := false, false // whether the layers were cleared or scaled; whether the fog's image copy was used
	err = db.InTx(ctx, s.pool, func(tx pgx.Tx) error {
		q := s.queries.WithTx(tx)
		changed, copied = false, false
		before, err := q.GetMapForUpdate(ctx, mapsdb.GetMapForUpdateParams{CampaignID: m.CampaignID, ID: mapID})
		if errors.Is(err, pgx.ErrNoRows) {
			return errMapNotFound()
		}
		if err != nil {
			return fmt.Errorf("find map: %w", err)
		}
		size, err := q.GetMapGrid(ctx, mapsdb.GetMapGridParams{CampaignID: m.CampaignID, ID: mapID})
		if err != nil {
			return fmt.Errorf("read the map's image size: %w", err)
		}
		var engine grid.Grid // the grid the rules will use; zero for none
		if drawn != 0 {
			if engine, err = grid.EngineGrid(int(drawn), int(factor), int(size.ImageWidth), int(size.ImageHeight)); err != nil {
				return connect.NewError(connect.CodeInvalidArgument, fmt.Errorf(
					"%d columns of %d squares each make a grid that passes the limit of %d x %d squares",
					drawn, factor, grid.MaxColumns, grid.MaxRows))
			}
		}
		engineColumns := int32(engine.Columns) //nolint:gosec // G115: at most 200
		var gridColumns *int32                 // NULL clears the grid
		if drawn != 0 {
			gridColumns = &engineColumns
		}
		if drawn == 0 {
			factor = 1
		}
		// What happens to the layers is decided from the row locked here, so a combat that
		// starts meanwhile is seen. The same grid and factor again change nothing; the
		// same drawing with a factor that is a multiple of the old one is scaled (D3);
		// anything else clears the layers, which a fight standing on them cannot lose (D2).
		// The grid is the same when the rules' columns and rows are: 24 drawn columns
		// of 1,5 m and 12 of 3 m make the same grid (when the image's proportions
		// agree), and nothing painted needs to change.
		oldGrid := gridOf(before.GridColumns, before.GridFactor, size.ImageWidth, size.ImageHeight)
		sameGrid := oldGrid == engine
		step, scales := grid.ScaleStep(int(before.GridFactor), int(factor))
		scales = scales && drawn != 0 && before.GridColumns != nil && drawn == *before.GridColumns/before.GridFactor
		if !sameGrid {
			if err := s.refuseWhileCombat(ctx, tx, m.CampaignID, mapID); err != nil {
				return err
			}
		}
		updated, err = q.SetMapGrid(ctx, mapsdb.SetMapGridParams{
			CampaignID: m.CampaignID, ID: mapID, GridColumns: gridColumns, GridFactor: factor, Now: s.now(),
		})
		if errors.Is(err, pgx.ErrNoRows) {
			return errMapNotFound()
		}
		if err != nil {
			return fmt.Errorf("set map grid: %w", err)
		}
		switch {
		case sameGrid:
		case scales:
			changed = true
			if err := scaleLayers(ctx, q, mapID, before.VisionEpoch, oldGrid, step, s.now()); err != nil {
				return err
			}
		default:
			changed = true
			if err := clearLayers(ctx, q, mapID); err != nil {
				return err
			}
		}
		if gridColumns != nil && before.FogOnFirstGrid && before.GridColumns == nil && !before.FogEnabled {
			if imageCopy != nil && before.ImageID == imageCopy.source.ID {
				if err := imageCopy.insert(ctx, q, s, m.CampaignID, m.UserID); err != nil {
					return err
				}
				if _, err := q.SetMapImageOnly(ctx, mapsdb.SetMapImageOnlyParams{CampaignID: m.CampaignID, ID: mapID, ImageID: imageCopy.id, Now: s.now()}); err != nil {
					return fmt.Errorf("give the map its copy of the image: %w", err)
				}
				copied = true
			}
			if updated, err = q.ApplyFogRule(ctx, mapsdb.ApplyFogRuleParams{CampaignID: m.CampaignID, ID: mapID, Now: s.now()}); err != nil {
				return fmt.Errorf("apply the table's fog rule: %w", err)
			}
		}
		return nil
	})
	if imageCopy != nil && (err != nil || !copied) {
		s.deleteFiles(ctx, m.CampaignID, imageCopy.id) // made for nothing
	}
	if err != nil {
		return nil, s.dbError(ctx, "set a map's grid", err)
	}
	s.publishMapChanged(m.CampaignID, mapID, playersSee(mapID, updated.RevealedAt, current))
	if changed {
		s.lits.forget(mapID)
		s.tiles.forget(mapID)
		s.refreshVision(ctx, m.CampaignID, mapID) // the players' memory went with the layers, or was scaled
	}
	out, err := s.masterMap(ctx, m, mapID)
	if err != nil {
		return nil, err
	}
	return connect.NewResponse(&mapsv1.SetMapGridResponse{Map: out}), nil
}

// int32PtrValue is the number a nullable column holds, 0 for NULL.
func int32PtrValue(n *int32) int32 {
	if n == nil {
		return 0
	}
	return *n
}

// pointChange is what the master asks to change in a point. Nil fields
// stay as they are.
type pointChange struct {
	kind        *string
	name        *string
	description *string
	hooks       *string // a SCENE point's private text; "" clears it
	showDC      *bool   // a SCENE point's "Mostrar a CD aos jogadores"
	x, y        *int32
	target      *string // "" removes the target
	revealed    *bool
	trap        *mapsv1.TrapSpec
	treasure    *int32
	light       *mapsv1.LightSpec
}

// CreateMapPoint implements mapsv1connect.MapServiceHandler.
func (s *Service) CreateMapPoint(
	ctx context.Context,
	req *connect.Request[mapsv1.CreateMapPointRequest],
) (*connect.Response[mapsv1.CreateMapPointResponse], error) {
	m, err := authz.RequireCampaignRole(ctx, req.Msg.GetCampaignId(), authz.RoleMaster)
	if err != nil {
		return nil, err
	}
	mapID, ok := parseID(req.Msg.GetMapId())
	if !ok {
		return nil, errMapNotFound()
	}
	kind, ok := kindToDB[req.Msg.GetKind()]
	if !ok {
		return nil, connect.NewError(connect.CodeInvalidArgument, errors.New("kind is required"))
	}
	name, err := cleanName("name", req.Msg.GetName())
	if err != nil {
		return nil, err
	}
	description, err := cleanDescription(req.Msg.GetDescription())
	if err != nil {
		return nil, err
	}
	hooks, err := cleanHooks(req.Msg.GetHooks())
	if err != nil {
		return nil, err
	}
	if hooks != "" && kind != kindToDB[mapsv1.MapPointKind_MAP_POINT_KIND_SCENE] {
		return nil, errOnlyScenesHaveHooks()
	}
	if req.Msg.GetShowDc() && kind != kindToDB[mapsv1.MapPointKind_MAP_POINT_KIND_SCENE] {
		return nil, errOnlyScenesShowDC()
	}
	if err := checkPosition(req.Msg.GetXBp(), req.Msg.GetYBp()); err != nil {
		return nil, err
	}
	target, err := parseTarget(req.Msg.GetTargetMapId(), kind, mapID)
	if err != nil {
		return nil, err
	}
	spec, err := s.specFor(kind, req.Msg.GetTrap(), req.Msg.TreasureValuePo, req.Msg.GetLight())
	if err != nil {
		return nil, err
	}

	current, err := s.currentMap(ctx, m.CampaignID)
	if err != nil {
		return nil, err
	}
	// The key is unique in the campaign (it shares the column of "Pôr no mapa"), and kept with a
	// hash of the whole request: a retry returns the first point only when it is the same request.
	scopedKey, requestHash, err := keyOf(m.CampaignID, req.Msg.GetIdempotencyKey(), req.Msg)
	if err != nil {
		return nil, err
	}
	var created mapsdb.MapPoint
	var mapRow mapsdb.Map
	replayed := false
	err = db.InTx(ctx, s.pool, func(tx pgx.Tx) error {
		q := s.queries.WithTx(tx)
		var err error
		if mapRow, err = s.campaignMap(ctx, q, m.CampaignID, mapID); err != nil {
			return err
		}
		created, replayed, err = idem.Create(ctx, scopedKey, requestHash, q.GetMapPointByCreateKey,
			func(p mapsdb.MapPoint) *string { return p.CreateHash },
			func() (mapsdb.MapPoint, error) {
				count, err := q.CountMapPoints(ctx, mapID)
				if err != nil {
					return created, fmt.Errorf("count points: %w", err)
				}
				if count >= s.maxPoints {
					return created, connect.NewError(connect.CodeResourceExhausted, fmt.Errorf("the map already has %d points", s.maxPoints))
				}
				if err := checkTarget(ctx, q, m.CampaignID, target); err != nil {
					return created, err
				}
				p, err := q.InsertMapPoint(ctx, mapsdb.InsertMapPointParams{
					MapID: mapID, Kind: kind, Name: name, Description: description, Hooks: hooks, ShowDc: req.Msg.GetShowDc(),
					XBp: req.Msg.GetXBp(), YBp: req.Msg.GetYBp(), TargetMapID: target, Now: s.now(),
					Trap: spec.trap, TrapState: spec.trapSt, TrapTriggeredAt: spec.triggeredAt(s.now), TreasureValuePo: spec.value,
					LightPreset: spec.light.preset, LightBrightFt: spec.lightBright(), LightDimFt: spec.lightDim(),
					CreateKey: scopedKey, CreateHash: requestHash,
				})
				if err != nil && !errors.Is(err, pgx.ErrNoRows) {
					return p, fmt.Errorf("insert point: %w", err)
				}
				return p, err
			})
		return err
	})
	if err != nil {
		return nil, s.dbError(ctx, "create a point", err)
	}
	// A new point is hidden: only the master hears about it (MR-009), unless it
	// is born visible to everyone (a trap created already triggered). A retry announces
	// nothing: the first call did.
	if !replayed {
		s.publishPointsChanged(ctx, m.CampaignID, mapRow, playersSee(mapID, mapRow.RevealedAt, current) && everyoneSees(created), created)
	}
	out, err := s.masterPoint(ctx, m, created)
	if err != nil {
		return nil, err
	}
	return connect.NewResponse(&mapsv1.CreateMapPointResponse{Point: out}), nil
}

// UpdateMapPoint implements mapsv1connect.MapServiceHandler.
func (s *Service) UpdateMapPoint(
	ctx context.Context,
	req *connect.Request[mapsv1.UpdateMapPointRequest],
) (*connect.Response[mapsv1.UpdateMapPointResponse], error) {
	m, err := authz.RequireCampaignRole(ctx, req.Msg.GetCampaignId(), authz.RoleMaster)
	if err != nil {
		return nil, err
	}
	mapID, ok := parseID(req.Msg.GetMapId())
	if !ok {
		return nil, errMapNotFound()
	}
	pointID, ok := parseID(req.Msg.GetPointId())
	if !ok {
		return nil, errPointNotFound()
	}
	msg := req.Msg
	if msg.Kind == nil && msg.Name == nil && msg.Description == nil && msg.Hooks == nil && msg.ShowDc == nil && msg.XBp == nil && msg.YBp == nil &&
		msg.TargetMapId == nil && msg.Revealed == nil && msg.Trap == nil && msg.TreasureValuePo == nil && msg.Light == nil {
		return nil, connect.NewError(connect.CodeInvalidArgument, errors.New("nothing to change"))
	}
	change := pointChange{
		x: msg.XBp, y: msg.YBp, target: msg.TargetMapId, revealed: msg.Revealed, showDC: msg.ShowDc,
		trap: msg.Trap, treasure: msg.TreasureValuePo, light: msg.Light,
	}
	if msg.Kind != nil {
		kind, ok := kindToDB[msg.GetKind()]
		if !ok {
			return nil, connect.NewError(connect.CodeInvalidArgument, errors.New("kind must not be unspecified"))
		}
		change.kind = &kind
	}
	if msg.Name != nil {
		name, err := cleanName("name", msg.GetName())
		if err != nil {
			return nil, err
		}
		change.name = &name
	}
	if msg.Description != nil {
		description, err := cleanDescription(msg.GetDescription())
		if err != nil {
			return nil, err
		}
		change.description = &description
	}
	if msg.Hooks != nil {
		hooks, err := cleanHooks(msg.GetHooks())
		if err != nil {
			return nil, err
		}
		change.hooks = &hooks
	}
	if err := checkPosition(msg.GetXBp(), msg.GetYBp()); err != nil {
		return nil, err // unset positions read as 0, which is valid
	}
	current, err := s.currentMap(ctx, m.CampaignID)
	if err != nil {
		return nil, err
	}

	var mapRow mapsdb.Map
	var before, after mapsdb.MapPoint
	var knowers []string
	err = db.InTx(ctx, s.pool, func(tx pgx.Tx) error {
		q := s.queries.WithTx(tx)
		var err error
		if mapRow, err = s.campaignMap(ctx, q, m.CampaignID, mapID); err != nil {
			return err
		}
		before, err = q.GetMapPointForUpdate(ctx, mapsdb.GetMapPointForUpdateParams{MapID: mapID, ID: pointID})
		if errors.Is(err, pgx.ErrNoRows) {
			return errPointNotFound()
		}
		if err != nil {
			return fmt.Errorf("find point: %w", err)
		}
		if knowers, err = s.trapKnowers(ctx, tx, q, m.CampaignID, before); err != nil {
			return err
		}
		after, err = s.applyPointChange(ctx, q, m.CampaignID, before, change)
		return err
	})
	if err != nil {
		return nil, s.dbError(ctx, "update a point", err)
	}
	s.publishPointsChanged(ctx, m.CampaignID, mapRow,
		playersSee(mapID, mapRow.RevealedAt, current) && (everyoneSees(before) || everyoneSees(after)), before, after)
	s.tellTrapKnowers(m.CampaignID, mapID, mapRow, current, knowers)
	// The open scene shows the point's name and description, and stops being
	// one when the point changes kind.
	s.publishSceneChangedIf(ctx, m.CampaignID, pointID)
	out, err := s.masterPoint(ctx, m, after)
	if err != nil {
		return nil, err
	}
	return connect.NewResponse(&mapsv1.UpdateMapPointResponse{Point: out}), nil
}

// applyPointChange checks a change against the point as it is, inside the
// transaction, and saves it.
func (s *Service) applyPointChange(ctx context.Context, q *mapsdb.Queries, campaignID string, p mapsdb.MapPoint, c pointChange) (mapsdb.MapPoint, error) {
	params := mapsdb.UpdateMapPointParams{
		MapID: p.MapID, ID: p.ID, Kind: p.Kind, Name: p.Name, Description: p.Description,
		Hooks: p.Hooks, ShowDc: p.ShowDc, XBp: p.XBp, YBp: p.YBp, TargetMapID: p.TargetMapID, RevealedAt: p.RevealedAt, Now: s.now(),
		Trap: p.Trap, TrapState: p.TrapState, TrapTriggeredAt: p.TrapTriggeredAt, TreasureValuePo: p.TreasureValuePo, TreasureFoundAt: p.TreasureFoundAt,
		TreasureSessionID: p.TreasureSessionID, LightPreset: p.LightPreset, LightBrightFt: p.LightBrightFt, LightDimFt: p.LightDimFt,
	}
	if c.kind != nil {
		params.Kind = *c.kind
	}
	if c.name != nil {
		params.Name = *c.name
	}
	if c.description != nil {
		params.Description = *c.description
	}
	if c.hooks != nil {
		params.Hooks = *c.hooks
	}
	if c.showDC != nil {
		params.ShowDc = *c.showDC
	}
	if c.x != nil {
		params.XBp = *c.x
	}
	if c.y != nil {
		params.YBp = *c.y
	}
	if c.target != nil {
		target, err := parseTarget(*c.target, params.Kind, p.MapID)
		if err != nil {
			return mapsdb.MapPoint{}, err
		}
		params.TargetMapID = target
	}
	if !leads(params.Kind) {
		params.TargetMapID = nil // only a submap or a battle leads somewhere
	}
	if err := checkTarget(ctx, q, campaignID, params.TargetMapID); err != nil {
		return mapsdb.MapPoint{}, err
	}
	if c.revealed != nil {
		params.RevealedAt = revealedAt(p.RevealedAt, *c.revealed, params.Now)
	}
	if err := s.applySpecChange(ctx, q, p, c, &params); err != nil {
		return mapsdb.MapPoint{}, err
	}
	scene := kindToDB[mapsv1.MapPointKind_MAP_POINT_KIND_SCENE]
	if params.Kind != scene && c.hooks != nil && *c.hooks != "" {
		return mapsdb.MapPoint{}, errOnlyScenesHaveHooks()
	}
	if params.Kind != scene && c.showDC != nil && *c.showDC {
		return mapsdb.MapPoint{}, errOnlyScenesShowDC()
	}
	if p.Kind == scene && params.Kind != p.Kind {
		// Only a scene has actions, clues, hooks and the DC switch (MR-015,
		// MR-029). A scene
		// still open in a session then reads as closed: it is not a scene
		// point anymore. What players already received stays in their notes.
		if err := q.DeleteSceneActionsOfPoint(ctx, p.ID); err != nil {
			return mapsdb.MapPoint{}, fmt.Errorf("delete the scene's actions: %w", err)
		}
		if err := q.DeleteSceneCluesOfPoint(ctx, p.ID); err != nil {
			return mapsdb.MapPoint{}, fmt.Errorf("delete the scene's clues: %w", err)
		}
		params.Hooks = ""
		params.ShowDc = false
	}
	if params.Kind == scene && params.RevealedAt != nil && (p.RevealedAt == nil || p.Kind != scene) {
		// Revealing a scene makes it "discovered" (MR-030, question 61): the
		// group may tag notes with it from now on, even if it is hidden again.
		if err := q.UpsertSceneDiscovery(ctx, mapsdb.UpsertSceneDiscoveryParams{CampaignID: campaignID, PointID: p.ID, DiscoveredAt: params.Now}); err != nil {
			return mapsdb.MapPoint{}, fmt.Errorf("record the scene's discovery: %w", err)
		}
	}
	out, err := q.UpdateMapPoint(ctx, params)
	if err != nil {
		return mapsdb.MapPoint{}, fmt.Errorf("update point: %w", err)
	}
	return out, nil
}

// applySpecChange works out the data of the new kinds (a trap, a treasure's
// value, a light) for the point after the change, and cleans up what a point
// that changes kind leaves behind. Each kind needs its own data, and refuses
// another's; a point keeps what it has unless the request replaces it.
func (s *Service) applySpecChange(ctx context.Context, q *mapsdb.Queries, p mapsdb.MapPoint, c pointChange, params *mapsdb.UpdateMapPointParams) error {
	trap, treasure, light := kindToDB[mapsv1.MapPointKind_MAP_POINT_KIND_TRAP], kindToDB[mapsv1.MapPointKind_MAP_POINT_KIND_TREASURE], kindToDB[mapsv1.MapPointKind_MAP_POINT_KIND_LIGHT]
	switch {
	case params.Kind == trap:
		if c.trap != nil {
			var current *string
			if p.Kind == trap {
				current = p.TrapState
			}
			t, err := s.cleanTrap(c.trap, current)
			if err != nil {
				return err
			}
			params.Trap, params.TrapState = t.json, &t.state
			if t.state == stateTriggered && params.TrapTriggeredAt == nil {
				params.TrapTriggeredAt = &params.Now // the first time it fires, for good
			}
		} else if p.Kind != trap {
			return badSpec("trap is required to make a point a TRAP")
		}
	case c.trap != nil:
		return errNotThatKind("trap", "TRAP")
	default:
		params.Trap, params.TrapState, params.TrapTriggeredAt = nil, nil, nil
		if p.Kind == trap {
			if err := q.DeletePointReveals(ctx, p.ID); err != nil {
				return fmt.Errorf("forget the trap's reveals: %w", err)
			}
		}
	}

	converted := p.TreasureConvertedAwardID != nil
	switch {
	case params.Kind == treasure:
		switch {
		case c.treasure != nil:
			v, err := cleanTreasureValue(*c.treasure)
			if err != nil {
				return err
			}
			if converted && (p.TreasureValuePo == nil || *p.TreasureValuePo != v) {
				return errTreasureConverted() // its worth became XP
			}
			params.TreasureValuePo = &v
		case p.Kind != treasure:
			zero := int32(0)
			params.TreasureValuePo = &zero
		}
	case c.treasure != nil:
		return errNotThatKind("treasure_value_po", "TREASURE")
	default:
		if p.Kind == treasure {
			if converted {
				return errTreasureConverted()
			}
			// Like a delete: a found treasure is part of the session's record, unmarked
			// before it goes, never lost silently with its finders.
			if p.TreasureFoundAt != nil {
				return errTreasureFound()
			}
			if err := q.DeleteTreasureFinders(ctx, p.ID); err != nil {
				return fmt.Errorf("forget the treasure's finders: %w", err)
			}
		}
		params.TreasureValuePo, params.TreasureFoundAt, params.TreasureSessionID = nil, nil, nil
	}

	switch {
	case params.Kind == light:
		if c.light != nil {
			l, err := s.cleanLight(c.light)
			if err != nil {
				return err
			}
			params.LightPreset, params.LightBrightFt, params.LightDimFt = l.preset, &l.bright, &l.dimmed
		} else if p.Kind != light {
			return badSpec("light is required to make a point a LIGHT")
		}
		// No player ever receives a light, so it has nothing to reveal.
		if c.revealed != nil && *c.revealed {
			return badSpec("a LIGHT point is never shown to the players, so it cannot be revealed")
		}
		params.RevealedAt = nil
	case c.light != nil:
		return errNotThatKind("light", "LIGHT")
	default:
		params.LightPreset, params.LightBrightFt, params.LightDimFt = nil, nil, nil
	}
	return nil
}

// DeleteMapPoint implements mapsv1connect.MapServiceHandler.
func (s *Service) DeleteMapPoint(
	ctx context.Context,
	req *connect.Request[mapsv1.DeleteMapPointRequest],
) (*connect.Response[mapsv1.DeleteMapPointResponse], error) {
	m, err := authz.RequireCampaignRole(ctx, req.Msg.GetCampaignId(), authz.RoleMaster)
	if err != nil {
		return nil, err
	}
	mapID, ok := parseID(req.Msg.GetMapId())
	if !ok {
		return nil, errMapNotFound()
	}
	pointID, ok := parseID(req.Msg.GetPointId())
	if !ok {
		return nil, errPointNotFound()
	}
	current, err := s.currentMap(ctx, m.CampaignID)
	if err != nil {
		return nil, err
	}
	openScene, err := s.live.OpenScenePoint(ctx, m.CampaignID) // the foreign key closes it with the point
	if err != nil {
		return nil, s.dbError(ctx, "read the open scene", err)
	}

	var mapRow mapsdb.Map
	var deleted mapsdb.MapPoint
	var knowers []string
	err = db.InTx(ctx, s.pool, func(tx pgx.Tx) error {
		q := s.queries.WithTx(tx)
		var err error
		if mapRow, err = s.campaignMap(ctx, q, m.CampaignID, mapID); err != nil {
			return err
		}
		point, err := q.GetMapPointForUpdate(ctx, mapsdb.GetMapPointForUpdateParams{MapID: mapID, ID: pointID})
		if errors.Is(err, pgx.ErrNoRows) {
			return errPointNotFound()
		}
		if err != nil {
			return fmt.Errorf("find point: %w", err)
		}
		// A treasure that was found or turned into XP is part of the session's
		// record: it is unmarked before it goes, never lost silently.
		if point.TreasureConvertedAwardID != nil {
			return errTreasureConverted()
		}
		if point.TreasureFoundAt != nil {
			return errTreasureFound()
		}
		// Read before the delete cascades the reveal rows away.
		if knowers, err = s.trapKnowers(ctx, tx, q, m.CampaignID, point); err != nil {
			return err
		}
		deleted, err = q.DeleteMapPoint(ctx, mapsdb.DeleteMapPointParams{MapID: mapID, ID: pointID})
		if errors.Is(err, pgx.ErrNoRows) {
			return errPointNotFound()
		}
		if err != nil {
			return fmt.Errorf("delete point: %w", err)
		}
		return nil
	})
	if err != nil {
		return nil, s.dbError(ctx, "delete a point", err)
	}
	s.publishPointsChanged(ctx, m.CampaignID, mapRow, playersSee(mapID, mapRow.RevealedAt, current) && everyoneSees(deleted), deleted)
	s.tellTrapKnowers(m.CampaignID, mapID, mapRow, current, knowers)
	if openScene == pointID {
		s.publishSceneChanged(m.CampaignID)
	}
	return connect.NewResponse(&mapsv1.DeleteMapPointResponse{}), nil
}

// SetMapPointRevealed implements mapsv1connect.MapServiceHandler.
func (s *Service) SetMapPointRevealed(
	ctx context.Context,
	req *connect.Request[mapsv1.SetMapPointRevealedRequest],
) (*connect.Response[mapsv1.SetMapPointRevealedResponse], error) {
	m, err := authz.RequireCampaignRole(ctx, req.Msg.GetCampaignId(), authz.RoleMaster)
	if err != nil {
		return nil, err
	}
	mapID, ok := parseID(req.Msg.GetMapId())
	if !ok {
		return nil, errMapNotFound()
	}
	pointID, ok := parseID(req.Msg.GetPointId())
	if !ok {
		return nil, errPointNotFound()
	}
	current, err := s.currentMap(ctx, m.CampaignID)
	if err != nil {
		return nil, err
	}
	revealed := req.Msg.GetRevealed()

	var mapRow mapsdb.Map
	var before, after mapsdb.MapPoint
	var knowers []string
	err = db.InTx(ctx, s.pool, func(tx pgx.Tx) error {
		q := s.queries.WithTx(tx)
		var err error
		if mapRow, err = s.campaignMap(ctx, q, m.CampaignID, mapID); err != nil {
			return err
		}
		before, err = q.GetMapPointForUpdate(ctx, mapsdb.GetMapPointForUpdateParams{MapID: mapID, ID: pointID})
		if errors.Is(err, pgx.ErrNoRows) {
			return errPointNotFound()
		}
		if err != nil {
			return fmt.Errorf("find point: %w", err)
		}
		if knowers, err = s.trapKnowers(ctx, tx, q, m.CampaignID, before); err != nil {
			return err
		}
		after, err = s.applyPointChange(ctx, q, m.CampaignID, before, pointChange{revealed: &revealed})
		return err
	})
	if err != nil {
		return nil, s.dbError(ctx, "reveal or hide a point", err)
	}
	s.publishPointsChanged(ctx, m.CampaignID, mapRow,
		playersSee(mapID, mapRow.RevealedAt, current) && (everyoneSees(before) || everyoneSees(after)), before, after)
	s.tellTrapKnowers(m.CampaignID, mapID, mapRow, current, knowers)
	out, err := s.masterPoint(ctx, m, after)
	if err != nil {
		return nil, err
	}
	return connect.NewResponse(&mapsv1.SetMapPointRevealedResponse{Point: out}), nil
}

// PlaceMapToken implements mapsv1connect.MapServiceHandler.
func (s *Service) PlaceMapToken(
	ctx context.Context,
	req *connect.Request[mapsv1.PlaceMapTokenRequest],
) (*connect.Response[mapsv1.PlaceMapTokenResponse], error) {
	m, err := authz.RequireCampaignRole(ctx, req.Msg.GetCampaignId(), authz.RoleMaster)
	if err != nil {
		return nil, err
	}
	mapID, ok := parseID(req.Msg.GetMapId())
	if !ok {
		return nil, errMapNotFound()
	}
	if err := checkPosition(req.Msg.GetXBp(), req.Msg.GetYBp()); err != nil {
		return nil, err
	}
	if err := tokenSubject(req.Msg.GetCharacterId(), req.Msg.GetCreatureId()); err != nil {
		return nil, err
	}
	if req.Msg.GetCreatureId() != "" {
		res, err := s.placeCreatureToken(ctx, m, req.Msg, mapID)
		if err != nil {
			return nil, err
		}
		return connect.NewResponse(res), nil
	}
	character, err := s.livingCharacter(ctx, m.CampaignID, req.Msg.GetCharacterId())
	if err != nil {
		return nil, err
	}
	current, err := s.currentMap(ctx, m.CampaignID)
	if err != nil {
		return nil, err
	}

	var mapRow mapsdb.Map
	var token, was mapsdb.MapToken
	var placed bool // a new token, not a move
	err = db.InTx(ctx, s.pool, func(tx pgx.Tx) error {
		q := s.queries.WithTx(tx)
		placed = false
		var err error
		if mapRow, err = s.campaignMap(ctx, q, m.CampaignID, mapID); err != nil {
			return err
		}
		key := mapsdb.GetMapTokenForUpdateParams{MapID: mapID, CharacterID: character.GetId()}
		was, err = q.GetMapTokenForUpdate(ctx, key)
		switch {
		case errors.Is(err, pgx.ErrNoRows):
			// A player's character starts visible, an NPC hidden (question
			// 31 for Samuel): the master reveals an NPC when the group meets
			// it.
			token, err = q.InsertMapToken(ctx, mapsdb.InsertMapTokenParams{
				MapID: mapID, CharacterID: character.GetId(), XBp: req.Msg.GetXBp(), YBp: req.Msg.GetYBp(),
				Hidden:    character.GetKind() != charactersv1.CharacterKind_CHARACTER_KIND_PLAYER,
				UpdatedAt: s.now(),
			})
			placed = true
		case err == nil:
			token, err = q.MoveMapToken(ctx, mapsdb.MoveMapTokenParams{
				MapID: mapID, CharacterID: character.GetId(), XBp: req.Msg.GetXBp(), YBp: req.Msg.GetYBp(), UpdatedAt: s.now(),
			})
		}
		if err != nil {
			return fmt.Errorf("place token: %w", err)
		}
		return nil
	})
	if err != nil {
		return nil, s.dbError(ctx, "place a token", err)
	}
	// A new token needs its name: the app reads the map again. A move is a
	// `token_moved`, except for an NPC on a fog map, which reaches a player only as
	// what they see (publishTokenWritten).
	var before *mapsdb.MapToken
	if !placed {
		before = &was
	}
	s.publishTokenWritten(ctx, m.CampaignID, mapRow, current, before, &token, isPlayerCharacter(character), !placed)
	s.tokenLanded(ctx, m.CampaignID, m.UserID, mapID, token, character) // a trap may fire, or be noticed (MR-035)
	return connect.NewResponse(&mapsv1.PlaceMapTokenResponse{Token: tokenToProto(token, character, newViewer(m, current))}), nil
}

// SetMapTokenHidden implements mapsv1connect.MapServiceHandler.
func (s *Service) SetMapTokenHidden(
	ctx context.Context,
	req *connect.Request[mapsv1.SetMapTokenHiddenRequest],
) (*connect.Response[mapsv1.SetMapTokenHiddenResponse], error) {
	m, err := authz.RequireCampaignRole(ctx, req.Msg.GetCampaignId(), authz.RoleMaster)
	if err != nil {
		return nil, err
	}
	mapID, ok := parseID(req.Msg.GetMapId())
	if !ok {
		return nil, errMapNotFound()
	}
	character, err := s.livingCharacter(ctx, m.CampaignID, req.Msg.GetCharacterId())
	if err != nil {
		return nil, err // a dead character's token is not listed either
	}
	current, err := s.currentMap(ctx, m.CampaignID)
	if err != nil {
		return nil, err
	}

	var mapRow mapsdb.Map
	var before, after mapsdb.MapToken
	err = db.InTx(ctx, s.pool, func(tx pgx.Tx) error {
		q := s.queries.WithTx(tx)
		var err error
		if mapRow, err = s.campaignMap(ctx, q, m.CampaignID, mapID); err != nil {
			return err
		}
		before, err = q.GetMapTokenForUpdate(ctx, mapsdb.GetMapTokenForUpdateParams{MapID: mapID, CharacterID: character.GetId()})
		if errors.Is(err, pgx.ErrNoRows) {
			return errTokenNotFound()
		}
		if err != nil {
			return fmt.Errorf("find token: %w", err)
		}
		after, err = q.SetMapTokenHidden(ctx, mapsdb.SetMapTokenHiddenParams{
			MapID: mapID, CharacterID: character.GetId(), Hidden: req.Msg.GetHidden(), Now: s.now(),
		})
		if err != nil {
			return fmt.Errorf("hide or show token: %w", err)
		}
		return nil
	})
	if err != nil {
		return nil, s.dbError(ctx, "hide or show a token", err)
	}
	s.publishTokenWritten(ctx, m.CampaignID, mapRow, current, &before, &after, isPlayerCharacter(character), false)
	return connect.NewResponse(&mapsv1.SetMapTokenHiddenResponse{Token: tokenToProto(after, character, newViewer(m, current))}), nil
}

// RemoveMapToken implements mapsv1connect.MapServiceHandler. It does not
// ask whether the character still lives: a dead character's token can be
// taken off too.
func (s *Service) RemoveMapToken(
	ctx context.Context,
	req *connect.Request[mapsv1.RemoveMapTokenRequest],
) (*connect.Response[mapsv1.RemoveMapTokenResponse], error) {
	m, err := authz.RequireCampaignRole(ctx, req.Msg.GetCampaignId(), authz.RoleMaster)
	if err != nil {
		return nil, err
	}
	mapID, ok := parseID(req.Msg.GetMapId())
	if !ok {
		return nil, errMapNotFound()
	}
	if err := tokenSubject(req.Msg.GetCharacterId(), req.Msg.GetCreatureId()); err != nil {
		return nil, err
	}
	if req.Msg.GetCreatureId() != "" {
		if err := s.removeCreatureToken(ctx, m, mapID, req.Msg.GetCreatureId()); err != nil {
			return nil, err
		}
		return connect.NewResponse(&mapsv1.RemoveMapTokenResponse{}), nil
	}
	characterID, ok := parseID(req.Msg.GetCharacterId())
	if !ok {
		return nil, errTokenNotFound()
	}
	current, err := s.currentMap(ctx, m.CampaignID)
	if err != nil {
		return nil, err
	}

	var mapRow mapsdb.Map
	var deleted mapsdb.MapToken
	err = db.InTx(ctx, s.pool, func(tx pgx.Tx) error {
		q := s.queries.WithTx(tx)
		var err error
		if mapRow, err = s.campaignMap(ctx, q, m.CampaignID, mapID); err != nil {
			return err
		}
		deleted, err = q.DeleteMapToken(ctx, mapsdb.DeleteMapTokenParams{MapID: mapID, CharacterID: characterID})
		if errors.Is(err, pgx.ErrNoRows) {
			return errTokenNotFound()
		}
		if err != nil {
			return fmt.Errorf("remove token: %w", err)
		}
		return nil
	})
	if err != nil {
		return nil, s.dbError(ctx, "remove a token", err)
	}
	// Whose token it was: a dead character's is told as an NPC's, which reaches no
	// player on a fog map but through what they see.
	player := false
	if found, err := s.characters.MapCharacters(ctx, nil, m.CampaignID, []string{characterID}); err == nil && len(found) == 1 {
		player = isPlayerCharacter(found[0])
	}
	s.publishTokenWritten(ctx, m.CampaignID, mapRow, current, &deleted, nil, player, false)
	return connect.NewResponse(&mapsv1.RemoveMapTokenResponse{}), nil
}

// Helpers.

// viewerOf reads the session's current map, which a player sees too.
func (s *Service) viewerOf(ctx context.Context, m authz.Membership) (viewer, error) {
	current, err := s.currentMap(ctx, m.CampaignID)
	if err != nil {
		return viewer{}, err
	}
	v := newViewer(m, current)
	if !v.master {
		if v.traps, err = s.knownTraps(ctx, m.CampaignID, m.UserID); err != nil {
			return viewer{}, s.dbError(ctx, "read the traps a player knows", err)
		}
	}
	return v, nil
}

// currentMap asks the live session for the current map.
func (s *Service) currentMap(ctx context.Context, campaignID string) (string, error) {
	current, _, err := s.live.OnScreen(ctx, campaignID)
	if err != nil {
		return "", s.dbError(ctx, "read the current map", err)
	}
	return current, nil
}

// masterMap reads a map again, as the master sees it, for a write's
// answer.
func (s *Service) masterMap(ctx context.Context, m authz.Membership, mapID string) (*mapsv1.Map, error) {
	v, err := s.viewerOf(ctx, m)
	if err != nil {
		return nil, err
	}
	cm, err := s.loadMaps(ctx, m.CampaignID)
	if err != nil {
		return nil, s.dbError(ctx, "read a map", err)
	}
	row, ok := cm.byID[mapID]
	if !ok {
		return nil, errMapNotFound() // deleted right after the change
	}
	return cm.mapToProto(row, v), nil
}

// masterPoint builds a write's answer: the point as the master sees it,
// with its target's name.
func (s *Service) masterPoint(ctx context.Context, m authz.Membership, p mapsdb.MapPoint) (*mapsv1.MapPoint, error) {
	cm, err := s.loadMaps(ctx, m.CampaignID)
	if err != nil {
		return nil, s.dbError(ctx, "read the maps", err)
	}
	out := s.pointToProto(cm, p, viewer{master: true, userID: m.UserID})
	if err := s.attachPointDetails(ctx, m.CampaignID, p.MapID, []*mapsv1.MapPoint{out}, true); err != nil {
		return nil, s.dbError(ctx, "list a point's traps and treasures", err)
	}
	if err := s.attachActions(ctx, p.MapID, []*mapsv1.MapPoint{out}, true); err != nil {
		return nil, s.dbError(ctx, "list a point's scene actions", err)
	}
	if err := s.attachClues(ctx, m.CampaignID, p.MapID, []*mapsv1.MapPoint{out}, true); err != nil {
		return nil, s.dbError(ctx, "list a point's scene clues", err)
	}
	return out, nil
}

// campaignMap returns the map, or `not_found` when it is not the
// campaign's.
func (s *Service) campaignMap(ctx context.Context, q *mapsdb.Queries, campaignID, mapID string) (mapsdb.Map, error) {
	row, err := q.GetMap(ctx, mapsdb.GetMapParams{CampaignID: campaignID, ID: mapID})
	if errors.Is(err, pgx.ErrNoRows) {
		return mapsdb.Map{}, errMapNotFound()
	}
	if err != nil {
		return mapsdb.Map{}, fmt.Errorf("find map: %w", err)
	}
	return row, nil
}

// livingCharacter returns a living character of the campaign, or
// `not_found`.
func (s *Service) livingCharacter(ctx context.Context, campaignID, characterID string) (*charactersv1.CharacterSummary, error) {
	id, ok := parseID(characterID)
	if !ok {
		return nil, errCharacterNotFound()
	}
	found, err := s.characters.MapCharacters(ctx, nil, campaignID, []string{id})
	if err != nil {
		return nil, s.dbError(ctx, "find a character", err)
	}
	if len(found) != 1 {
		return nil, errCharacterNotFound()
	}
	return found[0], nil
}

// checkImage checks that an image is in the campaign's gallery.
func checkImage(ctx context.Context, q *mapsdb.Queries, campaignID, imageID string) error {
	_, err := q.GetGalleryImageInCampaign(ctx, mapsdb.GetGalleryImageInCampaignParams{CampaignID: campaignID, ID: imageID})
	if errors.Is(err, pgx.ErrNoRows) {
		return errNotAGalleryImage()
	}
	if err != nil {
		return fmt.Errorf("find image: %w", err)
	}
	return nil
}

// leads says whether a point of this kind (as the database stores it) may
// lead to another map: a submap to the map it opens, a battle to the map
// of its fight (MR-013).
func leads(kind string) bool {
	return kind == kindToDB[mapsv1.MapPointKind_MAP_POINT_KIND_SUBMAP] || kind == kindToDB[mapsv1.MapPointKind_MAP_POINT_KIND_BATTLE]
}

// parseTarget reads a point's target from a request: nil for none. Only a
// submap or a battle leads somewhere, and never to its own map.
func parseTarget(raw, kind, mapID string) (*string, error) {
	if raw == "" {
		return nil, nil
	}
	if !leads(kind) {
		return nil, connect.NewError(connect.CodeInvalidArgument, errors.New("target_map_id is only for a SUBMAP or a BATTLE point"))
	}
	id, ok := parseID(raw)
	if !ok {
		return nil, errNotACampaignMap()
	}
	if id == mapID {
		return nil, connect.NewError(connect.CodeInvalidArgument, errors.New("target_map_id must be another map, not the point's own"))
	}
	return &id, nil
}

// checkTarget checks that a point's target (submap or battle) is a map of
// the campaign.
func checkTarget(ctx context.Context, q *mapsdb.Queries, campaignID string, target *string) error {
	if target == nil {
		return nil
	}
	_, err := q.GetMap(ctx, mapsdb.GetMapParams{CampaignID: campaignID, ID: *target})
	if errors.Is(err, pgx.ErrNoRows) {
		return errNotACampaignMap()
	}
	if err != nil {
		return fmt.Errorf("find the target map: %w", err)
	}
	return nil
}

// revealedAt is a point's revealed_at after a reveal or hide: revealing
// keeps the first time.
func revealedAt(was *time.Time, revealed bool, now time.Time) *time.Time {
	switch {
	case !revealed:
		return nil
	case was != nil:
		return was
	default:
		return &now
	}
}

// cleanName checks a map or point name. The message says the rule, never
// the name, which is free text.
func cleanName(field, raw string) (string, error) {
	name, err := names.Clean(raw, maxNameLength)
	if err != nil {
		return "", connect.NewError(connect.CodeInvalidArgument, fmt.Errorf("%s %w", field, err))
	}
	return name, nil
}

// cleanDescription checks a point's description: several lines allowed.
func cleanDescription(raw string) (string, error) {
	description, err := names.CleanText(raw, maxDescriptionLength)
	if err != nil {
		return "", connect.NewError(connect.CodeInvalidArgument, fmt.Errorf("description %w", err))
	}
	return description, nil
}

// cleanHooks checks a point's hooks (MR-029): the master's private Markdown,
// several lines allowed, empty for none.
func cleanHooks(raw string) (string, error) {
	hooks, err := names.CleanText(raw, maxHooksLength)
	if err != nil {
		return "", connect.NewError(connect.CodeInvalidArgument, fmt.Errorf("hooks %w", err))
	}
	return hooks, nil
}

func errOnlyScenesShowDC() error {
	return connect.NewError(connect.CodeInvalidArgument, errors.New("only a SCENE point shows a DC to the players"))
}

func errOnlyScenesHaveHooks() error {
	return connect.NewError(connect.CodeInvalidArgument, errors.New("only a SCENE point has hooks"))
}

// checkPosition checks a position in basis points.
func checkPosition(x, y int32) error {
	if x < 0 || x > maxPosition || y < 0 || y > maxPosition {
		return connect.NewError(connect.CodeInvalidArgument, fmt.Errorf("x_bp and y_bp must be 0 to %d", maxPosition))
	}
	return nil
}

// parseID returns an ID in canonical form, or false when it is not a UUID:
// such an ID names nothing.
func parseID(raw string) (string, bool) {
	id, err := uuid.Parse(raw)
	if err != nil {
		return "", false
	}
	return id.String(), true
}

// The answers for what is not in the campaign. Their texts are the same
// whether the thing exists or not.
func errMapNotFound() error {
	return connect.NewError(connect.CodeNotFound, errors.New("map not found"))
}

func errPointNotFound() error {
	return connect.NewError(connect.CodeNotFound, errors.New("point not found"))
}

func errTokenNotFound() error {
	return connect.NewError(connect.CodeNotFound, errors.New("token not found"))
}

func errCharacterNotFound() error {
	return connect.NewError(connect.CodeNotFound, errors.New("character not found"))
}

func errNotAGalleryImage() error {
	return connect.NewError(connect.CodeInvalidArgument, errors.New("image_id is not an image of the campaign's gallery"))
}

func errNotACampaignMap() error {
	return connect.NewError(connect.CodeInvalidArgument, errors.New("target_map_id is not a map of the campaign"))
}

// errStaleMap is the answer when the map changed since the client read it
// (AIP-154).
func errStaleMap() error {
	return connect.NewError(connect.CodeAborted, errors.New("the map changed since you opened it; reload it and try again"))
}

// keyOf checks a create's idempotency key and returns it as it is stored (the campaign's ID and
// the key; nil for no key) with the hash of the whole request, for idem.Create.
func keyOf(campaignID, key string, msg proto.Message) (scoped, hash *string, err error) {
	key, err = idem.Clean(key)
	if err != nil {
		return nil, nil, err
	}
	return idem.Scope(campaignID, key), idem.Hash(msg), nil
}
