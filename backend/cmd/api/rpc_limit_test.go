package main

import (
	"context"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/PuraFome/meuRPG/backend/gen/meurpg/system/v1/systemv1connect"
	"github.com/PuraFome/meuRPG/backend/internal/identity"
	"github.com/PuraFome/meuRPG/backend/internal/platform/config"
	"github.com/PuraFome/meuRPG/backend/internal/platform/dbtest"
	"github.com/PuraFome/meuRPG/backend/internal/platform/ratelimit"
	"github.com/PuraFome/meuRPG/backend/internal/platform/secret"
	"github.com/PuraFome/meuRPG/backend/internal/rules"
	"github.com/PuraFome/meuRPG/backend/internal/system"
)

func rpcStatus(t *testing.T, h http.Handler, procedure, token string) int {
	t.Helper()
	req := httptest.NewRequestWithContext(t.Context(), http.MethodPost, procedure, strings.NewReader("{}"))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Connect-Protocol-Version", "1")
	req.AddCookie(&http.Cookie{Name: identity.SessionCookieName, Value: token}) //nolint:gosec // G124: a request cookie in a test
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	return rec.Code
}

// The identity RPCs write to the database (profile, sign-out of other
// sessions), so they get the same per-user limit as every other service.
func TestIdentityRPCsAreRateLimitedPerUser(t *testing.T) {
	pool := dbtest.NewPool(t, "identityrpclimit")
	ctx := context.Background()
	logger := slog.New(slog.DiscardHandler)
	content, err := rules.LoadSRD()
	must(t, err)
	m, err := wireModules(logger, pool, content, nil, nil, wireOptions{MonthlyImages: 20})
	must(t, err)
	t.Cleanup(m.play.Close)
	svc, err := identity.New(ctx, identity.Config{
		OIDC: config.OIDC{
			IssuerURL: "http://localhost:1", ClientID: "test", ClientSecret: "test",
			RedirectURL: "http://localhost:8080/auth/callback",
		},
		Store: m.users, Logger: logger,
	})
	must(t, err)

	userID, err := m.users.UpsertUser(ctx, identity.ExternalIdentity{Issuer: "http://x", Subject: "limit-a"})
	must(t, err)
	token, hash := secret.New()
	now := time.Now()
	insertSession(t, pool, hash, userID, now)

	burst := 3
	limiter := ratelimit.New(ratelimit.Config{
		PerClient:  ratelimit.Rate{Burst: burst, Every: time.Hour},
		Global:     ratelimit.Rate{Burst: 1000, Every: time.Millisecond},
		MaxClients: 100,
	})
	mux := http.NewServeMux()
	opts := connectOptions(logger)
	mux.Handle(systemv1connect.NewSystemServiceHandler(system.NewService(version, commit), opts...))
	mountModules(mux.Handle, m, svc, opts, limiter, logger)

	const identityProc = "/meurpg.identity.v1.IdentityService/CountOtherSessions"
	const campaignProc = "/meurpg.campaigns.v1.CampaignService/ListMyCampaigns"

	// Control: a campaigns call past the burst is limited (429 for connect resource_exhausted).
	limited := false
	for range burst + 5 {
		if rpcStatus(t, mux, campaignProc, token) == http.StatusTooManyRequests {
			limited = true
		}
	}
	if !limited {
		t.Fatalf("control failed: campaigns calls were never limited")
	}

	// A fresh user's identity calls must be limited too (a fresh user, so the
	// campaigns calls above did not spend the budget).
	userID2, err := m.users.UpsertUser(ctx, identity.ExternalIdentity{Issuer: "http://x", Subject: "limit-b"})
	must(t, err)
	token2, hash2 := secret.New()
	insertSession(t, pool, hash2, userID2, now)
	limited = false
	for range burst + 20 {
		if rpcStatus(t, mux, identityProc, token2) == http.StatusTooManyRequests {
			limited = true
		}
	}
	if !limited {
		t.Errorf("%d identity calls with burst %d were never rate limited", burst+20, burst)
	}
}

func insertSession(t *testing.T, pool *pgxpool.Pool, hash []byte, userID string, now time.Time) {
	t.Helper()
	_, err := pool.Exec(context.Background(),
		`INSERT INTO auth_sessions (token_hash, user_id, created_at, expires_at) VALUES ($1, $2, $3, $4)`,
		hash, userID, now, now.Add(24*time.Hour))
	must(t, err)
}
