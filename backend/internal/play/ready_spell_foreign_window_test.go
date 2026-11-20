package play

import (
	"slices"
	"testing"

	playv1 "github.com/PuraFome/meuRPG/backend/gen/meurpg/play/v1"
	rulesv1 "github.com/PuraFome/meuRPG/backend/gen/meurpg/rules/v1"
)

// A cast is a release only for the spell its own caster's open READIED window holds (findReadied): another combatant's window
// of the same spell, and an open window of another kind, make it an ordinary cast, refused off the caster's turn.

// newLightCasters is newCasters with Light, a cantrip of the wizard's and the cleric's, known to both.
func newLightCasters(t *testing.T) *armed {
	t.Helper()
	return newArmedWith(t, func(a *armed) {
		a.toren = a.caio.hero(t, a.campaignID, "Toren", "class:fighter", "race:human", 5,
			&rulesv1.AbilityScores{Strength: 16, Dexterity: 13, Constitution: 14, Intelligence: 10, Wisdom: 10, Charisma: 8}, []string{battleaxe}, nil)
		a.pens = a.ana.caster(t, a.campaignID, "Pensantus", "class:wizard", "race:gnome", 3,
			&rulesv1.AbilityScores{Strength: 10, Dexterity: 14, Constitution: 12, Intelligence: 16, Wisdom: 10, Charisma: 8}, nil, []string{fireBolt, lightSpell},
			[]string{magicMissileSpell}, []string{magicMissileSpell})
		a.bri = a.bia.caster(t, a.campaignID, "Brisa", "class:cleric", "race:human", 3,
			&rulesv1.AbilityScores{Strength: 10, Dexterity: 16, Constitution: 14, Intelligence: 10, Wisdom: 16, Charisma: 8}, []string{maceKey}, []string{sacredFlame, lightSpell}, nil,
			[]string{cureWounds, healingWord, guidingBolt})
	})
}

// Brisa holds Light and her window is open: Pensantus, who knows Light too and is off his turn, is not released by it. His cast is
// refused, and her window, her row and his reaction are as they were.
func TestTheSameSpellHeldByAnotherCombatantIsNotTheCastersRelease(t *testing.T) {
	t.Parallel()
	a := newLightCasters(t)
	e := a.castersFight(t, 1)
	e = a.passTo(t, e, "Brisa")
	a.mustReady(t, a.bia, a.readySpell(t, e, "Brisa", lightSpell, 0, ""))
	e = a.mustEndTurn(t, a.master, e)
	a.mustFire(t, e, "Brisa")
	before := a.events(t)
	_, err := a.cast(t, a.ana, e, "Pensantus", lightSpell, nil, nil, noCastRoll)
	wantBlockedBy(t, "Pensantus's cast of the spell Brisa holds", err, blockedNotYourTurn)
	if n := a.events(t); n != before || a.readiedStatus(t, "Brisa") != readiedWaiting || !a.readiedOpen(t, "Brisa") || byLabel(t, a.get(t, a.master), "Pensantus").GetReactionUsed() {
		t.Errorf("after the refusal: events %d (want %d), Brisa's row %q, her window open %v", n, before, a.readiedStatus(t, "Brisa"), a.readiedOpen(t, "Brisa"))
	}
}

// A window of another kind open for the caster (the concentration save of its held spell) is not its READIED window: its cast
// of the held spell off turn is no release. It is refused, the row waits and the save is still asked.
func TestAnOpenWindowOfAnotherKindIsNotAReleaseOfTheHeldSpell(t *testing.T) {
	t.Parallel()
	a := newCasters(t)
	_, w := a.heldSave(t) // Hold Person is held, and Pensantus owes the save: no READIED window is open
	e := a.get(t, a.master)
	levels := []int32{1, 2, 3, 4, 5}
	slots, before := a.slotsOf(t, "Pensantus", levels...), a.events(t)
	_, err := a.cast(t, a.ana, e, "Pensantus", holdPerson, slotOfLevel(2), a.at(t, "Capitão Goblin"), noCastRoll)
	wantBlockedBy(t, "the cast of the held spell with only a save open", err, blockedNotYourTurn)
	pens := byLabel(t, a.get(t, a.master), "Pensantus")
	if n := a.events(t); n != before || a.readiedStatus(t, "Pensantus") != readiedWaiting || a.readiedOpen(t, "Pensantus") || pens.GetReactionUsed() {
		t.Errorf("after the refusal: events %d (want %d), row %q, READIED window open %v, reaction used %v; want nothing written, the row waiting and no window", n, before, a.readiedStatus(t, "Pensantus"), a.readiedOpen(t, "Pensantus"), pens.GetReactionUsed())
	}
	if got := a.slotsOf(t, "Pensantus", levels...); !slices.Equal(got, slots) {
		t.Errorf("slots after the refusal = %v, want %v: the refused cast spent one", got, slots)
	}
	if got := a.storedConcentration(t, "Pensantus"); got != concentrationReadied {
		t.Errorf("the stored concentration after the refusal = %q, want the sentinel %q", got, concentrationReadied)
	}
	if got := a.windowOf(t, a.ana, playv1.ReactionKind_REACTION_KIND_CONCENTRATION_SAVE); got.GetId() != w.GetId() {
		t.Errorf("the save window after the refusal = %v, want %s still open", got, w.GetId())
	}
}
