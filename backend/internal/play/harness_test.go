package play

import (
	"context"
	"crypto/rand"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
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
	"github.com/PuraFome/meuRPG/backend/internal/platform/blob"
	"github.com/PuraFome/meuRPG/backend/internal/platform/choicetest"
	"github.com/PuraFome/meuRPG/backend/internal/platform/dbtest"
	"github.com/PuraFome/meuRPG/backend/internal/platform/dice/dicetest"
	"github.com/PuraFome/meuRPG/backend/internal/platform/hinttest"
	"github.com/PuraFome/meuRPG/backend/internal/platform/httpserver"
	"github.com/PuraFome/meuRPG/backend/internal/platform/reqlimit"
	"github.com/PuraFome/meuRPG/backend/internal/play/voice"
	"github.com/PuraFome/meuRPG/backend/internal/rules"
)

// The database tests run against CockroachDB with the real campaigns and
// characters services next to this one, as cmd/api wires them: set
// MEURPG_TEST_DATABASE_URL (see package dbtest). Without it they skip.

var testRules = sync.OnceValues(rules.LoadSRD)

// testUserHeader names the signed-in user in a test request. fakeSessions
// trusts it; production code has no way to set a caller (see
// docs/architecture/identity.md#who-is-calling).
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

// AuthenticateRequest finds the caller of a plain HTTP request (the audio route), like identity's.
func (fakeSessions) AuthenticateRequest(r *http.Request) (context.Context, error) {
	ctx := withTestUser(r.Context(), r.Header)
	if _, ok := ctx.Value(testUserKey{}).(string); !ok {
		return nil, errSignIn()
	}
	return ctx, nil
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

// advance moves the clock on: a test waits by this, never by sleeping.
func (c *fakeClock) advance(d time.Duration) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.now = c.now.Add(d)
}

type harness struct {
	hints  hinttest.Tracker // the sends of the maps service's hint gate that are still pending
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
	clock  *fakeClock     // the service's clock, which a test moves on instead of waiting
	voice  *voice.Fake    // makes the lines' audio; counts the voices it made
	vitals *vitalsReads   // the vitals keeper the service uses: the characters' service, counting the vitals it derived
}

// newHarness serves the play, campaigns and characters services through
// the API's real HTTP stack (httpserver: request logging, cross-origin
// protection), as cmd/api does. live, when given, sets the stream's timing.
func newHarness(t *testing.T, live ...LiveConfig) *harness {
	t.Helper()
	pool := dbtest.NewPool(t, "meurpg_play_test")
	h := &harness{t: t, pool: pool, users: identity.NewPostgresStore(pool), roller: &scriptedRoller{}}
	clock := &fakeClock{now: time.Now().Truncate(time.Microsecond)}
	h.clock = clock
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
	chars, err := characters.New(characters.Config{Pool: pool, Profiles: h.users, Members: camps, Content: contenttest.NewSource(pool, content), SRD: content, Roller: dicetest.New(), Logger: logger, Now: clock.Now})
	if err != nil {
		t.Fatalf("characters.New() error = %v", err)
	}
	h.vitals = &vitalsReads{VitalsKeeper: chars}
	h.vision = &visionCounter{SessionMaps: maps.NewSessionMaps(pool)}
	chars.SetGallery(maps.NewSessionMaps(pool)) // an NPC's portrait is an image of the gallery (MR-031)
	blobs, err := blob.NewFS(t.TempDir())
	if err != nil {
		t.Fatalf("blob.NewFS() error = %v", err)
	}
	rel := &releaser{}
	t.Cleanup(rel.run)
	rel.add(func() { _ = blobs.Close() })
	h.voice = &voice.Fake{}
	messages := MessagesConfig{Profiles: h.users, Voice: h.voice, Blobs: blobs}
	svc, err := New(Config{Messages: messages, Pool: pool, Sheets: chars, Vitals: h.vitals, Campaigns: camps, Maps: h.vision, Roster: chars, Dice: testDice{camps}, Defaults: camps, Roller: h.roller, Live: liveConfig, Logger: logger, Now: clock.Now})
	if err != nil {
		t.Fatalf("New() error = %v", err)
	}
	h.svc, h.chars, h.camps = svc, chars, camps
	chars.SetCreatureHost(svc) // a creature dismissed leaves its combat, and its events and hints go through play (MR-037)
	chars.SetReviewHost(svc)
	srv := httpserver.New(httpserver.Config{Logger: logger})
	h.http = srv
	opt := reqlimit.HandlerOption()
	camps.Mount(srv.Handle, testSessions, opt)
	chars.Mount(srv.Handle, testSessions, camps, opt)
	svc.Mount(srv.Handle, testSessions, camps, opt)
	h.server = httptest.NewServer(srv.Handler())
	rel.add(h.server.Close)
	rel.add(svc.Close) // end the streams first, so the server can close
	return h
}

