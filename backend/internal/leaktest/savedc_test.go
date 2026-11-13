package leaktest

import (
	"slices"
	"strconv"
	"strings"
	"testing"

	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/reflect/protoreflect"

	campaignsv1 "github.com/PuraFome/meuRPG/backend/gen/meurpg/campaigns/v1"
	charactersv1 "github.com/PuraFome/meuRPG/backend/gen/meurpg/characters/v1"
	playv1 "github.com/PuraFome/meuRPG/backend/gen/meurpg/play/v1"
	"github.com/PuraFome/meuRPG/backend/gen/meurpg/play/v1/playv1connect"
)

// The table rule "Os jogadores veem a CD dos testes de resistência" (RN-24, RN-10): the DC of a
// saving throw is the master's. With the rule on, the player who rolls the save reads it too, in
// what they are asked and in what they are told; no other player ever does. The DC is a number
// no other number of these answers matches (37 for an effect, the fixture's trap save for a
// trap), searched in the numeric fields named "dc", and the positive controls show the number in
// the master's answer, and in the roller's when the rule is on.

// saveDCEffect is the DC the Hold Person of these tests asks.
const saveDCEffect = 37

// showSaveDC turns the table's rule on or off, as the master.
func (w *world) showSaveDC(on bool) {
	w.t.Helper()
	ctx := w.t.Context()
	rules := must(w.master.campaigns.GetTableRules(ctx, rq(&campaignsv1.GetTableRulesRequest{CampaignId: w.campaign}))).GetRules()
	rules.ShowSaveDc = on
	must(w.master.campaigns.SetTableRules(ctx, rq(&campaignsv1.SetTableRulesRequest{CampaignId: w.campaign, Rules: rules})))
}

// holdsNumber says whether the answer holds the canary's number in one of its fields.
func holdsNumber(r reply, c *canary) bool {
	if r.msg == nil {
		return false
	}
	found := false
	walk(r.msg.ProtoReflect(), "", func(_ string, _ protoreflect.Message, fd protoreflect.FieldDescriptor, v protoreflect.Value) {
		if isNumber(fd) && c.matchesNumber(fd, asInt(fd, v)) {
			found = true
		}
	})
	return found
}

// canaryOf finds the registered number of a kind.
func (w *world) canaryOf(kind string) *canary {
	w.t.Helper()
	for _, c := range w.secrets.list {
		if c.kind == kind && c.numeric {
			return c
		}
	}
	w.t.Fatalf("no number of the kind %q is registered", kind)
	return nil
}

