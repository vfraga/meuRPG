package characters

import (
	"context"
	"errors"
	"log/slog"
	"time"
	"unicode/utf8"

	"connectrpc.com/connect"
	"github.com/jackc/pgx/v5"
	"google.golang.org/protobuf/types/known/timestamppb"

	charactersv1 "github.com/PuraFome/meuRPG/backend/gen/meurpg/characters/v1"
	"github.com/PuraFome/meuRPG/backend/internal/authz"
	"github.com/PuraFome/meuRPG/backend/internal/characters/charactersdb"
	"github.com/PuraFome/meuRPG/backend/internal/platform/conv"
	"github.com/PuraFome/meuRPG/backend/internal/platform/db"
	"github.com/PuraFome/meuRPG/backend/internal/platform/idem"
	"github.com/PuraFome/meuRPG/backend/internal/platform/logging"
	"github.com/PuraFome/meuRPG/backend/internal/platform/names"
	"github.com/PuraFome/meuRPG/backend/internal/platform/uid"
	"github.com/PuraFome/meuRPG/backend/internal/play/link"
)

// "Pedir ajustes" (RN-15, MR-024).
//
// The master cannot only approve or reject a pending character (a rejection deletes
// it): he can send it back with a reason. The character stays PENDING, its player edits
// it and sends it again, and the master may approve or reject at any moment. The
// state of the request is a row of character_reviews; no row means REVIEW_AWAITING.
//
// The reason is free text of the master about the player, so it is personal data of the
// player (docs/privacy.md): only the master and the owning player read it, no log line
// carries it (only its length), and it is deleted when the character is approved or
// rejected and, by the table's foreign keys, when the character, the campaign or the
// player's account is deleted. After the resubmission it stays for the master's history
// until one of those.

// MaxReviewReasonLength is the longest reason, in characters.
const MaxReviewReasonLength = 500

// The database's values for character_reviews.status.
const (
	reviewChangesRequested = "changes_requested"
	reviewResubmitted      = "resubmitted"
)

// The events ReviewHost.PublishCharacterEvent tells the streams: the master asked for changes,
// the owner sent the character again, a dead character lives again. They are strings so that
// package play, which implements the interface, needs no import of this one.
const (
	hintChangesRequested = "changes_requested"
	hintResubmitted      = "resubmitted"
	hintRevived          = "revived"
)

// reviewToProto is a review row as the master and the owner read it.
func reviewToProto(r charactersdb.CharacterReview) *charactersv1.CharacterReview {
	out := &charactersv1.CharacterReview{
		Status:      charactersv1.Review_REVIEW_CHANGES_REQUESTED,
		Reason:      r.Reason,
		RequestedAt: timestamppb.New(r.RequestedAt),
	}
	if r.Status == reviewResubmitted {
		out.Status = charactersv1.Review_REVIEW_RESUBMITTED
	}
	if r.ResubmittedAt != nil {
		out.ResubmittedAt = timestamppb.New(*r.ResubmittedAt)
	}
	return out
}

// reviewStatusToProto is the status alone, which is all a list carries.
func reviewStatusToProto(status string) charactersv1.Review {
	switch status {
	case reviewChangesRequested:
		return charactersv1.Review_REVIEW_CHANGES_REQUESTED
	case reviewResubmitted:
		return charactersv1.Review_REVIEW_RESUBMITTED
	}
	return charactersv1.Review_REVIEW_AWAITING
}

// mayReadReview says whether the caller is the master or the character's owner: the only
// ones who read the review (RN-10).
func mayReadReview(m authz.Membership, row charactersdb.Character) bool {
	if isMaster(m) {
		return true
	}
	return row.PlayerUserID != nil && *row.PlayerUserID == m.UserID
}

// fillReview sets what a pending character carries for the master and the owner: the review
// (REVIEW_AWAITING when the master asked nothing) and whether the owner may send it again.
// Everyone else, and any other character, gets nothing. It reads outside a transaction.
func (s *Service) fillReview(ctx context.Context, c *charactersv1.Character, row charactersdb.Character, m authz.Membership) error {
	if row.Kind != kindPlayer || row.Status != statusPending || !mayReadReview(m, row) {
		return nil
	}
	r, err := s.queries.GetCharacterReview(ctx, row.ID)
	switch {
	case errors.Is(err, pgx.ErrNoRows):
		c.Review = &charactersv1.CharacterReview{Status: charactersv1.Review_REVIEW_AWAITING}
		return nil
	case err != nil:
		return wrap("read the review", err)
	}
	c.Review = reviewToProto(r)
	c.CanResubmit = !isMaster(m) && r.Status == reviewChangesRequested
	return nil
}

