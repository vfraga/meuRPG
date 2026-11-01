package play

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"slices"
	"time"
	"uuid"

	"connectrpc.com/connect"
	"github.com/jackc/pgx/v5"

	playv1 "github.com/PuraFome/meuRPG/backend/gen/meurpg/play/v1"
	"github.com/PuraFome/meuRPG/backend/internal/authz"
	"github.com/PuraFome/meuRPG/backend/internal/platform/db"
	"github.com/PuraFome/meuRPG/backend/internal/platform/dice"
	"github.com/PuraFome/meuRPG/backend/internal/platform/idem"
	"github.com/PuraFome/meuRPG/backend/internal/platform/logging"
	"github.com/PuraFome/meuRPG/backend/internal/play/link"
	"github.com/PuraFome/meuRPG/backend/internal/play/playdb"
	"github.com/PuraFome/meuRPG/backend/internal/rules"
)

// Rests and the hit dice (SRD 5.1, "Resting"): the master takes a short or a long
// rest for the party, and a player spends hit dice on a short rest. The numbers
// are the characters module's (RestKeeper); this file decides who may ask and
// when, writes the history, and tells the streams.

// The kinds of session_events rows of the rests (session_event_kinds).
const (
	eventRestTaken    = "rest_taken"
	eventHitDiceSpent = "hit_dice_spent"
	// eventArcaneRecovery is the wizard's Arcane Recovery; eventNaturalRecovery the Land
	// druid's Natural Recovery (session_event_kinds).
	eventArcaneRecovery  = "arcane_recovery_used"
	eventNaturalRecovery = "natural_recovery_used"
)

// RestKeeper is what the characters module does for the rests and the resources
// that move points between a character's vitals. Like VitalsKeeper's methods, they
// take no caller and run inside the transaction they are given; the refusals
// that are rules are link's errors, which resourceError turns into the typed answer.
type RestKeeper interface {
	// PreviewRest says what a rest would give back to each living, active player
	// character of the campaign, writing nothing.
	PreviewRest(ctx context.Context, tx pgx.Tx, campaignID string, kind playv1.RestKind) ([]*playv1.RestPreview, error)
	// TakeRest gives the party what the rest gives back, and returns the vitals before
	// and after of the characters it changed.
	TakeRest(ctx context.Context, tx pgx.Tx, campaignID string, req *playv1.TakeRestRequest) (before, after []*playv1.CharacterVitals, err error)
	// SpendHitDie spends one hit die of the size and heals the face plus the
	// Constitution modifier (at least 0, up to the maximum).
	SpendHitDie(ctx context.Context, tx pgx.Tx, campaignID, characterID string, faces, face, extra int) (before, after *playv1.CharacterVitals, conMod, healed int, err error)
	// SongOfRest is the die of the Song of Rest the party hears (0: nobody sings).
	SongOfRest(ctx context.Context, tx pgx.Tx, campaignID string) (int, error)
	// UseSlotRecovery spends the use of the recovery (Arcane or Natural Recovery) and gives
	// back the expended slots (a map of spell level to count), and returns the vitals before
	// and after and the combined level recovered.
	UseSlotRecovery(ctx context.Context, tx pgx.Tx, campaignID, characterID string, rec rules.SlotRecovery, slots map[int]int) (before, after *playv1.CharacterVitals, levels int, err error)
	// UseTableSlotRecovery is the same for a table feature's slot_recovery effect, by the feature's key.
	UseTableSlotRecovery(ctx context.Context, tx pgx.Tx, campaignID, characterID, feature string, slots map[int]int) (before, after *playv1.CharacterVitals, levels int, err error)
	// CreateSpellSlot is Flexible Casting's creation of a slot; ConvertSpellSlot its
	// conversion into sorcery points.
	CreateSpellSlot(ctx context.Context, tx pgx.Tx, campaignID, characterID string, level int) (before, after *playv1.CharacterVitals, cost int, err error)
	ConvertSpellSlot(ctx context.Context, tx pgx.Tx, campaignID, characterID string, level int) (before, after *playv1.CharacterVitals, gain int, err error)
	// UndoCreateSpellSlot and UndoConvertSpellSlot take them back (the master's undo).
	UndoCreateSpellSlot(ctx context.Context, tx pgx.Tx, campaignID, characterID string, level, cost int) (*playv1.CharacterVitals, error)
	UndoConvertSpellSlot(ctx context.Context, tx pgx.Tx, campaignID, characterID string, level, gain int) (*playv1.CharacterVitals, error)
	// GiveSpellSlot adds a free slot of the level until the next long rest (a table action
	// that turns uses into a slot), SpendSpellSlot expends a free one (a table action that
	// heals by it); the Undo ones take them back.
	GiveSpellSlot(ctx context.Context, tx pgx.Tx, campaignID, characterID string, level int) (*playv1.CharacterVitals, error)
	UndoGiveSpellSlot(ctx context.Context, tx pgx.Tx, campaignID, characterID string, level int) (*playv1.CharacterVitals, error)
	SpendSpellSlot(ctx context.Context, tx pgx.Tx, campaignID, characterID string, level int) (*playv1.CharacterVitals, error)
	UndoSpendSpellSlot(ctx context.Context, tx pgx.Tx, campaignID, characterID string, level int) (*playv1.CharacterVitals, error)
}

