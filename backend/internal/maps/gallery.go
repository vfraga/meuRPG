package maps

import (
	"context"
	"errors"
	"fmt"
	"math"
	"uuid"

	"connectrpc.com/connect"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"google.golang.org/protobuf/types/known/timestamppb"

	mapsv1 "github.com/PuraFome/meuRPG/backend/gen/meurpg/maps/v1"
	playv1 "github.com/PuraFome/meuRPG/backend/gen/meurpg/play/v1"
	"github.com/PuraFome/meuRPG/backend/internal/authz"
	"github.com/PuraFome/meuRPG/backend/internal/maps/images"
	"github.com/PuraFome/meuRPG/backend/internal/maps/mapsdb"
	"github.com/PuraFome/meuRPG/backend/internal/platform/db"
	"github.com/PuraFome/meuRPG/backend/internal/platform/names"
	"github.com/PuraFome/meuRPG/backend/internal/platform/rpcerr"
	"github.com/PuraFome/meuRPG/backend/internal/platform/sessiontell"
	"github.com/PuraFome/meuRPG/backend/internal/play/link"
)

// Every GalleryService method is the master's: the gallery is preparation,
// and a player could learn about a map before it is revealed (RN-10). Each
// handler starts with authz.RequireCampaignRole, whose error is already the
// right Connect error.

// ListGalleryImages implements mapsv1connect.GalleryServiceHandler.
func (s *Service) ListGalleryImages(
	ctx context.Context,
	req *connect.Request[mapsv1.ListGalleryImagesRequest],
) (*connect.Response[mapsv1.ListGalleryImagesResponse], error) {
	m, err := authz.RequireCampaignRole(ctx, req.Msg.GetCampaignId(), authz.RoleMaster)
	if err != nil {
		return nil, err
	}
	if s.blobs == nil {
		return nil, errImagesOff()
	}
	// The images, their totals and the maps that use them are one moment: an
	// upload, a delete or a redraw between the statements would show a total
	// that does not match the list.
	var (
		rows  []mapsdb.GalleryImage
		usage mapsdb.GetGalleryUsageRow
		maps  []mapsdb.ListMapImageIDsRow
	)
	err = db.ReadTx(ctx, s.pool, func(tx pgx.Tx) error {
		q := s.queries.WithTx(tx)
		var err error
		if rows, err = q.ListGalleryImages(ctx, m.CampaignID); err != nil {
			return fmt.Errorf("list the gallery: %w", err)
		}
		if usage, err = q.GetGalleryUsage(ctx, m.CampaignID); err != nil {
			return fmt.Errorf("read the gallery usage: %w", err)
		}
		// The maps that use each image ("Usada em Mirathel e arredores").
		if maps, err = q.ListMapImageIDs(ctx, m.CampaignID); err != nil {
			return fmt.Errorf("list the maps' images: %w", err)
		}
		return nil
	})
	if err != nil {
		return nil, s.dbError(ctx, "list the gallery", err)
	}
	usedIn := map[string][]*mapsv1.MapRef{}
	for _, mp := range maps {
		usedIn[mp.ImageID] = append(usedIn[mp.ImageID], &mapsv1.MapRef{Id: mp.ID, Name: mp.Name})
	}
	res := &mapsv1.ListGalleryImagesResponse{Usage: s.usageToProto(usage)}
	for _, row := range rows {
		img := imageToProto(row)
		img.UsedInMaps = usedIn[row.ID]
		res.Images = append(res.Images, img)
	}
	return connect.NewResponse(res), nil
}

