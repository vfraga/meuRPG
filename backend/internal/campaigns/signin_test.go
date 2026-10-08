package campaigns

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"sync"
	"testing"
	"time"

	"connectrpc.com/connect"
	"github.com/jackc/pgx/v5/pgxpool"

	campaignsv1 "github.com/PuraFome/meuRPG/backend/gen/meurpg/campaigns/v1"
	"github.com/PuraFome/meuRPG/backend/gen/meurpg/campaigns/v1/campaignsv1connect"
	identityv1 "github.com/PuraFome/meuRPG/backend/gen/meurpg/identity/v1"
	"github.com/PuraFome/meuRPG/backend/gen/meurpg/identity/v1/identityv1connect"
	"github.com/PuraFome/meuRPG/backend/internal/identity"
	"github.com/PuraFome/meuRPG/backend/internal/identity/oidctest"
	"github.com/PuraFome/meuRPG/backend/internal/platform/config"
	"github.com/PuraFome/meuRPG/backend/internal/platform/dbtest"
	"github.com/PuraFome/meuRPG/backend/internal/platform/httpserver"
	"github.com/PuraFome/meuRPG/backend/internal/platform/logging"
	"github.com/PuraFome/meuRPG/backend/internal/platform/rpclog"
	"github.com/PuraFome/meuRPG/backend/internal/platform/secret"
)

// These tests accept invites through sign-in (InviteIntentKind), end to end
// in Go: a fake OpenID Connect provider, the identity and campaigns modules
// wired as cmd/api wires them, on one CockroachDB database, behind the real
// HTTP server with its cross-origin protection. They need
// MEURPG_TEST_DATABASE_URL, like the rest of this package.

const redirectURL = "https://meurpg.test" + config.CallbackPath

// signInStack is the whole server, plus the provider.
type signInStack struct {
	t       *testing.T
	pool    *pgxpool.Pool
	idp     *oidctest.Provider
	idpHTTP *http.Client // trusts the provider's test certificate
	clock   *fakeClock   // campaigns' clock only; identity keeps real time, like the provider's tokens
	logs    *syncBuffer
	handler http.Handler
	// campaignsSvc is the campaigns service behind handler.
	campaignsSvc *Service
	apiURL       string
	apiHTTP      *http.Client
	subjects     int
}

func newSignInStack(t *testing.T) *signInStack {
	t.Helper()
	pool := dbtest.NewPool(t, "meurpg_campaigns_signin_test")
	st := &signInStack{
		t:     t,
		pool:  pool,
		clock: &fakeClock{now: time.Now().Truncate(time.Microsecond)},
		logs:  &syncBuffer{},
	}
	logger := logging.New(st.logs, slog.LevelDebug)

	// The provider, over TLS, signing in its only user at once.
	idpServer := httptest.NewUnstartedServer(nil)
	issuer := "https://" + idpServer.Listener.Addr().String()
	provider, err := oidctest.New(oidctest.Config{
		Issuer:       issuer,
		ClientID:     "meurpg-test",
		ClientSecret: "test-client-secret",
		RedirectURIs: []string{redirectURL},
		Users:        []oidctest.User{{Subject: "nobody-yet"}},
		AutoSignIn:   true,
	})
	if err != nil {
		t.Fatalf("oidctest.New() error = %v", err)
	}
	idpServer.Config.Handler = provider
	idpServer.StartTLS()
	t.Cleanup(idpServer.Close)
	st.idp = provider
	st.idpHTTP = idpServer.Client()
	st.idpHTTP.CheckRedirect = func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }

	// The modules, wired as in cmd/api.
	users := identity.NewPostgresStore(pool)
	campaigns, err := New(Config{Pool: pool, Profiles: users, Logger: logger, Now: st.clock.Now})
	if err != nil {
		t.Fatalf("New() error = %v", err)
	}
	signIn, err := identity.New(t.Context(), identity.Config{
		OIDC: config.OIDC{
			IssuerURL:    issuer,
			ClientID:     "meurpg-test",
			ClientSecret: config.Secret("test-client-secret"),
			RedirectURL:  redirectURL,
		},
		Store:      users,
		Logger:     logger,
		HTTPClient: idpServer.Client(),
		Intents:    map[string]identity.IntentHandler{InviteIntentKind: campaigns.InviteIntent()},
	})
	if err != nil {
		t.Fatalf("identity.New() error = %v", err)
	}
	srv := httpserver.New(httpserver.Config{Logger: logger})
	// The logging interceptor goes first, as in cmd/api.
	opts := []connect.HandlerOption{connect.WithRequireConnectProtocolHeader(), connect.WithInterceptors(rpclog.Interceptor(logger))}
	signIn.Mount(srv.Handle, opts...)
	campaigns.Mount(srv.Handle, signIn, opts...)
	st.handler = srv.Handler()
	st.campaignsSvc = campaigns

	api := httptest.NewServer(st.handler)
	t.Cleanup(api.Close)
	st.apiURL, st.apiHTTP = api.URL, api.Client()
	return st
}

