package rules

import (
	"encoding/json"
	"fmt"
	"strings"
	"testing"
	"testing/fstest"
)

// trapMembers writes the members of one trap in a fixed order. A key of over
// replaces the base value (raw JSON); the empty string removes the member.
func trapMembers(over map[string]string) string {
	base := map[string]string{
		"key": `"a"`, "kind": `"mechanical"`, "description_pt": `"x"`, "find_dc": `12`,
		"trigger": `"enter"`, "area_size": `1`, "targets": `"area"`,
	}
	order := []string{"key", "kind", "description_pt", "notice_dc", "find_dc", "find_dc_ours", "trigger", "area_size", "targets", "fall_ft", "damage", "conditions", "attack", "save", "loud"}
	var parts []string
	for _, k := range order {
		v, ok := over[k]
		if !ok {
			v, ok = base[k]
		}
		if ok && v != "" {
			parts = append(parts, fmt.Sprintf("%q:%s", k, v))
		}
	}
	return "{" + strings.Join(parts, ",") + "}"
}

const (
	trapPoison = `{"dice":"1d6","type":"damage-type:poison"}`
	trapFall20 = `[{"dice":"2d6","type":"damage-type:bludgeoning"}]`
)

// trapSave is a good saving throw with members replaced by over.
func trapSave(over map[string]string) string {
	base := map[string]string{
		"ability": `"dex"`, "dc": `15`, "applies_to": `"caught"`,
		"on_fail": `{"damage":[` + trapPoison + `]}`, "on_pass": `"half"`,
	}
	var parts []string
	for _, k := range []string{"ability", "dc", "applies_to", "on_fail", "on_pass"} {
		v, ok := over[k]
		if !ok {
			v = base[k]
		}
		if v != "" {
			parts = append(parts, fmt.Sprintf("%q:%s", k, v))
		}
	}
	return "{" + strings.Join(parts, ",") + "}"
}

type trapCase struct {
	doc string
	// names are the Portuguese names the content has (defaults to trap:a and trap:b).
	names []string
}

