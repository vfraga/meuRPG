package campaigns

import (
	"bytes"
	"context"
	"encoding/hex"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"net/url"
	"slices"
	"strings"
	"testing"
	"time"

	"connectrpc.com/connect"
	"github.com/jackc/pgx/v5/pgxpool"
	"google.golang.org/protobuf/reflect/protoreflect"
	"google.golang.org/protobuf/types/descriptorpb"
	"google.golang.org/protobuf/types/known/durationpb"

	campaignsv1 "github.com/PuraFome/meuRPG/backend/gen/meurpg/campaigns/v1"
	"github.com/PuraFome/meuRPG/backend/gen/meurpg/campaigns/v1/campaignsv1connect"
	"github.com/PuraFome/meuRPG/backend/internal/campaigns/campaignsdb"
	"github.com/PuraFome/meuRPG/backend/internal/identity"
	"github.com/PuraFome/meuRPG/backend/internal/platform/reqlimit"
	"github.com/PuraFome/meuRPG/backend/internal/platform/secret"
)

// These tests need no database, so they also run in `go test ./...`
// without MEURPG_TEST_DATABASE_URL.

func TestInviteState(t *testing.T) {
	t.Parallel()
	now := time.Now()
	revokedAt := now.Add(-time.Minute)
	tests := []struct {
		name   string
		invite campaignsdb.CampaignInvite
		want   campaignsv1.InviteState
	}{
		{"active", campaignsdb.CampaignInvite{MaxUses: 1, ExpiresAt: now.Add(time.Second)}, campaignsv1.InviteState_INVITE_STATE_ACTIVE},
		{"expired at expires_at", campaignsdb.CampaignInvite{MaxUses: 1, ExpiresAt: now}, campaignsv1.InviteState_INVITE_STATE_EXPIRED},
		{"used up", campaignsdb.CampaignInvite{MaxUses: 2, UseCount: 2, ExpiresAt: now.Add(time.Hour)}, campaignsv1.InviteState_INVITE_STATE_USED_UP},
		{"used up wins over expired", campaignsdb.CampaignInvite{MaxUses: 1, UseCount: 1, ExpiresAt: now}, campaignsv1.InviteState_INVITE_STATE_USED_UP},
		{"revoked wins over everything", campaignsdb.CampaignInvite{MaxUses: 1, UseCount: 1, ExpiresAt: now, RevokedAt: &revokedAt}, campaignsv1.InviteState_INVITE_STATE_REVOKED},
	}
	for _, tt := range tests {
		if got := inviteState(tt.invite, now); got != tt.want {
			t.Errorf("%s: inviteState() = %v, want %v", tt.name, got, tt.want)
		}
	}
}

func TestInviteLimits(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name         string
		maxUses      int32
		expiresIn    *durationpb.Duration
		wantUses     int32
		wantLifetime time.Duration // 0: want invalid_argument
	}{
		{"defaults: one use, 7 days", 0, nil, 1, 7 * 24 * time.Hour},
		{"most uses", MaxInviteUses, nil, MaxInviteUses, DefaultInviteLifetime},
		{"too many uses", MaxInviteUses + 1, nil, 0, 0},
		{"negative uses", -1, nil, 0, 0},
		{"shortest", 1, durationpb.New(MinInviteLifetime), 1, MinInviteLifetime},
		{"too short", 1, durationpb.New(MinInviteLifetime - time.Second), 0, 0},
		{"longest", 1, durationpb.New(MaxInviteLifetime), 1, MaxInviteLifetime},
		{"too long", 1, durationpb.New(MaxInviteLifetime + time.Second), 0, 0},
		{"zero", 1, durationpb.New(0), 0, 0},
		{"not a valid duration", 1, &durationpb.Duration{Seconds: 3600, Nanos: -1}, 0, 0},
	}
	for _, tt := range tests {
		uses, lifetime, err := inviteLimits(&campaignsv1.CreateInviteRequest{MaxUses: tt.maxUses, ExpiresIn: tt.expiresIn})
		if tt.wantLifetime == 0 {
			if connect.CodeOf(err) != connect.CodeInvalidArgument {
				t.Errorf("%s: inviteLimits() error = %v, want invalid_argument", tt.name, err)
			}
			continue
		}
		if err != nil || uses != tt.wantUses || lifetime != tt.wantLifetime {
			t.Errorf("%s: inviteLimits() = %d, %v, %v; want %d, %v", tt.name, uses, lifetime, err, tt.wantUses, tt.wantLifetime)
		}
	}
}

