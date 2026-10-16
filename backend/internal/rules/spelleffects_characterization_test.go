package rules

import (
	"fmt"
	"strings"
	"testing"
	"testing/fstest"

	"github.com/PuraFome/meuRPG/backend/internal/rules/srd51"
)

// spellEffectsCases is the corpus of loadSpellEffects: one corrupted entry per
// refusal, and, for each chain of "takes nothing else" checks, one entry per
// forbidden field, so no term of the chain can be dropped unseen.
func spellEffectsCases() map[string]string {
	entry := func(body string) string { return `{"spells":{"spell:sleep":{` + body + `}}}` }
	cases := map[string]string{}
	// forbidden lists the fields of an entry, as JSON members, with a value
	// that is not the zero one.
	forbidden := map[string]string{
		"dice": `"dice":"1d4"`, "dice_per_level": `"dice_per_level":"1d4"`, "condition": `"condition":"condition:blinded"`,
		"threshold": `"threshold":5`, "dies": `"dies":true`, "amount": `"amount":3`, "amount_per_level": `"amount_per_level":2`,
		"ends": `"ends":["condition:blinded"]`, "hit_points": `"hit_points":4`, "window_rounds": `"window_rounds":6`,
		"source": `"source":"SRD"`, "creatures": `"creatures":["monster:wolf"]`, "count": `"count":2`, "attack": `"attack":"none"`,
		"type": `"type":"beast"`, "options": `"options":[{"count":1,"max_cr":"1"}]`,
	}
	// withOnly builds an entry of a kind plus one extra field.
	add := func(prefix, base string, fields ...string) {
		for _, f := range fields {
			cases[prefix+" with "+f] = entry(base + "," + forbidden[f])
		}
	}
	cases["ok: nothing"] = `{"spells":{}}`
	cases["bad: not json"] = `{"spells":`
	cases["bad: unknown field"] = entry(`"kind":"zero_hp_target","power":9`)
	cases["bad: spell not in content"] = `{"spells":{"spell:toll-the-dead":{"kind":"zero_hp_target"}}}`
	cases["bad: first of two keys wins"] = `{"spells":{"spell:zzz":{"kind":"mind_blast"},"spell:sleep":{"kind":"mind_blast"}}}`
	cases["bad: unknown kind"] = entry(`"kind":"mind_blast"`)
	cases["bad: empty kind"] = entry(`"kind":""`)

	// revive
	const revive = `"kind":"revive","hit_points":1,"window_rounds":10,"source":"SRD"`
	cases["ok: revive"] = entry(revive)
	cases["bad: revive without hit points"] = entry(`"kind":"revive","window_rounds":10,"source":"SRD"`)
	cases["bad: revive without a window"] = entry(`"kind":"revive","hit_points":1,"source":"SRD"`)
	cases["bad: revive without a source"] = entry(`"kind":"revive","hit_points":1,"window_rounds":10`)
	add("bad: revive", revive, "dice", "dice_per_level", "condition", "threshold", "dies", "amount", "amount_per_level", "ends")
	cases["bad: hit points on another kind"] = entry(`"kind":"zero_hp_target","hit_points":4`)
	cases["bad: window on another kind"] = entry(`"kind":"zero_hp_target","window_rounds":6`)

	// summon
	const summon = `"kind":"summon","creatures":["monster:wolf"],"count":1,"attack":"none"`
	cases["ok: summon"] = entry(summon)
	add("bad: summon", summon, "dice", "dice_per_level", "condition", "threshold", "dies", "amount", "amount_per_level", "ends")
	cases["bad: summon refused by the summon loader"] = entry(`"kind":"summon","creatures":["monster:wolf"],"count":1,"attack":"sometimes"`)
	cases["bad: summon fields on a pool"] = entry(`"kind":"hp_pool","dice":"5d8","dice_per_level":"2d8","condition":"condition:blinded","count":2`)
	for _, f := range []string{"creatures", "count", "attack", "type", "options"} {
		add("bad: summon fields on a zero_hp_target:", `"kind":"zero_hp_target"`, f)
	}

	// ignores_cover
	const cover = `"kind":"ignores_cover"`
	cases["ok: ignores_cover"] = entry(cover)
	add("bad: ignores_cover", cover, "dice", "dice_per_level", "condition", "threshold", "dies", "amount", "amount_per_level", "ends")

	// hp_pool
	const pool = `"kind":"hp_pool","dice":"5d8","dice_per_level":"2d8","condition":"condition:blinded"`
	cases["ok: hp_pool"] = entry(pool)
	cases["bad: pool without dice"] = entry(`"kind":"hp_pool","dice_per_level":"2d8","condition":"condition:blinded"`)
	cases["bad: pool without dice per level"] = entry(`"kind":"hp_pool","dice":"5d8","condition":"condition:blinded"`)
	cases["bad: pool of other dice"] = entry(`"kind":"hp_pool","dice":"5d8","dice_per_level":"2d10","condition":"condition:blinded"`)
	cases["bad: pool die that does not exist"] = entry(`"kind":"hp_pool","dice":"5d7","dice_per_level":"2d7","condition":"condition:blinded"`)
	cases["bad: pool with a modifier"] = entry(`"kind":"hp_pool","dice":"5d8+2","dice_per_level":"2d8","condition":"condition:blinded"`)
	cases["bad: pool without a condition"] = entry(`"kind":"hp_pool","dice":"5d8","dice_per_level":"2d8"`)
	add("bad: pool", pool, "threshold", "dies", "amount", "amount_per_level", "ends")
	cases["bad: pool with a condition that is a spell"] = entry(`"kind":"hp_pool","dice":"5d8","dice_per_level":"2d8","condition":"spell:x"`)
	cases["bad: pool with a condition that is unknown"] = entry(`"kind":"hp_pool","dice":"5d8","dice_per_level":"2d8","condition":"condition:nope"`)

	// hp_threshold
	cases["ok: threshold with a condition"] = entry(`"kind":"hp_threshold","threshold":100,"condition":"condition:blinded"`)
	cases["ok: threshold that kills"] = entry(`"kind":"hp_threshold","threshold":100,"dies":true`)
	cases["bad: threshold of 0"] = entry(`"kind":"hp_threshold","dies":true`)
	cases["bad: threshold with both"] = entry(`"kind":"hp_threshold","threshold":100,"dies":true,"condition":"condition:blinded"`)
	cases["bad: threshold with neither"] = entry(`"kind":"hp_threshold","threshold":100`)
	for _, f := range []string{"dice", "dice_per_level", "amount", "amount_per_level", "ends"} {
		cases["bad: threshold with "+f] = entry(`"kind":"hp_threshold","threshold":100,"dies":true,` + forbidden[f])
	}
	cases["bad: threshold with a condition that is unknown"] = entry(`"kind":"hp_threshold","threshold":100,"condition":"condition:nope"`)

	// zero_hp_target
	const zero = `"kind":"zero_hp_target"`
	cases["ok: zero_hp_target"] = entry(zero)
	add("bad: zero_hp_target", zero, "dice", "dice_per_level", "condition", "threshold", "dies", "amount", "amount_per_level", "ends")

	// temp_hp
	const temp = `"kind":"temp_hp","dice":"1d4","amount":4,"amount_per_level":5`
	cases["ok: temp_hp"] = entry(temp)
	cases["ok: temp_hp without a base amount"] = entry(`"kind":"temp_hp","dice":"1d4","amount_per_level":5`)
	cases["bad: temp_hp without dice"] = entry(`"kind":"temp_hp","amount":4,"amount_per_level":5`)
	cases["bad: temp_hp with a negative amount"] = entry(`"kind":"temp_hp","dice":"1d4","amount":-1,"amount_per_level":5`)
	cases["bad: temp_hp that never grows"] = entry(`"kind":"temp_hp","dice":"1d4","amount":4`)
	add("bad: temp_hp", temp, "dice_per_level", "condition", "threshold", "dies", "ends")

	// max_hp
	const maxhp = `"kind":"max_hp","amount":5,"amount_per_level":5`
	cases["ok: max_hp"] = entry(maxhp)
	cases["ok: max_hp that never grows"] = entry(`"kind":"max_hp","amount":5`)
	cases["bad: max_hp of nothing"] = entry(`"kind":"max_hp","amount_per_level":5`)
	cases["bad: max_hp that shrinks"] = entry(`"kind":"max_hp","amount":5,"amount_per_level":-1`)
	add("bad: max_hp", maxhp, "dice", "dice_per_level", "condition", "threshold", "dies", "ends")

	// flat_heal
	const heal = `"kind":"flat_heal","amount":70,"amount_per_level":10,"ends":["condition:blinded"]`
	cases["ok: flat_heal"] = entry(heal)
	cases["bad: flat_heal of nothing"] = entry(`"kind":"flat_heal","ends":["condition:blinded"]`)
	cases["bad: flat_heal that shrinks"] = entry(`"kind":"flat_heal","amount":70,"amount_per_level":-1,"ends":["condition:blinded"]`)
	cases["bad: flat_heal that ends nothing"] = entry(`"kind":"flat_heal","amount":70`)
	add("bad: flat_heal", heal, "dice", "dice_per_level", "condition", "threshold", "dies")
	cases["bad: flat_heal ending a condition that is a spell"] = entry(`"kind":"flat_heal","amount":70,"ends":["spell:x"]`)
	cases["bad: flat_heal ending a condition that is unknown"] = entry(`"kind":"flat_heal","amount":70,"ends":["condition:blinded","condition:nope"]`)

	// The shape of an entry is judged before its conditions, and the conditions in order.
	cases["bad: shape before conditions"] = entry(`"kind":"hp_pool","dice":"5d8","dice_per_level":"2d8","condition":"condition:nope","threshold":3`)
	cases["bad: first bad end wins"] = entry(`"kind":"flat_heal","amount":70,"ends":["condition:nope","spell:x"]`)

	// two good spells and the state they leave
	cases["ok: three kinds together"] = `{"spells":{"spell:sleep":{` + pool + `},"spell:aid":{` + maxhp + `},"spell:revivify":{` + revive + `}}}`
	return cases
}

