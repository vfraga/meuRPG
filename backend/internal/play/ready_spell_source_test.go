package play

import (
	"testing"

	"connectrpc.com/connect"

	playv1 "github.com/PuraFome/meuRPG/backend/gen/meurpg/play/v1"
	rulesv1 "github.com/PuraFome/meuRPG/backend/gen/meurpg/rules/v1"
)

// The release takes the source the row was readied with: a spell is readied from the caster's classes (Ready names no grant), so
// a release that names a feat's grant of the same spell is refused, and spends neither a use of the grant nor the held slot.
func TestAReadiedSpellIsReleasedFromItsOwnSourceNotAGrant(t *testing.T) {
	t.Parallel()
	a, feat := newGracePartyWith(t, []*rulesv1.TableEffect{
		{Type: "resource", Resource: graceUses, Max: "3", Recharge: "long_rest"},
		{Type: "granted_spell", Spell: magicMissileSpell, Ability: "int", Uses: graceUses},
	})
	e := a.readyForRelease(t, magicMissileSpell, 1, "Capitão Goblin")
	a.mustFire(t, e, "Pensantus")
	before := a.events(t)
	_, err := a.cast(t, a.ana, e, "Pensantus", magicMissileSpell, nil, []*playv1.SpellTarget{darts(a, t, "Capitão Goblin", 3)}, func(r *playv1.CastSpellRequest) { r.GrantedBy = feat })
	wantRefusal(t, "the release naming a grant", err, connect.CodeInvalidArgument, "a readied spell is released from the source it was readied with: name no grant")
	if used, _ := resourceUsed(a.vitals(t, a.pens), graceUses); used != 0 || a.events(t) != before || a.readiedStatus(t, "Pensantus") != readiedWaiting || !a.readiedOpen(t, "Pensantus") {
		t.Errorf("after the refusal: grant uses %d, events %d (want %d), row %q, window open %v; want nothing spent", used, a.events(t), before, a.readiedStatus(t, "Pensantus"), a.readiedOpen(t, "Pensantus"))
	}
	if _, err := a.cast(t, a.ana, e, "Pensantus", magicMissileSpell, nil, []*playv1.SpellTarget{darts(a, t, "Capitão Goblin", 3)}, noCastRoll); err != nil {
		t.Fatalf("the release from its own source: %v", err)
	}
	if used, _ := resourceUsed(a.vitals(t, a.pens), graceUses); used != 0 || a.readiedStatus(t, "Pensantus") != readiedFired {
		t.Errorf("after the release: grant uses %d, row %q; want 0 and fired", used, a.readiedStatus(t, "Pensantus"))
	}
}
