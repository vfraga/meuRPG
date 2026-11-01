package identity

import (
	"context"
	"errors"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"connectrpc.com/connect"

	identityv1 "github.com/PuraFome/meuRPG/backend/gen/meurpg/identity/v1"
	"github.com/PuraFome/meuRPG/backend/gen/meurpg/identity/v1/identityv1connect"
	"github.com/PuraFome/meuRPG/backend/internal/platform/reqlimit"
	"github.com/PuraFome/meuRPG/backend/internal/platform/secret"
)

func discardLogger() *slog.Logger { return slog.New(slog.DiscardHandler) }

func isUnauthenticated(err error) bool {
	return connect.CodeOf(err) == connect.CodeUnauthenticated
}

func TestGetMeUnauthenticated(t *testing.T) {
	t.Parallel()
	h := newHarness(t)
	unknown, _ := secret.New()

	tests := []struct {
		name   string
		cookie *http.Cookie
	}{
		{"no cookie", nil},
		{"empty cookie", reqCookie(SessionCookieName, "")},
		{"malformed token", reqCookie(SessionCookieName, "not-a-token")},
		{"well-formed token that was never issued", reqCookie(SessionCookieName, unknown)},
		{"the right token under another cookie name", reqCookie("meurpg_session", h.signIn().Value)},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			_, err := h.getMe(tt.cookie)
			if !isUnauthenticated(err) {
				t.Fatalf("GetMe() error = %v, want unauthenticated", err)
			}
			if ce, ok := errors.AsType[*connect.Error](err); !ok || ce.Meta().Get("Cache-Control") != "no-store" {
				t.Errorf("unauthenticated response lacks Cache-Control: no-store")
			}
		})
	}
}

func TestGetMeWithHTTPGet(t *testing.T) {
	t.Parallel()
	h := newHarness(t)
	session := h.signIn()

	// GetMe is NO_SIDE_EFFECTS, so Connect clients may use GET. The request
	// is empty, so the URL carries no personal data.
	me, err := h.getMe(session, connect.WithHTTPGet())
	if err != nil {
		t.Fatalf("GetMe() over GET error = %v", err)
	}
	if got := me.Header().Get("Cache-Control"); got != "no-store" {
		t.Errorf("GetMe Cache-Control = %q, want no-store", got)
	}
}

func TestSessionHasAnAbsoluteThirtyDayLifetime(t *testing.T) {
	t.Parallel()
	for _, store := range testStores(t) {
		t.Run(store.name, func(t *testing.T) {
			t.Parallel()
			h := newHarness(t, withStore(store.new(t)))
			start := h.clock.Now()
			session := h.signIn()

			// Using the session does not extend it (no sliding expiry).
			for _, day := range []int{1, 10, 20, 29} { // never more than 14 idle days apart
				h.clock.Set(start.Add(time.Duration(day) * 24 * time.Hour))
				me, err := h.getMe(session)
				if err != nil {
					t.Fatalf("day %d: GetMe() error = %v", day, err)
				}
				if got := me.Msg.GetSessionExpiresAt().AsTime(); !got.Equal(start.Add(SessionLifetime)) {
					t.Errorf("day %d: session ends %v, want %v (never extended)", day, got, start.Add(SessionLifetime))
				}
			}

			h.clock.Set(start.Add(SessionLifetime - time.Second))
			if _, err := h.getMe(session); err != nil {
				t.Errorf("one second before 30 days: GetMe() error = %v, want the session to work", err)
			}
			h.clock.Advance(time.Second)
			if _, err := h.getMe(session); !isUnauthenticated(err) {
				t.Errorf("at 30 days: GetMe() error = %v, want unauthenticated", err)
			}
		})
	}
}

