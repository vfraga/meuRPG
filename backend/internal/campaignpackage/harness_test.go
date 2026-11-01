package campaignpackage_test

import (
	"bytes"
	"context"
	"crypto/rand"
	"errors"
	"image"
	"image/color"
	"image/png"
	"io"
	"log/slog"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"strconv"
	"sync"
	"testing"
	"time"

	"connectrpc.com/connect"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	pkgv1 "github.com/PuraFome/meuRPG/backend/gen/meurpg/campaignpackage/v1"
	"github.com/PuraFome/meuRPG/backend/gen/meurpg/campaignpackage/v1/campaignpackagev1connect"
	campaignsv1 "github.com/PuraFome/meuRPG/backend/gen/meurpg/campaigns/v1"
	"github.com/PuraFome/meuRPG/backend/gen/meurpg/campaigns/v1/campaignsv1connect"
	"github.com/PuraFome/meuRPG/backend/gen/meurpg/characters/v1/charactersv1connect"
	mapsv1 "github.com/PuraFome/meuRPG/backend/gen/meurpg/maps/v1"
	"github.com/PuraFome/meuRPG/backend/gen/meurpg/maps/v1/mapsv1connect"
	"github.com/PuraFome/meuRPG/backend/gen/meurpg/notes/v1/notesv1connect"
	"github.com/PuraFome/meuRPG/backend/gen/meurpg/play/v1/playv1connect"
	"github.com/PuraFome/meuRPG/backend/gen/meurpg/rules/v1/rulesv1connect"
	"github.com/PuraFome/meuRPG/backend/internal/campaignpackage"
	"github.com/PuraFome/meuRPG/backend/internal/campaigns"
	"github.com/PuraFome/meuRPG/backend/internal/characters"
	"github.com/PuraFome/meuRPG/backend/internal/characters/contenttest"
	"github.com/PuraFome/meuRPG/backend/internal/identity"
	"github.com/PuraFome/meuRPG/backend/internal/maps"
	"github.com/PuraFome/meuRPG/backend/internal/notes"
	"github.com/PuraFome/meuRPG/backend/internal/platform/blob"
	"github.com/PuraFome/meuRPG/backend/internal/platform/choicetest"
	"github.com/PuraFome/meuRPG/backend/internal/platform/dbtest"
	"github.com/PuraFome/meuRPG/backend/internal/platform/dice/dicetest"
	"github.com/PuraFome/meuRPG/backend/internal/platform/httpserver"
	"github.com/PuraFome/meuRPG/backend/internal/platform/reqlimit"
	"github.com/PuraFome/meuRPG/backend/internal/play"
	"github.com/PuraFome/meuRPG/backend/internal/rules"
)

// The tests run against CockroachDB with the real campaigns, characters, maps
// and play services wired as cmd/api wires them, a blob store in a temporary
// folder, and the campaign package on top: set MEURPG_TEST_DATABASE_URL (see
// package dbtest). Without it they skip.

const testUserHeader = "Test-User-Id"

type testUserKey struct{}

func withTestUser(ctx context.Context, header http.Header) context.Context {
	if id := header.Get(testUserHeader); id != "" {
		return context.WithValue(ctx, testUserKey{}, id)
	}
	return ctx
}

type fakeSessions struct{}

func (fakeSessions) Interceptor() connect.Interceptor { return fakeSessionInterceptor{} }

func (fakeSessions) AuthenticateRequest(r *http.Request) (context.Context, error) {
	return withTestUser(r.Context(), r.Header), nil
}

func (fakeSessions) UserID(ctx context.Context) (string, error) {
	if id, ok := ctx.Value(testUserKey{}).(string); ok {
		return id, nil
	}
	return "", connect.NewError(connect.CodeUnauthenticated, errors.New("sign in to continue"))
}

func (f fakeSessions) RecheckSession(ctx context.Context) error {
	_, err := f.UserID(ctx)
	return err
}

type fakeSessionInterceptor struct{}

func (fakeSessionInterceptor) WrapUnary(next connect.UnaryFunc) connect.UnaryFunc {
	return func(ctx context.Context, req connect.AnyRequest) (connect.AnyResponse, error) {
		return next(withTestUser(ctx, req.Header()), req)
	}
}

func (fakeSessionInterceptor) WrapStreamingClient(next connect.StreamingClientFunc) connect.StreamingClientFunc {
	return next
}

