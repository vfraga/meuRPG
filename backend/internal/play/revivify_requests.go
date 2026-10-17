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
	"google.golang.org/protobuf/types/known/timestamppb"

	playv1 "github.com/PuraFome/meuRPG/backend/gen/meurpg/play/v1"
	"github.com/PuraFome/meuRPG/backend/internal/authz"
	"github.com/PuraFome/meuRPG/backend/internal/platform/db"
	"github.com/PuraFome/meuRPG/backend/internal/platform/idem"
	"github.com/PuraFome/meuRPG/backend/internal/platform/logging"
	"github.com/PuraFome/meuRPG/backend/internal/play/link"
	"github.com/PuraFome/meuRPG/backend/internal/play/live"
	"github.com/PuraFome/meuRPG/backend/internal/play/playdb"
)

// Revivify outside a combat (SRD 5.1, Revivify). The spell reaches back one minute and the app
// does not count time outside a combat, so the cast waits for the master (REVIVIFY_TIME_PENDING):
// nothing is spent, and the target is not touched, until he says whether the creature died less
// than a minute ago. "Já passou" ends it at no cost, and the caster is told only that the master
// said it does not work (RN-10). A cast inside a combat is CastSpell.

// The database's statuses of a revivify request.
const (
	revivifyPending   = "pending"
	revivifyConfirmed = "confirmed"
	revivifyDenied    = "denied"
)

var revivifyStatusToProto = map[string]playv1.RevivifyRequestStatus{
	revivifyPending:   playv1.RevivifyRequestStatus_REVIVIFY_REQUEST_STATUS_PENDING,
	revivifyConfirmed: playv1.RevivifyRequestStatus_REVIVIFY_REQUEST_STATUS_CONFIRMED,
	revivifyDenied:    playv1.RevivifyRequestStatus_REVIVIFY_REQUEST_STATUS_DENIED,
}

// errRevivifyRequestNotFound is the answer for a request the caller may not answer or read:
// the same for one that does not exist, so no pending_id is probed.
func errRevivifyRequestNotFound() error {
	return connect.NewError(connect.CodeNotFound, errors.New("revivify request not found"))
}

// castersCharacter finds the living character that casts outside a combat, a player's own for a
// player. not_found for any other, as the same answer for one that is not there.
func (s *Service) castersCharacter(ctx context.Context, tx pgx.Tx, m authz.Membership, characterID string) (link.Character, error) {
	id, err := uuid.Parse(characterID)
	if err != nil {
		return link.Character{}, connect.NewError(connect.CodeNotFound, errors.New("character not found"))
	}
	chars, err := s.roster.CombatCharacters(ctx, tx, m.CampaignID, []string{id.String()})
	if err != nil {
		return link.Character{}, err
	}
	if len(chars) != 1 || !chars[0].Player {
		return link.Character{}, connect.NewError(connect.CodeNotFound, errors.New("character not found"))
	}
	if m.Role != authz.RoleMaster && chars[0].PlayerUserID != m.UserID {
		return link.Character{}, connect.NewError(connect.CodePermissionDenied, errors.New("only the character's player or the master may do this"))
	}
	return chars[0], nil
}

// outsideOptions is what the caster could cast now, outside a combat: a fresh turn.
func (s *Service) outsideOptions(ctx context.Context, tx pgx.Tx, campaignID string, caster link.Character) (castable, error) {
	opts, err := s.roster.CombatTurnOptions(ctx, tx, campaignID, caster.ID, link.Turn{})
	if err != nil {
		return castable{}, err
	}
	return castableOf(opts, revivifyKey)
}

