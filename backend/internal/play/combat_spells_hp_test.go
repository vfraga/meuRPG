package play

import (
	"slices"
	"strings"
	"testing"

	"connectrpc.com/connect"

	campaignsv1 "github.com/PuraFome/meuRPG/backend/gen/meurpg/campaigns/v1"
	charactersv1 "github.com/PuraFome/meuRPG/backend/gen/meurpg/characters/v1"
	playv1 "github.com/PuraFome/meuRPG/backend/gen/meurpg/play/v1"
	rulesv1 "github.com/PuraFome/meuRPG/backend/gen/meurpg/rules/v1"
)

// The spells that read hit points (MR-014, RN-02, RN-18, RN-20, Etapa 8, slice
// 8.1): Sono, Leque Cromático, Palavra de Poder Atordoar and Matar, Poupar os
// Moribundos and Cura Completa. These tests need the database
// (MEURPG_TEST_DATABASE_URL). The numbers of E8-03: Pensantus's Sono rolls 5d8
// (2, 4, 1, 5, 3) = 15 against a goblin with 7 PV and the Capitão with 27.

const (
	colorSpray   = "spell:color-spray"
	wordStun     = "spell:power-word-stun"
	wordKill     = "spell:power-word-kill"
	spareDying   = "spell:spare-the-dying"
	healSpell    = "spell:heal"
	unconsciousC = "condition:unconscious"
	blindedC     = "condition:blinded"
	deafenedC    = "condition:deafened"
	poisonedC    = "condition:poisoned"
	stunnedC     = "condition:stunned"
)

func poolInApp(r *playv1.CastSpellRequest) {
	r.Roll = &playv1.CastSpellRequest_RollInApp{RollInApp: true}
}

func poolSum(n int32) func(*playv1.CastSpellRequest) {
	return func(r *playv1.CastSpellRequest) { r.Roll = &playv1.CastSpellRequest_PoolSum{PoolSum: n} }
}

// newHPCasters is the party of newCasters with the characters that can cast the
// high spells: Pensantus a level 17 wizard (the 8th and 9th circle) and Brisa a
// level 11 cleric (the 6th), with Estabilizar.
func newHPCasters(t *testing.T) *armed {
	t.Helper()
	return newArmedWith(t, func(a *armed) {
		a.toren = a.caio.hero(t, a.campaignID, "Toren", "class:fighter", "race:human", 5,
			&rulesv1.AbilityScores{Strength: 16, Dexterity: 13, Constitution: 14, Intelligence: 10, Wisdom: 10, Charisma: 8}, []string{battleaxe}, nil)
		spells := []string{sleepSpell, colorSpray, wordStun, wordKill}
		a.pens = a.ana.caster(t, a.campaignID, "Pensantus", "class:wizard", "race:gnome", 17,
			&rulesv1.AbilityScores{Strength: 10, Dexterity: 14, Constitution: 12, Intelligence: 20, Wisdom: 10, Charisma: 8}, nil, nil, spells, spells)
		a.bri = a.bia.caster(t, a.campaignID, "Brisa", "class:cleric", "race:human", 11,
			&rulesv1.AbilityScores{Strength: 10, Dexterity: 16, Constitution: 14, Intelligence: 10, Wisdom: 16, Charisma: 8}, []string{maceKey}, []string{spareDying}, nil,
			[]string{healSpell})
	})
}

// spellEntry is the latest cast entry of the log (the log lists the newest
// round, and the newest entry of a round, first).
func spellEntry(t *testing.T, l *playv1.ListCombatLogResponse) *playv1.CombatLogEntry {
	t.Helper()
	for _, r := range l.GetRounds() {
		for _, en := range r.GetEntries() {
			if en.GetKind() == playv1.CombatLogKind_COMBAT_LOG_KIND_SPELL_CAST {
				return en
			}
		}
	}
	t.Fatal("no spell cast in the log")
	return nil
}

// effectOf is the effect of the target with the label, in a cast.
func effectOf(t *testing.T, e *playv1.Encounter, targets []*playv1.SpellTargetResult, label string) *playv1.SpellEffectResult {
	t.Helper()
	id := byLabel(t, e, label).GetId()
	for _, r := range targets {
		if r.GetCombatantId() == id {
			if r.GetEffect() == nil {
				t.Fatalf("%s has no effect in the cast", label)
			}
			return r.GetEffect()
		}
	}
	t.Fatalf("%s is not in the cast", label)
	return nil
}

