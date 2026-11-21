package rules

import (
	"slices"
	"strconv"
	"strings"
	"testing"
)

// stamped is a fresh genOverlay whose entries all carry a content revision, as
// the server's do: the issues are tied to an entry only when the content knows
// the entries' revisions.
func stamped(t *testing.T, srd *Content) Overlay {
	t.Helper()
	o := genOverlay(t, srd)
	for i := range o.Classes {
		o.Classes[i].Revision = 1
	}
	for i := range o.Subclasses {
		o.Subclasses[i].Revision = 1
	}
	return o
}

func classOf(o *Overlay, key string) *TableClass {
	for i := range o.Classes {
		if o.Classes[i].Key == key {
			return &o.Classes[i]
		}
	}
	panic("no class " + key)
}

func subclassOf2(o *Overlay, key string) *TableSubclass {
	for i := range o.Subclasses {
		if o.Subclasses[i].Key == key {
			return &o.Subclasses[i]
		}
	}
	panic("no subclass " + key)
}

// changeOf is the issue of the code a sheet has under the content, with the
// sentence "A classe mudou" tells it with.
func changeOf(t *testing.T, b Build, c *Content, code string) Issue {
	t.Helper()
	for _, is := range Derive(b, c).Issues {
		if is.Code == code {
			return is
		}
	}
	t.Fatalf("the sheet has no %s issue: %v", code, Derive(b, c).Issues)
	return Issue{}
}

