package play

import (
	"testing"

	playv1 "github.com/PuraFome/meuRPG/backend/gen/meurpg/play/v1"
)

// The characterization of MoveCombatant (see combat_move_frozen_test.go): what the
// other modules add to a move, a drag, a surprise, a trap.

// A grappler drags the creature it holds, at half speed.
func TestMoveCombatantFrozen_ADrag(t *testing.T) {
	t.Parallel()
	t.Run("the creature is left behind and the walk costs twice", func(t *testing.T) {
		t.Parallel()
		a := newArmed(t)
		c := dragFight(t, a, arenaPlan{})
		c.frozenCheck(t, a.caio, "Toren", 3, 0, nil, nil, moveWant{col: 3, row: 0, usedDft: 300, lastDft: 150, event: `{"col": 3, "combatant_id": "<Toren>", "cost_dft": 150, "cost_ft": 15, "distance_dft": 150, "distance_ft": 15, "dragged_from": {"col": 4, "cover": "none", "placed": true, "row": 3}, "dragged_id": "<Hobgoblin>", "dragging": true, "from": {"col": 3, "cover": "none", "placed": true, "row": 3}, "on_turn": true, "round": 2, "spent_dft": 300}`})
		if got := c.square(t, c.hob); got != [2]int32{3, 1} {
			t.Errorf("the dragged creature stands on %v, want (3, 1)", got)
		}
		if got := c.moveRowOf(t, c.hob); got.UsedDft != 0 || got.LastDft != 0 {
			t.Errorf("the dragged creature spent %v, want nothing", got)
		}
	})
	t.Run("a forced move drags nobody", func(t *testing.T) {
		t.Parallel()
		a := newArmed(t)
		c := dragFight(t, a, arenaPlan{})
		c.frozenCheck(t, a.master, "Toren", 3, 0, []func(*playv1.MoveCombatantRequest){asForced}, nil, moveWant{col: 3, row: 0, usedDft: 0, lastDft: 0, event: `{"col": 3, "combatant_id": "<Toren>", "distance_dft": 150, "distance_ft": 15, "from": {"col": 3, "cover": "none", "placed": true, "row": 3}, "on_turn": true, "round": 2}`})
		if got := c.square(t, c.hob); got != [2]int32{4, 3} {
			t.Errorf("the creature stands on %v after a forced move, want where it was, (4, 3)", got)
		}
	})
	t.Run("a long jump drags nobody", func(t *testing.T) {
		t.Parallel()
		a := newArmed(t)
		c := dragFight(t, a, arenaPlan{})
		c.frozenCheck(t, a.caio, "Toren", 3, 2, []func(*playv1.MoveCombatantRequest){jumpTo(playv1.JumpKind_JUMP_KIND_LONG)}, nil, moveWant{col: 3, row: 2, usedDft: 50, lastDft: 0, event: `{"col": 3, "combatant_id": "<Toren>", "cost_dft": 50, "cost_ft": 5, "distance_dft": 50, "distance_ft": 5, "from": {"col": 3, "cover": "none", "placed": true, "row": 3}, "jump": "long", "on_turn": true, "round": 2, "row": 2}`})
		if got := c.square(t, c.hob); got != [2]int32{4, 3} {
			t.Errorf("the creature stands on %v after a jump, want where it was, (4, 3)", got)
		}
	})
	t.Run("no room to leave the creature", func(t *testing.T) {
		t.Parallel()
		a := newArmed(t)
		c := a.arena(t, arenaPlan{})
		c.grappleWon(t)
		c.advance(t, c.hob)
		c.advance(t, "Toren")
		if _, err := c.moveTo(t, a.master, "Pensantus", 3, 2); err != nil {
			t.Fatalf("placing Pensantus: %v", err)
		}
		before, events := c.moveRowOf(t, "Toren"), c.events(t)
		_, err := c.frozenMove(t, a.caio, "Toren", 3, 1)
		c.frozenWantRefusal(t, err, contestBlockedBy("NO_ROOM_TO_DRAG", "there is no room to leave the creature"))
		if c.moveRowOf(t, "Toren") != before || c.events(t) != events {
			t.Error("a refused drag changed the grappler or wrote an event")
		}
	})
}

