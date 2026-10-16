package rules

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"testing"
)

// The characterization tests of the guided level-up freeze, before the split of
// levelUpOptionsWith, checkLevelUp, ApplyLevelUp and their helpers into smaller
// functions, what each of them answers: every offer of every class and subclass at
// every level (as digests, and a few in full), the Build ApplyLevelUp makes, and
// every refusal of CheckLevelUp with all its fields. Rewrite the goldens with
// -update only for a deliberate change, and review the diff.

// levelUpDigestLen is how many hex digits of a SHA-256 a digest keeps: enough to
// tell two offers apart, short enough to read in a diff.
const levelUpDigestLen = 16

// levelUpDigest is a short SHA-256 of v as JSON.
func levelUpDigest(t *testing.T, v any) string {
	t.Helper()
	raw, err := json.Marshal(v)
	if err != nil {
		t.Fatal(err)
	}
	sum := sha256.Sum256(raw)
	return hex.EncodeToString(sum[:])[:levelUpDigestLen]
}

// levelUpRefusalText is every field of a refusal, or the word accepted.
func levelUpRefusalText(err error) string {
	if err == nil {
		return "accepted"
	}
	if le := asRefusal(err); le != nil {
		return fmt.Sprintf("%+v", *le)
	}
	return "error: " + err.Error()
}

// levelUpGoldenJSON compares v, as indented JSON, with testdata/golden/<name>.json,
// or rewrites the file under -update.
func levelUpGoldenJSON(t *testing.T, name string, v any) {
	t.Helper()
	got, err := json.MarshalIndent(v, "", "  ")
	if err != nil {
		t.Fatal(err)
	}
	got = append(got, '\n')
	path := filepath.Join("testdata", "golden", name+".json")
	if *update {
		if err := os.WriteFile(path, got, 0o600); err != nil {
			t.Fatal(err)
		}
		return
	}
	want, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("%v (run with -update to create it)", err)
	}
	if !bytes.Equal(got, want) {
		t.Errorf("%s differs from the golden; if the change is right, run with -update and review the diff.\ngot:\n%s", path, got)
	}
}

// levelUpCharAt is a character of the class (and subclass) leveled by the sweep's
// picks up to the total level.
func levelUpCharAt(t *testing.T, c *Content, class, sub string, level int) Build {
	t.Helper()
	b := sweepBase(t, c, class, sub)
	for b.totalLevel() < level {
		b = mustApply(t, c, b, satisfy(t, c, b, class, sub))
	}
	return b
}

// levelUpSweepDigests levels the class from 1 to 20 with the sweep's picks and
// records, at each level, digests of the offer, of the Build ApplyLevelUp makes
// and of the offers of every other class (as a new class), with the check's
// answer.
func levelUpSweepDigests(t *testing.T, c *Content, classKey, subKey string, got map[string]string) {
	t.Helper()
	b := sweepBase(t, c, classKey, subKey)
	for b.totalLevel() < MaxLevel {
		o, err := LevelUpOptions(b, classKey, c)
		if err != nil {
			t.Fatalf("%s/%s at %d: %v", classKey, subKey, b.totalLevel(), err)
		}
		others := map[string]string{}
		for _, other := range sortedKeys(c.c.classes) {
			if other == classKey {
				continue
			}
			oo, err := LevelUpOptions(b, other, c)
			if err != nil {
				others[other] = levelUpRefusalText(err)
				continue
			}
			others[other] = levelUpDigest(t, oo)
		}
		ch := satisfy(t, c, b, classKey, subKey)
		after := mustApply(t, c, b, ch)
		key := fmt.Sprintf("%s/%s/%02d", classKey, subKey, b.totalLevel())
		got[key] = fmt.Sprintf("options %s, others %s, apply %s, check %s",
			levelUpDigest(t, o), levelUpDigest(t, others), levelUpDigest(t, after), levelUpRefusalText(CheckLevelUp(b, after, c)))
		b = after
	}
}

// TestLevelUpOptionsCharacterization freezes every offer of the SRD's classes and
// subclasses, and of the table's (the generated classes and the third casters),
// at every level, with the offers of the other classes as a new class.
func TestLevelUpOptionsCharacterization(t *testing.T) {
	t.Parallel()
	srd := loadForTest(t)
	table := thirdCasterContent(t)
	got := map[string]string{}
	for _, classKey := range sortedKeys(srd.c.classes) {
		for _, subKey := range srd.c.classes[classKey].Subclasses {
			levelUpSweepDigests(t, srd, classKey, subKey, got)
		}
	}
	for _, classKey := range sortedKeys(table.c.classes) {
		for _, subKey := range table.c.classes[classKey].Subclasses {
			if isTableKey(classKey) || isTableKey(subKey) {
				levelUpSweepDigests(t, table, classKey, subKey, got)
			}
		}
	}
	goldenText(t, "levelup-options-digests", got)
}

