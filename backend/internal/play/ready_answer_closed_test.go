package play

import (
	"testing"

	playv1 "github.com/PuraFome/meuRPG/backend/gen/meurpg/play/v1"
)

// "Usar" on a READIED window that was answered is refused as any answered window is, whatever the readier's turn:
// the readied spell is not cast, least of all as an ordinary cast of the readier's own turn.
func TestUsingAnAnsweredReadiedWindowIsRefusedAndCastsNothing(t *testing.T) {
	t.Parallel()
	a := newCasters(t)
	e := a.castersFight(t, 1)
	e = a.passTo(t, e, "Brisa")
	a.mustReady(t, a.bia, a.readySpell(t, e, "Brisa", sacredFlame, 0, "Goblin"))
	e = a.mustEndTurn(t, a.master, e)
	w := a.mustFire(t, e, "Brisa")
	a.mustAnswer(t, a.bia, e, w.GetId(), passAnswer)
	e = a.nextTurnOf(t, e, "Brisa")
	_, err := a.answerReaction(t, a.bia, e, w.GetId(), useSpellAnswer)
	wantBlockedBy(t, "Usar on a window that was answered", err, playv1.EncounterBlockedReason_ENCOUNTER_BLOCKED_REASON_NOT_YOUR_TURN_TO_ANSWER)
	if brisa := byLabel(t, a.get(t, a.bia), "Brisa"); brisa.GetActionUsed() || a.castLine(t, e, sacredFlame) != nil {
		t.Errorf("the refused answer left Brisa's action used %v and a cast line %v", brisa.GetActionUsed(), a.castLine(t, e, sacredFlame))
	}
}
