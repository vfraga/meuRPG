package leaktest

import (
	"strings"
	"testing"

	charactersv1 "github.com/PuraFome/meuRPG/backend/gen/meurpg/characters/v1"
	playv1 "github.com/PuraFome/meuRPG/backend/gen/meurpg/play/v1"
	rulesv1 "github.com/PuraFome/meuRPG/backend/gen/meurpg/rules/v1"
)

// The same rows as cutline_test.go for the rolls the builder shares: a roller who is a player's character (Ana's halfling wizard,
// Pensantus), asked about Lucky or a Bardic Inspiration die, with a weapon attack or a spell attack, cut by an enemy lore bard of the
// master's; and a roller a player does not see through the fog. The needles and the positive controls are the same.

const labelEnemyBard = "Bardo"

// playerCut is the table with an enemy bard and the roll of Pensantus that it cut: her question waits.
type playerCut struct {
	*rollsTable
	offer *playv1.HeldRollOffer
	// bard is the enemy bard's label as the players read it.
	bard string
}

// newPlayerCut builds the table and puts Pensantus's roll at its question. ask is "die" (a weapon attack, a Bardic Inspiration die),
// "lucky" (a weapon attack, a halfling's 1) or "spell" (Acid Arrow, a halfling's 1).
func newPlayerCut(t *testing.T, ask string) *playerCut {
	t.Helper()
	var enemy *charactersv1.Character
	rt := newRollsTableWith(t, func(rt *rollsTable) {
		w := rt.world
		sheet := &charactersv1.CharacterSheet{Content: &charactersv1.CharacterSheet_Full{Full: &charactersv1.FullSheet{
			BaseScores: &rulesv1.AbilityScores{Strength: 10, Dexterity: 14, Constitution: 12, Intelligence: 16, Wisdom: 10, Charisma: 8}, RaceKey: "race:halfling",
			Classes:    []*charactersv1.ClassLevel{{ClassKey: "class:wizard", Level: 3}},
			WeaponKeys: []string{dagger}, KnownSpellKeys: []string{"spell:acid-arrow"}, PreparedSpellKeys: []string{"spell:acid-arrow"},
		}}}
		must(w.ana.characters.UpdateCharacter(t.Context(), rq(&charactersv1.UpdateCharacterRequest{
			CampaignId: w.campaign, CharacterId: w.pens.GetId(), Revision: w.pens.GetRevision(), Name: w.pens.GetName(), Sheet: sheet,
		})))
		enemy = must(w.master.characters.CreateCharacter(t.Context(), rq(&charactersv1.CreateCharacterRequest{
			CampaignId: w.campaign, Kind: charactersv1.CharacterKind_CHARACTER_KIND_ENEMY, Name: labelEnemyBard,
			Sheet: &charactersv1.CharacterSheet{Content: &charactersv1.CharacterSheet_Full{Full: &charactersv1.FullSheet{
				BaseScores: &rulesv1.AbilityScores{Strength: 10, Dexterity: 14, Constitution: 12, Intelligence: 10, Wisdom: 16, Charisma: 16}, RaceKey: "race:human",
				Classes:    []*charactersv1.ClassLevel{{ClassKey: "class:bard", Level: 3, Subclass: &charactersv1.ClassLevel_SubclassKey{SubclassKey: "subclass:lore"}}},
				WeaponKeys: []string{dagger},
			}}},
		}))).GetCharacter()
	})
	pc := &playerCut{rollsTable: rt, bard: labelEnemyBard}
	defer func() {
		if r := recover(); r != nil {
			t.Fatalf("the table: %v", r)
		}
	}()
	ctx := t.Context()
	added := must(rt.master.combat.AddCombatants(ctx, rq(&playv1.AddCombatantsRequest{
		CampaignId: rt.campaign, EncounterId: rt.encounter.GetId(), IdempotencyKey: newKey(), Participants: []*playv1.Participant{{CharacterId: enemy.GetId()}},
	}))).GetEncounter()
	for _, c := range added.GetCombatants() {
		if c.GetCharacterId() == enemy.GetId() {
			rt.ids[labelEnemyBard] = c.GetId()
		}
	}
	if rt.ids[labelEnemyBard] == "" {
		t.Fatal("the enemy bard is not in the combat")
	}
	rt.encounter = added
	must(rt.master.combat.SetCombatantHidden(ctx, rq(&playv1.SetCombatantHiddenRequest{
		CampaignId: rt.campaign, EncounterId: rt.encounter.GetId(), CombatantId: rt.ids[labelEnemyBard], IdempotencyKey: newKey(), Hidden: false,
	})))
	for label, sq := range map[string][2]int32{labelEnemyBard: {2, 7}, labelPens: {3, 6}} {
		must(rt.master.combat.MoveCombatant(ctx, rq(&playv1.MoveCombatantRequest{
			CampaignId: rt.campaign, EncounterId: rt.encounter.GetId(), CombatantId: rt.ids[label], IdempotencyKey: newKey(), Col: sq[0], Row: sq[1], Forced: true,
		})))
	}
	rt.toTurn(labelPens)
	if ask == "die" {
		rt.giveDie(labelPens)
	}
	var held bool
	if ask == "spell" {
		res := must(rt.ana.combat.CastSpell(ctx, rq(&playv1.CastSpellRequest{
			CampaignId: rt.campaign, EncounterId: rt.encounter.GetId(), CasterId: rt.ids[labelPens], SpellKey: "spell:acid-arrow", Slot: &playv1.SpellSlot{Level: 2},
			Targets: []*playv1.SpellTarget{{CombatantId: rt.ids[labelEnemyBard]}}, IdempotencyKey: newKey(), D20Faces: []int32{1, 1}, // a hostile creature is within 5 feet of the caster: the spell attack has disadvantage, two dice
		})))
		held = res.GetCast() == nil && res.GetHeldRollOffer() == nil
	} else {
		face := int32(1)
		if ask == "die" {
			face = 10
		}
		held = rt.mustAttack(rt.ana, labelPens, dagger, labelEnemyBard, []int32{face}, nil).GetRoll().GetHeldForReaction()
	}
	if !held {
		t.Fatal("the roll is not held for the bard's window")
	}
	win := windowOfKind(rt.master, rt.world, playv1.ReactionKind_REACTION_KIND_CUTTING_WORDS)
	if win == nil {
		t.Fatal("the enemy bard has no Cutting Words window")
	}
	must(rt.master.combat.AnswerReaction(ctx, rq(&playv1.AnswerReactionRequest{
		CampaignId: rt.campaign, EncounterId: rt.encounter.GetId(), WindowId: win.GetId(), Answer: playv1.ReactionChoice_REACTION_CHOICE_USE,
		Roll: &playv1.AnswerReactionRequest_Typed{Typed: 5}, IdempotencyKey: newKey(),
	})))
	offers := rt.listHeld(rt.ana)
	if len(offers) != 1 || (ask == "die") != (offers[0].GetDie() != nil) || (ask != "die") != (offers[0].GetLucky() != nil) {
		t.Fatalf("Pensantus's questions after the cut = %v, want the %s question)", offers, ask)
	}
	pc.offer = offers[0]
	return pc
}

