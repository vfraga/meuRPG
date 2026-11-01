package identity

import (
	"context"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"errors"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"sync"
	"testing"

	"github.com/PuraFome/meuRPG/backend/internal/platform/httpserver"
	"github.com/PuraFome/meuRPG/backend/internal/platform/reqlimit"
	"github.com/PuraFome/meuRPG/backend/internal/platform/secret"
)

// fakeIntent is an IntentHandler that behaves like the campaign invite one
// by default: Prepare keeps the SHA-256 of a well-formed secret, and
// Complete sends the browser to /feito. Tests replace either step.
type fakeIntent struct {
	mu        sync.Mutex
	prepare   func(payload string) ([]byte, error)
	complete  func(userID string, data []byte) (string, error)
	completed []fakeCompletion
}

type fakeCompletion struct {
	userID string
	data   []byte
}

func newFakeIntent() *fakeIntent {
	return &fakeIntent{
		prepare: func(payload string) ([]byte, error) {
			hash, ok := secret.Hash(payload)
			if !ok {
				return nil, errors.New("not a token")
			}
			return hash, nil
		},
		complete: func(string, []byte) (string, error) { return "/feito", nil },
	}
}

func (f *fakeIntent) Prepare(payload string) ([]byte, error) { return f.prepare(payload) }

func (f *fakeIntent) Complete(_ context.Context, who SignedIn, data []byte) (string, error) {
	if who.UserID() == "" {
		return "", errors.New("no signed-in user")
	}
	f.mu.Lock()
	f.completed = append(f.completed, fakeCompletion{userID: who.UserID(), data: data})
	f.mu.Unlock()
	return f.complete(who.UserID(), data)
}

func (f *fakeIntent) completions() []fakeCompletion {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.completed
}

const testIntent = "test_intent"

// intentForm is the form the app posts to sign in with an intent.
func intentForm(returnTo, intent, payload string) url.Values {
	form := url.Values{"return_to": {returnTo}}
	if intent != "" {
		form.Set("intent", intent)
	}
	if payload != "" {
		form.Set("intent_payload", payload)
	}
	return form
}

// signInWithIntent posts the form, lets the fake provider sign the user in
// and visits the callback. It returns the callback's response.
func (h *harness) signInWithIntent(form url.Values) *httptest.ResponseRecorder {
	h.t.Helper()
	rec := h.postLogin(h.mux, form, nil)
	if rec.Code != http.StatusSeeOther {
		h.t.Fatalf("POST /auth/login status = %d, want 303; body: %s", rec.Code, rec.Body)
	}
	loginCookie := findCookie(h.t, rec, loginCookieName)
	return h.finishLogin(h.authorize(rec.Header().Get("Location")), loginCookie)
}

// loginStates returns the login states in the in-memory store.
func (m *memStore) loginStateList() []LoginState {
	m.mu.Lock()
	defer m.mu.Unlock()
	var states []LoginState
	for _, st := range m.loginStates {
		states = append(states, st)
	}
	return states
}

func TestSignInWithAnIntent(t *testing.T) {
	t.Parallel()
	for _, store := range testStores(t) {
		t.Run(store.name, func(t *testing.T) {
			t.Parallel()
			intent := newFakeIntent()
			h := newHarness(t, withStore(store.new(t)), withIntents(map[string]IntentHandler{testIntent: intent}))
			token, hash := secret.New()

			// POST /auth/login answers like GET: a redirect to the
			// provider, with a login cookie, and the /auth headers.
			rec := h.postLogin(h.mux, intentForm("/invite", testIntent, token), nil)
			if rec.Code != http.StatusSeeOther {
				t.Fatalf("POST /auth/login status = %d, want 303; body: %s", rec.Code, rec.Body)
			}
			assertAuthHeaders(t, rec.Header())
			authURL := rec.Header().Get("Location")
			if strings.Contains(authURL, token) {
				t.Error("the provider URL carries the intent payload")
			}
			loginCookie := findCookie(t, rec, loginCookieName)
			assertHostCookie(t, loginCookie)

			// What is kept is what Prepare returned, never the payload.
			if h.mem != nil {
				states := h.mem.loginStateList()
				if len(states) != 1 || states[0].IntentKind != testIntent || string(states[0].IntentData) != string(hash) {
					t.Fatalf("login states = %+v, want one with the payload's hash", states)
				}
				if states[0].ReturnTo != "/invite" {
					t.Errorf("return_to = %q, want /invite", states[0].ReturnTo)
				}
			}

			// The callback creates the session, then completes the intent
			// as the new user, and goes where Complete says.
			rec = h.finishLogin(h.authorize(authURL), loginCookie)
			if rec.Code != http.StatusSeeOther || rec.Header().Get("Location") != "/feito" {
				t.Fatalf("callback = %d to %q, want 303 to /feito; logs: %s", rec.Code, rec.Header().Get("Location"), h.logs)
			}
			me, err := h.getMe(findCookie(t, rec, SessionCookieName))
			if err != nil {
				t.Fatalf("GetMe() error = %v", err)
			}
			done := intent.completions()
			if len(done) != 1 || done[0].userID != me.Msg.GetUser().GetId() || string(done[0].data) != string(hash) {
				t.Errorf("Complete() calls = %+v, want one for user %s with the hash", done, me.Msg.GetUser().GetId())
			}
			if !strings.Contains(h.logs.String(), `"msg":"sign-in intent completed","intent":"test_intent"`) {
				t.Errorf("logs do not record the completed intent: %s", h.logs)
			}
		})
	}
}

