package play

import (
	"bytes"
	"fmt"
	"log/slog"
	"strings"
	"testing"

	"connectrpc.com/connect"

	mapsv1 "github.com/PuraFome/meuRPG/backend/gen/meurpg/maps/v1"
	playv1 "github.com/PuraFome/meuRPG/backend/gen/meurpg/play/v1"
	rulesv1 "github.com/PuraFome/meuRPG/backend/gen/meurpg/rules/v1"
)

// Finding U4-12: fireInCombat (combat_traps.go) builds ONE trapFireEvent with every
// caught creature's save, damages and conditions, and insertEvent has no size guard,
// while session_events.payload has CHECK octet_length(payload::TEXT) <= 4096
// (migration 00024). maxTrapTargets is 40, so a trap that catches 8 or 9 creatures
// (Dex save, 2d6 on a fail, poisoned) answers `internal`, and the same happens from
// MoveCombatant when the mover enters an area holding many creatures. The expected
// answer is success or a typed refusal, never internal.
func manyTrapEffect() func(*mapsv1.TrapSpec) {
	return func(s *mapsv1.TrapSpec) {
		s.Trigger = rulesv1.TrapTrigger_TRAP_TRIGGER_MANUAL
		s.Effect = &rulesv1.TrapEffect{
			Save: &rulesv1.TrapSaveEffect{
				Ability: rulesv1.Ability_ABILITY_DEXTERITY, Dc: 12, AppliesTo: rulesv1.TrapSaveApplies_TRAP_SAVE_APPLIES_CAUGHT,
				OnFail: &rulesv1.TrapOnFail{
					Damage:    []*rulesv1.TrapDamage{{Dice: "2d6", DamageTypeKey: "damage-type:fire"}},
					Condition: &rulesv1.TrapCondition{ConditionKey: "condition:poisoned"},
				},
				OnPass: rulesv1.TrapPassOutcome_TRAP_PASS_OUTCOME_HALF,
			},
			Targets: rulesv1.TrapTargets_TRAP_TARGETS_MANUAL,
		}
	}
}

func TestReview4_TrapFiringOnManyCreaturesOverflowsEventPayload(t *testing.T) {
	t.Parallel()
	r := newTrapRig(t)
	const goblins = 10
	r.start(t, plan{
		npcs:     []*playv1.Participant{{CharacterId: r.goblins.GetId(), Count: int32(goblins)}},
		npcRolls: []int{2},
		players:  map[string]int32{"Toren": 18, "Pensantus": 10, "Brisa": 1},
	})
	label := func(i int) string { return fmt.Sprintf("Goblin %d", i) }
	var logs bytes.Buffer
	r.h.svc.logger = slog.New(slog.NewTextHandler(&logs, nil))

	var firstBad int
	var retry *mapsv1.MapPoint
	for n := 1; n <= goblins; n++ {
		p := r.trap(t, fmt.Sprintf("Estátua %d", n), 15, 12, manyTrapEffect())
		var ids []string
		for i := 1; i <= n; i++ {
			ids = append(ids, r.id(t, label(i)))
		}
		_, err := r.fireByHand(t, p, ids...)
		if err != nil {
			t.Errorf("FireTrap on %d creatures: error = %v (code %v), want success or a typed refusal, never internal", n, err, connect.CodeOf(err))
			if firstBad == 0 {
				firstBad = n
				retry = p
			}
			if connect.CodeOf(err) != connect.CodeInternal {
				t.Logf("typed refusal at %d: %v", n, err)
			}
			break
		}
	}
	if firstBad == 0 {
		return
	}
	for _, line := range strings.Split(logs.String(), "\n") {
		if strings.Contains(line, "4096") || strings.Contains(line, "23514") {
			t.Logf("server log: %s", line)
		}
	}
	t.Logf("smallest failing number of caught creatures: %d", firstBad)
	// Is the trap left armed, and does a master retry with fewer targets recover?
	if got := r.point(t, r.master, retry.GetId()).GetTrap().GetState(); got != mapsv1.TrapState_TRAP_STATE_ARMED {
		t.Errorf("the trap after the failed firing = %v, want armed", got)
	}
	if _, err := r.fireByHand(t, retry, r.id(t, label(1)), r.id(t, label(2))); err != nil {
		t.Errorf("retry with 2 targets: %v", err)
	} else {
		t.Logf("retry with 2 targets succeeded: the failed attempt left the trap armed")
	}
}

// Finding U4-12 (move path): a player's move into an area trap holding many creatures.
func TestReview4_TrapFiringOnManyCreaturesOverflowsEventPayload_Move(t *testing.T) {
	t.Parallel()
	for _, n := range []int{3, 10} {
		t.Run(fmt.Sprintf("%d creatures", n), func(t *testing.T) { moveInto(t, n) })
	}
}

func moveInto(t *testing.T, goblins int) {
	t.Helper()
	r := newTrapRig(t)
	at := map[string][2]int32{"Toren": {6, 7}, "Pensantus": {5, 8}, "Brisa": {4, 7}}
	for i := 1; i <= goblins; i++ {
		at[fmt.Sprintf("Goblin %d", i)] = [2]int32{int32(9 + (i-1)%4), int32(7 + (i-1)/4)} //nolint:gosec // small
	}
	r.trap(t, "Estátua", 9, 7, func(s *mapsv1.TrapSpec) {
		manyTrapEffect()(s)
		s.Trigger = rulesv1.TrapTrigger_TRAP_TRIGGER_ENTER
		s.AreaSize = 4
		s.Effect.Targets = rulesv1.TrapTargets_TRAP_TARGETS_UNSPECIFIED
	})
	r.start(t, plan{
		npcs:     []*playv1.Participant{{CharacterId: r.goblins.GetId(), Count: int32(goblins)}}, //nolint:gosec // small
		npcRolls: []int{2},
		players:  map[string]int32{"Toren": 18, "Pensantus": 10, "Brisa": 1},
		at:       at,
	})
	res, err := r.moveResult(t, r.caio, "Toren", 12, 7)
	if err != nil {
		t.Fatalf("MoveCombatant into a trap area with %d creatures: error = %v (code %v); the move is lost", goblins, err, connect.CodeOf(err))
	}
	t.Logf("move ok, stopped early %v", res.GetStoppedEarly())
}
