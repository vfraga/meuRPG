package rules

import (
	"slices"
	"testing"
)

// toren is the table's fighter (the canonical fight): STR 16, a battleaxe.
func toren() Build {
	b := standard("class:fighter", 3)
	b.Weapons = []string{"equipment:battleaxe"}
	return b
}

func TestResourcesAndActions(t *testing.T) {
	t.Parallel()
	c := loadForTest(t)

	d := Derive(toren(), c)
	var sw *Resource
	for i := range d.Resources {
		if d.Resources[i].Key == "second_wind" {
			sw = &d.Resources[i]
		}
	}
	if sw == nil || sw.Max != 1 || sw.Recharge != RechargeShortRest || sw.NamePT != "Retomar o Fôlego" || sw.Source != "feature:second-wind" {
		t.Errorf("Second Wind = %+v", sw)
	}
	var act *Action
	for i := range d.Actions {
		if d.Actions[i].Key == "feature:second-wind" {
			act = &d.Actions[i]
		}
	}
	if act == nil || act.Economy != EconomyBonusAction || act.Resource != "second_wind" {
		t.Errorf("Second Wind action = %+v", act)
	}

	// Action Surge comes at fighter level 2, with its own resource and a free
	// action that spends it.
	if !hasResource(d, "action_surge") {
		t.Errorf("a level 3 fighter has no Action Surge: %+v", d.Resources)
	}
	if hasResource(Derive(standard("class:fighter", 1), c), "action_surge") {
		t.Error("a level 1 fighter has Action Surge")
	}
	// Formulas run at the character's level.
	for level, want := range map[int]int{1: 2, 3: 3, 6: 4, 12: 5, 17: 6} {
		if got := resourceMax(Derive(standard("class:barbarian", level), c), "rage"); got != want {
			t.Errorf("rage at level %d = %d, want %d", level, got, want)
		}
	}
	if got := resourceMax(Derive(standard("class:monk", 5), c), "ki"); got != 5 {
		t.Errorf("ki at monk 5 = %d", got)
	}

	// The wizard has Arcane Recovery once a day and no feature actions.
	p := Derive(pensantus(), c)
	if got := resourceMax(p, "arcane_recovery"); got != 1 || len(p.Actions) != 0 {
		t.Errorf("Pensantus: arcane recovery %d, actions %+v", got, p.Actions)
	}

	// The standard actions are the same for everyone, in this order.
	want := []string{"Atacar", "Conjurar uma magia", "Disparada", "Desengajar", "Esquivar", "Ajudar", "Esconder", "Preparar", "Procurar", "Usar um objeto"}
	if len(p.StandardActions) != len(want) {
		t.Fatalf("standard actions = %+v", p.StandardActions)
	}
	for i, a := range p.StandardActions {
		if a.NamePT != want[i] || a.Economy != EconomyAction || a.Resource != "" {
			t.Errorf("standard action %d = %+v, want %s", i, a, want[i])
		}
	}
}

func hasResource(d Derived, key string) bool { return resourceMax(d, key) > 0 }

func resourceMax(d Derived, key string) int {
	for _, r := range d.Resources {
		if r.Key == key {
			return r.Max
		}
	}
	return 0
}

// TestAttackDice: the structured damage is the string, as numbers.
func TestAttackDice(t *testing.T) {
	t.Parallel()
	c := loadForTest(t)
	d := Derive(toren(), c)
	a, ok := attackOf(d, "equipment:battleaxe")
	if !ok || a.AttackBonus != 5 || a.Damage != "1d8+3" || a.DamageDice != (DiceFormula{Count: 1, Sides: 8, Bonus: 3}) ||
		a.VersatileDice != (DiceFormula{Count: 1, Sides: 10, Bonus: 3}) {
		t.Errorf("battleaxe = %+v, want +5 1d8+3, versatile 1d10+3", a)
	}
	p := Derive(pensantus(), c)
	if fb, _ := attackOf(p, "spell:fire-bolt"); fb.AttackBonus != 6 || fb.DamageDice != (DiceFormula{Count: 1, Sides: 10}) {
		t.Errorf("fire bolt = %+v", fb)
	}
	if qs, _ := attackOf(p, "equipment:quarterstaff"); qs.DamageDice != (DiceFormula{Count: 1, Sides: 6, Bonus: 1}) {
		t.Errorf("quarterstaff = %+v", qs)
	}
}

