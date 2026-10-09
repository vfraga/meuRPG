package rules

import (
	"encoding/json"
	"io/fs"
	"slices"
	"strings"
	"testing"
	"testing/fstest"

	"github.com/PuraFome/meuRPG/backend/internal/rules/srd51"
)

// fighter5 is a human Fighter 5: STR 16, DEX 15, CON 14 with the human +1.
func fighter5(items ...Item) Build {
	b := standard("class:fighter", 5)
	b.Items = items
	return b
}

func derived(t *testing.T, b Build) Derived {
	t.Helper()
	return Derive(b, loadForTest(t))
}

func worn(id, key string) Item { return Item{ID: id, Key: key, Quantity: 1, Equipped: true} }

func attuned(it Item) Item { it.Attuned = true; return it }

func withBase(it Item, base string) Item { it.Base = base; return it }

func TestItemsLeaveTheSheetAloneUntilEquipped(t *testing.T) {
	t.Parallel()
	plain := derived(t, fighter5())
	carried := derived(t, fighter5(
		Item{ID: "a", Key: "item:ring-of-protection", Quantity: 1, Attuned: true},
		Item{ID: "b", Key: "equipment:chain-mail", Quantity: 1},
		Item{ID: "c", Key: "item:weapon-3", Base: "equipment:longsword", Quantity: 1},
		Item{ID: "d", Name: "Corda", Quantity: 1},
	))
	if plain.ArmorClass != carried.ArmorClass || len(plain.Attacks) != len(carried.Attacks) || plain.SavingThrows[0].Bonus != carried.SavingThrows[0].Bonus {
		t.Fatalf("carried items changed the sheet: AC %d/%d, attacks %d/%d", plain.ArmorClass, carried.ArmorClass, len(plain.Attacks), len(carried.Attacks))
	}
}

func TestEquippedArmorAndShieldSetTheArmorClass(t *testing.T) {
	t.Parallel()
	for _, tc := range []struct {
		name  string
		items []Item
		want  int
		desc  string
	}{
		{"no armor: 10 + DEX", nil, 12, "Sem armadura"},
		{"chain mail", []Item{worn("a", "equipment:chain-mail")}, 16, "Cota de malha"},
		{"chain mail and shield", []Item{worn("a", "equipment:chain-mail"), worn("b", "equipment:shield")}, 18, "Cota de malha + escudo"},
		{"armor +1 on chain mail", []Item{withBase(worn("a", "item:armor-1"), "equipment:chain-mail")}, 17, "Cota de malha +1"},
		{"armor +3 on leather: base 11 + DEX 2 + 3", []Item{withBase(worn("a", "item:armor-3"), "equipment:leather-armor")}, 16, "Armadura de couro +3"},
		{"dwarven plate: plate 18 + 2", []Item{worn("a", "item:dwarven-plate")}, 20, ""},
		{"elven chain: chain shirt 13 + DEX 2 + 1", []Item{worn("a", "item:elven-chain")}, 16, ""},
		{"mithral on half plate: 15 + DEX 2", []Item{withBase(worn("a", "item:mithral-armor"), "equipment:half-plate-armor")}, 17, ""},
		{"ring of protection, attuned", []Item{attuned(worn("a", "item:ring-of-protection"))}, 13, "Sem armadura"},
		{"ring of protection, not attuned", []Item{worn("a", "item:ring-of-protection")}, 12, "Sem armadura"},
		{"ring of protection, attuned but not worn", []Item{{ID: "a", Key: "item:ring-of-protection", Attuned: true, Quantity: 1}}, 12, "Sem armadura"},
		{"cloak and ring of protection", []Item{attuned(worn("a", "item:cloak-of-protection")), attuned(worn("b", "item:ring-of-protection"))}, 14, ""},
		{"bracers of defense with no armor", []Item{attuned(worn("a", "item:bracers-of-defense"))}, 14, ""},
		{"bracers of defense with a shield do nothing", []Item{attuned(worn("a", "item:bracers-of-defense")), worn("b", "equipment:shield")}, 14, ""},
		{"bracers of defense with armor do nothing", []Item{attuned(worn("a", "item:bracers-of-defense")), worn("b", "equipment:chain-mail")}, 16, ""},
		{"a magic shield works as a plain shield until attuned", []Item{worn("a", "item:spellguard-shield")}, 14, ""},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			d := derived(t, fighter5(tc.items...))
			if d.ArmorClass != tc.want {
				t.Errorf("AC = %d, want %d (%s)", d.ArmorClass, tc.want, d.ArmorClassDescription)
			}
			if tc.desc != "" && !strings.HasPrefix(d.ArmorClassDescription, tc.desc) {
				t.Errorf("description = %q, want it to start with %q", d.ArmorClassDescription, tc.desc)
			}
		})
	}
}

