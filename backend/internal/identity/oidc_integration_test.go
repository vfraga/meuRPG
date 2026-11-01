package identity

import (
	"bytes"
	"encoding/json"
	"fmt"
	"log/slog"
	"net/http"
	"net/http/cookiejar"
	"net/url"
	"os"
	"strings"
	"testing"
	"time"

	"connectrpc.com/connect"

	identityv1 "github.com/PuraFome/meuRPG/backend/gen/meurpg/identity/v1"
	"github.com/PuraFome/meuRPG/backend/internal/platform/config"
	"github.com/PuraFome/meuRPG/backend/internal/platform/reqlimit"
)

// TestRealProviderSignIn signs in against a real OpenID Connect provider
// running locally. It is skipped unless these variables are set:
//
//	MEURPG_TEST_OIDC_ISSUER         e.g. https://localhost:9443/oauth2/token
//	MEURPG_TEST_OIDC_CLIENT_ID
//	MEURPG_TEST_OIDC_CLIENT_SECRET
//	MEURPG_TEST_OIDC_REDIRECT_URL   e.g. http://localhost:8080/auth/callback
//	MEURPG_TEST_OIDC_CA_FILE        the provider's CA certificate (PEM)
//	MEURPG_TEST_OIDC_USERNAME       a test user at the provider
//	MEURPG_TEST_OIDC_PASSWORD
//
// The Service under test is the production code, unchanged. Only the part
// that plays the browser is provider-specific: with no browser to type the
// password, it uses WSO2 Identity Server's app-native authentication API
// (response_mode=direct, then POST /oauth2/authn) to get the authorization
// code, and hands that code to our callback like the browser would.
//
// Nothing secret is logged, even when the test fails.
func TestRealProviderSignIn(t *testing.T) {
	env := map[string]string{}
	for _, name := range []string{"ISSUER", "CLIENT_ID", "CLIENT_SECRET", "REDIRECT_URL", "CA_FILE", "USERNAME", "PASSWORD"} {
		env[name] = os.Getenv("MEURPG_TEST_OIDC_" + name)
		if env[name] == "" {
			t.Skipf("MEURPG_TEST_OIDC_%s is not set; skipping the real-provider test", name)
		}
	}

	mem := newMemStore()
	logs := &syncBuffer{}
	svc, err := New(t.Context(), Config{
		OIDC: config.OIDC{
			IssuerURL:    env["ISSUER"],
			ClientID:     env["CLIENT_ID"],
			ClientSecret: config.Secret(env["CLIENT_SECRET"]),
			RedirectURL:  env["REDIRECT_URL"],
			CAFile:       env["CA_FILE"],
			// Makes the provider return auth_time, which we record.
			MaxAge: time.Hour,
		},
		Store:  mem,
		Logger: slog.New(slog.NewJSONHandler(logs, nil)),
	})
	if err != nil {
		t.Fatalf("New() error = %v", err)
	}
	if _, err := svc.providers.get(t.Context()); err != nil {
		t.Fatalf("discovery against the real provider failed: %v", err)
	}
	h := &harness{t: t, store: mem, mem: mem, logs: logs, svc: svc, mux: http.NewServeMux()}
	svc.Mount(h.mux.Handle, reqlimit.HandlerOption())

	browser := newProviderBrowser(t, svc.providers.p.client, env)

	signIn := func() *http.Cookie {
		t.Helper()
		rec := h.get("/auth/login?return_to=/campaigns")
		if rec.Code != http.StatusFound {
			t.Fatalf("GET /auth/login status = %d, want 302", rec.Code)
		}
		loginCookie := findCookie(t, rec, loginCookieName)
		authURL, _ := url.Parse(rec.Header().Get("Location"))
		if got := authURL.Query().Get("max_age"); got != "3600" {
			t.Errorf("authorize max_age = %q, want 3600", got)
		}

		code := browser.signIn(authURL)
		callback := env["REDIRECT_URL"] + "?" + url.Values{"code": {code}, "state": {authURL.Query().Get("state")}}.Encode()
		rec = h.finishLogin(callback, loginCookie)
		if rec.Code != http.StatusSeeOther || rec.Header().Get("Location") != "/campaigns" {
			t.Fatalf("callback: status %d, Location %q; want 303 to /campaigns. Logs: %s", rec.Code, rec.Header().Get("Location"), logs)
		}
		return findCookie(t, rec, SessionCookieName)
	}

	session := signIn()
	me, err := h.getMe(session)
	if err != nil {
		t.Fatalf("GetMe() error = %v", err)
	}
	userID := me.Msg.GetUser().GetId()
	h.mem.mu.Lock()
	for _, id := range h.mem.identities {
		// Only whether it was stored: the value is personal data.
		t.Logf("verified e-mail stored as a security contact: %v", id.email != "")
	}
	h.mem.mu.Unlock()
	if got := h.mem.sessionAuthTime(sessionID(t, h, session)); got.IsZero() {
		t.Error("no auth_time recorded, although max_age was sent")
	} else {
		t.Logf("auth_time recorded (informational only): %s ago", time.Since(got).Round(time.Second))
	}

	// A second sign-in (the provider's SSO session may skip the password)
	// reaches the same account through (issuer, subject).
	again, err := h.getMe(signIn())
	if err != nil {
		t.Fatalf("GetMe() after the second sign-in: %v", err)
	}
	if again.Msg.GetUser().GetId() != userID {
		t.Errorf("second sign-in reached account %s, want %s", again.Msg.GetUser().GetId(), userID)
	}

	if _, err := h.client(session).SignOut(t.Context(), connect.NewRequest(&identityv1.SignOutRequest{})); err != nil {
		t.Fatalf("SignOut() error = %v", err)
	}
	if _, err := h.getMe(session); !isUnauthenticated(err) {
		t.Errorf("GetMe() after SignOut error = %v, want unauthenticated", err)
	}

	for _, secret := range []string{env["CLIENT_SECRET"], env["PASSWORD"], session.Value} {
		if strings.Contains(logs.String(), secret) {
			t.Error("the logs contain a secret")
		}
	}
}

