package rules

import (
	"encoding/json"
	"slices"
	"testing"
)

// collectDump runs the deriver up to collectEffects, as deriveWith does, and
// writes what the step leaves: the active effects (by owner, type and place in
// the owner's list), the features without their texts, and the issues.
func collectDump(b Build, c *content, inactive map[string]bool) (string, error) {
	x := &deriver{b: b, c: c, d: &Derived{ContentVersion: c.version}, proficient: map[string]bool{}, conditions: map[*Effect]bool{}, appliedTagged: map[*Effect]bool{}, inactiveFeats: inactive}
	x.resolve()
	x.resolveArmor()
	x.names()
	x.abilities()
	x.levelAndProficiency()
	x.collectEffects()

	type active struct {
		Owner, Type string
		Index       int
	}
	type feature struct {
		Key, Name, NamePT, Source, SourcePT, SummaryPT string
		Level, Paragraphs                              int
		PTMissing, PTOnly                              bool
	}
	var out struct {
		Active   []active
		Features []feature
		Issues   []Issue
	}
	for _, a := range x.active {
		out.Active = append(out.Active, active{a.owner, a.effect.Type, slices.Index(c.effects[a.owner], a.effect)})
	}
	for _, f := range x.d.Features {
		out.Features = append(out.Features, feature{f.Key, f.Name, f.NamePT, f.Source, f.SourcePT, f.SummaryPT, f.Level, len(f.Description), f.DescriptionPTMissing, f.DescriptionPTOnly})
	}
	out.Issues = x.d.Issues
	raw, err := json.Marshal(out)
	return string(raw), err
}

type collectCase struct {
	build    func() Build
	inactive map[string]bool
}

func collectCases() map[string]collectCase {
	with := func(edit func(b *Build)) func() Build {
		return func() Build {
			b := pensantus()
			edit(&b)
			return b
		}
	}
	// A fighter 6 / paladin 2 who took the Defense style from both classes.
	fighterPaladin := func(edit func(b *Build)) func() Build {
		return func() Build {
			b := Build{
				BaseScores: map[Ability]int{STR: 15, DEX: 14, CON: 13, INT: 12, WIS: 10, CHA: 14},
				Race:       "race:human", Background: "background:acolyte",
				Classes: []ClassLevel{{Class: "class:fighter", Subclass: "subclass:champion", Level: 6}, {Class: "class:paladin", Level: 2}},
			}
			edit(&b)
			return b
		}
	}
	const (
		fighterDefense = "feature:fighter-fighting-style-defense"
		paladinDefense = "feature:fighting-style-defense"
		fighterASI     = "feature:fighter-ability-score-improvement-1"
	)
	return map[string]collectCase{
		"pensantus":            {build: pensantus},
		"unknown race":         {build: with(func(b *Build) { b.Race = "race:nope"; b.Subrace = "" })},
		"race without subrace": {build: with(func(b *Build) { b.Subrace = "" })},
		"unknown subrace":      {build: with(func(b *Build) { b.Subrace = "subrace:nope" })},
		"no classes":           {build: with(func(b *Build) { b.Classes = nil })},
		"unknown class":        {build: with(func(b *Build) { b.Classes = append(b.Classes, ClassLevel{Class: "class:nope", Level: 1}) })},
		"class twice": {build: with(func(b *Build) {
			b.Classes = append(b.Classes, ClassLevel{Class: "class:wizard", Level: 1})
		})},
		"subclass of another class": {build: with(func(b *Build) { b.Classes[0].Subclass = "subclass:champion" })},
		"level 1 without the subclass": {build: with(func(b *Build) {
			b.Classes[0] = ClassLevel{Class: "class:wizard", Subclass: "subclass:evocation", Level: 1}
		})},
		"level 20 wizard": {build: with(func(b *Build) { b.Classes[0].Level = 20 })},
		"known background": {build: with(func(b *Build) {
			b.Background = "background:acolyte"
			b.dropCustomBackground()
		})},
		"unknown background": {build: with(func(b *Build) { b.Background = "background:nope" })},
		"custom background, name only": {build: with(func(b *Build) {
			b.CustomBackgroundFeature = ""
		})},
		"custom background, text only": {build: with(func(b *Build) {
			b.CustomBackgroundFeatureName = ""
		})},
		"custom background, no feature": {build: with(func(b *Build) {
			b.CustomBackgroundFeatureName, b.CustomBackgroundFeature = "", ""
		})},
		"no background": {build: with(func(b *Build) { b.dropCustomBackground() })},
		"a feat in place of a class improvement": {build: fighterPaladin(func(b *Build) {
			b.Feats = []string{"feat:grappler"}
			b.FeatSlots = map[string]string{"feat:grappler": fighterASI}
		})},
		"a feat in place of an unknown slot": {build: fighterPaladin(func(b *Build) {
			b.Feats = []string{"feat:grappler"}
			b.FeatSlots = map[string]string{"feat:grappler": "feature:nope"}
		})},
		"a slot whose feat was not taken": {build: fighterPaladin(func(b *Build) {
			b.FeatSlots = map[string]string{"feat:grappler": fighterASI}
		})},
		"a feat without a slot": {build: fighterPaladin(func(b *Build) { b.Feats = []string{"feat:grappler"} })},
		"an unknown feat":       {build: fighterPaladin(func(b *Build) { b.Feats = []string{"feat:nope", "feat:grappler"} })},
		"an inactive feat": {
			build:    fighterPaladin(func(b *Build) { b.Feats = []string{"feat:grappler"} }),
			inactive: map[string]bool{"feat:grappler": true},
		},
		"the Defense style of the fighter": {build: fighterPaladin(func(b *Build) { b.FeatureChoices = []string{fighterDefense} })},
		"the Defense style twice by name": {build: fighterPaladin(func(b *Build) {
			b.FeatureChoices = []string{fighterDefense, paladinDefense}
		})},
		"the paladin's style alone": {build: fighterPaladin(func(b *Build) { b.FeatureChoices = []string{paladinDefense} })},
		"an option nothing offers":  {build: with(func(b *Build) { b.FeatureChoices = []string{fighterDefense} })},
		"an unknown option":         {build: fighterPaladin(func(b *Build) { b.FeatureChoices = []string{"feature:nope"} })},
		"a trait option": {build: buildFighter3(func(b *Build) {
			b.Race, b.Subrace = "race:dragonborn", ""
			b.FeatureChoices = []string{"trait:draconic-ancestry-red"}
		})},
		"a draconic sorcerer's ancestor": {build: buildFighter3(func(b *Build) {
			b.Classes = []ClassLevel{{Class: "class:sorcerer", Subclass: "subclass:draconic", Level: 1}}
			b.FeatureChoices = []string{"feature:dragon-ancestor-red---fire-damage"}
		})},
		"a feat taken twice":            {build: fighterPaladin(func(b *Build) { b.Feats = []string{"feat:grappler", "feat:grappler"} })},
		"a trait option nothing offers": {build: with(func(b *Build) { b.FeatureChoices = []string{"trait:draconic-ancestry-red"} })},
		"an unknown trait":              {build: with(func(b *Build) { b.FeatureChoices = []string{"trait:nope"} })},
		"a trait that is no option":     {build: with(func(b *Build) { b.FeatureChoices = []string{"trait:darkvision"} })},
		"a key of no kind":              {build: with(func(b *Build) { b.FeatureChoices = []string{"garbage"} })},
		"a scoped pick of the choice engine": {build: with(func(b *Build) {
			b.FeatureChoices = []string{ScopedChoice("trait:darkvision", "language:elvish"), ScopedChoice("feature:nope", "language:elvish"), "skill:arcana"}
		})},
		"options in a row": {build: fighterPaladin(func(b *Build) {
			b.FeatureChoices = []string{"feature:nope", fighterDefense, "garbage", paladinDefense, "trait:nope"}
		})},
	}
}

