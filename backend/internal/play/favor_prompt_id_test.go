package play

import (
	"testing"

	playv1 "github.com/PuraFome/meuRPG/backend/gen/meurpg/play/v1"
)

// A roll's refund prompt is found by its own id, never by the key it was spent under: two prompts of one character can share a key (a
// held roll whose question went stale is forgotten, and the client sends its key again, which rolls and spends afresh).

// twoSpendsUnderOneKey is Toren's attack with the Inspiração over a hidden advantage under key K, with a Bardic Inspiration die: the
// master answers "Devolver" to the first prompt, the question goes stale and is forgotten by the roller's next roll, and the client sends K
// again, which rolls and spends afresh and opens a second prompt. The second roll waits for the die's question.
func twoSpendsUnderOneKey(t *testing.T) (f *favorFight, first, second *playv1.FavorRefundPrompt, holdID string) {
	t.Helper()
	f = newFavorFight(t)
	f.give(t, f.toren)
	f.giveCombatantDie(t, "Toren", f.e)
	f.hiddenEffect(t, "Toren", false, "["+attackAdv+"]")
	key := newKey()
	f.h.roller.queue(4, 15)
	held, err := favorPaths[0].send(t, f, f.caio, key, true, nil)
	if err != nil || held.holdID == "" {
		t.Fatalf("the first attack = %+v, %v, want the die's question", held, err)
	}
	prompts := f.prompts(t, f.master)
	if len(prompts) != 1 {
		t.Fatalf("the master reads %d prompts of the first attack, want 1", len(prompts))
	}
	first = prompts[0]
	if _, err := f.answerRefund(f.master, first.GetId(), true, newKey()); err != nil {
		t.Fatalf("AnswerFavorRefund(Devolver) error = %v", err)
	}
	if _, err := f.endTurn(t, f.master, f.e, true); err != nil {
		t.Fatalf("EndTurn() error = %v", err)
	}
	f.e = f.passTo(t, f.e, "Toren") // the round turned: the first question is stale
	f.h.roller.queue(7, 8)
	next, err := favorPaths[0].send(t, f, f.caio, newKey(), false, nil) // the roller's next roll meets the stale question and forgets it
	if err != nil || next.holdID == "" {
		t.Fatalf("the next attack = %+v, %v, want the die's question", next, err)
	}
	if f.holdExists(t, held.holdID) {
		t.Fatal("the stale question of the first attack is still there after the roller's next roll")
	}
	if _, err := f.answerHeldRoll(t, f.caio, next.holdID, dieAnswer(false)); err != nil {
		t.Fatalf("AnswerHeldRoll(the next attack) error = %v", err)
	}
	f.h.roller.queue(4, 15)
	again, err := favorPaths[0].send(t, f, f.caio, key, true, nil) // the client sends K again: a new roll, a new spend
	if err != nil || again.holdID == "" || again.holdID == held.holdID {
		t.Fatalf("the second attack = %+v, %v, want a new question", again, err)
	}
	prompts = f.prompts(t, f.master)
	if len(prompts) != 1 || prompts[0].GetId() == first.GetId() {
		t.Fatalf("the master reads %v after the second spend, want only its own prompt", prompts)
	}
	return f, first, prompts[0], again.holdID
}

func (f *favorFight) wantPrompts(t *testing.T, what string, first, second *playv1.FavorRefundPrompt) {
	t.Helper()
	if !f.favorHolds(t) {
		t.Errorf("%s: the Inspiração of the second roll did not come back: the first prompt's answer was read for it", what)
	}
	if status := f.promptStatus(t, second.GetId()); status != "kept" {
		t.Errorf("%s: the second prompt is %q, want kept", what, status)
	}
	if status := f.promptStatus(t, first.GetId()); status != "returned" {
		t.Errorf("%s: the first prompt is %q, want returned", what, status)
	}
}

// The undo of the second roll gives its Inspiração back and closes its prompt: the returned answer of the first belongs to the first roll.
func TestUndoOfARollFindsItsOwnPromptWhenAnEarlierSpendUnderTheSameKeyWasReturned(t *testing.T) {
	t.Parallel()
	f, first, second, holdID := twoSpendsUnderOneKey(t)
	if _, err := f.answerHeldRoll(t, f.caio, holdID, dieAnswer(false)); err != nil {
		t.Fatalf("AnswerHeldRoll() error = %v", err)
	}
	if err := f.undo(t, f.master, f.e, f.log(t, f.master, f.e).GetUndoableEventId()); err != nil {
		t.Fatalf("UndoLastAction(the second roll) error = %v", err)
	}
	f.wantPrompts(t, "the undo of the second roll", first, second)
}

// The second roll's question is forgotten before it is answered: the same, for the hold.
func TestForgottenHoldFindsItsOwnPromptWhenAnEarlierSpendUnderTheSameKeyWasReturned(t *testing.T) {
	t.Parallel()
	f, first, second, holdID := twoSpendsUnderOneKey(t)
	f.dropTheHold(t, holdID)
	f.wantPrompts(t, "the forgotten question of the second roll", first, second)
}
