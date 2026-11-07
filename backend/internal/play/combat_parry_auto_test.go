package play

import (
	"strings"
	"testing"

	"connectrpc.com/connect"

	playv1 "github.com/PuraFome/meuRPG/backend/gen/meurpg/play/v1"
)

// An NPC's Parry is decided by the server, with no window (RN-10, RN-31). These tests need the database
// (MEURPG_TEST_DATABASE_URL). Toren attacks with +6 to hit; the Knight's armor class is 18 and its Parry adds 2.

const (
	// knightLine is what the master reads when the Knight parried a hit that Parry turned into a miss.
	knightLine = "Cavaleiro aparou (+2 CA): 18 contra CA 20, o ataque errou."
	// knightPlayersLine is what the players read of it.
	knightPlayersLine = "Cavaleiro aparou o ataque."
)

// reactionLines are the lines of reactions u reads of the combat's log.
func (a *armed) reactionLines(t *testing.T, u *user, e *playv1.Encounter) []string {
	t.Helper()
	var out []string
	for _, en := range a.logEntries(t, u, e) {
		if en.GetKind() == playv1.CombatLogKind_COMBAT_LOG_KIND_REACTION_WINDOW && en.GetReactionTextPt() != "" {
			out = append(out, en.GetReactionTextPt())
		}
	}
	return out
}

// pendingCount is how many pending damages the combat holds.
func (a *armed) pendingCount(t *testing.T) int {
	t.Helper()
	var n int
	if err := a.h.pool.QueryRow(t.Context(), `SELECT count(*) FROM pending_damages`).Scan(&n); err != nil {
		t.Fatalf("count the pending damages: %v", err)
	}
	return n
}

// wantNoWait fails if u reads a reaction window, a prompt or a wait in the combat.
func (a *armed) wantNoWait(t *testing.T, u *user, who string) {
	t.Helper()
	got := a.get(t, u)
	if len(got.GetReactionWindows()) != 0 || len(got.GetReactionPrompts()) != 0 || got.GetReactionWait() != nil {
		t.Errorf("%s reads windows %v prompts %v wait %v, want none", who, got.GetReactionWindows(), got.GetReactionPrompts(), got.GetReactionWait())
	}
}

// A hit that Parry's +2 turns into a miss is a miss at once: the reaction is spent, no damage opens, no window waits,
// and the answer to the player reads as any miss. The master reads the numbers, the players that the Knight parried.
func TestParryAuto_TurnsAHitIntoAMissWithNoWindow(t *testing.T) {
	t.Parallel()
	a, e := autoParryFight(t, knightKey, knight)
	hit := a.mustAttack(t, a.caio, e, "Toren", battleaxe, knight, d20(knightHit))
	if hit.GetRoll().GetOutcome() != playv1.AttackOutcome_ATTACK_OUTCOME_MISS || hit.GetPendingDamage() != nil {
		t.Errorf("Toren's attack = %v / %v, want a miss with no pending damage", hit.GetRoll(), hit.GetPendingDamage())
	}
	if hit.GetRoll().TargetArmorClass != nil {
		t.Errorf("a player reads the armor class %d", hit.GetRoll().GetTargetArmorClass())
	}
	if !a.reactionUsedBy(t, knight) {
		t.Error("the Knight's reaction is not spent by the Parry")
	}
	if n := a.pendingCount(t); n != 0 {
		t.Errorf("the combat holds %d pending damages, want none: a miss", n)
	}
	for name, u := range map[string]*user{"the master": a.master, "Toren's player": a.caio, "Pensantus's player": a.ana} {
		a.wantNoWait(t, u, name)
		if w := a.windowOf(t, u, parryKind); w != nil {
			t.Errorf("%s reads the window %v", name, w)
		}
	}
	if got := a.reactionLines(t, a.master, e); len(got) != 1 || got[0] != knightLine {
		t.Errorf("the master's reaction lines = %q, want %q", got, knightLine)
	}
	for name, u := range map[string]*user{"Toren's player": a.caio, "Pensantus's player": a.ana} {
		if got := a.reactionLines(t, u, e); len(got) != 1 || got[0] != knightPlayersLine {
			t.Errorf("%s's reaction lines = %q, want %q", name, got, knightPlayersLine)
		}
		for _, en := range a.logEntries(t, u, e) {
			if en.GetKind() == playv1.CombatLogKind_COMBAT_LOG_KIND_ATTACK && (en.GetOutcome() != playv1.AttackOutcome_ATTACK_OUTCOME_MISS || en.GetStoppedByReaction() || en.TargetArmorClass != nil) {
				t.Errorf("%s's attack line = %v, want a plain miss with no armor class", name, en)
			}
		}
	}
}

