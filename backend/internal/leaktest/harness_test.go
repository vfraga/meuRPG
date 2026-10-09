package leaktest

import (
	"bytes"
	"context"
	"crypto/rand"
	"errors"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"os"
	"sync"
	"testing"
	"time"

	"connectrpc.com/connect"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"google.golang.org/protobuf/encoding/protojson"
	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/reflect/protoreflect"
	"google.golang.org/protobuf/reflect/protoregistry"
	"google.golang.org/protobuf/types/dynamicpb"

	"github.com/PuraFome/meuRPG/backend/gen/meurpg/campaigns/v1/campaignsv1connect"
	"github.com/PuraFome/meuRPG/backend/gen/meurpg/characters/v1/charactersv1connect"
	"github.com/PuraFome/meuRPG/backend/gen/meurpg/maps/v1/mapsv1connect"
	"github.com/PuraFome/meuRPG/backend/gen/meurpg/notes/v1/notesv1connect"
	"github.com/PuraFome/meuRPG/backend/gen/meurpg/play/v1/playv1connect"
	"github.com/PuraFome/meuRPG/backend/gen/meurpg/progression/v1/progressionv1connect"
	"github.com/PuraFome/meuRPG/backend/gen/meurpg/rules/v1/rulesv1connect"
	"github.com/PuraFome/meuRPG/backend/internal/campaigns"
	"github.com/PuraFome/meuRPG/backend/internal/characters"
	"github.com/PuraFome/meuRPG/backend/internal/identity"
	"github.com/PuraFome/meuRPG/backend/internal/maps"
	"github.com/PuraFome/meuRPG/backend/internal/maps/images/gen"
	"github.com/PuraFome/meuRPG/backend/internal/notes"
	"github.com/PuraFome/meuRPG/backend/internal/platform/blob"
	"github.com/PuraFome/meuRPG/backend/internal/platform/dbtest"
	"github.com/PuraFome/meuRPG/backend/internal/platform/httpserver"
	"github.com/PuraFome/meuRPG/backend/internal/play"
	"github.com/PuraFome/meuRPG/backend/internal/progression"
	"github.com/PuraFome/meuRPG/backend/internal/rules"
)

// The leak tests run against CockroachDB with every module of the API wired the
// way cmd/api wires them (same constructors, same Set* calls, the production
// table-content source, the real HTTP stack), so a read that goes through a
// seam between two modules is the read the app makes. Only the sign-in is
// replaced: fakeSessions trusts a test header. Without MEURPG_TEST_DATABASE_URL
// the tests skip (see package dbtest).

const testUserHeader = "Test-User-Id"

type fakeSessions struct{}

type testUserKey struct{}

func withTestUser(ctx context.Context, header http.Header) context.Context {
	if userID := header.Get(testUserHeader); userID != "" {
		return context.WithValue(ctx, testUserKey{}, userID)
	}
	return ctx
}

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

func (fakeSessions) UserID(ctx context.Context) (string, error) {
	if userID, ok := ctx.Value(testUserKey{}).(string); ok {
		return userID, nil
	}
	return "", connect.NewError(connect.CodeUnauthenticated, errors.New("sign in to continue"))
}

// RecheckSession is what the live stream calls from time to time; the test
// sessions never end.
func (f fakeSessions) RecheckSession(ctx context.Context) error {
	_, err := f.UserID(ctx)
	return err
}

func (fakeSessions) AuthenticateRequest(r *http.Request) (context.Context, error) {
	return withTestUser(r.Context(), r.Header), nil
}

// userTransport adds the test user header to every request, calls and streams alike.
type userTransport struct {
	userID string
	next   http.RoundTripper
}

func (t userTransport) RoundTrip(req *http.Request) (*http.Response, error) {
	req = req.Clone(req.Context())
	if t.userID != "" {
		req.Header.Set(testUserHeader, t.userID)
	}
	return t.next.RoundTrip(req)
}

type fakeClock struct {
	mu  sync.Mutex
	now time.Time
}

// Now ticks one microsecond each time it is read (CockroachDB keeps microseconds).
func (c *fakeClock) Now() time.Time {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.now = c.now.Add(time.Microsecond)
	return c.now
}

