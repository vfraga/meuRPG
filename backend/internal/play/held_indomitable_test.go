package play

import (
	"testing"

	"connectrpc.com/connect"

	campaignsv1 "github.com/PuraFome/meuRPG/backend/gen/meurpg/campaigns/v1"
	charactersv1 "github.com/PuraFome/meuRPG/backend/gen/meurpg/characters/v1"
	rulesv1 "github.com/PuraFome/meuRPG/backend/gen/meurpg/rules/v1"

	playv1 "github.com/PuraFome/meuRPG/backend/gen/meurpg/play/v1"
)

// Indomitable (SRD 5.1, Fighter 9/13/17) rerolls a saving throw that fails: the roll is held only for a fighter with
// the feature and a use left, the server works out whether the save failed (the table's DC decides what the roller is
// told), and the new roll is used even when worse. These tests cover the effect's saving throw and the death save.

// indomitableDC is the DC of the effect the master adds by hand (the default of an effect with none), against the
// Wisdom save of Toren (+0).
const indomitableDC = 10

// fighterTable is the casters' table with Toren a fighter of level 9 (one use of Indomitable).
func fighterTable(t *testing.T, race string) *armed {
	t.Helper()
	raceOr := func(own string) string {
		if race == "" {
			return own
		}
		return race
	}
	return newArmedWith(t, func(a *armed) {
		a.toren = a.caio.hero(t, a.campaignID, "Toren", "class:fighter", raceOr("race:human"), 9,
			&rulesv1.AbilityScores{Strength: 16, Dexterity: 13, Constitution: 14, Intelligence: 10, Wisdom: 10, Charisma: 8}, []string{battleaxe}, nil)
		a.pens = a.ana.caster(t, a.campaignID, "Pensantus", "class:wizard", "race:gnome", 3,
			&rulesv1.AbilityScores{Strength: 10, Dexterity: 14, Constitution: 12, Intelligence: 16, Wisdom: 10, Charisma: 8}, nil, []string{fireBolt},
			[]string{magicMissileSpell, shieldSpell, sleepSpell, burningHands, holdPerson, webSpell},
			[]string{magicMissileSpell, shieldSpell, sleepSpell, burningHands, holdPerson, webSpell})
		a.bri = a.bia.caster(t, a.campaignID, "Brisa", "class:cleric", "race:human", 3,
			&rulesv1.AbilityScores{Strength: 10, Dexterity: 16, Constitution: 14, Intelligence: 10, Wisdom: 16, Charisma: 8}, []string{maceKey}, []string{sacredFlame}, nil,
			[]string{cureWounds, healingWord, guidingBolt})
	})
}

// holdOnToren has Hold Person on Toren and his turn end, which opens the save, and returns the open window.
func (a *armed) holdOnToren(t *testing.T) (*playv1.Encounter, *playv1.ReactionWindow) {
	t.Helper()
	e := a.closeFight(t)
	a.mustAddEffect(t, e, holdPerson, []string{"Toren"}, a.rounds(t, 10, "Toren"))
	a.passTo(t, e, "Toren")
	if _, err := a.endTurnRaw(t, a.master, e, true); err != nil {
		t.Fatalf("EndTurn() error = %v", err)
	}
	w := a.windowOf(t, a.caio, playv1.ReactionKind_REACTION_KIND_EFFECT_SAVE)
	if w == nil {
		t.Fatalf("Toren's windows = %v, want the save of Hold Person", a.windows(t, a.caio))
	}
	return e, w
}

// indomitableTable is a fighter table that shows the DC of a save, so the question comes only when the save failed.
func indomitableTable(t *testing.T, race string) (*armed, *playv1.Encounter, *playv1.ReactionWindow) {
	t.Helper()
	a := fighterTable(t, race)
	a.setRules(t, showSaveDC)
	e, w := a.holdOnToren(t)
	return a, e, w
}

func (a *armed) indomitableUsed(t *testing.T) int32 {
	t.Helper()
	used, total := resourceUsed(a.vitals(t, a.toren), "indomitable")
	if total != 1 {
		t.Fatalf("indomitable = %d of %d, want a fighter of level 9 with 1 use", used, total)
	}
	return used
}

// indomitableAnswer rolls the saving throw again, typed (face 0: in the app) or keeps the roll (reroll false).
func indomitableAnswer(reroll bool, face int32) func(*playv1.AnswerHeldRollRequest) {
	return func(r *playv1.AnswerHeldRollRequest) {
		a := &playv1.IndomitableAnswer{Reroll: reroll}
		switch {
		case reroll && face == 0:
			a.Roll = &playv1.IndomitableAnswer_RollInApp{RollInApp: true}
		case reroll:
			a.Roll = &playv1.IndomitableAnswer_TypedFace{TypedFace: face}
		}
		r.Answer = &playv1.AnswerHeldRollRequest_Indomitable{Indomitable: a}
	}
}

