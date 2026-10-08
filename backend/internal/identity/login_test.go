package identity

import (
	"context"
	"crypto/rand"
	"crypto/rsa"
	"crypto/sha256"
	"encoding/base64"
	"errors"
	"net/http"
	"net/url"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"connectrpc.com/connect"

	identityv1 "github.com/PuraFome/meuRPG/backend/gen/meurpg/identity/v1"
	"github.com/PuraFome/meuRPG/backend/internal/identity/oidctest"
	"github.com/PuraFome/meuRPG/backend/internal/platform/config"
	"github.com/PuraFome/meuRPG/backend/internal/platform/secret"
)

func TestSignInHappyPath(t *testing.T) {
	t.Parallel()
	for _, store := range testStores(t) {
		t.Run(store.name, func(t *testing.T) {
			t.Parallel()
			h := newHarness(t, withStore(store.new(t)))
			start := h.clock.Now()

			// 1. /auth/login redirects to the provider with every parameter
			// of Authorization Code + PKCE (S256) + nonce.
			rec := h.get("/auth/login?return_to=" + url.QueryEscape("/campaigns/42?aba=mapa"))
			if rec.Code != http.StatusFound {
				t.Fatalf("GET /auth/login status = %d, want 302", rec.Code)
			}
			assertAuthHeaders(t, rec.Header())
			authURL, err := url.Parse(rec.Header().Get("Location"))
			if err != nil {
				t.Fatalf("parse Location: %v", err)
			}
			q := authURL.Query()
			want := map[string]string{
				"response_type":         "code",
				"client_id":             h.idp.clientID,
				"redirect_uri":          h.idp.redirectURL,
				"scope":                 "openid email",
				"code_challenge_method": "S256",
			}
			for k, v := range want {
				if got := q.Get(k); got != v {
					t.Errorf("authorize %s = %q, want %q", k, got, v)
				}
			}
			for _, k := range []string{"state", "nonce", "code_challenge"} {
				if len(q.Get(k)) != 43 { // 32 bytes, base64url without padding
					t.Errorf("authorize %s = %q, want 43 base64url characters", k, q.Get(k))
				}
			}
			if q.Has("max_age") {
				t.Errorf("authorize has max_age = %q without OIDC_MAX_AGE", q.Get("max_age"))
			}

			// The login cookie holds the state and is locked to this host.
			loginCookie := findCookie(t, rec, loginCookieName)
			assertHostCookie(t, loginCookie)
			if loginCookie.Value != q.Get("state") {
				t.Error("login cookie value is not the state sent to the provider")
			}
			if loginCookie.MaxAge != 600 {
				t.Errorf("login cookie Max-Age = %d, want 600", loginCookie.MaxAge)
			}

			// 2. The provider signs the user in and sends the browser back.
			callbackURL := h.authorize(authURL.String())

			// 3. The callback creates a session and returns to return_to.
			rec = h.finishLogin(callbackURL, loginCookie)
			if rec.Code != http.StatusSeeOther {
				t.Fatalf("callback status = %d, want 303; body: %s; logs: %s", rec.Code, rec.Body, h.logs)
			}
			assertAuthHeaders(t, rec.Header())
			if got := rec.Header().Get("Location"); got != "/campaigns/42?aba=mapa" {
				t.Errorf("callback redirects to %q, want the return_to path", got)
			}
			if c := findCookie(t, rec, loginCookieName); c.MaxAge >= 0 || c.Value != "" {
				t.Errorf("login cookie not cleared: %+v", c)
			}

			session := findCookie(t, rec, SessionCookieName)
			assertHostCookie(t, session)
			if session.MaxAge != int(SessionLifetime.Seconds()) {
				t.Errorf("session cookie Max-Age = %d, want %d (30 days)", session.MaxAge, int(SessionLifetime.Seconds()))
			}
			if !session.Expires.Equal(start.Add(SessionLifetime).Truncate(time.Second)) {
				t.Errorf("session cookie Expires = %v, want %v", session.Expires, start.Add(SessionLifetime))
			}
			if raw, err := base64.RawURLEncoding.DecodeString(session.Value); err != nil || len(raw) != 32 {
				t.Errorf("session token is not 32 random bytes in base64url: %q", session.Value)
			}

			// The session works, and says when it ends.
			me, err := h.getMe(session)
			if err != nil {
				t.Fatalf("GetMe() error = %v", err)
			}
			if me.Msg.GetUser().GetId() == "" {
				t.Error("GetMe() returned no user ID")
			}
			if got := me.Msg.GetSessionExpiresAt().AsTime(); !got.Equal(start.Add(SessionLifetime)) {
				t.Errorf("session_expires_at = %v, want %v", got, start.Add(SessionLifetime))
			}

			// Signing in again as the same person reaches the same account.
			again, err := h.getMe(h.signIn())
			if err != nil {
				t.Fatalf("GetMe() after the second sign-in: %v", err)
			}
			if again.Msg.GetUser().GetId() != me.Msg.GetUser().GetId() {
				t.Errorf("second sign-in user = %s, want the same account %s", again.Msg.GetUser().GetId(), me.Msg.GetUser().GetId())
			}

			// The callback URL works once.
			if rec := h.finishLogin(callbackURL, loginCookie); rec.Code != http.StatusBadRequest {
				t.Errorf("replayed callback status = %d, want 400", rec.Code)
			}
		})
	}
}

