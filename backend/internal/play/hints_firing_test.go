package play

import (
	"fmt"
	"slices"
	"testing"

	playv1 "github.com/PuraFome/meuRPG/backend/gen/meurpg/play/v1"
	"github.com/PuraFome/meuRPG/backend/internal/play/playdb"
)

// Which audience hears what after a trap fires (#735): afterFiring's branches and the fog split of publishEncounterChanged.
// The hints are only "read again", but who gets one is itself a leak: a player who hears a hint for every firing, or an
// encounter revision on a fog map, can count what happens in the dark. Each case reads the hints of the master, of Toren's
// player and of Pensantus's player, as the live hub delivers them.

// brief names an event for a comparison: its kind, and the revision of an encounter hint (the master's number, or 0).
func brief(ev *playv1.WatchGameSessionResponse) string {
	switch e := ev.GetEvent().(type) {
	case *playv1.WatchGameSessionResponse_EncounterChanged_:
		return fmt.Sprintf("encounter_changed rev=%d", e.EncounterChanged.GetRevision())
	case *playv1.WatchGameSessionResponse_CombatLogChanged_:
		return fmt.Sprintf("log_changed full=%v", e.CombatLogChanged.GetFull())
	case *playv1.WatchGameSessionResponse_MapChanged_:
		return "map_changed"
	}
	return fmt.Sprintf("%T", ev.GetEvent())
}

func briefs(evs []*playv1.WatchGameSessionResponse) []string {
	out := make([]string, 0, len(evs))
	for _, ev := range evs {
		out = append(out, brief(ev))
	}
	slices.Sort(out)
	return out
}

// firingEars are the three streams a firing is told to.
type firingEars struct {
	h                   *harness
	master, toren, pens heardBy
}

func (e firingEars) heard(t *testing.T) (master, toren, pens []string) {
	t.Helper()
	return briefs(e.master.heard(t, e.h)), briefs(e.toren.heard(t, e.h)), briefs(e.pens.heard(t, e.h))
}

func openEars(t *testing.T, a *armed) firingEars {
	t.Helper()
	a.h.svc.SetTraps(emptyTrapBook{}) // the trap's own hint is the maps module's: not what these tests read
	e := firingEars{
		h:      a.h,
		master: listen(t, a.h, a.campaignID, a.master.id, true),
		toren:  listen(t, a.h, a.campaignID, a.caio.id, false),
		pens:   listen(t, a.h, a.campaignID, a.ana.id, false),
	}
	e.heard(t) // what the setup left in the streams
	return e
}

// combatOf reads the campaign's open combat as the funnel keeps it.
func combatOf(t *testing.T, a *armed, id string) playdb.Encounter {
	t.Helper()
	sess, err := a.h.svc.queries.GetOpenGameSession(t.Context(), a.campaignID)
	if err != nil {
		t.Fatalf("GetOpenGameSession() error = %v", err)
	}
	enc, err := a.h.svc.queries.GetEncounterInSession(t.Context(), playdb.GetEncounterInSessionParams{GameSessionID: sess.ID, ID: id})
	if err != nil {
		t.Fatalf("GetEncounterInSession() error = %v", err)
	}
	if enc.Revision == 0 {
		t.Fatal("the combat's revision is 0: the fog split could not be told from a blind hint")
	}
	return enc
}

func want(t *testing.T, who string, got []string, wanted ...string) {
	t.Helper()
	slices.Sort(wanted)
	if !slices.Equal(got, wanted) && (len(got)+len(wanted) > 0) {
		t.Errorf("%s heard %q, want %q", who, got, wanted)
	}
}

