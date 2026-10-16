package play

import (
	"maps"
	"slices"
	"strings"
	"testing"

	playv1 "github.com/PuraFome/meuRPG/backend/gen/meurpg/play/v1"
	"github.com/PuraFome/meuRPG/backend/internal/authz"
)

// takeBack called on its own, for what the API cannot reach: UndoLastAction only
// hands it an undoable kind (lastAction) of a combatant that is still in the combat,
// but a character deleted between the event and its undo leaves events about
// combatants that are gone, and takeBack answers for those too.

// takeBackAs runs takeBack inside the combat's transaction, as the master's undo does,
// and returns what it answered.
func (a *armed) takeBackAs(t *testing.T, e *playv1.Encounter, kind string, ev actionEvent) ([]*playv1.CharacterVitals, error) {
	t.Helper()
	m := authz.Membership{CampaignID: a.campaignID, UserID: a.master.id, Role: authz.RoleMaster}
	var vitals []*playv1.CharacterVitals
	_, err := a.h.svc.write(t.Context(), combatWrite{m: m, key: newKey(), kind: eventActionUndone, encounterID: e.GetId()}, func(c *combatTx) (any, error) {
		var err error
		vitals, err = a.h.svc.takeBack(t.Context(), c, kind, ev)
		return actionEvent{Round: c.enc.Round}, err
	})
	return vitals, err
}

// TestTakeBackRefusesWhatIsNotAnUndoableEvent: a kind with no undo is an internal error
// that names it, and a move written before the undo knew moves is the "nothing to undo"
// refusal.
func TestTakeBackRefusesWhatIsNotAnUndoableEvent(t *testing.T) {
	t.Parallel()
	a := newArmed(t)
	e := a.threeAndAGoblin(t)

	_, err := a.takeBackAs(t, e, eventActionUndone, actionEvent{})
	if err == nil || err.Error() != `event kind "action_undone" cannot be undone` {
		t.Errorf("takeBack(the undo itself) = %v, want event kind %q cannot be undone", err, eventActionUndone)
	}
	_, err = a.takeBackAs(t, e, eventCombatantMoved, actionEvent{Actor: a.id(t, "Toren")})
	wantBlockedBy(t, "takeBack(a move with no From)", err, blockedNothingToUndo)
	if !strings.Contains(err.Error(), "this move cannot be undone") {
		t.Errorf("error = %q, want the message %q", err, "this move cannot be undone")
	}
}

// TestTakeBackSkipsACombatantThatIsGone: every kind that puts something back on its
// actor or target does nothing, and answers no vitals and no error, when that combatant
// is not in the combat any more.
func TestTakeBackSkipsACombatantThatIsGone(t *testing.T) {
	t.Parallel()
	a := newArmed(t)
	e := a.threeAndAGoblin(t)
	w := newUndoWatch(t, a)
	gone := newKey()
	hp := &hpState{HP: 3}
	kinds := map[string]actionEvent{
		eventAttackRolled:      {Actor: gone, Res: &resourceEvent{Kind: resBardicUse}, ExtraUsed: true},
		eventDamageRolled:      {Actor: gone, Target: gone, Pending: gone, Applied: true, Before: hp, Slot: &slotRef{Level: 1}},
		eventDamageApplied:     {Target: gone, Before: hp, Relentless: true},
		eventCombatantMoved:    {Actor: gone, From: &moveState{Placed: true, Col: 1, Row: 1}, CondSet: true},
		eventActionTaken:       {Actor: gone, Heal: true, Before: hp},
		eventHitPointsAdjusted: {Actor: gone, Before: hp},
		eventSpellCast:         {Actor: gone, Slot: &slotRef{Level: 1}, Concentrate: true},
		eventReactionUsed:      {Actor: gone, Slot: &slotRef{Level: 1}},
		eventDeathSaveRolled:   {Actor: gone, Before: hp},
		eventConditionsSet:     {Actor: gone, CondSet: true, ConcEnded: "spell:hold-person"},
	}
	for _, kind := range slices.Sorted(maps.Keys(kinds)) {
		before := w.picture(t)
		vitals, err := a.takeBackAs(t, e, kind, kinds[kind])
		if err != nil || len(vitals) != 0 {
			t.Errorf("takeBack(%s) with its combatant gone = %v, %v; want nothing and no error", kind, vitals, err)
		}
		if diff := difference(before, w.picture(t)); len(diff) != 0 {
			t.Errorf("takeBack(%s) with its combatant gone changed the combat:\n%v", kind, diff)
		}
	}
}

