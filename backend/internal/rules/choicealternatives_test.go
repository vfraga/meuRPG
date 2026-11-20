package rules

import (
	"encoding/json"
	"errors"
	"regexp"
	"slices"
	"strings"
	"testing"
)

// A choice "one of these" that mixes kinds: a skill or a language, a skill or a tool. Every name here is invented.

var (
	altOwner = "background-feature:rede-do-cais" + tableSuffix
	// skillOrLanguage lets the player pick two, of two skills and two languages.
	skillOrLanguage = Effect{Type: "choice", Choice: choiceAlternatives, Count: 2, Alternatives: []ChoiceAlternative{
		{Kind: altSkill, From: []string{"skill:perception", "skill:stealth"}},
		{Kind: altLanguage, From: []string{"language:dwarvish", "language:elvish"}},
	}}
)

func altBuild(picks ...string) Build {
	b := pickBuild("race:human", "")
	b.Background = filterBackground
	b.FeatureChoices = picks
	return b
}

func altKey(e Effect) string { return AlternativesKey(altOwner, &e) }

func altStored(e Effect, value string) string { return ScopedChoice(altKey(e), value) }

func hasSkill(d Derived, key string) bool {
	return slices.ContainsFunc(d.Skills, func(s Skill) bool { return s.Key == key && s.Proficiency >= ProficiencyFull })
}

func missingIssues(d Derived) int {
	n := 0
	for _, is := range d.Issues {
		if is.Code == IssueMissing && regexp.MustCompile(`^Falta escolher \d+: `).MatchString(is.Message) {
			n++
		}
	}
	return n
}

func TestASkillOrLanguageChoiceOffersBothKinds(t *testing.T) {
	t.Parallel()
	c := withOverlayOn(t, loadForTest(t), filterBackgroundWith(skillOrLanguage))
	ch := choiceOf(t, c.Choices(altBuild()), altKey(skillOrLanguage))
	if got, want := sortedOptionKeys(ch), []string{"language:dwarvish", "language:elvish", "skill:perception", "skill:stealth"}; !slices.Equal(got, want) {
		t.Fatalf("options = %v, want %v", got, want)
	}
	if ch.Picks != 2 || ch.Advisory {
		t.Errorf("picks %d, advisory %v; want two picks asked", ch.Picks, ch.Advisory)
	}
	for _, o := range ch.Options {
		kind := map[bool]string{true: "Perícia.", false: "Idioma."}[strings.HasPrefix(o.Key, "skill:")]
		if o.NamePT == "" || o.SummaryPT != kind || o.Stored != altStored(skillOrLanguage, o.Key) {
			t.Errorf("option %+v: want a name, the kind %q and the stored pick", o, kind)
		}
	}
	if ch.TitlePT != "Perícia ou idioma" {
		t.Errorf("title %q", ch.TitlePT)
	}
}

func TestAPickOfEachKindIsAcceptedAndCounted(t *testing.T) {
	t.Parallel()
	c := withOverlayOn(t, loadForTest(t), filterBackgroundWith(skillOrLanguage))
	open := Derive(altBuild(), c)
	if missingIssues(open) != 1 {
		t.Fatalf("the sheet with no pick has %d missing-pick issues, want 1", missingIssues(open))
	}
	one := altBuild(altStored(skillOrLanguage, "skill:stealth"))
	d := Derive(one, c)
	set := c.Choices(one)
	if !hasSkill(d, "skill:stealth") || len(set.NotOffered) != 0 {
		t.Errorf("the skill pick: proficient %v, not offered %v", hasSkill(d, "skill:stealth"), set.NotOffered)
	}
	if got := choiceOf(t, set, altKey(skillOrLanguage)); got.Missing() != 1 || missingIssues(d) != 1 {
		t.Errorf("one pick of two: the choice misses %d, the sheet has %d issues; want 1 and 1", got.Missing(), missingIssues(d))
	}
	both := altBuild(altStored(skillOrLanguage, "skill:stealth"), altStored(skillOrLanguage, "language:elvish"))
	d = Derive(both, c)
	if !slices.Contains(languageKeys(d), "language:elvish") || !hasSkill(d, "skill:stealth") {
		t.Errorf("a skill and a language: languages %v, stealth %v", languageKeys(d), hasSkill(d, "skill:stealth"))
	}
	if got := choiceOf(t, c.Choices(both), altKey(skillOrLanguage)); got.Missing() != 0 || missingIssues(d) != 0 {
		t.Errorf("both picks made: the choice misses %d, the sheet has %d issues", got.Missing(), missingIssues(d))
	}
	// A third pick is past the count and gives nothing.
	over := altBuild(altStored(skillOrLanguage, "skill:stealth"), altStored(skillOrLanguage, "language:elvish"), altStored(skillOrLanguage, "language:dwarvish"))
	if d = Derive(over, c); slices.Contains(languageKeys(d), "language:dwarvish") {
		t.Errorf("a pick past the count gave the language: %v", languageKeys(d))
	}
}

