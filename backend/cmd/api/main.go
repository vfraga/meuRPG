// Command api is MeuRPG's HTTP server: it serves the Connect RPCs used by the
// web app plus the /healthz and /readyz probes.
//
// Configuration comes from the environment (see internal/platform/config):
//
//	PORT                listen port (default 8080)
//	LISTEN_HOST         interface to listen on, e.g. 127.0.0.1 (default: all, what Cloud Run needs)
//	GOOGLE_CLOUD_PROJECT the project id, set by the deploy (Cloud Run doesn't): log lines then carry the trace
//	DATABASE_URL        CockroachDB connection string (optional)
//	LOG_LEVEL           debug, info, warn or error (default info)
//	OIDC_ISSUER         sign-in provider, e.g. https://accounts.google.com (optional)
//	OIDC_CLIENT_ID      this app's client ID at the provider
//	OIDC_CLIENT_SECRET  this app's client secret at the provider
//	OIDC_REDIRECT_URL   https://<this server>/auth/callback
//	OIDC_CA_FILE        extra CA certificates (PEM) to trust, for a local provider (optional)
//	OIDC_MAX_AGE        max_age sent to the provider, e.g. 1h (optional)
//	BLOB_DIR            directory for uploaded images (optional)
//	GEMINI_API_KEY      key of the Gemini API: turns image generation on (optional, secret)
//	GEMINI_IMAGE_MODEL  the image model (default gemini-3.1-flash-image)
//	IMAGE_GENERATOR     "fake" uses the deterministic fake generator, never on Cloud Run (optional)
//	IMAGE_MONTHLY_LIMIT images a campaign may generate per month (default 20)
//	IMAGE_DAILY_LIMIT   images the whole server may generate per day (default 100)
//	MAX_CAMPAIGNS_PER_USER campaigns one account may be master of (default 10)
//	CAMPAIGN_CREATORS   verified e-mails, comma-separated, that may create campaigns (default: anyone)
//	RATE_LIMIT_MULTIPLIER scales every request-rate limit (default 1; the e2e suite raises it)
//
// Sign-in needs both the OIDC_* variables and DATABASE_URL. Without them
// the API still starts, and the sign-in routes answer 503. CampaignService,
// CampaignDocumentService, CharacterService, ContentService,
// TableContentService, PlayService,
// ProgressionService,
// GalleryService and MapService need sign-in too; without it, they are not
// mounted. Images also need BLOB_DIR: without it, the image routes and
// GalleryService answer 503 (unavailable), and the rest works (MapService
// too, but no image can be uploaded, so no map can be created).
//
// The rules content (the SRD 5.1 snapshot, package rules) is embedded in
// the binary and loaded at startup, always: a broken snapshot stops the
// server right away instead of failing on a player's sheet.
//
// The modules meet here and nowhere else: campaigns, characters, play and
// maps learn who is calling from identity (the authz.Caller interface; the
// live stream also reads the session again, authz.SessionRechecker), and
// each member's role from campaigns (authz.MembershipSource); identity
// completes the "accept this invite" sign-in intent through campaigns (an
// identity.IntentHandler); maps stores images in the blob store
// (platform/blob), learns who is calling on its plain HTTP routes from
// identity too (maps.Sessions), finds the characters that may stand on a
// map through characters (maps.CharacterDirectory), and reads the session's
// current map and shown image and publishes map changes on the live stream
// through play (maps.LiveSession), and asks play whether a combat runs on a map
// (maps.CombatMaps); play locks the players' sheets and keeps the
// characters' vitals through characters (play.SheetLocker,
// play.VitalsKeeper), lists a user's campaigns through campaigns
// (play.CampaignDirectory), and reveals the map it makes current and reads
// the image it shows through maps (play.MapKeeper, maps.SessionMaps); and
// characters settles a pending
// member's membership through campaigns when the master approves or rejects
// their character (characters.PendingMembers, RN-15), and campaigns approves
// a pending member's character through characters when an invite without
// approval promotes them (campaigns.Characters, Q25). No package imports
// another's internals.
package main

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"net"
	"net/http"
	"os"
	"os/signal"
	"runtime/debug"
	"strconv"
	"syscall"
	"time"

	"connectrpc.com/connect"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	charactersv1 "github.com/PuraFome/meuRPG/backend/gen/meurpg/characters/v1"
	"github.com/PuraFome/meuRPG/backend/gen/meurpg/system/v1/systemv1connect"
	"github.com/PuraFome/meuRPG/backend/internal/campaigns"
	"github.com/PuraFome/meuRPG/backend/internal/characters"
	"github.com/PuraFome/meuRPG/backend/internal/identity"
	"github.com/PuraFome/meuRPG/backend/internal/maps"
	"github.com/PuraFome/meuRPG/backend/internal/maps/images/gen"
	"github.com/PuraFome/meuRPG/backend/internal/notes"
	"github.com/PuraFome/meuRPG/backend/internal/platform/blob"
	"github.com/PuraFome/meuRPG/backend/internal/platform/config"
	"github.com/PuraFome/meuRPG/backend/internal/platform/db"
	"github.com/PuraFome/meuRPG/backend/internal/platform/httpserver"
	"github.com/PuraFome/meuRPG/backend/internal/platform/logging"
	"github.com/PuraFome/meuRPG/backend/internal/platform/ratelimit"
	"github.com/PuraFome/meuRPG/backend/internal/platform/rpclog"
	"github.com/PuraFome/meuRPG/backend/internal/platform/slowclient"
	"github.com/PuraFome/meuRPG/backend/internal/play"
	"github.com/PuraFome/meuRPG/backend/internal/progression"
	"github.com/PuraFome/meuRPG/backend/internal/rules"
	"github.com/PuraFome/meuRPG/backend/internal/system"
)

