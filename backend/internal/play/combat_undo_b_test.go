package play

import (
	"regexp"
	"slices"
	"sort"
	"strconv"
	"strings"
	"testing"

	"connectrpc.com/connect"

	campaignsv1 "github.com/PuraFome/meuRPG/backend/gen/meurpg/campaigns/v1"
	playv1 "github.com/PuraFome/meuRPG/backend/gen/meurpg/play/v1"
	rulesv1 "github.com/PuraFome/meuRPG/backend/gen/meurpg/rules/v1"
	"github.com/PuraFome/meuRPG/backend/internal/rules"
)

// More scenarios of the undo property (combat_undo_snapshot_test.go) for the events the
// master's Desfazer takes back (takeBack in combat_undo.go), one for each branch the
// first scenarios of combat_undo_kinds_test.go leave out. A scenario that names a
// difference (undoDelta) freezes what today's undo does not put back.

// heldRollBlob is the request a held roll keeps (roll_holds.request): the bytes of the
// attack's message, whose idempotency key and IDs are drawn at random for each run.
var heldRollBlob = regexp.MustCompile(`request=\\x[0-9a-f]+`)

// runWithHolds is undoWatch.run for an action that made a held roll (a Bardic Inspiration
// die or Lucky asked about it). The held roll keeps the attack's request as the bytes of a
// message with random IDs, which the picture cannot name as it names a UUID, so the one
// line of it reads "request=<blob>" here and the rest of the check is run's.
func (w *undoWatch) runWithHolds(t *testing.T, name string, want undoDelta, do func()) {
	t.Helper()
	a := w.a
	before := w.picture(t)
	do()
	e := a.get(t, a.master)
	id := a.log(t, a.master, e).GetUndoableEventId()
	if id == "" {
		t.Fatalf("%s: nothing to undo after it", name)
	}
	if len(difference(before, w.picture(t))) == 0 {
		t.Fatalf("%s changed nothing", name)
	}
	if err := a.undo(t, a.master, e, id); err != nil {
		t.Fatalf("%s: UndoLastAction() error = %v", name, err)
	}
	got := difference(before, w.picture(t))
	for i, l := range got {
		got[i] = heldRollBlob.ReplaceAllString(l, "request=<blob>")
	}
	wantSorted := slices.Clone([]string(want))
	sort.Strings(wantSorted)
	if !slices.Equal(unnumbered(got), unnumbered(wantSorted)) {
		t.Errorf("%s: after the undo the combat differs from before the action by\n%s\nwant\n%s", name, strings.Join(got, "\n"), strings.Join(wantSorted, "\n"))
	}
}

// deltaAnsweredHold: the row of a held roll stays after the answer (AnswerRollHold, queries.sql)
// as the record of the answer's idempotency key, and the undo of the attack does not delete it
// (the attack case of takeBack, combat_undo.go, puts back the action and the die, not the
// hold). Nothing reads it: only the unanswered holds are listed, so it is invisible. lucky is
// the columns of the Lucky answer, for a roll that was asked about its 1.
func deltaAnsweredHold(face, modifier int, lucky bool) undoDelta {
	out := undoDelta{
		"+ db/roll_holds[#].answer_key=#", "+ db/roll_holds[#].combatant_id=#", "+ db/roll_holds[#].encounter_id=#",
		"+ db/roll_holds[#].idempotency_key=#", "+ db/roll_holds[#].request=<blob>", "+ db/roll_holds[#].round=1",
		"+ db/roll_holds[#].face=" + strconv.Itoa(face), "+ db/roll_holds[#].modifier=" + strconv.Itoa(modifier),
	}
	if lucky {
		return append(out, "+ db/roll_holds[#].lucky_answer_key=#", "+ db/roll_holds[#].lucky_index=0", "+ db/roll_holds[#].lucky_to=15")
	}
	return append(out, "+ db/roll_holds[#].lucky_answer_key=<nil>", "+ db/roll_holds[#].lucky_index=<nil>", "+ db/roll_holds[#].lucky_to=<nil>")
}

// TestUndoKindAttackRolledBardicDie: the attack that used a Bardic Inspiration die
// holds the die again.
func TestUndoKindAttackRolledBardicDie(t *testing.T) {
	t.Parallel()
	a := newResourceTable(t)
	e := a.inspired(t)
	w := newUndoWatch(t, a)

	w.runWithHolds(t, "an attack with the die", deltaAnsweredHold(5, 6, false), func() {
		a.h.roller.queue(5)
		held, err := a.attack(t, a.caio, e, "Tavo", battleaxe, "Goblin", inAppRoll)
		if err != nil {
			t.Fatalf("RollAttack() error = %v", err)
		}
		a.h.roller.queue(7)
		if _, err := a.answer(t, a.caio, e, held.GetInspirationOffer().GetHoldId(), useDieInApp); err != nil {
			t.Fatalf("AnswerBardicInspiration() error = %v", err)
		}
	})
}

// TestUndoKindAttackRolledExtraAction: the attack of Haste's extra action gives that
// action back and leaves the action of the turn alone.
func TestUndoKindAttackRolledExtraAction(t *testing.T) {
	t.Parallel()
	a := newCasters(t)
	e := a.closeFight(t)
	a.mustAddEffect(t, e, "spell:haste", []string{"Toren"}, a.rounds(t, 3, "Toren"))
	w := newUndoWatch(t, a)

	w.run(t, "an attack with the extra action", nil, func() {
		a.mustAttack(t, a.caio, e, "Toren", battleaxe, "Goblin", func(r *playv1.RollAttackRequest) { d20(15)(r); useExtra(r) })
	})
}

