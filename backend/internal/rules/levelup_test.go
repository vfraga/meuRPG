package rules

import (
	"errors"
	"slices"
	"strings"
	"testing"
)

// torenLevelUp is the table's fighter: a human Champion, level 3, fixed hit points.
func torenLevelUp() Build {
	return Build{
		BaseScores: map[Ability]int{STR: 15, DEX: 12, CON: 14, INT: 8, WIS: 10, CHA: 10},
		Race:       "race:human", Background: "background:acolyte",
		Classes:            []ClassLevel{{Class: "class:fighter", Subclass: "subclass:champion", Level: 3}},
		SkillProficiencies: []string{"skill:acrobatics", "skill:perception"},
		Armor:              "equipment:chain-mail", Shield: true,
		Weapons:        []string{"equipment:longsword"},
		FeatureChoices: []string{"feature:fighter-fighting-style-defense"},
	}
}

// clericBuild is a level 3 cleric (Life), who prepares from the whole list.
func clericBuild(level int) Build {
	return Build{
		BaseScores: map[Ability]int{STR: 12, DEX: 10, CON: 14, INT: 10, WIS: 16, CHA: 10},
		Race:       "race:human", Background: "background:acolyte",
		Classes:            []ClassLevel{{Class: "class:cleric", Subclass: "subclass:life", Level: level}},
		SkillProficiencies: []string{"skill:medicine", "skill:religion"},
		Cantrips:           []string{"spell:guidance", "spell:light", "spell:sacred-flame"},
		SpellsPrepared:     []string{"spell:cure-wounds", "spell:bless", "spell:aid"},
	}
}

func sorcererBuild(level int) Build {
	b := Build{
		BaseScores: map[Ability]int{STR: 8, DEX: 14, CON: 14, INT: 10, WIS: 10, CHA: 16},
		Race:       "race:human", Background: "background:acolyte",
		Classes:            []ClassLevel{{Class: "class:sorcerer", Subclass: "subclass:draconic", Level: level}},
		SkillProficiencies: []string{"skill:arcana", "skill:persuasion"},
		Cantrips:           []string{"spell:fire-bolt", "spell:light", "spell:mage-hand", "spell:prestidigitation"},
		FeatureChoices:     []string{"trait:draconic-ancestry-red"},
	}
	b.SpellsKnown = []string{"spell:magic-missile", "spell:shield", "spell:burning-hands"}[:min(level+1, 3)]
	return b
}

func mustApply(t *testing.T, c *Content, before Build, ch LevelUpChoices) Build {
	t.Helper()
	after, err := ApplyLevelUp(before, ch, c)
	if err != nil {
		t.Fatalf("ApplyLevelUp() error = %v", err)
	}
	return after
}

// wantRefusal fails unless err is a *LevelUpError with this reason (and
// field, when given).
func wantRefusal(t *testing.T, err error, reason, field string) {
	t.Helper()
	le, ok := errors.AsType[*LevelUpError](err)
	if !ok {
		t.Fatalf("error = %v, want a LevelUpError %s", err, reason)
	}
	if le.Reason != reason || (field != "" && le.Field != field) {
		t.Errorf("refusal = {%s, %s} (%s), want {%s, %s}", le.Field, le.Reason, le.Message, field, reason)
	}
}

