package play

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"math"
	"uuid"

	"connectrpc.com/connect"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"google.golang.org/protobuf/types/known/timestamppb"

	playv1 "github.com/PuraFome/meuRPG/backend/gen/meurpg/play/v1"
	"github.com/PuraFome/meuRPG/backend/internal/authz"
	"github.com/PuraFome/meuRPG/backend/internal/platform/db"
	"github.com/PuraFome/meuRPG/backend/internal/platform/idem"
	"github.com/PuraFome/meuRPG/backend/internal/platform/logging"
	"github.com/PuraFome/meuRPG/backend/internal/platform/rpcerr"
	"github.com/PuraFome/meuRPG/backend/internal/play/live"
	"github.com/PuraFome/meuRPG/backend/internal/play/playdb"
)

// Every handler starts with one explicit check, authz.RequireCampaignRole
// for what only the master may do and authz.RequireCampaignMember for
// reads. The check's error is already the right Connect error, so handlers
// return it as is.

// StartGameSession implements playv1connect.PlayServiceHandler.
func (s *Service) StartGameSession(
	ctx context.Context,
	req *connect.Request[playv1.StartGameSessionRequest],
) (*connect.Response[playv1.StartGameSessionResponse], error) {
	m, err := authz.RequireCampaignRole(ctx, req.Msg.GetCampaignId(), authz.RoleMaster)
	if err != nil {
		return nil, err
	}

	key, err := idem.Clean(req.Msg.GetIdempotencyKey())
	if err != nil {
		return nil, err
	}
	// The key is unique in the campaign, and kept with a hash of the whole request: a retry
	// returns the first session, open or ended by now, and starts no other.
	scopedKey, requestHash := idem.Scope(m.CampaignID, key), idem.Hash(req.Msg)

	var session playdb.GameSession
	var locked int64
	var replayed bool
	err = db.InTx(ctx, s.pool, func(tx pgx.Tx) error {
		locked = 0
		var err error
		session, replayed, err = idem.Create(ctx, scopedKey, requestHash, s.queries.WithTx(tx).GetGameSessionByCreateKey,
			func(gs playdb.GameSession) *string { return gs.CreateHash },
			func() (playdb.GameSession, error) {
				gs, n, err := s.openGameSession(ctx, tx, m.CampaignID, scopedKey, requestHash)
				locked = n
				return gs, err
			})
		return err
	})
	if isUniqueViolation(err) {
		// Another start of the same campaign won the race.
		return nil, errSessionOpen()
	}
	if err != nil {
		return nil, s.dbError(ctx, "start a game session", err)
	}
	// A campaign has a handful of player characters, so the count always
	// fits; the check only makes the conversion provably safe.
	var lockedCount int32
	if locked > 0 && locked <= math.MaxInt32 {
		lockedCount = int32(locked)
	}
	if replayed {
		// The first call started the session and locked the sheets: nothing more to do, and the
		// count of locked sheets is the first call's to tell (0 here).
		return connect.NewResponse(&playv1.StartGameSessionResponse{GameSession: sessionToProto(session)}), nil
	}
	logging.Event(ctx, s.logger, "session.started",
		slog.String("session_id", session.ID), slog.Int("session_number", int(session.SessionNumber)), slog.Int("sheets_locked", int(lockedCount)))
	return connect.NewResponse(&playv1.StartGameSessionResponse{
		GameSession:      sessionToProto(session),
		LockedSheetCount: lockedCount,
	}), nil
}

