package rules

import (
	"encoding/json"
	"strings"
	"testing"
	"testing/fstest"

	"github.com/PuraFome/meuRPG/backend/internal/rules/srd51"
)

// combatModifierCases are one modifier per accepted shape and per refusal of
// every kind, for checkModifier.
func combatModifierCases() map[string]EffectModifier {
	return map[string]EffectModifier{
		"bad: unknown kind":                        {Kind: "fly"},
		"bad: empty kind":                          {},
		"bad: roll_die die 1":                      {Kind: ModifierRollDie, Die: 1, Sign: 1, AppliesTo: []string{"attack"}},
		"bad: roll_die sign 0":                     {Kind: ModifierRollDie, Die: 4, AppliesTo: []string{"attack"}},
		"bad: roll_die sign 2":                     {Kind: ModifierRollDie, Die: 4, Sign: 2, AppliesTo: []string{"attack"}},
		"bad: roll_die no applies_to":              {Kind: ModifierRollDie, Die: 4, Sign: -1},
		"bad: roll_die applies to damage":          {Kind: ModifierRollDie, Die: 4, Sign: 1, AppliesTo: []string{"attack", "damage"}},
		"ok: roll_die every roll":                  {Kind: ModifierRollDie, Die: 2, Sign: -1, AppliesTo: []string{"attack", "save", "check"}},
		"bad: check_bonus no skill":                {Kind: ModifierCheckBonus, Value: 10},
		"bad: check_bonus no value":                {Kind: ModifierCheckBonus, Skill: "skill:stealth"},
		"ok: check_bonus":                          {Kind: ModifierCheckBonus, Skill: "skill:stealth", Value: -2},
		"bad: base_ac 0":                           {Kind: ModifierBaseAC},
		"bad: base_ac 61":                          {Kind: ModifierBaseAC, Value: 61},
		"ok: base_ac 1":                            {Kind: ModifierBaseAC, Value: 1},
		"ok: base_ac 60":                           {Kind: ModifierBaseAC, Value: 60},
		"bad: speed_add 0":                         {Kind: ModifierSpeedAdd},
		"ok: speed_add":                            {Kind: ModifierSpeedAdd, Value: 1},
		"bad: ac_bonus 0":                          {Kind: ModifierACBonus},
		"ok: ac_bonus":                             {Kind: ModifierACBonus, Value: 1},
		"bad: speed_multiplier -1":                 {Kind: ModifierSpeedMultiplier, Value: -1},
		"bad: speed_multiplier 401":                {Kind: ModifierSpeedMultiplier, Value: 401},
		"ok: speed_multiplier 0":                   {Kind: ModifierSpeedMultiplier},
		"ok: speed_multiplier 400":                 {Kind: ModifierSpeedMultiplier, Value: 400},
		"bad: save_advantage no abilities":         {Kind: ModifierSaveAdvantage, Choose: true},
		"bad: save_advantage unknown ability":      {Kind: ModifierSaveAdvantage, Abilities: []string{"dex", "luck"}},
		"ok: save_advantage":                       {Kind: ModifierSaveAdvantage, Abilities: []string{"str", "cha"}},
		"bad: check_advantage nothing":             {Kind: ModifierCheckAdvantage},
		"ok: check_advantage choose":               {Kind: ModifierCheckAdvantage, Choose: true},
		"bad: check_advantage unknown ability":     {Kind: ModifierCheckAdvantage, Choose: true, Abilities: []string{"luck"}},
		"ok: check_advantage abilities":            {Kind: ModifierCheckAdvantage, Abilities: []string{"wis"}},
		"bad: condition_immunity none":             {Kind: ModifierConditionImmunity},
		"ok: condition_immunity unchecked keys":    {Kind: ModifierConditionImmunity, Conditions: []string{"condition:anything"}},
		"bad: turn_temp_hp nothing":                {Kind: ModifierTurnTempHP},
		"ok: turn_temp_hp from caster":             {Kind: ModifierTurnTempHP, FromCaster: true},
		"ok: turn_temp_hp value":                   {Kind: ModifierTurnTempHP, Value: 1},
		"bad: extra_action nothing allowed":        {Kind: ModifierExtraAction, MaxWeaponAttacks: 1},
		"bad: extra_action no attacks":             {Kind: ModifierExtraAction, Allowed: []string{"dash"}},
		"bad: extra_action of no standard action":  {Kind: ModifierExtraAction, Allowed: []string{"attack", "fly"}, MaxWeaponAttacks: 1},
		"ok: extra_action":                         {Kind: ModifierExtraAction, Allowed: []string{"attack", "dash", "disengage", "hide", "use-an-object"}, MaxWeaponAttacks: 1},
		"ok: no_move ignores its fields":           {Kind: ModifierNoMove, Value: -5, Die: 1},
		"ok: no_action":                            {Kind: ModifierNoAction},
		"ok: no_reaction":                          {Kind: ModifierNoReaction},
		"ok: attack_disadvantage ignores fields":   {Kind: ModifierAttackDisadvantage, Once: true, Value: 3},
		"order: roll_die numbers before applies":   {Kind: ModifierRollDie, Die: 1, AppliesTo: []string{"damage"}},
		"order: save_advantage empty before names": {Kind: ModifierSaveAdvantage},
		"order: extra_action counts before names":  {Kind: ModifierExtraAction, Allowed: []string{"fly"}},
	}
}

