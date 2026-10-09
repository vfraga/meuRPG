package combat

import (
	"errors"
	"slices"
	"testing"

	"github.com/PuraFome/meuRPG/backend/internal/rules"
)

func testPlan() *rules.MonsterPlan {
	return &rules.MonsterPlan{
		Key: "monster:wyrm",
		Actions: []rules.ActionPlan{
			{Key: "monster:wyrm#breath", Usage: rules.ActionUsage{Kind: rules.UsageRecharge, RechargeMin: 5}},
			{Key: "monster:wyrm#roar", Usage: rules.ActionUsage{Kind: rules.UsageRecharge, RechargeMin: 6}},
			{Key: "monster:wyrm#gaze", Usage: rules.ActionUsage{Kind: rules.UsagePerDay, Uses: 3}},
			{Key: "monster:wyrm#dive", Usage: rules.ActionUsage{Kind: rules.UsageRest}},
			{Key: "monster:wyrm#bite"},
			{Key: "monster:wyrm#multiattack", Kind: rules.ActionKindMultiattack, Routines: [][]rules.MultiattackStep{
				{{ActionKey: "monster:wyrm#bite", Count: 1}, {ActionKey: "monster:wyrm#claw", Count: 2}},
				{{ActionKey: "monster:wyrm#claw", Count: 3}},
			}},
		},
		Legendary:           &rules.LegendaryPlan{PerRound: 3},
		LegendaryResistance: 3,
	}
}

// TestMonsterActionLimits: a recharging action is unavailable once used, an action with uses
// counts them, and a failed use changes nothing (SRD 5.1, "Monsters": actions).
func TestMonsterActionLimits(t *testing.T) {
	t.Parallel()
	plan := testPlan()
	breath, gaze, dive := plan.Actions[0], plan.Actions[2], plan.Actions[3]

	var s MonsterState
	if err := s.Spend(breath.Usage, breath.Key); err != nil {
		t.Fatalf("first use of a recharging action: %v", err)
	}
	if err := s.Spend(breath.Usage, breath.Key); !errors.Is(err, ErrMonsterRecharging) {
		t.Errorf("second use = %v, want ErrMonsterRecharging", err)
	}
	if len(s.Recharging) != 1 {
		t.Errorf("recharging = %v, want it once (a refused use changes nothing)", s.Recharging)
	}
	for i := range 3 {
		if got := s.UsesLeft(gaze.Usage, gaze.Key); got != 3-i {
			t.Errorf("uses left before use %d = %d, want %d", i+1, got, 3-i)
		}
		if err := s.Spend(gaze.Usage, gaze.Key); err != nil {
			t.Fatalf("use %d of 3/day: %v", i+1, err)
		}
	}
	if err := s.Spend(gaze.Usage, gaze.Key); !errors.Is(err, ErrNoMonsterUses) {
		t.Errorf("fourth use of 3/day = %v, want ErrNoMonsterUses", err)
	}
	if s.Used[gaze.Key] != 3 {
		t.Errorf("used = %d, want 3", s.Used[gaze.Key])
	}
	if err := s.Spend(dive.Usage, dive.Key); err != nil {
		t.Fatalf("the one use of a rest action: %v", err)
	}
	if err := s.Spend(dive.Usage, dive.Key); !errors.Is(err, ErrNoMonsterUses) {
		t.Errorf("second use of a rest action = %v, want ErrNoMonsterUses", err)
	}
	if got := s.UsesLeft(plan.Actions[4].Usage, plan.Actions[4].Key); got != -1 {
		t.Errorf("uses left of an action with no limit = %d, want -1", got)
	}
	// An undo gives the use back.
	s.Refund(gaze.Usage, gaze.Key)
	if got := s.UsesLeft(gaze.Usage, gaze.Key); got != 1 {
		t.Errorf("uses left after a refund = %d, want 1", got)
	}
	s.Refund(breath.Usage, breath.Key)
	if s.IsRecharging(breath.Key) {
		t.Error("a refunded recharging action still waits")
	}
}

