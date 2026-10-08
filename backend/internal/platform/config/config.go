// Package config reads the process configuration from environment variables.
//
// Following the twelve-factor app style, everything that changes between
// environments (local, CI, Cloud Run) comes from the environment, and the
// defaults are chosen so that `go run ./cmd/api` works with no setup at all.
// The package uses the standard library, plus pgx to read DATABASE_URL's TLS
// settings (checkDatabaseTLS), on purpose: configuration is
// small enough that a framework would add more concepts than it removes.
package config

import (
	"crypto/tls"
	"errors"
	"fmt"
	"log/slog"
	"math"
	"net"
	"net/url"
	"strconv"
	"strings"
	"time"

	"github.com/jackc/pgx/v5/pgconn"
)

// Defaults used when a variable is unset or empty.
const (
	DefaultPort     = 8080
	DefaultLogLevel = slog.LevelInfo
	// DefaultWebDir is where backend/Dockerfile's Node build stage copies
	// the Angular production build.
	DefaultWebDir = "/app/web"
)

// Config is the validated configuration shared by the binaries in cmd/.
type Config struct {
	// Port is the TCP port the HTTP server listens on. Cloud Run injects it
	// through $PORT (usually 8080).
	Port int

	// ListenHost is the interface the HTTP server listens on, from
	// $LISTEN_HOST. Empty means every interface, which is what Cloud Run
	// needs; the local stacks set 127.0.0.1 so nothing on the LAN reaches a
	// development server. (Not $HOST: zsh sets HOST to the machine's name.)
	ListenHost string

	// DatabaseURL is a PostgreSQL connection string for CockroachDB, e.g.
	// postgresql://user:pass@host:26257/meurpg?sslmode=verify-full.
	// Empty means "run without a database": the API still starts, and
	// /readyz reports the database as disabled. It holds the password, so it is
	// a Secret: a log line or an error that formats the whole Config shows
	// [REDACTED]. On Cloud Run, Load refuses a URL that does not verify the
	// server's certificate (sslmode=verify-full or verify-ca).
	DatabaseURL Secret

	// LogLevel is the minimum level written to the logs.
	LogLevel slog.Level

	// WebDir is the directory holding the Angular production build (see
	// internal/platform/httpserver.NewStatic). When nothing exists there,
	// the server logs it and runs API-only instead of failing to start:
	// that is the normal case outside the Docker image, e.g. `make run`.
	WebDir string

	// OIDC configures sign-in. The zero value means "not configured": the
	// API still starts, and the sign-in routes answer 503.
	OIDC OIDC

	// BlobDir is the directory where uploaded images are stored (package
	// internal/platform/blob), such as a Docker volume in the local stack.
	// Empty means "images are off": the API still starts, and the image
	// routes answer 503. Production will store images in Cloud Storage
	// instead (docs/operations.md).
	BlobDir string

	// CloudRun is true when the process runs on Cloud Run, which sets
	// K_SERVICE in every service container (see "Container runtime
	// contract" in the Cloud Run docs). There, every request reaches the
	// container through Google's front end, so the client IP comes from
	// X-Forwarded-For instead of the connection (see
	// internal/platform/ratelimit.ClientKey).
	CloudRun bool

	// TraceProject is the Google Cloud project ID (GOOGLE_CLOUD_PROJECT).
	// It is read only on Cloud Run, where it lets each log line carry the
	// request's Cloud Trace id; elsewhere it stays empty.
	TraceProject string

	// Images configures the image generator (MR-039, RN-28, ADR-0019).
	Images Images

	// SessionIdleTimeout is how long a sign-in session may go unused before
	// it stops working (SESSION_IDLE_TIMEOUT, ASVS 5.0 V7.3.1). Zero means
	// the identity module's default (14 days). It never lengthens the
	// 30-day absolute lifetime.
	SessionIdleTimeout time.Duration
	// Limits are the abuse limits: request rates and what one account may create.
	Limits Limits
}

// Bounds of the abuse limits' variables: a typo must not lift a cap.
const (
	maxCampaignsPerUserLimit = 1000
	maxImageDailyLimit       = 10000
	maxRateMultiplier        = 1000
)

