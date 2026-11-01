package play

import (
	"testing"

	"connectrpc.com/connect"

	charactersv1 "github.com/PuraFome/meuRPG/backend/gen/meurpg/characters/v1"
	playv1 "github.com/PuraFome/meuRPG/backend/gen/meurpg/play/v1"
	rulesv1 "github.com/PuraFome/meuRPG/backend/gen/meurpg/rules/v1"
)

// Stroke of Luck (SRD 5.1, Rogue 20) turns an attack that missed into a hit, or counts the d20 of a failed ability check
// as a 20. The roll is held only for a rogue with the feature and a use left; the server works out whether it missed or
// failed, and the table's DC decides what the roller is told. The fixture is Pipo, a rogue of level 20 (Caio's, a rapier
// at +9: Dexterity +3 and proficiency +6), Toren, a fighter (Ana's), and Orla, a bard (Bia's), with a goblin (AC 12).

// strokeTable is the party of this file; race is Pipo's ("" for a human), level his rogue level.
func strokeTable(t *testing.T, race string, level int32) *armed {
	t.Helper()
	if race == "" {
		race = "race:human"
	}
	rogue := &rulesv1.AbilityScores{Strength: 10, Dexterity: 16, Constitution: 14, Intelligence: 10, Wisdom: 10, Charisma: 10}
	fighter := &rulesv1.AbilityScores{Strength: 16, Dexterity: 12, Constitution: 14, Intelligence: 10, Wisdom: 10, Charisma: 10}
	return newArmedWith(t, func(a *armed) {
		a.toren = a.caio.hero(t, a.campaignID, "Pipo", "class:rogue", race, level, rogue, []string{rapier}, nil)
		a.pens = a.ana.hero(t, a.campaignID, "Toren", "class:fighter", "race:human", 2, fighter, []string{battleaxe}, nil)
		a.bri = a.bia.hero(t, a.campaignID, "Orla", "class:bard", "race:human", 5,
			&rulesv1.AbilityScores{Strength: 8, Dexterity: 14, Constitution: 14, Intelligence: 10, Wisdom: 10, Charisma: 16}, nil, nil)
	})
}

// strokeFight starts the combat with Pipo first, the goblin below him.
func (a *armed) strokeFight(t *testing.T) *playv1.Encounter {
	t.Helper()
	return a.start(t, plan{
		npcs: []*playv1.Participant{{CharacterId: a.goblin.GetId()}}, npcRolls: []int{3},
		players: map[string]int32{"Pipo": 20, "Toren": 14, "Orla": 10}, reveal: []string{"Goblin"},
		at: map[string][2]int32{"Pipo": {4, 3}, "Toren": {3, 4}, "Orla": {5, 3}, "Goblin": {4, 4}},
	})
}

// strokeAnswer uses Stroke of Luck (true) or keeps the roll.
func strokeAnswer(use bool) func(*playv1.AnswerHeldRollRequest) {
	return func(r *playv1.AnswerHeldRollRequest) {
		r.Answer = &playv1.AnswerHeldRollRequest_StrokeOfLuck{StrokeOfLuck: &playv1.StrokeOfLuckAnswer{Use: use}}
	}
}

func (a *armed) strokeUsed(t *testing.T) int32 {
	t.Helper()
	used, total := resourceUsed(a.vitals(t, a.toren), "stroke_of_luck")
	if total != 1 {
		t.Fatalf("stroke_of_luck = %d of %d, want a rogue of level 20 with 1 use", used, total)
	}
	return used
}

// wantStroke checks the question is Stroke of Luck's, and whether it says the roll failed.
func wantStroke(t *testing.T, what string, offer *playv1.HeldRollOffer, kind playv1.HeldRollKind, failed bool) {
	t.Helper()
	if offer.GetStrokeOfLuck() == nil || offer.GetStrokeOfLuck().GetFailed() != failed || offer.GetKind() != kind {
		t.Fatalf("%s = %v; want Stroke of Luck's question (kind %v, failed %v)", what, offer, kind, failed)
	}
}

