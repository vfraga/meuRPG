package play

import (
	"testing"

	"connectrpc.com/connect"

	playv1 "github.com/PuraFome/meuRPG/backend/gen/meurpg/play/v1"
)

// The characterization of the rows of the effects: making them (addEffects), what they leave on
// the combatants (refreshCombatants), ending them (endEffectRows) and what follows (afterEffectEnded).
// See lasting_core_frozen_test.go.

// coreEffectID is the id of the effect row of a source key on the combatant with the label.
func (a *armed) coreEffectID(t *testing.T, e *playv1.Encounter, label, key string) string {
	t.Helper()
	var id string
	if err := a.h.pool.QueryRow(t.Context(), `SELECT s.id::TEXT FROM combatant_states s WHERE s.encounter_id = $1 AND s.combatant_id = $2 AND s.source_key = $3 AND s.kind = 'effect'`,
		e.GetId(), a.id(t, label), key).Scan(&id); err != nil {
		t.Fatalf("find the effect %s on %s: %v", key, label, err)
	}
	return id
}

// coreEndEffect has the master end the effect of the key on the label with the scope.
func (a *armed) coreEndEffect(t *testing.T, e *playv1.Encounter, label, key string, scope playv1.EffectEndScope) error {
	t.Helper()
	_, err := a.master.lasting.EndLastingEffect(t.Context(), connect.NewRequest(&playv1.EndLastingEffectRequest{
		CampaignId: a.campaignID, EncounterId: e.GetId(), IdempotencyKey: newKey(), EffectId: a.coreEffectID(t, e, label, key), Scope: scope,
	}))
	return err
}

func withCaster(t *testing.T, a *armed, label string) func(*playv1.AddLastingEffectRequest) {
	t.Helper()
	return func(r *playv1.AddLastingEffectRequest) { r.CasterId = a.id(t, label) }
}

// One casting on several targets is several rows with a group and a key; the master ends it as
// one effect with one line, and casting the same spell again on a target replaces its row.
func TestLastingCoreACastingOnSeveralTargetsEndsTogether(t *testing.T) {
	t.Parallel()
	a := newCasters(t)
	e := a.closeFight(t)
	var l coreLog
	add := func(edit func(*playv1.AddLastingEffectRequest)) {
		t.Helper()
		a.mustAddEffect(t, e, "spell:bless", []string{"Toren", "Pensantus", "Brisa"}, edit)
	}
	add(func(r *playv1.AddLastingEffectRequest) { withCaster(t, a, "Brisa")(r); a.rounds(t, 4, "Brisa")(r) })
	l.say("a casting on three targets:\n%s%s", a.coreEffects(t, e.GetId()), a.coreCombatants(t, e.GetId(), "Brisa"))
	a.mustAddEffect(t, e, "spell:bless", []string{"Toren"}, a.rounds(t, 9, "Toren"))
	l.say("cast again on Toren, no caster:\n%s", a.coreEffects(t, e.GetId()))
	if err := a.coreEndEffect(t, e, "Pensantus", "spell:bless", playv1.EffectEndScope_EFFECT_END_SCOPE_THIS); err != nil {
		t.Fatalf("EndLastingEffect(THIS) error = %v", err)
	}
	l.say("the master ends the casting of Pensantus:\n%s%s", a.coreEffects(t, e.GetId()), a.coreCombatants(t, e.GetId(), "Brisa"))
	if err := a.coreEndEffect(t, e, "Toren", "spell:bless", playv1.EffectEndScope_EFFECT_END_SCOPE_CONCENTRATION_GROUP); err != nil {
		t.Fatalf("EndLastingEffect(GROUP) error = %v", err)
	}
	l.say("the master ends the last one:\n%s", a.coreEffects(t, e.GetId()))
	l.say("events:\n%s", a.coreEvents(t, eventLastingAdded, eventLastingEnded, eventConditionsSet))
	wantLastingGolden(t, "lasting_core", "core_castings", a.coreNames(t, l.String()))
}

