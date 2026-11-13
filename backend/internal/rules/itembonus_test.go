package rules

import "testing"

func derivedWithBuild(t *testing.T, mut func(*Build)) Derived {
	t.Helper()
	c := loadForTest(t)
	b := standard("class:fighter", 1)
	mut(&b)
	return Derive(b, c)
}

func weaponLine(t *testing.T, d Derived, key string) Attack {
	t.Helper()
	for _, a := range d.Attacks {
		if a.Key == key {
			return a
		}
	}
	t.Fatalf("no attack %s", key)
	return Attack{}
}

const sword = "equipment:longsword"

func TestWeaponBonusAddsToAttackDamageAndMakesItMagical(t *testing.T) {
	t.Parallel()
	plain := derivedWithBuild(t, func(b *Build) { b.Weapons = []string{sword} })
	plus := derivedWithBuild(t, func(b *Build) {
		b.Weapons = []string{sword}
		b.ItemBonuses = []ItemBonus{{Key: sword, Bonus: 1}}
	})
	p, q := weaponLine(t, plain, sword), weaponLine(t, plus, sword)
	if q.AttackBonus != p.AttackBonus+1 {
		t.Errorf("attack %d, want %d", q.AttackBonus, p.AttackBonus+1)
	}
	if q.Damage != "1d8+4" || p.Damage != "1d8+3" {
		t.Errorf("damage %q (plain %q), want 1d8+4 (1d8+3)", q.Damage, p.Damage)
	}
	if !q.Magical || p.Magical {
		t.Errorf("magical %v (plain %v), want true (false)", q.Magical, p.Magical)
	}
	if len(plus.ItemModifiers) != 2 || !plus.ItemModifiers[0].Counts || plus.ItemModifiers[0].Target != "attack:"+sword {
		t.Errorf("modifiers %+v", plus.ItemModifiers)
	}
}

func TestArmorAndShieldBonusAddToArmorClass(t *testing.T) {
	t.Parallel()
	base := derivedWithBuild(t, func(b *Build) { b.Armor, b.Shield = "equipment:chain-mail", true }).ArmorClass
	d := derivedWithBuild(t, func(b *Build) {
		b.Armor, b.Shield = "equipment:chain-mail", true
		b.ItemBonuses = []ItemBonus{{Key: "equipment:chain-mail", Bonus: 2}, {Key: ShieldKey, Bonus: 1}}
	})
	if d.ArmorClass != base+3 {
		t.Errorf("AC %d, want %d", d.ArmorClass, base+3)
	}
	if len(d.ItemModifiers) != 2 || d.ItemModifiers[0].Target != "ac" || !d.ItemModifiers[1].Counts {
		t.Errorf("modifiers %+v", d.ItemModifiers)
	}
}

func TestACofACarriedPieceAddsNothingAndSaysWhy(t *testing.T) {
	t.Parallel()
	base := derivedWithBuild(t, func(b *Build) { b.Armor, b.Carried = "equipment:chain-mail", []string{"equipment:chain-mail"} }).ArmorClass
	d := derivedWithBuild(t, func(b *Build) {
		b.Armor, b.Carried = "equipment:chain-mail", []string{"equipment:chain-mail", sword}
		b.Weapons = []string{sword}
		b.ItemBonuses = []ItemBonus{{Key: "equipment:chain-mail", Bonus: 1}, {Key: sword, Bonus: 1}}
	})
	if d.ArmorClass != base {
		t.Errorf("AC %d, want %d", d.ArmorClass, base)
	}
	for _, m := range d.ItemModifiers {
		if m.Counts || m.ReasonPT == "" {
			t.Errorf("a carried piece counts or has no reason: %+v", m)
		}
	}
	if len(d.Attacks) != 1 { // only the unarmed strike
		t.Errorf("attacks %d", len(d.Attacks))
	}
}

func TestShieldBehindATwoHandedWeaponAddsNothing(t *testing.T) {
	t.Parallel()
	d := derivedWithBuild(t, func(b *Build) {
		b.Shield, b.Weapons = true, []string{"equipment:greatsword"}
		b.ItemBonuses = []ItemBonus{{Key: ShieldKey, Bonus: 2}}
	})
	base := derivedWithBuild(t, func(b *Build) { b.Shield, b.Weapons = true, []string{"equipment:greatsword"} })
	if d.ArmorClass != base.ArmorClass || d.ItemModifiers[0].Counts {
		t.Errorf("AC %d (base %d), modifier %+v", d.ArmorClass, base.ArmorClass, d.ItemModifiers[0])
	}
}

// Only an equipped two-handed weapon idles the shield: one in the backpack leaves the shield's +N counting.
func TestACarriedTwoHandedWeaponLeavesTheShieldBonusCounting(t *testing.T) {
	t.Parallel()
	d := derivedWithBuild(t, func(b *Build) {
		b.Shield, b.Weapons, b.Carried = true, []string{"equipment:greatsword"}, []string{"equipment:greatsword"}
		b.ItemBonuses = []ItemBonus{{Key: ShieldKey, Bonus: 2}}
	})
	base := derivedWithBuild(t, func(b *Build) {
		b.Shield, b.Weapons, b.Carried = true, []string{"equipment:greatsword"}, []string{"equipment:greatsword"}
	})
	if d.ArmorClass != base.ArmorClass+2 || d.ShieldIdle || len(d.ItemModifiers) != 1 || !d.ItemModifiers[0].Counts {
		t.Errorf("AC %d (base %d), idle %v, modifiers %+v, want +2 counting", d.ArmorClass, base.ArmorClass, d.ShieldIdle, d.ItemModifiers)
	}
}

