package rules

import (
	"fmt"
	"slices"
	"strconv"
	"strings"
)

// The refusal of a new pick that no source takes. A pick the stored sheet already holds is never refused here, whether it
// fits or not: the fit issue of a stored sheet is the master's alone and the sheet still saves and plays.

// UnplacedNewPick returns the sheet field and the key of the first skill, expertise or tool pick of the new sheet that
// the stored sheet (nil for a new character) does not hold and that no source of the content takes: the largest placement
// of the picks in the sources is no larger with the pick than without it.
// A held pick that fits takes part in the placement, so a new pick that fits only when a held pick moves to another source
// is not refused.
// A held pick that the stored sheet could not place leaves the sources alone.
// c is the caller's view of the content.
func (c *Content) UnplacedNewPick(stored *Build, now Build) (field, key string, found bool) {
	x := c.deriverFor(now)
	if len(x.classes) == 0 {
		return "", "", false
	}
	var held Build
	var xs *deriver
	if stored != nil {
		held, xs = *stored, c.deriverFor(*stored)
	}
	if f, k, ok := c.c.unplacedNewSkillPick(x, xs, held, now); ok {
		return f, k, true
	}
	return x.unplacedNewTool(held, xs)
}

// unplacedNewSkillPick is UnplacedNewPick for the skills and the expertise: x is the deriver of the new sheet now, xs the
// one of the stored sheet (nil when there is none) and held the stored sheet.
func (c *content) unplacedNewSkillPick(x, xs *deriver, held, now Build) (field, key string, found bool) {
	skill := func(d *deriver) func(string) bool {
		return func(k string) bool { _, ok := c.skills[k]; return ok && !d.automaticSkills[k] }
	}
	expertise := func(k string) bool { _, ok := c.skills[k]; return ok }
	var stuckSkills, stuckExpertise, heldSkills []string
	if xs != nil {
		// A skill the stored sheet has automatically is no pick of it: when the save makes it one, it is new.
		heldSkills = pickList(held.SkillProficiencies, skill(xs))
		stuckSkills = unplacedPicks(xs.skillSources(), heldSkills)
		stuckExpertise = unplacedPicks(xs.expertiseSources(), pickList(held.Expertise, expertise))
	}
	if f, k, ok := unplacedNewList(skillsField, x.skillSources(), now.SkillProficiencies, heldSkills, stuckSkills, skill(x)); ok {
		return f, k, true
	}
	return unplacedNewList(expertiseField, x.expertiseSources(), now.Expertise, held.Expertise, stuckExpertise, expertise)
}

// deriverFor is the deriver of the sheet b, made with the entries it loses left out.
func (c *Content) deriverFor(b Build) *deriver { return c.c.deriverFor(b) }

func (c *content) deriverFor(b Build) *deriver {
	_, lost, _ := c.settleLost(b)
	return deriverOf(b, c, lost)
}

// pickList are the entries of the list that count, each once.
func pickList(list []string, counted func(string) bool) []string {
	var picks []string
	for _, k := range list {
		if counted(k) && !slices.Contains(picks, k) {
			picks = append(picks, k)
		}
	}
	return picks
}

// unplacedNewList is UnplacedNewPick for one list of the sheet: counted says an entry is a pick of the sources, and stuck are
// the held picks the stored sheet could not place.
func unplacedNewList(field string, sources []pickSource, list, held, stuck []string, counted func(string) bool) (string, string, bool) {
	picks := pickList(list, counted)
	var first namedPick
	for i, k := range list {
		if slices.Contains(held, k) || !slices.Contains(picks, k) {
			continue
		}
		if !placesAlone(sources, competing(picks, held, stuck), k) {
			first.note(sources, fmt.Sprintf("%s[%d]", field, i), k)
		}
	}
	return first.field, first.key, first.found
}

// namedPick is the new pick a refusal names, among the new picks the sources cannot take.
// A pick that no source lists is named before one that only competes for room: its sentence is true whatever the other picks are.
// Of the others the first in sheet order is named.
type namedPick struct {
	field, key string
	found      bool
	listed     bool // the pick named is on some source's list
}

// note offers a pick that adds nothing to the placement; it is named when no pick before it is better.
func (r *namedPick) note(sources []pickSource, field, key string) {
	listed := placedCount(sources, []string{key}) > 0
	if r.found && (!r.listed || listed) {
		return
	}
	*r = namedPick{field: field, key: key, found: true, listed: listed}
}