// Limits holds the abuse limits' settings (docs/operations.md#abuse-limits).
type Limits struct {
	// RateMultiplier (RATE_LIMIT_MULTIPLIER) scales every request-rate limit
	// (platform/ratelimit.NewPolicy); zero means 1. Raise it only where many
	// accounts share one address, such as the e2e suite.
	RateMultiplier float64
	// MaxCampaignsPerUser (MAX_CAMPAIGNS_PER_USER) is how many campaigns one
	// account may be master of (RN-30); zero means campaigns.DefaultMaxCampaignsPerUser.
	MaxCampaignsPerUser int
	// CampaignCreators (CAMPAIGN_CREATORS) are the verified e-mails, in lower
	// case, that may create campaigns (RN-30). Empty means anyone.
	CampaignCreators []string
}

// maxImageMonthlyLimit bounds IMAGE_MONTHLY_LIMIT: a typo must not lift the cap.
const maxImageMonthlyLimit = 500

// Images holds the generated images' settings.
type Images struct {
	// GeminiAPIKey is the Gemini API key (GEMINI_API_KEY), a secret. Empty
	// means generation is off: the RPCs answer with a typed "off" reason.
	GeminiAPIKey Secret
	// Model is the image model (GEMINI_IMAGE_MODEL); empty means the generator's default.
	Model string
	// Fake (IMAGE_GENERATOR=fake) uses the deterministic fake generator, for
	// local runs and CI, and turns generation on without a key. It is refused
	// on Cloud Run.
	Fake bool
	// MonthlyLimit is the images per campaign per month (IMAGE_MONTHLY_LIMIT);
	// zero means the maps module's default (maps.DefaultMonthlyImages).
	MonthlyLimit int
	// DailyLimit is the images the whole server may generate per day
	// (IMAGE_DAILY_LIMIT), on top of the campaigns' monthly limit: a ceiling
	// on the Gemini bill. Zero means maps.DefaultDailyImages.
	DailyLimit int
}

// OIDC holds the settings of the OpenID Connect provider the game master
// signs in with: Google in production, a local provider in development.
// Nothing here is provider-specific.
type OIDC struct {
	// IssuerURL identifies the provider, e.g. https://accounts.google.com.
	// The server reads <IssuerURL>/.well-known/openid-configuration to find
	// the provider's endpoints and signing keys.
	IssuerURL string

	// ClientID and ClientSecret are this app's credentials at the provider.
	ClientID     string
	ClientSecret Secret

	// RedirectURL is where the provider sends the browser back after
	// sign-in: this server's public origin plus /auth/callback. It must be
	// registered at the provider exactly as written here.
	RedirectURL string

	// CAFile is an optional PEM file with extra CA certificates to trust
	// when talking to the provider, for a local provider with a self-signed
	// certificate. The system roots are always trusted too.
	CAFile string

	// MaxAge, when positive, is sent as the max_age sign-in parameter: if
	// the user authenticated at the provider longer ago than this, the
	// provider must ask for their credentials again. Zero means max_age is
	// not sent. Set it only for providers that document max_age (Google
	// does not).
	MaxAge time.Duration
}

// Configured reports whether sign-in is configured. Load guarantees that
// either all required OIDC variables are set or none is.
func (o OIDC) Configured() bool {
	return o.IssuerURL != ""
}

// CallbackPath is the path the provider must redirect back to. It is fixed
// because the server registers its callback handler there.
const CallbackPath = "/auth/callback"

// maxOIDCMaxAge caps OIDC_MAX_AGE at the session lifetime: a larger max_age
// would let a new 30-day session rest on an even older authentication.
const maxOIDCMaxAge = 30 * 24 * time.Hour

// Secret is a string that never prints itself: fmt and slog show
// "[REDACTED]" instead, so a secret cannot leak into a log line or an error
// message by accident. Use Reveal to get the real value.
type Secret string

// Reveal returns the secret itself. Call it only where the value is sent
// to its destination, never to log or format it.
func (s Secret) Reveal() string {
	return string(s)
}

// String implements fmt.Stringer.
func (s Secret) String() string {
	if s == "" {
		return ""
	}
	return "[REDACTED]"
}

