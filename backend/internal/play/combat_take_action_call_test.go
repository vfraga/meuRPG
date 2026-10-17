package play

import (
	"testing"

	"connectrpc.com/connect"

	playv1 "github.com/PuraFome/meuRPG/backend/gen/meurpg/play/v1"
)

// The request of TakeAction read on its own (readTakeActionCall): the refusals that depend on the request
// alone. Whether the action exists for the combatant is read from its sheet and stays with the harness
// scenarios.
func TestReadTakeActionCall(t *testing.T) {
	t.Parallel()
	ownCall := callRefusal{connect.CodeInvalidArgument, "an attack is rolled with RollAttack, and a spell is cast with CastSpell"}
	cases := []struct {
		name string
		edit func(*playv1.TakeActionRequest)
		want callRefusal
	}{
		{"a key that is not a UUID", func(r *playv1.TakeActionRequest) { r.IdempotencyKey = "not-a-key" }, refusedKey},
		{"an encounter that is not a UUID", func(r *playv1.TakeActionRequest) { r.EncounterId = "nope" }, refusedEncounter},
		{"a combatant that is not a UUID", func(r *playv1.TakeActionRequest) { r.CombatantId = "nope" }, refusedCombatant},
		{"the attack, which has its own call", func(r *playv1.TakeActionRequest) { r.ActionKey = standardAttack }, ownCall},
		{"the cast, which has its own call", func(r *playv1.TakeActionRequest) { r.ActionKey = standardCast }, ownCall},
		{"roll_in_app false", func(r *playv1.TakeActionRequest) {
			r.Roll = &playv1.TakeActionRequest_RollInApp{RollInApp: false}
		}, callRefusal{connect.CodeInvalidArgument, "roll_in_app must be true"}},
		{"an action with no roll", func(*playv1.TakeActionRequest) {}, callRefusal{}},
		{"a roll in the app", func(r *playv1.TakeActionRequest) { r.Roll = &playv1.TakeActionRequest_RollInApp{RollInApp: true} }, callRefusal{}},
		{"a typed sum", func(r *playv1.TakeActionRequest) { r.Roll = &playv1.TakeActionRequest_TypedSum{TypedSum: 7} }, callRefusal{}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			req := &playv1.TakeActionRequest{IdempotencyKey: callKey, EncounterId: callEncounterID, CombatantId: callCombatantID, ActionKey: actionDodge}
			tc.edit(req)
			call, err := readTakeActionCall(req)
			wantCallRefusal(t, err, tc.want)
			if tc.want.msg == "" && (call.key != callKey || call.encounterID != callEncounterID || call.combatantID != callCombatantID) {
				t.Errorf("call ids = %q %q %q, want the request's", call.key, call.encounterID, call.combatantID)
			}
		})
	}
}
