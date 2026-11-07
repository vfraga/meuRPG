package campaignpackage

import (
	"cmp"
	"context"
	"errors"
	"log/slog"
	"net/http"
	"slices"
	"sync"
	"time"

	"connectrpc.com/connect"
	"github.com/jackc/pgx/v5/pgxpool"

	pkgv1 "github.com/PuraFome/meuRPG/backend/gen/meurpg/campaignpackage/v1"
	"github.com/PuraFome/meuRPG/backend/gen/meurpg/campaignpackage/v1/campaignpackagev1connect"
	campaignsv1 "github.com/PuraFome/meuRPG/backend/gen/meurpg/campaigns/v1"
	"github.com/PuraFome/meuRPG/backend/internal/authz"
	"github.com/PuraFome/meuRPG/backend/internal/campaignpackage/packagedb"
	"github.com/PuraFome/meuRPG/backend/internal/platform/blob"
	"github.com/PuraFome/meuRPG/backend/internal/platform/nostore"
	"github.com/PuraFome/meuRPG/backend/internal/platform/ratelimit"
	"github.com/PuraFome/meuRPG/backend/internal/platform/rpcerr"
	"github.com/PuraFome/meuRPG/backend/internal/platform/safego"
	"github.com/PuraFome/meuRPG/backend/internal/platform/tasks"
)

// The routes. Both live in the namespaces the server reserves for the API
// (httpserver.isAPIPath).
const (
	// PartsPath is where the parts of an upload are sent: PUT PartsPath +
	// "{id}/parts/{n}".
	PartsPath = "/uploads/campaign-imports/"
	// DownloadsPath is where a finished export is downloaded: GET
	// DownloadsPath + "{id}".
	DownloadsPath = "/downloads/campaign-exports/"
)

// How long things are kept.
const (
	// ExportTTL is how long a finished export stays.
	ExportTTL = 24 * time.Hour
	// UploadTTL is how long the parts of an upload stay after the last one.
	UploadTTL = time.Hour
	// failedExportTTL is how long a failed or canceled export's row stays, so
	// the page can say so.
	failedExportTTL = time.Hour
	// staleExportAfter is how long a running export may go without moving
	// before it is taken for dead (a restart killed it).
	staleExportAfter = 10 * time.Minute
	// exportTimeout bounds one export.
	exportTimeout = 20 * time.Minute
)

// Creation is what the package needs from the campaigns module: who may create
// a campaign and the campaign a retried create already made.
type Creation interface {
	// CheckCreation refuses, with the errors CreateCampaign gives, a person who
	// may not create another campaign now (the creators list and the cap). It
	// reads through the pool.
	CheckCreation(ctx context.Context, userID string) error
	// FindCreated returns the campaign a create with this key already made for
	// the person, if any. A key used for another request is an error.
	FindCreated(ctx context.Context, userID, key string, hash *string) (*campaignsv1.Campaign, bool, error)
}

// Sessions is who is calling: the identity service in production.
type Sessions interface {
	Interceptor() connect.Interceptor
	AuthenticateRequest(r *http.Request) (context.Context, error)
	authz.Caller
}

// Config is what New needs.
type Config struct {
	Pool *pgxpool.Pool
	// Tasks runs the detached work (an export being written, the sweep of what expired).
	// Nil means an in-process runner of its own, which is what tests and local runs use.
	Tasks tasks.Runner
	// Blobs is the blob store; nil turns the package off (every call answers
	// IMAGES_OFF).
	Blobs blob.Store
	// Parts are the modules' parts, in the order they stage and apply: the
	// campaign first, then what the others refer to.
	Parts    []Part
	Creation Creation
	Logger   *slog.Logger
	Now      func() time.Time
	// ContentVersion is written into the manifest of an export (the rules
	// content's revision and hash).
	ContentVersion string
	// Heavy limits the calls that read or write a whole campaign (start an
	// export, preview, create, download) and PartsLimit the parts of an
	// upload, per user. Nil: no limit (tests).
	Heavy, PartsLimit *ratelimit.Limiter
	// PartReadTimeout is how long a client has to send one part of an import; zero means partReadTimeout. Tests shorten it.
	PartReadTimeout time.Duration
}

// Service is the campaign package: the export job, the upload and the import.
type Service struct {
	pool     *pgxpool.Pool
	queries  *packagedb.Queries
	blobs    blob.Store
	parts    []Part
	creation Creation
	logger   *slog.Logger
	now      func() time.Time
	version  string
	heavy    *ratelimit.Limiter
	partsLim *ratelimit.Limiter
	// partReadTimeout is how long a client has to send one part (Config.PartReadTimeout).
	partReadTimeout time.Duration
	heavyN          *ratelimit.Notifier
	partsN          *ratelimit.Notifier
	tasks           tasks.Runner
	// ownTasks is the in-process runner New made when Config.Tasks was nil; the service waits
	// for it and cancels it. A runner of the caller is the caller's to start, wait and cancel.
	ownTasks *tasks.InProcess

	// exportSlot lets one export run at a time on this instance: an export
	// holds a snapshot and a zip in flight.
	exportSlot chan struct{}
	mu         sync.Mutex
	running    map[string]context.CancelFunc
	wg         sync.WaitGroup
}

