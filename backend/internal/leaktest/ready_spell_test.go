package leaktest

import (
	"testing"

	"google.golang.org/protobuf/encoding/protojson"
	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/types/known/timestamppb"

	charactersv1 "github.com/PuraFome/meuRPG/backend/gen/meurpg/characters/v1"
	playv1 "github.com/PuraFome/meuRPG/backend/gen/meurpg/play/v1"
	"github.com/PuraFome/meuRPG/backend/gen/meurpg/play/v1/playv1connect"
)

// A readied spell (RN-10): the spell held, its level, that it is a spell, and the concentration that holds it are the
// master's and the readier's player's. Another player reads that the combatant readied an action and, when it is lost,
// that it was lost; never which spell. The master's and Caio's reads are the positive controls.

// readiedSpellWords are what only the master and Caio read of Toren's readied Hold Person: its key and its name (they
// hold letters outside a-f, so no id matches them), that it is a spell, Toren's concentration, and the slot level of the
// row (`"slotLevel"`, the key alone: protojson puts a random space after a colon; it omits a zero, and no other read of
// these tests holds a slot level that is not zero).
var readiedSpellWords = []string{"spell:hold-person", "Imobilizar Pessoa", "READIED_KIND_SPELL", `"concentrationSpell"`, `"slotLevel"`}

// onTorenTurnAsWizard makes Toren a 3rd-level wizard who prepared Hold Person and plays the combat until his turn.
func (w *world) onTorenTurnAsWizard() {
	w.t.Helper()
	sheet := w.fullSheet("class:wizard", "race:human", 3)
	sheet.GetFull().KnownSpellKeys = []string{"spell:hold-person"}
	sheet.GetFull().PreparedSpellKeys = []string{"spell:hold-person"}
	must(w.master.characters.UpdateCharacter(w.t.Context(), rq(&charactersv1.UpdateCharacterRequest{
		CampaignId: w.campaign, CharacterId: w.toren.GetId(), Revision: w.toren.GetRevision(), Name: w.toren.GetName(), Sheet: sheet,
	})))
	w.onTurnOf(w.combatant(w.toren).GetId())
}

// waitRead is what a person who does not answer a window reads of the open windows: the windows themselves (none) and
// the wait line.
type waitRead struct {
	windows []*playv1.ReactionWindow
	wait    *playv1.ReactionWait
}

// waitReadOf reads the combat as p and keeps what it says of the open windows.
func (w *world) waitReadOf(p *person) waitRead {
	w.t.Helper()
	e := p.currentEncounter(w)
	return waitRead{windows: e.GetReactionWindows(), wait: e.GetReactionWait()}
}

// markRolling calls MarkReactionRolling as p, with off when it takes the mark off.
func (w *world) markRolling(p *person, windowID string, off bool) reply {
	return p.call(playv1connect.CombatServiceMarkReactionRollingProcedure, &playv1.MarkReactionRollingRequest{
		CampaignId: w.campaign, EncounterId: w.encounter.GetId(), WindowId: windowID, Clear: off,
	})
}

// waitBeganOf is when the wait began, as the master reads it on the READIED window (nil before the first mark).
func (w *world) waitBeganOf(p *person) *timestamppb.Timestamp {
	return windowOfKind(p, w, playv1.ReactionKind_REACTION_KIND_READIED).GetWaitBeganAt()
}

// sameWaitAs fails unless p, who does not answer the READIED window, reads of it the same as before: no window, and the
// wait line equal to the one read before the reactor's player marked the window, whatever the marks since (set, cleared,
// set again). It also fails when any of p's reads, or the TV's answer, holds a `rolling`, a `waitBeganAt` or a `waitedMs` field:
// the mark, the beginning of the wait and how long it has run are the master's and the reactor's player's alone.
func (w *world) sameWaitAs(t *testing.T, when string, p *person, before waitRead, title string, tv *playv1.GetTableScreenResponse) {
	t.Helper()
	now := w.waitReadOf(p)
	if len(now.windows) != 0 || now.wait.GetTitlePt() != title || len(before.windows) != 0 {
		t.Errorf("%s: %s reads the windows %v and the wait %q, want no window and %q", when, p.name, now.windows, now.wait.GetTitlePt(), title)
	}
	if !proto.Equal(now.wait, before.wait) {
		t.Errorf("%s: %s reads the wait %v, want the one read before the mark, %v", when, p.name, now.wait, before.wait)
	}
	for _, field := range []string{`"rolling"`, `"waitBeganAt"`, `"waitedMs"`} {
		if w.readsHold(p, field) {
			t.Errorf("%s: %s reads a %s field of the window", when, p.name, field)
		}
		if body, err := protojson.Marshal(tv); err != nil || contains(body, field) {
			t.Errorf("%s: the TV's answer holds a %s field (marshal error %v)", when, field, err)
		}
	}
}

