package maps

import (
	"slices"
	"testing"

	"connectrpc.com/connect"
	"google.golang.org/protobuf/proto"

	mapsv1 "github.com/PuraFome/meuRPG/backend/gen/meurpg/maps/v1"
)

// The refusals of CreateDungeonMap, GetDungeonRooms and RedrawDungeonMap, one by one,
// with the code and the sentence each answers (the sentence is what the page shows).
// They freeze the order of the checks: a request that breaks two rules gets the first
// one's answer.

func TestDungeonCreateRefusalsAreFrozen(t *testing.T) {
	t.Parallel()
	d := newDungeonTable(t)
	seed, _ := testDungeonSeed(t)

	// Who may ask: the master only; a player and a stranger read `not_found`.
	_, err := d.ana.tryCreateDungeon(d.campaign, "Masmorra", testDungeonOptions(), &seed)
	wantRefusal(t, "a player creates", err, connect.CodeNotFound, "map not found")
	stranger := d.h.newUser("Estranho")
	_, err = stranger.tryCreateDungeon(d.campaign, "Masmorra", testDungeonOptions(), &seed)
	wantCode(t, "a stranger creates", err, connect.CodeNotFound)

	// The name is checked before the key and before the options.
	_, err = d.master.tryCreateDungeon(d.campaign, "   ", &mapsv1.DungeonOptions{Width: proto.Int32(1)}, &seed)
	wantCode(t, "an empty name", err, connect.CodeInvalidArgument)

	// A key that is not a key is refused before the dungeon is drawn.
	_, err = d.master.dungeons.CreateDungeonMap(t.Context(), connect.NewRequest(&mapsv1.CreateDungeonMapRequest{
		CampaignId: d.campaign, Name: "Masmorra", Options: testDungeonOptions(), Seed: &seed, IdempotencyKey: string(make([]byte, 300)),
	}))
	wantCode(t, "a bad key", err, connect.CodeInvalidArgument)

	// A bad option, then the limits: the options are checked before the room.
	_, err = d.master.tryCreateDungeon(d.campaign, "Masmorra", &mapsv1.DungeonOptions{Width: proto.Int32(1)}, &seed)
	wantCode(t, "a width out of range", err, connect.CodeInvalidArgument)

	// Images off come before the name.
	d.h.svc.blobs = nil
	_, err = d.master.tryCreateDungeon(d.campaign, "   ", testDungeonOptions(), &seed)
	wantRefusal(t, "images off", err, connect.CodeUnavailable, "images are not configured on this server")
	// ... and after who asks.
	_, err = d.ana.tryCreateDungeon(d.campaign, "Masmorra", testDungeonOptions(), &seed)
	wantRefusal(t, "a player, images off", err, connect.CodeNotFound, "map not found")
	var maps int
	if err := d.h.pool.QueryRow(t.Context(), `SELECT count(*) FROM maps WHERE campaign_id = $1`, d.campaign).Scan(&maps); err != nil || maps != 0 {
		t.Errorf("%d maps after the refusals (%v), want none", maps, err)
	}
}