// The recorded reaction is the Parry's, in the round it was used.
func TestParryAuto_RecordsTheReaction(t *testing.T) {
	t.Parallel()
	a, e := autoParryFight(t, knightKey, knight)
	a.wantNoRecord(t, knight, "before Parry")
	a.mustAttack(t, a.caio, e, "Toren", battleaxe, knight, d20(knightHit))
	round := a.get(t, a.master).GetRound()
	a.wantRecord(t, knight, "parry", round)
	a.wantViewRecord(t, a.master, knight, playv1.ReactionKind_REACTION_KIND_PARRY, round)
	for _, u := range []*user{a.ana, a.bia, a.caio} {
		a.wantNoViewRecord(t, u, knight)
	}
}

// A Parry that would not turn the hit does nothing: the hit goes on to its damage, the reaction stays free, nobody
// reads a line of it and nothing waits.
func TestParryAuto_NothingHappensWhenTheBonusDoesNotMatter(t *testing.T) {
	t.Parallel()
	a, e := autoParryFight(t, knightKey, knight)
	hit := a.mustAttack(t, a.caio, e, "Toren", battleaxe, knight, d20(19)) // 25 against 18, and 20 with Parry
	if hit.GetRoll().GetOutcome() != playv1.AttackOutcome_ATTACK_OUTCOME_HIT || hit.GetPendingDamage().GetStatus() != playv1.PendingDamageStatus_PENDING_DAMAGE_STATUS_AWAITING_ROLL {
		t.Fatalf("Toren's attack = %v / %v, want a hit whose damage waits for its roll", hit.GetRoll(), hit.GetPendingDamage())
	}
	if a.reactionUsedBy(t, knight) {
		t.Error("the Knight's reaction is spent by a Parry that did not matter")
	}
	for name, u := range map[string]*user{"the master": a.master, "Toren's player": a.caio, "Pensantus's player": a.ana} {
		a.wantNoWait(t, u, name)
		if got := a.reactionLines(t, u, e); len(got) != 0 {
			t.Errorf("%s reads the reaction lines %q, want none", name, got)
		}
	}
	a.mustDamage(t, a.caio, e, hit.GetPendingDamage().GetId(), inAppDamage)
}

// A roll under the armor class is an ordinary miss: the reaction stays free.
func TestParryAuto_AnOrdinaryMissSpendsNothing(t *testing.T) {
	t.Parallel()
	a, e := autoParryFight(t, knightKey, knight)
	hit := a.mustAttack(t, a.caio, e, "Toren", battleaxe, knight, d20(knightHit-1)) // 17 against 18
	if hit.GetRoll().GetOutcome() != playv1.AttackOutcome_ATTACK_OUTCOME_MISS || a.reactionUsedBy(t, knight) {
		t.Errorf("a miss = %v, reaction spent %v; want a miss and a free reaction", hit.GetRoll().GetOutcome(), a.reactionUsedBy(t, knight))
	}
	if got := a.reactionLines(t, a.master, e); len(got) != 0 {
		t.Errorf("the master reads %q for an ordinary miss, want none", got)
	}
}

// A critical hit hits whatever the armor class: it is not parried and the reaction stays free.
func TestParryAuto_ACriticalHitIsNotParried(t *testing.T) {
	t.Parallel()
	a, e := autoParryFight(t, knightKey, knight)
	hit := a.mustAttack(t, a.caio, e, "Toren", battleaxe, knight, d20(20))
	if hit.GetRoll().GetOutcome() != playv1.AttackOutcome_ATTACK_OUTCOME_CRITICAL_HIT || a.reactionUsedBy(t, knight) {
		t.Errorf("a natural 20 = %v, reaction spent %v; want a critical hit and a free reaction", hit.GetRoll().GetOutcome(), a.reactionUsedBy(t, knight))
	}
}

