package play

import (
	"testing"

	"connectrpc.com/connect"

	charactersv1 "github.com/PuraFome/meuRPG/backend/gen/meurpg/characters/v1"
	playv1 "github.com/PuraFome/meuRPG/backend/gen/meurpg/play/v1"
	rulesv1 "github.com/PuraFome/meuRPG/backend/gen/meurpg/rules/v1"
)

// A granted_spell with a cost of more than one use and no material components (MR-025). The
// feat is the master's and its names are invented; the spell is the SRD's. These tests need the
// database (MEURPG_TEST_DATABASE_URL).

// newCostParty is newCasters with a feat that grants Bless for two of three uses of a resource,
// with no material components.
func newCostParty(t *testing.T) (a *armed, feat string) {
	t.Helper()
	return newGracePartyWith(t, []*rulesv1.TableEffect{
		{Type: "resource", Resource: graceUses, Max: "3", Recharge: "long_rest"},
		{Type: "granted_spell", Spell: blessKey, Ability: "cha", Uses: graceUses, UsesCost: 2, NoMaterialComponents: true},
	})
}

// The cast spends the cost, a cast the uses left do not cover is refused with nothing spent, and
// the listing says the cost and the waived material.
func TestMR025_AGrantedSpellSpendsItsCostOutsideACombat(t *testing.T) {
	t.Parallel()
	a, feat := newCostParty(t)

	res, err := a.ana.casting.GetCastOptions(t.Context(), connect.NewRequest(&playv1.GetCastOptionsRequest{CampaignId: a.campaignID, CharacterId: a.pens.GetId()}))
	if err != nil {
		t.Fatalf("GetCastOptions() error = %v", err)
	}
	var g *playv1.CastingGrant
	for _, s := range res.Msg.GetSpells() {
		if s.GetSpell().GetKey() == blessKey && len(s.GetGrants()) == 1 {
			g = s.GetGrants()[0]
		}
	}
	if g == nil || g.GetUsesCost() != 2 || g.GetUsesLeft() != 3 || !g.GetReady() || !g.GetNoMaterialComponents() {
		t.Fatalf("grant = %+v, want cost 2, 3 uses left, ready, no material components", g)
	}

	if _, err := a.castOut(t, a.ana, a.pens, blessKey, nil, false, []*charactersv1.Character{a.pens}, grantedBy(feat)); err != nil {
		t.Fatalf("CastSpellOutsideCombat(bless through the grant) error = %v", err)
	}
	if used, total := resourceUsed(a.vitals(t, a.pens), graceUses); used != 2 || total != 3 {
		t.Errorf("grant uses = %d of %d, want 2 of 3", used, total)
	}
	// One use is left and the cast costs two.
	_, err = a.castOut(t, a.ana, a.pens, blessKey, nil, false, []*charactersv1.Character{a.pens}, grantedBy(feat))
	wantCastingBlocked(t, "a cast the uses left do not cover", err, playv1.CastingBlockedReason_CASTING_BLOCKED_REASON_NO_USES)
	if used, _ := resourceUsed(a.vitals(t, a.pens), graceUses); used != 2 {
		t.Errorf("a refused cast spent uses: %d, want 2", used)
	}
}

// In a combat, the option says the cost; the cast spends it and the undo gives all of it back.
func TestMR025_AGrantedSpellCostIsSpentAndGivenBackInCombat(t *testing.T) {
	t.Parallel()
	a, feat := newCostParty(t)
	e := a.castersFight(t, 1)
	granted := func(r *playv1.CastSpellRequest) { r.GrantedBy = feat; noCastRoll(r) }

	a.undoes(t, "the costly grant", func() {
		if _, err := a.cast(t, a.ana, e, "Pensantus", blessKey, nil, a.at(t, "Pensantus", "Toren"), granted); err != nil {
			t.Fatalf("CastSpell(bless through the grant) error = %v", err)
		}
		if used, _ := resourceUsed(a.vitals(t, a.pens), graceUses); used != 2 {
			t.Errorf("grant uses after the cast = %d, want 2", used)
		}
	})
	if used, _ := resourceUsed(a.vitals(t, a.pens), graceUses); used != 0 {
		t.Errorf("grant uses after the undo = %d, want 0", used)
	}
}

