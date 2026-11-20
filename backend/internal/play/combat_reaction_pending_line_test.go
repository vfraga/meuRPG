package play

import (
	"strings"
	"testing"

	charactersv1 "github.com/PuraFome/meuRPG/backend/gen/meurpg/characters/v1"
	playv1 "github.com/PuraFome/meuRPG/backend/gen/meurpg/play/v1"
	rulesv1 "github.com/PuraFome/meuRPG/backend/gen/meurpg/rules/v1"
)

// The line of a Cutting Words that cut a held roll whose replay asked its roller (a halfling's Lucky, a Bardic Inspiration die the
// roller holds) says nothing of the roll until the answer settles it: the master reads the die and that the roll is awaited, a player
// the neutral "diminuiu", the same text whatever the question. Once the answer is written the line is the one a roll that asked nothing
// has, and an undo of the answer puts it back to waiting. Events are never changed: the line is worked out when it is read.

// cutPendingMaster and cutPendingPlayers are the lines while the roller's question waits.
const (
	cutPendingMaster  = "Bardo usou Palavras de Interrupção: d6 (5) subtraído do ataque de Pipo (aguardando a rolagem)."
	cutPendingPlayers = "Bardo usou Palavras de Interrupção: o ataque de Pipo diminuiu."
)

// cutWith answers a Cutting Words window by using the die, typed.
func cutWith(face int32) func(*playv1.AnswerReactionRequest) {
	return func(r *playv1.AnswerReactionRequest) {
		r.Answer, r.Roll = playv1.ReactionChoice_REACTION_CHOICE_USE, &playv1.AnswerReactionRequest_Typed{Typed: face}
	}
}

// bardTable is the party of newLuckyTable (Pipo the halfling, Caio's; Toren, Ana's; Orla the bard, Bia's) against an enemy lore bard, Bardo,
// next to Pipo, on Pipo's turn. Bardo's armor class is 12; Pipo's battleaxe adds 5.
func bardTable(t *testing.T) (*armed, *playv1.Encounter) {
	t.Helper()
	fighter := &rulesv1.AbilityScores{Strength: 16, Dexterity: 12, Constitution: 14, Intelligence: 10, Wisdom: 10, Charisma: 10}
	var bard *charactersv1.Character
	a := newArmedWith(t, func(a *armed) {
		a.toren = a.caio.hero(t, a.campaignID, "Pipo", "class:fighter", "race:halfling", 2, fighter, []string{battleaxe}, nil)
		a.pens = a.ana.hero(t, a.campaignID, "Toren", "class:fighter", "race:human", 2, fighter, []string{battleaxe}, nil)
		a.bri = a.bia.hero(t, a.campaignID, "Orla", "class:bard", "race:human", 5,
			&rulesv1.AbilityScores{Strength: 8, Dexterity: 14, Constitution: 14, Intelligence: 10, Wisdom: 10, Charisma: 16}, nil, nil)
		bard = loreBardEnemy(t, a)
	})
	e := a.start(t, plan{
		npcs: []*playv1.Participant{{CharacterId: bard.GetId()}}, npcRolls: []int{3},
		players: map[string]int32{"Pipo": 20, "Toren": 14, "Orla": 10}, reveal: []string{"Bardo"},
		at: map[string][2]int32{"Pipo": {4, 3}, "Toren": {3, 4}, "Orla": {5, 3}, "Bardo": {4, 4}},
	})
	return a, e
}

// cutLinesOf are the lines of reactions u reads of the combat's log, the latest first.
func (a *armed) cutLinesOf(t *testing.T, u *user, e *playv1.Encounter) []string {
	t.Helper()
	var out []string
	for _, l := range a.logEntries(t, u, e) {
		if l.GetKind() == playv1.CombatLogKind_COMBAT_LOG_KIND_REACTION_WINDOW {
			out = append(out, l.GetReactionTextPt())
		}
	}
	return out
}

// wantCutLine checks u reads exactly one reaction line, and that it is want.
func (a *armed) wantCutLine(t *testing.T, e *playv1.Encounter, who string, u *user, want string) {
	t.Helper()
	got := a.cutLinesOf(t, u, e)
	if len(got) != 1 || got[0] != want {
		t.Errorf("%s reads the reaction lines %q, want [%q]", who, got, want)
	}
}

// wantCutLines checks the master and the three players read the lines the test says.
func (a *armed) wantCutLines(t *testing.T, e *playv1.Encounter, master, players string) {
	t.Helper()
	a.wantCutLine(t, e, "the master", a.master, master)
	a.wantCutLine(t, e, "Pipo's player", a.caio, players)
	a.wantCutLine(t, e, "Toren's player", a.ana, players)
	a.wantCutLine(t, e, "Orla's player", a.bia, players)
}

// A halfling's 1 that the bard's die left on the roll is asked about when the replay rolls it: the line waits, and says no number or
// outcome, until Lucky is answered. Rolled again to 15, 15 + 5 - 5 = 15 against 12 hits; the line then says what a roll that asked
// nothing says.
func TestACutLineWaitsForTheLuckyAnswerOfTheRollItCut(t *testing.T) {
	t.Parallel()
	a, e := bardTable(t)
	held := a.mustAttack(t, a.caio, e, "Pipo", battleaxe, "Bardo", d20(1))
	if !held.GetRoll().GetHeldForReaction() {
		t.Fatalf("the attack = %v, want it held for the bard's window", held)
	}
	w := a.windowOf(t, a.master, playv1.ReactionKind_REACTION_KIND_CUTTING_WORDS)
	if w == nil {
		t.Fatalf("the master's windows = %v, want the bard's Cutting Words", a.windows(t, a.master))
	}
	a.mustAnswer(t, a.master, e, w.GetId(), cutWith(5))
	offers := a.listHeldRolls(t, a.caio)
	if len(offers) != 1 || offers[0].GetLucky() == nil {
		t.Fatalf("Pipo's questions after the window = %v, want Lucky's", offers)
	}
	a.wantCutLines(t, e, cutPendingMaster, cutPendingPlayers)

	if _, err := a.answerHeldRoll(t, a.caio, offers[0].GetHoldId(), luckyAnswer(15)); err != nil {
		t.Fatalf("AnswerHeldRoll() error = %v", err)
	}
	a.wantCutLines(t, e,
		"Bardo usou Palavras de Interrupção: d6 (5) subtraído do ataque de Pipo: 20 → 15 contra CA 12: acertou.",
		"Bardo usou Palavras de Interrupção: o ataque de Pipo ainda acertou.")
	if strings.Contains(strings.Join(a.cutLinesOf(t, a.master, e), "\n"), "CA 0") {
		t.Error("the master reads an armor class of 0")
	}
}
