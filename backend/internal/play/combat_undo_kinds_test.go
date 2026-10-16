package play

import (
	"testing"

	playv1 "github.com/PuraFome/meuRPG/backend/gen/meurpg/play/v1"
)

// One test for each kind of event the master's Desfazer takes back (takeBack in
// combat_undo.go), each as the undo property of combat_undo_snapshot_test.go: the
// picture of everything the master and the players read, and the rows of the combat,
// before the action and after its undo. A scenario that names a difference
// (undoDelta) freezes what today's undo does not put back.

// TestUndoKindAttackRolled: the attack of the Attack action, with Extra Attack, a
// bonus action attack, an opportunity attack (a reaction) and a miss.
func TestUndoKindAttackRolled(t *testing.T) {
	t.Parallel()
	a := newCasters(t)
	e := a.castersFight(t, 1)
	w := newUndoWatch(t, a)
	a.passTo(t, e, "Toren")

	w.run(t, "a hit", nil, func() { a.mustAttack(t, a.caio, e, "Toren", battleaxe, "Goblin", d20(15)) })
	a.mustAttack(t, a.caio, e, "Toren", battleaxe, "Goblin", d20(15))
	w.run(t, "the second attack of Extra Attack", nil, func() {
		a.mustAttack(t, a.caio, e, "Toren", battleaxe, "Goblin", d20(15))
	})
	w.run(t, "a miss", nil, func() { a.mustAttack(t, a.caio, e, "Toren", battleaxe, "Goblin", d20(2)) })
	w.run(t, "an opportunity attack", nil, func() {
		if _, err := a.attackAs(t, a.master, e, "Brisa", maceKey, "Goblin", d20(15), true); err != nil {
			t.Fatalf("RollAttack(as_reaction) error = %v", err)
		}
	})
}

// TestUndoKindActionTaken: Retomar o fôlego (a heal), Surto de ação, the Dash and
// the Disengage.
func TestUndoKindActionTaken(t *testing.T) {
	t.Parallel()
	a := newCasters(t)
	e := a.castersFight(t, 1)
	w := newUndoWatch(t, a)
	a.passTo(t, e, "Toren")
	a.correct(t, a.toren, hpIs(20))

	w.run(t, "Retomar o fôlego", nil, func() {
		a.h.roller.queue(6)
		if _, err := a.feature(t, a.caio, e, "Toren", secondWindKey, takeInApp); err != nil {
			t.Fatalf("TakeAction() error = %v", err)
		}
	})
	w.run(t, "Surto de ação", nil, func() {
		if _, err := a.feature(t, a.caio, e, "Toren", actionSurgeKey, noTakeRoll); err != nil {
			t.Fatalf("TakeAction(surge) error = %v", err)
		}
	})
	w.run(t, "Disparada", nil, func() {
		if _, err := a.action(t, a.caio, e, "Toren", "standard:dash"); err != nil {
			t.Fatalf("TakeAction(dash) error = %v", err)
		}
	})
	w.run(t, "Desengajar", nil, func() {
		if _, err := a.action(t, a.caio, e, "Toren", "standard:disengage"); err != nil {
			t.Fatalf("TakeAction(disengage) error = %v", err)
		}
	})
}

// TestUndoKindHitPointsAdjusted: the master's damage and heal on an NPC.
func TestUndoKindHitPointsAdjusted(t *testing.T) {
	t.Parallel()
	a := newArmed(t)
	e := a.threeAndAGoblin(t)
	w := newUndoWatch(t, a)

	w.run(t, "damage on an NPC", nil, func() {
		if _, err := a.adjustHP(t, e, "Goblin", damageHP(3)); err != nil {
			t.Fatalf("AdjustCombatantHitPoints() error = %v", err)
		}
	})
	if _, err := a.adjustHP(t, e, "Goblin", damageHP(3)); err != nil {
		t.Fatalf("AdjustCombatantHitPoints() error = %v", err)
	}
	w.run(t, "a heal on an NPC", nil, func() {
		if _, err := a.adjustHP(t, e, "Goblin", healHP(2)); err != nil {
			t.Fatalf("AdjustCombatantHitPoints() error = %v", err)
		}
	})
}