// previewOutside is PreviewRevivify with no combat: every dead character of the campaign, to be
// confirmed by the master when the spell is cast.
func (s *Service) previewOutside(ctx context.Context, m authz.Membership, req *playv1.PreviewRevivifyRequest) (*connect.Response[playv1.PreviewRevivifyResponse], error) {
	caster, err := s.castersCharacter(ctx, nil, m, req.GetCasterCharacterId())
	if err != nil {
		return nil, err
	}
	cast, err := s.outsideOptions(ctx, nil, m.CampaignID, caster)
	if err != nil {
		return nil, err
	}
	dead, err := s.roster.DeadCharacters(ctx, nil, m.CampaignID)
	if err != nil {
		return nil, s.dbError(ctx, "list the dead", err)
	}
	res := &playv1.PreviewRevivifyResponse{}
	res.Slot, res.SlotsFree = revivifySlot(cast)
	for _, d := range dead {
		switch {
		case !d.RevivifyBlocked:
			res.Targets = append(res.Targets, &playv1.RevivifyTarget{TargetId: d.ID, Name: d.Name, NeedsMasterConfirmation: true})
		case m.Role == authz.RoleMaster:
			res.Unavailable = append(res.Unavailable, &playv1.RevivifyUnavailable{
				TargetId: d.ID, Name: d.Name, Reason: playv1.RevivifyUnavailableReason_REVIVIFY_UNAVAILABLE_REASON_MASTER_BLOCKED,
			})
		}
	}
	return connect.NewResponse(res), nil
}

// RequestRevivify implements playv1connect.RevivifyServiceHandler.
func (s *Service) RequestRevivify(
	ctx context.Context,
	req *connect.Request[playv1.RequestRevivifyRequest],
) (*connect.Response[playv1.RequestRevivifyResponse], error) {
	m, err := authz.RequireCampaignMember(ctx, req.Msg.GetCampaignId())
	if err != nil {
		return nil, err
	}
	key, err := parseKey(req.Msg.GetIdempotencyKey())
	if err != nil {
		return nil, err
	}
	if !req.Msg.GetMaterialConfirmed() {
		return nil, connect.NewError(connect.CodeInvalidArgument, errors.New("material_confirmed is required: the spell consumes diamonds worth 300 gp"))
	}
	targetID, err := uuid.Parse(req.Msg.GetTargetCharacterId())
	if err != nil {
		return nil, connect.NewError(connect.CodeNotFound, errors.New("character not found"))
	}
	scoped, hash := idem.Scope(m.CampaignID+":"+m.UserID, key), idem.Hash(req.Msg)

	var row playdb.RevivifyRequest
	var replayed bool
	err = db.InTx(ctx, s.pool, func(tx pgx.Tx) error {
		replayed = false
		q := s.queries.WithTx(tx)
		session, err := lockOpenSession(ctx, q, m.CampaignID)
		if err != nil {
			return err
		}
		get := func(ctx context.Context, k *string) (playdb.RevivifyRequest, error) {
			return q.GetRevivifyRequestByCreateKey(ctx, playdb.GetRevivifyRequestByCreateKeyParams{CampaignID: m.CampaignID, CreateKey: k})
		}
		row, replayed, err = idem.Create(ctx, scoped, hash, get,
			func(r playdb.RevivifyRequest) *string { return r.CreateHash },
			func() (playdb.RevivifyRequest, error) {
				return s.openRevivifyRequest(ctx, tx, q, m, session, req.Msg, targetID.String(), scoped, hash)
			})
		return err
	})
	if err != nil {
		return nil, s.dbError(ctx, "ask for Revivify", err)
	}
	if !replayed {
		logging.Event(ctx, s.logger, "revivify.requested", slog.String("request_id", row.ID), slog.String("character_id", row.CasterCharacterID))
		s.publishRevivifyChanged(m.CampaignID, deref(row.RequestedByUserID))
	}
	out, err := s.revivifyRequestsProto(ctx, m.CampaignID, []playdb.RevivifyRequest{row}, nil)
	if err != nil {
		return nil, err
	}
	return connect.NewResponse(&playv1.RequestRevivifyResponse{Request: out[0]}), nil
}

