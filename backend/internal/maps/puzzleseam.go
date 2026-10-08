package maps

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"connectrpc.com/connect"
	"github.com/jackc/pgx/v5"

	mapsv1 "github.com/PuraFome/meuRPG/backend/gen/meurpg/maps/v1"
	playv1 "github.com/PuraFome/meuRPG/backend/gen/meurpg/play/v1"
	"github.com/PuraFome/meuRPG/backend/internal/authz"
	"github.com/PuraFome/meuRPG/backend/internal/maps/mapsdb"
	"github.com/PuraFome/meuRPG/backend/internal/rules/grid"
)

// What a solved puzzle does to the maps (MR-038, RN-27, "Ao resolver"): open a
// door, reveal a point, reveal a clue to whoever solved it. The play module owns
// the puzzles and calls these inside the transaction of the winning move
// (play.PuzzleMaps); this module owns the doors, the points and the clues. Like
// OpenDoors they take no caller and check nobody: they run after play's own
// authorization. Each Check* answers `not_found` for something that is not the
// campaign's. Each action that changes something returns a function to call after
// the commit, which tells the watchers (never inside the transaction: it reads
// through the pool).

// PuzzleCheckDoor says, inside tx, whether the square of the campaign's map
// has a door (a door of any state): the target a master picks for "Abrir uma
// porta". `not_found` for a map that is not the campaign's, a square outside it
// or a square with no door.
func (s *Service) PuzzleCheckDoor(ctx context.Context, tx pgx.Tx, campaignID, mapID string, col, row int) error {
	dm, err := s.puzzleDoors(ctx, tx, campaignID, mapID)
	if err != nil {
		return err
	}
	sq := grid.Square{Col: col, Row: row}
	if !dm.grid.Contains(sq) || dm.set.doors.At(sq) == grid.DoorNone {
		return errors.Join(errPuzzleTarget, connect.NewError(connect.CodeNotFound, errors.New("door not found")))
	}
	return nil
}

// errPuzzleTarget marks the errors of this file that mean "that target is not
// there"; the others are the database's.
var errPuzzleTarget = errors.New("maps: the puzzle's target is not there")

// PuzzleOpenDoor opens, inside tx, the door of the map at the square, whatever it
// was: closed, locked, barred or secret (the puzzle is the key). It reports false,
// changing nothing, when the door is open already or gone. A door opened raises
// the layers' revision, as a move that opens one does. Call DoorsChanged after the
// commit when it opened one.
func (s *Service) PuzzleOpenDoor(ctx context.Context, tx pgx.Tx, campaignID, mapID string, col, row int) (bool, error) {
	dm, err := s.puzzleDoors(ctx, tx, campaignID, mapID)
	if err != nil {
		return false, err
	}
	set, id := dm.set, dm.id
	sq := grid.Square{Col: col, Row: row}
	if !dm.grid.Contains(sq) {
		return false, nil
	}
	switch set.doors.At(sq) {
	case grid.DoorClosed, grid.DoorLocked, grid.DoorBarred, grid.DoorSecret:
	default:
		return false, nil
	}
	// The door is the whole square of the drawing (RN-25): on a calibrated map every
	// door square of the block opens, as walking through it does (OpenDoors).
	f := dm.factor
	bc, br := sq.Col/f*f, sq.Row/f*f
	for r := br; r < br+f; r++ {
		for c := bc; c < bc+f; c++ {
			if set.doors.Get(c, r) != grid.DoorNone {
				set.doors.Set(c, r, grid.DoorOpen)
			}
		}
	}
	q := queriesIn(s.queries, tx)
	err = q.UpsertMapLayers(ctx, mapsdb.UpsertMapLayersParams{
		MapID: id, DifficultTerrain: nilIfBlank(set.terrain.Encode()), Walls: nilIfBlank(set.walls.Encode()),
		Cover: nilIfBlank(set.cover.Encode()), Light: nilIfBlank(set.light.Encode()), Doors: nilIfBlank(set.doors.Encode()), Now: s.now(),
	})
	if err != nil {
		return false, fmt.Errorf("write the layers: %w", err)
	}
	if _, err := q.BumpMapLayersRevision(ctx, id); err != nil {
		return false, fmt.Errorf("bump the layers' revision: %w", err)
	}
	return true, nil
}

