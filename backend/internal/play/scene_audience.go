package play

import (
	"context"
	"errors"
	"fmt"
	"slices"
	"uuid"

	"connectrpc.com/connect"
	"github.com/jackc/pgx/v5"

	playv1 "github.com/PuraFome/meuRPG/backend/gen/meurpg/play/v1"
	"github.com/PuraFome/meuRPG/backend/internal/authz"
	"github.com/PuraFome/meuRPG/backend/internal/platform/db"
	"github.com/PuraFome/meuRPG/backend/internal/platform/logging"
	"github.com/PuraFome/meuRPG/backend/internal/play/link"
	"github.com/PuraFome/meuRPG/backend/internal/play/live"
	"github.com/PuraFome/meuRPG/backend/internal/play/playdb"
)

// The players a scene is open to (MR-015, RN-10).
//
//   - A scene is open to the whole table (game_sessions.open_scene_everyone, the default, which includes a
//     player who joins later) or to exactly the players of game_session_scene_players.
//   - A player outside it gets no scene at all, exactly as when none is open: GetOpenScene answers nil,
//     RollSceneCheck is refused as with no open scene, the stream tells them nothing, and the group check
//     asked while it is open goes to its players only. The master is always in.
//   - OpenScene takes the audience; SetSceneAudience changes it while the scene is open. The players who
//     come in and the ones who go out hear scene_changed, so the first read the scene and the second read
//     "no scene".
//   - The shown image of the scene's players follows (onscreen_audience.go).

// viewers is a set of the campaign's players: all of them, or exactly these users. The master is not in
// it: he reads everything and hears every hint.
type viewers struct {
	all   bool
	users []string // sorted, no repeats, empty when all
}

func allViewers() viewers { return viewers{all: true} }

// usersViewers is exactly these users.
func usersViewers(ids []string) viewers {
	users := slices.Clone(ids)
	slices.Sort(users)
	return viewers{users: slices.Compact(users)}
}

func (v viewers) has(userID string) bool { return v.all || slices.Contains(v.users, userID) }

func (v viewers) equal(o viewers) bool { return v.all == o.all && slices.Equal(v.users, o.users) }

// union is who is in v or in o.
func (v viewers) union(o viewers) viewers {
	if v.all || o.all {
		return allViewers()
	}
	return usersViewers(append(slices.Clone(v.users), o.users...))
}

// audiences is what the hub reaches for these players.
func (v viewers) audiences() []live.Audience {
	return v.minus(viewers{})
}

// minus is the players in v who are not in o.
func (v viewers) minus(o viewers) []live.Audience {
	switch {
	case o.all:
		return nil
	case v.all:
		return []live.Audience{{Players: true, Except: o.users}}
	}
	var out []live.Audience
	for _, u := range v.users {
		if !o.has(u) {
			out = append(out, live.Audience{UserID: u})
		}
	}
	return out
}

// publishTo sends ev to the master (when master) and to each audience.
func (s *Service) publishTo(campaignID string, master bool, auds []live.Audience, ev *playv1.WatchGameSessionResponse) {
	if master {
		s.hub.Publish(campaignID, live.Event{Audience: live.Audience{Master: true}, Message: ev})
	}
	for _, a := range auds {
		s.hub.Publish(campaignID, live.Event{Audience: a, Message: ev})
	}
}

// publishToViewers sends ev to the master and to the players in v.
func (s *Service) publishToViewers(campaignID string, v viewers, ev *playv1.WatchGameSessionResponse) {
	s.publishTo(campaignID, true, v.audiences(), ev)
}

// sceneViewersOf is who the scene open in the session is open to. A session with no scene open has
// nobody to name: the whole table.
func sceneViewersOf(ctx context.Context, q *playdb.Queries, session playdb.GameSession) (viewers, error) {
	if session.OpenSceneEveryone {
		return allViewers(), nil
	}
	return scenePlayersOf(ctx, q, session.ID)
}

// scenePlayersOf are the players listed for the session's scene, whether or not it is open to everyone.
func scenePlayersOf(ctx context.Context, q *playdb.Queries, sessionID string) (viewers, error) {
	rows, err := q.ListScenePlayers(ctx, sessionID)
	if err != nil {
		return viewers{}, fmt.Errorf("list the players of the scene: %w", err)
	}
	ids := make([]string, 0, len(rows))
	for _, r := range rows {
		ids = append(ids, r.UserID)
	}
	return usersViewers(ids), nil
}