// answer is Pensantus's answer to her question: Lucky rolled again to a 15, or the die kept.
func (pc *playerCut) answer() {
	req := &playv1.AnswerHeldRollRequest{CampaignId: pc.campaign, HoldId: pc.offer.GetHoldId(), IdempotencyKey: newKey()}
	if pc.offer.GetLucky() != nil {
		req.Answer = luckyAnswerOf(15)
	} else {
		req.Answer = &playv1.AnswerHeldRollRequest_Die{Die: &playv1.DieAnswer{Use: false}}
	}
	must(pc.ana.resources.AnswerHeldRoll(pc.t.Context(), rq(req)))
}

// The pending line of a cut that asked a player's roller reaches the players with no numbers, for every ask; the line that the answer
// completes is the master's alone but for the outcome in a word.
func TestAPendingCutLineOfAPlayersRollReachesPlayersWithNoNumbers(t *testing.T) {
	for _, ask := range []string{"die", "lucky", "spell"} {
		t.Run(ask, func(t *testing.T) {
			pc := newPlayerCut(t, ask)
			players := []*person{pc.ana, pc.caio}
			pending := map[*person][]reply{}
			for _, p := range players {
				pending[p] = pc.readsOfLog(p)
			}
			pc.needle(t, "cut-pending-master-line", cutPendingNeedle, pc.readsOfLog(pc.master))
			wantPending := pc.bard + " usou Palavras de Interrupção: o ataque de " + labelPens + " diminuiu."
			for _, p := range players {
				if lines := reactionTexts(logReplyOf(pending[p]).body); len(lines) != 1 || lines[0] != wantPending {
					t.Errorf("%s reads %q while the question waits, want [%q]", p.name, lines, wantPending)
				}
			}
			pc.checkNothingOf(t, "while the question waits", pending)

			pc.answer()
			done := pc.readsOfLog(pc.master)
			fragment := completedCut.FindString(string(logReplyOf(done).body))
			if fragment == "" {
				t.Fatalf("the master's log holds no completed line: %s", logReplyOf(done).body)
			}
			pc.needle(t, "cut-completed-master-line", strings.TrimSuffix(fragment, "."), done)
			verb := "errou"
			if strings.HasSuffix(fragment, "acertou.") {
				verb = "ainda acertou"
			}
			wantAfter := pc.bard + " usou Palavras de Interrupção: o ataque de " + labelPens + " " + verb + "."
			after := map[*person][]reply{}
			for _, p := range players {
				after[p] = pc.readsOfLog(p)
				if lines := reactionTexts(logReplyOf(after[p]).body); len(lines) != 1 || lines[0] != wantAfter {
					t.Errorf("%s reads %q after the answer, want [%q]", p.name, lines, wantAfter)
				}
			}
			pc.checkNothingOf(t, "after the answer", after)
			pc.checkNothingOf(t, "while the question waited, against the numbers the answer wrote", pending)
		})
	}
}

