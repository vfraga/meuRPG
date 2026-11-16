package play

import (
	"sync"
	"testing"

	"connectrpc.com/connect"

	playv1 "github.com/PuraFome/meuRPG/backend/gen/meurpg/play/v1"
)

// The refund prompt (favor_refund.go): the spend that added something over what the roller reads but nothing over every source. Toren
// attacks with a hidden advantage on him.

// refundFight is a fight where Toren's Inspiração was spent on an attack that a hidden advantage made no better: the prompt is open.
func refundFight(t *testing.T) (*favorFight, *playv1.FavorRefundPrompt) {
	t.Helper()
	f := newFavorFight(t)
	f.give(t, f.toren)
	f.hiddenEffect(t, "Toren", false, "["+attackAdv+"]")
	f.h.roller.queue(4, 17)
	if _, err := favorPaths[0].send(t, f, f.caio, newKey(), true, nil); err != nil {
		t.Fatalf("the attack with the Inspiração error = %v", err)
	}
	prompts := f.prompts(t, f.master)
	if len(prompts) != 1 {
		t.Fatalf("the master reads %d refund prompts, want 1", len(prompts))
	}
	return f, prompts[0]
}

func (f *favorFight) prompts(t *testing.T, u *user) []*playv1.FavorRefundPrompt {
	t.Helper()
	res, err := u.favor.GetFavor(t.Context(), connect.NewRequest(&playv1.GetFavorRequest{CampaignId: f.campaignID}))
	if err != nil {
		t.Fatalf("GetFavor() error = %v", err)
	}
	return res.Msg.GetRefundPrompts()
}

func (f *favorFight) answerRefund(u *user, id string, giveBack bool, key string) (*playv1.FavorRefundPrompt, error) {
	res, err := u.favor.AnswerFavorRefund(f.h.t.Context(), connect.NewRequest(&playv1.AnswerFavorRefundRequest{CampaignId: f.campaignID, PromptId: id, GiveBack: giveBack, IdempotencyKey: key}))
	if err != nil {
		return nil, err
	}
	return res.Msg.GetPrompt(), nil
}

// The prompt is the master's alone, names the roll and the hidden source, and the roll resolved at once with the Inspiração spent.
func TestRefundPromptOpensForTheMasterAlone(t *testing.T) {
	t.Parallel()
	f, p := refundFight(t)
	if p.GetCharacterId() != f.toren.GetId() || p.GetCharacterName() != "Toren" || p.GetRollLabelPt() != "Ataque" || len(p.GetHiddenSources()) != 1 ||
		p.GetStatus() != playv1.FavorRefundStatus_FAVOR_REFUND_STATUS_OPEN {
		t.Errorf("prompt = %v, want Toren's open attack with one hidden source", p)
	}
	if f.favorHolds(t) || f.eventsOf(t, eventFavorRefundPrompted) != 1 {
		t.Errorf("the Inspiração is there %v, prompted events %d, want it spent and one event", f.favorHolds(t), f.eventsOf(t, eventFavorRefundPrompted))
	}
	for name, u := range map[string]*user{"the roller's player": f.caio, "another player": f.ana, "a third player": f.bia} {
		if got := f.prompts(t, u); len(got) != 0 {
			t.Errorf("%s reads %v, want no refund prompt", name, got)
		}
	}
	if _, err := f.answerRefund(f.caio, p.GetId(), true, newKey()); connect.CodeOf(err) != connect.CodePermissionDenied {
		t.Errorf("a player's answer = %v, want permission_denied", err)
	}
}

// A spend that adds visibly and over every source opens no prompt.
func TestRefundPromptNeverOpensForASpendThatHelped(t *testing.T) {
	t.Parallel()
	f := newFavorFight(t)
	f.give(t, f.toren)
	f.h.roller.queue(4, 17)
	if _, err := favorPaths[0].send(t, f, f.caio, newKey(), true, nil); err != nil {
		t.Fatal(err)
	}
	if got := f.prompts(t, f.master); len(got) != 0 {
		t.Errorf("the master reads %v, want no prompt", got)
	}
}