// openGameSession inserts the campaign's next session and locks the player sheets, and
// returns how many it locked.
func (s *Service) openGameSession(ctx context.Context, tx pgx.Tx, campaignID string, scopedKey, requestHash *string) (playdb.GameSession, int64, error) {
	q := s.queries.WithTx(tx)
	// One open session per campaign. This check gives the clear error;
	// the unique index makes it hold when two starts race (the caller).
	_, err := q.GetOpenGameSession(ctx, campaignID)
	switch {
	case err == nil:
		return playdb.GameSession{}, 0, errSessionOpen()
	case !errors.Is(err, pgx.ErrNoRows):
		return playdb.GameSession{}, 0, fmt.Errorf("find the open session: %w", err)
	}
	number, err := q.NextSessionNumber(ctx, campaignID)
	if err != nil {
		return playdb.GameSession{}, 0, fmt.Errorf("next session number: %w", err)
	}
	now := s.now()
	gs, err := q.InsertGameSession(ctx, playdb.InsertGameSessionParams{
		CampaignID: campaignID, SessionNumber: number, StartedAt: now, CreateKey: scopedKey, CreateHash: requestHash,
	})
	if err != nil && !errors.Is(err, pgx.ErrNoRows) {
		return gs, 0, fmt.Errorf("insert game session: %w", err)
	}
	if err != nil {
		return gs, 0, err
	}
	// RN-01, in the same transaction: the session and the lock happen
	// together, or not at all.
	locked, err := s.sheets.LockSheets(ctx, tx, campaignID, now)
	if err != nil {
		return gs, 0, fmt.Errorf("lock sheets: %w", err)
	}
	return gs, locked, nil
}

// EndGameSession implements playv1connect.PlayServiceHandler.
func (s *Service) EndGameSession(
	ctx context.Context,
	req *connect.Request[playv1.EndGameSessionRequest],
) (*connect.Response[playv1.EndGameSessionResponse], error) {
	m, err := authz.RequireCampaignRole(ctx, req.Msg.GetCampaignId(), authz.RoleMaster)
	if err != nil {
		return nil, err
	}
	id, err := uuid.Parse(req.Msg.GetGameSessionId())
	if err != nil {
		return nil, errSessionNotFound()
	}

	var out sessionEnd
	err = db.InTx(ctx, s.pool, func(tx pgx.Tx) error {
		out = sessionEnd{}
		return s.endSessionIn(ctx, tx, m, id.String(), &out)
	})
	if err != nil {
		return nil, s.dbError(ctx, "end a game session", err)
	}
	res := sessionToProto(out.session)
	if out.ended {
		logging.Event(ctx, s.logger, "session.ended", slog.String("session_id", out.session.ID))
		s.publishCreaturesOfCombat(m.CampaignID, out.fought) // the combat wrote their hit points back
		for _, v := range out.told {
			s.publishVitals(m.CampaignID, v)
		}
		// Every stream of the session ends (WatchGameSession).
		s.hub.Publish(m.CampaignID, live.Event{
			Audience: live.Audience{Everyone: true},
			Message: &playv1.WatchGameSessionResponse{Event: &playv1.WatchGameSessionResponse_SessionEnded_{
				SessionEnded: &playv1.WatchGameSessionResponse_SessionEnded{GameSession: res},
			}},
		})
	}
	return connect.NewResponse(&playv1.EndGameSessionResponse{GameSession: res}), nil
}

// sessionEnd is what ending a session did.
type sessionEnd struct {
	session playdb.GameSession
	ended   bool                      // this call ended an open session
	fought  []playdb.Combatant        // the combatants of the combat the session ended, for the creatures' owners
	told    []*playv1.CharacterVitals // the forms and sights the combat's end ended
}

// endSessionIn ends the session inside tx, with its combat and the image the master
// left with the players. A session ended before is read back and nothing changes.
func (s *Service) endSessionIn(ctx context.Context, tx pgx.Tx, m authz.Membership, sessionID string, out *sessionEnd) error {
	q := s.queries.WithTx(tx)
	// Lock the row first: a vitals correction in progress finishes
	// before the session ends, and this call learns whether the session
	// was still open.
	current, err := q.GetGameSessionForUpdate(ctx, playdb.GetGameSessionForUpdateParams{CampaignID: m.CampaignID, ID: sessionID})
	if errors.Is(err, pgx.ErrNoRows) {
		return errSessionNotFound()
	}
	if err != nil {
		return fmt.Errorf("find game session: %w", err)
	}
	if current.EndedAt != nil {
		out.session = current // ended before: nothing changes
		return nil
	}
	// A combat ends with its session; the player characters' tokens move to
	// where they stood (MR-013).
	if out.fought, out.told, err = s.endOpenEncounter(ctx, tx, q, current, m.UserID); err != nil {
		return err
	}
	if current.ShownImageKeep && current.ShownImageID != nil {
		// The image left with the players outlives the session (MR-028).
		if err := s.maps.LeaveImage(ctx, tx, m.CampaignID, *current.ShownImageID, s.now()); err != nil {
			return err
		}
	}
	out.session, err = q.EndGameSession(ctx, playdb.EndGameSessionParams{
		CampaignID: m.CampaignID, ID: sessionID, Now: s.now(),
	})
	if err != nil {
		return fmt.Errorf("end game session: %w", err)
	}
	out.ended = true
	return nil
}