// GoString implements fmt.GoStringer, which %#v uses.
func (s Secret) GoString() string {
	return fmt.Sprintf("%q", s.String())
}

// LogValue implements slog.LogValuer.
func (s Secret) LogValue() slog.Value {
	return slog.StringValue(s.String())
}

// MarshalText implements encoding.TextMarshaler, which encoding/json uses.
// It matters when a struct holding a Secret is logged as a whole: slog's
// JSON handler encodes it with encoding/json, not with String.
func (s Secret) MarshalText() ([]byte, error) {
	return []byte(s.String()), nil
}

// Load builds a Config from getenv, which is usually os.Getenv. Taking the
// lookup function as a parameter keeps Load free of global state, so tests
// can pass a fake environment instead of mutating the real one.
//
// All problems are reported together, so a misconfigured deploy shows every
// mistake in a single log line instead of one per restart.
func Load(getenv func(string) string) (Config, error) {
	cfg := Config{
		Port:        DefaultPort,
		DatabaseURL: Secret(strings.TrimSpace(getenv("DATABASE_URL"))),
		LogLevel:    DefaultLogLevel,
		WebDir:      DefaultWebDir,
		CloudRun:    strings.TrimSpace(getenv("K_SERVICE")) != "",
	}

	if cfg.CloudRun {
		cfg.TraceProject = strings.TrimSpace(getenv("GOOGLE_CLOUD_PROJECT"))
	}

	var errs []error

	if raw := strings.TrimSpace(getenv("PORT")); raw != "" {
		port, err := strconv.Atoi(raw)
		switch {
		case err != nil:
			errs = append(errs, fmt.Errorf("PORT must be a number, got %q", raw))
		case port < 1 || port > 65535:
			errs = append(errs, fmt.Errorf("PORT must be between 1 and 65535, got %d", port))
		default:
			cfg.Port = port
		}
	}

	cfg.ListenHost = strings.TrimSpace(getenv("LISTEN_HOST"))

	if raw := strings.TrimSpace(getenv("WEB_DIR")); raw != "" {
		cfg.WebDir = raw
	}

	cfg.BlobDir = strings.TrimSpace(getenv("BLOB_DIR"))

	images, imageErrs := loadImages(getenv, cfg.CloudRun)
	cfg.Images = images
	errs = append(errs, imageErrs...)

	if raw := strings.TrimSpace(getenv("LOG_LEVEL")); raw != "" {
		level, err := parseLogLevel(raw)
		if err != nil {
			errs = append(errs, err)
		} else {
			cfg.LogLevel = level
		}
	}

	if cfg.CloudRun && cfg.DatabaseURL != "" {
		if err := checkDatabaseTLS(cfg.DatabaseURL.Reveal()); err != nil {
			errs = append(errs, err)
		}
	}

	limits, limitErrs := loadLimits(getenv)
	cfg.Limits = limits
	errs = append(errs, limitErrs...)
	if cfg.CloudRun && limits.MaxCampaignsPerUser == CampaignCapOff {
		errs = append(errs, errors.New("MAX_CAMPAIGNS_PER_USER=off is for the local and CI stacks only, never on Cloud Run"))
	}

	if raw := strings.TrimSpace(getenv("SESSION_IDLE_TIMEOUT")); raw != "" {
		d, err := time.ParseDuration(raw)
		switch {
		case err != nil:
			errs = append(errs, fmt.Errorf("SESSION_IDLE_TIMEOUT must be a duration like 336h, got %q", raw))
		case d < time.Hour || d > maxOIDCMaxAge:
			errs = append(errs, fmt.Errorf("SESSION_IDLE_TIMEOUT must be between 1h and 720h, got %q", raw))
		default:
			cfg.SessionIdleTimeout = d
		}
	}

	oidc, oidcErrs := loadOIDC(getenv)
	cfg.OIDC = oidc
	errs = append(errs, oidcErrs...)

	if err := errors.Join(errs...); err != nil {
		return Config{}, fmt.Errorf("invalid configuration: %w", err)
	}
	return cfg, nil
}