// TestCheckModifierCharacterization freezes every refusal of checkModifier.
func TestCheckModifierCharacterization(t *testing.T) {
	t.Parallel()
	got := map[string]string{}
	for name, m := range combatModifierCases() {
		err := checkModifier(m)
		if strings.HasPrefix(name, "ok:") != (err == nil) {
			t.Errorf("%s: %v", name, err)
		}
		got[name] = errText(err)
	}
	goldenText(t, "combat-modifier-refusals", got)
}

// combatDefCases corrupt a good definition one field at a time, and some at
// once to freeze the order of checkEffectDef's checks.
func combatDefCases() map[string]func(d *EffectDef) {
	save := func(s EffectSave) *EffectSave { return &s }
	return map[string]func(d *EffectDef){
		"ok: a good definition":        func(_ *EffectDef) {},
		"ok: nothing but a kind":       func(d *EffectDef) { *d = EffectDef{Key: d.Key, Kind: "master"} },
		"ok: a rounds duration":        func(d *EffectDef) { d.Duration = &EffectDuration{Kind: EffectDurationRounds, Rounds: 1} },
		"ok: owner visibility":         func(d *EffectDef) { d.Visibility = EffectVisibilityOwner },
		"ok: on_end names a spell":     func(d *EffectDef) { d.OnEnd = "spell:bless" },
		"ok: a label of 30 characters": func(d *EffectDef) { d.PlayerLabelPT = strings.Repeat("é", maxPlayerLabel) },
		"bad: empty kind":              func(d *EffectDef) { d.Kind = "" },
		"bad: unknown kind":            func(d *EffectDef) { d.Kind = "item" },
		"bad: unknown duration":        func(d *EffectDef) { d.Duration = &EffectDuration{Kind: "forever"} },
		"bad: rounds without a count":  func(d *EffectDef) { d.Duration = &EffectDuration{Kind: EffectDurationRounds} },
		"bad: a count without rounds":  func(d *EffectDef) { d.Duration = &EffectDuration{Kind: EffectDurationLongRest, Rounds: 2} },
		"bad: applies":                 func(d *EffectDef) { d.Applies = "some" },
		"ok: applies to the pool":      func(d *EffectDef) { d.Applies = "pool" },
		"ok: a clock on the caster": func(d *EffectDef) {
			d.Duration = &EffectDuration{Kind: EffectDurationUntilEndOfTurnOf, Anchor: EffectAnchorCaster}
		},
		"bad: an anchor that is no one": func(d *EffectDef) {
			d.Duration = &EffectDuration{Kind: EffectDurationUntilEndOfTurnOf, Anchor: "target"}
		},
		"bad: an anchor on a long clock": func(d *EffectDef) {
			d.Duration = &EffectDuration{Kind: EffectDurationLongRest, Anchor: EffectAnchorCaster}
		},
		"bad: visibility":                 func(d *EffectDef) { d.Visibility = "everyone" },
		"bad: target type":                func(d *EffectDef) { d.TargetType = "beast" },
		"bad: condition":                  func(d *EffectDef) { d.Conditions = []string{"condition:prone", "condition:bored"} },
		"bad: second modifier":            func(d *EffectDef) { d.Modifiers = append(d.Modifiers, EffectModifier{Kind: "fly"}) },
		"bad: end save ability":           func(d *EffectDef) { d.EndSave = save(EffectSave{Ability: "luck"}) },
		"bad: start save ability":         func(d *EffectDef) { d.StartSave = save(EffectSave{}) },
		"bad: on_pass":                    func(d *EffectDef) { d.EndSave = save(EffectSave{Ability: "wis", OnPass: "fail"}) },
		"bad: on_fail":                    func(d *EffectDef) { d.StartSave = save(EffectSave{Ability: "dex", OnFail: "spell:bless"}) },
		"bad: trigger dice":               func(d *EffectDef) { d.Trigger.Dice = "lots" },
		"bad: trigger of no dice":         func(d *EffectDef) { d.Trigger.Dice = "0d4" },
		"bad: trigger dice with modifier": func(d *EffectDef) { d.Trigger.Dice = "1d4 + MOD" },
		"bad: trigger damage prefix":      func(d *EffectDef) { d.Trigger.DamageType = "fire" },
		"bad: trigger damage unknown":     func(d *EffectDef) { d.Trigger.DamageType = "damage-type:fun" },
		"bad: trigger damage nil":         func(d *EffectDef) { d.Trigger.DamageType = "damage-type:ghost" },
		"bad: trigger max":                func(d *EffectDef) { d.Trigger.MaxTriggers = 0 },
		"bad: on_end":                     func(d *EffectDef) { d.OnEnd = "effect:nope" },
		"bad: ends_effects_of":            func(d *EffectDef) { d.EndsEffectsOf = []string{"effect:x", "spell:nope"} },
		"bad: break_free":                 func(d *EffectDef) { d.BreakFree = "luck" },
		"bad: a label of 31 characters":   func(d *EffectDef) { d.PlayerLabelPT = strings.Repeat("a", maxPlayerLabel+1) },
		"order: kind before duration": func(d *EffectDef) {
			d.Kind = ""
			d.Duration = &EffectDuration{Kind: "forever"}
		},
		"order: duration before applies": func(d *EffectDef) {
			d.Duration = &EffectDuration{Kind: EffectDurationRounds}
			d.Applies = "some"
		},
		"order: applies before visibility": func(d *EffectDef) { d.Applies, d.Visibility = "some", "x" },
		"order: visibility before target":  func(d *EffectDef) { d.Visibility, d.TargetType = "x", "beast" },
		"order: target before conditions": func(d *EffectDef) {
			d.TargetType = "beast"
			d.Conditions = []string{"condition:bored"}
		},
		"order: conditions before modifiers": func(d *EffectDef) {
			d.Conditions = []string{"condition:bored"}
			d.Modifiers[0].Kind = "fly"
		},
		"order: modifiers before saves": func(d *EffectDef) {
			d.Modifiers[0].Kind = "fly"
			d.EndSave = save(EffectSave{})
		},
		"order: end save before start save": func(d *EffectDef) {
			d.EndSave = save(EffectSave{Ability: "wis", OnPass: "x"})
			d.StartSave = save(EffectSave{})
		},
		"order: save ability before on_pass": func(d *EffectDef) { d.EndSave = save(EffectSave{OnPass: "x", OnFail: "effect:nope"}) },
		"order: on_pass before on_fail":      func(d *EffectDef) { d.EndSave = save(EffectSave{Ability: "wis", OnPass: "x", OnFail: "effect:nope"}) },
		"order: saves before trigger": func(d *EffectDef) {
			d.StartSave = save(EffectSave{})
			d.Trigger.Dice = "x"
		},
		"order: trigger dice before damage": func(d *EffectDef) {
			d.Trigger.Dice = "x"
			d.Trigger.DamageType = "x"
			d.Trigger.MaxTriggers = 0
		},
		"order: trigger damage before max": func(d *EffectDef) {
			d.Trigger.DamageType = "x"
			d.Trigger.MaxTriggers = 0
		},
		"order: trigger before on_end": func(d *EffectDef) {
			d.Trigger.MaxTriggers = 0
			d.OnEnd = "effect:nope"
		},
		"order: on_end before ends_effects_of": func(d *EffectDef) {
			d.OnEnd = "effect:one"
			d.EndsEffectsOf = []string{"effect:two"}
		},
		"order: ends_effects_of before break_free": func(d *EffectDef) {
			d.EndsEffectsOf = []string{"effect:two"}
			d.BreakFree = "luck"
		},
		"order: break_free before label": func(d *EffectDef) {
			d.BreakFree = "luck"
			d.PlayerLabelPT = strings.Repeat("a", maxPlayerLabel+1)
		},
	}
}