// TestLevelUpPensantus is the reference level-up: Pensantus, Wizard 3 to 4
// (RN-12). The numbers are the golden sheet's: Int 18 to 20, 23 to 30 hit
// points, save DC 14 to 15, prepared 7 to 9, cantrips 3 to 4, a spellbook of
// 10 to 12, and a third 2nd-circle slot.
func TestLevelUpPensantus(t *testing.T) {
	t.Parallel()
	c := loadForTest(t)
	before := pensantus()
	d0 := Derive(before, c)

	ch := LevelUpChoices{
		Class:           "class:wizard",
		AbilityIncrease: map[Ability]int{INT: 2},
		Cantrips:        []string{"spell:mage-hand"},
		Spells:          []string{"spell:misty-step", "spell:invisibility"},
		Prepared:        []string{"spell:misty-step", "spell:invisibility"},
		HitPoints:       LevelUpHitPoints{Average: true},
	}
	after := mustApply(t, c, before, ch)
	if err := CheckLevelUp(before, after, c); err != nil {
		t.Fatalf("CheckLevelUp() error = %v", err)
	}
	d1 := Derive(after, c)
	if len(d1.Issues) != 0 {
		t.Errorf("issues after = %v", d1.Issues)
	}
	wizard0, wizard1 := d0.Spellcasting[0], d1.Spellcasting[0]
	for what, p := range map[string][2]int{
		"Intelligence": {abilityOf(d0, INT).Score, 18}, "hit points": {d0.HitPointsMax, 23},
		"save DC": {wizard0.SaveDC, 14}, "prepared": {wizard0.PreparedMax, 7}, "2nd-circle slots": {d0.SpellSlots[1], 2},
	} {
		if p[0] != p[1] {
			t.Errorf("before: %s = %d, want %d", what, p[0], p[1])
		}
	}
	for what, p := range map[string][2]int{
		"Intelligence": {abilityOf(d1, INT).Score, 20}, "hit points": {d1.HitPointsMax, 30},
		"save DC": {wizard1.SaveDC, 15}, "prepared": {wizard1.PreparedMax, 9}, "2nd-circle slots": {d1.SpellSlots[1], 3},
		"cantrips": {len(after.Cantrips), 4}, "spellbook": {len(after.SpellsKnown), 12},
	} {
		if p[0] != p[1] {
			t.Errorf("after: %s = %d, want %d", what, p[0], p[1])
		}
	}
	if after.HitPoints.Method != HitPointsFixed || len(after.HitPoints.Rolls) != 0 {
		t.Errorf("hit points = %+v, want the fixed average untouched", after.HitPoints)
	}
	// ApplyLevelUp never changes its input.
	if before.Classes[0].Level != 3 || len(before.Cantrips) != 3 {
		t.Error("ApplyLevelUp changed the Build it was given")
	}

	// The Constitution case: the retroactive hit points, 23 to 34.
	con := mustApply(t, c, before, LevelUpChoices{
		Class: "class:wizard", AbilityIncrease: map[Ability]int{CON: 2}, HitPoints: LevelUpHitPoints{Average: true},
		Cantrips: ch.Cantrips, Spells: ch.Spells, Prepared: ch.Prepared[:1],
	})
	if err := CheckLevelUp(before, con, c); err != nil {
		t.Fatalf("CheckLevelUp(Con) error = %v", err)
	}
	if got := Derive(con, c).HitPointsMax; got != 34 {
		t.Errorf("hit points with Con +2 = %d, want 34", got)
	}
}

// TestLevelUpOptionsPensantus: what the screens draw for Wizard 3 to 4.
func TestLevelUpOptionsPensantus(t *testing.T) {
	t.Parallel()
	c := loadForTest(t)
	o, err := LevelUpOptions(pensantus(), "class:wizard", c)
	if err != nil {
		t.Fatalf("LevelUpOptions() error = %v", err)
	}
	if o.FromLevel != 3 || o.ToLevel != 4 || o.TotalTo != 4 || o.HitDie != 6 || o.HitPointAverage != 4 {
		t.Errorf("levels/die = %+v", o)
	}
	if !o.AbilityScoreImprovement || o.SubclassDue || o.Cantrips != 1 || o.Spells != 2 || o.SpellsKind != PreparationSpellbook {
		t.Errorf("choices = %+v", o)
	}
	if o.MaxSpellLevel != 2 || !o.Prepares || o.PreparedMax != 7 || o.PreparedMaxAfter != 8 {
		t.Errorf("spells = %+v (prepared 7 now, 8 with the level alone, 9 with Int +2)", o)
	}
	if o.SlotsBefore[1] != 2 || o.SlotsAfter[1] != 3 || o.ProficiencyBonusBefore != 2 || o.ProficiencyBonusAfter != 2 {
		t.Errorf("automatic = %+v", o)
	}
	if len(o.NewFeatures) != 1 || o.NewFeatures[0].Key != "feature:wizard-ability-score-improvement-1" {
		t.Errorf("new features = %+v", o.NewFeatures)
	}
}

