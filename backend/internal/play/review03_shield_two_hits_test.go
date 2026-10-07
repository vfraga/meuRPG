package play

import (
	"testing"

	playv1 "github.com/PuraFome/meuRPG/backend/gen/meurpg/play/v1"
)

// Review finding U3-12: Escudo only re-checks the one pending hit it answers, so a second hit already made (total 13 < AC 17) still lands.
func TestReview3_ShieldAlsoStopsTheOtherHitsAwaitingReaction(t *testing.T) {
	t.Parallel()
	a := newCasters(t)
	e := a.castersFightNPCFirst(t)
	before := byLabel(t, e, "Pensantus").GetHitPointsCurrent()
	first := a.mustAttack(t, a.master, e, "Capitão Goblin", sword, "Pensantus", d20(9)) // 13 vs AC 12: a hit
	second := a.mustAttack(t, a.master, e, "Capitão Goblin", sword, "Pensantus", d20(9))
	for i, r := range []*playv1.RollAttackResponse{first, second} {
		if r.GetPendingDamage().GetStatus() != playv1.PendingDamageStatus_PENDING_DAMAGE_STATUS_AWAITING_REACTION {
			t.Fatalf("hit %d = %v, want it awaiting the reaction", i+1, r.GetPendingDamage())
		}
	}
	res, err := a.useReaction(t, a.ana, e, first.GetPendingDamage().GetId(), slotOfLevel(1))
	if err != nil {
		t.Fatalf("UseReaction() error = %v", err)
	}
	if res.GetOutcome() != playv1.ReactionOutcome_REACTION_OUTCOME_STOPPED {
		t.Fatalf("outcome = %v, want stopped", res.GetOutcome())
	}
	// Escudo is up (AC 17): the other hit, total 13, would miss, so it must not hurt her.
	if _, err := a.declineReaction(t, a.ana, e, second.GetPendingDamage().GetId()); err != nil {
		t.Logf("DeclineReaction() error = %v", err)
	}
	a.h.roller.queue(7)
	if _, err := a.damage(t, a.master, e, second.GetPendingDamage().GetId(), inAppDamage); err != nil {
		t.Logf("RollDamage() error = %v", err)
	} else if _, err := a.settle(t, a.master, e, second.GetPendingDamage().GetId(), true); err != nil {
		t.Logf("ApplyPendingDamage() error = %v", err)
	}
	if got := byLabel(t, a.get(t, a.master), "Pensantus").GetHitPointsCurrent(); got != before {
		t.Errorf("Pensantus HP = %d, want %d: a hit with total 13 against the Escudo's AC 17 must not land", got, before)
	}
}
