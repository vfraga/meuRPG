package rules

import (
	"slices"
	"testing"
)

func monkBuild(level, str, dex int) Build {
	b := standard("class:monk", level)
	b.BaseScores = map[Ability]int{STR: str, DEX: dex, CON: 13, INT: 12, WIS: 10, CHA: 8}
	b.Weapons = []string{"equipment:dagger", "equipment:quarterstaff"}
	return b
}

// Martial Arts (the monk die, DEX on monk weapons) holds only while the monk
// wears no armor and no shield.
func TestMartialArtsNeedsNoArmorOrShield(t *testing.T) {
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
			t.Errorf("%s: dagger damage = %q, want 1d4+4 (finesse with DEX, the weapon's own die)", tc.name, dagger.Damage)
		}
		staff, _ := attackOf(d, "equipment:quarterstaff")
		if staff.Ability != STR {
			t.Errorf("%s: quarterstaff ability = %s, want STR (DEX only while unarmored)", tc.name, staff.Ability)
		}
		unarmed, _ := attackOf(d, UnarmedStrikeKey)
		if unarmed.Ability != STR || unarmed.Damage != "1" {
			t.Errorf("%s: unarmed strike = %s %q, want STR and 1 (no monk die with armor)", tc.name, unarmed.Ability, unarmed.Damage)
		}
	}
	d := Derive(monkBuild(5, 9, 18), c)
	if dagger, _ := attackOf(d, "equipment:dagger"); dagger.Damage != "1d6+4" {
		t.Errorf("unarmored dagger damage = %q, want 1d6+4 (monk die)", dagger.Damage)
	}
}

// The monk die replaces the weapon's die on the two-handed line too.
func TestMonkDieAppliesToVersatileDamage(t *testing.T) {
	t.Parallel()
	c := loadForTest(t)
	for _, tc := range []struct {
		level int
		want  string
	}{{11, "1d8+4"}, {17, "1d10+4"}} {
		a, ok := attackOf(Derive(monkBuild(tc.level, 9, 18), c), "equipment:quarterstaff")
		if !ok {
			t.Fatal("no quarterstaff attack")
		}
		if a.Damage != tc.want || a.VersatileDamage != tc.want {
			t.Errorf("monk %d quarterstaff Damage=%q Versatile=%q, want both %s", tc.level, a.Damage, a.VersatileDamage, tc.want)
		}
	}
}

// Everyone has an unarmed strike: proficient, 1 + STR bludgeoning; a monk
// rolls the Martial Arts die and takes the better of DEX and STR.
func TestUnarmedStrike(t *testing.T) {
	t.Parallel()
	c := loadForTest(t)
	for _, tc := range []struct {
		name    string
		build   Build
		ability Ability
		bonus   int
		damage  string
	}{
		{"monk 1 DEX 16", Build{
			BaseScores: map[Ability]int{STR: 10, DEX: 16, CON: 10, INT: 10, WIS: 10, CHA: 10},
			Race:       "race:human", Classes: []ClassLevel{{Class: "class:monk", Level: 1}},
		}, DEX, 5, "1d4+3"},
		{"wizard 1 STR 10", Build{
			BaseScores: map[Ability]int{STR: 10, DEX: 10, CON: 10, INT: 10, WIS: 10, CHA: 10},
			Race:       "race:human", Classes: []ClassLevel{{Class: "class:wizard", Level: 1}},
		}, STR, 2, "1"},
		{"wizard 1 STR 16", Build{
			BaseScores: map[Ability]int{STR: 15, DEX: 18, CON: 10, INT: 10, WIS: 10, CHA: 10},
			Race:       "race:human", Classes: []ClassLevel{{Class: "class:wizard", Level: 1}},
		}, STR, 5, "4"},
	} {
		a, ok := attackOf(Derive(tc.build, c), UnarmedStrikeKey)
		if !ok {
			t.Errorf("%s: no unarmed strike line", tc.name)
			continue
		}
		if a.Ability != tc.ability || a.AttackBonus != tc.bonus || a.Damage != tc.damage || a.DamageType != "damage-type:bludgeoning" || !a.Melee || !a.Proficient {
			t.Errorf("%s: got %s %+d %q %s melee=%v proficient=%v, want %s %+d %q bludgeoning melee proficient",
				tc.name, a.Ability, a.AttackBonus, a.Damage, a.DamageType, a.Melee, a.Proficient, tc.ability, tc.bonus, tc.damage)
		}
	}
}