// A surprised creature does not move on its first turn.
func TestMoveCombatantFrozen_Surprised(t *testing.T) {
	t.Parallel()
	a := newSneaks(t)
	c := a.arena(t, arenaPlan{setup: true})
	c.mustSetSurprised(t, "Toren", true)
	c.e = a.begin(t, c.e)
	_, err := c.frozenMove(t, a.caio, "Toren", 4, 3)
	c.frozenWantRefusal(t, err, contestBlockedBy("SURPRISED", "the combatant is surprised"))
	// The master, who moves a token off the rules, is not held by it.
	c.frozenCheck(t, a.master, "Toren", 4, 2, nil, nil, moveWant{col: 4, row: 2, usedDft: 0, lastDft: 0, event: `{"col": 4, "combatant_id": "<Toren>", "distance_dft": 71, "distance_ft": 7, "from": {"col": 3, "cover": "none", "placed": true, "row": 3}, "on_turn": true, "round": 1, "row": 2}`})
}

// An armed trap on the way stops the move, and fires where the move ends.
func TestMoveCombatantFrozen_Traps(t *testing.T) {
	t.Parallel()
	for _, tc := range []struct {
		name string
		trap [2]int
		as   string // "master" or Toren's player
		col  int32
		edit []func(*playv1.MoveCombatantRequest)
		want moveWant
	}{
		{
			name: "a walk stops on the first square of the area", trap: [2]int{9, 7}, col: 12,
			want: moveWant{col: 9, row: 7, usedDft: 150, lastDft: 150, stopped: true, event: `{"col": 9, "combatant_id": "<Toren>", "cost_dft": 150, "cost_ft": 15, "distance_dft": 150, "distance_ft": 15, "from": {"col": 6, "cover": "none", "placed": true, "row": 7}, "on_turn": true, "round": 1, "row": 7, "spent_dft": 150, "stopped_early": true}`},
		},
		{
			name: "a walk that ends in the area is not cut short", trap: [2]int{9, 7}, col: 9,
			want: moveWant{col: 9, row: 7, usedDft: 150, lastDft: 150, event: `{"col": 9, "combatant_id": "<Toren>", "cost_dft": 150, "cost_ft": 15, "distance_dft": 150, "distance_ft": 15, "from": {"col": 6, "cover": "none", "placed": true, "row": 7}, "on_turn": true, "round": 1, "row": 7, "spent_dft": 150}`},
		},
		{
			name: "a long jump fires where it lands", trap: [2]int{7, 7}, col: 7, edit: []func(*playv1.MoveCombatantRequest){jumpTo(playv1.JumpKind_JUMP_KIND_LONG)},
			want: moveWant{col: 7, row: 7, usedDft: 50, lastDft: 0, event: `{"col": 7, "combatant_id": "<Toren>", "cost_dft": 50, "cost_ft": 5, "distance_dft": 50, "distance_ft": 5, "from": {"col": 6, "cover": "none", "placed": true, "row": 7}, "jump": "long", "on_turn": true, "round": 1, "row": 7}`},
		},
		{
			name: "the master's move fires where it ends", trap: [2]int{9, 7}, as: "master", col: 9,
			want: moveWant{col: 9, row: 7, usedDft: 0, lastDft: 0, event: `{"col": 9, "combatant_id": "<Toren>", "distance_dft": 150, "distance_ft": 15, "from": {"col": 6, "cover": "none", "placed": true, "row": 7}, "on_turn": true, "round": 1, "row": 7}`},
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			r := newTrapRig(t)
			r.trap(t, "Fosso Dourado", tc.trap[0], tc.trap[1], pit("1d6"))
			r.fight(t)
			r.h.roller.queue(4)
			u := r.caio
			if tc.as == "master" {
				u = r.master
			}
			fired := r.moveKindCount(t, "trap_triggered")
			r.frozenCheck(t, u, "Toren", tc.col, 7, tc.edit, nil, tc.want)
			if got := r.moveKindCount(t, "trap_triggered") - fired; got != 1 {
				t.Errorf("trap_triggered events written = %d, want 1", got)
			}
		})
	}
}
