package maps

import (
	"bytes"
	"context"
	"crypto/rand"
	"encoding/binary"
	"encoding/json"
	"errors"
	"image"
	"image/color"
	"image/jpeg"
	"image/png"
	"io"
	"io/fs"
	"log/slog"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"connectrpc.com/connect"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"google.golang.org/protobuf/encoding/protojson"

	campaignsv1 "github.com/PuraFome/meuRPG/backend/gen/meurpg/campaigns/v1"
	"github.com/PuraFome/meuRPG/backend/gen/meurpg/campaigns/v1/campaignsv1connect"
	"github.com/PuraFome/meuRPG/backend/gen/meurpg/characters/v1/charactersv1connect"
	mapsv1 "github.com/PuraFome/meuRPG/backend/gen/meurpg/maps/v1"
	"github.com/PuraFome/meuRPG/backend/gen/meurpg/maps/v1/mapsv1connect"
	"github.com/PuraFome/meuRPG/backend/gen/meurpg/notes/v1/notesv1connect"
	"github.com/PuraFome/meuRPG/backend/gen/meurpg/play/v1/playv1connect"
	"github.com/PuraFome/meuRPG/backend/gen/meurpg/progression/v1/progressionv1connect"
	"github.com/PuraFome/meuRPG/backend/gen/meurpg/rules/v1/rulesv1connect"
	"github.com/PuraFome/meuRPG/backend/internal/campaigns"
	"github.com/PuraFome/meuRPG/backend/internal/characters"
	"github.com/PuraFome/meuRPG/backend/internal/characters/contenttest"
	"github.com/PuraFome/meuRPG/backend/internal/identity"
	"github.com/PuraFome/meuRPG/backend/internal/maps/mapsdb"
	"github.com/PuraFome/meuRPG/backend/internal/notes"
	"github.com/PuraFome/meuRPG/backend/internal/platform/blob"
	"github.com/PuraFome/meuRPG/backend/internal/platform/choicetest"
	"github.com/PuraFome/meuRPG/backend/internal/platform/db"
	"github.com/PuraFome/meuRPG/backend/internal/platform/dbtest"
	"github.com/PuraFome/meuRPG/backend/internal/platform/httpserver"
	"github.com/PuraFome/meuRPG/backend/internal/play"
	"github.com/PuraFome/meuRPG/backend/internal/progression"
	"github.com/PuraFome/meuRPG/backend/internal/rules"
)

// The database tests run against CockroachDB, with the real campaigns,
// characters and play services next to this one, as cmd/api wires them,
// and a blob store in a temporary folder: set MEURPG_TEST_DATABASE_URL (see
// package dbtest). Without it they skip.
//
// The services are mounted on package httpserver's server, the one
// cmd/api runs, so http.CrossOriginProtection (CSRF) is in front of the
// routes, as in production.

// testUserHeader names the signed-in user in a test request. fakeSessions
// trusts it; production code has no way to set a caller (see
// docs/architecture/identity.md#who-is-calling).
const testUserHeader = "Test-User-Id"

type fakeSessions struct{}

type testUserKey struct{}

var testSessions Sessions = fakeSessions{}

func withTestUser(ctx context.Context, header http.Header) context.Context {
	if userID := header.Get(testUserHeader); userID != "" {
		return context.WithValue(ctx, testUserKey{}, userID)
	}
	return ctx
}

// Interceptor reads the test user, for calls and streams alike.
func (fakeSessions) Interceptor() connect.Interceptor { return fakeSessionInterceptor{} }

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

// RecheckSession is what the live stream calls from time to time; the test
// sessions never end.
func (f fakeSessions) RecheckSession(ctx context.Context) error {
	_, err := f.UserID(ctx)
	return err
}

// userTransport adds the test user header to every request, calls and
// streams alike.
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

func (fakeSessions) AuthenticateRequest(r *http.Request) (context.Context, error) {
	return withTestUser(r.Context(), r.Header), nil
}

