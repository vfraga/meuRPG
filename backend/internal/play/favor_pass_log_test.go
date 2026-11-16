package play

import (
	"slices"
	"testing"

	"connectrpc.com/connect"

	playv1 "github.com/PuraFome/meuRPG/backend/gen/meurpg/play/v1"
)

// The pass and the grant in a combat (favor_pass.go, combat_log_favor.go, combat_undo_favor.go): the log's lines, who reads each, the
// master's undo of the pass and of a grant. Toren is Caio's, Pensantus is Ana's and Brisa is Bia's, all in the casters' combat.

// favorTexts are the sentences about the Inspiration u reads in the combat's log, oldest first.
func (f *favorFight) favorTexts(t *testing.T, u *user) []string {
	t.Helper()
	var out []string
	for _, l := range slices.Backward(logEntries(f.log(t, u, f.e))) {
		if l.GetFavor() != nil {
			out = append(out, l.GetFavor().GetTextPt())
		}
	}
	return out
}

// setupGrantLine is the master's line of the grant that gives Toren the Inspiration these fights start from (f.give): wantTexts leaves it out,
// as it is not what the tests judge; the tests of the grant itself read the lines whole (wantAllTexts).
const setupGrantLine = "O mestre deu a Inspiração a Toren."

// wantTexts checks what each reader reads, apart from the setup grant's line: nothing for a name missing from want.
func (f *favorFight) wantTexts(t *testing.T, what string, want map[*user][]string) {
	t.Helper()
	for _, u := range []*user{f.master, f.caio, f.ana, f.bia} {
		got := slices.DeleteFunc(f.favorTexts(t, u), func(text string) bool { return text == setupGrantLine })
		if !slices.Equal(got, want[u]) {
			t.Errorf("%s: a reader reads %q, want %q", what, got, want[u])
		}
	}
}

// wantAllTexts is wantTexts with every line, the setup grant's included.
func (f *favorFight) wantAllTexts(t *testing.T, what string, want map[*user][]string) {
	t.Helper()
	for _, u := range []*user{f.master, f.caio, f.ana, f.bia} {
		got := f.favorTexts(t, u)
		if !slices.Equal(got, want[u]) {
			t.Errorf("%s: a reader reads %q, want %q", what, got, want[u])
		}
	}
}

// eventRow reads the encounter column of the newest event of the kind.
func (f *favorFight) eventEncounter(t *testing.T, kind string) (encounter *string, round int) {
	t.Helper()
	err := f.h.pool.QueryRow(t.Context(), `SELECT encounter_id::text, COALESCE((payload->>'round')::int, 0) FROM session_events WHERE kind = $1 AND game_session_id = $2 ORDER BY seq DESC LIMIT 1`, kind, f.sessionID).Scan(&encounter, &round)
	if err != nil {
		t.Fatalf("read the newest %s event: %v", kind, err)
	}
	return encounter, round
}

// undoableLine is the entry of the combat's log the master undoes as its last action: the undo reads its id from the line the master reads.
func (f *favorFight) undoableLine(t *testing.T) *playv1.CombatLogEntry {
	t.Helper()
	for _, l := range logEntries(f.log(t, f.master, f.e)) {
		if l.GetUndoable() {
			return l
		}
	}
	t.Fatal("no line of the master's log is the combat's last action")
	return nil
}

// logHints counts, for each reader, the hints that the combat's log changed since the marker.
func logHints(heard map[string][]*playv1.WatchGameSessionResponse) map[string]int {
	out := map[string]int{}
	for name, got := range heard {
		for _, ev := range got {
			if ev.GetCombatLogChanged() != nil {
				out[name]++
			}
		}
	}
	return out
}