// TestLevelUpToren: Fighter 3 to 4 has an ability increase, and 4 to 5 has
// nothing to choose but the hit points.
func TestLevelUpToren(t *testing.T) {
	t.Parallel()
	c := loadForTest(t)
	before := torenLevelUp()

	o, err := LevelUpOptions(before, "class:fighter", c)
	if err != nil {
		t.Fatal(err)
	}
	if !o.AbilityScoreImprovement || o.Cantrips != 0 || o.Spells != 0 || o.Prepares || o.SubclassDue || len(o.FeatureChoices) != 0 || o.HitDie != 10 {
		t.Errorf("Fighter 3 to 4 = %+v", o)
	}
	l4 := mustApply(t, c, before, LevelUpChoices{
		Class: "class:fighter", AbilityIncrease: map[Ability]int{STR: 1, CON: 1}, HitPoints: LevelUpHitPoints{Roll: 7},
	})
	if err := CheckLevelUp(before, l4, c); err != nil {
		t.Fatalf("CheckLevelUp(3 to 4) error = %v", err)
	}
	// A roll on a fixed sheet fills the earlier levels with their averages.
	if l4.HitPoints.Method != HitPointsRolled || !slices.Equal(l4.HitPoints.Rolls, []int{6, 6, 7}) {
		t.Errorf("hit points = %+v, want rolled with [6 6 7]", l4.HitPoints)
	}
	d3, d4 := Derive(before, c), Derive(l4, c)
	// Human Con 15 is +2: 10+2, then 6+2 twice is 28. The increase makes it
	// 16 (+3), which counts from level 1: 13+9+9 is 31, plus the roll 7+3.
	if d3.HitPointsMax != 28 || d4.HitPointsMax != 41 || len(d4.Issues) != 0 {
		t.Errorf("hit points %d to %d, issues %v", d3.HitPointsMax, d4.HitPointsMax, d4.Issues)
	}

	o5, err := LevelUpOptions(l4, "class:fighter", c)
	if err != nil {
		t.Fatal(err)
	}
	if o5.AbilityScoreImprovement || o5.SubclassDue || len(o5.FeatureChoices) != 0 || o5.Cantrips != 0 || o5.Spells != 0 || o5.SkillChoices != 0 {
		t.Errorf("Fighter 4 to 5 = %+v, want nothing to choose", o5)
	}
	l5 := mustApply(t, c, l4, LevelUpChoices{Class: "class:fighter", HitPoints: LevelUpHitPoints{Average: true}})
	if err := CheckLevelUp(l4, l5, c); err != nil {
		t.Fatalf("CheckLevelUp(4 to 5) error = %v", err)
	}
	// Average on a rolled sheet is one more roll, at the average.
	if !slices.Equal(l5.HitPoints.Rolls, []int{6, 6, 7, 6}) {
		t.Errorf("rolls = %v, want [6 6 7 6]", l5.HitPoints.Rolls)
	}
	if Derive(l5, c).AttacksPerAction != 2 {
		t.Error("Fighter 5 should have Extra Attack")
	}
}

// TestLevelUpClerics: a class that prepares from its whole list, no book.
func TestLevelUpCleric(t *testing.T) {
	t.Parallel()
	c := loadForTest(t)
	before := clericBuild(3)
	o, err := LevelUpOptions(before, "class:cleric", c)
	if err != nil {
		t.Fatal(err)
	}
	if o.Cantrips != 1 || o.Spells != 0 || o.SpellsKind != "" || !o.Prepares || o.PreparedMaxAfter != o.PreparedMax+1 || o.MaxSpellLevel != 2 {
		t.Errorf("Cleric 3 to 4 = %+v", o)
	}
	after := mustApply(t, c, before, LevelUpChoices{
		Class: "class:cleric", AbilityIncrease: map[Ability]int{WIS: 2},
		Cantrips: []string{"spell:mending"}, Prepared: []string{"spell:spiritual-weapon", "spell:lesser-restoration"},
		HitPoints: LevelUpHitPoints{Average: true},
	})
	if err := CheckLevelUp(before, after, c); err != nil {
		t.Fatalf("CheckLevelUp() error = %v", err)
	}
	// A cleric who adds a spell to the "known" list is refused: it has none.
	bad := mustApply(t, c, before, LevelUpChoices{
		Class: "class:cleric", Cantrips: []string{"spell:mending"}, Spells: []string{"spell:aid"}, HitPoints: LevelUpHitPoints{Average: true},
	})
	wantRefusal(t, CheckLevelUp(before, bad, c), LevelUpReasonSpells, "full.known_spell_keys")
}

// TestLevelUpSorcerer: a class that knows a fixed number of spells, and a
// level with options to choose (metamagic).
func TestLevelUpSorcerer(t *testing.T) {
	t.Parallel()
	c := loadForTest(t)
	before := sorcererBuild(2)
	o, err := LevelUpOptions(before, "class:sorcerer", c)
	if err != nil {
		t.Fatal(err)
	}
	if o.Spells != 1 || o.SpellsKind != PreparationKnown || o.Prepares || o.MaxSpellLevel != 2 {
		t.Errorf("Sorcerer 2 to 3 = %+v", o)
	}
	if len(o.FeatureChoices) != 1 || o.FeatureChoices[0].Choose != 2 || len(o.FeatureChoices[0].Options) < 3 {
		t.Fatalf("feature choices = %+v, want Metamagic: choose 2", o.FeatureChoices)
	}
	opts := o.FeatureChoices[0].Options
	ch := LevelUpChoices{
		Class: "class:sorcerer", Spells: []string{"spell:blur"},
		FeatureChoices: []string{opts[0].Key, opts[1].Key}, HitPoints: LevelUpHitPoints{Average: true},
	}
	after := mustApply(t, c, before, ch)
	if err := CheckLevelUp(before, after, c); err != nil {
		t.Fatalf("CheckLevelUp() error = %v", err)
	}
	// One option too few, or one that the features do not offer.
	for name, keys := range map[string][]string{
		"one option":   {opts[0].Key},
		"three":        {opts[0].Key, opts[1].Key, opts[2].Key},
		"not an offer": {opts[0].Key, "feature:fighter-fighting-style-defense"},
	} {
		ch.FeatureChoices = keys
		err := CheckLevelUp(before, mustApply(t, c, before, ch), c)
		if le, ok := errors.AsType[*LevelUpError](err); !ok || (le.Reason != LevelUpReasonFeatureChoice && le.Reason != LevelUpReasonSheetIssue) {
			t.Errorf("%s: error = %v, want a feature choice refusal", name, err)
		}
	}
}

