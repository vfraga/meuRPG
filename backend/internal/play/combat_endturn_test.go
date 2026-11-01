package play

import (
	"encoding/base64"
	"regexp"
	"slices"
	"strings"
	"testing"

	"connectrpc.com/connect"

	playv1 "github.com/PuraFome/meuRPG/backend/gen/meurpg/play/v1"
	"github.com/PuraFome/meuRPG/backend/internal/platform/dbtest"
)

// These tests freeze what EndTurn does today, branch by branch: every refusal with its code,
// reason and message, the events it writes (in order, with the ids named by label) and what it
// publishes to the streams. They are the net under the refactor of EndTurn into phases.

// endTurnAs calls EndTurn as u with a new key; edit changes the request. It does not skip the
// saving throws of effects (endTurnRaw's way), and it does not fill in the expected combatant
// unless the request says so.
func (a *armed) endTurnAs(t *testing.T, u *user, e *playv1.Encounter, edit func(*playv1.EndTurnRequest)) (*playv1.Encounter, error) {
	t.Helper()
	req := &playv1.EndTurnRequest{CampaignId: a.campaignID, EncounterId: e.GetId(), IdempotencyKey: newKey()}
	if edit != nil {
		edit(req)
	}
	res, err := u.combat.EndTurn(t.Context(), connect.NewRequest(req))
	if err != nil {
		return nil, err
	}
	return res.Msg.GetEncounter(), nil
}

var uuidPattern = regexp.MustCompile(`[0-9a-f]{8}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{12}`)

// eventTrace lists the session's events after the first `after` ones, as "kind payload" with the
// combatants' ids replaced by their labels, so a golden does not depend on random ids.
func (a *armed) eventTrace(t *testing.T, e *playv1.Encounter, after int) []string {
	t.Helper()
	names := map[string]string{}
	for _, c := range e.GetCombatants() {
		names[c.GetId()] = c.GetLabel()
	}
	rows, err := a.h.pool.Query(t.Context(), `SELECT kind, payload::TEXT FROM session_events ORDER BY seq OFFSET $1`, after)
	if err != nil {
		t.Fatalf("read the events: %v", err)
	}
	defer rows.Close()
	var out []string
	for rows.Next() {
		var kind, payload string
		if err := rows.Scan(&kind, &payload); err != nil {
			t.Fatalf("scan an event: %v", err)
		}
		payload = uuidPattern.ReplaceAllStringFunc(payload, func(id string) string {
			if l, ok := names[id]; ok {
				return l
			}
			return "?"
		})
		out = append(out, kind+" "+sortEmbeddedJSON(t, dbtest.SortJSONKeys(payload)))
	}
	return out
}

func wantTrace(t *testing.T, got, want []string) {
	t.Helper()
	if g, w := strings.Join(got, "\n"), strings.Join(want, "\n"); g != w {
		t.Errorf("events:\n%s\nwant:\n%s", g, w)
	}
}