// A Hold Person the Mago casts on Toren asks his player the saving throw at the end of his turn,
// against a DC of 37. Rule off: the DC is in no answer of a player. Rule on: Caio, who rolls it,
// reads it in the question, the card and the answer; Ana, who does not, never does.
func TestTheDCOfAnEffectSaveReachesOnlyItsRollerWhenTheTableShowsIt(t *testing.T) {
	for _, on := range []bool{false, true} {
		t.Run("rule "+strconv.FormatBool(on), func(t *testing.T) {
			w := newWorld(t)
			mage := w.reactionFight()
			w.secrets.number("effect-save-dc", saveDCEffect, "dc")
			dc := w.canaryOf("effect-save-dc")
			if on {
				w.showSaveDC(true)
				w.secrets.allowNeedle(strconv.Itoa(saveDCEffect), w.caio)
			}
			toren := w.combatant(w.toren)
			visible := true
			must(w.master.lasting.AddLastingEffect(t.Context(), rq(&playv1.AddLastingEffectRequest{
				CampaignId: w.campaign, EncounterId: w.encounter.GetId(), IdempotencyKey: newKey(), CatalogKey: "spell:hold-person",
				TargetIds: []string{toren.GetId()}, CasterId: mage.GetId(), SaveDc: new(int32(saveDCEffect)), PlayerVisible: &visible,
				Duration: &playv1.EffectDurationChoice{Kind: playv1.EffectDurationKind_EFFECT_DURATION_KIND_ROUNDS, Rounds: 10, AnchorCombatantId: toren.GetId()},
			})))
			must(w.master.combat.EndTurn(t.Context(), rq(&playv1.EndTurnRequest{
				CampaignId: w.campaign, EncounterId: w.encounter.GetId(), IdempotencyKey: newKey(), ExpectedCombatantId: toren.GetId(), DiscardPendingDamage: true,
			})))
			enc := must(w.master.combat.GetEncounter(t.Context(), rq(&playv1.GetEncounterRequest{CampaignId: w.campaign}))).GetEncounter()
			var window string
			for _, rw := range enc.GetReactionWindows() {
				if rw.GetKind() == playv1.ReactionKind_REACTION_KIND_EFFECT_SAVE {
					window = rw.GetId()
				}
			}
			if window == "" {
				t.Fatal("Toren's save is not asked")
			}

			roll := w.caio.call(playv1connect.LastingEffectServiceRollEffectSaveProcedure, &playv1.RollEffectSaveRequest{
				CampaignId: w.campaign, EncounterId: enc.GetId(), WindowId: window, IdempotencyKey: newKey(), Roll: &playv1.RollEffectSaveRequest_D20Face{D20Face: 5},
			})
			if !roll.ok() {
				t.Fatalf("Caio's save: status %d %s", roll.status, roll.body)
			}
			// What Caio is told (the answer, the log line, the card) and what Ana reads.
			for _, p := range []*person{w.ana, w.caio} {
				replies := w.reads(p)
				if p == w.caio {
					replies = append(replies, roll)
				}
				for _, r := range replies {
					for _, f := range w.inspect(p, r, nil) {
						t.Errorf("%s: %s", p.name, f)
					}
				}
			}
			// The positive controls: the master reads the number, and Caio does when the rule is on.
			masterHolds := false
			for _, r := range w.reads(w.master) {
				masterHolds = masterHolds || holdsNumber(r, dc)
			}
			if !masterHolds {
				t.Error("the master's reads do not hold the DC: the needle is unreachable")
			}
			caio := append(w.reads(w.caio), roll)
			got := false
			for _, r := range caio {
				got = got || holdsNumber(r, dc)
			}
			if got != on {
				t.Errorf("Caio reads the DC = %v, want %v (the rule is %v)", got, on, on)
			}
		})
	}
}

// The question itself, while it waits: the window of the end-of-turn save is read by its roller,
// who reads the DC only with the rule on, and by nobody else.
func TestTheQuestionOfAnEffectSaveCarriesTheDCOnlyForItsRollerWhenTheTableShowsIt(t *testing.T) {
	for _, on := range []bool{false, true} {
		t.Run("rule "+strconv.FormatBool(on), func(t *testing.T) {
			w := newWorld(t)
			mage := w.reactionFight()
			w.secrets.number("effect-save-dc", saveDCEffect, "dc")
			dc := w.canaryOf("effect-save-dc")
			if on {
				w.showSaveDC(true)
				w.secrets.allowNeedle(strconv.Itoa(saveDCEffect), w.caio)
			}
			toren := w.combatant(w.toren)
			must(w.master.lasting.AddLastingEffect(t.Context(), rq(&playv1.AddLastingEffectRequest{
				CampaignId: w.campaign, EncounterId: w.encounter.GetId(), IdempotencyKey: newKey(), CatalogKey: "spell:hold-person",
				TargetIds: []string{toren.GetId()}, CasterId: mage.GetId(), SaveDc: new(int32(saveDCEffect)),
				Duration: &playv1.EffectDurationChoice{Kind: playv1.EffectDurationKind_EFFECT_DURATION_KIND_ROUNDS, Rounds: 10, AnchorCombatantId: toren.GetId()},
			})))
			must(w.master.combat.EndTurn(t.Context(), rq(&playv1.EndTurnRequest{
				CampaignId: w.campaign, EncounterId: w.encounter.GetId(), IdempotencyKey: newKey(), ExpectedCombatantId: toren.GetId(), DiscardPendingDamage: true,
			})))
			for _, p := range []*person{w.ana, w.caio} {
				for _, r := range w.reads(p) {
					for _, f := range w.inspect(p, r, nil) {
						t.Errorf("%s: %s", p.name, f)
					}
				}
			}
			if !holdsNumber(w.reads(w.master)[0], dc) {
				t.Error("the master's combat does not hold the DC of the question: the needle is unreachable")
			}
			if got := holdsNumber(w.reads(w.caio)[0], dc); got != on {
				t.Errorf("Caio's combat holds the DC = %v, want %v", got, on)
			}
			if holdsNumber(w.reads(w.ana)[0], dc) {
				t.Error("Ana's combat holds the DC of Toren's save")
			}
		})
	}
}