// A roller a player does not see through the fog is a roller they do not hear: Caio, in the west, never reads the completed line of
// the skeleton's roll, before or after the answer, while the bard's player (Ana) reads it completed and the master too.
func TestAFoggedRollersCutLineIsNotCompletedForWhoDidNotSeeIt(t *testing.T) {
	ct := newCutTableAt(t, true)
	pendingCaio := ct.readsOfLog(ct.caio)
	ct.needle(t, "cut-pending-master-line", cutPendingNeedle, ct.readsOfLog(ct.master))
	before := reactionTexts(logReplyOf(pendingCaio).body)

	ct.answer()
	done := ct.readsOfLog(ct.master)
	fragment := completedCut.FindString(string(logReplyOf(done).body))
	if fragment == "" {
		t.Fatalf("the master's log holds no completed line: %s", logReplyOf(done).body)
	}
	ct.needle(t, "fogged-roller-completed-master-line", strings.TrimSuffix(fragment, "."), done)
	verb := "errou"
	if strings.HasSuffix(fragment, "acertou.") {
		verb = "ainda acertou"
	}
	ana := ct.readsOfLog(ct.ana)
	want := cutBard + " usou Palavras de Interrupção: o ataque de " + ct.name + " " + verb + "."
	if lines := reactionTexts(logReplyOf(ana).body); len(lines) != 1 || lines[0] != want {
		t.Errorf("Ana reads %q after the answer, want [%q] (the positive control)", lines, want)
	}
	caio := ct.readsOfLog(ct.caio)
	if lines := reactionTexts(logReplyOf(caio).body); strings.Join(lines, "\n") != strings.Join(before, "\n") {
		t.Errorf("Caio reads %q after the answer and read %q before: a roller he does not see must change nothing for him", lines, before)
	}
	ct.checkNothingOf(t, "after the answer", map[*person][]reply{ct.caio: caio})
	ct.checkNothingOf(t, "while the question waited", map[*person][]reply{ct.caio: pendingCaio})
}
