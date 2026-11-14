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

// UpdateMap refuses in this order: something to change, the revision, the name, the
// image's id; and, inside the transaction, the map, the revision and the image.

// UpdateMap implements mapsv1connect.MapServiceHandler.
func (s *Service) UpdateMap(
	ctx context.Context,
	req *connect.Request[mapsv1.UpdateMapRequest],
) (*connect.Response[mapsv1.UpdateMapResponse], error) {
	m, err := authz.RequireCampaignRole(ctx, req.Msg.GetCampaignId(), authz.RoleMaster)
	if err != nil {
		return nil, err
	}
	mapID, ok := uid.Canonical(req.Msg.GetMapId())
	if !ok {
		return nil, errMapNotFound()
	}
	update, err := checkMapUpdate(req.Msg)
	if err != nil {
		return nil, err
	}
	current, err := s.currentMap(ctx, m.CampaignID)
	if err != nil {
		return nil, err
	}
	var imageCopy *fogCopy
	var fogSeen bool
	if update.imageID != nil {
		if imageCopy, fogSeen, err = s.imageCopyFor(ctx, m.CampaignID, mapID, *update.imageID); err != nil {
			return nil, s.dbError(ctx, "copy the map's image", err)
		}
	}
	var w mapUpdateWrite
	err = s.inSession(ctx, m.CampaignID, func(tx pgx.Tx, t sessiontell.Tell) error {
		var err error
		if w, err = s.writeMapUpdate(ctx, tx, mapUpdateTx{mapUpdate: update, campaignID: m.CampaignID, userID: m.UserID, mapID: mapID, imageCopy: imageCopy, fogSeen: fogSeen}); err != nil {
			return err
		}
		s.announce(t, func(ctx context.Context) { s.announceMapUpdate(ctx, m, mapID, current, w) })
		return nil
	})
	s.discardFogCopy(ctx, m.CampaignID, imageCopy, err != nil || !w.copied)
	if err != nil {
		return nil, s.dbError(ctx, "update a map", err)
	}
	out, err := s.masterMap(ctx, m, mapID)
	if err != nil {
		return nil, err
	}
	return connect.NewResponse(&mapsv1.UpdateMapResponse{Map: out}), nil
}

// mapUpdate is what an UpdateMap request asks for, checked. A nil field stays as
// it is.
type mapUpdate struct {
	name, imageID *string
	revision      int32
}

// checkMapUpdate refuses a request that changes nothing, carries no revision, or
// whose name or image's id cannot be a map's.
func checkMapUpdate(msg *mapsv1.UpdateMapRequest) (mapUpdate, error) {
	if msg.Name == nil && msg.ImageId == nil {
		return mapUpdate{}, connect.NewError(connect.CodeInvalidArgument, errors.New("set name, image_id or both"))
	}
	if msg.GetRevision() < 1 {
		return mapUpdate{}, connect.NewError(connect.CodeInvalidArgument, errors.New("revision must be at least 1"))
	}
	u := mapUpdate{revision: msg.GetRevision()}
	if msg.Name != nil {
		clean, err := cleanName("name", msg.GetName())
		if err != nil {
			return mapUpdate{}, err
		}
		u.name = &clean
	}
	if msg.ImageId != nil {
		id, ok := uid.Canonical(msg.GetImageId())
		if !ok {
			return mapUpdate{}, errNotAGalleryImage()
		}
		u.imageID = &id
	}
	return u, nil
}

// mapUpdateTx is one UpdateMap call as its transaction sees it.
type mapUpdateTx struct {
	mapUpdate
	campaignID, userID, mapID string
	imageCopy                 *fogCopy
	fogSeen                   bool // the fog the copy was prepared for (imageCopyFor)
}

// mapUpdateWrite is what the transaction did.
type mapUpdateWrite struct {
	updated mapsdb.Map
	cleared bool // the layers were cleared by a new image
	copied  bool // the fog's image copy was used
}