// What the effects leave on a combatant is worked out again after every change: the conditions
// the effects give come and go and the ones set by hand stay, the armor class, the speed, the
// actions, and the exhaustion a character carries (its vitals').
func TestLastingCoreWhatTheEffectsLeaveOnTheCombatants(t *testing.T) {
	t.Parallel()
	a := newCasters(t)
	e := a.closeFight(t)
	var l coreLog
	snap := func(what string, labels ...string) {
		t.Helper()
		l.say("%s:\n%s", what, a.coreCombatants(t, e.GetId(), labels...))
	}
	a.setConditions(t, e, "Brisa", "condition:poisoned")
	a.mustAddEffect(t, e, "spell:haste", []string{"Toren"}, func(r *playv1.AddLastingEffectRequest) {
		withCaster(t, a, "Pensantus")(r)
		a.rounds(t, 3, "Pensantus")(r)
	})
	a.mustAddEffect(t, e, "spell:longstrider", []string{"Pensantus"}, nil)
	a.mustAddEffect(t, e, "spell:hold-person", []string{"Goblin"}, a.rounds(t, 3, "Pensantus"))
	a.mustAddEffect(t, e, "spell:hideous-laughter", []string{"Brisa"}, a.rounds(t, 3, "Pensantus"))
	a.mustAddEffect(t, e, "spell:mage-armor", []string{"Pensantus"}, nil)
	snap("haste, longstrider, hold person, hideous laughter on a poisoned Brisa, mage armor")
	// The exhaustion of a character is its vitals': the combatant's copy is made again.
	if _, err := a.setExhaustion(t, e, "Toren", 5, 0, false); err != nil {
		t.Fatalf("SetExhaustion(5) error = %v", err)
	}
	a.execSQL(t, `UPDATE combatants SET exhaustion_level = 0 WHERE id = $1`, a.id(t, "Toren"))
	a.mustAddEffect(t, e, "spell:shield-of-faith", []string{"Toren"}, a.rounds(t, 3, "Toren"))
	snap("Toren at exhaustion 5 after a copy that was out of date and shield of faith", "Toren")
	if err := a.coreEndEffect(t, e, "Brisa", "spell:hideous-laughter", playv1.EffectEndScope_EFFECT_END_SCOPE_THIS); err != nil {
		t.Fatalf("EndLastingEffect() error = %v", err)
	}
	if err := a.coreEndEffect(t, e, "Pensantus", "spell:mage-armor", playv1.EffectEndScope_EFFECT_END_SCOPE_THIS); err != nil {
		t.Fatalf("EndLastingEffect() error = %v", err)
	}
	snap("the laughter and the armor ended: the hand-set condition stays", "Brisa", "Pensantus")
	l.say("effects:\n%s", a.coreEffects(t, e.GetId()))
	l.say("events:\n%s", a.coreEvents(t, eventLastingAdded, eventLastingEnded, eventConditionsSet, eventExhaustion))
	wantLastingGolden(t, "lasting_core", "core_leave", a.coreNames(t, l.String()))
}

// A caster that becomes incapacitated loses its concentration, and the effects it held end with it.
func TestLastingCoreAnIncapacitatedCasterLosesItsConcentration(t *testing.T) {
	t.Parallel()
	a := newCasters(t)
	e := a.closeFight(t)
	var l coreLog
	e = a.passTo(t, e, "Pensantus")
	a.h.roller.queue(1)
	if _, err := a.cast(t, a.ana, e, "Pensantus", holdPerson, slotOfLevel(2), a.at(t, "Goblin"), noCastRoll); err != nil {
		t.Fatalf("CastSpell(Hold Person) error = %v", err)
	}
	l.say("after the cast:\n%s%s", a.coreEffects(t, e.GetId()), a.coreCombatants(t, e.GetId(), "Pensantus", "Goblin"))
	a.mustAddEffect(t, e, "spell:hideous-laughter", []string{"Pensantus"}, a.rounds(t, 3, "Toren"))
	l.say("Pensantus laughs:\n%s%s", a.coreEffects(t, e.GetId()), a.coreCombatants(t, e.GetId(), "Pensantus", "Goblin"))
	l.say("events:\n%s", a.coreEvents(t, eventLastingAdded, eventLastingEnded, eventConditionsSet))
	wantLastingGolden(t, "lasting_core", "core_incapacitated_caster", a.coreNames(t, l.String()))
}