func logEffect(t *testing.T, en *playv1.CombatLogEntry, label string) *playv1.SpellEffectResult {
	t.Helper()
	for _, tg := range en.GetSpell().GetTargets() {
		if tg.GetTargetLabel() == label {
			if tg.GetEffect() == nil {
				t.Fatalf("%s has no effect in the log", label)
			}
			return tg.GetEffect()
		}
	}
	t.Fatalf("%s is not in the log entry", label)
	return nil
}

// setConditions is the master's marking of a combatant's conditions.
func (a *armed) setConditions(t *testing.T, e *playv1.Encounter, label string, keys ...string) {
	t.Helper()
	if _, err := a.conditions(t, a.master, e, label, keys, true, false); err != nil {
		t.Fatalf("SetCombatantConditions(%s) error = %v", label, err)
	}
}

func hasCondition(e *playv1.Encounter, t *testing.T, label, key string) bool {
	t.Helper()
	return slices.Contains(byLabel(t, e, label).GetConditions(), key)
}

func TestMR014_SleepUsesTheRealHitPoints(t *testing.T) {
	t.Parallel()
	a := newCasters(t)
	e := a.castersFight(t, 1)

	// Pensantus rolls 5d8 (2, 4, 1, 5, 3) = 15. The goblin (7 PV) is the lowest:
	// it falls asleep and 8 are left; the Capitão (27 PV) does not fit.
	a.h.roller.queue(2, 4, 1, 5, 3)
	res := a.mustCast(t, a.ana, e, "Pensantus", sleepSpell, slotOfLevel(1), a.at(t, "Capitão Goblin", "Goblin"), poolInApp)
	cast := res.GetCast()
	if cast.GetEffectKind() != playv1.SpellEffectKind_SPELL_EFFECT_KIND_POOL || cast.GetEffectConditionKey() != unconsciousC {
		t.Errorf("the cast's kind and condition = %v, %q; want a pool giving Inconsciente", cast.GetEffectKind(), cast.GetEffectConditionKey())
	}
	if r := cast.GetPoolRoll(); r.GetDiceCount() != 5 || r.GetDiceSides() != 8 || len(r.GetFaces()) != 5 || r.GetFaces()[1] != 4 || r.GetTotal() != 15 || r.GetPhysical() {
		t.Errorf("the caster's pool roll = %v, want 5d8 (2, 4, 1, 5, 3) = 15", r)
	}
	pe := res.GetEncounter()
	if got := effectOf(t, pe, cast.GetTargets(), "Goblin").GetOutcome(); got != playv1.SpellEffectOutcome_SPELL_EFFECT_OUTCOME_AFFECTED {
		t.Errorf("the goblin's outcome = %v, want affected", got)
	}
	if got := effectOf(t, pe, cast.GetTargets(), "Capitão Goblin").GetOutcome(); got != playv1.SpellEffectOutcome_SPELL_EFFECT_OUTCOME_NOT_AFFECTED {
		t.Errorf("the Capitão's outcome = %v, want not affected", got)
	}

	// The player never gets an enemy's hit points, the pool's arithmetic or why
	// somebody was not affected, in the answer or in the log (RN-20).
	mine := asJSON(t, cast)
	for _, who := range []struct {
		name string
		json string
	}{
		{"Ana's answer", mine},
		{"Ana's log", asJSON(t, spellEntry(t, a.log(t, a.ana, e)))},
		{"Caio's log", asJSON(t, spellEntry(t, a.log(t, a.caio, e)))},
	} {
		for _, leak := range []string{"hitPointsBefore", "poolLeft", "poolOrder", "reason", "effectThreshold"} {
			if strings.Contains(who.json, leak) {
				t.Errorf("%s carries %q: %s", who.name, leak, who.json)
			}
		}
	}

	// The master gets the pool, each creature's hit points, the order and who was
	// affected.
	master := spellEntry(t, a.log(t, a.master, e))
	if r := master.GetSpell().GetPoolRoll(); r.GetTotal() != 15 || len(r.GetFaces()) != 5 {
		t.Errorf("the master's pool roll = %v, want 15", r)
	}
	gob, capt := logEffect(t, master, "Goblin"), logEffect(t, master, "Capitão Goblin")
	if gob.GetHitPointsBefore() != 7 || gob.GetPoolOrder() != 1 || gob.GetPoolLeft() != 8 || gob.GetOutcome() != playv1.SpellEffectOutcome_SPELL_EFFECT_OUTCOME_AFFECTED {
		t.Errorf("the goblin for the master = %v, want 7 PV, first, 8 left, affected", gob)
	}
	if capt.GetHitPointsBefore() != 27 || capt.GetPoolOrder() != 2 || capt.GetPoolLeft() != 8 || capt.GetReason() != playv1.SpellEffectReason_SPELL_EFFECT_REASON_ABOVE_POOL {
		t.Errorf("the Capitão for the master = %v, want 27 PV, second, 8 left, above the pool", capt)
	}
	// The caster's player keeps the roll in the log; the others do not have it.
	if r := spellEntry(t, a.log(t, a.ana, e)).GetSpell().GetPoolRoll(); r.GetTotal() != 15 {
		t.Errorf("the caster's player's log pool roll = %v, want 15", r)
	}
	if r := spellEntry(t, a.log(t, a.caio, e)).GetSpell().GetPoolRoll(); r != nil {
		t.Errorf("another player's log carries the pool roll: %v", r)
	}
	// Everybody sees the words: who is affected, by the targets in the order the
	// caster listed them.
	for who, u := range map[string]*user{"Caio": a.caio, "Ana": a.ana, "master": a.master} {
		en := spellEntry(t, a.log(t, u, e))
		if logEffect(t, en, "Goblin").GetOutcome() != playv1.SpellEffectOutcome_SPELL_EFFECT_OUTCOME_AFFECTED ||
			logEffect(t, en, "Capitão Goblin").GetOutcome() != playv1.SpellEffectOutcome_SPELL_EFFECT_OUTCOME_NOT_AFFECTED {
			t.Errorf("%s's log outcomes = %v, want the goblin affected and the Capitão not", who, en.GetSpell().GetTargets())
		}
	}

	// The state: the goblin is Inconsciente and keeps its 7 PV; the Capitão is
	// untouched; the slot and the action are spent.
	now := a.get(t, a.master)
	if !hasCondition(now, t, "Goblin", unconsciousC) || hasCondition(now, t, "Capitão Goblin", unconsciousC) {
		t.Errorf("conditions after Sono: goblin %v, Capitão %v", byLabel(t, now, "Goblin").GetConditions(), byLabel(t, now, "Capitão Goblin").GetConditions())
	}
	if hp, _, defeated := a.hp(t, "Goblin"); hp != 7 || defeated {
		t.Errorf("the sleeping goblin has %d PV, defeated %v; want 7 and not defeated", hp, defeated)
	}
	if got := usedSlots(a.vitals(t, a.pens), 1); got != 1 {
		t.Errorf("1st-level slots used = %d, want 1", got)
	}
	// The condition shows to the players as a tag.
	if !hasCondition(a.get(t, a.caio), t, "Goblin", unconsciousC) {
		t.Error("the players do not see the goblin's Inconsciente tag")
	}
}

