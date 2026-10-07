package play

import (
	"testing"

	"connectrpc.com/connect"
	"google.golang.org/protobuf/proto"

	mapsv1 "github.com/PuraFome/meuRPG/backend/gen/meurpg/maps/v1"
	playv1 "github.com/PuraFome/meuRPG/backend/gen/meurpg/play/v1"
	rulesv1 "github.com/PuraFome/meuRPG/backend/gen/meurpg/rules/v1"
)

// Finding U2-4: ApplyTrapDamage changes vitals of a combatant without touching the running encounter
func TestReview2_TrapDamageDoesNotTouchEncounter(t *testing.T) {
	t.Parallel()
	r := newTrapRig(t)
	hole := r.trap(t, "Fosso Dourado", 12, 7, pit("2d6"), func(s *mapsv1.TrapSpec) {
		s.Trigger = rulesv1.TrapTrigger_TRAP_TRIGGER_MANUAL
	})
	r.place(t, r.pens.GetId(), 12, 7)
	r.h.roller.queue(3, 4)
	if _, err := r.fireByHand(t, hole); err != nil {
		t.Fatalf("FireTrap() error = %v", err)
	}
	damages := r.trapDamages(t)
	if len(damages) != 1 || damages[0].GetEncounterId() != "" {
		t.Fatalf("trap damages = %v, want one outside a combat", damages)
	}
	// A combat starts later with Pensantus as a combatant.
	r.fight(t)
	w := r.watch(t, r.ana, r.campaignID)
	r.drain(w)
	before := r.get(t, r.master).GetRevision()
	hp := r.vitals(t, r.pens).GetHitPointsCurrent()
	if _, err := r.master.play.ApplyTrapDamage(t.Context(), connect.NewRequest(&playv1.ApplyTrapDamageRequest{
		CampaignId: r.campaignID, TrapDamageId: damages[0].GetId(), IdempotencyKey: newKey(), Amount: proto.Int32(hp + 50),
	})); err != nil {
		t.Fatalf("ApplyTrapDamage() error = %v", err)
	}
	if got := r.vitals(t, r.pens).GetHitPointsCurrent(); got != 0 {
		t.Fatalf("Pensantus HP = %d, want 0", got)
	}
	e := r.get(t, r.master)
	if got := byLabel(t, e, "Pensantus").GetState(); got != playv1.CombatantState_COMBATANT_STATE_DOWN {
		t.Errorf("Pensantus state = %v, want DOWN", got)
	}
	if e.GetRevision() <= before {
		t.Errorf("encounter revision after = %d, before = %d; want it raised (as AdjustCharacterVitals does)", e.GetRevision(), before)
	}
	seen := false
	for _, ev := range r.drain(w) {
		if ev.GetEncounterChanged() != nil {
			seen = true
		}
	}
	if !seen {
		t.Errorf("no encounter_changed event reached the player's stream after ApplyTrapDamage dropped a combatant to 0")
	}
}
