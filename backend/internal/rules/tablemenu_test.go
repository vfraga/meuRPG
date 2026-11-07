package rules

import (
	"slices"
	"strconv"
	"strings"
	"testing"
)

func menuList(t *testing.T, m EffectMenu, name string) []MenuValue {
	t.Helper()
	for _, l := range m.Lists {
		if l.Name == name {
			return l.Values
		}
	}
	t.Fatalf("the menu has no list %q", name)
	return nil
}

func valueKeys(vs []MenuValue) []string {
	out := make([]string, len(vs))
	for i, v := range vs {
		out[i] = v.Key
	}
	return out
}

// classWithEffects is a table class that does not cast whose features carry the
// effects, 15 to a feature, so a test can offer the whole menu to With at once.
func classWithEffects(effects []Effect) TableClass {
	tc := classFromDefaults(DefaultTable{Kind: CastingNone, Rows: loadDefaultRows()}, "cardapio")
	for i := range tc.Levels {
		tc.Levels[i].Features = nil
	}
	// A feature offers one choice of options, so each of those has a feature of
	// its own.
	var groups [][]Effect
	var plain []Effect
	for _, e := range effects {
		if e.Type == "choice" && e.Choice == "feature" {
			groups = append(groups, []Effect{e})
			continue
		}
		plain = append(plain, e)
	}
	for len(plain) > 0 {
		n := min(15, len(plain))
		groups, plain = append(groups, plain[:n]), plain[n:]
	}
	for n, g := range groups {
		row := &tc.Levels[n%MaxLevel]
		row.Features = append(row.Features, TableFeature{
			Key: "feature:cardapio-" + strconv.Itoa(n) + tableSuffix, NamePT: "Cardápio " + strconv.Itoa(n), Effects: g,
		})
	}
	return tc
}

// loadDefaultRows are 20 plain rows (no casting).
func loadDefaultRows() []DefaultRow {
	rows := make([]DefaultRow, MaxLevel)
	for i := range rows {
		rows[i] = DefaultRow{Level: i + 1, ProfBonus: 2 + i/4}
	}
	return rows
}

