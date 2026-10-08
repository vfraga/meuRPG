package play

import (
	"context"
	"errors"
	"fmt"
	"uuid"

	"connectrpc.com/connect"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"

	playv1 "github.com/PuraFome/meuRPG/backend/gen/meurpg/play/v1"
	"github.com/PuraFome/meuRPG/backend/internal/authz"
	"github.com/PuraFome/meuRPG/backend/internal/platform/db"
	"github.com/PuraFome/meuRPG/backend/internal/play/live"
	"github.com/PuraFome/meuRPG/backend/internal/play/playdb"
)

// What the session shows at the table, one of each, side by side:
//   - the current map (game_sessions.current_map_id, SetCurrentMap), which
//     the players see and which setting reveals (RN-10);
//   - a gallery image the master shows (game_sessions.shown_image_id,
//     SetShownImage, MR-028): a handout, such as a portrait or a letter. It
//     reveals nothing else. With its "keep" switch on, it is left with the
//     players when it stops being shown (campaign_left_images: ListLeftImages,
//     TakeBackLeftImage), and they keep seeing it until the master takes it
//     back. Those images belong to the campaign, not to the session, so they
//     outlive it: ending the session would otherwise take them away.
//
// The maps, their points and tokens, and the gallery are the maps module's;
// this package only keeps what is on screen, and carries the maps' changes
// on the live stream. The two modules need each other, so each declares
// what it needs, the other implements it, and cmd/api connects them:
//   - here, MapKeeper (maps.SessionMaps): check and reveal the current map,
//     inside this package's transaction, and read the shown image;
//   - there, maps.LiveSession (this Service: OnScreen and Publish, below): a
//     player also sees the current map and the shown image, and the maps'
//     changes go out on the stream.

// SetCurrentMap implements playv1connect.PlayServiceHandler.
func (s *Service) SetCurrentMap(
	ctx context.Context,
	req *connect.Request[playv1.SetCurrentMapRequest],
) (*connect.Response[playv1.SetCurrentMapResponse], error) {
	m, err := authz.RequireCampaignRole(ctx, req.Msg.GetCampaignId(), authz.RoleMaster)
	if err != nil {
		return nil, err
	}
	mapID, err := optionalID(req.Msg.GetMapId(), "map not found")
	if err != nil {
		return nil, err
	}

	err = db.InTx(ctx, s.pool, func(tx pgx.Tx) error {
		q := s.queries.WithTx(tx)
		// Lock the open session, like every change made during it: an end
		// in progress finishes first, and this call then sees no session.
		session, err := q.GetOpenGameSessionForUpdate(ctx, m.CampaignID)
		if errors.Is(err, pgx.ErrNoRows) {
			return errNoOpenSession()
		}
		if err != nil {
			return fmt.Errorf("lock the open session: %w", err)
		}
		if mapID != nil {
			// The players see the current map, so it is revealed with it,
			// in the same transaction (RN-10).
			if err := s.maps.RevealMap(ctx, tx, m.CampaignID, *mapID, s.now()); err != nil {
				return err
			}
		}
		if _, err := q.SetCurrentMap(ctx, playdb.SetCurrentMapParams{ID: session.ID, CurrentMapID: mapID}); err != nil {
			return fmt.Errorf("set the current map: %w", err)
		}
		return nil
	})
	if err != nil {
		return nil, s.dbError(ctx, "set the current map", err)
	}

	current := deref(mapID)
	if mapID != nil {
		s.maps.MapShown(ctx, m.CampaignID, *mapID)
	}
	s.Publish(m.CampaignID, true, &playv1.WatchGameSessionResponse{Event: &playv1.WatchGameSessionResponse_CurrentMapChanged_{
		CurrentMapChanged: &playv1.WatchGameSessionResponse_CurrentMapChanged{MapId: current},
	}})
	return connect.NewResponse(&playv1.SetCurrentMapResponse{CurrentMapId: current}), nil
}

