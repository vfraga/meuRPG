package rules

import (
	"errors"
	"slices"
	"testing"
	"time"
)

// The numbers of an infusion record (Build.Infusions): the typed data of the content, the class level and the sheet's
// picks give what a manual ItemBonus gives, and a record that is not valid now gives nothing. Every name is invented.

const (
	infSubclass = "subclass:mestre-de-ligas" + tableSuffix
	infHost     = "feature:gravar-pecas" + tableSuffix
	infGume     = "feature:gume-vivo" + tableSuffix
	infCouraca  = "feature:couraca-de-petalas" + tableSuffix
	infSelo     = "feature:selo-de-ferro" + tableSuffix
	infFoco     = "feature:diapasao-interno" + tableSuffix
	infCesto    = "feature:cesto-que-repoe" + tableSuffix
	infRetorno  = "feature:retorno-de-lamina" + tableSuffix
	infResiste  = "feature:manto-de-cinza" + tableSuffix
	infTexto    = "feature:marca-muda" + tableSuffix
	infPlain    = "feature:so-um-nome" + tableSuffix

	infSword = "equipment:longsword"
	infBow   = "equipment:longbow"
	infBow2  = "equipment:shortbow"
	infStaff = "equipment:quarterstaff"
	infAxe   = "equipment:handaxe"
	infArmor = "equipment:chain-mail"
)

// infusionRulesOverlay is a table subclass of the Wizard whose level 2 feature holds the infusions: eight with data, one
// without. It allows 2 live from level 2, 3 from 6 and 4 from 10.
func infusionRulesOverlay() Overlay {
	steps := []TableInfusionStep{{ClassLevel: 2, Bonus: 1}, {ClassLevel: 10, Bonus: 2}}
	opt := func(key, name string, in *TableInfusion) TableFeature {
		f := postureOption(key, name)
		f.Infusion = in
		return f
	}
	host := TableFeature{
		Key: infHost, NamePT: "Gravar Peças", DescPT: []string{"Escolha infusões."},
		Effects:      []Effect{{Type: "choice", Choice: "feature", Count: 8}},
		InfusionRule: &TableInfusionRule{LimitSteps: []TableInfusionLimitStep{{ClassLevel: 2, Active: 2}, {ClassLevel: 6, Active: 3}, {ClassLevel: 10, Active: 4}}},
		Options: []TableFeature{
			opt(infGume, "Gume Vivo", &TableInfusion{Accepts: InfusionAcceptsWeapon, Effect: InfusionEffectItemBonus, BonusSteps: steps}),
			opt(infCouraca, "Couraça de Pétalas", &TableInfusion{Accepts: InfusionAcceptsArmorOrShield, Effect: InfusionEffectItemBonus, BonusSteps: steps}),
			opt(infSelo, "Selo de Ferro", &TableInfusion{Accepts: InfusionAcceptsWeapon, Effect: InfusionEffectItemBonus, RequiresAttunement: true, BonusSteps: steps}),
			opt(infFoco, "Diapasão Interno", &TableInfusion{Accepts: InfusionAcceptsSpellcastingFocus, Effect: InfusionEffectSpellFocusBonus, RequiresAttunement: true, BonusSteps: steps}),
			opt(infCesto, "Cesto Que Repõe", &TableInfusion{Accepts: InfusionAcceptsAmmunitionWeapon, Effect: InfusionEffectRepeatingAmmunition}),
			opt(infRetorno, "Retorno de Lâmina", &TableInfusion{Accepts: InfusionAcceptsThrownWeapon, Effect: InfusionEffectReturning}),
			opt(infResiste, "Manto de Cinza", &TableInfusion{Accepts: InfusionAcceptsArmor, Effect: InfusionEffectResistanceLabel, DamageTypes: []string{"damage-type:fire", "damage-type:cold"}}),
			opt(infTexto, "Marca Muda", &TableInfusion{Accepts: InfusionAcceptsAnyObject, Effect: InfusionEffectNone}),
			opt(infPlain, "Só Um Nome", nil),
		},
	}
	return Overlay{Revision: 1, Subclasses: []TableSubclass{{
		Key: infSubclass, NamePT: "Mestre de Ligas", Class: "class:wizard",
		Levels: []TableSubclassLevel{{Level: 2, Features: []TableFeature{host}}},
	}}}
}

var infAll = []string{infGume, infCouraca, infSelo, infFoco, infCesto, infRetorno, infResiste, infTexto, infPlain}

// infBuild is a Wizard of the subclass at the level that picked the infusions.
func infBuild(level int, picks ...string) Build {
	b := standard("class:wizard", level)
	b.Classes[0].Subclass = infSubclass
	b.FeatureChoices = picks
	return b
}

func infRec(id, infusion, key string) InfusionRecord {
	return InfusionRecord{ID: id, InfusionKey: infusion, ItemKind: InfusionItemGear, ItemKey: key}
}

