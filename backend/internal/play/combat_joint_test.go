package play

import (
	"slices"
	"strings"
	"testing"
	"time"

	"connectrpc.com/connect"
	"google.golang.org/protobuf/encoding/protojson"
	"google.golang.org/protobuf/proto"

	charactersv1 "github.com/PuraFome/meuRPG/backend/gen/meurpg/characters/v1"
	playv1 "github.com/PuraFome/meuRPG/backend/gen/meurpg/play/v1"
)

// Joint turns (MR-013, RN-19, RN-20): combatants adjacent in the order with the
// same initiative total take one turn together. These tests need the database.

// jointFight is the canonical fight with Toren at 19 like Brisa (the artboard's
// example): the order is Brisa 19, Toren 19, Capitão 16, Pensantus 14,
// Goblin 1 12, Goblin 2 12. Both goblins are revealed unless hidden says so.
func jointFight(t *testing.T, hideGoblins bool) (*armed, *playv1.Encounter) {
	t.Helper()
	a := newArmed(t)
	reveal := []string{"Goblin 1", "Goblin 2"}
	if hideGoblins {
		reveal = nil
	}
	e := a.start(t, plan{
		npcs: []*playv1.Participant{
			{CharacterId: a.capitao.GetId(), Hidden: new(false)},
			{CharacterId: a.goblin.GetId(), Count: 2},
		},
		npcRolls: []int{16, 12, 12},
		players:  map[string]int32{"Brisa": 16, "Toren": 17, "Pensantus": 12}, // 19, 19 (the bonuses are +3 and +2), 14
		reveal:   reveal,
		at: map[string][2]int32{
			"Brisa": {9, 4}, "Toren": {5, 5}, "Capitão Goblin": {10, 5}, "Pensantus": {8, 2}, "Goblin 1": {6, 5}, "Goblin 2": {14, 2},
		},
	})
	return a, e
}

// endPart ends one member's part, as u.
func (a *armed) endPart(t *testing.T, u *user, e *playv1.Encounter, label string) (*playv1.Encounter, error) {
	t.Helper()
	res, err := u.combat.EndTurn(t.Context(), connect.NewRequest(&playv1.EndTurnRequest{
		CampaignId: a.campaignID, EncounterId: e.GetId(), IdempotencyKey: newKey(), ExpectedCombatantId: a.id(t, label),
	}))
	if err != nil {
		return nil, err
	}
	return res.Msg.GetEncounter(), nil
}

func (a *armed) mustEndPart(t *testing.T, u *user, e *playv1.Encounter, label string) *playv1.Encounter {
	t.Helper()
	out, err := a.endPart(t, u, e, label)
	if err != nil {
		t.Fatalf("EndTurn(%s's part) error = %v", label, err)
	}
	return out
}

// group says who is on turn, by label, as the viewer's copy lists them.
func group(e *playv1.Encounter) string {
	var out []string
	for _, id := range e.GetTurnGroupIds() {
		for _, c := range e.GetCombatants() {
			if c.GetId() == id {
				out = append(out, c.GetLabel())
			}
		}
	}
	return strings.Join(out, ",")
}

func current(e *playv1.Encounter) string {
	for _, c := range e.GetCombatants() {
		if c.GetId() == e.GetCurrentCombatantId() {
			return c.GetLabel()
		}
	}
	return ""
}

func TestMR013_PlayersWithTheSameInitiativeShareATurn(t *testing.T) {
	t.Parallel()
	a, e := jointFight(t, false)
	if got := strings.Join(labels(a.get(t, a.master)), ","); got != "Brisa,Toren,Capitão Goblin,Pensantus,Goblin 1,Goblin 2" {
		t.Fatalf("order = %s", got)
	}
	// The master and every player see the group on turn: Brisa and Toren.
	for who, u := range map[string]*user{"master": a.master, "Brisa's player": a.bia, "Toren's player": a.caio, "Pensantus's player": a.ana} {
		seen := a.get(t, u)
		if group(seen) != "Brisa,Toren" || current(seen) != "Brisa" || seen.GetMasterTurn() {
			t.Errorf("%s sees group %q, current %q, master_turn %v; want Brisa,Toren on turn, Brisa first", who, group(seen), current(seen), seen.GetMasterTurn())
		}
		for _, c := range seen.GetCombatants() {
			if c.GetTurnPartEnded() {
				t.Errorf("%s: %s has ended its part at the start of the turn", who, c.GetLabel())
			}
		}
	}
	// Both act at once; Pensantus does not.
	if got := a.mustOptions(t, a.caio, e, "Toren"); !got.GetYourTurn() {
		t.Error("Toren's your_turn = false in the joint turn")
	}
	if got := a.mustOptions(t, a.bia, e, "Brisa"); !got.GetYourTurn() {
		t.Error("Brisa's your_turn = false in the joint turn")
	}
	if got := a.mustOptions(t, a.ana, e, "Pensantus"); got.GetYourTurn() {
		t.Error("Pensantus's your_turn = true: her group is not on turn")
	}
	if _, err := a.action(t, a.caio, e, "Toren", "standard:dodge"); err != nil {
		t.Errorf("Toren's action in the joint turn: %v", err)
	}
	if _, err := a.action(t, a.bia, e, "Brisa", "standard:dodge"); err != nil {
		t.Errorf("Brisa's action in the joint turn: %v", err)
	}
	if _, err := a.action(t, a.ana, e, "Pensantus", "standard:dodge"); err == nil {
		t.Error("Pensantus acted in a turn that is not hers")
	} else {
		wantEncounterBlocked(t, err, reasonNotTurn)
	}
	// A player walks only on the turn they are in: Toren yes, Pensantus no.
	if _, err := a.ana.combat.MoveCombatant(t.Context(), connect.NewRequest(&playv1.MoveCombatantRequest{
		CampaignId: a.campaignID, EncounterId: e.GetId(), CombatantId: a.id(t, "Pensantus"), IdempotencyKey: newKey(), Col: 9, Row: 2,
	})); err == nil {
		t.Error("Pensantus moved on a turn that is not hers")
	}
	if _, err := a.caio.combat.MoveCombatant(t.Context(), connect.NewRequest(&playv1.MoveCombatantRequest{
		CampaignId: a.campaignID, EncounterId: e.GetId(), CombatantId: a.id(t, "Toren"), IdempotencyKey: newKey(), Col: 5, Row: 6,
	})); err != nil {
		t.Errorf("Toren's move in the joint turn: %v", err)
	}
}

