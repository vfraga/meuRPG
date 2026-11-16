package rules

import (
	"slices"
	"strings"
	"testing"
)

// A choice of cantrips or of spells that offers the spells of some class lists (the filter), and a choice of spells that
// lists classes (from). Every name here is invented.

const (
	spellFilterClass   = "class:gen-none" + tableSuffix
	spellFilterFeature = "feature:escolha-de-magia" + tableSuffix
	// spellFilterList is a table class that reuses the cleric's spell list.
	spellFilterList = "class:gen-full-prepared" + tableSuffix
	// onlyOnTheList is a table spell that no list but the table class's has.
	onlyOnTheList = "spell:so-na-lista" + tableSuffix
)

// spellFilterOverlay is two table classes, the second of which reuses the cleric's list, and a spell that only that list
// has; the first asks for the choice e at the level.
func spellFilterOverlay(t testing.TB, level int, e Effect) Overlay {
	t.Helper()
	base := loadForTest(t)
	asker, listed := genClass(base, genKinds[0]), genClass(base, genKinds[1])
	asker.Levels[level-1].Features = append(asker.Levels[level-1].Features, tf("escolha-de-magia", "Escolha de magia", e))
	only := tableTestSpells()[0]
	only.Key, only.NamePT, only.Level, only.Classes = onlyOnTheList, "Só na lista", 1, []string{spellFilterList}
	spells := append(tableTestSpells(), only)
	return Overlay{Revision: 3, Classes: []TableClass{asker, listed}, Spells: spells}
}

func spellFilterBuild(level int, picks ...string) Build {
	return choiceBuild("race:human", "", ClassLevel{Class: spellFilterClass, Level: level}, picks...)
}

func spellChoiceOf(t *testing.T, c *Content, level int, picks ...string) Choice {
	t.Helper()
	return choiceOf(t, c.Choices(spellFilterBuild(level, picks...)), spellFilterFeature)
}

func TestAFilterOffersTheSpellsOfItsClassListsCirclesAndSchools(t *testing.T) {
	t.Parallel()
	const evocation = "school:evocation"
	cases := []struct {
		name   string
		effect Effect
		want   SpellFilter // read from the content's own list of spells
		lists  []string
	}{
		{
			"the cantrips of a list",
			Effect{Type: "choice", Choice: "cantrip", Count: 1, Filter: &ChoiceFilter{ClassLists: []string{"class:wizard"}}},
			SpellFilter{Levels: []int{0}},
			[]string{"class:wizard"},
		},
		{
			"the cantrips of two lists",
			Effect{Type: "choice", Choice: "cantrip", Count: 1, Filter: &ChoiceFilter{ClassLists: []string{"class:wizard", "class:cleric"}}},
			SpellFilter{Levels: []int{0}},
			[]string{"class:wizard", "class:cleric"},
		},
		{
			"the cantrips of a school",
			Effect{Type: "choice", Choice: "cantrip", Count: 1, Filter: &ChoiceFilter{ClassLists: []string{"class:wizard"}, Schools: []string{evocation}, SpellLevels: []int{0}}},
			SpellFilter{Levels: []int{0}, Schools: []string{evocation}},
			[]string{"class:wizard"},
		},
		{
			"the rituals of two circles",
			Effect{Type: "choice", Choice: "spell", Count: 2, Filter: &ChoiceFilter{ClassLists: []string{"class:wizard"}, SpellLevels: []int{1, 2}, Ritual: true}},
			SpellFilter{Levels: []int{1, 2}, Ritual: true},
			[]string{"class:wizard"},
		},
		{
			"every circle of a list",
			Effect{Type: "choice", Choice: "spell", Count: 1, Filter: &ChoiceFilter{ClassLists: []string{"class:wizard"}}},
			SpellFilter{Levels: []int{1, 2, 3, 4, 5, 6, 7, 8, 9}},
			[]string{"class:wizard"},
		},
		{
			"the list a table class reuses",
			Effect{Type: "choice", Choice: "spell", Count: 1, Filter: &ChoiceFilter{ClassLists: []string{spellFilterList}}},
			SpellFilter{Levels: []int{1, 2, 3, 4, 5, 6, 7, 8, 9}},
			[]string{spellFilterList},
		},
		{
			"a list of classes with no filter",
			Effect{Type: "choice", Choice: "spell", Count: 1, From: []string{"class:wizard", "class:bard"}},
			SpellFilter{Levels: []int{1, 2, 3, 4, 5, 6, 7, 8, 9}},
			[]string{"class:wizard", "class:bard"},
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			c := withOverlay(t, spellFilterOverlay(t, 1, tc.effect))
			var want []string
			for _, l := range tc.lists {
				f := tc.want
				f.Class = l
				for _, k := range spellKeys(c.ListSpells(f)) {
					if !slices.Contains(want, k) {
						want = append(want, k)
					}
				}
			}
			slices.Sort(want)
			ch := spellChoiceOf(t, c, 1)
			got := optionKeys(ch)
			slices.Sort(got)
			if len(want) == 0 || !slices.Equal(got, want) {
				t.Fatalf("%d options, want the %d spells of %v", len(got), len(want), tc.lists)
			}
			if ch.Kind != ChoiceKindSpells || ch.Picks != min(tc.effect.Count, len(want)) || ch.Advisory {
				t.Errorf("kind %v, %d picks, advisory %v", ch.Kind, ch.Picks, ch.Advisory)
			}
			if tc.want.Ritual {
				for _, o := range ch.Options {
					if !o.Ritual {
						t.Errorf("%s is no ritual", o.Key)
					}
				}
			}
		})
	}
}