// A weapon attack that misses is held with the question; the armor class is never told. Using the feature turns the miss
// into an ordinary hit (not a critical one): the damage follows the hit path, and the use is spent.
func TestARogueWhoMissesIsAskedAndTheMissBecomesAHit(t *testing.T) {
	t.Parallel()
	a := strokeTable(t, "", 20)
	e := a.strokeFight(t)
	res, err := a.attack(t, a.caio, e, "Pipo", rapier, "Goblin", d20(2))
	if err != nil {
		t.Fatalf("RollAttack(2) error = %v", err)
	}
	offer := res.GetHeldRollOffer()
	wantStroke(t, "the held attack", offer, playv1.HeldRollKind_HELD_ROLL_KIND_WEAPON_ATTACK, true)
	if res.GetRoll().GetOutcome() != playv1.AttackOutcome_ATTACK_OUTCOME_UNSPECIFIED || res.GetPendingDamage() != nil || offer.GetD20().GetFaces()[0] != 2 ||
		offer.GetAttackKey() != rapier || offer.GetTargetId() != a.id(t, "Goblin") {
		t.Fatalf("the held attack = %v; want the 2 on the goblin, no result and no damage", res)
	}
	if c := byLabel(t, a.get(t, a.caio), "Pipo"); c.GetActionUsed() || c.GetHeldRollOffer().GetHoldId() != offer.GetHoldId() {
		t.Errorf("Pipo = action used %v, question %v; want the action kept and the question on him", c.GetActionUsed(), c.GetHeldRollOffer())
	}
	if c := byLabel(t, a.get(t, a.ana), "Pipo"); c.GetHeldRollOffer() != nil {
		t.Errorf("another player reads the question %v, want none", c.GetHeldRollOffer())
	}
	if list := a.listHeldRolls(t, a.ana); len(list) != 0 {
		t.Errorf("another player's list = %v, want nothing", list)
	}
	if used := a.strokeUsed(t); used != 0 {
		t.Errorf("a use was spent (%d) before the answer", used)
	}
	if _, err := a.answerHeldRoll(t, a.ana, offer.GetHoldId(), strokeAnswer(true)); err == nil {
		t.Error("another player answered the question")
	}
	out, err := a.answerHeldRoll(t, a.caio, offer.GetHoldId(), strokeAnswer(true))
	if err != nil {
		t.Fatalf("AnswerHeldRoll() error = %v", err)
	}
	r := out.GetAttack()
	if r.GetRoll().GetOutcome() != playv1.AttackOutcome_ATTACK_OUTCOME_HIT || r.GetPendingDamage() == nil || out.GetNext() != nil {
		t.Fatalf("the attack = outcome %v, pending %v; want an ordinary hit with damage to roll", r.GetRoll().GetOutcome(), r.GetPendingDamage())
	}
	if r.GetPendingDamage().GetCritical() {
		t.Error("the hit Stroke of Luck made is a critical hit; the d20 is not changed")
	}
	if rr := r.GetRoll().GetD20().GetRerolled(); r.GetRoll().GetD20().GetFaces()[0] != 2 || len(rr) != 1 || rr[0].GetSourceKey() != strokeSourceKey {
		t.Errorf("the d20 = %v, want the 2 unchanged, marked as Stroke of Luck's", r.GetRoll().GetD20())
	}
	if used := a.strokeUsed(t); used != 1 {
		t.Errorf("stroke_of_luck used = %d, want the one use spent", used)
	}
	if c := byLabel(t, r.GetEncounter(), "Pipo"); !c.GetActionUsed() || c.GetHeldRollOffer() != nil {
		t.Errorf("Pipo = action used %v, question %v; want the action spent and no question", c.GetActionUsed(), c.GetHeldRollOffer())
	}
}

// A natural 1 misses whatever the bonus, and the feature turns any miss into a hit (RAW).
func TestAStrokeOfLuckTurnsANaturalOneIntoAHit(t *testing.T) {
	t.Parallel()
	a := strokeTable(t, "", 20)
	e := a.strokeFight(t)
	res, err := a.attack(t, a.caio, e, "Pipo", rapier, "Goblin", d20(1))
	if err != nil {
		t.Fatalf("RollAttack(1) error = %v", err)
	}
	wantStroke(t, "the held 1", res.GetHeldRollOffer(), playv1.HeldRollKind_HELD_ROLL_KIND_WEAPON_ATTACK, true)
	out, err := a.answerHeldRoll(t, a.caio, res.GetHeldRollOffer().GetHoldId(), strokeAnswer(true))
	if err != nil || out.GetAttack().GetRoll().GetOutcome() != playv1.AttackOutcome_ATTACK_OUTCOME_HIT || out.GetAttack().GetPendingDamage() == nil {
		t.Errorf("the answer = %v, %v; want a hit with damage", out, err)
	}
}

// An attack that hits is not asked, and leaves no hold.
func TestAHitIsNotAskedAboutStrokeOfLuck(t *testing.T) {
	t.Parallel()
	a := strokeTable(t, "", 20)
	e := a.strokeFight(t)
	res, err := a.attack(t, a.caio, e, "Pipo", rapier, "Goblin", d20(3)) // 3 + 9 = 12 reaches the goblin's AC
	if err != nil || res.GetHeldRollOffer() != nil || res.GetRoll().GetOutcome() != playv1.AttackOutcome_ATTACK_OUTCOME_HIT {
		t.Fatalf("RollAttack(3) = %v, %v; want the hit settled", res, err)
	}
	if list := a.listHeldRolls(t, a.master); len(list) != 0 {
		t.Errorf("holds = %v, want none", list)
	}
	if used := a.strokeUsed(t); used != 0 {
		t.Errorf("stroke_of_luck used = %d, want 0", used)
	}
}

// A rogue below the 20th level has no Stroke of Luck, and one with no use left asks nothing.
func TestNoFeatureOrNoUseLeftNoQuestion(t *testing.T) {
	t.Parallel()
	a := strokeTable(t, "", 19)
	e := a.strokeFight(t)
	res, err := a.attack(t, a.caio, e, "Pipo", rapier, "Goblin", d20(2))
	if err != nil || res.GetHeldRollOffer() != nil || res.GetRoll().GetOutcome() != playv1.AttackOutcome_ATTACK_OUTCOME_MISS {
		t.Errorf("RollAttack(2) by a rogue of 19 = %v, %v; want the miss settled", res, err)
	}
	b := strokeTable(t, "", 20)
	f := b.strokeFight(t)
	b.correct(t, b.toren, func(r *playv1.AdjustCharacterVitalsRequest) {
		r.ResourcesUsed = []*playv1.ResourceUsed{{Key: "stroke_of_luck", Used: 1}}
	})
	res, err = b.attack(t, b.caio, f, "Pipo", rapier, "Goblin", d20(2))
	if err != nil || res.GetHeldRollOffer() != nil || res.GetRoll().GetOutcome() != playv1.AttackOutcome_ATTACK_OUTCOME_MISS {
		t.Errorf("RollAttack(2) with no use = %v, %v; want the miss settled", res, err)
	}
	// Another class attacks as it always did.
	g := strokeTable(t, "", 20)
	h := g.strokeFight(t)
	g.passTo(t, h, "Toren")
	res, err = g.attack(t, g.ana, h, "Toren", battleaxe, "Goblin", d20(2))
	if err != nil || res.GetHeldRollOffer() != nil {
		t.Errorf("a fighter's RollAttack(2) = %v, %v; want it settled", res, err)
	}
}