// dice adapts the campaigns service to play's DiceModes, as cmd/api does.
type dice struct{ camps *campaigns.Service }

func (d dice) ForcedDice(ctx context.Context, tx pgx.Tx, campaignID, userID string) (play.DiceForce, error) {
	mode, err := d.camps.CampaignDiceMode(ctx, tx, campaignID, userID)
	switch mode {
	case campaigns.DiceModeApp:
		return play.DiceForcedInApp, err
	case campaigns.DiceModePhysical:
		return play.DiceForcedPhysical, err
	}
	return play.DiceChoice, err
}

// stack is every module of the API on one test server.
type stack struct {
	t      *testing.T
	pool   *pgxpool.Pool
	users  *identity.PostgresStore
	server *httptest.Server
	play   *play.Service
	maps   *maps.Service
}

// heartbeat is how often a stream says it is alive. The stream test reads every event
// until the session ends, so a short one makes the heartbeat an event it sees.
const heartbeat = 100 * time.Millisecond

func newStack(t *testing.T) *stack {
	t.Helper()
	pool := dbtest.NewPool(t, "meurpg_leaktest_test")
	s := &stack{t: t, pool: pool, users: identity.NewPostgresStore(pool)}
	logger := slog.New(slog.DiscardHandler)
	if os.Getenv("LEAKTEST_LOG") != "" { // the servers' own log lines, to find out why a call went wrong
		logger = slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: slog.LevelWarn}))
	}
	clock := &fakeClock{now: time.Now().Truncate(time.Microsecond)}
	srd, err := rules.LoadSRD()
	if err != nil {
		t.Fatalf("rules.LoadSRD() error = %v", err)
	}
	blobs, err := blob.NewFS(t.TempDir())
	if err != nil {
		t.Fatalf("blob.NewFS() error = %v", err)
	}
	t.Cleanup(func() { _ = blobs.Close() })

	camps, err := campaigns.New(campaigns.Config{Pool: pool, Profiles: s.users, Logger: logger, Now: clock.Now})
	if err != nil {
		t.Fatalf("campaigns.New() error = %v", err)
	}
	chars, err := characters.New(characters.Config{
		Pool: pool, Profiles: s.users, Members: camps,
		Content: characters.NewTableSource(pool, srd, camps), SRD: srd, Logger: logger, Now: clock.Now,
	})
	if err != nil {
		t.Fatalf("characters.New() error = %v", err)
	}
	camps.SetCharacters(chars)
	sessionMaps := maps.NewSessionMaps(pool)
	chars.SetGallery(sessionMaps)
	live, err := play.New(play.Config{
		Pool: pool, Sheets: chars, Vitals: chars, Campaigns: camps, Maps: sessionMaps, Roster: chars,
		Dice: dice{camps}, Defaults: camps, Live: play.LiveConfig{Heartbeat: heartbeat}, Logger: logger, Now: clock.Now,
	})
	if err != nil {
		t.Fatalf("play.New() error = %v", err)
	}
	chars.SetLive(live)
	chars.SetCreatureHost(live)
	msvc, err := maps.New(maps.Config{
		Pool: pool, Blobs: blobs, Characters: chars, Live: live, Generator: &gen.Fake{}, Rules: srd, Combats: live,
		Defaults: camps, Logger: logger, Now: clock.Now,
	})
	if err != nil {
		t.Fatalf("maps.New() error = %v", err)
	}
	live.SetTerrain(msvc)
	live.SetPuzzleMaps(msvc)
	live.SetFog(msvc)
	live.SetTraps(msvc)
	msvc.SetTrapFirer(live)
	sessionMaps.SetService(msvc)
	t.Cleanup(func() {
		ctx, cancel := context.WithTimeout(context.Background(), time.Minute)
		defer cancel()
		msvc.WaitForGenerations(ctx)
		msvc.CancelGenerations()
	})
	xp, err := progression.New(progression.Config{
		Pool: pool, Party: chars, Combats: live, Log: live, Treasures: maps.NewTreasures(pool), Campaigns: camps, Profiles: s.users, Logger: logger, Now: clock.Now,
	})
	if err != nil {
		t.Fatalf("progression.New() error = %v", err)
	}
	camps.SetXPAwards(xp)
	chars.SetLevelUps(xp)
	notesSvc, err := notes.New(notes.Config{Pool: pool, Scenes: sessionMaps, Logger: logger, Now: clock.Now})
	if err != nil {
		t.Fatalf("notes.New() error = %v", err)
	}

	srv := httpserver.New(httpserver.Config{Logger: logger})
	opt := connect.WithRequireConnectProtocolHeader()
	sessions := fakeSessions{}
	camps.Mount(srv.Handle, sessions, opt)
	chars.Mount(srv.Handle, sessions, camps, opt)
	live.Mount(srv.Handle, sessions, camps, opt)
	xp.Mount(srv.Handle, sessions, camps, opt)
	notesSvc.Mount(srv.Handle, sessions, camps, opt)
	msvc.Mount(srv.Handle, sessions, camps, opt)
	s.server = httptest.NewServer(srv.Handler())
	t.Cleanup(s.server.Close)
	t.Cleanup(live.Close) // end the streams first, so the server can close
	s.play, s.maps = live, msvc
	return s
}