// puzzleDoors locks the campaign's map inside tx (as painting and OpenDoors do, so
// the three take turns) and returns its decoded layers, its grid and its ID. A map
// with no grid or no layers painted has no door: the layers are empty.
func (s *Service) puzzleDoors(ctx context.Context, tx pgx.Tx, campaignID, mapID string) (doorMap, error) {
	id, ok := parseID(mapID)
	if !ok {
		return doorMap{}, errors.Join(errPuzzleTarget, errMapNotFound())
	}
	q := queriesIn(s.queries, tx)
	if _, err := q.GetMapForUpdate(ctx, mapsdb.GetMapForUpdateParams{CampaignID: campaignID, ID: id}); errors.Is(err, pgx.ErrNoRows) {
		return doorMap{}, errors.Join(errPuzzleTarget, errMapNotFound())
	} else if err != nil {
		return doorMap{}, fmt.Errorf("lock the map: %w", err)
	}
	size, err := q.GetMapGrid(ctx, mapsdb.GetMapGridParams{CampaignID: campaignID, ID: id})
	if err != nil {
		return doorMap{}, fmt.Errorf("read the map's grid: %w", err)
	}
	g := gridOf(size.GridColumns, size.GridFactor, size.ImageWidth, size.ImageHeight)
	dm := doorMap{grid: g, factor: max(int(size.GridFactor), 1), id: id}
	if !g.Valid() {
		dm.set = layerSet{doors: grid.NewDoorLayer(g)}
		return dm, nil
	}
	stored, err := q.GetMapLayers(ctx, id)
	if errors.Is(err, pgx.ErrNoRows) {
		dm.set = loadLayers(mapsdb.MapLayer{}, g)
		return dm, nil
	}
	if err != nil {
		return doorMap{}, fmt.Errorf("read the layers: %w", err)
	}
	dm.set = loadLayers(stored, g)
	return dm, nil
}

// doorMap is what puzzleDoors reads: the map's decoded layers, its grid, its
// calibration factor (a door is a whole square of the drawing, factor x factor
// squares of the rules, RN-25) and its ID.
type doorMap struct {
	set    layerSet
	grid   grid.Grid
	factor int
	id     string
}

// PuzzleCheckPoint says, inside tx, whether the point is on the campaign's map and is
// one a puzzle can reveal: a light is never shown to the players, so it is not a
// target.
func (s *Service) PuzzleCheckPoint(ctx context.Context, tx pgx.Tx, campaignID, mapID, pointID string) error {
	_, point, _, err := s.puzzlePoint(ctx, queriesIn(s.queries, tx), campaignID, mapID, pointID, false)
	if err != nil {
		return err
	}
	if point.Kind == kindToDB[mapsv1.MapPointKind_MAP_POINT_KIND_LIGHT] {
		return errors.Join(errPuzzleTarget, errPointNotFound())
	}
	return nil
}

// puzzlePoint finds the point of the campaign's map, locking it when asked, and
// returns the map's row and the point as they are.
func (s *Service) puzzlePoint(ctx context.Context, q *mapsdb.Queries, campaignID, mapID, pointID string, lock bool) (mapsdb.Map, mapsdb.MapPoint, string, error) {
	mid, ok := parseID(mapID)
	pid, ok2 := parseID(pointID)
	if !ok || !ok2 {
		return mapsdb.Map{}, mapsdb.MapPoint{}, "", errors.Join(errPuzzleTarget, errPointNotFound())
	}
	row, err := q.GetMap(ctx, mapsdb.GetMapParams{CampaignID: campaignID, ID: mid})
	if errors.Is(err, pgx.ErrNoRows) {
		return mapsdb.Map{}, mapsdb.MapPoint{}, "", errors.Join(errPuzzleTarget, errMapNotFound())
	}
	if err != nil {
		return mapsdb.Map{}, mapsdb.MapPoint{}, "", fmt.Errorf("find map: %w", err)
	}
	var point mapsdb.MapPoint
	if lock {
		point, err = q.GetMapPointForUpdate(ctx, mapsdb.GetMapPointForUpdateParams{MapID: mid, ID: pid})
	} else {
		point, err = q.GetMapPoint(ctx, mapsdb.GetMapPointParams{MapID: mid, ID: pid})
	}
	if errors.Is(err, pgx.ErrNoRows) {
		return mapsdb.Map{}, mapsdb.MapPoint{}, "", errors.Join(errPuzzleTarget, errPointNotFound())
	}
	if err != nil {
		return mapsdb.Map{}, mapsdb.MapPoint{}, "", fmt.Errorf("find point: %w", err)
	}
	return row, point, mid, nil
}

