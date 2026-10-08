package play

import (
	"context"
	"encoding/json"
	"log/slog"
	"slices"
	"sync/atomic"
	"testing"
	"time"

	"connectrpc.com/connect"

	playv1 "github.com/PuraFome/meuRPG/backend/gen/meurpg/play/v1"
	"github.com/PuraFome/meuRPG/backend/internal/play/playdb"
	"github.com/PuraFome/meuRPG/backend/internal/rules/grid"
)

// These tests need no database: the undo's search, the log's building and
// who it shows what are plain functions.

func payload(t *testing.T, ev actionEvent) []byte {
	t.Helper()
	raw, err := json.Marshal(ev)
	if err != nil {
		t.Fatalf("Marshal() error = %v", err)
	}
	return raw
}

func recent(t *testing.T, enc string, rows ...[3]string) []playdb.ListRecentSessionEventsRow {
	t.Helper() // each row: id, kind, undone id (for an undo)
	out := make([]playdb.ListRecentSessionEventsRow, len(rows))
	for i, r := range rows {
		out[i] = playdb.ListRecentSessionEventsRow{ID: r[0], Kind: r[1], EncounterID: &enc, Payload: payload(t, actionEvent{Undone: r[2]})}
	}
	return out
}

// TestLastActionSkipsWhatAnUndoTookBack: the search walks from the newest event,
// skipping each undo and the event it took back, and stops at the first one
// left: an action is the last action only if nothing but undone events
// follows it.
func TestLastActionSkipsWhatAnUndoTookBack(t *testing.T) {
	t.Parallel()
	const enc = "enc"
	tests := []struct {
		name string
		rows [][3]string // newest first
		want string
	}{
		{"the newest action", [][3]string{{"c", eventDamageRolled}, {"b", eventAttackRolled}}, "c"},
		{"one undo back", [][3]string{{"u", eventActionUndone, "c"}, {"c", eventDamageRolled}, {"b", eventAttackRolled}}, "b"},
		{"two undos back", [][3]string{{"u2", eventActionUndone, "b"}, {"u1", eventActionUndone, "c"}, {"c", eventDamageRolled}, {"b", eventAttackRolled}, {"a", eventActionTaken}}, "a"},
		{"everything undone", [][3]string{{"u", eventActionUndone, "b"}, {"b", eventAttackRolled}}, ""},
		{"a turn passed after it", [][3]string{{"t", eventTurnEnded}, {"b", eventAttackRolled}}, ""},
		{"a correction of the vitals after it", [][3]string{{"v", eventCharacterVitalsAdjusted}, {"b", eventDamageApplied}}, ""},
		{"nothing at all", nil, ""},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			got, ok := lastAction(recent(t, enc, tt.rows...), enc)
			switch {
			case tt.want == "" && ok:
				t.Errorf("lastAction() = %q, want none", got.ID)
			case tt.want != "" && (!ok || got.ID != tt.want):
				t.Errorf("lastAction() = %q (%v), want %q", got.ID, ok, tt.want)
			}
		})
	}
	// An action of another combat is not this one's to undo.
	if _, ok := lastAction(recent(t, "other", [3]string{"b", eventAttackRolled, ""}), enc); ok {
		t.Error("lastAction() found an action of another combat")
	}
}

func eventRow(t *testing.T, id, kind string, ev actionEvent) playdb.ListEncounterEventsRow {
	t.Helper()
	return playdb.ListEncounterEventsRow{ID: id, Kind: kind, Payload: payload(t, ev), CreatedAt: time.Unix(0, 0)}
}

