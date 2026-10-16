package rules

import (
	"encoding/json"
	"testing"
)

// poolScenario is a choice builder with the entries of a few choices, built
// without the deriver: assignPools only reads the entries, the picks and the
// names. The entry order is the order the character got the choices.
type poolScenario struct {
	plain   []string
	entries []poolEntry
}

type poolEntry struct {
	key     string
	choose  int
	options []string
	level   int
	order   int
}

// poolNames gives the options Portuguese names; two keys that share a name are
// the same option taken from two classes (the Defense style).
var poolNames = map[string]string{
	"feature:style-a": "Estilo A", "feature:style-b": "Estilo B", "feature:style-c": "Estilo C",
	"feature:fighter-defense": "Defesa", "feature:paladin-defense": "Defesa",
	"feature:meta-x": "Meta: X", "feature:meta-y": "Meta: Y", "feature:meta-z": "Meta: Z",
	"feature:o1": "O1", "feature:o2": "O2", "feature:o3": "O3", "feature:o4": "O4",
}

func poolScenarios() map[string]poolScenario {
	a, b, c := "feature:style-a", "feature:style-b", "feature:style-c"
	return map[string]poolScenario{
		"nothing": {},
		"one choice, no picks": {entries: []poolEntry{
			{"feature:fs", 1, []string{a, b, c}, 1, 0},
		}},
		"one choice, one pick": {plain: []string{b}, entries: []poolEntry{
			{"feature:fs", 1, []string{a, b, c}, 1, 0},
		}},
		"two choices share the options, picks dealt in order": {plain: []string{c, a}, entries: []poolEntry{
			{"feature:fs", 1, []string{a, b, c}, 1, 0},
			{"feature:fs2", 1, []string{a, b, c}, 10, 1},
		}},
		"entries given out of level order": {plain: []string{a, b}, entries: []poolEntry{
			{"feature:late", 1, []string{a, b, c}, 10, 0},
			{"feature:early", 1, []string{a, b, c}, 1, 1},
		}},
		"same level, order decides": {plain: []string{a, b}, entries: []poolEntry{
			{"feature:second", 1, []string{a, b, c}, 3, 5},
			{"feature:first", 1, []string{a, b, c}, 3, 2},
		}},
		"more picks than the choices take overflow to the last": {plain: []string{a, b, c}, entries: []poolEntry{
			{"feature:fs", 1, []string{a, b, c}, 1, 0},
			{"feature:fs2", 1, []string{a, b, c}, 10, 1},
		}},
		"a choice that takes two": {plain: []string{c, a, b}, entries: []poolEntry{
			{"feature:meta", 2, []string{a, b, c}, 3, 0},
		}},
		"a pick outside every pool is left alone": {plain: []string{"feature:elsewhere", a}, entries: []poolEntry{
			{"feature:fs", 1, []string{a, b}, 1, 0},
		}},
		"a pick the first choice cannot take goes to the next": {plain: []string{c, a}, entries: []poolEntry{
			{"feature:one", 1, []string{a, b}, 1, 0},
			{"feature:two", 1, []string{a, c}, 5, 1},
		}},
		"a new choice bridges two pools": {plain: []string{"feature:o1", "feature:o3", "feature:o2"}, entries: []poolEntry{
			{"feature:p", 1, []string{"feature:o1", "feature:o2"}, 1, 0},
			{"feature:q", 1, []string{"feature:o3", "feature:o4"}, 2, 1},
			{"feature:bridge", 1, []string{"feature:o2", "feature:o3"}, 3, 2},
		}},
		"a bridge between three pools": {plain: []string{"feature:o4", "feature:o1"}, entries: []poolEntry{
			{"feature:p", 1, []string{"feature:o1"}, 1, 0},
			{"feature:q", 1, []string{"feature:o2"}, 2, 1},
			{"feature:r", 1, []string{"feature:o3"}, 3, 2},
			{"feature:bridge", 2, []string{"feature:o1", "feature:o2", "feature:o3", "feature:o4"}, 4, 3},
		}},
		"the same option by name from two classes": {plain: []string{"feature:fighter-defense", "feature:paladin-defense"}, entries: []poolEntry{
			{"feature:fighter-style", 1, []string{"feature:fighter-defense", a}, 1, 0},
			{"feature:paladin-style", 1, []string{"feature:paladin-defense", b}, 2, 1},
		}},
		"the same name taken by the later class only": {plain: []string{"feature:paladin-defense"}, entries: []poolEntry{
			{"feature:fighter-style", 1, []string{"feature:fighter-defense", a}, 1, 0},
			{"feature:paladin-style", 1, []string{"feature:paladin-defense", b}, 2, 1},
		}},
		"the earlier option wins the name whatever the entry order": {plain: []string{"feature:fighter-defense", "feature:paladin-defense"}, entries: []poolEntry{
			{"feature:paladin-style", 1, []string{"feature:paladin-defense", b}, 2, 7},
			{"feature:fighter-style", 1, []string{"feature:fighter-defense", a}, 1, 3},
		}},
		"an option already taken in the same pool": {plain: []string{a}, entries: []poolEntry{
			{"feature:fs", 1, []string{a, b}, 1, 0},
			{"feature:fs2", 1, []string{a, b}, 9, 1},
			{"feature:fs3", 1, []string{a, b}, 12, 2},
		}},
		"picked options come out in the option order": {plain: []string{"feature:meta-z", "feature:meta-x"}, entries: []poolEntry{
			{"feature:metamagic", 2, []string{"feature:meta-x", "feature:meta-y", "feature:meta-z"}, 3, 0},
		}},
		"a choice without options": {plain: []string{a}, entries: []poolEntry{
			{"feature:empty", 1, nil, 1, 0},
			{"feature:fs", 1, []string{a}, 2, 1},
		}},
		"a choice that takes nothing": {plain: []string{a, b}, entries: []poolEntry{
			{"feature:none", 0, []string{a, b}, 1, 0},
		}},
	}
}

// poolDump is what assignPools leaves on a choice.
type poolDump struct {
	Key      string
	Picked   []string
	Overflow []string
	Unmet    []string
	Options  []ChoiceOption
}

func runPoolScenario(s poolScenario) (string, error) {
	cb := &choiceBuilder{c: &content{namesPT: poolNames}, plain: s.plain, used: map[string]bool{}}
	for _, e := range s.entries {
		ch := &Choice{Key: e.key, FeatureKey: e.key}
		cb.all = append(cb.all, ch)
		cb.entries = append(cb.entries, &optionEntry{choice: ch, choose: e.choose, options: e.options, level: e.level, order: e.order})
	}
	cb.assignPools()
	var out struct {
		Choices []poolDump
		Used    map[string]bool
	}
	out.Used = cb.used
	for _, ch := range cb.all {
		out.Choices = append(out.Choices, poolDump{ch.Key, ch.Picked, ch.Overflow, ch.Unmet, ch.Options})
	}
	b, err := json.Marshal(out)
	return string(b), err
}

// TestAssignPoolsCharacterization freezes what assignPools deals: the picks of
// each choice, the overflow, the unmet picks, the options with their taken
// prerequisites, and the picks it marks used.
func TestAssignPoolsCharacterization(t *testing.T) {
	t.Parallel()
	got := map[string]string{}
	for name, s := range poolScenarios() {
		text, err := runPoolScenario(s)
		if err != nil {
			t.Fatal(err)
		}
		got[name] = text
	}
	goldenText(t, "assignpools", got)
}
