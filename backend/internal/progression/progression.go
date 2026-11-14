// Package progression is how the characters grow (MR-016, RN-09, RN-12): the
// XP the master gives the party (ProgressionService), by the defeated enemies
// of a combat, by gold or by an amount the master picks, the milestones the
// master marks, the history of both (the whole campaign reads it, question
// 50) and who can go up a level.
//
// An award writes each character's share into the XP on its sheet, in the same
// transaction, through a Party interface the characters module implements
// (characters.Service.AddExperience), like play's VitalsKeeper: the sheet is
// the characters module's. The combat's defeated NPCs and the session's log
// are play's, reached through Combats and SessionLog. The campaign's XP mode
// is campaigns'. cmd/api connects them; no package imports another's
// internals. The tables (xp_awards, xp_award_shares) are this module's, and,
// like session_events, they are never rewritten: an undo sets undone_at and
// takes the XP back through the sheets (ADR-0007).
//
// The other way round, the characters module asks LevelUpReason whether a
// character can go up a level, for GetCharacter's can_level_up (RN-12).
//
// The SQL lives in queries.sql, and sqlc turns it into package progressiondb.
// Every write runs inside db.InTx, which retries CockroachDB's serialization
// errors (40001).
package progression

import (
	"context"
	"errors"
	"log/slog"
	"net/http"
	"slices"
	"time"

	"connectrpc.com/connect"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	campaignsv1 "github.com/PuraFome/meuRPG/backend/gen/meurpg/campaigns/v1"
	"github.com/PuraFome/meuRPG/backend/gen/meurpg/progression/v1/progressionv1connect"
	"github.com/PuraFome/meuRPG/backend/internal/authz"
	"github.com/PuraFome/meuRPG/backend/internal/platform/nostore"
	"github.com/PuraFome/meuRPG/backend/internal/platform/rpcerr"
	"github.com/PuraFome/meuRPG/backend/internal/platform/sessiontell"
	"github.com/PuraFome/meuRPG/backend/internal/progression/link"
	"github.com/PuraFome/meuRPG/backend/internal/progression/progressiondb"
)

// Party is what the XP needs from the characters (the characters module
// implements it). The methods take no caller: they run after this package's
// own authorization check.
type Party interface {
	// Party returns the campaign's living, active player characters, oldest
	// first, with their level and XP, read in tx (nil outside a transaction).
	Party(ctx context.Context, tx pgx.Tx, campaignID string) ([]link.Member, error)
	// Names returns the names of those of ids that are characters of the
	// campaign, by ID, whatever their kind or status.
	Names(ctx context.Context, campaignID string, ids []string) (map[string]string, error)
	// AddExperience adds delta (negative to take back) to a player character's
	// sheet inside tx, keeping the XP between 0 and 1,000,000, and returns the
	// XP before and after. `not_found` for any character but a living or dead
	// player character of the campaign.
	AddExperience(ctx context.Context, tx pgx.Tx, campaignID, characterID string, delta int32, at time.Time) (before, after int32, err error)
}

// Combats is what an enemies award needs from the combats (the play module
// implements it).
type Combats interface {
	// CampaignEncounter returns a combat of the campaign: its name, whether it
	// ended and the XP its defeated NPCs give. `not_found` for any other.
	CampaignEncounter(ctx context.Context, tx pgx.Tx, campaignID, encounterID string) (link.Encounter, error)
	// EncounterNames returns the names of those of ids that are combats of the
	// campaign, by ID.
	EncounterNames(ctx context.Context, campaignID string, ids []string) (map[string]string, error)
}

// SessionLog is the open session's history (the play module
// implements it): an award is an event of the session, when one is open
// (ADR-0007).
type SessionLog interface {
	// AppendEvent appends an event to the history of the campaign's open
	// session inside tx, and says whether there was one. The payload holds IDs
	// and numbers only.
	AppendEvent(ctx context.Context, tx pgx.Tx, campaignID, kind, actorUserID string, payload []byte, at time.Time) (bool, error)
}

// Treasures is the found treasure of the maps, for "Voltar à cidade" (MR-041;
// the maps module implements it, as maps.Treasures). The treasure points are
// the maps' own tables: this package only asks, and links a treasure to an
// award inside the award's transaction.
type Treasures interface {
	// ListUnconverted returns the campaign's treasures that were found and no
	// award converted, the oldest find first.
	ListUnconverted(ctx context.Context, campaignID string) ([]link.Treasure, error)
	// LockForConversion returns, inside tx, those of pointIDs that are treasures
	// of the campaign, with their rows locked until tx ends (two awards that
	// race for one treasure take turns). Any other ID is left out.
	LockForConversion(ctx context.Context, tx pgx.Tx, campaignID string, pointIDs []string) ([]link.Treasure, error)
	// MarkConverted links the treasures, already locked and checked, to the
	// award inside tx.
	MarkConverted(ctx context.Context, tx pgx.Tx, awardID string, pointIDs []string) error
	// Release frees the treasures the award converted inside tx (it was
	// undone): they are "found, not converted" again. It returns the IDs of the
	// maps whose points it freed.
	Release(ctx context.Context, tx pgx.Tx, awardID string) ([]string, error)
	// PublishChanged tells the watching members that the points of those maps
	// changed state, after a conversion or an undo committed. It carries no
	// content: the app reads the map again, where it is allowed (RN-10).
	PublishChanged(campaignID string, mapIDs []string)
}

