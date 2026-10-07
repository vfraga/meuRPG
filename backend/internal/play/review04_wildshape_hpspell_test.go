package play

import (
	"testing"

	charactersv1 "github.com/PuraFome/meuRPG/backend/gen/meurpg/characters/v1"
	playv1 "github.com/PuraFome/meuRPG/backend/gen/meurpg/play/v1"
	rulesv1 "github.com/PuraFome/meuRPG/backend/gen/meurpg/rules/v1"
)

// Finding U4-3: readFxTarget (combat_spells_hp.go) reads a player character's hit
// points from the vitals' OWN pool even when the druid is in a Wild Shape beast
// form, while damage and healing (RN-02, damageBeast) treat the beast's separate
// pool as the creature's hit points. So Sleep judges a druid in a wolf form (beast
// 11/11, own 38/38) by 38 and says "not affected" with a pool of 15, although the
// wolf form has 11 hit points (<= 15) and should fall asleep (and lose the form).
// docs/architecture.md lists this under "Known limits" (1), so it is a documented
// gap, not a deliberate rule.
func TestReview4_HPSpellReadsDruidsOwnPoolInBeastForm(t *testing.T) {
	t.Parallel()
	var salvia *charactersv1.Character
	a := newArmedWith(t, func(a *armed) {
		a.toren = a.caio.hero(t, a.campaignID, "Toren", "class:fighter", "race:human", 5,
			&rulesv1.AbilityScores{Strength: 16, Dexterity: 13, Constitution: 14, Intelligence: 10, Wisdom: 10, Charisma: 8}, []string{battleaxe}, nil)
		spells := []string{sleepSpell}
		a.pens = a.ana.caster(t, a.campaignID, "Pensantus", "class:wizard", "race:gnome", 9,
			&rulesv1.AbilityScores{Strength: 10, Dexterity: 14, Constitution: 12, Intelligence: 16, Wisdom: 10, Charisma: 8}, nil, nil, spells, spells)
		a.bri = a.bia.caster(t, a.campaignID, "Sálvia", "class:druid", "race:half-elf", 5,
			&rulesv1.AbilityScores{Strength: 10, Dexterity: 12, Constitution: 14, Intelligence: 10, Wisdom: 16, Charisma: 8}, nil, nil, nil,
			[]string{conjureAnimals})
		salvia = a.bri
	})
	a.start(t, plan{
		npcs:     []*playv1.Participant{{CharacterId: a.capitao.GetId()}, {CharacterId: a.goblin.GetId()}},
		npcRolls: []int{1, 1},
		players:  map[string]int32{"Sálvia": 20, "Pensantus": 12, "Toren": 10},
		reveal:   []string{"Capitão Goblin", "Goblin"},
		at: map[string][2]int32{
			"Sálvia": {6, 5}, "Capitão Goblin": {5, 5}, "Goblin": {6, 6}, "Pensantus": {12, 2}, "Toren": {2, 2},
		},
	})
	e := a.mustAssume(t, a.bia, salvia, wolfKey).GetEncounter()
	w := a.vitals(t, salvia)
	if w.GetWildShape().GetHitPointsCurrent() != 11 || w.GetHitPointsCurrent() != 38 {
		t.Fatalf("fixture: beast %v, own %d; want the wolf at 11 and her own 38", w.GetWildShape(), w.GetHitPointsCurrent())
	}

	a.passTo(t, e, "Pensantus")
	a.h.roller.queue(2, 4, 1, 5, 3) // 5d8 = 15
	res := a.mustCast(t, a.ana, a.get(t, a.ana), "Pensantus", sleepSpell, slotOfLevel(1), a.at(t, "Sálvia"), poolInApp)
	pe := res.GetEncounter()
	eff := effectOf(t, pe, res.GetCast().GetTargets(), "Sálvia")
	if eff.GetOutcome() != playv1.SpellEffectOutcome_SPELL_EFFECT_OUTCOME_AFFECTED {
		t.Errorf("Sleep (pool 15) on the druid in the 11-HP wolf form: outcome = %v, want affected (the creature's hit points are the beast's)", eff.GetOutcome())
	}
	master := logEffect(t, spellEntry(t, a.log(t, a.master, pe)), "Sálvia")
	if master.GetHitPointsBefore() != 11 {
		t.Errorf("hit points the master sees for the druid in the form = %d, want the beast's 11 (not her own 38)", master.GetHitPointsBefore())
	}
}