// releaser runs the cleanups of a harness once, the last added first.
// Add a cleanup that holds the services with rel.add, never with t.Cleanup.
// A finished parallel subtest stays reachable from its parent until the parent returns, and so does every function it gave to t.Cleanup.
type releaser struct{ fns []func() }

func (r *releaser) add(f func()) { r.fns = append(r.fns, f) }

func (r *releaser) run() {
	fns := r.fns
	r.fns = nil
	for i := len(fns) - 1; i >= 0; i-- {
		fns[i]()
	}
}

type user struct {
	id         string
	campaigns  campaignsv1connect.CampaignServiceClient
	characters charactersv1connect.CharacterServiceClient
	play       playv1connect.PlayServiceClient
	combat     playv1connect.CombatServiceClient
	contests   playv1connect.ContestServiceClient
	lasting    playv1connect.LastingEffectServiceClient
	resource   playv1connect.ResourceServiceClient
	casting    playv1connect.CastingServiceClient
	encounters playv1connect.EncounterServiceClient
	revivify   playv1connect.RevivifyServiceClient
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
		characters: charactersv1connect.NewCharacterServiceClient(c, url, connect.WithInterceptors(choicetest.Interceptor(charactersv1connect.NewCharacterServiceClient(c, url)))),
		play:       playv1connect.NewPlayServiceClient(c, url),
		combat:     playv1connect.NewCombatServiceClient(c, url),
		contests:   playv1connect.NewContestServiceClient(c, url),
		lasting:    playv1connect.NewLastingEffectServiceClient(c, url),
		resource:   playv1connect.NewResourceServiceClient(c, url),
		casting:    playv1connect.NewCastingServiceClient(c, url),
		encounters: playv1connect.NewEncounterServiceClient(c, url),
		revivify:   playv1connect.NewRevivifyServiceClient(c, url),
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

// wantRefusal fails unless err is a connect error with this code and exactly this message. An empty
// msg checks the code only.
func wantRefusal(t *testing.T, call string, err error, code connect.Code, msg string) {
	t.Helper()
	ce, ok := errors.AsType[*connect.Error](err)
	if !ok {
		t.Errorf("%s: error = %v, want a connect error %v %q", call, err, code, msg)
		return
	}
	if ce.Code() != code || (msg != "" && ce.Message() != msg) {
		t.Errorf("%s: error = %v %q, want %v %q", call, ce.Code(), ce.Message(), code, msg)
	}
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

// afterThePlayersChance moves the clock past the time a player has to answer a question before the master may (the
// test clock: nothing waits).
func (h *harness) afterThePlayersChance() { h.clock.advance(heldPlayerFirst + time.Second) }

// goldenBlocks collects what the scenarios of a test read, by name.
type goldenBlocks struct {
	mu     sync.Mutex
	blocks map[string]string
}

func (g *goldenBlocks) put(name, text string) {
	g.mu.Lock()
	defer g.mu.Unlock()
	if g.blocks == nil {
		g.blocks = map[string]string{}
	}
	g.blocks[name] = text
}

// settle compares the blocks with the golden file, or writes it when UPDATE_GOLDEN is set.
func (g *goldenBlocks) settle(t *testing.T, path string) {
	t.Helper()
	names := make([]string, 0, len(g.blocks))
	for n := range g.blocks {
		names = append(names, n)
	}
	sort.Strings(names)
	var sb strings.Builder
	for _, n := range names {
		sb.WriteString("== " + n + " ==\n" + g.blocks[n] + "\n")
	}
	if os.Getenv("UPDATE_GOLDEN") != "" {
		if err := os.WriteFile(path, []byte(sb.String()), 0o600); err != nil {
			t.Fatalf("write %s: %v", path, err)
		}
		return
	}
	want, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read %s: %v (run with UPDATE_GOLDEN=1)", path, err)
	}
	if string(want) == sb.String() {
		return
	}
	wantBlocks := map[string]string{}
	for _, part := range strings.Split(string(want), "== ")[1:] {
		head, body, _ := strings.Cut(part, " ==\n")
		wantBlocks[head] = strings.TrimSuffix(body, "\n")
	}
	for _, n := range names {
		if w, ok := wantBlocks[n]; !ok {
			t.Errorf("scenario %q is not in %s", n, path)
		} else if w != g.blocks[n] {
			t.Errorf("scenario %q reads:\n%s\nwant:\n%s", n, g.blocks[n], w)
		}
	}
	for n := range wantBlocks {
		if _, ok := g.blocks[n]; !ok {
			t.Errorf("scenario %q of %s did not run", n, path)
		}
	}
}

// labelsOf names the combatants of the combat by id.
func labelsOf(e *playv1.Encounter) map[string]string {
	names := map[string]string{}
	for _, c := range e.GetCombatants() {
		names[c.GetId()] = c.GetLabel()
	}
	return names
}

// goldenScenario is one scenario of a golden: it plays a fight of its own and returns what it reads.
type goldenScenario struct {
	name string
	run  func(t *testing.T) string
}

// runGolden runs every scenario on a fight of its own and compares what each reads with the golden file
// at path, or writes the file when UPDATE_GOLDEN is set.
func runGolden(t *testing.T, path string, scenarios []goldenScenario) {
	t.Helper()
	g := &goldenBlocks{}
	t.Run("scenarios", func(t *testing.T) {
		for _, sc := range scenarios {
			t.Run(sc.name, func(t *testing.T) {
				t.Parallel()
				g.put(sc.name, sc.run(t))
			})
		}
	})
	g.settle(t, path)
}

// wantGolden compares a whole text with the golden file at path and names the first line that differs,
// or writes the file when UPDATE_GOLDEN is set.
func wantGolden(t *testing.T, path, got string) {
	t.Helper()
	if os.Getenv("UPDATE_GOLDEN") != "" {
		if err := os.MkdirAll(filepath.Dir(path), 0o750); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(path, []byte(got), 0o600); err != nil {
			t.Fatalf("write %s: %v", path, err)
		}
		return
	}
	want, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read %s: %v (run with UPDATE_GOLDEN=1)", path, err)
	}
	if string(want) == got {
		return
	}
	gl, wl := strings.Split(got, "\n"), strings.Split(string(want), "\n")
	for i := 0; i < len(gl) && i < len(wl); i++ {
		if gl[i] != wl[i] {
			t.Fatalf("%s differs at line %d:\n got: %s\nwant: %s", path, i+1, gl[i], wl[i])
		}
	}
	t.Fatalf("the text has %d lines, %s has %d", len(gl), path, len(wl))
}

