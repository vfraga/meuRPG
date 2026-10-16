package rules

import (
	"strings"
	"testing"

	"github.com/PuraFome/meuRPG/backend/internal/rules/srd51"
)

// monsterChecks corrupts one stat block one invariant at a time, to freeze the
// text and the order of checkMonsters's refusals.
func monsterChecks() map[string]func(m *srd51.Monster) {
	multi := func(m *srd51.Monster, routine ...srd51.MonsterAttackCount) {
		m.Actions = append(m.Actions, srd51.MonsterAction{Name: "Multiattack", Multiattack: [][]srd51.MonsterAttackCount{routine}})
	}
	return map[string]func(m *srd51.Monster){
		"ok: a good stat block": func(_ *srd51.Monster) {},
		"ok: ability and magic counts need no action": func(m *srd51.Monster) {
			multi(m, srd51.MonsterAttackCount{Name: "Breath", Count: 1, Kind: "ability"}, srd51.MonsterAttackCount{Name: "Spell", Count: 2, Kind: "magic"}, srd51.MonsterAttackCount{Name: "Bite", Count: 2, Kind: "melee"})
		},
		"ok: armor class alternatives": func(m *srd51.Monster) {
			m.ArmorClassAlts = []srd51.MonsterACAlt{{Value: 15, Spell: "spell:armor"}, {Value: 11, Condition: "condition:poisoned"}, {Value: 9}}
		},
		"ok: no challenge issue at 1/8":   func(m *srd51.Monster) { m.ChallengeRating = "1/8" },
		"bad: challenge rating empty":     func(m *srd51.Monster) { m.ChallengeRating = "" },
		"bad: challenge rating 1/3":       func(m *srd51.Monster) { m.ChallengeRating = "1/3" },
		"bad: challenge rating 31":        func(m *srd51.Monster) { m.ChallengeRating = "31" },
		"bad: skill unknown":              func(m *srd51.Monster) { m.Skills["skill:nope"] = 1 },
		"bad: save unknown":               func(m *srd51.Monster) { m.Saves["luck"] = 1 },
		"bad: condition unknown":          func(m *srd51.Monster) { m.ConditionImmunities = []string{"condition:bored"} },
		"bad: condition is a damage type": func(m *srd51.Monster) { m.ConditionImmunities = []string{"damage-type:fire"} },
		"bad: condition is nil":           func(m *srd51.Monster) { m.ConditionImmunities = []string{"condition:ghost"} },
		"bad: vulnerability unknown": func(m *srd51.Monster) {
			m.Vulnerabilities = []srd51.MonsterDamageMod{{Types: []string{"damage-type:fun"}}}
		},
		"bad: resistance unknown": func(m *srd51.Monster) {
			m.Resistances = []srd51.MonsterDamageMod{{Types: []string{"damage-type:fire", "damage-type:fun"}}}
		},
		"bad: immunity unknown":        func(m *srd51.Monster) { m.Immunities[0].Types[0] = "damage-type:fun" },
		"bad: immunity is a condition": func(m *srd51.Monster) { m.Immunities[0].Types[0] = "condition:poisoned" },
		"bad: armor item unknown":      func(m *srd51.Monster) { m.ArmorClassItems = []string{"equipment:shield", "equipment:nope"} },
		"bad: armor alt spell unknown": func(m *srd51.Monster) { m.ArmorClassAlts = []srd51.MonsterACAlt{{Value: 15, Spell: "spell:nope"}} },
		"bad: armor alt condition": func(m *srd51.Monster) {
			m.ArmorClassAlts = []srd51.MonsterACAlt{{Value: 11, Condition: "condition:nope"}}
		},
		"bad: multiattack count 0": func(m *srd51.Monster) {
			multi(m, srd51.MonsterAttackCount{Name: "Bite", Count: 0, Kind: "melee"})
		},
		"bad: multiattack count negative": func(m *srd51.Monster) {
			multi(m, srd51.MonsterAttackCount{Name: "Bite", Count: 1, Kind: "melee"}, srd51.MonsterAttackCount{Name: "Bite", Count: -1, Kind: "melee"})
		},
		"bad: multiattack action missing": func(m *srd51.Monster) {
			multi(m, srd51.MonsterAttackCount{Name: "Tail", Count: 1, Kind: "melee"})
		},
		"bad: multiattack kind decides": func(m *srd51.Monster) {
			multi(m, srd51.MonsterAttackCount{Name: "Tail", Count: 1, Kind: "ranged"})
		},
		"bad: action damage type unknown": func(m *srd51.Monster) { m.Actions[0].Damage[0].DamageType = "damage-type:fun" },
		"bad: action damage type is a condition": func(m *srd51.Monster) {
			m.Actions[0].Damage[0].DamageType = "condition:poisoned"
		},
		"bad: action damage dice":  func(m *srd51.Monster) { m.Actions[0].Damage[0].Dice = "lots" },
		"bad: action save ability": func(m *srd51.Monster) { m.Actions[0].Save = &srd51.MonsterSave{Ability: "luck", DC: 10} },
		"ok: action save ability":  func(m *srd51.Monster) { m.Actions[0].Save = &srd51.MonsterSave{Ability: "dex", DC: 10} },
		"order: challenge before skill": func(m *srd51.Monster) {
			m.ChallengeRating = "x"
			m.Skills["skill:nope"] = 1
		},
		"order: skill before save condition": func(m *srd51.Monster) {
			m.Skills["skill:nope"] = 1
			m.Saves["luck"] = 1
			m.ConditionImmunities = []string{"condition:bored"}
		},
		"order: condition before damage types": func(m *srd51.Monster) {
			m.ConditionImmunities = []string{"condition:bored"}
			m.Immunities[0].Types[0] = "damage-type:fun"
		},
		"order: vulnerabilities before resistances": func(m *srd51.Monster) {
			m.Vulnerabilities = []srd51.MonsterDamageMod{{Types: []string{"damage-type:v"}}}
			m.Resistances = []srd51.MonsterDamageMod{{Types: []string{"damage-type:r"}}}
			m.Immunities[0].Types[0] = "damage-type:i"
		},
		"order: damage types before armor": func(m *srd51.Monster) {
			m.Immunities[0].Types[0] = "damage-type:fun"
			m.ArmorClassItems = []string{"equipment:nope"}
		},
		"order: armor before alternatives": func(m *srd51.Monster) {
			m.ArmorClassItems = []string{"equipment:nope"}
			m.ArmorClassAlts = []srd51.MonsterACAlt{{Spell: "spell:nope"}}
		},
		"order: alternative spell before condition": func(m *srd51.Monster) {
			m.ArmorClassAlts = []srd51.MonsterACAlt{{Spell: "spell:nope", Condition: "condition:nope"}}
		},
		"order: alternatives before actions": func(m *srd51.Monster) {
			m.ArmorClassAlts = []srd51.MonsterACAlt{{Spell: "spell:nope"}}
			m.Actions[0].Damage[0].Dice = "lots"
		},
		"order: multiattack before the damage of the same action": func(m *srd51.Monster) {
			m.Actions = append(m.Actions, srd51.MonsterAction{
				Name: "Multiattack", Multiattack: [][]srd51.MonsterAttackCount{{{Name: "Tail", Count: 1, Kind: "melee"}}},
				Damage: []srd51.MonsterDamage{{Dice: "lots", DamageType: "damage-type:fun"}},
			})
		},
		"order: damage type before dice": func(m *srd51.Monster) {
			m.Actions[0].Damage[0] = srd51.MonsterDamage{Dice: "lots", DamageType: "damage-type:fun"}
		},
		"order: dice before save": func(m *srd51.Monster) {
			m.Actions[0].Damage[0].Dice = "lots"
			m.Actions[0].Save = &srd51.MonsterSave{Ability: "luck"}
		},
		"order: first action before second": func(m *srd51.Monster) {
			m.Actions[0].Save = &srd51.MonsterSave{Ability: "luck"}
			m.Actions = append(m.Actions, srd51.MonsterAction{Name: "Claw", Damage: []srd51.MonsterDamage{{Dice: "lots", DamageType: "damage-type:fire"}}})
		},
	}
}

