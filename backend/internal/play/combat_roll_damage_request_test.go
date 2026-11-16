package play

import (
	"testing"

	"connectrpc.com/connect"

	playv1 "github.com/PuraFome/meuRPG/backend/gen/meurpg/play/v1"
)

// TestRollDamageRefusesTheRequest sends one request after another on one fight, each with a field spoiled
// as Toren's player on the damage of his hit, and reads the exact code and message of each refusal and that
// none writes anything.
func TestRollDamageRefusesTheRequest(t *testing.T) {
	t.Parallel()
	a := newArmed(t)
	e := a.threeAndAGoblin(t)
	id := torenHits(t, a, e, battleaxe, 15)
	for _, c := range []struct {
		name string
		edit func(*playv1.RollDamageRequest)
		code connect.Code
		msg  string
	}{
		{"a key that is not a UUID", func(r *playv1.RollDamageRequest) { r.IdempotencyKey = "not-a-key" }, connect.CodeInvalidArgument, "idempotency_key must be a UUID"},
		{"an encounter that is not a UUID", func(r *playv1.RollDamageRequest) { r.EncounterId = "nope" }, connect.CodeNotFound, "encounter not found"},
		{"a pending damage that is not a UUID", func(r *playv1.RollDamageRequest) { r.PendingDamageId = "nope" }, connect.CodeNotFound, "pending damage not found"},
		{"roll_in_app false", func(r *playv1.RollDamageRequest) {
			r.Roll = &playv1.RollDamageRequest_RollInApp{RollInApp: false}
		}, connect.CodeInvalidArgument, "roll_in_app must be true"},
		{"no roll at all", func(r *playv1.RollDamageRequest) { r.Roll = nil }, connect.CodeInvalidArgument, "set roll_in_app, typed_sum or typed_parts"},
		{"a typed sum of 0", typedSum(0), connect.CodeInvalidArgument, "typed_sum must be 1 to 10"},
		{"a typed sum of 11 for 1d10", typedSum(11), connect.CodeInvalidArgument, "typed_sum must be 1 to 10"},
		{"a pending damage that is not there", func(r *playv1.RollDamageRequest) {
			r.PendingDamageId = "00000000-0000-4000-8000-000000000001"
		}, connect.CodeNotFound, "pending damage not found"},
		{"an encounter that is not there", func(r *playv1.RollDamageRequest) {
			r.EncounterId = "00000000-0000-4000-8000-000000000002"
		}, connect.CodeNotFound, "encounter not found"},
	} {
		before := a.stateSnapshot(t)
		_, err := a.rawDamage(t, a.caio, e, id, c.edit)
		wantRefusal(t, c.name, err, c.code, c.msg)
		if a.stateSnapshot(t) != before {
			t.Errorf("%s changed the fight", c.name)
		}
	}
}
