package rules

// The rest-bound fields of an effect (#580): a proficiency that lasts until the next rest, and a choice of a skill or a
// tool the player makes again each day.

// The durations of an effect: empty lasts for ever.
const (
	// DurationShortRest lasts until the next short rest (a long rest ends it too).
	DurationShortRest = "short_rest"
	// DurationLongRest lasts until the next long rest.
	DurationLongRest = "long_rest"
)

// dailyChoiceRefusal is the text at the field of a daily choice that lasts for ever.
const dailyChoiceRefusal = "Uma escolha diária dura até um descanso: escolha o próximo descanso curto ou o longo."

// checkRestFields checks Duration and DailyChoice at their own fields. A fixed proficiency lasts for ever or until the
// next short rest (the next long rest grants it again, so a long-rest duration would mean for ever); a daily choice is
// of a skill or a tool and says which rest ends it; no other effect has either field.
func checkRestFields(site effectSite, e *Effect) error {
	switch {
	case e.DailyChoice && (e.Type != "choice" || (e.Choice != "skill" && e.Choice != "tool")):
		return site.fail(".daily_choice", ReasonValue, "a daily choice is a choice of a skill or a tool")
	case e.DailyChoice && e.Duration != DurationShortRest && e.Duration != DurationLongRest:
		return site.fail(".duration", ReasonValue, "%s", dailyChoiceRefusal)
	case e.DailyChoice && e.Count > 1:
		return site.fail(".count", ReasonValue, "a daily choice picks one")
	case e.Duration == "" || e.DailyChoice:
		return nil
	case e.Type == "proficiency" && e.Duration == DurationShortRest:
		return nil
	case e.Type == "proficiency":
		return site.fail(".duration", ReasonValue, "a fixed proficiency lasts for ever or until the next short rest: a long rest grants it again")
	default:
		return site.fail(".duration", ReasonValue, "a duration belongs to a proficiency or to a daily choice of a skill or a tool")
	}
}
