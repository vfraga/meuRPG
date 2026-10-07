package maps

import (
	"bytes"
	"image/color"
	"image/jpeg"
	"image/png"
	"io"
	"net/http"
	"testing"

	"connectrpc.com/connect"

	mapsv1 "github.com/PuraFome/meuRPG/backend/gen/meurpg/maps/v1"
	"github.com/PuraFome/meuRPG/backend/internal/rules/grid"
)

// Finding U5-3: a tiny image with a 200-column grid makes tiles with zero pixels; the index lists them and they panic (JPEG) or answer 503 (PNG).
func TestReview5_TinyImageTiles(t *testing.T) {
	var jbuf bytes.Buffer
	if err := jpeg.Encode(&jbuf, solid(10, 10, color.NRGBA{R: 200, G: 120, B: 40, A: 255}), nil); err != nil {
		t.Fatal(err)
	}
	var pbuf bytes.Buffer
	if err := png.Encode(&pbuf, solid(10, 10, color.NRGBA{R: 30, G: 90, B: 160, A: 255})); err != nil {
		t.Fatal(err)
	}
	for _, tc := range []struct {
		name, file string
		data       []byte
	}{{"jpeg", "tiny.jpg", jbuf.Bytes()}, {"png", "tiny.png", pbuf.Bytes()}} {
		t.Run(tc.name, func(t *testing.T) {
			h := newHarness(t)
			master, ana := h.newUser("Mestre"), h.newUser("Ana")
			campaign := h.newCampaign(master, ana)
			imageID := master.mustUpload(campaign, tc.file, tc.data).GetId()
			mapID := master.createMap(campaign, "Minúsculo", imageID).GetId()
			master.setMapRevealed(campaign, mapID, true)
			m := master.mustSetGrid(campaign, mapID, 200)
			if _, err := master.maps.SetMapFog(t.Context(), connect.NewRequest(&mapsv1.SetMapFogRequest{CampaignId: campaign, MapId: mapID, FogEnabled: new(true)})); err != nil {
				t.Fatalf("SetMapFog(on) error = %v", err)
			}
			master.start(campaign)
			if _, err := master.setCurrentMap(campaign, mapID); err != nil {
				t.Fatalf("SetCurrentMap() error = %v", err)
			}
			hero := ana.pc(campaign, "Pensantus", "race:gnome")
			g := grid.Grid{Columns: int(m.GetGridColumns()), Rows: int(m.GetGridRows())}
			x, y := g.CenterOf(grid.Square{Col: 0, Row: 0})
			master.placeToken(campaign, mapID, hero.GetId(), int32(x), int32(y)) //nolint:gosec // G115: at most 10000
			res := ana.mustVision(campaign, mapID)
			if len(res.GetTiles()) == 0 {
				t.Fatal("the player has no tile at all")
			}
			t.Logf("grid %dx%d, %d tiles listed", g.Columns, g.Rows, len(res.GetTiles()))
			for _, tile := range res.GetTiles() {
				req, err := http.NewRequestWithContext(t.Context(), http.MethodGet, h.server.URL+tileURL(res, tile), nil)
				if err != nil {
					t.Fatal(err)
				}
				req.Header.Set(testUserHeader, ana.id)
				resp, err := h.server.Client().Do(req)
				if err != nil {
					t.Errorf("tile (%d,%d): request failed (server panic?): %v", tile.GetTx(), tile.GetTy(), err)
					continue
				}
				body, _ := io.ReadAll(resp.Body)
				_ = resp.Body.Close()
				if resp.StatusCode != http.StatusOK {
					t.Errorf("tile (%d,%d): status %d, body %s; want 200", tile.GetTx(), tile.GetTy(), resp.StatusCode, body)
					continue
				}
				if _, err := png.Decode(bytes.NewReader(body)); err != nil {
					t.Errorf("tile (%d,%d): not a PNG: %v", tile.GetTx(), tile.GetTy(), err)
				}
			}
		})
	}
}
