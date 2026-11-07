package play

import (
	"testing"

	playv1 "github.com/PuraFome/meuRPG/backend/gen/meurpg/play/v1"
)

// With the switch on (cmd/api turns it on, #573) a character whose table feature gives a reroll is asked "rolar de novo?"
// on a low d20, and a character of the same table with no such rule is not: the low face settles at once. These tests need
// the database (MEURPG_TEST_DATABASE_URL).
func TestWithTheSwitchOnOnlyTheCharacterWithTheRuleIsAsked(t *testing.T) {
	t.Parallel()
	r := usualD20RerollTable(t, true)
	e := r.closeFight(t)

	asked, err := r.attack(t, r.caio, e, "Toren", battleaxe, "Goblin", d20(1))
	if err != nil {
		t.Fatalf("RollAttack(Toren, 1) error = %v", err)
	}
	r.wantReroll(t, "Toren's low d20", asked.GetHeldRollOffer(), playv1.HeldRollKind_HELD_ROLL_KIND_WEAPON_ATTACK, 1)
	if _, err := r.answerHeldRoll(t, r.caio, asked.GetHeldRollOffer().GetHoldId(), rerollAnswer(false)); err != nil {
		t.Fatalf("AnswerHeldRoll(keep) error = %v", err)
	}

	e = r.turnOf(t, "Brisa")
	res, err := r.attack(t, r.bia, e, "Brisa", maceKey, "Goblin", d20(1))
	if err != nil {
		t.Fatalf("RollAttack(Brisa, 1) error = %v", err)
	}
	if res.GetHeldRollOffer() != nil || res.GetRoll().GetOutcome() == playv1.AttackOutcome_ATTACK_OUTCOME_UNSPECIFIED {
		t.Fatalf("Brisa's low d20 = %v; want it resolved with no question, she has no reroll rule", res)
	}
	if list := r.listHeldRolls(t, r.bia); len(list) != 0 {
		t.Errorf("Brisa's list = %v, want nothing", list)
	}
}