// RenameGalleryImage implements mapsv1connect.GalleryServiceHandler.
func (s *Service) RenameGalleryImage(
	ctx context.Context,
	req *connect.Request[mapsv1.RenameGalleryImageRequest],
) (*connect.Response[mapsv1.RenameGalleryImageResponse], error) {
	m, err := authz.RequireCampaignRole(ctx, req.Msg.GetCampaignId(), authz.RoleMaster)
	if err != nil {
		return nil, err
	}
	if s.blobs == nil {
		return nil, errImagesOff()
	}
	id, err := uuid.Parse(req.Msg.GetImageId())
	if err != nil {
		return nil, errImageNotFound()
	}
	name, err := names.Clean(req.Msg.GetName(), maxNameLength)
	if err != nil {
		// The message says the rule, never the name, which is free text.
		return nil, connect.NewError(connect.CodeInvalidArgument, fmt.Errorf("name %w", err))
	}

	var row mapsdb.GalleryImage
	err = s.inSession(ctx, m.CampaignID, func(tx pgx.Tx, t sessiontell.Tell) error {
		var err error
		q := s.queries.WithTx(tx)
		row, err = q.RenameGalleryImage(ctx, mapsdb.RenameGalleryImageParams{
			CampaignID: m.CampaignID, ID: id.String(), Name: name,
		})
		if errors.Is(err, pgx.ErrNoRows) {
			return errImageNotFound()
		}
		if err != nil {
			return fmt.Errorf("rename gallery image: %w", err)
		}
		// The name is the caption of the image on a screen: no event says so, the screens read it again. An image no screen shows
		// moves no number. What a screen shows is read here, under the session's lock, so a show made while the rename waited for it counts.
		onAScreen, err := imageIsOnAScreen(ctx, tx, q, s.live, m.CampaignID, id.String())
		if err != nil {
			return err
		}
		if onAScreen {
			t.Touch()
		}
		return nil
	})
	if err != nil {
		return nil, s.dbError(ctx, "rename a gallery image", err)
	}
	img := imageToProto(row)
	if img.UsedInMaps, err = s.mapsUsing(ctx, s.queries, m.CampaignID, row.ID); err != nil {
		return nil, s.dbError(ctx, "list the maps that use an image", err)
	}
	return connect.NewResponse(&mapsv1.RenameGalleryImageResponse{Image: img}), nil
}

// imageIsOnAScreen says whether a screen shows the image's name as a caption: the image on screen, and one left with the players. It reads
// through tx, so the caller holds the session's lock when it asks.
func imageIsOnAScreen(ctx context.Context, tx pgx.Tx, q *mapsdb.Queries, live LiveSession, campaignID, imageID string) (bool, error) {
	shown, err := live.ImageShown(ctx, tx, campaignID, imageID)
	if err != nil {
		return false, fmt.Errorf("read the shown image: %w", err)
	}
	if shown {
		return true, nil
	}
	if _, err := leftWith(ctx, q, campaignID, imageID); err != nil {
		if connect.CodeOf(err) == connect.CodeNotFound {
			return false, nil
		}
		return false, fmt.Errorf("check whether the image is left with the players: %w", err)
	}
	return true, nil
}

// DeleteGalleryImage implements mapsv1connect.GalleryServiceHandler. It
// deletes the row first, then the files: once the row is gone nobody can
// fetch the files, so a failure to delete them (logged) leaves nothing
// reachable behind. An image a map uses stays; an image the session shows
// (MR-028) goes, and the session stops showing it (the foreign key sets
// game_sessions.shown_image_id to NULL). An image that is an NPC's portrait
// (MR-031) goes too, and the NPC loses the portrait (in the same
// transaction): a portrait falls back to the initials, so it never blocks.
func (s *Service) DeleteGalleryImage(
	ctx context.Context,
	req *connect.Request[mapsv1.DeleteGalleryImageRequest],
) (*connect.Response[mapsv1.DeleteGalleryImageResponse], error) {
	m, err := authz.RequireCampaignRole(ctx, req.Msg.GetCampaignId(), authz.RoleMaster)
	if err != nil {
		return nil, err
	}
	if s.blobs == nil {
		return nil, errImagesOff()
	}
	id, err := uuid.Parse(req.Msg.GetImageId())
	if err != nil {
		return nil, errImageNotFound()
	}
	_, shown, err := s.live.OnScreen(ctx, m.CampaignID)
	if err != nil {
		return nil, s.dbError(ctx, "read the shown image", err)
	}
	// Who sees the image on screen, if it is this one: they are the ones to hear it go (RN-10).
	var shownTo link.ImageAudience
	if shown == id.String() {
		if shownTo, err = s.live.ShownImageViewers(ctx, m.CampaignID); err != nil {
			return nil, s.dbError(ctx, "read who sees the shown image", err)
		}
	}
	// Who the image was left with, if it was: they are the ones to hear it go.
	leftTo, err := leftWith(ctx, s.queries, m.CampaignID, id.String())
	wasLeft := err == nil
	if err != nil && connect.CodeOf(err) != connect.CodeNotFound {
		return nil, s.dbError(ctx, "check whether the image is left with the players", err)
	}
	_, err = s.deleteImageRows(ctx, m.CampaignID, id.String(), func(ctx context.Context, portraits int64) {
		s.publishImageDeleted(ctx, m.CampaignID, imageGone{
			shown: shown == id.String(), shownTo: shownTo, portraits: portraits > 0, left: wasLeft, leftTo: leftTo,
		})
	})
	if err != nil {
		return nil, err
	}
	s.deleteFiles(ctx, m.CampaignID, id.String())
	return connect.NewResponse(&mapsv1.DeleteGalleryImageResponse{}), nil
}

