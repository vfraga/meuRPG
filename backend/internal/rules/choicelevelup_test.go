package rules

import (
	"slices"
	"strings"
	"testing"
)

// without removes the picks that have this prefix from a list of keys.
func without(keys []string, prefix string) []string {
	return slices.DeleteFunc(slices.Clone(keys), func(k string) bool { return strings.HasPrefix(k, prefix) })
}

// kaelith is a ranger 5 (Hunter) that picked everything but the Fighting Style of level
// 2, the case of a sheet written before the style was asked.
func kaelith(t *testing.T, c *Content) Build {
	t.Helper()
	b := sweepUp(t, c, sweepBase(t, c, "class:ranger", "subclass:hunter"), "class:ranger", "subclass:hunter", 5)
	b.FeatureChoices = without(b.FeatureChoices, "feature:ranger-fighting-style-")
	return b
}

// TestTheLevelUpAsksFirstForWhatAnEarlierLevelLeftOpen: the offer lists the open
// choices of the earlier levels apart from the new level's, and the check refuses the
// level without them, with the reason that tells the two apart.
func TestTheLevelUpAsksFirstForWhatAnEarlierLevelLeftOpen(t *testing.T) {
	t.Parallel()
	c := loadForTest(t)
	b := kaelith(t, c)
	o, err := LevelUpOptions(b, "class:ranger", c)
	if err != nil {
		t.Fatal(err)
	}
	if !offersChoice(o.LateChoices, "feature:ranger-fighting-style") || len(o.LateChoices) != 1 || len(o.LateChoices[0].Choices) != 1 {
		t.Errorf("late choices = %+v, want only the Fighting Style of level 2", o.LateChoices)
	}
	if g := o.LateChoices[0]; g.Level != 2 || g.SourceKey != "class:ranger" {
		t.Errorf("the late style came from %s level %d, want the ranger's level 2", g.SourceKey, g.Level)
	}
	for _, key := range []string{"feature:favored-enemy-2-types", "feature:favored-enemy-2-types#language", "feature:natural-explorer-2-terrain-types"} {
		if !offersChoice(o.NewChoices, key) {
			t.Errorf("the level 6 choices lack %s: %v", key, o.NewChoices)
		}
	}
	if offersChoice(o.NewChoices, "feature:ranger-fighting-style") {
		t.Error("the late style is also offered as a new choice")
	}

	ch := satisfy(t, c, b, "class:ranger", "")
	if !slices.ContainsFunc(ch.LateChoices, func(k string) bool { return strings.HasPrefix(k, "feature:ranger-fighting-style-") }) {
		t.Fatalf("the picks %v do not answer the late style", ch.LateChoices)
	}
	if err := CheckLevelUp(b, mustApply(t, c, b, ch), c); err != nil {
		t.Fatalf("CheckLevelUp with everything answered: %v", err)
	}

	noStyle := ch
	noStyle.LateChoices = without(ch.LateChoices, "feature:ranger-fighting-style-")
	wantRefusal(t, CheckLevelUp(b, mustApply(t, c, b, noStyle), c), LevelUpReasonLateChoice, "full.feature_choice_keys")

	noTerrain := ch
	noTerrain.LateChoices = without(ch.LateChoices, "feature:natural-explorer-2-terrain-types=")
	wantRefusal(t, CheckLevelUp(b, mustApply(t, c, b, noTerrain), c), LevelUpReasonFeatureChoice, "full.feature_choice_keys")

	extra := ch
	extra.LateChoices = append(slices.Clone(ch.LateChoices), ScopedChoice("feature:favored-enemy-3-enemies", "creature-type:beast"))
	wantRefusal(t, CheckLevelUp(b, mustApply(t, c, b, extra), c), LevelUpReasonFeatureChoice, "full.feature_choice_keys")
}