// buildFighter3 is a base build for the cases that are not Pensantus.
func buildFighter3(edit func(b *Build)) func() Build {
	return func() Build {
		b := Build{
			BaseScores: map[Ability]int{STR: 15, DEX: 14, CON: 13, INT: 12, WIS: 10, CHA: 8},
			Race:       "race:human", Background: "background:acolyte",
			Classes: []ClassLevel{{Class: "class:fighter", Level: 3}},
		}
		edit(&b)
		return b
	}
}

// TestCollectEffectsCharacterization freezes what collectEffects leaves on the
// deriver: the effects in order, the features it lists with their sources and
// the issues, over the SRD content and over a table content that offers options.
func TestCollectEffectsCharacterization(t *testing.T) {
	t.Parallel()
	srd := loadForTest(t)
	got := map[string]string{}
	for name, tc := range collectCases() {
		text, err := collectDump(tc.build(), srd.c, tc.inactive)
		if err != nil {
			t.Fatal(err)
		}
		got["srd: "+name] = text
	}

	// A table content whose class offers a fighting style: the sheet keeps the
	// pick while it is offered, and an issue tied to the entry when it is not.
	const none = "class:gen-none@mesa"
	const sub = "subclass:gen-none-a@mesa"
	o1 := stamped(t, srd)
	c1 := withOverlayOn(t, srd, o1)
	b := sweepBase(t, c1, none, sub)
	b = sweepUp(t, c1, b, none, sub, 6)
	if len(b.FeatureChoices) == 0 {
		t.Fatal("the sheet made no feature choice")
	}
	chosen := b.FeatureChoices[0]
	other := "feature:fighter-fighting-style-dueling"
	if chosen == other {
		other = "feature:fighter-fighting-style-defense"
	}
	o2 := stamped(t, srd)
	classOf(&o2, none).Levels[5].Features[0].Effects[0].From = []string{other}
	c2 := withOverlayOn(t, srd, o2)
	o3 := stamped(t, srd)
	classOf(&o3, none).Levels[5].Features = nil
	c3 := withOverlayOn(t, srd, o3)
	srdOption := b
	srdOption.FeatureChoices = append(slices.Clone(b.FeatureChoices), "feature:fighter-fighting-style-defense")
	for name, tc := range map[string]struct {
		b Build
		c *Content
	}{
		"table: as made":                  {b, c1},
		"table: the style is not offered": {b, c2},
		"table: the feature is gone":      {b, c3},
		"table: an SRD style of no one":   {srdOption, c1},
		"table: base level 1":             {sweepBase(t, c1, none, sub), c1},
	} {
		text, err := collectDump(tc.b, tc.c.c, nil)
		if err != nil {
			t.Fatal(err)
		}
		got[name] = text
	}
	goldenText(t, "collecteffects", got)
}
