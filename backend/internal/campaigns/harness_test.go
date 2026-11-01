package campaigns

import (
	"context"
	"crypto/rand"
	"errors"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"slices"
	"sync"
	"testing"
	"time"

	"connectrpc.com/connect"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"google.golang.org/protobuf/types/known/durationpb"

	campaignsv1 "github.com/PuraFome/meuRPG/backend/gen/meurpg/campaigns/v1"
	"github.com/PuraFome/meuRPG/backend/gen/meurpg/campaigns/v1/campaignsv1connect"
	"github.com/PuraFome/meuRPG/backend/internal/identity"
	"github.com/PuraFome/meuRPG/backend/internal/platform/dbtest"
	"github.com/PuraFome/meuRPG/backend/internal/platform/reqlimit"
)

// These tests run against CockroachDB, because the rules they check live in
// SQL transactions: set MEURPG_TEST_DATABASE_URL (see package dbtest).
// Without it they skip.

// testUserHeader names the signed-in user in a test request. It exists only
// in these tests: fakeSessions stands in for the identity module, whose
// interceptor reads a session cookie and looks it up in the database.
// Everything after it (the authz interceptor, the handlers, the SQL) is the
// production code.
const testUserHeader = "Test-User-Id"

// fakeSessions implements Sessions for tests: its interceptor trusts the
// Test-User-Id header, and UserID reads it back. The context key is private
// to these tests; production code has no way to set a caller.
type fakeSessions struct{}

// testUserKey is fakeSessions' context key.
type testUserKey struct{}

var testSessions Sessions = fakeSessions{}

func (fakeSessions) Interceptor() connect.Interceptor {
	return connect.UnaryInterceptorFunc(func(next connect.UnaryFunc) connect.UnaryFunc {
		return func(ctx context.Context, req connect.AnyRequest) (connect.AnyResponse, error) {
			if userID := req.Header().Get(testUserHeader); userID != "" {
				ctx = context.WithValue(ctx, testUserKey{}, userID)
			}
			return next(ctx, req)
		}
	})
}

func (fakeSessions) UserID(ctx context.Context) (string, error) {
	if userID, ok := ctx.Value(testUserKey{}).(string); ok {
		return userID, nil
	}
	return "", connect.NewError(connect.CodeUnauthenticated, errors.New("sign in to continue"))
}

// fakeClock is a clock the test moves by hand.
type fakeClock struct {
	mu  sync.Mutex
	now time.Time
}

func (c *fakeClock) Now() time.Time {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.now
}

func (c *fakeClock) Advance(d time.Duration) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.now = c.now.Add(d)
}

// harness is a Service on a fresh, migrated database, served over real HTTP
// the way cmd/api serves it.
type harness struct {
	t      *testing.T
	pool   *pgxpool.Pool
	users  *identity.PostgresStore // creates accounts and display names
	clock  *fakeClock
	server *httptest.Server
	// service is the campaigns service behind server, for calls that are not RPCs.
	service *Service
	// characters stands in for package characters (Characters).
	characters fakeCharacters
}

// fakeCharacters is a Characters that records the players whose pending
// character it was asked to approve.
type fakeCharacters struct {
	mu       sync.Mutex
	approved []string // user IDs
}

func (f *fakeCharacters) ApprovePendingCharacter(_ context.Context, _ pgx.Tx, _, userID string) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.approved = append(f.approved, userID)
	return nil
}

func (f *fakeCharacters) approvedUsers() []string {
	f.mu.Lock()
	defer f.mu.Unlock()
	return slices.Clone(f.approved)
}

func newHarness(t *testing.T) *harness {
	t.Helper()
	return newHarnessWith(t, nil)
}