// checkDatabaseTLS refuses, on Cloud Run, a DATABASE_URL whose connection could
// go in plain text or could talk to an impostor: the traffic to the database
// carries every password hash and every player's data. pgx's default
// (sslmode=prefer) silently falls back to plain text, and `require` encrypts
// without checking who is on the other end, so only verify-full (the host name
// is checked too) and verify-ca are accepted. The URL is parsed by pgx, so the
// PG* environment variables count as well; the error never echoes the URL, which
// holds the password.
func checkDatabaseTLS(databaseURL string) error {
	pc, err := pgconn.ParseConfig(databaseURL)
	if err != nil {
		return errors.New("DATABASE_URL is not a valid PostgreSQL connection string")
	}
	verified := func(c *tls.Config) bool {
		if c == nil {
			return false
		}
		// verify-full: pgx leaves InsecureSkipVerify off and sets the ServerName.
		// verify-ca: pgx skips the stock check and installs its own (VerifyPeerCertificate).
		return !c.InsecureSkipVerify || c.VerifyPeerCertificate != nil
	}
	ok := verified(pc.TLSConfig)
	for _, fb := range pc.Fallbacks {
		ok = ok && verified(fb.TLSConfig) // a fallback without TLS is how `prefer` and `allow` work
	}
	if !ok {
		return errors.New("DATABASE_URL must use sslmode=verify-full (or verify-ca) on Cloud Run: the connection to the database must be encrypted and the server's certificate checked")
	}
	return nil
}

// loadOIDC reads the OIDC_* variables. They go together: either all the
// required ones are set, or none is and sign-in stays off.
func loadOIDC(getenv func(string) string) (OIDC, []error) {
	o := OIDC{
		IssuerURL:    strings.TrimSpace(getenv("OIDC_ISSUER")),
		ClientID:     strings.TrimSpace(getenv("OIDC_CLIENT_ID")),
		ClientSecret: Secret(strings.TrimSpace(getenv("OIDC_CLIENT_SECRET"))),
		RedirectURL:  strings.TrimSpace(getenv("OIDC_REDIRECT_URL")),
		CAFile:       strings.TrimSpace(getenv("OIDC_CA_FILE")),
	}
	rawMaxAge := strings.TrimSpace(getenv("OIDC_MAX_AGE"))
	if o == (OIDC{}) && rawMaxAge == "" {
		return OIDC{}, nil
	}

	var errs []error
	if rawMaxAge != "" {
		maxAge, err := time.ParseDuration(rawMaxAge)
		switch {
		case err != nil:
			errs = append(errs, fmt.Errorf("OIDC_MAX_AGE must be a duration like 1h or 90m, got %q", rawMaxAge))
		case maxAge < time.Second || maxAge > maxOIDCMaxAge:
			errs = append(errs, fmt.Errorf("OIDC_MAX_AGE must be between 1s and 720h, got %q", rawMaxAge))
		default:
			// max_age is a whole number of seconds.
			o.MaxAge = maxAge.Truncate(time.Second)
		}
	}

	required := []struct {
		name  string
		value string
	}{
		{"OIDC_ISSUER", o.IssuerURL},
		{"OIDC_CLIENT_ID", o.ClientID},
		{"OIDC_CLIENT_SECRET", o.ClientSecret.Reveal()},
		{"OIDC_REDIRECT_URL", o.RedirectURL},
	}
	var missing []string
	for _, r := range required {
		if r.value == "" {
			missing = append(missing, r.name)
		}
	}
	if len(missing) > 0 {
		errs = append(errs, fmt.Errorf("sign-in needs all of OIDC_ISSUER, OIDC_CLIENT_ID, OIDC_CLIENT_SECRET and OIDC_REDIRECT_URL; missing %s", strings.Join(missing, ", ")))
	}

	if o.IssuerURL != "" {
		if err := checkIssuerURL(o.IssuerURL); err != nil {
			errs = append(errs, err)
		}
	}
	if o.RedirectURL != "" {
		if err := checkRedirectURL(o.RedirectURL); err != nil {
			errs = append(errs, err)
		}
	}
	if len(errs) > 0 {
		return OIDC{}, errs
	}
	return o, nil
}

