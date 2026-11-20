package play

import (
	"encoding/json"
	"fmt"
	"slices"
	"strings"
	"testing"

	"connectrpc.com/connect"

	charactersv1 "github.com/PuraFome/meuRPG/backend/gen/meurpg/characters/v1"
	playv1 "github.com/PuraFome/meuRPG/backend/gen/meurpg/play/v1"
	rulesv1 "github.com/PuraFome/meuRPG/backend/gen/meurpg/rules/v1"
	"github.com/PuraFome/meuRPG/backend/internal/play/playdb"
	"github.com/PuraFome/meuRPG/backend/internal/rules/reaction"
)

// The line of a Cutting Words that cut a held roll whose replay asked its roller, for every ask and every reader
// (combat_reaction_pending_line_test.go has the first case and the words). The roll that asks may be a weapon attack or a spell
// attack, the question Lucky's, the Bardic Inspiration die's (used or kept), and the roller may be a player's character or a creature
// of the master's that holds a die. Whatever the question, a player reads the same words while it waits.

// readers are the people who read the log of a table, by the name a failure gives them, and the line each must read.
type readers map[string]*user

// wantLines checks the master reads exactly one reaction line, master, and every player of the readers reads players.
func (a *armed) wantLines(t *testing.T, e *playv1.Encounter, who readers, master, players string) {
	t.Helper()
	a.wantCutLine(t, e, "the master", a.master, master)
	for name, u := range who {
		a.wantCutLine(t, e, name, u, players)
	}
}

// bardTableReaders are the players of bardTable: the roller's (Pipo's), Toren's, and the bard player's, who has no part in the cut.
func (a *armed) bardTableReaders() readers {
	return readers{"Pipo's player": a.caio, "Toren's player": a.ana, "Orla's player": a.bia}
}

// heldWeaponCut holds Pipo's attack on Bardo for the window, and answers the window with a die of 5: the replay then asks Pipo. It
// returns the question that waits.
func heldWeaponCut(t *testing.T, a *armed, e *playv1.Encounter, roll func(*playv1.RollAttackRequest)) *playv1.HeldRollOffer {
	t.Helper()
	held := a.mustAttack(t, a.caio, e, "Pipo", battleaxe, "Bardo", roll)
	if !held.GetRoll().GetHeldForReaction() {
		t.Fatalf("the attack = %v, want it held for the bard's window", held)
	}
	w := a.windowOf(t, a.master, playv1.ReactionKind_REACTION_KIND_CUTTING_WORDS)
	if w == nil {
		t.Fatalf("the master's windows = %v, want the bard's Cutting Words", a.windows(t, a.master))
	}
	a.mustAnswer(t, a.master, e, w.GetId(), cutWith(5))
	offers := a.listHeldRolls(t, a.caio)
	if len(offers) != 1 {
		t.Fatalf("Pipo's questions after the window = %v, want one", offers)
	}
	return offers[0]
}

// A Bardic Inspiration die the roller holds is asked about the roll the bard's die left: the line waits, and the die the roller adds
// lands in the total the line then reads (10 + 5 - 5 + 4 = 14, a hit against 12), while a die kept leaves 10, a miss.
func TestACutLineWaitsForTheBardicDieOfTheRollItCut(t *testing.T) {
	t.Parallel()
	for _, tc := range []struct {
		name           string
		use            bool
		master, player string
	}{
		{
			"the die used", true,
			"Bardo usou Palavras de Interrupção: d6 (5) subtraído do ataque de Pipo: 19 → 14 contra CA 12: acertou.",
			"Bardo usou Palavras de Interrupção: o ataque de Pipo ainda acertou.",
		},
		{
			"the die kept", false,
			"Bardo usou Palavras de Interrupção: d6 (5) subtraído do ataque de Pipo: 15 → 10 contra CA 12: errou.",
			"Bardo usou Palavras de Interrupção: o ataque de Pipo errou.",
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			a, e := bardTable(t)
			a.giveCombatantDie(t, "Pipo", e)
			offer := heldWeaponCut(t, a, e, d20(10))
			if offer.GetDie() == nil {
				t.Fatalf("Pipo's question = %v, want the die's", offer)
			}
			a.wantLines(t, e, a.bardTableReaders(), cutPendingMaster, cutPendingPlayers)
			if tc.use {
				a.h.roller.queue(4)
			}
			if _, err := a.answerHeldRoll(t, a.caio, offer.GetHoldId(), dieAnswer(tc.use)); err != nil {
				t.Fatalf("AnswerHeldRoll() error = %v", err)
			}
			a.wantLines(t, e, a.bardTableReaders(), tc.master, tc.player)
		})
	}
}

