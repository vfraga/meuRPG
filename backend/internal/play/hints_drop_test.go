package play

import (
	"sync/atomic"
	"testing"
	"time"

	"connectrpc.com/connect"

	playv1 "github.com/PuraFome/meuRPG/backend/gen/meurpg/play/v1"
	"github.com/PuraFome/meuRPG/backend/internal/authz"
	"github.com/PuraFome/meuRPG/backend/internal/play/live"
)

// The "drop a hint on purpose" test (#735): a combat write commits and its events are lost on the way to the streams (the hook
// drops every send while it is on). A player's stream then hears nothing of it, but its next heartbeat carries the session's
// stored number, which is ahead of the one Ready gave, so the page knows it is behind and reads again.

// dropWorld is a table with a player watching, a short heartbeat, and a switch that loses the events the service sends.
type dropWorld struct {
	*coreFixture
	player  *watcher
	session string
	ready   int64        // the number the player's stream started with
	on      atomic.Bool  // while true, send drops
	dropped atomic.Int32 // how many events were dropped
}

func newDropWorld(t *testing.T) *dropWorld {
	t.Helper()
	w := &dropWorld{coreFixture: newCoreFixture(t)}
	w.h.svc.live.Heartbeat = 10 * time.Millisecond // before any stream is open
	open, err := w.h.svc.queries.GetOpenGameSession(t.Context(), w.campaignID)
	if err != nil {
		t.Fatalf("GetOpenGameSession() error = %v", err)
	}
	w.session = open.ID
	w.h.svc.dropSend = func(string, live.Event) bool {
		if w.on.Load() {
			w.dropped.Add(1)
			return true
		}
		return false
	}
	w.h.hints.Settle(t) // the start's trailing hints are sent: the stream opens after them
	w.player = w.caio.watch(t, w.campaignID)
	ev := w.player.next(t)
	if ev.GetReady() == nil || ev.GetReady().GetSessionSeq() != w.h.hintSeq(t, w.session) || ev.GetReady().GetSessionSeq() == 0 {
		t.Fatalf("first event = %v, want ready with the stored number (not 0)", ev)
	}
	w.ready = ev.GetReady().GetSessionSeq()
	return w
}

// torenHit is a combat change that takes 3 hit points from Toren, as a damage does, and tells the vitals it left. It commits
// through write, the one door of the combat's changes, so its events go out stamped with the number it stored.
func (w *dropWorld) torenHit(t *testing.T, key string) {
	t.Helper()
	ctx := t.Context()
	torenID := w.toren.GetId()
	hp := w.vitals(t, w.toren).GetHitPointsCurrent() - 3
	m := authz.Membership{UserID: w.master.id, CampaignID: w.campaignID, Role: authz.RoleMaster}
	_, err := w.h.svc.write(ctx, combatWrite{m: m, key: key, kind: eventHitPointsAdjusted, encounterID: w.e.GetId()}, func(c *combatTx) (any, error) {
		_, after, err := w.h.svc.vitalsOf(ctx, c, torenID, &playv1.AdjustCharacterVitalsRequest{HitPointsCurrent: &hp})
		if err != nil {
			return nil, err
		}
		c.told = append(c.told, after)
		return map[string]any{"encounter_id": w.e.GetId()}, nil
	})
	if err != nil {
		t.Fatalf("write() error = %v", err)
	}
	w.h.hints.Settle(t)
}

// goblinHit is the master's hand on Goblin's hit points: the RPC, whose events the call publishes once it returns.
func (w *dropWorld) goblinHit(t *testing.T, key string) {
	t.Helper()
	_, err := w.master.combat.AdjustCombatantHitPoints(t.Context(), connect.NewRequest(&playv1.AdjustCombatantHitPointsRequest{
		CampaignId: w.campaignID, EncounterId: w.e.GetId(), CombatantId: w.id(t, "Goblin 1"), IdempotencyKey: key,
		Change: &playv1.AdjustCombatantHitPointsRequest_Damage{Damage: 2},
	}))
	if err != nil {
		t.Fatalf("AdjustCombatantHitPoints() error = %v", err)
	}
	w.h.hints.Settle(t)
}

// hit makes the change of one of the two kinds: a player's vitals through write, or an NPC's hit points by the master's hand.
func (w *dropWorld) hit(t *testing.T, npc bool) {
	t.Helper()
	if npc {
		w.goblinHit(t, newKey())
		return
	}
	w.torenHit(t, newKey())
}

// hitPoints reads what hit changes.
func (w *dropWorld) hitPoints(t *testing.T, npc bool) int32 {
	t.Helper()
	if npc {
		hp, _, _ := w.hp(t, "Goblin 1")
		return hp
	}
	return w.vitals(t, w.toren).GetHitPointsCurrent()
}

// heartbeatPast reads the player's stream up to a heartbeat whose number is above floor and returns that number. Anything but a
// heartbeat on the way fails the test: the player must have heard nothing.
func (w *dropWorld) heartbeatPast(t *testing.T, floor int64) int64 {
	t.Helper()
	limit := time.After(waitLimit) // the heartbeats keep coming: a number that never moves must fail, not spin
	for {
		select {
		case <-limit:
			t.Fatalf("no heartbeat above %d within the time limit", floor)
		default:
		}
		ev := w.player.next(t)
		hb := ev.GetHeartbeat()
		if hb == nil {
			t.Fatalf("the player heard %T, want only heartbeats", ev.GetEvent())
		}
		if ev.GetSessionSeq() != hb.GetSessionSeq() {
			t.Fatalf("heartbeat stamp %d and number %d differ", ev.GetSessionSeq(), hb.GetSessionSeq())
		}
		if hb.GetSessionSeq() > floor {
			return hb.GetSessionSeq()
		}
	}
}

