package play

import (
	"context"
	"crypto/sha256"
	"errors"
	"log/slog"
	"time"
	"uuid"

	"connectrpc.com/connect"
	"github.com/jackc/pgx/v5"

	playv1 "github.com/PuraFome/meuRPG/backend/gen/meurpg/play/v1"
	"github.com/PuraFome/meuRPG/backend/internal/authz"
	"github.com/PuraFome/meuRPG/backend/internal/platform/db"
	"github.com/PuraFome/meuRPG/backend/internal/platform/idem"
	"github.com/PuraFome/meuRPG/backend/internal/platform/logging"
	"github.com/PuraFome/meuRPG/backend/internal/platform/ratelimit"
	"github.com/PuraFome/meuRPG/backend/internal/play/live"
	"github.com/PuraFome/meuRPG/backend/internal/play/playdb"
)

// freeRollDeps is the free dice roller's rate limit in the Service.
type freeRollDeps struct {
	limit *ratelimit.Limiter
	hit   *ratelimit.Notifier
}

// allowFreeRoll applies the per-user limit of rolls.
func (s *Service) allowFreeRoll(ctx context.Context, userID string) error {
	if s.free.limit == nil {
		return nil
	}
	if ok, wait := s.free.limit.Allow(userID); !ok {
		s.free.hit.Hit(ctx)
		return ratelimit.RPCError(wait)
	}
	return nil
}

// freeRollCall is what RollFreeDice reads from its request before the transaction.
type freeRollCall struct {
	m       authz.Membership
	ask     freeRollAsk
	secret  bool
	keyText string
	hash    *string
}

// freeRollRun is what the transaction leaves for after the commit.
type freeRollRun struct {
	id       string
	at       time.Time
	ev       freeRollEvent
	repeated bool
}

// freeRollKey turns a member's key (1 to 64 characters) into the UUID the session_events column
// keeps: a hash of the member and the key, so two members may use the same text and one member's
// key is unique in the session.
func freeRollKey(userID, key string) string {
	sum := sha256.Sum256([]byte(userID + ":" + key))
	var id uuid.UUID
	copy(id[:], sum[:])
	id[6] = id[6]&0x0f | 0x50 // version 5 (a name-based hash)
	id[8] = id[8]&0x3f | 0x80 // variant of RFC 9562
	return id.String()
}

// readFreeRollCall makes RollFreeDice's checks in order: who the caller is, the rate limit, the
// master's "Só eu", the dice and the key.
func (s *Service) readFreeRollCall(ctx context.Context, req *playv1.RollFreeDiceRequest) (freeRollCall, error) {
	m, err := authz.RequireCampaignMember(ctx, req.GetCampaignId())
	if err != nil {
		return freeRollCall{}, err
	}
	if err := s.allowFreeRoll(ctx, m.UserID); err != nil {
		return freeRollCall{}, err
	}
	if req.GetSecret() && m.Role != authz.RoleMaster {
		return freeRollCall{}, connect.NewError(connect.CodePermissionDenied, errors.New("only the master may roll in secret"))
	}
	ask, err := parseFreeRoll(req.GetSides(), req.GetCount(), req.GetModifier(), req.GetMode())
	if err != nil {
		return freeRollCall{}, err
	}
	key, err := idem.Clean(req.GetIdempotencyKey())
	if err != nil {
		return freeRollCall{}, err
	}
	if key == "" {
		return freeRollCall{}, connect.NewError(connect.CodeInvalidArgument, errors.New("idempotency_key is required"))
	}
	return freeRollCall{m: m, ask: ask, secret: req.GetSecret(), keyText: freeRollKey(m.UserID, key), hash: idem.Hash(req)}, nil
}