// TestExtraAttackAndActionSurge: the closed extra_attack effect gives the
// attacks of the Attack action (2 at level 5, never at level 4), and Action
// Surge is a free action that spends its resource.
func TestExtraAttackAndActionSurge(t *testing.T) {
	t.Parallel()
	c := loadForTest(t)
	for _, tt := range []struct {
		class string
		level int
		want  int
	}{
		{"class:fighter", 4, 1},
		{"class:fighter", 5, 2},
		{"class:barbarian", 5, 2},
		{"class:monk", 5, 2},
		{"class:paladin", 5, 2},
		{"class:ranger", 5, 2},
		{"class:wizard", 5, 1},
		{"class:rogue", 5, 1},
	} {
		if got := Derive(standard(tt.class, tt.level), c).AttacksPerAction; got != tt.want {
			t.Errorf("%s %d: AttacksPerAction = %d, want %d", tt.class, tt.level, got, tt.want)
		}
	}
	var surge *Action
	d := Derive(toren(), c)
	for i := range d.Actions {
		if d.Actions[i].Key == "feature:action-surge-1-use" {
			surge = &d.Actions[i]
		}
	}
	if surge == nil || surge.Economy != EconomyFree || surge.Resource != "action_surge" {
		t.Fatalf("Action Surge action = %+v, want a free action that spends action_surge", surge)
	}
	// The action takes its resource's name, not the per-level feature name
	// "Surto de Ação (1 uso)": the screen shows the uses on their own line.
	if surge.NamePT != "Surto de Ação" {
		t.Errorf("Action Surge NamePT = %q, want %q", surge.NamePT, "Surto de Ação")
	}
}

func TestConditions(t *testing.T) {
	t.Parallel()
	got := loadForTest(t).Conditions()
	if len(got) != 15 || got[0].Key != "condition:blinded" || got[0].NamePT != "Cego" {
		t.Errorf("Conditions() = %v, want the 15 SRD conditions from blinded", got)
	}
}

// resourceOf is the derived resource with the key, if the character has one.
func resourceOf(d Derived, key string) (Resource, bool) {
	for _, r := range d.Resources {
		if r.Key == key {
			return r, true
		}
	}
	return Resource{}, false
}

// TestFighterExtraAttackAtLevels11And20: the fighter makes 2 attacks from level
// 5, 3 from 11 and 4 from 20, and a multiclass character takes the highest count
// of its classes.
func TestFighterExtraAttackAtLevels11And20(t *testing.T) {
	t.Parallel()
	c := loadForTest(t)
	for level, want := range map[int]int{4: 1, 5: 2, 10: 2, 11: 3, 19: 3, 20: 4} {
		if got := Derive(standard("class:fighter", level), c).AttacksPerAction; got != want {
			t.Errorf("fighter %d: AttacksPerAction = %d, want %d", level, got, want)
		}
	}
	b := standard("class:fighter", 11)
	b.Classes = []ClassLevel{{Class: "class:fighter", Level: 11}, {Class: "class:barbarian", Level: 5}}
	if got := Derive(b, c).AttacksPerAction; got != 3 {
		t.Errorf("fighter 11 / barbarian 5: AttacksPerAction = %d, want 3", got)
	}
}

// TestIndomitableUsesGrowWithTheFighterLevel: one use at level 9, two at 13 and
// three at 17, back on a long rest, and nothing before level 9.
func TestIndomitableUsesGrowWithTheFighterLevel(t *testing.T) {
	t.Parallel()
	c := loadForTest(t)
	for level, want := range map[int]int{8: 0, 9: 1, 12: 1, 13: 2, 16: 2, 17: 3, 20: 3} {
		r, ok := resourceOf(Derive(standard("class:fighter", level), c), "indomitable")
		if want == 0 {
			if ok {
				t.Errorf("fighter %d: Indomitable is tracked too early: %+v", level, r)
			}
			continue
		}
		if !ok || r.Max != want || r.Recharge != RechargeLongRest {
			t.Errorf("fighter %d: Indomitable = %+v (found %v), want %d uses on a long rest", level, r, ok, want)
		}
	}
}

// TestFontOfInspirationRechargesBardicInspirationOnAShortRest: the uses come back
// on a long rest only until bard level 5, and on a short rest too from then on.
func TestFontOfInspirationRechargesBardicInspirationOnAShortRest(t *testing.T) {
	t.Parallel()
	c := loadForTest(t)
	for level, want := range map[int]string{1: RechargeLongRest, 4: RechargeLongRest, 5: RechargeShortRest, 20: RechargeShortRest} {
		r, ok := resourceOf(Derive(standard("class:bard", level), c), "bardic_inspiration")
		if !ok || r.Recharge != want {
			t.Errorf("bard %d: Bardic Inspiration = %+v (found %v), want recharge %q", level, r, ok, want)
		}
	}
}

