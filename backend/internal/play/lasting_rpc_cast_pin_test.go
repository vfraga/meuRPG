package play

import (
	"testing"

	"connectrpc.com/connect"

	charactersv1 "github.com/PuraFome/meuRPG/backend/gen/meurpg/characters/v1"
	playv1 "github.com/PuraFome/meuRPG/backend/gen/meurpg/play/v1"
)

// TestLastingRPCACastOutsideACombatLeavesItsEffects freezes castCharacterEffects: which targets get
// a record, what the same spell does to its earlier cast, and what a spell with a duration of
// its own is.
func TestLastingRPCACastOutsideACombatLeavesItsEffects(t *testing.T) {
	t.Parallel()
	a := newCastingParty(t)
	s := a.rpcScript(t)
	// An NPC cannot be a target of a cast outside a combat: it is not found, so the NPC branch of
	// castCharacterEffects (no record for it) is out of reach of the call.
	a.mustCastOut(t, a.bia, a.bri, blessKey, slotOfLevel(1), false, []*charactersv1.Character{a.toren})
	s.note("bless on Toren", a.rpcCharRows(t))
	// The same spell on the same target replaces the first.
	a.mustCastOut(t, a.bia, a.bri, blessKey, slotOfLevel(1), false, []*charactersv1.Character{a.toren, a.pens})
	s.note("bless again on Toren and Pensantus", a.rpcCharRows(t))
	// A base armor class: the armor class is worked out for each target.
	a.mustCastOut(t, a.ana, a.pens, mageArmorKey, slotOfLevel(1), false, []*charactersv1.Character{a.toren})
	s.note("mage armor on Toren", a.rpcCharRows(t))
	s.note("the events", a.rpcEvents(t, eventLastingEnded))
	s.want("cast_outside")
}

// TestLastingRPCACastThatEndsWhileTheFightRunsTakesItsEffectsOutOfIt freezes (a finding:
// AbandonCast of an active cast leaves its effects in the combat, nothing ends them)
// what AbandonCast does to the effects the cast left in a combat.
func TestLastingRPCACastThatEndsWhileTheFightRunsTakesItsEffectsOutOfIt(t *testing.T) {
	t.Parallel()
	a := newCastingParty(t)
	s := a.rpcScript(t)
	bless := a.mustCastOut(t, a.bia, a.bri, blessKey, slotOfLevel(1), false, []*charactersv1.Character{a.toren, a.pens}).GetCast()
	a.mustCastOut(t, a.ana, a.pens, mageArmorKey, slotOfLevel(1), false, []*charactersv1.Character{a.toren})
	a.closeFight(t)
	s.note("in the combat: rows", a.rpcClean(t, a.rpcRows(t)))
	s.note("in the combat: armor", a.rpcArmor(t))
	if err := a.interrupt(t, a.bia, bless.GetId()); err != nil {
		t.Fatalf("AbandonCast(bless) error = %v", err)
	}
	s.note("bless abandoned: rows", a.rpcClean(t, a.rpcRows(t)))
	for _, c := range a.casts(t, a.ana).GetActive() {
		if c.GetSpellKey() == mageArmorKey {
			if err := a.interrupt(t, a.ana, c.GetId()); err != nil {
				t.Fatalf("AbandonCast(mage armor) error = %v", err)
			}
		}
	}
	s.note("mage armor abandoned: rows", a.rpcClean(t, a.rpcRows(t)))
	s.note("mage armor abandoned: armor", a.rpcArmor(t))
	s.want("cast_ends_in_combat")
}

