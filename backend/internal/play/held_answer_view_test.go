package play

import (
	"context"
	"strings"
	"testing"

	"google.golang.org/protobuf/encoding/protojson"
	"google.golang.org/protobuf/proto"

	campaignsv1 "github.com/PuraFome/meuRPG/backend/gen/meurpg/campaigns/v1"
	playv1 "github.com/PuraFome/meuRPG/backend/gen/meurpg/play/v1"
	"github.com/PuraFome/meuRPG/backend/internal/authz"
)

// The answer to a held roll renders its response for the member who answered it, whoever rolled: the roller's
// membership stays the one the path writes and checks as. These tests run a held roll made for another than the
// answerer both ways (the master's roll answered by the player, the player's answered by the master) and read the
// combat or the group check the response carries.

const (
	// shadowName is the name of an NPC the master hides, and shadowHP its hit points.
	shadowName = "Sombra-Zeta-Quatro"
	shadowHP   = 4321
)

// shadowFight is brisaFight with a second NPC that stays hidden: the party's turn order is the same.
func shadowFight(t *testing.T, race string, wizardLevel int32, die bool) (*armed, *playv1.Encounter) {
	t.Helper()
	a := newCastersOf(t, wizardLevel, race)
	shadow := a.master.npc(t, a.campaignID, shadowName, shadowHP, 17)
	e := a.start(t, plan{
		npcs:     []*playv1.Participant{{CharacterId: a.goblin.GetId()}, {CharacterId: shadow.GetId()}},
		npcRolls: []int{3, 2},
		players:  map[string]int32{"Toren": 18, "Pensantus": 10, "Brisa": 1},
		reveal:   []string{"Goblin"},
		at:       map[string][2]int32{"Toren": {3, 3}, "Goblin": {4, 3}, "Pensantus": {10, 3}, "Brisa": {4, 4}},
	})
	if die {
		a.passTo(t, e, "Brisa")
		e = a.get(t, a.master)
		a.giveCombatantDie(t, "Brisa", e)
	}
	return a, e
}

// readsShadow says the combat carries the hidden NPC, by its label or by its hit points.
func readsShadow(e *playv1.Encounter) bool {
	for _, c := range e.GetCombatants() {
		if strings.Contains(c.GetLabel(), shadowName) || c.GetHitPointsMax() == shadowHP || c.GetHitPointsCurrent() == shadowHP {
			return true
		}
	}
	return false
}

// wantNoShadow fails when the combat of a response carries the hidden NPC, and the whole response when its text does.
func wantNoShadow(t *testing.T, what string, e *playv1.Encounter, whole proto.Message) {
	t.Helper()
	if e == nil {
		t.Fatalf("%s carries no combat", what)
	}
	if readsShadow(e) || strings.Contains(protojson.Format(whole), shadowName) {
		t.Errorf("%s carries the hidden combatant", what)
	}
}

// wantShadow fails when the combat of a response lacks the hidden NPC: the master reads it.
func wantShadow(t *testing.T, what string, e *playv1.Encounter) {
	t.Helper()
	if !readsShadow(e) {
		t.Errorf("%s lacks the hidden combatant, want the master's combat", what)
	}
}

// typedDie answers the Bardic Inspiration question with the die used, typed from a real die.
func typedDie(face int32) func(*playv1.AnswerHeldRollRequest) {
	return func(r *playv1.AnswerHeldRollRequest) {
		r.Answer = &playv1.AnswerHeldRollRequest_Die{Die: &playv1.DieAnswer{Use: true, Roll: &playv1.DieAnswer_TypedFace{TypedFace: face}}}
	}
}

// mastersGuidingBolt has the master cast Guiding Bolt for Brisa, who holds a die: the cast waits for her answer.
func mastersGuidingBolt(t *testing.T) (a *armed, hold string) {
	t.Helper()
	a, e := shadowFight(t, "race:halfling", 3, true)
	typed := func(r *playv1.CastSpellRequest) { r.Roll, r.D20Faces = nil, []int32{4, 4} } // two dice: the cast rolls with advantage or disadvantage here
	res, err := a.cast(t, a.master, e, "Brisa", guidingBolt, slotOfLevel(1), a.at(t, "Goblin"), typed)
	if err != nil || res.GetCast() != nil || res.GetHeldRollOffer() == nil {
		t.Fatalf("CastSpell(master) = %v, %v; want the die's question and no cast", res, err)
	}
	return a, res.GetHeldRollOffer().GetHoldId()
}

