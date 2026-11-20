package rules

import (
	"fmt"
	"slices"
	"strings"
	"testing"
	"time"
)

// The warlock's Book of Ancient Secrets (SRD 5.1, Warlock, Eldritch Invocations): two
// 1st-level spells with the ritual tag, from any class list, that are in the Book of
// Shadows, do not count against the spells known and are cast only as rituals.

const (
	bookInvocation = "feature:eldritch-invocation-book-of-ancient-secrets"
	bookKey        = bookInvocation + "#rituals"
)

// bookWarlock is a warlock 5 of the Tome with the invocation and the given picks.
func bookWarlock(picks ...string) Build {
	return choiceBuild("race:human", "", ClassLevel{Class: "class:warlock", Subclass: "subclass:fiend", Level: 5},
		append([]string{"feature:pact-of-the-tome", bookInvocation, "feature:eldritch-invocation-devils-sight"}, picks...)...)
}

func spellOn(d Derived, key string) (CharacterSpell, bool) {
	for _, s := range d.Spells {
		if s.Spell.Key == key {
			return s, true
		}
	}
	return CharacterSpell{}, false
}

func TestTheBookOfAncientSecretsAsksForTwoRitualsOfAnyClass(t *testing.T) {
	t.Parallel()
	c := loadForTest(t)
	ch := choiceOf(t, c.Choices(bookWarlock()), bookKey)
	if ch.Kind != ChoiceKindSpells || ch.Picks != 2 || ch.Missing() != 2 || ch.TitlePT != "Rituais do Livro" || ch.HintPT == "" {
		t.Errorf("the choice = kind %v, picks %d, missing %d, title %q, hint %q; want two spells to pick, titled Rituais do Livro", ch.Kind, ch.Picks, ch.Missing(), ch.TitlePT, ch.HintPT)
	}
	for _, o := range ch.Options {
		if e := c.c.spellEntries[o.Key]; e.Level != 1 || !e.Ritual || e.Archived {
			t.Errorf("option %s is level %d, ritual %v: only 1st-level rituals are offered", o.Key, e.Level, e.Ritual)
		}
	}
	for _, k := range []string{"spell:find-familiar", "spell:detect-magic", "spell:comprehend-languages", "spell:speak-with-animals"} {
		if !slices.Contains(optionKeys(ch), k) {
			t.Errorf("the 1st-level ritual %s of another class list is not offered", k)
		}
	}
	for _, k := range []string{"spell:bless", "spell:charm-person", "spell:light", "spell:augury"} {
		if slices.Contains(optionKeys(ch), k) {
			t.Errorf("%s is not a 1st-level ritual and is offered", k)
		}
	}
	if o := optionOf(t, ch, "spell:detect-magic"); o.Stored != ScopedChoice(bookKey, "spell:detect-magic") || len(o.ClassesPT) == 0 {
		t.Errorf("the option is stored as %q with classes %v", o.Stored, o.ClassesPT)
	}
}

func TestTheBookRitualsAreAskedOnlyWithTheInvocation(t *testing.T) {
	t.Parallel()
	c := loadForTest(t)
	b := bookWarlock()
	b.FeatureChoices = slices.DeleteFunc(b.FeatureChoices, func(k string) bool { return k == bookInvocation })
	if slices.Contains(choiceKeys(c.Choices(b)), bookKey) {
		t.Error("the rituals are asked without the invocation")
	}
	b.FeatureChoices = append(b.FeatureChoices, ScopedChoice(bookKey, "spell:detect-magic"))
	if set := c.Choices(b); !slices.Contains(set.NotOffered, ScopedChoice(bookKey, "spell:detect-magic")) {
		t.Errorf("a pick without the invocation is offered: not offered = %v", set.NotOffered)
	}
	if d := Derive(b, c); func() bool { _, ok := spellOn(d, "spell:detect-magic"); return ok }() {
		t.Error("a pick without the invocation puts the spell on the sheet")
	}
	wizard := choiceBuild("race:human", "", ClassLevel{Class: "class:wizard", Level: 5}, ScopedChoice(bookKey, "spell:detect-magic"))
	if slices.Contains(choiceKeys(c.Choices(wizard)), bookKey) {
		t.Error("a wizard is asked the Book of Ancient Secrets")
	}
}