func infStatus(t *testing.T, d Derived, id string) InfusionStatus {
	t.Helper()
	for _, s := range d.Infusions {
		if s.ID == id {
			return s
		}
	}
	t.Fatalf("no status of the record %s in %+v", id, d.Infusions)
	return InfusionStatus{}
}

func infModifiers(d Derived, source string) []ItemModifier {
	var out []ItemModifier
	for _, m := range d.ItemModifiers {
		if m.SourceKey == source {
			out = append(out, m)
		}
	}
	return out
}

// plain strips the infusion fields of a modifier, to compare it with the one a manual bonus gives.
func plain(m ItemModifier) ItemModifier {
	m.InfusionNamePT, m.InfusionID = "", ""
	return m
}

func TestAnItemBonusInfusionGivesWhatAManualBonusGives(t *testing.T) {
	t.Parallel()
	c := withOverlay(t, infusionRulesOverlay())
	cases := []struct {
		name     string
		infusion string
		key      string
		mut      func(*Build)
	}{
		{"a weapon", infGume, infSword, func(b *Build) { b.Weapons = []string{infSword} }},
		{"the armor", infCouraca, infArmor, func(b *Build) { b.Armor = infArmor }},
		{"the shield", infCouraca, ShieldKey, func(b *Build) { b.Shield = true }},
	}
	for _, tc := range cases {
		manual := infBuild(5, infAll...)
		tc.mut(&manual)
		withRecord := manual
		manual.ItemBonuses = []ItemBonus{{Key: tc.key, Bonus: 1}}
		withRecord.Infusions = []InfusionRecord{infRec("r1", tc.infusion, tc.key)}

		want, got := Derive(manual, c), Derive(withRecord, c)
		if got.ArmorClass != want.ArmorClass {
			t.Errorf("%s: AC %d, want %d", tc.name, got.ArmorClass, want.ArmorClass)
		}
		if len(got.ItemModifiers) == 0 || len(got.ItemModifiers) != len(want.ItemModifiers) {
			t.Fatalf("%s: modifiers %+v, want the lines of %+v", tc.name, got.ItemModifiers, want.ItemModifiers)
		}
		for i, m := range got.ItemModifiers {
			if plain(m) != want.ItemModifiers[i] {
				t.Errorf("%s: modifier %d = %+v, want %+v", tc.name, i, m, want.ItemModifiers[i])
			}
			if m.InfusionID != "r1" || m.InfusionNamePT != "Gume Vivo" && m.InfusionNamePT != "Couraça de Pétalas" {
				t.Errorf("%s: modifier %d names no infusion: %+v", tc.name, i, m)
			}
		}
		if a, ok := attackOf(want, tc.key); ok {
			g, _ := attackOf(got, tc.key)
			if g.AttackBonus != a.AttackBonus || g.Damage != a.Damage || g.MagicBonus != a.MagicBonus || g.Magical != a.Magical {
				t.Errorf("%s: attack %+v, want the numbers of %+v", tc.name, g, a)
			}
			if g.InfusionNamePT != "Gume Vivo" {
				t.Errorf("%s: the attack names %q", tc.name, g.InfusionNamePT)
			}
		}
	}
}

func TestTheInfusionBonusGrowsAtLevel10(t *testing.T) {
	t.Parallel()
	c := withOverlay(t, infusionRulesOverlay())
	for _, tc := range []struct{ level, want int }{{2, 1}, {9, 1}, {10, 2}, {20, 2}} {
		b := infBuild(tc.level, infAll...)
		b.Weapons = []string{infSword}
		plainAttack, _ := attackOf(Derive(b, c), infSword)
		b.Infusions = []InfusionRecord{infRec("r1", infGume, infSword)}
		d := Derive(b, c)
		a, _ := attackOf(d, infSword)
		if got := a.AttackBonus - plainAttack.AttackBonus; got != tc.want {
			t.Errorf("level %d: the attack grows %d, want %d", tc.level, got, tc.want)
		}
		if s := infStatus(t, d, "r1"); s.Bonus != tc.want || !s.Counts || !s.Valid {
			t.Errorf("level %d: status %+v, want bonus %d", tc.level, s, tc.want)
		}
	}
}

