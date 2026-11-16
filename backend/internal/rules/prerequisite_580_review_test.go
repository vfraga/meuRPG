package rules

import (
	"errors"
	"fmt"
	"slices"
	"strings"
	"testing"
)

// #580 S1, the review round: one meaning for "knows a spell" and for a class level on the feat path and the option
// path, option prerequisites judged once every entry is built, and cycles refused.

const (
	lightFeat  = "feat:pede-luz" + tableSuffix
	blessFeat  = "feat:pede-bencao" + tableSuffix
	blessOpt   = postureHeron
	lightOpt   = postureBull
	eldritch   = "spell:eldritch-blast"
	blessSpell = "spell:bless"
	lightSpell = "spell:light"
)

// knowsOverlay joins the posture subclass (its level 3 always-prepared Bless), a race whose trait grants Light as a
// cantrip that counts as known, and two feats that ask for those spells. Heron asks for Bless and Bull for Light.
func knowsOverlay(countsAsKnown bool) Overlay {
	o := graceOverlay(nil, []Effect{grant(lightSpell, "int", func(e *Effect) { e.CountsAsKnown = countsAsKnown })})
	p := postureOverlay()
	p.Subclasses[0].Class = "class:cleric"
	p.Subclasses[0].AlwaysPrepared = []TableAlwaysPrepared{{ClassLevel: 5, Spell: blessSpell}}
	opts := p.Subclasses[0].Levels[0].Features[0].Options
	opts[0].Prerequisite = TableOptionPrerequisite{RequiresSpell: blessSpell}
	opts[1].Prerequisite = TableOptionPrerequisite{RequiresSpell: lightSpell}
	o.Subclasses = p.Subclasses
	o.Feats = []TableFeat{
		{TableEntry: TableEntry{Key: lightFeat, NamePT: "Pede Luz"}, DescPT: []string{"Texto."}, Prerequisite: FeatPrerequisite{RequiresSpell: lightSpell}},
		{TableEntry: TableEntry{Key: blessFeat, NamePT: "Pede Bênção"}, DescPT: []string{"Texto."}, Prerequisite: FeatPrerequisite{RequiresSpell: blessSpell}},
	}
	return o
}

func knowsBuild(level int, picks ...string) Build {
	b := postureBuild(level, picks...)
	b.Race = graceRace
	b.Classes[0].Class = "class:cleric"
	return b
}

// Both paths read "knows a spell" the same way: an always-prepared spell of the subclass and a cantrip a race grants as
// known count; a granted spell that does not count as known does not.
func TestKnowingASpellMeansTheSameForAFeatAndAnOption(t *testing.T) {
	t.Parallel()
	c := withOverlay(t, knowsOverlay(true))
	opts := optionChoiceOf(t, c.Choices(knowsBuild(5)), postureFeature)
	if optionOf(t, opts, blessOpt).Blocked() {
		t.Errorf("the always-prepared Bless does not satisfy the option: %+v", optionOf(t, opts, blessOpt))
	}
	if optionOf(t, opts, lightOpt).Blocked() {
		t.Errorf("the race's counts-as-known Light does not satisfy the option: %+v", optionOf(t, opts, lightOpt))
	}
	for _, f := range []string{blessFeat, lightFeat} {
		if got := unmetOf(t, c, knowsBuild(5), f); len(got) != 0 {
			t.Errorf("%s unmet = %+v, want none", f, got)
		}
	}
	// Level 3 has no always-prepared Bless: both paths say so.
	low := optionChoiceOf(t, c.Choices(knowsBuild(3)), postureFeature)
	if !optionOf(t, low, blessOpt).Blocked() {
		t.Error("Bless is not known at level 3, but the option is open")
	}
	if got := unmetOf(t, c, knowsBuild(3), blessFeat); len(got) != 1 || got[0].Kind != FeatUnmetSpell {
		t.Errorf("unmet at level 3 = %+v, want the spell", got)
	}

	n := withOverlay(t, knowsOverlay(false))
	not := optionChoiceOf(t, n.Choices(knowsBuild(5)), postureFeature)
	if !optionOf(t, not, lightOpt).Blocked() {
		t.Error("Light granted without counting as known satisfies the option")
	}
	if got := unmetOf(t, n, knowsBuild(5), lightFeat); len(got) != 1 {
		t.Errorf("a granted spell that is not known satisfies the feat: %+v", got)
	}
}

