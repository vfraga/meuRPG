package characters

import (
	"context"
	"testing"

	"connectrpc.com/connect"
	"google.golang.org/protobuf/proto"

	charactersv1 "github.com/PuraFome/meuRPG/backend/gen/meurpg/characters/v1"
)

// TestUpdateCharacterWeaponMarksAreCheckedAndRoundTrip: a save with a mark for a weapon the sheet does
// not carry is refused with the field path of the mark, and a valid mark survives the save and the read.
func TestUpdateCharacterWeaponMarksAreCheckedAndRoundTrip(t *testing.T) {
	t.Parallel()
	et := newEditorTable(t, nil)
	pc := et.editorStandardFighter(t)

	notCarried := editorSaveOf(pc)
	notCarried.Sheet = editorClone(pc.GetSheet(), func(f *charactersv1.FullSheet) {
		f.WeaponKeys = []string{"equipment:dagger"}
		f.WeaponMarks = []*charactersv1.WeaponMark{{WeaponKey: "equipment:rapier", Silvered: true}}
	})
	_, err := editorUpdate(et.dona, notCarried)
	wantEditorRefusal(t, "a mark for a weapon not carried", err,
		`invalid_argument: sheet.full.weapon_marks[0].weapon_key is not one of the weapon_keys | InvalidField{field="sheet.full.weapon_marks[0].weapon_key"}`)

	valid := editorSaveOf(pc)
	valid.Sheet = editorClone(pc.GetSheet(), func(f *charactersv1.FullSheet) {
		f.WeaponKeys = []string{"equipment:dagger", "equipment:rapier"}
		f.WeaponMarks = []*charactersv1.WeaponMark{{WeaponKey: "equipment:rapier", Silvered: true, Magical: true}}
	})
	saved, err := editorUpdate(et.dona, valid)
	if err != nil {
		t.Fatalf("UpdateCharacter(valid mark) error = %v", err)
	}
	want := []*charactersv1.WeaponMark{{WeaponKey: "equipment:rapier", Silvered: true, Magical: true}}
	check := func(where string, got *charactersv1.Character) {
		t.Helper()
		marks := got.GetSheet().GetFull().GetWeaponMarks()
		if len(marks) != len(want) || !proto.Equal(marks[0], want[0]) {
			t.Errorf("%s: weapon_marks = %v, want %v", where, marks, want)
		}
	}
	check("the save's answer", saved)
	res, err := et.dona.api.GetCharacter(context.Background(), connect.NewRequest(&charactersv1.GetCharacterRequest{CampaignId: et.campaign, CharacterId: pc.GetId()}))
	if err != nil {
		t.Fatalf("GetCharacter() error = %v", err)
	}
	check("the read after the save", res.Msg.GetCharacter())
}

// TestWeaponMarksFollowTheSheetLock: RN-01 locks the whole sheet for its owner, weapon marks included;
// the master still changes them.
func TestWeaponMarksFollowTheSheetLock(t *testing.T) {
	t.Parallel()
	et := newEditorTable(t, nil)
	pc := et.editorStandardFighter(t)
	marked := func(c *charactersv1.Character, mark *charactersv1.WeaponMark) *charactersv1.UpdateCharacterRequest {
		req := editorSaveOf(c)
		req.Sheet = editorClone(c.GetSheet(), func(f *charactersv1.FullSheet) {
			f.WeaponKeys = []string{"equipment:rapier"}
			f.WeaponMarks = []*charactersv1.WeaponMark{mark}
		})
		return req
	}
	if _, err := editorUpdate(et.dona, marked(pc, &charactersv1.WeaponMark{WeaponKey: "equipment:rapier", Magical: true})); err != nil {
		t.Fatalf("the owner's mark before the lock: %v", err)
	}
	et.h.lockSheets(et.campaign)
	current := et.dona.get(t, et.campaign, pc.GetId())
	req := marked(current, &charactersv1.WeaponMark{WeaponKey: "equipment:rapier", Silvered: true})
	_, err := editorUpdate(et.dona, req)
	wantEditorRefusal(t, "the owner's mark change on a locked sheet", err,
		`failed_precondition: the sheet is locked: a game session has started, so only the master edits it | CharacterBlocked{reason=CHARACTER_BLOCKED_REASON_SHEET_LOCKED character_id="`+pc.GetId()+`"}`)
	if _, err := editorUpdate(et.master, req); err != nil {
		t.Errorf("the master's mark change on a locked sheet: %v", err)
	}
}

// TestPactWeaponMarkRoundTripsIsOnlyOneAndFollowsTheSheetLock: the pact weapon mark is saved and read back,
// a second pact weapon is refused at its field, and the lock holds it like the other marks.
func TestPactWeaponMarkRoundTripsIsOnlyOneAndFollowsTheSheetLock(t *testing.T) {
	t.Parallel()
	et := newEditorTable(t, nil)
	pc := et.editorStandardFighter(t)
	marked := func(c *charactersv1.Character, marks ...*charactersv1.WeaponMark) *charactersv1.UpdateCharacterRequest {
		req := editorSaveOf(c)
		req.Sheet = editorClone(c.GetSheet(), func(f *charactersv1.FullSheet) {
			f.WeaponKeys = []string{"equipment:dagger", "equipment:rapier"}
			f.WeaponMarks = marks
		})
		return req
	}
	two := marked(pc, &charactersv1.WeaponMark{WeaponKey: "equipment:dagger", PactWeapon: true}, &charactersv1.WeaponMark{WeaponKey: "equipment:rapier", PactWeapon: true})
	_, err := editorUpdate(et.dona, two)
	wantEditorRefusal(t, "two pact weapons", err,
		`invalid_argument: sheet.full.weapon_marks[1].pact_weapon at most one weapon is the pact weapon | InvalidField{field="sheet.full.weapon_marks[1].pact_weapon"}`)

	saved, err := editorUpdate(et.dona, marked(pc, &charactersv1.WeaponMark{WeaponKey: "equipment:dagger", PactWeapon: true}))
	if err != nil {
		t.Fatalf("UpdateCharacter(pact mark) error = %v", err)
	}
	if marks := saved.GetSheet().GetFull().GetWeaponMarks(); len(marks) != 1 || !marks[0].GetPactWeapon() || marks[0].GetWeaponKey() != "equipment:dagger" {
		t.Errorf("weapon_marks after the save = %v, want the dagger as the pact weapon", marks)
	}

	et.h.lockSheets(et.campaign)
	current := et.dona.get(t, et.campaign, pc.GetId())
	moved := marked(current, &charactersv1.WeaponMark{WeaponKey: "equipment:rapier", PactWeapon: true})
	_, err = editorUpdate(et.dona, moved)
	wantEditorRefusal(t, "the owner's pact weapon change on a locked sheet", err,
		`failed_precondition: the sheet is locked: a game session has started, so only the master edits it | CharacterBlocked{reason=CHARACTER_BLOCKED_REASON_SHEET_LOCKED character_id="`+pc.GetId()+`"}`)
	if _, err := editorUpdate(et.master, moved); err != nil {
		t.Errorf("the master's pact weapon change on a locked sheet: %v", err)
	}
}