// newPerson makes the provider sign in a new person from now on.
func (st *signInStack) newPerson() {
	st.subjects++
	subject := fmt.Sprintf("person-%d", st.subjects)
	st.idp.EditUser(0, func(u *oidctest.User) { u.Subject = subject })
}

// do sends a request to the server as a browser on this site would.
func (st *signInStack) do(req *http.Request, cookies ...*http.Cookie) *httptest.ResponseRecorder {
	st.t.Helper()
	for _, c := range cookies {
		// Only name and value travel in a request; the attributes are the
		// ones the server set, so the literal reads like the real cookie.
		req.AddCookie(&http.Cookie{Name: c.Name, Value: c.Value, Path: "/", Secure: true, HttpOnly: true, SameSite: http.SameSiteLaxMode})
	}
	rec := httptest.NewRecorder()
	st.handler.ServeHTTP(rec, req)
	return rec
}

// postLogin sends the invite page's sign-in form. headers default to a
// same-origin browser request.
func (st *signInStack) postLogin(form url.Values, headers map[string]string) *httptest.ResponseRecorder {
	st.t.Helper()
	req := httptest.NewRequestWithContext(st.t.Context(), http.MethodPost, "https://meurpg.test/auth/login", strings.NewReader(form.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.Header.Set("Sec-Fetch-Site", "same-origin")
	for k, v := range headers {
		req.Header.Set(k, v)
	}
	return st.do(req)
}

// inviteForm is what the invite page posts.
func inviteForm(token string) url.Values {
	return url.Values{
		"return_to":      {"/invite"},
		"intent":         {InviteIntentKind},
		"intent_payload": {token},
	}
}

// finish follows a sign-in from the redirect to the provider to the end of
// the callback, whose response it returns.
func (st *signInStack) finish(start *httptest.ResponseRecorder) *httptest.ResponseRecorder {
	st.t.Helper()
	if start.Code != http.StatusFound && start.Code != http.StatusSeeOther {
		st.t.Fatalf("sign-in start status = %d, want a redirect; body: %s", start.Code, start.Body)
	}
	var loginCookie *http.Cookie
	for _, c := range start.Result().Cookies() {
		if c.Name == "__Host-meurpg_login" {
			loginCookie = c
		}
	}
	if loginCookie == nil {
		st.t.Fatal("the sign-in start set no login cookie")
	}

	req, err := http.NewRequestWithContext(st.t.Context(), http.MethodGet, start.Header().Get("Location"), nil)
	if err != nil {
		st.t.Fatalf("new request: %v", err)
	}
	res, err := st.idpHTTP.Do(req)
	if err != nil {
		st.t.Fatalf("authorize: %v", err)
	}
	_ = res.Body.Close()
	callback, err := url.Parse(res.Header.Get("Location"))
	if err != nil || res.StatusCode != http.StatusFound {
		st.t.Fatalf("authorize = %d to %q, want 302 back to the callback", res.StatusCode, res.Header.Get("Location"))
	}
	return st.do(httptest.NewRequestWithContext(st.t.Context(), http.MethodGet, "https://meurpg.test"+callback.RequestURI(), nil), loginCookie)
}

// signIn signs a new person in, with no intent, and returns their session.
func (st *signInStack) signIn() *http.Cookie {
	st.t.Helper()
	st.newPerson()
	rec := st.finish(st.do(httptest.NewRequestWithContext(st.t.Context(), http.MethodGet, "https://meurpg.test/auth/login?return_to=/", nil)))
	return sessionIn(st.t, rec)
}

// signInWithInvite signs a new person in with the invite as the intent. It
// returns where the callback sent the browser, and the session.
func (st *signInStack) signInWithInvite(token string) (string, *http.Cookie) {
	st.t.Helper()
	st.newPerson()
	return st.continueWithInvite(token)
}

// continueWithInvite is signInWithInvite for the person the provider signs
// in now.
func (st *signInStack) continueWithInvite(token string) (string, *http.Cookie) {
	st.t.Helper()
	rec := st.finish(st.postLogin(inviteForm(token), nil))
	if rec.Code != http.StatusSeeOther {
		st.t.Fatalf("callback status = %d, want 303; body: %s; logs: %s", rec.Code, rec.Body, st.logs)
	}
	return rec.Header().Get("Location"), sessionIn(st.t, rec)
}

func sessionIn(t *testing.T, rec *httptest.ResponseRecorder) *http.Cookie {
	t.Helper()
	for _, c := range rec.Result().Cookies() {
		if c.Name == identity.SessionCookieName && c.Value != "" {
			return c
		}
	}
	t.Fatalf("no session cookie; status %d, Set-Cookie %q", rec.Code, rec.Header().Values("Set-Cookie"))
	return nil
}

// withCookie is a Connect client option that sends a session cookie.
func withCookie(c *http.Cookie) connect.ClientOption {
	return connect.WithInterceptors(connect.UnaryInterceptorFunc(func(next connect.UnaryFunc) connect.UnaryFunc {
		return func(ctx context.Context, req connect.AnyRequest) (connect.AnyResponse, error) {
			req.Header().Set("Cookie", c.Name+"="+c.Value)
			return next(ctx, req)
		}
	}))
}

func (st *signInStack) campaigns(session *http.Cookie) campaignsv1connect.CampaignServiceClient {
	return campaignsv1connect.NewCampaignServiceClient(st.apiHTTP, st.apiURL, withCookie(session))
}

func (st *signInStack) userID(session *http.Cookie) string {
	st.t.Helper()
	me, err := identityv1connect.NewIdentityServiceClient(st.apiHTTP, st.apiURL, withCookie(session)).
		GetMe(st.t.Context(), connect.NewRequest(&identityv1.GetMeRequest{}))
	if err != nil {
		st.t.Fatalf("GetMe() error = %v, want the session to work", err)
	}
	return me.Msg.GetUser().GetId()
}

// masterWithInvite signs a master in, creates a campaign and an invite with
// maxUses uses (0 for the default), and returns the master's session, the
// campaign and the invite's ID and token.
func (st *signInStack) masterWithInvite(maxUses int32) (*http.Cookie, *campaignsv1.Campaign, string, string) {
	st.t.Helper()
	master := st.signIn()
	api := st.campaigns(master)
	created, err := api.CreateCampaign(st.t.Context(), connect.NewRequest(&campaignsv1.CreateCampaignRequest{
		Name: "Mirathel", XpMode: campaignsv1.XpMode_XP_MODE_MILESTONES,
	}))
	if err != nil {
		st.t.Fatalf("CreateCampaign() error = %v", err)
	}
	invite, err := api.CreateInvite(st.t.Context(), connect.NewRequest(&campaignsv1.CreateInviteRequest{
		CampaignId: created.Msg.GetCampaign().GetId(), MaxUses: maxUses,
	}))
	if err != nil {
		st.t.Fatalf("CreateInvite() error = %v", err)
	}
	return master, created.Msg.GetCampaign(), invite.Msg.GetInvite().GetId(), invite.Msg.GetToken()
}

func (st *signInStack) role(campaignID, userID string) string {
	st.t.Helper()
	var role string
	err := st.pool.QueryRow(st.t.Context(), "SELECT role FROM campaign_members WHERE campaign_id = $1 AND user_id = $2", campaignID, userID).Scan(&role)
	if err != nil {
		return ""
	}
	return role
}

func (st *signInStack) count(query string, args ...any) int {
	st.t.Helper()
	var n int
	if err := st.pool.QueryRow(st.t.Context(), query, args...).Scan(&n); err != nil {
		st.t.Fatalf("%s: %v", query, err)
	}
	return n
}

// assertNoSecretsInLogs checks that no token, nor its hash in any common
// encoding, reached the logs.
func (st *signInStack) assertNoSecretsInLogs(tokens ...string) {
	st.t.Helper()
	logs := st.logs.String()
	for _, token := range tokens {
		hash, _ := secret.Hash(token)
		for what, value := range map[string]string{
			"token":            token,
			"hash (hex)":       hex.EncodeToString(hash),
			"hash (base64)":    base64.StdEncoding.EncodeToString(hash),
			"hash (base64url)": base64.RawURLEncoding.EncodeToString(hash),
		} {
			if strings.Contains(logs, value) {
				st.t.Errorf("logs contain an invite %s: %s", what, logs)
			}
		}
	}
}

// MR-003 through sign-in: a signed-out player opens the invite link, signs
// in, and lands in the campaign as a player. Only the token's hash is ever
// stored, and only for the minutes of the sign-in.
func TestSignInWithAnInviteJoinsTheCampaign(t *testing.T) {
	t.Parallel()
	st := newSignInStack(t)
	_, campaign, inviteID, token := st.masterWithInvite(0)

	st.newPerson()
	start := st.postLogin(inviteForm(token), nil)
	if start.Code != http.StatusSeeOther {
		t.Fatalf("POST /auth/login status = %d, want 303; body: %s", start.Code, start.Body)
	}
	if loc := start.Header().Get("Location"); strings.Contains(loc, token) {
		t.Errorf("the redirect to the provider carries the token: %s", loc)
	}

	// The login state row: the intent and the token's hash, and nothing
	// that holds the token itself, in any column.
	rows, err := st.pool.Query(t.Context(), "SELECT * FROM oidc_login_states")
	if err != nil {
		t.Fatalf("read login states: %v", err)
	}
	var stored [][]any
	for rows.Next() {
		values, err := rows.Values()
		if err != nil {
			t.Fatalf("read login state: %v", err)
		}
		stored = append(stored, values)
	}
	rows.Close()
	if len(stored) != 1 {
		t.Fatalf("login states = %d, want 1", len(stored))
	}
	var kind string
	var data []byte
	if err := st.pool.QueryRow(t.Context(), "SELECT intent_kind, intent_data FROM oidc_login_states").Scan(&kind, &data); err != nil {
		t.Fatalf("read intent: %v", err)
	}
	wantHash, _ := secret.Hash(token)
	if kind != InviteIntentKind || !bytes.Equal(data, wantHash) {
		t.Errorf("stored intent = %q %x, want %q and the token's SHA-256 %x", kind, data, InviteIntentKind, wantHash)
	}
	rawToken, _ := base64.RawURLEncoding.DecodeString(token)
	for i, value := range stored[0] {
		text := fmt.Sprintf("%v %s", value, value)
		if b, ok := value.([]byte); ok {
			text += " " + string(b)
			if bytes.Contains(b, rawToken) {
				t.Errorf("column %d holds the token's raw bytes", i)
			}
		}
		if strings.Contains(text, token) {
			t.Errorf("column %d holds the token: %v", i, value)
		}
	}

	// The callback signs the player in and makes them a player.
	rec := st.finish(start)
	if rec.Code != http.StatusSeeOther || rec.Header().Get("Location") != "/campaigns/"+campaign.GetId() {
		t.Fatalf("callback = %d to %q, want 303 to /campaigns/%s; logs: %s", rec.Code, rec.Header().Get("Location"), campaign.GetId(), st.logs)
	}
	session := sessionIn(t, rec)
	player := st.userID(session)
	if got := st.role(campaign.GetId(), player); got != "player" {
		t.Errorf("role = %q, want player", got)
	}
	list, err := st.campaigns(session).ListMyCampaigns(t.Context(), connect.NewRequest(&campaignsv1.ListMyCampaignsRequest{}))
	if err != nil || len(list.Msg.GetCampaigns()) != 1 || list.Msg.GetCampaigns()[0].GetMyRole() != campaignsv1.Role_ROLE_PLAYER {
		t.Errorf("ListMyCampaigns() = %v, %v; want Mirathel as a player", list, err)
	}
	if n := st.count("SELECT use_count FROM campaign_invites WHERE id = $1", inviteID); n != 1 {
		t.Errorf("use_count = %d, want 1", n)
	}
	if n := st.count("SELECT count(*) FROM oidc_login_states"); n != 0 {
		t.Errorf("login states after the callback = %d, want 0 (single use)", n)
	}
	st.assertNoSecretsInLogs(token)
}

// TestSignInWithAnUnusableInvite: the sign-in works, the session stays, and
// the browser goes to the invite error page with the reason.
func TestSignInWithAnUnusableInvite(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name   string
		spoil  func(st *signInStack, master *http.Cookie, campaignID, inviteID, token string) string
		reason string
	}{
		{
			name: "expired",
			spoil: func(st *signInStack, _ *http.Cookie, _, _, token string) string {
				st.clock.Advance(DefaultInviteLifetime)
				return token
			},
			reason: InviteFailureExpired,
		},
		{
			name: "revoked",
			spoil: func(st *signInStack, master *http.Cookie, campaignID, inviteID, token string) string {
				_, err := st.campaigns(master).RevokeInvite(st.t.Context(), connect.NewRequest(&campaignsv1.RevokeInviteRequest{CampaignId: campaignID, InviteId: inviteID}))
				if err != nil {
					st.t.Fatalf("RevokeInvite() error = %v", err)
				}
				return token
			},
			reason: InviteFailureRevoked,
		},
		{
			name: "used up by someone else",
			spoil: func(st *signInStack, _ *http.Cookie, _, _, token string) string {
				if loc, _ := st.signInWithInvite(token); !strings.HasPrefix(loc, "/campaigns/") {
					st.t.Fatalf("first sign-in with the invite went to %q", loc)
				}
				return token
			},
			reason: InviteFailureUsedUp,
		},
		{
			name: "never issued",
			spoil: func(*signInStack, *http.Cookie, string, string, string) string {
				forged, _ := secret.New()
				return forged
			},
			reason: InviteFailureNotFound,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			st := newSignInStack(t)
			master, campaign, inviteID, token := st.masterWithInvite(0)
			used := tt.spoil(st, master, campaign.GetId(), inviteID, token)

			location, session := st.signInWithInvite(used)
			if want := "/invite/error?reason=" + tt.reason; location != want {
				t.Errorf("callback went to %q, want %q; logs: %s", location, want, st.logs)
			}
			player := st.userID(session) // the session works
			if got := st.role(campaign.GetId(), player); got != "" {
				t.Errorf("role = %q, want no membership", got)
			}
			if !strings.Contains(st.logs.String(), `"error":"campaign invite: `+tt.reason+`"`) {
				t.Errorf("logs do not give the reason %s: %s", tt.reason, st.logs)
			}
			st.assertNoSecretsInLogs(token, used)
		})
	}
}