// TestUndoKindAttackRolledBonusAction: the attack of the bonus action of two-weapon
// fighting. The attack of the action before it already marked the turn with a melee hit,
// so the undo of this one leaves the mark where it was.
func TestUndoKindAttackRolledBonusAction(t *testing.T) {
	t.Parallel()
	a := newArmedWith(t, func(a *armed) {
		weapons := []string{shortsword, dagger}
		a.toren = a.caio.heroWith(t, a.campaignID, "Toren", classLevel("class:fighter", 1, ""), abilities(16, 13, 14, 10, 8), weapons, nil,
			[]string{"feature:fighter-fighting-style-defense"})
		a.pens = a.ana.hero(t, a.campaignID, "Pensantus", "class:wizard", "race:gnome", 1, abilities(10, 14, 12, 16, 8), nil, []string{fireBolt})
		a.bri = a.bia.hero(t, a.campaignID, "Brisa", "class:fighter", "race:human", 2, abilities(10, 16, 14, 10, 8), []string{rapier}, nil)
	})
	e := a.closeFight(t)
	w := newUndoWatch(t, a)

	a.mustAttack(t, a.caio, e, "Toren", shortsword, "Goblin", d20(15))
	w.run(t, "the off hand attack", nil, func() {
		a.mustAttack(t, a.caio, e, "Toren", dagger, "Goblin", d20(15))
	})
}

// TestUndoKindAttackRolledFlurry: the strikes of Flurry of Blows count down on the
// combatant (the Martial Arts strike is the bonus action's), and the undo counts them up.
func TestUndoKindAttackRolledFlurry(t *testing.T) {
	t.Parallel()
	a := newArmedWith(t, func(a *armed) {
		a.toren = a.caio.heroWith(t, a.campaignID, "Toren", classLevel("class:monk", 3, ""), abilities(10, 16, 14, 14, 8), nil, nil, nil)
		a.pens = a.ana.hero(t, a.campaignID, "Pensantus", "class:wizard", "race:gnome", 1, abilities(10, 14, 12, 16, 8), nil, []string{fireBolt})
		a.bri = a.bia.hero(t, a.campaignID, "Brisa", "class:fighter", "race:human", 2, abilities(10, 16, 14, 10, 8), []string{rapier}, nil)
	})
	e := a.threeAndAGoblin(t)
	w := newUndoWatch(t, a)

	w.run(t, "the Attack action's strike", nil, func() { a.mustAttack(t, a.caio, e, "Toren", unarmed, "Goblin", d20(15)) })
	w.run(t, "the Martial Arts strike", nil, func() { a.mustAttack(t, a.caio, e, "Toren", unarmed, "Goblin", d20(15)) })
	e = a.turnOf(t, "Toren")
	a.mustAttack(t, a.caio, e, "Toren", unarmed, "Goblin", d20(15))
	w.run(t, "Flurry of Blows", nil, func() {
		if _, err := a.action(t, a.caio, e, "Toren", flurry); err != nil {
			t.Fatalf("TakeAction(flurry) error = %v", err)
		}
	})
	if _, err := a.action(t, a.caio, e, "Toren", flurry); err != nil {
		t.Fatalf("TakeAction(flurry) error = %v", err)
	}
	w.run(t, "a flurry strike", nil, func() { a.mustAttack(t, a.caio, e, "Toren", unarmed, "Goblin", d20(15)) })
}

// TestUndoKindAttackRolledLucky: the attack whose 1 was rolled again.
func TestUndoKindAttackRolledLucky(t *testing.T) {
	t.Parallel()
	a := newLuckyTable(t)
	e := a.luckyFight(t)
	w := newUndoWatch(t, a)

	w.runWithHolds(t, "a rerolled attack", deltaAnsweredHold(1, 5, true), func() {
		held, err := a.attack(t, a.caio, e, "Pipo", battleaxe, "Goblin", d20(1))
		if err != nil {
			t.Fatalf("RollAttack() error = %v", err)
		}
		if _, err := a.answerLucky(t, a.caio, e, held.GetLuckyOffer().GetHoldId(), rerollTyped(15)); err != nil {
			t.Fatalf("AnswerLucky() error = %v", err)
		}
	})
}

// TestUndoKindAttackRolledFromHiding: an attack that gave the hider's position away
// (restoreContestState): the hiding comes back.
func TestUndoKindAttackRolledFromHiding(t *testing.T) {
	t.Parallel()
	a := newSneaks(t)
	c := a.arena(t, arenaPlan{})
	tried := c.mustHide(t, a.caio, "Toren", cunningHide, 12)
	c.mustResolveHide(t, tried.GetId(), nil)
	w := newUndoWatch(t, a)

	w.run(t, "an attack from hiding", nil, func() {
		a.mustAttack(t, a.caio, c.refresh(t), "Toren", rapier, c.hob, func(r *playv1.RollAttackRequest) { r.D20Faces = []int32{2, 3} })
	})
}

// TestUndoKindAttackRolledUsingAHelp: an attack that used the Help aimed at its target.
func TestUndoKindAttackRolledUsingAHelp(t *testing.T) {
	t.Parallel()
	a := newSneaks(t)
	c := a.arena(t, arenaPlan{})
	c.advance(t, "Brisa")
	if _, err := c.moveTo(t, a.bia, "Brisa", 5, 3); err != nil {
		t.Fatalf("moving Brisa next to the Hobgoblin: %v", err)
	}
	helped(t)(c.helpAttack(t, a.bia, "Brisa", "Toren", c.hob))
	c.advance(t, "Goblin")
	c.advance(t, "Toren")
	w := newUndoWatch(t, a)

	w.run(t, "an attack that uses the Help", nil, func() {
		a.mustAttack(t, a.caio, c.refresh(t), "Toren", rapier, c.hob, func(r *playv1.RollAttackRequest) { r.D20Faces = []int32{2, 3} })
	})
}

// TestUndoKindAttackRolledSpendingTheMockery: the attack of a goblin made with the
// disadvantage of Vicious Mockery spends the effect, and the undo gives it back.
func TestUndoKindAttackRolledSpendingTheMockery(t *testing.T) {
	t.Parallel()
	a := newMockers(t)
	e := a.mockFight(t, false)
	if got := a.mock(t, e, 5); got != playv1.SaveOutcome_SAVE_OUTCOME_FAILED {
		t.Fatalf("the goblin's save = %v, want failed", got)
	}
	e = a.passTo(t, e, "Goblin")
	w := newUndoWatch(t, a)

	w.run(t, "the mocked attack", nil, func() {
		a.h.roller.queue(3, 2)
		a.mustAttack(t, a.master, e, "Goblin", sword, "Orla", inAppRoll)
	})
}