// A finesse weapon takes the better of STR and DEX whether it is melee or
// thrown (a dart).
func TestFinesseWeaponTakesBetterAbilityWhenRanged(t *testing.T) {
	t.Parallel()
	c := loadForTest(t)
	b := standard("class:fighter", 1)
	b.BaseScores = map[Ability]int{STR: 18, DEX: 10, CON: 13, INT: 12, WIS: 10, CHA: 8}
	b.Weapons = []string{"equipment:dart"}
	a, ok := attackOf(Derive(b, c), "equipment:dart")
	if !ok {
		t.Fatal("no dart attack")
	}
	str := abilityOf(Derive(b, c), STR).Modifier
	if a.Ability != STR || a.AttackBonus != str+2 || a.Damage != withModifier("1d4", str) {
		t.Errorf("dart = %s %+d %q, want STR %+d and 1d4%+d", a.Ability, a.AttackBonus, a.Damage, str+2, str)
	}
}

// A Small creature has disadvantage with a heavy weapon, and the sheet says
// so.
func TestHeavyWeaponHintForSmallRace(t *testing.T) {
	t.Parallel()
	c := loadForTest(t)
	b := standard("class:fighter", 1)
	b.Weapons = []string{"equipment:greatsword"}
	hasHint := func(d Derived) bool {
		for _, h := range d.Hints {
			if h.Source == "equipment:greatsword" && h.Mode == "disadvantage" {
				return true
			}
		}
		return false
	}
	if hasHint(Derive(b, c)) {
		t.Error("a Medium character got the heavy weapon hint")
	}
	b.Race, b.Subrace = "race:halfling", "subrace:lightfoot-halfling"
	if d := Derive(b, c); !hasHint(d) {
		t.Errorf("hints = %+v, want a disadvantage hint for the greatsword", d.Hints)
	}
}

// Eldritch Blast shows its beams: 1, then 2, 3 and 4 at warlock levels 5, 11
// and 17.
func TestEldritchBlastBeamCount(t *testing.T) {
	t.Parallel()
	c := loadForTest(t)
	for lv, want := range map[int]int{1: 1, 4: 1, 5: 2, 10: 2, 11: 3, 16: 3, 17: 4, 20: 4} {
		b := standard("class:warlock", lv)
		b.Cantrips = []string{"spell:eldritch-blast"}
		a, ok := attackOf(Derive(b, c), "spell:eldritch-blast")
		if !ok {
			t.Fatalf("level %d: no eldritch blast attack", lv)
		}
		if a.Beams != want {
			t.Errorf("level %d: Beams = %d, want %d", lv, a.Beams, want)
		}
	}
}

// A damage cantrip says the plain dice it rolls at the character's level, so the
// app shows the caster's row of the spell's table: Sacred Flame is 1d8, 2d8, 3d8
// and 4d8 at character levels 1, 5, 11 and 17.
func TestCantripSpellDiceFollowCharacterLevel(t *testing.T) {
	t.Parallel()
	c := loadForTest(t)
	for lv, want := range map[int]string{1: "1d8", 4: "1d8", 5: "2d8", 10: "2d8", 11: "3d8", 16: "3d8", 17: "4d8", 20: "4d8"} {
		b := standard("class:cleric", lv)
		b.Cantrips = []string{"spell:sacred-flame"}
		a, ok := attackOf(Derive(b, c), "spell:sacred-flame")
		if !ok {
			t.Fatalf("level %d: no sacred flame attack", lv)
		}
		if a.SpellDice != want {
			t.Errorf("level %d: SpellDice = %q, want %q", lv, a.SpellDice, want)
		}
	}
	w := standard("class:fighter", 5)
	for _, a := range Derive(w, c).Attacks {
		if a.Kind == "weapon" && a.SpellDice != "" {
			t.Errorf("weapon %s has SpellDice %q", a.Key, a.SpellDice)
		}
	}
}