// TestXpModes: every XP mode of the API maps to a value that the
// campaigns_xp_mode_valid CHECK accepts, and back.
func TestXpModes(t *testing.T) {
	t.Parallel()
	for number := range campaignsv1.XpMode_name {
		mode := campaignsv1.XpMode(number)
		if mode == campaignsv1.XpMode_XP_MODE_UNSPECIFIED {
			continue
		}
		dbValue, ok := xpModeToDB[mode]
		if !ok {
			t.Errorf("%v has no database value", mode)
		}
		if xpModeFromDB[dbValue] != mode {
			t.Errorf("%v -> %q does not map back", mode, dbValue)
		}
	}
	if len(xpModeFromDB) != 3 {
		t.Errorf("xpModeFromDB has %d values; the CHECK allows enemies, gold and milestones", len(xpModeFromDB))
	}
}

func TestNewValidatesItsConfig(t *testing.T) {
	t.Parallel()
	if _, err := New(Config{Profiles: noProfiles{}}); err == nil {
		t.Error("New() without a Pool succeeded")
	}
	if _, err := New(Config{Pool: lazyPool(t)}); err == nil {
		t.Error("New() without Profiles succeeded")
	}
}

// TestEveryMethodNeedsASession calls every method signed out. The session
// check comes first, so no database is needed to see it refuse.
func TestEveryMethodNeedsASession(t *testing.T) {
	t.Parallel()
	svc, err := New(Config{Pool: lazyPool(t), Profiles: noProfiles{}, Logger: slog.New(slog.DiscardHandler)})
	if err != nil {
		t.Fatalf("New() error = %v", err)
	}
	mux := http.NewServeMux()
	svc.Mount(mux.Handle, testSessions, reqlimit.HandlerOption())
	server := httptest.NewServer(reqlimit.Middleware(mux))
	t.Cleanup(server.Close)
	c := campaignsv1connect.NewCampaignServiceClient(server.Client(), server.URL)
	ctx := t.Context()
	id := "6f1c7a52-3b5e-4c55-9d0b-2a51f0c1e001"

	calls := map[string]error{}
	_, calls["CreateCampaign"] = c.CreateCampaign(ctx, connect.NewRequest(&campaignsv1.CreateCampaignRequest{Name: "Mirathel", XpMode: campaignsv1.XpMode_XP_MODE_GOLD}))
	_, calls["ListMyCampaigns"] = c.ListMyCampaigns(ctx, connect.NewRequest(&campaignsv1.ListMyCampaignsRequest{}))
	_, calls["GetCampaign"] = c.GetCampaign(ctx, connect.NewRequest(&campaignsv1.GetCampaignRequest{CampaignId: id}))
	_, calls["ListMembers"] = c.ListMembers(ctx, connect.NewRequest(&campaignsv1.ListMembersRequest{CampaignId: id}))
	_, calls["ListPendingMembers"] = c.ListPendingMembers(ctx, connect.NewRequest(&campaignsv1.ListPendingMembersRequest{CampaignId: id}))
	_, calls["RemovePendingMember"] = c.RemovePendingMember(ctx, connect.NewRequest(&campaignsv1.RemovePendingMemberRequest{CampaignId: id, UserId: id}))
	_, calls["CreateInvite"] = c.CreateInvite(ctx, connect.NewRequest(&campaignsv1.CreateInviteRequest{CampaignId: id}))
	_, calls["ListInvites"] = c.ListInvites(ctx, connect.NewRequest(&campaignsv1.ListInvitesRequest{CampaignId: id}))
	_, calls["RevokeInvite"] = c.RevokeInvite(ctx, connect.NewRequest(&campaignsv1.RevokeInviteRequest{CampaignId: id, InviteId: id}))
	_, calls["SetCampaignDiceMode"] = c.SetCampaignDiceMode(ctx, connect.NewRequest(&campaignsv1.SetCampaignDiceModeRequest{CampaignId: id, Mode: campaignsv1.DiceMode_DICE_MODE_APP}))
	_, calls["SetMyDicePreference"] = c.SetMyDicePreference(ctx, connect.NewRequest(&campaignsv1.SetMyDicePreferenceRequest{CampaignId: id, Preference: campaignsv1.DicePreference_DICE_PREFERENCE_APP}))
	_, calls["GetTableRules"] = c.GetTableRules(ctx, connect.NewRequest(&campaignsv1.GetTableRulesRequest{CampaignId: id}))
	_, calls["SetTableRules"] = c.SetTableRules(ctx, connect.NewRequest(&campaignsv1.SetTableRulesRequest{CampaignId: id, Rules: validRules()}))
	_, calls["SetCampaignXpMode"] = c.SetCampaignXpMode(ctx, connect.NewRequest(&campaignsv1.SetCampaignXpModeRequest{CampaignId: id, XpMode: campaignsv1.XpMode_XP_MODE_GOLD}))
	_, calls["AcceptInvite"] = c.AcceptInvite(ctx, connect.NewRequest(&campaignsv1.AcceptInviteRequest{Token: "x"}))

	methods := campaignsv1.File_meurpg_campaigns_v1_campaigns_proto.Services().ByName("CampaignService").Methods()
	if len(calls) != methods.Len() {
		t.Errorf("called %d methods, the service has %d", len(calls), methods.Len())
	}
	for name, err := range calls {
		if connect.CodeOf(err) != connect.CodeUnauthenticated {
			t.Errorf("%s signed out: error = %v, want unauthenticated", name, err)
		}
		if ce, ok := errors.AsType[*connect.Error](err); !ok || !slices.Equal(ce.Meta().Values("Cache-Control"), []string{"no-store"}) {
			t.Errorf("%s: error response Cache-Control = %q, want no-store, once", name, ce.Meta().Values("Cache-Control"))
		}
	}
}

