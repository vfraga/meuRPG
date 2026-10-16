package characters

import (
	"context"
	"errors"

	"github.com/jackc/pgx/v5"

	charactersv1 "github.com/PuraFome/meuRPG/backend/gen/meurpg/characters/v1"
	"github.com/PuraFome/meuRPG/backend/internal/authz"
	"github.com/PuraFome/meuRPG/backend/internal/characters/charactersdb"
	"github.com/PuraFome/meuRPG/backend/internal/platform/db"
	"github.com/PuraFome/meuRPG/backend/internal/platform/names"
	"github.com/PuraFome/meuRPG/backend/internal/rules"
)

// The phases of UpdateCharacter: the request read and checked (readSheetSave), the
// portrait's copy prepared (preparePortraitSave), then the transaction that checks
// the stored character and writes the sheet (writeSheetSave), and what runs after
// its commit.

// sheetSave is an UpdateCharacter request on its way to the transaction. content and
// sheet become the ones the transaction reads when the table's content moved in
// between, and the response is built with that content.
type sheetSave struct {
	m        authz.Membership
	id       string
	revision int32
	name     string
	content  *rules.Content
	sheet    *charactersv1.CharacterSheet
	// portraitErr is the portrait's field problem, found before the transaction and
	// answered inside it after the checks that come first; portraitCopy is the copy of
	// a fog map's background whose gallery row the transaction inserts, and copyCreated
	// says the row is the one this call inserted.
	portraitErr  error
	portraitCopy PortraitCopy
	copyCreated  bool
	// afterArmor is what package play asks to run after the commit, nil for nothing.
	afterArmor func(ctx context.Context)
}

// readSheetSave reads and checks an UpdateCharacter request, in the order the client
// is answered: the character's ID, the revision, the name, then the sheet.
func (s *Service) readSheetSave(ctx context.Context, m authz.Membership, msg *charactersv1.UpdateCharacterRequest) (*sheetSave, error) {
	id, ok := parseUUID(msg.GetCharacterId())
	if !ok {
		return nil, errCharacterNotFound()
	}
	revision := msg.GetRevision()
	if revision < 1 {
		return nil, invalidArgument(fieldErr("revision", "must be at least 1"))
	}
	name, err := names.Clean(msg.GetName(), MaxNameLength)
	if err != nil {
		return nil, invalidArgument(&fieldError{field: "name", err: err})
	}
	content, err := s.contentFor(ctx, nil, m.CampaignID)
	if err != nil {
		return nil, s.dbError(ctx, "read rules content", err)
	}
	sheet, err := checkSheet(content, msg.GetSheet())
	if err != nil {
		return nil, invalidArgument(err)
	}
	return &sheetSave{m: m, id: id, revision: revision, name: name, content: content, sheet: sheet}, nil
}

// preparePortraitSave prepares the portrait before the transaction: a fog map's
// background is copied (its files now, its gallery row inside the transaction). It is
// prepared only for a caller the transaction would let save the sheet: the same checks,
// in the same order (visible, RN-01, kind), so nobody else makes the server copy files.
// A caller who fails one skips the preparation, and the transaction answers with that
// check's error. A character's kind never changes, so the row read here is the one the
// transaction reads. A read that fails is the request's failure: an unchecked portrait
// never goes through.
func (s *Service) preparePortraitSave(ctx context.Context, u *sheetSave) error {
	if portraitOf(u.sheet) == "" {
		return nil
	}
	pre, err := s.queries.GetCharacter(ctx, charactersdb.GetCharacterParams{CampaignID: u.m.CampaignID, ID: u.id})
	switch {
	case errors.Is(err, pgx.ErrNoRows):
		// No such character: the transaction says so.
	case err != nil:
		return s.dbError(ctx, "read a character to check its portrait", err)
	case canSee(u.m, pre.Kind, pre.Status, pre.PlayerUserID) && mayEditSheet(u.m, pre) && checkSheetKind(pre.Kind, u.sheet) == nil:
		var failure error
		u.portraitCopy, u.portraitErr, failure = s.preparePortrait(ctx, u.m.CampaignID, pre.Kind, u.sheet)
		return failure
	}
	return nil
}

// writeSheetSave runs the save's transaction, then what play asked for after the commit,
// and lets the portrait copy's files go when no gallery row of this call keeps them.
func (s *Service) writeSheetSave(ctx context.Context, u *sheetSave) (charactersdb.Character, error) {
	var row charactersdb.Character
	err := db.InTx(ctx, s.pool, func(tx pgx.Tx) error {
		u.afterArmor = nil
		var err error
		row, err = s.saveSheetIn(ctx, tx, u)
		return err
	})
	if u.afterArmor != nil && err == nil {
		u.afterArmor(ctx)
	}
	if u.portraitCopy != nil && (err != nil || !u.copyCreated) {
		u.portraitCopy.Discard(ctx) // no gallery row: the copy's files go
	}
	if err != nil {
		return row, s.dbError(ctx, "update a character", err)
	}
	return row, nil
}