// TestUndoKindDamageRolledAndApplied: the damage of an attack on an NPC (settled at
// once), on a character (waiting for the master), its application and its discard.
func TestUndoKindDamageRolledAndApplied(t *testing.T) {
	t.Parallel()
	a := newArmed(t)
	e := a.threeAndAGoblin(t)
	w := newUndoWatch(t, a)

	// Toren hurts the goblin: the damage lands at once.
	a.h.roller.queue(15)
	hit := a.mustAttack(t, a.caio, e, "Toren", battleaxe, "Goblin", inAppRoll)
	w.run(t, "the damage on an NPC", nil, func() {
		a.h.roller.queue(3)
		a.mustDamage(t, a.caio, e, hit.GetPendingDamage().GetId(), inAppDamage)
	})
	if _, err := a.settle(t, a.master, e, hit.GetPendingDamage().GetId(), false); err != nil { // the damage still waits: the master drops it
		t.Fatalf("DiscardPendingDamage() error = %v", err)
	}
	a.mustEndTurn(t, a.caio, e)
	a.mustEndTurn(t, a.ana, e)
	a.mustEndTurn(t, a.bia, e) // the goblin

	a.h.roller.queue(15)
	hit = a.mustAttack(t, a.master, e, "Goblin", sword, "Toren", inAppRoll)
	id := hit.GetPendingDamage().GetId()
	w.run(t, "the damage on a character", nil, func() {
		a.h.roller.queue(3)
		a.mustDamage(t, a.master, e, id, inAppDamage)
	})
	a.h.roller.queue(3)
	a.mustDamage(t, a.master, e, id, inAppDamage)
	w.run(t, "applying the damage on a character", nil, func() {
		if _, err := a.settle(t, a.master, e, id, true); err != nil {
			t.Fatalf("ApplyPendingDamage() error = %v", err)
		}
	})
	w.run(t, "discarding the damage", nil, func() {
		if _, err := a.settle(t, a.master, e, id, false); err != nil {
			t.Fatalf("DiscardPendingDamage() error = %v", err)
		}
	})
}

// TestUndoKindSpellCast: the spell with its slot, the concentration, the missiles,
// the area damage roll that settles a whole cast and a heal.
func TestUndoKindSpellCast(t *testing.T) {
	t.Parallel()
	a := newCasters(t)
	e := a.castersFight(t, 1)
	w := newUndoWatch(t, a)
	both := a.at(t, "Goblin", "Capitão Goblin")

	w.run(t, "Sono", nil, func() {
		a.mustCast(t, a.ana, e, "Pensantus", sleepSpell, slotOfLevel(1), a.at(t, "Goblin"), poolInApp)
	})
	w.run(t, "a concentration spell", nil, func() {
		a.mustCast(t, a.ana, e, "Pensantus", holdPerson, slotOfLevel(2), a.at(t, "Goblin"), noCastRoll)
	})
	w.run(t, "Mísseis Mágicos", nil, func() {
		a.mustCast(t, a.ana, e, "Pensantus", magicMissileSpell, slotOfLevel(1), []*playv1.SpellTarget{darts(a, t, "Goblin", 2), darts(a, t, "Capitão Goblin", 1)}, noCastRoll)
	})
	a.stand(t, map[string][2]int32{"Goblin": {6, 3}, "Capitão Goblin": {6, 2}})
	var cast *playv1.CastSpellResponse
	w.run(t, "Mãos Flamejantes", nil, func() {
		cast = a.mustCast(t, a.ana, e, "Pensantus", burningHands, slotOfLevel(1), both, noCastRoll)
	})
	cast = a.mustCast(t, a.ana, e, "Pensantus", burningHands, slotOfLevel(1), both, noCastRoll)
	w.run(t, "the damage roll of an area spell", nil, func() {
		a.h.roller.queue(3, 3, 3)
		a.mustDamage(t, a.ana, e, cast.GetCast().GetPendingDamages()[0].GetId(), inAppDamage)
	})
}