// lazyPool is a pool that never connects: pgxpool connects on first use,
// and these tests never get that far.
func lazyPool(t *testing.T) *pgxpool.Pool {
	t.Helper()
	pool, err := pgxpool.New(t.Context(), "postgresql://nobody@127.0.0.1:1/none")
	if err != nil {
		t.Fatalf("pgxpool.New() error = %v", err)
	}
	t.Cleanup(pool.Close)
	return pool
}

type noProfiles struct{}

func (noProfiles) DisplayNames(context.Context, []string) (map[string]string, error) {
	return nil, errors.New("no profiles in this test")
}

func (noProfiles) VerifiedEmails(context.Context, string) ([]string, error) {
	return nil, errors.New("no profiles in this test")
}

// TestInviteIntentPrepare: at login start, only a well-formed token is
// accepted, and what is kept is its hash, never the token.
func TestInviteIntentPrepare(t *testing.T) {
	t.Parallel()
	intent := inviteIntent{}
	token, hash := secret.New()

	got, err := intent.Prepare(token)
	if err != nil || !bytes.Equal(got, hash) {
		t.Errorf("Prepare(token) = %x, %v; want the token's SHA-256 %x", got, err, hash)
	}
	for name, payload := range map[string]string{
		"empty":     "",
		"garbage":   "not a token",
		"padded":    token + "=",
		"too long":  token + "A",
		"too short": token[:42],
	} {
		if data, err := intent.Prepare(payload); err == nil {
			t.Errorf("Prepare(%s) = %x, want an error", name, data)
		} else if strings.Contains(err.Error(), payload) && payload != "" {
			t.Errorf("Prepare(%s) error %q repeats the payload", name, err)
		}
	}
}

// TestInviteIntentCompleteRefusesTheZeroSignedIn: identity.SignedIn{} is
// all this package (or any other but identity) can build, and it must not
// act as anyone. It is refused before the database is touched: the pool here
// cannot even connect.
func TestInviteIntentCompleteRefusesTheZeroSignedIn(t *testing.T) {
	t.Parallel()
	svc, err := New(Config{Pool: lazyPool(t), Profiles: noProfiles{}, Logger: slog.New(slog.DiscardHandler)})
	if err != nil {
		t.Fatalf("New() error = %v", err)
	}
	_, hash := secret.New()
	path, err := svc.InviteIntent().Complete(t.Context(), identity.SignedIn{}, hash)
	if err == nil || path != "" {
		t.Errorf("Complete(SignedIn{}) = %q, %v; want no path and an error", path, err)
	}
}

