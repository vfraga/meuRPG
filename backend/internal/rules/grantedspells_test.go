package rules

import (
	"errors"
	"slices"
	"strings"
	"testing"
)

// The granted_spell effect. Every name here is invented; the spells are the SRD's.

const (
	graceFeat   = "feat:graca-de-teste" + tableSuffix
	graceTrait  = "trait:dom-de-teste" + tableSuffix
	graceRace   = "race:agraciada" + tableSuffix
	graceResrce = "usos_da_graca"
)

func grant(spell, ability string, edit ...func(*Effect)) Effect {
	e := Effect{Type: "granted_spell", Spell: spell, Ability: ability}
	for _, f := range edit {
		f(&e)
	}
	return e
}

func graceOverlay(feat []Effect, trait []Effect) Overlay {
	return Overlay{
		Revision: 1,
		Races: []TableRace{{
			TableEntry: TableEntry{Key: graceRace, NamePT: "Agraciada"}, Size: "Medium", SpeedFt: 30,
			Traits: []TableFeature{{Key: graceTrait, NamePT: "Dom de teste", DescPT: []string{"Texto de teste."}, Effects: trait}},
		}},
		Feats: []TableFeat{{TableEntry: TableEntry{Key: graceFeat, NamePT: "Graça de teste"}, DescPT: []string{"Texto de teste."}, Effects: feat}},
	}
}

func graceBuild(level int, feats []string, choices ...string) Build {
	return Build{
		BaseScores: map[Ability]int{STR: 10, DEX: 12, CON: 13, INT: 14, WIS: 8, CHA: 16},
		Race:       graceRace, Background: "background:acolyte", Feats: feats, FeatureChoices: choices,
		Classes: []ClassLevel{{Class: "class:fighter", Level: level}},
	}
}

func grantedOf(d Derived, spell string) (GrantedSpell, bool) {
	i := slices.IndexFunc(d.GrantedSpells, func(g GrantedSpell) bool { return g.Spell.Key == spell })
	if i < 0 {
		return GrantedSpell{}, false
	}
	return d.GrantedSpells[i], true
}

func TestAGrantedSpellUsesItsOwnAbilityAndTheProficiencyBonus(t *testing.T) {
	uses := func(e *Effect) { e.Uses = graceResrce }
	c := withOverlay(t, graceOverlay([]Effect{
		{Type: "resource", Resource: graceResrce, Max: "1", Recharge: "long_rest"},
		grant("spell:misty-step", "cha", uses),
		grant("spell:light", "int"),
	}, nil))
	d := Derive(graceBuild(5, []string{graceFeat}), c)

	step, ok := grantedOf(d, "spell:misty-step")
	if !ok {
		t.Fatalf("no granted spell: %+v", d.GrantedSpells)
	}
	// Level 5: proficiency +3; Charisma 16 is +3.
	if step.Ability != CHA || step.SaveDC != 8+3+3 || step.AttackBonus != 3+3 {
		t.Errorf("misty step: ability %q, DC %d, attack %+d, want cha, 14 and +6", step.Ability, step.SaveDC, step.AttackBonus)
	}
	if step.AtWill || step.Resource != graceResrce || step.Source != graceFeat || step.SourcePT != "Graça de teste" {
		t.Errorf("misty step: %+v, want a limited grant from the feat", step)
	}
	light, _ := grantedOf(d, "spell:light")
	if !light.AtWill || light.Ability != INT || light.SaveDC != 8+3+2 {
		t.Errorf("light: %+v, want at will with Intelligence (DC 13)", light)
	}
	if got := d.GrantedSpells[0].Spell.Key; got != "spell:light" {
		t.Errorf("first granted spell = %s, want the cantrip first", got)
	}
}