func TestACarriedPieceOrAMissingAttunementGivesNoNumber(t *testing.T) {
	t.Parallel()
	c := withOverlay(t, infusionRulesOverlay())
	base := func() Build {
		b := infBuild(5, infAll...)
		b.Weapons = []string{infSword}
		b.Armor = infArmor
		return b
	}
	plainD := Derive(base(), c)
	sword, _ := attackOf(plainD, infSword)

	carried := base()
	carried.Carried = []string{infSword, infArmor}
	carried.Infusions = []InfusionRecord{infRec("w", infGume, infSword), infRec("a", infCouraca, infArmor)}
	d := Derive(carried, c)
	if d.ArmorClass != Derive(func() Build { b := base(); b.Carried = []string{infArmor}; return b }(), c).ArmorClass {
		t.Errorf("a carried armor adds %d AC", d.ArmorClass)
	}
	for _, m := range d.ItemModifiers {
		if m.Counts || m.ReasonPT != "Carregado, não equipado." || m.InfusionID == "" {
			t.Errorf("a carried piece's line = %+v", m)
		}
	}
	for _, id := range []string{"w", "a"} {
		if s := infStatus(t, d, id); !s.Valid || s.Counts || s.Bonus != 1 {
			t.Errorf("carried %s: status %+v, want valid, not counting, bonus 1", id, s)
		}
	}

	asked := base()
	asked.Infusions = []InfusionRecord{infRec("s", infSelo, infSword)}
	d = Derive(asked, c)
	if a, _ := attackOf(d, infSword); a.AttackBonus != sword.AttackBonus || a.Magical {
		t.Errorf("an infusion that asks attunement without it changed the attack: %+v", a)
	}
	for _, m := range infModifiers(d, infSword) {
		if m.Counts || m.ReasonPT != "Pede sintonização." {
			t.Errorf("unattuned line = %+v", m)
		}
	}
	asked.Infusions[0].Attuned = true
	if a, _ := attackOf(Derive(asked, c), infSword); a.AttackBonus != sword.AttackBonus+1 || !a.Magical {
		t.Errorf("an attuned one gives %+v, want +1", a)
	}
}

func TestTheBonusOfAKeyAppliesToEveryCopy(t *testing.T) {
	t.Parallel()
	c := withOverlay(t, infusionRulesOverlay())
	b := infBuild(5, infAll...)
	b.Weapons = []string{infAxe, infAxe}
	plainD := Derive(b, c)
	b.Infusions = []InfusionRecord{infRec("r1", infGume, infAxe)}
	d := Derive(b, c)
	var lines []Attack
	for _, a := range d.Attacks {
		if a.Key == infAxe {
			lines = append(lines, a)
		}
	}
	if len(lines) != 2 {
		t.Fatalf("%d axe lines, want 2", len(lines))
	}
	want, _ := attackOf(plainD, infAxe)
	for _, a := range lines {
		if a.AttackBonus != want.AttackBonus+1 || a.InfusionNamePT != "Gume Vivo" {
			t.Errorf("copy = %+v, want +1 and the name", a)
		}
	}
	gear := DeriveGear(b, c, d)
	if len(gear) != 1 || gear[0].Count != 2 || gear[0].InfusionNamePT != "Gume Vivo" {
		t.Errorf("gear = %+v, want one row of two copies with the infusion's name", gear)
	}
}

func infHostUsed(d Derived) int {
	n := 0
	for _, h := range d.InfusionHosts {
		n += h.Used
	}
	return n
}

// infObject is a record on an object that is no piece of the sheet.
func infObject(id, infusion, name string) InfusionRecord {
	return InfusionRecord{ID: id, InfusionKey: infusion, ItemKind: InfusionItemObject, ItemKey: "object:" + id, ObjectName: name}
}

func TestARecordThatIsNotValidNowGivesNothingAndTakesNoSlot(t *testing.T) {
	t.Parallel()
	o := infusionRulesOverlay()
	known := withOverlay(t, o)
	noRule := withOverlay(t, o)
	delete(noRule.c.infusions.rules, infHost)

	// r1 is a +1 weapon infusion on the sword; t1 is a text only one on a lantern.
	build := func(picks ...string) Build {
		b := infBuild(6, picks...)
		b.Weapons = []string{infSword}
		b.Infusions = []InfusionRecord{infRec("r1", infGume, infSword), infObject("t1", infTexto, "Lanterna")}
		return b
	}
	cases := []struct {
		name     string
		c        *Content
		b        Build
		wantOK   bool
		wantUsed int
	}{
		{"valid", known, build(infGume, infTexto), true, 2},
		{"an option the character does not know", known, build(infTexto), false, 1},
		{"a host without the rule", noRule, build(infGume, infTexto), false, 0},
		{"an infusion key the content lacks", known, func() Build {
			b := build(infGume, infTexto, "feature:fantasma"+tableSuffix)
			b.Infusions[0].InfusionKey = "feature:fantasma" + tableSuffix
			return b
		}(), false, 1},
		{"an option that is no infusion", known, func() Build {
			b := build(infGume, infTexto, infPlain)
			b.Infusions[0].InfusionKey = infPlain
			return b
		}(), false, 1},
		{"a class below the feature's level", known, func() Build {
			b := build(infGume, infTexto)
			b.Classes[0].Level = 1
			return b
		}(), false, 0},
		{"another subclass", known, func() Build {
			b := build(infGume, infTexto)
			b.Classes[0].Subclass = ""
			return b
		}(), false, 0},
	}
	for _, tc := range cases {
		d := Derive(tc.b, tc.c)
		s := infStatus(t, d, "r1")
		if s.Valid != tc.wantOK || (s.ReasonPT == "") != tc.wantOK {
			t.Errorf("%s: status %+v, want valid %v and a reason only when not", tc.name, s, tc.wantOK)
		}
		a, _ := attackOf(d, infSword)
		if tc.wantOK != (a.MagicBonus == 1) || tc.wantOK != (a.InfusionNamePT != "") || tc.wantOK != (len(d.ItemModifiers) == 2) || tc.wantOK != s.Counts || (!tc.wantOK && s.Bonus != 0) {
			t.Errorf("%s: valid %v but the numbers are %+v %+v %+v", tc.name, tc.wantOK, a, d.ItemModifiers, s)
		}
		if len(d.Infusions) != 2 {
			t.Errorf("%s: %d statuses, want one per record", tc.name, len(d.Infusions))
		}
		if got := infHostUsed(d); got != tc.wantUsed {
			t.Errorf("%s: used %d, want %d", tc.name, got, tc.wantUsed)
		}
		if gear := DeriveGear(tc.b, tc.c, d); tc.wantOK != (gear[0].InfusionNamePT != "") {
			t.Errorf("%s: the gear row names %q", tc.name, gear[0].InfusionNamePT)
		}
	}
}

