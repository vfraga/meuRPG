package leaktest

import (
	"regexp"
	"strings"
	"testing"

	charactersv1 "github.com/PuraFome/meuRPG/backend/gen/meurpg/characters/v1"
	playv1 "github.com/PuraFome/meuRPG/backend/gen/meurpg/play/v1"
	"github.com/PuraFome/meuRPG/backend/gen/meurpg/play/v1/playv1connect"
	rulesv1 "github.com/PuraFome/meuRPG/backend/gen/meurpg/rules/v1"
)

// The line of a Cutting Words that cut a roll whose replay asked its roller (a held roll: the roller holds a Bardic Inspiration die).
// While the question waits the master reads the die and that the roll is awaited, and the players read the neutral "diminuiu"; no read
// of a player holds the total, the armor class or the outcome the master reads once the roll is written. A roller the master hides before
// the answer is a roller the players never hear: their line stays neutral after it, and no line of the roll reaches them.
//
// The table is the attack rolls' one, with a lore bard (Reator, Ana's) next to the skeleton, who holds the die and attacks Toren (Caio's).
// The needle is text because the line has no numeric field (as the Parry rows' words).
// The needles are the pieces of the master's own lines that carry the numbers (they hold letters outside a to f and an arrow, and
// match nothing else), and the master's read of each is the positive control.

const (
	cutPendingNeedle = "(aguardando a rolagem)"
	cutBard          = labelPens
)

// completedCut is the numbers of the master's completed line: "7 → 7 contra CA 12: errou".
var completedCut = regexp.MustCompile(`\d+ → \d+ contra CA \d+: \S+?\.`)

// cutTable is the table with the bard, the skeleton's die and the attack the bard cut: the skeleton's roll waits for the master.
type cutTable struct {
	*rollsTable
	offer *playv1.HeldRollOffer
	// name is the skeleton's name as the players read it.
	name string
}

// newCutTable builds the table and puts the skeleton's attack at the question of its die.
func newCutTable(t *testing.T) *cutTable {
	t.Helper()
	rt := newRollsTableWith(t, func(rt *rollsTable) {
		w := rt.world
		// Ana's barbarian is a lore bard for this table.
		sheet := &charactersv1.CharacterSheet{Content: &charactersv1.CharacterSheet_Full{Full: &charactersv1.FullSheet{
			BaseScores: &rulesv1.AbilityScores{Strength: 8, Dexterity: 14, Constitution: 14, Intelligence: 10, Wisdom: 10, Charisma: 16}, RaceKey: "race:human",
			Classes:    []*charactersv1.ClassLevel{{ClassKey: "class:bard", Level: 3, Subclass: &charactersv1.ClassLevel_SubclassKey{SubclassKey: "subclass:lore"}}},
			WeaponKeys: []string{dagger},
		}}}
		must(w.ana.characters.UpdateCharacter(t.Context(), rq(&charactersv1.UpdateCharacterRequest{
			CampaignId: w.campaign, CharacterId: w.pens.GetId(), Revision: w.pens.GetRevision(), Name: w.pens.GetName(), Sheet: sheet,
		})))
	})
	ct := &cutTable{rollsTable: rt}
	defer func() {
		if r := recover(); r != nil {
			t.Fatalf("the table: %v", r)
		}
	}()
	// The bard stands next to the skeleton, in plain sight of it.
	must(rt.master.combat.MoveCombatant(t.Context(), rq(&playv1.MoveCombatantRequest{
		CampaignId: rt.campaign, EncounterId: rt.encounter.GetId(), CombatantId: rt.ids[labelPens], IdempotencyKey: newKey(), Col: 3, Row: 6, Forced: true,
	})))
	rt.toTurn("Esqueleto")
	rt.giveDie("Esqueleto")
	ct.name = rt.combatantOf(rt.caio, "Esqueleto").GetLabel()
	key := ""
	for _, a := range rt.optionsOf(rt.master, "Esqueleto").GetOptions().GetAttacks() {
		if a.GetEnabled() && a.GetAttack().GetKey() != "" && a.GetAttack().GetSaveDc() == 0 {
			key = a.GetAttack().GetKey()
			break
		}
	}
	if key == "" {
		t.Fatal("the skeleton has no attack to make")
	}
	held := rt.mustAttack(rt.master, "Esqueleto", key, labelToren, []int32{8}, nil)
	if !held.GetRoll().GetHeldForReaction() {
		t.Fatalf("the skeleton's attack = %v, want it held for the bard's window", held.GetRoll())
	}
	win := windowOfKind(rt.ana, rt.world, playv1.ReactionKind_REACTION_KIND_CUTTING_WORDS)
	if win == nil {
		t.Fatal("the bard has no Cutting Words window")
	}
	must(rt.ana.combat.AnswerReaction(t.Context(), rq(&playv1.AnswerReactionRequest{
		CampaignId: rt.campaign, EncounterId: rt.encounter.GetId(), WindowId: win.GetId(), Answer: playv1.ReactionChoice_REACTION_CHOICE_USE,
		Roll: &playv1.AnswerReactionRequest_Typed{Typed: 5}, IdempotencyKey: newKey(),
	})))
	offers := rt.listHeld(rt.master)
	if len(offers) != 1 || offers[0].GetDie() == nil {
		t.Fatalf("the master's questions after the cut = %v, want the skeleton's die", offers)
	}
	ct.offer = offers[0]
	return ct
}