func TestAGrantedSpellTakesNothingFromTheClassNumbers(t *testing.T) {
	c := withOverlay(t, graceOverlay([]Effect{grant("spell:misty-step", "wis", func(e *Effect) { e.CountsAsKnown = true })}, nil))
	without := Derive(graceBuild(5, nil), c)
	with := Derive(graceBuild(5, []string{graceFeat}), c)
	if !slices.EqualFunc(with.Issues, without.Issues, func(a, b Issue) bool { return a.Code == b.Code && a.Message == b.Message }) {
		t.Errorf("issues changed: %+v, want %+v", with.Issues, without.Issues)
	}
	if !slices.EqualFunc(with.Spellcasting, without.Spellcasting, func(a, b Spellcasting) bool {
		return a.SpellsKnownMax == b.SpellsKnownMax && a.CantripsKnown == b.CantripsKnown && a.PreparedMax == b.PreparedMax
	}) {
		t.Error("the class's numbers changed")
	}
	i := slices.IndexFunc(with.Spells, func(s CharacterSpell) bool { return s.Spell.Key == "spell:misty-step" })
	if i < 0 || with.Spells[i].Prepared {
		t.Errorf("a spell the grant counts as known is not listed as known and not prepared: %+v", with.Spells)
	}
	plain := withOverlay(t, graceOverlay([]Effect{grant("spell:misty-step", "wis")}, nil))
	if d := Derive(graceBuild(5, []string{graceFeat}), plain); slices.ContainsFunc(d.Spells, func(s CharacterSpell) bool { return s.Spell.Key == "spell:misty-step" }) {
		t.Error("a grant that does not count as known lists the spell among the known ones")
	}
}

func TestAGrantedSpellWaitsForItsLevel(t *testing.T) {
	// The race's trait gates on the character level; the formula may read any level.
	c := withOverlay(t, graceOverlay(nil, []Effect{grant("spell:misty-step", "cha", func(e *Effect) { e.FromLevel = "3" })}))
	if _, ok := grantedOf(Derive(graceBuild(2, nil), c), "spell:misty-step"); ok {
		t.Error("the spell is granted at level 2, before its level 3")
	}
	if _, ok := grantedOf(Derive(graceBuild(3, nil), c), "spell:misty-step"); !ok {
		t.Error("the spell is not granted at level 3")
	}

	// A formula: one level above the fighter's, which is the character's whole level.
	c = withOverlay(t, graceOverlay(nil, []Effect{grant("spell:misty-step", "cha", func(e *Effect) { e.FromLevel = "classLevel(\"fighter\") + 1" })}))
	if _, ok := grantedOf(Derive(graceBuild(3, nil), c), "spell:misty-step"); ok {
		t.Error("a grant from level 4 is there at 3")
	}
}

func TestAClassFeatureGatesOnTheClassLevel(t *testing.T) {
	c := withOverlay(t, pathOverlay(grant("spell:misty-step", "cha", func(e *Effect) { e.FromLevel = "5" })))
	b := pathBuild(3)
	b.Classes = []ClassLevel{{Class: "class:barbarian", Level: 3, Subclass: pathSubclass}, {Class: "class:fighter", Level: 3}}
	// Level 6 in all, but only 3 barbarian levels: the feature's own level is the gate.
	if _, ok := grantedOf(Derive(b, c), "spell:misty-step"); ok {
		t.Error("a barbarian feature granted at class level 3 when it asks for 5")
	}
	b.Classes[0].Level = 5
	if _, ok := grantedOf(Derive(b, c), "spell:misty-step"); !ok {
		t.Error("a barbarian feature did not grant at class level 5")
	}
}

