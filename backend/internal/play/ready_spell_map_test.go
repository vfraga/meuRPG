package play

import (
	"slices"
	"testing"
	"time"

	"connectrpc.com/connect"
	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/types/known/timestamppb"

	playv1 "github.com/PuraFome/meuRPG/backend/gen/meurpg/play/v1"
	"github.com/PuraFome/meuRPG/backend/internal/rules"
)

// A readied spell that needs the map (a point, a direction, a wall) is released with CastSpell by the readier's player
// or the master while the readier's READIED window is open (#857). These tests are the server side of that path.

// readiedInTheCave readies the spell at 1st level on Pensantus's turn in the area cave, spends every slot he has left,
// and gives the turn on: the row waits and its window is not fired yet.
func (c *cave) readiedInTheCave(t *testing.T, spell string) *playv1.Encounter {
	t.Helper()
	e := c.areaFight(t)
	c.mustReady(t, c.ana, c.readySpell(t, e, "Pensantus", spell, 1, ""))
	c.execSQL(t, `UPDATE character_vitals SET spell_slots_used = '{9,9,9}' WHERE character_id = $1`, c.pens.GetId())
	return c.mustEndTurn(t, c.master, e)
}

// While the READIED window is open the readier previews the held spell's area at the held slot, with no free slot of that
// level left and no slot in the request; with no open window the preview is refused as before.
func TestAReadierPreviewsTheAreaOfItsHeldSpellWhileTheWindowIsOpen(t *testing.T) {
	t.Parallel()
	for _, tt := range []struct {
		name, spell string
		origin      *playv1.SpellOrigin
		dir         *playv1.SpellDirection
	}{
		{"a point", sleepSpell, at(20, 7), nil},
		{"a direction", burningHands, nil, toward(1, 0)},
	} {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			c := newAreaCave(t)
			e := c.readiedInTheCave(t, tt.spell)
			_, err := c.preview(t, c.ana, "Pensantus", tt.spell, nil, tt.origin, tt.dir)
			wantCode(t, "a preview while the row waits and no window is open", err, connect.CodeInvalidArgument)
			w := c.mustFire(t, e, "Pensantus")
			for who, u := range map[string]*user{"the readier's player": c.ana, "the master": c.master} {
				res, err := c.preview(t, u, "Pensantus", tt.spell, nil, tt.origin, tt.dir)
				if err != nil || len(res.GetSquares()) == 0 {
					t.Errorf("%s: preview = %v, %v; want the area of the held spell", who, res, err)
				}
			}
			if _, err := c.preview(t, c.ana, "Pensantus", tt.spell, slotOfLevel(3), tt.origin, tt.dir); err != nil {
				t.Errorf("a request slot is not read for the held spell: error = %v", err)
			}
			if _, err := c.preview(t, c.ana, "Pensantus", fireball, slotOfLevel(3), at(20, 7), nil); err == nil {
				t.Error("another spell, with no free slot, was previewed through the window")
			}
			c.mustAnswer(t, c.ana, e, w.GetId(), passAnswer)
			_, err = c.preview(t, c.ana, "Pensantus", tt.spell, nil, tt.origin, tt.dir)
			wantCode(t, "a preview once the window is closed", err, connect.CodeInvalidArgument)
		})
	}
}

