package play

import (
	"strings"
	"testing"

	"connectrpc.com/connect"

	playv1 "github.com/PuraFome/meuRPG/backend/gen/meurpg/play/v1"
)

// A readied spell (ready_spell.go): Ready casts it at once (its slot, its concentration, the concentration it ends), its
// release is the cast as a reaction, and it is lost, with its slot, when the concentration ends before it.

const (
	blockedReadySpellTime = playv1.EncounterBlockedReason_ENCOUNTER_BLOCKED_REASON_READY_SPELL_TIME
	readySpellTrigger     = "Se o xamã erguer o cajado"
)

// readySpell is a request that readies the caster's spell with a slot of the level (0 for a cantrip).
func (a *armed) readySpell(t *testing.T, e *playv1.Encounter, caster, spell string, slot int32, target string) *playv1.ReadyActionRequest {
	t.Helper()
	req := &playv1.ReadyActionRequest{
		CampaignId: a.campaignID, EncounterId: e.GetId(), CombatantId: a.id(t, caster), IdempotencyKey: newKey(),
		TriggerText: readySpellTrigger, Kind: playv1.ReadiedKind_READIED_KIND_SPELL, SpellKey: spell, SlotLevel: slot,
	}
	if target != "" {
		req.TargetCombatantId = a.id(t, target)
	}
	return req
}

// useSpellAnswer is "Usar" on a readied spell, with the app's dice.
func useSpellAnswer(r *playv1.AnswerReactionRequest) {
	r.Answer = playv1.ReactionChoice_REACTION_CHOICE_USE
}

// concentrationAs is the combatant's concentration as u reads it.
func (a *armed) concentrationAs(t *testing.T, u *user, label string) string {
	t.Helper()
	return byLabel(t, a.get(t, u), label).GetConcentrationSpell()
}

// slotsUsed is how many slots of the level the character spent.
func (a *armed) slotsUsed(t *testing.T, label string, level int32) int32 {
	t.Helper()
	switch label {
	case "Pensantus":
		return usedSlots(a.vitals(t, a.pens), level)
	case "Brisa":
		return usedSlots(a.vitals(t, a.bri), level)
	}
	t.Fatalf("no vitals for %s", label)
	return 0
}

// heldEffects counts the lasting effects of a spell in the combat.
func (a *armed) heldEffects(t *testing.T, spell string) int {
	t.Helper()
	var n int
	if err := a.h.pool.QueryRow(t.Context(), `SELECT count(*) FROM combatant_states WHERE source_key = $1`, spell).Scan(&n); err != nil {
		t.Fatalf("count the effects: %v", err)
	}
	return n
}