// TestSignInWithAnInviteAsAMember: a member who opens the link again, even
// after it stopped working, goes to the campaign, and no use is spent.
func TestSignInWithAnInviteAsAMember(t *testing.T) {
	t.Parallel()
	st := newSignInStack(t)
	_, campaign, inviteID, token := st.masterWithInvite(2)
	want := "/campaigns/" + campaign.GetId()

	// The master opens their own link: they stay the master.
	st.idp.EditUser(0, func(u *oidctest.User) { u.Subject = "person-1" })
	if location, session := st.continueWithInvite(token); location != want || st.role(campaign.GetId(), st.userID(session)) != "master" {
		t.Errorf("master's sign-in went to %q, want %q as the master", location, want)
	}
	if n := st.count("SELECT use_count FROM campaign_invites WHERE id = $1", inviteID); n != 0 {
		t.Errorf("use_count = %d, want 0", n)
	}

	// A player joins, then signs in with the link again after it expired.
	if location, _ := st.signInWithInvite(token); location != want {
		t.Fatalf("player's first sign-in went to %q, want %q", location, want)
	}
	st.clock.Advance(DefaultInviteLifetime)
	if location, session := st.continueWithInvite(token); location != want || st.role(campaign.GetId(), st.userID(session)) != "player" {
		t.Errorf("player's second sign-in went to %q, want %q as a player", location, want)
	}
	if n := st.count("SELECT use_count FROM campaign_invites WHERE id = $1", inviteID); n != 1 {
		t.Errorf("use_count = %d, want 1", n)
	}
	st.assertNoSecretsInLogs(token)
}