// A held point spell (Sleep) is released with CastSpell and a point of the map: the held slot, no other slot spent (every
// slot of the caster is used), the reaction spent, the row fired and the window closed.
func TestAReadiedPointSpellIsReleasedWithCastSpellAtAPointOfTheMap(t *testing.T) {
	t.Parallel()
	c := newAreaCave(t)
	e := c.readiedInTheCave(t, sleepSpell)
	c.mustFire(t, e, "Pensantus")
	slots := c.slotsOf(t, "Pensantus", 1, 2, 3)
	if _, err := c.castArea(t, c.ana, "Pensantus", sleepSpell, nil, at(20, 7), nil, poolSum(20)); err != nil {
		t.Fatalf("CastSpell() error = %v", err)
	}
	if cast := c.castLine(t, e, sleepSpell); cast == nil || cast.GetSpell().GetSlot().GetLevel() != 1 {
		t.Errorf("the release's line = %v, want Sleep at 1st level", cast)
	}
	if got := c.slotsOf(t, "Pensantus", 1, 2, 3); !slices.Equal(got, slots) {
		t.Errorf("slots used by level = %v, want %v: the release spends no slot", got, slots)
	}
	if !byLabel(t, c.get(t, c.ana), "Pensantus").GetReactionUsed() || c.readiedStatus(t, "Pensantus") != readiedFired || c.readiedOpen(t, "Pensantus") {
		t.Errorf("after the release: reaction used %v, row %q, window open %v", byLabel(t, c.get(t, c.ana), "Pensantus").GetReactionUsed(), c.readiedStatus(t, "Pensantus"), c.readiedOpen(t, "Pensantus"))
	}
}

// A held spell that lays a zone (Web) is released with CastSpell and an origin: the zone is laid on the map, the held
// slot is not spent again, the reaction is spent and the row fired. The answer of the window would have refused it.
func TestAReadiedZoneSpellIsReleasedWithCastSpellAndLaysTheZone(t *testing.T) {
	t.Parallel()
	c := zoneCave(t)
	e := c.zoneFight(t)
	e = c.passTo(t, e, "Pensantus")
	c.mustReady(t, c.ana, c.readySpell(t, e, "Pensantus", webKey, 2, ""))
	e = c.mustEndTurn(t, c.master, e)
	c.mustFire(t, e, "Pensantus")
	if zs := zonesOf(c.get(t, c.master), webKey); len(zs) != 0 {
		t.Fatalf("Ready laid the zones %v", zs)
	}
	slots := c.slotsOf(t, "Pensantus", 2, 3)
	if _, err := c.castArea(t, c.ana, "Pensantus", webKey, nil, at(9, 7), nil); err != nil {
		t.Fatalf("CastSpell() error = %v", err)
	}
	if zs := zonesOf(c.get(t, c.master), webKey); len(zs) != 1 {
		t.Errorf("zones = %v, want the Web laid by the release", zs)
	}
	if got := c.slotsOf(t, "Pensantus", 2, 3); !slices.Equal(got, slots) {
		t.Errorf("slots used by level = %v, want %v: the release spends no slot", got, slots)
	}
	if !byLabel(t, c.get(t, c.ana), "Pensantus").GetReactionUsed() || c.readiedStatus(t, "Pensantus") != readiedFired {
		t.Errorf("after the release: reaction used %v, row %q", byLabel(t, c.get(t, c.ana), "Pensantus").GetReactionUsed(), c.readiedStatus(t, "Pensantus"))
	}
}

// slotsOf is the readier's used slots of the levels, as a line to compare before and after a release.
func (a *armed) slotsOf(t *testing.T, label string, levels ...int32) []int32 {
	t.Helper()
	out := make([]int32, len(levels))
	for i, l := range levels {
		out[i] = a.slotsUsed(t, label, l)
	}
	return out
}

