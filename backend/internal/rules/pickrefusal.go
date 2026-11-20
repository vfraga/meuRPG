package rules

import (
	"fmt"
	"slices"
	"strings"
)

// The refusal of a new pick that no source takes. A pick the stored sheet already holds is never refused here, whether it
// fits or not: the fit issue of a stored sheet is the master's alone and the sheet still saves and plays.

// UnplacedNewPick returns the sheet field and the key of the first skill, expertise or tool pick of the new sheet that
// the stored sheet (nil for a new character) does not hold and that no source of the content takes: the maximum placement
// of all the picks in the sources is no larger with the pick than without it. The held picks count in the placement, so
// a held pick that does not fit leaves a source alone and a new pick that fits only when a held pick moves to another
// source is not refused. c is the caller's view of the content.
func (c *Content) UnplacedNewPick(stored *Build, now Build) (field, key string, found bool) {
	_, lost, _ := c.c.settleLost(now)
	x := deriverOf(now, c.c, lost)
	if len(x.classes) == 0 {
		return "", "", false
	}
	var held Build
	if stored != nil {
		held = *stored
	}
	if f, k, ok := x.unplacedNewList("full.skill_proficiency_keys", x.skillSources(), now.SkillProficiencies, held.SkillProficiencies,
		func(k string) bool { _, ok := c.c.skills[k]; return ok && !x.automaticSkills[k] }); ok {
		return f, k, true
	}
	if f, k, ok := x.unplacedNewList("full.expertise_skill_keys", x.expertiseSources(), now.Expertise, held.Expertise,
		func(k string) bool { _, ok := c.c.skills[k]; return ok }); ok {
		return f, k, true
	}
	return x.unplacedNewTool(held)
}

// unplacedNewList is UnplacedNewPick for one list of the sheet: counted says an entry is a pick of the sources.
func (x *deriver) unplacedNewList(field string, sources []pickSource, list, held []string, counted func(string) bool) (string, string, bool) {
	var picks []string
	for _, k := range list {
		if counted(k) && !slices.Contains(picks, k) {
			picks = append(picks, k)
		}
	}
	for i, k := range list {
		if slices.Contains(held, k) || !slices.Contains(picks, k) {
			continue
		}
		if !placesAlone(sources, picks, k) {
			return fmt.Sprintf("%s[%d]", field, i), k, true
		}
	}
	return "", "", false
}

// placesAlone says the largest placement is bigger with the pick than without it.
func placesAlone(sources []pickSource, picks []string, pick string) bool {
	without := slices.DeleteFunc(slices.Clone(picks), func(k string) bool { return k == pick })
	return placedCount(sources, picks) > placedCount(sources, without)
}

// placedCount is how many of the picks a placement in the sources takes.
func placedCount(sources []pickSource, picks []string) int {
	return len(picks) - len(unplacedPicks(sources, picks))
}

// unplacedNewTool is UnplacedNewPick for the tool picks of the sheet, owner by owner.
func (x *deriver) unplacedNewTool(held Build) (string, string, bool) {
	onSheet := ownedBy(x.b, *x.d)
	byOwner := map[string][]string{}
	for _, stored := range x.b.FeatureChoices {
		choiceKey, value, ok := SplitScopedChoice(stored)
		if !ok {
			continue
		}
		if owner, isTool := strings.CutSuffix(choiceKey, toolPickSuffix); isTool && (x.owned[owner] || onSheet(owner)) && !slices.Contains(byOwner[owner], value) {
			byOwner[owner] = append(byOwner[owner], value)
		}
	}
	for i, stored := range x.b.FeatureChoices {
		choiceKey, value, ok := SplitScopedChoice(stored)
		if !ok || slices.Contains(held.FeatureChoices, stored) {
			continue
		}
		owner, isTool := strings.CutSuffix(choiceKey, toolPickSuffix)
		if isTool && byOwner[owner] != nil && !placesAlone(x.c.toolSources(owner), byOwner[owner], value) {
			return fmt.Sprintf("full.feature_choice_keys[%d]", i), value, true
		}
	}
	return "", "", false
}