func (fakeSessionInterceptor) WrapStreamingHandler(next connect.StreamingHandlerFunc) connect.StreamingHandlerFunc {
	return func(ctx context.Context, conn connect.StreamingHandlerConn) error {
		return next(withTestUser(ctx, conn.RequestHeader()), conn)
	}
}

type userTransport struct {
	userID string
	next   http.RoundTripper
}

func (t userTransport) RoundTrip(req *http.Request) (*http.Response, error) {
	req = req.Clone(req.Context())
	req.Header.Set(testUserHeader, t.userID)
	return t.next.RoundTrip(req)
}

var testRules = sync.OnceValues(rules.LoadSRD)

// fakeClock ticks a microsecond each time it is read, so rows made one after
// the other have different times.
type fakeClock struct {
	mu  sync.Mutex
	now time.Time
}

func (c *fakeClock) Now() time.Time {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.now = c.now.Add(time.Microsecond)
	return c.now
}

type testDice struct{ camps *campaigns.Service }

func (d testDice) ForcedDice(ctx context.Context, tx pgx.Tx, campaignID, userID string) (play.DiceForce, error) {
	mode, err := d.camps.CampaignDiceMode(ctx, tx, campaignID, userID)
	switch mode {
	case campaigns.DiceModeApp:
		return play.DiceForcedInApp, err
	case campaigns.DiceModePhysical:
		return play.DiceForcedPhysical, err
	}
	return play.DiceChoice, err
}

// options changes how the harness wires the package.
type options struct {
	maxCampaigns int
	configure    func(*campaignpackage.Config)
	noBlobs      bool
}

type harness struct {
	t       *testing.T
	pool    *pgxpool.Pool
	users   *identity.PostgresStore
	blobs   *blob.FS
	blobDir string
	server  *httptest.Server
	pkg     *campaignpackage.Service
	maps    *maps.Service
	clock   *fakeClock
}

func newHarness(t *testing.T, opts ...func(*options)) *harness {
	t.Helper()
	var o options
	for _, f := range opts {
		f(&o)
	}
	pool := dbtest.NewPool(t, "meurpg_package_test")
	h := &harness{t: t, pool: pool, users: identity.NewPostgresStore(pool), blobDir: t.TempDir()}
	blobs, err := blob.NewFS(h.blobDir)
	if err != nil {
		t.Fatalf("blob.NewFS() error = %v", err)
	}
	t.Cleanup(func() { _ = blobs.Close() })
	h.blobs = blobs
	h.clock = &fakeClock{now: time.Now().Truncate(time.Microsecond)}
	logger := slog.New(slog.DiscardHandler)

	camps, err := campaigns.New(campaigns.Config{Pool: pool, Profiles: h.users, Logger: logger, Now: h.clock.Now, MaxCampaignsPerUser: o.maxCampaigns})
	if err != nil {
		t.Fatalf("campaigns.New() error = %v", err)
	}
	srd, err := testRules()
	if err != nil {
		t.Fatalf("rules.LoadSRD() error = %v", err)
	}
	chars, err := characters.New(characters.Config{Pool: pool, Profiles: h.users, Members: camps, Content: contenttest.NewSource(pool, srd), SRD: srd, Roller: dicetest.New(), Logger: logger, Now: h.clock.Now})
	if err != nil {
		t.Fatalf("characters.New() error = %v", err)
	}
	camps.SetCharacters(chars)
	sessionMaps := maps.NewSessionMaps(pool)
	chars.SetGallery(sessionMaps)
	live, err := play.New(play.Config{
		Pool: pool, Sheets: chars, Vitals: chars, Campaigns: camps, Maps: sessionMaps, Roster: chars, Dice: testDice{camps},
		Live: play.LiveConfig{Heartbeat: time.Hour}, Roller: dicetest.New(), Logger: logger, Now: h.clock.Now,
	})
	if err != nil {
		t.Fatalf("play.New() error = %v", err)
	}
	var blobStore blob.Store = blobs
	if o.noBlobs {
		blobStore = nil
	}
	mapsSvc, err := maps.New(maps.Config{Pool: pool, Blobs: blobStore, Characters: chars, Live: live, Combats: live, Defaults: camps, Rules: srd, Logger: logger, Now: h.clock.Now})
	if err != nil {
		t.Fatalf("maps.New() error = %v", err)
	}
	h.maps = mapsSvc
	sessionMaps.SetService(mapsSvc)
	live.SetTerrain(mapsSvc)
	live.SetPuzzleMaps(mapsSvc)
	live.SetFog(mapsSvc)
	live.SetTraps(mapsSvc)
	mapsSvc.SetTrapFirer(live)

	cfg := campaignpackage.Config{
		Pool: pool, Blobs: blobStore, Creation: camps, Logger: logger, Now: h.clock.Now, ContentVersion: srd.Version(),
		Parts: []campaignpackage.Part{
			camps.PackagePart(), mapsSvc.PackageImagesPart(), chars.PackagePart(), mapsSvc.PackageMapsPart(), live.PackagePart(srd),
		},
	}
	if o.configure != nil {
		o.configure(&cfg)
	}
	pkg, err := campaignpackage.New(cfg)
	if err != nil {
		t.Fatalf("campaignpackage.New() error = %v", err)
	}
	h.pkg = pkg
	t.Cleanup(func() {
		ctx, cancel := context.WithTimeout(context.Background(), time.Minute)
		defer cancel()
		pkg.Cancel()
		pkg.Wait(ctx)
	})

	notesSvc, err := notes.New(notes.Config{Pool: pool, Scenes: sessionMaps, Logger: logger, Now: h.clock.Now})
	if err != nil {
		t.Fatalf("notes.New() error = %v", err)
	}
	srv := httpserver.New(httpserver.Config{Logger: logger})
	opt := reqlimit.HandlerOption()
	camps.Mount(srv.Handle, fakeSessions{}, opt)
	chars.Mount(srv.Handle, fakeSessions{}, camps, opt)
	live.Mount(srv.Handle, fakeSessions{}, camps, opt)
	mapsSvc.Mount(srv.Handle, fakeSessions{}, camps, opt)
	pkg.Mount(srv.Handle, fakeSessions{}, camps, opt)
	notesSvc.Mount(srv.Handle, fakeSessions{}, camps, opt)
	h.server = httptest.NewServer(srv.Handler())
	t.Cleanup(h.server.Close)
	t.Cleanup(live.Close)
	return h
}