func TestAPickOutsideEveryAlternativeIsAKeyThatDoesNotExist(t *testing.T) {
	t.Parallel()
	c := withOverlayOn(t, loadForTest(t), filterBackgroundWith(skillOrLanguage))
	key := altKey(skillOrLanguage)
	for _, value := range []string{"skill:arcana", "language:giant", "proficiency:disguise-kit", "skill:nao-existe"} {
		if c.c.choiceValueOK(ScopedChoice(key, value)) {
			t.Errorf("a pick of %s is carried: no alternative offers it", value)
		}
	}
	for _, value := range []string{"skill:stealth", "language:elvish"} {
		if !c.c.choiceValueOK(ScopedChoice(key, value)) {
			t.Errorf("a pick of %s is refused: an alternative offers it", value)
		}
	}
	// The choice the master edited since: its picks count for nothing and the choice asks again.
	edited := skillOrLanguage
	edited.Alternatives = slices.Clone(skillOrLanguage.Alternatives)
	edited.Alternatives[0].From = []string{"skill:perception", "skill:arcana"}
	stale := altBuild(altStored(skillOrLanguage, "skill:stealth"))
	ce := withOverlayOn(t, loadForTest(t), filterBackgroundWith(edited))
	if d := Derive(stale, ce); hasSkill(d, "skill:stealth") || missingIssues(d) != 1 {
		t.Errorf("a pick of the choice before its edit: stealth %v, missing issues %d; want none and 1", hasSkill(d, "skill:stealth"), missingIssues(d))
	}
}

// A skill picked through an alternative is no pick of the class's or the other sources' lists: it takes none of their places,
// and the sheet whose class list holds the same skill is still placed.
func TestASkillPickedThroughAnAlternativeTakesNoPlaceOfAnotherSource(t *testing.T) {
	t.Parallel()
	c := withOverlayOn(t, loadForTest(t), filterBackgroundWith(skillOrLanguage))
	b := altBuild(altStored(skillOrLanguage, "skill:perception"), altStored(skillOrLanguage, "language:elvish"))
	b.SkillProficiencies = []string{"skill:survival", "skill:insight"} // two of the fighter's list; perception is in it too
	d := Derive(b, c)
	if !hasSkill(d, "skill:perception") || !hasSkill(d, "skill:survival") || !hasSkill(d, "skill:insight") {
		t.Errorf("the sheet lacks one of perception, survival and insight")
	}
	if openOf(d, OpenChoiceSkills) != 0 || len(fitIssues(d)) != 0 {
		t.Errorf("open skills %d, fit issues %v; want the class's two places filled", openOf(d, OpenChoiceSkills), fitIssues(d))
	}
	if f, k, found := c.UnplacedNewPick(nil, b); found {
		t.Errorf("the skill picks are refused: %s %s", f, k)
	}
	// The same skill in the class's picks is a skill the sheet has: the alternative does not offer it again.
	b.SkillProficiencies = []string{"skill:perception", "skill:insight"}
	b.FeatureChoices = nil
	ch := choiceOf(t, c.Choices(b), altKey(skillOrLanguage))
	if o := optionOf(t, ch, "skill:perception"); !o.Blocked() {
		t.Errorf("perception, which the class's picks hold, is offered again")
	}
	if ch.Picks != 2 {
		t.Errorf("picks = %d, want 2 (three keys are still to take)", ch.Picks)
	}
}

