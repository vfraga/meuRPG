package identity

import (
	"bytes"
	"context"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"sync"
	"testing"
	"time"

	"connectrpc.com/connect"

	identityv1 "github.com/PuraFome/meuRPG/backend/gen/meurpg/identity/v1"
	"github.com/PuraFome/meuRPG/backend/gen/meurpg/identity/v1/identityv1connect"
	"github.com/PuraFome/meuRPG/backend/internal/platform/config"
	"github.com/PuraFome/meuRPG/backend/internal/platform/reqlimit"
)

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

func (c *fakeClock) Set(t time.Time) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.now = t
}

// syncBuffer is a bytes.Buffer safe for the concurrent writes of a server.
type syncBuffer struct {
	mu  sync.Mutex
	buf bytes.Buffer
}

func (b *syncBuffer) Write(p []byte) (int, error) {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.buf.Write(p)
}

func (b *syncBuffer) String() string {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.buf.String()
}

// harness is a Service wired to a fake provider, a store, a fake clock and
// a log buffer, mounted on a mux the way cmd/api mounts it.
type harness struct {
	t     *testing.T
	idp   *fakeIDP
	store Store
	mem   *memStore // the store, when it is the in-memory one
	clock *fakeClock
	logs  *syncBuffer
	svc   *Service
	mux   *http.ServeMux
}

type harnessOption func(h *harness, cfg *Config)

// withStore replaces the default in-memory store.
func withStore(store Store) harnessOption {
	return func(h *harness, cfg *Config) {
		h.store = store
		h.mem, _ = store.(*memStore)
		cfg.Store = store
	}
}

// withIDP adjusts the fake provider before the Service runs discovery.
func withIDP(setup func(idp *fakeIDP)) harnessOption {
	return func(h *harness, _ *Config) { setup(h.idp) }
}

// withCloudRun makes the Service read the client IP from X-Forwarded-For
// (Cloud Run alone: one trusted hop).
func withCloudRun() harnessOption {
	return func(_ *harness, cfg *Config) { cfg.TrustedProxyHops = 1 }
}

// withIntents registers sign-in intent handlers.
func withIntents(intents map[string]IntentHandler) harnessOption {
	return func(_ *harness, cfg *Config) { cfg.Intents = intents }
}

// withMaxAge sets OIDC_MAX_AGE.
func withMaxAge(d time.Duration) harnessOption {
	return func(_ *harness, cfg *Config) { cfg.OIDC.MaxAge = d }
}

func newHarness(t *testing.T, opts ...harnessOption) *harness {
	t.Helper()

	mem := newMemStore()
	h := &harness{
		t:     t,
		idp:   newFakeIDP(t),
		store: mem,
		mem:   mem,
		// Real time, so the fake provider's tokens (stamped with time.Now)
		// are valid; tests then move it forward. Truncated to the
		// database's precision (microseconds), like the other modules'
		// harnesses, so a time read back from CockroachDB equals the one
		// the test computed. Without it the cockroachdb subtests fail on
		// Linux, whose clock has nanoseconds, and pass on macOS, whose
		// clock stops at microseconds.
		clock: &fakeClock{now: time.Now().Truncate(time.Microsecond)},
		logs:  &syncBuffer{},
	}
	cfg := Config{
		OIDC: config.OIDC{
			IssuerURL:    h.idp.issuer(),
			ClientID:     h.idp.clientID,
			ClientSecret: config.Secret(h.idp.clientSecret),
			RedirectURL:  h.idp.redirectURL,
			CAFile:       h.idp.caFile,
		},
		Store:  mem,
		Logger: slog.New(slog.NewJSONHandler(h.logs, &slog.HandlerOptions{Level: slog.LevelDebug})),
		Now:    h.clock.Now,
	}
	for _, opt := range opts {
		opt(h, &cfg)
	}

	svc, err := New(t.Context(), cfg)
	if err != nil {
		t.Fatalf("New() error = %v", err)
	}
	h.svc = svc
	h.mux = http.NewServeMux()
	svc.Mount(h.mux.Handle, reqlimit.HandlerOption())
	return h
}

// get sends a GET to the mux, like a browser following a link.
func (h *harness) get(target string, cookies ...*http.Cookie) *httptest.ResponseRecorder {
	h.t.Helper()
	req := httptest.NewRequestWithContext(h.t.Context(), http.MethodGet, "https://meurpg.test"+target, nil)
	for _, c := range cookies {
		req.AddCookie(reqCookie(c.Name, c.Value))
	}
	rec := httptest.NewRecorder()
	h.mux.ServeHTTP(rec, req)
	return rec
}

