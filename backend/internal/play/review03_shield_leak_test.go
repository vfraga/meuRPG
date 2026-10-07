package play

import (
	"testing"
)

// Review finding U3-5: Escudo against a master-hidden attacker is not Secret, so other players get a REACTION log line and a combat_log_changed hint (RN-10).
func TestReview3_ShieldAgainstHiddenAttackerIsNotInOthersLog(t *testing.T) {
	t.Parallel()
	a := newCasters(t)
	e := a.hiddenCapitaoFight(t) // the Capitão is first and hidden

	other := a.caio.watch(t, a.campaignID)
	other.ready(t)

	lines := func() int {
		n := 0
		for _, r := range a.log(t, a.caio, e).GetRounds() {
			n += len(r.GetEntries())
		}
		return n
	}
	before := lines()

	hit := a.mustAttack(t, a.master, e, "Capitão Goblin", sword, "Pensantus", d20(9))
	if _, err := a.useReaction(t, a.ana, e, hit.GetPendingDamage().GetId(), slotOfLevel(1)); err != nil {
		t.Fatalf("UseReaction() error = %v", err)
	}
	a.mustEndTurn(t, a.master, e) // the sentinel: a turn change reaches the stream

	if after := lines(); after != before {
		for _, r := range a.log(t, a.caio, e).GetRounds() {
			for _, en := range r.GetEntries() {
				t.Logf("Toren's log: %v", en)
			}
		}
		t.Errorf("Toren's log has %d lines after a hidden NPC's attack met Pensantus's Escudo, want the %d it had before", after, before)
	}
	hints := 0
	for turns := 0; turns < 1; {
		ev := other.nextChange(t)
		switch {
		case ev.GetCombatLogChanged() != nil:
			hints++
		case ev.GetTurnChanged() != nil:
			turns++
		}
	}
	if hints != 0 {
		t.Errorf("Toren's stream got %d combat_log_changed hints, want none: the only change was the hidden NPC's attack and its Escudo", hints)
	}
}
