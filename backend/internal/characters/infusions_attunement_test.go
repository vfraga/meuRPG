package characters

import (
	"testing"

	charactersv1 "github.com/PuraFome/meuRPG/backend/gen/meurpg/characters/v1"
	"github.com/PuraFome/meuRPG/backend/internal/characters/charactersdb"
	"github.com/PuraFome/meuRPG/backend/internal/rules"
)

// The ash cloak asks attunement: its record is an entry of the character named "infusion:<record id>".
func (it *infusedTable) infuseCloak(t *testing.T) (charactersdb.CharacterInfusion, string) {
	t.Helper()
	rec, err := it.h.svc.insertInfusion(t.Context(), it.h.svc.queries, newInfusion{
		CampaignID: it.campaign, ArtificerID: it.pc.GetId(), HolderID: it.pc.GetId(),
		InfusionKey: infusionKeyAsh, ItemKind: infusionItemGear, GearKey: infusionLeather, Parameter: "damage-type:fire",
	})
	if err != nil {
		t.Fatalf("insertInfusion(the cloak) error = %v", err)
	}
	return rec, "infusion:" + rec.ID
}

// withMagicItems saves the master's edit that puts catalog magic items on the sheet.
func (it *infusedTable) withMagicItems(t *testing.T, keys ...string) *charactersv1.Character {
	t.Helper()
	req := editorSaveOf(it.read(t))
	for _, k := range keys {
		req.Sheet.GetFull().MagicItems = append(req.Sheet.GetFull().MagicItems, &charactersv1.SheetMagicItem{Key: k})
	}
	saved, err := editorUpdate(it.master, req)
	if err != nil {
		t.Fatalf("the master puts the items on the sheet: %v", err)
	}
	return saved
}

func infusionAttuned(c *charactersv1.Character, id string) bool {
	for _, in := range c.GetSheet().GetFull().GetInfusions() {
		if in.GetId() == id {
			return in.GetAttuned()
		}
	}
	return false
}

func (it *infusedTable) record(t *testing.T, id string) charactersdb.CharacterInfusion {
	t.Helper()
	rec, err := it.h.svc.queries.GetCharacterInfusionForUpdate(t.Context(), charactersdb.GetCharacterInfusionForUpdateParams{CampaignID: it.campaign, ID: id})
	if err != nil {
		t.Fatalf("read the record %s: %v", id, err)
	}
	return rec
}

// The owner attunes an infused item inside the window a rest opens: the record says so, the reply lists it attuned, the
// window is spent and the owner puts the attunement off again with a window of the next rest.
func TestSetAttunementTakesTheItemOfAnInfusion(t *testing.T) {
	t.Parallel()
	it := newInfusedTable(t)
	rec, entry := it.infuseCloak(t)
	pc := it.read(t)
	if infusionAttuned(pc, rec.ID) {
		t.Fatal("a new record is attuned")
	}
	_, err := it.attuneEntry(it.dona, pc, entry, true)
	wantEditorRefusal(t, "outside the window", err, needsRest+pc.GetId()+`"}`)
	if it.record(t, rec.ID).Attuned {
		t.Error("a refused attunement changed the record")
	}

	it.openWindow(t, pc.GetId(), true)
	got, err := it.attuneEntry(it.dona, pc, entry, true)
	if err != nil {
		t.Fatalf("the owner attunes the cloak in the window: %v", err)
	}
	if !infusionAttuned(got, rec.ID) || !it.record(t, rec.ID).Attuned || got.GetRevision() <= pc.GetRevision() {
		t.Errorf("the reply lists the cloak attuned = %v, the record = %v, revision %d after %d; want attuned both places and a new revision",
			infusionAttuned(got, rec.ID), it.record(t, rec.ID).Attuned, got.GetRevision(), pc.GetRevision())
	}
	_, err = it.attuneEntry(it.dona, got, entry, false)
	wantEditorRefusal(t, "the window was spent", err, needsRest+pc.GetId()+`"}`)

	it.openWindow(t, pc.GetId(), true)
	off, err := it.attuneEntry(it.dona, got, entry, false)
	if err != nil || infusionAttuned(off, rec.ID) || it.record(t, rec.ID).Attuned {
		t.Errorf("putting the attunement off: err %v, listed attuned %v, record attuned %v", err, infusionAttuned(off, rec.ID), it.record(t, rec.ID).Attuned)
	}
	// Already as asked: nothing changes and no window is spent.
	it.openWindow(t, pc.GetId(), true)
	if _, err := it.attuneEntry(it.dona, off, entry, false); err != nil {
		t.Errorf("asking for the state it has: %v", err)
	}
	if open, _ := it.h.svc.queries.GetAttunementOpen(t.Context(), pc.GetId()); !open {
		t.Error("asking for the state it already has spent the window")
	}
}

