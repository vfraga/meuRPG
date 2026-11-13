package play

import (
	"slices"
	"testing"

	playv1 "github.com/PuraFome/meuRPG/backend/gen/meurpg/play/v1"
	rulesv1 "github.com/PuraFome/meuRPG/backend/gen/meurpg/rules/v1"
)

// A table feature's weapon rules and pseudo-weapon in a combat: the attack roll and the damage roll
// agree with the sheet, because both read the one derived attack line. Every name is invented.

const rayName = "Raio de Teste"

// weaponEffects: a built-in ranged attack that uses Charisma (1d6 force, 60 ft), +1 to hit and +2
// damage for the weapon with that name, and Charisma for the attack and the damage of melee weapons.
func weaponEffects() []*rulesv1.TableEffect {
	return []*rulesv1.TableEffect{
		{Type: "pseudo_weapon", AttackName: rayName, Weapon: "ranged", DamageDice: "1d6", DamageType: "damage-type:force", Ability: "cha", RangeFt: 60},
		{Type: "weapon_rule", Weapon: "named", WeaponName: rayName, AttackBonus: 1, DamageBonus: 2},
		{Type: "weapon_rule", Weapon: "melee", AttackAbility: "cha", DamageAbility: "cha"},
	}
}

// weaponRig is a fight with a Toren whose Strength modifier is 0 and whose Charisma modifier is +3 (the
// proficiency bonus of level 5 is +3).
func weaponRig(t *testing.T) *stanceRig {
	t.Helper()
	return newStanceRigOf(t, weaponEffects(), abilities(10, 13, 14, 10, 16))
}

// attackLine is the line of the attack list that has the name.
func attackLine(t *testing.T, d *rulesv1.DerivedSheet, name string) *rulesv1.Attack {
	t.Helper()
	i := slices.IndexFunc(d.GetAttacks(), func(a *rulesv1.Attack) bool { return a.GetNamePt() == name })
	if i < 0 {
		t.Fatalf("no attack named %q among %v", name, d.GetAttacks())
	}
	return d.GetAttacks()[i]
}

func TestAPseudoWeaponIsRolledAsTheSheetShowsIt(t *testing.T) {
	t.Parallel()
	r := weaponRig(t)
	line := attackLine(t, r.derived(t), rayName)
	if line.GetAttackBonus() != 7 || line.GetDamage() != "1d6+5" || line.GetDamageTypeKey() != "damage-type:force" || line.GetRangeFt() != 60 || line.GetMelee() {
		t.Fatalf("sheet line = %v, want +7 (3 + 3 + 1), 1d6+5 force, 60 ft, ranged", line)
	}

	e := r.a.get(t, r.a.master)
	res := r.a.mustAttack(t, r.a.caio, e, "Toren", line.GetKey(), "Goblin", disadvantage(5)) // a ranged attack next to a hostile creature rolls two d20
	if roll := res.GetRoll(); roll.GetD20().GetModifier() != line.GetAttackBonus() || roll.GetD20().GetTotal() != 5+line.GetAttackBonus() || roll.GetOutcome() != playv1.AttackOutcome_ATTACK_OUTCOME_HIT {
		t.Fatalf("the attack roll = %v, want a hit with the sheet's +%d", roll, line.GetAttackBonus())
	}
	p := res.GetPendingDamage()
	if p.GetDiceCount() != 1 || p.GetDiceSides() != 6 || p.GetBonus() != line.GetDamageDice().GetBonus() || p.GetDamageTypeKey() != "damage-type:force" {
		t.Fatalf("the damage to roll = %v, want 1d6 + %d force as the sheet says", p, line.GetDamageDice().GetBonus())
	}
	r.a.h.roller.queue(4)
	if d := r.a.mustDamage(t, r.a.caio, e, p.GetId(), inAppDamage).GetPendingDamage(); d.GetAmount() != 9 {
		t.Errorf("the damage = %v, want 4 + 5 = 9", d)
	}
}