// TestSignInWithCorruptIntentData: if what was kept at login start is not a
// token hash (only a bug could cause it), the sign-in still works and the
// browser goes to the "invalid" error page. This runs Complete through
// identity's real callback, the only code that can call it for a user.
func TestSignInWithCorruptIntentData(t *testing.T) {
	t.Parallel()
	st := newSignInStack(t)
	_, campaign, _, token := st.masterWithInvite(0)

	st.newPerson()
	start := st.postLogin(inviteForm(token), nil)
	if _, err := st.pool.Exec(t.Context(), "UPDATE oidc_login_states SET intent_data = '\\x01' WHERE intent_kind IS NOT NULL"); err != nil {
		t.Fatalf("corrupt the login state: %v", err)
	}
	rec := st.finish(start)
	if got := rec.Header().Get("Location"); rec.Code != http.StatusSeeOther || got != "/invite/error?reason=invalid" {
		t.Errorf("callback = %d to %q, want 303 to /invite/error?reason=invalid", rec.Code, got)
	}
	player := st.userID(sessionIn(t, rec)) // the session works
	if got := st.role(campaign.GetId(), player); got != "" {
		t.Errorf("role = %q, want no membership", got)
	}
}

// TestInviteIntentWithoutASignedInUser: with a real, active invite, the
// SignedIn{} that code outside identity could build makes nobody a member
// and spends no use.
func TestInviteIntentWithoutASignedInUser(t *testing.T) {
	t.Parallel()
	st := newSignInStack(t)
	_, campaign, inviteID, token := st.masterWithInvite(0)
	hash, _ := secret.Hash(token)

	path, err := st.campaignsSvc.InviteIntent().Complete(t.Context(), identity.SignedIn{}, hash)
	if err == nil || path != "" {
		t.Errorf("Complete(SignedIn{}) = %q, %v; want no path and an error", path, err)
	}
	if n := st.count("SELECT count(*) FROM campaign_members WHERE campaign_id = $1", campaign.GetId()); n != 1 {
		t.Errorf("members = %d, want 1 (the master)", n)
	}
	if n := st.count("SELECT use_count FROM campaign_invites WHERE id = $1", inviteID); n != 0 {
		t.Errorf("use_count = %d, want 0", n)
	}
}

