package play

import (
	"testing"

	"connectrpc.com/connect"

	playv1 "github.com/PuraFome/meuRPG/backend/gen/meurpg/play/v1"
)

// The free dice roller (#735 part 2, 4d) goes through Service.tx: a roll moves the session's number once, a table roll is told to the
// table with the number, a secret roll is told to nobody (the number moves all the same), and a repeat of the key tells again.

const kFreeRoll = "*playv1.WatchGameSessionResponse_FreeRollRolled_"

func (a *armed) freeRollCall(u *user, key string, edit ...func(*playv1.RollFreeDiceRequest)) func() error {
	return func() error {
		_, err := a.rollFreeAs(u, append([]func(*playv1.RollFreeDiceRequest){func(r *playv1.RollFreeDiceRequest) {
			if key != "" {
				r.IdempotencyKey = key
			}
		}}, edit...)...)
		return err
	}
}

func TestRollFreeDiceHints(t *testing.T) {
	t.Parallel()
	a := newArmed(t)
	g := a.stageHints(t)
	heard := g.first(a.freeRollCall(a.ana, ""), true)
	heardKinds(t, heard, map[string][]string{"master": {kFreeRoll}, "Ana": {kFreeRoll}, "Caio": {kFreeRoll}, "Bia": {kFreeRoll}})
}

func TestRollFreeDiceSecretMovesTheNumberAndTellsNobody(t *testing.T) {
	t.Parallel()
	a := newArmed(t)
	g := a.stageHints(t, "Ana", "Caio", "Bia")
	heard := g.first(a.freeRollCall(a.master, "", secretRoll), false)
	heardKinds(t, heard, map[string][]string{})
}

func TestRollFreeDiceKeyedHints(t *testing.T) {
	t.Parallel()
	a := newArmed(t)
	g := a.stageHints(t)
	key := newKey()
	g.keyed(a.freeRollCall(a.ana, key), a.freeRollCall(a.ana, key), a.freeRollCall(a.ana, key, func(r *playv1.RollFreeDiceRequest) { r.Sides = 8 }))
}

func TestRollFreeDiceSecretKeyedTellsNobodyAgain(t *testing.T) {
	t.Parallel()
	a := newArmed(t)
	g := a.stageHints(t, "Ana", "Caio", "Bia")
	key := newKey()
	do(t, a.freeRollCall(a.master, key, secretRoll))
	g.drain()
	before := g.seq()
	do(t, a.freeRollCall(a.master, key, secretRoll))
	if got := g.seq(); got != before {
		t.Errorf("hint_seq = %d, want %d: the repeat moves nothing", got, before)
	}
	for name, evs := range g.drain() {
		if len(evs) != 0 {
			t.Errorf("%s heard %v of a secret roll", name, eventKinds(evs))
		}
	}
}

func TestRollFreeDiceLostHints(t *testing.T) {
	t.Parallel()
	a := newArmed(t)
	g := a.stageHints(t)
	g.lost(a.freeRollCall(a.ana, newKey()))
}

func TestRollFreeDiceRefusedTellsNothing(t *testing.T) {
	t.Parallel()
	a := newArmed(t)
	g := a.stageHints(t)
	g.refused(a.freeRollCall(a.ana, "", secretRoll)) // a player may not roll in secret
}

func TestRollFreeDiceWithNoOpenSessionRefusesAsBefore(t *testing.T) {
	t.Parallel()
	a := newArmed(t)
	a.master.end(t, a.master.liveSession(t, a.campaignID).GetGameSession())
	if err := a.freeRollCall(a.ana, "")(); connect.CodeOf(err) != connect.CodeFailedPrecondition {
		t.Fatalf("RollFreeDice(no open session) error = %v, want failed_precondition", err)
	}
}
