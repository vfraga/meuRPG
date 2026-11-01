package play

import (
	"testing"

	"connectrpc.com/connect"

	"google.golang.org/protobuf/proto"

	campaignsv1 "github.com/PuraFome/meuRPG/backend/gen/meurpg/campaigns/v1"
	playv1 "github.com/PuraFome/meuRPG/backend/gen/meurpg/play/v1"
)

// "Reações dos inimigos" (PM-04): by default only an enemy that can react opens a
// window, and "Sempre" opens the master's one-tap check on every hit, cast and damage
// of a player against an enemy, so that waiting does not tell the table whether the
// enemy could.

func enemyReactionsAlways(r *campaignsv1.TableRules) {
	r.EnemyReactions = campaignsv1.EnemyReactionsRule_ENEMY_REACTIONS_RULE_ALWAYS
}

// TestTheMastersCheckWaitsOnEveryHitWhenTheTableAsksForIt: with "Sempre" Toren's hit on a
// Goblin that has no reaction at all still waits for the master, who answers with one tap.
// The players read what they read when a real reaction is pending.
func TestTheMastersCheckWaitsOnEveryHitWhenTheTableAsksForIt(t *testing.T) {
	t.Parallel()
	a := newCasters(t)
	e := a.threeAndAGoblin(t)
	hit := a.mustAttack(t, a.caio, e, "Toren", battleaxe, "Goblin", d20(15))
	if hit.GetPendingDamage().GetStatus() != playv1.PendingDamageStatus_PENDING_DAMAGE_STATUS_AWAITING_ROLL {
		t.Fatalf("by default the hit waits for %v, want the damage roll: the Goblin cannot react", hit.GetPendingDamage().GetStatus())
	}
	if got := a.get(t, a.caio).GetReactionWait(); got != nil {
		t.Errorf("Toren's player reads %v with no reaction pending", got)
	}

	// Second fight, same table, now with "Sempre".
	b := newCasters(t)
	b.setRules(t, enemyReactionsAlways)
	f := b.threeAndAGoblin(t)
	held := b.mustAttack(t, b.caio, f, "Toren", battleaxe, "Goblin", d20(15))
	if !held.GetRoll().GetHeldForReaction() {
		t.Fatalf("with \"Sempre\" the attack = %v, want it held for the master's check before the roll", held.GetRoll())
	}
	w := b.windowOf(t, b.master, playv1.ReactionKind_REACTION_KIND_MASTER_CHECK)
	if w == nil || !w.GetAnswerNow() || w.GetReactorId() != "" {
		t.Fatalf("the master's window = %v, want the check with no reactor", w)
	}
	if got := b.get(t, b.caio).GetReactionWait().GetTitlePt(); got != "Esperando o mestre" {
		t.Errorf("Toren's player reads %q, want \"Esperando o mestre\"", got)
	}
	for _, u := range []*user{b.caio, b.ana, b.bia} {
		if n := len(b.get(t, u).GetReactionWindows()); n != 0 {
			t.Errorf("a player got %d windows: the check is the master's", n)
		}
	}
	// "Nada a reagir": one tap rolls the attack, a second one on the hit lets it go on to its damage.
	res := b.mustAnswer(t, b.master, f, w.GetId(), passAnswer)
	w2 := b.windowOf(t, b.master, playv1.ReactionKind_REACTION_KIND_MASTER_CHECK)
	if w2 == nil || w2.GetId() == w.GetId() {
		t.Fatalf("the hit has no check of its own after the roll: %v (answer %v)", w2, res.GetResult())
	}
	b.mustAnswer(t, b.master, f, w2.GetId(), passAnswer)
	pending := b.firstPending(t)
	if _, err := b.damage(t, b.caio, f, pending, inAppDamage); err != nil {
		t.Errorf("RollDamage() after the checks error = %v", err)
	}
}