func TestMR014_SleepSkipsTheUnconsciousAndTheOneAtZero(t *testing.T) {
	t.Parallel()
	a := newCasters(t)
	e := a.castersFight(t, 1)
	a.setConditions(t, e, "Goblin", unconsciousC)
	a.correct(t, a.toren, hpIs(0))

	// 5d8 = 40: enough for the Capitão (27 PV), but the sleeping goblin and Toren
	// (0 PV) are skipped and spend nothing.
	a.h.roller.queue(8, 8, 8, 8, 8)
	res := a.mustCast(t, a.ana, e, "Pensantus", sleepSpell, slotOfLevel(1), a.at(t, "Goblin", "Toren", "Capitão Goblin"), poolInApp)
	master := spellEntry(t, a.log(t, a.master, e))
	for label, want := range map[string]playv1.SpellEffectReason{
		"Goblin": playv1.SpellEffectReason_SPELL_EFFECT_REASON_SKIPPED, "Toren": playv1.SpellEffectReason_SPELL_EFFECT_REASON_SKIPPED,
	} {
		if en := logEffect(t, master, label); en.GetReason() != want || en.GetOutcome() != playv1.SpellEffectOutcome_SPELL_EFFECT_OUTCOME_NOT_AFFECTED {
			t.Errorf("%s = %v, want skipped", label, en)
		}
	}
	now := res.GetEncounter()
	if !hasCondition(a.get(t, a.master), t, "Capitão Goblin", unconsciousC) {
		t.Errorf("the Capitão was not put to sleep: %v", byLabel(t, now, "Capitão Goblin").GetConditions())
	}
	if hasCondition(a.get(t, a.master), t, "Toren", unconsciousC) {
		t.Error("Toren at 0 PV was put to sleep")
	}
}

