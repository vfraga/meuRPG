package play

import (
	"errors"
	"testing"

	"connectrpc.com/connect"

	playv1 "github.com/PuraFome/meuRPG/backend/gen/meurpg/play/v1"
)

// TestMountedCombat_ARidersPlayerMovesNoMountWhosePartEnded: the master ends the Ogro's part of the joint turn while Toren's goes on: the
// rider's player no longer moves the Ogro, and its move options say it is not their turn (the mount's part is its own); the master still
// places it, and the rider's player read its options before the part ended (the positive control).
func TestMountedCombat_ARidersPlayerMovesNoMountWhosePartEnded(t *testing.T) {
	t.Parallel()
	c := newCave(t)
	c.fight(t)
	c.steed(t)
	c.mustMount(t, c.caio, "Toren", "Ogro")
	if _, err := c.options(t, c.caio, "Ogro"); err != nil {
		t.Fatalf("the positive control failed: the options of the mount the rider drives = %v", err)
	}
	e := c.mustEndPart(t, c.master, c.get(t, c.master), "Ogro")
	if !byLabel(t, e, "Ogro").GetTurnPartEnded() || byLabel(t, e, "Toren").GetTurnPartEnded() {
		t.Fatalf("premise: after the master ended the Ogro's part, parts ended = Ogro %v, Toren %v", byLabel(t, e, "Ogro").GetTurnPartEnded(), byLabel(t, e, "Toren").GetTurnPartEnded())
	}
	_, err := c.move(t, c.caio, "Ogro", 8, 7)
	wantBlockedBy(t, "the rider's player moving a mount whose part ended", err, reasonNotYourTurn)
	_, err = c.options(t, c.caio, "Ogro")
	wantBlockedBy(t, "the move options of a mount whose part ended", err, reasonNotYourTurn)
	if got := c.rideRowOf(t, "Ogro"); got.Col != 7 || got.UsedDft != 0 {
		t.Errorf("the Ogro after the refused move = %+v, want where it was and nothing spent", got)
	}
	if _, err := c.move(t, c.master, "Ogro", 8, 7); err != nil {
		t.Errorf("the master placing the mount = %v, want it moved", err)
	}
}

var reasonNotYourTurn = playv1.EncounterBlockedReason_ENCOUNTER_BLOCKED_REASON_NOT_YOUR_TURN

// TestMountedCombat_AMountMarkedControlledMidTurnJoinsTheRidersPart: the Ogro carries Toren as an independent mount and the master marks it
// controlled while Toren's turn runs. It takes the rider's total and joins his part of the turn at once, as a mount the rider just got on
// does: acting, in the joint group, with a fresh economy; or, when its own turn came before this one, with the economy it had spent.
func TestMountedCombat_AMountMarkedControlledMidTurnJoinsTheRidersPart(t *testing.T) {
	t.Parallel()
	t.Run("its own turn still to come", func(t *testing.T) {
		t.Parallel()
		c := newCave(t)
		c.fight(t)
		c.steed(t)
		c.mustMark(t, "Ogro", "independent")
		c.mustMount(t, c.caio, "Toren", "Ogro")
		if got := group(c.get(t, c.master)); got != "Toren" {
			t.Fatalf("premise: the group on turn = %q, want Toren alone with an independent mount", got)
		}
		c.mustMark(t, "Ogro", "controlled")
		if got := group(c.get(t, c.master)); got != "Toren,Ogro" {
			t.Errorf("the group on turn = %q after the mark, want Toren,Ogro", got)
		}
		if ogre := c.joinRowOf(t, "Ogro"); ogre.State != "acting" || ogre.ActionUsed {
			t.Errorf("the Ogro after the mark = %+v, want acting with a fresh economy", ogre)
		}
		if _, err := c.action(t, c.caio, c.get(t, c.caio), "Ogro", "standard:dash"); err != nil {
			t.Errorf("the rider's player making the mount Dash = %v, want it taken", err)
		}
		e := c.mustEndPart(t, c.caio, c.get(t, c.caio), "Toren")
		if current(e) != "Ogro" {
			t.Errorf("after Toren's part the turn is %q's, want the Ogro's part to run", current(e))
		}
	})
	t.Run("its own turn passed", func(t *testing.T) {
		t.Parallel()
		c := newCave(t)
		c.ogreFirstFight(t)
		c.steed(t)
		c.mustMark(t, "Ogro", "independent")
		c.spendTheOgre(t)
		c.passTo(t, c.get(t, c.master), "Toren")
		c.mustMount(t, c.caio, "Toren", "Ogro")
		c.mustMark(t, "Ogro", "controlled")
		if got := group(c.get(t, c.master)); got != "Toren,Ogro" {
			t.Errorf("the group on turn = %q after the mark, want Toren,Ogro", got)
		}
		if ogre := c.joinRowOf(t, "Ogro"); ogre.State != "acting" || !ogre.ActionUsed || !ogre.ReactionUsed {
			t.Errorf("the Ogro after the mark = %+v, want acting with the economy it had spent", ogre)
		}
	})
}