func TestMR013_TheTurnPassesWhenTheLastMemberEnds(t *testing.T) {
	t.Parallel()
	a, e := jointFight(t, false)
	// Brisa ends her part: the turn stays with the group, and Toren acts.
	after := a.mustEndPart(t, a.bia, e, "Brisa")
	if after.GetRound() != 1 || group(after) != "Brisa,Toren" || current(after) != "Toren" {
		t.Fatalf("after Brisa's part: round %d, group %q, current %q; want round 1, Brisa,Toren, Toren", after.GetRound(), group(after), current(after))
	}
	if !byLabel(t, after, "Brisa").GetTurnPartEnded() || byLabel(t, after, "Toren").GetTurnPartEnded() {
		t.Error("turn_part_ended: Brisa must be true and Toren false")
	}
	// Her player and Pensantus see it too (the group holds a player).
	if seen := a.get(t, a.ana); !byLabel(t, seen, "Brisa").GetTurnPartEnded() || current(seen) != "Toren" {
		t.Errorf("Pensantus's player sees Brisa ended %v, current %q; want true, Toren", byLabel(t, seen, "Brisa").GetTurnPartEnded(), current(seen))
	}
	// A part that ended cannot act, nor be reopened.
	if _, err := a.action(t, a.bia, e, "Brisa", "standard:dodge"); err == nil {
		t.Error("Brisa acted after her part ended")
	} else {
		wantEncounterBlocked(t, err, reasonNotTurn)
	}
	if got := a.mustOptions(t, a.bia, e, "Brisa"); got.GetYourTurn() {
		t.Error("Brisa's your_turn = true after her part ended")
	}
	if _, err := a.endPart(t, a.bia, e, "Brisa"); connect.CodeOf(err) != connect.CodeAborted {
		t.Errorf("ending Brisa's part again: %v, want aborted", err)
	}
	// The log has the line, for the master and for the players.
	for who, u := range map[string]*user{"master": a.master, "Pensantus's player": a.ana} {
		if got := lines(a.log(t, u, e)); !slices.Contains(got, "R1 Brisa encerrou a parte") {
			t.Errorf("%s's log = %v, want the line \"Brisa encerrou a parte\"", who, got)
		}
	}
	// Toren ends the last part: the turn passes to the Capitão, alone.
	next := a.mustEndPart(t, a.caio, e, "Toren")
	if group(next) != "Capitão Goblin" || current(next) != "Capitão Goblin" || next.GetRound() != 1 {
		t.Fatalf("after the last part: group %q current %q round %d; want the Capitão alone in round 1", group(next), current(next), next.GetRound())
	}
	for _, c := range next.GetCombatants() {
		if c.GetTurnPartEnded() {
			t.Errorf("%s still has an ended part after the turn passed", c.GetLabel())
		}
	}
	// A group of one works as always: one end, the turn passes.
	if got := a.mustEndPart(t, a.master, e, "Capitão Goblin"); group(got) != "Pensantus" {
		t.Errorf("after the Capitão: group %q, want Pensantus", group(got))
	}
	// One part-ended event, and one turn-ended event for each turn that passed.
	var parts, turns int
	for kind, n := range map[string]*int{"turn_part_ended": &parts, "turn_ended": &turns} {
		if err := a.h.pool.QueryRow(t.Context(), `SELECT count(*) FROM session_events WHERE kind = $1`, kind).Scan(n); err != nil {
			t.Fatalf("count events: %v", err)
		}
	}
	if parts != 1 || turns != 2 {
		t.Errorf("events: turn_part_ended %d, turn_ended %d; want 1 and 2", parts, turns)
	}
}

