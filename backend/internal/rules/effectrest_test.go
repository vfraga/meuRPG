package rules

import "testing"

// #580 S3: a proficiency that lasts until the next rest, and a choice of a skill or a tool made again each day.

func TestARestDurationAndADailyChoiceAreCheckedAtTheirFields(t *testing.T) {
	t.Parallel()
	skill := func(f func(e *Effect)) Effect {
		e := Effect{Type: "choice", Choice: "skill", Count: 1, DailyChoice: true, Duration: "long_rest"}
		f(&e)
		return e
	}
	for name, tc := range map[string]struct {
		effect Effect
		field  string
	}{
		"a daily choice with no duration":    {skill(func(e *Effect) { e.Duration = "" }), ".duration"},
		"a daily choice that lasts always":   {skill(func(e *Effect) { e.Duration = "always" }), ".duration"},
		"a daily choice of a language":       {skill(func(e *Effect) { e.Choice = "language" }), ".daily_choice"},
		"a daily choice of expertise":        {skill(func(e *Effect) { e.Choice = "expertise" }), ".daily_choice"},
		"a duration on a choice that is not": {skill(func(e *Effect) { e.DailyChoice = false }), ".duration"},
		"a long rest on a fixed proficiency": {Effect{Type: "proficiency", Proficiency: "skill:perception", Duration: "long_rest"}, ".duration"},
		"an unknown duration":                {Effect{Type: "proficiency", Proficiency: "skill:perception", Duration: "week"}, ".duration"},
		"a duration on a note":               {Effect{Type: "note", Duration: "short_rest"}, ".duration"},
		"a daily choice on a note":           {Effect{Type: "note", DailyChoice: true}, ".daily_choice"},
	} {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			_, err := loadForTest(t).With(featWith(flagFeat, tc.effect))
			if err == nil {
				t.Fatalf("With() accepted %+v", tc.effect)
			}
			violationAt(t, err, "feats[0].effects[0]"+tc.field, ReasonValue)
		})
	}
}

func TestARestDurationAndADailyChoiceAreAccepted(t *testing.T) {
	t.Parallel()
	for name, e := range map[string]Effect{
		"a daily skill until the long rest":  {Type: "choice", Choice: "skill", Count: 1, DailyChoice: true, Duration: "long_rest"},
		"a daily tool until the short rest":  {Type: "choice", Choice: "tool", Count: 1, DailyChoice: true, Duration: "short_rest"},
		"a proficiency until the short rest": {Type: "proficiency", Proficiency: "skill:perception", Duration: "short_rest"},
		"a proficiency for ever":             {Type: "proficiency", Proficiency: "skill:perception"},
	} {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			if _, err := loadForTest(t).With(featWith(flagFeat, e)); err != nil {
				t.Errorf("With() = %v", err)
			}
		})
	}
}