// wantCastSettled checks the roll and the write the master's cast makes, whoever answers it.
func wantCastSettled(t *testing.T, a *armed, cast *playv1.SpellCast) {
	t.Helper()
	if len(cast.GetTargets()) != 1 || len(cast.GetTargets()[0].GetBonusDice()) != 1 || cast.GetTargets()[0].GetAttackRoll().GetFaces()[0] != 4 {
		t.Errorf("the cast = %v, want the master's d20 of 4 with the player's d8", cast)
	}
	if !byLabel(t, a.get(t, a.master), "Brisa").GetActionUsed() {
		t.Error("the action is not spent after the answer")
	}
}

// TestHeldAnswerRendersTheAnsweringMembersView: the master's cast for a player's character, answered by the player
// through either RPC, comes back as the player's combat.
func TestHeldAnswerRendersTheAnsweringMembersView(t *testing.T) {
	t.Parallel()
	t.Run("AnswerHeldRoll", func(t *testing.T) {
		t.Parallel()
		a, hold := mastersGuidingBolt(t)
		wantShadow(t, "the master's combat", a.get(t, a.master)) // the positive control
		out, err := a.answerHeldRoll(t, a.bia, hold, typedDie(8))
		if err != nil {
			t.Fatalf("the player's answer: error = %v", err)
		}
		wantNoShadow(t, "the player's answer", out.GetSpellCast().GetEncounter(), out)
		wantCastSettled(t, a, out.GetSpellCast().GetCast())
	})
	t.Run("AnswerOutsideInspiration", func(t *testing.T) {
		t.Parallel()
		a, hold := mastersGuidingBolt(t)
		out, err := a.answerOutside(t, a.bia, hold, true, false, 8)
		if err != nil {
			t.Fatalf("the player's answer: error = %v", err)
		}
		wantNoShadow(t, "the player's answer", out.GetSpellCast().GetEncounter(), out)
		wantCastSettled(t, a, out.GetSpellCast().GetCast())
	})
}

// mastersEffectSave has the master roll the end-of-turn save of Hold Person for Toren, a halfling, on a 1: the save
// waits for Lucky.
func mastersEffectSave(t *testing.T) (a *armed, hold string) {
	t.Helper()
	a, e := shadowFight(t, "race:halfling", 3, false)
	a.mustAddEffect(t, e, holdPerson, []string{"Toren"}, a.rounds(t, 10, "Toren"))
	a.passTo(t, e, "Toren")
	if _, err := a.endTurnRaw(t, a.master, e, true); err != nil {
		t.Fatalf("EndTurn() error = %v", err)
	}
	w := a.windowOf(t, a.master, playv1.ReactionKind_REACTION_KIND_EFFECT_SAVE)
	if w == nil {
		t.Fatalf("the master's windows = %v, want the save of Hold Person", a.windows(t, a.master))
	}
	res, err := a.rollEffectSave(t, a.master, e, w.GetId(), effectSaveFace(1))
	if err != nil || res.GetResult() != nil || res.GetHeldRollOffer() == nil {
		t.Fatalf("RollEffectSave(master) = %v, %v; want Lucky's question alone", res, err)
	}
	return a, res.GetHeldRollOffer().GetHoldId()
}

// wantSaveSettled checks the roll the master's save makes, whoever answers it, and that its DC is not in the response.
func wantSaveSettled(t *testing.T, res *playv1.RollEffectSaveResponse) {
	t.Helper()
	if res.GetResult().GetD20() != 17 || len(res.GetResult().GetRerolled()) != 1 {
		t.Errorf("the save = %v, want the 1 rolled again to 17", res.GetResult())
	}
	if res.GetResult().Dc != nil {
		t.Errorf("the save carries the DC %d at a table that does not show it", res.GetResult().GetDc())
	}
}

// TestHeldAnswerRendersTheAnsweringMembersViewOfASave: the master's save for a halfling, answered by Lucky.
func TestHeldAnswerRendersTheAnsweringMembersViewOfASave(t *testing.T) {
	t.Parallel()
	t.Run("AnswerHeldRoll", func(t *testing.T) {
		t.Parallel()
		a, hold := mastersEffectSave(t)
		out, err := a.answerHeldRoll(t, a.caio, hold, luckyAnswer(17))
		if err != nil {
			t.Fatalf("the player's answer: error = %v", err)
		}
		wantNoShadow(t, "the player's answer", out.GetEffectSave().GetEncounter(), out)
		wantSaveSettled(t, out.GetEffectSave())
	})
	t.Run("AnswerOutsideLucky", func(t *testing.T) {
		t.Parallel()
		a, hold := mastersEffectSave(t)
		out, err := a.answerOutsideLucky(t, a.caio, hold, true, 17)
		if err != nil {
			t.Fatalf("the player's answer: error = %v", err)
		}
		wantNoShadow(t, "the player's answer", out.GetEffectSave().GetEncounter(), out)
		wantSaveSettled(t, out.GetEffectSave())
	})
}