// TestInviteIntentCompleteWithoutADatabase covers the failures that never
// reach an invite: corrupt stored data, and a database that does not
// answer. It calls complete, the step after the SignedIn check, because only
// identity can build a SignedIn with a user; the whole Complete runs through
// identity's real callback in signin_test.go.
func TestInviteIntentCompleteWithoutADatabase(t *testing.T) {
	t.Parallel()
	svc, err := New(Config{Pool: lazyPool(t), Profiles: noProfiles{}, Logger: slog.New(slog.DiscardHandler)})
	if err != nil {
		t.Fatalf("New() error = %v", err)
	}
	intent := inviteIntent{s: svc}
	_, hash := secret.New()

	path, err := intent.complete(t.Context(), "user", []byte("short"))
	if path != "/invite/error?reason=invalid" || err == nil {
		t.Errorf("complete(corrupt data) = %q, %v; want the invalid page and an error", path, err)
	}
	path, err = intent.complete(t.Context(), "user", hash)
	if path != "/invite/error?reason=unavailable" || err == nil {
		t.Errorf("complete() with the database down = %q, %v; want the unavailable page and an error", path, err)
	}
	if err != nil && strings.Contains(err.Error(), hex.EncodeToString(hash)) {
		t.Errorf("error %q contains the hash", err)
	}
}

func TestInviteFailureReasons(t *testing.T) {
	t.Parallel()
	for err, want := range map[error]string{
		&unusableInviteError{state: campaignsv1.InviteState_INVITE_STATE_EXPIRED}: InviteFailureExpired,
		&unusableInviteError{state: campaignsv1.InviteState_INVITE_STATE_REVOKED}: InviteFailureRevoked,
		&unusableInviteError{state: campaignsv1.InviteState_INVITE_STATE_USED_UP}: InviteFailureUsedUp,
		&unusableInviteError{state: campaignsv1.InviteState_INVITE_STATE_ACTIVE}:  InviteFailureInvalid,
		fmt.Errorf("in a transaction: %w", errNoInvite):                           InviteFailureNotFound,
		errors.New("connection refused"):                                          InviteFailureUnavailable,
	} {
		if got := inviteFailure(err); got != want {
			t.Errorf("inviteFailure(%v) = %q, want %q", err, got, want)
		}
	}
}

// TestReadsWithIDsArePostOnly: the reads that carry a campaign ID are
// IDEMPOTENT, not NO_SIDE_EFFECTS, so the server refuses them over GET,
// which would put the ID in the URL. ListMyCampaigns, with an empty
// request, is the one read that GET reaches.
func TestReadsWithIDsArePostOnly(t *testing.T) {
	t.Parallel()
	svc, err := New(Config{Pool: lazyPool(t), Profiles: noProfiles{}, Logger: slog.New(slog.DiscardHandler)})
	if err != nil {
		t.Fatalf("New() error = %v", err)
	}
	mux := http.NewServeMux()
	svc.Mount(mux.Handle, testSessions, reqlimit.HandlerOption())

	get := func(procedure, message string) int {
		t.Helper()
		target := procedure + "?connect=v1&encoding=json&message=" + url.QueryEscape(message)
		rec := httptest.NewRecorder()
		mux.ServeHTTP(rec, httptest.NewRequestWithContext(t.Context(), http.MethodGet, target, nil))
		return rec.Code
	}
	id := `{"campaignId":"6f1c7a52-3b5e-4c55-9d0b-2a51f0c1e001"}`
	for procedure, message := range map[string]string{
		campaignsv1connect.CampaignServiceGetCampaignProcedure: id,
		campaignsv1connect.CampaignServiceListMembersProcedure: id,
		campaignsv1connect.CampaignServiceListInvitesProcedure: id,
	} {
		if code := get(procedure, message); code != http.StatusMethodNotAllowed {
			t.Errorf("GET %s: status = %d, want 405", procedure, code)
		}
	}
	// ListMyCampaigns reaches the handler over GET (and is refused there,
	// signed out).
	if code := get(campaignsv1connect.CampaignServiceListMyCampaignsProcedure, "{}"); code != http.StatusUnauthorized {
		t.Errorf("GET ListMyCampaigns: status = %d, want 401 from the handler", code)
	}

	methods := campaignsv1.File_meurpg_campaigns_v1_campaigns_proto.Services().ByName("CampaignService").Methods()
	for _, name := range []string{"GetCampaign", "ListMembers", "ListInvites"} {
		opts, _ := methods.ByName(protoreflect.Name(name)).Options().(*descriptorpb.MethodOptions)
		if opts.GetIdempotencyLevel() != descriptorpb.MethodOptions_IDEMPOTENT {
			t.Errorf("%s idempotency_level = %v, want IDEMPOTENT", name, opts.GetIdempotencyLevel())
		}
	}
}
