package play

import (
	"testing"

	playv1 "github.com/PuraFome/meuRPG/backend/gen/meurpg/play/v1"
	rulesv1 "github.com/PuraFome/meuRPG/backend/gen/meurpg/rules/v1"
)

// A spell that keeps a zone is readied on a map (#857) and released with CastSpell, which carries the point. The answer of
// its window carries none and refuses it. A combat with no map lays no zones, so the answer releases it there.

const (
	tableCasterBall = "spell:esfera-de-teste@mesa"
	tableSphereZone = "spell:zona-de-teste@mesa"
	readyZoneCaster = "Pensantus"
)

// tableCasterSphere is a table spell with a sphere of 10 feet centered on its caster (range Self), a saving throw and damage, and no zone.
func tableCasterSphere() *rulesv1.TableSpell {
	s := tableSpell("Esfera de Teste", 1, "class:wizard", rulesv1.SpellRangeKind_SPELL_RANGE_KIND_SELF,
		&rulesv1.TableSpellTarget{Kind: rulesv1.TableSpellTargetKind_TABLE_SPELL_TARGET_KIND_AREA, Shape: rulesv1.TableAreaShape_TABLE_AREA_SHAPE_SPHERE, SizeFt: 10})
	s.Save = &rulesv1.SpellSave{Ability: rulesv1.Ability_ABILITY_DEXTERITY, OnSuccess: rulesv1.SpellSaveSuccess_SPELL_SAVE_SUCCESS_HALF}
	s.Damage = []*rulesv1.TableSpellDamage{{DamageTypeKey: "damage-type:force", Dice: "2d6", PerSlotLevel: "1d6"}}
	return s
}

// newTableReadier is Pensantus, a wizard 5, who knows Web, Grease and the table spells below.
func newTableReadier(t *testing.T) *armed {
	t.Helper()
	return newArmedWith(t, func(a *armed) {
		for _, s := range []*rulesv1.TableSpell{tableCasterSphere(), tableZoneSpell("Zona de Teste", true, rulesv1.TableZoneTrigger_TABLE_ZONE_TRIGGER_FIRST_ENTRY, false)} {
			if err := addTableSpell(t, a, s); err != nil {
				t.Fatalf("CreateTableEntry(%s) error = %v", s.GetNamePt(), err)
			}
		}
		book := []string{webSpell, greaseKey, tableCasterBall, tableSphereZone}
		a.toren = a.caio.hero(t, a.campaignID, "Toren", "class:fighter", "race:human", 5,
			&rulesv1.AbilityScores{Strength: 16, Dexterity: 13, Constitution: 14, Intelligence: 10, Wisdom: 10, Charisma: 8}, []string{battleaxe}, nil)
		a.pens = a.ana.caster(t, a.campaignID, readyZoneCaster, "class:wizard", "race:gnome", 5,
			&rulesv1.AbilityScores{Strength: 10, Dexterity: 14, Constitution: 12, Intelligence: 16, Wisdom: 10, Charisma: 8}, nil, nil, book, book)
		a.bri = a.bia.caster(t, a.campaignID, "Brisa", "class:cleric", "race:human", 3,
			&rulesv1.AbilityScores{Strength: 10, Dexterity: 16, Constitution: 14, Intelligence: 10, Wisdom: 16, Charisma: 8}, []string{maceKey}, []string{sacredFlame}, nil,
			[]string{cureWounds, healingWord, guidingBolt})
	})
}

// theatreFight is the fight of castersFight with no map.
func (a *armed) theatreFight(t *testing.T) *playv1.Encounter {
	t.Helper()
	return a.start(t, plan{
		npcs: []*playv1.Participant{{CharacterId: a.capitao.GetId()}}, npcRolls: []int{1}, reveal: []string{"Capitão Goblin"}, theatre: true,
		players: map[string]int32{readyZoneCaster: 20, "Toren": 15, "Brisa": 10},
	})
}

// Ready holds, on a map, a spell that keeps a zone, whether the SRD's or a table's: CastSpell releases it with the point.
func TestReadyHoldsASpellThatKeepsAZoneOnAMap(t *testing.T) {
	t.Parallel()
	for _, tc := range []struct {
		name  string
		spell string
		slot  int32
	}{
		{"Web", webSpell, 2},
		{"Grease", greaseKey, 1},
		{"a table spell with a zone", tableSphereZone, 2},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			a := newTableReadier(t)
			e := a.castersFight(t, 1)
			a.mustReady(t, a.ana, a.readySpell(t, e, readyZoneCaster, tc.spell, tc.slot, ""))
			if a.readiedStatus(t, readyZoneCaster) != readiedWaiting || a.concentrationAs(t, a.ana, readyZoneCaster) != tc.spell {
				t.Errorf("rows %q, concentration %q; want the spell held", a.readiedStatus(t, readyZoneCaster), a.concentrationAs(t, a.ana, readyZoneCaster))
			}
		})
	}
}

// A combat with no map lays no zone, so the same spells are readied there.
func TestReadyStillHoldsASpellThatKeepsAZoneWithNoMap(t *testing.T) {
	t.Parallel()
	for _, tc := range []struct {
		name  string
		spell string
		slot  int32
	}{
		{"Web", webSpell, 2},
		{"Grease", greaseKey, 1},
		{"a table spell with a zone", tableSphereZone, 2},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			a := newTableReadier(t)
			e := a.theatreFight(t)
			a.mustReady(t, a.ana, a.readySpell(t, e, readyZoneCaster, tc.spell, tc.slot, ""))
			if a.readiedStatus(t, readyZoneCaster) != readiedWaiting || a.concentrationAs(t, a.ana, readyZoneCaster) != tc.spell {
				t.Errorf("rows %q, concentration %q; want the spell held", a.readiedStatus(t, readyZoneCaster), a.concentrationAs(t, a.ana, readyZoneCaster))
			}
		})
	}
}

// A sphere centered on its caster (placement CASTER) needs no point from the answer: it is readied on a map. No SRD spell
// has that shape in the data (the SRD's self spheres, such as Antimagic Field, carry no area), so a table spell is the fixture.
func TestReadyHoldsASphereCenteredOnItsCasterOnAMap(t *testing.T) {
	t.Parallel()
	a := newTableReadier(t)
	e := a.castersFight(t, 1)
	a.mustReady(t, a.ana, a.readySpell(t, e, readyZoneCaster, tableCasterBall, 1, ""))
	if a.readiedStatus(t, readyZoneCaster) != readiedWaiting || a.concentrationAs(t, a.ana, readyZoneCaster) != tableCasterBall {
		t.Errorf("rows %q, concentration %q; want the spell held", a.readiedStatus(t, readyZoneCaster), a.concentrationAs(t, a.ana, readyZoneCaster))
	}
}
