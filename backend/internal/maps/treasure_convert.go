package maps

import (
	"context"
	"fmt"
	"slices"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/PuraFome/meuRPG/backend/internal/platform/db"
	"github.com/PuraFome/meuRPG/backend/internal/progression/link"
)

// What the progression and play modules ask of the treasures (MR-041, MR-032): the found ones to turn into XP ("Voltar à cidade"), and the PO
// found in a session. Plain SQL, not sqlc, so this file adds nothing to the
// generated queries the other slices share. The types are small and pool-only,
// like SessionMaps: they need no Service.

// Treasures is what package progression needs from the maps: it implements
// progression.Treasures. A treasure is converted by setting
// map_points.treasure_converted_award_id inside the award's transaction; the
// maps' own calls then refuse to unmark or change it (errTreasureConverted).
type Treasures struct {
	pool *pgxpool.Pool
	// svc tells the watching members a map changed. Connected by SetService;
	// nil until then, and PublishChanged then does nothing.
	svc *Service
}

// SetService connects the maps service, which is made after the progression
// module needs this (see cmd/api).
func (t *Treasures) SetService(s *Service) { t.svc = s }

// PublishChanged tells the watching members that the points of those maps
// changed state (converted into XP, or freed by an undo). Call it after the
// transaction committed. Only the master reads a point's converted state, so
// only the master is told: a player's view of the map did not change.
func (t *Treasures) PublishChanged(campaignID string, mapIDs []string) {
	if t.svc == nil {
		return
	}
	for _, id := range mapIDs {
		t.svc.publishMapChanged(context.Background(), campaignID, id, false)
	}
}

// NewTreasures returns the Treasures for progression.
func NewTreasures(pool *pgxpool.Pool) *Treasures { return &Treasures{pool: pool} }

// treasureSelect is what a link.Treasure is read from; the WHERE of each query
// follows it.
const treasureSelect = `
	SELECT p.id::TEXT, p.map_id::TEXT, m.name, p.name, COALESCE(p.treasure_value_po, 0),
	       p.treasure_found_at, p.treasure_session_id IS NOT NULL, p.treasure_converted_award_id::TEXT
	FROM map_points AS p
	JOIN maps AS m ON m.id = p.map_id`

// ListUnconverted returns the campaign's treasures that were found and no
// award converted, the oldest find first.
func (t *Treasures) ListUnconverted(ctx context.Context, campaignID string) ([]link.Treasure, error) {
	// The treasures and their finders are one moment: a treasure unmarked between
	// the two reads would be listed as found, with nobody who found it.
	var out []link.Treasure
	err := db.ReadTx(ctx, t.pool, func(tx pgx.Tx) error {
		rows, err := tx.Query(ctx, treasureSelect+`
		WHERE m.campaign_id = $1 AND p.kind = 'treasure'
		  AND p.treasure_found_at IS NOT NULL AND p.treasure_converted_award_id IS NULL
		ORDER BY p.treasure_found_at, p.id`, campaignID)
		if err != nil {
			return fmt.Errorf("list the treasures to convert: %w", err)
		}
		out, err = t.collect(ctx, tx, rows)
		return err
	})
	return out, err
}

// LockForConversion returns, inside tx, those of pointIDs that are treasures of
// the campaign, with their rows locked until tx ends: two awards that race for
// the same treasure take turns, and the second finds it converted. A point that
// is not a treasure of the campaign is left out of the answer. The caller
// checks Found and ConvertedAwardID.
func (t *Treasures) LockForConversion(ctx context.Context, tx pgx.Tx, campaignID string, pointIDs []string) ([]link.Treasure, error) {
	rows, err := tx.Query(ctx, treasureSelect+`
		WHERE m.campaign_id = $1 AND p.kind = 'treasure' AND p.id = ANY($2::UUID[])
		ORDER BY p.id
		FOR UPDATE OF p`, campaignID, pointIDs)
	if err != nil {
		return nil, fmt.Errorf("lock the treasures: %w", err)
	}
	return t.collect(ctx, tx, rows)
}

// MarkConverted links the treasures to the award inside tx. Call it after
// LockForConversion has checked them.
func (t *Treasures) MarkConverted(ctx context.Context, tx pgx.Tx, awardID string, pointIDs []string) error {
	tag, err := tx.Exec(ctx, `
		UPDATE map_points SET treasure_converted_award_id = $1::UUID
		WHERE id = ANY($2::UUID[]) AND kind = 'treasure'
		  AND treasure_found_at IS NOT NULL AND treasure_converted_award_id IS NULL`, awardID, pointIDs)
	if err != nil {
		return fmt.Errorf("convert the treasures: %w", err)
	}
	if tag.RowsAffected() != int64(len(pointIDs)) {
		return fmt.Errorf("convert the treasures: %d of %d changed", tag.RowsAffected(), len(pointIDs))
	}
	return nil
}

