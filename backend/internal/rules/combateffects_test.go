package rules

import (
	"strings"
	"testing"
	"testing/fstest"

	"github.com/PuraFome/meuRPG/backend/internal/rules/srd51"
)

func TestTheSpellsThatLastHaveTheirEffects(t *testing.T) {
	t.Parallel()
	c := loadForTest(t)
	bless, ok := c.CombatSpellEffect("spell:bless")
	if !ok || bless.Applies != "all" || len(bless.Modifiers) != 1 || bless.Modifiers[0].Sign != 1 || bless.Modifiers[0].Die != 4 || !bless.Concentration {
		t.Fatalf("Bless = %+v, %v", bless, ok)
	}
	bane, _ := c.CombatSpellEffect("spell:bane")
	if bane.Applies != "failed_save" || bane.Modifiers[0].Sign != -1 {
		t.Errorf("Bane subtracts the die from the targets that failed the save: %+v", bane)
	}
	for _, m := range append(bless.Modifiers, bane.Modifiers...) {
		if len(m.AppliesTo) != 2 || m.AppliesTo[0] != RollAppliesAttack || m.AppliesTo[1] != RollAppliesSave {
			t.Errorf("Bless and Bane touch attack rolls and saving throws, never ability checks: %+v", m)
		}
	}
	hold, _ := c.CombatSpellEffect("spell:hold-person")
	if hold.TargetType != "humanoid" || len(hold.Conditions) != 1 || hold.Conditions[0] != "condition:paralyzed" || hold.EndSave == nil || hold.EndSave.Ability != "wis" {
		t.Errorf("Hold Person = %+v", hold)
	}
	haste, _ := c.CombatSpellEffect("spell:haste")
	if haste.OnEnd != "effect:lethargy" || len(haste.Modifiers) != 4 {
		t.Errorf("Haste = %+v", haste)
	}
	fairy, _ := c.CombatSpellEffect("spell:faerie-fire")
	if fairy.Applies != "failed_save" || fairy.PlayerLabelPT != "Delineado" {
		t.Errorf("Faerie Fire outlines the targets that failed the save: %+v", fairy)
	}
	laugh, _ := c.CombatSpellEffect("spell:hideous-laughter")
	if len(laugh.Conditions) != 2 || laugh.EndSave == nil || !laugh.EndSave.OnDamage || laugh.EndSave.Ability != "wis" {
		t.Errorf("Hideous Laughter = %+v", laugh)
	}
	web, _ := c.CombatSpellEffect("spell:web")
	if web.StartSave == nil || web.StartSave.Ability != "dex" || web.StartSave.OnFail != "effect:web-restrained" {
		t.Errorf("Web = %+v", web)
	}
	held, ok := c.CombatEffect("effect:web-restrained")
	if !ok || held.BreakFree != "str" || held.PlayerLabelPT != "Preso numa teia" {
		t.Errorf("the restrained web = %+v", held)
	}
	fire, _ := c.CombatEffect("effect:web-fire")
	if fire.Trigger == nil || fire.Trigger.Dice != "2d4" || fire.Trigger.DamageType != "damage-type:fire" || fire.Trigger.MaxTriggers != 1 {
		t.Errorf("the web that burns deals 2d4 fire once: %+v", fire)
	}
	if _, ok := c.CombatSpellEffect("spell:fireball"); ok {
		t.Error("Fireball has no lasting effect")
	}
	// Vicious Mockery: an instantaneous spell whose rider lasts until the end of the target's next
	// turn, and gives the next attack roll disadvantage.
	mock, ok := c.CombatSpellEffect("spell:vicious-mockery")
	if !ok || mock.Applies != "failed_save" || mock.Duration == nil || mock.Duration.Kind != EffectDurationUntilEndOfTurnOf || mock.Concentration {
		t.Fatalf("Vicious Mockery = %+v, %v", mock, ok)
	}
	if len(mock.Modifiers) != 1 || mock.Modifiers[0].Kind != ModifierAttackDisadvantage || !mock.Modifiers[0].Once {
		t.Errorf("Vicious Mockery's modifiers = %+v, want one attack_disadvantage, once", mock.Modifiers)
	}
	for _, key := range []string{"spell:bless", "spell:bane", "spell:faerie-fire", "spell:hold-person"} {
		if d, _ := c.CombatSpellEffect(key); d.Duration != nil {
			t.Errorf("%s has a duration of its own: %+v; a spell's comes from the spell", key, d.Duration)
		}
	}
}