// CastSpell for the held spell while the caster's READIED window is open is its release: it takes the slot of Ready
// (the request names none, or the same level), spends no slot, spends the reaction, answers the window and fires the row.
func TestCastSpellReleasesTheHeldSpellAtTheHeldSlot(t *testing.T) {
	t.Parallel()
	for _, tt := range []struct {
		name string
		slot *playv1.SpellSlot
	}{{"no slot", nil}, {"the held level", slotOfLevel(2)}} {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			a := newCasters(t)
			e := a.readyForRelease(t, magicMissileSpell, 2, "Capitão Goblin")
			w := a.mustFire(t, e, "Pensantus")
			slots := a.slotsOf(t, "Pensantus", 1, 2)
			res, err := a.cast(t, a.ana, e, "Pensantus", magicMissileSpell, tt.slot, []*playv1.SpellTarget{darts(a, t, "Capitão Goblin", 4)}, noCastRoll)
			if err != nil || res.GetCast() == nil {
				t.Fatalf("CastSpell() = %v, %v; want the release", res, err)
			}
			if got := a.slotsOf(t, "Pensantus", 1, 2); !slices.Equal(got, slots) {
				t.Errorf("slots used by level = %v, want %v: the release spends no slot", got, slots)
			}
			if cast := a.castLine(t, e, magicMissileSpell); cast == nil || cast.GetSpell().GetSlot().GetLevel() != 2 {
				t.Errorf("the release's line = %v, want Magic Missile at 2nd level", cast)
			}
			if !byLabel(t, a.get(t, a.ana), "Pensantus").GetReactionUsed() || a.readiedStatus(t, "Pensantus") != readiedFired {
				t.Errorf("after the release: reaction used %v, row %q", byLabel(t, a.get(t, a.ana), "Pensantus").GetReactionUsed(), a.readiedStatus(t, "Pensantus"))
			}
			if got := a.windowOf(t, a.master, kindReadied); got != nil {
				t.Errorf("the READIED window %v is still open", got)
			}
			if _, err := a.answerReaction(t, a.ana, e, w.GetId(), passAnswer); err == nil {
				t.Error("the answered window took another answer")
			}
		})
	}
}

// What CastSpell refuses while the READIED window is open, each time writing nothing: another spell, a slot of another
// level than the held one (lower or an upcast) and Metamagic. The release then still works.
func TestCastSpellRefusesWhatIsNotTheHeldReleaseWhileTheWindowIsOpen(t *testing.T) {
	t.Parallel()
	a := newCasters(t)
	e := a.readyForRelease(t, magicMissileSpell, 2, "Capitão Goblin")
	a.mustFire(t, e, "Pensantus")
	slots := a.slotsOf(t, "Pensantus", 1, 2)
	missiles := []*playv1.SpellTarget{darts(a, t, "Capitão Goblin", 4)}
	quickened := func(r *playv1.CastSpellRequest) {
		r.Metamagic = []*playv1.MetamagicChoice{{Key: rules.MetamagicQuickened}}
	}
	for _, tt := range []struct {
		name    string
		spell   string
		slot    *playv1.SpellSlot
		targets []*playv1.SpellTarget
		roll    func(*playv1.CastSpellRequest)
		code    connect.Code
		msg     string
	}{
		{"another spell", burningHands, slotOfLevel(1), a.at(t, "Capitão Goblin"), noCastRoll, connect.CodeFailedPrecondition, "it is not this combatant's turn"},
		{"a lower slot", magicMissileSpell, slotOfLevel(1), missiles, noCastRoll, connect.CodeInvalidArgument, "a readied spell is released with the slot it was held with (level 2): name no slot or that level"},
		{"an upcast", magicMissileSpell, slotOfLevel(3), missiles, noCastRoll, connect.CodeInvalidArgument, "a readied spell is released with the slot it was held with (level 2): name no slot or that level"},
		{"Metamagic", magicMissileSpell, nil, missiles, quickened, connect.CodeInvalidArgument, "a readied spell was cast at Ready: its release takes no Metamagic"},
	} {
		_, err := a.cast(t, a.ana, e, "Pensantus", tt.spell, tt.slot, tt.targets, tt.roll)
		wantRefusal(t, tt.name, err, tt.code, tt.msg)
	}
	if got := a.slotsOf(t, "Pensantus", 1, 2); !slices.Equal(got, slots) {
		t.Errorf("slots used by level = %v, want %v: a refusal spends nothing", got, slots)
	}
	if byLabel(t, a.get(t, a.ana), "Pensantus").GetReactionUsed() || a.readiedStatus(t, "Pensantus") != readiedWaiting || !a.readiedOpen(t, "Pensantus") {
		t.Errorf("after the refusals: reaction used, row %q, window open %v; want the window as it was", a.readiedStatus(t, "Pensantus"), a.readiedOpen(t, "Pensantus"))
	}
	if res, err := a.cast(t, a.ana, e, "Pensantus", magicMissileSpell, nil, missiles, noCastRoll); err != nil || res.GetCast() == nil {
		t.Errorf("the release after the refusals = %v, %v", res, err)
	}
}