// One reaction a round: the second hit of the same round, which Parry would turn, lands; the reaction is back when
// the Knight's own turn starts and the next round's hit is parried again.
func TestParryAuto_OncePerRound(t *testing.T) {
	t.Parallel()
	a, e := autoParryFight(t, knightKey, knight)
	first := a.mustAttack(t, a.caio, e, "Toren", battleaxe, knight, d20(knightHit))
	if first.GetRoll().GetOutcome() != playv1.AttackOutcome_ATTACK_OUTCOME_MISS || !a.reactionUsedBy(t, knight) {
		t.Fatalf("the first hit = %v, reaction spent %v; want it parried", first.GetRoll().GetOutcome(), a.reactionUsedBy(t, knight))
	}
	second := a.mustAttack(t, a.caio, e, "Toren", battleaxe, knight, d20(knightHit))
	if second.GetRoll().GetOutcome() != playv1.AttackOutcome_ATTACK_OUTCOME_HIT || second.GetPendingDamage().GetStatus() != playv1.PendingDamageStatus_PENDING_DAMAGE_STATUS_AWAITING_ROLL {
		t.Fatalf("the second hit of the round = %v / %v, want it to land", second.GetRoll().GetOutcome(), second.GetPendingDamage().GetStatus())
	}
	a.mustDamage(t, a.caio, e, second.GetPendingDamage().GetId(), inAppDamage)
	if got := a.reactionLines(t, a.master, e); len(got) != 1 {
		t.Errorf("the master reads %q, want the one Parry", got)
	}

	e = a.passTo(t, e, knight)
	if a.reactionUsedBy(t, knight) {
		t.Error("the Knight's reaction is still spent at the start of its own turn")
	}
	e = a.passTo(t, e, "Toren")
	next := a.mustAttack(t, a.caio, e, "Toren", battleaxe, knight, d20(knightHit))
	if next.GetRoll().GetOutcome() != playv1.AttackOutcome_ATTACK_OUTCOME_MISS || !a.reactionUsedBy(t, knight) {
		t.Errorf("the next round's hit = %v, reaction spent %v; want it parried again", next.GetRoll().GetOutcome(), a.reactionUsedBy(t, knight))
	}
}

// What takes the reaction away takes the Parry: a creature that cannot act, cannot see the attacker, has spent its
// reaction, or is hit by a ranged or spell attack parries nothing, and the hit goes on.
func TestParryAuto_NotWhenTheReactionCannotBeUsed(t *testing.T) {
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
			a, e := autoParryFight(t, knightKey, knight)
			c.prep(a, e)
			hit := a.mustAttack(t, a.caio, e, "Toren", battleaxe, knight, c.roll)
			if hit.GetRoll().GetOutcome() != playv1.AttackOutcome_ATTACK_OUTCOME_HIT || hit.GetPendingDamage().GetStatus() != playv1.PendingDamageStatus_PENDING_DAMAGE_STATUS_AWAITING_ROLL {
				t.Errorf("the attack = %v / %v, want a hit whose damage waits for its roll", hit.GetRoll().GetOutcome(), hit.GetPendingDamage().GetStatus())
			}
			if got := a.reactionLines(t, a.master, e); len(got) != 0 {
				t.Errorf("the master reads the reaction lines %q, want none", got)
			}
		})
	}
}

// A ranged weapon attack is not parried, and nor is a creature without the reaction.
func TestParryAuto_OnlyAMeleeWeaponAttackOnACreatureWithParry(t *testing.T) {
	t.Parallel()
	a, e := autoParryFight(t, knightKey, knight)
	hit := a.mustAttack(t, a.caio, e, "Toren", shortbowKey, knight, disadvantage(knightHit+1)) // a ranged weapon attack at 5 ft
	if hit.GetRoll().GetOutcome() != playv1.AttackOutcome_ATTACK_OUTCOME_HIT || a.reactionUsedBy(t, knight) {
		t.Errorf("a shortbow hit = %v, reaction spent %v; want it to land and the reaction free", hit.GetRoll().GetOutcome(), a.reactionUsedBy(t, knight))
	}

	b, f := autoParryFight(t, "monster:bandit", "Bandido")
	got := b.mustAttack(t, b.caio, f, "Toren", battleaxe, "Bandido", d20(knightHit)) // 18 against the bandit's armor class 12
	if got.GetRoll().GetOutcome() != playv1.AttackOutcome_ATTACK_OUTCOME_HIT || b.reactionUsedBy(t, "Bandido") {
		t.Errorf("the bandit's hit = %v, reaction spent %v; want it to land and the reaction free", got.GetRoll().GetOutcome(), b.reactionUsedBy(t, "Bandido"))
	}
}

