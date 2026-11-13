package rules

import (
	"slices"
	"strings"
	"testing"
)

// The natural_attack effect (#572): an unarmed attack with a name of its own. The names are invented.
// ruleBuild has STR +0, DEX +1 and a proficiency bonus of +2.

func biteEffect(ability string) Effect {
	return Effect{Type: "natural_attack", AttackName: "Mordida de Teste", DamageDice: "1d6", DamageType: "damage-type:piercing", Ability: ability}
}

// naturalLine derives the build with the effects and returns the line named like the bite.
func naturalLine(t *testing.T, effects ...Effect) Attack {
	t.Helper()
	c := withOverlay(t, stateOverlay(effects))
	d := Derive(ruleBuild(), c)
	i := slices.IndexFunc(d.Attacks, func(a Attack) bool { return a.NamePT == "Mordida de Teste" })
	if i < 0 {
		t.Fatalf("no bite among %+v", d.Attacks)
	}
	return d.Attacks[i]
}

func TestANaturalAttackIsAMeleeLineWithTheAbilitiesNumbers(t *testing.T) {
	t.Parallel()
	for _, tc := range []struct {
		ability string
		want    Ability
		bonus   int
		damage  string
	}{
		{"str", STR, 0 + 2, "1d6"},
		{"dex", DEX, 1 + 2, "1d6+1"},
		{"best", DEX, 1 + 2, "1d6+1"},
	} {
		a := naturalLine(t, biteEffect(tc.ability))
		if a.Kind != "weapon" || !a.Melee || !a.Proficient || a.Magical || a.Ability != tc.want {
			t.Errorf("%s: line = %+v, want a proficient nonmagical melee weapon line with %s", tc.ability, a, tc.want)
		}
		if a.AttackBonus != tc.bonus || a.Damage != tc.damage || a.DamageType != "damage-type:piercing" || a.DamageTypeNamePT == "" {
			t.Errorf("%s: numbers = +%d, %q, %q (%q), want +%d, %q, piercing, named", tc.ability, a.AttackBonus, a.Damage, a.DamageType, a.DamageTypeNamePT, tc.bonus, tc.damage)
		}
		if a.RangeFt != 5 || a.ReachFt != 0 || !strings.HasPrefix(a.Key, "natural:") {
			t.Errorf("%s: range %d, reach %d, key %q, want the usual 5 ft and a natural: key", tc.ability, a.RangeFt, a.ReachFt, a.Key)
		}
	}
}

func TestABestNaturalAttackTakesStrengthWhenItIsHigher(t *testing.T) {
	t.Parallel()
	b := ruleBuild()
	b.BaseScores[STR] = 16
	c := withOverlay(t, stateOverlay([]Effect{biteEffect("best")}))
	for _, a := range Derive(b, c).Attacks {
		if a.NamePT == "Mordida de Teste" {
			if a.Ability != STR || a.AttackBonus != 3+2 || a.Damage != "1d6+3" {
				t.Errorf("line = %+v, want STR, +5, 1d6+3", a)
			}
			return
		}
	}
	t.Fatal("no bite")
}

func TestANaturalAttackHasItsOwnReachAndItsOwnKeys(t *testing.T) {
	t.Parallel()
	long := biteEffect("str")
	long.ReachFt = 10
	other := biteEffect("str")
	other.AttackName = "Garra de Teste"
	c := withOverlay(t, stateOverlay([]Effect{long, other}))
	d := Derive(ruleBuild(), c)
	keys := map[string]Attack{}
	for _, a := range d.Attacks {
		if strings.HasPrefix(a.Key, "natural:") {
			keys[a.Key] = a
		}
	}
	if len(keys) != 2 {
		t.Fatalf("natural keys = %v, want two different ones", keys)
	}
	if got := keys[NaturalAttackKey(stateFeature, 1)]; got.ReachFt != 10 || got.RangeFt != 10 {
		t.Errorf("first line = %+v, want a reach of 10 ft", got)
	}
	if got := keys[NaturalAttackKey(stateFeature, 2)]; got.NamePT != "Garra de Teste" || got.ReachFt != 0 {
		t.Errorf("second line = %+v, want the claw with the usual reach", got)
	}
	// It joins the list before the unarmed strike, which stays last.
	if last := d.Attacks[len(d.Attacks)-1]; last.Key != UnarmedStrikeKey {
		t.Errorf("the last line is %s, want the unarmed strike", last.Key)
	}
}

func TestANaturalAttackWhoseWhenIsFalseIsNotOnTheSheet(t *testing.T) {
	t.Parallel()
	e := biteEffect("str")
	e.When = "level() >= 5"
	c := withOverlay(t, stateOverlay([]Effect{e}))
	for _, a := range Derive(ruleBuild(), c).Attacks {
		if a.NamePT == "Mordida de Teste" {
			t.Fatalf("the bite is on the sheet at level 3: %+v", a)
		}
	}
	e.When = "level() >= 3"
	naturalLine(t, e)
}

