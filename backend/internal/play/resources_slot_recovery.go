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
	"google.golang.org/protobuf/proto"

	playv1 "github.com/PuraFome/meuRPG/backend/gen/meurpg/play/v1"
	"github.com/PuraFome/meuRPG/backend/internal/authz"
	"github.com/PuraFome/meuRPG/backend/internal/platform/db"
	"github.com/PuraFome/meuRPG/backend/internal/platform/idem"
	"github.com/PuraFome/meuRPG/backend/internal/platform/logging"
	"github.com/PuraFome/meuRPG/backend/internal/play/playdb"
	"github.com/PuraFome/meuRPG/backend/internal/rules"
)

// slotRecoveryEvent is the payload of an arcane_recovery_used or natural_recovery_used event:
// the slots that came back (level and count) and their combined level. Numbers only.
type slotRecoveryEvent struct {
	Slots  []slotRecoverySlot `json:"slots"`
	Levels int32              `json:"levels"`
	// Feature is the table feature whose slot_recovery effect the recovery was; empty for the SRD's own.
	Feature string `json:"feature,omitempty"`
}

type slotRecoverySlot struct {
	Level int32 `json:"level"`
	Count int32 `json:"count"`
}

// slotRecoveryRequest is what UseArcaneRecoveryRequest and UseNaturalRecoveryRequest share.
type slotRecoveryRequest interface {
	proto.Message
	GetCampaignId() string
	GetCharacterId() string
	GetIdempotencyKey() string
	GetSlots() []*playv1.SpellSlotBack
}