// TestSessionIdleTimeout: a session unused for 14 days is refused like a
// missing one, a used one is not, and the 30-day limit stays.
func TestSessionIdleTimeout(t *testing.T) {
	t.Parallel()
	for _, store := range testStores(t) {
		t.Run(store.name, func(t *testing.T) {
			t.Parallel()
			h := newHarness(t, withStore(store.new(t)))
			start := h.clock.Now()
			idle := h.signIn()
			used := h.signIn()

			// Both are used on day 10; only `used` is used again on day 20.
			h.clock.Set(start.Add(10 * 24 * time.Hour))
			for name, c := range map[string]*http.Cookie{"idle": idle, "used": used} {
				if _, err := h.getMe(c); err != nil {
					t.Fatalf("day 10, %s: GetMe() error = %v", name, err)
				}
			}
			h.clock.Set(start.Add(20 * 24 * time.Hour))
			if _, err := h.getMe(used); err != nil {
				t.Errorf("day 20, used: GetMe() error = %v, want it to work", err)
			}
			h.clock.Set(start.Add(24*24*time.Hour - time.Second))
			if _, err := h.getMe(used); err != nil {
				t.Errorf("day 24, used: GetMe() error = %v, want it to work", err)
			}
			// `idle` was last used on day 10: at day 24 it has been unused for
			// exactly 14 days. `used` was used a second ago.
			h.clock.Advance(time.Second)
			if _, err := h.getMe(idle); !isUnauthenticated(err) {
				t.Errorf("idle for 14 days: GetMe() error = %v, want unauthenticated", err)
			}
			if _, err := h.getMe(used); err != nil {
				t.Errorf("used: GetMe() error = %v, want it to work", err)
			}

			// The absolute limit still applies to a session in constant use.
			h.clock.Set(start.Add(SessionLifetime))
			if _, err := h.getMe(used); !isUnauthenticated(err) {
				t.Errorf("day 30, used: GetMe() error = %v, want unauthenticated (30 days absolute)", err)
			}
		})
	}
}

// TestSessionIdleTimeoutIsConfigurable: Config.SessionIdleTimeout replaces
// the 14 days.
func TestSessionIdleTimeoutIsConfigurable(t *testing.T) {
	t.Parallel()
	h := newHarness(t, func(_ *harness, cfg *Config) { cfg.SessionIdleTimeout = 48 * time.Hour })
	session := h.signIn()
	h.clock.Advance(47 * time.Hour)
	if _, err := h.getMe(session); err != nil {
		t.Fatalf("after 47 h: GetMe() error = %v, want it to work", err)
	}
	h.clock.Advance(48 * time.Hour)
	if _, err := h.getMe(session); !isUnauthenticated(err) {
		t.Errorf("after 48 idle hours: GetMe() error = %v, want unauthenticated", err)
	}
}

// TestSessionLastUseIsThrottled: the last use is written at most once per
// ten minutes (no write on every request).
func TestSessionLastUseIsThrottled(t *testing.T) {
	t.Parallel()
	h := newHarness(t)
	session := h.signIn()

	use := func() {
		t.Helper()
		if _, err := h.getMe(session); err != nil {
			t.Fatalf("GetMe() error = %v", err)
		}
	}
	for range 5 {
		h.clock.Advance(time.Minute)
		use()
	}
	if n := h.mem.touchCount(); n != 0 {
		t.Fatalf("writes after 5 uses in 5 minutes = %d, want 0", n)
	}
	h.clock.Advance(sessionTouchEvery)
	use()
	if n := h.mem.touchCount(); n != 1 {
		t.Fatalf("writes after the first use past 10 minutes = %d, want 1", n)
	}
	// The write moved the clock on the stored session: nine more minutes of
	// use write nothing.
	for range 9 {
		h.clock.Advance(time.Minute)
		use()
	}
	if n := h.mem.touchCount(); n != 1 {
		t.Errorf("writes 9 minutes after the last = %d, want still 1", n)
	}
}

