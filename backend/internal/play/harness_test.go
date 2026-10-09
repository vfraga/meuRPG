package play

import (
	"context"
	"crypto/rand"
	"errors"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"connectrpc.com/connect"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	campaignsv1 "github.com/PuraFome/meuRPG/backend/gen/meurpg/campaigns/v1"
	"github.com/PuraFome/meuRPG/backend/gen/meurpg/campaigns/v1/campaignsv1connect"
	charactersv1 "github.com/PuraFome/meuRPG/backend/gen/meurpg/characters/v1"
	"github.com/PuraFome/meuRPG/backend/gen/meurpg/characters/v1/charactersv1connect"
	playv1 "github.com/PuraFome/meuRPG/backend/gen/meurpg/play/v1"
	"github.com/PuraFome/meuRPG/backend/gen/meurpg/play/v1/playv1connect"
	rulesv1 "github.com/PuraFome/meuRPG/backend/gen/meurpg/rules/v1"
	"github.com/PuraFome/meuRPG/backend/gen/meurpg/rules/v1/rulesv1connect"
	"github.com/PuraFome/meuRPG/backend/internal/campaigns"
	"github.com/PuraFome/meuRPG/backend/internal/characters"
	"github.com/PuraFome/meuRPG/backend/internal/characters/contenttest"
	"github.com/PuraFome/meuRPG/backend/internal/identity"
	"github.com/PuraFome/meuRPG/backend/internal/maps"
	"github.com/PuraFome/meuRPG/backend/internal/platform/dbtest"
	"github.com/PuraFome/meuRPG/backend/internal/platform/httpserver"
	"github.com/PuraFome/meuRPG/backend/internal/rules"
)

// The database tests run against CockroachDB with the real campaigns and
// characters services next to this one, as cmd/api wires them: set
// MEURPG_TEST_DATABASE_URL (see package dbtest). Without it they skip.

var testRules = sync.OnceValues(rules.LoadSRD)

// testUserHeader names the signed-in user in a test request. fakeSessions
// trusts it; production code has no way to set a caller (see
// docs/architecture.md#who-is-calling).
const testUserHeader = "Test-User-Id"

type fakeSessions struct{}

type testUserKey struct{}

var testSessions Sessions = fakeSessions{}

// signedOut holds the users whose sessions a test ended: RecheckSession
// answers unauthenticated for them, as identity does after SignOut. User
// IDs are random per test, so tests running in parallel never collide.
var signedOut sync.Map

// rechecks counts the RecheckSession calls per user ID (an *atomic.Int64), so a test
// can tell how many rechecks a stream has been through instead of sleeping.
var rechecks sync.Map

func recheckCount(userID string) int64 {
	if n, ok := rechecks.Load(userID); ok {
		return n.(*atomic.Int64).Load()
	}
	return 0
}

func errSignIn() error {
	return connect.NewError(connect.CodeUnauthenticated, errors.New("sign in to continue"))
}

// Interceptor reads the test user, for calls and streams alike.
func (fakeSessions) Interceptor() connect.Interceptor { return fakeSessionInterceptor{} }

type fakeSessionInterceptor struct{}

func withTestUser(ctx context.Context, header http.Header) context.Context {
	if userID := header.Get(testUserHeader); userID != "" {
		return context.WithValue(ctx, testUserKey{}, userID)
	}
	return ctx
}

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
	return "", errSignIn()
}

func (f fakeSessions) RecheckSession(ctx context.Context) error {
	userID, err := f.UserID(ctx)
	if err != nil {
		return err
	}
	n, _ := rechecks.LoadOrStore(userID, new(atomic.Int64))
	n.(*atomic.Int64).Add(1)
	if _, out := signedOut.Load(userID); out {
		return errSignIn()
	}
	return nil
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

// fakeClock ticks one microsecond each time it is read, so rows created one
// after the other have different times.
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
	t      *testing.T
	pool   *pgxpool.Pool
	users  *identity.PostgresStore
	svc    *Service
	roller *scriptedRoller    // the dice the service rolls
	http   *httpserver.Server // the API's server, for tests that Serve it
	server *httptest.Server   // serves http's handler over HTTP/1.1
	chars  *characters.Service
	camps  *campaigns.Service
	vision *visionCounter // how many times the fog was told that what the players see changed
}

