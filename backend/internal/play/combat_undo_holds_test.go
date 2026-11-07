package play

import (
	"regexp"
	"strconv"
	"strings"
	"testing"

	"connectrpc.com/connect"

	playv1 "github.com/PuraFome/meuRPG/backend/gen/meurpg/play/v1"
)

// appliedDamageOnToren plays a goblin's hit on Toren up to the applied damage, the last action.
func appliedDamageOnToren(t *testing.T, a *armed) (*playv1.Encounter, string) {
	t.Helper()
	e := a.threeAndAGoblin(t)
	a.mustEndTurn(t, a.caio, e)
	a.mustEndTurn(t, a.ana, e)
	a.mustEndTurn(t, a.bia, e) // the goblin
	a.h.roller.queue(15, 3)
	hit := a.mustAttack(t, a.master, e, "Goblin", sword, "Toren", inAppRoll)
	id := hit.GetPendingDamage().GetId()
	a.mustDamage(t, a.master, e, id, inAppDamage)
	if _, err := a.settle(t, a.master, e, id, true); err != nil {
		t.Fatalf("ApplyPendingDamage() error = %v", err)
	}
	return e, a.log(t, a.master, e).GetUndoableEventId()
}

// blockedOf reads the EncounterBlocked detail of a refusal.
func blockedOf(t *testing.T, err error) *playv1.EncounterBlocked {
	t.Helper()
	var ce *connect.Error
	if !asConnectError(err, &ce) {
		t.Fatalf("error = %v, want a connect error", err)
	}
	for _, d := range ce.Details() {
		if v, derr := d.Value(); derr == nil {
			if b, ok := v.(*playv1.EncounterBlocked); ok {
				return b
			}
		}
	}
	t.Fatalf("error %v has no EncounterBlocked detail", err)
	return nil
}

// TestUndoIsRefusedWhenACharactersHitPointsChangedAfterTheAction: a write that leaves no event to close the
// chain (here, the vitals row itself) changed Toren after the damage was applied. The undo would put the
// "before" over it, so the whole undo is refused and nothing is written.
func TestUndoIsRefusedWhenACharactersHitPointsChangedAfterTheAction(t *testing.T) {
	t.Parallel()
	a := newArmed(t)
	e, last := appliedDamageOnToren(t, a)
	afterHit := a.vitals(t, a.toren).GetHitPointsCurrent()
	healed := afterHit + 2
	if _, err := a.h.pool.Exec(t.Context(), `UPDATE character_vitals SET hit_points_current = $2 WHERE character_id = $1`, a.toren.GetId(), healed); err != nil {
		t.Fatalf("change the hit points: %v", err)
	}
	err := a.undo(t, a.master, e, last)
	wantCode(t, "UndoLastAction after the change", err, connect.CodeFailedPrecondition)
	b := blockedOf(t, err)
	if b.GetReason() != playv1.EncounterBlockedReason_ENCOUNTER_BLOCKED_REASON_UNDO_TARGET_CHANGED {
		t.Errorf("reason = %v, want UNDO_TARGET_CHANGED", b.GetReason())
	}
	if want := a.id(t, "Toren"); len(b.GetCombatantIds()) != 1 || b.GetCombatantIds()[0] != want {
		t.Errorf("combatant_ids = %v, want [%s]", b.GetCombatantIds(), want)
	}
	if got := a.vitals(t, a.toren).GetHitPointsCurrent(); got != healed {
		t.Errorf("hit points = %d after the refused undo, want %d: nothing is written", got, healed)
	}
	if got := a.log(t, a.master, e).GetUndoableEventId(); got != last {
		t.Errorf("the damage is no longer the last action after a refused undo")
	}
}

// TestUndoLeavesWhatAnActionNeverChanged: a heal at full hit points changes no number, so its undo writes none and a
// change made by someone else afterwards stays.
func TestUndoLeavesWhatAnActionNeverChanged(t *testing.T) {
	t.Parallel()
	a := newArmed(t)
	e := a.threeAndAGoblin(t)
	if _, err := a.adjustHP(t, e, "Goblin", healHP(3)); err != nil {
		t.Fatalf("AdjustCombatantHitPoints() error = %v", err)
	}
	last := a.log(t, a.master, e).GetUndoableEventId()
	var hp int32
	if err := a.h.pool.QueryRow(t.Context(), `UPDATE combatants SET hp_current = hp_current - 1, hp_temp = 6 WHERE id = $1 RETURNING hp_current`, a.id(t, "Goblin")).Scan(&hp); err != nil {
		t.Fatalf("change the hit points: %v", err)
	}
	if err := a.undo(t, a.master, e, last); err != nil {
		t.Fatalf("UndoLastAction() error = %v, want it to go through", err)
	}
	var gotHP, gotTemp int32
	if err := a.h.pool.QueryRow(t.Context(), `SELECT hp_current, hp_temp FROM combatants WHERE id = $1`, a.id(t, "Goblin")).Scan(&gotHP, &gotTemp); err != nil {
		t.Fatalf("read the hit points: %v", err)
	}
	if gotHP != hp || gotTemp != 6 {
		t.Errorf("the goblin has %d and %d temporary after the undo, want the other change kept (%d and 6)", gotHP, gotTemp, hp)
	}
}