// openRevivifyRequest checks the cast and writes the request, inside the transaction. It returns
// pgx.ErrNoRows when the key's unique index already holds a request (idem.Create reads it).
func (s *Service) openRevivifyRequest(ctx context.Context, tx pgx.Tx, q *playdb.Queries, m authz.Membership, session playdb.GameSession, req *playv1.RequestRevivifyRequest, targetID string, scoped, hash *string) (playdb.RevivifyRequest, error) {
	caster, err := s.castersCharacter(ctx, tx, m, req.GetCasterCharacterId())
	if err != nil {
		return playdb.RevivifyRequest{}, err
	}
	if err := s.refuseCasterInCombat(ctx, q, session.ID, caster); err != nil {
		return playdb.RevivifyRequest{}, err
	}
	slot, err := s.outsideSlot(ctx, tx, m.CampaignID, caster, req.GetSlot())
	if err != nil {
		return playdb.RevivifyRequest{}, err
	}
	if err := s.requireDeadTarget(ctx, tx, m.CampaignID, targetID); err != nil {
		return playdb.RevivifyRequest{}, err
	}
	return q.InsertRevivifyRequest(ctx, playdb.InsertRevivifyRequestParams{
		CampaignID: m.CampaignID, GameSessionID: session.ID, CasterCharacterID: caster.ID, TargetCharacterID: targetID,
		RequestedByUserID: &m.UserID, SlotLevel: slot.Level, SlotPact: slot.Pact, CreatedAt: s.now(), CreateKey: scoped, CreateHash: hash,
	})
}

// refuseCasterInCombat stops a cast outside a combat for a caster that is in one: there the
// minute is counted.
func (s *Service) refuseCasterInCombat(ctx context.Context, q *playdb.Queries, sessionID string, caster link.Character) error {
	_, cs, found, err := openCombatants(ctx, q, sessionID)
	if err != nil || !found {
		return err
	}
	if slices.ContainsFunc(cs, playerNotDefeatedOf(caster.ID)) {
		return connect.NewError(connect.CodeInvalidArgument, errors.New("the caster is in a combat: cast Revivify inside it"))
	}
	return nil
}

// outsideSlot checks the caster has the spell and the slot the cast names.
func (s *Service) outsideSlot(ctx context.Context, tx pgx.Tx, campaignID string, caster link.Character, in *playv1.SpellSlot) (*slotRef, error) {
	cast, err := s.outsideOptions(ctx, tx, campaignID, caster)
	if err != nil {
		return nil, err
	}
	if !cast.enabled {
		if err := castError(cast.reason, true); err != nil {
			return nil, err
		}
	}
	slot, err := slotOf(in, cast)
	if err != nil {
		return nil, err
	}
	if slot == nil {
		return nil, connect.NewError(connect.CodeInvalidArgument, errors.New("slot must be a spell slot of the 3rd level or more"))
	}
	return slot, nil
}

// requireDeadTarget gives the same answer for a character that is not dead, one that does not
// exist and one the master marked (RN-10).
func (s *Service) requireDeadTarget(ctx context.Context, tx pgx.Tx, campaignID, targetID string) error {
	dead, err := s.roster.DeadCharacters(ctx, tx, campaignID)
	if err != nil {
		return err
	}
	if !slices.ContainsFunc(dead, func(d link.DeadCharacter) bool { return d.ID == targetID && !d.RevivifyBlocked }) {
		return connect.NewError(connect.CodeNotFound, errors.New("character not found"))
	}
	return nil
}