// resourceBlocked is the failed_precondition of a rule's refusal, with the
// ResourceBlocked detail that tells the app why.
func resourceBlocked(reason playv1.ResourceBlockedReason, msg string, edit ...func(*playv1.ResourceBlocked)) error {
	err := connect.NewError(connect.CodeFailedPrecondition, errors.New(msg))
	blocked := &playv1.ResourceBlocked{Reason: reason}
	for _, e := range edit {
		e(blocked)
	}
	if detail, detailErr := connect.NewErrorDetail(blocked); detailErr == nil {
		err.AddDetail(detail)
	}
	return err
}

// resourceError turns a refusal of the characters module into the typed answer;
// any other error passes as it is.
func resourceError(err error) error {
	if err == nil {
		return nil
	}
	points := func(b *playv1.ResourceBlocked) {
		if pe, ok := errors.AsType[*link.PointsError](err); ok {
			b.Needed, b.Available = clamp32(pe.Needed, 0, 1<<20), clamp32(pe.Available, 0, 1<<20)
		}
	}
	switch {
	case errors.Is(err, link.ErrNoHitDiceLeft):
		return resourceBlocked(playv1.ResourceBlockedReason_RESOURCE_BLOCKED_REASON_NO_HIT_DICE_LEFT, "the character has no hit die of that size left")
	case errors.Is(err, link.ErrNotEnoughPoints):
		return resourceBlocked(playv1.ResourceBlockedReason_RESOURCE_BLOCKED_REASON_NOT_ENOUGH_POINTS, "there are not enough points", points)
	case errors.Is(err, link.ErrSlotLevelTooHigh):
		return resourceBlocked(playv1.ResourceBlockedReason_RESOURCE_BLOCKED_REASON_SLOT_LEVEL_TOO_HIGH, "the slot level is too high for the feature")
	case errors.Is(err, link.ErrNoFreeSlot):
		return resourceBlocked(playv1.ResourceBlockedReason_RESOURCE_BLOCKED_REASON_NO_FREE_SLOT, "there is no free spell slot of that level")
	case errors.Is(err, link.ErrPointsFull):
		return resourceBlocked(playv1.ResourceBlockedReason_RESOURCE_BLOCKED_REASON_SORCERY_POINTS_FULL, "the sorcerer has the most sorcery points already", points)
	case errors.Is(err, link.ErrPointsOver):
		return resourceBlocked(playv1.ResourceBlockedReason_RESOURCE_BLOCKED_REASON_SORCERY_POINTS_OVER, "the slot would take the sorcery points past the maximum", points)
	case errors.Is(err, link.ErrNoUsesLeft):
		return resourceBlocked(playv1.ResourceBlockedReason_RESOURCE_BLOCKED_REASON_NO_USES_LEFT, "the feature was used since the last long rest")
	case errors.Is(err, link.ErrSlotNotExpended):
		return resourceBlocked(playv1.ResourceBlockedReason_RESOURCE_BLOCKED_REASON_SLOT_NOT_EXPENDED, "there are not that many expended spell slots of that level")
	case errors.Is(err, link.ErrSlotRecoveryOver):
		return resourceBlocked(playv1.ResourceBlockedReason_RESOURCE_BLOCKED_REASON_OVER_ALLOWANCE, "the slots add up to more than the feature allows", points)
	case errors.Is(err, link.ErrNoResource):
		return resourceBlocked(playv1.ResourceBlockedReason_RESOURCE_BLOCKED_REASON_NOT_AVAILABLE, "the character does not have this feature")
	case errors.Is(err, link.ErrBadHitDiceChoice):
		return connect.NewError(connect.CodeInvalidArgument, errors.New("the hit dice are not the character's"))
	}
	return err
}