// TestLevelUpSubclassDue: the subclass is chosen at the class's subclass
// level, and only then.
func TestLevelUpSubclassDue(t *testing.T) {
	t.Parallel()
	c := loadForTest(t)
	before := torenLevelUp()
	before.Classes[0].Subclass, before.Classes[0].Level = "", 2
	o, err := LevelUpOptions(before, "class:fighter", c)
	if err != nil {
		t.Fatal(err)
	}
	if !o.SubclassDue || len(o.Subclasses) != 1 || o.Subclasses[0].Key != "subclass:champion" {
		t.Fatalf("Fighter 2 to 3 = %+v", o)
	}
	ch := LevelUpChoices{Class: "class:fighter", Subclass: "subclass:champion", HitPoints: LevelUpHitPoints{Average: true}}
	after := mustApply(t, c, before, ch)
	if err := CheckLevelUp(before, after, c); err != nil {
		t.Fatalf("CheckLevelUp() error = %v", err)
	}
	if got := Derive(after, c).Classes[0].SubclassNamePT; got == "" {
		t.Error("the subclass did not apply")
	}
	ch.Subclass = ""
	wantRefusal(t, CheckLevelUp(before, mustApply(t, c, before, ch), c), LevelUpReasonSubclass, "full.classes[0].subclass_key")
	ch.Subclass = "subclass:evocation"
	wantRefusal(t, CheckLevelUp(before, mustApply(t, c, before, ch), c), LevelUpReasonSubclass, "full.classes[0].subclass_key")

	// Not due: a subclass chosen at the wrong level is refused.
	notDue := torenLevelUp()
	notDue.Classes[0].Subclass = ""
	d := mustApply(t, c, notDue, LevelUpChoices{Class: "class:fighter", Subclass: "subclass:champion", HitPoints: LevelUpHitPoints{Average: true}})
	wantRefusal(t, CheckLevelUp(notDue, d, c), LevelUpReasonSubclass, "full.classes[0].subclass_key")
}

// TestLevelUpHitPointsRolled: a rolled sheet keeps its rolls, and the new
// roll goes in the right place when more than one class has levels.
func TestLevelUpHitPointsRolled(t *testing.T) {
	t.Parallel()
	c := loadForTest(t)
	before := torenLevelUp()
	before.HitPoints = HitPoints{Method: HitPointsRolled, Rolls: []int{9, 3}}
	after := mustApply(t, c, before, LevelUpChoices{Class: "class:fighter", HitPoints: LevelUpHitPoints{Roll: 10}})
	if !slices.Equal(after.HitPoints.Rolls, []int{9, 3, 10}) {
		t.Errorf("rolls = %v", after.HitPoints.Rolls)
	}
	if err := CheckLevelUp(before, after, c); err != nil {
		t.Errorf("CheckLevelUp() error = %v", err)
	}

	// A rolled sheet that lost a roll gets the average for it, and the
	// result is still a legal level up.
	short := torenLevelUp()
	short.HitPoints = HitPoints{Method: HitPointsRolled, Rolls: []int{9}}
	after = mustApply(t, c, short, LevelUpChoices{Class: "class:fighter", HitPoints: LevelUpHitPoints{Roll: 2}})
	if !slices.Equal(after.HitPoints.Rolls, []int{9, 6, 2}) {
		t.Errorf("rolls = %v, want [9 6 2]", after.HitPoints.Rolls)
	}
	if err := CheckLevelUp(short, after, c); err != nil {
		t.Errorf("CheckLevelUp(short) error = %v", err)
	}

	// Two classes: the new level of the first class goes before the second's.
	multi := torenLevelUp()
	multi.Classes = append(multi.Classes, ClassLevel{Class: "class:wizard", Level: 2})
	multi.HitPoints = HitPoints{Method: HitPointsRolled, Rolls: []int{9, 3, 5, 4}} // F2 F3 W1 W2
	after = mustApply(t, c, multi, LevelUpChoices{Class: "class:fighter", HitPoints: LevelUpHitPoints{Roll: 8}})
	if !slices.Equal(after.HitPoints.Rolls, []int{9, 3, 8, 5, 4}) {
		t.Errorf("rolls = %v, want [9 3 8 5 4]", after.HitPoints.Rolls)
	}
	if err := CheckLevelUp(multi, after, c); err != nil {
		t.Errorf("CheckLevelUp(multi) error = %v", err)
	}
}