// TestLevelUpOptionsInFullCharacterization keeps a few offers whole, so a change
// the digests show can be read: an Ability Score Improvement with the table's
// feats, a spellbook, a subclass that is due (with a third caster's part), the
// Warlock's invocations with the blocked ones, Magical Secrets and a new class.
func TestLevelUpOptionsInFullCharacterization(t *testing.T) {
	t.Parallel()
	srd := loadForTest(t)
	feats := tableFeats(t)
	table := thirdCasterContent(t)
	cases := []struct {
		name  string
		c     *Content
		b     Build
		class string
	}{
		{"fighter 3 to 4 with the table's feats", feats, torenLevelUp(), "class:fighter"},
		{"wizard 3 to 4", srd, pensantus(), "class:wizard"},
		{"bard 2 to 3, the subclass due", srd, levelUpCharAt(t, srd, "class:bard", "subclass:lore", 2), "class:bard"},
		{"bard 9 to 10, Magical Secrets", srd, levelUpCharAt(t, srd, "class:bard", "subclass:lore", 9), "class:bard"},
		{"warlock 4 to 5, invocations", srd, levelUpCharAt(t, srd, "class:warlock", "subclass:fiend", 4), "class:warlock"},
		{"fighter 5 enters the wizard", srd, doranFighter5(), "class:wizard"},
		{"fighter 5 enters the bard", srd, richFighter5(), "class:bard"},
		{"fighter 2 to 3, a third caster due", table, levelUpCharAt(t, table, "class:fighter", "subclass:champion", 2), "class:fighter"},
		{"rogue 2 to 3, a third caster due", table, levelUpCharAt(t, table, "class:rogue", "subclass:thief", 2), "class:rogue"},
	}
	out := map[string]any{}
	for _, tc := range cases {
		o, err := LevelUpOptions(tc.b, tc.class, tc.c)
		if err != nil {
			out[tc.name] = levelUpRefusalText(err)
			continue
		}
		out[tc.name] = o
	}
	levelUpGoldenJSON(t, "levelup-options", out)
}

// TestApplyLevelUpCharacterization freezes the Build ApplyLevelUp makes for each
// kind of choice, and its refusals.
func TestApplyLevelUpCharacterization(t *testing.T) {
	t.Parallel()
	srd := loadForTest(t)
	fixed := levelUpFixedFeats(t)
	avg := LevelUpHitPoints{Average: true}
	rolled := torenLevelUp()
	rolled.HitPoints = HitPoints{Method: HitPointsRolled, Rolls: []int{9, 3}}
	top := torenLevelUp()
	top.Classes[0].Level = MaxLevel
	warlock := levelUpCharAt(t, srd, "class:warlock", "subclass:fiend", 4)
	cases := []struct {
		name string
		c    *Content
		b    Build
		ch   LevelUpChoices
	}{
		{"ASI on a fixed sheet", srd, torenLevelUp(), LevelUpChoices{Class: "class:fighter", AbilityIncrease: map[Ability]int{STR: 1, DEX: 1}, HitPoints: avg}},
		{"a roll on a fixed sheet", srd, torenLevelUp(), LevelUpChoices{Class: "class:fighter", HitPoints: LevelUpHitPoints{Roll: 7}}},
		{"the average on a rolled sheet", srd, rolled, LevelUpChoices{Class: "class:fighter", HitPoints: avg}},
		{"a roll on a rolled sheet", srd, rolled, LevelUpChoices{Class: "class:fighter", HitPoints: LevelUpHitPoints{Roll: 2}}},
		{"spells and a subclass", srd, pensantus(), LevelUpChoices{
			Class: "class:wizard", Subclass: "subclass:evocation", Cantrips: []string{"spell:mage-hand"},
			Spells: []string{"spell:misty-step"}, Prepared: []string{"spell:misty-step"}, HitPoints: avg,
		}},
		{"choices, text, skills and expertise", srd, torenLevelUp(), LevelUpChoices{
			Class: "class:fighter", FeatureChoices: []string{"feature:x"}, LateChoices: []string{"feature:late"},
			FeatureChoiceText: map[string]string{"feature:x": "texto"}, SkillProficiencies: []string{"skill:arcana"},
			Expertise: []string{"skill:perception"}, HitPoints: avg,
		}},
		{
			"text on a sheet that has some", srd, func() Build { b := torenLevelUp(); b.FeatureChoiceText = map[string]string{"a": "1"}; return b }(),
			LevelUpChoices{Class: "class:fighter", FeatureChoiceText: map[string]string{"b": "2"}, HitPoints: avg},
		},
		{"an invocation swapped", srd, warlock, LevelUpChoices{
			Class: "class:warlock", SwapInvocation: levelUpFirstInvocation(srd, warlock), FeatureChoices: []string{"feature:eldritch-invocation-beast-speech"}, HitPoints: avg,
		}},
		{"a new class with an instrument", srd, richFighter5(), LevelUpChoices{Class: "class:bard", Instrument: "proficiency:lute", HitPoints: LevelUpHitPoints{Roll: 3}}},
		{"an unknown instrument", srd, richFighter5(), LevelUpChoices{Class: "class:bard", Instrument: "proficiency:nada", HitPoints: avg}},
		{"a feat that raises what it lists", fixed, torenLevelUp(), LevelUpChoices{Class: "class:fighter", Feat: "feat:robusto" + tableSuffix, HitPoints: avg}},
		{
			"a feat that raises what it lists, at 20", fixed, func() Build { b := torenLevelUp(); b.BaseScores[STR] = 19; return b }(),
			LevelUpChoices{Class: "class:fighter", Feat: "feat:robusto" + tableSuffix, HitPoints: avg},
		},
		{
			"a feat that raises what it lists, with an increase sent", fixed, torenLevelUp(),
			LevelUpChoices{Class: "class:fighter", Feat: "feat:robusto" + tableSuffix, AbilityIncrease: map[Ability]int{CON: 1}, HitPoints: avg},
		},
		{"a feat to choose from", fixed, torenLevelUp(), LevelUpChoices{Class: "class:fighter", Feat: "feat:meio" + tableSuffix, AbilityIncrease: map[Ability]int{DEX: 1}, HitPoints: avg}},
		{
			"a feat on a sheet with feats", fixed, func() Build { b := torenLevelUp(); b.FeatSlots = map[string]string{"x": "y"}; return b }(),
			LevelUpChoices{Class: "class:fighter", Feat: "feat:meio" + tableSuffix, HitPoints: avg},
		},
		{"an unknown feat", fixed, torenLevelUp(), LevelUpChoices{Class: "class:fighter", Feat: "feat:nada", HitPoints: avg}},
		{"refused: unknown class", srd, torenLevelUp(), LevelUpChoices{Class: "class:artificer"}},
		{"refused: level 20", srd, top, LevelUpChoices{Class: "class:fighter"}},
		{"refused: the prerequisite", srd, torenLevelUp(), LevelUpChoices{Class: "class:wizard"}},
	}
	out := map[string]any{}
	for _, tc := range cases {
		before := tc.b.clone()
		after, err := ApplyLevelUp(tc.b, tc.ch, tc.c)
		if err != nil {
			out[tc.name] = levelUpRefusalText(err)
			continue
		}
		out[tc.name] = after
		if levelUpDigest(t, before) != levelUpDigest(t, tc.b) {
			t.Errorf("%s: ApplyLevelUp changed its input", tc.name)
		}
	}
	levelUpGoldenJSON(t, "levelup-apply", out)
}