// TestHeldAnswerRendersTheAnsweringMembersViewOfAGroupCheck: the master rolls for a halfling on a group check that
// does not show its DC; the answer carries the player's own roll and the table's rules for the DC.
func TestHeldAnswerRendersTheAnsweringMembersViewOfAGroupCheck(t *testing.T) {
	t.Parallel()
	a := newResourceTable(t)
	dalila, dalilaID := joinDalila(t, a.h, a.master, a.campaignID)
	const dc = 23
	view := a.mustRequestGroup(t, func(r *playv1.RequestGroupCheckRequest) { r.Dc = dc })
	a.mustRollGroup(t, a.caio, view.GetId(), 17)
	res, err := a.rollForPlayer(t, a.master, view.GetId(), dalilaID, 1)
	if err != nil || res.GetGroupCheck() != nil || res.GetHeldRollOffer() == nil {
		t.Fatalf("RollForPlayer() = %v, %v; want Lucky's question alone", res, err)
	}
	out, err := a.answerHeldRoll(t, dalila, res.GetHeldRollOffer().GetHoldId(), luckyAnswer(14))
	if err != nil {
		t.Fatalf("the player's answer: error = %v", err)
	}
	got := out.GetGroupCheck().GetGroupCheck()
	if len(got.GetMembers()) != 1 || got.GetMembers()[0].GetCharacterId() != dalilaID || got.GetDc() != 0 || got.GetVerdictKnown() {
		t.Errorf("the player's answer = %v, want her own roll alone, with no DC", got)
	}
	if r := memberOf(a.groupView(t, a.master), dalilaID).GetRoll(); r.GetFaces()[0] != 14 || len(r.GetRerolled()) != 1 || !r.GetRolledByMaster() {
		t.Errorf("the roll for Dalila = %v, want the 1 rolled again to 14, still the master's roll", r)
	}
}

// TestHeldAnswerByTheMasterRendersTheMastersView: the player's own held roll, answered by the master, comes back as
// the master's combat and the master's group check.
func TestHeldAnswerByTheMasterRendersTheMastersView(t *testing.T) {
	t.Parallel()
	t.Run("a cast", func(t *testing.T) {
		t.Parallel()
		a, e := shadowFight(t, "race:halfling", 3, true)
		a.h.roller.queue(9, 13)
		held, err := a.castGuidingBolt(t, e)
		if err != nil || held.GetHeldRollOffer() == nil {
			t.Fatalf("Brisa's Guiding Bolt = %v, %v; want the die's question", held, err)
		}
		a.h.afterThePlayersChance()
		out, err := a.answerHeldRoll(t, a.master, held.GetHeldRollOffer().GetHoldId(), dieAnswer(false))
		if err != nil {
			t.Fatalf("the master's answer: error = %v", err)
		}
		wantShadow(t, "the master's answer", out.GetSpellCast().GetEncounter())
		if got := out.GetSpellCast().GetCast().GetTargets()[0].GetAttackRoll().GetFaces()[0]; got != 9 {
			t.Errorf("the settled cast's d20 = %d, want 9", got)
		}
	})
	t.Run("a group check", func(t *testing.T) {
		t.Parallel()
		a := newResourceTable(t)
		dalila, dalilaID := joinDalila(t, a.h, a.master, a.campaignID)
		const dc = 23
		view := a.mustRequestGroup(t, func(r *playv1.RequestGroupCheckRequest) { r.Dc = dc })
		a.mustRollGroup(t, a.caio, view.GetId(), 17)
		held, err := a.rollGroup(t, dalila, view.GetId(), 1)
		if err != nil || held.GetHeldRollOffer() == nil {
			t.Fatalf("RollGroupCheck() = %v, %v; want Lucky's question alone", held, err)
		}
		a.h.afterThePlayersChance()
		out, err := a.answerHeldRoll(t, a.master, held.GetHeldRollOffer().GetHoldId(), luckyAnswer(14))
		if err != nil {
			t.Fatalf("the master's answer: error = %v", err)
		}
		got := out.GetGroupCheck().GetGroupCheck()
		if got.GetDc() != dc || len(got.GetMembers()) < 2 || memberOf(got, dalilaID) == nil {
			t.Errorf("the master's answer = %v, want the DC and every roll", got)
		}
	})
}