// requiredKey checks the idempotency key of a call that needs one.
func requiredKey(raw string) (string, error) {
	key, err := idem.Clean(raw)
	if err != nil {
		return "", invalidArgument(fieldErr("idempotency_key", "must be 1 to 64 characters"))
	}
	if key == "" {
		return "", invalidArgument(fieldErr("idempotency_key", "is required"))
	}
	return key, nil
}

// keyOf is the key as it is stored: scoped to the campaign and the caller, so two people
// can reuse the same string (idem.Scope).
func keyOf(m authz.Membership, key string) *string {
	return idem.Scope(m.CampaignID+":"+m.UserID, key)
}

// RequestCharacterChanges implements charactersv1connect.CharacterServiceHandler.
func (s *Service) RequestCharacterChanges(
	ctx context.Context,
	req *connect.Request[charactersv1.RequestCharacterChangesRequest],
) (*connect.Response[charactersv1.RequestCharacterChangesResponse], error) {
	in, err := changeRequestOf(ctx, req.Msg)
	if err != nil {
		return nil, err
	}
	m := in.m
	content, err := s.contentFor(ctx, nil, m.CampaignID) // before the write: a failure after the commit would make the client retry it
	if err != nil {
		return nil, s.dbError(ctx, "read rules content", err)
	}
	var row charactersdb.Character
	var replayed bool
	err = db.InTx(ctx, s.pool, func(tx pgx.Tx) error {
		var err error
		row, replayed, err = s.requestChangesTx(ctx, s.queries.WithTx(tx), in)
		return err
	})
	if err != nil {
		return nil, s.dbError(ctx, "request changes of a character", err)
	}
	if !replayed {
		logging.Event(ctx, s.logger, "character.changes_requested", slog.String("character_id", row.ID), slog.Int("reason_length", utf8.RuneCountInString(in.reason)))
		s.publishCharacterEvent(ctx, m.CampaignID, row, hintChangesRequested)
	}
	c, err := s.characterAfterWrite(ctx, content, row, m)
	if err != nil {
		return nil, err
	}
	return connect.NewResponse(&charactersv1.RequestCharacterChangesResponse{Character: c}), nil
}

// changeRequest is a RequestCharacterChanges request once checked: the master's
// membership, the character's ID, the cleaned reason, and the scoped key and hash of
// the request (idem).
type changeRequest struct {
	m            authz.Membership
	id, reason   string
	scoped, hash *string
}

// changeRequestOf checks a RequestCharacterChanges request before anything is read: the
// caller, the character's ID, the key and the reason, in this order.
func changeRequestOf(ctx context.Context, msg *charactersv1.RequestCharacterChangesRequest) (changeRequest, error) {
	m, id, err := masterAndCharacter(ctx, msg.GetCampaignId(), msg.GetCharacterId())
	if err != nil {
		return changeRequest{}, err
	}
	key, err := requiredKey(msg.GetIdempotencyKey())
	if err != nil {
		return changeRequest{}, err
	}
	reason, err := names.CleanText(msg.GetReason(), MaxReviewReasonLength)
	if err != nil {
		return changeRequest{}, invalidArgument(fieldErr("reason", "must be at most %d characters", MaxReviewReasonLength))
	}
	if reason == "" {
		return changeRequest{}, invalidArgument(fieldErr("reason", "is required"))
	}
	return changeRequest{m: m, id: id, reason: reason, scoped: keyOf(m, key), hash: idem.Hash(msg)}, nil
}