// Each answer to a pass in a combat is a line of its log, in the words of each reader; a player the pass is not about reads nothing.
func TestPassAnsweredInACombatWritesALinePerReader(t *testing.T) {
	t.Parallel()
	for _, tc := range []struct {
		name string
		do   func(t *testing.T, f *favorFight, id string)
		kind string
		want func(f *favorFight) map[*user][]string
	}{
		{"accepted", func(t *testing.T, f *favorFight, id string) {
			t.Helper()
			if _, err := f.answer(id, true); err != nil {
				t.Fatalf("accept: %v", err)
			}
		}, eventFavorPassed, func(f *favorFight) map[*user][]string {
			return map[*user][]string{
				f.master: {"Toren passou a Inspiração para Pensantus."},
				f.caio:   {"O mestre aceitou: a sua Inspiração foi para Pensantus."},
				f.ana:    {"Toren passou a Inspiração para você."},
			}
		}},
		{"refused", func(t *testing.T, f *favorFight, id string) {
			t.Helper()
			if _, err := f.answer(id, false); err != nil {
				t.Fatalf("refuse: %v", err)
			}
		}, eventFavorAskRefused, func(f *favorFight) map[*user][]string {
			return map[*user][]string{
				f.master: {"O mestre recusou o pedido de Toren: a Inspiração continua com Toren."},
				f.caio:   {"O mestre recusou: a Inspiração continua com você."},
			}
		}},
		{"canceled", func(t *testing.T, f *favorFight, id string) {
			t.Helper()
			if _, err := f.cancelAs(f.caio, id, newKey()); err != nil {
				t.Fatalf("cancel: %v", err)
			}
		}, eventFavorPassCanceled, func(f *favorFight) map[*user][]string {
			return map[*user][]string{
				f.master: {"Toren desistiu de passar a Inspiração para Pensantus."},
				f.caio:   {"Você desistiu de passar a Inspiração."},
			}
		}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			f := newFavorFight(t)
			f.give(t, f.toren)
			p := f.mustAsk(t, f.toren.GetId(), f.pens.GetId())
			f.wantTexts(t, "the ask", nil) // an ask is no line
			tc.do(t, f, p.GetId())
			f.wantTexts(t, tc.name, tc.want(f))
			if enc, round := f.eventEncounter(t, tc.kind); enc == nil || *enc != f.e.GetId() || round < 1 {
				t.Errorf("the %s event carries encounter %v and round %d, want the open combat and its round", tc.kind, enc, round)
			}
		})
	}
}

// Outside a combat the answers write no line and carry no combat: the answers show in GetFavor and the live hint.
func TestPassOutsideACombatWritesNoLineAndNoCombat(t *testing.T) {
	t.Parallel()
	f := newFavorParty(t)
	if err := f.grant(f.toren.GetId()); err != nil {
		t.Fatal(err)
	}
	p := f.mustAsk(t, f.toren.GetId(), f.pens.GetId())
	if _, err := f.answer(p.GetId(), true); err != nil {
		t.Fatal(err)
	}
	for _, kind := range []string{eventFavorGranted, eventFavorPassed} {
		var n int
		if err := f.h.pool.QueryRow(t.Context(), `SELECT count(*) FROM session_events WHERE kind = $1 AND game_session_id = $2 AND encounter_id IS NOT NULL`, kind, f.sessionID).Scan(&n); err != nil || n != 0 {
			t.Errorf("%s events with a combat outside one = %d (%v), want 0", kind, n, err)
		}
	}
}

// hideBySQL sets the hidden flag of a player's combatant straight in the table. The calls refuse to (a player's combatant is never hidden,
// see TestAPlayersCombatantCannotBeHidden), so this is the only way to reach the defensive stamp that makes a favor line the master's alone.
func (f *favorFight) hideBySQL(t *testing.T, characterID string, hidden bool) {
	t.Helper()
	f.execSQL(t, `UPDATE combatants SET hidden = $1 WHERE encounter_id = $2 AND character_id = $3`, hidden, f.e.GetId(), characterID)
}