func TestDungeonRoomsRefusalsAndReadAreFrozen(t *testing.T) {
	t.Parallel()
	d := newDungeonTable(t)
	m := d.master
	seed, want := testDungeonSeed(t)
	created := m.createDungeon(d.campaign, "Masmorra de Vesna", testDungeonOptions(), seed).GetMap()
	mapID := created.GetId()

	// Who may read: the master only.
	_, err := d.ana.dungeonRooms(d.campaign, mapID)
	wantRefusal(t, "a player reads the rooms", err, connect.CodeNotFound, "map not found")
	_, err = d.h.newUser("Estranho").dungeonRooms(d.campaign, mapID)
	wantCode(t, "a stranger reads the rooms", err, connect.CodeNotFound)
	_, err = m.dungeonRooms(d.campaign, "not-a-uuid")
	wantRefusal(t, "a bad map ID", err, connect.CodeNotFound, "map not found")
	_, err = m.dungeonRooms(d.campaign, "00000000-0000-4000-8000-000000000000")
	wantRefusal(t, "a map that does not exist", err, connect.CodeNotFound, "map not found")
	plain := m.createMap(d.campaign, "Comum", m.newImage(d.campaign))
	_, err = m.dungeonRooms(d.campaign, plain.GetId())
	wantRefusal(t, "a plain map", err, connect.CodeNotFound, "map not found")

	// The read: the stored seed, version, size, options and rooms, and the scene points of
	// each room (a scene on the floor of the room; a battle point is not a scene).
	scene, err := m.placeScene(d.campaign, mapID, int32(want.Rooms[1].ID)) //nolint:gosec // G115: at most 500 rooms
	if err != nil {
		t.Fatal(err)
	}
	cx, cy := want.Rooms[3].Center()
	m.createPoint(&mapsv1.CreateMapPointRequest{
		CampaignId: d.campaign, MapId: mapID, Kind: mapsv1.MapPointKind_MAP_POINT_KIND_BATTLE, Name: "Emboscada",
		XBp: int32(cx*250 + 100), YBp: int32(cy*250 + 100), //nolint:gosec // G115: inside the map
	})
	got, err := m.dungeonRooms(d.campaign, mapID)
	if err != nil {
		t.Fatal(err)
	}
	rec, err := d.rec(mapID)
	if err != nil {
		t.Fatal(err)
	}
	if got.GetSeed() != uint64(rec.Seed) || got.GetGeneratorVersion() != rec.GeneratorVersion || got.GetWidth() != rec.Width || got.GetHeight() != rec.Height { //nolint:gosec // G115: the same 64 bits
		t.Errorf("seed, version and size = %d, %d, %dx%d; want the stored %d, %d, %dx%d", got.GetSeed(), got.GetGeneratorVersion(), got.GetWidth(), got.GetHeight(), rec.Seed, rec.GeneratorVersion, rec.Width, rec.Height)
	}
	if !got.GetImageIsGenerated() || got.GetCreatedAt() == nil || got.GetOptions().GetWidth() != 41 || got.GetOptions().GetStairs() != 2 {
		t.Errorf("generated %v, created %v, options %v; want a generated image, a date and the options the dungeon was made with", got.GetImageIsGenerated(), got.GetCreatedAt(), got.GetOptions())
	}
	if len(got.GetRooms()) != len(want.Rooms) {
		t.Fatalf("%d rooms, want %d", len(got.GetRooms()), len(want.Rooms))
	}
	for i, r := range got.GetRooms() {
		w := want.Rooms[i]
		f := r.GetFloor()
		if int(r.GetId()) != w.ID || int(f.GetX()) != w.X || int(f.GetY()) != w.Y || int(f.GetWidth()) != w.Width || int(f.GetHeight()) != w.Height {
			t.Errorf("room %d = %v, want room %d at (%d, %d) %dx%d", i, r, w.ID, w.X, w.Y, w.Width, w.Height)
		}
		wantScenes := []string(nil)
		if w.ID == want.Rooms[1].ID {
			wantScenes = []string{scene.GetPoint().GetId()}
		}
		if !slices.Equal(r.GetScenePointIds(), wantScenes) {
			t.Errorf("room %d scenes = %v, want %v", w.ID, r.GetScenePointIds(), wantScenes)
		}
	}

	// A record that cannot be decoded is an internal failure with a fixed sentence, and the rooms
	// come first: the options are not read.
	if _, err := d.h.pool.Exec(t.Context(), `UPDATE generated_dungeons SET options = '{"nope": 1}'::jsonb WHERE map_id = $1`, mapID); err != nil {
		t.Fatal(err)
	}
	_, err = m.dungeonRooms(d.campaign, mapID)
	wantCode(t, "options that cannot be decoded", err, connect.CodeInternal)
	if _, err := d.h.pool.Exec(t.Context(), `UPDATE generated_dungeons SET rooms = '{"nope": 1}'::jsonb WHERE map_id = $1`, mapID); err != nil {
		t.Fatal(err)
	}
	_, err = m.dungeonRooms(d.campaign, mapID)
	wantCode(t, "rooms that cannot be decoded", err, connect.CodeInternal)
}

