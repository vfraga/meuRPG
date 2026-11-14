package maps

import (
	"context"
	"errors"
	"fmt"

	"connectrpc.com/connect"
	"github.com/jackc/pgx/v5"

	mapsv1 "github.com/PuraFome/meuRPG/backend/gen/meurpg/maps/v1"
	"github.com/PuraFome/meuRPG/backend/internal/authz"
	"github.com/PuraFome/meuRPG/backend/internal/maps/mapsdb"
	"github.com/PuraFome/meuRPG/backend/internal/platform/sessiontell"
	"github.com/PuraFome/meuRPG/backend/internal/platform/uid"
	"github.com/PuraFome/meuRPG/backend/internal/rules/grid"
)

// The battle grid's limits (MR-013): how many squares of the drawing fit across
// the image's width. The maps_grid_columns_valid CHECK says the same of the
// rules' columns, which are at most 200 as well (the calibration multiplies the
// drawn columns, and rules/grid.EngineGrid refuses what passes the limits).
const (
	minGridColumns = 4
	maxGridColumns = grid.MaxColumns // 200, the rules' limit too
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
	mapID, ok := uid.Canonical(req.Msg.GetMapId())
	if !ok {
		return nil, errMapNotFound()
	}
	drawn := req.Msg.GetColumns()
	factor, err := checkGridRequest(drawn, req.Msg.GetSquareFactor())
	if err != nil {
		return nil, err
	}
	current, err := s.currentMap(ctx, m.CampaignID)
	if err != nil {
		return nil, err
	}
	imageCopy, err := s.gridFogCopy(ctx, m.CampaignID, mapID, drawn)
	if err != nil {
		return nil, err
	}
	var w gridWrite
	err = s.inSession(ctx, m.CampaignID, func(tx pgx.Tx, t sessiontell.Tell) error {
		var err error
		if w, err = s.writeGrid(ctx, tx, gridChange{campaignID: m.CampaignID, userID: m.UserID, mapID: mapID, drawn: drawn, factor: factor, imageCopy: imageCopy}); err != nil {
			return err
		}
		s.announce(t, func(ctx context.Context) { s.announceGrid(ctx, m, mapID, current, w) })
		return nil
	})
	s.discardFogCopy(ctx, m.CampaignID, imageCopy, err != nil || !w.copied)
	if err != nil {
		return nil, s.dbError(ctx, "set a map's grid", err)
	}
	out, err := s.masterMap(ctx, m, mapID)
	if err != nil {
		return nil, err
	}
	return connect.NewResponse(&mapsv1.SetMapGridResponse{Map: out}), nil
}

// checkGridRequest refuses the columns and the square factor a request may not
// carry, columns first, and returns the factor with 0 read as 1.
func checkGridRequest(drawn, factor int32) (int32, error) {
	if drawn != 0 && (drawn < minGridColumns || drawn > maxGridColumns) {
		return 0, connect.NewError(connect.CodeInvalidArgument,
			fmt.Errorf("columns must be 0 (no grid) or %d to %d", minGridColumns, maxGridColumns))
	}
	if factor < 0 || factor > grid.MaxFactor || (drawn == 0 && factor > 1) {
		return 0, connect.NewError(connect.CodeInvalidArgument,
			fmt.Errorf("square_factor must be 1 to %d (0 reads as 1), and only with columns", grid.MaxFactor))
	}
	return max(factor, 1), nil
}

// gridFogCopy makes the copy of the image a map takes when its first grid brings
// the table's fog (RN-24): the fog comes on, and the raw image never reaches the
// players (RN-10), so an image that serves another use is copied first, as
// SetMapFog does. It is nil when the call sets no grid, the map is not marked for
// the rule or already has a grid, or nothing else uses the image.
func (s *Service) gridFogCopy(ctx context.Context, campaignID, mapID string, drawn int32) (*fogCopy, error) {
	if drawn == 0 {
		return nil, nil
	}
	imageCopy, err := s.fogCopyOf(ctx, campaignID, mapID, func(m mapsdb.Map) bool { return m.FogOnFirstGrid && m.GridColumns == nil })
	if err != nil {
		return nil, s.dbError(ctx, "copy the map's image for the fog", err)
	}
	return imageCopy, nil
}