func TestAFighterWhoFailsAnEffectSaveIsAskedAboutIndomitable(t *testing.T) {
	t.Parallel()
	a, e, w := indomitableTable(t, "")
	res, err := a.rollEffectSave(t, a.caio, e, w.GetId(), effectSaveFace(5))
	offer := res.GetHeldRollOffer()
	if err != nil || res.GetResult() != nil || offer.GetIndomitable() == nil || !offer.GetIndomitable().GetFailed() ||
		offer.GetKind() != playv1.HeldRollKind_HELD_ROLL_KIND_EFFECT_SAVE || offer.GetD20().GetFaces()[0] != 5 {
		t.Fatalf("RollEffectSave() = %v, %v; want Indomitable's question alone about the 5", res, err)
	}
	if got := byLabel(t, a.get(t, a.master), "Toren").GetHeldRollOffer().GetHoldId(); got != offer.GetHoldId() {
		t.Errorf("the master reads %q, want the question of the hold", got)
	}
	if list := a.listHeldRolls(t, a.caio); len(list) != 1 || list[0].GetIndomitable() == nil {
		t.Errorf("the player's list = %v, want the one question", list)
	}
	if list := a.listHeldRolls(t, a.ana); len(list) != 0 {
		t.Errorf("another player reads %v, want nothing", list)
	}
	if used := a.indomitableUsed(t); used != 0 {
		t.Errorf("a use was spent (%d) before the answer", used)
	}
	_, err = a.answerHeldRoll(t, a.ana, offer.GetHoldId(), indomitableAnswer(true, 17))
	wantCode(t, "another player answers", err, connect.CodeNotFound)

	a.h.roller.queue(17)
	out, err := a.answerHeldRoll(t, a.caio, offer.GetHoldId(), indomitableAnswer(true, 0))
	if err != nil {
		t.Fatalf("AnswerHeldRoll() error = %v", err)
	}
	r := out.GetEffectSave().GetResult()
	if !r.GetSaved() || r.GetD20() != 17 || r.GetTotal() != 17 || !r.GetEffectEnded() || out.GetNext() != nil {
		t.Errorf("the save = %v, next %v; want the new 17 to save and end Hold Person", r, out.GetNext())
	}
	if rr := r.GetRerolled(); len(rr) != 1 || rr[0].GetFrom() != 5 || rr[0].GetTo() != 17 || rr[0].GetSourceKey() != indomitableSourceKey {
		t.Errorf("rerolled = %v, want Indomitable: 5 -> 17", rr)
	}
	if used := a.indomitableUsed(t); used != 1 {
		t.Errorf("indomitable used = %d, want the one use spent", used)
	}
	if list := a.listHeldRolls(t, a.master); len(list) != 0 {
		t.Errorf("the master reads %v after the answer, want nothing", list)
	}
}

func TestAPassedSaveIsNotAskedAndLeavesNoHold(t *testing.T) {
	t.Parallel()
	a, e, w := indomitableTable(t, "")
	res, err := a.rollEffectSave(t, a.caio, e, w.GetId(), effectSaveFace(indomitableDC))
	if err != nil || res.GetHeldRollOffer() != nil || !res.GetResult().GetSaved() {
		t.Fatalf("RollEffectSave() = %v, %v; want the save settled and passed", res, err)
	}
	if list := a.listHeldRolls(t, a.master); len(list) != 0 {
		t.Errorf("holds = %v, want none", list)
	}
	if used := a.indomitableUsed(t); used != 0 {
		t.Errorf("indomitable used = %d, want 0", used)
	}
}

func TestNoUseLeftNoQuestion(t *testing.T) {
	t.Parallel()
	a, e, w := indomitableTable(t, "")
	a.correct(t, a.toren, func(r *playv1.AdjustCharacterVitalsRequest) {
		r.ResourcesUsed = []*playv1.ResourceUsed{{Key: "indomitable", Used: 1}}
	})
	res, err := a.rollEffectSave(t, a.caio, e, w.GetId(), effectSaveFace(5))
	if err != nil || res.GetHeldRollOffer() != nil || res.GetResult().GetSaved() || res.GetResult().GetD20() != 5 {
		t.Fatalf("RollEffectSave() = %v, %v; want the failed 5 settled", res, err)
	}
}

func TestAPlayerWithoutTheFeatureIsNotAsked(t *testing.T) {
	t.Parallel()
	a := newCasters(t) // Toren is a fighter of level 5: no Indomitable
	a.setRules(t, showSaveDC)
	e, w := a.holdOnToren(t)
	res, err := a.rollEffectSave(t, a.caio, e, w.GetId(), effectSaveFace(5))
	if err != nil || res.GetHeldRollOffer() != nil || res.GetResult().GetSaved() {
		t.Fatalf("RollEffectSave() = %v, %v; want the failed 5 settled", res, err)
	}
}

func TestTheNewRollIsUsedEvenWhenWorse(t *testing.T) {
	t.Parallel()
	a, e, w := indomitableTable(t, "")
	res, err := a.rollEffectSave(t, a.caio, e, w.GetId(), effectSaveFace(9))
	if err != nil || res.GetHeldRollOffer().GetIndomitable() == nil {
		t.Fatalf("RollEffectSave() = %v, %v; want the question about the 9", res, err)
	}
	out, err := a.answerHeldRoll(t, a.caio, res.GetHeldRollOffer().GetHoldId(), indomitableAnswer(true, 2))
	if err != nil {
		t.Fatalf("AnswerHeldRoll() error = %v", err)
	}
	if r := out.GetEffectSave().GetResult(); r.GetSaved() || r.GetD20() != 2 || r.GetTotal() != 2 || r.GetEffectEnded() || out.GetNext() != nil {
		t.Errorf("the save = %v, want the worse 2 used: not saved", r)
	}
	if used := a.indomitableUsed(t); used != 1 {
		t.Errorf("indomitable used = %d, want 1: the use is spent even for a worse roll", used)
	}
}

func TestKeepingTheFailedRollSpendsNoUse(t *testing.T) {
	t.Parallel()
	a, e, w := indomitableTable(t, "")
	res, err := a.rollEffectSave(t, a.caio, e, w.GetId(), effectSaveFace(5))
	if err != nil {
		t.Fatalf("RollEffectSave() error = %v", err)
	}
	out, err := a.answerHeldRoll(t, a.caio, res.GetHeldRollOffer().GetHoldId(), indomitableAnswer(false, 0))
	if err != nil {
		t.Fatalf("AnswerHeldRoll() error = %v", err)
	}
	if r := out.GetEffectSave().GetResult(); r.GetSaved() || r.GetD20() != 5 || len(r.GetRerolled()) != 0 {
		t.Errorf("the save = %v, want the failed 5 kept", r)
	}
	if used := a.indomitableUsed(t); used != 0 {
		t.Errorf("indomitable used = %d, want 0", used)
	}
}