// postLogin sends POST /auth/login with form as the body, like the app's
// sign-in form. Extra headers (e.g. Sec-Fetch-Site) are set as given.
func (h *harness) postLogin(handler http.Handler, form url.Values, headers map[string]string) *httptest.ResponseRecorder {
	h.t.Helper()
	req := httptest.NewRequestWithContext(h.t.Context(), http.MethodPost, "https://meurpg.test/auth/login", strings.NewReader(form.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	for k, v := range headers {
		req.Header.Set(k, v)
	}
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)
	return rec
}

// beginLogin visits /auth/login, then lets the fake provider sign the user
// in. It returns the callback URL the provider redirected to (with code
// and state) and the login cookie the browser now holds.
func (h *harness) beginLogin(returnTo string) (callbackURL string, loginCookie *http.Cookie) {
	h.t.Helper()
	rec := h.get("/auth/login?return_to=" + url.QueryEscape(returnTo))
	if rec.Code != http.StatusFound {
		h.t.Fatalf("GET /auth/login status = %d, want 302; body: %s", rec.Code, rec.Body)
	}
	loginCookie = findCookie(h.t, rec, loginCookieName)
	return h.authorize(rec.Header().Get("Location")), loginCookie
}

// authorize follows the redirect to the fake provider and returns where it
// sends the browser back to.
func (h *harness) authorize(authURL string) string {
	h.t.Helper()
	if !strings.HasPrefix(authURL, h.idp.issuer()+"/authorize?") {
		h.t.Fatalf("login redirected to %q, want the provider's authorization endpoint", authURL)
	}
	client := h.idp.server.Client()
	client.CheckRedirect = func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }
	req, err := http.NewRequestWithContext(h.t.Context(), http.MethodGet, authURL, nil)
	if err != nil {
		h.t.Fatalf("new request: %v", err)
	}
	resp, err := client.Do(req)
	if err != nil {
		h.t.Fatalf("GET authorize: %v", err)
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode != http.StatusFound {
		h.t.Fatalf("authorize status = %d, want 302", resp.StatusCode)
	}
	return resp.Header.Get("Location")
}

// finishLogin visits the callback URL with the given cookies.
func (h *harness) finishLogin(callbackURL string, cookies ...*http.Cookie) *httptest.ResponseRecorder {
	h.t.Helper()
	u, err := url.Parse(callbackURL)
	if err != nil {
		h.t.Fatalf("parse callback URL: %v", err)
	}
	return h.get(u.RequestURI(), cookies...)
}

// signIn runs the whole flow and returns the session cookie.
func (h *harness) signIn(cookies ...*http.Cookie) *http.Cookie {
	h.t.Helper()
	callbackURL, loginCookie := h.beginLogin("/")
	rec := h.finishLogin(callbackURL, append([]*http.Cookie{loginCookie}, cookies...)...)
	if rec.Code != http.StatusSeeOther {
		h.t.Fatalf("callback status = %d, want 303; body: %s; logs: %s", rec.Code, rec.Body, h.logs)
	}
	return findCookie(h.t, rec, SessionCookieName)
}

// client returns an IdentityService client that talks to the mux over real
// HTTP, sending the given session cookie (nil for none).
func (h *harness) client(session *http.Cookie, opts ...connect.ClientOption) identityv1connect.IdentityServiceClient {
	h.t.Helper()
	server := httptest.NewServer(reqlimit.Middleware(h.mux))
	h.t.Cleanup(server.Close)
	if session != nil {
		cookie := session.Name + "=" + session.Value
		opts = append(opts, connect.WithInterceptors(connect.UnaryInterceptorFunc(
			func(next connect.UnaryFunc) connect.UnaryFunc {
				return func(ctx context.Context, req connect.AnyRequest) (connect.AnyResponse, error) {
					req.Header().Set("Cookie", cookie)
					return next(ctx, req)
				}
			})))
	}
	return identityv1connect.NewIdentityServiceClient(server.Client(), server.URL, opts...)
}

// getMe calls GetMe with a session cookie and returns the response or error.
func (h *harness) getMe(session *http.Cookie, opts ...connect.ClientOption) (*connect.Response[identityv1.GetMeResponse], error) {
	h.t.Helper()
	return h.client(session, opts...).GetMe(h.t.Context(), connect.NewRequest(&identityv1.GetMeRequest{}))
}

// findCookie returns the named cookie a response sets, or fails the test.
func findCookie(t *testing.T, rec *httptest.ResponseRecorder, name string) *http.Cookie {
	t.Helper()
	if c := cookieIn(rec.Result().Cookies(), name); c != nil {
		return c
	}
	t.Fatalf("response sets no %s cookie; Set-Cookie: %q", name, rec.Header().Values("Set-Cookie"))
	return nil
}

func cookieIn(cookies []*http.Cookie, name string) *http.Cookie {
	for _, c := range cookies {
		if c.Name == name {
			return c
		}
	}
	return nil
}

// reqCookie is a cookie as a browser sends it back. Only name and value
// travel in a request; the attributes are set to what the server sets, so
// the literal reads like the real cookie.
func reqCookie(name, value string) *http.Cookie {
	return &http.Cookie{Name: name, Value: value, Path: "/", Secure: true, HttpOnly: true, SameSite: http.SameSiteLaxMode}
}