func TestSignInWithoutAnIntentByPOST(t *testing.T) {
	t.Parallel()
	h := newHarness(t, withIntents(map[string]IntentHandler{testIntent: newFakeIntent()}))
	rec := h.signInWithIntent(url.Values{"return_to": {"/campaigns"}})
	if rec.Code != http.StatusSeeOther || rec.Header().Get("Location") != "/campaigns" {
		t.Errorf("callback = %d to %q, want 303 to /campaigns", rec.Code, rec.Header().Get("Location"))
	}
	findCookie(t, rec, SessionCookieName)
}

// TestFailedIntentKeepsTheSignIn: when Complete fails, the user is signed
// in all the same; only where the browser goes changes.
func TestFailedIntentKeepsTheSignIn(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name     string
		complete func(string, []byte) (string, error)
		want     string
		wantLog  string
	}{
		{
			name:     "an error with a path goes to that path",
			complete: func(string, []byte) (string, error) { return "/invite/error?reason=expired", errors.New("expired") },
			want:     "/invite/error?reason=expired",
			wantLog:  `"msg":"sign-in intent failed","intent":"test_intent","error":"expired"`,
		},
		{
			name:     "an error without a path goes to return_to",
			complete: func(string, []byte) (string, error) { return "", errors.New("database is down") },
			want:     "/invite",
			wantLog:  `"msg":"sign-in intent failed"`,
		},
		{
			name:     "no path and no error goes to return_to",
			complete: func(string, []byte) (string, error) { return "", nil },
			want:     "/invite",
		},
		{
			name:     "a path on another site goes to return_to",
			complete: func(string, []byte) (string, error) { return "https://evil.example/", nil },
			want:     "/invite",
			wantLog:  `"msg":"sign-in intent returned a path outside this site; using return_to"`,
		},
		{
			name:     "a protocol-relative path goes to return_to",
			complete: func(string, []byte) (string, error) { return "//evil.example", errors.New("x") },
			want:     "/invite",
		},
		{
			name:     "a fragment is dropped",
			complete: func(string, []byte) (string, error) { return "/campaigns/1#t=segredo", nil },
			want:     "/campaigns/1",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			intent := newFakeIntent()
			intent.complete = tt.complete
			h := newHarness(t, withIntents(map[string]IntentHandler{testIntent: intent}))
			token, _ := secret.New()

			rec := h.signInWithIntent(intentForm("/invite", testIntent, token))
			if rec.Code != http.StatusSeeOther || rec.Header().Get("Location") != tt.want {
				t.Errorf("callback = %d to %q, want 303 to %q", rec.Code, rec.Header().Get("Location"), tt.want)
			}
			if _, err := h.getMe(findCookie(t, rec, SessionCookieName)); err != nil {
				t.Errorf("GetMe() after a failed intent error = %v, want the session to work", err)
			}
			if tt.wantLog != "" && !strings.Contains(h.logs.String(), tt.wantLog) {
				t.Errorf("logs lack %s: %s", tt.wantLog, h.logs)
			}
		})
	}
}

