package play

import (
	"net/http"
	"testing"

	"connectrpc.com/connect"

	playv1 "github.com/PuraFome/meuRPG/backend/gen/meurpg/play/v1"
)

// Taking back an image left on the table (#735 part 2) goes through Service.tx: the session's number moves once with the take-back,
// the list's hint carries it and goes to the master and the players the image was left with, a player it was not left with hears
// nothing, and an image that is not on the list refuses and moves nothing.

func (at *sceneTable) takeBackCall(image string) func() error {
	return func() error {
		_, err := at.master.play.TakeBackLeftImage(at.h.t.Context(), connect.NewRequest(&playv1.TakeBackLeftImageRequest{CampaignId: at.campaignID, ImageId: image}))
		return err
	}
}

// leave leaves a new image with aud (the whole table when nil) and returns its id.
func (at *sceneTable) leave(t *testing.T, aud *playv1.ImageAudience) string {
	t.Helper()
	image := at.image(t)
	at.mustShow(t, image, true, aud)
	at.mustShow(t, "", false, nil)
	return image
}

func TestTakeBackLeftImageToSomePlayersTellsOnlyThemWithTheNumber(t *testing.T) {
	t.Parallel()
	at := newSceneTable(t)
	image := at.leave(t, imageAudience(kindChosen, at.ana))
	g := at.sceneHints(t, "Caio", "Bia")
	heard := g.first(at.takeBackCall(image), true)
	heardKinds(t, heard, map[string][]string{"master": {kLeftImages}, "Ana": {kLeftImages}})
}

func TestTakeBackLeftImageOfTheWholeTableTellsEveryoneWithTheNumber(t *testing.T) {
	t.Parallel()
	at := newSceneTable(t)
	image := at.leave(t, nil)
	g := at.sceneHints(t)
	heard := g.first(at.takeBackCall(image), true)
	heardKinds(t, heard, map[string][]string{"master": {kLeftImages}, "Ana": {kLeftImages}, "Caio": {kLeftImages}, "Bia": {kLeftImages}})
}

func TestTakeBackLeftImageNotOnTheListRefusesAndMovesNothing(t *testing.T) {
	t.Parallel()
	at := newSceneTable(t)
	image := at.leave(t, nil)
	do(t, at.takeBackCall(image))
	g := at.sceneHints(t)
	g.refused(at.takeBackCall(image))    // taken back already
	g.refused(at.takeBackCall(newKey())) // never left
}

func TestTakeBackLeftImageWithNoOpenSessionStillTakesItBack(t *testing.T) {
	t.Parallel()
	at := newSceneTable(t)
	image := at.leave(t, nil)
	at.master.end(t, at.master.liveSession(t, at.campaignID).GetGameSession())
	do(t, at.takeBackCall(image))
	if got := at.leftFor(t, at.ana); len(got) != 0 {
		t.Errorf("Ana's left images = %v, want none", got)
	}
}

// The line's voice (#735 part 2): making it moves the number once, so every screen reads the line again; a request that finds it
// made moves nothing.
func TestMakingAVoiceMovesTheNumberOnce(t *testing.T) {
	t.Parallel()
	a := newArmed(t)
	line := a.mustSay(a.master, asNarrator, "Um trovão ecoa.")
	g := a.stageHints(t)
	heard := g.firstOf(func() error {
		if r := a.audio(a.ana.id, line.GetId()); r.status != http.StatusOK {
			t.Errorf("audio = %d, want 200", r.status)
		}
		return nil
	}, false, 1)
	heardKinds(t, heard, map[string][]string{})
	before := g.seq()
	if r := a.audio(a.caio.id, line.GetId()); r.status != http.StatusOK {
		t.Errorf("the voice made, asked by Caio = %d, want 200", r.status)
	}
	if r := a.audio(a.ana.id, line.GetId()); r.status != http.StatusOK {
		t.Errorf("the voice made, asked again = %d, want 200", r.status)
	}
	if got := g.seq(); got != before {
		t.Errorf("hint_seq = %d after a request that found the voice made, want %d", got, before)
	}
}

// A second maker that finds the line already marked (the blob was lost, so it was made again) moves nothing.
func TestMakingAVoiceAlreadyMarkedMovesNothing(t *testing.T) {
	t.Parallel()
	a := newArmed(t)
	line := a.mustSay(a.master, asNarrator, "Outro trovão.")
	if r := a.audio(a.ana.id, line.GetId()); r.status != http.StatusOK {
		t.Fatalf("audio = %d, want 200", r.status)
	}
	g := a.stageHints(t)
	if err := a.h.svc.msgs.blobs.Delete(t.Context(), audioKey(a.campaignID, line.GetId())); err != nil {
		t.Fatalf("Delete() error = %v", err)
	}
	before := g.seq()
	if r := a.audio(a.ana.id, line.GetId()); r.status != http.StatusOK {
		t.Fatalf("audio after the blob was lost = %d, want 200", r.status)
	}
	if got := g.seq(); got != before {
		t.Errorf("hint_seq = %d after a voice that was marked already, want %d", got, before)
	}
}
