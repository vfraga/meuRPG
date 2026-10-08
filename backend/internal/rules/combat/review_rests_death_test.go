package combat

import "testing"

// SRD 5.1, "Death Saving Throws": the successes and the failures both go back
// to zero when the creature becomes stable (third success).
func TestRulesReviewRests_StableResetsTheCounts(t *testing.T) {
	r := DeathSave(15, 2, 2)
	if r.Outcome != DeathSaveStable {
		t.Fatalf("outcome = %q, want stable", r.Outcome)
	}
	if r.Failures != 0 {
		t.Errorf("failures after becoming stable = %d, want 0 (the SRD zeroes both counts)", r.Failures)
	}

	// What failuresWhileDown does for a hit on the stable character: successes
	// reset to 0 (they were 3), failures carried from the stored result.
	hit := AddFailures(0, r.Failures, DamageWhileDown(false))
	if hit.Failures != 1 {
		t.Errorf("failures after the first hit on a stable character = %d, want 1", hit.Failures)
	}
	if hit.Outcome == DeathSaveDying {
		t.Errorf("a stable character that was zeroed must not be dying from one hit")
	}
}