// TestWhatThePlayersReadDoesNotTellWhetherTheEnemyCouldReact: with "Sempre", a hit on a Goblin and a
// hit on a Mago that really has Shield read the same for the attacker: the held attack, the wait,
// and then, after the master's first answer, a second pause (the hit's check, or the real Shield).
func TestWhatThePlayersReadDoesNotTellWhetherTheEnemyCouldReact(t *testing.T) {
	t.Parallel()
	unmasked, e := mageFight(t)
	unmasked.setRules(t, enemyReactionsAlways)
	unmaskedHit := unmasked.mustAttack(t, unmasked.caio, e, "Toren", battleaxe, "Mago", d20(9))

	masked := newCasters(t)
	masked.setRules(t, enemyReactionsAlways)
	f := masked.threeAndAGoblin(t)
	maskedHit := masked.mustAttack(t, masked.caio, f, "Toren", battleaxe, "Goblin", d20(15))

	bare := func(r *playv1.AttackRoll) *playv1.AttackRoll {
		q := proto.Clone(r).(*playv1.AttackRoll)
		q.AttackerId, q.TargetId = "", ""
		return q
	}
	if !proto.Equal(bare(unmaskedHit.GetRoll()), bare(maskedHit.GetRoll())) || !unmaskedHit.GetRoll().GetHeldForReaction() {
		t.Errorf("the held attacks differ:\n unmasked %v\n masked %v", unmaskedHit.GetRoll(), maskedHit.GetRoll())
	}
	for name, u := range map[string][2]*user{"the attacker": {unmasked.caio, masked.caio}, "another player": {unmasked.ana, masked.ana}} {
		if !proto.Equal(unmasked.get(t, u[0]).GetReactionWait(), masked.get(t, u[1]).GetReactionWait()) {
			t.Errorf("%s: the wait differs:\n unmasked %v\n masked %v", name, unmasked.get(t, u[0]).GetReactionWait(), masked.get(t, u[1]).GetReactionWait())
		}
	}
	// After the master's first answer both wait a second time, with the same words.
	unmasked.mustAnswer(t, unmasked.master, e, unmasked.windowOf(t, unmasked.master, playv1.ReactionKind_REACTION_KIND_MASTER_CHECK).GetId(), passAnswer)
	masked.mustAnswer(t, masked.master, f, masked.windowOf(t, masked.master, playv1.ReactionKind_REACTION_KIND_MASTER_CHECK).GetId(), passAnswer)
	if !proto.Equal(unmasked.get(t, unmasked.caio).GetReactionWait(), masked.get(t, masked.caio).GetReactionWait()) || unmasked.get(t, unmasked.caio).GetReactionWait() == nil {
		t.Errorf("the second wait differs:\n unmasked %v\n masked %v", unmasked.get(t, unmasked.caio).GetReactionWait(), masked.get(t, masked.caio).GetReactionWait())
	}
}

// shieldedHit is Toren hitting the Mago, which holds the hit for its Shield.
func shieldedHit(t *testing.T) (*armed, *playv1.Encounter, *playv1.ReactionWindow) {
	t.Helper()
	a, e := mageFight(t)
	a.mustAttack(t, a.caio, e, "Toren", battleaxe, "Mago", d20(9))
	w := a.windowOf(t, a.master, playv1.ReactionKind_REACTION_KIND_SHIELD)
	if w == nil {
		t.Fatal("the Mago has no Shield window")
	}
	return a, e, w
}

// TestTwoAnswersToOneWindowSettleToOne: the master answers the same window twice at the
// same time; one wins and the other is told it was answered already, and Shield is spent once.
func TestTwoAnswersToOneWindowSettleToOne(t *testing.T) {
	t.Parallel()
	a, e, w := shieldedHit(t)
	errs := make(chan error, 2)
	for range 2 {
		go func() {
			_, err := a.answerReaction(t, a.master, e, w.GetId(), useAnswer(1))
			errs <- err
		}()
	}
	var failed, won int
	for range 2 {
		if err := <-errs; err != nil {
			failed++
			wantBlockedBy(t, "the late answer", err, playv1.EncounterBlockedReason_ENCOUNTER_BLOCKED_REASON_REACTION_ALREADY_ANSWERED)
		} else {
			won++
		}
	}
	if won != 1 || failed != 1 {
		t.Fatalf("answers: %d won, %d failed; want one of each", won, failed)
	}
	var used string
	if err := a.h.pool.QueryRow(t.Context(), `SELECT slots_used::TEXT FROM combatants WHERE id = $1`, a.id(t, "Mago")).Scan(&used); err != nil || used != `{"1": 1}` {
		t.Errorf("the Mago's slots used = %q, %v; want one 1st-level slot", used, err)
	}
}