// partialScene says the session has a scene open to only some players.
func partialScene(session playdb.GameSession) bool {
	return session.OpenScenePointID != nil && !session.OpenSceneEveryone
}

// readsScene says whether the member reads the scene open in the session: the master always does, a
// player when the scene is open to everyone or to them (RN-10).
func readsScene(ctx context.Context, q *playdb.Queries, session playdb.GameSession, m authz.Membership) (bool, error) {
	if m.Role == authz.RoleMaster || session.OpenSceneEveryone {
		return true, nil
	}
	v, err := scenePlayersOf(ctx, q, session.ID)
	return v.has(m.UserID), err
}

// requireInScene refuses a player who is not in the open scene as when no scene is open.
func requireInScene(ctx context.Context, q *playdb.Queries, session playdb.GameSession, m authz.Membership) error {
	in, err := readsScene(ctx, q, session, m)
	if err != nil {
		return err
	}
	if !in {
		return errNoOpenScene()
	}
	return nil
}

// outsideOpenScene says whether a scene open to only some players is open and the user is not in it. An
// empty userID is nobody in particular, who is in no list of players: outside a scene that is not open to the
// whole table. It is a guard for any reader with no user: no caller on the image route reaches that branch (they
// pass real ids; the TV tab fetches images as the master). The table screen's own scene read checks the flag.
func outsideOpenScene(ctx context.Context, q *playdb.Queries, session playdb.GameSession, userID string) (bool, error) {
	if !partialScene(session) {
		return false, nil
	}
	if userID == "" {
		return true, nil
	}
	v, err := scenePlayersOf(ctx, q, session.ID)
	return !v.has(userID), err
}

// sceneViewersNow is who the scene open in the campaign's open session is open to, and whether a session
// is open at all.
func (s *Service) sceneViewersNow(ctx context.Context, campaignID string) (viewers, bool, error) {
	session, err := s.queries.GetOpenGameSession(ctx, campaignID)
	if errors.Is(err, pgx.ErrNoRows) {
		return viewers{}, false, nil
	}
	if err != nil {
		return viewers{}, false, fmt.Errorf("find the open session: %w", err)
	}
	v, err := sceneViewersOf(ctx, s.queries, session)
	return v, true, err
}

// PublishToScene sends ev to the streams of the master and of the players in the open scene (everyone
// when the scene is open to the whole table or none is open); without an open session nothing happens.
// When the audience cannot be read the master alone hears it. It implements maps.LiveSession.
func (s *Service) PublishToScene(ctx context.Context, campaignID string, ev *playv1.WatchGameSessionResponse) {
	v, open, err := s.sceneViewersNow(ctx, campaignID)
	if err != nil {
		s.logger.ErrorContext(ctx, "play: cannot read the players of the scene", "error", err)
		v = viewers{}
	} else if !open {
		return
	}
	s.publishToViewers(campaignID, v, ev)
}

// sceneChangedEvent is the hint that the open scene changed. It names nothing: each app reads the scene
// again, as its member is allowed.
func sceneChangedEvent() *playv1.WatchGameSessionResponse {
	return &playv1.WatchGameSessionResponse{Event: &playv1.WatchGameSessionResponse_SceneChanged_{
		SceneChanged: &playv1.WatchGameSessionResponse_SceneChanged{},
	}}
}

// playersOf checks that every account names a player of the campaign, and returns them without repeats.
// The audience of a scene or of an image is about players, not characters: the player of a dead character
// follows a scene like anyone (MR-015).
func playersOf(ctx context.Context, campaignID string, raw []string) ([]string, error) {
	var out []string
	for _, r := range raw {
		id, err := uuid.Parse(r)
		if err != nil {
			return nil, errAudienceNotPlayers()
		}
		ok, err := authz.IsActivePlayer(ctx, campaignID, id.String())
		if err != nil {
			return nil, err
		}
		if !ok {
			return nil, errAudienceNotPlayers()
		}
		if !slices.Contains(out, id.String()) {
			out = append(out, id.String())
		}
	}
	return out, nil
}