func TestSheetArmorYieldsToTheWornArmorItem(t *testing.T) {
	t.Parallel()
	b := fighter5(withBase(worn("a", "item:armor-1"), "equipment:plate-armor"))
	b.Armor, b.Shield = "equipment:leather-armor", true
	d := derived(t, b)
	if d.ArmorClass != 18+1+2 {
		t.Fatalf("AC = %d, want plate 18 + 1 + the sheet's shield 2 (%s)", d.ArmorClass, d.ArmorClassDescription)
	}
	// With nothing equipped the sheet's own armor stays what it was.
	if got := derived(t, fighter5(Item{ID: "a", Key: "equipment:plate-armor", Quantity: 1})); got.ArmorClass != 12 {
		t.Fatalf("a carried plate changed the AC to %d", got.ArmorClass)
	}
	b.Items = nil
	if d := derived(t, b); d.ArmorClass != 11+2+2 {
		t.Fatalf("the sheet's armor and shield = %d, want 15", d.ArmorClass)
	}
}

func TestTwoSuitsOfArmorWornAreAnIssue(t *testing.T) {
	t.Parallel()
	d := derived(t, fighter5(worn("a", "equipment:chain-mail"), worn("b", "equipment:plate-armor")))
	if d.ArmorClass != 16 || !slices.ContainsFunc(d.Issues, func(i Issue) bool { return i.Code == IssueInventory }) {
		t.Fatalf("AC = %d, issues %v: the first suit should count and the second be an issue", d.ArmorClass, d.Issues)
	}
}

func TestMagicArmorBonusNeedsAttunementOnlyWhenTheItemAsksForIt(t *testing.T) {
	t.Parallel()
	// Demon armor requires attunement: unattuned it is plain plate (SRD 5.1 "Attunement").
	plain := derived(t, fighter5(worn("a", "item:demon-armor")))
	if plain.ArmorClass != 18 {
		t.Errorf("unattuned demon armor: AC = %d, want 18", plain.ArmorClass)
	}
	if got := derived(t, fighter5(attuned(worn("a", "item:demon-armor")))); got.ArmorClass != 19 {
		t.Errorf("attuned demon armor: AC = %d, want 19", got.ArmorClass)
	}
}

func TestAbilityScoresFromItems(t *testing.T) {
	t.Parallel()
	strOf := func(d Derived) AbilityScore { return abilityOf(d, STR) }
	for _, tc := range []struct {
		name string
		item Item
		base map[Ability]int
		want int
	}{
		{"gauntlets lift 16 to 19", attuned(worn("a", "item:gauntlets-of-ogre-power")), nil, 19},
		{"gauntlets leave 20 alone", attuned(worn("a", "item:gauntlets-of-ogre-power")), map[Ability]int{STR: 19}, 20},
		{"gauntlets not attuned", worn("a", "item:gauntlets-of-ogre-power"), nil, 16},
		{"hill giant belt: 21", attuned(worn("a", "item:belt-of-giant-strength-hill")), nil, 21},
		{"storm giant belt: 29", attuned(worn("a", "item:belt-of-giant-strength-storm")), nil, 29},
		{"a belt below the score does nothing", attuned(worn("a", "item:belt-of-giant-strength-hill")), map[Ability]int{STR: 25}, 26},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			b := fighter5(tc.item)
			for a, v := range tc.base {
				b.BaseScores[a] = v
			}
			d := derived(t, b)
			if got := strOf(d).Score; got != tc.want {
				t.Errorf("STR = %d, want %d", got, tc.want)
			}
			if tc.want <= 29 && slices.Contains(issueCodes(d), IssueScoreAbove20) && tc.base == nil {
				t.Errorf("an item's score raised the score-above-20 issue: %v", issueCodes(d))
			}
		})
	}
	t.Run("amulet of health and headband", func(t *testing.T) {
		t.Parallel()
		d := derived(t, fighter5(attuned(worn("a", "item:amulet-of-health")), attuned(worn("b", "item:headband-of-intellect"))))
		if con, in := abilityOf(d, CON).Score, abilityOf(d, INT).Score; con != 19 || in != 19 {
			t.Errorf("CON %d, INT %d, want 19 and 19", con, in)
		}
	})
	t.Run("belt of dwarvenkind: CON + 2, to 20", func(t *testing.T) {
		t.Parallel()
		d := derived(t, fighter5(attuned(worn("a", "item:belt-of-dwarvenkind"))))
		if got := abilityOf(d, CON).Score; got != 16 {
			t.Errorf("CON = %d, want 16", got)
		}
	})
}

