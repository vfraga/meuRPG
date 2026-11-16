package play

import (
	"testing"

	"connectrpc.com/connect"

	campaignsv1 "github.com/PuraFome/meuRPG/backend/gen/meurpg/campaigns/v1"
	charactersv1 "github.com/PuraFome/meuRPG/backend/gen/meurpg/characters/v1"
	playv1 "github.com/PuraFome/meuRPG/backend/gen/meurpg/play/v1"
)

// The refund prompt of a weapon attack a reaction window held (favor_refund.go, hold_forget.go). The prompt is opened by the replay under the
// request's key; the roll is written later, by the player's answer to the die's question or to Lucky's, under the answer's own key. The undo
// of that roll, and the forgetting of the question, must still find the prompt of the spend.
//
// A roll the replay writes at once is not undone either, but for one answer: the master's answer of the window is written after it and
// closes the undo chain (chainAction), so only the roll that waited for a question is the last action. The exception is a Protection that was
// used: its answer is skipped by the chain (chainProtection), so the roll its replay wrote at once, with no key, is the last action and its
// undo reaches the prompt by what the event says (protectedRefund, below). A held spell attack asks no question in its replay
// (inspiration_held.go), so the weapon attack is the one roll that reaches the undo these ways.

// windowQuestion is what the replay of the attack asks the player before the roll is written.
type windowQuestion int

const (
	windowAsksTheDie windowQuestion = iota
	windowAsksLucky
)

func (q windowQuestion) String() string {
	if q == windowAsksLucky {
		return "Lucky"
	}
	return "the Bardic Inspiration die"
}

var windowQuestions = []windowQuestion{windowAsksTheDie, windowAsksLucky}

// windowRefund is a weapon attack a reaction window held, whose replay spent the Inspiração over a hidden advantage and waits for the
// player's answer: the master reads its prompt.
type windowRefund struct {
	f      *favorFight
	u      *user
	who    *charactersv1.Character
	q      windowQuestion
	holdID string // the question the replay kept
	prompt *playv1.FavorRefundPrompt
}

// newWindowRefund holds Toren's attack with a window, spends the Inspiração in the replay and leaves the question unanswered.
func newWindowRefund(t *testing.T, q windowQuestion) *windowRefund {
	t.Helper()
	p := favorPaths[0]
	f, faces := newFavorFight(t), []int{4, 15}
	if q == windowAsksLucky {
		f, faces = newFavorFightOf(t, "race:halfling"), []int{1, 15}
	} else {
		f.giveCombatantDie(t, p.actor, f.e)
	}
	f.setRules(t, enemyReactionsAlways)
	u, who := p.roller(f)
	f.give(t, who)
	f.hiddenEffect(t, p.actor, false, "["+attackAdv+"]")
	held, err := p.send(t, f, u, newKey(), true, nil)
	if err != nil || !held.heldByWindow {
		t.Fatalf("the spend on a roll a window holds = %+v, %v, want it held", held, err)
	}
	w := f.windowOf(t, f.master, playv1.ReactionKind_REACTION_KIND_MASTER_CHECK)
	f.h.roller.queue(faces...)
	f.mustAnswer(t, f.master, f.e, w.GetId(), passAnswer)
	prompts := f.prompts(t, f.master)
	if len(prompts) != 1 || f.favorHolds(t) {
		t.Fatalf("the master reads %d prompts and the Inspiração is there %v, want one prompt and it spent", len(prompts), f.favorHolds(t))
	}
	r := &windowRefund{f: f, u: u, who: who, q: q, prompt: prompts[0]}
	if err := f.h.pool.QueryRow(t.Context(), `SELECT id FROM inspiration_holds WHERE character_id = $1 AND answer_key IS NULL`, who.GetId()).Scan(&r.holdID); err != nil {
		t.Fatalf("the replay kept no question for %s: %v", q, err)
	}
	return r
}

// answered answers the question (the die kept, the 1 rolled again), which writes the roll under the answer's own key.
func (r *windowRefund) answered(t *testing.T) *windowRefund {
	t.Helper()
	answer := dieAnswer(false)
	if r.q == windowAsksLucky {
		r.f.h.roller.queue(12)
		answer = func(a *playv1.AnswerHeldRollRequest) {
			a.Answer = &playv1.AnswerHeldRollRequest_Lucky{Lucky: &playv1.LuckyAnswer{Reroll: true, Roll: &playv1.LuckyAnswer_RollInApp{RollInApp: true}}}
		}
	}
	if _, err := r.f.answerHeldRoll(t, r.u, r.holdID, answer); err != nil {
		t.Fatalf("AnswerHeldRoll(%s) error = %v", r.q, err)
	}
	return r
}

