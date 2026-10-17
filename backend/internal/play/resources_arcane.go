package play

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"slices"
	"uuid"

	"connectrpc.com/connect"
	"github.com/jackc/pgx/v5"

	playv1 "github.com/PuraFome/meuRPG/backend/gen/meurpg/play/v1"
	"github.com/PuraFome/meuRPG/backend/internal/authz"
	"github.com/PuraFome/meuRPG/backend/internal/platform/db"
	"github.com/PuraFome/meuRPG/backend/internal/platform/idem"
	"github.com/PuraFome/meuRPG/backend/internal/platform/logging"
	"github.com/PuraFome/meuRPG/backend/internal/play/playdb"
	"github.com/PuraFome/meuRPG/backend/internal/rules"
)

// arcaneRecoveryEvent is the payload of an arcane_recovery_used event: the slots that
// came back (level and count) and their combined level. Numbers only.
type arcaneRecoveryEvent struct {
	Slots  []arcaneSlot `json:"slots"`
	Levels int32        `json:"levels"`
}

type arcaneSlot struct {
	Level int32 `json:"level"`
	Count int32 `json:"count"`
}

// arcaneSlots reads the slots of the request: one entry per level, a count of at least 1.
func arcaneSlots(req *playv1.UseArcaneRecoveryRequest) (map[int]int, error) {
	if len(req.GetSlots()) == 0 || len(req.GetSlots()) > maxSlotLevel {
		return nil, connect.NewError(connect.CodeInvalidArgument, errors.New("slots must name one to nine spell levels"))
	}
	out := map[int]int{}
	for _, s := range req.GetSlots() {
		level, count := int(s.GetLevel()), int(s.GetCount())
		if level < 1 || level > maxSlotLevel {
			return nil, connect.NewError(connect.CodeInvalidArgument, errors.New("a slot level must be 1 to 9"))
		}
		if count < 1 || count > maxSlotsPerLevel {
			return nil, connect.NewError(connect.CodeInvalidArgument, errors.New("the count of slots of a level must be at least 1"))
		}
		if _, dup := out[level]; dup {
			return nil, connect.NewError(connect.CodeInvalidArgument, errors.New("each slot level may appear once"))
		}
		out[level] = count
	}
	return out, nil
}

// maxSlotsPerLevel bounds the count a request names (no sheet has more slots of a level).
const maxSlotsPerLevel = 9

// arcaneCall is what UseArcaneRecovery reads from its request before the transaction.
type arcaneCall struct {
	m        authz.Membership
	charText string
	keyText  string
	hash     *string
	slots    map[int]int
}

// arcaneRun is what the transaction of UseArcaneRecovery leaves for after the commit.
type arcaneRun struct {
	ev       arcaneRecoveryEvent
	after    *playv1.CharacterVitals
	repeated bool
}

// readArcaneCall reads and checks the request, in the order the refusals have always come.
func readArcaneCall(ctx context.Context, req *playv1.UseArcaneRecoveryRequest) (arcaneCall, error) {
	m, err := authz.RequireCampaignMember(ctx, req.GetCampaignId())
	if err != nil {
		return arcaneCall{}, err
	}
	characterID, err := uuid.Parse(req.GetCharacterId())
	if err != nil {
		return arcaneCall{}, connect.NewError(connect.CodeNotFound, errors.New("character not found"))
	}
	key, err := uuid.Parse(req.GetIdempotencyKey())
	if err != nil {
		return arcaneCall{}, connect.NewError(connect.CodeInvalidArgument, errors.New("idempotency_key must be a UUID"))
	}
	slots, err := arcaneSlots(req)
	if err != nil {
		return arcaneCall{}, err
	}
	return arcaneCall{m: m, charText: characterID.String(), keyText: key.String(), hash: idem.Hash(req), slots: slots}, nil
}

