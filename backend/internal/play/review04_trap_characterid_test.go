package play

import (
	"testing"

	mapsv1 "github.com/PuraFome/meuRPG/backend/gen/meurpg/maps/v1"
	rulesv1 "github.com/PuraFome/meuRPG/backend/gen/meurpg/rules/v1"
)

// Finding U4-4: firingProto copies cc.Character into TrapCaught.character_id
// without gating it, so a player reading the combat log gets a visible NPC's
// character UUID ("an NPC's character is the master's secret", combat_view.go),
// although combatantToProto withholds it from the same player's GetEncounter.
func TestReview4_TrapFiringLeaksNPCCharacterID(t *testing.T) {
	t.Parallel()
	r := newTrapRig(t)
	statue := r.trap(t, "Estátua de Fogo", 15, 12, func(s *mapsv1.TrapSpec) {
		s.Trigger = rulesv1.TrapTrigger_TRAP_TRIGGER_MANUAL
		s.Effect = &rulesv1.TrapEffect{
			Damage:  []*rulesv1.TrapDamage{{Dice: "1d4", DamageTypeKey: "damage-type:fire"}},
			Targets: rulesv1.TrapTargets_TRAP_TARGETS_MANUAL,
		}
	})
	r.fight(t)
	r.h.roller.queue(2)
	if _, err := r.fireByHand(t, statue, r.id(t, "Goblin 1")); err != nil {
		t.Fatalf("FireTrap: %v", err)
	}
	enc := r.get(t, r.ana)
	if id := byLabel(t, enc, "Goblin 1").GetCharacterId(); id != "" {
		t.Fatalf("precondition: the player's GetEncounter already shows the goblin's character_id %q", id)
	}
	found := false
	for _, rd := range r.log(t, r.ana, enc).GetRounds() {
		for _, en := range rd.GetEntries() {
			for _, c := range en.GetTrap().GetCaught() {
				found = true
				if c.GetCharacterId() != "" {
					t.Errorf("player's combat log shows NPC %q character_id %q (GetEncounter withholds it)", c.GetTargetLabel(), c.GetCharacterId())
				}
			}
		}
	}
	if !found {
		t.Fatal("no trap entry in the player's log")
	}
}
