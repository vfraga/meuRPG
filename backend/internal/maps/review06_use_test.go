package maps

import (
	"testing"

	mapsv1 "github.com/PuraFome/meuRPG/backend/gen/meurpg/maps/v1"
	"github.com/PuraFome/meuRPG/backend/internal/maps/images/gen"
)

// Finding U6-A: after Use(T1), an edit T2 of T1 (basis = original image O) is refused as MAP_CHANGED.
func TestReview6_UseAfterEditOfUsedPicture(t *testing.T) {
	t.Parallel()
	c := newCave(t, withFake(&gen.Fake{}, 20))
	m := c.master
	t1 := wantDone(t, "textured map", m.mustGenerateFromMap(c.campaign, c.mapID, kindTexturedAPI, "Uma caverna"))
	if _, err := m.useAsMapImage(c.campaign, t1.GetId()); err != nil {
		t.Fatalf("Use(T1) error = %v", err)
	}
	res, err := m.editImage(c.campaign, t1.GetId(), "mais clara")
	if err != nil {
		t.Fatalf("EditGeneratedImage(T1) error = %v", err)
	}
	t2 := wantDone(t, "edit", res)
	used, err := m.useAsMapImage(c.campaign, t2.GetId())
	if err != nil {
		wantGenerationBlocked(t, "Use(T2) after Use(T1)", err, mapsv1.ImageGenerationBlockedReason_IMAGE_GENERATION_BLOCKED_REASON_MAP_CHANGED)
		t.Fatalf("Use(T2) after Use(T1) refused as MAP_CHANGED: %v", err)
	}
	_ = mapsv1.ImageGenerationBlockedReason_IMAGE_GENERATION_BLOCKED_REASON_MAP_CHANGED
	if used.GetMap().GetImage().GetId() != t2.GetId() {
		t.Errorf("map image = %s, want %s", used.GetMap().GetImage().GetId(), t2.GetId())
	}
}