// ListGameSessions implements playv1connect.PlayServiceHandler.
func (s *Service) ListGameSessions(
	ctx context.Context,
	req *connect.Request[playv1.ListGameSessionsRequest],
) (*connect.Response[playv1.ListGameSessionsResponse], error) {
	m, err := authz.RequireCampaignMember(ctx, req.Msg.GetCampaignId())
	if err != nil {
		return nil, err
	}
	sessions, err := s.queries.ListGameSessions(ctx, m.CampaignID)
	if err != nil {
		return nil, s.dbError(ctx, "list game sessions", err)
	}
	res := &playv1.ListGameSessionsResponse{}
	for _, session := range sessions {
		res.GameSessions = append(res.GameSessions, sessionToProto(session))
	}
	return connect.NewResponse(res), nil
}

func sessionToProto(s playdb.GameSession) *playv1.GameSession {
	out := &playv1.GameSession{
		Id:            s.ID,
		CampaignId:    s.CampaignID,
		SessionNumber: s.SessionNumber,
		StartedAt:     timestamppb.New(s.StartedAt),
	}
	if s.EndedAt != nil {
		out.EndedAt = timestamppb.New(*s.EndedAt)
	}
	return out
}

// errSessionOpen is StartGameSession's failed_precondition.
func errSessionOpen() error {
	return errBlocked(playv1.GameSessionBlockedReason_GAME_SESSION_BLOCKED_REASON_SESSION_ALREADY_OPEN,
		"the campaign already has an open game session; end it first")
}

// errNoOpenSession is the live session's failed_precondition: the campaign
// has no open session.
func errNoOpenSession() error {
	return errBlocked(playv1.GameSessionBlockedReason_GAME_SESSION_BLOCKED_REASON_NO_OPEN_SESSION,
		"the campaign has no open game session")
}

// errKeyReused is the refusal of an idempotency key that was used for another change.
func errKeyReused() error {
	return connect.NewError(connect.CodeInvalidArgument, errors.New("idempotency_key was already used for another change"))
}

// errBlocked is PlayService's failed_precondition, with the
// GameSessionBlocked detail that tells the app why.
func errBlocked(reason playv1.GameSessionBlockedReason, msg string) error {
	err := connect.NewError(connect.CodeFailedPrecondition, errors.New(msg))
	if detail, detailErr := connect.NewErrorDetail(&playv1.GameSessionBlocked{Reason: reason}); detailErr == nil {
		err.AddDetail(detail)
	}
	return err
}

// errSessionNotFound is the answer for a session that is not in the
// campaign.
func errSessionNotFound() error {
	return connect.NewError(connect.CodeNotFound, errors.New("game session not found"))
}

// isUniqueViolation reports whether err is a unique violation (23505).
// Inside StartGameSession's transaction only game_sessions' unique indexes
// can raise it (one open session per campaign, and the session number), so
// it means that another start of the same campaign won the race.
func isUniqueViolation(err error) bool {
	pgErr, ok := errors.AsType[*pgconn.PgError](err)
	return ok && pgErr.Code == "23505"
}

// dbError maps an error from the database to the Connect error the client gets
// (see platform/rpcerr).
func (s *Service) dbError(ctx context.Context, action string, err error) error {
	return rpcerr.FromDB(ctx, s.logger, "play", action, err)
}