func TestTheLimitFollowsTheClassLevelAndTheUsedCountIgnoresInvalidRecords(t *testing.T) {
	t.Parallel()
	c := withOverlay(t, infusionRulesOverlay())
	for _, tc := range []struct{ level, limit int }{{2, 2}, {5, 2}, {6, 3}, {9, 3}, {10, 4}, {20, 4}} {
		b := infBuild(tc.level, infGume, infTexto, infResiste)
		b.Weapons = []string{infSword}
		b.Infusions = []InfusionRecord{
			infRec("ok", infGume, infSword),
			infObject("txt", infTexto, "Lanterna"),
			infObject("unk", "feature:fantasma"+tableSuffix, "Pedra"),
			infObject("unpicked", infCouraca, "Anel"),
		}
		d := Derive(b, c)
		if len(d.InfusionHosts) != 1 {
			t.Fatalf("level %d: hosts %+v, want the one", tc.level, d.InfusionHosts)
		}
		h := d.InfusionHosts[0]
		if h.HostKey != infHost || h.ClassLevel != tc.level || h.Limit != tc.limit || h.Used != 2 {
			t.Errorf("level %d: host %+v, want limit %d, used 2 (the unknown and the unpicked take no slot)", tc.level, h, tc.limit)
		}
	}
	// A character without the class has no host: nothing to count against.
	if d := Derive(standard("class:fighter", 5), c); len(d.InfusionHosts) != 0 || len(d.Infusions) != 0 {
		t.Errorf("a fighter has hosts %+v and statuses %+v", d.InfusionHosts, d.Infusions)
	}
}

func TestAnInfusionWithOtherEffectsLeavesTheNumbersAndNamesThePiece(t *testing.T) {
	t.Parallel()
	c := withOverlay(t, infusionRulesOverlay())
	b := infBuild(6, infAll...) // three records fit the limit of level 6
	b.Weapons = []string{infAxe, infSword}
	b.Armor = infArmor
	plainD := Derive(b, c)
	b.Infusions = []InfusionRecord{
		infRec("ret", infRetorno, infAxe),
		{ID: "res", InfusionKey: infResiste, ItemKind: InfusionItemGear, ItemKey: infArmor, Parameter: "damage-type:fire"},
		infObject("txt", infTexto, "Lanterna"),
	}
	d := Derive(b, c)
	if d.ArmorClass != plainD.ArmorClass || len(d.ItemModifiers) != 0 || !slices.Equal(d.Resistances, plainD.Resistances) {
		t.Errorf("a label changed a number: AC %d (%d), modifiers %+v, resistances %v", d.ArmorClass, plainD.ArmorClass, d.ItemModifiers, d.Resistances)
	}
	axe, _ := attackOf(d, infAxe)
	want, _ := attackOf(plainD, infAxe)
	axe.InfusionNamePT = ""
	if axe.AttackBonus != want.AttackBonus || axe.Damage != want.Damage || axe.Magical {
		t.Errorf("a returning weapon's numbers moved: %+v", axe)
	}
	if a, _ := attackOf(d, infAxe); a.InfusionNamePT != "Retorno de Lâmina" {
		t.Errorf("the returning axe names %q", a.InfusionNamePT)
	}
	names := map[string]string{}
	for _, g := range DeriveGear(b, c, d) {
		names[g.Key] = g.InfusionNamePT
	}
	if names[infAxe] != "Retorno de Lâmina" || names[infArmor] != "Manto de Cinza" || names[infSword] != "" {
		t.Errorf("gear names = %v", names)
	}
	res := infStatus(t, d, "res")
	if !res.Valid || res.Parameter != "damage-type:fire" || !res.Computed || res.Bonus != 0 || res.Effect != InfusionEffectResistanceLabel {
		t.Errorf("resistance status = %+v", res)
	}
	if txt := infStatus(t, d, "txt"); !txt.Valid || txt.Computed || txt.ItemNamePT != "Lanterna" || txt.NamePT != "Marca Muda" {
		t.Errorf("text only status = %+v", txt)
	}
}