// Pensantus holds Hold Person on the Capitão; on his next turn he readies Magic Missile. Ready casts it now: the 1st-level
// slot is spent, the action too, Hold Person ends (its paralysis goes) and the readied spell is his concentration. Ana
// and the master read the held spell as his concentration and in the row, and the line that says Hold Person ended; Caio
// reads no concentration and the line with the actor alone. Nothing of the spell happened: no damage, no effect.
func TestReadySpellCastsItNowAndHoldsItWithConcentration(t *testing.T) {
	t.Parallel()
	a := newCasters(t)
	e := a.castersFight(t, 1)
	a.h.roller.queue(1) // the Capitão fails its save
	a.mustCast(t, a.ana, e, "Pensantus", holdPerson, slotOfLevel(2), a.at(t, "Capitão Goblin"), noCastRoll)
	if a.heldEffects(t, holdPerson) == 0 {
		t.Fatal("Hold Person took no hold of the Capitão")
	}
	e = a.nextTurnOf(t, e, "Pensantus")
	first := a.slotsUsed(t, "Pensantus", 1)

	a.mustReady(t, a.ana, a.readySpell(t, e, "Pensantus", magicMissileSpell, 1, "Goblin"))

	if got := a.slotsUsed(t, "Pensantus", 1); got != first+1 {
		t.Errorf("1st-level slots used = %d, want %d: Ready spends the slot", got, first+1)
	}
	if a.heldEffects(t, holdPerson) != 0 {
		t.Error("Hold Person's paralysis outlived the concentration Ready ended")
	}
	for _, u := range []*user{a.ana, a.master} {
		pens := byLabel(t, a.get(t, u), "Pensantus")
		if !pens.GetActionUsed() || pens.GetConcentrationSpell() != magicMissileSpell || pens.GetReadied().GetKind() != playv1.ReadiedKind_READIED_KIND_SPELL ||
			pens.GetReadied().GetActionKey() != magicMissileSpell {
			t.Errorf("Pensantus as %s: action used %v, concentration %q, readied %v", u.id, pens.GetActionUsed(), pens.GetConcentrationSpell(), pens.GetReadied())
		}
		lines := a.readiedLines(t, u, e, playv1.CombatLogKind_COMBAT_LOG_KIND_ACTION_READIED)
		if len(lines) != 1 || lines[0].GetSpell().GetSlot().GetLevel() != 1 || lines[0].GetConcentrationEndedKey() != holdPerson || lines[0].GetReadied().GetActionKey() != magicMissileSpell {
			t.Errorf("the readied lines of %s = %v", u.id, lines)
		}
	}
	if got := a.concentrationAs(t, a.caio, "Pensantus"); got != "" {
		t.Errorf("Caio reads Pensantus's concentration %q, want none", got)
	}
	lines := a.readiedLines(t, a.caio, e, playv1.CombatLogKind_COMBAT_LOG_KIND_ACTION_READIED)
	if len(lines) != 1 || lines[0].GetSpell() != nil || lines[0].GetReadied() != nil || lines[0].GetConcentrationEndedKey() != "" {
		t.Errorf("Caio's readied lines = %v, want the actor alone (RN-10: not the concentration it ended)", lines)
	}
	var slot int32
	if err := a.h.pool.QueryRow(t.Context(), `SELECT slot_level FROM readied_actions`).Scan(&slot); err != nil || slot != 1 {
		t.Errorf("the row's slot level = %d, %v; want 1", slot, err)
	}
	if hp := byLabel(t, a.get(t, a.master), "Goblin").GetHitPointsCurrent(); hp != 7 {
		t.Errorf("the goblin has %d hit points: the held spell hit it", hp)
	}
}

// What Ready refuses of a spell, writing nothing: a casting time that is not 1 action (a reaction, a bonus action), a
// spell the caster has not, a slot that does not fit, an attack cantrip (RollAttack's), no free slot. A save cantrip
// needs no slot and holds the concentration all the same.
func TestReadySpellRefusals(t *testing.T) {
	t.Parallel()
	a := newCasters(t)
	e := a.castersFight(t, 1)
	before := a.events(t)
	_, err := a.ready(t, a.ana, a.readySpell(t, e, "Pensantus", shieldSpell, 1, ""))
	wantBlockedBy(t, "a reaction spell", err, blockedReadySpellTime)
	for _, c := range []struct {
		name  string
		spell string
		slot  int32
	}{
		{"a spell he has not", "spell:fireball", 3},
		{"a 1st-level spell with no slot", magicMissileSpell, 0},
		{"a slot he has not", magicMissileSpell, 3},
		{"an attack cantrip with a slot", fireBolt, 1},
	} {
		_, err := a.ready(t, a.ana, a.readySpell(t, e, "Pensantus", c.spell, c.slot, ""))
		wantCode(t, c.name, err, connect.CodeInvalidArgument)
	}
	if n := a.events(t); n != before {
		t.Fatalf("the refusals wrote %d events, want %d", n, before)
	}
	a.correct(t, a.pens, func(r *playv1.AdjustCharacterVitalsRequest) {
		r.SpellSlotsUsed = []*playv1.SpellSlotsUsed{{Level: 2, Used: 2}}
	})
	before = a.events(t)
	_, err = a.ready(t, a.ana, a.readySpell(t, e, "Pensantus", holdPerson, 2, ""))
	wantBlockedBy(t, "Hold Person with no 2nd-level slot", err, blockedNoSlot)
	if n := a.events(t); n != before || a.readiedStatus(t, "Pensantus") != "" || byLabel(t, a.get(t, a.master), "Pensantus").GetActionUsed() {
		t.Fatalf("the refusals wrote: events %d (want %d), rows %q", n, before, a.readiedStatus(t, "Pensantus"))
	}

	e = a.passTo(t, e, "Brisa")
	_, err = a.ready(t, a.bia, a.readySpell(t, e, "Brisa", healingWord, 1, ""))
	wantBlockedBy(t, "a bonus action spell", err, blockedReadySpellTime)
	slots := a.slotsUsed(t, "Brisa", 1)
	a.mustReady(t, a.bia, a.readySpell(t, e, "Brisa", sacredFlame, 0, "Goblin"))
	if got := a.concentrationAs(t, a.bia, "Brisa"); got != sacredFlame || a.slotsUsed(t, "Brisa", 1) != slots {
		t.Errorf("Brisa's readied cantrip: concentration %q, 1st-level slots %d (want %d)", got, a.slotsUsed(t, "Brisa", 1), slots)
	}
}