// TestChangedClassSentences: the common changes of a table class give the sheet
// that uses it an issue tied to the entry, with the exact sentence of "A classe
// mudou": the skill count, the cantrips, the known
// and the prepared spells, the level of the subclass, the multiclass
// prerequisite and an option the class no longer offers.
func TestChangedClassSentences(t *testing.T) {
	t.Parallel()
	srd := loadForTest(t)
	o1 := stamped(t, srd)
	c1 := withOverlayOn(t, srd, o1)
	if issues := Derive(sweepBase(t, c1, "class:gen-none@mesa", "subclass:gen-none-a@mesa"), c1).Issues; len(issues) != 0 {
		t.Fatalf("the sheet starts with issues: %v", issues)
	}
	change := func(edit func(o *Overlay)) *Content {
		o := stamped(t, srd)
		edit(&o)
		return withOverlayOn(t, srd, o)
	}
	const none, fullPrepared, fullKnown = "class:gen-none@mesa", "class:gen-full-prepared@mesa", "class:gen-full-known@mesa"

	t.Run("fewer skills", func(t *testing.T) {
		t.Parallel()
		b := sweepBase(t, c1, none, "subclass:gen-none-a@mesa") // chose 2
		c2 := change(func(o *Overlay) { classOf(o, none).SkillChoose = 1 })
		is := changeOf(t, b, c2, IssueSkillCount)
		if want := "agora dá 1 perícia no nível 1; esta ficha tem 2."; is.ChangeMessage != want {
			t.Errorf("change = %q, want %q", is.ChangeMessage, want)
		}
		if is.Message != "Há 2 perícias escolhidas; o personagem escolhe 1." || !slices.Contains(is.Keys, none) {
			t.Errorf("issue = %+v, want its own sentence and the class's key", is)
		}
	})
	t.Run("more skills", func(t *testing.T) {
		t.Parallel()
		b := sweepBase(t, c1, none, "subclass:gen-none-a@mesa")
		c2 := change(func(o *Overlay) { classOf(o, none).SkillChoose = 3 })
		if want := "agora dá 3 perícias no nível 1; esta ficha tem 2."; changeOf(t, b, c2, IssueSkillCount).ChangeMessage != want {
			t.Errorf("change = %q, want %q", changeOf(t, b, c2, IssueSkillCount).ChangeMessage, want)
		}
	})
	t.Run("fewer cantrips", func(t *testing.T) {
		t.Parallel()
		b := sweepBase(t, c1, fullPrepared, "subclass:gen-full-prepared-a@mesa") // 3 cantrips
		c2 := change(func(o *Overlay) { classOf(o, fullPrepared).Levels[0].CantripsKnown = 2 })
		is := changeOf(t, b, c2, IssueSpellCount)
		if want := "agora conhece 2 truques; esta ficha tem 3."; is.ChangeMessage != want || !slices.Contains(is.Keys, fullPrepared) {
			t.Errorf("issue = %+v, want %q tied to the class", is, want)
		}
	})
	t.Run("fewer known spells", func(t *testing.T) {
		t.Parallel()
		b := sweepBase(t, c1, fullKnown, "subclass:gen-full-known-a@mesa") // knows 2
		c2 := change(func(o *Overlay) { classOf(o, fullKnown).Levels[0].SpellsKnown = 1 })
		if want := "agora conhece 1 magia; esta ficha tem 2."; changeOf(t, b, c2, IssueSpellCount).ChangeMessage != want {
			t.Errorf("change = %q, want %q", changeOf(t, b, c2, IssueSpellCount).ChangeMessage, want)
		}
	})
	t.Run("fewer prepared spells", func(t *testing.T) {
		t.Parallel()
		b := sweepBase(t, c1, fullPrepared, "subclass:gen-full-prepared-a@mesa")
		b = sweepUp(t, c1, b, fullPrepared, "subclass:gen-full-prepared-a@mesa", 3)
		prepared := len(b.SpellsPrepared)
		c2 := change(func(o *Overlay) { classOf(o, fullPrepared).Casting.PreparedMax = "1" })
		is := changeOf(t, b, c2, IssueSpellCount)
		if want := "agora prepara 1 magia; esta ficha tem " + strconv.Itoa(prepared) + "."; is.ChangeMessage != want {
			t.Errorf("change = %q, want %q", is.ChangeMessage, want)
		}
	})
	t.Run("a third caster's cantrips name the subclass", func(t *testing.T) {
		t.Parallel()
		sub := "subclass:cavaleiro-runico@mesa"
		b := sweepBase(t, c1, "class:fighter", sub)
		b = sweepUp(t, c1, b, "class:fighter", sub, 3)
		c2 := change(func(o *Overlay) { subclassOf2(o, sub).Levels[0].CantripsKnown = 1 })
		if want := "agora conhece 1 truque; esta ficha tem 2."; changeOf(t, b, c2, IssueSpellCount).ChangeMessage != want {
			t.Errorf("change = %q, want %q", changeOf(t, b, c2, IssueSpellCount).ChangeMessage, want)
		}
	})
	t.Run("the subclass level moved", func(t *testing.T) {
		t.Parallel()
		sub := "subclass:gen-none-a@mesa"
		b := sweepBase(t, c1, none, sub)
		b = sweepUp(t, c1, b, none, sub, 3)
		c2 := change(func(o *Overlay) {
			classOf(o, none).SubclassLevel = 5
			subclassOf2(o, sub).Levels[0].Level = 5 // its first features move with it
			subclassOf2(o, sub).Levels = subclassOf2(o, sub).Levels[:1]
		})
		is := changeOf(t, b, c2, IssueSubclassLevel)
		if want := "agora escolhe a subclasse no nível 5; esta ficha tem nível 3 nela."; is.ChangeMessage != want {
			t.Errorf("change = %q, want %q", is.ChangeMessage, want)
		}
		if is.Message != "Classe none escolhe a subclasse no nível 5." || !slices.Contains(is.Keys, none) {
			t.Errorf("issue = %+v", is)
		}
	})
	t.Run("an option the class stopped offering", func(t *testing.T) {
		t.Parallel()
		sub := "subclass:gen-none-a@mesa"
		b := sweepBase(t, c1, none, sub)
		b = sweepUp(t, c1, b, none, sub, 6) // the fighting style at level 6
		if len(b.FeatureChoices) == 0 {
			t.Fatal("the sheet made no feature choice")
		}
		chosen := b.FeatureChoices[0]
		other := "feature:fighter-fighting-style-dueling"
		if chosen == other {
			other = "feature:fighter-fighting-style-defense"
		}
		// The feature stays but offers another style: the chosen one is no longer offered.
		c2 := change(func(o *Overlay) { classOf(o, none).Levels[5].Features[0].Effects[0].From = []string{other} })
		is := changeOf(t, b, c2, IssueUnknownKey)
		if !slices.Contains(is.Keys, none) || is.ChangeSubject != none {
			t.Errorf("issue = %+v, want it tied to the class whose feature offers that set of options", is)
		}
		if want := "agora não oferece a escolha " + c2.NamePT(chosen) + "; ela não vale mais nesta ficha."; is.ChangeMessage != want {
			t.Errorf("change = %q, want %q", is.ChangeMessage, want)
		}
		// The feature gone altogether: nothing is left to blame.
		c3 := change(func(o *Overlay) { classOf(o, none).Levels[5].Features = nil })
		if is := changeOf(t, b, c3, IssueUnknownKey); len(is.Keys) != 0 || is.ChangeMessage != "" {
			t.Errorf("issue = %+v, want no entry tied when no feature offers the set", is)
		}
	})
	t.Run("an SRD option is not tied to a table subclass that has nothing to do with it", func(t *testing.T) {
		t.Parallel()
		// A Wizard with a table subclass and a Fighter's style: the style is invalid
		// and the subclass is not where it comes from.
		sub := "subclass:gen-none-a@mesa"
		b := sweepBase(t, c1, none, sub)
		b.FeatureChoices = append(b.FeatureChoices, "feature:fighter-fighting-style-defense")
		is := changeOf(t, b, c1, IssueUnknownKey)
		if len(is.Keys) != 0 || is.ChangeMessage != "" || is.ChangeSubject != "" {
			t.Errorf("issue = %+v, want it tied to nothing", is)
		}
	})
	t.Run("the multiclass prerequisite", func(t *testing.T) {
		t.Parallel()
		b := sweepBase(t, c1, none, "subclass:gen-none-a@mesa")
		b.BaseScores = map[Ability]int{STR: 14, DEX: 14, CON: 14, INT: 14, WIS: 14, CHA: 14}
		b = addClass(t, c1, b, "class:gen-half-prepared@mesa", 2)
		if issues := Derive(b, c1).Issues; len(issues) != 0 {
			t.Fatalf("the multiclass sheet has issues: %v", issues)
		}
		c2 := change(func(o *Overlay) { classOf(o, "class:gen-half-prepared@mesa").Minimums = map[Ability]int{STR: 20} })
		is := changeOf(t, b, c2, IssueMulticlass)
		if want := "agora pede outras habilidades para multiclasse; os desta ficha não cumprem."; is.ChangeMessage != want {
			t.Errorf("change = %q, want %q", is.ChangeMessage, want)
		}
	})
	t.Run("the SRD never says changed", func(t *testing.T) {
		t.Parallel()
		b := sweepBase(t, srd, "class:wizard", "")
		b.SkillProficiencies = append(b.SkillProficiencies, nextSkill(srd, b))
		is := changeOf(t, b, srd, IssueSkillCount)
		if is.ChangeMessage != "" || len(is.Keys) != 0 {
			t.Errorf("an SRD issue = %+v, want no change sentence and no key", is)
		}
	})
}