// person is someone who calls the API: the master, a player, a pending member or
// a stranger.
type person struct {
	name   string
	id     string
	client *http.Client
	stack  *stack

	// Typed clients, for building the fixture. The matrix itself goes through call.
	campaigns  campaignsv1connect.CampaignServiceClient
	document   campaignsv1connect.CampaignDocumentServiceClient
	characters charactersv1connect.CharacterServiceClient
	inventory  charactersv1connect.InventoryServiceClient
	content    rulesv1connect.ContentServiceClient
	table      rulesv1connect.TableContentServiceClient
	play       playv1connect.PlayServiceClient
	combat     playv1connect.CombatServiceClient
	puzzles    playv1connect.PuzzleServiceClient
	encounters playv1connect.EncounterServiceClient
	maps       mapsv1connect.MapServiceClient
	gallery    mapsv1connect.GalleryServiceClient
	imagegen   mapsv1connect.ImageGenerationServiceClient
	dungeons   mapsv1connect.DungeonServiceClient
	treasure   mapsv1connect.TreasureServiceClient
	xp         progressionv1connect.ProgressionServiceClient
	notes      notesv1connect.NotesServiceClient
}

func (s *stack) newPerson(name string) *person {
	s.t.Helper()
	id, err := s.users.UpsertUser(s.t.Context(), identity.ExternalIdentity{Issuer: "https://idp.test", Subject: rand.Text()})
	if err != nil {
		s.t.Fatalf("UpsertUser() error = %v", err)
	}
	if err := s.users.SetDisplayName(s.t.Context(), id, name); err != nil {
		s.t.Fatalf("SetDisplayName() error = %v", err)
	}
	return s.personWith(name, id)
}

// personWith makes the clients of someone who calls with a user id, or with none ("").
func (s *stack) personWith(name, id string) *person {
	c := &http.Client{Transport: userTransport{userID: id, next: s.server.Client().Transport}}
	url := s.server.URL
	return &person{
		name: name, id: id, stack: s, client: c,
		campaigns:  campaignsv1connect.NewCampaignServiceClient(c, url),
		document:   campaignsv1connect.NewCampaignDocumentServiceClient(c, url),
		characters: charactersv1connect.NewCharacterServiceClient(c, url),
		inventory:  charactersv1connect.NewInventoryServiceClient(c, url),
		content:    rulesv1connect.NewContentServiceClient(c, url),
		table:      rulesv1connect.NewTableContentServiceClient(c, url),
		play:       playv1connect.NewPlayServiceClient(c, url),
		combat:     playv1connect.NewCombatServiceClient(c, url),
		puzzles:    playv1connect.NewPuzzleServiceClient(c, url),
		encounters: playv1connect.NewEncounterServiceClient(c, url),
		maps:       mapsv1connect.NewMapServiceClient(c, url),
		gallery:    mapsv1connect.NewGalleryServiceClient(c, url),
		imagegen:   mapsv1connect.NewImageGenerationServiceClient(c, url),
		dungeons:   mapsv1connect.NewDungeonServiceClient(c, url),
		treasure:   mapsv1connect.NewTreasureServiceClient(c, url),
		xp:         progressionv1connect.NewProgressionServiceClient(c, url),
		notes:      notesv1connect.NewNotesServiceClient(c, url),
	}
}