// A readied spell is cast at Ready, not when it is released: Web lays no area while it waits. (On a map Ready refuses it,
// so the fight is one with no map.)
func TestReadiedSpellLaysNoArea(t *testing.T) {
	t.Parallel()
	a := newCasters(t)
	e := a.theatreFight(t)
	a.mustReady(t, a.ana, a.readySpell(t, e, "Pensantus", webSpell, 2, ""))
	var zones int
	if err := a.h.pool.QueryRow(t.Context(), `SELECT count(*) FROM spell_zones`).Scan(&zones); err != nil || zones != 0 {
		t.Errorf("spell zones after a readied Web = %d, %v; want none", zones, err)
	}
	if lasting := a.heldEffects(t, webSpell); lasting != 0 {
		t.Errorf("a readied Web left %d effects", lasting)
	}
}

// readyHoldPerson has Pensantus ready Hold Person on the Capitão and ends his turn: Toren acts.
func (a *armed) readyHoldPerson(t *testing.T) *playv1.Encounter {
	t.Helper()
	e := a.castersFight(t, 1)
	a.mustReady(t, a.ana, a.readySpell(t, e, "Pensantus", holdPerson, 2, "Capitão Goblin"))
	return a.mustEndTurn(t, a.master, e)
}

// "Usar" releases Hold Person: the cast is Pensantus's reaction, the Capitão saves, the paralysis holds, the slot is not
// spent again and the concentration is now the spell's own, which every player reads. Its undo makes the row wait with
// the window open and the spell held. "Deixar passar" then loses the spell: the concentration ends, the slot stays
// spent, the lost line says so; its undo holds the spell again.
func TestReleaseReadiedSaveSpellAndPassIt(t *testing.T) {
	t.Parallel()
	a := newCasters(t)
	e := a.readyHoldPerson(t)
	slots := a.slotsUsed(t, "Pensantus", 2)
	w := a.mustFire(t, e, "Pensantus")
	a.h.roller.queue(1)
	res := a.mustAnswer(t, a.ana, e, w.GetId(), useSpellAnswer)
	if !res.GetResult().GetUsed() {
		t.Errorf("the use's result = %v", res.GetResult())
	}
	pens := byLabel(t, a.get(t, a.ana), "Pensantus")
	if !pens.GetReactionUsed() || pens.GetConcentrationSpell() != holdPerson || a.readiedStatus(t, "Pensantus") != readiedFired || a.readiedOpen(t, "Pensantus") {
		t.Errorf("after the release: reaction %v, concentration %q, rows %q", pens.GetReactionUsed(), pens.GetConcentrationSpell(), a.readiedStatus(t, "Pensantus"))
	}
	if a.heldEffects(t, holdPerson) == 0 || a.slotsUsed(t, "Pensantus", 2) != slots {
		t.Errorf("after the release: effects %d, 2nd-level slots %d (want %d)", a.heldEffects(t, holdPerson), a.slotsUsed(t, "Pensantus", 2), slots)
	}

	a.mustUndo(t, e)
	pens = byLabel(t, a.get(t, a.master), "Pensantus")
	if pens.GetReactionUsed() || pens.GetConcentrationSpell() != holdPerson || a.readiedStatus(t, "Pensantus") != readiedWaiting || !a.readiedOpen(t, "Pensantus") {
		t.Errorf("after the undo: reaction %v, concentration %q, rows %q", pens.GetReactionUsed(), pens.GetConcentrationSpell(), a.readiedStatus(t, "Pensantus"))
	}
	if a.heldEffects(t, holdPerson) != 0 || a.slotsUsed(t, "Pensantus", 2) != slots || a.concentrationAs(t, a.caio, "Pensantus") != "" {
		t.Errorf("after the undo: effects %d, slots %d (want %d), Caio reads %q", a.heldEffects(t, holdPerson), a.slotsUsed(t, "Pensantus", 2), slots, a.concentrationAs(t, a.caio, "Pensantus"))
	}

	a.mustAnswer(t, a.ana, e, a.windowOf(t, a.master, kindReadied).GetId(), passAnswer)
	if got := a.concentrationAs(t, a.ana, "Pensantus"); got != "" || a.readiedStatus(t, "Pensantus") != readiedIgnored || a.slotsUsed(t, "Pensantus", 2) != slots {
		t.Errorf("after the pass: concentration %q, rows %q, slots %d", got, a.readiedStatus(t, "Pensantus"), a.slotsUsed(t, "Pensantus", 2))
	}
	lost := a.readiedLines(t, a.ana, e, playv1.CombatLogKind_COMBAT_LOG_KIND_READIED_LOST)
	if len(lost) != 1 || lost[0].GetReadiedLost().GetKind() != playv1.ReadiedKind_READIED_KIND_SPELL || lost[0].GetReadiedLost().GetActionKey() != holdPerson {
		t.Errorf("Ana's lost lines = %v", lost)
	}
	if lost := a.readiedLines(t, a.caio, e, playv1.CombatLogKind_COMBAT_LOG_KIND_READIED_LOST); len(lost) != 1 || lost[0].GetReadiedLost() != nil {
		t.Errorf("Caio's lost lines = %v, want one with the actor alone", lost)
	}
	a.mustUndo(t, e)
	if got := a.concentrationAs(t, a.ana, "Pensantus"); got != holdPerson || a.readiedStatus(t, "Pensantus") != readiedWaiting {
		t.Errorf("after the undo of the pass: concentration %q, rows %q", got, a.readiedStatus(t, "Pensantus"))
	}
	if lost := a.readiedLines(t, a.ana, e, playv1.CombatLogKind_COMBAT_LOG_KIND_READIED_LOST); len(lost) != 0 {
		t.Errorf("the lost line outlived the undo of the pass: %v", lost)
	}
}

