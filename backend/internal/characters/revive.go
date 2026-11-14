package characters

import (
	"context"
	"encoding/json"
	"errors"
	"log/slog"
	"time"

	"connectrpc.com/connect"
	"github.com/jackc/pgx/v5"

	charactersv1 "github.com/PuraFome/meuRPG/backend/gen/meurpg/characters/v1"
	playv1 "github.com/PuraFome/meuRPG/backend/gen/meurpg/play/v1"
	"github.com/PuraFome/meuRPG/backend/internal/authz"
	"github.com/PuraFome/meuRPG/backend/internal/characters/charactersdb"
	"github.com/PuraFome/meuRPG/backend/internal/platform/conv"
	"github.com/PuraFome/meuRPG/backend/internal/platform/idem"
	"github.com/PuraFome/meuRPG/backend/internal/platform/logging"
	"github.com/PuraFome/meuRPG/backend/internal/platform/sessiontell"
	"github.com/PuraFome/meuRPG/backend/internal/platform/uid"
)

// Reviver (RN-03, SRD 5.1 "Dropping to 0 Hit Points").
//
// A dead character is never deleted, so bringing it back changes its status again: 'active',
// with the sheet's lock as it was (a death never touched it), 1 hit point and no death save
// counted. It is the master's power, not a spell: it spends nothing and has no time limit. The
// same function serves the spell Revivify (ReviveDead, for package play), so both bring a
// character back the same way.
//
// RN-03 allows one living character per player in a campaign. A player who made another one
// after the death cannot have the dead one back until the master archives it or marks it dead:
// the refusal names it. Nothing here does that by itself.

// reviveHitPoints is what a creature that lives again has (SRD 5.1, Revivify; "Reviver" gives
// the same).
const reviveHitPoints = 1

// ReviveCharacter implements charactersv1connect.CharacterServiceHandler.
func (s *Service) ReviveCharacter(
	ctx context.Context,
	req *connect.Request[charactersv1.ReviveCharacterRequest],
) (*connect.Response[charactersv1.ReviveCharacterResponse], error) {
	m, err := authz.RequireCampaignRole(ctx, req.Msg.GetCampaignId(), authz.RoleMaster)
	if err != nil {
		return nil, err
	}
	id, ok := uid.Canonical(req.Msg.GetCharacterId())
	if !ok {
		return nil, errCharacterNotFound()
	}
	key, err := requiredKey(req.Msg.GetIdempotencyKey())
	if err != nil {
		return nil, err
	}
	scoped, hash := keyOf(m, key), idem.Hash(req.Msg)

	content, err := s.contentFor(ctx, nil, m.CampaignID)
	if err != nil {
		return nil, s.dbError(ctx, "read rules content", err)
	}
	var done revival
	now := s.now()
	// Under the funnel: the session's row first, then the character's, and the write moves the session's number once.
	err = s.inSession(ctx, m.CampaignID, func(tx pgx.Tx, t sessiontell.Tell) error {
		var err error
		done, err = s.reviveCharacterTx(ctx, tx, m, id, scoped, hash, now)
		if err != nil {
			return err
		}
		if done.replayed {
			t.Replay() // the first call may have died before it told: the repeat tells again, with the number as it stands
		}
		tellRevived(t, done.row, done.encounterID)
		return nil
	})
	if err != nil {
		return nil, s.dbError(ctx, "revive a character", err)
	}
	if !done.replayed {
		logging.Event(ctx, s.logger, "character.revived", slog.String("character_id", done.row.ID))
	}
	c, err := s.character(ctx, content, done.row, m)
	if err != nil {
		return nil, s.dbError(ctx, "read a character", err)
	}
	return connect.NewResponse(&charactersv1.ReviveCharacterResponse{Character: c}), nil
}

// revival is what one try of ReviveCharacter's transaction did: the character as it is
// now, whether the call was a retry of one that brought it back, and the combat that
// holds it (empty for none).
type revival struct {
	row         charactersdb.Character
	replayed    bool
	encounterID string
}

// reviveCharacterTx is ReviveCharacter's transaction: the character, locked FOR UPDATE, must be
// a dead player character, or the same call again; then it lives again and the table
// reads the line.
func (s *Service) reviveCharacterTx(ctx context.Context, tx pgx.Tx, m authz.Membership, id string, key, hash *string, now time.Time) (revival, error) {
	// The session's row is held already (inSession), before the character's: a dead character comes back in its combat, which
	// writes under that lock (ReviveInCombat), as every combat write does.
	row, err := visibleForUpdate(ctx, s.queries.WithTx(tx), m, id)
	if err != nil {
		return revival{}, err
	}
	if row.Kind != kindPlayer {
		return revival{}, invalidArgument(fieldErr("character_id", "is an NPC: only player characters die"))
	}
	if row.Status != statusDead {
		replayed, err := reviveReplayed(row, key, hash)
		return revival{row: row, replayed: replayed}, err
	}
	if row, err = s.revivedRow(ctx, tx, row, key, hash, now); err != nil {
		return revival{}, err
	}
	done := revival{row: row}
	err = s.revivedInCombat(ctx, tx, m.CampaignID, row, m.UserID, now, &done.encounterID)
	return done, err
}

// reviveReplayed says whether a living character is the answer to a retry of the call
// that brought it back: the same key and request. Any other call is NOT_DEAD.
func reviveReplayed(row charactersdb.Character, key, hash *string) (bool, error) {
	if row.ReviveKey == nil || *row.ReviveKey != *key || row.RevivedAt == nil {
		return false, errBlocked(charactersv1.CharacterBlockedReason_CHARACTER_BLOCKED_REASON_NOT_DEAD, row.ID)
	}
	if err := idem.SameRequest(row.ReviveHash, hash); err != nil {
		return false, err
	}
	return true, nil
}