// TestAFavoredEnemyAtLevelSixIsAskedWithItsLanguage: SRD 5.1, Ranger, Favored Enemy:
// at 6th level another enemy and another language.
func TestAFavoredEnemyAtLevelSixIsAskedWithItsLanguage(t *testing.T) {
	t.Parallel()
	c := loadForTest(t)
	b := sweepUp(t, c, sweepBase(t, c, "class:ranger", "subclass:hunter"), "class:ranger", "subclass:hunter", 5)
	ch := satisfy(t, c, b, "class:ranger", "")
	if len(ch.LateChoices) != 3 {
		t.Fatalf("picks %v, want the second enemy, its language and the second terrain", ch.LateChoices)
	}
	after := mustApply(t, c, b, ch)
	if err := CheckLevelUp(b, after, c); err != nil {
		t.Fatal(err)
	}
	if d := Derive(after, c); len(d.Issues) != 0 {
		t.Errorf("issues: %v", d.Issues)
	}
	// The level asks the same of the master: nothing is skipped for an NPC.
	ch.LateChoices = nil
	wantRefusal(t, CheckLevelUp(b, mustApply(t, c, b, ch), c), LevelUpReasonFeatureChoice, "full.feature_choice_keys")
}

// TestAWarlockSwapsOneInvocationAtALevel: SRD 5.1, Warlock, Eldritch Invocations: when
// the warlock gains a level, one invocation may be replaced by another. The Pact Boon
// and a second invocation cannot go.
func TestAWarlockSwapsOneInvocationAtALevel(t *testing.T) {
	t.Parallel()
	c := loadForTest(t)
	b := sweepUp(t, c, sweepBase(t, c, "class:warlock", "subclass:fiend"), "class:warlock", "subclass:fiend", 4)
	var have []string
	for _, k := range b.FeatureChoices {
		if c.c.isInvocation(k) {
			have = append(have, k)
		}
	}
	if len(have) != 2 {
		t.Fatalf("a warlock 4 knows %v, want two invocations", have)
	}
	o, _ := LevelUpOptions(b, "class:warlock", c)
	if !o.CanSwapInvocation {
		t.Error("the warlock's offer does not allow a swap")
	}
	if o2, _ := LevelUpOptions(sweepBase(t, c, "class:fighter", ""), "class:fighter", c); o2.CanSwapInvocation {
		t.Error("a fighter can swap an invocation")
	}
	ch := satisfy(t, c, b, "class:warlock", "subclass:fiend")
	fresh := slices.DeleteFunc(slices.Clone(ch.FeatureChoices), func(k string) bool { return !c.c.isInvocation(k) })
	if len(fresh) != 1 {
		t.Fatalf("level 5 picks %v, want one new invocation", fresh)
	}
	// Invocations the sheet can take besides the ones it has.
	var takable []string
	for _, op := range choiceOf(t, c.c.choiceSet(mustApply(t, c, b, ch)), "feature:eldritch-invocations").Options {
		if !op.Blocked() && !slices.Contains(have, op.Key) && !slices.Contains(fresh, op.Key) && !slices.ContainsFunc(c.c.effects[op.Key], func(e *Effect) bool { return e.Type == "proficiency" }) {
			takable = append(takable, op.Key)
		}
	}
	if len(takable) < 2 {
		t.Fatalf("takable invocations: %v", takable)
	}
	swapIn := takable[0]
	swap := ch
	swap.SwapInvocation = have[0]
	swap.FeatureChoices = append(slices.Clone(ch.FeatureChoices), swapIn)
	after := mustApply(t, c, b, swap)
	if err := CheckLevelUp(b, after, c); err != nil {
		t.Fatalf("a swap of one invocation: %v", err)
	}
	if slices.Contains(after.FeatureChoices, have[0]) || !slices.Contains(after.FeatureChoices, swapIn) {
		t.Errorf("after the swap the picks are %v", after.FeatureChoices)
	}

	// Two swapped is one too many; the Pact Boon never goes.
	twoAfter := mustApply(t, c, b, ch)
	twoAfter.FeatureChoices = slices.DeleteFunc(twoAfter.FeatureChoices, func(k string) bool { return k == have[0] || k == have[1] })
	twoAfter.FeatureChoices = append(twoAfter.FeatureChoices, takable[0], takable[1])
	wantRefusal(t, CheckLevelUp(b, twoAfter, c), LevelUpReasonFeatureChoice, "full.feature_choice_keys")

	pact := swap
	pact.SwapInvocation = "feature:pact-of-the-chain"
	wantRefusal(t, CheckLevelUp(b, mustApply(t, c, b, pact), c), LevelUpReasonFeatureChoice, "full.feature_choice_keys")

	// A swap for an invocation the sheet cannot take is refused as an unmet prerequisite.
	blocked := swap
	blocked.FeatureChoices = append(slices.Clone(ch.FeatureChoices), "feature:eldritch-invocation-lifedrinker")
	wantRefusal(t, CheckLevelUp(b, mustApply(t, c, b, blocked), c), LevelUpReasonFeatureChoice, "full.feature_choice_keys")
}

