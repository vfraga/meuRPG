package rules

import (
	"errors"
	"slices"
	"strings"
	"testing"
)

// The summon of a table class feature (#578): the entry is a feature of an SRD class that brings the table's creatures
// to every character that has it, through the same hidden spell as a feature's summon.

const summonerFeatureKey = "classfeature:chamado-da-matilha" + tableSuffix

func classFeatureSummonOverlay(edit func(*TableClassFeature)) Overlay {
	cf := TableClassFeature{
		TableEntry: TableEntry{Key: summonerFeatureKey, NamePT: "Chamado da Matilha"}, DescPT: []string{"O druida chama a matilha."},
		Class: "class:druid", Level: 2,
		Effects: []Effect{{Type: "resource", Resource: packResource, Max: "2", Recharge: RechargeLongRest}},
		Summon: &TableSummon{
			CreatureKeys: []string{packLeader, wolfKey}, Count: "1 + floor(prof() / 2)", MaxLive: 6,
			Replaces: SummonReplacesSame, ResourceKey: packResource,
		},
	}
	if edit != nil {
		edit(&cf)
	}
	return Overlay{Revision: 1, Creatures: []TableCreature{packLeaderCreature()}, ClassFeatures: []TableClassFeature{cf}}
}

func TestATableClassFeatureSummonsThroughTheSpellChecks(t *testing.T) {
	t.Parallel()
	c := withOverlayOn(t, loadForTest(t), classFeatureSummonOverlay(nil))
	spell := FeatureSummonSpellKey(summonerFeatureKey)
	rule, ok := c.SummonRule(spell)
	if !ok || rule.Feature != summonerFeatureKey || rule.Resource != packResource || rule.MaxLive != 6 {
		t.Fatalf("SummonRule = %+v, %v; want the class feature, its resource and its rule", rule, ok)
	}
	if slices.ContainsFunc(c.ListSpells(SpellFilter{}), func(e SpellEntry) bool { return e.Key == spell }) {
		t.Error("ListSpells lists the class feature's hidden spell")
	}
	b := Build{
		BaseScores: standard("class:x", 1).BaseScores, Race: "race:human", Background: "background:acolyte", Languages: testLanguages,
		Classes: []ClassLevel{{Class: "class:druid", Level: 2}},
	}
	got, err := c.SummonOptions(spell, featureSummonLevel, b)
	if err != nil || len(got.Options) != 1 || got.Options[0].Count != 2 || len(got.Options[0].Forms) != 2 {
		t.Fatalf("SummonOptions = %+v, %v; want one option of 2 creatures and two forms", got, err)
	}
	if _, err := c.CheckSummon(spell, featureSummonLevel, b, SummonPick{Creatures: []string{packLeader, wolfKey}}); err != nil {
		t.Errorf("CheckSummon = %v, want the pick accepted", err)
	}
	d := Derive(b, c)
	if !slices.ContainsFunc(d.GrantedSpells, func(g GrantedSpell) bool {
		return g.Spell.Key == spell && g.Source == summonerFeatureKey && g.Resource == packResource && g.FeatureSummon
	}) {
		t.Errorf("GrantedSpells = %+v, want the class feature's spell, spending the resource", d.GrantedSpells)
	}
	// At level 1 the feature has not come yet.
	b.Classes = []ClassLevel{{Class: "class:druid", Level: 1}}
	if d := Derive(b, c); slices.ContainsFunc(d.GrantedSpells, func(g GrantedSpell) bool { return g.Spell.Key == spell }) {
		t.Error("a level 1 druid is granted the level 2 class feature's spell")
	}
}

