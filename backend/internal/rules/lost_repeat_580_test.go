package rules

import (
	"fmt"
	"slices"
	"strings"
	"testing"
)

// #580, preflight: an entry that is unmet only after other lost entries stop applying is lost too. Derive judges the
// feats and the options the sheet picked, makes the sheet again without the lost ones, and judges again over that
// sheet, until nothing new is lost.

// scoreFeat is a table feat that asks Strength `asks` (0 for nothing, then it asks level 20, which a test sheet lacks) and
// gives Strength +1.
func scoreFeat(slug string, asks int, gives bool) TableFeat {
	p := FeatPrerequisite{Level: 20}
	if asks > 0 {
		p = FeatPrerequisite{Minimums: map[Ability]int{STR: asks}}
	}
	f := TableFeat{TableEntry: TableEntry{Key: "feat:" + slug + tableSuffix, NamePT: "Talento " + slug}, DescPT: []string{"Texto."}, Prerequisite: p}
	if gives {
		f.Effects = []Effect{{Type: "modifier", Target: "score.str", Mode: "add", Value: "1", Cap: 20}}
	}
	return f
}

// featsOverlay is a Fighter whose Strength is 12 before any feat, with the table feats given and taken.
func featsOverlay(t *testing.T, feats ...TableFeat) (*Content, Build) {
	t.Helper()
	c := withOverlay(t, Overlay{Revision: 1, Feats: feats})
	b := postureBuild(3)
	b.Classes[0].Subclass = ""
	b.BaseScores[STR] = 11 // 12 with the human's +1
	for _, f := range feats {
		b.Feats = append(b.Feats, f.Key)
	}
	return c, b
}

func strengthOf(d Derived) int {
	for _, s := range d.Abilities {
		if s.Ability == STR {
			return s.Score
		}
	}
	return 0
}

func lostKeys(d Derived) []string {
	var out []string
	for _, is := range d.Issues {
		if is.Lost != nil {
			out = append(out, is.Lost.Key)
		}
	}
	slices.Sort(out)
	return out
}

// Feat A gives Strength +1 and is lost; feat B asks Strength 15 and the sheet has 14 only without A: B is lost too, has
// its issue and gives nothing.
func TestAFeatUnmetOnlyWithoutALostFeatIsLostToo(t *testing.T) {
	t.Parallel()
	a, bFeat := scoreFeat("a", 0, true), scoreFeat("b", 14, true)
	c, b := featsOverlay(t, a, bFeat)
	// 12 +1 (A) +1 (B) = 14 asks 14: met with A in place; without A the sheet has 13.
	d := Derive(b, c)
	if got := lostKeys(d); !slices.Equal(got, []string{a.Key, bFeat.Key}) {
		t.Fatalf("lost = %v, want both the feat that asks level 20 and the one that asks Strength 14", got)
	}
	if got := strengthOf(d); got != 12 {
		t.Errorf("Strength = %d, want 12: neither feat gives its +1", got)
	}
	for _, is := range d.Issues {
		if is.Lost != nil && is.Lost.Key == bFeat.Key {
			if is.Code != IssueFeatPrerequisite || is.Field != "full.feat_keys[1]" {
				t.Errorf("B's issue = %s at %s, want %s at full.feat_keys[1]", is.Code, is.Field, IssueFeatPrerequisite)
			}
			const want = "Talento b pede Força 14, e Tor tem 12: o talento só vale de novo quando Tor cumprir."
			if got := is.Lost.SentencePT("Tor", false); got != want {
				t.Errorf("B's sentence = %q, want %q (what the final sheet shows)", got, want)
			}
		}
	}
}

// A chain of three: A is lost, which loses B, which loses C.
func TestALostFeatLosesTheNextWhichLosesTheNext(t *testing.T) {
	t.Parallel()
	a, bFeat, cFeat := scoreFeat("a", 0, true), scoreFeat("b", 15, true), scoreFeat("c", 14, true)
	c, b := featsOverlay(t, a, bFeat, cFeat)
	// With all three the sheet has 15: both ask and get it. Without A it has 14: B asks 15. Without A and B, 13: C asks 14.
	d, rounds := deriveRounds(b, c.c)
	if got := lostKeys(d); !slices.Equal(got, []string{a.Key, bFeat.Key, cFeat.Key}) {
		t.Fatalf("lost = %v, want the three", got)
	}
	if got := strengthOf(d); got != 12 {
		t.Errorf("Strength = %d, want 12: no feat gives anything", got)
	}
	if rounds != 4 {
		t.Errorf("the sheet was made %d times, want 4: the first and one for each pass that lost something", rounds)
	}
}

