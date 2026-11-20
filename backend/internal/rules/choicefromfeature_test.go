package rules

import (
	"errors"
	"slices"
	"strings"
	"testing"
)

// A choice that offers the options of another feature (from_feature), from the same entry, from another one and from
// the SRD. Every name here is invented.

const (
	// fromFeatureSubclass is a table subclass of the Barbarian, another entry than the Mestre das Posturas, whose level 3
	// feature asks for two of the postures.
	fromFeatureSubclass = "subclass:rumor-das-posturas" + tableSuffix
	fromFeatureAsk      = "feature:rumor" + tableSuffix
)

// fromFeatureOverlay is the postures overlay with a second subclass, of the Barbarian, whose feature offers the postures.
func fromFeatureOverlay(e Effect) Overlay {
	o := postureOverlay()
	o.Subclasses = append(o.Subclasses, TableSubclass{
		TableEntry: TableEntry{Key: fromFeatureSubclass, NamePT: "Rumor das Posturas"}, Class: "class:barbarian",
		Levels: []TableSubclassLevel{{Level: 3, Features: []TableFeature{{Key: fromFeatureAsk, NamePT: "Rumor", DescPT: []string{"Texto de teste."}, Effects: []Effect{e}}}}},
	})
	return o
}

func askPostures(count int) Effect {
	return Effect{Type: "choice", Choice: "feature", Count: count, FromFeature: postureFeature}
}

func rumorBuild(level int, picks ...string) Build {
	return Build{
		BaseScores: map[Ability]int{STR: 15, DEX: 14, CON: 13, INT: 12, WIS: 10, CHA: 8},
		Race:       "race:human", Background: "background:acolyte", Languages: testLanguages,
		Classes:        []ClassLevel{{Class: "class:barbarian", Level: level, Subclass: fromFeatureSubclass}},
		FeatureChoices: picks,
	}
}

var allPostures = []string{postureHeron, postureBull, postureWolf, postureCrow, postureBear}

func TestAChoiceOffersTheOptionsOfAFeatureOfAnotherEntry(t *testing.T) {
	t.Parallel()
	c := withOverlay(t, fromFeatureOverlay(askPostures(2)))
	ch := optionChoiceOf(t, c.Choices(rumorBuild(3)), fromFeatureAsk)
	got := optionKeys(ch)
	slices.Sort(got)
	want := slices.Clone(allPostures)
	slices.Sort(want)
	if ch.Picks != 2 || !slices.Equal(got, want) {
		t.Fatalf("%d picks of %v, want 2 of the five postures", ch.Picks, got)
	}
	picked := []string{postureHeron, postureWolf}
	d := Derive(rumorBuild(3, picked...), c)
	for _, k := range picked {
		if !slices.ContainsFunc(d.Features, func(f Feature) bool { return f.Key == k }) {
			t.Errorf("the picked %s is not on the sheet", k)
		}
	}
	for _, is := range d.Issues {
		if strings.Contains(is.Field, "feature_choice_keys") {
			t.Errorf("issue %v about the picks, want none", is)
		}
	}
	set := c.Choices(rumorBuild(3, picked...))
	if p := optionChoiceOf(t, set, fromFeatureAsk); !slices.Equal(p.Picked, picked) || p.Missing() != 0 || len(set.NotOffered) != 0 {
		t.Errorf("picked %v, missing %d, not offered %v; want the two taken", p.Picked, p.Missing(), set.NotOffered)
	}
}