func TestMR013_EachMemberHasItsOwnEconomy(t *testing.T) {
	t.Parallel()
	a, e := jointFight(t, false)
	// Brisa uses her action and dashes; Toren uses only his reaction's opposite:
	// nothing of his changes.
	if _, err := a.action(t, a.bia, e, "Brisa", "standard:dash"); err != nil {
		t.Fatalf("Brisa's Dash: %v", err)
	}
	brisa, toren := byLabel(t, a.get(t, a.bia), "Brisa"), byLabel(t, a.get(t, a.caio), "Toren")
	if !brisa.GetActionUsed() || !brisa.GetDashed() {
		t.Errorf("Brisa after the Dash: action_used %v dashed %v, want true and true", brisa.GetActionUsed(), brisa.GetDashed())
	}
	if toren.GetActionUsed() || toren.GetDashed() || toren.GetMovementLeftFt() != 30 {
		t.Errorf("Toren: action_used %v dashed %v movement left %d ft; want his own turn untouched (false, false, 30)", toren.GetActionUsed(), toren.GetDashed(), toren.GetMovementLeftFt())
	}
	// The players of the group read each other's economy (they act together); a
	// player outside the group does not.
	if other := byLabel(t, a.get(t, a.caio), "Brisa"); !other.GetActionUsed() || !other.GetDashed() {
		t.Error("Toren's player cannot read Brisa's economy, in the same joint turn")
	}
	if outside := byLabel(t, a.get(t, a.ana), "Brisa"); outside.GetActionUsed() || outside.GetDashed() || outside.GetMovementLeftFt() != 0 {
		t.Error("Pensantus's player, outside the group, reads Brisa's economy")
	}
	// Each one's movement is its own.
	if _, err := a.bia.combat.MoveCombatant(t.Context(), connect.NewRequest(&playv1.MoveCombatantRequest{
		CampaignId: a.campaignID, EncounterId: e.GetId(), CombatantId: a.id(t, "Brisa"), IdempotencyKey: newKey(), Col: 12, Row: 4,
	})); err != nil {
		t.Fatalf("Brisa's move: %v", err)
	}
	// Brisa walked out of the goblins' reach: they are offered an opportunity
	// attack (slice 9.6b), which this test is not about.
	a.skipOffers(t)
	if got := byLabel(t, a.get(t, a.caio), "Toren").GetMovementUsedFt(); got != 0 {
		t.Errorf("Toren's movement used = %d ft after Brisa walked, want 0", got)
	}

	// A new turn resets every member: pass the round and come back to the group.
	a.mustEndPart(t, a.bia, e, "Brisa")
	a.mustEndPart(t, a.caio, e, "Toren")
	a.mustEndPart(t, a.master, e, "Capitão Goblin")
	a.mustEndPart(t, a.ana, e, "Pensantus")
	a.mustEndPart(t, a.master, e, "Goblin 1")
	back := a.mustEndPart(t, a.master, e, "Goblin 2")
	if back.GetRound() != 2 || group(back) != "Brisa,Toren" {
		t.Fatalf("after the goblins: round %d group %q, want round 2 and Brisa,Toren", back.GetRound(), group(back))
	}
	brisa = byLabel(t, a.get(t, a.bia), "Brisa")
	if brisa.GetActionUsed() || brisa.GetDashed() || brisa.GetMovementUsedFt() != 0 || brisa.GetTurnPartEnded() {
		t.Errorf("Brisa at the start of the next round = %v, want everything reset", brisa)
	}
}

func TestMR013_TheMasterEndsAPartForAnAbsentPlayer(t *testing.T) {
	t.Parallel()
	a, e := jointFight(t, false)
	// Toren's player is away: the master ends Toren's part first, and Brisa's after.
	after := a.mustEndPart(t, a.master, e, "Toren")
	if !byLabel(t, after, "Toren").GetTurnPartEnded() || current(after) != "Brisa" || group(after) != "Brisa,Toren" {
		t.Fatalf("after the master ended Toren's part: group %q current %q; want Brisa,Toren and Brisa", group(after), current(after))
	}
	if got := a.mustEndPart(t, a.master, e, "Brisa"); group(got) != "Capitão Goblin" {
		t.Errorf("after the last part: group %q, want the Capitão", group(got))
	}
}

func TestMR013_APlayerCannotEndAnotherMembersPart(t *testing.T) {
	t.Parallel()
	a, e := jointFight(t, false)
	for _, c := range []struct {
		who  string
		u    *user
		part string
	}{
		{"Brisa's player ending Toren's part", a.bia, "Toren"},
		{"Toren's player ending Brisa's part", a.caio, "Brisa"},
		{"Pensantus's player ending Brisa's part", a.ana, "Brisa"},
		{"Brisa's player ending a revealed NPC's part", a.bia, "Capitão Goblin"},
	} {
		_, err := a.endPart(t, c.u, e, c.part)
		if c.part == "Capitão Goblin" { // not acting: a stale tap, not a permission
			wantCode(t, c.who, err, connect.CodeAborted)
			continue
		}
		wantCode(t, c.who, err, connect.CodePermissionDenied)
	}
	if got := a.get(t, a.master); group(got) != "Brisa,Toren" || byLabel(t, got, "Brisa").GetTurnPartEnded() || byLabel(t, got, "Toren").GetTurnPartEnded() {
		t.Error("a refused call changed the turn")
	}
}

func TestMR013_ADoubleTapEndsOnePartOnly(t *testing.T) {
	t.Parallel()
	a, e := jointFight(t, false)
	key := newKey()
	endBrisa := func(key string) error {
		_, err := a.bia.combat.EndTurn(t.Context(), connect.NewRequest(&playv1.EndTurnRequest{
			CampaignId: a.campaignID, EncounterId: e.GetId(), IdempotencyKey: key, ExpectedCombatantId: a.id(t, "Brisa"),
		}))
		return err
	}
	if err := endBrisa(key); err != nil {
		t.Fatalf("the first tap: %v", err)
	}
	if err := endBrisa(key); err != nil { // a retry of the same tap changes nothing
		t.Errorf("the retry of the same key: %v, want the same answer", err)
	}
	if err := endBrisa(newKey()); connect.CodeOf(err) != connect.CodeAborted { // a second tap
		t.Errorf("the double tap: %v, want aborted", err)
	}
	got := a.get(t, a.master)
	if group(got) != "Brisa,Toren" || current(got) != "Toren" || byLabel(t, got, "Toren").GetTurnPartEnded() || got.GetRound() != 1 {
		t.Errorf("after the double tap: group %q current %q; want Toren still acting in the same turn", group(got), current(got))
	}
	var parts int
	if err := a.h.pool.QueryRow(t.Context(), `SELECT count(*) FROM session_events WHERE kind = 'turn_part_ended'`).Scan(&parts); err != nil || parts != 1 {
		t.Errorf("turn_part_ended events = %d (%v), want 1", parts, err)
	}
	// Ending the last part with a stale expected id (Brisa again) never skips the next turn.
	a.mustEndPart(t, a.caio, e, "Toren")
	if err := endBrisa(newKey()); connect.CodeOf(err) != connect.CodeAborted {
		t.Errorf("a tap on Brisa after the turn passed: %v, want aborted", err)
	}
	if got := a.get(t, a.master); group(got) != "Capitão Goblin" {
		t.Errorf("group after the stale tap = %q, want the Capitão", group(got))
	}
}