func TestSavingThrowsFromItems(t *testing.T) {
	t.Parallel()
	base := derived(t, fighter5())
	d := derived(t, fighter5(attuned(worn("a", "item:cloak-of-protection")), attuned(worn("b", "item:ring-of-protection"))))
	for i, st := range d.SavingThrows {
		if st.Bonus != base.SavingThrows[i].Bonus+2 {
			t.Errorf("%s save = %d, want %d + 2", st.Ability, st.Bonus, base.SavingThrows[i].Bonus)
		}
	}
}

func TestMagicWeaponsAreAttackLines(t *testing.T) {
	t.Parallel()
	base := derived(t, fighter5())
	longsword, _ := attackOf(derived(t, func() Build { b := fighter5(); b.Weapons = []string{"equipment:longsword"}; return b }()), "equipment:longsword")
	d := derived(t, fighter5(
		withBase(worn("sword", "item:weapon-2"), "equipment:longsword"),
		Item{ID: "spare", Key: "equipment:dagger", Quantity: 1}, // carried, not wielded
		worn("bow", "equipment:longbow"),
	))
	if len(d.Attacks) != len(base.Attacks)+2 {
		t.Fatalf("%d attacks, want %d: the sword and the bow join the unarmed strike", len(d.Attacks), len(base.Attacks)+2)
	}
	a, ok := attackOf(d, "inv:sword")
	if !ok {
		t.Fatalf("no attack for the wielded sword: %+v", d.Attacks)
	}
	if a.AttackBonus != longsword.AttackBonus+2 || a.NamePT != "Espada longa +2" || !a.Proficient {
		t.Errorf("attack = %+v, want the longsword's bonus %d + 2, named \"Espada longa +2\"", a, longsword.AttackBonus)
	}
	if want := withModifier("1d8", 3+2); a.Damage != want || a.VersatileDamage != withModifier("1d10", 3+2) {
		t.Errorf("damage = %q / %q, want %q and the versatile die with the same +5", a.Damage, a.VersatileDamage, want)
	}
	if _, found := attackOf(d, "inv:spare"); found {
		t.Error("a carried dagger became an attack")
	}
}

func TestMagicWeaponBonusNeedsAttunementWhenTheItemAsksForIt(t *testing.T) {
	t.Parallel()
	b := func(it Item) Build { return fighter5(it) }
	plain := derived(t, b(worn("a", "item:holy-avenger"))) // paladins only
	_ = plain
	for _, tc := range []struct {
		name string
		item Item
		want int // bonus over the plain longsword
	}{
		{"flame tongue-style: attuned but no bonus", attuned(withBase(worn("a", "item:flame-tongue"), "equipment:longsword")), 0},
		{"defender, not attuned: plain sword", withBase(worn("a", "item:defender"), "equipment:longsword"), 0},
		{"defender, attuned: +3", attuned(withBase(worn("a", "item:defender"), "equipment:longsword")), 3},
		{"holy avenger, attuned by a fighter: the restriction is not met", attuned(withBase(worn("a", "item:holy-avenger"), "equipment:longsword")), 0},
		{"dagger of venom needs no attunement: +1", worn("a", "item:dagger-of-venom"), 1},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			d := derived(t, b(tc.item))
			a, ok := attackOf(d, "inv:a")
			if !ok {
				t.Fatalf("no attack: %+v", d.Attacks)
			}
			baseKey := "equipment:longsword"
			if tc.item.Key == "item:dagger-of-venom" {
				baseKey = "equipment:dagger"
			}
			plainBuild := fighter5()
			plainBuild.Weapons = []string{baseKey}
			plain, _ := attackOf(derived(t, plainBuild), baseKey)
			if a.AttackBonus-plain.AttackBonus != tc.want {
				t.Errorf("bonus = %d over the plain weapon, want %d", a.AttackBonus-plain.AttackBonus, tc.want)
			}
		})
	}
}

func TestSunBladeChangesTheBaseWeapon(t *testing.T) {
	t.Parallel()
	b := fighter5(attuned(worn("sun", "item:sun-blade")))
	b.BaseScores[DEX] = 20 // 21 with the human +1: +5
	a, ok := attackOf(derived(t, b), "inv:sun")
	if !ok {
		t.Fatal("no attack for the sun blade")
	}
	if a.DamageType != "damage-type:radiant" || a.Ability != DEX {
		t.Errorf("sun blade: damage %s, ability %s; want radiant and DEX (finesse)", a.DamageType, a.Ability)
	}
	if a.AttackBonus != 5+3+2 { // DEX +5, proficiency +3, magic +2
		t.Errorf("attack bonus = %d, want 10", a.AttackBonus)
	}
}

