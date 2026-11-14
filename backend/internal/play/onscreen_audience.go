package play

import (
	"context"
	"errors"
	"fmt"
	"slices"

	"connectrpc.com/connect"
	"github.com/jackc/pgx/v5"
	"google.golang.org/protobuf/proto"

	playv1 "github.com/PuraFome/meuRPG/backend/gen/meurpg/play/v1"
	"github.com/PuraFome/meuRPG/backend/internal/authz"
	"github.com/PuraFome/meuRPG/backend/internal/platform/uid"
	"github.com/PuraFome/meuRPG/backend/internal/play/link"
	"github.com/PuraFome/meuRPG/backend/internal/play/live"
	"github.com/PuraFome/meuRPG/backend/internal/play/playdb"
)

// The audience of the image on screen (MR-028, RN-10): game_sessions.shown_image_audience.
//
//   - 'table': every member. The only audience while no scene open to only some players is open.
//   - 'scene': the players of the scene open to only some players (game_session_scene_players), so it follows
//     who is in the scene while that scene stays open. It is the default while such a scene is open.
//   - 'chosen': the players of game_session_image_viewers.
//
// When the scene stops being the image's (it closes, another one opens, or it opens to everyone) the image
// stays with the players who were in at that moment, as 'chosen' (settleShownImage): nothing already shown
// reaches anyone else. A player outside the audience gets the image nowhere: not in GetLiveSession, not on the
// stream, not through the image route; and an image left with the players (campaign_left_images) keeps the
// audience the shown image had when it was left.
const (
	audienceTable  = "table"
	audienceScene  = "scene"
	audienceChosen = "chosen"
)

// notPartialScene is the refusal of the scene's players as an audience when no scene is open to only some players.
func notPartialScene() error {
	return errScene(playv1.SceneBlockedReason_SCENE_BLOCKED_REASON_SCENE_NOT_PARTIAL, "no scene open to only some players is open")
}

// audienceAsk is the audience a call asks for: kind "" is the default.
type audienceAsk struct {
	kind  string
	users []string
}

// parseImageAudience reads the audience of a request. The default (unset, or UNSPECIFIED) is for the caller
// when allowDefault.
func (s *Service) parseImageAudience(ctx context.Context, campaignID string, a *playv1.ImageAudience, allowDefault bool) (audienceAsk, error) {
	bad := func(msg string) error { return connect.NewError(connect.CodeInvalidArgument, errors.New(msg)) }
	switch a.GetKind() {
	case playv1.ImageAudienceKind_IMAGE_AUDIENCE_KIND_UNSPECIFIED:
		if !allowDefault {
			return audienceAsk{}, bad("audience is required")
		}
		return audienceAsk{}, nil
	case playv1.ImageAudienceKind_IMAGE_AUDIENCE_KIND_WHOLE_TABLE, playv1.ImageAudienceKind_IMAGE_AUDIENCE_KIND_SCENE_PLAYERS:
		if len(a.GetUserIds()) > 0 {
			return audienceAsk{}, bad("only chosen players come with a list of players")
		}
		if a.GetKind() == playv1.ImageAudienceKind_IMAGE_AUDIENCE_KIND_SCENE_PLAYERS {
			return audienceAsk{kind: audienceScene}, nil
		}
		return audienceAsk{kind: audienceTable}, nil
	case playv1.ImageAudienceKind_IMAGE_AUDIENCE_KIND_CHOSEN_PLAYERS:
		if len(a.GetUserIds()) == 0 {
			return audienceAsk{}, bad("pick at least one player")
		}
		users, err := playersOf(ctx, campaignID, a.GetUserIds())
		return audienceAsk{kind: audienceChosen, users: users}, err
	}
	return audienceAsk{}, bad("unknown audience")
}

// resolveAudience turns what a call asks for into the audience to store: the default is the scene's
// players while a scene open to only some players is open, the whole table otherwise; the scene's players
// asked for with no such scene are refused.
func resolveAudience(session playdb.GameSession, ask audienceAsk) (audienceAsk, error) {
	switch {
	case ask.kind == "" && partialScene(session):
		return audienceAsk{kind: audienceScene}, nil
	case ask.kind == "":
		return audienceAsk{kind: audienceTable}, nil
	case ask.kind == audienceScene && !partialScene(session):
		return audienceAsk{}, notPartialScene()
	}
	return ask, nil
}