func TestAnIndomitableQuestionHasOneAnswer(t *testing.T) {
	t.Parallel()
	a, e, w := indomitableTable(t, "")
	res, err := a.rollEffectSave(t, a.caio, e, w.GetId(), effectSaveFace(5))
	if err != nil {
		t.Fatalf("RollEffectSave() error = %v", err)
	}
	hold := res.GetHeldRollOffer().GetHoldId()
	req := &playv1.AnswerHeldRollRequest{CampaignId: a.campaignID, HoldId: hold, IdempotencyKey: newKey()}
	indomitableAnswer(true, 16)(req)
	first, err := a.caio.resource.AnswerHeldRoll(t.Context(), connect.NewRequest(req))
	if err != nil {
		t.Fatalf("AnswerHeldRoll() error = %v", err)
	}
	again, err := a.caio.resource.AnswerHeldRoll(t.Context(), connect.NewRequest(req))
	if err != nil || again.Msg.GetEffectSave().GetResult().GetD20() != 16 || again.Msg.GetEffectSave().GetResult().GetD20() != first.Msg.GetEffectSave().GetResult().GetD20() ||
		len(again.Msg.GetEffectSave().GetResult().GetRerolled()) != 1 {
		t.Errorf("the same answer again = %v, %v; want the same result", again, err)
	}
	if used := a.indomitableUsed(t); used != 1 {
		t.Errorf("indomitable used = %d after a retry, want 1", used)
	}
	_, err = a.answerHeldRoll(t, a.caio, hold, indomitableAnswer(true, 3))
	wantCode(t, "another answer", err, connect.CodeFailedPrecondition)
	if used := a.indomitableUsed(t); used != 1 {
		t.Errorf("indomitable used = %d after the second answer, want 1", used)
	}
}

// RN-18: the new d20 comes the way the campaign has the player roll.
func TestIndomitableFollowsTheCampaignsDice(t *testing.T) {
	t.Parallel()
	a, e, w := indomitableTable(t, "")
	a.forceDice(t, campaignsv1.DiceMode_DICE_MODE_PHYSICAL)
	res, err := a.rollEffectSave(t, a.caio, e, w.GetId(), effectSaveFace(5))
	if err != nil {
		t.Fatalf("RollEffectSave(typed) error = %v", err)
	}
	hold := res.GetHeldRollOffer().GetHoldId()
	_, err = a.answerHeldRoll(t, a.caio, hold, indomitableAnswer(true, 0))
	wantEncounterBlocked(t, err, playv1.EncounterBlockedReason_ENCOUNTER_BLOCKED_REASON_WRONG_DICE_MODE)
	for _, face := range []int32{-3, 21} {
		_, err = a.answerHeldRoll(t, a.caio, hold, indomitableAnswer(true, face))
		wantCode(t, "a typed face outside the d20", err, connect.CodeInvalidArgument)
	}
	_, err = a.answerHeldRoll(t, a.caio, hold, func(r *playv1.AnswerHeldRollRequest) {
		r.Answer = &playv1.AnswerHeldRollRequest_Indomitable{Indomitable: &playv1.IndomitableAnswer{Reroll: true}}
	})
	wantCode(t, "no way to roll", err, connect.CodeInvalidArgument)
	if used := a.indomitableUsed(t); used != 0 {
		t.Fatalf("indomitable used = %d after the refusals, want 0", used)
	}
	if out, err := a.answerHeldRoll(t, a.caio, hold, indomitableAnswer(true, 15)); err != nil || !out.GetEffectSave().GetResult().GetSaved() {
		t.Errorf("the typed 15 = %v, %v; want the save passed", out, err)
	}
}

func TestIndomitableInAnAppTableRefusesATypedFace(t *testing.T) {
	t.Parallel()
	a := fighterTable(t, "")
	a.setRules(t, showSaveDC)
	e, w := a.holdOnToren(t)
	a.forceDice(t, campaignsv1.DiceMode_DICE_MODE_APP)
	a.h.roller.queue(5)
	res, err := a.rollEffectSave(t, a.caio, e, w.GetId(), func(r *playv1.RollEffectSaveRequest) {
		r.Roll = &playv1.RollEffectSaveRequest_RollInApp{RollInApp: true}
	})
	if err != nil || res.GetHeldRollOffer().GetIndomitable() == nil {
		t.Fatalf("RollEffectSave(app) = %v, %v; want the question", res, err)
	}
	_, err = a.answerHeldRoll(t, a.caio, res.GetHeldRollOffer().GetHoldId(), indomitableAnswer(true, 15))
	wantEncounterBlocked(t, err, playv1.EncounterBlockedReason_ENCOUNTER_BLOCKED_REASON_WRONG_DICE_MODE)
	a.h.roller.queue(15)
	if out, err := a.answerHeldRoll(t, a.caio, res.GetHeldRollOffer().GetHoldId(), indomitableAnswer(true, 0)); err != nil || !out.GetEffectSave().GetResult().GetSaved() {
		t.Errorf("the app roll = %v, %v; want the save passed", out, err)
	}
}

// A table that hides the DC asks on every save the fighter could use Indomitable on, so the question says nothing
// about the result; a rerolled save that passed is still a use spent, since the roller could not know.
func TestAHiddenDCAsksOnEverySaveAndTellsNothing(t *testing.T) {
	t.Parallel()
	a := fighterTable(t, "")
	e, w := a.holdOnToren(t)
	res, err := a.rollEffectSave(t, a.caio, e, w.GetId(), effectSaveFace(20))
	offer := res.GetHeldRollOffer()
	if err != nil || offer.GetIndomitable() == nil || offer.GetIndomitable().GetFailed() {
		t.Fatalf("RollEffectSave(20) = %v, %v; want the question with nothing said of the result", res, err)
	}
	out, err := a.answerHeldRoll(t, a.caio, offer.GetHoldId(), indomitableAnswer(false, 0))
	if err != nil || !out.GetEffectSave().GetResult().GetSaved() || a.indomitableUsed(t) != 0 {
		t.Errorf("keeping = %v, %v; want the saved 20 and no use spent", out, err)
	}
}