// levelUpFixedFeats is the SRD with two table feats: one that raises Strength and
// Constitution by 1 each (nothing to choose) and one that raises one of
// Strength or Dexterity by 1.
func levelUpFixedFeats(t *testing.T) *Content {
	t.Helper()
	return withOverlayOn(t, loadForTest(t), Overlay{Revision: 1, Feats: []TableFeat{
		{
			TableEntry: TableEntry{Key: "feat:robusto" + tableSuffix, NamePT: "Robusto"}, DescPT: []string{"x"},
			Effects: []Effect{{Type: "ability_increase", Count: 2, From: []string{"str", "con"}, Value: "1"}},
		},
		{
			TableEntry: TableEntry{Key: "feat:meio" + tableSuffix, NamePT: "Meio"}, DescPT: []string{"x"},
			Effects: []Effect{{Type: "ability_increase", Count: 1, From: []string{"str", "dex"}, Value: "1"}},
		},
	}})
}

// levelUpFirstInvocation is the first Eldritch Invocation the sheet has.
func levelUpFirstInvocation(c *Content, b Build) string {
	for _, k := range b.FeatureChoices {
		if c.c.isInvocation(k) {
			return k
		}
	}
	return ""
}

// levelUpCheckCase is a level up to check: before, the sheet sent, and the content.
type levelUpCheckCase struct {
	c             *Content
	before, after Build
}

