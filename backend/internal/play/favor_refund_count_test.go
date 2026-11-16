package play

import (
	"testing"

	"connectrpc.com/connect"

	playv1 "github.com/PuraFome/meuRPG/backend/gen/meurpg/play/v1"
)

// The counting rules of the refund prompts: a roll's prompt closes for its own roller alone, and an Inspiração a roll spent is
// given back once.

// heldRefundFight is Toren's attack made with the Inspiração and a Bardic Inspiration die over a hidden advantage: the roll waits for the
// die's question, and the master reads the prompt of that spend.
func heldRefundFight(t *testing.T) (f *favorFight, holdID string, prompt *playv1.FavorRefundPrompt) {
	t.Helper()
	f = newFavorFight(t)
	f.give(t, f.toren)
	f.giveCombatantDie(t, "Toren", f.e)
	f.hiddenEffect(t, "Toren", false, "["+attackAdv+"]")
	f.h.roller.queue(4, 15)
	held, err := favorPaths[0].send(t, f, f.caio, newKey(), true, nil)
	if err != nil || held.holdID == "" {
		t.Fatalf("the attack with the Inspiração and a die = %+v, %v, want the die's question", held, err)
	}
	prompts := f.prompts(t, f.master)
	if len(prompts) != 1 {
		t.Fatalf("the master reads %d prompts of the held roll, want 1", len(prompts))
	}
	return f, held.holdID, prompts[0]
}

// dropTheHold turns the round and makes the next roll of Toren, which forgets the stale hold.
func (f *favorFight) dropTheHold(t *testing.T, holdID string) {
	t.Helper()
	if _, err := f.endTurn(t, f.master, f.e, true); err != nil {
		t.Fatalf("EndTurn() error = %v", err)
	}
	f.e = f.passTo(t, f.e, "Toren")
	if !f.holdExists(t, holdID) {
		t.Fatalf("the hold %s went before the next roll: the test does not reach the gate", holdID)
	}
	f.h.roller.queue(7)
	if _, err := favorPaths[0].send(t, f, f.caio, newKey(), false, nil); err != nil {
		t.Fatalf("the next attack error = %v", err)
	}
}

// otherPromptOfTheRoll opens, by SQL, a prompt of another member of the same group check for the same label, which the roll's own prompt must
// leave open when it closes. It is by SQL because the other member's spend is another roll with a prompt of its own, which the API makes
// only by a request of that member (favor_groupcheck_refund_test.go drives the real members of a group check); this guards that closing a
// roll's prompt (by its id) never reaches the prompts of the other characters.
func (f *favorFight) otherPromptOfTheRoll(t *testing.T, of *playv1.FavorRefundPrompt, characterID string) string {
	t.Helper()
	var id string
	if err := f.h.pool.QueryRow(t.Context(), `INSERT INTO favor_refund_prompts (campaign_id, game_session_id, character_id, roll_label_pt)
		SELECT campaign_id, game_session_id, $2, roll_label_pt FROM favor_refund_prompts WHERE id = $1 RETURNING id`, of.GetId(), characterID).Scan(&id); err != nil {
		t.Fatalf("open the other member's prompt: %v", err)
	}
	return id
}

// The held roll of a group check is forgotten: Toren's prompt closes, and Pensantus's, of the same kind of roll, stays open.
func TestForgettingARollKeepsOnlyItsRollersPrompt(t *testing.T) {
	t.Parallel()
	f, holdID, p := heldRefundFight(t)
	other := f.otherPromptOfTheRoll(t, p, f.pens.GetId())
	f.dropTheHold(t, holdID)
	if status := f.promptStatus(t, p.GetId()); status != "kept" {
		t.Errorf("the roller's prompt is %q, want kept", status)
	}
	if status := f.promptStatus(t, other); status != "open" {
		t.Errorf("the other member's prompt is %q, want open: it is another character's spend", status)
	}
}