// TestEffectMenuIsWhatTheValidatorAccepts: the menu's types are the table's
// (overlayEffectTypes), their fields are the ones the validator reads, the
// required ones are refused when missing, and every value of every closed list is
// accepted by With, so the editor offers nothing the server will refuse.
func TestEffectMenuIsWhatTheValidatorAccepts(t *testing.T) {
	t.Parallel()
	srd := loadForTest(t)
	m := srd.EffectMenu()

	var types []string
	for _, mt := range m.Types {
		types = append(types, mt.Type)
		if mt.NamePT == "" || mt.HintPT == "" {
			t.Errorf("type %s has no Portuguese name or hint", mt.Type)
		}
		var own []string
		for _, f := range mt.Fields {
			if f.Name != "when" && f.Name != "tags" && f.Name != "text_pt" {
				own = append(own, f.Name)
			}
			if f.List != "" {
				menuList(t, m, f.List) // the list exists
			}
		}
		want := slices.Clone(ownFields[mt.Type])
		slices.Sort(want)
		slices.Sort(own)
		if !slices.Equal(own, want) {
			t.Errorf("type %s: menu fields %v, validator reads %v", mt.Type, own, want)
		}
	}
	if !slices.Equal(types, overlayEffectTypes) {
		t.Errorf("menu types = %v, the table's menu is %v", types, overlayEffectTypes)
	}
	for _, l := range m.Lists {
		if len(l.Values) == 0 {
			t.Errorf("list %s is empty", l.Name)
		}
		seen := map[string]bool{}
		for _, v := range l.Values {
			if v.Key == "" || v.NamePT == "" {
				t.Errorf("list %s has a value without a key or a Portuguese name: %+v", l.Name, v)
			}
			// A value is unique by its key within its group (several races share the trait Darkvision).
			if id := v.Group + "/" + v.Key; seen[id] {
				t.Errorf("list %s repeats %q in %q", l.Name, v.Key, v.Group)
			} else {
				seen[id] = true
			}
		}
	}

	// The closed sets are the engine's.
	for name, want := range map[string][]string{
		ListModifierModes: modifierModes, ListProficiencyLevels: proficiencyLevels, ListRollModes: rollModes,
		ListSenses: senses, ListRecharges: recharges, ListEconomies: economies, ListChoiceKinds: overlayChoiceKinds,
		ListRangedAttacks: rangedAttackApplies, ListStateEnds: stateEnds, ListStateEconomies: stateEconomies,
		ListWeaponFilters: weaponFilters, ListWeaponKinds: weaponKinds, ListEvents: eventKinds,
	} {
		if got := valueKeys(menuList(t, m, name)); !slices.Equal(got, want) {
			t.Errorf("list %s = %v, want %v", name, got, want)
		}
	}

	// The ability list and the amounts of an ability_increase, hosted by a feat.
	var increases []Effect
	for _, a := range menuList(t, m, ListAbilities) {
		for _, amount := range menuList(t, m, ListIncreaseAmounts) {
			increases = append(increases, Effect{Type: "ability_increase", Count: 1, From: []string{a.Key}, Value: amount.Key})
		}
	}
	if got := len(menuList(t, m, ListAbilities)); got != 6 {
		t.Errorf("the abilities list has %d values, want 6", got)
	}
	if _, err := srd.With(Overlay{Revision: 1, Feats: []TableFeat{{TableEntry: TableEntry{Key: "feat:aumentos" + tableSuffix, NamePT: "Aumentos"}, Effects: increases[:1]}}}); err != nil {
		t.Fatalf("a feat with an ability_increase is refused: %v", err)
	}
	for i, e := range increases {
		key := "feat:aumento-" + strconv.Itoa(i) + tableSuffix
		if _, err := srd.With(Overlay{Revision: 1, Feats: []TableFeat{{TableEntry: TableEntry{Key: key, NamePT: "Aumento " + strconv.Itoa(i)}, Effects: []Effect{e}}}}); err != nil {
			t.Errorf("the menu offers an ability_increase %+v that With refuses: %v", e, err)
		}
	}

	// The picks of abilities and damage types are a feat's, as the choice kinds list says.
	for _, kind := range []string{pickAbility, pickDamageType} {
		key := "feat:pick-" + strings.ReplaceAll(kind, "_", "-") + tableSuffix
		o := Overlay{Revision: 1, Feats: []TableFeat{{TableEntry: TableEntry{Key: key, NamePT: "Pick"}, Effects: []Effect{{Type: "choice", Choice: kind, Name: "escolha", Count: 1}}}}}
		if _, err := srd.With(o); err != nil {
			t.Errorf("the menu offers the choice %s, which a feat is refused: %v", kind, err)
		}
	}

	// Every value of every list is accepted, in the field it goes in.
	var effects []Effect
	for _, v := range menuList(t, m, ListModifierTargets) {
		// What the menu says the target takes: its modes (every one by default) and a value within its bounds.
		value := "1"
		if v.Max > 0 {
			value = strconv.Itoa(v.Max)
		}
		for _, mode := range menuList(t, m, ListModifierModes) {
			e := Effect{Type: "modifier", Target: v.Key, Mode: mode.Key, Value: value}
			if strings.HasPrefix(v.Key, "score.") {
				e.Cap = 24 // a score modifier asks for its cap
			}
			if len(v.OnlyModes) == 0 || slices.Contains(v.OnlyModes, mode.Key) {
				effects = append(effects, e)
			} else if _, err := srd.With(Overlay{Revision: 1, Classes: []TableClass{classWithEffects([]Effect{e})}}); err == nil {
				t.Errorf("target %s takes only %v, yet mode %s is accepted", v.Key, v.OnlyModes, mode.Key)
			}
		}
		if v.Max > 0 {
			over := Effect{Type: "modifier", Target: v.Key, Mode: "set", Value: strconv.Itoa(v.Max + 1)}
			if _, err := srd.With(Overlay{Revision: 1, Classes: []TableClass{classWithEffects([]Effect{over})}}); err == nil {
				t.Errorf("target %s is bounded to %d..%d, yet %s is accepted", v.Key, v.Min, v.Max, over.Value)
			}
		}
	}
	for _, v := range menuList(t, m, ListRollApplies) {
		effects = append(effects, Effect{Type: "reroll", Rolls: []string{v.Key}, AtMost: 2})
	}
	for _, v := range menuList(t, m, ListRollApplies) {
		effects = append(effects, Effect{Type: "roll_minimum", Rolls: []string{v.Key}, Value: "2"})
	}
	for _, v := range menuList(t, m, ListCriticalScopes) {
		effects = append(effects, Effect{Type: "modifier", Target: CriticalRangeTarget, Mode: "set", Value: "19", Applies: v.Key})
	}
	for _, v := range menuList(t, m, ListModifierModes) {
		effects = append(effects, Effect{Type: "modifier", Target: "ac", Mode: v.Key, Value: "1"})
	}
	for _, v := range menuList(t, m, ListProficiencies) {
		effects = append(effects, Effect{Type: "proficiency", Proficiency: v.Key})
	}
	for _, v := range menuList(t, m, ListProficiencyLevels) {
		effects = append(effects, Effect{Type: "proficiency", Proficiency: "skill:arcana", Level: v.Key})
	}
	for _, v := range menuList(t, m, ListRollModes) {
		effects = append(effects, Effect{Type: "roll_mode", Roll: v.Key, Targets: []string{"attack"}})
	}
	for _, v := range menuList(t, m, ListRollTargets) {
		effects = append(effects, Effect{Type: "roll_mode", Roll: "advantage", Targets: []string{v.Key}})
	}
	for _, v := range menuList(t, m, ListSenses) {
		effects = append(effects, Effect{Type: "sense", Sense: v.Key, RangeFt: 30})
	}
	for i, v := range menuList(t, m, ListRecharges) {
		effects = append(effects, Effect{Type: "resource", Resource: "uso_" + strconv.Itoa(i), Max: "1", Recharge: v.Key})
	}
	for _, v := range menuList(t, m, ListEconomies) {
		effects = append(effects, Effect{Type: "grant_action", Economy: v.Key})
	}
	for _, v := range menuList(t, m, ListEvents) {
		effects = append(effects, Effect{Type: "on_event", Event: v.Key, TempHP: "2"})
	}
	for _, v := range menuList(t, m, ListSkills) {
		effects = append(effects, Effect{Type: "choice", Choice: "skill", Count: 1, From: []string{v.Key}})
	}
	for _, v := range menuList(t, m, ListLanguages) {
		effects = append(effects, Effect{Type: "choice", Choice: "language", Count: 1, From: []string{v.Key}})
	}
	for _, v := range menuList(t, m, ListTools) {
		effects = append(effects, Effect{Type: "choice", Choice: "tool", Count: 1, From: []string{v.Key}})
	}
	for _, kind := range menuList(t, m, ListChoiceKinds) {
		if kind.Key == "feature" || kind.Key == pickAbility || kind.Key == pickDamageType {
			continue // a feature choice needs its options (below); the picks are a feat's (below)
		}
		effects = append(effects, Effect{Type: "choice", Choice: kind.Key, Count: 1, From: nil})
	}
	for n := m.ExtraAttackMin; n <= m.ExtraAttackMax; n++ {
		effects = append(effects, Effect{Type: "extra_attack", Count: n})
	}
	for _, v := range menuList(t, m, ListRangedAttacks) {
		effects = append(effects, Effect{Type: "ranged_attack", Applies: v.Key, IgnoreCover: true})
	}
	for n := MinSpellRangeMultiplier; n <= MaxSpellRangeMultiplier; n++ {
		effects = append(effects, Effect{Type: "ranged_attack", Applies: RangedAttackSpell, RangeMultiplier: n})
	}
	effects = append(effects, Effect{Type: "ranged_attack", Applies: RangedAttackWeapon, NoLongRangeDisadvantage: true})
	// The states: each way to end and each economy, the weapon filters and the kinds of a pseudo-weapon.
	for _, v := range menuList(t, m, ListStateEnds) {
		effects = append(effects, Effect{Type: "state", StateName: "fim_" + v.Key, EndsOn: []string{v.Key}, Economy: EconomyFree})
	}
	for _, v := range menuList(t, m, ListStateEconomies) {
		effects = append(effects, Effect{Type: "state", StateName: "custo_" + v.Key, EndsOn: []string{StateEndsDismiss}, Economy: v.Key})
	}
	for _, v := range menuList(t, m, ListWeaponFilters) {
		e := Effect{Type: "weapon_rule", Weapon: v.Key, AttackBonus: 1}
		if v.Key == WeaponNamed {
			e.WeaponName = "Espada longa"
		}
		effects = append(effects, e)
	}
	for i, v := range menuList(t, m, ListWeaponKinds) {
		e := Effect{Type: "pseudo_weapon", AttackName: "Arma " + strconv.Itoa(i), Weapon: v.Key, DamageDice: "1d6", DamageType: "damage-type:force", Ability: "int"}
		if v.Key == WeaponRanged {
			e.RangeFt = 30
		}
		effects = append(effects, e)
	}
	// The option sets: each option of each set, offered by a feature choice.
	if len(m.OptionSets) == 0 {
		t.Fatal("the menu has no option set (fighting styles...)")
	}
	for _, set := range m.OptionSets {
		if set.NamePT == "" || set.Choose < 0 || len(set.Options) == 0 {
			t.Errorf("option set %+v is incomplete", set)
		}
		for _, o := range set.Options {
			if o.NamePT == "" {
				t.Errorf("option %s of %s has no name", o.Key, set.Key)
			}
		}
		for keys := valueKeys(set.Options); len(keys) > 0; {
			n := min(len(keys), maxEffectList)
			effects = append(effects, Effect{Type: "choice", Choice: "feature", Count: 1, From: keys[:n]})
			keys = keys[n:]
		}
	}
	// The tags: a prefix and something after it.
	for _, p := range menuList(t, m, ListTagPrefixes) {
		effects = append(effects, Effect{Type: "roll_mode", Roll: "advantage", Targets: []string{"attack"}, Tags: []string{p.Key + "teste"}})
	}
	if len(effects) < 100 {
		t.Fatalf("only %d effects built from the menu", len(effects))
	}
	o := Overlay{Revision: 1}
	// The same effect is only worth offering once; the budget counts the distinct
	// formulas, which here are "1".
	o.Classes = append(o.Classes, classWithEffects(effects))
	if _, err := srd.With(o); err != nil {
		t.Fatalf("the menu offers what With refuses: %v", err)
	}
}