// TestEndTurnRefusals: every refusal that comes before anything is written, with its exact
// code and message, on one fight; none of them writes an event or moves the turn.
func TestEndTurnRefusals(t *testing.T) {
	t.Parallel()
	a := newArmed(t)
	e := a.threeAndAGoblin(t) // Toren, Pensantus, Brisa, the goblin; Toren acts
	toren, pens := a.id(t, "Toren"), a.id(t, "Pensantus")
	stranger := a.h.newUser("Zé")
	cases := []struct {
		name    string
		u       *user
		edit    func(*playv1.EndTurnRequest)
		code    connect.Code
		message string
	}{
		{"a stranger to the campaign", stranger, func(r *playv1.EndTurnRequest) { r.ExpectedCombatantId = toren }, connect.CodeNotFound, "campaign not found"},
		{"a key that is not a UUID", a.caio, func(r *playv1.EndTurnRequest) { r.IdempotencyKey = "not-a-key"; r.ExpectedCombatantId = toren }, connect.CodeInvalidArgument, "idempotency_key must be a UUID"},
		{"an encounter id that is not a UUID", a.caio, func(r *playv1.EndTurnRequest) { r.EncounterId = "x"; r.ExpectedCombatantId = toren }, connect.CodeNotFound, "encounter not found"},
		{"an expected combatant that is not a UUID", a.caio, func(r *playv1.EndTurnRequest) { r.ExpectedCombatantId = "x" }, connect.CodeNotFound, "combatant not found"},
		{"an encounter of nobody", a.caio, func(r *playv1.EndTurnRequest) { r.EncounterId = newKey(); r.ExpectedCombatantId = toren }, connect.CodeNotFound, "encounter not found"},
		{"a screen of an earlier round", a.caio, func(r *playv1.EndTurnRequest) { r.ExpectedCombatantId = toren; r.ExpectedRound = 2 }, connect.CodeAborted, "that combatant's part of the turn is not the one running"},
		{"no expected combatant while one acts", a.master, nil, connect.CodeAborted, "that combatant's part of the turn is not the one running"},
		{"an expected combatant that is not in the fight", a.master, func(r *playv1.EndTurnRequest) { r.ExpectedCombatantId = newKey() }, connect.CodeAborted, "that combatant's part of the turn is not the one running"},
		{"an expected combatant whose turn has not come", a.master, func(r *playv1.EndTurnRequest) { r.ExpectedCombatantId = pens }, connect.CodeAborted, "that combatant's part of the turn is not the one running"},
		{"a player ending another player's turn", a.ana, func(r *playv1.EndTurnRequest) { r.ExpectedCombatantId = toren }, connect.CodePermissionDenied, "only the combatant's player or the master may do this"},
	}
	before := a.events(t)
	for _, c := range cases {
		_, err := a.endTurnAs(t, c.u, e, c.edit)
		wantRefusal(t, c.name, err, c.code, c.message)
	}
	if n := a.events(t); n != before {
		t.Errorf("events after the refusals = %d, want %d", n, before)
	}
	if got := a.get(t, a.master); got.GetRound() != 1 || got.GetCurrentCombatantId() != toren {
		t.Errorf("after the refusals: round %d, on turn %s; want round 1, Toren", got.GetRound(), got.GetCurrentCombatantId())
	}
}

// TestEndTurnRefusesWhileTheCombatIsNotRunning: a combat still in SETUP has no turn to end.
func TestEndTurnRefusesWhileTheCombatIsNotRunning(t *testing.T) {
	t.Parallel()
	a := newArmed(t)
	e := a.start(t, plan{
		npcs:     []*playv1.Participant{{CharacterId: a.goblin.GetId()}},
		npcRolls: []int{3},
		setup:    true,
	})
	_, err := a.endTurnAs(t, a.master, e, nil)
	wantBlockedBy(t, "EndTurn in SETUP", err, blockedNotActive)
	wantRefusal(t, "EndTurn in SETUP", err, connect.CodeFailedPrecondition, "the combat is not running")
}

// TestEndTurnOfAHiddenCombatantIsNotFoundForAPlayer: a player naming a combatant it does not see
// is told "not found" before any stale or permission answer, whoever is acting.
func TestEndTurnOfAHiddenCombatantIsNotFoundForAPlayer(t *testing.T) {
	t.Parallel()
	a := newArmed(t)
	e := a.start(t, plan{
		npcs:     []*playv1.Participant{{CharacterId: a.goblin.GetId()}},
		npcRolls: []int{20}, // the hidden goblin plays first
		players:  map[string]int32{"Toren": 10, "Pensantus": 5, "Brisa": 1},
		at:       map[string][2]int32{"Toren": {3, 3}, "Goblin": {4, 3}, "Pensantus": {10, 3}, "Brisa": {8, 8}},
	})
	goblin := a.id(t, "Goblin")
	_, err := a.endTurnAs(t, a.caio, e, func(r *playv1.EndTurnRequest) { r.ExpectedCombatantId = goblin })
	wantRefusal(t, "a player ending the hidden goblin's turn", err, connect.CodeNotFound, "combatant not found")
	_, err = a.endTurnAs(t, a.caio, e, func(r *playv1.EndTurnRequest) { r.ExpectedCombatantId = goblin; r.ExpectedRound = 3 })
	wantRefusal(t, "the same on a stale round", err, connect.CodeAborted, "that combatant's part of the turn is not the one running")
	got := a.eventTrace(t, e, 0)
	before := len(got)
	if _, err := a.endTurnAs(t, a.master, e, func(r *playv1.EndTurnRequest) { r.ExpectedCombatantId = goblin }); err != nil {
		t.Fatalf("the master ending the hidden goblin's turn: %v", err)
	}
	wantTrace(t, a.eventTrace(t, e, before), []string{`turn_ended {"discarded": 0, "from": "Goblin", "round": 1, "to": "Toren"}`})
}