// TestLevelUpRefusals: every kind of thing the level does not allow, each
// with its field and reason.
func TestLevelUpRefusals(t *testing.T) {
	t.Parallel()
	c := loadForTest(t)
	good := LevelUpChoices{Class: "class:fighter", HitPoints: LevelUpHitPoints{Average: true}}
	asi := LevelUpChoices{Class: "class:fighter", AbilityIncrease: map[Ability]int{STR: 2}, HitPoints: LevelUpHitPoints{Average: true}}

	tests := []struct {
		name   string
		before Build
		change func(b *Build) // applied after ApplyLevelUp, to break the result
		ch     LevelUpChoices
		reason string
		field  string
	}{
		{"ASI where there is none", func() Build { b := torenLevelUp(); b.Classes[0].Level = 4; return b }(), nil, LevelUpChoices{Class: "class:fighter", AbilityIncrease: map[Ability]int{STR: 2}, HitPoints: good.HitPoints}, "", ""},
		{"locked: name of the race", torenLevelUp(), func(b *Build) { b.Race = "race:elf" }, asi, LevelUpReasonLocked, "full.race_key"},
		{"locked: base score", torenLevelUp(), func(b *Build) { b.BaseScores[DEX]++ }, asi, LevelUpReasonLocked, "full.base_scores.dexterity"},
		{"locked: armor", torenLevelUp(), func(b *Build) { b.Armor = "" }, asi, LevelUpReasonLocked, "full.armor_key"},
		{"locked: shield", torenLevelUp(), func(b *Build) { b.Shield = false }, asi, LevelUpReasonLocked, "full.shield"},
		{"locked: weapons", torenLevelUp(), func(b *Build) { b.Weapons = nil }, asi, LevelUpReasonLocked, "full.weapon_keys"},
		{"locked: background", torenLevelUp(), func(b *Build) { b.Background = "background:criminal" }, asi, LevelUpReasonLocked, "full.background_key"},
		{"no level gained", torenLevelUp(), func(b *Build) { b.Classes[0].Level = 3 }, asi, LevelUpReasonClass, "full.classes"},
		{"two levels", torenLevelUp(), func(b *Build) { b.Classes[0].Level = 5 }, asi, LevelUpReasonClass, "full.classes[0].level"},
		{"a new class", torenLevelUp(), func(b *Build) { b.Classes = append(b.Classes, ClassLevel{Class: "class:wizard", Level: 1}) }, asi, LevelUpReasonClass, "full.classes"},
		{"ASI of +1", torenLevelUp(), nil, LevelUpChoices{Class: "class:fighter", AbilityIncrease: map[Ability]int{STR: 1}, HitPoints: good.HitPoints}, LevelUpReasonAbilityShape, "full.extra_ability_bonuses"},
		{"ASI of +3", torenLevelUp(), nil, LevelUpChoices{Class: "class:fighter", AbilityIncrease: map[Ability]int{STR: 2, DEX: 1}, HitPoints: good.HitPoints}, LevelUpReasonAbilityShape, "full.extra_ability_bonuses"},
		{"ASI lowers", torenLevelUp(), func(b *Build) { b.ExtraAbilityBonuses = map[Ability]int{STR: -1, DEX: 2} }, good, LevelUpReasonAbilityShape, "full.extra_ability_bonuses.strength"},
		{"ASI past 20", func() Build { b := torenLevelUp(); b.BaseScores[STR] = 19; return b }(), nil, asi, LevelUpReasonAbilityAbove20, "full.extra_ability_bonuses.strength"},
		{"hit points above the die", torenLevelUp(), nil, LevelUpChoices{Class: "class:fighter", HitPoints: LevelUpHitPoints{Roll: 11}}, LevelUpReasonHitPoints, "full.hit_points.rolls[2]"},
		{"hit points of zero", torenLevelUp(), nil, LevelUpChoices{Class: "class:fighter", HitPoints: LevelUpHitPoints{Roll: 0}}, LevelUpReasonHitPoints, "full.hit_points.rolls[2]"},
		{
			"earlier rolls change", func() Build {
				b := torenLevelUp()
				b.HitPoints = HitPoints{Method: HitPointsRolled, Rolls: []int{9, 3}}
				return b
			}(),
			func(b *Build) { b.HitPoints.Rolls[0] = 10 },
			LevelUpChoices{Class: "class:fighter", HitPoints: LevelUpHitPoints{Roll: 5}},
			LevelUpReasonHitPoints, "full.hit_points.rolls",
		},
		{
			"rolled back to fixed", func() Build {
				b := torenLevelUp()
				b.HitPoints = HitPoints{Method: HitPointsRolled, Rolls: []int{9, 3}}
				return b
			}(),
			func(b *Build) { b.HitPoints = HitPoints{} }, good, LevelUpReasonHitPoints, "full.hit_points.method",
		},
		{"a cantrip for a fighter", torenLevelUp(), nil, LevelUpChoices{Class: "class:fighter", Cantrips: []string{"spell:light"}, HitPoints: good.HitPoints}, LevelUpReasonCantrips, "full.cantrip_keys"},
		{"a spell for a fighter", torenLevelUp(), nil, LevelUpChoices{Class: "class:fighter", Spells: []string{"spell:shield"}, HitPoints: good.HitPoints}, LevelUpReasonSpells, "full.known_spell_keys"},
		{"a prepared spell for a fighter", torenLevelUp(), nil, LevelUpChoices{Class: "class:fighter", Prepared: []string{"spell:shield"}, HitPoints: good.HitPoints}, LevelUpReasonPrepared, "full.prepared_spell_keys"},
		{"a fighting style from nowhere", torenLevelUp(), nil, LevelUpChoices{Class: "class:fighter", FeatureChoices: []string{"feature:fighter-fighting-style-dueling"}, HitPoints: good.HitPoints}, LevelUpReasonFeatureChoice, "full.feature_choice_keys"},
		{"an option removed", torenLevelUp(), func(b *Build) { b.FeatureChoices = nil }, good, LevelUpReasonFeatureChoice, "full.feature_choice_keys"},
		{"a skill for a fighter", torenLevelUp(), nil, LevelUpChoices{Class: "class:fighter", SkillProficiencies: []string{"skill:arcana"}, HitPoints: good.HitPoints}, LevelUpReasonSkills, "full.skill_proficiency_keys"},
		{"expertise for a fighter", torenLevelUp(), nil, LevelUpChoices{Class: "class:fighter", Expertise: []string{"skill:perception"}, HitPoints: good.HitPoints}, LevelUpReasonExpertise, "full.expertise_skill_keys"},
		{"a skill removed", torenLevelUp(), func(b *Build) { b.SkillProficiencies = b.SkillProficiencies[:1] }, good, LevelUpReasonSkills, "full.skill_proficiency_keys"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			after, err := ApplyLevelUp(tt.before, tt.ch, c)
			if tt.name == "no level gained" || tt.name == "two levels" || tt.name == "a new class" {
				// Break the Build the way a client could, from the plain next level.
				after, err = ApplyLevelUp(tt.before, good, c)
			}
			if err != nil {
				t.Fatalf("ApplyLevelUp() error = %v", err)
			}
			if tt.change != nil {
				tt.change(&after)
			}
			err = CheckLevelUp(tt.before, after, c)
			if tt.reason == "" {
				wantRefusal(t, err, LevelUpReasonAbilityNotDue, "full.extra_ability_bonuses")
				return
			}
			wantRefusal(t, err, tt.reason, tt.field)
		})
	}
}