func TestDungeonRedrawRefusalsAreFrozen(t *testing.T) {
	t.Parallel()
	d := newDungeonTable(t)
	m := d.master
	seed, _ := testDungeonSeed(t)
	created := m.createDungeon(d.campaign, "Masmorra de Vesna", testDungeonOptions(), seed).GetMap()
	mapID := created.GetId()
	const missing = "00000000-0000-4000-8000-000000000000"

	// Who may ask, and what the map must be.
	_, err := d.ana.redraw(d.campaign, mapID)
	wantRefusal(t, "a player redraws", err, connect.CodeNotFound, "map not found")
	_, err = d.h.newUser("Estranho").redraw(d.campaign, mapID)
	wantCode(t, "a stranger redraws", err, connect.CodeNotFound)
	_, err = m.redraw(d.campaign, "not-a-uuid")
	wantRefusal(t, "a bad map ID", err, connect.CodeNotFound, "map not found")
	_, err = m.redraw(d.campaign, missing)
	wantRefusal(t, "a map that does not exist", err, connect.CodeNotFound, "map not found")
	plain := m.createMap(d.campaign, "Comum", m.newImage(d.campaign))
	_, err = m.redraw(d.campaign, plain.GetId())
	wantRefusal(t, "a plain map", err, connect.CodeNotFound, "map not found")

	// No grid: its own reason. A grid with another number of squares: the image changed.
	m.mustSetGrid(d.campaign, mapID, 0)
	_, err = m.redraw(d.campaign, mapID)
	wantMapBlocked(t, "a map without a grid", err, mapsv1.MapBlockedReason_MAP_BLOCKED_REASON_NO_GRID)
	wantRefusal(t, "a map without a grid", err, connect.CodeFailedPrecondition, "the map has no grid")
	m.mustSetGrid(d.campaign, mapID, 20)
	_, err = m.redraw(d.campaign, mapID)
	wantMapBlocked(t, "a grid of other squares", err, mapsv1.MapBlockedReason_MAP_BLOCKED_REASON_IMAGE_CHANGED)
	wantRefusal(t, "a grid of other squares", err, connect.CodeFailedPrecondition,
		"the map is no longer the generated dungeon's: its image, size, grid or layers changed")

	// Layers cleared (a grid change does that): the image changed, as well.
	other := m.createDungeon(d.campaign, "Outra", testDungeonOptions(), seed).GetMap()
	if _, err := d.h.pool.Exec(t.Context(), `DELETE FROM map_layers WHERE map_id = $1`, other.GetId()); err != nil {
		t.Fatal(err)
	}
	_, err = m.redraw(d.campaign, other.GetId())
	wantMapBlocked(t, "layers cleared", err, mapsv1.MapBlockedReason_MAP_BLOCKED_REASON_IMAGE_CHANGED)

	// Images off come after who asks, before the map ID.
	d.h.svc.blobs = nil
	_, err = m.redraw(d.campaign, "not-a-uuid")
	wantRefusal(t, "images off", err, connect.CodeUnavailable, "images are not configured on this server")
	_, err = d.ana.redraw(d.campaign, mapID)
	wantRefusal(t, "a player, images off", err, connect.CodeNotFound, "map not found")
}

// The window between the render and the transaction: what changes there is read again
// under the map's lock, and the new image's files are deleted when the redraw is refused.
func TestDungeonRedrawRefusalsInsideTheTransactionAreFrozen(t *testing.T) {
	t.Parallel()
	seed, _ := testDungeonSeed(t)
	cases := []struct {
		name   string
		during func(d *dungeonTable, mapID string)
		check  func(t *testing.T, err error)
	}{
		{"the map is deleted", func(d *dungeonTable, mapID string) {
			if _, err := d.master.maps.DeleteMap(d.h.t.Context(), connect.NewRequest(&mapsv1.DeleteMapRequest{CampaignId: d.campaign, MapId: mapID})); err != nil {
				d.h.t.Errorf("DeleteMap() error = %v", err)
			}
		}, func(t *testing.T, err error) {
			t.Helper()
			wantRefusal(t, "a redraw of a map deleted meanwhile", err, connect.CodeNotFound, "map not found")
		}},
		{"the layers are cleared", func(d *dungeonTable, mapID string) {
			if _, err := d.h.pool.Exec(d.h.t.Context(), `DELETE FROM map_layers WHERE map_id = $1`, mapID); err != nil {
				d.h.t.Errorf("clear the layers: %v", err)
			}
		}, func(t *testing.T, err error) {
			t.Helper()
			wantMapBlocked(t, "a redraw with the layers cleared meanwhile", err, mapsv1.MapBlockedReason_MAP_BLOCKED_REASON_IMAGE_CHANGED)
		}},
		{"the gallery fills up", func(d *dungeonTable, _ string) { d.h.svc.maxImages = 0 }, func(t *testing.T, err error) {
			t.Helper()
			wantRefusal(t, "a redraw with a full gallery meanwhile", err, connect.CodeResourceExhausted, "the campaign's gallery is full")
		}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			d := newDungeonTable(t)
			created := d.master.createDungeon(d.campaign, "Masmorra de Vesna", testDungeonOptions(), seed).GetMap()
			var before int
			if err := d.h.pool.QueryRow(t.Context(), `SELECT count(*) FROM gallery_images WHERE campaign_id = $1`, d.campaign).Scan(&before); err != nil {
				t.Fatal(err)
			}
			d.h.svc.beforeRedrawTx = func() { d.h.svc.beforeRedrawTx = nil; tc.during(d, created.GetId()) }
			_, err := d.master.redraw(d.campaign, created.GetId())
			tc.check(t, err)
			var after int
			if err := d.h.pool.QueryRow(t.Context(), `SELECT count(*) FROM gallery_images WHERE campaign_id = $1`, d.campaign).Scan(&after); err != nil {
				t.Fatal(err)
			}
			if after > before {
				t.Errorf("%d images after the refused redraw, want at most the %d from before", after, before)
			}
		})
	}
}
