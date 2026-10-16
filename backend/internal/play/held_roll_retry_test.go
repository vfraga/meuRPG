package play

import (
	"testing"

	"connectrpc.com/connect"
)

// The faces the story uses: a 1 (Lucky asks), a 14 (nothing to ask but the die), the 15 Lucky rolls
// again to, the Bardic Inspiration die's 5, and the sentinel a retry must never take from the roller.
const (
	heldOne      = 1
	heldFourteen = 14
	heldRerolled = 15
	heldDieFace  = 5
	heldSentinel = 2
)

// heldAnswer is one answer of a flow: to the Lucky question (lucky), or the die's; yes is "roll
// again" for Lucky and "use the die" for the die.
type heldAnswer struct{ lucky, yes bool }

// heldFlows are the five ways a held roll can go, each from the d20 the roll shows to the answers
// the player gives, in order.
var heldFlows = []struct {
	name    string
	die     bool  // the character holds a Bardic Inspiration die
	face    int32 // the d20 of the roll
	answers []heldAnswer
}{
	{"die used", true, heldFourteen, []heldAnswer{{false, true}}},
	{"die kept", true, heldFourteen, []heldAnswer{{false, false}}},
	{"Lucky rerolled", false, heldOne, []heldAnswer{{true, true}}},
	{"Lucky kept", false, heldOne, []heldAnswer{{true, false}}},
	{"Lucky then die", true, heldOne, []heldAnswer{{true, true}, {false, true}}},
}

// TestEveryKindRetriesTheRollAndTheAnswerToTheSameResult walks the eight kinds of held roll through
// the five flows (the Bardic Inspiration die used or kept, Lucky rolling again or keeping the 1, Lucky
// and then the die) and, at every step, asks the three things a flaky network does: the roll again
// (the same question comes back and nothing is rolled), the answer again (the same result comes
// back and nothing is rolled), and another answer under a new key (refused: one answer per
// question). It is what the unified gate of E4 has to keep, kind by kind.
func TestEveryKindRetriesTheRollAndTheAnswerToTheSameResult(t *testing.T) {
	t.Parallel()
	for _, kind := range heldKinds {
		for _, flow := range heldFlows {
			t.Run(kind.name+"/"+flow.name, func(t *testing.T) {
				t.Parallel()
				walkHeldFlow(t, kind.rig(t, flow.die), flow.face, flow.answers)
			})
		}
	}
}

// walkHeldFlow runs the story of one kind and one flow.
func walkHeldFlow(t *testing.T, r heldRig, face int32, answers []heldAnswer) {
	t.Helper()
	rollKey := newKey()
	r.roller.queue(int(face))
	cur, err := r.roll(t, rollKey)
	if err != nil {
		t.Fatalf("the roll: error = %v", err)
	}
	wantQuestion(t, "the roll", cur, answers[0].lucky, face, 0)
	// replayOf is what a retry says: the question as it is, or the written roll as the kind replays it.
	replayOf := func(v heldView) heldView {
		if v.resolved && r.replayed != nil {
			return r.replayed(v)
		}
		return v
	}
	retryTheRoll := func(what string, want heldView) {
		t.Helper()
		r.roller.queue(heldSentinel)
		got, err := r.roll(t, rollKey)
		if want.resolved && r.refusesTheRollReplay {
			wantCode(t, what+": the roll again", err, connect.CodeInvalidArgument)
			wantNothingRolled(t, r, what+": the roll again")
			return
		}
		want = replayOf(want)
		if err != nil {
			t.Fatalf("%s: the roll again: error = %v", what, err)
		}
		if got != want {
			t.Errorf("%s: the roll again = %+v, want what it gave before: %+v", what, got, want)
		}
		wantNothingRolled(t, r, what+": the roll again")
		if n := r.holds(t); n != 1 {
			t.Errorf("%s: %d held rolls after the roll again, want 1", what, n)
		}
	}
	rerolls := 0
	for i, ans := range answers {
		retryTheRoll("before answer "+string(rune('1'+i)), cur)
		answer := r.answerDie
		if ans.lucky {
			answer = r.answerLucky
		}
		holdID := cur.dieHold
		if ans.lucky {
			holdID = cur.luckyHold
		}
		key := newKey()
		next := answerHeld(t, r, answer, holdID, key, ans)
		// The same answer again: the same result, nothing rolled; another key: one answer only.
		r.roller.queue(heldSentinel)
		again, err := answer(t, holdID, key, ans.yes)
		if err != nil {
			t.Fatalf("answer %d again: error = %v", i+1, err)
		}
		if want := replayOf(next); again != want {
			t.Errorf("answer %d again = %+v, want %+v", i+1, again, want)
		}
		wantNothingRolled(t, r, "the answer again")
		r.roller.queue(heldSentinel)
		_, err = answer(t, holdID, newKey(), ans.yes)
		wantCode(t, "another answer under a new key", err, connect.CodeFailedPrecondition)
		r.roller.drain()
		if ans.lucky && ans.yes {
			rerolls = 1
		}
		last := i == len(answers)-1
		if last {
			wantResolved(t, next, ans, rerolls, heldRerolled, face)
		} else {
			wantQuestion(t, "after Lucky", next, false, heldRerolled, rerolls)
		}
		cur = next
	}
	retryTheRoll("once resolved", cur)
}

