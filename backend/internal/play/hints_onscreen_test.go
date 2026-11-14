package play

import (
	"testing"

	"connectrpc.com/connect"

	playv1 "github.com/PuraFome/meuRPG/backend/gen/meurpg/play/v1"
)

// The current map and the shown image (#735 part 2, 4d) go through Service.tx: the session's number moves once with the write, the
// hints carry it, an image shown to some players is told to them alone, and a refused call tells nothing.

const (
	kCurrentMap = "*playv1.WatchGameSessionResponse_CurrentMapChanged_"
	kShownImage = "*playv1.WatchGameSessionResponse_ShownImageChanged_"
	kLeftImages = "*playv1.WatchGameSessionResponse_LeftImagesChanged_"
)

func (at *sceneTable) currentMapCall(t *testing.T, mapID string) func() error {
	t.Helper()
	return func() error {
		_, err := at.master.play.SetCurrentMap(t.Context(), connect.NewRequest(&playv1.SetCurrentMapRequest{CampaignId: at.campaignID, MapId: mapID}))
		return err
	}
}

func (at *sceneTable) showCall(image string, keep bool, aud *playv1.ImageAudience) func() error {
	return func() error { _, err := at.show(image, keep, aud); return err }
}

func (at *sceneTable) imageAudienceCall(aud *playv1.ImageAudience) func() error {
	return func() error { _, err := at.setImageAudience(at.master, aud); return err }
}

func TestSetCurrentMapHints(t *testing.T) {
	t.Parallel()
	at := newSceneTable(t)
	g := at.sceneHints(t)
	g.unnumbered = []string{"*playv1.WatchGameSessionResponse_MapChanged_"}
	heard := g.first(at.currentMapCall(t, at.mapID), true)
	heardKinds(t, heard, map[string][]string{"master": {kCurrentMap}, "Ana": {kCurrentMap}, "Caio": {kCurrentMap}, "Bia": {kCurrentMap}})
}

func TestSetCurrentMapRefusedTellsNothing(t *testing.T) {
	t.Parallel()
	at := newSceneTable(t)
	g := at.sceneHints(t)
	g.refused(at.currentMapCall(t, newKey())) // no such map
}

func TestSetCurrentMapWithNoOpenSessionRefusesAsBefore(t *testing.T) {
	t.Parallel()
	at := newSceneTable(t)
	at.master.end(t, at.master.liveSession(t, at.campaignID).GetGameSession())
	if err := at.currentMapCall(t, at.mapID)(); connect.CodeOf(err) != connect.CodeFailedPrecondition {
		t.Fatalf("SetCurrentMap(no open session) error = %v, want failed_precondition", err)
	}
}

func TestSetShownImageHints(t *testing.T) {
	t.Parallel()
	at := newSceneTable(t)
	image := at.image(t)
	g := at.sceneHints(t)
	heard := g.first(at.showCall(image, false, nil), true)
	heardKinds(t, heard, map[string][]string{"master": {kShownImage}, "Ana": {kShownImage}, "Caio": {kShownImage}, "Bia": {kShownImage}})
}

// An image shown to chosen players is told to them and the master: nobody else hears of it.
func TestSetShownImageToSomePlayersTellsOnlyThem(t *testing.T) {
	t.Parallel()
	at := newSceneTable(t)
	image := at.image(t)
	g := at.sceneHints(t, "Caio", "Bia")
	heard := g.first(at.showCall(image, false, imageAudience(kindChosen, at.ana)), true)
	heardKinds(t, heard, map[string][]string{"master": {kShownImage}, "Ana": {kShownImage}})
}

