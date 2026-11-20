package rules

import (
	"errors"
	"slices"
	"strings"
	"testing"
)

// The swap of the options of a table feature at a level up (#715): the feature says at which class levels a pick of its
// pool may be swapped. Every name here is invented.

const (
	postureHawk = "feature:postura-falcao" + tableSuffix
	postureOx   = "feature:postura-boi" + tableSuffix
)

// swapPostureOverlay is postureOverlay for class, with two more options (seven in the pool; five are picked by
// level 7) and the swap levels of its two features. The Ox asks for the Heron.
func swapPostureOverlay(class string, first, more FeatureSwapLevels) Overlay {
	o := postureOverlay()
	s := &o.Subclasses[0]
	s.Class = class
	f := &s.Levels[0].Features[0]
	ox := postureOption(postureOx, "Postura do Boi")
	ox.Prerequisite = TableOptionPrerequisite{RequiresOptions: []string{postureHeron}}
	f.Options = append(f.Options, postureOption(postureHawk, "Postura do Falcão"), ox)
	f.SwapLevels = first
	s.Levels[1].Features[0].SwapLevels = more
	return o
}

// postureFighter is a fighter of the Master of Postures at total level to, with every choice made.
func postureFighter(t *testing.T, c *Content, to int) Build {
	t.Helper()
	return sweepUp(t, c, sweepBase(t, c, "class:fighter", postureSubclass), "class:fighter", postureSubclass, to)
}

func featureGroupOf(o LevelUpOffer, key string) (SwapGroup, bool) {
	i := slices.IndexFunc(o.FeatureSwaps, func(g SwapGroup) bool { return g.Group == key })
	if i < 0 {
		return SwapGroup{}, false
	}
	return o.FeatureSwaps[i], true
}

func namedKeys(ns []NamedKey) []string {
	var out []string
	for _, n := range ns {
		out = append(out, n.Key)
	}
	return out
}

func mustOffer(t *testing.T, c *Content, b Build, class string) LevelUpOffer {
	t.Helper()
	o, err := LevelUpOptions(b, class, c)
	if err != nil {
		t.Fatalf("LevelUpOptions: %v", err)
	}
	return o
}

// A fighter 4 reaching level 5, which has no Ability Score Improvement, is offered a swap of the Postures the feature
// allows at level 5, with no rule of the table involved; the check accepts the swap and refuses anything else.
func TestAFeatureOffersASwapAtItsListedLevel(t *testing.T) {
	t.Parallel()
	c := withOverlay(t, swapPostureOverlay("class:fighter", FeatureSwapLevels{Levels: []int{5}}, FeatureSwapLevels{}))
	at4 := postureFighter(t, c, 4)

	o := mustOffer(t, c, at4, "class:fighter")
	g, ok := featureGroupOf(o, postureFeature)
	if o.AbilityScoreImprovement || !ok || len(o.FeatureSwaps) != 1 || len(o.PickSwaps) != 0 {
		t.Fatalf("level 5 offers %+v (ASI %v, pick swaps %v), want the Postures group only", o.FeatureSwaps, o.AbilityScoreImprovement, o.PickSwaps)
	}
	if g.NamePT != "Posturas" || !slices.Equal(namedKeys(g.Picks), []string{postureHeron, postureBull, postureWolf}) ||
		!slices.Equal(namedKeys(g.Pool), []string{postureCrow, postureBear, postureHawk, postureOx}) {
		t.Fatalf("group = %+v, want the three picks and the four other options of the pool", g)
	}

	ch := satisfy(t, c, at4, "class:fighter", postureSubclass)
	plain := mustApply(t, c, at4, ch)
	if err := CheckLevelUp(at4, plain, c); err != nil {
		t.Fatalf("the level without a swap: %v", err)
	}
	ch.FeatureSwaps = []PickSwap{{Group: postureFeature, From: postureHeron, To: postureCrow}}
	if !o.AllowsFeatureSwaps(ch.FeatureSwaps) {
		t.Fatalf("the offer does not allow %+v", ch.FeatureSwaps)
	}
	after := mustApply(t, c, at4, ch)
	if err := CheckLevelUp(at4, after, c); err != nil {
		t.Fatalf("the swap: %v", err)
	}
	if slices.Contains(after.FeatureChoices, postureHeron) || !slices.Contains(after.FeatureChoices, postureCrow) || len(after.FeatureChoices) != len(at4.FeatureChoices) {
		t.Errorf("picks after the swap = %v, want the Crow for the Heron", after.FeatureChoices)
	}

	// Giving up a pick with nothing in its place is refused.
	lost := plain
	lost.FeatureChoices = slices.DeleteFunc(slices.Clone(plain.FeatureChoices), func(k string) bool { return k == postureHeron })
	wantRefusal(t, CheckLevelUp(at4, lost, c), LevelUpReasonFeatureChoice, "full.feature_choice_keys")
}

