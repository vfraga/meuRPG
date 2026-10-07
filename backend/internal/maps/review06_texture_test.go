package maps

import (
	"testing"

	mapsv1 "github.com/PuraFome/meuRPG/backend/gen/meurpg/maps/v1"
	"github.com/PuraFome/meuRPG/backend/internal/maps/images/gen"
	"github.com/PuraFome/meuRPG/backend/internal/rules/grid"
)

// Finding U6-B: a hidden NPC's portrait is refused as an object for the scene art and the
// isometric view, but the textured map (which can become the map the players read) skips
// checkCharacters and lets it through as a reference to the model.
func TestReview6_TexturedMapRefusesAHiddenNpcPortrait(t *testing.T) {
	t.Parallel()
	fake := &gen.Fake{}
	c := newCave(t, withFake(fake, 20))
	m := c.master
	portrait := m.mustUpload(c.campaign, "emboscado.png", pngImage(t, 30, 40)).GetId()
	hidden := m.createNPC(c.campaign, "Emboscado Seis", portrait)
	m.placeAt(c.campaign, c.mapID, hidden.GetId(), grid.Square{Col: 8, Row: 8}) // a token starts hidden

	// Control: the isometric view refuses it.
	_, err := m.generateFromMap(c.campaign, c.mapID, kindIsometricAPI, "Uma sala", func(r *mapsv1.GenerateMapImageRequest) { r.ObjectImageIds = []string{portrait} })
	wantInvalidField(t, "isometric with a hidden NPC's portrait", err, "object_image_ids")

	_, err = m.generateFromMap(c.campaign, c.mapID, kindTexturedAPI, "Uma sala", func(r *mapsv1.GenerateMapImageRequest) { r.ObjectImageIds = []string{portrait} })
	if err == nil {
		// Accepted: wait for it and see what the model got.
		done := m.mustGenerateFromMap(c.campaign, c.mapID, kindTexturedAPI, "Uma sala", func(r *mapsv1.GenerateMapImageRequest) { r.ObjectImageIds = []string{portrait} })
		t.Errorf("textured map accepted a hidden NPC's portrait as object_image_ids (state %v)", done.GetGeneration().GetState())
		for _, call := range fake.Calls() {
			t.Logf("model call: layout %q, %d references", call.Request.Layout, len(call.Request.References))
		}
		return
	}
	wantInvalidField(t, "textured map with a hidden NPC's portrait", err, "object_image_ids")
}
