package play

import (
	"slices"
	"testing"

	playv1 "github.com/PuraFome/meuRPG/backend/gen/meurpg/play/v1"
)

// One test for each kind of event the master's Desfazer takes back (takeBack in
// combat_undo.go), each as the undo property of combat_undo_snapshot_test.go: the
// picture of everything the master and the players read, and the rows of the combat,
// before the action and after its undo. A scenario that names a difference
// (undoDelta) freezes what today's undo does not put back.

// What today's undo leaves different, frozen so the code can be reorganized without
// moving it. Each is a finding of the report, not a decision: the day one is fixed,
// its scenarios stop failing here and lose the delta.
var (
	// A melee weapon hit marks the turn on the attacker (markMeleeHit, combat_maneuvers.go),
	// and nothing takes the mark off: after the undo of the hit a maneuver that starts a
	// grapple is still offered.
	deltaMeleeHit = undoDelta{"+ db/combatants[#].melee_hit_turn=1:#", "- db/combatants[#].melee_hit_turn=<nil>"}
	// ClearPendingDamageRoll (queries.sql) puts the roll's columns back but not `parts`,
	// which the roll wrote as NULL where the damage was made with [].
	deltaPartsNull = undoDelta{"+ db/pending_damages[#].parts=<nil>", "- db/pending_damages[#].parts=[]"}
)

// deltaEffectEndedLine is the log line of a lasting effect that ended (the master's
// conditions took the held condition off the Goblin, or its caster's concentration
// ended). The line is an event of its own, written before the change's, and the undo
// restores the effect without taking it away: every log still says the effect ended.
func deltaEffectEndedLine(reason string) undoDelta {
	var out undoDelta
	for _, who := range []string{"ana", "bia", "caio", "master"} {
		out = append(out, "+ "+who+"/log R1 EFFECT id=# Pensantus>Goblin effect=COMBAT_LOG_EFFECT_CHANGE_ENDED/"+reason)
	}
	return out
}

// deltaCastNotUndoable: the line above also closes the undo chain, so the cast before it
// is no longer the master's to take back.
var deltaCastNotUndoable = undoDelta{
	"+ master/log R1 SPELL_CAST id=# Pensantus> key=spell:hold-person", "+ master/log.undoable=",
	"- master/log R1 SPELL_CAST id=# Pensantus> key=spell:hold-person undoable", "- master/log.undoable=#",
}

// TestUndoKindAttackRolled: the attack of the Attack action, with Extra Attack, a
// bonus action attack, an opportunity attack (a reaction) and a miss.
func TestUndoKindAttackRolled(t *testing.T) {
	t.Parallel()
	a := newCasters(t)
	e := a.castersFight(t, 1)
	w := newUndoWatch(t, a)
	a.passTo(t, e, "Toren")

	w.run(t, "a hit", deltaMeleeHit, func() { a.mustAttack(t, a.caio, e, "Toren", battleaxe, "Goblin", d20(15)) })
	a.mustAttack(t, a.caio, e, "Toren", battleaxe, "Goblin", d20(15))
	w.run(t, "the second attack of Extra Attack", nil, func() {
		a.mustAttack(t, a.caio, e, "Toren", battleaxe, "Goblin", d20(15))
	})
	w.run(t, "a miss", nil, func() { a.mustAttack(t, a.caio, e, "Toren", battleaxe, "Goblin", d20(2)) })
	w.run(t, "an opportunity attack", deltaMeleeHit, func() {
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
	w.run(t, "the damage on an NPC", deltaPartsNull, func() {
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
	w.run(t, "the damage roll of an area spell", slices.Concat(deltaPartsNull, deltaPartsNull), func() {
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

	w.run(t, "ending the concentration", slices.Concat(deltaEffectEndedLine("concentration"), deltaCastNotUndoable), func() {
		if _, err := a.conditions(t, a.ana, e, "Pensantus", nil, false, true); err != nil {
			t.Fatalf("SetCombatantConditions() error = %v", err)
		}
	})
	w.run(t, "the conditions", deltaEffectEndedLine("ended_by_master"), func() {
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
	w.run(t, "the heal's roll", deltaPartsNull, func() {
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
