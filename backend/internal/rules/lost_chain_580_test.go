package rules

import (
	"slices"
	"testing"
)

// #580, board 580-05: the entries that wait for another lost entry. What the sentence says of the one they wait for is what
// is true of it: the sheet still lists it (a lost entry stays listed), and does not apply it.

func lostIssueOf(d Derived, key string) (Issue, bool) {
	for _, is := range d.Issues {
		if is.Lost != nil && is.Lost.Key == key {
			return is, true
		}
	}
	return Issue{}, false
}

// B needs A, and A is lost by its own prerequisite (a class level): B is lost too, gives nothing, and says A is not applied
// now, not that the character has no A.
func TestAnOptionThatNeedsAnOptionLostByItsOwnPrerequisiteIsLost(t *testing.T) {
	t.Parallel()
	c := withOverlay(t, gatedPostureOverlay())
	b := postureBuild(3, postureBull, postureWolf)
	d := Derive(b, c)
	if got := lostKeys(d); !slices.Equal(got, []string{postureWolf, postureBull}) {
		t.Fatalf("lost = %v, want the Bull (asks Guerreiro 5) and the Wolf (asks the Bull)", got)
	}
	wolf, _ := lostIssueOf(d, postureWolf)
	want := "Postura do Lobo pede " + optionLabel(c.c.namePT(postureBull)) + ", que Tor não aplica agora: a opção só vale de novo quando Tor cumprir."
	if got := wolf.Lost.SentencePT("Tor", false); got != want {
		t.Errorf("the Wolf's sentence = %q, want %q", got, want)
	}
	bull, _ := lostIssueOf(d, postureBull)
	if got, want := bull.Lost.SentencePT("Tor", false), "Postura do Touro pede Guerreiro 5, e Tor está no 3: a opção só vale de novo quando Tor cumprir."; got != want {
		t.Errorf("the Bull's sentence = %q, want %q", got, want)
	}
}

// A lost Pact Boon the sheet still lists is not a pact the character has lost: the invocation that asks for it says it is not
// applied now.
func TestAnInvocationThatNeedsALostPactSaysThePactIsNotAppliedNow(t *testing.T) {
	t.Parallel()
	pact := TableOption{
		TableEntry: TableEntry{Key: "option:pact_boon:pacto-da-lanterna@mesa", NamePT: "Pacto da Lanterna"}, Set: "pact_boon", DescPT: []string{"Uma luz."},
		Prerequisite: TableOptionPrerequisite{ClassKey: "class:warlock", ClassLevel: 5},
	}
	needs := crowGazeOption()
	needs.Prerequisite = TableOptionPrerequisite{RequiresPact: pact.Key}
	c := withOverlay(t, optionsOverlay(pact, needs))
	d := Derive(warlock(3, pact.Key, needs.Key), c)
	if got := lostKeys(d); !slices.Equal(got, slices.Sorted(slices.Values([]string{needs.Key, pact.Key}))) {
		t.Fatalf("lost = %v, want the pact (asks Bruxo 5) and the invocation (asks the pact)", got)
	}
	is, _ := lostIssueOf(d, needs.Key)
	want := "Olhar do Corvo pede a Dádiva do Pacto Pacto da Lanterna, que Tor não aplica agora: a opção só vale de novo quando Tor cumprir."
	if got := is.Lost.SentencePT("Tor", false); got != want {
		t.Errorf("sentence = %q, want %q", got, want)
	}
	// A pact the sheet does not list at all is still one the character does not have.
	gone := Derive(warlock(3, needs.Key), c)
	is, ok := lostIssueOf(gone, needs.Key)
	if !ok {
		t.Fatalf("issues = %+v, want the invocation lost", gone.Issues)
	}
	want = "Olhar do Corvo pede a Dádiva do Pacto Pacto da Lanterna, que Tor não tem mais: a opção só vale de novo quando Tor cumprir."
	if got := is.Lost.SentencePT("Tor", false); got != want {
		t.Errorf("without the pact: sentence = %q, want %q", got, want)
	}
}