// The bound: N feats, each lost by the one before, end with N lost and N+1 makes of the sheet, never an endless loop.
func TestTheRepeatEndsAtOnePassForEachEntry(t *testing.T) {
	t.Parallel()
	const n = 5
	feats := []TableFeat{scoreFeat("f0", 0, true)}
	total := 12 + n + 1 // every feat gives +1 on the first sheet
	for k := 1; k <= n; k++ {
		feats = append(feats, scoreFeat(fmt.Sprintf("f%d", k), total-k+1, true))
	}
	c, b := featsOverlay(t, feats...)
	d, rounds := deriveRounds(b, c.c)
	if got := len(lostKeys(d)); got != n+1 {
		t.Errorf("lost %d feats, want all %d", got, n+1)
	}
	if rounds != n+2 {
		t.Errorf("the sheet was made %d times, want %d", rounds, n+2)
	}
	if got := strengthOf(d); got != 12 {
		t.Errorf("Strength = %d, want 12", got)
	}
}

// A sheet with one plainly lost feat is made twice, as it always was; a sheet that loses nothing is made once.
func TestOrdinarySheetsCostWhatTheyDid(t *testing.T) {
	t.Parallel()
	lost, fine := scoreFeat("a", 0, true), scoreFeat("ok", 12, true)
	c, b := featsOverlay(t, lost, fine)
	d, rounds := deriveRounds(b, c.c)
	if got := lostKeys(d); !slices.Equal(got, []string{lost.Key}) || rounds != 2 {
		t.Errorf("lost %v in %d makes, want only the plain one in 2", got, rounds)
	}
	if got := strengthOf(d); got != 13 {
		t.Errorf("Strength = %d, want 13: only the feat that fits gives its +1", got)
	}
	c2, b2 := featsOverlay(t, fine)
	if d2, rounds := deriveRounds(b2, c2.c); len(lostKeys(d2)) != 0 || rounds != 1 {
		t.Errorf("a sheet that loses nothing: lost %v in %d makes, want none in 1", lostKeys(d2), rounds)
	}
}

// An option whose prerequisite is an option that a lost feat offered is lost too: it has its issue and its effects stop.
func TestAnOptionWhoseOptionCameFromALostFeatIsLostToo(t *testing.T) {
	t.Parallel()
	const archery = "feature:ranger-fighting-style-archery"
	o := gatedPostureOverlay()
	wolf := &o.Subclasses[0].Levels[0].Features[0].Options[2]
	wolf.Prerequisite = TableOptionPrerequisite{RequiresOptions: []string{archery}}
	wolf.Effects = []Effect{{Type: "modifier", Target: "initiative", Mode: "add", Value: "3"}}
	feat := TableFeat{
		TableEntry: TableEntry{Key: "feat:estilo-emprestado" + tableSuffix, NamePT: "Estilo emprestado"}, DescPT: []string{"Texto."},
		Prerequisite: FeatPrerequisite{Level: 20},
		Effects:      []Effect{{Type: "choice", Choice: "feature", Count: 1, From: []string{archery}}},
	}
	o.Feats = []TableFeat{feat}
	c := withOverlay(t, o)

	b := postureBuild(3, archery, postureWolf)
	b.Feats = []string{feat.Key}
	d, rounds := deriveRounds(b, c.c)
	if got := lostKeys(d); !slices.Contains(got, feat.Key) || !slices.Contains(got, postureWolf) {
		t.Fatalf("lost = %v, want the feat and the Wolf", got)
	}
	var issue *Issue
	for i, is := range d.Issues {
		if is.Code == IssueOptionPrerequisite && is.Lost != nil && is.Lost.Key == postureWolf {
			issue = &d.Issues[i]
		}
	}
	if issue == nil || issue.Field != "full.feature_choice_keys[1]" {
		t.Fatalf("issues = %+v, want the Wolf's option_prerequisite at full.feature_choice_keys[1]", d.Issues)
	}
	if got := issue.Lost.SentencePT("Tor", false); got != "Postura do Lobo pede "+optionLabel(c.c.namePT(archery))+", que Tor não aplica agora: a opção só vale de novo quando Tor cumprir." {
		t.Errorf("the Wolf's sentence = %q", got)
	}
	bare := b
	bare.FeatureChoices = []string{archery}
	if want := Derive(bare, c).Initiative; d.Initiative != want {
		t.Errorf("initiative %d, want %d: the lost Wolf gives no +3", d.Initiative, want)
	}
	if rounds != 3 {
		t.Errorf("the sheet was made %d times, want 3", rounds)
	}
}

