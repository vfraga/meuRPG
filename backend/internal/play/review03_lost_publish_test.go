package play

import (
	"context"
	"log/slog"
	"sync/atomic"
	"testing"
	"time"

	"connectrpc.com/connect"

	playv1 "github.com/PuraFome/meuRPG/backend/gen/meurpg/play/v1"
)

// Review finding U3-4: a committed EndTurn whose request context dies before finish() publishes is never announced, and the retry (res.repeated) does not announce it either.

// cancelOnEvent cancels the client's call when the DEBUG event line of name is
// logged (after the commit, before finish), and waits for the server's request
// context to notice.
type cancelOnEvent struct {
	name   string
	armed  *atomic.Bool
	cancel *atomic.Pointer[context.CancelFunc]
}

func (h cancelOnEvent) Enabled(context.Context, slog.Level) bool { return true }
func (h cancelOnEvent) WithAttrs([]slog.Attr) slog.Handler       { return h }
func (h cancelOnEvent) WithGroup(string) slog.Handler            { return h }
func (h cancelOnEvent) Handle(ctx context.Context, r slog.Record) error {
	if !h.armed.Load() {
		return nil
	}
	r.Attrs(func(a slog.Attr) bool {
		if a.Key == "event" && a.Value.String() == h.name {
			if c := h.cancel.Load(); c != nil {
				(*c)()
			}
			select {
			case <-ctx.Done():
			case <-time.After(5 * time.Second):
			}
			return false
		}
		return true
	})
	return nil
}

func TestReview3_CommittedChangeIsStillAnnounced(t *testing.T) {
	t.Parallel()
	a := newArmed(t)
	var armedFlag atomic.Bool
	var cancel atomic.Pointer[context.CancelFunc]
	a.h.svc.logger = slog.New(cancelOnEvent{name: "combat.turn_passed", armed: &armedFlag, cancel: &cancel}) // before any stream starts
	e := a.start(t, plan{
		npcs:     []*playv1.Participant{{CharacterId: a.goblin.GetId(), Hidden: new(false)}},
		npcRolls: []int{20},
		players:  map[string]int32{"Toren": 10, "Pensantus": 5, "Brisa": 1},
		at:       map[string][2]int32{"Toren": {3, 3}, "Goblin": {4, 3}, "Pensantus": {10, 3}, "Brisa": {8, 8}},
	})
	masterStream, playerStream := a.master.watch(t, a.campaignID), a.ana.watch(t, a.campaignID)
	masterStream.ready(t)
	playerStream.ready(t)

	cur := a.get(t, a.master).GetCurrentCombatantId()
	req := func() *connect.Request[playv1.EndTurnRequest] {
		return connect.NewRequest(&playv1.EndTurnRequest{CampaignId: a.campaignID, EncounterId: e.GetId(), IdempotencyKey: newKey(), ExpectedCombatantId: cur})
	}
	first := req()
	retry := connect.NewRequest(&playv1.EndTurnRequest{CampaignId: a.campaignID, EncounterId: e.GetId(), IdempotencyKey: first.Msg.GetIdempotencyKey(), ExpectedCombatantId: cur})
	ctx, c := context.WithCancel(t.Context())
	cancel.Store(&c)
	armedFlag.Store(true)
	if _, err := a.master.combat.EndTurn(ctx, first); err == nil {
		t.Fatal("first EndTurn: want a context error, got none (the hook did not cancel)")
	}
	armedFlag.Store(false)
	// The turn was committed: the retry answers.
	if _, err := a.master.combat.EndTurn(t.Context(), retry); err != nil {
		t.Fatalf("retry EndTurn error = %v, want the answer of the committed change", err)
	}
	for name, w := range map[string]*watcher{"master": masterStream, "player": playerStream} {
		select {
		case <-w.events:
		case <-time.After(2 * time.Second):
			t.Errorf("%s's stream got no event for the committed turn change", name)
		}
	}
}