// A sheet that lists a cantrip in its own cantrips satisfies both paths.
func TestAListedCantripSatisfiesBothPaths(t *testing.T) {
	t.Parallel()
	c := withOverlay(t, knowsOverlay(false))
	b := knowsBuild(5)
	b.Cantrips = []string{lightSpell}
	if optionOf(t, optionChoiceOf(t, c.Choices(b), postureFeature), lightOpt).Blocked() {
		t.Error("a listed cantrip does not satisfy the option")
	}
	if got := unmetOf(t, c, b, lightFeat); len(got) != 0 {
		t.Errorf("a listed cantrip does not satisfy the feat: %+v", got)
	}
}

// With two classes the class level is the sum of the entries of that class, on both paths.
func TestAClassLevelIsTheSameInAMulticlassSheet(t *testing.T) {
	t.Parallel()
	o := postureOverlay()
	o.Subclasses[0].Levels[0].Features[0].Options[1].Prerequisite = TableOptionPrerequisite{ClassKey: "class:fighter", ClassLevel: 5}
	o.Feats = []TableFeat{{TableEntry: TableEntry{Key: lightFeat, NamePT: "Marcial"}, DescPT: []string{"Texto."}, Prerequisite: FeatPrerequisite{ClassKey: "class:fighter", ClassLevel: 5}}}
	c := withOverlay(t, o)
	split := func(a, b int) Build {
		bd := postureBuild(a)
		bd.Classes = []ClassLevel{{Class: "class:fighter", Level: a, Subclass: postureSubclass}, {Class: "class:fighter", Level: b}}
		return bd
	}
	for _, tc := range []struct {
		a, b int
		open bool
	}{{3, 2, true}, {3, 1, false}} {
		got := optionOf(t, optionChoiceOf(t, c.Choices(split(tc.a, tc.b)), postureFeature), postureBull)
		if got.Blocked() == tc.open {
			t.Errorf("%d+%d option blocked = %v, want %v", tc.a, tc.b, got.Blocked(), !tc.open)
		}
		if unmet := unmetOf(t, c, split(tc.a, tc.b), lightFeat); (len(unmet) == 0) != tc.open {
			t.Errorf("%d+%d feat unmet = %+v", tc.a, tc.b, unmet)
		}
	}
}

// An option may ask for an option of a feature built after it, or before it, whatever the order of the keys.
func TestAnOptionMayAskForAnOptionOfAnotherFeatureBuiltLater(t *testing.T) {
	t.Parallel()
	const (
		earlySub = "subclass:aa-primeiro" + tableSuffix
		lateSub  = "subclass:zz-ultimo" + tableSuffix
		earlyOpt = "feature:aa-opcao" + tableSuffix
		lateOpt  = "feature:zz-opcao" + tableSuffix
	)
	sub := func(key, feature string, opt TableFeature) TableSubclass {
		return TableSubclass{Key: key, NamePT: key, Class: "class:fighter", Levels: []TableSubclassLevel{{Level: 3, Features: []TableFeature{{
			Key: feature, NamePT: feature, DescPT: []string{"Texto."},
			Effects: []Effect{{Type: "choice", Choice: "feature", Count: 1}}, Options: []TableFeature{opt, postureOption(strings.TrimSuffix(feature, tableSuffix)+"-b"+tableSuffix, feature+" B")},
		}}}}}
	}
	early := postureOption(earlyOpt, "Cedo")
	early.Prerequisite = TableOptionPrerequisite{RequiresOptions: []string{lateOpt}}
	late := postureOption(lateOpt, "Tarde")
	late.Prerequisite = TableOptionPrerequisite{RequiresOptions: []string{"feature:fighting-style-defense"}}
	o := Overlay{Revision: 1, Subclasses: []TableSubclass{sub(earlySub, "feature:aa-feature"+tableSuffix, early), sub(lateSub, "feature:zz-feature"+tableSuffix, late)}}
	if _, err := loadForTest(t).With(o); err != nil {
		t.Errorf("an option asking for an option of a feature built after it was refused: %v", err)
	}
}

func TestAnOptionPrerequisiteCycleIsRefused(t *testing.T) {
	t.Parallel()
	o := postureOverlay()
	opts := o.Subclasses[0].Levels[0].Features[0].Options
	opts[0].Prerequisite = TableOptionPrerequisite{RequiresOptions: []string{postureBull}}
	opts[1].Prerequisite = TableOptionPrerequisite{RequiresOptions: []string{postureWolf}}
	opts[2].Prerequisite = TableOptionPrerequisite{RequiresOptions: []string{postureHeron}}
	_, err := loadForTest(t).With(o)
	var oe *OverlayError
	if !errors.As(err, &oe) {
		t.Fatalf("want *OverlayError, got %T: %v", err, err)
	}
	if !hasViolation(oe, "subclasses[0].levels[0].features[0].options[0].prerequisite.requires_option_keys[0]") {
		t.Errorf("no violation at the first option of the cycle: %v", err)
	}
}