func TestSignInStoresOnlyIssuerSubjectAndVerifiedEmail(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name          string
		emailVerified any
		wantEmail     string
	}{
		{"verified e-mail is kept as a security contact", true, "mestre@example.com"},
		{"verified as the string \"true\"", "true", "mestre@example.com"},
		{"unverified e-mail is dropped", false, ""},
		{"no email_verified claim", nil, ""},
		{"odd email_verified value", "yes", ""},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			h := newHarness(t, withIDP(func(idp *fakeIDP) {
				idp.editUser(func(u *oidctest.User) { u.EmailVerified = tt.emailVerified })
			}))
			h.signIn()

			id, ok := h.mem.identity(h.idp.issuer(), h.idp.user().Subject)
			if !ok {
				t.Fatal("no identity stored for (issuer, subject)")
			}
			if id.email != tt.wantEmail {
				t.Errorf("stored email = %q, want %q", id.email, tt.wantEmail)
			}
		})
	}

	t.Run("a later unverified sign-in clears the stored e-mail", func(t *testing.T) {
		t.Parallel()
		h := newHarness(t)
		h.signIn()
		h.idp.editUser(func(u *oidctest.User) { u.EmailVerified = false })
		h.signIn()
		if id, _ := h.mem.identity(h.idp.issuer(), h.idp.user().Subject); id.email != "" {
			t.Errorf("stored email = %q, want it cleared", id.email)
		}
	})
}

