package characters

import (
	"testing"

	"connectrpc.com/connect"
	"github.com/jackc/pgx/v5"

	charactersv1 "github.com/PuraFome/meuRPG/backend/gen/meurpg/characters/v1"
	"github.com/PuraFome/meuRPG/backend/internal/characters/charactersdb"
)

const potionKey = "item:potion-of-healing-common"

// useUp removes the entry of a sheet in a transaction of its own and returns the entry it took.
func (et *editorTable) useUp(t *testing.T, characterID, entryID string) (*charactersv1.SheetMagicItem, error) {
	t.Helper()
	var removed *charactersv1.SheetMagicItem
	err := et.inTx(t, func(tx pgx.Tx) error {
		var err error
		removed, err = et.h.svc.RemoveConsumable(t.Context(), tx, et.campaign, characterID, entryID)
		return err
	})
	return removed, err
}

func sheetItemKeys(c *charactersv1.Character) []string {
	var out []string
	for _, it := range c.GetSheet().GetFull().GetMagicItems() {
		out = append(out, it.GetKey())
	}
	return out
}

// Using up a potion takes one entry and only that one, even beside another entry of the same potion, returns its key and
// name, drops the entry's stored state and bumps the sheet's revision so an open sheet is told it changed.
func TestRemoveConsumableTakesOneEntry(t *testing.T) {
	t.Parallel()
	et, pc := attuneTable(t, potionKey, wandKey, potionKey)
	var potions []*charactersv1.SheetMagicItem
	for _, it := range pc.GetSheet().GetFull().GetMagicItems() {
		if it.GetKey() == potionKey {
			potions = append(potions, it)
		}
	}
	if len(potions) != 2 || potions[0].GetId() == potions[1].GetId() {
		t.Fatalf("the sheet has %v", sheetItemKeys(pc))
	}
	if err := et.h.svc.queries.SetItemCharges(t.Context(), charactersdb.SetItemChargesParams{
		CharacterID: pc.GetId(), EntryID: potions[0].GetId(), ChargesUsed: 1, Now: et.h.clock.Now(),
	}); err != nil {
		t.Fatalf("a stored state for the entry: %v", err)
	}
	removed, err := et.useUp(t, pc.GetId(), potions[0].GetId())
	if err != nil {
		t.Fatalf("RemoveConsumable: %v", err)
	}
	if removed.GetKey() != potionKey || removed.GetId() != potions[0].GetId() || removed.GetNamePt() == "" {
		t.Errorf("removed %+v", removed)
	}
	after := et.read(t, et.master, pc.GetId())
	if got := sheetItemKeys(after); len(got) != 2 || got[0] != wandKey || got[1] != potionKey {
		t.Errorf("the sheet holds %v, want the wand and one potion", got)
	}
	if after.GetSheet().GetFull().GetMagicItems()[1].GetId() != potions[1].GetId() {
		t.Errorf("the other potion lost its id")
	}
	if after.GetRevision() != pc.GetRevision()+1 {
		t.Errorf("revision %d, was %d: the sheet did not change for open editors", after.GetRevision(), pc.GetRevision())
	}
	rows, err := et.h.svc.queries.ListItemCharges(t.Context(), pc.GetId())
	if err != nil || len(rows) != 0 {
		t.Errorf("the entry's stored state stayed: %v %v", rows, err)
	}
}

// The use works on a locked sheet (a game session is open) and refuses what is no consumable of the sheet.
func TestRemoveConsumableRefuses(t *testing.T) {
	t.Parallel()
	et, pc := attuneTable(t, potionKey, wandKey)
	et.h.lockSheets(et.campaign)
	wand := magicItemOf(pc, wandKey).GetId()
	potion := magicItemOf(pc, potionKey).GetId()
	for name, c := range map[string]struct {
		character, entry string
		code             connect.Code
	}{
		"a wand, which is no consumable": {pc.GetId(), wand, connect.CodeInvalidArgument},
		"an entry that is not there":     {pc.GetId(), "5b2f5d1c-0000-4000-8000-000000000000", connect.CodeNotFound},
		"a character that is not there":  {"5b2f5d1c-0000-4000-8000-000000000000", potion, connect.CodeNotFound},
		"a bad character id":             {"not-an-id", potion, connect.CodeNotFound},
	} {
		if _, err := et.useUp(t, c.character, c.entry); connect.CodeOf(err) != c.code {
			t.Errorf("%s: want %v, got %v", name, c.code, err)
		}
	}
	if got := sheetItemKeys(et.read(t, et.master, pc.GetId())); len(got) != 2 {
		t.Errorf("a refusal wrote: %v", got)
	}
	if _, err := et.useUp(t, pc.GetId(), potion); err != nil {
		t.Fatalf("on a locked sheet: %v", err)
	}
	if _, err := et.useUp(t, pc.GetId(), potion); connect.CodeOf(err) != connect.CodeNotFound {
		t.Errorf("the same unit twice: %v", err)
	}
}

// Reading a consumable entry returns its key and name and changes nothing: no revision, no entry taken, and a refusal for what is
// no consumable of the sheet.
func TestConsumableItemReadsTheEntryAndWritesNothing(t *testing.T) {
	t.Parallel()
	et, pc := attuneTable(t, potionKey, wandKey)
	potion, wand := magicItemOf(pc, potionKey).GetId(), magicItemOf(pc, wandKey).GetId()
	read := func(character, entry string) (item *charactersv1.SheetMagicItem, err error) {
		err = et.inTx(t, func(tx pgx.Tx) error {
			item, err = et.h.svc.ConsumableItem(t.Context(), tx, et.campaign, character, entry)
			return err
		})
		return item, err
	}
	item, err := read(pc.GetId(), potion)
	if err != nil || item.GetKey() != potionKey || item.GetId() != potion || item.GetNamePt() == "" {
		t.Fatalf("ConsumableItem = %+v, %v; want the potion with its name", item, err)
	}
	for name, c := range map[string]struct {
		character, entry string
		code             connect.Code
	}{
		"a wand, which is no consumable": {pc.GetId(), wand, connect.CodeInvalidArgument},
		"an entry that is not there":     {pc.GetId(), "5b2f5d1c-0000-4000-8000-000000000000", connect.CodeNotFound},
		"a character that is not there":  {"5b2f5d1c-0000-4000-8000-000000000000", potion, connect.CodeNotFound},
	} {
		if _, err := read(c.character, c.entry); connect.CodeOf(err) != c.code {
			t.Errorf("%s: want %v, got %v", name, c.code, err)
		}
	}
	after := et.read(t, et.master, pc.GetId())
	if got := sheetItemKeys(after); len(got) != 2 || after.GetRevision() != pc.GetRevision() {
		t.Errorf("the reads wrote: %v at revision %d, was %d", got, after.GetRevision(), pc.GetRevision())
	}
}
