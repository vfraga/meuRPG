package leaktest

import (
	"bytes"
	"strings"
	"testing"

	"google.golang.org/protobuf/encoding/protojson"

	playv1 "github.com/PuraFome/meuRPG/backend/gen/meurpg/play/v1"
	"github.com/PuraFome/meuRPG/backend/gen/meurpg/play/v1/playv1connect"
)

// ridingWorld is the table of the dismount tests: Caio's Toren rides the Large mount (the fixture's visible NPC, of the party), on
// (12, 8), and it is his turn. The hidden NPC is the creature a player must not learn of.
func ridingWorld(t *testing.T) (*world, *playv1.Combatant, *playv1.Combatant, *playv1.Combatant) {
	t.Helper()
	w := newWorld(t)
	ctx := t.Context()
	e := w.encounter
	mount, hidden, rider := w.combatant(w.seenNPC), w.combatant(w.hiddenNPC), w.combatant(w.toren)
	if _, err := w.pool.Exec(ctx, `UPDATE combatants SET size = 'large', side = 'party' WHERE id = $1`, mount.GetId()); err != nil {
		t.Fatalf("make the mount Large and of the party: %v", err)
	}
	w.placeForced(w.master, e.GetId(), placement{mount.GetId(), 12, 8}, placement{hidden.GetId(), 20, 3}, placement{rider.GetId(), 12, 8})
	if _, err := w.pool.Exec(ctx, `UPDATE combatants SET mounted_on_id = $1 WHERE id = $2`, mount.GetId(), rider.GetId()); err != nil {
		t.Fatalf("put Toren on the mount: %v", err)
	}
	must(w.master.combat.SetCombatantMount(ctx, rq(&playv1.SetCombatantMountRequest{CampaignId: w.campaign, EncounterId: e.GetId(), CombatantId: mount.GetId(), IdempotencyKey: newKey(), Kind: "controlled"})))
	for range 60 {
		cur := must(w.master.combat.GetEncounter(ctx, rq(&playv1.GetEncounterRequest{CampaignId: w.campaign}))).GetEncounter()
		if cur.GetCurrentCombatantId() == rider.GetId() {
			return w, mount, hidden, rider
		}
		must(w.master.combat.EndTurn(ctx, rq(&playv1.EndTurnRequest{CampaignId: w.campaign, EncounterId: e.GetId(), IdempotencyKey: newKey(), ExpectedCombatantId: cur.GetCurrentCombatantId(), ExpectedRound: cur.GetRound()})))
	}
	t.Fatalf("it never became Toren's turn")
	return nil, nil, nil, nil
}

func (w *world) dismount(p *person, rider *playv1.Combatant, col, row int32) reply {
	return p.call(playv1connect.CombatServiceDismountCombatantProcedure, &playv1.DismountCombatantRequest{
		CampaignId: w.campaign, EncounterId: w.encounter.GetId(), RiderId: rider.GetId(), Col: col, Row: row, IdempotencyKey: newKey(),
	})
}

func (w *world) turnOptions(p *person, who *playv1.Combatant) reply {
	return p.call(playv1connect.CombatServiceGetTurnOptionsProcedure, &playv1.GetTurnOptionsRequest{
		CampaignId: w.campaign, EncounterId: w.encounter.GetId(), CombatantId: who.GetId(),
	})
}

func landsOn(r reply, col, row int32) bool {
	var o playv1.GetTurnOptionsResponse
	if !r.ok() || protojson.Unmarshal(r.body, &o) != nil {
		return false
	}
	for _, sq := range o.GetOptions().GetMount().GetLanding() {
		if sq.GetCol() == col && sq.GetRow() == row {
			return true
		}
	}
	return false
}