// TestUndoKindAttackRolledFrenzy: the bonus action attack of a Berserker's frenzy.
func TestUndoKindAttackRolledFrenzy(t *testing.T) {
	t.Parallel()
	a, e := berserkerTable(t, 3, "subclass:berserker")
	if _, err := a.rage(t, e, true); err != nil {
		t.Fatalf("TakeAction(Rage, frenzy) error = %v", err)
	}
	a.mustAttack(t, a.bia, e, "Brisa", battleaxe, "Goblin", d20(15))
	e = a.passTo(t, a.mustEndTurnDiscarding(t, e), "Brisa")
	a.mustAttack(t, a.bia, e, "Brisa", battleaxe, "Goblin", d20(15))
	w := newUndoWatch(t, a)

	w.run(t, "the frenzy attack", nil, func() { a.mustAttack(t, a.bia, e, "Brisa", battleaxe, "Goblin", d20(15)) })
}

// TestUndoKindAttackRolledOfAnOffer: the opportunity attack of an offer (ev.OfferID)
// spends the reactor's reaction and answers the offer, and the undo makes the offer wait
// again.
func TestUndoKindAttackRolledOfAnOffer(t *testing.T) {
	t.Parallel()
	c := newCave(t)
	c.oppFight(t)
	c.leaveGoblin(t)
	offer := c.offersOf(t, c.master)[0]
	w := newUndoWatch(t, c.armed)

	w.run(t, "the attack of the offer", nil, func() {
		if _, err := c.offerAttack(t, c.master, "Goblin 1", sword, "Toren", offer.GetId(), d20(15)); err != nil {
			t.Fatalf("the offer's attack error = %v", err)
		}
	})
}

// TestUndoKindReactionDeclinedOffer: an opportunity offer turned down, then skipped (ev.OfferID
// of the declined case).
func TestUndoKindReactionDeclinedOffer(t *testing.T) {
	t.Parallel()
	c := newCave(t)
	c.oppFight(t)
	c.leaveGoblin(t)
	offer := c.offersOf(t, c.master)[0]
	w := newUndoWatch(t, c.armed)

	w.run(t, "declining the offer", nil, func() {
		if err := c.decline(t, c.master, offer.GetId()); err != nil {
			t.Fatalf("DeclineOpportunity() error = %v", err)
		}
	})
	w.run(t, "skipping the offer", nil, func() {
		if err := c.skip(t, c.master, offer.GetId()); err != nil {
			t.Fatalf("SkipOpportunity() error = %v", err)
		}
	})
}

// TestUndoKindActionTakenLayOnHands: the touch spends points of the pool and gives
// hit points to a player's character, to an NPC (which keeps them on the combatant), or
// takes a condition away (takeBackResource, resLayOnHands).
func TestUndoKindActionTakenLayOnHands(t *testing.T) {
	t.Parallel()
	a := newResourceTable(t)
	e := a.resourceFight(t)
	a.correct(t, a.pens, hpIs(10))
	a.setConditions(t, e, "Nael", "condition:poisoned", "condition:frightened")
	if _, err := a.adjustHP(t, e, "Goblin", damageHP(3)); err != nil {
		t.Fatalf("AdjustCombatantHitPoints() error = %v", err)
	}
	w := newUndoWatch(t, a)

	touch := func(target string, effect func(*playv1.UseLayOnHandsRequest)) func() {
		return func() {
			if _, err := a.layOnHands(t, a.caio, e, "Tavo", target, effect); err != nil {
				t.Fatalf("UseLayOnHands(%s) error = %v", target, err)
			}
		}
	}
	w.run(t, "a touch on a player's character", nil, touch("Nael", healAmount(8)))
	w.run(t, "a touch on an NPC", nil, touch("Goblin", healAmount(3)))
	w.run(t, "a cure of a poison", nil, touch("Nael", cureOf(playv1.LayOnHandsCure_LAY_ON_HANDS_CURE_POISON)))
	w.run(t, "a cure of a disease", nil, touch("Nael", cureOf(playv1.LayOnHandsCure_LAY_ON_HANDS_CURE_DISEASE)))
}

// TestUndoKindActionTakenLayOnHandsAtZero: the touch on a character at 0 hit points
// that has a failed death save gives back the hit points and the death saves.
func TestUndoKindActionTakenLayOnHandsAtZero(t *testing.T) {
	t.Parallel()
	a := newResourceTable(t)
	e := a.resourceFight(t)
	a.correct(t, a.pens, hpIs(0))
	a.passTo(t, e, "Nael")
	if _, err := a.deathSave(t, a.ana, e, "Nael", deathFace(5)); err != nil {
		t.Fatalf("RollDeathSave() error = %v", err)
	}
	e = a.turnOf(t, "Tavo")
	w := newUndoWatch(t, a)

	w.run(t, "a touch on a character at 0", nil, func() {
		if _, err := a.layOnHands(t, a.caio, e, "Tavo", "Nael", healAmount(5)); err != nil {
			t.Fatalf("UseLayOnHands() error = %v", err)
		}
	})
}

// sorcererFirst starts the combat of newResourceTable with Nael first.
func (a *armed) sorcererFirst(t *testing.T) *playv1.Encounter {
	t.Helper()
	return a.start(t, plan{
		npcs: []*playv1.Participant{{CharacterId: a.goblin.GetId()}}, npcRolls: []int{3},
		players: map[string]int32{"Nael": 18, "Tavo": 14, "Orla": 10}, reveal: []string{"Goblin"},
		at: map[string][2]int32{"Tavo": {3, 3}, "Nael": {4, 3}, "Orla": {5, 3}, "Goblin": {4, 4}},
	})
}

// deltaSlotsCreatedZeros: the first slot a sorcerer makes writes the list of slots made (one
// count for each level, characters/rests.go CreateSpellSlot) where the vitals had an empty
// list, and UndoCreateSpellSlot (rests.go) takes the count back to 0 without emptying the
// list. A list of zeros reads as no slot made, so it is invisible.
var deltaSlotsCreatedZeros = func() undoDelta {
	out := undoDelta{"- db/character_vitals[#].spell_slots_created=[]"}
	for i := range 9 {
		out = append(out, "+ db/character_vitals[#].spell_slots_created["+strconv.Itoa(i)+"]=0")
	}
	return out
}()

