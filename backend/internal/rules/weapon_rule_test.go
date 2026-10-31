package rules

import (
	"errors"
	"slices"
	"strings"
	"testing"
)

// The weapon_rule effect (a bonus, an ability or a reach for some weapons) and the pseudo_weapon
// effect (an attack a feature gives, never in the inventory). Every name here is invented.

const (
	ruleLongsword = "equipment:longsword"
	ruleRapier    = "equipment:rapier"
	ruleShortbow  = "equipment:shortbow"
	ruleDagger    = "equipment:dagger"
)

// ruleBuild is a 3rd-level fighter-subclass character with four weapons and scores that tell the
// abilities apart: STR +0, DEX +1, INT +3, CHA +2.
func ruleBuild(active ...string) Build {
	b := stateBuild(3, active...)
	b.BaseScores = map[Ability]int{STR: 10, DEX: 12, CON: 13, INT: 16, WIS: 10, CHA: 14}
	b.Weapons = []string{ruleLongsword, ruleRapier, ruleShortbow, ruleDagger}
	return b
}

// ruleAttacks derives the build with the effects and returns its attack lines by key.
func ruleAttacks(t *testing.T, b Build, effects ...Effect) map[string]Attack {
	t.Helper()
	c := withOverlay(t, stateOverlay(effects))
	out := map[string]Attack{}
	for _, a := range Derive(b, c).Attacks {
		out[a.Key] = a
	}
	return out
}

func baseAttacks(t *testing.T) map[string]Attack {
	t.Helper()
	return ruleAttacks(t, ruleBuild(), Effect{Type: "note", TextPT: "Sem regra."})
}

func TestWeaponRuleWithoutAnyRuleLeavesTheLinesAsTheyWere(t *testing.T) {
	a := baseAttacks(t)
	if got := a[ruleLongsword]; got.Ability != STR || got.AttackBonus != 2 || got.Damage != "1d8" {
		t.Fatalf("longsword = %+v, want STR, +2, 1d8", got)
	}
	if got := a[ruleRapier]; got.Ability != DEX || got.AttackBonus != 3 || got.Damage != "1d8+1" {
		t.Fatalf("rapier = %+v, want DEX, +3, 1d8+1", got)
	}
}

func TestWeaponRuleUsesTheBestOfTheAbilitiesForTheAttackAndForTheDamage(t *testing.T) {
	rule := Effect{Type: "weapon_rule", Weapon: WeaponAny, AttackAbility: "int", DamageAbility: "cha"}
	a := ruleAttacks(t, ruleBuild(), rule)
	// The longsword takes the rule's abilities: Intelligence (+3) to hit and Charisma (+2) to hurt.
	if got := a[ruleLongsword]; got.Ability != INT || got.AttackBonus != 5 || got.Damage != "1d8+2" || got.AbilityMod != 2 {
		t.Errorf("longsword = %+v, want INT, +5 (3 + proficiency 2), 1d8+2 with an ability modifier of 2", got)
	}
	// The rapier's own Dexterity (+1) loses to both: the best of the options wins, as for a finesse weapon.
	if got := a[ruleRapier]; got.Ability != INT || got.AttackBonus != 5 || got.Damage != "1d8+2" {
		t.Errorf("rapier = %+v, want INT, +5, 1d8+2", got)
	}
	// A rule that offers a worse ability changes nothing.
	weaker := ruleAttacks(t, ruleBuild(), Effect{Type: "weapon_rule", Weapon: WeaponAny, AttackAbility: "str", DamageAbility: "str"})
	if got := weaker[ruleRapier]; got.Ability != DEX || got.AttackBonus != 3 || got.Damage != "1d8+1" {
		t.Errorf("rapier with a worse ability on offer = %+v, want it unchanged", got)
	}
	// Two rules offer two abilities: the best of all of them.
	both := ruleAttacks(t, ruleBuild(),
		Effect{Type: "weapon_rule", Weapon: WeaponAny, AttackAbility: "cha"},
		Effect{Type: "weapon_rule", Weapon: WeaponMelee, AttackAbility: "int"})
	if both[ruleLongsword].Ability != INT || both[ruleShortbow].Ability != CHA {
		t.Errorf("abilities = %s, %s, want INT on the melee weapon and CHA on the bow (+2 beats its DEX +1)", both[ruleLongsword].Ability, both[ruleShortbow].Ability)
	}
}