// restEvent is the payload of a rest_taken event: the kind and the characters the
// rest changed. IDs only (docs/privacy.md).
type restEvent struct {
	Kind       string   `json:"kind"`
	Characters []string `json:"character_ids,omitempty"`
}

// hitDieEvent is the payload of a hit_dice_spent event: the die, the face
// rolled, the Constitution modifier and what the character regained.
type hitDieEvent struct {
	Faces    int32 `json:"faces"`
	Face     int32 `json:"face"`
	Modifier int32 `json:"modifier,omitempty"`
	Healed   int32 `json:"healed"`
	Physical bool  `json:"physical,omitempty"`
	// SongSides and SongFace are the extra die of a bard's Song of Rest and the face
	// rolled; both are absent when no song applied. Healed counts it.
	SongSides int32 `json:"song_sides,omitempty"`
	SongFace  int32 `json:"song_face,omitempty"`
}

// activeCombatBlocks refuses with COMBAT_OPEN while a combat is running: a rest in
// the middle of a fight has no place in the turn order.
func activeCombatBlocks(ctx context.Context, q *playdb.Queries, sessionID string) error {
	enc, err := q.GetOpenEncounter(ctx, sessionID)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil
	}
	if err != nil {
		return fmt.Errorf("find the open encounter: %w", err)
	}
	if enc.Status == statusActive {
		return resourceBlocked(playv1.ResourceBlockedReason_RESOURCE_BLOCKED_REASON_COMBAT_OPEN, "a combat is running: finish it before resting")
	}
	return nil
}

// restKindName names the kind of rest TakeRest and GetRestPreview take.
func restKindName(kind playv1.RestKind) (string, error) {
	switch kind {
	case playv1.RestKind_REST_KIND_SHORT:
		return "short", nil
	case playv1.RestKind_REST_KIND_LONG:
		return "long", nil
	}
	return "", connect.NewError(connect.CodeInvalidArgument, errors.New("kind must be a short or a long rest"))
}

// GetRestPreview implements playv1connect.ResourceServiceHandler.
func (s *Service) GetRestPreview(
	ctx context.Context,
	req *connect.Request[playv1.GetRestPreviewRequest],
) (*connect.Response[playv1.GetRestPreviewResponse], error) {
	m, err := authz.RequireCampaignRole(ctx, req.Msg.GetCampaignId(), authz.RoleMaster)
	if err != nil {
		return nil, err
	}
	if _, err := restKindName(req.Msg.GetKind()); err != nil {
		return nil, err
	}
	res := &playv1.GetRestPreviewResponse{}
	err = db.ReadTx(ctx, s.pool, func(tx pgx.Tx) error {
		*res = playv1.GetRestPreviewResponse{}
		q := s.queries.WithTx(tx)
		session, err := openSessionWith(ctx, q, m.CampaignID)
		if err != nil {
			return err
		}
		if res.Characters, err = s.vitals.PreviewRest(ctx, tx, m.CampaignID, req.Msg.GetKind()); err != nil {
			return err
		}
		if req.Msg.GetKind() == playv1.RestKind_REST_KIND_LONG {
			if res.LongRestAlreadyTaken, err = q.HasLongRestInSession(ctx, session.ID); err != nil {
				return fmt.Errorf("find the long rest of the session: %w", err)
			}
		}
		return nil
	})
	if err != nil {
		return nil, s.dbError(ctx, "preview a rest", err)
	}
	return connect.NewResponse(res), nil
}

// eventByKey finds the event a change with this key wrote and checks that the key
// is not being reused for another change: the same kind, the same request. It says
// repeated for a retry. An event of another kind or request is `invalid_argument`.
func eventByKey(ctx context.Context, q *playdb.Queries, sessionID, key, kind string, hash *string) (done playdb.GetSessionEventByIdempotencyKeyRow, repeated bool, err error) {
	done, err = q.GetSessionEventByIdempotencyKey(ctx, playdb.GetSessionEventByIdempotencyKeyParams{GameSessionID: sessionID, IdempotencyKey: &key})
	switch {
	case err == nil:
		if done.Kind != kind || hashDiffers(done.IdempotencyHash, hash) {
			return done, false, errKeyReused()
		}
		return done, true, nil
	case errors.Is(err, pgx.ErrNoRows):
		return done, false, nil
	}
	return done, false, fmt.Errorf("find the event of this idempotency key: %w", err)
}