// TestTheOfferKeepsAnInvocationThatCannotBeTakenApartWithItsReason: the guided
// level-up lists the options the sheet can take and, apart, the ones it cannot, with
// what is missing.
func TestTheOfferKeepsAnInvocationThatCannotBeTakenApartWithItsReason(t *testing.T) {
	t.Parallel()
	c := loadForTest(t)
	b := sweepUp(t, c, sweepBase(t, c, "class:warlock", "subclass:fiend"), "class:warlock", "subclass:fiend", 4)
	o, _ := LevelUpOptions(b, "class:warlock", c)
	var inv LevelUpFeatureChoice
	for _, fc := range o.FeatureChoices {
		if fc.Feature.Key == invocationsFeature {
			inv = fc
		}
	}
	if inv.Choose != 1 || len(inv.Options) == 0 || len(inv.Blocked) == 0 {
		t.Fatalf("invocations offer = %+v, want options and blocked ones", inv)
	}
	for _, blocked := range inv.Blocked {
		if blocked.ReasonPT == "" || slices.ContainsFunc(inv.Options, func(k NamedKey) bool { return k.Key == blocked.Key }) {
			t.Errorf("blocked option %+v is also listed as available, or has no reason", blocked)
		}
	}
	if !slices.ContainsFunc(inv.Blocked, func(bo LevelUpBlockedOption) bool {
		return bo.Key == "feature:eldritch-invocation-ascendant-step" && strings.Contains(bo.ReasonPT, "nível 9")
	}) {
		t.Errorf("blocked = %+v, want Ascendant Step (level 9) among them", inv.Blocked)
	}
}

// TestTheLandDruidPicksTheTerrainAtLevelTwo: SRD 5.1, Druid, Circle of the Land: at 2nd
// level the druid chooses a terrain and learns one more druid cantrip. The data lists
// the terrain among the level's features.
func TestTheLandDruidPicksTheTerrainAtLevelTwo(t *testing.T) {
	t.Parallel()
	c := loadForTest(t)
	b := sweepBase(t, c, "class:druid", "subclass:land")
	o, err := LevelUpOptions(b, "class:druid", c)
	if err != nil {
		t.Fatal(err)
	}
	var land *LevelUpSubclass
	for i := range o.Subclasses {
		if o.Subclasses[i].Key == "subclass:land" {
			land = &o.Subclasses[i]
		}
	}
	if land == nil || len(land.FeatureChoices) != 1 || land.FeatureChoices[0].Feature.Key != "feature:circle-of-the-land" || len(land.FeatureChoices[0].Options) != 7 || land.Cantrips != 1 {
		t.Fatalf("Land at level 2 = %+v, want the terrain (7 options) and one cantrip", land)
	}
	ch := satisfy(t, c, b, "class:druid", "subclass:land")
	after := mustApply(t, c, b, ch)
	if err := CheckLevelUp(b, after, c); err != nil {
		t.Fatal(err)
	}
	// Without the terrain the level is refused.
	ch.FeatureChoices = nil
	wantRefusal(t, CheckLevelUp(b, mustApply(t, c, b, ch), c), LevelUpReasonFeatureChoice, "full.feature_choice_keys")
	// The bonus cantrip comes with the cantrips and counts as the feature's choice.
	if bonus := choiceOf(t, c.Choices(after), "feature:bonus-cantrip"); bonus.Missing() != 0 {
		t.Errorf("the bonus cantrip is still open after the level: %+v", bonus)
	}
}