// TestEffectMenuRequiredFieldsAreRequired: each field the menu marks required is
// refused when empty, and the effect is accepted with it.
func TestEffectMenuRequiredFieldsAreRequired(t *testing.T) {
	t.Parallel()
	srd := loadForTest(t)
	m := srd.EffectMenu()
	valid := map[string]Effect{
		"modifier":           {Type: "modifier", Target: "ac", Mode: "add", Value: "1"},
		"proficiency":        {Type: "proficiency", Proficiency: "skill:arcana"},
		"resource":           {Type: "resource", Resource: "surto", Max: "1", Recharge: "long_rest"},
		"sense":              {Type: "sense", Sense: "darkvision", RangeFt: 30},
		"roll_mode":          {Type: "roll_mode", Roll: "advantage", Targets: []string{"attack"}},
		"grant_action":       {Type: "grant_action", Economy: "action"},
		"extra_attack":       {Type: "extra_attack", Count: 2},
		"choice":             {Type: "choice", Choice: "skill", Count: 1},
		"note":               {Type: "note"},
		"superiority_die":    {Type: "superiority_die", Applies: ManeuverDamage, Resource: "dados_de_teste", Value: "8"},
		"ranged_attack":      {Type: "ranged_attack", Applies: RangedAttackSpell, IgnoreCover: true},
		"extra_damage":       {Type: "extra_damage", Applies: ExtraDamageLargerTarget, Value: "2", Resource: "dados_de_teste"},
		"ambush":             {Type: "ambush", Applies: AmbushUnactedAdvantage},
		"reroll":             {Type: "reroll", Rolls: []string{RollAppliesAttack}, AtMost: 2},
		"roll_minimum":       {Type: "roll_minimum", Rolls: []string{RollDamage}, Value: "2"},
		"wild_shape":         {Type: "wild_shape", MaxCR: "1"},
		"slot_recovery":      {Type: "slot_recovery", Resource: "dados_de_teste", RecoverMaxLevel: "3", RecoverLevels: "2"},
		"spell_modifier":     {Type: "spell_modifier", AppliesTo: SpellAppliesCantrip, SpellChange: SpellChangeExtraDice, Count: 1},
		"on_event":           {Type: "on_event", Event: EventLongRest, TempHP: "level()"},
		"resistance":         {Type: "resistance", Except: "damage-type:psychic", While: ResistanceWhileRage},
		"state":              {Type: "state", StateName: "estado_de_teste", EndsOn: []string{StateEndsDismiss}, Economy: EconomyFree},
		"weapon_rule":        {Type: "weapon_rule", Weapon: WeaponAny, AttackBonus: 1},
		"pseudo_weapon":      {Type: "pseudo_weapon", AttackName: "Raio de Teste", Weapon: WeaponRanged, DamageDice: "1d6", DamageType: "damage-type:force", Ability: "int", RangeFt: 60},
		"condition_immunity": {Type: "condition_immunity", Conditions: []string{"condition:poisoned"}},
		"breathes_water":     {Type: "breathes_water"},
		"granted_spell":      {Type: "granted_spell", Spell: "spell:detect-magic", Ability: "wis"},
		// Only a feat has it, so it is hosted by a feat below.
		"ability_increase": {Type: "ability_increase", Count: 1, From: []string{"str", "dex"}, Value: "1"},
	}
	clearField := func(e *Effect, field string) {
		switch field {
		case "target":
			e.Target = ""
		case "mode":
			e.Mode = ""
		case "value":
			e.Value = ""
		case "proficiency":
			e.Proficiency = ""
		case "resource":
			e.Resource = ""
		case "max":
			e.Max = ""
		case "recharge":
			e.Recharge = ""
		case "sense":
			e.Sense = ""
		case "range_ft":
			e.RangeFt = 0
		case "roll":
			e.Roll = ""
		case "targets":
			e.Targets = nil
		case "economy":
			e.Economy = ""
		case "applies":
			e.Applies = ""
		case "applies_to":
			e.AppliesTo = ""
		case "spell_change":
			e.SpellChange = ""
		case "count":
			e.Count = 0
		case "spell":
			e.Spell = ""
		case "ability":
			e.Ability = ""
		case "except":
			e.Except = ""
		case "while":
			e.While = ""
		case "state_name":
			e.StateName = ""
		case "weapon":
			e.Weapon = ""
		case "attack_name":
			e.AttackName = ""
		case "damage_dice":
			e.DamageDice = ""
		case "damage_type":
			e.DamageType = ""
		case "conditions":
			e.Conditions = nil
		case "event":
			e.Event = ""
		case "choice":
			e.Choice = ""
		case "from":
			e.From = nil
		case "recover_max_level":
			e.RecoverMaxLevel = ""
		case "recover_levels":
			e.RecoverLevels = ""
		case "rolls":
			e.Rolls = nil
		case "at_most":
			e.AtMost = 0
		default:
			t.Fatalf("no way to clear %q", field)
		}
	}
	with := func(e Effect) error {
		if e.Type == "ability_increase" {
			_, err := srd.With(Overlay{Revision: 1, Feats: []TableFeat{{TableEntry: TableEntry{Key: "feat:cardapio" + tableSuffix, NamePT: "Cardápio"}, Effects: []Effect{e}}}})
			return err
		}
		effects := []Effect{e}
		if (e.Type == "superiority_die" || e.Type == "extra_damage" || e.Type == "slot_recovery") && e.Resource != "" {
			// The die spends a resource the same class defines.
			effects = []Effect{{Type: "resource", Resource: "dados_de_teste", Max: "4", Recharge: "short_rest"}, e}
		}
		_, err := srd.With(Overlay{Revision: 1, Classes: []TableClass{classWithEffects(effects)}})
		return err
	}
	for _, mt := range m.Types {
		e := valid[mt.Type]
		if err := with(e); err != nil {
			t.Fatalf("the valid %s effect is refused: %v", mt.Type, err)
		}
		for _, f := range mt.Fields {
			if !f.Required {
				continue
			}
			broken := e
			clearField(&broken, f.Name)
			if with(broken) == nil {
				t.Errorf("a %s effect without %s is accepted, the menu says it is required", mt.Type, f.Name)
			}
		}
	}
	// Bounds the menu states.
	for _, mt := range m.Types {
		for _, f := range mt.Fields {
			if f.Name == "count" && mt.Type == "extra_attack" && (f.Min != m.ExtraAttackMin || f.Max != m.ExtraAttackMax) {
				t.Errorf("extra_attack count bounds = %d..%d, menu says %d..%d", f.Min, f.Max, m.ExtraAttackMin, m.ExtraAttackMax)
			}
		}
	}
	for _, n := range []int{m.ExtraAttackMin - 1, m.ExtraAttackMax + 1} {
		if with(Effect{Type: "extra_attack", Count: n}) == nil {
			t.Errorf("extra_attack with %d attacks is accepted", n)
		}
	}
	// A ranged_attack's multiplier: the bounds the menu states are the validator's, and
	// its switches are shown though none is required alone.
	for _, mt := range m.Types {
		for _, f := range mt.Fields {
			if mt.Type != "ranged_attack" || f.Name == "applies" || f.Name == "when" || f.Name == "text_pt" {
				continue
			}
			if !f.AlwaysShown {
				t.Errorf("ranged_attack's %s is folded: what the effect does is in it", f.Name)
			}
			if f.Name == "range_multiplier" && (f.Min != MinSpellRangeMultiplier || f.Max != MaxSpellRangeMultiplier) {
				t.Errorf("range_multiplier bounds = %d..%d, want %d..%d", f.Min, f.Max, MinSpellRangeMultiplier, MaxSpellRangeMultiplier)
			}
		}
	}
	for _, n := range []int{MinSpellRangeMultiplier - 1, MaxSpellRangeMultiplier + 1} {
		if with(Effect{Type: "ranged_attack", Applies: RangedAttackSpell, RangeMultiplier: n}) == nil {
			t.Errorf("a ranged_attack that multiplies the range by %d is accepted", n)
		}
	}
}