// Keeping the miss spends nothing and settles the miss.
func TestKeepingTheMissSpendsNoUse(t *testing.T) {
	t.Parallel()
	a := strokeTable(t, "", 20)
	e := a.strokeFight(t)
	res, err := a.attack(t, a.caio, e, "Pipo", rapier, "Goblin", d20(2))
	if err != nil {
		t.Fatalf("RollAttack(2) error = %v", err)
	}
	out, err := a.answerHeldRoll(t, a.caio, res.GetHeldRollOffer().GetHoldId(), strokeAnswer(false))
	if err != nil {
		t.Fatalf("AnswerHeldRoll() error = %v", err)
	}
	r := out.GetAttack()
	if r.GetRoll().GetOutcome() != playv1.AttackOutcome_ATTACK_OUTCOME_MISS || r.GetPendingDamage() != nil || len(r.GetRoll().GetD20().GetRerolled()) != 0 {
		t.Errorf("the attack = %v, want the miss kept", r)
	}
	if used := a.strokeUsed(t); used != 0 {
		t.Errorf("stroke_of_luck used = %d, want 0", used)
	}
	if cur, _, _ := a.hp(t, "Goblin"); cur != 7 {
		t.Errorf("the goblin has %d hit points, want 7", cur)
	}
}

// One answer per question: the same answer again gives the same result and spends nothing more; another answer is refused.
func TestAStrokeOfLuckQuestionHasOneAnswer(t *testing.T) {
	t.Parallel()
	a := strokeTable(t, "", 20)
	e := a.strokeFight(t)
	res, err := a.attack(t, a.caio, e, "Pipo", rapier, "Goblin", d20(2))
	if err != nil {
		t.Fatalf("RollAttack(2) error = %v", err)
	}
	hold := res.GetHeldRollOffer().GetHoldId()
	req := &playv1.AnswerHeldRollRequest{CampaignId: a.campaignID, HoldId: hold, IdempotencyKey: newKey()}
	strokeAnswer(true)(req)
	first, err := a.caio.resource.AnswerHeldRoll(t.Context(), connect.NewRequest(req))
	if err != nil {
		t.Fatalf("AnswerHeldRoll() error = %v", err)
	}
	again, err := a.caio.resource.AnswerHeldRoll(t.Context(), connect.NewRequest(req))
	if err != nil || again.Msg.GetAttack().GetRoll().GetOutcome() != first.Msg.GetAttack().GetRoll().GetOutcome() {
		t.Errorf("the same answer again = %v, %v; want the same hit", again, err)
	}
	if used := a.strokeUsed(t); used != 1 {
		t.Errorf("stroke_of_luck used = %d after a retry, want 1", used)
	}
	_, err = a.answerHeldRoll(t, a.caio, hold, strokeAnswer(false))
	wantCode(t, "another answer", err, connect.CodeFailedPrecondition)
	if used := a.strokeUsed(t); used != 1 {
		t.Errorf("stroke_of_luck used = %d after the second answer, want 1", used)
	}
}

// strokeLine tells whether u reads, in the combat log, an attack that Stroke of Luck turned into a hit.
func (a *armed) strokeLine(t *testing.T, u *user, e *playv1.Encounter) bool {
	t.Helper()
	for _, l := range logEntries(a.log(t, u, e)) {
		for _, r := range l.GetAttackRoll().GetRerolled() {
			if r.GetSourceKey() == strokeSourceKey {
				return true
			}
		}
	}
	return false
}

// The log says "Golpe de Sorte" to the roller's player and to the master (RN-10), to nobody else; the master's undo of the
// attack gives the use back, and the question is not asked again by the undone roll.
func TestTheLogSaysStrokeOfLuckToTheOwnerAndTheMasterOnlyAndUndoGivesTheUseBack(t *testing.T) {
	t.Parallel()
	a := strokeTable(t, "", 20)
	e := a.strokeFight(t)
	res, err := a.attack(t, a.caio, e, "Pipo", rapier, "Goblin", d20(2))
	if err != nil {
		t.Fatalf("RollAttack(2) error = %v", err)
	}
	if _, err := a.answerHeldRoll(t, a.caio, res.GetHeldRollOffer().GetHoldId(), strokeAnswer(true)); err != nil {
		t.Fatalf("AnswerHeldRoll() error = %v", err)
	}
	for name, u := range map[string]*user{"the owner": a.caio, "the master": a.master} {
		if !a.strokeLine(t, u, e) {
			t.Errorf("%s reads no Stroke of Luck in the log", name)
		}
	}
	for name, u := range map[string]*user{"Toren's player": a.ana, "Orla's player": a.bia} {
		if a.strokeLine(t, u, e) || a.logSays(t, u, e, "Golpe de Sorte") {
			t.Errorf("%s reads Stroke of Luck in the log", name)
		}
	}
	if used := a.strokeUsed(t); used != 1 {
		t.Fatalf("stroke_of_luck used = %d, want 1", used)
	}
	l := a.log(t, a.master, e)
	if err := a.undo(t, a.master, e, l.GetUndoableEventId()); err != nil {
		t.Fatalf("UndoLastAction() error = %v", err)
	}
	if used := a.strokeUsed(t); used != 0 {
		t.Errorf("stroke_of_luck used = %d after the undo, want the use back", used)
	}
	if a.strokeLine(t, a.master, e) {
		t.Error("the undone attack is still in the log")
	}
}