// TestTheEndOfTheCombatDiscardsTheWindows: nothing is left of an unanswered window.
func TestTheEndOfTheCombatDiscardsTheWindows(t *testing.T) {
	t.Parallel()
	a, e, w := shieldedHit(t)
	if _, err := a.master.combat.EndEncounter(t.Context(), connect.NewRequest(&playv1.EndEncounterRequest{CampaignId: a.campaignID, EncounterId: e.GetId(), IdempotencyKey: newKey()})); err != nil {
		t.Fatalf("EndEncounter() error = %v", err)
	}
	var n int
	if err := a.h.pool.QueryRow(t.Context(), `SELECT (SELECT count(*) FROM reaction_windows) + (SELECT count(*) FROM reaction_holds)`).Scan(&n); err != nil || n != 0 {
		t.Errorf("rows left = %d, %v; want none", n, err)
	}
	if _, err := a.answerReaction(t, a.master, e, w.GetId(), passAnswer); err == nil {
		t.Error("a window of a finished combat was answered")
	}
}

// TestAWindowClosesByItselfWhenTheReactorIsIncapacitated: the master paralyzes the Mago
// while Toren's hit waits for it; the window closes, the hit goes on to its damage, and
// nobody is asked anything.
func TestAWindowClosesByItselfWhenTheReactorIsIncapacitated(t *testing.T) {
	t.Parallel()
	a, e, _ := shieldedHit(t)
	a.setConditions(t, e, "Mago", "condition:paralyzed")
	if w := a.windowOf(t, a.master, playv1.ReactionKind_REACTION_KIND_SHIELD); w != nil {
		t.Fatalf("the paralyzed Mago still has a window: %v", w)
	}
	if got := a.get(t, a.caio).GetReactionWait(); got != nil {
		t.Errorf("Toren's player still waits: %v", got)
	}
	if _, err := a.damage(t, a.caio, e, a.firstPending(t), inAppDamage); err != nil {
		t.Errorf("RollDamage() after the window closed error = %v", err)
	}
}

// firstPending is the id of the only pending damage of the combat.
func (a *armed) firstPending(t *testing.T) string {
	t.Helper()
	var id string
	if err := a.h.pool.QueryRow(t.Context(), `SELECT id FROM pending_damages ORDER BY created_at DESC LIMIT 1`).Scan(&id); err != nil {
		t.Fatalf("read the pending damage: %v", err)
	}
	return id
}

// TestAHeldCastAnsweredAgainWithItsKeyHoldsOnce: a client that sends the same cast again (its
// network dropped) gets the same answer and no second window or second hold.
func TestAHeldCastAnsweredAgainWithItsKeyHoldsOnce(t *testing.T) {
	t.Parallel()
	a, e := mageFight(t)
	a.pensantusTurn(t, e)
	key := newKey()
	targets := []*playv1.SpellTarget{darts(a, t, "Goblin", 3)}
	first, err := a.castKey(t, a.ana, e, "Pensantus", magicMissileSpell, slotOfLevel(1), targets, noCastRoll, key)
	if err != nil {
		t.Fatalf("CastSpell() error = %v", err)
	}
	again, err := a.castKey(t, a.ana, e, "Pensantus", magicMissileSpell, slotOfLevel(1), targets, noCastRoll, key)
	if err != nil {
		t.Fatalf("CastSpell(again) error = %v", err)
	}
	if !proto.Equal(first.GetCast(), again.GetCast()) || first.GetCast() != nil {
		t.Errorf("the second answer differs: %v / %v", first, again)
	}
	var holds, windows int
	if err := a.h.pool.QueryRow(t.Context(), `SELECT (SELECT count(*) FROM reaction_holds), (SELECT count(*) FROM reaction_windows)`).Scan(&holds, &windows); err != nil {
		t.Fatalf("count: %v", err)
	}
	if holds != 1 || windows < 1 {
		t.Errorf("holds %d, windows %d; want one hold", holds, windows)
	}
}