func TestAWeaponRuleChangesACarriedWeaponInTheRollsToo(t *testing.T) {
	t.Parallel()
	r := weaponRig(t)
	line := attackLine(t, r.derived(t), "Espada longa")
	// Strength is +0 and Charisma +3: the rule gives the sword the better one, and the proficiency bonus is +3.
	if line.GetAttackBonus() != 6 || line.GetDamage() != "1d8+3" {
		t.Fatalf("sheet line = %v, want +6 and 1d8+3", line)
	}
	e := r.a.get(t, r.a.master)
	res := r.a.mustAttack(t, r.a.caio, e, "Toren", line.GetKey(), "Goblin", d20(9))
	if roll := res.GetRoll(); roll.GetD20().GetModifier() != 6 || roll.GetOutcome() != playv1.AttackOutcome_ATTACK_OUTCOME_HIT {
		t.Errorf("the attack roll = %v, want a hit with +6", roll)
	}
	if p := res.GetPendingDamage(); p.GetBonus() != 3 || p.GetDiceSides() != 8 {
		t.Errorf("the damage to roll = %v, want 1d8 + 3", p)
	}
}

func TestAPseudoWeaponIsNeverInTheInventory(t *testing.T) {
	t.Parallel()
	r := weaponRig(t)
	if got := r.a.toren.GetSheet().GetFull().GetWeaponKeys(); !slices.Equal(got, []string{longsword}) {
		t.Errorf("weapon keys = %v, want only the sword the player carries", got)
	}
	if d := r.derived(t); len(d.GetAttacks()) < 3 {
		t.Errorf("attacks = %v, want the sword, the ray and the unarmed strike", d.GetAttacks())
	}
}

const biteName = "Mordida de Teste"

// A natural attack: the better of Strength (+0) and Dexterity (+2) for the attack and the damage, the
// proficiency bonus of level 5 is +3, and a melee rule adds +1 to hit and +2 damage to it like to any melee line.
func TestANaturalAttackIsRolledAsTheSheetShowsIt(t *testing.T) {
	t.Parallel()
	r := newStanceRigOf(t, []*rulesv1.TableEffect{
		{Type: "natural_attack", AttackName: biteName, DamageDice: "1d6", DamageType: "damage-type:piercing", Ability: "best"},
		{Type: "weapon_rule", Weapon: "melee", AttackBonus: 1, DamageBonus: 2},
	}, abilities(10, 13, 14, 10, 16))
	line := attackLine(t, r.derived(t), biteName)
	if line.GetAttackBonus() != 6 || line.GetDamage() != "1d6+4" || line.GetDamageTypeKey() != "damage-type:piercing" || !line.GetMelee() || line.GetRangeFt() != 5 {
		t.Fatalf("sheet line = %v, want +6 (2 + 3 + 1), 1d6+4 piercing, melee, 5 ft", line)
	}

	e := r.a.get(t, r.a.master)
	res := r.a.mustAttack(t, r.a.caio, e, "Toren", line.GetKey(), "Goblin", d20(9))
	if roll := res.GetRoll(); roll.GetD20().GetModifier() != 6 || roll.GetD20().GetTotal() != 15 || roll.GetOutcome() != playv1.AttackOutcome_ATTACK_OUTCOME_HIT {
		t.Fatalf("the attack roll = %v, want a hit with the sheet's +6", roll)
	}
	p := res.GetPendingDamage()
	if p.GetDiceCount() != 1 || p.GetDiceSides() != 6 || p.GetBonus() != 4 || p.GetDamageTypeKey() != "damage-type:piercing" {
		t.Fatalf("the damage to roll = %v, want 1d6 + 4 piercing as the sheet says", p)
	}
	r.a.h.roller.queue(4)
	if d := r.a.mustDamage(t, r.a.caio, e, p.GetId(), inAppDamage).GetPendingDamage(); d.GetAmount() != 8 {
		t.Errorf("the damage = %v, want 4 + 4 = 8", d)
	}
}
