package rules

import "testing"

// #580 S3: what the sheet shows of a rest-bound proficiency and a daily choice.

const restFeat = "feat:sinal-do-dia" + tableSuffix

func restOverlay() Overlay {
	return featWith(restFeat,
		Effect{Type: "choice", Choice: "skill", Count: 1, DailyChoice: true, Duration: DurationLongRest},
		Effect{Type: "proficiency", Proficiency: "skill:perception", Duration: DurationShortRest},
		Effect{Type: "choice", Choice: "tool", Count: 1, DailyChoice: true, Duration: DurationShortRest},
	)
}

func restBuild(mut func(b *Build)) Build {
	b := weaponBuild("class:wizard")
	b.Feats = []string{restFeat}
	b.SkillProficiencies = nil
	if mut != nil {
		mut(&b)
	}
	return b
}

func TestADailyChoiceWaitsUntilThePlayerPicksAndTheSkillCarriesUntilWhichRest(t *testing.T) {
	t.Parallel()
	c := withOverlay(t, restOverlay())

	d := Derive(restBuild(nil), c)
	if len(d.DailyChoices) != 2 || d.DailyChoices[0].ID != restFeat+"#0" || d.DailyChoices[0].Kind != "skill" || d.DailyChoices[0].Picked != "" || d.DailyChoices[1].Kind != "tool" {
		t.Fatalf("daily choices = %+v, want a skill and a tool, both waiting", d.DailyChoices)
	}
	if s := skillOf(d, "skill:stealth"); s.Proficiency != ProficiencyNone || s.UntilRest != "" {
		t.Errorf("stealth before the pick = %+v", s)
	}

	d = Derive(restBuild(func(b *Build) {
		b.DailyPicks = []DailyPick{{ChoiceID: restFeat + "#0", Key: "skill:stealth", Until: DurationLongRest, Signature: "skill|long_rest|"}}
	}), c)
	if s := skillOf(d, "skill:stealth"); s.Proficiency != ProficiencyFull || s.UntilRest != DurationLongRest {
		t.Errorf("stealth after the pick = %+v, want full proficiency until the long rest", s)
	}
	if d.DailyChoices[0].Picked != "skill:stealth" {
		t.Errorf("the choice reads %+v, want the pick", d.DailyChoices[0])
	}

	// A pick that is no skill, or for a choice the sheet does not have, counts for nothing.
	d = Derive(restBuild(func(b *Build) {
		b.DailyPicks = []DailyPick{{ChoiceID: restFeat + "#0", Key: "skill:nada", Until: DurationLongRest}, {ChoiceID: "feat:outro#0", Key: "skill:stealth", Until: DurationLongRest, Signature: "skill|long_rest|"}}
	}), c)
	if s := skillOf(d, "skill:stealth"); s.Proficiency != ProficiencyNone || d.DailyChoices[0].Picked != "" {
		t.Errorf("a pick that does not fit counted: %+v %+v", s, d.DailyChoices)
	}
}

func TestAFixedProficiencyUntilTheShortRestIsSpentByIt(t *testing.T) {
	t.Parallel()
	c := withOverlay(t, restOverlay())
	if s := skillOf(Derive(restBuild(nil), c), "skill:perception"); s.Proficiency != ProficiencyFull || s.UntilRest != "" {
		t.Errorf("perception before the rest = %+v, want proficient", s)
	}
	spent := Derive(restBuild(func(b *Build) { b.ShortRested = true }), c)
	if s := skillOf(spent, "skill:perception"); s.Proficiency != ProficiencyNone {
		t.Errorf("perception after a short rest = %+v, want it spent", s)
	}
}

func TestADailyChoiceOfMoreThanOneIsRefused(t *testing.T) {
	t.Parallel()
	e := Effect{Type: "choice", Choice: "skill", Count: 2, DailyChoice: true, Duration: DurationLongRest}
	_, err := loadForTest(t).With(featWith(flagFeat, e))
	violationAt(t, err, "feats[0].effects[0].count", ReasonValue)
}

// The skill picked for the day is no skill the player keeps: it never raises the count the sheet may list.
func TestADailyChoiceDoesNotRaiseTheSkillsThePlayerKeeps(t *testing.T) {
	t.Parallel()
	c := withOverlay(t, restOverlay())
	d := Derive(restBuild(func(b *Build) {
		b.SkillProficiencies = []string{"skill:arcana", "skill:history", "skill:investigation"}
	}), c)
	if !hasIssue(d, IssueSkillCount) {
		t.Errorf("issues = %+v, want the skill count refused: a wizard keeps 2 and the daily choice adds none", d.Issues)
	}
}

// A pick belongs to the choice it was made for, not to the place of an effect: when the master reorders a feat's effects, a pick
// that no longer fits the choice at its id counts for nothing, and a choice that only changed place is not given another's pick.
func TestAPickDoesNotFollowAReorderedChoice(t *testing.T) {
	t.Parallel()
	arcane := Effect{Type: "choice", Choice: "skill", Count: 1, From: []string{"skill:arcana", "skill:history"}, DailyChoice: true, Duration: DurationLongRest}
	wild := Effect{Type: "choice", Choice: "skill", Count: 1, From: []string{"skill:nature", "skill:survival"}, DailyChoice: true, Duration: DurationLongRest}
	before := withOverlay(t, featWith(restFeat, arcane, wild))
	d := Derive(restBuild(nil), before)
	pick := DailyPick{ChoiceID: restFeat + "#0", Key: "skill:arcana", Until: DurationLongRest, Signature: d.DailyChoices[0].Signature}
	if got := Derive(restBuild(func(b *Build) { b.DailyPicks = []DailyPick{pick} }), before).DailyChoices[0].Picked; got != "skill:arcana" {
		t.Fatalf("the pick on its own choice reads %q", got)
	}
	after := withOverlay(t, featWith(restFeat, wild, arcane)) // the two swap places
	got := Derive(restBuild(func(b *Build) { b.DailyPicks = []DailyPick{pick} }), after)
	if got.DailyChoices[0].Picked != "" || got.DailyChoices[1].Picked != "" {
		t.Errorf("daily choices after a reorder = %+v, want the pick dropped", got.DailyChoices)
	}
}