// TestEndTurnRefusalsThatHoldThePart: a death save due, and a damage to roll or apply, hold a
// member's part; the master passes the damage over only when he asks to.
func TestEndTurnRefusalsThatHoldThePart(t *testing.T) {
	t.Parallel()
	a := newArmed(t)
	e := a.threeAndAGoblin(t)
	a.h.roller.queue(15)
	a.mustAttack(t, a.caio, e, "Toren", battleaxe, "Goblin", inAppRoll) // a hit whose damage is not rolled
	before := a.events(t)
	for who, u := range map[string]*user{"Toren's player": a.caio, "the master": a.master} {
		_, err := a.endTurn(t, u, e, false)
		wantBlockedBy(t, who+" with a damage to roll", err, blockedPendingDamage)
		wantRefusal(t, who+" with a damage to roll", err, connect.CodeFailedPrecondition, "a damage is still to roll or to apply")
	}
	// Only the master may drop it: a player asking to is held all the same.
	_, err := a.endTurn(t, a.caio, e, true)
	wantBlockedBy(t, "a player discarding", err, blockedPendingDamage)
	if n := a.events(t); n != before {
		t.Errorf("events after the refusals = %d, want %d", n, before)
	}
}

// TestEndTurnWaitsForTheDeathSaveOfAPlayer: Toren is down when his turn starts; neither his
// player nor the master can pass his part before the save is rolled (the master rolls it for
// him, in the app or with a typed d20), and after it the master passes it.
func TestEndTurnWaitsForTheDeathSaveOfAPlayer(t *testing.T) {
	t.Parallel()
	a := newCasters(t)
	e := a.castersFight(t, 1) // Pensantus, Toren, Brisa, Goblin, Capitão
	a.correct(t, a.toren, hpIs(0))
	a.mustEndTurn(t, a.ana, e) // Toren's turn starts with a death save due
	toren := func(r *playv1.EndTurnRequest) { r.ExpectedCombatantId = a.id(t, "Toren") }
	before := len(a.eventTrace(t, e, 0))
	for who, u := range map[string]*user{"Toren's player": a.caio, "the master": a.master} {
		_, err := a.endTurnAs(t, u, e, toren)
		wantBlockedBy(t, who, err, playv1.EncounterBlockedReason_ENCOUNTER_BLOCKED_REASON_DEATH_SAVE_DUE)
		wantRefusal(t, who, err, connect.CodeFailedPrecondition, "roll your death save before ending the turn")
	}
	// Discarding does not skip the save either.
	_, err := a.endTurnAs(t, a.master, e, func(r *playv1.EndTurnRequest) { toren(r); r.DiscardPendingDamage = true })
	wantBlockedBy(t, "the master discarding", err, playv1.EncounterBlockedReason_ENCOUNTER_BLOCKED_REASON_DEATH_SAVE_DUE)
	if n := len(a.eventTrace(t, e, 0)); n != before {
		t.Errorf("events after the refusals = %d, want %d", n, before)
	}
	// The master rolls it for Toren with a typed d20 (physical dice), then passes the part.
	if _, err := a.deathSave(t, a.master, e, "Toren", deathFace(14)); err != nil {
		t.Fatalf("the master's death save for Toren: %v", err)
	}
	before = len(a.eventTrace(t, e, 0))
	if _, err := a.endTurnAs(t, a.master, e, toren); err != nil {
		t.Fatalf("the master passing Toren's part after the save: %v", err)
	}
	got := a.eventTrace(t, e, before)
	if len(got) != 1 || !strings.HasPrefix(got[0], `turn_ended {"discarded": 0, "from": "Toren"`) {
		t.Errorf("events = %v, want one turn_ended from Toren", got)
	}
}

// TestEndTurnWritesOneEventPerPassAndTheRoundTurnsAtTheLast: the turn order of the usual fight,
// round 1 to round 2, with the events it writes and what the players read as on turn.
func TestEndTurnWritesOneEventPerPassAndTheRoundTurnsAtTheLast(t *testing.T) {
	t.Parallel()
	a := newArmed(t)
	e := a.threeAndAGoblin(t)
	start := len(a.eventTrace(t, e, 0))
	a.mustEndTurn(t, a.caio, e)
	a.mustEndTurn(t, a.ana, e)
	a.mustEndTurn(t, a.bia, e)
	out := a.mustEndTurn(t, a.master, e) // the goblin: the last of the order
	wantTrace(t, a.eventTrace(t, e, start), []string{
		`turn_ended {"discarded": 0, "from": "Toren", "round": 1, "to": "Pensantus"}`,
		`turn_ended {"discarded": 0, "from": "Pensantus", "round": 1, "to": "Brisa"}`,
		`turn_ended {"discarded": 0, "from": "Brisa", "round": 1, "to": "Goblin"}`,
		`turn_ended {"discarded": 0, "from": "Goblin", "round": 2, "to": "Toren"}`,
	})
	if out.GetRound() != 2 || current(out) != "Toren" {
		t.Errorf("after the last turn: round %d on turn %q, want round 2, Toren", out.GetRound(), current(out))
	}
}

