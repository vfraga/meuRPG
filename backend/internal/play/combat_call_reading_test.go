package play

import (
	"errors"
	"strings"
	"testing"

	"connectrpc.com/connect"

	playv1 "github.com/PuraFome/meuRPG/backend/gen/meurpg/play/v1"
)

// The requests of the combat calls are read, and refused, before any transaction opens: the readXCall
// functions take the request message and nothing else. These tests hold every refusal that depends on the
// request alone (a malformed id, a number out of range, a missing or contradictory roll) with its code and
// message. The harness cases of the golden scenarios keep one refusal of each call, to prove that the call
// reads its request through these functions and writes nothing when it is refused.

const (
	callKey         = "0b8e6a54-3a61-4e0c-9a64-5a7a2a1c0001"
	callEncounterID = "0b8e6a54-3a61-4e0c-9a64-5a7a2a1c0002"
	callCombatantID = "0b8e6a54-3a61-4e0c-9a64-5a7a2a1c0003"
	callPendingID   = "0b8e6a54-3a61-4e0c-9a64-5a7a2a1c0004"
)

// callRefusal is the refusal a request reads into: its code and message.
type callRefusal struct {
	code connect.Code
	msg  string
}

var (
	refusedKey       = callRefusal{connect.CodeInvalidArgument, "idempotency_key must be a UUID"}
	refusedEncounter = callRefusal{connect.CodeNotFound, "encounter not found"}
	refusedCombatant = callRefusal{connect.CodeNotFound, "combatant not found"}
	refusedPending   = callRefusal{connect.CodeNotFound, "pending damage not found"}
)

// wantCallRefusal checks that err is the connect error want names; a zero want means no error.
func wantCallRefusal(t *testing.T, err error, want callRefusal) {
	t.Helper()
	if want.msg == "" {
		if err != nil {
			t.Errorf("error = %v, want none", err)
		}
		return
	}
	var cerr *connect.Error
	if !errors.As(err, &cerr) {
		t.Errorf("error = %v, want %v: %s", err, want.code, want.msg)
		return
	}
	if cerr.Code() != want.code || cerr.Message() != want.msg {
		t.Errorf("error = %v: %s, want %v: %s", cerr.Code(), cerr.Message(), want.code, want.msg)
	}
}