// TestRecheckSessionIdle: an open stream's recheck sees the idle timeout, and
// counts as a use of the session.
func TestRecheckSessionIdle(t *testing.T) {
	t.Parallel()
	for _, store := range testStores(t) {
		t.Run(store.name, func(t *testing.T) {
			t.Parallel()
			h := newHarness(t, withStore(store.new(t)))
			start := h.clock.Now()
			cookie := h.signIn()
			ctx, err := h.svc.authenticate(t.Context(), http.Header{"Cookie": {cookie.String()}})
			if err != nil {
				t.Fatalf("authenticate() error = %v", err)
			}
			// A stream rechecking every minute for 20 days is a used session.
			for minute := time.Duration(1); minute <= 20*24*60; minute += 30 {
				h.clock.Set(start.Add(minute * time.Minute))
				if err := h.svc.RecheckSession(ctx); err != nil {
					t.Fatalf("day %d: RecheckSession() error = %v, want the stream's session to live", minute/(24*60), err)
				}
			}
			// No more rechecks: 14 idle days later the next one fails.
			h.clock.Advance(DefaultSessionIdleTimeout)
			if err := h.svc.RecheckSession(ctx); !isUnauthenticated(err) {
				t.Errorf("RecheckSession() after 14 idle days error = %v, want unauthenticated", err)
			}
		})
	}
}

// TestSignOutOtherSessions: it ends every other session of the caller and
// keeps the current one, and CountOtherSessions counts them.
func TestSignOutOtherSessions(t *testing.T) {
	t.Parallel()
	for _, store := range testStores(t) {
		t.Run(store.name, func(t *testing.T) {
			t.Parallel()
			h := newHarness(t, withStore(store.new(t)))
			current := h.signIn()
			phone := h.signIn()
			laptop := h.signIn()
			mutateClaims(func(c map[string]any) { c["sub"] = "another-person" })(h)
			stranger := h.signIn()

			count := func(c *http.Cookie) int32 {
				t.Helper()
				res, err := h.client(c).CountOtherSessions(t.Context(), connect.NewRequest(&identityv1.CountOtherSessionsRequest{}))
				if err != nil {
					t.Fatalf("CountOtherSessions() error = %v", err)
				}
				if got := res.Header().Get("Cache-Control"); got != "no-store" {
					t.Errorf("Cache-Control = %q, want no-store", got)
				}
				return res.Msg.GetOtherSessions()
			}
			if got := count(current); got != 2 {
				t.Errorf("CountOtherSessions() = %d, want 2 (the phone and the laptop)", got)
			}
			if got := count(stranger); got != 0 {
				t.Errorf("CountOtherSessions() of the other person = %d, want 0", got)
			}

			res, err := h.client(current).SignOutOtherSessions(t.Context(), connect.NewRequest(&identityv1.SignOutOtherSessionsRequest{}))
			if err != nil {
				t.Fatalf("SignOutOtherSessions() error = %v", err)
			}
			if got := res.Msg.GetEndedCount(); got != 2 {
				t.Errorf("ended_count = %d, want 2", got)
			}
			if got := res.Header().Get("Cache-Control"); got != "no-store" {
				t.Errorf("Cache-Control = %q, want no-store", got)
			}
			if len(res.Header().Values("Set-Cookie")) != 0 {
				t.Errorf("Set-Cookie = %q, want the cookie left alone", res.Header().Values("Set-Cookie"))
			}
			for name, c := range map[string]*http.Cookie{"phone": phone, "laptop": laptop} {
				if _, err := h.getMe(c); !isUnauthenticated(err) {
					t.Errorf("%s: GetMe() error = %v, want unauthenticated", name, err)
				}
			}
			for name, c := range map[string]*http.Cookie{"current": current, "another person": stranger} {
				if _, err := h.getMe(c); err != nil {
					t.Errorf("%s: GetMe() error = %v, want it still signed in", name, err)
				}
			}
			if got := count(current); got != 0 {
				t.Errorf("CountOtherSessions() after = %d, want 0", got)
			}
			// Nothing left to end is not an error.
			res, err = h.client(current).SignOutOtherSessions(t.Context(), connect.NewRequest(&identityv1.SignOutOtherSessionsRequest{}))
			if err != nil || res.Msg.GetEndedCount() != 0 {
				t.Errorf("second SignOutOtherSessions() = %v, %v; want 0 ended", res, err)
			}
			// Without a session: unauthenticated.
			if _, err := h.client(nil).SignOutOtherSessions(t.Context(), connect.NewRequest(&identityv1.SignOutOtherSessionsRequest{})); !isUnauthenticated(err) {
				t.Errorf("SignOutOtherSessions() signed out error = %v, want unauthenticated", err)
			}
			if _, err := h.client(nil).CountOtherSessions(t.Context(), connect.NewRequest(&identityv1.CountOtherSessionsRequest{})); !isUnauthenticated(err) {
				t.Errorf("CountOtherSessions() signed out error = %v, want unauthenticated", err)
			}
		})
	}
}

