package play

import (
	"context"
	"errors"
	"fmt"
	"slices"

	"connectrpc.com/connect"

	playv1 "github.com/PuraFome/meuRPG/backend/gen/meurpg/play/v1"
	"github.com/PuraFome/meuRPG/backend/internal/play/link"
	"github.com/PuraFome/meuRPG/backend/internal/play/playdb"
	"github.com/PuraFome/meuRPG/backend/internal/rules"
	"github.com/PuraFome/meuRPG/backend/internal/rules/combat"
)

// The temporary states a table feature defines (a stance for 1 minute, a form that lasts
// until a long rest: the state effect of the rules). They are the character's, kept in its
// vitals (CharacterVitals.active_states) and read by its sheet; this file is how a combat
// starts, counts down and ends them.
//
//   - Starting and ending one is TakeAction on the card of the state (rules.Action.State):
//     starting spends the economy and the resource like a granted action, ending is free.
//   - A round of the owner's turns counts a state with a duration down; at 0 it ends.
//   - Hit points at 0 and a condition that incapacitates end the ones that say so; a rest
//     (characters.TakeRest) ends the ones that say so; the end of the combat ends the ones
//     that have a duration in rounds, because the rounds are counted in a combat only.
//
// The combatant copies the character's speed, size and jumps when it joins, so every
// change of the states writes them again (applyBody).

// The reasons a table state ends in a combat, as the log line says them
// (CombatLogStateChange.reason). Every start and every end in a combat writes a line;
// outside a combat there is no combat log, so a rest or a correction writes none.
const (
	stateEndDuration      = "duration"
	stateEndIncapacitated = "incapacitated"
	stateEndByChoice      = "by_choice"
	stateEndMaster        = "master"
	stateEndCombat        = "left"
)

// stateRec is a state of the vitals as an event keeps it: the key and the rounds left.
type stateRec struct {
	Key    string `json:"key"`
	Rounds int32  `json:"rounds,omitempty"`
}

// recsOf are the states of the vitals, for an event.
func recsOf(v *playv1.CharacterVitals) []stateRec {
	out := make([]stateRec, 0, len(v.GetActiveStates()))
	for _, st := range v.GetActiveStates() {
		out = append(out, stateRec{Key: st.GetKey(), Rounds: st.GetRoundsLeft()})
	}
	return out
}

// activeOf are the states the keeper is given: a key and the rounds left of each.
func activeOf(recs []stateRec) []*playv1.ActiveState {
	out := make([]*playv1.ActiveState, 0, len(recs))
	for _, r := range recs {
		out = append(out, &playv1.ActiveState{Key: r.Key, RoundsLeft: r.Rounds})
	}
	return out
}

// checkState refuses a start of a state that is on and an end of one that is not,
// before anything is spent: the card the player saw may be out of date.
func (f *takeActionFlow) checkState() error {
	a := f.action.GetAction()
	if a.GetStateKey() == "" {
		return nil
	}
	switch {
	case a.GetStateActive() && !f.call.endState:
		return errEncounter(playv1.EncounterBlockedReason_ENCOUNTER_BLOCKED_REASON_STATE_ALREADY_ON, "the state is on already")
	case !a.GetStateActive() && f.call.endState:
		return errEncounter(playv1.EncounterBlockedReason_ENCOUNTER_BLOCKED_REASON_STATE_NOT_ON, "the state is not on")
	}
	return nil
}

// toggleState starts the state of the card, or ends it when it is on. The economy and the
// resource are spent already.
func (f *takeActionFlow) toggleState(ctx context.Context, fa link.FeatureAction) error {
	if f.who.Kind != kindPlayer {
		return connect.NewError(connect.CodeInvalidArgument, errors.New("only a player's character has states"))
	}
	now, err := f.s.vitals.GetVitalsTx(ctx, f.c.tx, f.m.CampaignID, f.who.CharacterID)
	if err != nil {
		return err
	}
	recs := recsOf(now)
	next := slices.DeleteFunc(slices.Clone(recs), func(r stateRec) bool { return r.Key == fa.State })
	if !fa.StateActive {
		next = append(next, stateRec{Key: fa.State, Rounds: clamp32(fa.StateRounds, 0, rules.MaxStateRounds)})
	}
	after, err := f.s.putStates(ctx, f.c, f.who, next)
	if err != nil {
		return err
	}
	// The line is written before the action's own event, which names it: the undo of the
	// action takes the line back with it (linesBefore).
	shown, reason := after, ""
	if fa.StateActive {
		shown, reason = now, stateEndByChoice
	}
	st := stateIn(shown, fa.State)
	line, err := f.s.tableStateLine(ctx, f.c, f.who, st, !fa.StateActive, reason)
	if err != nil {
		return err
	}
	f.out.vitals = after
	f.made.StatesBefore, f.made.StatesChanged = recs, true
	f.made.StateLines = append(f.made.StateLines, line)
	return nil
}