// levelUpCheckCases breaks a good level up one way at a time (and a few at once,
// to freeze which refusal comes first), for every refusal of CheckLevelUp.
func levelUpCheckCases(t *testing.T) map[string]levelUpCheckCase {
	t.Helper()
	srd := loadForTest(t)
	feats := tableFeats(t)
	fixed := levelUpFixedFeats(t)
	avg := LevelUpHitPoints{Average: true}
	fighter := func(ch LevelUpChoices) LevelUpChoices {
		ch.Class, ch.HitPoints = "class:fighter", avg
		return ch
	}
	asi := fighter(LevelUpChoices{AbilityIncrease: map[Ability]int{STR: 2}})
	plain := fighter(LevelUpChoices{})
	at := func(c *Content, before Build, ch LevelUpChoices, edit func(b *Build)) levelUpCheckCase {
		after := mustApply(t, c, before, ch)
		if edit != nil {
			edit(&after)
		}
		return levelUpCheckCase{c: c, before: before, after: after}
	}
	toren := torenLevelUp()
	toren4 := mustApply(t, srd, toren, asi)
	edit := func(b Build, f func(b *Build)) Build {
		b = b.clone()
		f(&b)
		return b
	}
	rolled := edit(toren, func(b *Build) { b.HitPoints = HitPoints{Method: HitPointsRolled, Rolls: []int{9, 3}} })
	wizardBook := LevelUpChoices{
		Class: "class:wizard", Cantrips: []string{"spell:mage-hand"}, Spells: []string{"spell:misty-step", "spell:invisibility"},
		HitPoints: avg,
	}
	withWizard := func(f func(ch *LevelUpChoices)) LevelUpChoices {
		ch := wizardBook
		ch.Cantrips, ch.Spells = slices.Clone(ch.Cantrips), slices.Clone(ch.Spells)
		f(&ch)
		return ch
	}
	cases := map[string]levelUpCheckCase{
		// Accepted.
		"ok: ASI +2":                   at(srd, toren, asi, nil),
		"ok: ASI +1 and +1":            at(srd, toren, fighter(LevelUpChoices{AbilityIncrease: map[Ability]int{STR: 1, DEX: 1}}), nil),
		"ok: a level without an ASI":   at(srd, toren4, plain, nil),
		"ok: a roll":                   at(srd, toren, LevelUpChoices{Class: "class:fighter", AbilityIncrease: map[Ability]int{STR: 2}, HitPoints: LevelUpHitPoints{Roll: 1}}, nil),
		"ok: a roll on a rolled sheet": at(srd, rolled, LevelUpChoices{Class: "class:fighter", AbilityIncrease: map[Ability]int{STR: 2}, HitPoints: LevelUpHitPoints{Roll: 10}}, nil),
		"ok: the wizard":               at(srd, pensantus(), wizardBook, nil),
		"ok: the wizard prepares":      at(srd, pensantus(), withWizard(func(ch *LevelUpChoices) { ch.Prepared = []string{"spell:misty-step"} }), nil),
		"ok: a new class":              at(srd, doranFighter5(), wizardOneChoices(), nil),

		// A new class.
		"new class: unknown": at(srd, toren, asi, func(b *Build) { b.Classes = append(b.Classes, ClassLevel{Class: "class:artificer", Level: 1}) }),
		"new class: twice":   at(srd, toren, asi, func(b *Build) { b.Classes = append(b.Classes, ClassLevel{Class: "class:fighter", Level: 1}) }),
		"new class: prerequisite": at(srd, toren, plain, func(b *Build) {
			b.Classes = append(b.Classes[:1:1], ClassLevel{Class: "class:wizard", Level: 1})
			b.Classes[0].Level = 3
		}),
		"new class: prerequisite of a class it has": {
			c: srd, before: wizardWith(map[Ability]int{STR: 13, DEX: 10, CON: 10, INT: 12, WIS: 10, CHA: 10}),
			after: edit(wizardWith(map[Ability]int{STR: 13, DEX: 10, CON: 10, INT: 12, WIS: 10, CHA: 10}), func(b *Build) {
				b.Classes = append(b.Classes, ClassLevel{Class: "class:fighter", Level: 1})
			}),
		},
		"new class: at level 2": at(srd, toren, plain, func(b *Build) {
			b.Classes = append(b.Classes[:1:1], ClassLevel{Class: "class:barbarian", Level: 2})
			b.Classes[0].Level = 3
		}),
		"new class: the bard without its instrument": at(srd, richFighter5(), LevelUpChoices{Class: "class:bard", HitPoints: avg}, nil),
		"new class: the rogue without its skill":     at(srd, richFighter5(), LevelUpChoices{Class: "class:rogue", Expertise: []string{"skill:athletics", "skill:perception"}, HitPoints: avg}, nil),
		"new class: the rogue with a skill it has": at(srd, richFighter5(), LevelUpChoices{
			Class: "class:rogue", SkillProficiencies: []string{"skill:athletics"}, Expertise: []string{"skill:athletics", "skill:perception"}, HitPoints: avg,
		}, nil),
		"new class: a tool from nowhere": at(srd, doranFighter5(), wizardOneChoices(), func(b *Build) { b.ToolProficiencies = []string{"Alaúde"} }),

		// The classes.
		"classes: one removed": {c: srd, before: mustApply(t, srd, doranFighter5(), wizardOneChoices()), after: doranFighter5()},
		"classes: one changed": at(srd, toren, asi, func(b *Build) { b.Classes[0].Class = "class:barbarian" }),
		"classes: no level":    at(srd, toren, asi, func(b *Build) { b.Classes[0].Level = 3 }),
		"classes: two levels":  at(srd, toren, asi, func(b *Build) { b.Classes[0].Level = 5 }),
		"classes: two gain": func() levelUpCheckCase {
			before := mustApply(t, srd, doranFighter5(), wizardOneChoices())
			return at(srd, before, LevelUpChoices{Class: "class:fighter", HitPoints: avg}, func(b *Build) { b.Classes[1].Level = 2 })
		}(),
		"classes: past 20": {c: srd, before: edit(toren, func(b *Build) { b.Classes[0].Level = MaxLevel }), after: edit(toren, func(b *Build) { b.Classes[0].Level = MaxLevel + 1 })},

		// Locked fields.
		"locked: base score":                  at(srd, toren, asi, func(b *Build) { b.BaseScores[DEX]++ }),
		"locked: race":                        at(srd, toren, asi, func(b *Build) { b.Race = "race:elf" }),
		"locked: subrace":                     at(srd, toren, asi, func(b *Build) { b.Subrace = "subrace:high-elf" }),
		"locked: background":                  at(srd, toren, asi, func(b *Build) { b.Background = "background:criminal" }),
		"locked: custom background":           at(srd, toren, asi, func(b *Build) { b.CustomBackgroundName = "Meu" }),
		"locked: custom background equipment": at(srd, toren, asi, func(b *Build) { b.CustomBackgroundEquipment = "x" }),
		"locked: armor":                       at(srd, toren, asi, func(b *Build) { b.Armor = "" }),
		"locked: shield":                      at(srd, toren, asi, func(b *Build) { b.Shield = false }),
		"locked: weapons":                     at(srd, toren, asi, func(b *Build) { b.Weapons = nil }),
		"locked: tools":                       at(srd, toren, asi, func(b *Build) { b.ToolProficiencies = []string{"x"} }),
		"locked: base score and race":         at(srd, toren, asi, func(b *Build) { b.BaseScores[DEX]++; b.Race = "race:elf" }),

		// Feats.
		"feat: ok":                 at(feats, toren, fighter(LevelUpChoices{Feat: "feat:forte" + tableSuffix}), nil),
		"feat: removed":            at(feats, edit(toren, func(b *Build) { b.Feats = []string{"feat:livre" + tableSuffix} }), asi, func(b *Build) { b.Feats = nil }),
		"feat: two":                at(feats, toren, fighter(LevelUpChoices{Feat: "feat:livre" + tableSuffix}), func(b *Build) { b.Feats = append(b.Feats, "feat:forte"+tableSuffix) }),
		"feat: not an ASI level":   at(feats, toren4, fighter(LevelUpChoices{Feat: "feat:livre" + tableSuffix}), nil),
		"feat: unknown":            at(feats, toren, fighter(LevelUpChoices{Feat: "feat:nada"}), nil),
		"feat: prerequisite":       at(feats, toren, fighter(LevelUpChoices{Feat: "feat:agil-ou-esperto" + tableSuffix}), nil),
		"feat: level prerequisite": at(feats, toren, fighter(LevelUpChoices{Feat: "feat:veterano" + tableSuffix}), nil),
		"feat: twice in the list":  at(feats, edit(toren, func(b *Build) { b.Feats = []string{"feat:livre" + tableSuffix} }), fighter(LevelUpChoices{Feat: "feat:livre" + tableSuffix}), nil),
		"feat: already picked by a choice": func() levelUpCheckCase {
			c, race := featChoiceTable(t)
			b := featChoiceBuild(race, alertaFeat)
			b.Classes[0].Level, b.Classes[0].Subclass = 3, "subclass:champion"
			return at(c, b, fighter(LevelUpChoices{Feat: alertaFeat}), nil)
		}(),
		"feat: no increase but raises":     at(feats, toren, fighter(LevelUpChoices{Feat: "feat:forte" + tableSuffix, AbilityIncrease: map[Ability]int{STR: 1}}), nil),
		"feat: choose, ok":                 at(fixed, toren, fighter(LevelUpChoices{Feat: "feat:meio" + tableSuffix, AbilityIncrease: map[Ability]int{DEX: 1}}), nil),
		"feat: choose, none":               at(fixed, toren, fighter(LevelUpChoices{Feat: "feat:meio" + tableSuffix}), nil),
		"feat: choose, unlisted":           at(fixed, toren, fighter(LevelUpChoices{Feat: "feat:meio" + tableSuffix, AbilityIncrease: map[Ability]int{CON: 1}}), nil),
		"feat: choose, two":                at(fixed, toren, fighter(LevelUpChoices{Feat: "feat:meio" + tableSuffix, AbilityIncrease: map[Ability]int{STR: 1, DEX: 1}}), nil),
		"feat: choose, by 2":               at(fixed, toren, fighter(LevelUpChoices{Feat: "feat:meio" + tableSuffix, AbilityIncrease: map[Ability]int{STR: 2}}), nil),
		"feat: choose, past 20":            at(fixed, edit(toren, func(b *Build) { b.BaseScores[STR] = 19 }), fighter(LevelUpChoices{Feat: "feat:meio" + tableSuffix, AbilityIncrease: map[Ability]int{STR: 1}}), nil),
		"feat: choose, both at 20":         at(fixed, edit(toren, func(b *Build) { b.BaseScores[STR], b.BaseScores[DEX] = 19, 19 }), fighter(LevelUpChoices{Feat: "feat:meio" + tableSuffix}), nil),
		"feat: fixed, ok":                  at(fixed, toren, fighter(LevelUpChoices{Feat: "feat:robusto" + tableSuffix}), nil),
		"feat: fixed, at 20":               at(fixed, edit(toren, func(b *Build) { b.BaseScores[STR] = 19 }), fighter(LevelUpChoices{Feat: "feat:robusto" + tableSuffix}), nil),
		"feat: fixed, wrong value":         at(fixed, toren, fighter(LevelUpChoices{Feat: "feat:robusto" + tableSuffix}), func(b *Build) { b.ExtraAbilityBonuses[CON] = 2 }),
		"feat: fixed, missing":             at(fixed, toren, fighter(LevelUpChoices{Feat: "feat:robusto" + tableSuffix}), func(b *Build) { delete(b.ExtraAbilityBonuses, CON) }),
		"feat: fixed, not at an ASI level": at(fixed, toren4, fighter(LevelUpChoices{Feat: "feat:robusto" + tableSuffix}), nil),

		// Abilities.
		"abilities: not due":   at(srd, toren4, fighter(LevelUpChoices{AbilityIncrease: map[Ability]int{STR: 2}}), nil),
		"abilities: +1":        at(srd, toren, fighter(LevelUpChoices{AbilityIncrease: map[Ability]int{STR: 1}}), nil),
		"abilities: +3 in two": at(srd, toren, fighter(LevelUpChoices{AbilityIncrease: map[Ability]int{STR: 2, DEX: 1}}), nil),
		"abilities: +3 in one": at(srd, toren, fighter(LevelUpChoices{AbilityIncrease: map[Ability]int{STR: 3}}), nil),
		"abilities: lowers":    at(srd, toren, plain, func(b *Build) { b.ExtraAbilityBonuses = map[Ability]int{STR: -1, DEX: 2} }),
		"abilities: unknown":   at(srd, toren, asi, func(b *Build) { b.ExtraAbilityBonuses[Ability("nope")] = 0 }),
		"abilities: past 20":   at(srd, edit(toren, func(b *Build) { b.BaseScores[STR] = 19 }), asi, nil),
		"abilities: past 20 in the second": at(srd, edit(toren, func(b *Build) { b.BaseScores[DEX] = 19 }),
			fighter(LevelUpChoices{AbilityIncrease: map[Ability]int{STR: 1, DEX: 1}}), nil),

		// Hit points.
		"hit points: rolls of a fixed sheet": at(srd, toren, asi, func(b *Build) { b.HitPoints.Rolls = []int{5} }),
		"hit points: back to fixed":          at(srd, rolled, asi, func(b *Build) { b.HitPoints = HitPoints{} }),
		"hit points: an unknown method":      at(srd, rolled, asi, func(b *Build) { b.HitPoints.Method = HitPointsRolled + 1 }),
		"hit points: two new rolls":          at(srd, rolled, asi, func(b *Build) { b.HitPoints.Rolls = append(b.HitPoints.Rolls, 4) }),
		"hit points: an earlier roll":        at(srd, rolled, asi, func(b *Build) { b.HitPoints.Rolls[0] = 10 }),
		"hit points: zero":                   at(srd, toren, LevelUpChoices{Class: "class:fighter", AbilityIncrease: map[Ability]int{STR: 2}, HitPoints: LevelUpHitPoints{Roll: 0}}, nil),
		"hit points: above the die":          at(srd, toren, asi, func(b *Build) { b.HitPoints = HitPoints{Method: HitPointsRolled, Rolls: []int{6, 6, 11}} }),

		// The subclass.
		"subclass: custom":           at(srd, toren, asi, func(b *Build) { b.Classes[0].CustomSubclassName = "Meu" }),
		"subclass: not due":          at(srd, toren, asi, func(b *Build) { b.Classes[0].Subclass = "subclass:life" }),
		"subclass: due, missing":     levelUpSubclassCase(t, srd, ""),
		"subclass: due, unknown":     levelUpSubclassCase(t, srd, "subclass:nada"),
		"subclass: due, wrong class": levelUpSubclassCase(t, srd, "subclass:champion"),
		"subclass: due, ok":          levelUpSubclassCase(t, srd, "subclass:lore"),

		// Duplicates.
		"duplicates: a spell twice":    at(srd, pensantus(), withWizard(func(ch *LevelUpChoices) { ch.Spells = []string{"spell:blur", "spell:blur"} }), nil),
		"duplicates: a cantrip it has": at(srd, pensantus(), withWizard(func(ch *LevelUpChoices) { ch.Cantrips = []string{"spell:fire-bolt"} }), nil),
		"duplicates: a prepared twice": at(srd, pensantus(), withWizard(func(ch *LevelUpChoices) { ch.Prepared = []string{"spell:blur", "spell:blur"} }), nil),
		"duplicates: an option twice":  at(srd, toren, asi, func(b *Build) { b.FeatureChoices = append(b.FeatureChoices, b.FeatureChoices[0]) }),
		"duplicates: a skill twice":    at(srd, toren, asi, func(b *Build) { b.SkillProficiencies = append(b.SkillProficiencies, "skill:arcana", "skill:arcana") }),
		"duplicates: expertise twice":  at(srd, toren, asi, func(b *Build) { b.Expertise = []string{"skill:perception", "skill:perception"} }),

		// Spells.
		"spells: a cantrip short":       at(srd, pensantus(), withWizard(func(ch *LevelUpChoices) { ch.Cantrips = nil }), nil),
		"spells: a cantrip removed":     at(srd, pensantus(), wizardBook, func(b *Build) { b.Cantrips = b.Cantrips[1:] }),
		"spells: a spell short":         at(srd, pensantus(), withWizard(func(ch *LevelUpChoices) { ch.Spells = ch.Spells[:1] }), nil),
		"spells: a spell removed":       at(srd, pensantus(), wizardBook, func(b *Build) { b.SpellsKnown = append(b.SpellsKnown[1:], "spell:blur") }),
		"spells: prepared by a fighter": at(srd, toren, fighter(LevelUpChoices{AbilityIncrease: map[Ability]int{STR: 2}, Prepared: []string{"spell:shield"}}), nil),
		"spells: a prepared removed":    at(srd, pensantus(), wizardBook, func(b *Build) { b.SpellsPrepared = b.SpellsPrepared[1:] }),
		"spells: a cleric spell":        at(srd, pensantus(), withWizard(func(ch *LevelUpChoices) { ch.Spells = []string{"spell:misty-step", "spell:aid"} }), nil),
		"spells: a 3rd-circle spell":    at(srd, pensantus(), withWizard(func(ch *LevelUpChoices) { ch.Spells = []string{"spell:misty-step", "spell:fireball"} }), nil),
		"spells: a cleric cantrip":      at(srd, pensantus(), withWizard(func(ch *LevelUpChoices) { ch.Cantrips = []string{"spell:guidance"} }), nil),
		"spells: too many prepared": at(srd, pensantus(), withWizard(func(ch *LevelUpChoices) {
			ch.Prepared = []string{"spell:misty-step", "spell:invisibility", "spell:find-familiar", "spell:detect-magic"}
		}), nil),
		"spells: magical secrets":      levelUpMagicalSecretsCase(t, srd, 2),
		"spells: magical secrets, one": levelUpMagicalSecretsCase(t, srd, 1),
		"spells: the patron's list": func() levelUpCheckCase {
			b := levelUpCharAt(t, srd, "class:warlock", "subclass:fiend", 2)
			ch := satisfy(t, srd, b, "class:warlock", "subclass:fiend")
			ch.Spells = []string{"spell:burning-hands"}
			return at(srd, b, ch, nil)
		}(),

		// The choice engine's picks.
		"choices: an option removed":        at(srd, toren, asi, func(b *Build) { b.FeatureChoices = nil }),
		"choices: an option from nowhere":   at(srd, toren, fighter(LevelUpChoices{AbilityIncrease: map[Ability]int{STR: 2}, FeatureChoices: []string{"feature:fighter-fighting-style-dueling"}}), nil),
		"choices: a text changed":           at(srd, edit(toren, func(b *Build) { b.FeatureChoiceText = map[string]string{"x": "a"} }), asi, func(b *Build) { b.FeatureChoiceText["x"] = "b" }),
		"choices: a text removed":           at(srd, edit(toren, func(b *Build) { b.FeatureChoiceText = map[string]string{"x": "a"} }), asi, func(b *Build) { b.FeatureChoiceText = nil }),
		"choices: a late choice still open": at(srd, edit(toren, func(b *Build) { b.FeatureChoices = nil }), asi, nil),
		"choices: a late choice made": at(srd, edit(toren, func(b *Build) { b.FeatureChoices = nil }),
			fighter(LevelUpChoices{AbilityIncrease: map[Ability]int{STR: 2}, LateChoices: []string{"feature:fighter-fighting-style-defense"}}), nil),
		"choices: a problem the sheet had": at(srd, edit(toren, func(b *Build) {
			b.FeatureChoices = append(b.FeatureChoices, "feature:fighter-fighting-style-dueling")
		}), asi, nil),
		"choices: a new option missing": func() levelUpCheckCase {
			b := levelUpCharAt(t, srd, "class:paladin", "subclass:devotion", 1)
			ch := satisfy(t, srd, b, "class:paladin", "subclass:devotion")
			ch.FeatureChoices = nil
			return at(srd, b, ch, nil)
		}(),
		"choices: an invocation swapped": levelUpSwapCase(t, srd, func(b Build) string {
			for _, k := range b.FeatureChoices {
				if srd.c.isInvocation(k) && len(firstOptions(srd, []string{k}, 1)) == 1 {
					return k
				}
			}
			return ""
		}),
		"choices: the first invocation swapped": levelUpSwapCase(t, srd, func(b Build) string { return levelUpFirstInvocation(srd, b) }),
		"choices: a swap for an invocation that gives skills": func() levelUpCheckCase {
			b := levelUpCharAt(t, srd, "class:warlock", "subclass:fiend", 4)
			ch := satisfy(t, srd, b, "class:warlock", "subclass:fiend")
			ch.SwapInvocation = levelUpFirstInvocation(srd, b)
			ch.FeatureChoices = append(ch.FeatureChoices, "feature:eldritch-invocation-beguiling-influence")
			return at(srd, b, ch, nil)
		}(),
		"choices: a pact boon swapped": levelUpSwapCase(t, srd, func(b Build) string {
			for _, k := range b.FeatureChoices {
				if !srd.c.isInvocation(k) {
					return k
				}
			}
			return ""
		}),
		"choices: two invocations removed": func() levelUpCheckCase {
			b := levelUpCharAt(t, srd, "class:warlock", "subclass:fiend", 4)
			ch := satisfy(t, srd, b, "class:warlock", "subclass:fiend")
			return at(srd, b, ch, func(a *Build) {
				n := 0
				a.FeatureChoices = slices.DeleteFunc(a.FeatureChoices, func(k string) bool {
					if srd.c.isInvocation(k) && n < 2 {
						n++
						return true
					}
					return false
				})
			})
		}(),
		"choices: an unmet prerequisite": func() levelUpCheckCase {
			b := levelUpCharAt(t, srd, "class:warlock", "subclass:fiend", 1)
			ch := satisfy(t, srd, b, "class:warlock", "subclass:fiend")
			ch.FeatureChoices = []string{"feature:eldritch-invocation-thirsting-blade", "feature:eldritch-invocation-beast-speech"}
			return at(srd, b, ch, nil)
		}(),

		// Skills and expertise.
		"skills: one for a fighter":       at(srd, toren, fighter(LevelUpChoices{AbilityIncrease: map[Ability]int{STR: 2}, SkillProficiencies: []string{"skill:arcana"}}), nil),
		"skills: one removed":             at(srd, toren, asi, func(b *Build) { b.SkillProficiencies = b.SkillProficiencies[:1] }),
		"skills: expertise for a fighter": at(srd, toren, fighter(LevelUpChoices{AbilityIncrease: map[Ability]int{STR: 2}, Expertise: []string{"skill:perception"}}), nil),
		"skills: expertise removed": func() levelUpCheckCase {
			b := levelUpCharAt(t, srd, "class:rogue", "subclass:thief", 2)
			return at(srd, b, satisfy(t, srd, b, "class:rogue", "subclass:thief"), func(a *Build) { a.Expertise = a.Expertise[1:] })
		}(),
		"skills: expertise missing": func() levelUpCheckCase {
			b := levelUpCharAt(t, srd, "class:rogue", "subclass:thief", 5)
			ch := satisfy(t, srd, b, "class:rogue", "subclass:thief")
			ch.Expertise = nil
			return at(srd, b, ch, nil)
		}(),
	}
	return cases
}

