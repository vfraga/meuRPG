package rules

import (
	"errors"
	"slices"
	"strings"
	"testing"
)

// The option lists of the table's features (a table subclass whose player picks
// postures from a list the table wrote). Every name here is invented.

const (
	postureSubclass = "subclass:mestre-das-posturas" + tableSuffix
	postureFeature  = "feature:posturas" + tableSuffix
	postureMore     = "feature:mais-posturas" + tableSuffix
	postureHeron    = "feature:postura-garca" + tableSuffix
	postureBull     = "feature:postura-touro" + tableSuffix
	postureWolf     = "feature:postura-lobo" + tableSuffix
	postureCrow     = "feature:postura-corvo" + tableSuffix
	postureBear     = "feature:postura-urso" + tableSuffix
	postureDice     = "dados_de_postura"
)

func postureOption(key, name string, effects ...Effect) TableFeature {
	return TableFeature{Key: key, NamePT: name, DescPT: []string{"Texto de teste de " + name + "."}, Effects: effects}
}

// postureOverlay is a table subclass of the SRD Fighter whose level 3 feature has
// five invented options (three to pick) and whose level 7 feature asks for two more
// of the same list.
func postureOverlay() Overlay {
	posturas := TableFeature{
		Key: postureFeature, NamePT: "Posturas", DescPT: []string{"Escolha posturas."},
		Effects: []Effect{
			{Type: "resource", Resource: postureDice, Max: "4", Recharge: "short_rest"},
			{Type: "choice", Choice: "feature", Count: 3},
		},
		Options: []TableFeature{
			postureOption(postureHeron, "Postura da Garça", Effect{Type: "grant_action", Economy: "bonus_action", Resource: postureDice}),
			postureOption(postureBull, "Postura do Touro", Effect{Type: "modifier", Target: "initiative", Mode: "add", Value: "2"}),
			postureOption(postureWolf, "Postura do Lobo"),
			postureOption(postureCrow, "Postura do Corvo"),
			postureOption(postureBear, "Postura do Urso"),
		},
	}
	more := tf("mais-posturas", "Mais Posturas", Effect{
		Type: "choice", Choice: "feature", Count: 2,
		From: []string{postureHeron, postureBull, postureWolf, postureCrow, postureBear},
	})
	return Overlay{Revision: 1, Subclasses: []TableSubclass{{
		Key: postureSubclass, NamePT: "Mestre das Posturas", Class: "class:fighter",
		Levels: []TableSubclassLevel{
			{Level: 3, Features: []TableFeature{posturas}},
			{Level: 7, Features: []TableFeature{more}},
		},
	}}}
}

func postureBuild(level int, picks ...string) Build {
	return Build{
		BaseScores: map[Ability]int{STR: 15, DEX: 14, CON: 13, INT: 12, WIS: 10, CHA: 8},
		Race:       "race:human", Background: "background:acolyte", Languages: testLanguages,
		Classes:        []ClassLevel{{Class: "class:fighter", Level: level, Subclass: postureSubclass}},
		FeatureChoices: picks,
	}
}

func optionChoiceOf(t *testing.T, set ChoiceSet, feature string) Choice {
	t.Helper()
	for _, g := range set.Groups {
		for _, ch := range g.Choices {
			if ch.FeatureKey == feature {
				return ch
			}
		}
	}
	t.Fatalf("no choice of %s in %+v", feature, set.Groups)
	return Choice{}
}

