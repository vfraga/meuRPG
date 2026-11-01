package play

import (
	"context"
	"testing"

	charactersv1 "github.com/PuraFome/meuRPG/backend/gen/meurpg/characters/v1"
	playv1 "github.com/PuraFome/meuRPG/backend/gen/meurpg/play/v1"
)

// The characterization of the effects a single roll spends (spendOne): see lasting_core_frozen_test.go.
// The roll that used the die is the caller's, so these tests call spendOne in a transaction of
// their own, the way each roll does, with the combat of the change loaded or not.

func (a *armed) coreSpend(t *testing.T, encounterID string, dice ...effectDie) {
	t.Helper()
	a.coreInTx(t, encounterID, func(ctx context.Context, c *combatTx) error {
		return a.h.svc.spendOnceEffects(ctx, c, dice)
	})
}

func (a *armed) coreCharacterEffectID(t *testing.T, characterID, key string) string {
	t.Helper()
	var id string
	if err := a.h.pool.QueryRow(t.Context(), `SELECT id::TEXT FROM character_effects WHERE character_id = $1 AND source_key = $2`, characterID, key).Scan(&id); err != nil {
		t.Fatalf("find the effect %s of the character: %v", key, err)
	}
	return id
}

// Out of a combat the effect a roll spent is a row of the character's: the whole casting that
// held it ends with it, and a row the master gave goes alone.
func TestLastingCoreARollSpendsAnEffectOutsideACombat(t *testing.T) {
	t.Parallel()
	a := newBlessersParty(t)
	var l coreLog
	toren := a.toren.GetId()
	// An effect the master gave, with no casting behind it.
	gift := a.mustGiveEffect(t, guidanceKey, []*charactersv1.Character{a.toren})
	a.coreSpend(t, "", effectDie{Once: true, EffectID: gift, CharacterID: toren})
	l.say("the master's gift spent:\n%s", a.coreCharacterEffects(t, toren))
	// An effect a casting made: the casting ends.
	cast := a.mustCastOut(t, a.bia, a.bri, guidanceKey, nil, false, []*charactersv1.Character{a.toren}).GetCast()
	a.coreSpend(t, "", effectDie{Once: true, EffectID: a.coreCharacterEffectID(t, toren, guidanceKey), CharacterID: toren})
	l.say("the cast spent: the effects:\n%scast status %v", a.coreCharacterEffects(t, toren), a.castByID(t, a.master, cast.GetId()).GetStatus())
	// A die that is not once, one with no effect, an effect that is gone and one nobody has.
	other := a.mustGiveEffect(t, guidanceKey, []*charactersv1.Character{a.toren})
	a.coreSpend(t, "", effectDie{Once: false, EffectID: other, CharacterID: toren}, effectDie{Once: true}, effectDie{Once: true, EffectID: gift, CharacterID: toren},
		effectDie{Once: true, EffectID: "00000000-0000-4000-8000-000000000001"})
	l.say("nothing to spend:\n%s", a.coreCharacterEffects(t, toren))
	wantLastingGolden(t, "lasting_core", "once_outside", a.coreNames(t, l.String()))
}