// TestEffectMenuFormulaHelpers: each helper the menu lists compiles where it is
// meant to, with the table's class indexes; one the menu does not list does not.
func TestEffectMenuFormulaHelpers(t *testing.T) {
	t.Parallel()
	srd := loadForTest(t)
	o := genOverlay(t, srd)
	c := withOverlayOn(t, srd, o)
	m := c.EffectMenu()

	samples := map[string]string{
		"level()": "level()", `classLevel("<classe>")`: `classLevel("wizard")`, `mod("<habilidade>")`: `mod("int")`,
		`score("<habilidade>")`: `score("dex")`, "prof()": "prof()", "floor(x)": `floor(level() / 2)`, "ceil(x)": `ceil(level() / 3)`,
		"min(a, b)": "min(level(), 5)", "max(a, b)": "max(level(), 1)",
	}
	conditions := map[string]string{"armor()": `armor() == "none"`, "shield()": "shield()"}
	for _, h := range m.Helpers {
		if h.HintPT == "" {
			t.Errorf("helper %s has no hint", h.Call)
		}
		if h.Call == `active("<estado>")` {
			// A gate: it reads a state of the same entry.
			state := Effect{Type: "state", StateName: "estado", EndsOn: []string{StateEndsDismiss}, Economy: EconomyFree}
			gated := Effect{Type: "modifier", Target: "ac", Mode: "add", Value: "1", When: `active("estado")`}
			if _, err := srd.With(Overlay{Revision: 1, Classes: []TableClass{classWithEffects([]Effect{state, gated})}}); err != nil {
				t.Errorf("helper %s is refused: %v", h.Call, err)
			}
			continue
		}
		var e Effect
		switch {
		case h.Call == "speedwalk()":
			e = Effect{Type: "modifier", Target: "speed.swim", Mode: "set", Value: "speedwalk()"}
		case samples[h.Call] != "":
			e = Effect{Type: "modifier", Target: "ac", Mode: "add", Value: samples[h.Call]}
		case conditions[h.Call] != "":
			e = Effect{Type: "modifier", Target: "ac", Mode: "add", Value: "1", When: conditions[h.Call]}
		default:
			t.Errorf("no sample for helper %s", h.Call)
			continue
		}
		if _, err := srd.With(Overlay{Revision: 1, Classes: []TableClass{classWithEffects([]Effect{e})}}); err != nil {
			t.Errorf("helper %s is refused: %v", h.Call, err)
		}
	}
	if _, err := srd.With(Overlay{Revision: 1, Classes: []TableClass{classWithEffects([]Effect{{Type: "modifier", Target: "ac", Mode: "add", Value: "random()"}})}}); err == nil {
		t.Error("a helper the menu does not list is accepted")
	}

	// classLevel takes the indexes the menu lists, the table's included.
	for _, idx := range []string{"wizard", "gen-full-prepared@mesa", "fighter"} {
		if !slices.Contains(m.ClassIndexes, idx) {
			t.Errorf("class index %q is not in the menu: %v", idx, m.ClassIndexes)
		}
	}
	if slices.Contains(srd.EffectMenu().ClassIndexes, "gen-full-prepared@mesa") {
		t.Error("the SRD's menu lists a table class")
	}
	if m.MaxFeaturesPerClass != MaxTableFeatures || m.MaxEffectsPerFeature != MaxFeatureEffects {
		t.Errorf("limits = %d, %d", m.MaxFeaturesPerClass, m.MaxEffectsPerFeature)
	}
}