func TestOptionListIsOfferedAtLevel3AndAgainAtLevel7(t *testing.T) {
	c := withOverlay(t, postureOverlay())

	first := optionChoiceOf(t, c.Choices(postureBuild(3)), postureFeature)
	if first.Picks != 3 || len(first.Options) != 5 {
		t.Fatalf("level 3 choice = %d picks, %d options; want 3 picks of 5", first.Picks, len(first.Options))
	}
	if first.Options[0].NamePT != "Postura da Garça" && first.Options[0].NamePT != "Garça" {
		// optionLabel cuts "Estilo de Luta: X"; these names have no colon.
		t.Errorf("first option = %q, want its own Portuguese name", first.Options[0].NamePT)
	}
	for _, o := range first.Options {
		if !strings.HasPrefix(o.SummaryPT, "Texto de teste de ") {
			t.Errorf("option %s has summary %q, want its text", o.Key, o.SummaryPT)
		}
		if o.Blocked() {
			t.Errorf("option %s is blocked at level 3: %q", o.Key, o.ReasonPT)
		}
	}

	known := []string{postureHeron, postureBull, postureWolf}
	set := c.Choices(postureBuild(7, known...))
	if got := optionChoiceOf(t, set, postureFeature).Picked; !slices.Equal(got, known) {
		t.Errorf("level 3 picked = %v, want %v", got, known)
	}
	more := optionChoiceOf(t, set, postureMore)
	if more.Picks != 2 {
		t.Fatalf("level 7 choice takes %d, want 2", more.Picks)
	}
	for _, o := range more.Options {
		if want := slices.Contains(known, o.Key); o.Blocked() != want {
			t.Errorf("level 7 option %s blocked = %v, want %v (a known one is not offered again)", o.Key, o.Blocked(), want)
		}
	}
}

func TestPickedOptionAppliesItsEffectsUnderItsParent(t *testing.T) {
	c := withOverlay(t, postureOverlay())
	d := Derive(postureBuild(3, postureHeron, postureBull, postureWolf), c)

	var heron *Feature
	for i := range d.Features {
		if d.Features[i].Key == postureHeron {
			heron = &d.Features[i]
		}
	}
	if heron == nil {
		t.Fatalf("the picked option is not on the sheet: %+v", d.Features)
	}
	if heron.SourcePT == "" || heron.NamePT != "Postura da Garça" {
		t.Errorf("option on the sheet = %+v, want its name under its parent's source", *heron)
	}
	if slices.ContainsFunc(d.Features, func(f Feature) bool { return f.Key == postureCrow }) {
		t.Error("an option nobody picked is on the sheet")
	}
	// The action spends the resource the parent feature defines.
	i := slices.IndexFunc(d.Actions, func(a Action) bool { return a.Key == postureHeron })
	if i < 0 || d.Actions[i].Resource != postureDice || d.Actions[i].Economy != EconomyBonusAction {
		t.Fatalf("actions = %+v, want the heron action spending %s", d.Actions, postureDice)
	}
	if !slices.ContainsFunc(d.Resources, func(r Resource) bool { return r.Key == postureDice && r.Max == 4 }) {
		t.Errorf("resources = %+v, want %s with 4 uses", d.Resources, postureDice)
	}
	// The unpicked heron grants no action.
	none := Derive(postureBuild(3, postureBull), c)
	if slices.ContainsFunc(none.Actions, func(a Action) bool { return a.Key == postureHeron }) {
		t.Error("an option that was not picked grants an action")
	}
}

func TestPickIsDroppedWithItsSubclass(t *testing.T) {
	c := withOverlay(t, postureOverlay())
	b := postureBuild(3, postureHeron)
	b.Classes[0].Subclass = ""
	d := Derive(b, c)
	if slices.ContainsFunc(d.Actions, func(a Action) bool { return a.Key == postureHeron }) {
		t.Error("the action stays after the subclass went")
	}
	if slices.ContainsFunc(d.Features, func(f Feature) bool { return f.Key == postureHeron }) {
		t.Error("the option stays on the sheet after the subclass went")
	}
	if !slices.ContainsFunc(d.Issues, func(i Issue) bool {
		return i.Code == IssueUnknownKey && strings.Contains(i.Field, "feature_choice_keys")
	}) {
		t.Errorf("issues = %+v, want the pick named as no longer valid", d.Issues)
	}
}

