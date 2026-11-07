package play

import (
	"strings"
	"testing"

	"connectrpc.com/connect"

	playv1 "github.com/PuraFome/meuRPG/backend/gen/meurpg/play/v1"
)

// Parry (SRD 5.1, the Reactions of the Bandit Captain, Erinyes, Gladiator, Knight, Marilith and Noble; RN-31,
// RN-20, RN-29): a melee weapon attack that hits a creature with the reaction (switched to "Perguntar") waits for the master, who answers
// for the creature. These tests need the database (MEURPG_TEST_DATABASE_URL). Toren attacks with +6 to hit.

const (
	knightKey = "monster:knight"
	knight    = "Cavaleiro"
	// knightHit is the d20 that makes Toren's total (face + 6) equal the Knight's armor class 18: a hit that
	// Parry's +2 turns into a miss.
	knightHit = 12
)

var parryKind = playv1.ReactionKind_REACTION_KIND_PARRY

// parryFight is Toren (a battleaxe, a dagger and a shortbow) on turn beside the Knight, one square away, with the
// master having asked to answer the creature's Parry (the window of these tests). autoParryFight leaves the mode
// as it is by default: the server decides.
func parryFight(t *testing.T, key, label string) (*armed, *playv1.Encounter) {
	t.Helper()
	a, e := autoParryFight(t, key, label)
	a.setParryAsk(t, e, label, true)
	return a, e
}

// autoParryFight is parryFight with the creature's Parry in its default mode.
func autoParryFight(t *testing.T, key, label string) (*armed, *playv1.Encounter) {
	t.Helper()
	return monsterFightOn(t, newCastersGear(t, 3, "", []string{battleaxe, daggerKey, shortbowKey}), key, label)
}

// setParryAsk is the master's SetMonsterReactionMode on the creature.
func (a *armed) setParryAsk(t *testing.T, e *playv1.Encounter, label string, ask bool) {
	t.Helper()
	if _, err := a.parryMode(t, a.master, e, label, ask); err != nil {
		t.Fatalf("SetMonsterReactionMode(%s, ask %v) error = %v", label, ask, err)
	}
}

func (a *armed) parryMode(t *testing.T, u *user, e *playv1.Encounter, label string, ask bool) (*connect.Response[playv1.SetMonsterReactionModeResponse], error) {
	t.Helper()
	return u.combat.SetMonsterReactionMode(t.Context(), connect.NewRequest(&playv1.SetMonsterReactionModeRequest{
		CampaignId: a.campaignID, EncounterId: e.GetId(), CombatantId: a.id(t, label), IdempotencyKey: newKey(), Ask: ask,
	}))
}

func (a *armed) pendingStatus(t *testing.T, id string) string {
	t.Helper()
	var s string
	if err := a.h.pool.QueryRow(t.Context(), `SELECT status FROM pending_damages WHERE id = $1`, id).Scan(&s); err != nil {
		t.Fatalf("read the pending damage: %v", err)
	}
	return s
}

// logEntries are the lines u reads of the combat's log, the rounds flattened.
func (a *armed) logEntries(t *testing.T, u *user, e *playv1.Encounter) []*playv1.CombatLogEntry {
	t.Helper()
	var out []*playv1.CombatLogEntry
	for _, r := range a.log(t, u, e).GetRounds() {
		out = append(out, r.GetEntries()...)
	}
	return out
}

func (a *armed) reactionUsedBy(t *testing.T, label string) bool {
	t.Helper()
	var used bool
	if err := a.h.pool.QueryRow(t.Context(), `SELECT reaction_used FROM combatants WHERE id = $1`, a.id(t, label)).Scan(&used); err != nil {
		t.Fatalf("read the reaction: %v", err)
	}
	return used
}

// parriedHit is Toren's hit on the Knight that waits for the master's Parry.
func parriedHit(t *testing.T, face int32) (*armed, *playv1.Encounter, *playv1.PendingDamage, *playv1.ReactionWindow) {
	t.Helper()
	a, e := parryFight(t, knightKey, knight)
	hit := a.mustAttack(t, a.caio, e, "Toren", battleaxe, knight, d20(face))
	w := a.windowOf(t, a.master, parryKind)
	if w == nil || hit.GetPendingDamage().GetStatus() != playv1.PendingDamageStatus_PENDING_DAMAGE_STATUS_AWAITING_REACTION {
		t.Fatalf("Toren's hit on the Knight = %v, window %v; want a hit that waits for Parry", hit.GetPendingDamage(), w)
	}
	return a, e, hit.GetPendingDamage(), w
}