// The choices that offer the same options are one pool: the picks of the first are not offered again to the second.
func TestTheChoicesOfTheSameOptionsShareOnePool(t *testing.T) {
	t.Parallel()
	c := withOverlay(t, fromFeatureOverlay(askPostures(2)))
	b := rumorBuild(3, postureHeron, postureBull, postureWolf)
	b.Classes = append([]ClassLevel{{Class: "class:fighter", Level: 3, Subclass: postureSubclass}}, b.Classes...)
	set := c.Choices(b)
	own, ask := optionChoiceOf(t, set, postureFeature), optionChoiceOf(t, set, fromFeatureAsk)
	if !slices.Equal(own.Picked, []string{postureHeron, postureBull, postureWolf}) || len(ask.Picked) != 0 {
		t.Fatalf("the feature took %v and the ask %v, want the three picks in the feature", own.Picked, ask.Picked)
	}
	for _, o := range ask.Options {
		if want := slices.Contains(own.Picked, o.Key); o.Blocked() != want {
			t.Errorf("option %s blocked = %v, want %v", o.Key, o.Blocked(), want)
		}
	}
}

func TestAChoiceOffersTheOptionsOfAnSRDFeature(t *testing.T) {
	t.Parallel()
	c := withOverlay(t, fromFeatureOverlay(Effect{Type: "choice", Choice: "feature", Count: 1, FromFeature: "feature:fighter-fighting-style"}))
	ch := optionChoiceOf(t, c.Choices(rumorBuild(3)), fromFeatureAsk)
	if want := c.c.features["feature:fighter-fighting-style"].Options; len(want) == 0 || !slices.Equal(optionKeys(ch), want) {
		t.Errorf("options = %v, want %v", optionKeys(ch), want)
	}
}

func TestAFromFeatureThatIsNotAFeatureWithOptionsIsRefused(t *testing.T) {
	t.Parallel()
	base := loadForTest(t)
	feature := func(e Effect) Effect { e.Type, e.Choice, e.Count = "choice", "feature", max(e.Count, 1); return e }
	cases := map[string]struct {
		e          Effect
		field, msg string
	}{
		"a key the content does not have": {feature(Effect{FromFeature: "feature:nao-existe" + tableSuffix}), ".from_feature", "is not a feature of the content"},
		"a feature with no options":       {feature(Effect{FromFeature: "feature:second-wind"}), ".from_feature", "has no options"},
		"an option":                       {feature(Effect{FromFeature: postureHeron}), ".from_feature", "is an option"},
		"a trait":                         {feature(Effect{FromFeature: "trait:darkvision"}), ".from_feature", "is not a feature of the content"},
		"a list as well":                  {feature(Effect{FromFeature: postureFeature, From: []string{postureHeron}}), ".from_feature", "not several"},
		"a set as well":                   {feature(Effect{FromFeature: postureFeature, Set: "fighting_style"}), ".from_feature", "not several"},
		"a filter as well":                {feature(Effect{FromFeature: postureFeature, Filter: toolFilter("tool")}), ".filter", "only a choice of tools"},
		"a choice of skills":              {Effect{Type: "choice", Choice: "skill", Count: 1, FromFeature: postureFeature}, ".from_feature", "only a choice of features"},
		"more than the feature has":       {feature(Effect{FromFeature: postureFeature, Count: 6}), ".count", "more than the 5 options"},
		"the invocations":                 {feature(Effect{FromFeature: "feature:eldritch-invocations"}), ".from_feature", "invocations"},
		"the feature that asks":           {feature(Effect{FromFeature: fromFeatureAsk}), ".from_feature", "its own options"},
		"a feature that offers another's": {feature(Effect{FromFeature: "feature:rumor-do-rumor" + tableSuffix}), ".from_feature", "another feature's"},
		"a daily pick":                    {feature(Effect{FromFeature: postureFeature, DailyChoice: true, Duration: "short_rest"}), ".daily_choice", ""},
	}
	for name, tc := range cases {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			o := fromFeatureOverlay(tc.e)
			o.Subclasses[1].Levels = append(o.Subclasses[1].Levels, TableSubclassLevel{Level: 7, Features: []TableFeature{{
				Key: "feature:rumor-do-rumor" + tableSuffix, NamePT: "Rumor do rumor", Effects: []Effect{{Type: "choice", Choice: "feature", Count: 1, FromFeature: postureFeature}},
			}}})
			_, err := base.With(o)
			var oe *OverlayError
			if !errors.As(err, &oe) {
				t.Fatalf("With: %v, want an overlay error", err)
			}
			if !slices.ContainsFunc(oe.Violations(), func(v *OverlayError) bool {
				return strings.HasSuffix(v.Field, tc.field) && strings.Contains(v.Message, tc.msg) && strings.Contains(v.Key, "rumor@")
			}) {
				t.Errorf("no violation of %s with %q at the asking feature: %v", tc.field, tc.msg, err)
			}
		})
	}
}

