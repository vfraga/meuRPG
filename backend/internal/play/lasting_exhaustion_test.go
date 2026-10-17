package play

import (
	"testing"

	"connectrpc.com/connect"

	charactersv1 "github.com/PuraFome/meuRPG/backend/gen/meurpg/characters/v1"

	playv1 "github.com/PuraFome/meuRPG/backend/gen/meurpg/play/v1"
)

// setExhaustion has the master set the level of a combatant.
func (a *armed) setExhaustion(t *testing.T, e *playv1.Encounter, label string, level, expected int32, confirm bool) (*playv1.SetExhaustionResponse, error) {
	t.Helper()
	res, err := a.master.lasting.SetExhaustion(t.Context(), connect.NewRequest(&playv1.SetExhaustionRequest{
		CampaignId: a.campaignID, IdempotencyKey: newKey(), EncounterId: e.GetId(), Level: level, ExpectedLevel: expected, ConfirmDeath: confirm,
		Subject: &playv1.SetExhaustionRequest_CombatantId{CombatantId: a.id(t, label)},
	}))
	if err != nil {
		return nil, err
	}
	return res.Msg, nil
}

// Exhaustion (SRD 5.1, Conditions): level 3 gives disadvantage on attack rolls, level 4 halves
// the hit point maximum, lowering a level never heals, level 6 is death and needs the master's
// confirmation. A stale expected level is refused.
func TestExhaustionLevelsHalveTheMaximumAndLevelSixAsksForConfirmation(t *testing.T) {
	t.Parallel()
	a := newCasters(t)
	e := a.closeFight(t)
	full := a.vitals(t, a.toren).GetHitPointsMax()
	if _, err := a.setExhaustion(t, e, "Toren", 2, 1, false); connect.CodeOf(err) != connect.CodeAborted {
		t.Errorf("a stale expected level: err = %v, want aborted", err)
	}
	if res, err := a.setExhaustion(t, e, "Toren", 4, 0, false); err != nil || res.GetLevel() != 4 || res.GetHitPointsMax() != full/2 {
		t.Fatalf("level 4 = %v, %v; want the maximum %d", res, err, full/2)
	}
	if v := a.vitals(t, a.toren); v.GetHitPointsMax() != full/2 || v.GetHitPointsCurrent() > full/2 || v.GetExhaustionLevel() != 4 {
		t.Errorf("Toren = max %d current %d level %d, want the halved maximum", v.GetHitPointsMax(), v.GetHitPointsCurrent(), v.GetExhaustionLevel())
	}
	if got := byLabel(t, a.get(t, a.caio), "Toren").GetExhaustionLevel(); got != 4 {
		t.Errorf("Toren's player reads level %d, want 4", got)
	}
	// Another player never reads it (owner-class); here Toren's player does, Pensantus's does not.
	if got := byLabel(t, a.get(t, a.ana), "Toren").GetExhaustionLevel(); got != 0 {
		t.Errorf("another player reads Toren's exhaustion level %d, want nothing", got)
	}
	// Lowering a level gives the maximum back and heals nothing.
	hp := a.vitals(t, a.toren).GetHitPointsCurrent()
	res, err := a.master.lasting.LowerExhaustion(t.Context(), connect.NewRequest(&playv1.LowerExhaustionRequest{
		CampaignId: a.campaignID, IdempotencyKey: newKey(), EncounterId: e.GetId(), By: 1, ExpectedLevel: 4,
		Reason:  playv1.ExhaustionLowerReason_EXHAUSTION_LOWER_REASON_MASTER,
		Subject: &playv1.LowerExhaustionRequest_CombatantId{CombatantId: a.id(t, "Toren")},
	}))
	if err != nil || res.Msg.GetLevel() != 3 || res.Msg.GetHitPointsMax() != full {
		t.Fatalf("LowerExhaustion = %v, %v; want level 3 and the full maximum", res.Msg, err)
	}
	if got := a.vitals(t, a.toren).GetHitPointsCurrent(); got != hp {
		t.Errorf("lowering healed: %d, want %d", got, hp)
	}
	// Level 3: disadvantage on attack rolls, in the sources of the roll.
	atk := a.mustAttack(t, a.caio, e, "Toren", battleaxe, "Goblin", func(r *playv1.RollAttackRequest) { r.D20Faces = []int32{15, 3} })
	found := false
	for _, s := range atk.GetRoll().GetSources() {
		found = found || s.GetKind() == playv1.AdvantageSourceKind_ADVANTAGE_SOURCE_KIND_EXHAUSTION_ATTACK
	}
	if !found || atk.GetRoll().GetMode() != playv1.RollMode_ROLL_MODE_DISADVANTAGE {
		t.Errorf("the attack at level 3 = %v, want disadvantage from the exhaustion", atk.GetRoll())
	}
	// Level 6 asks for the confirmation.
	if _, err := a.setExhaustion(t, e, "Toren", 6, 3, false); connect.CodeOf(err) != connect.CodeFailedPrecondition {
		t.Errorf("level 6 without confirmation: err = %v, want failed_precondition", err)
	}
	if _, err := a.setExhaustion(t, e, "Toren", 6, 3, true); err != nil {
		t.Fatalf("level 6 confirmed: err = %v", err)
	}
	if got := byLabel(t, a.get(t, a.master), "Toren"); got.GetDeathFailures() < 3 {
		t.Errorf("Toren at level 6 = %d death failures, want the death confirmation", got.GetDeathFailures())
	}
}