// A hit that Parry turns into a miss: the master is asked, with the numbers; he uses it, the kept d20 is compared
// with the armor class plus 2 and the damage is never applied. Nothing is rolled again.
func TestParry_UsedTurnsAHitIntoAMiss(t *testing.T) {
	t.Parallel()
	a, e, hit, w := parriedHit(t, knightHit)
	tr := w.GetTrigger()
	if tr.GetAttackTotal() != 18 || tr.GetTargetArmorClass() != 18 || tr.GetActorLabel() != "Toren" || w.GetReactorLabel() != knight || !w.GetAnswerNow() {
		t.Errorf("the master's window = %v, want the Knight's Parry to answer now, 18 against armor class 18", w)
	}
	if p := w.GetParry(); p.GetAcBonus() != 2 || p.GetArmorClassWithParry() != 20 || p.GetAttackerLabel() != "Toren" || p.GetPendingDamageId() != hit.GetId() {
		t.Errorf("the master's prompt = %v, want +2 (armor class 20) against Toren's hit", p)
	}
	asked := a.h.roller.taken()
	res := a.mustAnswer(t, a.master, e, w.GetId(), useAnswer(0))
	if got := a.h.roller.taken(); got != asked {
		t.Errorf("the answer rolled %d dice, want none: the d20 is kept", got-asked)
	}
	if r := res.GetResult(); r.GetKind() != parryKind || !r.GetUsed() || !r.GetParry().GetStopped() || !r.GetByMaster() {
		t.Errorf("the answer = %v, want Parry used and the hit stopped", r)
	}
	if got := a.pendingStatus(t, hit.GetId()); got != "discarded" {
		t.Errorf("the hit's damage is %q after Parry, want discarded: a miss", got)
	}
	if !a.reactionUsedBy(t, knight) {
		t.Error("the Knight's reaction is not spent after Parry")
	}
	if w := a.windowOf(t, a.master, parryKind); w != nil {
		t.Errorf("the window %v is still open after the answer", w)
	}
}

// "Deixar passar": the hit lands as it was, the reaction is not spent, and the damage can be rolled.
func TestParry_PassLetsTheHitLand(t *testing.T) {
	t.Parallel()
	a, e, hit, w := parriedHit(t, knightHit)
	res := a.mustAnswer(t, a.master, e, w.GetId(), passAnswer)
	if res.GetResult().GetUsed() || res.GetResult().GetKind() != parryKind {
		t.Errorf("the answer = %v, want Parry passed", res.GetResult())
	}
	if got := a.pendingStatus(t, hit.GetId()); got != "awaiting_roll" {
		t.Errorf("the hit's damage is %q after the pass, want awaiting_roll", got)
	}
	if a.reactionUsedBy(t, knight) {
		t.Error("the Knight's reaction is spent by a pass")
	}
	if _, err := a.damage(t, a.caio, e, hit.GetId(), inAppDamage); err != nil {
		t.Errorf("RollDamage() after the pass error = %v", err)
	}
}

// A Parry that does not turn the hit (the roll is far over the armor class) is spent all the same, and the hit
// goes on. The window opened for it, since a pause that skipped such hits would tell the table the margin (RN-10).
func TestParry_AHugeMarginIsOfferedAndStillHits(t *testing.T) {
	t.Parallel()
	a, e, hit, w := parriedHit(t, 19) // 25 against 18, and 20 with Parry
	res := a.mustAnswer(t, a.master, e, w.GetId(), useAnswer(0))
	if res.GetResult().GetParry().GetStopped() || !res.GetResult().GetUsed() {
		t.Errorf("the answer = %v, want Parry used and the hit not stopped", res.GetResult())
	}
	if got := a.pendingStatus(t, hit.GetId()); got != "awaiting_roll" {
		t.Errorf("the hit's damage is %q, want awaiting_roll", got)
	}
	if !a.reactionUsedBy(t, knight) {
		t.Error("the reaction is not spent by a Parry that did not matter")
	}
}

