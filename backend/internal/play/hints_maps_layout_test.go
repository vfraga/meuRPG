package play

import (
	"testing"

	"connectrpc.com/connect"
	"google.golang.org/protobuf/proto"

	mapsv1 "github.com/PuraFome/meuRPG/backend/gen/meurpg/maps/v1"
	playv1 "github.com/PuraFome/meuRPG/backend/gen/meurpg/play/v1"
)

// The writes of the maps module that change a map, its layers, its fog, its points or its fog memory run under play's funnel
// (Service.InSession, #735 part 2, 4d maps): one number per write, carried by what the write tells; a repeat of a key moves nothing
// and tells again; a refused call tells nothing; with no open session nothing moves.

func (r *trapRig) imageOf(t *testing.T, mapID string) string {
	t.Helper()
	var imageID string
	if err := r.h.pool.QueryRow(t.Context(), `SELECT image_id::TEXT FROM maps WHERE id = $1`, mapID).Scan(&imageID); err != nil {
		t.Fatalf("read the map's image: %v", err)
	}
	return imageID
}

func (r *trapRig) revisionOf(t *testing.T, mapID string) int32 {
	t.Helper()
	var rev int32
	if err := r.h.pool.QueryRow(t.Context(), `SELECT revision FROM maps WHERE id = $1`, mapID).Scan(&rev); err != nil {
		t.Fatalf("read the map's revision: %v", err)
	}
	return rev
}

func TestCreateMapHints(t *testing.T) {
	t.Parallel()
	r, g := mapRigHints(t, "Caio", "Ana", "Bia") // a new map is hidden: the master alone hears
	imageID := r.imageOf(t, r.mapID)
	create := func(key, name string) func() error {
		return func() error {
			_, err := r.mc(r.master).CreateMap(t.Context(), connect.NewRequest(&mapsv1.CreateMapRequest{CampaignId: r.campaignID, Name: name, ImageId: imageID, IdempotencyKey: key}))
			return err
		}
	}
	key := newKey()
	g.keyed(create(key, "Porão"), create(key, "Porão"), create(key, "Outro"))
	g.lost(create(newKey(), "Torre"))
	g.refused(func() error {
		_, err := r.mc(r.master).CreateMap(t.Context(), connect.NewRequest(&mapsv1.CreateMapRequest{CampaignId: r.campaignID, Name: "Sem imagem", ImageId: newKey()}))
		return err
	})
}

func TestMapUpdateRevealGridAndDeleteHints(t *testing.T) {
	t.Parallel()
	r, g := mapRigHints(t)
	second := r.secondMap(t)
	rev := r.revisionOf(t, second)
	mc := r.mc(r.master)
	g.first(func() error {
		_, err := mc.UpdateMap(t.Context(), connect.NewRequest(&mapsv1.UpdateMapRequest{CampaignId: r.campaignID, MapId: second, Revision: rev, Name: proto.String("Adega")}))
		return err
	}, true)
	g.refused(func() error {
		_, err := mc.UpdateMap(t.Context(), connect.NewRequest(&mapsv1.UpdateMapRequest{CampaignId: r.campaignID, MapId: second, Revision: rev, Name: proto.String("Velha")}))
		return err // a stale revision
	})
	g.first(func() error {
		_, err := mc.SetMapGrid(t.Context(), connect.NewRequest(&mapsv1.SetMapGridRequest{CampaignId: r.campaignID, MapId: second, Columns: 20}))
		return err
	}, true)
	reveal := func(revealed bool) func() error {
		return func() error {
			_, err := mc.SetMapRevealed(t.Context(), connect.NewRequest(&mapsv1.SetMapRevealedRequest{CampaignId: r.campaignID, MapId: second, Revealed: revealed}))
			return err
		}
	}
	g.first(reveal(true), true)
	g.refused(func() error {
		_, err := mc.SetMapRevealed(t.Context(), connect.NewRequest(&mapsv1.SetMapRevealedRequest{CampaignId: r.campaignID, MapId: newKey(), Revealed: true}))
		return err
	})
	g.first(func() error {
		_, err := mc.DeleteMap(t.Context(), connect.NewRequest(&mapsv1.DeleteMapRequest{CampaignId: r.campaignID, MapId: second}))
		return err
	}, true)
}