func goodCombatDef() *EffectDef {
	return &EffectDef{
		Key: "effect:x", Kind: "feature", Duration: &EffectDuration{Kind: EffectDurationConcentration},
		Conditions: []string{"condition:prone"}, Modifiers: []EffectModifier{{Kind: ModifierNoMove}},
		EndSave:   &EffectSave{Ability: "wis", OnPass: "end"},
		StartSave: &EffectSave{Ability: "dex", OnFail: "effect:x"},
		Trigger:   &EffectTrigger{Dice: "2d4", DamageType: "damage-type:fire", MaxTriggers: 1},
		Applies:   "failed_save", TargetType: "humanoid", OnEnd: "effect:x", EndsEffectsOf: []string{"effect:x", "spell:bless"},
		BreakFree: "str", PlayerLabelPT: "Preso", Visibility: EffectVisibilityPublic,
	}
}

// TestCheckEffectDefCharacterization freezes every refusal of checkEffectDef
// and the order of its checks.
func TestCheckEffectDefCharacterization(t *testing.T) {
	t.Parallel()
	c := &content{named: map[string]*srd51.Named{"damage-type:fire": {}, "damage-type:ghost": nil}}
	all := &combatEffects{
		conditions: map[string]ConditionInfo{"condition:prone": {}},
		spells:     map[string]*EffectDef{"spell:bless": {}},
		effects:    map[string]*EffectDef{"effect:x": {}},
	}
	got := map[string]string{}
	for name, mutate := range combatDefCases() {
		d := goodCombatDef()
		mutate(d)
		err := c.checkEffectDef("f.json", d, all)
		if strings.HasPrefix(name, "ok:") != (err == nil) {
			t.Errorf("%s: %v", name, err)
		}
		got[name] = errText(err)
	}
	goldenText(t, "combat-effectdef-refusals", got)
}