func TestACastingAbilityChoiceIsPickedOnce(t *testing.T) {
	c := withOverlay(t, graceOverlay([]Effect{
		grant("spell:misty-step", GrantedAbilityChoice),
		grant("spell:light", GrantedAbilityChoice),
	}, nil))
	key := GrantedAbilityChoiceKey(graceFeat)

	d := Derive(graceBuild(5, []string{graceFeat}), c)
	if g, _ := grantedOf(d, "spell:light"); !g.AbilityPending || g.Ability != "" || g.SaveDC != 0 || g.AttackBonus != 0 {
		t.Errorf("before the pick: %+v, want a pending ability and no numbers", g)
	}
	set := c.Choices(graceBuild(5, []string{graceFeat}))
	var found *Choice
	for gi := range set.Groups {
		for ci := range set.Groups[gi].Choices {
			if ch := &set.Groups[gi].Choices[ci]; ch.Key == key {
				found = ch
			}
		}
	}
	if found == nil || found.Picks != 1 || len(found.Options) != 3 {
		t.Fatalf("choice = %+v, want one pick among int, wis and cha, asked once for the two grants", found)
	}

	picked := Derive(graceBuild(5, []string{graceFeat}, ScopedChoice(key, "ability:int")), c)
	for _, spell := range []string{"spell:misty-step", "spell:light"} {
		if g, _ := grantedOf(picked, spell); g.Ability != INT || g.SaveDC != 8+3+2 {
			t.Errorf("%s after the pick: %+v, want Intelligence with DC 13", spell, g)
		}
	}
	// Strength is not one of the three.
	bad := Derive(graceBuild(5, []string{graceFeat}, ScopedChoice(key, "ability:str")), c)
	if g, _ := grantedOf(bad, "spell:light"); !g.AbilityPending {
		t.Errorf("a pick outside int, wis and cha was taken: %+v", g)
	}
}

func TestAGrantedSpellFollowsItsWhen(t *testing.T) {
	c := withOverlay(t, graceOverlay([]Effect{grant("spell:light", "cha", func(e *Effect) { e.When = `armor() == "none"` })}, nil))
	if _, ok := grantedOf(Derive(graceBuild(1, []string{graceFeat}), c), "spell:light"); !ok {
		t.Error("the spell is not granted without armor")
	}
	b := graceBuild(1, []string{graceFeat})
	b.Armor = "equipment:chain-mail"
	if _, ok := grantedOf(Derive(b, c), "spell:light"); ok {
		t.Error("the spell is granted in chain mail, against its when")
	}
}

func TestGrantedSpellRefusalsNameTheirField(t *testing.T) {
	t.Parallel()
	srd := loadForTest(t)
	res := Effect{Type: "resource", Resource: graceResrce, Max: "1", Recharge: "long_rest"}
	cases := []struct {
		name    string
		effects []Effect
		field   string
	}{
		{"unknown spell", []Effect{grant("spell:no-such-spell", "wis")}, ".spell"},
		{"a cantrip as a ritual", []Effect{grant("spell:light", "wis", func(e *Effect) { e.RitualOnly = true })}, ".ritual_only"},
		{"a ritual with a slot", []Effect{grant("spell:detect-magic", "wis", func(e *Effect) { e.RitualOnly, e.AlsoWithSlot = true, true })}, ".also_with_slot"},
		{"a made-up ability", []Effect{grant("spell:light", "str")}, ".ability"},
		{"no ability", []Effect{grant("spell:light", "")}, ".ability"},
		{"a resource nobody defines", []Effect{grant("spell:light", "wis", func(e *Effect) { e.Uses = "recurso_fantasma" })}, ".uses"},
		{"a resource with a bad name", []Effect{res, grant("spell:light", "wis", func(e *Effect) { e.Uses = "Sem Espaço" })}, ".uses"},
		{"a cost with no resource", []Effect{grant("spell:light", "wis", func(e *Effect) { e.UsesCost = 2 })}, ".uses_cost"},
		{"a cost with an at-will grant", []Effect{grant("spell:light", "wis", func(e *Effect) { e.Uses, e.UsesCost = GrantedAtWill, 2 })}, ".uses_cost"},
		{"a cost over the bound", []Effect{res, grant("spell:light", "wis", func(e *Effect) { e.Uses, e.UsesCost = graceResrce, MaxGrantUsesCost+1 })}, ".uses_cost"},
		{"a negative cost", []Effect{res, grant("spell:light", "wis", func(e *Effect) { e.Uses, e.UsesCost = graceResrce, -1 })}, ".uses_cost"},
		{"a level formula that fails", []Effect{grant("spell:light", "wis", func(e *Effect) { e.FromLevel = "nope()" })}, ".from_level"},
	}
	for _, tc := range cases {
		_, err := srd.With(graceOverlayNoRace(tc.effects))
		oe, ok := errors.AsType[*OverlayError](err)
		if !ok {
			t.Errorf("%s: err = %v, want an overlay error", tc.name, err)
			continue
		}
		if !strings.HasSuffix(oe.Field, tc.field) {
			t.Errorf("%s: refused at %q, want the field %s", tc.name, oe.Field, tc.field)
		}
	}
	// What is right goes through: a ritual spell as a ritual, a resource of the feat.
	ok := []Effect{res, grant("spell:detect-magic", "wis", func(e *Effect) { e.RitualOnly = true; e.Uses = graceResrce })}
	if _, err := srd.With(graceOverlayNoRace(ok)); err != nil {
		t.Errorf("a right grant is refused: %v", err)
	}
}

