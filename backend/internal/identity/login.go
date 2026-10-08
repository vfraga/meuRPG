package identity

import (
	"context"
	"crypto/subtle"
	"errors"
	"log/slog"
	"math"
	"mime"
	"net/http"
	"net/url"
	"regexp"
	"strconv"
	"strings"
	"time"

	"golang.org/x/oauth2"

	"github.com/PuraFome/meuRPG/backend/internal/platform/ratelimit"
	"github.com/PuraFome/meuRPG/backend/internal/platform/secret"
	"github.com/PuraFome/meuRPG/backend/internal/platform/slowclient"
)

// loginFormReadTimeout is how long a client has to send POST /auth/login's form.
const loginFormReadTimeout = 10 * time.Second

const (
	// loginCookieName ties a sign-in to the browser that started it: it
	// holds the same random state that goes to the provider. Without it,
	// an attacker could start a sign-in with their own account and trick a
	// victim's browser into finishing it ("login CSRF").
	loginCookieName = "__Host-meurpg_login"

	// loginStateLifetime is how long the user has to finish signing in at
	// the provider.
	loginStateLifetime = 10 * time.Minute

	// maxReturnToLength keeps return_to from bloating the login state row.
	maxReturnToLength = 1024
)

// loginClientBurst is how many requests one client may send at once, and
// loginClientEvery how often it earns another.
const (
	loginClientBurst = 40
	loginClientEvery = 3 * time.Second
)

// LoginClientBurst is loginClientBurst for the tests of the modules that sign in
// through /auth/login (the invite form shares the limit).
const LoginClientBurst = loginClientBurst

// loginRateLimit caps /auth/login (GET and POST together) and
// /auth/callback, because every login hit writes a login state row and every
// callback with a matching cookie deletes one. A sign-in is one of each. The
// limits are per server instance (in memory, see package ratelimit):
//
//   - per client IP: 40 at once (20 sign-ins), then one every 3 seconds.
//     That is plenty for a whole table of players behind one Wi-Fi, and
//     stops one client from filling the table or hammering the database.
//   - overall: 200 at once, then 2 a second (120 a minute), which bounds
//     the rows a botnet can write: at most 7,200 an hour per instance,
//     deleted by the row TTL within the next hour.
var loginRateLimit = ratelimit.Config{
	PerClient:  ratelimit.Rate{Burst: loginClientBurst, Every: loginClientEvery},
	Global:     ratelimit.Rate{Burst: 200, Every: 500 * time.Millisecond},
	MaxClients: 10_000,
}

// loginRequest is what starts a sign-in: where to go back to, and an
// optional intent to complete after it (see IntentHandler).
type loginRequest struct {
	returnTo      string
	intent        string
	intentPayload string
}

// handleLogin starts a plain sign-in: GET /auth/login?return_to=/path.
func (s *Service) handleLogin(w http.ResponseWriter, r *http.Request) {
	setAuthHeaders(w)
	if !s.allowLogin(w, r) {
		return
	}
	query := r.URL.Query()
	// An intent's payload can be a secret (an invite token), and URLs end
	// up in logs, so intents only come in a POST form.
	if query.Has("intent") || query.Has("intent_payload") {
		s.rejectLogin(w, r, badLogin("intent_in_url", nil), "send intent and intent_payload in a POST form, never in the URL")
		return
	}
	s.startLogin(w, r, loginRequest{returnTo: query.Get("return_to")})
}

// handleLoginForm starts a sign-in from a form: POST /auth/login with an
// application/x-www-form-urlencoded body holding return_to and, optionally,
// intent and intent_payload. It answers like GET /auth/login.
//
// It is a POST that the app sends from its own page, so
// http.CrossOriginProtection (around the whole mux) refuses it from another
// site: nobody can make a victim's browser start a sign-in that joins a
// campaign of the attacker's choosing.
func (s *Service) handleLoginForm(w http.ResponseWriter, r *http.Request) {
	setAuthHeaders(w)
	if !s.allowLogin(w, r) {
		return
	}
	req, le, message := parseLoginForm(w, r, loginFormReadTimeout)
	if le != nil {
		s.rejectLogin(w, r, le, message)
		return
	}
	s.startLogin(w, r, req)
}