func TestMR014_ColorSprayBlindsByThePool(t *testing.T) {
	t.Parallel()
	a := newHPCasters(t)
	e := a.castersFight(t, 1)

	// 6d10 = 36 at the 1st circle: the goblin (7) and the Capitão (27) both fit.
	a.h.roller.queue(6, 6, 6, 6, 6, 6)
	res := a.mustCast(t, a.ana, e, "Pensantus", colorSpray, slotOfLevel(1), a.at(t, "Capitão Goblin", "Goblin"), poolInApp)
	if r := res.GetCast().GetPoolRoll(); r.GetDiceCount() != 6 || r.GetDiceSides() != 10 || r.GetTotal() != 36 {
		t.Errorf("the pool = %v, want 6d10 = 36", r)
	}
	now := a.get(t, a.master)
	for _, label := range []string{"Goblin", "Capitão Goblin"} {
		if !hasCondition(now, t, label, blindedC) {
			t.Errorf("%s is not blinded: %v", label, byLabel(t, now, label).GetConditions())
		}
	}

	// Upcast at the 2nd circle: 8d10, two more dice.
	a.mustEndTurn(t, a.ana, e)
	a.passTo(t, e, "Pensantus")
	a.h.roller.queue(1, 1, 1, 1, 1, 1, 1, 1)
	a.setConditions(t, e, "Goblin")
	res = a.mustCast(t, a.ana, a.get(t, a.master), "Pensantus", colorSpray, slotOfLevel(2), a.at(t, "Goblin"), poolInApp)
	if r := res.GetCast().GetPoolRoll(); r.GetDiceCount() != 8 || len(r.GetFaces()) != 8 || r.GetTotal() != 8 {
		t.Errorf("the upcast pool = %v, want 8d10 = 8", r)
	}
	// 8 PV of pool against a goblin with 7: blinded; 1 left.
	if !hasCondition(a.get(t, a.master), t, "Goblin", blindedC) {
		t.Error("the goblin is not blinded by the upcast pool")
	}
}

func TestMR014_PowerWordStunAndKillOnNPCs(t *testing.T) {
	t.Parallel()
	a := newHPCasters(t)
	dragon := a.master.npc(t, a.campaignID, "Dragão", 160, 17)
	e := a.start(t, plan{
		npcs:     []*playv1.Participant{{CharacterId: a.capitao.GetId()}, {CharacterId: dragon.GetId()}},
		npcRolls: []int{1, 1}, reveal: []string{"Capitão Goblin", "Dragão"},
		at:      map[string][2]int32{"Pensantus": {5, 5}, "Toren": {6, 5}, "Brisa": {5, 6}, "Capitão Goblin": {8, 5}, "Dragão": {8, 6}},
		players: map[string]int32{"Pensantus": 20, "Toren": 15, "Brisa": 10},
	})
	refill := func() {
		a.correct(t, a.pens, func(r *playv1.AdjustCharacterVitalsRequest) {
			r.SpellSlotsUsed = []*playv1.SpellSlotsUsed{{Level: 8, Used: 0}, {Level: 9, Used: 0}}
		})
	}

	// 160 PV is above Atordoar's 150: not affected, and the master is told why.
	a.mustCast(t, a.ana, e, "Pensantus", wordStun, slotOfLevel(8), a.at(t, "Dragão"), noCastRoll)
	en := spellEntry(t, a.log(t, a.master, e))
	if f := logEffect(t, en, "Dragão"); f.GetOutcome() != playv1.SpellEffectOutcome_SPELL_EFFECT_OUTCOME_NOT_AFFECTED || f.GetReason() != playv1.SpellEffectReason_SPELL_EFFECT_REASON_ABOVE_LIMIT || f.GetHitPointsBefore() != 160 {
		t.Errorf("Atordoar on the dragon = %v, want not affected: above the limit, 160 PV", f)
	}
	if en.GetSpell().GetEffectThreshold() != 150 || en.GetSpell().GetEffectKind() != playv1.SpellEffectKind_SPELL_EFFECT_KIND_THRESHOLD {
		t.Errorf("the master's threshold = %v (%v), want 150", en.GetSpell().GetEffectThreshold(), en.GetSpell().GetEffectKind())
	}
	if p := spellEntry(t, a.log(t, a.caio, e)).GetSpell(); p.GetEffectThreshold() != 0 {
		t.Errorf("a player gets the threshold %d", p.GetEffectThreshold())
	}
	if hasCondition(a.get(t, a.master), t, "Dragão", stunnedC) {
		t.Error("the dragon is stunned")
	}

	// Round 2: the Capitão (27 PV) is stunned.
	a.passTo(t, e, "Pensantus")
	a.passTo(t, e, "Toren")
	a.passTo(t, e, "Pensantus")
	refill()
	a.mustCast(t, a.ana, e, "Pensantus", wordStun, slotOfLevel(8), a.at(t, "Capitão Goblin"), noCastRoll)
	if !hasCondition(a.get(t, a.master), t, "Capitão Goblin", stunnedC) {
		t.Error("the Capitão is not stunned")
	}

	// Matar: the Capitão (27 PV, at or below 100) is defeated at 0 PV; the undo
	// brings it back exactly.
	a.passTo(t, e, "Toren")
	a.passTo(t, e, "Pensantus")
	refill()
	a.undoes(t, "Palavra de Poder Matar on an NPC", func() {
		a.mustCast(t, a.ana, a.get(t, a.master), "Pensantus", wordKill, slotOfLevel(9), a.at(t, "Capitão Goblin"), noCastRoll)
		if hp, _, defeated := a.hp(t, "Capitão Goblin"); hp != 0 || !defeated {
			t.Errorf("the Capitão after Matar = %d PV, defeated %v; want 0 and defeated", hp, defeated)
		}
	})
	// Matar on a dragon with more than 100 PV does nothing.
	a.mustCast(t, a.ana, a.get(t, a.master), "Pensantus", wordKill, slotOfLevel(9), a.at(t, "Dragão"), noCastRoll)
	if hp, _, defeated := a.hp(t, "Dragão"); hp != 160 || defeated {
		t.Errorf("the dragon after Matar = %d PV, defeated %v; want 160 and not defeated", hp, defeated)
	}
	en = spellEntry(t, a.log(t, a.master, e))
	if en.GetSpell().GetEffectThreshold() != 100 || en.GetSpell().GetEffectConditionKey() != "" {
		t.Errorf("Matar's header = threshold %d, condition %q; want 100 and none", en.GetSpell().GetEffectThreshold(), en.GetSpell().GetEffectConditionKey())
	}
}