// TestUndoKindConditionsSet: conditions on a combatant, and the end of a concentration.
func TestUndoKindConditionsSet(t *testing.T) {
	t.Parallel()
	a := newCasters(t)
	e := a.castersFight(t, 1)
	w := newUndoWatch(t, a)
	a.mustCast(t, a.ana, e, "Pensantus", holdPerson, slotOfLevel(2), a.at(t, "Goblin"), noCastRoll)

	w.run(t, "ending the concentration", nil, func() {
		if _, err := a.conditions(t, a.ana, e, "Pensantus", nil, false, true); err != nil {
			t.Fatalf("SetCombatantConditions() error = %v", err)
		}
	})
	w.run(t, "the conditions", nil, func() {
		if _, err := a.conditions(t, a.master, e, "Goblin", []string{"condition:poisoned"}, true, false); err != nil {
			t.Fatalf("SetCombatantConditions() error = %v", err)
		}
	})
}

// TestUndoKindDeathSaveRolled: a death save and a natural 20 (which brings the
// character back with 1 hit point).
func TestUndoKindDeathSaveRolled(t *testing.T) {
	t.Parallel()
	a := newCasters(t)
	e := a.castersFight(t, 1)
	w := newUndoWatch(t, a)
	a.passTo(t, e, "Toren")
	a.correct(t, a.toren, hpIs(0))
	if _, err := a.endTurn(t, a.master, e, true); err != nil {
		t.Fatalf("EndTurn() error = %v", err)
	}
	a.passTo(t, e, "Toren")

	w.run(t, "a death save", nil, func() {
		if _, err := a.deathSave(t, a.caio, e, "Toren", deathFace(12)); err != nil {
			t.Fatalf("RollDeathSave() error = %v", err)
		}
	})
	w.run(t, "a natural 20", nil, func() {
		if _, err := a.deathSave(t, a.caio, e, "Toren", deathFace(20)); err != nil {
			t.Fatalf("RollDeathSave(20) error = %v", err)
		}
	})
}

// TestUndoKindHealOnACharacterAtZero: a heal that reaches a character at 0 hit
// points gives back the hit points, and the undo the death saves too.
func TestUndoKindHealOnACharacterAtZero(t *testing.T) {
	t.Parallel()
	a := newCasters(t)
	e := a.castersFight(t, 1)
	w := newUndoWatch(t, a)
	a.passTo(t, e, "Toren")
	a.correct(t, a.toren, hpIs(0))
	if _, err := a.endTurn(t, a.master, e, true); err != nil {
		t.Fatalf("EndTurn() error = %v", err)
	}
	a.passTo(t, e, "Toren")
	if _, err := a.deathSave(t, a.caio, e, "Toren", deathFace(1)); err != nil {
		t.Fatalf("RollDeathSave(1) error = %v", err)
	}
	a.passTo(t, e, "Brisa")
	var healed *playv1.CastSpellResponse
	w.run(t, "Curar Ferimentos", nil, func() {
		healed = a.mustCast(t, a.bia, e, "Brisa", cureWounds, slotOfLevel(1), a.at(t, "Toren"), noCastRoll)
	})
	healed = a.mustCast(t, a.bia, e, "Brisa", cureWounds, slotOfLevel(1), a.at(t, "Toren"), noCastRoll)
	w.run(t, "the heal's roll", nil, func() {
		a.h.roller.queue(4)
		a.mustDamage(t, a.bia, e, healed.GetCast().GetPendingDamages()[0].GetId(), inAppDamage)
	})
}

// TestUndoKindReactions: Escudo used on a hit, the same reaction declined.
func TestUndoKindReactions(t *testing.T) {
	t.Parallel()
	a := newCasters(t)
	e := a.castersFightNPCFirst(t)
	w := newUndoWatch(t, a)
	hit := a.mustAttack(t, a.master, e, "Capitão Goblin", sword, "Pensantus", d20(9))

	w.run(t, "Escudo", nil, func() {
		if _, err := a.useReaction(t, a.ana, e, hit.GetPendingDamage().GetId(), slotOfLevel(1)); err != nil {
			t.Fatalf("UseReaction() error = %v", err)
		}
	})
	w.run(t, "declining Escudo", nil, func() {
		if _, err := a.declineReaction(t, a.ana, e, hit.GetPendingDamage().GetId()); err != nil {
			t.Fatalf("DeclineReaction() error = %v", err)
		}
	})
}