// A short rest gives the use back (it recovers on a short or a long rest).
func TestAShortRestGivesStrokeOfLuckBack(t *testing.T) {
	t.Parallel()
	a := strokeTable(t, "", 20)
	e := a.strokeFight(t)
	res, err := a.attack(t, a.caio, e, "Pipo", rapier, "Goblin", d20(2))
	if err != nil {
		t.Fatalf("RollAttack(2) error = %v", err)
	}
	if _, err := a.answerHeldRoll(t, a.caio, res.GetHeldRollOffer().GetHoldId(), strokeAnswer(true)); err != nil {
		t.Fatalf("AnswerHeldRoll() error = %v", err)
	}
	a.endEncounter(t, e)
	if _, err := a.master.resource.TakeRest(t.Context(), connect.NewRequest(&playv1.TakeRestRequest{CampaignId: a.campaignID, Kind: playv1.RestKind_REST_KIND_SHORT, IdempotencyKey: newKey()})); err != nil {
		t.Fatalf("TakeRest(short) error = %v", err)
	}
	if used := a.strokeUsed(t); used != 0 {
		t.Errorf("stroke_of_luck used = %d after a short rest, want 0", used)
	}
}

// An NPC's attack is never held, whoever rolls it.
func TestAnNPCMissIsNeverAskedAboutStrokeOfLuck(t *testing.T) {
	t.Parallel()
	a := strokeTable(t, "", 20)
	e := a.strokeFight(t)
	e = a.passTo(t, e, "Goblin")
	res, err := a.attack(t, a.master, e, "Goblin", sword, "Pipo", d20(1))
	if err != nil || res.GetHeldRollOffer() != nil || res.GetRoll().GetOutcome() != playv1.AttackOutcome_ATTACK_OUTCOME_MISS {
		t.Errorf("the goblin's RollAttack(1) = %v, %v; want the miss settled", res, err)
	}
}

// A halfling rogue: Lucky's 1 is rolled again first; if the new roll misses, Stroke of Luck asks about that one.
func TestLuckyComesBeforeStrokeOfLuck(t *testing.T) {
	t.Parallel()
	a := strokeTable(t, "race:halfling", 20)
	e := a.strokeFight(t)
	res, err := a.attack(t, a.caio, e, "Pipo", rapier, "Goblin", d20(1))
	if err != nil || res.GetHeldRollOffer().GetLucky() == nil {
		t.Fatalf("RollAttack(1) = %v, %v; want Lucky's question first", res, err)
	}
	out, err := a.answerHeldRoll(t, a.caio, res.GetHeldRollOffer().GetHoldId(), luckyAnswer(1))
	if err != nil || out.GetNext() == nil {
		t.Fatalf("Lucky's answer (1) = %v, %v; want Stroke of Luck's question on the new 1", out, err)
	}
	wantStroke(t, "the next question", out.GetNext(), playv1.HeldRollKind_HELD_ROLL_KIND_WEAPON_ATTACK, true)
	if out.GetAttack().GetRoll() != nil && out.GetAttack().GetRoll().GetOutcome() != playv1.AttackOutcome_ATTACK_OUTCOME_UNSPECIFIED {
		t.Errorf("the attack was settled before Stroke of Luck's answer: %v", out.GetAttack())
	}
	fin, err := a.answerHeldRoll(t, a.caio, out.GetNext().GetHoldId(), strokeAnswer(true))
	if err != nil || fin.GetAttack().GetRoll().GetOutcome() != playv1.AttackOutcome_ATTACK_OUTCOME_HIT || fin.GetNext() != nil {
		t.Errorf("Stroke of Luck's answer = %v, %v; want the hit, settled", fin, err)
	}
	if used := a.strokeUsed(t); used != 1 {
		t.Errorf("stroke_of_luck used = %d, want 1", used)
	}
}

// A scene check that fails: the question says so; the use counts the d20 as a 20, and the total is 20 plus the modifier.
// Typed dice (RN-18) work the same way. A check that passes is not asked, and the question has one answer.
func TestAFailedSceneCheckIsAskedAndTheUseCountsTheD20AsATwenty(t *testing.T) {
	t.Parallel()
	a := strokeTable(t, "", 20)
	point, actions := a.h.newScene(a.mapID, "O muro do pátio", true, 5, sceneSpec{key: "skill:athletics", dc: new(int32(15))}, sceneSpec{key: "skill:athletics", dc: new(int32(15))})
	a.openScene(t, point)

	// A pass: no question.
	ok, err := a.rollSceneTyped(t, a.caio, actions[1], newKey(), 20)
	if err != nil || ok.GetHeldRollOffer() != nil || ok.GetRoll() == nil {
		t.Fatalf("a passed check = %v, %v; want it written, with no question", ok, err)
	}
	// A failure: held, nothing written, the question says it failed.
	held, err := a.rollSceneTyped(t, a.caio, actions[0], newKey(), 2)
	if err != nil || held.GetRoll() != nil {
		t.Fatalf("a failed check = %v, %v; want it held", held, err)
	}
	wantStroke(t, "the failed check", held.GetHeldRollOffer(), playv1.HeldRollKind_HELD_ROLL_KIND_SCENE_CHECK, true)
	if got := a.listHeldRolls(t, a.ana); len(got) != 0 {
		t.Errorf("another player reads %v, want nothing", got)
	}
	out, err := a.answerHeldRoll(t, a.caio, held.GetHeldRollOffer().GetHoldId(), strokeAnswer(true))
	if err != nil {
		t.Fatalf("AnswerHeldRoll() error = %v", err)
	}
	r := out.GetSceneCheck().GetRoll()
	if r == nil || r.GetRoll().GetFaces()[0] != 20 && r.GetRoll().GetFaces()[r.GetRoll().GetCountedIndex()] != 20 {
		t.Fatalf("the roll = %v, want the d20 counted as 20", r)
	}
	if r.GetRoll().GetTotal() != 20+ok.GetRoll().GetRoll().GetModifier() || !r.GetPassed() {
		t.Errorf("the roll's total = %d, passed %v; want 20 plus the modifier and passed", r.GetRoll().GetTotal(), r.GetPassed())
	}
	if rr := r.GetRoll().GetRerolled(); len(rr) != 1 || rr[0].GetSourceKey() != strokeSourceKey || rr[0].GetFrom() != 2 || rr[0].GetTo() != 20 {
		t.Errorf("rerolled = %v, want Stroke of Luck: 2 -> 20", rr)
	}
	if used := a.strokeUsed(t); used != 1 {
		t.Errorf("stroke_of_luck used = %d, want 1", used)
	}
	if n := a.countSceneRolls(t); n != 2 {
		t.Errorf("%d scene rolls written, want 2 (the pass and the used failure)", n)
	}
	// The use is spent: the next failure is not asked.
	next, err := a.rollSceneTyped(t, a.caio, actions[0], newKey(), 2)
	if err != nil || next.GetHeldRollOffer() != nil || next.GetRoll() == nil || next.GetRoll().GetPassed() {
		t.Errorf("a failure with no use left = %v, %v; want it written as a failure", next, err)
	}
}