func TestResistancesFromItems(t *testing.T) {
	t.Parallel()
	got := func(items ...Item) []string {
		var out []string
		for _, r := range derived(t, fighter5(items...)).ItemResistances {
			out = append(out, r.DamageType)
		}
		return out
	}
	if r := got(attuned(worn("a", "item:ring-of-resistance-fire"))); !slices.Equal(r, []string{"damage-type:fire"}) {
		t.Errorf("ring of fire resistance: %v", r)
	}
	if r := got(worn("a", "item:ring-of-resistance-fire")); len(r) != 0 {
		t.Errorf("an unattuned ring gave %v", r)
	}
	armor := attuned(withBase(worn("a", "item:armor-of-resistance"), "equipment:chain-mail"))
	armor.Option = "damage-type:cold"
	// Two suits of armor: only the first counts, so the red dragon's fire does not.
	if r := got(armor, attuned(worn("b", "item:dragon-scale-mail-red"))); len(r) != 1 || r[0] != "damage-type:cold" {
		t.Errorf("armor of resistance (cold) over dragon scale mail: %v", r)
	}
	if r := got(attuned(worn("a", "item:dragon-scale-mail-red"))); !slices.Equal(r, []string{"damage-type:fire"}) {
		t.Errorf("red dragon scale mail: %v", r)
	}
	// The same type from two items counts once; a third ring does not fit on the hands.
	if r := got(attuned(worn("a", "item:ring-of-resistance-cold")), attuned(worn("b", "item:ring-of-warmth")), attuned(worn("c", "item:brooch-of-shielding"))); len(r) != 2 {
		t.Errorf("resistances = %v, want cold once (ring and warmth) and force (brooch)", r)
	}
	if r := got(attuned(worn("a", "item:ring-of-resistance-cold")), attuned(worn("b", "item:ring-of-resistance-fire")), attuned(worn("c", "item:ring-of-resistance-acid"))); len(r) != 2 {
		t.Errorf("three rings: %v, want only the first two", r)
	}
}

func TestSensesAndSpeedFromItems(t *testing.T) {
	t.Parallel()
	senseRange := func(d Derived, key string) int {
		for _, s := range d.Senses {
			if s.Key == key {
				return s.RangeFt
			}
		}
		return 0
	}
	human := derived(t, fighter5(worn("a", "item:goggles-of-night")))
	if got := senseRange(human, "darkvision"); got != 60 {
		t.Errorf("goggles on a human: darkvision %d, want 60", got)
	}
	dwarf := fighter5(worn("a", "item:goggles-of-night"))
	dwarf.Race = "race:dwarf"
	dwarf.Subrace = "subrace:hill-dwarf"
	if got := senseRange(derived(t, dwarf), "darkvision"); got != 120 {
		t.Errorf("goggles on a dwarf: darkvision %d, want 60 + 60", got)
	}
	gnome := fighter5(attuned(worn("a", "item:boots-of-striding-and-springing")))
	gnome.Race, gnome.Subrace = "race:gnome", "subrace:rock-gnome"
	if got := derived(t, gnome).SpeedWalkFt; got != 30 {
		t.Errorf("boots of striding on a gnome: %d ft, want 30", got)
	}
	if got := derived(t, fighter5(attuned(worn("a", "item:boots-of-striding-and-springing")))).SpeedWalkFt; got != 30 {
		t.Errorf("boots of striding on a human: %d ft, want the 30 it has", got)
	}
}

func TestSpellBonusesAndHitPointsFromItems(t *testing.T) {
	t.Parallel()
	wizard := standard("class:wizard", 5)
	base := Derive(wizard, loadForTest(t))
	wizard.Items = []Item{attuned(worn("a", "item:wand-of-the-war-mage-2"))}
	d := derived(t, wizard)
	if d.Spellcasting[0].AttackBonus != base.Spellcasting[0].AttackBonus+2 || d.Spellcasting[0].SaveDC != base.Spellcasting[0].SaveDC {
		t.Errorf("war mage +2: attack %d/%d, DC %d/%d", d.Spellcasting[0].AttackBonus, base.Spellcasting[0].AttackBonus, d.Spellcasting[0].SaveDC, base.Spellcasting[0].SaveDC)
	}
	// A fighter does not meet "by a spellcaster": the wand does nothing.
	if f := derived(t, fighter5(attuned(worn("a", "item:wand-of-the-war-mage-2")))); len(f.Spellcasting) != 0 {
		t.Error("a fighter has spellcasting")
	}
	axe := attuned(withBase(worn("axe", "item:berserker-axe"), "equipment:greataxe"))
	hp := derived(t, fighter5(axe)).HitPointsMax - derived(t, fighter5()).HitPointsMax
	if hp != 5 {
		t.Errorf("berserker axe at level 5 added %d hit points, want 5", hp)
	}
}

