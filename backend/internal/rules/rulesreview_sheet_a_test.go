package rules

import (
	"strings"
	"testing"
)

func monkBuild(level int, str, dex int) Build {
	b := standard("class:monk", level)
	b.BaseScores = map[Ability]int{STR: str, DEX: dex, CON: 13, INT: 12, WIS: 10, CHA: 8}
	b.Weapons = []string{"equipment:dagger", "equipment:quarterstaff"}
	return b
}

// SRD 5.1 Monk, Martial Arts: the benefits apply only while unarmed or with
// monk weapons "and you aren't wearing armor or wielding a shield".
func TestRulesReviewSheet_MartialArtsNeedsNoArmor(t *testing.T) {
	t.Parallel()
	c := loadForTest(t)
	for _, tc := range []struct {
		name   string
		armor  string
		shield bool
	}{{"leather armor", "equipment:leather-armor", false}, {"shield", "", true}} {
		b := monkBuild(5, 9, 18)
		b.Armor, b.Shield = tc.armor, tc.shield
		d := Derive(b, c)
		dagger, _ := attackOf(d, "equipment:dagger")
		if dagger.Damage != "1d4+4" {
			t.Errorf("%s: dagger damage = %q, want 1d4+4 (finesse DEX, normal die ; SRD Monk, Martial Arts: no armor or shield)", tc.name, dagger.Damage)
		}
		if dagger.Damage == "1d6+4" {
			t.Errorf("%s: dagger damage = %q, want a plain 1d4 + STR mod (martial arts die d6 must not apply)", tc.name, dagger.Damage)
		}
		staff, _ := attackOf(d, "equipment:quarterstaff")
		if staff.Ability != STR {
			t.Errorf("%s: quarterstaff ability = %s, want STR (SRD Monk, Martial Arts: DEX only while unarmored)", tc.name, staff.Ability)
		}
	}
}

// SRD 5.1 Monk, Martial Arts: the die is "in place of the normal damage" of a
// monk weapon, so a two-handed quarterstaff also rolls the d10 at level 17.
func TestRulesReviewSheet_MartialArtsDieOnVersatile(t *testing.T) {
	t.Parallel()
	c := loadForTest(t)
	d := Derive(monkBuild(17, 9, 18), c)
	a, ok := attackOf(d, "equipment:quarterstaff")
	if !ok {
		t.Fatal("no quarterstaff attack")
	}
	if a.Damage != "1d10+4" || a.VersatileDamage != "1d10+4" {
		t.Errorf("quarterstaff Damage=%q Versatile=%q, want both 1d10+4 (SRD Monk, Martial Arts)", a.Damage, a.VersatileDamage)
	}
}

// SRD 5.1 Equipment, Weapon Properties, Heavy: Small creatures have
// disadvantage on attack rolls with heavy weapons.
func TestRulesReviewSheet_HeavyWeaponSmall(t *testing.T) {
	t.Parallel()
	c := loadForTest(t)
	b := standard("class:fighter", 1)
	b.Race, b.Subrace = "race:halfling", "subrace:lightfoot-halfling"
	b.Weapons = []string{"equipment:greatsword"}
	d := Derive(b, c)
	for _, h := range d.Hints {
		if h.Source == "equipment:greatsword" {
			return
		}
	}
	t.Errorf("hints = %+v, want a hint with Source equipment:greatsword (SRD Weapon Properties, Heavy: Small creatures have disadvantage on attack rolls)", d.Hints)
}

// SRD 5.1: an unarmed strike deals 1 + STR modifier bludgeoning damage; Monk
// Martial Arts lets it use the martial arts die and DEX.
func TestRulesReviewSheet_UnarmedStrike(t *testing.T) {
	t.Parallel()
	c := loadForTest(t)
	for _, class := range []string{"class:monk", "class:fighter"} {
		d := Derive(standard(class, 1), c)
		found := false
		for _, a := range d.Attacks {
			if strings.Contains(strings.ToLower(a.Key+a.Name), "unarmed") {
				found = true
			}
		}
		if !found {
			t.Errorf("%s with no weapons: Attacks = %d lines, want an unarmed strike line (SRD Combat, Unarmed Strike)", class, len(d.Attacks))
		}
	}
}

// SRD 5.1 Equipment, Armor (Strength): a character below the armor's Strength
// requirement has speed reduced by 10 feet.
func TestRulesReviewSheet_HeavyArmorStrengthSpeed(t *testing.T) {
	t.Parallel()
	c := loadForTest(t)
	b := standard("class:fighter", 1)
	b.BaseScores[STR] = 8
	b.Armor = "equipment:chain-mail"
	d := Derive(b, c)
	if d.SpeedWalkFt != 20 {
		t.Errorf("SpeedWalkFt = %d, want 20 (STR 9 < 13 in chain mail; SRD Armor, Strength requirement)", d.SpeedWalkFt)
	}
	b.Race, b.Subrace = "race:dwarf", "subrace:hill-dwarf"
	dd := Derive(b, c)
	if dd.SpeedWalkFt != 25 {
		t.Errorf("dwarf SpeedWalkFt = %d, want 25 (dwarf speed not reduced by heavy armor)", dd.SpeedWalkFt)
	}
}