// parseLoginForm reads POST /auth/login's form. On failure it returns the
// error to log and the message for the browser.
func parseLoginForm(w http.ResponseWriter, r *http.Request, readTimeout time.Duration) (loginRequest, *loginError, string) {
	// Everything goes in the body. A query string could carry the payload
	// into the logs, so it is refused rather than ignored.
	if r.URL.RawQuery != "" {
		return loginRequest{}, badLogin("query_on_post", nil), "send the fields in the form body, not in the URL"
	}
	mediaType, _, err := mime.ParseMediaType(r.Header.Get("Content-Type"))
	if err != nil || mediaType != "application/x-www-form-urlencoded" {
		le := &loginError{status: http.StatusUnsupportedMediaType, reason: "not_a_form"}
		return loginRequest{}, le, "send an application/x-www-form-urlencoded form"
	}
	slowclient.ReadBody(w, readTimeout) // a form is a few hundred bytes; no reason to wait on a trickle
	r.Body = http.MaxBytesReader(w, r.Body, maxLoginFormBytes)
	if err := r.ParseForm(); err != nil {
		if _, ok := errors.AsType[*http.MaxBytesError](err); ok {
			le := &loginError{status: http.StatusRequestEntityTooLarge, reason: "form_too_large"}
			return loginRequest{}, le, "the form is too large"
		}
		return loginRequest{}, badLogin("malformed_form", nil), "the form could not be read"
	}
	form := r.PostForm
	for _, field := range []string{"return_to", "intent", "intent_payload"} {
		if len(form[field]) > 1 {
			return loginRequest{}, badLogin("repeated_field", nil), "each field may appear only once"
		}
	}
	req := loginRequest{
		returnTo:      form.Get("return_to"),
		intent:        form.Get("intent"),
		intentPayload: form.Get("intent_payload"),
	}
	if req.intent == "" && req.intentPayload != "" {
		return loginRequest{}, badLogin("payload_without_intent", nil), "intent_payload needs an intent"
	}
	return req, nil, ""
}

// allowLogin applies the sign-in rate limit. It comes before anything else,
// so a refused request costs nothing. The client IP is not logged
// (docs/privacy.md); the request log line already shows the 429.
func (s *Service) allowLogin(w http.ResponseWriter, r *http.Request) bool {
	ok, wait := s.loginLimiter.Allow(ratelimit.ClientKey(r, s.behindCloudRun))
	if !ok {
		// Retry-After is in whole seconds (RFC 9110, section 10.2.3).
		w.Header().Set("Retry-After", strconv.Itoa(max(1, int(math.Ceil(wait.Seconds())))))
		http.Error(w, "too many sign-in attempts, please wait a moment and try again", http.StatusTooManyRequests)
	}
	return ok
}

// rejectLogin refuses to start a sign-in: it logs the reason (never a value
// from the request) and answers with the error's status and message.
func (s *Service) rejectLogin(w http.ResponseWriter, r *http.Request, le *loginError, message string) {
	attrs := []any{"reason", le.reason}
	if le.err != nil {
		attrs = append(attrs, "error", le.err)
	}
	level := slog.LevelWarn
	if le.status >= http.StatusInternalServerError {
		level = slog.LevelError
	}
	s.logger.Log(r.Context(), level, "sign-in rejected", attrs...)
	http.Error(w, message, le.status)
}