// RollFreeDice implements playv1connect.PlayServiceHandler: dice the server rolls on request, outside
// every game rule (MR-052). The table's dice mode (RN-18) is not asked: the app always rolls.
func (s *Service) RollFreeDice(
	ctx context.Context,
	req *connect.Request[playv1.RollFreeDiceRequest],
) (*connect.Response[playv1.RollFreeDiceResponse], error) {
	call, err := s.readFreeRollCall(ctx, req.Msg)
	if err != nil {
		return nil, err
	}
	var run freeRollRun
	err = s.txOpen(ctx, call.m.CampaignID, func(tx pgx.Tx, h *hints, session playdb.GameSession) error {
		run = freeRollRun{} // a retry starts over
		return s.freeRollTx(ctx, tx, h, session, call, &run)
	})
	if err != nil {
		return nil, s.dbError(ctx, "roll free dice", err)
	}
	roll := freeRollToProto(run.id, s.freeRollerName(ctx, call.m.UserID, run.ev.ByMaster), run.at, run.ev)
	if !run.repeated {
		logging.Event(ctx, s.logger, "free_roll.rolled", slog.Int("sides", int(run.ev.Sides)), slog.Int("count", int(run.ev.Count)), slog.Bool("secret", run.ev.Secret))
	}
	return connect.NewResponse(&playv1.RollFreeDiceResponse{Roll: roll}), nil
}

// freeRollTx is the transaction of RollFreeDice: find the roll of a retry of the key, or roll and write the event, and have the
// roll told once it committed (again for a retry: a table roll to the table, a secret one to nobody).
func (s *Service) freeRollTx(ctx context.Context, tx pgx.Tx, h *hints, session playdb.GameSession, call freeRollCall, run *freeRollRun) error {
	q := s.queries.WithTx(tx)
	done, again, err := eventByKey(ctx, q, session.ID, call.keyText, eventFreeRoll, call.hash)
	if err != nil {
		return err
	}
	if again {
		run.repeated, run.id, run.at = true, done.ID, done.CreatedAt
		h.replay()
		if err := decodeFreeRoll(done.Payload, &run.ev); err != nil {
			return err
		}
		s.tellFreeRoll(h, call, run)
		return nil
	}
	ev, err := rollFree(s.roller, call.ask)
	if err != nil {
		return err
	}
	ev.Secret, ev.ByMaster = call.secret, call.m.Role == authz.RoleMaster
	run.ev = ev
	run.id, run.at, err = s.insertKeyedEvent(ctx, q, keyedEvent{
		sessionID: session.ID, kind: eventFreeRoll, userID: call.m.UserID, key: call.keyText, hash: call.hash,
	}, ev)
	if err != nil {
		return err
	}
	h.touch() // a secret roll tells nobody, and the master's list changed
	s.tellFreeRoll(h, call, run)
	return nil
}

// tellFreeRoll has the roll told to the streams after the commit: a table roll whole, and a secret roll to nobody (RN-10). The
// roller's name is read then, outside the transaction.
func (s *Service) tellFreeRoll(h *hints, call freeRollCall, run *freeRollRun) {
	ev, id, at := run.ev, run.id, run.at
	aud, ok := ev.audience()
	if !ok {
		return
	}
	h.after(func(ctx context.Context, say sayFn) {
		roll := freeRollToProto(id, s.freeRollerName(ctx, call.m.UserID, ev.ByMaster), at, ev)
		say(call.m.CampaignID, live.Event{
			Audience: aud,
			Message: &playv1.WatchGameSessionResponse{Event: &playv1.WatchGameSessionResponse_FreeRollRolled_{
				FreeRollRolled: &playv1.WatchGameSessionResponse_FreeRollRolled{Roll: roll},
			}},
		})
	})
}

// freeRollerName is the display name of a roller, "Mestre" or "Jogador" when there is none.
func (s *Service) freeRollerName(ctx context.Context, userID string, byMaster bool) string {
	names := s.freeRollNames(ctx, []string{userID})
	return freeRollRollerName(names[userID], byMaster)
}