// publishVitalsOf tells the streams the new vitals of the characters.
func (s *Service) publishVitalsOf(campaignID string, vitals []*playv1.CharacterVitals) {
	for _, v := range vitals {
		s.publishVitals(campaignID, v)
	}
}

// restCall is what TakeRest reads from its request before the transaction.
type restCall struct {
	m       authz.Membership
	kind    string
	keyText string
	hash    *string
	msg     *playv1.TakeRestRequest
}

// restRun is what the transaction of TakeRest leaves for after the commit.
type restRun struct {
	after        []*playv1.CharacterVitals
	repeated     []string
	touched      []playdb.Encounter
	castsChanged int // how much of the casts outside a combat the rest changed (casting_combat.go)
	spellVitals  []*playv1.CharacterVitals
}

// readRestCall reads and checks the request. The refusals come in the order of the checks below.
func readRestCall(ctx context.Context, msg *playv1.TakeRestRequest) (restCall, error) {
	m, err := authz.RequireCampaignRole(ctx, msg.GetCampaignId(), authz.RoleMaster)
	if err != nil {
		return restCall{}, err
	}
	kind, err := restKindName(msg.GetKind())
	if err != nil {
		return restCall{}, err
	}
	key, err := uuid.Parse(msg.GetIdempotencyKey())
	if err != nil {
		return restCall{}, connect.NewError(connect.CodeInvalidArgument, errors.New("idempotency_key must be a UUID"))
	}
	return restCall{m: m, kind: kind, keyText: key.String(), hash: idem.Hash(msg), msg: msg}, nil
}

// TakeRest implements playv1connect.ResourceServiceHandler: the master's short or
// long rest.
func (s *Service) TakeRest(
	ctx context.Context,
	req *connect.Request[playv1.TakeRestRequest],
) (*connect.Response[playv1.TakeRestResponse], error) {
	call, err := readRestCall(ctx, req.Msg)
	if err != nil {
		return nil, err
	}
	var run restRun
	err = db.InTx(ctx, s.pool, func(tx pgx.Tx) error {
		run = restRun{castsChanged: castsUnchanged} // a retry starts over
		return s.takeRestTx(ctx, tx, call, &run)
	})
	if err != nil {
		return nil, s.dbError(ctx, "take a rest", err)
	}
	if run.repeated != nil {
		return connect.NewResponse(s.repeatedRestAnswer(ctx, call.m.CampaignID, run.repeated)), nil
	}
	s.publishRest(ctx, call.m.CampaignID, run)
	logging.Event(ctx, s.logger, "rest.taken", slog.String("kind", call.kind), slog.Int("characters", len(run.after)))
	return connect.NewResponse(&playv1.TakeRestResponse{Vitals: run.after}), nil
}

// takeRestTx is the transaction of TakeRest: lock, retry, the keeper's rest, the spells
// and the clock, the combats that show the vitals, the event.
func (s *Service) takeRestTx(ctx context.Context, tx pgx.Tx, call restCall, run *restRun) error {
	q := s.queries.WithTx(tx)
	session, err := lockOpenSessionOrNone(ctx, q, call.m.CampaignID)
	if err != nil {
		return err
	}
	done, again, err := eventByKey(ctx, q, session.ID, call.keyText, eventRestTaken, call.hash)
	if err != nil {
		return err
	}
	if again { // a retry of a rest already taken
		var ev restEvent
		if err := json.Unmarshal(done.Payload, &ev); err != nil {
			return fmt.Errorf("read the rest event: %w", err)
		}
		run.repeated = append([]string{}, ev.Characters...)
		return nil
	}
	if err := activeCombatBlocks(ctx, q, session.ID); err != nil {
		return err
	}
	if _, run.after, err = s.vitals.TakeRest(ctx, tx, call.m.CampaignID, call.msg); err != nil {
		return resourceError(err)
	}
	if err := restNPCUses(ctx, q, call); err != nil {
		return err
	}
	if err := s.restEndsSpells(ctx, tx, q, session, call, run); err != nil {
		return err
	}
	ids, err := s.restTouchCombats(ctx, q, session.ID, run)
	if err != nil {
		return err
	}
	return s.appendKeyedEvent(ctx, q, keyedEvent{
		sessionID: session.ID, kind: eventRestTaken, userID: call.m.UserID, key: call.keyText, hash: call.hash,
	}, restEvent{Kind: call.kind, Characters: ids})
}