// startLogin stores a login state (PKCE verifier, nonce, return_to and the
// prepared intent, if any) under the hash of a random state, puts the state
// in a short-lived cookie, and redirects the browser to the provider.
func (s *Service) startLogin(w http.ResponseWriter, r *http.Request, req loginRequest) {
	ctx := r.Context()

	returnTo, ok := safeReturnTo(req.returnTo)
	if !ok {
		s.rejectLogin(w, r, badLogin("unsafe_return_to", nil), "return_to must be a path on this site, like /campaigns")
		return
	}

	var intentData []byte
	if req.intent != "" {
		var le *loginError
		if intentData, le = s.prepareIntent(req.intent, req.intentPayload); le != nil {
			message := "this sign-in request is not valid"
			if le.status >= http.StatusInternalServerError {
				message = "sign-in is temporarily unavailable, please try again later"
			}
			s.rejectLogin(w, r, le, message)
			return
		}
	}

	p, err := s.providers.get(ctx)
	if err != nil {
		s.logger.WarnContext(ctx, "sign-in unavailable: OIDC discovery failed", "error", err)
		http.Error(w, "sign-in is temporarily unavailable, please try again later", http.StatusServiceUnavailable)
		return
	}

	state, stateHash := secret.New()
	nonce, _ := secret.New()
	verifier := oauth2.GenerateVerifier()

	now := s.now()
	err = s.store.saveLoginState(ctx, LoginState{
		StateHash:    stateHash,
		CodeVerifier: verifier,
		Nonce:        nonce,
		ReturnTo:     returnTo,
		CreatedAt:    now,
		ExpiresAt:    now.Add(loginStateLifetime),
		IntentKind:   req.intent,
		IntentData:   intentData,
	})
	if err != nil {
		s.logger.ErrorContext(ctx, "sign-in unavailable: cannot save the login state", "error", err)
		http.Error(w, "sign-in is temporarily unavailable, please try again later", http.StatusServiceUnavailable)
		return
	}

	http.SetCookie(w, &http.Cookie{
		Name:     loginCookieName,
		Value:    state,
		Path:     "/",
		MaxAge:   int(loginStateLifetime.Seconds()),
		Secure:   true,
		HttpOnly: true,
		// Lax is what lets the cookie come back on the provider's
		// top-level redirect to /auth/callback. Strict would drop it.
		SameSite: http.SameSiteLaxMode,
	})
	// 302 for a link (GET); 303 for the form (POST), which tells the
	// browser to follow with a GET, as it would for a 302 anyway.
	status := http.StatusFound
	if r.Method == http.MethodPost {
		status = http.StatusSeeOther
	}
	http.Redirect(w, r, p.authCodeURL(state, nonce, verifier), status)
}

// loginError is a sign-in that failed, at its start or in the callback: the
// HTTP status for the browser, and a short reason for the logs. Neither
// ever includes a token, code, cookie, state, e-mail or intent payload.
type loginError struct {
	status int
	reason string
	err    error
}

func (e *loginError) Error() string {
	if e.err == nil {
		return e.reason
	}
	return e.reason + ": " + e.err.Error()
}

func (e *loginError) Unwrap() error { return e.err }

// badLogin is a failure caused by the request or the provider's answer.
func badLogin(reason string, err error) *loginError {
	return &loginError{status: http.StatusBadRequest, reason: reason, err: err}
}

// unavailable is a failure on our side (database, provider unreachable).
func unavailable(reason string, err error) *loginError {
	return &loginError{status: http.StatusServiceUnavailable, reason: reason, err: err}
}

