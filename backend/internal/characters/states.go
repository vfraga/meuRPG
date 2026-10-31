package characters

import (
	"context"
	"encoding/json"
	"slices"

	"github.com/jackc/pgx/v5"

	charactersv1 "github.com/PuraFome/meuRPG/backend/gen/meurpg/characters/v1"
	playv1 "github.com/PuraFome/meuRPG/backend/gen/meurpg/play/v1"
	"github.com/PuraFome/meuRPG/backend/internal/characters/charactersdb"
	"github.com/PuraFome/meuRPG/backend/internal/platform/uid"
	"github.com/PuraFome/meuRPG/backend/internal/rules"
)

// The temporary states that are on for a character (a table feature's `state` effect: a
// stance, a form). They live in characters.active_states as a JSON list of {key,
// rounds_left}; the sheet's numbers read them (rules.Build.ActiveStates), the vitals show
// them with the rounds left, and the play module starts and ends them through
// play.VitalsKeeper.SetActiveStates. What each state is (its name, duration and ways to
// end) is the content's, read when the vitals are: a key the content no longer defines is
// not shown, and a write drops it.

// storedState is one entry of characters.active_states.
type storedState struct {
	Key string `json:"key"`
	// RoundsLeft is how many rounds of the owner's turns are left; 0 lasts until it ends.
	RoundsLeft int32 `json:"rounds_left,omitempty"`
}

// maxStoredStates is the most states a character has on, as the column's check allows.
const maxStoredStates = 32

// parseStoredStates reads the column; text it cannot read is no state.
func parseStoredStates(raw []byte) []storedState {
	var out []storedState
	if len(raw) == 0 || json.Unmarshal(raw, &out) != nil {
		return nil
	}
	return slices.DeleteFunc(out, func(s storedState) bool { return s.Key == "" })
}

// stateKeysOf are the keys of the states in the column, for Build.ActiveStates.
func stateKeysOf(raw []byte) []string {
	stored := parseStoredStates(raw)
	keys := make([]string, 0, len(stored))
	for _, s := range stored {
		keys = append(keys, s.Key)
	}
	return keys
}

// buildWithStates is buildOf for a sheet whose states are on.
func buildWithStates(f *charactersv1.FullSheet, states []string) rules.Build {
	b := buildOf(f)
	b.ActiveStates = states
	return b
}

// activeStatesProto is the states of the column the content still defines, as the vitals
// show them. defs are Derived.States of the sheet.
func activeStatesProto(stored []storedState, defs []rules.State) []*playv1.ActiveState {
	var out []*playv1.ActiveState
	for _, st := range stored {
		i := slices.IndexFunc(defs, func(d rules.State) bool { return d.Key == st.Key })
		if i < 0 {
			continue
		}
		d := defs[i]
		out = append(out, &playv1.ActiveState{
			Key: d.Key, NamePt: d.NamePT, TextPt: d.TextPT, DurationRounds: i32(d.DurationRounds), RoundsLeft: min(max(st.RoundsLeft, 0), i32(d.DurationRounds)),
			EndsOn: slices.Clone(d.EndsOn),
		})
	}
	return out
}

// storedOf is the column's value for the states the vitals show: the key and the rounds
// left of each.
func storedOf(states []*playv1.ActiveState) []byte {
	raw, _ := json.Marshal(storedFrom(states)) // a list of plain structs cannot fail to encode
	return raw
}

// storedFrom is the entries of the column for the states the vitals show.
func storedFrom(states []*playv1.ActiveState) []storedState {
	out := make([]storedState, 0, len(states))
	for _, st := range states {
		out = append(out, storedState{Key: st.GetKey(), RoundsLeft: max(st.GetRoundsLeft(), 0)})
	}
	return out
}

// HasActiveStates says the character has a state stored, without deriving its sheet. It
// is false for a character that is not living and active in the campaign. It implements
// play.VitalsKeeper, so the combat reads the vitals of a player's character for its states
// only when it has some.
func (s *Service) HasActiveStates(ctx context.Context, tx pgx.Tx, campaignID, characterID string) (bool, error) {
	id, ok := uid.Canonical(characterID)
	if !ok {
		return false, nil
	}
	rows, err := s.queriesIn(tx).ListCombatCharacters(ctx, charactersdb.ListCombatCharactersParams{CampaignID: campaignID, Ids: []string{id}})
	if err != nil {
		return false, s.dbError(ctx, "read the states of a character", err)
	}
	return len(rows) == 1 && len(parseStoredStates(rows[0].ActiveStates)) > 0, nil
}