// A spell only a table class's list has is offered by that list, the table class that reuses another's list offers that
// list's spells, and a class that casts from none of them offers nothing.
func TestAFilterReadsTheListOfATableClass(t *testing.T) {
	t.Parallel()
	c := withOverlay(t, spellFilterOverlay(t, 1, Effect{Type: "choice", Choice: "spell", Count: 1, Filter: &ChoiceFilter{ClassLists: []string{spellFilterList}, SpellLevels: []int{1}}}))
	got := optionKeys(spellChoiceOf(t, c, 1))
	if !slices.Contains(got, onlyOnTheList) || !slices.Contains(got, "spell:cure-wounds") {
		t.Errorf("options %v lack the table spell of the list or the cleric's cure wounds", got)
	}
	if slices.Contains(got, "spell:magic-missile") {
		t.Error("the options hold a wizard spell that is not on the cleric's list")
	}
}

// A choice of spells with no list of classes asks nothing; a choice of cantrips that lists classes (from) is as it was.
func TestAChoiceOfSpellsWithNoListAsksNothing(t *testing.T) {
	t.Parallel()
	c := withOverlay(t, spellFilterOverlay(t, 1, Effect{Type: "choice", Choice: "spell", Count: 1}))
	for _, g := range c.Choices(spellFilterBuild(1)).Groups {
		for _, ch := range g.Choices {
			if ch.Key == spellFilterFeature {
				t.Errorf("the choice %v is asked with no list", ch.Key)
			}
		}
	}
}

// The reader's pool never holds a spell the table switched off, retired or hid with its list; the master reads what is
// switched off, and what is retired closes for everyone. A pick the sheet already holds is kept.
func TestTheReaderOfASpellFilterIsOfferedWhatTheTableLetsItRead(t *testing.T) {
	t.Parallel()
	filter := &ChoiceFilter{ClassLists: []string{"class:wizard", spellFilterList}, SpellLevels: []int{1, 3}}
	effect := Effect{Type: "choice", Choice: "spell", Count: 1, Filter: filter}
	const off = "spell:fireball"
	offered := func(c *Content, picks ...string) (Choice, ChoiceSet) {
		set := c.Choices(spellFilterBuild(1, picks...))
		return choiceOf(t, set, spellFilterFeature), set
	}
	build := func(edit func(*Overlay)) *Content {
		o := spellFilterOverlay(t, 1, effect)
		edit(&o)
		return withOverlay(t, o)
	}
	rows := []struct {
		name         string
		edit         func(o *Overlay)
		spell        string
		master, play bool // offered to the master, and to a player
	}{
		{"a spell switched off", func(o *Overlay) { o.Off = []string{off} }, off, true, false},
		{"a spell retired", func(o *Overlay) { o.Spells[0].Archived = true }, "spell:raio-de-teste" + tableSuffix, false, false},
		{"the list of a table class switched off", func(o *Overlay) { o.Off = []string{spellFilterList} }, onlyOnTheList, true, false},
		{"the list of a table class retired", func(o *Overlay) { o.Classes[1].Archived = true }, onlyOnTheList, false, false},
		{"nothing", func(*Overlay) {}, onlyOnTheList, true, true},
	}
	for _, tc := range rows {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			c := build(tc.edit)
			ch, _ := offered(c)
			if got := slices.Contains(optionKeys(ch), tc.spell); got != tc.master {
				t.Errorf("the master is offered %s: %v, want %v", tc.spell, got, tc.master)
			}
			ch, _ = offered(c.ForPlayer())
			if got := slices.Contains(optionKeys(ch), tc.spell); got != tc.play {
				t.Errorf("a player is offered %s: %v, want %v", tc.spell, got, tc.play)
			}
			// A sheet that holds the pick keeps it, and the reader's set says it is a pick the pool keeps out.
			stored := ScopedChoice(spellFilterFeature, tc.spell)
			for name, view := range map[string]*Content{"the master": c, "a player": c.ForPlayer()} {
				ch, set := offered(view, stored)
				if !slices.Contains(ch.Picked, tc.spell) || slices.Contains(set.NotOffered, stored) {
					t.Errorf("%s holds %s: picked %v, not offered %v; want the pick kept", name, tc.spell, ch.Picked, set.NotOffered)
				}
				kept := slices.Contains(set.KeptOut, stored)
				if want := (name == "the master" && !tc.master) || (name == "a player" && !tc.play); kept != want {
					t.Errorf("%s holds %s: kept out %v, want %v", name, tc.spell, kept, want)
				}
			}
		})
	}
}