// Three things the undo used to leave behind, each with a test of its own beside the
// pictures above: the melee hit mark of the turn, the empty parts list of a damage and
// the "ended" line of an effect the change ended.

// gripOffered says whether Pensantus's turn options offer the grapple maneuver (m.grip) as
// usable now, and checks that a refusal says why: no melee hit this turn.
func (m *maneuverTable) gripOffered(t *testing.T, e *playv1.Encounter, when string) bool {
	t.Helper()
	grapple := grappleOption(m.mustOptions(t, m.ana, e, "Pensantus"), playv1.ContestAttackOptionKind_CONTEST_ATTACK_OPTION_KIND_GRAPPLE)
	for _, o := range grapple.GetManeuvers() {
		if o.GetKey() != m.grip {
			continue
		}
		if o.GetEnabled() == o.GetNoMeleeHit() {
			t.Fatalf("%s: the grapple maneuver is enabled %v with no_melee_hit %v, want one or the other", when, o.GetEnabled(), o.GetNoMeleeHit())
		}
		return o.GetEnabled()
	}
	t.Fatalf("%s: the grapple options %v do not list the grapple maneuver", when, grapple)
	return false
}

// TestUndoOfAMeleeHitTakesItsMarkOff: a melee weapon hit marks the turn (markMeleeHit), and
// the grapple maneuver needs that mark. The master's undo of the hit takes the mark off,
// so the maneuver is no longer offered and its grapple is refused. With two hits in the
// turn (Extra Attack), the undo of the second puts back the first one's mark, and the undo
// of the first then takes it off.
func TestUndoOfAMeleeHitTakesItsMarkOff(t *testing.T) {
	t.Parallel()
	m := newManeuverTable(t, "4")
	e := m.passTo(t, m.castersFight(t, 1), "Pensantus")
	if m.gripOffered(t, e, "before any hit") {
		t.Fatal("the grapple maneuver is offered before any melee hit this turn")
	}

	m.hit(t, e)
	if !m.gripOffered(t, e, "after a hit") {
		t.Fatal("the grapple maneuver is not offered after a melee hit")
	}
	m.undoLast(t, e)
	if m.gripOffered(t, e, "after the undo of the hit") {
		t.Error("the grapple maneuver is still offered after the master undid the only melee hit of the turn")
	}
	_, err := m.grapple(t, e, "Capitão Goblin", nil)
	wantBlockedBy(t, "StartContest(the grapple maneuver) after the undo of the hit", err, playv1.EncounterBlockedReason_ENCOUNTER_BLOCKED_REASON_NO_MELEE_HIT)

	m.hit(t, e)
	m.hit(t, e) // the second attack of the action (Extra Attack)
	m.undoLast(t, e)
	if !m.gripOffered(t, e, "after the undo of the second hit") {
		t.Error("the undo of the second hit took off the mark of the first, which still stands")
	}
	m.undoLast(t, e)
	if m.gripOffered(t, e, "after the undo of both hits") {
		t.Error("the grapple maneuver is still offered after the master undid both melee hits of the turn")
	}
}