// TestBuildLogMergesTheDamageIntoTheAttack: the roll, the damage and the master's
// apply of one attack are one entry; an undone event leaves nothing; the
// setup (round 0) and the turns are not lines of the log.
func TestBuildLogMergesTheDamageIntoTheAttack(t *testing.T) {
	t.Parallel()
	events := []playdb.ListEncounterEventsRow{
		eventRow(t, "setup", eventCombatantMoved, actionEvent{OnTurn: false, DistanceFt: 15}),
		eventRow(t, "begun", eventCombatBegun, actionEvent{Round: 1}),
		eventRow(t, "turn", eventTurnEnded, actionEvent{Round: 1}),
		eventRow(t, "a1", eventAttackRolled, actionEvent{Round: 1, Actor: "cap", Target: "tor", Key: "basic:0", Outcome: outcomeHit, Pending: "p1", D20: 15, Total: 19}),
		eventRow(t, "d1", eventDamageRolled, actionEvent{Round: 1, Actor: "cap", Target: "tor", Pending: "p1", Amount: 5, DamageType: "damage-type:slashing", DiceCount: 1, DiceSides: 6}),
		eventRow(t, "x1", eventDamageApplied, actionEvent{Round: 1, Pending: "p1", Amount: 5, After: &hpState{HP: 26}}),
		eventRow(t, "a2", eventAttackRolled, actionEvent{Round: 1, Actor: "tor", Target: "cap", Key: "e", Outcome: outcomeMiss}),
		eventRow(t, "u", eventActionUndone, actionEvent{Round: 1, Undone: "a2"}),
		eventRow(t, "a3", eventAttackRolled, actionEvent{Round: 2, Actor: "tor", Target: "cap", Key: "e", Outcome: outcomeCrit, Pending: "p3"}),
		eventRow(t, "end", eventEncounterEnded, actionEvent{Round: 2}),
	}
	got := buildLog(events)
	var ids []string
	for _, e := range got {
		ids = append(ids, e.id)
	}
	if want := []string{"begun", "a1", "a3", "end"}; !slices.Equal(ids, want) {
		t.Fatalf("entries = %v, want %v (the setup move, the turn and the undone attack are not in the log)", ids, want)
	}
	attack := got[1]
	if attack.status != playv1.PendingDamageStatus_PENDING_DAMAGE_STATUS_APPLIED || attack.dmg == nil || attack.dmg.Amount != 5 || attack.dmg.After.HP != 26 {
		t.Errorf("the attack's entry = %+v, want the damage applied, 5, Toren at 26", attack)
	}
	if want := []string{"a1", "d1", "x1"}; !slices.Equal(attack.hosts, want) {
		t.Errorf("events of the attack = %v, want %v (the undo of the last one finds its entry)", attack.hosts, want)
	}
	if got[2].status != playv1.PendingDamageStatus_PENDING_DAMAGE_STATUS_AWAITING_ROLL || got[2].dmg != nil {
		t.Errorf("a hit whose damage was not rolled = %+v, want it waiting", got[2])
	}
}

func TestTargetsAndReach(t *testing.T) {
	t.Parallel()
	at := func(id string, col, row int32, hidden, defeated bool) playdb.Combatant {
		return playdb.Combatant{ID: id, Label: id, Kind: kindNPC, Hidden: hidden, Defeated: defeated, GridCol: &col, GridRow: &row}
	}
	attacker := at("a", 0, 0, false, false)
	cs := []playdb.Combatant{
		attacker, at("near", 1, 1, false, false), at("far", 7, 0, false, false), at("secret", 1, 0, true, false), at("dead", 2, 0, false, true),
		{ID: "lost", Label: "lost", Kind: kindNPC}, // not on the grid
	}
	open := grid.Terrain{Grid: grid.Grid{Columns: 20, Rows: 10}}
	reach := reachFt(0, 0)
	if reach != 5 {
		t.Fatalf("reachFt(0, 0) = %d, want a melee reach of 5", reach)
	}
	if got := reachFt(80, 320); got != 320 {
		t.Errorf("reachFt(80, 320) = %d, want the long range", got)
	}

	player := targetsFor(open, cs, attacker, combatViewer{userID: "u"}, reach, false)
	var ids []string
	for _, tg := range player {
		ids = append(ids, tg.GetCombatantId())
	}
	if want := []string{"near", "far", "lost"}; !slices.Equal(ids, want) {
		t.Fatalf("a player's targets = %v, want %v (no hidden, no defeated, no self)", ids, want)
	}
	if player[0].GetDistanceFt() != 5 || player[0].GetTooFar() || player[1].GetDistanceFt() != 35 || !player[1].GetTooFar() {
		t.Errorf("distances = %v, %v; want 5 ft in reach and 35 ft too far", player[0], player[1])
	}
	if player[2].DistanceFt != nil || !player[2].GetTooFar() {
		t.Errorf("a target with no square for a player = %v, want no distance and too far", player[2])
	}
	master := targetsFor(open, cs, attacker, combatViewer{master: true}, reach, false)
	if len(master) != 4 {
		t.Fatalf("the master's targets = %v, want the hidden one too", master)
	}
	if lost := master[len(master)-1]; lost.GetTooFar() {
		t.Errorf("an unplaced target for the master = %v, want it not too far (the master is not held to the reach)", lost)
	}
}