// checkIssuerURL applies OpenID Connect Discovery's rules: an https URL with
// no query or fragment. Plain http is accepted only on the loopback
// interface, for a provider running on the developer's machine.
func checkIssuerURL(raw string) error {
	u, err := url.Parse(raw)
	if err != nil || u.Host == "" || u.User != nil {
		return fmt.Errorf("OIDC_ISSUER must be an absolute URL, got %q", raw)
	}
	if !secureOrLoopback(u) {
		return fmt.Errorf("OIDC_ISSUER must use https (http only on localhost or *.localhost), got %q", raw)
	}
	if u.RawQuery != "" || u.Fragment != "" {
		return fmt.Errorf("OIDC_ISSUER must not have a query or fragment, got %q", raw)
	}
	return nil
}

// checkRedirectURL makes sure the provider will send the browser back to
// this server's callback handler, over https (or http on localhost).
func checkRedirectURL(raw string) error {
	u, err := url.Parse(raw)
	if err != nil || u.Host == "" || u.User != nil {
		return fmt.Errorf("OIDC_REDIRECT_URL must be an absolute URL, got %q", raw)
	}
	if !secureOrLoopback(u) {
		return fmt.Errorf("OIDC_REDIRECT_URL must use https (http only on localhost or *.localhost), got %q", raw)
	}
	if u.Path != CallbackPath || u.RawQuery != "" || u.Fragment != "" {
		return fmt.Errorf("OIDC_REDIRECT_URL must end in %s with no query or fragment, got %q", CallbackPath, raw)
	}
	return nil
}

// secureOrLoopback reports whether u uses https, or http to a loopback
// host (see IsLoopbackHost), where nothing crosses the network.
func secureOrLoopback(u *url.URL) bool {
	switch u.Scheme {
	case "https":
		return true
	case "http":
		return IsLoopbackHost(u.Hostname())
	default:
		return false
	}
}

// IsLoopbackHost reports whether host names this machine: a loopback IP
// (127.0.0.0/8 or ::1), "localhost", or a name ending in ".localhost".
//
// RFC 6761, section 6.3, reserves the whole .localhost domain for loopback,
// and browsers resolve *.localhost to 127.0.0.1 on their own. That is what
// lets the local stack (deploy/local/compose.yaml) use one issuer URL,
// http://idp.localhost:9090, for both sides: the browser reaches the
// development provider through the published port, and the API container
// reaches it through a Docker network alias with the same name.
func IsLoopbackHost(host string) bool {
	host = strings.ToLower(strings.TrimSuffix(host, "."))
	if host == "localhost" || strings.HasSuffix(host, ".localhost") {
		return true
	}
	ip := net.ParseIP(host)
	return ip != nil && ip.IsLoopback()
}

// parseLogLevel accepts the four slog levels, case-insensitively.
func parseLogLevel(raw string) (slog.Level, error) {
	switch strings.ToLower(raw) {
	case "debug":
		return slog.LevelDebug, nil
	case "info":
		return slog.LevelInfo, nil
	case "warn":
		return slog.LevelWarn, nil
	case "error":
		return slog.LevelError, nil
	default:
		return 0, fmt.Errorf("LOG_LEVEL must be one of debug, info, warn, error; got %q", raw)
	}
}