// gridChange is one SetMapGrid call as its transaction sees it.
type gridChange struct {
	campaignID, userID, mapID string
	drawn, factor             int32 // the drawn columns (0 for no grid) and the square factor, 0 already read as 1
	imageCopy                 *fogCopy
}

// gridWrite is what the transaction did.
type gridWrite struct {
	updated mapsdb.Map
	changed bool // the layers were cleared or scaled
	copied  bool // the fog's image copy was used
}

// writeGrid is SetMapGrid's transaction: every decision is made from the row
// locked here, so a combat that starts meanwhile is seen.
func (s *Service) writeGrid(ctx context.Context, tx pgx.Tx, c gridChange) (gridWrite, error) {
	q := s.queries.WithTx(tx)
	before, size, err := lockForGrid(ctx, q, c.campaignID, c.mapID)
	if err != nil {
		return gridWrite{}, err
	}
	engine, err := engineGridFor(c.drawn, c.factor, size)
	if err != nil {
		return gridWrite{}, err
	}
	plan := planGridLayers(before, size, engine, c)
	if !plan.same {
		if err := s.refuseWhileCombat(ctx, tx, c.campaignID, c.mapID); err != nil {
			return gridWrite{}, err
		}
	}
	var w gridWrite
	if w.updated, err = s.storeGrid(ctx, q, c, engine); err != nil {
		return gridWrite{}, err
	}
	if w.changed, err = s.settleLayers(ctx, q, c.mapID, before, plan); err != nil {
		return gridWrite{}, err
	}
	if c.drawn != 0 && before.FogOnFirstGrid && before.GridColumns == nil && !before.FogEnabled {
		if w.updated, w.copied, err = s.applyFirstGridFog(ctx, q, c, before); err != nil {
			return gridWrite{}, err
		}
	}
	return w, nil
}

// lockForGrid locks the map's row and reads its image's size.
func lockForGrid(ctx context.Context, q *mapsdb.Queries, campaignID, mapID string) (mapsdb.Map, mapsdb.GetMapGridRow, error) {
	before, err := lockMap(ctx, q, campaignID, mapID)
	if err != nil {
		return before, mapsdb.GetMapGridRow{}, err
	}
	size, err := q.GetMapGrid(ctx, mapsdb.GetMapGridParams{CampaignID: campaignID, ID: mapID})
	if err != nil {
		return before, size, fmt.Errorf("read the map's image size: %w", err)
	}
	return before, size, nil
}

// engineGridFor is the grid the rules will use for a drawing of drawn columns
// over the image: zero for no grid, an error for one past the limits.
func engineGridFor(drawn, factor int32, size mapsdb.GetMapGridRow) (grid.Grid, error) {
	if drawn == 0 {
		return grid.Grid{}, nil
	}
	engine, err := grid.EngineGrid(int(drawn), int(factor), int(size.ImageWidth), int(size.ImageHeight))
	if err != nil {
		return grid.Grid{}, connect.NewError(connect.CodeInvalidArgument, fmt.Errorf(
			"%d columns of %d squares each make a grid that passes the limit of %d x %d squares",
			drawn, factor, grid.MaxColumns, grid.MaxRows))
	}
	return engine, nil
}

// layerPlan is what happens to the painted layers when the grid changes.
type layerPlan struct {
	old    grid.Grid // the grid the layers are sized for now
	same   bool      // the rules' grid does not change: nothing painted needs to change
	scales bool      // the same drawing at a factor that is a multiple of the old one
	step   int       // how many squares a square becomes, when it scales
}

