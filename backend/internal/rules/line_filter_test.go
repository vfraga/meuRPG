package rules

import (
	"errors"
	"slices"
	"strings"
	"testing"
)

// The line filter of weapon_rule and extra_damage (TableEffect.line) and the sheet's pact weapon mark.
// Every name here is invented.

func launcherLine() Effect {
	return Effect{Type: "pseudo_weapon", AttackName: "Lançador de Teste", Weapon: WeaponRanged, DamageDice: "1d6", DamageType: "damage-type:force", Ability: "int", RangeFt: 60}
}

func clawLine() Effect {
	return Effect{Type: "natural_attack", AttackName: "Garra de Teste", DamageDice: "1d4", DamageType: "damage-type:slashing", Ability: "str"}
}

// boosted lists the lines a weapon_rule with the filter gave a +1 to the attack roll, over the same
// character without the rule.
func boosted(t *testing.T, b Build, rule Effect, lines ...Effect) []string {
	t.Helper()
	rule.AttackBonus = 1
	with := ruleAttacks(t, b, append(slices.Clone(lines), rule)...)
	without := ruleAttacks(t, b, lines...)
	var out []string
	for k, a := range with {
		if a.AttackBonus == without[k].AttackBonus+1 {
			out = append(out, k)
		}
	}
	slices.Sort(out)
	return out
}

func TestWeaponRuleLineChoosesOneAttackLine(t *testing.T) {
	t.Parallel()
	b := ruleBuild()
	b.WeaponMarks = []WeaponMark{{Weapon: ruleDagger, PactWeapon: true}}
	pseudo, natural := PseudoWeaponKey(stateFeature, 1), NaturalAttackKey(stateFeature, 1)
	for _, tc := range []struct {
		name, line string
		want       []string
	}{
		{"a pseudo_weapon by its name", "Lançador de Teste", []string{pseudo}},
		{"a pseudo_weapon by name, ignoring case and accents", " lancador DE teste ", []string{pseudo}},
		{"a pseudo_weapon by its line key", pseudo, []string{pseudo}},
		{"a natural_attack by its name", "Garra de Teste", []string{natural}},
		{"a natural_attack by its line key", natural, []string{natural}},
		{"an SRD weapon by key", ruleRapier, []string{ruleRapier}},
		{"the unarmed strike by key", UnarmedStrikeKey, []string{UnarmedStrikeKey}},
		{"the pact weapon", LinePact, []string{ruleDagger}},
	} {
		got := boosted(t, b, Effect{Type: "weapon_rule", Weapon: WeaponAny, Line: tc.line}, launcherLine(), clawLine())
		if !slices.Equal(got, tc.want) {
			t.Errorf("%s: lines changed = %v, want %v", tc.name, got, tc.want)
		}
	}
}

func TestWeaponRuleLineAndFilterBothHold(t *testing.T) {
	t.Parallel()
	got := boosted(t, ruleBuild(), Effect{Type: "weapon_rule", Weapon: WeaponRanged, Line: ruleRapier})
	if len(got) != 0 {
		t.Errorf("a ranged filter on the rapier changed %v, want nothing", got)
	}
	got = boosted(t, ruleBuild(), Effect{Type: "weapon_rule", Weapon: WeaponMelee, Line: ruleRapier})
	if !slices.Equal(got, []string{ruleRapier}) {
		t.Errorf("a melee filter on the rapier changed %v, want the rapier", got)
	}
}

func TestWeaponRuleLineChoosesATableWeaponByKey(t *testing.T) {
	t.Parallel()
	derive := func(rule Effect) Derived {
		o := stateOverlay([]Effect{rule})
		o.Weapons = ruleWeapons()
		b := ruleBuild()
		b.Weapons = []string{wFinesse, wLight}
		return Derive(b, withOverlay(t, o))
	}
	base := derive(Effect{Type: "note", TextPT: "Sem regra."})
	d := derive(Effect{Type: "weapon_rule", Weapon: WeaponAny, Line: wFinesse, DamageBonus: 2})
	if got, want := tableLine(t, d, wFinesse).DamageDice.Bonus, tableLine(t, base, wFinesse).DamageDice.Bonus+2; got != want {
		t.Errorf("the named table weapon's damage bonus = %d, want %d", got, want)
	}
	if got, want := tableLine(t, d, wLight).DamageDice.Bonus, tableLine(t, base, wLight).DamageDice.Bonus; got != want {
		t.Errorf("the other table weapon's damage bonus = %d, want %d (unchanged)", got, want)
	}
}