// A level the feature does not list offers nothing, and a sheet that gave up an option there is refused.
func TestAFeatureOffersNoSwapOutsideItsLevels(t *testing.T) {
	t.Parallel()
	c := withOverlay(t, swapPostureOverlay("class:fighter", FeatureSwapLevels{Levels: []int{5}}, FeatureSwapLevels{}))
	for _, from := range []int{2, 3, 5} { // to levels 3, 4 and 6
		b := postureFighter(t, c, from)
		if o := mustOffer(t, c, b, "class:fighter"); len(o.FeatureSwaps) != 0 {
			t.Errorf("level %d offers %+v, want no swap", from+1, o.FeatureSwaps)
		}
	}
	at5 := postureFighter(t, c, 5)
	ch := satisfy(t, c, at5, "class:fighter", postureSubclass)
	ch.FeatureSwaps = []PickSwap{{Group: postureFeature, From: postureHeron, To: postureCrow}}
	if mustOffer(t, c, at5, "class:fighter").AllowsFeatureSwaps(ch.FeatureSwaps) {
		t.Error("the offer allows a swap at level 6")
	}
	wantRefusal(t, CheckLevelUp(at5, mustApply(t, c, at5, ch), c), LevelUpReasonFeatureChoice, "full.feature_choice_keys")
}

// "Every level" is every class level from the feature's own level on: not at the level that gives the feature, and not
// before it.
func TestAFeatureThatSwapsAtEveryLevelStartsAtItsOwnLevel(t *testing.T) {
	t.Parallel()
	c := withOverlay(t, swapPostureOverlay("class:fighter", FeatureSwapLevels{EveryLevel: true}, FeatureSwapLevels{}))
	for from, want := range map[int]bool{1: false, 2: false, 3: true, 4: true, 5: true, 6: true} {
		b := postureFighter(t, c, from)
		_, got := featureGroupOf(mustOffer(t, c, b, "class:fighter"), postureFeature)
		if got != want {
			t.Errorf("level %d offers the group = %v, want %v", from+1, got, want)
		}
	}
	b := postureFighter(t, c, 4)
	ch := satisfy(t, c, b, "class:fighter", postureSubclass)
	ch.FeatureSwaps = []PickSwap{{Group: postureFeature, From: postureWolf, To: postureBear}}
	if err := CheckLevelUp(b, mustApply(t, c, b, ch), c); err != nil {
		t.Errorf("a swap at level 5: %v", err)
	}
}

