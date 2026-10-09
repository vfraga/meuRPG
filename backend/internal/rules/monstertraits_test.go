package rules

import (
	"testing"
	"testing/fstest"

	"github.com/PuraFome/meuRPG/backend/internal/rules/srd51"
)

// The table of the traits the engine applies is closed: a trait no creature has, one listed twice and
// one without a source are refused (CONTRIBUTING.md, the revision rule).
func TestMonsterTraitsTableIsClosed(t *testing.T) {
	t.Parallel()
	newContent := func() *content {
		return &content{monsters: map[string]*srd51.Monster{
			"monster:wolf": {SpecialAbilities: []srd51.MonsterAbility{{Name: "Pack Tactics"}}},
		}}
	}
	file := func(body string) fstest.MapFS {
		return fstest.MapFS{"effects/monster_traits.json": {Data: []byte(`{"engine_reads":[` + body + `]}`)}}
	}
	for name, body := range map[string]string{
		"a trait no creature has": `{"trait":"Fire Aura","reads":"x","source":"SRD"}`,
		"a trait without source":  `{"trait":"Pack Tactics","reads":"x"}`,
		"a trait reading nothing": `{"trait":"Pack Tactics","source":"SRD"}`,
		"a trait listed twice":    `{"trait":"Pack Tactics","reads":"x","source":"SRD"},{"trait":"Pack Tactics","reads":"x","source":"SRD"}`,
	} {
		if err := newContent().loadMonsterTraits(file(body)); err == nil {
			t.Errorf("%s: the loader accepted it", name)
		}
	}
	c := newContent()
	if err := c.loadMonsterTraits(file(`{"trait":"Pack Tactics","reads":"advantage on the attack roll","source":"SRD 5.1, Wolf"}`)); err != nil {
		t.Fatalf("a good table: %v", err)
	}
	if !c.engineTraits["Pack Tactics"] {
		t.Errorf("engineTraits = %v, want Pack Tactics", c.engineTraits)
	}
}

// A creature's spellcasting gives the turn options the shapes a character's does: the mage's slots, its
// spells prepared, and its damaging cantrips as spell attacks with its attack bonus (SRD 5.1, Mage).
func TestMonsterCastingGivesTheTurnOptionsTheirShapes(t *testing.T) {
	t.Parallel()
	c := loadForTest(t)
	mc, ok := c.MonsterCasting("monster:mage")
	if !ok {
		t.Fatal("the mage has no spellcasting")
	}
	if got := mc.Slots[:5]; got[0] != 4 || got[1] != 3 || got[2] != 3 || got[3] != 3 || got[4] != 1 {
		t.Errorf("slots = %v, want 4, 3, 3, 3, 1", mc.Slots)
	}
	if mc.Level != 9 || len(mc.Spellcasting) != 1 || mc.Spellcasting[0].AttackBonus != 6 || mc.Spellcasting[0].SaveDC != 14 {
		t.Errorf("casting = %+v, want a 9th-level caster with +6 and DC 14", mc)
	}
	var fireBolt *Attack
	for i := range mc.Cantrips {
		if mc.Cantrips[i].Key == "spell:fire-bolt" {
			fireBolt = &mc.Cantrips[i]
		}
	}
	if fireBolt == nil || fireBolt.AttackBonus != 6 || fireBolt.Damage != "2d10" {
		t.Errorf("fire bolt = %+v, want +6 and 2d10 at level 9 (two dice at the 5th level or more)", fireBolt)
	}
	if _, ok := c.MonsterCasting("monster:goblin"); ok {
		t.Error("the goblin casts")
	}
}