func TestUnidentifiedItemsAreNamedByTheirLook(t *testing.T) {
	t.Parallel()
	sword := withBase(worn("sword", "item:weapon-1"), "equipment:longsword")
	sword.Unidentified, sword.Look = true, "Uma espada com runas"
	armor := withBase(worn("armor", "item:armor-1"), "equipment:chain-mail")
	armor.Unidentified, armor.Look = true, "Uma armadura com runas"
	d := derived(t, fighter5(sword, armor))
	a, ok := attackOf(d, "inv:sword")
	if !ok || a.NamePT != "Uma espada com runas" {
		t.Fatalf("attack name = %q, want the look", a.NamePT)
	}
	if !strings.HasPrefix(d.ArmorClassDescription, "Uma armadura com runas") || strings.Contains(strings.ToLower(d.ArmorClassDescription), "cota") {
		t.Errorf("AC description %q names the armor", d.ArmorClassDescription)
	}
	for _, h := range d.Hints {
		if strings.Contains(h.TextPT, "Espada longa") || strings.Contains(h.TextPT, "+1") && strings.Contains(h.TextPT, "Armadura") {
			t.Errorf("a hint names the unidentified item: %q", h.TextPT)
		}
	}
	// Without a look, a generic sentence stands in; the bonus still counts.
	sword.Look = ""
	if a, _ := attackOf(derived(t, fighter5(sword)), "inv:sword"); a.NamePT != unidentifiedLook {
		t.Errorf("attack name = %q, want the default look", a.NamePT)
	}
}

func TestMagicAmmunitionIsSpentFirstWhenQuivered(t *testing.T) {
	t.Parallel()
	bow := func(items ...Item) Attack {
		b := fighter5(items...)
		b.Weapons = []string{"equipment:longbow"}
		a, _ := attackOf(derived(t, b), "equipment:longbow")
		return a
	}
	plain := Item{ID: "plain", Key: "equipment:arrow", Quantity: 20}
	magic := Item{ID: "magic", Key: "item:ammunition-2", Base: "equipment:arrow", Quantity: 3}
	none := bow()
	if none.Ammunition != "equipment:arrow" || none.AmmunitionItem != "" || none.AmmunitionOut {
		t.Errorf("no counted arrows: %+v, want an unlimited bow", none)
	}
	if a := bow(plain, magic); a.AmmunitionItem != "plain" || a.AttackBonus != none.AttackBonus {
		t.Errorf("both carried: %+v, want the plain arrows", a)
	}
	quivered := magic
	quivered.Equipped = true
	if a := bow(plain, quivered); a.AmmunitionItem != "magic" || a.AttackBonus != none.AttackBonus+2 || a.Damage == none.Damage {
		t.Errorf("magic arrows quivered: %+v, want them fired with +2", a)
	}
	empty := plain
	empty.Quantity = 0
	if a := bow(empty); !a.AmmunitionOut || a.AmmunitionItem != "" {
		t.Errorf("an empty quiver: %+v, want the bow out of ammunition", a)
	}
	if a := bow(empty, magic); a.AmmunitionItem != "magic" {
		t.Errorf("empty plain stack and magic arrows left: %+v", a)
	}
	// A melee weapon and a thrown dagger never ask for ammunition.
	b := fighter5(plain)
	b.Weapons = []string{"equipment:dagger", "equipment:longsword"}
	for _, a := range derived(t, b).Attacks {
		if a.Ammunition != "" {
			t.Errorf("%s asks for %s", a.Key, a.Ammunition)
		}
	}
}

func TestParseRestriction(t *testing.T) {
	t.Parallel()
	c := Character{Classes: []string{"cleric"}, Race: "dwarf", Spellcaster: true, Alignment: AlignmentGood}
	for _, tc := range []struct {
		by   string
		who  Character
		want bool
	}{
		{"", Character{}, true},
		{"by a spellcaster", c, true},
		{"by a spellcaster", Character{Classes: []string{"fighter"}}, false},
		{"by a paladin", Character{Classes: []string{"paladin"}}, true},
		{"by a paladin", c, false},
		{"by a cleric, druid, or paladin", c, true},
		{"by a bard, cleric, druid, sorcerer, warlock, or wizard", Character{Classes: []string{"druid"}}, true},
		{"by a sorcerer, warlock, or wizard", c, false},
		{"by a dwarf", c, true},
		{"by a dwarf", Character{Race: "elf"}, false},
		{"by a creature of good alignment", c, true},
		{"by a creature of good alignment", Character{Alignment: AlignmentEvil}, false},
		{"by a creature of evil alignment", Character{Alignment: AlignmentEvil}, true},
		{"by a creature of evil alignment", Character{}, false},
		{"outdoors at night", Character{}, true}, // not testable: the master decides
	} {
		if got := ParseRestriction(tc.by).Met(tc.who); got != tc.want {
			t.Errorf("%q met by %+v = %v, want %v", tc.by, tc.who, got, tc.want)
		}
	}
	if !ParseRestriction("outdoors at night").Unchecked {
		t.Error("\"outdoors at night\" should be unchecked")
	}
}