type user struct {
	id         string
	h          *harness
	campaigns  campaignsv1connect.CampaignServiceClient
	document   campaignsv1connect.CampaignDocumentServiceClient
	characters charactersv1connect.CharacterServiceClient
	table      rulesv1connect.TableContentServiceClient
	gallery    mapsv1connect.GalleryServiceClient
	maps       mapsv1connect.MapServiceClient
	puzzles    playv1connect.PuzzleServiceClient
	play       playv1connect.PlayServiceClient
	encounters playv1connect.EncounterServiceClient
	pkg        campaignpackagev1connect.CampaignPackageServiceClient
	notes      notesv1connect.NotesServiceClient
}

func (h *harness) newUser(displayName string) *user {
	h.t.Helper()
	id, err := h.users.UpsertUser(h.t.Context(), identity.ExternalIdentity{Issuer: "https://idp.test", Subject: rand.Text()})
	if err != nil {
		h.t.Fatalf("UpsertUser() error = %v", err)
	}
	if err := h.users.SetDisplayName(h.t.Context(), id, displayName); err != nil {
		h.t.Fatalf("SetDisplayName() error = %v", err)
	}
	return h.clients(id)
}

func (h *harness) clients(userID string) *user {
	c, url := h.server.Client(), h.server.URL
	if userID != "" {
		c = &http.Client{Transport: userTransport{userID: userID, next: c.Transport}}
	}
	return &user{
		id: userID, h: h,
		campaigns:  campaignsv1connect.NewCampaignServiceClient(c, url),
		document:   campaignsv1connect.NewCampaignDocumentServiceClient(c, url),
		characters: charactersv1connect.NewCharacterServiceClient(c, url, connect.WithInterceptors(choicetest.Interceptor(charactersv1connect.NewCharacterServiceClient(c, url)))),
		table:      rulesv1connect.NewTableContentServiceClient(c, url),
		gallery:    mapsv1connect.NewGalleryServiceClient(c, url),
		maps:       mapsv1connect.NewMapServiceClient(c, url),
		puzzles:    playv1connect.NewPuzzleServiceClient(c, url),
		play:       playv1connect.NewPlayServiceClient(c, url),
		encounters: playv1connect.NewEncounterServiceClient(c, url),
		pkg:        campaignpackagev1connect.NewCampaignPackageServiceClient(c, url),
		notes:      notesv1connect.NewNotesServiceClient(c, url),
	}
}

// newCampaign creates a campaign whose master is master.
func (h *harness) newCampaign(master *user, name string) string {
	h.t.Helper()
	res, err := master.campaigns.CreateCampaign(h.t.Context(), connect.NewRequest(&campaignsv1.CreateCampaignRequest{Name: name, XpMode: campaignsv1.XpMode_XP_MODE_MILESTONES}))
	if err != nil {
		h.t.Fatalf("CreateCampaign() error = %v", err)
	}
	return res.Msg.GetCampaign().GetId()
}