// TestUndoKindActionTakenFlexibleCasting: a slot made of sorcery points, and points made
// of a slot (resCreateSlot, resConvert).
func TestUndoKindActionTakenFlexibleCasting(t *testing.T) {
	t.Parallel()
	a := newResourceTable(t)
	e := a.sorcererFirst(t)
	w := newUndoWatch(t, a)

	w.run(t, "a slot made of points", deltaSlotsCreatedZeros, func() {
		if _, err := a.createSlot(t, a.ana, e, "Nael", 2); err != nil {
			t.Fatalf("CreateSpellSlot() error = %v", err)
		}
	})
	a.correct(t, a.pens, func(r *playv1.AdjustCharacterVitalsRequest) {
		r.ResourcesUsed = []*playv1.ResourceUsed{{Key: rules.SorceryPointsKey, Used: 4}}
	})
	w.run(t, "points made of a slot", nil, func() {
		if _, err := a.convertSlot(t, a.ana, e, "Nael", 1); err != nil {
			t.Fatalf("ConvertSpellSlot() error = %v", err)
		}
	})
}

// TestUndoKindActionTakenBardicInspirationGiven: the use is spent and the die goes to
// the ally (resBardicGive).
func TestUndoKindActionTakenBardicInspirationGiven(t *testing.T) {
	t.Parallel()
	a := newResourceTable(t)
	e := a.start(t, plan{
		npcs: []*playv1.Participant{{CharacterId: a.goblin.GetId()}}, npcRolls: []int{3},
		players: map[string]int32{"Orla": 20, "Tavo": 14, "Nael": 10}, reveal: []string{"Goblin"},
		at: map[string][2]int32{"Orla": {3, 3}, "Tavo": {4, 3}, "Nael": {5, 3}, "Goblin": {4, 4}},
	})
	w := newUndoWatch(t, a)

	w.run(t, "the die given to an ally", nil, func() {
		if _, err := a.giveInspiration(t, a.bia, e, "Orla", "Tavo"); err != nil {
			t.Fatalf("GiveBardicInspiration() error = %v", err)
		}
	})
}

// TestUndoKindActionTakenStates: the actions that begin a state of the combatant (a rage,
// a dodge, a reckless attack) take it off again (ev.StateID).
func TestUndoKindActionTakenStates(t *testing.T) {
	t.Parallel()
	a, e := rollsTable(t)
	w := newUndoWatch(t, a)

	for _, key := range []string{"feature:rage", "feature:reckless-attack", "standard:dodge"} {
		w.run(t, key, nil, func() {
			if _, err := a.action(t, a.bia, e, "Brisa", key); err != nil {
				t.Fatalf("TakeAction(%s) error = %v", key, err)
			}
		})
	}
}

// TestUndoKindActionTakenMonkFeatures: Patient Defense (a ki use, and a dodge), Step of
// the Wind (which doubles the jump distance of the turn).
func TestUndoKindActionTakenMonkFeatures(t *testing.T) {
	t.Parallel()
	c := monkCave(t)
	e := c.fight(t)
	w := newUndoWatch(t, c.armed)

	w.run(t, "Patient Defense", nil, func() {
		if _, err := c.action(t, c.caio, e, "Toren", patient); err != nil {
			t.Fatalf("TakeAction(patient defense) error = %v", err)
		}
	})
	w.run(t, "Step of the Wind", nil, func() {
		if _, err := c.action(t, c.caio, e, "Toren", windDash); err != nil {
			t.Fatalf("TakeAction(step of the wind) error = %v", err)
		}
	})
}

// TestUndoKindActionTakenAfterTheSurge: an action taken after Action Surge finds the
// surge marked (SurgedBefore) and the undo leaves it so.
func TestUndoKindActionTakenAfterTheSurge(t *testing.T) {
	t.Parallel()
	a := newCasters(t)
	e := a.castersFight(t, 1)
	a.passTo(t, e, "Toren")
	if _, err := a.feature(t, a.caio, e, "Toren", actionSurgeKey, noTakeRoll); err != nil {
		t.Fatalf("TakeAction(surge) error = %v", err)
	}
	w := newUndoWatch(t, a)

	w.run(t, "Disparada after the surge", nil, func() {
		if _, err := a.action(t, a.caio, e, "Toren", "standard:dash"); err != nil {
			t.Fatalf("TakeAction(dash) error = %v", err)
		}
	})
}

// TestUndoKindActionTakenAfterARun: an action taken after a walk breaks the running start
// (RunBefore), which the undo puts back.
func TestUndoKindActionTakenAfterARun(t *testing.T) {
	t.Parallel()
	a := newCasters(t)
	e := a.castersFight(t, 1)
	a.passTo(t, e, "Toren")
	if _, err := a.caio.combat.MoveCombatant(t.Context(), connect.NewRequest(&playv1.MoveCombatantRequest{
		CampaignId: a.campaignID, EncounterId: e.GetId(), CombatantId: a.id(t, "Toren"), IdempotencyKey: newKey(), Col: 6, Row: 6,
	})); err != nil {
		t.Fatalf("MoveCombatant() error = %v", err)
	}
	var run int32
	if err := a.h.pool.QueryRow(t.Context(), "SELECT last_move_dft FROM combatants WHERE id = $1", a.id(t, "Toren")).Scan(&run); err != nil || run == 0 {
		t.Fatalf("the running start after the walk = %d, %v; want some", run, err)
	}
	w := newUndoWatch(t, a)

	w.run(t, "Esquiva after the walk", nil, func() {
		if _, err := a.action(t, a.caio, e, "Toren", "standard:dodge"); err != nil {
			t.Fatalf("TakeAction(dodge) error = %v", err)
		}
	})
}

// TestUndoKindActionTakenDisengagedBefore: a Disengage taken when the combatant had
// disengaged already (the rogue's Cunning Action after the Disengage action) leaves it
// disengaged when taken back.
func TestUndoKindActionTakenDisengagedBefore(t *testing.T) {
	t.Parallel()
	a, e := rollsTable(t)
	a.passTo(t, e, "Toren")
	if _, err := a.action(t, a.caio, e, "Toren", "standard:disengage"); err != nil {
		t.Fatalf("TakeAction(disengage) error = %v", err)
	}
	w := newUndoWatch(t, a)

	w.run(t, "a second Disengage", nil, func() {
		if _, err := a.action(t, a.caio, e, "Toren", "feature:cunning-action:disengage"); err != nil {
			t.Fatalf("TakeAction(cunning disengage) error = %v", err)
		}
	})
}