// Wall of Fire held with Ready is released with CastSpell: the point and the wall of the request lay the zone at the held
// slot, the ogre in it saves, and no other slot is spent.
func TestAReadiedWallOfFireIsReleasedWithCastSpell(t *testing.T) {
	t.Parallel()
	c := wallCave(t)
	e := c.zoneFight(t)
	e = c.passTo(t, e, "Pensantus")
	c.mustReady(t, c.ana, c.readySpell(t, e, "Pensantus", wallOfFireKey, 4, ""))
	if zs := c.get(t, c.master).GetZones(); len(zs) != 0 {
		t.Fatalf("Ready laid the zones %v", zs)
	}
	e = c.mustEndTurn(t, c.master, e)
	c.mustFire(t, e, "Pensantus")
	slots := c.slotsOf(t, "Pensantus", 4, 5)
	c.h.roller.queue(5, 5, 5, 5, 5, 5) // the ogre's Dexterity save, then 5d8 = 25
	if _, err := c.castArea(t, c.ana, "Pensantus", wallOfFireKey, nil, at(7, 8), nil, northWall(playv1.WallSide_WALL_SIDE_LEFT)); err != nil {
		t.Fatalf("CastSpell() error = %v", err)
	}
	z := c.oneZone(t, c.master, wallOfFireKey)
	if len(z.GetSquares()) != 2 || !zoneHas(z, 7, 8) || !zoneHas(z, 7, 7) || !z.GetConcentration() {
		t.Errorf("zone = %v, want the wall of (7,8) and (7,7), held with concentration", z)
	}
	if cur, _, _ := c.hp(t, "Ogro"); cur != 59-25 {
		t.Errorf("the ogre has %d hit points, want %d: the failed save takes 5d8 = 25", cur, 59-25)
	}
	if got := c.slotsOf(t, "Pensantus", 4, 5); !slices.Equal(got, slots) || c.readiedStatus(t, "Pensantus") != readiedFired {
		t.Errorf("slots used by level = %v (want %v), row %q (want fired)", got, slots, c.readiedStatus(t, "Pensantus"))
	}
	if got := c.concentrationAs(t, c.caio, "Pensantus"); got != wallOfFireKey {
		t.Errorf("the table reads Pensantus concentrating on %q, want the spell's own key", got)
	}
}

// The readier's player marks its READIED window (it is on the map) and the master reads it; marking twice is the same as
// once, only the reactor's player may mark (the master and another player are refused), nothing is written to the log, and
// the window stays open and answerable under the mark.
func TestTheReadiersPlayerMarksItsReadiedWindowOnTheMap(t *testing.T) {
	t.Parallel()
	a := newCasters(t)
	e := a.readyForRelease(t, magicMissileSpell, 1, "Capitão Goblin")
	w := a.mustFire(t, e, "Pensantus")
	rolling := func() bool { return a.windowOf(t, a.master, kindReadied).GetRolling() }
	wantCode(t, "the master marking", a.markRolling(t, a.master, e, w.GetId()), connect.CodePermissionDenied)
	wantCode(t, "another player marking", a.markRolling(t, a.caio, e, w.GetId()), connect.CodePermissionDenied)
	if rolling() {
		t.Error("a refused mark marked the window")
	}
	logBefore := logLines(a.log(t, a.master, e))
	for range 2 {
		if err := a.markRolling(t, a.ana, e, w.GetId()); err != nil || !rolling() {
			t.Fatalf("marking the window: error = %v, the master reads rolling %v; want it marked", err, rolling())
		}
	}
	if n := logLines(a.log(t, a.master, e)); n != logBefore {
		t.Errorf("marking wrote %d log lines", n-logBefore)
	}
	if !a.readiedOpen(t, "Pensantus") || a.readiedStatus(t, "Pensantus") != readiedWaiting {
		t.Fatalf("the window is not open and waiting: row %q", a.readiedStatus(t, "Pensantus"))
	}
	a.mustAnswer(t, a.ana, e, w.GetId(), withTargets(darts(a, t, "Capitão Goblin", 3)))
	if a.readiedStatus(t, "Pensantus") != readiedFired {
		t.Errorf("the window answered after the mark: row %q, want fired", a.readiedStatus(t, "Pensantus"))
	}
}