// Campaigns tells how the campaign levels (the campaigns module implements it).
type Campaigns interface {
	// CampaignXPMode returns the campaign's XP mode (RN-09).
	CampaignXPMode(ctx context.Context, tx pgx.Tx, campaignID string) (campaignsv1.XpMode, error)
}

// Profiles tells what to call users. The identity module implements it.
type Profiles interface {
	// DisplayNames returns the display names of the given users, keyed by user
	// ID. Users without one are left out.
	DisplayNames(ctx context.Context, userIDs []string) (map[string]string, error)
}

// Config holds what the progression service needs.
type Config struct {
	// Pool is the CockroachDB connection pool. Required.
	Pool *pgxpool.Pool
	// Party gives and takes the XP on the sheets. Required.
	Party Party
	// Combats reads a combat's defeated NPCs. Required.
	Combats Combats
	// Log is the session's history and stream. Required.
	Log SessionLog
	// Treasures is the found treasure "Voltar à cidade" converts. Required.
	Treasures Treasures
	// Campaigns tells the campaign's XP mode. Required.
	Campaigns Campaigns
	// Profiles gives display names. Required.
	Profiles Profiles
	// Logger receives errors, without personal data. Nil means
	// slog.Default().
	Logger *slog.Logger
	// Now returns the current time. Nil means time.Now.
	Now func() time.Time
}

// Service implements the ProgressionService Connect API, and the characters
// module's LevelUps.
type Service struct {
	pool    *pgxpool.Pool
	queries *progressiondb.Queries
	party   Party
	combats Combats
	log     SessionLog
	// sessionHost is play's funnel, connected by SetSessionHost (session.go).
	sessionHost sessiontell.Host
	treasures   Treasures
	campaigns   Campaigns
	profiles    Profiles
	logger      *slog.Logger
	now         func() time.Time
}

// The compiler checks that Service implements the handler.
var _ progressionv1connect.ProgressionServiceHandler = (*Service)(nil)

// New returns a Service.
func New(cfg Config) (*Service, error) {
	switch {
	case cfg.Pool == nil:
		return nil, errors.New("progression: a Pool is required")
	case cfg.Party == nil:
		return nil, errors.New("progression: Party is required")
	case cfg.Combats == nil:
		return nil, errors.New("progression: Combats is required")
	case cfg.Log == nil:
		return nil, errors.New("progression: Log is required")
	case cfg.Treasures == nil:
		return nil, errors.New("progression: Treasures is required")
	case cfg.Campaigns == nil:
		return nil, errors.New("progression: Campaigns is required")
	case cfg.Profiles == nil:
		return nil, errors.New("progression: Profiles is required")
	}
	s := &Service{
		pool:      cfg.Pool,
		queries:   progressiondb.New(cfg.Pool),
		party:     cfg.Party,
		combats:   cfg.Combats,
		log:       cfg.Log,
		treasures: cfg.Treasures,
		campaigns: cfg.Campaigns,
		profiles:  cfg.Profiles,
		logger:    cfg.Logger,
		now:       cfg.Now,
	}
	if s.logger == nil {
		s.logger = slog.Default()
	}
	if s.now == nil {
		s.now = time.Now
	}
	return s, nil
}

// Sessions is what this package needs to know who is calling: an interceptor
// that finds the caller's session, and the authz.Caller that reads it back.
// *identity.Service is the real one; tests pass a fake.
type Sessions interface {
	// Interceptor finds the caller's session (from the session cookie).
	Interceptor() connect.Interceptor
	authz.Caller
}

// Mount registers ProgressionService on a mux. handle is usually
// httpserver.Server.Handle or http.ServeMux.Handle.
//
// sessions tells who is calling (the identity service in production), and
// members tells each caller's role in a campaign (the campaigns service).
// Mount adds, in this order, an interceptor that marks every response
// `Cache-Control: no-store`, the sessions interceptor, and the authz
// interceptor. opts are the Connect options shared by every service.
func (s *Service) Mount(handle func(pattern string, handler http.Handler), sessions Sessions, members authz.MembershipSource, opts ...connect.HandlerOption) {
	// Clip so append copies instead of writing into the caller's array.
	opts = append(slices.Clip(opts), connect.WithInterceptors(
		nostore.Interceptor(),                          // no response is cacheable
		sessions.Interceptor(),                         // who is calling
		authz.Interceptor(sessions, members, s.logger), // what they may do, memoized per request
	))
	handle(progressionv1connect.NewProgressionServiceHandler(s, opts...))
}

// dbError maps an error from the database to the Connect error the client gets
// (see platform/rpcerr).
func (s *Service) dbError(ctx context.Context, action string, err error) error {
	return rpcerr.FromDB(ctx, s.logger, "progression", action, err)
}

// queriesIn is the queries on the transaction, or on the pool when tx is nil. A
// read made while the caller holds a transaction must use the transaction: a
// read through the pool takes a second connection (see
// docs/architecture/transactions.md#transactions-and-the-connection-pool).
func (s *Service) queriesIn(tx pgx.Tx) *progressiondb.Queries {
	if tx == nil {
		return s.queries
	}
	return s.queries.WithTx(tx)
}