// TestLevelUpRefusesBadSpells: the spells go through the same checks as any
// sheet, so a spell off the class list or above the new circle is refused.
func TestLevelUpRefusesBadSpells(t *testing.T) {
	t.Parallel()
	c := loadForTest(t)
	before := pensantus()
	base := LevelUpChoices{
		Class: "class:wizard", Cantrips: []string{"spell:mage-hand"}, Spells: []string{"spell:misty-step", "spell:invisibility"},
		HitPoints: LevelUpHitPoints{Average: true},
	}
	for _, mod := range map[string]func(ch *LevelUpChoices){
		"a cleric spell in the book": func(ch *LevelUpChoices) { ch.Spells = []string{"spell:misty-step", "spell:aid"} },
		"a 3rd-circle spell":         func(ch *LevelUpChoices) { ch.Spells = []string{"spell:misty-step", "spell:fireball"} },
		"a cleric cantrip":           func(ch *LevelUpChoices) { ch.Cantrips = []string{"spell:guidance"} },
		// The level allows 8 prepared; the book has them, but 4 more is 11.
		"too many prepared": func(ch *LevelUpChoices) {
			ch.Prepared = []string{"spell:misty-step", "spell:invisibility", "spell:find-familiar", "spell:detect-magic"}
		},
	} {
		ch := base
		mod(&ch)
		// A spell off the class's list is refused as such; the rest are sheet issues.
		err := CheckLevelUp(before, mustApply(t, c, before, ch), c)
		if le, ok := errors.AsType[*LevelUpError](err); ok && le.Reason == LevelUpReasonSpells && slices.Contains(ch.Spells, "spell:aid") {
			continue
		}
		wantRefusal(t, err, LevelUpReasonSheetIssue, "")
	}
	// One spell short.
	ch := base
	ch.Spells = []string{"spell:misty-step"}
	wantRefusal(t, CheckLevelUp(before, mustApply(t, c, before, ch), c), LevelUpReasonSpells, "full.known_spell_keys")
	// A removed spell.
	after := mustApply(t, c, before, base)
	after.SpellsKnown = slices.Delete(after.SpellsKnown, 0, 1)
	after.SpellsKnown = append(after.SpellsKnown, "spell:blur")
	wantRefusal(t, CheckLevelUp(before, after, c), LevelUpReasonSpells, "full.known_spell_keys")
}