// The master's Inspiração, spent with the attack, is no question of its own: it is chosen before the d20 and gives the roll two dice.
// The replay of a held attack that spent it asks Lucky when both dice show a 1, and the line waits for that answer as it does for any.
func TestACutLineWaitsForTheLuckyAnswerOfARollMadeWithTheFavor(t *testing.T) {
	t.Parallel()
	a, e := bardTable(t)
	f := favorFightOf(t, a, e)
	f.give(t, a.toren)
	offer := heldWeaponCut(t, a, e, func(r *playv1.RollAttackRequest) { r.D20Faces, r.SpendFavor = []int32{1, 1}, true })
	if offer.GetLucky() == nil {
		t.Fatalf("Pipo's question = %v, want Lucky's", offer)
	}
	a.wantLines(t, e, a.bardTableReaders(), cutPendingMaster, cutPendingPlayers)
	if _, err := a.answerHeldRoll(t, a.caio, offer.GetHoldId(), luckyAnswer(15)); err != nil {
		t.Fatalf("AnswerHeldRoll() error = %v", err)
	}
	a.wantLines(t, e, a.bardTableReaders(),
		"Bardo usou Palavras de Interrupção: d6 (5) subtraído do ataque de Pipo: 20 → 15 contra CA 12: acertou.",
		"Bardo usou Palavras de Interrupção: o ataque de Pipo ainda acertou.")
	if f.favorHolds(t) || f.eventFavor(t, eventAttackRolled) != "true" {
		t.Errorf("the Inspiração is there = %v, the attack says %q, want it spent and the roll to say so", f.favorHolds(t), f.eventFavor(t, eventAttackRolled))
	}
}

// What a player reads while the roll waits is the same words whatever it waits for: Lucky, the Bardic Inspiration die, or the roll the
// Inspiração made. The pending line tells the roller nothing their own question does not.
func TestThePendingLineOfACutIsTheSameForEveryQuestion(t *testing.T) {
	t.Parallel()
	asks := []struct {
		name string
		set  func(t *testing.T, a *armed, e *playv1.Encounter) func(*playv1.RollAttackRequest)
		asks func(*playv1.HeldRollOffer) bool
	}{
		{"the Bardic die", func(t *testing.T, a *armed, e *playv1.Encounter) func(*playv1.RollAttackRequest) {
			t.Helper()
			a.giveCombatantDie(t, "Pipo", e)
			return d20(10)
		}, func(o *playv1.HeldRollOffer) bool { return o.GetDie() != nil }},
		{"Lucky asked after the Inspiração was spent with the roll", func(t *testing.T, a *armed, e *playv1.Encounter) func(*playv1.RollAttackRequest) {
			t.Helper()
			favorFightOf(t, a, e).give(t, a.toren)
			return func(r *playv1.RollAttackRequest) { r.D20Faces, r.SpendFavor = []int32{1, 1}, true }
		}, func(o *playv1.HeldRollOffer) bool { return o.GetLucky() != nil }},
		{
			"Lucky", func(*testing.T, *armed, *playv1.Encounter) func(*playv1.RollAttackRequest) { return d20(1) },
			func(o *playv1.HeldRollOffer) bool { return o.GetLucky() != nil },
		},
	}
	read := map[string][]string{}
	for _, ask := range asks {
		a, e := bardTable(t)
		if offer := heldWeaponCut(t, a, e, ask.set(t, a, e)); !ask.asks(offer) {
			t.Fatalf("%s: Pipo's question = %v, not the one the case is about", ask.name, offer)
		}
		for name, u := range a.bardTableReaders() {
			read[name] = append(read[name], strings.Join(a.cutLinesOf(t, u, e), "\n"))
		}
	}
	for name, lines := range read {
		if len(lines) != len(asks) || slices.Compact(slices.Clone(lines))[0] != cutPendingPlayers || len(slices.Compact(slices.Clone(lines))) != 1 {
			t.Errorf("%s reads %q across the Bardic die, Lucky after the Inspiração and Lucky, want %q every time", name, lines, cutPendingPlayers)
		}
	}
}

// spellBardTable is a table of a gnome or halfling wizard (Pensantus, Ana's) who casts Acid Arrow at an enemy lore bard, Bardo, whose
// window holds the cast. Bardo's armor class is 12.
func spellBardTable(t *testing.T, race string) (*armed, *playv1.Encounter) {
	t.Helper()
	var bard *charactersv1.Character
	a := newArmedWith(t, func(a *armed) {
		a.toren = a.caio.hero(t, a.campaignID, "Toren", "class:fighter", "race:human", 2, scores16(), []string{battleaxe}, nil)
		a.pens = a.ana.caster(t, a.campaignID, "Pensantus", "class:wizard", race, 3,
			&rulesv1.AbilityScores{Strength: 10, Dexterity: 14, Constitution: 12, Intelligence: 16, Wisdom: 10, Charisma: 8}, nil, []string{fireBolt},
			[]string{acidArrow}, []string{acidArrow})
		a.bri = a.bia.hero(t, a.campaignID, "Brisa", "class:fighter", "race:human", 2, scores16(), []string{rapier}, nil)
		bard = loreBardEnemy(t, a)
	})
	e := a.start(t, plan{
		npcs: []*playv1.Participant{{CharacterId: bard.GetId()}}, npcRolls: []int{3},
		players: map[string]int32{"Toren": 18, "Pensantus": 10, "Brisa": 1}, reveal: []string{"Bardo"},
		at: map[string][2]int32{"Toren": {3, 3}, "Bardo": {4, 3}, "Pensantus": {10, 3}, "Brisa": {4, 9}},
	})
	turn := a.passTo(t, e, "Pensantus")
	return a, turn
}