// stateIn is the state of the vitals with the key (an empty one with that key when the
// vitals do not list it).
func stateIn(v *playv1.CharacterVitals, key string) *playv1.ActiveState {
	for _, st := range v.GetActiveStates() {
		if st.GetKey() == key {
			return st
		}
	}
	return &playv1.ActiveState{Key: key}
}

// putStates sets the states of a player's character inside the combat's transaction, gives
// its combatant the numbers the new states make, and returns the vitals after (which the
// change tells the streams). Setting the states it has changes nothing.
func (s *Service) putStates(ctx context.Context, c *combatTx, who playdb.Combatant, next []stateRec) (*playv1.CharacterVitals, error) {
	_, after, err := s.vitals.SetActiveStates(ctx, c.tx, c.session.CampaignID, who.CharacterID, activeOf(next))
	if err != nil {
		return nil, err
	}
	chars, err := s.roster.CombatCharacters(ctx, c.tx, c.session.CampaignID, []string{who.CharacterID})
	if err != nil {
		return nil, err
	}
	if len(chars) == 1 {
		if err := applyBody(ctx, c, who, chars[0]); err != nil {
			return nil, err
		}
	}
	return after, nil
}

// statesBack puts the states an action began or ended back as they were.
func (u *undoer) statesBack(ctx context.Context, who playdb.Combatant, ev actionEvent) error {
	if !ev.StatesChanged || who.Kind != kindPlayer {
		return nil
	}
	after, err := u.s.putStates(ctx, u.c, who, ev.StatesBefore)
	if err != nil {
		return err
	}
	u.keep(after)
	return nil
}

// tableStateLine writes the log line of a table state that began or ended, for the
// combatant it belongs to, and returns the line's id.
func (s *Service) tableStateLine(ctx context.Context, c *combatTx, who playdb.Combatant, st *playv1.ActiveState, started bool, reason string) (string, error) {
	return insertEventID(ctx, c, eventStateChanged, &c.actorUserID, nil, actionEvent{
		Round: c.enc.Round, Secret: who.Hidden, Actor: who.ID, StateKind: stateTable, Key: st.GetKey(), StateName: st.GetNamePt(), StateStarted: started, StateReason: reason,
	})
}

// stateTable is the kind of the state events of a table's states.
const stateTable = "table"

// statesNow reads the vitals of a combatant's character for its states. A character the
// vitals do not list (dead, or gone from the table) has no state to end: nil, no error.
// A character with no state stored is nil too, before its sheet is derived for the vitals.
func (s *Service) statesNow(ctx context.Context, c *combatTx, who playdb.Combatant) (*playv1.CharacterVitals, error) {
	has, err := s.vitals.HasActiveStates(ctx, c.tx, c.session.CampaignID, who.CharacterID)
	if err != nil || !has {
		return nil, err
	}
	now, err := s.vitals.GetVitalsTx(ctx, c.tx, c.session.CampaignID, who.CharacterID)
	if connect.CodeOf(err) == connect.CodeNotFound {
		return nil, nil
	}
	return now, err
}

// endStatesWhere ends the states of a combat character for which ends says so, with a line
// each, and tells the streams. It does nothing for a combatant that is not a player's character.
func (s *Service) endStatesWhere(ctx context.Context, c *combatTx, who playdb.Combatant, reason string, ends func(*playv1.ActiveState) bool) error {
	if who.Kind != kindPlayer {
		return nil
	}
	now, err := s.statesNow(ctx, c, who)
	if err != nil || now == nil {
		return err
	}
	var next []stateRec
	var ended []*playv1.ActiveState
	for _, st := range now.GetActiveStates() {
		if ends(st) {
			ended = append(ended, st)
		} else {
			next = append(next, stateRec{Key: st.GetKey(), Rounds: st.GetRoundsLeft()})
		}
	}
	if len(ended) == 0 {
		return nil
	}
	after, err := s.putStates(ctx, c, who, next)
	if err != nil {
		return err
	}
	c.told = append(c.told, after)
	for _, st := range ended {
		if _, err := s.tableStateLine(ctx, c, who, st, false, reason); err != nil {
			return err
		}
	}
	return nil
}

