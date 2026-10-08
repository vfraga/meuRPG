package play

import (
	"fmt"
	"sort"
	"strings"
	"testing"

	playv1 "github.com/PuraFome/meuRPG/backend/gen/meurpg/play/v1"
	rulesv1 "github.com/PuraFome/meuRPG/backend/gen/meurpg/rules/v1"
)

// dualPendingSummary lists the pending damages of a cast as "NdS+B type".
func dualPendingSummary(res *playv1.CastSpellResponse) []string {
	var out []string
	for _, p := range res.GetCast().GetPendingDamages() {
		out = append(out, fmt.Sprintf("%dd%d+%d %s", p.GetDiceCount(), p.GetDiceSides(), p.GetBonus(), p.GetDamageTypeKey()))
	}
	sort.Strings(out)
	return out
}

// TestRulesReviewExpressions_IceStormBothDamageTypes: SRD 5.1 Ice Storm deals
// 2d8 bludgeoning AND 4d6 cold. A cast at one target must open both pending damages.
func TestRulesReviewExpressions_IceStormBothDamageTypes(t *testing.T) {
	t.Parallel()
	const iceStorm = "spell:ice-storm"
	a := newArmedWith(t, func(a *armed) {
		a.toren = a.caio.hero(t, a.campaignID, "Toren", "class:fighter", "race:human", 5,
			&rulesv1.AbilityScores{Strength: 16, Dexterity: 13, Constitution: 14, Intelligence: 10, Wisdom: 10, Charisma: 8}, []string{battleaxe}, nil)
		a.pens = a.ana.caster(t, a.campaignID, "Pensantus", "class:wizard", "race:gnome", 7,
			&rulesv1.AbilityScores{Strength: 10, Dexterity: 14, Constitution: 12, Intelligence: 16, Wisdom: 10, Charisma: 8}, nil, []string{fireBolt},
			[]string{iceStorm}, []string{iceStorm})
		a.bri = a.bia.caster(t, a.campaignID, "Brisa", "class:cleric", "race:human", 3,
			&rulesv1.AbilityScores{Strength: 10, Dexterity: 16, Constitution: 14, Intelligence: 10, Wisdom: 16, Charisma: 8}, []string{maceKey}, []string{sacredFlame}, nil,
			[]string{cureWounds})
	})
	e := a.castersFight(t, 1)
	a.h.roller.queue(1) // the goblin fails its save
	res := a.mustCast(t, a.ana, e, "Pensantus", iceStorm, slotOfLevel(4), a.at(t, "Goblin"), noCastRoll)
	got := dualPendingSummary(res)
	want := []string{"2d8+0 damage-type:bludgeoning", "4d6+0 damage-type:cold"}
	if strings.Join(got, "; ") != strings.Join(want, "; ") {
		t.Errorf("Ice Storm at one target opens pending damages %v; SRD 5.1 says %v (2d8 bludgeoning and 4d6 cold)", got, want)
	}
}

// TestRulesReviewExpressions_FlameStrikeSlot6: SRD 5.1 Flame Strike deals 4d6 fire AND
// 4d6 radiant, +1d6 (of a type the caster picks) per slot level above 5th.
func TestRulesReviewExpressions_FlameStrikeSlot6(t *testing.T) {
	t.Parallel()
	const flameStrike = "spell:flame-strike"
	a := newArmedWith(t, func(a *armed) {
		a.toren = a.caio.hero(t, a.campaignID, "Toren", "class:fighter", "race:human", 5,
			&rulesv1.AbilityScores{Strength: 16, Dexterity: 13, Constitution: 14, Intelligence: 10, Wisdom: 10, Charisma: 8}, []string{battleaxe}, nil)
		a.pens = a.ana.caster(t, a.campaignID, "Pensantus", "class:wizard", "race:gnome", 3,
			&rulesv1.AbilityScores{Strength: 10, Dexterity: 14, Constitution: 12, Intelligence: 16, Wisdom: 10, Charisma: 8}, nil, []string{fireBolt},
			[]string{magicMissileSpell}, []string{magicMissileSpell})
		a.bri = a.bia.caster(t, a.campaignID, "Brisa", "class:cleric", "race:human", 11,
			&rulesv1.AbilityScores{Strength: 10, Dexterity: 16, Constitution: 14, Intelligence: 10, Wisdom: 16, Charisma: 8}, []string{maceKey}, []string{sacredFlame}, nil,
			[]string{flameStrike})
	})
	e := a.castersFight(t, 1)
	e = a.mustEndTurn(t, a.ana, e) // Toren
	e = a.mustEndTurn(t, a.caio, e) // Brisa
	a.h.roller.queue(1)
	res, err := a.cast(t, a.bia, e, "Brisa", flameStrike, slotOfLevel(6), a.at(t, "Goblin"), noCastRoll)
	if err != nil {
		t.Fatalf("CastSpell(Flame Strike, slot 6) error = %v (SRD: legal cast of 4d6 fire + 4d6 radiant + 1d6)", err)
	}
	dice := 0
	for _, p := range res.GetCast().GetPendingDamages() {
		dice += int(p.GetDiceCount())
	}
	if dice != 9 {
		t.Errorf("Flame Strike at slot 6 opens %v (%d dice); SRD 5.1 says 4d6 fire + 4d6 radiant + 1d6 = 9 dice", dualPendingSummary(res), dice)
	}
}