// A hit that Parry would not meet (a natural 20 hits whatever the armor class, as the table's Shield rule keeps) opens no window.
func TestParry_ACriticalHitOpensNoWindow(t *testing.T) {
	t.Parallel()
	a, e := parryFight(t, knightKey, knight)
	hit := a.mustAttack(t, a.caio, e, "Toren", battleaxe, knight, d20(20))
	if w := a.windowOf(t, a.master, parryKind); w != nil || hit.GetPendingDamage().GetStatus() != playv1.PendingDamageStatus_PENDING_DAMAGE_STATUS_AWAITING_ROLL {
		t.Errorf("a critical hit = window %v, damage %v; want no window and the damage to roll", w, hit.GetPendingDamage().GetStatus())
	}
}

// One reaction a round: after a Parry the second attack of the same turn (Extra Attack) lands with no window; the
// reaction is back when the Knight's own turn starts, and the next round's hit is offered again.
func TestParry_OnePerRoundAndItReturnsAtTheCreaturesTurn(t *testing.T) {
	t.Parallel()
	a, e, _, w := parriedHit(t, knightHit)
	a.mustAnswer(t, a.master, e, w.GetId(), useAnswer(0))
	second := a.mustAttack(t, a.caio, e, "Toren", battleaxe, knight, d20(knightHit))
	if a.windowOf(t, a.master, parryKind) != nil || second.GetPendingDamage().GetStatus() != playv1.PendingDamageStatus_PENDING_DAMAGE_STATUS_AWAITING_ROLL {
		t.Fatalf("the second hit of the round = %v, window %v; want no window and the damage to roll", second.GetPendingDamage().GetStatus(), a.windowOf(t, a.master, parryKind))
	}
	a.mustDamage(t, a.caio, e, second.GetPendingDamage().GetId(), inAppDamage)

	e = a.passTo(t, e, knight)
	if a.reactionUsedBy(t, knight) {
		t.Error("the Knight's reaction is still spent at the start of its own turn")
	}
	e = a.passTo(t, e, "Toren")
	a.mustAttack(t, a.caio, e, "Toren", battleaxe, knight, d20(knightHit))
	if a.windowOf(t, a.master, parryKind) == nil {
		t.Error("the next round's hit is not offered Parry: the reaction did not come back")
	}
}

// What takes the reaction away takes the window: a creature that cannot act, cannot see the attacker or has
// no Parry gets none, and a ranged or spell attack is not a melee weapon attack.
func TestParry_NoWindowWhenTheReactionCannotBeUsed(t *testing.T) {
	t.Parallel()
	for _, c := range []struct {
		name string
		prep func(a *armed, e *playv1.Encounter)
		roll func(*playv1.RollAttackRequest)
	}{
		{"the creature is stunned", func(a *armed, e *playv1.Encounter) { a.setConditions(t, e, knight, "condition:stunned") }, advantage(knightHit)},
		{"the creature is incapacitated", func(a *armed, e *playv1.Encounter) { a.setConditions(t, e, knight, "condition:incapacitated") }, d20(knightHit)},
		{"the creature is blinded", func(a *armed, e *playv1.Encounter) { a.setConditions(t, e, knight, "condition:blinded") }, advantage(knightHit)},
		{"the attacker is invisible", func(a *armed, e *playv1.Encounter) { a.setConditions(t, e, "Toren", "condition:invisible") }, advantage(knightHit)},
		{"the reaction is already spent", func(a *armed, _ *playv1.Encounter) {
			a.execSQL(t, `UPDATE combatants SET reaction_used = true WHERE id = $1`, a.id(t, knight))
		}, d20(knightHit)},
	} {
		t.Run(c.name, func(t *testing.T) {
			t.Parallel()
			a, e := parryFight(t, knightKey, knight)
			c.prep(a, e)
			hit := a.mustAttack(t, a.caio, e, "Toren", battleaxe, knight, c.roll)
			if w := a.windowOf(t, a.master, parryKind); w != nil || hit.GetPendingDamage().GetStatus() != playv1.PendingDamageStatus_PENDING_DAMAGE_STATUS_AWAITING_ROLL {
				t.Errorf("hit = window %v, damage %v; want no window and the damage to roll", w, hit.GetPendingDamage().GetStatus())
			}
		})
	}
}