// deleteImageRows deletes the image's row, unless a map uses it, and clears it
// as the portrait of the NPCs that had it, under the session's lock (the screen it was on loses it); it returns how many lost it.
// Once it committed, tell runs with that number.
func (s *Service) deleteImageRows(ctx context.Context, campaignID, imageID string, tell func(ctx context.Context, portraits int64)) (int64, error) {
	var portraits int64 // the NPCs that lost it as their portrait
	err := s.inSession(ctx, campaignID, func(tx pgx.Tx, t sessiontell.Tell) error {
		var err error
		if portraits, err = s.deleteImageTx(ctx, tx, campaignID, imageID); err != nil {
			return err
		}
		s.announce(t, func(ctx context.Context) { tell(ctx, portraits) })
		return nil
	})
	// The check in the transaction runs in it, so a map created at the same
	// time makes one of the two retry (SERIALIZABLE). The foreign key
	// (maps.image_id, ON DELETE RESTRICT) is the backstop.
	if isForeignKeyViolation(err) {
		usedIn, _ := s.mapsUsing(ctx, s.queries, campaignID, imageID)
		return 0, errImageInUse(usedIn)
	}
	if err != nil {
		return 0, s.dbError(ctx, "delete a gallery image", err)
	}
	return portraits, nil
}

// deleteImageTx is the transaction of a delete. An image a map uses stays, and
// the answer names the maps (MR-019: "o app diz em qual mapa ela está").
func (s *Service) deleteImageTx(ctx context.Context, tx pgx.Tx, campaignID, imageID string) (int64, error) {
	q := s.queries.WithTx(tx)
	usedIn, err := s.mapsUsing(ctx, q, campaignID, imageID)
	if err != nil {
		return 0, fmt.Errorf("list the maps that use the image: %w", err)
	}
	if len(usedIn) > 0 {
		return 0, errImageInUse(usedIn)
	}
	_, err = q.DeleteGalleryImage(ctx, mapsdb.DeleteGalleryImageParams{CampaignID: campaignID, ID: imageID})
	if errors.Is(err, pgx.ErrNoRows) {
		return 0, errImageNotFound()
	}
	if err != nil {
		return 0, fmt.Errorf("delete gallery image: %w", err)
	}
	portraits, err := s.characters.ClearPortraits(ctx, tx, campaignID, imageID)
	if err != nil {
		return 0, fmt.Errorf("clear the portraits: %w", err)
	}
	return portraits, nil
}

// imageGone is what a deleted image took with it.
type imageGone struct {
	shown     bool               // it was the image on screen,
	shownTo   link.ImageAudience // and these players saw it
	portraits bool               // it was the portrait of NPCs
	left      bool               // it was left with the players,
	leftTo    link.ImageAudience // and these players had it
}

// publishImageDeleted tells the session what a deleted image took with it: the
// image on the screen (to the master and the players who saw it), the NPC portraits
// on the stage and the image left with the players (only the master and the players
// it was left with hear that).
func (s *Service) publishImageDeleted(ctx context.Context, campaignID string, gone imageGone) {
	if gone.shown {
		ev := &playv1.WatchGameSessionResponse{Event: &playv1.WatchGameSessionResponse_ShownImageChanged_{
			ShownImageChanged: &playv1.WatchGameSessionResponse_ShownImageChanged{},
		}}
		s.tellAudience(ctx, campaignID, gone.shownTo, ev)
	}
	if gone.portraits {
		// The NPC may have been on the stage: its portrait is gone for the scene.
		s.publishScene(ctx, campaignID, &playv1.WatchGameSessionResponse{Event: &playv1.WatchGameSessionResponse_StageChanged_{
			StageChanged: &playv1.WatchGameSessionResponse_StageChanged{},
		}})
	}
	if gone.left {
		// It left the players' list too (the foreign key deleted the row).
		s.tellAudience(ctx, campaignID, gone.leftTo, &playv1.WatchGameSessionResponse{Event: &playv1.WatchGameSessionResponse_LeftImagesChanged_{
			LeftImagesChanged: &playv1.WatchGameSessionResponse_LeftImagesChanged{},
		}})
	}
}