func TestAFeatAndTableOptionsMakeNoCycleWhenOneWayOnly(t *testing.T) {
	t.Parallel()
	o := postureOverlay()
	opts := o.Subclasses[0].Levels[0].Features[0].Options
	opts[0].Prerequisite = TableOptionPrerequisite{RequiresOptions: []string{postureBull}}
	opts[1].Prerequisite = TableOptionPrerequisite{RequiresOptions: []string{postureWolf}}
	if _, err := loadForTest(t).With(o); err != nil {
		t.Errorf("a chain of options without a cycle was refused: %v", err)
	}
}

// The limits the prerequisite fields hold.
func TestPrerequisiteLimits(t *testing.T) {
	t.Parallel()
	opt0 := "subclasses[0].levels[0].features[0].options[0].prerequisite"
	cases := []struct {
		name  string
		p     TableOptionPrerequisite
		field string
	}{
		{"level 21", TableOptionPrerequisite{ClassKey: "class:fighter", ClassLevel: 21}, opt0 + ".class_level.level"},
		{"eleven option keys", TableOptionPrerequisite{RequiresOptions: slices.Repeat([]string{"feature:fighting-style-defense"}, 11)}, opt0 + ".requires_option_keys"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			o := postureOverlay()
			o.Subclasses[0].Levels[0].Features[0].Options[0].Prerequisite = tc.p
			_, err := loadForTest(t).With(o)
			var oe *OverlayError
			if !errors.As(err, &oe) || !hasViolation(oe, tc.field) {
				t.Errorf("want a violation at %s, got %v", tc.field, err)
			}
		})
	}
}

// An option of a trait asks nothing: the player picks one.
func TestAnOptionOfATraitWithAPrerequisiteIsRefused(t *testing.T) {
	t.Parallel()
	o := graceOverlay(nil, nil)
	o.Races[0].Traits[0].Effects = []Effect{{Type: "choice", Choice: "feature", Count: 1}}
	o.Races[0].Traits[0].Options = []TableFeature{
		{Key: "trait:um" + tableSuffix, NamePT: "Um", DescPT: []string{"Texto."}, Prerequisite: TableOptionPrerequisite{ClassKey: "class:fighter", ClassLevel: 2}},
		{Key: "trait:dois" + tableSuffix, NamePT: "Dois", DescPT: []string{"Texto."}},
	}
	_, err := loadForTest(t).With(o)
	var oe *OverlayError
	if !errors.As(err, &oe) || !hasViolation(oe, "races[0].traits[0].options[0].prerequisite") {
		t.Errorf("want a violation at the trait option's prerequisite, got %v", err)
	}
}

// A feat whose option the character holds passes; a pick keeps the problem after it loses a pact, a spell or an option.
func TestAHeldPickKeepsTheProblemWhenWhatItAskedIsLost(t *testing.T) {
	t.Parallel()
	o := knowsOverlay(true)
	opts := o.Subclasses[0].Levels[0].Features[0].Options
	opts[2].Prerequisite = TableOptionPrerequisite{RequiresOptions: []string{postureCrow}}
	opts[3].Prerequisite = TableOptionPrerequisite{RequiresPact: tome}
	o.Feats = append(o.Feats, TableFeat{TableEntry: TableEntry{Key: featKeyNeedsStyle, NamePT: "Estilo"}, DescPT: []string{"Texto."}, Prerequisite: FeatPrerequisite{RequiresOptions: []string{postureHeron}}})
	c := withOverlay(t, o)

	held := knowsBuild(5, postureHeron)
	if got := unmetOf(t, c, held, featKeyNeedsStyle); len(got) != 0 {
		t.Errorf("a feat whose option is held is unmet: %+v", got)
	}
	problem := func(b Build, key string) bool {
		return slices.ContainsFunc(c.Choices(b).ChoiceProblems(false), func(p ChoiceProblem) bool { return p.Code == ChoiceProblemPrerequisite && p.OptionKey == key })
	}
	// The spell is lost: level 3 has no always-prepared Bless, and the Heron stays picked.
	if !problem(knowsBuild(3, postureHeron), postureHeron) {
		t.Error("a held option does not show the problem after losing its spell")
	}
	// The option is lost: Wolf asks for Crow, which is not held.
	if !problem(knowsBuild(5, postureWolf), postureWolf) {
		t.Error("a held option does not show the problem after losing its option")
	}
	// The pact is lost: Crow asks for the Tome.
	if !problem(knowsBuild(5, postureCrow), postureCrow) {
		t.Error("a held option does not show the problem after losing its pact")
	}
}