// tickTableStates counts a round down on the states of the characters whose turn starts:
// a state with rounds left loses one, and ends when none is left. A state with no duration
// is not counted.
func (s *Service) tickTableStates(ctx context.Context, c *combatTx, ids []string) error {
	cs, err := listCombatants(ctx, c)
	if err != nil {
		return err
	}
	for _, id := range ids {
		who, ok := findByID(cs, id)
		if !ok || who.Kind != kindPlayer {
			continue
		}
		if err := s.tickOne(ctx, c, who); err != nil {
			return err
		}
	}
	return nil
}

// tickOne counts a round down for one character.
func (s *Service) tickOne(ctx context.Context, c *combatTx, who playdb.Combatant) error {
	now, err := s.statesNow(ctx, c, who)
	if err != nil || now == nil {
		return err
	}
	if !slices.ContainsFunc(now.GetActiveStates(), func(st *playv1.ActiveState) bool { return st.GetRoundsLeft() > 0 }) {
		return nil
	}
	var ended []*playv1.ActiveState
	var next []stateRec
	for _, st := range now.GetActiveStates() {
		switch {
		case st.GetRoundsLeft() > 1:
			next = append(next, stateRec{Key: st.GetKey(), Rounds: st.GetRoundsLeft() - 1})
		case st.GetRoundsLeft() == 1:
			ended = append(ended, st)
		default:
			next = append(next, stateRec{Key: st.GetKey()})
		}
	}
	after, err := s.putStates(ctx, c, who, next)
	if err != nil {
		return err
	}
	c.told = append(c.told, after)
	for _, st := range ended {
		if _, err := s.tableStateLine(ctx, c, who, st, false, stateEndDuration); err != nil {
			return err
		}
	}
	return nil
}

// dropStatesIfIncapacitated ends the states that end when the character is incapacitated,
// for a combatant that cannot act (a condition that incapacitates it).
func (s *Service) dropStatesIfIncapacitated(ctx context.Context, c *combatTx, cb playdb.Combatant) error {
	if cb.Kind != kindPlayer || !(combat.Creature{Conditions: cb.Conditions}).Incapacitated() {
		return nil
	}
	return s.endStatesWhere(ctx, c, cb, stateEndIncapacitated, func(st *playv1.ActiveState) bool {
		return slices.Contains(st.GetEndsOn(), rules.StateEndsIncapacitated)
	})
}

// endCombatStates ends, with the combat, the states that have a duration in rounds: the
// rounds are counted in a combat only.
func (s *Service) endCombatStates(ctx context.Context, c *combatTx, cs []playdb.Combatant) error {
	for _, who := range cs {
		if err := s.endStatesWhere(ctx, c, who, stateEndCombat, func(st *playv1.ActiveState) bool { return st.GetDurationRounds() > 0 }); err != nil {
			return fmt.Errorf("end the states with the combat: %w", err)
		}
	}
	return nil
}

// endedBetween are the states the vitals had before and no longer have after.
func endedBetween(before, after *playv1.CharacterVitals) []*playv1.ActiveState {
	var out []*playv1.ActiveState
	for _, st := range before.GetActiveStates() {
		if !slices.ContainsFunc(after.GetActiveStates(), func(a *playv1.ActiveState) bool { return a.GetKey() == st.GetKey() }) {
			out = append(out, st)
		}
	}
	return out
}

// followEndedStates is what a change of the vitals that ended states leaves in the combat
// of the character (the hit points at 0, the master's correction): its combatant takes the
// numbers of the states it still has (a speed bonus ends with its state), and each state
// that ended has its line. A character that is not in the combat leaves nothing to do.
func (s *Service) followEndedStates(ctx context.Context, c *combatTx, characterID string, ended []*playv1.ActiveState, reasonOf func(key string) string) error {
	if len(ended) == 0 {
		return nil
	}
	cs, err := listCombatants(ctx, c)
	if err != nil {
		return err
	}
	i := slices.IndexFunc(cs, func(cb playdb.Combatant) bool { return cb.Kind == kindPlayer && cb.CharacterID == characterID })
	if i < 0 {
		return nil
	}
	chars, err := s.roster.CombatCharacters(ctx, c.tx, c.session.CampaignID, []string{characterID})
	if err != nil {
		return err
	}
	if len(chars) == 1 {
		if err := applyBody(ctx, c, cs[i], chars[0]); err != nil {
			return err
		}
	}
	for _, st := range ended {
		if _, err := s.tableStateLine(ctx, c, cs[i], st, false, reasonOf(st.GetKey())); err != nil {
			return err
		}
	}
	return nil
}