// spellCut holds Pensantus's cast at Bardo for the window and answers it with a die of 5, and returns the question that waits for her.
func spellCut(t *testing.T, a *armed, e *playv1.Encounter, face int32) *playv1.HeldRollOffer {
	t.Helper()
	held, err := a.castKey(t, a.ana, e, "Pensantus", acidArrow, slotOfLevel(2), a.at(t, "Bardo"), d20Face(face), newKey())
	if err != nil || held.GetCast() != nil || held.GetHeldRollOffer() != nil {
		t.Fatalf("CastSpell() = %v, %v; want it held in the bard's window", held, err)
	}
	w := a.windowOf(t, a.master, playv1.ReactionKind_REACTION_KIND_CUTTING_WORDS)
	if w == nil {
		t.Fatalf("the master's windows = %v, want the bard's Cutting Words", a.windows(t, a.master))
	}
	a.mustAnswer(t, a.master, e, w.GetId(), cutWith(5))
	offers := a.listHeldRolls(t, a.ana)
	if len(offers) != 1 {
		t.Fatalf("Pensantus's questions after the window = %v, want one", offers)
	}
	return offers[0]
}

// spellReaders are the players of spellBardTable: the caster's, and the other two.
func (a *armed) spellReaders() readers {
	return readers{"Pensantus's player": a.ana, "Toren's player": a.caio, "Brisa's player": a.bia}
}

const (
	spellPendingMaster  = "Bardo usou Palavras de Interrupção: d6 (5) subtraído do ataque de Pensantus (aguardando a rolagem)."
	spellPendingPlayers = "Bardo usou Palavras de Interrupção: o ataque de Pensantus diminuiu."
)

// A halfling's spell attack that the bard's die left on a 1: the line of the cut waits for Lucky's answer, then says what a spell attack
// that asked nothing says: 15 + 5 - 5 = 15 against 12.
func TestACutLineWaitsForTheLuckyAnswerOfASpellAttack(t *testing.T) {
	t.Parallel()
	a, e := spellBardTable(t, "race:halfling")
	offer := spellCut(t, a, e, 1)
	if offer.GetLucky() == nil {
		t.Fatalf("Pensantus's question = %v, want Lucky's", offer)
	}
	a.wantLines(t, e, a.spellReaders(), spellPendingMaster, spellPendingPlayers)
	if _, err := a.answerHeldRoll(t, a.ana, offer.GetHoldId(), luckyAnswer(15)); err != nil {
		t.Fatalf("AnswerHeldRoll() error = %v", err)
	}
	a.wantLines(t, e, a.spellReaders(),
		"Bardo usou Palavras de Interrupção: d6 (5) subtraído do ataque de Pensantus: 20 → 15 contra CA 12: acertou.",
		"Bardo usou Palavras de Interrupção: o ataque de Pensantus ainda acertou.")
}

// A gnome's spell attack with a Bardic Inspiration die: used (15 + 6 - 5 + 4 = 20, a hit) and kept (16, a hit), the line waiting for
// either.
func TestACutLineWaitsForTheBardicDieOfASpellAttack(t *testing.T) {
	t.Parallel()
	for _, tc := range []struct {
		name           string
		use            bool
		master, player string
	}{
		{
			"the die used", true,
			"Bardo usou Palavras de Interrupção: d6 (5) subtraído do ataque de Pensantus: 25 → 20 contra CA 12: acertou.",
			"Bardo usou Palavras de Interrupção: o ataque de Pensantus ainda acertou.",
		},
		{
			"the die kept", false,
			"Bardo usou Palavras de Interrupção: d6 (5) subtraído do ataque de Pensantus: 21 → 16 contra CA 12: acertou.",
			"Bardo usou Palavras de Interrupção: o ataque de Pensantus ainda acertou.",
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			a, e := spellBardTable(t, "race:gnome")
			a.giveCombatantDie(t, "Pensantus", e)
			offer := spellCut(t, a, e, 15)
			if offer.GetDie() == nil {
				t.Fatalf("Pensantus's question = %v, want the die's", offer)
			}
			a.wantLines(t, e, a.spellReaders(), spellPendingMaster, spellPendingPlayers)
			if tc.use {
				a.h.roller.queue(4)
			}
			if _, err := a.answerHeldRoll(t, a.ana, offer.GetHoldId(), dieAnswer(tc.use)); err != nil {
				t.Fatalf("AnswerHeldRoll() error = %v", err)
			}
			a.wantLines(t, e, a.spellReaders(), tc.master, tc.player)
		})
	}
}

// goblinCut is a table where the bard is a player (Ana's Reator): the master's goblin holds a Bardic Inspiration die and attacks Toren,
// the bard cuts the attack with a die of 5, and the replay asks the master, who rolls for the goblin. It returns the question.
func goblinCut(t *testing.T) (*armed, *playv1.Encounter, *playv1.HeldRollOffer) {
	t.Helper()
	a, e := reactorFight(t, loreBard)
	a.giveCombatantDie(t, "Goblin", e)
	w := bardWindow(t, a, e, 8)
	a.mustAnswer(t, a.ana, e, w.GetId(), typedAnswer(5))
	offers := a.listHeldRolls(t, a.master)
	if len(offers) != 1 || offers[0].GetDie() == nil {
		t.Fatalf("the master's questions after the cut = %v, want the goblin's die", offers)
	}
	return a, e, offers[0]
}

// goblinReaders are the players of reactorFight: the bard's, the target's and the third.
func (a *armed) goblinReaders() readers {
	return readers{"the bard's player": a.ana, "Toren's player": a.caio, "Brisa's player": a.bia}
}

const (
	goblinPendingMaster  = "Reator usou Palavras de Interrupção: d6 (5) subtraído do ataque de Goblin (aguardando a rolagem)."
	goblinPendingPlayers = "Reator usou Palavras de Interrupção: o ataque de Goblin diminuiu."
)