// Two features that allow a swap at the same level each have their own group, and the pool joins their options.
func TestTwoFeaturesSwapAtTheSameLevelEachWithItsOwnGroup(t *testing.T) {
	t.Parallel()
	both := FeatureSwapLevels{Levels: []int{8}}
	c := withOverlay(t, swapPostureOverlay("class:fighter", both, both))
	at7 := postureFighter(t, c, 7)
	o := mustOffer(t, c, at7, "class:fighter")
	first, ok1 := featureGroupOf(o, postureFeature)
	more, ok2 := featureGroupOf(o, postureMore)
	if !ok1 || !ok2 || len(o.FeatureSwaps) != 2 {
		t.Fatalf("level 8 offers %+v, want one group for each feature", o.FeatureSwaps)
	}
	if want := []string{postureHawk, postureOx}; !slices.Equal(namedKeys(first.Pool), want) || !slices.Equal(namedKeys(more.Pool), want) || len(first.Picks) != 5 {
		t.Fatalf("groups = %+v and %+v, want the five picks and the two options left", first, more)
	}

	ch := satisfy(t, c, at7, "class:fighter", postureSubclass)
	ch.FeatureSwaps = []PickSwap{
		{Group: postureFeature, From: postureBull, To: postureHawk},
		{Group: postureMore, From: postureWolf, To: postureOx},
	}
	if !o.AllowsFeatureSwaps(ch.FeatureSwaps) {
		t.Fatal("the offer does not allow one swap for each feature")
	}
	after := mustApply(t, c, at7, ch)
	if err := CheckLevelUp(at7, after, c); err != nil {
		t.Fatalf("two swaps: %v", err)
	}
	for _, k := range []string{postureBull, postureWolf} {
		if slices.Contains(after.FeatureChoices, k) {
			t.Errorf("%s was not given up: %v", k, after.FeatureChoices)
		}
	}
	for _, k := range []string{postureHawk, postureOx, postureHeron, postureCrow, postureBear} {
		if !slices.Contains(after.FeatureChoices, k) {
			t.Errorf("%s is not among the picks: %v", k, after.FeatureChoices)
		}
	}
}

// The offer lets a swap be made only as it offered it.
func TestTheOfferRefusesSwapsItDidNotOffer(t *testing.T) {
	t.Parallel()
	both := FeatureSwapLevels{Levels: []int{8}}
	c := withOverlay(t, swapPostureOverlay("class:fighter", both, both))
	o := mustOffer(t, c, postureFighter(t, c, 7), "class:fighter")
	swap := func(group, from, to string) PickSwap { return PickSwap{Group: group, From: from, To: to} }
	cases := []struct {
		name  string
		swaps []PickSwap
		want  bool
	}{
		{"none", nil, true},
		{"one", []PickSwap{swap(postureFeature, postureBull, postureHawk)}, true},
		{"a group that is not offered", []PickSwap{swap("feature:outra@mesa", postureBull, postureHawk)}, false},
		{"a pick the sheet does not have", []PickSwap{swap(postureFeature, postureHawk, postureOx)}, false},
		{"an option already picked", []PickSwap{swap(postureFeature, postureBull, postureWolf)}, false},
		{"an option outside the pool", []PickSwap{swap(postureFeature, postureBull, "feature:fighter-fighting-style-dueling")}, false},
		{"the same option given up and taken", []PickSwap{swap(postureFeature, postureBull, postureBull)}, false},
		{"two swaps in one group", []PickSwap{swap(postureFeature, postureBull, postureHawk), swap(postureFeature, postureWolf, postureOx)}, false},
		{"the same pick given up twice", []PickSwap{swap(postureFeature, postureBull, postureHawk), swap(postureMore, postureBull, postureOx)}, false},
		{"the same option taken twice", []PickSwap{swap(postureFeature, postureBull, postureHawk), swap(postureMore, postureWolf, postureHawk)}, false},
	}
	for _, tc := range cases {
		if got := o.AllowsFeatureSwaps(tc.swaps); got != tc.want {
			t.Errorf("%s: allowed = %v, want %v", tc.name, got, tc.want)
		}
	}
}

// The new option is checked as any pick is: its prerequisite (read on the sheet that comes out, without the option given
// up) and the options already taken.
func TestASwapTakesAnOptionOnlyIfItsPrerequisiteHolds(t *testing.T) {
	t.Parallel()
	c := withOverlay(t, swapPostureOverlay("class:fighter", FeatureSwapLevels{Levels: []int{8}}, FeatureSwapLevels{}))
	at7 := postureFighter(t, c, 7)
	ch := satisfy(t, c, at7, "class:fighter", postureSubclass)

	// The Ox asks for the Heron: a swap that keeps the Heron works, one that gives it up does not.
	ch.FeatureSwaps = []PickSwap{{Group: postureFeature, From: postureBull, To: postureOx}}
	if err := CheckLevelUp(at7, mustApply(t, c, at7, ch), c); err != nil {
		t.Fatalf("the Ox with the Heron kept: %v", err)
	}
	ch.FeatureSwaps = []PickSwap{{Group: postureFeature, From: postureHeron, To: postureOx}}
	wantRefusal(t, CheckLevelUp(at7, mustApply(t, c, at7, ch), c), LevelUpReasonFeatureChoice, "full.feature_choice_keys")

	// An option the sheet already has is not a new one.
	ch.FeatureSwaps = []PickSwap{{Group: postureFeature, From: postureHeron, To: postureBull}}
	wantRefusal(t, CheckLevelUp(at7, mustApply(t, c, at7, ch), c), LevelUpReasonFeatureChoice, "")
}