// spentAgain takes the row away: the character spent the Inspiração given back, or passed it, on something else.
func (r *windowRefund) spentAgain(t *testing.T) {
	t.Helper()
	r.f.execSQL(t, `DELETE FROM character_favor WHERE character_id = $1`, r.who.GetId())
}

// undoTheRoll is the master's Desfazer of the combat's last action, which is the roll the answer wrote.
func (r *windowRefund) undoTheRoll(t *testing.T) {
	t.Helper()
	id := r.f.log(t, r.f.master, r.f.e).GetUndoableEventId()
	if id == "" {
		t.Fatal("the roll is not the last action of the combat")
	}
	if err := r.f.undo(t, r.f.master, r.f.e, id); err != nil {
		t.Fatalf("UndoLastAction(the roll) error = %v", err)
	}
}

// forget turns the round, so the question is stale, and lets the roller's next roll meet it (the gate forgets it).
func (r *windowRefund) forget(t *testing.T) {
	t.Helper()
	f, p := r.f, favorPaths[0]
	f.setRules(t, func(rules *campaignsv1.TableRules) {
		rules.EnemyReactions = campaignsv1.EnemyReactionsRule_ENEMY_REACTIONS_RULE_ONLY_WHEN_POSSIBLE
	})
	if _, err := f.endTurn(t, f.master, f.e, true); err != nil {
		t.Fatalf("EndTurn() error = %v", err)
	}
	f.e = f.passTo(t, f.e, p.actor)
	if !f.holdExists(t, r.holdID) {
		t.Fatalf("the hold %s went before the next roll: the test does not reach the gate", r.holdID)
	}
	f.h.roller.queue(7)
	if _, err := p.send(t, f, r.u, newKey(), false, nil); err != nil {
		t.Fatalf("the next roll error = %v", err)
	}
	if f.holdExists(t, r.holdID) {
		t.Fatal("the stale question is still there after the roller's next roll")
	}
}

// eachWindowQuestion runs a test once for each question the replay can ask.
func eachWindowQuestion(t *testing.T, run func(t *testing.T, q windowQuestion)) {
	t.Helper()
	for _, q := range windowQuestions {
		t.Run(q.String(), func(t *testing.T) {
			t.Parallel()
			run(t, q)
		})
	}
}

// Scenario A: the master answered "Devolver", the character spent the Inspiração again, and the master undoes the roll: the roll's undo
// gives back nothing, since the Inspiração it spent was returned already.
func TestUndoOfAWindowHeldRollNeverGivesBackAReturnedRefund(t *testing.T) {
	t.Parallel()
	eachWindowQuestion(t, func(t *testing.T, q windowQuestion) {
		t.Helper()
		r := newWindowRefund(t, q).answered(t)
		if _, err := r.f.answerRefund(r.f.master, r.prompt.GetId(), true, newKey()); err != nil {
			t.Fatalf("AnswerFavorRefund(Devolver) error = %v", err)
		}
		r.spentAgain(t)
		r.undoTheRoll(t)
		if n := r.f.rows(t); n != 0 {
			t.Errorf("character_favor has %d rows after the undo of the roll, want 0: the Inspiração was returned once already", n)
		}
		if status := r.f.promptStatus(t, r.prompt.GetId()); status != "returned" {
			t.Errorf("the prompt is %q, want returned", status)
		}
	})
}

// Scenario B: the roll is undone first (the Inspiração is back, so the prompt closes as kept), and a "Devolver" that comes after the next
// spend is refused: the one spend is never refunded a second time.
func TestUndoOfAWindowHeldRollKeepsItsRefundPrompt(t *testing.T) {
	t.Parallel()
	eachWindowQuestion(t, func(t *testing.T, q windowQuestion) {
		t.Helper()
		r := newWindowRefund(t, q).answered(t)
		r.undoTheRoll(t)
		if !r.f.favorHolds(t) {
			t.Fatal("the undo of the roll did not give the Inspiração back")
		}
		if status := r.f.promptStatus(t, r.prompt.GetId()); status != "kept" {
			t.Errorf("the prompt of the undone roll is %q, want kept", status)
		}
		if got := r.f.prompts(t, r.f.master); len(got) != 0 {
			t.Errorf("the master still reads %v after the roll was undone, want no prompt", got)
		}
		r.spentAgain(t)
		_, err := r.f.answerRefund(r.f.master, r.prompt.GetId(), true, newKey())
		wantFavorBlocked(t, "a Devolver to the prompt of an undone roll", err, blockedPassClosed)
		if n := r.f.rows(t); n != 0 {
			t.Errorf("character_favor has %d rows, want 0: no second Inspiração", n)
		}
	})
}