func TestOptionListRefusalsNameTheirField(t *testing.T) {
	t.Parallel()
	cases := []struct {
		name  string
		edit  func(o *Overlay)
		field string
	}{
		{"options on a feature without a choice", func(o *Overlay) {
			o.Subclasses[0].Levels[0].Features[0].Effects = o.Subclasses[0].Levels[0].Features[0].Effects[:1]
		}, "subclasses[0].levels[0].features[0].effects"},
		{"a repeated option name", func(o *Overlay) {
			o.Subclasses[0].Levels[0].Features[0].Options[1].NamePT = "Postura da Garça"
		}, "subclasses[0].levels[0].features[0].options[1].name_pt"},
		{"options that nest", func(o *Overlay) {
			o.Subclasses[0].Levels[0].Features[0].Options[0].Options = []TableFeature{postureOption("feature:aninhada"+tableSuffix, "Aninhada")}
		}, "subclasses[0].levels[0].features[0].options[0].options"},
		{"an option that asks for a choice", func(o *Overlay) {
			o.Subclasses[0].Levels[0].Features[0].Options[2].Effects = []Effect{{Type: "choice", Choice: "skill", Count: 1}}
		}, "subclasses[0].levels[0].features[0].options[2].effects[0].type"},
		{"a count beyond the options", func(o *Overlay) {
			o.Subclasses[0].Levels[0].Features[0].Effects[1].Count = 6
		}, "subclasses[0].levels[0].features[0].effects[1].count"},
		{"a repeated option in from", func(o *Overlay) {
			o.Subclasses[0].Levels[1].Features[0].Effects[0].From = []string{postureHeron, postureHeron, postureBull}
		}, "subclasses[0].levels[1].features[0].effects[0].from"},
		{"an option that does not exist", func(o *Overlay) {
			o.Subclasses[0].Levels[1].Features[0].Effects[0].From = []string{postureHeron, "feature:postura-fantasma" + tableSuffix}
		}, "subclasses[0].levels[1].features[0].effects[0].from"},
		{"an action that spends a resource nobody defines", func(o *Overlay) {
			o.Subclasses[0].Levels[0].Features[0].Options[0].Effects[0].Resource = "dados_fantasma"
		}, "subclasses[0].levels[0].features[0].options[0].effects[0].resource"},
		{"options on a background feature", func(o *Overlay) {
			_, _, bg := tableMisc()
			bg.Feature.Options = []TableFeature{postureOption(postureCrow, "Postura do Corvo")}
			o.Backgrounds = []TableBackground{bg}
		}, "backgrounds[0].feature.options"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			o := postureOverlay()
			tc.edit(&o)
			_, err := loadForTest(t).With(o)
			if err == nil {
				t.Fatal("With accepted it")
			}
			var oe *OverlayError
			if !errors.As(err, &oe) {
				t.Fatalf("want *OverlayError, got %T: %v", err, err)
			}
			found := false
			for _, v := range oe.Violations() {
				found = found || v.Field == tc.field
			}
			if !found {
				t.Errorf("no violation at %s: %v", tc.field, err)
			}
		})
	}
}

func TestAnotherEntrysOptionsCannotBeOffered(t *testing.T) {
	o := postureOverlay()
	other := o.Subclasses[0]
	other.Key = "subclass:outro-mestre" + tableSuffix
	other.NamePT = "Outro Mestre"
	other.Levels = []TableSubclassLevel{{Level: 3, Features: []TableFeature{tf("outro-escolhe", "Escolhe", Effect{
		Type: "choice", Choice: "feature", Count: 1, From: []string{postureHeron},
	})}}}
	o.Subclasses = append(o.Subclasses, other)
	_, err := loadForTest(t).With(o)
	var oe *OverlayError
	if !errors.As(err, &oe) {
		t.Fatalf("want an OverlayError, got %v", err)
	}
	if !slices.ContainsFunc(oe.Violations(), func(v *OverlayError) bool { return strings.HasSuffix(v.Field, ".effects[0].from") }) {
		t.Errorf("violations = %+v, want one at effects[0].from", oe.Violations())
	}
}

