package leaktest

import (
	"regexp"
	"slices"
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
	return newCutTableAt(t, false)
}

// newCutTableAt is newCutTable with the bard and the skeleton on the east side of the wall (east): Caio, in the west, does not see them,
// and the skeleton attacks the bard herself.
func newCutTableAt(t *testing.T, east bool) *cutTable {
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
	target := labelToren
	if east {
		target = labelPens
		for label, row := range map[string]int32{"Esqueleto": 5, labelPens: 6} {
			must(rt.master.combat.MoveCombatant(t.Context(), rq(&playv1.MoveCombatantRequest{
				CampaignId: rt.campaign, EncounterId: rt.encounter.GetId(), CombatantId: rt.ids[label], IdempotencyKey: newKey(), Col: 14, Row: row, Forced: true,
			})))
		}
	}
	rt.toTurn("Esqueleto")
	rt.giveDie("Esqueleto")
	ct.name = rt.combatantOf(rt.master, "Esqueleto").GetLabel()
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
	held := rt.mustAttack(rt.master, "Esqueleto", key, target, []int32{8}, nil)
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
func (rt *rollsTable) readsOfLog(p *person) []reply {
	rt.encounter = rt.encounterOf(rt.master)
	return rt.reads(p)
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
func (rt *rollsTable) checkNothingOf(t *testing.T, when string, replies map[*person][]reply) {
	t.Helper()
	for p, rs := range replies {
		for _, r := range rs {
			for _, f := range rt.inspect(p, r, nil) {
				t.Errorf("%s %s: %s", when, p.name, f)
			}
		}
	}
}

// the needle is registered for the master alone, and the master's own reads must hold it.
func (rt *rollsTable) needle(t *testing.T, kind, needle string, master []reply) {
	t.Helper()
	rt.secrets.add(&canary{needle: needle, kind: kind, readers: names([]*person{rt.master})})
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

// Two lore bards cut one held roll of the skeleton, each in a change of its own: the first answer is written before the roll is, and the
// roll completes the line of each bard, with or without a question for the master's die. The master reads both completed lines (the
// positive control), and each player reads the neutral line of each bard, completed in words only for a player who hears the roller.
//
// The bards are Ana's Pensantus and Caio's Toren, who the skeleton attacks; in the fogged table (cutline_rollers_test.go) they are
// Pensantus and Brisa, both Ana's, east of the wall.

// twoCutTable is a table with two bards that cut the skeleton's attack with a die of 5 and one of 3.
type twoCutTable struct {
	*rollsTable
	// bards are the labels of the two bards, the one that answered first first.
	bards []string
	// name is the skeleton's name as the players read it, and asks whether its roll asks the master about its die.
	name  string
	asks  bool
	offer *playv1.HeldRollOffer
}

// loreBardSheet is the sheet of a lore bard of the level 3.
func loreBardSheet() *charactersv1.CharacterSheet {
	return &charactersv1.CharacterSheet{Content: &charactersv1.CharacterSheet_Full{Full: &charactersv1.FullSheet{
		BaseScores: &rulesv1.AbilityScores{Strength: 8, Dexterity: 14, Constitution: 14, Intelligence: 10, Wisdom: 10, Charisma: 16}, RaceKey: "race:human",
		Classes:    []*charactersv1.ClassLevel{{ClassKey: "class:bard", Level: 3, Subclass: &charactersv1.ClassLevel_SubclassKey{SubclassKey: "subclass:lore"}}},
		WeaponKeys: []string{dagger},
	}}}
}

// newTwoCutTable builds the table of two bards and the skeleton's attack they cut. east puts the skeleton and the bards on the east side
// of the wall, where Caio does not see them: the second bard is then a character of Ana's (Brisa), and the skeleton attacks Pensantus.
// asks gives the skeleton a Bardic Inspiration die, so that its roll asks the master after the second cut; without it the second answer
// writes the roll.
func newTwoCutTable(t *testing.T, east, asks bool) *twoCutTable {
	t.Helper()
	second := labelToren
	rt := newRollsTableWith(t, func(rt *rollsTable) {
		w := rt.world
		for _, c := range []struct {
			owner *person
			char  *charactersv1.Character
		}{{w.ana, w.pens}, {w.caio, w.toren}} {
			must(c.owner.characters.UpdateCharacter(t.Context(), rq(&charactersv1.UpdateCharacterRequest{
				CampaignId: w.campaign, CharacterId: c.char.GetId(), Revision: c.char.GetRevision(), Name: c.char.GetName(), Sheet: loreBardSheet(),
			})))
		}
	})
	tt := &twoCutTable{rollsTable: rt, asks: asks}
	defer func() {
		if r := recover(); r != nil {
			t.Fatalf("the table: %v", r)
		}
	}()
	ctx := t.Context()
	at := map[string][2]int32{labelPens: {3, 6}}
	target := labelToren
	if east {
		second, target = "Brisa", labelPens
		brisa := must(rt.ana.characters.CreateCharacter(ctx, rq(&charactersv1.CreateCharacterRequest{
			CampaignId: rt.campaign, Kind: charactersv1.CharacterKind_CHARACTER_KIND_PLAYER, Name: second, Sheet: loreBardSheet(),
		}))).GetCharacter()
		added := must(rt.master.combat.AddCombatants(ctx, rq(&playv1.AddCombatantsRequest{
			CampaignId: rt.campaign, EncounterId: rt.encounter.GetId(), IdempotencyKey: newKey(), Participants: []*playv1.Participant{{CharacterId: brisa.GetId()}},
		}))).GetEncounter()
		for _, c := range added.GetCombatants() {
			if c.GetCharacterId() == brisa.GetId() {
				rt.ids[second] = c.GetId()
			}
		}
		if rt.ids[second] == "" {
			t.Fatal("Brisa is not in the combat")
		}
		rt.encounter = added
		at = map[string][2]int32{"Esqueleto": {14, 5}, labelPens: {14, 6}, second: {14, 7}}
	} else {
		at[labelToren] = [2]int32{3, 5}
	}
	for label, sq := range at {
		must(rt.master.combat.MoveCombatant(ctx, rq(&playv1.MoveCombatantRequest{
			CampaignId: rt.campaign, EncounterId: rt.encounter.GetId(), CombatantId: rt.ids[label], IdempotencyKey: newKey(), Col: sq[0], Row: sq[1], Forced: true,
		})))
	}
	rt.toTurn("Esqueleto")
	if asks {
		rt.giveDie("Esqueleto")
	}
	tt.name = rt.combatantOf(rt.master, "Esqueleto").GetLabel()
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
	if !rt.mustAttack(rt.master, "Esqueleto", key, target, []int32{8}, nil).GetRoll().GetHeldForReaction() {
		t.Fatal("the skeleton's attack is not held for the bards' windows")
	}
	cutters := []*person{rt.ana, rt.caio}
	if east {
		cutters = []*person{rt.ana, rt.ana}
	}
	for i, p := range cutters {
		win := windowOfKind(p, rt.world, playv1.ReactionKind_REACTION_KIND_CUTTING_WORDS)
		if win == nil {
			t.Fatalf("bard %d has no Cutting Words window", i+1)
		}
		must(p.combat.AnswerReaction(ctx, rq(&playv1.AnswerReactionRequest{
			CampaignId: rt.campaign, EncounterId: rt.encounter.GetId(), WindowId: win.GetId(), Answer: playv1.ReactionChoice_REACTION_CHOICE_USE,
			Roll: &playv1.AnswerReactionRequest_Typed{Typed: int32(5 - 2*i)}, IdempotencyKey: newKey(),
		})))
	}
	tt.bards = []string{labelPens, second}
	if asks {
		offers := rt.listHeld(rt.master)
		if len(offers) != 1 || offers[0].GetDie() == nil {
			t.Fatalf("the master's questions after the cuts = %v, want the skeleton's die", offers)
		}
		tt.offer = offers[0]
	}
	return tt
}

// answer is the master's answer to the skeleton's die: kept.
func (tt *twoCutTable) answer() {
	must(tt.master.resources.AnswerHeldRoll(tt.t.Context(), rq(&playv1.AnswerHeldRollRequest{
		CampaignId: tt.campaign, HoldId: tt.offer.GetHoldId(), IdempotencyKey: newKey(),
		Answer: &playv1.AnswerHeldRollRequest_Die{Die: &playv1.DieAnswer{Use: false}},
	})))
}

// neutralLines are the lines a player reads of the two bards while the roll is not heard: "diminuiu", with the skeleton's name.
func (tt *twoCutTable) neutralLines() []string {
	var want []string
	for _, b := range tt.bards {
		want = append(want, b+" usou Palavras de Interrupção: o ataque de "+tt.name+" diminuiu.")
	}
	slices.Sort(want)
	return want
}

// wantReactionLines checks the reaction lines of a reader (in any order), and names the reader and the moment for a failure.
func wantReactionLines(t *testing.T, who, when string, rs []reply, want []string) {
	t.Helper()
	got := reactionTexts(logReplyOf(rs).body)
	slices.Sort(got)
	if !slices.Equal(got, want) {
		t.Errorf("%s reads %q %s, want %q", who, got, when, want)
	}
}

// completed reads the master's two completed lines of the bards: the positive control, and the words the players read of the outcome.
func (tt *twoCutTable) completed(t *testing.T) (words []string) {
	t.Helper()
	done := tt.readsOfLog(tt.master)
	fragments := completedCut.FindAllString(string(logReplyOf(done).body), -1)
	if len(fragments) != 2 || fragments[0] != fragments[1] {
		t.Fatalf("the master's log holds the completed fragments %q, want the same one for both bards: %s", fragments, logReplyOf(done).body)
	}
	tt.needle(t, "two-cuts-completed-master-line", strings.TrimSuffix(fragments[0], "."), done)
	verb := "errou"
	if strings.HasSuffix(fragments[0], "acertou.") {
		verb = "ainda acertou"
	}
	for _, b := range tt.bards {
		words = append(words, b+" usou Palavras de Interrupção: o ataque de "+tt.name+" "+verb+".")
	}
	slices.Sort(words)
	return words
}

// Two bards cut one roll: while its question waits (when it asks) both players read the neutral line of each bard and the master the
// pending ones; once the roll is written the master reads both completed, and both players, who hear the skeleton, read the outcome in
// words for both. No read of a player holds the numbers of the master's lines.
func TestTwoCutLinesOfOneRollReachPlayersWithNoNumbers(t *testing.T) {
	for _, asks := range []bool{true, false} {
		t.Run(map[bool]string{true: "asks", false: "asks nothing"}[asks], func(t *testing.T) {
			tt := newTwoCutTable(t, false, asks)
			players := []*person{tt.ana, tt.caio}
			if asks {
				pending := map[*person][]reply{}
				for _, p := range players {
					pending[p] = tt.readsOfLog(p)
					wantReactionLines(t, p.name, "while the question waits", pending[p], tt.neutralLines())
				}
				master := tt.readsOfLog(tt.master)
				tt.needle(t, "two-cuts-pending-master-line", cutPendingNeedle, master)
				if n := strings.Count(string(logReplyOf(master).body), cutPendingNeedle); n != 2 {
					t.Errorf("the master reads %d pending lines, want one for each bard", n)
				}
				tt.checkNothingOf(t, "while the question waits", pending)
				tt.answer()
				defer tt.checkNothingOf(t, "while the question waited, against the numbers the answer wrote", pending)
			}
			want := tt.completed(t)
			after := map[*person][]reply{}
			for _, p := range players {
				after[p] = tt.readsOfLog(p)
				wantReactionLines(t, p.name, "after the roll is written", after[p], want)
			}
			tt.checkNothingOf(t, "after the roll is written", after)
		})
	}
}