// TestChangeSentenceNeverNamesTheWrongEntry: the issue's
// sentence is about a class or subclass (ChangeSubject) and the notice names it
// only when it is the entry that changed; an SRD class whose count moved because a
// table race's trait changed is never named.
func TestChangeSentenceNeverNamesTheWrongEntry(t *testing.T) {
	t.Parallel()
	srd := loadForTest(t)
	race := TableRace{
		Key: "race:perito@mesa", NamePT: "Perito", Revision: 1, Size: "Medium", SpeedFt: 30,
		Traits: []TableFeature{{Key: "trait:faro@mesa", NamePT: "Faro", Effects: []Effect{{Type: "choice", Choice: "skill", Count: 1}}}},
	}
	with := func(r TableRace) *Content { return withOverlayOn(t, srd, Overlay{Revision: 2, Races: []TableRace{r}}) }
	c1 := with(race)
	// An SRD Rogue (4 skills) of the table's race (+1): five chosen.
	b := sweepBase(t, c1, "class:rogue", "")
	b.Race = race.Key
	b.SkillProficiencies = append(b.SkillProficiencies, nextSkill(c1, b))
	if issues := Derive(b, c1).Issues; len(issues) != 0 {
		t.Fatalf("the sheet starts with issues: %v", issues)
	}
	race.Traits = nil // the master removes the trait
	c2 := with(race)
	is := changeOf(t, b, c2, IssueSkillCount)
	if !slices.Contains(is.Keys, race.Key) {
		t.Fatalf("issue = %+v, want it tied to the race", is)
	}
	if is.ChangeSubject != "class:rogue" || !strings.HasPrefix(is.ChangeMessage, "agora dá 4 perícias") {
		t.Fatalf("issue = %+v, want a sentence about the Rogue, to be told without a name", is)
	}
	// The sentence the notice reads names the Rogue only for the Rogue: the entry that
	// changed here is the race (characters.changeSentence does that; it is the
	// subject comparison that matters).
	if is.ChangeSubject == race.Key {
		t.Error("the subject is the race")
	}
}