// The question the replay kept is forgotten before it is answered: the Inspiração goes back with it and the prompt closes as kept, as it
// does for a roll no window held.
func TestForgottenWindowHeldRollKeepsItsRefundPrompt(t *testing.T) {
	t.Parallel()
	eachWindowQuestion(t, func(t *testing.T, q windowQuestion) {
		t.Helper()
		r := newWindowRefund(t, q)
		r.forget(t)
		if !r.f.favorHolds(t) {
			t.Fatal("the forgotten roll did not give the Inspiração back")
		}
		if status := r.f.promptStatus(t, r.prompt.GetId()); status != "kept" {
			t.Errorf("the prompt of the forgotten roll is %q, want kept", status)
		}
		r.spentAgain(t)
		_, err := r.f.answerRefund(r.f.master, r.prompt.GetId(), true, newKey())
		wantFavorBlocked(t, "a Devolver to the prompt of a forgotten roll", err, blockedPassClosed)
		if n := r.f.rows(t); n != 0 {
			t.Errorf("character_favor has %d rows, want 0", n)
		}
	})
}

// "Devolver" gave it back, the character spent it again, and the question the replay kept is forgotten: it gives nothing back.
func TestForgottenWindowHeldRollNeverGivesBackAReturnedRefund(t *testing.T) {
	t.Parallel()
	eachWindowQuestion(t, func(t *testing.T, q windowQuestion) {
		t.Helper()
		r := newWindowRefund(t, q)
		if _, err := r.f.answerRefund(r.f.master, r.prompt.GetId(), true, newKey()); err != nil {
			t.Fatalf("AnswerFavorRefund(Devolver) error = %v", err)
		}
		r.spentAgain(t)
		r.forget(t)
		if n := r.f.rows(t); n != 0 {
			t.Errorf("character_favor has %d rows, want 0: the forgotten roll gave back an Inspiração that was returned already", n)
		}
	})
}

// zoneSaveRefund is the spend of a catch's save at the cast of a wall of fire over a hidden advantage: the master reads its prompt. The save's
// event is written under the key of the request that rolled it, the key the spend was made under, so no path writes it without one: the helper
// nulls the key by SQL, to show that the undo of the cast finds the prompt by what the event itself says (its favor_prompt) and not by its key.
func zoneSaveRefund(t *testing.T) (*favorFight, *playv1.FavorRefundPrompt) {
	t.Helper()
	c := wallCave(t)
	c.wallFight(t)
	e := c.get(t, c.master)
	f := favorFightOf(t, c.armed, e)
	c.mustCastArea(t, c.ana, "Pensantus", wallOfFireKey, slotOfLevel(4), at(5, 8), nil, northWall(playv1.WallSide_WALL_SIDE_LEFT))
	w := c.windowOf(t, c.caio, playv1.ReactionKind_REACTION_KIND_EFFECT_SAVE)
	if w == nil {
		t.Fatal("Toren's player has no question: the wall caught his character at the cast")
	}
	f.execSQL(t, `INSERT INTO character_favor (character_id, campaign_id) SELECT $1, campaign_id FROM characters WHERE id = $1`, c.toren.GetId())
	f.hiddenEffect(t, "Toren", false, `[{"kind":"save_advantage","abilities":["dex"]}]`)
	c.h.roller.queue(4, 4, 4, 4, 4, 4, 4)
	if _, err := c.rollEffectSave(t, c.caio, e, w.GetId(), func(r *playv1.RollEffectSaveRequest) {
		r.Roll, r.SpendFavor = &playv1.RollEffectSaveRequest_RollInApp{RollInApp: true}, true
	}); err != nil {
		t.Fatalf("the spend on the zone's save error = %v", err)
	}
	prompts := f.prompts(t, f.master)
	if len(prompts) != 1 {
		t.Fatalf("the master reads %d refund prompts of the zone's save, want 1", len(prompts))
	}
	f.execSQL(t, `UPDATE session_events SET idempotency_key = NULL WHERE kind = $1 AND game_session_id = $2`, eventLastingSaved, f.sessionID)
	return f, prompts[0]
}