// TestInviteSignInRefusals: requests that must not start a sign-in, and
// must store nothing.
func TestInviteSignInRefusals(t *testing.T) {
	t.Parallel()
	st := newSignInStack(t)
	_, _, _, token := st.masterWithInvite(0)
	statesBefore := st.count("SELECT count(*) FROM oidc_login_states")

	tests := []struct {
		name    string
		form    url.Values
		headers map[string]string
		want    int
	}{
		{"unknown intent kind", url.Values{"return_to": {"/"}, "intent": {"campaign_join"}, "intent_payload": {token}}, nil, http.StatusBadRequest},
		{"garbage payload", url.Values{"return_to": {"/"}, "intent": {InviteIntentKind}, "intent_payload": {"not a token"}}, nil, http.StatusBadRequest},
		{"padded token", url.Values{"return_to": {"/"}, "intent": {InviteIntentKind}, "intent_payload": {token + "="}}, nil, http.StatusBadRequest},
		{"oversized payload", url.Values{"return_to": {"/"}, "intent": {InviteIntentKind}, "intent_payload": {strings.Repeat(token, 30)}}, nil, http.StatusBadRequest},
		{"oversized form", url.Values{"return_to": {"/"}, "intent": {InviteIntentKind}, "intent_payload": {strings.Repeat(token, 300)}}, nil, http.StatusRequestEntityTooLarge},
		{"cross-site form", inviteForm(token), map[string]string{"Sec-Fetch-Site": "cross-site"}, http.StatusForbidden},
		{"form from another origin", inviteForm(token), map[string]string{"Sec-Fetch-Site": "", "Origin": "https://evil.example"}, http.StatusForbidden},
	}
	for _, tt := range tests {
		rec := st.postLogin(tt.form, tt.headers)
		if rec.Code != tt.want {
			t.Errorf("%s: status = %d, want %d; body: %s", tt.name, rec.Code, tt.want, rec.Body)
		}
	}
	if n := st.count("SELECT count(*) FROM oidc_login_states"); n != statesBefore {
		t.Errorf("login states = %d, want %d: a refused request stores nothing", n, statesBefore)
	}
	st.assertNoSecretsInLogs(token)
}