// revivedRow brings the dead character row back inside tx: RN-03 first, then the status, then
// its hit points. row is the character locked FOR UPDATE and dead. It returns the character
// as it is now.
func (s *Service) revivedRow(ctx context.Context, tx pgx.Tx, row charactersdb.Character, key, hash *string, at time.Time) (charactersdb.Character, error) {
	q := s.queries.WithTx(tx)
	if row.PlayerUserID != nil && row.CampaignID != nil {
		other, err := q.GetLivingPlayerCharacterID(ctx, charactersdb.GetLivingPlayerCharacterIDParams{CampaignID: *row.CampaignID, PlayerUserID: *row.PlayerUserID})
		switch {
		case err == nil:
			return row, s.livingExists(ctx, q, *row.CampaignID, other)
		case !errors.Is(err, pgx.ErrNoRows):
			return row, wrap("read the living character", err)
		}
	}
	revived, err := q.ReviveCharacter(ctx, charactersdb.ReviveCharacterParams{
		CampaignID: conv.OrZero(row.CampaignID), ID: row.ID, Now: at, ReviveKey: key, ReviveHash: hash,
	})
	if err != nil {
		return row, wrap("revive a character", err)
	}
	hp := int32(reviveHitPoints)
	if _, _, err := s.AdjustVitals(ctx, tx, conv.OrZero(revived.CampaignID), revived.ID, &playv1.AdjustCharacterVitalsRequest{HitPointsCurrent: &hp}); err != nil {
		return row, err
	}
	return revived, nil
}

// livingExists is the refusal of RN-03: the player has another living character. The error
// names it, so the master can open it and archive it or mark it dead.
func (s *Service) livingExists(ctx context.Context, q *charactersdb.Queries, campaignID, otherID string) error {
	other, err := q.GetCharacter(ctx, charactersdb.GetCharacterParams{CampaignID: campaignID, ID: otherID})
	if err != nil {
		return wrap("read the living character", err)
	}
	e := connect.NewError(connect.CodeFailedPrecondition, errors.New("the player already has another living character in this campaign: archive it or mark it dead first"))
	if detail, detailErr := connect.NewErrorDetail(&charactersv1.CharacterBlocked{
		Reason:              charactersv1.CharacterBlockedReason_CHARACTER_BLOCKED_REASON_LIVING_CHARACTER_EXISTS,
		CharacterId:         other.ID,
		LivingCharacterName: other.Name,
	}); detailErr == nil {
		e.AddDetail(detail)
	}
	return e
}

// revivedInCombat puts the character back in the combat that holds it, and writes the line
// the table reads: "O mestre reviveu <name>" in the combat's log, or in the session's when no
// combat has it. encounterID is set to the combat's, if any.
func (s *Service) revivedInCombat(ctx context.Context, tx pgx.Tx, campaignID string, row charactersdb.Character, actorUserID string, at time.Time, encounterID *string) error {
	if s.reviewHost == nil {
		return nil
	}
	enc, err := s.reviewHost.ReviveInCombat(ctx, tx, campaignID, row.ID, actorUserID, at)
	if err != nil {
		return err
	}
	*encounterID = enc
	if enc != "" || s.creatureHost == nil {
		return nil
	}
	body, err := json.Marshal(map[string]string{"character_id": row.ID})
	if err != nil {
		return wrap("encode the event", err)
	}
	if _, err := s.creatureHost.AppendEvent(ctx, tx, campaignID, eventCharacterRevived, actorUserID, body, at); err != nil {
		return err
	}
	return nil
}

// tellRevived tells the master and the owner, and the combat when the character was in one, once the transaction committed.
func tellRevived(t sessiontell.Tell, row charactersdb.Character, encounterID string) {
	t.CharacterEvent(conv.OrZero(row.PlayerUserID), row.ID, hintRevived)
	t.EncounterChanged(encounterID)
}

// eventCharacterRevived is the session event kind of a revival outside a combat (the table
// session_event_kinds has it).
const eventCharacterRevived = "character_revived"

// ReviveDead implements play.CombatRoster for Revivify: the dead character lives again
// inside tx, as ReviveCharacter does it (RN-03 included: `failed_precondition` and nothing
// changes when the player has another living character). It takes no caller, like MarkDead: the
// spell was checked by whoever calls it. It does not touch a combat or write a log line:
// package play does, in the same transaction. A character that is not dead is
// `failed_precondition`.
func (s *Service) ReviveDead(ctx context.Context, tx pgx.Tx, campaignID, characterID string, at time.Time) (name string, err error) {
	id, ok := uid.Canonical(characterID)
	if !ok {
		return "", errCharacterNotFound()
	}
	q := s.queries.WithTx(tx)
	row, err := q.GetCharacterForUpdate(ctx, charactersdb.GetCharacterForUpdateParams{CampaignID: campaignID, ID: id})
	if errors.Is(err, pgx.ErrNoRows) {
		return "", errCharacterNotFound()
	}
	if err != nil {
		return "", wrap("read character", err)
	}
	if row.Kind != kindPlayer || row.Status != statusDead {
		return "", errBlocked(charactersv1.CharacterBlockedReason_CHARACTER_BLOCKED_REASON_NOT_DEAD, row.ID)
	}
	if _, err := s.revivedRow(ctx, tx, row, nil, nil, at); err != nil {
		return "", err
	}
	return row.Name, nil
}
