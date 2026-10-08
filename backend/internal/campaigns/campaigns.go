// Package campaigns manages campaigns, their members, the invites that let
// new players in (MR-001, MR-002, MR-003, MR-024) and each campaign's
// document (MR-018, document.go).
//
// A campaign's master creates it and shares invite links; a player signs in
// and accepts an invite, which makes them a player of the campaign. A
// signed-out player can do both at once: the invite rides the sign-in as an
// identity sign-in intent (signin.go). Who may
// do what is decided by package authz from campaign_members, which this
// package owns: Service implements authz.MembershipSource.
//
// An invite can require the master's approval (RN-15, MR-024). Accepting
// such an invite makes the caller a pending member (campaign_members.status
// 'pending'), who may only work on their one character until the master
// approves or rejects it (package authz, pending.go). The master decides in
// package characters (ApproveCharacter, RejectCharacter), which settles the
// membership in the same transaction through ActivatePendingMember and
// DeletePendingMember below; cmd/api connects the two packages.
//
// A pending member who has not created a character is removed after 30
// days (campaign_members.pending_expires_at, deleted by CockroachDB's TTL
// job, migration 00035). The master sees them with ListPendingMembers and
// may remove them sooner with RemovePendingMember. An invite WITHOUT
// approval accepted by a pending member promotes them (acceptInvite), and
// approves their character, if any, through Characters.
//
// The SQL lives in queries.sql, and sqlc turns it into package campaignsdb.
// Every write runs inside db.InTx, which retries CockroachDB's
// serialization errors (40001).
package campaigns

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

	campaignsv1 "github.com/PuraFome/meuRPG/backend/gen/meurpg/campaigns/v1"
	"github.com/PuraFome/meuRPG/backend/gen/meurpg/campaigns/v1/campaignsv1connect"
	"github.com/PuraFome/meuRPG/backend/internal/authz"
	"github.com/PuraFome/meuRPG/backend/internal/campaigns/campaignsdb"
	"github.com/PuraFome/meuRPG/backend/internal/platform/wiring"
)

// MaxNameLength is the longest campaign name, in characters. The
// campaigns_name_length CHECK says the same.
const MaxNameLength = 80

// Invite rules. RN-07 is still open, so the defaults follow ADR-0009's
// proposal (one use, 7 days) and the master can change them within limits.
const (
	DefaultInviteUses = 1
	// MaxInviteUses keeps an invite from becoming a public link: a table
	// rarely has more than a handful of players.
	MaxInviteUses = 20

	DefaultInviteLifetime = 7 * 24 * time.Hour
	MinInviteLifetime     = 5 * time.Minute
	// MaxInviteLifetime matches the campaign_invites_lifetime CHECK and
	// bounds how long an invite row lives (docs/privacy.md).
	MaxInviteLifetime = 30 * 24 * time.Hour
)

// Profiles tells what to call users. The identity module implements it
// (identity.PostgresStore.DisplayNames), so this package never reads the
// users table itself.
type Profiles interface {
	// DisplayNames returns the display names of the given users, keyed by
	// user ID. Users without one are left out.
	DisplayNames(ctx context.Context, userIDs []string) (map[string]string, error)
	// VerifiedEmails returns a user's verified e-mail addresses, for the
	// allow-list of who may create campaigns (RN-30). Empty when the provider
	// did not verify one.
	VerifiedEmails(ctx context.Context, userID string) ([]string, error)
}

// PendingMemberLifetime is how long a pending membership without a
// character lasts before the database deletes it (RN-15). Migration 00035
// backfills the same 30 days.
const PendingMemberLifetime = 30 * 24 * time.Hour

// Characters approves a pending member's character when an invite without
// approval promotes them (RN-15, Q25). The characters module implements it
// (characters.Service); cmd/api connects the two with SetCharacters, and
// neither package imports the other.
type Characters interface {
	// ApprovePendingCharacter approves userID's character waiting for
	// approval in campaignID, inside tx, exactly as the master's
	// ApproveCharacter would. Without such a character, nothing changes.
	ApprovePendingCharacter(ctx context.Context, tx pgx.Tx, campaignID, userID string) error
}

// Config holds what the campaigns service needs.
type Config struct {
	// Pool is the CockroachDB connection pool. Required.
	Pool *pgxpool.Pool
	// Profiles gives members' display names. Required.
	Profiles Profiles
	// MaxCampaignsPerUser is how many campaigns one account may be master of
	// (RN-30). Zero means DefaultMaxCampaignsPerUser; negative means no cap
	// (MAX_CAMPAIGNS_PER_USER=off, for the local and CI stacks only).
	MaxCampaignsPerUser int
	// Creators are the verified e-mails, in lower case, allowed to create
	// campaigns (RN-30). Empty means anyone.
	Creators []string
	// Logger receives errors, without personal data. Nil means
	// slog.Default().
	Logger *slog.Logger
	// Now returns the current time. Nil means time.Now. Tests move it
	// forward, e.g. past an invite's 7 days.
	Now func() time.Time
}