// A creature of the master's that holds a die asks the master, and the bard's own player reads no more than the other players do while
// the question waits. The goblin's total is 8 + 4 - 5 = 7; the master's answer settles it.
func TestACutLineOfAPlayersBardWaitsForTheMastersAnswer(t *testing.T) {
	t.Parallel()
	a, e, offer := goblinCut(t)
	a.wantLines(t, e, a.goblinReaders(), goblinPendingMaster, goblinPendingPlayers)
	if _, err := a.answerHeldRoll(t, a.master, offer.GetHoldId(), dieAnswer(false)); err != nil {
		t.Fatalf("AnswerHeldRoll() error = %v", err)
	}
	a.wantLines(t, e, a.goblinReaders(),
		"Reator usou Palavras de Interrupção: d6 (5) subtraído do ataque de Goblin: 12 → 7 contra CA 12: errou.",
		"Reator usou Palavras de Interrupção: o ataque de Goblin errou.")
}

// A roller the master hides before the answer is a roller the players do not hear: the line stays what it was for them, whatever the roll
// became, with no total, armor class or outcome; the master reads the completed line.
func TestACutLineOfAHiddenRollerStaysNeutralForThePlayers(t *testing.T) {
	t.Parallel()
	a, e, offer := goblinCut(t)
	setHidden(t, a, e, "Goblin", true)
	if _, err := a.answerHeldRoll(t, a.master, offer.GetHoldId(), dieAnswer(false)); err != nil {
		t.Fatalf("AnswerHeldRoll() error = %v", err)
	}
	a.wantCutLine(t, e, "the master", a.master,
		"Reator usou Palavras de Interrupção: d6 (5) subtraído do ataque de Goblin: 12 → 7 contra CA 12: errou.")
	for name, u := range a.goblinReaders() {
		a.wantCutLine(t, e, name, u, "Reator usou Palavras de Interrupção: o ataque diminuiu.") // the goblin is hidden: no name, no total, no outcome
		for _, l := range a.logEntries(t, u, e) {
			if l.GetKind() == playv1.CombatLogKind_COMBAT_LOG_KIND_ATTACK {
				t.Errorf("%s reads the hidden goblin's attack %v", name, l)
			}
		}
	}
}

// A roller hidden from the start cannot be cut: the bard does not see it, so no window holds the attack and no cut line is written for
// anyone, the master included.
func TestAHiddenRollerFromTheStartIsNotCut(t *testing.T) {
	t.Parallel()
	a, e := reactorFight(t, loreBard)
	setHidden(t, a, e, "Goblin", true)
	held := a.mustAttack(t, a.master, e, "Goblin", sword, "Toren", d20(8))
	if held.GetRoll().GetHeldForReaction() || a.windowOf(t, a.ana, playv1.ReactionKind_REACTION_KIND_CUTTING_WORDS) != nil {
		t.Fatalf("the hidden goblin's attack = %v, want no Cutting Words window", held.GetRoll())
	}
	for name, u := range map[string]*user{"the master": a.master, "the bard's player": a.ana, "Toren's player": a.caio, "Brisa's player": a.bia} {
		if got := a.cutLinesOf(t, u, e); len(got) != 0 {
			t.Errorf("%s reads the cut lines %q of a roller that was never cut", name, got)
		}
	}
}

// reactionEntry is the reaction entry of the log response, nil when it holds none.
func reactionEntry(res *playv1.ListCombatLogResponse) *playv1.CombatLogEntry {
	for _, r := range res.GetRounds() {
		for _, l := range r.GetEntries() {
			if l.GetKind() == playv1.CombatLogKind_COMBAT_LOG_KIND_REACTION_WINDOW {
				return l
			}
		}
	}
	return nil
}

// since reads the log of u after the number.
func (a *armed) since(t *testing.T, u *user, e *playv1.Encounter, n int64) *playv1.ListCombatLogResponse {
	t.Helper()
	res, err := u.combat.ListCombatLog(t.Context(), connect.NewRequest(&playv1.ListCombatLogRequest{CampaignId: a.campaignID, EncounterId: e.GetId(), SinceSeq: n}))
	if err != nil {
		t.Fatalf("ListCombatLog(since %d) error = %v", n, err)
	}
	return res.Msg
}

// A reader who holds the pending line and asks for what changed after the answer is given the completed one, as a reader who opens the log
// then is; a reader who opens it between the two reads the pending one.
func TestACutLineIsCompletedInTheDeltaOfTheAnswer(t *testing.T) {
	t.Parallel()
	a, e := bardTable(t)
	offer := heldWeaponCut(t, a, e, d20(1))
	if offer.GetLucky() == nil {
		t.Fatalf("Pipo's question = %v, want Lucky's", offer)
	}
	held := map[string]int64{}
	users := map[string]*user{"the master": a.master, "Pipo's player": a.caio, "Toren's player": a.ana, "Orla's player": a.bia}
	for name, u := range users {
		first := a.log(t, u, e)
		want := cutPendingPlayers
		if u == a.master {
			want = cutPendingMaster
		}
		if got := reactionEntry(first).GetReactionTextPt(); got != want {
			t.Errorf("%s opens the log in the middle and reads %q, want %q", name, got, want)
		}
		held[name] = first.GetLastSeq()
	}
	if _, err := a.answerHeldRoll(t, a.caio, offer.GetHoldId(), luckyAnswer(15)); err != nil {
		t.Fatalf("AnswerHeldRoll() error = %v", err)
	}
	for name, u := range users {
		delta := a.since(t, u, e, held[name])
		got := reactionEntry(delta).GetReactionTextPt()
		want := "Bardo usou Palavras de Interrupção: o ataque de Pipo ainda acertou."
		if u == a.master {
			want = "Bardo usou Palavras de Interrupção: d6 (5) subtraído do ataque de Pipo: 20 → 15 contra CA 12: acertou."
		}
		if got != want {
			t.Errorf("%s reads %q in the delta after the answer (full = %v), want %q", name, got, delta.GetFull(), want)
		}
		if delta.GetFull() {
			t.Errorf("%s is answered with the whole log, want a delta: the roll that completes the line is a kind a delta may carry", name)
		}
	}
}