// levelUpSubclassCase is a Bard 2 going to 3, the level its subclass is due,
// with this subclass.
func levelUpSubclassCase(t *testing.T, c *Content, sub string) levelUpCheckCase {
	t.Helper()
	b := levelUpCharAt(t, c, "class:bard", "subclass:lore", 2)
	ch := satisfy(t, c, b, "class:bard", "subclass:lore")
	ch.Subclass = sub
	return levelUpCheckCase{c: c, before: b, after: mustApply(t, c, b, ch)}
}

// levelUpMagicalSecretsCase is a Bard 9 going to 10 with offList of its new spells
// from other classes' lists.
func levelUpMagicalSecretsCase(t *testing.T, c *Content, offList int) levelUpCheckCase {
	t.Helper()
	b := levelUpCharAt(t, c, "class:bard", "subclass:lore", 9)
	ch := satisfy(t, c, b, "class:bard", "subclass:lore")
	var other []string
	for _, key := range spellsOff(c, 5, "class:bard") {
		if !slices.Contains(b.SpellsKnown, key) && len(other) < offList {
			other = append(other, key)
		}
	}
	ch.Spells = slices.Concat(other, ch.Spells)[:len(ch.Spells)]
	return levelUpCheckCase{c: c, before: b, after: mustApply(t, c, b, ch)}
}