// restNPCUses gives the hand-made NPCs what the rest returns: a long rest their spell slots, uses a day and
// Legendary Resistance, and a short or long rest the actions that recharge after a rest. A combat in setup
// holds a copy of it, cleared first.
func restNPCUses(ctx context.Context, q *playdb.Queries, call restCall) error {
	if call.kind == rules.RestLong {
		if err := q.ClearSetupCombatantsSpellUse(ctx, &call.m.CampaignID); err != nil {
			return fmt.Errorf("clear the NPCs' spell use in a combat in setup: %w", err)
		}
		if err := q.ResetNPCSpellUseOfCampaign(ctx, &call.m.CampaignID); err != nil {
			return fmt.Errorf("give the NPCs' spell use back: %w", err)
		}
		return nil
	}
	if err := q.ClearSetupCombatantsRestUses(ctx, &call.m.CampaignID); err != nil {
		return fmt.Errorf("clear the NPCs' rest uses in a combat in setup: %w", err)
	}
	if err := q.ResetNPCRestUsesOfCampaign(ctx, &call.m.CampaignID); err != nil {
		return fmt.Errorf("give the NPCs' rest uses back: %w", err)
	}
	if err := q.DeleteEmptyNPCSpellUseOfCampaign(ctx, &call.m.CampaignID); err != nil {
		return fmt.Errorf("drop the NPCs' empty rows: %w", err)
	}
	return nil
}

// restEndsSpells ends the spells that last end with the rest when their whole duration fits it
// (SRD 5.1, "Resting"): 8 hours or less at a long rest, an hour or less at a short one. The
// same transaction, so a rest never leaves a spell running that it ended on the vitals. The
// rest is game time passing too: what had a clock loses the hour or the eight hours.
func (s *Service) restEndsSpells(ctx context.Context, tx pgx.Tx, q *playdb.Queries, session playdb.GameSession, call restCall, run *restRun) error {
	minutes := rules.ShortRestMinutes
	if call.kind == rules.RestLong {
		minutes = rules.LongRestMinutes
	}
	ct, err := s.openTx(ctx, combatTx{tx: tx, q: q, session: session, now: s.now(), kind: eventRestTaken, actorUserID: call.m.UserID, svc: s, master: true})
	if err != nil {
		return err
	}
	if _, run.spellVitals, err = s.endSpellsAtRest(ctx, ct, minutes); err != nil {
		return err
	}
	if call.kind == rules.RestLong {
		if err := s.endLongRestEffects(ctx, ct); err != nil {
			return err
		}
	}
	if err := s.advanceGameTime(ctx, ct, clamp32(minutes*secondsPerMinute, 0, 1<<30)); err != nil {
		return err
	}
	run.spellVitals = append(run.spellVitals, ct.told...)
	run.castsChanged = ct.castsChanged
	return nil
}

// restTouchCombats touches the combat that shows each changed character's state from the
// vitals (one in setup), once for each combat, and returns the characters' IDs.
func (s *Service) restTouchCombats(ctx context.Context, q *playdb.Queries, sessionID string, run *restRun) ([]string, error) {
	ids := make([]string, 0, len(run.after))
	for _, v := range run.after {
		ids = append(ids, v.GetCharacterId())
		enc, err := s.touchCombatOf(ctx, q, sessionID, v.GetCharacterId(), nil)
		if err != nil {
			return nil, err
		}
		if enc != nil && !slices.ContainsFunc(run.touched, func(e playdb.Encounter) bool { return e.ID == enc.ID }) {
			run.touched = append(run.touched, *enc)
		}
	}
	return ids, nil
}

// repeatedRestAnswer answers a retry: nothing changed and nothing goes out, so it is the
// vitals as they are now.
func (s *Service) repeatedRestAnswer(ctx context.Context, campaignID string, characterIDs []string) *playv1.TakeRestResponse {
	res := &playv1.TakeRestResponse{}
	for _, id := range characterIDs {
		if v, err := s.vitals.GetVitals(ctx, campaignID, id); err == nil {
			res.Vitals = append(res.Vitals, v)
		}
	}
	return res
}