func TestParry_OnlyAMeleeWeaponAttackIsParried(t *testing.T) {
	t.Parallel()
	a, e := parryFight(t, knightKey, knight)
	hit := a.mustAttack(t, a.caio, e, "Toren", shortbowKey, knight, disadvantage(knightHit+1)) // a ranged weapon attack at 5 ft
	if w := a.windowOf(t, a.master, parryKind); w != nil || hit.GetPendingDamage().GetStatus() != playv1.PendingDamageStatus_PENDING_DAMAGE_STATUS_AWAITING_ROLL {
		t.Errorf("a shortbow hit = window %v, damage %v; want no window", w, hit.GetPendingDamage().GetStatus())
	}
	// A spell attack is no weapon attack either.
	e = a.passTo(t, e, "Pensantus")
	bolt := a.mustAttack(t, a.ana, e, "Pensantus", fireBolt, knight, d20(15)) // 20 against 18: a hit
	if bolt.GetPendingDamage().GetStatus() != playv1.PendingDamageStatus_PENDING_DAMAGE_STATUS_AWAITING_ROLL {
		t.Errorf("Fire Bolt's hit = %v, want the damage to roll", bolt.GetPendingDamage().GetStatus())
	}
	if w := a.windowOf(t, a.master, parryKind); w != nil {
		t.Errorf("a spell attack opened %v, want no Parry window", w)
	}
}

// A dagger thrown from beyond the reach is a ranged attack, at 5 ft it is a melee one.
func TestParry_AThrownWeaponIsMeleeOnlyInReach(t *testing.T) {
	t.Parallel()
	a, e := parryFight(t, knightKey, knight)
	a.mustAttack(t, a.caio, e, "Toren", daggerKey, knight, d20(knightHit))
	if a.windowOf(t, a.master, parryKind) == nil {
		t.Error("a dagger used in reach opened no Parry window")
	}
}

// The same dagger thrown from beyond the reach is a ranged attack: its hit waits for no Parry.
func TestParry_AThrownWeaponBeyondReachOpensNoWindow(t *testing.T) {
	t.Parallel()
	a, e := parryFight(t, knightKey, knight)
	k := byLabel(t, e, knight)
	a.put(t, "Toren", k.GetCol()-3, k.GetRow())                                     // 15 ft from the Knight
	hit := a.mustAttack(t, a.caio, e, "Toren", daggerKey, knight, d20(knightHit+2)) // +2: the half cover the map gives at 15 ft
	if w := a.windowOf(t, a.master, parryKind); w != nil || hit.GetPendingDamage().GetStatus() != playv1.PendingDamageStatus_PENDING_DAMAGE_STATUS_AWAITING_ROLL {
		t.Errorf("a dagger thrown from 15 ft = window %v, hit %v; want a hit with no window", w, hit.GetRoll())
	}
}

// A creature whose stat block has no Parry opens no window.
func TestParry_ACreatureWithoutTheReactionHasNone(t *testing.T) {
	t.Parallel()
	a, e := autoParryFight(t, "monster:bandit", "Bandido")                    // no Parry: the mode cannot be set either
	hit := a.mustAttack(t, a.caio, e, "Toren", battleaxe, "Bandido", d20(14)) // 20 against the Bandit's armor class 12
	if w := a.windowOf(t, a.master, parryKind); w != nil || hit.GetPendingDamage().GetStatus() != playv1.PendingDamageStatus_PENDING_DAMAGE_STATUS_AWAITING_ROLL {
		t.Errorf("a hit on a Bandit = window %v, damage %v; want no window", w, hit.GetPendingDamage().GetStatus())
	}
}

