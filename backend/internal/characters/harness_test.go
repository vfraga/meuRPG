package characters

import (
	"context"
	"crypto/rand"
	"errors"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"sync"
	"testing"
	"time"

	"connectrpc.com/connect"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	campaignsv1 "github.com/PuraFome/meuRPG/backend/gen/meurpg/campaigns/v1"
	"github.com/PuraFome/meuRPG/backend/gen/meurpg/campaigns/v1/campaignsv1connect"
	charactersv1 "github.com/PuraFome/meuRPG/backend/gen/meurpg/characters/v1"
	"github.com/PuraFome/meuRPG/backend/gen/meurpg/characters/v1/charactersv1connect"
	rulesv1 "github.com/PuraFome/meuRPG/backend/gen/meurpg/rules/v1"
	"github.com/PuraFome/meuRPG/backend/gen/meurpg/rules/v1/rulesv1connect"
	"github.com/PuraFome/meuRPG/backend/internal/campaigns"
	"github.com/PuraFome/meuRPG/backend/internal/identity"
	"github.com/PuraFome/meuRPG/backend/internal/platform/db"
	"github.com/PuraFome/meuRPG/backend/internal/platform/dbtest"
	"github.com/PuraFome/meuRPG/backend/internal/rules"
)

// The tests in *_db_test.go files run against CockroachDB, because the rules
// they check live in SQL transactions and constraints: set
// MEURPG_TEST_DATABASE_URL (see package dbtest). Without it they skip.

// testRules is the SRD content, loaded once for every test in the package.
var testRules = sync.OnceValues(rules.LoadSRD)

func loadRules(t *testing.T) *rules.Content {
	t.Helper()
	c, err := testRules()
	if err != nil {
		t.Fatalf("rules.LoadSRD() error = %v", err)
	}
	return c
}

// testUserHeader names the signed-in user in a test request. It exists only
// in these tests: fakeSessions stands in for the identity module, whose
// interceptor reads a session cookie and looks it up in the database.
// Everything after it (the authz interceptor, the handlers, the SQL) is the
// production code.
const testUserHeader = "Test-User-Id"

// fakeSessions implements Sessions for tests: its interceptor trusts the
// Test-User-Id header, and UserID reads it back. The context key is private
// to these tests; production code has no way to set a caller.
type fakeSessions struct{}

type testUserKey struct{}

var testSessions Sessions = fakeSessions{}

func (fakeSessions) Interceptor() connect.Interceptor {
	return connect.UnaryInterceptorFunc(func(next connect.UnaryFunc) connect.UnaryFunc {
		return func(ctx context.Context, req connect.AnyRequest) (connect.AnyResponse, error) {
			if userID := req.Header().Get(testUserHeader); userID != "" {
				ctx = context.WithValue(ctx, testUserKey{}, userID)
			}
			return next(ctx, req)
		}
	})
}

func (fakeSessions) UserID(ctx context.Context) (string, error) {
	if userID, ok := ctx.Value(testUserKey{}).(string); ok {
		return userID, nil
	}
	return "", connect.NewError(connect.CodeUnauthenticated, errors.New("sign in to continue"))
}

// fakeClock is a clock the test moves by hand. It also ticks one
// microsecond (the database's precision) every time it is read, so rows
// created one after the other have different times, and lists ordered by
// time come out in creation order.
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

func (c *fakeClock) Advance(d time.Duration) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.now = c.now.Add(d)
}

// harness is the characters service on a fresh, migrated database, served
// over real HTTP the way cmd/api serves it, next to the real campaigns
// service, which creates the campaigns, lets players in and says who is a
// member (authz.MembershipSource).
type harness struct {
	t      *testing.T
	pool   *pgxpool.Pool
	users  *identity.PostgresStore
	clock  *fakeClock
	svc    *Service
	server *httptest.Server
}

func newHarness(t *testing.T) *harness { return newHarnessWith(t, nil) }

