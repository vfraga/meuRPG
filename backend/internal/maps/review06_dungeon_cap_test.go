package maps

import (
	"testing"
	"time"

	"connectrpc.com/connect"
)

// Finding U6-E: CreateDungeonMap renders the dungeon (holding the server-wide processing slot)
// and writes the blobs before the transaction that checks the map cap and the gallery quota.
// A campaign already at its cap must be refused before any render. The test holds the
// processing slot: a refusal decided before the render answers at once; one decided after it
// blocks waiting for the slot.
func TestReview6_DungeonCapIsCheckedBeforeRendering(t *testing.T) {
	t.Parallel()
	cases := map[string]func(*Config){
		"map cap":       func(c *Config) { c.MaxMaps = 1 },
		"gallery quota": func(c *Config) { c.MaxImages = 1 },
	}
	for name, configure := range cases {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			d := newDungeonTable(t, configure)
			seed, _ := testDungeonSeed(t)
			if name == "map cap" {
				d.master.createDungeon(d.campaign, "Primeira", testDungeonOptions(), seed)
			} else {
				d.master.newImage(d.campaign)
			}
			// Another request holds the server's one image-processing slot.
			d.h.svc.processing <- struct{}{}
			released := false
			release := func() {
				if !released {
					released = true
					<-d.h.svc.processing
				}
			}
			defer release()

			type result struct{ err error }
			done := make(chan result, 1)
			go func() {
				_, err := d.master.tryCreateDungeon(d.campaign, "Segunda", testDungeonOptions(), &seed)
				done <- result{err}
			}()
			select {
			case r := <-done:
				wantCode(t, "over the limit", r.err, connect.CodeResourceExhausted)
			case <-time.After(2 * time.Second):
				release()
				r := <-done
				t.Errorf("the refusal (%v) waited for the processing slot: the dungeon is rendered before the %s is checked", r.err, name)
			}
		})
	}
}