func TestTheMastersAnswerWaitsForThePlayerFirst(t *testing.T) {
	t.Parallel()
	a, e, w := indomitableTable(t, "")
	res, err := a.rollEffectSave(t, a.caio, e, w.GetId(), effectSaveFace(5))
	if err != nil {
		t.Fatalf("RollEffectSave() error = %v", err)
	}
	hold := res.GetHeldRollOffer().GetHoldId()
	_, err = a.answerHeldRoll(t, a.master, hold, indomitableAnswer(true, 14))
	wantResourceBlocked(t, err, playv1.ResourceBlockedReason_RESOURCE_BLOCKED_REASON_PLAYER_FIRST)
	a.h.afterThePlayersChance()
	if out, err := a.answerHeldRoll(t, a.master, hold, indomitableAnswer(true, 14)); err != nil || !out.GetEffectSave().GetResult().GetSaved() {
		t.Errorf("the master's answer = %v, %v; want the save passed", out, err)
	}
	if used := a.indomitableUsed(t); used != 1 {
		t.Errorf("indomitable used = %d, want 1", used)
	}
}

// The question is a roll of the character that waits: another roll is refused until it is answered.
func TestAnIndomitableQuestionBlocksTheNextRoll(t *testing.T) {
	t.Parallel()
	a, e, w := indomitableTable(t, "")
	if _, err := a.rollEffectSave(t, a.caio, e, w.GetId(), effectSaveFace(5)); err != nil {
		t.Fatalf("RollEffectSave() error = %v", err)
	}
	_, err := a.rollEffectSave(t, a.caio, e, w.GetId(), effectSaveFace(6))
	wantResourceBlocked(t, err, playv1.ResourceBlockedReason_RESOURCE_BLOCKED_REASON_INDOMITABLE_PENDING)
}

func TestALongRestGivesIndomitableBack(t *testing.T) {
	t.Parallel()
	a, e, w := indomitableTable(t, "")
	res, err := a.rollEffectSave(t, a.caio, e, w.GetId(), effectSaveFace(5))
	if err != nil {
		t.Fatalf("RollEffectSave() error = %v", err)
	}
	if _, err := a.answerHeldRoll(t, a.caio, res.GetHeldRollOffer().GetHoldId(), indomitableAnswer(true, 9)); err != nil {
		t.Fatalf("AnswerHeldRoll() error = %v", err)
	}
	if used := a.indomitableUsed(t); used != 1 {
		t.Fatalf("indomitable used = %d, want 1", used)
	}
	a.endEncounter(t, e)
	if err := a.rpcLongRest(t); err != nil {
		t.Fatalf("TakeRest(long) error = %v", err)
	}
	if used := a.indomitableUsed(t); used != 0 {
		t.Errorf("indomitable used = %d after a long rest, want 0", used)
	}
}

// A halfling fighter: Lucky's 1 first, then Indomitable about the result of the new roll.
func TestLuckyComesBeforeIndomitable(t *testing.T) {
	t.Parallel()
	a, e, w := indomitableTable(t, "race:halfling")
	res, err := a.rollEffectSave(t, a.caio, e, w.GetId(), effectSaveFace(1))
	if err != nil || res.GetHeldRollOffer().GetLucky() == nil {
		t.Fatalf("RollEffectSave(1) = %v, %v; want Lucky's question first", res, err)
	}
	out, err := a.answerHeldRoll(t, a.caio, res.GetHeldRollOffer().GetHoldId(), luckyAnswer(8))
	if err != nil || out.GetNext().GetIndomitable() == nil || !out.GetNext().GetIndomitable().GetFailed() || out.GetEffectSave().GetResult() != nil {
		t.Fatalf("Lucky's answer (8) = %v, %v; want Indomitable's question on the new 8", out, err)
	}
	if used := a.indomitableUsed(t); used != 0 {
		t.Errorf("indomitable used = %d, want 0 before the answer", used)
	}
	// The Lucky answer again (a retry) reads the question and spends nothing.
	if again, err := a.answerHeldRoll(t, a.caio, out.GetNext().GetHoldId(), indomitableAnswer(true, 16)); err != nil || !again.GetEffectSave().GetResult().GetSaved() {
		t.Fatalf("Indomitable's answer = %v, %v; want the 16 to save", again, err)
	} else if rr := again.GetEffectSave().GetResult().GetRerolled(); len(rr) != 1 || rr[0].GetSourceKey() != indomitableSourceKey || rr[0].GetFrom() != 8 || rr[0].GetTo() != 16 {
		t.Errorf("rerolled = %v, want Indomitable: 8 -> 16 (the new roll replaces Lucky's)", rr)
	}
}

func TestALuckyRerollThatPassesAsksNothingMore(t *testing.T) {
	t.Parallel()
	a, e, w := indomitableTable(t, "race:halfling")
	res, err := a.rollEffectSave(t, a.caio, e, w.GetId(), effectSaveFace(1))
	if err != nil {
		t.Fatalf("RollEffectSave(1) error = %v", err)
	}
	out, err := a.answerHeldRoll(t, a.caio, res.GetHeldRollOffer().GetHoldId(), luckyAnswer(15))
	if err != nil || out.GetNext() != nil || !out.GetEffectSave().GetResult().GetSaved() {
		t.Errorf("Lucky's answer (15) = %v, %v; want the save passed and settled", out, err)
	}
}