// cancelOnEvent cancels the client's call when the DEBUG event line of name is
// logged (after the commit, before finish), and waits for the server's request
// context to notice.
type cancelOnEvent struct {
	name   string
	armed  *atomic.Bool
	cancel *atomic.Pointer[context.CancelFunc]
}

func (h cancelOnEvent) Enabled(context.Context, slog.Level) bool { return true }
func (h cancelOnEvent) WithAttrs([]slog.Attr) slog.Handler       { return h }
func (h cancelOnEvent) WithGroup(string) slog.Handler            { return h }
func (h cancelOnEvent) Handle(ctx context.Context, r slog.Record) error {
	if !h.armed.Load() {
		return nil
	}
	r.Attrs(func(a slog.Attr) bool {
		if a.Key == "event" && a.Value.String() == h.name {
			if c := h.cancel.Load(); c != nil {
				(*c)()
			}
			select {
			case <-ctx.Done():
			case <-time.After(5 * time.Second):
			}
			return false
		}
		return true
	})
	return nil
}

// A turn passed and committed, whose caller hangs up before the streams are told, is announced
// anyway, and so is the retry with the same key that finds it done.
func TestACommittedChangeIsAnnouncedEvenIfTheCallerLeaves(t *testing.T) {
	t.Parallel()
	a := newArmed(t)
	var armedFlag atomic.Bool
	var cancel atomic.Pointer[context.CancelFunc]
	a.h.svc.logger = slog.New(cancelOnEvent{name: "combat.turn_passed", armed: &armedFlag, cancel: &cancel}) // before any stream starts
	e := a.start(t, plan{
		npcs:     []*playv1.Participant{{CharacterId: a.goblin.GetId(), Hidden: new(false)}},
		npcRolls: []int{20},
		players:  map[string]int32{"Toren": 10, "Pensantus": 5, "Brisa": 1},
		at:       map[string][2]int32{"Toren": {3, 3}, "Goblin": {4, 3}, "Pensantus": {10, 3}, "Brisa": {8, 8}},
	})
	masterStream, playerStream := a.master.watch(t, a.campaignID), a.ana.watch(t, a.campaignID)
	masterStream.ready(t)
	playerStream.ready(t)

	cur := a.get(t, a.master).GetCurrentCombatantId()
	req := func() *connect.Request[playv1.EndTurnRequest] {
		return connect.NewRequest(&playv1.EndTurnRequest{CampaignId: a.campaignID, EncounterId: e.GetId(), IdempotencyKey: newKey(), ExpectedCombatantId: cur})
	}
	first := req()
	retry := connect.NewRequest(&playv1.EndTurnRequest{CampaignId: a.campaignID, EncounterId: e.GetId(), IdempotencyKey: first.Msg.GetIdempotencyKey(), ExpectedCombatantId: cur})
	ctx, c := context.WithCancel(t.Context())
	cancel.Store(&c)
	armedFlag.Store(true)
	if _, err := a.master.combat.EndTurn(ctx, first); err == nil {
		t.Fatal("first EndTurn: want a context error, got none (the hook did not cancel)")
	}
	armedFlag.Store(false)
	// The turn was committed: the retry answers.
	if _, err := a.master.combat.EndTurn(t.Context(), retry); err != nil {
		t.Fatalf("retry EndTurn error = %v, want the answer of the committed change", err)
	}
	for name, w := range map[string]*watcher{"master": masterStream, "player": playerStream} {
		select {
		case <-w.events:
		case <-time.After(2 * time.Second):
			t.Errorf("%s's stream got no event for the committed turn change", name)
		}
	}
}