// A swap of a feature's option and the swap of the table's rule (a fighting style) are made at the same level, and so are a
// cantrip and a feature's option.
func TestAFeatureSwapGoesWithThePickSwapOfTheLevel(t *testing.T) {
	t.Parallel()
	t.Run("a fighting style", func(t *testing.T) {
		t.Parallel()
		c := withOverlay(t, swapPostureOverlay("class:fighter", FeatureSwapLevels{Levels: []int{8}}, FeatureSwapLevels{}))
		at7 := postureFighter(t, c, 7)
		o := mustOffer(t, c, at7, "class:fighter")
		i := slices.IndexFunc(o.PickSwaps, func(g SwapGroup) bool { return g.Group == SwapGroupFightStyle })
		if !o.AbilityScoreImprovement || i < 0 {
			t.Fatalf("level 8 offers pick swaps %+v, want the fighting style", o.PickSwaps)
		}
		g := o.PickSwaps[i]
		ch := satisfy(t, c, at7, "class:fighter", postureSubclass)
		ch.Swap = PickSwap{Group: SwapGroupFightStyle, From: g.Picks[0].Key, To: g.Pool[0].Key}
		ch.FeatureSwaps = []PickSwap{{Group: postureFeature, From: postureWolf, To: postureHawk}}
		after := mustApply(t, c, at7, ch)
		if err := CheckLevelUp(at7, after, c); err != nil {
			t.Fatalf("both swaps: %v", err)
		}
		if slices.Contains(after.FeatureChoices, ch.Swap.From) || slices.Contains(after.FeatureChoices, postureWolf) ||
			!slices.Contains(after.FeatureChoices, ch.Swap.To) || !slices.Contains(after.FeatureChoices, postureHawk) {
			t.Errorf("picks after both swaps = %v", after.FeatureChoices)
		}
	})
	t.Run("a cantrip", func(t *testing.T) {
		t.Parallel()
		c := withOverlay(t, swapPostureOverlay("class:wizard", FeatureSwapLevels{Levels: []int{4}}, FeatureSwapLevels{}))
		b := sweepUp(t, c, sweepBase(t, c, "class:wizard", postureSubclass), "class:wizard", postureSubclass, 3)
		o := mustOffer(t, c, b, "class:wizard")
		i := slices.IndexFunc(o.PickSwaps, func(g SwapGroup) bool { return g.Group == SwapGroupCantrip })
		if _, ok := featureGroupOf(o, postureFeature); i < 0 || !ok {
			t.Fatalf("level 4 offers %+v and %+v, want the cantrips and the Postures", o.PickSwaps, o.FeatureSwaps)
		}
		ch := satisfy(t, c, b, "class:wizard", postureSubclass)
		pool := o.PickSwaps[i].Pool
		to := pool[slices.IndexFunc(pool, func(n NamedKey) bool { return !slices.Contains(ch.Cantrips, n.Key) })].Key
		ch.Swap = PickSwap{Group: SwapGroupCantrip, From: o.PickSwaps[i].Picks[0].Key, To: to}
		ch.FeatureSwaps = []PickSwap{{Group: postureFeature, From: postureHeron, To: postureCrow}}
		after := mustApply(t, c, b, ch)
		if err := CheckLevelUp(b, after, c); err != nil {
			t.Fatalf("a cantrip and an option: %v", err)
		}
		if slices.Contains(after.Cantrips, ch.Swap.From) || !slices.Contains(after.Cantrips, to) || !slices.Contains(after.FeatureChoices, postureCrow) {
			t.Errorf("after both swaps: cantrips %v, picks %v", after.Cantrips, after.FeatureChoices)
		}
	})
}