// TestIntentUnknownAtTheCallback: a login state whose intent is no longer
// registered (the server restarted with other handlers) still signs in.
func TestIntentUnknownAtTheCallback(t *testing.T) {
	t.Parallel()
	h := newHarness(t, withIntents(map[string]IntentHandler{testIntent: newFakeIntent()}))
	token, _ := secret.New()
	rec := h.postLogin(h.mux, intentForm("/invite", testIntent, token), nil)
	loginCookie := findCookie(t, rec, loginCookieName)
	h.mem.tamperLoginStates(func(st *LoginState) { st.IntentKind = "removed_intent" })

	rec = h.finishLogin(h.authorize(rec.Header().Get("Location")), loginCookie)
	if rec.Code != http.StatusSeeOther || rec.Header().Get("Location") != "/invite" {
		t.Errorf("callback = %d to %q, want 303 to return_to", rec.Code, rec.Header().Get("Location"))
	}
	findCookie(t, rec, SessionCookieName)
	if !strings.Contains(h.logs.String(), `"msg":"sign-in intent skipped"`) {
		t.Errorf("logs do not record the skipped intent: %s", h.logs)
	}
}

// TestLoginFormRejects breaks one thing at a time in POST /auth/login. No
// case may save a login state, set a cookie or redirect.
func TestLoginFormRejects(t *testing.T) {
	t.Parallel()
	token, _ := secret.New()
	valid := intentForm("/invite", testIntent, token)

	tests := []struct {
		name       string
		body       string
		query      string
		setup      func(req *http.Request)
		intent     func(f *fakeIntent)
		wantStatus int
		wantReason string
	}{
		{name: "unknown intent kind", body: intentForm("/", "not_registered", token).Encode(), wantStatus: 400, wantReason: "unknown_intent"},
		{name: "payload without an intent", body: intentForm("/", "", token).Encode(), wantStatus: 400, wantReason: "payload_without_intent"},
		{name: "garbage payload", body: intentForm("/", testIntent, "<script>").Encode(), wantStatus: 400, wantReason: "invalid_intent_payload"},
		{name: "empty payload", body: intentForm("/", testIntent, "").Encode(), wantStatus: 400, wantReason: "invalid_intent_payload"},
		{name: "oversized payload", body: intentForm("/", testIntent, strings.Repeat("a", maxIntentPayloadLength+1)).Encode(), wantStatus: 400, wantReason: "intent_payload_too_long"},
		{name: "oversized body", body: valid.Encode() + "&padding=" + strings.Repeat("a", maxLoginFormBytes), wantStatus: 413, wantReason: "form_too_large"},
		{name: "unsafe return_to", body: intentForm("https://evil.example", testIntent, token).Encode(), wantStatus: 400, wantReason: "unsafe_return_to"},
		{name: "repeated intent_payload", body: valid.Encode() + "&intent_payload=" + token, wantStatus: 400, wantReason: "repeated_field"},
		{name: "fields in the query string", body: valid.Encode(), query: "intent_payload=" + token, wantStatus: 400, wantReason: "query_on_post"},
		{
			name: "JSON instead of a form", body: `{"intent":"test_intent"}`,
			setup:      func(r *http.Request) { r.Header.Set("Content-Type", "application/json") },
			wantStatus: 415, wantReason: "not_a_form",
		},
		{
			name: "multipart form", body: valid.Encode(),
			setup:      func(r *http.Request) { r.Header.Set("Content-Type", "multipart/form-data; boundary=x") },
			wantStatus: 415, wantReason: "not_a_form",
		},
		{
			name: "Prepare keeps more than MaxIntentDataBytes", body: valid.Encode(),
			intent: func(f *fakeIntent) {
				f.prepare = func(string) ([]byte, error) { return make([]byte, MaxIntentDataBytes+1), nil }
			},
			wantStatus: 503, wantReason: "intent_data_too_long",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			intent := newFakeIntent()
			if tt.intent != nil {
				tt.intent(intent)
			}
			h := newHarness(t, withIntents(map[string]IntentHandler{testIntent: intent}))

			target := "https://meurpg.test/auth/login"
			if tt.query != "" {
				target += "?" + tt.query
			}
			req := httptest.NewRequestWithContext(t.Context(), http.MethodPost, target, strings.NewReader(tt.body))
			req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
			if tt.setup != nil {
				tt.setup(req)
			}
			rec := httptest.NewRecorder()
			h.mux.ServeHTTP(rec, req)

			if rec.Code != tt.wantStatus {
				t.Errorf("status = %d, want %d; body: %s", rec.Code, tt.wantStatus, rec.Body)
			}
			assertAuthHeaders(t, rec.Header())
			if loc := rec.Header().Get("Location"); loc != "" {
				t.Errorf("redirected to %q", loc)
			}
			if c := cookieIn(rec.Result().Cookies(), loginCookieName); c != nil {
				t.Errorf("set a login cookie: %+v", c)
			}
			if n := len(h.mem.loginStateList()); n != 0 {
				t.Errorf("saved %d login states, want 0", n)
			}
			logs := h.logs.String()
			if !strings.Contains(logs, `"reason":"`+tt.wantReason+`"`) {
				t.Errorf("logs lack reason %q: %s", tt.wantReason, logs)
			}
			// Nothing the client sent is logged: not the payload, not an
			// unknown intent kind.
			for _, value := range []string{token, "not_registered", "<script>", "evil.example"} {
				if strings.Contains(logs, value) {
					t.Errorf("logs contain %q from the request: %s", value, logs)
				}
			}
		})
	}
}

