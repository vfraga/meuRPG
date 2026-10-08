// Package httpserver runs the API's HTTP server: health probes, request
// logging, cross-origin (CSRF) protection, HTTP/1 + h2c, and a graceful
// shutdown that fits Cloud Run.
package httpserver

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"net"
	"net/http"
	"sync/atomic"
	"time"

	"github.com/PuraFome/meuRPG/backend/internal/platform/slowclient"
)

// Timeouts. There is deliberately no ReadTimeout or WriteTimeout: they cap
// the whole request/response, which would cut Connect streaming RPCs short.
// Cloud Run already enforces a per-request timeout (5 minutes by default;
// the live session's stream lasts up to 30, so the service needs 35, see
// docs/operations.md), and unary RPCs can be bounded per handler. What a slow
// client could stall is bounded where it happens, with a deadline per operation
// (platform/slowclient): each Send on a stream, the body of an upload.
const (
	// Time a client has to send the request headers. Protects against
	// Slowloris-style clients that open connections and trickle bytes.
	readHeaderTimeout = 10 * time.Second

	// How long an idle keep-alive connection stays open.
	idleTimeout = 2 * time.Minute

	// Cloud Run sends SIGTERM and kills the container 10 seconds later, so
	// in-flight requests get slightly less than that to finish.
	defaultShutdownTimeout = 8 * time.Second
)

// Pinger reports whether a dependency is reachable. *pgxpool.Pool
// implements it.
type Pinger interface {
	Ping(ctx context.Context) error
}

// Config holds what the server needs from the outside world.
type Config struct {
	// Addr is the listen address, e.g. ":8080".
	Addr string

	// Logger receives startup, shutdown and per-request logs. Nil means
	// slog.Default().
	Logger *slog.Logger

	// TraceProject is the Google Cloud project ID, set on Cloud Run only
	// (GOOGLE_CLOUD_PROJECT). With it, each log line carries the request's
	// Cloud Trace id. Empty means no trace key.
	TraceProject string

	// DB is checked by /readyz. Leave it nil when running without a
	// database; readiness then reports the database as "disabled".
	DB Pinger

	// Limit, when set, wraps the routes: it is where the per-IP rate limit sits
	// (ratelimit.Middleware). It runs after the request log and the security
	// headers, so a refused request is logged and carries them, and before
	// anything reads a session or the database.
	Limit func(http.Handler) http.Handler

	// ShutdownTimeout bounds the graceful shutdown. Zero means 8 seconds.
	ShutdownTimeout time.Duration
}

// Server is the API's HTTP server. Create it with New, register handlers
// with Handle, then call Run.
type Server struct {
	mux             *http.ServeMux
	httpServer      *http.Server
	logger          *slog.Logger
	db              Pinger
	shutdownTimeout time.Duration

	// draining becomes true once shutdown starts, so /readyz answers 503
	// and load balancers stop sending new requests.
	draining atomic.Bool

	// ping caches the database answer of /readyz (see pingCache).
	ping pingCache
	now  func() time.Time
}

