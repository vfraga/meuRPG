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

// TestDuelingAddsTwoDamageToEveryOneHandedMeleeWeapon: SRD 5.1 Dueling gives +2 to
// damage while a melee weapon is wielded in one hand and no other weapon is. The
// sheet lists the weapons, not the ones in hand, so the bonus is applied to
// the damage every one-handed melee weapon rolls in one hand, and the attack line
// says the condition for the table to check.
func TestDuelingAddsTwoDamageToEveryOneHandedMeleeWeapon(t *testing.T) {
	t.Parallel()
	c := loadForTest(t)
	const note = "Inclui +2 de Estilo de Luta: Duelismo (sem outra arma na mão)"
	attack := func(d Derived, key string) Attack {
		a, ok := attackOf(d, key)
		if !ok {
			t.Fatalf("no attack for %s in %+v", key, d.Attacks)
		}
		return a
	}
	dueling := func(d Derived) bool {
		return slices.ContainsFunc(d.Hints, func(h Hint) bool { return h.Source == "feature:fighter-fighting-style-dueling" })
	}

	// STR 16 (+3): a longsword rolls 1d8 + 3, +2 for Dueling, with the condition written on the line.
	d := Derive(duelist("equipment:longsword"), c)
	if a := attack(d, "equipment:longsword"); a.Damage != "1d8+5" || a.VersatileDamage != "1d10+3" || a.DamageNotePT != note {
		t.Errorf("longsword = %q / %q (%q), want 1d8+5, 1d10+3 two-handed and the Dueling note", a.Damage, a.VersatileDamage, a.DamageNotePT)
	}
	if dueling(d) {
		t.Error("the applied bonus is also a hint for the master to apply again")
	}

	// A sword and a bow: the sword gets the bonus, the bow does not.
	d = Derive(duelist("equipment:longsword", "equipment:longbow"), c)
	if a := attack(d, "equipment:longsword"); a.Damage != "1d8+5" || a.DamageNotePT != note {
		t.Errorf("longsword with a bow carried = %q (%q), want 1d8+5 with the note", a.Damage, a.DamageNotePT)
	}
	if a := attack(d, "equipment:longbow"); a.Damage != "1d8+2" || a.DamageNotePT != "" {
		t.Errorf("longbow = %q (%q), want 1d8+2 and no note", a.Damage, a.DamageNotePT)
	}

	// Two melee weapons: each one-handed line carries the bonus and the condition.
	d = Derive(duelist("equipment:longsword", "equipment:dagger"), c)
	if a := attack(d, "equipment:longsword"); a.Damage != "1d8+5" || a.DamageNotePT != note {
		t.Errorf("longsword beside a dagger = %q (%q)", a.Damage, a.DamageNotePT)
	}
	if a := attack(d, "equipment:dagger"); a.Damage != "1d4+5" || a.DamageNotePT != note {
		t.Errorf("dagger beside a longsword = %q (%q), want 1d4+5 with the note", a.Damage, a.DamageNotePT)
	}

	// A two-handed weapon gets nothing; with nothing to apply it to, the bonus stays a hint.
	d = Derive(duelist("equipment:greatsword"), c)
	if a := attack(d, "equipment:greatsword"); a.Damage != "2d6+3" || a.DamageNotePT != "" {
		t.Errorf("greatsword = %q (%q), want 2d6+3 and no note", a.Damage, a.DamageNotePT)
	}
	if !dueling(d) {
		t.Error("with no weapon to apply it to, the bonus should stay a hint")
	}

	// Without the style there is no bonus and no note.
	b := duelist("equipment:longsword")
	b.FeatureChoices = nil
	if a := attack(Derive(b, c), "equipment:longsword"); a.Damage != "1d8+3" || a.DamageNotePT != "" {
		t.Errorf("longsword without Dueling = %q (%q), want 1d8+3", a.Damage, a.DamageNotePT)
	}
}

