// Package characters manages a campaign's characters: the players'
// characters and the master's NPCs, their sheets and stories, and the
// master's private notes (MR-003, MR-004, MR-005, MR-006; RN-01, RN-03,
// RN-04, RN-11, RN-16). It also serves ContentService, the rules catalog the
// character editor offers, until the table's own content has a home of its
// own (ADR-0008).
//
// The sheet and the story are stored as JSON documents: the protojson of
// charactersv1.CharacterSheet and charactersv1.CharacterStory. Derived
// numbers are never stored; package rules computes them on every read
// (rules.Derive), and checks every sheet before it is written
// (rules.Validate).
//
// Who may do what is decided at the start of every handler with package
// authz, from the caller's role in the campaign (ADR-0011). A player sees
// only their own characters; everything else is "not found" to them, so NPC
// IDs never leak.
//
// Invites with approval (RN-15, MR-024). A pending member (package authz,
// pending.go) may create one player character, which starts 'pending', and
// read and edit it while they wait; they see nothing else. The master
// approves it (ApproveCharacter: the character becomes a draft and the
// membership active) or rejects it (RejectCharacter: both are deleted), in
// one transaction that also touches campaign_members through
// PendingMembers, which package campaigns implements (approval.go).
//
// A pending member who has not created a character is removed after 30
// days (campaign_members.pending_expires_at, migration 00035). Creating the
// character clears that deadline in the same transaction (CreateCharacter).
// Package campaigns calls ApprovePendingCharacter when an ordinary invite
// promotes a pending member (approval.go).
//
// The sheet lock (RN-01) is set by package play when a game session starts,
// through LockSheets, inside play's own transaction. The vitals (RN-02:
// current hit points, spell slots and hit dice used, table
// character_vitals) are kept here too, and package play reads and corrects
// them during a session through ListVitals, GetVitals and AdjustVitals
// (vitals.go). Neither package imports the other: cmd/api connects them.
//
// The SQL lives in queries.sql, and sqlc turns it into package charactersdb.
// Every write runs inside db.InTx, which retries CockroachDB's serialization
// errors (40001).
package characters

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"slices"
	"time"

	"connectrpc.com/connect"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/PuraFome/meuRPG/backend/gen/meurpg/characters/v1/charactersv1connect"
	"github.com/PuraFome/meuRPG/backend/gen/meurpg/rules/v1/rulesv1connect"
	"github.com/PuraFome/meuRPG/backend/internal/authz"
	"github.com/PuraFome/meuRPG/backend/internal/characters/charactersdb"
	"github.com/PuraFome/meuRPG/backend/internal/platform/dice"
	"github.com/PuraFome/meuRPG/backend/internal/platform/nostore"
	"github.com/PuraFome/meuRPG/backend/internal/platform/wiring"
	"github.com/PuraFome/meuRPG/backend/internal/rules"
)

// Profiles tells what to call users. The identity module implements it
// (identity.PostgresStore.DisplayNames), so this package never reads the
// users table itself.
type Profiles interface {
	// DisplayNames returns the display names of the given users, keyed by
	// user ID. Users without one are left out.
	DisplayNames(ctx context.Context, userIDs []string) (map[string]string, error)
}

// PendingMembers settles a pending membership when the master decides on
// the pending member's character (RN-15, MR-024). The campaigns module
// implements it (campaigns.Service), because campaign_members is its table;
// cmd/api connects the two, and neither package imports the other.
type PendingMembers interface {
	// ActivatePendingMember makes userID's pending membership of
	// campaignID an ordinary one, inside tx. An active membership, or none,
	// stays as it is.
	ActivatePendingMember(ctx context.Context, tx pgx.Tx, campaignID, userID string) error
	// DeletePendingMember deletes userID's pending membership of
	// campaignID, inside tx. An active membership is never deleted.
	DeletePendingMember(ctx context.Context, tx pgx.Tx, campaignID, userID string) error
	// ClearPendingExpiry removes the 30-day deadline of userID's pending
	// membership of campaignID, inside tx. The deadline only applies to a
	// pending member who has no character: once there is one, the master
	// decides on it.
	ClearPendingExpiry(ctx context.Context, tx pgx.Tx, campaignID, userID string) error
}

// Config holds what the characters service needs.
type Config struct {
	// Pool is the CockroachDB connection pool. Required.
	Pool *pgxpool.Pool
	// Profiles gives players' display names. Required.
	Profiles Profiles
	// Members settles a pending member's membership when the master
	// approves or rejects their character (RN-15). Required.
	Members PendingMembers
	// Content gives the rules content of a campaign (ContentSource). Required;
	// NewSRDSource(rules.LoadSRD()) is the one that serves the SRD to every
	// campaign.
	Content ContentSource
	// SRD is the base SRD content (rules.LoadSRD), for what no table can
	// change and so needs no campaign: the conditions' names (RN-22) and the
	// scene checks' names (the skills). Required.
	SRD *rules.Content
	// MaxCharactersPerCampaign is the most characters and NPCs one campaign
	// may hold (RN-30). Zero means DefaultMaxCharactersPerCampaign; tests set
	// a small one.
	MaxCharactersPerCampaign int
	// Dice tells what the campaign's dice setting makes a player do with the
	// hit die of a level-up (RN-18). Nil means every player chooses.
	Dice DiceRules
	// Roller rolls the level-up's hit die in the app. Nil means the operating
	// system's random source (dice.Crypto); tests pass faces.
	Roller dice.Roller
	// Logger receives errors, without personal data. Nil means
	// slog.Default().
	Logger *slog.Logger
	// Now returns the current time. Nil means time.Now.
	Now func() time.Time
}