func TestTheRepeatingWeaponHasNoAmmunitionKeyAndTheRestStays(t *testing.T) {
	t.Parallel()
	c := withOverlay(t, infusionRulesOverlay())
	b := infBuild(5, infAll...)
	b.Weapons = []string{infBow, infBow2}
	plainD := Derive(b, c)
	if a, _ := attackOf(plainD, infBow); a.AmmunitionKey == "" {
		t.Fatalf("the longbow fires nothing without the infusion: %+v", a)
	}
	b.Infusions = []InfusionRecord{infRec("r1", infCesto, infBow)}
	d := Derive(b, c)
	bow, _ := attackOf(d, infBow)
	other, _ := attackOf(d, infBow2)
	want, _ := attackOf(plainD, infBow)
	if bow.AmmunitionKey != "" || other.AmmunitionKey == "" {
		t.Errorf("ammunition keys: repeating %q, other %q", bow.AmmunitionKey, other.AmmunitionKey)
	}
	if bow.AttackBonus != want.AttackBonus || bow.Damage != want.Damage || bow.InfusionNamePT != "Cesto Que Repõe" {
		t.Errorf("the repeating line = %+v, want the plain numbers and the name", bow)
	}
	if _, ok := attackOf(d, infBow+ImprovisedMeleeSuffix); !ok {
		t.Errorf("the improvised melee line of the repeating weapon is gone")
	}
	if _, ok := attackOf(d, infBow2+ImprovisedMeleeSuffix); !ok {
		t.Errorf("the improvised melee line of the other bow is gone")
	}
}

func TestTheSpellFocusAddsToTheSpellAttackAndLeavesTheWeaponAlone(t *testing.T) {
	t.Parallel()
	c := withOverlay(t, infusionRulesOverlay())
	base := func(level int) Build {
		b := infBuild(level, infAll...)
		b.Cantrips = []string{"spell:fire-bolt"}
		b.Weapons = []string{infStaff}
		return b
	}
	plainD := Derive(base(5), c)
	if len(plainD.Spellcasting) == 0 {
		t.Fatal("the wizard casts nothing")
	}
	bolt, _ := attackOf(plainD, "spell:fire-bolt")
	staff, _ := attackOf(plainD, infStaff)
	focus := func(level int, attuned bool, kind string) Derived {
		b := base(level)
		rec := infObject("f1", infFoco, "Cajado de Prata")
		if kind == InfusionItemGear {
			rec = infRec("f1", infFoco, infStaff)
		}
		rec.Attuned = attuned
		b.Infusions = []InfusionRecord{rec}
		return Derive(b, c)
	}
	for _, kind := range []string{InfusionItemObject, InfusionItemGear} {
		d := focus(5, true, kind)
		if got, want := d.Spellcasting[0].AttackBonus, plainD.Spellcasting[0].AttackBonus+1; got != want {
			t.Errorf("%s: spell attack %d, want %d", kind, got, want)
		}
		if a, _ := attackOf(d, "spell:fire-bolt"); a.AttackBonus != bolt.AttackBonus+1 {
			t.Errorf("%s: the cantrip attacks at %d, want %d", kind, a.AttackBonus, bolt.AttackBonus+1)
		}
		if d.Spellcasting[0].SaveDC != plainD.Spellcasting[0].SaveDC {
			t.Errorf("%s: the save DC moved to %d", kind, d.Spellcasting[0].SaveDC)
		}
		st, _ := attackOf(d, infStaff)
		if st.AttackBonus != staff.AttackBonus || st.Damage != staff.Damage || st.Magical || st.MagicBonus != 0 {
			t.Errorf("%s: the staff's own numbers moved: %+v", kind, st)
		}
		lines := d.ItemModifiers
		if len(lines) != 1 || lines[0].Target != "spell_attack" || lines[0].Value != 1 || !lines[0].Counts || lines[0].InfusionID != "f1" || lines[0].InfusionNamePT != "Diapasão Interno" {
			t.Errorf("%s: modifier lines = %+v", kind, lines)
		}
	}
	if d := focus(5, true, InfusionItemGear); d.Attacks[slices.IndexFunc(d.Attacks, func(a Attack) bool { return a.Key == infStaff })].InfusionNamePT != "Diapasão Interno" {
		t.Errorf("the focus quarterstaff's attack does not name the infusion")
	}
	if d := focus(10, true, InfusionItemObject); d.Spellcasting[0].AttackBonus != Derive(base(10), c).Spellcasting[0].AttackBonus+2 {
		t.Errorf("level 10: spell attack %d, want +2", d.Spellcasting[0].AttackBonus)
	}
	d := focus(5, false, InfusionItemObject)
	if d.Spellcasting[0].AttackBonus != plainD.Spellcasting[0].AttackBonus || len(d.ItemModifiers) != 1 || d.ItemModifiers[0].Counts || d.ItemModifiers[0].ReasonPT != "Pede sintonização." {
		t.Errorf("a focus that is not attuned: spell attack %d, lines %+v", d.Spellcasting[0].AttackBonus, d.ItemModifiers)
	}
	// A focus carried, not held, adds nothing.
	b := base(5)
	b.Carried = []string{infStaff}
	b.Infusions = []InfusionRecord{{ID: "f1", InfusionKey: infFoco, ItemKind: InfusionItemGear, ItemKey: infStaff, Attuned: true}}
	if d := Derive(b, c); d.Spellcasting[0].AttackBonus != plainD.Spellcasting[0].AttackBonus {
		t.Errorf("a carried focus adds %d", d.Spellcasting[0].AttackBonus-plainD.Spellcasting[0].AttackBonus)
	}
}