func TestWeaponRuleFiltersChooseTheWeapons(t *testing.T) {
	cases := []struct {
		filter, name string
		hit, miss    []string
	}{
		{WeaponMelee, "", []string{ruleLongsword, ruleRapier, ruleDagger, UnarmedStrikeKey}, []string{ruleShortbow}},
		{WeaponRanged, "", []string{ruleShortbow}, []string{ruleLongsword, ruleRapier, UnarmedStrikeKey}},
		{WeaponFinesse, "", []string{ruleRapier, ruleDagger}, []string{ruleLongsword, ruleShortbow, UnarmedStrikeKey}},
		{WeaponAny, "", []string{ruleLongsword, ruleRapier, ruleShortbow, ruleDagger, UnarmedStrikeKey}, nil},
		{WeaponNamed, "Espada longa", []string{ruleLongsword}, []string{ruleRapier, ruleShortbow, ruleDagger}},
		{WeaponNamed, "  rapieira ", []string{ruleRapier}, []string{ruleLongsword}},
		{WeaponNamed, "ESPADA LONGA", []string{ruleLongsword}, []string{ruleRapier}},
		{WeaponMagic, "", nil, []string{ruleLongsword, ruleRapier, ruleShortbow, ruleDagger}},
	}
	base := baseAttacks(t)
	for _, tc := range cases {
		a := ruleAttacks(t, ruleBuild(), Effect{Type: "weapon_rule", Weapon: tc.filter, WeaponName: tc.name, AttackBonus: 1, DamageBonus: 2})
		for _, k := range tc.hit {
			if a[k].AttackBonus != base[k].AttackBonus+1 || a[k].DamageDice.Bonus != base[k].DamageDice.Bonus+2 {
				t.Errorf("%s %q: %s = +%d %+v, want +1 to hit and +2 to damage over %+v", tc.filter, tc.name, k, a[k].AttackBonus, a[k].DamageDice, base[k].DamageDice)
			}
		}
		for _, k := range tc.miss {
			if a[k].AttackBonus != base[k].AttackBonus || a[k].Damage != base[k].Damage {
				t.Errorf("%s %q: %s changed to %+v, want %+v", tc.filter, tc.name, k, a[k], base[k])
			}
		}
	}
}

func TestWeaponRuleBonusesAddUpAndMayBeNegative(t *testing.T) {
	a := ruleAttacks(t, ruleBuild(),
		Effect{Type: "weapon_rule", Weapon: WeaponAny, AttackBonus: 2, DamageBonus: 1},
		Effect{Type: "weapon_rule", Weapon: WeaponMelee, AttackBonus: -1, DamageBonus: 3})
	if got := a[ruleLongsword]; got.AttackBonus != 2+2-1 || got.Damage != "1d8+4" || got.Ability != STR {
		t.Errorf("longsword = %+v, want +3 to hit, 1d8+4, still STR (the bonuses are not an ability)", got)
	}
	if got := a[ruleShortbow]; got.AttackBonus != 3+2 || got.Damage != "1d6+2" {
		t.Errorf("shortbow = %+v, want +5 to hit and +1 over its own damage modifier", got)
	}
}

func TestWeaponRuleReachWidensMeleeLinesOnly(t *testing.T) {
	a := ruleAttacks(t, ruleBuild(), Effect{Type: "weapon_rule", Weapon: WeaponAny, ReachFt: 10})
	base := baseAttacks(t)
	if u := a[UnarmedStrikeKey]; u.ReachFt != 10 || u.RangeFt != 10 {
		t.Errorf("unarmed strike: reach %d, range %d, want 10 and 10 (it has no range of its own)", u.ReachFt, u.RangeFt)
	}
	for _, k := range []string{ruleLongsword, ruleRapier} {
		if a[k].ReachFt != 10 || a[k].RangeFt != baseAttacks(t)[k].RangeFt {
			t.Errorf("%s: reach %d, range %d, want a reach of 10 and the weapon's own range %d", k, a[k].ReachFt, a[k].RangeFt, baseAttacks(t)[k].RangeFt)
		}
	}
	if a[ruleShortbow].RangeFt != base[ruleShortbow].RangeFt || a[ruleShortbow].ReachFt != 0 {
		t.Errorf("the bow changed: %+v, want %+v", a[ruleShortbow], base[ruleShortbow])
	}
	// A weapon thrown farther than the rule's reach keeps its throwing range.
	if a[ruleDagger].RangeFt != base[ruleDagger].RangeFt || a[ruleDagger].ReachFt != 10 {
		t.Errorf("dagger = %+v, want the throwing range %d and a reach of 10", a[ruleDagger], base[ruleDagger].RangeFt)
	}
}