// The undo of the roll the answer wrote puts the line back to waiting, for the master and for the players. The reaction's own event is not
// one an undo takes back (it is not in undoableKinds): it is never the action the log offers, and asking to undo it is refused.
func TestUndoingTheAnsweredRollPutsTheCutLineBackToWaiting(t *testing.T) {
	t.Parallel()
	a, e := bardTable(t)
	offer := heldWeaponCut(t, a, e, d20(1))
	if offer.GetLucky() == nil {
		t.Fatalf("Pipo's question = %v, want Lucky's", offer)
	}
	if _, err := a.answerHeldRoll(t, a.caio, offer.GetHoldId(), luckyAnswer(15)); err != nil {
		t.Fatalf("AnswerHeldRoll() error = %v", err)
	}
	a.undoLast(t, e)
	a.wantLines(t, e, a.bardTableReaders(), cutPendingMaster, cutPendingPlayers)

	var reactionID string
	if err := a.h.pool.QueryRow(t.Context(), `SELECT id FROM session_events WHERE kind = $1`, eventReactionAnswered).Scan(&reactionID); err != nil {
		t.Fatalf("read the reaction's event: %v", err)
	}
	if got := a.log(t, a.master, e).GetUndoableEventId(); got == reactionID {
		t.Errorf("the log offers the reaction's own event %s to undo", got)
	}
	err := a.undo(t, a.master, e, reactionID)
	if err == nil {
		t.Fatal("UndoLastAction() of the reaction's own event succeeded, want it refused")
	}
	if connect.CodeOf(err) != connect.CodeFailedPrecondition {
		t.Errorf("UndoLastAction() of the reaction's own event error = %v, want failed_precondition", err)
	}
	a.wantLines(t, e, a.bardTableReaders(), cutPendingMaster, cutPendingPlayers)
}

// A player who does not see the roller through the fog does not hear of the roll that completes the line: it stays what it was for
// them, whatever the roll became, while a player who saw it and the master read the completed one (the positive controls).
func TestACutLineOfAFoggedRollerStaysPendingForWhoDidNotSeeIt(t *testing.T) {
	t.Parallel()
	row := func(id string, seq int32, kind, payload string) playdb.ListEncounterEventsRow {
		return playdb.ListEncounterEventsRow{ID: id, Seq: seq, Kind: kind, Payload: []byte(payload)}
	}
	events := []playdb.ListEncounterEventsRow{
		row("cut", 1, eventReactionAnswered, fmt.Sprintf(`{"round":1,"combatant_id":"c-bard","target_id":"c-roller","reaction":{"kind":%q,"used":true,"roll":"attack","die":5,"die_sides":6,"waiting":true,"group":"g","reduction":5}}`, reaction.CuttingWords)),
		row("roll", 2, eventAttackRolled, `{"round":1,"combatant_id":"c-roller","target_id":"c-target","cut_group":"g","outcome":"miss","total":7,"target_ac":12,"fogged":true,"seen_by":["saw"]}`),
	}
	var cut *logEntry
	for _, en := range buildLog(events) {
		if en.kind == playv1.CombatLogKind_COMBAT_LOG_KIND_REACTION_WINDOW {
			cut = en
		}
	}
	if cut == nil {
		t.Fatal("the log has no reaction line")
	}
	for name, tc := range map[string]struct {
		v    combatViewer
		done bool
	}{
		"the master":           {combatViewer{userID: "m", master: true}, true},
		"a player who saw it":  {combatViewer{userID: "saw"}, true},
		"a player who did not": {combatViewer{userID: "blind"}, false},
	} {
		got := cut.reactionFor(tc.v)
		if got.Waiting == tc.done || (tc.done && (got.After != 7 || got.Before != 12 || got.AC != 12 || got.Outcome != outcomeMiss)) {
			t.Errorf("%s reads %+v, want the line completed = %v (12 → 7 against 12, a miss)", name, got, tc.done)
		}
	}
}