// Only the class that gains the level offers a swap of its features.
func TestAnotherClassGainingTheLevelOffersNoFeatureSwap(t *testing.T) {
	t.Parallel()
	c := withOverlay(t, swapPostureOverlay("class:fighter", FeatureSwapLevels{EveryLevel: true}, FeatureSwapLevels{}))
	b := postureFighter(t, c, 4)
	b.BaseScores[INT] = 13
	if _, ok := featureGroupOf(mustOffer(t, c, b, "class:fighter"), postureFeature); !ok {
		t.Fatal("the fighter's own level offers no group")
	}
	if o := mustOffer(t, c, b, "class:wizard"); len(o.FeatureSwaps) != 0 {
		t.Errorf("a new class offers %+v, want none", o.FeatureSwaps)
	}
	b.Classes = append(b.Classes, ClassLevel{Class: "class:wizard", Level: 1})
	if o := mustOffer(t, c, b, "class:wizard"); len(o.FeatureSwaps) != 0 {
		t.Errorf("the wizard's second level offers %+v, want none", o.FeatureSwaps)
	}
}

// A table option the master archived is not offered to take, and the table's options of a feature are in its pool.
func TestAFeatureSwapOffersTheTablesOptionsAndNotTheArchivedOnes(t *testing.T) {
	t.Parallel()
	const (
		blow = "option:table_feature:golpe-do-corvo@mesa"
		old  = "option:table_feature:golpe-antigo@mesa"
	)
	o := swapPostureOverlay("class:fighter", FeatureSwapLevels{Levels: []int{5}}, FeatureSwapLevels{})
	o.Options = []TableOption{
		{TableEntry: TableEntry{Key: blow, NamePT: "Golpe do Corvo"}, Set: "table_feature", OfferedBy: postureFeature, DescPT: []string{"Um golpe."}},
		{TableEntry: TableEntry{Key: old, NamePT: "Golpe Antigo", Archived: true}, Set: "table_feature", OfferedBy: postureFeature, DescPT: []string{"Um golpe velho."}},
	}
	c := withOverlay(t, o)
	b := postureFighter(t, c, 4)
	g, ok := featureGroupOf(mustOffer(t, c, b, "class:fighter"), postureFeature)
	if !ok || !slices.Contains(namedKeys(g.Pool), blow) || slices.Contains(namedKeys(g.Pool), old) {
		t.Fatalf("pool = %v, want the table's option %s and not the archived %s", namedKeys(g.Pool), blow, old)
	}
	ch := satisfy(t, c, b, "class:fighter", postureSubclass)
	ch.FeatureSwaps = []PickSwap{{Group: postureFeature, From: postureHeron, To: blow}}
	after := mustApply(t, c, b, ch)
	if err := CheckLevelUp(b, after, c); err != nil {
		t.Fatalf("a swap for the table's option: %v", err)
	}
	if !slices.Contains(after.FeatureChoices, blow) {
		t.Errorf("picks = %v, want the table's option", after.FeatureChoices)
	}
}

// Two features that share no option have separate pools: a group offers only its own feature's options.
func TestFeaturesThatShareNoOptionHaveSeparatePools(t *testing.T) {
	t.Parallel()
	const (
		other  = "feature:esquivas" + tableSuffix
		dodgeA = "feature:esquiva-a" + tableSuffix
		dodgeB = "feature:esquiva-b" + tableSuffix
	)
	o := swapPostureOverlay("class:fighter", FeatureSwapLevels{Levels: []int{5}}, FeatureSwapLevels{})
	esquivas := TableFeature{
		Key: other, NamePT: "Esquivas", DescPT: []string{"Escolha."}, SwapLevels: FeatureSwapLevels{Levels: []int{5}},
		Effects: []Effect{{Type: "choice", Choice: "feature", Count: 1}},
		Options: []TableFeature{postureOption(dodgeA, "Esquiva A"), postureOption(dodgeB, "Esquiva B")},
	}
	o.Subclasses[0].Levels[0].Features = append(o.Subclasses[0].Levels[0].Features, esquivas)
	c := withOverlay(t, o)
	b := postureFighter(t, c, 4)
	offer := mustOffer(t, c, b, "class:fighter")
	a, ok1 := featureGroupOf(offer, postureFeature)
	d, ok2 := featureGroupOf(offer, other)
	if !ok1 || !ok2 {
		t.Fatalf("level 5 offers %+v, want both groups", offer.FeatureSwaps)
	}
	if slices.Contains(namedKeys(a.Pool), dodgeB) || !slices.Equal(namedKeys(d.Pool), []string{dodgeB}) {
		t.Errorf("pools = %v and %v, want each the feature's own options", namedKeys(a.Pool), namedKeys(d.Pool))
	}
}

