package play

import (
	"context"
	"errors"
	"fmt"
	"time"

	"connectrpc.com/connect"
	"github.com/jackc/pgx/v5"

	playv1 "github.com/PuraFome/meuRPG/backend/gen/meurpg/play/v1"
	maplink "github.com/PuraFome/meuRPG/backend/internal/maps/link"
	"github.com/PuraFome/meuRPG/backend/internal/play/playdb"
)

// Traps in play, the session's side (MR-035; RN-10,
// RN-02): the player's "Procurar armadilhas", the master's firing by hand and
// the damage a trap does to a player's character while no combat runs. The
// moves of a combat and the combat's own firing are combat_traps.go's; the dice
// are traps_effect.go's. The maps module owns the trap itself and tells this one,
// through TrapBook, everything that is a map's: where the area is, what a
// character sees of it, who knows it.
//
// What a player never receives: a trap their characters do not know, in any
// response or stream event. A search that finds nothing reads the same as one that
// failed; a trap that fired is public.

// TrapBook is what the play module asks of the maps module for the traps (MR-035,
// D5). *maps.Service implements it; cmd/api connects it with SetTraps, after both
// services exist (play and maps need each other). Nil means no trap does anything
// in play. The methods take no caller: they run after this package's own
// authorization, and the maps module never sends a trap's data to a player.
type TrapBook interface {
	// Traps returns the map's traps with the master's data (DCs, effect), or a
	// `not_found` Connect error when it is not a map of the campaign.
	Traps(ctx context.Context, tx pgx.Tx, campaignID, mapID string) ([]maplink.Trap, error)
	// KnownTraps returns the armed traps the character knows, with their squares
	// and names and without their data.
	KnownTraps(ctx context.Context, campaignID, mapID, characterID string) ([]maplink.Trap, error)
	// FallFt is how deep the pit of a trap preset is, in feet: 0 for a trap that is
	// not a pit or a spec with no preset. A Feather Fall can slow the fall.
	FallFt(presetKey string) int
	// Notice is the passive notice of observers that ended a move (see
	// maps.Service.Notice): it reveals the traps they notice to their characters
	// and tells only their players.
	Notice(ctx context.Context, campaignID, mapID string, who []maplink.Observer) error
	// SearchTraps is "Procurar armadilhas" with the roll's total: it reveals, inside
	// tx, the traps the observer finds to its character.
	SearchTraps(ctx context.Context, tx pgx.Tx, campaignID, mapID string, who maplink.Observer, skill string, totals []int, at time.Time) (maplink.SearchResult, error)
	// Told tells the players, after the commit, that the map changed for them.
	Told(ctx context.Context, campaignID, mapID string, users []string)
	// TriggerTrap marks the trap triggered inside tx and returns it as it was;
	// maplink.ErrTrapNotArmed when it is not armed. RestoreTrap is its undo.
	TriggerTrap(ctx context.Context, tx pgx.Tx, campaignID, mapID, pointID string, at time.Time) (maplink.Trap, error)
	RestoreTrap(ctx context.Context, tx pgx.Tx, campaignID, mapID, pointID, state string, triggeredAt *time.Time, at time.Time) error
	// TrapChanged tells the watching members, after the commit, that a trap's state
	// changed (it fired, or an undo armed it again).
	TrapChanged(ctx context.Context, campaignID, mapID, pointID string)
	// TrapNames returns the names of the traps among the points, by point ID: a trap
	// that fired is public, so its name may go into the log.
	TrapNames(ctx context.Context, campaignID string, pointIDs []string) (map[string]string, error)
}

// SetTraps connects the maps module's traps. play and maps need each other, so
// cmd/api calls it once maps exists, before the server starts.
func (s *Service) SetTraps(b TrapBook) { s.traps = b }

// The skills a search rolls, and the keys of their checks.
const (
	searchPerception    = "perception"
	searchInvestigation = "investigation"
)

var searchCheckKey = map[string]string{searchPerception: "skill:perception", searchInvestigation: "skill:investigation"}

// errNoTraps is the error of a server with the traps off (no maps module wired).
func errNoTraps() error {
	return connect.NewError(connect.CodeUnavailable, errors.New("traps are not available on this server"))
}

func errTrapNotFound() error {
	return connect.NewError(connect.CodeNotFound, errors.New("trap not found"))
}

// errTrapNotArmed is the failed_precondition of firing a trap that is not armed.
func errTrapNotArmed() error {
	return errEncounter(playv1.EncounterBlockedReason_ENCOUNTER_BLOCKED_REASON_TRAP_NOT_ARMED, "the trap is not armed")
}

// runningEncounterOn is the session's combat on the map that is not ended.
func (s *Service) runningEncounterOn(ctx context.Context, sessionID, mapID string) (playdb.Encounter, bool, error) {
	return runningEncounterIn(ctx, s.queries, sessionID, mapID)
}

// runningEncounterIn is runningEncounterOn on the queries given (a transaction's, for a caller that holds one).
func runningEncounterIn(ctx context.Context, q *playdb.Queries, sessionID, mapID string) (playdb.Encounter, bool, error) {
	enc, err := q.GetLatestEncounter(ctx, sessionID)
	if errors.Is(err, pgx.ErrNoRows) {
		return playdb.Encounter{}, false, nil
	}
	if err != nil {
		return playdb.Encounter{}, false, fmt.Errorf("find the session's combat: %w", err)
	}
	return enc, enc.Status != statusEnded && enc.MapID != nil && *enc.MapID == mapID, nil
}

// errCombatBegan is the refusal of a firing outside a combat when a combat began on the
// trap's map after the firing was read: the trap fires from the combat.
func errCombatBegan() error {
	return connect.NewError(connect.CodeFailedPrecondition, errors.New("a combat began on this map: fire the trap from the combat"))
}

// mapChangedHint is the stream event that makes a watcher read the map again.
func mapChangedHint(mapID string) *playv1.WatchGameSessionResponse {
	return &playv1.WatchGameSessionResponse{Event: &playv1.WatchGameSessionResponse_MapChanged_{
		MapChanged: &playv1.WatchGameSessionResponse_MapChanged{MapId: mapID},
	}}
}

// queriesIn is q on tx, or q itself when there is none.
func queriesIn(q *playdb.Queries, tx pgx.Tx) *playdb.Queries {
	if tx == nil {
		return q
	}
	return q.WithTx(tx)
}