// TestEffectMenuNamesArePortuguese: every value of every closed list and every
// option set has a name the content translated, never the English fallback.
func TestEffectMenuNamesArePortuguese(t *testing.T) {
	t.Parallel()
	srd := loadForTest(t)
	m := srd.EffectMenu()
	missing := map[string]bool{}
	check := func(list, key string) {
		// The menu's own words (modes, senses, tag prefixes...) are not content keys.
		if strings.ContainsAny(key, ".*") || !strings.Contains(key, ":") || strings.HasSuffix(key, ":") {
			return
		}
		_, own := srd.c.namesPT[key]
		viaRef := false
		if p := srd.c.proficiencies[key]; p != nil && len(p.Refs) == 1 {
			_, viaRef = srd.c.namesPT[p.Refs[0]]
		}
		if !own && !viaRef {
			missing[list+" "+key] = true
		}
	}
	for _, l := range m.Lists {
		for _, v := range l.Values {
			check(l.Name, v.Key)
		}
	}
	setNames := map[string]string{}
	for _, set := range m.OptionSets {
		if other, dup := setNames[set.NamePT]; dup {
			t.Errorf("option sets %s and %s are both called %q", other, set.Key, set.NamePT)
		}
		setNames[set.NamePT] = set.Key
		check("option set", set.Key)
		for _, o := range set.Options {
			check("option of "+set.Key, o.Key)
		}
	}
	if len(missing) > 0 {
		t.Errorf("%d menu values with no Portuguese name: %v", len(missing), sortedKeys(missing))
	}
}

