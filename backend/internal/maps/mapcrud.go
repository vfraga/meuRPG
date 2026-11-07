package maps

import (
	"context"
	"errors"
	"fmt"

	"connectrpc.com/connect"
	"github.com/jackc/pgx/v5"

	charactersv1 "github.com/PuraFome/meuRPG/backend/gen/meurpg/characters/v1"
	mapsv1 "github.com/PuraFome/meuRPG/backend/gen/meurpg/maps/v1"
	"github.com/PuraFome/meuRPG/backend/internal/authz"
	"github.com/PuraFome/meuRPG/backend/internal/maps/mapsdb"
	"github.com/PuraFome/meuRPG/backend/internal/platform/db"
	"github.com/PuraFome/meuRPG/backend/internal/platform/idem"
	"github.com/PuraFome/meuRPG/backend/internal/platform/uid"
)

// GetMap (RN-10): the snapshot of the map, its points and its tokens is
// read in one read-only transaction, and what a player may see of it is decided
// from that snapshot, point by point and token by token.

// GetMap implements mapsv1connect.MapServiceHandler.
func (s *Service) GetMap(
	ctx context.Context,
	req *connect.Request[mapsv1.GetMapRequest],
) (*connect.Response[mapsv1.GetMapResponse], error) {
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
	res, err := s.mapFor(ctx, m.CampaignID, mapID, v)
	if err != nil {
		return nil, err
	}
	return connect.NewResponse(res), nil
}

// mapFor is the map, its points and its tokens as the viewer reads them: the one code path of GetMap and of
// the table screen's map.
func (s *Service) mapFor(ctx context.Context, campaignID, mapID string, v viewer) (*mapsv1.GetMapResponse, error) {
	snap, err := s.readMapSnapshot(ctx, campaignID, mapID, v)
	if err != nil {
		return nil, s.dbError(ctx, "get a map", err)
	}
	res, _, err := s.mapOfSnapshot(ctx, snap, v)
	return res, err
}

// mapOfSnapshot is mapFor on a snapshot already read. It also returns what the viewer sees of the squares
// (nil for the master and for a map without fog).
func (s *Service) mapOfSnapshot(ctx context.Context, snap mapSnapshot, v viewer) (*mapsv1.GetMapResponse, *playerView, error) {
	campaignID, mapID := snap.row.CampaignID, snap.row.ID
	if !v.master {
		v.parentKnows = s.parentKnows(ctx, snap.cm, v)
	}
	res := &mapsv1.GetMapResponse{Map: snap.cm.mapToProto(snap.row, v)}
	pv, err := s.mapFogView(ctx, snap, v)
	if err != nil {
		return nil, nil, err
	}
	if err := s.fillMapPoints(ctx, campaignID, mapID, res, snap, v, pv); err != nil {
		return nil, nil, err
	}
	if res.Tokens, err = s.mapTokensFor(ctx, campaignID, mapID, snap, v, pv); err != nil {
		return nil, nil, err
	}
	return res, pv, nil
}

// mapSnapshot is what GetMap reads in one transaction. The map's rows, its points
// and its tokens are one snapshot: three autocommit reads could show a token moved
// on a revision that does not count the move.
type mapSnapshot struct {
	cm      campaignMaps
	row     mapsdb.ListMapDetailsRow
	points  []mapsdb.MapPoint
	tokens  []mapsdb.MapToken
	details pointDetails
}

// readMapSnapshot reads the snapshot. A hidden map is "not found" to a player
// exactly as a map that does not exist (RN-10), and its points are not read. Who
// found each treasure and who knows each trap belong to the same moment as the
// points.
func (s *Service) readMapSnapshot(ctx context.Context, campaignID, mapID string, v viewer) (mapSnapshot, error) {
	var snap mapSnapshot
	err := db.ReadTx(ctx, s.pool, func(tx pgx.Tx) error {
		q := s.queries.WithTx(tx)
		var err error
		if snap.cm, err = loadMapsWith(ctx, q, campaignID); err != nil {
			return err
		}
		var ok bool
		if snap.row, ok = snap.cm.byID[mapID]; !ok || !v.seesMap(snap.row.ID, snap.row.RevealedAt) {
			return errMapNotFound()
		}
		if snap.points, err = q.ListMapPoints(ctx, mapID); err != nil {
			return fmt.Errorf("list a map's points: %w", err)
		}
		if snap.tokens, err = q.ListMapTokens(ctx, mapID); err != nil {
			return fmt.Errorf("list a map's tokens: %w", err)
		}
		snap.details, err = readPointDetails(ctx, q, mapID, v.master)
		return err
	})
	return snap, err
}

