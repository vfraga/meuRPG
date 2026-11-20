package leaktest

import (
	"testing"
	"time"

	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/types/known/timestamppb"

	charactersv1 "github.com/PuraFome/meuRPG/backend/gen/meurpg/characters/v1"
	playv1 "github.com/PuraFome/meuRPG/backend/gen/meurpg/play/v1"
	"github.com/PuraFome/meuRPG/backend/gen/meurpg/play/v1/playv1connect"
)

// A readied move and its release (RN-10, #856): while the READIED window of the move is open and its player is on the map
// (marked as rolling), the trigger, the held move and the window are the readier's player's and the master's; Ana and the TV
// read the wait and nothing of it: the wait is the same before the mark, after it, after the clear and after the second mark, and no read of theirs holds `rolling` or `waitBeganAt`.
// The move itself is a move like any other: once made, the window is gone, the token is
// where it went, and Ana reads the fired line with the actor alone. The master's and Caio's reads are the positive controls.

// movedLinesOf counts the moved lines of a person's log whose actor is id.
func movedLinesOf(entries []*playv1.CombatLogEntry, id string) int {
	n := 0
	for _, e := range entries {
		if e.GetKind() == playv1.CombatLogKind_COMBAT_LOG_KIND_MOVED && e.GetActorId() == id {
			n++
		}
	}
	return n
}

func TestAReadiedMovesWindowAndReleaseAreTheOwnersAndTheMasters(t *testing.T) {
	w := newWorld(t)
	w.onTorenTurn()
	toren := w.combatant(w.toren)
	trigger := w.secrets.marker("readied-move-trigger", w.caio)
	words := []string{`"triggerText"`, "READIED_KIND_MOVE", "READIED_STATUS_WAITING", "READIED_STATUS_FIRED"}
	for _, n := range words {
		w.secrets.add(&canary{needle: n, kind: "readied-move-detail", readers: names([]*person{w.caio})})
	}
	rowID := w.combatantOf(must(w.caio.ready.ReadyAction(t.Context(), rq(&playv1.ReadyActionRequest{
		CampaignId: w.campaign, EncounterId: w.encounter.GetId(), CombatantId: toren.GetId(), IdempotencyKey: newKey(),
		TriggerText: trigger, Kind: playv1.ReadiedKind_READIED_KIND_MOVE,
	}))).GetEncounter(), w.toren.GetId()).GetReadied().GetId()
	w.endTurnOf(toren)
	must(w.master.ready.FireReadied(t.Context(), rq(&playv1.FireReadiedRequest{CampaignId: w.campaign, EncounterId: w.encounter.GetId(), ReadiedId: rowID, IdempotencyKey: newKey()})))
	windows := w.master.currentEncounter(w).GetReactionWindows()
	if len(windows) != 1 || windows[0].GetKind() != playv1.ReactionKind_REACTION_KIND_READIED {
		t.Fatalf("the master's windows = %v, want the READIED one", windows)
	}
	id := windows[0].GetId()
	w.secrets.add(&canary{needle: id, kind: "readied-move-window-id", readers: names([]*person{w.caio})})
	anaBefore := w.waitReadOf(w.ana)
	var began *timestamppb.Timestamp // when the wait began: the first mark's, whatever the marks and clears after it
	readsOf := func(when string, rolling bool) {
		t.Helper()
		for _, p := range []*person{w.master, w.caio} { // the positive controls
			w.readsHoldAll(t, p, when, append([]string{id, trigger}, words[:3]...)...)
			if got := windowOfKind(p, w, playv1.ReactionKind_REACTION_KIND_READIED); got == nil || got.GetRolling() != rolling {
				t.Errorf("%s: %s reads the window %v, want rolling %v", when, p.name, got, rolling)
			}
			if got := w.waitBeganOf(p); !proto.Equal(got, began) {
				t.Errorf("%s: %s reads the wait began at %v, want %v", when, p.name, got, began)
			}
		}
		w.inspectPlayers(t, w.ana, w.pending)
		tv := w.readScreen(t, when, w.tvReader(), nil)
		w.sameWaitAs(t, when, w.ana, anaBefore, "Esperando a reação de Toren", tv)
	}
	if r := w.markRolling(w.caio, id, false); !r.ok() {
		t.Fatalf("Caio's mark = %d %s", r.status, r.body)
	}
	if began = w.waitBeganOf(w.master); began == nil {
		t.Fatal("the master reads no beginning of the wait after the first mark")
	}
	w.clock.advance(5 * time.Second) // the wait has run: waitedMs is not 0, and is in their reads
	readsOf("while Toren's player is on the map", true)
	for _, p := range []*person{w.master, w.caio} { // the positive controls: both fields are in their reads
		w.readsHoldAll(t, p, "while Toren's player is on the map", `"rolling"`, `"waitBeganAt"`, `"waitedMs"`)
	}
	if r := w.markRolling(w.caio, id, true); !r.ok() {
		t.Fatalf("Caio's clear = %d %s", r.status, r.body)
	}
	readsOf("after Toren's player went back", false)
	if r := w.markRolling(w.caio, id, false); !r.ok() {
		t.Fatalf("Caio's second mark = %d %s", r.status, r.body)
	}
	readsOf("after Toren's player went to the map again", true)

	from := w.combatant(w.toren)
	moved := w.caio.call(playv1connect.CombatServiceMoveCombatantProcedure, &playv1.MoveCombatantRequest{
		CampaignId: w.campaign, EncounterId: w.encounter.GetId(), CombatantId: toren.GetId(), IdempotencyKey: newKey(), Col: from.GetCol() - 2, Row: from.GetRow(),
	})
	if !moved.ok() {
		t.Fatalf("Toren's readied move: status %d %s", moved.status, moved.body)
	}
	if got := w.master.currentEncounter(w); len(got.GetReactionWindows()) != 0 || !w.combatantOf(got, w.toren.GetId()).GetReactionUsed() {
		t.Errorf("after the move: windows %v, reaction used %v", got.GetReactionWindows(), w.combatantOf(got, w.toren.GetId()).GetReactionUsed())
	}
	for _, p := range []*person{w.master, w.caio} { // the positive controls
		if n := movedLinesOf(w.logOfPerson(p), toren.GetId()); n != 1 {
			t.Errorf("%s reads %d moved lines of Toren, want 1", p.name, n)
		}
		if lines, detailed := readiedKindLinesOf(w.logOfPerson(p), toren.GetId(), playv1.CombatLogKind_COMBAT_LOG_KIND_READIED_FIRED); lines != 1 || detailed != 1 {
			t.Errorf("%s reads %d fired lines, %d with the detail; want 1 and 1", p.name, lines, detailed)
		}
	}
	if lines, detailed := readiedKindLinesOf(w.logOfPerson(w.ana), toren.GetId(), playv1.CombatLogKind_COMBAT_LOG_KIND_READIED_FIRED); lines != 1 || detailed != 0 {
		t.Errorf("Ana reads %d fired lines, %d with the detail; want 1 and 0", lines, detailed)
	}
	if n := movedLinesOf(w.logOfPerson(w.ana), toren.GetId()); n != 1 {
		t.Errorf("Ana reads %d moved lines of Toren, want 1 as for any move", n)
	}
	w.inspectPlayers(t, w.ana, w.pending)
	w.readScreen(t, "after Toren's readied move", w.tvReader(), nil)
}