func TestWeaponRuleAndPseudoWeaponFollowTheirState(t *testing.T) {
	effects := append(stanceEffects(),
		Effect{Type: "weapon_rule", Weapon: WeaponAny, AttackBonus: 1, While: stateName},
		Effect{Type: "pseudo_weapon", AttackName: "Raio de Teste", Weapon: WeaponRanged, DamageDice: "1d6", DamageType: "damage-type:force", Ability: "int", RangeFt: 60, While: stateName},
	)
	off := ruleAttacks(t, ruleBuild(), effects...)
	on := ruleAttacks(t, ruleBuild(stateKeyOfTest()), effects...)
	if off[ruleLongsword].AttackBonus != 2 || on[ruleLongsword].AttackBonus != 3 {
		t.Errorf("longsword to hit = %d off, %d on, want 2 and 3", off[ruleLongsword].AttackBonus, on[ruleLongsword].AttackBonus)
	}
	if len(on) != len(off)+1 {
		t.Errorf("%d lines on, %d off: the pseudo-weapon should join only while the state is on", len(on), len(off))
	}
}

func TestPseudoWeaponIsAnAttackLineWithTheFeaturesNumbers(t *testing.T) {
	ray := Effect{Type: "pseudo_weapon", AttackName: "Raio de Teste", Weapon: WeaponRanged, DamageDice: "1d6", DamageType: "damage-type:force", Ability: "int", RangeFt: 60}
	b := ruleBuild()
	c := withOverlay(t, stateOverlay([]Effect{ray}))
	d := Derive(b, c)

	i := slices.IndexFunc(d.Attacks, func(a Attack) bool { return a.NamePT == "Raio de Teste" })
	if i < 0 {
		t.Fatalf("no attack line: %+v", d.Attacks)
	}
	a := d.Attacks[i]
	if a.Kind != "weapon" || a.Ability != INT || !a.Proficient || a.Melee || a.Magical {
		t.Errorf("line = %+v, want a proficient ranged weapon line that uses Intelligence", a)
	}
	if a.AttackBonus != 3+2 || a.Damage != "1d6+3" || a.DamageDice != (DiceFormula{Count: 1, Sides: 6, Bonus: 3}) || a.AbilityMod != 3 {
		t.Errorf("numbers = +%d, %q, %+v, ability modifier %d, want +5 (3 + proficiency 2), 1d6+3", a.AttackBonus, a.Damage, a.DamageDice, a.AbilityMod)
	}
	if a.DamageType != "damage-type:force" || a.DamageTypeNamePT == "" || a.RangeFt != 60 {
		t.Errorf("damage type %q (%q), range %d, want force, named, 60 ft", a.DamageType, a.DamageTypeNamePT, a.RangeFt)
	}
	if a.Key == "" {
		t.Error("the line has no key")
	}
	// It joins the list after the carried weapons and before the unarmed strike, and the inventory is untouched.
	if last := d.Attacks[len(d.Attacks)-1]; last.Key != UnarmedStrikeKey || i != len(d.Attacks)-2 {
		t.Errorf("the line is at %d of %d, the last one is %s", i, len(d.Attacks), last.Key)
	}
	if !slices.Equal(b.Weapons, []string{ruleLongsword, ruleRapier, ruleShortbow, ruleDagger}) {
		t.Errorf("weapons = %v: a pseudo-weapon is never in the inventory", b.Weapons)
	}
}

func TestPseudoWeaponTakesTheWeaponRulesLikeAnyWeapon(t *testing.T) {
	sword := Effect{Type: "pseudo_weapon", AttackName: "Lâmina de Teste", Weapon: WeaponMelee, DamageDice: "1d8", DamageType: "damage-type:force", Ability: "cha", ReachFt: 10}
	rules := []Effect{
		sword,
		{Type: "weapon_rule", Weapon: WeaponNamed, WeaponName: "Lâmina de Teste", AttackBonus: 1, DamageBonus: 2},
		{Type: "weapon_rule", Weapon: WeaponMelee, AttackAbility: "int"},
	}
	c := withOverlay(t, stateOverlay(rules))
	d := Derive(ruleBuild(), c)
	i := slices.IndexFunc(d.Attacks, func(a Attack) bool { return a.NamePT == "Lâmina de Teste" })
	if i < 0 {
		t.Fatal("no line")
	}
	a := d.Attacks[i]
	if !a.Melee || a.ReachFt != 10 || a.RangeFt != 10 {
		t.Errorf("line = %+v, want a melee line with a reach of 10 ft", a)
	}
	// Intelligence (+3) beats Charisma (+2) to hit; the damage keeps Charisma; both bonuses add.
	if a.Ability != INT || a.AttackBonus != 3+2+1 || a.Damage != "1d8+4" {
		t.Errorf("line = %v +%d %q, want INT, +6 to hit, 1d8+4", a.Ability, a.AttackBonus, a.Damage)
	}
}