// ListRevivifyRequests implements playv1connect.RevivifyServiceHandler.
func (s *Service) ListRevivifyRequests(
	ctx context.Context,
	req *connect.Request[playv1.ListRevivifyRequestsRequest],
) (*connect.Response[playv1.ListRevivifyRequestsResponse], error) {
	m, err := authz.RequireCampaignMember(ctx, req.Msg.GetCampaignId())
	if err != nil {
		return nil, err
	}
	session, err := openSessionWith(ctx, s.queries, m.CampaignID)
	if err != nil {
		return nil, s.dbError(ctx, "find the open session", err)
	}
	rows, err := s.queries.ListRevivifyRequests(ctx, session.ID)
	if err != nil {
		return nil, s.dbError(ctx, "list the Revivify requests", err)
	}
	if m.Role != authz.RoleMaster { // a player reads the casts they made
		rows = slices.DeleteFunc(rows, func(r playdb.RevivifyRequest) bool { return deref(r.RequestedByUserID) != m.UserID })
	}
	out, err := s.revivifyRequestsProto(ctx, m.CampaignID, rows, nil)
	if err != nil {
		return nil, err
	}
	return connect.NewResponse(&playv1.ListRevivifyRequestsResponse{Requests: out}), nil
}

// revivifyAnswer is what the master's answer names, read before the transaction opens.
type revivifyAnswer struct {
	id           string
	scoped, hash *string
	within       bool
}

// revivifyAnswerOf reads the answer. Only the master answers: a player gets the same not_found
// for any pending_id.
func revivifyAnswerOf(m authz.Membership, msg *playv1.ConfirmRevivifyTimeRequest) (revivifyAnswer, error) {
	if m.Role != authz.RoleMaster {
		return revivifyAnswer{}, errRevivifyRequestNotFound()
	}
	id, err := uuid.Parse(msg.GetPendingId())
	if err != nil {
		return revivifyAnswer{}, errRevivifyRequestNotFound()
	}
	key, err := parseKey(msg.GetIdempotencyKey())
	if err != nil {
		return revivifyAnswer{}, err
	}
	return revivifyAnswer{id: id.String(), scoped: idem.Scope(m.CampaignID+":"+m.UserID, key), hash: idem.Hash(msg), within: msg.GetWithinMinute()}, nil
}

// revivifyAnswered is what the transaction of an answer leaves for after the commit.
type revivifyAnswered struct {
	row         playdb.RevivifyRequest
	replayed    bool
	vitals      []*playv1.CharacterVitals
	slotsLeft   int32
	encounterID string // the combat that had the dead character, if one did
}

// ConfirmRevivifyTime implements playv1connect.RevivifyServiceHandler.
func (s *Service) ConfirmRevivifyTime(
	ctx context.Context,
	req *connect.Request[playv1.ConfirmRevivifyTimeRequest],
) (*connect.Response[playv1.ConfirmRevivifyTimeResponse], error) {
	m, err := authz.RequireCampaignMember(ctx, req.Msg.GetCampaignId())
	if err != nil {
		return nil, err
	}
	in, err := revivifyAnswerOf(m, req.Msg)
	if err != nil {
		return nil, err
	}
	out := &revivifyAnswered{}
	err = db.InTx(ctx, s.pool, func(tx pgx.Tx) error {
		*out = revivifyAnswered{}
		return s.answerRevivify(ctx, tx, m, in, out)
	})
	if err != nil {
		return nil, s.dbError(ctx, "answer Revivify", err)
	}
	if !out.replayed {
		s.announceAnswer(ctx, m, out)
	}
	left := map[string]int32{out.row.ID: out.slotsLeft}
	rows, err := s.revivifyRequestsProto(ctx, m.CampaignID, []playdb.RevivifyRequest{out.row}, left)
	if err != nil {
		return nil, err
	}
	return connect.NewResponse(&playv1.ConfirmRevivifyTimeResponse{Request: rows[0]}), nil
}