func TestTheBookRitualsAreKnownNotPreparedAndOutsideTheNumbers(t *testing.T) {
	t.Parallel()
	c := loadForTest(t)
	picks := []string{ScopedChoice(bookKey, "spell:detect-magic"), ScopedChoice(bookKey, "spell:find-familiar")}
	b := bookWarlock(picks...)
	b.Cantrips = []string{"spell:chill-touch", "spell:mage-hand", "spell:prestidigitation"}
	b.SpellsKnown = []string{"spell:charm-person", "spell:expeditious-retreat", "spell:hellish-rebuke", "spell:protection-from-evil-and-good", "spell:illusory-script", "spell:unseen-servant"}
	d := Derive(b, c)
	for _, is := range d.Issues {
		if is.Code == IssueSpellCount || is.Code == IssueSpellNotOnList || is.Code == IssueSpellLevel {
			t.Errorf("issue %s: %s", is.Code, is.Message)
		}
	}
	for _, o := range d.OpenChoices {
		if o.Kind == OpenChoiceSpellsKnown {
			t.Errorf("the book's rituals leave the spells known open: %+v", o)
		}
	}
	for _, k := range []string{"spell:detect-magic", "spell:find-familiar"} {
		cs, ok := spellOn(d, k)
		if !ok || cs.Prepared || !cs.FromBook {
			t.Errorf("%s on the sheet = %+v (found %v), want known, not prepared, from the book", k, cs, ok)
		}
		if got := d.SpellStanding(k, true); got != (SpellStanding{Known: true, CanRitual: true}) {
			t.Errorf("SpellStanding(%s) = %+v, want known and a ritual only", k, got)
		}
	}
	if ch := choiceOf(t, c.Choices(b), bookKey); ch.Missing() != 0 || len(ch.Picked) != 2 {
		t.Errorf("the book's picks: missing %d, picked %v", ch.Missing(), ch.Picked)
	}
}

func TestAWarlockSpellThatIsAlsoInTheBookKeepsItsStanding(t *testing.T) {
	t.Parallel()
	c := loadForTest(t)
	b := bookWarlock(ScopedChoice(bookKey, "spell:comprehend-languages"), ScopedChoice(bookKey, "spell:detect-magic"))
	b.SpellsKnown = []string{"spell:comprehend-languages", "spell:charm-person", "spell:expeditious-retreat", "spell:hellish-rebuke", "spell:protection-from-evil-and-good", "spell:illusory-script"}
	d := Derive(b, c)
	if got := d.SpellStanding("spell:comprehend-languages", true); got != (SpellStanding{Known: true, Prepared: true, CanRitual: true}) {
		t.Errorf("a spell known twice = %+v, want prepared too", got)
	}
	count := 0
	for _, s := range d.Spells {
		if s.Spell.Key == "spell:comprehend-languages" {
			count++
		}
	}
	if count != 1 {
		t.Errorf("the spell is on the sheet %d times", count)
	}
	// A spell known and on the warlock's list is cast as a ritual with the invocation, and not without it.
	if got := d.SpellStanding("spell:charm-person", false); got != (SpellStanding{Known: true, Prepared: true}) {
		t.Errorf("Charm Person = %+v, want an ordinary spell", got)
	}
	plain := b
	plain.FeatureChoices = slices.DeleteFunc(slices.Clone(b.FeatureChoices), func(k string) bool { return k == bookInvocation })
	if got := Derive(plain, c).SpellStanding("spell:comprehend-languages", true); got != (SpellStanding{Known: true, Prepared: true}) {
		t.Errorf("a known ritual without the invocation = %+v, want no ritual", got)
	}
}

// aBookWarlock is a warlock 5 of the Tome with the Book of Ancient Secrets and nothing left open but the book's rituals.
func aBookWarlock(t *testing.T, c *Content) Build {
	t.Helper()
	b := sweepUp(t, c, sweepBase(t, c, "class:warlock", "subclass:fiend"), "class:warlock", "subclass:fiend", 5)
	b.FeatureChoices = slices.DeleteFunc(b.FeatureChoices, func(k string) bool {
		return strings.HasPrefix(k, "feature:pact-of-the-") || strings.HasPrefix(k, "feature:eldritch-invocation-")
	})
	tome := "feature:pact-of-the-tome#cantrips"
	b.FeatureChoices = append(b.FeatureChoices, "feature:pact-of-the-tome", bookInvocation,
		"feature:eldritch-invocation-devils-sight", "feature:eldritch-invocation-beast-speech",
		ScopedChoice(tome, "spell:guidance"), ScopedChoice(tome, "spell:shillelagh"), ScopedChoice(tome, "spell:eldritch-blast"))
	return b
}