// tellAudience sends ev to the master and to the players of the audience, and to no other.
func (s *Service) tellAudience(ctx context.Context, campaignID string, who link.ImageAudience, ev *playv1.WatchGameSessionResponse) {
	s.tellStreams(ctx, campaignID, sessiontell.Audience{Master: true, Players: who.Everyone}, ev)
	if !who.Everyone {
		s.publishUsers(ctx, campaignID, who.UserIDs, ev)
	}
}

func imageToProto(r mapsdb.GalleryImage) *mapsv1.GalleryImage {
	img := &mapsv1.GalleryImage{
		Id:           r.ID,
		CampaignId:   r.CampaignID,
		Name:         r.Name,
		ContentType:  r.ContentType,
		Width:        r.Width,
		Height:       r.Height,
		ByteSize:     r.ByteSize,
		CreatedAt:    timestamppb.New(r.CreatedAt),
		Url:          imageURL(r.ID),
		ThumbnailUrl: thumbnailURL(r.ID),
		Generated:    r.Generated,
		// The whole map is in a textured map and in what was made from one (RN-10).
		ShowsWholeMap: r.GeneratedKind == kindTexturedMap,
	}
	if r.ParentImageID != nil {
		img.ParentImageId = *r.ParentImageID
	}
	return img
}

func (s *Service) usageToProto(u mapsdb.GetGalleryUsageRow) *mapsv1.GalleryUsage {
	return &mapsv1.GalleryUsage{
		ImageCount:    u.ImageCount,
		MaxImages:     s.maxImages,
		ByteCount:     clampInt32(u.ByteCount),
		MaxBytes:      s.maxBytes,
		MaxImageBytes: images.MaxBytes,
	}
}

// clampInt32 fits a byte count into the API's int32. The quota keeps a
// gallery far below 2 GiB, the most an int32 holds; this only makes the
// conversion provably safe.
func clampInt32(n int64) int32 {
	if n > math.MaxInt32 {
		return math.MaxInt32
	}
	if n < 0 {
		return 0
	}
	return int32(n)
}

// mapsUsing lists the maps whose image this is, oldest first.
func (s *Service) mapsUsing(ctx context.Context, q *mapsdb.Queries, campaignID, imageID string) ([]*mapsv1.MapRef, error) {
	rows, err := q.ListMapsUsingImage(ctx, mapsdb.ListMapsUsingImageParams{CampaignID: campaignID, ImageID: imageID})
	if err != nil {
		return nil, err
	}
	out := make([]*mapsv1.MapRef, 0, len(rows))
	for _, r := range rows {
		out = append(out, &mapsv1.MapRef{Id: r.ID, Name: r.Name})
	}
	return out, nil
}

// errImageInUse is DeleteGalleryImage's failed_precondition, with the
// ImageInUse detail that names the maps for the app. The message itself
// never names them: map names are free text.
func errImageInUse(maps []*mapsv1.MapRef) error {
	err := connect.NewError(connect.CodeFailedPrecondition, errors.New("a map uses this image; change the map's image first"))
	if detail, detailErr := connect.NewErrorDetail(&mapsv1.ImageInUse{Maps: maps}); detailErr == nil {
		err.AddDetail(detail)
	}
	return err
}

// errImageNotFound is the answer for an image that is not in the campaign.
func errImageNotFound() error {
	return connect.NewError(connect.CodeNotFound, errors.New("image not found"))
}

// isForeignKeyViolation reports whether err is a foreign key violation
// (23503).
func isForeignKeyViolation(err error) bool {
	pgErr, ok := errors.AsType[*pgconn.PgError](err)
	return ok && pgErr.Code == "23503"
}

// dbError maps an error from the database to the Connect error the client gets
// (see platform/rpcerr).
func (s *Service) dbError(ctx context.Context, action string, err error) error {
	return rpcerr.FromDB(ctx, s.logger, "maps", action, err)
}