func (fakeSessions) UserID(ctx context.Context) (string, error) {
	if userID, ok := ctx.Value(testUserKey{}).(string); ok {
		return userID, nil
	}
	return "", connect.NewError(connect.CodeUnauthenticated, errors.New("sign in to continue"))
}

// fakeClock ticks one microsecond each time it is read (CockroachDB keeps
// microseconds), so images uploaded one after the other have different
// times.
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

type harness struct {
	t       *testing.T
	pool    *pgxpool.Pool
	users   *identity.PostgresStore
	blobDir string
	blobs   *blob.FS
	server  *httptest.Server
	svc     *Service // the maps service, for the tests that look inside it
}

// noHeartbeat keeps heartbeats out of the tests' streams: a test that
// checks that nothing reached a player reads the next event, and a
// heartbeat would only get in the way.
const noHeartbeat = time.Hour

// newHarness starts the services. configure, when given, changes the maps
// service's Config (a small quota, no blob store...).
func newHarness(t *testing.T, configure ...func(*Config)) *harness {
	t.Helper()
	pool := dbtest.NewPool(t, "meurpg_maps_test")
	h := &harness{t: t, pool: pool, users: identity.NewPostgresStore(pool), blobDir: t.TempDir()}
	blobs, err := blob.NewFS(h.blobDir)
	if err != nil {
		t.Fatalf("blob.NewFS() error = %v", err)
	}
	t.Cleanup(func() { _ = blobs.Close() })
	h.blobs = blobs

	clock := &fakeClock{now: time.Now().Truncate(time.Microsecond)}
	logger := slog.New(slog.DiscardHandler)
	camps, err := campaigns.New(campaigns.Config{Pool: pool, Profiles: h.users, Logger: logger, Now: clock.Now})
	if err != nil {
		t.Fatalf("campaigns.New() error = %v", err)
	}
	content, err := testRules()
	if err != nil {
		t.Fatalf("rules.LoadSRD() error = %v", err)
	}
	chars, err := characters.New(characters.Config{Pool: pool, Profiles: h.users, Members: camps, Content: contenttest.NewSource(pool, content), SRD: content, Logger: logger, Now: clock.Now})
	if err != nil {
		t.Fatalf("characters.New() error = %v", err)
	}
	portraits := NewSessionMaps(pool)
	chars.SetGallery(portraits) // an NPC's portrait is an image of the gallery (MR-031)
	// Wired as in cmd/api: play reveals the current map through
	// SessionMaps, and this service reads it and publishes through play.
	sessionMaps := NewSessionMaps(pool)
	live, err := play.New(play.Config{
		Pool: pool, Sheets: chars, Vitals: chars, Campaigns: camps, Maps: sessionMaps, Roster: chars, Dice: testDice{camps},
		Live: play.LiveConfig{Heartbeat: noHeartbeat}, Logger: logger, Now: clock.Now,
	})
	if err != nil {
		t.Fatalf("play.New() error = %v", err)
	}
	cfg := Config{Pool: pool, Blobs: blobs, Characters: chars, Live: live, Combats: live, Defaults: camps, Rules: content, Logger: logger, Now: clock.Now}
	for _, c := range configure {
		c(&cfg)
	}
	svc, err := New(cfg)
	if err != nil {
		t.Fatalf("New() error = %v", err)
	}
	// Every hint at once: a test reads the stream right after a change, and the
	// gate's once-a-second timing (armed by the setup's own paints) would race it.
	// TestMR036_APaintHeldByTheGateStillReachesThePlayers sets its own interval.
	svc.layerHintEvery = 0
	h.svc = svc
	// A request's goroutine must end before the test's pool goes away.
	t.Cleanup(func() {
		ctx, cancel := context.WithTimeout(context.Background(), time.Minute)
		defer cancel()
		svc.WaitForGenerations(ctx)
		svc.CancelGenerations()
	})
	sessionMaps.SetService(svc)
	portraits.SetService(svc)

	notesSvc, err := notes.New(notes.Config{Pool: pool, Scenes: sessionMaps, Logger: logger, Now: clock.Now})
	if err != nil {
		t.Fatalf("notes.New() error = %v", err)
	}

	treasures := NewTreasures(pool)
	treasures.SetService(svc) // a conversion tells the open maps, as cmd/api wires it
	// The XP: "Voltar à cidade" turns a found treasure into XP (MR-041, MR-044).
	xp, err := progression.New(progression.Config{
		Pool: pool, Party: chars, Combats: live, Log: live, Treasures: treasures, Campaigns: camps, Profiles: h.users, Logger: logger, Now: clock.Now,
	})
	if err != nil {
		t.Fatalf("progression.New() error = %v", err)
	}
	chars.SetLevelUps(xp)
	camps.SetXPAwards(xp)

	srv := httpserver.New(httpserver.Config{Logger: logger})
	opt := connect.WithRequireConnectProtocolHeader()
	xp.Mount(srv.Handle, fakeSessions{}, camps, opt)
	camps.Mount(srv.Handle, fakeSessions{}, opt)
	chars.Mount(srv.Handle, fakeSessions{}, camps, opt)
	live.Mount(srv.Handle, fakeSessions{}, camps, opt)
	svc.Mount(srv.Handle, testSessions, camps, opt)
	notesSvc.Mount(srv.Handle, fakeSessions{}, camps, opt)
	h.server = httptest.NewServer(srv.Handler())
	t.Cleanup(h.server.Close)
	t.Cleanup(live.Close) // end the streams first, so the server can close
	return h
}