// SetActiveStates puts the states that are on for the character at states (a key and the
// rounds left of each) inside tx, and returns the vitals before and after. A key the
// character's sheet does not define is `invalid_argument`; more than the most a character
// has on is too. The vitals' revision moves only when the list does. `not_found` for
// anything but a living, active player's character of the campaign. It implements
// play.VitalsKeeper: the caller decides when a state starts and ends, spends what it costs
// and writes the history.
func (s *Service) SetActiveStates(ctx context.Context, tx pgx.Tx, campaignID, characterID string, states []*playv1.ActiveState) (before, after *playv1.CharacterVitals, err error) {
	r, _, err := s.vitalsOfRow(ctx, tx, campaignID, characterID)
	if err != nil {
		return nil, nil, err
	}
	if len(states) > maxStoredStates {
		return nil, nil, invalidArgument(fieldErr("active_states", "has more than %d states", maxStoredStates))
	}
	for _, st := range states {
		if !slices.ContainsFunc(r.m.states, func(d rules.State) bool { return d.Key == st.GetKey() }) {
			return nil, nil, invalidArgument(fieldErr("active_states", "the character's sheet has no state %q", st.GetKey()))
		}
	}
	next := activeStatesProto(storedFrom(states), r.m.states)
	if equalStates(r.view.GetActiveStates(), next) {
		return r.view, r.view, nil
	}
	if err := s.writeStates(ctx, tx, r.row.ID, next); err != nil {
		return nil, nil, err
	}
	again, _, err := s.vitalsOfRow(ctx, tx, campaignID, characterID)
	if err != nil {
		return nil, nil, err
	}
	return r.view, again.view, nil
}

// equalStates says two lists of states hold the same keys with the same rounds left, in order.
func equalStates(a, b []*playv1.ActiveState) bool {
	return slices.EqualFunc(a, b, func(x, y *playv1.ActiveState) bool {
		return x.GetKey() == y.GetKey() && x.GetRoundsLeft() == y.GetRoundsLeft()
	})
}

// writeStates stores the list and moves the vitals' revision.
func (s *Service) writeStates(ctx context.Context, tx pgx.Tx, characterID string, states []*playv1.ActiveState) error {
	q := s.queries.WithTx(tx)
	if err := q.SetActiveStates(ctx, charactersdb.SetActiveStatesParams{ID: characterID, States: storedOf(states)}); err != nil {
		return wrap("save the states", err)
	}
	if _, err := q.TouchVitals(ctx, charactersdb.TouchVitalsParams{CharacterID: characterID, Now: s.now()}); err != nil {
		return wrap("save the vitals", err)
	}
	return nil
}

// endStates takes the states for which ends says so out of v and reports whether any
// went.
func endStates(v *playv1.CharacterVitals, ends func(*playv1.ActiveState) bool) bool {
	kept := slices.DeleteFunc(slices.Clone(v.GetActiveStates()), ends)
	if len(kept) == len(v.GetActiveStates()) {
		return false
	}
	v.ActiveStates = kept
	return true
}

// endsOn is a predicate for endStates: the state lists the way among its ends_on.
func endsOn(ways ...string) func(*playv1.ActiveState) bool {
	return func(st *playv1.ActiveState) bool {
		return slices.ContainsFunc(st.GetEndsOn(), func(w string) bool { return slices.Contains(ways, w) })
	}
}

// restStates ends the states a rest of the kind ends: a short rest ends the ones that say
// "short_rest", a long rest those and the ones that say "long_rest".
func restStates(kind string, after *playv1.CharacterVitals) {
	ways := []string{rules.StateEndsShortRest}
	if kind == rules.RestLong {
		ways = append(ways, rules.StateEndsLongRest)
	}
	endStates(after, endsOn(ways...))
}

// dropIncapacitated ends the states that end when the character is incapacitated, when the
// change from before to after took its hit points to 0.
func dropIncapacitated(before, after *playv1.CharacterVitals) {
	if before.GetHitPointsCurrent() > 0 && after.GetHitPointsCurrent() == 0 {
		endStates(after, endsOn(rules.StateEndsIncapacitated))
	}
}

// endChosenStates ends the states the master's correction names; a key that is not on is
// `invalid_argument`.
func endChosenStates(after *playv1.CharacterVitals, keys []string) error {
	for _, k := range keys {
		if !slices.ContainsFunc(after.GetActiveStates(), func(st *playv1.ActiveState) bool { return st.GetKey() == k }) {
			return fieldErr("end_states", "the character has no state %q on", k)
		}
	}
	endStates(after, func(st *playv1.ActiveState) bool { return slices.Contains(keys, st.GetKey()) })
	return nil
}

// statesChanged says the two vitals hold different states.
func statesChanged(a, b *playv1.CharacterVitals) bool {
	return !equalStates(a.GetActiveStates(), b.GetActiveStates())
}

// saveStateChange stores the states of after when the correction changed them; the vitals
// revision has moved with the rest of the correction.
func (s *Service) saveStateChange(ctx context.Context, tx pgx.Tx, characterID string, before, after *playv1.CharacterVitals) error {
	if !statesChanged(before, after) {
		return nil
	}
	if err := s.queries.WithTx(tx).SetActiveStates(ctx, charactersdb.SetActiveStatesParams{ID: characterID, States: storedOf(after.GetActiveStates())}); err != nil {
		return wrap("save the states", err)
	}
	return nil
}