// newHarnessWith is newHarness with a say in the service's Config (the
// level-up tests set the dice setting and the die).
func newHarnessWith(t *testing.T, tweak func(*Config)) *harness {
	t.Helper()
	t.Helper()
	pool := dbtest.NewPool(t, "meurpg_characters_test")
	h := &harness{
		t:     t,
		pool:  pool,
		users: identity.NewPostgresStore(pool),
		// The database keeps microseconds; truncating keeps timestamps
		// equal after a round trip.
		clock: &fakeClock{now: time.Now().Truncate(time.Microsecond)},
	}
	logger := slog.New(slog.DiscardHandler)
	camps, err := campaigns.New(campaigns.Config{Pool: pool, Profiles: h.users, Logger: logger, Now: h.clock.Now})
	if err != nil {
		t.Fatalf("campaigns.New() error = %v", err)
	}
	srd := loadRules(t)
	cfg := Config{Pool: pool, Profiles: h.users, Members: camps, Content: NewTableSource(pool, srd, camps), SRD: srd, Logger: logger, Now: h.clock.Now}
	if tweak != nil {
		tweak(&cfg)
	}
	h.svc, err = New(cfg)
	if err != nil {
		t.Fatalf("New() error = %v", err)
	}
	camps.SetCharacters(h.svc) // an ordinary invite approves a pending member's character (Q25)
	mux := http.NewServeMux()
	camps.Mount(mux.Handle, testSessions, connect.WithRequireConnectProtocolHeader())
	h.svc.Mount(mux.Handle, testSessions, camps, connect.WithRequireConnectProtocolHeader())
	h.server = httptest.NewServer(mux)
	t.Cleanup(h.server.Close)
	return h
}

// user is a signed-in account (or, with an empty id, nobody) with its own
// API clients.
type user struct {
	id        string
	campaigns campaignsv1connect.CampaignServiceClient
	api       charactersv1connect.CharacterServiceClient
	inventory charactersv1connect.InventoryServiceClient
	content   rulesv1connect.ContentServiceClient
	table     rulesv1connect.TableContentServiceClient
}

// newUser creates an account, as a first sign-in would, with a display
// name.
func (h *harness) newUser(displayName string) *user {
	h.t.Helper()
	id, err := h.users.UpsertUser(h.t.Context(), identity.ExternalIdentity{Issuer: "https://idp.test", Subject: rand.Text()})
	if err != nil {
		h.t.Fatalf("UpsertUser() error = %v", err)
	}
	if displayName != "" {
		if err := h.users.SetDisplayName(h.t.Context(), id, displayName); err != nil {
			h.t.Fatalf("SetDisplayName() error = %v", err)
		}
	}
	return h.clients(id)
}

// anonymous returns clients with no session.
func (h *harness) anonymous() *user { return h.clients("") }

func (h *harness) clients(userID string) *user {
	var opts []connect.ClientOption
	if userID != "" {
		opts = append(opts, connect.WithInterceptors(connect.UnaryInterceptorFunc(
			func(next connect.UnaryFunc) connect.UnaryFunc {
				return func(ctx context.Context, req connect.AnyRequest) (connect.AnyResponse, error) {
					req.Header().Set(testUserHeader, userID)
					return next(ctx, req)
				}
			})))
	}
	c, url := h.server.Client(), h.server.URL
	return &user{
		id:        userID,
		campaigns: campaignsv1connect.NewCampaignServiceClient(c, url, opts...),
		api:       charactersv1connect.NewCharacterServiceClient(c, url, opts...),
		inventory: charactersv1connect.NewInventoryServiceClient(c, url, opts...),
		content:   rulesv1connect.NewContentServiceClient(c, url, opts...),
		table:     rulesv1connect.NewTableContentServiceClient(c, url, opts...),
	}
}

// newCampaign creates a campaign whose master is master, lets every player
// in through one invite, and returns the campaign's ID.
func (h *harness) newCampaign(master *user, name string, players ...*user) string {
	h.t.Helper()
	ctx := h.t.Context()
	res, err := master.campaigns.CreateCampaign(ctx, connect.NewRequest(&campaignsv1.CreateCampaignRequest{
		Name: name, XpMode: campaignsv1.XpMode_XP_MODE_MILESTONES,
	}))
	if err != nil {
		h.t.Fatalf("CreateCampaign() error = %v", err)
	}
	id := res.Msg.GetCampaign().GetId()
	h.join(master, id, players...)
	return id
}