// A trap that fires on Toren asks his save against its DC. The master fires it by hand: the log
// carries the DC for the master, and for Caio, whose character rolls it, only with the rule on.
func TestTheDCOfATrapSaveReachesOnlyItsRollerWhenTheTableShowsIt(t *testing.T) {
	for _, on := range []bool{false, true} {
		t.Run("rule "+strconv.FormatBool(on), func(t *testing.T) {
			w := newWorld(t)
			dc := w.canaryOf("trap-save-dc")
			if on {
				w.showSaveDC(true)
				w.secrets.allowNeedle(strconv.Itoa(trapSave), w.caio)
			}
			fireByHand(w, w.combatant(w.toren).GetId())
			firedTrapIsPublic(w)
			for _, p := range []*person{w.ana, w.caio} {
				for _, f := range w.inspect(p, trapLog(w, p), nil) {
					t.Errorf("%s: %s", p.name, f)
				}
			}
			if !holdsNumber(trapLog(w, w.master), dc) {
				t.Error("the master's log does not hold the trap's DC: the needle is unreachable")
			}
			if got := holdsNumber(trapLog(w, w.caio), dc); got != on {
				t.Errorf("Caio's log holds the trap's DC = %v, want %v", got, on)
			}
			if holdsNumber(trapLog(w, w.ana), dc) {
				t.Error("Ana's log holds the DC of the save Toren rolls")
			}
		})
	}
}

// saveDCConcentration is the DC of the concentration save the damage asks of Toren: half of the 34 he takes.
// No other number of the "dc" fields in these answers is 17.
const saveDCConcentration = 17

// saveDCRebuke is the DC of a tiefling's Hellish Rebuke: 8, the proficiency bonus of the 9th level (4)
// and the Charisma modifier of a 22 (6).
const saveDCRebuke = 18

// banditsTurn puts the bandit in plain sight of the players (an attacker they see, as in the Hellish Rebuke
// test above) and passes the turns up to its own, with Toren (a wizard of the 8th level, so that he lives
// through the damage) next to it.
func (w *world) banditsTurn() (toren, bandit *playv1.Combatant) {
	ctx := w.t.Context()
	m := w.master
	must(m.characters.UpdateCharacter(ctx, rq(&charactersv1.UpdateCharacterRequest{
		CampaignId: w.campaign, CharacterId: w.toren.GetId(), Revision: w.toren.GetRevision(), Name: w.toren.GetName(), Sheet: w.fullSheet("class:wizard", "race:human", 8),
	})))
	toren, bandit = w.combatant(w.toren), w.combatant(w.bandit)
	must(m.combat.SetCombatantHidden(ctx, rq(&playv1.SetCombatantHiddenRequest{CampaignId: w.campaign, EncounterId: w.encounter.GetId(), CombatantId: bandit.GetId(), IdempotencyKey: newKey(), Hidden: false})))
	w.allow("bandit-name", w.ana, w.caio)
	w.secrets.allowNeedle(bandit.GetId(), w.ana, w.caio)
	w.placeForced(m, w.encounter.GetId(), placement{toren.GetId(), 12, 14}, placement{bandit.GetId(), 13, 14})
	w.passTurnsTo(bandit.GetId())
	return toren, bandit
}

