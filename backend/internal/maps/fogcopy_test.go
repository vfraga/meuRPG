package maps

import (
	"context"
	"errors"
	"strings"
	"testing"

	"connectrpc.com/connect"
	"github.com/jackc/pgx/v5"

	mapsv1 "github.com/PuraFome/meuRPG/backend/gen/meurpg/maps/v1"
	playv1 "github.com/PuraFome/meuRPG/backend/gen/meurpg/play/v1"
	"github.com/PuraFome/meuRPG/backend/internal/maps/images/gen"
	"github.com/PuraFome/meuRPG/backend/internal/maps/mapsdb"
)

// A fog map's image is its own (RN-10): the calls that give a map an image read the
// map before their transaction to know which copy to prepare. These tests pin what
// happens to that copy when the transaction retries (40001), when the read fails, and
// when the fog is switched on between the read and the transaction.

// copiesOf is how many fog copies the campaign's gallery holds.
func (h *harness) copiesOf(campaign string) int {
	h.t.Helper()
	var n int
	if err := h.pool.QueryRow(h.t.Context(), `SELECT count(*) FROM gallery_images WHERE campaign_id = $1 AND name LIKE '%(névoa)'`, campaign).Scan(&n); err != nil {
		h.t.Fatal(err)
	}
	return n
}

// mapRow is the map's image and whether its fog is on, read straight from the database.
func (h *harness) mapRow(mapID string) (imageID string, fog bool) {
	h.t.Helper()
	if err := h.pool.QueryRow(h.t.Context(), `SELECT image_id, fog_enabled FROM maps WHERE id = $1`, mapID).Scan(&imageID, &fog); err != nil {
		h.t.Fatal(err)
	}
	return imageID, fog
}

func (u *user) mustSetFog(campaign, mapID string) {
	u.h.t.Helper()
	if _, err := u.maps.SetMapFog(u.h.t.Context(), connect.NewRequest(&mapsv1.SetMapFogRequest{CampaignId: campaign, MapId: mapID, FogEnabled: new(true)})); err != nil {
		u.h.t.Fatalf("SetMapFog() error = %v", err)
	}
}

// rivalCommits makes the first read of the gallery's usage, which every copy's insert
// makes inside the call's transaction, be followed by a writer that commits a gallery row
// and reads the table: the commit of the call's transaction then fails with 40001 and
// InTx runs the closure again. Call it once the setup is done.
func (h *harness) rivalCommits(imageID string) {
	h.t.Helper()
	conn, err := pgx.Connect(h.t.Context(), h.pool.Config().ConnString())
	if err != nil {
		h.t.Fatal(err)
	}
	h.t.Cleanup(func() { _ = conn.Close(h.t.Context()) })
	h.hookAfterQuery("-- name: GetGalleryUsage", func() {
		_, err := conn.Exec(context.Background(),
			`INSERT INTO gallery_images (id, campaign_id, name, content_type, width, height, byte_size, created_at)
			 SELECT gen_random_uuid(), campaign_id, 'rival', content_type, width, height, byte_size, now() FROM gallery_images WHERE id = $1`, imageID)
		if err != nil {
			h.t.Errorf("rival insert: %v", err)
		}
		// Reading the table afterwards pushes the timestamp of the call's later
		// write above this insert, which makes its refresh fail.
		var n int
		if err := conn.QueryRow(context.Background(), `SELECT count(*) FROM gallery_images`).Scan(&n); err != nil {
			h.t.Errorf("rival read: %v", err)
		}
	})
}

