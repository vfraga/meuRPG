package play

import (
	"slices"
	"strings"
	"testing"

	playv1 "github.com/PuraFome/meuRPG/backend/gen/meurpg/play/v1"
)

// A request may write its idempotency key in upper case (uuid.Parse accepts it): the prompt of the spend, the held roll and the roll's event
// must all keep the one parsed key, or a lookup by key finds nothing and the Inspiração is given back twice.

// upperHeldCast is Brisa's spell attack with the Inspiração over a hidden advantage while she holds a Bardic Inspiration die, sent with its key
// in upper case: the roll waits for the die's question, and the master reads the prompt of the spend.
type upperHeldCast struct {
	f      *favorFight
	key    string
	holdID string
	prompt *playv1.FavorRefundPrompt
}

func newUpperHeldCast(t *testing.T) *upperHeldCast {
	t.Helper()
	f := newFavorFight(t)
	p := favorPaths[1]
	p.ready(t, f)
	f.give(t, f.bri)
	f.giveCombatantDie(t, "Brisa", f.e)
	f.hiddenEffect(t, "Brisa", false, "["+attackAdv+"]")
	f.h.roller.queue(4, 15)
	key := strings.ToUpper(newKey())
	held, err := p.send(t, f, f.bia, key, true, nil)
	if err != nil || held.holdID == "" {
		t.Fatalf("the cast with the Inspiração and a die = %+v, %v, want the die's question", held, err)
	}
	prompts := f.prompts(t, f.master)
	if len(prompts) != 1 {
		t.Fatalf("the master reads %d prompts of the held cast, want 1", len(prompts))
	}
	return &upperHeldCast{f: f, key: key, holdID: held.holdID, prompt: prompts[0]}
}

// returnedAndPassed is "Devolver" and then Brisa's pass of it to Pensantus, accepted: the one Inspiração of the table is Pensantus's.
func (c *upperHeldCast) returnedAndPassed(t *testing.T) {
	t.Helper()
	f := c.f
	if _, err := f.answerRefund(f.master, c.prompt.GetId(), true, newKey()); err != nil {
		t.Fatalf("AnswerFavorRefund(Devolver) error = %v", err)
	}
	pass := f.mustAsk(t, f.bri.GetId(), f.pens.GetId())
	if _, err := f.answer(pass.GetId(), true); err != nil {
		t.Fatalf("AnswerFavorPass(Aceitar) error = %v", err)
	}
	if n := f.rows(t); n != 1 || !f.has(t, f.pens.GetId()) {
		t.Fatalf("character_favor has %d rows (Pensantus has it %v), want the one Pensantus got", n, f.has(t, f.pens.GetId()))
	}
}

func (c *upperHeldCast) wantOnlyPensantus(t *testing.T, what string) {
	t.Helper()
	if n := c.f.rows(t); n != 1 || !c.f.has(t, c.f.pens.GetId()) || c.f.has(t, c.f.bri.GetId()) {
		t.Errorf("%s: character_favor has %d rows (Pensantus %v, Brisa %v), want only the one Pensantus got", what, n, c.f.has(t, c.f.pens.GetId()), c.f.has(t, c.f.bri.GetId()))
	}
}

// The held cast's roll is answered after the pass and undone: the Inspiração it spent was returned already, so the undo gives nothing back.
// What it guards: the id path. The prompt is found by the id the event and the hold carry; it passes with the cast's key as typed too (checked by
// putting the raw key back in spellModes), so it is not the proof of the parsed key.
func TestUndoOfACastSentWithAnUpperCaseKeyNeverGivesBackAReturnedRefund(t *testing.T) {
	t.Parallel()
	c := newUpperHeldCast(t)
	c.returnedAndPassed(t)
	if _, err := c.f.answerHeldRoll(t, c.f.bia, c.holdID, dieAnswer(false)); err != nil {
		t.Fatalf("AnswerHeldRoll() error = %v", err)
	}
	if err := c.f.undo(t, c.f.master, c.f.e, c.f.log(t, c.f.master, c.f.e).GetUndoableEventId()); err != nil {
		t.Fatalf("UndoLastAction(the cast) error = %v", err)
	}
	c.wantOnlyPensantus(t, "the undo of the cast")
	if status := c.f.promptStatus(t, c.prompt.GetId()); status != "returned" {
		t.Errorf("the prompt is %q, want returned", status)
	}
}