// An improvised weapon is not a melee weapon of its kind: a bow used as a club gets no Dueling bonus, beside a sword that
// gets it or alone (SRD 5.1, "Improvised Weapons" and the Dueling style, a melee weapon wielded in one hand).
func TestDuelingNeverAppliesToTheImprovisedMeleeLineOfABow(t *testing.T) {
	t.Parallel()
	c := loadForTest(t)
	for name, weapons := range map[string][]string{
		"a bow alone":          {"equipment:shortbow"},
		"a bow beside a sword": {"equipment:longsword", "equipment:shortbow"},
	} {
		d := Derive(duelist(weapons...), c)
		club, ok := attackOf(d, "equipment:shortbow:melee")
		if !ok {
			t.Fatalf("%s: no improvised line", name)
		}
		if club.Damage != "1d4+3" || club.DamageNotePT != "" || club.DamageDice != (DiceFormula{Count: 1, Sides: 4, Bonus: 3}) {
			t.Errorf("%s: improvised line = %q (%q), want 1d4+3 and no Dueling note", name, club.Damage, club.DamageNotePT)
		}
	}
	// With only the bow there is no weapon the bonus applies to: it stays a hint, as for a two-handed weapon.
	d := Derive(duelist("equipment:shortbow"), c)
	if !slices.ContainsFunc(d.Hints, func(h Hint) bool { return h.Source == "feature:fighter-fighting-style-dueling" }) {
		t.Error("with a bow alone the Dueling bonus should stay a hint")
	}
}

// Brutal Critical (SRD 5.1, Barbarian) adds one weapon die to a melee critical hit
// at level 9, two at 13 and three at 17; nobody else has it.
func TestBrutalCriticalDiceFollowTheBarbarianLevel(t *testing.T) {
	t.Parallel()
	c := loadForTest(t)
	for _, tc := range []struct {
		class string
		level int
		want  int
	}{
		{"class:barbarian", 8, 0},
		{"class:barbarian", 9, 1},
		{"class:barbarian", 12, 1},
		{"class:barbarian", 13, 2},
		{"class:barbarian", 16, 2},
		{"class:barbarian", 17, 3},
		{"class:barbarian", 20, 3},
		{"class:fighter", 20, 0},
	} {
		if got := Derive(standard(tc.class, tc.level), c).BrutalCriticalDice; got != tc.want {
			t.Errorf("%s %d: BrutalCriticalDice = %d, want %d", tc.class, tc.level, got, tc.want)
		}
	}
}

// Reliable Talent (SRD 5.1, Rogue 11) lifts a d20 of 9 or lower to 10 on an ability
// check that adds the proficiency bonus: a skill the sheet is proficient in, with
// expertise too, and never a plain ability check, a saving throw or a skill without
// proficiency.
func TestReliableTalentCountsOnlyForChecksThatAddProficiency(t *testing.T) {
	t.Parallel()
	c := loadForTest(t)
	rogue := func(level int) Build {
		b := standard("class:rogue", level)
		b.SkillProficiencies = []string{"skill:acrobatics", "skill:stealth"}
		b.Expertise = []string{"skill:stealth", "skill:perception"}
		return b
	}
	checks := func(d Derived, keys ...string) map[string]bool {
		t.Helper()
		actions := make([]SceneAction, len(keys))
		for i, k := range keys {
			actions[i] = SceneAction{Key: k}
		}
		opts, err := SceneOptions(d, actions)
		if err != nil {
			t.Fatalf("SceneOptions: %v", err)
		}
		out := map[string]bool{}
		for _, o := range opts {
			out[o.Action.Key] = o.ReliableTalent
		}
		return out
	}
	keys := []string{"skill:acrobatics", "skill:stealth", "skill:arcana", "ability:dex", "save:dex"}

	if d := Derive(rogue(11), c); !d.ReliableTalent {
		t.Fatal("a level 11 rogue has no Reliable Talent")
	} else {
		got := checks(d, keys...)
		want := map[string]bool{"skill:acrobatics": true, "skill:stealth": true, "skill:arcana": false, "ability:dex": false, "save:dex": false}
		for k, w := range want {
			if got[k] != w {
				t.Errorf("level 11 rogue, %s: ReliableTalent = %v, want %v", k, got[k], w)
			}
		}
	}
	if d := Derive(rogue(10), c); d.ReliableTalent || checks(d, "skill:acrobatics")["skill:acrobatics"] {
		t.Error("a level 10 rogue has Reliable Talent")
	}
	if Derive(standard("class:fighter", 20), c).ReliableTalent {
		t.Error("a fighter has Reliable Talent")
	}
}

