package play

import (
	"testing"

	"connectrpc.com/connect"

	campaignsv1 "github.com/PuraFome/meuRPG/backend/gen/meurpg/campaigns/v1"
	playv1 "github.com/PuraFome/meuRPG/backend/gen/meurpg/play/v1"
)

// The dice of a cast are checked before a window can hold it (#938, #947): a Counterspell holds the cast, and a refusal at the
// replay would come only after the windows were answered. What a sum or a count of faces needs from the dice of the spell is known
// at once, and refused with nothing spent and no window opened.

// counterFight is the fight of the casters with a Mago Sombrio who can Counterspell: a cast of Pensantus, or the release of
// what it readied, is held for his window. The Capitão stands next to Pensantus, so each ray is rolled with disadvantage.
//
// A theatre fight has no map: a spell that lays an area on the map (Sleep) is released with the creatures the answer names.
func (a *armed) counterFight(t *testing.T, theatre bool) *playv1.Encounter {
	t.Helper()
	mago := a.master.bossCaster(t, a.campaignID, "Mago Sombrio", 7, []string{fireBolt}, []string{counterspellSpell}, []string{counterspellSpell})
	p := plan{
		npcs:     []*playv1.Participant{{CharacterId: a.goblin.GetId()}, {CharacterId: a.capitao.GetId()}, {CharacterId: mago}},
		npcRolls: []int{1, 1, 1}, reveal: []string{"Goblin", "Capitão Goblin", "Mago Sombrio"},
		players: map[string]int32{"Pensantus": 20, "Toren": 15, "Brisa": 10}, theatre: theatre,
	}
	if !theatre {
		p.at = map[string][2]int32{"Pensantus": {5, 5}, "Toren": {6, 5}, "Brisa": {5, 6}, "Capitão Goblin": {4, 4}, "Goblin": {7, 5}, "Mago Sombrio": {5, 9}}
	}
	return a.start(t, p)
}

// refusedAtOnce checks a call was refused as an invalid argument with nothing written: no table changed, no event, no
// Counterspell window.
func (a *armed) refusedAtOnce(t *testing.T, what string, before string, err error) {
	t.Helper()
	wantCode(t, what, err, connect.CodeInvalidArgument)
	if w := a.windowOf(t, a.master, playv1.ReactionKind_REACTION_KIND_COUNTERSPELL); w != nil {
		t.Errorf("%s: a Counterspell window was opened: %v", what, w)
	}
	if after := a.stateSnapshot(t); after != before {
		t.Errorf("%s: the refusal changed the state:\nbefore:\n%s\nafter:\n%s", what, before, after)
	}
}

func TestAPoolSumOutOfRangeIsRefusedBeforeACounterspellCanHoldTheCast(t *testing.T) {
	t.Parallel()
	for name, sum := range map[string]int32{"below the dice": 4, "above the dice": 41} {
		t.Run("on the turn, "+name, func(t *testing.T) {
			t.Parallel()
			a := newCasters(t)
			a.forceDice(t, campaignsv1.DiceMode_DICE_MODE_PHYSICAL)
			e := a.counterFight(t, false)
			before := a.stateSnapshot(t)
			_, err := a.cast(t, a.ana, e, "Pensantus", sleepSpell, slotOfLevel(1), a.at(t, "Goblin"), poolSum(sum))
			a.refusedAtOnce(t, "a Sleep cast with a sum out of range", before, err)
		})
		t.Run("the release of a readied spell, "+name, func(t *testing.T) {
			t.Parallel()
			a := newCasters(t)
			a.forceDice(t, campaignsv1.DiceMode_DICE_MODE_PHYSICAL)
			e := a.counterFight(t, true)
			a.mustReady(t, a.ana, a.readySpell(t, e, "Pensantus", sleepSpell, 1, "Goblin"))
			e = a.mustEndTurn(t, a.master, e)
			w := a.mustFire(t, e, "Pensantus")
			before := a.stateSnapshot(t)
			_, err := a.answerReaction(t, a.ana, e, w.GetId(), withPool(sum))
			a.refusedAtOnce(t, "a Sleep release with a sum out of range", before, err)
			if !a.readiedOpen(t, "Pensantus") || a.readiedStatus(t, "Pensantus") != readiedWaiting {
				t.Errorf("the window is open %v and the row %q, want open and waiting", a.readiedOpen(t, "Pensantus"), a.readiedStatus(t, "Pensantus"))
			}
		})
	}
}