// SetShownImage implements playv1connect.PlayServiceHandler.
func (s *Service) SetShownImage(
	ctx context.Context,
	req *connect.Request[playv1.SetShownImageRequest],
) (*connect.Response[playv1.SetShownImageResponse], error) {
	m, err := authz.RequireCampaignRole(ctx, req.Msg.GetCampaignId(), authz.RoleMaster)
	if err != nil {
		return nil, err
	}
	imageID, err := optionalID(req.Msg.GetImageId(), "image not found")
	if err != nil {
		return nil, err
	}
	keep := req.Msg.GetKeep() && imageID != nil
	var shown *playv1.ShownImage
	var shownCopy ShownCopy // the files of a fog map's image copy, its row not yet made
	if imageID != nil {
		// The image must be the campaign's. The foreign key keeps it from
		// disappearing before the commit (below).
		if shown, shownCopy, err = s.maps.PrepareShow(ctx, m.CampaignID, *imageID); err != nil {
			return nil, s.dbError(ctx, "find the image to show", err)
		}
		// What is shown is the image the maps module answered with: the copy, when
		// the one asked for is a fog map's.
		shownID := shown.GetId()
		imageID = &shownID
	}

	var moved bool       // an image went to the left list
	var changed bool     // another image (or none) is shown now
	var copyCreated bool // the shown copy's row is the one this call inserted
	var showing string   // the image shown at the end of the transaction, "" for none
	err = db.InTx(ctx, s.pool, func(tx pgx.Tx) error {
		q := s.queries.WithTx(tx)
		session, err := q.GetOpenGameSessionForUpdate(ctx, m.CampaignID)
		if errors.Is(err, pgx.ErrNoRows) {
			return errNoOpenSession()
		}
		if err != nil {
			return fmt.Errorf("lock the open session: %w", err)
		}
		// An image kept leaves with the players when it stops being shown:
		// replaced by another, or by nothing. The same image again only
		// moves the switch.
		moved = false
		copyCreated = false
		showID := imageID
		if shownCopy != nil {
			// The copy's row first: another show of the same image may have made it meanwhile.
			id, created, err := shownCopy.Insert(ctx, tx)
			if err != nil {
				return err
			}
			showID, copyCreated = &id, created
		}
		showing = deref(showID)
		changed = !equal(session.ShownImageID, showID)
		if session.ShownImageKeep && session.ShownImageID != nil && !equal(session.ShownImageID, showID) {
			if err := s.maps.LeaveImage(ctx, tx, m.CampaignID, *session.ShownImageID, s.now()); err != nil {
				return err
			}
			moved = true
		}
		if _, err := q.SetShownImage(ctx, playdb.SetShownImageParams{ID: session.ID, ShownImageID: showID, ShownImageKeep: keep}); err != nil {
			return fmt.Errorf("set the shown image: %w", err)
		}
		return nil
	})
	if shownCopy != nil && (err != nil || !copyCreated) {
		shownCopy.Discard(ctx) // no gallery row: the copy's files go
	}
	if err == nil && showing != shown.GetId() {
		// Another show committed the copy first: its image is the one shown.
		if shown, err = s.maps.ShownImage(ctx, m.CampaignID, showing); err != nil {
			return nil, s.dbError(ctx, "find the shown image", err)
		}
	}
	if isForeignKeyViolation(err) {
		// The master deleted the image in another tab meanwhile.
		return nil, connect.NewError(connect.CodeNotFound, errors.New("image not found"))
	}
	if err != nil {
		return nil, s.dbError(ctx, "set the shown image", err)
	}

	// Only the switch moved when the image is the same: nothing for the
	// players, who never learn the switch, and the master's page already
	// knows.
	if changed {
		s.Publish(m.CampaignID, true, &playv1.WatchGameSessionResponse{Event: &playv1.WatchGameSessionResponse_ShownImageChanged_{
			ShownImageChanged: &playv1.WatchGameSessionResponse_ShownImageChanged{Image: shown},
		}})
	}
	if moved {
		s.publishLeftImagesChanged(m.CampaignID)
	}
	return connect.NewResponse(&playv1.SetShownImageResponse{ShownImage: shown, Keep: keep}), nil
}

// ListLeftImages implements playv1connect.PlayServiceHandler.
func (s *Service) ListLeftImages(
	ctx context.Context,
	req *connect.Request[playv1.ListLeftImagesRequest],
) (*connect.Response[playv1.ListLeftImagesResponse], error) {
	m, err := authz.RequireCampaignMember(ctx, req.Msg.GetCampaignId())
	if err != nil {
		return nil, err
	}
	images, err := s.maps.ListLeftImages(ctx, m.CampaignID)
	if err != nil {
		return nil, s.dbError(ctx, "list the left images", err)
	}
	return connect.NewResponse(&playv1.ListLeftImagesResponse{Images: images}), nil
}

// TakeBackLeftImage implements playv1connect.PlayServiceHandler.
func (s *Service) TakeBackLeftImage(
	ctx context.Context,
	req *connect.Request[playv1.TakeBackLeftImageRequest],
) (*connect.Response[playv1.TakeBackLeftImageResponse], error) {
	m, err := authz.RequireCampaignRole(ctx, req.Msg.GetCampaignId(), authz.RoleMaster)
	if err != nil {
		return nil, err
	}
	imageID, err := optionalID(req.Msg.GetImageId(), "image not found")
	if err != nil {
		return nil, err
	}
	if imageID == nil {
		return nil, connect.NewError(connect.CodeNotFound, errors.New("image not found"))
	}
	if err := s.maps.TakeBackImage(ctx, m.CampaignID, *imageID); err != nil {
		return nil, s.dbError(ctx, "take a left image back", err)
	}
	s.publishLeftImagesChanged(m.CampaignID)
	return connect.NewResponse(&playv1.TakeBackLeftImageResponse{}), nil
}