func TestAToolAlternativeTakesAFilter(t *testing.T) {
	t.Parallel()
	e := Effect{Type: "choice", Choice: choiceAlternatives, Count: 1, Alternatives: []ChoiceAlternative{
		{Kind: altSkill, From: []string{"skill:stealth", "skill:deception"}},
		{Kind: altTool, Filter: toolFilter("tool")},
	}}
	c := withOverlayOn(t, loadForTest(t), filterBackgroundWith(e))
	ch := choiceOf(t, c.Choices(altBuild()), altKey(e))
	want := append([]string{"skill:deception", "skill:stealth"}, c.c.proficiencyKeysOfKinds("tool")...)
	slices.Sort(want)
	if got := sortedOptionKeys(ch); !slices.Equal(got, want) {
		t.Fatalf("%d options, want the 2 skills and the %d tools", len(got), len(want)-2)
	}
	b := altBuild(altStored(e, "proficiency:disguise-kit"))
	d := Derive(b, c)
	if !slices.ContainsFunc(d.Proficiencies, func(p Proficiency) bool { return p.Key == "proficiency:disguise-kit" }) {
		t.Errorf("the tool pick gave no proficiency: %v", d.Proficiencies)
	}
	if got := c.Choices(b); len(got.NotOffered) != 0 || choiceOf(t, got, altKey(e)).Missing() != 0 || missingIssues(d) != 0 || openOf(d, OpenChoiceTools) != 0 {
		t.Errorf("the tool pick is not counted: not offered %v, open tools %d, missing %d, issues %d %v", got.NotOffered, openOf(d, OpenChoiceTools), choiceOf(t, got, altKey(e)).Missing(), missingIssues(d), d.Issues)
	}
	// A tool the filter leaves out is no key a pick may carry.
	if c.c.choiceValueOK(altStored(e, "proficiency:glaives")) {
		t.Error("a tool outside the filter's kinds is carried")
	}
}

// A tool picked through an alternative goes through the matching of the owner's tool picks, with the picks of its plain
// tool choice: each choice keeps its own count, and neither asks the other's picks.
func TestAToolAlternativeAndAPlainToolChoiceKeepTheirOwnCounts(t *testing.T) {
	t.Parallel()
	alt := Effect{Type: "choice", Choice: choiceAlternatives, Count: 1, Alternatives: []ChoiceAlternative{
		{Kind: altSkill, From: []string{"skill:stealth", "skill:deception"}},
		{Kind: altTool, Filter: toolFilter("weapon")},
	}}
	plain := Effect{Type: "choice", Choice: "tool", Count: 1, Filter: toolFilter("weapon")}
	o := filterBackgroundWith(alt)
	o.Backgrounds[0].Feature.Effects = append(o.Backgrounds[0].Feature.Effects, plain)
	c := withOverlayOn(t, loadForTest(t), o)
	b := altBuild(altStored(alt, "proficiency:glaives"))
	if d := Derive(b, c); openOf(d, OpenChoiceTools) != 1 || missingIssues(d) != 0 {
		t.Errorf("the alternative's pick answers the plain choice: open tools %d, alternatives issues %d; want 1 and 0", openOf(d, OpenChoiceTools), missingIssues(d))
	}
	b.FeatureChoices = append(b.FeatureChoices, ScopedChoice(filterPickKey(), "proficiency:halberds"))
	d := Derive(b, c)
	if openOf(d, OpenChoiceTools) != 0 || missingIssues(d) != 0 {
		t.Errorf("both made: open tools %d, alternatives issues %d", openOf(d, OpenChoiceTools), missingIssues(d))
	}
	for _, k := range []string{"proficiency:glaives", "proficiency:halberds"} {
		if !slices.ContainsFunc(d.Proficiencies, func(p Proficiency) bool { return p.Key == k }) {
			t.Errorf("the sheet lacks %s", k)
		}
	}
	// The alternative's skill is no place of the plain tool choice.
	if d := Derive(altBuild(altStored(alt, "skill:stealth")), c); openOf(d, OpenChoiceTools) != 1 || !hasSkill(d, "skill:stealth") {
		t.Errorf("a skill pick: open tools %d, stealth %v; want 1 and true", openOf(d, OpenChoiceTools), hasSkill(d, "skill:stealth"))
	}
}