func TestReadAdjustHitPointsCall(t *testing.T) {
	t.Parallel()
	temp := func(n int32) *int32 { return &n }
	damage := func(n int32) *playv1.AdjustCombatantHitPointsRequest_Damage {
		return &playv1.AdjustCombatantHitPointsRequest_Damage{Damage: n}
	}
	heal := func(n int32) *playv1.AdjustCombatantHitPointsRequest_Heal {
		return &playv1.AdjustCombatantHitPointsRequest_Heal{Heal: n}
	}
	setTo := func(n int32) *playv1.AdjustCombatantHitPointsRequest_HitPoints {
		return &playv1.AdjustCombatantHitPointsRequest_HitPoints{HitPoints: n}
	}
	cases := []struct {
		name string
		edit func(*playv1.AdjustCombatantHitPointsRequest)
		want callRefusal
	}{
		{"a key that is not a UUID", func(r *playv1.AdjustCombatantHitPointsRequest) { r.IdempotencyKey = "not-a-key"; r.Change = damage(1) }, refusedKey},
		{"an encounter that is not a UUID", func(r *playv1.AdjustCombatantHitPointsRequest) { r.EncounterId = "nope"; r.Change = damage(1) }, refusedEncounter},
		{"a combatant that is not a UUID", func(r *playv1.AdjustCombatantHitPointsRequest) { r.CombatantId = "nope"; r.Change = damage(1) }, refusedCombatant},
		{"a request that changes nothing", func(*playv1.AdjustCombatantHitPointsRequest) {}, callRefusal{connect.CodeInvalidArgument, "set damage, heal, hit_points or hit_points_temporary"}},
		{"a negative damage", func(r *playv1.AdjustCombatantHitPointsRequest) { r.Change = damage(-1) }, callRefusal{connect.CodeInvalidArgument, "damage must be 0 to 9999"}},
		{"a damage over the limit", func(r *playv1.AdjustCombatantHitPointsRequest) { r.Change = damage(10000) }, callRefusal{connect.CodeInvalidArgument, "damage must be 0 to 9999"}},
		{"a negative heal", func(r *playv1.AdjustCombatantHitPointsRequest) { r.Change = heal(-1) }, callRefusal{connect.CodeInvalidArgument, "heal must be 0 to 9999"}},
		{"a heal over the limit", func(r *playv1.AdjustCombatantHitPointsRequest) { r.Change = heal(10000) }, callRefusal{connect.CodeInvalidArgument, "heal must be 0 to 9999"}},
		{"a negative hit point count", func(r *playv1.AdjustCombatantHitPointsRequest) { r.Change = setTo(-1) }, callRefusal{connect.CodeInvalidArgument, "set must be 0 to 9999"}},
		{"a hit point count over the limit", func(r *playv1.AdjustCombatantHitPointsRequest) { r.Change = setTo(10000) }, callRefusal{connect.CodeInvalidArgument, "set must be 0 to 9999"}},
		{"negative temporary hit points", func(r *playv1.AdjustCombatantHitPointsRequest) { r.HitPointsTemporary = temp(-1) }, callRefusal{connect.CodeInvalidArgument, "hit_points_temporary must be 0 to 999"}},
		{"temporary hit points over the limit", func(r *playv1.AdjustCombatantHitPointsRequest) { r.HitPointsTemporary = temp(1000) }, callRefusal{connect.CodeInvalidArgument, "hit_points_temporary must be 0 to 999"}},
		{"temporary hit points next to a damage over the limit", func(r *playv1.AdjustCombatantHitPointsRequest) {
			r.Change, r.HitPointsTemporary = damage(10000), temp(1000)
		}, callRefusal{connect.CodeInvalidArgument, "damage must be 0 to 9999"}},
		{"the largest damage", func(r *playv1.AdjustCombatantHitPointsRequest) { r.Change = damage(9999) }, callRefusal{}},
		{"the largest temporary hit points alone", func(r *playv1.AdjustCombatantHitPointsRequest) { r.HitPointsTemporary = temp(999) }, callRefusal{}},
		{"a damage of 0", func(r *playv1.AdjustCombatantHitPointsRequest) { r.Change = damage(0) }, callRefusal{}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			req := &playv1.AdjustCombatantHitPointsRequest{IdempotencyKey: callKey, EncounterId: callEncounterID, CombatantId: callCombatantID}
			tc.edit(req)
			call, err := readAdjustHitPointsCall(req)
			wantCallRefusal(t, err, tc.want)
			if tc.want.msg == "" && (call.key != callKey || call.encounterID != callEncounterID || call.combatantID != callCombatantID) {
				t.Errorf("call ids = %q %q %q, want the request's", call.key, call.encounterID, call.combatantID)
			}
		})
	}
}

