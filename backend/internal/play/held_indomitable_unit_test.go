package play

import (
	"testing"

	"connectrpc.com/connect"
)

func TestASaveRuleTellsWhenTheQuestionIsAskedAndWhatTheRollerMayKnow(t *testing.T) {
	t.Parallel()
	for _, tc := range []struct {
		name       string
		rule       saveRule
		face, tot  int
		ask, tells bool
	}{
		{"a save under the DC fails", saveRule{DC: 13}, 5, 5, true, true},
		{"a save at the DC passes", saveRule{DC: 13}, 13, 13, false, false},
		{"a blind table asks a pass too", saveRule{DC: 13, Blind: true}, 18, 18, true, false},
		{"a blind table asks a failure and tells nothing", saveRule{DC: 13, Blind: true}, 3, 3, true, false},
		{"a death save under 10 fails", saveRule{Death: true}, 9, 9, true, true},
		{"a death save of 10 passes", saveRule{Death: true}, 10, 10, false, false},
		{"a natural 1 fails whatever the die adds", saveRule{Death: true}, 1, 12, true, true},
		{"a natural 20 never fails", saveRule{Death: true}, 20, 5, false, false},
	} {
		ask, tells := tc.rule.decide(tc.face, tc.tot)
		if ask != tc.ask || tells != tc.tells {
			t.Errorf("%s: decide(%d, %d) = %v, %v; want %v, %v", tc.name, tc.face, tc.tot, ask, tells, tc.ask, tc.tells)
		}
	}
}

func TestAnIndomitableRerollIsListedByTheFeatureThatMadeIt(t *testing.T) {
	t.Parallel()
	ind := &indomitableUse{Index: 1, From: 4, To: 15}
	got := rerollsOf(&luckyReroll{Index: 0, From: 1, To: 9}, ind)
	if len(got) != 2 || got[1].GetSourceKey() != indomitableSourceKey || got[1].GetFrom() != 4 || got[1].GetTo() != 15 || got[1].GetIndex() != 1 {
		t.Errorf("rerollsOf() = %v, want Lucky's then Indomitable's 4 -> 15", got)
	}
	if rerollsOf(nil, nil) != nil {
		t.Error("rerollsOf(nil, nil) is not empty")
	}
}

// typed_faces belongs to Indomitable's answer alone and excludes the other ways to roll.
func TestRollOfAnswerRefusesConflictingWaysToRoll(t *testing.T) {
	t.Parallel()
	yes, face := true, int32(9)
	for name, tc := range map[string]struct {
		pick rollPick
		q    questionKind
		ok   bool
	}{
		"a pair for Indomitable":         {rollPick{faces: []int32{4, 15}}, askIndomitable, true},
		"a pair with the app":            {rollPick{faces: []int32{4, 15}, app: &yes}, askIndomitable, false},
		"a pair with a typed face":       {rollPick{faces: []int32{4, 15}, face: &face}, askIndomitable, false},
		"a pair for Lucky":               {rollPick{faces: []int32{4, 15}}, askLucky, false},
		"a pair for the die":             {rollPick{faces: []int32{4, 15}}, askDie, false},
		"a pair with a face out of 1-20": {rollPick{faces: []int32{4, 21}}, askIndomitable, false},
	} {
		_, err := rollOfAnswer(tc.pick, tc.q)
		if (err == nil) != tc.ok {
			t.Errorf("%s: error = %v, want ok = %v", name, err, tc.ok)
		} else if err != nil && connect.CodeOf(err) != connect.CodeInvalidArgument {
			t.Errorf("%s: code = %v, want invalid_argument", name, connect.CodeOf(err))
		}
	}
}
