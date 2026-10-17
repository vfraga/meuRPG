package play

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"uuid"

	"connectrpc.com/connect"
	"github.com/jackc/pgx/v5"

	playv1 "github.com/PuraFome/meuRPG/backend/gen/meurpg/play/v1"
	"github.com/PuraFome/meuRPG/backend/internal/authz"
	"github.com/PuraFome/meuRPG/backend/internal/platform/db"
	"github.com/PuraFome/meuRPG/backend/internal/platform/idem"
	"github.com/PuraFome/meuRPG/backend/internal/play/playdb"
)

const eventGameTimeAdvanced = "game_time_advanced"

// maxGameTimeStep is the most a call moves game time on: a day.
const maxGameTimeStep = 24 * 3600

type gameTimeEvent struct {
	Seconds int32 `json:"seconds"`
	Ended   int32 `json:"ended"`
}

// AdvanceGameTime implements playv1connect.LastingEffectServiceHandler.
func (s *Service) AdvanceGameTime(
	ctx context.Context,
	req *connect.Request[playv1.AdvanceGameTimeRequest],
) (*connect.Response[playv1.AdvanceGameTimeResponse], error) {
	m, err := authz.RequireCampaignRole(ctx, req.Msg.GetCampaignId(), authz.RoleMaster)
	if err != nil {
		return nil, err
	}
	key, err := uuid.Parse(req.Msg.GetIdempotencyKey())
	if err != nil {
		return nil, connect.NewError(connect.CodeInvalidArgument, errors.New("idempotency_key must be a UUID"))
	}
	if req.Msg.GetSeconds() < 1 || req.Msg.GetSeconds() > maxGameTimeStep {
		return nil, connect.NewError(connect.CodeInvalidArgument, errors.New("seconds must be 1 to 86400"))
	}
	keyText, hash := key.String(), idem.Hash(req.Msg)
	var done gameTimeDone
	err = db.InTx(ctx, s.pool, func(tx pgx.Tx) error {
		done = gameTimeDone{}
		return s.advanceGameTimeIn(ctx, tx, m, keyText, hash, req.Msg.GetSeconds(), &done)
	})
	if err != nil {
		return nil, s.dbError(ctx, "move game time on", err)
	}
	if done.told {
		s.publishCastsChanged(m.CampaignID, false)
		s.publishVitalsOf(m.CampaignID, done.vitals)
	}
	return connect.NewResponse(&playv1.AdvanceGameTimeResponse{EffectsEnded: done.ended}), nil
}

// gameTimeDone is what the transaction of AdvanceGameTime leaves for the answer and the streams.
type gameTimeDone struct {
	ended int32
	// told says game time moved on in this call (not a repeat of one), so the streams are told.
	told   bool
	vitals []*playv1.CharacterVitals
}

// advanceGameTimeIn is the transaction of AdvanceGameTime: the same key again answers with what
// the first call did; a combat that runs stops it.
func (s *Service) advanceGameTimeIn(ctx context.Context, tx pgx.Tx, m authz.Membership, keyText string, hash *string, seconds int32, done *gameTimeDone) error {
	q := s.queries.WithTx(tx)
	session, err := lockOpenSession(ctx, q, m.CampaignID)
	if err != nil {
		return err
	}
	first, again, err := eventByKey(ctx, q, session.ID, keyText, eventGameTimeAdvanced, hash)
	if err != nil {
		return err
	}
	if again {
		var ev gameTimeEvent
		_ = json.Unmarshal(first.Payload, &ev)
		done.ended = ev.Ended
		return nil
	}
	if err := activeCombatBlocks(ctx, q, session.ID); err != nil {
		return err
	}
	before, err := q.ListCharacterEffectsOfCampaign(ctx, m.CampaignID)
	if err != nil {
		return fmt.Errorf("list the effects: %w", err)
	}
	c, err := s.openTx(ctx, combatTx{tx: tx, q: q, session: session, now: s.now(), kind: eventGameTimeAdvanced, actorUserID: m.UserID, svc: s, master: true})
	if err != nil {
		return err
	}
	if err := s.advanceGameTime(ctx, c, seconds); err != nil {
		return err
	}
	done.vitals = c.told
	after, err := q.ListCharacterEffectsOfCampaign(ctx, m.CampaignID)
	if err != nil {
		return fmt.Errorf("list the effects: %w", err)
	}
	done.ended = int32(len(before) - len(after)) //nolint:gosec // a handful of effects
	if err := s.insertGameTimeEvent(ctx, q, session.ID, m.UserID, keyText, hash, gameTimeEvent{Seconds: seconds, Ended: done.ended}); err != nil {
		return err
	}
	done.told = true
	return nil
}

// insertGameTimeEvent writes the event that carries the key of the call.
func (s *Service) insertGameTimeEvent(ctx context.Context, q *playdb.Queries, sessionID, userID, keyText string, hash *string, ev gameTimeEvent) error {
	payload, err := json.Marshal(ev)
	if err != nil {
		return fmt.Errorf("encode the event payload: %w", err)
	}
	seq, err := q.NextSessionEventSeq(ctx, sessionID)
	if err != nil {
		return fmt.Errorf("next event number: %w", err)
	}
	if _, err := q.InsertSessionEvent(ctx, playdb.InsertSessionEventParams{
		GameSessionID: sessionID, Seq: seq, Kind: eventGameTimeAdvanced, ActorUserID: &userID,
		Payload: payload, IdempotencyKey: &keyText, IdempotencyHash: hash, CreatedAt: s.now(),
	}); err != nil {
		return fmt.Errorf("insert session event: %w", err)
	}
	return nil
}