// deltaAnsweredInspirationHold: the row of the question about a Bardic Inspiration die
// stays after the answer (AnswerInspirationHold, queries.sql) as the record of the
// answer's idempotency key, and the undo of the cast (the spell case of takeBack,
// combat_undo.go) holds the die again without deleting it. Only the unanswered holds are
// listed, so nobody reads it.
var deltaAnsweredInspirationHold = undoDelta{
	"+ db/inspiration_holds[#].answer_key=#", "+ db/inspiration_holds[#].campaign_id=#", "+ db/inspiration_holds[#].character_id=#",
	"+ db/inspiration_holds[#].counted=0", "+ db/inspiration_holds[#].faces[0]=4", "+ db/inspiration_holds[#].idempotency_key=#",
	"+ db/inspiration_holds[#].kind=spell_attack", "+ db/inspiration_holds[#].lucky_answer_key=<nil>",
	"+ db/inspiration_holds[#].lucky_index=<nil>", "+ db/inspiration_holds[#].lucky_to=<nil>", "+ db/inspiration_holds[#].modifier=5",
	"+ db/inspiration_holds[#].physical=true", "+ db/inspiration_holds[#].request=<blob>", "+ db/inspiration_holds[#].round=1",
	"+ db/inspiration_holds[#].total=9", "+ db/inspiration_holds[#].user_id=#",
}

// TestUndoKindSpellCastBardicDie: the spell attack that used a Bardic Inspiration die holds
// the die again (ev.Die).
func TestUndoKindSpellCastBardicDie(t *testing.T) {
	t.Parallel()
	a := newCasters(t)
	e := a.start(t, plan{
		npcs:     []*playv1.Participant{{CharacterId: a.goblin.GetId()}},
		npcRolls: []int{3},
		players:  map[string]int32{"Toren": 18, "Pensantus": 10, "Brisa": 1},
		reveal:   []string{"Goblin"},
		at:       map[string][2]int32{"Toren": {3, 3}, "Goblin": {4, 3}, "Pensantus": {10, 3}, "Brisa": {4, 9}},
	})
	a.passTo(t, e, "Brisa")
	e = a.get(t, a.master)
	a.giveCombatantDie(t, "Brisa", e)
	w := newUndoWatch(t, a)

	w.runWithHolds(t, "a spell attack with the die", deltaAnsweredInspirationHold, func() {
		typed := func(r *playv1.CastSpellRequest) { r.Roll = &playv1.CastSpellRequest_D20Face{D20Face: 4} }
		res, err := a.cast(t, a.bia, e, "Brisa", guidingBolt, slotOfLevel(1), a.at(t, "Goblin"), typed)
		if err != nil {
			t.Fatalf("CastSpell() error = %v", err)
		}
		if _, err := a.answerOutside(t, a.bia, res.GetInspirationOffer().GetHoldId(), true, false, 8); err != nil {
			t.Fatalf("AnswerOutsideInspiration() error = %v", err)
		}
	})
}

// TestUndoKindSpellCastLasting: the effect a cast put on its target (ev.Lasting) goes away.
func TestUndoKindSpellCastLasting(t *testing.T) {
	t.Parallel()
	a := newCastersWithMageArmor(t)
	e := a.castersFight(t, 1)
	w := newUndoWatch(t, a)

	w.run(t, "Armadura Arcana", nil, func() {
		a.mustCast(t, a.ana, e, "Pensantus", mageArmorKey, slotOfLevel(1), a.at(t, "Toren"), noCastRoll)
	})
}

// newRangers is newCasters with Brisa a ranger of level 5 who knows Hunter's Mark.
func newRangers(t *testing.T) *armed {
	t.Helper()
	return newArmedWith(t, func(a *armed) {
		a.toren = a.caio.hero(t, a.campaignID, "Toren", "class:fighter", "race:human", 5,
			&rulesv1.AbilityScores{Strength: 16, Dexterity: 13, Constitution: 14, Intelligence: 10, Wisdom: 10, Charisma: 8}, []string{battleaxe}, nil)
		a.pens = a.ana.caster(t, a.campaignID, "Pensantus", "class:wizard", "race:gnome", 3,
			&rulesv1.AbilityScores{Strength: 10, Dexterity: 14, Constitution: 12, Intelligence: 16, Wisdom: 10, Charisma: 8}, nil, []string{fireBolt},
			[]string{magicMissileSpell}, []string{magicMissileSpell})
		a.bri = a.bia.caster(t, a.campaignID, "Brisa", "class:ranger", "race:human", 5,
			&rulesv1.AbilityScores{Strength: 10, Dexterity: 16, Constitution: 14, Intelligence: 10, Wisdom: 16, Charisma: 8}, []string{rapier}, nil,
			[]string{huntersMark}, []string{huntersMark})
	})
}

// TestUndoKindSpellCastHuntersMark: the cast marks its target and takes the concentration;
// cast again on another target, it moves the mark.
func TestUndoKindSpellCastHuntersMark(t *testing.T) {
	t.Parallel()
	a := newRangers(t)
	e := a.castersFight(t, 1)
	a.passTo(t, e, "Brisa")
	w := newUndoWatch(t, a)

	w.run(t, "Marca do Caçador", nil, func() {
		a.mustCast(t, a.bia, e, "Brisa", huntersMark, slotOfLevel(1), a.at(t, "Goblin"), noCastRoll)
	})
	a.mustCast(t, a.bia, e, "Brisa", huntersMark, slotOfLevel(1), a.at(t, "Goblin"), noCastRoll)
	a.passTo(t, a.mustEndTurnDiscarding(t, e), "Brisa")
	w.run(t, "Marca do Caçador on another target", nil, func() {
		a.mustCast(t, a.bia, a.get(t, a.master), "Brisa", huntersMark, slotOfLevel(1), a.at(t, "Capitão Goblin"), noCastRoll)
	})
}