// PuzzleRevealPoint reveals the map's point to the players inside tx, as
// SetMapPointRevealed does, and returns its name for the line the players read, when
// they see the point now: currentMapID is the map the session shows, and a point of a
// map the players cannot open keeps its name to itself (RN-10); name is "" then. It
// reports false, changing nothing, when the point was revealed already. The returned
// function, called after the commit, tells the map's watchers.
func (s *Service) PuzzleRevealPoint(ctx context.Context, tx pgx.Tx, campaignID, mapID, pointID, currentMapID string) (changed bool, name string, after func(context.Context), err error) {
	q := queriesIn(s.queries, tx)
	mapRow, before, _, err := s.puzzlePoint(ctx, q, campaignID, mapID, pointID, true)
	if err != nil {
		return false, "", nil, err
	}
	if before.RevealedAt != nil {
		return false, before.Name, nil, nil
	}
	revealed := true
	updated, err := s.applyPointChange(ctx, q, campaignID, before, pointChange{revealed: &revealed})
	if err != nil {
		return false, "", nil, err
	}
	if playersSee(mapRow.ID, mapRow.RevealedAt, currentMapID) && everyoneSees(updated) {
		name = updated.Name
	}
	return true, name, func(ctx context.Context) {
		current, err := s.currentMap(ctx, campaignID)
		if err != nil {
			s.logger.ErrorContext(ctx, "maps: read the current map after a puzzle revealed a point", "error", err)
			return
		}
		s.publishPointsChanged(ctx, campaignID, mapRow,
			playersSee(mapRow.ID, mapRow.RevealedAt, current) && (everyoneSees(before) || everyoneSees(updated)), before, updated)
	}, nil
}

// PuzzleCheckClue says, inside tx, whether the clue is on a scene point of the
// campaign's maps.
func (s *Service) PuzzleCheckClue(ctx context.Context, tx pgx.Tx, campaignID, clueID string) error {
	id, ok := parseID(clueID)
	if !ok {
		return errors.Join(errPuzzleTarget, errClueNotFound())
	}
	if _, err := queriesIn(s.queries, tx).GetSceneClueInCampaign(ctx, mapsdb.GetSceneClueInCampaignParams{CampaignID: campaignID, ID: id}); errors.Is(err, pgx.ErrNoRows) {
		return errors.Join(errPuzzleTarget, errClueNotFound())
	} else if err != nil {
		return fmt.Errorf("find clue: %w", err)
	}
	return nil
}