// TestMonsterRechargeAtTheStartOfTheTurn: a d6 for each action that waits, at its minimum or
// more it is back ("Recharge 5-6" on a 5 or 6, "Recharge 6" only on a 6); the legendary actions
// are regained and the Multiattack routine is forgotten.
func TestMonsterRechargeAtTheStartOfTheTurn(t *testing.T) {
	t.Parallel()
	plan := testPlan()
	for _, tt := range []struct {
		name          string
		faces         []int
		wantRecharged []bool
		wantWaiting   []string
	}{
		{"both miss", []int{4, 5}, []bool{false, false}, []string{"monster:wyrm#breath", "monster:wyrm#roar"}},
		{"the first comes back on its minimum", []int{5, 5}, []bool{true, false}, []string{"monster:wyrm#roar"}},
		{"a 6 brings both back", []int{6, 6}, []bool{true, true}, nil},
		{"the second needs the 6", []int{1, 6}, []bool{false, true}, []string{"monster:wyrm#breath"}},
	} {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			s := MonsterState{Recharging: []string{"monster:wyrm#breath", "monster:wyrm#roar"}, LegendaryUsed: 2, Multiattack: &MultiattackState{Action: "x"}}
			i := 0
			rolls := s.StartTurn(plan, func() int { i++; return tt.faces[i-1] })
			if len(rolls) != 2 {
				t.Fatalf("rolls = %v, want one for each waiting action", rolls)
			}
			for j, r := range rolls {
				if r.Recharged != tt.wantRecharged[j] || r.Face != tt.faces[j] {
					t.Errorf("roll %d = %+v, want face %d recharged %v", j, r, tt.faces[j], tt.wantRecharged[j])
				}
			}
			if !slices.Equal(s.Recharging, tt.wantWaiting) {
				t.Errorf("waiting = %v, want %v", s.Recharging, tt.wantWaiting)
			}
			if s.LegendaryUsed != 0 || s.Multiattack != nil {
				t.Errorf("legendary used %d, multiattack %v; want both cleared at the creature's own turn", s.LegendaryUsed, s.Multiattack)
			}
		})
	}
	t.Run("an action that is available is not rolled for", func(t *testing.T) {
		t.Parallel()
		var s MonsterState
		if rolls := s.StartTurn(plan, func() int { t.Error("rolled a die with nothing waiting"); return 1 }); len(rolls) != 0 {
			t.Errorf("rolls = %v, want none", rolls)
		}
	})
}

// TestMonsterLegendaryActions: three a round, spent by cost, regained at the creature's own turn
// (SRD 5.1, "Legendary Creatures"); Legendary Resistance counts its uses.
func TestMonsterLegendaryActions(t *testing.T) {
	t.Parallel()
	plan := testPlan()
	var s MonsterState
	if err := s.SpendLegendary(plan, 2); err != nil {
		t.Fatalf("a legendary action of cost 2: %v", err)
	}
	if err := s.SpendLegendary(plan, 2); !errors.Is(err, ErrNoLegendaryActions) {
		t.Errorf("a second one of cost 2 with 1 left = %v, want ErrNoLegendaryActions", err)
	}
	if err := s.SpendLegendary(plan, 1); err != nil {
		t.Fatalf("a legendary action of cost 1: %v", err)
	}
	if got := s.LegendaryLeft(plan); got != 0 {
		t.Errorf("left = %d, want 0", got)
	}
	s.StartTurn(plan, func() int { return 1 })
	if got := s.LegendaryLeft(plan); got != 3 {
		t.Errorf("left after the creature's turn began = %d, want 3", got)
	}
	if got := (MonsterState{}).LegendaryLeft(&rules.MonsterPlan{}); got != 0 {
		t.Errorf("a creature with none has %d", got)
	}
	for range 3 {
		if err := s.SpendResistance(plan); err != nil {
			t.Fatal(err)
		}
	}
	if err := s.SpendResistance(plan); !errors.Is(err, ErrNoLegendaryResistance) {
		t.Errorf("a fourth Legendary Resistance = %v, want ErrNoLegendaryResistance", err)
	}
	// A new day is the master's: Legendary Resistance is not regained at the start of a turn.
	s.StartTurn(plan, func() int { return 1 })
	if got := s.ResistanceLeft(plan); got != 0 {
		t.Errorf("Legendary Resistance left after a turn began = %d, want 0", got)
	}
}