func TestSignOut(t *testing.T) {
	t.Parallel()
	for _, store := range testStores(t) {
		t.Run(store.name, func(t *testing.T) {
			t.Parallel()
			h := newHarness(t, withStore(store.new(t)))
			session := h.signIn()
			otherDevice := h.signIn()

			res, err := h.client(session).SignOut(t.Context(), connect.NewRequest(&identityv1.SignOutRequest{}))
			if err != nil {
				t.Fatalf("SignOut() error = %v", err)
			}
			cleared := cookieIn((&http.Response{Header: res.Header()}).Cookies(), SessionCookieName)
			if cleared == nil || cleared.Value != "" || cleared.MaxAge >= 0 {
				t.Errorf("SignOut Set-Cookie = %q, want the session cookie deleted", res.Header().Values("Set-Cookie"))
			} else {
				assertHostCookie(t, cleared)
			}
			if got := res.Header().Get("Cache-Control"); got != "no-store" {
				t.Errorf("SignOut Cache-Control = %q, want no-store", got)
			}

			// Revoked on the server: the same token no longer works...
			if _, err := h.getMe(session); !isUnauthenticated(err) {
				t.Errorf("GetMe() after SignOut error = %v, want unauthenticated", err)
			}
			// ...but the user's other device is still signed in.
			if _, err := h.getMe(otherDevice); err != nil {
				t.Errorf("GetMe() on the other device error = %v, want it still signed in", err)
			}

			// Signing out again: unauthenticated, and the stale cookie is
			// cleared all the same.
			_, err = h.client(session).SignOut(t.Context(), connect.NewRequest(&identityv1.SignOutRequest{}))
			if !isUnauthenticated(err) {
				t.Fatalf("second SignOut() error = %v, want unauthenticated", err)
			}
			if ce, ok := errors.AsType[*connect.Error](err); !ok || !strings.Contains(strings.Join(ce.Meta().Values("Set-Cookie"), ";"), SessionCookieName+"=;") {
				t.Errorf("second SignOut did not clear the cookie: %v", err)
			}
		})
	}
}

// TestRecheckSession: a long-lived stream reads its session again
// (authz.RecheckCampaignMember), and notices a sign-out or the 30 days
// passing.
func TestRecheckSession(t *testing.T) {
	t.Parallel()
	for _, store := range testStores(t) {
		t.Run(store.name, func(t *testing.T) {
			t.Parallel()
			h := newHarness(t, withStore(store.new(t)))
			start := h.clock.Now()
			cookie := h.signIn()
			header := http.Header{"Cookie": {cookie.String()}}
			ctx, err := h.svc.authenticate(t.Context(), header)
			if err != nil {
				t.Fatalf("authenticate() error = %v", err)
			}
			if err := h.svc.RecheckSession(ctx); err != nil {
				t.Fatalf("RecheckSession() error = %v, want the session to be valid", err)
			}

			h.clock.Set(start.Add(SessionLifetime))
			if err := h.svc.RecheckSession(ctx); !isUnauthenticated(err) {
				t.Errorf("RecheckSession() at 30 days error = %v, want unauthenticated", err)
			}
			h.clock.Set(start)
			if _, err := h.client(cookie).SignOut(t.Context(), connect.NewRequest(&identityv1.SignOutRequest{})); err != nil {
				t.Fatalf("SignOut() error = %v", err)
			}
			if err := h.svc.RecheckSession(ctx); !isUnauthenticated(err) {
				t.Errorf("RecheckSession() after SignOut error = %v, want unauthenticated", err)
			}
			if err := h.svc.RecheckSession(t.Context()); !isUnauthenticated(err) {
				t.Errorf("RecheckSession() without a session error = %v, want unauthenticated", err)
			}
		})
	}
}