// TestIntentsNeverTravelInAURL: GET /auth/login refuses intent fields, so
// a secret payload cannot end up in the request logs by mistake.
func TestIntentsNeverTravelInAURL(t *testing.T) {
	t.Parallel()
	h := newHarness(t, withIntents(map[string]IntentHandler{testIntent: newFakeIntent()}))
	token, _ := secret.New()
	for _, query := range []string{
		"return_to=/&intent=" + testIntent + "&intent_payload=" + token,
		"return_to=/&intent_payload=" + token,
		"return_to=/&intent=" + testIntent,
	} {
		rec := h.get("/auth/login?" + query)
		if rec.Code != http.StatusBadRequest {
			t.Errorf("GET /auth/login?%s status = %d, want 400", query, rec.Code)
		}
	}
	if n := len(h.mem.loginStateList()); n != 0 {
		t.Errorf("saved %d login states, want 0", n)
	}
	if strings.Contains(h.logs.String(), token) {
		t.Errorf("logs contain the payload: %s", h.logs)
	}
}

// TestLoginFormCrossOrigin mounts the Service on the real HTTP server, whose
// http.CrossOriginProtection must refuse a sign-in form posted from another
// site: otherwise a page anywhere could make a visitor's browser sign in and
// join a campaign chosen by the attacker.
func TestLoginFormCrossOrigin(t *testing.T) {
	t.Parallel()
	intent := newFakeIntent()
	h := newHarness(t, withIntents(map[string]IntentHandler{testIntent: intent}))
	srv := httpserver.New(httpserver.Config{Logger: slog.New(slog.DiscardHandler)})
	h.svc.Mount(srv.Handle, reqlimit.HandlerOption())
	token, _ := secret.New()
	form := intentForm("/invite", testIntent, token)

	tests := []struct {
		name    string
		headers map[string]string
		want    int
	}{
		{"cross-site form", map[string]string{"Sec-Fetch-Site": "cross-site"}, http.StatusForbidden},
		{"same-site, other origin", map[string]string{"Sec-Fetch-Site": "same-site"}, http.StatusForbidden},
		{"old browser, other Origin", map[string]string{"Origin": "https://evil.example"}, http.StatusForbidden},
		{"the app's own form", map[string]string{"Sec-Fetch-Site": "same-origin"}, http.StatusSeeOther},
		{"old browser, same Origin", map[string]string{"Origin": "https://meurpg.test"}, http.StatusSeeOther},
	}
	for _, tt := range tests {
		rec := h.postLogin(srv.Handler(), form, tt.headers)
		if rec.Code != tt.want {
			t.Errorf("%s: status = %d, want %d", tt.name, rec.Code, tt.want)
		}
	}
	if n := len(h.mem.loginStateList()); n != 2 {
		t.Errorf("login states = %d, want 2: only the same-origin posts start a sign-in", n)
	}
}

