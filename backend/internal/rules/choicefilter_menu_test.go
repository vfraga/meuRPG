package rules

import (
	"slices"
	"testing"
)

// The master's editor counts what a filter of proficiency kinds matches from the menu's own lists: one list for
// each kind, with every proficiency of the content of that kind.
func TestTheMenuListsTheProficienciesOfEachKind(t *testing.T) {
	t.Parallel()
	c := loadForTest(t)
	m := c.EffectMenu()
	for list, kind := range map[string]string{"weapon_proficiencies": "weapon", "tool_proficiencies": "tool", "other_proficiencies": "other"} {
		var want []string
		for k, p := range c.c.proficiencies {
			if p.Kind == kind {
				want = append(want, k)
			}
		}
		slices.Sort(want)
		got := valueKeys(menuList(t, m, list))
		slices.Sort(got)
		if len(want) == 0 || !slices.Equal(got, want) {
			t.Errorf("list %s has %d keys, want the %d proficiencies of kind %s", list, len(got), len(want), kind)
		}
	}
}

// The choice type reads the filter and the feature it offers the options of, each with the kind the editor asks it by.
func TestTheChoiceTypeAsksForAFilterAndAFeature(t *testing.T) {
	t.Parallel()
	m := loadForTest(t).EffectMenu()
	var fields []MenuField
	for _, mt := range m.Types {
		if mt.Type == "choice" {
			fields = mt.Fields
		}
	}
	for name, kind := range map[string]string{"filter": "choice_filter", "from_feature": "feature_ref"} {
		i := slices.IndexFunc(fields, func(f MenuField) bool { return f.Name == name })
		if i < 0 || fields[i].Kind != kind {
			t.Errorf("choice field %s = %+v, want kind %s", name, fields, kind)
		}
	}
	kinds := valueKeys(menuList(t, m, "proficiency_kinds"))
	if !slices.Equal(kinds, []string{"weapon", "tool", "other"}) {
		t.Errorf("proficiency_kinds = %v, want weapon, tool, other", kinds)
	}
}

// The list a choice of tools may name holds the tools, vehicles and kits and the weapons; the list of tools stays as it was,
// as a background's tools read it.
func TestTheMenuListsTheProficienciesAToolChoiceMayName(t *testing.T) {
	t.Parallel()
	c := loadForTest(t)
	m := c.EffectMenu()
	got, tools := valueKeys(menuList(t, m, ListToolChoices)), valueKeys(menuList(t, m, ListTools))
	var want []string
	for k, p := range c.c.proficiencies {
		if slices.Contains(filterKinds, p.Kind) {
			want = append(want, k)
		}
	}
	slices.Sort(want)
	slices.Sort(got)
	if !slices.Equal(got, want) {
		t.Errorf("tool_choices has %d keys, want the %d proficiencies of the kinds weapon, tool and other", len(got), len(want))
	}
	for _, k := range tools {
		if c.c.proficiencies[k].Kind == filterKindWeapon {
			t.Errorf("the list of tools names the weapon %s", k)
		}
	}
}
