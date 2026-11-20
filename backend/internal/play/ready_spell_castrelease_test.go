package play

import (
	"testing"

	"connectrpc.com/connect"
	"google.golang.org/protobuf/proto"

	playv1 "github.com/PuraFome/meuRPG/backend/gen/meurpg/play/v1"
)

// The master releases a held spell for its readier with CastSpell and PreviewSpellArea while the READIED window is open,
// as he casts for any of his players; another player is refused as for any combatant that is not theirs.
func TestTheMasterReleasesTheHeldSpellForItsReadierAndAnotherPlayerCannot(t *testing.T) {
	t.Parallel()
	c := newAreaCave(t)
	e := c.readiedInTheCave(t, sleepSpell)
	w := c.mustFire(t, e, "Pensantus")
	events := c.events(t)
	for _, other := range []struct {
		u      *user
		theirs string // a combatant that is not the other player's, and not the readier
	}{{c.caio, "Brisa"}, {c.bia, "Toren"}} {
		_, err := c.preview(t, other.u, "Pensantus", sleepSpell, nil, at(20, 7), nil)
		_, elsewhere := c.preview(t, other.u, other.theirs, sleepSpell, nil, at(20, 7), nil)
		wantCode(t, "another player's preview of the held spell", err, connect.CodePermissionDenied)
		if refusalOf(t, err) != refusalOf(t, elsewhere) {
			t.Errorf("the preview on the held spell is refused as %v, on a combatant that is not theirs as %v", refusalOf(t, err), refusalOf(t, elsewhere))
		}
		_, err = c.castArea(t, other.u, "Pensantus", sleepSpell, nil, at(20, 7), nil)
		_, elsewhere = c.castArea(t, other.u, other.theirs, sleepSpell, nil, at(20, 7), nil)
		wantCode(t, "another player's cast of the held spell", err, connect.CodePermissionDenied)
		if refusalOf(t, err) != refusalOf(t, elsewhere) {
			t.Errorf("the cast of the held spell is refused as %v, on a combatant that is not theirs as %v", refusalOf(t, err), refusalOf(t, elsewhere))
		}
	}
	if n := c.events(t); n != events || !c.readiedOpen(t, "Pensantus") || c.readiedStatus(t, "Pensantus") != readiedWaiting {
		t.Fatalf("after the refusals: events %d (want %d), window open %v, row %q", n, events, c.readiedOpen(t, "Pensantus"), c.readiedStatus(t, "Pensantus"))
	}
	if res, err := c.preview(t, c.master, "Pensantus", sleepSpell, nil, at(20, 7), nil); err != nil || len(res.GetSquares()) == 0 {
		t.Errorf("the master's preview of the held spell = %v, %v", res, err)
	}
	if _, err := c.castArea(t, c.master, "Pensantus", sleepSpell, nil, at(20, 7), nil, poolInApp); err != nil {
		t.Fatalf("the master's release: %v", err)
	}
	pens := byLabel(t, c.get(t, c.ana), "Pensantus")
	if !pens.GetReactionUsed() || c.readiedStatus(t, "Pensantus") != readiedFired || c.readiedOpen(t, "Pensantus") {
		t.Errorf("after the master's release: reaction used %v, row %q, window open %v", pens.GetReactionUsed(), c.readiedStatus(t, "Pensantus"), c.readiedOpen(t, "Pensantus"))
	}
	if _, err := c.answerReaction(t, c.ana, e, w.GetId(), passAnswer); err == nil {
		t.Error("the answered window took another answer")
	}
}

// The release made with CastSpell is idempotent: the same key after the window closed returns the same result and casts
// nothing twice; its undo makes the row wait with the window open, the reaction free and the spell held again.
func TestAReadiedSpellReleasedWithCastSpellReplaysAndUndoesLikeAnAnswer(t *testing.T) {
	t.Parallel()
	a := newCasters(t)
	e := a.readyForRelease(t, magicMissileSpell, 1, "Capitão Goblin")
	a.mustFire(t, e, "Pensantus")
	slots, key := a.slotsUsed(t, "Pensantus", 1), newKey()
	targets := []*playv1.SpellTarget{darts(a, t, "Capitão Goblin", 3)}
	first, err := a.castKey(t, a.ana, e, "Pensantus", magicMissileSpell, nil, targets, noCastRoll, key)
	if err != nil {
		t.Fatalf("the release: %v", err)
	}
	events := a.events(t)
	again, err := a.castKey(t, a.ana, e, "Pensantus", magicMissileSpell, nil, targets, noCastRoll, key)
	if err != nil || !proto.Equal(first.GetCast(), again.GetCast()) || a.events(t) != events || len(a.castLines(t, e, magicMissileSpell)) != 1 {
		t.Errorf("replay: result %v, error %v (first %v), events %d (want %d), cast lines %d (want 1)", again.GetCast(), err, first.GetCast(), a.events(t), events, len(a.castLines(t, e, magicMissileSpell)))
	}
	a.mustUndo(t, e)
	pens := byLabel(t, a.get(t, a.master), "Pensantus")
	if a.readiedStatus(t, "Pensantus") != readiedWaiting || !a.readiedOpen(t, "Pensantus") || pens.GetReactionUsed() ||
		pens.GetConcentrationSpell() != magicMissileSpell || a.slotsUsed(t, "Pensantus", 1) != slots || len(a.castLines(t, e, magicMissileSpell)) != 0 {
		t.Errorf("after the undo: row %q, window open %v, reaction used %v, concentration %q, slots %d (want %d), cast lines %d (want 0)", a.readiedStatus(t, "Pensantus"),
			a.readiedOpen(t, "Pensantus"), pens.GetReactionUsed(), pens.GetConcentrationSpell(), a.slotsUsed(t, "Pensantus", 1), slots, len(a.castLines(t, e, magicMissileSpell)))
	}
}

// The same key of the readied move's release, again after the window closed, returns the same result and walks nothing twice.
func TestAReadiedMoveReleaseReplaysWithItsKey(t *testing.T) {
	t.Parallel()
	a := newArmed(t)
	e := a.readyMove(t)
	a.mustFire(t, e, "Toren")
	key := newKey()
	keyed := func(r *playv1.MoveCombatantRequest) { r.IdempotencyKey = key }
	first, err := a.frozenMove(t, a.caio, "Toren", 3, 5, keyed)
	if err != nil {
		t.Fatalf("the readied move: %v", err)
	}
	events := a.events(t)
	again, err := a.frozenMove(t, a.caio, "Toren", 3, 5, keyed)
	if err != nil || !proto.Equal(first, again) || a.events(t) != events || a.squareOf(t, "Toren") != [2]int32{3, 5} {
		t.Errorf("replay: error %v, same result %v, events %d (want %d), at %v", err, proto.Equal(first, again), a.events(t), events, a.squareOf(t, "Toren"))
	}
	if lines := a.firedLines(t, a.master, e); len(lines) != 1 {
		t.Errorf("the fired lines = %v, want one", lines)
	}
}
