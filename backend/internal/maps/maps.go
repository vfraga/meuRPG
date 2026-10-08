// Package maps is about a campaign's maps and the images they are made of:
// the gallery (MR-019), the images the master uploads to use in maps and in
// the campaign document, and the maps themselves (MR-008, MR-009, MR-012),
// with their points of interest and tokens (MapService, mapservice.go).
//
// Who sees what on a map (RN-10) is decided here, on the server
// (visibility.go): a player never receives a hidden map, point or token,
// not even its ID. Two other modules help, each through a small interface
// that cmd/api connects, so no package imports another's code:
//   - the characters module says which characters may stand on a map as
//     tokens (CharacterDirectory);
//   - the play module says which map is the open session's current one,
//     which the players see too, and which gallery image it shows, and
//     carries the maps' changes to the members watching the session
//     (LiveSession). The other way round, play reveals the map it makes
//     current, and reads the image it shows, through SessionMaps.
//
// An image travels like this:
//
//   - The master uploads it with POST /uploads/images (upload.go), a plain
//     HTTP route, because its body is a file. Package images checks it and
//     encodes it again, which drops every piece of metadata. The image and
//     its thumbnail go to the blob store (package platform/blob), then a
//     gallery_images row records them, inside the transaction that checks
//     the campaign's quota.
//   - The campaign's master fetches it with GET /images/{id} (serve.go); a
//     player too, but only while they see it: on a map they see, or shown
//     in the session (RN-10). Knowing its ID is not enough.
//   - The master lists, renames and deletes images with GalleryService
//     (gallery.go), a Connect service like the others.
//
// The SQL lives in queries.sql, and sqlc turns it into package mapsdb.
// Every write runs inside db.InTx, which retries CockroachDB's serialization
// errors (40001).
package maps

import (
	"context"
	"errors"
	"log/slog"
	"net/http"
	"slices"
	"sync"
	"sync/atomic"
	"time"

	"connectrpc.com/connect"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	charactersv1 "github.com/PuraFome/meuRPG/backend/gen/meurpg/characters/v1"
	"github.com/PuraFome/meuRPG/backend/gen/meurpg/maps/v1/mapsv1connect"
	playv1 "github.com/PuraFome/meuRPG/backend/gen/meurpg/play/v1"
	"github.com/PuraFome/meuRPG/backend/internal/authz"
	"github.com/PuraFome/meuRPG/backend/internal/maps/images/gen"
	"github.com/PuraFome/meuRPG/backend/internal/maps/link"
	"github.com/PuraFome/meuRPG/backend/internal/maps/mapsdb"
	"github.com/PuraFome/meuRPG/backend/internal/platform/blob"
	"github.com/PuraFome/meuRPG/backend/internal/platform/nostore"
	"github.com/PuraFome/meuRPG/backend/internal/platform/ratelimit"
	"github.com/PuraFome/meuRPG/backend/internal/platform/wiring"
	"github.com/PuraFome/meuRPG/backend/internal/rules"
)

// A campaign's gallery limits (a proposal, question 30 of the progress
// doc). The per-image limits are in package images.
const (
	// DefaultMaxImages is how many images a campaign may have.
	DefaultMaxImages = 300
	// DefaultMaxBytes is how many bytes its images may add up to, as
	// stored: 500 MiB.
	DefaultMaxBytes = 500 << 20
)

// The HTTP routes, next to the Connect service.
const (
	// UploadPath receives uploads (POST).
	UploadPath = "/uploads/images"
	// ImagesPath serves images (GET ImagesPath+id, and +"/thumb").
	ImagesPath = "/images/"
)

// maxNameLength is the longest image, map or point name, in characters.
// The gallery_images_name_length, maps_name_length and
// map_points_name_length CHECKs say the same.
const maxNameLength = 80