// TestEndTurnDiscardWritesTheDroppedDamageBeforeTheTurn: the master passing the turn over a
// damage drops it, one event for each before the turn's own, and counts it in "discarded".
func TestEndTurnDiscardWritesTheDroppedDamageBeforeTheTurn(t *testing.T) {
	t.Parallel()
	a := newArmed(t)
	e := a.threeAndAGoblin(t)
	a.h.roller.queue(15)
	a.mustAttack(t, a.caio, e, "Toren", battleaxe, "Goblin", inAppRoll)
	start := len(a.eventTrace(t, e, 0))
	if _, err := a.endTurn(t, a.master, e, true); err != nil {
		t.Fatalf("EndTurn(discard) error = %v", err)
	}
	wantTrace(t, a.eventTrace(t, e, start), []string{
		`damage_discarded {"combatant_id": "Toren", "key": "equipment:battleaxe", "pending_id": "?", "prev_status": "awaiting_roll", "round": 1, "target_id": "Goblin"}`,
		`turn_ended {"discarded": 1, "from": "Toren", "round": 1, "to": "Pensantus"}`,
	})
}

// TestEndTurnPassesOverAHiddenTargetsDamageForAPlayer: a damage on a creature the player does not
// see does not hold the player's turn and is not dropped by it (it is the master's to roll).
func TestEndTurnPassesOverAHiddenTargetsDamageForAPlayer(t *testing.T) {
	t.Parallel()
	a := newArmed(t)
	e := a.start(t, plan{
		npcs:     []*playv1.Participant{{CharacterId: a.goblin.GetId()}},
		npcRolls: []int{3},
		players:  map[string]int32{"Toren": 18, "Pensantus": 10, "Brisa": 1},
		reveal:   []string{"Goblin"},
		at:       map[string][2]int32{"Toren": {3, 3}, "Goblin": {4, 3}, "Pensantus": {10, 3}, "Brisa": {8, 8}},
	})
	a.h.roller.queue(15)
	a.mustAttack(t, a.caio, e, "Toren", battleaxe, "Goblin", inAppRoll)
	if _, err := a.master.combat.SetCombatantHidden(t.Context(), connect.NewRequest(&playv1.SetCombatantHiddenRequest{
		CampaignId: a.campaignID, EncounterId: e.GetId(), CombatantId: a.id(t, "Goblin"), IdempotencyKey: newKey(), Hidden: true,
	})); err != nil {
		t.Fatalf("SetCombatantHidden() error = %v", err)
	}
	start := len(a.eventTrace(t, e, 0))
	if _, err := a.endTurn(t, a.caio, e, false); err != nil {
		t.Fatalf("the player's EndTurn with a hidden target's damage waiting: %v", err)
	}
	wantTrace(t, a.eventTrace(t, e, start), []string{`turn_ended {"discarded": 0, "from": "Toren", "round": 1, "to": "Pensantus"}`})
	// The damage is still there for the master: the turn of Toren is over, so it is stale to roll
	// but the pending row was not discarded.
	var open int
	if err := a.h.pool.QueryRow(t.Context(), `SELECT count(*) FROM pending_damages WHERE status <> 'discarded'`).Scan(&open); err != nil {
		t.Fatalf("count the pending damages: %v", err)
	}
	if open != 1 {
		t.Errorf("pending damages not discarded = %d, want 1", open)
	}
}