// shownViewersOf is who sees the image the session shows.
func shownViewersOf(ctx context.Context, q *playdb.Queries, session playdb.GameSession) (viewers, error) {
	switch session.ShownImageAudience {
	case audienceScene:
		return scenePlayersOf(ctx, q, session.ID)
	case audienceChosen:
		ids, err := q.ListImageViewers(ctx, session.ID)
		if err != nil {
			return viewers{}, fmt.Errorf("list the players of the shown image: %w", err)
		}
		return usersViewers(ids), nil
	}
	return allViewers(), nil
}

// imageAudienceProto is the audience of the shown image as the master reads it.
func imageAudienceProto(kind string, v viewers) *playv1.ImageAudience {
	switch kind {
	case audienceScene:
		return &playv1.ImageAudience{Kind: playv1.ImageAudienceKind_IMAGE_AUDIENCE_KIND_SCENE_PLAYERS, UserIds: slices.Clone(v.users)}
	case audienceChosen:
		return &playv1.ImageAudience{Kind: playv1.ImageAudienceKind_IMAGE_AUDIENCE_KIND_CHOSEN_PLAYERS, UserIds: slices.Clone(v.users)}
	}
	return &playv1.ImageAudience{Kind: playv1.ImageAudienceKind_IMAGE_AUDIENCE_KIND_WHOLE_TABLE}
}

// leftAudience is who an image left with the players is left with, when it leaves with v.
func leftAudience(v viewers) link.ImageAudience {
	return link.ImageAudience{Everyone: v.all, UserIDs: slices.Clone(v.users)}
}

// writeShownAudience stores the audience of the image shown.
func writeShownAudience(ctx context.Context, q *playdb.Queries, sessionID string, ask audienceAsk) error {
	if err := q.SetShownImageAudienceKind(ctx, playdb.SetShownImageAudienceKindParams{ID: sessionID, ShownImageAudience: ask.kind}); err != nil {
		return fmt.Errorf("set the audience of the shown image: %w", err)
	}
	if err := q.ClearImageViewers(ctx, sessionID); err != nil {
		return fmt.Errorf("forget the players of the shown image: %w", err)
	}
	for _, u := range ask.users {
		if err := q.InsertImageViewer(ctx, playdb.InsertImageViewerParams{GameSessionID: sessionID, UserID: u}); err != nil {
			return fmt.Errorf("put a player among the viewers of the shown image: %w", err)
		}
	}
	return nil
}

// imageMove is what a change of the shown image or of its audience does to the streams.
type imageMove struct {
	imageID  string                // the image shown now, "" for none
	shown    *playv1.ShownImage    // the image as the maps module gave it, when the caller has it
	toImage  []live.Audience       // the players who get the image
	toNone   []live.Audience       // the players who see no image now, though they saw one
	master   bool                  // the master gets the image with its audience
	audience *playv1.ImageAudience // the audience as the master reads it
}

func (mv imageMove) empty() bool { return !mv.master && len(mv.toImage) == 0 && len(mv.toNone) == 0 }

// settleShownImage turns the audience 'scene' of the shown image into the players who are in the scene now,
// for the moment the scene stops being the image's. The players are the same: only the master hears of it.
func (s *Service) settleShownImage(ctx context.Context, q *playdb.Queries, session playdb.GameSession) (imageMove, error) {
	if session.ShownImageID == nil || session.ShownImageAudience != audienceScene {
		return imageMove{}, nil
	}
	v, err := scenePlayersOf(ctx, q, session.ID)
	if err != nil {
		return imageMove{}, err
	}
	if err := writeShownAudience(ctx, q, session.ID, audienceAsk{kind: audienceChosen, users: v.users}); err != nil {
		return imageMove{}, err
	}
	return imageMove{imageID: *session.ShownImageID, master: true, audience: imageAudienceProto(audienceChosen, v)}, nil
}

// sceneImageMove is what the shown image of the scene's players does when the scene's players change
// from before to after (the session is the row after the change).
func sceneImageMove(session playdb.GameSession, before, after viewers) imageMove {
	if session.ShownImageID == nil {
		return imageMove{}
	}
	return imageMove{
		imageID: *session.ShownImageID, master: true, audience: imageAudienceProto(audienceScene, after),
		toImage: after.minus(before), toNone: before.minus(after),
	}
}

// tellImageMove collects what the move did, for once the transaction committed (the image is read then, from the maps module). A move
// that tells something moves the session's number.
func (s *Service) tellImageMove(h *hints, campaignID string, mv imageMove) {
	if mv.empty() {
		return
	}
	h.touch()
	h.after(func(ctx context.Context, say sayFn) { s.sayImageMove(ctx, say, campaignID, mv) })
}

