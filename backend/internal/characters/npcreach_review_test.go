package characters

import (
	"testing"

	"github.com/PuraFome/meuRPG/backend/internal/rules"
)

// TestRulesReviewCreaturesReach: an NPC made from a creature keeps its melee
// attacks melee. A reach-10 attack and a "melee or ranged" spear are melee ones
// in SRD 5.1 (opportunity attacks need a melee attack).
func TestRulesReviewCreaturesReach(t *testing.T) {
	t.Parallel()
	content := loadRules(t)
	for _, tc := range []struct {
		key   string
		index int
		reach int
	}{
		{"monster:hill-giant", 0, 10},
		{"monster:guard", 0, 5},
		{"monster:bandit-captain", -1, 5}, // some kept attack must be melee
	} {
		b, err := npcSheetFromCreature(content, tc.key)
		if err != nil {
			t.Fatal(err)
		}
		d := basicDerived(content, b)
		t.Logf("%s sheet attacks: %v", tc.key, b.GetAttacks())
		for i, a := range d.Attacks {
			t.Logf("  %s: Melee=%v RangeFt=%d", a.Name, a.Melee, a.RangeFt)
			_ = i
		}
		if tc.index >= 0 {
			if len(d.Attacks) <= tc.index || !d.Attacks[tc.index].Melee {
				t.Errorf("%s: attack %d should be Melee (reach %d), got %+v", tc.key, tc.index, tc.reach, d.Attacks)
			}
			continue
		}
		any := false
		for _, a := range d.Attacks {
			any = any || a.Melee
		}
		if !any {
			t.Errorf("%s: no melee attack in %+v", tc.key, d.Attacks)
		}
	}
}

// TestRulesReviewCreaturesNoMelee counts the creatures with a melee stat-block
// action whose basic sheet has no melee attack at all.
func TestRulesReviewCreaturesNoMelee(t *testing.T) {
	t.Parallel()
	content := loadRules(t)
	var bad []string
	total := 0
	list, _ := content.ListCreatures(rules.CreatureFilter{})
	for _, e := range list {
		m, ok := content.MonsterDerived(e.Key)
		if !ok {
			continue
		}
		hasMelee := false
		for _, a := range m.Attacks {
			hasMelee = hasMelee || a.Melee
		}
		if !hasMelee {
			continue
		}
		total++
		b, err := npcSheetFromCreature(content, e.Key)
		if err != nil || len(b.GetAttacks()) == 0 {
			continue
		}
		any := false
		for _, a := range basicDerived(content, b).Attacks {
			any = any || a.Melee
		}
		if !any {
			bad = append(bad, e.Key)
		}
	}
	t.Logf("%d of %d creatures with a melee attack have none on the basic sheet: %v", len(bad), total, bad)
	if len(bad) > 0 {
		t.Errorf("%d creatures lose every melee attack: %v", len(bad), bad)
	}
}