// TestEndTurnSkipsTheDefeated: a defeated NPC has no turn; the order passes it by, and the round
// turns when the last one standing ends.
func TestEndTurnSkipsTheDefeated(t *testing.T) {
	t.Parallel()
	a := newArmed(t)
	e := a.threeAndAGoblin(t)
	if _, err := a.adjustHP(t, e, "Goblin", damageHP(7)); err != nil {
		t.Fatalf("AdjustCombatantHitPoints() error = %v", err)
	}
	start := len(a.eventTrace(t, e, 0))
	a.mustEndTurn(t, a.caio, e)
	a.mustEndTurn(t, a.ana, e)
	out := a.mustEndTurn(t, a.bia, e) // the goblin is defeated: the turn skips it
	wantTrace(t, a.eventTrace(t, e, start), []string{
		`turn_ended {"discarded": 0, "from": "Toren", "round": 1, "to": "Pensantus"}`,
		`turn_ended {"discarded": 0, "from": "Pensantus", "round": 1, "to": "Brisa"}`,
		`turn_ended {"discarded": 0, "from": "Brisa", "round": 2, "to": "Toren"}`,
	})
	if out.GetRound() != 2 || current(out) != "Toren" {
		t.Errorf("round %d on turn %q, want round 2, Toren", out.GetRound(), current(out))
	}
}

// publishedPerCall reads a stream after n calls and one more (the sentinel, whose own turn_changed
// closes the last): every call publishes the turn first, so the events between two turn_changed are
// the first one's. It returns "encounter,log" and the like, one string for each of the n calls.
func publishedPerCall(t *testing.T, w *watcher, n int) []string {
	t.Helper()
	var out []string
	var cur []string
	turns := 0
	for turns <= n {
		ev := w.nextChange(t)
		switch {
		case ev.GetTurnChanged() != nil:
			if turns > 0 {
				out = append(out, strings.Join(cur, ","))
			}
			cur = nil
			turns++
		case ev.GetEncounterChanged() != nil:
			cur = append(cur, "encounter")
		case ev.GetCombatLogChanged() != nil:
			cur = append(cur, "log")
		}
	}
	return out
}

// TestEndTurnPublishesPerAudience: a pass tells the streams the turn and the combat changed; a
// part that ends without passing, or a damage dropped, also tells them the log changed, and the
// players are not told of a hidden member's part.
func TestEndTurnPublishesPerAudience(t *testing.T) {
	t.Parallel()
	a, e := jointFight(t, true) // Brisa and Toren (one group), Capitao, Pensantus, two hidden goblins
	master, player := a.master.watch(t, a.campaignID), a.ana.watch(t, a.campaignID)
	master.ready(t)
	player.ready(t)
	a.mustEndPart(t, a.bia, e, "Brisa")             // 1. a part ends, the group goes on
	a.mustEndPart(t, a.caio, e, "Toren")            // 2. the last of the group: the turn passes
	a.mustEndPart(t, a.master, e, "Capitão Goblin") // 3. the turn passes to Pensantus
	a.mustEndPart(t, a.ana, e, "Pensantus")         // 4. the turn passes to the hidden goblins (one group)
	a.mustEndPart(t, a.master, e, "Goblin 1")       // 5. a hidden member's part ends: no line for the players
	a.mustEndPart(t, a.master, e, "Goblin 2")       // 6. the turn passes: round 2
	a.mustEndPart(t, a.bia, e, "Brisa")             // the sentinel
	if got, want := publishedPerCall(t, master, 6), []string{"encounter,log", "encounter", "encounter", "encounter", "encounter,log", "encounter"}; !slices.Equal(got, want) {
		t.Errorf("the master's stream: %q, want %q", got, want)
	}
	// The hidden goblin's part is the fifth: the players get the combat change, but no line.
	if got, want := publishedPerCall(t, player, 6), []string{"encounter,log", "encounter", "encounter", "encounter", "encounter", "encounter"}; !slices.Equal(got, want) {
		t.Errorf("the player's stream: %q, want %q", got, want)
	}
}

// TestEndTurnPublishesTheDroppedDamageToTheAudienceThatSeesIt: passing the turn over a damage tells
// the log changed, and the players only when the target is not hidden.
func TestEndTurnPublishesTheDroppedDamageToTheAudienceThatSeesIt(t *testing.T) {
	t.Parallel()
	a := newArmed(t)
	e := a.threeAndAGoblin(t)
	master, player := a.master.watch(t, a.campaignID), a.ana.watch(t, a.campaignID)
	master.ready(t)
	player.ready(t)
	a.h.roller.queue(15)
	a.mustAttack(t, a.caio, e, "Toren", battleaxe, "Goblin", inAppRoll)
	if _, err := a.endTurn(t, a.master, e, true); err != nil { // 1. a visible target: both are told
		t.Fatalf("EndTurn(discard) error = %v", err)
	}
	a.mustEndTurn(t, a.ana, e) // the sentinel: Pensantus passes to Brisa
	if got, want := publishedPerCall(t, master, 1), []string{"encounter,log"}; !slices.Equal(got, want) {
		t.Errorf("the master's stream: %q, want %q", got, want)
	}
	if got, want := publishedPerCall(t, player, 1), []string{"encounter,log"}; !slices.Equal(got, want) {
		t.Errorf("the player's stream: %q, want %q", got, want)
	}
}