// TestUndoKindSpellCastMetamagic: the cast with a Metamagic option spends sorcery points
// and the undo gives them back (resMetamagic).
func TestUndoKindSpellCastMetamagic(t *testing.T) {
	t.Parallel()
	a := newResourceTable(t)
	e := a.sorcererFirst(t)
	w := newUndoWatch(t, a)

	w.run(t, "Bola de Fogo with Careful Spell", nil, func() {
		if _, err := a.castWith(t, a.ana, e, "Nael", fireballSpell, slotOfLevel(3), a.at(t, "Goblin"), &playv1.MetamagicChoice{Key: rules.MetamagicCareful, CarefulIds: []string{a.id(t, "Goblin")}}); err != nil {
			t.Fatalf("CastSpell() error = %v", err)
		}
	})
}

// TestUndoKindSpellCastCreatures: Conjure Animals makes creatures and takes the
// concentration (ev.Created); a second casting dismisses the first creatures (ev.Dismissed,
// ev.ConcBefore) and the undo brings them back.
func TestUndoKindSpellCastCreatures(t *testing.T) {
	t.Parallel()
	a := newSummoners(t)
	e := a.toSalvia(t, a.summonersFight(t))
	w := newUndoWatch(t, a)

	w.run(t, "Conjurar Animais", nil, func() {
		if _, err := a.summonWolves(t, a.bia, e, 11); err != nil {
			t.Fatalf("CastSpell() error = %v", err)
		}
	})
	if _, err := a.summonWolves(t, a.bia, e, 11); err != nil {
		t.Fatalf("CastSpell() error = %v", err)
	}
	e = a.turnOf(t, "Sálvia")
	w.run(t, "Conjurar Animais again", nil, func() {
		if _, err := a.summonWolves(t, a.bia, e, 12); err != nil {
			t.Fatalf("CastSpell() error = %v", err)
		}
	})
}

// TestUndoKindSpellCastHitPointSpells: the spells that change hit points, death saves and
// conditions at once (h.Fx, h.Restore, h.DeathBefore, h.CondSet), on a player's character
// and on an NPC.
func TestUndoKindSpellCastHitPointSpells(t *testing.T) {
	t.Parallel()
	a := newHPCasters(t)
	e := a.castersFight(t, 1)
	a.stand(t, map[string][2]int32{"Toren": {5, 3}, "Capitão Goblin": {7, 4}})
	w := newUndoWatch(t, a)

	kill := func(target string) func() {
		return func() { a.mustCast(t, a.ana, e, "Pensantus", wordKill, slotOfLevel(9), a.at(t, target), noCastRoll) }
	}
	w.run(t, "Palavra de Poder Matar on an NPC", nil, kill("Goblin"))
	w.run(t, "Palavra de Poder Matar on a player's character", nil, kill("Toren"))
	w.run(t, "Spray Prismático", nil, func() {
		a.h.roller.queue(6, 6, 6, 6, 6, 6)
		a.mustCast(t, a.ana, e, "Pensantus", colorSpray, slotOfLevel(1), a.at(t, "Capitão Goblin", "Goblin"), poolInApp)
	})

	a.correct(t, a.toren, hpIs(10))
	a.setConditions(t, e, "Toren", blindedC, poisonedC, deafenedC)
	if _, err := a.adjustHP(t, e, "Goblin", damageHP(3)); err != nil {
		t.Fatalf("AdjustCombatantHitPoints() error = %v", err)
	}
	a.passTo(t, e, "Brisa")
	heal := func(target string) func() {
		return func() { a.mustCast(t, a.bia, e, "Brisa", healSpell, slotOfLevel(6), a.at(t, target), noCastRoll) }
	}
	w.run(t, "Cura Completa on a character with conditions", nil, heal("Toren"))
	w.run(t, "Cura Completa on an NPC", nil, heal("Goblin"))

	a.correct(t, a.toren, hpIs(0))
	a.put(t, "Toren", 4, 6) // next to Brisa: the spell touches
	w.run(t, "Estabilizar", nil, func() {
		a.mustCast(t, a.bia, e, "Brisa", spareDying, nil, a.at(t, "Toren"), noCastRoll)
	})
}

// TestUndoKindSpellCastTemporaryAndMaximumHitPoints: False Life (temporary hit points) and
// Aid (the maximum and the bonus, on an NPC, a character, and a character at 0 hit points).
func TestUndoKindSpellCastTemporaryAndMaximumHitPoints(t *testing.T) {
	t.Parallel()
	a := newTempHPCasters(t)
	e := a.castersFight(t, 1)
	a.correct(t, a.pens, hpIs(5))
	w := newUndoWatch(t, a)

	w.run(t, "Vitalidade Falsa", nil, func() {
		a.h.roller.queue(3)
		a.mustCast(t, a.ana, e, "Pensantus", falseLife, slotOfLevel(1), nil, poolInApp)
	})
	a.passTo(t, e, "Brisa")
	w.run(t, "Ajuda on an NPC and a character", nil, func() {
		a.mustCast(t, a.bia, e, "Brisa", aidSpell, slotOfLevel(2), a.at(t, "Goblin", "Toren"), noCastRoll)
	})
	a.correct(t, a.toren, hpIs(0))
	w.run(t, "Ajuda on a character at 0", nil, func() {
		a.mustCast(t, a.bia, e, "Brisa", aidSpell, slotOfLevel(2), a.at(t, "Toren"), noCastRoll)
	})
}

// TestUndoKindSpellCastHiddenHits: an area spell that hit hidden creatures, with the table
// asking the master (the question, ev.PendingReveal) and revealing them (ev.Revealed).
func TestUndoKindSpellCastHiddenHits(t *testing.T) {
	t.Parallel()
	c := newAreaCave(t)
	c.askFight(t)
	w := newUndoWatch(t, c.armed)

	w.run(t, "a Bola de Fogo that asks", nil, func() {
		c.h.roller.queue(10, 10, 10, 10)
		c.mustCastArea(t, c.ana, "Pensantus", fireball, slotOfLevel(3), at(20, 7), nil)
	})
	c.setRules(t, func(r *campaignsv1.TableRules) {
		r.HiddenAreaHits = campaignsv1.HiddenAreaHitRule_HIDDEN_AREA_HIT_RULE_REVEAL
	})
	w.run(t, "a Bola de Fogo that reveals", nil, func() {
		c.h.roller.queue(10, 10, 10, 10)
		c.mustCastArea(t, c.ana, "Pensantus", fireball, slotOfLevel(3), at(20, 7), nil)
	})
}