func TestGrantActionResourceIsAcceptedWhenTheEntryDefinesIt(t *testing.T) {
	// The option's own resource, and the class's for a subclass, both count.
	if _, err := loadForTest(t).With(postureOverlay()); err != nil {
		t.Fatalf("the heron action spending the parent's resource was refused: %v", err)
	}
	menu := withOverlay(t, postureOverlay()).EffectMenu()
	for _, ty := range menu.Types {
		if ty.Type != "grant_action" {
			continue
		}
		if !slices.ContainsFunc(ty.Fields, func(f MenuField) bool { return f.Name == "resource" }) {
			t.Errorf("grant_action fields = %+v, want resource", ty.Fields)
		}
		return
	}
	t.Fatal("no grant_action in the menu")
}

// A table option joins the options of a table feature (the Battle Master's maneuvers of a table
// subclass): `set` is "table_feature" and `offered_by` is the feature's key.
const postureOwn = "option:table_feature:postura-da-raposa" + tableSuffix

func postureFoxOption() TableOption {
	return TableOption{
		TableEntry: TableEntry{Key: postureOwn, NamePT: "Postura da Raposa"}, Set: "table_feature", OfferedBy: postureFeature,
		DescPT:       []string{"Texto de teste da raposa."},
		Prerequisite: TableOptionPrerequisite{ClassKey: "class:fighter", ClassLevel: 5},
		Effects:      []Effect{{Type: "modifier", Target: "initiative", Mode: "add", Value: "3"}},
	}
}

func TestATableOptionJoinsTheOptionsOfATableFeature(t *testing.T) {
	o := postureOverlay()
	o.Options = []TableOption{postureFoxOption()}
	c := withOverlay(t, o)

	ch := optionChoiceOf(t, c.Choices(postureBuild(3)), postureFeature)
	if len(ch.Options) != 6 {
		t.Fatalf("the feature offers %d options, want its 5 and the table option", len(ch.Options))
	}
	// The option asks for level 5 in Fighter: listed and blocked at level 3, free at level 5.
	i := slices.IndexFunc(ch.Options, func(op ChoiceOption) bool { return op.Key == postureOwn })
	if i < 0 || !ch.Options[i].Blocked() {
		t.Fatalf("the option is %v at level 3, want listed and blocked by its level", i)
	}
	at5 := optionChoiceOf(t, c.Choices(postureBuild(5)), postureFeature)
	if j := slices.IndexFunc(at5.Options, func(op ChoiceOption) bool { return op.Key == postureOwn }); j < 0 || at5.Options[j].Blocked() {
		t.Errorf("the option at level 5 = %d, want listed and free", j)
	}
	// Picked, it applies under its parent and the later feature that lists the parent's options does not need it.
	base := Derive(postureBuild(5, postureHeron, postureBull, postureWolf), c).Initiative
	with := Derive(postureBuild(5, postureHeron, postureBull, postureOwn), c).Initiative
	if with-base != 3 { // the Fox's +3 in place of the Wolf's nothing
		t.Errorf("initiative with the table option = %d, without = %d, want the Fox's +3 to apply", with, base)
	}
	// The same overlay without the option keeps the five.
	if got := optionChoiceOf(t, withOverlay(t, postureOverlay()).Choices(postureBuild(3)), postureFeature); len(got.Options) != 5 {
		t.Errorf("without the option the feature offers %d, want 5", len(got.Options))
	}
}

func TestATableFeatureOptionRefusalsNameTheirField(t *testing.T) {
	srd := loadForTest(t)
	for name, tc := range map[string]struct {
		edit  func(*TableOption)
		field string
	}{
		"no feature":                {func(o *TableOption) { o.OfferedBy = "" }, ".offered_by"},
		"unknown feature":           {func(o *TableOption) { o.OfferedBy = "feature:nao-existe" + tableSuffix }, ".offered_by"},
		"an SRD feature":            {func(o *TableOption) { o.OfferedBy = "feature:second-wind" }, ".offered_by"},
		"a feature with no options": {func(o *TableOption) { o.OfferedBy = "feature:simples" + tableSuffix }, ".offered_by"},
	} {
		o := postureOverlay()
		o.Subclasses[0].Levels = append(o.Subclasses[0].Levels, TableSubclassLevel{Level: 9, Features: []TableFeature{tf("simples", "Simples")}})
		fox := postureFoxOption()
		tc.edit(&fox)
		o.Options = []TableOption{fox}
		_, err := srd.With(o)
		var oe *OverlayError
		if !errors.As(err, &oe) || !strings.HasSuffix(oe.Field, tc.field) {
			t.Errorf("%s: error = %v, want it at %s", name, err, tc.field)
		}
	}
	// offered_by belongs to the set "table_feature" alone.
	o := postureOverlay()
	crow := crowGazeOption()
	crow.OfferedBy = postureFeature
	o.Options = []TableOption{crow}
	if _, err := srd.With(o); err == nil {
		t.Error("an SRD set with offered_by was accepted")
	}
}