// Keeping the failed check writes it as it came and spends nothing.
func TestKeepingAFailedSceneCheckSpendsNoUse(t *testing.T) {
	t.Parallel()
	a := strokeTable(t, "", 20)
	point, actions := a.h.newScene(a.mapID, "O muro do pátio", true, 5, sceneSpec{key: "skill:athletics", dc: new(int32(15))})
	a.openScene(t, point)
	held, err := a.rollSceneTyped(t, a.caio, actions[0], newKey(), 2)
	if err != nil || held.GetHeldRollOffer().GetStrokeOfLuck() == nil {
		t.Fatalf("a failed check = %v, %v; want the question", held, err)
	}
	out, err := a.answerHeldRoll(t, a.caio, held.GetHeldRollOffer().GetHoldId(), strokeAnswer(false))
	if err != nil || out.GetSceneCheck().GetRoll().GetPassed() || len(out.GetSceneCheck().GetRoll().GetRoll().GetRerolled()) != 0 {
		t.Errorf("keeping = %v, %v; want the failure as it came", out, err)
	}
	if used := a.strokeUsed(t); used != 0 {
		t.Errorf("stroke_of_luck used = %d, want 0", used)
	}
}

// A scene that hides its DC: the rogue is asked on every roll, even a 19 that passes, and the question says nothing of the
// result (RN-20). Keeping the pass leaves the use.
func TestAHiddenDCAsksOnEveryCheckAndTellsNothing(t *testing.T) {
	t.Parallel()
	a := strokeTable(t, "", 20)
	point, actions := a.h.newScene(a.mapID, "O muro do pátio", false, 5, sceneSpec{key: "skill:athletics", dc: new(int32(10))})
	a.openScene(t, point)
	for _, face := range []int32{19, 2} {
		held, err := a.rollSceneTyped(t, a.caio, actions[0], newKey(), face)
		if err != nil {
			t.Fatalf("RollSceneCheck(%d) error = %v", face, err)
		}
		offer := held.GetHeldRollOffer()
		if offer.GetStrokeOfLuck() == nil || offer.GetStrokeOfLuck().GetFailed() || held.GetRoll() != nil {
			t.Fatalf("a roll of %d at a hidden DC = %v; want the question, with nothing said of the result", face, held)
		}
		if _, err := a.answerHeldRoll(t, a.caio, offer.GetHoldId(), strokeAnswer(false)); err != nil {
			t.Fatalf("AnswerHeldRoll() error = %v", err)
		}
	}
	if used := a.strokeUsed(t); used != 0 {
		t.Errorf("stroke_of_luck used = %d, want 0", used)
	}
}

// A check with no DC set has no failure to turn: it is never asked.
func TestACheckWithNoDCIsNotAskedAboutStrokeOfLuck(t *testing.T) {
	t.Parallel()
	a := strokeTable(t, "", 20)
	point, actions := a.h.newScene(a.mapID, "O muro do pátio", true, 5, sceneSpec{key: "skill:athletics"})
	a.openScene(t, point)
	res, err := a.rollSceneTyped(t, a.caio, actions[0], newKey(), 2)
	if err != nil || res.GetHeldRollOffer() != nil || res.GetRoll() == nil {
		t.Errorf("a roll with no DC = %v, %v; want it written", res, err)
	}
}

// strokeArenaTable is the party of the arena (combat_contests_test.go) with Toren a rogue of level 20 (Dexterity 16:
// Stealth +3, Cunning Action). With elf true he is a high elf with Fire Bolt, the one way a rogue of level 20 makes a
// spell attack: his levels leave no room for a spellcasting class.
func strokeArenaTable(t *testing.T, elf bool) *armed {
	t.Helper()
	return newArmedWith(t, func(a *armed) {
		sheet := &charactersv1.FullSheet{
			BaseScores: &rulesv1.AbilityScores{Strength: 12, Dexterity: 16, Constitution: 14, Intelligence: 16, Wisdom: 10, Charisma: 8},
			RaceKey:    "race:human", Classes: []*charactersv1.ClassLevel{{ClassKey: "class:rogue", Level: 20}}, WeaponKeys: []string{rapier},
		}
		if elf {
			sheet.RaceKey, sheet.SubraceKey, sheet.CantripKeys = "race:elf", "subrace:high-elf", []string{fireBolt}
		}
		res, err := a.caio.characters.CreateCharacter(t.Context(), connect.NewRequest(&charactersv1.CreateCharacterRequest{
			CampaignId: a.campaignID, Kind: charactersv1.CharacterKind_CHARACTER_KIND_PLAYER, Name: "Toren",
			Sheet: &charactersv1.CharacterSheet{Content: &charactersv1.CharacterSheet_Full{Full: sheet}},
		}))
		if err != nil {
			t.Fatalf("CreateCharacter(Toren) error = %v", err)
		}
		a.toren = res.Msg.GetCharacter()
		a.pens = a.ana.caster(t, a.campaignID, "Pensantus", "class:wizard", "race:gnome", 3,
			&rulesv1.AbilityScores{Strength: 10, Dexterity: 14, Constitution: 12, Intelligence: 16, Wisdom: 10, Charisma: 8}, nil, []string{fireBolt},
			[]string{magicMissileSpell}, []string{magicMissileSpell})
		a.bri = a.bia.hero(t, a.campaignID, "Brisa", "class:fighter", "race:human", 2, scores16(), []string{rapier}, nil)
	})
}

