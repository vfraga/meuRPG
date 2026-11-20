package characters

import (
	"testing"

	charactersv1 "github.com/PuraFome/meuRPG/backend/gen/meurpg/characters/v1"
	"github.com/PuraFome/meuRPG/backend/internal/characters/charactersdb"
)

// endedWith is the reason a record ended with, "" while it is live.
func endedWith(rec charactersdb.CharacterInfusion) string {
	if rec.EndedAt == nil || rec.EndedReason == nil {
		return ""
	}
	return *rec.EndedReason
}

// A save that takes a piece off the sheet ends the live record on it with item_gone, in the same transaction: the armor,
// the shield and a weapon each. The records of the pieces that stay stay live.
func TestTheSaveEndsTheRecordOfAPieceThatLeftTheSheet(t *testing.T) {
	t.Parallel()
	it := newInfusedTable(t)
	blade := it.infuse(t, infusionKeyBlade, infusionMace)
	plate := it.infuse(t, infusionKeyPlate, infusionLeather)
	seal := it.infuse(t, infusionKeyFalse, infusionShieldGear)

	// A save that keeps the gear leaves every record as it is.
	pc, err := editorUpdate(it.dona, editorSaveOf(it.read(t)))
	if err != nil {
		t.Fatalf("a save that changes nothing: %v", err)
	}
	for _, rec := range []charactersdb.CharacterInfusion{blade, plate, seal} {
		if got := it.record(t, rec.ID); endedWith(got) != "" {
			t.Errorf("a save that kept the gear ended %s with %q", rec.ID, endedWith(got))
		}
	}

	// The weapon goes: only its record ends.
	req := editorSaveOf(pc)
	req.Sheet.GetFull().WeaponKeys = []string{"equipment:club"}
	saved, err := editorUpdate(it.dona, req)
	if err != nil {
		t.Fatalf("the save that trades the mace for a club: %v", err)
	}
	if got := endedWith(it.record(t, blade.ID)); got != infusionEndedItemGone {
		t.Errorf("the record of the mace ended with %q, want %q", got, infusionEndedItemGone)
	}
	if endedWith(it.record(t, plate.ID)) != "" || endedWith(it.record(t, seal.ID)) != "" {
		t.Error("the save ended a record of a piece that stayed")
	}
	if left := saved.GetSheet().GetFull().GetInfusions(); len(left) != 2 {
		t.Errorf("the reply lists %d infusions, want the 2 live ones", len(left))
	}

	// The armor trades for another and the shield goes.
	req = editorSaveOf(saved)
	req.Sheet.GetFull().ArmorKey, req.Sheet.GetFull().Shield = "equipment:studded-leather-armor", false
	if _, err := editorUpdate(it.dona, req); err != nil {
		t.Fatalf("the save that trades the armor and drops the shield: %v", err)
	}
	for name, rec := range map[string]charactersdb.CharacterInfusion{"the armor": plate, "the shield": seal} {
		if got := endedWith(it.record(t, rec.ID)); got != infusionEndedItemGone {
			t.Errorf("the record of %s ended with %q, want %q", name, got, infusionEndedItemGone)
		}
	}
}

// The record of a piece on another character's sheet ends when that character's save takes the piece off: the holder's
// write is the one that ends it, not the artificer's.
func TestTheHoldersSaveEndsTheRecordOfItsPiece(t *testing.T) {
	t.Parallel()
	it := newInfusedTable(t)
	friend := it.caio.create(t, it.campaign, charactersv1.CharacterKind_CHARACTER_KIND_PLAYER, "Amigo", pensantusSheet())
	given, err := it.h.svc.insertInfusion(t.Context(), it.h.svc.queries, newInfusion{
		CampaignID: it.campaign, ArtificerID: it.pc.GetId(), HolderID: friend.GetId(),
		InfusionKey: infusionKeyBlade, ItemKind: infusionItemGear, GearKey: "equipment:quarterstaff",
	})
	if err != nil {
		t.Fatal(err)
	}
	req := editorSaveOf(friend)
	req.Sheet.GetFull().WeaponKeys = []string{"equipment:dagger"}
	if _, err := editorUpdate(it.caio, req); err != nil {
		t.Fatalf("the friend trades the staff for a dagger: %v", err)
	}
	if got := endedWith(it.record(t, given.ID)); got != infusionEndedItemGone {
		t.Errorf("the record on the friend's staff ended with %q, want %q", got, infusionEndedItemGone)
	}
}