// join lets players into the campaign through an invite from its master.
func (h *harness) join(master *user, campaignID string, players ...*user) {
	h.t.Helper()
	if len(players) == 0 {
		return
	}
	ctx := h.t.Context()
	inv, err := master.campaigns.CreateInvite(ctx, connect.NewRequest(&campaignsv1.CreateInviteRequest{
		CampaignId: campaignID, MaxUses: campaigns.MaxInviteUses,
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

// memberStatus reads a membership's status directly: "active", "pending",
// or "" when there is none.
func (h *harness) memberStatus(campaignID, userID string) string {
	h.t.Helper()
	var status string
	err := h.pool.QueryRow(h.t.Context(), "SELECT status FROM campaign_members WHERE campaign_id = $1 AND user_id = $2", campaignID, userID).Scan(&status)
	if errors.Is(err, pgx.ErrNoRows) {
		return ""
	}
	if err != nil {
		h.t.Fatalf("read member status: %v", err)
	}
	return status
}

// approve calls ApproveCharacter as u, or fails the test.
func (u *user) approve(t *testing.T, c *charactersv1.Character) *charactersv1.Character {
	t.Helper()
	res, err := u.api.ApproveCharacter(t.Context(), connect.NewRequest(&charactersv1.ApproveCharacterRequest{
		CampaignId: c.GetCampaignId(), CharacterId: c.GetId(),
	}))
	if err != nil {
		t.Fatalf("ApproveCharacter() error = %v", err)
	}
	return res.Msg.GetCharacter()
}

// reject calls RejectCharacter as u.
func (u *user) reject(t *testing.T, c *charactersv1.Character) error {
	t.Helper()
	_, err := u.api.RejectCharacter(t.Context(), connect.NewRequest(&charactersv1.RejectCharacterRequest{
		CampaignId: c.GetCampaignId(), CharacterId: c.GetId(),
	}))
	return err
}

// lockSheets does what starting a game session does to the campaign's
// sheets (package play calls the same method, in its own transaction).
func (h *harness) lockSheets(campaignID string) int64 {
	h.t.Helper()
	var n int64
	err := db.InTx(h.t.Context(), h.pool, func(tx pgx.Tx) error {
		var err error
		n, err = h.svc.LockSheets(h.t.Context(), tx, campaignID, h.clock.Now())
		return err
	})
	if err != nil {
		h.t.Fatalf("LockSheets() error = %v", err)
	}
	return n
}

// deleteUser deletes an account straight in the database, as account
// deletion will (docs/privacy.md).
func (h *harness) deleteUser(id string) {
	h.t.Helper()
	if _, err := h.pool.Exec(h.t.Context(), "DELETE FROM users WHERE id = $1", id); err != nil {
		h.t.Fatalf("delete user: %v", err)
	}
}

// create calls CreateCharacter as u, or fails the test.
func (u *user) create(t *testing.T, campaignID string, kind charactersv1.CharacterKind, name string, sheet *charactersv1.CharacterSheet) *charactersv1.Character {
	t.Helper()
	res, err := u.api.CreateCharacter(t.Context(), connect.NewRequest(&charactersv1.CreateCharacterRequest{
		CampaignId: campaignID, Kind: kind, Name: name, Sheet: sheet,
	}))
	if err != nil {
		t.Fatalf("CreateCharacter(%v %q) error = %v", kind, name, err)
	}
	return res.Msg.GetCharacter()
}

// createPensantus creates Pensantus as u's player character.
func (u *user) createPensantus(t *testing.T, campaignID string) *charactersv1.Character {
	t.Helper()
	return u.create(t, campaignID, charactersv1.CharacterKind_CHARACTER_KIND_PLAYER, "Pensantus", pensantusSheet())
}

// get calls GetCharacter as u, or fails the test.
func (u *user) get(t *testing.T, campaignID, characterID string) *charactersv1.Character {
	t.Helper()
	res, err := u.api.GetCharacter(t.Context(), connect.NewRequest(&charactersv1.GetCharacterRequest{CampaignId: campaignID, CharacterId: characterID}))
	if err != nil {
		t.Fatalf("GetCharacter() error = %v", err)
	}
	return res.Msg.GetCharacter()
}

// list calls ListCharacters as u, or fails the test.
func (u *user) list(t *testing.T, campaignID string) []*charactersv1.CharacterSummary {
	t.Helper()
	res, err := u.api.ListCharacters(t.Context(), connect.NewRequest(&charactersv1.ListCharactersRequest{CampaignId: campaignID}))
	if err != nil {
		t.Fatalf("ListCharacters() error = %v", err)
	}
	return res.Msg.GetCharacters()
}

// update calls UpdateCharacter as u with the character's current revision.
func (u *user) update(t *testing.T, c *charactersv1.Character, name string, sheet *charactersv1.CharacterSheet) (*charactersv1.Character, error) {
	t.Helper()
	res, err := u.api.UpdateCharacter(t.Context(), connect.NewRequest(&charactersv1.UpdateCharacterRequest{
		CampaignId: c.GetCampaignId(), CharacterId: c.GetId(), Revision: c.GetRevision(), Name: name, Sheet: sheet,
	}))
	if err != nil {
		return nil, err
	}
	return res.Msg.GetCharacter(), nil
}

// updateStory calls UpdateCharacterStory as u with the character's current
// revision.
func (u *user) updateStory(t *testing.T, c *charactersv1.Character, story *charactersv1.CharacterStory) (*charactersv1.Character, error) {
	t.Helper()
	res, err := u.api.UpdateCharacterStory(t.Context(), connect.NewRequest(&charactersv1.UpdateCharacterStoryRequest{
		CampaignId: c.GetCampaignId(), CharacterId: c.GetId(), Revision: c.GetRevision(), Story: story,
	}))
	if err != nil {
		return nil, err
	}
	return res.Msg.GetCharacter(), nil
}

// setStoryEditing calls SetStoryEditing as u, or fails the test.
func (u *user) setStoryEditing(t *testing.T, c *charactersv1.Character, allowed bool) *charactersv1.Character {
	t.Helper()
	res, err := u.api.SetStoryEditing(t.Context(), connect.NewRequest(&charactersv1.SetStoryEditingRequest{
		CampaignId: c.GetCampaignId(), CharacterId: c.GetId(), Allowed: allowed,
	}))
	if err != nil {
		t.Fatalf("SetStoryEditing(%v) error = %v", allowed, err)
	}
	return res.Msg.GetCharacter()
}

// markDead calls MarkCharacterDead as u, or fails the test.
func (u *user) markDead(t *testing.T, c *charactersv1.Character) *charactersv1.Character {
	t.Helper()
	res, err := u.api.MarkCharacterDead(t.Context(), connect.NewRequest(&charactersv1.MarkCharacterDeadRequest{
		CampaignId: c.GetCampaignId(), CharacterId: c.GetId(),
	}))
	if err != nil {
		t.Fatalf("MarkCharacterDead() error = %v", err)
	}
	return res.Msg.GetCharacter()
}

// wantCode fails the test unless err has the Connect code want.
func wantCode(t *testing.T, call string, err error, want connect.Code) {
	t.Helper()
	if got := connect.CodeOf(err); err == nil || got != want {
		t.Fatalf("%s error = %v, want %v", call, err, want)
	}
}

// blocked returns the CharacterBlocked detail of a failed_precondition.
func blocked(t *testing.T, call string, err error) *charactersv1.CharacterBlocked {
	t.Helper()
	wantCode(t, call, err, connect.CodeFailedPrecondition)
	ce, _ := errors.AsType[*connect.Error](err)
	for _, d := range ce.Details() {
		v, err := d.Value()
		if err != nil {
			t.Fatalf("decode error detail: %v", err)
		}
		if detail, ok := v.(*charactersv1.CharacterBlocked); ok {
			return detail
		}
	}
	t.Fatalf("%s error %v has no CharacterBlocked detail", call, err)
	return nil
}

// pensantusSheet is the reference character of ADR-0008: Rock Gnome, Wizard
// 3 (Evocation), with the table's Sage as a custom background.
func pensantusSheet() *charactersv1.CharacterSheet {
	return &charactersv1.CharacterSheet{Content: &charactersv1.CharacterSheet_Full{Full: &charactersv1.FullSheet{
		BaseScores: &rulesv1.AbilityScores{Strength: 12, Dexterity: 16, Constitution: 15, Intelligence: 16, Wisdom: 13, Charisma: 12},
		RaceKey:    "race:gnome",
		SubraceKey: "subrace:rock-gnome",
		Classes: []*charactersv1.ClassLevel{{
			ClassKey: "class:wizard", Level: 3,
			Subclass: &charactersv1.ClassLevel_SubclassKey{SubclassKey: "subclass:evocation"},
		}},
		Background: &charactersv1.FullSheet_CustomBackground{CustomBackground: &charactersv1.CustomBackground{
			Name: "Sábio", SkillKeys: []string{"skill:arcana", "skill:history"},
			// SRD 5.1 "Customizing a Background": two languages, the feature, the equipment.
			ProficiencyKeys: []string{"language:draconic", "language:elvish"},
			FeatureName:     "Pesquisador", FeatureText: "Quando você não sabe uma informação, sabe a quem perguntar.",
			Equipment: "Um tinteiro, uma pena e roupas comuns.",
		}},
		SkillProficiencyKeys: []string{"skill:investigation", "skill:insight"},
		WeaponKeys:           []string{"equipment:quarterstaff"},
		CantripKeys:          []string{"spell:fire-bolt", "spell:ray-of-frost", "spell:minor-illusion"},
		KnownSpellKeys: []string{
			"spell:magic-missile", "spell:burning-hands", "spell:shield", "spell:mage-armor", "spell:sleep",
			"spell:find-familiar", "spell:detect-magic", "spell:comprehend-languages", "spell:scorching-ray", "spell:web",
		},
		PreparedSpellKeys: []string{
			"spell:magic-missile", "spell:burning-hands", "spell:shield", "spell:mage-armor", "spell:sleep",
			"spell:scorching-ray", "spell:web",
		},
		Equipment:        []*charactersv1.Item{{Name: "Grimório", Quantity: 1}, {Name: "Corda de cânhamo"}},
		Coins:            &charactersv1.Coins{Gold: 15, Silver: 3},
		Languages:        []string{"Dracônico"},
		ExperiencePoints: 900,
		Alignment:        charactersv1.Alignment_ALIGNMENT_NEUTRAL_GOOD,
	}}}
}

// enemySheet is a full sheet for an NPC: a human fighter.
func enemySheet() *charactersv1.CharacterSheet {
	return &charactersv1.CharacterSheet{Content: &charactersv1.CharacterSheet_Full{Full: &charactersv1.FullSheet{
		BaseScores: &rulesv1.AbilityScores{Strength: 16, Dexterity: 12, Constitution: 14, Intelligence: 8, Wisdom: 10, Charisma: 10},
		RaceKey:    "race:human",
		Classes:    []*charactersv1.ClassLevel{{ClassKey: "class:fighter", Level: 2}},
		ArmorKey:   "equipment:chain-mail",
		Shield:     true,
		WeaponKeys: []string{"equipment:longsword"},
	}}}
}

// basicSheet is a basic sheet, for a minion or a story NPC.
func basicSheet() *charactersv1.CharacterSheet {
	return &charactersv1.CharacterSheet{Content: &charactersv1.CharacterSheet_Basic{Basic: &charactersv1.BasicSheet{
		HitPointsMax: 7, ArmorClass: 15, SpeedFt: 30, InitiativeBonus: 2,
		Attacks: []*charactersv1.BasicAttack{{
			Name: "Cimitarra", AttackBonus: 4, DamageDiceCount: 1, DamageDiceSides: 6, DamageBonus: 2,
			DamageType: charactersv1.DamageType_DAMAGE_TYPE_SLASHING,
		}},
		Description: "Um goblin nervoso.",
	}}}
}