// competing are the picks that take part in the placement of a new pick: the new ones and the held ones the stored sheet could place.
func competing(picks, held, stuck []string) []string {
	return slices.DeleteFunc(slices.Clone(picks), func(k string) bool { return slices.Contains(held, k) && slices.Contains(stuck, k) })
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

// toolPicksByOwner are the tool picks of the sheet whose owner it has, owner by owner, each value once.
func (x *deriver) toolPicksByOwner() map[string][]string {
	onSheet := ownedBy(x.b, *x.d)
	out := map[string][]string{}
	for _, stored := range x.b.FeatureChoices {
		choiceKey, value, ok := SplitScopedChoice(stored)
		if !ok {
			continue
		}
		if owner, isTool := strings.CutSuffix(choiceKey, toolPickSuffix); isTool && (x.owned[owner] || onSheet(owner)) && !slices.Contains(out[owner], value) {
			out[owner] = append(out[owner], value)
		}
	}
	return out
}

// unplacedNewTool is UnplacedNewPick for the tool picks of the sheet, owner by owner; xs is the stored sheet's deriver or nil.
func (x *deriver) unplacedNewTool(held Build, xs *deriver) (string, string, bool) {
	byOwner := x.toolPicksByOwner()
	stuck := map[string][]string{}
	if xs != nil {
		for owner, values := range xs.toolPicksByOwner() {
			stuck[owner] = unplacedPicks(xs.c.toolSources(owner), values)
		}
	}
	var first namedPick
	for i, stored := range x.b.FeatureChoices {
		choiceKey, value, ok := SplitScopedChoice(stored)
		if !ok || slices.Contains(held.FeatureChoices, stored) {
			continue
		}
		owner, isTool := strings.CutSuffix(choiceKey, toolPickSuffix)
		if !isTool || byOwner[owner] == nil {
			continue
		}
		heldValues := slices.DeleteFunc(slices.Clone(byOwner[owner]), func(k string) bool { return !slices.Contains(held.FeatureChoices, ScopedChoice(choiceKey, k)) })
		sources := x.c.toolSources(owner)
		if !placesAlone(sources, competing(byOwner[owner], heldValues, stuck[owner]), value) {
			first.note(sources, fmt.Sprintf("full.feature_choice_keys[%d]", i), value)
		}
	}
	return first.field, first.key, first.found
}

// IsToolProficiency says the key is a proficiency a tool choice may offer: a tool, a vehicle, a kit or a weapon.
func (c *Content) IsToolProficiency(key string) bool { return c.c.isToolPickKey(key) }

// PickAllowed is the keys the sources of one kind of pick allow. Any is set when a source takes any key.
type PickAllowed struct {
	Any  bool
	Keys []string
}

// PickLists are the skills the sheet's skill sources allow and, when every expertise source names a list, the skills
// expertise may go on (nil otherwise). They are the lists the save's refusal reads (unplacedNewList).
func (c *Content) PickLists(b Build) (skills PickAllowed, expertise *PickAllowed) {
	x := c.deriverFor(b)
	skills = allowedBy(x.skillSources())
	if sources := x.expertiseSources(); len(sources) > 0 {
		if e := allowedBy(sources); !e.Any {
			expertise = &e
		}
	}
	return skills, expertise
}

// allowedBy is the union of the lists of sources, each key once in the order met.
func allowedBy(sources []pickSource) PickAllowed {
	var out PickAllowed
	for _, s := range sources {
		if s.Any {
			out.Any = true
			continue
		}
		for _, k := range s.From {
			if !slices.Contains(out.Keys, k) {
				out.Keys = append(out.Keys, k)
			}
		}
	}
	if out.Any {
		out.Keys = nil
	}
	return out
}

// OnAList says whether some source of the sheet b lists key for the pick field (the skill, the expertise or a tool pick
// of the sheet's choice keys). A source that names no list lists every key. A field of another kind is never on a list.
func (c *Content) OnAList(b Build, field, key string) bool {
	x := c.deriverFor(b)
	var sources []pickSource
	switch {
	case strings.HasPrefix(field, skillsField):
		sources = x.skillSources()
	case strings.HasPrefix(field, expertiseField):
		sources = x.expertiseSources()
	case strings.HasPrefix(field, choiceKeysField):
		sources = x.toolSourcesAt(field)
	}
	return slices.ContainsFunc(sources, func(s pickSource) bool { return s.allows(key) })
}

// toolSourcesAt are the sources of the tool pick the sheet's choice keys hold at the field's place: the lists of the tool
// choices of its owner. A place that holds no tool pick has none.
func (x *deriver) toolSourcesAt(field string) []pickSource {
	place, ok := strings.CutPrefix(field, choiceKeysField+"[")
	if !ok {
		return nil
	}
	i, err := strconv.Atoi(strings.TrimSuffix(place, "]"))
	if err != nil || i < 0 || i >= len(x.b.FeatureChoices) {
		return nil
	}
	choiceKey, _, ok := SplitScopedChoice(x.b.FeatureChoices[i])
	owner, isTool := strings.CutSuffix(choiceKey, toolPickSuffix)
	if !ok || !isTool {
		return nil
	}
	return x.c.toolSources(owner)
}