// TestLoginFormSharesTheRateLimit: GET and POST /auth/login draw from the
// same per-client budget, so switching methods does not double it.
func TestLoginFormSharesTheRateLimit(t *testing.T) {
	t.Parallel()
	h := newHarness(t, withIntents(map[string]IntentHandler{testIntent: newFakeIntent()}))
	burst := loginRateLimit.PerClient.Burst
	token, _ := secret.New()
	form := intentForm("/invite", testIntent, token)

	for i := range burst {
		var code int
		if i%2 == 0 {
			code = h.get("/auth/login").Code
		} else {
			code = h.postLogin(h.mux, form, nil).Code
		}
		if code != http.StatusFound && code != http.StatusSeeOther {
			t.Fatalf("sign-in %d: status = %d, want a redirect", i+1, code)
		}
	}
	rec := h.postLogin(h.mux, form, nil)
	if rec.Code != http.StatusTooManyRequests || rec.Header().Get("Retry-After") == "" {
		t.Errorf("POST over the limit: status = %d, Retry-After %q; want 429 with Retry-After", rec.Code, rec.Header().Get("Retry-After"))
	}
	if cookieIn(rec.Result().Cookies(), loginCookieName) != nil {
		t.Error("a refused sign-in set a login cookie")
	}
	if n := len(h.mem.loginStateList()); n != burst {
		t.Errorf("login states = %d, want %d: a refused sign-in must not write", n, burst)
	}
}

// TestIntentLogsHaveNoSecrets runs a sign-in with an intent, a failed one
// and a refused one, then searches the logs for the payload and its hash.
func TestIntentLogsHaveNoSecrets(t *testing.T) {
	t.Parallel()
	intent := newFakeIntent()
	intent.complete = func(string, []byte) (string, error) { return "/invite/error?reason=expired", errors.New("expired") }
	h := newHarness(t, withIntents(map[string]IntentHandler{testIntent: intent}))
	token, hash := secret.New()

	h.signInWithIntent(intentForm("/invite", testIntent, token))
	h.postLogin(h.mux, intentForm("/invite", "unknown_kind", token), nil)
	h.postLogin(h.mux, intentForm("/invite", testIntent, token+"x"), nil)

	logs := h.logs.String()
	for what, value := range map[string]string{
		"payload":              token,
		"hash (hex)":           hex.EncodeToString(hash),
		"hash (base64)":        base64.StdEncoding.EncodeToString(hash),
		"hash (base64url)":     base64.RawURLEncoding.EncodeToString(hash),
		"payload's raw sha256": hex.EncodeToString(sha256Bytes(token)),
	} {
		if strings.Contains(logs, value) {
			t.Errorf("logs contain the %s: %s", what, logs)
		}
	}
}

func sha256Bytes(s string) []byte {
	sum := sha256.Sum256([]byte(s))
	return sum[:]
}

func TestNewValidatesIntents(t *testing.T) {
	t.Parallel()
	idp := newFakeIDP(t)
	base := Config{Store: newMemStore(), Logger: discardLogger()}
	base.OIDC.IssuerURL = idp.issuer()
	base.OIDC.ClientID = idp.clientID
	base.OIDC.RedirectURL = idp.redirectURL

	for name, intents := range map[string]map[string]IntentHandler{
		"empty kind":          {"": newFakeIntent()},
		"upper case":          {"Campaign_Invite": newFakeIntent()},
		"starts with a digit": {"1invite": newFakeIntent()},
		"too long":            {strings.Repeat("a", 33): newFakeIntent()},
		"with a space":        {"campaign invite": newFakeIntent()},
		"nil handler":         {"campaign_invite": nil},
	} {
		cfg := base
		cfg.Intents = intents
		if _, err := New(t.Context(), cfg); err == nil {
			t.Errorf("%s: New() error = nil, want an error", name)
		}
	}

	// The Service keeps its own copy: changing the map later changes nothing.
	intents := map[string]IntentHandler{"campaign_invite": newFakeIntent()}
	cfg := base
	cfg.Intents = intents
	svc, err := New(t.Context(), cfg)
	if err != nil {
		t.Fatalf("New() error = %v", err)
	}
	delete(intents, "campaign_invite")
	if _, ok := svc.intents["campaign_invite"]; !ok {
		t.Error("deleting from the caller's map removed the Service's intent")
	}
}

// TestSignedInIsACapability: outside this package, SignedIn{} is all anyone
// can build, and it holds no user. The callback is the only place that
// fills one, with the user whose session it just created
// (TestSignInWithAnIntent checks that user).
func TestSignedInIsACapability(t *testing.T) {
	t.Parallel()
	if got := (SignedIn{}).UserID(); got != "" {
		t.Errorf("SignedIn{}.UserID() = %q, want empty", got)
	}
	if got := (SignedIn{userID: "u1"}).UserID(); got != "u1" {
		t.Errorf("UserID() = %q, want u1", got)
	}
}