// combatEffectsState is what a good file leaves, with the fields the file's
// JSON hides.
func combatEffectsState(ce *combatEffects) string {
	type def struct {
		*EffectDef
		K, N string
		C    bool
	}
	wrap := func(m map[string]*EffectDef) map[string]def {
		out := map[string]def{}
		for k, d := range m {
			out[k] = def{d, d.Key, d.NamePT, d.Concentration}
		}
		return out
	}
	conds := map[string][]string{}
	for k, i := range ce.conditions {
		conds[k] = append([]string{i.Key, i.Visibility}, i.ChangesPT...)
	}
	b, err := json.Marshal(struct {
		Conditions      map[string][]string
		Spells, Effects map[string]def
	}{conds, wrap(ce.spells), wrap(ce.effects)})
	if err != nil {
		return err.Error()
	}
	return string(b)
}

// combatEffectsCases are whole files for loadCombatEffects, each with at most
// one fault, as the loader walks maps in no fixed order.
func combatEffectsCases() map[string]fstest.MapFS {
	conds := allConditionsJSON("")
	x := `"effect:x":{"visibility":"public"}`
	return map[string]fstest.MapFS{
		"ok: a good file":                    fixtureFile(conds, `"spell:bless":{"applies":"all","modifiers":[{"kind":"roll_die","die":4,"sign":1,"applies_to":["attack"]}]}`, x),
		"ok: a master effect keeps its kind": fixtureFile(conds, ``, `"effect:x":{"kind":"master","on_end":"effect:x"}`),
		"ok: a rider until the end of the turn": fixtureFile(conds,
			`"spell:vicious-mockery":{"applies":"failed_save","duration":{"kind":"until_end_of_turn_of"},"modifiers":[{"kind":"attack_disadvantage","once":true}]}`, x),
		"ok: a rider until the start of the turn": fixtureFile(conds, `"spell:vicious-mockery":{"duration":{"kind":"until_start_of_turn_of"}}`, x),
		"bad: not json":                     {"effects/combat_effects.json": {Data: []byte(`{"source":`)}},
		"bad: no file":                      {},
		"bad: an unknown top field":         {"effects/combat_effects.json": {Data: []byte(`{"source":"x","rules":1}`)}},
		"bad: no source":                    {"effects/combat_effects.json": {Data: []byte(`{"source":"  ","conditions":{},"spells":{},"effects":{}}`)}},
		"bad: a condition that is not SRD":  fixtureFile(conds+`,"condition:bored":{"visibility":"public","changes_pt":["x"]}`, ``, x),
		"bad: a damage type as a condition": fixtureFile(allConditionsJSON("prone")+`,"damage-type:fire":{"visibility":"owner","changes_pt":["x"]}`, ``, x),
		"bad: a condition of no audience":   fixtureFile(allConditionsJSON("prone")+`,"condition:prone":{"changes_pt":["x"]}`, ``, x),
		"bad: a condition of no lines":      fixtureFile(allConditionsJSON("prone")+`,"condition:prone":{"visibility":"owner"}`, ``, x),
		"bad: a missing condition":          fixtureFile(allConditionsJSON("prone"), ``, x),
		"bad: a spell that is not SRD":      fixtureFile(conds, `"spell:made-up":{"applies":"all"}`, x),
		"bad: a spell's own duration":       fixtureFile(conds, `"spell:bless":{"duration":{"kind":"until_dismissed"}}`, x),
		"bad: a rider that counts rounds":   fixtureFile(conds, `"spell:vicious-mockery":{"duration":{"kind":"rounds","rounds":1}}`, x),
		"bad: an unknown field of a spell":  fixtureFile(conds, `"spell:bless":{"cones":true}`, x),
		"bad: an effect with no key prefix": fixtureFile(conds, ``, x+`,"x":{}`),
		"bad: an effect with no name":       fixtureFile(conds, ``, x+`,"effect:y":{}`),
		"bad: a name with no effect":        fixtureFile(conds, ``, ``),
		"bad: a spell that fails its check": fixtureFile(conds, `"spell:bless":{"target_type":"beast"}`, x),
		// Finding: a spell's kind is overwritten with "spell" before the check, so a
		// wrong kind in the file is silently ignored.
		"ok: a spell's kind is overwritten":   fixtureFile(conds, `"spell:bless":{"kind":"item"}`, x),
		"bad: an effect that fails its check": fixtureFile(conds, ``, `"effect:x":{"kind":"item"}`),
		"bad: a spell checked before effects": fixtureFile(conds, `"spell:bless":{"applies":"x"}`, `"effect:x":{"kind":"item"}`),
	}
}

// TestLoadCombatEffectsCharacterization freezes loadCombatEffects's refusals
// and what a good file leaves.
func TestLoadCombatEffectsCharacterization(t *testing.T) {
	t.Parallel()
	got := map[string]string{}
	for name, fsys := range combatEffectsCases() {
		c := combatEffectsFixture()
		err := c.loadCombatEffects(fsys)
		text := errText(err)
		if err == nil {
			text += " " + combatEffectsState(c.combatEffects)
		} else if c.combatEffects != nil {
			t.Errorf("%s: a refused file left its effects", name)
		}
		if strings.HasPrefix(name, "ok:") != (err == nil) {
			t.Errorf("%s: %v", name, err)
		}
		got[name] = text
	}
	goldenText(t, "combat-effects-refusals", got)
}