// TestTheLevelUpAsksForTheBookRitualsOfAnInvocationAlreadyTaken: the rituals are open as
// soon as the sheet has the invocation, so the next level asks for them first, as it asks
// for the Tome's cantrips, and refuses the level without them.
func TestTheLevelUpAsksForTheBookRitualsOfAnInvocationAlreadyTaken(t *testing.T) {
	t.Parallel()
	c := loadForTest(t)
	b := aBookWarlock(t, c)
	o, err := LevelUpOptions(b, "class:warlock", c)
	if err != nil {
		t.Fatal(err)
	}
	if !offersChoice(o.LateChoices, bookKey) {
		t.Fatalf("late choices = %+v, want the Book of Ancient Secrets' rituals", o.LateChoices)
	}
	ch := satisfy(t, c, b, "class:warlock", "")
	var picked []string
	for _, k := range ch.LateChoices {
		if choice, value, ok := SplitScopedChoice(k); ok && choice == bookKey {
			picked = append(picked, value)
		}
	}
	if len(picked) != 2 {
		t.Fatalf("the picks %v answer %d rituals, want 2", ch.LateChoices, len(picked))
	}
	if err := CheckLevelUp(b, mustApply(t, c, b, ch), c); err != nil {
		t.Fatalf("CheckLevelUp with the rituals picked: %v", err)
	}
	none := ch
	none.LateChoices = without(ch.LateChoices, bookKey)
	wantRefusal(t, CheckLevelUp(b, mustApply(t, c, b, none), c), LevelUpReasonLateChoice, "full.feature_choice_keys")
}

// TestSwappingTheBookAwayTakesItsRitualsWithIt: the rituals belong to the invocation, so a
// warlock that swaps it at a level keeps no pick of a choice the sheet no longer has, and the
// swap is still one change; both ways of swapping say the same.
func TestSwappingTheBookAwayTakesItsRitualsWithIt(t *testing.T) {
	t.Parallel()
	c := loadForTest(t)
	b := aBookWarlock(t, c)
	b.FeatureChoices = append(b.FeatureChoices, ScopedChoice(bookKey, "spell:detect-magic"), ScopedChoice(bookKey, "spell:find-familiar"))
	const to = "feature:eldritch-invocation-armor-of-shadows"
	ways := map[string]func(ch *LevelUpChoices){
		"a swap of the pick": func(ch *LevelUpChoices) {
			ch.Swap = PickSwap{Group: SwapGroupInvocation, From: bookInvocation, To: to}
		},
		"the invocation given up": func(ch *LevelUpChoices) {
			ch.SwapInvocation = bookInvocation
			ch.FeatureChoices = append(ch.FeatureChoices, to)
		},
	}
	for name, way := range ways {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			ch := satisfy(t, c, b, "class:warlock", "")
			way(&ch)
			after := mustApply(t, c, b, ch)
			if err := CheckLevelUp(b, after, c); err != nil {
				t.Fatalf("CheckLevelUp: %v", err)
			}
			for _, k := range after.FeatureChoices {
				if k == bookInvocation || strings.HasPrefix(k, bookInvocation+textSeparator) {
					t.Errorf("the sheet keeps %q after the book was swapped away", k)
				}
			}
			if !slices.Contains(after.FeatureChoices, to) {
				t.Errorf("the invocation swapped in is missing: %v", after.FeatureChoices)
			}
		})
	}
}

// TestASwitchedOffRitualIsNotOfferedToAPlayerInTheBook: the table's switches apply to the book's choice
// (RN-23). A sheet that holds the pick keeps it, and the master reads it all the same.
func TestASwitchedOffRitualIsNotOfferedToAPlayerInTheBook(t *testing.T) {
	t.Parallel()
	const off = "spell:detect-magic"
	c := withOverlayOn(t, loadForTest(t), Overlay{Revision: 1, Off: []string{off}})
	offered := func(c *Content, b Build) bool {
		return slices.Contains(optionKeys(choiceOf(t, c.Choices(b), bookKey)), off)
	}
	if offered(c.ForPlayer(), bookWarlock()) {
		t.Errorf("a player is offered %s, which the master switched off", off)
	}
	if !offered(c, bookWarlock()) {
		t.Errorf("the master is not offered %s", off)
	}
	held := bookWarlock(ScopedChoice(bookKey, off))
	ch := choiceOf(t, c.ForPlayer().Choices(held), bookKey)
	if !slices.Contains(ch.Picked, off) || ch.Missing() != 1 {
		t.Errorf("a sheet that picked %s: picked %v, missing %d; want the pick kept and one missing", off, ch.Picked, ch.Missing())
	}
	if !slices.Contains(optionKeys(choiceOf(t, c.ForPlayer().Choices(bookWarlock()), bookKey)), "spell:find-familiar") {
		t.Error("a ritual that is not switched off is missing")
	}
}