// A 40001 retry runs the transaction again: the copy's gallery row exists only inside
// one attempt, so the call ends with one copy of the image and the row of the attempt
// that failed is not kept (CreateMap has TestCreateMapRetryChecksTheOriginalImage).
func TestFogCopyRetry(t *testing.T) {
	t.Parallel()
	t.Run("UpdateMap", func(t *testing.T) {
		t.Parallel()
		h := newHarness(t)
		master := h.newUser("Mestre")
		campaign := h.newCampaign(master)
		x := master.newImage(campaign)
		fog := master.createMap(campaign, "Com névoa", x)
		master.mustSetGrid(campaign, fog.GetId(), 12)
		master.mustSetFog(campaign, fog.GetId())
		plain := master.createMap(campaign, "Sem névoa", master.newImage(campaign))
		h.rivalCommits(x)
		got, err := master.updateMapWith(&mapsv1.UpdateMapRequest{CampaignId: campaign, MapId: plain.GetId(), Revision: plain.GetRevision(), ImageId: new(x)})
		if err != nil {
			t.Fatalf("UpdateMap() error = %v, want success after the retry", err)
		}
		if got.GetImage().GetId() == x || h.copiesOf(campaign) != 1 {
			t.Errorf("after the retry: image %q (the fog map's is %q), %d copies, want a copy of its own and 1 copy in all", got.GetImage().GetId(), x, h.copiesOf(campaign))
		}
	})

	t.Run("SetMapGrid", func(t *testing.T) {
		t.Parallel()
		h := newHarness(t)
		master := h.newUser("Mestre")
		campaign := h.newCampaign(master)
		master.setNewMapFog(campaign, true)
		shared := master.newImage(campaign)
		master.createMap(campaign, "Outro mapa", shared)
		m := master.createMap(campaign, "Imagem comum", shared)
		h.rivalCommits(shared)
		got, err := master.maps.SetMapGrid(t.Context(), connect.NewRequest(&mapsv1.SetMapGridRequest{CampaignId: campaign, MapId: m.GetId(), Columns: 20}))
		if err != nil {
			t.Fatalf("SetMapGrid() error = %v, want success after the retry", err)
		}
		if image := got.Msg.GetMap().GetImage().GetId(); !got.Msg.GetMap().GetFogEnabled() || image == shared || h.copiesOf(campaign) != 1 {
			t.Errorf("after the retry: fog %v, image %q (the shared one is %q), %d copies, want fog on, a copy of its own and 1 copy in all",
				got.Msg.GetMap().GetFogEnabled(), image, shared, h.copiesOf(campaign))
		}
	})

	t.Run("SetMapFog", func(t *testing.T) {
		t.Parallel()
		h := newHarness(t)
		master := h.newUser("Mestre")
		campaign := h.newCampaign(master)
		shared := master.newImage(campaign)
		master.createMap(campaign, "Outro mapa", shared)
		m := master.createMap(campaign, "Imagem comum", shared)
		master.mustSetGrid(campaign, m.GetId(), 12)
		h.rivalCommits(shared)
		master.mustSetFog(campaign, m.GetId())
		if image, fog := h.mapRow(m.GetId()); !fog || image == shared || h.copiesOf(campaign) != 1 {
			t.Errorf("after the retry: fog %v, image %q (the shared one is %q), %d copies, want fog on, a copy of its own and 1 copy in all", fog, image, shared, h.copiesOf(campaign))
		}
	})

	t.Run("UseGeneratedImageAsMapImage", func(t *testing.T) {
		t.Parallel()
		c := newCave(t, withFake(&gen.Fake{}, 20))
		m := c.master
		picture := wantDone(t, "textured map", m.mustGenerateFromMap(c.campaign, c.mapID, kindTexturedAPI, "Uma caverna")).GetId()
		m.createNPC(c.campaign, "Retrato do Mapa", picture) // a portrait: the picture is used elsewhere
		c.h.rivalCommits(c.imageID)
		if _, err := m.useAsMapImage(c.campaign, picture); err != nil {
			t.Fatalf("Use() error = %v, want success after the retry", err)
		}
		if image, _ := c.h.mapRow(c.mapID); image == picture || c.h.copiesOf(c.campaign) != 1 {
			t.Errorf("after the retry: the fog map's image %q (the picture is %q), %d copies, want a copy of its own and 1 copy in all", image, picture, c.h.copiesOf(c.campaign))
		}
	})
}

// readFails makes the service's reads of the statements that contain match fail with an
// error that is not "no rows". Reads inside a transaction go through the transaction and
// are not touched.
type readFails struct {
	mapsdb.DBTX
	match string
}

func (r readFails) QueryRow(ctx context.Context, sql string, args ...any) pgx.Row {
	if strings.Contains(sql, r.match) {
		return failedRow{}
	}
	return r.DBTX.QueryRow(ctx, sql, args...)
}

type failedRow struct{}

func (failedRow) Scan(...any) error { return errors.New("the database went away") }

