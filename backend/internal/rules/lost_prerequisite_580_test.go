package rules

import (
	"cmp"
	"slices"
	"strconv"
	"strings"
	"testing"
)

// #580, board 580-05: a lost prerequisite reads "X pede A, e Tor está no 5: a opção só vale de novo quando Tor cumprir.",
// saying of each missing condition what the character has. The issue's own Message names nobody and nothing hidden; the
// sentence a reader gets (LostPrerequisite.SentencePT) is built for that reader.

func lostIssue(t *testing.T, d Derived, code string) Issue {
	t.Helper()
	i := slices.IndexFunc(d.Issues, func(i Issue) bool { return i.Code == code })
	if i < 0 {
		t.Fatalf("issues = %+v, want %q", d.Issues, code)
	}
	if d.Issues[i].Lost == nil {
		t.Fatalf("issue %+v has no Lost", d.Issues[i])
	}
	return d.Issues[i]
}

func TestALostOptionSaysTheLevelAskedAndTheLevelTheCharacterHas(t *testing.T) {
	t.Parallel()
	c := withOverlay(t, gatedPostureOverlay())
	is := lostIssue(t, Derive(postureBuild(3, postureBull), c), IssueOptionPrerequisite)
	const want = "Postura do Touro pede Guerreiro 5, e Tor está no 3: a opção só vale de novo quando Tor cumprir."
	for _, master := range []bool{false, true} {
		if got := is.Lost.SentencePT("Tor", master); got != want {
			t.Errorf("master=%v: sentence = %q, want %q", master, got, want)
		}
	}
	const stored = "Postura do Touro pede Guerreiro 5, e o personagem está no 3: a opção só vale de novo quando o personagem cumprir."
	if is.Message != stored {
		t.Errorf("message = %q, want %q", is.Message, stored)
	}
}

func TestALostFeatSaysWhatIsMissingInTheSameShape(t *testing.T) {
	t.Parallel()
	c := gatedFeats(t)
	spell := c.c.namePT("spell:magic-missile")
	fighter := choiceBuild("race:human", "", ClassLevel{Class: "class:fighter", Level: 3})
	cases := []struct {
		name string
		b    Build
		feat string
		want string
	}{
		{"a level in the class, too low", warlock(3), featKeyWarlock5, "Bruxo 5 pede Bruxo 5, e Tor está no 3: o talento só vale de novo quando Tor cumprir."},
		{"no level in the class", fighter, featKeyWarlock5, "Bruxo 5 pede Bruxo 5, e Tor não tem nível de Bruxo: o talento só vale de novo quando Tor cumprir."},
		{"a spell", warlock(3), featKeyMagicMissl, "Mísseis pede a magia " + spell + ", que Tor não conhece mais: o talento só vale de novo quando Tor cumprir."},
		{"two options, joined as a list", fighter, featKeyNeedsStyle, "Estilo pede " + optionLabel(c.c.namePT("feature:fighting-style-defense")) + ", que Tor não tem mais e " +
			optionLabel(c.c.namePT("feature:fighting-style-dueling")) + ", que Tor não tem mais: o talento só vale de novo quando Tor cumprir."},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			b := tc.b
			b.Feats = []string{tc.feat}
			is := lostIssue(t, Derive(b, c), IssueFeatPrerequisite)
			if got := is.Lost.SentencePT("Tor", false); got != tc.want {
				t.Errorf("sentence = %q, want %q", got, tc.want)
			}
		})
	}
}

// A feat also asks for an ability score or a proficiency: the sentence says the score the character has, or that it lacks the proficiency.
func TestALostFeatSaysTheScoreAndTheProficiency(t *testing.T) {
	t.Parallel()
	c := gatedFeats(t)
	b := warlock(3)
	d := Derive(b, c)
	score := 0
	for _, s := range d.Abilities {
		if s.Ability == STR {
			score = s.Score
		}
	}
	l := LostPrerequisite{EntryPT: "Peso", Conditions: []LostCondition{
		c.c.lostCondition(FeatUnmet{Kind: FeatUnmetAbilityMinimum, Abilities: []AbilityMinimum{{Ability: STR, Minimum: 13}}}, b, d),
		c.c.lostCondition(FeatUnmet{Kind: FeatUnmetProficiency, Key: "proficiency:heavy-armor"}, b, d),
	}}
	want := "Peso pede " + c.c.namePT(string(STR)) + " 13, e Tor tem " + strconv.Itoa(score) + " e proficiência em " + c.c.namePT("proficiency:heavy-armor") +
		", que Tor não tem: o talento só vale de novo quando Tor cumprir."
	if got := l.SentencePT("Tor", false); got != want {
		t.Errorf("sentence = %q, want %q", got, want)
	}
}

// RN-23: a hidden missing condition reads "uma condição que a mesa não mostra" for a player and is named for the master;
// the issue's own Message never names it, and a player's view of the content holds no name to give.
func TestALostOptionWithAHiddenConditionIsMaskedForAPlayerAndNamedForTheMaster(t *testing.T) {
	t.Parallel()
	o, words := hiddenAskOverlay(t)
	c := withOverlay(t, o)
	is := lostIssue(t, Derive(postureBuild(3, postureHeron), c), IssueOptionPrerequisite)
	player, master := is.Lost.SentencePT("Tor", false), is.Lost.SentencePT("Tor", true)
	if !strings.Contains(player, "pede uma condição que a mesa não mostra") || !strings.HasSuffix(player, ": a opção só vale de novo quando Tor cumprir.") {
		t.Errorf("player's sentence = %q", player)
	}
	for _, w := range words {
		if strings.Contains(player, w) || strings.Contains(is.Message, w) {
			t.Errorf("the player's sentence or the issue's Message names %q: %q / %q", w, player, is.Message)
		}
	}
	for _, w := range words[:4] {
		if !strings.Contains(master, w) {
			t.Errorf("the master's sentence does not name %q: %q", w, master)
		}
	}

	pv := lostIssue(t, Derive(postureBuild(3, postureHeron), c.ForPlayer()), IssueOptionPrerequisite)
	for _, w := range words {
		if s := pv.Lost.SentencePT("Tor", true); strings.Contains(s, w) {
			t.Errorf("a player's view names %q even read as the master's: %q", w, s)
		}
	}
}