// UseArcaneRecovery implements playv1connect.ResourceServiceHandler: the wizard's
// recovery of expended spell slots after a short rest.
func (s *Service) UseArcaneRecovery(
	ctx context.Context,
	req *connect.Request[playv1.UseArcaneRecoveryRequest],
) (*connect.Response[playv1.UseArcaneRecoveryResponse], error) {
	call, err := readArcaneCall(ctx, req.Msg)
	if err != nil {
		return nil, err
	}
	var run arcaneRun
	err = db.InTx(ctx, s.pool, func(tx pgx.Tx) error {
		run = arcaneRun{} // a retry starts over
		return s.arcaneRecoveryTx(ctx, tx, call, &run)
	})
	if err != nil {
		return nil, s.dbError(ctx, "use arcane recovery", err)
	}
	if run.repeated {
		current, err := s.vitals.GetVitals(ctx, call.m.CampaignID, call.charText)
		if err != nil {
			return nil, s.dbError(ctx, "get vitals", err)
		}
		return connect.NewResponse(&playv1.UseArcaneRecoveryResponse{Vitals: current, RecoveredLevels: run.ev.Levels}), nil
	}
	s.publishVitals(call.m.CampaignID, run.after)
	logging.Event(ctx, s.logger, "arcane_recovery.used", slog.String("character_id", call.charText), slog.Int("levels", int(run.ev.Levels)))
	return connect.NewResponse(&playv1.UseArcaneRecoveryResponse{Vitals: run.after, RecoveredLevels: run.ev.Levels}), nil
}

// arcaneRecoveryTx is the transaction of UseArcaneRecovery: lock, retry, who may, the
// keeper's recovery, the short rest it needs, the event.
func (s *Service) arcaneRecoveryTx(ctx context.Context, tx pgx.Tx, call arcaneCall, run *arcaneRun) error {
	q := s.queries.WithTx(tx)
	session, err := lockOpenSession(ctx, q, call.m.CampaignID)
	if err != nil {
		return err
	}
	done, again, err := eventByKey(ctx, q, session.ID, call.keyText, eventArcaneRecovery, call.hash)
	if err != nil {
		return err
	}
	if again {
		run.repeated = true
		return replayCharacterEvent(done, call.charText, "arcane recovery", &run.ev)
	}
	if err := s.requireCharacterOwnerOrMaster(ctx, tx, call.m, call.charText); err != nil {
		return err
	}
	if err := activeCombatBlocks(ctx, q, session.ID); err != nil {
		return err
	}
	_, afterVitals, levels, err := s.vitals.UseArcaneRecovery(ctx, tx, call.m.CampaignID, call.charText, call.slots)
	if err != nil {
		return resourceError(err)
	}
	if err := mustFollowShortRest(ctx, q, session.ID); err != nil {
		return err
	}
	run.after = afterVitals
	run.ev = arcaneEventOf(call.slots, levels)
	return s.appendKeyedEvent(ctx, q, keyedEvent{
		sessionID: session.ID, kind: eventArcaneRecovery, userID: call.m.UserID, characterID: &call.charText, key: call.keyText, hash: call.hash,
	}, run.ev)
}

// mustFollowShortRest says "when you finish a short rest": the latest rest of the session is a
// short one. Asked after the keeper so a character that cannot use the feature is told so
// first; the refusal rolls the keeper's write back with the transaction.
func mustFollowShortRest(ctx context.Context, q *playdb.Queries, sessionID string) error {
	kind, err := q.GetLatestRestKindInSession(ctx, sessionID)
	if err != nil && !errors.Is(err, pgx.ErrNoRows) {
		return fmt.Errorf("read the latest rest: %w", err)
	}
	if kind != rules.RestShort {
		return resourceBlocked(playv1.ResourceBlockedReason_RESOURCE_BLOCKED_REASON_NO_SHORT_REST, "Arcane Recovery needs a short rest first")
	}
	return nil
}

// arcaneEventOf is the event of a recovery: the slots by level, lowest first, and the levels.
func arcaneEventOf(slots map[int]int, levels int) arcaneRecoveryEvent {
	ev := arcaneRecoveryEvent{Levels: clamp32(levels, 1, 1<<10)}
	for level, count := range slots {
		ev.Slots = append(ev.Slots, arcaneSlot{Level: clamp32(level, 1, maxSlotLevel), Count: clamp32(count, 1, maxSlotsPerLevel)})
	}
	slices.SortFunc(ev.Slots, func(a, b arcaneSlot) int { return int(a.Level - b.Level) })
	return ev
}
