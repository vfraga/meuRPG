package maps

import (
	"testing"

	mapsv1 "github.com/PuraFome/meuRPG/backend/gen/meurpg/maps/v1"
)

// Finding U6-C: a picture the model returned (the call was billed) that cannot
// be stored because the gallery filled up while the call was in the air must
// keep its slot spent; refunding it lets paid pictures escape the monthly and
// daily caps.
func TestReview6_AGalleryFilledMidCallDoesNotRefundAPaidPicture(t *testing.T) {
	t.Parallel()
	fake, entered, release := holdingFake()
	h := newHarness(t, withFake(fake, 5), func(c *Config) { c.MaxImages = 3 })
	master := h.newUser("Mestre")
	campaign := h.newCampaign(master)
	master.newImage(campaign)
	res, err := master.generate(campaign, "uma cena")
	if err != nil {
		t.Fatalf("reserve with room for the picture: %v", err)
	}
	id := res.GetGeneration().GetId()
	<-entered
	// Uploads ignore the open request: fill the gallery meanwhile.
	master.newImage(campaign)
	master.newImage(campaign)
	close(release)
	got := master.waitGeneration(campaign, id)
	if len(fake.Calls()) != 1 {
		t.Fatalf("model calls = %d, want 1", len(fake.Calls()))
	}
	g := got.GetGeneration()
	t.Logf("state=%v failure=%v slotSpent=%v used=%d", g.GetState(), g.GetFailure(), g.GetSlotSpent(), master.imageStatus(campaign).GetUsedThisMonth())
	if g.GetState() != mapsv1.ImageGenerationState_IMAGE_GENERATION_STATE_FAILED {
		t.Fatalf("state = %v, want failed (gallery full)", g.GetState())
	}
	if !g.GetSlotSpent() {
		t.Errorf("a paid picture that could not be stored gave its slot back (SlotSpent=false)")
	}
	if used := master.imageStatus(campaign).GetUsedThisMonth(); used != 1 {
		t.Errorf("UsedThisMonth = %d, want 1 (the model was called and billed)", used)
	}
}