func TestReadApplyDamageCall(t *testing.T) {
	t.Parallel()
	amount := func(n int32) *int32 { return &n }
	sources := func(n int) []string { return strings.Split(strings.Repeat("feature:rage,", n), ",")[:n] }
	amountRefusal := callRefusal{connect.CodeInvalidArgument, "amount must be 0 to 9999"}
	cases := []struct {
		name string
		edit func(*playv1.ApplyPendingDamageRequest)
		want callRefusal
	}{
		{"a key that is not a UUID", func(r *playv1.ApplyPendingDamageRequest) { r.IdempotencyKey = "not-a-key" }, refusedKey},
		{"an encounter that is not a UUID", func(r *playv1.ApplyPendingDamageRequest) { r.EncounterId = "nope" }, refusedEncounter},
		{"a pending damage that is not a UUID", func(r *playv1.ApplyPendingDamageRequest) { r.PendingDamageId = "nope" }, refusedPending},
		{"a negative amount", func(r *playv1.ApplyPendingDamageRequest) { r.Amount = amount(-1) }, amountRefusal},
		{"an amount over the limit", func(r *playv1.ApplyPendingDamageRequest) { r.Amount = amount(10000) }, amountRefusal},
		{"the largest amount", func(r *playv1.ApplyPendingDamageRequest) { r.Amount = amount(9999) }, callRefusal{}},
		{"an amount of 0", func(r *playv1.ApplyPendingDamageRequest) { r.Amount = amount(0) }, callRefusal{}},
		{"21 sources to ignore", func(r *playv1.ApplyPendingDamageRequest) { r.IgnoreModifiers = sources(21) }, callRefusal{connect.CodeInvalidArgument, "ignore_modifiers must have at most 20 sources"}},
		{"20 sources to ignore", func(r *playv1.ApplyPendingDamageRequest) { r.IgnoreModifiers = sources(20) }, callRefusal{}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			req := &playv1.ApplyPendingDamageRequest{IdempotencyKey: callKey, EncounterId: callEncounterID, PendingDamageId: callPendingID}
			tc.edit(req)
			call, err := readApplyDamageCall(req)
			wantCallRefusal(t, err, tc.want)
			if tc.want.msg == "" && (call.key != callKey || call.encounterID != callEncounterID || call.pendingID != callPendingID) {
				t.Errorf("call ids = %q %q %q, want the request's", call.key, call.encounterID, call.pendingID)
			}
		})
	}
}

func TestReadDiscardDamageCall(t *testing.T) {
	t.Parallel()
	cases := []struct {
		name string
		edit func(*playv1.DiscardPendingDamageRequest)
		want callRefusal
	}{
		{"a key that is not a UUID", func(r *playv1.DiscardPendingDamageRequest) { r.IdempotencyKey = "not-a-key" }, refusedKey},
		{"an encounter that is not a UUID", func(r *playv1.DiscardPendingDamageRequest) { r.EncounterId = "nope" }, refusedEncounter},
		{"a pending damage that is not a UUID", func(r *playv1.DiscardPendingDamageRequest) { r.PendingDamageId = "nope" }, refusedPending},
		{"ids that are all UUIDs", func(*playv1.DiscardPendingDamageRequest) {}, callRefusal{}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			req := &playv1.DiscardPendingDamageRequest{IdempotencyKey: callKey, EncounterId: callEncounterID, PendingDamageId: callPendingID}
			tc.edit(req)
			call, err := readDiscardDamageCall(req)
			wantCallRefusal(t, err, tc.want)
			if tc.want.msg == "" && (call.key != callKey || call.encounterID != callEncounterID || call.pendingID != callPendingID) {
				t.Errorf("call ids = %q %q %q, want the request's", call.key, call.encounterID, call.pendingID)
			}
		})
	}
}

func TestReadTurnOptionsCall(t *testing.T) {
	t.Parallel()
	cases := []struct {
		name string
		edit func(*playv1.GetTurnOptionsRequest)
		want callRefusal
	}{
		{"an encounter that is not a UUID", func(r *playv1.GetTurnOptionsRequest) { r.EncounterId = "nope" }, refusedEncounter},
		{"a combatant that is not a UUID", func(r *playv1.GetTurnOptionsRequest) { r.CombatantId = "nope" }, refusedCombatant},
		{"ids that are all UUIDs", func(*playv1.GetTurnOptionsRequest) {}, callRefusal{}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			req := &playv1.GetTurnOptionsRequest{EncounterId: callEncounterID, CombatantId: callCombatantID}
			tc.edit(req)
			call, err := readTurnOptionsCall(req)
			wantCallRefusal(t, err, tc.want)
			if tc.want.msg == "" && (call.encounterID != callEncounterID || call.combatantID != callCombatantID) {
				t.Errorf("call ids = %q %q, want the request's", call.encounterID, call.combatantID)
			}
		})
	}
}