// publishLeftImagesChanged tells every stream of the campaign that the
// images left with the players changed; the app reads the list again.
func (s *Service) publishLeftImagesChanged(campaignID string) {
	s.Publish(campaignID, true, &playv1.WatchGameSessionResponse{Event: &playv1.WatchGameSessionResponse_LeftImagesChanged_{
		LeftImagesChanged: &playv1.WatchGameSessionResponse_LeftImagesChanged{},
	}})
}

// equal reports whether two optional IDs are the same.
func equal(a, b *string) bool {
	return deref(a) == deref(b)
}

// OnScreen returns what the campaign's open game session shows: the IDs of
// its current map and of the gallery image the master shows, each "" when
// there is none, both "" when no session is open. It implements
// maps.LiveSession, in one read; its errors are ordinary errors, for the
// maps module to log.
func (s *Service) OnScreen(ctx context.Context, campaignID string) (currentMapID, shownImageID string, err error) {
	row, err := s.queries.GetOnScreen(ctx, campaignID)
	if errors.Is(err, pgx.ErrNoRows) {
		return "", "", nil // no open session
	}
	if err != nil {
		return "", "", fmt.Errorf("read what the session shows: %w", err)
	}
	return deref(row.CurrentMapID), deref(row.ShownImageID), nil
}

// ImageShown says whether the image is the one the campaign's open session
// shows the players, reading through tx, for a transaction that is about to
// delete it. It implements maps.LiveSession.
func (s *Service) ImageShown(ctx context.Context, tx pgx.Tx, campaignID, imageID string) (bool, error) {
	row, err := s.queries.WithTx(tx).GetOnScreen(ctx, campaignID)
	if errors.Is(err, pgx.ErrNoRows) {
		return false, nil // no open session
	}
	if err != nil {
		return false, fmt.Errorf("read what the session shows: %w", err)
	}
	return deref(row.ShownImageID) == imageID, nil
}

// Publish sends an event on the campaign's live streams: to the master's
// always, and to the players' only when players is true. The maps module
// decides that for its own changes, because only it knows what each change
// touches (RN-10). Without an open session nobody is subscribed, and it
// does nothing. It implements maps.LiveSession; ev must not be changed
// after the call.
func (s *Service) Publish(campaignID string, players bool, ev *playv1.WatchGameSessionResponse) {
	s.hub.Publish(campaignID, live.Event{Audience: live.Audience{Master: true, Everyone: players}, Message: ev})
}

// PublishToUsers sends ev to the streams of those of userIDs who watch the
// campaign's session, and to nobody else: not the master, not the other
// players (a clue reaches only the players who got it, MR-029). It implements
// maps.LiveSession.
func (s *Service) PublishToUsers(campaignID string, userIDs []string, ev *playv1.WatchGameSessionResponse) {
	for _, id := range userIDs {
		s.hub.Publish(campaignID, live.Event{Audience: live.Audience{UserID: id}, Message: ev})
	}
}

// PublishToUsersCoalesced is PublishToUsers for a hint that says "read it
// again": while a user's stream still has an event with the same key waiting
// in its queue, no other is queued for it (live.Event.Coalesce). It implements
// maps.LiveSession.
func (s *Service) PublishToUsersCoalesced(campaignID string, userIDs []string, key string, ev *playv1.WatchGameSessionResponse) {
	for _, id := range userIDs {
		s.hub.Publish(campaignID, live.Event{Audience: live.Audience{UserID: id}, Message: ev, Coalesce: key})
	}
}

// optionalID reads an optional ID from a request: nil when empty, a
// `not_found` with notFound when it is not a UUID (it names nothing).
func optionalID(raw, notFound string) (*string, error) {
	if raw == "" {
		return nil, nil
	}
	id, err := uuid.Parse(raw)
	if err != nil {
		return nil, connect.NewError(connect.CodeNotFound, errors.New(notFound))
	}
	text := id.String()
	return &text, nil
}

func deref(s *string) string {
	if s == nil {
		return ""
	}
	return *s
}

// isForeignKeyViolation reports whether err is a foreign key violation
// (23503).
func isForeignKeyViolation(err error) bool {
	pgErr, ok := errors.AsType[*pgconn.PgError](err)
	return ok && pgErr.Code == "23503"
}