// foxSpending is the table option of the feature "Posturas" with the effects given.
func foxSpending(effects ...Effect) Overlay {
	o := postureOverlay()
	fox := postureFoxOption()
	fox.Prerequisite = TableOptionPrerequisite{}
	fox.Effects = effects
	o.Options = []TableOption{fox}
	return o
}

func TestATableFeatureOptionSpendsAndReadsWhatItsFeatureDeclares(t *testing.T) {
	srd := loadForTest(t)

	// The feature declares the resource and a state; the option spends the one and reads the other.
	spends := foxSpending(
		Effect{Type: "grant_action", Economy: "bonus_action", Resource: postureDice},
		Effect{Type: "modifier", Target: "initiative", Mode: "add", Value: "1", When: `active("foco")`},
	)
	spends.Subclasses[0].Levels[0].Features[0].Effects = append(spends.Subclasses[0].Levels[0].Features[0].Effects,
		Effect{Type: "state", StateName: "foco", EndsOn: []string{StateEndsDismiss}, Economy: EconomyFree})
	if _, err := srd.With(spends); err != nil {
		t.Fatalf("an option that spends the dice and reads the state of its feature was refused: %v", err)
	}

	// A resource nobody declares is still refused, at the effect.
	_, err := srd.With(foxSpending(Effect{Type: "grant_action", Economy: "bonus_action", Resource: "dados_que_nao_existem"}))
	var oe *OverlayError
	if !errors.As(err, &oe) || !strings.HasSuffix(oe.Field, ".effects[0].resource") {
		t.Errorf("an unknown resource: error = %v, want it at .effects[0].resource", err)
	}

	// A state the feature does not declare is refused too.
	_, err = srd.With(foxSpending(Effect{Type: "modifier", Target: "initiative", Mode: "add", Value: "1", When: `active("foco")`}))
	if err == nil {
		t.Error("an option reading a state no effect declares was accepted")
	}
}

// The identity of the issue of a pick that nothing offers is the sentence that names the entry whoever reads the sheet: a
// sheet that stored it before the entry was switched off still holds the issue as known.
func TestTheIdentityOfAPickNothingOffersDoesNotDependOnTheReader(t *testing.T) {
	const blow = "option:table_feature:golpe-do-corvo@mesa"
	o := postureOverlay()
	o.Options = []TableOption{{TableEntry: TableEntry{Key: blow, NamePT: "Golpe do Corvo"}, Set: "table_feature", OfferedBy: postureFeature, DescPT: []string{"Um golpe."}}}
	o.Off = append(o.Off, blow)
	c := withOverlay(t, o)
	b := postureBuild(3, postureHeron, blow)
	b.Classes[0].Subclass = ""
	const sentence = "Golpe do Corvo não vale para este personagem."
	for name, view := range map[string]*Content{"master": c, "player": c.ForPlayer()} {
		var got []Issue
		for _, is := range Derive(b, view).Issues {
			if is.Code == IssueUnknownKey && is.Field == "full.feature_choice_keys[1]" {
				got = append(got, is)
			}
		}
		if len(got) != 1 {
			t.Fatalf("%s: issues = %+v, want one about the pick", name, got)
		}
		stored := []string{IssueUnknownKey + "|full.feature_choice_keys[1]|" + sentence}
		if !KnownIssues(stored, got)[0] {
			t.Errorf("%s: the stored identity %q does not hold the issue %q", name, stored[0], got[0].ID())
		}
	}
}