// New builds a Server with /healthz and /readyz already registered.
func New(cfg Config) *Server {
	s := &Server{
		mux:             http.NewServeMux(),
		logger:          cfg.Logger,
		db:              cfg.DB,
		now:             time.Now,
		shutdownTimeout: cfg.ShutdownTimeout,
	}
	if s.logger == nil {
		s.logger = slog.Default()
	}
	if s.shutdownTimeout == 0 {
		s.shutdownTimeout = defaultShutdownTimeout
	}

	s.mux.HandleFunc("GET /healthz", s.handleHealthz)
	s.mux.HandleFunc("GET /readyz", s.handleReadyz)

	// Serve HTTP/1.1 and HTTP/2 without TLS ("h2c"). Cloud Run terminates
	// TLS at its front end and, when the service is deployed with --use-http2,
	// talks HTTP/2 to the container in clear text. HTTP/2 is what lets
	// Connect stream in both directions at once.
	var protocols http.Protocols
	protocols.SetHTTP1(true)
	protocols.SetUnencryptedHTTP2(true)

	// CrossOriginProtection stops CSRF: it rejects, with 403, a browser
	// request that changes state (POST, PUT, DELETE...) and comes from
	// another origin, judged by the Sec-Fetch-Site header or by comparing
	// Origin with Host. Safe methods (GET, HEAD, OPTIONS) always pass, so a
	// GET must never change state unless it carries its own protection, as
	// the OIDC callback does with state, PKCE and nonce. It sits inside
	// logRequests so rejected requests are logged too.
	csrf := http.NewCrossOriginProtection()

	routes := csrf.Handler(s.mux)
	if cfg.Limit != nil {
		routes = cfg.Limit(routes)
	}

	s.httpServer = &http.Server{
		Addr: cfg.Addr,
		// Outermost first: the request log, the security headers, the slow-client
		// deadlines (they need the real ResponseWriter; the others pass it through),
		// then the rate limit (if any) and CSRF.
		Handler:           logRequests(s.logger, cfg.TraceProject, securityHeaders(slowclient.Middleware(routes))),
		Protocols:         &protocols,
		ReadHeaderTimeout: readHeaderTimeout,
		IdleTimeout:       idleTimeout,
		ErrorLog:          slog.NewLogLogger(s.logger.Handler(), slog.LevelWarn),
	}
	return s
}

// Handle registers a handler, typically one returned by a generated
// New<Service>Handler function, whose signature matches this method's:
//
//	srv.Handle(systemv1connect.NewSystemServiceHandler(svc))
func (s *Server) Handle(pattern string, handler http.Handler) {
	s.mux.Handle(pattern, handler)
}

// OnShutdown registers f to run when the graceful shutdown starts, in its
// own goroutine. It is for handlers that never finish on their own, such as
// live streams (PlayService.WatchGameSession): the shutdown waits for every
// request in flight, so f must make them return, or the shutdown waits for
// its whole deadline and then cuts them off.
func (s *Server) OnShutdown(f func()) {
	s.httpServer.RegisterOnShutdown(f)
}

// Handler returns the server's root handler (routes plus request logging).
// It is handy for tests with net/http/httptest.
func (s *Server) Handler() http.Handler {
	return s.httpServer.Handler
}

// Run listens on Config.Addr and serves until ctx is canceled, then shuts
// down gracefully. See Serve.
func (s *Server) Run(ctx context.Context) error {
	var lc net.ListenConfig
	ln, err := lc.Listen(ctx, "tcp", s.httpServer.Addr)
	if err != nil {
		return fmt.Errorf("listen on %s: %w", s.httpServer.Addr, err)
	}
	return s.Serve(ctx, ln)
}

// Serve accepts connections on ln until ctx is canceled (for example by
// SIGTERM), then shuts down gracefully:
//
//  1. /readyz starts answering 503, so no new traffic is routed here.
//  2. The listener closes: no new connections are accepted.
//  3. In-flight requests get up to ShutdownTimeout to finish.
//
// It returns nil after a clean shutdown.
func (s *Server) Serve(ctx context.Context, ln net.Listener) error {
	serveErr := make(chan error, 1)
	go func() {
		serveErr <- s.httpServer.Serve(ln)
	}()
	s.logger.InfoContext(ctx, "http server started", "addr", ln.Addr().String())

	select {
	case err := <-serveErr:
		// Serve only returns early on a real failure.
		return fmt.Errorf("http server: %w", err)
	case <-ctx.Done():
	}

	s.logger.InfoContext(ctx, "shutting down", "timeout", s.shutdownTimeout.String())
	s.draining.Store(true)

	// ctx is already canceled; WithoutCancel keeps its values but gives the
	// shutdown its own deadline.
	shutdownCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), s.shutdownTimeout)
	defer cancel()

	if err := s.httpServer.Shutdown(shutdownCtx); err != nil {
		// The deadline passed with requests still running: close them.
		_ = s.httpServer.Close()
		return fmt.Errorf("graceful shutdown: %w", err)
	}
	if err := <-serveErr; !errors.Is(err, http.ErrServerClosed) {
		return fmt.Errorf("http server: %w", err)
	}

	s.logger.InfoContext(ctx, "http server stopped")
	return nil
}
