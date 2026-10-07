package maps

import (
	"testing"

	"connectrpc.com/connect"

	mapsv1 "github.com/PuraFome/meuRPG/backend/gen/meurpg/maps/v1"
	playv1 "github.com/PuraFome/meuRPG/backend/gen/meurpg/play/v1"
)

// Finding U5-4: showing a fog map's image copies it outside the show transaction, so a failed or repeated SetShownImage leaves extra gallery images and blobs.
func TestReview5_ShowFogImageCopyLeaks(t *testing.T) {
	t.Parallel()
	h := newHarness(t)
	master := h.newUser("Mestre")
	campaign := h.newCampaign(master)
	x := master.newImage(campaign)
	fog := master.createMap(campaign, "Com névoa", x)
	master.mustSetGrid(campaign, fog.GetId(), 12)
	if _, err := master.maps.SetMapFog(t.Context(), connect.NewRequest(&mapsv1.SetMapFogRequest{CampaignId: campaign, MapId: fog.GetId(), FogEnabled: new(true)})); err != nil {
		t.Fatal(err)
	}
	fogImage := master.getMapImage(campaign, fog.GetId())
	show := func() error {
		_, err := master.play.SetShownImage(t.Context(), connect.NewRequest(&playv1.SetShownImageRequest{CampaignId: campaign, ImageId: fogImage}))
		return err
	}
	counts := func() (int, int) { return len(master.list(campaign).GetImages()), len(h.storedFiles()) }

	// (a) no open session: fails, must leave nothing behind.
	imgs0, files0 := counts()
	err := show()
	t.Logf("(a) error = %v", err)
	if err == nil {
		t.Fatal("SetShownImage without an open session succeeded, want an error")
	}
	imgs1, files1 := counts()
	t.Logf("(a) images %d -> %d, files %d -> %d", imgs0, imgs1, files0, files1)
	if imgs1 != imgs0 || files1 != files0 {
		t.Errorf("(a) failed show left images %d -> %d and files %d -> %d, want no growth", imgs0, imgs1, files0, files1)
	}

	// (b) open session, same image twice: the second show must not grow anything.
	master.start(campaign)
	imgs2, files2 := counts()
	if err := show(); err != nil {
		t.Fatal(err)
	}
	imgs3, files3 := counts()
	if err := show(); err != nil {
		t.Fatal(err)
	}
	imgs4, files4 := counts()
	t.Logf("(b) images %d -> %d -> %d, files %d -> %d -> %d", imgs2, imgs3, imgs4, files2, files3, files4)
	if imgs4 != imgs3 || files4 != files3 {
		t.Errorf("(b) showing the same fog image again grew images %d -> %d and files %d -> %d, want no growth", imgs3, imgs4, files3, files4)
	}
}
