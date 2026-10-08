package rules

import "testing"

func resourceOf(d Derived, key string) (Resource, bool) {
	for _, r := range d.Resources {
		if r.Key == key {
			return r, true
		}
	}
	return Resource{}, false
}

// SRD 5.1 Fighter: Extra Attack 2 at 5th, 3 at 11th, 4 at 20th.
func TestRulesReviewExpressions_FighterExtraAttack(t *testing.T) {
	c := loadForTest(t)
	for lvl, want := range map[int]int{5: 2, 10: 2, 11: 3, 19: 3, 20: 4} {
		if got := Derive(standard("class:fighter", lvl), c).AttacksPerAction; got != want {
			t.Errorf("fighter %d: app AttacksPerAction = %d, SRD = %d", lvl, got, want)
		}
	}
}

// SRD 5.1 Bard: Font of Inspiration, from 5th level inspiration returns on a short or long rest.
func TestRulesReviewExpressions_BardFontOfInspiration(t *testing.T) {
	c := loadForTest(t)
	for lvl, want := range map[int]string{4: RechargeLongRest, 5: RechargeShortRest, 20: RechargeShortRest} {
		r, ok := resourceOf(Derive(standard("class:bard", lvl), c), "bardic_inspiration")
		if !ok {
			t.Fatalf("bard %d: no bardic_inspiration resource", lvl)
		}
		if r.Recharge != want {
			t.Errorf("bard %d: app recharge = %q, SRD = %q", lvl, r.Recharge, want)
		}
	}
}

// SRD 5.1 Paladin: Aura of Protection adds Cha mod (min +1) to all the paladin's saving throws at 6th.
func TestRulesReviewExpressions_PaladinAuraOfProtection(t *testing.T) {
	c := loadForTest(t)
	b := standard("class:paladin", 6)
	b.BaseScores[CHA] = 15 // human +1 = 16 -> +3
	d := Derive(b, c)
	cha := abilityOf(d, CHA).Modifier
	for _, a := range []Ability{STR, DEX, CON, INT, WIS, CHA} {
		raw := abilityOf(d, a).Modifier
		if saveOf(d, a).Proficient {
			raw += d.ProficiencyBonus
		}
		if got := saveOf(d, a).Bonus; got != raw+cha {
			t.Errorf("paladin 6 save %s: app = %+d, SRD = %+d (raw %+d + Cha %+d)", a, got, raw+cha, raw, cha)
		}
	}
}

// SRD 5.1 Barbarian: Primal Champion (20th) STR and CON +4, max 24.
func TestRulesReviewExpressions_PrimalChampion(t *testing.T) {
	c := loadForTest(t)
	d := Derive(standard("class:barbarian", 20), c) // STR 15+1, CON 13+1
	if got := abilityOf(d, STR).Score; got != 20 {
		t.Errorf("barbarian 20 STR: app = %d, SRD = 20 (16 + 4)", got)
	}
	if got := abilityOf(d, CON).Score; got != 18 {
		t.Errorf("barbarian 20 CON: app = %d, SRD = 18 (14 + 4)", got)
	}
}

// SRD 5.1 Monk: Empty Body... Body and Mind (20th) DEX and WIS +4, max 24.
func TestRulesReviewExpressions_BodyAndMind(t *testing.T) {
	c := loadForTest(t)
	d := Derive(standard("class:monk", 20), c) // DEX 14+1, WIS 10+1
	if got := abilityOf(d, DEX).Score; got != 19 {
		t.Errorf("monk 20 DEX: app = %d, SRD = 19 (15 + 4)", got)
	}
	if got := abilityOf(d, WIS).Score; got != 15 {
		t.Errorf("monk 20 WIS: app = %d, SRD = 15 (11 + 4)", got)
	}
}

// SRD 5.1 Druid: Archdruid (20th) unlimited Wild Shape uses.
func TestRulesReviewExpressions_Archdruid(t *testing.T) {
	c := loadForTest(t)
	r, ok := resourceOf(Derive(standard("class:druid", 20), c), "wild_shape")
	if ok && r.Max < 99 {
		t.Errorf("druid 20 wild_shape: app max = %d, SRD = unlimited (barbarian rage uses 99)", r.Max)
	}
}

// SRD 5.1 Fighter: Indomitable 1/2/3 uses per long rest at 9/13/17.
func TestRulesReviewExpressions_Indomitable(t *testing.T) {
	c := loadForTest(t)
	for lvl, want := range map[int]int{9: 1, 13: 2, 17: 3} {
		r, ok := resourceOf(Derive(standard("class:fighter", lvl), c), "indomitable")
		if !ok {
			t.Errorf("fighter %d: app has no indomitable resource, SRD = %d use(s)", lvl, want)
			continue
		}
		if r.Max != want || r.Recharge != RechargeLongRest {
			t.Errorf("fighter %d indomitable: app = %d/%s, SRD = %d/%s", lvl, r.Max, r.Recharge, want, RechargeLongRest)
		}
	}
}