// clearRolling calls MarkReactionRolling with clear as u.
func (a *armed) clearRolling(t *testing.T, u *user, e *playv1.Encounter, windowID string) error {
	t.Helper()
	_, err := u.combat.MarkReactionRolling(t.Context(), connect.NewRequest(&playv1.MarkReactionRollingRequest{CampaignId: a.campaignID, EncounterId: e.GetId(), WindowId: windowID, Clear: true}))
	return err
}

// The readier's player takes the mark off its READIED window when it comes back from the map, and the master reads it
// each time; both ways are idempotent, only the reactor's player may do either, nothing is written to the log, and the
// window stays open and answerable.
func TestTheReadiersPlayerTakesTheRollingMarkOffItsWindow(t *testing.T) {
	t.Parallel()
	a := newCasters(t)
	e := a.readyForRelease(t, magicMissileSpell, 1, "Capitão Goblin")
	w := a.mustFire(t, e, "Pensantus")
	rolling := func() bool { return a.windowOf(t, a.master, kindReadied).GetRolling() }
	if err := a.markRolling(t, a.ana, e, w.GetId()); err != nil || !rolling() {
		t.Fatalf("marking the window: error = %v, the master reads rolling %v; want it marked", err, rolling())
	}
	wantCode(t, "the master taking the mark off", a.clearRolling(t, a.master, e, w.GetId()), connect.CodePermissionDenied)
	wantCode(t, "another player taking the mark off", a.clearRolling(t, a.caio, e, w.GetId()), connect.CodePermissionDenied)
	if !rolling() {
		t.Error("a refused clear took the mark off")
	}
	logBefore := logLines(a.log(t, a.master, e))
	for range 2 {
		if err := a.clearRolling(t, a.ana, e, w.GetId()); err != nil || rolling() {
			t.Fatalf("clearing the mark: error = %v, the master reads rolling %v; want it off", err, rolling())
		}
	}
	if n := logLines(a.log(t, a.master, e)); n != logBefore {
		t.Errorf("clearing wrote %d log lines", n-logBefore)
	}
	if !a.readiedOpen(t, "Pensantus") || a.readiedStatus(t, "Pensantus") != readiedWaiting {
		t.Fatalf("the window is not open and waiting: row %q", a.readiedStatus(t, "Pensantus"))
	}
	a.mustAnswer(t, a.ana, e, w.GetId(), withTargets(darts(a, t, "Capitão Goblin", 3)))
	wantCode(t, "clearing an answered window", a.clearRolling(t, a.ana, e, w.GetId()), connect.CodeFailedPrecondition)
}