// TestTheLoreBardLearnsTwoMoreSecrets: SRD 5.1, Bard, College of Lore, Additional
// Magical Secrets (6th level): two spells from any class, outside the number the
// bard knows.
func TestTheLoreBardLearnsTwoMoreSecrets(t *testing.T) {
	t.Parallel()
	c := loadForTest(t)
	b := sweepUp(t, c, sweepBase(t, c, "class:bard", "subclass:lore"), "class:bard", "subclass:lore", 5)
	o, err := LevelUpOptions(b, "class:bard", c)
	if err != nil {
		t.Fatal(err)
	}
	// The table's spells known go from 8 at level 5 to 9 at level 6 (SRD 5.1, Bard
	// table): one more, and the two secrets besides.
	if o.AnyClassSpells != 2 || o.Spells != 3 || len(o.MasterAdds) != 0 {
		t.Errorf("Lore at 6: %d new spells, %d of any class, master adds %v; want 3, 2 and none", o.Spells, o.AnyClassSpells, o.MasterAdds)
	}
	ch := satisfy(t, c, b, "class:bard", "subclass:lore")
	after := mustApply(t, c, b, ch)
	if err := CheckLevelUp(b, after, c); err != nil {
		t.Fatalf("a level with the two secrets: %v", err)
	}
	if d := Derive(after, c); len(d.Issues) != 0 {
		t.Errorf("issues: %v", d.Issues)
	}
	// A third spell off the Bard's list is one too many.
	ch.Spells = pickSpells(c, "class:bard", 1, 3, b.SpellsKnown, 3, true)[len(b.SpellsKnown):]
	wantRefusal(t, CheckLevelUp(b, mustApply(t, c, b, ch), c), LevelUpReasonSpells, "full.known_spell_keys")
}

// TestTheFiendLearnsItsPatronSpellsAtLevelUp: the patron's expanded list (SRD 5.1, The
// Fiend) is part of the warlock's list, so a new known spell from it is not "off the
// list" and uses none of the spells of another class.
func TestTheFiendLearnsItsPatronSpellsAtLevelUp(t *testing.T) {
	t.Parallel()
	c := loadForTest(t)
	b := sweepBase(t, c, "class:warlock", "subclass:fiend")
	ch := satisfy(t, c, b, "class:warlock", "subclass:fiend")
	if len(ch.Spells) != 1 {
		t.Fatalf("level 2 gives %v, want one new spell", ch.Spells)
	}
	ch.Spells = []string{"spell:burning-hands"}
	after := mustApply(t, c, b, ch)
	if err := CheckLevelUp(b, after, c); err != nil {
		t.Fatalf("a patron spell at level up: %v", err)
	}
	if d := Derive(after, c); len(d.Issues) != 0 {
		t.Errorf("issues: %v", d.Issues)
	}
	// A spell of another class's list, with no patron behind it, is still refused.
	ch.Spells = []string{"spell:cure-wounds"}
	wantRefusal(t, CheckLevelUp(b, mustApply(t, c, b, ch), c), LevelUpReasonSpells, "full.known_spell_keys")
}

