package characters

import (
	"context"
	"fmt"

	"github.com/jackc/pgx/v5"

	charactersv1 "github.com/PuraFome/meuRPG/backend/gen/meurpg/characters/v1"
	"github.com/PuraFome/meuRPG/backend/internal/characters/charactersdb"
)

// The NPC's portrait (MR-031, D7, question 62): an image of the campaign's
// gallery, kept on the NPC's sheet as `portrait_image_id`. The sheet is JSON,
// so the field needs no migration.
//
//   - Only an NPC has one: a player's character with a portrait is an error.
//   - The image must be in the campaign's gallery, which package maps owns, so
//     the check goes through a small interface (Gallery), connected by
//     SetGallery. An image of another campaign, or one that does not exist,
//     is the same error: nobody learns that an image exists elsewhere.
//   - Deleting the gallery image clears the portrait (ClearPortraits, which
//     package maps calls inside the delete's transaction), where a map's
//     image refuses the delete: a portrait is a convenience that falls back to
//     the initials, a map without its image is not a map.
//   - A portrait set in the instant the image is deleted may outlive it: the
//     URL then answers 404, and the app draws the initials, as for any NPC with
//     none. The next save of the sheet is refused until the portrait is fixed.

// Gallery says which images are in a campaign's gallery. The maps module
// implements it (maps.SessionMaps), because the gallery is its own.
type Gallery interface {
	// PortraitImage reports whether imageID is an image of the campaign's gallery
	// and returns the image the portrait should be: the same one, or, when it is
	// the background of a map with the fog of war on (a player never gets such an
	// image, MR-036), a copy of it of its own.
	PortraitImage(ctx context.Context, campaignID, imageID string) (string, bool, error)
	// PreparePortrait is PortraitImage for a save that has a transaction: when the
	// portrait needs a copy, it copies the files now (before the transaction) and
	// hands back the copy, whose gallery row the caller adds inside its transaction
	// (Insert, with the quota check) or whose files it deletes (Discard) when the
	// save does not happen. The copy is nil when the image is used as it is.
	PreparePortrait(ctx context.Context, campaignID, imageID string) (use string, found bool, imageCopy PortraitCopy, err error)
}

// PortraitCopy is a copy of a gallery image made for a portrait, not yet in the
// gallery (see Gallery.PreparePortrait).
type PortraitCopy = interface {
	// Insert adds the copy to the gallery inside tx and returns the image the
	// portrait is: the copy, or the one another use of the same image committed first
	// (created false: the caller then discards these files, as when the save does
	// not happen).
	Insert(ctx context.Context, tx pgx.Tx) (id string, created bool, err error)
	// Discard deletes the copy's files: the save did not happen.
	Discard(ctx context.Context)
}

// SetGallery connects the gallery, which package maps owns (the same
// arrangement as SetLevelUps). Without it no portrait is accepted.
func (s *Service) SetGallery(g Gallery) { s.gallery = g }

// portraitOf is the portrait image ID on a sheet, "" for none.
func portraitOf(sheet *charactersv1.CharacterSheet) string {
	if sheet.GetFull() != nil {
		return sheet.GetFull().GetPortraitImageId()
	}
	return sheet.GetBasic().GetPortraitImageId()
}

// portraitToCheck returns the portrait image a sheet being written to a character
// of kind in the campaign carries, with the name of its field: "" when there is
// none. It returns a fieldError for a player's character with a portrait, and for
// an image that is no UUID.
func (s *Service) portraitToCheck(kind string, sheet *charactersv1.CharacterSheet) (id, field string, err error) {
	id = portraitOf(sheet)
	if id == "" {
		return "", "", nil
	}
	field = "sheet.basic.portrait_image_id"
	if sheet.GetFull() != nil {
		field = "sheet.full.portrait_image_id"
	}
	if kind == kindPlayer {
		return "", field, fieldErr(field, "must be empty for a player's character")
	}
	id, ok := parseUUID(id)
	if !ok {
		return "", field, fieldErr(field, "must be an image of the campaign's gallery")
	}
	if s.gallery == nil {
		return "", field, fieldErr(field, "cannot be set: the gallery is off")
	}
	return id, field, nil
}

// setPortrait puts the image the portrait should be on the sheet.
func setPortrait(sheet *charactersv1.CharacterSheet, id string) {
	if sheet.GetFull() != nil {
		sheet.GetFull().PortraitImageId = id
	} else {
		sheet.GetBasic().PortraitImageId = id
	}
}

// checkPortrait checks the portrait of a sheet being written to a character of
// kind in the campaign. It returns a fieldError, for the handler to turn into
// invalid_argument, or an error of the database. It may make a copy of the image
// in a transaction of its own (a fog map's background), so it never runs inside
// one: a transaction would hold a connection while it waits for another. A save
// that has a transaction uses preparePortrait instead.
func (s *Service) checkPortrait(ctx context.Context, campaignID, kind string, sheet *charactersv1.CharacterSheet) error {
	id, field, err := s.portraitToCheck(kind, sheet)
	if err != nil || id == "" {
		return err
	}
	use, found, err := s.gallery.PortraitImage(ctx, campaignID, id)
	if err != nil {
		return s.dbError(ctx, "check a portrait", err)
	}
	if !found {
		return fieldErr(field, "must be an image of the campaign's gallery")
	}
	if use != id { // a copy of a fog map's image: the portrait is the copy
		setPortrait(sheet, use)
	}
	return nil
}

// preparePortrait is checkPortrait for a save with a transaction: the copy of a
// fog map's image, when the portrait needs one, has its files made now and its
// gallery row left for the transaction (PortraitCopy.Insert), so a save that does
// not happen leaves no gallery image behind (Discard). A database error is
// returned as it is (the first result is a fieldError for the handler to turn into
// invalid_argument, the second a failure of the server).
func (s *Service) preparePortrait(ctx context.Context, campaignID, kind string, sheet *charactersv1.CharacterSheet) (imageCopy PortraitCopy, fieldProblem, failure error) {
	id, field, err := s.portraitToCheck(kind, sheet)
	if err != nil || id == "" {
		return nil, err, nil
	}
	use, found, imageCopy, err := s.gallery.PreparePortrait(ctx, campaignID, id)
	if err != nil {
		return nil, nil, s.dbError(ctx, "check a portrait", err)
	}
	if !found {
		return nil, fieldErr(field, "must be an image of the campaign's gallery"), nil
	}
	if use != id {
		setPortrait(sheet, use)
	}
	return imageCopy, nil, nil
}

// ClearPortraits takes the image off the portrait of every NPC of the campaign
// that has it, inside tx, and returns how many it cleared. Package maps calls it
// when the master deletes the image from the gallery. Each NPC's revision goes
// up, so an editor still holding the old sheet is told it is stale instead of
// saving the dead portrait again.
func (s *Service) ClearPortraits(ctx context.Context, tx pgx.Tx, campaignID, imageID string) (int64, error) {
	n, err := s.queries.WithTx(tx).ClearPortraits(ctx, charactersdb.ClearPortraitsParams{CampaignID: campaignID, ImageID: imageID, Now: s.now()})
	if err != nil {
		return 0, fmt.Errorf("clear the portraits of an image: %w", err)
	}
	return n, nil
}