// A removal that no group offered is refused, whichever it is.
func TestALevelUpThatGivesUpMoreThanTheGroupsOfferIsRefused(t *testing.T) {
	t.Parallel()
	c := withOverlay(t, swapPostureOverlay("class:fighter", FeatureSwapLevels{Levels: []int{8}}, FeatureSwapLevels{}))
	at7 := postureFighter(t, c, 7)
	ch := satisfy(t, c, at7, "class:fighter", postureSubclass)
	// Two options of the group swapped for two others: one swap is offered.
	ch.FeatureSwaps = []PickSwap{
		{Group: postureFeature, From: postureBull, To: postureHawk},
		{Group: postureFeature, From: postureWolf, To: postureOx},
	}
	wantRefusal(t, CheckLevelUp(at7, mustApply(t, c, at7, ch), c), LevelUpReasonFeatureChoice, "full.feature_choice_keys")
}

// The refusals of the overlay for the swap levels name their field.
func TestSwapLevelsRefusalsNameTheirField(t *testing.T) {
	t.Parallel()
	const feature = "subclasses[0].levels[0].features[0]"
	cases := []struct {
		name  string
		edit  func(o *Overlay)
		field string
	}{
		{"a level below 1", func(o *Overlay) { setSwapLevels(o, FeatureSwapLevels{Levels: []int{5, 0}}) }, feature + ".swap_levels.levels[1]"},
		{"a level above 20", func(o *Overlay) { setSwapLevels(o, FeatureSwapLevels{Levels: []int{21}}) }, feature + ".swap_levels.levels[0]"},
		{"a repeated level", func(o *Overlay) { setSwapLevels(o, FeatureSwapLevels{Levels: []int{4, 6, 4}}) }, feature + ".swap_levels.levels[2]"},
		{"levels and every level", func(o *Overlay) { setSwapLevels(o, FeatureSwapLevels{Levels: []int{4}, EveryLevel: true}) }, feature + ".swap_levels.every_level"},
		{"a feature with no options", func(o *Overlay) {
			o.Subclasses[0].Levels[1].Features = append(o.Subclasses[0].Levels[1].Features, tf("sem-opcoes", "Sem opções"))
			o.Subclasses[0].Levels[1].Features[1].SwapLevels = FeatureSwapLevels{EveryLevel: true}
		}, "subclasses[0].levels[1].features[1].swap_levels"},
		{"an option", func(o *Overlay) {
			o.Subclasses[0].Levels[0].Features[0].Options[1].SwapLevels = FeatureSwapLevels{Levels: []int{5}}
		}, feature + ".options[1].swap_levels"},
		{"a trait", func(o *Overlay) {
			race, _, _ := tableMisc()
			race.Traits[0].Options = []TableFeature{{Key: "trait:resistente-a" + tableSuffix, NamePT: "A", DescPT: []string{"A."}}}
			race.Traits[0].SwapLevels = FeatureSwapLevels{EveryLevel: true}
			o.Races = append(o.Races, race)
		}, "races[0].traits[0].swap_levels"},
		{"a background feature", func(o *Overlay) {
			_, _, bg := tableMisc()
			bg.Feature.SwapLevels = FeatureSwapLevels{EveryLevel: true}
			o.Backgrounds = append(o.Backgrounds, bg)
		}, "backgrounds[0].feature.swap_levels"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			o := postureOverlay()
			tc.edit(&o)
			_, err := loadForTest(t).With(o)
			oe, ok := errors.AsType[*OverlayError](err)
			if !ok {
				t.Fatalf("err = %v, want an overlay error", err)
			}
			found := slices.ContainsFunc(oe.Violations(), func(v *OverlayError) bool { return v.Field == tc.field })
			if !found {
				t.Errorf("no violation at %s: %v", tc.field, err)
			}
		})
	}
}