// newHarnessWith is newHarness with the chance to change the service's Config.
func newHarnessWith(t *testing.T, change func(*Config)) *harness {
	t.Helper()
	pool := dbtest.NewPool(t, "meurpg_campaigns_test")
	h := &harness{
		t:     t,
		pool:  pool,
		users: identity.NewPostgresStore(pool),
		// The database's precision is microseconds; truncating keeps
		// timestamps equal after a round trip.
		clock: &fakeClock{now: time.Now().Truncate(time.Microsecond)},
	}
	cfg := Config{
		Pool:     pool,
		Profiles: h.users,
		Logger:   slog.New(slog.DiscardHandler),
		Now:      h.clock.Now,
	}
	if change != nil {
		change(&cfg)
	}
	svc, err := New(cfg)
	if err != nil {
		t.Fatalf("New() error = %v", err)
	}
	// The real approval is package characters' (and its tests'); here a fake
	// that only records who was approved, and never has a character.
	svc.SetCharacters(&h.characters)
	h.service = svc
	mux := http.NewServeMux()
	svc.Mount(mux.Handle, testSessions, reqlimit.HandlerOption())
	h.server = httptest.NewServer(reqlimit.Middleware(mux))
	t.Cleanup(h.server.Close)
	return h
}

// user is a signed-in account with its own API clients.
type user struct {
	id  string
	api campaignsv1connect.CampaignServiceClient
	// doc calls CampaignDocumentService (MR-018) as this user.
	doc campaignsv1connect.CampaignDocumentServiceClient
}

// newUser creates an account, as a first sign-in would, and sets its
// display name when one is given.
func (h *harness) newUser(displayName string) *user {
	h.t.Helper()
	id, err := h.users.UpsertUser(h.t.Context(), identity.ExternalIdentity{
		Issuer:  "https://idp.test",
		Subject: rand.Text(), // a new person every time
	})
	if err != nil {
		h.t.Fatalf("UpsertUser() error = %v", err)
	}
	if displayName != "" {
		if err := h.users.SetDisplayName(h.t.Context(), id, displayName); err != nil {
			h.t.Fatalf("SetDisplayName() error = %v", err)
		}
	}
	return &user{id: id, api: h.client(id), doc: h.documentClient(id)}
}

// anonymous returns a client with no session.
func (h *harness) anonymous() campaignsv1connect.CampaignServiceClient {
	return h.client("")
}

func (h *harness) client(userID string) campaignsv1connect.CampaignServiceClient {
	return campaignsv1connect.NewCampaignServiceClient(h.server.Client(), h.server.URL, clientOptions(userID)...)
}

// documentClient returns a CampaignDocumentService client acting as userID,
// or with no session when userID is empty.
func (h *harness) documentClient(userID string) campaignsv1connect.CampaignDocumentServiceClient {
	return campaignsv1connect.NewCampaignDocumentServiceClient(h.server.Client(), h.server.URL, clientOptions(userID)...)
}

// clientOptions makes a client send userID as the signed-in user; none for
// an empty userID.
func clientOptions(userID string) []connect.ClientOption {
	if userID == "" {
		return nil
	}
	return []connect.ClientOption{connect.WithInterceptors(connect.UnaryInterceptorFunc(
		func(next connect.UnaryFunc) connect.UnaryFunc {
			return func(ctx context.Context, req connect.AnyRequest) (connect.AnyResponse, error) {
				req.Header().Set(testUserHeader, userID)
				return next(ctx, req)
			}
		}))}
}

// createCampaign creates a campaign as u, or fails the test.
func (u *user) createCampaign(t *testing.T, name string) *campaignsv1.Campaign {
	t.Helper()
	res, err := u.api.CreateCampaign(t.Context(), connect.NewRequest(&campaignsv1.CreateCampaignRequest{
		Name:   name,
		XpMode: campaignsv1.XpMode_XP_MODE_MILESTONES,
	}))
	if err != nil {
		t.Fatalf("CreateCampaign(%q) error = %v", name, err)
	}
	return res.Msg.GetCampaign()
}

// createInvite creates an invite as u, or fails the test. maxUses 0 and
// expiresIn 0 mean the defaults.
func (u *user) createInvite(t *testing.T, campaignID string, maxUses int32, expiresIn time.Duration) (*campaignsv1.Invite, string) {
	t.Helper()
	req := &campaignsv1.CreateInviteRequest{CampaignId: campaignID, MaxUses: maxUses}
	if expiresIn != 0 {
		req.ExpiresIn = durationpb.New(expiresIn)
	}
	res, err := u.api.CreateInvite(t.Context(), connect.NewRequest(req))
	if err != nil {
		t.Fatalf("CreateInvite() error = %v", err)
	}
	return res.Msg.GetInvite(), res.Msg.GetToken()
}