// A feature that offers the options of another feature makes no other choice of features, in whatever order they are written.
func TestAFeatureThatOffersAnotherFeaturesOptionsMakesNoOtherChoiceOfFeatures(t *testing.T) {
	t.Parallel()
	base := loadForTest(t)
	other := Effect{Type: "choice", Choice: "feature", Count: 1, From: []string{postureHeron, postureBull}}
	for name, effects := range map[string][]Effect{
		"the offer first": {askPostures(1), other},
		"the offer last":  {other, askPostures(1)},
		"two offers":      {askPostures(1), askPostures(1)},
	} {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			o := fromFeatureOverlay(askPostures(1))
			o.Subclasses[1].Levels[0].Features[0].Effects = effects
			_, err := base.With(o)
			var oe *OverlayError
			if !errors.As(err, &oe) || !slices.ContainsFunc(oe.Violations(), func(v *OverlayError) bool {
				return strings.Contains(v.Message, "no other choice of features") || strings.Contains(v.Message, "offers one choice of options")
			}) {
				t.Errorf("With: %v, want the several choices refused", err)
			}
		})
	}
}

// Only a class or a subclass feature offers the options of another feature.
func TestAFromFeatureOnAnotherKindOfOwnerIsRefused(t *testing.T) {
	t.Parallel()
	base := loadForTest(t)
	o := postureOverlay()
	o.Backgrounds = []TableBackground{{
		TableEntry: TableEntry{Key: "background:cais-do-farol" + tableSuffix, NamePT: "Cais do farol"}, Skills: []string{"skill:athletics", "skill:history"},
		Feature: TableFeature{Key: "background-feature:rede-do-cais" + tableSuffix, NamePT: "Rede do cais", DescPT: []string{"Texto de teste."}, Effects: []Effect{askPostures(1)}},
	}}
	_, err := base.With(o)
	var oe *OverlayError
	if !errors.As(err, &oe) || !strings.HasSuffix(oe.Field, ".from_feature") || !strings.Contains(oe.Message, "only a class or subclass feature") {
		t.Errorf("With: %v, want the background feature refused at its from_feature", err)
	}
}