// Heavy armor whose Strength requirement is not met costs 10 ft of speed;
// dwarves keep theirs.
func TestHeavyArmorStrengthRequirementCutsSpeed(t *testing.T) {
	t.Parallel()
	c := loadForTest(t)
	for _, tc := range []struct {
		name, race, subrace, armor string
		str, bonus, want           int
	}{
		{"human STR 10 chain mail (needs 13)", "race:human", "", "equipment:chain-mail", 10, 1, 20},
		{"human STR 13 chain mail", "race:human", "", "equipment:chain-mail", 13, 1, 30},
		{"human STR 14 splint (needs 15)", "race:human", "", "equipment:splint-armor", 14, 1, 20},
		{"human STR 15 splint", "race:human", "", "equipment:splint-armor", 15, 1, 30},
		{"human STR 14 plate (needs 15)", "race:human", "", "equipment:plate-armor", 14, 1, 20},
		{"human STR 15 plate", "race:human", "", "equipment:plate-armor", 15, 1, 30},
		{"human STR 8 leather", "race:human", "", "equipment:leather-armor", 8, 1, 30},
		{"hill dwarf STR 8 chain mail", "race:dwarf", "subrace:hill-dwarf", "equipment:chain-mail", 8, 0, 25},
	} {
		b := standard("class:fighter", 1)
		b.Race, b.Subrace = tc.race, tc.subrace
		b.BaseScores[STR] = tc.str - tc.bonus
		b.Armor = tc.armor
		d := Derive(b, c)
		if got := abilityOf(d, STR).Score; got != tc.str {
			t.Fatalf("%s: setup STR = %d, want %d", tc.name, got, tc.str)
		}
		if d.SpeedWalkFt != tc.want {
			t.Errorf("%s: speed = %d, want %d", tc.name, d.SpeedWalkFt, tc.want)
		}
	}
}

// A feature offers a fixed number of options, and an option name counts once
// even when two classes offer it.
func TestFightingStyleCountAndRepeats(t *testing.T) {
	c := loadForTest(t)
	t.Run("the same style from two classes does not stack", func(t *testing.T) {
		b := standard("class:fighter", 2)
		b.Classes = []ClassLevel{{Class: "class:fighter", Level: 2}, {Class: "class:paladin", Level: 2}}
		b.BaseScores[CHA] = 13
		b.Armor = "equipment:chain-mail"
		b.FeatureChoices = []string{"feature:fighter-fighting-style-defense", "feature:fighting-style-defense"}
		d := Derive(b, c)
		if d.ArmorClass != 17 {
			t.Errorf("AC = %d, want 17 (chain mail 16 + Defense once)", d.ArmorClass)
		}
		if !hasIssue(d, IssueChoiceCount) {
			t.Errorf("issues = %v, want a choice_count issue for the repeated style", d.Issues)
		}
	})
	t.Run("two styles for one feature raise an issue", func(t *testing.T) {
		b := standard("class:fighter", 1)
		b.FeatureChoices = []string{"feature:fighter-fighting-style-archery"}
		if hasIssue(Derive(b, c), IssueChoiceCount) {
			t.Fatal("one style raised an issue")
		}
		b.FeatureChoices = append(b.FeatureChoices, "feature:fighter-fighting-style-defense")
		if !hasIssue(Derive(b, c), IssueChoiceCount) {
			t.Error("two styles at Fighter 1: want a choice_count issue")
		}
	})
	t.Run("metamagic and pact boon are counted too", func(t *testing.T) {
		b := standard("class:sorcerer", 3)
		b.FeatureChoices = []string{"feature:metamagic-careful-spell", "feature:metamagic-distant-spell"}
		if hasIssue(Derive(b, c), IssueChoiceCount) {
			t.Fatal("two metamagic options at Sorcerer 3 raised an issue")
		}
		b.FeatureChoices = append(b.FeatureChoices, "feature:metamagic-empowered-spell")
		if !hasIssue(Derive(b, c), IssueChoiceCount) {
			t.Error("three metamagic options at Sorcerer 3: want a choice_count issue")
		}
		w := standard("class:warlock", 3)
		w.FeatureChoices = []string{"feature:pact-of-the-chain", "feature:pact-of-the-blade"}
		if !hasIssue(Derive(w, c), IssueChoiceCount) {
			t.Error("two pact boons: want a choice_count issue")
		}
	})
}