// nextHeartbeat is the heartbeat after the one just read.
func (w *dropWorld) nextHeartbeat(t *testing.T) int64 {
	t.Helper()
	ev := w.player.next(t)
	if ev.GetHeartbeat() == nil {
		t.Fatalf("the player heard %T, want a heartbeat", ev.GetEvent())
	}
	return ev.GetHeartbeat().GetSessionSeq()
}

// A hit-point change whose events are lost: committed, unheard, and the heartbeat is ahead of Ready and equal to the stored number,
// and stays there. Two changes: a player's vitals through write, and the master's hand on an NPC through its RPC.
func TestDroppedHintOfAHitPointChangeShowsAtTheNextHeartbeat(t *testing.T) {
	t.Parallel()
	cases := []struct {
		name string
		npc  bool
	}{{"a player's vitals", false}, {"an NPC by the master's hand", true}}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			w := newDropWorld(t)
			before := w.hitPoints(t, tc.npc)

			w.on.Store(true)
			w.hit(t, tc.npc)
			w.on.Store(false)

			if hp := w.hitPoints(t, tc.npc); hp >= before {
				t.Fatalf("hit points = %d, were %d: the write must commit", hp, before)
			}
			if w.dropped.Load() == 0 {
				t.Fatal("no event was dropped: the test would prove nothing")
			}
			stored := w.h.hintSeq(t, w.session)
			if stored <= w.ready {
				t.Fatalf("stored number = %d, want ahead of Ready's %d", stored, w.ready)
			}
			if got := w.heartbeatPast(t, w.ready); got != stored {
				t.Errorf("first heartbeat past Ready carries %d, want the stored %d", got, stored)
			}
			if got := w.nextHeartbeat(t); got != stored {
				t.Errorf("a second heartbeat after no write carries %d, want the same %d", got, stored)
			}
		})
	}
}

// Control: the same write with nothing dropped is heard by the player, stamped with the number the heartbeat carries.
func TestHeardHintOfAHitPointChangeCarriesTheHeartbeatNumber(t *testing.T) {
	t.Parallel()
	w := newDropWorld(t)

	w.torenHit(t, newKey())

	stored := w.h.hintSeq(t, w.session)
	if stored <= w.ready {
		t.Fatalf("stored number = %d, want ahead of Ready's %d", stored, w.ready)
	}
	if w.dropped.Load() != 0 {
		t.Fatalf("%d events dropped in the control", w.dropped.Load())
	}
	ev := w.player.nextChange(t)
	if ev.GetSessionSeq() != stored {
		t.Errorf("%T carries %d, want the stored %d", ev.GetEvent(), ev.GetSessionSeq(), stored)
	}
	for {
		next := w.player.next(t)
		if next.GetHeartbeat() == nil {
			if next.GetSessionSeq() != stored {
				t.Errorf("%T carries %d, want the stored %d", next.GetEvent(), next.GetSessionSeq(), stored)
			}
			continue
		}
		if next.GetHeartbeat().GetSessionSeq() == stored {
			return
		}
		t.Fatalf("heartbeat after the event carries %d, want %d", next.GetHeartbeat().GetSessionSeq(), stored)
	}
}

// A repeat of the key of a write whose events were lost sends them again, stamped with the stored number and without a new
// bump; the heartbeat before it already showed that number.
func TestReplayOfADroppedHitPointChangeSendsAgainWithTheStoredNumber(t *testing.T) {
	t.Parallel()
	w := newDropWorld(t)
	key := newKey()

	w.on.Store(true)
	w.torenHit(t, key)
	w.on.Store(false)
	stored := w.h.hintSeq(t, w.session)
	if w.dropped.Load() == 0 || stored <= w.ready {
		t.Fatalf("dropped %d events, stored %d, Ready %d: want a drop and a bump", w.dropped.Load(), stored, w.ready)
	}
	if got := w.heartbeatPast(t, w.ready); got != stored {
		t.Fatalf("heartbeat carries %d, want the stored %d", got, stored)
	}

	w.torenHit(t, key) // the repeat: nothing is dropped
	if got := w.h.hintSeq(t, w.session); got != stored {
		t.Errorf("hint_seq = %d after the repeat, want %d: no new bump", got, stored)
	}
	ev := w.player.nextChange(t)
	if ev.GetSessionSeq() != stored {
		t.Errorf("the replayed %T carries %d, want the stored %d", ev.GetEvent(), ev.GetSessionSeq(), stored)
	}
	for {
		next := w.player.next(t)
		if next.GetHeartbeat() == nil {
			if next.GetSessionSeq() != stored {
				t.Errorf("replayed %T carries %d, want %d", next.GetEvent(), next.GetSessionSeq(), stored)
			}
			continue
		}
		if next.GetHeartbeat().GetSessionSeq() != stored {
			t.Fatalf("heartbeat after the replay carries %d, want %d", next.GetHeartbeat().GetSessionSeq(), stored)
		}
		return
	}
}