func TestMR013_AnUndoStaysClosedAcrossAPartEnding(t *testing.T) {
	t.Parallel()
	a, e := jointFight(t, false)
	if _, err := a.action(t, a.bia, e, "Brisa", "standard:dodge"); err != nil {
		t.Fatalf("Brisa's Dodge: %v", err)
	}
	if a.log(t, a.master, e).GetUndoableEventId() == "" {
		t.Fatal("the Dodge is not undoable before the part ends")
	}
	a.mustEndPart(t, a.bia, e, "Brisa")
	if id := a.log(t, a.master, e).GetUndoableEventId(); id != "" {
		t.Errorf("after the part ended, the undoable event = %q, want none", id)
	}
}

func TestMR013_AMemberWhoLeavesDoesNotBlockTheTurn(t *testing.T) {
	t.Parallel()
	a, e := jointFight(t, false)
	for _, label := range []string{"Brisa", "Toren", "Capitão Goblin", "Pensantus"} {
		e = a.mustEndPart(t, a.master, e, label)
	}
	if group(e) != "Goblin 1,Goblin 2" {
		t.Fatalf("group = %q, want the goblins", group(e))
	}
	// Goblin 1 ends; the master removes Goblin 2, who still acts: the turn passes.
	a.mustEndPart(t, a.master, e, "Goblin 1")
	res, err := a.master.combat.RemoveCombatant(t.Context(), connect.NewRequest(&playv1.RemoveCombatantRequest{
		CampaignId: a.campaignID, EncounterId: e.GetId(), CombatantId: a.id(t, "Goblin 2"), IdempotencyKey: newKey(),
	}))
	if err != nil {
		t.Fatalf("RemoveCombatant() error = %v", err)
	}
	if got := res.Msg.GetEncounter(); got.GetRound() != 2 || group(got) != "Brisa,Toren" {
		t.Errorf("after the last acting member left: round %d group %q; want round 2 and Brisa,Toren", got.GetRound(), group(got))
	}
}

func TestMR013_AReinforcementWithTheSameTotalActsFromTheNextTurn(t *testing.T) {
	t.Parallel()
	a, e := jointFight(t, false)
	a.h.roller.queue(12)
	res, err := a.master.combat.AddCombatants(t.Context(), connect.NewRequest(&playv1.AddCombatantsRequest{
		CampaignId: a.campaignID, EncounterId: e.GetId(), IdempotencyKey: newKey(),
		Participants: []*playv1.Participant{{CharacterId: a.goblin.GetId(), Hidden: new(false)}},
	}))
	if err != nil {
		t.Fatalf("AddCombatants() error = %v", err)
	}
	if group(res.Msg.GetEncounter()) != "Brisa,Toren" {
		t.Fatalf("the group on turn changed to %q when a goblin joined", group(res.Msg.GetEncounter()))
	}
	for _, label := range []string{"Brisa", "Toren", "Capitão Goblin", "Pensantus"} {
		e = a.mustEndPart(t, a.master, e, label)
	}
	// Goblin 3 has the same 12: the three goblins are one group, all acting.
	if group(e) != "Goblin 1,Goblin 2,Goblin 3" {
		t.Errorf("group = %q, want the three goblins in one turn", group(e))
	}
}

func TestMR013_EachMemberOwesItsOwnDeathSave(t *testing.T) {
	t.Parallel()
	a, e := jointFight(t, false)
	a.correct(t, a.toren, hpIs(0)) // Toren is down at the start of the next joint turn
	for _, label := range []string{"Brisa", "Toren", "Capitão Goblin", "Pensantus", "Goblin 1", "Goblin 2"} {
		e = a.mustEndPart(t, a.master, e, label)
	}
	if group(e) != "Brisa,Toren" {
		t.Fatalf("group = %q, want Brisa,Toren", group(e))
	}
	if !byLabel(t, a.get(t, a.caio), "Toren").GetDeathSaveDue() || byLabel(t, a.get(t, a.bia), "Brisa").GetDeathSaveDue() {
		t.Error("death_save_due: Toren must owe one and Brisa not")
	}
	// Toren cannot end his part before the save; Brisa can end hers.
	_, err := a.endPart(t, a.caio, e, "Toren")
	wantBlockedBy(t, "Toren ending his part with a death save due", err, playv1.EncounterBlockedReason_ENCOUNTER_BLOCKED_REASON_DEATH_SAVE_DUE)
	a.mustEndPart(t, a.bia, e, "Brisa")
	a.h.roller.queue(14)
	if _, err := a.deathSave(t, a.caio, e, "Toren", rollApp); err != nil {
		t.Fatalf("Toren's death save: %v", err)
	}
	if got := a.mustEndPart(t, a.caio, e, "Toren"); group(got) != "Capitão Goblin" {
		t.Errorf("after his save and his part: group %q, want the Capitão", group(got))
	}
}