// The Bardic Inspiration die comes before the result, Indomitable after it: the die is spent on the roll the fighter
// then sees, and the question counts it.
func TestTheBardicDieComesBeforeIndomitable(t *testing.T) {
	t.Parallel()
	a, e, w := indomitableTable(t, "")
	a.giveCombatantDie(t, "Toren", e)
	res, err := a.rollEffectSave(t, a.caio, e, w.GetId(), effectSaveFace(5))
	if err != nil || res.GetHeldRollOffer().GetDie() == nil {
		t.Fatalf("RollEffectSave(5) = %v, %v; want the die's question", res, err)
	}
	hold := res.GetHeldRollOffer().GetHoldId()
	a.h.roller.queue(4) // 5 + 4 = 9: still a failure
	req := &playv1.AnswerHeldRollRequest{CampaignId: a.campaignID, HoldId: hold, IdempotencyKey: newKey()}
	dieAnswer(true)(req)
	first, err := a.caio.resource.AnswerHeldRoll(t.Context(), connect.NewRequest(req))
	if err != nil {
		t.Fatalf("AnswerHeldRoll(die) error = %v", err)
	}
	next := first.Msg.GetNext()
	if next.GetIndomitable() == nil || !next.GetIndomitable().GetFailed() || next.GetD20().GetTotal() != 9 || first.Msg.GetEffectSave().GetResult() != nil {
		t.Fatalf("the die's answer = %v; want Indomitable's question about the 9", first.Msg)
	}
	if byLabel(t, a.get(t, a.master), "Toren").GetInspirationDie() != nil {
		t.Error("Toren still holds the die after using it")
	}
	// The die's answer again reads the question, rolls and spends nothing more.
	again, err := a.caio.resource.AnswerHeldRoll(t.Context(), connect.NewRequest(req))
	if err != nil || again.Msg.GetNext().GetIndomitable() == nil {
		t.Fatalf("the die's answer again = %v, %v; want the same question", again, err)
	}
	// A different answer to the die's question is refused now.
	_, err = a.answerHeldRoll(t, a.caio, hold, dieAnswer(false))
	wantCode(t, "another die answer", err, connect.CodeFailedPrecondition)
	out, err := a.answerHeldRoll(t, a.caio, hold, indomitableAnswer(true, 12))
	if err != nil {
		t.Fatalf("AnswerHeldRoll(indomitable) error = %v", err)
	}
	r := out.GetEffectSave().GetResult()
	if !r.GetSaved() || r.GetD20() != 12 || r.GetTotal() != 12 || r.GetInspirationFace() != 4 {
		t.Errorf("the save = %v; want the new 12 alone: the die went with the roll it was added to", r)
	}
}

// The die added to a roll that is rerolled is spent and not added to the new roll; keeping the roll keeps the die in
// the total.
func TestTheDieStaysWithTheRollItWasAddedTo(t *testing.T) {
	t.Parallel()
	for _, reroll := range []bool{true, false} {
		a, e, w := indomitableTable(t, "")
		a.giveCombatantDie(t, "Toren", e)
		res, err := a.rollEffectSave(t, a.caio, e, w.GetId(), effectSaveFace(5))
		if err != nil || res.GetHeldRollOffer().GetDie() == nil {
			t.Fatalf("RollEffectSave(5) = %v, %v; want the die's question", res, err)
		}
		hold := res.GetHeldRollOffer().GetHoldId()
		a.h.roller.queue(4) // 5 + 4 = 9: still a failure
		if _, err := a.answerHeldRoll(t, a.caio, hold, dieAnswer(true)); err != nil {
			t.Fatalf("AnswerHeldRoll(die) error = %v", err)
		}
		answer, want := indomitableAnswer(false, 0), int32(9)
		if reroll {
			answer, want = indomitableAnswer(true, 8), 8
		}
		out, err := a.answerHeldRoll(t, a.caio, hold, answer)
		if err != nil {
			t.Fatalf("AnswerHeldRoll(indomitable, reroll=%v) error = %v", reroll, err)
		}
		r := out.GetEffectSave().GetResult()
		if r.GetTotal() != want || r.GetInspirationFace() != 4 || r.GetSaved() {
			t.Errorf("reroll=%v: the save = %v; want the total %d (the 4 spent, still a failure)", reroll, r, want)
		}
		if got := byLabel(t, a.get(t, a.master), "Toren").GetInspirationDie(); got != nil {
			t.Errorf("reroll=%v: Toren still holds the die", reroll)
		}
	}
}

// A death save fails by the roll (a total under 10, a natural 1 counted); a natural 20 never does.
func TestADeathSaveThatFailsIsAskedAboutIndomitable(t *testing.T) {
	t.Parallel()
	a, e := dyingFighter(t, "")
	a.undoes(t, "a death save rerolled with Indomitable", func() {
		res, err := a.deathSave(t, a.caio, e, "Toren", deathFace(5))
		offer := res.GetHeldRollOffer()
		if err != nil || res.GetDeathSave() != nil || offer.GetIndomitable() == nil || !offer.GetIndomitable().GetFailed() ||
			offer.GetKind() != playv1.HeldRollKind_HELD_ROLL_KIND_DEATH_SAVE {
			t.Fatalf("RollDeathSave(5) = %v, %v; want Indomitable's question alone", res, err)
		}
		if s, f := torenCounts(t, a); s != 0 || f != 0 {
			t.Errorf("counts %d/%d while the question waits; want 0/0", s, f)
		}
		out, err := a.answerHeldRoll(t, a.caio, offer.GetHoldId(), indomitableAnswer(true, 14))
		if err != nil {
			t.Fatalf("AnswerHeldRoll() error = %v", err)
		}
		ds := out.GetDeathSave().GetDeathSave()
		if ds.GetOutcome() != playv1.DeathSaveOutcome_DEATH_SAVE_OUTCOME_SUCCESS || ds.GetSuccesses() != 1 || ds.GetRoll().GetFaces()[0] != 14 ||
			len(ds.GetRoll().GetRerolled()) != 1 || ds.GetRoll().GetRerolled()[0].GetFrom() != 5 {
			t.Errorf("the death save = %v; want the new 14 to succeed", ds)
		}
		if used := a.indomitableUsed(t); used != 1 {
			t.Errorf("indomitable used = %d, want 1", used)
		}
	})
	if used := a.indomitableUsed(t); used != 0 {
		t.Errorf("indomitable used = %d after the undo, want the use back", used)
	}
}

func TestADeathSaveThatPassesOrIsATwentyIsNotAsked(t *testing.T) {
	t.Parallel()
	for _, face := range []int32{10, 20} {
		a, e := dyingFighter(t, "")
		res, err := a.deathSave(t, a.caio, e, "Toren", deathFace(face))
		if err != nil || res.GetHeldRollOffer() != nil || res.GetDeathSave() == nil {
			t.Errorf("RollDeathSave(%d) = %v, %v; want it settled", face, res, err)
		}
	}
}

