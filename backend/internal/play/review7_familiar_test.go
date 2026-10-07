package play

import (
	"testing"

	"connectrpc.com/connect"

	charactersv1 "github.com/PuraFome/meuRPG/backend/gen/meurpg/characters/v1"
)

// Finding U7-7 (review/unit-07-characters.md): the owner starts the familiar's eyes in a
// combat (blinded + deafened given), then the familiar is dismissed. liveFamiliar hides
// the sight from every read, so endSight never sees it and the conditions stay.
func TestReview7_DismissingTheFamiliarEndsItsEyes(t *testing.T) {
	t.Parallel()
	s := newShapers(t)
	a := s.armed
	owl := s.nanquim(t)
	e := s.pensantusFirst(t, nil)
	a.mustSight(t, s.ana, s.pens, true)
	if _, err := s.ana.characters.DismissCreature(t.Context(), connect.NewRequest(&charactersv1.DismissCreatureRequest{CampaignId: s.campaignID, CreatureId: owl})); err != nil {
		t.Fatalf("DismissCreature() error = %v", err)
	}
	// His next turn starts.
	e = a.passTo(t, a.mustEndTurn(t, s.ana, a.get(t, s.ana)), "Pensantus")
	_ = e
	if got := byLabel(t, a.get(t, s.master), "Pensantus").GetConditions(); len(got) != 0 {
		t.Errorf("conditions at the start of his next turn after the familiar was dismissed = %v, want none (the sight's blinded/deafened)", got)
	}
	// And ending the combat must not leave them either.
	a.endEncounter(t, a.get(t, s.master))
	if got := byLabel(t, a.get(t, s.master), "Pensantus").GetConditions(); len(got) != 0 {
		t.Errorf("conditions after the combat ended = %v, want none", got)
	}
}