func TestMapWithNoOpenSessionMovesNothing(t *testing.T) {
	t.Parallel()
	r, g := mapRigHints(t)
	second := r.secondMap(t)
	r.noSession(t, g)
	// What the write tells still goes out, with no number.
	defer func() {
		heard := g.drain()
		if len(heard["master"]) == 0 {
			t.Errorf("the master heard nothing of a map write with no open session")
		}
		for _, ev := range heard["master"] {
			if ev.GetSessionSeq() != 0 {
				t.Errorf("%T carries number %d with no open session", ev.GetEvent(), ev.GetSessionSeq())
			}
		}
	}()
	g.withNoOpenSession(func() error {
		_, err := r.mc(r.master).SetMapRevealed(t.Context(), connect.NewRequest(&mapsv1.SetMapRevealedRequest{CampaignId: r.campaignID, MapId: second, Revealed: true}))
		return err
	})
	g.withNoOpenSession(func() error {
		_, err := r.mc(r.master).UpdateMap(t.Context(), connect.NewRequest(&mapsv1.UpdateMapRequest{CampaignId: r.campaignID, MapId: second, Revision: r.revisionOf(t, second), Name: proto.String("Adega")}))
		return err
	})
}

func TestPaintMapCellsHints(t *testing.T) {
	t.Parallel()
	r, g := mapRigHints(t)
	g.unnumbered = []string{kMapChanged} // the paint's hint goes through the gate and carries no number
	paint := func(value int32, cols ...int32) func() error {
		var squares []*mapsv1.MapSquare
		for _, c := range cols {
			squares = append(squares, &mapsv1.MapSquare{Col: c, Row: 3})
		}
		return func() error {
			_, err := r.mc(r.master).PaintMapCells(t.Context(), connect.NewRequest(&mapsv1.PaintMapCellsRequest{
				CampaignId: r.campaignID, MapId: r.mapID, Layer: mapsv1.MapLayer_MAP_LAYER_WALL, Value: value, Squares: squares,
			}))
			return err
		}
	}
	g.first(paint(1, 4, 5, 6), true) // three squares, one number
	g.refused(paint(1, 99))          // outside the grid
	// Painting what is there changes nothing: the number stays and nobody hears.
	before := g.seq()
	if err := paint(1, 4)(); err != nil {
		t.Fatalf("PaintMapCells(again) error = %v", err)
	}
	if got := g.seq(); got != before {
		t.Errorf("hint_seq = %d after a paint that changed nothing, want %d", got, before)
	}
}

func TestMapPointHints(t *testing.T) {
	t.Parallel()
	r, g := mapRigHints(t, "Caio", "Ana", "Bia") // a new point is hidden: the master alone hears
	x, y := sq(10, 4)
	create := func(key, name string) func() error {
		return func() error {
			_, err := r.mc(r.master).CreateMapPoint(t.Context(), connect.NewRequest(&mapsv1.CreateMapPointRequest{
				CampaignId: r.campaignID, MapId: r.mapID, Kind: mapsv1.MapPointKind_MAP_POINT_KIND_BATTLE, Name: name, XBp: x, YBp: y, IdempotencyKey: key,
			}))
			return err
		}
	}
	key := newKey()
	g.keyed(create(key, "Emboscada"), create(key, "Emboscada"), create(key, "Outra"))
	g.lost(create(newKey(), "Ninho"))
	var pointID string
	if err := r.h.pool.QueryRow(t.Context(), `SELECT id::TEXT FROM map_points WHERE map_id = $1 AND name = 'Emboscada'`, r.mapID).Scan(&pointID); err != nil {
		t.Fatalf("find the point: %v", err)
	}
	mc := r.mc(r.master)
	g.first(func() error {
		_, err := mc.UpdateMapPoint(t.Context(), connect.NewRequest(&mapsv1.UpdateMapPointRequest{CampaignId: r.campaignID, MapId: r.mapID, PointId: pointID, Name: proto.String("Cilada")}))
		return err
	}, true)
	g.refused(func() error {
		_, err := mc.UpdateMapPoint(t.Context(), connect.NewRequest(&mapsv1.UpdateMapPointRequest{CampaignId: r.campaignID, MapId: r.mapID, PointId: newKey(), Name: proto.String("Nada")}))
		return err
	})
	g.quiet = nil // revealing it tells the players too
	g.first(func() error {
		_, err := mc.SetMapPointRevealed(t.Context(), connect.NewRequest(&mapsv1.SetMapPointRevealedRequest{CampaignId: r.campaignID, MapId: r.mapID, PointId: pointID, Revealed: true}))
		return err
	}, true)
	g.first(func() error {
		_, err := mc.DeleteMapPoint(t.Context(), connect.NewRequest(&mapsv1.DeleteMapPointRequest{CampaignId: r.campaignID, MapId: r.mapID, PointId: pointID}))
		return err
	}, true)
}