func TestSignOutUnauthenticated(t *testing.T) {
	t.Parallel()
	h := newHarness(t)
	_, err := h.client(nil).SignOut(t.Context(), connect.NewRequest(&identityv1.SignOutRequest{}))
	if !isUnauthenticated(err) {
		t.Errorf("SignOut() without a session error = %v, want unauthenticated", err)
	}
}

func TestConnectRequiresTheProtocolHeader(t *testing.T) {
	t.Parallel()
	h := newHarness(t)

	// A plain JSON POST, like an HTML form or fetch() from another site
	// could send without a CORS preflight, is refused before the handler.
	req := httptest.NewRequestWithContext(t.Context(), http.MethodPost,
		identityv1connect.IdentityServiceSignOutProcedure, strings.NewReader("{}"))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	h.mux.ServeHTTP(rec, req)
	if rec.Code != http.StatusBadRequest || !strings.Contains(rec.Body.String(), "Connect-Protocol-Version") {
		t.Errorf("POST without Connect-Protocol-Version: status %d, body %s; want 400 asking for the header", rec.Code, rec.Body)
	}
}

func TestSessionLookupWhenTheStoreIsDown(t *testing.T) {
	t.Parallel()
	h := newHarness(t, withStore(failingStore{}))
	token, _ := secret.New()

	// Answering "unauthenticated" would make the app think the user signed
	// out; "unavailable" says to try again.
	_, err := h.getMe(reqCookie(SessionCookieName, token))
	if connect.CodeOf(err) != connect.CodeUnavailable {
		t.Errorf("GetMe() error = %v, want unavailable", err)
	}
}

func TestUserIDFailsClosed(t *testing.T) {
	t.Parallel()
	h := newHarness(t)
	// Without the interceptor there is never a session in the context.
	if _, err := requireSession(context.Background()); !isUnauthenticated(err) {
		t.Errorf("requireSession() error = %v, want unauthenticated", err)
	}
	if id, err := h.svc.UserID(context.Background()); !isUnauthenticated(err) || id != "" {
		t.Errorf("UserID() = %q, %v; want unauthenticated", id, err)
	}
}

// TestUserIDThroughTheInterceptor: UserID answers with the account behind
// the session cookie, for a handler mounted behind Interceptor, and with
// `unauthenticated` for a request without one.
func TestUserIDThroughTheInterceptor(t *testing.T) {
	t.Parallel()
	h := newHarness(t)
	session := h.signIn()
	me, err := h.getMe(session)
	if err != nil {
		t.Fatalf("GetMe() error = %v", err)
	}

	// A handler of another module, behind identity's interceptor.
	var gotID string
	var gotErr error
	handler := h.svc.Interceptor().WrapUnary(func(ctx context.Context, _ connect.AnyRequest) (connect.AnyResponse, error) {
		gotID, gotErr = h.svc.UserID(ctx)
		return nil, nil
	})
	call := func(cookie *http.Cookie) {
		t.Helper()
		req := connect.NewRequest(&identityv1.GetMeRequest{})
		if cookie != nil {
			req.Header().Set("Cookie", cookie.Name+"="+cookie.Value)
		}
		if _, err := handler(t.Context(), req); err != nil {
			t.Fatalf("interceptor error = %v", err)
		}
	}

	call(session)
	if gotErr != nil || gotID != me.Msg.GetUser().GetId() {
		t.Errorf("UserID() = %q, %v; want %q", gotID, gotErr, me.Msg.GetUser().GetId())
	}
	call(nil)
	if !isUnauthenticated(gotErr) || gotID != "" {
		t.Errorf("UserID() without a cookie = %q, %v; want unauthenticated", gotID, gotErr)
	}
}

