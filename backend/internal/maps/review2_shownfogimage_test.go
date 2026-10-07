package maps

import (
	"testing"
	"time"

	"connectrpc.com/connect"

	mapsv1 "github.com/PuraFome/meuRPG/backend/gen/meurpg/maps/v1"
	playv1 "github.com/PuraFome/meuRPG/backend/gen/meurpg/play/v1"
)

// Finding U2-6: SetShownImage copies a fog map's image on every call, before locking the session
func TestReview2_ShownFogImageCopies(t *testing.T) {
	t.Parallel()
	h := newHarness(t)
	master, ana := h.newUser("Mestre"), h.newUser("Ana")
	campaign := h.newCampaign(master, ana)
	x := master.newImage(campaign)
	fog := master.createMap(campaign, "Com névoa", x)
	master.mustSetGrid(campaign, fog.GetId(), 12)
	master.setMapRevealed(campaign, fog.GetId(), true)
	if _, err := master.maps.SetMapFog(t.Context(), connect.NewRequest(&mapsv1.SetMapFogRequest{CampaignId: campaign, MapId: fog.GetId(), FogEnabled: new(true)})); err != nil {
		t.Fatal(err)
	}
	count := func() int { return len(master.list(campaign).GetImages()) }
	show := func(keep bool) (*playv1.SetShownImageResponse, error) {
		res, err := master.play.SetShownImage(t.Context(), connect.NewRequest(&playv1.SetShownImageRequest{CampaignId: campaign, ImageId: x, Keep: keep}))
		if err != nil {
			return nil, err
		}
		return res.Msg, nil
	}
	left := func() int {
		res, err := ana.play.ListLeftImages(t.Context(), connect.NewRequest(&playv1.ListLeftImagesRequest{CampaignId: campaign}))
		if err != nil {
			t.Fatal(err)
		}
		return len(res.Msg.GetImages())
	}

	// Claim 1: no open session.
	g0 := count()
	for i := 0; i < 2; i++ {
		_, err := show(false)
		wantBlocked(t, "SetShownImage without a session", err, playv1.GameSessionBlockedReason_GAME_SESSION_BLOCKED_REASON_NO_OPEN_SESSION)
	}
	g1 := count()
	t.Logf("claim 1: gallery before=%d after two refused calls=%d", g0, g1)
	if g1 != g0 {
		t.Errorf("claim 1: gallery grew from %d to %d images after refused calls (orphan copies)", g0, g1)
	}

	// Claim 2: session open, same image again to turn keep on.
	master.start(campaign)
	pw := ana.watch(campaign)
	first, err := show(false)
	if err != nil {
		t.Fatal(err)
	}
	firstID := first.GetShownImage().GetId()
	if ev := pw.next().GetShownImageChanged(); ev.GetImage().GetId() != firstID {
		t.Fatalf("first event = %v, want the shown image %s", ev, firstID)
	}
	g2 := count()
	second, err := show(true)
	if err != nil {
		t.Fatal(err)
	}
	secondID := second.GetShownImage().GetId()
	g3 := count()
	t.Logf("claim 2: first shown=%s second shown=%s gallery before=%d after=%d left images (player)=%d", firstID, secondID, g2, g3, left())
	if secondID != firstID {
		t.Errorf("claim 2: calling again with the same image shows %s, want the same %s", secondID, firstID)
	}
	if g3 != g2 {
		t.Errorf("claim 2: gallery grew from %d to %d on the keep call", g2, g3)
	}
	select {
	case ev := <-pw.events:
		if ev.GetHeartbeat() == nil {
			t.Errorf("claim 2: the player received %v on the keep call, want nothing", ev)
		}
	case <-time.After(700 * time.Millisecond):
	}
	// Stop showing: the kept image is left, exactly one.
	if _, err := master.play.SetShownImage(t.Context(), connect.NewRequest(&playv1.SetShownImageRequest{CampaignId: campaign})); err != nil {
		t.Fatal(err)
	}
	if n := left(); n != 1 {
		t.Errorf("claim 2: left images = %d, want 1", n)
	}
}