// Giving back returns the Inspiração through the one refund path, closes the prompt RETURNED, tells the master and the owner and writes
// the combat's line: the player reads only that the master gave it back.
func TestAnswerFavorRefundGiveBack(t *testing.T) {
	t.Parallel()
	f, p := refundFight(t)
	before, key := f.seq(t), newKey()
	got, err := f.answerRefund(f.master, p.GetId(), true, key)
	if err != nil || got.GetStatus() != playv1.FavorRefundStatus_FAVOR_REFUND_STATUS_RETURNED {
		t.Fatalf("AnswerFavorRefund(give back) = %v, %v, want RETURNED", got, err)
	}
	if !f.favorHolds(t) || f.seq(t) != before+1 || f.eventsOf(t, eventFavorRefunded) != 1 {
		t.Errorf("row %v, number %d (want %d), refunded events %d, want it back in one step", f.favorHolds(t), f.seq(t), before+1, f.eventsOf(t, eventFavorRefunded))
	}
	f.wantTexts(t, "the refund", map[*user][]string{f.master: {"O mestre devolveu a Inspiração de Toren."}, f.caio: {"O mestre devolveu a sua Inspiração."}})
	if again, err := f.answerRefund(f.master, p.GetId(), true, key); err != nil || again.GetStatus() != got.GetStatus() || f.seq(t) != before+1 || f.rows(t) != 1 {
		t.Errorf("the retry = %v, %v (number %d), want the same answer and no new step", again, err, f.seq(t))
	}
	if _, err := f.answerRefund(f.master, p.GetId(), false, newKey()); !isBlockedBy(err, blockedPassClosed) {
		t.Errorf("a new answer to a closed prompt = %v, want PASS_CLOSED", err)
	}
	if got := f.prompts(t, f.master); len(got) != 0 {
		t.Errorf("the master still reads the answered prompt: %v", got)
	}
}

// A roller that holds an Inspiração again by then (the master granted another) is refused with ALREADY_HAS, not given a silent RETURNED: the
// answer rolls back, the prompt stays open and the character keeps the one row.
func TestAnswerFavorRefundNeverStacks(t *testing.T) {
	t.Parallel()
	f, p := refundFight(t)
	f.give(t, f.toren)
	seq := f.seq(t)
	_, err := f.answerRefund(f.master, p.GetId(), true, newKey())
	wantFavorBlocked(t, "a refund to a roller that has one", err, playv1.FavorBlockedReason_FAVOR_BLOCKED_REASON_ALREADY_HAS)
	if f.rows(t) != 1 || f.seq(t) != seq || f.eventsOf(t, eventFavorRefunded) != 0 {
		t.Errorf("rows %d, number %d (want %d), refunded events %d, want the refusal to change nothing", f.rows(t), f.seq(t), seq, f.eventsOf(t, eventFavorRefunded))
	}
	if got := f.prompts(t, f.master); len(got) != 1 || got[0].GetStatus() != playv1.FavorRefundStatus_FAVOR_REFUND_STATUS_OPEN {
		t.Errorf("the master reads %v, want the prompt still open", got)
	}
}

// The master undoes the roll that spent the Inspiração: it is back with the roller (the undo's give-back), so the prompt closes as kept, and
// an answer to it is PASS_CLOSED: the same spend is never refunded twice.
func TestUndoOfTheRollKeepsItsRefundPrompt(t *testing.T) {
	t.Parallel()
	f, p := refundFight(t)
	if err := f.undo(t, f.master, f.e, f.undoableLine(t).GetId()); err != nil {
		t.Fatalf("UndoLastAction(the roll) error = %v", err)
	}
	if !f.favorHolds(t) {
		t.Fatal("the undo of the roll did not give the Inspiração back")
	}
	if got := f.prompts(t, f.master); len(got) != 0 {
		t.Errorf("the master still reads %v after the roll was undone, want no prompt", got)
	}
	if status := f.promptStatus(t, p.GetId()); status != "kept" {
		t.Errorf("the prompt of the undone roll is %q, want kept", status)
	}
	_, err := f.answerRefund(f.master, p.GetId(), true, newKey())
	wantFavorBlocked(t, "an answer to the prompt of an undone roll", err, blockedPassClosed)
	if f.rows(t) != 1 {
		t.Errorf("character_favor has %d rows, want 1", f.rows(t))
	}
}

