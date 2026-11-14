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
	"github.com/PuraFome/meuRPG/backend/internal/platform/conv"
	"github.com/PuraFome/meuRPG/backend/internal/play/link"
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

	err = s.txOpen(ctx, m.CampaignID, func(tx pgx.Tx, h *hints, session playdb.GameSession) error {
		if mapID != nil {
			// The players see the current map, so it is revealed with it,
			// in the same transaction (RN-10).
			if err := s.maps.RevealMap(ctx, tx, m.CampaignID, *mapID, s.now()); err != nil {
				return err
			}
		}
		if _, err := s.queries.WithTx(tx).SetCurrentMap(ctx, playdb.SetCurrentMapParams{ID: session.ID, CurrentMapID: mapID}); err != nil {
			return fmt.Errorf("set the current map: %w", err)
		}
		current := conv.OrZero(mapID)
		h.after(func(ctx context.Context, _ sayFn) {
			if mapID != nil {
				s.maps.MapShown(ctx, m.CampaignID, *mapID)
			}
		})
		h.touch() // the number moves with the map, whichever steps follow
		// After the maps module's step: the streams hear the current map once the fog knows it.
		h.after(func(_ context.Context, say sayFn) {
			say(m.CampaignID, live.Event{Audience: live.Audience{Master: true, Everyone: true}, Message: &playv1.WatchGameSessionResponse{Event: &playv1.WatchGameSessionResponse_CurrentMapChanged_{
				CurrentMapChanged: &playv1.WatchGameSessionResponse_CurrentMapChanged{MapId: current},
			}}})
		})
		return nil
	})
	if err != nil {
		return nil, s.dbError(ctx, "set the current map", err)
	}

	current := conv.OrZero(mapID)
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
	ask, err := s.showAudienceAsk(ctx, m.CampaignID, imageID, req.Msg.GetAudience())
	if err != nil {
		return nil, err
	}
	shown, shownCopy, imageID, err := s.prepareShownImage(ctx, m.CampaignID, imageID)
	if err != nil {
		return nil, err
	}

	out, err := s.swapShownImage(ctx, m.CampaignID, imageID, keep, ask, shown, shownCopy)
	if shownCopy != nil && (err != nil || !out.copyCreated) {
		shownCopy.Discard(ctx) // no gallery row: the copy's files go
	}
	if err == nil && out.showing != shown.GetId() {
		// Another show committed the copy first: its image is the one shown.
		if shown, err = s.maps.ShownImage(ctx, m.CampaignID, out.showing); err != nil {
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
	return connect.NewResponse(&playv1.SetShownImageResponse{ShownImage: withAudience(shown, out.move.audience), Keep: keep}), nil
}

// showAudienceAsk reads the audience of a show; it is ignored when nothing is shown.
func (s *Service) showAudienceAsk(ctx context.Context, campaignID string, imageID *string, a *playv1.ImageAudience) (audienceAsk, error) {
	if imageID == nil {
		return audienceAsk{}, nil
	}
	return s.parseImageAudience(ctx, campaignID, a, true)
}

// prepareShownImage asks the maps module for the image to show (none for nil). The
// image must be the campaign's; the foreign key keeps it from disappearing before
// the commit. It returns the image the maps module answered with, which is the
// copy when the one asked for is a fog map's, the files of that copy (its row not
// yet made) and the id to show.
func (s *Service) prepareShownImage(ctx context.Context, campaignID string, imageID *string) (*playv1.ShownImage, ShownCopy, *string, error) {
	if imageID == nil {
		return nil, nil, nil, nil
	}
	shown, shownCopy, err := s.maps.PrepareShow(ctx, campaignID, *imageID)
	if err != nil {
		return nil, nil, nil, s.dbError(ctx, "find the image to show", err)
	}
	shownID := shown.GetId()
	return shown, shownCopy, &shownID, nil
}

// tellShownImage collects what a change of the shown image did: the master and the players who see it now get the image, the ones
// who saw an image and do not any more get no image, and the players of an image left with the players hear that the list changed.
// Only the switch moved when the image is the same: nothing for the players, who never learn the switch. A change of the switch
// alone still moves the session's number. shown is the image the maps module answered with; when another show made the copy first,
// the stream reads the image that is shown.
func (s *Service) tellShownImage(h *hints, campaignID string, out shownOutcome, shown *playv1.ShownImage, switchMoved bool) {
	mv := out.move
	if shown.GetId() == out.showing {
		mv.shown = shown
	}
	if mv.empty() && !out.changed && !out.moved && !switchMoved {
		return
	}
	h.touch()
	s.tellImageMove(h, campaignID, mv)
	if out.moved {
		// After the image, as it was told before: the streams hear the image, then the list.
		h.after(func(_ context.Context, say sayFn) {
			sayTo(say, campaignID, true, out.leftTo.audiences(), leftImagesChangedMessage)
		})
	}
}

// shownOutcome is what a change of the shown image did.
type shownOutcome struct {
	moved       bool      // an image went to the left list
	leftTo      viewers   // the players it was left with
	changed     bool      // another image (or none) is shown now
	copyCreated bool      // the shown copy's row is the one this call inserted
	showing     string    // the image shown at the end of the transaction, "" for none
	move        imageMove // what the streams are to hear
}

// swapShownImage shows the image (nil for none) in the open session, inside one
// transaction, and has what the streams are to hear told once it committed.
func (s *Service) swapShownImage(ctx context.Context, campaignID string, imageID *string, keep bool, ask audienceAsk, shown *playv1.ShownImage, shownCopy ShownCopy) (shownOutcome, error) {
	var out shownOutcome
	err := s.txOpen(ctx, campaignID, func(tx pgx.Tx, h *hints, session playdb.GameSession) error {
		out = shownOutcome{}
		switchMoved, err := s.swapShownImageIn(ctx, tx, campaignID, session, imageID, keep, ask, shownCopy, &out)
		if err != nil {
			return err
		}
		s.tellShownImage(h, campaignID, out, shown, switchMoved)
		return nil
	})
	return out, err
}

// swapShownImageIn is swapShownImage's transaction; it says whether the keep switch moved.
func (s *Service) swapShownImageIn(ctx context.Context, tx pgx.Tx, campaignID string, session playdb.GameSession, imageID *string, keep bool, ask audienceAsk, shownCopy ShownCopy, out *shownOutcome) (bool, error) {
	q := s.queries.WithTx(tx)
	var err error
	// An image kept leaves with the players when it stops being shown:
	// replaced by another, or by nothing. The same image again only
	// moves the switch.
	showID := imageID
	if shownCopy != nil {
		// The copy's row first: another show of the same image may have made it meanwhile.
		id, created, err := shownCopy.Insert(ctx, tx)
		if err != nil {
			return false, err
		}
		showID, out.copyCreated = &id, created
	}
	out.showing = conv.OrZero(showID)
	out.changed = !equal(session.ShownImageID, showID)
	var old viewers
	if out.move, old, err = s.shownMove(ctx, q, session, showID, ask); err != nil {
		return false, err
	}
	if session.ShownImageKeep && session.ShownImageID != nil && out.changed {
		// It is left with the players who saw it, no others (RN-10).
		if err := s.maps.LeaveImage(ctx, tx, campaignID, *session.ShownImageID, s.now(), leftAudience(old)); err != nil {
			return false, err
		}
		out.moved, out.leftTo = true, old
	}
	if _, err := q.SetShownImage(ctx, playdb.SetShownImageParams{ID: session.ID, ShownImageID: showID, ShownImageKeep: keep}); err != nil {
		return false, fmt.Errorf("set the shown image: %w", err)
	}
	return session.ShownImageKeep != keep, nil
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
	images, err := s.maps.ListLeftImages(ctx, m.CampaignID, m.UserID, m.Role == authz.RoleMaster)
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
	// One write that moves the session's number once: the list's hint goes to the master and the players the image was left with,
	// with the number. With no open session there is no number, and the take-back runs plain and tells nothing.
	err = s.txOrNone(ctx, m.CampaignID, func(tx pgx.Tx, h *hints, _ string) error {
		was, err := s.maps.TakeBackImage(ctx, tx, m.CampaignID, *imageID)
		if err != nil {
			return err
		}
		tellViewers(h, m.CampaignID, leftViewers(was), leftImagesChangedMessage)
		return nil
	})
	if err != nil {
		return nil, s.dbError(ctx, "take a left image back", err)
	}
	return connect.NewResponse(&playv1.TakeBackLeftImageResponse{}), nil
}

// leftImagesChangedMessage is the hint that the images left with the players changed; the app reads the list again.
func leftImagesChangedMessage() *playv1.WatchGameSessionResponse {
	return &playv1.WatchGameSessionResponse{Event: &playv1.WatchGameSessionResponse_LeftImagesChanged_{
		LeftImagesChanged: &playv1.WatchGameSessionResponse_LeftImagesChanged{},
	}}
}

// leftViewers is who an image left with the players was left with.
func leftViewers(a link.ImageAudience) viewers {
	if a.Everyone {
		return allViewers()
	}
	return usersViewers(a.UserIDs)
}

// equal reports whether two optional IDs are the same.
func equal(a, b *string) bool {
	return conv.OrZero(a) == conv.OrZero(b)
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
	return conv.OrZero(row.CurrentMapID), conv.OrZero(row.ShownImageID), nil
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
	return conv.OrZero(row.ShownImageID) == imageID, nil
}

// CurrentMapIn returns the open session's current map, reading through tx, for a write that holds the session's lock. It implements
// maps.LiveSession.
func (s *Service) CurrentMapIn(ctx context.Context, tx pgx.Tx, campaignID string) (string, error) {
	row, err := s.queries.WithTx(tx).GetOnScreen(ctx, campaignID)
	if errors.Is(err, pgx.ErrNoRows) {
		return "", nil // no open session
	}
	if err != nil {
		return "", fmt.Errorf("read what the session shows: %w", err)
	}
	return conv.OrZero(row.CurrentMapID), nil
}

// Publish sends an event on the campaign's live streams: to the master's
// always, and to the players' only when players is true. The maps module
// decides that for its own changes, because only it knows what each change
// touches (RN-10). Without an open session nobody is subscribed, and it
// does nothing. It implements maps.LiveSession; ev must not be changed
// after the call.
func (s *Service) Publish(campaignID string, players bool, ev *playv1.WatchGameSessionResponse) {
	s.tellMembers(campaignID, players, ev)
}

// PublishToUsers sends ev to the streams of those of userIDs who watch the
// campaign's session, and to nobody else: not the master, not the other
// players (a clue reaches only the players who got it, MR-029). It implements
// maps.LiveSession.
func (s *Service) PublishToUsers(campaignID string, userIDs []string, ev *playv1.WatchGameSessionResponse) {
	for _, id := range userIDs {
		s.send(campaignID, live.Event{Audience: live.Audience{UserID: id}, Message: ev})
	}
}

// PublishToUsersCoalesced is PublishToUsers for a hint that says "read it
// again": while a user's stream still has an event with the same key waiting
// in its queue, no other is queued for it (live.Event.Coalesce). It implements
// maps.LiveSession.
func (s *Service) PublishToUsersCoalesced(campaignID string, userIDs []string, key string, ev *playv1.WatchGameSessionResponse) {
	for _, id := range userIDs {
		s.send(campaignID, live.Event{Audience: live.Audience{UserID: id}, Message: ev, Coalesce: key})
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

// deref is conv.OrZero for the files of the held rolls, which still call it.
func deref(s *string) string { return conv.OrZero(s) }

// isForeignKeyViolation reports whether err is a foreign key violation
// (23503).
func isForeignKeyViolation(err error) bool {
	pgErr, ok := errors.AsType[*pgconn.PgError](err)
	return ok && pgErr.Code == "23503"
}