func TestAnAttunedInfusionCountsAsAnAttunedItem(t *testing.T) {
	t.Parallel()
	c := withOverlay(t, infusionRulesOverlay())
	b := infBuild(5, infGume, infSelo, infFoco, infTexto)
	b.Weapons = []string{infSword, infAxe}
	b.ItemBonuses = []ItemBonus{{Key: infAxe, Bonus: 1, RequiresAttunement: true, Attuned: true}}
	b.MagicItems = []SheetMagicItem{{Key: "item:ring-of-protection", Attuned: true, ID: "m1"}}
	if got := AttunedCount(b, c); got != 2 {
		t.Fatalf("without records %d, want 2", got)
	}
	seal := infRec("s", infSelo, infSword)
	seal.Attuned = true
	focus := infObject("f", infFoco, "Cajado")
	focus.Attuned = true
	plainRec := infRec("g", infGume, infSword) // asks no attunement
	plainRec.Attuned = true
	invalid := infObject("x", infCouraca, "Anel") // not picked
	invalid.Attuned = true
	b.Infusions = []InfusionRecord{seal, focus, plainRec, invalid}
	if got := AttunedCount(b, c); got != 4 {
		t.Errorf("with records %d, want 4 (the seal and the focus join; an infusion that asks nothing and a not valid one do not)", got)
	}
	b.Infusions[0].Attuned = false
	if got := AttunedCount(b, c); got != 3 {
		t.Errorf("an unattuned seal still counts: %d", got)
	}
	if MaxAttunements != 3 {
		t.Errorf("MaxAttunements = %d", MaxAttunements)
	}
}

func TestAManualBonusOnAnInfusedKeyIsRefusedAndOtherEffectsLeaveItAlone(t *testing.T) {
	t.Parallel()
	c := withOverlay(t, infusionRulesOverlay())
	b := infBuild(5, infAll...)
	b.Weapons = []string{infSword, infStaff}
	b.Armor = infArmor
	b.Infusions = []InfusionRecord{infRec("r1", infGume, infSword)}
	b.ItemBonuses = []ItemBonus{{Key: infArmor, Bonus: 1}}
	stored := b
	if err := CheckManualBonuses(stored, b, c); err != nil {
		t.Fatalf("a manual bonus on another piece: %v", err)
	}
	// A new manual bonus on the infused sword is refused, at its own entry.
	b.ItemBonuses = []ItemBonus{{Key: infArmor, Bonus: 1}, {Key: infSword, Bonus: 1}}
	var ve *ValidationError
	if err := CheckManualBonuses(stored, b, c); !errors.As(err, &ve) || ve.Field != "full.item_bonuses[1].key" {
		t.Fatalf("a manual bonus on the infused sword: %v, want a refusal at full.item_bonuses[1].key", err)
	}
	// So is a changed one: the stored pair stands, a bigger bonus does not.
	stored.ItemBonuses = []ItemBonus{{Key: infSword, Bonus: 1}}
	b.ItemBonuses = []ItemBonus{{Key: infSword, Bonus: 2}}
	if err := CheckManualBonuses(stored, b, c); !errors.As(err, &ve) || ve.Field != "full.item_bonuses[0].key" {
		t.Fatalf("a changed manual bonus: %v, want a refusal", err)
	}
	// Other effects leave the manual bonus alone: a focus on the staff, a label on the armor, a text only object.
	b.Infusions = []InfusionRecord{infRec("f", infFoco, infStaff), infRec("l", infResiste, infArmor), infObject("o", infTexto, "Lanterna")}
	b.ItemBonuses = []ItemBonus{{Key: infStaff, Bonus: 1}, {Key: infArmor, Bonus: 1}}
	if err := CheckManualBonuses(Build{}, b, c); err != nil {
		t.Errorf("a manual bonus beside an infusion with another effect: %v", err)
	}
	// An infusion that is not valid now refuses nothing.
	b.Infusions = []InfusionRecord{infRec("r1", infGume, infSword)}
	b.FeatureChoices = []string{infTexto}
	b.ItemBonuses = []ItemBonus{{Key: infSword, Bonus: 1}}
	if err := CheckManualBonuses(Build{}, b, c); err != nil {
		t.Errorf("a record that is not valid now refused a manual bonus: %v", err)
	}
}