func TestMR014_PowerWordKillOnAPlayerCharacterWaitsForTheMaster(t *testing.T) {
	t.Parallel()
	a := newHPCasters(t)
	e := a.castersFight(t, 1)

	// Toren has fewer than 100 PV: Matar drops him to 0 PV with three failures and
	// the master confirms the death. The undo puts the character back.
	a.undoes(t, "Matar on a player's character", func() {
		a.mustCast(t, a.ana, e, "Pensantus", wordKill, slotOfLevel(9), a.at(t, "Toren"), noCastRoll)
		if v := a.vitals(t, a.toren); v.GetHitPointsCurrent() != 0 {
			t.Errorf("Toren after Matar has %d PV, want 0", v.GetHitPointsCurrent())
		}
	})
	if v := a.vitals(t, a.toren); v.GetHitPointsCurrent() == 0 {
		t.Fatal("the undo left Toren at 0 PV")
	}
	a.mustCast(t, a.ana, e, "Pensantus", wordKill, slotOfLevel(9), a.at(t, "Toren"), noCastRoll)
	now := a.get(t, a.master)
	if c := byLabel(t, now, "Toren"); c.GetDeathFailures() != 3 || c.GetState() != playv1.CombatantState_COMBATANT_STATE_DYING || c.GetDefeated() {
		t.Errorf("Toren after Matar = failures %d, state %v, defeated %v; want 3, dying, not yet dead", c.GetDeathFailures(), c.GetState(), c.GetDefeated())
	}
	if _, err := a.master.combat.ConfirmDeath(t.Context(), connect.NewRequest(&playv1.ConfirmDeathRequest{
		CampaignId: a.campaignID, EncounterId: e.GetId(), CombatantId: a.id(t, "Toren"), IdempotencyKey: newKey(),
	})); err != nil {
		t.Fatalf("ConfirmDeath() after Matar error = %v", err)
	}
	if got := a.caio.character(t, a.toren).GetState(); got != charactersv1.CharacterState_CHARACTER_STATE_DEAD {
		t.Errorf("Toren's character state = %v, want DEAD", got)
	}
}