// The read of the map before the transaction is what says whether a copy is needed.
// When it fails the call fails with it, instead of going on without a copy and giving a
// fog map an image another use shares (RN-10); nothing is written.
func TestFogCopyPreReadThatFailsRefusesTheCall(t *testing.T) {
	t.Parallel()
	failTheRead := func(h *harness) {
		h.svc.queries = mapsdb.New(readFails{DBTX: h.pool, match: "-- name: GetMap :one"})
	}
	t.Run("UpdateMap", func(t *testing.T) {
		t.Parallel()
		h := newHarness(t)
		master := h.newUser("Mestre")
		campaign := h.newCampaign(master)
		shared := master.newImage(campaign)
		master.createMap(campaign, "Outro mapa", shared)
		fog := master.createMap(campaign, "Com névoa", master.newImage(campaign))
		master.mustSetGrid(campaign, fog.GetId(), 12)
		master.mustSetFog(campaign, fog.GetId())
		before, _ := h.mapRow(fog.GetId())
		files := h.storedFiles()
		failTheRead(h)
		cur := h.revisionOf(fog.GetId())
		_, err := master.updateMapWith(&mapsv1.UpdateMapRequest{CampaignId: campaign, MapId: fog.GetId(), Revision: cur, ImageId: &shared})
		wantFailedPreRead(t, h, "UpdateMap", err, fog.GetId(), before, true, files) // the map already had fog
	})

	t.Run("SetMapGrid", func(t *testing.T) {
		t.Parallel()
		h := newHarness(t)
		master := h.newUser("Mestre")
		campaign := h.newCampaign(master)
		master.setNewMapFog(campaign, true)
		shared := master.newImage(campaign)
		master.createMap(campaign, "Outro mapa", shared)
		m := master.createMap(campaign, "Imagem comum", shared)
		files := h.storedFiles()
		failTheRead(h)
		_, err := master.maps.SetMapGrid(t.Context(), connect.NewRequest(&mapsv1.SetMapGridRequest{CampaignId: campaign, MapId: m.GetId(), Columns: 20}))
		wantFailedPreRead(t, h, "SetMapGrid", err, m.GetId(), shared, false, files)
	})

	t.Run("SetMapFog", func(t *testing.T) {
		t.Parallel()
		h := newHarness(t)
		master := h.newUser("Mestre")
		campaign := h.newCampaign(master)
		shared := master.newImage(campaign)
		master.createMap(campaign, "Outro mapa", shared)
		m := master.createMap(campaign, "Imagem comum", shared)
		master.mustSetGrid(campaign, m.GetId(), 12)
		files := h.storedFiles()
		failTheRead(h)
		_, err := master.maps.SetMapFog(t.Context(), connect.NewRequest(&mapsv1.SetMapFogRequest{CampaignId: campaign, MapId: m.GetId(), FogEnabled: new(true)}))
		wantFailedPreRead(t, h, "SetMapFog", err, m.GetId(), shared, false, files)
	})

	t.Run("UseGeneratedImageAsMapImage", func(t *testing.T) {
		t.Parallel()
		c := newCave(t, withFake(&gen.Fake{}, 20))
		m := c.master
		picture := wantDone(t, "textured map", m.mustGenerateFromMap(c.campaign, c.mapID, kindTexturedAPI, "Uma caverna")).GetId()
		if _, err := m.play.SetShownImage(t.Context(), connect.NewRequest(&playv1.SetShownImageRequest{CampaignId: c.campaign, ImageId: picture})); err != nil {
			t.Fatalf("SetShownImage() error = %v", err)
		}
		before, fog := c.h.mapRow(c.mapID)
		files := c.h.storedFiles()
		failTheRead(c.h)
		_, err := m.useAsMapImage(c.campaign, picture)
		wantFailedPreRead(t, c.h, "Use", err, c.mapID, before, fog, files)
	})
}

// wantFailedPreRead checks that the call failed and wrote nothing: the map keeps its
// image and its fog, and the store gains no file.
func wantFailedPreRead(t *testing.T, h *harness, call string, err error, mapID, image string, fog bool, files []string) {
	t.Helper()
	if err == nil {
		t.Errorf("%s with a failing read of the map: no error", call)
	}
	if gotImage, gotFog := h.mapRow(mapID); gotImage != image || gotFog != fog {
		t.Errorf("%s with a failing read of the map left image %q and fog %v, want %q and %v", call, gotImage, gotFog, image, fog)
	}
	if now := h.storedFiles(); len(now) != len(files) {
		t.Errorf("%s with a failing read of the map changed the store: %v, want %v", call, now, files)
	}
}

// revisionOf is the map's revision, read from the database: the failing read would stop
// the API from answering it.
func (h *harness) revisionOf(mapID string) int32 {
	h.t.Helper()
	var revision int32
	if err := h.pool.QueryRow(h.t.Context(), `SELECT revision FROM maps WHERE id = $1`, mapID).Scan(&revision); err != nil {
		h.t.Fatal(err)
	}
	return revision
}

// The fog can come on between the read before the transaction and the lock: the copy
// prepared for a map without fog does not make the new image the fog map's own, and
// the fog switch does not move the revision. The call is refused as a stale map.
func TestUpdateMap_AFogSwitchedOnMeanwhileRefusesTheCall(t *testing.T) {
	t.Parallel()
	h := newHarness(t)
	master := h.newUser("Mestre")
	campaign := h.newCampaign(master)
	shared := master.newImage(campaign)
	master.createMap(campaign, "Outro mapa", shared)
	plain := master.createMap(campaign, "Sem névoa", master.newImage(campaign))
	before, _ := h.mapRow(plain.GetId())
	files := h.storedFiles()

	h.hookAfterQuery("-- name: GetMap :one", func() {
		if _, err := h.pool.Exec(context.Background(), "UPDATE maps SET fog_enabled = true WHERE id = $1", plain.GetId()); err != nil {
			t.Errorf("switch the fog on from the hook: %v", err)
		}
	})
	_, err := master.updateMapWith(&mapsv1.UpdateMapRequest{CampaignId: campaign, MapId: plain.GetId(), Revision: plain.GetRevision(), ImageId: &shared})
	wantUpdateRefusal(t, "UpdateMap with the fog switched on meanwhile", err, updateRefusal{connect.CodeAborted, "the map changed since you opened it; reload it and try again"})
	if image, fog := h.mapRow(plain.GetId()); image != before || !fog {
		t.Errorf("the refused call left image %q (want %q) and fog %v (the hook's)", image, before, fog)
	}
	if now := h.storedFiles(); len(now) != len(files) {
		t.Errorf("the refused call changed the store: %v, want %v", now, files)
	}
}