// Brisa readies Guiding Bolt: its release rolls the spell attack as her reaction, and the concentration it held ends
// (the spell needs none).
func TestReleaseReadiedAttackSpell(t *testing.T) {
	t.Parallel()
	a := newCasters(t)
	e := a.castersFight(t, 1)
	e = a.passTo(t, e, "Brisa")
	a.mustReady(t, a.bia, a.readySpell(t, e, "Brisa", guidingBolt, 1, "Capitão Goblin"))
	e = a.mustEndTurn(t, a.master, e)
	w := a.mustFire(t, e, "Brisa")
	a.h.roller.queue(19, 3, 3, 3, 3)
	res := a.mustAnswer(t, a.bia, e, w.GetId(), useSpellAnswer)
	if !res.GetResult().GetUsed() {
		t.Fatalf("the use's result = %v", res.GetResult())
	}
	brisa := byLabel(t, a.get(t, a.bia), "Brisa")
	if !brisa.GetReactionUsed() || brisa.GetConcentrationSpell() != "" || a.readiedStatus(t, "Brisa") != readiedFired {
		t.Errorf("after the release: reaction %v, concentration %q, rows %q", brisa.GetReactionUsed(), brisa.GetConcentrationSpell(), a.readiedStatus(t, "Brisa"))
	}
	var cast *playv1.CombatLogEntry
	for _, r := range a.log(t, a.master, e).GetRounds() {
		for _, en := range r.GetEntries() {
			if en.GetKind() == playv1.CombatLogKind_COMBAT_LOG_KIND_SPELL_CAST && en.GetKey() == guidingBolt {
				cast = en
			}
		}
	}
	if cast == nil || len(cast.GetSpell().GetTargets()) != 1 || cast.GetSpell().GetSlot().GetLevel() != 1 {
		t.Errorf("the release's line = %v, want Guiding Bolt at 1st level on the Capitão", cast)
	}
}