// TestUndoIsRefusedWhenACharactersTempOrDefeatChangedAfterTheAction: the temporary hit points and the defeat of a
// player's combatant are held like its hit points.
func TestUndoIsRefusedWhenACharactersTempOrDefeatChangedAfterTheAction(t *testing.T) {
	t.Parallel()
	for name, change := range map[string]string{
		"temporary hit points": `UPDATE character_vitals SET hit_points_temporary = 5 WHERE character_id = $1`,
		"defeat":               `UPDATE combatants SET defeated = NOT defeated WHERE character_id = $1`,
	} {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			a := newArmed(t)
			e, last := appliedDamageOnToren(t, a)
			if _, err := a.h.pool.Exec(t.Context(), change, a.toren.GetId()); err != nil {
				t.Fatalf("change the %s: %v", name, err)
			}
			err := a.undo(t, a.master, e, last)
			wantCode(t, "UndoLastAction after the change", err, connect.CodeFailedPrecondition)
			if b := blockedOf(t, err); b.GetReason() != playv1.EncounterBlockedReason_ENCOUNTER_BLOCKED_REASON_UNDO_TARGET_CHANGED {
				t.Errorf("reason = %v, want UNDO_TARGET_CHANGED", b.GetReason())
			}
		})
	}
}

// TestUndoIsRefusedWhenANpcsHitPointsChangedAfterTheAction is the same for a combatant that holds its own hit points.
func TestUndoIsRefusedWhenANpcsHitPointsChangedAfterTheAction(t *testing.T) {
	t.Parallel()
	a := newArmed(t)
	e := a.threeAndAGoblin(t)
	if _, err := a.adjustHP(t, e, "Goblin", damageHP(2)); err != nil {
		t.Fatalf("AdjustCombatantHitPoints() error = %v", err)
	}
	last := a.log(t, a.master, e).GetUndoableEventId()
	if _, err := a.h.pool.Exec(t.Context(), `UPDATE combatants SET hp_current = hp_current + 1 WHERE id = $1`, a.id(t, "Goblin")); err != nil {
		t.Fatalf("change the hit points: %v", err)
	}
	err := a.undo(t, a.master, e, last)
	wantCode(t, "UndoLastAction after the change", err, connect.CodeFailedPrecondition)
	if got := blockedOf(t, err).GetReason(); got != playv1.EncounterBlockedReason_ENCOUNTER_BLOCKED_REASON_UNDO_TARGET_CHANGED {
		t.Errorf("reason = %v, want UNDO_TARGET_CHANGED", got)
	}
}

// TestUndoWithNothingInBetweenStillWorks: the hit points are the ones the action left, so the undo goes through.
func TestUndoWithNothingInBetweenStillWorks(t *testing.T) {
	t.Parallel()
	a := newArmed(t)
	e, last := appliedDamageOnToren(t, a)
	before := a.vitals(t, a.toren).GetHitPointsMax()
	if err := a.undo(t, a.master, e, last); err != nil {
		t.Fatalf("UndoLastAction() error = %v", err)
	}
	if got := a.vitals(t, a.toren).GetHitPointsCurrent(); got != before {
		t.Errorf("Toren = %d after the undo, want the full %d", got, before)
	}
}

// TestUndoOfAnEventWithoutHoldsIsNotChecked: an event from before the hit points were kept has nothing to compare.
func TestUndoOfAnEventWithoutHoldsIsNotChecked(t *testing.T) {
	t.Parallel()
	a := newArmed(t)
	e, last := appliedDamageOnToren(t, a)
	if _, err := a.h.pool.Exec(t.Context(), `UPDATE session_events SET payload = payload - 'holds' WHERE id = $1`, last); err != nil {
		t.Fatalf("drop the holds: %v", err)
	}
	if _, err := a.h.pool.Exec(t.Context(), `UPDATE character_vitals SET hit_points_current = hit_points_current + 1 WHERE character_id = $1`, a.toren.GetId()); err != nil {
		t.Fatalf("change the hit points: %v", err)
	}
	if err := a.undo(t, a.master, e, last); err != nil {
		t.Fatalf("UndoLastAction() of an event without holds error = %v, want it to go through", err)
	}
}

