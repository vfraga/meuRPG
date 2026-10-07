package maps

import (
	"testing"

	"connectrpc.com/connect"

	mapsv1 "github.com/PuraFome/meuRPG/backend/gen/meurpg/maps/v1"
	"github.com/PuraFome/meuRPG/backend/internal/maps/images/gen"
)

// Finding U9-12: changing the kind of a found (not converted) treasure silently erases the finders and the found mark instead of refusing with TREASURE_FOUND.
func TestReview9_FoundTreasureKindChange(t *testing.T) {
	t.Parallel()
	s := newScenes(t, false)
	m := s.master
	chest := s.newTreasure("Baú", "Moedas", 250, 100, 100)
	if _, err := m.maps.MarkTreasureFound(t.Context(), connect.NewRequest(&mapsv1.MarkTreasureFoundRequest{
		CampaignId: s.campaign, MapId: s.mapID, PointId: chest.GetId(), CharacterIds: []string{s.pens.GetId()},
	})); err != nil {
		t.Fatal(err)
	}
	_, err := m.updatePoint(&mapsv1.UpdateMapPointRequest{
		CampaignId: s.campaign, MapId: s.mapID, PointId: chest.GetId(), Kind: mapsv1.MapPointKind_MAP_POINT_KIND_SCENE.Enum(),
	})
	wantMapBlocked(t, "change the kind of a found treasure", err, mapsv1.MapBlockedReason_MAP_BLOCKED_REASON_TREASURE_FOUND)
	var found int
	if err := s.h.pool.QueryRow(t.Context(), `SELECT count(*) FROM map_points WHERE id = $1 AND kind = 'treasure' AND treasure_found_at IS NOT NULL`, chest.GetId()).Scan(&found); err != nil {
		t.Fatal(err)
	}
	if found != 1 {
		t.Error("the found treasure is no longer a found treasure after the kind change")
	}
}

// Finding U9-13: the same idempotency key with a different prompt returns the first generation (200) instead of invalid_argument (architecture.md rule 9).
func TestReview9_ImageKeyReuse(t *testing.T) {
	t.Parallel()
	fake := &gen.Fake{}
	h := newHarness(t, withFake(fake, 5))
	master := h.newUser("Mestre")
	campaign := h.newCampaign(master)
	first, err := master.imagegen.GenerateSceneImage(t.Context(), connect.NewRequest(&mapsv1.GenerateSceneImageRequest{CampaignId: campaign, IdempotencyKey: "k-reuse", Prompt: "uma taverna"}))
	if err != nil {
		t.Fatal(err)
	}
	master.waitGeneration(campaign, first.Msg.GetGeneration().GetId())
	_, err = master.imagegen.GenerateSceneImage(t.Context(), connect.NewRequest(&mapsv1.GenerateSceneImageRequest{CampaignId: campaign, IdempotencyKey: "k-reuse", Prompt: "um dragão furioso"}))
	wantCode(t, "same key, another prompt", err, connect.CodeInvalidArgument)
}