// answerHeld queues what the answer rolls (the new d20 or the die) and gives it.
func answerHeld(t *testing.T, r heldRig, answer func(*testing.T, string, string, bool) (heldView, error), holdID, key string, ans heldAnswer) heldView {
	t.Helper()
	switch {
	case ans.lucky && ans.yes:
		r.roller.queue(heldRerolled)
	case !ans.lucky && ans.yes:
		r.roller.queue(heldDieFace)
	}
	got, err := answer(t, holdID, key, ans.yes)
	if err != nil {
		t.Fatalf("the answer (lucky %v, yes %v): error = %v", ans.lucky, ans.yes, err)
	}
	return got
}

// wantNothingRolled checks a retry left the sentinel in the roller, and empties it.
func wantNothingRolled(t *testing.T, r heldRig, what string) {
	t.Helper()
	if n := r.roller.pending(); n != 1 {
		t.Errorf("%s took %d faces from the roller, want none", what, 1-n)
	}
	r.roller.drain()
}

// wantQuestion checks a roll that waits: Lucky's question (a 1) comes alone first, else the die's, on
// the d20 the roll shows.
func wantQuestion(t *testing.T, what string, v heldView, lucky bool, face int32, rerolled int) {
	t.Helper()
	asked := v.dieHold
	if lucky {
		asked = v.luckyHold
	}
	other := v.luckyHold
	if lucky {
		other = v.dieHold
	}
	if v.resolved || asked == "" || other != "" || v.face != face || v.rerolled != rerolled {
		t.Errorf("%s = %+v, want the question of %s alone on the d20 %d (rolled again %d)", what, v, map[bool]string{true: "Lucky", false: "the die"}[lucky], face, rerolled)
	}
}

// wantResolved checks the roll written by the last answer: the d20 that counts (the new one when Lucky
// rolled again, the 1 when it kept it), the die added or not, and the total of the three.
func wantResolved(t *testing.T, v heldView, last heldAnswer, rerolls int, rerolled, first int32) {
	t.Helper()
	want := first
	if rerolls == 1 {
		want = rerolled
	}
	var bonus int32
	if !last.lucky && last.yes {
		bonus = heldDieFace
	}
	if !v.resolved || v.luckyHold != "" || v.dieHold != "" {
		t.Errorf("the last answer = %+v, want the roll written and no question left", v)
	}
	if v.face != want || v.rerolled != rerolls || v.bonus != bonus {
		t.Errorf("the roll = %+v, want the d20 %d (rolled again %d) and the die %d", v, want, rerolls, bonus)
	}
	// Every kind folds the die into the modifier it shows, so the total is the d20 and the modifier.
	if v.total != v.value+v.modifier {
		t.Errorf("the roll = %+v, want the total to be the d20 (%d) + the modifier (%d, the die's face in it)", v, v.value, v.modifier)
	}
}
