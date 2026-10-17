package play

import (
	"context"
	"errors"
	"fmt"
	"time"

	"connectrpc.com/connect"
	"github.com/jackc/pgx/v5"

	campaignsv1 "github.com/PuraFome/meuRPG/backend/gen/meurpg/campaigns/v1"
	playv1 "github.com/PuraFome/meuRPG/backend/gen/meurpg/play/v1"
	"github.com/PuraFome/meuRPG/backend/internal/authz"
	"github.com/PuraFome/meuRPG/backend/internal/platform/db"
	"github.com/PuraFome/meuRPG/backend/internal/play/live"
	"github.com/PuraFome/meuRPG/backend/internal/play/playdb"
)

// The live session (Etapa 5; ADR-0005, docs/architecture/play/live-session.md#live-session).
//
//   - The notice (RN-06) is a light poll, not a stream: the app calls
//     ListOpenGameSessions about every 30 seconds while its tab is visible.
//   - The session page opens WatchGameSession, waits for `ready`, then reads
//     the snapshot (GetLiveSession). Events patch the snapshot; after any
//     reconnection the app reads the snapshot again, so a missed event
//     never leaves the screen stale.
//   - Who sees what is decided here, on the server: the master sees every
//     living player character's vitals; a player, only their own
//     character's (question 28 for Samuel; the default is "no").

// errWatchSlow ends a stream that fell behind (live.ErrSlow): the app
// reconnects and reads the snapshot again.
func errWatchSlow() error {
	return connect.NewError(connect.CodeUnavailable, errors.New("the stream fell behind; reconnect"))
}

// errWatchReplaced ends a stream that a newer stream of the same user replaced
// (live.ErrReplaced, past live.DefaultMaxPerUser). It is almost always the
// stream of a page that already went away (the server is not told when one
// does); if the tab is still open, the app reconnects with its backoff, like
// after any transient end.
func errWatchReplaced() error {
	return connect.NewError(connect.CodeUnavailable, errors.New("replaced by a newer stream of this user; reconnect"))
}

// ListOpenGameSessions implements playv1connect.PlayServiceHandler.
func (s *Service) ListOpenGameSessions(
	ctx context.Context,
	_ *connect.Request[playv1.ListOpenGameSessionsRequest],
) (*connect.Response[playv1.ListOpenGameSessionsResponse], error) {
	userID, err := authz.RequireSignedIn(ctx)
	if err != nil {
		return nil, err
	}
	campaigns, err := s.campaigns.ActiveCampaigns(ctx, userID)
	if err != nil {
		return nil, s.dbError(ctx, "list the caller's campaigns", err)
	}
	res := &playv1.ListOpenGameSessionsResponse{}
	if len(campaigns) == 0 {
		return connect.NewResponse(res), nil
	}
	byID := make(map[string]*campaignsv1.Campaign, len(campaigns))
	ids := make([]string, 0, len(campaigns))
	for _, c := range campaigns {
		byID[c.GetId()] = c
		ids = append(ids, c.GetId())
	}
	sessions, err := s.queries.ListOpenGameSessions(ctx, ids)
	if err != nil {
		return nil, s.dbError(ctx, "list open game sessions", err)
	}
	for _, session := range sessions {
		c := byID[session.CampaignID]
		res.OpenGameSessions = append(res.OpenGameSessions, &playv1.OpenGameSession{
			GameSession:  sessionToProto(session),
			CampaignName: c.GetName(),
			MyRole:       c.GetMyRole(),
		})
	}
	return connect.NewResponse(res), nil
}

// GetLiveSession implements playv1connect.PlayServiceHandler.
func (s *Service) GetLiveSession(
	ctx context.Context,
	req *connect.Request[playv1.GetLiveSessionRequest],
) (*connect.Response[playv1.GetLiveSessionResponse], error) {
	m, err := authz.RequireCampaignMember(ctx, req.Msg.GetCampaignId())
	if err != nil {
		return nil, err
	}
	session, err := s.openSession(ctx, m.CampaignID)
	if err != nil {
		return nil, err
	}
	all, err := s.vitals.ListVitals(ctx, m.CampaignID)
	if err != nil {
		return nil, s.dbError(ctx, "list vitals", err)
	}
	res := &playv1.GetLiveSessionResponse{GameSession: sessionToProto(session)}
	if session.CurrentMapID != nil {
		// Every member may see it: setting a map current reveals it, and a
		// player sees the current map even if the master hides it again.
		res.CurrentMapId = *session.CurrentMapID
	}
	if session.ShownImageID != nil {
		res.ShownImage, err = s.maps.ShownImage(ctx, m.CampaignID, *session.ShownImageID)
		if connect.CodeOf(err) == connect.CodeNotFound {
			res.ShownImage, err = nil, nil // deleted since the read above
		}
		if err != nil {
			return nil, s.dbError(ctx, "read the shown image", err)
		}
	}
	// The switch is the master's: players get false.
	res.ShownImageKeep = m.Role == authz.RoleMaster && session.ShownImageKeep && res.ShownImage != nil
	for _, v := range all {
		if maySee(m, v) {
			res.Vitals = append(res.Vitals, v)
		}
	}
	return connect.NewResponse(res), nil
}