// readsOfLog are what a person reads of the combat and its log now.
func (ct *cutTable) readsOfLog(p *person) []reply {
	ct.encounter = ct.encounterOf(ct.master)
	return ct.reads(p)
}

var reactionText = regexp.MustCompile(`"reactionTextPt":"([^"]*)"`)

// reactionTexts are the reaction lines of a log's body.
func reactionTexts(body []byte) []string {
	var out []string
	for _, m := range reactionText.FindAllSubmatch(body, -1) {
		out = append(out, string(m[1]))
	}
	return out
}

// logReplyOf is the log among the replies of reads.
func logReplyOf(rs []reply) reply { return rs[1] }

// checkNothingOf says no read of the players holds a needle they may not read, and no field the master keeps.
func (ct *cutTable) checkNothingOf(t *testing.T, when string, replies map[*person][]reply) {
	t.Helper()
	for p, rs := range replies {
		for _, r := range rs {
			for _, f := range ct.inspect(p, r, nil) {
				t.Errorf("%s %s: %s", when, p.name, f)
			}
		}
	}
}

// the needle is registered for the master alone, and the master's own reads must hold it.
func (ct *cutTable) needle(t *testing.T, kind, needle string, master []reply) {
	t.Helper()
	ct.secrets.add(&canary{needle: needle, kind: kind, readers: names([]*person{ct.master})})
	if !contains(logReplyOf(master).body, needle) {
		t.Fatalf("the master's log does not hold %q: the needle is unreachable", needle)
	}
}

// answer is the master's answer to the skeleton's die: kept.
func (ct *cutTable) answer() {
	must(ct.master.resources.AnswerHeldRoll(ct.t.Context(), rq(&playv1.AnswerHeldRollRequest{
		CampaignId: ct.campaign, HoldId: ct.offer.GetHoldId(), IdempotencyKey: newKey(),
		Answer: &playv1.AnswerHeldRollRequest_Die{Die: &playv1.DieAnswer{Use: false}},
	})))
}

