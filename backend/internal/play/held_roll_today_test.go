package play

import (
	"testing"

	"connectrpc.com/connect"

	playv1 "github.com/PuraFome/meuRPG/backend/gen/meurpg/play/v1"
)

// The creatures' tests freeze a rule the unified gate has to keep. The four tests named "...Today"
// freeze what the two hold tables do only because of the way they were written (the master's side
// of the held rolls); E4-7 flips them on purpose, when the master gets to answer every question,
// and the PR that flips one flips only that test.

// giftedWolfBite is the monster the master gives Brisa, a wolf called Presa, as the creature of her character.
const giftedWolfBite = "monster:wolf#bite"

// giftedFight starts a combat in a theatre of the mind (no reach, no disadvantage) of Brisa, the cleric of
// Bia's, and Presa, the wolf the master gave her: the creature carries Brisa's character_id. The d20 given is
// Presa's initiative, so that she plays before Brisa (20) or after her (1).
func giftedFight(t *testing.T, wolfFace int32) (*armed, *playv1.Encounter) {
	t.Helper()
	a := newCasters(t)
	a.give(t, a.bri, "monster:wolf", "Presa")
	e := a.start(t, plan{
		npcs:     []*playv1.Participant{{CharacterId: a.goblin.GetId()}},
		npcRolls: []int{2},
		players:  map[string]int32{"Toren": 12, "Pensantus": 10, "Brisa": 15},
		reveal:   []string{"Goblin"},
		theatre:  true,
		setup:    true,
	})
	if _, err := a.submitFor(t, a.bia, e, "Presa", wolfFace); err != nil {
		t.Fatalf("SubmitInitiative(Presa) error = %v", err)
	}
	begun, err := a.master.combat.BeginCombat(t.Context(), connect.NewRequest(&playv1.BeginCombatRequest{CampaignId: a.campaignID, EncounterId: e.GetId(), IdempotencyKey: newKey()}))
	if err != nil {
		t.Fatalf("BeginCombat() error = %v", err)
	}
	return a, begun.Msg.GetEncounter()
}

// holdWolfBite has the wolf bite the goblin with a die in hand: the attack waits in roll_holds.
func (a *armed) holdWolfBite(t *testing.T, e *playv1.Encounter) *playv1.RollAttackResponse {
	t.Helper()
	a.passTo(t, e, "Presa")
	a.giveCombatantDie(t, "Presa", a.get(t, a.master))
	held, err := a.attack(t, a.bia, a.get(t, a.master), "Presa", giftedWolfBite, "Goblin", d20(9))
	if err != nil || held.GetInspirationOffer() == nil {
		t.Fatalf("the wolf's bite = %v, %v; want the question about its die", held, err)
	}
	return held
}

// holdGuidingBolt has Brisa cast Guiding Bolt with a die in hand: the cast waits in inspiration_holds.
func (a *armed) holdGuidingBolt(t *testing.T, e *playv1.Encounter) *playv1.CastSpellResponse {
	t.Helper()
	a.passTo(t, e, "Brisa")
	a.giveCombatantDie(t, "Brisa", a.get(t, a.master))
	held, err := a.cast(t, a.bia, a.get(t, a.master), "Brisa", guidingBolt, slotOfLevel(1), a.at(t, "Goblin"),
		func(r *playv1.CastSpellRequest) { r.Roll = &playv1.CastSpellRequest_D20Face{D20Face: 9} })
	if err != nil || held.GetInspirationOffer() == nil {
		t.Fatalf("Brisa's Guiding Bolt = %v, %v; want the question about her die", held, err)
	}
	return held
}

// answerWolf answers the wolf's question, keeping the die.
func (a *armed) answerWolf(t *testing.T, e *playv1.Encounter, wolf *playv1.RollAttackResponse) {
	t.Helper()
	bite, err := a.answer(t, a.bia, e, wolf.GetInspirationOffer().GetHoldId(), keepTheDie)
	if err != nil || bite.GetAttack().GetRoll().GetD20().GetFaces()[0] != 9 {
		t.Errorf("the wolf's answer = %v, %v; want its bite settled on the d20 of 9", bite.GetAttack().GetRoll(), err)
	}
}

// answerBolt answers Brisa's question, keeping the die.
func (a *armed) answerBolt(t *testing.T, bolt *playv1.CastSpellResponse) {
	t.Helper()
	cast, err := a.answerOutside(t, a.bia, bolt.GetInspirationOffer().GetHoldId(), false, false, 0)
	if err != nil || cast.GetSpellCast().GetCast().GetTargets()[0].GetAttackRoll().GetFaces()[0] != 9 {
		t.Errorf("Brisa's answer = %v, %v; want her Guiding Bolt settled on the d20 of 9", cast.GetSpellCast().GetCast(), err)
	}
}