// Lucky answered, then the Bardic die on the same hold: the line is still pending after Lucky's answer, which writes nothing, and
// completes only on the event the die's answer writes.
func TestACutLineCompletesOnlyOnTheFinalEventOfChainedQuestions(t *testing.T) {
	t.Parallel()
	a, e := bardTable(t)
	a.giveCombatantDie(t, "Pipo", e)
	offer := heldWeaponCut(t, a, e, d20(1))
	if offer.GetLucky() == nil {
		t.Fatalf("Pipo's first question = %v, want Lucky's", offer)
	}
	if _, err := a.answerHeldRoll(t, a.caio, offer.GetHoldId(), luckyAnswer(15)); err != nil {
		t.Fatalf("AnswerHeldRoll(Lucky) error = %v", err)
	}
	next := a.listHeldRolls(t, a.caio)
	if len(next) != 1 || next[0].GetDie() == nil {
		t.Fatalf("Pipo's questions after Lucky = %v, want the die's", next)
	}
	a.wantLines(t, e, a.bardTableReaders(), cutPendingMaster, cutPendingPlayers)
	if _, err := a.answerHeldRoll(t, a.caio, next[0].GetHoldId(), dieAnswer(false)); err != nil {
		t.Fatalf("AnswerHeldRoll(die) error = %v", err)
	}
	a.wantLines(t, e, a.bardTableReaders(),
		"Bardo usou Palavras de Interrupção: d6 (5) subtraído do ataque de Pipo: 20 → 15 contra CA 12: acertou.",
		"Bardo usou Palavras de Interrupção: o ataque de Pipo ainda acertou.")
}

// twoBards is a goblin's attack on Toren that two lore bards cut, Reator by 5 and Brisa by 3, each in a change of its own. With the
// goblin holding a die its replay asks the master; without it the second answer writes the roll. The goblin's 8 + 4 became 12 - 8 = 4
// against 12.
func twoBards(t *testing.T, asks bool) (*armed, *playv1.Encounter) {
	t.Helper()
	a := newArmedWith(t, func(a *armed) {
		a.toren = a.caio.hero(t, a.campaignID, "Toren", "class:fighter", "race:human", 5, abilities(16, 13, 14, 10, 8), []string{battleaxe}, nil)
		a.pens = a.ana.heroWith(t, a.campaignID, "Reator", classLevel("class:bard", 3, "subclass:lore"), abilities(10, 14, 12, 10, 16), []string{rapier}, nil, nil)
		a.bri = a.bia.heroWith(t, a.campaignID, "Brisa", classLevel("class:bard", 3, "subclass:lore"), abilities(10, 14, 12, 10, 16), []string{rapier}, nil, nil)
	})
	e := a.start(t, plan{
		npcs: []*playv1.Participant{{CharacterId: a.goblin.GetId()}}, npcRolls: []int{3},
		players: map[string]int32{"Toren": 18, "Reator": 10, "Brisa": 1}, reveal: []string{"Goblin"},
		at: map[string][2]int32{"Toren": {3, 3}, "Goblin": {4, 3}, "Reator": {4, 4}, "Brisa": {5, 4}},
	})
	e = a.passTo(t, e, "Goblin")
	if asks {
		a.giveCombatantDie(t, "Goblin", e)
	}
	a.mustAttack(t, a.master, e, "Goblin", sword, "Toren", d20(8))
	first := a.windowOf(t, a.ana, playv1.ReactionKind_REACTION_KIND_CUTTING_WORDS)
	if first == nil {
		t.Fatal("the first bard has no window")
	}
	a.mustAnswer(t, a.ana, e, first.GetId(), typedAnswer(5))
	second := a.windowOf(t, a.bia, playv1.ReactionKind_REACTION_KIND_CUTTING_WORDS)
	if second == nil {
		t.Fatal("the second bard has no window")
	}
	a.mustAnswer(t, a.bia, e, second.GetId(), typedAnswer(3))
	return a, e
}

// wantBothBards checks what the master and the three players read of the two bards' lines: waiting, or completed on the one roll.
func (a *armed) wantBothBards(t *testing.T, e *playv1.Encounter, done bool) {
	t.Helper()
	lines := func(u *user) []string {
		return slices.DeleteFunc(a.cutLinesOf(t, u, e), func(l string) bool { return !strings.HasPrefix(l, "Reator ") && !strings.HasPrefix(l, "Brisa ") })
	}
	want := func(master bool) []string {
		var out []string
		for _, bard := range []struct {
			name string
			die  int
		}{{"Brisa", 3}, {"Reator", 5}} {
			switch {
			case master && !done:
				out = append(out, fmt.Sprintf("%s usou Palavras de Interrupção: d6 (%d) subtraído do ataque de Goblin (aguardando a rolagem).", bard.name, bard.die))
			case master:
				out = append(out, fmt.Sprintf("%s usou Palavras de Interrupção: d6 (%d) subtraído do ataque de Goblin: 12 → 4 contra CA 12: errou.", bard.name, bard.die))
			case !done:
				out = append(out, bard.name+" usou Palavras de Interrupção: o ataque de Goblin diminuiu.")
			default:
				out = append(out, bard.name+" usou Palavras de Interrupção: o ataque de Goblin errou.")
			}
		}
		return out
	}
	if got := lines(a.master); !slices.Equal(got, want(true)) {
		t.Errorf("the master reads %q, want %q", got, want(true))
	}
	for name, u := range a.goblinReaders() {
		if got := lines(u); !slices.Equal(got, want(false)) {
			t.Errorf("%s reads %q, want %q", name, got, want(false))
		}
	}
}