// A held roll that spent the Inspiração is forgotten before it is answered (the round turned): the Inspiração goes back with it and the
// prompt of that spend closes as kept.
func TestForgottenHeldRollKeepsItsRefundPrompt(t *testing.T) {
	t.Parallel()
	f := newFavorFight(t)
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
	if _, err := f.endTurn(t, f.master, f.e, true); err != nil {
		t.Fatalf("EndTurn() error = %v", err)
	}
	f.e = f.passTo(t, f.e, "Toren") // the round turned: the hold is stale
	f.h.roller.queue(7)
	if _, err := favorPaths[0].send(t, f, f.caio, newKey(), false, nil); err != nil {
		t.Fatalf("the next attack error = %v", err)
	}
	if f.holdExists(t, held.holdID) || !f.favorHolds(t) {
		t.Fatalf("the stale hold is there = %v, the Inspiração is back = %v, want it forgotten and given back", f.holdExists(t, held.holdID), f.favorHolds(t))
	}
	if status := f.promptStatus(t, prompts[0].GetId()); status != "kept" {
		t.Errorf("the prompt of the forgotten roll is %q, want kept", status)
	}
	if _, err := f.answerRefund(f.master, prompts[0].GetId(), true, newKey()); !isBlockedBy(err, blockedPassClosed) {
		t.Errorf("an answer to the prompt of a forgotten roll = %v, want PASS_CLOSED", err)
	}
	if f.rows(t) != 1 {
		t.Errorf("character_favor has %d rows, want 1", f.rows(t))
	}
}

func (f *favorFight) promptStatus(t *testing.T, id string) string {
	t.Helper()
	var status string
	if err := f.h.pool.QueryRow(t.Context(), `SELECT status FROM favor_refund_prompts WHERE id = $1`, id).Scan(&status); err != nil {
		t.Fatalf("read the prompt: %v", err)
	}
	return status
}

// vitalsHints counts, for each reader, the vitals hints of a character since the marker.
func vitalsHints(heard map[string][]*playv1.WatchGameSessionResponse, characterID string) map[string]int {
	out := map[string]int{}
	for name, got := range heard {
		for _, ev := range got {
			if ev.GetVitalsChanged().GetVitals().GetCharacterId() == characterID {
				out[name]++
			}
		}
	}
	return out
}

// The prompt opens for the master alone: Toren's vitals hint of the prompt reaches him, and Ana and Bia hear nothing of Toren (his own
// player is told by the roll itself).
func TestRefundPromptHintsReachTheMasterAlone(t *testing.T) {
	t.Parallel()
	f := newFavorFight(t)
	f.give(t, f.toren)
	f.hiddenEffect(t, "Toren", false, "["+attackAdv+"]")
	f.h.roller.queue(4, 17)
	ws := f.passWatchers(t)
	if _, err := favorPaths[0].send(t, f, f.caio, newKey(), true, nil); err != nil {
		t.Fatal(err)
	}
	got := vitalsHints(f.heardBy(t, ws), f.toren.GetId())
	if got["master"] == 0 || got["ana"] != 0 || got["bia"] != 0 {
		t.Errorf("vitals hints of Toren = %v, want the master's and none for Ana and Bia", got)
	}
}

// A refund tells the master and the roller's player (the vitals hint and the combat's line); Ana and Bia hear nothing, and Ana's number
// of hints of the log does not move.
func TestRefundHintsReachTheMasterAndTheRollerAlone(t *testing.T) {
	t.Parallel()
	f, p := refundFight(t)
	ws := f.passWatchers(t)
	before := f.seq(t)
	if _, err := f.answerRefund(f.master, p.GetId(), true, newKey()); err != nil {
		t.Fatal(err)
	}
	if now := f.seq(t); now != before+1 { // read before heardBy: its marker is a write of its own
		t.Errorf("hint_seq = %d, want %d (one step)", now, before+1)
	}
	heard := f.heardBy(t, ws)
	if v := vitalsHints(heard, f.toren.GetId()); v["master"] != 1 || v["caio"] != 1 || v["ana"] != 0 || v["bia"] != 0 {
		t.Errorf("vitals hints of Toren = %v, want the master 1 and Caio 1", v)
	}
	if l := logHints(heard); l["master"] != 1 || l["caio"] != 1 || l["ana"] != 0 || l["bia"] != 0 {
		t.Errorf("log hints = %v, want the master 1 and Caio 1", l)
	}
}