// Improved Critical (Champion 3) makes a weapon attack critical on 19 and 20,
// and Superior Critical (Champion 15) on 18 to 20; anyone else needs a 20.
func TestCriticalRangeFollowsTheChampionFeatures(t *testing.T) {
	t.Parallel()
	c := loadForTest(t)
	champion := func(level int) Build {
		b := standard("class:fighter", level)
		b.Classes[0].Subclass = "subclass:champion"
		return b
	}
	for _, tc := range []struct {
		name  string
		build Build
		want  int
	}{
		{"fighter 3 without a subclass", standard("class:fighter", 3), 20},
		{"champion 2", standard("class:fighter", 2), 20},
		{"champion 3", champion(3), 19},
		{"champion 14", champion(14), 19},
		{"champion 15", champion(15), 18},
		{"barbarian 20", standard("class:barbarian", 20), 20},
	} {
		if got := Derive(tc.build, c).CriticalRange; got != tc.want {
			t.Errorf("%s: CriticalRange = %d, want %d", tc.name, got, tc.want)
		}
	}
}

// The bonus action attack rules read the weapon's light property, the Martial
// Arts strike and whether the damage holds the ability modifier.
func TestAttacksCarryWhatTheBonusActionAttacksRead(t *testing.T) {
	t.Parallel()
	c := loadForTest(t)
	rogue := standard("class:rogue", 3)
	rogue.Weapons = []string{"equipment:shortsword", "equipment:longsword", "equipment:shortbow"}
	d := Derive(rogue, c)
	for _, key := range rogue.Weapons {
		if _, ok := attackOf(d, key); !ok {
			t.Fatalf("no %s attack", key)
		}
	}
	if sword, _ := attackOf(d, "equipment:shortsword"); !sword.Light || sword.AbilityMod != 3 {
		t.Errorf("shortsword = Light %v, AbilityMod %d; want a light weapon with the better modifier, +3", sword.Light, sword.AbilityMod)
	}
	if long, _ := attackOf(d, "equipment:longsword"); long.Light {
		t.Error("a longsword is not light")
	}
	if bow, _ := attackOf(d, "equipment:shortbow"); bow.Light {
		t.Error("a ranged weapon is never a light melee weapon")
	}

	armedMonk := monkBuild(2, 9, 18)
	armedMonk.Weapons = append(armedMonk.Weapons, "equipment:longsword")
	monk := Derive(armedMonk, c)
	if u, _ := attackOf(monk, UnarmedStrikeKey); !u.MartialArts {
		t.Error("a monk's unarmed strike goes with the Martial Arts strike")
	}
	if sword, _ := attackOf(monk, "equipment:longsword"); sword.MartialArts {
		t.Error("a longsword is not a monk weapon")
	}
	if staff, _ := attackOf(monk, "equipment:quarterstaff"); !staff.MartialArts {
		t.Error("a quarterstaff is a monk weapon")
	}
	armored := monkBuild(2, 9, 18)
	armored.Armor = "equipment:leather-armor"
	if u, _ := attackOf(Derive(armored, c), UnarmedStrikeKey); u.MartialArts {
		t.Error("an armored monk has no Martial Arts strike")
	}
	if u, _ := attackOf(Derive(standard("class:fighter", 1), c), UnarmedStrikeKey); u.MartialArts {
		t.Error("a fighter has no Martial Arts strike")
	}
}