// TestRN20_NPCOnlyGroupsStayTheMasters reads everything a player gets while a
// group of NPCs alone tie on the same total: no total, no box, no member of it
// picked as the one on turn, no ended flags.
func TestRN20_NPCOnlyGroupsStayTheMasters(t *testing.T) {
	t.Parallel()
	a, e := jointFight(t, false)
	pens := a.ana.watch(t, a.campaignID)
	pens.ready(t)
	var seen []string
	read := func(what string, m proto.Message) {
		raw, err := protojson.Marshal(m)
		if err != nil {
			t.Fatalf("Marshal(%s): %v", what, err)
		}
		seen = append(seen, what+": "+string(raw))
	}
	for _, label := range []string{"Brisa", "Toren", "Capitão Goblin", "Pensantus"} {
		e = a.mustEndPart(t, a.master, e, label)
		read("GetEncounter after "+label, a.get(t, a.ana))
	}
	// The goblins' turn.
	onTurn := a.get(t, a.ana)
	read("GetEncounter on the goblins' turn", onTurn)
	if onTurn.GetCurrentCombatantId() != "" || onTurn.GetMasterTurn() {
		t.Errorf("on the goblins' turn a player has current %q master_turn %v; want no member picked, and not the master's", onTurn.GetCurrentCombatantId(), onTurn.GetMasterTurn())
	}
	if group(onTurn) != "Goblin 1,Goblin 2" {
		t.Errorf("the player's group on turn = %q, want the two goblins, to say \"Vez dos Goblins\"", group(onTurn))
	}
	if len(onTurn.GetNpcOnlyGroups()) != 1 || len(onTurn.GetNpcOnlyGroups()[0].GetCombatantIds()) != 2 {
		t.Errorf("npc_only_groups = %v, want the one pair of goblins", onTurn.GetNpcOnlyGroups())
	}
	a.mustEndPart(t, a.master, e, "Goblin 1")
	read("GetEncounter after Goblin 1's part", a.get(t, a.ana))
	// The master's log has the line; the player's never does (the group is the master's).
	for _, l := range lines(a.log(t, a.ana, e)) {
		seen = append(seen, "log: "+l)
		if strings.Contains(l, "Goblin") && strings.Contains(l, "encerrou a parte") {
			t.Errorf("a player's log has %q, a line of a group of NPCs alone", l)
		}
	}
	if !slices.ContainsFunc(lines(a.log(t, a.master, e)), func(l string) bool { return strings.Contains(l, "Goblin 1 encerrou a parte") }) {
		t.Errorf("the master's log has no line for Goblin 1's part: %v", lines(a.log(t, a.master, e)))
	}
	// Everything the player was sent, as JSON: no NPC total, no ended flag of an
	// NPC-only group, nothing of the master's.
	deadline := time.After(300 * time.Millisecond)
drain:
	for {
		select {
		case ev, ok := <-pens.events:
			if !ok {
				break drain
			}
			read("stream", ev)
		case <-deadline:
			break drain
		}
	}
	all := strings.Join(seen, "\n")
	for _, banned := range []string{`"initiative":12`, `"initiative": 12`, `"tieUnresolved"`, `"hitPoints`, `"armorClass"`} {
		if strings.Contains(all, banned) {
			t.Errorf("a player was sent %s:\n%s", banned, all)
		}
	}
	for _, c := range a.get(t, a.ana).GetCombatants() {
		if strings.HasPrefix(c.GetLabel(), "Goblin") && (c.Initiative != nil || c.InitiativeBonus != nil || c.GetTurnPartEnded()) {
			t.Errorf("a player's %s = %v, want no total and no ended flag", c.GetLabel(), c)
		}
	}
	// The master has the totals and the flags.
	master := a.get(t, a.master)
	if g1 := byLabel(t, master, "Goblin 1"); g1.GetInitiative() != 12 || !g1.GetTurnPartEnded() {
		t.Errorf("the master's Goblin 1 = %v, want initiative 12 and an ended part", g1)
	}
	if len(master.GetNpcOnlyGroups()) != 0 {
		t.Error("the master gets npc_only_groups: he works the groups out from the totals")
	}
}

// TestRN20_AHiddenMemberIsNeverNamed: Toren and a hidden goblin tie on 19. The
// player sees Toren alone, the turn waits for the master's part without a name,
// and the hidden member's line never reaches a player.
func TestRN20_AHiddenMemberIsNeverNamed(t *testing.T) {
	t.Parallel()
	a := newArmed(t)
	e := a.start(t, plan{
		npcs:     []*playv1.Participant{{CharacterId: a.goblin.GetId()}}, // hidden: the default
		npcRolls: []int{19},
		players:  map[string]int32{"Toren": 17, "Brisa": 5, "Pensantus": 5}, // Toren 19; the others far behind
		at:       map[string][2]int32{"Toren": {5, 5}, "Goblin": {6, 5}, "Brisa": {1, 1}, "Pensantus": {2, 2}},
	})
	goblinID := a.id(t, "Goblin")
	var seen []string
	read := func(what string, u *user) *playv1.Encounter {
		t.Helper()
		got := a.get(t, u)
		raw, err := protojson.Marshal(got)
		if err != nil {
			t.Fatalf("Marshal: %v", err)
		}
		seen = append(seen, what+": "+string(raw))
		return got
	}
	check := func(when string) {
		t.Helper()
		for _, s := range seen {
			if strings.Contains(s, goblinID) || strings.Contains(s, "Goblin") {
				t.Errorf("%s: a player was sent the hidden member: %s", when, s)
			}
		}
		seen = nil
	}

	if group(e) != "Toren,Goblin" {
		t.Fatalf("the master's group = %q, want Toren and the goblin", group(e))
	}
	for who, u := range map[string]*user{"Toren's player": a.caio, "Pensantus's player": a.ana} {
		got := read(who, u)
		if group(got) != "Toren" || current(got) != "Toren" || len(got.GetTurnGroupIds()) != 1 {
			t.Errorf("%s sees group %q current %q; want Toren alone", who, group(got), current(got))
		}
	}
	check("at the start of the turn")
	// A player cannot even name the hidden member who acts: not_found, as for any hidden combatant.
	_, err := a.caio.combat.EndTurn(t.Context(), connect.NewRequest(&playv1.EndTurnRequest{
		CampaignId: a.campaignID, EncounterId: e.GetId(), IdempotencyKey: newKey(), ExpectedCombatantId: goblinID,
	}))
	wantCode(t, "EndTurn naming the hidden member", err, connect.CodeNotFound)

	// Toren ends first: the turn waits for the master's part, with no name.
	a.mustEndPart(t, a.caio, e, "Toren")
	for who, u := range map[string]*user{"Toren's player": a.caio, "Pensantus's player": a.ana} {
		got := read(who, u)
		if got.GetCurrentCombatantId() != "" || !got.GetMasterTurn() || group(got) != "Toren" || !byLabel(t, got, "Toren").GetTurnPartEnded() {
			t.Errorf("%s after Toren's part: current %q master_turn %v group %q; want the master's turn (\"Falta o mestre\"), Toren ended", who, got.GetCurrentCombatantId(), got.GetMasterTurn(), group(got))
		}
		for _, l := range lines(a.log(t, u, e)) {
			seen = append(seen, "log: "+l)
		}
	}
	check("after Toren's part")
	// The master ends the hidden member's part: the turn passes; the player's
	// log never has a line of the hidden member.
	a.mustEndPart(t, a.master, e, "Goblin")
	for _, u := range []*user{a.caio, a.ana} {
		read("after the turn passed", u)
		for _, l := range lines(a.log(t, u, e)) {
			seen = append(seen, "log: "+l)
		}
	}
	check("after the turn passed")

	// The other way round: next round the hidden member ends first, and its line
	// is the master's alone.
	for _, label := range []string{"Brisa", "Pensantus"} {
		a.mustEndPart(t, a.master, e, label)
	}
	a.mustEndPart(t, a.master, e, "Goblin")
	got := read("Pensantus's player", a.ana)
	if current(got) != "Toren" || group(got) != "Toren" || got.GetMasterTurn() {
		t.Errorf("with only the hidden member ended: current %q group %q master_turn %v; want Toren acting", current(got), group(got), got.GetMasterTurn())
	}
	for _, l := range lines(a.log(t, a.ana, e)) {
		seen = append(seen, "log: "+l)
	}
	check("after the hidden member's part")
	if !slices.ContainsFunc(lines(a.log(t, a.master, e)), func(l string) bool { return strings.Contains(l, "Goblin encerrou a parte") }) {
		t.Errorf("the master's log has no line for the goblin's part: %v", lines(a.log(t, a.master, e)))
	}
}

