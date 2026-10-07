package play

import (
	"log/slog"
	"net/http"
	"net/http/httptest"
	"testing"

	"connectrpc.com/connect"

	mapsv1 "github.com/PuraFome/meuRPG/backend/gen/meurpg/maps/v1"
	"github.com/PuraFome/meuRPG/backend/gen/meurpg/maps/v1/mapsv1connect"
	playv1 "github.com/PuraFome/meuRPG/backend/gen/meurpg/play/v1"
	rulesv1 "github.com/PuraFome/meuRPG/backend/gen/meurpg/rules/v1"
	"github.com/PuraFome/meuRPG/backend/internal/maps"
	"github.com/PuraFome/meuRPG/backend/internal/platform/httpserver"
)

// Finding U4-7: settleTrapDamage (traps_damage.go), used by ApplyTrapDamage for a
// trap fired outside a combat, takes the damage from the character's own hit points
// and ignores a Wild Shape beast form. In combat the damage falls on the beast pool
// first (damageBeast), only the leftover going to the druid (RN-02).
func TestReview4_OutOfCombatTrapDamageIgnoresBeastForm(t *testing.T) {
	t.Parallel()
	for _, tc := range []struct {
		name         string
		dice         [2]int
		wantBeast    int32 // beast HP after, 0 = form ended
		wantDruidLos int32
	}{
		{"beast absorbs", [2]int{3, 4}, 4, 0},          // 7 of 11
		{"overflow ends the form", [2]int{6, 6}, 0, 1}, // 12 vs 11: 1 left over
	} {
		t.Run(tc.name, func(t *testing.T) {
			s := newShapers(t)
			content, err := testRules()
			if err != nil {
				t.Fatal(err)
			}
			msvc, err := maps.New(maps.Config{Pool: s.h.pool, Characters: s.h.chars, Live: s.h.svc, Rules: content, Combats: s.h.svc, Logger: slog.New(slog.DiscardHandler)})
			if err != nil {
				t.Fatal(err)
			}
			s.h.svc.SetTraps(msvc)
			msvc.SetTrapFirer(s.h.svc)
			srv := httpserver.New(httpserver.Config{Logger: slog.New(slog.DiscardHandler)})
			msvc.Mount(srv.Handle, mapsSessions{testSessions}, s.h.camps, connect.WithRequireConnectProtocolHeader())
			server := httptest.NewServer(srv.Handler())
			t.Cleanup(server.Close)
			mc := mapsv1connect.NewMapServiceClient(&http.Client{Transport: userTransport{userID: s.master.id, next: server.Client().Transport}}, server.URL)

			mapID := s.h.newMapOf(s.campaignID, 24, 1200, 800)
			if _, err := s.master.play.SetCurrentMap(t.Context(), connect.NewRequest(&playv1.SetCurrentMapRequest{CampaignId: s.campaignID, MapId: mapID})); err != nil {
				t.Fatal(err)
			}
			x, y := sq(5, 5)
			spec := &mapsv1.TrapSpec{NoticeDc: 12, FindDc: 15, AreaSize: 1, Trigger: rulesv1.TrapTrigger_TRAP_TRIGGER_MANUAL,
				Effect: &rulesv1.TrapEffect{Damage: []*rulesv1.TrapDamage{{Dice: "2d6", DamageTypeKey: "damage-type:bludgeoning"}}}}
			pt, err := mc.CreateMapPoint(t.Context(), connect.NewRequest(&mapsv1.CreateMapPointRequest{
				CampaignId: s.campaignID, MapId: mapID, Kind: mapsv1.MapPointKind_MAP_POINT_KIND_TRAP, Name: "Fosso", XBp: x, YBp: y, Trap: spec,
			}))
			if err != nil {
				t.Fatal(err)
			}
			if _, err := mc.PlaceMapToken(t.Context(), connect.NewRequest(&mapsv1.PlaceMapTokenRequest{CampaignId: s.campaignID, MapId: mapID, CharacterId: s.bri.GetId(), XBp: x, YBp: y})); err != nil {
				t.Fatal(err)
			}
			s.mustAssume(t, s.bia, s.bri, wolfKey)
			before := s.vitals(t, s.bri)
			if before.GetWildShape() == nil {
				t.Fatal("no beast form")
			}
			beastBefore := before.GetWildShape().GetHitPointsCurrent()
			s.h.roller.queue(tc.dice[0], tc.dice[1])
			if _, err := s.master.play.FireTrap(t.Context(), connect.NewRequest(&playv1.FireTrapRequest{CampaignId: s.campaignID, MapId: mapID, PointId: pt.Msg.GetPoint().GetId(), IdempotencyKey: newKey()})); err != nil {
				t.Fatal(err)
			}
			list, err := s.master.play.ListTrapDamages(t.Context(), connect.NewRequest(&playv1.ListTrapDamagesRequest{CampaignId: s.campaignID}))
			if err != nil || len(list.Msg.GetDamages()) != 1 {
				t.Fatalf("damages = %v %v", list, err)
			}
			if _, err := s.master.play.ApplyTrapDamage(t.Context(), connect.NewRequest(&playv1.ApplyTrapDamageRequest{CampaignId: s.campaignID, TrapDamageId: list.Msg.GetDamages()[0].GetId(), IdempotencyKey: newKey()})); err != nil {
				t.Fatal(err)
			}
			after := s.vitals(t, s.bri)
			beastAfter := int32(0)
			if w := after.GetWildShape(); w != nil {
				beastAfter = w.GetHitPointsCurrent()
			}
			t.Logf("druid hp %d -> %d; beast %d -> %d (form kept: %v)", before.GetHitPointsCurrent(), after.GetHitPointsCurrent(), beastBefore, beastAfter, after.GetWildShape() != nil)
			if beastAfter != tc.wantBeast {
				t.Errorf("beast hp = %d, want %d: the trap damage bypassed the beast's pool", beastAfter, tc.wantBeast)
			}
			if got := before.GetHitPointsCurrent() - after.GetHitPointsCurrent(); got != tc.wantDruidLos {
				t.Errorf("druid lost %d own hp, want %d", got, tc.wantDruidLos)
			}
		})
	}
}