func TestSpellDurationsAreInRounds(t *testing.T) {
	t.Parallel()
	c := loadForTest(t)
	// SRD, "Duration": a round is 6 seconds, so a minute is 10 rounds and an hour 600.
	for key, want := range map[string]int{"spell:vicious-mockery": 0, "spell:bless": 10, "spell:bane": 10, "spell:haste": 10, "spell:hold-person": 10, "spell:faerie-fire": 10, "spell:hideous-laughter": 10, "spell:web": 600, "spell:longstrider": 600, "spell:pass-without-trace": 600, "spell:shield-of-faith": 100, "spell:fireball": 0} {
		if got := c.SpellEffectRounds(key); got != want {
			t.Errorf("SpellEffectRounds(%s) = %d, want %d", key, got, want)
		}
	}
}

func TestEveryConditionSaysWhoReadsIt(t *testing.T) {
	t.Parallel()
	c := loadForTest(t)
	public := []string{"blinded", "grappled", "incapacitated", "paralyzed", "petrified", "prone", "restrained", "stunned", "unconscious"}
	owner := []string{"charmed", "deafened", "exhaustion", "frightened", "invisible", "poisoned"}
	for _, k := range public {
		if i, ok := c.ConditionInfo("condition:" + k); !ok || i.Visibility != EffectVisibilityPublic || len(i.ChangesPT) == 0 {
			t.Errorf("%s = %+v, %v: the table sees it", k, i, ok)
		}
	}
	for _, k := range owner {
		if i, ok := c.ConditionInfo("condition:" + k); !ok || i.Visibility != EffectVisibilityOwner {
			t.Errorf("%s = %+v, %v: only its player and the master", k, i, ok)
		}
	}
	if len(public)+len(owner) != 15 {
		t.Errorf("the SRD has 15 conditions, listed %d", len(public)+len(owner))
	}
}

func TestEffectNamesAreInPortuguese(t *testing.T) {
	t.Parallel()
	c := loadForTest(t)
	for key, want := range map[string]string{"effect:lethargy": "Letargia", "effect:goaded": "Provocado", "effect:web-fire": "Teia em chamas", "effect:web-restrained": "Preso numa teia", "effect:entangle-restrained": "Preso pelas plantas", "effect:open-hand-no-reactions": "Mão Aberta: sem reações", "effect:stunning-strike": "Golpe Atordoante", "effect:turned": "Expulso", "effect:spirit-guardians-slowed": "Reduzido pelos espíritos"} {
		d, ok := c.CombatEffect(key)
		if !ok || d.NamePT != want || c.NamePT(key) != want {
			t.Errorf("%s = %+v, %v; want the name %q", key, d, ok, want)
		}
	}
	if keys := c.CombatEffectKeys(); len(keys) != 9 {
		t.Errorf("CombatEffectKeys = %v", keys)
	}
}

// combatEffectsFixture is a minimal content the loader can read a file against.
func combatEffectsFixture() *content {
	named := map[string]*srd51.Named{"damage-type:fire": {}}
	for _, k := range []string{"blinded", "charmed", "deafened", "exhaustion", "frightened", "grappled", "incapacitated", "invisible", "paralyzed", "petrified", "poisoned", "prone", "restrained", "stunned", "unconscious"} {
		named["condition:"+k] = &srd51.Named{}
	}
	return &content{
		named:   named,
		spells:  map[string]*srd51.Spell{"spell:bless": {Concentration: true, Duration: "Up to 1 minute"}, "spell:vicious-mockery": {Duration: "Instantaneous"}},
		namesPT: map[string]string{"spell:bless": "Bênção", "effect:x": "X"},
	}
}

func fixtureFile(conditions, spells, effects string) fstest.MapFS {
	return fstest.MapFS{"effects/combat_effects.json": {Data: []byte(`{"source":"SRD 5.1","conditions":{` + conditions + `},"spells":{` + spells + `},"effects":{` + effects + `}}`)}}
}

func allConditionsJSON(skip string) string {
	var parts []string
	for _, k := range []string{"blinded", "charmed", "deafened", "exhaustion", "frightened", "grappled", "incapacitated", "invisible", "paralyzed", "petrified", "poisoned", "prone", "restrained", "stunned", "unconscious"} {
		if k == skip {
			continue
		}
		parts = append(parts, `"condition:`+k+`":{"visibility":"public","changes_pt":["x"]}`)
	}
	return strings.Join(parts, ",")
}

