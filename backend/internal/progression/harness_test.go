package progression

import (
	"context"
	"crypto/rand"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"slices"
	"sync"
	"testing"
	"time"
	"uuid"

	"connectrpc.com/connect"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgxpool"

	campaignsv1 "github.com/PuraFome/meuRPG/backend/gen/meurpg/campaigns/v1"
	"github.com/PuraFome/meuRPG/backend/gen/meurpg/campaigns/v1/campaignsv1connect"
	charactersv1 "github.com/PuraFome/meuRPG/backend/gen/meurpg/characters/v1"
	"github.com/PuraFome/meuRPG/backend/gen/meurpg/characters/v1/charactersv1connect"
	"github.com/PuraFome/meuRPG/backend/gen/meurpg/play/v1/playv1connect"
	progressionv1 "github.com/PuraFome/meuRPG/backend/gen/meurpg/progression/v1"
	"github.com/PuraFome/meuRPG/backend/gen/meurpg/progression/v1/progressionv1connect"
	rulesv1 "github.com/PuraFome/meuRPG/backend/gen/meurpg/rules/v1"
	"github.com/PuraFome/meuRPG/backend/internal/campaigns"
	"github.com/PuraFome/meuRPG/backend/internal/characters"
	"github.com/PuraFome/meuRPG/backend/internal/characters/contenttest"
	"github.com/PuraFome/meuRPG/backend/internal/identity"
	"github.com/PuraFome/meuRPG/backend/internal/maps"
	"github.com/PuraFome/meuRPG/backend/internal/platform/dbtest"
	"github.com/PuraFome/meuRPG/backend/internal/platform/dice"
	"github.com/PuraFome/meuRPG/backend/internal/platform/dice/dicetest"
	"github.com/PuraFome/meuRPG/backend/internal/platform/httpserver"
	"github.com/PuraFome/meuRPG/backend/internal/platform/reqlimit"
	"github.com/PuraFome/meuRPG/backend/internal/play"
	"github.com/PuraFome/meuRPG/backend/internal/rules"
)

// The database tests run against CockroachDB with the real campaigns,
// characters and play services next to this one, as cmd/api wires them: set
// MEURPG_TEST_DATABASE_URL (see package dbtest). Without it they skip.

var testRules = sync.OnceValues(rules.LoadSRD)

// testUserHeader names the signed-in user in a test request. fakeSessions
// trusts it; production code has no way to set a caller (see
// docs/architecture/identity.md#who-is-calling).
const testUserHeader = "Test-User-Id"

type fakeSessions struct{}

type testUserKey struct{}

func errSignIn() error {
	return connect.NewError(connect.CodeUnauthenticated, errors.New("sign in to continue"))
}

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
	_, err := f.UserID(ctx)
	return err
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
	play   *play.Service
	roller dice.Roller // the dice play rolls with
	server *httptest.Server
	// cfg and camps are what the progression service was made from, so a test can make another over a pool of its own.
	cfg   Config
	camps *campaigns.Service
	// treasures is the maps' treasures, noting what is published about maps.
	treasures *treasureSpy
	// beforeAward, when set, runs once after an award's checks outside the
	// transaction and before the transaction opens: the window where another
	// request commits.
	beforeAward func(ctx context.Context)
}

// published is one PublishChanged call.
type published struct {
	campaignID string
	mapIDs     []string
}

// treasureSpy is maps.Treasures that notes what progression publishes about
// maps, and can fail the next conversion with a serialization conflict.
type treasureSpy struct {
	*maps.Treasures
	mu        sync.Mutex
	calls     []published
	conflicts int // MarkConverted fails with 40001 this many more times
}

func (s *treasureSpy) MarkConverted(ctx context.Context, tx pgx.Tx, awardID string, pointIDs []string) error {
	s.mu.Lock()
	conflict := s.conflicts > 0
	if conflict {
		s.conflicts--
	}
	s.mu.Unlock()
	if err := s.Treasures.MarkConverted(ctx, tx, awardID, pointIDs); err != nil {
		return err
	}
	if conflict {
		return fmt.Errorf("convert the treasures: %w", &pgconn.PgError{Code: "40001", Message: "restart transaction"})
	}
	return nil
}