// TestAPickOfTheBookIsNotRemovedAlone: only the invocation takes its picks away; dropping one pick at a level is refused.
func TestAPickOfTheBookIsNotRemovedAlone(t *testing.T) {
	t.Parallel()
	c := loadForTest(t)
	b := aBookWarlock(t, c)
	b.FeatureChoices = append(b.FeatureChoices, ScopedChoice(bookKey, "spell:detect-magic"), ScopedChoice(bookKey, "spell:find-familiar"))
	after := mustApply(t, c, b, satisfy(t, c, b, "class:warlock", ""))
	after.FeatureChoices = slices.DeleteFunc(after.FeatureChoices, func(k string) bool { return k == ScopedChoice(bookKey, "spell:detect-magic") })
	wantRefusal(t, CheckLevelUp(b, after, c), LevelUpReasonFeatureChoice, "full.feature_choice_keys")
}

// bookTaker is a warlock of the Tome at the given level, with two invocations and no book.
func bookTaker(t *testing.T, c *Content, level int) Build {
	t.Helper()
	b := sweepUp(t, c, sweepBase(t, c, "class:warlock", "subclass:fiend"), "class:warlock", "subclass:fiend", level)
	b.FeatureChoices = slices.DeleteFunc(b.FeatureChoices, func(k string) bool {
		return strings.HasPrefix(k, "feature:pact-of-the-") || strings.HasPrefix(k, "feature:eldritch-invocation-")
	})
	tome := "feature:pact-of-the-tome#cantrips"
	b.FeatureChoices = append(b.FeatureChoices, "feature:pact-of-the-tome", "feature:eldritch-invocation-devils-sight",
		ScopedChoice(tome, "spell:guidance"), ScopedChoice(tome, "spell:shillelagh"), ScopedChoice(tome, "spell:eldritch-blast"))
	return b
}

// TestTheBookTakenWithoutItsRitualsIsAskedAtTheNextLevel: a plain option picked at a level
// opens its choice, and the level does not have to answer it; the next level asks it as a late
// choice and refuses to go on without it. The level-up lists only the choices of the sheet
// before and of the bare new level, so a pick made in the same request is the only way to
// answer it now.
func TestTheBookTakenWithoutItsRitualsIsAskedAtTheNextLevel(t *testing.T) {
	t.Parallel()
	c := loadForTest(t)
	ways := []struct {
		name  string
		level int
		take  func(ch *LevelUpChoices)
	}{
		{"as the new invocation", 4, func(ch *LevelUpChoices) {
			ch.FeatureChoices = slices.DeleteFunc(ch.FeatureChoices, c.c.isInvocation)
			ch.FeatureChoices = append(ch.FeatureChoices, bookInvocation)
		}},
		{"in place of an invocation given up", 5, func(ch *LevelUpChoices) {
			ch.FeatureChoices = slices.DeleteFunc(ch.FeatureChoices, c.c.isInvocation)
			ch.SwapInvocation = "feature:eldritch-invocation-devils-sight"
			ch.FeatureChoices = append(ch.FeatureChoices, bookInvocation)
		}},
	}
	for _, way := range ways {
		t.Run(way.name, func(t *testing.T) {
			t.Parallel()
			b := bookTaker(t, c, way.level)
			if way.level == 5 {
				b.FeatureChoices = append(b.FeatureChoices, "feature:eldritch-invocation-beast-speech")
			}
			ch := satisfy(t, c, b, "class:warlock", "")
			way.take(&ch)
			after := mustApply(t, c, b, ch)
			if !slices.Contains(after.FeatureChoices, bookInvocation) {
				t.Fatalf("the sheet after the level lacks the book: %v", after.FeatureChoices)
			}
			if err := CheckLevelUp(b, after, c); err != nil {
				t.Fatalf("the level that takes the book without its rituals: %v", err)
			}
			o, err := LevelUpOptions(after, "class:warlock", c)
			if err != nil {
				t.Fatal(err)
			}
			if !offersChoice(o.LateChoices, bookKey) {
				t.Fatalf("the next level's late choices = %+v, want the book's rituals", o.LateChoices)
			}
			next := satisfy(t, c, after, "class:warlock", "")
			if !slices.ContainsFunc(next.LateChoices, func(k string) bool { choice, _, ok := SplitScopedChoice(k); return ok && choice == bookKey }) {
				t.Fatalf("the next level's picks %v do not answer the rituals", next.LateChoices)
			}
			if err := CheckLevelUp(after, mustApply(t, c, after, next), c); err != nil {
				t.Fatalf("the next level with the rituals: %v", err)
			}
			none := next
			none.LateChoices = without(next.LateChoices, bookKey)
			wantRefusal(t, CheckLevelUp(after, mustApply(t, c, after, none), c), LevelUpReasonLateChoice, "full.feature_choice_keys")
		})
	}
}

