package characters

import (
	"context"
	"errors"
	"slices"

	"connectrpc.com/connect"
	"github.com/jackc/pgx/v5"
	"google.golang.org/protobuf/proto"

	charactersv1 "github.com/PuraFome/meuRPG/backend/gen/meurpg/characters/v1"
	"github.com/PuraFome/meuRPG/backend/internal/characters/charactersdb"
	"github.com/PuraFome/meuRPG/backend/internal/platform/uid"
)

// RemoveConsumable uses up one unit of a consumable: it takes the entry (one entry of the sheet's magic items is one
// unit) off the character's sheet and returns it, with the key and the name the line of the use shows (`name_pt`, the
// SRD's English name when the content has no Portuguese one). Like the charges' methods it runs in the caller's
// transaction, locks the character's row, works on a locked sheet and never asks who the caller is: play authorizes
// and checks the character is alive first. It deletes the entry's stored charge state and writes the sheet as a save
// does, so the revision goes up and an open sheet is told it changed. `not_found` for a character that is not in the
// campaign and an entry that is not on its sheet, `invalid_argument` for an entry whose item is not a consumable.
func (s *Service) RemoveConsumable(ctx context.Context, tx pgx.Tx, campaignID, characterID, entryID string) (*charactersv1.SheetMagicItem, error) {
	found, err := s.loadConsumable(ctx, tx, campaignID, characterID, entryID, true)
	if err != nil {
		return nil, err
	}
	q, row, full, at := found.q, found.row, found.full, found.at
	next := proto.CloneOf(full)
	next.MagicItems = slices.Delete(next.MagicItems, at, at+1)
	doc, err := storeJSON.Marshal(&charactersv1.CharacterSheet{Content: &charactersv1.CharacterSheet_Full{Full: next}})
	if err != nil {
		return nil, wrap("encode a sheet", err)
	}
	_, err = q.UpdateCharacterSheet(ctx, charactersdb.UpdateCharacterSheetParams{
		CampaignID: campaignID, ID: row.ID, Revision: row.Revision, Name: row.Name, Sheet: doc, Now: s.now(),
	})
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, errStaleRevision()
	}
	if err != nil {
		return nil, wrap("update sheet", err)
	}
	if err := dropRemovedItemCharges(ctx, q, row.ID, next); err != nil {
		return nil, err
	}
	return found.item, nil
}

// ConsumableItem reads a consumable entry of a character's sheet: the key and the name (see RemoveConsumable), and nothing is
// locked or written. `not_found` for a character that is not in the campaign and an entry that is not on its sheet,
// `invalid_argument` for an entry whose item is not a consumable.
func (s *Service) ConsumableItem(ctx context.Context, tx pgx.Tx, campaignID, characterID, entryID string) (*charactersv1.SheetMagicItem, error) {
	found, err := s.loadConsumable(ctx, tx, campaignID, characterID, entryID, false)
	if err != nil {
		return nil, err
	}
	return found.item, nil
}

// consumableEntry is a consumable on a character's sheet: the character's row, its full sheet, the entry's place in the sheet's
// magic items and the entry with the name the line of the use shows.
type consumableEntry struct {
	q    *charactersdb.Queries
	row  charactersdb.Character
	full *charactersv1.FullSheet
	at   int
	item *charactersv1.SheetMagicItem
}

// loadConsumable finds the entry on the sheet of the character (locked for a write), and checks that it is a consumable.
func (s *Service) loadConsumable(ctx context.Context, tx pgx.Tx, campaignID, characterID, entryID string, lock bool) (consumableEntry, error) {
	id, ok := uid.Canonical(characterID)
	if !ok {
		return consumableEntry{}, errCharacterNotFound()
	}
	q := s.queries.WithTx(tx)
	row, err := s.characterOfItem(ctx, q, campaignID, id, lock)
	if errors.Is(err, pgx.ErrNoRows) {
		return consumableEntry{}, errCharacterNotFound()
	}
	if err != nil {
		return consumableEntry{}, wrap("read the character", err)
	}
	sheet, err := loadSheet(row.ID, row.Sheet)
	if err != nil {
		return consumableEntry{}, err
	}
	full := sheet.GetFull()
	at := slices.IndexFunc(full.GetMagicItems(), func(it *charactersv1.SheetMagicItem) bool { return it.GetId() == entryID })
	if at < 0 {
		return consumableEntry{}, connect.NewError(connect.CodeNotFound, errors.New("the item is not on the sheet"))
	}
	item := proto.CloneOf(full.GetMagicItems()[at])
	content, err := s.contentFor(ctx, tx, campaignID)
	if err != nil {
		return consumableEntry{}, wrap("read rules content", err)
	}
	if def, ok := content.MagicItem(item.GetKey()); !ok || !def.Consumable {
		return consumableEntry{}, invalidArgument(fieldErr("entry_id", "is not a consumable"))
	}
	item.NamePt = content.MagicItemNamePT(item.GetKey())
	return consumableEntry{q: q, row: row, full: full, at: at, item: item}, nil
}