type user struct {
	id         string
	h          *harness
	campaigns  campaignsv1connect.CampaignServiceClient
	characters charactersv1connect.CharacterServiceClient
	play       playv1connect.PlayServiceClient
	combat     playv1connect.CombatServiceClient
	content    rulesv1connect.ContentServiceClient
	gallery    mapsv1connect.GalleryServiceClient
	imagegen   mapsv1connect.ImageGenerationServiceClient
	maps       mapsv1connect.MapServiceClient
	dungeons   mapsv1connect.DungeonServiceClient
	treasure   mapsv1connect.TreasureServiceClient
	xp         progressionv1connect.ProgressionServiceClient
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

func (h *harness) anonymous() *user { return h.clients("") }

func (h *harness) clients(userID string) *user {
	c, url := h.server.Client(), h.server.URL
	if userID != "" {
		c = &http.Client{Transport: userTransport{userID: userID, next: c.Transport}}
	}
	return &user{
		id:         userID,
		h:          h,
		campaigns:  campaignsv1connect.NewCampaignServiceClient(c, url),
		characters: charactersv1connect.NewCharacterServiceClient(c, url, connect.WithInterceptors(choicetest.Interceptor(charactersv1connect.NewCharacterServiceClient(c, url)))),
		play:       playv1connect.NewPlayServiceClient(c, url),
		combat:     playv1connect.NewCombatServiceClient(c, url),
		content:    rulesv1connect.NewContentServiceClient(c, url),
		gallery:    mapsv1connect.NewGalleryServiceClient(c, url),
		imagegen:   mapsv1connect.NewImageGenerationServiceClient(c, url),
		maps:       mapsv1connect.NewMapServiceClient(c, url),
		dungeons:   mapsv1connect.NewDungeonServiceClient(c, url),
		treasure:   mapsv1connect.NewTreasureServiceClient(c, url),
		xp:         progressionv1connect.NewProgressionServiceClient(c, url),
		notes:      notesv1connect.NewNotesServiceClient(c, url),
	}
}

// newCampaign creates a campaign whose master is master, lets the players
// in, and returns its ID.
func (h *harness) newCampaign(master *user, players ...*user) string {
	h.t.Helper()
	res, err := master.campaigns.CreateCampaign(h.t.Context(), connect.NewRequest(&campaignsv1.CreateCampaignRequest{Name: "Mirathel", XpMode: campaignsv1.XpMode_XP_MODE_MILESTONES}))
	if err != nil {
		h.t.Fatalf("CreateCampaign() error = %v", err)
	}
	id := res.Msg.GetCampaign().GetId()
	h.join(master, id, false, players...)
	return id
}

// join lets the players in, through an invite from the master; pending
// makes them pending members (RN-15, MR-024).
func (h *harness) join(master *user, campaignID string, pending bool, players ...*user) {
	h.t.Helper()
	ctx := h.t.Context()
	inv, err := master.campaigns.CreateInvite(ctx, connect.NewRequest(&campaignsv1.CreateInviteRequest{
		CampaignId: campaignID, MaxUses: campaigns.MaxInviteUses, RequiresApproval: pending,
	}))
	if err != nil {
		h.t.Fatalf("CreateInvite() error = %v", err)
	}
	for _, p := range players {
		if _, err := p.campaigns.AcceptInvite(ctx, connect.NewRequest(&campaignsv1.AcceptInviteRequest{Token: inv.Msg.GetToken()})); err != nil {
			h.t.Fatalf("AcceptInvite() error = %v", err)
		}
	}
}

// httpResult is an answer of the image routes.
type httpResult struct {
	status int
	header http.Header
	body   []byte
}

// errorBody decodes an error answer.
func (r httpResult) errorBody(t *testing.T) errorBody {
	t.Helper()
	var e errorBody
	if err := json.Unmarshal(r.body, &e); err != nil {
		t.Fatalf("error body %q is not JSON: %v", r.body, err)
	}
	return e
}

// image decodes a successful upload's answer.
func (r httpResult) image(t *testing.T) *mapsv1.GalleryImage {
	t.Helper()
	var img mapsv1.GalleryImage
	if err := protojson.Unmarshal(r.body, &img); err != nil {
		t.Fatalf("upload answer %q is not a GalleryImage: %v", r.body, err)
	}
	return &img
}

// do sends req as u and reads the whole answer.
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

// formField is one part of a multipart form: a text field when fileName is
// "", else a file.
type formField struct {
	name, fileName string
	content        []byte
}

// uploadRequest builds a POST /uploads/images with the fields in order.
func (u *user) uploadRequest(fields ...formField) *http.Request {
	u.h.t.Helper()
	var body bytes.Buffer
	w := multipart.NewWriter(&body)
	for _, f := range fields {
		var part io.Writer
		var err error
		if f.fileName == "" {
			part, err = w.CreateFormField(f.name)
		} else {
			part, err = w.CreateFormFile(f.name, f.fileName)
		}
		if err != nil {
			u.h.t.Fatal(err)
		}
		_, _ = part.Write(f.content)
	}
	if err := w.Close(); err != nil {
		u.h.t.Fatal(err)
	}
	req, err := http.NewRequestWithContext(u.h.t.Context(), http.MethodPost, u.h.server.URL+UploadPath, &body)
	if err != nil {
		u.h.t.Fatal(err)
	}
	req.Header.Set("Content-Type", w.FormDataContentType())
	return req
}

// upload sends a file the way the app does: campaign_id, then file.
func (u *user) upload(campaignID, fileName string, content []byte) httpResult {
	u.h.t.Helper()
	return u.do(u.uploadRequest(formField{name: "campaign_id", content: []byte(campaignID)}, formField{name: "file", fileName: fileName, content: content}))
}

// mustUpload uploads as u, or fails the test.
func (u *user) mustUpload(campaignID, fileName string, content []byte) *mapsv1.GalleryImage {
	u.h.t.Helper()
	res := u.upload(campaignID, fileName, content)
	if res.status != http.StatusCreated {
		u.h.t.Fatalf("upload %s: status %d, body %s; want 201", fileName, res.status, res.body)
	}
	return res.image(u.h.t)
}

// get fetches a path as u, with extra headers (key, value, ...).
func (u *user) get(path string, headers ...string) httpResult {
	u.h.t.Helper()
	req, err := http.NewRequestWithContext(u.h.t.Context(), http.MethodGet, u.h.server.URL+path, nil)
	if err != nil {
		u.h.t.Fatal(err)
	}
	for i := 0; i+1 < len(headers); i += 2 {
		req.Header.Set(headers[i], headers[i+1])
	}
	return u.do(req)
}

// list calls ListGalleryImages as u, or fails the test.
func (u *user) list(campaignID string) *mapsv1.ListGalleryImagesResponse {
	u.h.t.Helper()
	res, err := u.gallery.ListGalleryImages(u.h.t.Context(), connect.NewRequest(&mapsv1.ListGalleryImagesRequest{CampaignId: campaignID}))
	if err != nil {
		u.h.t.Fatalf("ListGalleryImages() error = %v", err)
	}
	return res.Msg
}

// storedFiles lists the files in the blob store, the temporary folder
// left out.
func (h *harness) storedFiles() []string {
	h.t.Helper()
	var files []string
	err := filepath.WalkDir(h.blobDir, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() && d.Name() == ".tmp" {
			return filepath.SkipDir
		}
		if !d.IsDir() {
			rel, _ := filepath.Rel(h.blobDir, path)
			files = append(files, filepath.ToSlash(rel))
		}
		return nil
	})
	if err != nil {
		h.t.Fatalf("list the stored files: %v", err)
	}
	return files
}

