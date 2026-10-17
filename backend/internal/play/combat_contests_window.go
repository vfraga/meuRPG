package play

import (
	"context"
	"errors"
	"fmt"

	"connectrpc.com/connect"

	"github.com/PuraFome/meuRPG/backend/internal/play/playdb"
	"github.com/PuraFome/meuRPG/backend/internal/rules/reaction"
)

// A contest waits in the reaction window of kind CONTEST (PM-04, decision 3 of W7-X): the
// same mechanism as every other wait, with no timeout. The window is derived from the
// contest row, so no write has to open or close it: settleReactions calls syncContests
// after every change, and the window closes by itself (trigger gone) once the contest no
// longer waits. The reactor is whoever owes the next step: the defender's player for the
// defender's roll, the shover's player for the shove's choice; the master holds it for an
// NPC, and for a roll a player left to him. Players read "Esperando o mestre" or
// "Esperando <nome>" (reaction.Wait).

// contestMaxListed is how many of the latest contests a sync looks at.
const contestMaxListed = 200

// contestReactor is who a waiting contest asks next: nil for the master.
func contestReactor(row playdb.CombatContest, byID map[string]playdb.Combatant) *playdb.Combatant {
	switch row.Status {
	case contestAwaitingDefender:
		d, ok := byID[row.DefenderID]
		if !ok || d.Kind != kindPlayer {
			return nil
		}
		if r, err := decodeRoll(row.DefenderRoll); err == nil && r != nil && r.Deferred {
			return nil
		}
		return &d
	case contestAwaitingOutcome:
		i, ok := byID[row.InitiatorID]
		if ok && i.Kind == kindPlayer {
			return &i
		}
	}
	return nil
}

// syncContests opens the window of each waiting contest that has none, and replaces the
// window whose reactor changed (a roll left to the master).
func (s *Service) syncContests(ctx context.Context, c *combatTx) error {
	if c.enc.ID == "" || c.enc.Status != statusActive {
		return nil
	}
	waiting, err := waitingContestsOf(ctx, c)
	if err != nil {
		return err
	}
	have, err := contestWindowsByContest(ctx, c)
	if err != nil {
		return err
	}
	if len(waiting) == 0 && len(have) == 0 {
		return nil
	}
	byID, err := contestCombatantsByID(ctx, c)
	if err != nil {
		return err
	}
	for _, row := range waiting {
		if err := s.syncContestWindow(ctx, c, row, contestReactor(row, byID), have); err != nil {
			return err
		}
	}
	return nil
}

// waitingContestsOf lists the latest contests of the encounter that wait for an answer.
func waitingContestsOf(ctx context.Context, c *combatTx) ([]playdb.CombatContest, error) {
	rows, err := c.q.ListContests(ctx, playdb.ListContestsParams{EncounterID: c.enc.ID, Limit: contestMaxListed})
	if err != nil {
		return nil, fmt.Errorf("list the contests: %w", err)
	}
	waiting := rows[:0:0]
	for _, r := range rows {
		if r.Status == contestAwaitingDefender || r.Status == contestAwaitingOutcome {
			waiting = append(waiting, r)
		}
	}
	return waiting, nil
}

// contestWindowsByContest is the open CONTEST windows, by the contest each one waits on.
func contestWindowsByContest(ctx context.Context, c *combatTx) (map[string]playdb.ReactionWindow, error) {
	open, err := openWindowsOf(ctx, c)
	if err != nil {
		return nil, err
	}
	have := map[string]playdb.ReactionWindow{}
	for _, w := range open {
		if reaction.Kind(w.Kind) == reaction.Contest {
			have[windowTriggerOf(w).Contest] = w
		}
	}
	return have, nil
}

// contestCombatantsByID is the combatants of the encounter by id.
func contestCombatantsByID(ctx context.Context, c *combatTx) (map[string]playdb.Combatant, error) {
	cs, err := c.q.ListCombatants(ctx, c.enc.ID)
	if err != nil {
		return nil, fmt.Errorf("list the combatants: %w", err)
	}
	return combatantsByID(cs), nil
}

// syncContestWindow keeps the window of a waiting contest when its reactor is the same, and
// otherwise closes it (if there is one) and opens the window of the reactor.
func (s *Service) syncContestWindow(ctx context.Context, c *combatTx, row playdb.CombatContest, reactor *playdb.Combatant, have map[string]playdb.ReactionWindow) error {
	if w, ok := have[row.ID]; ok {
		if deref(w.ReactorID) == idOf(reactor) {
			return nil
		}
		if _, err := s.closeWindow(ctx, c, w, windowClosed, reaction.ReasonTriggerGone, windowOutcome{}, ""); err != nil {
			return err
		}
	}
	_, err := s.openWindows(ctx, c, row.ID, nil, []windowSpec{{
		kind: reaction.Contest, reactor: reactor,
		trigger: windowTrigger{Contest: row.ID, Actor: row.InitiatorID, Target: row.DefenderID},
	}})
	return err
}

func idOf(c *playdb.Combatant) string {
	if c == nil {
		return ""
	}
	return c.ID
}

// contestStillWaits says whether the contest of a CONTEST window still waits.
func contestStillWaits(ctx context.Context, c *combatTx, w playdb.ReactionWindow) (bool, error) {
	row, err := c.q.GetContest(ctx, playdb.GetContestParams{EncounterID: c.enc.ID, ID: windowTriggerOf(w).Contest})
	if err != nil {
		return false, nil //nolint:nilerr // a contest that is not there waits for nothing
	}
	return row.Status == contestAwaitingDefender || row.Status == contestAwaitingOutcome, nil
}

// errContestWindow is the refusal of AnswerReaction for a CONTEST window.
func errContestWindow() error {
	return connect.NewError(connect.CodeInvalidArgument, errors.New("a contest is answered with RespondContest or ResolveShove"))
}
