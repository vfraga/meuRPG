package characters

import (
	"context"
	"testing"

	"connectrpc.com/connect"
	"google.golang.org/protobuf/proto"

	charactersv1 "github.com/PuraFome/meuRPG/backend/gen/meurpg/characters/v1"
)

// TestUpdateCharacterItemBonusesAreCheckedAndRoundTrip: a bonus on a piece the sheet does not carry, or outside 1 to 3,
// is refused with the field path; a valid one survives the save and the read, and the derived sheet lists it with its
// origin and counts it in the attack.
func TestUpdateCharacterItemBonusesAreCheckedAndRoundTrip(t *testing.T) {
	t.Parallel()
	et := newEditorTable(t, nil)
	pc := et.editorStandardFighter(t)
	with := func(bonuses ...*charactersv1.ItemBonus) *charactersv1.UpdateCharacterRequest {
		req := editorSaveOf(pc)
		req.Sheet = editorClone(pc.GetSheet(), func(f *charactersv1.FullSheet) {
			f.WeaponKeys = []string{"equipment:dagger", "equipment:rapier"}
			f.ItemBonuses = bonuses
		})
		return req
	}
	_, err := editorUpdate(et.dona, with(&charactersv1.ItemBonus{Key: "equipment:longsword", Bonus: 1}))
	wantEditorRefusal(t, "a bonus for a weapon not carried", err,
		`invalid_argument: sheet.full.item_bonuses[0].key is not the sheet's armor, its shield or one of its weapons | InvalidField{field="sheet.full.item_bonuses[0].key"}`)
	_, err = editorUpdate(et.dona, with(&charactersv1.ItemBonus{Key: "equipment:rapier", Bonus: 4}))
	wantEditorRefusal(t, "a bonus of +4", err,
		`invalid_argument: sheet.full.item_bonuses[0].bonus must be from 1 to 3 | InvalidField{field="sheet.full.item_bonuses[0].bonus"}`)

	want := &charactersv1.ItemBonus{Key: "equipment:rapier", Bonus: 2}
	saved, err := editorUpdate(et.dona, with(want))
	if err != nil {
		t.Fatalf("UpdateCharacter(valid bonus) error = %v", err)
	}
	check := func(where string, got *charactersv1.Character) {
		t.Helper()
		bonuses := got.GetSheet().GetFull().GetItemBonuses()
		if len(bonuses) != 1 || !proto.Equal(bonuses[0], want) {
			t.Errorf("%s: item_bonuses = %v, want [%v]", where, bonuses, want)
		}
	}
	check("the save's answer", saved)
	res, err := et.dona.api.GetCharacter(context.Background(), connect.NewRequest(&charactersv1.GetCharacterRequest{CampaignId: et.campaign, CharacterId: pc.GetId()}))
	if err != nil {
		t.Fatalf("GetCharacter() error = %v", err)
	}
	check("the read after the save", res.Msg.GetCharacter())
	mods := res.Msg.GetCharacter().GetDerived().GetItemModifiers()
	if len(mods) != 2 || mods[0].GetTarget() != "attack:equipment:rapier" || mods[0].GetValue() != 2 || !mods[0].GetCounts() ||
		mods[1].GetTarget() != "damage:equipment:rapier" || mods[1].GetSourceKey() != "equipment:rapier" || mods[1].GetSourceNamePt() == "" {
		t.Errorf("item_modifiers = %v, want the +2 of the rapier on attack and damage", mods)
	}
}

// TestItemBonusesFollowTheSheetLock: the lock stops the owner from changing a bonus; the master still can.
func TestItemBonusesFollowTheSheetLock(t *testing.T) {
	t.Parallel()
	et := newEditorTable(t, nil)
	pc := et.editorStandardFighter(t)
	bonused := func(c *charactersv1.Character, b *charactersv1.ItemBonus) *charactersv1.UpdateCharacterRequest {
		req := editorSaveOf(c)
		req.Sheet = editorClone(c.GetSheet(), func(f *charactersv1.FullSheet) {
			f.WeaponKeys = []string{"equipment:rapier"}
			f.ItemBonuses = []*charactersv1.ItemBonus{b}
		})
		return req
	}
	if _, err := editorUpdate(et.dona, bonused(pc, &charactersv1.ItemBonus{Key: "equipment:rapier", Bonus: 1})); err != nil {
		t.Fatalf("the owner's bonus before the lock: %v", err)
	}
	et.h.lockSheets(et.campaign)
	current := et.dona.get(t, et.campaign, pc.GetId())
	req := bonused(current, &charactersv1.ItemBonus{Key: "equipment:rapier", Bonus: 2})
	_, err := editorUpdate(et.dona, req)
	wantEditorRefusal(t, "the owner's bonus change on a locked sheet", err,
		`failed_precondition: the sheet is locked: a game session has started, so only the master edits it | CharacterBlocked{reason=CHARACTER_BLOCKED_REASON_SHEET_LOCKED character_id="`+pc.GetId()+`"}`)
	if _, err := editorUpdate(et.master, req); err != nil {
		t.Errorf("the master's bonus change on a locked sheet: %v", err)
	}
}