func TestPactMarkIsTheSheetsAndIsRead(t *testing.T) {
	t.Parallel()
	b := ruleBuild()
	b.WeaponMarks = []WeaponMark{{Weapon: ruleRapier, PactWeapon: true}}
	a := ruleAttacks(t, b, Effect{Type: "note", TextPT: "Sem regra."})
	for k, line := range a {
		if line.Pact != (k == ruleRapier) {
			t.Errorf("%s: Pact = %v", k, line.Pact)
		}
	}
	// With no pact mark no line is the pact weapon, and a rule on "pact" changes nothing.
	if got := boosted(t, ruleBuild(), Effect{Type: "weapon_rule", Weapon: WeaponAny, Line: LinePact}); len(got) != 0 {
		t.Errorf("no pact mark, yet %v changed", got)
	}
	// A pact mark alone is a valid mark and makes the line neither magical nor silvered.
	if got := a[ruleRapier]; got.Magical || got.Silvered || got.Adamantine {
		t.Errorf("the pact mark marked the material: %+v", got)
	}
}

func TestPactMarkAlsoMarksTheMagicFilterOnlyWhenMagical(t *testing.T) {
	t.Parallel()
	b := ruleBuild()
	b.WeaponMarks = []WeaponMark{{Weapon: ruleRapier, PactWeapon: true}, {Weapon: ruleDagger, Magical: true}}
	got := boosted(t, b, Effect{Type: "weapon_rule", Weapon: WeaponMagic, Line: LinePact})
	if len(got) != 0 {
		t.Errorf("magic filter with the pact line changed %v, want nothing: the pact weapon is not magical", got)
	}
}

// refusedWith is refusedAt that also checks the reason code of the refusal at the field.
func refusedWith(t *testing.T, o Overlay, field, reason string) {
	t.Helper()
	o.Strict = []string{stateSubclass}
	_, err := loadForTest(t).With(o)
	var oe *OverlayError
	if !errors.As(err, &oe) {
		t.Fatalf("want an OverlayError, got %v", err)
	}
	if !slices.ContainsFunc(oe.Violations(), func(v *OverlayError) bool { return strings.HasSuffix(v.Field, field) && v.Reason == reason }) {
		t.Errorf("violations = %+v, want one at %s with reason %s", oe.Violations(), field, reason)
	}
}

func TestLineRefusalsNameTheField(t *testing.T) {
	t.Parallel()
	for _, tc := range []struct {
		name, field, reason string
		e                   Effect
	}{
		{"a name no line has", "effects[0].line", ReasonReference, Effect{Type: "weapon_rule", Weapon: WeaponAny, AttackBonus: 1, Line: "Sem Linha"}},
		{"a key that is no weapon", "effects[0].line", ReasonReference, Effect{Type: "weapon_rule", Weapon: WeaponAny, AttackBonus: 1, Line: "equipment:backpack"}},
		{"a weapon the content lacks", "effects[0].line", ReasonReference, Effect{Type: "weapon_rule", Weapon: WeaponAny, AttackBonus: 1, Line: "equipment:espada-que-nao-existe"}},
		{"a line key of an effect the entry lacks", "effects[0].line", ReasonReference, Effect{Type: "weapon_rule", Weapon: WeaponAny, AttackBonus: 1, Line: PseudoWeaponKey(stateFeature, 3)}},
		{"extra_damage naming nothing", "effects[0].line", ReasonReference, Effect{Type: "extra_damage", Applies: ExtraDamageAnyTarget, Value: "1", Line: "Sem Linha"}},
		{"a line on a note", "effects[0].line", ReasonValue, Effect{Type: "note", TextPT: "x", Line: ruleRapier}},
		{"a line on a pseudo_weapon", "effects[0].line", ReasonValue, Effect{Type: "pseudo_weapon", AttackName: "Raio", Weapon: WeaponRanged, DamageDice: "1d6", DamageType: "damage-type:force", Ability: "int", RangeFt: 30, Line: ruleRapier}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			refusedWith(t, stateOverlay([]Effect{tc.e}), tc.field, tc.reason)
		})
	}
}