// The undo of the roll closes the prompt of the character that rolled it, and the other member's stays open.
func TestUndoOfARollKeepsOnlyItsRollersPrompt(t *testing.T) {
	t.Parallel()
	f, p := refundFight(t)
	other := f.otherPromptOfTheRoll(t, p, f.pens.GetId())
	if err := f.undo(t, f.master, f.e, f.undoableLine(t).GetId()); err != nil {
		t.Fatalf("UndoLastAction(the roll) error = %v", err)
	}
	if status := f.promptStatus(t, p.GetId()); status != "kept" {
		t.Errorf("the roller's prompt is %q, want kept", status)
	}
	if status := f.promptStatus(t, other); status != "open" {
		t.Errorf("the other member's prompt is %q, want open: it is another character's spend", status)
	}
}

// "Devolver" gave the Inspiração back, and Toren passed it away: the hold of that roll dies after, and gives nothing back, so the table
// has the one Inspiração it had.
func TestReturnedRefundIsNotGivenBackAgainWhenTheHoldDiesAfterAPass(t *testing.T) {
	t.Parallel()
	f, holdID, p := heldRefundFight(t)
	if _, err := f.answerRefund(f.master, p.GetId(), true, newKey()); err != nil {
		t.Fatalf("AnswerFavorRefund(Devolver) error = %v", err)
	}
	pass := f.mustAsk(t, f.toren.GetId(), f.pens.GetId())
	if _, err := f.answer(pass.GetId(), true); err != nil {
		t.Fatalf("AnswerFavorPass(Aceitar) error = %v", err)
	}
	f.dropTheHold(t, holdID)
	if n := f.rows(t); n != 1 || !f.has(t, f.pens.GetId()) {
		t.Errorf("character_favor has %d rows (Pensantus has it %v), want the one Pensantus got", n, f.has(t, f.pens.GetId()))
	}
}

// "Devolver" gave it back and Toren spent it on something else (the row is gone): the dying hold of the first roll gives nothing back.
func TestReturnedRefundIsNotGivenBackAgainWhenTheHoldDiesAfterASpend(t *testing.T) {
	t.Parallel()
	f, holdID, p := heldRefundFight(t)
	if _, err := f.answerRefund(f.master, p.GetId(), true, newKey()); err != nil {
		t.Fatalf("AnswerFavorRefund(Devolver) error = %v", err)
	}
	f.execSQL(t, `DELETE FROM character_favor WHERE character_id = $1`, f.toren.GetId()) // spent again, on another roll
	f.dropTheHold(t, holdID)
	if n := f.rows(t); n != 0 {
		t.Errorf("character_favor has %d rows, want 0: the forgotten hold gave back an Inspiração that was already returned and spent", n)
	}
}

// The undo of the roll after "Devolver" gives nothing back either, and the prompt keeps its answer.
func TestReturnedRefundIsNotGivenBackAgainByTheUndoOfTheRoll(t *testing.T) {
	t.Parallel()
	f, p := refundFight(t)
	if _, err := f.answerRefund(f.master, p.GetId(), true, newKey()); err != nil {
		t.Fatalf("AnswerFavorRefund(Devolver) error = %v", err)
	}
	f.execSQL(t, `DELETE FROM character_favor WHERE character_id = $1`, f.toren.GetId()) // spent again, on another roll
	if err := f.undo(t, f.master, f.e, f.undoableLine(t).GetId()); err != nil {
		t.Fatalf("UndoLastAction(the roll) error = %v", err)
	}
	if n := f.rows(t); n != 0 {
		t.Errorf("character_favor has %d rows, want 0: the undo gave back an Inspiração that was already returned and spent", n)
	}
	if status := f.promptStatus(t, p.GetId()); status != "returned" {
		t.Errorf("the prompt is %q, want returned", status)
	}
}

// reserve sends a player character back to the reserve: nobody owns it any more.
func (f *favorFight) reserve(t *testing.T, id string) {
	t.Helper()
	f.execSQL(t, `UPDATE characters SET reserved = true, player_user_id = NULL WHERE id = $1`, id)
}