// What an effect leaves when it ends: the lethargy after Haste, the temporary hit points
// Heroism gave go with it (a character's and an NPC's), a fire that ends takes the web it was
// burning with it.
func TestLastingCoreWhatFollowsAnEffectThatEnded(t *testing.T) {
	t.Parallel()
	a := newCasters(t)
	e := a.closeFight(t)
	var l coreLog
	a.mustAddEffect(t, e, "spell:heroism", []string{"Toren", "Goblin"}, withCaster(t, a, "Pensantus"))
	a.mustAddEffect(t, e, "spell:haste", []string{"Pensantus"}, a.rounds(t, 9, "Pensantus"))
	// Toren's turn is next (round 2): he and the goblin (in round 1) start theirs with Heroism's
	// temporary hit points; the goblin's second turn adds nothing, it has them.
	a.turnOf(t, "Toren")
	l.say("Toren's turn in round 2:\n%s%s", a.coreCombatants(t, e.GetId(), "Toren", "Goblin"), a.coreVitals(t, "Toren", a.toren))
	a.turnOf(t, "Goblin")
	l.say("the goblin's turn in round 2:\n%s", a.coreCombatants(t, e.GetId(), "Goblin"))
	a.mustAddEffect(t, e, "spell:web", []string{"Goblin"}, a.rounds(t, 9, "Pensantus"))
	a.mustAddEffect(t, e, "effect:web-fire", []string{"Goblin"}, nil)
	l.say("a web and a fire on it:\n%s", a.coreEffects(t, e.GetId()))
	// The fire is put out by the master: the webs of its targets burn with it.
	if err := a.coreEndEffect(t, e, "Goblin", "effect:web-fire", playv1.EffectEndScope_EFFECT_END_SCOPE_THIS); err != nil {
		t.Fatalf("EndLastingEffect(fire) error = %v", err)
	}
	l.say("the fire ended:\n%s", a.coreEffects(t, e.GetId()))
	// Heroism ends by the master's hand: the temporary hit points go.
	if err := a.coreEndEffect(t, e, "Toren", "spell:heroism", playv1.EffectEndScope_EFFECT_END_SCOPE_THIS); err != nil {
		t.Fatalf("EndLastingEffect(heroism) error = %v", err)
	}
	l.say("heroism ended:\n%s%s", a.coreCombatants(t, e.GetId(), "Toren", "Goblin"), a.coreVitals(t, "Toren", a.toren))
	// Haste ends by the master's hand: the lethargy follows.
	if err := a.coreEndEffect(t, e, "Pensantus", "spell:haste", playv1.EffectEndScope_EFFECT_END_SCOPE_THIS); err != nil {
		t.Fatalf("EndLastingEffect(haste) error = %v", err)
	}
	l.say("haste ended:\n%s%s", a.coreEffects(t, e.GetId()), a.coreCombatants(t, e.GetId(), "Pensantus"))
	l.say("events:\n%s", a.coreEvents(t, eventLastingAdded, eventLastingEnded, eventLastingTriggered))
	wantLastingGolden(t, "lasting_core", "core_follow_ups", a.coreNames(t, l.String()))
}

// A creature that starts its turn under an effect with a damage of its own (Teia em Chamas,
// "effect:web-fire") takes the burn and the turn passes to it: the row of that damage has neither an
// attacker nor a trap point, and the CHECK of pending_damages accepts it by its effect source.
func TestLastingCoreAnEffectWithItsOwnDamageLetsTheTurnPassAndBurns(t *testing.T) {
	t.Parallel()
	a := newCasters(t)
	e := a.closeFight(t)
	var l coreLog
	a.mustAddEffect(t, e, "effect:web-fire", []string{"Goblin"}, nil)
	a.passTo(t, e, "Brisa")
	a.h.roller.queue(1, 2)
	_, err := a.endTurnRaw(t, a.master, a.get(t, a.master), true)
	l.say("the turn that passes to the goblin -> %s", coreRefusal(err))
	l.say("current: %q\n%s%s", a.get(t, a.master).GetCurrentCombatantId(), a.coreEffects(t, e.GetId()), a.coreCombatants(t, e.GetId(), "Goblin", "Brisa"))
	l.say("events:\n%s", a.coreEvents(t, eventLastingTriggered, eventTurnEnded))
	wantLastingGolden(t, "lasting_core", "core_web_fire_burns", a.coreNames(t, l.String()))
}