// An empty pool asks nothing: the choice is advisory and takes no pick, as a choice of tools with no tool does.
func TestAnEmptyPoolOfASpellFilterTakesNoPick(t *testing.T) {
	t.Parallel()
	effect := Effect{Type: "choice", Choice: "cantrip", Count: 2, Filter: &ChoiceFilter{ClassLists: []string{spellFilterList}}}
	o := spellFilterOverlay(t, 1, effect)
	o.Off = []string{spellFilterList}
	c := withOverlay(t, o)
	ch := spellChoiceOf(t, c.ForPlayer(), 1)
	if len(ch.Options) != 0 || ch.Picks != 0 || !ch.Advisory {
		t.Errorf("a player: %d options, %d picks, advisory %v; want none, none, advisory", len(ch.Options), ch.Picks, ch.Advisory)
	}
	// The master reads the list: its cantrips are offered, within what the filter allows.
	if got := spellChoiceOf(t, c, 1); got.Picks == 0 && len(got.Options) > 0 {
		t.Errorf("the master has %d options for %d picks", len(got.Options), got.Picks)
	}
}

// The pick of a choice of spells is a spell the sheet knows, besides the class's own numbers.
func TestThePickOfAChoiceOfSpellsIsAGrantedSpell(t *testing.T) {
	t.Parallel()
	c := withOverlay(t, spellFilterOverlay(t, 1, Effect{Type: "choice", Choice: "spell", Count: 1, Filter: &ChoiceFilter{ClassLists: []string{"class:wizard"}, SpellLevels: []int{3}}}))
	b := spellFilterBuild(1, ScopedChoice(spellFilterFeature, "spell:fireball"))
	d := Derive(b, c)
	if !slices.ContainsFunc(d.Spells, func(s CharacterSpell) bool { return s.Spell.Key == "spell:fireball" }) {
		t.Errorf("the sheet's spells %v lack the pick", d.Spells)
	}
	for _, is := range d.Issues {
		if strings.Contains(is.Field, "feature_choice_keys") || strings.Contains(is.Field, "spell_keys") {
			t.Errorf("issue %v about the pick, want none", is)
		}
	}
	if set := c.Choices(b); len(set.NotOffered) != 0 || choiceOf(t, set, spellFilterFeature).Missing() != 0 {
		t.Errorf("not offered %v, missing %d; want the pick taken", set.NotOffered, choiceOf(t, set, spellFilterFeature).Missing())
	}
}

// A pick outside the pool is a pick nothing offers: the set reads it as it reads a spell that does not exist.
func TestAPickOutsideTheSpellPoolReadsAsASpellThatDoesNotExist(t *testing.T) {
	t.Parallel()
	c := withOverlay(t, spellFilterOverlay(t, 1, Effect{Type: "choice", Choice: "spell", Count: 1, Filter: &ChoiceFilter{ClassLists: []string{"class:wizard"}, SpellLevels: []int{3}}}))
	answer := func(spell string) string {
		set := c.Choices(spellFilterBuild(1, ScopedChoice(spellFilterFeature, spell)))
		return strings.ReplaceAll(jsonText(set), spell, "K")
	}
	want := answer("spell:nao-existe")
	for _, spell := range []string{"spell:cure-wounds", "spell:fire-bolt"} {
		if got := answer(spell); got != want {
			t.Errorf("%s: the choices differ from those of a spell that does not exist:\n%s\n%s", spell, got, want)
		}
	}
}