// A rider's dismount onto a square a hidden creature holds is a move into that square (movement.md): the options offer it as they offer
// an empty one (the twin: the hidden creature on the landing, then away, and the rider reads the same bytes), the attempt stops early,
// leaves the rider on the mount and tells it only that, and no player reads the creature, its name or a line of a dismount that did not
// happen, in the answer, the combat, the log or the stream. The master's copy is the positive control: it sees the occupant and is refused.
// The same dismount onto the square once the creature has gone is made, and the other players read it.
func TestADismountOntoAHiddenCreatureReadsAsAMoveCutShort(t *testing.T) {
	w, _, hidden, rider := ridingWorld(t)
	ctx := t.Context()
	e := w.encounter
	landing := placement{hidden.GetId(), 13, 8}
	away := placement{hidden.GetId(), 20, 3}

	// The twin of the options: with the hidden creature on the landing and away from it, the rider's options are the same bytes.
	w.placeForced(w.master, e.GetId(), landing)
	holds := w.turnOptions(w.caio, rider)
	w.placeForced(w.master, e.GetId(), away)
	empty := w.turnOptions(w.caio, rider)
	if !holds.ok() || !landsOn(empty, 13, 8) {
		t.Fatalf("premise: the rider's options with the square empty = %d %s, want it offered", empty.status, empty.body)
	}
	if holds.status != empty.status || !bytes.Equal(holds.body, empty.body) {
		t.Errorf("the rider's options with a hidden creature on the landing are %s, with the square empty %s; want the same", holds.body, empty.body)
	}

	w.placeForced(w.master, e.GetId(), landing)
	read := func(p *person) string {
		log := jsonOf(must(p.combat.ListCombatLog(ctx, rq(&playv1.ListCombatLogRequest{CampaignId: w.campaign, EncounterId: e.GetId()}))))
		return log + jsonOf(must(p.combat.GetEncounter(ctx, rq(&playv1.GetEncounterRequest{CampaignId: w.campaign}))))
	}
	var attempt, masterTry reply
	reads, landed := map[string]string{}, map[string]string{}
	var landedReply reply
	got := w.hiddenOnlyEvents(func() {
		masterTry = w.dismount(w.master, rider, 13, 8)
		attempt = w.dismount(w.caio, rider, 13, 8)
		for _, p := range []*person{w.master, w.ana, w.caio} {
			reads[p.name] = read(p)
		}
		// The control: the creature goes away and the same dismount is made.
		w.placeForced(w.master, e.GetId(), away)
		landedReply = w.dismount(w.caio, rider, 13, 8)
		for _, p := range []*person{w.master, w.ana, w.caio} {
			landed[p.name] = read(p)
		}
	})

	if masterTry.ok() || !strings.Contains(string(masterTry.body), "NO_ROOM_TO_DISMOUNT") {
		t.Fatalf("the positive control failed: the master's dismount onto the hidden creature = %d %s, want NO_ROOM_TO_DISMOUNT", masterTry.status, masterTry.body)
	}
	if !attempt.ok() {
		t.Fatalf("Caio's dismount onto the square the hidden creature holds = %d %s, want it answered as a move cut short", attempt.status, attempt.body)
	}
	if !stoppedEarly(attempt) {
		t.Errorf("the answer says %s, want stopped_early", attempt.body)
	}
	for _, p := range []*person{w.ana, w.caio} {
		for when, body := range map[string]string{"after the attempt": reads[p.name] + strings.Join(got[p.name], "\n"), "answer": string(attempt.body)} {
			for _, needle := range []string{hidden.GetId(), hidden.GetLabel()} {
				if strings.Contains(body, needle) {
					t.Errorf("%s reads %q in the %s", p.name, needle, when)
				}
			}
		}
		if strings.Contains(reads[p.name], "COMBAT_LOG_KIND_DISMOUNTED") {
			t.Errorf("%s reads a line of a dismount that did not happen", p.name)
		}
		if !strings.Contains(reads[p.name], "mountedOnId") {
			t.Errorf("%s no longer reads Toren on the mount after the attempt", p.name)
		}
	}
	if !strings.Contains(reads[w.master.name], hidden.GetLabel()) {
		t.Errorf("the positive control failed: the master's copy has no %s", hidden.GetLabel())
	}
	// The control: with the square empty the dismount is made, and the table reads it.
	if !landedReply.ok() || stoppedEarly(landedReply) {
		t.Fatalf("the dismount onto the empty square = %d %s, want it made", landedReply.status, landedReply.body)
	}
	for _, p := range []*person{w.ana, w.caio} {
		if !strings.Contains(landed[p.name], "COMBAT_LOG_KIND_DISMOUNTED") {
			t.Errorf("the positive control failed: %s reads no line of the dismount that was made", p.name)
		}
	}
}

// stoppedEarly reads the flag of a DismountCombatant answer.
func stoppedEarly(r reply) bool {
	var o playv1.DismountCombatantResponse
	return r.ok() && protojson.Unmarshal(r.body, &o) == nil && o.GetStoppedEarly()
}
