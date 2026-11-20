package leaktest

import (
	"slices"
	"testing"
	"time"

	"connectrpc.com/connect"
	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/types/known/timestamppb"

	playv1 "github.com/PuraFome/meuRPG/backend/gen/meurpg/play/v1"
)

// The targets of a held spell (#857) are listed in its readier's options, which only the readier's player and the master
// read (RN-10). Ana is refused the options of Toren, and nothing of the held spell is in what she or the TV reads.
func TestTheTargetsOfAHeldSpellAreTheOwnersAndTheMasters(t *testing.T) {
	w := newWorld(t)
	w.onTorenTurnAsWizard()
	toren := w.combatant(w.toren)
	for _, n := range readiedSpellWords {
		w.secrets.add(&canary{needle: n, kind: "readied-spell-targets", readers: names([]*person{w.caio})})
	}
	must(w.caio.ready.ReadyAction(t.Context(), rq(&playv1.ReadyActionRequest{
		CampaignId: w.campaign, EncounterId: w.encounter.GetId(), CombatantId: toren.GetId(), IdempotencyKey: newKey(),
		TriggerText: w.secrets.marker("readied-spell-targets-trigger", w.caio), Kind: playv1.ReadiedKind_READIED_KIND_SPELL, SpellKey: "spell:hold-person", SlotLevel: 2,
	})))
	w.endTurnOf(toren) // the options list the held spell off the readier's turn
	options := func(p *person) (*playv1.GetTurnOptionsResponse, error) {
		res, err := p.combat.GetTurnOptions(t.Context(), rq(&playv1.GetTurnOptionsRequest{CampaignId: w.campaign, EncounterId: w.encounter.GetId(), CombatantId: toren.GetId()}))
		if err != nil {
			return nil, err
		}
		return res.Msg, nil
	}
	for _, p := range []*person{w.master, w.caio} {
		res, err := options(p)
		if err != nil {
			t.Fatalf("%s reads Toren's options: %v", p.name, err)
		}
		held := false
		for _, st := range res.GetSpellTargets() {
			held = held || (st.GetSpellKey() == "spell:hold-person" && len(st.GetTargets()) > 0)
		}
		if !held {
			t.Errorf("%s's options list no targets of the held Hold Person: the canary is unreachable", p.name)
		}
	}
	if _, err := options(w.ana); err == nil {
		t.Error("Ana read the options of Toren while he holds a spell")
	}
	w.inspectPlayers(t, w.ana, w.pending)
	w.readScreen(t, "after Toren readied a spell and his options were read", w.tvReader(), nil)
}