func TestANaturalOneOnADeathSaveIsAskedAndKeepingItIsTwoFailures(t *testing.T) {
	t.Parallel()
	a, e := dyingFighter(t, "")
	res, err := a.deathSave(t, a.caio, e, "Toren", deathFace(1))
	if err != nil || res.GetHeldRollOffer().GetIndomitable() == nil {
		t.Fatalf("RollDeathSave(1) = %v, %v; want the question", res, err)
	}
	out, err := a.answerHeldRoll(t, a.caio, res.GetHeldRollOffer().GetHoldId(), indomitableAnswer(false, 0))
	if ds := out.GetDeathSave().GetDeathSave(); err != nil || ds.GetFailures() != 2 {
		t.Errorf("keeping the 1 = %v, %v; want two failures", ds, err)
	}
	if used := a.indomitableUsed(t); used != 0 {
		t.Errorf("indomitable used = %d, want 0", used)
	}
}

// dyingFighter is the table with Toren (a fighter of level 9) at 0 hit points and a death save due on his turn.
func dyingFighter(t *testing.T, race string) (*armed, *playv1.Encounter) {
	t.Helper()
	a := fighterTable(t, race)
	a.setRules(t, showSaveDC)
	e := a.castersFight(t, 1)
	a.correct(t, a.toren, hpIs(0))
	a.mustEndTurn(t, a.ana, e)
	return a, e
}

// multiclassTable is the casters' table with Pensantus a wizard 3 and a fighter 9, who concentrates on a spell and
// has Indomitable.
func multiclassTable(t *testing.T) *armed {
	t.Helper()
	a := newArmedWith(t, func(a *armed) {
		a.toren = a.caio.hero(t, a.campaignID, "Toren", "class:fighter", "race:human", 5,
			&rulesv1.AbilityScores{Strength: 16, Dexterity: 13, Constitution: 14, Intelligence: 10, Wisdom: 10, Charisma: 8}, []string{battleaxe}, nil)
		res, err := a.ana.characters.CreateCharacter(t.Context(), connect.NewRequest(&charactersv1.CreateCharacterRequest{
			CampaignId: a.campaignID, Kind: charactersv1.CharacterKind_CHARACTER_KIND_PLAYER, Name: "Pensantus",
			Sheet: &charactersv1.CharacterSheet{Content: &charactersv1.CharacterSheet_Full{Full: &charactersv1.FullSheet{
				BaseScores: &rulesv1.AbilityScores{Strength: 10, Dexterity: 14, Constitution: 12, Intelligence: 16, Wisdom: 10, Charisma: 8},
				RaceKey:    "race:gnome", Classes: []*charactersv1.ClassLevel{{ClassKey: "class:wizard", Level: 3}, {ClassKey: "class:fighter", Level: 9}},
				CantripKeys: []string{fireBolt}, KnownSpellKeys: []string{webSpell, shieldSpell}, PreparedSpellKeys: []string{webSpell, shieldSpell},
			}}},
		}))
		if err != nil {
			t.Fatalf("CreateCharacter(Pensantus) error = %v", err)
		}
		a.pens = res.Msg.GetCharacter()
		a.bri = a.bia.caster(t, a.campaignID, "Brisa", "class:cleric", "race:human", 3,
			&rulesv1.AbilityScores{Strength: 10, Dexterity: 16, Constitution: 14, Intelligence: 10, Wisdom: 16, Charisma: 8}, []string{maceKey}, []string{sacredFlame}, nil,
			[]string{cureWounds, healingWord, guidingBolt})
	})
	a.setRules(t, showSaveDC)
	return a
}

// The concentration save is a saving throw too: Constitution +1 against DC 10, so a 5 fails.
func TestAFailedConcentrationSaveIsAskedAboutIndomitable(t *testing.T) {
	t.Parallel()
	a, e := concentratingHitOn(t, multiclassTable(t), 6)
	w := a.windowOf(t, a.ana, playv1.ReactionKind_REACTION_KIND_CONCENTRATION_SAVE)
	if w == nil {
		t.Fatalf("Pensantus's windows = %v, want a concentration save", a.windows(t, a.ana))
	}
	res, err := a.resolveConcentration(t, a.ana, e, w.GetId(), concentrationFace(5))
	offer := res.GetHeldRollOffer()
	if err != nil || res.GetResult() != nil || offer.GetIndomitable() == nil || !offer.GetIndomitable().GetFailed() ||
		offer.GetKind() != playv1.HeldRollKind_HELD_ROLL_KIND_CONCENTRATION_SAVE {
		t.Fatalf("ResolveConcentrationSave(5) = %v, %v; want Indomitable's question alone", res, err)
	}
	out, err := a.answerHeldRoll(t, a.ana, offer.GetHoldId(), indomitableAnswer(true, 15))
	r := out.GetConcentrationSave().GetResult()
	if err != nil || !r.GetKept() || r.GetSave().GetFaces()[0] != 15 || len(r.GetSave().GetRerolled()) != 1 || r.GetSave().GetRerolled()[0].GetFrom() != 5 {
		t.Errorf("the save = %v, %v; want the new 15 to keep the concentration", r, err)
	}
	if used, total := resourceUsed(a.vitals(t, a.pens), "indomitable"); used != 1 || total != 1 {
		t.Errorf("indomitable = %d of %d, want 1 of 1", used, total)
	}
}

// A passed concentration save asks nothing.
func TestAPassedConcentrationSaveIsNotAsked(t *testing.T) {
	t.Parallel()
	a, e := concentratingHitOn(t, multiclassTable(t), 6)
	w := a.windowOf(t, a.ana, playv1.ReactionKind_REACTION_KIND_CONCENTRATION_SAVE)
	res, err := a.resolveConcentration(t, a.ana, e, w.GetId(), concentrationFace(15))
	if err != nil || res.GetHeldRollOffer() != nil || !res.GetResult().GetKept() {
		t.Errorf("ResolveConcentrationSave(15) = %v, %v; want it settled and kept", res, err)
	}
}