// orphan leaves the fight with nobody acting, as it is when the members of the turn left it (the
// last one who could act was removed, or its character was deleted): every turn_state goes idle.
// The combat is still active and its current_combatant_id still names the one who left.
func (a *armed) orphan(t *testing.T) {
	t.Helper()
	if _, err := a.h.pool.Exec(t.Context(), `UPDATE combatants SET turn_state = 'idle'`); err != nil {
		t.Fatalf("leave nobody acting: %v", err)
	}
}

// TestEndTurnOfAnOrphanedTurn: when nobody acts, only the master starts the turns again, from the
// top of the order and in the same round, and only from a screen that still names who was on turn.
func TestEndTurnOfAnOrphanedTurn(t *testing.T) {
	t.Parallel()
	a := newArmed(t)
	e := a.threeAndAGoblin(t)
	a.mustEndTurn(t, a.caio, e) // Pensantus, round 1: the orphan is not at the start of the order
	e = a.get(t, a.master)
	pens := a.id(t, "Pensantus")
	a.orphan(t)
	start := len(a.eventTrace(t, e, 0))

	_, err := a.endTurnAs(t, a.ana, e, func(r *playv1.EndTurnRequest) { r.ExpectedCombatantId = pens })
	wantRefusal(t, "a player", err, connect.CodePermissionDenied, "only the master can start the turns again")
	_, err = a.endTurnAs(t, a.master, e, func(r *playv1.EndTurnRequest) { r.ExpectedCombatantId = a.id(t, "Toren") })
	wantRefusal(t, "the master naming someone else", err, connect.CodeAborted, "that combatant's part of the turn is not the one running")
	if n := len(a.eventTrace(t, e, 0)); n != start {
		t.Fatalf("a refusal wrote %d events", n-start)
	}

	out, err := a.endTurnAs(t, a.master, e, func(r *playv1.EndTurnRequest) { r.ExpectedCombatantId = pens })
	if err != nil {
		t.Fatalf("the master starting the turns again: %v", err)
	}
	if out.GetRound() != 1 || current(out) != "Toren" || group(out) != "Toren" {
		t.Errorf("after the restart: round %d on turn %q group %q, want round 1, Toren", out.GetRound(), current(out), group(out))
	}
	wantTrace(t, a.eventTrace(t, e, start), []string{`turn_ended {"round": 1, "to": "Toren"}`})
}

// TestEndTurnOfAnOrphanedTurnWithoutAnExpectedCombatant: the master's screen may hold no id at
// all, and the restart still works; with no current combatant stored, naming one is stale.
func TestEndTurnOfAnOrphanedTurnWithoutAnExpectedCombatant(t *testing.T) {
	t.Parallel()
	a := newArmed(t)
	e := a.threeAndAGoblin(t)
	a.orphan(t)
	out, err := a.endTurnAs(t, a.master, e, nil)
	if err != nil {
		t.Fatalf("the master starting the turns again without an id: %v", err)
	}
	if out.GetRound() != 1 || current(out) != "Toren" {
		t.Errorf("after the restart: round %d on turn %q, want round 1, Toren", out.GetRound(), current(out))
	}
	a.orphan(t)
	if _, err := a.h.pool.Exec(t.Context(), `UPDATE encounters SET current_combatant_id = NULL`); err != nil {
		t.Fatalf("forget who was on turn: %v", err)
	}
	_, err = a.endTurnAs(t, a.master, e, func(r *playv1.EndTurnRequest) { r.ExpectedCombatantId = a.id(t, "Toren") })
	wantRefusal(t, "naming one when none is stored", err, connect.CodeAborted, "that combatant's part of the turn is not the one running")
}