func errAudienceNotPlayers() error {
	return connect.NewError(connect.CodeInvalidArgument, errors.New("the audience must name players of the campaign"))
}

// parseSceneAudience reads the players a scene is open to. An unset audience is for the caller to
// decide (ok false).
func (s *Service) parseSceneAudience(ctx context.Context, campaignID string, a *playv1.SceneAudience) (v viewers, ok bool, err error) {
	if a == nil {
		return viewers{}, false, nil
	}
	if a.GetEveryone() {
		if len(a.GetUserIds()) > 0 {
			return viewers{}, false, connect.NewError(connect.CodeInvalidArgument, errors.New("an audience is everyone, or some players, not both"))
		}
		return allViewers(), true, nil
	}
	if len(a.GetUserIds()) == 0 {
		return viewers{}, false, connect.NewError(connect.CodeInvalidArgument, errors.New("pick at least one player, or everyone"))
	}
	users, err := playersOf(ctx, campaignID, a.GetUserIds())
	if err != nil {
		return viewers{}, false, err
	}
	return usersViewers(users), true, nil
}

// audienceChange is what a change of the players of the open scene did.
type audienceChange struct {
	before, after viewers
	changed       bool
	wide          bool      // the scene let players join, before or after: every player reads its prompt appear or go
	image         imageMove // the shown image that follows the scene's players
}

// notify is who hears that the scene changed: the players in it before and after, and every player when
// the scene let others join.
func (c audienceChange) notify() viewers {
	if c.wide {
		return allViewers()
	}
	return c.before.union(c.after)
}

// applySceneAudience sets who the open scene (the session's locked row) is open to, and moves the shown
// image that follows the scene's players. It writes nothing when the audience is the same.
func (s *Service) applySceneAudience(ctx context.Context, q *playdb.Queries, session playdb.GameSession, to viewers) (playdb.GameSession, audienceChange, error) {
	before, err := sceneViewersOf(ctx, q, session)
	if err != nil {
		return session, audienceChange{}, err
	}
	ch := audienceChange{before: before, after: to}
	if before.equal(to) {
		return session, ch, nil
	}
	ch.changed = true
	if to.all {
		// The scene stops being a few players': the image shown to them stays with them.
		if ch.image, err = s.settleShownImage(ctx, q, session); err != nil {
			return session, ch, err
		}
	}
	joined, err := joinedBySelf(ctx, q, session.ID) // who joined by themselves stays marked
	if err != nil {
		return session, ch, err
	}
	if err := q.ClearScenePlayers(ctx, session.ID); err != nil {
		return session, ch, fmt.Errorf("clear the players of the scene: %w", err)
	}
	for _, u := range to.users {
		if err := q.InsertScenePlayer(ctx, playdb.InsertScenePlayerParams{GameSessionID: session.ID, UserID: u, JoinedBySelf: joined[u]}); err != nil {
			return session, ch, fmt.Errorf("put a player in the scene: %w", err)
		}
	}
	if session, err = q.SetOpenSceneEveryone(ctx, playdb.SetOpenSceneEveryoneParams{ID: session.ID, OpenSceneEveryone: to.all}); err != nil {
		return session, ch, fmt.Errorf("set who the scene is open to: %w", err)
	}
	if !to.all && session.ShownImageAudience == audienceScene {
		ch.image = sceneImageMove(session, before, to)
	}
	return session, ch, nil
}

// joinedBySelf are the players of the scene who joined it by themselves.
func joinedBySelf(ctx context.Context, q *playdb.Queries, sessionID string) (map[string]bool, error) {
	rows, err := q.ListScenePlayers(ctx, sessionID)
	if err != nil {
		return nil, fmt.Errorf("list the players of the scene: %w", err)
	}
	out := map[string]bool{}
	for _, r := range rows {
		out[r.UserID] = r.JoinedBySelf
	}
	return out, nil
}