// A spell attack (Fire Bolt, +9, against the goblin's armor class 12) that misses is held with the question; using
// the feature makes it an ordinary hit that wounds the goblin and spends the use; the undo of the cast gives it back.
func TestASpellAttackThatMissesAsksStrokeOfLuckAndTheUndoGivesTheUseBack(t *testing.T) {
	t.Parallel()
	a := strokeArenaTable(t, true)
	c := a.arena(t, arenaPlan{at: map[string][2]int32{"Hobgoblin": {8, 5}}})
	res, err := a.cast(t, a.caio, c.e, "Toren", fireBolt, nil, a.at(t, "Goblin"), d20Face(2))
	if err != nil || res.GetCast() != nil {
		t.Fatalf("CastSpell(2) = %v, %v; want the question and no cast", res, err)
	}
	wantStroke(t, "the held spell attack", res.GetHeldRollOffer(), playv1.HeldRollKind_HELD_ROLL_KIND_SPELL_ATTACK, true)
	if byLabel(t, a.get(t, a.caio), "Toren").GetActionUsed() {
		t.Error("the action was spent before the answer")
	}
	out, err := a.answerHeldRoll(t, a.caio, res.GetHeldRollOffer().GetHoldId(), strokeAnswer(true))
	if err != nil {
		t.Fatalf("AnswerHeldRoll() error = %v", err)
	}
	targets := out.GetSpellCast().GetCast().GetTargets()
	if len(targets) != 1 || targets[0].GetOutcome() != playv1.AttackOutcome_ATTACK_OUTCOME_HIT || targets[0].GetPendingDamageId() == "" {
		t.Fatalf("the cast = %v, want a hit with damage to roll", out.GetSpellCast().GetCast())
	}
	if rr := targets[0].GetAttackRoll().GetRerolled(); targets[0].GetAttackRoll().GetFaces()[0] != 2 || len(rr) != 1 || rr[0].GetSourceKey() != strokeSourceKey {
		t.Errorf("the attack roll = %v, want the 2 unchanged, marked as Stroke of Luck's", targets[0].GetAttackRoll())
	}
	if used := a.strokeUsed(t); used != 1 {
		t.Fatalf("stroke_of_luck used = %d, want 1", used)
	}
	l := a.log(t, a.master, c.e)
	if err := a.undo(t, a.master, c.e, l.GetUndoableEventId()); err != nil {
		t.Fatalf("UndoLastAction() error = %v", err)
	}
	if used := a.strokeUsed(t); used != 0 {
		t.Errorf("stroke_of_luck used = %d after the undo of the cast, want the use back", used)
	}
}

// A spell attack that hits is not asked and spends nothing.
func TestASpellAttackThatHitsIsNotAskedAboutStrokeOfLuck(t *testing.T) {
	t.Parallel()
	a := strokeArenaTable(t, true)
	c := a.arena(t, arenaPlan{at: map[string][2]int32{"Hobgoblin": {8, 5}}})
	res, err := a.cast(t, a.caio, c.e, "Toren", fireBolt, nil, a.at(t, "Goblin"), d20Face(19))
	if err != nil || res.GetHeldRollOffer() != nil || res.GetCast().GetTargets()[0].GetOutcome() != playv1.AttackOutcome_ATTACK_OUTCOME_HIT {
		t.Fatalf("CastSpell(3) = %v, %v; want the hit settled", res, err)
	}
	if used := a.strokeUsed(t); used != 0 {
		t.Errorf("stroke_of_luck used = %d, want 0", used)
	}
}

// A group check whose DC is hidden asks on every roll and says nothing of the result; the answer counts the d20 as 20
// and the roll stands for the member. With the DC shown, only a failure asks and says so.
func TestAGroupCheckAsksStrokeOfLuckBlindWithAHiddenDCAndTellsWithAShownOne(t *testing.T) {
	t.Parallel()
	a := strokeTable(t, "", 20)
	hidden := a.mustRequestGroup(t, func(r *playv1.RequestGroupCheckRequest) { r.Dc = 15 })
	held, err := a.rollGroup(t, a.caio, hidden.GetId(), 19)
	if err != nil || held.GetGroupCheck() != nil {
		t.Fatalf("RollGroupCheck(19) = %v, %v; want the question alone", held, err)
	}
	wantStroke(t, "the hidden group check", held.GetHeldRollOffer(), playv1.HeldRollKind_HELD_ROLL_KIND_GROUP_CHECK, false)
	out, err := a.answerHeldRoll(t, a.caio, held.GetHeldRollOffer().GetHoldId(), strokeAnswer(false))
	if err != nil || out.GetGroupCheck() == nil {
		t.Fatalf("keeping the 19 = %v, %v; want the roll written", out, err)
	}
	if used := a.strokeUsed(t); used != 0 {
		t.Errorf("stroke_of_luck used = %d after keeping, want 0", used)
	}
	a.mustCloseGroup(t, hidden.GetId())

	shown := a.mustRequestGroup(t, func(r *playv1.RequestGroupCheckRequest) { r.Dc, r.ShowDc = 25, true })
	held, err = a.rollGroup(t, a.caio, shown.GetId(), 2)
	if err != nil || held.GetGroupCheck() != nil {
		t.Fatalf("RollGroupCheck(2) = %v, %v; want the question alone", held, err)
	}
	wantStroke(t, "the failed group check", held.GetHeldRollOffer(), playv1.HeldRollKind_HELD_ROLL_KIND_GROUP_CHECK, true)
	out, err = a.answerHeldRoll(t, a.caio, held.GetHeldRollOffer().GetHoldId(), strokeAnswer(true))
	if err != nil {
		t.Fatalf("AnswerHeldRoll() error = %v", err)
	}
	roll := memberOf(out.GetGroupCheck().GetGroupCheck(), a.toren.GetId()).GetRoll()
	if roll.GetFaces()[0] != 20 || roll.GetTotal() != 20+roll.GetModifier() {
		t.Errorf("the member's roll = %v, want the d20 counted as 20 plus the modifier", roll)
	}
	if rr := roll.GetRerolled(); len(rr) != 1 || rr[0].GetSourceKey() != strokeSourceKey || rr[0].GetFrom() != 2 || rr[0].GetTo() != 20 {
		t.Errorf("rerolled = %v, want Stroke of Luck: 2 -> 20", rr)
	}
	if used := a.strokeUsed(t); used != 1 {
		t.Errorf("stroke_of_luck used = %d, want 1", used)
	}
	// The master reads the reroll; another player's view of the check has no roll of Pipo's at all.
	if r := memberOf(a.groupView(t, a.master), a.toren.GetId()).GetRoll(); len(r.GetRerolled()) != 1 || r.GetRerolled()[0].GetSourceKey() != strokeSourceKey {
		t.Errorf("the master reads %v, want the Stroke of Luck reroll", r)
	}
	if other := a.groupView(t, a.ana); memberOf(other, a.toren.GetId()) != nil {
		t.Errorf("another player reads Pipo's roll: %v", other)
	}
}