// No call can hide a player's combatant, so a giver or receiver of the Inspiration is never hidden: the precondition of the stamp below.
func TestAPlayersCombatantCannotBeHidden(t *testing.T) {
	t.Parallel()
	f := newFavorFight(t)
	for _, name := range []string{"Toren", "Pensantus"} {
		_, err := f.master.combat.SetCombatantHidden(t.Context(), connect.NewRequest(&playv1.SetCombatantHiddenRequest{
			CampaignId: f.campaignID, EncounterId: f.e.GetId(), CombatantId: f.id(t, name), IdempotencyKey: newKey(), Hidden: true,
		}))
		if connect.CodeOf(err) != connect.CodeInvalidArgument {
			t.Errorf("hiding %s = %v, want invalid_argument", name, err)
		}
	}
}

// Grants and passes involve player characters only, and a player's combatant cannot be hidden by any call, so no call reaches a hidden giver
// or receiver, and no combatant the player does not see is in a pass (the audience is the owners of the characters named). The stamp that
// a hidden combatant makes a line the master's alone (favorInCombat.Secret) is defensive: this test reaches it by setting the flag in the
// table, and proves the line stays the master's alone, whoever the hidden one is and after the flag is cleared (RN-10).
func TestPassLineOfAHiddenCombatantIsTheMastersAlone(t *testing.T) {
	t.Parallel()
	for _, hidden := range []string{"Toren", "Pensantus"} {
		t.Run(hidden+" hidden", func(t *testing.T) {
			t.Parallel()
			f := newFavorFight(t)
			f.give(t, f.toren)
			p := f.mustAsk(t, f.toren.GetId(), f.pens.GetId())
			id := f.toren.GetId()
			if hidden == "Pensantus" {
				id = f.pens.GetId()
			}
			f.hideBySQL(t, id, true)
			if _, err := f.answer(p.GetId(), true); err != nil {
				t.Fatalf("accept: %v", err)
			}
			want := map[*user][]string{f.master: {"Toren passou a Inspiração para Pensantus."}}
			f.wantTexts(t, "the line with "+hidden+" hidden", want)
			f.hideBySQL(t, id, false)
			f.wantTexts(t, "the line after "+hidden+" is shown", want)
		})
	}
}

// The master's undo of an accepted pass is one step: the receiver loses it and the giver has it again, the line goes, and the receiver
// reads the one sentence that names nobody. A third player hears nothing of it.
func TestUndoOfAnAcceptedPass(t *testing.T) {
	t.Parallel()
	f := newFavorFight(t)
	f.give(t, f.toren)
	p := f.mustAsk(t, f.toren.GetId(), f.pens.GetId())
	if _, err := f.answer(p.GetId(), true); err != nil {
		t.Fatal(err)
	}
	id := f.undoableLine(t).GetId()
	ws := f.passWatchers(t)
	before := f.seq(t)
	if err := f.undo(t, f.master, f.e, id); err != nil {
		t.Fatalf("UndoLastAction() error = %v", err)
	}
	if !f.has(t, f.toren.GetId()) || f.has(t, f.pens.GetId()) {
		t.Errorf("rows: Toren %v, Pensantus %v, want it back with Toren and gone from Pensantus", f.has(t, f.toren.GetId()), f.has(t, f.pens.GetId()))
	}
	if now := f.seq(t); now != before+1 {
		t.Errorf("hint_seq = %d, want %d (one step)", now, before+1)
	}
	f.wantTexts(t, "the undone pass", map[*user][]string{f.ana: {"O mestre desfez a passagem: a Inspiração não está mais com você."}})
	if l := f.lineOf(t, f.ana, playv1.CombatLogKind_COMBAT_LOG_KIND_FAVOR_PASS_ANSWERED); l.GetActorId() != "" || l.GetActorLabel() != "" {
		t.Errorf("the receiver's line names %q %q, want nobody", l.GetActorId(), l.GetActorLabel())
	}
	for name, got := range f.heardBy(t, ws) {
		if name == "bia" && len(got) != 0 {
			t.Errorf("a third player heard %v of the undo, want nothing", got)
		}
	}
}