// requestChangesTx is RequestCharacterChanges's transaction: it locks the character and
// writes the request, unless the key's first call did it already (replayed).
func (s *Service) requestChangesTx(ctx context.Context, q *charactersdb.Queries, in changeRequest) (row charactersdb.Character, replayed bool, err error) {
	row, err = visibleForUpdate(ctx, q, in.m, in.id)
	if err != nil {
		return row, false, err
	}
	if err := requireAnswerablePending(row); err != nil {
		return row, false, err
	}
	replayed, err = requestReplayed(ctx, q, row.ID, in.scoped, in.hash)
	if err != nil || replayed {
		return row, replayed, err
	}
	if _, err := q.UpsertCharacterReviewRequest(ctx, charactersdb.UpsertCharacterReviewRequestParams{
		CharacterID: row.ID, CampaignID: in.m.CampaignID, PlayerUserID: *row.PlayerUserID, Reason: in.reason,
		Now: s.now(), RequestKey: in.scoped, RequestHash: in.hash,
	}); err != nil {
		return row, false, wrap("request changes", err)
	}
	return row, false, nil
}

// requireAnswerablePending refuses a character the master cannot send back: an NPC, one
// that does not wait for approval, and one with no player left to answer.
func requireAnswerablePending(row charactersdb.Character) error {
	if row.Kind != kindPlayer {
		return invalidArgument(fieldErr("character_id", "is an NPC: only player characters wait for approval"))
	}
	if row.Status != statusPending {
		return errBlocked(charactersv1.CharacterBlockedReason_CHARACTER_BLOCKED_REASON_NOT_PENDING, row.ID)
	}
	if row.PlayerUserID == nil {
		return invalidArgument(fieldErr("character_id", "has no player to answer: approve or reject it"))
	}
	return nil
}

// requestReplayed says whether the character's review was written by a call with this
// key: the first call did it, so nothing changes and nothing is published. The same key
// with another request is refused.
func requestReplayed(ctx context.Context, q *charactersdb.Queries, characterID string, scoped, hash *string) (bool, error) {
	prior, err := q.GetCharacterReview(ctx, characterID)
	switch {
	case err == nil && prior.RequestKey != nil && *prior.RequestKey == *scoped:
		if err := idem.SameRequest(prior.RequestHash, hash); err != nil {
			return false, err
		}
		return true, nil
	case err != nil && !errors.Is(err, pgx.ErrNoRows):
		return false, wrap("read the review", err)
	}
	return false, nil
}

// ResubmitCharacter implements charactersv1connect.CharacterServiceHandler.
func (s *Service) ResubmitCharacter(
	ctx context.Context,
	req *connect.Request[charactersv1.ResubmitCharacterRequest],
) (*connect.Response[charactersv1.ResubmitCharacterResponse], error) {
	in, err := resubmissionOf(ctx, req.Msg)
	if err != nil {
		return nil, err
	}
	m := in.m
	content, err := s.contentFor(ctx, nil, m.CampaignID)
	if err != nil {
		return nil, s.dbError(ctx, "read rules content", err)
	}
	var row charactersdb.Character
	var replayed bool
	err = db.InTx(ctx, s.pool, func(tx pgx.Tx) error {
		var err error
		row, replayed, err = s.resubmitTx(ctx, s.queries.WithTx(tx), in)
		return err
	})
	if err != nil {
		return nil, s.dbError(ctx, "resubmit a character", err)
	}
	if !replayed {
		logging.Event(ctx, s.logger, "character.resubmitted", slog.String("character_id", row.ID))
		s.publishCharacterEvent(ctx, m.CampaignID, row, hintResubmitted)
	}
	c, err := s.characterAfterWrite(ctx, content, row, m)
	if err != nil {
		return nil, err
	}
	return connect.NewResponse(&charactersv1.ResubmitCharacterResponse{Character: c}), nil
}

// resubmission is a ResubmitCharacter request once checked: the caller's membership
// (a pending one too), the character's ID, and the scoped key and hash of the request.
type resubmission struct {
	m            authz.Membership
	id           string
	scoped, hash *string
}

// resubmissionOf checks a ResubmitCharacter request before anything is read: the
// caller, the character's ID and the key, in this order.
func resubmissionOf(ctx context.Context, msg *charactersv1.ResubmitCharacterRequest) (resubmission, error) {
	m, err := authz.RequireCampaignMemberOrPending(ctx, msg.GetCampaignId())
	if err != nil {
		return resubmission{}, err
	}
	id, ok := uid.Canonical(msg.GetCharacterId())
	if !ok {
		return resubmission{}, errCharacterNotFound()
	}
	key, err := requiredKey(msg.GetIdempotencyKey())
	if err != nil {
		return resubmission{}, err
	}
	return resubmission{m: m, id: id, scoped: keyOf(m, key), hash: idem.Hash(msg)}, nil
}