func setSwapLevels(o *Overlay, s FeatureSwapLevels) {
	o.Subclasses[0].Levels[0].Features[0].SwapLevels = s
}

// The swap levels a feature may have: a list, every level, none; on a class or a subclass feature that has options,
// whether its options are its own or another feature of the entry lists them.
func TestSwapLevelsOfAFeatureWithOptionsAreAccepted(t *testing.T) {
	t.Parallel()
	for name, s := range map[string]FeatureSwapLevels{
		"a list": {Levels: []int{1, 20}}, "every level": {EveryLevel: true}, "none": {},
	} {
		o := postureOverlay()
		setSwapLevels(&o, s)
		o.Subclasses[0].Levels[1].Features[0].SwapLevels = s // the feature that lists the options of the first
		if _, err := loadForTest(t).With(o); err != nil {
			t.Errorf("%s: %v", name, err)
		}
	}
	// What the message says: the text of a refusal is for the master, in plain words.
	o := postureOverlay()
	setSwapLevels(&o, FeatureSwapLevels{Levels: []int{99}})
	if _, err := loadForTest(t).With(o); err == nil || !strings.Contains(err.Error(), "1 to 20") {
		t.Errorf("the refusal of level 99 = %v, want it to say 1 to 20", err)
	}
}

// The sheet's text of a feature that allows a swap ends with one line that says at which levels, and a feature that allows none keeps
// the master's text as it is.
func TestTheSheetTextOfAFeatureSaysAtWhichLevelsAnOptionCanBeSwapped(t *testing.T) {
	t.Parallel()
	last := func(c *Content, picks ...string) (desc, descPT []string) {
		for _, f := range Derive(postureBuild(7, picks...), c).Features {
			if f.Key == postureFeature {
				return f.Description, f.DescriptionPT
			}
		}
		t.Fatalf("the sheet has no %s", postureFeature)
		return nil, nil
	}
	picks := []string{postureHeron, postureBull, postureWolf, postureCrow, postureBear}
	for _, tc := range []struct {
		name string
		swap FeatureSwapLevels
		want string
	}{
		{"one level", FeatureSwapLevels{Levels: []int{5}}, "**Troca:** no nível 5 de Guerreiro você pode trocar uma opção desta característica por outra."},
		{"two levels, unordered", FeatureSwapLevels{Levels: []int{8, 5}}, "**Troca:** nos níveis 5 e 8 de Guerreiro você pode trocar uma opção desta característica por outra."},
		{"three levels", FeatureSwapLevels{Levels: []int{5, 8, 12}}, "**Troca:** nos níveis 5, 8 e 12 de Guerreiro você pode trocar uma opção desta característica por outra."},
		{"every level", FeatureSwapLevels{EveryLevel: true}, "**Troca:** em todo nível de Guerreiro a partir do nível 3 você pode trocar uma opção desta característica por outra."},
	} {
		c := withOverlay(t, swapPostureOverlay("class:fighter", tc.swap, FeatureSwapLevels{}))
		desc, descPT := last(c, picks...)
		if len(desc) != 2 || desc[1] != tc.want || !slices.Equal(desc, descPT) {
			t.Errorf("%s: text = %q (pt %q), want the master's paragraph and %q", tc.name, desc, descPT, tc.want)
		}
	}
	plain := withOverlay(t, swapPostureOverlay("class:fighter", FeatureSwapLevels{}, FeatureSwapLevels{}))
	if desc, _ := last(plain, picks...); len(desc) != 1 {
		t.Errorf("a feature with no swap levels reads %q, want the master's text alone", desc)
	}
}