// The undo is refused, and changes nothing, when the receiver spent it (UNDO_TARGET_CHANGED, the reason the other undos use when the
// target changed) or when the giver has another by then (ALREADY_HAS).
func TestUndoOfAnAcceptedPassRefusals(t *testing.T) {
	t.Parallel()
	for _, tc := range []struct {
		name   string
		change string
		check  func(t *testing.T, err error)
	}{
		{"the receiver spent it", `DELETE FROM character_favor WHERE character_id = $1`, func(t *testing.T, err error) {
			t.Helper()
			if got := blockedOf(t, err).GetReason(); got != playv1.EncounterBlockedReason_ENCOUNTER_BLOCKED_REASON_UNDO_TARGET_CHANGED {
				t.Errorf("reason = %v, want UNDO_TARGET_CHANGED", got)
			}
		}},
		{"the giver has another", `INSERT INTO character_favor (character_id, campaign_id) SELECT $1, campaign_id FROM characters WHERE id = $1`, func(t *testing.T, err error) {
			t.Helper()
			wantFavorBlocked(t, "the undo", err, playv1.FavorBlockedReason_FAVOR_BLOCKED_REASON_ALREADY_HAS)
		}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			f := newFavorFight(t)
			f.give(t, f.toren)
			p := f.mustAsk(t, f.toren.GetId(), f.pens.GetId())
			if _, err := f.answer(p.GetId(), true); err != nil {
				t.Fatal(err)
			}
			id := f.log(t, f.master, f.e).GetUndoableEventId()
			target := f.pens.GetId()
			if tc.name == "the giver has another" {
				target = f.toren.GetId()
			}
			f.execSQL(t, tc.change, target)
			rows, seq := f.rows(t), f.seq(t)
			tc.check(t, f.undo(t, f.master, f.e, id))
			if f.rows(t) != rows || f.seq(t) != seq || f.eventsOf(t, eventActionUndone) != 0 {
				t.Errorf("the refused undo changed rows %d->%d, number %d->%d, undo events %d", rows, f.rows(t), seq, f.seq(t), f.eventsOf(t, eventActionUndone))
			}
		})
	}
}

// The undo of a refused or canceled pass only closes the line: nothing moves.
func TestUndoOfARefusedPassOnlyClosesTheLine(t *testing.T) {
	t.Parallel()
	f := newFavorFight(t)
	f.give(t, f.toren)
	p := f.mustAsk(t, f.toren.GetId(), f.pens.GetId())
	if _, err := f.answer(p.GetId(), false); err != nil {
		t.Fatal(err)
	}
	id := f.undoableLine(t).GetId()
	if err := f.undo(t, f.master, f.e, id); err != nil {
		t.Fatalf("UndoLastAction() error = %v", err)
	}
	f.wantTexts(t, "the undone refusal", nil)
	if !f.has(t, f.toren.GetId()) || f.has(t, f.pens.GetId()) || f.passStatus(t, p.GetId()) != "refused" {
		t.Error("the undo of a refusal moved the Inspiration or the pass")
	}
}

// The master's undo of a grant takes it back, in the combat the grant was made in, and reads it from the grant's own line (the master's
// alone); a grant already spent is refused.
func TestUndoOfAGrant(t *testing.T) {
	t.Parallel()
	f := newFavorFight(t)
	f.give(t, f.toren)
	line := f.undoableLine(t)
	if got := line.GetFavor().GetTextPt(); got != "O mestre deu a Inspiração a Toren." {
		t.Fatalf("the grant's line reads %q, want the master's sentence", got)
	}
	if enc, _ := f.eventEncounter(t, eventFavorGranted); enc == nil {
		t.Fatal("the grant carries no combat")
	}
	f.execSQL(t, `DELETE FROM character_favor WHERE character_id = $1`, f.toren.GetId()) // spent
	if got := blockedOf(t, f.undo(t, f.master, f.e, line.GetId())).GetReason(); got != playv1.EncounterBlockedReason_ENCOUNTER_BLOCKED_REASON_UNDO_TARGET_CHANGED {
		t.Errorf("the undo of a spent grant = %v, want UNDO_TARGET_CHANGED", got)
	}
	f.give(t, f.pens)
	id := f.undoableLine(t).GetId()
	if id == line.GetId() {
		t.Fatal("the second grant is not the last action")
	}
	if err := f.undo(t, f.master, f.e, id); err != nil {
		t.Fatalf("UndoLastAction() error = %v", err)
	}
	if f.has(t, f.pens.GetId()) {
		t.Error("the undo did not take the grant back")
	}
	if f.undoableLine(t).GetId() != line.GetId() {
		t.Error("the undone grant is still the last action")
	}
}