// A sheet can reach the stale pair (a record on a key the content lost, a manual bonus added, the key back): Derive
// resolves it, so the pair saves, validates and levels up, and only a new or changed manual bonus is refused.
func TestAStalePairKeepsSavingAndValidating(t *testing.T) {
	t.Parallel()
	c := withOverlay(t, infusionRulesOverlay())
	b := infBuild(5, infAll...)
	b.Weapons = []string{infSword}
	b.ItemBonuses = []ItemBonus{{Key: infSword, Bonus: 1}}
	b.Infusions = []InfusionRecord{infRec("r1", infGume, infSword)}
	if err := Validate(b, c); err != nil {
		t.Errorf("Validate refused the stale pair: %v", err)
	}
	if err := CheckManualBonuses(b, b, c); err != nil {
		t.Errorf("an untouched stale pair was refused on a save: %v", err)
	}
	after := b
	after.Cantrips = []string{"spell:fire-bolt"} // an unrelated edit
	if err := CheckManualBonuses(b, after, c); err != nil {
		t.Errorf("an unrelated edit of a stale pair was refused: %v", err)
	}
	if s := infStatus(t, Derive(b, c), "r1"); s.Valid {
		t.Errorf("the stale record is valid: %+v", s)
	}
	next := b
	next.Classes = []ClassLevel{{Class: "class:wizard", Subclass: infSubclass, Level: 6}}
	if err := Validate(next, c); err != nil {
		t.Errorf("Validate of the level-up build with the stale pair: %v", err)
	}
}

func TestALostOptionMakesItsRecordNotValidNow(t *testing.T) {
	t.Parallel()
	c := withOverlay(t, infusionRulesOverlay())
	b := infBuild(5, infAll...)
	b.Weapons = []string{infSword}
	b.Infusions = []InfusionRecord{infRec("r1", infGume, infSword)}
	d := deriveWith(b, c.c, map[string]*LostPrerequisite{infGume: {}})
	s := infStatus(t, d, "r1")
	if s.Valid || s.ReasonPT != reasonInfusionLost || s.Bonus != 0 {
		t.Errorf("status = %+v, want not valid now: lost", s)
	}
	if a, _ := attackOf(d, infSword); a.MagicBonus != 0 || a.InfusionNamePT != "" || len(d.ItemModifiers) != 0 || infHostUsed(d) != 0 {
		t.Errorf("a lost option's record gave %+v %+v used %d", a, d.ItemModifiers, infHostUsed(d))
	}
}

func TestBelowTheFirstStepAValidRecordGivesNoLine(t *testing.T) {
	t.Parallel()
	o := infusionRulesOverlay()
	opts := o.Subclasses[0].Levels[0].Features[0].Options
	late := []TableInfusionStep{{ClassLevel: 5, Bonus: 1}}
	opts[0].Infusion.BonusSteps, opts[3].Infusion.BonusSteps = late, late
	c := withOverlay(t, o)
	b := infBuild(3, infAll...)
	b.Weapons = []string{infSword}
	plainD := Derive(b, c)
	focus := infObject("f", infFoco, "Cajado")
	focus.Attuned = true
	b.Infusions = []InfusionRecord{infRec("r1", infGume, infSword), focus}
	d := Derive(b, c)
	if len(d.ItemModifiers) != 0 {
		t.Errorf("lines below the first step: %+v", d.ItemModifiers)
	}
	if a, _ := attackOf(d, infSword); a.MagicBonus != 0 || a.Magical {
		t.Errorf("a zero bonus changed the attack: %+v", a)
	}
	if d.Spellcasting[0].AttackBonus != plainD.Spellcasting[0].AttackBonus {
		t.Errorf("spell attack %d, want %d", d.Spellcasting[0].AttackBonus, plainD.Spellcasting[0].AttackBonus)
	}
	for _, id := range []string{"r1", "f"} {
		if s := infStatus(t, d, id); !s.Valid || s.Bonus != 0 || s.Counts {
			t.Errorf("%s: status %+v, want valid with no number", id, s)
		}
	}
}

func TestDeriveLeavesTheBuildAloneAndOneRecordHoldsAPiece(t *testing.T) {
	t.Parallel()
	c := withOverlay(t, infusionRulesOverlay())
	b := infBuild(5, infAll...)
	b.Weapons = []string{infSword}
	b.ItemBonuses = []ItemBonus{{Key: infArmor, Bonus: 1}}
	b.Armor = infArmor
	old, newer := infRec("old", infGume, infSword), infRec("new", infSelo, infSword)
	old.Since, newer.Since = time.Unix(100, 0), time.Unix(200, 0)
	newer.Attuned = true
	for _, order := range [][]InfusionRecord{{old, newer}, {newer, old}} {
		b.Infusions = order
		before := slices.Clone(b.ItemBonuses)
		d := Derive(b, c)
		if !slices.Equal(b.ItemBonuses, before) {
			t.Errorf("Derive changed the build's item bonuses: %+v", b.ItemBonuses)
		}
		if s := infStatus(t, d, "old"); !s.Valid {
			t.Errorf("the oldest record is not valid: %+v", s)
		}
		if s := infStatus(t, d, "new"); s.Valid || s.ReasonPT != reasonInfusionTaken {
			t.Errorf("the newer record = %+v, want not valid: taken", s)
		}
		if a, _ := attackOf(d, infSword); a.MagicBonus != 1 || a.InfusionNamePT != "Gume Vivo" {
			t.Errorf("the piece = %+v, want the oldest infusion's +1", a)
		}
		if infHostUsed(d) != 1 || AttunedCount(b, c) != 0 {
			t.Errorf("used %d, attuned %d, want 1 and 0", infHostUsed(d), AttunedCount(b, c))
		}
	}
}

