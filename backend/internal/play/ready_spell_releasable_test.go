package play

import (
	"testing"

	playv1 "github.com/PuraFome/meuRPG/backend/gen/meurpg/play/v1"
	rulesv1 "github.com/PuraFome/meuRPG/backend/gen/meurpg/rules/v1"
)

// The spells the READIED window cannot release are refused at Ready (#857): the answer carries targets and darts only, so
// a spell that asks for more could only be let pass, and the slot and the action would be lost with it.

const (
	blockedReadyNotReleasable = playv1.EncounterBlockedReason_ENCOUNTER_BLOCKED_REASON_READY_SPELL_NOT_RELEASABLE

	enhanceAbilitySpell  = "spell:enhance-ability"
	spiritGuardiansSpell = "spell:spirit-guardians"
	flameStrikeSpell     = "spell:flame-strike"
)

// newReadierOf is the table of newCasters with Pensantus as a caster of another class, who knows and prepares the spells.
func newReadierOf(t *testing.T, class string, level int32, spells ...string) *armed {
	t.Helper()
	return newArmedWith(t, func(a *armed) {
		a.toren = a.caio.hero(t, a.campaignID, "Toren", "class:fighter", "race:human", 5,
			&rulesv1.AbilityScores{Strength: 16, Dexterity: 13, Constitution: 14, Intelligence: 10, Wisdom: 10, Charisma: 8}, []string{battleaxe}, nil)
		a.pens = a.ana.caster(t, a.campaignID, "Pensantus", class, "race:gnome", level,
			&rulesv1.AbilityScores{Strength: 10, Dexterity: 14, Constitution: 12, Intelligence: 16, Wisdom: 16, Charisma: 16}, nil, nil, spells, spells)
		a.bri = a.bia.caster(t, a.campaignID, "Brisa", "class:cleric", "race:human", 3,
			&rulesv1.AbilityScores{Strength: 10, Dexterity: 16, Constitution: 14, Intelligence: 10, Wisdom: 16, Charisma: 8}, []string{maceKey}, []string{sacredFlame}, nil,
			[]string{cureWounds, healingWord, guidingBolt})
	})
}

// wantNothingReadied checks a refused Ready spent nothing: no event, no slot of the level, no action, no row and no concentration.
func (a *armed) wantNothingReadied(t *testing.T, events int, level, slots int32) {
	t.Helper()
	pens := byLabel(t, a.get(t, a.master), "Pensantus")
	if n := a.events(t); n != events || a.slotsUsed(t, "Pensantus", level) != slots || pens.GetActionUsed() || a.readiedStatus(t, "Pensantus") != "" ||
		pens.GetConcentrationSpell() != "" {
		t.Errorf("after the refusal: events %d (want %d), level-%d slots %d (want %d), action used %v, rows %q, concentration %q; want nothing spent",
			n, events, level, a.slotsUsed(t, "Pensantus", level), slots, pens.GetActionUsed(), a.readiedStatus(t, "Pensantus"), pens.GetConcentrationSpell())
	}
}

// Ready refuses, with READY_SPELL_NOT_RELEASABLE and writing nothing, a spell whose release the answer cannot carry: the
// creatures of a summon, an ability, a damage type or the dead creature of a revival. A spell placed on the map is not
// one: CastSpell releases it with the point or the direction of the map (TestReadyHoldsASpellPlacedOnAMap).
func TestReadyRefusesASpellTheWindowCannotRelease(t *testing.T) {
	t.Parallel()
	cases := []struct {
		name   string
		class  string
		level  int32
		spell  string
		slot   int32
		spells []string
	}{
		{"a summon (Conjure Animals)", "class:druid", 5, conjureAnimals, 3, []string{conjureAnimals}},
		{"an ability (Enhance Ability)", "class:druid", 5, enhanceAbilitySpell, 2, []string{enhanceAbilitySpell}},
		{"a damage type (Spirit Guardians)", "class:cleric", 7, spiritGuardiansSpell, 3, []string{spiritGuardiansSpell}},
		{"a damage type (Flame Strike)", "class:cleric", 11, flameStrikeSpell, 5, []string{flameStrikeSpell}},
		{"a revival (Revivify)", "class:cleric", 5, revivify, 3, []string{revivify}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			a := newReadierOf(t, tc.class, tc.level, tc.spells...)
			e := a.castersFight(t, 1)
			events, slots := a.events(t), a.slotsUsed(t, "Pensantus", tc.slot)
			_, err := a.ready(t, a.ana, a.readySpell(t, e, "Pensantus", tc.spell, tc.slot, ""))
			wantBlockedBy(t, "Ready of "+tc.name, err, blockedReadyNotReleasable)
			a.wantNothingReadied(t, events, tc.slot, slots)
		})
	}
}

// A spell the window releases is still readied: Magic Missile (darts) and Hold Person (a target) with their targets, and an
// area in a combat with no map, which the answer releases with the creatures it picks.
func TestReadyStillHoldsASpellTheWindowReleases(t *testing.T) {
	t.Parallel()
	for _, tc := range []struct {
		name    string
		spell   string
		slot    int32
		theatre bool
	}{
		{"Magic Missile", magicMissileSpell, 1, false},
		{"Hold Person", holdPerson, 2, false},
		{"Sleep with no map", sleepSpell, 1, true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			a := newCasters(t)
			var e *playv1.Encounter
			if tc.theatre {
				e = a.start(t, plan{
					npcs: []*playv1.Participant{{CharacterId: a.capitao.GetId()}}, npcRolls: []int{1}, reveal: []string{"Capitão Goblin"}, theatre: true,
					players: map[string]int32{"Pensantus": 20, "Toren": 15, "Brisa": 10},
				})
			} else {
				e = a.castersFight(t, 1)
			}
			a.mustReady(t, a.ana, a.readySpell(t, e, "Pensantus", tc.spell, tc.slot, ""))
			if a.readiedStatus(t, "Pensantus") != readiedWaiting || a.concentrationAs(t, a.ana, "Pensantus") != tc.spell {
				t.Errorf("rows %q, concentration %q; want the spell held", a.readiedStatus(t, "Pensantus"), a.concentrationAs(t, a.ana, "Pensantus"))
			}
		})
	}
}

// A spell placed on the map is readied: the window's "Soltar no mapa" releases it with CastSpell, which carries the point or
// the direction.
func TestReadyHoldsASpellPlacedOnAMap(t *testing.T) {
	t.Parallel()
	for _, tc := range []struct {
		name  string
		spell string
		slot  int32
	}{
		{"a point on a map (Sleep)", sleepSpell, 1},
		{"a point on a map (Fireball)", fireballSpell, 3},
		{"a direction on a map (Burning Hands)", burningHands, 1},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			a := newReadierOf(t, "class:sorcerer", 5, tc.spell)
			e := a.castersFight(t, 1)
			a.mustReady(t, a.ana, a.readySpell(t, e, "Pensantus", tc.spell, tc.slot, ""))
			if a.readiedStatus(t, "Pensantus") != readiedWaiting || a.concentrationAs(t, a.ana, "Pensantus") != tc.spell {
				t.Errorf("rows %q, concentration %q; want the spell held", a.readiedStatus(t, "Pensantus"), a.concentrationAs(t, a.ana, "Pensantus"))
			}
		})
	}
}