// Release frees the treasures the award converted inside tx (the award was
// undone): they are "found, not converted" again. It returns the IDs of the
// maps whose points it freed, each once.
func (t *Treasures) Release(ctx context.Context, tx pgx.Tx, awardID string) ([]string, error) {
	rows, err := tx.Query(ctx, `
		UPDATE map_points SET treasure_converted_award_id = NULL
		WHERE treasure_converted_award_id = $1::UUID
		RETURNING map_id::TEXT`, awardID)
	if err != nil {
		return nil, fmt.Errorf("free the treasures: %w", err)
	}
	defer rows.Close()
	var mapIDs []string
	for rows.Next() {
		var id string
		if err := rows.Scan(&id); err != nil {
			return nil, fmt.Errorf("read a freed treasure's map: %w", err)
		}
		if !slices.Contains(mapIDs, id) {
			mapIDs = append(mapIDs, id)
		}
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("free the treasures: %w", err)
	}
	slices.Sort(mapIDs)
	return mapIDs, nil
}

// querier is what pool and tx both do.
type querier interface {
	Query(ctx context.Context, sql string, args ...any) (pgx.Rows, error)
}

// collect reads the treasure rows, then their finders.
func (t *Treasures) collect(ctx context.Context, q querier, rows pgx.Rows) ([]link.Treasure, error) {
	defer rows.Close()
	var out []link.Treasure
	var ids []string
	for rows.Next() {
		var tr link.Treasure
		var foundAt *time.Time
		var converted *string
		if err := rows.Scan(&tr.PointID, &tr.MapID, &tr.MapName, &tr.Name, &tr.ValuePO, &foundAt, &tr.InSession, &converted); err != nil {
			return nil, fmt.Errorf("read a treasure: %w", err)
		}
		if foundAt != nil {
			tr.Found, tr.FoundAt = true, *foundAt
		}
		if converted != nil {
			tr.ConvertedAwardID = *converted
		}
		out = append(out, tr)
		ids = append(ids, tr.PointID)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("read the treasures: %w", err)
	}
	rows.Close() // a connection runs one query at a time: free it for the finders
	if len(out) == 0 {
		return out, nil
	}
	finders, err := q.Query(ctx, `
		SELECT f.point_id::TEXT, f.character_id::TEXT
		FROM map_treasure_finders AS f
		WHERE f.point_id = ANY($1::UUID[])
		ORDER BY f.character_id`, ids)
	if err != nil {
		return nil, fmt.Errorf("read the treasures' finders: %w", err)
	}
	defer finders.Close()
	byPoint := map[string][]string{}
	for finders.Next() {
		var point, character string
		if err := finders.Scan(&point, &character); err != nil {
			return nil, fmt.Errorf("read a finder: %w", err)
		}
		byPoint[point] = append(byPoint[point], character)
	}
	if err := finders.Err(); err != nil {
		return nil, fmt.Errorf("read the finders: %w", err)
	}
	for i := range out {
		out[i].FinderIDs = byPoint[out[i].PointID]
	}
	return out, nil
}

// TreasureFoundIn returns the gold pieces each character found in the game
// session, by character ID (MR-032, "Mais tesouro encontrado"): each treasure
// whose treasure_session_id is the session, its value split among its finders
// and rounded down. A treasure unmarked later has no session and no finders, so
// it counts for nothing. It implements play.MapKeeper.
func (sm *SessionMaps) TreasureFoundIn(ctx context.Context, sessionID string) (map[string]int32, error) {
	rows, err := sm.queries.ListTreasureFindsOfSession(ctx, sessionID)
	if err != nil {
		return nil, fmt.Errorf("read the treasures found in the session: %w", err)
	}
	type find struct {
		value      int32
		characters []string
	}
	finds := map[string]*find{}
	for _, r := range rows {
		f := finds[r.PointID]
		if f == nil {
			f = &find{value: r.ValuePo}
			finds[r.PointID] = f
		}
		f.characters = append(f.characters, r.CharacterID)
	}
	out := map[string]int32{}
	for _, f := range finds {
		share := f.value / int32(len(f.characters)) //nolint:gosec // at most a handful of finders
		for _, c := range f.characters {
			out[c] += share
		}
	}
	return out, nil
}