// Every SRD stat block with Parry parries on its own with its own number.
func TestParryAuto_EveryStatBlockWithTheReaction(t *testing.T) {
	t.Parallel()
	for _, c := range []struct {
		key, label string
		face       int32 // the d20 that makes Toren's total (face + 6) the armor class itself
	}{
		{"monster:bandit-captain", "Capitão bandido", 9},
		{"monster:erinyes", "Erínia", 12},
		{"monster:gladiator", "Gladiador", 10},
		{knightKey, knight, knightHit},
		{"monster:marilith", "Marilith", 12},
		{"monster:noble", "Nobre", 9},
	} {
		t.Run(c.key, func(t *testing.T) {
			t.Parallel()
			a, e := autoParryFight(t, c.key, c.label)
			hit := a.mustAttack(t, a.caio, e, "Toren", battleaxe, c.label, d20(c.face))
			if hit.GetRoll().GetOutcome() != playv1.AttackOutcome_ATTACK_OUTCOME_MISS || !a.reactionUsedBy(t, c.label) {
				t.Errorf("the hit on %s = %v, reaction spent %v; want it parried", c.label, hit.GetRoll().GetOutcome(), a.reactionUsedBy(t, c.label))
			}
		})
	}
}

// The master's Desfazer takes the attack back whole: the reaction returns, the Parry's line goes with it and the same
// attack is parried again.
func TestParryAuto_UndoOfTheAttackGivesTheReactionBack(t *testing.T) {
	t.Parallel()
	a, e := autoParryFight(t, knightKey, knight)
	a.mustAttack(t, a.caio, e, "Toren", battleaxe, knight, d20(knightHit))
	a.undoLast(t, e)
	if a.reactionUsedBy(t, knight) {
		t.Error("the Knight's reaction is still spent after the undo")
	}
	a.wantNoRecord(t, knight, "after the undo")
	for name, u := range map[string]*user{"the master": a.master, "Toren's player": a.caio} {
		if got := a.reactionLines(t, u, e); len(got) != 0 {
			t.Errorf("%s still reads the lines %q after the undo", name, got)
		}
	}
	again := a.mustAttack(t, a.caio, e, "Toren", battleaxe, knight, d20(knightHit))
	if again.GetRoll().GetOutcome() != playv1.AttackOutcome_ATTACK_OUTCOME_MISS || !a.reactionUsedBy(t, knight) {
		t.Errorf("the attack after the undo = %v, reaction spent %v; want it parried again", again.GetRoll().GetOutcome(), a.reactionUsedBy(t, knight))
	}
}

// "Perguntar": the creature asked for opens the window, for the hit that Parry turns and for the one it does not.
func TestParryAuto_AskModeOpensTheWindow(t *testing.T) {
	t.Parallel()
	a, e := autoParryFight(t, knightKey, knight)
	a.setParryAsk(t, e, knight, true)
	hit := a.mustAttack(t, a.caio, e, "Toren", battleaxe, knight, d20(knightHit))
	if hit.GetPendingDamage().GetStatus() != playv1.PendingDamageStatus_PENDING_DAMAGE_STATUS_AWAITING_REACTION || a.windowOf(t, a.master, parryKind) == nil {
		t.Fatalf("Toren's hit on the asked Knight = %v, window %v; want a hit that waits for Parry", hit.GetPendingDamage().GetStatus(), a.windowOf(t, a.master, parryKind))
	}
	if a.reactionUsedBy(t, knight) {
		t.Error("the Knight's reaction is spent before the master answered")
	}
	// Back to the default: the next hit is decided by the server.
	a.mustAnswer(t, a.master, e, a.windowOf(t, a.master, parryKind).GetId(), passAnswer)
	a.setParryAsk(t, e, knight, false)
	e = a.passTo(t, e, knight)
	e = a.passTo(t, e, "Toren")
	hit = a.mustAttack(t, a.caio, e, "Toren", battleaxe, knight, d20(knightHit))
	if hit.GetRoll().GetOutcome() != playv1.AttackOutcome_ATTACK_OUTCOME_MISS || a.windowOf(t, a.master, parryKind) != nil {
		t.Errorf("the hit after the switch back = %v, window %v; want a Parry with no window", hit.GetRoll().GetOutcome(), a.windowOf(t, a.master, parryKind))
	}
}