// passTurnsTo ends the turns until the combatant's.
func (w *world) passTurnsTo(combatantID string) {
	ctx := w.t.Context()
	for range 40 {
		cur := must(w.master.combat.GetEncounter(ctx, rq(&playv1.GetEncounterRequest{CampaignId: w.campaign}))).GetEncounter()
		if cur.GetCurrentCombatantId() == combatantID {
			w.encounter = cur
			return
		}
		must(w.master.combat.EndTurn(ctx, rq(&playv1.EndTurnRequest{CampaignId: w.campaign, EncounterId: cur.GetId(), IdempotencyKey: newKey(), ExpectedCombatantId: cur.GetCurrentCombatantId(), ExpectedRound: cur.GetRound()})))
	}
	w.t.Fatal("the turn of the combatant never came")
}

// hurtToren has the bandit hit Toren, who concentrates on Teia, and the master apply 34 damage: the
// damage that asks his player the Constitution save.
func (w *world) hurtToren(toren, bandit *playv1.Combatant) {
	ctx := w.t.Context()
	m := w.master
	if _, err := w.pool.Exec(ctx, `UPDATE combatants SET concentration_spell = $2 WHERE id = $1`, toren.GetId(), "spell:web"); err != nil {
		w.t.Fatalf("make Toren concentrate: %v", err)
	}
	opts := must(m.combat.GetTurnOptions(ctx, rq(&playv1.GetTurnOptionsRequest{CampaignId: w.campaign, EncounterId: w.encounter.GetId(), CombatantId: bandit.GetId()})))
	key := ""
	for _, a := range opts.GetOptions().GetAttacks() {
		if a.GetEnabled() && a.GetAttack().GetKey() != "" && a.GetAttack().GetSaveDc() == 0 {
			key = a.GetAttack().GetKey()
			break
		}
	}
	if key == "" {
		w.t.Fatal("the bandit has no attack to make")
	}
	hit := must(m.combat.RollAttack(ctx, rq(&playv1.RollAttackRequest{
		CampaignId: w.campaign, EncounterId: w.encounter.GetId(), AttackerId: bandit.GetId(), AttackKey: key, TargetId: toren.GetId(), IdempotencyKey: newKey(),
		Roll: &playv1.RollAttackRequest_D20Face{D20Face: 19},
	})))
	if hit.GetPendingDamage().GetId() == "" {
		w.t.Fatalf("the bandit's attack = %v, want a hit", hit.GetRoll())
	}
	must(m.combat.RollDamage(ctx, rq(&playv1.RollDamageRequest{CampaignId: w.campaign, EncounterId: w.encounter.GetId(), PendingDamageId: hit.GetPendingDamage().GetId(), IdempotencyKey: newKey(), Roll: &playv1.RollDamageRequest_RollInApp{RollInApp: true}})))
	must(m.combat.ApplyPendingDamage(ctx, rq(&playv1.ApplyPendingDamageRequest{
		CampaignId: w.campaign, EncounterId: w.encounter.GetId(), PendingDamageId: hit.GetPendingDamage().GetId(), IdempotencyKey: newKey(), Amount: proto.Int32(2 * saveDCConcentration),
	})))
}