// TestUndoKindSpellCastFromHiding: a spell with a verbal component gives the hider's position
// away (HidBefore), and the undo hides it again.
func TestUndoKindSpellCastFromHiding(t *testing.T) {
	t.Parallel()
	a := newSneaks(t)
	c := a.arena(t, arenaPlan{})
	c.advance(t, "Pensantus")
	tried := c.mustHide(t, a.ana, "Pensantus", "", 14)
	c.mustResolveHide(t, tried.GetId(), nil)
	c.advance(t, "Goblin")
	c.advance(t, "Pensantus")
	w := newUndoWatch(t, a)

	w.run(t, "Mísseis Mágicos from hiding", nil, func() {
		a.mustCast(t, a.ana, c.refresh(t), "Pensantus", magicMissileSpell, slotOfLevel(1), []*playv1.SpellTarget{darts(a, t, c.hob, 3)}, noCastRoll)
	})
}

// deltaNPCSlotsZero: an NPC's spent slots are a JSON object on the combatant (bumpUsed,
// combat_reaction_kit.go:190) that starts as {}; the Mage's Escudo writes {"1": 1} and the undo
// (giveBackNPCSlot, the NPC branch of the reaction case of takeBack, combat_undo.go) writes
// {"1": 0}: the level stays as a key with no use. Nothing reads a zero as a slot spent, so it is
// invisible.
var deltaNPCSlotsZero = undoDelta{"+ db/combatants[#].slots_used.1=0", "- db/combatants[#].slots_used={}"}

// TestUndoKindReactionUsedByAnNPC: the Mage's Escudo, answered by the master, counts the NPC's
// slot on the combatant (spendReactionSlot), and the undo gives it back (giveBackNPCSlot).
func TestUndoKindReactionUsedByAnNPC(t *testing.T) {
	t.Parallel()
	a, e, window := shieldedHit(t)
	w := newUndoWatch(t, a)

	w.run(t, "the Mage's Escudo", deltaNPCSlotsZero, func() { a.mustAnswer(t, a.master, e, window.GetId(), useAnswer(1)) })
}

// TestUndoKindReactionUsedStoppingOtherHits: Escudo's armor class bonus holds for every hit
// on the target, so the other hit that waits is compared again (ev.AlsoStopped) and the undo
// puts its status back.
func TestUndoKindReactionUsedStoppingOtherHits(t *testing.T) {
	t.Parallel()
	a := newCasters(t)
	e := a.castersFightNPCFirst(t)
	first := a.mustAttack(t, a.master, e, "Capitão Goblin", sword, "Pensantus", d20(9))
	if _, err := a.declineReaction(t, a.ana, e, first.GetPendingDamage().GetId()); err != nil { // the first hit waits for its damage
		t.Fatalf("DeclineReaction() error = %v", err)
	}
	second := a.mustAttack(t, a.master, e, "Capitão Goblin", sword, "Pensantus", d20(9))
	w := newUndoWatch(t, a)

	w.run(t, "Escudo with another hit waiting", nil, func() {
		if _, err := a.useReaction(t, a.ana, e, second.GetPendingDamage().GetId(), slotOfLevel(1)); err != nil {
			t.Fatalf("UseReaction() error = %v", err)
		}
	})
}

// TestUndoKindConditionsSetDismissedCreatures: the end of a concentration that dismissed the
// creatures of the spell (ev.Dismissed) brings them back.
func TestUndoKindConditionsSetDismissedCreatures(t *testing.T) {
	t.Parallel()
	a := newSummoners(t)
	e := a.toSalvia(t, a.summonersFight(t))
	if _, err := a.summonWolves(t, a.bia, e, 11); err != nil {
		t.Fatalf("CastSpell() error = %v", err)
	}
	w := newUndoWatch(t, a)

	w.run(t, "ending the concentration of Conjurar Animais", nil, func() {
		if _, err := a.conditions(t, a.bia, e, "Sálvia", nil, false, true); err != nil {
			t.Fatalf("SetCombatantConditions() error = %v", err)
		}
	})
}

// setupShapes starts the combat of the shapers in SETUP, where nobody has a turn to spend.
func (s *shapers) setupShapes(t *testing.T) *playv1.Encounter {
	t.Helper()
	return s.start(t, plan{
		npcs:     []*playv1.Participant{{CharacterId: s.capitao.GetId()}, {CharacterId: s.goblin.GetId()}},
		npcRolls: []int{1, 1},
		players:  map[string]int32{"Sálvia": 20, "Irmã": 15, "Pensantus": 12, "Toren": 10},
		reveal:   []string{"Capitão Goblin", "Goblin"},
		at: map[string][2]int32{
			"Sálvia": {6, 5}, "Irmã": {7, 5}, "Capitão Goblin": {5, 5}, "Goblin": {6, 6}, "Pensantus": {12, 2}, "Toren": {2, 2},
		},
		setup: true,
	})
}

// TestUndoKindWildShape: the druid becomes a wolf (an action, and a use of the resource),
// leaves the form on its own (a bonus action) or by the master's hand (free), with the beast's
// hit points kept for the undo.
func TestUndoKindWildShape(t *testing.T) {
	t.Parallel()
	s := newShapers(t)
	s.fight(t)
	w := newUndoWatch(t, s.armed)

	w.run(t, "Forma Selvagem", nil, func() { s.mustAssume(t, s.bia, s.bri, wolfKey) })
	s.mustAssume(t, s.bia, s.bri, wolfKey)
	s.correct(t, s.bri, func(r *playv1.AdjustCharacterVitalsRequest) { r.WildShapeHitPointsCurrent = new(int32(6)) })
	w.run(t, "Voltar à forma normal", nil, func() { s.mustLeave(t, s.bia, s.bri) })
	w.run(t, "Voltar à forma normal by the master", nil, func() { s.mustLeave(t, s.master, s.bri) })
}

