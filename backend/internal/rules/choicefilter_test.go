package rules

import (
	"errors"
	"slices"
	"strings"
	"testing"
)

// A choice of tools that offers every proficiency of some kinds (the filter), and one whose own list names a weapon, a
// vehicle or a kit. Every name here is invented.

const filterBackground = "background:cais-do-farol" + tableSuffix

// filterBackgroundWith is a table background whose feature carries one choice effect.
func filterBackgroundWith(e Effect) Overlay {
	return Overlay{Revision: 1, Backgrounds: []TableBackground{{
		TableEntry: TableEntry{Key: filterBackground, NamePT: "Cais do farol"},
		Skills:     []string{"skill:athletics", "skill:history"},
		Feature: TableFeature{
			Key: "background-feature:rede-do-cais" + tableSuffix, NamePT: "Rede do cais",
			DescPT: []string{"Texto de teste."}, Effects: []Effect{e},
		},
	}}}
}

func filterBuild() Build {
	b := pickBuild("race:human", "")
	b.Background = filterBackground
	b.Languages = []string{"Anão"}
	return b
}

func filterPickKey() string { return ToolPickKey("background-feature:rede-do-cais" + tableSuffix) }

func toolFilter(kinds ...string) *ChoiceFilter { return &ChoiceFilter{ProficiencyKinds: kinds} }

func sortedOptionKeys(ch Choice) []string {
	out := optionKeys(ch)
	slices.Sort(out)
	return out
}

func TestAFilterOffersEveryProficiencyOfItsKinds(t *testing.T) {
	t.Parallel()
	base := loadForTest(t)
	cases := map[string][]string{
		"weapons":               {"weapon"},
		"tools":                 {"tool"},
		"the vehicles and kits": {"other"},
		"tools and kits":        {"tool", "other"},
		"all three":             {"weapon", "tool", "other"},
	}
	for name, kinds := range cases {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			c := withOverlayOn(t, base, filterBackgroundWith(Effect{Type: "choice", Choice: "tool", Count: 2, Filter: toolFilter(kinds...)}))
			ch := choiceOf(t, c.Choices(filterBuild()), filterPickKey())
			want := c.c.proficiencyKeysOfKinds(kinds...)
			if got := sortedOptionKeys(ch); !slices.Equal(got, want) || ch.Picks != 2 {
				t.Errorf("%d options for %d picks, want the %d proficiencies of %v for 2", len(got), ch.Picks, len(want), kinds)
			}
		})
	}
}

func TestAPickOfAKindTheFilterNamesGivesTheProficiency(t *testing.T) {
	t.Parallel()
	c := withOverlayOn(t, loadForTest(t), filterBackgroundWith(Effect{Type: "choice", Choice: "tool", Count: 1, Filter: toolFilter("weapon", "other")}))
	b := filterBuild()
	for _, key := range []string{"proficiency:glaives", "proficiency:land-vehicles"} {
		b.FeatureChoices = []string{ScopedChoice(filterPickKey(), key)}
		if got := c.Choices(b).NotOffered; len(got) != 0 {
			t.Errorf("%s: not offered = %v, want it taken", key, got)
		}
		d := Derive(b, c)
		if !slices.ContainsFunc(d.Proficiencies, func(p Proficiency) bool { return p.Key == key }) {
			t.Errorf("%s: the sheet's proficiencies %v lack it", key, d.Proficiencies)
		}
		if openOf(d, OpenChoiceTools) != 0 {
			t.Errorf("%s: a tool pick is still open", key)
		}
	}
}

// A pick of a proficiency outside the filter's kinds answers as a key the content does not have.
func TestAPickOutsideTheFilterAnswersAsAKeyThatDoesNotExist(t *testing.T) {
	t.Parallel()
	c := withOverlayOn(t, loadForTest(t), filterBackgroundWith(Effect{Type: "choice", Choice: "tool", Count: 1, Filter: toolFilter("weapon")}))
	answer := func(key string) (ChoiceSet, []Issue) {
		b := filterBuild()
		b.FeatureChoices = []string{ScopedChoice(filterPickKey(), key)}
		return c.Choices(b), Derive(b, c).Issues
	}
	for _, key := range []string{"proficiency:thieves-tools", "proficiency:light-armor", "proficiency:skill-arcana"} {
		set, issues := answer(key)
		want, wantIssues := answer("proficiency:nao-existe")
		got := strings.ReplaceAll(jsonText(set), key, "K")
		if exp := strings.ReplaceAll(jsonText(want), "proficiency:nao-existe", "K"); got != exp {
			t.Errorf("%s: the choices differ from those of a key that does not exist:\n%s\n%s", key, got, exp)
		}
		if len(issues) != len(wantIssues) {
			t.Errorf("%s: %d issues, a key that does not exist has %d", key, len(issues), len(wantIssues))
		}
	}
}