// maySee says whether the member may see a character's vitals: the master
// sees everyone's; a player, only their own character's.
func maySee(m authz.Membership, v *playv1.CharacterVitals) bool {
	return m.Role == authz.RoleMaster || (v.GetPlayerUserId() != "" && v.GetPlayerUserId() == m.UserID)
}

// vitalsAudience is who receives a character's new vitals: the master and
// the character's player.
func vitalsAudience(v *playv1.CharacterVitals) live.Audience {
	return live.Audience{Master: true, UserID: v.GetPlayerUserId()}
}

// openSession returns the campaign's open session, or the NO_OPEN_SESSION
// failed_precondition.
func (s *Service) openSession(ctx context.Context, campaignID string) (playdb.GameSession, error) {
	session, err := openSessionWith(ctx, s.queries, campaignID)
	if err != nil {
		return playdb.GameSession{}, s.dbError(ctx, "find the open session", err) // a connect error passes through
	}
	return session, nil
}

// openSessionWith is openSession on the queries of a read-only transaction
// (db.ReadTx). It returns the NO_OPEN_SESSION error as is and any database
// error wrapped, for the caller to map once the transaction is over.
func openSessionWith(ctx context.Context, q *playdb.Queries, campaignID string) (playdb.GameSession, error) {
	session, err := q.GetOpenGameSession(ctx, campaignID)
	if errors.Is(err, pgx.ErrNoRows) {
		return playdb.GameSession{}, errNoOpenSession()
	}
	if err != nil {
		return playdb.GameSession{}, fmt.Errorf("find the open session: %w", err)
	}
	return session, nil
}

// lockOpenSession locks the campaign's open game session inside the transaction of q, or
// refuses with the answer of a table without one.
func lockOpenSession(ctx context.Context, q *playdb.Queries, campaignID string) (playdb.GameSession, error) {
	return lockOpenSessionOr(ctx, q, campaignID, errNoOpenSession)
}

// lockOpenSessionOr is lockOpenSession for a call that tells the lack of a session in its own
// words: noSession builds the refusal.
func lockOpenSessionOr(ctx context.Context, q *playdb.Queries, campaignID string, noSession func() error) (playdb.GameSession, error) {
	session, err := q.GetOpenGameSessionForUpdate(ctx, campaignID)
	if errors.Is(err, pgx.ErrNoRows) {
		return playdb.GameSession{}, noSession()
	}
	if err != nil {
		return playdb.GameSession{}, fmt.Errorf("lock the open session: %w", err)
	}
	return session, nil
}

// encounterInSessionWith finds an encounter of the open session, or the
// "encounter not found" error. Like openSessionWith, for a read-only transaction.
func encounterInSessionWith(ctx context.Context, q *playdb.Queries, sessionID, encounterID string) (playdb.Encounter, error) {
	enc, err := q.GetEncounterInSession(ctx, playdb.GetEncounterInSessionParams{GameSessionID: sessionID, ID: encounterID})
	if errors.Is(err, pgx.ErrNoRows) {
		return playdb.Encounter{}, connect.NewError(connect.CodeNotFound, errors.New("encounter not found"))
	}
	if err != nil {
		return playdb.Encounter{}, fmt.Errorf("find the encounter: %w", err)
	}
	return enc, nil
}

// readEncounter reads the open session, one of its encounters and the
// encounter's combatants in one read-only transaction (audit D-03), so a combat
// the master changes while the request runs is seen whole or not at all: never
// new combatants beside the old revision.
func (s *Service) readEncounter(ctx context.Context, campaignID, encounterID string) (playdb.GameSession, *encounterData, error) {
	var (
		session playdb.GameSession
		d       *encounterData
	)
	err := db.ReadTx(ctx, s.pool, func(tx pgx.Tx) error {
		q := s.queries.WithTx(tx)
		var err error
		if session, err = openSessionWith(ctx, q, campaignID); err != nil {
			return err
		}
		enc, err := encounterInSessionWith(ctx, q, session.ID, encounterID)
		if err != nil {
			return err
		}
		d, err = loadEncounter(ctx, q, enc)
		return err
	})
	if err != nil {
		return playdb.GameSession{}, nil, s.dbError(ctx, "read the encounter", err)
	}
	return session, d, nil
}