// sayImageMove tells the streams what the move did through say: the master gets the image with its audience, the
// players who see it now the image, and the ones who stopped seeing it the update for no image.
func (s *Service) sayImageMove(ctx context.Context, say sayFn, campaignID string, mv imageMove) {
	if mv.empty() {
		return
	}
	shown := mv.shown
	if shown == nil && mv.imageID != "" {
		var err error
		if shown, err = s.maps.ShownImage(ctx, campaignID, mv.imageID); err != nil && connect.CodeOf(err) != connect.CodeNotFound {
			s.logger.ErrorContext(ctx, "play: cannot read the shown image", "error", err)
			return
		}
	}
	event := func(img *playv1.ShownImage) func() *playv1.WatchGameSessionResponse {
		return func() *playv1.WatchGameSessionResponse {
			return &playv1.WatchGameSessionResponse{Event: &playv1.WatchGameSessionResponse_ShownImageChanged_{
				ShownImageChanged: &playv1.WatchGameSessionResponse_ShownImageChanged{Image: img},
			}}
		}
	}
	if mv.master {
		sayTo(say, campaignID, true, nil, event(withAudience(shown, mv.audience)))
	}
	if shown != nil {
		sayTo(say, campaignID, false, mv.toImage, event(withAudience(shown, nil)))
	}
	sayTo(say, campaignID, false, mv.toNone, event(nil))
}

// withAudience is a copy of the image carrying the audience (nil for none), nil for no image: the shown image
// goes to the players without it, and to the master with it.
func withAudience(img *playv1.ShownImage, audience *playv1.ImageAudience) *playv1.ShownImage {
	if img == nil {
		return nil
	}
	out, ok := proto.Clone(img).(*playv1.ShownImage)
	if !ok {
		return nil
	}
	out.Audience = audience
	return out
}

// SetShownImageAudience implements playv1connect.PlayServiceHandler.
func (s *Service) SetShownImageAudience(
	ctx context.Context,
	req *connect.Request[playv1.SetShownImageAudienceRequest],
) (*connect.Response[playv1.SetShownImageAudienceResponse], error) {
	m, err := authz.RequireCampaignRole(ctx, req.Msg.GetCampaignId(), authz.RoleMaster)
	if err != nil {
		return nil, err
	}
	ask, err := s.parseImageAudience(ctx, m.CampaignID, req.Msg.GetAudience(), false)
	if err != nil {
		return nil, err
	}
	var mv imageMove
	err = s.txOpen(ctx, m.CampaignID, func(tx pgx.Tx, h *hints, session playdb.GameSession) error {
		mv = imageMove{}
		if session.ShownImageID == nil {
			return connect.NewError(connect.CodeNotFound, errors.New("no image is shown"))
		}
		var err error
		if mv, err = s.changeShownAudience(ctx, s.queries.WithTx(tx), session, ask); err != nil {
			return err
		}
		s.tellImageMove(h, m.CampaignID, mv)
		return nil
	})
	if err != nil {
		return nil, s.dbError(ctx, "set who sees the shown image", err)
	}
	shown, err := s.maps.ShownImage(ctx, m.CampaignID, mv.imageID)
	if err != nil {
		return nil, s.dbError(ctx, "find the shown image", err)
	}
	return connect.NewResponse(&playv1.SetShownImageAudienceResponse{ShownImage: withAudience(shown, mv.audience)}), nil
}

// changeShownAudience gives the image the session shows (the locked row) the audience asked for, and says
// what the streams are to hear. The audience is the same: nothing is written, and the move only names the
// audience for the answer.
func (s *Service) changeShownAudience(ctx context.Context, q *playdb.Queries, session playdb.GameSession, ask audienceAsk) (imageMove, error) {
	ask, err := resolveAudience(session, ask)
	if err != nil {
		return imageMove{}, err
	}
	old, err := shownViewersOf(ctx, q, session)
	if err != nil {
		return imageMove{}, err
	}
	next, err := viewersFor(ctx, q, session, ask)
	if err != nil {
		return imageMove{}, err
	}
	mv := imageMove{imageID: *session.ShownImageID, audience: imageAudienceProto(ask.kind, next)}
	if session.ShownImageAudience == ask.kind && old.equal(next) {
		return mv, nil
	}
	if err := writeShownAudience(ctx, q, session.ID, ask); err != nil {
		return imageMove{}, err
	}
	mv.master, mv.toImage, mv.toNone = true, next.minus(old), old.minus(next)
	return mv, nil
}