// In a combat the effect is a state of the character's combatant. The caster stops concentrating
// on a spell that was held by its concentration; any other effect ends with the reason "used".
func TestLastingCoreARollSpendsAnEffectInACombat(t *testing.T) {
	t.Parallel()
	a := newCasters(t)
	e := a.closeFight(t)
	var l coreLog
	toren := a.toren.GetId()
	spend := func(enc string, key, label string) {
		t.Helper()
		a.coreSpend(t, enc, effectDie{Once: true, EffectID: a.coreEffectID(t, e, label, key), CharacterID: toren})
	}
	snap := func(what string) {
		t.Helper()
		l.say("%s:\n%s%s", what, a.coreEffects(t, e.GetId()), a.coreCombatants(t, e.GetId(), "Brisa", "Toren"))
	}
	// Held by the concentration of Brisa: she stops concentrating.
	a.mustAddEffect(t, e, resistanceKey, []string{"Toren"}, withCaster(t, a, "Brisa"))
	snap("resistance held by Brisa's concentration")
	spend(e.GetId(), resistanceKey, "Toren")
	snap("spent")
	// With no caster nothing holds it.
	a.mustAddEffect(t, e, resistanceKey, []string{"Toren"}, nil)
	spend(e.GetId(), resistanceKey, "Toren")
	snap("spent with no caster")
	// A caster that concentrates on something else: the effect ends alone, the concentration stays.
	a.mustAddEffect(t, e, resistanceKey, []string{"Toren"}, withCaster(t, a, "Brisa"))
	a.execSQL(t, `UPDATE combatants SET concentration_spell = 'spell:bless' WHERE id = $1`, a.id(t, "Brisa"))
	spend(e.GetId(), resistanceKey, "Toren")
	snap("spent while the caster concentrates on another spell")
	// An effect that is not in the combat of the change: nothing happens.
	a.coreSpend(t, e.GetId(), effectDie{Once: true, EffectID: "00000000-0000-4000-8000-000000000002", CharacterID: toren})
	// With no combat in the change, the character's combat is found by the character.
	a.mustAddEffect(t, e, resistanceKey, []string{"Toren"}, nil)
	a.coreSpend(t, "", effectDie{Once: true, EffectID: "00000000-0000-4000-8000-000000000003", CharacterID: toren})
	a.coreSpend(t, "", effectDie{Once: true, EffectID: a.coreEffectID(t, e, "Toren", resistanceKey)})
	snap("not spent: no character, an effect nobody has")
	a.coreSpend(t, "", effectDie{Once: true, EffectID: a.coreEffectID(t, e, "Toren", resistanceKey), CharacterID: toren})
	snap("spent from outside the combat")
	l.say("events:\n%s", a.coreEvents(t, eventLastingAdded, eventLastingEnded, eventConditionsSet))
	wantLastingGolden(t, "lasting_core", "once_in_combat", a.coreNames(t, l.String()))
}

// The save of an effect that asks one takes the die of Resistance, and the die is spent.
func TestLastingCoreAnEffectSaveSpendsResistance(t *testing.T) {
	t.Parallel()
	a := newCasters(t)
	e := a.closeFight(t)
	var l coreLog
	a.mustAddEffect(t, e, resistanceKey, []string{"Toren"}, nil)
	a.mustAddEffect(t, e, holdPerson, []string{"Toren"}, a.rounds(t, 5, "Pensantus"))
	a.passTo(t, e, "Toren")
	if _, err := a.endTurnRaw(t, a.master, e, true); err != nil {
		t.Fatalf("EndTurn() error = %v", err)
	}
	w := a.windowOf(t, a.caio, playv1.ReactionKind_REACTION_KIND_EFFECT_SAVE)
	if w == nil {
		t.Fatalf("Toren's windows = %v, want the save of Hold Person", a.windows(t, a.caio))
	}
	a.h.roller.queue(10, 3)
	res, err := a.rollEffectSave(t, a.caio, e, w.GetId(), func(r *playv1.RollEffectSaveRequest) {
		r.Roll = &playv1.RollEffectSaveRequest_RollInApp{RollInApp: true}
	})
	if err != nil {
		t.Fatalf("RollEffectSave() error = %v", err)
	}
	l.say("the save: d20 %d total %d saved %v", res.GetResult().GetD20(), res.GetResult().GetTotal(), res.GetResult().GetSaved())
	for _, d := range res.GetResult().GetExtraDice() {
		l.say("  extra die d%d sign %d face %d", d.GetFaces(), d.GetSign(), d.GetFace())
	}
	l.say("%s%s", a.coreEffects(t, e.GetId()), a.coreCombatants(t, e.GetId(), "Toren"))
	l.say("events:\n%s", a.coreEvents(t, eventLastingEnded, eventLastingSaved))
	wantLastingGolden(t, "lasting_core", "once_effect_save", a.coreNames(t, l.String()))
}