// WatchGameSession implements playv1connect.PlayServiceHandler. See the
// method's comment in play.proto for what the app sees.
func (s *Service) WatchGameSession(
	ctx context.Context,
	req *connect.Request[playv1.WatchGameSessionRequest],
	stream *connect.ServerStream[playv1.WatchGameSessionResponse],
) error {
	m, err := authz.RequireCampaignMember(ctx, req.Msg.GetCampaignId())
	if err != nil {
		return err
	}
	// Subscribe before reading the session: an end that happens after the
	// read below is then always delivered to this stream.
	sub, err := s.hub.Subscribe(m.CampaignID, live.Subscriber{UserID: m.UserID, Master: m.Role == authz.RoleMaster})
	if err != nil {
		return nil // the server is shutting down: the app reconnects to the next one
	}
	defer sub.Close()
	session, err := s.openSession(ctx, m.CampaignID)
	if err != nil {
		return err
	}
	if err := stream.Send(&playv1.WatchGameSessionResponse{Event: &playv1.WatchGameSessionResponse_Ready_{
		Ready: &playv1.WatchGameSessionResponse_Ready{GameSession: sessionToProto(session)},
	}}); err != nil {
		return nil // the app went away
	}
	return s.watchLoop(ctx, stream, m, sub, session)
}

// watchLoop sends the stream's events, its heartbeat and its checks until the
// stream is over.
func (s *Service) watchLoop(ctx context.Context, stream *connect.ServerStream[playv1.WatchGameSessionResponse], m authz.Membership, sub *live.Subscription, session playdb.GameSession) error {
	heartbeat := time.NewTicker(s.live.Heartbeat)
	defer heartbeat.Stop()
	recheck := time.NewTicker(s.live.Recheck)
	defer recheck.Stop()
	lifetime := time.NewTimer(s.live.MaxLifetime)
	defer lifetime.Stop()

	for {
		select {
		case <-ctx.Done():
			return nil // the app closed the stream
		case <-lifetime.C:
			return nil // the app opens a new one
		case ev, ok := <-sub.Events():
			if done, err := forwardHubEvent(stream, sub, session.ID, ev, ok); done {
				return err
			}
		case <-heartbeat.C:
			if err := stream.Send(&playv1.WatchGameSessionResponse{Event: &playv1.WatchGameSessionResponse_Heartbeat_{
				Heartbeat: &playv1.WatchGameSessionResponse_Heartbeat{},
			}}); err != nil {
				return nil
			}
		case <-recheck.C:
			if done, err := s.recheckWatch(ctx, stream, m.CampaignID, session.ID); done {
				return err
			}
		}
	}
}

// forwardHubEvent sends an event of the hub to the app. It says whether the
// stream is over, and why: a nil error is an ordinary end.
func forwardHubEvent(stream *connect.ServerStream[playv1.WatchGameSessionResponse], sub *live.Subscription, sessionID string, ev live.Event, ok bool) (bool, error) {
	if !ok {
		switch err := sub.Err(); {
		case errors.Is(err, live.ErrSlow):
			return true, errWatchSlow()
		case errors.Is(err, live.ErrReplaced):
			return true, errWatchReplaced()
		}
		return true, nil // the server is shutting down
	}
	sub.Taken(ev) // a coalesced hint that follows is queued again
	if ended := ev.Message.GetSessionEnded(); ended != nil && ended.GetGameSession().GetId() != sessionID {
		return false, nil // another session of the campaign; not this stream's
	}
	if err := stream.Send(ev.Message); err != nil {
		return true, nil //nolint:nilerr // the app went away
	}
	return ev.Message.GetSessionEnded() != nil, nil
}

// recheckWatch is what a stream does every Recheck: it reads the login again
// and the open session. It says whether the stream is over, and why.
func (s *Service) recheckWatch(ctx context.Context, stream *connect.ServerStream[playv1.WatchGameSessionResponse], campaignID, sessionID string) (bool, error) {
	if _, err := authz.RecheckCampaignMember(ctx, campaignID); err != nil {
		return true, err // unauthenticated or not_found, as a new call would get
	}
	// A session that ended without this stream hearing about it
	// (it cannot happen with one server; it costs one read).
	current, err := s.queries.GetOpenGameSession(ctx, campaignID)
	switch {
	case errors.Is(err, pgx.ErrNoRows) || (err == nil && current.ID != sessionID):
		return true, s.sendSessionEnded(ctx, stream, sessionID, campaignID)
	case err != nil:
		return true, s.dbError(ctx, "check the open session", err)
	}
	return false, nil
}

// sendSessionEnded sends session_ended for a session that ended while the
// stream missed the event, read again for its ended_at, and ends the
// stream.
func (s *Service) sendSessionEnded(ctx context.Context, stream *connect.ServerStream[playv1.WatchGameSessionResponse], sessionID, campaignID string) error {
	session, err := s.queries.GetGameSessionInCampaign(ctx, playdb.GetGameSessionInCampaignParams{CampaignID: campaignID, ID: sessionID})
	if errors.Is(err, pgx.ErrNoRows) {
		return nil // the session is gone (the campaign was deleted): nothing to say
	}
	if err != nil {
		return s.dbError(ctx, "read the ended session", err)
	}
	// A failed send means the app already left: nothing to do.
	_ = stream.Send(&playv1.WatchGameSessionResponse{Event: &playv1.WatchGameSessionResponse_SessionEnded_{
		SessionEnded: &playv1.WatchGameSessionResponse_SessionEnded{GameSession: sessionToProto(session)},
	}})
	return nil
}