// TestLimitedUseFeaturesHaveACounter: the features the SRD limits to a number of
// uses between rests are resources, with their recharge.
func TestLimitedUseFeaturesHaveACounter(t *testing.T) {
	t.Parallel()
	c := loadForTest(t)
	for _, tt := range []struct {
		name, class, subclass string
		level                 int
		resource              string
		max                   int
		recharge              string
	}{
		{"wholeness of body", "class:monk", "subclass:open-hand", 6, "wholeness_of_body", 1, RechargeLongRest},
		{"cleansing touch", "class:paladin", "", 14, "cleansing_touch", 1, RechargeLongRest},
		{"stroke of luck", "class:rogue", "", 20, "stroke_of_luck", 1, RechargeShortRest},
		{"dark one's own luck", "class:warlock", "subclass:fiend", 6, "dark_ones_own_luck", 1, RechargeShortRest},
		{"hurl through hell", "class:warlock", "subclass:fiend", 14, "hurl_through_hell", 1, RechargeLongRest},
	} {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			b := standard(tt.class, tt.level)
			b.Classes[0].Subclass = tt.subclass
			r, ok := resourceOf(Derive(b, c), tt.resource)
			if !ok || r.Max != tt.max || r.Recharge != tt.recharge {
				t.Errorf("%s = %+v (found %v), want %d use(s), %s", tt.name, r, ok, tt.max, tt.recharge)
			}
		})
	}
	// Cleansing Touch has as many uses as the Charisma modifier, at least one.
	b := standard("class:paladin", 14)
	b.BaseScores[CHA] = 17 // human +1 = 18 -> +4
	if r, _ := resourceOf(Derive(b, c), "cleansing_touch"); r.Max != 4 {
		t.Errorf("paladin 14, Cha 18: Cleansing Touch Max = %d, want 4", r.Max)
	}
}

// TestUnlimitedUsesAreStoredAs99: a barbarian's Rage at level 20 is a counter of
// 99 uses, which the screens show as "ilimitado".
func TestUnlimitedUsesAreStoredAs99(t *testing.T) {
	t.Parallel()
	c := loadForTest(t)
	for _, tt := range []struct {
		class, resource string
		level, want     int
	}{
		{"class:barbarian", "rage", 19, 6},
		{"class:barbarian", "rage", 20, 99},
		{"class:druid", "wild_shape", 19, 2},
		{"class:druid", "wild_shape", 20, 99},
	} {
		if r, _ := resourceOf(Derive(standard(tt.class, tt.level), c), tt.resource); r.Max != tt.want {
			t.Errorf("%s %d: %s Max = %d, want %d", tt.class, tt.level, tt.resource, r.Max, tt.want)
		}
	}
}

// TestChannelDivinityIsOnePoolWithTheLargerMax: a second class that grants
// Channel Divinity gives new effects but no extra use, so the pool has the
// larger Max whichever class is listed first.
func TestChannelDivinityIsOnePoolWithTheLargerMax(t *testing.T) {
	t.Parallel()
	c := loadForTest(t)
	for _, order := range [][]ClassLevel{
		{{Class: "class:paladin", Level: 3}, {Class: "class:cleric", Level: 6}},
		{{Class: "class:cleric", Level: 6}, {Class: "class:paladin", Level: 3}},
	} {
		b := standard("class:paladin", 3)
		b.Classes = order
		b.BaseScores[WIS], b.BaseScores[STR], b.BaseScores[CHA] = 14, 14, 13
		n := 0
		var pool Resource
		for _, r := range Derive(b, c).Resources {
			if r.Key == "channel_divinity" {
				n++
				pool = r
			}
		}
		if n != 1 || pool.Max != 2 {
			t.Errorf("%v: %d channel_divinity pool(s) with Max %d, want one with 2 uses (cleric 6)", order, n, pool.Max)
		}
	}
}