func TestEveryRestrictionOfTheSRDIsRead(t *testing.T) {
	t.Parallel()
	seen := map[string]bool{}
	for _, m := range loadForTest(t).MagicItems() {
		if m.AttunementBy == "" || seen[m.AttunementBy] {
			continue
		}
		seen[m.AttunementBy] = true
		r := ParseRestriction(m.AttunementBy)
		if r.Unchecked && m.AttunementBy != "outdoors at night" {
			t.Errorf("the restriction %q (%s) has a term the sheet cannot read", m.AttunementBy, m.Key)
		}
	}
	if len(seen) < 10 {
		t.Fatalf("only %d restrictions read", len(seen))
	}
}

func TestItemNames(t *testing.T) {
	t.Parallel()
	c := loadForTest(t)
	armor := Item{Key: "item:armor-of-resistance", Base: "equipment:plate-armor", Option: "damage-type:fire"}
	for _, tc := range []struct {
		it   Item
		want string
	}{
		{Item{Name: "Corda de cânhamo"}, "Corda de cânhamo"},
		{Item{Key: "equipment:chain-mail"}, "Cota de malha"},
		{Item{Key: "item:weapon-3", Base: "equipment:rapier"}, "Rapieira +3"},
		{Item{Key: "item:ammunition-1", Base: "equipment:arrow"}, "Flecha +1"},
		{Item{Key: "item:ring-of-protection"}, c.c.namePT("item:ring-of-protection")},
		{Item{Key: "item:ring-of-protection", Unidentified: true, Look: "Um anel liso"}, "Um anel liso"},
		{Item{Key: "equipment:chain-mail", Unidentified: true, Look: "não vale para o mundano"}, "Cota de malha"},
	} {
		if got := c.ItemName(tc.it); got != tc.want {
			t.Errorf("ItemName(%+v) = %q, want %q", tc.it, got, tc.want)
		}
	}
	if got := c.ItemName(armor); !strings.Contains(got, "fogo") {
		t.Errorf("ItemName(armor of resistance) = %q, want the damage type in it", got)
	}
}