// writeMapUpdate is UpdateMap's transaction. Whether the layers are cleared is
// decided from the row locked here, so a combat that starts meanwhile is seen.
func (s *Service) writeMapUpdate(ctx context.Context, tx pgx.Tx, c mapUpdateTx) (mapUpdateWrite, error) {
	q := s.queries.WithTx(tx)
	row, err := lockMap(ctx, q, c.campaignID, c.mapID)
	if err != nil {
		return mapUpdateWrite{}, err
	}
	if row.Revision != c.revision {
		return mapUpdateWrite{}, errStaleMap()
	}
	params := mapsdb.UpdateMapParams{CampaignID: c.campaignID, ID: c.mapID, Name: row.Name, ImageID: row.ImageID, Revision: row.Revision, Now: s.now()}
	if c.name != nil {
		params.Name = *c.name
	}
	var w mapUpdateWrite
	if c.imageID != nil {
		if params.ImageID, w.copied, err = s.pickMapImage(ctx, q, c, row); err != nil {
			return mapUpdateWrite{}, err
		}
	}
	// A new image clears the painted layers (D2), which a fight standing on
	// them cannot lose; the same image again clears nothing.
	imageChanged := params.ImageID != row.ImageID
	if imageChanged {
		if err := s.refuseWhileCombat(ctx, tx, c.campaignID, c.mapID); err != nil {
			return mapUpdateWrite{}, err
		}
	}
	if w.updated, err = q.UpdateMap(ctx, params); errors.Is(err, pgx.ErrNoRows) {
		return mapUpdateWrite{}, errStaleMap() // never, under the lock above: a second guard
	} else if err != nil {
		return mapUpdateWrite{}, fmt.Errorf("update map: %w", err)
	}
	if imageChanged {
		w.cleared = true
		return w, clearLayers(ctx, q, c.mapID)
	}
	return w, nil
}

// pickMapImage checks that the new image is in the campaign's gallery and fits the map's
// grid, and returns the image the map takes: the new one, or the copy made for it, which
// is then added to the gallery.
func (s *Service) pickMapImage(ctx context.Context, q *mapsdb.Queries, c mapUpdateTx, row mapsdb.Map) (imageID string, copied bool, err error) {
	img, err := q.GetGalleryImageInCampaign(ctx, mapsdb.GetGalleryImageInCampaignParams{CampaignID: c.campaignID, ID: *c.imageID})
	if errors.Is(err, pgx.ErrNoRows) {
		return "", false, errNotAGalleryImage()
	}
	if err != nil {
		return "", false, fmt.Errorf("find image: %w", err)
	}
	if err := checkImageFitsGrid(row, *c.imageID, img); err != nil {
		return "", false, err
	}
	if *c.imageID != row.ImageID && c.fogSeen != row.FogEnabled {
		return "", false, errStaleMap() // the fog changed after the copy was prepared, and the revision did not
	}
	if c.imageCopy == nil || *c.imageID != c.imageCopy.source.ID {
		return *c.imageID, false, nil
	}
	if err := c.imageCopy.insert(ctx, q, s, c.campaignID, c.userID); err != nil {
		return "", false, err
	}
	return c.imageCopy.id, true, nil
}

// checkImageFitsGrid refuses an image that would make a calibrated map's grid
// pass the limits. The rows follow the image's proportions: a taller image on a
// calibrated map could pass the rules' 400 rows (MR-025), which no layer, combat
// or CHECK accepts. The master picks another image or recalibrates first.
func checkImageFitsGrid(row mapsdb.Map, imageID string, img mapsdb.GalleryImage) error {
	if row.GridColumns == nil || imageID == row.ImageID {
		return nil
	}
	if _, err := grid.EngineGrid(int(*row.GridColumns/row.GridFactor), int(row.GridFactor), int(img.Width), int(img.Height)); err != nil {
		return connect.NewError(connect.CodeInvalidArgument, fmt.Errorf(
			"this image would make the map's grid pass the limit of %d x %d squares: lower the calibration first", grid.MaxColumns, grid.MaxRows))
	}
	return nil
}

// announceMapUpdate tells the watchers of an updated map, and forgets what
// depended on the layers when a new image cleared them.
func (s *Service) announceMapUpdate(ctx context.Context, m authz.Membership, mapID, current string, w mapUpdateWrite) {
	s.publishMapChanged(ctx, m.CampaignID, mapID, playersSee(mapID, w.updated.RevealedAt, current))
	if w.cleared {
		s.tiles.forget(mapID)
		s.refreshVision(ctx, m.CampaignID, mapID) // the players' memory went with the layers
	}
}