// TestResourceAndScoreEffectsAreClosed: the loader refuses a recharge that
// changes by level without both halves or with an unknown recharge, a cap on
// anything but a score or with a normal ceiling, a score modifier that is not an
// add, and a beast_spells effect that carries anything.
func TestResourceAndScoreEffectsAreClosed(t *testing.T) {
	t.Parallel()
	c := loadForTest(t).c
	for name, e := range map[string]*Effect{
		"recharge_if without recharge_then":  {Type: "resource", Resource: "x", Max: "1", Recharge: "long_rest", RechargeIf: "level() >= 5"},
		"recharge_then without recharge_if":  {Type: "resource", Resource: "x", Max: "1", Recharge: "long_rest", RechargeThen: "short_rest"},
		"an unknown recharge_then":           {Type: "resource", Resource: "x", Max: "1", Recharge: "long_rest", RechargeIf: "level() >= 5", RechargeThen: "never"},
		"a recharge_if that is not a Bool":   {Type: "resource", Resource: "x", Max: "1", Recharge: "long_rest", RechargeIf: "level()", RechargeThen: "short_rest"},
		"a cap on an armor class":            {Type: "modifier", Target: "ac", Mode: "add", Value: "1", Cap: 24},
		"a score modifier without a cap":     {Type: "modifier", Target: "score.str", Mode: "add", Value: "4"},
		"a cap that is the normal ceiling":   {Type: "modifier", Target: "score.str", Mode: "add", Value: "4", Cap: 20},
		"a score modifier that sets":         {Type: "modifier", Target: "score.str", Mode: "set", Value: "4", Cap: 24},
		"a score of an unknown ability":      {Type: "modifier", Target: "score.luck", Mode: "add", Value: "4", Cap: 24},
		"beast_spells with a resource":       {Type: "beast_spells", Resource: "x"},
		"beast_spells with a challenge rate": {Type: "beast_spells", MaxCR: "1"},
	} {
		if err := c.compileEffect("feature:x", e); err == nil {
			t.Errorf("%s: the loader accepted it", name)
		}
	}
	for name, e := range map[string]*Effect{
		"a recharge by level": {Type: "resource", Resource: "x", Max: "1", Recharge: "long_rest", RechargeIf: "level() >= 5", RechargeThen: "short_rest"},
		"a score with a cap":  {Type: "modifier", Target: "score.str", Mode: "add", Value: "4", Cap: 24},
		"a save for all":      {Type: "modifier", Target: "save.all", Mode: "add", Value: "1"},
		"beast spells":        {Type: "beast_spells"},
	} {
		if err := c.compileEffect("feature:x", e); err != nil {
			t.Errorf("%s: %v", name, err)
		}
	}
}

// Flurry of Blows, Patient Defense and Step of the Wind each spend 1 ki point;
// a monk has none of them before level 2, when ki comes.
func TestMonkBonusActionsSpendKi(t *testing.T) {
	t.Parallel()
	c := loadForTest(t)
	d := Derive(standard("class:monk", 3), c)
	for _, key := range []string{"feature:flurry-of-blows", "feature:patient-defense", "feature:step-of-the-wind:disengage", "feature:step-of-the-wind:dash"} {
		i := slices.IndexFunc(d.Actions, func(a Action) bool { return a.Key == key })
		if i < 0 {
			t.Errorf("no %s action: %+v", key, d.Actions)
			continue
		}
		if a := d.Actions[i]; a.Resource != "ki" || a.Economy != EconomyBonusAction {
			t.Errorf("%s = %+v, want a bonus action that spends ki", key, a)
		}
	}
	if i := slices.IndexFunc(d.Actions, func(a Action) bool { return a.Key == "feature:flurry-of-blows" }); i >= 0 && d.Actions[i].NamePT == "Ki" {
		t.Error("the action took the name of the resource it spends")
	}
	for _, a := range Derive(standard("class:monk", 1), c).Actions {
		if a.Resource == "ki" {
			t.Errorf("a level 1 monk has the ki action %s", a.Key)
		}
	}
}

func actionOf(d Derived, key string) (Action, bool) {
	for _, a := range d.Actions {
		if a.Key == key {
			return a, true
		}
	}
	return Action{}, false
}