// TestLoadItemsRefuses feeds effects/items.json bad edits, one at a time: each must
// be refused by the loader.
func TestLoadItemsRefuses(t *testing.T) {
	t.Parallel()
	base := loadForTest(t).c
	raw, err := fs.ReadFile(srd51.Files, "effects/items.json")
	if err != nil {
		t.Fatal(err)
	}
	load := func(body []byte) error {
		c := *base
		c.items = nil
		return c.loadItems(fstest.MapFS{"effects/items.json": {Data: body}})
	}
	if err := load(raw); err != nil {
		t.Fatalf("the real file: %v", err)
	}
	// edit decodes the file, lets fn change it and encodes it again.
	edit := func(fn func(doc, items map[string]any)) []byte {
		t.Helper()
		var doc map[string]any
		if err := json.Unmarshal(raw, &doc); err != nil {
			t.Fatal(err)
		}
		fn(doc, doc["items"].(map[string]any))
		out, err := json.Marshal(doc)
		if err != nil {
			t.Fatal(err)
		}
		return out
	}
	entry := func(items map[string]any, key string) map[string]any { return items[key].(map[string]any) }
	effect := func(items map[string]any, key string, i int) map[string]any {
		return entry(items, key)["effects"].([]any)[i].(map[string]any)
	}
	const ring, fireball, armor = "item:ring-of-protection", "item:wand-of-fireballs", "item:elven-chain"
	for name, body := range map[string][]byte{
		"an item that is not in the SRD":    edit(func(_, it map[string]any) { it["item:ring-of-nonsense"] = entry(it, ring) }),
		"a family":                          edit(func(_, it map[string]any) { it["item:ring-of-resistance"] = entry(it, ring) }),
		"an unknown field":                  edit(func(_, it map[string]any) { entry(it, ring)["cost"] = 1 }),
		"an unknown slot":                   edit(func(_, it map[string]any) { entry(it, ring)["slot"] = "finger" }),
		"an unknown effect type":            edit(func(_, it map[string]any) { effect(it, ring, 0)["type"] = "luck" }),
		"an effect with a field it ignores": edit(func(_, it map[string]any) { effect(it, ring, 0)["damage_type"] = "damage-type:fire" }),
		"a bonus above +3":                  edit(func(_, it map[string]any) { effect(it, ring, 0)["value"] = 4 }),
		"a damage type that does not exist": edit(func(_, it map[string]any) {
			it["item:ring-of-warmth"].(map[string]any)["effects"].([]any)[0].(map[string]any)["damage_type"] = "damage-type:cheese"
		}),
		"a base that is not equipment": edit(func(_, it map[string]any) {
			entry(it, "item:dagger-of-venom")["base"] = map[string]any{"fixed": "equipment:rope-hempen-50-feet"}
		}),
		"a base and a group": edit(func(_, it map[string]any) {
			entry(it, armor)["base"] = map[string]any{"fixed": "equipment:chain-shirt", "group": "sword"}
		}),
		"a group that is not": edit(func(_, it map[string]any) { entry(it, armor)["base"] = map[string]any{"group": "spear"} }),
		"a spell that is not": edit(func(_, it map[string]any) {
			entry(it, fireball)["charges"].(map[string]any)["spells"].([]any)[0].(map[string]any)["spell"] = "spell:fireworks"
		}),
		"charges of zero": edit(func(_, it map[string]any) { entry(it, fireball)["charges"].(map[string]any)["max"] = 0 }),
		"dice that do not parse": edit(func(_, it map[string]any) {
			entry(it, fireball)["charges"].(map[string]any)["regain_dice"] = "some"
		}),
		"a cost above the charges": edit(func(_, it map[string]any) {
			entry(it, fireball)["charges"].(map[string]any)["spells"].([]any)[0].(map[string]any)["cost"] = 8
		}),
		"a spell level below its own": edit(func(_, it map[string]any) {
			entry(it, fireball)["charges"].(map[string]any)["spells"].([]any)[0].(map[string]any)["level"] = 2
		}),
		"no source": edit(func(doc, _ map[string]any) { doc["source"] = " " }),
		"a weapon bonus without a weapon": edit(func(_, it map[string]any) {
			entry(it, ring)["effects"] = []any{map[string]any{"type": "weapon_bonus", "value": 1}}
		}),
		"an armor bonus on a weapon": edit(func(_, it map[string]any) {
			entry(it, "item:dagger-of-venom")["effects"] = []any{map[string]any{"type": "armor_bonus", "value": 1}}
		}),
		"an armor in the ring slot": edit(func(_, it map[string]any) { entry(it, armor)["slot"] = "ring" }),
		"a weapon in the body slot": edit(func(_, it map[string]any) { entry(it, "item:dagger-of-venom")["slot"] = "body" }),
		"a non-bow that fires arrows": edit(func(doc, _ map[string]any) {
			doc["ammunition"].(map[string]any)["equipment:dagger"] = "equipment:arrow"
		}),
		"a bow with no ammunition": edit(func(doc, _ map[string]any) { delete(doc["ammunition"].(map[string]any), "equipment:sling") }),
		"ammunition that is not": edit(func(doc, _ map[string]any) {
			doc["ammunition"].(map[string]any)["equipment:sling"] = "equipment:backpack"
		}),
		"a reserved group name":        edit(func(doc, _ map[string]any) { doc["groups"].(map[string]any)["@weapons"] = []any{"equipment:dagger"} }),
		"a group with a shield":        edit(func(doc, _ map[string]any) { doc["groups"].(map[string]any)["axe"] = []any{"equipment:shield"} }),
		"an ability set above 30":      edit(func(_, it map[string]any) { effect(it, "item:amulet-of-health", 0)["value"] = 31 }),
		"a set ability with a cap":     edit(func(_, it map[string]any) { effect(it, "item:amulet-of-health", 0)["cap"] = 19 }),
		"a speed that is not a step":   edit(func(_, it map[string]any) { effect(it, "item:boots-of-striding-and-springing", 0)["value"] = 33 }),
		"a sense that does not exist":  edit(func(_, it map[string]any) { effect(it, "item:goggles-of-night", 0)["sense"] = "x-ray" }),
		"a scroll above the 9th level": edit(func(_, it map[string]any) { entry(it, "item:spell-scroll-9th")["use"] = map[string]any{"scroll": 10} }),
		"a scroll that also heals": edit(func(_, it map[string]any) {
			entry(it, "item:spell-scroll-9th")["use"] = map[string]any{"scroll": 9, "heal": "1d4"}
		}),
		"a use that does nothing": edit(func(_, it map[string]any) { entry(it, "item:potion-of-heroism")["use"] = map[string]any{} }),
		"a note over the limit":   edit(func(_, it map[string]any) { entry(it, armor)["note_pt"] = strings.Repeat("x", 301) }),
		"an option with no types": edit(func(_, it map[string]any) {
			entry(it, "item:armor-of-resistance")["option"] = map[string]any{"damage_types": []any{}}
		}),
		"a resistance option without an option": edit(func(_, it map[string]any) {
			entry(it, armor)["effects"] = []any{map[string]any{"type": "option_resistance"}}
		}),
	} {
		if err := load(body); err == nil {
			t.Errorf("%s was accepted", name)
		}
	}
}