func TestAMeleeWeaponRuleAndTheMeleeReachChangeANaturalAttack(t *testing.T) {
	t.Parallel()
	base := naturalLine(t, biteEffect("str"))
	ruled := naturalLine(t, biteEffect("str"), Effect{Type: "weapon_rule", Weapon: WeaponMelee, AttackBonus: 1, DamageBonus: 2})
	if ruled.AttackBonus != base.AttackBonus+1 || ruled.DamageDice.Bonus != base.DamageDice.Bonus+2 {
		t.Errorf("with a melee rule = +%d %+v, want +1 to hit and +2 to damage over +%d %+v", ruled.AttackBonus, ruled.DamageDice, base.AttackBonus, base.DamageDice)
	}
	ranged := naturalLine(t, biteEffect("str"), Effect{Type: "weapon_rule", Weapon: WeaponRanged, AttackBonus: 1})
	if ranged.AttackBonus != base.AttackBonus {
		t.Errorf("a ranged rule changed the bite: +%d", ranged.AttackBonus)
	}
	named := naturalLine(t, biteEffect("str"), Effect{Type: "weapon_rule", Weapon: WeaponNamed, WeaponName: "Mordida de Teste", AttackAbility: "dex"})
	if named.Ability != DEX {
		t.Errorf("a rule by name left the ability %s, want dex", named.Ability)
	}
	wide := naturalLine(t, biteEffect("str"), Effect{Type: "modifier", Target: ReachMeleeTarget, Mode: "set", Value: "15"})
	if wide.ReachFt != 15 || wide.RangeFt != 15 {
		t.Errorf("with reach.melee = %+v, want a reach of 15 ft", wide)
	}
}

func TestANaturalAttackIsNeverTheUnarmedStrikeLine(t *testing.T) {
	t.Parallel()
	a := naturalLine(t, biteEffect("str"))
	if a.Key == UnarmedStrikeKey || a.MartialArts || a.Light {
		t.Errorf("line = %+v: it is no unarmed strike, no martial art and no light weapon", a)
	}
}

var naturalAttackRefusals = []struct {
	name  string
	edit  func(e *Effect)
	field string
}{
	{"no name", func(e *Effect) { e.AttackName = "" }, ".attack_name"},
	{"a blank name", func(e *Effect) { e.AttackName = "  " }, ".attack_name"},
	{"a name over 40 characters", func(e *Effect) { e.AttackName = strings.Repeat("a", 41) }, ".attack_name"},
	{"no dice", func(e *Effect) { e.DamageDice = "" }, ".damage_dice"},
	{"a die that does not exist", func(e *Effect) { e.DamageDice = "1d7" }, ".damage_dice"},
	{"dice with a bonus", func(e *Effect) { e.DamageDice = "1d6+1" }, ".damage_dice"},
	{"too many dice", func(e *Effect) { e.DamageDice = "11d6" }, ".damage_dice"},
	{"no damage type", func(e *Effect) { e.DamageType = "" }, ".damage_type"},
	{"a key that is no damage type", func(e *Effect) { e.DamageType = "skill:arcana" }, ".damage_type"},
	{"no ability", func(e *Effect) { e.Ability = "" }, ".ability"},
	{"an ability that is not offered", func(e *Effect) { e.Ability = "int" }, ".ability"},
	{"an unknown ability", func(e *Effect) { e.Ability = "luck" }, ".ability"},
	{"a reach off the grid", func(e *Effect) { e.ReachFt = 7 }, ".reach_ft"},
	{"a reach too long", func(e *Effect) { e.ReachFt = MaxWeaponReachFt + 5 }, ".reach_ft"},
	{"a negative reach", func(e *Effect) { e.ReachFt = -5 }, ".reach_ft"},
	{"tags", func(e *Effect) { e.Tags = []string{"about:teste"} }, ".tags"},
	{"a weapon kind", func(e *Effect) { e.Weapon = WeaponMelee }, ".weapon"},
	{"a range", func(e *Effect) { e.RangeFt = 30 }, ".range_ft"},
	{"a state", func(e *Effect) { e.While = "postura" }, ".while"},
	{"a field of another type", func(e *Effect) { e.Target = "ac" }, ".target"},
	{"a field of a weapon rule", func(e *Effect) { e.AttackBonus = 1 }, ".attack_bonus"},
	{"a mark", func(e *Effect) { e.Silvered = true }, ".silvered"},
}

func TestNaturalAttackRefusalsNameTheirField(t *testing.T) {
	t.Parallel()
	for _, tc := range naturalAttackRefusals {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			e := biteEffect("best")
			tc.edit(&e)
			refusedAt(t, e, tc.field)
		})
	}
}

func TestNaturalAttackFieldsOnOtherEffectsAreRefused(t *testing.T) {
	t.Parallel()
	refusedAt(t, Effect{Type: "note", TextPT: "x", AttackName: "Mordida"}, ".attack_name")
	refusedAt(t, Effect{Type: "note", TextPT: "x", DamageDice: "1d6"}, ".damage_dice")
}

func TestTheNaturalAbilityListOffersStrengthDexterityAndTheBest(t *testing.T) {
	t.Parallel()
	m := loadForTest(t).EffectMenu()
	if got := valueKeys(menuList(t, m, ListNaturalAbilities)); !slices.Equal(got, []string{"str", "dex", "best"}) {
		t.Errorf("the list = %v, want str, dex, best", got)
	}
	for _, mt := range m.Types {
		if mt.Type == "natural_attack" && (mt.NamePT != "Ataque natural" || mt.HintPT == "") {
			t.Errorf("the type reads %q / %q", mt.NamePT, mt.HintPT)
		}
	}
}

// A gated-off natural attack still counts, so the line of the one after it keeps its key.
func TestANaturalAttackKeepsItsKeyWhileAnEarlierOneIsOff(t *testing.T) {
	t.Parallel()
	off := biteEffect("str")
	off.AttackName = "Garra de Teste"
	off.When = "level() >= 5"
	c := withOverlay(t, stateOverlay([]Effect{off, biteEffect("str")}))
	var keys []string
	for _, a := range Derive(ruleBuild(), c).Attacks {
		if strings.HasPrefix(a.Key, "natural:") {
			keys = append(keys, a.Key)
		}
	}
	if want := []string{NaturalAttackKey(stateFeature, 2)}; !slices.Equal(keys, want) || !strings.HasSuffix(keys[0], "#2") {
		t.Errorf("natural keys = %v, want %v", keys, want)
	}
}
