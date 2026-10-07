package play

import (
	"testing"

	"connectrpc.com/connect"

	mapsv1 "github.com/PuraFome/meuRPG/backend/gen/meurpg/maps/v1"
	playv1 "github.com/PuraFome/meuRPG/backend/gen/meurpg/play/v1"
	rulesv1 "github.com/PuraFome/meuRPG/backend/gen/meurpg/rules/v1"
)

// Finding U4-5: a trap firing that caught an NPC the master had HIDDEN is judged
// by the combatant's hidden flag at read time (combat_log.go trapEntry), so when
// the master later reveals the NPC the old firing (save, damage, condition,
// defeated) appears in the players' log, against the log's rule that a revealed
// combatant does not bring its old entries with it.
func TestReview4_TrapFiringOfAHiddenNPCReappearsOnReveal(t *testing.T) {
	t.Parallel()
	r := newTrapRig(t)
	statue := r.trap(t, "Estátua de Fogo", 15, 12, func(s *mapsv1.TrapSpec) {
		s.Trigger = rulesv1.TrapTrigger_TRAP_TRIGGER_MANUAL
		s.Effect = &rulesv1.TrapEffect{
			Save: &rulesv1.TrapSaveEffect{
				Ability: rulesv1.Ability_ABILITY_DEXTERITY, Dc: 12, AppliesTo: rulesv1.TrapSaveApplies_TRAP_SAVE_APPLIES_CAUGHT,
				OnFail: &rulesv1.TrapOnFail{
					Damage:    []*rulesv1.TrapDamage{{Dice: "2d6", DamageTypeKey: "damage-type:fire"}},
					Condition: &rulesv1.TrapCondition{ConditionKey: "condition:poisoned"},
				},
				OnPass: rulesv1.TrapPassOutcome_TRAP_PASS_OUTCOME_HALF,
			},
			Targets: rulesv1.TrapTargets_TRAP_TARGETS_MANUAL,
		}
	})
	e := r.fight(t)
	r.hide(t, "Goblin 1")
	r.h.roller.queue(5, 3, 4)
	if _, err := r.fireByHand(t, statue, r.id(t, "Goblin 1")); err != nil {
		t.Fatalf("FireTrap() error = %v", err)
	}

	leaks := func() (n int) {
		for _, round := range r.log(t, r.caio, r.get(t, r.caio)).GetRounds() {
			for _, en := range round.GetEntries() {
				for _, c := range en.GetTrap().GetCaught() {
					if c.GetTargetLabel() == "Goblin 1" {
						n++
						t.Logf("player sees: %v", c)
					}
				}
			}
		}
		return n
	}
	if n := leaks(); n != 0 {
		t.Fatalf("while hidden, the player's log already shows the goblin caught (%d), want nothing", n)
	}
	if _, err := r.master.combat.SetCombatantHidden(t.Context(), connect.NewRequest(&playv1.SetCombatantHiddenRequest{
		CampaignId: r.campaignID, EncounterId: e.GetId(), CombatantId: r.id(t, "Goblin 1"), IdempotencyKey: newKey(), Hidden: false,
	})); err != nil {
		t.Fatalf("SetCombatantHidden(reveal) error = %v", err)
	}
	if n := leaks(); n != 0 {
		t.Errorf("after the reveal the old firing on the formerly hidden goblin reached the player's log (%d caught); a revealed combatant must not bring its old entries", n)
	}
}
