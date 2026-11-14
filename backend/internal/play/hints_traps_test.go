package play

import (
	"testing"

	"connectrpc.com/connect"

	mapsv1 "github.com/PuraFome/meuRPG/backend/gen/meurpg/maps/v1"
	playv1 "github.com/PuraFome/meuRPG/backend/gen/meurpg/play/v1"
)

// The writers of traps (a firing outside a combat, a token dropped on a trap, a search, the settling of a trap's damage) go
// through Service.tx (#735, part 2, 4c): the session's number moves once with the write, the events the play module tells carry
// it, a repeat of the key moves nothing and tells again, and a refused call tells nothing and moves nothing. The events of the
// maps module (map_changed to the players who learn of a trap, trap_noticed) carry no number: they are that module's own sending.

const (
	kMapChanged  = "*playv1.WatchGameSessionResponse_MapChanged_"
	kTrapNoticed = "*playv1.WatchGameSessionResponse_TrapNoticed_"
)

// trapRigHints is the rig of traps with a stream open for each person, made after the traps are on the map.
func trapRigHints(t *testing.T, r *trapRig, quiet ...string) *hintRig {
	t.Helper()
	g := newHintRig(t, r.h, r.campaignID, r.master, map[string]*user{"Caio": r.caio, "Ana": r.ana, "Bia": r.bia}, quiet...)
	g.unnumbered = []string{kMapChanged, kTrapNoticed}
	return g
}

// setupFire is the table with a pit Toren stands on, and the writer's call (repeatable: one key; the dice are rolled once) and
// its key used for another request.
func setupFire(t *testing.T) (r *trapRig, g *hintRig, call, other func() error) {
	t.Helper()
	r = newTrapRig(t)
	pitA := r.trap(t, "Poço A", 12, 7, pit("1d6"), manualTrap)
	r.place(t, r.toren.GetId(), 12, 7)
	r.place(t, r.pens.GetId(), 12, 7)
	g = trapRigHints(t, r)
	key := newKey()
	queued := false
	fire := func(who string) func() error {
		return func() error {
			if !queued {
				queued = true
				r.h.roller.queue(3)
			}
			_, err := r.fireWithKey(t, key, pitA, who)
			return err
		}
	}
	return r, g, fire(r.toren.GetId()), fire(r.pens.GetId())
}

func TestFireTrapOutsideCombatHints(t *testing.T) {
	t.Parallel()
	_, g, call, other := setupFire(t)
	g.keyed(call, call, other)
}

// The first call's events are all lost: its repeat tells them again, with the stored number.
func TestFireTrapOutsideCombatLostHints(t *testing.T) {
	t.Parallel()
	_, g, call, _ := setupFire(t)
	g.lost(call)
}

// A trap that fired is refused after the session was locked: nothing moves, nobody hears.
func TestFireTrapOutsideCombatRefusedTellsNothing(t *testing.T) {
	t.Parallel()
	r := newTrapRig(t)
	pitA := r.trap(t, "Poço A", 12, 7, pit("1d6"), manualTrap)
	r.place(t, r.toren.GetId(), 12, 7)
	r.h.roller.queue(3)
	if _, err := r.fireByHand(t, pitA, r.toren.GetId()); err != nil {
		t.Fatalf("FireTrap() error = %v", err)
	}
	g := trapRigHints(t, r)
	g.refused(func() error {
		_, err := r.fireByHand(t, pitA, r.toren.GetId())
		return err
	})
}

// A token dropped in the area of an armed trap fires it with no key, in a transaction of its own: the number moves once and the
// master hears that the map changed, with it.
func TestTokenDroppedFiresWithHints(t *testing.T) {
	t.Parallel()
	r := newTrapRig(t)
	r.place(t, r.toren.GetId(), 12, 7)
	armed := r.trap(t, "Poço de entrada", 12, 7, pit("1d6"))
	g := trapRigHints(t, r)
	r.h.roller.queue(3)
	g.first(func() error {
		return r.h.svc.TokenDropped(t.Context(), r.campaignID, r.mapID, r.toren.GetId(), r.master.id, trapSquare(12, 7))
	}, true)
	if got := r.stateOf(t, r.mapID, armed.GetId()); got != mapsv1.TrapState_TRAP_STATE_TRIGGERED {
		t.Errorf("the pit = %v after the drop, want triggered", got)
	}
}