// TestUndoOfADamageRollGivesThePartsBack: a weapon with no extra hits with an empty list of
// parts, and its damage roll on an NPC (which takes it at once and so keeps the hit points
// it had) leaves that list as it is: the undo of the roll finds the empty list, never a
// JSON null. A roll made before that wrote a JSON null over the list, and the undo of
// such a roll gives the empty list back.
func TestUndoOfADamageRollGivesThePartsBack(t *testing.T) {
	t.Parallel()
	a := newArmed(t)
	e := a.threeAndAGoblin(t)
	a.h.roller.queue(15)
	id := a.mustAttack(t, a.caio, e, "Toren", battleaxe, "Goblin", inAppRoll).GetPendingDamage().GetId()
	parts := func(when string) string {
		t.Helper()
		var got string
		if err := a.h.pool.QueryRow(t.Context(), `SELECT parts::STRING FROM pending_damages WHERE id = $1`, id).Scan(&got); err != nil {
			t.Fatalf("%s: read the parts: %v", when, err)
		}
		return got
	}
	if got := parts("after the hit"); got != "[]" {
		t.Fatalf("the hit's parts = %s, want [] (a weapon with no extra)", got)
	}

	a.h.roller.queue(3)
	a.mustDamage(t, a.caio, e, id, inAppDamage)
	if got := parts("after the roll"); got != "[]" {
		t.Errorf("after the roll the parts = %s, want the [] the hit left", got)
	}
	a.undoLast(t, e)
	if got := parts("after the undo"); got != "[]" {
		t.Errorf("after the undo of the roll the parts = %s, want []", got)
	}

	a.h.roller.queue(3)
	a.mustDamage(t, a.caio, e, id, inAppDamage)
	if _, err := a.h.pool.Exec(t.Context(), `UPDATE pending_damages SET parts = 'null'::JSONB WHERE id = $1`, id); err != nil {
		t.Fatalf("write the JSON null an older roll left: %v", err)
	}
	a.undoLast(t, e)
	if got := parts("after the undo of an older roll"); got != "[]" {
		t.Errorf("after the undo of a roll that left a JSON null the parts = %s, want []", got)
	}
}

// endedLines counts the "ended" lines of lasting effects in u's combat log.
func (a *armed) endedLines(t *testing.T, u *user, e *playv1.Encounter) int {
	t.Helper()
	n := 0
	for _, r := range a.log(t, u, e).GetRounds() {
		for _, entry := range r.GetEntries() {
			if entry.GetEffect().GetChange() == playv1.CombatLogEffectChange_COMBAT_LOG_EFFECT_CHANGE_ENDED {
				n++
			}
		}
	}
	return n
}

// TestUndoOfAnEndedConcentrationTakesTheEndedLineBack: ending a concentration writes the
// "ended" line of the effect it held (Hold Person on the Goblin) before the change's own
// event. The master's undo of the change puts the effect back and takes that line back
// with it: no log, the master's or a player's, still says the effect ended, and the next
// undo is the cast's.
func TestUndoOfAnEndedConcentrationTakesTheEndedLineBack(t *testing.T) {
	t.Parallel()
	a := newCasters(t)
	e := a.castersFight(t, 1)
	a.mustCast(t, a.ana, e, "Pensantus", holdPerson, slotOfLevel(2), a.at(t, "Goblin"), noCastRoll)
	cast := a.log(t, a.master, e).GetUndoableEventId()
	if cast == "" {
		t.Fatal("the cast is not the master's to undo")
	}
	if _, err := a.conditions(t, a.ana, e, "Pensantus", nil, false, true); err != nil {
		t.Fatalf("SetCombatantConditions(end the concentration) error = %v", err)
	}
	readers := []struct {
		name string
		u    *user
	}{{"master", a.master}, {"ana", a.ana}, {"bia", a.bia}, {"caio", a.caio}}
	for _, r := range readers { // the line is there to take back: everyone reads it
		if n := a.endedLines(t, r.u, e); n != 1 {
			t.Fatalf("%s's log has %d ended lines after the end of the concentration, want 1", r.name, n)
		}
	}

	a.undoLast(t, e)
	for _, r := range readers {
		if n := a.endedLines(t, r.u, e); n != 0 {
			t.Errorf("%s's log has %d ended lines after the undo, want none: the effect is back", r.name, n)
		}
	}
	if got := byLabel(t, a.get(t, a.master), "Pensantus").GetConcentrationSpell(); got != holdPerson {
		t.Errorf("Pensantus concentrates on %q after the undo, want %q", got, holdPerson)
	}
	if got := a.log(t, a.master, e).GetUndoableEventId(); got != cast {
		t.Fatalf("after the undo the master may undo %q, want the cast %q", got, cast)
	}
	if err := a.undo(t, a.master, e, cast); err != nil {
		t.Fatalf("UndoLastAction(the cast) error = %v", err)
	}
	if got := byLabel(t, a.get(t, a.master), "Pensantus").GetConcentrationSpell(); got != "" {
		t.Errorf("Pensantus concentrates on %q after the undo of the cast, want nothing", got)
	}
}