// publishRest tells the streams what the committed rest changed.
func (s *Service) publishRest(ctx context.Context, campaignID string, run restRun) {
	s.publishVitalsOf(campaignID, run.after)
	s.publishVitalsOf(campaignID, run.spellVitals)
	if run.castsChanged != castsUnchanged {
		s.publishCastsChanged(campaignID, run.castsChanged == castsMasterOnly)
	}
	pctx, stop := afterCommit(ctx)
	defer stop()
	for _, enc := range run.touched {
		s.publishEncounterChanged(pctx, campaignID, enc)
	}
}

// hitDieRequest reads the size of the die to spend and how its face is given: rolled in the
// app or typed from a real die.
func hitDieRequest(req *playv1.SpendHitDiceRequest) (faces int, in rollInput, err error) {
	faces = int(req.GetFaces())
	if faces < 1 || faces > maxHitDie {
		return 0, in, connect.NewError(connect.CodeInvalidArgument, errors.New("faces must be the size of one of the character's hit dice"))
	}
	switch roll := req.GetRoll().(type) {
	case *playv1.SpendHitDiceRequest_RollInApp:
		if !roll.RollInApp {
			return 0, in, connect.NewError(connect.CodeInvalidArgument, errors.New("roll_in_app must be true"))
		}
		in.inApp = true
	case *playv1.SpendHitDiceRequest_TypedFace:
		in.typed = int(roll.TypedFace)
		if in.typed < 1 || in.typed > faces {
			return 0, in, connect.NewError(connect.CodeInvalidArgument, errors.New("typed_face must be a face of the die"))
		}
	default:
		return 0, in, connect.NewError(connect.CodeInvalidArgument, errors.New("set roll_in_app or typed_face"))
	}
	return faces, in, nil
}

// hitDiceCall is what SpendHitDice reads from its request before the transaction.
type hitDiceCall struct {
	m         authz.Membership
	charText  string
	keyText   string
	hash      *string
	faces     int
	in        rollInput
	typedSong int
}

// hitDiceRun is what the transaction of SpendHitDice leaves for after the commit.
type hitDiceRun struct {
	ev       hitDieEvent
	after    *playv1.CharacterVitals
	repeated bool
}

// readHitDiceCall reads and checks the request. The refusals come in the order of the checks below.
func readHitDiceCall(ctx context.Context, req *playv1.SpendHitDiceRequest) (hitDiceCall, error) {
	m, err := authz.RequireCampaignMember(ctx, req.GetCampaignId())
	if err != nil {
		return hitDiceCall{}, err
	}
	characterID, err := uuid.Parse(req.GetCharacterId())
	if err != nil {
		return hitDiceCall{}, connect.NewError(connect.CodeNotFound, errors.New("character not found"))
	}
	key, err := uuid.Parse(req.GetIdempotencyKey())
	if err != nil {
		return hitDiceCall{}, connect.NewError(connect.CodeInvalidArgument, errors.New("idempotency_key must be a UUID"))
	}
	faces, in, err := hitDieRequest(req)
	if err != nil {
		return hitDiceCall{}, err
	}
	return hitDiceCall{
		m: m, charText: characterID.String(), keyText: key.String(), hash: idem.Hash(req),
		faces: faces, in: in, typedSong: int(req.GetTypedSongFace()),
	}, nil
}

// SpendHitDice implements playv1connect.ResourceServiceHandler: a player spends one
// hit die on a short rest.
func (s *Service) SpendHitDice(
	ctx context.Context,
	req *connect.Request[playv1.SpendHitDiceRequest],
) (*connect.Response[playv1.SpendHitDiceResponse], error) {
	call, err := readHitDiceCall(ctx, req.Msg)
	if err != nil {
		return nil, err
	}
	var run hitDiceRun
	err = db.InTx(ctx, s.pool, func(tx pgx.Tx) error {
		run = hitDiceRun{} // a retry starts over
		return s.spendHitDiceTx(ctx, tx, call, &run)
	})
	if err != nil {
		return nil, s.dbError(ctx, "spend a hit die", err)
	}
	if run.repeated {
		current, err := s.vitals.GetVitals(ctx, call.m.CampaignID, call.charText)
		if err != nil {
			return nil, s.dbError(ctx, "get vitals", err)
		}
		return connect.NewResponse(spendResponse(current, run.ev)), nil
	}
	s.publishVitals(call.m.CampaignID, run.after)
	logging.Event(ctx, s.logger, "hit_dice.spent", slog.String("character_id", call.charText), slog.Int("faces", call.faces))
	return connect.NewResponse(spendResponse(run.after, run.ev)), nil
}