// A piece moved between equipped and carried stays on the sheet: its record stays live. The other writers of the sheet
// (the equip call, the attunement of an item) take no piece off, so they end nothing.
func TestAPieceMovedBetweenEquippedAndCarriedKeepsItsRecord(t *testing.T) {
	t.Parallel()
	it := newInfusedTable(t)
	blade := it.infuse(t, infusionKeyBlade, infusionMace)
	cloak, cloakEntry := it.infuseCloak(t)
	pc := it.read(t)
	for _, equipped := range []bool{false, true} {
		if _, err := setEquipped(it.dona, pc, infusionMace, equipped); err != nil {
			t.Fatalf("SetEquipped(mace, %v): %v", equipped, err)
		}
		if got := endedWith(it.record(t, blade.ID)); got != "" {
			t.Errorf("the mace equipped = %v ended its record with %q", equipped, got)
		}
	}
	// A save that carries the armor instead of wearing it keeps it listed.
	req := editorSaveOf(it.read(t))
	req.Sheet.GetFull().CarriedKeys = []string{infusionLeather}
	if _, err := editorUpdate(it.dona, req); err != nil {
		t.Fatalf("the save that carries the armor: %v", err)
	}
	if got := endedWith(it.record(t, cloak.ID)); got != "" {
		t.Errorf("carrying the armor ended its record with %q", got)
	}
	it.openWindow(t, pc.GetId(), true)
	if _, err := it.attuneEntry(it.dona, it.read(t), cloakEntry, true); err != nil {
		t.Fatalf("attuning the cloak: %v", err)
	}
	if got := endedWith(it.record(t, cloak.ID)); got != "" {
		t.Errorf("attuning the cloak ended its record with %q", got)
	}
}

// A save that takes a gear key off the sheet ends no object record: an object is no piece of the sheet.
func TestASaveThatRemovesAGearKeyLeavesAnObjectRecordLive(t *testing.T) {
	t.Parallel()
	it := newInfusedTable(t)
	lantern, err := it.h.svc.insertInfusion(t.Context(), it.h.svc.queries, newInfusion{
		CampaignID: it.campaign, ArtificerID: it.pc.GetId(), HolderID: it.pc.GetId(),
		InfusionKey: infusionKeyHouse, ItemKind: infusionItemObject, ObjectName: "Lanterna",
	})
	if err != nil {
		t.Fatal(err)
	}
	blade := it.infuse(t, infusionKeyBlade, infusionMace)
	req := editorSaveOf(it.read(t))
	req.Sheet.GetFull().WeaponKeys = []string{"equipment:club"}
	if _, err := editorUpdate(it.dona, req); err != nil {
		t.Fatalf("the save that trades the mace for a club: %v", err)
	}
	if got := endedWith(it.record(t, blade.ID)); got != infusionEndedItemGone {
		t.Errorf("the record of the mace ended with %q, want %q", got, infusionEndedItemGone)
	}
	if got := endedWith(it.record(t, lantern.ID)); got != "" {
		t.Errorf("the object record ended with %q, want it live", got)
	}
}

// A save refused for a stale revision rolls back with the rest: the record of the piece it would have taken off stays live.
func TestASaveRefusedForAStaleRevisionLeavesTheGearRecordLive(t *testing.T) {
	t.Parallel()
	it := newInfusedTable(t)
	blade := it.infuse(t, infusionKeyBlade, infusionMace)
	req := editorSaveOf(it.read(t))
	req.Revision += 5
	req.Sheet.GetFull().WeaponKeys = []string{"equipment:club"}
	if _, err := editorUpdate(it.dona, req); err == nil {
		t.Fatal("the save with a stale revision was accepted")
	}
	if got := endedWith(it.record(t, blade.ID)); got != "" {
		t.Errorf("a refused save ended the record with %q, want it live", got)
	}
}