// Death ends the effects on a character and the concentration it held; living again brings back
// none of them (SRD 5.1, Dropping to 0 Hit Points; RN-03).
func TestDeathEndsTheEffectsAndReviveBringsNoneBack(t *testing.T) {
	t.Parallel()
	a := newCasters(t)
	e := a.closeFight(t)
	a.mustAddEffect(t, e, "spell:bless", []string{"Toren"}, a.rounds(t, 10, "Toren"))
	if _, err := a.setExhaustion(t, e, "Toren", 6, 0, true); err != nil {
		t.Fatalf("SetExhaustion(6) error = %v", err)
	}
	if _, err := a.master.combat.ConfirmDeath(t.Context(), connect.NewRequest(&playv1.ConfirmDeathRequest{
		CampaignId: a.campaignID, EncounterId: e.GetId(), CombatantId: a.id(t, "Toren"), IdempotencyKey: newKey(),
	})); err != nil {
		t.Fatalf("ConfirmDeath() error = %v", err)
	}
	if got := byLabel(t, a.get(t, a.master), "Toren").GetEffects(); len(got) != 0 {
		t.Errorf("the dead Toren still has %v", got)
	}
	a.execSQL(t, `INSERT INTO character_effects (campaign_id, character_id, group_id, source_key, source_kind, duration_kind)
		VALUES ($1, $2, gen_random_uuid(), 'spell:bless', 'spell', 'until_dismissed')`, a.campaignID, a.toren.GetId())
	if _, err := a.master.characters.ReviveCharacter(t.Context(), connect.NewRequest(&charactersv1.ReviveCharacterRequest{CampaignId: a.campaignID, CharacterId: a.toren.GetId(), IdempotencyKey: newKey()})); err != nil {
		t.Fatalf("ReviveCharacter() error = %v", err)
	}
	var n int
	if err := a.h.pool.QueryRow(t.Context(), `SELECT count(*) FROM character_effects WHERE character_id = $1`, a.toren.GetId()).Scan(&n); err != nil || n != 0 {
		t.Errorf("the revived Toren has %d old effects (%v), want none", n, err)
	}
}

// The line of an exhaustion change is the owner's and the master's, so the hint that the combat log
// changed goes to the master alone: a player who must not read the line is not told it exists (the
// owner is told through the vitals).
func TestSetExhaustionTellsTheLogChangedToTheMasterAlone(t *testing.T) {
	t.Parallel()
	a := newEffectsParty(t)
	e := a.closeFight(t)
	h := a.listen(t)
	if _, err := a.setExhaustion(t, e, "Toren", 1, 0, false); err != nil {
		t.Fatalf("SetExhaustion() error = %v", err)
	}
	const hint = "CombatLogChanged"
	got := a.heard(t, h)
	want := "master[EncounterChanged," + hint + ",VitalsChanged] caio[EncounterChanged,VitalsChanged] ana[EncounterChanged] bia[EncounterChanged]"
	if got != want {
		t.Errorf("heard = %s, want %s", got, want)
	}
}