// The count of three holds the infused item with the others: two magic items and a cloak make three, a third magic item is
// refused, and with three magic items attuned the cloak is refused. Putting the cloak off frees its place.
func TestSetAttunementCountsTheInfusedItemTowardThree(t *testing.T) {
	t.Parallel()
	it := newInfusedTable(t)
	_, entry := it.infuseCloak(t)
	pc := it.withMagicItems(t, ringA, ringB, ringC)
	var err error
	for _, k := range []string{ringA, ringB} {
		if pc, err = it.attune(it.master, pc, k, true); err != nil {
			t.Fatalf("attune %s: %v", k, err)
		}
	}
	if pc, err = it.attuneEntry(it.master, pc, entry, true); err != nil {
		t.Fatalf("the cloak is the third: %v", err)
	}
	it.openWindow(t, pc.GetId(), true)
	_, err = it.attune(it.dona, pc, ringC, true)
	wantEditorRefusal(t, "a fourth with the cloak counted", err, limit+pc.GetId()+`"}`)
	_, err = it.attune(it.master, pc, ringC, true)
	wantEditorRefusal(t, "the master's fourth", err, limit+pc.GetId()+`"}`)
	if pc, err = it.attuneEntry(it.dona, pc, entry, false); err != nil {
		t.Fatalf("putting the cloak off: %v", err)
	}
	it.openWindow(t, pc.GetId(), true)
	if pc, err = it.attune(it.dona, pc, ringC, true); err != nil {
		t.Errorf("the third magic item after the cloak was put off: %v", err)
	}
	// With three magic items attuned, the cloak is the fourth.
	_, err = it.attuneEntry(it.master, pc, entry, true)
	wantEditorRefusal(t, "the cloak as the fourth", err, limit+pc.GetId()+`"}`)
}

// An entry that is no entry of this character is the refusal of an unknown entry: a record whose infusion asks no
// attunement, an ended record, a record of another holder and an id that names no record.
func TestSetAttunementRefusesWhatIsNoEntryOfTheCharacter(t *testing.T) {
	t.Parallel()
	it := newInfusedTable(t)
	blade := it.infuse(t, infusionKeyBlade, infusionMace)
	cloak, cloakEntry := it.infuseCloak(t)
	pc := it.read(t)
	friend := it.caio.create(t, it.campaign, charactersv1.CharacterKind_CHARACTER_KIND_PLAYER, "Amigo", pensantusSheet())
	given, err := it.h.svc.insertInfusion(t.Context(), it.h.svc.queries, newInfusion{
		CampaignID: it.campaign, ArtificerID: it.pc.GetId(), HolderID: friend.GetId(),
		InfusionKey: infusionKeyAsh, ItemKind: infusionItemGear, GearKey: "equipment:quarterstaff", Parameter: "damage-type:fire",
	})
	if err != nil {
		t.Fatal(err)
	}
	const unknown = `invalid_argument: entry_id is not an entry of the sheet | InvalidField{field="entry_id"}`
	for name, entry := range map[string]string{
		"an infusion that asks no attunement": "infusion:" + blade.ID,
		"a record of another holder":          "infusion:" + given.ID,
		"an id that names no record":          "infusion:00000000-0000-4000-8000-0000000000ff",
		"the id without its prefix":           cloak.ID,
	} {
		_, err := it.attuneEntry(it.master, pc, entry, true)
		wantEditorRefusal(t, name, err, unknown)
	}
	// The holder reaches no entry of the artificer's record in this version.
	friendNow := it.caio.get(t, it.campaign, friend.GetId())
	// The artificer is another character, so the holder's sheet does not judge the record: no entry to attune.
	_, err = it.attuneEntry(it.master, friendNow, "infusion:"+given.ID, true)
	wantEditorRefusal(t, "a record whose artificer is another character", err, unknown)
	_, err = it.attuneEntry(it.master, friendNow, cloakEntry, true)
	wantEditorRefusal(t, "the artificer's record on another sheet", err, unknown)

	_, ended, err := it.h.svc.endInfusion(t.Context(), it.h.svc.queries, it.campaign, cloak.ID, infusionEndedEnded)
	if err != nil || !ended {
		t.Fatalf("endInfusion() = %v, %v", ended, err)
	}
	_, err = it.attuneEntry(it.master, pc, cloakEntry, true)
	wantEditorRefusal(t, "an ended record", err, unknown)
}