// The maps module opens the doors the combat walks into (RN-26): play picks it up
// from SetTerrain by a type assertion, so a drifted signature must fail here.
var (
	_ play.TerrainSource = (*maps.Service)(nil)
	_ play.DoorKeeper    = (*maps.Service)(nil)
	_ play.PuzzleMaps    = (*maps.Service)(nil) // a solved puzzle opens a door, reveals a point or a clue (MR-038)
)

// Build information, replaced at build time with:
//
//	go build -ldflags "-X main.version=v0.1.0 -X main.commit=$(git rev-parse HEAD)"
var (
	version = "dev"
	commit  = "unknown"
)

// maxRequestBytes caps the size of a single RPC message the server will
// read, so a client cannot exhaust memory with one huge request.
const maxRequestBytes = 4 << 20 // 4 MiB

// streamSendTimeout is how long one message of a live stream may take to be
// written. A message is a few kilobytes; 30 s is generous for a bad mobile
// connection and short next to the 30-minute stream.
const streamSendTimeout = 30 * time.Second

func main() {
	cfg, err := config.Load(os.Getenv)
	if err != nil {
		// The configured log level is unknown here, so use the default one.
		logging.New(os.Stdout, config.DefaultLogLevel, logging.WithVersion(version)).Error("cannot start", "error", err)
		os.Exit(2)
	}
	logger := logging.New(os.Stdout, cfg.LogLevel, logging.WithVersion(version))

	if err := run(logger, cfg); err != nil {
		logger.Error("api stopped with an error", "error", err)
		os.Exit(1)
	}
}