// A group check of a saving throw is not an ability check: the rogue is never asked about Stroke of Luck.
func TestAGroupSavingThrowDoesNotAskStrokeOfLuck(t *testing.T) {
	t.Parallel()
	a := strokeTable(t, "", 20)
	save := a.mustRequestGroup(t, func(r *playv1.RequestGroupCheckRequest) {
		r.SkillKey, r.Dc, r.ShowDc = "save:con", 25, true
	})
	out, err := a.rollGroup(t, a.caio, save.GetId(), 2)
	if err != nil || out.GetHeldRollOffer() != nil || out.GetGroupCheck() == nil {
		t.Fatalf("RollGroupCheck(2) = %v, %v; want the roll written with no question", out, err)
	}
	if used := a.strokeUsed(t); used != 0 {
		t.Errorf("stroke_of_luck used = %d, want 0", used)
	}
}

// The contest roll of a rogue is opposed, so the question comes blind on any d20 but a 20; using it counts the d20 as 20.
func TestAContestRollAsksStrokeOfLuckBlind(t *testing.T) {
	t.Parallel()
	a := strokeArenaTable(t, false)
	c := a.arena(t, arenaPlan{})
	c.h.roller.queue(15)
	held, err := c.startContest(t, a.caio, func(r *playv1.StartContestRequest) {
		r.InitiatorId, r.TargetId, r.Purpose = c.id(t, "Toren"), c.id(t, c.hob), playv1.ContestPurpose_CONTEST_PURPOSE_GRAPPLE
	})
	if err != nil || held.GetContest() != nil {
		t.Fatalf("StartContest() = %v, %v; want the question alone", held, err)
	}
	wantStroke(t, "the contest roll", held.GetHeldRollOffer(), playv1.HeldRollKind_HELD_ROLL_KIND_CONTEST, false)
	out, err := a.answerHeldRoll(t, a.caio, held.GetHeldRollOffer().GetHoldId(), strokeAnswer(true))
	if err != nil {
		t.Fatalf("AnswerHeldRoll() error = %v", err)
	}
	roll := out.GetContest().GetContest().GetInitiatorRoll()
	if roll.GetFaces()[0] != 20 || roll.GetTotal() != 20+roll.GetModifier() {
		t.Errorf("Toren's roll = %v, want the 15 counted as 20 plus the modifier", roll)
	}
	if used := a.strokeUsed(t); used != 1 {
		t.Errorf("stroke_of_luck used = %d, want 1", used)
	}
}

// Hide is opposed too: the Stealth d20 is asked blind, and using the feature counts it as 20.
func TestAHideRollAsksStrokeOfLuckBlind(t *testing.T) {
	t.Parallel()
	a := strokeArenaTable(t, false)
	c := a.arena(t, arenaPlan{})
	held, err := c.hide(t, a.caio, "Toren", cunningHide, 7)
	if err != nil || held.GetAttempt() != nil {
		t.Fatalf("Hide() = %v, %v; want the question alone", held, err)
	}
	wantStroke(t, "the Hide roll", held.GetHeldRollOffer(), playv1.HeldRollKind_HELD_ROLL_KIND_HIDE, false)
	out, err := a.answerHeldRoll(t, a.caio, held.GetHeldRollOffer().GetHoldId(), strokeAnswer(true))
	if err != nil {
		t.Fatalf("AnswerHeldRoll() error = %v", err)
	}
	roll := out.GetHide().GetAttempt().GetRoll()
	if roll.GetFaces()[0] != 20 || roll.GetTotal() != 20+roll.GetModifier() {
		t.Errorf("the Stealth roll = %v, want the 7 counted as 20 plus the modifier", roll)
	}
	if used := a.strokeUsed(t); used != 1 {
		t.Errorf("stroke_of_luck used = %d, want 1", used)
	}
}