// TestHeldViewerIsTheCallerOfTheAnswer: the member a path renders for is the one who answered, and the roller
// otherwise.
func TestHeldViewerIsTheCallerOfTheAnswer(t *testing.T) {
	t.Parallel()
	roller := authz.Membership{CampaignID: "c1", UserID: "master", Role: authz.RoleMaster}
	caller := authz.Membership{CampaignID: "c1", UserID: "player", Role: authz.RolePlayer}
	cases := []struct {
		name string
		ctx  context.Context
		want authz.Membership
	}{
		{"no answer in the context", context.Background(), roller},
		{"an answer by the roller", withHeldReply(context.Background(), heldReply{}), roller},
		{"an answer by another", withHeldReply(context.Background(), heldReply{as: &roller, caller: &caller}), caller},
		{"an answer of another campaign", withHeldReply(context.Background(), heldReply{as: &roller, caller: &authz.Membership{CampaignID: "c2", Role: authz.RolePlayer}}), roller},
	}
	for _, tc := range cases {
		if got := heldViewer(tc.ctx, roller); got != tc.want {
			t.Errorf("%s: heldViewer() = %v, want %v", tc.name, got, tc.want)
		}
	}
	if heldViewerIsMaster(withHeldReply(context.Background(), heldReply{as: &roller, caller: &caller}), roller) {
		t.Error("heldViewerIsMaster() = true for a player's answer of the master's roll")
	}
}

// TestAConcentrationSaveAnswerAgainFollowsTheTableRules: the answer to a held concentration save, sent again with the
// same key, comes back as the first did, with the DC the table's rules show.
func TestAConcentrationSaveAnswerAgainFollowsTheTableRules(t *testing.T) {
	t.Parallel()
	a := newCastersOf(t, 3, "race:halfling")
	a.setRules(t, func(r *campaignsv1.TableRules) { r.ShowSaveDc = false })
	a, e := concentratingHitOn(t, a, 6)
	w := a.windowOf(t, a.ana, playv1.ReactionKind_REACTION_KIND_CONCENTRATION_SAVE)
	res, err := a.resolveConcentration(t, a.ana, e, w.GetId(), concentrationFace(1))
	hold := res.GetHeldRollOffer().GetHoldId()
	if err != nil || hold == "" {
		t.Fatalf("ResolveConcentrationSave(1) = %v, %v; want the question", res, err)
	}
	key := newKey()
	withKey := func(r *playv1.AnswerHeldRollRequest) { luckyAnswer(15)(r); r.IdempotencyKey = key }
	for _, which := range []string{"the first answer", "the same answer again"} {
		out, err := a.answerHeldRoll(t, a.ana, hold, withKey)
		if err != nil || out.GetConcentrationSave().GetResult() == nil {
			t.Fatalf("%s = %v, %v; want the result", which, out, err)
		}
		if dc := out.GetConcentrationSave().GetResult().GetDc(); dc != 0 {
			t.Errorf("%s: dc = %d, want 0 while the table does not show the DC of a save", which, dc)
		}
	}
}

// withAnswerKey gives the answer the key it is sent under, so the same answer can be sent again.
func withAnswerKey(key string, answer func(*playv1.AnswerHeldRollRequest)) func(*playv1.AnswerHeldRollRequest) {
	return func(r *playv1.AnswerHeldRollRequest) { answer(r); r.IdempotencyKey = key }
}

// labeled says whether the combat names a combatant by the label.
func labeled(e *playv1.Encounter, label string) bool {
	for _, c := range e.GetCombatants() {
		if c.GetLabel() == label {
			return true
		}
	}
	return false
}