// levelUpSwapCase is a Warlock 4 going to 5 that gives up the option gone picks,
// for a new invocation.
func levelUpSwapCase(t *testing.T, c *Content, gone func(b Build) string) levelUpCheckCase {
	t.Helper()
	b := levelUpCharAt(t, c, "class:warlock", "subclass:fiend", 4)
	ch := satisfy(t, c, b, "class:warlock", "subclass:fiend")
	ch.SwapInvocation = gone(b)
	ch.FeatureChoices = append(ch.FeatureChoices, levelUpFreeInvocation(t, c, b, ch.FeatureChoices))
	return levelUpCheckCase{c: c, before: b, after: mustApply(t, c, b, ch)}
}

// levelUpFreeInvocation is the first invocation the offer lets b take that the
// picks do not already hold.
func levelUpFreeInvocation(t *testing.T, c *Content, b Build, picks []string) string {
	t.Helper()
	o, err := LevelUpOptions(b, "class:warlock", c)
	if err != nil {
		t.Fatal(err)
	}
	for _, fc := range o.FeatureChoices {
		for _, opt := range fc.Options {
			if fc.Feature.Key == invocationsFeature && !slices.Contains(picks, opt.Key) && len(firstOptions(c, []string{opt.Key}, 1)) == 1 {
				return opt.Key
			}
		}
	}
	t.Fatal("no free invocation")
	return ""
}