func TestMR014_SpareTheDyingWorksOnlyAtZero(t *testing.T) {
	t.Parallel()
	a := newHPCasters(t)
	e := a.castersFight(t, 1)
	a.correct(t, a.toren, hpIs(0))
	a.passTo(t, e, "Brisa")

	// Pensantus is not at 0 PV: not affected (the master is told why), and the
	// cast is undone so the turn is free again.
	a.mustCast(t, a.bia, e, "Brisa", spareDying, nil, a.at(t, "Pensantus"), noCastRoll)
	if f := logEffect(t, spellEntry(t, a.log(t, a.master, e)), "Pensantus"); f.GetOutcome() != playv1.SpellEffectOutcome_SPELL_EFFECT_OUTCOME_NOT_AFFECTED || f.GetReason() != playv1.SpellEffectReason_SPELL_EFFECT_REASON_NOT_AT_ZERO {
		t.Errorf("Estabilizar on a healthy one = %v, want not affected: not at zero", f)
	}
	if err := a.undo(t, a.master, e, a.log(t, a.master, e).GetUndoableEventId()); err != nil {
		t.Fatalf("UndoLastAction() error = %v", err)
	}

	// Toren, dying at 0 PV, becomes stable; the undo makes him dying again.
	a.undoes(t, "Estabilizar", func() {
		a.mustCast(t, a.bia, e, "Brisa", spareDying, nil, a.at(t, "Toren"), noCastRoll)
		if c := byLabel(t, a.get(t, a.master), "Toren"); c.GetState() != playv1.CombatantState_COMBATANT_STATE_STABLE {
			t.Errorf("Toren after Estabilizar = %v, want stable", c.GetState())
		}
	})
	a.mustCast(t, a.bia, e, "Brisa", spareDying, nil, a.at(t, "Toren"), noCastRoll)
	for who, u := range map[string]*user{"master": a.master, "Toren's player": a.caio} {
		if c := byLabel(t, a.get(t, u), "Toren"); c.GetState() != playv1.CombatantState_COMBATANT_STATE_STABLE || c.GetDeathSuccesses() != 3 {
			t.Errorf("Toren for %s = %v with %d successes, want stable", who, c.GetState(), c.GetDeathSuccesses())
		}
	}
}

func TestMR014_CompleteHealHealsAndEndsBlindnessAndDeafness(t *testing.T) {
	t.Parallel()
	a := newHPCasters(t)
	e := a.castersFight(t, 1)
	a.correct(t, a.toren, hpIs(10))
	a.setConditions(t, e, "Toren", blindedC, poisonedC, deafenedC)
	a.passTo(t, e, "Brisa")

	a.undoes(t, "Cura Completa", func() {
		a.mustCast(t, a.bia, e, "Brisa", healSpell, slotOfLevel(6), a.at(t, "Toren"), noCastRoll)
	})
	res := a.mustCast(t, a.bia, e, "Brisa", healSpell, slotOfLevel(6), a.at(t, "Toren"), noCastRoll)
	maxHP := a.vitals(t, a.toren).GetHitPointsMax()
	if v := a.vitals(t, a.toren); v.GetHitPointsCurrent() != maxHP {
		t.Errorf("Toren has %d PV after Cura Completa, want his maximum %d (70 is more than he lacks)", v.GetHitPointsCurrent(), maxHP)
	}
	now := a.get(t, a.master)
	if c := byLabel(t, now, "Toren"); len(c.GetConditions()) != 1 || c.GetConditions()[0] != poisonedC {
		t.Errorf("Toren's conditions = %v, want only Envenenado: blindness and deafness ended", c.GetConditions())
	}
	if got := effectOf(t, now, res.GetCast().GetTargets(), "Toren"); got.Healed != nil || got.GetOutcome() != playv1.SpellEffectOutcome_SPELL_EFFECT_OUTCOME_AFFECTED {
		t.Errorf("the caster's effect = %v, want affected with no amount (it is capped at Toren's maximum)", got)
	}
	// The amount, capped at the maximum, goes to the master and Toren's player only
	// (as any heal, slice 6.4b): on an NPC it would tell the caster what it lacked.
	for who, c := range map[string]struct {
		u    *user
		want bool
	}{"master": {a.master, true}, "Brisa's player": {a.bia, false}, "Toren's player": {a.caio, true}, "Pensantus's player": {a.ana, false}} {
		got := logEffect(t, spellEntry(t, a.log(t, c.u, e)), "Toren")
		if (got.Healed != nil) != c.want {
			t.Errorf("%s gets the heal amount = %v, want %v", who, got.Healed != nil, c.want)
		}
	}
}

