package rules

import (
	"encoding/json"
	"fmt"
	"strings"
	"testing"
)

// effectCases is the corpus of compileEffect: one effect per refusal and, for
// each type, the accepted shapes. The effects are JSON, as in effects/*.json.
func effectCases() map[string]string {
	c := map[string]string{}
	add := func(name, doc string) { c[name] = doc }

	add("bad: unknown type", `{"type":"bogus"}`)
	add("bad: empty type", `{"type":""}`)

	// The condition and the tags, judged before the type.
	add("ok: when", `{"type":"note","when":"armor() == \"none\""}`)
	add("bad: when does not parse", `{"type":"note","when":"armor() =="}`)
	add("bad: when is not a Bool", `{"type":"note","when":"1 + 1"}`)
	add("bad: empty tag", `{"type":"note","tags":["a",""]}`)
	add("bad: tag with a leading space", `{"type":"note","tags":[" a"]}`)
	add("bad: tag with a trailing space", `{"type":"note","tags":["a "]}`)
	add("ok: tags", `{"type":"note","tags":["against:magic","about:magic-items"]}`)
	add("bad: when before tags", `{"type":"note","when":"1 + 1","tags":[""]}`)
	add("bad: tags before stray fields", `{"type":"note","tags":[""],"sides":6}`)
	add("bad: unknown type before when", `{"type":"bogus","when":"1 + 1"}`)

	// Fields that belong to one type only, in the order of the checks.
	add("bad: max_cr on a note", `{"type":"note","max_cr":"1"}`)
	add("bad: no_fly on a note", `{"type":"note","no_fly":true}`)
	add("bad: no_swim on a note", `{"type":"note","no_swim":true}`)
	add("bad: sides on a note", `{"type":"note","sides":6}`)
	add("bad: replaces on a note", `{"type":"note","replaces":"feature:x"}`)
	add("bad: ignore_cover on a note", `{"type":"note","ignore_cover":true}`)
	add("bad: range_multiplier on a note", `{"type":"note","range_multiplier":2}`)
	add("bad: no_long_range_disadvantage on a note", `{"type":"note","no_long_range_disadvantage":true}`)
	add("bad: max_cr before sides", `{"type":"note","max_cr":"1","sides":6}`)
	add("bad: sides before replaces", `{"type":"note","sides":6,"replaces":"x"}`)
	add("bad: replaces before ranged fields", `{"type":"note","replaces":"x","ignore_cover":true}`)

	// ranged_attack.
	add("ok: ranged_attack spell multiplier", `{"type":"ranged_attack","applies":"spell","range_multiplier":2}`)
	add("ok: ranged_attack weapon cover", `{"type":"ranged_attack","applies":"weapon","ignore_cover":true,"no_long_range_disadvantage":true}`)
	add("bad: ranged_attack without applies", `{"type":"ranged_attack","ignore_cover":true}`)
	add("bad: ranged_attack multiplier on weapons", `{"type":"ranged_attack","applies":"weapon","range_multiplier":2}`)
	add("bad: ranged_attack multiplier of 4", `{"type":"ranged_attack","applies":"spell","range_multiplier":4}`)
	add("bad: ranged_attack long range on spells", `{"type":"ranged_attack","applies":"spell","no_long_range_disadvantage":true}`)
	add("bad: ranged_attack that changes nothing", `{"type":"ranged_attack","applies":"spell"}`)
	add("bad: ranged_attack with tags", `{"type":"ranged_attack","applies":"spell","ignore_cover":true,"tags":["x"]}`)

	// replaces.
	add("ok: replaces", `{"type":"replaces","replaces":"feature:rage"}`)
	add("bad: replaces with a field", `{"type":"replaces","replaces":"feature:rage","text_pt":"x"}`)
	add("bad: replaces unknown feature", `{"type":"replaces","replaces":"feature:nope"}`)
	add("bad: replaces with a condition", `{"type":"replaces","replaces":"feature:rage","when":"true"}`)
	add("bad: replaces with tags", `{"type":"replaces","replaces":"feature:rage","tags":["x"]}`)
	add("bad: wild_shape with a condition", `{"type":"wild_shape","max_cr":"1","when":"true"}`)
	add("bad: song_of_rest with tags", `{"type":"song_of_rest","sides":6,"tags":["x"]}`)
	add("bad: beast_spells with a condition", `{"type":"beast_spells","when":"true"}`)
	add("bad: replaces nothing", `{"type":"replaces"}`)
	add("bad: replaces itself", `{"type":"replaces","replaces":"feature:x"}`)

	// modifier.
	add("ok: modifier", `{"type":"modifier","target":"ac.base","mode":"set","value":"10 + mod(\"dex\")"}`)
	add("bad: modifier unknown target", `{"type":"modifier","target":"luck","mode":"add","value":"1"}`)
	add("bad: modifier without target", `{"type":"modifier","mode":"add","value":"1"}`)
	add("bad: modifier unknown mode", `{"type":"modifier","target":"ac","mode":"mul","value":"1"}`)
	add("bad: modifier without value", `{"type":"modifier","target":"ac","mode":"add"}`)
	add("bad: modifier value does not parse", `{"type":"modifier","target":"ac","mode":"add","value":"1 +"}`)
	add("bad: modifier value is not an Int", `{"type":"modifier","target":"ac","mode":"add","value":"true"}`)
	for _, t := range []string{"hp.max", "speed.walk", "initiative", "attack.weapon.melee", "damage.weapon.ranged", "skill.stealth", "save.dex", "save.all", "damage.spell.fire-bolt"} {
		add("ok: modifier target "+t, `{"type":"modifier","target":"`+t+`","mode":"add","value":"1"}`)
	}
	for _, t := range []string{"skill.nope", "save.luck", "score.luck", "damage.spell.nope", "damage.weapon.thrown", "ac.", "skill.", "save."} {
		add("bad: modifier target "+t, `{"type":"modifier","target":"`+t+`","mode":"add","value":"1"}`)
	}
	add("ok: score modifier with a cap", `{"type":"modifier","target":"score.str","mode":"add","value":"4","cap":24}`)
	add("ok: score modifier with the highest cap", `{"type":"modifier","target":"score.str","mode":"add","value":"4","cap":30}`)
	add("bad: score modifier without a cap", `{"type":"modifier","target":"score.str","mode":"add","value":"4"}`)
	add("bad: cap on another target", `{"type":"modifier","target":"ac","mode":"add","value":"1","cap":24}`)
	add("bad: score modifier that is not an add", `{"type":"modifier","target":"score.str","mode":"set","value":"4","cap":24}`)
	add("bad: score modifier with a cap of 20", `{"type":"modifier","target":"score.str","mode":"add","value":"4","cap":20}`)
	add("bad: score modifier with a cap of 31", `{"type":"modifier","target":"score.str","mode":"add","value":"4","cap":31}`)
	add("bad: modifier target before mode", `{"type":"modifier","target":"x","mode":"x"}`)
	add("bad: modifier mode before value", `{"type":"modifier","target":"ac","mode":"x"}`)
	add("bad: modifier value before cap", `{"type":"modifier","target":"ac","mode":"add","cap":4}`)
	add("bad: modifier cap before formula", `{"type":"modifier","target":"ac","mode":"add","value":"1 +","cap":4}`)

	// proficiency.
	for _, p := range []string{"skill:*", "initiative", "skill:stealth", "save.wis", "proficiency:heavy-armor"} {
		add("ok: proficiency "+p, `{"type":"proficiency","proficiency":"`+p+`"}`)
	}
	for _, p := range []string{"", "bogus", "skill:nope", "save.luck", "proficiency:nope", "initiative2"} {
		add("bad: proficiency "+p, `{"type":"proficiency","proficiency":"`+p+`"}`)
	}
	for _, l := range []string{"half", "full", "expertise"} {
		add("ok: proficiency level "+l, `{"type":"proficiency","proficiency":"skill:stealth","level":"`+l+`"}`)
	}
	add("bad: proficiency level", `{"type":"proficiency","proficiency":"skill:stealth","level":"double"}`)

	// roll_mode.
	add("ok: roll_mode", `{"type":"roll_mode","roll":"advantage","targets":["attack","death_save","initiative","check.all","save.all","check.str","skill.stealth","save.int"]}`)
	add("bad: roll_mode unknown roll", `{"type":"roll_mode","roll":"luck","targets":["attack"]}`)
	add("bad: roll_mode without targets", `{"type":"roll_mode","roll":"advantage"}`)
	for _, t := range []string{"check.luck", "damage.weapon.melee", "ac", "bogus", "score.str", "skill.nope", "save.luck"} {
		add("bad: roll_mode target "+t, `{"type":"roll_mode","roll":"disadvantage","targets":["attack","`+t+`"]}`)
	}

	// sense.
	add("ok: sense", `{"type":"sense","sense":"darkvision","range_ft":60}`)
	add("bad: sense unknown", `{"type":"sense","sense":"echo","range_ft":60}`)
	add("bad: sense without range", `{"type":"sense","sense":"darkvision"}`)
	add("bad: sense with a negative range", `{"type":"sense","sense":"darkvision","range_ft":-5}`)

	// spellcasting.
	add("ok: spellcasting that prepares", `{"type":"spellcasting","ability":"wis","progression":"full","prepares":true,"prepared_max":"mod(\"wis\") + level()"}`)
	add("ok: spellcasting that knows", `{"type":"spellcasting","ability":"cha","progression":"pact"}`)
	add("ok: spellcasting with a spellbook", `{"type":"spellcasting","ability":"int","progression":"third","prepares":true,"prepared_max":"1","spellbook":true,"ritual":true}`)
	add("bad: spellcasting unknown ability", `{"type":"spellcasting","ability":"luck","progression":"full"}`)
	add("bad: spellcasting unknown progression", `{"type":"spellcasting","ability":"wis","progression":"quarter"}`)
	add("bad: spellcasting prepares without a max", `{"type":"spellcasting","ability":"wis","progression":"full","prepares":true}`)
	add("bad: spellcasting max without prepares", `{"type":"spellcasting","ability":"wis","progression":"full","prepared_max":"1"}`)
	add("bad: spellcasting spellbook without prepares", `{"type":"spellcasting","ability":"wis","progression":"full","spellbook":true}`)
	add("bad: spellcasting max does not parse", `{"type":"spellcasting","ability":"wis","progression":"full","prepares":true,"prepared_max":"1 +"}`)

	// resource.
	add("ok: resource", `{"type":"resource","resource":"rage","max":"level()","recharge":"long_rest"}`)
	add("ok: resource that recharges differently", `{"type":"resource","resource":"inspiration","max":"1","recharge":"long_rest","recharge_if":"level() >= 5","recharge_then":"short_rest"}`)
	add("bad: resource without a name", `{"type":"resource","max":"1","recharge":"none"}`)
	add("bad: resource without a max", `{"type":"resource","resource":"x","recharge":"none"}`)
	add("bad: resource with an unknown recharge", `{"type":"resource","resource":"x","max":"1","recharge":"hourly"}`)
	add("bad: resource max does not parse", `{"type":"resource","resource":"x","max":"1 +","recharge":"none"}`)
	add("bad: resource recharge_if alone", `{"type":"resource","resource":"x","max":"1","recharge":"none","recharge_if":"true"}`)
	add("bad: resource recharge_then alone", `{"type":"resource","resource":"x","max":"1","recharge":"none","recharge_then":"dawn"}`)
	add("bad: resource unknown recharge_then", `{"type":"resource","resource":"x","max":"1","recharge":"none","recharge_if":"true","recharge_then":"hourly"}`)
	add("bad: resource recharge_if does not parse", `{"type":"resource","resource":"x","max":"1","recharge":"none","recharge_if":"1 +","recharge_then":"dawn"}`)
	add("bad: resource recharge_if is not a Bool", `{"type":"resource","resource":"x","max":"1","recharge":"none","recharge_if":"1","recharge_then":"dawn"}`)
	add("bad: resource fields before max", `{"type":"resource","max":"1 +","recharge":"x"}`)
	add("bad: resource max before pair", `{"type":"resource","resource":"x","max":"1 +","recharge":"none","recharge_if":"true"}`)

	// choice.
	add("ok: choice", `{"type":"choice","choice":"skill","count":2,"from":["skill:stealth","skill:arcana"]}`)
	add("ok: choice of zero", `{"type":"choice","choice":"feat"}`)
	add("bad: choice unknown", `{"type":"choice","choice":"shade"}`)
	add("bad: choice with a negative count", `{"type":"choice","choice":"skill","count":-1}`)
	add("bad: choice from an unknown key", `{"type":"choice","choice":"skill","count":1,"from":["skill:stealth","skill:nope"]}`)

	// ability_increase.
	add("ok: ability_increase", `{"type":"ability_increase","count":1,"from":["str","dex"],"value":"1"}`)
	add("ok: ability_increase by 2", `{"type":"ability_increase","count":2,"from":["str","dex"],"value":"2"}`)
	add("bad: ability_increase without from", `{"type":"ability_increase","count":1,"value":"1"}`)
	add("bad: ability_increase count 0", `{"type":"ability_increase","from":["str"],"value":"1"}`)
	add("bad: ability_increase count above from", `{"type":"ability_increase","count":3,"from":["str","dex"],"value":"1"}`)
	add("bad: ability_increase value 3", `{"type":"ability_increase","count":1,"from":["str"],"value":"3"}`)
	add("bad: ability_increase unknown ability", `{"type":"ability_increase","count":1,"from":["luck"],"value":"1"}`)
	add("bad: ability_increase repeated ability", `{"type":"ability_increase","count":1,"from":["str","str"],"value":"1"}`)

	// grant_action.
	add("ok: grant_action", `{"type":"grant_action","economy":"bonus_action"}`)
	add("bad: grant_action unknown economy", `{"type":"grant_action","economy":"never"}`)

	// superiority_die.
	add("ok: superiority_die damage", `{"type":"superiority_die","resource":"superiority","applies":"damage","value":"8"}`)
	add("ok: superiority_die grapple free", `{"type":"superiority_die","resource":"superiority","applies":"grapple","value":"8"}`)
	add("ok: superiority_die grapple bonus action", `{"type":"superiority_die","resource":"superiority","applies":"grapple","value":"8","economy":"bonus_action"}`)
	add("ok: superiority_die reduce", `{"type":"superiority_die","resource":"superiority","applies":"reduce_melee_damage","value":"8","ability":"dex","economy":"reaction"}`)
	add("bad: superiority_die without a resource", `{"type":"superiority_die","applies":"damage","value":"8"}`)
	add("bad: superiority_die unknown applies", `{"type":"superiority_die","resource":"s","applies":"heal","value":"8"}`)
	add("bad: superiority_die without a value", `{"type":"superiority_die","resource":"s","applies":"damage"}`)
	add("bad: superiority_die value does not parse", `{"type":"superiority_die","resource":"s","applies":"damage","value":"8 +"}`)
	add("bad: superiority_die reduce without an ability", `{"type":"superiority_die","resource":"s","applies":"reduce_melee_damage","value":"8","economy":"reaction"}`)
	add("bad: superiority_die reduce without a reaction", `{"type":"superiority_die","resource":"s","applies":"reduce_melee_damage","value":"8","ability":"dex"}`)
	add("bad: superiority_die grapple costing an action", `{"type":"superiority_die","resource":"s","applies":"grapple","value":"8","economy":"action"}`)
	add("bad: superiority_die damage with an ability", `{"type":"superiority_die","resource":"s","applies":"damage","value":"8","ability":"dex"}`)
	add("bad: superiority_die attack with an economy", `{"type":"superiority_die","resource":"s","applies":"attack","value":"8","economy":"reaction"}`)

	// extra_attack.
	for _, n := range []int{0, 1, 2, 3, 4, 5} {
		add(fmt.Sprintf("extra_attack %d", n), fmt.Sprintf(`{"type":"extra_attack","count":%d}`, n))
	}

	// wild_shape.
	add("ok: wild_shape", `{"type":"wild_shape","max_cr":"1/4","no_fly":true,"no_swim":true}`)
	add("bad: wild_shape with a field", `{"type":"wild_shape","max_cr":"1","text_pt":"x"}`)
	add("bad: wild_shape without a CR", `{"type":"wild_shape"}`)
	add("bad: wild_shape with a CR that is not", `{"type":"wild_shape","max_cr":"lots"}`)

	// song_of_rest.
	for _, n := range []int{0, 6, 7, 8, 10, 12} {
		add(fmt.Sprintf("song_of_rest %d", n), fmt.Sprintf(`{"type":"song_of_rest","sides":%d}`, n))
	}
	add("bad: song_of_rest with a field", `{"type":"song_of_rest","sides":6,"text_pt":"x"}`)

	// beast_spells, handler, note.
	add("ok: beast_spells", `{"type":"beast_spells"}`)
	add("bad: beast_spells with a field", `{"type":"beast_spells","text_pt":"x"}`)
	add("ok: handler", `{"type":"handler","handler":"monk.martial_arts"}`)
	add("bad: handler unknown", `{"type":"handler","handler":"monk.nothing"}`)
	add("ok: note", `{"type":"note","text_pt":"x"}`)
	add("ok: note with a value", `{"type":"note","value":"prof()","text_pt":"CD {n}"}`)
	add("bad: note with a value and no text", `{"type":"note","value":"prof()"}`)
	add("bad: note value does not parse", `{"type":"note","value":"1 +","text_pt":"x"}`)

	// spells granted, any type.
	add("ok: spells granted", `{"type":"note","spells":["spell:fire-bolt"]}`)
	add("bad: spell granted unknown", `{"type":"note","spells":["spell:fire-bolt","spell:nope"]}`)
	add("bad: type checks before spells", `{"type":"sense","sense":"echo","spells":["spell:nope"]}`)
	return c
}

func TestCompileEffectCharacterization(t *testing.T) {
	t.Parallel()
	srd := loadForTest(t)
	srd.c.features["feature:x"] = srd.c.features[firstFeatureKey(srd.c)]
	got := map[string]string{}
	for name, doc := range effectCases() {
		var e Effect
		if err := json.Unmarshal([]byte(doc), &e); err != nil {
			t.Fatalf("%s: %v", name, err)
		}
		err := srd.c.compileEffect("feature:x", &e)
		text := errText(err)
		if err == nil {
			text += fmt.Sprintf(" compiled value=%t when=%t prepared=%t max=%t recharge_if=%t",
				e.value != nil, e.when != nil, e.preparedMax != nil, e.max != nil, e.rechargeIf != nil)
		}
		if strings.HasPrefix(name, "ok:") != (err == nil) && (strings.HasPrefix(name, "ok:") || strings.HasPrefix(name, "bad:")) {
			t.Errorf("%s: %v", name, err)
		}
		got[name] = text
	}
	goldenText(t, "effects-refusals", got)
}

func firstFeatureKey(c *content) string {
	for _, k := range sortedKeys(c.features) {
		return k
	}
	return ""
}
