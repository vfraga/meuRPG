package rules

import (
	"testing"
)

// A choice is complete when it holds as many picks as a player may make today, the picks the sheet already holds
// counted: the same number for the player and the master, whoever reads it.

// completeViews are the two readers of the same content.
var completeViews = map[string]func(*Content) *Content{
	"a player":   (*Content).ForPlayer,
	"the master": func(c *Content) *Content { return c },
}

func TestAChoiceWhoseSourceIsSwitchedOffAndPoolEmptyIsCompleteWithNoPick(t *testing.T) {
	t.Parallel()
	base := loadForTest(t)
	o := fromFeatureOverlay(askPostures(2))
	o.Off = []string{postureSubclass}
	c := withOverlayOn(t, base, o)
	for name, view := range completeViews {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			v := view(c)
			set := v.Choices(rumorBuild(3))
			ch := optionChoiceOf(t, set, fromFeatureAsk)
			if ch.Picks != 0 || ch.Missing() != 0 || set.PendingCount() != 0 || len(set.ChoiceProblems(true)) != 0 {
				t.Errorf("picks %d, missing %d, pending %d, problems %v; want nothing to pick", ch.Picks, ch.Missing(), set.PendingCount(), set.ChoiceProblems(true))
			}
			due := v.c.namedChoices(rumorBuild(3), v.c.featureGains([]string{fromFeatureAsk}, fromFeatureSubclass).choices)
			if len(due) != 1 || due[0].Choose != 0 {
				t.Errorf("the level-up asks %+v, want the choice with nothing to pick", due)
			}
			// The level that gives the choice, with no pick, is allowed.
			before, after := rumorBuild(2), rumorBuild(3)
			before.HitPoints, after.HitPoints = HitPoints{}, HitPoints{}
			if lerr := checkChoiceProblems(v.Choices(before), v.Choices(after), nil); lerr != nil {
				t.Errorf("the level-up with no pick: %v", lerr)
			}
		})
	}
}

// The pool cut by one option the table switched off, where the choice asks for one more than it lists: every pick the
// pool lists completes it, for the player and for the master.
func TestAChoiceWhosePoolIsCutIsCompleteWithAsManyPicksAsItLists(t *testing.T) {
	t.Parallel()
	base := loadForTest(t)
	const style, guard = "feature:fighter-fighting-style", "option:fighting_style:guarda-firme" + tableSuffix
	styles := base.c.features[style].Options
	o := fromFeatureOverlay(Effect{Type: "choice", Choice: "feature", Count: len(styles) + 1, FromFeature: style})
	o.Options = []TableOption{{TableEntry: TableEntry{Key: guard, NamePT: "Guarda Firme"}, Set: "fighting_style", DescPT: []string{"Texto de teste."}}}
	o.Off = []string{guard}
	c := withOverlayOn(t, base, o)
	for name, view := range completeViews {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			v := view(c)
			bare := optionChoiceOf(t, v.Choices(rumorBuild(3)), fromFeatureAsk)
			if bare.Picks != len(styles) || bare.Missing() != len(styles) {
				t.Fatalf("with no pick: %d picks, missing %d, want %d and %d", bare.Picks, bare.Missing(), len(styles), len(styles))
			}
			set := v.Choices(rumorBuild(3, styles...))
			ch := optionChoiceOf(t, set, fromFeatureAsk)
			if ch.Missing() != 0 || set.PendingCount() != 0 || len(set.ChoiceProblems(true)) != 0 {
				t.Errorf("every listed pick: missing %d, pending %d, problems %v; want it complete", ch.Missing(), set.PendingCount(), set.ChoiceProblems(true))
			}
		})
	}
}

// A tool choice whose filter offers fewer proficiencies than it asks for is complete with the ones it offers.
func TestAToolChoiceWhosePoolIsCutIsCompleteWithTheOnesItOffers(t *testing.T) {
	t.Parallel()
	base := loadForTest(t)
	probe := withOverlayOn(t, base, filterBackgroundWith(Effect{Type: "choice", Choice: "tool", Count: 1, Filter: toolFilter("other")}))
	offered := probe.c.proficiencyKeysOfKinds("other")
	if len(offered) < 2 {
		t.Fatalf("the content has %v of the kind other, want at least two", offered)
	}
	c := withOverlayOn(t, base, filterBackgroundWith(Effect{Type: "choice", Choice: "tool", Count: len(offered) + 1, Filter: toolFilter("other")}))
	for name, view := range completeViews {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			v := view(c)
			var picks []string
			for _, k := range offered {
				picks = append(picks, ScopedChoice(filterPickKey(), k))
			}
			b := filterBuild()
			b.FeatureChoices = picks
			ch := choiceOf(t, v.Choices(b), filterPickKey())
			if ch.Picks != len(offered) || ch.Missing() != 0 {
				t.Errorf("%d picks, missing %d, want %d and none", ch.Picks, ch.Missing(), len(offered))
			}
			if got := choiceOf(t, v.Choices(filterBuild()), filterPickKey()); got.Picks != len(offered) {
				t.Errorf("with no pick: %d picks, want %d", got.Picks, len(offered))
			}
		})
	}
}
