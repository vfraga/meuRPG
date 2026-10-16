package rules

import (
	"encoding/json"
	"strings"
	"testing"
	"testing/fstest"

	"github.com/PuraFome/meuRPG/backend/internal/rules/srd51"
)

// correctionsContent is a small content that has one of everything the
// sections of effects/corrections.json look up.
func correctionsContent() *content {
	rows := func(specific string) []*srd51.Level {
		out := make([]*srd51.Level, MaxLevel)
		for i := range out {
			out[i] = &srd51.Level{Class: "x", Level: i + 1}
			if specific != "" {
				out[i].ClassSpecific = []byte(specific)
			}
		}
		return out
	}
	return &content{
		classes: map[string]*srd51.Class{
			"class:bard":   {Key: "class:bard"},
			"class:cleric": {Key: "class:cleric"},
		},
		classLevels: map[string][]*srd51.Level{
			"class:warlock": rows(`{"invocations_known":9}`),
			"class:wizard":  rows(`{"arcane_recovery":1}`),
			"class:fighter": rows(""),
		},
		proficiencies: map[string]*srd51.Proficiency{
			"proficiency:lute":   {Key: "proficiency:lute", Kind: "tool"},
			"proficiency:flute":  {Key: "proficiency:flute", Kind: "tool"},
			"proficiency:shield": {Key: "proficiency:shield", Kind: "armor"},
		},
		skills: map[string]*srd51.Skill{"skill:perception": {}},
		named: map[string]*srd51.Named{
			"damage-type:fire": {}, "damage-type:cold": {}, "condition:deafened": {}, "damage-type:ghost": nil,
		},
		monsters: map[string]*srd51.Monster{
			"monster:veteran": {HitDice: "6d8", HitPoints: 22, Actions: []srd51.MonsterAction{{Name: "Multiattack", Multiattack: [][]srd51.MonsterAttackCount{{{Name: "Longsword", Count: 2, Kind: "melee"}}}}}},
			"monster:rat":     {HitDice: "1d4", Actions: []srd51.MonsterAction{{Name: "Bite"}}},
		},
		spells: map[string]*srd51.Spell{
			"spell:ray":    {Key: "spell:ray", Level: 2, Classes: []string{"class:cleric"}},
			"spell:ward":   {Key: "spell:ward", Level: 3},
			"spell:spark":  {Key: "spell:spark", Level: 0},
			"spell:strike": {Key: "spell:strike", Level: 5, Damage: []srd51.SpellDamage{{DamageType: "damage-type:fire"}, {DamageType: "damage-type:cold"}}},
		},
		features: map[string]*srd51.Feature{
			"feature:a": {Key: "feature:a", Subclass: "subclass:life"},
			"feature:b": {Key: "feature:b", Subclass: "subclass:life"},
			"feature:z": {Key: "feature:z", Subclass: "subclass:other"},
		},
		subclasses: map[string]*srd51.Subclass{
			"subclass:life":  {Key: "subclass:life", Class: "class:cleric", Spells: []srd51.SubclassSpell{{Spell: "spell:ray", ClassLevel: 3}}},
			"subclass:empty": {Key: "subclass:empty", Class: "class:cleric"},
		},
		subclassLevels: map[string]map[int]*srd51.Level{
			"subclass:life": {1: {Class: "class:cleric", Subclass: "subclass:life", Level: 1, Features: []string{"feature:a"}}},
		},
	}
}

