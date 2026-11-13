package leaktest

import (
	"slices"
	"strings"
	"testing"

	charactersv1 "github.com/PuraFome/meuRPG/backend/gen/meurpg/characters/v1"
	playv1 "github.com/PuraFome/meuRPG/backend/gen/meurpg/play/v1"
)

// A swap of gear in a combat leaves a line in the combat log with the piece and what the swap cost: the master's
// and the owner's (RN-10), as the gear list is. Another player reads no word of it, in the log, the stream's hints
// or any other read, and the line is gone for everyone once the master undoes the swap. The master's and the owner's
// reads are the positive controls.

var gearSwapWords = []string{`"gearSwapped"`, "COMBAT_LOG_KIND_GEAR_SWAPPED"}

// onTorenTurn gives Toren a dagger and plays the combat until his turn.
func (w *world) onTorenTurn() {
	w.t.Helper()
	ctx := w.t.Context()
	sheet := w.fullSheet("class:fighter", "race:human", 3)
	sheet.GetFull().WeaponKeys = []string{"equipment:dagger"}
	must(w.master.characters.UpdateCharacter(ctx, rq(&charactersv1.UpdateCharacterRequest{
		CampaignId: w.campaign, CharacterId: w.toren.GetId(), Revision: w.toren.GetRevision(), Name: w.toren.GetName(), Sheet: sheet,
	})))
	toren := w.combatant(w.toren)
	for range 40 {
		cur := must(w.master.combat.GetEncounter(ctx, rq(&playv1.GetEncounterRequest{CampaignId: w.campaign}))).GetEncounter()
		if cur.GetCurrentCombatantId() == toren.GetId() {
			w.encounter = cur
			return
		}
		must(w.master.combat.EndTurn(ctx, rq(&playv1.EndTurnRequest{CampaignId: w.campaign, EncounterId: cur.GetId(), IdempotencyKey: newKey(), ExpectedCombatantId: cur.GetCurrentCombatantId(), ExpectedRound: cur.GetRound()})))
	}
	w.t.Fatal("it never became Toren's turn")
}

func (w *world) stowDagger() {
	w.t.Helper()
	must(w.caio.characters.SetEquipped(w.t.Context(), rq(&charactersv1.SetEquippedRequest{
		CampaignId: w.campaign, CharacterId: w.toren.GetId(), Key: "equipment:dagger", Equipped: false,
	})))
}

func gearLines(entries []*playv1.CombatLogEntry) int {
	n := 0
	for _, e := range entries {
		if e.GetKind() == playv1.CombatLogKind_COMBAT_LOG_KIND_GEAR_SWAPPED || e.GetGearSwapped() != nil {
			n++
		}
	}
	return n
}

// The line is in the master's and the owner's log (the positive controls), in no other player's, and the master's
// undo takes it away from both; nothing else a player reads holds a word of it.
func TestAGearSwapLineIsTheOwnersAndTheMastersAlone(t *testing.T) {
	w := newWorld(t)
	w.onTorenTurn()
	for _, n := range gearSwapWords {
		w.secrets.add(&canary{needle: n, kind: "gear-swapped"})
	}
	w.stowDagger()

	for _, p := range []*person{w.master, w.caio} {
		entries := w.logOfPerson(p)
		if gearLines(entries) != 1 {
			t.Fatalf("%s reads %d gear lines, want 1", p.name, gearLines(entries))
		}
		for _, e := range entries {
			if g := e.GetGearSwapped(); g != nil && g.GetNamePt() == "" {
				t.Errorf("%s: the line is empty: %v", p.name, g)
			}
		}
	}
	if got := gearLines(w.logOfPerson(w.ana)); got != 0 {
		t.Errorf("Ana reads %d gear lines, want none", got)
	}
	for _, p := range []*person{w.ana, w.pending} { // a pending member has no log at all: the reads are the check
		for _, r := range w.reads(p) {
			for _, f := range w.inspect(p, r, nil) {
				t.Errorf("%s: %s", p.name, f)
			}
		}
	}
	found := false
	for _, r := range w.reads(w.master) {
		for _, n := range gearSwapWords {
			found = found || contains(r.body, n)
		}
	}
	if !found {
		t.Error("the master's reads hold no needle: the canary is unreachable")
	}

	// The master undoes the swap: the line is gone for everyone.
	log := must(w.master.combat.ListCombatLog(t.Context(), rq(&playv1.ListCombatLogRequest{CampaignId: w.campaign, EncounterId: w.encounter.GetId()})))
	must(w.master.combat.UndoLastAction(t.Context(), rq(&playv1.UndoLastActionRequest{
		CampaignId: w.campaign, EncounterId: w.encounter.GetId(), ExpectedEventId: log.GetUndoableEventId(), IdempotencyKey: newKey(),
	})))
	for _, p := range []*person{w.caio, w.master, w.ana} {
		if got := gearLines(w.logOfPerson(p)); got != 0 {
			t.Errorf("%s reads %d gear lines after the undo, want none", p.name, got)
		}
	}
}

// The hint that the log changed goes to the master and the owner; another player hears the encounter change (its
// revision moved) and nothing of the log.
func TestAGearSwapTellsTheLogToTheOwnerAndTheMasterAlone(t *testing.T) {
	w := newWorld(t)
	w.onTorenTurn()
	got := w.hiddenOnlyEvents(w.stowDagger)
	heard := func(name string) bool {
		return slices.ContainsFunc(got[name], func(h string) bool { return strings.HasPrefix(h, "combat_log_changed") })
	}
	for _, name := range []string{w.master.name, w.caio.name} {
		if !heard(name) {
			t.Errorf("%s hears %v, want a combat_log_changed", name, got[name])
		}
	}
	if heard(w.ana.name) {
		t.Errorf("Ana hears %v, want no combat_log_changed", got[w.ana.name])
	}
}