func TestAttunementGatesTheBonus(t *testing.T) {
	t.Parallel()
	for _, attuned := range []bool{false, true} {
		d := derivedWithBuild(t, func(b *Build) {
			b.Weapons = []string{sword}
			b.ItemBonuses = []ItemBonus{{Key: sword, Bonus: 3, RequiresAttunement: true, Attuned: attuned}}
		})
		a := weaponLine(t, d, sword)
		plain := weaponLine(t, derivedWithBuild(t, func(b *Build) { b.Weapons = []string{sword} }), sword)
		if got := a.AttackBonus - plain.AttackBonus; (got == 3) != attuned || (got != 0 && got != 3) {
			t.Errorf("attuned %v: attack differs by %d", attuned, got)
		}
		if a.Magical != attuned || d.ItemModifiers[0].Counts != attuned {
			t.Errorf("attuned %v: magical %v, counts %v", attuned, a.Magical, d.ItemModifiers[0].Counts)
		}
	}
}

func TestABonusOnAWeaponTheContentDoesNotKnowListsNothing(t *testing.T) {
	t.Parallel()
	d := derivedWithBuild(t, func(b *Build) {
		b.Weapons = []string{"equipment:no-such-blade"}
		b.ItemBonuses = []ItemBonus{{Key: "equipment:no-such-blade", Bonus: 1}}
	})
	if len(d.ItemModifiers) != 0 {
		t.Errorf("modifiers %+v, want none: the weapon has no attack line", d.ItemModifiers)
	}
}

func TestItemBonusesAreValidated(t *testing.T) {
	t.Parallel()
	c := loadForTest(t)
	for name, ibs := range map[string][]ItemBonus{
		"off the sheet": {{Key: "equipment:dagger", Bonus: 1}},
		"zero":          {{Key: sword, Bonus: 0}},
		"four":          {{Key: sword, Bonus: 4}},
		"twice":         {{Key: sword, Bonus: 1}, {Key: sword, Bonus: 2}},
	} {
		b := standard("class:fighter", 1)
		b.Weapons, b.ItemBonuses = []string{sword}, ibs
		if err := Validate(b, c); err == nil {
			t.Errorf("%s: accepted", name)
		}
	}
}

// TestItemBonusGolden pins the numbers a magic piece changes in each situation, read line by line:
// equipped, carried, behind a two-handed weapon and awaiting attunement.
func TestItemBonusGolden(t *testing.T) {
	t.Parallel()
	type line struct {
		Key    string
		Attack int
		Damage string
		// Versatile is the two-handed damage line.
		Versatile string
		Magical   bool
	}
	type view struct {
		ArmorClass    int
		Description   string
		Attacks       []line
		ItemModifiers []ItemModifier
	}
	mail, rapier := "equipment:chain-mail", "equipment:rapier"
	plus := func(key string, n int) ItemBonus { return ItemBonus{Key: key, Bonus: n} }
	cases := map[string]func(*Build){
		"plain": func(b *Build) { b.Armor, b.Shield, b.Weapons = mail, true, []string{rapier} },
		"all equipped": func(b *Build) {
			b.Armor, b.Shield, b.Weapons = mail, true, []string{rapier}
			b.ItemBonuses = []ItemBonus{plus(mail, 1), plus(ShieldKey, 2), plus(rapier, 3)}
		},
		"all carried": func(b *Build) {
			b.Armor, b.Shield, b.Weapons = mail, true, []string{rapier}
			b.Carried = []string{mail, ShieldKey, rapier}
			b.ItemBonuses = []ItemBonus{plus(mail, 1), plus(ShieldKey, 2), plus(rapier, 3)}
		},
		"shield behind a greatsword": func(b *Build) {
			b.Shield, b.Weapons = true, []string{"equipment:greatsword"}
			b.ItemBonuses = []ItemBonus{plus(ShieldKey, 2), plus("equipment:greatsword", 1)}
		},
		"versatile longsword": func(b *Build) {
			b.Weapons = []string{"equipment:longsword"}
			b.ItemBonuses = []ItemBonus{plus("equipment:longsword", 2)}
		},
		"awaiting attunement": func(b *Build) {
			b.Armor, b.Weapons = mail, []string{rapier}
			b.ItemBonuses = []ItemBonus{{Key: mail, Bonus: 2, RequiresAttunement: true}, {Key: rapier, Bonus: 1, RequiresAttunement: true, Attuned: true}}
		},
	}
	got := map[string]view{}
	for name, mut := range cases {
		d := derivedWithBuild(t, mut)
		v := view{ArmorClass: d.ArmorClass, Description: d.ArmorClassDescription, ItemModifiers: d.ItemModifiers}
		for _, a := range d.Attacks {
			v.Attacks = append(v.Attacks, line{a.Key, a.AttackBonus, a.Damage, a.VersatileDamage, a.Magical})
		}
		got[name] = v
	}
	levelUpGoldenJSON(t, "item-bonuses", got)
}