func TestLevelUpLimits(t *testing.T) {
	t.Parallel()
	c := loadForTest(t)
	if _, err := LevelUpOptions(pensantus(), "class:fighter", c); err == nil {
		t.Error("LevelUpOptions for a class the character does not have worked")
	} else {
		wantRefusal(t, err, LevelUpReasonClass, "full.classes")
	}
	top := torenLevelUp()
	top.Classes[0].Level = 20
	_, err := LevelUpOptions(top, "class:fighter", c)
	wantRefusal(t, err, LevelUpReasonMaxLevel, "")
	_, err = ApplyLevelUp(top, LevelUpChoices{Class: "class:fighter"}, c)
	wantRefusal(t, err, LevelUpReasonMaxLevel, "")
}

// TestLevelUpOptionsForTheSubclassLevel: what a candidate subclass adds is in
// its own part of the options, so the options and the check agree (the
// College of Lore's three skills, the Circle of the Land's bonus cantrip).
func TestLevelUpOptionsForTheSubclassLevel(t *testing.T) {
	t.Parallel()
	c := loadForTest(t)
	b2 := sweepBase(t, c, "class:bard", "subclass:lore")
	b2 = mustApply(t, c, b2, satisfy(t, c, b2, "class:bard", "subclass:lore")) // level 2
	o, err := LevelUpOptions(b2, "class:bard", c)
	if err != nil {
		t.Fatal(err)
	}
	if !o.SubclassDue || o.SkillChoices != 0 || o.ExpertiseChoices != 2 || len(o.Subclasses) != 1 || o.Subclasses[0].SkillChoices != 3 {
		t.Errorf("Bard 2 to 3 = skills %d, expertise %d, subclasses %+v; want the 3 skills in the Lore part", o.SkillChoices, o.ExpertiseChoices, o.Subclasses)
	}
	// Without the skills the check refuses, as the options said.
	ch := satisfy(t, c, b2, "class:bard", "subclass:lore")
	ch.SkillProficiencies = nil
	wantRefusal(t, CheckLevelUp(b2, mustApply(t, c, b2, ch), c), LevelUpReasonSkills, "full.skill_proficiency_keys")

	d1 := sweepBase(t, c, "class:druid", "subclass:land")
	o, err = LevelUpOptions(d1, "class:druid", c)
	if err != nil {
		t.Fatal(err)
	}
	if !o.SubclassDue || o.Cantrips != 0 || o.Subclasses[0].Cantrips != 1 {
		t.Errorf("Druid 1 to 2 = cantrips %d, subclasses %+v; want the bonus cantrip in the Land part", o.Cantrips, o.Subclasses)
	}
}

// TestLevelUpGainsTheEngineModels: the Warlock's new invocations (from the
// class table), the Bard's Magical Secrets from any list, and the features the
// guided flow leaves to the master.
func TestLevelUpGainsTheEngineModels(t *testing.T) {
	t.Parallel()
	c := loadForTest(t)
	at := func(class, sub string, level int) Build {
		b := sweepBase(t, c, class, sub)
		for b.totalLevel() < level {
			b = mustApply(t, c, b, satisfy(t, c, b, class, sub))
		}
		return b
	}
	// The SRD's Invocations Known column: the snapshot has 3 at level 4 and 4
	// at level 6, which effects/corrections.json fixes.
	srd := []int{0, 2, 2, 2, 3, 3, 4, 4, 5, 5, 5, 6, 6, 6, 7, 7, 7, 8, 8, 8}
	for lvl, want := range srd {
		if got := invocationsKnown(c.c.classLevels["class:warlock"][lvl]); got != want {
			t.Errorf("invocations known at level %d = %d, want %d", lvl+1, got, want)
		}
	}
	for _, tt := range []struct {
		from int
		want int // new invocations on the way to from+1
	}{{3, 0}, {4, 1}, {5, 0}, {6, 1}, {8, 1}, {11, 1}, {14, 1}, {17, 1}} {
		o, err := LevelUpOptions(at("class:warlock", "subclass:fiend", tt.from), "class:warlock", c)
		if err != nil {
			t.Fatal(err)
		}
		got := 0
		for _, fc := range o.FeatureChoices {
			if fc.Feature.Key == "feature:eldritch-invocations" {
				got = fc.Choose
			}
		}
		if got != tt.want {
			t.Errorf("Warlock %d to %d: %d new invocations, want %d", tt.from, tt.from+1, got, tt.want)
		}
	}
	var o LevelUpOffer
	if o, _ = LevelUpOptions(at("class:warlock", "subclass:fiend", 10), "class:warlock", c); len(o.MasterAdds) != 1 || o.MasterAdds[0].Key != "feature:mystic-arcanum-6th-level" {
		t.Errorf("Warlock 10 to 11 master adds = %+v, want Mystic Arcanum", o.MasterAdds)
	}
	if o, _ = LevelUpOptions(at("class:wizard", "subclass:evocation", 17), "class:wizard", c); len(o.MasterAdds) != 1 || o.MasterAdds[0].Key != "feature:spell-mastery" {
		t.Errorf("Wizard 17 to 18 master adds = %+v, want Spell Mastery", o.MasterAdds)
	}
	bard := at("class:bard", "subclass:lore", 9)
	if o, _ = LevelUpOptions(bard, "class:bard", c); o.AnyClassSpells != 2 || o.Spells < 2 {
		t.Errorf("Bard 9 to 10 = any-class %d of %d spells, want 2", o.AnyClassSpells, o.Spells)
	}
	// Three off-list spells are one too many.
	ch := satisfy(t, c, bard, "class:bard", "subclass:lore")
	ch.Spells = pickSpells(c, "class:bard", 1, 5, bard.SpellsKnown, 3, true)[len(bard.SpellsKnown):]
	ch.Cantrips = ch.Cantrips[:0:0]
	if err := CheckLevelUp(bard, mustApply(t, c, bard, ch), c); err == nil {
		t.Error("CheckLevelUp accepted more off-list spells than Magical Secrets gives")
	}
}