// setupSearch is the table with a hidden trap next to Toren, and the writer's call (repeatable: one key) and its key used for
// another request. Only the searcher and the master hear of a search; the other players hear nothing.
func setupSearch(t *testing.T) (g *hintRig, call, other func() error) {
	t.Helper()
	r := newTrapRig(t)
	r.trap(t, "Fosso Dourado", 7, 7, func(s *mapsv1.TrapSpec) { s.NoticeDc = 10 })
	r.place(t, r.toren.GetId(), 8, 7)
	g = trapRigHints(t, r, "Ana", "Bia")
	key := newKey()
	search := func(skill playv1.TrapSearchSkill) func() error {
		return func() error {
			_, err := r.caio.play.SearchForTraps(t.Context(), connect.NewRequest(&playv1.SearchForTrapsRequest{
				CampaignId: r.campaignID, IdempotencyKey: key, Skill: skill, Roll: &playv1.SearchForTrapsRequest_D20Face{D20Face: 14},
			}))
			return err
		}
	}
	return g, search(perception), search(investigation)
}

func TestSearchForTrapsHints(t *testing.T) {
	t.Parallel()
	g, call, other := setupSearch(t)
	g.keyed(call, call, other)
}

// The first call's events are all lost: its repeat tells them again, with the stored number.
func TestSearchForTrapsLostHints(t *testing.T) {
	t.Parallel()
	g, call, _ := setupSearch(t)
	g.lost(call)
}

// A search from a character with no token on the map is refused after the session was locked: nothing moves, nobody hears.
func TestSearchForTrapsRefusedTellsNothing(t *testing.T) {
	t.Parallel()
	r := newTrapRig(t)
	r.trap(t, "Fosso Dourado", 7, 7)
	g := trapRigHints(t, r)
	g.refused(func() error {
		_, err := r.search(t, r.caio, perception, 14) // Toren has no token on the map
		return err
	})
}

// setupSettle is the table with a trap damage that waits for the master, and the writer's call (repeatable: one key) and its key
// used for another request (another amount).
func setupSettle(t *testing.T) (r *trapRig, g *hintRig, call, other func() error) {
	t.Helper()
	r = newTrapRig(t)
	pitA := r.trap(t, "Poço A", 12, 7, pit("1d6"), manualTrap)
	r.place(t, r.toren.GetId(), 12, 7)
	r.h.roller.queue(4)
	if _, err := r.fireByHand(t, pitA, r.toren.GetId()); err != nil {
		t.Fatalf("FireTrap() error = %v", err)
	}
	waiting := r.trapDamages(t)
	if len(waiting) != 1 {
		t.Fatalf("trap damages waiting = %d, want 1", len(waiting))
	}
	g = trapRigHints(t, r, "Ana", "Bia")
	key, other1 := newKey(), int32(1)
	return r, g, func() error {
			_, err := r.settleTrapDmg(t, true, key, waiting[0].GetId(), nil)
			return err
		}, func() error {
			_, err := r.settleTrapDmg(t, true, key, waiting[0].GetId(), &other1)
			return err
		}
}

func TestApplyTrapDamageHints(t *testing.T) {
	t.Parallel()
	_, g, call, other := setupSettle(t)
	g.keyed(call, call, other)
}

// The first call's events are all lost: its repeat tells them again, with the stored number.
func TestApplyTrapDamageLostHints(t *testing.T) {
	t.Parallel()
	_, g, call, _ := setupSettle(t)
	g.lost(call)
}

// A discard tells the master that the trap card has one damage less, and nothing else.
func TestDiscardTrapDamageHints(t *testing.T) {
	t.Parallel()
	r, _, _, _ := setupSettle(t)
	g := trapRigHints(t, r, "Caio", "Ana", "Bia")
	waiting := r.trapDamages(t)
	g.first(func() error {
		_, err := r.settleTrapDmg(t, false, newKey(), waiting[0].GetId(), nil)
		return err
	}, true)
}

// A damage that is not there is refused after the session was locked: nothing moves, nobody hears.
func TestApplyTrapDamageRefusedTellsNothing(t *testing.T) {
	t.Parallel()
	r, g, _, _ := setupSettle(t)
	g.refused(func() error {
		_, err := r.settleTrapDmg(t, true, newKey(), newKey(), nil)
		return err
	})
}