// Keeping it moves the number by one and tells the master's other tabs (his vitals hint of the roller) so that they close the prompt; the
// roller's player and the others hear nothing. The retry tells it again and moves no number.
func TestAnswerFavorRefundKeepMovesTheNumberAndTellsTheMaster(t *testing.T) {
	t.Parallel()
	f, p := refundFight(t)
	ws := f.passWatchers(t)
	before, key := f.seq(t), newKey()
	if _, err := f.answerRefund(f.master, p.GetId(), false, key); err != nil {
		t.Fatal(err)
	}
	if now := f.seq(t); now != before+1 { // read before heardBy: its marker is a write of its own
		t.Errorf("hint_seq = %d, want %d (one step)", now, before+1)
	}
	if v := vitalsHints(f.heardBy(t, ws), f.toren.GetId()); v["master"] != 1 || v["caio"] != 0 || v["ana"] != 0 || v["bia"] != 0 {
		t.Errorf("vitals hints of Toren = %v, want the master's alone", v)
	}
	ws, marked := f.passWatchers(t), f.seq(t)
	if _, err := f.answerRefund(f.master, p.GetId(), false, key); err != nil {
		t.Fatalf("the retry of the keep error = %v", err)
	}
	if now := f.seq(t); now != marked {
		t.Errorf("hint_seq after the retry = %d, want %d (no new step)", now, marked)
	}
	if v := vitalsHints(f.heardBy(t, ws), f.toren.GetId()); v["master"] != 1 || v["caio"] != 0 || v["ana"] != 0 || v["bia"] != 0 {
		t.Errorf("vitals hints of the retry = %v, want the master's again", v)
	}
}

// The retry of a refund tells the roller's vitals hint again, as the first call did, and moves no number.
func TestAnswerFavorRefundRetryTellsAgain(t *testing.T) {
	t.Parallel()
	f, p := refundFight(t)
	key := newKey()
	if _, err := f.answerRefund(f.master, p.GetId(), true, key); err != nil {
		t.Fatal(err)
	}
	ws, before := f.passWatchers(t), f.seq(t)
	if _, err := f.answerRefund(f.master, p.GetId(), true, key); err != nil {
		t.Fatalf("the retry of the refund error = %v", err)
	}
	if now := f.seq(t); now != before {
		t.Errorf("hint_seq after the retry = %d, want %d", now, before)
	}
	if v := vitalsHints(f.heardBy(t, ws), f.toren.GetId()); v["master"] != 1 || v["caio"] != 1 || v["ana"] != 0 || v["bia"] != 0 {
		t.Errorf("vitals hints of the retry = %v, want the master's and Caio's again", v)
	}
}

// Keeping it leaves it spent, tells the master alone and writes nothing the player reads.
func TestAnswerFavorRefundKeep(t *testing.T) {
	t.Parallel()
	f, p := refundFight(t)
	got, err := f.answerRefund(f.master, p.GetId(), false, newKey())
	if err != nil || got.GetStatus() != playv1.FavorRefundStatus_FAVOR_REFUND_STATUS_KEPT {
		t.Fatalf("AnswerFavorRefund(keep) = %v, %v, want KEPT", got, err)
	}
	if f.favorHolds(t) || f.eventsOf(t, eventFavorRefundKept) != 1 {
		t.Errorf("row %v, kept events %d, want it spent and one event", f.favorHolds(t), f.eventsOf(t, eventFavorRefundKept))
	}
	f.wantTexts(t, "the kept prompt", nil)
}

