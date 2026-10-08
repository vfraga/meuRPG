package rules

import "testing"


// SRD Fighter "Fighting Style": choose one option, and a style can't be taken
// more than once even when the choice comes again.
func TestRulesReviewSheet_FightingStyleCount(t *testing.T) {
	c := loadForTest(t)
	t.Run("defense does not stack across classes", func(t *testing.T) {
		b := standard("class:fighter", 2)
		b.Classes = []ClassLevel{{Class: "class:fighter", Level: 2}, {Class: "class:paladin", Level: 2}}
		b.BaseScores[CHA] = 13
		b.Armor = "equipment:chain-mail"
		b.FeatureChoices = []string{"feature:fighter-fighting-style-defense", "feature:fighting-style-defense"}
		d := Derive(b, c)
		// chain mail 16 + Defense 1 once = 17 (SRD Fighter, Fighting Style: no style twice)
		if d.ArmorClass != 17 {
			t.Errorf("AC got %d, want 17 (SRD Fighter 'Fighting Style': can't take a style more than once)", d.ArmorClass)
		}
	})
	t.Run("two styles for one fighting style feature raise an issue", func(t *testing.T) {
		b := standard("class:fighter", 1)
		b.Armor = "equipment:chain-mail"
		b.FeatureChoices = []string{"feature:fighter-fighting-style-archery", "feature:fighter-fighting-style-defense"}
		d := Derive(b, c)
		b.FeatureChoices = b.FeatureChoices[:1]
		base := Derive(b, c)
		if len(d.Issues) <= len(base.Issues) {
			t.Errorf("Fighter 1 with 2 fighting styles: got %d Issues (same as with one style: %v), want an extra one (SRD Fighter 'Fighting Style': choose one option)", len(d.Issues), base.Issues)
		}
	})
}

// SRD Multiclassing, "Channel Divinity": a second class granting it gives new
// effects but no extra use; extra uses come only from a class level that
// grants them. Paladin 3 / Cleric 6: cleric 6 gives 2 uses.
func TestRulesReviewSheet_ChannelDivinityPool(t *testing.T) {
	c := loadForTest(t)
	b := standard("class:paladin", 3)
	b.Classes = []ClassLevel{{Class: "class:paladin", Level: 3}, {Class: "class:cleric", Level: 6}}
	b.BaseScores[WIS] = 14
	b.BaseScores[STR] = 14
	b.BaseScores[CHA] = 13
	d := Derive(b, c)
	got := -1
	for _, r := range d.Resources {
		if r.Key == "channel_divinity" {
			got = r.Max
		}
	}
	if got != 2 {
		t.Errorf("channel_divinity Max got %d, want 2 (SRD Multiclassing 'Channel Divinity': cleric 6 grants two uses)", got)
	}
}

// SRD Fighter "Extra Attack": 2 attacks at 5th, 3 at 11th, 4 at 20th.
func TestRulesReviewSheet_FighterExtraAttack(t *testing.T) {
	c := loadForTest(t)
	for _, tc := range []struct{ level, want int }{{5, 2}, {11, 3}, {20, 4}} {
		d := Derive(standard("class:fighter", tc.level), c)
		if d.AttacksPerAction != tc.want {
			t.Errorf("Fighter %d AttacksPerAction got %d, want %d (SRD Fighter 'Extra Attack')", tc.level, d.AttacksPerAction, tc.want)
		}
	}
}

// SRD: ability scores top out at 20 (Ability Score Improvement).
func TestRulesReviewSheet_ScoreAbove20(t *testing.T) {
	c := loadForTest(t)
	b := standard("class:fighter", 19)
	b.ExtraAbilityBonuses = map[Ability]int{STR: 10}
	d := Derive(b, c)
	if got := abilityOf(d, STR).Score; got != 26 {
		t.Fatalf("setup: STR got %d, want 26", got)
	}
	found := false
	for _, i := range d.Issues {
		if i.Code == "score_above_20" {
			found = true
		}
	}
	if !found {
		t.Errorf("STR 26: got no score_above_20 issue, want one (SRD Ability Score Improvement: max 20); issues=%v", d.Issues)
	}
}

// SRD Barbarian "Primal Champion": at 20th STR and CON +4, maximum 24.
func TestRulesReviewSheet_PrimalChampion(t *testing.T) {
	c := loadForTest(t)
	b := standard("class:barbarian", 20)
	b.BaseScores[STR] = 18
	b.BaseScores[CON] = 18
	d := Derive(b, c)
	// 18 base + 1 human + 4 Primal Champion = 23
	for _, a := range []Ability{STR, CON} {
		if got := abilityOf(d, a).Score; got != 23 {
			t.Errorf("Barbarian 20 %s got %d, want 23 (SRD Barbarian 'Primal Champion': +4)", a, got)
		}
	}
}