// spendHitDiceTx is the transaction of SpendHitDice: lock, retry, who may, roll, spend, write.
func (s *Service) spendHitDiceTx(ctx context.Context, tx pgx.Tx, call hitDiceCall, run *hitDiceRun) error {
	q := s.queries.WithTx(tx)
	session, err := lockOpenSessionOrNone(ctx, q, call.m.CampaignID)
	if err != nil {
		return err
	}
	done, again, err := eventByKey(ctx, q, session.ID, call.keyText, eventHitDiceSpent, call.hash)
	if err != nil {
		return err
	}
	if again {
		run.repeated = true
		return replayCharacterEvent(done, call.charText, "hit die", &run.ev)
	}
	if err := s.requireCharacterOwnerOrMaster(ctx, tx, call.m, call.charText); err != nil {
		return err
	}
	if err := activeCombatBlocks(ctx, q, session.ID); err != nil {
		return err
	}
	if err := s.mustRollThisWay(ctx, tx, call.m, call.in); err != nil {
		return err
	}
	if err := s.spendHitDieRolled(ctx, tx, q, session.ID, call, run); err != nil {
		return err
	}
	return s.appendKeyedEvent(ctx, q, keyedEvent{
		sessionID: session.ID, kind: eventHitDiceSpent, userID: call.m.UserID, characterID: &call.charText, key: call.keyText, hash: call.hash,
	}, run.ev)
}

// spendHitDieRolled rolls the die (or takes the typed face), asks for the Song of Rest's
// die, spends the hit die and fills the event of the run.
func (s *Service) spendHitDieRolled(ctx context.Context, tx pgx.Tx, q *playdb.Queries, sessionID string, call hitDiceCall, run *hitDiceRun) error {
	face := call.in.typed
	if call.in.inApp {
		rolled, err := dice.Roll(s.roller, dice.Expr{Count: 1, Sides: call.faces})
		if err != nil {
			return fmt.Errorf("roll the hit die: %w", err)
		}
		face = rolled.Faces[0]
	}
	songSides, songFace, err := s.songOfRestDie(ctx, tx, q, sessionID, call.m.CampaignID, call.charText, call.in, call.typedSong)
	if err != nil {
		return err
	}
	_, afterVitals, conMod, healed, err := s.vitals.SpendHitDie(ctx, tx, call.m.CampaignID, call.charText, call.faces, face, songFace)
	if err != nil {
		return resourceError(err)
	}
	run.after = afterVitals
	run.ev = hitDieEvent{
		Faces: clamp32(call.faces, 1, maxHitDie), Face: clamp32(face, 1, maxHitDie), Modifier: clamp32(conMod, -maxHitDie, maxHitDie),
		Healed: clamp32(healed, 0, 1<<20), Physical: !call.in.inApp,
		SongSides: clamp32(songSides, 0, maxHitDie), SongFace: clamp32(songFace, 0, maxHitDie),
	}
	return nil
}

// requireCharacterOwnerOrMaster refuses a member who is neither the master nor the
// character's player: another player's character is not theirs.
func (s *Service) requireCharacterOwnerOrMaster(ctx context.Context, tx pgx.Tx, m authz.Membership, characterID string) error {
	current, err := s.vitals.GetVitalsTx(ctx, tx, m.CampaignID, characterID)
	if err != nil {
		return err
	}
	if m.Role != authz.RoleMaster && (current.GetPlayerUserId() == "" || current.GetPlayerUserId() != m.UserID) {
		return connect.NewError(connect.CodePermissionDenied, errors.New("only the character's player or the master may do this"))
	}
	return nil
}

// replayCharacterEvent reads the event a retry finds, which must be about the same
// character; what names the event in the error of a payload that cannot be read.
func replayCharacterEvent(done playdb.GetSessionEventByIdempotencyKeyRow, characterID, what string, into any) error {
	if done.CharacterID == nil || *done.CharacterID != characterID {
		return errKeyReused()
	}
	if err := json.Unmarshal(done.Payload, into); err != nil {
		return fmt.Errorf("read the %s event: %w", what, err)
	}
	return nil
}

// keyedEvent says whose session event a rest handler writes, under which key.
type keyedEvent struct {
	sessionID, kind, userID, key string
	characterID                  *string
	hash                         *string
}

