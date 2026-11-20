package characters

import (
	"context"
	"encoding/json"
	"errors"
	"slices"

	"github.com/jackc/pgx/v5"

	"github.com/PuraFome/meuRPG/backend/internal/characters/charactersdb"
	"github.com/PuraFome/meuRPG/backend/internal/platform/sessiontell"
	"github.com/PuraFome/meuRPG/backend/internal/rules"
)

// OptionSwap is one option of a table feature that a level-up gave up for another.
type OptionSwap struct {
	// FeatureKey is the table feature whose option pool the swap was made in (the group of the swap).
	FeatureKey string
	// GivenUp is the option the character no longer has, and Taken the option of the same pool it took in its place.
	GivenUp, Taken string
	// ClassKey is the class that gained the level.
	ClassKey string
	// ToLevel is the character's total level after the level-up, the number the level-up's own record
	// (ListLevelUps, `to_level`) holds.
	ToLevel int
}

// OptionSwaps is what another module hears when a level-up swaps the options of a table feature
// (LevelUpChoices.swap_feature_options). cmd/api may connect one with SetOptionSwaps; without it the
// level-up works the same and nobody is told.
type OptionSwaps interface {
	// OptionsGivenUp is called by LevelUpCharacter, in the level-up's own transaction tx (the one that holds the
	// character's row and the session's funnel), right after the new sheet and the level-up's record are
	// written and before the transaction commits, once in each run of the transaction of a level-up that swaps at least one option of a table
	// feature, with every swap of that level in swaps (never empty). It is not called for a preview, for a level-up
	// that is refused, or for a level without such a swap. The character is campaignID's characterID; what the
	// streams are told goes through t, once the transaction committed. An error aborts the level-up: nothing
	// of it is saved and the caller gets the error.
	//
	// The transaction can run again, so the hook may be called more than once for one level-up, and it must do nothing outside
	// tx. The character's own tables are not the hook's: the level-up itself, in the same transaction and before the hook, clears the
	// used counts of the resources the given-up option defined and ends the states it started, so an option taken again later starts
	// fresh; what other modules keep for the option is for the hook to clear.
	OptionsGivenUp(ctx context.Context, tx pgx.Tx, campaignID, characterID string, swaps []OptionSwap, t sessiontell.Tell) error
}

// SetOptionSwaps connects the module that hears the swaps of a table feature's options. It is optional: nil, the
// default, calls nobody.
func (s *Service) SetOptionSwaps(h OptionSwaps) { s.optionSwaps = h }

// tellOptionSwaps calls the module that hears the swaps of a table feature's options, in the level-up's transaction, for a level
// that made at least one. A nil module, or a level with no swap, calls nobody.
func (s *Service) tellOptionSwaps(ctx context.Context, tx pgx.Tx, tell sessiontell.Tell, campaignID, characterID, classKey string, plan levelUpPlan) error {
	swaps := plan.record.GetSwapFeatureOptions()
	if s.optionSwaps == nil || len(swaps) == 0 {
		return nil
	}
	told := make([]OptionSwap, 0, len(swaps))
	for _, sw := range swaps {
		told = append(told, OptionSwap{FeatureKey: sw.GetGroup(), GivenUp: sw.GetFromKey(), Taken: sw.GetToKey(), ClassKey: classKey, ToLevel: plan.toLevel})
	}
	return s.optionSwaps.OptionsGivenUp(ctx, tx, campaignID, characterID, told, tell)
}

// endGivenUpOptions clears what the options a level-up gave up had stored in the character's own tables, in the level-up's
// transaction: the used counts of the resources the option defined (a resource another feature of the new sheet still
// defines keeps its count) and the states the option started. An option picked again later starts fresh.
func (s *Service) endGivenUpOptions(ctx context.Context, tx pgx.Tx, current charactersdb.Character, t levelUpTarget, plan levelUpPlan) error {
	var givenUp []string
	for _, sw := range plan.record.GetSwapFeatureOptions() {
		givenUp = append(givenUp, sw.GetFromKey())
	}
	if len(givenUp) == 0 {
		return nil
	}
	before, after := rules.Derive(t.build, t.content), rules.Derive(buildOf(plan.sheet), t.content)
	resources, states := endedByGivingUp(before, after, givenUp)
	q := s.queries.WithTx(tx)
	if err := s.clearResourceUses(ctx, q, current.ID, resources); err != nil {
		return err
	}
	return s.clearStates(ctx, tx, current, states)
}

// endedByGivingUp are the keys of the resources and of the states that the given-up options defined and nothing else of the new
// sheet defines: a resource or state another source in after still defines keeps what it has stored.
func endedByGivingUp(before, after rules.Derived, givenUp []string) (resources, states []string) {
	for _, r := range before.Resources {
		if slices.Contains(givenUp, r.Source) && !slices.ContainsFunc(after.Resources, func(a rules.Resource) bool { return a.Key == r.Key }) {
			resources = append(resources, r.Key)
		}
	}
	for _, st := range before.States {
		if slices.Contains(givenUp, st.Source) && !slices.ContainsFunc(after.States, func(a rules.State) bool { return a.Key == st.Key }) {
			states = append(states, st.Key)
		}
	}
	return resources, states
}

// clearResourceUses takes the stored used counts of the resources out of the character's vitals row, when it has one.
func (s *Service) clearResourceUses(ctx context.Context, q *charactersdb.Queries, characterID string, keys []string) error {
	raw, err := q.GetResourcesUsed(ctx, characterID)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil
	}
	if err != nil {
		return wrap("read the used resources", err)
	}
	used := storedResourcesUsed(raw)
	had := len(used)
	for _, k := range keys {
		delete(used, k)
	}
	if len(used) == had {
		return nil
	}
	doc, err := json.Marshal(used)
	if err != nil {
		return wrap("encode the used resources", err)
	}
	if err := q.SetResourcesUsed(ctx, charactersdb.SetResourcesUsedParams{CharacterID: characterID, ResourcesUsed: doc, Now: s.now()}); err != nil {
		return wrap("save the used resources", err)
	}
	return nil
}

// clearStates ends the states with these keys that are on for the character.
func (s *Service) clearStates(ctx context.Context, tx pgx.Tx, current charactersdb.Character, keys []string) error {
	stored := parseStoredStates(current.ActiveStates)
	kept := slices.DeleteFunc(slices.Clone(stored), func(st storedState) bool { return slices.Contains(keys, st.Key) })
	if len(kept) == len(stored) {
		return nil
	}
	raw, err := json.Marshal(kept)
	if err != nil {
		return wrap("encode the states", err)
	}
	return s.writeStoredStates(ctx, tx, current.ID, raw)
}