// resubmitTx is ResubmitCharacter's transaction: it locks the owner's pending character
// and marks the master's request answered, unless the key's first call did it already
// (replayed).
func (s *Service) resubmitTx(ctx context.Context, q *charactersdb.Queries, in resubmission) (row charactersdb.Character, replayed bool, err error) {
	row, err = visibleForUpdate(ctx, q, in.m, in.id)
	if err != nil {
		return row, false, err
	}
	// Only the owner: the master and any other member get the answer of a character
	// that is not there, so nobody probes an ID.
	if isMaster(in.m) || row.PlayerUserID == nil || *row.PlayerUserID != in.m.UserID {
		return row, false, errCharacterNotFound()
	}
	if row.Status != statusPending {
		return row, false, errBlocked(charactersv1.CharacterBlockedReason_CHARACTER_BLOCKED_REASON_NOT_PENDING, row.ID)
	}
	replayed, err = resubmitReplayed(ctx, q, row.ID, in.scoped, in.hash)
	if err != nil || replayed {
		return row, replayed, err
	}
	now := s.now()
	if _, err := q.MarkCharacterReviewResubmitted(ctx, charactersdb.MarkCharacterReviewResubmittedParams{
		CharacterID: row.ID, Now: &now, ResubmitKey: in.scoped, ResubmitHash: in.hash,
	}); err != nil {
		return row, false, wrap("resubmit a character", err)
	}
	return row, false, nil
}

// resubmitReplayed says whether the character was resubmitted by a call with this key
// (the same key with another request is refused), and refuses a character the master
// asked nothing of: no review, or one answered already.
func resubmitReplayed(ctx context.Context, q *charactersdb.Queries, characterID string, scoped, hash *string) (bool, error) {
	prior, err := q.GetCharacterReview(ctx, characterID)
	switch {
	case err == nil && prior.ResubmitKey != nil && *prior.ResubmitKey == *scoped:
		if err := idem.SameRequest(prior.ResubmitHash, hash); err != nil {
			return false, err
		}
		return true, nil
	case errors.Is(err, pgx.ErrNoRows) || (err == nil && prior.Status != reviewChangesRequested):
		return false, errBlocked(charactersv1.CharacterBlockedReason_CHARACTER_BLOCKED_REASON_NO_CHANGES_REQUESTED, characterID)
	case err != nil:
		return false, wrap("read the review", err)
	}
	return false, nil
}

// ReviewHost is what the characters module needs from the play module for the
// reviews and the revivals: the streams, and a combat that holds the character. cmd/api
// connects play.Service (SetReviewHost); without it, a review or a revival still works and
// nothing reaches a stream or a combat.
type ReviewHost interface {
	// PublishCharacterEvent tells the master and the character's owner (ownerUserID, empty
	// for none) that a review changed or a character lives again. A hint with no content
	// (RN-10): the reason is read, never streamed. Call it after the commit.
	PublishCharacterEvent(ctx context.Context, campaignID, ownerUserID, characterID, event string)
	// ReviveInCombat puts the character back in the campaign's combat that is not ended,
	// inside tx, if it is one of its combatants: no longer defeated, no death save counted,
	// still in its place in the order, and acting on its next turn. It writes the line of
	// the combat's log ("O mestre reviveu <name>") and returns the combat's ID, empty when
	// the character is in none (the caller then writes the line to the session).
	ReviveInCombat(ctx context.Context, tx pgx.Tx, campaignID, characterID, actorUserID string, at time.Time) (encounterID string, err error)
	// LockSession takes the campaign's open session row FOR UPDATE, inside tx, and does nothing when
	// the campaign has none. A change of gear locks the character's row, and play's combat writes
	// lock the session's row first and then reach the character's: so a path that goes on to call
	// EquipInCombat, GearChanged or ArmorWorn calls this before it locks the character, and the two
	// take their locks in the one order, session then character.
	// It is the same method as CreatureHost.LockSession.
	LockSession(ctx context.Context, tx pgx.Tx, campaignID string) error
	// ArmorWorn tells play that the character's sheet wears armor now, inside tx: the effects
	// that need none (Mage Armor, SRD 5.1: "the spell ends if the target dons armor") end. It
	// returns what to call after the commit (the streams are told), or nil.
	ArmorWorn(ctx context.Context, tx pgx.Tx, campaignID, characterID string) (afterCommit func(ctx context.Context), err error)
	// GearChanged tells play that the character's armor or shield changed without donning
	// armor, inside tx: the base armor class an effect gives (Mage Armor, 13 + Dexterity, with
	// the shield) is worked out again from the gear it wears now. It returns what to call after
	// the commit, or nil.
	GearChanged(ctx context.Context, tx pgx.Tx, campaignID, characterID string) (afterCommit func(ctx context.Context), err error)
	// EquipInCombat tells play that the character equips or stows a piece of gear, inside
	// the transaction that writes the sheet and before it writes, only when the state really
	// changes. Play may refuse (a coded error, which aborts the write) or spend the turn's
	// economy for the change in the combat that holds the character. It returns what to call
	// after the commit (the combat's streams are told), or nil.
	EquipInCombat(ctx context.Context, tx pgx.Tx, campaignID, characterID, actorUserID string, change EquipChange) (afterCommit func(ctx context.Context), err error)
	// PublishEncounterChanged tells every stream that the combat changed. Call it after the
	// commit.
	PublishEncounterChanged(ctx context.Context, campaignID, encounterID string)
}

