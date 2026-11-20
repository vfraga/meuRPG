package play

import (
	"testing"
	"time"

	playv1 "github.com/PuraFome/meuRPG/backend/gen/meurpg/play/v1"
)

// The wait mark of a READIED window as the server keeps it (combat_reaction_rpc.go, markRolling; combat_reaction_view.go):
// the clock of the wait is the server's, and a mark that changes nothing writes and tells nothing.

// The wait's milliseconds are the server's: the master and the reactor's player read now minus the first mark, 0 before it, the
// same wait whoever reads, growing as the clock does and never moved by a second mark or a clear.
func TestTheWaitOfAReadiedWindowCountsOnTheServersClock(t *testing.T) {
	t.Parallel()
	a := newCasters(t)
	e := a.readyForRelease(t, magicMissileSpell, 1, "Capitão Goblin")
	w := a.mustFire(t, e, "Pensantus")
	waited := func(u *user) *playv1.ReactionWindow {
		t.Helper()
		win := a.windowOf(t, u, kindReadied)
		if win == nil {
			t.Fatalf("%s reads no READIED window", u.id)
		}
		return win
	}
	a.h.clock.advance(7 * time.Second)
	if got := waited(a.master).GetWaitedMs(); got != 0 {
		t.Fatalf("waited_ms before any mark = %d, want 0", got)
	}
	if err := a.markRolling(t, a.ana, e, w.GetId()); err != nil {
		t.Fatalf("the first mark: %v", err)
	}
	if got := waited(a.master).GetWaitedMs(); got != 0 {
		t.Errorf("waited_ms at the mark = %d, want 0", got)
	}
	a.h.clock.advance(3 * time.Second)
	if err := a.markRolling(t, a.ana, e, w.GetId()); err != nil { // a second mark moves nothing
		t.Fatalf("the second mark: %v", err)
	}
	for _, u := range []*user{a.master, a.ana} {
		win := waited(u)
		if got := win.GetWaitedMs(); got != 3000 || a.h.clock.Now().Sub(win.GetWaitBeganAt().AsTime()).Milliseconds() != got {
			t.Errorf("%s reads waited_ms %d, began %v, now %v; want 3000 and now - began", u.id, got, win.GetWaitBeganAt().AsTime(), a.h.clock.Now())
		}
	}
	a.h.clock.advance(4 * time.Second)
	if err := a.clearRolling(t, a.ana, e, w.GetId()); err != nil { // a clear keeps the wait running
		t.Fatalf("the clear: %v", err)
	}
	if got := waited(a.master).GetWaitedMs(); got != 7000 {
		t.Errorf("waited_ms after a clear = %d, want 7000", got)
	}
}

// A mark that changes nothing (set when it is set, cleared when it is clear, or cleared before any mark) writes nothing and tells
// nothing; one that changes it tells the master and the reactor's player alone, never another player (they would count the
// hints of a wait they do not read).
func TestAMarkThatChangesNothingTellsNothingAndOneThatDoesTellsTwo(t *testing.T) {
	t.Parallel()
	a := newCasters(t)
	e := a.readyForRelease(t, magicMissileSpell, 1, "Capitão Goblin")
	w := a.mustFire(t, e, "Pensantus")
	campaign := a.campaignID
	var sid string
	if err := a.h.pool.QueryRow(t.Context(), `SELECT id FROM game_sessions WHERE campaign_id = $1 AND ended_at IS NULL`, campaign).Scan(&sid); err != nil {
		t.Fatalf("find the open session: %v", err)
	}
	master, ana, caio := listen(t, a.h, campaign, a.master.id, true), listen(t, a.h, campaign, a.ana.id, false), listen(t, a.h, campaign, a.caio.id, false)
	heard := func(h heardBy) []string { return eventKinds(h.heard(t, a.h)) }
	steps := []struct {
		name    string
		do      func() error
		changes bool
	}{
		{"a clear before any mark", func() error { return a.clearRolling(t, a.ana, e, w.GetId()) }, false},
		{"the first mark", func() error { return a.markRolling(t, a.ana, e, w.GetId()) }, true},
		{"a second mark", func() error { return a.markRolling(t, a.ana, e, w.GetId()) }, false},
		{"a clear", func() error { return a.clearRolling(t, a.ana, e, w.GetId()) }, true},
		{"a second clear", func() error { return a.clearRolling(t, a.ana, e, w.GetId()) }, false},
		{"a mark after the clear", func() error { return a.markRolling(t, a.ana, e, w.GetId()) }, true},
	}
	heard(master)
	heard(ana)
	heard(caio)
	for _, s := range steps {
		before := a.h.hintSeq(t, sid)
		rev := a.get(t, a.master).GetRevision()
		if err := s.do(); err != nil {
			t.Fatalf("%s: %v", s.name, err)
		}
		m, p, o := heard(master), heard(ana), heard(caio)
		if s.changes {
			if len(m) != 1 || m[0] != kEncChanged || len(p) != 1 || p[0] != kEncChanged || len(o) != 0 {
				t.Errorf("%s: the master heard %v, the reactor's player %v, another player %v; want one encounter change for the first two and nothing for the last", s.name, m, p, o)
			}
			continue
		}
		if len(m)+len(p)+len(o) != 0 || a.h.hintSeq(t, sid) != before || a.get(t, a.master).GetRevision() != rev {
			t.Errorf("%s: heard %v %v %v, hint number %d (was %d), revision %d (was %d); want nothing told and nothing written",
				s.name, m, p, o, a.h.hintSeq(t, sid), before, a.get(t, a.master).GetRevision(), rev)
		}
	}
}

// On a fog map the players read the encounter's hint blind (revision 0): a player who read the real revision could count the
// moves in the dark. The reactor's player is told of a mark as every player is, so a mark tells the master the real number and
// the reactor's player none.
func TestAMarkOnAFogMapTellsTheReactorsPlayerTheBlindHint(t *testing.T) {
	t.Parallel()
	f := newFogCave(t)
	f.fightInSeparateTurns(t)
	e := f.passTo(t, f.get(t, f.master), "Pensantus")
	f.mustReady(t, f.ana, f.readySpell(t, e, "Pensantus", magicMissileSpell, 1, ""))
	e = f.mustEndTurn(t, f.master, e)
	w := f.mustFire(t, e, "Pensantus")
	master, ana := listen(t, f.h, f.campaignID, f.master.id, true), listen(t, f.h, f.campaignID, f.ana.id, false)
	master.heard(t, f.h)
	ana.heard(t, f.h)
	if err := f.markRolling(t, f.ana, e, w.GetId()); err != nil {
		t.Fatalf("the mark: %v", err)
	}
	revisionOf := func(who string, h heardBy) int32 {
		t.Helper()
		var got []int32
		for _, m := range h.heard(t, f.h) {
			if c := m.GetEncounterChanged(); c != nil {
				got = append(got, c.GetRevision())
			}
		}
		if len(got) != 1 {
			t.Fatalf("%s heard %d encounter hints, want 1", who, len(got))
		}
		return got[0]
	}
	actual := f.get(t, f.master).GetRevision()
	if got := revisionOf("the master", master); got != actual || actual == 0 {
		t.Errorf("the master's hint carries revision %d, want the real %d (not 0)", got, actual)
	}
	if got := revisionOf("the reactor's player", ana); got != 0 {
		t.Errorf("the reactor's player's hint carries revision %d on a fog map, want 0 (blind)", got)
	}
}