// The wait of the READIED window begins at the first mark of the reactor's player, once: the master and the reactor's
// player (a second device) read it with the mark, a clear and a second mark leave it as it was, a clear before any mark
// starts nothing, and no other player reads the window at all.
func TestTheWaitOfAReadiedWindowBeginsAtTheFirstMarkAndNeverMoves(t *testing.T) {
	t.Parallel()
	a := newCasters(t)
	e := a.readyForRelease(t, magicMissileSpell, 1, "Capitão Goblin")
	w := a.mustFire(t, e, "Pensantus")
	reads := func(who string, u *user) *playv1.ReactionWindow {
		t.Helper()
		win := a.windowOf(t, u, kindReadied)
		if win == nil {
			t.Fatalf("%s reads no READIED window", who)
		}
		return win
	}
	both := func(step string, rolling bool) *timestamppb.Timestamp {
		t.Helper()
		m, p := reads("the master", a.master), reads("the reactor's player", a.ana)
		for who, win := range map[string]*playv1.ReactionWindow{"the master": m, "the reactor's player": p} {
			if win.GetRolling() != rolling {
				t.Errorf("%s: %s reads rolling %v, want %v", step, who, win.GetRolling(), rolling)
			}
		}
		if !proto.Equal(m.GetWaitBeganAt(), p.GetWaitBeganAt()) {
			t.Errorf("%s: the master reads the wait began at %v, the reactor's player at %v", step, m.GetWaitBeganAt(), p.GetWaitBeganAt())
		}
		return m.GetWaitBeganAt()
	}
	if err := a.clearRolling(t, a.ana, e, w.GetId()); err != nil {
		t.Fatalf("clearing before any mark: %v", err)
	}
	if began := both("a clear before any mark", false); began != nil {
		t.Fatalf("a clear before any mark started the wait at %v", began)
	}
	wantCode(t, "the master marking", a.markRolling(t, a.master, e, w.GetId()), connect.CodePermissionDenied)
	wantCode(t, "another player marking", a.markRolling(t, a.caio, e, w.GetId()), connect.CodePermissionDenied)
	if began := both("a refused mark", false); began != nil {
		t.Fatalf("a refused mark started the wait at %v", began)
	}
	logBefore := logLines(a.log(t, a.master, e))
	if err := a.markRolling(t, a.ana, e, w.GetId()); err != nil {
		t.Fatalf("the first mark: %v", err)
	}
	first := both("the first mark", true)
	if first == nil || first.AsTime().IsZero() {
		t.Fatalf("the first mark left the wait's beginning unset: %v", first)
	}
	steps := []struct {
		name    string
		do      func() error
		rolling bool
	}{
		{"a second mark", func() error { return a.markRolling(t, a.ana, e, w.GetId()) }, true},
		{"a clear", func() error { return a.clearRolling(t, a.ana, e, w.GetId()) }, false},
		{"a second clear", func() error { return a.clearRolling(t, a.ana, e, w.GetId()) }, false},
		{"a mark after the clear", func() error { return a.markRolling(t, a.ana, e, w.GetId()) }, true},
	}
	for _, s := range steps {
		if err := s.do(); err != nil {
			t.Fatalf("%s: %v", s.name, err)
		}
		if got := both(s.name, s.rolling); !proto.Equal(got, first) {
			t.Errorf("after %s the wait began at %v, want the first mark's %v", s.name, got, first)
		}
	}
	if n := logLines(a.log(t, a.master, e)); n != logBefore {
		t.Errorf("marking and clearing wrote %d log lines", n-logBefore)
	}
	if w := a.windowOf(t, a.caio, kindReadied); w != nil {
		t.Errorf("another player reads the window %v", w)
	}
	if !a.readiedOpen(t, "Pensantus") || a.readiedStatus(t, "Pensantus") != readiedWaiting {
		t.Fatalf("the window is not open and waiting: row %q", a.readiedStatus(t, "Pensantus"))
	}
	a.mustAnswer(t, a.ana, e, w.GetId(), withTargets(darts(a, t, "Capitão Goblin", 3)))
	if a.readiedStatus(t, "Pensantus") != readiedFired {
		t.Errorf("the window answered after the marks: row %q, want fired", a.readiedStatus(t, "Pensantus"))
	}
}