// SetMonsterReactionMode is the master's: the mode is on the master's copy of a creature with a Parry and on no one else's.
func TestSetMonsterReactionMode_MasterOnlyAndShownToTheMasterAlone(t *testing.T) {
	t.Parallel()
	a, e := autoParryFight(t, knightKey, knight)
	if _, err := a.parryMode(t, a.caio, e, knight, true); err == nil || connect.CodeOf(err) != connect.CodePermissionDenied {
		t.Fatalf("a player's SetMonsterReactionMode error = %v, want permission_denied", err)
	}
	if c := byLabel(t, a.get(t, a.master), knight); c.ParryAsk == nil || c.GetParryAsk() {
		t.Errorf("the master reads parry_ask %v for the Knight by default, want set and false", c.ParryAsk)
	}
	a.setParryAsk(t, e, knight, true)
	if c := byLabel(t, a.get(t, a.master), knight); c.ParryAsk == nil || !c.GetParryAsk() {
		t.Errorf("the master reads parry_ask %v after asking, want true", c.ParryAsk)
	}
	for name, u := range map[string]*user{"Toren's player": a.caio, "Pensantus's player": a.ana} {
		for _, c := range a.get(t, u).GetCombatants() {
			if c.ParryAsk != nil {
				t.Errorf("%s reads parry_ask on %s", name, c.GetLabel())
			}
		}
	}
	// A creature without a Parry has no mode, and the master's copy of it has no field.
	b, f := autoParryFight(t, "monster:bandit", "Bandido")
	_, err := b.parryMode(t, b.master, f, "Bandido", true)
	wantCode(t, "SetMonsterReactionMode on a bandit", err, connect.CodeInvalidArgument)
	if c := byLabel(t, b.get(t, b.master), "Bandido"); c.ParryAsk != nil {
		t.Errorf("the master reads parry_ask %v on a creature with no Parry", c.ParryAsk)
	}
	// A player's combatant has none either.
	_, err = a.parryMode(t, a.master, e, "Toren", true)
	wantCode(t, "SetMonsterReactionMode on a player's combatant", err, connect.CodeInvalidArgument)
}

// The same key counts once: a retry changes nothing and writes no second event.
func TestSetMonsterReactionMode_IsIdempotent(t *testing.T) {
	t.Parallel()
	a, e := autoParryFight(t, knightKey, knight)
	req := &playv1.SetMonsterReactionModeRequest{CampaignId: a.campaignID, EncounterId: e.GetId(), CombatantId: a.id(t, knight), IdempotencyKey: newKey(), Ask: true}
	first, err := a.master.combat.SetMonsterReactionMode(t.Context(), connect.NewRequest(req))
	if err != nil {
		t.Fatalf("SetMonsterReactionMode() error = %v", err)
	}
	var events int
	count := `SELECT count(*) FROM session_events WHERE kind = 'monster_reaction_mode_set'`
	if err := a.h.pool.QueryRow(t.Context(), count).Scan(&events); err != nil {
		t.Fatalf("count the events: %v", err)
	}
	again, err := a.master.combat.SetMonsterReactionMode(t.Context(), connect.NewRequest(req))
	if err != nil {
		t.Fatalf("the retry error = %v", err)
	}
	var after int
	if err := a.h.pool.QueryRow(t.Context(), count).Scan(&after); err != nil {
		t.Fatalf("count the events: %v", err)
	}
	if events != 1 || after != 1 || again.Msg.GetEncounter().GetRevision() != first.Msg.GetEncounter().GetRevision() {
		t.Errorf("events %d then %d, revisions %d then %d; want one event and one revision", events, after, first.Msg.GetEncounter().GetRevision(), again.Msg.GetEncounter().GetRevision())
	}
	if line := strings.Join(a.reactionLines(t, a.master, e), ""); line != "" {
		t.Errorf("the master reads %q for a mode change, want no line", line)
	}
}