// A record that is not valid now is no entry to attune: the piece it sits on is not on the sheet (the record stays live
// until a write ends it), so the refusal is the one of an unknown entry, and the record is left as it was.
func TestSetAttunementRefusesARecordThatIsNotValidNow(t *testing.T) {
	t.Parallel()
	it := newInfusedTable(t)
	rec, err := it.h.svc.insertInfusion(t.Context(), it.h.svc.queries, newInfusion{
		CampaignID: it.campaign, ArtificerID: it.pc.GetId(), HolderID: it.pc.GetId(),
		InfusionKey: infusionKeyAsh, ItemKind: infusionItemGear, GearKey: "equipment:chain-mail", Parameter: "damage-type:fire",
	})
	if err != nil {
		t.Fatal(err)
	}
	pc := it.read(t)
	it.openWindow(t, pc.GetId(), true)
	_, err = it.attuneEntry(it.dona, pc, "infusion:"+rec.ID, true)
	wantEditorRefusal(t, "a record on a piece the sheet does not list", err, `invalid_argument: entry_id is not an entry of the sheet | InvalidField{field="entry_id"}`)
	if it.record(t, rec.ID).Attuned {
		t.Error("a refused attunement changed the record")
	}
}

// A record whose artificer is another character is not valid on the sheet of a holder who has the host feature: the
// holder's features do not judge it, so the blade gives no bonus.
func TestTheSheetOfAHolderDoesNotJudgeAnotherArtificersInfusionRecord(t *testing.T) {
	t.Parallel()
	it := newInfusedTable(t)
	friend := it.caio.create(t, it.campaign, charactersv1.CharacterKind_CHARACTER_KIND_PLAYER, "Amigo", pensantusSheet())
	if _, err := it.h.svc.insertInfusion(t.Context(), it.h.svc.queries, newInfusion{
		CampaignID: it.campaign, ArtificerID: friend.GetId(), HolderID: it.pc.GetId(),
		InfusionKey: infusionKeyBlade, ItemKind: infusionItemGear, GearKey: infusionMace,
	}); err != nil {
		t.Fatal(err)
	}
	c := it.read(t)
	list := c.GetSheet().GetFull().GetInfusions()
	if len(list) != 1 || list[0].GetValidNow() || list[0].GetBonus() != 0 {
		t.Errorf("the holder's sheet lists %v, want one record, not valid now and no bonus", list)
	}
	if a := attackOf(c, infusionMace); a == nil || a.GetMagicBonus() != 0 {
		t.Errorf("the mace attack = %v, want no infusion bonus", a)
	}
}

// entriesOf with records and no content lists the sheet's own entries and no infusion: it has no way to say which ask.
func TestAttunementEntriesOfRecordsAndNoContentDoNotPanic(t *testing.T) {
	t.Parallel()
	records := []rules.InfusionRecord{{ID: "r1", InfusionKey: infusionKeyAsh}}
	f := &charactersv1.FullSheet{MagicItems: []*charactersv1.SheetMagicItem{{Key: "x"}}}
	if got := entriesOf(f, records, nil); len(got) != 1 {
		t.Errorf("entriesOf(records, nil content) = %d entries, want the 1 magic item", len(got))
	}
}