func (s *treasureSpy) PublishChanged(campaignID string, mapIDs []string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.calls = append(s.calls, published{campaignID: campaignID, mapIDs: mapIDs})
}

// published returns the calls so far.
func (s *treasureSpy) published() []published {
	s.mu.Lock()
	defer s.mu.Unlock()
	return slices.Clone(s.calls)
}

// racingCampaigns is the real campaigns service with a hook after the XP mode
// is read outside a transaction, the last read before an award's transaction.
type racingCampaigns struct {
	*campaigns.Service
	h *harness
}

func (c racingCampaigns) CampaignXPMode(ctx context.Context, tx pgx.Tx, campaignID string) (campaignsv1.XpMode, error) {
	mode, err := c.Service.CampaignXPMode(ctx, tx, campaignID)
	if hook := c.h.beforeAward; hook != nil && tx == nil && err == nil {
		c.h.beforeAward = nil
		hook(ctx)
	}
	return mode, err
}

// newHarness serves the progression, play, campaigns and characters services
// through the API's real HTTP stack, wired as cmd/api wires them.
func newHarness(t *testing.T) *harness {
	t.Helper()
	return newHarnessRolling(t, dicetest.New())
}

// newHarnessRolling is newHarness with the dice the play service rolls with.
func newHarnessRolling(t *testing.T, roller dice.Roller) *harness {
	t.Helper()
	pool := dbtest.NewPool(t, "meurpg_progression_test")
	h := &harness{t: t, pool: pool, users: identity.NewPostgresStore(pool), roller: roller}
	clock := &fakeClock{now: time.Now().Truncate(time.Microsecond)}
	logger := slog.New(slog.DiscardHandler)
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
	pl, err := play.New(play.Config{
		Pool: pool, Sheets: chars, Vitals: chars, Campaigns: camps, Maps: maps.NewSessionMaps(pool), Roster: chars,
		Dice: testDice{camps}, Roller: roller, Logger: logger, Now: clock.Now,
	})
	if err != nil {
		t.Fatalf("play.New() error = %v", err)
	}
	h.play = pl
	h.camps = camps
	h.treasures = &treasureSpy{Treasures: maps.NewTreasures(pool)}
	h.cfg = Config{Pool: pool, Party: chars, Combats: pl, Log: pl, Treasures: h.treasures, Campaigns: racingCampaigns{Service: camps, h: h}, Profiles: h.users, Logger: logger, Now: clock.Now}
	svc, err := New(h.cfg)
	if err != nil {
		t.Fatalf("New() error = %v", err)
	}
	chars.SetLevelUps(svc)
	camps.SetXPAwards(svc) // changing the XP mode asks once XP was awarded (RN-09)
	srv := httpserver.New(httpserver.Config{Logger: logger})
	opt := reqlimit.HandlerOption()
	var sessions Sessions = fakeSessions{}
	camps.Mount(srv.Handle, sessions.(campaigns.Sessions), opt)
	chars.Mount(srv.Handle, sessions.(characters.Sessions), camps, opt)
	pl.Mount(srv.Handle, sessions.(play.Sessions), camps, opt)
	svc.Mount(srv.Handle, sessions, camps, opt)
	h.server = httptest.NewServer(srv.Handler())
	t.Cleanup(h.server.Close)
	t.Cleanup(pl.Close) // end the streams first, so the server can close
	return h
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

type user struct {
	id         string
	campaigns  campaignsv1connect.CampaignServiceClient
	characters charactersv1connect.CharacterServiceClient
	play       playv1connect.PlayServiceClient
	combat     playv1connect.CombatServiceClient
	xp         progressionv1connect.ProgressionServiceClient
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
		play:       playv1connect.NewPlayServiceClient(c, url),
		combat:     playv1connect.NewCombatServiceClient(c, url),
		xp:         progressionv1connect.NewProgressionServiceClient(c, url),
	}
}