// TestCheckLevelUpCharacterization freezes every refusal of CheckLevelUp, with all
// its fields.
func TestCheckLevelUpCharacterization(t *testing.T) {
	t.Parallel()
	got := map[string]string{}
	for name, tc := range levelUpCheckCases(t) {
		got[name] = levelUpRefusalText(CheckLevelUp(tc.before, tc.after, tc.c))
	}
	goldenText(t, "levelup-check-refusals", got)
}

// TestMulticlassSummaryCharacterization freezes the summary of a multiclass level
// up: the caster levels, the proficiencies gained and the three exceptions.
func TestMulticlassSummaryCharacterization(t *testing.T) {
	t.Parallel()
	c := loadForTest(t)
	with := func(classes ...ClassLevel) Build {
		b := slotsBuild(classes...)
		b.BaseScores = map[Ability]int{STR: 14, DEX: 14, CON: 16, INT: 14, WIS: 18, CHA: 14}
		return b
	}
	fighter := func(n int) ClassLevel { return ClassLevel{Class: "class:fighter", Level: n} }
	paladin := func(n int) ClassLevel { return ClassLevel{Class: "class:paladin", Level: n} }
	cleric := func(n int) ClassLevel { return ClassLevel{Class: "class:cleric", Subclass: "subclass:life", Level: n} }
	barbarian, monk := ClassLevel{Class: "class:barbarian", Level: 1}, ClassLevel{Class: "class:monk", Level: 1}
	bard := mustApply(t, c, richFighter5(), LevelUpChoices{Class: "class:bard", Instrument: "proficiency:lute", SkillProficiencies: []string{"skill:arcana"}, HitPoints: LevelUpHitPoints{Average: true}})
	cases := []struct {
		name          string
		before, after Build
		newClass      string
	}{
		{"single class", with(fighter(4)), with(fighter(5)), ""},
		{"Extra Attack does not add", with(fighter(5), paladin(4)), with(fighter(5), paladin(5)), ""},
		{"Extra Attack adds", with(fighter(4), paladin(4)), with(fighter(5), paladin(4)), ""},
		{"Channel Divinity gives no use", with(cleric(6), paladin(2)), with(cleric(6), paladin(3)), ""},
		{"Channel Divinity's first use", with(cleric(1), paladin(2)), with(cleric(1), paladin(3)), ""},
		{"Unarmored Defense twice", with(barbarian), with(barbarian, monk), "class:monk"},
		{"two casters", with(cleric(3), paladin(2)), with(cleric(3), paladin(3)), ""},
		{"a new class's proficiencies", with(fighter(5)), with(fighter(5), ClassLevel{Class: "class:rogue", Level: 1}), "class:rogue"},
		{"the bard's instrument and skill", richFighter5(), bard, "class:bard"},
	}
	out := map[string]any{}
	for _, tc := range cases {
		s, ok := MulticlassSummaryOf(tc.before, tc.after, tc.newClass, c)
		out[tc.name] = map[string]any{"summary": s, "ok": ok}
	}
	levelUpGoldenJSON(t, "levelup-multiclass-summary", out)
}