// stateTables are the tables a golden prints, which markCall snapshots to tell a call that changed nothing.
var stateTables = []string{
	"combatants", "combatant_states", "character_vitals", "character_wild_shapes", "pending_damages",
	"reaction_windows", "reaction_holds", "hit_riders", "roll_holds", "inspiration_holds", "hidden_reveals", "spell_casts",
}

// stateSnapshot is the rows of the stateTables and the number of events as one text.
func (a *armed) stateSnapshot(t *testing.T) string {
	t.Helper()
	var sb strings.Builder
	for _, tb := range stateTables {
		var rows string
		if err := a.h.pool.QueryRow(t.Context(), `SELECT COALESCE(string_agg(x::TEXT, E'\n' ORDER BY x::TEXT), '') FROM `+tb+` x`).Scan(&rows); err != nil {
			t.Fatalf("snapshot %s: %v", tb, err)
		}
		sb.WriteString(tb + "\n" + rows + "\n")
	}
	fmt.Fprintf(&sb, "events %d", a.events(t))
	return sb.String()
}

// markCall is the number of events a fight has written so far. It also keeps the state of the tables a
// golden prints, so that the report after the call can tell a call that changed nothing.
func (a *armed) markCall(t *testing.T) int {
	t.Helper()
	a.callState = a.stateSnapshot(t)
	return a.events(t)
}