func TestLineNamingTwoLinesIsAmbiguous(t *testing.T) {
	t.Parallel()
	o := stateOverlay([]Effect{launcherLine(), launcherLine(), {Type: "weapon_rule", Weapon: WeaponAny, AttackBonus: 1, Line: "Lançador de Teste"}})
	refusedWith(t, o, "effects[2].line", ReasonValue)
}

func TestLineOfAnotherEntryIsRefused(t *testing.T) {
	t.Parallel()
	other := Overlay{Revision: 1, Subclasses: []TableSubclass{
		{Key: stateSubclass, NamePT: "Guardião de Teste", Class: "class:fighter", Levels: []TableSubclassLevel{{Level: 3, Features: []TableFeature{
			tf("postura-firme", "Postura Firme", Effect{Type: "weapon_rule", Weapon: WeaponAny, AttackBonus: 1, Line: "Lançador de Teste"}),
		}}}},
		{Key: "subclass:outro-de-teste" + tableSuffix, NamePT: "Outro de Teste", Class: "class:fighter", Levels: []TableSubclassLevel{{Level: 3, Features: []TableFeature{
			tf("passo-firme", "Passo Firme", launcherLine()),
		}}}},
	}}
	refusedWith(t, other, "effects[0].line", ReasonReference)
}

func TestExtraDamageLineIsDerivedAsTheResolvedLine(t *testing.T) {
	t.Parallel()
	c := withOverlay(t, stateOverlay([]Effect{launcherLine(), {Type: "extra_damage", Applies: ExtraDamageAnyTarget, Dice: "1", DiceSides: 6, DamageType: "damage-type:lightning", Line: "Lançador de Teste"}}))
	d := Derive(ruleBuild(), c)
	if len(d.ExtraDamages) != 1 || d.ExtraDamages[0].Line != PseudoWeaponKey(stateFeature, 1) {
		t.Fatalf("extra damages = %+v, want one on the launcher's line key", d.ExtraDamages)
	}
	c = withOverlay(t, stateOverlay([]Effect{{Type: "extra_damage", Applies: ExtraDamageAnyTarget, Value: "2", Line: LinePact}}))
	if d = Derive(ruleBuild(), c); len(d.ExtraDamages) != 1 || d.ExtraDamages[0].Line != LinePact {
		t.Errorf("extra damages = %+v, want one on the pact line", d.ExtraDamages)
	}
}

func TestLineMatches(t *testing.T) {
	t.Parallel()
	for _, tc := range []struct {
		line, key string
		pact      bool
		want      bool
	}{
		{"", "equipment:dagger", false, true},
		{LinePact, "equipment:dagger", true, true},
		{LinePact, "equipment:dagger", false, false},
		{"equipment:dagger", "equipment:dagger", false, true},
		{"equipment:dagger", "equipment:rapier", true, false},
	} {
		if got := LineMatches(tc.line, tc.key, tc.pact); got != tc.want {
			t.Errorf("LineMatches(%q, %q, %v) = %v, want %v", tc.line, tc.key, tc.pact, got, tc.want)
		}
	}
}

func TestLineIsOnTheEffectMenu(t *testing.T) {
	t.Parallel()
	menu := loadForTest(t).EffectMenu()
	for _, typ := range []string{"weapon_rule", "extra_damage"} {
		i := slices.IndexFunc(menu.Types, func(m MenuType) bool { return m.Type == typ })
		if i < 0 {
			t.Fatalf("no %s type on the menu", typ)
		}
		j := slices.IndexFunc(menu.Types[i].Fields, func(f MenuField) bool { return f.Name == "line" })
		if j < 0 {
			t.Fatalf("%s has no line field", typ)
		}
		if f := menu.Types[i].Fields[j]; f.Required || f.Kind != "text" {
			t.Errorf("%s line field = %+v, want an optional text field", typ, f)
		}
		if !strings.Contains(menu.Types[i].HintPT, "linha de ataque") {
			t.Errorf("%s hint does not explain the line: %q", typ, menu.Types[i].HintPT)
		}
	}
}