// newCampaign creates a campaign that levels by mode, whose master is master,
// lets the players in, and returns its ID.
func (h *harness) newCampaign(master *user, name string, mode campaignsv1.XpMode, players ...*user) string {
	h.t.Helper()
	ctx := h.t.Context()
	res, err := master.campaigns.CreateCampaign(ctx, connect.NewRequest(&campaignsv1.CreateCampaignRequest{Name: name, XpMode: mode}))
	if err != nil {
		h.t.Fatalf("CreateCampaign() error = %v", err)
	}
	id := res.Msg.GetCampaign().GetId()
	inv, err := master.campaigns.CreateInvite(ctx, connect.NewRequest(&campaignsv1.CreateInviteRequest{CampaignId: id, MaxUses: campaigns.MaxInviteUses}))
	if err != nil {
		h.t.Fatalf("CreateInvite() error = %v", err)
	}
	for _, p := range players {
		if _, err := p.campaigns.AcceptInvite(ctx, connect.NewRequest(&campaignsv1.AcceptInviteRequest{Token: inv.Msg.GetToken()})); err != nil {
			h.t.Fatalf("AcceptInvite() error = %v", err)
		}
	}
	return id
}

// joinPending lets a player in as a pending member (RN-15).
func (h *harness) joinPending(master *user, campaignID string, p *user) {
	h.t.Helper()
	ctx := h.t.Context()
	inv, err := master.campaigns.CreateInvite(ctx, connect.NewRequest(&campaignsv1.CreateInviteRequest{
		CampaignId: campaignID, MaxUses: campaigns.MaxInviteUses, RequiresApproval: true,
	}))
	if err != nil {
		h.t.Fatalf("CreateInvite(requires_approval) error = %v", err)
	}
	if _, err := p.campaigns.AcceptInvite(ctx, connect.NewRequest(&campaignsv1.AcceptInviteRequest{Token: inv.Msg.GetToken()})); err != nil {
		h.t.Fatalf("AcceptInvite(requires_approval) error = %v", err)
	}
}

// pc creates u's player character (a level 1 gnome wizard), or fails the test.
func (u *user) pc(t *testing.T, campaignID, name string) *charactersv1.Character {
	t.Helper()
	sheet := &charactersv1.CharacterSheet{Content: &charactersv1.CharacterSheet_Full{Full: &charactersv1.FullSheet{
		BaseScores: &rulesv1.AbilityScores{Strength: 10, Dexterity: 14, Constitution: 12, Intelligence: 16, Wisdom: 10, Charisma: 8},
		RaceKey:    "race:gnome",
		Classes:    []*charactersv1.ClassLevel{{ClassKey: "class:wizard", Level: 1}},
	}}}
	return u.create(t, campaignID, charactersv1.CharacterKind_CHARACTER_KIND_PLAYER, name, sheet)
}

// minion creates a minion NPC that gives xp when defeated, or fails the test.
func (u *user) minion(t *testing.T, campaignID, name string, xp int32) *charactersv1.Character {
	t.Helper()
	sheet := &charactersv1.CharacterSheet{Content: &charactersv1.CharacterSheet_Basic{Basic: &charactersv1.BasicSheet{
		HitPointsMax: 7, ArmorClass: 12, SpeedFt: 30, ChallengeRating: "1/4", XpValue: xp,
	}}}
	return u.create(t, campaignID, charactersv1.CharacterKind_CHARACTER_KIND_MINION, name, sheet)
}