// answerGoblinsDie settles the goblin's die with the master's answer.
func (a *armed) answerGoblinsDie(t *testing.T) {
	t.Helper()
	offers := a.listHeldRolls(t, a.master)
	if len(offers) != 1 || offers[0].GetDie() == nil {
		t.Fatalf("the master's questions = %v, want the goblin's die", offers)
	}
	if _, err := a.answerHeldRoll(t, a.master, offers[0].GetHoldId(), dieAnswer(false)); err != nil {
		t.Fatalf("AnswerHeldRoll() error = %v", err)
	}
}

// Two bards cut one held roll that asks its roller: both lines wait, and both complete on the one roll with the whole of what the group
// took off (5 + 3). The first bard's answer was written before the replay, in a change of its own, and the roll completes it all the same.
func TestTwoCutLinesOfOneHeldRollCompleteTogether(t *testing.T) {
	t.Parallel()
	a, e := twoBards(t, true)
	a.wantBothBards(t, e, false)
	a.answerGoblinsDie(t)
	a.wantBothBards(t, e, true)
}

// Two bards cut one held roll that asks nothing: the second answer writes the roll in its own change, and the line of the first bard,
// written before it, reads what it became as the second's does.
func TestTwoCutLinesOfARollThatAsksNothingCompleteTogether(t *testing.T) {
	t.Parallel()
	a, e := twoBards(t, false)
	a.wantBothBards(t, e, true)
}

// The undo of the roll the master's answer wrote takes both lines back to waiting. When the second answer wrote the roll, the answers' own
// events come after it and close the chain of what an undo takes back: the log offers nothing to undo, and both lines stay as they read.
func TestUndoingTheRollOfTwoCutsPutsBothLinesBackToWaiting(t *testing.T) {
	t.Parallel()
	a, e := twoBards(t, true)
	a.answerGoblinsDie(t)
	a.wantBothBards(t, e, true)
	a.undoLast(t, e)
	a.wantBothBards(t, e, false)
}

func TestTheRollOfTwoCutsThatAskedNothingIsNotUndone(t *testing.T) {
	t.Parallel()
	a, e := twoBards(t, false)
	if id := a.log(t, a.master, e).GetUndoableEventId(); id != "" {
		t.Errorf("the log offers %s to undo, want nothing: the answers' events close the chain", id)
	}
	a.wantBothBards(t, e, true)
}

// A roller the master hides before the answer is a roller the players do not hear: both lines stay neutral for them, with no name, total,
// armor class or outcome; the master reads both completed.
func TestTwoCutLinesOfAHiddenRollerStayNeutralForThePlayers(t *testing.T) {
	t.Parallel()
	a, e := twoBards(t, true)
	setHidden(t, a, e, "Goblin", true)
	a.answerGoblinsDie(t)
	for _, bard := range []string{"Reator", "Brisa"} {
		for name, u := range a.goblinReaders() {
			var got []string
			for _, l := range a.cutLinesOf(t, u, e) {
				if strings.HasPrefix(l, bard+" ") {
					got = append(got, l)
				}
			}
			if want := []string{bard + " usou Palavras de Interrupção: o ataque diminuiu."}; !slices.Equal(got, want) {
				t.Errorf("%s reads %q of %s, want %q", name, got, bard, want)
			}
		}
	}
	master := a.cutLinesOf(t, a.master, e)
	want := []string{ // the latest first
		"Brisa usou Palavras de Interrupção: d6 (3) subtraído do ataque de Goblin: 12 → 4 contra CA 12: errou.",
		"Reator usou Palavras de Interrupção: d6 (5) subtraído do ataque de Goblin: 12 → 4 contra CA 12: errou.",
	}
	if !slices.Equal(master, want) {
		t.Errorf("the master reads %q, want %q", master, want)
	}
}

// A roll whose holding event is gone cannot name its group: the answer still goes through, and the cut line is not completed, so it
// keeps waiting (the cut's own event says only that the roll was asked).
func TestAnAnswerWhoseHoldingEventIsGoneStillGoesThrough(t *testing.T) {
	t.Parallel()
	a, e := bardTable(t)
	offer := heldWeaponCut(t, a, e, d20(1))
	tag, err := a.h.pool.Exec(t.Context(), `DELETE FROM session_events WHERE idempotency_key IN (SELECT idempotency_key FROM inspiration_holds WHERE id = $1)`, offer.GetHoldId())
	if err != nil || tag.RowsAffected() != 1 {
		t.Fatalf("delete the event that held the roll: rows %d, error %v", tag.RowsAffected(), err)
	}
	res, err := a.answerHeldRoll(t, a.caio, offer.GetHoldId(), luckyAnswer(15))
	if err != nil || res.GetAttack().GetRoll().GetD20().GetFaces()[0] != 15 {
		t.Fatalf("AnswerHeldRoll() = %v, %v; want Pipo's attack settled on the 15", res, err)
	}
	a.wantLines(t, e, a.bardTableReaders(), cutPendingMaster, cutPendingPlayers)
}

