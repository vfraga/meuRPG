package leaktest

import (
	"testing"

	playv1 "github.com/PuraFome/meuRPG/backend/gen/meurpg/play/v1"
	"github.com/PuraFome/meuRPG/backend/gen/meurpg/play/v1/playv1connect"
)

// The release of a readied attack and what its answer carries (#938, RN-10): the attack roll and the damage it owes are what
// RollAttack would have shown the same caller, and no other player reads them or the window they came from. What the player
// is busy with while the window is marked is read by whoever reads the mark.

// The answer of Toren's player to his readied attack carries the roll and the pending damage; Ana, who does not answer the
// window, reads the wait alone, is refused her own answer as she is refused an answer to no window (a table's dice on it
// included), and holds nothing of what the release rolled in any read of hers or the TV's.
func TestTheAnswerOfAReleaseIsItsReadersAlone(t *testing.T) {
	w := newWorld(t)
	w.onTorenTurn()
	toren := w.combatant(w.toren)
	foe := w.combatant(w.seenNPC)
	w.placeForced(w.master, w.encounter.GetId(), placement{toren.GetId(), foe.GetCol() + 1, foe.GetRow()}) // next to the foe: the dagger reaches
	w.encounter = w.master.currentEncounter(w)
	w.nowSeenBy(w.caio) // moved next to the foe, Toren's player sees what Toren sees
	trigger := w.secrets.marker("readied-release-trigger", w.caio)
	rowID := w.combatantOf(must(w.caio.ready.ReadyAction(t.Context(), rq(&playv1.ReadyActionRequest{
		CampaignId: w.campaign, EncounterId: w.encounter.GetId(), CombatantId: toren.GetId(), IdempotencyKey: newKey(),
		TriggerText: trigger, Kind: playv1.ReadiedKind_READIED_KIND_ATTACK, ActionKey: "equipment:dagger", TargetCombatantId: foe.GetId(),
	}))).GetEncounter(), w.toren.GetId()).GetReadied().GetId()
	w.endTurnOf(toren)
	must(w.master.ready.FireReadied(t.Context(), rq(&playv1.FireReadiedRequest{CampaignId: w.campaign, EncounterId: w.encounter.GetId(), ReadiedId: rowID, IdempotencyKey: newKey()})))
	windows := w.master.currentEncounter(w).GetReactionWindows()
	if len(windows) != 1 || windows[0].GetKind() != playv1.ReactionKind_REACTION_KIND_READIED {
		t.Fatalf("the master's windows = %v, want the READIED one", windows)
	}
	id := windows[0].GetId()
	w.secrets.add(&canary{needle: id, kind: "readied-release-window-id", readers: names([]*person{w.caio})})
	// What the release rolled is Toren's player's and the master's: the attack's own words are the canaries of the answer.
	for _, n := range []string{`"attackRoll"`, `"pendingDamage"`} {
		w.secrets.add(&canary{needle: n, kind: "readied-release-roll", readers: names([]*person{w.caio})})
	}

	answer := func(p *person, window string) reply {
		return p.call(playv1connect.CombatServiceAnswerReactionProcedure, &playv1.AnswerReactionRequest{
			CampaignId: w.campaign, EncounterId: w.encounter.GetId(), WindowId: window, Answer: playv1.ReactionChoice_REACTION_CHOICE_USE,
			IdempotencyKey: newKey(), ExtraDieFaces: []int32{3},
		})
	}
	got, unknown := answer(w.ana, id), answer(w.ana, newKey())
	if got.ok() || got.status != unknown.status || normalizeRead(string(got.body)) != normalizeRead(string(unknown.body)) {
		t.Errorf("Ana's answer to Toren's window = %d %s, to no window = %d %s", got.status, got.body, unknown.status, unknown.body)
	}
	for _, f := range w.inspect(w.ana, got, nil) {
		t.Errorf("Ana's refused answer: %s", f)
	}

	released := w.caio.call(playv1connect.CombatServiceAnswerReactionProcedure, &playv1.AnswerReactionRequest{
		CampaignId: w.campaign, EncounterId: w.encounter.GetId(), WindowId: id, Answer: playv1.ReactionChoice_REACTION_CHOICE_USE, IdempotencyKey: newKey(),
	})
	if !released.ok() {
		t.Fatalf("Caio's release = %d %s", released.status, released.body)
	}
	if !contains(released.body, `"attackRoll"`) { // the positive control: the answer carries the roll
		t.Fatalf("the answer of the release holds no attack roll: %s", released.body)
	}
	for _, f := range w.inspect(w.caio, released, nil) {
		t.Errorf("Caio's answer: %s", f)
	}
	w.inspectPlayers(t, w.ana, w.pending)
	for _, n := range []string{`"attackRoll"`, `"pendingDamage"`} { // nothing of the answer's new fields is in a read of Ana's
		if w.readsHold(w.ana, n) {
			t.Errorf("Ana's reads hold a %s field", n)
		}
	}
	w.readScreen(t, "after Toren's readied attack was released", w.tvReader(), nil)
}