// An unanswered prompt keeps it spent and closes KEPT with the session.
func TestTheEndOfASessionKeepsOpenRefundPrompts(t *testing.T) {
	t.Parallel()
	f, p := refundFight(t)
	if _, err := f.master.play.EndGameSession(t.Context(), connect.NewRequest(&playv1.EndGameSessionRequest{CampaignId: f.campaignID, GameSessionId: f.sessionID})); err != nil {
		t.Fatalf("EndGameSession() error = %v", err)
	}
	var status string
	if err := f.h.pool.QueryRow(t.Context(), `SELECT status FROM favor_refund_prompts WHERE id = $1`, p.GetId()).Scan(&status); err != nil || status != "kept" {
		t.Errorf("status = %q (%v), want kept", status, err)
	}
	if f.eventsOf(t, eventFavorRefundKept) != 1 || f.favorHolds(t) {
		t.Errorf("kept events %d, row %v, want one event and the Inspiração still spent", f.eventsOf(t, eventFavorRefundKept), f.favorHolds(t))
	}
}

// Two answers at once: one wins, the other reads PASS_CLOSED, and the Inspiração is back at most once.
func TestAnswerFavorRefundRace(t *testing.T) {
	t.Parallel()
	f, p := refundFight(t)
	var wg sync.WaitGroup
	errs := make([]error, 2)
	for i := range errs {
		wg.Add(1)
		go func() {
			defer wg.Done()
			_, errs[i] = f.answerRefund(f.master, p.GetId(), i == 0, newKey())
		}()
	}
	wg.Wait()
	wins := 0
	for _, err := range errs {
		if err == nil {
			wins++
		} else if !isBlockedBy(err, blockedPassClosed) {
			t.Errorf("the losing answer = %v, want PASS_CLOSED", err)
		}
	}
	if wins != 1 || f.rows(t) > 1 {
		t.Errorf("%d answers won, %d rows, want one winner and at most one row", wins, f.rows(t))
	}
}

// A refund answered with the roller hidden is the master's line alone, even after the flag is cleared (RN-10). A player's combatant cannot
// be hidden by any call (TestAPlayersCombatantCannotBeHidden), so the flag is set in the table to reach the defensive stamp.
func TestRefundLineOfAHiddenRollerIsTheMastersAlone(t *testing.T) {
	t.Parallel()
	f, p := refundFight(t)
	f.hideBySQL(t, f.toren.GetId(), true)
	if _, err := f.answerRefund(f.master, p.GetId(), true, newKey()); err != nil {
		t.Fatal(err)
	}
	want := map[*user][]string{f.master: {"O mestre devolveu a Inspiração de Toren."}}
	f.wantTexts(t, "the refund with Toren hidden", want)
	f.hideBySQL(t, f.toren.GetId(), false)
	f.wantTexts(t, "the refund after the flag is cleared", want)
}

// The undo of a zone's cast gives back the Inspiração spent on the save of a catch at the cast, as it does the roll's own undo: the prompt
// of that spend closes as kept too, or the master's later "Devolver" would mint a second Inspiração for a roller that spent the first again.
func TestUndoOfTheCastKeepsTheRefundPromptOfAZoneSave(t *testing.T) {
	t.Parallel()
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
	if err := f.undo(t, f.master, e, f.log(t, f.master, e).GetUndoableEventId()); err != nil {
		t.Fatalf("UndoLastAction(the cast) error = %v", err)
	}
	if !f.favorHolds(t) {
		t.Fatal("the undo of the cast did not give the Inspiração back")
	}
	f.execSQL(t, `DELETE FROM character_favor WHERE campaign_id = $1`, f.campaignID) // the roller spends it again
	if status := f.promptStatus(t, prompts[0].GetId()); status != "kept" {
		t.Errorf("the prompt of the undone cast's save is %q, want kept", status)
	}
	_, err := f.answerRefund(f.master, prompts[0].GetId(), true, newKey())
	wantFavorBlocked(t, "an answer to the prompt of an undone cast's save", err, blockedPassClosed)
	if f.rows(t) != 0 {
		t.Errorf("character_favor has %d rows, want 0: no second Inspiração", f.rows(t))
	}
}