// appendKeyedEvent encodes the payload, numbers the event and inserts it.
func (s *Service) appendKeyedEvent(ctx context.Context, q *playdb.Queries, e keyedEvent, payload any) error {
	_, _, err := s.insertKeyedEvent(ctx, q, e, payload)
	return err
}

// insertKeyedEvent is appendKeyedEvent for a writer that answers with the event's id and time.
func (s *Service) insertKeyedEvent(ctx context.Context, q *playdb.Queries, e keyedEvent, payload any) (id string, at time.Time, err error) {
	body, err := json.Marshal(payload)
	if err != nil {
		return "", time.Time{}, fmt.Errorf("encode the event payload: %w", err)
	}
	seq, err := q.NextSessionEventSeq(ctx, e.sessionID)
	if err != nil {
		return "", time.Time{}, fmt.Errorf("next event number: %w", err)
	}
	at = s.now()
	row, err := insertSessionEvent(ctx, q, playdb.InsertSessionEventParams{
		GameSessionID: e.sessionID, Seq: seq, Kind: e.kind, ActorUserID: &e.userID, CharacterID: e.characterID,
		Payload: body, IdempotencyKey: &e.key, IdempotencyHash: e.hash, CreatedAt: at,
	})
	if err != nil {
		return "", time.Time{}, fmt.Errorf("insert session event: %w", err)
	}
	return row.ID, at, nil
}

// songOfRestDie says whether a bard's Song of Rest gives the character an extra die with
// the hit die it spends now (SRD 5.1, Bard 2): the latest rest of the session is a short
// rest, a bard of the party with at least 1 hit point has the feature (the party hears
// it, the bard included), and the character has not had the extra die since that rest.
// It returns the die's size and its face (rolled here for an app roll, the typed face for
// a typed one), or 0 and 0 when no song applies. A typed roll without the song's face is
// refused with SONG_OF_REST_FACE_NEEDED.
func (s *Service) songOfRestDie(ctx context.Context, tx pgx.Tx, q *playdb.Queries, sessionID, campaignID, characterID string, in rollInput, typedFace int) (sides, face int, err error) {
	kind, err := q.GetLatestRestKindInSession(ctx, sessionID)
	if errors.Is(err, pgx.ErrNoRows) || (err == nil && kind != rules.RestShort) {
		return 0, 0, nil // outside a short rest there is no song to rest to
	}
	if err != nil {
		return 0, 0, fmt.Errorf("read the latest rest: %w", err)
	}
	if sides, err = s.vitals.SongOfRest(ctx, tx, campaignID); err != nil || sides == 0 {
		return 0, 0, err
	}
	sung, err := q.HasSongOfRestSinceLatestRest(ctx, playdb.HasSongOfRestSinceLatestRestParams{GameSessionID: sessionID, CharacterID: &characterID})
	if err != nil {
		return 0, 0, fmt.Errorf("read the song of rest: %w", err)
	}
	if sung {
		return 0, 0, nil // once for each rest
	}
	if in.inApp {
		rolled, err := dice.Roll(s.roller, dice.Expr{Count: 1, Sides: sides})
		if err != nil {
			return 0, 0, fmt.Errorf("roll the song of rest die: %w", err)
		}
		return sides, rolled.Faces[0], nil
	}
	switch {
	case typedFace == 0:
		return 0, 0, resourceBlocked(playv1.ResourceBlockedReason_RESOURCE_BLOCKED_REASON_SONG_OF_REST_FACE_NEEDED,
			"the party hears a Song of Rest: type the face of its die too", func(b *playv1.ResourceBlocked) { b.Needed = clamp32(sides, 0, maxHitDie) })
	case typedFace < 1 || typedFace > sides:
		return 0, 0, connect.NewError(connect.CodeInvalidArgument, errors.New("typed_song_face must be a face of the song's die"))
	}
	return sides, typedFace, nil
}

// maxHitDie is the largest hit die there is (the d12), a bound for the faces a request names.
const maxHitDie = 12

func spendResponse(v *playv1.CharacterVitals, ev hitDieEvent) *playv1.SpendHitDiceResponse {
	return &playv1.SpendHitDiceResponse{
		Vitals: v, Face: ev.Face, ConstitutionModifier: ev.Modifier, Healed: ev.Healed,
		SongOfRestSides: ev.SongSides, SongOfRestFace: ev.SongFace,
	}
}