// Every SRD stat block with Parry offers it with its own number, and the hit that Parry turns is stopped.
func TestParry_EveryStatBlockWithTheReaction(t *testing.T) {
	t.Parallel()
	for _, c := range []struct {
		key, label    string
		ac, bonus     int32
		attackerFaces int32
	}{
		{"monster:bandit-captain", "Capitão bandido", 15, 2, 9},
		{"monster:erinyes", "Erínia", 18, 4, 12},
		{"monster:gladiator", "Gladiador", 16, 3, 10},
		{"monster:knight", "Cavaleiro", 18, 2, 12},
		{"monster:marilith", "Marilith", 18, 5, 12},
		{"monster:noble", "Nobre", 15, 2, 9},
	} {
		t.Run(c.key, func(t *testing.T) {
			t.Parallel()
			a, e := parryFight(t, c.key, c.label)
			a.mustAttack(t, a.caio, e, "Toren", battleaxe, c.label, d20(c.attackerFaces))
			w := a.windowOf(t, a.master, parryKind)
			if w == nil || w.GetTrigger().GetTargetArmorClass() != c.ac || w.GetParry().GetAcBonus() != c.bonus || w.GetParry().GetArmorClassWithParry() != c.ac+c.bonus {
				t.Fatalf("the window = %v, want armor class %d and Parry +%d", w, c.ac, c.bonus)
			}
			if r := a.mustAnswer(t, a.master, e, w.GetId(), useAnswer(0)).GetResult(); !r.GetParry().GetStopped() {
				t.Errorf("the answer = %v, want the hit (equal to the armor class) stopped", r)
			}
		})
	}
}

// Only the master answers: a player is refused as for a window that never existed (RN-10).
func TestParry_APlayerCannotAnswer(t *testing.T) {
	t.Parallel()
	a, e, _, w := parriedHit(t, knightHit)
	_, errReal := a.answerReaction(t, a.caio, e, w.GetId(), useAnswer(0))
	_, errFake := a.answerReaction(t, a.caio, e, "00000000-0000-4000-8000-000000000001", useAnswer(0))
	wantCode(t, "Toren's player answering Parry", errReal, connect.CodePermissionDenied)
	wantCode(t, "Toren's player answering a window that does not exist", errFake, connect.CodePermissionDenied)
	if errReal.Error() != errFake.Error() {
		t.Errorf("a real window is refused with %q, an invented one with %q: they must read the same", errReal, errFake)
	}
	if a.reactionUsedBy(t, knight) {
		t.Error("a refused answer spent the reaction")
	}
}

// The same key answers once; a second answer with another key is told it came late.
func TestParry_AnAnswerRetriedWithTheSameKeyCountsOnce(t *testing.T) {
	t.Parallel()
	a, e, _, w := parriedHit(t, knightHit)
	key := newKey()
	req := func() *connect.Request[playv1.AnswerReactionRequest] {
		return connect.NewRequest(&playv1.AnswerReactionRequest{
			CampaignId: a.campaignID, EncounterId: e.GetId(), WindowId: w.GetId(), IdempotencyKey: key, Answer: playv1.ReactionChoice_REACTION_CHOICE_USE,
		})
	}
	events := a.events(t)
	first, err := a.master.combat.AnswerReaction(t.Context(), req())
	if err != nil {
		t.Fatalf("AnswerReaction() error = %v", err)
	}
	again, err := a.master.combat.AnswerReaction(t.Context(), req())
	if err != nil {
		t.Fatalf("AnswerReaction() again error = %v", err)
	}
	if !first.Msg.GetResult().GetParry().GetStopped() || again.Msg.GetEncounter().GetRevision() != first.Msg.GetEncounter().GetRevision() {
		t.Errorf("the retry answered revision %d, the first %d (%v); want the same combat", again.Msg.GetEncounter().GetRevision(), first.Msg.GetEncounter().GetRevision(), first.Msg.GetResult())
	}
	if got := a.events(t); got != events+1 {
		t.Errorf("%d events after a retry, want one more than before (%d)", got, events+1)
	}
	_, err = a.answerReaction(t, a.master, e, w.GetId(), useAnswer(0))
	wantBlockedBy(t, "an answer to a window answered", err, playv1.EncounterBlockedReason_ENCOUNTER_BLOCKED_REASON_REACTION_ALREADY_ANSWERED)
}

