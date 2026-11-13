package rules

import (
	"slices"
	"testing"
)

// The flag effect (#572): a closed list of rules a table feature switches on. The features are invented.

const flagFeat = "feat:sinal-da-mesa" + tableSuffix

func TestAFlagEffectNeedsOneKnownFlagAndNothingElse(t *testing.T) {
	t.Parallel()
	for name, tc := range map[string]struct {
		effect Effect
		field  string
	}{
		"no flag":              {Effect{Type: "flag"}, ".flag"},
		"an unknown flag":      {Effect{Type: "flag", Flag: "always_lucky"}, ".flag"},
		"tags":                 {Effect{Type: "flag", Flag: FlagNeverSurprised, Tags: []string{"source:fiction"}}, ".tags"},
		"a value":              {Effect{Type: "flag", Flag: FlagNeverSurprised, Value: "1"}, ".value"},
		"a resource":           {Effect{Type: "flag", Flag: FlagNeverSurprised, Resource: "x"}, ".resource"},
		"a while":              {Effect{Type: "flag", Flag: FlagNeverSurprised, While: ResistanceWhileRage}, ""},
		"a flag on a note":     {Effect{Type: "note", Flag: FlagNeverSurprised}, ".flag"},
		"a flag on a modifier": {Effect{Type: "modifier", Target: "ac", Mode: "add", Value: "1", Flag: FlagIgnoreHostileNearby}, ".flag"},
	} {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			_, err := loadForTest(t).With(featWith(flagFeat, tc.effect))
			if err == nil {
				t.Fatalf("With() accepted %+v", tc.effect)
			}
			if tc.field != "" {
				violationAt(t, err, "feats[0].effects[0]"+tc.field, ReasonValue)
			}
		})
	}
}

func TestAFlagEffectWithAWhenIsAccepted(t *testing.T) {
	t.Parallel()
	for _, flag := range effectFlags {
		if _, err := loadForTest(t).With(featWith(flagFeat, Effect{Type: "flag", Flag: flag, When: "level() >= 2"})); err != nil {
			t.Errorf("the flag %s with a when is refused: %v", flag, err)
		}
	}
}

func TestTheFlagListHasAPortugueseNameAndHintForEachFlag(t *testing.T) {
	t.Parallel()
	m := loadForTest(t).EffectMenu()
	values := menuList(t, m, ListEffectFlags)
	if !slices.Equal(valueKeys(values), effectFlags) {
		t.Errorf("the list = %v, want %v", valueKeys(values), effectFlags)
	}
	for _, v := range values {
		if v.NamePT == "" || v.HintPT == "" {
			t.Errorf("flag %s has no Portuguese name or hint: %+v", v.Key, v)
		}
	}
	var fields []MenuField
	for _, mt := range m.Types {
		if mt.Type == "flag" {
			if mt.NamePT != "Sinalizador" || mt.HintPT == "" {
				t.Errorf("the flag type reads %q / %q", mt.NamePT, mt.HintPT)
			}
			fields = mt.Fields
		}
	}
	if len(fields) == 0 || fields[0].Name != "flag" || !fields[0].Required || fields[0].Kind != "cards" || fields[0].List != ListEffectFlags {
		t.Errorf("the flag type's fields = %+v, want a required flag of cards first", fields)
	}
	for _, f := range fields[1:] {
		if f.Name != "when" && f.Name != "text_pt" {
			t.Errorf("the flag type offers %s: only when and text_pt are allowed beside the flag", f.Name)
		}
	}
}

func TestTheFlagsOfEffectsThatHoldAreSortedAndUnique(t *testing.T) {
	t.Parallel()
	d := derivedWith(t, 3,
		Effect{Type: "flag", Flag: FlagNeverSurprised},
		Effect{Type: "flag", Flag: FlagIgnoreHostileNearby},
		Effect{Type: "flag", Flag: FlagNeverSurprised},
	)
	if want := []string{FlagIgnoreHostileNearby, FlagNeverSurprised}; !slices.Equal(d.Flags, want) {
		t.Errorf("Flags = %v, want %v", d.Flags, want)
	}
}

func TestAFlagWhoseWhenIsFalseIsLeftOut(t *testing.T) {
	t.Parallel()
	d := derivedWith(t, 3,
		Effect{Type: "flag", Flag: FlagNeverSurprised, When: "level() >= 10"},
		Effect{Type: "flag", Flag: FlagIgnoreHostileNearby, When: "level() >= 2"},
	)
	if want := []string{FlagIgnoreHostileNearby}; !slices.Equal(d.Flags, want) {
		t.Errorf("Flags = %v, want %v: the first when is false", d.Flags, want)
	}
	if got := derivedWith(t, 3).Flags; len(got) != 0 {
		t.Errorf("Flags = %v, want none without a flag effect", got)
	}
}