// providerBrowser plays the user's browser at a WSO2 Identity Server, via
// its app-native authentication API. It keeps the provider's cookies, so a
// second sign-in can reuse the provider's session like a browser would.
type providerBrowser struct {
	t      *testing.T
	client *http.Client
	env    map[string]string
}

func newProviderBrowser(t *testing.T, base *http.Client, env map[string]string) *providerBrowser {
	t.Helper()
	jar, err := cookiejar.New(nil)
	if err != nil {
		t.Fatalf("cookie jar: %v", err)
	}
	client := *base
	client.Jar = jar
	return &providerBrowser{t: t, client: &client, env: env}
}

// appNativeResponse is the part of the app-native API's answers we read.
type appNativeResponse struct {
	FlowID     string `json:"flowId"`
	FlowStatus string `json:"flowStatus"`
	NextStep   struct {
		Authenticators []struct {
			AuthenticatorID string `json:"authenticatorId"`
		} `json:"authenticators"`
	} `json:"nextStep"`
	Links []struct {
		Name string `json:"name"`
		Href string `json:"href"`
	} `json:"links"`
	AuthData struct {
		Code string `json:"code"`
	} `json:"authData"`
}

// signIn follows authURL (built by our /auth/login) and returns the
// authorization code the provider issues for it.
func (b *providerBrowser) signIn(authURL *url.URL) string {
	b.t.Helper()
	direct := *authURL
	q := direct.Query()
	q.Set("response_mode", "direct")
	direct.RawQuery = q.Encode()

	var start appNativeResponse
	b.do(http.MethodGet, direct.String(), nil, &start)
	switch start.FlowStatus {
	case "SUCCESS_COMPLETED": // the provider's session was reused (SSO)
		return b.code(start)
	case "INCOMPLETE":
	default:
		b.t.Fatalf("authorize flowStatus = %q, want INCOMPLETE or SUCCESS_COMPLETED", start.FlowStatus)
	}
	if len(start.NextStep.Authenticators) == 0 {
		b.t.Fatal("authorize response offers no authenticator")
	}

	authnURL := strings.Replace(authURL.Scheme+"://"+authURL.Host+authURL.Path, "/authorize", "/authn", 1)
	for _, link := range start.Links {
		if link.Name == "authentication" && link.Href != "" {
			authnURL = link.Href
		}
	}
	body, _ := json.Marshal(map[string]any{
		"flowId": start.FlowID,
		"selectedAuthenticator": map[string]any{
			"authenticatorId": start.NextStep.Authenticators[0].AuthenticatorID,
			"params":          map[string]string{"username": b.env["USERNAME"], "password": b.env["PASSWORD"]},
		},
	})
	var done appNativeResponse
	b.do(http.MethodPost, authnURL, body, &done)
	if done.FlowStatus != "SUCCESS_COMPLETED" {
		b.t.Fatalf("authn flowStatus = %q, want SUCCESS_COMPLETED", done.FlowStatus)
	}
	return b.code(done)
}

func (b *providerBrowser) code(r appNativeResponse) string {
	b.t.Helper()
	if r.AuthData.Code == "" {
		b.t.Fatal("the provider completed the flow without a code")
	}
	return r.AuthData.Code
}

// do sends one request with the client's HTTP Basic credentials (the
// app-native API wants them even at the authorization endpoint) and
// decodes the JSON answer. On failure it reports only the status.
func (b *providerBrowser) do(method, target string, body []byte, out any) {
	b.t.Helper()
	req, err := http.NewRequestWithContext(b.t.Context(), method, target, bytes.NewReader(body))
	if err != nil {
		b.t.Fatalf("new request: %v", err)
	}
	req.SetBasicAuth(b.env["CLIENT_ID"], b.env["CLIENT_SECRET"])
	req.Header.Set("Accept", "application/json")
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	resp, err := b.client.Do(req)
	if err != nil {
		b.t.Fatalf("%s %s: %v", method, redactQuery(target), err)
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode != http.StatusOK {
		b.t.Fatalf("%s %s: status %d", method, redactQuery(target), resp.StatusCode)
	}
	if err := json.NewDecoder(resp.Body).Decode(out); err != nil {
		b.t.Fatalf("decode %s answer: %v", redactQuery(target), err)
	}
}

// redactQuery drops the query (state, nonce, PKCE challenge) from a URL
// before it goes into a failure message.
func redactQuery(target string) string {
	u, err := url.Parse(target)
	if err != nil {
		return "<unparseable URL>"
	}
	return fmt.Sprintf("%s://%s%s", u.Scheme, u.Host, u.Path)
}
