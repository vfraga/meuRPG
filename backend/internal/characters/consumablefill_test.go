package characters

import (
	"slices"
	"testing"

	"connectrpc.com/connect"
	"github.com/jackc/pgx/v5"
	"google.golang.org/protobuf/proto"

	charactersv1 "github.com/PuraFome/meuRPG/backend/gen/meurpg/characters/v1"
	"github.com/PuraFome/meuRPG/backend/internal/rules"
)

// The sheet says what a use of each entry is and costs, and says nothing for an item that is no consumable.
func TestSheetReadsWhatTheUseOfAConsumableIsAndCosts(t *testing.T) {
	t.Parallel()
	et, pc := attuneTable(t, potionKey, "item:potion-of-resistance-fire", "item:potion-of-climbing", "item:oil-of-sharpness", "item:spell-scroll-1st", "item:feather-token-bird", bagKey, wandKey)
	sheet := et.read(t, et.master, pc.GetId())
	for key, want := range map[string]*charactersv1.SheetConsumable{
		potionKey:                        {Kind: charactersv1.ConsumableKind_CONSUMABLE_KIND_POTION, CostsAction: true, HealDiceCount: 2, HealDiceSides: 4, HealBonus: 2},
		"item:potion-of-resistance-fire": {Kind: charactersv1.ConsumableKind_CONSUMABLE_KIND_POTION, CostsAction: true, EffectTitlePt: "Resistência a fogo", EffectDurationTextPt: "por 1 hora"},
		"item:potion-of-climbing":        {Kind: charactersv1.ConsumableKind_CONSUMABLE_KIND_POTION, CostsAction: true},
		"item:oil-of-sharpness":          {Kind: charactersv1.ConsumableKind_CONSUMABLE_KIND_OIL},
		"item:spell-scroll-1st":          {Kind: charactersv1.ConsumableKind_CONSUMABLE_KIND_SCROLL},
		"item:feather-token-bird":        {Kind: charactersv1.ConsumableKind_CONSUMABLE_KIND_OTHER},
		bagKey:                           nil,
		wandKey:                          nil,
	} {
		got := magicItemOf(sheet, key).GetConsumable()
		if (want == nil) != (got == nil) || (want != nil && !proto.Equal(got, want)) {
			t.Errorf("%s: consumable = %v, want %v", key, got, want)
		}
	}
}

// The sheet's field and the use of a consumable decide from the same function: for every consumable of the catalog an entry the
// use accepts reads a consumable, and every other item reads none.
func TestSheetConsumableAgreesWithTheUseForEveryItemOfTheCatalog(t *testing.T) {
	t.Parallel()
	srd, err := rules.LoadSRD()
	if err != nil {
		t.Fatal(err)
	}
	var keys []string
	for _, e := range srd.MagicItems() {
		if !e.IsFamily() && (e.Consumable || e.Category == "wondrous-item") {
			keys = append(keys, e.Key)
		}
	}
	if len(keys) < 100 {
		t.Fatalf("only %d items to compare", len(keys))
	}
	et, pc := attuneTable(t)
	for chunk := range slices.Chunk(keys, 20) { // a sheet holds 20 entries
		req := editorSaveOf(pc)
		req.Sheet = editorClone(pc.GetSheet(), func(f *charactersv1.FullSheet) {
			f.MagicItems = nil
			for _, k := range chunk {
				f.MagicItems = append(f.MagicItems, &charactersv1.SheetMagicItem{Key: k})
			}
		})
		if pc, err = editorUpdate(et.master, req); err != nil {
			t.Fatalf("the master puts %v on the sheet: %v", chunk, err)
		}
		for _, it := range et.read(t, et.master, pc.GetId()).GetSheet().GetFull().GetMagicItems() {
			err := et.inTx(t, func(tx pgx.Tx) error {
				_, err := et.h.svc.ConsumableItem(t.Context(), tx, et.campaign, pc.GetId(), it.GetId())
				return err
			})
			if err != nil && connect.CodeOf(err) != connect.CodeInvalidArgument {
				t.Fatalf("%s: %v", it.GetKey(), err)
			}
			accepted := err == nil
			c, ok := srd.ConsumableOf(it.GetKey())
			if accepted != (it.GetConsumable() != nil) || ok != accepted || (ok && c.CostsAction != it.GetConsumable().GetCostsAction()) {
				t.Errorf("%s: the use accepts it = %v, rules say %+v/%v, the sheet reads %v", it.GetKey(), accepted, c, ok, it.GetConsumable())
			}
		}
	}
}

// A save carrying a consumable stores none: the stored sheet holds no field the server says, and the next read says it afresh.
func TestSaveStoresNoConsumableTheClientSends(t *testing.T) {
	t.Parallel()
	et, pc := attuneTable(t, bagKey, potionKey)
	req := editorSaveOf(pc)
	req.Sheet = editorClone(pc.GetSheet(), func(f *charactersv1.FullSheet) {
		forged := &charactersv1.SheetConsumable{Kind: charactersv1.ConsumableKind_CONSUMABLE_KIND_SCROLL, CostsAction: true, HealDiceCount: 99, HealDiceSides: 99, HealBonus: 99}
		for _, it := range f.GetMagicItems() {
			it.Consumable = forged
		}
	})
	saved, err := editorUpdate(et.master, req)
	if err != nil {
		t.Fatalf("save: %v", err)
	}
	if got := magicItemOf(saved, bagKey).GetConsumable(); got != nil {
		t.Errorf("a bag reads %v after a forged save", got)
	}
	if got := magicItemOf(saved, potionKey).GetConsumable(); !proto.Equal(got, &charactersv1.SheetConsumable{Kind: charactersv1.ConsumableKind_CONSUMABLE_KIND_POTION, CostsAction: true, HealDiceCount: 2, HealDiceSides: 4, HealBonus: 2}) {
		t.Errorf("the potion reads %v after a forged save", got)
	}
	var stored *charactersv1.CharacterSheet
	err = et.inTx(t, func(tx pgx.Tx) error {
		row, err := et.h.svc.characterOfItem(t.Context(), et.h.svc.queries.WithTx(tx), et.campaign, pc.GetId(), false)
		if err != nil {
			return err
		}
		stored, err = loadSheet(row.ID, row.Sheet)
		return err
	})
	if err != nil {
		t.Fatal(err)
	}
	for _, it := range stored.GetFull().GetMagicItems() {
		if it.GetConsumable() != nil {
			t.Errorf("%s is stored with %v", it.GetKey(), it.GetConsumable())
		}
	}
}