// A hidden class, pact, spell or option is named by nobody a player reads: not the "Pede:" text, not the unmet reason,
// and not the unmet condition the server hands out.
func TestAPlayerReadsNoHiddenKeyOfAKnowledgePrerequisite(t *testing.T) {
	t.Parallel()
	base := loadForTest(t)
	class := genClass(base, genKinds[2])
	opt := TableOption{TableEntry: TableEntry{Key: "option:pact_boon:pacto-oculto" + tableSuffix, NamePT: "Pacto Oculto"}, Set: "pact_boon", DescPT: []string{"Texto."}}
	style := TableOption{TableEntry: TableEntry{Key: "option:fighting_style:estilo-oculto" + tableSuffix, NamePT: "Estilo Oculto"}, Set: "fighting_style", DescPT: []string{"Texto."}}
	o := Overlay{Revision: 1, Classes: []TableClass{class}, Options: []TableOption{opt, style}, Off: []string{class.Key, opt.Key, style.Key}}
	c := withOverlayOn(t, base, o)
	player := c.ForPlayer().c

	cases := []struct {
		name string
		p    FeatPrerequisite
		u    FeatUnmet
		hide string
	}{
		{"class", FeatPrerequisite{ClassKey: class.Key, ClassLevel: 3}, FeatUnmet{Kind: FeatUnmetClassLevel, Key: class.Key, Value: 3}, class.NamePT},
		{"pact", FeatPrerequisite{RequiresPact: opt.Key}, FeatUnmet{Kind: FeatUnmetPact, Key: opt.Key}, opt.NamePT},
		{"option", FeatPrerequisite{RequiresOptions: []string{style.Key}}, FeatUnmet{Kind: FeatUnmetOption, Key: style.Key}, style.NamePT},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			b := Build{}
			asks := player.featAsksPT(tc.p, b)
			unmet := player.featUnmetPT(tc.u, b)
			for _, text := range []string{asks, unmet} {
				if strings.Contains(text, tc.hide) || strings.Contains(text, "@mesa") {
					t.Errorf("a player reads %q", text)
				}
			}
			// The master reads the name, as the positive control.
			if got := c.c.featAsksPT(tc.p, b); !strings.Contains(got, tc.hide) {
				t.Errorf("the master's text = %q, want %q", got, tc.hide)
			}
		})
	}
}

// hiddenAskOverlay is the posture overlay whose Heron asks for a table class, a table option, a Pact Boon and a table
// spell, all switched off.
func hiddenAskOverlay(t *testing.T) (Overlay, []string) {
	t.Helper()
	class := genClass(loadForTest(t), genKinds[2])
	pact := TableOption{TableEntry: TableEntry{Key: "option:pact_boon:pacto-oculto" + tableSuffix, NamePT: "Pacto Oculto"}, Set: "pact_boon", DescPT: []string{"Texto."}}
	style := TableOption{TableEntry: TableEntry{Key: "option:fighting_style:estilo-oculto" + tableSuffix, NamePT: "Estilo Oculto"}, Set: "fighting_style", DescPT: []string{"Texto."}}
	spell := tableTestSpells()[0]
	o := postureOverlay()
	o.Classes, o.Options, o.Spells = []TableClass{class}, []TableOption{pact, style}, []TableSpell{spell}
	o.Off = []string{class.Key, pact.Key, style.Key, spell.Key}
	o.Subclasses[0].Levels[0].Features[0].Options[0].Prerequisite = TableOptionPrerequisite{
		ClassKey: class.Key, ClassLevel: 3, RequiresOptions: []string{style.Key}, RequiresPact: pact.Key, RequiresSpell: spell.Key,
	}
	return o, []string{class.NamePT, pact.NamePT, style.NamePT, spell.NamePT, "@mesa\"", class.Key, pact.Key, style.Key, spell.Key}
}