// PuzzleRevealClue gives the clue inside tx to one player's character, as the
// master's reveal does: the player's notes get it, and the session's history
// records it (`clue_revealed`, by the player who solved the puzzle). It reports
// false, changing nothing, when the player had it already. The returned function,
// called after the commit, tells the player's stream and the master's scene.
func (s *Service) PuzzleRevealClue(ctx context.Context, tx pgx.Tx, campaignID, clueID, characterID, userID string, at time.Time) (gave bool, after func(context.Context), err error) {
	id, ok := parseID(clueID)
	if !ok {
		return false, nil, errors.Join(errPuzzleTarget, errClueNotFound())
	}
	q := queriesIn(s.queries, tx)
	clue, err := q.GetSceneClueInCampaign(ctx, mapsdb.GetSceneClueInCampaignParams{CampaignID: campaignID, ID: id})
	if errors.Is(err, pgx.ErrNoRows) {
		return false, nil, errors.Join(errPuzzleTarget, errClueNotFound())
	}
	if err != nil {
		return false, nil, fmt.Errorf("find clue: %w", err)
	}
	n, err := q.InsertClueReveal(ctx, mapsdb.InsertClueRevealParams{
		CampaignID: campaignID, ClueID: &clue.ID, PointID: &clue.PointID, UserID: userID,
		CharacterID: &characterID, Text: clue.Text, Now: at,
	})
	if err != nil {
		return false, nil, fmt.Errorf("reveal the clue: %w", err)
	}
	if n == 0 {
		return false, nil, nil
	}
	payload, err := json.Marshal(clueRevealedEvent{ClueID: clue.ID, PointID: clue.PointID, CharacterIDs: []string{characterID}})
	if err != nil {
		return false, nil, fmt.Errorf("encode the event payload: %w", err)
	}
	if _, err := s.live.AppendEvent(ctx, tx, campaignID, eventClueRevealed, userID, payload, at); err != nil {
		return false, nil, fmt.Errorf("record the reveal: %w", err)
	}
	return true, func(ctx context.Context) {
		s.live.PublishToUsers(campaignID, []string{userID}, &playv1.WatchGameSessionResponse{Event: &playv1.WatchGameSessionResponse_NotesChanged_{
			NotesChanged: &playv1.WatchGameSessionResponse_NotesChanged{},
		}})
		s.publishSceneChangedToMasterIf(ctx, campaignID, clue.PointID)
	}, nil
}

// PuzzleClueFoundBy says whether the player has found the clue (it is in their
// notes): the cipher puzzle tells a player where its key is only then. A clue that is
// gone, or not the campaign's, is not found, and not an error: the puzzle still reads.
func (s *Service) PuzzleClueFoundBy(ctx context.Context, tx pgx.Tx, campaignID, clueID, userID string) (bool, error) {
	id, ok := parseID(clueID)
	if !ok {
		return false, nil
	}
	found, err := queriesIn(s.queries, tx).HasClueReveal(ctx, mapsdb.HasClueRevealParams{CampaignID: campaignID, ClueID: &id, UserID: userID})
	if err != nil {
		return false, fmt.Errorf("find whether the player has the clue: %w", err)
	}
	return found, nil
}

// PuzzleTrapSeenBy says whether the caller sees the trap point as a map read gives it to
// them (RN-10): the point's map is on their screen (the current one, or revealed), a trap
// that fired is public, and on a map with the fog of war the player sees or remembers its
// square. The master sees everything. A point that is gone is not seen. It reads through
// the pool.
func (s *Service) PuzzleTrapSeenBy(ctx context.Context, m authz.Membership, mapID, pointID string) (bool, error) {
	mid, ok1 := parseID(mapID)
	pid, ok2 := parseID(pointID)
	if !ok1 || !ok2 {
		return false, nil
	}
	v, err := s.viewerOf(ctx, m)
	if err != nil {
		return false, err
	}
	if v.master {
		return true, nil
	}
	cm, err := s.loadMaps(ctx, m.CampaignID)
	if err != nil {
		return false, s.dbError(ctx, "read the maps", err)
	}
	row, found := cm.byID[mid]
	if !found || !v.seesMap(row.ID, row.RevealedAt) {
		return false, nil
	}
	p, err := s.queries.GetMapPoint(ctx, mapsdb.GetMapPointParams{MapID: mid, ID: pid})
	if errors.Is(err, pgx.ErrNoRows) {
		return false, nil
	}
	if err != nil {
		return false, fmt.Errorf("find the trap point: %w", err)
	}
	if foggedFor(v, row) {
		pv, _, _, err := s.fogViewOf(ctx, row, v)
		if err != nil {
			return false, s.dbError(ctx, "work out what a player sees", err)
		}
		return len(s.visiblePoints([]mapsdb.MapPoint{p}, v, pv)) == 1, nil
	}
	return v.seesPoint(p), nil
}