// TestEffectMenuRerollAndCriticalCopy pins the approved board copy of the reroll, the minimum and the critical range.
func TestEffectMenuRerollAndCriticalCopy(t *testing.T) {
	t.Parallel()
	m := loadForTest(t).EffectMenu()
	hints := map[string]string{}
	for _, mt := range m.Types {
		hints[mt.Type] = mt.HintPT
	}
	if got := hints["reroll"]; got != "Rola de novo um dado que caiu baixo. A nova rolagem vale, mesmo se for pior." {
		t.Errorf("reroll hint = %q", got)
	}
	if hints["roll_minimum"] == "" || len(hints["roll_minimum"]) > 140 {
		t.Errorf("roll_minimum hint = %q, want a short one", hints["roll_minimum"])
	}
	for _, v := range menuList(t, m, ListModifierTargets) {
		if v.Key == CriticalRangeTarget && (v.NamePT != "Margem de acerto crítico" || v.HintPT != "O menor d20 natural que é acerto crítico.") {
			t.Errorf("critical range = %q / %q", v.NamePT, v.HintPT)
		}
	}
	for _, v := range menuList(t, m, ListRollApplies) {
		want := map[string]string{RollAppliesAttack: "Jogada de ataque (o d20)", RollAppliesCheck: "Teste de habilidade (o d20)", RollAppliesSave: "Teste de resistência (o d20)", RollDamage: "Dado de dano (de arma ou magia)"}[v.Key]
		if v.NamePT != want {
			t.Errorf("roll %s = %q, want %q", v.Key, v.NamePT, want)
		}
	}
}

