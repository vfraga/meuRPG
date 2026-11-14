package maps

import (
	"context"
	"errors"
	"fmt"
	"slices"

	"connectrpc.com/connect"

	mapsv1 "github.com/PuraFome/meuRPG/backend/gen/meurpg/maps/v1"
	"github.com/PuraFome/meuRPG/backend/internal/maps/mapsdb"
	"github.com/PuraFome/meuRPG/backend/internal/platform/uid"
	"github.com/PuraFome/meuRPG/backend/internal/play/link"
)

// The images of an RP scene (MR-015, "Imagens da cena"): the gallery images the
// master attached to a SCENE point, in order, up to maxSceneImages. It is the
// master's preparation, like the clues: only the master ever receives the list
// (RN-10), and a player sees one of the images only when the master shows it
// with the gallery's "Mostrar aos jogadores" (MR-019), which does not read
// these rows. So the list is built for the master reads only (attachImages,
// ScenePoint) and never for a player's.

// maxSceneImages is how many images a scene point may hold: enough for the
// pictures of one scene, and a row of thumbnails that fits a phone's screen.
const maxSceneImages = 8

// SetSceneImages implements mapsv1connect.MapServiceHandler.
func (s *Service) SetSceneImages(
	ctx context.Context,
	req *connect.Request[mapsv1.SetSceneImagesRequest],
) (*connect.Response[mapsv1.SetSceneImagesResponse], error) {
	m, mapID, pointID, err := s.sceneCall(ctx, req.Msg.GetCampaignId(), req.Msg.GetMapId(), req.Msg.GetPointId())
	if err != nil {
		return nil, err
	}
	ids, err := cleanSceneImageIDs(req.Msg.GetImageIds())
	if err != nil {
		return nil, err
	}
	var rows []mapsdb.ListPointImagesRow
	changed := false
	err = s.changeActions(ctx, m, mapID, pointID, func(q *mapsdb.Queries) error {
		var err error
		rows, changed, err = s.replaceSceneImages(ctx, q, m.CampaignID, pointID, ids)
		return err
	}, func(ch changedPoint) *sceneTold {
		if !changed {
			return nil // the same list: nothing changes, nobody is told
		}
		return &sceneTold{tell: func(ctx context.Context) { s.actionsChanged(ctx, m.CampaignID, ch) }}
	})
	if err != nil {
		return nil, s.dbError(ctx, "set a scene's images", err)
	}
	return connect.NewResponse(&mapsv1.SetSceneImagesResponse{Images: sceneImagesToProto(rows)}), nil
}

// replaceSceneImages makes ids the point's list, if it is not already, and
// returns the list as it is now and whether it changed. The point is locked by
// now, so two calls take turns.
func (s *Service) replaceSceneImages(ctx context.Context, q *mapsdb.Queries, campaignID, pointID string, ids []string) ([]mapsdb.ListPointImagesRow, bool, error) {
	if err := checkImagesInCampaign(ctx, q, campaignID, ids); err != nil {
		return nil, false, err
	}
	before, err := q.ListPointImages(ctx, pointID)
	if err != nil {
		return nil, false, fmt.Errorf("list the scene's images: %w", err)
	}
	if slices.Equal(imageIDsOf(before), ids) {
		return before, false, nil // the same list: nothing changes, nobody is told
	}
	if err := q.DeletePointImages(ctx, pointID); err != nil {
		return nil, false, fmt.Errorf("clear the scene's images: %w", err)
	}
	now := s.now()
	for i, id := range ids {
		if err := q.InsertPointImage(ctx, mapsdb.InsertPointImageParams{PointID: pointID, ImageID: id, Position: int32(i), CreatedAt: now}); err != nil {
			return nil, false, fmt.Errorf("attach an image: %w", err)
		}
	}
	rows, err := q.ListPointImages(ctx, pointID)
	if err != nil {
		return nil, false, fmt.Errorf("list the scene's images: %w", err)
	}
	return rows, true, nil
}

// checkImagesInCampaign refuses a list that names an image the campaign's
// gallery does not hold.
func checkImagesInCampaign(ctx context.Context, q *mapsdb.Queries, campaignID string, ids []string) error {
	if len(ids) == 0 {
		return nil
	}
	n, err := q.CountCampaignImagesIn(ctx, mapsdb.CountCampaignImagesInParams{CampaignID: campaignID, Ids: ids})
	if err != nil {
		return fmt.Errorf("count the images: %w", err)
	}
	if int(n) != len(ids) {
		return connect.NewError(connect.CodeNotFound, errors.New("image not found"))
	}
	return nil
}

// cleanSceneImageIDs checks the list the master sends: at most maxSceneImages UUIDs,
// none twice.
func cleanSceneImageIDs(raw []string) ([]string, error) {
	if len(raw) > maxSceneImages {
		return nil, connect.NewError(connect.CodeInvalidArgument, fmt.Errorf("a scene holds at most %d images", maxSceneImages))
	}
	ids := make([]string, 0, len(raw))
	for _, r := range raw {
		id, ok := uid.Canonical(r)
		if !ok {
			return nil, connect.NewError(connect.CodeInvalidArgument, errors.New("image_ids must be UUIDs"))
		}
		if slices.Contains(ids, id) {
			return nil, connect.NewError(connect.CodeInvalidArgument, errors.New("image_ids must not repeat an image"))
		}
		ids = append(ids, id)
	}
	return ids, nil
}

func imageIDsOf(rows []mapsdb.ListPointImagesRow) []string {
	ids := make([]string, 0, len(rows))
	for _, r := range rows {
		ids = append(ids, r.ImageID)
	}
	return ids
}

func sceneImagesToProto(rows []mapsdb.ListPointImagesRow) []*mapsv1.SceneImage {
	out := make([]*mapsv1.SceneImage, 0, len(rows))
	for _, r := range rows {
		out = append(out, &mapsv1.SceneImage{Id: r.ImageID, Name: r.Name, ShowsWholeMap: r.GeneratedKind == kindTexturedMap})
	}
	return out
}

// attachImages fills the images of the SCENE points of a map, for the master.
// A player gets none, so this reads nothing for a player (RN-10).
func (s *Service) attachImages(ctx context.Context, mapID string, points []*mapsv1.MapPoint, master bool) error {
	if !master || !slices.ContainsFunc(points, func(p *mapsv1.MapPoint) bool { return p.GetKind() == mapsv1.MapPointKind_MAP_POINT_KIND_SCENE }) {
		return nil
	}
	rows, err := s.queries.ListPointImagesOfMap(ctx, mapID)
	if err != nil {
		return err
	}
	byPoint := map[string][]*mapsv1.SceneImage{}
	for _, r := range rows {
		byPoint[r.PointID] = append(byPoint[r.PointID], &mapsv1.SceneImage{Id: r.ImageID, Name: r.Name, ShowsWholeMap: r.GeneratedKind == kindTexturedMap})
	}
	for _, p := range points {
		p.Images = byPoint[p.GetId()]
	}
	return nil
}

// sceneImages reads a scene point's images for package play (the master's open
// scene).
func sceneImages(ctx context.Context, q *mapsdb.Queries, pointID string) ([]link.SceneImage, error) {
	rows, err := q.ListPointImages(ctx, pointID)
	if err != nil {
		return nil, fmt.Errorf("list the scene's images: %w", err)
	}
	out := make([]link.SceneImage, 0, len(rows))
	for _, r := range rows {
		out = append(out, link.SceneImage{ID: r.ImageID, Name: r.Name, ShowsWholeMap: r.GeneratedKind == kindTexturedMap})
	}
	return out, nil
}
