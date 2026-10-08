package characters

import (
	"testing"

	"github.com/PuraFome/meuRPG/backend/internal/rules"
)

// TestRulesReviewCombat_ReachTenIsMelee: a creature whose melee attack has a
// reach over 5 ft (Hill Giant's greatclub, reach 10 ft) must keep that attack as
// a melee one once it is a basic-sheet NPC in a combat, or it never makes
// opportunity attacks (play.meleeReachOf finds no melee attack).
func TestRulesReviewCombat_ReachTenIsMelee(t *testing.T) {
	t.Parallel()
	content := loadRules(t)

	entries, err := content.ListCreatures(rules.CreatureFilter{})
	if err != nil {
		t.Fatal(err)
	}
	affected := 0
	for _, e := range entries {
		d, ok := content.MonsterDerived(e.Key)
		if !ok {
			continue
		}
		longReach := false
		for _, a := range d.Attacks {
			if a.Melee && a.RangeFt > 5 && a.LongRangeFt == 0 {
				longReach = true
			}
		}
		if !longReach {
			continue
		}
		sheet, err := npcSheetFromCreature(content, e.Key)
		if err != nil {
			continue
		}
		meleeKept := false
		for _, a := range basicDerived(content, sheet).Attacks {
			if a.Melee && a.RangeFt > 5 {
				meleeKept = true
			}
		}
		if !meleeKept {
			affected++
			t.Logf("affected: %s (%s)", e.Key, e.NamePT)
		}
	}
	t.Logf("creatures with a melee reach > 5 ft that lose Melee in basicDerived: %d", affected)

	sheet, err := npcSheetFromCreature(content, "monster:hill-giant")
	if err != nil {
		t.Fatal(err)
	}
	found := false
	for _, a := range basicDerived(content, sheet).Attacks {
		t.Logf("hill giant attack %q RangeFt=%d Melee=%v", a.Name, a.RangeFt, a.Melee)
		if a.RangeFt == 10 {
			found = true
			if !a.Melee {
				t.Errorf("hill giant %q has reach 10 ft but Melee = false", a.Name)
			}
		}
	}
	if !found {
		t.Fatal("hill giant sheet has no attack with reach 10")
	}
	if affected > 0 {
		t.Errorf("%d creatures lose their long-reach melee attack in basicDerived", affected)
	}
}