// CharacterDirectory tells which characters may stand on a map as tokens.
// The characters module implements it (characters.Service.MapCharacters),
// so this package never reads the characters table. The reads take the
// caller's transaction (nil: the pool): a caller that holds one must pass it,
// or the read takes a second connection while the transaction keeps the first.
type CharacterDirectory interface {
	// MapCharacters returns those of ids that are living characters of the
	// campaign (a player's that is neither dead nor waiting for approval, or
	// an NPC), players' characters first, then NPCs, each group oldest
	// first. Only id, kind, name and player_user_id are set.
	MapCharacters(ctx context.Context, tx pgx.Tx, campaignID string, ids []string) ([]*charactersv1.CharacterSummary, error)
	// ClearPortraits takes the image off the portrait of every NPC of the
	// campaign that has it, inside tx, and returns how many it cleared
	// (MR-031): the master deleted the image from the gallery.
	ClearPortraits(ctx context.Context, tx pgx.Tx, campaignID, imageID string) (int64, error)
	// PartyVision returns the campaign's living player characters that have a
	// player, each with what it sees with (its derived darkvision, blindsight
	// and truesight): the fog of war's viewers (MR-036). A beast's senses in Wild
	// Shape replace the character's, and a familiar the player looks through comes
	// as Eyes (MR-037).
	PartyVision(ctx context.Context, tx pgx.Tx, campaignID string) ([]link.PartyMember, error)
	// MapCreatures returns those of ids that are live creatures of a living
	// player's character of the campaign, oldest first: the ones that may have a
	// token on a map (MR-037).
	MapCreatures(ctx context.Context, campaignID string, ids []string) ([]link.MapCreature, error)
	// PortraitInUse says whether any NPC of the campaign has the gallery image as
	// its portrait.
	PortraitInUse(ctx context.Context, campaignID, imageID string) (bool, error)
	// NpcPortraits returns, for those of ids that are living NPCs of the campaign,
	// the gallery image of each one's portrait ("" for none), by character ID: the
	// portraits of the NPCs a picture made from a map shows go to the image model
	// as character references (MR-039).
	NpcPortraits(ctx context.Context, tx pgx.Tx, campaignID string, ids []string) (map[string]string, error)
	// PartyTotalLevels returns the total level of each living, active player character
	// of the campaign (1 to 20), oldest first: the treasure generator's party level is
	// the lowest of them (MR-044).
	PartyTotalLevels(ctx context.Context, tx pgx.Tx, campaignID string) ([]int, error)
}

// LiveSession is what this package needs from the live session. The play
// module implements it (play.Service), because the sessions and their
// stream are its own. The events are play's API messages (playv1), as the
// vitals are for the characters module: this package builds them, and
// never imports package play.
type LiveSession interface {
	// OnScreen returns what the campaign's open game session shows: the
	// IDs of its current map, which a player sees even if it is hidden
	// (RN-10), and of the gallery image the master shows the players
	// (MR-028). Each is "" when there is none, both when no session is
	// open.
	OnScreen(ctx context.Context, campaignID string) (currentMapID, shownImageID string, err error)
	// Publish sends ev to the streams of the campaign's master and, when
	// players is true, of its players too. Without an open session nobody
	// is watching, and nothing happens.
	Publish(campaignID string, players bool, ev *playv1.WatchGameSessionResponse)
	// OpenScenePoint returns the map point of the RP scene open in the
	// campaign's open game session (MR-015), "" when none is open or no
	// session is.
	OpenScenePoint(ctx context.Context, campaignID string) (pointID string, err error)
	// ImageOnStage reports whether the image is the portrait of an NPC on the
	// stage of the open scene of the campaign's open game session (MR-031).
	// False when no session or no scene is open.
	ImageOnStage(ctx context.Context, campaignID, imageID string) (bool, error)
	// ImageShown reports whether the image is the one the open session shows the
	// players. It reads through tx, for a transaction that is about to delete it.
	ImageShown(ctx context.Context, tx pgx.Tx, campaignID, imageID string) (bool, error)
	// PublishToUsers sends ev to the streams of those of userIDs who watch
	// the campaign's session, and to nobody else: not the master, not the
	// other players. Without an open session nothing happens.
	PublishToUsers(campaignID string, userIDs []string, ev *playv1.WatchGameSessionResponse)
	// PublishToUsersCoalesced is PublishToUsers for a hint that says "read it
	// again" (`vision_changed`): while a user's stream still has an event with the
	// same key waiting in its queue, no other is queued for it.
	PublishToUsersCoalesced(campaignID string, userIDs []string, key string, ev *playv1.WatchGameSessionResponse)
	// AppendEvent appends an event to the history of the campaign's open
	// session inside tx, and says whether there was one. The payload holds
	// IDs only (docs/privacy.md).
	AppendEvent(ctx context.Context, tx pgx.Tx, campaignID, kind, actorUserID string, payload []byte, at time.Time) (bool, error)
	// OpenSessionID returns the ID of the campaign's open game session, locking
	// its row inside tx, or "" when none is open: a treasure found remembers it
	// (MR-041), so the session's summary counts the treasure.
	OpenSessionID(ctx context.Context, tx pgx.Tx, campaignID string) (string, error)
}