// A Scorching Ray of two rays at one lore bard, cut by him: the cast is held in his window, and the answer writes the roll in its own
// change. No ray's question can be asked (a replay asks only when its attack has one ray) and no one roll says what the cut did, so the
// line names the die and what it came off and no number or outcome: it never waits for a roll that cannot complete it, and never says
// "errou" of rays that may have hit.
func TestACutLineOfASpellAttackOfSeveralRaysDoesNotWaitForever(t *testing.T) {
	t.Parallel()
	var bard *charactersv1.Character
	a := newArmedWith(t, func(a *armed) {
		a.toren = a.caio.hero(t, a.campaignID, "Toren", "class:fighter", "race:human", 2, scores16(), []string{battleaxe}, nil)
		a.pens = a.ana.caster(t, a.campaignID, "Pensantus", "class:wizard", "race:human", 3,
			&rulesv1.AbilityScores{Strength: 10, Dexterity: 14, Constitution: 12, Intelligence: 16, Wisdom: 10, Charisma: 8}, nil, []string{fireBolt},
			[]string{scorchingRay}, []string{scorchingRay})
		a.bri = a.bia.hero(t, a.campaignID, "Brisa", "class:fighter", "race:human", 2, scores16(), []string{rapier}, nil)
		bard = loreBardEnemy(t, a)
	})
	e := a.start(t, plan{
		npcs: []*playv1.Participant{{CharacterId: bard.GetId()}}, npcRolls: []int{3},
		players: map[string]int32{"Toren": 18, "Pensantus": 10, "Brisa": 1}, reveal: []string{"Bardo"},
		at: map[string][2]int32{"Toren": {3, 3}, "Bardo": {4, 3}, "Pensantus": {10, 3}, "Brisa": {4, 9}},
	})
	e = a.passTo(t, e, "Pensantus")
	held, err := a.castKey(t, a.ana, e, "Pensantus", scorchingRay, slotOfLevel(2), rayTargets(t, a, map[string]int32{"Bardo": 2}, "Bardo"),
		withRays(rays([]int32{15}, []int32{15})...), newKey())
	if err != nil || held.GetCast() != nil || held.GetHeldRollOffer() != nil {
		t.Fatalf("CastSpell() = %v, %v; want it held in the bard's window", held, err)
	}
	w := a.windowOf(t, a.master, playv1.ReactionKind_REACTION_KIND_CUTTING_WORDS)
	if w == nil {
		t.Fatalf("the master's windows = %v, want the bard's Cutting Words", a.windows(t, a.master))
	}
	a.mustAnswer(t, a.master, e, w.GetId(), cutWith(5))
	a.wantLines(t, e, a.spellReaders(),
		"Bardo usou Palavras de Interrupção: d6 (5) subtraído do ataque de Pensantus.",
		"Bardo usou Palavras de Interrupção: o ataque de Pensantus diminuiu.")
}

// An event that #949 stored marks its line with "asked" where #1061 reads "waiting": it is read as waiting, pending until the roll of
// its group is written and completed by it after, for the master and for a player who hears the roll. Its reduction is the whole group's
// (one bard answered), which is what the line takes off.
func TestACutLineStoredAsAskedByTheFirstVersionIsStillWaiting(t *testing.T) {
	t.Parallel()
	stored := fmt.Sprintf(`{"round":1,"combatant_id":"c-bard","target_id":"c-roller","reaction":{"kind":%q,"used":true,"roll":"attack","die":5,"die_sides":6,"asked":true,"group":"g","reduction":5}}`, reaction.CuttingWords)
	cut := func(t *testing.T, withRoll bool) *logEntry {
		t.Helper()
		events := []playdb.ListEncounterEventsRow{{ID: "cut", Seq: 1, Kind: eventReactionAnswered, Payload: []byte(stored)}}
		if withRoll {
			events = append(events, playdb.ListEncounterEventsRow{ID: "roll", Seq: 2, Kind: eventAttackRolled,
				Payload: []byte(`{"round":1,"combatant_id":"c-roller","target_id":"c-target","cut_group":"g","outcome":"miss","total":7,"target_ac":12}`)})
		}
		for _, en := range buildLog(events) {
			if en.kind == playv1.CombatLogKind_COMBAT_LOG_KIND_REACTION_WINDOW {
				return en
			}
		}
		t.Fatal("the log has no reaction line")
		return nil
	}
	byID := map[string]playdb.Combatant{"c-bard": {ID: "c-bard", Label: "Bardo"}, "c-roller": {ID: "c-roller", Label: "Goblin"}}
	line := func(en *logEntry, v combatViewer) string {
		s, _ := en.reactionLine(t.Context(), v, byID, nil)
		return s
	}
	master, player := combatViewer{userID: "m", master: true}, combatViewer{userID: "p"}
	pending := cut(t, false)
	if got, want := line(pending, master), "Bardo usou Palavras de Interrupção: d6 (5) subtraído do ataque de Goblin (aguardando a rolagem)."; got != want {
		t.Errorf("the master reads %q while the roll is not written, want %q", got, want)
	}
	if got, want := line(pending, player), "Bardo usou Palavras de Interrupção: o ataque de Goblin diminuiu."; got != want {
		t.Errorf("a player reads %q while the roll is not written, want %q", got, want)
	}
	done := cut(t, true)
	if got, want := line(done, master), "Bardo usou Palavras de Interrupção: d6 (5) subtraído do ataque de Goblin: 12 → 7 contra CA 12: errou."; got != want {
		t.Errorf("the master reads %q once the roll is written, want %q", got, want)
	}
	if got, want := line(done, player), "Bardo usou Palavras de Interrupção: o ataque de Goblin errou."; got != want {
		t.Errorf("a player reads %q once the roll is written, want %q", got, want)
	}
	if out, err := json.Marshal(done.ev.Reaction); err != nil || strings.Contains(string(out), `"asked"`) {
		t.Errorf("the event is written as %s (%v), want no \"asked\" key: only #949's events carry it", out, err)
	}
}
