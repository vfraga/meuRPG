package play

import (
	"testing"

	"google.golang.org/protobuf/proto"

	playv1 "github.com/PuraFome/meuRPG/backend/gen/meurpg/play/v1"
)

// A spell attack cast with the master's Inspiração that a reaction window holds, by a caster that holds a Bardic Inspiration die: the hold only
// checks the Inspiração is there, the replay spends it and asks the die's question under the cast's key (as the window's own), and the
// answer, with the die or without it, settles the cast once. What it guards: the Inspiração is spent once (the replay's), the answer
// neither spends nor refunds it, the event says the roll was made with it, and the undo of the cast gives it back once.
func TestFavorOnASpellAttackAWindowHeldThatAsksForTheDie(t *testing.T) {
	t.Parallel()
	for _, use := range []bool{true, false} {
		name := map[bool]string{true: "the die is used", false: "the die is kept"}[use]
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			p := favorPaths[1]
			f := newFavorFight(t)
			f.setRules(t, enemyReactionsAlways)
			p.ready(t, f)
			u, who := p.roller(f)
			f.giveCombatantDie(t, p.actor, f.e)
			f.give(t, who)

			key := newKey()
			held, err := p.send(t, f, u, key, true, nil)
			if err != nil || !held.heldByWindow {
				t.Fatalf("the spend on a cast a window holds = %+v, %v, want it held", held, err)
			}
			if !f.favorHolds(t) {
				t.Fatal("the Inspiração was spent while the cast waits for the window")
			}
			f.h.roller.queue(3, 17) // the advantage the Inspiração gives: two d20
			f.mustAnswer(t, f.master, f.e, f.windowOf(t, f.master, playv1.ReactionKind_REACTION_KIND_MASTER_CHECK).GetId(), passAnswer)
			if f.favorHolds(t) {
				t.Fatal("the replay did not spend the Inspiração")
			}
			row := f.holdRowOf(t, key)
			if !row.spent || !row.window || !row.asked {
				t.Fatalf("the replay's hold = %+v, want the die's question open, with the Inspiração spent and the window's mark", row)
			}
			if n := f.countOf(t, `SELECT count(*) FROM session_events WHERE kind = $1 AND NOT COALESCE((payload->'reaction'->>'hold')::bool, false)`, eventSpellCast); n != 0 {
				t.Fatalf("%s events before the answer = %d, want 0", eventSpellCast, n)
			}

			offers := f.listHeldRolls(t, u)
			if len(offers) != 1 || offers[0].GetDie() == nil {
				t.Fatalf("the questions after the window = %v, want the die's", offers)
			}
			answerKey := newKey()
			if use {
				f.h.roller.queue(5)
			}
			out, err := f.answerHeldRollKey(t, u, offers[0].GetHoldId(), answerKey, dieAnswer(use))
			if err != nil {
				t.Fatalf("AnswerHeldRoll() error = %v", err)
			}
			// The answer again gives the stored cast (which does not keep the sources of the roll's mode, as with no window) and spends nothing.
			f.h.roller.queue(heldSentinel)
			again, err := f.answerHeldRollKey(t, u, offers[0].GetHoldId(), answerKey, dieAnswer(use))
			if first, second := out.GetSpellCast().GetCast().GetTargets(), again.GetSpellCast().GetCast().GetTargets(); err != nil || len(second) != 1 ||
				!proto.Equal(first[0].GetAttackRoll(), second[0].GetAttackRoll()) || f.h.roller.pending() != 1 || f.favorHolds(t) {
				t.Errorf("the answer again = %v, %v (rolls left %d, Inspiração there %v), want the same roll and nothing rolled or spent", again.GetSpellCast().GetCast().GetTargets(), err, f.h.roller.pending(), f.favorHolds(t))
			}
			f.h.roller.drain()
			got := p.answered(out)
			if len(got.faces) != 2 || got.mode != playv1.RollMode_ROLL_MODE_ADVANTAGE || !got.hasFavorSource() {
				t.Errorf("the answered cast = %+v, want the two dice with the advantage and the source of the Inspiração", got)
			}
			if f.favorHolds(t) || f.eventFavor(t, p.kind) != "true" {
				t.Errorf("after the answer the Inspiração is there = %v and the event says %q, want it spent once and the event to say so", f.favorHolds(t), f.eventFavor(t, p.kind))
			}
			if n := f.countOf(t, `SELECT count(*) FROM session_events WHERE kind = $1 AND NOT COALESCE((payload->'reaction'->>'hold')::bool, false)`, eventSpellCast); n != 1 {
				t.Errorf("%s events = %d, want 1", eventSpellCast, n)
			}
			if dice := len(out.GetSpellCast().GetCast().GetTargets()[0].GetBonusDice()); (dice == 1) != use {
				t.Errorf("the cast has %d bonus dice, want one when the die is used (%v)", dice, use)
			}

			id := f.log(t, f.master, f.e).GetUndoableEventId()
			if id == "" {
				t.Fatal("the cast is not undoable")
			}
			if err := f.undo(t, f.master, f.e, id); err != nil {
				t.Fatalf("UndoLastAction() error = %v", err)
			}
			if got := f.rows(t); got != 1 {
				t.Errorf("character_favor has %d rows after the undo, want the Inspiração back once", got)
			}
		})
	}
}