// TestMR013_EndTurnPartAuthorization: the callers EndTurn turns away.
func TestMR013_EndTurnPartAuthorization(t *testing.T) {
	t.Parallel()
	a, e := jointFight(t, true) // the goblins are hidden
	outsider := a.h.newUser("Intruso")
	pending := a.h.newUser("Pendente")
	a.h.joinPending(a.master, a.campaignID, pending)
	pending.createCharacter(t, a.campaignID, charactersv1.CharacterKind_CHARACTER_KIND_PLAYER, "Esperando")
	call := func(u *user, expected string) error {
		_, err := u.combat.EndTurn(t.Context(), connect.NewRequest(&playv1.EndTurnRequest{
			CampaignId: a.campaignID, EncounterId: e.GetId(), IdempotencyKey: newKey(), ExpectedCombatantId: expected,
		}))
		return err
	}
	brisa := a.id(t, "Brisa")
	wantCode(t, "signed out", call(a.h.anonymous(), brisa), connect.CodeUnauthenticated)
	wantCode(t, "not a member", call(outsider, brisa), connect.CodeNotFound)
	wantCode(t, "another member's part", call(a.caio, brisa), connect.CodePermissionDenied)
	// A hidden member is not found by a player, as the combatant of every other call (an
	// aborted "stale turn" would tell it is there).
	wantCode(t, "a hidden member", call(a.caio, a.id(t, "Goblin 1")), connect.CodeNotFound)
	if err := call(a.bia, brisa); err != nil {
		t.Errorf("the member's own player: %v", err)
	}
	if err := call(a.master, a.id(t, "Toren")); err != nil {
		t.Errorf("the master for a member: %v", err)
	}
}

// TestMR013_OrderingATieKeepsTheGroup: the master orders the goblins' tie
// (SetInitiativeOrder); they stay one group, now listed in his order, and
// tie_unresolved goes away without changing who acts.
func TestMR013_OrderingATieKeepsTheGroup(t *testing.T) {
	t.Parallel()
	a := newArmed(t)
	e := a.start(t, plan{
		npcs:     []*playv1.Participant{{CharacterId: a.goblin.GetId(), Count: 2, Hidden: new(false)}},
		npcRolls: []int{12, 12},
		players:  map[string]int32{"Brisa": 16, "Toren": 17, "Pensantus": 12},
		setup:    true,
	})
	if !byLabel(t, e, "Goblin 1").GetTieUnresolved() {
		t.Fatal("the goblins' tie is not flagged before the master orders it")
	}
	if _, err := a.master.combat.SetInitiativeOrder(t.Context(), connect.NewRequest(&playv1.SetInitiativeOrderRequest{
		CampaignId: a.campaignID, EncounterId: e.GetId(), IdempotencyKey: newKey(),
		CombatantIds: []string{a.id(t, "Goblin 2"), a.id(t, "Goblin 1")},
	})); err != nil {
		t.Fatalf("SetInitiativeOrder() error = %v", err)
	}
	res, err := a.master.combat.BeginCombat(t.Context(), connect.NewRequest(&playv1.BeginCombatRequest{CampaignId: a.campaignID, EncounterId: e.GetId(), IdempotencyKey: newKey()}))
	if err != nil {
		t.Fatalf("BeginCombat() error = %v", err)
	}
	e = res.Msg.GetEncounter()
	for _, label := range []string{"Brisa", "Toren", "Pensantus"} {
		e = a.mustEndPart(t, a.master, e, label)
	}
	if group(e) != "Goblin 2,Goblin 1" || current(e) != "Goblin 2" {
		t.Errorf("after ordering the tie: group %q current %q; want Goblin 2,Goblin 1 acting together", group(e), current(e))
	}
	if byLabel(t, e, "Goblin 1").GetTieUnresolved() {
		t.Error("the tie is still unresolved after the master ordered it")
	}
}

// jointAll is a combat where everybody ties on 19, so the whole table is one
// group: Brisa, Toren and Pensantus (players) and a Goblin.
func jointAll(t *testing.T, goblins int32, reveal ...string) (*armed, *playv1.Encounter) {
	t.Helper()
	a := newArmed(t)
	e := a.start(t, plan{
		npcs:     []*playv1.Participant{{CharacterId: a.goblin.GetId(), Count: goblins}},
		npcRolls: []int{19, 19, 19},
		players:  map[string]int32{"Brisa": 16, "Toren": 17, "Pensantus": 17}, // 19, 19, 19
		reveal:   reveal,
	})
	return a, e
}