// recoverySlots reads the slots of the request: one entry per level, a count of at least 1.
func recoverySlots(req slotRecoveryRequest) (map[int]int, error) {
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

// slotRecovery is one feature that recovers slots: what its rule is, the event it writes and
// its name in logs and refusals.
type slotRecovery struct {
	rules rules.SlotRecovery
	// feature is the table feature whose slot_recovery effect this is, "" for the SRD's own (rules).
	feature string
	event   string
	name    string
	log     string // the structured log event
}

var (
	arcaneRecovery  = slotRecovery{rules: rules.ArcaneRecovery, event: eventArcaneRecovery, name: "Arcane Recovery", log: "arcane_recovery.used"}
	naturalRecovery = slotRecovery{rules: rules.NaturalRecovery, event: eventNaturalRecovery, name: "Natural Recovery", log: "natural_recovery.used"}
)

// recoveryCall is what a recovery reads from its request before the transaction.
type recoveryCall struct {
	m        authz.Membership
	charText string
	keyText  string
	hash     *string
	slots    map[int]int
}

// maxFeatureKeyLen bounds the feature key a request names (a table feature's key is a slug and a hash).
const maxFeatureKeyLen = 200

// recoveryRun is what the transaction of a recovery leaves for after the commit.
type recoveryRun struct {
	ev       slotRecoveryEvent
	after    *playv1.CharacterVitals
	repeated bool
}

// readRecoveryCall reads and checks the request. The refusals come in the order of the checks below.
func readRecoveryCall(ctx context.Context, req slotRecoveryRequest) (recoveryCall, error) {
	m, err := authz.RequireCampaignMember(ctx, req.GetCampaignId())
	if err != nil {
		return recoveryCall{}, err
	}
	characterID, err := uuid.Parse(req.GetCharacterId())
	if err != nil {
		return recoveryCall{}, connect.NewError(connect.CodeNotFound, errors.New("character not found"))
	}
	key, err := uuid.Parse(req.GetIdempotencyKey())
	if err != nil {
		return recoveryCall{}, connect.NewError(connect.CodeInvalidArgument, errors.New("idempotency_key must be a UUID"))
	}
	slots, err := recoverySlots(req)
	if err != nil {
		return recoveryCall{}, err
	}
	return recoveryCall{m: m, charText: characterID.String(), keyText: key.String(), hash: idem.Hash(req), slots: slots}, nil
}

// UseArcaneRecovery implements playv1connect.ResourceServiceHandler: the wizard's
// recovery of expended spell slots after a short rest.
func (s *Service) UseArcaneRecovery(
	ctx context.Context,
	req *connect.Request[playv1.UseArcaneRecoveryRequest],
) (*connect.Response[playv1.UseArcaneRecoveryResponse], error) {
	rec := arcaneRecovery
	if k := req.Msg.GetFeatureKey(); k != "" { // the table feature's recovery, with the table's limits
		if len(k) > maxFeatureKeyLen {
			return nil, connect.NewError(connect.CodeInvalidArgument, errors.New("feature_key is too long"))
		}
		rec.feature = k
	}
	vitals, levels, err := s.useRecovery(ctx, rec, req.Msg)
	if err != nil {
		return nil, err
	}
	return connect.NewResponse(&playv1.UseArcaneRecoveryResponse{Vitals: vitals, RecoveredLevels: levels}), nil
}

// UseNaturalRecovery implements playv1connect.ResourceServiceHandler: the Circle of the Land
// druid's recovery of expended spell slots after a short rest.
func (s *Service) UseNaturalRecovery(
	ctx context.Context,
	req *connect.Request[playv1.UseNaturalRecoveryRequest],
) (*connect.Response[playv1.UseNaturalRecoveryResponse], error) {
	vitals, levels, err := s.useRecovery(ctx, naturalRecovery, req.Msg)
	if err != nil {
		return nil, err
	}
	return connect.NewResponse(&playv1.UseNaturalRecoveryResponse{Vitals: vitals, RecoveredLevels: levels}), nil
}

// useRecovery is the call of a slot recovery: the vitals after and the combined level recovered.
func (s *Service) useRecovery(ctx context.Context, rec slotRecovery, req slotRecoveryRequest) (*playv1.CharacterVitals, int32, error) {
	call, err := readRecoveryCall(ctx, req)
	if err != nil {
		return nil, 0, err
	}
	var run recoveryRun
	err = db.InTx(ctx, s.pool, func(tx pgx.Tx) error {
		run = recoveryRun{} // a retry starts over
		return s.recoveryTx(ctx, tx, rec, call, &run)
	})
	if err != nil {
		return nil, 0, s.dbError(ctx, "use "+rec.name, err)
	}
	if run.repeated {
		current, err := s.vitals.GetVitals(ctx, call.m.CampaignID, call.charText)
		if err != nil {
			return nil, 0, s.dbError(ctx, "get vitals", err)
		}
		return current, run.ev.Levels, nil
	}
	s.publishVitals(call.m.CampaignID, run.after)
	logging.Event(ctx, s.logger, rec.log, slog.String("character_id", call.charText), slog.Int("levels", int(run.ev.Levels)))
	return run.after, run.ev.Levels, nil
}

// recoveryTx is the transaction of a slot recovery: lock, retry, who may, the
// keeper's recovery, the short rest it needs, the event.
func (s *Service) recoveryTx(ctx context.Context, tx pgx.Tx, rec slotRecovery, call recoveryCall, run *recoveryRun) error {
	q := s.queries.WithTx(tx)
	session, err := lockOpenSessionOrNone(ctx, q, call.m.CampaignID)
	if err != nil {
		return err
	}
	done, again, err := eventByKey(ctx, q, session.ID, call.keyText, rec.event, call.hash)
	if err != nil {
		return err
	}
	if again {
		run.repeated = true
		return replayCharacterEvent(done, call.charText, rec.name, &run.ev)
	}
	if err := s.requireCharacterOwnerOrMaster(ctx, tx, call.m, call.charText); err != nil {
		return err
	}
	if err := activeCombatBlocks(ctx, q, session.ID); err != nil {
		return err
	}
	var afterVitals *playv1.CharacterVitals
	var levels int
	if rec.feature != "" {
		_, afterVitals, levels, err = s.vitals.UseTableSlotRecovery(ctx, tx, call.m.CampaignID, call.charText, rec.feature, call.slots)
	} else {
		_, afterVitals, levels, err = s.vitals.UseSlotRecovery(ctx, tx, call.m.CampaignID, call.charText, rec.rules, call.slots)
	}
	if err != nil {
		return resourceError(err)
	}
	if err := mustFollowShortRest(ctx, q, session.ID, rec.name); err != nil {
		return err
	}
	run.after = afterVitals
	run.ev = recoveryEventOf(call.slots, levels)
	run.ev.Feature = rec.feature
	return s.appendKeyedEvent(ctx, q, keyedEvent{
		sessionID: session.ID, kind: rec.event, userID: call.m.UserID, characterID: &call.charText, key: call.keyText, hash: call.hash,
	}, run.ev)
}

// mustFollowShortRest says "when you finish a short rest": the latest rest of the session is a
// short one. Asked after the keeper so a character that cannot use the feature is told so
// first; the refusal rolls the keeper's write back with the transaction.
func mustFollowShortRest(ctx context.Context, q *playdb.Queries, sessionID, feature string) error {
	kind, err := q.GetLatestRestKindInSession(ctx, sessionID)
	if err != nil && !errors.Is(err, pgx.ErrNoRows) {
		return fmt.Errorf("read the latest rest: %w", err)
	}
	if kind != rules.RestShort {
		return resourceBlocked(playv1.ResourceBlockedReason_RESOURCE_BLOCKED_REASON_NO_SHORT_REST, feature+" needs a short rest first")
	}
	return nil
}

// recoveryEventOf is the event of a recovery: the slots by level, lowest first, and the levels.
func recoveryEventOf(slots map[int]int, levels int) slotRecoveryEvent {
	ev := slotRecoveryEvent{Levels: clamp32(levels, 1, 1<<10)}
	for level, count := range slots {
		ev.Slots = append(ev.Slots, slotRecoverySlot{Level: clamp32(level, 1, maxSlotLevel), Count: clamp32(count, 1, maxSlotsPerLevel)})
	}
	slices.SortFunc(ev.Slots, func(a, b slotRecoverySlot) int { return int(a.Level - b.Level) })
	return ev
}