// Preflight #253, A6: the exact shape of the list. Several conditions a player may not read are one phrase, and the
// last of the list whatever their place in the prerequisite (its place would say which kind it stands for); three
// conditions read "A, B e C".
func TestTheListOfMissingConditions(t *testing.T) {
	t.Parallel()
	score := LostCondition{AskPT: "Força 13", LinkPT: "e", HasPT: "tem 11"}
	level := LostCondition{AskPT: "nível 8", LinkPT: "e", HasPT: "está no 4"}
	spell := LostCondition{AskPT: "a magia Raio", LinkPT: "que", HasPT: "não conhece mais"}
	class := LostCondition{AskPT: "Ordem 3", LinkPT: "e", HasPT: "não tem nível de Ordem", Hidden: true}
	pact := LostCondition{AskPT: "a Dádiva do Pacto Elo", LinkPT: "que", HasPT: "não tem mais", Hidden: true}
	races := LostCondition{AskPT: "ser Oculto ou Rubro", ShownPT: "ser Rubro", LinkPT: "que", HasPT: "não é", Hidden: true}
	const tail = ": o talento só vale de novo quando Tor cumprir."
	cases := []struct {
		name           string
		conditions     []LostCondition
		player, master string
	}{
		{
			"three conditions",
			[]LostCondition{score, level, spell},
			"X pede Força 13, e Tor tem 11, nível 8, e Tor está no 4 e a magia Raio, que Tor não conhece mais" + tail, "",
		},
		{
			"several hidden conditions are one phrase",
			[]LostCondition{class, pact},
			"X pede uma condição que a mesa não mostra" + tail,
			"X pede Ordem 3, e Tor não tem nível de Ordem e a Dádiva do Pacto Elo, que Tor não tem mais" + tail,
		},
		{
			"a hidden condition among the others is told last",
			[]LostCondition{class, score, pact, level},
			"X pede Força 13, e Tor tem 11, nível 8, e Tor está no 4 e uma condição que a mesa não mostra" + tail,
			"X pede Ordem 3, e Tor não tem nível de Ordem, Força 13, e Tor tem 11, a Dádiva do Pacto Elo, que Tor não tem mais e nível 8, e Tor está no 4" + tail,
		},
		{
			"a list of races with a hidden one names the others, as the picker does",
			[]LostCondition{races, score},
			"X pede ser Rubro, que Tor não é e Força 13, e Tor tem 11" + tail,
			"X pede ser Oculto ou Rubro, que Tor não é e Força 13, e Tor tem 11" + tail,
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			l := LostPrerequisite{EntryPT: "X", Conditions: tc.conditions}
			if got := l.SentencePT("Tor", false); got != tc.player {
				t.Errorf("a player reads %q, want %q", got, tc.player)
			}
			if want := cmp.Or(tc.master, tc.player); l.SentencePT("Tor", true) != want {
				t.Errorf("the master reads %q, want %q", l.SentencePT("Tor", true), want)
			}
		})
	}
}

// Preflight #253, A5: the sentence says the score the sheet shows. A feat that gives Strength +1 and is lost gives
// nothing: the feat that asks for Strength 15 says the 13 of the sheet, not the 14 it would have with the other.
func TestALostFeatSaysTheScoreTheSheetShows(t *testing.T) {
	t.Parallel()
	const giver, asker = "feat:da-forca" + tableSuffix, "feat:pede-forca" + tableSuffix
	feat := func(key, name string, p FeatPrerequisite, effects ...Effect) TableFeat {
		return TableFeat{TableEntry: TableEntry{Key: key, NamePT: name}, DescPT: []string{"Texto."}, Prerequisite: p, Effects: effects}
	}
	c := withOverlayOn(t, loadForTest(t), Overlay{Revision: 1, Feats: []TableFeat{
		feat(giver, "Dá força", FeatPrerequisite{Level: 20}, Effect{Type: "modifier", Target: "score.str", Mode: "add", Value: "1", Cap: 20}),
		feat(asker, "Pede força", FeatPrerequisite{Minimums: map[Ability]int{STR: 15}}),
	}})
	b := postureBuild(3)
	b.Classes[0].Subclass = ""
	b.BaseScores[STR] = 12 // 13 with the race; 14 if the lost feat gave its +1
	b.Feats = []string{giver, asker}
	d := Derive(b, c)
	shown := 0
	for _, s := range d.Abilities {
		if s.Ability == STR {
			shown = s.Score
		}
	}
	var got string
	for _, is := range d.Issues {
		if is.Lost != nil && is.Lost.Key == asker {
			got = is.Lost.SentencePT("Tor", false)
		}
	}
	if shown != 13 {
		t.Fatalf("the sheet shows Strength %d, want 13: the lost feat gives nothing", shown)
	}
	const want = "Pede força pede Força 15, e Tor tem 13: o talento só vale de novo quando Tor cumprir."
	if got != want {
		t.Errorf("sentence = %q, want %q", got, want)
	}
}