// run wires the application together. Keeping it apart from main means
// deferred cleanups (closing the pool) run before the process exits.
func run(logger *slog.Logger, cfg config.Config) error {
	// Cancel ctx on Ctrl+C (SIGINT) or SIGTERM, which is how Cloud Run asks
	// a container to stop. Canceling ctx starts the graceful shutdown.
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	// After the first signal, restore the default behavior so a second
	// Ctrl+C kills the process right away instead of waiting for the drain.
	context.AfterFunc(ctx, stop)

	logger.Info("starting api", "commit", commit) // the version is on every line already

	// Loading the rules content checks every entry and compiles every
	// formula (ADR-0008). An error is a bug in the embedded data, so stop.
	rulesContent, err := rules.LoadSRD()
	if err != nil {
		return fmt.Errorf("load the rules content: %w", err)
	}
	logger.Info("rules content loaded", "content_version", rulesContent.Version())

	// database stays a nil interface when DATABASE_URL is empty. It must not
	// hold a nil *pgxpool.Pool: an interface holding a typed nil pointer is
	// not == nil, and /readyz would then call Ping on it.
	var database httpserver.Pinger
	var pool *pgxpool.Pool
	if cfg.DatabaseURL == "" {
		logger.Warn("DATABASE_URL is not set; running without a database")
	} else {
		pool, err = db.NewPool(ctx, cfg.DatabaseURL.Reveal())
		if err != nil {
			return err
		}
		defer pool.Close()

		// A failed ping is not fatal: the database may come up later, and
		// until it does /readyz reports it as down.
		if err := pool.Ping(ctx); err != nil {
			logger.Warn("database is not reachable yet", "error", err)
		}
		database = pool
	}

	// Images need somewhere to live. blobs stays a nil interface without
	// BLOB_DIR (never a nil *blob.FS, for the reason given for database
	// above), and the maps module then answers 503 for images.
	var blobs blob.Store
	if cfg.BlobDir == "" {
		logger.Warn("BLOB_DIR is not set; images are off: uploads, image downloads and the gallery answer 503")
	} else {
		fs, err := blob.NewFS(cfg.BlobDir)
		if err != nil {
			return err
		}
		defer func() { _ = fs.Close() }()
		blobs = fs
		logger.Info("images are stored on disk", "dir", cfg.BlobDir)
	}

	// The request-rate limits (docs/architecture.md#abuse-limits), in this
	// instance's memory. They are built before the modules, which hold two of them.
	policy := ratelimit.NewPolicy(cfg.Limits.RateMultiplier)

	// Image generation (MR-039, RN-28) needs a key, or the fake. imageGenerator
	// stays a nil interface without them (never a nil *gen.Gemini), and the maps
	// module then answers with the typed "off" reason.
	var imageGenerator gen.Generator
	switch {
	case cfg.Images.Fake:
		imageGenerator = &gen.Fake{}
		logger.Warn("IMAGE_GENERATOR=fake: images are made by the fake generator, not by a model")
	case cfg.Images.GeminiAPIKey != "":
		imageGenerator = &gen.Gemini{Key: cfg.Images.GeminiAPIKey, ModelName: cfg.Images.Model, Logger: logger}
		// The model name is not secret; the key never goes into a log line.
		logger.Info("image generation is on", "model", imageGenerator.Model())
	default:
		logger.Warn("GEMINI_API_KEY is not set; image generation is off")
	}

	// Sign-in needs a provider and a database. identityService stays nil
	// when either is missing, and the sign-in routes then answer 503.
	// Campaigns, characters and game sessions need to know who is calling,
	// so they come with sign-in.
	var identityService *identity.Service
	var m *modules
	switch {
	case !cfg.OIDC.Configured():
		logger.Warn("OIDC_ISSUER is not set; sign-in is disabled")
	case pool == nil:
		logger.Warn("sign-in is disabled: it needs DATABASE_URL")
	default:
		m, err = wireModules(logger, pool, rulesContent, blobs, imageGenerator, wireOptions{
			MonthlyImages:       cfg.Images.MonthlyLimit,
			DailyImages:         cfg.Images.DailyLimit,
			MaxCampaignsPerUser: cfg.Limits.MaxCampaignsPerUser,
			CampaignCreators:    cfg.Limits.CampaignCreators,
			Policy:              policy,
		})
		if err != nil {
			return err
		}
		identityService, err = identity.New(ctx, identity.Config{
			OIDC:   cfg.OIDC,
			Store:  m.users,
			Logger: logger,
			// Zero (SESSION_IDLE_TIMEOUT unset) means the 14-day default.
			SessionIdleTimeout: cfg.SessionIdleTimeout,
			// On Cloud Run the sign-in rate limit reads the client IP from
			// X-Forwarded-For; anywhere else, from the connection.
			BehindCloudRun: cfg.CloudRun,
			// What a user may ask to finish right after signing in
			// (POST /auth/login): today, accepting a campaign invite.
			Intents: map[string]identity.IntentHandler{
				campaigns.InviteIntentKind: m.campaigns.InviteIntent(),
			},
		})
		if err != nil {
			return err
		}
		// The issuer and client ID are not secret; the client secret is
		// never logged (config.Secret prints as [REDACTED] anyway).
		logger.Info("sign-in is enabled", "issuer", cfg.OIDC.IssuerURL, "client_id", cfg.OIDC.ClientID, "max_age", cfg.OIDC.MaxAge.String())
	}

	srv := httpserver.New(httpserver.Config{
		Addr:   net.JoinHostPort(cfg.ListenHost, strconv.Itoa(cfg.Port)),
		Logger: logger,
		DB:     database,
		// Set on Cloud Run only: the log lines then carry the request's trace.
		TraceProject: cfg.TraceProject,
		// The per-IP limit on every API request, before a session is looked up.
		Limit: ratelimit.Middleware(policy.IP, cfg.CloudRun, ratelimit.NewNotifier(logger, "api requests by ip")),
	})

	connectOpts := connectOptions(logger)
	srv.Handle(systemv1connect.NewSystemServiceHandler(system.NewService(version, commit), connectOpts...))
	if identityService != nil {
		mountModules(srv.Handle, m, identityService, connectOpts, policy.RPC, logger)
		// Live streams never end on their own: end them when the graceful
		// shutdown starts, instead of holding it until its deadline.
		srv.OnShutdown(m.play.Close)
		// At shutdown the pictures in flight stop: a request still waiting for a
		// call slot gives its slot back, the long polls answer, and a call already
		// sent is cut off (its slot stays spent). Cloud Run gives 10 s in all.
		srv.OnShutdown(m.maps.CancelGenerations)
	} else {
		identity.MountDisabled(srv.Handle, connectOpts...)
		logger.Warn("campaigns, characters, game sessions, images and maps are disabled: they need sign-in")
	}

	// Forms on the app's pages (the invite page's POST to /auth/login) are
	// redirected to the provider, so its origin must pass the CSP's
	// form-action. The issuer and its authorization endpoint share an origin
	// for Google and for the local providers.
	var staticOpts []httpserver.StaticOption
	if cfg.OIDC.Configured() {
		staticOpts = append(staticOpts, httpserver.WithFormActionOrigin(cfg.OIDC.IssuerURL))
	}
	static, ok, err := httpserver.NewStatic(cfg.WebDir, staticOpts...)
	switch {
	case err != nil:
		return err
	case ok:
		srv.Handle("/", static)
		logger.Info("serving the web app", "dir", cfg.WebDir)
	default:
		logger.Info("no web build found; running API-only", "dir", cfg.WebDir)
	}

	err = srv.Run(ctx)
	if m != nil {
		// The goroutines write their last lines before the pool closes: about a
		// second, inside Cloud Run's 10 s with the HTTP drain.
		waitCtx, cancel := context.WithTimeout(context.Background(), time.Second)
		defer cancel()
		m.maps.WaitForGenerations(waitCtx)
	}
	return err
}