// answerRevivify is the transaction of the answer: "less than a minute" revives the target,
// "já passou" ends the request at no cost.
func (s *Service) answerRevivify(ctx context.Context, tx pgx.Tx, m authz.Membership, in revivifyAnswer, out *revivifyAnswered) error {
	q := s.queries.WithTx(tx)
	session, err := lockOpenSession(ctx, q, m.CampaignID)
	if err != nil {
		return err
	}
	row, replayed, err := s.pendingRevivify(ctx, q, session, m, in)
	if err != nil {
		return err
	}
	out.row = row
	if replayed {
		out.replayed = true // the same answer again
		return nil
	}
	status := revivifyDenied
	if in.within {
		status = revivifyConfirmed
		if out.vitals, out.slotsLeft, out.encounterID, err = s.reviveOutside(ctx, tx, q, session, m, row); err != nil {
			return err
		}
	}
	at := s.now()
	out.row, err = q.AnswerRevivifyRequest(ctx, playdb.AnswerRevivifyRequestParams{ID: row.ID, Status: status, AnsweredAt: &at, AnswerKey: in.scoped, AnswerHash: in.hash})
	if err != nil {
		return fmt.Errorf("answer the request: %w", err)
	}
	return nil
}

// pendingRevivify reads and locks the request the answer names. It returns replayed for a request
// the same answer (same key, same request) already closed.
func (s *Service) pendingRevivify(ctx context.Context, q *playdb.Queries, session playdb.GameSession, m authz.Membership, in revivifyAnswer) (playdb.RevivifyRequest, bool, error) {
	row, err := q.GetRevivifyRequestForUpdate(ctx, playdb.GetRevivifyRequestForUpdateParams{CampaignID: m.CampaignID, ID: in.id})
	if errors.Is(err, pgx.ErrNoRows) || (err == nil && row.GameSessionID != session.ID) {
		return row, false, errRevivifyRequestNotFound()
	}
	if err != nil {
		return row, false, fmt.Errorf("read the request: %w", err)
	}
	if row.Status == revivifyPending {
		return row, false, nil
	}
	if row.AnswerKey != nil && *row.AnswerKey == *in.scoped {
		return row, true, idem.SameRequest(row.AnswerHash, in.hash)
	}
	return row, false, connect.NewError(connect.CodeFailedPrecondition, errors.New("the request was answered already"))
}

// announceAnswer logs the answer and tells the table: the master and the caster's player, the
// vitals the slot changed and, for a confirmation, the character that came back.
func (s *Service) announceAnswer(ctx context.Context, m authz.Membership, out *revivifyAnswered) {
	row := out.row
	logging.Event(ctx, s.logger, "revivify.answered", slog.String("request_id", row.ID), slog.String("status", row.Status))
	s.publishRevivifyChanged(m.CampaignID, deref(row.RequestedByUserID))
	for _, v := range out.vitals {
		s.publishVitals(m.CampaignID, v)
	}
	if row.Status != revivifyConfirmed {
		return
	}
	s.PublishCharacterEvent(ctx, m.CampaignID, s.ownerOf(ctx, m.CampaignID, row.TargetCharacterID), row.TargetCharacterID, characterRevived)
	if out.encounterID != "" {
		s.PublishEncounterChanged(ctx, m.CampaignID, out.encounterID)
	}
}

// reviveOutside is the master's "less than a minute": the slot is spent, the diamonds are noted,
// and the target lives again with 1 hit point. RN-03 may refuse, and nothing is spent then. If a
// combat that is not ended still holds the character as defeated, its combatant lives again too,
// as after the master's Reviver (it returns that combat's ID, "" for none).
func (s *Service) reviveOutside(ctx context.Context, tx pgx.Tx, q *playdb.Queries, session playdb.GameSession, m authz.Membership, row playdb.RevivifyRequest) ([]*playv1.CharacterVitals, int32, string, error) {
	c, err := s.openTx(ctx, combatTx{tx: tx, q: q, session: session, svc: s})
	if err != nil {
		return nil, 0, "", err
	}
	slot := slotRef{Level: row.SlotLevel, Pact: row.SlotPact}
	spent, err := s.spendSlot(ctx, c, row.CasterCharacterID, slot, 1)
	if err != nil {
		return nil, 0, "", err
	}
	now := s.now()
	if _, err := s.roster.ReviveDead(ctx, tx, m.CampaignID, row.TargetCharacterID, now); err != nil {
		return nil, 0, "", err
	}
	encounterID, err := s.ReviveInCombat(ctx, tx, m.CampaignID, row.TargetCharacterID, m.UserID, now)
	if err != nil {
		return nil, 0, "", err
	}
	after, err := s.vitals.GetVitalsTx(ctx, tx, m.CampaignID, row.TargetCharacterID)
	if err != nil {
		return nil, 0, "", err
	}
	if encounterID == "" { // no combat has it: the line goes to the session
		body := fmt.Sprintf(`{"character_id":%q,"caster_character_id":%q,"spell":"revivify","material":true}`, row.TargetCharacterID, row.CasterCharacterID)
		if _, err := s.AppendEvent(ctx, tx, m.CampaignID, eventCharacterRevived, m.UserID, []byte(body), now); err != nil {
			return nil, 0, "", err
		}
	}
	return []*playv1.CharacterVitals{spent, after}, slotsLeftOf(spent, slot), encounterID, nil
}