func TestLoadSpellEffectsCharacterization(t *testing.T) {
	t.Parallel()
	got := map[string]string{}
	for name, body := range spellEffectsCases() {
		c := &content{
			spells: map[string]*srd51.Spell{
				"spell:sleep":    {Key: "spell:sleep", Level: 1},
				"spell:aid":      {Key: "spell:aid", Level: 2},
				"spell:revivify": {Key: "spell:revivify", Level: 3},
				"spell:zzz":      {Key: "spell:zzz", Level: 1},
			},
			named:    map[string]*srd51.Named{"condition:blinded": {}, "condition:unconscious": {}, "spell:x": {}},
			monsters: map[string]*srd51.Monster{"monster:wolf": {}},
			features: map[string]*srd51.Feature{},
		}
		err := c.loadSpellEffects(fstest.MapFS{"effects/spells.json": {Data: []byte(body)}})
		text := errText(err)
		if err == nil {
			text += fmt.Sprintf(" effects=%+v cover=%v revives=%+v summons=%d", c.spellEffects, c.coverIgnoring, c.revives, len(c.summons))
		}
		if strings.HasPrefix(name, "ok:") != (err == nil) {
			t.Errorf("%s: %v", name, err)
		}
		got[name] = text
	}
	goldenText(t, "spelleffects-refusals", got)
}