// A damage that threatens the concentration of Toren's Teia asks his player the Constitution save, against
// a DC of 17: in the question, the reminder on the damage, the answer and the log line. Rule off: the DC is
// in no answer of a player. Rule on: Caio, who rolls it, reads it; Ana, who does not, never does. The
// master reads it always (the positive control).
func TestTheDCOfAConcentrationSaveReachesOnlyItsRollerWhenTheTableShowsIt(t *testing.T) {
	for _, on := range []bool{false, true} {
		t.Run("rule "+strconv.FormatBool(on), func(t *testing.T) {
			w := newWorld(t)
			w.secrets.number("concentration-save-dc", saveDCConcentration, "dc")
			dc := w.canaryOf("concentration-save-dc")
			if on {
				w.showSaveDC(true)
				w.secrets.allowNeedle(strconv.Itoa(saveDCConcentration), w.caio)
			}
			toren, bandit := w.banditsTurn()
			w.hurtToren(toren, bandit)
			enc := must(w.master.combat.GetEncounter(t.Context(), rq(&playv1.GetEncounterRequest{CampaignId: w.campaign}))).GetEncounter()
			window := ""
			for _, rw := range enc.GetReactionWindows() {
				if rw.GetKind() == playv1.ReactionKind_REACTION_KIND_CONCENTRATION_SAVE {
					window = rw.GetId()
				}
			}
			if window == "" {
				t.Fatal("Toren's concentration save is not asked")
			}
			beforeRoll := map[*person][]reply{}
			for _, p := range []*person{w.master, w.ana, w.caio} {
				beforeRoll[p] = w.reads(p)
			}
			roll := w.caio.call(playv1connect.CombatServiceResolveConcentrationSaveProcedure, &playv1.ResolveConcentrationSaveRequest{
				CampaignId: w.campaign, EncounterId: enc.GetId(), WindowId: window, IdempotencyKey: newKey(), Roll: &playv1.ResolveConcentrationSaveRequest_D20Face{D20Face: 5},
			})
			if !roll.ok() {
				t.Fatalf("Caio's save: status %d %s", roll.status, roll.body)
			}
			holds := func(p *person) (number, line bool) {
				replies := slices.Concat(beforeRoll[p], w.reads(p))
				if p == w.caio {
					replies = append(replies, roll)
				}
				for _, r := range replies {
					if p != w.master {
						for _, f := range w.inspect(p, r, nil) {
							t.Errorf("%s: %s", p.name, f)
						}
					}
					number = number || holdsNumber(r, dc)
					line = line || strings.Contains(string(r.body), "contra CD "+strconv.Itoa(saveDCConcentration))
				}
				return number, line
			}
			if number, line := holds(w.master); !number || !line {
				t.Errorf("the master's reads hold the DC = %v and the line's = %v: the needle is unreachable", number, line)
			}
			if number, line := holds(w.caio); number != on || line != on {
				t.Errorf("Caio's reads hold the DC = %v and the line's = %v, want both %v (the rule is %v)", number, line, on, on)
			}
			if number, line := holds(w.ana); number || line {
				t.Errorf("Ana's reads hold the DC = %v and the line's = %v of Toren's save", number, line)
			}
		})
	}
}