// SetSceneAudience implements playv1connect.PlayServiceHandler.
func (s *Service) SetSceneAudience(
	ctx context.Context,
	req *connect.Request[playv1.SetSceneAudienceRequest],
) (*connect.Response[playv1.SetSceneAudienceResponse], error) {
	m, err := authz.RequireCampaignRole(ctx, req.Msg.GetCampaignId(), authz.RoleMaster)
	if err != nil {
		return nil, err
	}
	to, ok, err := s.parseSceneAudience(ctx, m.CampaignID, req.Msg.GetAudience())
	if err != nil {
		return nil, err
	}
	if !ok {
		return nil, connect.NewError(connect.CodeInvalidArgument, errors.New("audience is required"))
	}
	var ch audienceChange
	err = db.InTx(ctx, s.pool, func(tx pgx.Tx) error {
		ch = audienceChange{}
		q := s.queries.WithTx(tx)
		session, err := lockOpenSessionOrNone(ctx, q, m.CampaignID)
		if err != nil {
			return err
		}
		scene, err := s.openSceneIn(ctx, tx, m.CampaignID, session)
		if err != nil {
			return err
		}
		if _, ch, err = s.applySceneAudience(ctx, q, session, to); err != nil {
			return err
		}
		if !ch.changed {
			return nil
		}
		return s.discoverScene(ctx, tx, m.CampaignID, scene.PointID, to, s.now()) // the players now in may know it
	})
	if err != nil {
		return nil, s.dbError(ctx, "set who the scene is open to", err)
	}
	s.publishAudienceChange(ctx, m.CampaignID, ch)
	info, err := s.openSceneInfo(ctx, m)
	if err != nil {
		return nil, err
	}
	return connect.NewResponse(&playv1.SetSceneAudienceResponse{Scene: info}), nil
}

// publishAudienceChange tells the streams that who is in the scene changed: the master and every player who
// was in or is in hear scene_changed, and the shown image follows.
func (s *Service) publishAudienceChange(ctx context.Context, campaignID string, ch audienceChange) {
	if !ch.changed {
		return
	}
	logging.Event(ctx, s.logger, "scene.audience_changed")
	s.publishToViewers(campaignID, ch.notify(), sceneChangedEvent())
	s.publishImageMove(ctx, campaignID, ch.image)
}

// sceneAudienceView is who is in the open scene and who is out of it, for the master.
func (s *Service) sceneAudienceView(ctx context.Context, session playdb.GameSession) (*playv1.SceneAudienceView, error) {
	in, err := sceneViewersOf(ctx, s.queries, session)
	if err != nil {
		return nil, s.dbError(ctx, "read who is in the scene", err)
	}
	party, err := s.roster.CombatParty(ctx, nil, session.CampaignID)
	if err != nil {
		return nil, s.dbError(ctx, "read the party", err)
	}
	joined, err := joinedBySelf(ctx, s.queries, session.ID)
	if err != nil {
		return nil, s.dbError(ctx, "read who joined the scene", err)
	}
	out := &playv1.SceneAudienceView{Everyone: in.all}
	for _, c := range party {
		if c.PlayerUserID == "" {
			continue
		}
		p := &playv1.ScenePlayer{UserId: c.PlayerUserID, CharacterId: c.ID, CharacterName: c.Name, JoinedBySelf: !in.all && joined[c.PlayerUserID]}
		if in.has(c.PlayerUserID) {
			out.Inside = append(out.Inside, p)
		} else {
			out.Outside = append(out.Outside, p)
		}
	}
	return out, nil
}

// partyPlayerOf is the player of the character in the party, and whether the character is a party member's.
func partyPlayerOf(party []link.Character, characterID string) (string, bool) {
	i := slices.IndexFunc(party, func(c link.Character) bool { return c.ID == characterID })
	if i < 0 {
		return "", false
	}
	return party[i].PlayerUserID, true
}

// partyInScene keeps the characters whose players are in the scene open to only some players; with no such
// scene open, the whole party.
func (s *Service) partyInScene(ctx context.Context, q *playdb.Queries, session playdb.GameSession, party []link.Character) ([]link.Character, error) {
	if !partialScene(session) {
		return party, nil
	}
	in, err := scenePlayersOf(ctx, q, session.ID)
	if err != nil {
		return nil, err
	}
	return slices.DeleteFunc(slices.Clone(party), func(c link.Character) bool { return !in.has(c.PlayerUserID) }), nil
}