// newHarness serves the play, campaigns and characters services through
// the API's real HTTP stack (httpserver: request logging, cross-origin
// protection), as cmd/api does. live, when given, sets the stream's timing.
func newHarness(t *testing.T, live ...LiveConfig) *harness {
	t.Helper()
	pool := dbtest.NewPool(t, "meurpg_play_test")
	h := &harness{t: t, pool: pool, users: identity.NewPostgresStore(pool), roller: &scriptedRoller{}}
	clock := &fakeClock{now: time.Now().Truncate(time.Microsecond)}
	logger := slog.New(slog.DiscardHandler)
	var liveConfig LiveConfig
	if len(live) > 0 {
		liveConfig = live[0]
	}
	content, err := testRules()
	if err != nil {
		t.Fatalf("rules.LoadSRD() error = %v", err)
	}
	camps, err := campaigns.New(campaigns.Config{Pool: pool, Profiles: h.users, Logger: logger, Now: clock.Now})
	if err != nil {
		t.Fatalf("campaigns.New() error = %v", err)
	}
	chars, err := characters.New(characters.Config{Pool: pool, Profiles: h.users, Members: camps, Content: contenttest.NewSource(pool, content), SRD: content, Logger: logger, Now: clock.Now})
	if err != nil {
		t.Fatalf("characters.New() error = %v", err)
	}
	h.vision = &visionCounter{SessionMaps: maps.NewSessionMaps(pool)}
	chars.SetGallery(maps.NewSessionMaps(pool)) // an NPC's portrait is an image of the gallery (MR-031)
	svc, err := New(Config{Pool: pool, Sheets: chars, Vitals: chars, Campaigns: camps, Maps: h.vision, Roster: chars, Dice: testDice{camps}, Defaults: camps, Roller: h.roller, Live: liveConfig, Logger: logger, Now: clock.Now})
	if err != nil {
		t.Fatalf("New() error = %v", err)
	}
	h.svc, h.chars, h.camps = svc, chars, camps
	chars.SetCreatureHost(svc) // a creature dismissed leaves its combat, and its events and hints go through play (MR-037)
	srv := httpserver.New(httpserver.Config{Logger: logger})
	h.http = srv
	opt := connect.WithRequireConnectProtocolHeader()
	camps.Mount(srv.Handle, testSessions, opt)
	chars.Mount(srv.Handle, testSessions, camps, opt)
	svc.Mount(srv.Handle, testSessions, camps, opt)
	h.server = httptest.NewServer(srv.Handler())
	t.Cleanup(h.server.Close)
	t.Cleanup(svc.Close) // end the streams first, so the server can close
	return h
}

type user struct {
	id         string
	campaigns  campaignsv1connect.CampaignServiceClient
	characters charactersv1connect.CharacterServiceClient
	inventory  charactersv1connect.InventoryServiceClient
	play       playv1connect.PlayServiceClient
	combat     playv1connect.CombatServiceClient
	encounters playv1connect.EncounterServiceClient
	content    rulesv1connect.ContentServiceClient
	table      rulesv1connect.TableContentServiceClient
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
		campaigns:  campaignsv1connect.NewCampaignServiceClient(c, url),
		characters: charactersv1connect.NewCharacterServiceClient(c, url),
		inventory:  charactersv1connect.NewInventoryServiceClient(c, url),
		play:       playv1connect.NewPlayServiceClient(c, url),
		combat:     playv1connect.NewCombatServiceClient(c, url),
		encounters: playv1connect.NewEncounterServiceClient(c, url),
		content:    rulesv1connect.NewContentServiceClient(c, url),
		table:      rulesv1connect.NewTableContentServiceClient(c, url),
	}
}

// newCampaign creates a campaign whose master is master, lets the players
// in, and returns its ID.
func (h *harness) newCampaign(master *user, name string, players ...*user) string {
	h.t.Helper()
	ctx := h.t.Context()
	res, err := master.campaigns.CreateCampaign(ctx, connect.NewRequest(&campaignsv1.CreateCampaignRequest{Name: name, XpMode: campaignsv1.XpMode_XP_MODE_MILESTONES}))
	if err != nil {
		h.t.Fatalf("CreateCampaign() error = %v", err)
	}
	id := res.Msg.GetCampaign().GetId()
	h.join(master, id, players...)
	return id
}

func (h *harness) join(master *user, campaignID string, players ...*user) {
	h.t.Helper()
	ctx := h.t.Context()
	inv, err := master.campaigns.CreateInvite(ctx, connect.NewRequest(&campaignsv1.CreateInviteRequest{CampaignId: campaignID, MaxUses: campaigns.MaxInviteUses}))
	if err != nil {
		h.t.Fatalf("CreateInvite() error = %v", err)
	}
	for _, p := range players {
		if _, err := p.campaigns.AcceptInvite(ctx, connect.NewRequest(&campaignsv1.AcceptInviteRequest{Token: inv.Msg.GetToken()})); err != nil {
			h.t.Fatalf("AcceptInvite() error = %v", err)
		}
	}
}

// joinPending lets each player in as a pending member (RN-15, MR-024),
// through an invite from the master that requires approval.
func (h *harness) joinPending(master *user, campaignID string, players ...*user) {
	h.t.Helper()
	ctx := h.t.Context()
	inv, err := master.campaigns.CreateInvite(ctx, connect.NewRequest(&campaignsv1.CreateInviteRequest{
		CampaignId: campaignID, MaxUses: campaigns.MaxInviteUses, RequiresApproval: true,
	}))
	if err != nil {
		h.t.Fatalf("CreateInvite(requires_approval) error = %v", err)
	}
	for _, p := range players {
		res, err := p.campaigns.AcceptInvite(ctx, connect.NewRequest(&campaignsv1.AcceptInviteRequest{Token: inv.Msg.GetToken()}))
		if err != nil || !res.Msg.GetCampaign().GetAwaitingApproval() {
			h.t.Fatalf("AcceptInvite(requires_approval) = %v, %v; want a pending member", res, err)
		}
	}
}