// freeRollNames reads display names; a name that cannot be read is left out, and the roll says
// "Mestre" or "Jogador" instead of failing: the roll is the answer, the name is a label.
func (s *Service) freeRollNames(ctx context.Context, userIDs []string) map[string]string {
	if s.msgs.profiles == nil {
		return nil
	}
	names, err := s.msgs.profiles.DisplayNames(ctx, userIDs)
	if err != nil {
		s.logger.WarnContext(ctx, "read the display names of free rolls", slog.Any("error", err))
		return nil
	}
	return names
}

// ListFreeRolls implements playv1connect.PlayServiceHandler: the free rolls of the open session the
// caller may read (MR-052). The SQL leaves the secret rolls out of a player's list, and the shaping
// checks it again (visibleTo): a player never gets one, nor a count of them.
func (s *Service) ListFreeRolls(
	ctx context.Context,
	req *connect.Request[playv1.ListFreeRollsRequest],
) (*connect.Response[playv1.ListFreeRollsResponse], error) {
	m, err := authz.RequireCampaignMember(ctx, req.Msg.GetCampaignId())
	if err != nil {
		return nil, err
	}
	v := viewer{master: m.Role == authz.RoleMaster, userID: m.UserID}
	var table, mine []playdb.ListFreeRollEventsRow
	err = db.InTx(ctx, s.pool, func(tx pgx.Tx) error {
		table, mine = nil, nil
		return s.readFreeRolls(ctx, tx, m, v, &table, &mine)
	})
	if err != nil {
		return nil, s.dbError(ctx, "list free rolls", err)
	}
	ids := make([]string, 0, len(table))
	for _, r := range table {
		if r.ActorUserID != nil {
			ids = append(ids, *r.ActorUserID)
		}
	}
	names := s.freeRollNames(ctx, ids)
	out := &playv1.ListFreeRollsResponse{}
	if out.Table, err = freeRollViews(table, names, v); err != nil {
		return nil, err
	}
	if out.Mine, err = freeRollViews(mine, names, v); err != nil {
		return nil, err
	}
	return connect.NewResponse(out), nil
}

// readFreeRolls reads, in one read-only transaction, the open session's rolls the viewer may read
// and the caller's own. The caller's own are among the newest 50 or not, so they are asked for.
func (s *Service) readFreeRolls(ctx context.Context, tx pgx.Tx, m authz.Membership, v viewer, table, mine *[]playdb.ListFreeRollEventsRow) error {
	q := s.queries.WithTx(tx)
	session, err := q.GetOpenGameSession(ctx, m.CampaignID)
	if errors.Is(err, pgx.ErrNoRows) {
		return errNoOpenSession()
	}
	if err != nil {
		return err
	}
	if *table, err = q.ListFreeRollEvents(ctx, playdb.ListFreeRollEventsParams{GameSessionID: session.ID, OnlyTable: !v.master, Limit: freeRollListLimit}); err != nil {
		return err
	}
	own, err := q.ListUserFreeRollEvents(ctx, playdb.ListUserFreeRollEventsParams{GameSessionID: session.ID, ActorUserID: &m.UserID, Limit: freeRollMineLimit})
	for _, r := range own {
		*mine = append(*mine, playdb.ListFreeRollEventsRow(r))
	}
	return err
}

// freeRollViews shapes rows for v, leaving out any roll v may not read.
func freeRollViews(rows []playdb.ListFreeRollEventsRow, names map[string]string, v viewer) ([]*playv1.FreeRoll, error) {
	out := make([]*playv1.FreeRoll, 0, len(rows))
	for _, r := range rows {
		var ev freeRollEvent
		if err := decodeFreeRoll(r.Payload, &ev); err != nil {
			return nil, err
		}
		if !ev.visibleTo(v) {
			continue
		}
		out = append(out, freeRollToProto(r.ID, freeRollRollerName(names[deref(r.ActorUserID)], ev.ByMaster), r.CreatedAt, ev))
	}
	return out, nil
}
