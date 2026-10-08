package play

import (
	"context"
	"fmt"
	"slices"

	"github.com/PuraFome/meuRPG/backend/internal/play/playdb"
)

// Joint turns (MR-013, RN-19, RN-20): combatants adjacent in the turn order
// with the same initiative total form a group, and a group takes one turn
// together. This file works the groups out and moves the turn between them;
// the proto's header ("Joint turns") is the API reference for the rules.
//
// A group is never stored. When its turn starts, its living members get
// turn_state 'acting' (migration 00089) and that answer stays until the turn
// passes, so a reorder, a reinforcement or a hidden member cannot change a
// turn already running. A member who ended its part is 'ended'.

// The database's values for combatants.turn_state (combatants_turn_state_valid).
const (
	turnIdle   = "idle"   // not in the turn that is running
	turnActing = "acting" // in the turn and its part has not ended
	turnEnded  = "ended"  // in the turn and its part ended
)

// groupRuns splits cs, which is in turn order, into the groups: runs of
// adjacent combatants with the same initiative total. A combatant without an
// initiative is a group of one.
func groupRuns(cs []playdb.Combatant) [][]playdb.Combatant {
	var out [][]playdb.Combatant
	for _, c := range cs {
		if n := len(out); n > 0 && c.Initiative != nil {
			if first := out[n-1][0]; first.Initiative != nil && *first.Initiative == *c.Initiative {
				out[n-1] = append(out[n-1], c)
				continue
			}
		}
		out = append(out, []playdb.Combatant{c})
	}
	return out
}

// livingIDs are the IDs of the group's members that can take the turn: not
// defeated, and not skip (the one that is leaving).
func livingIDs(group []playdb.Combatant, skip string) []string {
	var ids []string
	for _, c := range group {
		if !c.Defeated && c.ID != skip {
			ids = append(ids, c.ID)
		}
	}
	return ids
}

// nextTurnGroup says who takes the turn after the group that holds current:
// the living members of the next group that has any, in order, and whether
// the round changed to find it (it wrapped past the last group). skip is a
// member that is leaving the fight. ok is false when nobody can play.
// current may be missing from cs (it was removed): then the turns start from
// the first group, in the same round.
func nextTurnGroup(cs []playdb.Combatant, current, skip string) (ids []string, newRound, ok bool) {
	runs := groupRuns(cs)
	start := slices.IndexFunc(runs, func(g []playdb.Combatant) bool {
		return slices.ContainsFunc(g, func(c playdb.Combatant) bool { return c.ID == current })
	})
	for step := 1; step <= len(runs); step++ {
		i, wrapped := start+step, false
		if start < 0 {
			i = step - 1
		}
		if i >= len(runs) {
			i, wrapped = i-len(runs), true
		}
		if ids := livingIDs(runs[i], skip); len(ids) > 0 {
			return ids, wrapped, true
		}
	}
	return nil, false, false
}

// inTurn says whether the combatant is in the turn that is running, whether
// its part ended or not.
func inTurn(e playdb.Encounter, c playdb.Combatant) bool {
	return e.Status == statusActive && c.TurnState != turnIdle
}

// actsNow says whether the combatant may act: it is in the running turn and
// its part has not ended. Every "is it this combatant's turn" check is this.
func actsNow(e playdb.Encounter, c playdb.Combatant) bool {
	return e.Status == statusActive && c.TurnState == turnActing
}

// turnMembers returns the combatants in the turn that is running, in order.
func turnMembers(cs []playdb.Combatant) []playdb.Combatant {
	var out []playdb.Combatant
	for _, c := range cs {
		if c.TurnState != turnIdle {
			out = append(out, c)
		}
	}
	return out
}

// startTurn starts the turn of the group whose living members are ids: every
// one's turn starts at once (the economy resets, the Escudo bonus ends, a
// death save is due again), and the combat's state points at the first.
func startTurn(ctx context.Context, c *combatTx, ids []string, round int32) error {
	if err := c.q.ClearCombatTurns(ctx, c.enc.ID); err != nil {
		return fmt.Errorf("clear the turn: %w", err)
	}
	for _, id := range ids {
		if err := c.q.ResetCombatantTurn(ctx, id); err != nil {
			return fmt.Errorf("reset the turn: %w", err)
		}
	}
	// A character looking through its familiar's eyes since its last turn looks
	// through its own again (MR-036).
	if c.svc != nil {
		if err := c.svc.endFamiliarSights(ctx, c, ids); err != nil {
			return err
		}
	}
	return setCurrent(ctx, c, ids[0], round)
}

// setCurrent saves where the combat is: the round and the member that the
// stored current_combatant_id names, the first of the group on turn (the API's
// current_combatant_id is worked out for each viewer from turn_state).
func setCurrent(ctx context.Context, c *combatTx, id string, round int32) error {
	var cur *string
	if id != "" {
		cur = &id
	}
	var err error
	if c.enc, err = c.q.SetEncounterState(ctx, playdb.SetEncounterStateParams{
		ID: c.enc.ID, Status: statusActive, Round: round, CurrentCombatantID: cur, StartedAt: c.enc.StartedAt,
	}); err != nil {
		return fmt.Errorf("pass the turn: %w", err)
	}
	return nil
}

// othersActing lists the members of the running turn, other than but, whose part
// has not ended and who can still take it. A defeated member (a death confirmed,
// an NPC taken to 0 hit points) keeps its turn_state but has no part left to
// take: it never holds the group back from passing.
func othersActing(cs []playdb.Combatant, but string) []string {
	var out []string
	for _, o := range cs {
		if o.ID != but && o.TurnState == turnActing && !o.Defeated {
			out = append(out, o.ID)
		}
	}
	return out
}

// leaveTurn settles the turn after a combatant leaves the fight (removed, or
// dead) while the combat is ACTIVE. If it was in the turn, the others go on;
// when nobody who acts is left, the turn passes to the next group. cs is the
// combatants as they were before the change, in order; the leaver is skipped.
// It reports whether the turn passed. The caller deletes or defeats the
// combatant.
func leaveTurn(ctx context.Context, c *combatTx, cs []playdb.Combatant, who playdb.Combatant) (passed bool, err error) {
	if !inTurn(c.enc, who) {
		c.enc, err = c.q.TouchEncounter(ctx, c.enc.ID)
		if err != nil {
			return false, fmt.Errorf("touch the encounter: %w", err)
		}
		return false, nil
	}
	if acting := othersActing(cs, who.ID); len(acting) > 0 {
		// The group's turn goes on without it.
		return false, setCurrent(ctx, c, acting[0], c.enc.Round)
	}
	next, newRound, ok := nextTurnGroup(cs, who.ID, who.ID)
	if !ok {
		if err := c.q.ClearCombatTurns(ctx, c.enc.ID); err != nil {
			return false, fmt.Errorf("clear the turn: %w", err)
		}
		return true, setCurrent(ctx, c, "", c.enc.Round)
	}
	round := c.enc.Round
	if newRound {
		round++
	}
	return true, startTurn(ctx, c, next, round)
}