// readsHoldAll fails unless the person's reads hold every needle: the positive control.
func (w *world) readsHoldAll(t *testing.T, p *person, when string, needles ...string) {
	t.Helper()
	for _, n := range needles {
		if !w.readsHold(p, n) {
			t.Errorf("%s: %s's reads hold no %q: the canary is unreachable", when, p.name, n)
		}
	}
}

func TestAReadiedSpellIsTheOwnersAndTheMasters(t *testing.T) {
	w := newWorld(t)
	w.onTorenTurnAsWizard()
	toren := w.combatant(w.toren)
	trigger := w.secrets.marker("readied-spell-trigger", w.caio)
	for _, n := range readiedSpellWords {
		w.secrets.add(&canary{needle: n, kind: "readied-spell", readers: names([]*person{w.caio})})
	}
	readied := must(w.caio.ready.ReadyAction(t.Context(), rq(&playv1.ReadyActionRequest{
		CampaignId: w.campaign, EncounterId: w.encounter.GetId(), CombatantId: toren.GetId(), IdempotencyKey: newKey(),
		TriggerText: trigger, Kind: playv1.ReadiedKind_READIED_KIND_SPELL, SpellKey: "spell:hold-person", SlotLevel: 2,
	}))).GetEncounter()
	for _, p := range []*person{w.master, w.caio} {
		w.readsHoldAll(t, p, "after the Ready", append([]string{trigger}, readiedSpellWords...)...)
		if lines, detailed := readiedLinesOf(w.logOfPerson(p), toren.GetId()); lines != 1 || detailed != 1 {
			t.Errorf("%s reads %d readied lines, %d with the spell; want 1 and 1", p.name, lines, detailed)
		}
	}
	if lines, detailed := readiedLinesOf(w.logOfPerson(w.ana), toren.GetId()); lines != 1 || detailed != 0 {
		t.Errorf("Ana reads %d readied lines, %d with the spell; want 1 and 0", lines, detailed)
	}
	w.inspectPlayers(t, w.ana, w.pending)
	w.readScreen(t, "after Toren readied a spell", w.tvReader(), nil)

	// Given up, the spell is lost with its line: the line names it for the master and Caio alone.
	must(w.caio.ready.CancelReadied(t.Context(), rq(&playv1.CancelReadiedRequest{
		CampaignId: w.campaign, EncounterId: w.encounter.GetId(), ReadiedId: w.combatantOf(readied, w.toren.GetId()).GetReadied().GetId(), IdempotencyKey: newKey(),
	})))
	lost := playv1.CombatLogKind_COMBAT_LOG_KIND_READIED_LOST
	for _, p := range []*person{w.master, w.caio} {
		w.readsHoldAll(t, p, "after the cancel", "spell:hold-person", "READIED_KIND_SPELL")
		if lines, detailed := readiedKindLinesOf(w.logOfPerson(p), toren.GetId(), lost); lines != 1 || detailed != 1 {
			t.Errorf("%s reads %d lost lines, %d with the spell; want 1 and 1", p.name, lines, detailed)
		}
	}
	if lines, detailed := readiedKindLinesOf(w.logOfPerson(w.ana), toren.GetId(), lost); lines != 1 || detailed != 0 {
		t.Errorf("Ana reads %d lost lines, %d with the spell; want 1 and 0", lines, detailed)
	}
	w.inspectPlayers(t, w.ana, w.pending)
}