// TestCallbackRejects breaks one thing at a time. Every case must end with
// a 400, no session cookie and no session in the store.
func TestCallbackRejects(t *testing.T) {
	t.Parallel()

	otherKey, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		t.Fatalf("generate RSA key: %v", err)
	}

	tests := []struct {
		name       string
		setup      func(h *harness)
		tamper     func(h *harness, callbackURL string, loginCookie *http.Cookie) (string, []*http.Cookie)
		wantReason string
		wantError  string // in the logged error, when the reason is shared
	}{
		{
			name: "no login cookie",
			tamper: func(_ *harness, u string, _ *http.Cookie) (string, []*http.Cookie) {
				return u, nil
			},
			wantReason: "missing_state",
		},
		{
			name: "no state in the URL",
			tamper: func(_ *harness, u string, c *http.Cookie) (string, []*http.Cookie) {
				return withQuery(u, "state", ""), []*http.Cookie{c}
			},
			wantReason: "missing_state",
		},
		{
			name: "state does not match the cookie",
			tamper: func(_ *harness, u string, c *http.Cookie) (string, []*http.Cookie) {
				other, _ := secret.New()
				return withQuery(u, "state", other), []*http.Cookie{c}
			},
			wantReason: "state_mismatch",
		},
		{
			// Login CSRF: the attacker signs in with their own account and
			// sends the victim their callback URL. The victim's browser has
			// its own login cookie (or none), never the attacker's.
			name: "callback URL from another browser's sign-in",
			tamper: func(h *harness, attackerURL string, _ *http.Cookie) (string, []*http.Cookie) {
				_, victimCookie := h.beginLogin("/")
				return attackerURL, []*http.Cookie{victimCookie}
			},
			wantReason: "state_mismatch",
		},
		{
			name: "state that was never issued",
			tamper: func(_ *harness, u string, _ *http.Cookie) (string, []*http.Cookie) {
				forged, _ := secret.New()
				return withQuery(u, "state", forged), []*http.Cookie{reqCookie(loginCookieName, forged)}
			},
			wantReason: "unknown_or_expired_state",
		},
		{
			name: "login state expired (more than 10 minutes)",
			tamper: func(h *harness, u string, c *http.Cookie) (string, []*http.Cookie) {
				h.clock.Advance(loginStateLifetime)
				return u, []*http.Cookie{c}
			},
			wantReason: "unknown_or_expired_state",
		},
		{
			name: "provider reports an error",
			tamper: func(_ *harness, u string, c *http.Cookie) (string, []*http.Cookie) {
				return withQuery(withQuery(u, "code", ""), "error", "access_denied"), []*http.Cookie{c}
			},
			wantReason: "provider_error",
			wantError:  "access_denied",
		},
		{
			name: "no code",
			tamper: func(_ *harness, u string, c *http.Cookie) (string, []*http.Cookie) {
				return withQuery(u, "code", ""), []*http.Cookie{c}
			},
			wantReason: "missing_code",
		},
		{
			name: "code the provider did not issue",
			tamper: func(_ *harness, u string, c *http.Cookie) (string, []*http.Cookie) {
				return withQuery(u, "code", "forged-code"), []*http.Cookie{c}
			},
			wantReason: "token_exchange_rejected",
			wantError:  "invalid_grant",
		},
		{
			// The provider checks BASE64URL(SHA256(verifier)) against the
			// challenge; a different verifier must fail the exchange.
			name: "PKCE verifier does not match the challenge",
			tamper: func(h *harness, u string, c *http.Cookie) (string, []*http.Cookie) {
				h.mem.tamperLoginStates(func(st *LoginState) { st.CodeVerifier = strings.Repeat("x", 43) })
				return u, []*http.Cookie{c}
			},
			wantReason: "token_exchange_rejected",
			wantError:  "invalid_grant",
		},
		{
			name:       "wrong nonce",
			setup:      mutateClaims(func(c map[string]any) { c["nonce"] = "a-nonce-from-another-sign-in" }),
			wantReason: "invalid_id_token",
			wantError:  "nonce does not match",
		},
		{
			name:       "no nonce",
			setup:      mutateClaims(func(c map[string]any) { delete(c, "nonce") }),
			wantReason: "invalid_id_token",
			wantError:  "nonce does not match",
		},
		{
			name:       "wrong audience",
			setup:      mutateClaims(func(c map[string]any) { c["aud"] = "another-client" }),
			wantReason: "invalid_id_token",
			wantError:  "expected audience",
		},
		{
			name:       "an extra audience we do not trust",
			setup:      mutateClaims(func(c map[string]any) { c["aud"] = []string{c["aud"].(string), "another-client"} }),
			wantReason: "invalid_id_token",
			wantError:  "audience other than this client",
		},
		{
			name:       "azp is another client",
			setup:      mutateClaims(func(c map[string]any) { c["azp"] = "another-client" }),
			wantReason: "invalid_id_token",
			wantError:  "azp is not this client",
		},
		{
			name:       "expired ID token",
			setup:      mutateClaims(func(c map[string]any) { c["exp"] = time.Now().Add(-time.Minute).Unix() }),
			wantReason: "invalid_id_token",
			wantError:  "token is expired",
		},
		{
			name:       "wrong issuer",
			setup:      mutateClaims(func(c map[string]any) { c["iss"] = "https://evil.example" }),
			wantReason: "invalid_id_token",
			wantError:  "issued by a different provider",
		},
		{
			name:       "no subject",
			setup:      mutateClaims(func(c map[string]any) { delete(c, "sub") }),
			wantReason: "invalid_id_token",
			wantError:  "sub is missing",
		},
		{
			name:       "subject longer than 255 characters",
			setup:      mutateClaims(func(c map[string]any) { c["sub"] = strings.Repeat("9", 256) }),
			wantReason: "invalid_id_token",
			wantError:  "sub is missing or longer than 255",
		},
		{
			name: "unsigned ID token (alg none)",
			setup: func(h *harness) {
				h.idp.Tweak(func(k *oidctest.Knobs) { k.Unsigned = true })
			},
			wantReason: "invalid_id_token",
			wantError:  "unexpected signature algorithm",
		},
		{
			name: "ID token signed with a key the provider does not publish",
			setup: func(h *harness) {
				h.idp.Tweak(func(k *oidctest.Knobs) { k.SigningKey = otherKey })
			},
			wantReason: "invalid_id_token",
			wantError:  "failed to verify signature",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			h := newHarness(t)
			if tt.setup != nil {
				tt.setup(h)
			}
			callbackURL, loginCookie := h.beginLogin("/")
			cookies := []*http.Cookie{loginCookie}
			if tt.tamper != nil {
				callbackURL, cookies = tt.tamper(h, callbackURL, loginCookie)
			}

			rec := h.finishLogin(callbackURL, cookies...)
			if rec.Code != http.StatusBadRequest {
				t.Errorf("callback status = %d, want 400; body: %s", rec.Code, rec.Body)
			}
			if c := cookieIn(rec.Result().Cookies(), SessionCookieName); c != nil {
				t.Errorf("a failed callback set a session cookie: %+v", c)
			}
			assertAuthHeaders(t, rec.Header())
			if n := h.mem.sessionCount(); n != 0 {
				t.Errorf("sessions in the store = %d, want 0", n)
			}
			logs := h.logs.String()
			if !strings.Contains(logs, `"reason":"`+tt.wantReason+`"`) || !strings.Contains(logs, tt.wantError) {
				t.Errorf("logs do not mention reason %q and error %q: %s", tt.wantReason, tt.wantError, logs)
			}
			if body := rec.Body.String(); strings.Contains(body, "oidc") || strings.Contains(body, "nonce") {
				t.Errorf("the error page reveals internals: %q", body)
			}
		})
	}
}