// TestMonsterCanAct: the conditions that leave a creature unable to take actions keep it from
// taking legendary actions (SRD 5.1, Conditions).
func TestMonsterCanAct(t *testing.T) {
	t.Parallel()
	for _, tt := range []struct {
		conditions []string
		want       bool
	}{
		{nil, true},
		{[]string{"condition:poisoned", "condition:prone"}, true},
		{[]string{"condition:frightened"}, true},
		{[]string{"condition:incapacitated"}, false},
		{[]string{"condition:paralyzed"}, false},
		{[]string{"condition:poisoned", "condition:stunned"}, false},
		{[]string{"condition:unconscious"}, false},
		{[]string{"condition:petrified"}, false},
	} {
		if got := CanAct(tt.conditions); got != tt.want {
			t.Errorf("CanAct(%v) = %v, want %v", tt.conditions, got, tt.want)
		}
	}
}

// TestMonsterMultiattackRoutine: the routine chosen is counted down as its attacks are made.
func TestMonsterMultiattackRoutine(t *testing.T) {
	t.Parallel()
	plan := testPlan()
	multi := plan.Actions[5]
	var s MonsterState
	if err := s.StartMultiattack(multi, 2); !errors.Is(err, ErrNotARoutine) {
		t.Errorf("routine 2 of 2 = %v, want ErrNotARoutine", err)
	}
	if err := s.StartMultiattack(multi, -1); !errors.Is(err, ErrNotARoutine) {
		t.Errorf("routine -1 = %v, want ErrNotARoutine", err)
	}
	if err := s.StartMultiattack(multi, 0); err != nil {
		t.Fatal(err)
	}
	if !slices.Equal(s.Multiattack.Left, []int{1, 2}) {
		t.Fatalf("left = %v, want [1 2]", s.Multiattack.Left)
	}
	firstClaw, secondClaw := s.CountAttack(plan, "monster:wyrm#claw"), s.CountAttack(plan, "monster:wyrm#claw")
	if !firstClaw || !secondClaw {
		t.Error("the two claws of the routine were not counted")
	}
	if s.CountAttack(plan, "monster:wyrm#claw") {
		t.Error("a third claw is outside the routine and must not be counted")
	}
	if !s.CountAttack(plan, "monster:wyrm#bite") || s.CountAttack(plan, "monster:wyrm#bite") {
		t.Error("the bite counts once")
	}
	if !slices.Equal(s.Multiattack.Left, []int{0, 0}) {
		t.Errorf("left = %v, want [0 0]", s.Multiattack.Left)
	}
	var none MonsterState
	if none.CountAttack(plan, "monster:wyrm#bite") {
		t.Error("an attack counted with no routine chosen")
	}
}

// TestMonsterSlotsAndClone: the slots spent by level, and a clone that does not share its maps.
func TestMonsterSlotsAndClone(t *testing.T) {
	t.Parallel()
	var s MonsterState
	s.SpendSlot(3)
	s.SpendSlot(3)
	s.SpendSlot(1)
	if got := s.SlotLeft(3, 3); got != 1 {
		t.Errorf("3rd-level slots left = %d, want 1", got)
	}
	if got := s.SlotLeft(9, 1); got != 1 {
		t.Errorf("9th-level slots left = %d, want 1 (none spent)", got)
	}
	if got := s.SlotLeft(0, 1); got != 0 {
		t.Errorf("slot level 0 = %d, want 0", got)
	}
	if got := s.SlotsUsedArray(); got != [9]int{1, 0, 2} {
		t.Errorf("array = %v", got)
	}
	s.RefundSlot(3)
	if got := s.SlotLeft(3, 3); got != 2 {
		t.Errorf("after a refund = %d, want 2", got)
	}
	s.Used = map[string]int{"a": 1}
	c := s.Clone()
	c.Used["a"] = 5
	c.SlotsUsed[0] = 9
	if s.Used["a"] != 1 || s.SlotsUsed[0] != 1 {
		t.Errorf("clone shares with the original: %v %v", s.Used, s.SlotsUsed)
	}
}

// TestRecharged: the d6 against the minimum.
func TestRecharged(t *testing.T) {
	t.Parallel()
	u := rules.ActionUsage{Kind: rules.UsageRecharge, RechargeMin: 4}
	for face, want := range map[int]bool{1: false, 3: false, 4: true, 5: true, 6: true} {
		if got := Recharged(u, face); got != want {
			t.Errorf("Recharged(4-6, %d) = %v, want %v", face, got, want)
		}
	}
	if Recharged(rules.ActionUsage{Kind: rules.UsagePerDay, Uses: 3}, 6) {
		t.Error("a per-day action recharges")
	}
}