// loadImages reads GEMINI_API_KEY, GEMINI_IMAGE_MODEL, IMAGE_GENERATOR and
// IMAGE_MONTHLY_LIMIT. None is required: without a key (and without the fake),
// generation is off and the rest of the app works.
func loadImages(getenv func(string) string, cloudRun bool) (Images, []error) {
	img := Images{
		GeminiAPIKey: Secret(strings.TrimSpace(getenv("GEMINI_API_KEY"))),
		Model:        strings.TrimSpace(getenv("GEMINI_IMAGE_MODEL")),
	}
	var errs []error
	switch mode := strings.TrimSpace(getenv("IMAGE_GENERATOR")); mode {
	case "", "gemini":
	case "fake":
		if cloudRun {
			errs = append(errs, errors.New("IMAGE_GENERATOR=fake is not allowed on Cloud Run"))
		}
		img.Fake = true
	default:
		errs = append(errs, fmt.Errorf("IMAGE_GENERATOR must be gemini or fake, got %q", mode))
	}
	if raw := strings.TrimSpace(getenv("IMAGE_MONTHLY_LIMIT")); raw != "" {
		n, err := strconv.Atoi(raw)
		switch {
		case err != nil:
			errs = append(errs, fmt.Errorf("IMAGE_MONTHLY_LIMIT must be a number, got %q", raw))
		case n < 1 || n > maxImageMonthlyLimit:
			errs = append(errs, fmt.Errorf("IMAGE_MONTHLY_LIMIT must be between 1 and %d, got %d", maxImageMonthlyLimit, n))
		default:
			img.MonthlyLimit = n
		}
	}
	if raw := strings.TrimSpace(getenv("IMAGE_DAILY_LIMIT")); raw != "" {
		n, err := strconv.Atoi(raw)
		switch {
		case err != nil:
			errs = append(errs, fmt.Errorf("IMAGE_DAILY_LIMIT must be a number, got %q", raw))
		case n < 1 || n > maxImageDailyLimit:
			errs = append(errs, fmt.Errorf("IMAGE_DAILY_LIMIT must be between 1 and %d, got %d", maxImageDailyLimit, n))
		default:
			img.DailyLimit = n
		}
	}
	return img, errs
}

// CampaignCapOff is MAX_CAMPAIGNS_PER_USER=off: no cap on campaigns per master.
const CampaignCapOff = -1

// loadLimits reads RATE_LIMIT_MULTIPLIER, MAX_CAMPAIGNS_PER_USER and
// CAMPAIGN_CREATORS. An e-mail in the list is not a secret, but it is
// personal data: the error for a bad entry never repeats it.
func loadLimits(getenv func(string) string) (Limits, []error) {
	var (
		l    Limits
		errs []error
	)
	if raw := strings.TrimSpace(getenv("RATE_LIMIT_MULTIPLIER")); raw != "" {
		f, err := strconv.ParseFloat(raw, 64)
		switch {
		case err != nil || math.IsNaN(f):
			errs = append(errs, fmt.Errorf("RATE_LIMIT_MULTIPLIER must be a number, got %q", raw))
		case f <= 0 || f > maxRateMultiplier:
			errs = append(errs, fmt.Errorf("RATE_LIMIT_MULTIPLIER must be above 0 and at most %d, got %s", maxRateMultiplier, raw))
		default:
			l.RateMultiplier = f
		}
	}
	if raw := strings.TrimSpace(getenv("MAX_CAMPAIGNS_PER_USER")); raw != "" {
		n, err := strconv.Atoi(raw)
		switch {
		case strings.EqualFold(raw, "off"):
			// No cap: for the local and CI stacks, where the e2e suite makes
			// hundreds of campaigns as one test master. Refused on Cloud Run (Load).
			l.MaxCampaignsPerUser = CampaignCapOff
		case err != nil:
			errs = append(errs, fmt.Errorf("MAX_CAMPAIGNS_PER_USER must be a number, got %q", raw))
		case n < 1 || n > maxCampaignsPerUserLimit:
			errs = append(errs, fmt.Errorf("MAX_CAMPAIGNS_PER_USER must be between 1 and %d, got %d", maxCampaignsPerUserLimit, n))
		default:
			l.MaxCampaignsPerUser = n
		}
	}
	if raw := strings.TrimSpace(getenv("CAMPAIGN_CREATORS")); raw != "" {
		seen := map[string]bool{}
		for i, entry := range strings.Split(raw, ",") {
			email := strings.ToLower(strings.TrimSpace(entry))
			switch {
			case email == "":
				continue // a trailing comma
			case !strings.Contains(email, "@"):
				errs = append(errs, fmt.Errorf("CAMPAIGN_CREATORS entry %d is not an e-mail address", i+1))
			case !seen[email]:
				seen[email] = true
				l.CampaignCreators = append(l.CampaignCreators, email)
			}
		}
		if len(l.CampaignCreators) == 0 && len(errs) == 0 {
			// Only separators: an empty list would let anyone create campaigns.
			errs = append(errs, errors.New("CAMPAIGN_CREATORS has no e-mail address; leave it empty to let anyone create"))
		}
	}
	return l, errs
}