// TestUserIDThroughAuthenticateRequest: the same, for a plain HTTP handler
// (the image routes of package maps). A session cookie that was never
// issued gives no session; a database that does not answer gives
// `unavailable`, not a signed-out caller.
func TestUserIDThroughAuthenticateRequest(t *testing.T) {
	t.Parallel()
	h := newHarness(t)
	session := h.signIn()
	me, err := h.getMe(session)
	if err != nil {
		t.Fatalf("GetMe() error = %v", err)
	}
	unknown, _ := secret.New()

	userID := func(svc *Service, cookie *http.Cookie) (string, error) {
		t.Helper()
		req := httptest.NewRequestWithContext(t.Context(), http.MethodGet, "/images/x", nil)
		if cookie != nil {
			req.AddCookie(cookie)
		}
		ctx, err := svc.AuthenticateRequest(req)
		if err != nil {
			return "", err
		}
		return svc.UserID(ctx)
	}

	if id, err := userID(h.svc, session); err != nil || id != me.Msg.GetUser().GetId() {
		t.Errorf("with the session cookie: UserID() = %q, %v; want %q", id, err, me.Msg.GetUser().GetId())
	}
	for name, cookie := range map[string]*http.Cookie{"no cookie": nil, "unknown token": reqCookie(SessionCookieName, unknown)} {
		if id, err := userID(h.svc, cookie); !isUnauthenticated(err) || id != "" {
			t.Errorf("%s: UserID() = %q, %v; want unauthenticated", name, id, err)
		}
	}

	down := newHarness(t, withStore(failingStore{}))
	if _, err := userID(down.svc, reqCookie(SessionCookieName, unknown)); connect.CodeOf(err) != connect.CodeUnavailable {
		t.Errorf("with the database down: error = %v, want unavailable", err)
	}
}

func TestMountDisabled(t *testing.T) {
	t.Parallel()
	mux := http.NewServeMux()
	MountDisabled(mux.Handle, reqlimit.HandlerOption())

	for _, route := range []struct{ method, path string }{
		{http.MethodGet, "/auth/login?return_to=/"},
		{http.MethodPost, "/auth/login"},
		{http.MethodGet, "/auth/callback?code=x&state=y"},
	} {
		rec := httptest.NewRecorder()
		mux.ServeHTTP(rec, httptest.NewRequestWithContext(t.Context(), route.method, route.path, nil))
		if rec.Code != http.StatusServiceUnavailable || !strings.Contains(rec.Body.String(), "not configured") {
			t.Errorf("%s %s: status %d, body %q; want 503 saying sign-in is not configured", route.method, route.path, rec.Code, rec.Body)
		}
	}

	server := httptest.NewServer(reqlimit.Middleware(mux))
	t.Cleanup(server.Close)
	client := identityv1connect.NewIdentityServiceClient(server.Client(), server.URL)
	if _, err := client.GetMe(t.Context(), connect.NewRequest(&identityv1.GetMeRequest{})); connect.CodeOf(err) != connect.CodeUnavailable {
		t.Errorf("GetMe() error = %v, want unavailable", err)
	}
	if _, err := client.SignOut(t.Context(), connect.NewRequest(&identityv1.SignOutRequest{})); connect.CodeOf(err) != connect.CodeUnavailable {
		t.Errorf("SignOut() error = %v, want unavailable", err)
	}
}