// A pick the new sheet lost is excused only by the swap the request sent for it, and the invocation field takes a pick out
// only for a warlock's invocation.
func TestARemovalIsExcusedOnlyByTheSwapSentForIt(t *testing.T) {
	t.Parallel()
	c := withOverlay(t, swapPostureOverlay("class:fighter", FeatureSwapLevels{Levels: []int{5}}, FeatureSwapLevels{}))
	at4 := postureFighter(t, c, 4)
	ch := satisfy(t, c, at4, "class:fighter", postureSubclass)
	ch.SwapInvocation = postureHeron
	ch.FeatureChoices = []string{postureCrow}
	after := mustApply(t, c, at4, ch)
	if !slices.Contains(after.FeatureChoices, postureHeron) {
		t.Errorf("the invocation field took %s out of a fighter's picks: %v", postureHeron, after.FeatureChoices)
	}

	// A sheet that shows the swap but was not made from one sent is refused; with the swap sent it is accepted.
	ch = satisfy(t, c, at4, "class:fighter", postureSubclass)
	ch.FeatureSwaps = []PickSwap{{Group: postureFeature, From: postureHeron, To: postureCrow}}
	swapped := mustApply(t, c, at4, ch)
	ch.FeatureSwaps = nil
	wantRefusal(t, CheckLevelUpOf(at4, swapped, ch, c), LevelUpReasonFeatureChoice, "full.feature_choice_keys")
	ch.FeatureSwaps = []PickSwap{{Group: postureFeature, From: postureHeron, To: postureCrow}}
	if err := CheckLevelUpOf(at4, swapped, ch, c); err != nil {
		t.Errorf("the swap that was sent: %v", err)
	}
}

// A swap that gives up an option another pick of the sheet asks for is refused: the pick that is left without its
// prerequisite is the level's problem, not one the sheet already had.
func TestASwapCannotGiveUpTheOptionAnotherPickAsksFor(t *testing.T) {
	t.Parallel()
	c := withOverlay(t, swapPostureOverlay("class:fighter", FeatureSwapLevels{Levels: []int{8}}, FeatureSwapLevels{}))
	at7 := postureFighter(t, c, 7)
	// The sheet holds the Ox in the place of the Bull: it asks for the Heron, which the sheet has.
	at7.FeatureChoices = slices.Clone(at7.FeatureChoices)
	if i := slices.Index(at7.FeatureChoices, postureBull); i >= 0 {
		at7.FeatureChoices[i] = postureOx
	} else {
		t.Fatalf("picks = %v, want the Bull among them", at7.FeatureChoices)
	}
	ch := satisfy(t, c, at7, "class:fighter", postureSubclass)
	ch.FeatureSwaps = []PickSwap{{Group: postureFeature, From: postureHeron, To: postureHawk}}
	wantRefusal(t, CheckLevelUpOf(at7, mustApply(t, c, at7, ch), ch, c), LevelUpReasonFeatureChoice, "full.feature_choice_keys")
	ch.FeatureSwaps = []PickSwap{{Group: postureFeature, From: postureWolf, To: postureHawk}}
	if err := CheckLevelUpOf(at7, mustApply(t, c, at7, ch), ch, c); err != nil {
		t.Errorf("a swap that leaves the Heron: %v", err)
	}
}

// A feature that offers the options of another feature (from_feature) cannot allow swaps: the refusal says so at the swap levels,
// and not that the feature has no options. A bad or repeated level, or a list together with every level, is refused first.
func TestAFeatureThatOffersAnotherFeaturesOptionsCannotAllowSwaps(t *testing.T) {
	t.Parallel()
	o := fromFeatureOverlay(askPostures(2))
	o.Subclasses[1].Levels[0].Features[0].SwapLevels = FeatureSwapLevels{EveryLevel: true}
	_, err := loadForTest(t).With(o)
	oe, ok := errors.AsType[*OverlayError](err)
	if !ok {
		t.Fatalf("err = %v, want an overlay error", err)
	}
	const field = "subclasses[1].levels[0].features[0].swap_levels"
	if !slices.ContainsFunc(oe.Violations(), func(v *OverlayError) bool {
		return v.Field == field && v.Reason == ReasonValue && strings.Contains(v.Message, "offers the options of another feature cannot allow swaps")
	}) {
		t.Errorf("no violation at %s about the options of another feature: %v", field, err)
	}
}