// A weapon that fires ammunition gets a second line, the improvised melee one: 1d4 plus Strength, no proficiency and
// no magic of the weapon, bludgeoning, and it is flagged so the combat spends no ammunition on it. The ammunition line
// names the kind; no other weapon gets an improvised line.
func TestAnAmmunitionWeaponHasAnImprovisedMeleeLine(t *testing.T) {
	t.Parallel()
	c := loadForTest(t)
	b := standard("class:fighter", 5)
	b.BaseScores = map[Ability]int{STR: 14, DEX: 16, CON: 13, INT: 12, WIS: 10, CHA: 8}
	b.Weapons = []string{"equipment:shortbow", "equipment:handaxe", "equipment:longsword", "equipment:crossbow-light"}
	d := Derive(b, c)
	bow, ok := attackOf(d, "equipment:shortbow")
	if !ok || bow.AmmunitionKey != "equipment:arrow" || bow.Improvised || bow.Melee {
		t.Fatalf("shortbow = %+v, want the ranged line that fires arrows", bow)
	}
	club, ok := attackOf(d, "equipment:shortbow:melee")
	if !ok {
		t.Fatal("the shortbow has no improvised melee line")
	}
	if !club.Improvised || !club.Melee || club.Kind != "weapon" || club.Ability != STR || club.AttackBonus != 2 || club.Proficient ||
		club.Damage != "1d4+2" || club.DamageDice != (DiceFormula{Count: 1, Sides: 4, Bonus: 2}) || club.DamageType != "damage-type:bludgeoning" ||
		club.AmmunitionKey != "" || club.MagicBonus != 0 || club.RangeFt > 5 || club.NamePT != bow.NamePT+", corpo a corpo" {
		t.Errorf("improvised line = %+v, want a melee 1d4+2 bludgeoning line at +2 with no proficiency and no ammunition", club)
	}
	if _, ok := attackOf(d, "equipment:crossbow-light:melee"); !ok {
		t.Error("the light crossbow has no improvised melee line")
	}
	for _, key := range []string{"equipment:handaxe", "equipment:longsword"} {
		if a, _ := attackOf(d, key); a.AmmunitionKey != "" || a.Improvised {
			t.Errorf("%s = %+v, want no ammunition and not improvised", key, a)
		}
		if _, ok := attackOf(d, key+":melee"); ok {
			t.Errorf("%s has an improvised line", key)
		}
	}
}