// newCostSummoner is newCasters with a feat that grants Conjure Animals (a summoning spell of the third circle) for two of
// three uses of a resource.
func newCostSummoner(t *testing.T) (a *armed, feat string) {
	t.Helper()
	return newGracePartyWith(t, []*rulesv1.TableEffect{
		{Type: "resource", Resource: graceUses, Max: "3", Recharge: "long_rest"},
		{Type: "granted_spell", Spell: conjureAnimals, Ability: "cha", Uses: graceUses, UsesCost: 2},
	})
}

// castDirewolves casts the grant's Conjure Animals in a combat as Pensantus.
func (a *armed) castDirewolves(t *testing.T, e *playv1.Encounter, feat string) (*playv1.CastSpellResponse, error) {
	t.Helper()
	return a.cast(t, a.ana, e, "Pensantus", conjureAnimals, nil, nil, func(r *playv1.CastSpellRequest) {
		r.GrantedBy = feat
		r.Roll = &playv1.CastSpellRequest_D20Face{D20Face: 11}
		r.Summon = &playv1.SummonChoice{Option: 1, CreatureKeys: []string{direWolf, direWolf}}
	})
}

// A summoning spell is not a grant's to give, so a cost on a summon cannot be reached: the cast through a granted_spell
// with a cost of two is refused and spends nothing. (The summon of a feature keeps its own grant, which costs one use:
// TestMR578_ATableFeatureSummonsInACombatAndTheUndoGivesTheUseBack.)
func TestMR025_AGrantedSummonIsRefusedAndSpendsNothing(t *testing.T) {
	t.Parallel()
	a, feat := newCostSummoner(t)
	e := a.castersFight(t, 1)
	_, err := a.castDirewolves(t, e, feat)
	if connect.CodeOf(err) != connect.CodeInvalidArgument {
		t.Fatalf("CastSpell(a summoning spell through a grant) error = %v, want invalid_argument", err)
	}
	if used, _ := resourceUsed(a.vitals(t, a.pens), graceUses); used != 0 {
		t.Errorf("a refused summon spent uses: %d, want 0", used)
	}
}

// A cast through a grant with fewer uses left than its cost is refused with NO_USES, and nothing is spent.
func TestMR025_AGrantedSpellCastWithTooFewUsesLeftIsRefusedInCombat(t *testing.T) {
	t.Parallel()
	a, feat := newCostParty(t)
	e := a.castersFight(t, 1)
	granted := func(r *playv1.CastSpellRequest) { r.GrantedBy = feat; noCastRoll(r) }
	if _, err := a.cast(t, a.ana, e, "Pensantus", blessKey, nil, a.at(t, "Pensantus", "Toren"), granted); err != nil {
		t.Fatalf("CastSpell(bless through the grant, 3 uses left) error = %v", err)
	}
	// One use is left and the cast costs two.
	_, err := a.cast(t, a.ana, a.turnOf(t, "Pensantus"), "Pensantus", blessKey, nil, a.at(t, "Pensantus", "Toren"), granted)
	wantBlockedBy(t, "a cast with 1 use left at cost 2", err, blockedNoUses)
	if used, _ := resourceUsed(a.vitals(t, a.pens), graceUses); used != 2 {
		t.Errorf("a refused cast spent uses: %d, want 2", used)
	}
}

// An event stored before uses_cost existed has no grant_cost: its undo gives back one use.
func TestMR025_TheUndoOfAnEventStoredWithoutAGrantCostGivesBackOneUse(t *testing.T) {
	t.Parallel()
	a, feat := newCostParty(t)
	e := a.castersFight(t, 1)
	if _, err := a.cast(t, a.ana, e, "Pensantus", blessKey, nil, a.at(t, "Pensantus", "Toren"), func(r *playv1.CastSpellRequest) { r.GrantedBy = feat; noCastRoll(r) }); err != nil {
		t.Fatalf("CastSpell(bless through the grant) error = %v", err)
	}
	if used, _ := resourceUsed(a.vitals(t, a.pens), graceUses); used != 2 {
		t.Fatalf("grant uses after the cast = %d, want 2", used)
	}
	// What the old server stored: the same event without the cost.
	tag, err := a.h.pool.Exec(t.Context(), `UPDATE session_events SET payload = payload - 'grant_cost' WHERE payload ? 'grant_cost'`)
	if err != nil || tag.RowsAffected() != 1 {
		t.Fatalf("strip grant_cost: %v, rows = %d, want 1", err, tag.RowsAffected())
	}
	a.undoLast(t, a.get(t, a.master))
	if used, _ := resourceUsed(a.vitals(t, a.pens), graceUses); used != 1 {
		t.Errorf("grant uses after the undo of an old event = %d, want 1 (one use given back of the two)", used)
	}
}