// TestLackableFeaturesAreTheSRDKeysAFeatMayName: the list holds the SRD features, traits, background
// features and feats a feat's lacks_feature may name (each value passes isFeatureKey), by name, with where each comes from, and a
// table entry never appears in it.
func TestLackableFeaturesAreTheSRDKeysAFeatMayName(t *testing.T) {
	overlay := Overlay{Revision: 1, Feats: []TableFeat{{
		TableEntry: TableEntry{Key: "feat:livre" + tableSuffix, NamePT: "Livre"}, DescPT: []string{"Texto."},
	}}}
	for name, c := range map[string]*Content{"srd": loadForTest(t), "with a table feat": withOverlay(t, overlay)} {
		t.Run(name, func(t *testing.T) {
			values := menuList(t, c.EffectMenu(), ListLackableFeatures)
			if len(values) < 300 {
				t.Fatalf("the list has %d values, want the SRD's features, traits and feats", len(values))
			}
			byKey := map[string]MenuValue{}
			for _, v := range values {
				byKey[v.Key] = v
				if !c.c.isFeatureKey(v.Key) || isTableKey(v.Key) || v.NamePT == "" || v.HintPT == "" {
					t.Errorf("value %+v: want an SRD key isFeatureKey accepts, a name and an origin", v)
				}
			}
			if len(byKey) != len(values) {
				t.Errorf("the list has %d values for %d keys", len(values), len(byKey))
			}
			for key, hint := range map[string]string{
				"feature:stunning-strike": "Monge, nível 5", "feat:grappler": "talento do SRD",
			} {
				if got := byKey[key].HintPT; got != hint {
					t.Errorf("hint of %s = %q, want %q", key, got, hint)
				}
			}
			for key := range c.c.traits {
				if !isTableKey(key) && byKey[key].HintPT != "traço do SRD" {
					t.Errorf("trait %s: hint = %q, want %q", key, byKey[key].HintPT, "traço do SRD")
					break
				}
			}
			if _, ok := byKey["feat:livre"+tableSuffix]; ok {
				t.Error("the list names a table feat")
			}
		})
	}
}