// Service implements the CharacterService and ContentService Connect APIs,
// and LockSheets and the vitals for package play.
type Service struct {
	pool     *pgxpool.Pool
	queries  *charactersdb.Queries
	profiles Profiles
	members  PendingMembers
	content  ContentSource
	srd      *rules.Content
	logger   *slog.Logger
	now      func() time.Time
	// catalogs holds ListContent's answer for the last few contents, built on
	// first use (catalogFor).
	catalogs catalogCache
	// levelUps says whether a character can go up a level (RN-12): package
	// progression, connected by SetLevelUps. Nil until then.
	levelUps LevelUps
	// gallery says which images are in a campaign's gallery, for the NPCs'
	// portraits (MR-031): package maps, connected by SetGallery. Nil until then.
	gallery Gallery
	// dice and roller are the level-up's hit die (MR-040, RN-18), and live the
	// session's stream, connected by SetLive (package play is made after this
	// one). Nil live publishes nothing.
	dice   DiceRules
	roller dice.Roller
	live   Live
	// creatureHost is package play, connected by SetCreatureHost: the combat
	// that holds a character's creatures and the history they are written to
	// (MR-037). Nil until then.
	creatureHost CreatureHost
	// maxCharacters is the cap on a campaign's characters (RN-30).
	maxCharacters int
}

// The compiler checks that Service implements both handlers.
var (
	_ charactersv1connect.CharacterServiceHandler = (*Service)(nil)
	_ rulesv1connect.ContentServiceHandler        = (*Service)(nil)
)

// New returns a Service.
func New(cfg Config) (*Service, error) {
	switch {
	case cfg.Pool == nil:
		return nil, errors.New("characters: a Pool is required")
	case cfg.Profiles == nil:
		return nil, errors.New("characters: Profiles is required")
	case cfg.Members == nil:
		return nil, errors.New("characters: Members is required")
	case cfg.SRD == nil:
		return nil, errors.New("characters: SRD is required")
	case cfg.Content == nil:
		return nil, errors.New("characters: Content is required")
	}
	s := &Service{
		pool:     cfg.Pool,
		queries:  charactersdb.New(cfg.Pool),
		profiles: cfg.Profiles,
		members:  cfg.Members,
		content:  cfg.Content,
		srd:      cfg.SRD,
		logger:   cfg.Logger,
		now:      cfg.Now,
		dice:     cfg.Dice,
		roller:   cfg.Roller,

		maxCharacters: cfg.MaxCharactersPerCampaign,
	}
	if s.maxCharacters == 0 {
		s.maxCharacters = DefaultMaxCharactersPerCampaign
	}
	if s.roller == nil {
		s.roller = dice.Crypto{}
	}
	if s.logger == nil {
		s.logger = slog.Default()
	}
	if s.now == nil {
		s.now = time.Now
	}
	return s, nil
}

// Sessions is what this package needs to know who is calling: an
// interceptor that finds the caller's session, and the authz.Caller that
// reads it back. *identity.Service is the real one. Tests pass a fake, so
// nothing here, or in package authz, can set the caller itself
// (docs/architecture.md#who-is-calling).
type Sessions interface {
	// Interceptor finds the caller's session (from the session cookie).
	Interceptor() connect.Interceptor
	authz.Caller
}

// Mount registers CharacterService, ContentService and TableContentService on a mux. handle is
// usually httpserver.Server.Handle or http.ServeMux.Handle.
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
	handle(charactersv1connect.NewCharacterServiceHandler(s, opts...))
	handle(charactersv1connect.NewInventoryServiceHandler(s, opts...))
	handle(rulesv1connect.NewContentServiceHandler(s, opts...))
	handle(rulesv1connect.NewTableContentServiceHandler(s, opts...))
}

// LockSheets locks the sheets of the campaign's living player characters
// that are still drafts (RN-01), and ends every permission the master gave
// to edit a story in the campaign. It returns how many sheets it locked.
//
// Package play calls it from StartGameSession, inside the transaction that
// opens the session, after checking that the caller is the campaign's
// master. It takes no caller on purpose: it is "the system" of RN-01, not
// someone editing a sheet. Nothing else calls it.
func (s *Service) LockSheets(ctx context.Context, tx pgx.Tx, campaignID string, at time.Time) (int64, error) {
	q := s.queries.WithTx(tx)
	locked, err := q.LockSheets(ctx, charactersdb.LockSheetsParams{CampaignID: campaignID, Now: at})
	if err != nil {
		return 0, fmt.Errorf("lock sheets: %w", err)
	}
	if _, err := q.EndStoryEditing(ctx, campaignID); err != nil {
		return 0, fmt.Errorf("end story editing: %w", err)
	}
	return locked, nil
}

// CheckWired fails when a collaborator that cmd/api connects after New is
// still nil (see platform/wiring).
func (s *Service) CheckWired() error {
	return wiring.Check("characters",
		wiring.Dep{Setter: "SetGallery", Missing: s.gallery == nil},
		wiring.Dep{Setter: "SetLive", Missing: s.live == nil},
		wiring.Dep{Setter: "SetCreatureHost", Missing: s.creatureHost == nil},
		wiring.Dep{Setter: "SetLevelUps", Missing: s.levelUps == nil})
}