func trapCases() map[string]trapCase {
	cases := map[string]trapCase{}
	one := func(name string, over map[string]string) {
		cases[name] = trapCase{doc: trapFile(trapMembers(over))}
	}
	file := func(name, doc string) { cases[name] = trapCase{doc: doc} }

	// The file itself.
	file("bad: not json", `{"severity":`)
	one("bad: unknown field", map[string]string{"loud": "true"})
	one("ok: bare trap", nil)

	// The identity of a trap, in the order of the checks.
	one("bad: empty key", map[string]string{"key": `""`})
	one("bad: key with a space", map[string]string{"key": `"a b"`})
	one("bad: key with a colon", map[string]string{"key": `"trap:a"`})
	file("bad: repeated key", trapFile(trapMembers(nil)+","+trapMembers(nil)))
	one("bad: unknown kind", map[string]string{"kind": `"divine"`})
	one("bad: empty kind", map[string]string{"kind": `""`})
	one("ok: magic kind", map[string]string{"kind": `"magic"`})
	one("bad: blank description", map[string]string{"description_pt": `"  "`})
	one("bad: negative notice DC", map[string]string{"notice_dc": `-1`})
	one("bad: notice DC above 30", map[string]string{"notice_dc": `31`})
	one("ok: notice DC 30", map[string]string{"notice_dc": `30`})
	one("bad: find DC 0", map[string]string{"find_dc": `0`})
	one("bad: find DC above 30", map[string]string{"find_dc": `31`})
	one("bad: unknown trigger", map[string]string{"trigger": `"touch"`})
	one("ok: manual trigger", map[string]string{"trigger": `"manual"`})
	one("bad: area 0", map[string]string{"area_size": `0`})
	one("bad: area 5", map[string]string{"area_size": `5`})
	one("ok: area 4", map[string]string{"area_size": `4`})
	one("bad: unknown targets", map[string]string{"targets": `"all"`})
	one("ok: manual targets", map[string]string{"targets": `"manual"`})
	one("bad: negative fall", map[string]string{"fall_ft": `-10`})
	one("bad: fall not a multiple of 10", map[string]string{"fall_ft": `15`})
	one("bad: fall above 200", map[string]string{"fall_ft": `210`})
	// Two faults: the earlier check in the list speaks.
	one("bad: kind before description", map[string]string{"kind": `"divine"`, "description_pt": `""`})
	one("bad: description before DCs", map[string]string{"description_pt": `""`, "find_dc": `0`})
	one("bad: DCs before trigger", map[string]string{"find_dc": `0`, "trigger": `"touch"`})
	one("bad: trigger before area", map[string]string{"trigger": `"touch"`, "area_size": `0`})
	one("bad: area before targets", map[string]string{"area_size": `0`, "targets": `"all"`})
	one("bad: targets before fall", map[string]string{"targets": `"all"`, "fall_ft": `15`})
	cases["bad: no Portuguese name"] = trapCase{doc: trapFile(trapMembers(nil)), names: []string{}}
	cases["bad: a leftover Portuguese name"] = trapCase{doc: trapFile(trapMembers(nil)), names: []string{"trap:a", "trap:gone"}}
	cases["ok: a name that is not a trap is not a leftover"] = trapCase{doc: trapFile(trapMembers(nil)), names: []string{"trap:a", "spell:x"}}
	cases["bad: second trap is the bad one"] = trapCase{doc: trapFile(trapMembers(nil) + "," + trapMembers(map[string]string{"key": `"b"`, "kind": `"x"`}))}
	cases["ok: two traps"] = trapCase{doc: trapFile(trapMembers(nil) + "," + trapMembers(map[string]string{"key": `"b"`, "kind": `"magic"`}))}

	// Damage.
	one("ok: damage", map[string]string{"damage": `[` + trapPoison + `]`})
	one("ok: flat damage", map[string]string{"damage": `[{"dice":"1","type":"damage-type:poison"}]`})
	one("bad: damage not dice", map[string]string{"damage": `[{"dice":"lots","type":"damage-type:poison"}]`})
	one("bad: damage with a modifier", map[string]string{"damage": `[{"dice":"1d6 + MOD","type":"damage-type:poison"}]`})
	one("bad: damage of nothing", map[string]string{"damage": `[{"dice":"0","type":"damage-type:poison"}]`})
	one("bad: dice with a bonus", map[string]string{"damage": `[{"dice":"1d6+2","type":"damage-type:poison"}]`})
	one("bad: damage type unknown", map[string]string{"damage": `[{"dice":"1d6","type":"damage-type:love"}]`})
	one("bad: damage type is a condition", map[string]string{"damage": `[{"dice":"1d6","type":"condition:poisoned"}]`})
	one("bad: damage type without a prefix", map[string]string{"damage": `[{"dice":"1d6","type":"poison"}]`})
	one("bad: second damage is the bad one", map[string]string{"damage": `[` + trapPoison + `,{"dice":"1d6","type":"damage-type:love"}]`})

	// Pits.
	one("ok: pit", map[string]string{"fall_ft": `20`, "damage": trapFall20})
	one("bad: pit without damage", map[string]string{"fall_ft": `20`})
	one("bad: pit with other dice", map[string]string{"fall_ft": `20`, "damage": `[{"dice":"3d6","type":"damage-type:bludgeoning"}]`})
	one("bad: pit with other type", map[string]string{"fall_ft": `20`, "damage": `[{"dice":"2d6","type":"damage-type:poison"}]`})
	one("bad: pit whose first damage is not the fall", map[string]string{"fall_ft": `20`, "damage": `[` + trapPoison + `,{"dice":"2d6","type":"damage-type:bludgeoning"}]`})
	one("ok: damage and fall of 0", map[string]string{"fall_ft": `0`, "damage": trapFall20})
	one("bad: pit of 200 ft with 20d6 only", map[string]string{"fall_ft": `200`, "damage": `[{"dice":"19d6","type":"damage-type:bludgeoning"}]`})
	one("ok: pit of 200 ft", map[string]string{"fall_ft": `200`, "damage": `[{"dice":"20d6","type":"damage-type:bludgeoning"}]`})

	// Conditions.
	one("ok: conditions", map[string]string{"conditions": `[{"condition":"condition:prone"},{"condition":"condition:poisoned","duration_pt":"1 hora"}]`})
	one("bad: condition unknown", map[string]string{"conditions": `[{"condition":"condition:sleepy"}]`})
	one("bad: condition is a spell", map[string]string{"conditions": `[{"condition":"spell:x"}]`})
	one("bad: condition without a prefix", map[string]string{"conditions": `[{"condition":"prone"}]`})
	one("bad: second condition is the bad one", map[string]string{"conditions": `[{"condition":"condition:prone"},{"condition":"nope"}]`})

	// Attacks.
	const atkDmg = `{"dice":"1d4","type":"damage-type:piercing"}`
	atk := func(bonus, count, dmg string) string {
		return `{"bonus":` + bonus + `,"count":` + count + `,"damage":` + dmg + `}`
	}
	one("ok: attack", map[string]string{"attack": atk("8", "3", atkDmg)})
	one("ok: attack at the limits", map[string]string{"attack": atk("20", "10", atkDmg)})
	one("ok: attack with bonus 0", map[string]string{"attack": atk("0", "1", atkDmg)})
	one("bad: attack with a negative bonus", map[string]string{"attack": atk("-1", "1", atkDmg)})
	one("bad: attack with bonus 21", map[string]string{"attack": atk("21", "1", atkDmg)})
	one("bad: attack with count 0", map[string]string{"attack": atk("8", "0", atkDmg)})
	one("bad: attack with count 11", map[string]string{"attack": atk("8", "11", atkDmg)})
	one("bad: attack with bad dice", map[string]string{"attack": atk("8", "1", `{"dice":"lots","type":"damage-type:piercing"}`)})
	one("bad: attack with bad type", map[string]string{"attack": atk("8", "1", `{"dice":"1d4","type":"damage-type:love"}`)})

	// Saves, in the order of the checks.
	sv := func(over map[string]string) map[string]string { return map[string]string{"save": trapSave(over)} }
	withAtk := func(over map[string]string) map[string]string {
		m := sv(over)
		m["attack"] = atk("8", "1", atkDmg)
		return m
	}
	one("ok: save", sv(nil))
	one("ok: save for half", sv(map[string]string{"on_pass": `"half"`}))
	one("ok: save for none", sv(map[string]string{"on_pass": `"none"`}))
	one("ok: save of the hit with an attack", withAtk(map[string]string{"applies_to": `"hit"`}))
	one("ok: condition-only save", sv(map[string]string{"on_fail": `{"condition":"condition:prone","duration_pt":"1 hora"}`, "on_pass": `"none"`}))
	one("bad: save without an ability", sv(map[string]string{"ability": ``}))
	one("bad: save with an unknown ability", sv(map[string]string{"ability": `"luck"`}))
	one("bad: save with DC 0", sv(map[string]string{"dc": `0`}))
	one("bad: save with DC 31", sv(map[string]string{"dc": `31`}))
	one("bad: save with unknown applies_to", sv(map[string]string{"applies_to": `"all"`}))
	one("bad: save of the hit without an attack", sv(map[string]string{"applies_to": `"hit"`}))
	one("bad: save with unknown on_pass", sv(map[string]string{"on_pass": `"double"`}))
	one("bad: save with bad fail damage", sv(map[string]string{"on_fail": `{"damage":[{"dice":"x","type":"damage-type:poison"}]}`}))
	one("bad: save whose failure does nothing", sv(map[string]string{"on_fail": `{}`, "on_pass": `"none"`}))
	one("bad: save with a fail condition that is a damage type", sv(map[string]string{"on_fail": `{"condition":"damage-type:poison"}`, "on_pass": `"none"`}))
	one("bad: save with an unknown fail condition", sv(map[string]string{"on_fail": `{"condition":"condition:sleepy"}`, "on_pass": `"none"`}))
	one("bad: save with a duration and no condition", sv(map[string]string{"on_fail": `{"damage":[` + trapPoison + `],"duration_pt":"1 hora"}`, "on_pass": `"none"`}))
	one("bad: save for half of no damage", sv(map[string]string{"on_fail": `{"condition":"condition:prone"}`, "on_pass": `"half"`}))
	one("bad: ability before DC", sv(map[string]string{"ability": `"luck"`, "dc": `0`}))
	one("bad: DC before applies_to", sv(map[string]string{"dc": `0`, "applies_to": `"all"`}))
	one("bad: applies_to before hit needs attack", sv(map[string]string{"applies_to": `"all"`}))
	one("bad: hit needs attack before on_pass", sv(map[string]string{"applies_to": `"hit"`, "on_pass": `"double"`}))
	one("bad: on_pass before fail damage", sv(map[string]string{"on_pass": `"double"`, "on_fail": `{"damage":[{"dice":"x","type":"damage-type:poison"}]}`}))
	one("bad: fail damage before empty failure", sv(map[string]string{"on_fail": `{"damage":[{"dice":"x","type":"damage-type:poison"}]}`}))
	one("bad: empty failure before fail condition", sv(map[string]string{"on_fail": `{"duration_pt":"x"}`, "on_pass": `"none"`}))
	one("bad: fail condition before duration", sv(map[string]string{"on_fail": `{"condition":"nope","duration_pt":"x"}`, "on_pass": `"none"`}))
	one("bad: duration before half", sv(map[string]string{"on_fail": `{"condition":"condition:prone","duration_pt":""}`}))
	// Parts of one trap, in order: damage, pit, conditions, attack, save.
	one("bad: damage before pit", map[string]string{"fall_ft": `20`, "damage": `[{"dice":"x","type":"damage-type:poison"}]`})
	one("bad: pit before conditions", map[string]string{"fall_ft": `20`, "conditions": `[{"condition":"nope"}]`})
	one("bad: conditions before attack", map[string]string{"conditions": `[{"condition":"nope"}]`, "attack": atk("99", "1", atkDmg)})
	one("bad: attack before save", map[string]string{"attack": atk("99", "1", atkDmg), "save": trapSave(map[string]string{"ability": `"luck"`})})

	// Severity.
	sev := func(name, from, to string) {
		file(name, strings.Replace(trapFile(trapMembers(nil)), from, to, 1))
	}
	const (
		setback   = `{"key":"setback","name_pt":"Revés","save_dc":{"min":10,"max":11},"attack_bonus":{"min":3,"max":5}}`
		dangerous = `{"key":"dangerous","name_pt":"Perigosa","save_dc":{"min":12,"max":15},"attack_bonus":{"min":6,"max":8}}`
		deadly    = `{"key":"deadly","name_pt":"Mortal","save_dc":{"min":16,"max":20},"attack_bonus":{"min":9,"max":12}}`
	)
	const rows = `[{"from":1,"to":10,"setback":"1d10","dangerous":"2d10","deadly":"4d10"},{"from":11,"to":20,"setback":"2d10","dangerous":"4d10","deadly":"10d10"}]`
	levels := func(name string, ls ...string) {
		file(name, `{"severity":{"levels":[`+strings.Join(ls, ",")+`],"damage_by_level":`+rows+`},"traps":[`+trapMembers(nil)+`]}`)
	}
	levels("bad: severity with a missing level", setback, dangerous)
	levels("bad: severity with a fourth level", setback, dangerous, deadly, deadly)
	levels("bad: severity with no level")
	levels("ok: severity levels in one line", setback, dangerous, deadly)
	sev("bad: severity out of order", `"key":"setback"`, `"key":"deadly"`)
	sev("bad: severity without a name", `"name_pt":"Revés"`, `"name_pt":" "`)
	sev("bad: severity DC below 1", `"save_dc":{"min":10,"max":11}`, `"save_dc":{"min":0,"max":11}`)
	sev("bad: severity DC range reversed", `"save_dc":{"min":10,"max":11}`, `"save_dc":{"min":12,"max":11}`)
	sev("bad: severity attack below 0", `"attack_bonus":{"min":3,"max":5}`, `"attack_bonus":{"min":-1,"max":5}`)
	sev("bad: severity attack range reversed", `"attack_bonus":{"min":3,"max":5}`, `"attack_bonus":{"min":6,"max":5}`)
	sev("ok: severity attack bonus from 0", `"attack_bonus":{"min":3,"max":5}`, `"attack_bonus":{"min":0,"max":5}`)
	sev("bad: severity DC not above the one before", `"save_dc":{"min":12,"max":15}`, `"save_dc":{"min":11,"max":15}`)
	sev("bad: severity attack not above the one before", `"attack_bonus":{"min":6,"max":8}`, `"attack_bonus":{"min":5,"max":8}`)
	sev("bad: order before name", `"key":"setback","name_pt":"Revés"`, `"key":"deadly","name_pt":""`)
	sev("bad: name before ranges", `"name_pt":"Revés","save_dc":{"min":10,"max":11}`, `"name_pt":"","save_dc":{"min":0,"max":11}`)
	sev("bad: ranges before above", `"save_dc":{"min":12,"max":15}`, `"save_dc":{"min":0,"max":15}`)
	sev("bad: damage rows with a gap", `"from":11`, `"from":12`)
	sev("bad: damage rows starting at 2", `"from":1,`, `"from":2,`)
	sev("bad: damage row ending before it starts", `"to":10`, `"to":0`)
	sev("bad: last damage row ending before it starts", `"to":20`, `"to":10`)
	sev("bad: damage row past the last level", `"to":20`, `"to":21`)
	sev("bad: damage rows that stop at 19", `"to":20`, `"to":19`)
	sev("bad: setback damage not dice", `"setback":"1d10"`, `"setback":"lots"`)
	sev("bad: dangerous damage of no dice", `"dangerous":"2d10"`, `"dangerous":"0d10"`)
	sev("bad: deadly damage with a bonus", `"deadly":"4d10"`, `"deadly":"4d10+1"`)
	sev("bad: deadly damage with a modifier", `"deadly":"10d10"`, `"deadly":"10d10 + MOD"`)
	sev("bad: gap before dice", `"from":11,"to":20,"setback":"2d10"`, `"from":12,"to":20,"setback":"x"`)
	file("bad: no damage rows", strings.Replace(trapFile(trapMembers(nil)), `"damage_by_level":[{"from":1,"to":10,"setback":"1d10","dangerous":"2d10","deadly":"4d10"},{"from":11,"to":20,"setback":"2d10","dangerous":"4d10","deadly":"10d10"}]`, `"damage_by_level":[]`, 1))
	file("bad: severity before traps", strings.Replace(trapFile(trapMembers(map[string]string{"kind": `"x"`})), `"key":"setback"`, `"key":"deadly"`, 1))
	return cases
}

func TestLoadTrapsCharacterization(t *testing.T) {
	t.Parallel()
	got := map[string]string{}
	for name, tc := range trapCases() {
		c := trapsContent()
		if tc.names != nil {
			c.namesPT = map[string]string{}
			for _, n := range tc.names {
				c.namesPT[n] = "N"
			}
		} else if strings.Contains(tc.doc, `"key":"b"`) {
			c.namesPT["trap:b"] = "B"
		}
		err := c.loadTraps(fstest.MapFS{"effects/traps.json": {Data: []byte(tc.doc)}})
		text := errText(err)
		if err == nil {
			state, merr := json.Marshal(struct {
				P []TrapPreset
				S []TrapSeverity
				D []TrapDamageRow
			}{c.traps.presets, c.traps.severities, c.traps.damageRows})
			if merr != nil {
				t.Fatal(merr)
			}
			text += " " + string(state)
		}
		if strings.HasPrefix(name, "ok:") != (err == nil) {
			t.Errorf("%s: %v", name, err)
		}
		got[name] = text
	}
	goldenText(t, "traps-refusals", got)
}