func TestNewValidatesItsConfig(t *testing.T) {
	t.Parallel()
	idp := newFakeIDP(t)
	valid := Config{Store: newMemStore(), Logger: discardLogger()}
	valid.OIDC.IssuerURL = idp.issuer()
	valid.OIDC.ClientID = idp.clientID
	valid.OIDC.RedirectURL = idp.redirectURL

	noOIDC := valid
	noOIDC.OIDC.IssuerURL = ""
	noStore := valid
	noStore.Store = nil
	missingCA := valid
	missingCA.OIDC.CAFile = "/does/not/exist.pem"
	notPEM := valid
	notPEM.OIDC.CAFile = writeTemp(t, "not a certificate")

	for name, cfg := range map[string]Config{
		"no OIDC":         noOIDC,
		"no store":        noStore,
		"missing CA file": missingCA,
		"CA file, no PEM": notPEM,
	} {
		if _, err := New(t.Context(), cfg); err == nil {
			t.Errorf("%s: New() error = nil, want an error", name)
		}
	}
}

func writeTemp(t *testing.T, content string) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "file")
	if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
		t.Fatalf("write %s: %v", path, err)
	}
	return path
}

func TestUpdateProfile(t *testing.T) {
	t.Parallel()
	for _, store := range testStores(t) {
		t.Run(store.name, func(t *testing.T) {
			t.Parallel()
			h := newHarness(t, withStore(store.new(t)))
			session := h.signIn()
			client := h.client(session)
			update := func(name string) (*connect.Response[identityv1.UpdateProfileResponse], error) {
				return client.UpdateProfile(t.Context(), connect.NewRequest(&identityv1.UpdateProfileRequest{DisplayName: name}))
			}
			displayName := func() string {
				t.Helper()
				me, err := h.getMe(session)
				if err != nil {
					t.Fatalf("GetMe() error = %v", err)
				}
				return me.Msg.GetUser().GetDisplayName()
			}

			// A new account has no display name: nothing comes from the
			// provider, even though the fake one sends a name.
			if got := displayName(); got != "" {
				t.Errorf("display name of a new account = %q, want none", got)
			}

			res, err := update("  Pensantus  ")
			if err != nil {
				t.Fatalf("UpdateProfile() error = %v", err)
			}
			if got := res.Msg.GetUser().GetDisplayName(); got != "Pensantus" {
				t.Errorf("UpdateProfile() display name = %q, want it trimmed", got)
			}
			if got := res.Header().Get("Cache-Control"); got != "no-store" {
				t.Errorf("UpdateProfile Cache-Control = %q, want no-store", got)
			}
			if got := displayName(); got != "Pensantus" {
				t.Errorf("GetMe() display name = %q, want Pensantus", got)
			}

			// Invalid names are refused, and the saved one stays.
			for _, bad := range []string{strings.Repeat("x", MaxDisplayNameLength+1), "Pensan\ntus", "Pensan\u202etus"} {
				if _, err := update(bad); connect.CodeOf(err) != connect.CodeInvalidArgument {
					t.Errorf("UpdateProfile(%q) error = %v, want invalid_argument", bad, err)
				}
			}
			if _, err := update(strings.Repeat("é", MaxDisplayNameLength)); err != nil {
				t.Errorf("UpdateProfile(40 characters) error = %v", err)
			}

			// An empty name removes it.
			if _, err := update("   "); err != nil {
				t.Fatalf("UpdateProfile(empty) error = %v", err)
			}
			if got := displayName(); got != "" {
				t.Errorf("display name after removing it = %q, want none", got)
			}

			if _, err := h.client(nil).UpdateProfile(t.Context(), connect.NewRequest(&identityv1.UpdateProfileRequest{DisplayName: "X"})); !isUnauthenticated(err) {
				t.Errorf("UpdateProfile() signed out error = %v, want unauthenticated", err)
			}
		})
	}
}