// TestAMulticlassLevelAsksTheChoicesOfTheNewClassesFirstLevel: the class step comes first,
// then the choices of the class taken, a new class's level 1 included (SRD 5.1,
// "Multiclassing": a class taken later gives its level 1 features but the starting
// proficiencies). A Wizard 5 that takes Fighter 1 picks a Fighting Style, one that takes
// Ranger 1 picks a Favored Enemy and a terrain, and a Fighter 5 that takes Warlock 1 picks
// nothing (the Pact Boon comes at Warlock 3). The level is refused without them, and
// accepted with them, leaving no choice open.
func TestAMulticlassLevelAsksTheChoicesOfTheNewClassesFirstLevel(t *testing.T) {
	t.Parallel()
	c := loadForTest(t)
	subOf := func(key string) string {
		if subs := c.c.classes[key].Subclasses; len(subs) > 0 {
			return subs[0]
		}
		return ""
	}
	base := func(class string) Build {
		b := sweepBase(t, c, class, subOf(class))
		b.BaseScores = map[Ability]int{STR: 14, DEX: 14, CON: 14, INT: 14, WIS: 14, CHA: 14}
		return sweepUp(t, c, b, class, subOf(class), 5)
	}
	for _, tc := range []struct {
		from, to string
		asks     bool
	}{
		{"class:wizard", "class:fighter", true},
		{"class:wizard", "class:ranger", true},
		{"class:fighter", "class:warlock", false},
	} {
		t.Run(tc.from+"+"+tc.to, func(t *testing.T) {
			t.Parallel()
			before := base(tc.from)
			o, err := LevelUpOptions(before, tc.to, c)
			if err != nil {
				t.Fatalf("LevelUpOptions: %v", err)
			}
			if !o.NewClass {
				t.Fatalf("%s is not a new class of the character", tc.to)
			}
			if len(o.LateChoices) != 0 {
				t.Errorf("late choices = %+v, want none: the sheet left nothing open", o.LateChoices)
			}
			asked := len(o.FeatureChoices) + len(o.NewChoices)
			if (asked > 0) != tc.asks {
				t.Fatalf("the level asks %d choices (feature options %d, new choices %d), want some: %v", asked, len(o.FeatureChoices), len(o.NewChoices), tc.asks)
			}
			ch := satisfy(t, c, before, tc.to, subOf(tc.to))
			if tc.asks {
				// Without the picks the level is refused, with the reason of the choices.
				bare := ch
				bare.FeatureChoices, bare.LateChoices = nil, nil
				without, aerr := ApplyLevelUp(before, bare, c)
				if aerr != nil {
					t.Fatalf("ApplyLevelUp without the picks: %v", aerr)
				}
				wantRefusal(t, CheckLevelUp(before, without, c), LevelUpReasonFeatureChoice, "full.feature_choice_keys")
			}
			after, err := ApplyLevelUp(before, ch, c)
			if err != nil {
				t.Fatalf("ApplyLevelUp: %v", err)
			}
			if err := CheckLevelUp(before, after, c); err != nil {
				t.Fatalf("CheckLevelUp: %v\nchoices: %+v", err, ch)
			}
			if n := c.c.choiceSet(after).PendingCount(); n != 0 {
				t.Errorf("%d choices still open after the level: %+v", n, c.c.choiceSet(after).Pending())
			}
		})
	}
}

// A level-up of a class that is not the Warlock refuses an invocation swap, whether the sheet holds the key sent or not.
func TestALevelUpOfANonWarlockRefusesAnInvocationSwap(t *testing.T) {
	t.Parallel()
	c := loadForTest(t)
	b := sweepBase(t, c, "class:fighter", "")
	ch := satisfy(t, c, b, "class:fighter", "")
	keys := map[string]string{"a key the sheet holds": "", "a key the sheet does not hold": "feature:eldritch-invocation-devils-sight"}
	if len(b.FeatureChoices) == 0 {
		t.Fatal("the fighter holds no feature choice")
	}
	keys["a key the sheet holds"] = b.FeatureChoices[0]
	for name, key := range keys {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			swap := ch
			swap.SwapInvocation = key
			after := mustApply(t, c, b, swap)
			wantRefusal(t, CheckLevelUpOf(b, after, swap, c), LevelUpReasonFeatureChoice, "full.feature_choice_keys")
		})
	}
}