// SetScenePlayersMayJoin implements playv1connect.PlayServiceHandler.
func (s *Service) SetScenePlayersMayJoin(
	ctx context.Context,
	req *connect.Request[playv1.SetScenePlayersMayJoinRequest],
) (*connect.Response[playv1.SetScenePlayersMayJoinResponse], error) {
	m, err := authz.RequireCampaignRole(ctx, req.Msg.GetCampaignId(), authz.RoleMaster)
	if err != nil {
		return nil, err
	}
	changed := false
	err = db.InTx(ctx, s.pool, func(tx pgx.Tx) error {
		changed = false
		q := s.queries.WithTx(tx)
		session, err := lockOpenSessionOrNone(ctx, q, m.CampaignID)
		if err != nil {
			return err
		}
		if _, err := s.openSceneIn(ctx, tx, m.CampaignID, session); err != nil {
			return err
		}
		if !partialScene(session) {
			return notPartialScene()
		}
		if changed = session.ScenePlayersMayJoin != req.Msg.GetPlayersMayJoin(); !changed {
			return nil
		}
		_, err = q.SetScenePlayersMayJoin(ctx, playdb.SetScenePlayersMayJoinParams{ID: session.ID, ScenePlayersMayJoin: req.Msg.GetPlayersMayJoin()})
		return err
	})
	if err != nil {
		return nil, s.dbError(ctx, "set whether players may join the scene", err)
	}
	if changed {
		// Every player hears it: the ones out read the prompt appear or go.
		s.publishTo(m.CampaignID, true, allViewers().audiences(), sceneChangedEvent())
	}
	info, err := s.openSceneInfo(ctx, m)
	if err != nil {
		return nil, err
	}
	return connect.NewResponse(&playv1.SetScenePlayersMayJoinResponse{Scene: info}), nil
}

// JoinScene implements playv1connect.PlayServiceHandler.
func (s *Service) JoinScene(
	ctx context.Context,
	req *connect.Request[playv1.JoinSceneRequest],
) (*connect.Response[playv1.JoinSceneResponse], error) {
	m, err := authz.RequireCampaignRole(ctx, req.Msg.GetCampaignId(), authz.RolePlayer)
	if err != nil {
		return nil, err
	}
	var ch audienceChange
	err = db.InTx(ctx, s.pool, func(tx pgx.Tx) error {
		ch = audienceChange{}
		q := s.queries.WithTx(tx)
		session, err := lockOpenSessionOrNone(ctx, q, m.CampaignID)
		if err != nil {
			return err
		}
		scene, err := s.openSceneIn(ctx, tx, m.CampaignID, session)
		if err != nil {
			return err
		}
		before, err := sceneViewersOf(ctx, q, session)
		if err != nil {
			return err
		}
		if before.has(m.UserID) {
			return nil // already in
		}
		if !session.ScenePlayersMayJoin {
			return errNoOpenScene() // no trace of the scene
		}
		after := usersViewers(append(slices.Clone(before.users), m.UserID))
		if err := q.InsertScenePlayer(ctx, playdb.InsertScenePlayerParams{GameSessionID: session.ID, UserID: m.UserID, JoinedBySelf: true}); err != nil {
			return fmt.Errorf("join the scene: %w", err)
		}
		ch = audienceChange{before: before, after: after, changed: true, image: sceneImageMove(session, before, after)}
		return s.discoverScene(ctx, tx, m.CampaignID, scene.PointID, usersViewers([]string{m.UserID}), s.now()) // what they read, they read
	})
	if err != nil {
		return nil, s.dbError(ctx, "join the scene", err)
	}
	s.publishAudienceChange(ctx, m.CampaignID, ch)
	info, err := s.openSceneInfo(ctx, m)
	if err != nil {
		return nil, err
	}
	return connect.NewResponse(&playv1.JoinSceneResponse{Scene: info}), nil
}

// joinableScene is what a player out of a scene that lets players join reads of it: its name and that they may join.
func (s *Service) joinableScene(scene link.Scene) *playv1.OpenSceneInfo {
	return &playv1.OpenSceneInfo{Name: scene.Name, Joinable: true}
}
