package play

import (
	"fmt"
	"testing"

	"connectrpc.com/connect"

	mapsv1 "github.com/PuraFome/meuRPG/backend/gen/meurpg/maps/v1"
	playv1 "github.com/PuraFome/meuRPG/backend/gen/meurpg/play/v1"
	rulesv1 "github.com/PuraFome/meuRPG/backend/gen/meurpg/rules/v1"
)

// manyTrapEffect is a trap that asks a Dexterity save of each creature it catches and, on a fail,
// burns and poisons it: the most a firing writes for each creature.
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

// A trap fired by hand on more creatures than one event holds is written as several events:
// the firing answers with every creature, the log has it as one entry, and one undo takes the
// whole firing back and arms the trap again.
func TestATrapFiringOnManyCreaturesIsWrittenInParts(t *testing.T) {
	t.Parallel()
	r := newTrapRig(t)
	const goblins = 10
	r.start(t, plan{
		npcs:     []*playv1.Participant{{CharacterId: r.goblins.GetId(), Count: int32(goblins)}},
		npcRolls: []int{2},
		players:  map[string]int32{"Toren": 18, "Pensantus": 10, "Brisa": 1},
	})
	p := r.trap(t, "Estátua", 15, 12, manyTrapEffect())
	var ids []string
	for i := 1; i <= goblins; i++ {
		ids = append(ids, r.id(t, fmt.Sprintf("Goblin %d", i)))
	}
	res, err := r.fireByHand(t, p, ids...)
	if err != nil {
		t.Fatalf("FireTrap on %d creatures: error = %v (code %v)", goblins, err, connect.CodeOf(err))
	}
	if got := len(res.GetFiring().GetCaught()); got != goblins {
		t.Errorf("the firing answers with %d creatures, want %d", got, goblins)
	}
	if got := r.eventCount(t, eventTrapTriggered); got < 2 {
		t.Errorf("the firing is %d event, want it written in parts", got)
	}
	e := r.get(t, r.master)
	firingsOf := func(e *playv1.Encounter) []*playv1.TrapFiring {
		var out []*playv1.TrapFiring
		for _, round := range r.log(t, r.master, e).GetRounds() {
			for _, en := range round.GetEntries() {
				if en.GetTrap() != nil {
					out = append(out, en.GetTrap())
				}
			}
		}
		return out
	}
	firings := firingsOf(e)
	if len(firings) != 1 || len(firings[0].GetCaught()) != goblins {
		t.Errorf("the log has %d firing entries, want one with %d creatures", len(firings), goblins)
	}
	if got := r.point(t, r.master, p.GetId()).GetTrap().GetState(); got != mapsv1.TrapState_TRAP_STATE_TRIGGERED {
		t.Errorf("the trap after the firing = %v, want triggered", got)
	}

	r.undoLast(t)
	if got := r.point(t, r.master, p.GetId()).GetTrap().GetState(); got != mapsv1.TrapState_TRAP_STATE_ARMED {
		t.Errorf("the trap after the undo = %v, want armed", got)
	}
	for _, f := range firingsOf(r.get(t, r.master)) {
		t.Errorf("the log still has the firing after one undo: %v", f)
	}
	for _, d := range r.trapDamages(t) {
		t.Errorf("a trap damage is left after the undo: %v", d)
	}
}

// A player's move into an area trap holding many creatures is not lost: the trap fires on all of them.
func TestAMoveIntoATrapAreaWithManyCreaturesFiresIt(t *testing.T) {
	t.Parallel()
	for _, n := range []int{3, 10} {
		t.Run(fmt.Sprintf("%d creatures", n), func(t *testing.T) {
			t.Parallel()
			moveInto(t, n)
		})
	}
}

func moveInto(t *testing.T, goblins int) {
	t.Helper()
	r := newTrapRig(t)
	at := map[string][2]int32{"Toren": {6, 7}, "Pensantus": {5, 8}, "Brisa": {4, 7}}
	for i := 1; i <= goblins; i++ {
		at[fmt.Sprintf("Goblin %d", i)] = [2]int32{int32(9 + (i-1)%4), int32(7 + (i-1)/4)}
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