// TestChannelDivinityAndCuttingWordsSpendTheirUses: the actions that come from
// Channel Divinity and from Bardic Inspiration spend the shared pool, which another
// feature owns (SRD 5.1, Cleric, Channel Divinity; Paladin, Channel Divinity; Bard,
// College of Lore, Cutting Words).
func TestChannelDivinityAndCuttingWordsSpendTheirUses(t *testing.T) {
	t.Parallel()
	c := loadForTest(t)
	cleric := standard("class:cleric", 2)
	cleric.Classes[0].Subclass = "subclass:life"
	paladin := standard("class:paladin", 3)
	paladin.Classes[0].Subclass = "subclass:devotion"
	bard := standard("class:bard", 3)
	bard.Classes[0].Subclass = "subclass:lore"
	for _, tc := range []struct {
		name     string
		build    Build
		action   string
		resource string
	}{
		{"Turn Undead", cleric, "feature:channel-divinity-turn-undead", "channel_divinity"},
		{"Preserve Life", cleric, "feature:channel-divinity-preserve-life", "channel_divinity"},
		{"Sacred Weapon", paladin, "feature:channel-divinity-sacred-weapon", "channel_divinity"},
		{"Turn the Unholy", paladin, "feature:channel-divinity-turn-the-unholy", "channel_divinity"},
		{"Cutting Words", bard, "feature:cutting-words", "bardic_inspiration"},
	} {
		d := Derive(tc.build, c)
		a, ok := actionOf(d, tc.action)
		if !ok {
			t.Errorf("%s: no action %s in %+v", tc.name, tc.action, d.Actions)
			continue
		}
		if a.Resource != tc.resource {
			t.Errorf("%s spends %q, want %q", tc.name, a.Resource, tc.resource)
		}
		if !hasResource(d, tc.resource) {
			t.Errorf("%s: the sheet has no %s resource to spend", tc.name, tc.resource)
		}
	}
}

// TestScalingFeatureListsOnlyItsCurrentTier: a feature that grows with the level is
// on the sheet once, at its current tier (SRD 5.1, Cleric Channel Divinity: once, then
// twice from 6th level, three times from 18th).
func TestScalingFeatureListsOnlyItsCurrentTier(t *testing.T) {
	t.Parallel()
	c := loadForTest(t)
	cleric := func(level int) Derived {
		b := standard("class:cleric", level)
		b.Classes[0].Subclass = "subclass:life"
		return Derive(b, c)
	}
	for _, tc := range []struct {
		d    Derived
		have string
		not  []string
	}{
		{cleric(2), "feature:channel-divinity-1-rest", []string{"feature:channel-divinity-2-rest", "feature:channel-divinity-3-rest"}},
		{cleric(6), "feature:channel-divinity-2-rest", []string{"feature:channel-divinity-1-rest", "feature:channel-divinity-3-rest"}},
		{cleric(18), "feature:channel-divinity-3-rest", []string{"feature:channel-divinity-1-rest", "feature:channel-divinity-2-rest"}},
		{Derive(standard("class:fighter", 11), c), "feature:extra-attack-2", []string{"feature:extra-attack-1"}},
		{Derive(standard("class:fighter", 17), c), "feature:indomitable-3-uses", []string{"feature:indomitable-1-use", "feature:indomitable-2-uses"}},
		{Derive(standard("class:fighter", 17), c), "feature:action-surge-2-uses", []string{"feature:action-surge-1-use"}},
		{Derive(standard("class:bard", 5), c), "feature:bardic-inspiration-d8", []string{"feature:bardic-inspiration-d6"}},
		{Derive(standard("class:bard", 9), c), "feature:song-of-rest-d8", []string{"feature:song-of-rest-d6"}},
		{Derive(standard("class:barbarian", 13), c), "feature:brutal-critical-2-dice", []string{"feature:brutal-critical-1-die"}},
		{Derive(standard("class:ranger", 6), c), "feature:favored-enemy-2-types", []string{"feature:favored-enemy-1-type"}},
		{Derive(standard("class:monk", 9), c), "feature:unarmored-movement-2", []string{"feature:unarmored-movement-1"}},
		{Derive(standard("class:druid", 8), c), "feature:wild-shape-cr-1-or-below", []string{"feature:wild-shape-cr-1-2-or-below-no-flying-speed", "feature:wild-shape-cr-1-4-or-below-no-flying-or-swim-speed"}},
	} {
		if !hasFeature(tc.d, tc.have) {
			t.Errorf("%s is not on the sheet", tc.have)
		}
		for _, k := range tc.not {
			if hasFeature(tc.d, k) {
				t.Errorf("%s stays on the sheet beside %s", k, tc.have)
			}
		}
	}
	// What the lower tier gave still counts: the cleric at 6 has two uses and the fighter at 11 three attacks.
	if got := resourceMax(cleric(6), "channel_divinity"); got != 2 {
		t.Errorf("Channel Divinity uses at cleric 6 = %d, want 2", got)
	}
	if got := Derive(standard("class:fighter", 11), c).AttacksPerAction; got != 3 {
		t.Errorf("attacks at fighter 11 = %d, want 3", got)
	}
}