// The master undoes an accepted pass whose giver went back to the reserve meanwhile: the Inspiração would go to a character that may not
// hold one, so the undo is refused as an accept is (CANNOT_RECEIVE) and nothing changes.
func TestUndoOfAPassRefusesAGiverInTheReserve(t *testing.T) {
	t.Parallel()
	f := newFavorFight(t)
	f.give(t, f.toren)
	pass := f.mustAsk(t, f.toren.GetId(), f.pens.GetId())
	if _, err := f.answer(pass.GetId(), true); err != nil {
		t.Fatalf("AnswerFavorPass(Aceitar) error = %v", err)
	}
	line := f.undoableLine(t).GetId()
	f.reserve(t, f.toren.GetId())
	before := f.world(t)
	err := f.undo(t, f.master, f.e, line)
	wantFavorBlocked(t, "the undo of a pass to a giver in the reserve", err, blockedCannotReceive)
	f.wantSame(t, "the refused undo", before)
	if !f.has(t, f.pens.GetId()) || f.has(t, f.toren.GetId()) {
		t.Errorf("Pensantus has it %v, Toren has it %v, want the receiver to keep it and the giver in the reserve to get none", f.has(t, f.pens.GetId()), f.has(t, f.toren.GetId()))
	}
}

// "Devolver" to a roller that went back to the reserve is refused (CANNOT_RECEIVE) and the prompt stays open: the master may still keep it
// spent. Nothing is given back.
func TestRefundToARollerInTheReserveIsRefusedAndStaysOpen(t *testing.T) {
	t.Parallel()
	f, p := refundFight(t)
	f.reserve(t, f.toren.GetId())
	_, err := f.answerRefund(f.master, p.GetId(), true, newKey())
	wantFavorBlocked(t, "Devolver to a roller in the reserve", err, blockedCannotReceive)
	if status := f.promptStatus(t, p.GetId()); status != "open" {
		t.Errorf("the prompt is %q, want open", status)
	}
	if f.rows(t) != 0 {
		t.Errorf("character_favor has %d rows, want 0", f.rows(t))
	}
	if _, err := f.answerRefund(f.master, p.GetId(), false, newKey()); err != nil {
		t.Errorf("Manter gasta after the refusal error = %v, want it to work", err)
	}
}

// A give-back the app makes by itself restores the Inspiração to the character that spent it, also when that character went back to the
// reserve since: it undoes a spend, and nobody is there to be told no (RN-43). The undo of the roll gives it back and keeps the prompt.
func TestUndoOfARollGivesBackToACharacterInTheReserve(t *testing.T) {
	t.Parallel()
	f, p := refundFight(t)
	line := f.undoableLine(t).GetId()
	f.reserve(t, f.toren.GetId())
	if err := f.undo(t, f.master, f.e, line); err != nil {
		t.Fatalf("UndoLastAction(the roll) error = %v", err)
	}
	if !f.has(t, f.toren.GetId()) || f.rows(t) != 1 {
		t.Errorf("Toren has it %v and character_favor has %d rows, want the one row back with the character in the reserve", f.has(t, f.toren.GetId()), f.rows(t))
	}
	if status := f.promptStatus(t, p.GetId()); status != "kept" {
		t.Errorf("the prompt is %q, want kept", status)
	}
}

// The same for a held roll that is dropped: the combat ends with the question unanswered and its roller in the reserve, and the
// Inspiração the roll spent goes back to the character.
func TestADroppedHoldGivesBackToACharacterInTheReserve(t *testing.T) {
	t.Parallel()
	f, holdID, p := heldRefundFight(t)
	f.reserve(t, f.toren.GetId())
	if _, err := f.master.combat.EndEncounter(t.Context(), connect.NewRequest(&playv1.EndEncounterRequest{CampaignId: f.campaignID, EncounterId: f.e.GetId(), IdempotencyKey: newKey()})); err != nil {
		t.Fatalf("EndEncounter() error = %v", err)
	}
	if f.holdExists(t, holdID) {
		t.Fatalf("the hold %s is still there after the combat ended: the test does not reach the drop", holdID)
	}
	if !f.has(t, f.toren.GetId()) || f.rows(t) != 1 {
		t.Errorf("Toren has it %v and character_favor has %d rows, want the one row back with the character in the reserve", f.has(t, f.toren.GetId()), f.rows(t))
	}
	if status := f.promptStatus(t, p.GetId()); status != "kept" {
		t.Errorf("the prompt is %q, want kept", status)
	}
}