// The grant's line, the pass's hints and the master's undo of a grant tell the master alone: the holder's player, another player and a
// third player hear one hint of the log changing only when the line is theirs, and read no sentence. A hidden holder changes nothing for
// them.
func TestGrantInACombatIsTheMastersLineAlone(t *testing.T) {
	t.Parallel()
	for _, hidden := range []bool{false, true} {
		t.Run(map[bool]string{false: "shown", true: "hidden"}[hidden], func(t *testing.T) {
			t.Parallel()
			f := newFavorFight(t)
			if hidden {
				f.hideBySQL(t, f.toren.GetId(), true)
			}
			ws := f.passWatchers(t)
			f.give(t, f.toren)
			f.wantAllTexts(t, "the grant", map[*user][]string{f.master: {setupGrantLine}})
			heard := logHints(f.heardBy(t, ws))
			if heard["master"] != 1 || heard["caio"] != 0 || heard["ana"] != 0 || heard["bia"] != 0 {
				t.Errorf("log hints of the grant = %v, want the master's alone", heard)
			}
			ws = f.passWatchers(t)
			if err := f.undo(t, f.master, f.e, f.undoableLine(t).GetId()); err != nil {
				t.Fatalf("UndoLastAction() error = %v", err)
			}
			heard = logHints(f.heardBy(t, ws))
			if heard["caio"] != 0 || heard["ana"] != 0 || heard["bia"] != 0 {
				t.Errorf("log hints of the undo of the grant = %v, want none for the players", heard)
			}
		})
	}
}

// The hints of a pass answered in a combat: the master and the giver's owner always hear that the log changed; the receiver's owner only when
// the pass was accepted (a refusal or a cancel leaves the Inspiration where it was, and is not theirs to hear of); a third player never.
func TestPassAnswerLogHintsInACombat(t *testing.T) {
	t.Parallel()
	for _, tc := range []struct {
		name   string
		answer func(t *testing.T, f *favorFight, id string)
		ana    int
	}{
		{"accepted", func(t *testing.T, f *favorFight, id string) {
			t.Helper()
			if _, err := f.answer(id, true); err != nil {
				t.Fatal(err)
			}
		}, 1},
		{"refused", func(t *testing.T, f *favorFight, id string) {
			t.Helper()
			if _, err := f.answer(id, false); err != nil {
				t.Fatal(err)
			}
		}, 0},
		{"canceled", func(t *testing.T, f *favorFight, id string) {
			t.Helper()
			if _, err := f.cancelAs(f.caio, id, newKey()); err != nil {
				t.Fatal(err)
			}
		}, 0},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			f := newFavorFight(t)
			f.give(t, f.toren)
			p := f.mustAsk(t, f.toren.GetId(), f.pens.GetId())
			ws := f.passWatchers(t)
			tc.answer(t, f, p.GetId())
			heard := logHints(f.heardBy(t, ws))
			if heard["master"] != 1 || heard["caio"] != 1 || heard["ana"] != tc.ana || heard["bia"] != 0 {
				t.Errorf("log hints = %v, want the master 1, Caio 1, Ana %d, Bia 0", heard, tc.ana)
			}
		})
	}
}