// What a request for an effect decides about its rows: the duration kinds, the audience, the
// visibility, the label, the saving throw, a condition with no catalog entry.
func TestLastingCoreAddingEffectsKeepsWhatTheRequestSays(t *testing.T) {
	t.Parallel()
	a := newCasters(t)
	e := a.closeFight(t)
	var l coreLog
	kind := func(k playv1.EffectDurationKind, rounds int32, anchor string) func(*playv1.AddLastingEffectRequest) {
		return func(r *playv1.AddLastingEffectRequest) {
			r.Duration = &playv1.EffectDurationChoice{Kind: k, Rounds: rounds}
			if anchor != "" {
				r.Duration.AnchorCombatantId = a.id(t, anchor)
			}
		}
	}
	add := func(key string, targets []string, edit func(*playv1.AddLastingEffectRequest)) {
		t.Helper()
		if _, err := a.addEffect(t, e, key, targets, edit); err != nil {
			l.say("add %s -> %s", key, coreRefusal(err))
		}
	}
	add("condition:poisoned", []string{"Goblin"}, nil)
	add("condition:frightened", []string{"Toren"}, kind(playv1.EffectDurationKind_EFFECT_DURATION_KIND_UNTIL_END_OF_TURN_OF, 0, "Pensantus"))
	add("condition:exhaustion", []string{"Toren"}, nil)
	add("spell:hold-person", []string{"Goblin"}, func(r *playv1.AddLastingEffectRequest) {
		dc, visible := int32(13), false
		r.SaveDc, r.PlayerVisible, r.PlayerLabel = &dc, &visible, "Imóvel"
		r.CasterId, r.Audience = a.id(t, "Pensantus"), playv1.EffectAudience_EFFECT_AUDIENCE_ALL
	})
	add("spell:bless", []string{"Brisa"}, func(r *playv1.AddLastingEffectRequest) {
		kind(playv1.EffectDurationKind_EFFECT_DURATION_KIND_UNTIL_START_OF_TURN_OF, 0, "Toren")(r)
		r.Audience = playv1.EffectAudience_EFFECT_AUDIENCE_OWNER
	})
	add("spell:enhance-ability", []string{"Toren"}, func(r *playv1.AddLastingEffectRequest) {
		kind(playv1.EffectDurationKind_EFFECT_DURATION_KIND_LONG_REST, 0, "")(r)
		r.AbilityKey = "dex"
	})
	add("spell:enhance-ability", []string{"Pensantus"}, func(r *playv1.AddLastingEffectRequest) { r.AbilityKey = "luck" })
	add("spell:heroism", []string{"Brisa"}, func(r *playv1.AddLastingEffectRequest) {
		withCaster(t, a, "Brisa")(r)
		kind(playv1.EffectDurationKind_EFFECT_DURATION_KIND_CONCENTRATION, 0, "")(r)
		r.Concentration = new(false)
	})
	add("spell:guidance", []string{"Toren"}, withCaster(t, a, "Pensantus"))
	add("effect:web-fire", []string{"Toren"}, nil)
	add("effect:lethargy", []string{"Toren"}, nil)
	add("spell:unknown", []string{"Toren"}, nil)
	l.say("effects:\n%s", a.coreEffects(t, e.GetId()))
	l.say("combatants:\n%s", a.coreCombatants(t, e.GetId()))
	l.say("events:\n%s", a.coreEvents(t, eventLastingAdded, eventConditionsSet))
	wantLastingGolden(t, "lasting_core", "core_adding", a.coreNames(t, l.String()))
}

// Two effects that ask a saving throw at the end of the same turn open two windows in a fixed
// order, and the turn waits for both.
func TestLastingCoreTwoEndSavesOpenTogether(t *testing.T) {
	t.Parallel()
	a := newCasters(t)
	e := a.closeFight(t)
	a.mustAddEffect(t, e, "spell:hold-person", []string{"Goblin"}, a.rounds(t, 5, "Pensantus"))
	a.mustAddEffect(t, e, "spell:hideous-laughter", []string{"Goblin"}, a.rounds(t, 5, "Pensantus"))
	e = a.passTo(t, e, "Goblin")
	if _, err := a.endTurnRaw(t, a.master, e, true); err != nil {
		t.Fatalf("EndTurn() error = %v", err)
	}
	var abilities, keys []string
	for _, w := range a.windows(t, a.master) {
		if w.GetKind() == playv1.ReactionKind_REACTION_KIND_EFFECT_SAVE {
			abilities = append(abilities, w.GetEffectSave().GetAbility())
			keys = append(keys, w.GetEffectSave().GetSourceNamePt())
		}
	}
	if len(keys) != 2 {
		t.Fatalf("the windows = %v (%v), want the two saves", keys, abilities)
	}
	if cur := a.get(t, a.master).GetCurrentCombatantId(); cur != "" && cur != a.id(t, "Goblin") {
		t.Errorf("the turn passed to %q before the saves were answered", cur)
	}
}