// handleCallback finishes a sign-in: GET /auth/callback?code=...&state=...
//
// It is a GET that creates a session, so http.CrossOriginProtection lets it
// through; state (checked against the cookie and the database), PKCE and
// nonce are what protect it.
func (s *Service) handleCallback(w http.ResponseWriter, r *http.Request) {
	setAuthHeaders(w)
	// Before the state check, so a refused request costs nothing: a made-up
	// state with a matching cookie would otherwise reach the database.
	if !s.allowLogin(w, r) {
		return
	}
	// The login cookie is single use, whatever happens next.
	http.SetCookie(w, expiredCookie(loginCookieName))

	login, token, session, err := s.completeLogin(r)
	if err != nil {
		le, ok := errors.AsType[*loginError](err)
		if !ok {
			le = unavailable("internal", err)
		}
		attrs := []any{"reason", le.reason}
		if le.err != nil {
			attrs = append(attrs, "error", le.err)
		}
		level := slog.LevelWarn
		if le.status >= http.StatusInternalServerError {
			level = slog.LevelError
		}
		s.logger.Log(r.Context(), level, "sign-in failed", attrs...)

		message := "sign-in failed, please try again"
		if le.status >= http.StatusInternalServerError {
			message = "sign-in is temporarily unavailable, please try again later"
		}
		http.Error(w, message, le.status)
		return
	}

	http.SetCookie(w, sessionCookie(token, session.CreatedAt, session.ExpiresAt))
	s.logger.InfoContext(r.Context(), "sign-in succeeded")

	// The session exists now, so an intent (accepting an invite, say) can
	// run as the signed-in user. Its outcome only changes where the browser
	// goes next; the sign-in stands either way.
	target := s.afterSignIn(r.Context(), session, login)

	// 303: the browser follows with a GET, whatever brought it here.
	http.Redirect(w, r, target, http.StatusSeeOther) //nolint:gosec // G710: target is a path on this site (safeReturnTo)
}

// completeLogin runs every check of the callback, in order, and starts the
// session. It returns the login state (where to go back to, and the intent)
// and the new session.
func (s *Service) completeLogin(r *http.Request) (login LoginState, token string, session Session, err error) {
	ctx := r.Context()
	query := r.URL.Query()

	// 1. The state in the URL must be the one in this browser's cookie.
	// This check comes before any database access, so a stranger who
	// learned a state (the URL goes to the platform's request logs) cannot
	// burn it for the real user.
	state := query.Get("state")
	cookieState, _ := cookieValue(r.Header, loginCookieName)
	if state == "" || cookieState == "" {
		return LoginState{}, "", Session{}, badLogin("missing_state", nil)
	}
	if subtle.ConstantTimeCompare([]byte(state), []byte(cookieState)) != 1 {
		return LoginState{}, "", Session{}, badLogin("state_mismatch", nil)
	}
	stateHash, ok := secret.Hash(state)
	if !ok {
		return LoginState{}, "", Session{}, badLogin("malformed_state", nil)
	}

	// 2. The state must exist and be fresh. Taking it deletes it, so a
	// callback URL works once.
	now := s.now()
	login, err = s.store.takeLoginState(ctx, stateHash, now)
	if errors.Is(err, ErrNotFound) {
		return LoginState{}, "", Session{}, badLogin("unknown_or_expired_state", nil)
	}
	if err != nil {
		return LoginState{}, "", Session{}, unavailable("store_error", err)
	}

	// 3. The provider may report an error instead of a code, e.g. when the
	// user cancels. Its error_description is not logged or shown: it is
	// free text from the query string.
	if providerErr := query.Get("error"); providerErr != "" {
		return LoginState{}, "", Session{}, badLogin("provider_error", errors.New(oauthErrorCode(providerErr)))
	}
	code := query.Get("code")
	if code == "" {
		return LoginState{}, "", Session{}, badLogin("missing_code", nil)
	}

	p, err := s.providers.get(ctx)
	if err != nil {
		return LoginState{}, "", Session{}, unavailable("discovery_failed", err)
	}

	// 4. Trade the code for tokens, proving with the PKCE verifier that we
	// started this flow.
	rawIDToken, err := p.exchange(ctx, code, login.CodeVerifier)
	if err != nil {
		if re, ok := errors.AsType[*oauth2.RetrieveError](err); ok {
			// Log only the OAuth error code, not the response body.
			return LoginState{}, "", Session{}, badLogin("token_exchange_rejected", errors.New(oauthErrorCode(re.ErrorCode)))
		}
		return LoginState{}, "", Session{}, &loginError{status: http.StatusBadGateway, reason: "token_exchange_failed", err: err}
	}

	// 5. Verify the ID token: signature, iss, aud, exp, nonce, azp, sub.
	id, err := p.verify(ctx, rawIDToken, login.Nonce)
	if err != nil {
		return LoginState{}, "", Session{}, badLogin("invalid_id_token", err)
	}

	userID, err := s.store.UpsertUser(ctx, ExternalIdentity{Issuer: id.Issuer, Subject: id.Subject, Email: id.Email})
	if err != nil {
		return LoginState{}, "", Session{}, unavailable("store_error", err)
	}

	// The 30 days count from now, whatever auth_time says: it is recorded,
	// not trusted (see verifiedIdentity).
	token, session, err = s.startSession(ctx, userID, now, id.AuthTime)
	if err != nil {
		return LoginState{}, "", Session{}, unavailable("store_error", err)
	}
	// A browser that signs in again gets a new session (never the old
	// token, which rules out session fixation), and the old one is revoked
	// instead of lingering until it expires. It goes after the new session
	// exists: if creating that fails, the user keeps the session they had.
	if old, ok := cookieValue(r.Header, SessionCookieName); ok {
		s.revokeQuietly(ctx, old)
	}
	return login, token, session, nil
}