// The pending line of a cut reaches the players with no total, armor class or outcome, and the line the answer completes is the master's
// alone but for the outcome the players read in words.
func TestAPendingCutLineReachesPlayersWithNoNumbers(t *testing.T) {
	ct := newCutTable(t)
	players := []*person{ct.ana, ct.caio}
	pending := map[*person][]reply{}
	for _, p := range players {
		pending[p] = ct.readsOfLog(p)
	}
	master := ct.readsOfLog(ct.master)
	ct.needle(t, "cut-pending-master-line", cutPendingNeedle, master)
	for _, p := range players {
		ct.neutral(t, p, "while the question waits", pending[p], false)
	}
	ct.checkNothingOf(t, "while the question waits", pending)

	ct.answer()
	done := ct.readsOfLog(ct.master)
	fragment := completedCut.FindString(string(logReplyOf(done).body))
	if fragment == "" {
		t.Fatalf("the master's log holds no completed line: %s", logReplyOf(done).body)
	}
	ct.needle(t, "cut-completed-master-line", strings.TrimSuffix(fragment, "."), done)
	verb := "errou" // what the players read of the outcome the master read
	if strings.HasSuffix(fragment, "acertou.") {
		verb = "ainda acertou"
	}
	wantAfter := cutBard + " usou Palavras de Interrupção: o ataque de " + ct.name + " " + verb + "."
	after := map[*person][]reply{}
	for _, p := range players {
		after[p] = ct.readsOfLog(p)
		if lines := reactionTexts(logReplyOf(after[p]).body); len(lines) != 1 || lines[0] != wantAfter {
			t.Errorf("%s reads %q after the answer, want [%q]", p.name, lines, wantAfter)
		}
	}
	ct.checkNothingOf(t, "after the answer", after)
	ct.checkNothingOf(t, "while the question waited, against the numbers the answer wrote", pending)
}

// neutral checks the reaction line of a player's log is the neutral one, with the skeleton's name when named.
func (ct *cutTable) neutral(t *testing.T, p *person, when string, rs []reply, hidden bool) {
	t.Helper()
	want := cutBard + " usou Palavras de Interrupção: o ataque de " + ct.name + " diminuiu."
	if hidden {
		want = cutBard + " usou Palavras de Interrupção: o ataque diminuiu."
	}
	if lines := reactionTexts(logReplyOf(rs).body); len(lines) != 1 || lines[0] != want {
		t.Errorf("%s reads %q %s, want [%q]", p.name, lines, when, want)
	}
}

// A roller the master hides before the answer stays unheard: the players' line stays neutral, and nothing of the roll the master reads
// (its numbers, its outcome, its line of the attack) reaches them, before or after.
func TestAHiddenRollersCutLineStaysNeutralForThePlayers(t *testing.T) {
	ct := newCutTable(t)
	players := []*person{ct.ana, ct.caio}
	master := ct.readsOfLog(ct.master)
	ct.needle(t, "cut-pending-master-line", cutPendingNeedle, master)
	held := map[*person]int64{} // the number each player holds while the question waits
	for _, p := range players {
		held[p] = ct.logOf(p).GetLastSeq()
	}

	must(ct.master.combat.SetCombatantHidden(t.Context(), rq(&playv1.SetCombatantHiddenRequest{
		CampaignId: ct.campaign, EncounterId: ct.encounter.GetId(), CombatantId: ct.ids["Esqueleto"], IdempotencyKey: newKey(), Hidden: true,
	})))
	ct.answer()
	done := ct.readsOfLog(ct.master)
	fragment := completedCut.FindString(string(logReplyOf(done).body))
	if fragment == "" {
		t.Fatalf("the master's log holds no completed line: %s", logReplyOf(done).body)
	}
	ct.needle(t, "hidden-roller-completed-master-line", strings.TrimSuffix(fragment, "."), done)
	if !contains(logReplyOf(done).body, "COMBAT_LOG_KIND_ATTACK") {
		t.Fatal("the master's log holds no attack line of the hidden skeleton: the control is unreachable")
	}

	after := map[*person][]reply{}
	for _, p := range players {
		after[p] = ct.readsOfLog(p)
		ct.neutral(t, p, "after the hidden roller's answer", after[p], true)
		if body := logReplyOf(after[p]).body; contains(body, "COMBAT_LOG_KIND_ATTACK") {
			t.Errorf("%s reads the hidden skeleton's attack: %s", p.name, body)
		}
	}
	ct.checkNothingOf(t, "after the hidden roller's answer", after)
	// The same reads through the other doors a player has to the log.
	for _, p := range players {
		since := p.call(playv1connect.CombatServiceListCombatLogProcedure, &playv1.ListCombatLogRequest{CampaignId: ct.campaign, EncounterId: ct.encounter.GetId(), SinceSeq: held[p]})
		for _, f := range ct.inspect(p, since, nil) {
			t.Errorf("the delta of %s: %s", p.name, f)
		}
	}
}