// modules are the services the sign-in needs, wired together.
// mountModules mounts the signed-in services. It is shared with the test that
// calls every procedure anonymously, so what that test checks is exactly what
// the server serves.
//
// rpcLimit, when set, is the per-user limit on Connect calls (nil: none, for
// tests). It runs right after each service's session interceptor, which is
// what tells it the user (limitedSessions).
func mountModules(handle func(string, http.Handler), m *modules, identityService *identity.Service, opts []connect.HandlerOption, rpcLimit *ratelimit.Limiter, logger *slog.Logger) {
	sessions := limitedSessions{Service: identityService}
	if rpcLimit != nil {
		sessions.limit = ratelimit.Interceptor(rpcLimit,
			func(ctx context.Context) (string, bool) {
				id, err := identityService.UserID(ctx)
				return id, err == nil
			}, ratelimit.NewNotifier(logger, "rpc calls by user"))
	}
	// The identity service is limited like the others: its own interceptor
	// finds the session, then the limit runs.
	identityService.MountLimited(handle, sessions.limit, opts...)
	// identityService is who is calling: its interceptor finds the session,
	// and its UserID reads it back (authz.Caller). This mounts CampaignService
	// and the campaign document's CampaignDocumentService (MR-018), with the
	// same interceptors.
	m.campaigns.Mount(handle, sessions, opts...)
	// campaignsService says who belongs to each campaign, and with which role
	// (authz.MembershipSource).
	m.characters.Mount(handle, sessions, m.campaigns, opts...)
	m.play.Mount(handle, sessions, m.campaigns, opts...)
	m.progression.Mount(handle, sessions, m.campaigns, opts...)
	m.notes.Mount(handle, sessions, m.campaigns, opts...)
	// GalleryService and MapService, plus the upload and download routes,
	// which find the session with identityService.AuthenticateRequest.
	m.maps.Mount(handle, sessions, m.campaigns, opts...)
}