// What the player is busy with follows the mark: the master and Toren's player read it with `rolling`, and the clear takes it
// off; Ana and the TV read no `busy` field in any of the states. The master's and Caio's reads are the positive controls.
func TestWhatAReadiersPlayerIsBusyWithIsTheOwnersAndTheMasters(t *testing.T) {
	w := newWorld(t)
	w.onTorenTurnAsWizard()
	toren := w.combatant(w.toren)
	rowID := w.combatantOf(must(w.caio.ready.ReadyAction(t.Context(), rq(&playv1.ReadyActionRequest{
		CampaignId: w.campaign, EncounterId: w.encounter.GetId(), CombatantId: toren.GetId(), IdempotencyKey: newKey(),
		TriggerText: w.secrets.marker("readied-busy-trigger", w.caio), Kind: playv1.ReadiedKind_READIED_KIND_SPELL, SpellKey: "spell:hold-person", SlotLevel: 2,
	}))).GetEncounter(), w.toren.GetId()).GetReadied().GetId()
	w.endTurnOf(toren)
	must(w.master.ready.FireReadied(t.Context(), rq(&playv1.FireReadiedRequest{CampaignId: w.campaign, EncounterId: w.encounter.GetId(), ReadiedId: rowID, IdempotencyKey: newKey()})))
	windows := w.master.currentEncounter(w).GetReactionWindows()
	if len(windows) != 1 || windows[0].GetKind() != playv1.ReactionKind_REACTION_KIND_READIED {
		t.Fatalf("the master's windows = %v, want the READIED one", windows)
	}
	id := windows[0].GetId()
	anaBefore := w.waitReadOf(w.ana)
	check := func(when string, want playv1.ReactionBusy) {
		t.Helper()
		for _, p := range []*person{w.master, w.caio} {
			if got := windowOfKind(p, w, playv1.ReactionKind_REACTION_KIND_READIED).GetBusy(); got != want {
				t.Errorf("%s: %s reads busy %v, want %v", when, p.name, got, want)
			}
			if want != playv1.ReactionBusy_REACTION_BUSY_UNSPECIFIED {
				w.readsHoldAll(t, p, when, `"busy"`) // the positive control
			}
		}
		w.inspectPlayers(t, w.ana, w.pending)
		tv := w.readScreen(t, when, w.tvReader(), nil)
		w.sameWaitAs(t, when, w.ana, anaBefore, "Esperando a reação de Toren", tv)
	}
	check("before the mark", playv1.ReactionBusy_REACTION_BUSY_UNSPECIFIED)
	if r := w.markBusy(w.caio, id, playv1.ReactionBusy_REACTION_BUSY_DICE); !r.ok() {
		t.Fatalf("Caio's mark of the dice = %d %s", r.status, r.body)
	}
	check("while Toren's player types the dice", playv1.ReactionBusy_REACTION_BUSY_DICE)
	if r := w.markBusy(w.caio, id, playv1.ReactionBusy_REACTION_BUSY_UNSPECIFIED); !r.ok() {
		t.Fatalf("Caio's mark that names nothing = %d %s", r.status, r.body)
	}
	check("while Toren's player is on the map", playv1.ReactionBusy_REACTION_BUSY_MAP)
	if r := w.markBusy(w.ana, id, playv1.ReactionBusy_REACTION_BUSY_DICE); r.ok() {
		t.Errorf("Ana's mark of Toren's window = %d %s, want it refused", r.status, r.body)
	}
	check("after Ana's refused mark", playv1.ReactionBusy_REACTION_BUSY_MAP)
	if r := w.markRolling(w.caio, id, true); !r.ok() {
		t.Fatalf("Caio's clear = %d %s", r.status, r.body)
	}
	check("after the clear", playv1.ReactionBusy_REACTION_BUSY_UNSPECIFIED)
}

// nowSeenBy makes the names and ids of the combatants p reads in the combat now p's to read: what the master's move of a
// character put in its sight is its player's, and the canaries of it are registered as the master's and Ana's.
func (w *world) nowSeenBy(p *person) {
	w.t.Helper()
	for _, c := range p.currentEncounter(w).GetCombatants() {
		for _, k := range w.secrets.list {
			if k.needle == c.GetId() || k.needle == c.GetLabel() {
				k.readers[p.name] = true
			}
		}
	}
}