// storedFile reads a file from the blob store, as it is kept.
func (h *harness) storedFile(key string) (string, []byte) {
	h.t.Helper()
	obj, err := h.blobs.Open(h.t.Context(), key)
	if err != nil {
		h.t.Fatalf("open %s: %v", key, err)
	}
	defer func() { _ = obj.Close() }()
	content, err := io.ReadAll(obj.Content)
	if err != nil {
		h.t.Fatalf("read %s: %v", key, err)
	}
	return obj.ContentType, content
}

// Test images.

// gpsNote is text in the test JPEG's EXIF, next to a GPS position.
const gpsNote = "photo taken at home"

// jpegWithGPS is a w x h JPEG whose EXIF has an image description
// (gpsNote) and a GPS directory with a latitude.
func jpegWithGPS(t *testing.T, w, h int) []byte {
	t.Helper()
	le := binary.LittleEndian
	const desc = gpsNote + "\x00"
	// TIFF header, then IFD0 with two entries (ImageDescription, GPSInfo),
	// the description, then the GPS directory with one entry
	// (GPSLatitudeRef "S").
	const (
		ifd0   = 8
		descAt = ifd0 + 2 + 2*12 + 4
		gpsAt  = descAt + len(desc)
	)
	tiff := make([]byte, gpsAt+2+12+4)
	copy(tiff, "II")
	le.PutUint16(tiff[2:], 42)
	le.PutUint32(tiff[4:], ifd0)
	le.PutUint16(tiff[ifd0:], 2)
	entry := func(at int, tag, typ uint16, count, value uint32) {
		le.PutUint16(tiff[at:], tag)
		le.PutUint16(tiff[at+2:], typ)
		le.PutUint32(tiff[at+4:], count)
		le.PutUint32(tiff[at+8:], value)
	}
	entry(ifd0+2, 0x010e, 2, uint32(len(desc)), uint32(descAt))
	entry(ifd0+14, 0x8825, 4, 1, uint32(gpsAt))
	copy(tiff[descAt:], desc)
	le.PutUint16(tiff[gpsAt:], 1)
	entry(gpsAt+2, 0x0001, 2, 2, 'S')

	var buf bytes.Buffer
	if err := jpeg.Encode(&buf, solid(w, h, color.NRGBA{R: 200, G: 120, B: 40, A: 255}), nil); err != nil {
		t.Fatal(err)
	}
	jpg := buf.Bytes()
	payload := append([]byte("Exif\x00\x00"), tiff...)
	out := append([]byte{}, jpg[:2]...)
	out = binary.BigEndian.AppendUint16(append(out, 0xff, 0xe1), uint16(len(payload)+2)) //nolint:gosec // G115: a test EXIF block of a few dozen bytes
	out = append(out, payload...)
	return append(out, jpg[2:]...)
}

