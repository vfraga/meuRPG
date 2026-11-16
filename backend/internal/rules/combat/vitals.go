package combat

// DamageResult is a creature's hit points after damage.
type DamageResult struct {
	HP, TempHP int
	// Absorbed is the part the temporary hit points took.
	Absorbed int
	// FellToZero says the creature was above 0 and is now at 0.
	FellToZero bool
	// Excess is the damage left over after reaching 0. A hit with Excess of
	// at least the maximum hit points kills a character outright, which the
	// master decides (RN-03).
	Excess int
}

// ApplyDamage takes damage from the temporary hit points first, then from
// the hit points, never below 0. A negative amount counts as 0.
func ApplyDamage(hp, tempHP, amount int) DamageResult {
	amount = max(amount, 0)
	r := DamageResult{HP: hp, TempHP: tempHP}
	r.Absorbed = min(amount, tempHP)
	r.TempHP -= r.Absorbed
	amount -= r.Absorbed
	r.HP = max(hp-amount, 0)
	r.Excess = max(amount-hp, 0)
	r.FellToZero = hp > 0 && r.HP == 0
	return r
}

// HealResult is a creature's hit points after healing.
type HealResult struct {
	HP int
	// Healed is how many points were really restored, after the cap.
	Healed int
}

// ApplyHeal restores hit points up to the maximum. A negative amount counts
// as 0.
func ApplyHeal(hp, maxHP, amount int) HealResult {
	after := min(hp+max(amount, 0), max(maxHP, hp))
	return HealResult{HP: after, Healed: after - hp}
}

// TempHPResult is a creature's temporary hit points after it is given some.
type TempHPResult struct {
	// TempHP is what the creature has after, Gained how many it gained.
	TempHP, Gained int
}

// ApplyTempHP gives temporary hit points: they do not stack, so the creature keeps
// the larger of what it had and amount (SRD 5.1, "Temporary Hit Points"). A negative
// amount counts as 0.
func ApplyTempHP(current, amount int) TempHPResult {
	after := max(current, amount, 0)
	return TempHPResult{TempHP: after, Gained: after - max(current, 0)}
}

// Death save outcomes.
const (
	// DeathSaveContinues: still making saves.
	DeathSaveContinues = "continues"
	// DeathSaveStable: three successes; the character stops rolling and
	// stays at 0 hit points.
	DeathSaveStable = "stable"
	// DeathSaveDying: three failures. The master confirms the death
	// (RN-03); the engine never kills a character by itself.
	DeathSaveDying = "dying"
	// DeathSaveRevived: a natural 20; the character is back with 1 hit
	// point and both counts reset.
	DeathSaveRevived = "revived"
)

// DeathSaveResult is the state after a death save or damage at 0.
type DeathSaveResult struct {
	Successes, Failures int
	// Outcome is one of the DeathSave* constants.
	Outcome string
	// HP is 1 for DeathSaveRevived, otherwise 0.
	HP int
}

// DeathSaveGoal is how many death save successes make a character stable and how many failures end it (SRD 5.1, RN-03): the one
// number the combat, its view, the lists of receivers and the rules of healing read.
const DeathSaveGoal = 3

// DeathSaveDC is the number a death save's roll reaches to be a success (SRD 5.1).
const DeathSaveDC = 10

// DeathSave applies one death save roll (d20Face, 1 to 20): 10 or more is a
// success, less than 10 a failure, a natural 1 two failures, a natural 20
// brings the character back with 1 hit point.
func DeathSave(d20Face, successes, failures int) DeathSaveResult {
	return DeathSaveWith(d20Face, 0, successes, failures)
}

// DeathSaveWith is DeathSave with a number added to the d20, the Bardic Inspiration die (SRD 5.1): the total decides
// a success or a failure, but a natural 1 is still two failures and a natural 20 still brings the character back.
func DeathSaveWith(d20Face, bonus, successes, failures int) DeathSaveResult {
	switch {
	case d20Face == 20:
		return DeathSaveResult{Outcome: DeathSaveRevived, HP: 1}
	case d20Face == 1:
		return tally(successes, failures+2)
	case d20Face+bonus >= DeathSaveDC:
		return tally(successes+1, failures)
	}
	return tally(successes, failures+1)
}

// DamageWhileDown is how many death save failures damage causes to a
// character at 0 hit points: 1, or 2 for a critical hit. The caller adds
// them with AddFailures.
func DamageWhileDown(critical bool) int {
	if critical {
		return 2
	}
	return 1
}

// AddFailures adds death save failures (from DamageWhileDown).
func AddFailures(successes, failures, n int) DeathSaveResult {
	return tally(successes, failures+n)
}

// tally caps both counts at 3 and names the state. A character that becomes
// stable has both counts back at zero (SRD 5.1): three successes stay as the
// marker of the stable state, and the failures are cleared, so the next hit
// starts from one failure.
func tally(successes, failures int) DeathSaveResult {
	r := DeathSaveResult{Successes: min(successes, DeathSaveGoal), Failures: min(failures, DeathSaveGoal), Outcome: DeathSaveContinues}
	switch {
	case r.Failures >= DeathSaveGoal:
		r.Outcome = DeathSaveDying
	case r.Successes >= DeathSaveGoal:
		r.Outcome, r.Failures = DeathSaveStable, 0
	}
	return r
}