// mapFogView is what a player sees of a fog map now or remembers (RN-10); nil for
// the master and for a map without fog.
func (s *Service) mapFogView(ctx context.Context, snap mapSnapshot, v viewer) (*playerView, error) {
	if !foggedFor(v, snap.row) {
		return nil, nil
	}
	pv, err := s.playerViewOf(ctx, fogInputOfDetails(snap.row), snap.points, snap.tokens, v.userID)
	if err != nil {
		return nil, s.dbError(ctx, "work out what a player sees", err)
	}
	return pv, nil
}

// fillMapPoints puts the points the viewer may see into the answer, with what each
// kind of point needs read separately.
func (s *Service) fillMapPoints(ctx context.Context, campaignID, mapID string, res *mapsv1.GetMapResponse, snap mapSnapshot, v viewer, pv *playerView) error {
	for _, p := range s.visiblePoints(snap.points, v, pv) {
		out := s.pointToProto(snap.cm, p, v)
		out.Remembered = pv != nil && !s.pointSeenNow(pv, p)
		res.Points = append(res.Points, out)
	}
	if pv != nil {
		res.Map.PointCount = int32(len(res.Points)) //nolint:gosec // G115: a map has at most 200 points
	}
	if err := s.attachPointDetails(ctx, campaignID, res.Points, snap.details); err != nil {
		return s.dbError(ctx, "list a map's traps and treasures", err)
	}
	if err := s.attachActions(ctx, mapID, res.Points, v.master); err != nil {
		return s.dbError(ctx, "list a map's scene actions", err)
	}
	if err := s.attachImages(ctx, mapID, res.Points, v.master); err != nil {
		return s.dbError(ctx, "read the scene images", err)
	}
	if err := s.attachClues(ctx, campaignID, mapID, res.Points, v.master); err != nil {
		return s.dbError(ctx, "list a map's scene clues", err)
	}
	return nil
}

// mapTokensFor lists the tokens the viewer may see: the characters' first, then the
// creatures' (party tokens, never kept from a player, fog or not, MR-037).
func (s *Service) mapTokensFor(ctx context.Context, campaignID, mapID string, snap mapSnapshot, v viewer, pv *playerView) ([]*mapsv1.MapToken, error) {
	byCharacter, ids := tokensToAsk(snap.tokens, v, pv)
	// The characters come back in their own order (players' first), and
	// only the living ones: a dead character's token stays on the map, but
	// is not listed.
	characters, err := s.characters.MapCharacters(ctx, nil, campaignID, ids)
	if err != nil {
		return nil, s.dbError(ctx, "read the characters on a map", err)
	}
	out := characterTokens(characters, byCharacter, v, pv)
	creatureTokens, err := s.creatureTokensOf(ctx, campaignID, mapID, v)
	if err != nil {
		return nil, s.dbError(ctx, "list a map's creature tokens", err)
	}
	return append(out, creatureTokens...), nil
}

// tokensToAsk picks the tokens whose characters are asked for, by character, and
// their IDs. On a fog map the kind decides (a player character is always shown), so
// the characters are asked for before the tokens are filtered.
func tokensToAsk(tokens []mapsdb.MapToken, v viewer, pv *playerView) (map[string]mapsdb.MapToken, []string) {
	byCharacter := make(map[string]mapsdb.MapToken, len(tokens))
	ids := make([]string, 0, len(tokens))
	for _, t := range tokens {
		if pv != nil || v.seesToken(t) {
			byCharacter[t.CharacterID] = t
			ids = append(ids, t.CharacterID)
		}
	}
	return byCharacter, ids
}

