package play

import (
	"testing"

	playv1 "github.com/PuraFome/meuRPG/backend/gen/meurpg/play/v1"
)

// A roll a reaction window holds is replayed when the window closes, and a refusal of the replay drops the hold but commits the transaction
// (settleReplay): what the replay spent before it refused is lost. The player asked a normal roll over a visible disadvantage, which the
// Inspiração makes (spend_favor on a disadvantage: normal); the disadvantage ends while the window is open, so at the replay the spend
// would give an advantage, a better mode than the one asked, and the roll is refused. The refusal must come before the spend.
func TestFavorAgainstOnAReplayRefusesBeforeItSpends(t *testing.T) {
	t.Parallel()
	for _, p := range favorPaths {
		t.Run(p.name, func(t *testing.T) {
			t.Parallel()
			f := newFavorFight(t)
			p.ready(t, f)
			f.setRules(t, enemyReactionsAlways)
			u, who := p.roller(f)
			f.give(t, who)
			f.hiddenEffect(t, p.actor, true, "["+attackDis+"]")
			held, err := p.send(t, f, u, newKey(), true, func(r any) { setRollMode(r, playv1.RollMode_ROLL_MODE_NORMAL, "sem pressa") })
			if err != nil || !held.heldByWindow {
				t.Fatalf("the spend on a roll a window holds = %+v, %v, want it held", held, err)
			}
			f.execSQL(t, `DELETE FROM combatant_states WHERE encounter_id = $1 AND kind = 'effect'`, f.e.GetId())
			w := f.windowOf(t, f.master, playv1.ReactionKind_REACTION_KIND_MASTER_CHECK)
			if w == nil {
				t.Fatal("the master has no window")
			}
			f.h.roller.queue(4, 15)
			f.mustAnswer(t, f.master, f.e, w.GetId(), passAnswer)
			var dropped int
			if err := f.h.pool.QueryRow(t.Context(), `SELECT count(*) FROM session_events WHERE kind = $1 AND game_session_id = $2 AND payload->'reaction'->>'kind' = $3`,
				eventReactionAnswered, f.sessionID, reactionHeldDropped).Scan(&dropped); err != nil || dropped != 1 {
				t.Fatalf("the hold was dropped %d times (%v), want the replay's refusal to drop it", dropped, err)
			}
			if !f.favorHolds(t) {
				t.Errorf("the replay refused the roll for the better mode the spend gave, with the Inspiração already spent: it is lost")
			}
		})
	}
}