func TestTwoPseudoWeaponsOfOneFeatureHaveOwnKeys(t *testing.T) {
	one := Effect{Type: "pseudo_weapon", AttackName: "Raio A", Weapon: WeaponRanged, DamageDice: "1d4", DamageType: "damage-type:fire", Ability: "int", RangeFt: 30}
	two := one
	two.AttackName = "Raio B"
	c := withOverlay(t, stateOverlay([]Effect{one, two}))
	seen := map[string]bool{}
	for _, a := range Derive(ruleBuild(), c).Attacks {
		if seen[a.Key] {
			t.Errorf("key %q is on two lines", a.Key)
		}
		seen[a.Key] = true
	}
}

// A gated pseudo-weapon that is off still counts, so the line of the one after it keeps its key.
func TestAPseudoWeaponKeepsItsKeyWhileAnEarlierOneIsOff(t *testing.T) {
	gated := Effect{Type: "pseudo_weapon", AttackName: "Raio A", Weapon: WeaponRanged, DamageDice: "1d4", DamageType: "damage-type:fire", Ability: "int", RangeFt: 30, While: stateName}
	plain := gated
	plain.AttackName, plain.While = "Raio B", ""
	effects := append(stanceEffects(), gated, plain)
	want := PseudoWeaponKey(stateFeature, 2)
	for _, on := range []bool{false, true} {
		b := ruleBuild()
		if on {
			b = ruleBuild(stateKeyOfTest())
		}
		got := ruleAttacks(t, b, effects...)
		if a, ok := got[want]; !ok || a.NamePT != "Raio B" {
			t.Errorf("state on=%v: line %q = %+v, want Raio B", on, want, a)
		}
		if _, ok := got[PseudoWeaponKey(stateFeature, 1)]; ok != on {
			t.Errorf("state on=%v: the first line present = %v", on, ok)
		}
	}
}

var weaponRuleRefusals = []struct {
	name  string
	edit  func(e *Effect)
	field string
}{
	{"no filter", func(e *Effect) { e.Weapon = "" }, ".weapon"},
	{"an unknown filter", func(e *Effect) { e.Weapon = "heavy" }, ".weapon"},
	{"a named filter with no name", func(e *Effect) { e.Weapon, e.WeaponName = WeaponNamed, "" }, ".weapon_name"},
	{"a name that is blank", func(e *Effect) { e.Weapon, e.WeaponName = WeaponNamed, "   " }, ".weapon_name"},
	{"a name over 60 characters", func(e *Effect) { e.Weapon, e.WeaponName = WeaponNamed, strings.Repeat("a", 61) }, ".weapon_name"},
	{"a name with another filter", func(e *Effect) { e.WeaponName = "Espada longa" }, ".weapon_name"},
	{"an unknown attack ability", func(e *Effect) { e.AttackAbility = "luck" }, ".attack_ability"},
	{"an unknown damage ability", func(e *Effect) { e.DamageAbility = "luck" }, ".damage_ability"},
	{"an attack bonus too high", func(e *Effect) { e.AttackBonus = MaxWeaponBonus + 1 }, ".attack_bonus"},
	{"an attack bonus too low", func(e *Effect) { e.AttackBonus = -MaxWeaponPenalty - 1 }, ".attack_bonus"},
	{"a damage bonus too high", func(e *Effect) { e.DamageBonus = MaxWeaponBonus + 1 }, ".damage_bonus"},
	{"a damage bonus too low", func(e *Effect) { e.DamageBonus = -MaxWeaponPenalty - 1 }, ".damage_bonus"},
	{"a reach off the 5 ft grid", func(e *Effect) { e.ReachFt = 7 }, ".reach_ft"},
	{"a reach too long", func(e *Effect) { e.ReachFt = MaxWeaponReachFt + 5 }, ".reach_ft"},
	{"a reach for ranged weapons", func(e *Effect) { e.Weapon, e.ReachFt = WeaponRanged, 10 }, ".reach_ft"},
	{"nothing to change", func(e *Effect) {
		e.AttackAbility, e.DamageAbility, e.AttackBonus, e.DamageBonus, e.ReachFt = "", "", 0, 0, 0
	}, ".type"},
	{"tags", func(e *Effect) { e.Tags = []string{"about:teste"} }, ".tags"},
	{"a state of nobody", func(e *Effect) { e.While = "fantasma" }, ".while"},
	{"a field of another type", func(e *Effect) { e.Target = "ac" }, ".target"},
	{"a field of a pseudo-weapon", func(e *Effect) { e.DamageDice = "1d6" }, ".damage_dice"},
}