// CombatMaps says whether a combat is running on a map, and where its combatants
// stand. The play module implements it (play.Service.CombatRunsOnMap,
// CombatPositions): a map's grid and image cannot change while a fight stands on
// its painted layers (MR-034, D2), and the fog of war sees from the combatant's
// square while it runs (MR-036, D6).
type CombatMaps interface {
	// CombatPositions says where the combatants of the combat running on the map
	// stand, by character, in squares of its grid, and whether a combat runs.
	CombatPositions(ctx context.Context, tx pgx.Tx, campaignID, mapID string) (link.CombatPositions, error)
	// CombatRunsOnMap reports, inside tx, whether a combat of the campaign that
	// is not ended (in setup or active) runs on the map.
	CombatRunsOnMap(ctx context.Context, tx pgx.Tx, campaignID, mapID string) (bool, error)
}

// MapDefaults says what a table chose for the maps it creates (RN-24). The
// campaigns module implements it (campaigns.Service.FogOnNewMaps); cmd/api
// connects the two, and neither package imports the other.
type MapDefaults interface {
	// FogOnNewMaps says whether a map created now starts with the fog of war on.
	// It reads inside tx when the caller has one (nil: the pool).
	FogOnNewMaps(ctx context.Context, tx pgx.Tx, campaignID string) (bool, error)
}

// Rules is what this package needs from the rules content: the scene checks
// (MR-015), and the names and presets that keep a trap's or a light's keys honest
// (MR-035, MR-036). *rules.Content implements it, so nothing here repeats the
// catalog.
type Rules interface {
	SceneChecks
	// NamePT is the Portuguese name of a content key, "" for an unknown one.
	NamePT(key string) string
	// TrapPreset and LightPreset find a preset by its key.
	TrapPreset(key string) (rules.TrapPreset, bool)
	LightPreset(key string) (rules.LightPreset, bool)
	// GenerateTreasure, MagicItem and MagicItemValue are the
	// treasure generator (MR-044, TreasureService): the treasure of a mode, party level
	// and seed, a magic item with its text, and its SRD 5.2.1 value.
	GenerateTreasure(mode string, level int, seed uint64) (rules.Treasure, error)
	MagicItem(key string) (rules.MagicItem, bool)
	MagicItemValue(key string) (rules.ItemValue, bool)
	// Version is the content version a treasure is rolled under.
	Version() string
}