// printedState is the state lines a report prints after a call. When collapse is set and the call left
// every table as markCall found it, it is one line, which keeps the check that the call wrote nothing.
func (a *armed) printedState(t *testing.T, collapse bool, lines []string) []string {
	t.Helper()
	if collapse && a.callState == a.stateSnapshot(t) {
		return []string{"state unchanged"}
	}
	return lines
}

// flatJSON is a JSON text as one entry per leaf, keyed by its path. The elements of a list are keyed by the
// first of their key, attackKey, combatantId or kind fields (looking one object down), so that an element
// inserted in the list does not move the keys of the others.
func flatJSON(text string) map[string]string {
	var v any
	if err := json.Unmarshal([]byte(text), &v); err != nil {
		return map[string]string{"": text}
	}
	out := map[string]string{}
	flatten(out, "", v)
	return out
}

func flatten(out map[string]string, path string, v any) {
	switch x := v.(type) {
	case map[string]any:
		for k, c := range x {
			flatten(out, path+"."+k, c)
		}
	case []any:
		if len(x) == 0 {
			out[path] = "[]"
		}
		for i, c := range x {
			key := strconv.Itoa(i)
			if id := elementID(c); id != "" {
				key = id
			}
			if _, dup := out[path+"["+key+"]"]; dup {
				key += "#" + strconv.Itoa(i)
			}
			flatten(out, path+"["+key+"]", c)
		}
	default:
		raw, _ := json.Marshal(x)
		out[path] = string(raw)
	}
}

// elementID is the identity of a list element, or "" when it has none.
func elementID(v any) string {
	m, ok := v.(map[string]any)
	if !ok {
		return ""
	}
	for _, k := range []string{"key", "attackKey", "combatantId", "kind"} {
		if s, ok := m[k].(string); ok {
			return s
		}
	}
	for _, c := range m {
		if inner, ok := c.(map[string]any); ok {
			if s, ok := inner["key"].(string); ok {
				return s
			}
		}
	}
	return ""
}

// flatLines are the entries of a flatJSON, sorted, one "path = value" line each.
func flatLines(flat map[string]string) []string {
	out := make([]string, 0, len(flat))
	for k, v := range flat {
		out = append(out, k+" = "+v)
	}
	sort.Strings(out)
	return out
}

// diffLines is how cur differs from base, one line each: "+" for a leaf that base lacks, "~" for one that
// has another value and "-" for one that cur lacks. It is the empty text when they are the same.
func diffLines(base, cur map[string]string) string {
	var out []string
	for k, v := range cur {
		if bv, ok := base[k]; !ok {
			out = append(out, "+ "+k+" = "+v)
		} else if bv != v {
			out = append(out, "~ "+k+" = "+v)
		}
	}
	for k := range base {
		if _, ok := cur[k]; !ok {
			out = append(out, "- "+k)
		}
	}
	sort.Strings(out)
	if len(out) == 0 {
		return " (no difference)"
	}
	return "\n" + strings.Join(out, "\n")
}