// The master's Desfazer takes the Parry back: the reaction returns, the window opens again and the hit waits.
func TestParry_UndoGivesTheReactionBack(t *testing.T) {
	t.Parallel()
	a, e, hit, w := parriedHit(t, knightHit)
	a.mustAnswer(t, a.master, e, w.GetId(), useAnswer(0))
	a.undoLast(t, e)
	if a.reactionUsedBy(t, knight) {
		t.Error("the Knight's reaction is still spent after the undo")
	}
	if got := a.pendingStatus(t, hit.GetId()); got != "awaiting_reaction" {
		t.Errorf("the hit is %q after the undo, want awaiting_reaction", got)
	}
	again := a.windowOf(t, a.master, parryKind)
	if again == nil || again.GetId() != w.GetId() {
		t.Fatalf("the window after the undo = %v, want %s open again", again, w.GetId())
	}
	if r := a.mustAnswer(t, a.master, e, again.GetId(), useAnswer(0)).GetResult(); !r.GetParry().GetStopped() {
		t.Errorf("Parry after the undo = %v, want the hit stopped again", r)
	}
}

// A pass is an answer, not an action: like the Shield's, it leaves nothing for the undo, and the hit goes on.
func TestParry_APassIsNotUndoable(t *testing.T) {
	t.Parallel()
	a, e, _, w := parriedHit(t, knightHit)
	a.mustAnswer(t, a.master, e, w.GetId(), passAnswer)
	if id := a.log(t, a.master, e).GetUndoableEventId(); id != "" {
		t.Errorf("the undo offers %s after a pass, want nothing", id)
	}
}

// What the players read: the wait (the same words as for any NPC that may react), no window, no prompt; and
// after the answer an ordinary miss in the log with no word of Parry, no armor class and no reaction line.
func TestParry_WhatThePlayersRead(t *testing.T) {
	t.Parallel()
	a, e, _, w := parriedHit(t, knightHit)
	for name, u := range map[string]*user{"Toren's player": a.caio, "Pensantus's player": a.ana} {
		got := a.get(t, u)
		if len(got.GetReactionWindows()) != 0 || len(got.GetReactionPrompts()) != 0 || got.GetReactionWait().GetTitlePt() != "Esperando o mestre" {
			t.Errorf("%s reads windows %v prompts %v wait %v, want only \"Esperando o mestre\"", name, got.GetReactionWindows(), got.GetReactionPrompts(), got.GetReactionWait())
		}
	}
	a.mustAnswer(t, a.master, e, w.GetId(), useAnswer(0))
	for name, u := range map[string]*user{"Toren's player": a.caio, "Pensantus's player": a.ana} {
		entries := a.logEntries(t, u, e)
		for _, en := range entries {
			if en.GetKind() == playv1.CombatLogKind_COMBAT_LOG_KIND_REACTION_WINDOW || en.GetKind() == playv1.CombatLogKind_COMBAT_LOG_KIND_REACTION || en.GetReactionTextPt() != "" {
				t.Errorf("%s got the reaction line %v", name, en)
			}
			if en.GetKind() == playv1.CombatLogKind_COMBAT_LOG_KIND_ATTACK && (en.GetOutcome() != playv1.AttackOutcome_ATTACK_OUTCOME_MISS || en.GetStoppedByReaction() || en.TargetArmorClass != nil) {
				t.Errorf("%s's attack line = %v, want a plain miss with no armor class", name, en)
			}
		}
	}
	var line string
	var attack *playv1.CombatLogEntry
	for _, en := range a.logEntries(t, a.master, e) {
		if en.GetKind() == playv1.CombatLogKind_COMBAT_LOG_KIND_REACTION_WINDOW {
			line = en.GetReactionTextPt()
		}
		if en.GetKind() == playv1.CombatLogKind_COMBAT_LOG_KIND_ATTACK {
			attack = en
		}
	}
	if !strings.Contains(line, "Cavaleiro aparou (+2 CA)") {
		t.Errorf("the master's line = %q, want \"Cavaleiro aparou (+2 CA)\"", line)
	}
	if attack == nil || attack.GetOutcome() != playv1.AttackOutcome_ATTACK_OUTCOME_MISS || !attack.GetStoppedByReaction() {
		t.Errorf("the master's attack line = %v, want a miss stopped by a reaction", attack)
	}
}