// Dice an effect adds after the gate (Bless) move the total, so the attack question comes blind: it comes even on a
// roll that hits, and never says the attack failed.
func TestAnAttackWithBlessAsksStrokeOfLuckAndTellsNothing(t *testing.T) {
	t.Parallel()
	for _, face := range []int32{19, 2} {
		a := strokeTable(t, "", 20)
		e := a.strokeFight(t)
		a.mustAddEffect(t, e, "spell:bless", []string{"Pipo"}, a.rounds(t, 10, "Pipo"))
		res, err := a.attack(t, a.caio, e, "Pipo", rapier, "Goblin", func(r *playv1.RollAttackRequest) { d20(face)(r); r.ExtraDieFaces = []int32{3} })
		if err != nil {
			t.Fatalf("RollAttack(%d) error = %v", face, err)
		}
		wantStroke(t, "the attack with Bless", res.GetHeldRollOffer(), playv1.HeldRollKind_HELD_ROLL_KIND_WEAPON_ATTACK, false)
		if res.GetRoll().GetOutcome() != playv1.AttackOutcome_ATTACK_OUTCOME_UNSPECIFIED {
			t.Fatalf("the attack of %d was settled before the answer: %v", face, res)
		}
		if _, err := a.answerHeldRoll(t, a.caio, res.GetHeldRollOffer().GetHoldId(), strokeAnswer(false)); err != nil {
			t.Fatalf("AnswerHeldRoll() error = %v", err)
		}
		if used := a.strokeUsed(t); used != 0 {
			t.Errorf("stroke_of_luck used = %d after keeping the %d, want 0", used, face)
		}
	}
}

// A Bardic Inspiration die comes before Stroke of Luck: the die is answered first, and the question that follows is about
// the roll as it stands with the die (a natural 1 misses whatever the total).
func TestTheBardicDieComesBeforeStrokeOfLuck(t *testing.T) {
	t.Parallel()
	a := strokeTable(t, "", 20)
	e := a.strokeFight(t)
	a.giveCombatantDie(t, "Pipo", e)
	res, err := a.attack(t, a.caio, e, "Pipo", rapier, "Goblin", d20(1))
	if err != nil || res.GetHeldRollOffer().GetDie() == nil {
		t.Fatalf("RollAttack(1) = %v, %v; want the die's question first", res, err)
	}
	a.h.roller.queue(3)
	next, err := a.answerHeldRoll(t, a.caio, res.GetHeldRollOffer().GetHoldId(), dieAnswer(true))
	if err != nil || next.GetNext() == nil {
		t.Fatalf("the die's answer = %v, %v; want Stroke of Luck's question next", next, err)
	}
	wantStroke(t, "the next question", next.GetNext(), playv1.HeldRollKind_HELD_ROLL_KIND_WEAPON_ATTACK, true)
	if next.GetAttack().GetRoll().GetOutcome() != playv1.AttackOutcome_ATTACK_OUTCOME_UNSPECIFIED {
		t.Errorf("the attack was settled before Stroke of Luck's answer: %v", next.GetAttack())
	}
	if byLabel(t, a.get(t, a.master), "Pipo").GetInspirationDie() != nil {
		t.Error("Pipo still holds the die after using it")
	}
	fin, err := a.answerHeldRoll(t, a.caio, next.GetNext().GetHoldId(), strokeAnswer(true))
	if err != nil || fin.GetAttack().GetRoll().GetOutcome() != playv1.AttackOutcome_ATTACK_OUTCOME_HIT || fin.GetNext() != nil {
		t.Fatalf("Stroke of Luck's answer = %v, %v; want the hit, settled", fin, err)
	}
	if used := a.strokeUsed(t); used != 1 {
		t.Errorf("stroke_of_luck used = %d, want 1", used)
	}
}

// The master's answer waits for the player's first 10 seconds; then it spends the player's use.
func TestTheMastersStrokeOfLuckAnswerWaitsForThePlayerFirst(t *testing.T) {
	t.Parallel()
	a := strokeTable(t, "", 20)
	e := a.strokeFight(t)
	res, err := a.attack(t, a.caio, e, "Pipo", rapier, "Goblin", d20(2))
	if err != nil {
		t.Fatalf("RollAttack(2) error = %v", err)
	}
	hold := res.GetHeldRollOffer().GetHoldId()
	if list := a.listHeldRolls(t, a.master); len(list) != 1 {
		t.Errorf("the master's list = %v, want the question", list)
	}
	_, err = a.answerHeldRoll(t, a.master, hold, strokeAnswer(true))
	wantResourceBlocked(t, err, playv1.ResourceBlockedReason_RESOURCE_BLOCKED_REASON_PLAYER_FIRST)
	a.h.afterThePlayersChance()
	out, err := a.answerHeldRoll(t, a.master, hold, strokeAnswer(true))
	if err != nil || out.GetAttack().GetRoll().GetOutcome() != playv1.AttackOutcome_ATTACK_OUTCOME_HIT {
		t.Fatalf("the master's answer = %v, %v; want the hit", out, err)
	}
	if used := a.strokeUsed(t); used != 1 {
		t.Errorf("stroke_of_luck used = %d, want 1", used)
	}
}

// The question is a roll of the character that waits: another roll is refused until it is answered.
func TestAStrokeOfLuckQuestionBlocksTheNextRoll(t *testing.T) {
	t.Parallel()
	a := strokeTable(t, "", 20)
	e := a.strokeFight(t)
	res, err := a.attack(t, a.caio, e, "Pipo", rapier, "Goblin", d20(2))
	if err != nil {
		t.Fatalf("RollAttack(2) error = %v", err)
	}
	_, err = a.attack(t, a.caio, e, "Pipo", rapier, "Goblin", d20(15))
	wantResourceBlocked(t, err, playv1.ResourceBlockedReason_RESOURCE_BLOCKED_REASON_STROKE_OF_LUCK_PENDING)
	if _, err := a.answerHeldRoll(t, a.caio, res.GetHeldRollOffer().GetHoldId(), strokeAnswer(false)); err != nil {
		t.Fatalf("AnswerHeldRoll() error = %v", err)
	}
}