// column reads one column of a combatant's row.
func (a *armed) column(t *testing.T, id, column string) (value *string) {
	t.Helper()
	if err := a.h.pool.QueryRow(t.Context(), "SELECT "+column+"::TEXT FROM combatants WHERE id = $1", id).Scan(&value); err != nil {
		t.Fatalf("read %s: %v", column, err)
	}
	return value
}

// TestTakeBackOfAMoveWrittenWithoutACoverMark: a move event with no cover mark (an
// event of before the mark was kept) puts the combatant back with "none".
func TestTakeBackOfAMoveWrittenWithoutACoverMark(t *testing.T) {
	t.Parallel()
	a := newArmed(t)
	e := a.threeAndAGoblin(t)
	toren := a.id(t, "Toren")
	if _, err := a.takeBackAs(t, e, eventCombatantMoved, actionEvent{Actor: toren, From: &moveState{Placed: true, Col: 2, Row: 2}}); err != nil {
		t.Fatalf("takeBack(a move) error = %v", err)
	}
	if got := a.column(t, toren, "cover_mark"); got == nil || *got != "none" {
		t.Errorf("cover_mark = %v, want none", got)
	}
	if got := a.column(t, toren, "grid_col"); got == nil || *got != "2" {
		t.Errorf("grid_col = %v, want 2", got)
	}
}

// TestTakeBackOfASpellAttackThatUsedADieFromABard: the die comes back with the bard's
// combatant that gave it, and none when no one is named.
func TestTakeBackOfASpellAttackThatUsedADieFromABard(t *testing.T) {
	t.Parallel()
	a := newArmed(t)
	e := a.threeAndAGoblin(t)
	toren, pens := a.id(t, "Toren"), a.id(t, "Pensantus")
	for _, tc := range []struct{ name, from, want string }{{"a named giver", pens, pens}, {"no giver", "", ""}} {
		ev := actionEvent{Actor: toren, Die: &resourceEvent{Kind: resBardicUse, Sides: 6, ExpiresRound: 3, FromID: tc.from}}
		if _, err := a.takeBackAs(t, e, eventSpellCast, ev); err != nil {
			t.Fatalf("%s: takeBack(a cast) error = %v", tc.name, err)
		}
		got := a.column(t, toren, "inspiration_from")
		if (tc.want == "") != (got == nil) || (got != nil && *got != tc.want) {
			t.Errorf("%s: inspiration_from = %v, want %q", tc.name, got, tc.want)
		}
		if sides := a.column(t, toren, "inspiration_sides"); sides == nil || *sides != "6" {
			t.Errorf("%s: inspiration_sides = %v, want 6", tc.name, sides)
		}
	}
}

// TestTakeBackOfADiscardFromAnUnknownStatus: a discard that kept no valid previous
// status (it never happens: only three can be discarded) goes back to "rolled".
func TestTakeBackOfADiscardFromAnUnknownStatus(t *testing.T) {
	t.Parallel()
	a := newArmed(t)
	e := a.threeAndAGoblin(t)
	a.h.roller.queue(15)
	hit := a.mustAttack(t, a.caio, e, "Toren", battleaxe, "Goblin", inAppRoll)
	id := hit.GetPendingDamage().GetId()
	if _, err := a.takeBackAs(t, e, eventDamageDiscarded, actionEvent{Pending: id, PrevStatus: "applied"}); err != nil {
		t.Fatalf("takeBack(a discard) error = %v", err)
	}
	var status string
	if err := a.h.pool.QueryRow(t.Context(), "SELECT status FROM pending_damages WHERE id = $1", id).Scan(&status); err != nil || status != pendingRolled {
		t.Errorf("status = %q (%v), want %q", status, err, pendingRolled)
	}
}