func TestARecordOnAPieceThatIsNotOnTheSheetIsNotValidNow(t *testing.T) {
	t.Parallel()
	c := withOverlay(t, infusionRulesOverlay())
	b := infBuild(5, infAll...)
	b.Weapons = []string{infSword}
	b.Infusions = []InfusionRecord{infRec("r1", infGume, infAxe)}
	d := Derive(b, c)
	if s := infStatus(t, d, "r1"); s.Valid || s.ReasonPT != reasonInfusionNoPiece {
		t.Errorf("status = %+v, want not valid: no piece", s)
	}
	if len(d.ItemModifiers) != 0 || infHostUsed(d) != 0 {
		t.Errorf("modifiers %+v used %d", d.ItemModifiers, infHostUsed(d))
	}
}

func TestAttunedCountWithoutContentCountsNoInfusions(t *testing.T) {
	t.Parallel()
	b := Build{ItemBonuses: []ItemBonus{{Key: infSword, Bonus: 1, RequiresAttunement: true, Attuned: true}}, Infusions: []InfusionRecord{infRec("r", infSelo, infSword)}}
	if got := AttunedCount(b, nil); got != 1 {
		t.Errorf("AttunedCount = %d, want 1", got)
	}
}

func TestAStaleManualBonusStandsAndTheRecordIsNotValid(t *testing.T) {
	t.Parallel()
	c := withOverlay(t, infusionRulesOverlay())
	b := infBuild(5, infAll...)
	b.Weapons = []string{infSword}
	b.ItemBonuses = []ItemBonus{{Key: infSword, Bonus: 2}}
	manual := Derive(b, c)
	b.Infusions = []InfusionRecord{infRec("r1", infGume, infSword)}
	d := Derive(b, c)
	a, _ := attackOf(d, infSword)
	m, _ := attackOf(manual, infSword)
	if a.AttackBonus != m.AttackBonus || a.Damage != m.Damage || a.MagicBonus != 2 || a.InfusionNamePT != "" {
		t.Errorf("attack %+v, want the manual +2 alone", a)
	}
	if len(d.ItemModifiers) != len(manual.ItemModifiers) {
		t.Fatalf("modifiers %+v, want the manual lines %+v", d.ItemModifiers, manual.ItemModifiers)
	}
	for _, mod := range d.ItemModifiers {
		if mod.Value != 2 || mod.InfusionID != "" || mod.InfusionNamePT != "" {
			t.Errorf("a line carries the record: %+v", mod)
		}
	}
	if s := infStatus(t, d, "r1"); s.Valid || s.ReasonPT == "" || s.Counts || s.Bonus != 0 {
		t.Errorf("status %+v, want not valid now", s)
	}
	if infHostUsed(d) != 0 {
		t.Errorf("the stale record takes %d slots", infHostUsed(d))
	}
	if n := AttunedCount(b, c); n != 0 {
		t.Errorf("attuned %d", n)
	}
}

func TestAnInfusionRecordOfAnotherArtificerIsNotValidNow(t *testing.T) {
	t.Parallel()
	c := withOverlay(t, infusionRulesOverlay())
	b := infBuild(5, infAll...)
	b.CharacterID, b.Weapons = "me", []string{infSword}
	own, other := infRec("r1", infGume, infSword), infRec("r2", infGume, infSword)
	own.ArtificerID, other.ArtificerID = "me", "someone-else"
	b.Infusions = []InfusionRecord{other}
	d := Derive(b, c)
	if s := infStatus(t, d, "r2"); s.Valid || s.ReasonPT != reasonInfusionOther || s.Bonus != 0 || infHostUsed(d) != 0 {
		t.Errorf("status = %+v used %d, want not valid now: another artificer, no slot", s, infHostUsed(d))
	}
	b.Infusions = []InfusionRecord{own}
	if s := infStatus(t, Derive(b, c), "r1"); !s.Valid {
		t.Errorf("the character's own record = %+v, want valid", s)
	}
}

func TestAnInfusionRecordOfAKeyTheContentLostIsNamedWithoutTheKey(t *testing.T) {
	t.Parallel()
	c := withOverlay(t, infusionRulesOverlay())
	b := infBuild(5, infAll...)
	b.Infusions = []InfusionRecord{infRec("r1", "feature:gone", infSword)}
	if s := infStatus(t, Derive(b, c), "r1"); s.NamePT != "Infusão que a ficha não conhece" {
		t.Errorf("NamePT = %q, want the name of an unknown infusion", s.NamePT)
	}
}