var pseudoWeaponRefusals = []struct {
	name  string
	edit  func(e *Effect)
	field string
}{
	{"no name", func(e *Effect) { e.AttackName = "" }, ".attack_name"},
	{"a name over 40 characters", func(e *Effect) { e.AttackName = strings.Repeat("a", 41) }, ".attack_name"},
	{"no kind", func(e *Effect) { e.Weapon = "" }, ".weapon"},
	{"a kind that is a filter", func(e *Effect) { e.Weapon = WeaponFinesse }, ".weapon"},
	{"no dice", func(e *Effect) { e.DamageDice = "" }, ".damage_dice"},
	{"a die that does not exist", func(e *Effect) { e.DamageDice = "1d7" }, ".damage_dice"},
	{"dice that do not parse", func(e *Effect) { e.DamageDice = "um d6" }, ".damage_dice"},
	{"dice with a bonus", func(e *Effect) { e.DamageDice = "1d6+1" }, ".damage_dice"},
	{"no dice at all", func(e *Effect) { e.DamageDice = "0d6" }, ".damage_dice"},
	{"too many dice", func(e *Effect) { e.DamageDice = "11d6" }, ".damage_dice"},
	{"no damage type", func(e *Effect) { e.DamageType = "" }, ".damage_type"},
	{"an unknown damage type", func(e *Effect) { e.DamageType = "damage-type:fixture-none" }, ".damage_type"},
	{"a key that is no damage type", func(e *Effect) { e.DamageType = "skill:arcana" }, ".damage_type"},
	{"no ability", func(e *Effect) { e.Ability = "" }, ".ability"},
	{"an unknown ability", func(e *Effect) { e.Ability = "luck" }, ".ability"},
	{"a ranged weapon with no range", func(e *Effect) { e.RangeFt = 0 }, ".range_ft"},
	{"a range off the grid", func(e *Effect) { e.RangeFt = 7 }, ".range_ft"},
	{"a range too long", func(e *Effect) { e.RangeFt = MaxWeaponRangeFt + 5 }, ".range_ft"},
	{"a reach for a ranged weapon", func(e *Effect) { e.ReachFt = 10 }, ".reach_ft"},
	{"a melee weapon with a range", func(e *Effect) { e.Weapon = WeaponMelee }, ".range_ft"},
	{"a melee reach off the grid", func(e *Effect) { e.Weapon, e.RangeFt, e.ReachFt = WeaponMelee, 0, 7 }, ".reach_ft"},
	{"a melee reach too long", func(e *Effect) { e.Weapon, e.RangeFt, e.ReachFt = WeaponMelee, 0, MaxWeaponReachFt+5 }, ".reach_ft"},
	{"tags", func(e *Effect) { e.Tags = []string{"about:teste"} }, ".tags"},
	{"a state of nobody", func(e *Effect) { e.While = "fantasma" }, ".while"},
	{"a field of another type", func(e *Effect) { e.Target = "ac" }, ".target"},
	{"a field of a weapon rule", func(e *Effect) { e.AttackBonus = 1 }, ".attack_bonus"},
}

func refusedAt(t *testing.T, e Effect, field string) {
	t.Helper()
	o := stateOverlay([]Effect{e})
	o.Strict = []string{stateSubclass}
	_, err := loadForTest(t).With(o)
	var oe *OverlayError
	if !errors.As(err, &oe) {
		t.Fatalf("want an OverlayError, got %v", err)
	}
	if !slices.ContainsFunc(oe.Violations(), func(v *OverlayError) bool { return strings.HasSuffix(v.Field, "effects[0]"+field) }) {
		t.Errorf("violations = %+v, want one at effects[0]%s", oe.Violations(), field)
	}
}