// TestSceneOptionsCharacterization freezes what a player sees of a scene: every
// skill, ability and save of a Rogue with Reliable Talent and of a Bard with Jack
// of All Trades, and the first unknown key.
func TestSceneOptionsCharacterization(t *testing.T) {
	t.Parallel()
	c := loadForTest(t)
	cat := c.Catalog()
	var all []SceneAction
	for _, s := range cat.Skills {
		all = append(all, SceneAction{Key: s.Key, DC: 10})
	}
	for _, a := range cat.Abilities {
		all = append(all,
			SceneAction{Key: SceneAbility + ":" + string(a.Ability), DC: 12},
			SceneAction{Key: SceneSave + ":" + string(a.Ability), DC: 14})
	}
	rogue := levelUpCharAt(t, c, "class:rogue", "subclass:thief", 11)
	bardB := levelUpCharAt(t, c, "class:bard", "subclass:lore", 2)
	out := map[string]any{}
	for name, b := range map[string]Build{"rogue 11": rogue, "bard 2": bardB, "fighter 3": torenLevelUp()} {
		got, err := SceneOptions(Derive(b, c), all)
		out[name] = map[string]any{"options": got, "error": errText(err)}
	}
	for name, actions := range map[string][]SceneAction{
		"unknown kind":    {all[0], {Key: "nada:x"}},
		"unknown skill":   {{Key: "skill:nada"}},
		"unknown ability": {{Key: SceneAbility + ":nope"}},
		"unknown save":    {all[0], all[1], {Key: SceneSave + ":nope"}},
	} {
		got, err := SceneOptions(Derive(torenLevelUp(), c), actions)
		out[name] = map[string]any{"options": got, "error": errText(err)}
	}
	levelUpGoldenJSON(t, "scene-options", out)
}