// A versatile weapon is held with both hands only when no shield and no other weapon is equipped (the grip
// follows the gear); a carried piece does not count as in hand.
func TestVersatileGripFollowsTheGear(t *testing.T) {
	t.Parallel()
	c := loadForTest(t)
	cases := []struct {
		name    string
		style   string
		weapons []string
		shield  bool
		carried []string
		want    Grip
	}{
		{"alone", styleDefense, []string{"equipment:longsword"}, false, nil, GripTwoHands},
		{"with a shield", styleDefense, []string{"equipment:longsword"}, true, nil, GripOneHandShield},
		{"with the shield carried", styleDefense, []string{"equipment:longsword"}, true, []string{ShieldKey}, GripTwoHands},
		{"with a second weapon", styleDefense, []string{"equipment:longsword", "equipment:dagger"}, false, nil, GripOneHandWeapon},
		{"with the second weapon carried", styleDefense, []string{"equipment:longsword", "equipment:dagger"}, false, []string{"equipment:dagger"}, GripTwoHands},
		{"with a key that is no weapon", styleDefense, []string{"equipment:longsword", "equipment:nonexistent"}, false, nil, GripTwoHands},
		{"with a greatsword and a shield", styleDefense, []string{"equipment:longsword", "equipment:greatsword"}, true, nil, GripOneHandWeapon},
		{"with a shield and a second weapon", styleDefense, []string{"equipment:longsword", "equipment:dagger"}, true, nil, GripOneHandShield},
		// The Dueling style keeps a lone weapon in one hand; a shield or a second weapon say their own grip.
		{"Dueling, alone", styleDueling, []string{"equipment:longsword"}, false, nil, GripOneHandDueling},
		{"Dueling, with a shield", styleDueling, []string{"equipment:longsword"}, true, nil, GripOneHandShield},
		{"Dueling, with a second weapon", styleDueling, []string{"equipment:longsword", "equipment:dagger"}, false, nil, GripOneHandWeapon},
		{"Dueling, with the shield and the second weapon carried", styleDueling, []string{"equipment:longsword", "equipment:dagger"}, true, []string{ShieldKey, "equipment:dagger"}, GripOneHandDueling},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			b := duelist(tc.weapons...)
			b.FeatureChoices = []string{tc.style}
			b.Shield, b.Carried = tc.shield, tc.carried
			d := Derive(b, c)
			sword, ok := attackOf(d, "equipment:longsword")
			if !ok {
				t.Fatalf("no longsword in %+v", d.Attacks)
			}
			if sword.Grip != tc.want {
				t.Errorf("longsword grip = %q, want %q", sword.Grip, tc.want)
			}
			if dagger, ok := attackOf(d, "equipment:dagger"); ok && dagger.Grip != GripNone {
				t.Errorf("a weapon that is not versatile has grip %q, want none", dagger.Grip)
			}
			// The Dueling note stands under the one-handed dice only: never under the dice of both hands.
			if note := sword.DamageNotePT != ""; note != (tc.style == styleDueling) || (note && sword.Grip == GripTwoHands) {
				t.Errorf("longsword note = %q with grip %q and style %s", sword.DamageNotePT, sword.Grip, tc.style)
			}
		})
	}
}

// Great Weapon Fighting takes a melee weapon wielded with two hands: a versatile weapon in both hands
// counts, the same weapon in one hand beside a shield does not, and a two-handed weapon always does.
func TestAWeaponInBothHandsFollowsTheGrip(t *testing.T) {
	t.Parallel()
	c := loadForTest(t)
	for _, tc := range []struct {
		name   string
		style  string
		weapon string
		shield bool
		want   bool
	}{
		{"a versatile weapon alone", styleDefense, "equipment:longsword", false, true},
		{"a versatile weapon beside a shield", styleDefense, "equipment:longsword", true, false},
		{"a two-handed weapon", styleDefense, "equipment:greatsword", false, true},
		{"a one-handed weapon", styleDefense, "equipment:shortsword", false, false},
		{"a versatile weapon alone, with Dueling", styleDueling, "equipment:longsword", false, false},
		{"a two-handed weapon, with Dueling", styleDueling, "equipment:greatsword", false, true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			b := duelist(tc.weapon)
			b.FeatureChoices = []string{tc.style}
			b.Shield = tc.shield
			a, ok := attackOf(Derive(b, c), tc.weapon)
			if !ok {
				t.Fatalf("no %s on the sheet", tc.weapon)
			}
			if got := a.InBothHands(); got != tc.want {
				t.Errorf("%s: in both hands = %v, want %v (grip %q)", tc.weapon, got, tc.want, a.Grip)
			}
		})
	}
}