// TestRaceTraitsAreTheSRDKeysASubraceMayReplace: every value is a trait of the SRD race in its group, and
// a subrace of that race replacing all of them is accepted by the same check the server uses
// (checkReplacedTraits); a table race or trait never appears in it.
func TestRaceTraitsAreTheSRDKeysASubraceMayReplace(t *testing.T) {
	base := loadForTest(t)
	values := menuList(t, base.EffectMenu(), ListRaceTraits)
	if len(values) < 20 {
		t.Fatalf("the list has %d values, want the traits of the SRD races", len(values))
	}
	byRace := map[string][]string{}
	seen := map[string]bool{}
	hinted := 0
	for _, v := range values {
		if v.NamePT == "" || v.Group == "" || isTableKey(v.Key) || isTableKey(v.Group) || seen[v.Group+v.Key] {
			t.Errorf("value %+v: want a name, an SRD race and an SRD key, once", v)
		}
		seen[v.Group+v.Key] = true
		if v.HintPT != "" {
			hinted++
		}
		byRace[v.Group] = append(byRace[v.Group], v.Key)
	}
	if hinted == 0 {
		t.Error("no trait has a summary")
	}
	for race, keys := range byRace {
		o := Overlay{Revision: 1, Subraces: []TableSubrace{{
			TableEntry: TableEntry{Key: "subrace:teste" + tableSuffix, NamePT: "Teste"}, Race: race, ReplacesTraitKeys: keys,
		}}}
		if _, err := base.With(o); err != nil {
			t.Errorf("a subrace of %s replacing %v: %v", race, keys, err)
		}
		if got := len(base.c.races[race].Traits); got < len(keys) {
			t.Errorf("%s: %d values for %d traits", race, len(keys), got)
		}
	}
	for key, r := range base.c.races {
		if _, ok := byRace[key]; !ok && len(r.Traits) > 0 {
			t.Errorf("the race %s has traits and none is in the list", key)
		}
	}
}