// TestEndTurnWhenNobodyCanTakeATurn: when every combatant is defeated, an orphaned turn cannot start
// again, and neither can the turn pass from the last one standing.
func TestEndTurnWhenNobodyCanTakeATurn(t *testing.T) {
	t.Parallel()
	a := newArmed(t)
	e := a.threeAndAGoblin(t)
	if _, err := a.h.pool.Exec(t.Context(), `UPDATE combatants SET defeated = true`); err != nil {
		t.Fatalf("defeat everybody: %v", err)
	}
	toren := a.id(t, "Toren")
	// Toren still acts (defeated members keep their turn_state): the turn has nowhere to go.
	_, err := a.endTurnAs(t, a.master, e, func(r *playv1.EndTurnRequest) { r.ExpectedCombatantId = toren })
	wantBlockedBy(t, "passing with nobody to take the turn", err, blockedNotActive)
	wantRefusal(t, "passing with nobody to take the turn", err, connect.CodeFailedPrecondition, "no combatant can take a turn")
	a.orphan(t)
	_, err = a.endTurnAs(t, a.master, e, nil)
	wantBlockedBy(t, "restarting with nobody to take the turn", err, blockedNotActive)
	wantRefusal(t, "restarting with nobody to take the turn", err, connect.CodeFailedPrecondition, "no combatant can take a turn")
}

// TestEndTurnWaitsForTheSavesOfTheEffects: the last member's part ends, the saving throws of the
// effects on it open, and the turn does not pass: the event is a part ended, and another tap
// is refused (not stale) until the save is answered.
func TestEndTurnWaitsForTheSavesOfTheEffects(t *testing.T) {
	t.Parallel()
	a := newCasters(t)
	e := a.holdTheGoblin(t)
	e = a.passTo(t, e, "Goblin")
	goblin := a.id(t, "Goblin")
	start := len(a.eventTrace(t, e, 0))
	player := a.ana.watch(t, a.campaignID)
	player.ready(t)
	if _, err := a.endTurnRaw(t, a.master, e, true); err != nil {
		t.Fatalf("EndTurn() error = %v", err)
	}
	got := a.eventTrace(t, e, start)
	if len(got) != 1 || !strings.HasPrefix(got[0], `turn_part_ended {`) || !strings.Contains(got[0], `"combatant_id": "Goblin"`) {
		t.Errorf("events = %q, want one turn_part_ended by the Goblin", got)
	}
	_, err := a.endTurnAs(t, a.master, e, func(r *playv1.EndTurnRequest) { r.ExpectedCombatantId = goblin })
	wantBlockedBy(t, "a second tap while the save waits", err, playv1.EncounterBlockedReason_ENCOUNTER_BLOCKED_REASON_REACTION_PENDING)
	wantRefusal(t, "a second tap while the save waits", err, connect.CodeFailedPrecondition, "a reaction waits for its answer")
	_, err = a.endTurnAs(t, a.caio, e, nil)
	wantBlockedBy(t, "a player's tap while the save waits", err, playv1.EncounterBlockedReason_ENCOUNTER_BLOCKED_REASON_REACTION_PENDING)
}

// TestEndTurnOfAnNPCGroupIsTheMastersAlone: a part of a group of NPCs with no player in the turn
// writes a part ended that the players get no line for, while the turn passing does tell them.
func TestEndTurnOfAnNPCGroupIsTheMastersAlone(t *testing.T) {
	t.Parallel()
	a, e := jointFight(t, false) // the goblins are revealed
	for _, label := range []string{"Brisa", "Toren"} {
		e = a.mustEndPart(t, a.master, e, label)
	}
	e = a.mustEndPart(t, a.master, e, "Capitão Goblin")
	e = a.mustEndPart(t, a.ana, e, "Pensantus")
	if group(e) != "Goblin 1,Goblin 2" {
		t.Fatalf("group = %q, want the two goblins", group(e))
	}
	master, player := a.master.watch(t, a.campaignID), a.ana.watch(t, a.campaignID)
	master.ready(t)
	player.ready(t)
	start := len(a.eventTrace(t, e, 0))
	a.mustEndPart(t, a.master, e, "Goblin 1") // a part ends, the group goes on
	a.mustEndPart(t, a.master, e, "Goblin 2") // the round turns
	a.mustEndPart(t, a.bia, e, "Brisa")       // the sentinel
	wantTrace(t, a.eventTrace(t, e, start), []string{
		`turn_part_ended {"combatant_id": "Goblin 1", "round": 1, "secret": true}`,
		`turn_ended {"discarded": 0, "from": "Goblin 2", "round": 2, "to": "Brisa"}`,
		`turn_part_ended {"combatant_id": "Brisa", "round": 2}`,
	})
	if got, want := publishedPerCall(t, master, 2), []string{"encounter,log", "encounter"}; !slices.Equal(got, want) {
		t.Errorf("the master's stream: %q, want %q", got, want)
	}
	if got, want := publishedPerCall(t, player, 2), []string{"encounter", "encounter"}; !slices.Equal(got, want) {
		t.Errorf("the player's stream: %q, want %q", got, want)
	}
}