// A feature's options that the table keeps out of a sheet's pools: the subclass they belong to is switched off (for a
// player) or archived (for everyone), and the sheet holds none of it. A pool that is left with nothing asks nothing.
func TestTheOptionsOfAFeatureTheTableKeepsOutAreInNoPool(t *testing.T) {
	t.Parallel()
	base := loadForTest(t)
	closed := map[string]struct {
		close func(*Overlay)
		view  func(*Content) *Content
	}{
		"switched off, to a player": {func(o *Overlay) { o.Off = []string{postureSubclass} }, (*Content).ForPlayer},
		"archived, to a player":     {func(o *Overlay) { o.Subclasses[0].Archived = true }, (*Content).ForPlayer},
		"archived, to the master":   {func(o *Overlay) { o.Subclasses[0].Archived = true }, func(c *Content) *Content { return c }},
	}
	for name, tc := range closed {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			o := fromFeatureOverlay(askPostures(2))
			control := withOverlayOn(t, base, o)
			if got := optionKeys(optionChoiceOf(t, tc.view(control).Choices(rumorBuild(3)), fromFeatureAsk)); len(got) != len(allPostures) {
				t.Fatalf("before the table closes it, the pool has %v, want the five postures", got)
			}
			tc.close(&o)
			c := tc.view(withOverlayOn(t, base, o))
			ch := optionChoiceOf(t, c.Choices(rumorBuild(3)), fromFeatureAsk)
			if len(ch.Options) != 0 || ch.Picks != 0 || ch.Missing() != 0 || !ch.Advisory {
				t.Errorf("the pool has %v and asks %d (advisory %v), want it empty and asking nothing of the sheet", optionKeys(ch), ch.Picks, ch.Advisory)
			}
			if problems := c.Choices(rumorBuild(3)).ChoiceProblems(true); len(problems) != 0 {
				t.Errorf("problems = %v, want none for an empty pool", problems)
			}
			// A sheet that holds the subclass keeps what it offers.
			b := rumorBuild(3)
			b.Classes = append([]ClassLevel{{Class: "class:fighter", Level: 3, Subclass: postureSubclass}}, b.Classes...)
			if got := optionKeys(optionChoiceOf(t, c.Choices(b), fromFeatureAsk)); len(got) != len(allPostures) {
				t.Errorf("a sheet that holds the subclass has %v, want the five postures", got)
			}
		})
	}
}

// The master reads what is switched off, as the rest of the table's content: only a retired entry is closed to everyone.
func TestTheMasterReadsTheOptionsOfASwitchedOffFeature(t *testing.T) {
	t.Parallel()
	o := fromFeatureOverlay(askPostures(2))
	o.Off = []string{postureSubclass}
	c := withOverlay(t, o)
	if got := optionKeys(optionChoiceOf(t, c.Choices(rumorBuild(3)), fromFeatureAsk)); len(got) != len(allPostures) {
		t.Errorf("the master's pool has %v, want the five postures", got)
	}
	if got := optionKeys(optionChoiceOf(t, c.ForPlayer().Choices(rumorBuild(3)), fromFeatureAsk)); len(got) != 0 {
		t.Errorf("a player's pool has %v, want none", got)
	}
}

// An option of an entry the table keeps out is hidden whoever asks for it: the choice that asks owns no option.
func TestAnAskerIsNoOwnerOfTheOptionsItOffers(t *testing.T) {
	t.Parallel()
	o := fromFeatureOverlay(askPostures(2))
	o.Off = []string{postureSubclass}
	c := withOverlay(t, o)
	for _, k := range allPostures {
		if !c.Hidden(k) {
			t.Errorf("%s is not hidden, but its subclass is switched off", k)
		}
		if owners := c.Owners(k); !slices.Equal(owners, []string{postureSubclass}) {
			t.Errorf("%s has the owners %v, want the subclass it belongs to alone", k, owners)
		}
		if c.SheetHas(rumorBuild(3), k) {
			t.Errorf("a sheet that holds only the asking subclass has %s", k)
		}
	}
	if c.Hidden(fromFeatureAsk) {
		t.Error("the asking feature is hidden")
	}
}

