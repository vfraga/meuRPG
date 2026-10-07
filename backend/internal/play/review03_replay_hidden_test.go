package play

import (
	"strings"
	"testing"

	"connectrpc.com/connect"

	playv1 "github.com/PuraFome/meuRPG/backend/gen/meurpg/play/v1"
)

// Review finding U3-9: a retried CastSpell (same key) rebuilds its answer without the fog filter and shows a target the master hid since.
func TestReview3_ReplayedCastDoesNotShowAHiddenTarget(t *testing.T) {
	t.Parallel()
	a := newCasters(t)
	e := a.castersFight(t, 1)
	goblin := a.id(t, "Goblin")

	key := newKey()
	targets := []*playv1.SpellTarget{darts(a, t, "Goblin", 3)}
	first, err := a.castKey(t, a.ana, e, "Pensantus", magicMissileSpell, slotOfLevel(1), targets, noCastRoll, key)
	if err != nil {
		t.Fatalf("CastSpell() error = %v", err)
	}
	a.h.roller.queue(2, 2, 2)
	a.mustDamage(t, a.ana, e, first.GetCast().GetPendingDamages()[0].GetId(), inAppDamage)

	if _, err := a.master.combat.SetCombatantHidden(t.Context(), connect.NewRequest(&playv1.SetCombatantHiddenRequest{
		CampaignId: a.campaignID, EncounterId: e.GetId(), CombatantId: goblin, IdempotencyKey: newKey(), Hidden: true,
	})); err != nil {
		t.Fatalf("SetCombatantHidden() error = %v", err)
	}

	replay, err := a.castKey(t, a.ana, e, "Pensantus", magicMissileSpell, slotOfLevel(1), targets, noCastRoll, key)
	if err != nil {
		return // refusing the replay leaks nothing
	}
	if strings.Contains(replay.String(), goblin) {
		t.Errorf("the replayed cast shows the hidden goblin %s: %v", goblin, replay.GetCast())
	}
	if n := len(replay.GetCast().GetPendingDamages()); n != 0 {
		t.Errorf("the replayed cast carries %d pending damages of the hidden goblin, want none", n)
	}
}