// A feat picked through the choice of a feat that is lost gives nothing while its giver is lost (the pick stays stored, and
// the giver's own issue is the one told).
func TestAFeatPickedThroughALostFeatsChoiceIsLostToo(t *testing.T) {
	t.Parallel()
	giver := TableFeat{
		TableEntry: TableEntry{Key: "feat:doador" + tableSuffix, NamePT: "Doador"}, DescPT: []string{"Texto."}, Prerequisite: FeatPrerequisite{Level: 20},
	}
	given := TableFeat{
		TableEntry: TableEntry{Key: "feat:dado" + tableSuffix, NamePT: "Dado"}, DescPT: []string{"Texto."},
		Effects: []Effect{{Type: "modifier", Target: "initiative", Mode: "add", Value: "5"}},
	}
	giver.Effects = []Effect{{Type: "choice", Choice: "feat", Count: 1, From: []string{given.Key}}}
	c := withOverlay(t, Overlay{Revision: 1, Feats: []TableFeat{giver, given}})
	b := postureBuild(3, ScopedChoice(FeatChoiceKey(giver.Key), given.Key))
	b.Classes[0].Subclass = ""
	b.Feats = []string{giver.Key}
	plain := b
	plain.Feats, plain.FeatureChoices = nil, nil

	d := Derive(b, c)
	if !slices.Contains(lostKeys(d), giver.Key) {
		t.Fatalf("lost = %v, want the giver (asks level 20)", lostKeys(d))
	}
	if d.Initiative != Derive(plain, c).Initiative {
		t.Errorf("initiative %d, want %d: the feat given by a lost feat gives nothing", d.Initiative, Derive(plain, c).Initiative)
	}
}

// Two entries that wait for each other: options cannot (the table refuses the cycle at the first option), and a feat that
// asks for the option its own choice offers, while it is lost for another condition, ends with both lost, each saying what
// is true of the other, in one pass for each entry.
func TestTwoEntriesThatNeedEachOtherAreRefusedOrEndLost(t *testing.T) {
	t.Parallel()
	const defense = "feature:ranger-fighting-style-defense"
	feat := TableFeat{
		TableEntry: TableEntry{Key: "feat:estilo-emprestado" + tableSuffix, NamePT: "Estilo emprestado"}, DescPT: []string{"Texto."},
		Prerequisite: FeatPrerequisite{Level: 20, RequiresOptions: []string{defense}},
		Effects:      []Effect{{Type: "choice", Choice: "feature", Count: 1, From: []string{defense}}},
	}
	c := withOverlay(t, Overlay{Revision: 1, Feats: []TableFeat{feat}})
	b := postureBuild(3, defense)
	b.Classes[0].Subclass = ""
	b.Armor, b.Feats = "equipment:chain-mail", []string{feat.Key}
	d, rounds := deriveRounds(b, c.c)
	if got := lostKeys(d); !slices.Equal(got, []string{feat.Key, defense}) {
		t.Fatalf("lost = %v, want the feat and the style it offers", got)
	}
	if rounds > 3 {
		t.Errorf("the sheet was made %d times, want at most one for each entry and the first", rounds)
	}
	is, _ := lostIssueOf(d, feat.Key)
	want := "Estilo emprestado pede nível 20, e Tor está no 3 e " + optionLabel(c.c.namePT(defense)) + ", que Tor não aplica agora: o talento só vale de novo quando Tor cumprir."
	if got := is.Lost.SentencePT("Tor", false); got != want {
		t.Errorf("the feat's sentence = %q, want %q", got, want)
	}
	style, _ := lostIssueOf(d, defense)
	want = c.c.namePT(defense) + " pede o talento Estilo emprestado, que Tor não aplica agora: a opção só vale de novo quando Tor cumprir."
	if got := style.Lost.SentencePT("Tor", false); got != want {
		t.Errorf("the style's sentence = %q, want %q", got, want)
	}
}