// Config holds what the maps service needs.
type Config struct {
	// Pool is the CockroachDB connection pool. Required.
	Pool *pgxpool.Pool
	// Blobs stores the image files. Nil means images are off on this
	// server: uploads, downloads and GalleryService answer `unavailable`.
	// MapService works without it, but no image can be uploaded, so no map
	// can be created.
	Blobs blob.Store
	// Characters says which characters may stand on a map. Required.
	Characters CharacterDirectory
	// Live is the live session: the current map, and where map changes
	// go. Required.
	Live LiveSession
	// Rules says which checks a scene may ask for (MR-015) and which traps,
	// lights, damage types and conditions exist: *rules.Content. Required.
	Rules Rules
	// Combats says whether a combat runs on a map: the play service. Required.
	Combats CombatMaps
	// Defaults says what the table chose for new maps: the campaigns service.
	// Optional; nil means a new map has no fog, as before the table's rules.
	Defaults MapDefaults
	// Logger receives errors, without personal data. Nil means
	// slog.Default().
	Logger *slog.Logger
	// Now returns the current time. Nil means time.Now.
	Now func() time.Time
	// MaxImages and MaxBytes are a campaign's gallery quota. Zero means
	// DefaultMaxImages and DefaultMaxBytes; tests set small ones.
	MaxImages int32
	MaxBytes  int32
	// MaxMaps and MaxPointsPerMap bound a campaign's maps and a map's
	// points. Zero means DefaultMaxMaps and DefaultMaxPointsPerMap.
	MaxMaps         int32
	MaxPointsPerMap int32
	// Generator makes the pictures of ImageGenerationService (MR-039, RN-28). Nil
	// means generation is off: the service answers failed_precondition with
	// reason OFF, and the rest works. It also needs Blobs.
	Generator gen.Generator
	// MonthlyImages is how many images a campaign may generate per month. Zero
	// means DefaultMonthlyImages.
	MonthlyImages int32
	// DailyImages is how many images the whole server may generate per day,
	// whichever the campaign: a ceiling on the Gemini bill on top of the monthly
	// limit. Zero means DefaultDailyImages.
	DailyImages int32
	// DownloadLimit and UploadLimit limit, per user, the image, thumbnail and
	// tile downloads and the uploads (platform/ratelimit.Policy). Nil means no
	// limit (tests).
	DownloadLimit, UploadLimit *ratelimit.Limiter
}

// Service implements GalleryService, MapService, DungeonService, TreasureService and the image routes.
type Service struct {
	pool       *pgxpool.Pool
	queries    *mapsdb.Queries
	blobs      blob.Store
	characters CharacterDirectory
	live       LiveSession
	checks     SceneChecks
	rules      Rules
	combats    CombatMaps
	defaults   MapDefaults
	firer      TrapFirer
	layerHints hintGate
	// layerHintEvery is the gate's interval (defaultLayerHintEvery); a test sets
	// it on its own service to see every hint at once.
	layerHintEvery time.Duration

	// The fog of war (fog.go): the compiled scenes, the revision each player was
	// last told, and the lock that makes a refresh and "Esquecer o que foi visto"
	// take turns, so the memory only ever grows from what was seen.
	lits        litCache
	seen        visionState
	visionLocks visionLocks
	counts      pointCounts
	logger      *slog.Logger
	now         func() time.Time
	maxImages   int32
	maxBytes    int32
	maxMaps     int32
	maxPoints   int32

	// dungeonGate bounds the dungeons generated at once, and dungeonLimit the
	// dungeons a campaign creates or redraws (dungeons.go).
	dungeonGate chan struct{}
	previews    campaignSlots
	// beforeRedrawTx, when set, runs after a redraw drew its image and before its
	// transaction: tests use it to change the map in that window.
	beforeRedrawTx func()
	dungeonLimit   *ratelimit.Limiter

	// processing lets one image at a time be decoded, so a few uploads at
	// once cannot take all the server's memory (package images bounds
	// what one image may use).
	processing chan struct{}

	// tiles is the image of a fog map as tiles per player (tiles.go).
	tiles *tileRenderer

	// routeLimits are the per-user limits of the image routes (ratelimit.go).
	routeLimits routeLimits

	// The generated images (imagegen.go): the model behind them (nil: off), the
	// month's cap per campaign, the calls in flight at once, and the goroutines
	// to wait for at shutdown and at the end of a test.
	generator     gen.Generator
	monthlyImages int32
	dailyImages   int32
	generating    chan struct{}
	generations   sync.WaitGroup
	// pending counts the image requests alive: the one calling the model and the ones
	// waiting for its slot (maxPendingRequests).
	pending atomic.Int32
	// baseCtx is what the generation goroutines derive from; CancelGenerations
	// cancels it at shutdown. waiters wakes the long polls of GetImageGeneration.
	baseCtx    context.Context
	cancelBase context.CancelFunc
	waiters    generationWaiters
	// onReferenceDecode, when set (a test), is called each time an original is
	// decoded to make a reference.
	onReferenceDecode func()
}