// createApprovalInvite creates, as u, a single-use invite whose accepter
// becomes a pending member (RN-15, MR-024), or fails the test.
func (u *user) createApprovalInvite(t *testing.T, campaignID string) (*campaignsv1.Invite, string) {
	t.Helper()
	res, err := u.api.CreateInvite(t.Context(), connect.NewRequest(&campaignsv1.CreateInviteRequest{
		CampaignId: campaignID, RequiresApproval: true,
	}))
	if err != nil {
		t.Fatalf("CreateInvite(requires_approval) error = %v", err)
	}
	return res.Msg.GetInvite(), res.Msg.GetToken()
}

// accept calls AcceptInvite as u.
func (u *user) accept(t *testing.T, token string) (*campaignsv1.AcceptInviteResponse, error) {
	t.Helper()
	res, err := u.api.AcceptInvite(t.Context(), connect.NewRequest(&campaignsv1.AcceptInviteRequest{Token: token}))
	if err != nil {
		return nil, err
	}
	return res.Msg, nil
}

// join makes u a player of the campaign behind token, or fails the test.
func (u *user) join(t *testing.T, token string) {
	t.Helper()
	if _, err := u.accept(t, token); err != nil {
		t.Fatalf("AcceptInvite() error = %v", err)
	}
}

// memberRoles reads campaign_members directly: user ID -> role.
func (h *harness) memberRoles(campaignID string) map[string]string {
	h.t.Helper()
	rows, err := h.pool.Query(h.t.Context(), "SELECT user_id, role FROM campaign_members WHERE campaign_id = $1", campaignID)
	if err != nil {
		h.t.Fatalf("read members: %v", err)
	}
	defer rows.Close()
	roles := map[string]string{}
	for rows.Next() {
		var userID, role string
		if err := rows.Scan(&userID, &role); err != nil {
			h.t.Fatalf("scan member: %v", err)
		}
		roles[userID] = role
	}
	if err := rows.Err(); err != nil {
		h.t.Fatalf("read members: %v", err)
	}
	return roles
}

// memberStatus reads a membership's status directly: "active", "pending",
// or "" when there is no membership.
func (h *harness) memberStatus(campaignID, userID string) string {
	h.t.Helper()
	var status string
	err := h.pool.QueryRow(h.t.Context(), "SELECT status FROM campaign_members WHERE campaign_id = $1 AND user_id = $2", campaignID, userID).Scan(&status)
	if errors.Is(err, pgx.ErrNoRows) {
		return ""
	}
	if err != nil {
		h.t.Fatalf("read member status: %v", err)
	}
	return status
}

// useCount reads an invite's use_count directly.
func (h *harness) useCount(inviteID string) int {
	h.t.Helper()
	var n int
	if err := h.pool.QueryRow(h.t.Context(), "SELECT use_count FROM campaign_invites WHERE id = $1", inviteID).Scan(&n); err != nil {
		h.t.Fatalf("read use_count: %v", err)
	}
	return n
}

// wantCode fails the test unless err has the Connect code want.
func wantCode(t *testing.T, call string, err error, want connect.Code) {
	t.Helper()
	if got := connect.CodeOf(err); err == nil || got != want {
		t.Fatalf("%s error = %v, want %v", call, err, want)
	}
}

// inviteUnusableState returns the state in AcceptInvite's
// failed_precondition detail.
func inviteUnusableState(t *testing.T, err error) campaignsv1.InviteState {
	t.Helper()
	wantCode(t, "AcceptInvite()", err, connect.CodeFailedPrecondition)
	ce, _ := errors.AsType[*connect.Error](err)
	for _, d := range ce.Details() {
		v, err := d.Value()
		if err != nil {
			t.Fatalf("decode error detail: %v", err)
		}
		if detail, ok := v.(*campaignsv1.InviteUnusable); ok {
			return detail.GetState()
		}
	}
	t.Fatalf("AcceptInvite() error %v has no InviteUnusable detail", err)
	return 0
}

// connectMessage returns the message of a Connect error, without the code.
func connectMessage(err error) string {
	if ce, ok := errors.AsType[*connect.Error](err); ok {
		return ce.Message()
	}
	return ""
}
