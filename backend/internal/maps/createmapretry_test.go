package maps

import (
	"os"
	"sync/atomic"
	"testing"
	"time"

	"connectrpc.com/connect"
	"github.com/jackc/pgx/v5"

	mapsv1 "github.com/PuraFome/meuRPG/backend/gen/meurpg/maps/v1"
)

// CreateMap's transaction closure must not change what it captured: the fog
// copy's gallery row exists only inside one attempt, so a 40001 retry checks
// the original image again.
func TestCreateMapRetryChecksTheOriginalImage(t *testing.T) {
	t.Parallel()
	var armed, calls atomic.Int32 // armed: 0 off, 1 counting, 2 counting and about to provoke the conflict, 3 provoked
	var rival func()
	h := newHarness(t, func(c *Config) {
		c.Now = func() time.Time {
			// s.now() runs inside CreateMap's transaction, after its reads: the first
			// time, a rival transaction commits a gallery row there, so the commit
			// of CreateMap's own transaction fails with 40001 and InTx runs it again.
			if armed.Load() >= 1 {
				calls.Add(1)
				if armed.CompareAndSwap(2, 3) {
					rival()
				}
			}
			return time.Now()
		}
	})
	master := h.newUser("Mestre")
	campaign := h.newCampaign(master)
	x := master.newImage(campaign)
	fog := master.createMap(campaign, "Com névoa", x)
	master.mustSetGrid(campaign, fog.GetId(), 12)
	if _, err := master.maps.SetMapFog(t.Context(), connect.NewRequest(&mapsv1.SetMapFogRequest{CampaignId: campaign, MapId: fog.GetId(), FogEnabled: new(true)})); err != nil {
		t.Fatal(err)
	}
	conn, err := pgx.Connect(t.Context(), os.Getenv("MEURPG_TEST_DATABASE_URL"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = conn.Close(t.Context()) })
	var dbName string
	if err := h.pool.QueryRow(t.Context(), "SELECT current_database()").Scan(&dbName); err != nil {
		t.Fatal(err)
	}
	if _, err := conn.Exec(t.Context(), "USE "+dbName); err != nil {
		t.Fatal(err)
	}
	rival = func() {
		_, err := conn.Exec(t.Context(),
			`INSERT INTO gallery_images (id, campaign_id, name, content_type, width, height, byte_size, created_at)
			 SELECT gen_random_uuid(), campaign_id, 'rival', content_type, width, height, byte_size, now() FROM gallery_images WHERE id = $1`, x)
		if err != nil {
			t.Errorf("rival insert: %v", err)
		}
		// Reading the table afterwards pushes the timestamp of CreateMap's later
		// write above this insert, which makes its refresh fail.
		var n int
		if err := conn.QueryRow(t.Context(), `SELECT count(*) FROM gallery_images`).Scan(&n); err != nil {
			t.Errorf("rival read: %v", err)
		}
	}
	// Without a conflict the closure runs once: that is how many times it
	// asks for the time per attempt.
	armed.Store(1)
	if _, err := master.maps.CreateMap(t.Context(), connect.NewRequest(&mapsv1.CreateMapRequest{CampaignId: campaign, Name: "Sem conflito", ImageId: x})); err != nil {
		t.Fatalf("CreateMap() without a conflict error = %v", err)
	}
	perAttempt := calls.Swap(0)
	armed.Store(2)
	_, err = master.maps.CreateMap(t.Context(), connect.NewRequest(&mapsv1.CreateMapRequest{CampaignId: campaign, Name: "Copia", ImageId: x}))
	if err != nil {
		t.Errorf("CreateMap() error = %v, want success after the retry", err)
	}
	// The rival really made the first attempt fail: the closure ran again.
	if armed.Load() != 3 || calls.Load() <= perAttempt {
		t.Errorf("the closure asked for the time %d time(s) against %d for one attempt: the conflict was not provoked, so nothing was retried", calls.Load(), perAttempt)
	}
}