// Service implements the CampaignService and CampaignDocumentService
// Connect APIs and authz.MembershipSource.
type Service struct {
	pool     *pgxpool.Pool
	queries  *campaignsdb.Queries
	profiles Profiles
	logger   *slog.Logger
	now      func() time.Time
	// maxCampaigns and creators are the abuse limits of CreateCampaign (RN-30).
	maxCampaigns int
	creators     []string
	// characters is set once, before the service handles any call
	// (SetCharacters).
	characters Characters
	// xpAwards is set once, before the service handles any call (SetXPAwards).
	xpAwards XPAwards
}

// The compiler checks that Service implements both interfaces.
var (
	_ campaignsv1connect.CampaignServiceHandler = (*Service)(nil)
	_ authz.MembershipSource                    = (*Service)(nil)
)

// New returns a Service.
func New(cfg Config) (*Service, error) {
	if cfg.Pool == nil {
		return nil, errors.New("campaigns: a Pool is required")
	}
	if cfg.Profiles == nil {
		return nil, errors.New("campaigns: Profiles is required")
	}
	s := &Service{
		pool:     cfg.Pool,
		queries:  campaignsdb.New(cfg.Pool),
		profiles: cfg.Profiles,
		logger:   cfg.Logger,
		now:      cfg.Now,

		maxCampaigns: cfg.MaxCampaignsPerUser,
		creators:     cfg.Creators,
	}
	if s.maxCampaigns == 0 {
		s.maxCampaigns = DefaultMaxCampaignsPerUser
	}
	if s.logger == nil {
		s.logger = slog.Default()
	}
	if s.now == nil {
		s.now = time.Now
	}
	return s, nil
}

// SetCharacters says which service approves a pending member's character
// (see Characters). It is a setter, not a Config field, because the two
// services need each other (characters.Config.Members is this service), so
// one of them must exist before the other can be given to it. Call it once,
// before Mount and before any call; without it, an invite that promotes a
// pending member fails.
func (s *Service) SetCharacters(c Characters) { s.characters = c }

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

// Mount registers CampaignService and CampaignDocumentService on a mux.
// handle is usually httpserver.Server.Handle or http.ServeMux.Handle.
//
// sessions tells who is calling (the identity service in production). Mount
// adds its interceptor, then the authz interceptor, backed by sessions and
// by this service's campaign_members, and one that marks every response
// `Cache-Control: no-store`, to both services. opts are the Connect options
// shared by every service.
func (s *Service) Mount(handle func(pattern string, handler http.Handler), sessions Sessions, opts ...connect.HandlerOption) {
	// Clip so append copies instead of writing into the caller's array.
	opts = append(slices.Clip(opts), connect.WithInterceptors(
		noStore{},                                // no response is cacheable
		sessions.Interceptor(),                   // who is calling
		authz.Interceptor(sessions, s, s.logger), // what they may do, memoized per request
	))
	handle(campaignsv1connect.NewCampaignServiceHandler(s, opts...))
	handle(campaignsv1connect.NewCampaignDocumentServiceHandler(s, opts...))
}

// noStore marks every response, errors included, `Cache-Control: no-store`:
// they all describe the caller's campaigns. It uses Set, so a response that
// already says so (identity's unauthenticated error does) is not sent the
// header twice.
type noStore struct{}

// WrapUnary implements connect.Interceptor.
func (noStore) WrapUnary(next connect.UnaryFunc) connect.UnaryFunc {
	return func(ctx context.Context, req connect.AnyRequest) (connect.AnyResponse, error) {
		if req.Spec().IsClient {
			return next(ctx, req)
		}
		res, err := next(ctx, req)
		if err != nil {
			if connectErr, ok := errors.AsType[*connect.Error](err); ok {
				connectErr.Meta().Set("Cache-Control", "no-store")
			}
			return nil, err
		}
		res.Header().Set("Cache-Control", "no-store")
		return res, nil
	}
}

// WrapStreamingClient implements connect.Interceptor; clients pass through.
func (noStore) WrapStreamingClient(next connect.StreamingClientFunc) connect.StreamingClientFunc {
	return next
}

// WrapStreamingHandler implements connect.Interceptor.
func (noStore) WrapStreamingHandler(next connect.StreamingHandlerFunc) connect.StreamingHandlerFunc {
	return func(ctx context.Context, conn connect.StreamingHandlerConn) error {
		conn.ResponseHeader().Set("Cache-Control", "no-store")
		return next(ctx, conn)
	}
}