// The filter is the asking choice's alone: every other pool offers what it offered without it.
func TestOnlyTheAskingChoiceLosesTheOptionsOfAFeatureTheTableKeepsOut(t *testing.T) {
	t.Parallel()
	base := loadForTest(t)
	const (
		other   = "subclass:rumor-do-estilo" + tableSuffix
		askMore = "feature:rumor-do-estilo" + tableSuffix
		guard   = "option:fighting_style:guarda-firme" + tableSuffix
	)
	overlay := func() Overlay {
		o := fromFeatureOverlay(Effect{Type: "choice", Choice: "feature", Count: 1, FromFeature: "feature:fighter-fighting-style"})
		o.Subclasses = append(o.Subclasses, TableSubclass{
			TableEntry: TableEntry{Key: other, NamePT: "Rumor do Estilo"}, Class: "class:barbarian",
			Levels: []TableSubclassLevel{{Level: 3, Features: []TableFeature{{Key: askMore, NamePT: "Rumor do estilo", DescPT: []string{"Texto de teste."}, Effects: []Effect{{Type: "choice", Choice: "feature", Count: 1, FromFeature: "feature:additional-fighting-style"}}}}}},
		})
		o.Options = []TableOption{{TableEntry: TableEntry{Key: guard, NamePT: "Guarda Firme"}, Set: "fighting_style", DescPT: []string{"Texto de teste."}}}
		return o
	}
	pool := func(c *Content, feature string, class ClassLevel) []string {
		b := Build{
			BaseScores: map[Ability]int{STR: 15, DEX: 14, CON: 13, INT: 12, WIS: 10, CHA: 8},
			Race:       "race:human", Background: "background:acolyte", Languages: testLanguages, Classes: []ClassLevel{class},
		}
		return optionKeys(optionChoiceOf(t, c.ForPlayer().Choices(b), feature))
	}
	fighter, paladin := ClassLevel{Class: "class:fighter", Level: 1}, ClassLevel{Class: "class:paladin", Level: 2}

	t.Run("two choices share the options of two features", func(t *testing.T) {
		t.Parallel()
		o := overlay()
		o.Off = []string{"subclass:champion"}
		c, plain := withOverlayOn(t, base, o), withOverlayOn(t, base, overlay())
		want := pool(plain, "feature:fighter-fighting-style", fighter)
		if got := pool(c, "feature:fighter-fighting-style", fighter); len(want) < 2 || !slices.Equal(got, want) {
			t.Errorf("a plain Fighter has %v, want %v", got, want)
		}
		if got := pool(c, "feature:fighter-fighting-style", fighter); !slices.Contains(got, "feature:fighter-fighting-style-archery") {
			t.Errorf("a plain Fighter has %v, want the Archery style", got)
		}
	})
	t.Run("a table option of the styles and the class that is switched off", func(t *testing.T) {
		t.Parallel()
		o := overlay()
		o.Off = []string{"class:fighter"}
		c := withOverlayOn(t, base, o)
		if got := pool(c, "feature:paladin-fighting-style", paladin); !slices.Contains(got, guard) {
			t.Errorf("a Paladin has %v, want the option %s", got, guard)
		}
	})
}

// An entry's own pick stays: a sheet that holds an option the table keeps out is not told it is not a choice, and what
// it holds is counted for its pool.
func TestAPickOfAnOptionTheTableKeepsOutIsKeptByASheetThatHoldsIt(t *testing.T) {
	t.Parallel()
	o := fromFeatureOverlay(askPostures(2))
	o.Off = []string{postureSubclass}
	c := withOverlay(t, o).ForPlayer()
	b := rumorBuild(3, postureHeron)
	if err := Validate(b, c); err != nil {
		t.Errorf("Validate = %v, want the held pick kept", err)
	}
	for _, is := range Derive(b, c).Issues {
		if strings.Contains(is.Field, "feature_choice_keys") {
			t.Errorf("issue %v about the held pick, want none", is)
		}
	}
	set := c.Choices(b)
	ch := optionChoiceOf(t, set, fromFeatureAsk)
	if !slices.Equal(ch.Picked, []string{postureHeron}) || !slices.Equal(optionKeys(ch), []string{postureHeron}) || ch.Picks != 1 || ch.Missing() != 0 {
		t.Errorf("picked %v of %v (%d picks), want the held pick alone, completed", ch.Picked, optionKeys(ch), ch.Picks)
	}
	if !slices.Equal(set.KeptOut, []string{postureHeron}) || len(set.NotOffered) != 0 {
		t.Errorf("kept out %v, not offered %v; want the held pick named as kept out, and offered", set.KeptOut, set.NotOffered)
	}
	if kept := c.Choices(rumorBuild(3)).KeptOut; len(kept) != 0 {
		t.Errorf("kept out %v for a sheet with no pick", kept)
	}
}