func mutateClaims(edit func(claims map[string]any)) func(h *harness) {
	return func(h *harness) {
		h.idp.Tweak(func(k *oidctest.Knobs) { k.MutateClaims = edit })
	}
}

// withQuery returns rawURL with one query parameter replaced; an empty
// value removes it.
func withQuery(rawURL, key, value string) string {
	u, err := url.Parse(rawURL)
	if err != nil {
		panic(err)
	}
	q := u.Query()
	if value == "" {
		q.Del(key)
	} else {
		q.Set(key, value)
	}
	u.RawQuery = q.Encode()
	return u.String()
}

func TestLoginRejectsOpenRedirects(t *testing.T) {
	t.Parallel()

	unsafe := []string{
		"https://evil.example/",
		"http://evil.example",
		"//evil.example",
		"///evil.example",
		`/\evil.example`,
		`\\evil.example`,
		"/\t/evil.example",
		"/\n/evil.example",
		"javascript:alert(1)",
		"evil.example",
		"campaigns",
		"http:/evil.example",
		" /campaigns",
		"/" + strings.Repeat("a", maxReturnToLength),
		"#t=segredo",
		"#/campaigns",
	}
	for _, returnTo := range unsafe {
		name := returnTo
		if len(name) > 40 {
			name = name[:20] + "... (too long)"
		}
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			h := newHarness(t)
			rec := h.get("/auth/login?return_to=" + url.QueryEscape(returnTo))
			if rec.Code != http.StatusBadRequest {
				t.Errorf("status = %d, want 400", rec.Code)
			}
			if loc := rec.Header().Get("Location"); loc != "" {
				t.Errorf("redirected to %q", loc)
			}
			if n := len(h.mem.loginStates); n != 0 {
				t.Errorf("saved %d login states, want 0", n)
			}
		})
	}

	safe := map[string]string{
		"":                       "/",
		"/":                      "/",
		"/campaigns":             "/campaigns",
		"/campaigns/42?aba=mapa": "/campaigns/42?aba=mapa",
		"/%2F%2Fevil.example":    "/%2F%2Fevil.example", // a path on this site
		// The fragment is dropped: the app keeps secrets there, and the
		// login state must not store one.
		"/invite#t=segredo":  "/invite",
		"/campaigns?aba=1#x": "/campaigns?aba=1",
	}
	for in, want := range safe {
		if got, ok := safeReturnTo(in); !ok || got != want {
			t.Errorf("safeReturnTo(%q) = %q, %v; want %q, true", in, got, ok, want)
		}
	}
}