// queriesIn is q on the transaction, or q itself (the pool) when tx is nil. A
// read made while the caller holds a transaction must use the transaction: a
// read through the pool takes a second connection (see docs/architecture.md,
// "Dentro de uma transação, nenhuma leitura pelo pool").
func queriesIn(q *mapsdb.Queries, tx pgx.Tx) *mapsdb.Queries {
	if tx == nil {
		return q
	}
	return q.WithTx(tx)
}

// The compiler checks that Service implements both handlers.
var (
	_ mapsv1connect.GalleryServiceHandler = (*Service)(nil)
	_ mapsv1connect.MapServiceHandler     = (*Service)(nil)

	_ mapsv1connect.ImageGenerationServiceHandler = (*Service)(nil)
	_ mapsv1connect.DungeonServiceHandler         = (*Service)(nil)
)

// New returns a Service.
func New(cfg Config) (*Service, error) {
	switch {
	case cfg.Pool == nil:
		return nil, errors.New("maps: a Pool is required")
	case cfg.Characters == nil:
		return nil, errors.New("maps: Characters is required")
	case cfg.Live == nil:
		return nil, errors.New("maps: Live is required")
	case cfg.Rules == nil:
		return nil, errors.New("maps: Rules is required")
	case cfg.Combats == nil:
		return nil, errors.New("maps: Combats is required")
	}
	s := &Service{
		layerHintEvery: defaultLayerHintEvery,
		pool:           cfg.Pool,
		queries:        mapsdb.New(cfg.Pool),
		blobs:          cfg.Blobs,
		characters:     cfg.Characters,
		live:           cfg.Live,
		checks:         cfg.Rules,
		rules:          cfg.Rules,
		combats:        cfg.Combats,
		defaults:       cfg.Defaults,
		logger:         cfg.Logger,
		now:            cfg.Now,
		maxImages:      cfg.MaxImages,
		maxBytes:       cfg.MaxBytes,
		maxMaps:        cfg.MaxMaps,
		maxPoints:      cfg.MaxPointsPerMap,
		processing:     make(chan struct{}, 1),
		dungeonGate:    make(chan struct{}, dungeonGenerators),
		dungeonLimit:   newDungeonLimiter(),
		tiles:          newTileRenderer(),
		generator:      cfg.Generator,
		monthlyImages:  cfg.MonthlyImages,
		dailyImages:    cfg.DailyImages,
		routeLimits:    newRouteLimits(cfg.DownloadLimit, cfg.UploadLimit, cfg.Logger),
		generating:     make(chan struct{}, maxGenerating),
	}
	if s.monthlyImages <= 0 {
		s.monthlyImages = DefaultMonthlyImages
	}
	if s.dailyImages <= 0 {
		s.dailyImages = DefaultDailyImages
	}
	s.baseCtx, s.cancelBase = context.WithCancel(context.Background())
	if s.logger == nil {
		s.logger = slog.Default()
	}
	if s.now == nil {
		s.now = time.Now
	}
	s.layerHints.logger = s.logger
	if s.maxImages <= 0 {
		s.maxImages = DefaultMaxImages
	}
	if s.maxBytes <= 0 {
		s.maxBytes = DefaultMaxBytes
	}
	if s.maxMaps <= 0 {
		s.maxMaps = DefaultMaxMaps
	}
	if s.maxPoints <= 0 {
		s.maxPoints = DefaultMaxPointsPerMap
	}
	return s, nil
}

// Sessions is what this package needs to know who is calling: the Connect
// interceptor and its twin for plain HTTP routes, which both find the
// caller's session, and the authz.Caller that reads it back.
// *identity.Service is the real one; tests pass a fake.
type Sessions interface {
	// Interceptor finds the caller's session (from the session cookie), for
	// the Connect service.
	Interceptor() connect.Interceptor
	// AuthenticateRequest does the same for a plain HTTP request: it
	// returns the request's context with the caller's session in it.
	AuthenticateRequest(r *http.Request) (context.Context, error)
	authz.Caller
}