// A pool that loses options for the reader asks for no more than it lists: it reads as a source that has that many.
func TestAPoolThatLosesOptionsAsksForTheOnesItLists(t *testing.T) {
	t.Parallel()
	base := loadForTest(t)
	// The Fighter's styles and one the table wrote and switched off, against the styles alone.
	const style, guard = "feature:fighter-fighting-style", "option:fighting_style:guarda-firme" + tableSuffix
	size := len(base.c.features[style].Options)
	ask := func(count int, options ...TableOption) Overlay {
		o := fromFeatureOverlay(Effect{Type: "choice", Choice: "feature", Count: count, FromFeature: style})
		o.Options = options
		return o
	}
	table := TableOption{TableEntry: TableEntry{Key: guard, NamePT: "Guarda Firme"}, Set: "fighting_style", DescPT: []string{"Texto de teste."}}
	off := ask(size+1, table)
	off.Off = []string{guard}
	got := optionChoiceOf(t, withOverlayOn(t, base, off).ForPlayer().Choices(rumorBuild(3)), fromFeatureAsk)
	want := optionChoiceOf(t, withOverlayOn(t, base, ask(size)).ForPlayer().Choices(rumorBuild(3)), fromFeatureAsk)
	if got.Picks != size || got.Picks != want.Picks || len(got.Options) != size || got.Missing() != want.Missing() || got.Advisory != want.Advisory {
		t.Errorf("%d picks of %d options, missing %d; want what a source with %d options gives: %d of %d, missing %d", got.Picks, len(got.Options), got.Missing(), size, want.Picks, len(want.Options), want.Missing())
	}
}

// The level-up lists the pool the same way.
func TestTheLevelUpOffersNoOptionTheTableKeepsOut(t *testing.T) {
	t.Parallel()
	base := loadForTest(t)
	o := fromFeatureOverlay(askPostures(2))
	open := withOverlayOn(t, base, o)
	o.Off = []string{postureSubclass}
	closed := withOverlayOn(t, base, o).ForPlayer()
	due := func(c *Content) []LevelUpFeatureChoice {
		return c.c.namedChoices(rumorBuild(3), c.c.featureGains([]string{fromFeatureAsk}, fromFeatureSubclass).choices)
	}
	if got := due(open); len(got) != 1 || len(got[0].Options) != len(allPostures) || got[0].Choose != 2 {
		t.Fatalf("before the table closes it: %+v, want the five postures, two to pick", got)
	}
	if got := due(closed); len(got) != 1 || len(got[0].Options) != 0 || got[0].Choose != 0 {
		t.Errorf("after: %+v, want the choice with no option and nothing to pick", got)
	}
	if got := due(withOverlayOn(t, base, o)); len(got) != 1 || len(got[0].Options) != len(allPostures) {
		t.Errorf("the master's: %+v, want the five postures", got)
	}
}