// The armor class the hit is compared with is the one that counts: the Knight's 18 plus an active Shield spell (+5),
// the master's cover (+2) or a Haste (+2). Each row has the lowest total the Parry turns (the hit reached the armor
// class) and the one that it does not.
func TestParryAuto_ThresholdFollowsTheArmorClassOfTheHit(t *testing.T) {
	t.Parallel()
	for _, c := range []struct {
		name         string
		prep         func(a *armed, e *playv1.Encounter)
		turned, over int32 // the d20 faces: Toren adds 6
	}{
		{"an active Shield spell", func(a *armed, _ *playv1.Encounter) {
			a.execSQL(t, `UPDATE combatants SET ac_bonus = 5 WHERE id = $1`, a.id(t, knight))
		}, 18, 19}, // 24 against 23 (25 with Parry), then 25
		{"half cover", func(a *armed, e *playv1.Encounter) {
			if _, err := a.master.combat.SetCombatantCover(t.Context(), connect.NewRequest(&playv1.SetCombatantCoverRequest{
				CampaignId: a.campaignID, EncounterId: e.GetId(), CombatantId: a.id(t, knight), IdempotencyKey: newKey(), Cover: playv1.CoverDegree_COVER_DEGREE_HALF,
			})); err != nil {
				t.Fatalf("SetCombatantCover() error = %v", err)
			}
		}, 15, 16}, // 21 against 20 (22 with Parry), then 22
		{"a Haste", func(a *armed, _ *playv1.Encounter) {
			a.execSQL(t, `UPDATE combatants SET effect_ac_bonus = 2 WHERE id = $1`, a.id(t, knight))
		}, 15, 16},
	} {
		t.Run(c.name, func(t *testing.T) {
			t.Parallel()
			a, e := autoParryFight(t, knightKey, knight)
			c.prep(a, e)
			hit := a.mustAttack(t, a.caio, e, "Toren", battleaxe, knight, d20(c.turned))
			if hit.GetRoll().GetOutcome() != playv1.AttackOutcome_ATTACK_OUTCOME_MISS || !a.reactionUsedBy(t, knight) {
				t.Errorf("the hit at the armor class = %v, reaction spent %v; want it parried", hit.GetRoll().GetOutcome(), a.reactionUsedBy(t, knight))
			}
			b, f := autoParryFight(t, knightKey, knight)
			c.prep(b, f)
			over := b.mustAttack(t, b.caio, f, "Toren", battleaxe, knight, d20(c.over))
			if over.GetRoll().GetOutcome() != playv1.AttackOutcome_ATTACK_OUTCOME_HIT || b.reactionUsedBy(t, knight) {
				t.Errorf("the hit at the armor class with Parry = %v, reaction spent %v; want it to land and the reaction free", over.GetRoll().GetOutcome(), b.reactionUsedBy(t, knight))
			}
		})
	}
}

// A hit that Stroke of Luck made is not parried: the rogue spent the feature on a hit, and the Knight's Parry would
// have turned it again.
func TestParryAuto_AStrokeOfLuckHitIsNotParried(t *testing.T) {
	t.Parallel()
	a := strokeTable(t, "", 20)
	e := a.start(t, plan{
		npcs: []*playv1.Participant{{CharacterId: a.goblin.GetId()}}, npcRolls: []int{3},
		players: map[string]int32{"Pipo": 20, "Toren": 14, "Orla": 10}, setup: true,
	})
	a.h.roller.queue(1)
	a.mustAddMonsters(t, e, func(r *playv1.AddMonstersRequest) { r.CreatureKey, r.Count = knightKey, 1 })
	e = a.get(t, a.master)
	for label, sq := range map[string][2]int32{"Pipo": {6, 5}, "Toren": {3, 4}, "Orla": {5, 3}, "Goblin": {12, 8}, knight: {7, 5}} {
		if _, err := a.master.combat.MoveCombatant(t.Context(), connect.NewRequest(&playv1.MoveCombatantRequest{
			CampaignId: a.campaignID, EncounterId: e.GetId(), CombatantId: a.id(t, label), IdempotencyKey: newKey(), Col: sq[0], Row: sq[1],
		})); err != nil {
			t.Fatalf("MoveCombatant(%s) error = %v", label, err)
		}
	}
	for _, shown := range []string{knight, "Goblin"} {
		if _, err := a.master.combat.SetCombatantHidden(t.Context(), connect.NewRequest(&playv1.SetCombatantHiddenRequest{
			CampaignId: a.campaignID, EncounterId: e.GetId(), CombatantId: a.id(t, shown), IdempotencyKey: newKey(), Hidden: false,
		})); err != nil {
			t.Fatalf("SetCombatantHidden(%s) error = %v", shown, err)
		}
	}
	e = a.begin(t, e)
	res, err := a.attack(t, a.caio, e, "Pipo", rapier, knight, d20(2)) // 11 against the Knight's 18: a miss, and Stroke is offered
	if err != nil {
		t.Fatalf("RollAttack() error = %v", err)
	}
	wantStroke(t, "the held attack", res.GetHeldRollOffer(), playv1.HeldRollKind_HELD_ROLL_KIND_WEAPON_ATTACK, true)
	out, err := a.answerHeldRoll(t, a.caio, res.GetHeldRollOffer().GetHoldId(), strokeAnswer(true))
	if err != nil {
		t.Fatalf("AnswerHeldRoll() error = %v", err)
	}
	r := out.GetAttack()
	if r.GetRoll().GetOutcome() != playv1.AttackOutcome_ATTACK_OUTCOME_HIT || r.GetPendingDamage().GetStatus() != playv1.PendingDamageStatus_PENDING_DAMAGE_STATUS_AWAITING_ROLL {
		t.Errorf("the Stroke of Luck hit = %v / %v, want a hit whose damage waits for its roll", r.GetRoll().GetOutcome(), r.GetPendingDamage().GetStatus())
	}
	if a.reactionUsedBy(t, knight) {
		t.Error("the Knight parried a hit Stroke of Luck made")
	}
}