// TestThePactOfTheTomeTakenWithoutItsCantripsIsAskedAtTheNextLevel is the same rule for the
// Pact Boon of level 3: the cantrips of the Tome stay open and the next level asks them.
func TestThePactOfTheTomeTakenWithoutItsCantripsIsAskedAtTheNextLevel(t *testing.T) {
	t.Parallel()
	c := loadForTest(t)
	b := sweepUp(t, c, sweepBase(t, c, "class:warlock", "subclass:fiend"), "class:warlock", "subclass:fiend", 2)
	ch := satisfy(t, c, b, "class:warlock", "")
	ch.FeatureChoices = slices.DeleteFunc(ch.FeatureChoices, func(k string) bool { return strings.HasPrefix(k, "feature:pact-of-the-") })
	ch.FeatureChoices = append(ch.FeatureChoices, "feature:pact-of-the-tome")
	after := mustApply(t, c, b, ch)
	if err := CheckLevelUp(b, after, c); err != nil {
		t.Fatalf("the level that takes the Pact of the Tome without its cantrips: %v", err)
	}
	const tomeKey = "feature:pact-of-the-tome#cantrips"
	o, err := LevelUpOptions(after, "class:warlock", c)
	if err != nil {
		t.Fatal(err)
	}
	if !offersChoice(o.LateChoices, tomeKey) {
		t.Fatalf("the next level's late choices = %+v, want the Tome's cantrips", o.LateChoices)
	}
	next := satisfy(t, c, after, "class:warlock", "")
	if err := CheckLevelUp(after, mustApply(t, c, after, next), c); err != nil {
		t.Fatalf("the next level with the cantrips: %v", err)
	}
	none := next
	none.LateChoices = without(next.LateChoices, tomeKey)
	wantRefusal(t, CheckLevelUp(after, mustApply(t, c, after, none), c), LevelUpReasonLateChoice, "full.feature_choice_keys")
}

// TestAChangedTableRitualOfTheBookIsNamedByChangedSince: the ritual the warlock holds in the book is a key
// of the sheet at the pick, so ChangedSince names it after the master changed it, and still does after the
// master switched it off.
func TestAChangedTableRitualOfTheBookIsNamedByChangedSince(t *testing.T) {
	t.Parallel()
	sp := tableTestSpells()[0]
	sp.Ritual = true
	sp.Revision, sp.ChangedAt = 3, time.Now()
	b := bookWarlock(ScopedChoice(bookKey, "spell:detect-magic"), ScopedChoice(bookKey, sp.Key))
	for name, o := range map[string]Overlay{
		"a ritual of the table":   {Revision: 3, Spells: []TableSpell{sp}},
		"switched off afterwards": {Revision: 3, Spells: []TableSpell{sp}, Off: []string{sp.Key}},
	} {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			c := withOverlay(t, o)
			want := fmt.Sprintf("full.feature_choice_keys[%d]", slices.Index(b.FeatureChoices, ScopedChoice(bookKey, sp.Key)))
			ch := c.ChangedSince(b, 2, nil)
			if len(ch) != 1 || ch[0].Key != sp.Key || ch[0].Field != want {
				t.Errorf("ChangedSince = %+v, want the ritual at %s", ch, want)
			}
			if got := c.ChangedSince(b, 3, nil); len(got) != 0 {
				t.Errorf("a sheet saved at the revision of the change is told of %+v", got)
			}
			if !c.SheetHas(b, sp.Key) {
				t.Error("the sheet that holds the ritual does not have it")
			}
		})
	}
}