func TestMaxAgeAndAuthTime(t *testing.T) {
	t.Parallel()

	t.Run("max_age is sent when configured", func(t *testing.T) {
		t.Parallel()
		h := newHarness(t, withMaxAge(time.Hour))
		h.beginLogin("/")
		if got := h.idp.LastAuthorize().Get("max_age"); got != "3600" {
			t.Errorf("authorize max_age = %q, want 3600", got)
		}
	})

	// auth_time is recorded, never trusted: a provider may report the time
	// of the first login even after forcing a new one. The session lasts
	// 30 days from now either way.
	t.Run("an old auth_time is recorded and does not shorten the session", func(t *testing.T) {
		t.Parallel()
		authTime := time.Now().Add(-40 * 24 * time.Hour).Truncate(time.Second)
		h := newHarness(t, withMaxAge(time.Hour))
		mutateClaims(func(c map[string]any) { c["auth_time"] = authTime.Unix() })(h)
		session := h.signIn()

		me, err := h.getMe(session)
		if err != nil {
			t.Fatalf("GetMe() error = %v", err)
		}
		if got, want := me.Msg.GetSessionExpiresAt().AsTime(), h.clock.Now().Add(SessionLifetime); !got.Equal(want) {
			t.Errorf("session ends %v, want %v", got, want)
		}
		if got := h.mem.sessionAuthTime(sessionID(t, h, session)); !got.Equal(authTime) {
			t.Errorf("recorded auth_time = %v, want %v", got, authTime)
		}
	})

	t.Run("the provider's auth_time is recorded", func(t *testing.T) {
		t.Parallel()
		h := newHarness(t, withMaxAge(time.Hour))
		before := time.Now().Truncate(time.Second)
		session := h.signIn()
		got := h.mem.sessionAuthTime(sessionID(t, h, session))
		if got.Before(before) || got.After(time.Now()) {
			t.Errorf("recorded auth_time = %v, want the time of this sign-in", got)
		}
	})

	t.Run("no auth_time is fine", func(t *testing.T) {
		t.Parallel()
		h := newHarness(t, withMaxAge(time.Hour))
		mutateClaims(func(c map[string]any) { delete(c, "auth_time") })(h)
		session := h.signIn()
		if got := h.mem.sessionAuthTime(sessionID(t, h, session)); !got.IsZero() {
			t.Errorf("recorded auth_time = %v, want none", got)
		}
	})

	t.Run("a malformed auth_time is ignored", func(t *testing.T) {
		t.Parallel()
		h := newHarness(t)
		mutateClaims(func(c map[string]any) { c["auth_time"] = "yesterday" })(h)
		session := h.signIn()
		if got := h.mem.sessionAuthTime(sessionID(t, h, session)); !got.IsZero() {
			t.Errorf("recorded auth_time = %v, want none", got)
		}
	})
}

// sessionID finds the stored session behind a session cookie.
func sessionID(t *testing.T, h *harness, cookie *http.Cookie) string {
	t.Helper()
	s, err := h.svc.lookupSession(t.Context(), cookie.Value)
	if err != nil {
		t.Fatalf("lookupSession() error = %v", err)
	}
	return s.ID
}

func TestSigningInAgainRevokesThePreviousSession(t *testing.T) {
	t.Parallel()
	h := newHarness(t)

	first := h.signIn()
	second := h.signIn(first) // the browser still sends the first cookie

	if first.Value == second.Value {
		t.Fatal("the second sign-in reused the session token (session fixation)")
	}
	if _, err := h.getMe(first); !isUnauthenticated(err) {
		t.Errorf("GetMe() with the replaced session = %v, want unauthenticated", err)
	}
	if _, err := h.getMe(second); err != nil {
		t.Errorf("GetMe() with the new session = %v", err)
	}
	if n := h.mem.sessionCount(); n != 1 {
		t.Errorf("sessions in the store = %d, want 1", n)
	}
}