// The undo of the cast gives back the Inspiração of the zone's save and closes its prompt as kept, though the save's event has no key.
func TestUndoOfTheCastKeepsThePromptOfAZoneSaveWithNoKey(t *testing.T) {
	t.Parallel()
	f, p := zoneSaveRefund(t)
	if err := f.undo(t, f.master, f.e, f.log(t, f.master, f.e).GetUndoableEventId()); err != nil {
		t.Fatalf("UndoLastAction(the cast) error = %v", err)
	}
	if !f.favorHolds(t) {
		t.Fatal("the undo of the cast did not give the Inspiração back")
	}
	if status := f.promptStatus(t, p.GetId()); status != "kept" {
		t.Errorf("the prompt of the undone cast's save is %q, want kept", status)
	}
}

// "Devolver" gave it back and the character spent it again: the undo of the cast gives nothing back.
func TestUndoOfTheCastNeverGivesBackAReturnedRefundOfAZoneSave(t *testing.T) {
	t.Parallel()
	f, p := zoneSaveRefund(t)
	if _, err := f.answerRefund(f.master, p.GetId(), true, newKey()); err != nil {
		t.Fatalf("AnswerFavorRefund(Devolver) error = %v", err)
	}
	f.execSQL(t, `DELETE FROM character_favor WHERE campaign_id = $1`, f.campaignID)
	if err := f.undo(t, f.master, f.e, f.log(t, f.master, f.e).GetUndoableEventId()); err != nil {
		t.Fatalf("UndoLastAction(the cast) error = %v", err)
	}
	if n := f.rows(t); n != 0 {
		t.Errorf("character_favor has %d rows after the undo of the cast, want 0: the Inspiração was returned once already", n)
	}
}

// protectedRefund is Toren's attack, a spend over a hidden advantage, with a shield-bearer of the master's next to the target: the attack waits
// for the Protection window, the master uses it, and the replay (the roll with the disadvantage) spends the Inspiração and opens the prompt.
// The Protection's own answer is written beside the roll and never closes the undo chain (chainProtection), so unlike the answer of the
// other windows it leaves the replayed roll as the combat's last action, and that roll is written with no key.
func protectedRefund(t *testing.T) (*favorFight, *playv1.FavorRefundPrompt) {
	t.Helper()
	a := newCasters(t)
	guard, err := a.master.characters.CreateCharacter(t.Context(), connect.NewRequest(&charactersv1.CreateCharacterRequest{
		CampaignId: a.campaignID, Kind: charactersv1.CharacterKind_CHARACTER_KIND_BOSS, Name: "Guardião", Sheet: protector().sheet(),
	}))
	if err != nil {
		t.Fatalf("CreateCharacter(Guardião) error = %v", err)
	}
	e := a.start(t, plan{
		npcs:     []*playv1.Participant{{CharacterId: a.goblin.GetId()}, {CharacterId: guard.Msg.GetCharacter().GetId()}},
		npcRolls: []int{3, 2},
		players:  map[string]int32{"Toren": 18, "Pensantus": 10, "Brisa": 1},
		reveal:   []string{"Goblin", "Guardião"},
		at:       map[string][2]int32{"Toren": {3, 3}, "Goblin": {4, 3}, "Guardião": {5, 4}, "Pensantus": {10, 3}, "Brisa": {4, 9}},
	})
	f := favorFightOf(t, a, e)
	f.give(t, a.toren)
	f.hiddenEffect(t, "Toren", false, "["+attackAdv+"]")
	held, err := favorPaths[0].send(t, f, a.caio, newKey(), true, nil)
	if err != nil || !held.heldByWindow {
		t.Fatalf("the spend on an attack the Guardião may protect against = %+v, %v, want it held for the window", held, err)
	}
	w := a.windowOf(t, a.master, playv1.ReactionKind_REACTION_KIND_PROTECTION)
	if w == nil {
		t.Fatalf("the master has no Protection window: %v", a.windows(t, a.master))
	}
	a.h.roller.queue(15)
	a.mustAnswer(t, a.master, f.e, w.GetId(), useAnswer(0))
	prompts := f.prompts(t, f.master)
	if len(prompts) != 1 || f.favorHolds(t) {
		t.Fatalf("the master reads %d prompts and the Inspiração is there %v, want one prompt and it spent", len(prompts), f.favorHolds(t))
	}
	var keyless int
	if err := f.h.pool.QueryRow(t.Context(), `SELECT count(*) FROM session_events WHERE kind = $1 AND game_session_id = $2 AND idempotency_key IS NULL`, eventAttackRolled, f.sessionID).Scan(&keyless); err != nil || keyless != 1 {
		t.Fatalf("the replay wrote %d keyless rolls (%v), want the one the undo reads", keyless, err)
	}
	return f, prompts[0]
}

