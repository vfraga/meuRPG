package play

import (
	"slices"
	"testing"

	"connectrpc.com/connect"

	campaignsv1 "github.com/PuraFome/meuRPG/backend/gen/meurpg/campaigns/v1"
	playv1 "github.com/PuraFome/meuRPG/backend/gen/meurpg/play/v1"
)

// The release of a readied action at a table with physical dice takes the dice a cast or an attack takes (#938): the
// answer's fields are copied one to one into the request the release builds.

func useFaces(roll func(*playv1.AnswerReactionRequest), faces ...int32) func(*playv1.AnswerReactionRequest) {
	return func(r *playv1.AnswerReactionRequest) {
		useSpellAnswer(r)
		if roll != nil {
			roll(r)
		}
		r.D20Faces = faces
	}
}

func withMode(mode playv1.RollMode, reason string, faces ...int32) func(*playv1.AnswerReactionRequest) {
	return func(r *playv1.AnswerReactionRequest) {
		useSpellAnswer(r)
		r.D20Faces, r.RollMode, r.ModeReason = faces, mode, reason
	}
}

func withPool(sum int32) func(*playv1.AnswerReactionRequest) {
	return func(r *playv1.AnswerReactionRequest) {
		useSpellAnswer(r)
		r.Roll = &playv1.AnswerReactionRequest_PoolSum{PoolSum: sum}
	}
}

func (a *armed) readiedAttackLine(t *testing.T, e *playv1.Encounter) *playv1.CombatLogEntry {
	t.Helper()
	for _, r := range a.log(t, a.master, e).GetRounds() {
		for _, en := range r.GetEntries() {
			if en.GetKind() == playv1.CombatLogKind_COMBAT_LOG_KIND_ATTACK {
				return en
			}
		}
	}
	return nil
}

// wantFaces checks the d20 faces the attack line of the log shows.
func (a *armed) wantFaces(t *testing.T, e *playv1.Encounter, want []int32) {
	t.Helper()
	line := a.readiedAttackLine(t, e)
	if line == nil {
		t.Fatalf("no attack line in the log")
	}
	if got := line.GetAttackRoll().GetFaces(); !slices.Equal(got, want) {
		t.Errorf("the attack's d20 faces = %v, want %v", got, want)
	}
}

func TestAReadiedWeaponAttackTakesTypedDiceAtAPhysicalTable(t *testing.T) {
	t.Parallel()
	cases := []struct {
		name string
		by   func(a *armed) *user
		set  func(*playv1.AnswerReactionRequest)
		want []int32
	}{
		{"the player, the old typed field", func(a *armed) *user { return a.caio }, useTyped(15), []int32{15}},
		{"the player, a disadvantage pair", func(a *armed) *user { return a.caio }, withMode(playv1.RollMode_ROLL_MODE_DISADVANTAGE, "no escuro", 15, 4), []int32{15, 4}},
		{"the master, a single face in d20_faces", func(a *armed) *user { return a.master }, useFaces(nil, 15), []int32{15}},
		{"the master, an advantage pair", func(a *armed) *user { return a.master }, withMode(playv1.RollMode_ROLL_MODE_ADVANTAGE, "flanco", 15, 4), []int32{15, 4}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			a := newArmed(t)
			a.forceDice(t, campaignsv1.DiceMode_DICE_MODE_PHYSICAL)
			e := a.readyThenPass(t, nil)
			w := a.mustFire(t, e, "Toren")
			res := a.mustAnswer(t, tc.by(a), e, w.GetId(), tc.set)
			if !res.GetResult().GetUsed() {
				t.Fatalf("the use's result = %v", res.GetResult())
			}
			if a.readiedOpen(t, "Toren") || a.readiedStatus(t, "Toren") != readiedFired {
				t.Errorf("window open %v, row %q, want fired", a.readiedOpen(t, "Toren"), a.readiedStatus(t, "Toren"))
			}
			a.wantFaces(t, e, tc.want)
		})
	}
}

func TestAReadiedWeaponAttackRefusesTheAppsDiceAtAPhysicalTable(t *testing.T) {
	t.Parallel()
	a := newArmed(t)
	a.forceDice(t, campaignsv1.DiceMode_DICE_MODE_PHYSICAL)
	e := a.readyThenPass(t, nil)
	w := a.mustFire(t, e, "Toren")
	_, err := a.answerReaction(t, a.caio, e, w.GetId(), useSpellAnswer)
	wantCode(t, "app dice at a physical table", err, connect.CodeFailedPrecondition)
}

func TestAReadiedWeaponAttackInAppIsUnchanged(t *testing.T) {
	t.Parallel()
	a := newArmed(t)
	a.forceDice(t, campaignsv1.DiceMode_DICE_MODE_APP)
	e := a.readyThenPass(t, nil)
	w := a.mustFire(t, e, "Toren")
	a.mustAnswer(t, a.caio, e, w.GetId(), useSpellAnswer)
	if a.readiedStatus(t, "Toren") != readiedFired {
		t.Errorf("row %q, want fired", a.readiedStatus(t, "Toren"))
	}
}