// A damage settled after the session ended is written (the damage outlives its session) and moves no session's number: nobody
// watches a session that is over.
func TestApplyTrapDamageWithNoOpenSession(t *testing.T) {
	t.Parallel()
	r, _, _, _ := setupSettle(t)
	waiting := r.trapDamages(t)
	ended, err := r.h.svc.queries.GetOpenGameSession(t.Context(), r.campaignID)
	if err != nil {
		t.Fatalf("GetOpenGameSession() error = %v", err)
	}
	r.master.end(t, r.master.liveSession(t, r.campaignID).GetGameSession())
	before := r.h.hintSeq(t, ended.ID)
	got, err := r.settleTrapDmg(t, true, newKey(), waiting[0].GetId(), nil)
	if err != nil || got.GetStatus() != playv1.PendingDamageStatus_PENDING_DAMAGE_STATUS_APPLIED {
		t.Fatalf("ApplyTrapDamage(no open session) = %v, %v; want it applied", got.GetStatus(), err)
	}
	if after := r.h.hintSeq(t, ended.ID); after != before {
		t.Errorf("hint_seq of the ended session = %d, want %d", after, before)
	}
}

// A search moves the session's number once and tells the players who did not search nothing, whether or not it found a trap: the
// bystanders' streams read the same in a world where Toren found the hidden trap and in its twin where he found nothing (the
// number, and silence).
func TestASearchReadsTheSameToTheOtherPlayersWhateverItFound(t *testing.T) {
	t.Parallel()
	bystanders := func(face int32) (moved int64, heard map[string][]string) {
		r := newTrapRig(t)
		r.trap(t, "Fosso Dourado", 7, 7, func(s *mapsv1.TrapSpec) { s.NoticeDc = 10 })
		r.place(t, r.toren.GetId(), 8, 7)
		g := trapRigHints(t, r, "Ana", "Bia")
		before := g.seq()
		if _, err := r.search(t, r.caio, perception, face); err != nil {
			t.Fatalf("SearchForTraps(%d) error = %v", face, err)
		}
		moved = g.seq() - before
		heard = kindsOf(g.drain())
		return moved, heard
	}
	foundMoved, found := bystanders(14)
	missedMoved, missed := bystanders(1)
	if foundMoved != 1 || missedMoved != 1 {
		t.Errorf("the number moved by %d (trap found) and %d (nothing found), want 1 and 1", foundMoved, missedMoved)
	}
	for _, who := range []string{"Ana", "Bia"} {
		if len(found[who]) != 0 || len(missed[who]) != 0 {
			t.Errorf("%s heard %v (trap found) and %v (nothing found), want nothing in both", who, found[who], missed[who])
		}
	}
}

// A combat move moves the session's number the same whether or not its passive notice found a trap, and tells the players who did not move
// nothing of it: the bystanders' streams read the same in a world where Toren noticed the trap and in its twin where he could not.
func TestAMoveReadsTheSameToTheOtherPlayersWhetherItNoticedATrapOrNot(t *testing.T) {
	t.Parallel()
	bystanders := func(noticeDC int32) (moved int64, heard map[string][]string) {
		r := newTrapRig(t)
		r.trap(t, "Fosso Dourado", 9, 7, func(s *mapsv1.TrapSpec) { s.NoticeDc = noticeDC })
		r.fight(t)
		g := trapRigHints(t, r, "Ana", "Bia")
		before := g.seq()
		r.mustMove(t, r.caio, "Toren", 7, 7)
		moved = g.seq() - before
		return moved, kindsOf(g.drain())
	}
	noticedMoved, noticed := bystanders(10)
	missedMoved, missed := bystanders(0)
	if noticedMoved != missedMoved {
		t.Errorf("the number moved by %d (trap noticed) and %d (none), want the same", noticedMoved, missedMoved)
	}
	for _, who := range []string{"Ana", "Bia"} {
		if len(noticed[who]) != len(missed[who]) {
			t.Errorf("%s heard %v (trap noticed) and %v (none), want the same", who, noticed[who], missed[who])
		}
	}
}

// The Search action of a combat whose d20 is kept for a question (a die the combatant holds) moves the combat's revision: the session's
// number moves once with it, and the combat's hint carries the number.
func TestAHeldSearchInACombatMovesTheNumber(t *testing.T) {
	t.Parallel()
	r := newTrapRig(t)
	r.trap(t, "Fosso Dourado", 7, 7, func(s *mapsv1.TrapSpec) { s.NoticeDc = 20 })
	e := r.fight(t)
	r.giveCombatantDie(t, "Toren", e)
	g := trapRigHints(t, r)
	g.first(func() error {
		held, err := r.trapSearchCall(t, r.caio, trapSearchTyped(perception, 16))
		if err == nil && held.GetHeldRollOffer().GetDie() == nil {
			t.Errorf("SearchForTraps() = %v, want the die's question", held)
		}
		return err
	}, true)
}