// TestEndTurnDroppedDamageOfAHiddenTargetIsTheMastersLog: the master dropping a damage on a creature
// the players do not see tells the master's log changed and not the players'.
func TestEndTurnDroppedDamageOfAHiddenTargetIsTheMastersLog(t *testing.T) {
	t.Parallel()
	a := newArmed(t)
	e := a.threeAndAGoblin(t)
	a.h.roller.queue(15)
	a.mustAttack(t, a.caio, e, "Toren", battleaxe, "Goblin", inAppRoll)
	if _, err := a.master.combat.SetCombatantHidden(t.Context(), connect.NewRequest(&playv1.SetCombatantHiddenRequest{
		CampaignId: a.campaignID, EncounterId: e.GetId(), CombatantId: a.id(t, "Goblin"), IdempotencyKey: newKey(), Hidden: true,
	})); err != nil {
		t.Fatalf("SetCombatantHidden() error = %v", err)
	}
	master, player := a.master.watch(t, a.campaignID), a.ana.watch(t, a.campaignID)
	master.ready(t)
	player.ready(t)
	if _, err := a.endTurn(t, a.master, e, true); err != nil {
		t.Fatalf("EndTurn(discard) error = %v", err)
	}
	a.mustEndTurn(t, a.ana, e) // the sentinel
	if got, want := publishedPerCall(t, master, 1), []string{"encounter,log"}; !slices.Equal(got, want) {
		t.Errorf("the master's stream: %q, want %q", got, want)
	}
	if got, want := publishedPerCall(t, player, 1), []string{"encounter"}; !slices.Equal(got, want) {
		t.Errorf("the player's stream: %q, want %q", got, want)
	}
}

// Finding fix/endturn-npc-group-line: a group of NPCs alone is the master's (RN-20), so the line
// of its part ending is the master's too, also when the last part ends and the turn waits for the
// saving throw of an effect: neither the player's log nor the player's stream says it.
func TestEndTurnNPCOnlyGroupWaitingOnSavesIsTheMastersLine(t *testing.T) {
	t.Parallel()
	a := newCasters(t)
	e := a.holdTheGoblin(t)
	e = a.passTo(t, e, "Goblin") // a group of one NPC: no player holds the turn
	player := a.caio.watch(t, a.campaignID)
	player.ready(t)
	if _, err := a.endTurnRaw(t, a.master, e, true); err != nil {
		t.Fatalf("EndTurn() error = %v", err)
	}
	if a.windowOf(t, a.master, playv1.ReactionKind_REACTION_KIND_EFFECT_SAVE) == nil {
		t.Fatal("the turn does not wait for the Goblin's save: the case is not the one under test")
	}
	a.master.markCurrentMap(t, a.campaignID, a.mapID, player)
	for _, ev := range player.beforeMarker(t) {
		if ev.GetCombatLogChanged() != nil {
			t.Errorf("a player was told the log changed by an NPC-only group's part: %v", ev)
		}
	}
	partEnded := func(l string) bool { return strings.Contains(l, "Goblin") && strings.Contains(l, "encerrou a parte") }
	if slices.ContainsFunc(lines(a.log(t, a.caio, e)), partEnded) {
		t.Errorf("a player's log has the Goblin's part ending: %v", lines(a.log(t, a.caio, e)))
	}
	if !slices.ContainsFunc(lines(a.log(t, a.master, e)), partEnded) {
		t.Errorf("the master's log has no line for the Goblin's part: %v", lines(a.log(t, a.master, e)))
	}
}

var embeddedJSON = regexp.MustCompile(`"Modifiers": "([A-Za-z0-9+/=]+)"`)

// sortEmbeddedJSON puts in byte order the keys of the jsonb that an event payload carries as base64
// (a []byte field), which PostgreSQL prints in another order than CockroachDB.
func sortEmbeddedJSON(t *testing.T, payload string) string {
	t.Helper()
	return embeddedJSON.ReplaceAllStringFunc(payload, func(m string) string {
		raw, err := base64.StdEncoding.DecodeString(embeddedJSON.FindStringSubmatch(m)[1])
		if err != nil {
			t.Fatalf("decode the Modifiers of an event: %v", err)
		}
		return `"Modifiers": "` + base64.StdEncoding.EncodeToString([]byte(dbtest.SortJSONKeys(string(raw)))) + `"`
	})
}