// pngImage is a w x h PNG of one color.
func pngImage(t *testing.T, w, h int) []byte {
	t.Helper()
	var buf bytes.Buffer
	if err := png.Encode(&buf, solid(w, h, color.NRGBA{R: 30, G: 90, B: 160, A: 255})); err != nil {
		t.Fatal(err)
	}
	return buf.Bytes()
}

func solid(w, h int, c color.NRGBA) *image.NRGBA {
	img := image.NewNRGBA(image.Rect(0, 0, w, h))
	for y := range h {
		for x := range w {
			img.SetNRGBA(x, y, c)
		}
	}
	return img
}

// allowed marks a call that must succeed in the authorization matrix.
const allowed connect.Code = 0

// wantRefusal fails unless err is a Connect error with this code and exactly this message.
func wantRefusal(t *testing.T, call string, err error, code connect.Code, message string) {
	t.Helper()
	ce, ok := errors.AsType[*connect.Error](err)
	if !ok {
		t.Errorf("%s error = %v, want %v %q", call, err, code, message)
		return
	}
	if ce.Code() != code || ce.Message() != message {
		t.Errorf("%s error = %v %q, want %v %q", call, ce.Code(), ce.Message(), code, message)
	}
}

func wantCode(t *testing.T, call string, err error, want connect.Code) {
	t.Helper()
	switch {
	case want == allowed && err != nil:
		t.Errorf("%s error = %v, want allowed", call, err)
	case want != allowed && connect.CodeOf(err) != want:
		t.Errorf("%s error = %v, want %v", call, err, want)
	}
}