// The master readies the Mago Sombrio's Magic Missile; released with CastSpell, the cast is a cast: Pensantus may
// counter it. While her window is open the READIED window takes no answer; countered, the spell is spent (the row fired,
// the reaction used, no concentration) and Toren takes nothing.
func TestCounterspellOnAReadiedSpellsRelease(t *testing.T) {
	t.Parallel()
	a, e := counterFight(t)
	a.mustReady(t, a.master, a.readySpell(t, e, "Mago Sombrio", magicMissileSpell, 1, "Toren"))
	e = a.mustEndTurn(t, a.master, e)
	w := a.mustFire(t, e, "Mago Sombrio")
	held, err := a.cast(t, a.master, e, "Mago Sombrio", magicMissileSpell, nil, []*playv1.SpellTarget{darts(a, t, "Toren", 3)}, noCastRoll)
	if err != nil || held.GetCast() != nil {
		t.Fatalf("the release = %v, %v; want it held for Counterspell", held, err)
	}
	if _, err := a.answerReaction(t, a.master, e, w.GetId(), useSpellAnswer); err == nil {
		t.Error("the READIED window took a second use while the cast waits")
	}
	cw := a.windowOf(t, a.ana, playv1.ReactionKind_REACTION_KIND_COUNTERSPELL)
	if cw == nil || a.readiedStatus(t, "Mago Sombrio") != readiedWaiting {
		t.Fatalf("Counterspell window %v, rows %q", cw, a.readiedStatus(t, "Mago Sombrio"))
	}
	hp := byLabel(t, a.get(t, a.master), "Toren").GetHitPointsCurrent()
	a.mustAnswer(t, a.ana, e, cw.GetId(), useAnswer(3))
	mago := byLabel(t, a.get(t, a.master), "Mago Sombrio")
	if a.readiedStatus(t, "Mago Sombrio") != readiedFired || !mago.GetReactionUsed() || mago.GetConcentrationSpell() != "" || a.readiedOpen(t, "Mago Sombrio") {
		t.Errorf("after the counter: rows %q, reaction %v, concentration %q", a.readiedStatus(t, "Mago Sombrio"), mago.GetReactionUsed(), mago.GetConcentrationSpell())
	}
	if got := byLabel(t, a.get(t, a.master), "Toren").GetHitPointsCurrent(); got != hp {
		t.Errorf("Toren's hit points = %d, want %d: the spell was countered", got, hp)
	}
}

// The readied spell is lost at the start of Pensantus's next turn: the row is lost, the concentration ends, the slot
// stays spent, and the lost line names the spell for Ana alone.
func TestReadiedSpellIsLostAtTheNextTurn(t *testing.T) {
	t.Parallel()
	a := newCasters(t)
	e := a.castersFight(t, 1)
	a.mustReady(t, a.ana, a.readySpell(t, e, "Pensantus", magicMissileSpell, 1, ""))
	slots := a.slotsUsed(t, "Pensantus", 1)
	e = a.nextTurnOf(t, e, "Pensantus")
	if got := a.concentrationAs(t, a.ana, "Pensantus"); got != "" || a.readiedStatus(t, "Pensantus") != readiedLost || a.slotsUsed(t, "Pensantus", 1) != slots {
		t.Errorf("at the next turn: concentration %q, rows %q, slots %d (want %d)", got, a.readiedStatus(t, "Pensantus"), a.slotsUsed(t, "Pensantus", 1), slots)
	}
	if lost := a.readiedLines(t, a.ana, e, playv1.CombatLogKind_COMBAT_LOG_KIND_READIED_LOST); len(lost) != 1 || lost[0].GetReadiedLost().GetActionKey() != magicMissileSpell {
		t.Errorf("Ana's lost lines = %v", lost)
	}
}

// Pensantus readies a spell and gives it up on his turn: the row is canceled, the concentration ends, the slot stays spent.
func TestCancelAReadiedSpell(t *testing.T) {
	t.Parallel()
	a := newCasters(t)
	e := a.castersFight(t, 1)
	a.mustReady(t, a.ana, a.readySpell(t, e, "Pensantus", magicMissileSpell, 1, ""))
	slots := a.slotsUsed(t, "Pensantus", 1)
	if err := a.cancelReadied(t, a.ana, e, byLabel(t, a.get(t, a.ana), "Pensantus").GetReadied().GetId()); err != nil {
		t.Fatalf("CancelReadied() error = %v", err)
	}
	if got := a.concentrationAs(t, a.ana, "Pensantus"); got != "" || a.readiedStatus(t, "Pensantus") != readiedCanceled || a.slotsUsed(t, "Pensantus", 1) != slots {
		t.Errorf("after the cancel: concentration %q, rows %q, slots %d (want %d)", got, a.readiedStatus(t, "Pensantus"), a.slotsUsed(t, "Pensantus", 1), slots)
	}
}