// A retry of the attack with the same key answers from the first call and spends the Knight's reaction once.
func TestParryAuto_ARetryWithTheSameKeyParriesOnce(t *testing.T) {
	t.Parallel()
	a, e := autoParryFight(t, knightKey, knight)
	req := &playv1.RollAttackRequest{
		CampaignId: a.campaignID, EncounterId: e.GetId(), AttackerId: a.id(t, "Toren"), AttackKey: battleaxe, TargetId: a.id(t, knight), IdempotencyKey: newKey(),
		Roll: &playv1.RollAttackRequest_D20Face{D20Face: knightHit},
	}
	first, err := a.caio.combat.RollAttack(t.Context(), connect.NewRequest(req))
	if err != nil {
		t.Fatalf("RollAttack() error = %v", err)
	}
	count := `SELECT count(*) FROM session_events WHERE kind = 'reaction_used'`
	var before, after int
	if err := a.h.pool.QueryRow(t.Context(), count).Scan(&before); err != nil {
		t.Fatalf("count the events: %v", err)
	}
	again, err := a.caio.combat.RollAttack(t.Context(), connect.NewRequest(req))
	if err != nil {
		t.Fatalf("the retry error = %v", err)
	}
	if err := a.h.pool.QueryRow(t.Context(), count).Scan(&after); err != nil {
		t.Fatalf("count the events: %v", err)
	}
	if before != 1 || after != 1 || again.Msg.GetRoll().GetOutcome() != first.Msg.GetRoll().GetOutcome() || again.Msg.GetEncounter().GetRevision() != first.Msg.GetEncounter().GetRevision() {
		t.Errorf("Parry events %d then %d, outcomes %v then %v; want one Parry and the same answer", before, after, first.Msg.GetRoll().GetOutcome(), again.Msg.GetRoll().GetOutcome())
	}
	if !a.reactionUsedBy(t, knight) {
		t.Error("the Knight's reaction is not spent")
	}
}

// Switching the creature to "Perguntar" after its Parry does not close the undo chain: the attack is still the last
// action, and undoing it gives the reaction back.
func TestParryAuto_TheModeSwitchDoesNotCloseTheUndoChain(t *testing.T) {
	t.Parallel()
	a, e := autoParryFight(t, knightKey, knight)
	a.mustAttack(t, a.caio, e, "Toren", battleaxe, knight, d20(knightHit))
	a.setParryAsk(t, e, knight, true)
	a.undoLast(t, e)
	if a.reactionUsedBy(t, knight) {
		t.Error("the Knight's reaction is still spent after the undo")
	}
	if got := a.reactionLines(t, a.master, e); len(got) != 0 {
		t.Errorf("the master still reads %q after the undo", got)
	}
}