// The branches of afterFiring with no fog on the combat's map: the master hears the trap's map change always; a firing outside
// a combat tells no combat hint to anyone; a firing that only adds creatures the players are not given tells the master alone;
// any other firing tells everyone to read the combat and its log again.
func TestAfterFiringTellsEachAudienceOnlyWhatItMayCount(t *testing.T) {
	x := newCoreFixture(t)
	ears := openEars(t, x.armed)
	enc := combatOf(t, x.armed, x.e.GetId())
	enc.MapID = nil // a combat with no map has no fog
	rev := fmt.Sprintf("encounter_changed rev=%d", enc.Revision)
	fire := func(fired *trapFireEvent, enc playdb.Encounter) {
		t.Helper()
		x.h.svc.afterFiring(t.Context(), x.campaignID, fired, enc)
	}
	trap := func(extends string, caught ...trapCaughtEvent) *trapFireEvent {
		return &trapFireEvent{MapID: "map-1", PointID: "point-1", ExtendsID: extends, Caught: caught}
	}

	t.Run("outside a combat", func(t *testing.T) {
		fire(trap("", trapCaughtEvent{Target: "c1"}), playdb.Encounter{})
		m, a, b := ears.heard(t)
		want(t, "the master", m, "map_changed")
		want(t, "Toren's player", a)
		want(t, "Pensantus's player", b)
	})
	t.Run("a nil firing tells nobody", func(t *testing.T) {
		fire(nil, enc)
		m, a, b := ears.heard(t)
		want(t, "the master", m)
		want(t, "Toren's player", a)
		want(t, "Pensantus's player", b)
	})
	masterOnly := map[string]*trapFireEvent{
		"creatures the master hid":                  trap("first", trapCaughtEvent{Target: "n1", Hidden: true}, trapCaughtEvent{Target: "n2", Hidden: true}),
		"creatures on a fog map no player saw then": trap("first", trapCaughtEvent{Target: "n1", Fogged: true}),
	}
	for name, fired := range masterOnly {
		t.Run("an addition of "+name, func(t *testing.T) {
			if !fired.addsNothingForAnyPlayer() {
				t.Fatal("the fixture's firing is not one that adds nothing for any player")
			}
			fire(fired, enc)
			m, a, b := ears.heard(t)
			want(t, "the master", m, "map_changed", rev, "log_changed full=false")
			want(t, "Toren's player", a)
			want(t, "Pensantus's player", b)
		})
	}
	everyone := map[string]*trapFireEvent{
		"a first firing":                           trap("", trapCaughtEvent{Target: "c1"}),
		"a firing that catches no one":             trap(""),
		"an addition of a creature players see":    trap("first", trapCaughtEvent{Target: "n1", Hidden: true}, trapCaughtEvent{Target: "n2"}),
		"an addition of a creature one player saw": trap("first", trapCaughtEvent{Target: "n1", Fogged: true, SeenBy: []string{"someone"}}),
		"an extension that catches no one":         trap("first"),
	}
	for name, fired := range everyone {
		t.Run(name, func(t *testing.T) {
			if fired.addsNothingForAnyPlayer() {
				t.Fatal("the fixture's firing adds nothing for any player: it belongs to the master-only cases")
			}
			fire(fired, enc)
			m, a, b := ears.heard(t)
			for who, got := range map[string][]string{"the master": m, "Toren's player": a, "Pensantus's player": b} {
				wantEvents := []string{rev, "log_changed full=false"}
				if who == "the master" {
					wantEvents = append(wantEvents, "map_changed")
				}
				want(t, who, got, wantEvents...)
			}
		})
	}
}

// On a map with the fog of war the revision is the master's alone: a firing in a combat tells the players "read again" with
// revision 0 and the master the real number (publishEncounterChanged's split), and a firing that adds nothing for any
// player tells the master alone. A combat that ended has no fog: everyone gets the real revision.
func TestAfterFiringOnAFogMapTellsThePlayersNoRevision(t *testing.T) {
	f := newFogCave(t)
	e := f.fight(t)
	ears := openEars(t, f.armed)
	enc := combatOf(t, f.armed, e.GetId())
	if enc.MapID == nil {
		t.Fatal("the combat has no map: the fog cannot split anything")
	}
	rev := fmt.Sprintf("encounter_changed rev=%d", enc.Revision)
	fire := func(fired *trapFireEvent, enc playdb.Encounter) {
		t.Helper()
		f.h.svc.afterFiring(t.Context(), f.campaignID, fired, enc)
	}

	t.Run("a firing the players may count", func(t *testing.T) {
		fire(&trapFireEvent{MapID: *enc.MapID, PointID: "point-1", Caught: []trapCaughtEvent{{Target: "c1"}}}, enc)
		m, a, b := ears.heard(t)
		want(t, "the master", m, "map_changed", rev, "log_changed full=false")
		for who, got := range map[string][]string{"Toren's player": a, "Pensantus's player": b} {
			want(t, who, got, "encounter_changed rev=0", "log_changed full=false")
		}
	})
	t.Run("an addition no player is given", func(t *testing.T) {
		fire(&trapFireEvent{MapID: *enc.MapID, PointID: "point-1", ExtendsID: "first", Caught: []trapCaughtEvent{{Target: "n1", Fogged: true}}}, enc)
		m, a, b := ears.heard(t)
		want(t, "the master", m, "map_changed", rev, "log_changed full=false")
		want(t, "Toren's player", a)
		want(t, "Pensantus's player", b)
	})
	t.Run("a combat that ended has no fog", func(t *testing.T) {
		over := enc
		over.Status = statusEnded
		fire(&trapFireEvent{MapID: *enc.MapID, PointID: "point-1", Caught: []trapCaughtEvent{{Target: "c1"}}}, over)
		m, a, b := ears.heard(t)
		want(t, "the master", m, "map_changed", rev, "log_changed full=false")
		for who, got := range map[string][]string{"Toren's player": a, "Pensantus's player": b} {
			want(t, who, got, rev, "log_changed full=false")
		}
	})
}