// correctionsState is what the corrections can change, for the golden.
func correctionsState(c *content) string {
	type spell struct {
		Classes                       []string
		Attack, Save, Success, Choice string
		Damage                        []srd51.SpellDamage
	}
	type sub struct {
		Spells   []srd51.SubclassSpell
		Expanded bool
	}
	state := struct {
		Spells     map[string]spell
		Subclasses map[string]sub
		SubRows    map[string]map[int]*srd51.Level
		Monsters   map[string]*srd51.Monster
		Instrument map[string]*srd51.Choice
		Attacks    map[string]int
		Columns    map[string][]string
	}{
		Spells: map[string]spell{}, Subclasses: map[string]sub{}, SubRows: c.subclassLevels, Monsters: c.monsters,
		Instrument: map[string]*srd51.Choice{}, Attacks: c.attacksPerAction, Columns: map[string][]string{},
	}
	for k, s := range c.spells {
		state.Spells[k] = spell{s.Classes, s.AttackType, s.SaveAbility, s.SaveSuccess, s.DamageChoice, s.Damage}
	}
	for k, s := range c.subclasses {
		state.Subclasses[k] = sub{s.Spells, s.ExpandedList}
	}
	for k, cl := range c.classes {
		state.Instrument[k] = cl.Multiclass.InstrumentChoices
	}
	for k, rows := range c.classLevels {
		for _, r := range rows {
			state.Columns[k] = append(state.Columns[k], string(r.ClassSpecific))
		}
	}
	b, err := json.Marshal(state)
	if err != nil {
		return err.Error()
	}
	return string(b)
}