func TestAGrantedSpellFieldsBelongToIt(t *testing.T) {
	t.Parallel()
	for _, e := range []Effect{
		{Type: "note", Spell: "spell:light"},
		{Type: "modifier", Target: "ac", Mode: "add", Value: "1", Uses: graceResrce},
		{Type: "note", AlsoWithSlot: true},
		{Type: "note", NoMaterialComponents: true},
		{Type: "note", UsesCost: 2},
	} {
		if got := unusedField(&e); got == "" {
			t.Errorf("%+v: no stray field found", e)
		}
	}
}

func graceOverlayNoRace(effects []Effect) Overlay {
	return Overlay{Revision: 1, Feats: []TableFeat{{TableEntry: TableEntry{Key: graceFeat, NamePT: "Graça de teste"}, DescPT: []string{"Texto de teste."}, Effects: effects}}}
}

// The grant says what a cast spends and whether the material is waived: one use by default,
// the effect's cost otherwise, nothing for an at-will grant.
func TestAGrantedSpellCarriesItsCostAndTheWaivedMaterial(t *testing.T) {
	c := withOverlay(t, graceOverlay([]Effect{
		{Type: "resource", Resource: graceResrce, Max: "3", Recharge: "long_rest"},
		grant("spell:misty-step", "cha", func(e *Effect) { e.Uses = graceResrce }),
		grant("spell:bless", "cha", func(e *Effect) { e.Uses, e.UsesCost, e.NoMaterialComponents = graceResrce, 2, true }),
		grant("spell:light", "int"),
	}, nil))
	d := Derive(graceBuild(5, []string{graceFeat}), c)
	for _, w := range []struct {
		spell      string
		cost       int
		noMaterial bool
	}{{"spell:misty-step", 1, false}, {"spell:bless", 2, true}, {"spell:light", 0, false}} {
		g, ok := grantedOf(d, w.spell)
		if !ok || g.Cost != w.cost || g.NoMaterialComponents != w.noMaterial {
			t.Errorf("%s = %+v (granted %v), want cost %d and no material %v", w.spell, g, ok, w.cost, w.noMaterial)
		}
	}
}

// A feat's spell arrives at its minimum level, whatever the character level is before it: a
// character below it has no grant, one at it has the grant with its numbers.
func TestAGrantedSpellArrivesAtItsMinimumLevelWithTheNumbersOfItsAbility(t *testing.T) {
	c := withOverlay(t, graceOverlay(nil, []Effect{grant("spell:misty-step", "wis", func(e *Effect) { e.FromLevel = "5" })}))
	if _, ok := grantedOf(Derive(graceBuild(4, nil), c), "spell:misty-step"); ok {
		t.Error("granted at level 4, before its level 5")
	}
	g, ok := grantedOf(Derive(graceBuild(5, nil), c), "spell:misty-step")
	// Level 5: proficiency +3; Wisdom 8 is -1.
	if !ok || g.FromLevel != 5 || g.Ability != WIS || g.SaveDC != 8+3-1 || g.AttackBonus != 3-1 {
		t.Errorf("granted = %+v (%v), want from level 5 with Wisdom: DC 10, attack +2", g, ok)
	}
}