// TestAWarlockSpellTheCharacterKnowsIsOneOnTheirWarlockList: the Book of Ancient Secrets casts the warlock
// spells the character knows as rituals, and the character's warlock list is the class's own with the
// patron's expanded list. A spell known from a list that is not the warlock's (here a sorcerer's) is not one.
func TestAWarlockSpellTheCharacterKnowsIsOneOnTheirWarlockList(t *testing.T) {
	t.Parallel()
	const patron = "subclass:patrono-do-livro" + tableSuffix
	const ritual = "spell:detect-magic" // a ritual of the 1st level that is not on the warlock's list
	c := withOverlay(t, Overlay{Revision: 1, Subclasses: []TableSubclass{{
		TableEntry: TableEntry{Key: patron, NamePT: "Patrono do Livro"}, Class: "class:warlock",
		AlwaysPrepared: []TableAlwaysPrepared{{ClassLevel: 1, Spell: ritual}}, SpellsExpanded: true,
	}}})
	known := func(b Build) Build {
		b.SpellsKnown = []string{ritual, "spell:charm-person"}
		return b
	}
	standing := func(t *testing.T, b Build) SpellStanding {
		t.Helper()
		return Derive(known(b), c).SpellStanding(ritual, true)
	}
	t.Run("the patron's expanded list counts", func(t *testing.T) {
		t.Parallel()
		b := bookWarlock()
		b.Classes[0].Subclass = patron
		if got := standing(t, b); !got.CanRitual {
			t.Errorf("a spell on the patron's expanded list = %+v, want a ritual", got)
		}
	})
	t.Run("a spell off the warlock list does not", func(t *testing.T) {
		t.Parallel()
		if got := standing(t, bookWarlock()); got.CanRitual {
			t.Errorf("a spell off the warlock's list = %+v, want no ritual", got)
		}
	})
	t.Run("a spell of another class's list does not", func(t *testing.T) {
		t.Parallel()
		b := bookWarlock()
		b.Classes = []ClassLevel{{Class: "class:warlock", Subclass: "subclass:fiend", Level: 5}, {Class: "class:sorcerer", Level: 1}}
		if got := standing(t, b); got.CanRitual {
			t.Errorf("a sorcerer's spell off the warlock's list = %+v, want no ritual", got)
		}
	})
	t.Run("a spell of the warlock list does", func(t *testing.T) {
		t.Parallel()
		b := bookWarlock()
		b.SpellsKnown = []string{"spell:comprehend-languages"}
		if got := Derive(b, c).SpellStanding("spell:comprehend-languages", true); !got.CanRitual {
			t.Errorf("a warlock spell known = %+v, want a ritual", got)
		}
	})
}

// TestSwappingThePactOfTheTomeAwayTakesItsCantripsWithIt: the Tome's three cantrips belong to the Pact Boon, so the
// table's swap of the Pact Boon at an Ability Score Improvement gives them up with it, and the swap is still one change.
func TestSwappingThePactOfTheTomeAwayTakesItsCantripsWithIt(t *testing.T) {
	t.Parallel()
	c := loadForTest(t)
	const tome, chain, tomeCantrips = "feature:pact-of-the-tome", "feature:pact-of-the-chain", "feature:pact-of-the-tome#cantrips"
	b := bookTaker(t, c, 3)
	o, err := LevelUpOptions(b, "class:warlock", c)
	if err != nil {
		t.Fatal(err)
	}
	if !slices.ContainsFunc(o.PickSwaps, func(g SwapGroup) bool { return g.Group == SwapGroupPactBoon }) {
		t.Fatalf("level 4 offers swaps %v, want the Pact Boon", o.PickSwaps)
	}
	ch := satisfy(t, c, b, "class:warlock", "")
	ch.Swap = PickSwap{Group: SwapGroupPactBoon, From: tome, To: chain}
	after := mustApply(t, c, b, ch)
	if err := CheckLevelUp(b, after, c); err != nil {
		t.Fatalf("CheckLevelUp: %v", err)
	}
	for _, k := range after.FeatureChoices {
		if k == tome || strings.HasPrefix(k, tomeCantrips+"=") {
			t.Errorf("the sheet keeps %q after the Pact of the Tome was swapped away", k)
		}
	}
	if !slices.Contains(after.FeatureChoices, chain) {
		t.Errorf("the Pact Boon swapped in is missing: %v", after.FeatureChoices)
	}
	if offersChoice(c.Choices(after).Groups, tomeCantrips) {
		t.Error("the sheet still asks the Tome's cantrips")
	}
}
