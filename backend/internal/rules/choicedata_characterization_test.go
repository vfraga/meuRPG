package rules

import (
	"maps"
	"strings"
	"testing"
	"testing/fstest"
)

// choiceDataEdits corrupts effects/choices.json one invariant at a time (and a
// few pairs, for the order of the checks).
func choiceDataEdits() map[string]func(d map[string]any) {
	row := func(d map[string]any, key string, i int) map[string]any { return rows(d, key)[i].(map[string]any) }
	sub := func(d map[string]any, key string) map[string]any { return d[key].(map[string]any) }
	dice := func(d map[string]any) []any { return sub(d, "breath_weapon")["dice"].([]any) }
	summaries := func(d map[string]any) map[string]any { return sub(d, "option_summaries") }
	return map[string]func(d map[string]any){
		"ok: as shipped":                 func(_ map[string]any) {},
		"bad: unknown field":             func(d map[string]any) { d["surprise"] = 1 },
		"bad: ancestry without a source": func(d map[string]any) { delete(row(d, "draconic_ancestry", 0), "source") },
		"bad: ancestry unknown trait":    func(d map[string]any) { row(d, "draconic_ancestry", 0)["trait"] = "trait:nope" },
		"bad: ancestry unknown feature":  func(d map[string]any) { row(d, "draconic_ancestry", 0)["dragon_ancestor"] = "feature:nope" },
		"bad: ancestry damage unknown":   func(d map[string]any) { row(d, "draconic_ancestry", 0)["damage_type"] = "damage-type:nope" },
		"bad: ancestry damage is a condition": func(d map[string]any) {
			row(d, "draconic_ancestry", 0)["damage_type"] = "condition:deafened"
		},
		"bad: ancestry shape":              func(d map[string]any) { row(d, "draconic_ancestry", 0)["shape"] = "sphere" },
		"bad: ancestry size 0":             func(d map[string]any) { row(d, "draconic_ancestry", 0)["size_ft"] = 0 },
		"bad: line without a width":        func(d map[string]any) { delete(row(d, "draconic_ancestry", 0), "width_ft") },
		"bad: cone with a width":           func(d map[string]any) { row(d, "draconic_ancestry", 5)["width_ft"] = 5 },
		"bad: ancestry save is no ability": func(d map[string]any) { row(d, "draconic_ancestry", 0)["save"] = "luck" },
		"bad: ancestry twice by trait": func(d map[string]any) {
			r := map[string]any{}
			maps.Copy(r, row(d, "draconic_ancestry", 0))
			r["color"] = "again"
			r["dragon_ancestor"] = row(d, "draconic_ancestry", 1)["dragon_ancestor"]
			d["draconic_ancestry"] = append(rows(d, "draconic_ancestry"), r)
		},
		"bad: ancestry twice": func(d map[string]any) {
			d["draconic_ancestry"] = append(rows(d, "draconic_ancestry"), row(d, "draconic_ancestry", 0))
		},
		"order: source before trait": func(d map[string]any) {
			delete(row(d, "draconic_ancestry", 0), "source")
			row(d, "draconic_ancestry", 0)["trait"] = "trait:nope"
		},
		"order: trait before feature": func(d map[string]any) {
			row(d, "draconic_ancestry", 0)["trait"] = "trait:nope"
			row(d, "draconic_ancestry", 0)["dragon_ancestor"] = "feature:nope"
		},
		"order: shape before size": func(d map[string]any) {
			row(d, "draconic_ancestry", 0)["shape"] = "sphere"
			row(d, "draconic_ancestry", 0)["size_ft"] = 0
		},
		"order: size before save": func(d map[string]any) {
			row(d, "draconic_ancestry", 0)["size_ft"] = 0
			row(d, "draconic_ancestry", 0)["save"] = "luck"
		},
		"order: first ancestry first": func(d map[string]any) {
			row(d, "draconic_ancestry", 0)["save"] = "luck"
			row(d, "draconic_ancestry", 1)["trait"] = "trait:nope"
		},
		"bad: breath trait unknown":   func(d map[string]any) { sub(d, "breath_weapon")["trait"] = "trait:nope" },
		"bad: breath without source":  func(d map[string]any) { delete(sub(d, "breath_weapon"), "source") },
		"bad: breath without dice":    func(d map[string]any) { sub(d, "breath_weapon")["dice"] = []any{} },
		"bad: breath dice not dice":   func(d map[string]any) { dice(d)[0].(map[string]any)["dice"] = "lots" },
		"bad: breath level 0":         func(d map[string]any) { dice(d)[0].(map[string]any)["level"] = 0 },
		"bad: breath level 21":        func(d map[string]any) { dice(d)[3].(map[string]any)["level"] = MaxLevel + 1 },
		"bad: breath levels not rise": func(d map[string]any) { dice(d)[1].(map[string]any)["level"] = 1 },
		"order: breath trait before dice": func(d map[string]any) {
			sub(d, "breath_weapon")["trait"] = "trait:nope"
			dice(d)[0].(map[string]any)["dice"] = "lots"
		},
		"order: ancestry before breath": func(d map[string]any) {
			row(d, "draconic_ancestry", 0)["save"] = "luck"
			sub(d, "breath_weapon")["trait"] = "trait:nope"
		},
		"bad: invocation without a source": func(d map[string]any) { delete(row(d, "invocation_prerequisites", 0), "source") },
		"bad: invocation not an invocation": func(d map[string]any) {
			row(d, "invocation_prerequisites", 0)["invocation"] = "feature:second-wind"
		},
		"bad: invocation unknown": func(d map[string]any) { row(d, "invocation_prerequisites", 0)["invocation"] = "feature:nope" },
		"bad: invocation twice": func(d map[string]any) {
			d["invocation_prerequisites"] = append(rows(d, "invocation_prerequisites"), row(d, "invocation_prerequisites", 0))
		},
		"bad: invocation spell unknown": func(d map[string]any) { row(d, "invocation_prerequisites", 0)["spell"] = "spell:nope" },
		"bad: invocation spell leveled": func(d map[string]any) { row(d, "invocation_prerequisites", 0)["spell"] = "spell:fireball" },
		"bad: invocation feature not a boon": func(d map[string]any) {
			row(d, "invocation_prerequisites", 0)["feature"] = "feature:second-wind"
		},
		"bad: invocation level 21":       func(d map[string]any) { row(d, "invocation_prerequisites", 0)["level"] = MaxLevel + 1 },
		"bad: invocation level negative": func(d map[string]any) { row(d, "invocation_prerequisites", 0)["level"] = -1 },
		"bad: invocation without a row":  func(d map[string]any) { d["invocation_prerequisites"] = rows(d, "invocation_prerequisites")[1:] },
		"order: invocation source before kind": func(d map[string]any) {
			delete(row(d, "invocation_prerequisites", 0), "source")
			row(d, "invocation_prerequisites", 0)["invocation"] = "feature:nope"
		},
		"order: invocation spell before feature": func(d map[string]any) {
			row(d, "invocation_prerequisites", 0)["spell"] = "spell:nope"
			row(d, "invocation_prerequisites", 0)["feature"] = "feature:nope"
		},
		"order: invocation feature before level": func(d map[string]any) {
			row(d, "invocation_prerequisites", 0)["feature"] = "feature:nope"
			row(d, "invocation_prerequisites", 0)["level"] = 99
		},
		"order: breath before invocation": func(d map[string]any) {
			sub(d, "breath_weapon")["trait"] = "trait:nope"
			delete(row(d, "invocation_prerequisites", 0), "source")
		},
		"order: a row before the missing row": func(d map[string]any) {
			d["invocation_prerequisites"] = rows(d, "invocation_prerequisites")[1:]
			row(d, "invocation_prerequisites", 0)["level"] = 99
		},
		"bad: enemies without a source": func(d map[string]any) { delete(sub(d, "favored_enemy"), "source") },
		"bad: enemies without types":    func(d map[string]any) { sub(d, "favored_enemy")["types"] = []any{} },
		"bad: enemies without humanoid": func(d map[string]any) { sub(d, "favored_enemy")["humanoid"] = "" },
		"bad: enemies humanoid races 0": func(d map[string]any) { sub(d, "favored_enemy")["humanoid_races"] = 0 },
		"bad: enemies repeated": func(d map[string]any) {
			sub(d, "favored_enemy")["types"] = []any{"creature-type:beast", "creature-type:beast"}
		},
		"bad: enemy not a creature type":    func(d map[string]any) { sub(d, "favored_enemy")["types"] = []any{"beast"} },
		"bad: humanoid not a creature type": func(d map[string]any) { sub(d, "favored_enemy")["humanoid"] = "humanoid" },
		"order: enemies shape before kind": func(d map[string]any) {
			sub(d, "favored_enemy")["types"] = []any{"beast"}
			delete(sub(d, "favored_enemy"), "source")
		},
		"bad: terrains without a source": func(d map[string]any) { delete(sub(d, "natural_explorer"), "source") },
		"bad: terrains empty":            func(d map[string]any) { sub(d, "natural_explorer")["terrains"] = []any{} },
		"bad: terrain not a terrain":     func(d map[string]any) { sub(d, "natural_explorer")["terrains"] = []any{"forest"} },
		"bad: terrain twice": func(d map[string]any) {
			sub(d, "natural_explorer")["terrains"] = []any{"terrain:forest", "terrain:forest"}
		},
		"order: enemies before terrains": func(d map[string]any) {
			sub(d, "favored_enemy")["types"] = []any{"beast"}
			sub(d, "natural_explorer")["terrains"] = []any{"forest"}
		},
		"bad: resistance unknown trait":  func(d map[string]any) { row(d, "racial_resistances", 0)["trait"] = "trait:nope" },
		"bad: resistance unknown damage": func(d map[string]any) { row(d, "racial_resistances", 0)["damage_type"] = "damage-type:nope" },
		"bad: resistance without source": func(d map[string]any) { delete(row(d, "racial_resistances", 0), "source") },
		"order: terrains before resistances": func(d map[string]any) {
			sub(d, "natural_explorer")["terrains"] = []any{"forest"}
			delete(row(d, "racial_resistances", 0), "source")
		},
		"bad: summary of a non-feature": func(d map[string]any) { summaries(d)["feature:nope"] = "Nada." },
		"bad: summary blank": func(d map[string]any) {
			summaries(d)["feature:fighter-fighting-style-archery"] = "   "
		},
		"bad: option without a summary": func(d map[string]any) {
			delete(summaries(d), "feature:fighter-fighting-style-archery")
		},
		"order: resistances before summaries": func(d map[string]any) {
			delete(row(d, "racial_resistances", 0), "source")
			summaries(d)["feature:nope"] = "Nada."
		},
		"order: bad summary before missing summary": func(d map[string]any) {
			summaries(d)["feature:nope"] = "Nada."
			delete(summaries(d), "feature:fighter-fighting-style-archery")
		},
	}
}