// typedFacesAnswer is Indomitable's answer for a save rolled with advantage or disadvantage in a table of real dice.
func typedFacesAnswer(faces ...int32) func(*playv1.AnswerHeldRollRequest) {
	return func(r *playv1.AnswerHeldRollRequest) {
		r.Answer = &playv1.AnswerHeldRollRequest_Indomitable{Indomitable: &playv1.IndomitableAnswer{Reroll: true, TypedFaces: faces}}
	}
}

// A gnome fighter saves a spell's Wisdom save with advantage (Gnome Cunning): two d20, so the new roll takes two typed
// faces. The count must match, each face is 1 to 20, and the higher of the new pair counts.
func TestIndomitableTakesTheTypedPairOfASaveWithAdvantage(t *testing.T) {
	t.Parallel()
	a, e, w := indomitableTable(t, "race:gnome")
	a.forceDice(t, campaignsv1.DiceMode_DICE_MODE_PHYSICAL)
	res, err := a.rollEffectSave(t, a.caio, e, w.GetId(), func(r *playv1.RollEffectSaveRequest) {
		r.Roll = &playv1.RollEffectSaveRequest_D20Face{D20Face: 3}
		r.SecondD20Face = new(int32(5))
	})
	offer := res.GetHeldRollOffer()
	if err != nil || offer.GetIndomitable() == nil || len(offer.GetD20().GetFaces()) != 2 {
		t.Fatalf("RollEffectSave(3, 5) = %v, %v; want the question about a pair", res, err)
	}
	hold := offer.GetHoldId()
	for name, faces := range map[string][]int32{"one face": {15}, "three faces": {15, 4, 9}, "a face of 0": {0, 9}, "a face of 21": {21, 9}} {
		if _, err := a.answerHeldRoll(t, a.caio, hold, typedFacesAnswer(faces...)); err == nil {
			t.Errorf("typed_faces with %s was accepted, want it refused", name)
		}
	}
	if used := a.indomitableUsed(t); used != 0 {
		t.Fatalf("indomitable used = %d after the refusals, want 0", used)
	}
	out, err := a.answerHeldRoll(t, a.caio, hold, typedFacesAnswer(4, 15))
	r := out.GetEffectSave().GetResult()
	if err != nil || !r.GetSaved() || r.GetD20() != 15 || len(r.GetD20Faces()) != 2 || len(r.GetRerolled()) == 0 {
		t.Fatalf("the typed pair (4, 15) = %v, %v; want the 15 counting and the save passed", out, err)
	}
	if used := a.indomitableUsed(t); used != 1 {
		t.Errorf("indomitable used = %d, want 1", used)
	}
}

// The log line "Indomitável: 5 → 17" is read by the roller's player and the master, and by nobody else (RN-10).
func TestTheLogSaysIndomitableToTheOwnerAndTheMasterOnly(t *testing.T) {
	t.Parallel()
	const words = "Indomitável: 5 → 17"
	a, e, w := indomitableTable(t, "")
	res, err := a.rollEffectSave(t, a.caio, e, w.GetId(), effectSaveFace(5))
	if err != nil {
		t.Fatalf("RollEffectSave(5) error = %v", err)
	}
	if _, err := a.answerHeldRoll(t, a.caio, res.GetHeldRollOffer().GetHoldId(), indomitableAnswer(true, 17)); err != nil {
		t.Fatalf("AnswerHeldRoll() error = %v", err)
	}
	// The effect's line carries the reroll as data, which the screen words as "Indomitável: 5 → 17".
	reads := func(u *user) bool {
		for _, l := range logEntries(a.log(t, u, e)) {
			for _, r := range l.GetEffect().GetRerolled() {
				if r.GetSourceKey() == indomitableSourceKey && r.GetFrom() == 5 && r.GetTo() == 17 {
					return true
				}
			}
		}
		return false
	}
	for name, u := range map[string]*user{"the owner": a.caio, "the master": a.master} {
		if !reads(u) {
			t.Errorf("the effect save: %s reads no Indomitable 5 -> 17", name)
		}
	}
	for name, u := range map[string]*user{"Pensantus's player": a.ana, "Brisa's player": a.bia} {
		if reads(u) || a.logSays(t, u, e, "Indomitável") {
			t.Errorf("the effect save: %s reads Indomitable", name)
		}
	}

	c, ce := concentratingHitOn(t, multiclassTable(t), 6)
	cw := c.windowOf(t, c.ana, playv1.ReactionKind_REACTION_KIND_CONCENTRATION_SAVE)
	cres, err := c.resolveConcentration(t, c.ana, ce, cw.GetId(), concentrationFace(5))
	if err != nil {
		t.Fatalf("ResolveConcentrationSave(5) error = %v", err)
	}
	if _, err := c.answerHeldRoll(t, c.ana, cres.GetHeldRollOffer().GetHoldId(), indomitableAnswer(true, 17)); err != nil {
		t.Fatalf("AnswerHeldRoll() error = %v", err)
	}
	for name, u := range map[string]*user{"the owner": c.ana, "the master": c.master} {
		if !c.logSays(t, u, ce, words) {
			t.Errorf("the concentration save: %s reads no %q", name, words)
		}
	}
	for name, u := range map[string]*user{"Toren's player": c.caio, "Brisa's player": c.bia} {
		if c.logSays(t, u, ce, "Indomitável") {
			t.Errorf("the concentration save: %s reads Indomitable", name)
		}
	}
}