func TestAChoiceOfAlternativesThatDoesNotFitIsRefused(t *testing.T) {
	t.Parallel()
	base := loadForTest(t)
	good := []ChoiceAlternative{{Kind: altSkill, From: []string{"skill:stealth"}}, {Kind: altLanguage, From: []string{"language:elvish"}}}
	with := func(f func(e *Effect)) Effect {
		e := Effect{Type: "choice", Choice: choiceAlternatives, Count: 1, Alternatives: slices.Clone(good)}
		f(&e)
		return e
	}
	cases := map[string]struct {
		e     Effect
		field string
	}{
		"with a list":              {with(func(e *Effect) { e.From = []string{"skill:stealth"} }), ".alternatives"},
		"with a set":               {with(func(e *Effect) { e.Set = "fighting_style" }), ".alternatives"},
		"with a filter":            {with(func(e *Effect) { e.Filter = toolFilter("tool") }), ".alternatives"},
		"with a feature":           {with(func(e *Effect) { e.FromFeature = "feature:fighter-fighting-style" }), ".alternatives"},
		"with a daily choice":      {with(func(e *Effect) { e.DailyChoice, e.Duration = true, DurationShortRest }), ".daily_choice"},
		"with one alternative":     {with(func(e *Effect) { e.Alternatives = good[:1] }), ".alternatives"},
		"with none":                {with(func(e *Effect) { e.Alternatives = nil }), ".alternatives"},
		"with a kind twice":        {with(func(e *Effect) { e.Alternatives = []ChoiceAlternative{good[0], good[0]} }), ".alternatives[1].kind"},
		"with an unknown kind":     {with(func(e *Effect) { e.Alternatives[1].Kind = "spell" }), ".alternatives[1].kind"},
		"with no keys":             {with(func(e *Effect) { e.Alternatives[1].From = nil }), ".alternatives[1].from"},
		"with a skill that is not": {with(func(e *Effect) { e.Alternatives[0].From = []string{"skill:nao-existe"} }), ".alternatives[0].from"},
		"with a language as skill": {with(func(e *Effect) { e.Alternatives[0].From = []string{"language:elvish"} }), ".alternatives[0].from"},
		"with a key twice":         {with(func(e *Effect) { e.Alternatives[0].From = []string{"skill:stealth", "skill:stealth"} }), ".alternatives[0].from"},
		"with a filter on a skill": {with(func(e *Effect) { e.Alternatives[0].Filter = toolFilter("tool") }), ".alternatives[0].filter"},
		"with a filter and a list": {with(func(e *Effect) {
			e.Alternatives = []ChoiceAlternative{good[0], {Kind: altTool, From: []string{"proficiency:glaives"}, Filter: toolFilter("tool")}}
		}), ".alternatives[1].filter"},
		"with a filter of no kind": {with(func(e *Effect) {
			e.Alternatives = []ChoiceAlternative{good[0], {Kind: altTool, Filter: &ChoiceFilter{}}}
		}), ".alternatives[1].filter.proficiency_kinds"},
		"with an armor in tools": {with(func(e *Effect) {
			e.Alternatives = []ChoiceAlternative{good[0], {Kind: altTool, From: []string{"proficiency:light-armor"}}}
		}), ".alternatives[1].from"},
		"with a count too high":          {with(func(e *Effect) { e.Count = 3 }), ".count"},
		"alternatives on a skill choice": {Effect{Type: "choice", Choice: "skill", Count: 1, From: []string{"skill:stealth"}, Alternatives: good}, ".alternatives"},
	}
	for name, tc := range cases {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			_, err := base.With(filterBackgroundWith(tc.e))
			var oe *OverlayError
			if !errors.As(err, &oe) {
				t.Fatalf("With: %v, want an overlay error", err)
			}
			if !slices.ContainsFunc(oe.Violations(), func(v *OverlayError) bool { return v.Reason == ReasonValue || v.Reason == ReasonReference }) ||
				!slices.ContainsFunc(oe.Violations(), func(v *OverlayError) bool { return strings.HasSuffix(v.Field, tc.field) }) {
				t.Errorf("no violation at %s: %v", tc.field, err)
			}
		})
	}
	// Two of the same choice share one key: the second is refused.
	twice := Overlay{Revision: 1, Backgrounds: []TableBackground{filterBackgroundWith(good2(good)).Backgrounds[0]}}
	twice.Backgrounds[0].Feature.Effects = append(twice.Backgrounds[0].Feature.Effects, twice.Backgrounds[0].Feature.Effects[0])
	if _, err := base.With(twice); err == nil || !strings.Contains(err.Error(), "alternatives") {
		t.Errorf("the same choice twice: %v, want a refusal at the second", err)
	}
}

func good2(alts []ChoiceAlternative) Effect {
	return Effect{Type: "choice", Choice: choiceAlternatives, Count: 1, Alternatives: alts}
}

// The signature follows the choice, not the order of its keys or of its alternatives.
func TestTheSignatureOfAChoiceOfAlternativesIgnoresOrder(t *testing.T) {
	t.Parallel()
	a := Effect{Count: 1, Alternatives: []ChoiceAlternative{{Kind: altSkill, From: []string{"skill:a", "skill:b"}}, {Kind: altLanguage, From: []string{"language:x"}}}}
	b := Effect{Count: 1, Alternatives: []ChoiceAlternative{{Kind: altLanguage, From: []string{"language:x"}}, {Kind: altSkill, From: []string{"skill:b", "skill:a"}}}}
	if alternativesSignature(&a) != alternativesSignature(&b) {
		t.Error("the same choice in another order has another signature")
	}
	b.Count = 2
	if alternativesSignature(&a) == alternativesSignature(&b) {
		t.Error("another count has the same signature")
	}
}