// TestUndoKindWildShapeInSetup: in SETUP nobody has a turn, so the change spends no action
// (ev.Spent is false) and the undo only puts the form and the use back.
func TestUndoKindWildShapeInSetup(t *testing.T) {
	t.Parallel()
	s := newShapers(t)
	s.setupShapes(t)
	w := newUndoWatch(t, s.armed)

	w.run(t, "Forma Selvagem in SETUP", nil, func() { s.mustAssume(t, s.bia, s.bri, wolfKey) })
	s.mustAssume(t, s.bia, s.bri, wolfKey)
	w.run(t, "Voltar à forma normal in SETUP", nil, func() { s.mustLeave(t, s.bia, s.bri) })
}

// TestUndoKindWildShapeAfterARun: the change of form breaks the running start, which the undo
// puts back (RunBefore).
func TestUndoKindWildShapeAfterARun(t *testing.T) {
	t.Parallel()
	s := newShapers(t)
	e := s.fight(t)
	if _, err := s.bia.combat.MoveCombatant(t.Context(), connect.NewRequest(&playv1.MoveCombatantRequest{
		CampaignId: s.campaignID, EncounterId: e.GetId(), CombatantId: s.id(t, "Sálvia"), IdempotencyKey: newKey(), Col: 7, Row: 6,
	})); err != nil {
		t.Fatalf("MoveCombatant() error = %v", err)
	}
	s.skipOffers(t)
	var run int32
	if err := s.h.pool.QueryRow(t.Context(), "SELECT last_move_dft FROM combatants WHERE id = $1", s.id(t, "Sálvia")).Scan(&run); err != nil || run == 0 {
		t.Fatalf("the running start after the walk = %d, %v; want some", run, err)
	}
	w := newUndoWatch(t, s.armed)

	w.run(t, "Forma Selvagem after the walk", nil, func() { s.mustAssume(t, s.bia, s.bri, wolfKey) })
}

// TestUndoKindFamiliarSight: the sight through the familiar's eyes spends the action and
// gives the blinded and deafened conditions (ev.Spent, ev.SightConds).
func TestUndoKindFamiliarSight(t *testing.T) {
	t.Parallel()
	s := newShapers(t)
	s.nanquim(t)
	s.pensantusFirst(t, nil)
	w := newUndoWatch(t, s.armed)

	w.run(t, "the familiar's sight", nil, func() { s.mustSight(t, s.ana, s.pens, true) })
}

// TestUndoKindActionTakenExtraAction: a standard action taken with Haste's extra action gives
// the extra action back and leaves the action of the turn alone.
func TestUndoKindActionTakenExtraAction(t *testing.T) {
	t.Parallel()
	a := newCasters(t)
	e := a.closeFight(t)
	a.mustAddEffect(t, e, "spell:haste", []string{"Toren"}, a.rounds(t, 3, "Toren"))
	w := newUndoWatch(t, a)

	w.run(t, "Disparada with the extra action", nil, func() {
		if _, err := a.caio.combat.TakeAction(t.Context(), connect.NewRequest(&playv1.TakeActionRequest{
			CampaignId: a.campaignID, EncounterId: e.GetId(), CombatantId: a.id(t, "Toren"), ActionKey: "standard:dash", IdempotencyKey: newKey(), UseExtraAction: true,
		})); err != nil {
			t.Fatalf("TakeAction(dash, extra action) error = %v", err)
		}
	})
}

// TestUndoKindSpellCastByAnNPC: the master casts for an NPC wizard. The slot of the cast is
// the player's to spend (an NPC's cast counts none on the combatant), so the player-only slot
// branch of the undo has nothing to put back here.
func TestUndoKindSpellCastByAnNPC(t *testing.T) {
	t.Parallel()
	a := newArmedWith(t, func(a *armed) {
		a.toren = a.caio.hero(t, a.campaignID, "Toren", "class:fighter", "race:human", 5,
			&rulesv1.AbilityScores{Strength: 16, Dexterity: 13, Constitution: 14, Intelligence: 10, Wisdom: 10, Charisma: 8}, []string{battleaxe}, nil)
		a.pens = a.ana.caster(t, a.campaignID, "Pensantus", "class:wizard", "race:gnome", 5,
			&rulesv1.AbilityScores{Strength: 10, Dexterity: 14, Constitution: 12, Intelligence: 16, Wisdom: 10, Charisma: 8}, nil, []string{fireBolt},
			[]string{magicMissileSpell}, []string{magicMissileSpell})
		a.bri = a.bia.caster(t, a.campaignID, "Brisa", "class:cleric", "race:human", 3,
			&rulesv1.AbilityScores{Strength: 10, Dexterity: 16, Constitution: 14, Intelligence: 10, Wisdom: 16, Charisma: 8}, []string{maceKey}, []string{sacredFlame}, nil,
			[]string{cureWounds})
	})
	id := a.master.bossCaster(t, a.campaignID, "Mago Sombrio", 7, []string{fireBolt}, []string{magicMissileSpell}, []string{magicMissileSpell})
	e := a.start(t, plan{
		npcs: []*playv1.Participant{{CharacterId: id}}, npcRolls: []int{20}, reveal: []string{"Mago Sombrio"},
		at:      map[string][2]int32{"Mago Sombrio": {2, 5}, "Pensantus": {5, 5}, "Toren": {6, 5}, "Brisa": {5, 6}},
		players: map[string]int32{"Pensantus": 10, "Toren": 9, "Brisa": 8},
	})
	w := newUndoWatch(t, a)

	w.run(t, "the Mago Sombrio's Mísseis Mágicos", nil, func() {
		a.mustCast(t, a.master, e, "Mago Sombrio", magicMissileSpell, slotOfLevel(1), []*playv1.SpellTarget{darts(a, t, "Toren", 3)}, noCastRoll)
	})
}

// TestUndoKindActionTakenSlowFall: the monk's Slow Fall takes points off the fall
// damage waiting for the windows, spends the reaction and a use, and the undo puts the
// damage back (ev.FallCuts).
func TestUndoKindActionTakenSlowFall(t *testing.T) {
	t.Parallel()
	r, e := monkFall(t)
	w := newUndoWatch(t, r.armed)

	w.run(t, "Slow Fall", nil, func() {
		if _, err := r.action(t, r.caio, e, "Toren", "feature:slow-fall"); err != nil {
			t.Fatalf("TakeAction(Slow Fall) error = %v", err)
		}
	})
}