func monsterContent(m *srd51.Monster, others ...*srd51.Monster) *content {
	c := &content{
		monsters:  map[string]*srd51.Monster{m.Key: m},
		skills:    map[string]*srd51.Skill{"skill:stealth": {}},
		equipment: map[string]*srd51.Equipment{"equipment:shield": {}},
		spells:    map[string]*srd51.Spell{"spell:armor": {}},
		named: map[string]*srd51.Named{
			"damage-type:fire": {}, "condition:poisoned": {}, "condition:ghost": nil,
		},
	}
	for _, o := range others {
		c.monsters[o.Key] = o
	}
	return c
}

func goodMonster(key string) *srd51.Monster {
	return &srd51.Monster{
		Key: key, ChallengeRating: "1", Saves: map[string]int{"dex": 2}, Skills: map[string]int{"skill:stealth": 4},
		Immunities: []srd51.MonsterDamageMod{{Types: []string{"damage-type:fire"}}}, ConditionImmunities: []string{"condition:poisoned"},
		Actions: []srd51.MonsterAction{{Name: "Bite", Damage: []srd51.MonsterDamage{{Dice: "1d6", DamageType: "damage-type:fire"}}}},
	}
}

// TestCheckMonstersCharacterization freezes every refusal of checkMonsters:
// the text, the stat block it names and the order of the checks.
func TestCheckMonstersCharacterization(t *testing.T) {
	t.Parallel()
	got := map[string]string{}
	for name, mutate := range monsterChecks() {
		m := goodMonster("monster:x")
		mutate(m)
		err := monsterContent(m).checkMonsters()
		got[name] = errText(err)
		if strings.HasPrefix(name, "ok:") != (err == nil) {
			t.Errorf("%s: %v", name, err)
		}
	}
	// The stat blocks are checked in key order: the first bad one is named.
	z, a := goodMonster("monster:z"), goodMonster("monster:a")
	z.ChallengeRating, a.ChallengeRating = "z", "a"
	got["order: stat blocks by key"] = errText(monsterContent(z, a).checkMonsters())
	b := goodMonster("monster:b")
	b.ChallengeRating = "b"
	got["order: a good block before a bad one"] = errText(monsterContent(goodMonster("monster:a"), b).checkMonsters())
	goldenText(t, "monsters-refusals", got)
}