// The faces of a ray are checked against the mode that ray rolls with (d20With), and that mode is read when the ray is rolled,
// not before: an earlier ray of the same cast spends what it used (a Help aimed at the target, the disadvantage that the caster's
// next attack takes), and the windows that hold the cast may change it (a Protection that gives the roll disadvantage), so
// the count of faces a ray needs is only known at the replay. A ray that typed the wrong count is then dropped with the cast, and
// nothing of the cast may be left: no slot, no action, no damage of the rays that had been rolled, and a readied release
// stays readied.

// stateOfRays reads what a dropped cast of Scorching Ray could have left of Pensantus's cast.
type stateOfRays struct{ pending, casts, usedAction int }

func (a *armed) stateOfRays(t *testing.T) stateOfRays {
	t.Helper()
	var s stateOfRays
	if err := a.h.pool.QueryRow(t.Context(), `SELECT (SELECT count(*) FROM pending_damages), (SELECT count(*) FROM spell_casts),
		(SELECT count(*) FROM combatants WHERE label = 'Pensantus' AND action_used)`).Scan(&s.pending, &s.casts, &s.usedAction); err != nil {
		t.Fatalf("read what the cast left: %v", err)
	}
	return s
}

func (a *armed) holdStates(t *testing.T) string {
	t.Helper()
	var s string
	if err := a.h.pool.QueryRow(t.Context(), `SELECT COALESCE(string_agg(kind || ':' || state, ',' ORDER BY created_at), '') FROM reaction_holds`).Scan(&s); err != nil {
		t.Fatalf("read the holds: %v", err)
	}
	return s
}

func (a *armed) windowStates(t *testing.T) string {
	t.Helper()
	var s string
	if err := a.h.pool.QueryRow(t.Context(), `SELECT COALESCE(string_agg(kind || ':' || status, ',' ORDER BY kind), '') FROM reaction_windows`).Scan(&s); err != nil {
		t.Fatalf("read the windows: %v", err)
	}
	return s
}