func TestSetShownImageReplacedOneKeptTellsTheListToo(t *testing.T) {
	t.Parallel()
	at := newSceneTable(t)
	first, second := at.image(t), at.image(t)
	do(t, at.showCall(first, true, imageAudience(kindChosen, at.ana)))
	g := at.sceneHints(t, "Caio", "Bia")
	heard := g.first(at.showCall(second, false, imageAudience(kindChosen, at.ana)), true)
	heardKinds(t, heard, map[string][]string{"master": {kLeftImages, kShownImage}, "Ana": {kLeftImages, kShownImage}})
}

// Only the switch moves when the image is the same: the number moves, and no player is told.
func TestSetShownImageSwitchOnlyMovesTheNumber(t *testing.T) {
	t.Parallel()
	at := newSceneTable(t)
	image := at.image(t)
	do(t, at.showCall(image, false, nil))
	g := at.sceneHints(t)
	before := g.seq()
	do(t, at.showCall(image, true, nil))
	if got := g.seq(); got != before+1 {
		t.Errorf("hint_seq = %d, want %d", got, before+1)
	}
	for name, evs := range g.drain() {
		if len(evs) != 0 {
			t.Errorf("%s heard %v of a switch", name, eventKinds(evs))
		}
	}
}

func TestSetShownImageRefusedTellsNothing(t *testing.T) {
	t.Parallel()
	at := newSceneTable(t)
	g := at.sceneHints(t)
	g.refused(at.showCall(newKey(), false, nil)) // no such image
}

func TestSetShownImageWithNoOpenSessionRefusesAsBefore(t *testing.T) {
	t.Parallel()
	at := newSceneTable(t)
	image := at.image(t)
	at.master.end(t, at.master.liveSession(t, at.campaignID).GetGameSession())
	if err := at.showCall(image, false, nil)(); connect.CodeOf(err) != connect.CodeFailedPrecondition {
		t.Fatalf("SetShownImage(no open session) error = %v, want failed_precondition", err)
	}
}

func TestSetShownImageAudienceHints(t *testing.T) {
	t.Parallel()
	at := newSceneTable(t)
	do(t, at.showCall(at.image(t), false, imageAudience(kindChosen, at.ana)))
	g := at.sceneHints(t, "Caio")
	// Bia gains the image, Ana keeps it (told nothing new), the master reads the new audience.
	heard := g.first(at.imageAudienceCall(imageAudience(kindChosen, at.ana, at.bia)), true)
	heardKinds(t, heard, map[string][]string{"master": {kShownImage}, "Bia": {kShownImage}})
}

// A player taken out of the audience hears an image that goes away; a player outside it before and after hears nothing.
func TestSetShownImageAudienceNarrowingTellsTheOnesWhoLoseIt(t *testing.T) {
	t.Parallel()
	at := newSceneTable(t)
	do(t, at.showCall(at.image(t), false, imageAudience(kindChosen, at.ana, at.bia)))
	g := at.sceneHints(t, "Caio")
	heard := g.first(at.imageAudienceCall(imageAudience(kindChosen, at.ana)), true)
	heardKinds(t, heard, map[string][]string{"master": {kShownImage}, "Bia": {kShownImage}})
}

func TestSetShownImageAudienceSameMovesNothing(t *testing.T) {
	t.Parallel()
	at := newSceneTable(t)
	do(t, at.showCall(at.image(t), false, imageAudience(kindChosen, at.ana)))
	g := at.sceneHints(t)
	before := g.seq()
	do(t, at.imageAudienceCall(imageAudience(kindChosen, at.ana)))
	if got := g.seq(); got != before {
		t.Errorf("hint_seq = %d, want %d: the audience is the same", got, before)
	}
	for name, evs := range g.drain() {
		if len(evs) != 0 {
			t.Errorf("%s heard %v of an audience that did not change", name, eventKinds(evs))
		}
	}
}

func TestSetShownImageAudienceRefusedTellsNothing(t *testing.T) {
	t.Parallel()
	at := newSceneTable(t)
	g := at.sceneHints(t)
	g.refused(at.imageAudienceCall(imageAudience(kindChosen, at.ana))) // no image is shown
}