// TestLoadChoiceDataCharacterization freezes every refusal of loadChoiceData:
// the text and the order of the checks. A refused load leaves c.choices alone.
func TestLoadChoiceDataCharacterization(t *testing.T) {
	t.Parallel()
	loaded := loadForTest(t).c
	got := map[string]string{}
	for name, edit := range choiceDataEdits() {
		c := *loaded
		c.choices = nil
		err := c.loadChoiceData(mutatedChoices(t, edit))
		got[name] = errText(err)
		if strings.HasPrefix(name, "ok:") != (err == nil) {
			t.Errorf("%s: %v", name, err)
		}
		if (c.choices != nil) != (err == nil) {
			t.Errorf("%s: choices set = %v with err %v", name, c.choices != nil, err)
		}
	}
	// A key the file introduces needs a Portuguese name.
	for _, key := range []string{"creature-type:beast", "creature-type:humanoid", "terrain:forest"} {
		c := *loaded
		c.namesPT = maps.Clone(loaded.namesPT)
		delete(c.namesPT, key)
		got["bad: no Portuguese name for "+key] = errText(c.loadChoiceData(mutatedChoices(t, func(map[string]any) {})))
	}
	// Without the file, and with a file that is not JSON.
	c := *loaded
	got["bad: no file"] = errText(c.loadChoiceData(fstest.MapFS{}))
	got["bad: not json"] = errText(c.loadChoiceData(fstest.MapFS{"effects/choices.json": {Data: []byte("{")}}))
	goldenText(t, "choicedata-refusals", got)
}