func TestAReadiedSpellAttackTakesTypedFaces(t *testing.T) {
	t.Parallel()
	a := newCasters(t)
	a.forceDice(t, campaignsv1.DiceMode_DICE_MODE_PHYSICAL)
	e := a.readyForRelease(t, fireBolt, 0, "Goblin")
	w := a.mustFire(t, e, "Pensantus")
	res := a.mustAnswer(t, a.ana, e, w.GetId(), withMode(playv1.RollMode_ROLL_MODE_DISADVANTAGE, "no escuro", 12, 3))
	if !res.GetResult().GetUsed() {
		t.Fatalf("the use's result = %v", res.GetResult())
	}
	a.wantFaces(t, e, []int32{12, 3})
}

// Sleep at its first slot rolls 5d8. The typed sum is the one resolved: 8 covers the goblin (7 hit points) and not the Capitão
// (27), so exactly the goblin falls asleep, and the log marks the roll as physical. A sum outside 5 to 40 is refused on the
// release, the player's or the master's, with nothing spent and the window open.
func (a *armed) readiedSleep(t *testing.T) (*playv1.Encounter, *playv1.ReactionWindow) {
	t.Helper()
	a.forceDice(t, campaignsv1.DiceMode_DICE_MODE_PHYSICAL)
	e := a.start(t, plan{
		npcs:     []*playv1.Participant{{CharacterId: a.goblin.GetId()}, {CharacterId: a.capitao.GetId()}},
		npcRolls: []int{1, 1}, reveal: []string{"Goblin", "Capitão Goblin"}, theatre: true,
		players: map[string]int32{"Pensantus": 20, "Toren": 15, "Brisa": 10},
	})
	a.mustReady(t, a.ana, a.readySpell(t, e, "Pensantus", sleepSpell, 1, "Capitão Goblin"))
	e = a.mustEndTurn(t, a.master, e)
	return e, a.mustFire(t, e, "Pensantus")
}

func TestAReadiedPoolSpellTakesTheTypedSum(t *testing.T) {
	t.Parallel()
	for name, by := range map[string]func(a *armed) *user{
		"the player": func(a *armed) *user { return a.ana },
		"the master": func(a *armed) *user { return a.master },
	} {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			a := newCasters(t)
			e, w := a.readiedSleep(t)
			res := a.mustAnswer(t, by(a), e, w.GetId(), func(r *playv1.AnswerReactionRequest) {
				withPool(8)(r)
				r.Targets = a.at(t, "Capitão Goblin", "Goblin")
			})
			if !res.GetResult().GetUsed() {
				t.Fatalf("the use's result = %v", res.GetResult())
			}
			now := a.get(t, a.master)
			if !hasCondition(now, t, "Goblin", unconsciousC) || hasCondition(now, t, "Capitão Goblin", unconsciousC) {
				t.Errorf("asleep: Goblin %v, Capitão %v; want only the goblin, whom the typed 8 covers and not the Capitão's 27",
					hasCondition(now, t, "Goblin", unconsciousC), hasCondition(now, t, "Capitão Goblin", unconsciousC))
			}
			line := a.castLine(t, e, sleepSpell)
			if line == nil {
				t.Fatalf("no Sleep line in the log")
			}
			if roll := line.GetSpell().GetPoolRoll(); roll.GetTotal() != 8 || !roll.GetPhysical() || roll.GetDiceCount() != 5 || roll.GetDiceSides() != 8 {
				t.Errorf("the log's pool roll = %v, want the typed 8 of 5d8 marked as physical", roll)
			}
		})
	}
}

func TestAReadiedPoolSpellRefusesATypedSumOutOfRange(t *testing.T) {
	t.Parallel()
	for name, sum := range map[string]int32{"below the dice count": 4, "above the dice count times the sides": 41} {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			a := newCasters(t)
			e, w := a.readiedSleep(t)
			before := a.stateSnapshot(t)
			_, err := a.answerReaction(t, a.ana, e, w.GetId(), func(r *playv1.AnswerReactionRequest) {
				withPool(sum)(r)
				r.Targets = a.at(t, "Capitão Goblin", "Goblin")
			})
			wantRefusal(t, "a typed sum out of range", err, connect.CodeInvalidArgument, "pool_sum must be 5 to 40")
			if !a.readiedOpen(t, "Pensantus") || a.readiedStatus(t, "Pensantus") != readiedWaiting {
				t.Errorf("the window is open %v and the row %q, want open and waiting", a.readiedOpen(t, "Pensantus"), a.readiedStatus(t, "Pensantus"))
			}
			if after := a.stateSnapshot(t); after != before || a.castLine(t, e, sleepSpell) != nil {
				t.Errorf("the refusal changed the state or left a line:\nbefore:\n%s\nafter:\n%s", before, after)
			}
		})
	}
}