func (u *user) create(t *testing.T, campaignID string, kind charactersv1.CharacterKind, name string, sheet *charactersv1.CharacterSheet) *charactersv1.Character {
	t.Helper()
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

func newKey() string { return uuid.New().String() }

// wantCode fails the test unless err has the Connect code.
func wantCode(t *testing.T, call string, err error, want connect.Code) {
	t.Helper()
	if got := connect.CodeOf(err); err == nil || got != want {
		t.Fatalf("%s error = %v, want %v", call, err, want)
	}
}

// wantBlocked fails the test unless err is a failed_precondition with the
// XPBlocked reason.
func wantBlocked(t *testing.T, call string, err error, want progressionv1.XPBlockedReason) {
	t.Helper()
	wantCode(t, call, err, connect.CodeFailedPrecondition)
	ce, _ := errors.AsType[*connect.Error](err)
	for _, d := range ce.Details() {
		if v, derr := d.Value(); derr == nil {
			if b, ok := v.(*progressionv1.XPBlocked); ok {
				if b.GetReason() != want {
					t.Fatalf("%s: XPBlocked reason = %v, want %v", call, b.GetReason(), want)
				}
				return
			}
		}
	}
	t.Fatalf("%s: no XPBlocked detail in %v", call, err)
}

// award calls AwardXP as u.
func (u *user) award(campaignID string, edit func(*progressionv1.AwardXPRequest)) (*progressionv1.AwardXPResponse, error) {
	req := &progressionv1.AwardXPRequest{CampaignId: campaignID, Reason: "Pela ajuda ao ferreiro", IdempotencyKey: newKey()}
	edit(req)
	res, err := u.xp.AwardXP(context.Background(), connect.NewRequest(req))
	if err != nil {
		return nil, err
	}
	return res.Msg, nil
}

// manual gives amount XP to the characters, or fails the test.
func (u *user) manual(t *testing.T, campaignID string, amount int32, ids ...string) *progressionv1.AwardXPResponse {
	t.Helper()
	res, err := u.award(campaignID, func(r *progressionv1.AwardXPRequest) {
		r.Mode, r.Amount, r.CharacterIds = progressionv1.XPAwardMode_XP_AWARD_MODE_MANUAL, amount, ids
	})
	if err != nil {
		t.Fatalf("AwardXP(manual %d) error = %v", amount, err)
	}
	return res
}

// milestone marks a milestone, or fails the test.
func (u *user) milestone(t *testing.T, campaignID string, ids ...string) *progressionv1.XPAward {
	t.Helper()
	res, err := u.xp.MarkMilestone(t.Context(), connect.NewRequest(&progressionv1.MarkMilestoneRequest{
		CampaignId: campaignID, Reason: "Derrotaram o chefe", CharacterIds: ids, IdempotencyKey: newKey(),
	}))
	if err != nil {
		t.Fatalf("MarkMilestone() error = %v", err)
	}
	return res.Msg.GetAward()
}

// undo calls UndoLastXPAward as u.
func (u *user) undo(campaignID, key string) (*progressionv1.XPAward, error) {
	res, err := u.xp.UndoLastXPAward(context.Background(), connect.NewRequest(&progressionv1.UndoLastXPAwardRequest{CampaignId: campaignID, IdempotencyKey: key}))
	if err != nil {
		return nil, err
	}
	return res.Msg.GetAward(), nil
}

// xpOf is the XP on a character's sheet, as the master reads it.
func (u *user) xpOf(t *testing.T, c *charactersv1.Character) int32 {
	t.Helper()
	return u.character(t, c).GetSheet().GetFull().GetExperiencePoints()
}

// history lists the campaign's awards as u, or fails the test.
func (u *user) history(t *testing.T, campaignID string) []*progressionv1.XPAward {
	t.Helper()
	res, err := u.xp.ListXPAwards(t.Context(), connect.NewRequest(&progressionv1.ListXPAwardsRequest{CampaignId: campaignID}))
	if err != nil {
		t.Fatalf("ListXPAwards() error = %v", err)
	}
	return res.Msg.GetAwards()
}

// experience reads the campaign's experience as u, or fails the test.
func (u *user) experience(t *testing.T, campaignID string) *progressionv1.GetCampaignExperienceResponse {
	t.Helper()
	res, err := u.xp.GetCampaignExperience(t.Context(), connect.NewRequest(&progressionv1.GetCampaignExperienceRequest{CampaignId: campaignID}))
	if err != nil {
		t.Fatalf("GetCampaignExperience() error = %v", err)
	}
	return res.Msg
}