// TestLevelUpRefusesDuplicates: a key twice in any list is refused, in the
// choices and against what the sheet already has.
func TestLevelUpRefusesDuplicates(t *testing.T) {
	t.Parallel()
	c := loadForTest(t)
	before := pensantus()
	good := LevelUpChoices{Class: "class:wizard", Cantrips: []string{"spell:mage-hand"}, Spells: []string{"spell:misty-step", "spell:blur"}, HitPoints: LevelUpHitPoints{Average: true}}
	for name, mod := range map[string]func(ch *LevelUpChoices){
		"a spell twice":        func(ch *LevelUpChoices) { ch.Spells = []string{"spell:blur", "spell:blur"} },
		"a spell it has":       func(ch *LevelUpChoices) { ch.Spells = []string{"spell:blur", "spell:shield"} },
		"a cantrip it has":     func(ch *LevelUpChoices) { ch.Cantrips = []string{"spell:fire-bolt"} },
		"a prepared one twice": func(ch *LevelUpChoices) { ch.Prepared = []string{"spell:blur", "spell:blur"} },
		"a skill twice":        func(ch *LevelUpChoices) { ch.SkillProficiencies = []string{"skill:history", "skill:history"} },
	} {
		ch := good
		mod(&ch)
		err := CheckLevelUp(before, mustApply(t, c, before, ch), c)
		if _, ok := errors.AsType[*LevelUpError](err); !ok {
			t.Errorf("%s: error = %v, want a refusal", name, err)
		}
	}
}

// TestLevelUpOffersTheLandDruidsTerrain: choosing the Circle of the Land at level 2
// asks which land the druid belongs to, with the SRD's seven (SRD 5.1, Druid, Circle
// of the Land, Circle Spells).
func TestLevelUpOffersTheLandDruidsTerrain(t *testing.T) {
	t.Parallel()
	c := loadForTest(t)
	d1 := sweepBase(t, c, "class:druid", "subclass:land")
	o, err := LevelUpOptions(d1, "class:druid", c)
	if err != nil {
		t.Fatal(err)
	}
	var land *LevelUpSubclass
	for i := range o.Subclasses {
		if o.Subclasses[i].Key == "subclass:land" {
			land = &o.Subclasses[i]
		}
	}
	if land == nil {
		t.Fatalf("subclasses = %+v, want the Land", o.Subclasses)
	}
	var terrain *LevelUpFeatureChoice
	for i := range land.FeatureChoices {
		if land.FeatureChoices[i].Feature.Key == "feature:circle-of-the-land" {
			terrain = &land.FeatureChoices[i]
		}
	}
	if terrain == nil || terrain.Choose != 1 || len(terrain.Options) != 7 {
		t.Fatalf("Land feature choices = %+v, want one terrain out of seven", land.FeatureChoices)
	}

	// A level taken with the choices the options asked for carries a terrain.
	ch := satisfy(t, c, d1, "class:druid", "subclass:land")
	if !slices.ContainsFunc(ch.FeatureChoices, func(k string) bool { return strings.HasPrefix(k, "feature:circle-of-the-land-") }) {
		t.Errorf("satisfy chose %v, want a terrain", ch.FeatureChoices)
	}
}