// brokenAs checks the held spell broke: the row is broken, the concentration ended, the slot spent, and the lost line
// says BROKEN to Ana. The concentration is the one that ended it, if any.
func (a *armed) brokenAs(t *testing.T, e *playv1.Encounter, how string, slots int32, concentration string) {
	t.Helper()
	if got := a.concentrationAs(t, a.ana, "Pensantus"); got != concentration || a.readiedStatus(t, "Pensantus") != readiedBroken || a.slotsUsed(t, "Pensantus", 1) != slots {
		t.Errorf("%s: concentration %q, rows %q, slots %d (want %d)", how, got, a.readiedStatus(t, "Pensantus"), a.slotsUsed(t, "Pensantus", 1), slots)
	}
	lost := a.readiedLines(t, a.ana, e, playv1.CombatLogKind_COMBAT_LOG_KIND_READIED_LOST)
	if len(lost) != 1 || lost[0].GetReadiedLost().GetStatus() != playv1.ReadiedStatus_READIED_STATUS_BROKEN {
		t.Errorf("%s: Ana's lost lines = %v", how, lost)
	}
}

// waitsAgain checks the undo gave the held spell back.
func (a *armed) waitsAgain(t *testing.T, e *playv1.Encounter, how string) {
	t.Helper()
	if got := a.concentrationAs(t, a.ana, "Pensantus"); got != magicMissileSpell || a.readiedStatus(t, "Pensantus") != readiedWaiting {
		t.Errorf("after the undo of %s: concentration %q, rows %q", how, got, a.readiedStatus(t, "Pensantus"))
	}
	if lost := a.readiedLines(t, a.ana, e, playv1.CombatLogKind_COMBAT_LOG_KIND_READIED_LOST); len(lost) != 0 {
		t.Errorf("after the undo of %s: lost lines %v", how, lost)
	}
}

// The concentration that holds a readied spell ends the spell whatever ends it: his player ending it, another
// concentration spell, a failed concentration save, 0 hit points. The undo of the change that ended it gives it back.
func TestReadiedSpellBreaksWithItsConcentration(t *testing.T) {
	t.Parallel()
	ready := func(t *testing.T) (*armed, *playv1.Encounter, int32) {
		t.Helper()
		a := newCasters(t)
		e := a.castersFight(t, 1)
		a.mustReady(t, a.ana, a.readySpell(t, e, "Pensantus", magicMissileSpell, 1, ""))
		slots := a.slotsUsed(t, "Pensantus", 1)
		return a, e, slots
	}
	t.Run("ended by his player", func(t *testing.T) {
		t.Parallel()
		a, e, slots := ready(t)
		if _, err := a.conditions(t, a.ana, e, "Pensantus", nil, false, true); err != nil {
			t.Fatalf("SetCombatantConditions(end_concentration) error = %v", err)
		}
		a.brokenAs(t, e, "ending it", slots, "")
		a.mustUndo(t, e)
		a.waitsAgain(t, e, "ending it")
	})
	t.Run("another concentration spell", func(t *testing.T) {
		t.Parallel()
		a, e, slots := ready(t)
		a.execSQL(t, `UPDATE combatants SET action_used = false WHERE label = 'Pensantus'`) // Action Surge's second action
		a.mustCast(t, a.ana, e, "Pensantus", holdPerson, slotOfLevel(2), a.at(t, "Capitão Goblin"), noCastRoll)
		a.brokenAs(t, e, "Hold Person", slots, holdPerson)
		if got := a.concentrationAs(t, a.caio, "Pensantus"); got != holdPerson {
			t.Errorf("Caio reads the concentration %q, want Hold Person", got)
		}
		a.mustUndo(t, e)
		a.waitsAgain(t, e, "Hold Person")
	})
	t.Run("a failed concentration save", func(t *testing.T) {
		t.Parallel()
		a, e, slots := ready(t)
		e = a.passTo(t, e, "Capitão Goblin")
		hit := a.mustAttack(t, a.master, e, "Capitão Goblin", sword, "Pensantus", d20(19))
		if _, err := a.declineReaction(t, a.ana, e, hit.GetPendingDamage().GetId()); err != nil {
			t.Fatalf("DeclineReaction() error = %v", err)
		}
		a.h.roller.queue(1)
		a.mustDamage(t, a.master, e, hit.GetPendingDamage().GetId(), inAppDamage)
		if _, err := a.settle(t, a.master, e, hit.GetPendingDamage().GetId(), true); err != nil {
			t.Fatalf("ApplyPendingDamage() error = %v", err)
		}
		w := a.windowOf(t, a.ana, playv1.ReactionKind_REACTION_KIND_CONCENTRATION_SAVE)
		if w == nil {
			t.Fatal("no concentration save for the readied spell")
		}
		if _, err := a.resolveConcentration(t, a.ana, e, w.GetId(), concentrationFace(1)); err != nil {
			t.Fatalf("ResolveConcentrationSave() error = %v", err)
		}
		a.brokenAs(t, e, "the failed save", slots, "")
		if text := asJSON(t, a.log(t, a.caio, e)); strings.Contains(text, magicMissileSpell) || strings.Contains(text, concentrationReadied) {
			t.Errorf("Caio's log tells of the held spell: %s", text)
		}
	})
	t.Run("an incapacitating condition", func(t *testing.T) {
		t.Parallel()
		a, e, slots := ready(t)
		a.mustAddEffect(t, e, "spell:hideous-laughter", []string{"Pensantus"}, a.rounds(t, 3, "Toren"))
		a.brokenAs(t, e, "incapacitated", slots, "")
	})
	t.Run("0 hit points", func(t *testing.T) {
		t.Parallel()
		a, e, slots := ready(t)
		a.correct(t, a.pens, hpIs(0))
		a.brokenAs(t, e, "0 hit points", slots, "")
	})
}