// A list of a tool choice may name a weapon, a vehicle or a kit; a choice with no list still offers the tools alone.
func TestAToolListMayNameWeaponsVehiclesAndKits(t *testing.T) {
	t.Parallel()
	base := loadForTest(t)
	from := []string{"proficiency:thieves-tools", "proficiency:land-vehicles", "proficiency:glaives", "proficiency:smiths-tools"}
	c := withOverlayOn(t, base, filterBackgroundWith(Effect{Type: "choice", Choice: "tool", Count: 1, From: from}))
	if got := sortedOptionKeys(choiceOf(t, c.Choices(filterBuild()), filterPickKey())); !slices.Equal(got, []string{
		"proficiency:glaives", "proficiency:land-vehicles", "proficiency:smiths-tools", "proficiency:thieves-tools",
	}) {
		t.Errorf("options = %v, want the four listed", got)
	}
	b := filterBuild()
	b.FeatureChoices = []string{ScopedChoice(filterPickKey(), "proficiency:thieves-tools")}
	if d := Derive(b, c); openOf(d, OpenChoiceTools) != 0 || !slices.ContainsFunc(d.Proficiencies, func(p Proficiency) bool { return p.Key == "proficiency:thieves-tools" }) {
		t.Errorf("open %d, proficiencies %v; want the thieves' tools taken", openOf(d, OpenChoiceTools), d.Proficiencies)
	}

	open := withOverlayOn(t, base, filterBackgroundWith(Effect{Type: "choice", Choice: "tool", Count: 1}))
	keys := sortedOptionKeys(choiceOf(t, open.Choices(filterBuild()), filterPickKey()))
	for _, k := range keys {
		if open.c.proficiencies[k].Kind != "tool" {
			t.Errorf("a choice with no list offers %s of kind %s", k, open.c.proficiencies[k].Kind)
		}
	}
}

func TestAFilterThatDoesNotFitItsChoiceIsRefused(t *testing.T) {
	t.Parallel()
	base := loadForTest(t)
	cases := map[string]Effect{
		"on a skill":              {Type: "choice", Choice: "skill", Count: 1, Filter: toolFilter("tool")},
		"on a language":           {Type: "choice", Choice: "language", Count: 1, Filter: toolFilter("tool")},
		"with a list":             {Type: "choice", Choice: "tool", Count: 1, From: []string{"proficiency:smiths-tools"}, Filter: toolFilter("tool")},
		"with a feature":          {Type: "choice", Choice: "tool", Count: 1, Filter: toolFilter("tool"), FromFeature: "feature:fighter-fighting-style"},
		"with no kind":            {Type: "choice", Choice: "tool", Count: 1, Filter: &ChoiceFilter{}},
		"with an armor kind":      {Type: "choice", Choice: "tool", Count: 1, Filter: toolFilter("armor")},
		"with a kind twice":       {Type: "choice", Choice: "tool", Count: 1, Filter: toolFilter("tool", "tool")},
		"with a daily choice":     {Type: "choice", Choice: "tool", Count: 1, Filter: toolFilter("tool"), DailyChoice: true, Duration: "short_rest"},
		"with a feature offering": {Type: "choice", Choice: "feature", Count: 1, Filter: toolFilter("tool")},
	}
	for name, e := range cases {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			_, err := base.With(filterBackgroundWith(e))
			var oe *OverlayError
			if !errors.As(err, &oe) {
				t.Fatalf("With: %v, want an overlay error", err)
			}
			if !slices.ContainsFunc(oe.Violations(), func(v *OverlayError) bool {
				return strings.Contains(v.Field, ".filter") || strings.Contains(v.Field, ".from_feature")
			}) {
				t.Errorf("no violation at the filter: %v", err)
			}
		})
	}
}