// The READIED window of a held spell, marked as being rolled while its player is on the map (#857): the held spell, its
// level and the mark are the readier's player's and the master's. Ana and the TV read the wait ("Esperando a reação de
// Toren") and nothing of the spell; Ana's wait line is the same after the mark, the clear and the second mark as before it, and no read of hers or of
// the TV's holds `rolling` or `waitBeganAt` (the wait began at the first mark and the master and Caio read the same moment
// after a clear and a second mark); her own mark is refused as the mark of a window that does not exist. The master's and Caio's
// reads are the positive controls.
func TestTheMarkedWindowOfAHeldSpellIsTheOwnersAndTheMasters(t *testing.T) {
	w := newWorld(t)
	w.onTorenTurnAsWizard()
	toren := w.combatant(w.toren)
	for _, n := range readiedSpellWords {
		w.secrets.add(&canary{needle: n, kind: "readied-spell-marked", readers: names([]*person{w.caio})})
	}
	rowID := w.combatantOf(must(w.caio.ready.ReadyAction(t.Context(), rq(&playv1.ReadyActionRequest{
		CampaignId: w.campaign, EncounterId: w.encounter.GetId(), CombatantId: toren.GetId(), IdempotencyKey: newKey(),
		TriggerText: w.secrets.marker("readied-spell-marked-trigger", w.caio), Kind: playv1.ReadiedKind_READIED_KIND_SPELL, SpellKey: "spell:hold-person", SlotLevel: 2,
	}))).GetEncounter(), w.toren.GetId()).GetReadied().GetId()
	w.endTurnOf(toren)
	must(w.master.ready.FireReadied(t.Context(), rq(&playv1.FireReadiedRequest{CampaignId: w.campaign, EncounterId: w.encounter.GetId(), ReadiedId: rowID, IdempotencyKey: newKey()})))
	windows := w.master.currentEncounter(w).GetReactionWindows()
	if len(windows) != 1 || windows[0].GetKind() != playv1.ReactionKind_REACTION_KIND_READIED {
		t.Fatalf("the master's windows = %v, want the READIED one", windows)
	}
	id := windows[0].GetId()
	w.secrets.add(&canary{needle: id, kind: "readied-spell-marked-window-id", readers: names([]*person{w.caio})})
	anaBefore := w.waitReadOf(w.ana)
	var began *timestamppb.Timestamp // when the wait began: the first mark's, whatever the marks and clears after it
	readsOf := func(when string, rolling bool) {
		t.Helper()
		for _, p := range []*person{w.master, w.caio} {
			w.readsHoldAll(t, p, when, append([]string{id}, readiedSpellWords...)...)
			if got := windowOfKind(p, w, playv1.ReactionKind_REACTION_KIND_READIED).GetRolling(); got != rolling {
				t.Errorf("%s: %s reads rolling %v, want %v", when, p.name, got, rolling)
			}
			if got := w.waitBeganOf(p); !proto.Equal(got, began) {
				t.Errorf("%s: %s reads the wait began at %v, want %v", when, p.name, got, began)
			}
		}
		w.inspectPlayers(t, w.ana, w.pending)
		tv := w.readScreen(t, when, w.tvReader(), nil)
		w.sameWaitAs(t, when, w.ana, anaBefore, "Esperando a reação de Toren", tv)
	}
	readsOf("before the mark", false)
	if r := w.markRolling(w.caio, id, false); !r.ok() {
		t.Fatalf("Caio's mark = %d %s", r.status, r.body)
	}
	if began = w.waitBeganOf(w.master); began == nil {
		t.Fatal("the master reads no beginning of the wait after the first mark")
	}
	w.clock.advance(5 * time.Second) // the wait has run: waitedMs is not 0, and is in their reads
	readsOf("while Toren's player is marked as rolling", true)
	for _, p := range []*person{w.master, w.caio} { // the positive controls: both fields are in their reads
		w.readsHoldAll(t, p, "while Toren's player is marked as rolling", `"rolling"`, `"waitBeganAt"`, `"waitedMs"`)
	}
	got, unknown := w.markRolling(w.ana, id, false), w.markRolling(w.ana, newKey(), false)
	if got.ok() || got.status != unknown.status || normalizeRead(string(got.body)) != normalizeRead(string(unknown.body)) {
		t.Errorf("Ana's mark of Toren's window = %d %s, of no window = %d %s", got.status, got.body, unknown.status, unknown.body)
	}
	for _, f := range w.inspect(w.ana, got, nil) {
		t.Errorf("Ana's refused mark: %s", f)
	}
	if r := w.markRolling(w.ana, id, true); r.ok() {
		t.Errorf("Ana's clear of Toren's window = %d %s, want it refused", r.status, r.body)
	}
	readsOf("after Ana's refused clear", true)
	if r := w.markRolling(w.caio, id, true); !r.ok() {
		t.Fatalf("Caio's clear = %d %s", r.status, r.body)
	}
	readsOf("after the mark is taken off", false)
	if r := w.markRolling(w.caio, id, false); !r.ok() {
		t.Fatalf("Caio's second mark = %d %s", r.status, r.body)
	}
	readsOf("after the mark is set again", true)
}

// heldTargetIDs are the combatants the person's options of the combatant list as targets of the spell.
func (w *world) heldTargetIDs(p *person, c *playv1.Combatant, spell string) []string {
	w.t.Helper()
	res := must(p.combat.GetTurnOptions(w.t.Context(), rq(&playv1.GetTurnOptionsRequest{CampaignId: w.campaign, EncounterId: w.encounter.GetId(), CombatantId: c.GetId()})))
	var ids []string
	for _, st := range res.GetSpellTargets() {
		if st.GetSpellKey() != spell {
			continue
		}
		for _, tg := range st.GetTargets() {
			ids = append(ids, tg.GetCombatantId())
		}
	}
	return ids
}