// limitedSessions is the identity service as the modules see it, with the
// per-user rate limit running right after the session interceptor. Every
// module mounts "its" interceptor chain from Sessions.Interceptor, and the
// limit needs the user, which only the session interceptor finds: so the
// limit has to come after it, and this is the one place that can arrange it
// for all of them.
type limitedSessions struct {
	*identity.Service
	limit connect.Interceptor // nil: no limit
}

// Interceptor finds the session and then applies the user's rate limit.
func (l limitedSessions) Interceptor() connect.Interceptor {
	if l.limit == nil {
		return l.Service.Interceptor()
	}
	return ratelimit.Chain(l.Service.Interceptor(), l.limit)
}

type modules struct {
	users       *identity.PostgresStore
	campaigns   *campaigns.Service
	characters  *characters.Service
	play        *play.Service
	maps        *maps.Service
	progression *progression.Service
	notes       *notes.Service
}

// wireOptions are the limits the modules get: zero means each module's own default.
type wireOptions struct {
	MonthlyImages, DailyImages int
	MaxCampaignsPerUser        int
	CampaignCreators           []string
	// Policy holds the limiters some modules apply themselves (the image
	// routes' per-user limits); the zero value limits nothing.
	Policy ratelimit.Policy
}

// wireModules builds the five modules that need each other, connects them in
// the order their setters need, and checks that nothing was left unconnected: a
// nil collaborator quietly turns a feature off (a nil fog source is "no fog of
// war": players would see the NPCs the master hid), so a missing Set... call
// stops the server at startup. It is a function of its own so a test can build
// the real wiring without a database or a sign-in provider.
func wireModules(
	logger *slog.Logger,
	pool *pgxpool.Pool,
	rulesContent *rules.Content,
	blobs blob.Store,
	imageGenerator gen.Generator,
	opts wireOptions,
) (*modules, error) {
	var (
		campaignsService   *campaigns.Service
		charactersService  *characters.Service
		playService        *play.Service
		mapsService        *maps.Service
		progressionService *progression.Service
		notesService       *notes.Service
		err                error
	)
	users := identity.NewPostgresStore(pool)
	campaignsService, err = campaigns.New(campaigns.Config{
		Pool:     pool,
		Profiles: users, // display names and verified e-mails come from the identity module
		// What one account may create (RN-30).
		MaxCampaignsPerUser: opts.MaxCampaignsPerUser,
		Creators:            opts.CampaignCreators,
		Logger:              logger,
	})
	if err != nil {
		return nil, err
	}
	charactersService, err = characters.New(characters.Config{
		Pool:     pool,
		Profiles: users,
		Members:  campaignsService, // approving or rejecting a character settles the membership (RN-15)
		// Every campaign plays with the SRD plus the table's own content (MR-025,
		// RN-23, ADR-0018) and the table rules its master saved (RN-24). SRD is
		// the base content for what no table changes (the conditions).
		Content: characters.NewTableSource(pool, rulesContent, campaignsService),
		SRD:     rulesContent,
		Dice:    levelUpDice{campaignsService}, // how a player rolls the hit die of a level-up (RN-18)
		Logger:  logger,
	})
	if err != nil {
		return nil, err
	}
	// campaigns and characters need each other too, so campaigns gets
	// characters now that it exists: an invite without approval accepted
	// by a pending member approves their character (RN-15).
	campaignsService.SetCharacters(charactersService)
	// maps.SessionMaps needs nothing but the database, so characters gets
	// it now too: an NPC's portrait must be an image of the campaign's
	// gallery (MR-031).
	sessionMaps := maps.NewSessionMaps(pool)
	charactersService.SetGallery(sessionMaps)
	// play and maps need each other: play reveals the map it makes
	// current and reads the image it shows, and maps reads what the
	// session shows and publishes on play's live stream. play gets the
	// SessionMaps made above first, and maps then gets play.
	playService, err = play.New(play.Config{
		Pool:      pool,
		Sheets:    charactersService,           // starting a session locks the sheets (RN-01)
		Vitals:    charactersService,           // the characters' hit points, slots and hit dice (RN-02)
		Campaigns: campaignsService,            // the caller's campaigns, for the session notice (RN-06)
		Maps:      sessionMaps,                 // the current map (RN-10), the shown image (MR-028), the grid and tokens (MR-013)
		Roster:    charactersService,           // who can fight, with which numbers (MR-013)
		Dice:      diceModes{campaignsService}, // where a player rolls (RN-18)
		Defaults:  campaignsService,            // the mode of a combat started without one (RN-24, RN-25)
		Logger:    logger,
	})
	if err != nil {
		return nil, err
	}
	// A level-up changes the sheet everyone in the session watches, so
	// characters publishes the same hint as an XP award.
	charactersService.SetLive(playService)
	// A character's creatures and the combat that holds them (MR-037):
	// dismissing a creature takes it out of the fight, and its events and
	// stream hints go through play.
	charactersService.SetCreatureHost(playService)
	mapsService, err = maps.New(maps.Config{
		Pool:       pool,
		Blobs:      blobs,             // nil: images are off
		Characters: charactersService, // the characters that may stand on a map (MR-012)
		Live:       playService,       // the current map, and where map changes go (RN-10)
		// maps stays on the base SRD: it reads only presets, damage types,
		// conditions, skills and the SRD's magic items and treasure tables, which no
		// table's content changes (MR-025, ADR-0018).
		Generator:     imageGenerator,
		MonthlyImages: int32(opts.MonthlyImages), //nolint:gosec // G115: the config caps it at 500
		DailyImages:   int32(opts.DailyImages),   //nolint:gosec // G115: the config caps it at 10000
		// The per-user limits of the image routes (nil in tests: no limit).
		DownloadLimit: opts.Policy.Download,
		UploadLimit:   opts.Policy.Upload,
		Rules:         rulesContent, // which checks an RP scene may ask for (MR-015), which traps and lights exist (MR-035, MR-036), the treasure generator (MR-044)
		Combats:       playService,  // whether a combat runs on a map: its grid and image cannot change then (MR-034)
		// Whether a new map starts with the fog on is a table rule (RN-24).
		Defaults: campaignsService,
		Logger:   logger,
	})
	if err != nil {
		return nil, err
	}
	// the combat walks over the layers the master painted (MR-034, RN-21)
	playService.SetTerrain(mapsService)
	playService.SetPuzzleMaps(mapsService) // "Ao resolver" of a puzzle (MR-038, RN-27)
	playService.SetFog(mapsService)        // combat per player on a fog map: who sees which NPC (MR-036)
	// traps in play (MR-035): play asks maps for the traps (where, what a character
	// sees, who knows them) and maps asks play to fire one when a token lands in it
	playService.SetTraps(mapsService)
	mapsService.SetTrapFirer(playService)
	// The fog of war and the copy of an image a fog map owns need the maps
	// service, which is made after SessionMaps (it needs play).
	sessionMaps.SetService(mapsService)
	// progression and characters need each other too (the XP lives on the
	// sheets, and a sheet shows "pode subir de nível"): characters gets
	// progression once it exists.
	progressionService, err = progression.New(progression.Config{
		Pool:      pool,
		Party:     charactersService,       // the party, and the XP on the sheets (MR-016)
		Combats:   playService,             // a combat's defeated NPCs and their XP
		Log:       playService,             // the session's history and the xp_changed hint
		Treasures: maps.NewTreasures(pool), // the found treasures "Voltar à cidade" converts (MR-041)
		Campaigns: campaignsService,        // how the campaign levels (RN-09)
		Profiles:  users,                   // who gave each award
		Logger:    logger,
	})
	if err != nil {
		return nil, err
	}
	// Changing the XP mode asks for a confirmation when XP was already awarded (RN-09).
	campaignsService.SetXPAwards(progressionService)
	charactersService.SetLevelUps(progressionService)
	// The players' private notes (MR-030) read the scenes the group
	// discovered and the clues revealed to each player from the maps
	// module's tables, through SessionMaps.
	notesService, err = notes.New(notes.Config{
		Pool:   pool,
		Scenes: sessionMaps, // the discovered scenes and the received clues (MR-029, MR-030)
		Logger: logger,
	})
	if err != nil {
		return nil, err
	}
	// Every Set... call above must have happened: see the function's comment.
	for _, check := range []func() error{
		campaignsService.CheckWired, charactersService.CheckWired, playService.CheckWired,
		mapsService.CheckWired, sessionMaps.CheckWired,
	} {
		if err := check(); err != nil {
			return nil, fmt.Errorf("wiring the modules: %w", err)
		}
	}
	return &modules{
		users: users, campaigns: campaignsService, characters: charactersService, play: playService,
		maps: mapsService, progression: progressionService, notes: notesService,
	}, nil
}