// The overlay that carries a choice of alternatives is the same after its JSON round trip, and the choice reads the same.
func TestAChoiceOfAlternativesIsCarriedByTheOverlay(t *testing.T) {
	t.Parallel()
	base := loadForTest(t)
	o := filterBackgroundWith(skillOrLanguage)
	raw, err := json.Marshal(o)
	if err != nil {
		t.Fatal(err)
	}
	var back Overlay
	if err := json.Unmarshal(raw, &back); err != nil {
		t.Fatal(err)
	}
	c := withOverlayOn(t, base, back)
	if got := sortedOptionKeys(choiceOf(t, c.Choices(altBuild()), altKey(skillOrLanguage))); len(got) != 4 {
		t.Errorf("the options after the round trip = %v, want four", got)
	}
}

// A pick that repeats what another source gives the sheet is one the character would have twice: the option is blocked, a
// held pick on it is unmet, and the sheet does not count it. The choice's own picks are not "another source".
func TestAPickOfWhatAnotherSourceGivesIsRefused(t *testing.T) {
	t.Parallel()
	toolsOrSkill := Effect{Type: "choice", Choice: choiceAlternatives, Count: 1, Alternatives: []ChoiceAlternative{
		{Kind: altSkill, From: []string{"skill:perception", "skill:stealth"}},
		{Kind: altTool, From: []string{"proficiency:disguise-kit", "proficiency:herbalism-kit"}},
	}}
	skillOrLanguage := Effect{Type: "choice", Choice: choiceAlternatives, Count: 1, Alternatives: skillOrLanguage.Alternatives}
	rows := []struct {
		name   string
		effect Effect
		build  func() Build
		value  string
		other  string
	}{
		{"a race language", skillOrLanguage, func() Build {
			b := altBuild()
			b.Race = "race:dwarf"
			return b
		}, "language:dwarvish", "skill:stealth"},
		{"a class skill", skillOrLanguage, func() Build {
			b := altBuild()
			b.SkillProficiencies = []string{"skill:perception", "skill:insight"}
			return b
		}, "skill:perception", "skill:stealth"},
		{"a background tool", toolsOrSkill, func() Build {
			b := altBuild()
			return b
		}, "proficiency:disguise-kit", "skill:stealth"},
	}
	for _, r := range rows {
		t.Run(r.name, func(t *testing.T) {
			t.Parallel()
			o := filterBackgroundWith(r.effect)
			if r.name == "a background tool" {
				o.Backgrounds[0].Tools = []string{"proficiency:disguise-kit"}
			}
			c := withOverlayOn(t, loadForTest(t), o)
			b := r.build()
			b.FeatureChoices = []string{altStored(r.effect, r.value)}
			set := c.Choices(b)
			ch := choiceOf(t, set, altKey(r.effect))
			if !optionOf(t, ch, r.value).Blocked() {
				t.Errorf("%s: the option the sheet has from another source is offered", r.value)
			}
			if !slices.Contains(ch.Unmet, r.value) {
				t.Errorf("%s: the pick is not unmet (unmet %v)", r.value, ch.Unmet)
			}
			if !slices.ContainsFunc(set.ChoiceProblems(false), func(p ChoiceProblem) bool {
				return p.Code == ChoiceProblemPrerequisite && p.OptionKey == r.value
			}) {
				t.Errorf("%s: the pick is no problem of the save: %v", r.value, set.ChoiceProblems(false))
			}
			// The pick counts for nothing: the choice asks again.
			if d := Derive(b, c); missingIssues(d) != 1 {
				t.Errorf("%s: the sheet counts the pick: missing issues %d, want 1", r.value, missingIssues(d))
			}
			// A pick of another key of the choice is whole, and stays so when the sheet is saved again.
			b.FeatureChoices = []string{altStored(r.effect, r.other)}
			set = c.Choices(b)
			if len(set.ChoiceProblems(false)) != 0 || missingIssues(Derive(b, c)) != 0 {
				t.Errorf("%s: the choice's own pick is refused: %v", r.other, set.ChoiceProblems(false))
			}
		})
	}
}