// slotsLeftOf is how many slots of the level are free in the vitals.
func slotsLeftOf(v *playv1.CharacterVitals, slot slotRef) int32 {
	if slot.Pact {
		return max(v.GetPactSlots().GetTotal()-v.GetPactSlots().GetUsed(), 0)
	}
	for _, u := range v.GetSpellSlots() {
		if u.GetLevel() == slot.Level {
			return max(u.GetTotal()-u.GetUsed(), 0)
		}
	}
	return 0
}

// ownerOf is the account that plays the character, empty for none.
func (s *Service) ownerOf(ctx context.Context, campaignID, characterID string) string {
	chars, err := s.roster.SessionCharacters(ctx, nil, campaignID, []string{characterID})
	if err != nil || len(chars) != 1 {
		return ""
	}
	return chars[0].PlayerUserID
}

// revivifyRequestsProto builds the requests as the caller reads them, with the names of the
// characters. slotsLeft, when not nil, is the slots left after a confirmed cast, by request.
func (s *Service) revivifyRequestsProto(ctx context.Context, campaignID string, rows []playdb.RevivifyRequest, slotsLeft map[string]int32) ([]*playv1.RevivifyRequest, error) {
	var ids []string
	for _, r := range rows {
		ids = append(ids, r.CasterCharacterID, r.TargetCharacterID)
	}
	names := map[string]string{}
	chars, err := s.roster.SessionCharacters(ctx, nil, campaignID, ids)
	if err != nil {
		return nil, s.dbError(ctx, "read the names", err)
	}
	for _, c := range chars {
		names[c.ID] = c.Name
	}
	out := make([]*playv1.RevivifyRequest, 0, len(rows))
	for _, r := range rows {
		p := &playv1.RevivifyRequest{
			Id: r.ID, Status: revivifyStatusToProto[r.Status], CasterCharacterId: r.CasterCharacterID, CasterName: names[r.CasterCharacterID],
			TargetCharacterId: r.TargetCharacterID, TargetName: names[r.TargetCharacterID],
			Slot: &playv1.SpellSlot{Level: r.SlotLevel, Pact: r.SlotPact}, CreatedAt: timestamppb.New(r.CreatedAt),
		}
		if r.AnsweredAt != nil {
			p.AnsweredAt = timestamppb.New(*r.AnsweredAt)
		}
		if r.Status == revivifyConfirmed {
			p.SlotsLeft = slotsLeft[r.ID]
		}
		out = append(out, p)
	}
	return out, nil
}

// publishRevivifyChanged tells the master and the caster's player that the casts changed.
func (s *Service) publishRevivifyChanged(campaignID, casterUserID string) {
	s.hub.Publish(campaignID, live.Event{
		Audience: live.Audience{Master: true, UserID: casterUserID},
		Message: &playv1.WatchGameSessionResponse{Event: &playv1.WatchGameSessionResponse_RevivifyChanged_{
			RevivifyChanged: &playv1.WatchGameSessionResponse_RevivifyChanged{},
		}},
	})
}