// reply is the answer to one call: the HTTP status, the raw body (what the app
// receives, JSON) and, for a 200, the decoded message.
type reply struct {
	status int
	body   []byte
	msg    proto.Message
}

// Code is the Connect error code of a failed call, "" for a successful one.
func (r reply) ok() bool { return r.status == http.StatusOK }

// call sends req to a unary procedure ("/meurpg.maps.v1.MapService/GetMap") as
// p, the way the app does: Connect over HTTP with a JSON body. One door for all
// the procedures is what lets the table of reads be data.
func (p *person) call(procedure string, req proto.Message) reply {
	p.stack.t.Helper()
	method := methodOf(p.stack.t, procedure)
	body, err := protojson.Marshal(req)
	if err != nil {
		p.stack.t.Fatalf("marshal the request of %s: %v", procedure, err)
	}
	hr, err := http.NewRequestWithContext(p.stack.t.Context(), http.MethodPost, p.stack.server.URL+procedure, bytes.NewReader(body))
	if err != nil {
		p.stack.t.Fatal(err)
	}
	hr.Header.Set("Content-Type", "application/json")
	hr.Header.Set("Connect-Protocol-Version", "1")
	res, err := p.client.Do(hr)
	if err != nil {
		p.stack.t.Fatalf("%s: %v", procedure, err)
	}
	defer func() { _ = res.Body.Close() }()
	raw, err := io.ReadAll(res.Body)
	if err != nil {
		p.stack.t.Fatalf("%s: read the answer: %v", procedure, err)
	}
	out := reply{status: res.StatusCode, body: raw}
	if out.ok() {
		m := dynamicpb.NewMessage(method.Output())
		if err := protojson.Unmarshal(raw, m); err != nil {
			p.stack.t.Fatalf("%s: the answer %s is not a %s: %v", procedure, raw, method.Output().FullName(), err)
		}
		out.msg = m
	}
	return out
}

// get fetches an HTTP path (an image, a fog tile) as p.
func (p *person) get(path string) (int, []byte) {
	p.stack.t.Helper()
	hr, err := http.NewRequestWithContext(p.stack.t.Context(), http.MethodGet, p.stack.server.URL+path, nil)
	if err != nil {
		p.stack.t.Fatal(err)
	}
	res, err := p.client.Do(hr)
	if err != nil {
		p.stack.t.Fatalf("GET %s: %v", path, err)
	}
	defer func() { _ = res.Body.Close() }()
	raw, err := io.ReadAll(res.Body)
	if err != nil {
		p.stack.t.Fatalf("GET %s: read: %v", path, err)
	}
	return res.StatusCode, raw
}

// methodOf finds the method descriptor of a procedure path.
func methodOf(t *testing.T, procedure string) protoreflect.MethodDescriptor {
	t.Helper()
	m, ok := findMethod(procedure)
	if !ok {
		t.Fatalf("no such procedure %s", procedure)
	}
	return m
}

func findMethod(procedure string) (protoreflect.MethodDescriptor, bool) {
	// "/meurpg.maps.v1.MapService/GetMap"
	if len(procedure) < 2 || procedure[0] != '/' {
		return nil, false
	}
	for i := len(procedure) - 1; i > 0; i-- {
		if procedure[i] == '/' {
			d, err := protoregistry.GlobalFiles.FindDescriptorByName(protoreflect.FullName(procedure[1:i]))
			if err != nil {
				return nil, false
			}
			svc, ok := d.(protoreflect.ServiceDescriptor)
			if !ok {
				return nil, false
			}
			m := svc.Methods().ByName(protoreflect.Name(procedure[i+1:]))
			return m, m != nil
		}
	}
	return nil, false
}

func protojsonUnmarshal(raw []byte, m proto.Message) error { return protojson.Unmarshal(raw, m) }