// The master releases the readied move of an NPC no player sees (release_readied): the MOVED line it writes is that NPC's, so no
// player reads it, as no player reads any move of a creature they do not see; nor the window, the fired line or the new square.
// The NPC is hidden, or revealed but in the fog (the boss, in the guard room that nobody's eyes reach). The master's reads are
// the positive controls.
func TestAnUnseenNPCsReadiedMoveLeavesNoLineForThePlayers(t *testing.T) {
	for _, tt := range []struct {
		name string
		npc  func(*world) *charactersv1.Character
	}{
		{"hidden", func(w *world) *charactersv1.Character { return w.hiddenNPC }},
		{"revealed in the fog", func(w *world) *charactersv1.Character { return w.boss }},
	} {
		t.Run(tt.name, func(t *testing.T) {
			w := newWorld(t)
			npc := tt.npc(w)
			readiedMoveOfAnUnseenNPC(t, w, npc)
		})
	}
}

func readiedMoveOfAnUnseenNPC(t *testing.T, w *world, npc *charactersv1.Character) {
	t.Helper()
	hidden := w.combatant(npc)
	w.onTurnOf(hidden.GetId())
	trigger := w.secrets.marker("readied-hidden-move-trigger")
	rowID := w.combatantOf(must(w.master.ready.ReadyAction(t.Context(), rq(&playv1.ReadyActionRequest{
		CampaignId: w.campaign, EncounterId: w.encounter.GetId(), CombatantId: hidden.GetId(), IdempotencyKey: newKey(),
		TriggerText: trigger, Kind: playv1.ReadiedKind_READIED_KIND_MOVE,
	}))).GetEncounter(), npc.GetId()).GetReadied().GetId()
	w.endTurnOf(hidden)
	must(w.master.ready.FireReadied(t.Context(), rq(&playv1.FireReadiedRequest{CampaignId: w.campaign, EncounterId: w.encounter.GetId(), ReadiedId: rowID, IdempotencyKey: newKey()})))
	from := w.combatant(npc)
	moved := w.master.call(playv1connect.CombatServiceMoveCombatantProcedure, &playv1.MoveCombatantRequest{
		CampaignId: w.campaign, EncounterId: w.encounter.GetId(), CombatantId: hidden.GetId(), IdempotencyKey: newKey(), Col: from.GetCol() - 1, Row: from.GetRow(), ReleaseReadied: true,
	})
	if !moved.ok() {
		t.Fatalf("the master's release of the unseen NPC's readied move: status %d %s", moved.status, moved.body)
	}
	if after := w.combatantOf(w.master.currentEncounter(w), npc.GetId()); after.GetCol() == from.GetCol() && after.GetRow() == from.GetRow() {
		t.Fatalf("the unseen NPC did not move from (%d,%d)", from.GetCol(), from.GetRow())
	}
	if n := movedLinesOf(w.logOfPerson(w.master), hidden.GetId()); n != 1 { // the positive control
		t.Errorf("the master reads %d moved lines of the unseen NPC, want 1", n)
	}
	if !w.readsHold(w.master, trigger) {
		t.Error("the master's reads hold no trigger: the canary is unreachable")
	}
	for _, p := range []*person{w.ana, w.caio} {
		if n := movedLinesOf(w.logOfPerson(p), hidden.GetId()); n != 0 {
			t.Errorf("%s reads %d moved lines of the unseen NPC, want none", p.name, n)
		}
		if lines, _ := readiedKindLinesOf(w.logOfPerson(p), hidden.GetId(), playv1.CombatLogKind_COMBAT_LOG_KIND_READIED_FIRED); lines != 0 {
			t.Errorf("%s reads %d fired lines of the unseen NPC, want none", p.name, lines)
		}
	}
	w.inspectPlayers(t, w.ana, w.caio, w.pending)
	w.readScreen(t, "after the unseen NPC's readied move", w.tvReader(), nil)
}