// The undo of a grant takes back the ask the holder had made to pass it on: the ask closes as canceled in the same step, with a
// favor_pass_canceled that is no line and no combat's action, and no hint of its own.
func TestUndoOfAGrantCancelsTheHoldersPendingAsk(t *testing.T) {
	t.Parallel()
	f := newFavorFight(t)
	f.give(t, f.toren)
	p := f.mustAsk(t, f.toren.GetId(), f.pens.GetId())
	ws := f.passWatchers(t)
	before := f.seq(t)
	if err := f.undo(t, f.master, f.e, f.undoableLine(t).GetId()); err != nil {
		t.Fatalf("UndoLastAction() error = %v", err)
	}
	if f.has(t, f.toren.GetId()) || f.passStatus(t, p.GetId()) != "canceled" {
		t.Errorf("Toren has it %v, ask %q, want it gone and the ask canceled", f.has(t, f.toren.GetId()), f.passStatus(t, p.GetId()))
	}
	if enc, _ := f.eventEncounter(t, eventFavorPassCanceled); enc != nil {
		t.Errorf("the cancel by the undo carries combat %v, want none", *enc)
	}
	if now := f.seq(t); now != before+1 {
		t.Errorf("hint_seq = %d, want %d (one step)", now, before+1)
	}
	if got := logHints(f.heardBy(t, ws)); got["bia"] != 0 || got["ana"] != 0 {
		t.Errorf("log hints of the undo = %v, want none for Ana and Bia", got)
	}
	f.wantTexts(t, "the undone grant", nil)
	if got := f.passesOf(t, f.master); len(got) != 0 {
		t.Errorf("the master still reads the passes %v, want none pending", got)
	}
}

// The undo of an accepted pass takes back the ask the receiver had made, in the same step, for the Inspiration it no longer has.
func TestUndoOfAnAcceptedPassCancelsTheReceiversPendingAsk(t *testing.T) {
	t.Parallel()
	f := newFavorFight(t)
	f.give(t, f.toren)
	p := f.mustAsk(t, f.toren.GetId(), f.pens.GetId())
	if _, err := f.answer(p.GetId(), true); err != nil {
		t.Fatal(err)
	}
	onward := f.mustAsk(t, f.pens.GetId(), f.bri.GetId()) // Pensantus asks to pass it on to Brisa
	if err := f.undo(t, f.master, f.e, f.undoableLine(t).GetId()); err != nil {
		t.Fatalf("UndoLastAction() error = %v", err)
	}
	if !f.has(t, f.toren.GetId()) || f.has(t, f.pens.GetId()) || f.passStatus(t, onward.GetId()) != "canceled" {
		t.Errorf("Toren %v, Pensantus %v, onward ask %q, want it back with Toren and the ask canceled", f.has(t, f.toren.GetId()), f.has(t, f.pens.GetId()), f.passStatus(t, onward.GetId()))
	}
	if all, pending := f.passRows(t); pending != 0 || all != 2 {
		t.Errorf("passes: %d rows, %d pending, want 2 and none pending", all, pending)
	}
}

// One player owns the giver and the receiver (Pensantus fell, and Caio plays her now): the accepted pass is one line for him, the giver's,
// and the undo of the pass writes nothing for him, as the receiver's sentence of it is the receiver's alone and the giver's has none.
func TestAGiverAndAReceiverOfOnePlayerReadTheGiversLineAlone(t *testing.T) {
	t.Parallel()
	f := newFavorFight(t)
	f.execSQL(t, `UPDATE characters SET status = 'dead', died_at = now(), player_user_id = $2 WHERE id = $1`, f.pens.GetId(), f.caio.id)
	f.execSQL(t, `UPDATE combatants SET user_id = $2 WHERE character_id = $1`, f.pens.GetId(), f.caio.id)
	f.give(t, f.toren)
	pass := f.mustAsk(t, f.toren.GetId(), f.pens.GetId())
	if _, err := f.answer(pass.GetId(), true); err != nil {
		t.Fatalf("AnswerFavorPass(Aceitar) error = %v", err)
	}
	f.wantTexts(t, "the accept of a pass between one player's characters", map[*user][]string{
		f.master: {"Toren passou a Inspiração para Pensantus."},
		f.caio:   {"O mestre aceitou: a sua Inspiração foi para Pensantus."},
	})
	if err := f.undo(t, f.master, f.e, f.undoableLine(t).GetId()); err != nil {
		t.Fatalf("UndoLastAction(the pass) error = %v", err)
	}
	f.wantTexts(t, "the undo of that pass", map[*user][]string{})
}