// The fog's own writes: the settings, a wall, a token and a carried light on a map with the fog on, and the master's clear of what
// the players remember. Each moves the number once, and what the players hear carries it.
func TestFogWritesHints(t *testing.T) {
	t.Parallel()
	f := newFogCave(t)
	g := newHintRig(t, f.h, f.campaignID, f.master, map[string]*user{"Caio": f.caio, "Ana": f.ana, "Bia": f.bia})
	g.unnumbered = []string{kMapChanged} // the paint's hint goes through the gate and carries no number
	mc := f.mapsAs(f.master)
	g.first(func() error {
		_, err := mc.SetMapFog(t.Context(), connect.NewRequest(&mapsv1.SetMapFogRequest{CampaignId: f.campaignID, MapId: f.mapID, GroupVision: proto.Bool(true)}))
		return err
	}, true)
	g.first(func() error {
		_, err := mc.PaintMapCells(t.Context(), connect.NewRequest(&mapsv1.PaintMapCellsRequest{
			CampaignId: f.campaignID, MapId: f.mapID, Layer: mapsv1.MapLayer_MAP_LAYER_WALL, Value: 1, Squares: []*mapsv1.MapSquare{{Col: 18, Row: 7}},
		}))
		return err
	}, true)
	g.first(func() error {
		_, err := mc.ForgetMapVision(t.Context(), connect.NewRequest(&mapsv1.ForgetMapVisionRequest{CampaignId: f.campaignID, MapId: f.mapID}))
		return err
	}, false) // the number moves; what the players see is told by the vision refresh when it changes
	g.refused(func() error {
		_, err := mc.SetMapFog(t.Context(), connect.NewRequest(&mapsv1.SetMapFogRequest{CampaignId: f.campaignID, MapId: newKey(), GroupVision: proto.Bool(false)}))
		return err
	})
}

// Paints close together share one `map_changed` (the gate holds the later ones for the first's interval): the hint carries no
// number, because the gate may send it for the last write while a closure kept the first's. A number the hint carried would be
// stale or none; the writes' own bumps move the number.
func TestPaintHintsThroughTheGateNeverCarryAStaleNumber(t *testing.T) {
	t.Parallel()
	f := newFogCave(t)
	g := newHintRig(t, f.h, f.campaignID, f.master, map[string]*user{"Caio": f.caio, "Ana": f.ana, "Bia": f.bia})
	before := g.seq()
	for _, col := range []int32{18, 19, 20} {
		f.paint(t, mapsv1.MapLayer_MAP_LAYER_WALL, 1, &mapsv1.MapSquare{Col: col, Row: 7})
	}
	f.h.hints.Settle(t)
	final := g.seq()
	if final != before+3 {
		t.Fatalf("hint_seq = %d after three paints, want %d", final, before+3)
	}
	for name, evs := range g.drain() {
		for _, ev := range evs {
			if _, ok := ev.GetEvent().(*playv1.WatchGameSessionResponse_MapChanged_); ok && ev.GetSessionSeq() != 0 && ev.GetSessionSeq() != final {
				t.Errorf("%s heard a map_changed with the stale number %d (the session is at %d)", name, ev.GetSessionSeq(), final)
			}
		}
	}
}