// Toren's concentration save on his held Hold Person (RN-10): the master and Caio read the save and the spell it is on;
// Ana and the TV read the master's wait ("Esperando o mestre"), never that Toren owes a save, and no line of it.
func TestTheConcentrationSaveOnAReadiedSpellIsTheOwnersAndTheMasters(t *testing.T) {
	w := newWorld(t)
	w.onTorenTurnAsWizard()
	toren, bandit := w.combatant(w.toren), w.combatant(w.bandit)
	must(w.caio.ready.ReadyAction(t.Context(), rq(&playv1.ReadyActionRequest{
		CampaignId: w.campaign, EncounterId: w.encounter.GetId(), CombatantId: toren.GetId(), IdempotencyKey: newKey(),
		TriggerText: w.secrets.marker("readied-save-trigger", w.caio), Kind: playv1.ReadiedKind_READIED_KIND_SPELL, SpellKey: "spell:hold-person", SlotLevel: 2,
	})))
	for _, n := range append([]string{"teste de Constituição", "CONCENTRATION_SAVE", "concentração"}, readiedSpellWords...) {
		w.secrets.add(&canary{needle: n, kind: "readied-spell-save", readers: names([]*person{w.caio})})
	}
	must(w.master.combat.SetCombatantHidden(t.Context(), rq(&playv1.SetCombatantHiddenRequest{CampaignId: w.campaign, EncounterId: w.encounter.GetId(), CombatantId: bandit.GetId(), IdempotencyKey: newKey(), Hidden: false})))
	w.allow("bandit-name", w.ana, w.caio)
	w.secrets.allowNeedle(bandit.GetId(), w.ana, w.caio)
	w.placeForced(w.master, w.encounter.GetId(), placement{toren.GetId(), 12, 14}, placement{bandit.GetId(), 13, 14})
	w.passTurnsTo(bandit.GetId())
	if w.combatant(w.toren).GetReadied() == nil {
		t.Fatal("Toren's readied spell was lost before the bandit's turn")
	}
	w.banditHitsToren(toren, bandit, 4)

	enc := must(w.master.combat.GetEncounter(t.Context(), rq(&playv1.GetEncounterRequest{CampaignId: w.campaign}))).GetEncounter()
	if !contains([]byte(enc.GetReactionWait().GetTitlePt()), "Toren") {
		t.Errorf("the master's wait = %q, want Toren's save", enc.GetReactionWait().GetTitlePt())
	}
	w.readsHoldAll(t, w.master, "the save", "CONCENTRATION_SAVE", "spell:hold-person")
	w.readsHoldAll(t, w.caio, "the save", "CONCENTRATION_SAVE", "spell:hold-person", "Imobilizar Pessoa")
	ana := must(w.ana.combat.GetEncounter(t.Context(), rq(&playv1.GetEncounterRequest{CampaignId: w.campaign}))).GetEncounter()
	if got := ana.GetReactionWait().GetTitlePt(); got != "Esperando o mestre" {
		t.Errorf("Ana's wait = %q, want \"Esperando o mestre\"", got)
	}
	w.inspectPlayers(t, w.ana, w.pending)
	w.readScreen(t, "while Toren's save on the held spell waits", w.tvReader(), nil)
}

// A Ready that ends the readier's earlier concentration (RN-10): the master's and Caio's ACTION_READIED line names the
// concentration it ended; Ana's and the TV's line names nothing, so it does not say the Ready was a spell. Ana still
// reads Toren's concentration indicator go blank, as for any concentration that ends.
func TestTheConcentrationAReadiedSpellEndedIsTheOwnersAndTheMasters(t *testing.T) {
	w := newWorld(t)
	w.onTorenTurnAsWizard()
	toren := w.combatant(w.toren)
	if _, err := w.pool.Exec(t.Context(), `UPDATE combatants SET concentration_spell = 'spell:detect-magic' WHERE id = $1`, toren.GetId()); err != nil {
		t.Fatalf("set Toren's concentration: %v", err)
	}
	must(w.caio.ready.ReadyAction(t.Context(), rq(&playv1.ReadyActionRequest{
		CampaignId: w.campaign, EncounterId: w.encounter.GetId(), CombatantId: toren.GetId(), IdempotencyKey: newKey(),
		TriggerText: "quando o goblin se mexer", Kind: playv1.ReadiedKind_READIED_KIND_SPELL, SpellKey: "spell:hold-person", SlotLevel: 2,
	})))
	for _, c := range []struct {
		p    *person
		want string
	}{{w.master, "spell:detect-magic"}, {w.caio, "spell:detect-magic"}, {w.ana, ""}} {
		n := 0
		for _, e := range w.logOfPerson(c.p) {
			if e.GetKind() != playv1.CombatLogKind_COMBAT_LOG_KIND_ACTION_READIED || e.GetActorId() != toren.GetId() {
				continue
			}
			n++
			if got := e.GetConcentrationEndedKey(); got != c.want {
				t.Errorf("%s's ACTION_READIED line ended concentration %q, want %q", c.p.name, got, c.want)
			}
			if c.want == "" && e.GetSpell() != nil {
				t.Errorf("%s's ACTION_READIED line carries a spell: %v", c.p.name, e.GetSpell())
			}
		}
		if n != 1 {
			t.Errorf("%s reads %d ACTION_READIED lines of Toren, want 1", c.p.name, n)
		}
	}
	if got := w.combatantOf(must(w.ana.combat.GetEncounter(t.Context(), rq(&playv1.GetEncounterRequest{CampaignId: w.campaign}))).GetEncounter(), w.toren.GetId()).GetConcentrationSpell(); got != "" {
		t.Errorf("Ana reads Toren's concentration %q after the Ready, want it blank", got)
	}
}