func TestProviderDiscovery(t *testing.T) {
	t.Parallel()

	t.Run("an unreachable provider is retried on the next sign-in", func(t *testing.T) {
		t.Parallel()
		h := newHarness(t, withIDP(func(idp *fakeIDP) {
			// Discovery at New fails; the next attempt succeeds.
			idp.SetMetadata("code_challenge_methods_supported", []string{"plain"})
		}))
		if !strings.Contains(h.logs.String(), "OIDC discovery failed") {
			t.Errorf("New() did not log the failed discovery: %s", h.logs)
		}
		if rec := h.get("/auth/login"); rec.Code != http.StatusServiceUnavailable {
			t.Errorf("GET /auth/login while discovery fails: status = %d, want 503", rec.Code)
		}
		h.idp.SetMetadata("code_challenge_methods_supported", []string{"S256"})
		h.signIn()
	})

	t.Run("client_secret_post when the provider only supports that", func(t *testing.T) {
		t.Parallel()
		h := newHarness(t, withIDP(func(idp *fakeIDP) {
			idp.SetMetadata("token_endpoint_auth_methods_supported", []string{"client_secret_post"})
		}))
		h.signIn()
	})

	t.Run("no PKCE S256 means no sign-in", func(t *testing.T) {
		t.Parallel()
		h := newHarness(t, withIDP(func(idp *fakeIDP) {
			idp.SetMetadata("code_challenge_methods_supported", []string{"plain"})
		}))
		if rec := h.get("/auth/login"); rec.Code != http.StatusServiceUnavailable {
			t.Errorf("status = %d, want 503", rec.Code)
		}
	})

	t.Run("no supported client authentication means no sign-in", func(t *testing.T) {
		t.Parallel()
		h := newHarness(t, withIDP(func(idp *fakeIDP) {
			idp.SetMetadata("token_endpoint_auth_methods_supported", []string{"private_key_jwt"})
		}))
		if rec := h.get("/auth/login"); rec.Code != http.StatusServiceUnavailable {
			t.Errorf("status = %d, want 503", rec.Code)
		}
	})

	t.Run("a provider whose TLS certificate is not trusted", func(t *testing.T) {
		t.Parallel()
		idp := newFakeIDP(t)
		svc, err := New(t.Context(), Config{
			// No CAFile: the test server's certificate is not trusted.
			OIDC:   config.OIDC{IssuerURL: idp.issuer(), ClientID: idp.clientID, ClientSecret: "x", RedirectURL: idp.redirectURL},
			Store:  newMemStore(),
			Logger: discardLogger(),
		})
		if err != nil {
			t.Fatalf("New() error = %v, want the failure deferred to sign-in", err)
		}
		if _, err := svc.providers.get(t.Context()); err == nil || !strings.Contains(err.Error(), "certificate") {
			t.Errorf("discovery error = %v, want a certificate error", err)
		}
	})
}

func TestLoginWhenTheStoreIsDown(t *testing.T) {
	t.Parallel()
	h := newHarness(t, withStore(failingStore{}))
	rec := h.get("/auth/login")
	if rec.Code != http.StatusServiceUnavailable {
		t.Errorf("status = %d, want 503", rec.Code)
	}
	if cookieIn(rec.Result().Cookies(), loginCookieName) != nil {
		t.Error("set a login cookie although the login state was not saved")
	}
}