func correctionsCases() map[string]string {
	cases := map[string]string{}
	sec := func(name, section, body string) { cases[name] = `{"` + section + `":[` + body + `]}` }

	cases["bad: not json"] = `{"corrections":`
	cases["bad: unknown field"] = `{"loud":1}`
	cases["ok: nothing"] = `{}`
	cases["ok: comment only"] = `{"_comment":"x"}`

	// spell_list_corrections
	const sl = "spell_list_corrections"
	sec("bad: list of an unknown class", sl, `{"class":"class:nope","add":["spell:ward"],"source":"SRD"}`)
	sec("bad: list without a source", sl, `{"class":"class:bard","add":["spell:ward"]}`)
	sec("bad: list corrected twice", sl, `{"class":"class:bard","add":["spell:ward"],"source":"SRD"},{"class":"class:bard","add":["spell:spark"],"source":"SRD"}`)
	sec("bad: list that corrects nothing", sl, `{"class":"class:bard","source":"SRD"}`)
	sec("bad: list with an unknown spell", sl, `{"class":"class:bard","add":["spell:nope"],"source":"SRD"}`)
	sec("bad: list naming a spell twice", sl, `{"class":"class:bard","add":["spell:ward"],"remove":["spell:ward"],"source":"SRD"}`)
	sec("bad: list adding what is there", sl, `{"class":"class:cleric","add":["spell:ray"],"source":"SRD"}`)
	sec("bad: list removing what is not there", sl, `{"class":"class:bard","remove":["spell:ray"],"source":"SRD"}`)
	sec("ok: list add and remove", sl, `{"class":"class:bard","add":["spell:ward"],"source":"SRD"},{"class":"class:cleric","remove":["spell:ray"],"add":["spell:spark"],"source":"SRD"}`)

	// subclass_feature_corrections
	const sf = "subclass_feature_corrections"
	sec("bad: row of an unknown subclass", sf, `{"subclass":"subclass:nope","level":3,"add_features":["feature:a"],"source":"SRD"}`)
	sec("bad: row without a source", sf, `{"subclass":"subclass:life","level":3,"add_features":["feature:a"]}`)
	sec("bad: row without features", sf, `{"subclass":"subclass:life","level":3,"source":"SRD"}`)
	sec("bad: row at level 0", sf, `{"subclass":"subclass:life","level":0,"add_features":["feature:a"],"source":"SRD"}`)
	sec("bad: row at level 21", sf, `{"subclass":"subclass:life","level":21,"add_features":["feature:a"],"source":"SRD"}`)
	sec("bad: row corrected twice", sf, `{"subclass":"subclass:life","level":3,"add_features":["feature:b"],"source":"SRD"},{"subclass":"subclass:life","level":3,"add_features":["feature:a"],"source":"SRD"}`)
	sec("bad: row with an unknown feature", sf, `{"subclass":"subclass:life","level":3,"add_features":["feature:nope"],"source":"SRD"}`)
	sec("bad: row with a feature of another subclass", sf, `{"subclass":"subclass:life","level":3,"add_features":["feature:z"],"source":"SRD"}`)
	sec("bad: row that already has the feature", sf, `{"subclass":"subclass:life","level":1,"add_features":["feature:a"],"source":"SRD"}`)
	sec("ok: row created and row extended", sf, `{"subclass":"subclass:life","level":3,"add_features":["feature:a"],"source":"SRD"},{"subclass":"subclass:life","level":1,"add_features":["feature:b"],"source":"SRD"}`)

	// creature_corrections
	const cc = "creature_corrections"
	cre := func(creature, field, value, source string) string {
		src := ""
		if source != "" {
			src = `,"source":"` + source + `"`
		}
		return `{"creature":"` + creature + `","field":"` + field + `","value":` + value + src + `}`
	}
	sec("bad: creature unknown", cc, cre("monster:nope", "armor_class", "12", "SRD"))
	sec("bad: creature field that cannot be corrected", cc, cre("monster:rat", "challenge_rating", "2", "SRD"))
	sec("bad: creature without a source", cc, cre("monster:rat", "armor_class", "12", ""))
	sec("bad: creature corrected twice", cc, cre("monster:rat", "armor_class", "12", "SRD")+","+cre("monster:rat", "armor_class", "13", "SRD"))
	sec("bad: creature number out of range", cc, cre("monster:rat", "armor_class", "0", "SRD"))
	sec("bad: creature number not a number", cc, cre("monster:rat", "speed_walk", `"fast"`, "SRD"))
	sec("bad: creature without Multiattack", cc, cre("monster:rat", "attacks_per_action", "2", "SRD"))
	sec("bad: creature attacks above 20", cc, cre("monster:veteran", "attacks_per_action", "21", "SRD"))
	sec("bad: creature roll of other dice", cc, cre("monster:veteran", "hit_points_roll", `"5d8+5"`, "SRD"))
	sec("bad: creature roll not a roll", cc, cre("monster:veteran", "hit_points_roll", `"many"`, "SRD"))
	sec("bad: creature roll not a string", cc, cre("monster:veteran", "hit_points_roll", `7`, "SRD"))
	sec("bad: creature skills empty", cc, cre("monster:rat", "skills", `{}`, "SRD"))
	sec("bad: creature skill unknown", cc, cre("monster:rat", "skills", `{"skill:nope":2}`, "SRD"))
	sec("bad: creature immunity list empty", cc, cre("monster:rat", "damage_immunities", `[]`, "SRD"))
	sec("bad: creature immunity unknown", cc, cre("monster:rat", "damage_immunities", `["damage-type:nope"]`, "SRD"))
	sec("bad: creature immunity nil entry", cc, cre("monster:rat", "damage_immunities", `["damage-type:ghost"]`, "SRD"))
	sec("bad: creature condition list twice", cc, cre("monster:rat", "condition_immunities", `["condition:deafened","condition:deafened"]`, "SRD"))
	sec("bad: creature condition of a damage type", cc, cre("monster:rat", "condition_immunities", `["damage-type:cold"]`, "SRD"))
	sec("ok: creature fields", cc, cre("monster:veteran", "attacks_per_action", "3", "SRD")+","+cre("monster:veteran", "hit_points_roll", `"6d8+6"`, "SRD")+","+
		cre("monster:rat", "speed_swim", "30", "SRD")+","+cre("monster:rat", "skills", `{"skill:perception":3}`, "SRD")+","+
		cre("monster:rat", "damage_immunities", `["damage-type:cold"]`, "SRD")+","+cre("monster:rat", "condition_immunities", `["condition:deafened"]`, "SRD"))

	// spell_corrections
	const sc = "spell_corrections"
	sp := func(body string) string { return `{"spell":"spell:ray",` + body + `,"source":"SRD"}` }
	sec("bad: spell unknown", sc, `{"spell":"spell:nope","attack_type":"ranged","source":"SRD"}`)
	sec("bad: spell without a source", sc, `{"spell":"spell:ray","attack_type":"ranged"}`)
	sec("bad: spell corrected twice", sc, sp(`"attack_type":"ranged"`)+","+sp(`"attack_type":"melee"`))
	sec("bad: spell that corrects nothing", sc, sp(`"attack_type":""`))
	sec("bad: spell attack that is not a type", sc, sp(`"attack_type":"thrown"`))
	sec("bad: spell save that is not an ability", sc, sp(`"save_ability":"luck","save_success":"half"`))
	sec("bad: spell save without an outcome", sc, sp(`"save_ability":"dex"`))
	sec("bad: spell save with an unknown outcome", sc, sp(`"save_ability":"dex","save_success":"luck"`))
	sec("bad: spell outcome without a save", sc, sp(`"save_success":"half","attack_type":"melee"`))
	sec("bad: spell damage unknown type", sc, sp(`"damage":[{"damage_type":"damage-type:nope","at_slot_level":{"2":"2d6"}}]`))
	sec("bad: spell damage nil type", sc, sp(`"damage":[{"damage_type":"damage-type:ghost","at_slot_level":{"2":"2d6"}}]`))
	sec("bad: spell damage of a condition", sc, sp(`"damage":[{"damage_type":"condition:deafened","at_slot_level":{"2":"2d6"}}]`))
	sec("bad: spell damage without slot levels", sc, sp(`"damage":[{"damage_type":"damage-type:fire","at_slot_level":{}}]`))
	sec("bad: spell slot below the spell", sc, sp(`"damage":[{"damage_type":"damage-type:fire","at_slot_level":{"1":"2d6"}}]`))
	sec("bad: spell slot above nine", sc, sp(`"damage":[{"damage_type":"damage-type:fire","at_slot_level":{"10":"2d6"}}]`))
	sec("bad: spell slot not a number", sc, sp(`"damage":[{"damage_type":"damage-type:fire","at_slot_level":{"two":"2d6"}}]`))
	sec("bad: spell dice that are not dice", sc, sp(`"damage":[{"damage_type":"damage-type:fire","at_slot_level":{"2":"lots"}}]`))
	sec("bad: cantrip slot 0", sc, `{"spell":"spell:spark","damage":[{"damage_type":"damage-type:fire","at_slot_level":{"0":"1d10"}}],"source":"SRD"}`)
	sec("bad: second damage entry is the bad one", sc, sp(`"damage":[{"damage_type":"damage-type:fire","at_slot_level":{"2":"2d6"}},{"damage_type":"damage-type:cold","at_slot_level":{}}]`))
	sec("bad: damage choice that is unknown", sc, `{"spell":"spell:strike","damage_choice":"both","source":"SRD"}`)
	sec("bad: damage choice of one damage type", sc, sp(`"damage_choice":"scale"`))
	sec("bad: damage choice with one corrected type", sc, `{"spell":"spell:strike","damage":[{"damage_type":"damage-type:fire","at_slot_level":{"5":"2d6"}}],"damage_choice":"scale","source":"SRD"}`)
	sec("ok: spell attack", sc, sp(`"attack_type":"melee"`))
	sec("ok: spell save", sc, sp(`"save_ability":"wis","save_success":"other"`))
	sec("ok: spell empty damage list", sc, sp(`"damage":[]`))
	sec("ok: spell everything", sc, sp(`"attack_type":"ranged","save_ability":"dex","save_success":"half","damage":[{"damage_type":"damage-type:fire","at_slot_level":{"2":"2d6","3":"3d6"}}]`))
	sec("ok: cantrip damage", sc, `{"spell":"spell:spark","damage":[{"damage_type":"damage-type:fire","at_slot_level":{"1":"1d10"}}],"source":"SRD"}`)
	sec("ok: damage choice scale", sc, `{"spell":"spell:strike","damage_choice":"scale","source":"SRD"}`)
	sec("ok: damage choice alternative", sc, `{"spell":"spell:strike","damage_choice":"alternative","source":"SRD"}`)

	// multiclass_corrections
	const mc = "multiclass_corrections"
	mcc := func(body string) string {
		return `{"class":"class:bard","field":"instrument_choices","source":"SRD",` + body + `}`
	}
	sec("bad: multiclass of an unknown class", mc, `{"class":"class:nope","field":"instrument_choices","source":"SRD","choose":1,"options":["proficiency:lute"]}`)
	sec("bad: multiclass field that cannot be corrected", mc, `{"class":"class:bard","field":"hit_die","source":"SRD","choose":1,"options":["proficiency:lute"]}`)
	sec("bad: multiclass without a source", mc, `{"class":"class:bard","field":"instrument_choices","choose":1,"options":["proficiency:lute"]}`)
	sec("bad: multiclass corrected twice", mc, mcc(`"choose":1,"options":["proficiency:lute"]`)+","+mcc(`"choose":1,"options":["proficiency:flute"]`))
	sec("bad: multiclass choosing 0", mc, mcc(`"choose":0,"options":["proficiency:lute"]`))
	sec("bad: multiclass choosing more than the options", mc, mcc(`"choose":3,"options":["proficiency:lute","proficiency:flute"]`))
	sec("bad: multiclass option that is unknown", mc, mcc(`"choose":1,"options":["proficiency:nope"]`))
	sec("bad: multiclass option that is not a tool", mc, mcc(`"choose":1,"options":["proficiency:shield"]`))
	sec("bad: multiclass option listed twice", mc, mcc(`"choose":1,"options":["proficiency:lute","proficiency:lute"]`))
	sec("ok: multiclass", mc, mcc(`"choose":2,"options":["proficiency:lute","proficiency:flute"]`))

	// expanded_list_corrections
	const el = "expanded_list_corrections"
	sec("bad: expanded list of an unknown subclass", el, `{"subclass":"subclass:nope","source":"SRD"}`)
	sec("bad: expanded list without a source", el, `{"subclass":"subclass:life"}`)
	sec("bad: expanded list of a subclass without spells", el, `{"subclass":"subclass:empty","source":"SRD"}`)
	sec("bad: expanded list twice", el, `{"subclass":"subclass:life","source":"SRD"},{"subclass":"subclass:life","source":"SRD"}`)
	sec("ok: expanded list", el, `{"subclass":"subclass:life","source":"SRD"}`)

	// subclass_corrections
	const sub = "subclass_corrections"
	sc2 := func(body string) string { return `{"subclass":"subclass:life",` + body + `}` }
	sec("bad: subclass unknown", sub, `{"subclass":"subclass:nope","add_spells":[{"spell":"spell:ward","class_level":7}],"source":"SRD"}`)
	sec("bad: subclass without a source", sub, sc2(`"add_spells":[{"spell":"spell:ward","class_level":7}]`))
	sec("bad: subclass without spells", sub, sc2(`"source":"SRD"`))
	sec("bad: subclass corrected twice", sub, sc2(`"add_spells":[{"spell":"spell:ward","class_level":7}],"source":"SRD"`)+","+sc2(`"add_spells":[{"spell":"spell:spark","class_level":7}],"source":"SRD"`))
	sec("bad: subclass spell unknown", sub, sc2(`"add_spells":[{"spell":"spell:nope","class_level":7}],"source":"SRD"`))
	sec("bad: subclass level 0", sub, sc2(`"add_spells":[{"spell":"spell:ward","class_level":0}],"source":"SRD"`))
	sec("bad: subclass level 21", sub, sc2(`"add_spells":[{"spell":"spell:ward","class_level":21}],"source":"SRD"`))
	sec("bad: subclass spell already there", sub, sc2(`"add_spells":[{"spell":"spell:ray","class_level":3}],"source":"SRD"`))
	sec("bad: subclass spell added twice", sub, sc2(`"add_spells":[{"spell":"spell:ward","class_level":7},{"spell":"spell:ward","class_level":7}],"source":"SRD"`))
	sec("ok: subclass spells", sub, sc2(`"add_spells":[{"spell":"spell:ward","class_level":7},{"spell":"spell:ray","class_level":9}],"source":"SRD"`))

	// corrections (class table rows)
	const cr = "corrections"
	sec("bad: class unknown", cr, `{"class":"class:nope","field":"invocations_known","by_level":{"1":1}}`)
	sec("bad: class field that cannot be corrected", cr, `{"class":"class:warlock","field":"rage_damage","by_level":{"1":1}}`)
	sec("bad: class level not a number", cr, `{"class":"class:warlock","field":"invocations_known","by_level":{"one":1}}`)
	sec("bad: class level 0", cr, `{"class":"class:warlock","field":"invocations_known","by_level":{"0":1}}`)
	sec("bad: class level 21", cr, `{"class":"class:warlock","field":"invocations_known","by_level":{"21":1}}`)
	sec("bad: class row without columns", cr, `{"class":"class:fighter","field":"invocations_known","by_level":{"2":1}}`)
	sec("bad: class row without the column", cr, `{"class":"class:wizard","field":"invocations_known","by_level":{"2":1}}`)
	sec("ok: class rows", cr, `{"class":"class:warlock","field":"invocations_known","by_level":{"4":2,"20":8}}`)
	sec("ok: class without levels to correct", cr, `{"class":"class:warlock","field":"invocations_known","by_level":{}}`)

	// The order of the sections: the first one with a fault speaks.
	two := func(name, a, b string) { cases[name] = `{` + a + `,` + b + `}` }
	badList := `"spell_list_corrections":[{"class":"class:nope","add":["spell:ward"],"source":"SRD"}]`
	badRow := `"subclass_feature_corrections":[{"subclass":"subclass:nope","level":3,"add_features":["feature:a"],"source":"SRD"}]`
	badCre := `"creature_corrections":[` + cre("monster:nope", "armor_class", "12", "SRD") + `]`
	badSp := `"spell_corrections":[{"spell":"spell:nope","attack_type":"ranged","source":"SRD"}]`
	badMc := `"multiclass_corrections":[{"class":"class:nope","field":"instrument_choices","source":"SRD","choose":1,"options":["proficiency:lute"]}]`
	badEl := `"expanded_list_corrections":[{"subclass":"subclass:nope","source":"SRD"}]`
	badSub := `"subclass_corrections":[{"subclass":"subclass:nope","add_spells":[{"spell":"spell:ward","class_level":7}],"source":"SRD"}]`
	badCr := `"corrections":[{"class":"class:nope","field":"invocations_known","by_level":{"1":1}}]`
	two("bad: lists before rows", badRow, badList)
	two("bad: rows before creatures", badCre, badRow)
	two("bad: creatures before spells", badSp, badCre)
	two("bad: spells before multiclass", badMc, badSp)
	two("bad: multiclass before expanded lists", badEl, badMc)
	two("bad: expanded lists before subclasses", badSub, badEl)
	two("bad: subclasses before class rows", badCr, badSub)
	// A valid section before a faulty one has already been applied; the error is the faulty one's.
	two("bad: good spell then bad creature", `"spell_corrections":[`+sp(`"attack_type":"melee"`)+`]`, badCre)
	return cases
}

func TestApplyCorrectionsCharacterization(t *testing.T) {
	t.Parallel()
	got := map[string]string{}
	for name, doc := range correctionsCases() {
		c := correctionsContent()
		err := c.applyCorrections(fstest.MapFS{"effects/corrections.json": {Data: []byte(doc)}})
		text := errText(err)
		if err == nil || strings.HasPrefix(name, "bad: good") {
			text += " " + correctionsState(c)
		}
		if strings.HasPrefix(name, "ok:") != (err == nil) {
			t.Errorf("%s: %v", name, err)
		}
		got[name] = text
	}
	goldenText(t, "corrections-refusals", got)
}