// TestMR013_AStaleTapAcrossTheRoundWrapIsRefused: with one group, the last part
// ending starts the next round with the same members acting; a late tap that
// carries the old round must not end a part of the new one.
func TestMR013_AStaleTapAcrossTheRoundWrapIsRefused(t *testing.T) {
	t.Parallel()
	a, e := jointAll(t, 1)
	if len(e.GetTurnGroupIds()) != 4 {
		t.Fatalf("group = %q, want all four", group(e))
	}
	tap := func(label string, round int32) (*playv1.Encounter, error) {
		res, err := a.master.combat.EndTurn(t.Context(), connect.NewRequest(&playv1.EndTurnRequest{
			CampaignId: a.campaignID, EncounterId: e.GetId(), IdempotencyKey: newKey(), ExpectedCombatantId: a.id(t, label), ExpectedRound: round,
		}))
		if err != nil {
			return nil, err
		}
		return res.Msg.GetEncounter(), nil
	}
	for _, label := range []string{"Brisa", "Toren", "Pensantus"} {
		if _, err := tap(label, 1); err != nil {
			t.Fatalf("EndTurn(%s): %v", label, err)
		}
	}
	wrapped, err := tap("Goblin", 1)
	if err != nil || wrapped.GetRound() != 2 || len(wrapped.GetTurnGroupIds()) != 4 {
		t.Fatalf("the last part: round %d, %v; want round 2 with the same four acting", wrapped.GetRound(), err)
	}
	// The same screen taps again, still in round 1: refused, nothing ends.
	if _, err := tap("Brisa", 1); connect.CodeOf(err) != connect.CodeAborted {
		t.Errorf("a tap carrying round 1 in round 2: %v, want aborted", err)
	}
	if got := a.get(t, a.master); byLabel(t, got, "Brisa").GetTurnPartEnded() {
		t.Error("the stale tap ended a part of round 2")
	}
	// Removing the last member who acts passes the turn, wrapping the round again.
	for _, label := range []string{"Brisa", "Toren", "Pensantus"} {
		if _, err := tap(label, 2); err != nil {
			t.Fatalf("round 2, EndTurn(%s): %v", label, err)
		}
	}
	res, err := a.master.combat.RemoveCombatant(t.Context(), connect.NewRequest(&playv1.RemoveCombatantRequest{
		CampaignId: a.campaignID, EncounterId: e.GetId(), CombatantId: a.id(t, "Goblin"), IdempotencyKey: newKey(),
	}))
	if err != nil {
		t.Fatalf("RemoveCombatant(): %v", err)
	}
	if got := res.Msg.GetEncounter(); got.GetRound() != 3 || len(got.GetTurnGroupIds()) != 3 {
		t.Errorf("after the last acting member left: round %d group %q; want round 3 with the three players", got.GetRound(), group(got))
	}
}

// TestMR013_EndingTheCombatMidGroupLeavesNoTurn: after EndEncounter nobody is on
// turn and no part is marked, for anyone.
func TestMR013_EndingTheCombatMidGroupLeavesNoTurn(t *testing.T) {
	t.Parallel()
	a, e := jointFight(t, false)
	a.mustEndPart(t, a.master, e, "Brisa")
	if _, err := a.master.combat.EndEncounter(t.Context(), connect.NewRequest(&playv1.EndEncounterRequest{
		CampaignId: a.campaignID, EncounterId: e.GetId(), IdempotencyKey: newKey(),
	})); err != nil {
		t.Fatalf("EndEncounter(): %v", err)
	}
	for who, u := range map[string]*user{"master": a.master, "player": a.ana} {
		got := a.get(t, u)
		if len(got.GetTurnGroupIds()) != 0 || got.GetCurrentCombatantId() != "" || got.GetMasterTurn() {
			t.Errorf("%s reads turn_group_ids %v, current %q after the combat ended; want none", who, got.GetTurnGroupIds(), got.GetCurrentCombatantId())
		}
		for _, c := range got.GetCombatants() {
			if c.GetTurnPartEnded() {
				t.Errorf("%s: %s still has an ended part after the combat ended", who, c.GetLabel())
			}
		}
	}
}

// TestMR013_HidingAnActingMemberThenEndingTheTurn: the master hides a goblin
// while it acts in a group with the players; the players are no longer sent it,
// the turn waits for the master's part without naming it, and ending that part
// passes the turn.
func TestMR013_HidingAnActingMemberThenEndingTheTurn(t *testing.T) {
	t.Parallel()
	a, e := jointAll(t, 1, "Goblin")
	if _, err := a.master.combat.SetCombatantHidden(t.Context(), connect.NewRequest(&playv1.SetCombatantHiddenRequest{
		CampaignId: a.campaignID, EncounterId: e.GetId(), CombatantId: a.id(t, "Goblin"), IdempotencyKey: newKey(), Hidden: true,
	})); err != nil {
		t.Fatalf("SetCombatantHidden(): %v", err)
	}
	got := a.get(t, a.ana)
	if raw, _ := protojson.Marshal(got); strings.Contains(string(raw), "Goblin") {
		t.Errorf("a player was sent the hidden member: %s", raw)
	}
	for _, label := range []string{"Brisa", "Toren", "Pensantus"} {
		a.mustEndPart(t, a.master, e, label)
	}
	got = a.get(t, a.ana)
	if !got.GetMasterTurn() || got.GetCurrentCombatantId() != "" || len(got.GetTurnGroupIds()) != 3 {
		t.Errorf("after the players' parts: master_turn %v current %q group %q; want the master's part, no name", got.GetMasterTurn(), got.GetCurrentCombatantId(), group(got))
	}
	next := a.mustEndPart(t, a.master, e, "Goblin")
	if next.GetRound() != 2 || len(next.GetTurnGroupIds()) != 4 {
		t.Errorf("after the master's part: round %d group %q; want round 2 with all four", next.GetRound(), group(next))
	}
}

