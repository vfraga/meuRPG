package leaktest

import (
	"testing"

	mapsv1 "github.com/PuraFome/meuRPG/backend/gen/meurpg/maps/v1"
	notesv1 "github.com/PuraFome/meuRPG/backend/gen/meurpg/notes/v1"
	"github.com/PuraFome/meuRPG/backend/gen/meurpg/notes/v1/notesv1connect"
	playv1 "github.com/PuraFome/meuRPG/backend/gen/meurpg/play/v1"
)

// Finding U9-15: ListNoteScenes names a scene point of a map the players cannot open once a puzzle's on_solve reveals the point.
func TestReview9_NoteScenesOfHiddenMap(t *testing.T) {
	w := newWorld(t)
	ctx := t.Context()
	pz := w.puzzles["shown"].GetId()
	for wheel, n := range []int{7, 3, 5, 1} {
		for range n {
			must(w.ana.puzzles.MakePuzzleMove(ctx, rq(&playv1.MakePuzzleMoveRequest{
				CampaignId: w.campaign, PuzzleId: pz, IdempotencyKey: newKey(),
				Move: &playv1.PuzzleMove{Kind: &playv1.PuzzleMove_Lock{Lock: &playv1.LockMove{Wheel: int32(wheel), Delta: 1}}},
			})))
		}
	}
	// The map stays unopenable: the positive control for "hidden".
	if _, err := w.ana.maps.GetMap(ctx, rq(&mapsv1.GetMapRequest{CampaignId: w.campaign, MapId: w.hiddenMap})); err == nil {
		t.Fatal("Ana can open the hidden map: the control is void")
	}
	for _, p := range []*person{w.ana, w.caio} {
		r := p.call(notesv1connect.NotesServiceListNoteScenesProcedure, &notesv1.ListNoteScenesRequest{CampaignId: w.campaign})
		if !r.ok() {
			t.Fatalf("%s: ListNoteScenes HTTP %d %s", p.name, r.status, r.body)
		}
		for _, f := range w.inspect(p, r, []string{"fog-point-name", "entrance-name", "fog-point-description"}) {
			t.Errorf("%s: %s\n\tanswer: %s", p.name, f, shorten(r.body))
		}
	}
}