// wantTwoQuestions checks that both questions are open at once: the wolf's on its combatant and Brisa's
// on her player's list. The answer of the earlier turn's is not asked here: a roll is answered on its
// own combatant's turn.
func (a *armed) wantTwoQuestions(t *testing.T, e *playv1.Encounter) {
	t.Helper()
	if n := countRows(t, a.h, countRollHolds, e.GetId()); n != 1 {
		t.Errorf("%d held attacks, want the wolf's", n)
	}
	if n := countRows(t, a.h, countInspirationHold, a.campaignID); n != 1 {
		t.Errorf("%d held rolls of the owner, want her cast", n)
	}
	if got := a.outsideState(t, a.bia).GetOffers(); len(got) != 1 {
		t.Errorf("Brisa's player reads %d questions of hers, want 1", len(got))
	}
}

// TestACreaturesHeldAttackDoesNotBlockItsOwner: a creature carries its owner's character_id, but it is
// a combatant of its own and not "backed by" the character: the wolf's bite that waits for its die,
// in roll_holds, does not stop Brisa's own roll in inspiration_holds, and both questions are open at once.
func TestACreaturesHeldAttackDoesNotBlockItsOwner(t *testing.T) {
	t.Parallel()
	a, e := giftedFight(t, 20) // the wolf plays before her
	wolf := a.holdWolfBite(t, e)
	bolt := a.holdGuidingBolt(t, e)
	a.wantTwoQuestions(t, e)
	a.answerBolt(t, bolt)
	if c := byLabel(t, a.get(t, a.master), "Presa"); c.GetInspirationOffer().GetHoldId() != wolf.GetInspirationOffer().GetHoldId() {
		t.Errorf("the wolf's question = %v after Brisa's answer, want it still open", c.GetInspirationOffer())
	}
}

// TestACreaturesHeldAttackDoesNotBlockItsOwnerReverse is the other way round: Brisa's cast waits
// in inspiration_holds (her turn comes first), and the wolf's bite is not refused for it.
func TestACreaturesHeldAttackDoesNotBlockItsOwnerReverse(t *testing.T) {
	t.Parallel()
	a, e := giftedFight(t, 1) // the wolf plays after her
	bolt := a.holdGuidingBolt(t, e)
	wolf := a.holdWolfBite(t, e)
	a.wantTwoQuestions(t, e)
	a.answerWolf(t, e, wolf)
	if got := a.outsideState(t, a.bia).GetOffers(); len(got) != 1 || got[0].GetHoldId() != bolt.GetInspirationOffer().GetHoldId() {
		t.Errorf("Brisa's player reads %v after the wolf's answer, want her question still open", got)
	}
}

// TestTheMasterCannotAnswerAnOutsideHoldToday: the question of a player's roll that waits in
// inspiration_holds (the scene check, the group check, the trap search, the hint try, a spell attack and
// the saves in a combat) is the player's alone: the master's answer, to the die and to Lucky, is refused
// with permission_denied, and the player's own goes through. A weapon attack's question the master
// answers (TestTheLuckyQuestionIsAnsweredByItsPlayerOrTheMaster). FROZEN: E4-7 lets the master answer
// every question; this test flips then.
func TestTheMasterCannotAnswerAnOutsideHoldToday(t *testing.T) {
	t.Parallel()
	a := newResourceTable(t)
	dalila, dalilaID := joinDalila(t, a.h, a.master, a.campaignID)
	point, actions := a.h.newScene(a.mapID, "O muro do pátio", true, 3, sceneSpec{key: "skill:athletics", dc: new(int32(25))})
	a.openScene(t, point)
	if _, err := a.giveOutside(t, a.bia, dalilaID); err != nil {
		t.Fatalf("GiveBardicInspirationOutside() error = %v", err)
	}
	// The die's question (a 14): the master's answer is refused, the player's goes through.
	held, err := a.rollSceneTyped(t, dalila, actions[0], newKey(), 14)
	if err != nil || held.GetInspirationOffer() == nil {
		t.Fatalf("RollSceneCheck() = %v, %v; want the die's question", held, err)
	}
	_, err = a.answerOutside(t, a.master, held.GetInspirationOffer().GetHoldId(), false, false, 0)
	wantCode(t, "the master's answer to the die", err, connect.CodePermissionDenied)
	if _, err := a.answerOutside(t, dalila, held.GetInspirationOffer().GetHoldId(), false, false, 0); err != nil {
		t.Errorf("the player's answer to the die: error = %v", err)
	}
	// Lucky's question (a 1, with the die still in her hand): the master's answer is refused too.
	lucky, err := a.rollSceneTyped(t, dalila, actions[0], newKey(), 1)
	if err != nil || lucky.GetLuckyOffer() == nil {
		t.Fatalf("RollSceneCheck() = %v, %v; want Lucky's question", lucky, err)
	}
	_, err = answerOutsideLucky(t, a.master, a.campaignID, lucky.GetLuckyOffer().GetHoldId(), outsideRerollTyped(15))
	wantCode(t, "the master's answer to Lucky", err, connect.CodePermissionDenied)
	if _, err := answerOutsideLucky(t, dalila, a.campaignID, lucky.GetLuckyOffer().GetHoldId(), outsideRerollTyped(15)); err != nil {
		t.Errorf("the player's answer to Lucky: error = %v", err)
	}
}