func TestLoadCombatEffectsRefuses(t *testing.T) {
	t.Parallel()
	conds := allConditionsJSON("")
	if err := combatEffectsFixture().loadCombatEffects(fixtureFile(conds, `"spell:bless":{"applies":"all","modifiers":[{"kind":"roll_die","die":4,"sign":1,"applies_to":["attack"]}]}`, `"effect:x":{"visibility":"public"}`)); err != nil {
		t.Fatalf("a good file: %v", err)
	}
	// The rider of an instantaneous spell has its own clock on its target's turn.
	for _, kind := range []string{EffectDurationUntilEndOfTurnOf, EffectDurationUntilStartOfTurnOf} {
		rider := `"spell:vicious-mockery":{"applies":"failed_save","duration":{"kind":"` + kind + `"},"modifiers":[{"kind":"attack_disadvantage","once":true}]}`
		if err := combatEffectsFixture().loadCombatEffects(fixtureFile(conds, rider, `"effect:x":{"visibility":"public"}`)); err != nil {
			t.Errorf("the rider of an instantaneous spell, %s: %v", kind, err)
		}
	}
	cases := map[string]fstest.MapFS{
		"a missing condition":          fixtureFile(allConditionsJSON("prone"), ``, ``),
		"a condition that is not SRD":  fixtureFile(conds+`,"condition:bored":{"visibility":"public","changes_pt":["x"]}`, ``, ``),
		"a spell that is not SRD":      fixtureFile(conds, `"spell:made-up":{"applies":"all"}`, ``),
		"a spell's own duration":       fixtureFile(conds, `"spell:bless":{"duration":{"kind":"until_dismissed"}}`, ``),
		"a turn clock on Bless":        fixtureFile(conds, `"spell:bless":{"duration":{"kind":"until_end_of_turn_of"}}`, ``),
		"a rider that counts rounds":   fixtureFile(conds, `"spell:vicious-mockery":{"duration":{"kind":"rounds","rounds":1}}`, ``),
		"a rider until dismissed":      fixtureFile(conds, `"spell:vicious-mockery":{"duration":{"kind":"until_dismissed"}}`, ``),
		"an unknown modifier":          fixtureFile(conds, `"spell:bless":{"modifiers":[{"kind":"fly"}]}`, ``),
		"a die on an ability check":    fixtureFile(conds, `"spell:bless":{"modifiers":[{"kind":"roll_die","die":4,"sign":1,"applies_to":["check"]}]}`, ``),
		"a die with no sign":           fixtureFile(conds, `"spell:bless":{"modifiers":[{"kind":"roll_die","die":4,"applies_to":["attack"]}]}`, ``),
		"an unknown condition":         fixtureFile(conds, `"spell:bless":{"conditions":["condition:bored"]}`, ``),
		"a save with no ability":       fixtureFile(conds, `"spell:bless":{"end_save":{"on_pass":"end"}}`, ``),
		"an on_fail with no effect":    fixtureFile(conds, `"spell:bless":{"start_save":{"ability":"dex","on_fail":"effect:nope"}}`, ``),
		"a trigger that is no dice":    fixtureFile(conds, `"spell:bless":{"turn_trigger":{"dice":"lots","damage_type":"damage-type:fire","max_triggers":1}}`, ``),
		"a trigger of no damage type":  fixtureFile(conds, `"spell:bless":{"turn_trigger":{"dice":"2d4","damage_type":"fire","max_triggers":1}}`, ``),
		"a trigger that never fires":   fixtureFile(conds, `"spell:bless":{"turn_trigger":{"dice":"2d4","damage_type":"damage-type:fire","max_triggers":0}}`, ``),
		"an effect with no key prefix": fixtureFile(conds, ``, `"x":{}`),
		"an effect with no name":       fixtureFile(conds, ``, `"effect:y":{}`),
		"an on_end with no effect":     fixtureFile(conds, `"spell:bless":{"on_end":"effect:nope"}`, ``),
		"a label over 30 characters":   fixtureFile(conds, `"spell:bless":{"player_label_pt":"uma etiqueta muito mais longa que trinta"}`, ``),
		"a rounds duration with none":  fixtureFile(conds, ``, `"effect:x":{"duration":{"kind":"rounds"}}`),
		"an unknown audience":          fixtureFile(conds, `"spell:bless":{"visibility":"everyone"}`, ``),
		"a target type that is no one": fixtureFile(conds, `"spell:bless":{"target_type":"beast"}`, ``),
		"an extra action of no action": fixtureFile(conds, `"spell:bless":{"modifiers":[{"kind":"extra_action","allowed":["fly"],"max_weapon_attacks":1}]}`, ``),
		"an unknown field":             fixtureFile(conds, `"spell:bless":{"cones":true}`, ``),
	}
	for name, fsys := range cases {
		if err := combatEffectsFixture().loadCombatEffects(fsys); err == nil {
			t.Errorf("%s: the loader accepted it", name)
		}
	}
	// Without its source line the file is refused too.
	if err := combatEffectsFixture().loadCombatEffects(fstest.MapFS{"effects/combat_effects.json": {Data: []byte(`{"source":"","conditions":{},"spells":{},"effects":{}}`)}}); err == nil {
		t.Error("a file with no source line was accepted")
	}
	// A name in names_pt.json with no effect behind it.
	c := combatEffectsFixture()
	c.namesPT["effect:orphan"] = "Órfão"
	if err := c.loadCombatEffects(fixtureFile(conds, ``, `"effect:x":{}`)); err == nil {
		t.Error("a Portuguese name for an effect the file does not have was accepted")
	}
}