// CampaignMembership implements authz.MembershipSource with one
// primary-key read of campaign_members.
func (s *Service) CampaignMembership(ctx context.Context, campaignID, userID string) (authz.Role, authz.Status, error) {
	m, err := s.queries.GetMembership(ctx, campaignsdb.GetMembershipParams{CampaignID: campaignID, UserID: userID, Now: s.now()})
	if errors.Is(err, pgx.ErrNoRows) {
		return "", "", authz.ErrNotMember
	}
	if err != nil {
		return "", "", fmt.Errorf("get membership: %w", err)
	}
	return authz.Role(m.Role), authz.Status(m.Status), nil
}

// ActiveCampaigns returns the campaigns userID is an active member of, as
// master or player, newest first, each with its name and the user's role
// (Campaign.my_role). Pending memberships (RN-15) are left out: a pending
// member is not a member.
//
// Package play calls it from ListOpenGameSessions, the in-app notice that a
// session started (RN-06), with the user ID from its own authorization
// check. It reads campaign_members through the index on user_id.
func (s *Service) ActiveCampaigns(ctx context.Context, userID string) ([]*campaignsv1.Campaign, error) {
	rows, err := s.queries.ListCampaignsOfUser(ctx, userID)
	if err != nil {
		return nil, fmt.Errorf("list campaigns of user: %w", err)
	}
	var out []*campaignsv1.Campaign
	for _, row := range rows {
		if row.Status == string(authz.StatusActive) {
			out = append(out, campaignToProto(row.Campaign, authz.Role(row.Role), false))
		}
	}
	return out, nil
}

// ActivatePendingMember makes userID's pending membership of campaignID an
// ordinary one, inside tx (RN-15, MR-024). An active membership, or none,
// stays as it is.
//
// Package characters calls it from ApproveCharacter, in the transaction
// that approves the character, after checking that the caller is the
// campaign's master. It takes no caller on purpose, like LockSheets: the
// check is ApproveCharacter's. Nothing else calls it.
func (s *Service) ActivatePendingMember(ctx context.Context, tx pgx.Tx, campaignID, userID string) error {
	if _, err := s.queries.WithTx(tx).ActivatePendingMember(ctx, campaignsdb.ActivatePendingMemberParams{CampaignID: campaignID, UserID: userID}); err != nil {
		return fmt.Errorf("activate pending member: %w", err)
	}
	return nil
}

// ClearPendingExpiry removes the 30-day deadline of userID's pending
// membership of campaignID, inside tx (RN-15). Package characters calls it
// from CreateCharacter, in the transaction that creates the pending
// member's character: the deadline only applies while they have none.
// Nothing else calls it.
func (s *Service) ClearPendingExpiry(ctx context.Context, tx pgx.Tx, campaignID, userID string) error {
	if _, err := s.queries.WithTx(tx).ClearPendingExpiry(ctx, campaignsdb.ClearPendingExpiryParams{CampaignID: campaignID, UserID: userID, Now: s.now()}); err != nil {
		return fmt.Errorf("clear pending expiry: %w", err)
	}
	return nil
}

// DeletePendingMember deletes userID's pending membership of campaignID,
// inside tx (RN-15, MR-024). An active membership is never deleted.
//
// Package characters calls it from RejectCharacter, in the transaction that
// deletes the rejected character, after checking that the caller is the
// campaign's master. Nothing else calls it.
func (s *Service) DeletePendingMember(ctx context.Context, tx pgx.Tx, campaignID, userID string) error {
	if _, err := s.queries.WithTx(tx).DeletePendingMember(ctx, campaignsdb.DeletePendingMemberParams{CampaignID: campaignID, UserID: userID}); err != nil {
		return fmt.Errorf("delete pending member: %w", err)
	}
	return nil
}

// queriesIn is the queries on the transaction, or on the pool when tx is nil. A
// read made while the caller holds a transaction must use the transaction: a
// read through the pool takes a second connection, and a few such requests
// at once hold every connection of the pool, each waiting for another (see
// docs/architecture.md#transactions-and-the-connection-pool).
func (s *Service) queriesIn(tx pgx.Tx) *campaignsdb.Queries {
	if tx == nil {
		return s.queries
	}
	return s.queries.WithTx(tx)
}

// CheckWired fails when a collaborator that cmd/api connects after New (the
// modules need each other, so a setter does it) is still nil. A nil one does
// not fail loudly later: the feature quietly does not happen. cmd/api calls it
// once the wiring is done, so a refactor that drops a Set... call stops the
// server at startup instead.
func (s *Service) CheckWired() error {
	return wiring.Check("campaigns",
		wiring.Dep{Setter: "SetCharacters", Missing: s.characters == nil},
		wiring.Dep{Setter: "SetXPAwards", Missing: s.xpAwards == nil})
}