// connectOptions are the options every Connect handler gets.
func connectOptions(logger *slog.Logger) []connect.HandlerOption {
	return []connect.HandlerOption{
		// First, so it is the outermost interceptor: it logs the final outcome
		// of every RPC and stream, after the session and authz interceptors
		// the services add have recorded the user and the campaign.
		connect.WithInterceptors(rpclog.Interceptor(logger)),
		// Right after rpclog, so it sits inside it: a handler that panics becomes
		// an `internal` error that rpclog still logs as an `rpc` line. Without
		// it, net/http only drops the stream, the client sees a network error
		// (which the app may retry) and no `rpc` line is written.
		connect.WithRecover(recoverRPC(logger)),
		// Each Send on a live stream gets streamSendTimeout to reach the client; a
		// reader that stopped reading ends its stream (client_gone) instead of
		// blocking the handler until Cloud Run closes the connection.
		connect.WithInterceptors(slowclient.Interceptor(streamSendTimeout)),
		connect.WithReadMaxBytes(maxRequestBytes),
		// Unary Connect requests must carry the Connect-Protocol-Version
		// header (or connect=v1 in a GET's query). A browser cannot add that
		// header to a cross-origin request without a CORS preflight, which
		// this server never grants: one more layer against CSRF on top of
		// SameSite cookies and http.CrossOriginProtection.
		connect.WithRequireConnectProtocolHeader(),
	}
}