// rpcArmor prints the base armor class the effects gave each combatant.
func (a *armed) rpcArmor(t *testing.T) string {
	t.Helper()
	rows, err := a.h.pool.Query(t.Context(), `SELECT label, COALESCE(mage_armor_ac::TEXT, '-') FROM combatants ORDER BY label`)
	if err != nil {
		t.Fatalf("read the armor: %v", err)
	}
	var out string
	for rows.Next() {
		var label, ac string
		if err := rows.Scan(&label, &ac); err != nil {
			t.Fatalf("scan the armor: %v", err)
		}
		out += label + " base armor class " + ac + "\n"
	}
	rows.Close()
	if err := rows.Err(); err != nil {
		t.Fatal(err)
	}
	return out
}

// TestLastingRPCEndCombatEffectRefusals freezes the refusals of EndCombatEffect, one by one.
func TestLastingRPCEndCombatEffectRefusals(t *testing.T) {
	t.Parallel()
	a := newCasters(t)
	e := a.closeFight(t)
	toren := a.id(t, "Toren")
	end := func(u *user, edit func(*playv1.EndCombatEffectRequest)) error {
		req := &playv1.EndCombatEffectRequest{CampaignId: a.campaignID, EncounterId: e.GetId(), CombatantId: toren, Effect: playv1.CombatEffect_COMBAT_EFFECT_AID, IdempotencyKey: newKey()}
		if edit != nil {
			edit(req)
		}
		_, err := u.combat.EndCombatEffect(t.Context(), connect.NewRequest(req))
		return err
	}
	wantCode(t, "a player", end(a.caio, nil), connect.CodePermissionDenied)
	wantRefusal(t, "a bad key", end(a.master, func(r *playv1.EndCombatEffectRequest) { r.IdempotencyKey = "no" }), connect.CodeInvalidArgument, "idempotency_key must be a UUID")
	wantRefusal(t, "a bad combat id", end(a.master, func(r *playv1.EndCombatEffectRequest) { r.EncounterId = "no" }), connect.CodeNotFound, "encounter not found")
	wantRefusal(t, "a bad combatant id", end(a.master, func(r *playv1.EndCombatEffectRequest) { r.CombatantId = "no" }), connect.CodeNotFound, "combatant not found")
	wantRefusal(t, "the effect unspecified", end(a.master, func(r *playv1.EndCombatEffectRequest) { r.Effect = playv1.CombatEffect_COMBAT_EFFECT_UNSPECIFIED }), connect.CodeInvalidArgument, "effect must be AID: the other effects end by themselves")
	wantRefusal(t, "Escudo", end(a.master, func(r *playv1.EndCombatEffectRequest) { r.Effect = playv1.CombatEffect_COMBAT_EFFECT_SHIELD }), connect.CodeInvalidArgument, "effect must be AID: the other effects end by themselves")
	wantRefusal(t, "a combat that is not there", end(a.master, func(r *playv1.EndCombatEffectRequest) { r.EncounterId = newKey() }), connect.CodeNotFound, "encounter not found")
	wantRefusal(t, "a combatant that is not there", end(a.master, func(r *playv1.EndCombatEffectRequest) { r.CombatantId = newKey() }), connect.CodeNotFound, "combatant not found")
	if err := end(a.master, nil); err != nil {
		t.Errorf("EndCombatEffect() with nothing to end error = %v, want no error", err)
	}
	if _, err := a.master.combat.EndEncounter(t.Context(), connect.NewRequest(&playv1.EndEncounterRequest{CampaignId: a.campaignID, EncounterId: e.GetId(), IdempotencyKey: newKey()})); err != nil {
		t.Fatalf("EndEncounter() error = %v", err)
	}
	_, err := a.master.combat.EndCombatEffect(t.Context(), connect.NewRequest(&playv1.EndCombatEffectRequest{
		CampaignId: a.campaignID, EncounterId: e.GetId(), CombatantId: toren, Effect: playv1.CombatEffect_COMBAT_EFFECT_AID, IdempotencyKey: newKey(),
	}))
	wantEncounterBlocked(t, err, playv1.EncounterBlockedReason_ENCOUNTER_BLOCKED_REASON_ENCOUNTER_ENDED)
}