// TestLogsHaveNoSecretsOrPersonalData runs a sign-in, a failed one and a
// sign-out, then searches the logs for every secret and personal value.
func TestLogsHaveNoSecretsOrPersonalData(t *testing.T) {
	t.Parallel()
	h := newHarness(t)

	callbackURL, loginCookie := h.beginLogin("/")
	rec := h.finishLogin(callbackURL, loginCookie)
	session := findCookie(t, rec, SessionCookieName)
	me, err := h.getMe(session)
	if err != nil {
		t.Fatalf("GetMe() error = %v", err)
	}
	// A failed sign-in logs an error; it must not leak either.
	failedURL, _ := h.beginLogin("/")
	h.finishLogin(failedURL, reqCookie(loginCookieName, loginCookie.Value))
	if _, err := h.client(session).SignOut(t.Context(), connect.NewRequest(&identityv1.SignOutRequest{})); err != nil {
		t.Fatalf("SignOut() error = %v", err)
	}

	u, _ := url.Parse(callbackURL)
	secrets := map[string]string{
		"session token": session.Value,
		"state":         loginCookie.Value,
		"code":          u.Query().Get("code"),
		"client secret": h.idp.clientSecret,
		"e-mail":        h.idp.user().Email,
		"subject":       h.idp.user().Subject,
		"name claim":    "Nome Que Nunca Guardamos",
		"access token":  "fake-access-token",
		"token hash":    base64.RawURLEncoding.EncodeToString(sha256Of(session.Value)),
		"account ID":    me.Msg.GetUser().GetId(),
		"client IP":     "192.0.2.1", // httptest.NewRequest's RemoteAddr
	}
	logs := h.logs.String()
	for what, value := range secrets {
		if value != "" && strings.Contains(logs, value) {
			t.Errorf("logs contain the %s: %s", what, logs)
		}
	}
	if !strings.Contains(logs, "sign-in succeeded") {
		t.Errorf("logs do not record the sign-in: %s", logs)
	}
}

func sha256Of(token string) []byte {
	raw, _ := base64.RawURLEncoding.DecodeString(token)
	sum := sha256.Sum256(raw)
	return sum[:]
}

// assertHostCookie checks the attributes a __Host- cookie needs, plus ours.
func assertHostCookie(t *testing.T, c *http.Cookie) {
	t.Helper()
	if !strings.HasPrefix(c.Name, "__Host-") {
		t.Errorf("cookie %s lacks the __Host- prefix", c.Name)
	}
	if !c.Secure || !c.HttpOnly || c.SameSite != http.SameSiteLaxMode || c.Path != "/" || c.Domain != "" {
		t.Errorf("cookie %s: Secure=%v HttpOnly=%v SameSite=%v Path=%q Domain=%q; want Secure, HttpOnly, Lax, /, no Domain",
			c.Name, c.Secure, c.HttpOnly, c.SameSite, c.Path, c.Domain)
	}
}

func assertAuthHeaders(t *testing.T, h http.Header) {
	t.Helper()
	if got := h.Get("Cache-Control"); got != "no-store" {
		t.Errorf("Cache-Control = %q, want no-store", got)
	}
	if got := h.Get("Referrer-Policy"); got != "no-referrer" {
		t.Errorf("Referrer-Policy = %q, want no-referrer", got)
	}
}

// failingCreateStore fails createSession while failCreate is set. fails createSession while failCreate is set.
type failingCreateStore struct {
	*memStore
	failCreate atomic.Bool
}

func (s *failingCreateStore) createSession(ctx context.Context, ns NewSession) (Session, error) {
	if s.failCreate.Load() {
		return Session{}, errors.New("simulated DB blip")
	}
	return s.memStore.createSession(ctx, ns)
}

// A sign-in that cannot create its session leaves the one the browser had:
// the old one is revoked only after the new one exists.
func TestOldSessionSurvivesAFailedRelogin(t *testing.T) {
	t.Parallel()
	st := &failingCreateStore{memStore: newMemStore()}
	h := newHarness(t, withStore(st))
	old := h.signIn()
	if st.sessionCount() != 1 {
		t.Fatalf("sessions = %d, want 1", st.sessionCount())
	}

	st.failCreate.Store(true)
	callbackURL, loginCookie := h.beginLogin("/")
	rec := h.finishLogin(callbackURL, loginCookie, old)
	if rec.Code == http.StatusSeeOther {
		t.Fatalf("expected the relogin to fail, got 303")
	}
	st.failCreate.Store(false)

	if _, err := h.getMe(old); err != nil {
		t.Fatalf("old session no longer valid after failed re-login (user logged out): %v", err)
	}
}
