package play

import (
	"testing"

	"connectrpc.com/connect"

	playv1 "github.com/PuraFome/meuRPG/backend/gen/meurpg/play/v1"
)

// The request of RollDamage read on its own (readDamageCall): the refusals that depend on the request alone.
// The ranges of the sums depend on the dice of the pending damage and stay with the harness scenarios.
func TestReadDamageCall(t *testing.T) {
	t.Parallel()
	typedParts := func(parts ...*playv1.TypedPart) func(*playv1.RollDamageRequest) {
		return func(r *playv1.RollDamageRequest) { r.Roll, r.TypedParts = nil, parts }
	}
	partsOnce := callRefusal{connect.CodeInvalidArgument, "typed_parts must name each part once"}
	cases := []struct {
		name string
		edit func(*playv1.RollDamageRequest)
		want callRefusal
	}{
		{"a key that is not a UUID", func(r *playv1.RollDamageRequest) { r.IdempotencyKey = "not-a-key" }, refusedKey},
		{"an encounter that is not a UUID", func(r *playv1.RollDamageRequest) { r.EncounterId = "nope" }, refusedEncounter},
		{"a pending damage that is not a UUID", func(r *playv1.RollDamageRequest) { r.PendingDamageId = "nope" }, refusedPending},
		{"roll_in_app false", func(r *playv1.RollDamageRequest) {
			r.Roll = &playv1.RollDamageRequest_RollInApp{RollInApp: false}
		}, callRefusal{connect.CodeInvalidArgument, "roll_in_app must be true"}},
		{"no roll at all", func(r *playv1.RollDamageRequest) { r.Roll = nil }, callRefusal{connect.CodeInvalidArgument, "set roll_in_app, typed_sum or typed_parts"}},
		{"a typed part named twice", typedParts(&playv1.TypedPart{PartKey: "weapon", Sum: 3}, &playv1.TypedPart{PartKey: "weapon", Sum: 4}), partsOnce},
		{"a typed part with no name", typedParts(&playv1.TypedPart{Sum: 3}), partsOnce},
		{"a roll in the app", func(*playv1.RollDamageRequest) {}, callRefusal{}},
		{"a typed sum", func(r *playv1.RollDamageRequest) { r.Roll = &playv1.RollDamageRequest_TypedSum{TypedSum: 5} }, callRefusal{}},
		{"typed parts with different names", typedParts(&playv1.TypedPart{PartKey: "weapon", Sum: 3}, &playv1.TypedPart{PartKey: "sneak-attack", Sum: 4}), callRefusal{}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			req := &playv1.RollDamageRequest{
				IdempotencyKey: callKey, EncounterId: callEncounterID, PendingDamageId: callPendingID,
				Roll: &playv1.RollDamageRequest_RollInApp{RollInApp: true},
			}
			tc.edit(req)
			call, err := readDamageCall(req)
			wantCallRefusal(t, err, tc.want)
			if tc.want.msg == "" && (call.key != callKey || call.encounterID != callEncounterID || call.pendingID != callPendingID) {
				t.Errorf("call ids = %q %q %q, want the request's", call.key, call.encounterID, call.pendingID)
			}
		})
	}
}