// TestRN20_NPCGroupsAreNamedFromTheWholeOrder: a hidden member of a run never
// lets two groups become one nor appears in the names, and a run with one
// visible NPC is no group for the player.
func TestRN20_NPCGroupsAreNamedFromTheWholeOrder(t *testing.T) {
	t.Parallel()
	a := newArmed(t)
	e := a.start(t, plan{
		npcs:     []*playv1.Participant{{CharacterId: a.goblin.GetId(), Count: 3}, {CharacterId: a.capitao.GetId(), Hidden: new(false)}},
		npcRolls: []int{12, 12, 12, 9},
		players:  map[string]int32{"Brisa": 5, "Toren": 5, "Pensantus": 5},
		reveal:   []string{"Goblin 1", "Goblin 3"},
		setup:    true,
	})
	g2 := a.id(t, "Goblin 2")
	got := a.get(t, a.ana)
	raw, _ := protojson.Marshal(got)
	if strings.Contains(string(raw), g2) || strings.Contains(string(raw), "Goblin 2") {
		t.Fatalf("a player was sent the hidden goblin: %s", raw)
	}
	_ = e
	// Active: the three goblins tie at 12 (Goblin 2 hidden): the player's group is Goblin 1 and 3.
	res, err := a.master.combat.BeginCombat(t.Context(), connect.NewRequest(&playv1.BeginCombatRequest{CampaignId: a.campaignID, EncounterId: e.GetId(), IdempotencyKey: newKey()}))
	if err != nil {
		t.Fatalf("BeginCombat(): %v", err)
	}
	_ = res
	got = a.get(t, a.ana)
	if len(got.GetNpcOnlyGroups()) != 1 || len(got.GetNpcOnlyGroups()[0].GetCombatantIds()) != 2 {
		t.Fatalf("npc_only_groups = %v, want the one pair of visible goblins", got.GetNpcOnlyGroups())
	}
	raw, _ = protojson.Marshal(got)
	if strings.Contains(string(raw), g2) {
		t.Errorf("a player was sent the hidden goblin: %s", raw)
	}
	// Hide Goblin 3 too: one visible goblin is no group.
	if _, err := a.master.combat.SetCombatantHidden(t.Context(), connect.NewRequest(&playv1.SetCombatantHiddenRequest{
		CampaignId: a.campaignID, EncounterId: e.GetId(), CombatantId: a.id(t, "Goblin 3"), IdempotencyKey: newKey(), Hidden: true,
	})); err != nil {
		t.Fatalf("SetCombatantHidden(): %v", err)
	}
	if got = a.get(t, a.ana); len(got.GetNpcOnlyGroups()) != 0 {
		t.Errorf("npc_only_groups = %v with one visible goblin, want none", got.GetNpcOnlyGroups())
	}
}

// A member of a joint turn whose death was confirmed has no part left: the turn passes when
// the last living member ends theirs.
func TestAConfirmedDeathInAJointTurnDoesNotHoldThePass(t *testing.T) {
	t.Parallel()
	a, e := jointFight(t, false)
	// Toren (second of the Brisa,Toren group) is down with three failures; the master confirms the death.
	a.correct(t, a.toren, hpIs(0))
	a.execSQL(t, `UPDATE combatants SET death_failures = 3 WHERE id = $1`, a.id(t, "Toren"))
	if _, err := a.master.combat.ConfirmDeath(t.Context(), connect.NewRequest(&playv1.ConfirmDeathRequest{
		CampaignId: a.campaignID, EncounterId: e.GetId(), CombatantId: a.id(t, "Toren"), IdempotencyKey: newKey(),
	})); err != nil {
		t.Fatalf("ConfirmDeath() error = %v", err)
	}
	// Brisa, the only living member, ends her part: the turn must pass to the Capitão.
	got := a.mustEndPart(t, a.bia, e, "Brisa")
	if group(got) != "Capitão Goblin" {
		t.Errorf("after the last living member ended: group %q, current %q; want the turn passed to Capitão Goblin", group(got), current(got))
	}
}

// An NPC taken to 0 hit points in a joint turn has no part left: the turn passes when the
// last living member ends theirs.
func TestADefeatedNPCInAJointTurnDoesNotHoldThePass(t *testing.T) {
	t.Parallel()
	a := newArmed(t)
	e := a.start(t, plan{
		npcs: []*playv1.Participant{
			{CharacterId: a.capitao.GetId(), Hidden: new(false)},
			{CharacterId: a.goblin.GetId(), Count: 2},
		},
		npcRolls: []int{16, 12, 12},
		players:  map[string]int32{"Brisa": 13, "Toren": 17, "Pensantus": 12}, // Toren 19, Brisa 16 = Capitão 16, Pensantus 14
		reveal:   []string{"Goblin 1", "Goblin 2"},
		at: map[string][2]int32{
			"Brisa": {9, 4}, "Toren": {5, 5}, "Capitão Goblin": {10, 5}, "Pensantus": {8, 2}, "Goblin 1": {6, 5}, "Goblin 2": {14, 2},
		},
	})
	e = a.mustEndPart(t, a.master, e, "Toren")
	if group(e) != "Brisa,Capitão Goblin" {
		t.Fatalf("group = %q, want Brisa,Capitão Goblin", group(e))
	}
	if _, err := a.adjustHP(t, e, "Capitão Goblin", damageHP(1000)); err != nil {
		t.Fatalf("AdjustCombatantHitPoints() error = %v", err)
	}
	got := a.mustEndPart(t, a.bia, e, "Brisa")
	if group(got) != "Pensantus" {
		t.Errorf("after the last living member ended: group %q, current %q; want the turn passed to Pensantus", group(got), current(got))
	}
}