// planGridLayers decides what happens to the layers (D2, D3). The same grid and
// factor again change nothing; the same drawing with a factor that is a multiple
// of the old one is scaled; anything else clears the layers, which a fight
// standing on them cannot lose. The grid is the same when the rules' columns and
// rows are: 24 drawn columns of 1,5 m and 12 of 3 m make the same grid (when the
// image's proportions agree), and nothing painted needs to change.
func planGridLayers(before mapsdb.Map, size mapsdb.GetMapGridRow, engine grid.Grid, c gridChange) layerPlan {
	old := gridOf(before.GridColumns, before.GridFactor, size.ImageWidth, size.ImageHeight)
	step, scales := grid.ScaleStep(int(before.GridFactor), int(c.factor))
	scales = scales && c.drawn != 0 && before.GridColumns != nil && c.drawn == *before.GridColumns/before.GridFactor
	return layerPlan{old: old, same: old == engine, scales: scales, step: step}
}

// storeGrid writes the grid, which is NULL when the call clears it.
func (s *Service) storeGrid(ctx context.Context, q *mapsdb.Queries, c gridChange, engine grid.Grid) (mapsdb.Map, error) {
	var gridColumns *int32 // NULL clears the grid
	if c.drawn != 0 {
		engineColumns := int32(engine.Columns) //nolint:gosec // G115: at most 200
		gridColumns = &engineColumns
	}
	updated, err := q.SetMapGrid(ctx, mapsdb.SetMapGridParams{
		CampaignID: c.campaignID, ID: c.mapID, GridColumns: gridColumns, GridFactor: c.factor, Now: s.now(),
	})
	if errors.Is(err, pgx.ErrNoRows) {
		return updated, errMapNotFound()
	}
	if err != nil {
		return updated, fmt.Errorf("set map grid: %w", err)
	}
	return updated, nil
}

// settleLayers scales or clears the layers as the plan says, and tells whether it
// touched them.
func (s *Service) settleLayers(ctx context.Context, q *mapsdb.Queries, mapID string, before mapsdb.Map, plan layerPlan) (bool, error) {
	switch {
	case plan.same:
		return false, nil
	case plan.scales:
		return true, scaleLayers(ctx, q, mapID, before.VisionEpoch, plan.old, plan.step, s.now())
	default:
		return true, clearLayers(ctx, q, mapID)
	}
}

// applyFirstGridFog is the table's fog rule meeting the map's first grid (RN-24):
// the map takes the image copy made for it, when that copy is still of the image
// the map has, and the fog comes on. It returns the map as it is now and whether
// the copy was used.
func (s *Service) applyFirstGridFog(ctx context.Context, q *mapsdb.Queries, c gridChange, before mapsdb.Map) (mapsdb.Map, bool, error) {
	copied := false
	if c.imageCopy != nil && before.ImageID == c.imageCopy.source.ID {
		if err := c.imageCopy.giveMapItsCopy(ctx, q, s, c.campaignID, c.userID, c.mapID); err != nil {
			return mapsdb.Map{}, false, err
		}
		copied = true
	}
	updated, err := q.ApplyFogRule(ctx, mapsdb.ApplyFogRuleParams{CampaignID: c.campaignID, ID: c.mapID, Now: s.now()})
	if err != nil {
		return updated, copied, fmt.Errorf("apply the table's fog rule: %w", err)
	}
	return updated, copied, nil
}

// announceGrid tells the watchers of a map whose grid was set, and forgets what
// depended on the layers when they changed.
func (s *Service) announceGrid(ctx context.Context, m authz.Membership, mapID, current string, w gridWrite) {
	s.publishMapChanged(ctx, m.CampaignID, mapID, playersSee(mapID, w.updated.RevealedAt, current))
	if w.changed {
		s.lits.forget(mapID)
		s.tiles.forget(mapID)
		s.refreshVision(ctx, m.CampaignID, mapID) // the players' memory went with the layers, or was scaled
	}
}