// EquipKind is the kind of gear a change moves; it is package link's type, which play shares.
type EquipKind = link.EquipKind

// EquipChange is one change of gear; it is package link's type, which play shares.
type EquipChange = link.EquipChange

// The kinds of gear SetEquipped moves.
const (
	EquipWeapon = link.EquipWeapon
	EquipShield = link.EquipShield
	EquipArmor  = link.EquipArmor
)

// lockSessionForGear takes play's session lock ahead of a character's lock (ReviewHost.LockSession).
func (s *Service) lockSessionForGear(ctx context.Context, tx pgx.Tx, campaignID string) error {
	if s.reviewHost == nil {
		return nil
	}
	return s.reviewHost.LockSession(ctx, tx, campaignID)
}

// lockSessionBeforeCharacter takes play's session lock ahead of the character's lock for a path that
// reaches play's combat and session writes inside its transaction (a sheet save that changes gear, a revive), unless the
// character's row, read here without a lock, settles that the path ends before play is reached: the
// character is not found, it is not one the path serves (skip), or its revision is ahead of the
// request's (the revision check refuses it as stale; a revision never goes back). Any other case takes
// the lock, so a path that the unlocked read could not rule out keeps the one order, session then character.
func (s *Service) lockSessionBeforeCharacter(ctx context.Context, tx pgx.Tx, campaignID, characterID string, skip func(charactersdb.Character) bool) error {
	if s.reviewHost == nil {
		return nil
	}
	pre, err := s.queries.WithTx(tx).GetCharacter(ctx, charactersdb.GetCharacterParams{CampaignID: campaignID, ID: characterID})
	switch {
	case errors.Is(err, pgx.ErrNoRows):
		return nil // the path answers: not found
	case err != nil:
		return wrap("read character", err)
	case skip(pre):
		return nil
	}
	return s.lockSessionForGear(ctx, tx, campaignID)
}

// SetReviewHost connects the play module.
func (s *Service) SetReviewHost(h ReviewHost) { s.reviewHost = h }

// publishCharacterEvent tells the master and the owner of row, after the commit.
func (s *Service) publishCharacterEvent(ctx context.Context, campaignID string, row charactersdb.Character, event string) {
	if s.reviewHost == nil {
		return
	}
	s.reviewHost.PublishCharacterEvent(ctx, campaignID, conv.OrZero(row.PlayerUserID), row.ID, event)
}

// reviewStatuses reads the status of each open review of the campaign, by character ID.
func (s *Service) reviewStatuses(ctx context.Context, campaignID string) (map[string]string, error) {
	rows, err := s.queries.ListCharacterReviewStatuses(ctx, campaignID)
	if err != nil {
		return nil, wrap("list the reviews", err)
	}
	out := make(map[string]string, len(rows))
	for _, r := range rows {
		out[r.CharacterID] = r.Status
	}
	return out, nil
}