// characterTokens is the tokens of the characters that came back, as the viewer
// reads them.
func characterTokens(characters []*charactersv1.CharacterSummary, byCharacter map[string]mapsdb.MapToken, v viewer, pv *playerView) []*mapsv1.MapToken {
	var out []*mapsv1.MapToken
	for _, c := range characters {
		t, ok := byCharacter[c.GetId()]
		if !ok {
			continue
		}
		if pv != nil {
			if t, ok = fogTokenOf(pv, t, c); !ok {
				continue
			}
		}
		out = append(out, tokenToProto(t, c, v))
	}
	return out
}

// fogTokenOf is a token as a player of a fog map reads it, and whether they see
// it at all: a hidden creature never, and a player's character is never hidden
// from the party (D6).
func fogTokenOf(pv *playerView, t mapsdb.MapToken, c *charactersv1.CharacterSummary) (mapsdb.MapToken, bool) {
	player := c.GetKind() == charactersv1.CharacterKind_CHARACTER_KIND_PLAYER
	if !tokenSeen(pv, t, player) || (!player && t.Hidden) {
		return t, false
	}
	t.Hidden = false
	return t, true
}

// CreateMap's phases (MR-008, RN-24, RN-10): the table's fog rule and the image copy
// are read before the transaction; inside it the key is checked, the campaign's cap
// counted and the map inserted.

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
	imageID, ok := uid.Canonical(req.Msg.GetImageId())
	if !ok {
		return nil, errNotAGalleryImage()
	}
	fogOn, err := s.fogOnNewMaps(ctx, m.CampaignID)
	if err != nil {
		return nil, err
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
	var used, replayed bool
	err = db.InTx(ctx, s.pool, func(tx pgx.Tx) error {
		var err error
		created, replayed, used, err = s.createMapTx(ctx, tx, newMap{
			campaignID: m.CampaignID, userID: m.UserID, name: name, imageID: imageID,
			fogOn: fogOn, reuse: reuse, key: scopedKey, hash: requestHash,
		})
		return err
	})
	s.discardFogCopy(ctx, m.CampaignID, reuse, err != nil || !used)
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

// fogOnNewMaps says whether the table's rules want the fog on new maps (RN-24; none
// by default). A new map has no grid, so it cannot have fog yet: the map is marked,
// and the fog comes on with its first grid (SetMapGrid).
func (s *Service) fogOnNewMaps(ctx context.Context, campaignID string) (bool, error) {
	if s.defaults == nil {
		return false, nil
	}
	on, err := s.defaults.FogOnNewMaps(ctx, nil, campaignID)
	if err != nil {
		return false, s.dbError(ctx, "read the table's rules", err)
	}
	return on, nil
}

// newMap is one CreateMap call as its transaction sees it.
type newMap struct {
	campaignID, userID, name, imageID string
	fogOn                             bool
	reuse                             *fogCopy
	key, hash                         *string // the scoped idempotency key and the request's hash
}

// createMapTx is CreateMap's transaction body: the map the key already made, or a
// new one. It tells whether it was a replay and whether the image copy was used.
func (s *Service) createMapTx(ctx context.Context, tx pgx.Tx, c newMap) (created mapsdb.Map, replayed, used bool, err error) {
	q := s.queries.WithTx(tx)
	created, replayed, err = idem.Create(ctx, c.key, c.hash, q.GetMapByCreateKey,
		func(row mapsdb.Map) *string { return row.CreateHash },
		func() (mapsdb.Map, error) {
			row, usedCopy, err := s.insertNewMap(ctx, q, c)
			used = usedCopy
			return row, err
		})
	return created, replayed, used, err
}

// insertNewMap counts the campaign's maps against the cap, checks the image, takes
// the copy made for it (the copy's gallery row exists only inside this attempt, and
// a retry checks the original again) and inserts the map. It returns pgx.ErrNoRows
// when the key's unique index already holds a row.
func (s *Service) insertNewMap(ctx context.Context, q *mapsdb.Queries, c newMap) (mapsdb.Map, bool, error) {
	count, err := q.CountMaps(ctx, c.campaignID)
	if err != nil {
		return mapsdb.Map{}, false, fmt.Errorf("count maps: %w", err)
	}
	if count >= s.maxMaps {
		return mapsdb.Map{}, false, connect.NewError(connect.CodeResourceExhausted, fmt.Errorf("the campaign already has %d maps", s.maxMaps))
	}
	mapImageID := c.imageID
	if err := checkImage(ctx, q, c.campaignID, mapImageID); err != nil {
		return mapsdb.Map{}, false, err
	}
	used := false
	if c.reuse != nil {
		if err := c.reuse.insert(ctx, q, s, c.campaignID, c.userID); err != nil {
			return mapsdb.Map{}, false, err
		}
		mapImageID, used = c.reuse.id, true
	}
	row, err := q.InsertMap(ctx, mapsdb.InsertMapParams{
		CampaignID: c.campaignID, Name: c.name, ImageID: mapImageID, FogOnFirstGrid: c.fogOn,
		CreateKey: c.key, CreateHash: c.hash, Now: s.now(),
	})
	if err != nil && !errors.Is(err, pgx.ErrNoRows) {
		return row, used, fmt.Errorf("insert map: %w", err)
	}
	return row, used, err
}

// DeleteMap's phases (MR-008): what the announcements need is read before the
// transaction; inside it the map is checked, the combat and the treasures that
// forbid the delete are refused and the map goes.

// DeleteMap implements mapsv1connect.MapServiceHandler.
func (s *Service) DeleteMap(
	ctx context.Context,
	req *connect.Request[mapsv1.DeleteMapRequest],
) (*connect.Response[mapsv1.DeleteMapResponse], error) {
	m, err := authz.RequireCampaignRole(ctx, req.Msg.GetCampaignId(), authz.RoleMaster)
	if err != nil {
		return nil, err
	}
	mapID, ok := uid.Canonical(req.Msg.GetMapId())
	if !ok {
		return nil, errMapNotFound()
	}
	pre, err := s.readDeleteContext(ctx, m.CampaignID)
	if err != nil {
		return nil, err
	}
	var deleted mapsdb.Map
	err = db.InTx(ctx, s.pool, func(tx pgx.Tx) error {
		var err error
		deleted, err = s.deleteMapRow(ctx, tx, m.CampaignID, mapID)
		return err
	})
	if err != nil {
		return nil, s.dbError(ctx, "delete a map", err)
	}
	s.announceDeletedMap(ctx, m.CampaignID, mapID, pre, deleted)
	return connect.NewResponse(&mapsv1.DeleteMapResponse{}), nil
}

// deleteContext is what DeleteMap reads before it deletes.
type deleteContext struct {
	current   string       // the session's current map
	links     campaignMaps // the links to the map, before they go, to tell the maps they leave
	openScene string       // a point of the map may be the open scene
}

func (s *Service) readDeleteContext(ctx context.Context, campaignID string) (deleteContext, error) {
	current, err := s.currentMap(ctx, campaignID)
	if err != nil {
		return deleteContext{}, err
	}
	links, err := s.loadMaps(ctx, campaignID)
	if err != nil {
		return deleteContext{}, s.dbError(ctx, "delete a map", err)
	}
	openScene, err := s.live.OpenScenePoint(ctx, campaignID)
	if err != nil {
		return deleteContext{}, s.dbError(ctx, "read the open scene", err)
	}
	return deleteContext{current: current, links: links, openScene: openScene}, nil
}

// deleteMapRow is DeleteMap's transaction. The map a combat that has not ended runs
// on cannot go: the fight stands on its layers, and the fog of war's filter on the
// combat's map link. A map holding a treasure that was found or turned into XP
// cannot go either: the treasure is part of the session's record (unmark it first).
func (s *Service) deleteMapRow(ctx context.Context, tx pgx.Tx, campaignID, mapID string) (mapsdb.Map, error) {
	q := s.queries.WithTx(tx)
	if _, err := s.campaignMap(ctx, q, campaignID, mapID); err != nil {
		return mapsdb.Map{}, err
	}
	if err := s.refuseWhileCombat(ctx, tx, campaignID, mapID); err != nil {
		return mapsdb.Map{}, err
	}
	locks, err := q.GetMapTreasureLocks(ctx, mapID)
	if err != nil {
		return mapsdb.Map{}, fmt.Errorf("read the map's treasures: %w", err)
	}
	if locks.Converted > 0 {
		return mapsdb.Map{}, errTreasureConverted()
	}
	if locks.Found > 0 {
		return mapsdb.Map{}, errTreasureFound()
	}
	deleted, err := q.DeleteMap(ctx, mapsdb.DeleteMapParams{CampaignID: campaignID, ID: mapID})
	if errors.Is(err, pgx.ErrNoRows) {
		return deleted, errMapNotFound()
	}
	if err != nil {
		return deleted, fmt.Errorf("delete map: %w", err)
	}
	return deleted, nil
}

// announceDeletedMap forgets what depended on the map and tells the watchers: the
// map, the maps that linked to it, the session if it was the current map, and the
// scene if it was one of the map's points.
func (s *Service) announceDeletedMap(ctx context.Context, campaignID, mapID string, pre deleteContext, deleted mapsdb.Map) {
	s.lits.forget(mapID)
	s.tiles.forget(mapID)
	s.seen.forget(mapID)
	seen := playersSee(mapID, deleted.RevealedAt, pre.current)
	s.publishMapChanged(campaignID, mapID, seen)
	s.publishParentsChanged(campaignID, mapID, pre.links, pre.current, seen, false)
	if mapID == pre.current {
		// The foreign key unset the session's current map.
		s.publishCurrentMapCleared(campaignID)
	}
	if pre.openScene != "" {
		// The map's points went with it: if the scene was one of them, the
		// foreign key closed it.
		if still, err := s.live.OpenScenePoint(ctx, campaignID); err == nil && still == "" {
			s.publishSceneChanged(ctx, campaignID)
		}
	}
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
	mapID, ok := uid.Canonical(req.Msg.GetMapId())
	if !ok {
		return nil, errMapNotFound()
	}
	current, err := s.currentMap(ctx, m.CampaignID)
	if err != nil {
		return nil, err
	}

	var before, after mapsdb.Map
	err = db.InTx(ctx, s.pool, func(tx pgx.Tx) error {
		var err error
		before, after, err = s.revealMapRow(ctx, tx, m, mapID, req.Msg.GetRevealed())
		return err
	})
	if err != nil {
		return nil, s.dbError(ctx, "reveal or hide a map", err)
	}
	s.announceRevealedMap(ctx, m.CampaignID, mapID, current, before, after)
	out, err := s.masterMap(ctx, m, mapID)
	if err != nil {
		return nil, err
	}
	return connect.NewResponse(&mapsv1.SetMapRevealedResponse{Map: out}), nil
}

// revealMapRow is SetMapRevealed's transaction: it locks the map, and returns the
// row as it was and as it is.
func (s *Service) revealMapRow(ctx context.Context, tx pgx.Tx, m authz.Membership, mapID string, revealed bool) (before, after mapsdb.Map, err error) {
	q := s.queries.WithTx(tx)
	if before, err = lockMap(ctx, q, m.CampaignID, mapID); err != nil {
		return before, after, err
	}
	after, err = q.SetMapRevealed(ctx, mapsdb.SetMapRevealedParams{CampaignID: m.CampaignID, ID: mapID, Revealed: revealed, Now: s.now()})
	if err != nil {
		return before, after, fmt.Errorf("set map revealed: %w", err)
	}
	return before, after, nil
}

// announceRevealedMap tells the watchers of a map that was revealed or hidden, and
// the maps that link to it.
func (s *Service) announceRevealedMap(ctx context.Context, campaignID, mapID, current string, before, after mapsdb.Map) {
	seenBefore, seenAfter := playersSee(mapID, before.RevealedAt, current), playersSee(mapID, after.RevealedAt, current)
	s.publishMapChanged(campaignID, mapID, seenBefore || seenAfter)
	if seenAfter && !seenBefore {
		s.refreshVision(ctx, campaignID, mapID) // the first view the players have of a fog map is recorded now
	}
	cm, err := s.loadMaps(ctx, campaignID)
	if err != nil {
		// The change is made; only a hint to other maps is lost, and the
		// app reads the maps again after any reconnection.
		s.logger.ErrorContext(ctx, "maps: cannot tell the parent maps about a revealed map", "error", err)
		return
	}
	s.publishParentsChanged(campaignID, mapID, cm, current, seenBefore, seenAfter)
}