// revokeQuietly revokes the session behind a cookie value, if there is one.
// Failing here must not fail the new sign-in, so errors are only logged.
func (s *Service) revokeQuietly(ctx context.Context, token string) {
	old, err := s.lookupSession(ctx, token)
	if err != nil {
		if !errors.Is(err, errNoSession) {
			s.logger.WarnContext(ctx, "cannot look up the previous session", "error", err)
		}
		return
	}
	if err := s.store.revokeSession(ctx, old.ID); err != nil {
		s.logger.WarnContext(ctx, "cannot revoke the previous session", "error", err)
	}
}

// oauthErrorPattern matches OAuth error codes (RFC 6749, section 4.1.2.1),
// which are safe to log. Anything else is replaced.
var oauthErrorPattern = regexp.MustCompile(`^[a-z_]{1,64}$`)

func oauthErrorCode(code string) string {
	if oauthErrorPattern.MatchString(code) {
		return code
	}
	return "unrecognized_error_code"
}

// safeReturnTo accepts only a path on this site, so /auth/login cannot be
// used as an open redirect to another site. Empty means the home page.
//
// Browsers are lenient with URLs, so the checks are strict: "//host" and
// "/\host" are other sites to a browser, and tabs or newlines are dropped
// before parsing ("/\t/host" becomes "//host").
//
// A fragment (#...) is dropped: this app keeps secrets there, such as an
// invite token (/invite#t=...), and the login state must not store one.
func safeReturnTo(raw string) (string, bool) {
	if raw == "" {
		return "/", true
	}
	if i := strings.IndexByte(raw, '#'); i >= 0 {
		raw = raw[:i]
	}
	if raw == "" || len(raw) > maxReturnToLength || raw[0] != '/' || strings.HasPrefix(raw, "//") {
		return "", false
	}
	for _, c := range raw {
		if c == '\\' || c < 0x20 || c == 0x7f {
			return "", false
		}
	}
	u, err := url.Parse(raw)
	if err != nil || u.Scheme != "" || u.Host != "" || u.User != nil || u.Opaque != "" {
		return "", false
	}
	return raw, true
}

// setAuthHeaders sets the headers every /auth/* response needs. The
// callback URL carries the authorization code and state in its query, and
// no-referrer keeps them out of the Referer header of whatever comes next.
func setAuthHeaders(w http.ResponseWriter) {
	setNoStore(w)
	w.Header().Set("Referrer-Policy", "no-referrer")
}
