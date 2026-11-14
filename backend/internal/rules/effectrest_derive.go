package rules

import (
	"slices"
	"strings"
)

// dailyPicks is what the daily choices of the sheet come to: the skills and tools picked for today, each with
// the rest that ends it.
type dailyPicks struct {
	skills map[string]string
	tools  map[string]bool
}

// dailyState reads the sheet's daily choices once: it lists them in Derived.DailyChoices, picked or waiting, and
// keeps the picks that fit. A pick of a choice the sheet no longer has, or of a key the choice does not offer, counts for nothing.
func (x *deriver) dailyState() *dailyPicks {
	if x.daily != nil {
		return x.daily
	}
	x.daily = &dailyPicks{skills: map[string]string{}, tools: map[string]bool{}}
	for _, a := range x.active {
		e := a.effect
		if e.Type != "choice" || !e.DailyChoice || !x.applies(a) {
			continue
		}
		dc := DailyChoice{ID: a.id, OwnerNamePT: x.c.namePT(a.owner), Kind: e.Choice, UntilRest: e.Duration, Options: slices.Clone(e.From), Signature: dailySignature(e)}
		x.takeDailyPick(a, &dc)
		x.d.DailyChoices = append(x.d.DailyChoices, dc)
	}
	return x.daily
}

// dailyKeyFits says the key is one the daily choice e offers: a skill (or a tool) of the content, and one of e.From when it lists them.
func (c *content) dailyKeyFits(e *Effect, key string) bool {
	if len(e.From) > 0 && !slices.Contains(e.From, key) {
		return false
	}
	if e.Choice == "skill" {
		_, ok := c.skills[key]
		return ok
	}
	return c.isToolKey(key)
}

// dailySignature is what a daily choice is, apart from its place among the effects: its kind, its duration and the keys it offers.
func dailySignature(e *Effect) string {
	from := slices.Clone(e.From)
	slices.Sort(from)
	return e.Choice + "|" + e.Duration + "|" + strings.Join(from, ",")
}

// takeDailyPick records the sheet's pick of the daily choice a, when it fits, in dc and in the picked skills or tools.
func (x *deriver) takeDailyPick(a activeEffect, dc *DailyChoice) {
	e := a.effect
	for _, p := range x.b.DailyPicks {
		if p.ChoiceID != a.id || p.Signature != dc.Signature || !x.c.dailyKeyFits(e, p.Key) {
			continue
		}
		dc.Picked = p.Key
		if e.Choice == "skill" {
			x.daily.skills[p.Key] = e.Duration
		} else {
			x.daily.tools[p.Key] = true
		}
	}
}