// Choices that offer the options of one feature share the pool, and the picks they ask in all are the ones a player may
// make today, dealt in the order the picks are: the options the table switched off are no pick for either choice.
func TestTwoChoicesOfOnePoolAskOnlyForThePicksLeft(t *testing.T) {
	t.Parallel()
	const second = "feature:rumor-segundo" + tableSuffix
	o := fromFeatureOverlay(askPostures(1))
	more := TableFeature{Key: second, NamePT: "Rumor segundo", DescPT: []string{"Texto de teste."}, Effects: []Effect{askPostures(1)}}
	o.Subclasses[1].Levels[0].Features = append(o.Subclasses[1].Levels[0].Features, more)
	// The source offers the heron and two options the table switched off.
	o.Subclasses[0].Levels[0].Features[0].Options = o.Subclasses[0].Levels[0].Features[0].Options[:1]
	o.Subclasses[0].Levels[0].Features[0].Effects[1].Count = 1
	o.Subclasses[0].Levels = o.Subclasses[0].Levels[:1]
	for _, k := range []string{"option:table_feature:rumor-um" + tableSuffix, "option:table_feature:rumor-dois" + tableSuffix} {
		o.Options = append(o.Options, TableOption{TableEntry: TableEntry{Key: k, NamePT: "Opção " + k}, Set: "table_feature", OfferedBy: postureFeature, DescPT: []string{"Texto de teste."}})
		o.Off = append(o.Off, k)
	}
	master := withOverlay(t, o)

	set := master.ForPlayer().Choices(rumorBuild(3, postureHeron))
	first, next := optionChoiceOf(t, set, fromFeatureAsk), optionChoiceOf(t, set, second)
	if m := master.Choices(rumorBuild(3, postureHeron)); optionChoiceOf(t, m, fromFeatureAsk).Picks != first.Picks || optionChoiceOf(t, m, second).Picks != next.Picks {
		t.Errorf("the master reads other numbers than the player")
	}
	if first.Picks+next.Picks != 1 || first.Missing() != 0 || next.Missing() != 0 {
		t.Errorf("the choices ask %d and %d picks, missing %d and %d; want one pick of the one option left, made",
			first.Picks, next.Picks, first.Missing(), next.Missing())
	}
	if p := set.Pending(); len(p) != 0 {
		t.Errorf("pending = %v, want none", p)
	}
	if problems := set.ChoiceProblems(true); len(problems) != 0 {
		t.Errorf("problems = %v, want none", problems)
	}
}

// The level-up offers the choices that share a pool the picks the pool has left, as the sheet's choices ask them: the
// level that adds the second choice finds the one option taken by the first, and offers nothing to pick for it.
func TestTheLevelUpOffersAChoiceOfASharedPoolTheShareThePoolHasLeft(t *testing.T) {
	t.Parallel()
	const second = "feature:rumor-segundo" + tableSuffix
	o := fromFeatureOverlay(askPostures(1))
	o.Subclasses[1].Levels = append(o.Subclasses[1].Levels, TableSubclassLevel{Level: 6, Features: []TableFeature{
		{Key: second, NamePT: "Rumor segundo", DescPT: []string{"Texto de teste."}, Effects: []Effect{askPostures(1)}},
	}})
	o.Subclasses[0].Levels[0].Features[0].Options = o.Subclasses[0].Levels[0].Features[0].Options[:1]
	o.Subclasses[0].Levels[0].Features[0].Effects[1].Count = 1
	o.Subclasses[0].Levels = o.Subclasses[0].Levels[:1]
	for _, k := range []string{"option:table_feature:rumor-um" + tableSuffix, "option:table_feature:rumor-dois" + tableSuffix} {
		o.Options = append(o.Options, TableOption{TableEntry: TableEntry{Key: k, NamePT: "Opção " + k}, Set: "table_feature", OfferedBy: postureFeature, DescPT: []string{"Texto de teste."}})
		o.Off = append(o.Off, k)
	}
	before := rumorBuild(5, postureHeron)
	for name, view := range map[string]*Content{"player": withOverlay(t, o).ForPlayer(), "master": withOverlay(t, o)} {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			offer, err := LevelUpOptions(before, "class:barbarian", view)
			if err != nil {
				t.Fatalf("LevelUpOptions: %v", err)
			}
			i := slices.IndexFunc(offer.FeatureChoices, func(fc LevelUpFeatureChoice) bool { return fc.Feature.Key == second })
			if i < 0 || offer.FeatureChoices[i].Choose != 0 {
				t.Fatalf("the offer is %+v, want the new choice with nothing to pick", offer.FeatureChoices)
			}
			ch := satisfy(t, view, before, "class:barbarian", "")
			if err := CheckLevelUpOf(before, mustApply(t, view, before, ch), ch, view); err != nil {
				t.Errorf("the level with no pick for the new choice: %v", err)
			}
		})
	}
}