// withoutUUIDs drops every UUID from text before a numeric canary is searched: a random id holds any four digits
// (run 42337730750 matched "4217" in one), and an id is not a hit point number.
func withoutUUIDs(text string) string { return uuidText.ReplaceAllString(text, "<id>") }

// hasNumber says the text holds n as a number: not inside a word or a timestamp (a fraction of a second follows a dot) either. A
// canary of digits alone searched with strings.Contains matched "4217" in a random id and in a timestamp; the number is searched as one.
func hasNumber(text string, n int) bool {
	return regexp.MustCompile(`(^|[^0-9A-Za-z.-])` + strconv.Itoa(n) + `($|[^0-9A-Za-z.-])`).MatchString(text)
}

// TestHasNumberFindsTheNumberAndNotATimestampHoldingIt: a timestamp or a word holding the digits is no leak, the number itself is.
func TestHasNumberFindsTheNumberAndNotATimestampHoldingIt(t *testing.T) {
	t.Parallel()
	for text, want := range map[string]bool{
		"at 2026-11-07T05:25:26.4217Z":    false,
		"abc4217def":                      false,
		`{"hp_temp":4217}`:                true,
		"the goblin has 4217 hit points":  true,
		"the goblin has 42170 hit points": false,
	} {
		if got := hasNumber(text, 4217); got != want {
			t.Errorf("hasNumber(%q, 4217) = %v, want %v", text, got, want)
		}
	}
}

// TestWithoutUUIDsKeepsTheNumberAndDropsTheID: an id holding the canary is not a leak, the bare number still is.
func TestWithoutUUIDsKeepsTheNumberAndDropsTheID(t *testing.T) {
	t.Parallel()
	id := "0b1c2d3e-4217-4a5b-8c9d-1e2f3a4b5c6d"
	if got := withoutUUIDs("combatant " + id + " changed"); strings.Contains(got, "4217") { // canary-ok: the helper's own test, on a text it has stripped
		t.Errorf("withoutUUIDs() = %q, want the id gone", got)
	}
	if got := withoutUUIDs("combatant " + id + " has 4217 hit points"); !strings.Contains(got, "4217") { // canary-ok: the helper's own test, on a text it has stripped
		t.Errorf("withoutUUIDs() = %q, want the bare number kept", got)
	}
}

// TestUndoRefusalNamesWhoChangedAndNoNumbers: the refusal carries the combatant, never a hit point number, and a
// player cannot ask for the undo at all (RN-10, RN-20): the master is the positive control.
func TestUndoRefusalNamesWhoChangedAndNoNumbers(t *testing.T) {
	t.Parallel()
	a := newArmed(t)
	e := a.threeAndAGoblin(t)
	if _, err := a.adjustHP(t, e, "Goblin", damageHP(2)); err != nil {
		t.Fatalf("AdjustCombatantHitPoints() error = %v", err)
	}
	last := a.log(t, a.master, e).GetUndoableEventId()
	const secretHP = 4217
	if _, err := a.h.pool.Exec(t.Context(), `UPDATE combatants SET hp_temp = $2 WHERE id = $1`, a.id(t, "Goblin"), secretHP); err != nil {
		t.Fatalf("change the hit points: %v", err)
	}
	// A player is refused before anything is read.
	err := a.undo(t, a.caio, e, last)
	wantCode(t, "UndoLastAction by a player", err, connect.CodePermissionDenied)
	if hasNumber(withoutUUIDs(err.Error()), secretHP) {
		t.Errorf("the player's refusal leaks the hit points: %v", err)
	}
	// The master's refusal names who and says no number.
	err = a.undo(t, a.master, e, last)
	wantCode(t, "UndoLastAction by the master", err, connect.CodeFailedPrecondition)
	b := blockedOf(t, err)
	if len(b.GetCombatantIds()) != 1 {
		t.Errorf("combatant_ids = %v, want the goblin", b.GetCombatantIds())
	}
	if raw := b.String() + err.Error(); hasNumber(withoutUUIDs(raw), secretHP) {
		t.Errorf("the master's refusal carries the hit points: %s", raw)
	}
}