// The options picked for a lost feat's own choice are lost too: they read as an option that needs its feat, not as a pick
// nothing offers, and they give nothing.
func TestAnOptionOfALostFeatReadsAsLostNotAsNotOffered(t *testing.T) {
	t.Parallel()
	const defense = "feature:ranger-fighting-style-defense"
	feat := TableFeat{
		TableEntry: TableEntry{Key: "feat:estilo-emprestado" + tableSuffix, NamePT: "Estilo emprestado"}, DescPT: []string{"Texto."},
		Prerequisite: FeatPrerequisite{Level: 20},
		Effects:      []Effect{{Type: "choice", Choice: "feature", Count: 1, From: []string{defense}}},
	}
	c := withOverlay(t, Overlay{Revision: 1, Feats: []TableFeat{feat}})
	b := postureBuild(3, defense)
	b.Classes[0].Subclass = ""
	b.Armor, b.Feats = "equipment:chain-mail", []string{feat.Key}
	plain := b
	plain.FeatureChoices, plain.Feats = nil, nil

	d := Derive(b, c)
	for _, is := range d.Issues {
		if is.Code == IssueUnknownKey {
			t.Errorf("issue %+v: the pick of a lost feat is no pick that nothing offers", is)
		}
	}
	var issue *Issue
	for i, is := range d.Issues {
		if is.Code == IssueOptionPrerequisite && is.Lost != nil && is.Lost.Key == defense {
			issue = &d.Issues[i]
		}
	}
	if issue == nil || issue.Field != "full.feature_choice_keys[0]" {
		t.Fatalf("issues = %+v, want the option_prerequisite of the pick", d.Issues)
	}
	want := c.c.namePT(defense) + " pede o talento Estilo emprestado, que Tor não aplica agora: a opção só vale de novo quando Tor cumprir."
	if got := issue.Lost.SentencePT("Tor", false); got != want {
		t.Errorf("sentence = %q, want %q", got, want)
	}
	if !slices.ContainsFunc(d.Features, func(f Feature) bool { return f.Key == defense }) {
		t.Errorf("the lost option left the sheet's list: %v", d.Features)
	}
	if d.ArmorClass != Derive(plain, c).ArmorClass {
		t.Errorf("AC %d, want %d: the lost option gives nothing", d.ArmorClass, Derive(plain, c).ArmorClass)
	}
}

// A feat lost in the first pass for one condition, that fails another only on the sheet as it ends up (the Strength a lost
// feat gave), says both: what the reader sees is what the final sheet shows.
func TestALostFeatSaysTheConditionThatFailsOnlyOnTheFinalSheet(t *testing.T) {
	t.Parallel()
	a := scoreFeat("a", 0, true)
	both := scoreFeat("b", 0, false)
	both.Prerequisite = FeatPrerequisite{Level: 20, Minimums: map[Ability]int{STR: 13}}
	c, b := featsOverlay(t, a, both)
	// With A in place the sheet has Strength 13, which B asks: B is lost only for its level in the first pass. Without A
	// the sheet has 12.
	d := Derive(b, c)
	if got := lostKeys(d); !slices.Equal(got, []string{a.Key, both.Key}) {
		t.Fatalf("lost = %v, want both", got)
	}
	var is Issue
	for _, i := range d.Issues {
		if i.Lost != nil && i.Lost.Key == both.Key {
			is = i
		}
	}
	const want = "Talento b pede Força 13, e Tor tem 12 e nível 20, e Tor está no 3: o talento só vale de novo quando Tor cumprir."
	if got := is.Lost.SentencePT("Tor", false); got != want {
		t.Errorf("sentence = %q, want %q", got, want)
	}
	if want := strings.ReplaceAll(want, "Tor", lostNobody); is.Message != want {
		t.Errorf("message = %q, want %q", is.Message, want)
	}
}