// Diabrete, an NPC tiefling, answers Toren's blow with a Hellish Rebuke, and the app rolls the Dexterity save
// of Toren's player at once, against a DC of 18. The master reads the DC always (the positive control); Caio,
// whose character rolls the save, only with the rule on; Ana never.
func TestTheDCOfAHellishRebukeSaveReachesOnlyItsRollerWhenTheTableShowsIt(t *testing.T) {
	for _, on := range []bool{false, true} {
		t.Run("rule "+strconv.FormatBool(on), func(t *testing.T) {
			w := newWorld(t)
			ctx := t.Context()
			m := w.master
			if on {
				w.showSaveDC(true)
				w.secrets.allowNeedle(strconv.Itoa(saveDCRebuke), w.caio)
			}
			sheet := w.fullSheet("class:fighter", "race:tiefling", 9)
			sheet.GetFull().BaseScores.Charisma = 20
			tiefling := must(m.characters.CreateCharacter(ctx, rq(&charactersv1.CreateCharacterRequest{
				CampaignId: w.campaign, Kind: charactersv1.CharacterKind_CHARACTER_KIND_BOSS, Name: w.secrets.public("tiefling-name"), Sheet: sheet,
			}))).GetCharacter()
			revealed := false
			w.scriptD20(ownFace)
			e := must(m.combat.AddCombatants(ctx, rq(&playv1.AddCombatantsRequest{
				CampaignId: w.campaign, EncounterId: w.encounter.GetId(), IdempotencyKey: newKey(), Participants: []*playv1.Participant{{CharacterId: tiefling.GetId(), Hidden: &revealed}},
			}))).GetEncounter()
			dagger := w.fullSheet("class:wizard", "race:human", 1)
			dagger.GetFull().WeaponKeys = []string{"equipment:dagger"}
			must(m.characters.UpdateCharacter(ctx, rq(&charactersv1.UpdateCharacterRequest{
				CampaignId: w.campaign, CharacterId: w.toren.GetId(), Revision: w.toren.GetRevision(), Name: w.toren.GetName(), Sheet: dagger,
			})))
			toren, devil := w.combatant(w.toren), w.combatantOf(e, tiefling.GetId())
			w.requireOwnTotal(devil.GetId())
			for id, sq := range map[string][2]int32{toren.GetId(): {12, 14}, devil.GetId(): {13, 14}} {
				must(m.combat.MoveCombatant(ctx, rq(&playv1.MoveCombatantRequest{CampaignId: w.campaign, EncounterId: e.GetId(), CombatantId: id, IdempotencyKey: newKey(), Col: sq[0], Row: sq[1], Forced: true})))
			}
			w.passTurnsTo(toren.GetId())
			hit := must(w.caio.combat.RollAttack(ctx, rq(&playv1.RollAttackRequest{
				CampaignId: w.campaign, EncounterId: e.GetId(), AttackerId: toren.GetId(), AttackKey: "equipment:dagger", TargetId: devil.GetId(),
				IdempotencyKey: newKey(), Roll: &playv1.RollAttackRequest_D20Face{D20Face: 19},
			})))
			pending := hit.GetPendingDamage().GetId()
			if pending == "" {
				t.Fatalf("Toren's attack = %v, want a hit", hit.GetRoll())
			}
			must(w.caio.combat.RollDamage(ctx, rq(&playv1.RollDamageRequest{CampaignId: w.campaign, EncounterId: e.GetId(), PendingDamageId: pending, IdempotencyKey: newKey(), Roll: &playv1.RollDamageRequest_RollInApp{RollInApp: true}})))
			win := windowOfKind(m, w, playv1.ReactionKind_REACTION_KIND_HELLISH_REBUKE)
			if win == nil {
				t.Fatal("Diabrete has no Hellish Rebuke window after being hurt")
			}
			if windowOfKind(w.ana, w, playv1.ReactionKind_REACTION_KIND_HELLISH_REBUKE) != nil || windowOfKind(w.caio, w, playv1.ReactionKind_REACTION_KIND_HELLISH_REBUKE) != nil {
				t.Fatal("a player reads the NPC's Hellish Rebuke window")
			}
			answer := must(m.combat.AnswerReaction(ctx, rq(&playv1.AnswerReactionRequest{
				CampaignId: w.campaign, EncounterId: e.GetId(), WindowId: win.GetId(), Answer: playv1.ReactionChoice_REACTION_CHOICE_USE, UseRacial: true, IdempotencyKey: newKey(),
			})))
			if answer.GetResult().GetHellishRebuke().GetSaveDc() != saveDCRebuke {
				t.Fatalf("the master's result = %v, want a save against DC %d", answer.GetResult(), saveDCRebuke)
			}
			w.encounter = e
			line := "CD " + strconv.Itoa(saveDCRebuke)
			for _, p := range []*person{w.master, w.caio, w.ana} {
				got := false
				for _, r := range w.reads(p) {
					if p != w.master {
						for _, f := range w.inspect(p, r, nil) {
							t.Errorf("%s: %s", p.name, f)
						}
					}
					got = got || strings.Contains(string(r.body), line)
				}
				want := p == w.master || (p == w.caio && on)
				if got != want {
					t.Errorf("%s reads the DC of the save = %v, want %v (the rule is %v)", p.name, got, want, on)
				}
			}
		})
	}
}