// Two-Weapon Fighting, from either class, keeps the ability modifier on the
// bonus action attack.
func TestTwoWeaponFightingStyleIsOnTheSheet(t *testing.T) {
	t.Parallel()
	c := loadForTest(t)
	if Derive(standard("class:fighter", 1), c).TwoWeaponFighting {
		t.Error("a fighter without the style has TwoWeaponFighting")
	}
	f := standard("class:fighter", 1)
	f.FeatureChoices = []string{"feature:fighter-fighting-style-two-weapon-fighting"}
	if !Derive(f, c).TwoWeaponFighting {
		t.Error("a fighter with the style lacks TwoWeaponFighting")
	}
	r := standard("class:ranger", 2)
	r.FeatureChoices = []string{"feature:ranger-fighting-style-two-weapon-fighting"}
	if !Derive(r, c).TwoWeaponFighting {
		t.Error("a ranger with the style lacks TwoWeaponFighting")
	}
}

// duelist is a human fighter of the level with the Dueling style and the weapons.
func duelist(weapons ...string) Build {
	b := standard("class:fighter", 1)
	b.FeatureChoices = []string{"feature:fighter-fighting-style-dueling"}
	b.Weapons = weapons
	return b
}

// TestDuelingAddsTwoDamageToTheOneMeleeWeaponInOneHand: SRD 5.1 Dueling gives +2 to
// damage while a melee weapon is wielded in one hand and no other weapon is. The sheet
// lists the weapons carried, not the ones in hand, so the bonus is applied to the
// sole weapon of the sheet, in the damage it rolls in one hand, and left as a note for
// the master when there are other weapons.
func TestDuelingAddsTwoDamageToTheOneMeleeWeaponInOneHand(t *testing.T) {
	t.Parallel()
	c := loadForTest(t)
	damageOf := func(d Derived, key string) string {
		a, ok := attackOf(d, key)
		if !ok {
			t.Fatalf("no attack for %s in %+v", key, d.Attacks)
		}
		return a.Damage
	}
	dueling := func(d Derived) bool {
		return slices.ContainsFunc(d.Hints, func(h Hint) bool { return h.Source == "feature:fighter-fighting-style-dueling" })
	}

	// STR 16 (+3): a longsword rolls 1d8 + 3, +2 for Dueling.
	d := Derive(duelist("equipment:longsword"), c)
	if got := damageOf(d, "equipment:longsword"); got != "1d8+5" {
		t.Errorf("longsword damage with Dueling = %q, want 1d8+5", got)
	}
	if a, _ := attackOf(d, "equipment:longsword"); a.VersatileDamage != "1d10+3" {
		t.Errorf("longsword two-handed damage = %q, want 1d10+3 (no Dueling with two hands)", a.VersatileDamage)
	}
	if dueling(d) {
		t.Error("the applied bonus is also a hint for the master to apply again")
	}

	// A weapon wielded with two hands gets nothing.
	d = Derive(duelist("equipment:greatsword"), c)
	if got := damageOf(d, "equipment:greatsword"); got != "2d6+3" {
		t.Errorf("greatsword damage = %q, want 2d6+3", got)
	}
	if !dueling(d) {
		t.Error("with no weapon to apply it to, the bonus should stay a hint")
	}

	// Another weapon on the sheet: the app cannot tell what is in hand, so it applies nothing and leaves the hint.
	d = Derive(duelist("equipment:longsword", "equipment:dagger"), c)
	if got := damageOf(d, "equipment:longsword"); got != "1d8+3" {
		t.Errorf("longsword damage with two weapons carried = %q, want 1d8+3", got)
	}
	if !dueling(d) {
		t.Error("two weapons carried: the Dueling bonus should stay a hint for the master")
	}

	// A ranged weapon is not melee.
	if got := damageOf(Derive(duelist("equipment:longbow"), c), "equipment:longbow"); got != "1d8+2" {
		t.Errorf("longbow damage = %q, want 1d8+2 (DEX 14 +2, no Dueling)", got)
	}

	// Without the style there is no bonus.
	b := duelist("equipment:longsword")
	b.FeatureChoices = nil
	if got := damageOf(Derive(b, c), "equipment:longsword"); got != "1d8+3" {
		t.Errorf("longsword damage without Dueling = %q, want 1d8+3", got)
	}
}