func TestARayOfTheWrongSizeIsOnlyMetAtTheReplayAndTheDroppedCastLeavesNothing(t *testing.T) {
	t.Parallel()
	t.Run("on the turn", func(t *testing.T) {
		t.Parallel()
		a := newRayCasters(t)
		a.forceDice(t, campaignsv1.DiceMode_DICE_MODE_PHYSICAL)
		e := a.counterFight(t, false)
		targets := rayTargets(t, a, map[string]int32{"Goblin": 3}, "Goblin")
		res, err := a.cast(t, a.ana, e, "Pensantus", scorchingRay, slotOfLevel(2), targets, withRays(rays(rayHit, rayHit, []int32{12})...))
		if err != nil || res.GetCast() != nil {
			t.Fatalf("the cast = %v, %v; want it held for the Counterspell: its third ray types one face for a pair, which only the replay can tell", res, err)
		}
		heard := []heardBy{
			listen(t, a.h, a.campaignID, a.master.id, true),
			listen(t, a.h, a.campaignID, a.ana.id, false),
		}
		for _, w := range heard {
			w.heard(t, a.h) // what the hold itself told
		}
		a.mustAnswer(t, a.master, e, a.windowOf(t, a.master, playv1.ReactionKind_REACTION_KIND_COUNTERSPELL).GetId(), passAnswer)
		for i, w := range heard {
			// The rays the replay rolled before it met the third one were rolled back: no stream hears of their damage, hit points or cast.
			got := w.heard(t, a.h)
			for _, ev := range got {
				switch ev.GetEvent().(type) {
				case *playv1.WatchGameSessionResponse_HiddenHitPending_, *playv1.WatchGameSessionResponse_AttackHit_,
					*playv1.WatchGameSessionResponse_VitalsChanged_, *playv1.WatchGameSessionResponse_SpellCastsChanged_:
					t.Errorf("stream %d heard %T of a cast that was dropped; want nothing of its rolled-back rays", i, ev.GetEvent())
				}
			}
		}
		if got := a.holdStates(t); got != "cast:dropped" {
			t.Errorf("the holds = %q, want the cast dropped", got)
		}
		if got := a.windowStates(t); got != "counterspell:answered" {
			t.Errorf("the windows = %q, want the Counterspell answered and nothing open", got)
		}
		if s := a.stateOfRays(t); s != (stateOfRays{}) || usedSlots(a.vitals(t, a.pens), 2) != 0 || a.castLine(t, e, scorchingRay) != nil {
			t.Errorf("the dropped cast left %+v, %d slots spent or a line in the log; want nothing", s, usedSlots(a.vitals(t, a.pens), 2))
		}
		if hp, _, _ := a.hp(t, "Goblin"); hp != 7 {
			t.Errorf("the goblin has %d hit points, want 7", hp)
		}
		// Pensantus still has its turn: the same cast with the right faces goes through the Mago's window again.
		res, err = a.cast(t, a.ana, e, "Pensantus", scorchingRay, slotOfLevel(2), targets, withRays(rays(rayHit, rayHit, rayHit)...))
		if err != nil || res.GetCast() != nil {
			t.Fatalf("the second cast = %v, %v; want it held again", res, err)
		}
		a.mustAnswer(t, a.master, e, a.windowOf(t, a.master, playv1.ReactionKind_REACTION_KIND_COUNTERSPELL).GetId(), passAnswer)
		if a.castLine(t, e, scorchingRay) == nil || usedSlots(a.vitals(t, a.pens), 2) != 1 {
			t.Errorf("the second cast did not land: line %v, slots spent %d", a.castLine(t, e, scorchingRay), usedSlots(a.vitals(t, a.pens), 2))
		}
	})
	t.Run("the release of a readied spell", func(t *testing.T) {
		t.Parallel()
		a := newRayCasters(t)
		a.forceDice(t, campaignsv1.DiceMode_DICE_MODE_PHYSICAL)
		e := a.counterFight(t, false)
		a.mustReady(t, a.ana, a.readySpell(t, e, "Pensantus", scorchingRay, 2, "Goblin"))
		e = a.mustEndTurn(t, a.master, e)
		w := a.mustFire(t, e, "Pensantus")
		spent := usedSlots(a.vitals(t, a.pens), 2) // the readying spent it
		before := a.stateOfRays(t)
		release := func(faces ...[]int32) *playv1.AnswerReactionResponse {
			return a.mustAnswer(t, a.ana, e, w.GetId(), func(r *playv1.AnswerReactionRequest) {
				withRayRolls(faces...)(r)
				r.Targets[0].CombatantId = a.id(t, "Goblin")
			})
		}
		if res := release(rayHit, rayHit, []int32{12}); res.GetResult().GetUsed() {
			t.Fatalf("the release = %v; want it held for the Counterspell", res.GetResult())
		}
		a.mustAnswer(t, a.master, e, a.windowOf(t, a.master, playv1.ReactionKind_REACTION_KIND_COUNTERSPELL).GetId(), passAnswer)
		if got := a.holdStates(t); got != "cast:dropped" {
			t.Errorf("the holds = %q, want the release dropped", got)
		}
		if got := a.windowStates(t); got != "counterspell:answered,readied:open" {
			t.Errorf("the windows = %q, want the Counterspell answered and the READIED window still open", got)
		}
		if a.readiedStatus(t, "Pensantus") != readiedWaiting || !a.readiedOpen(t, "Pensantus") {
			t.Errorf("the row is %q and the window open %v, want the readied spell waiting", a.readiedStatus(t, "Pensantus"), a.readiedOpen(t, "Pensantus"))
		}
		if s := a.stateOfRays(t); s != before || usedSlots(a.vitals(t, a.pens), 2) != spent || a.castLine(t, e, scorchingRay) != nil {
			t.Errorf("the dropped release left %+v (was %+v), %d slots spent (was %d) or a line in the log", s, before, usedSlots(a.vitals(t, a.pens), 2), spent)
		}
		// The readier answers the same window again, with the right faces: the spell is released and the row fired.
		release(rayHit, rayHit, rayHit)
		a.mustAnswer(t, a.master, e, a.windowOf(t, a.master, playv1.ReactionKind_REACTION_KIND_COUNTERSPELL).GetId(), passAnswer)
		if a.castLine(t, e, scorchingRay) == nil || a.readiedStatus(t, "Pensantus") != readiedFired || usedSlots(a.vitals(t, a.pens), 2) != spent {
			t.Errorf("the second release did not land: line %v, row %q, slots spent %d (was %d)", a.castLine(t, e, scorchingRay), a.readiedStatus(t, "Pensantus"), usedSlots(a.vitals(t, a.pens), 2), spent)
		}
	})
}