// An explicit list of a choice of tools names proficiencies of the kinds tool, weapon and other alone: the master's write
// is refused for any other key, and a stored entry that holds one loads as it always did.
func TestAToolListRefusesAProficiencyOfAnotherKindInAWrite(t *testing.T) {
	t.Parallel()
	base := loadForTest(t)
	for _, key := range []string{"proficiency:light-armor", "proficiency:heavy-armor", "proficiency:shields"} {
		if p := base.c.proficiencies[key]; p == nil || slices.Contains(filterKinds, p.Kind) {
			t.Fatalf("%s is %+v, want a proficiency of another kind than the filter's", key, p)
		}
	}
	for _, key := range []string{"skill:athletics", "language:common", "proficiency:light-armor", "saving-throw:strength", "proficiency:nenhuma"} {
		t.Run(key, func(t *testing.T) {
			t.Parallel()
			o := filterBackgroundWith(Effect{Type: "choice", Choice: "tool", Count: 1, From: []string{"proficiency:smiths-tools", key}})
			o.Strict = []string{filterBackground}
			_, err := base.With(o)
			var oe *OverlayError
			if !errors.As(err, &oe) || !slices.ContainsFunc(oe.Violations(), func(v *OverlayError) bool { return strings.HasSuffix(v.Field, ".from") }) {
				t.Errorf("With: %v, want %s refused at from", err, key)
			}
		})
	}
}

// An entry stored with a key of another kind in a tool choice's list loads, as it did before the list was checked by kind:
// the pool leaves the key out and the content says so for the master.
func TestAStoredToolListThatNamesAKeyOfAnotherKindLoads(t *testing.T) {
	t.Parallel()
	base := loadForTest(t)
	for _, key := range []string{"skill:athletics", "language:common", "proficiency:light-armor"} {
		t.Run(key, func(t *testing.T) {
			t.Parallel()
			c, err := base.With(filterBackgroundWith(Effect{Type: "choice", Choice: "tool", Count: 1, From: []string{"proficiency:smiths-tools", key}}))
			if err != nil {
				t.Fatalf("With: %v, want the stored entry to load", err)
			}
			if got := optionKeys(choiceOf(t, c.Choices(filterBuild()), filterPickKey())); !slices.Equal(got, []string{"proficiency:smiths-tools"}) {
				t.Errorf("the pool is %v, want the listed tool alone", got)
			}
			issues := c.StoredIssues()
			if len(issues) != 1 || !strings.HasSuffix(issues[0].Field, ".effects[0].from[1]") || issues[0].Reason != ReasonValue {
				t.Errorf("issues = %+v, want one at the list", issues)
			}
		})
	}
}

// A choice of tools asks for no more than the sheet can make, in the choice and in the sheet's open choices alike: two
// picks of a list of two, one of which the sheet has already, are complete at one.
func TestAToolChoiceWithOneProficiencyLeftIsCompleteAtOnePick(t *testing.T) {
	t.Parallel()
	const had, left = "proficiency:martial-weapons", "proficiency:thieves-tools" // the fighter has the first
	c := withOverlayOn(t, loadForTest(t), filterBackgroundWith(Effect{Type: "choice", Choice: "tool", Count: 2, From: []string{had, left}}))
	b := filterBuild()
	if ch := choiceOf(t, c.Choices(b), filterPickKey()); ch.Picks != 1 {
		t.Fatalf("the choice asks %d picks, want 1: the sheet has %s", ch.Picks, had)
	}
	if d := Derive(b, c); openOf(d, OpenChoiceTools) != 1 {
		t.Errorf("open tool picks = %d with none made, want 1", openOf(d, OpenChoiceTools))
	}
	b.FeatureChoices = []string{ScopedChoice(filterPickKey(), left)}
	d := Derive(b, c)
	if n := openOf(d, OpenChoiceTools); n != 0 {
		t.Errorf("open tool picks = %d with the one pick made, want 0", n)
	}
	for _, is := range d.Issues {
		if strings.Contains(is.Message, "ferramenta") {
			t.Errorf("issue %q about tools, want none", is.Message)
		}
	}
	if n := picksPending(c.Choices(b)); n != 0 {
		t.Errorf("pending tool picks = %d, want none", n)
	}
}