// httpResult is the answer of a plain route.
type httpResult struct {
	status int
	header http.Header
	body   []byte
}

func (u *user) do(req *http.Request) httpResult {
	u.h.t.Helper()
	if u.id != "" {
		req.Header.Set(testUserHeader, u.id)
	}
	res, err := u.h.server.Client().Do(req)
	if err != nil {
		u.h.t.Fatalf("%s %s: %v", req.Method, req.URL.Path, err)
	}
	defer func() { _ = res.Body.Close() }()
	body, err := io.ReadAll(res.Body)
	if err != nil {
		u.h.t.Fatalf("read the answer: %v", err)
	}
	return httpResult{status: res.StatusCode, header: res.Header, body: body}
}

func (u *user) request(method, path string, body []byte) httpResult {
	u.h.t.Helper()
	req, err := http.NewRequestWithContext(u.h.t.Context(), method, u.h.server.URL+path, bytes.NewReader(body))
	if err != nil {
		u.h.t.Fatal(err)
	}
	return u.do(req)
}

// upload sends an image to the gallery and returns it.
func (u *user) upload(campaignID, fileName string, content []byte) *mapsv1.GalleryImage {
	u.h.t.Helper()
	var body bytes.Buffer
	w := multipart.NewWriter(&body)
	if err := w.WriteField("campaign_id", campaignID); err != nil {
		u.h.t.Fatal(err)
	}
	part, err := w.CreateFormFile("file", fileName)
	if err != nil {
		u.h.t.Fatal(err)
	}
	_, _ = part.Write(content)
	_ = w.Close()
	req, err := http.NewRequestWithContext(u.h.t.Context(), http.MethodPost, u.h.server.URL+maps.UploadPath, &body)
	if err != nil {
		u.h.t.Fatal(err)
	}
	req.Header.Set("Content-Type", w.FormDataContentType())
	res := u.do(req)
	if res.status != http.StatusCreated {
		u.h.t.Fatalf("upload %s: status %d: %s", fileName, res.status, res.body)
	}
	var img mapsv1.GalleryImage
	if err := protojsonUnmarshal(res.body, &img); err != nil {
		u.h.t.Fatalf("upload answer: %v", err)
	}
	return &img
}

// pngImage is a w x h PNG; seed changes its color so two images differ.
func pngImage(t *testing.T, w, h int, seed byte) []byte {
	t.Helper()
	img := image.NewNRGBA(image.Rect(0, 0, w, h))
	for y := range h {
		for x := range w {
			img.SetNRGBA(x, y, color.NRGBA{R: seed, G: byte(x * 255 / max(w, 1)), B: byte(y * 255 / max(h, 1)), A: 255}) //nolint:gosec // G115: 0 to 255
		}
	}
	var buf bytes.Buffer
	if err := png.Encode(&buf, img); err != nil {
		t.Fatal(err)
	}
	return buf.Bytes()
}

// export runs an export to its end and returns the zip.
func (u *user) export(campaignID string) []byte {
	u.h.t.Helper()
	ctx := u.h.t.Context()
	started, err := u.pkg.StartCampaignExport(ctx, connect.NewRequest(&pkgv1.StartCampaignExportRequest{CampaignId: campaignID, IdempotencyKey: rand.Text()}))
	if err != nil {
		u.h.t.Fatalf("StartCampaignExport() error = %v", err)
	}
	deadline := time.Now().Add(2 * time.Minute)
	for {
		got, err := u.pkg.GetCampaignExport(ctx, connect.NewRequest(&pkgv1.GetCampaignExportRequest{CampaignId: campaignID}))
		if err != nil {
			u.h.t.Fatalf("GetCampaignExport() error = %v", err)
		}
		e := got.Msg.GetExport()
		switch e.GetState() {
		case pkgv1.CampaignExportState_CAMPAIGN_EXPORT_STATE_DONE:
			res := u.request(http.MethodGet, e.GetDownloadPath(), nil)
			if res.status != http.StatusOK {
				u.h.t.Fatalf("download: status %d: %s", res.status, res.body)
			}
			_ = started
			return res.body
		case pkgv1.CampaignExportState_CAMPAIGN_EXPORT_STATE_FAILED, pkgv1.CampaignExportState_CAMPAIGN_EXPORT_STATE_CANCELED:
			u.h.t.Fatalf("the export ended as %v (failure %v)", e.GetState(), e.GetFailure())
		}
		if time.Now().After(deadline) {
			u.h.t.Fatal("the export did not finish")
		}
		time.Sleep(20 * time.Millisecond)
	}
}