// The undo of the Ready of a spell gives back the action, the slot and the concentration it ended (Hold Person's
// paralysis with it), and takes the row away.
func TestUndoOfAReadiedSpell(t *testing.T) {
	t.Parallel()
	a := newCasters(t)
	e := a.castersFight(t, 1)
	a.h.roller.queue(1)
	a.mustCast(t, a.ana, e, "Pensantus", holdPerson, slotOfLevel(2), a.at(t, "Capitão Goblin"), noCastRoll)
	e = a.nextTurnOf(t, e, "Pensantus")
	slots := a.slotsUsed(t, "Pensantus", 1)
	a.mustReady(t, a.ana, a.readySpell(t, e, "Pensantus", magicMissileSpell, 1, ""))
	a.mustUndo(t, e)
	pens := byLabel(t, a.get(t, a.ana), "Pensantus")
	if pens.GetActionUsed() || pens.GetConcentrationSpell() != holdPerson || a.slotsUsed(t, "Pensantus", 1) != slots || a.readiedStatus(t, "Pensantus") != "" {
		t.Errorf("after the undo: action %v, concentration %q, slots %d (want %d), rows %q", pens.GetActionUsed(), pens.GetConcentrationSpell(), a.slotsUsed(t, "Pensantus", 1), slots, a.readiedStatus(t, "Pensantus"))
	}
	if a.heldEffects(t, holdPerson) == 0 {
		t.Error("Hold Person's paralysis did not come back with its concentration")
	}
}

// The same ReadyAction twice is one Ready: one slot, one row.
func TestReadySpellIsIdempotent(t *testing.T) {
	t.Parallel()
	a := newCasters(t)
	e := a.castersFight(t, 1)
	slots := a.slotsUsed(t, "Pensantus", 1)
	req := a.readySpell(t, e, "Pensantus", magicMissileSpell, 1, "")
	a.mustReady(t, a.ana, req)
	a.mustReady(t, a.ana, req)
	if got := a.slotsUsed(t, "Pensantus", 1); got != slots+1 || a.readiedStatus(t, "Pensantus") != readiedWaiting {
		t.Errorf("after the same Ready twice: slots %d (want %d), rows %q", got, slots+1, a.readiedStatus(t, "Pensantus"))
	}
}

// The master readies the Mago Sombrio's Magic Missile and the Mago falls: the row is lost as a defeated readier's is,
// and its concentration ends with it.
func TestADefeatedReadierLosesItsReadiedSpell(t *testing.T) {
	t.Parallel()
	a, e := counterFight(t)
	a.mustReady(t, a.master, a.readySpell(t, e, "Mago Sombrio", magicMissileSpell, 1, "Toren"))
	if _, err := a.adjustHP(t, e, "Mago Sombrio", damageHP(999)); err != nil {
		t.Fatalf("damage the mago: %v", err)
	}
	if got := a.concentrationAs(t, a.master, "Mago Sombrio"); got != "" || a.readiedStatus(t, "Mago Sombrio") != readiedLost {
		t.Errorf("after the defeat: concentration %q, rows %q", got, a.readiedStatus(t, "Mago Sombrio"))
	}
}