// TestInviteSignInRateLimit: the invite form shares /auth/login's limit.
func TestInviteSignInRateLimit(t *testing.T) {
	t.Parallel()
	st := newSignInStack(t)
	token, _ := secret.New()
	for i := range identity.LoginClientBurst {
		if rec := st.postLogin(inviteForm(token), nil); rec.Code != http.StatusSeeOther {
			t.Fatalf("sign-in %d: status = %d, want 303", i+1, rec.Code)
		}
	}
	rec := st.postLogin(inviteForm(token), nil)
	if rec.Code != http.StatusTooManyRequests || rec.Header().Get("Retry-After") == "" {
		t.Errorf("sign-in %d: status = %d, Retry-After %q; want 429 with Retry-After", identity.LoginClientBurst+1, rec.Code, rec.Header().Get("Retry-After"))
	}
	if n := st.count("SELECT count(*) FROM oidc_login_states"); n != identity.LoginClientBurst {
		t.Errorf("login states = %d, want %d", n, identity.LoginClientBurst)
	}
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

// TestRPCLogLineCarriesTheRequestsFields runs a real session through the whole
// server (identity's interceptor, authz, the handler) and reads the `rpc` line
// back: it must name the request, the user and the campaign, and nothing a
// person typed.
func TestRPCLogLineCarriesTheRequestsFields(t *testing.T) {
	t.Parallel()
	st := newSignInStack(t)
	session := st.signIn()
	userID := st.userID(session)
	api := st.campaigns(session)

	created, err := api.CreateCampaign(t.Context(), connect.NewRequest(&campaignsv1.CreateCampaignRequest{
		Name: "A very secret campaign name", XpMode: campaignsv1.XpMode_XP_MODE_MILESTONES,
	}))
	if err != nil {
		t.Fatalf("CreateCampaign() error = %v", err)
	}
	campaignID := created.Msg.GetCampaign().GetId()
	res, err := api.GetCampaign(t.Context(), connect.NewRequest(&campaignsv1.GetCampaignRequest{CampaignId: campaignID}))
	if err != nil {
		t.Fatalf("GetCampaign() error = %v", err)
	}

	var line map[string]any
	for raw := range strings.SplitSeq(st.logs.String(), "\n") {
		var m map[string]any
		if json.Unmarshal([]byte(raw), &m) == nil && m["message"] == "rpc" && strings.HasSuffix(fmt.Sprint(m["procedure"]), "/GetCampaign") {
			line = m
		}
	}
	if line == nil {
		t.Fatalf("no rpc line for GetCampaign in:\n%s", st.logs)
	}
	if line["user_id"] != userID || line["campaign_id"] != campaignID || line["code"] != "ok" {
		t.Errorf("line = %v, want user %s, campaign %s, code ok", line, userID, campaignID)
	}
	if id, _ := line["request_id"].(string); len(id) != 32 {
		t.Errorf("request_id = %v, want 32 hex chars", line["request_id"])
	}
	// The same id comes back to the caller in X-Request-Id.
	if got := res.Header().Get(httpserver.RequestIDHeader); got != line["request_id"] {
		t.Errorf("X-Request-Id = %q, log request_id = %v", got, line["request_id"])
	}
	if strings.Contains(st.logs.String(), "A very secret campaign name") {
		t.Error("the campaign's name reached the logs")
	}
}