// viewersFor is who sees an image with the audience asked for (resolved), in the session.
func viewersFor(ctx context.Context, q *playdb.Queries, session playdb.GameSession, ask audienceAsk) (viewers, error) {
	switch ask.kind {
	case audienceScene:
		return scenePlayersOf(ctx, q, session.ID)
	case audienceChosen:
		return usersViewers(ask.users), nil
	}
	return allViewers(), nil
}

// shownMove works out the audience of the image that is shown after SetShownImage (the locked row is the
// session before it), writes it, and says what the streams are to hear.
func (s *Service) shownMove(ctx context.Context, q *playdb.Queries, session playdb.GameSession, showID *string, ask audienceAsk) (imageMove, viewers, error) {
	old, err := shownViewersOf(ctx, q, session)
	if err != nil {
		return imageMove{}, old, err
	}
	hadImage := session.ShownImageID != nil
	switch {
	case showID == nil:
		if err := writeShownAudience(ctx, q, session.ID, audienceAsk{kind: audienceTable}); err != nil {
			return imageMove{}, old, err
		}
		if !hadImage {
			return imageMove{}, old, nil
		}
		return imageMove{master: true, toNone: old.audiences()}, old, nil
	case hadImage && equal(session.ShownImageID, showID) && ask.kind == "":
		return imageMove{imageID: *showID, audience: imageAudienceProto(session.ShownImageAudience, old)}, old, nil // the same image again, to turn a switch: its audience stays
	case hadImage && equal(session.ShownImageID, showID):
		mv, err := s.changeShownAudience(ctx, q, session, ask)
		return mv, old, err
	}
	ask, err = resolveAudience(session, ask)
	if err != nil {
		return imageMove{}, old, err
	}
	if err := writeShownAudience(ctx, q, session.ID, ask); err != nil {
		return imageMove{}, old, err
	}
	next, err := viewersFor(ctx, q, session, ask)
	if err != nil {
		return imageMove{}, old, err
	}
	mv := imageMove{imageID: *showID, master: true, audience: imageAudienceProto(ask.kind, next), toImage: next.audiences()}
	if hadImage {
		mv.toNone = old.minus(next)
	}
	return mv, old, nil
}

// ImageShownTo says whether the image is the one the open session shows and the user is in the audience
// that sees it. It implements maps.LiveSession: the image route asks it of a player on every request.
func (s *Service) ImageShownTo(ctx context.Context, campaignID, userID, imageID string) (bool, error) {
	if _, ok := uid.Canonical(userID); !ok {
		return false, nil // no user, no audience
	}
	seen, err := s.queries.ShownImageSeenBy(ctx, playdb.ShownImageSeenByParams{CampaignID: campaignID, ImageID: imageID, UserID: userID})
	if err != nil {
		return false, fmt.Errorf("read who sees the shown image: %w", err)
	}
	return seen, nil
}

// ShownImageViewers says who sees the image the open session shows: the players of its audience, nobody
// when no session is open or it shows no image. It implements maps.LiveSession: deleting the image tells them.
func (s *Service) ShownImageViewers(ctx context.Context, campaignID string) (link.ImageAudience, error) {
	session, err := s.queries.GetOpenGameSession(ctx, campaignID)
	if errors.Is(err, pgx.ErrNoRows) {
		return link.ImageAudience{}, nil
	}
	if err != nil {
		return link.ImageAudience{}, fmt.Errorf("find the open session: %w", err)
	}
	if session.ShownImageID == nil {
		return link.ImageAudience{}, nil
	}
	v, err := shownViewersOf(ctx, s.queries, session)
	if err != nil {
		return link.ImageAudience{}, err
	}
	return leftAudience(v), nil
}

// shownImageFor reads the image the session shows as the member sees it: the master with its audience, a
// player only when they are in the audience.
func (s *Service) shownImageFor(ctx context.Context, m authz.Membership, session playdb.GameSession) (*playv1.ShownImage, error) {
	if session.ShownImageID == nil {
		return nil, nil
	}
	v, err := shownViewersOf(ctx, s.queries, session)
	if err != nil {
		return nil, s.dbError(ctx, "read who sees the shown image", err)
	}
	master := m.Role == authz.RoleMaster
	if !master && !v.has(m.UserID) {
		return nil, nil
	}
	img, err := s.maps.ShownImage(ctx, m.CampaignID, *session.ShownImageID)
	if connect.CodeOf(err) == connect.CodeNotFound {
		return nil, nil // deleted since the read above
	}
	if err != nil {
		return nil, s.dbError(ctx, "read the shown image", err)
	}
	if master {
		img.Audience = imageAudienceProto(session.ShownImageAudience, v)
	}
	return img, nil
}