// Mount registers GalleryService, MapService and the image routes on a mux. handle is
// usually httpserver.Server.Handle or http.ServeMux.Handle.
//
// sessions tells who is calling (the identity service in production), and
// members tells each caller's role in a campaign (the campaigns service).
// The Connect service gets, in this order, an interceptor that marks every
// response `Cache-Control: no-store`, the sessions interceptor, and the
// authz interceptor; the HTTP routes get the same two checks as
// middleware. opts are the Connect options shared by every service.
func (s *Service) Mount(handle func(pattern string, handler http.Handler), sessions Sessions, members authz.MembershipSource, opts ...connect.HandlerOption) {
	// Clip so append copies instead of writing into the caller's array.
	opts = append(slices.Clip(opts), connect.WithInterceptors(
		nostore.Interceptor(),                          // no response is cacheable
		sessions.Interceptor(),                         // who is calling
		authz.Interceptor(sessions, members, s.logger), // what they may do, memoized per request
	))
	handle(mapsv1connect.NewGalleryServiceHandler(s, opts...))
	handle(mapsv1connect.NewMapServiceHandler(s, opts...))
	handle(mapsv1connect.NewImageGenerationServiceHandler(s, opts...))
	handle(mapsv1connect.NewDungeonServiceHandler(s, opts...))
	handle(mapsv1connect.NewTreasureServiceHandler(s, opts...))

	withAuthz := authz.Middleware(sessions, members, s.logger)
	// limit is the per-user rate limit: after the session, which tells the user,
	// and before the authorization, which reads the database.
	route := func(limit *routeLimit, h http.HandlerFunc) http.Handler {
		return s.imagesOn(s.withSession(sessions, s.limitUser(sessions, limit, withAuthz(h))))
	}
	handle("POST "+UploadPath, route(s.routeLimits.upload, s.handleUpload))
	handle("GET "+ImagesPath+"{id}", route(s.routeLimits.download, s.handleImage))
	handle("GET "+ImagesPath+"{id}/thumb", route(s.routeLimits.download, s.handleThumbnail))
	handle("GET "+TilesPath+"{map}/tiles/{tx}/{ty}", route(s.routeLimits.download, s.handleTile))
}

// imagesOn answers 503 while images are off (no blob store), before
// anything else: there is nothing to check a session for.
func (s *Service) imagesOn(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if s.blobs == nil {
			s.writeError(w, r, errImagesOff())
			return
		}
		next.ServeHTTP(w, r)
	})
}

// withSession finds the caller's session, like the Connect interceptor.
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

// imageURL and thumbnailURL are where an image and its thumbnail are
// served.
func imageURL(id string) string     { return ImagesPath + id }
func thumbnailURL(id string) string { return ImagesPath + id + "/thumb" }

// blobKeys are the keys of an image's files in the blob store. The
// campaign comes first, so all of a campaign's files share a prefix.
func blobKeys(campaignID, imageID string) (image, thumbnail string) {
	image = "campaigns/" + campaignID + "/images/" + imageID
	return image, image + ".thumb"
}

// referenceKey is the key of the image's reference, the small JPEG it travels
// as to the image model (images.ReferenceSide). Only an image bigger than that
// has one; it is made at upload, or the first time the image is used (older
// images), and deleted with the image.
func referenceKey(campaignID, imageID string) string {
	image, _ := blobKeys(campaignID, imageID)
	return image + ".ref"
}

// deleteFiles deletes an image's files, for an image whose row is gone or
// was never written. It goes on even if the request was canceled, and a
// failure only leaves unreachable files behind, so it is logged, not
// returned.
func (s *Service) deleteFiles(ctx context.Context, campaignID, imageID string) {
	ctx = context.WithoutCancel(ctx)
	image, thumbnail := blobKeys(campaignID, imageID)
	for _, key := range []string{image, thumbnail, referenceKey(campaignID, imageID)} {
		if err := s.blobs.Delete(ctx, key); err != nil {
			s.logger.ErrorContext(ctx, "maps: cannot delete an image file; it is left behind", "error", err)
		}
	}
}

// errImagesOff is the answer while no blob store is configured.
func errImagesOff() error {
	return connect.NewError(connect.CodeUnavailable, errors.New("images are not configured on this server"))
}

// CheckWired fails when a collaborator that cmd/api connects after New is
// still nil (see platform/wiring).
func (s *Service) CheckWired() error {
	return wiring.Check("maps", wiring.Dep{Setter: "SetTrapFirer", Missing: s.firer == nil})
}