// start calls StartGameSession as u, or fails the test.
func (u *user) start(t *testing.T, campaignID string) *playv1.StartGameSessionResponse {
	t.Helper()
	res, err := u.play.StartGameSession(t.Context(), connect.NewRequest(&playv1.StartGameSessionRequest{CampaignId: campaignID}))
	if err != nil {
		t.Fatalf("StartGameSession() error = %v", err)
	}
	return res.Msg
}

// end calls EndGameSession as u, or fails the test.
func (u *user) end(t *testing.T, s *playv1.GameSession) *playv1.GameSession {
	t.Helper()
	res, err := u.play.EndGameSession(t.Context(), connect.NewRequest(&playv1.EndGameSessionRequest{CampaignId: s.GetCampaignId(), GameSessionId: s.GetId()}))
	if err != nil {
		t.Fatalf("EndGameSession() error = %v", err)
	}
	return res.Msg.GetGameSession()
}

// createCharacter creates a character as u, or fails the test.
func (u *user) createCharacter(t *testing.T, campaignID string, kind charactersv1.CharacterKind, name string) *charactersv1.Character {
	t.Helper()
	sheet := &charactersv1.CharacterSheet{Content: &charactersv1.CharacterSheet_Full{Full: &charactersv1.FullSheet{
		BaseScores: &rulesv1.AbilityScores{Strength: 10, Dexterity: 14, Constitution: 12, Intelligence: 16, Wisdom: 10, Charisma: 8},
		RaceKey:    "race:gnome",
		Classes:    []*charactersv1.ClassLevel{{ClassKey: "class:wizard", Level: 1}},
	}}}
	if kind == charactersv1.CharacterKind_CHARACTER_KIND_MINION {
		sheet = &charactersv1.CharacterSheet{Content: &charactersv1.CharacterSheet_Basic{Basic: &charactersv1.BasicSheet{HitPointsMax: 7, ArmorClass: 12, SpeedFt: 30}}}
	}
	res, err := u.characters.CreateCharacter(t.Context(), connect.NewRequest(&charactersv1.CreateCharacterRequest{CampaignId: campaignID, Kind: kind, Name: name, Sheet: sheet}))
	if err != nil {
		t.Fatalf("CreateCharacter(%v) error = %v", kind, err)
	}
	return res.Msg.GetCharacter()
}

// character reads a character as u, or fails the test.
func (u *user) character(t *testing.T, c *charactersv1.Character) *charactersv1.Character {
	t.Helper()
	res, err := u.characters.GetCharacter(t.Context(), connect.NewRequest(&charactersv1.GetCharacterRequest{CampaignId: c.GetCampaignId(), CharacterId: c.GetId()}))
	if err != nil {
		t.Fatalf("GetCharacter() error = %v", err)
	}
	return res.Msg.GetCharacter()
}

func wantCode(t *testing.T, call string, err error, want connect.Code) {
	t.Helper()
	if got := connect.CodeOf(err); err == nil || got != want {
		t.Fatalf("%s error = %v, want %v", call, err, want)
	}
}

// testDice is the service's DiceModes over the campaigns service, as cmd/api
// wires it.
type testDice struct{ camps *campaigns.Service }

func (d testDice) ForcedDice(ctx context.Context, tx pgx.Tx, campaignID, userID string) (DiceForce, error) {
	mode, err := d.camps.CampaignDiceMode(ctx, tx, campaignID, userID)
	switch mode {
	case campaigns.DiceModeApp:
		return DiceForcedInApp, err
	case campaigns.DiceModePhysical:
		return DiceForcedPhysical, err
	}
	return DiceChoice, err
}

// scriptedRoller gives the faces a test queued, in order, and a 10 (or the
// highest face of a smaller die) when none is left, so a test sets only the
// rolls it cares about.
type scriptedRoller struct {
	mu    sync.Mutex
	faces []int
}

func (r *scriptedRoller) queue(faces ...int) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.faces = append(r.faces, faces...)
}

func (r *scriptedRoller) Roll(sides int) (int, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	if len(r.faces) == 0 {
		return min(10, sides), nil
	}
	face := r.faces[0]
	r.faces = r.faces[1:]
	return face, nil
}

// visionCounter is the maps seam with a count of the fog refreshes play asks for.
type visionCounter struct {
	*maps.SessionMaps
	n atomic.Int32
}

func (v *visionCounter) VisionChanged(ctx context.Context, campaignID, mapID string) {
	v.n.Add(1)
	v.SessionMaps.VisionChanged(ctx, campaignID, mapID)
}
