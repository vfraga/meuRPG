package rules

import (
	"fmt"
	"slices"
)

// The fit of a sheet's picks to the lists they come from.
// A stored sheet with the right count of picks that no placement fits still saves, levels up and plays.
// Its held picks still count and still give what they give.
// The issue is the master's alone, it is no choice problem, and it blocks nothing.

// The sheet fields of the lists the fit issue is about.
const (
	skillsField    = "full.skill_proficiency_keys"
	expertiseField = "full.expertise_skill_keys"
)

// readsFit says the derivation is the master's: a player's view of the table never carries the fit issue.
func (x *deriver) readsFit() bool {
	return x.c.table && !x.c.player && len(x.classes) > 0
}

// checkPickFit reports the picks the sources do not take, once the count of each kind is right.
func (x *deriver) checkPickFit(chosen map[string]bool) {
	if !x.readsFit() {
		return
	}
	skills, expertise := x.skillAllowance()
	x.checkFitOf(skillsField, x.skillSources(), skills, x.b.SkillProficiencies, func(k string) bool { return chosen[k] },
		"A perícia %s não cabe em nenhuma das listas de onde o personagem escolhe perícias.")
	_, known := x.knownExpertise()
	x.checkFitOf(expertiseField, x.expertiseSources(), expertise, x.b.Expertise, func(k string) bool { return known[k] },
		"A especialização em %s não cabe em nenhuma das listas de onde o personagem escolhe especializações.")
	x.checkToolFit()
}

// knownExpertise are the expertise keys of the sheet that name a skill of the content.
func (x *deriver) knownExpertise() (list []string, known map[string]bool) {
	known = map[string]bool{}
	for _, s := range x.b.Expertise {
		if _, ok := x.c.skills[s]; ok {
			known[s] = true
			list = append(list, s)
		}
	}
	return list, known
}

// checkFitOf places the sheet's list (the entries counted: keys, each once) in the sources and gives an issue to each one
// left out, when the picks are as many as total. field is the sheet's list; the issue is on the entry's place in it.
func (x *deriver) checkFitOf(field string, sources []pickSource, total int, list []string, counted func(string) bool, format string) {
	var picks []string
	for _, k := range list {
		if counted(k) && !slices.Contains(picks, k) {
			picks = append(picks, k)
		}
	}
	if len(picks) != total {
		return
	}
	for _, k := range unplacedPicks(sources, picks) {
		x.issue(IssuePickFit, fmt.Sprintf("%s[%d]", field, slices.Index(list, k)), format, x.c.namePT(k))
	}
}

// checkToolFit reports the tool picks an owner offers that none of its tool choices takes, when the owner's picks are as
// many as its choices ask.
func (x *deriver) checkToolFit() {
	for _, p := range x.unplacedTools {
		offered := 0
		for _, q := range x.pickedTools {
			if q.Owner == p.Owner {
				offered++
			}
		}
		total := 0
		for _, s := range x.c.toolSources(p.Owner) {
			total += s.Count
		}
		if offered == total {
			x.issue(IssuePickFit, choiceKeysField, "A ferramenta %s não cabe em nenhuma das listas de onde o personagem escolhe ferramentas.", x.c.proficiencyNamePT(p.Value))
		}
	}
}