var _ campaignpackagev1connect.CampaignPackageServiceHandler = (*Service)(nil)

// New returns a Service.
func New(cfg Config) (*Service, error) {
	if cfg.Pool == nil {
		return nil, errors.New("campaignpackage: a Pool is required")
	}
	if cfg.Creation == nil {
		return nil, errors.New("campaignpackage: Creation is required")
	}
	s := &Service{
		pool: cfg.Pool, queries: packagedb.New(cfg.Pool), blobs: cfg.Blobs, parts: cfg.Parts, creation: cfg.Creation,
		logger: cfg.Logger, now: cfg.Now, version: cfg.ContentVersion, heavy: cfg.Heavy, partsLim: cfg.PartsLimit,
		exportSlot: make(chan struct{}, 1), running: map[string]context.CancelFunc{},
	}
	s.partReadTimeout = cmp.Or(cfg.PartReadTimeout, partReadTimeout)
	if s.logger == nil {
		s.logger = slog.Default()
	}
	if s.now == nil {
		s.now = time.Now
	}
	s.tasks = cfg.Tasks
	if s.tasks == nil {
		s.ownTasks = tasks.NewInProcess(s.logger)
		s.tasks = s.ownTasks
	}
	s.tasks.Register(exportKind, s.handleExport, tasks.Options{Deadline: exportDeadline})
	s.tasks.Every(sweepKind, sweepEvery, s.handleSweep, tasks.Options{Deadline: sweepDeadline})
	s.heavyN = ratelimit.NewNotifier(s.logger, "campaign package calls")
	s.partsN = ratelimit.NewNotifier(s.logger, "campaign package parts")
	return s, nil
}

// CheckWired reports a missing collaborator (platform/wiring).
func (s *Service) CheckWired() error {
	if len(s.parts) == 0 {
		return errors.New("campaignpackage: no parts are connected")
	}
	return nil
}

// Mount registers the service and the two routes.
func (s *Service) Mount(handle func(pattern string, handler http.Handler), sessions Sessions, members authz.MembershipSource, opts ...connect.HandlerOption) {
	opts = append(slices.Clip(opts), connect.WithInterceptors(
		nostore.Interceptor(),
		sessions.Interceptor(),
		authz.Interceptor(sessions, members, s.logger),
	))
	handle(campaignpackagev1connect.NewCampaignPackageServiceHandler(s, opts...))

	withAuthz := authz.Middleware(sessions, members, s.logger)
	route := func(h http.HandlerFunc) http.Handler {
		return s.withSession(sessions, withAuthz(h))
	}
	handle("PUT "+PartsPath+"{id}/parts/{n}", route(s.handlePart))
	handle("GET "+DownloadsPath+"{id}", route(s.handleDownload))
}

// withSession finds the caller's session for a plain route, like the Connect
// interceptor.
func (s *Service) withSession(sessions Sessions, next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		ctx, err := sessions.AuthenticateRequest(r)
		if err != nil {
			s.writeError(w, r, err)
			return
		}
		next.ServeHTTP(w, r.WithContext(ctx))
	})
}

// Wait blocks until the exports running in this process end, or ctx ends.
func (s *Service) Wait(ctx context.Context) {
	if s.ownTasks != nil {
		s.ownTasks.Wait(ctx)
	}
	done := make(chan struct{})
	safego.Go(s.logger, "wait for the export goroutines", func() { defer close(done); s.wg.Wait() })
	select {
	case <-done:
	case <-ctx.Done():
	}
}

// Cancel stops every export running in this process (the server is going
// down); they are marked interrupted by the next read.
func (s *Service) Cancel() {
	s.mu.Lock()
	defer s.mu.Unlock()
	for _, cancel := range s.running {
		cancel()
	}
}

func (s *Service) dbError(ctx context.Context, action string, err error) error {
	return rpcerr.FromDB(ctx, s.logger, "campaignpackage", action, err)
}

// allow applies a per-user limiter: a nil error lets the call go on.
func (s *Service) allow(ctx context.Context, l *ratelimit.Limiter, n *ratelimit.Notifier, userID string) error {
	if l == nil {
		return nil
	}
	if ok, wait := l.Allow(userID); !ok {
		n.Hit(ctx)
		return ratelimit.RPCError(wait)
	}
	return nil
}

// blocked is the failed_precondition of this service, with its detail.
func blocked(reason pkgv1.CampaignPackageBlockedReason, msg string, preview *pkgv1.PreviewCampaignImportResponse) error {
	err := connect.NewError(connect.CodeFailedPrecondition, errors.New(msg))
	if detail, derr := connect.NewErrorDetail(&pkgv1.CampaignPackageBlocked{Reason: reason, Preview: preview}); derr == nil {
		err.AddDetail(detail)
	}
	return err
}

func errImagesOff() error {
	return blocked(pkgv1.CampaignPackageBlockedReason_CAMPAIGN_PACKAGE_BLOCKED_REASON_IMAGES_OFF, "the server has no store for files, so packages are off", nil)
}