func TestAReadiedReleaseRefusesDiceItDoesNotUseAndStaysOpen(t *testing.T) {
	t.Parallel()
	cases := []struct {
		name string
		set  func(*playv1.AnswerReactionRequest)
	}{
		{"pool_sum on a weapon attack", withPool(8)},
		{"d20_faces with roll_in_app", func(r *playv1.AnswerReactionRequest) {
			useSpellAnswer(r)
			r.Roll, r.D20Faces = &playv1.AnswerReactionRequest_RollInApp{RollInApp: true}, []int32{9}
		}},
		{"d20_faces with typed", func(r *playv1.AnswerReactionRequest) {
			useTyped(9)(r)
			r.D20Faces = []int32{9}
		}},
		{"three faces", useFaces(nil, 1, 2, 3)},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			a := newArmed(t)
			a.forceDice(t, campaignsv1.DiceMode_DICE_MODE_PHYSICAL)
			e := a.readyThenPass(t, nil)
			w := a.mustFire(t, e, "Toren")
			_, err := a.answerReaction(t, a.caio, e, w.GetId(), tc.set)
			wantCode(t, tc.name, err, connect.CodeInvalidArgument)
			if !a.readiedOpen(t, "Toren") || a.readiedStatus(t, "Toren") != readiedWaiting {
				t.Errorf("the refused answer left the window open %v and row %q", a.readiedOpen(t, "Toren"), a.readiedStatus(t, "Toren"))
			}
		})
	}
}

func TestAReadiedSpellRefusesDiceItDoesNotUse(t *testing.T) {
	t.Parallel()
	cases := []struct {
		name  string
		spell string
		set   func(*playv1.AnswerReactionRequest)
	}{
		{"d20_faces on a pool spell", sleepSpell, useFaces(nil, 9)},
		{"roll_mode on a pool spell", sleepSpell, withMode(playv1.RollMode_ROLL_MODE_DISADVANTAGE, "no escuro", 9)},
		{"pool_sum on a spell attack", fireBolt, withPool(8)},
		{"pool_sum on a save spell", burningHands, withPool(8)},
		{"d20_faces on a save spell", burningHands, useFaces(nil, 9)},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			a := newCasters(t)
			a.forceDice(t, campaignsv1.DiceMode_DICE_MODE_PHYSICAL)
			e := a.start(t, plan{
				npcs: []*playv1.Participant{{CharacterId: a.capitao.GetId()}}, npcRolls: []int{1}, reveal: []string{"Capitão Goblin"}, theatre: true,
				players: map[string]int32{"Pensantus": 20, "Toren": 15, "Brisa": 10},
			})
			slot := int32(1)
			if tc.spell == fireBolt {
				slot = 0
			}
			a.mustReady(t, a.ana, a.readySpell(t, e, "Pensantus", tc.spell, slot, "Capitão Goblin"))
			e = a.mustEndTurn(t, a.master, e)
			w := a.mustFire(t, e, "Pensantus")
			_, err := a.answerReaction(t, a.ana, e, w.GetId(), tc.set)
			wantCode(t, tc.name, err, connect.CodeInvalidArgument)
			if !a.readiedOpen(t, "Pensantus") || a.readiedStatus(t, "Pensantus") != readiedWaiting {
				t.Errorf("the refused answer left the window open %v and row %q", a.readiedOpen(t, "Pensantus"), a.readiedStatus(t, "Pensantus"))
			}
		})
	}
}

func TestAReadiedTypedReleaseReplaysWithTheSameKey(t *testing.T) {
	t.Parallel()
	a := newArmed(t)
	a.forceDice(t, campaignsv1.DiceMode_DICE_MODE_PHYSICAL)
	e := a.readyThenPass(t, nil)
	w := a.mustFire(t, e, "Toren")
	set := withMode(playv1.RollMode_ROLL_MODE_DISADVANTAGE, "no escuro", 15, 4)
	key := newKey()
	again := func(r *playv1.AnswerReactionRequest) { set(r); r.IdempotencyKey = key }
	first := a.mustAnswer(t, a.caio, e, w.GetId(), again)
	second := a.mustAnswer(t, a.caio, e, w.GetId(), again)
	if first.GetResult().GetUsed() != second.GetResult().GetUsed() || !second.GetResult().GetUsed() {
		t.Errorf("replay result = %v, first %v", second.GetResult(), first.GetResult())
	}
	if n := len(a.firedLines(t, a.master, e)); n != 1 {
		t.Errorf("fired lines = %d, want 1", n)
	}
}