func TestRN18_PoolSpellsFollowTheDiceMode(t *testing.T) {
	t.Parallel()
	a := newCasters(t)
	e := a.castersFight(t, 1)

	// Every player rolls in the app: a typed sum is refused, the app's roll passes.
	a.forceDice(t, campaignsv1.DiceMode_DICE_MODE_APP)
	_, err := a.cast(t, a.ana, e, "Pensantus", sleepSpell, slotOfLevel(1), a.at(t, "Goblin"), poolSum(20))
	wantBlockedBy(t, "a physical pool with the app forced", err, blockedWrongDice)
	if got := usedSlots(a.vitals(t, a.pens), 1); got != 0 {
		t.Errorf("a refused cast spent %d slots", got)
	}
	// Every player rolls real dice: the app's roll is refused, the typed sum passes.
	a.forceDice(t, campaignsv1.DiceMode_DICE_MODE_PHYSICAL)
	_, err = a.cast(t, a.ana, e, "Pensantus", sleepSpell, slotOfLevel(1), a.at(t, "Goblin"), poolInApp)
	wantBlockedBy(t, "the app's pool with the physical dice forced", err, blockedWrongDice)

	// The typed sum is checked against the dice: 5d8 is 5 to 40, and a d20 face is
	// not a pool.
	for _, bad := range []int32{4, 41, 0, -3} {
		_, err = a.cast(t, a.ana, e, "Pensantus", sleepSpell, slotOfLevel(1), a.at(t, "Goblin"), poolSum(bad))
		wantCode(t, "a pool sum out of range", err, connect.CodeInvalidArgument)
	}
	_, err = a.cast(t, a.ana, e, "Pensantus", sleepSpell, slotOfLevel(1), a.at(t, "Goblin"), func(r *playv1.CastSpellRequest) {
		r.Roll = &playv1.CastSpellRequest_D20Face{D20Face: 12}
	})
	wantCode(t, "a d20 face for a pool", err, connect.CodeInvalidArgument)
	_, err = a.cast(t, a.ana, e, "Pensantus", sleepSpell, slotOfLevel(1), a.at(t, "Goblin"), noCastRoll)
	wantCode(t, "no roll for a pool", err, connect.CodeInvalidArgument)

	// 22 typed from real dice: the faces are unknown, so only the total is shown,
	// and the goblin (7 PV) and the Capitão (27 PV) do not both fit.
	res := a.mustCast(t, a.ana, e, "Pensantus", sleepSpell, slotOfLevel(1), a.at(t, "Goblin", "Capitão Goblin"), poolSum(22))
	if r := res.GetCast().GetPoolRoll(); !r.GetPhysical() || r.GetTotal() != 22 || len(r.GetFaces()) != 0 || r.GetDiceCount() != 5 || r.GetDiceSides() != 8 {
		t.Errorf("the physical pool = %v, want 5d8 = 22 typed", r)
	}
	now := a.get(t, a.master)
	if !hasCondition(now, t, "Goblin", unconsciousC) || hasCondition(now, t, "Capitão Goblin", unconsciousC) {
		t.Errorf("after a pool of 22: goblin %v, Capitão %v; want only the goblin asleep", byLabel(t, now, "Goblin").GetConditions(), byLabel(t, now, "Capitão Goblin").GetConditions())
	}
}

