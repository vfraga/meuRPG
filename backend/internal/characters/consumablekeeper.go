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
	"github.com/PuraFome/meuRPG/backend/internal/play/link"
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
	if _, ok := content.ConsumableOf(item.GetKey()); !ok {
		return consumableEntry{}, invalidArgument(fieldErr("entry_id", "is not a consumable"))
	}
	item.NamePt = content.MagicItemNamePT(item.GetKey())
	return consumableEntry{q: q, row: row, full: full, at: at, item: item}, nil
}

// ScrollOf reads a Spell Scroll entry of a character's sheet as its reader reads it (the spell it gives and what reading it comes to
// for the character, by the rules: ReadScrollSpell), and locks the character's row until the transaction ends, so what the read
// decides on is what the unit's removal then finds. Like the keeper's other methods it asks nobody who the caller is. `not_found`
// for a character or an entry that is not there, `invalid_argument` for an entry that is no Spell Scroll.
func (s *Service) ScrollOf(ctx context.Context, tx pgx.Tx, campaignID, characterID, entryID string) (link.ScrollEntry, error) {
	found, err := s.loadConsumable(ctx, tx, campaignID, characterID, entryID, true)
	if err != nil {
		return link.ScrollEntry{}, err
	}
	content, err := s.contentFor(ctx, tx, campaignID)
	if err != nil {
		return link.ScrollEntry{}, wrap("read rules content", err)
	}
	item := found.item
	sp := content.ScrollSpellOf(item.GetKey(), item.GetSpellKey())
	if !sp.IsScroll {
		return link.ScrollEntry{}, invalidArgument(fieldErr("entry_id", "is not a Spell Scroll"))
	}
	out := link.ScrollEntry{EntryID: item.GetId(), ItemKey: item.GetKey(), ItemNamePT: item.GetNamePt(), NoSpell: !sp.Gives(), Off: sp.Off}
	if out.NoSpell {
		return out, nil
	}
	_, derived, _, err := s.fighter(ctx, tx, campaignID, characterID)
	if err != nil {
		return link.ScrollEntry{}, err
	}
	read, _ := content.ReadScrollSpell(derived, sp.Key)
	out.SpellKey, out.SpellNamePT, out.Level = sp.Key, content.NamePT(sp.Key), read.Level
	out.Reads, out.UnreadablePT, out.Economy = read.Reads, read.UnreadablePT, read.Economy
	out.CheckNeeded, out.CheckDC, out.Modifier = read.CheckNeeded, read.CheckDC, derived.AbilityModifier(read.Ability)
	return out, nil
}
