package characters

import (
	"context"
	"errors"
	"fmt"

	"connectrpc.com/connect"
	"github.com/jackc/pgx/v5"

	charactersv1 "github.com/PuraFome/meuRPG/backend/gen/meurpg/characters/v1"
	"github.com/PuraFome/meuRPG/backend/internal/characters/charactersdb"
	"github.com/PuraFome/meuRPG/backend/internal/platform/rpcerr"
)

// errCharacterNotFound is the answer for a character that is not in the
// campaign, or that the caller may not see. Its text is the same in both
// cases (ADR-0011).
func errCharacterNotFound() error {
	return connect.NewError(connect.CodeNotFound, errors.New("character not found"))
}

// errStaleRevision is the answer when the character changed since the
// client read it (AIP-154).
func errStaleRevision() error {
	return connect.NewError(connect.CodeAborted, errors.New("the character changed since you opened it; reload it and try again"))
}

// errBlocked is CharacterService's failed_precondition, with the
// CharacterBlocked detail that tells the app why.
func errBlocked(reason charactersv1.CharacterBlockedReason, characterID string) error {
	msg := map[charactersv1.CharacterBlockedReason]string{
		charactersv1.CharacterBlockedReason_CHARACTER_BLOCKED_REASON_SHEET_LOCKED:            "the sheet is locked: a game session has started, so only the master edits it",
		charactersv1.CharacterBlockedReason_CHARACTER_BLOCKED_REASON_CHARACTER_DEAD:          "the character is dead, so only the master edits its sheet",
		charactersv1.CharacterBlockedReason_CHARACTER_BLOCKED_REASON_LIVING_CHARACTER_EXISTS: "you already have a living character in this campaign",
		charactersv1.CharacterBlockedReason_CHARACTER_BLOCKED_REASON_STORY_LOCKED:            "the story is locked: ask the master to allow editing it",
		charactersv1.CharacterBlockedReason_CHARACTER_BLOCKED_REASON_NOT_PENDING:             "the character does not wait for approval: once approved, it stays in the campaign",
		charactersv1.CharacterBlockedReason_CHARACTER_BLOCKED_REASON_AWAITING_APPROVAL:       "the character waits for your approval: approve or reject it instead",
		charactersv1.CharacterBlockedReason_CHARACTER_BLOCKED_REASON_HIT_POINTS_AVERAGE_ONLY: "the table gives everybody the average hit points: there is no roll",
	}[reason]
	err := connect.NewError(connect.CodeFailedPrecondition, errors.New(msg))
	if detail, detailErr := connect.NewErrorDetail(&charactersv1.CharacterBlocked{Reason: reason, CharacterId: characterID}); detailErr == nil {
		err.AddDetail(detail)
	}
	return err
}

// errBlockedByContent is errBlocked for a table entry that stops the call
// (ARCHIVED_CONTENT, TABLE_CONTENT_STAYS): the detail names the key.
func errBlockedByContent(reason charactersv1.CharacterBlockedReason, characterID, contentKey string) error {
	msg := map[charactersv1.CharacterBlockedReason]string{
		charactersv1.CharacterBlockedReason_CHARACTER_BLOCKED_REASON_ARCHIVED_CONTENT:     "the master archived a table entry this sheet picks: it is not a new choice any more",
		charactersv1.CharacterBlockedReason_CHARACTER_BLOCKED_REASON_SWITCHED_OFF_CONTENT: "the master switched off an option this sheet picks: it is not a new choice for players",
		charactersv1.CharacterBlockedReason_CHARACTER_BLOCKED_REASON_TABLE_CONTENT_STAYS:  "the sheet uses the table's own content, so it cannot go to another campaign",
	}[reason]
	err := connect.NewError(connect.CodeFailedPrecondition, errors.New(msg))
	if detail, detailErr := connect.NewErrorDetail(&charactersv1.CharacterBlocked{Reason: reason, CharacterId: characterID, ContentKey: contentKey}); detailErr == nil {
		err.AddDetail(detail)
	}
	return err
}