// A table that hides the DC of a save asks about a concentration save that would have been kept too, tells nothing
// of the result, and shows nothing of the question or of the reroll to the other players.
func TestAHiddenDCConcentrationSaveLeaksNothing(t *testing.T) {
	t.Parallel()
	a := multiclassTable(t)
	a.setRules(t, func(r *campaignsv1.TableRules) { r.ShowSaveDc = false })
	a, e := concentratingHitOn(t, a, 6)
	w := a.windowOf(t, a.ana, playv1.ReactionKind_REACTION_KIND_CONCENTRATION_SAVE)
	res, err := a.resolveConcentration(t, a.ana, e, w.GetId(), concentrationFace(20))
	offer := res.GetHeldRollOffer()
	if err != nil || offer.GetIndomitable() == nil || offer.GetIndomitable().GetFailed() {
		t.Fatalf("ResolveConcentrationSave(20) = %v, %v; want the question with nothing said of the result", res, err)
	}
	for name, u := range map[string]*user{"Toren's player": a.caio, "Brisa's player": a.bia} {
		if list := a.listHeldRolls(t, u); len(list) != 0 {
			t.Errorf("%s reads %v, want nothing", name, list)
		}
		if byLabel(t, a.get(t, u), "Pensantus").GetHeldRollOffer() != nil {
			t.Errorf("%s reads Pensantus's question in the combat", name)
		}
	}
	if _, err := a.answerHeldRoll(t, a.ana, offer.GetHoldId(), indomitableAnswer(true, 2)); err != nil {
		t.Fatalf("AnswerHeldRoll() error = %v", err)
	}
	for name, u := range map[string]*user{"Toren's player": a.caio, "Brisa's player": a.bia} {
		if a.logSays(t, u, e, "Indomitável") {
			t.Errorf("%s reads Indomitable in the log", name)
		}
	}
}

// Dice an effect adds after the gate (Bless) move the total, so the question comes blind even when the DC is shown:
// it does not say the save failed.
func TestEffectDiceAfterTheGateMakeTheQuestionBlind(t *testing.T) {
	t.Parallel()
	a := fighterTable(t, "")
	a.setRules(t, showSaveDC)
	e := a.closeFight(t)
	a.mustAddEffect(t, e, "spell:bless", []string{"Toren"}, a.rounds(t, 10, "Pensantus"))
	a.mustAddEffect(t, e, holdPerson, []string{"Toren"}, a.rounds(t, 10, "Toren"))
	a.passTo(t, e, "Toren")
	if _, err := a.endTurnRaw(t, a.master, e, true); err != nil {
		t.Fatalf("EndTurn() error = %v", err)
	}
	w := a.windowOf(t, a.caio, playv1.ReactionKind_REACTION_KIND_EFFECT_SAVE)
	if w == nil {
		t.Fatalf("Toren's windows = %v, want the save of Hold Person", a.windows(t, a.caio))
	}
	res, err := a.rollEffectSave(t, a.caio, e, w.GetId(), effectSaveFace(5))
	offer := res.GetHeldRollOffer()
	if err != nil || offer.GetIndomitable() == nil || offer.GetIndomitable().GetFailed() {
		t.Fatalf("RollEffectSave(5) = %v, %v; want the question with nothing said of the result", res, err)
	}
}

// The master rolls a save for a player's character: the player is asked, the master reads the question too, and the
// player's answer spends the player's use.
func TestAMasterRolledSaveAsksThePlayer(t *testing.T) {
	t.Parallel()
	a, e, w := indomitableTable(t, "")
	res, err := a.rollEffectSave(t, a.master, e, w.GetId(), effectSaveFace(5))
	offer := res.GetHeldRollOffer()
	if err != nil || offer.GetIndomitable() == nil || !offer.GetIndomitable().GetFailed() {
		t.Fatalf("RollEffectSave(master, 5) = %v, %v; want Indomitable's question", res, err)
	}
	if list := a.listHeldRolls(t, a.caio); len(list) != 1 {
		t.Errorf("the player's list = %v, want the one question", list)
	}
	if list := a.listHeldRolls(t, a.master); len(list) != 1 {
		t.Errorf("the master's list = %v, want the one question", list)
	}
	out, err := a.answerHeldRoll(t, a.caio, offer.GetHoldId(), indomitableAnswer(true, 16))
	if err != nil || !out.GetEffectSave().GetResult().GetSaved() || a.indomitableUsed(t) != 1 {
		t.Errorf("the player's answer = %v, %v; want the save passed and the use spent", out, err)
	}
}

// A save with advantage takes both faces: a single typed face is refused, and so is a mix of ways to roll.
func TestIndomitableRefusesWrongWaysToRollAPair(t *testing.T) {
	t.Parallel()
	a, e, w := indomitableTable(t, "race:gnome")
	a.forceDice(t, campaignsv1.DiceMode_DICE_MODE_PHYSICAL)
	res, err := a.rollEffectSave(t, a.caio, e, w.GetId(), func(r *playv1.RollEffectSaveRequest) {
		r.Roll = &playv1.RollEffectSaveRequest_D20Face{D20Face: 3}
		r.SecondD20Face = new(int32(5))
	})
	if err != nil || res.GetHeldRollOffer().GetIndomitable() == nil {
		t.Fatalf("RollEffectSave(3, 5) = %v, %v; want the question", res, err)
	}
	hold := res.GetHeldRollOffer().GetHoldId()
	_, err = a.answerHeldRoll(t, a.caio, hold, indomitableAnswer(true, 15))
	wantCode(t, "one typed face for a pair", err, connect.CodeInvalidArgument)
	_, err = a.answerHeldRoll(t, a.caio, hold, func(r *playv1.AnswerHeldRollRequest) {
		r.Answer = &playv1.AnswerHeldRollRequest_Indomitable{Indomitable: &playv1.IndomitableAnswer{
			Reroll: true, Roll: &playv1.IndomitableAnswer_TypedFace{TypedFace: 9}, TypedFaces: []int32{4, 15},
		}}
	})
	wantCode(t, "typed_faces with typed_face", err, connect.CodeInvalidArgument)
	_, err = a.answerHeldRoll(t, a.caio, hold, func(r *playv1.AnswerHeldRollRequest) {
		r.Answer = &playv1.AnswerHeldRollRequest_Indomitable{Indomitable: &playv1.IndomitableAnswer{
			Reroll: true, Roll: &playv1.IndomitableAnswer_RollInApp{RollInApp: true}, TypedFaces: []int32{4, 15},
		}}
	})
	wantCode(t, "typed_faces with roll_in_app", err, connect.CodeInvalidArgument)
	if used := a.indomitableUsed(t); used != 0 {
		t.Errorf("indomitable used = %d after the refusals, want 0", used)
	}
}