func TestItemCoverageOfTheSRD(t *testing.T) {
	t.Parallel()
	c := loadForTest(t)
	counts := map[ItemCoverage]int{}
	for _, m := range c.MagicItems() {
		counts[c.ItemCoverage(m.Key)]++
	}
	if counts[CoverageApplied] == 0 || counts[CoveragePartial] == 0 || counts[CoverageReminder] == 0 {
		t.Fatalf("coverage %v: every class should have items", counts)
	}
	if total := counts[CoverageApplied] + counts[CoveragePartial] + counts[CoverageReminder]; total != magicItemsTotal {
		t.Fatalf("%d items counted, want %d", total, magicItemsTotal)
	}
	t.Logf("of the %d SRD magic items: %d applied, %d partly applied, %d reminders", magicItemsTotal, counts[CoverageApplied], counts[CoveragePartial], counts[CoverageReminder])
	for _, key := range []string{"item:ring-of-protection", "item:gauntlets-of-ogre-power", "item:weapon-1", "item:potion-of-healing-greater"} {
		if got := c.ItemCoverage(key); got != CoverageApplied {
			t.Errorf("%s = %s, want applied", key, got)
		}
	}
	for _, key := range []string{"item:dwarven-plate", "item:wand-of-fireballs", "item:sun-blade"} {
		if got := c.ItemCoverage(key); got != CoveragePartial {
			t.Errorf("%s = %s, want partial", key, got)
		}
	}
	for _, key := range []string{"item:bag-of-holding", "item:flame-tongue", "item:armor-of-invulnerability"} {
		if got := c.ItemCoverage(key); got != CoverageReminder {
			t.Errorf("%s = %s, want a reminder (nothing a number can say)", key, got)
		}
	}
}

func TestChargeSpellCost(t *testing.T) {
	t.Parallel()
	c := loadForTest(t)
	d, ok := c.ItemDef("item:wand-of-fireballs")
	if !ok || d.Charges == nil || d.Charges.Max != 7 {
		t.Fatalf("wand of fireballs = %+v", d)
	}
	s := d.Charges.Spells[0]
	for level, want := range map[int]int{3: 1, 4: 2, 5: 3, 9: 7} {
		if got, ok := s.CostAt(level); !ok || got != want {
			t.Errorf("fireball at level %d costs %d (%v), want %d", level, got, ok, want)
		}
	}
	for _, level := range []int{1, 2, 10} {
		if _, ok := s.CostAt(level); ok {
			t.Errorf("fireball at level %d should be refused", level)
		}
	}
	heal, _ := c.ItemDef("item:staff-of-healing")
	cure := heal.Charges.Spells[0]
	for level, want := range map[int]int{1: 1, 2: 2, 4: 4} {
		if got, ok := cure.CostAt(level); !ok || got != want {
			t.Errorf("cure wounds at level %d costs %d, want %d", level, got, want)
		}
	}
	if _, ok := cure.CostAt(5); ok {
		t.Error("cure wounds from the staff of healing goes up to the 4th level")
	}
	if _, ok := d.Charges.Spells[0].CostAt(3); !ok {
		t.Error("the base level must be castable")
	}
}

func TestScrollNumbersAreTheSRDTable(t *testing.T) {
	t.Parallel()
	c := loadForTest(t)
	for level, want := range []struct{ dc, attack int }{{13, 5}, {13, 5}, {13, 5}, {15, 7}, {15, 7}, {17, 9}, {17, 9}, {18, 10}, {18, 10}, {19, 11}} {
		key := "item:spell-scroll-cantrip"
		if level > 0 {
			key = "item:spell-scroll-" + []string{"", "1st", "2nd", "3rd", "4th", "5th", "6th", "7th", "8th", "9th"}[level]
		}
		d, ok := c.ItemDef(key)
		if !ok || d.Use == nil || !d.Use.Scroll || d.Use.ScrollLevel != level || d.Use.ScrollDC != want.dc || d.Use.ScrollAttack != want.attack {
			t.Errorf("%s = %+v, want level %d DC %d attack +%d", key, d.Use, level, want.dc, want.attack)
		}
	}
}

func TestPotionsOfHealing(t *testing.T) {
	t.Parallel()
	c := loadForTest(t)
	for key, want := range map[string]string{
		"item:potion-of-healing-common": "2d4+2", "item:potion-of-healing-greater": "4d4+4",
		"item:potion-of-healing-superior": "8d4+8", "item:potion-of-healing-supreme": "10d4+20",
	} {
		if d, ok := c.ItemDef(key); !ok || d.Use == nil || d.Use.HealDice != want {
			t.Errorf("%s heals %+v, want %s", key, d, want)
		}
	}
}
