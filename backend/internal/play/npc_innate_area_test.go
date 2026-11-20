package play

import (
	"testing"

	"connectrpc.com/connect"

	playv1 "github.com/PuraFome/meuRPG/backend/gen/meurpg/play/v1"
)

// The master casts an NPC's innate area spell (the Pit Fiend's Fireball) the way the web sends it: with granted_by and no slot, at the grant's level.
// These tests need the database (MEURPG_TEST_DATABASE_URL).

func TestTheMasterCastsAnNPCsInnateAreaSpellThroughItsGrantWithNoSlot(t *testing.T) {
	t.Parallel()
	a, e := monsterFight(t, "monster:pit-fiend", "Diabo das profundezas")
	e = a.passTo(t, e, "Diabo das profundezas")
	var source string
	for _, s := range a.mustOptions(t, a.master, e, "Diabo das profundezas").GetOptions().GetSpells() {
		if s.GetSpell().GetKey() == fireball && s.GetGrant() != nil {
			source = s.GetGrant().GetSourceKey()
		}
	}
	if source == "" {
		t.Fatal("the Pit Fiend's options list no granted Fireball")
	}
	grantedBySource := func(r *playv1.CastSpellRequest) { r.GrantedBy = source }
	_, err := a.master.combat.PreviewSpellArea(t.Context(), connect.NewRequest(&playv1.PreviewSpellAreaRequest{
		CampaignId: a.campaignID, EncounterId: e.GetId(), CasterId: a.id(t, "Diabo das profundezas"), SpellKey: fireball, Slot: slotOfLevel(3), GrantedBy: source,
		Area: &playv1.PreviewSpellAreaRequest_Origin{Origin: &playv1.SpellOrigin{Col: 6, Row: 5}},
	}))
	if connect.CodeOf(err) != connect.CodeInvalidArgument {
		t.Errorf("a slot through a grant that allows none: code = %v, want invalid_argument", connect.CodeOf(err))
	}
	a.h.roller.queue(5, 18, 12)
	res := a.mustCast(t, a.master, e, "Diabo das profundezas", fireball, nil, a.at(t, "Toren", "Pensantus", "Brisa"), grantedBySource)
	if res.GetCast().GetSlot() != nil || len(res.GetCast().GetTargets()) != 3 {
		t.Errorf("cast = %v, want no slot and 3 targets", res.GetCast())
	}
}
