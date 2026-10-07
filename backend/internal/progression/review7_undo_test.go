package progression

import (
	"testing"

	"connectrpc.com/connect"

	charactersv1 "github.com/PuraFome/meuRPG/backend/gen/meurpg/characters/v1"
)

// Finding U7-1 (review/unit-07-characters.md)
//
// TestReview7_UndoRestoresClampedXP: a sheet with 999,950 XP gets a MANUAL
// award of 100. characters.AddExperience clamps the sheet at 1,000,000, so the
// character really gained 50, but the share stores the nominal 100. Undoing
// the award must give back the XP the sheet had before it (999,950), not
// subtract the nominal amount (999,900).
func TestReview7_UndoRestoresClampedXP(t *testing.T) {
	t.Parallel()
	tb := newTable(t, enemies, 1)
	pc := tb.pcs[0]

	ch := tb.master.character(t, pc)
	ch.GetSheet().GetFull().ExperiencePoints = 999_950
	if _, err := tb.master.characters.UpdateCharacter(t.Context(), connect.NewRequest(&charactersv1.UpdateCharacterRequest{
		CampaignId: tb.campaign, CharacterId: ch.GetId(), Revision: ch.GetRevision(), Name: ch.GetName(), Sheet: ch.GetSheet(),
	})); err != nil {
		t.Fatalf("UpdateCharacter() error = %v", err)
	}
	if got := tb.master.xpOf(t, pc); got != 999_950 {
		t.Fatalf("sheet XP = %d before the award, want 999950", got)
	}

	tb.master.manual(t, tb.campaign, 100, pc.GetId())
	if got := tb.master.xpOf(t, pc); got != 1_000_000 {
		t.Fatalf("sheet XP = %d after the award, want 1000000 (clamped)", got)
	}

	if _, err := tb.master.undo(tb.campaign, newKey()); err != nil {
		t.Fatalf("UndoLastXPAward() error = %v", err)
	}
	if got := tb.master.xpOf(t, pc); got != 999_950 {
		t.Errorf("sheet XP = %d after the undo, want 999950 (the XP before the award)", got)
	}
}