// errPermission is the answer for a member whose role may not do this.
func errPermission(msg string) error {
	return connect.NewError(connect.CodePermissionDenied, errors.New(msg))
}

// invalidArgument turns a fieldError into the error the client gets. The
// message names the field and the rule, never the value, which could be
// personal data.
func invalidArgument(err error) error {
	if fe, ok := errors.AsType[*fieldError](err); ok {
		out := connect.NewError(connect.CodeInvalidArgument, fe)
		if detail, detailErr := connect.NewErrorDetail(&charactersv1.InvalidField{Field: fe.field}); detailErr == nil {
			out.AddDetail(detail)
		}
		return out
	}
	return err
}

// dbError maps an error from the database to the Connect error the client gets
// (see platform/rpcerr). Two cases are the characters module's own: a sheet
// deleted after its checks passed, and a stored document that cannot be read.
func (s *Service) dbError(ctx context.Context, action string, err error) error {
	if errors.Is(err, pgx.ErrNoRows) {
		// The character passed its checks and was deleted before the next
		// query: it is gone now.
		return errCharacterNotFound()
	}
	if errors.Is(err, errCorruptDocument) {
		s.logger.ErrorContext(ctx, "characters: cannot "+action, "error", err) //nolint:sloglint // each call site passes a fixed action, so the message is fixed per site
		return connect.NewError(connect.CodeInternal, errors.New("this character cannot be read right now"))
	}
	return rpcerr.FromDB(ctx, s.logger, "characters", action, err)
}

// wrap adds what failed to an error from a query, keeping it unwrappable
// (%w), so db.InTx still sees a 40001 and retries.
func wrap(what string, err error) error {
	return fmt.Errorf("%s: %w", what, err)
}

// DefaultMaxCharactersPerCampaign is the most characters and NPCs, of every
// kind and living or dead, one campaign may hold (RN-30). A table has a
// handful of players and a few hundred NPCs at the most; the cap stops one
// campaign from growing without end, and every list of the campaign with it.
const DefaultMaxCharactersPerCampaign = 1000

// errCharacterCapReached is the refusal of every create when the campaign
// already holds its most characters.
func errCharacterCapReached(limit int) error {
	return connect.NewError(connect.CodeResourceExhausted,
		fmt.Errorf("the campaign already has %d characters and NPCs, the most it may have; delete one to create another", limit))
}

// DefaultMaxCombatOnlyNpcsPerCampaign is the most NPCs the app may make for the creatures of the
// campaign's combats (RN-30). They do not count toward the characters' cap, so they have their own:
// a table creature edited many times makes one for each set of numbers, and each is loaded with the
// campaign's sheets.
const DefaultMaxCombatOnlyNpcsPerCampaign = 1000

// errCombatOnlyNpcCapReached is the refusal of a combat that would make one more NPC for a creature
// when the campaign already holds its most.
func errCombatOnlyNpcCapReached(limit int) error {
	return connect.NewError(connect.CodeResourceExhausted,
		fmt.Errorf("the campaign already has %d NPCs made for the creatures of its combats, the most it may have; edit or use a creature that already has one", limit))
}

// checkRoom refuses a create when the campaign has no room for one more
// character. It counts inside the create's transaction, which is SERIALIZABLE:
// two creates racing for the last place conflict, and the one that retries
// counts again. inserted says the new row is already in the count (the NPC
// creates insert first, so a replay of an old key, which inserts nothing, is
// answered even in a full campaign).
func (s *Service) checkRoom(ctx context.Context, q *charactersdb.Queries, campaignID string, inserted bool) error {
	n, err := q.CountCampaignCharacters(ctx, campaignID)
	if err != nil {
		return wrap("count the campaign's characters", err)
	}
	if inserted {
		n--
	}
	if int(n) >= s.maxCharacters {
		return errCharacterCapReached(s.maxCharacters)
	}
	return nil
}