// wantUndoableIsTheKeylessRoll says the event the master's Desfazer reaches is the attack_rolled the replay wrote with no key, and no other
// event (not the Protection's answer written beside it, not a roll under a key): the undo that follows reads the prompt by what that event
// says, and these tests mean to prove that and nothing else.
func (f *favorFight) wantUndoableIsTheKeylessRoll(t *testing.T, undoable string) {
	t.Helper()
	var id, kind string
	var keyless bool
	if err := f.h.pool.QueryRow(t.Context(), `SELECT id::text, kind, idempotency_key IS NULL FROM session_events WHERE id::text = $1 AND game_session_id = $2`, undoable, f.sessionID).Scan(&id, &kind, &keyless); err != nil {
		t.Fatalf("the event the master can undo (%q) is not an event of the session: %v", undoable, err)
	}
	if kind != eventAttackRolled || !keyless {
		t.Fatalf("the master can undo the event %s of kind %q (keyless %v), want the keyless %s the replay wrote", id, kind, keyless, eventAttackRolled)
	}
}

// A Protection that was used does not close the chain, so the replayed roll (no key) is the last action: the "Devolver" that came before is
// found by what the event says, and the undo gives nothing back.
func TestUndoOfAProtectedRollNeverGivesBackAReturnedRefund(t *testing.T) {
	t.Parallel()
	f, p := protectedRefund(t)
	if _, err := f.answerRefund(f.master, p.GetId(), true, newKey()); err != nil {
		t.Fatalf("AnswerFavorRefund(Devolver) error = %v", err)
	}
	f.execSQL(t, `DELETE FROM character_favor WHERE character_id = $1`, f.toren.GetId()) // spent again, on another roll
	id := f.log(t, f.master, f.e).GetUndoableEventId()
	f.wantUndoableIsTheKeylessRoll(t, id)
	if err := f.undo(t, f.master, f.e, id); err != nil {
		t.Fatalf("UndoLastAction(the roll) error = %v", err)
	}
	if n := f.rows(t); n != 0 {
		t.Errorf("character_favor has %d rows after the undo of the roll, want 0: the Inspiração was returned once already", n)
	}
	if status := f.promptStatus(t, p.GetId()); status != "returned" {
		t.Errorf("the prompt is %q, want returned", status)
	}
}

// The roll is undone first: the Inspiração is back and the prompt closes as kept, so a "Devolver" that comes after the next spend is refused.
func TestUndoOfAProtectedRollKeepsItsRefundPrompt(t *testing.T) {
	t.Parallel()
	f, p := protectedRefund(t)
	id := f.log(t, f.master, f.e).GetUndoableEventId()
	f.wantUndoableIsTheKeylessRoll(t, id)
	if err := f.undo(t, f.master, f.e, id); err != nil {
		t.Fatalf("UndoLastAction(the roll) error = %v", err)
	}
	if !f.favorHolds(t) {
		t.Fatal("the undo of the roll did not give the Inspiração back")
	}
	if status := f.promptStatus(t, p.GetId()); status != "kept" {
		t.Errorf("the prompt of the undone roll is %q, want kept", status)
	}
	if got := f.prompts(t, f.master); len(got) != 0 {
		t.Errorf("the master still reads %v after the roll was undone, want no prompt", got)
	}
	f.execSQL(t, `DELETE FROM character_favor WHERE character_id = $1`, f.toren.GetId())
	_, err := f.answerRefund(f.master, p.GetId(), true, newKey())
	wantFavorBlocked(t, "a Devolver to the prompt of an undone roll", err, blockedPassClosed)
	if n := f.rows(t); n != 0 {
		t.Errorf("character_favor has %d rows, want 0: no second Inspiração", n)
	}
}