// The held cast is forgotten (the round turned) after the pass: it gives nothing back.
// What it guards: the id path (the hold's favor_prompt_id), as the test above; it passes with the cast's key as typed too, so it is not the proof
// of the parsed key.
func TestForgottenCastSentWithAnUpperCaseKeyNeverGivesBackAReturnedRefund(t *testing.T) {
	t.Parallel()
	c := newUpperHeldCast(t)
	c.returnedAndPassed(t)
	f := c.f
	if _, err := f.endTurn(t, f.master, f.e, true); err != nil {
		t.Fatalf("EndTurn() error = %v", err)
	}
	f.e = f.passTo(t, f.e, "Brisa")
	if !f.holdExists(t, c.holdID) {
		t.Fatalf("the hold %s went before the next roll: the test does not reach the gate", c.holdID)
	}
	f.h.roller.queue(7, 8)
	if _, err := favorPaths[1].send(t, f, f.bia, newKey(), false, nil); err != nil {
		t.Fatalf("the next cast error = %v", err)
	}
	if f.holdExists(t, c.holdID) {
		t.Fatal("the stale question is still there after the roller's next roll")
	}
	c.wantOnlyPensantus(t, "the forgotten cast")
}

// A retry of the held cast, with the key as typed the first time, is the same cast: it answers the die's question again and does not spend.
// What it guards: the retry of a held cast whose key was typed in upper case; it passes with the cast's key as typed too (the hold is found by its
// UUID column, which reads either spelling), so it is not the proof of the parsed key.
func TestRetryOfAHeldCastSentWithAnUpperCaseKeyDoesNotSpendAgain(t *testing.T) {
	t.Parallel()
	c := newUpperHeldCast(t)
	c.returnedAndPassed(t) // Brisa has none now: a spend would be HAS_NONE
	f := c.f
	f.h.roller.queue(4, 15)
	again, err := favorPaths[1].send(t, f, f.bia, c.key, true, nil)
	if err != nil {
		t.Fatalf("the retry of the held cast error = %v, want the same question", err)
	}
	if again.holdID != c.holdID {
		t.Errorf("the retry asks the question %q, want the first one, %q", again.holdID, c.holdID)
	}
}

// A weapon attack that spent the Inspiração and waits for a die's question is answered, and the client sends the request again with its key in
// upper case, as the first time: the retry gives the attack the answer wrote and spends nothing more. What it guards: the retry of an answered
// hold under a key as typed. It cannot be red on the lookup alone: retryOfHeldAttack (held_registry.go) reads the hold by the key as typed, and
// inspiration_holds.idempotency_key is a UUID column (00244), so the database reads an upper-case key as the parsed one and finds the hold. The
// hash of the retry differs from the stored one (the hold pinned the mode), so the retry does reach that lookup.
func TestRetryOfAnAnsweredWeaponAttackSentWithAnUpperCaseKeyGivesTheWrittenAttack(t *testing.T) {
	t.Parallel()
	f := newFavorFight(t)
	f.give(t, f.toren)
	f.giveCombatantDie(t, "Toren", f.e)
	f.hiddenEffect(t, "Toren", false, "["+attackAdv+"]")
	p := favorPaths[0]
	key := strings.ToUpper(newKey())
	f.h.roller.queue(4, 15)
	held, err := p.send(t, f, f.caio, key, true, nil)
	if err != nil || held.holdID == "" {
		t.Fatalf("the attack with the Inspiração and a die = %+v, %v, want the die's question", held, err)
	}
	res, err := f.answerHeldRoll(t, f.caio, held.holdID, dieAnswer(false))
	if err != nil {
		t.Fatalf("AnswerHeldRoll() error = %v", err)
	}
	answered := p.answered(res)
	again, err := p.send(t, f, f.caio, key, true, nil)
	if err != nil {
		t.Fatalf("the retry of the answered attack error = %v, want the attack the answer wrote", err)
	}
	if !slices.Equal(again.faces, answered.faces) || again.holdID != "" {
		t.Errorf("the retry = %+v, want the attack the answer wrote (%+v) and no question", again, answered)
	}
	if f.favorHolds(t) || len(f.prompts(t, f.master)) != 1 || f.eventsOf(t, eventFavorRefundPrompted) != 1 {
		t.Errorf("the Inspiração is there %v, the master reads %d prompts, %d prompted events, want it spent once and one prompt", f.favorHolds(t), len(f.prompts(t, f.master)), f.eventsOf(t, eventFavorRefundPrompted))
	}
}