// newBeastFormCasters is a party with Pensantus, a level 17 wizard with Sono and
// Palavra de Poder Matar, and Sálvia, a level 5 druid (38 hit points) already in
// the wolf form (11 hit points). Pensantus is on turn.
func newBeastFormCasters(t *testing.T) (a *armed, e *playv1.Encounter) {
	t.Helper()
	a = newArmedWith(t, func(a *armed) {
		a.toren = a.caio.hero(t, a.campaignID, "Toren", "class:fighter", "race:human", 5,
			&rulesv1.AbilityScores{Strength: 16, Dexterity: 13, Constitution: 14, Intelligence: 10, Wisdom: 10, Charisma: 8}, []string{battleaxe}, nil)
		spells := []string{sleepSpell, wordKill}
		a.pens = a.ana.caster(t, a.campaignID, "Pensantus", "class:wizard", "race:gnome", 17,
			&rulesv1.AbilityScores{Strength: 10, Dexterity: 14, Constitution: 12, Intelligence: 20, Wisdom: 10, Charisma: 8}, nil, nil, spells, spells)
		a.bri = a.bia.caster(t, a.campaignID, "Sálvia", "class:druid", "race:half-elf", 5,
			&rulesv1.AbilityScores{Strength: 10, Dexterity: 12, Constitution: 14, Intelligence: 10, Wisdom: 16, Charisma: 8}, nil, nil, nil,
			[]string{conjureAnimals})
	})
	a.start(t, plan{
		npcs:     []*playv1.Participant{{CharacterId: a.capitao.GetId()}, {CharacterId: a.goblin.GetId()}},
		npcRolls: []int{1, 1},
		players:  map[string]int32{"Sálvia": 20, "Pensantus": 12, "Toren": 10},
		reveal:   []string{"Capitão Goblin", "Goblin"},
		at: map[string][2]int32{
			"Sálvia": {6, 5}, "Capitão Goblin": {5, 5}, "Goblin": {6, 6}, "Pensantus": {12, 2}, "Toren": {2, 2},
		},
	})
	e = a.mustAssume(t, a.bia, a.bri, wolfKey).GetEncounter()
	w := a.vitals(t, a.bri)
	if w.GetWildShape().GetHitPointsCurrent() != 11 || w.GetHitPointsCurrent() != 38 {
		t.Fatalf("fixture: beast %v, own %d; want the wolf at 11 and her own 38", w.GetWildShape(), w.GetHitPointsCurrent())
	}
	e = a.passTo(t, e, "Pensantus")
	return a, e
}

// A druid in a beast form has the beast's hit points for the spells that read
// them, as damage and healing do (RN-02): Sono with a pool of 15 puts the
// 11-hit-point wolf to sleep, though her own 38 are above the pool, and the
// master sees the beast's numbers.
func TestHPSpellsReadTheBeastPoolOfADruidInBeastForm(t *testing.T) {
	t.Parallel()
	a, _ := newBeastFormCasters(t)
	a.h.roller.queue(2, 4, 1, 5, 3) // 5d8 = 15
	res := a.mustCast(t, a.ana, a.get(t, a.ana), "Pensantus", sleepSpell, slotOfLevel(1), a.at(t, "Sálvia"), poolInApp)
	pe := res.GetEncounter()
	eff := effectOf(t, pe, res.GetCast().GetTargets(), "Sálvia")
	if eff.GetOutcome() != playv1.SpellEffectOutcome_SPELL_EFFECT_OUTCOME_AFFECTED {
		t.Errorf("Sleep (pool 15) on the druid in the 11-HP wolf form: outcome = %v, want affected (the creature's hit points are the beast's)", eff.GetOutcome())
	}
	master := logEffect(t, spellEntry(t, a.log(t, a.master, pe)), "Sálvia")
	if master.GetHitPointsBefore() != 11 {
		t.Errorf("hit points the master sees for the druid in the form = %d, want the beast's 11 (not her own 38)", master.GetHitPointsBefore())
	}
	if w := a.vitals(t, a.bri); w.GetWildShape() != nil || w.GetHitPointsCurrent() != 38 {
		t.Errorf("after sleep the druid has form %v and %d own PV, want herself again with 38 (a druid put to sleep leaves the form)", w.GetWildShape(), w.GetHitPointsCurrent())
	}
}

// Palavra de Poder Matar on a druid in a beast form takes the beast: the form
// ends with nothing carried over, the druid keeps her own hit points (the death
// of a character is the master's to confirm, RN-03) and the undo brings the
// wolf back with its 11.
func TestPowerWordKillOnADruidInBeastFormEndsTheForm(t *testing.T) {
	t.Parallel()
	a, e := newBeastFormCasters(t)
	a.undoes(t, "Matar on the wolf form", func() {
		a.mustCast(t, a.ana, e, "Pensantus", wordKill, slotOfLevel(9), a.at(t, "Sálvia"), noCastRoll)
		v := a.vitals(t, a.bri)
		if v.GetWildShape() != nil || v.GetHitPointsCurrent() != 38 {
			t.Errorf("after Matar the druid has form %v and %d PV, want herself again with 38", v.GetWildShape(), v.GetHitPointsCurrent())
		}
	})
	if v := a.vitals(t, a.bri); v.GetWildShape().GetHitPointsCurrent() != 11 || v.GetHitPointsCurrent() != 38 {
		t.Errorf("after the undo the druid has beast %v and %d own PV, want the wolf at 11 and her own 38", v.GetWildShape(), v.GetHitPointsCurrent())
	}
}
