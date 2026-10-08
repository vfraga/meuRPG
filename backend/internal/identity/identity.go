// Package identity signs users in and keeps them signed in.
//
// The game master signs in with an OpenID Connect provider: Google in
// production, a local provider or an in-test fake in development. Nothing in
// this package is provider-specific; the provider comes from configuration
// and is found through OpenID Connect Discovery.
//
// The flow is the Authorization Code flow with PKCE (S256), as a
// confidential client, with state and nonce (ADR-0002):
//
//	GET /auth/login?return_to=/path  saves a login state and redirects to the provider
//	POST /auth/login                 the same, with a sign-in intent in the form (intent.go)
//	GET /auth/callback               checks everything, then starts a session
//
// A session is opaque: 32 random bytes in the __Host-meurpg_session cookie,
// stored in the database only as a SHA-256 hash, valid for at most 30 days
// and never extended, and ended early when unused for 14 days (the idle
// timeout; its last use is written at most every 10 minutes). The Connect
// service IdentityService answers GetMe, UpdateProfile, SignOut,
// SignOutOtherSessions and CountOtherSessions. Other modules add Interceptor to their Connect
// handlers and learn who is calling from UserID, through the authz.Caller
// interface; nothing outside Interceptor can put a session into a context.
package identity

import (
	"context"
	"errors"
	"log/slog"
	"net/http"
	"slices"
	"time"

	"connectrpc.com/connect"

	"github.com/PuraFome/meuRPG/backend/gen/meurpg/identity/v1/identityv1connect"
	"github.com/PuraFome/meuRPG/backend/internal/platform/config"
	"github.com/PuraFome/meuRPG/backend/internal/platform/ratelimit"
)

// discoveryTimeout bounds the first discovery attempt at startup, so an
// unreachable provider delays the boot by seconds, not minutes.
const discoveryTimeout = 10 * time.Second

// Config holds what the identity service needs from the outside world.
type Config struct {
	// OIDC is the provider configuration. It must be configured.
	OIDC config.OIDC

	// Store persists users, sessions and login states.
	Store Store

	// Logger receives warnings about failed sign-ins, without personal data
	// or secrets. Nil means slog.Default().
	Logger *slog.Logger

	// Now returns the current time. Nil means time.Now. Tests replace it to
	// move the clock forward, e.g. past a session's 30 days.
	Now func() time.Time

	// HTTPClient talks to the provider. Nil means a client that trusts the
	// system roots plus OIDC.CAFile, with a timeout.
	HTTPClient *http.Client

	// BehindCloudRun says every request reaches the server through Cloud
	// Run's front end, so the sign-in rate limit reads the client IP from
	// X-Forwarded-For (see ratelimit.ClientKey). Never set it where clients
	// connect directly: they could then claim any IP they like.
	BehindCloudRun bool

	// SessionIdleTimeout is how long a session may go unused before it
	// stops working (SESSION_IDLE_TIMEOUT). Zero means
	// DefaultSessionIdleTimeout. It cannot lengthen the 30 days.
	SessionIdleTimeout time.Duration

	// Intents are the sign-in intents that POST /auth/login accepts, by
	// kind (e.g. "campaign_invite"): what the user may ask to finish right
	// after signing in. Kinds are lowercase letters, digits and
	// underscores, at most 32 characters. Nil means none.
	Intents map[string]IntentHandler
}

// Service implements sign-in (the HTTP handlers), sessions and the
// IdentityService Connect API.
type Service struct {
	store     Store
	logger    *slog.Logger
	now       func() time.Time
	providers *providerSource

	// loginLimiter caps /auth/login (GET and POST share it), which writes a
	// login state row on every hit (see loginRateLimit).
	loginLimiter   *ratelimit.Limiter
	behindCloudRun bool

	// intents are the sign-in intents, by kind (see IntentHandler).
	intents map[string]IntentHandler

	// idleTimeout is how long a session may go unused (see Config).
	idleTimeout time.Duration
}

// The compiler checks that Service implements the generated interface.
var _ identityv1connect.IdentityServiceHandler = (*Service)(nil)