// TestAnOutsideHoldInACombatIsNotInTheCombatViewToday: a spell attack that waits for its die in a combat
// (inspiration_holds) is not on the combatant, for the player or the master: the combat view carries
// the questions of roll_holds only (a weapon attack's, which the master reads too); the question of
// the spell attack reaches the player through GetOutsideInspiration. FROZEN: E4-7 unifies the view.
func TestAnOutsideHoldInACombatIsNotInTheCombatViewToday(t *testing.T) {
	t.Parallel()
	a, e := brisaFight(t, true)
	a.h.roller.queue(4, 13)
	held, err := a.castGuidingBolt(t, e)
	if err != nil || held.GetInspirationOffer() == nil {
		t.Fatalf("CastSpell() = %v, %v; want the die's question", held, err)
	}
	for name, u := range map[string]*user{"Bia": a.bia, "master": a.master} {
		if c := byLabel(t, a.get(t, u), "Brisa"); c.GetInspirationOffer() != nil || c.GetLuckyOffer() != nil {
			t.Errorf("%s reads %v and %v on Brisa, want no question in the combat view", name, c.GetInspirationOffer(), c.GetLuckyOffer())
		}
	}
	if got := a.outsideState(t, a.bia).GetOffers(); len(got) != 1 || got[0].GetHoldId() != held.GetInspirationOffer().GetHoldId() {
		t.Errorf("Bia reads the offers %v, want the question %s", got, held.GetInspirationOffer().GetHoldId())
	}
}

// TestAnOutsideHoldInACombatDoesNotTouchTheEncounterToday: a question in roll_holds moves the combat's
// revision (the player and the master read the combat again to see it); a question in inspiration_holds,
// in the same combat, does not. FROZEN: E4-7 moves the revision for every question.
func TestAnOutsideHoldInACombatDoesNotTouchTheEncounterToday(t *testing.T) {
	t.Parallel()
	t.Run("weapon attack", func(t *testing.T) {
		t.Parallel()
		a, e := brisaFight(t, true)
		a.h.roller.queue(4)
		before := a.get(t, a.master).GetRevision()
		if _, err := a.attack(t, a.bia, e, "Brisa", maceKey, "Goblin", inAppRoll); err != nil {
			t.Fatalf("RollAttack() error = %v", err)
		}
		if after := a.get(t, a.master).GetRevision(); after != before+1 {
			t.Errorf("the revision went from %d to %d, want +1 for the question", before, after)
		}
	})
	t.Run("spell attack", func(t *testing.T) {
		t.Parallel()
		a, e := brisaFight(t, true)
		a.h.roller.queue(4, 13)
		before := a.get(t, a.master).GetRevision()
		if _, err := a.castGuidingBolt(t, e); err != nil {
			t.Fatalf("CastSpell() error = %v", err)
		}
		if after := a.get(t, a.master).GetRevision(); after != before {
			t.Errorf("the revision went from %d to %d, want it still for the question", before, after)
		}
	})
}

// TestTheMastersListHasNoOffersToday: GetOutsideInspiration, the list of the held rolls out of the combat
// view, answers the master the dice the characters hold and no question at all, whoever's roll waits
// (the player reads their own). FROZEN: E4-7 shows the master every question.
func TestTheMastersListHasNoOffersToday(t *testing.T) {
	t.Parallel()
	a := newResourceTable(t)
	dalila, dalilaID := joinDalila(t, a.h, a.master, a.campaignID)
	point, actions := a.h.newScene(a.mapID, "O muro do pátio", true, 3, sceneSpec{key: "skill:athletics", dc: new(int32(25))})
	a.openScene(t, point)
	if _, err := a.giveOutside(t, a.bia, dalilaID); err != nil {
		t.Fatalf("GiveBardicInspirationOutside() error = %v", err)
	}
	held, err := a.rollSceneTyped(t, dalila, actions[0], newKey(), 14)
	if err != nil || held.GetInspirationOffer() == nil {
		t.Fatalf("RollSceneCheck() = %v, %v; want the die's question", held, err)
	}
	if got := a.outsideState(t, dalila); len(got.GetOffers()) != 1 {
		t.Errorf("the player reads %d offers, want her question", len(got.GetOffers()))
	}
	got := a.outsideState(t, a.master)
	if len(got.GetOffers()) != 0 || len(got.GetLuckyOffers()) != 0 || len(got.GetHeld()) != 1 {
		t.Errorf("the master reads %d offers, %d Lucky offers and %d dice held, want no question and Dalila's die", len(got.GetOffers()), len(got.GetLuckyOffers()), len(got.GetHeld()))
	}
}