// TestLevelUpSubclassOffersAPreparedThirdCaster: a third
// caster's subclass that prepares its spells says so in the offer, with how many it
// may prepare at the new level, and one that knows them does not.
func TestLevelUpSubclassOffersAPreparedThirdCaster(t *testing.T) {
	t.Parallel()
	srd := loadForTest(t)
	c := withOverlayOn(t, srd, genOverlay(t, srd))
	b := sweepBase(t, c, "class:rogue", "")
	b = sweepUp(t, c, b, "class:rogue", "subclass:trapaceiro-mistico@mesa", 2)
	o, err := LevelUpOptions(b, "class:rogue", c)
	if err != nil || !o.SubclassDue {
		t.Fatalf("offer = %+v, %v; want the subclass due", o, err)
	}
	for _, s := range o.Subclasses {
		switch s.Key {
		case "subclass:trapaceiro-mistico@mesa":
			if !s.Prepares || s.PreparedMaxAfter < 1 || s.SpellList != "class:wizard" {
				t.Errorf("the prepared third caster = %+v, want it to prepare, with a maximum", s)
			}
		case "subclass:thief":
			if s.Prepares || s.PreparedMaxAfter != 0 {
				t.Errorf("the Thief = %+v, want it to prepare nothing", s)
			}
		}
	}
}

// TestThirdCasterUnderAClassWhoseSubclassComesLater: a third
// caster's subclass of a table class that chooses its subclass at level 5 casts
// from level 5, and goes from the pick to level 20 without an issue.
func TestThirdCasterUnderAClassWhoseSubclassComesLater(t *testing.T) {
	t.Parallel()
	srd := loadForTest(t)
	o := genOverlay(t, srd)
	const class, subKey = "class:gen-none@mesa", "subclass:terco-cinco@mesa"
	classOf(&o, class).SubclassLevel = 5
	plain := subclassOf2(&o, "subclass:gen-none-a@mesa")
	plain.Levels[0].Level = 5
	third := thirdCaster(srd, "terco-cinco", class, "class:wizard", PreparationKnown)
	third.Casting.StartLevel = 5
	third.Levels = third.Levels[2:] // levels 5 to 20
	o.Subclasses = append(o.Subclasses, third)
	c := withOverlayOn(t, srd, o)
	b := sweepBase(t, c, class, subKey)
	b = sweepUp(t, c, b, class, subKey, 4)
	if d := Derive(b, c); len(d.Spellcasting) != 0 {
		t.Fatalf("casts at level 4: %v", d.Spellcasting)
	}
	b = sweepUp(t, c, b, class, subKey, MaxLevel)
	d := Derive(b, c)
	if len(d.Spellcasting) != 1 || d.Spellcasting[0].SpellsKnownMax == 0 || d.SpellSlots[0] != 4 {
		t.Errorf("at level 20: casting %v, slots %v; want the third caster's table", d.Spellcasting, d.SpellSlots)
	}
}