// New returns a Service. It tries OpenID Connect Discovery once; if the
// provider is unreachable, New still succeeds and discovery is retried on
// the next sign-in, so a provider outage at boot does not take the whole API
// down. Invalid configuration (a missing or unreadable CA file, for example)
// is an error.
func New(ctx context.Context, cfg Config) (*Service, error) {
	if !cfg.OIDC.Configured() {
		return nil, errors.New("identity: OIDC is not configured")
	}
	if cfg.Store == nil {
		return nil, errors.New("identity: a Store is required")
	}
	intents, err := checkIntents(cfg.Intents)
	if err != nil {
		return nil, err
	}

	s := &Service{
		store:   cfg.Store,
		logger:  cfg.Logger,
		now:     cfg.Now,
		intents: intents,
	}
	if s.logger == nil {
		s.logger = slog.Default()
	}
	if s.now == nil {
		s.now = time.Now
	}
	s.idleTimeout = cfg.SessionIdleTimeout
	if s.idleTimeout <= 0 {
		s.idleTimeout = DefaultSessionIdleTimeout
	}
	limits := loginRateLimit
	limits.Now = s.now
	s.loginLimiter = ratelimit.New(limits)
	s.behindCloudRun = cfg.BehindCloudRun

	client := cfg.HTTPClient
	if client == nil {
		if client, err = newHTTPClient(cfg.OIDC.CAFile); err != nil {
			return nil, err
		}
	}
	s.providers = &providerSource{cfg: cfg.OIDC, client: client, now: s.now}

	discoverCtx, cancel := context.WithTimeout(ctx, discoveryTimeout)
	defer cancel()
	if _, err := s.providers.get(discoverCtx); err != nil {
		s.logger.WarnContext(ctx, "OIDC discovery failed; retrying on the next sign-in", "issuer", cfg.OIDC.IssuerURL, "error", err)
	}
	return s, nil
}

// Mount registers the sign-in routes and the IdentityService on a mux.
// handle is usually httpserver.Server.Handle or http.ServeMux.Handle; opts
// are the Connect options shared by every service (size limits, the
// Connect-Protocol-Version requirement). Mount adds Interceptor to them.
func (s *Service) Mount(handle func(pattern string, handler http.Handler), opts ...connect.HandlerOption) {
	s.MountLimited(handle, nil, opts...)
}

// MountLimited is Mount with an interceptor that runs right after the
// session interceptor, which is where the per-user rate limit goes: it needs
// the user the session interceptor finds. A nil after adds nothing.
func (s *Service) MountLimited(handle func(pattern string, handler http.Handler), after connect.Interceptor, opts ...connect.HandlerOption) {
	handle("GET /auth/login", http.HandlerFunc(s.handleLogin))
	handle("POST /auth/login", http.HandlerFunc(s.handleLoginForm))
	handle("GET "+config.CallbackPath, http.HandlerFunc(s.handleCallback))

	interceptor := s.Interceptor()
	if after != nil {
		interceptor = ratelimit.Chain(interceptor, after)
	}
	// Clip so append copies instead of writing into the caller's array.
	opts = append(slices.Clip(opts), connect.WithInterceptors(interceptor))
	handle(identityv1connect.NewIdentityServiceHandler(s, opts...))
}

// MountDisabled registers the same routes as Mount for a server where
// sign-in is not configured (no OIDC provider or no database). They answer
// 503 (HTTP) or unavailable (Connect) with a clear message, instead of a 404
// that would look like a wrong URL.
func MountDisabled(handle func(pattern string, handler http.Handler), opts ...connect.HandlerOption) {
	disabled := http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		setNoStore(w)
		http.Error(w, errDisabled.Error(), http.StatusServiceUnavailable)
	})
	handle("GET /auth/login", disabled)
	handle("POST /auth/login", disabled)
	handle("GET "+config.CallbackPath, disabled)
	handle(identityv1connect.NewIdentityServiceHandler(disabledService{}, opts...))
}

var errDisabled = errors.New("sign-in is not configured on this server")