// testDice is play's DiceModes over the campaigns service, as cmd/api wires it.
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

// afterQuery runs hook once, right after the first statement whose text
// contains match finishes.
type afterQuery struct {
	match string
	hook  func()
	once  sync.Once
}

type queryTextKey struct{}

func (*afterQuery) TraceQueryStart(ctx context.Context, _ *pgx.Conn, data pgx.TraceQueryStartData) context.Context {
	return context.WithValue(ctx, queryTextKey{}, data.SQL)
}

func (a *afterQuery) TraceQueryEnd(ctx context.Context, _ *pgx.Conn, _ pgx.TraceQueryEndData) {
	if sql, _ := ctx.Value(queryTextKey{}).(string); strings.Contains(sql, a.match) {
		a.once.Do(func() {
			// From a goroutine of its own: a write through another pool from
			// inside a transaction's closure would look like a nested acquisition.
			done := make(chan struct{})
			go func() {
				defer close(done)
				a.hook()
			}()
			<-done
		})
	}
}

// hookAfterQuery makes the maps service read through a pool of its own whose
// statements run hook once, after the first one that contains match. The hook
// must write through h.pool or the API, never through the service's pool: a
// write committed there lands exactly between two statements of a read.
func (h *harness) hookAfterQuery(match string, hook func()) {
	h.t.Helper()
	pool, err := db.NewPoolWith(h.t.Context(), h.pool.Config().ConnString(), func(cfg *pgxpool.Config) {
		cfg.MaxConns = 2
		cfg.ConnConfig.Tracer = &afterQuery{match: match, hook: hook}
	})
	if err != nil {
		h.t.Fatalf("NewPoolWith() error = %v", err)
	}
	h.t.Cleanup(pool.Close)
	h.svc.pool, h.svc.queries = pool, mapsdb.New(pool)
}