// saveSheetIn is the save's transaction: the stored character locked and checked, the
// sheet checked against it, the portrait's gallery row, the write, and what play is told.
func (s *Service) saveSheetIn(ctx context.Context, tx pgx.Tx, u *sheetSave) (charactersdb.Character, error) {
	q := s.queries.WithTx(tx)
	current, err := lockSheetToSave(ctx, q, u)
	if err != nil {
		return charactersdb.Character{}, err
	}
	stored, err := s.checkSheetSaveIn(ctx, tx, u, current)
	if err != nil {
		return charactersdb.Character{}, err
	}
	u.copyCreated = false
	if err := u.insertPortraitCopy(ctx, tx); err != nil {
		return charactersdb.Character{}, err
	}
	// An NPC made from a creature keeps the link to it (MR-042): the
	// client never sends it, so it is carried over from the saved sheet.
	sheetDoc, err := storeJSON.Marshal(keepCreatureLink(current, u.sheet))
	if err != nil {
		return charactersdb.Character{}, wrap("encode a sheet", err)
	}
	row, err := q.UpdateCharacterSheet(ctx, charactersdb.UpdateCharacterSheetParams{
		CampaignID: u.m.CampaignID, ID: u.id, Revision: u.revision, Name: u.name, Sheet: sheetDoc, Now: s.now(),
	})
	if errors.Is(err, pgx.ErrNoRows) {
		return row, errStaleRevision()
	}
	if err != nil {
		return row, wrap("update sheet", err)
	}
	if err := s.carryHitPoints(ctx, q, u.content, u.id, current.Sheet, sheetDoc); err != nil {
		return row, err
	}
	u.afterArmor, err = s.tellGearChange(ctx, tx, u.m.CampaignID, u.id, stored.GetFull(), u.sheet.GetFull())
	return row, err
}

// lockSheetToSave reads the stored character, locked, and checks that the caller may
// save this sheet on it now: visible, RN-01, the sheet's kind, the portrait, the revision.
func lockSheetToSave(ctx context.Context, q *charactersdb.Queries, u *sheetSave) (charactersdb.Character, error) {
	current, err := visibleForUpdate(ctx, q, u.m, u.id)
	if err != nil {
		return current, err
	}
	// RN-01: after a game session starts, only the master edits the
	// sheet. This comes before the revision: retrying would not help.
	if !mayEditSheet(u.m, current) {
		return current, errBlocked(blockedReason(characterState(current.Status, current.SheetLockedAt)), current.ID)
	}
	if err := checkSheetKind(current.Kind, u.sheet); err != nil {
		return current, invalidArgument(err)
	}
	if u.portraitErr != nil {
		return current, invalidArgument(u.portraitErr)
	}
	if current.Revision != u.revision {
		return current, errStaleRevision()
	}
	return current, nil
}

// checkSheetSaveIn checks the sheet against the content the transaction reads and the
// stored sheet, which it returns.
func (s *Service) checkSheetSaveIn(ctx context.Context, tx pgx.Tx, u *sheetSave, current charactersdb.Character) (*charactersv1.CharacterSheet, error) {
	// The table's content, read in this transaction: if it moved since it was
	// read above (an archive, an edit), the sheet is checked again with it, so a
	// write is always ordered against a content write (ADR-0018, section 5).
	tc, err := s.contentFor(ctx, tx, u.m.CampaignID)
	if err != nil {
		return nil, wrap("read rules content", err)
	}
	if tc.TableRevision() != u.content.TableRevision() {
		if u.sheet, err = checkSheet(tc, u.sheet); err != nil {
			return nil, invalidArgument(err)
		}
	}
	u.content = tc
	stored, err := loadSheet(current.ID, current.Sheet)
	if err != nil {
		return nil, err
	}
	if err := refuseSheetChanges(u.content, u.m, current, stored, u.sheet); err != nil {
		return nil, err
	}
	return stored, nil
}

// refuseSheetChanges refuses what a save may not change from the stored sheet, in the
// order the client is answered, and carries over what the server keeps.
func refuseSheetChanges(content *rules.Content, m authz.Membership, current charactersdb.Character, stored, sheet *charactersv1.CharacterSheet) error {
	if err := refuseNewChoices(content, m, current.ID, stored.GetFull(), sheet.GetFull()); err != nil {
		return err
	}
	// A player's own save leaves no choice open (the master may).
	if err := refuseChoices(content, m, current.Kind, sheet.GetFull(), stored.GetFull()); err != nil {
		return err
	}
	if err := refuseMulticlassGap(content, m, stored.GetFull(), sheet.GetFull()); err != nil {
		return err
	}
	// Only the master adds or removes a feat in the editor (MR-025).
	if err := refuseFeatChange(m, stored.GetFull(), sheet.GetFull()); err != nil {
		return err
	}
	// The ability origin is the server's: kept from the stored sheet, and a
	// player's draft edit of the base scores is checked against it (RN-24).
	if err := keepAbilityOrigin(m, content, stored, sheet); err != nil {
		return err
	}
	// A save never clears an issue that a change of the table's content flagged
	// and that still stands.
	carryFlags(content, stored.GetFull(), sheet.GetFull())
	return nil
}

// insertPortraitCopy writes the portrait copy's gallery row in the transaction and puts
// the copy on the sheet.
func (u *sheetSave) insertPortraitCopy(ctx context.Context, tx pgx.Tx) error {
	if u.portraitCopy == nil {
		return nil
	}
	id, created, err := u.portraitCopy.Insert(ctx, tx)
	if err != nil {
		return err
	}
	u.copyCreated = created
	setPortrait(u.sheet, id)
	return nil
}

// tellGearChange tells package play, inside the transaction, what a save changed of the
// character's armor and shield, and returns what play asks to run after the commit.
func (s *Service) tellGearChange(ctx context.Context, tx pgx.Tx, campaignID, characterID string, stored, next *charactersv1.FullSheet) (func(ctx context.Context), error) {
	if s.reviewHost == nil {
		return nil, nil
	}
	before, after := stored.GetArmorKey(), next.GetArmorKey()
	switch {
	case after != "" && before != after:
		// A character that dons armor loses Mage Armor.
		return s.reviewHost.ArmorWorn(ctx, tx, campaignID, characterID)
	case before != after || stored.GetShield() != next.GetShield():
		// A shield (or armor taken off) changes what Mage Armor gives.
		return s.reviewHost.GearChanged(ctx, tx, campaignID, characterID)
	}
	return nil, nil
}
