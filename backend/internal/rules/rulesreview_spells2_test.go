package rules

import (
	"slices"
	"testing"
)

// SRD 5.1, Warlock, The Fiend, "Expanded Spell List": the patron lets you
// CHOOSE from an expanded list when you learn a warlock spell; the spells "are
// added to the warlock spell list for you". They are not free and not always
// prepared (that wording belongs to the Cleric, Druid and Paladin subclasses).
func TestRulesReviewSpells_FiendSpellsAreNotFree(t *testing.T) {
	c := loadForTest(t)
	b := standard("class:warlock", 5)
	b.Classes[0].Subclass = "subclass:fiend"
	d := Derive(b, c)
	free := []string{"spell:command", "spell:burning-hands", "spell:blindness-deafness", "spell:scorching-ray", "spell:fireball", "spell:stinking-cloud"}
	var got []string
	for _, s := range d.Spells {
		if slices.Contains(free, s.Spell.Key) {
			got = append(got, s.Spell.Key)
		}
	}
	if len(got) > 0 {
		t.Errorf("warlock 5 (Fiend), no spell chosen: SRD says the Fiend spells only join the warlock list (chosen when learning, none free), 0 expected in Derived.Spells; the app grants %d for free as always-prepared: %v (warlock 5 knows 6)",
			len(got), got)
	}
}

// SRD 5.1, Scorching Ray: "Make a ranged spell attack for each ray" (2d6 fire
// each). The data has no attack_type, so nothing rolls an attack.
func TestRulesReviewSpells_ScorchingRayIsASpellAttack(t *testing.T) {
	c := loadForTest(t)
	d, ok := c.SpellDetails("spell:scorching-ray")
	if !ok {
		t.Fatal("no scorching ray")
	}
	if d.AttackType != "ranged" {
		t.Errorf("Scorching Ray AttackType: SRD says a ranged spell attack for each ray (\"ranged\"), the app has %q", d.AttackType)
	}
	rolls := d.DamageAt(2, 5)
	if len(rolls) != 1 || rolls[0].Raw != "2d6" {
		t.Errorf("Scorching Ray DamageAt(2,5): SRD says 2d6 fire per ray, the app has %+v", rolls)
	}
}

// SRD 5.1, Flame Blade: "make a melee spell attack with the fiery blade" (3d6 fire).
func TestRulesReviewSpells_FlameBladeIsASpellAttack(t *testing.T) {
	c := loadForTest(t)
	d, ok := c.SpellDetails("spell:flame-blade")
	if !ok {
		t.Fatal("no flame blade")
	}
	if d.AttackType != "melee" {
		t.Errorf("Flame Blade AttackType: SRD says a melee spell attack (\"melee\"), the app has %q", d.AttackType)
	}
}

// SRD 5.1, Call Lightning: when cast, a bolt strikes and "each creature within 5
// feet of that point must make a dexterity saving throw": 3d10 lightning on a
// failed save, half as much on a success.
func TestRulesReviewSpells_CallLightningSave(t *testing.T) {
	c := loadForTest(t)
	d, ok := c.SpellDetails("spell:call-lightning")
	if !ok {
		t.Fatal("no call lightning")
	}
	if d.Save == nil {
		t.Fatalf("Call Lightning Save: SRD says a Dexterity save, half damage on a success; the app has nil (Damage=%+v), so a cast opens no save and no damage", d.Damage)
	}
	if d.Save.Ability != DEX || d.Save.OnSuccess != "half" {
		t.Errorf("Call Lightning Save: SRD says dex/half, the app has %+v", *d.Save)
	}
}