// shapeOfRefusal is what a player reads of a refusal: its code, its words and its reason.
func shapeOfRefusal(t *testing.T, err error) string {
	t.Helper()
	var ce *connect.Error
	if !errors.As(err, &ce) {
		t.Fatalf("error = %v, want a connect error", err)
	}
	reason := ""
	for _, d := range ce.Details() {
		if v, e := d.Value(); e == nil {
			if b, ok := v.(*playv1.EncounterBlocked); ok {
				reason = b.GetReason().String()
			}
		}
	}
	return ce.Code().String() + "|" + ce.Message() + "|" + reason
}

// TestMountedCombat_ARiderOfAHiddenMountTeleportsAsAnUnmountedCasterDoes: Ana's Pensantus rides an Ogro the players do not see and casts
// Misty Step. The destination checks answer as they do for a caster on foot (a square off the grid, too far), and a square that would
// be free is refused as a square a hidden creature holds is: the same code, words and reason, so the refusal names no mount. The caster
// on foot, with a hidden goblin on the square, is the twin; a visible mount is MOUNTED as ever.
func TestMountedCombat_ARiderOfAHiddenMountTeleportsAsAnUnmountedCasterDoes(t *testing.T) {
	t.Parallel()
	ride := func(t *testing.T, hideTheMount bool) *cave {
		t.Helper()
		c := newCaveWith(t, 3, []string{mistyStep})
		c.fight(t)
		c.mustMark(t, "Ogro", "controlled")
		c.side(t, "Ogro", playv1.CombatantSide_COMBATANT_SIDE_PARTY)
		c.mustMove(t, c.master, "Ogro", 5, 9)
		c.passTo(t, c.get(t, c.master), "Pensantus")
		c.mustMount(t, c.ana, "Pensantus", "Ogro")
		if hideTheMount {
			c.hide(t, "Ogro")
		}
		return c
	}
	riding := ride(t, true)
	onFoot := newCaveWith(t, 3, []string{mistyStep})
	onFoot.fight(t)
	onFoot.passTo(t, onFoot.get(t, onFoot.master), "Pensantus")
	from := riding.squareOf(t, riding.ana, "Pensantus")
	home := onFoot.squareOf(t, onFoot.ana, "Pensantus")
	if from != [2]int32{5, 9} {
		t.Fatalf("premise: the rider stands on %v, want its mount's (5,9)", from)
	}
	// A hidden goblin on a square next to the caster on foot is the square a free one reads as for the rider.
	onFoot.mustMove(t, onFoot.master, "Goblin 1", home[0]-1, home[1])
	onFoot.hide(t, "Goblin 1")

	for name, tc := range map[string]struct {
		rider, foot *playv1.SpellOrigin
	}{
		"off the grid": {at(400, 400), at(400, 400)},
		"too far":      {at(from[0]+9, from[1]), at(home[0]+9, home[1])},
		"a free square (the twin holds a hidden creature)": {at(from[0], from[1]-1), at(home[0]-1, home[1])},
	} {
		_, a := riding.misty(t, riding.ana, tc.rider)
		_, b := onFoot.misty(t, onFoot.ana, tc.foot)
		if a == nil || b == nil || shapeOfRefusal(t, a) != shapeOfRefusal(t, b) {
			t.Errorf("%s: the rider of a hidden mount reads %v, the caster on foot %v; want the same refusal", name, a, b)
		}
	}
	// The mount a player sees is named, as before: MOUNTED, whatever the square.
	seen := ride(t, false)
	_, err := seen.misty(t, seen.ana, at(400, 400))
	wantBlockedBy(t, "a rider of a visible mount to a square off the grid", err, reasonMounted)
}

// TestMountedCombat_NoRideIsMadeBeforeTheCombatRuns: the initiative is rolled in SETUP, and a mount or a dismount is refused there
// (NOT_ACTIVE, for the master too), so no rider exists while a total is written and no writer of the initiative meets a controlled mount
// that carries one. The master's marks of a mount are allowed in SETUP: they carry no rider.
func TestMountedCombat_NoRideIsMadeBeforeTheCombatRuns(t *testing.T) {
	t.Parallel()
	c := newCave(t)
	e := c.setupUnplaced(t)
	if e.GetStatus() != playv1.EncounterStatus_ENCOUNTER_STATUS_SETUP {
		t.Fatalf("premise: the combat is %v, want SETUP", e.GetStatus())
	}
	c.mustMark(t, "Ogro", "controlled")
	c.mustMove(t, c.master, "Ogro", 7, 7)
	for who, u := range map[string]*user{"the rider's player": c.caio, "the master": c.master} {
		_, err := c.mountAs(t, u, "Toren", "Ogro")
		wantBlockedBy(t, "a mount in SETUP by "+who, err, blockedNotActive)
	}
	if got := c.rideRowOf(t, "Toren"); got.MountedOn != "" {
		t.Errorf("Toren rides %s in SETUP, want nothing", got.MountedOn)
	}
}