// Option prerequisites name no hidden entry: not in the key, the name or the reason of a player's choice list.
func TestOptionPrerequisitesNameNoHiddenEntry(t *testing.T) {
	t.Parallel()
	o, words := hiddenAskOverlay(t)
	c := withOverlay(t, o)
	heron := func(c *Content) ChoiceOption {
		return optionOf(t, optionChoiceOf(t, c.Choices(postureBuild(3)), postureFeature), postureHeron)
	}
	control, got := fmt.Sprintf("%+v", heron(c)), fmt.Sprintf("%+v", heron(c.ForPlayer()))
	for _, w := range words[:4] {
		if !strings.Contains(control, w) {
			t.Errorf("the master's option does not name %q: %s", w, control)
		}
	}
	for _, w := range words {
		if strings.Contains(got, w) {
			t.Errorf("a player's option names %q: %s", w, got)
		}
	}
	opt := heron(c.ForPlayer())
	if !opt.Blocked() || len(opt.Prerequisites) != 4 {
		t.Errorf("the player's option = %+v, want it blocked by four conditions", opt)
	}
}

// A lost option prerequisite is an issue of the sheet (as a lost feat prerequisite is), and the issue names no hidden entry.
func TestALostOptionPrerequisiteIsAnIssueOfTheSheet(t *testing.T) {
	t.Parallel()
	c := withOverlay(t, gatedPostureOverlay())
	lost := Derive(postureBuild(3, postureBull), c)
	i := slices.IndexFunc(lost.Issues, func(i Issue) bool { return i.Code == IssueOptionPrerequisite })
	if i < 0 {
		t.Fatalf("issues = %+v, want %q", lost.Issues, IssueOptionPrerequisite)
	}
	if got := lost.Issues[i]; got.Field != "full.feature_choice_keys[0]" || !strings.Contains(got.Message, "Postura do Touro") || !strings.Contains(got.Message, "Guerreiro 5, e o personagem está no 3") {
		t.Errorf("issue = %+v, want the option and what is missing", got)
	}
	if kept := Derive(postureBuild(5, postureBull), c); slices.ContainsFunc(kept.Issues, func(i Issue) bool { return i.Code == IssueOptionPrerequisite }) {
		t.Errorf("a met prerequisite raised an issue: %+v", kept.Issues)
	}

	o, words := hiddenAskOverlay(t)
	hc := withOverlay(t, o).ForPlayer()
	d := Derive(postureBuild(3, postureHeron), hc)
	i = slices.IndexFunc(d.Issues, func(i Issue) bool { return i.Code == IssueOptionPrerequisite })
	if i < 0 {
		t.Fatalf("a hidden condition raised no issue: %+v", d.Issues)
	}
	for _, w := range words {
		if strings.Contains(d.Issues[i].Message, w) {
			t.Errorf("the issue names %q: %s", w, d.Issues[i].Message)
		}
	}
}

// A lost option stops working as a lost feat does: its effect is gone while its prerequisite is unmet, the pick stays,
// and the effect comes back with the class level.
func TestAnOptionWhoseLevelIsLostStopsWorkingUntilItIsBack(t *testing.T) {
	t.Parallel()
	c := withOverlay(t, gatedPostureOverlay())
	base := Derive(postureBuild(3), c).Initiative
	lost := Derive(postureBuild(3, postureBull), c)
	if lost.Initiative != base {
		t.Errorf("initiative with the Bull lost = %d, want %d (no +2)", lost.Initiative, base)
	}
	if !slices.ContainsFunc(lost.Features, func(f Feature) bool { return f.Key == postureBull }) {
		t.Error("the lost option left the sheet")
	}
	if back := Derive(postureBuild(5, postureBull), c); back.Initiative != Derive(postureBuild(5), c).Initiative+2 {
		t.Errorf("initiative with the Bull held = %d, want the +2 back", back.Initiative)
	}
}

// The sentence of the issue reads as one, naming nobody: "X pede Y 5, e o personagem está no 3: …".
func TestTheOptionPrerequisiteIssueReadsAsOneSentence(t *testing.T) {
	t.Parallel()
	c := withOverlay(t, gatedPostureOverlay())
	d := Derive(postureBuild(3, postureBull), c)
	i := slices.IndexFunc(d.Issues, func(i Issue) bool { return i.Code == IssueOptionPrerequisite })
	const want = "Postura do Touro pede Guerreiro 5, e o personagem está no 3: a opção só vale de novo quando o personagem cumprir."
	if i < 0 || d.Issues[i].Message != want {
		t.Errorf("issue = %+v, want %q", d.Issues, want)
	}
}