// TestHeldAnswerGivenAgainRendersTheAnsweringMembersView: an answer sent again after the roll was settled gives the
// response the first one did, for the member who answers: the roller's membership writes and checks, and the answering
// member's is what the response is rendered for.
func TestHeldAnswerGivenAgainRendersTheAnsweringMembersView(t *testing.T) {
	t.Parallel()
	t.Run("a cast the master made, answered by the player", func(t *testing.T) {
		t.Parallel()
		a, hold := mastersGuidingBolt(t)
		key := newKey()
		answer := withAnswerKey(key, typedDie(8))
		first, err := a.answerHeldRoll(t, a.bia, hold, answer)
		if err != nil {
			t.Fatalf("the player's answer: error = %v", err)
		}
		wantCastSettled(t, a, first.GetSpellCast().GetCast())
		// The Goblin is hidden since: a response built for the master would still name it.
		e := a.get(t, a.master)
		a.castMasterHides(t, e, "Goblin")
		goblin := a.id(t, "Goblin")
		again, err := a.answerHeldRoll(t, a.bia, hold, answer)
		if err != nil {
			t.Fatalf("the same answer again: error = %v", err)
		}
		got := again.GetSpellCast()
		wantNoShadow(t, "the answer again", got.GetEncounter(), again)
		if labeled(got.GetEncounter(), "Goblin") {
			t.Error("the answer again carries the combat as the master reads it, want the player's")
		}
		for _, tr := range got.GetCast().GetTargets() {
			if tr.GetCombatantId() == goblin {
				t.Errorf("the answer again names the creature hidden since: %v", got.GetCast())
			}
		}
		if len(got.GetHiddenHits()) != 0 || got.GetPendingRevealId() != "" {
			t.Errorf("the answer again = hidden hits %v, pending %q; want the player's", got.GetHiddenHits(), got.GetPendingRevealId())
		}
		// The master's write is the first answer's: one action spent, the d20 and the die of the first.
		if !byLabel(t, a.get(t, a.master), "Brisa").GetActionUsed() {
			t.Error("the action is not spent after the answer again")
		}
	})
	t.Run("a cast the player made, answered by the master", func(t *testing.T) {
		t.Parallel()
		a, e := shadowFight(t, "race:halfling", 3, true)
		a.h.roller.queue(9, 13)
		held, err := a.castGuidingBolt(t, e)
		if err != nil || held.GetHeldRollOffer() == nil {
			t.Fatalf("Brisa's Guiding Bolt = %v, %v; want the die's question", held, err)
		}
		a.h.afterThePlayersChance()
		key := newKey()
		answer := withAnswerKey(key, dieAnswer(false))
		hold := held.GetHeldRollOffer().GetHoldId()
		for _, which := range []string{"the first answer", "the same answer again"} {
			out, err := a.answerHeldRoll(t, a.master, hold, answer)
			if err != nil {
				t.Fatalf("%s: error = %v", which, err)
			}
			wantShadow(t, which, out.GetSpellCast().GetEncounter())
			if got := out.GetSpellCast().GetCast().GetTargets()[0].GetAttackRoll().GetFaces()[0]; got != 9 {
				t.Errorf("%s: the cast's d20 = %d, want 9", which, got)
			}
		}
	})
	t.Run("a save the master rolled, answered by Lucky", func(t *testing.T) {
		t.Parallel()
		a, hold := mastersEffectSave(t)
		answer := withAnswerKey(newKey(), luckyAnswer(17))
		for _, which := range []string{"the first answer", "the same answer again"} {
			out, err := a.answerHeldRoll(t, a.caio, hold, answer)
			if err != nil {
				t.Fatalf("%s: error = %v", which, err)
			}
			wantNoShadow(t, which, out.GetEffectSave().GetEncounter(), out)
			wantSaveSettled(t, out.GetEffectSave())
		}
	})
	t.Run("a group check the master rolled, answered by the player", func(t *testing.T) {
		t.Parallel()
		a := newResourceTable(t)
		dalila, dalilaID := joinDalila(t, a.h, a.master, a.campaignID)
		const dc = 23
		view := a.mustRequestGroup(t, func(r *playv1.RequestGroupCheckRequest) { r.Dc = dc })
		a.mustRollGroup(t, a.caio, view.GetId(), 17)
		res, err := a.rollForPlayer(t, a.master, view.GetId(), dalilaID, 1)
		if err != nil || res.GetHeldRollOffer() == nil {
			t.Fatalf("RollForPlayer() = %v, %v; want Lucky's question alone", res, err)
		}
		answer := withAnswerKey(newKey(), luckyAnswer(14))
		for _, which := range []string{"the first answer", "the same answer again"} {
			out, err := a.answerHeldRoll(t, dalila, res.GetHeldRollOffer().GetHoldId(), answer)
			if err != nil {
				t.Fatalf("%s: error = %v", which, err)
			}
			got := out.GetGroupCheck().GetGroupCheck()
			if len(got.GetMembers()) != 1 || got.GetMembers()[0].GetCharacterId() != dalilaID || got.GetDc() != 0 || got.GetVerdictKnown() {
				t.Errorf("%s = %v, want her own roll alone, with no DC", which, got)
			}
		}
	})
}