// A READIED window an undo opens again starts its wait clean: the mark and the beginning of the wait the window had
// before its use (or its pass) are gone, so the master's screen does not pass it at once, and the next mark starts a new
// wait. The same for the spell's release, the move's and the pass.
func TestAnUndoThatReopensAReadiedWindowStartsItsWaitClean(t *testing.T) {
	t.Parallel()
	type setup struct {
		a      *armed
		e      *playv1.Encounter
		w      *playv1.ReactionWindow
		player *user
		use    func(t *testing.T) // the use or the pass that closes the window
	}
	spell := func(release func(t *testing.T, a *armed, e *playv1.Encounter, w *playv1.ReactionWindow)) func(t *testing.T) setup {
		return func(t *testing.T) setup {
			t.Helper()
			a := newCasters(t)
			e := a.readyForRelease(t, magicMissileSpell, 1, "Capitão Goblin")
			w := a.mustFire(t, e, "Pensantus")
			return setup{a, e, w, a.ana, func(t *testing.T) {
				t.Helper()
				release(t, a, e, w)
			}}
		}
	}
	for _, tt := range []struct {
		name  string
		setup func(t *testing.T) setup
	}{
		{"a spell released with CastSpell", spell(func(t *testing.T, a *armed, e *playv1.Encounter, _ *playv1.ReactionWindow) {
			t.Helper()
			if _, err := a.cast(t, a.ana, e, "Pensantus", magicMissileSpell, nil, []*playv1.SpellTarget{darts(a, t, "Capitão Goblin", 3)}, noCastRoll); err != nil {
				t.Fatalf("the release: %v", err)
			}
		})},
		{"a spell released with an answer", spell(func(t *testing.T, a *armed, e *playv1.Encounter, w *playv1.ReactionWindow) {
			t.Helper()
			a.mustAnswer(t, a.ana, e, w.GetId(), withTargets(darts(a, t, "Capitão Goblin", 3)))
		})},
		{"a pass", spell(func(t *testing.T, a *armed, e *playv1.Encounter, w *playv1.ReactionWindow) {
			t.Helper()
			a.mustAnswer(t, a.master, e, w.GetId(), passAnswer)
		})},
		{"a move", func(t *testing.T) setup {
			t.Helper()
			a := newArmed(t)
			e := a.readyMove(t)
			w := a.mustFire(t, e, "Toren")
			return setup{a, e, w, a.caio, func(t *testing.T) {
				t.Helper()
				if _, err := a.frozenMove(t, a.caio, "Toren", 3, 5); err != nil {
					t.Fatalf("the readied move: %v", err)
				}
			}}
		}},
	} {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			s := tt.setup(t)
			a := s.a
			if err := a.markRolling(t, s.player, s.e, s.w.GetId()); err != nil {
				t.Fatalf("the first mark: %v", err)
			}
			first := a.windowOf(t, a.master, kindReadied).GetWaitBeganAt()
			if first == nil || !a.windowOf(t, a.master, kindReadied).GetRolling() {
				t.Fatalf("the first mark left rolling %v and the wait's beginning %v", a.windowOf(t, a.master, kindReadied).GetRolling(), first)
			}
			a.h.clock.advance(time.Minute) // the player keeps the window a while before it uses it
			s.use(t)
			a.mustUndo(t, s.e)
			for who, u := range map[string]*user{"the master": a.master, "the reactor's player": s.player} {
				w := a.windowOf(t, u, kindReadied)
				if w == nil {
					t.Fatalf("after the undo %s reads no READIED window", who)
				}
				if w.GetRolling() || w.GetWaitBeganAt() != nil {
					t.Errorf("after the undo %s reads rolling %v and the wait began at %v, want a clean wait", who, w.GetRolling(), w.GetWaitBeganAt())
				}
			}
			if err := a.markRolling(t, s.player, s.e, s.w.GetId()); err != nil {
				t.Fatalf("the mark after the undo: %v", err)
			}
			if again := a.windowOf(t, a.master, kindReadied).GetWaitBeganAt(); again == nil || !again.AsTime().After(first.AsTime()) {
				t.Errorf("the new wait began at %v, want a time after the first wait's %v", again, first)
			}
		})
	}
}