// The targets of a held spell and the answer that names them leave out an NPC the players do not see (RN-10): the readier's
// options list the hidden NPC for nobody, the master included (the release picks as the readier's player does); a pick of it is
// refused exactly as a pick of an id that is nowhere, so the refusal does not tell it from one; and a player who is not the readier's is denied the answer with or without targets.
func TestAHeldSpellsTargetsAndItsAnswerKeepTheHiddenNPCApart(t *testing.T) {
	w := newWorld(t)
	w.onTorenTurnAsWizard()
	toren, hidden := w.combatant(w.toren), w.combatant(w.hiddenNPC)
	readied := must(w.caio.ready.ReadyAction(t.Context(), rq(&playv1.ReadyActionRequest{
		CampaignId: w.campaign, EncounterId: w.encounter.GetId(), CombatantId: toren.GetId(), IdempotencyKey: newKey(),
		TriggerText: w.secrets.marker("readied-hidden-pick-trigger", w.caio), Kind: playv1.ReadiedKind_READIED_KIND_SPELL, SpellKey: "spell:hold-person", SlotLevel: 2,
	}))).GetEncounter()
	w.endTurnOf(toren)
	if w.combatantOf(w.master.currentEncounter(w), w.hiddenNPC.GetId()).GetId() != hidden.GetId() { // the positive control: the master's own read of the combat lists the hidden NPC
		t.Fatal("the master's combatant list has no hidden NPC: the canary is unreachable")
	}
	// The release picks what the readier's player could pick, whoever answers: both lists hold the same creatures, not the hidden NPC.
	player, master := w.heldTargetIDs(w.caio, toren, "spell:hold-person"), w.heldTargetIDs(w.master, toren, "spell:hold-person")
	slices.Sort(player)
	slices.Sort(master)
	if len(player) == 0 || slices.Contains(player, hidden.GetId()) || !slices.Equal(player, master) {
		t.Errorf("the held Hold Person's targets: Caio's list = %v, the master's = %v; want the same creatures, not the hidden NPC", player, master)
	}

	ctx := t.Context()
	must(w.master.ready.FireReadied(ctx, rq(&playv1.FireReadiedRequest{CampaignId: w.campaign, EncounterId: w.encounter.GetId(), ReadiedId: w.combatantOf(readied, w.toren.GetId()).GetReadied().GetId(), IdempotencyKey: newKey()})))
	window := w.master.currentEncounter(w).GetReactionWindows()[0].GetId()
	answer := func(p *person, window string, targets ...string) error {
		req := &playv1.AnswerReactionRequest{CampaignId: w.campaign, EncounterId: w.encounter.GetId(), WindowId: window, Answer: playv1.ReactionChoice_REACTION_CHOICE_USE, IdempotencyKey: newKey()}
		for _, id := range targets {
			req.Targets = append(req.Targets, &playv1.SpellTarget{CombatantId: id})
		}
		_, err := p.combat.AnswerReaction(ctx, rq(req))
		return err
	}
	picked, nowhere := answer(w.caio, window, hidden.GetId()), answer(w.caio, window, newKey()) // newKey is a UUID, as a combatant's id is
	if picked == nil || nowhere == nil || picked.Error() != nowhere.Error() || connect.CodeOf(picked) != connect.CodeNotFound || connect.CodeOf(nowhere) != connect.CodeNotFound {
		t.Errorf("a pick of the hidden NPC = %v, a pick of an id that is nowhere = %v; want the same refusal, not_found", picked, nowhere)
	}
	none, with, without := answer(w.ana, newKey(), w.combatant(w.pens).GetId()), answer(w.ana, window, w.combatant(w.pens).GetId()), answer(w.ana, window)
	if none == nil || with == nil || without == nil || with.Error() != none.Error() || without.Error() != none.Error() || connect.CodeOf(with) != connect.CodeOf(none) {
		t.Errorf("Ana's answer to a window that is nowhere = %v, to Toren's with targets = %v and without = %v; want the same denial", none, with, without)
	}
	w.inspectPlayers(t, w.ana, w.pending)
	w.readScreen(t, "after the answers to Toren's held spell were refused", w.tvReader(), nil)
}