func TestWeaponRuleRefusalsNameTheirField(t *testing.T) {
	t.Parallel()
	for _, tc := range weaponRuleRefusals {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			e := Effect{Type: "weapon_rule", Weapon: WeaponMelee, AttackAbility: "int", AttackBonus: 1, ReachFt: 10}
			tc.edit(&e)
			refusedAt(t, e, tc.field)
		})
	}
}

func TestPseudoWeaponRefusalsNameTheirField(t *testing.T) {
	t.Parallel()
	for _, tc := range pseudoWeaponRefusals {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			e := Effect{Type: "pseudo_weapon", AttackName: "Raio de Teste", Weapon: WeaponRanged, DamageDice: "1d6", DamageType: "damage-type:force", Ability: "int", RangeFt: 60}
			tc.edit(&e)
			refusedAt(t, e, tc.field)
		})
	}
}

func TestWeaponRulesAreAcceptedAtTheirLimits(t *testing.T) {
	for _, e := range []Effect{
		{Type: "weapon_rule", Weapon: WeaponAny, AttackBonus: MaxWeaponBonus, DamageBonus: -MaxWeaponPenalty},
		{Type: "weapon_rule", Weapon: WeaponMelee, ReachFt: MaxWeaponReachFt},
		{Type: "weapon_rule", Weapon: WeaponNamed, WeaponName: strings.Repeat("a", 60), DamageAbility: "wis"},
		{Type: "pseudo_weapon", AttackName: strings.Repeat("a", 40), Weapon: WeaponMelee, DamageDice: "10d12", DamageType: "damage-type:acid", Ability: "str", ReachFt: MaxWeaponReachFt},
		{Type: "pseudo_weapon", AttackName: "Raio", Weapon: WeaponRanged, DamageDice: "1d4", DamageType: "damage-type:cold", Ability: "dex", RangeFt: MaxWeaponRangeFt},
		{Type: "pseudo_weapon", AttackName: "Toque", Weapon: WeaponMelee, DamageDice: "1d4", DamageType: "damage-type:necrotic", Ability: "wis"},
	} {
		o := stateOverlay([]Effect{e})
		o.Strict = []string{stateSubclass}
		if _, err := loadForTest(t).With(o); err != nil {
			t.Errorf("%+v is refused: %v", e, err)
		}
	}
}

func TestWeaponEffectsAreOnTheEffectMenu(t *testing.T) {
	menu := loadForTest(t).EffectMenu()
	for typ, required := range map[string][]string{
		"weapon_rule":   {"weapon"},
		"pseudo_weapon": {"attack_name", "weapon", "damage_dice", "damage_type", "ability"},
	} {
		i := slices.IndexFunc(menu.Types, func(m MenuType) bool { return m.Type == typ })
		if i < 0 {
			t.Fatalf("no %s type on the menu", typ)
		}
		for _, name := range required {
			if !slices.ContainsFunc(menu.Types[i].Fields, func(f MenuField) bool { return f.Name == name && f.Required }) {
				t.Errorf("%s: field %s is missing or not required", typ, name)
			}
		}
	}
	if got := valueKeys(menuList(t, menu, ListWeaponFilters)); !slices.Equal(got, []string{WeaponAny, WeaponMelee, WeaponRanged, WeaponFinesse, WeaponMagic, WeaponNamed}) {
		t.Errorf("weapon filters = %v", got)
	}
	if got := valueKeys(menuList(t, menu, ListWeaponKinds)); !slices.Equal(got, []string{WeaponMelee, WeaponRanged}) {
		t.Errorf("weapon kinds = %v", got)
	}
}

// A weapon rule's reach is the melee reach of the line, never the weapon's range: a dagger (thrown 20/60 ft)
// with a 30 ft reach still throws at 20/60.
func TestWeaponRuleReachNeverChangesTheThrowingRange(t *testing.T) {
	base := baseAttacks(t)[ruleDagger]
	if base.LongRangeFt != 60 || base.RangeFt != 20 {
		t.Fatalf("dagger = %+v, want it thrown 20/60", base)
	}
	for _, reach := range []int{30, 60} {
		got := ruleAttacks(t, ruleBuild(), Effect{Type: "weapon_rule", Weapon: WeaponAny, ReachFt: reach})[ruleDagger]
		if got.ReachFt != reach || got.RangeFt != 20 || got.LongRangeFt != 60 {
			t.Errorf("dagger with a %d ft reach = reach %d, range %d/%d, want the reach and 20/60", reach, got.ReachFt, got.RangeFt, got.LongRangeFt)
		}
	}
}