// send uploads data in parts (the way the app does) and returns the upload.
func (u *user) send(name string, data []byte) *pkgv1.CampaignImport {
	u.h.t.Helper()
	ctx := u.h.t.Context()
	begun, err := u.pkg.BeginCampaignImport(ctx, connect.NewRequest(&pkgv1.BeginCampaignImportRequest{
		FileName: name, TotalBytes: int64(len(data)), Fingerprint: name + ":" + strconv.Itoa(len(data)),
	}))
	if err != nil {
		u.h.t.Fatalf("BeginCampaignImport() error = %v", err)
	}
	up := begun.Msg.GetUpload()
	size := int(up.GetPartSize())
	for n := 1; n <= int(up.GetPartCount()); n++ {
		lo, hi := (n-1)*size, min(n*size, len(data))
		res := u.request(http.MethodPut, campaignpackage.PartsPath+up.GetId()+"/parts/"+strconv.Itoa(n), data[lo:hi])
		if res.status != http.StatusNoContent {
			u.h.t.Fatalf("part %d: status %d: %s", n, res.status, res.body)
		}
	}
	return up
}

// importPackage sends data and creates the campaign, failing the test on
// any problem.
func (u *user) importPackage(name string, data []byte) *pkgv1.CreateCampaignFromImportResponse {
	u.h.t.Helper()
	up := u.send(name, data)
	ctx := u.h.t.Context()
	prev, err := u.pkg.PreviewCampaignImport(ctx, connect.NewRequest(&pkgv1.PreviewCampaignImportRequest{ImportId: up.GetId()}))
	if err != nil {
		u.h.t.Fatalf("PreviewCampaignImport() error = %v", err)
	}
	if len(prev.Msg.GetProblems()) > 0 {
		u.h.t.Fatalf("the preview has problems: %v", prev.Msg.GetProblems())
	}
	res, err := u.pkg.CreateCampaignFromImport(ctx, connect.NewRequest(&pkgv1.CreateCampaignFromImportRequest{ImportId: up.GetId(), IdempotencyKey: rand.Text()}))
	if err != nil {
		u.h.t.Fatalf("CreateCampaignFromImport() error = %v", err)
	}
	return res.Msg
}

// advance moves the clock forward (the package keeps things for hours).
func (c *fakeClock) advance(d time.Duration) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.now = c.now.Add(d)
}

// smallPackage is the package of a small campaign of the user's own.
func (h *harness) smallPackage(u *user) []byte {
	h.t.Helper()
	return u.export(h.smallCampaign(u))
}

// invite lets the players in through an invite from the master.
func (h *harness) invite(master *user, campaignID string, players ...*user) {
	h.t.Helper()
	ctx := h.t.Context()
	inv := must(master.campaigns.CreateInvite(ctx, connect.NewRequest(&campaignsv1.CreateInviteRequest{CampaignId: campaignID, MaxUses: campaigns.MaxInviteUses})))
	for _, p := range players {
		_ = must(p.campaigns.AcceptInvite(ctx, connect.NewRequest(&campaignsv1.AcceptInviteRequest{Token: inv.GetToken()})))
	}
}

// creationGate is the campaigns service as the package sees it, with the early checks
// (the cap and the creators list, made before a byte is sent) switched off while skip
// is true: a test of the check that the transaction makes at the end needs to get past them.
type creationGate struct {
	campaignpackage.Creation
	skip *bool
}

func (g creationGate) CheckCreation(ctx context.Context, userID string) error {
	if *g.skip {
		return nil
	}
	return g.Creation.CheckCreation(ctx, userID)
}

// pngNoise is a PNG of random pixels: it does not compress, so a few of them fill several parts.
func pngNoise(t *testing.T, w, h int, seed byte) []byte {
	t.Helper()
	img := image.NewNRGBA(image.Rect(0, 0, w, h))
	buf := make([]byte, len(img.Pix))
	if _, err := rand.Read(buf); err != nil {
		t.Fatal(err)
	}
	copy(img.Pix, buf)
	for i := 3; i < len(img.Pix); i += 4 {
		img.Pix[i] = 255
	}
	img.Pix[0] = seed
	var out bytes.Buffer
	if err := png.Encode(&out, img); err != nil {
		t.Fatal(err)
	}
	return out.Bytes()
}