func TestATableClassFeatureSummonIsRefusedAtTheFieldThatIsWrong(t *testing.T) {
	t.Parallel()
	for _, tc := range []struct {
		name  string
		edit  func(*TableClassFeature)
		field string
		more  string
	}{
		{"a resource nobody defines", func(f *TableClassFeature) { f.Summon.ResourceKey = "pontos" }, ".summon.resource_key", ReasonReference},
		{"no resource", func(f *TableClassFeature) { f.Summon.ResourceKey = "" }, ".summon.resource_key", ReasonValue},
		{"a count that does not compile", func(f *TableClassFeature) { f.Summon.Count = "1 +" }, ".summon.count", ReasonFormula},
		{"a creature that does not exist", func(f *TableClassFeature) { f.Summon.CreatureKeys = []string{"creature:fantasma@mesa"} }, ".summon.creature_keys[0]", ReasonReference},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			_, err := loadForTest(t).With(classFeatureSummonOverlay(tc.edit))
			var oe *OverlayError
			if !errors.As(err, &oe) {
				t.Fatalf("With() error = %v, want a refusal", err)
			}
			for _, v := range oe.Violations() {
				if strings.HasSuffix(v.Field, tc.field) && v.Reason == tc.more && strings.HasPrefix(v.Field, "class_features[0]") {
					return
				}
			}
			t.Errorf("no %s violation at class_features[0]%s: %v", tc.more, tc.field, err)
		})
	}
}

func grantsClassFeatureSummon(c *Content, class string, level int) bool {
	b := Build{
		BaseScores: standard("class:x", 1).BaseScores, Race: "race:human", Background: "background:acolyte", Languages: testLanguages,
		Classes: []ClassLevel{{Class: class, Level: level}},
	}
	spell := FeatureSummonSpellKey(summonerFeatureKey)
	return slices.ContainsFunc(Derive(b, c).GrantedSpells, func(g GrantedSpell) bool { return g.Spell.Key == spell })
}

func TestAClassFeatureSummonFollowsTheSwitchesOfItsEntry(t *testing.T) {
	t.Parallel()
	o := classFeatureSummonOverlay(nil)
	if !grantsClassFeatureSummon(withOverlayOn(t, loadForTest(t), o), "class:druid", 2) {
		t.Fatal("the live entry grants nothing (the control)")
	}
	// Archived is not off: the sheets that have the class keep the summon.
	o.ClassFeatures[0].Archived = true
	if !grantsClassFeatureSummon(withOverlayOn(t, loadForTest(t), o), "class:druid", 2) {
		t.Error("an archived class feature stopped granting its summon")
	}
	// Off: the class reads as it did without the entry.
	o.Off = []string{summonerFeatureKey}
	if grantsClassFeatureSummon(withOverlayOn(t, loadForTest(t), o), "class:druid", 2) {
		t.Error("a switched-off class feature still grants its summon")
	}
}

func TestATableClassCanHostAClassFeatureSummon(t *testing.T) {
	t.Parallel()
	tc := classFromDefaults(DefaultTable{Kind: CastingNone, Rows: loadDefaultRows()}, "cavaleiro")
	o := classFeatureSummonOverlay(func(f *TableClassFeature) { f.Class = tc.Key })
	o.Classes = []TableClass{tc}
	c := withOverlayOn(t, loadForTest(t), o)
	if !grantsClassFeatureSummon(c, tc.Key, 2) {
		t.Error("a table class does not grant the class feature's summon")
	}
	if grantsClassFeatureSummon(c, tc.Key, 1) {
		t.Error("a level 1 character is granted the level 2 summon")
	}
}

func TestAReplacedClassFeatureSummonGrantsNothing(t *testing.T) {
	t.Parallel()
	o := classFeatureSummonOverlay(nil)
	o.ClassFeatures = append(o.ClassFeatures, TableClassFeature{
		TableEntry: TableEntry{Key: "classfeature:matilha-maior" + tableSuffix, NamePT: "Matilha Maior"},
		Class:      "class:druid", Level: 2, Replaces: summonerFeatureKey,
	})
	c := withOverlayOn(t, loadForTest(t), o)
	if grantsClassFeatureSummon(c, "class:druid", 2) {
		t.Error("the replaced class feature still grants its summon")
	}
}