// recoverRPC logs a handler's panic with its stack, through the request's
// logger (so the line carries the request id, user and campaign), and gives the
// client a fixed `internal` error: the panic value could hold anything.
func recoverRPC(logger *slog.Logger) func(context.Context, connect.Spec, http.Header, any) error {
	return func(ctx context.Context, spec connect.Spec, _ http.Header, panicValue any) error {
		logger.ErrorContext(ctx, "rpc handler panicked",
			slog.String("procedure", spec.Procedure),
			slog.Any("panic", panicValue),
			slog.String("stack", string(debug.Stack())))
		return connect.NewError(connect.CodeInternal, errors.New("internal error"))
	}
}

// diceModes adapts the campaigns service to play's DiceModes: play only needs
// to know what the campaign's setting forces on a player, not campaigns' own types.
type diceModes struct{ campaigns *campaigns.Service }

func (d diceModes) ForcedDice(ctx context.Context, tx pgx.Tx, campaignID, userID string) (play.DiceForce, error) {
	mode, err := d.campaigns.CampaignDiceMode(ctx, tx, campaignID, userID)
	switch mode {
	case campaigns.DiceModeApp:
		return play.DiceForcedInApp, err
	case campaigns.DiceModePhysical:
		return play.DiceForcedPhysical, err
	}
	return play.DiceChoice, err
}

// levelUpDice adapts the campaigns service to the characters module's
// DiceRules: the guided level-up only needs to know what the campaign's
// setting forces on a player, not campaigns' own types.
type levelUpDice struct{ campaigns *campaigns.Service }

func (d levelUpDice) LevelUpDice(ctx context.Context, tx pgx.Tx, campaignID, userID string) (charactersv1.LevelUpDiceRule, error) {
	mode, err := d.campaigns.CampaignDiceMode(ctx, tx, campaignID, userID)
	switch mode {
	case campaigns.DiceModeApp:
		return charactersv1.LevelUpDiceRule_LEVEL_UP_DICE_RULE_FORCED_IN_APP, err
	case campaigns.DiceModePhysical:
		return charactersv1.LevelUpDiceRule_LEVEL_UP_DICE_RULE_FORCED_PHYSICAL, err
	}
	return charactersv1.LevelUpDiceRule_LEVEL_UP_DICE_RULE_PLAYER_CHOOSES, err
}