// A level that gives the choice offers it and asks it: it is a new choice, which the level cannot leave open.
func TestALevelThatGivesAChoiceOfSpellsAsksIt(t *testing.T) {
	t.Parallel()
	c := withOverlay(t, spellFilterOverlay(t, 2, Effect{Type: "choice", Choice: "spell", Count: 1, Filter: &ChoiceFilter{ClassLists: []string{"class:wizard"}, SpellLevels: []int{1}}}))
	b := spellFilterBuild(1)
	o, err := LevelUpOptions(b, spellFilterClass, c)
	if err != nil {
		t.Fatal(err)
	}
	if !offersChoice(o.NewChoices, spellFilterFeature) {
		t.Fatalf("the level offers %v, want the choice of spells", o.NewChoices)
	}
	ch := satisfy(t, c, b, spellFilterClass, "")
	if len(ch.LateChoices) != 1 || !strings.HasPrefix(ch.LateChoices[0], spellFilterFeature+"=spell:") {
		t.Fatalf("the picks %v do not answer the choice", ch.LateChoices)
	}
	if err := CheckLevelUp(b, mustApply(t, c, b, ch), c); err != nil {
		t.Fatalf("CheckLevelUp with the pick: %v", err)
	}
	ch.LateChoices = nil
	wantRefusal(t, CheckLevelUp(b, mustApply(t, c, b, ch), c), LevelUpReasonFeatureChoice, "full.feature_choice_keys")
	ch = satisfy(t, c, b, spellFilterClass, "")
	for _, is := range Derive(mustApply(t, c, b, ch), c).Issues {
		if strings.Contains(is.Field, "feature_choice_keys") || strings.Contains(is.Field, "spell_keys") {
			t.Errorf("issue %v after the level, want none about the pick", is)
		}
	}
}

// SpellsOfChoiceFilter is the pool the choices read, for a caller that has its own reader: the lists, circles, schools and
// rituals of the filter, less what the reader's test hides and what the table retired.
func TestSpellsOfChoiceFilterIsThePoolOfAFilter(t *testing.T) {
	t.Parallel()
	o := spellFilterOverlay(t, 1, Effect{Type: "note"})
	o.Off = []string{"spell:fireball"}
	c := withOverlay(t, o)
	f := ChoiceFilter{ClassLists: []string{"class:wizard", spellFilterList}, SpellLevels: []int{3}}
	all := spellKeys(c.SpellsOfChoiceFilter(f, nil))
	if !slices.Contains(all, "spell:fireball") || !slices.Contains(all, "spell:cone-de-teste"+tableSuffix) {
		t.Errorf("the pool %v lacks fireball or the table's cone", all)
	}
	if got := spellKeys(c.SpellsOfChoiceFilter(f, c.Hidden)); slices.Contains(got, "spell:fireball") || len(got) == 0 {
		t.Errorf("the player's pool %v holds the switched-off fireball or nothing", got)
	}
	if got := c.SpellsOfChoiceFilter(ChoiceFilter{SpellLevels: []int{3}}, nil); len(got) != 0 {
		t.Errorf("a filter with no class list has %d spells, want none", len(got))
	}
	if !slices.IsSortedFunc(c.SpellsOfChoiceFilter(f, nil), func(a, b SpellEntry) int { return strings.Compare(foldPT(a.NamePT), foldPT(b.NamePT)) }) {
		t.Error("the pool is not sorted by name")
	}
}

// A sheet that predates the picks holds the cantrip of a filter in its cantrip list: a cantrip of the filter's lists is
// allowed there as a cantrip of a list (from) is, and the pick answers the choice.
func TestACantripOfAFilterMayStandInTheCantripList(t *testing.T) {
	t.Parallel()
	c := withOverlay(t, spellFilterOverlay(t, 1, Effect{Type: "choice", Choice: "cantrip", Count: 1, Filter: &ChoiceFilter{ClassLists: []string{"class:wizard"}}}))
	b := spellFilterBuild(1)
	b.Cantrips = []string{"spell:fire-bolt"}
	for _, is := range Derive(b, c).Issues {
		if strings.Contains(is.Field, "cantrip_keys") {
			t.Errorf("issue %v about the cantrip, want none", is)
		}
	}
	if ch := choiceOf(t, c.Choices(b), spellFilterFeature); ch.Missing() != 0 {
		t.Errorf("the cantrip in the list leaves %d picks open, want none", ch.Missing())
	}
}
