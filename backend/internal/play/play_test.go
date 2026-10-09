package play

import (
	"context"
	"errors"
	"log/slog"
	"maps"
	"net/http"
	"net/http/httptest"
	"net/url"
	"slices"
	"sync"
	"testing"
	"time"

	"connectrpc.com/connect"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/reflect/protoreflect"
	"google.golang.org/protobuf/types/descriptorpb"

	campaignsv1 "github.com/PuraFome/meuRPG/backend/gen/meurpg/campaigns/v1"
	charactersv1 "github.com/PuraFome/meuRPG/backend/gen/meurpg/characters/v1"
	playv1 "github.com/PuraFome/meuRPG/backend/gen/meurpg/play/v1"
	"github.com/PuraFome/meuRPG/backend/gen/meurpg/play/v1/playv1connect"
	rulesv1 "github.com/PuraFome/meuRPG/backend/gen/meurpg/rules/v1"
	"github.com/PuraFome/meuRPG/backend/internal/authz"
	maplink "github.com/PuraFome/meuRPG/backend/internal/maps/link"
	"github.com/PuraFome/meuRPG/backend/internal/platform/dbtest"
	"github.com/PuraFome/meuRPG/backend/internal/play/link"
	"github.com/PuraFome/meuRPG/backend/internal/rules"
	"github.com/PuraFome/meuRPG/backend/internal/rules/combat"
)

const allowed connect.Code = 0

// TestAuthorizationMatrix calls every PlayService method as each kind of
// caller (ADR-0011): only the master starts and ends sessions and corrects
// vitals; any member lists sessions and watches the live one; a non-member
// and a pending member (RN-15) get not_found; anonymous, unauthenticated.
func TestAuthorizationMatrix(t *testing.T) {
	t.Parallel()
	h := newHarness(t)
	h.svc.SetTraps(emptyTrapBook{}) // a map book with no traps (see the traps rows below)
	master, player, pending := h.newUser("Mestre"), h.newUser("Jogadora"), h.newUser("Pendente")
	campaign := h.newCampaign(master, "Mirathel", player)
	h.joinPending(master, campaign, pending)
	pc := player.createCharacter(t, campaign, charactersv1.CharacterKind_CHARACTER_KIND_PLAYER, "Pensantus")
	open := master.start(t, campaign).GetGameSession()

	type client = playv1connect.PlayServiceClient
	rows := []struct {
		name string
		call func(ctx context.Context, c client) error
		// master, player, non-member, pending member, anonymous
		want [5]connect.Code
	}{
		{"EndGameSession", func(ctx context.Context, c client) error {
			_, err := c.EndGameSession(ctx, connect.NewRequest(&playv1.EndGameSessionRequest{CampaignId: campaign, GameSessionId: open.GetId()}))
			return err
		}, [5]connect.Code{allowed, connect.CodePermissionDenied, connect.CodeNotFound, connect.CodeNotFound, connect.CodeUnauthenticated}},
		// After the row above, no session is open, so the master may start
		// one; the rows after it run during that session.
		{"StartGameSession", func(ctx context.Context, c client) error {
			_, err := c.StartGameSession(ctx, connect.NewRequest(&playv1.StartGameSessionRequest{CampaignId: campaign}))
			return err
		}, [5]connect.Code{allowed, connect.CodePermissionDenied, connect.CodeNotFound, connect.CodeNotFound, connect.CodeUnauthenticated}},
		{"ListGameSessions", func(ctx context.Context, c client) error {
			_, err := c.ListGameSessions(ctx, connect.NewRequest(&playv1.ListGameSessionsRequest{CampaignId: campaign}))
			return err
		}, [5]connect.Code{allowed, allowed, connect.CodeNotFound, connect.CodeNotFound, connect.CodeUnauthenticated}},
		// Anyone signed in; each sees only their own campaigns.
		{"ListOpenGameSessions", func(ctx context.Context, c client) error {
			_, err := c.ListOpenGameSessions(ctx, connect.NewRequest(&playv1.ListOpenGameSessionsRequest{}))
			return err
		}, [5]connect.Code{allowed, allowed, allowed, allowed, connect.CodeUnauthenticated}},
		{"GetLiveSession", func(ctx context.Context, c client) error {
			_, err := c.GetLiveSession(ctx, connect.NewRequest(&playv1.GetLiveSessionRequest{CampaignId: campaign}))
			return err
		}, [5]connect.Code{allowed, allowed, connect.CodeNotFound, connect.CodeNotFound, connect.CodeUnauthenticated}},
		{"WatchGameSession", func(ctx context.Context, c client) error {
			return firstEventError(ctx, c, campaign)
		}, [5]connect.Code{allowed, allowed, connect.CodeNotFound, connect.CodeNotFound, connect.CodeUnauthenticated}},
		{"AdjustCharacterVitals", func(ctx context.Context, c client) error {
			_, err := c.AdjustCharacterVitals(ctx, connect.NewRequest(&playv1.AdjustCharacterVitalsRequest{
				CampaignId: campaign, CharacterId: pc.GetId(), IdempotencyKey: newKey(), HitPointsTemporary: proto.Int32(1),
			}))
			return err
		}, [5]connect.Code{allowed, connect.CodePermissionDenied, connect.CodeNotFound, connect.CodeNotFound, connect.CodeUnauthenticated}},
		// Clearing needs no map nor image; setting them is in package maps'
		// tests, which have maps and images.
		{"SetCurrentMap", func(ctx context.Context, c client) error {
			_, err := c.SetCurrentMap(ctx, connect.NewRequest(&playv1.SetCurrentMapRequest{CampaignId: campaign}))
			return err
		}, [5]connect.Code{allowed, connect.CodePermissionDenied, connect.CodeNotFound, connect.CodeNotFound, connect.CodeUnauthenticated}},
		{"SetShownImage", func(ctx context.Context, c client) error {
			_, err := c.SetShownImage(ctx, connect.NewRequest(&playv1.SetShownImageRequest{CampaignId: campaign}))
			return err
		}, [5]connect.Code{allowed, connect.CodePermissionDenied, connect.CodeNotFound, connect.CodeNotFound, connect.CodeUnauthenticated}},
		// Any member, in or out of a session.
		{"ListLeftImages", func(ctx context.Context, c client) error {
			_, err := c.ListLeftImages(ctx, connect.NewRequest(&playv1.ListLeftImagesRequest{CampaignId: campaign}))
			return err
		}, [5]connect.Code{allowed, allowed, connect.CodeNotFound, connect.CodeNotFound, connect.CodeUnauthenticated}},
		// Nothing is left, so the master's call is not_found, like the
		// non-member's; the real one is in package maps' tests.
		{"TakeBackLeftImage", func(ctx context.Context, c client) error {
			_, err := c.TakeBackLeftImage(ctx, connect.NewRequest(&playv1.TakeBackLeftImageRequest{CampaignId: campaign, ImageId: campaign}))
			return err
		}, [5]connect.Code{connect.CodeNotFound, connect.CodePermissionDenied, connect.CodeNotFound, connect.CodeNotFound, connect.CodeUnauthenticated}},
		// The scenes (MR-015). This harness has no maps, so no scene point: the
		// master's OpenScene is not_found and the player's roll fails with
		// NO_OPEN_SCENE; the full matrix, with a real scene, is in package maps'
		// tests (TestSceneAuthorizationMatrix).
		{"OpenScene", func(ctx context.Context, c client) error {
			_, err := c.OpenScene(ctx, connect.NewRequest(&playv1.OpenSceneRequest{CampaignId: campaign, PointId: campaign}))
			return err
		}, [5]connect.Code{connect.CodeNotFound, connect.CodePermissionDenied, connect.CodeNotFound, connect.CodeNotFound, connect.CodeUnauthenticated}},
		{"CloseScene", func(ctx context.Context, c client) error {
			_, err := c.CloseScene(ctx, connect.NewRequest(&playv1.CloseSceneRequest{CampaignId: campaign}))
			return err
		}, [5]connect.Code{allowed, connect.CodePermissionDenied, connect.CodeNotFound, connect.CodeNotFound, connect.CodeUnauthenticated}},
		{"GetOpenScene", func(ctx context.Context, c client) error {
			_, err := c.GetOpenScene(ctx, connect.NewRequest(&playv1.GetOpenSceneRequest{CampaignId: campaign}))
			return err
		}, [5]connect.Code{allowed, allowed, connect.CodeNotFound, connect.CodeNotFound, connect.CodeUnauthenticated}},
		{"RollSceneCheck", func(ctx context.Context, c client) error {
			_, err := c.RollSceneCheck(ctx, connect.NewRequest(&playv1.RollSceneCheckRequest{
				CampaignId: campaign, ActionId: campaign, IdempotencyKey: newKey(), Roll: &playv1.RollSceneCheckRequest_RollInApp{RollInApp: true},
			}))
			return err
		}, [5]connect.Code{connect.CodePermissionDenied, connect.CodeFailedPrecondition, connect.CodeNotFound, connect.CodeNotFound, connect.CodeUnauthenticated}},
		// One more attempt (MR-015, question 55). No scene is open here, so the
		// master's grant is refused by the state (NO_OPEN_SCENE); the full matrix,
		// with a scene, is in package maps' tests (TestSceneAuthorizationMatrix).
		{"GrantSceneAttempt", func(ctx context.Context, c client) error {
			_, err := c.GrantSceneAttempt(ctx, connect.NewRequest(&playv1.GrantSceneAttemptRequest{
				CampaignId: campaign, ActionId: campaign, CharacterId: pc.GetId(), IdempotencyKey: newKey(),
			}))
			return err
		}, [5]connect.Code{connect.CodeFailedPrecondition, connect.CodePermissionDenied, connect.CodeNotFound, connect.CodeNotFound, connect.CodeUnauthenticated}},
		// The session summary (MR-032): any member, for an ended session. The
		// first row ended `open`, so it is ended for every caller after it.
		{"GetSessionSummary", func(ctx context.Context, c client) error {
			_, err := c.GetSessionSummary(ctx, connect.NewRequest(&playv1.GetSessionSummaryRequest{CampaignId: campaign, GameSessionId: open.GetId()}))
			return err
		}, [5]connect.Code{allowed, allowed, connect.CodeNotFound, connect.CodeNotFound, connect.CodeUnauthenticated}},
		// The stage (MR-031). No scene is open here, so the master's PutOnStage
		// is refused by the state (NO_OPEN_SCENE); the full matrix, with a scene
		// and NPCs, is in package maps' tests (TestStageAuthorizationMatrix).
		{"PutOnStage", func(ctx context.Context, c client) error {
			_, err := c.PutOnStage(ctx, connect.NewRequest(&playv1.PutOnStageRequest{CampaignId: campaign, CharacterId: pc.GetId()}))
			return err
		}, [5]connect.Code{connect.CodeFailedPrecondition, connect.CodePermissionDenied, connect.CodeNotFound, connect.CodeNotFound, connect.CodeUnauthenticated}},
		{"TakeOffStage", func(ctx context.Context, c client) error {
			_, err := c.TakeOffStage(ctx, connect.NewRequest(&playv1.TakeOffStageRequest{CampaignId: campaign, CharacterId: pc.GetId()}))
			return err
		}, [5]connect.Code{allowed, connect.CodePermissionDenied, connect.CodeNotFound, connect.CodeNotFound, connect.CodeUnauthenticated}},
		// Casting a summoning spell outside a combat (MR-037). The character is a
		// level 1 wizard with no Convocar Familiar, so the rules refuse after the
		// authorization has passed (invalid_argument); a player may cast for their
		// own character only, which TestMR037 covers with another player.
		{"CastSummon", func(ctx context.Context, c client) error {
			_, err := c.CastSummon(ctx, connect.NewRequest(&playv1.CastSummonRequest{
				CampaignId: campaign, CharacterId: pc.GetId(), SpellKey: "spell:find-familiar", Ritual: true, IdempotencyKey: newKey(),
				Summon: &playv1.SummonChoice{CreatureKeys: []string{"monster:owl"}},
			}))
			return err
		}, [5]connect.Code{connect.CodeInvalidArgument, connect.CodeInvalidArgument, connect.CodeNotFound, connect.CodeNotFound, connect.CodeUnauthenticated}},
		// Wild Shape and the familiar's eyes (MR-037, MR-036). The character is a level 1
		// wizard with no familiar and no Wild Shape, so the state refuses after the
		// authorization has passed (failed_precondition, with the typed reason); the
		// player may do it for their own character, and another player's is
		// permission_denied (TestMR037_WildShapeIsTheOwnersAndTheMasters).
		{"AssumeWildShape", func(ctx context.Context, c client) error {
			_, err := c.AssumeWildShape(ctx, connect.NewRequest(&playv1.AssumeWildShapeRequest{CampaignId: campaign, CharacterId: pc.GetId(), BeastKey: "monster:wolf", IdempotencyKey: newKey()}))
			return err
		}, [5]connect.Code{connect.CodeFailedPrecondition, connect.CodeFailedPrecondition, connect.CodeNotFound, connect.CodeNotFound, connect.CodeUnauthenticated}},
		{"LeaveWildShape", func(ctx context.Context, c client) error {
			_, err := c.LeaveWildShape(ctx, connect.NewRequest(&playv1.LeaveWildShapeRequest{CampaignId: campaign, CharacterId: pc.GetId(), IdempotencyKey: newKey()}))
			return err
		}, [5]connect.Code{connect.CodeFailedPrecondition, connect.CodeFailedPrecondition, connect.CodeNotFound, connect.CodeNotFound, connect.CodeUnauthenticated}},
		{"StartFamiliarSight", func(ctx context.Context, c client) error {
			_, err := c.StartFamiliarSight(ctx, connect.NewRequest(&playv1.StartFamiliarSightRequest{CampaignId: campaign, CharacterId: pc.GetId(), IdempotencyKey: newKey()}))
			return err
		}, [5]connect.Code{connect.CodeFailedPrecondition, connect.CodeFailedPrecondition, connect.CodeNotFound, connect.CodeNotFound, connect.CodeUnauthenticated}},
		{"StopFamiliarSight", func(ctx context.Context, c client) error {
			_, err := c.StopFamiliarSight(ctx, connect.NewRequest(&playv1.StopFamiliarSightRequest{CampaignId: campaign, CharacterId: pc.GetId(), IdempotencyKey: newKey()}))
			return err
		}, [5]connect.Code{connect.CodeFailedPrecondition, connect.CodeFailedPrecondition, connect.CodeNotFound, connect.CodeNotFound, connect.CodeUnauthenticated}},
		{"SetSpeaker", func(ctx context.Context, c client) error {
			_, err := c.SetSpeaker(ctx, connect.NewRequest(&playv1.SetSpeakerRequest{CampaignId: campaign}))
			return err
		}, [5]connect.Code{allowed, connect.CodePermissionDenied, connect.CodeNotFound, connect.CodeNotFound, connect.CodeUnauthenticated}},
		// The traps in play (MR-035, ADR-0011). This harness has a map book with no
		// traps and no map on screen: a player's search is refused by the state
		// (TRAP_NOT_ON_MAP), the master's is the player's alone; firing a trap that is
		// not there is not_found; the master reads and settles the damage, a player
		// never does. The full matrix with real traps is in TestMR035_*.
		{"SearchForTraps", func(ctx context.Context, c client) error {
			_, err := c.SearchForTraps(ctx, connect.NewRequest(&playv1.SearchForTrapsRequest{
				CampaignId: campaign, IdempotencyKey: newKey(), Skill: playv1.TrapSearchSkill_TRAP_SEARCH_SKILL_PERCEPTION,
				Roll: &playv1.SearchForTrapsRequest_RollInApp{RollInApp: true},
			}))
			return err
		}, [5]connect.Code{connect.CodePermissionDenied, connect.CodeFailedPrecondition, connect.CodeNotFound, connect.CodeNotFound, connect.CodeUnauthenticated}},
		{"ListTrapActivity", func(ctx context.Context, c client) error {
			_, err := c.ListTrapActivity(ctx, connect.NewRequest(&playv1.ListTrapActivityRequest{CampaignId: campaign}))
			return err
		}, [5]connect.Code{allowed, allowed, connect.CodeNotFound, connect.CodeNotFound, connect.CodeUnauthenticated}},
		{"FireTrap", func(ctx context.Context, c client) error {
			_, err := c.FireTrap(ctx, connect.NewRequest(&playv1.FireTrapRequest{CampaignId: campaign, MapId: campaign, PointId: campaign, IdempotencyKey: newKey()}))
			return err
		}, [5]connect.Code{connect.CodeNotFound, connect.CodePermissionDenied, connect.CodeNotFound, connect.CodeNotFound, connect.CodeUnauthenticated}},
		{"ListTrapDamages", func(ctx context.Context, c client) error {
			_, err := c.ListTrapDamages(ctx, connect.NewRequest(&playv1.ListTrapDamagesRequest{CampaignId: campaign}))
			return err
		}, [5]connect.Code{allowed, connect.CodePermissionDenied, connect.CodeNotFound, connect.CodeNotFound, connect.CodeUnauthenticated}},
		{"ApplyTrapDamage", func(ctx context.Context, c client) error {
			_, err := c.ApplyTrapDamage(ctx, connect.NewRequest(&playv1.ApplyTrapDamageRequest{CampaignId: campaign, TrapDamageId: campaign, IdempotencyKey: newKey()}))
			return err
		}, [5]connect.Code{connect.CodeNotFound, connect.CodePermissionDenied, connect.CodeNotFound, connect.CodeNotFound, connect.CodeUnauthenticated}},
		{"DiscardTrapDamage", func(ctx context.Context, c client) error {
			_, err := c.DiscardTrapDamage(ctx, connect.NewRequest(&playv1.DiscardTrapDamageRequest{CampaignId: campaign, TrapDamageId: campaign, IdempotencyKey: newKey()}))
			return err
		}, [5]connect.Code{connect.CodeNotFound, connect.CodePermissionDenied, connect.CodeNotFound, connect.CodeNotFound, connect.CodeUnauthenticated}},
	}

	covered := map[string]bool{}
	for _, r := range rows {
		covered[r.name] = true
	}
	methods := playv1.File_meurpg_play_v1_play_proto.Services().ByName("PlayService").Methods()
	for i := range methods.Len() {
		if name := string(methods.Get(i).Name()); !covered[name] {
			t.Errorf("PlayService.%s is missing from the authorization matrix", name)
		}
	}

	callers := []struct {
		name   string
		client client
	}{
		{"master", master.play},
		{"player", player.play},
		{"non-member", h.newUser("De fora").play},
		{"pending member", pending.play},
		{"anonymous", h.anonymous().play},
	}
	for _, r := range rows {
		for i, caller := range callers {
			t.Run(r.name+"/"+caller.name, func(t *testing.T) {
				err := r.call(t.Context(), caller.client)
				switch want := r.want[i]; {
				case want == allowed && err != nil:
					t.Errorf("error = %v, want allowed", err)
				case want != allowed && connect.CodeOf(err) != want:
					t.Errorf("error = %v, want %v", err, want)
				}
			})
		}
	}
}

// RN-01 (and MR-011's third criterion): when the first session of a
// campaign starts, its players' sheets lock, in the same transaction. Only
// living player characters of that campaign lock: NPCs never do, a dead
// character keeps its state, and other campaigns are untouched.
func TestRN01_StartingTheFirstSessionLocksPlayerSheetsOnly(t *testing.T) {
	t.Parallel()
	h := newHarness(t)
	master, player, fallen := h.newUser("Mestre"), h.newUser("Jogadora"), h.newUser("Caída")
	campaign := h.newCampaign(master, "Mirathel", player, fallen)
	other := h.newCampaign(master, "Outra", player)

	pc := player.createCharacter(t, campaign, charactersv1.CharacterKind_CHARACTER_KIND_PLAYER, "Pensantus")
	dead := fallen.createCharacter(t, campaign, charactersv1.CharacterKind_CHARACTER_KIND_PLAYER, "Morta")
	if _, err := master.characters.MarkCharacterDead(t.Context(), connect.NewRequest(&charactersv1.MarkCharacterDeadRequest{CampaignId: campaign, CharacterId: dead.GetId()})); err != nil {
		t.Fatalf("MarkCharacterDead() error = %v", err)
	}
	npc := master.createCharacter(t, campaign, charactersv1.CharacterKind_CHARACTER_KIND_ENEMY, "Orc")
	minion := master.createCharacter(t, campaign, charactersv1.CharacterKind_CHARACTER_KIND_MINION, "Goblin")
	elsewhere := player.createCharacter(t, other, charactersv1.CharacterKind_CHARACTER_KIND_PLAYER, "Pensantus")

	res := master.start(t, campaign)
	s := res.GetGameSession()
	if s.GetSessionNumber() != 1 || s.GetCampaignId() != campaign || s.GetStartedAt() == nil || s.GetEndedAt() != nil {
		t.Errorf("StartGameSession() = %v, want open session 1", s)
	}
	if res.GetLockedSheetCount() != 1 {
		t.Errorf("locked_sheet_count = %d, want 1 (only the living player character)", res.GetLockedSheetCount())
	}

	got := master.character(t, pc)
	if got.GetState() != charactersv1.CharacterState_CHARACTER_STATE_LOCKED || !got.GetSheetLockedAt().AsTime().Equal(s.GetStartedAt().AsTime()) {
		t.Errorf("player character = %v at %v, want locked at the session's start %v", got.GetState(), got.GetSheetLockedAt(), s.GetStartedAt())
	}
	if got := master.character(t, dead); got.GetState() != charactersv1.CharacterState_CHARACTER_STATE_DEAD || got.GetSheetLockedAt() != nil {
		t.Errorf("dead character = %v, locked at %v; want dead, never locked", got.GetState(), got.GetSheetLockedAt())
	}
	for _, c := range []*charactersv1.Character{npc, minion} {
		if got := master.character(t, c); got.GetState() != charactersv1.CharacterState_CHARACTER_STATE_DRAFT || !got.GetCanEdit() {
			t.Errorf("NPC %s = %v, want a draft the master edits", c.GetName(), got.GetState())
		}
	}
	if got := player.character(t, elsewhere); got.GetState() != charactersv1.CharacterState_CHARACTER_STATE_DRAFT {
		t.Errorf("the player's character in another campaign = %v, want a draft", got.GetState())
	}
}

// TestOnlyOneOpenSessionPerCampaign: a second start while a session is
// open fails, even when the starts race; after the end, the next start gets
// the next number.
func TestOnlyOneOpenSessionPerCampaign(t *testing.T) {
	t.Parallel()
	dbtest.PoolSize(t, 8) // the racers must overlap: one connection would run them one by one
	h := newHarness(t)
	master := h.newUser("Mestre")
	campaign := h.newCampaign(master, "Mirathel")

	first := master.start(t, campaign).GetGameSession()
	_, err := master.play.StartGameSession(t.Context(), connect.NewRequest(&playv1.StartGameSessionRequest{CampaignId: campaign}))
	wantCode(t, "StartGameSession() with a session open", err, connect.CodeFailedPrecondition)
	master.end(t, first)

	var wg sync.WaitGroup
	results := make([]error, 5)
	start := dbtest.NewBarrier(len(results))
	for i := range results {
		wg.Go(func() {
			start.Wait()
			_, results[i] = master.play.StartGameSession(t.Context(), connect.NewRequest(&playv1.StartGameSessionRequest{CampaignId: campaign}))
		})
	}
	wg.Wait()
	started := 0
	for _, err := range results {
		switch connect.CodeOf(err) {
		case connect.CodeFailedPrecondition:
		default:
			if err != nil {
				t.Errorf("racing StartGameSession() error = %v, want ok or failed_precondition", err)
			}
			started++
		}
	}
	if started != 1 {
		t.Errorf("%d of 5 racing starts succeeded, want exactly 1", started)
	}

	res, err := master.play.ListGameSessions(t.Context(), connect.NewRequest(&playv1.ListGameSessionsRequest{CampaignId: campaign}))
	if err != nil {
		t.Fatalf("ListGameSessions() error = %v", err)
	}
	sessions := res.Msg.GetGameSessions()
	if len(sessions) != 2 || sessions[0].GetSessionNumber() != 2 || sessions[0].GetEndedAt() != nil ||
		sessions[1].GetId() != first.GetId() || sessions[1].GetEndedAt() == nil {
		t.Errorf("ListGameSessions() = %v, want session 2 (open) then session 1 (ended)", sessions)
	}
}

// RN-01 and MR-006's third criterion: a character created after the first
// session (a replacement, or a late player's) stays a draft until the next
// session starts, and then locks.
func TestCharacterCreatedLaterLocksAtTheNextSession(t *testing.T) {
	t.Parallel()
	h := newHarness(t)
	master, player, late := h.newUser("Mestre"), h.newUser("Jogadora"), h.newUser("Atrasada")
	campaign := h.newCampaign(master, "Mirathel", player)
	player.createCharacter(t, campaign, charactersv1.CharacterKind_CHARACTER_KIND_PLAYER, "Pensantus")
	master.end(t, master.start(t, campaign).GetGameSession())

	h.join(master, campaign, late)
	c := late.createCharacter(t, campaign, charactersv1.CharacterKind_CHARACTER_KIND_PLAYER, "Nova")
	if got := late.character(t, c); got.GetState() != charactersv1.CharacterState_CHARACTER_STATE_DRAFT || !got.GetCanEdit() {
		t.Fatalf("a character created after the first session = %v, want an editable draft", got.GetState())
	}

	res := master.start(t, campaign)
	if res.GetLockedSheetCount() != 1 || res.GetGameSession().GetSessionNumber() != 2 {
		t.Errorf("second StartGameSession() = %v, want session 2 locking 1 sheet", res)
	}
	if got := late.character(t, c); got.GetState() != charactersv1.CharacterState_CHARACTER_STATE_LOCKED || got.GetCanEdit() {
		t.Errorf("after the next session the character = %v, want locked", got.GetState())
	}
}

// RN-01: starting a session ends every story permission the master gave
// (story_editing_allowed), in the same transaction as the lock.
func TestStartingASessionTurnsStoryEditingOff(t *testing.T) {
	t.Parallel()
	h := newHarness(t)
	master, player := h.newUser("Mestre"), h.newUser("Jogadora")
	campaign := h.newCampaign(master, "Mirathel", player)
	c := player.createCharacter(t, campaign, charactersv1.CharacterKind_CHARACTER_KIND_PLAYER, "Pensantus")
	master.end(t, master.start(t, campaign).GetGameSession())

	if _, err := master.characters.SetStoryEditing(t.Context(), connect.NewRequest(&charactersv1.SetStoryEditingRequest{
		CampaignId: campaign, CharacterId: c.GetId(), Allowed: true,
	})); err != nil {
		t.Fatalf("SetStoryEditing() error = %v", err)
	}
	if got := player.character(t, c); !got.GetStoryEditingAllowed() || !got.GetCanEditStory() {
		t.Fatalf("after SetStoryEditing(true) the player may not edit the story: %v", got)
	}

	master.start(t, campaign)
	if got := player.character(t, c); got.GetStoryEditingAllowed() || got.GetCanEditStory() {
		t.Errorf("after the next session story_editing_allowed = %v, can_edit_story = %v; want both false",
			got.GetStoryEditingAllowed(), got.GetCanEditStory())
	}
}

// TestEndGameSession: ending twice keeps the first ended_at; a session of
// another campaign, or an ID that is not one, is not found; ending never
// unlocks a sheet.
func TestEndGameSession(t *testing.T) {
	t.Parallel()
	h := newHarness(t)
	master, player := h.newUser("Mestre"), h.newUser("Jogadora")
	campaign := h.newCampaign(master, "Mirathel", player)
	other := h.newCampaign(master, "Outra")
	c := player.createCharacter(t, campaign, charactersv1.CharacterKind_CHARACTER_KIND_PLAYER, "Pensantus")
	s := master.start(t, campaign).GetGameSession()

	ended := master.end(t, s)
	again := master.end(t, s)
	if ended.GetEndedAt() == nil || !again.GetEndedAt().AsTime().Equal(ended.GetEndedAt().AsTime()) {
		t.Errorf("ending twice: %v then %v, want the first ended_at kept", ended.GetEndedAt(), again.GetEndedAt())
	}
	if got := player.character(t, c); got.GetState() != charactersv1.CharacterState_CHARACTER_STATE_LOCKED {
		t.Errorf("after the session ended the character = %v, want still locked", got.GetState())
	}
	for _, req := range []*playv1.EndGameSessionRequest{
		{CampaignId: other, GameSessionId: s.GetId()},
		{CampaignId: campaign, GameSessionId: "6f1c7a52-3b5e-4c55-9d0b-2a51f0c1e0ff"},
		{CampaignId: campaign, GameSessionId: "sessão 1"},
	} {
		_, err := master.play.EndGameSession(t.Context(), connect.NewRequest(req))
		wantCode(t, "EndGameSession("+req.GetGameSessionId()+")", err, connect.CodeNotFound)
	}
	if res, err := master.play.ListGameSessions(t.Context(), connect.NewRequest(&playv1.ListGameSessionsRequest{CampaignId: other})); err != nil || len(res.Msg.GetGameSessions()) != 0 {
		t.Errorf("ListGameSessions(other) = %v, %v; want none", res, err)
	}
}

// TestResponsesAreNotCached: game sessions describe the caller's campaign.
func TestResponsesAreNotCached(t *testing.T) {
	t.Parallel()
	h := newHarness(t)
	master := h.newUser("Mestre")
	campaign := h.newCampaign(master, "Mirathel")
	res, err := master.play.ListGameSessions(t.Context(), connect.NewRequest(&playv1.ListGameSessionsRequest{CampaignId: campaign}))
	if err != nil {
		t.Fatalf("ListGameSessions() error = %v", err)
	}
	if got := res.Header().Get("Cache-Control"); got != "no-store" {
		t.Errorf("Cache-Control = %q, want no-store", got)
	}
}

// Tests below need no database.

type noSheets struct{}

func (noSheets) LockSheets(context.Context, pgx.Tx, string, time.Time) (int64, error) {
	return 0, errors.New("not in this test")
}

type noVitals struct{}

func (noVitals) ListVitals(context.Context, string) ([]*playv1.CharacterVitals, error) {
	return nil, errors.New("not in this test")
}

func (noVitals) GetVitals(context.Context, string, string) (*playv1.CharacterVitals, error) {
	return nil, errors.New("not in this test")
}

func (noVitals) GetVitalsTx(context.Context, pgx.Tx, string, string) (*playv1.CharacterVitals, error) {
	return nil, errors.New("not in this test")
}

func (noVitals) AdjustVitals(context.Context, pgx.Tx, string, string, *playv1.AdjustCharacterVitalsRequest) (before, after *playv1.CharacterVitals, err error) {
	return nil, nil, errors.New("not in this test")
}

func (noVitals) AssumeWildShape(context.Context, pgx.Tx, string, string, string) (before, after *playv1.CharacterVitals, body link.Character, err error) {
	return nil, nil, link.Character{}, errors.New("not in this test")
}

func (noVitals) SetWildShape(context.Context, pgx.Tx, string, string, string, int32) (*playv1.CharacterVitals, link.Character, error) {
	return nil, link.Character{}, errors.New("not in this test")
}

func (noVitals) FamiliarOf(context.Context, pgx.Tx, string, string) (link.Creature, bool, error) {
	return link.Creature{}, false, errors.New("not in this test")
}

func (noVitals) SetFamiliarSight(context.Context, pgx.Tx, string, string, string, bool, []string) (*playv1.CharacterVitals, error) {
	return nil, errors.New("not in this test")
}

type noMaps struct{}

func (noMaps) RevealMap(context.Context, pgx.Tx, string, string, time.Time) error {
	return errors.New("not in this test")
}

func (noMaps) PrepareShow(context.Context, string, string) (*playv1.ShownImage, ShownCopy, error) {
	return nil, nil, errors.New("not in this test")
}

func (noMaps) MapShown(context.Context, string, string) {}

func (noMaps) VisionChanged(context.Context, string, string) {}

func (noMaps) ShownImage(context.Context, string, string) (*playv1.ShownImage, error) {
	return nil, errors.New("not in this test")
}

func (noMaps) LeaveImage(context.Context, pgx.Tx, string, string, time.Time) error {
	return errors.New("not in this test")
}

func (noMaps) ListLeftImages(context.Context, string) ([]*playv1.ShownImage, error) {
	return nil, errors.New("not in this test")
}

func (noMaps) TakeBackImage(context.Context, string, string) error {
	return errors.New("not in this test")
}

func (noMaps) MapGrid(context.Context, pgx.Tx, string, string) (link.Grid, error) {
	return link.Grid{}, errors.New("not in this test")
}

func (noMaps) BattlePoint(context.Context, string, string) (link.BattlePoint, error) {
	return link.BattlePoint{}, errors.New("not in this test")
}

func (noMaps) ScenePoint(context.Context, pgx.Tx, string, string) (link.Scene, error) {
	return link.Scene{}, errors.New("not in this test")
}

func (noMaps) DiscoverScene(context.Context, pgx.Tx, string, string, time.Time) error {
	return errors.New("not in this test")
}

func (noMaps) MapTokens(context.Context, pgx.Tx, string) ([]link.TokenPosition, error) {
	return nil, errors.New("not in this test")
}

func (noMaps) SetTokenPositions(context.Context, pgx.Tx, string, []link.TokenPosition, time.Time) error {
	return errors.New("not in this test")
}

type noRoster struct{}

func (noRoster) CombatParty(context.Context, pgx.Tx, string) ([]link.Character, error) {
	return nil, errors.New("not in this test")
}

func (noRoster) CombatCharacters(context.Context, pgx.Tx, string, []string) ([]link.Character, error) {
	return nil, errors.New("not in this test")
}

func (noRoster) SessionCharacters(context.Context, pgx.Tx, string, []string) ([]link.Character, error) {
	return nil, nil
}

func (noRoster) CombatSheet(context.Context, pgx.Tx, string, string) (link.Sheet, error) {
	return link.Sheet{}, errors.New("not in this test")
}

func (noRoster) SpendAmmunition(context.Context, pgx.Tx, string, string, string) error {
	return errors.New("not in this test")
}

func (noRoster) ItemForUse(context.Context, pgx.Tx, string, string, string) (link.UsableItem, error) {
	return link.UsableItem{}, errors.New("not in this test")
}

func (noRoster) ApplyItemUse(context.Context, pgx.Tx, string, string, link.ItemUseWrite) error {
	return errors.New("not in this test")
}

func (noRoster) CombatTurnOptions(context.Context, pgx.Tx, string, string, link.Turn) (*rulesv1.TurnOptions, error) {
	return nil, errors.New("not in this test")
}

func (noRoster) CombatSpell(context.Context, pgx.Tx, string, string, string, int, string) (link.Spell, error) {
	return link.Spell{}, errors.New("not in this test")
}

func (noRoster) SceneOptions(context.Context, pgx.Tx, string, string, []string) ([]link.SceneOption, error) {
	return nil, errors.New("not in this test")
}

func (noRoster) SceneCheckName(string) string { return "" }

func (noRoster) CombatSave(context.Context, pgx.Tx, string, string, string) (link.Save, error) {
	return link.Save{}, errors.New("not in this test")
}

func (noRoster) MarkDead(context.Context, pgx.Tx, string, string, time.Time) error {
	return errors.New("not in this test")
}

func (noRoster) Conditions() []link.Named { return nil }

func (noRoster) ContentNames(context.Context, pgx.Tx, string) (func(string) string, error) {
	return func(string) string { return "" }, nil
}

func (noRoster) CharacterCreatures(context.Context, pgx.Tx, string, []string) ([]link.Creature, error) {
	return nil, errors.New("not in this test")
}

func (noRoster) ConcentrationCreatures(context.Context, pgx.Tx, string, string) ([]link.Creature, error) {
	return nil, errors.New("not in this test")
}

func (noRoster) CheckSummon(context.Context, pgx.Tx, string, string, string, int, int, []string) (link.SummonSpell, error) {
	return link.SummonSpell{}, errors.New("not in this test")
}

func (noRoster) SummonCreatures(context.Context, pgx.Tx, link.Summon) (link.SummonResult, error) {
	return link.SummonResult{}, errors.New("not in this test")
}

func (noRoster) DismissCreatures(context.Context, pgx.Tx, string, []string, string, time.Time) ([]string, error) {
	return nil, errors.New("not in this test")
}

func (noRoster) ReviveCreatures(context.Context, pgx.Tx, string, []string, string) ([]string, error) {
	return nil, errors.New("not in this test")
}

func (noRoster) DeleteCreatures(context.Context, pgx.Tx, string, []string) error {
	return errors.New("not in this test")
}

func (noRoster) SyncCreatures(context.Context, pgx.Tx, string, []link.CreatureState, time.Time) (link.CreatureChanges, error) {
	return link.CreatureChanges{}, errors.New("not in this test")
}

func (noRoster) WriteBackCreatures(context.Context, pgx.Tx, string, []link.CreatureState, time.Time) error {
	return errors.New("not in this test")
}

func (noRoster) CreatureSheet(context.Context, pgx.Tx, string, string, string) (link.Sheet, bool, error) {
	return link.Sheet{}, false, nil
}

func (noRoster) CreatureTurnOptions(context.Context, pgx.Tx, string, string, string, link.Turn) (*rulesv1.TurnOptions, bool, error) {
	return nil, false, nil
}

func (noRoster) CreatureSave(context.Context, pgx.Tx, string, string, string) (link.Save, error) {
	return link.Save{}, nil
}

func (noRoster) DamageModifiers(context.Context, pgx.Tx, string, string, string) (combat.TypeModifiers, error) {
	return combat.TypeModifiers{}, nil
}

func (noRoster) CreatureEyes(context.Context, pgx.Tx, string, string) (maplink.Eyes, bool, error) {
	return maplink.Eyes{}, false, nil
}

func (noRoster) MonsterHitPoints(context.Context, pgx.Tx, string, string) (link.MonsterHitPoints, bool, error) {
	return link.MonsterHitPoints{}, false, nil
}

func (noRoster) MonsterNpc(context.Context, pgx.Tx, string, string, string, time.Time) (link.Character, bool, error) {
	return link.Character{}, false, nil
}

func (noRoster) PartyLevels(context.Context, pgx.Tx, string) ([]link.PartyMember, error) {
	return nil, errors.New("not in this test")
}

func (noRoster) RulesContent(context.Context, pgx.Tx, string) (*rules.Content, error) {
	return nil, errors.New("not in this test")
}

type noDice struct{}

func (noDice) ForcedDice(context.Context, pgx.Tx, string, string) (DiceForce, error) {
	return DiceChoice, errors.New("not in this test")
}

type noCampaigns struct{}

func (noCampaigns) ActiveCampaigns(context.Context, string) ([]*campaignsv1.Campaign, error) {
	return nil, errors.New("not in this test")
}

type noMembers struct{}

func (noMembers) CampaignMembership(context.Context, string, string) (authz.Role, authz.Status, error) {
	return "", "", authz.ErrNotMember
}

func lazyPool(t *testing.T) *pgxpool.Pool {
	t.Helper()
	pool, err := pgxpool.New(t.Context(), "postgresql://nobody@127.0.0.1:1/none")
	if err != nil {
		t.Fatalf("pgxpool.New() error = %v", err)
	}
	t.Cleanup(pool.Close)
	return pool
}

func TestNewValidatesItsConfig(t *testing.T) {
	t.Parallel()
	pool := lazyPool(t)
	for name, cfg := range map[string]Config{
		"Pool":      {Sheets: noSheets{}, Vitals: noVitals{}, Campaigns: noCampaigns{}, Maps: noMaps{}, Roster: noRoster{}, Dice: noDice{}},
		"Sheets":    {Pool: pool, Vitals: noVitals{}, Campaigns: noCampaigns{}, Maps: noMaps{}, Roster: noRoster{}, Dice: noDice{}},
		"Vitals":    {Pool: pool, Sheets: noSheets{}, Campaigns: noCampaigns{}, Maps: noMaps{}, Roster: noRoster{}, Dice: noDice{}},
		"Campaigns": {Pool: pool, Sheets: noSheets{}, Vitals: noVitals{}, Maps: noMaps{}, Roster: noRoster{}, Dice: noDice{}},
		"Maps":      {Pool: pool, Sheets: noSheets{}, Vitals: noVitals{}, Campaigns: noCampaigns{}, Roster: noRoster{}, Dice: noDice{}},
		"Roster":    {Pool: pool, Sheets: noSheets{}, Vitals: noVitals{}, Campaigns: noCampaigns{}, Maps: noMaps{}, Dice: noDice{}},
		"Dice":      {Pool: pool, Sheets: noSheets{}, Vitals: noVitals{}, Campaigns: noCampaigns{}, Maps: noMaps{}, Roster: noRoster{}},
	} {
		if _, err := New(cfg); err == nil {
			t.Errorf("New() without %s succeeded", name)
		}
	}
}

// TestEveryMethodNeedsASession calls every method signed out, and checks
// that the refusal is not cacheable. Reads are POST-only.
func TestEveryMethodNeedsASession(t *testing.T) {
	t.Parallel()
	svc, err := New(Config{Pool: lazyPool(t), Sheets: noSheets{}, Vitals: noVitals{}, Campaigns: noCampaigns{}, Maps: noMaps{}, Roster: noRoster{}, Dice: noDice{}, Logger: slog.New(slog.DiscardHandler)})
	if err != nil {
		t.Fatalf("New() error = %v", err)
	}
	mux := http.NewServeMux()
	svc.Mount(mux.Handle, testSessions, noMembers{}, connect.WithRequireConnectProtocolHeader())
	server := httptest.NewServer(mux)
	t.Cleanup(server.Close)
	c := playv1connect.NewPlayServiceClient(server.Client(), server.URL)
	cc := playv1connect.NewCombatServiceClient(server.Client(), server.URL)
	ctx := t.Context()
	id := "6f1c7a52-3b5e-4c55-9d0b-2a51f0c1e001"

	calls := map[string]error{}
	_, calls["StartGameSession"] = c.StartGameSession(ctx, connect.NewRequest(&playv1.StartGameSessionRequest{CampaignId: id}))
	_, calls["EndGameSession"] = c.EndGameSession(ctx, connect.NewRequest(&playv1.EndGameSessionRequest{CampaignId: id, GameSessionId: id}))
	_, calls["ListGameSessions"] = c.ListGameSessions(ctx, connect.NewRequest(&playv1.ListGameSessionsRequest{CampaignId: id}))
	_, calls["ListOpenGameSessions"] = c.ListOpenGameSessions(ctx, connect.NewRequest(&playv1.ListOpenGameSessionsRequest{}))
	_, calls["GetLiveSession"] = c.GetLiveSession(ctx, connect.NewRequest(&playv1.GetLiveSessionRequest{CampaignId: id}))
	_, calls["AdjustCharacterVitals"] = c.AdjustCharacterVitals(ctx, connect.NewRequest(&playv1.AdjustCharacterVitalsRequest{CampaignId: id, CharacterId: id, IdempotencyKey: id}))
	_, calls["SetCurrentMap"] = c.SetCurrentMap(ctx, connect.NewRequest(&playv1.SetCurrentMapRequest{CampaignId: id, MapId: id}))
	_, calls["SetShownImage"] = c.SetShownImage(ctx, connect.NewRequest(&playv1.SetShownImageRequest{CampaignId: id, ImageId: id}))
	_, calls["ListLeftImages"] = c.ListLeftImages(ctx, connect.NewRequest(&playv1.ListLeftImagesRequest{CampaignId: id}))
	_, calls["TakeBackLeftImage"] = c.TakeBackLeftImage(ctx, connect.NewRequest(&playv1.TakeBackLeftImageRequest{CampaignId: id, ImageId: id}))
	_, calls["OpenScene"] = c.OpenScene(ctx, connect.NewRequest(&playv1.OpenSceneRequest{CampaignId: id, PointId: id}))
	_, calls["CloseScene"] = c.CloseScene(ctx, connect.NewRequest(&playv1.CloseSceneRequest{CampaignId: id}))
	_, calls["GetOpenScene"] = c.GetOpenScene(ctx, connect.NewRequest(&playv1.GetOpenSceneRequest{CampaignId: id}))
	_, calls["RollSceneCheck"] = c.RollSceneCheck(ctx, connect.NewRequest(&playv1.RollSceneCheckRequest{
		CampaignId: id, ActionId: id, IdempotencyKey: id, Roll: &playv1.RollSceneCheckRequest_RollInApp{RollInApp: true},
	}))
	_, calls["GrantSceneAttempt"] = c.GrantSceneAttempt(ctx, connect.NewRequest(&playv1.GrantSceneAttemptRequest{CampaignId: id, ActionId: id, CharacterId: id, IdempotencyKey: id}))
	_, calls["GetSessionSummary"] = c.GetSessionSummary(ctx, connect.NewRequest(&playv1.GetSessionSummaryRequest{CampaignId: id, GameSessionId: id}))
	_, calls["PutOnStage"] = c.PutOnStage(ctx, connect.NewRequest(&playv1.PutOnStageRequest{CampaignId: id, CharacterId: id}))
	_, calls["TakeOffStage"] = c.TakeOffStage(ctx, connect.NewRequest(&playv1.TakeOffStageRequest{CampaignId: id, CharacterId: id}))
	_, calls["SetSpeaker"] = c.SetSpeaker(ctx, connect.NewRequest(&playv1.SetSpeakerRequest{CampaignId: id}))
	_, calls["SearchForTraps"] = c.SearchForTraps(ctx, connect.NewRequest(&playv1.SearchForTrapsRequest{
		CampaignId: id, IdempotencyKey: id, Skill: playv1.TrapSearchSkill_TRAP_SEARCH_SKILL_PERCEPTION, Roll: &playv1.SearchForTrapsRequest_RollInApp{RollInApp: true},
	}))
	_, calls["ListTrapActivity"] = c.ListTrapActivity(ctx, connect.NewRequest(&playv1.ListTrapActivityRequest{CampaignId: id}))
	_, calls["FireTrap"] = c.FireTrap(ctx, connect.NewRequest(&playv1.FireTrapRequest{CampaignId: id, MapId: id, PointId: id, IdempotencyKey: id}))
	_, calls["ListTrapDamages"] = c.ListTrapDamages(ctx, connect.NewRequest(&playv1.ListTrapDamagesRequest{CampaignId: id}))
	_, calls["ApplyTrapDamage"] = c.ApplyTrapDamage(ctx, connect.NewRequest(&playv1.ApplyTrapDamageRequest{CampaignId: id, TrapDamageId: id, IdempotencyKey: id}))
	_, calls["DiscardTrapDamage"] = c.DiscardTrapDamage(ctx, connect.NewRequest(&playv1.DiscardTrapDamageRequest{CampaignId: id, TrapDamageId: id, IdempotencyKey: id}))
	_, calls["CastSummon"] = c.CastSummon(ctx, connect.NewRequest(&playv1.CastSummonRequest{CampaignId: id, CharacterId: id, IdempotencyKey: id}))
	_, calls["AssumeWildShape"] = c.AssumeWildShape(ctx, connect.NewRequest(&playv1.AssumeWildShapeRequest{CampaignId: id, CharacterId: id, BeastKey: "monster:wolf", IdempotencyKey: id}))
	_, calls["LeaveWildShape"] = c.LeaveWildShape(ctx, connect.NewRequest(&playv1.LeaveWildShapeRequest{CampaignId: id, CharacterId: id, IdempotencyKey: id}))
	_, calls["StartFamiliarSight"] = c.StartFamiliarSight(ctx, connect.NewRequest(&playv1.StartFamiliarSightRequest{CampaignId: id, CharacterId: id, IdempotencyKey: id}))
	_, calls["StopFamiliarSight"] = c.StopFamiliarSight(ctx, connect.NewRequest(&playv1.StopFamiliarSightRequest{CampaignId: id, CharacterId: id, IdempotencyKey: id}))
	calls["WatchGameSession"] = firstEventError(ctx, c, id)
	methods := playv1.File_meurpg_play_v1_play_proto.Services().ByName("PlayService").Methods()
	if len(calls) != methods.Len() {
		t.Errorf("called %d methods, the service has %d", len(calls), methods.Len())
	}

	// CombatService (MR-013) is mounted with the same checks.
	combat := map[string]error{}
	_, combat["StartEncounter"] = cc.StartEncounter(ctx, connect.NewRequest(&playv1.StartEncounterRequest{CampaignId: id}))
	_, combat["GetEncounter"] = cc.GetEncounter(ctx, connect.NewRequest(&playv1.GetEncounterRequest{CampaignId: id}))
	_, combat["SubmitInitiative"] = cc.SubmitInitiative(ctx, connect.NewRequest(&playv1.SubmitInitiativeRequest{CampaignId: id}))
	_, combat["SetInitiativeOrder"] = cc.SetInitiativeOrder(ctx, connect.NewRequest(&playv1.SetInitiativeOrderRequest{CampaignId: id}))
	_, combat["BeginCombat"] = cc.BeginCombat(ctx, connect.NewRequest(&playv1.BeginCombatRequest{CampaignId: id}))
	_, combat["EndTurn"] = cc.EndTurn(ctx, connect.NewRequest(&playv1.EndTurnRequest{CampaignId: id}))
	_, combat["MoveCombatant"] = cc.MoveCombatant(ctx, connect.NewRequest(&playv1.MoveCombatantRequest{CampaignId: id}))
	_, combat["GetMoveOptions"] = cc.GetMoveOptions(ctx, connect.NewRequest(&playv1.GetMoveOptionsRequest{CampaignId: id}))
	_, combat["SetCombatantSide"] = cc.SetCombatantSide(ctx, connect.NewRequest(&playv1.SetCombatantSideRequest{CampaignId: id}))
	_, combat["SetCombatantCover"] = cc.SetCombatantCover(ctx, connect.NewRequest(&playv1.SetCombatantCoverRequest{CampaignId: id}))
	_, combat["SetCombatantHidden"] = cc.SetCombatantHidden(ctx, connect.NewRequest(&playv1.SetCombatantHiddenRequest{CampaignId: id}))
	_, combat["AddCombatants"] = cc.AddCombatants(ctx, connect.NewRequest(&playv1.AddCombatantsRequest{CampaignId: id}))
	_, combat["AddMonsters"] = cc.AddMonsters(ctx, connect.NewRequest(&playv1.AddMonstersRequest{CampaignId: id}))
	_, combat["RemoveCombatant"] = cc.RemoveCombatant(ctx, connect.NewRequest(&playv1.RemoveCombatantRequest{CampaignId: id}))
	_, combat["EndEncounter"] = cc.EndEncounter(ctx, connect.NewRequest(&playv1.EndEncounterRequest{CampaignId: id}))
	_, combat["GetTurnOptions"] = cc.GetTurnOptions(ctx, connect.NewRequest(&playv1.GetTurnOptionsRequest{CampaignId: id}))
	_, combat["RollAttack"] = cc.RollAttack(ctx, connect.NewRequest(&playv1.RollAttackRequest{CampaignId: id}))
	_, combat["RollDamage"] = cc.RollDamage(ctx, connect.NewRequest(&playv1.RollDamageRequest{CampaignId: id}))
	_, combat["ApplyPendingDamage"] = cc.ApplyPendingDamage(ctx, connect.NewRequest(&playv1.ApplyPendingDamageRequest{CampaignId: id}))
	_, combat["DiscardPendingDamage"] = cc.DiscardPendingDamage(ctx, connect.NewRequest(&playv1.DiscardPendingDamageRequest{CampaignId: id}))
	_, combat["TakeAction"] = cc.TakeAction(ctx, connect.NewRequest(&playv1.TakeActionRequest{CampaignId: id}))
	_, combat["AdjustCombatantHitPoints"] = cc.AdjustCombatantHitPoints(ctx, connect.NewRequest(&playv1.AdjustCombatantHitPointsRequest{CampaignId: id}))
	_, combat["UndoLastAction"] = cc.UndoLastAction(ctx, connect.NewRequest(&playv1.UndoLastActionRequest{CampaignId: id}))
	_, combat["CastSpell"] = cc.CastSpell(ctx, connect.NewRequest(&playv1.CastSpellRequest{CampaignId: id}))
	_, combat["UseReaction"] = cc.UseReaction(ctx, connect.NewRequest(&playv1.UseReactionRequest{CampaignId: id}))
	_, combat["DeclineReaction"] = cc.DeclineReaction(ctx, connect.NewRequest(&playv1.DeclineReactionRequest{CampaignId: id}))
	_, combat["DeclineOpportunity"] = cc.DeclineOpportunity(ctx, connect.NewRequest(&playv1.DeclineOpportunityRequest{CampaignId: id}))
	_, combat["SpendMovement"] = cc.SpendMovement(ctx, connect.NewRequest(&playv1.SpendMovementRequest{CampaignId: id}))
	_, combat["OfferOpportunity"] = cc.OfferOpportunity(ctx, connect.NewRequest(&playv1.OfferOpportunityRequest{CampaignId: id}))
	_, combat["WithdrawOpportunity"] = cc.WithdrawOpportunity(ctx, connect.NewRequest(&playv1.WithdrawOpportunityRequest{CampaignId: id}))
	_, combat["SkipOpportunity"] = cc.SkipOpportunity(ctx, connect.NewRequest(&playv1.SkipOpportunityRequest{CampaignId: id}))
	_, combat["RollDeathSave"] = cc.RollDeathSave(ctx, connect.NewRequest(&playv1.RollDeathSaveRequest{CampaignId: id}))
	_, combat["ConfirmDeath"] = cc.ConfirmDeath(ctx, connect.NewRequest(&playv1.ConfirmDeathRequest{CampaignId: id}))
	_, combat["SetCombatantConditions"] = cc.SetCombatantConditions(ctx, connect.NewRequest(&playv1.SetCombatantConditionsRequest{CampaignId: id}))
	_, combat["EndConcentration"] = cc.EndConcentration(ctx, connect.NewRequest(&playv1.EndConcentrationRequest{CampaignId: id}))
	_, combat["ListCombatLog"] = cc.ListCombatLog(ctx, connect.NewRequest(&playv1.ListCombatLogRequest{CampaignId: id}))
	_, combat["GetCombatHighlights"] = cc.GetCombatHighlights(ctx, connect.NewRequest(&playv1.GetCombatHighlightsRequest{CampaignId: id}))
	combatMethods := playv1.File_meurpg_play_v1_combat_proto.Services().ByName("CombatService").Methods()
	if len(combat) != combatMethods.Len() {
		t.Errorf("called %d combat methods, the service has %d", len(combat), combatMethods.Len())
	}
	maps.Copy(calls, combat)
	for name, err := range calls {
		if connect.CodeOf(err) != connect.CodeUnauthenticated {
			t.Errorf("%s signed out: error = %v, want unauthenticated", name, err)
		}
		if ce, ok := errors.AsType[*connect.Error](err); !ok || !slices.Equal(ce.Meta().Values("Cache-Control"), []string{"no-store"}) {
			t.Errorf("%s: error Cache-Control = %q, want no-store, once", name, ce.Meta().Values("Cache-Control"))
		}
	}

	for method, want := range map[string]descriptorpb.MethodOptions_IdempotencyLevel{
		"GetTurnOptions": descriptorpb.MethodOptions_IDEMPOTENT,
		"GetMoveOptions": descriptorpb.MethodOptions_IDEMPOTENT,
		"ListCombatLog":  descriptorpb.MethodOptions_IDEMPOTENT,
	} {
		opts, _ := combatMethods.ByName(protoreflect.Name(method)).Options().(*descriptorpb.MethodOptions)
		if opts.GetIdempotencyLevel() != want {
			t.Errorf("%s idempotency_level = %v, want %v", method, opts.GetIdempotencyLevel(), want)
		}
	}
	for method, want := range map[string]descriptorpb.MethodOptions_IdempotencyLevel{
		"ListGameSessions":     descriptorpb.MethodOptions_IDEMPOTENT,
		"GetLiveSession":       descriptorpb.MethodOptions_IDEMPOTENT,
		"GetSessionSummary":    descriptorpb.MethodOptions_IDEMPOTENT,
		"ListLeftImages":       descriptorpb.MethodOptions_IDEMPOTENT,
		"ListOpenGameSessions": descriptorpb.MethodOptions_NO_SIDE_EFFECTS, // an empty request
	} {
		opts, _ := methods.ByName(protoreflect.Name(method)).Options().(*descriptorpb.MethodOptions)
		if opts.GetIdempotencyLevel() != want {
			t.Errorf("%s idempotency_level = %v, want %v", method, opts.GetIdempotencyLevel(), want)
		}
	}
	for _, procedure := range []string{
		playv1connect.PlayServiceListGameSessionsProcedure, playv1connect.PlayServiceGetLiveSessionProcedure,
		playv1connect.CombatServiceGetTurnOptionsProcedure, playv1connect.CombatServiceListCombatLogProcedure,
	} {
		target := procedure + "?connect=v1&encoding=json&message=" + url.QueryEscape(`{"campaignId":"`+id+`"}`)
		rec := httptest.NewRecorder()
		mux.ServeHTTP(rec, httptest.NewRequestWithContext(ctx, http.MethodGet, target, nil))
		if rec.Code != http.StatusMethodNotAllowed {
			t.Errorf("GET %s: status = %d, want 405", procedure, rec.Code)
		}
	}
}

// firstEventError opens WatchGameSession and returns the error that ends
// it before or at its first event (nil if the first event arrived).
func firstEventError(ctx context.Context, c playv1connect.PlayServiceClient, campaignID string) error {
	stream, err := c.WatchGameSession(ctx, connect.NewRequest(&playv1.WatchGameSessionRequest{CampaignId: campaignID}))
	if err != nil {
		return err
	}
	defer stream.Close()
	if stream.Receive() {
		return nil
	}
	return stream.Err()
}

// emptyTrapBook is a map book with no trap: the authorization matrix's harness has
// no maps module.
type emptyTrapBook struct{}

func (emptyTrapBook) Traps(context.Context, pgx.Tx, string, string) ([]maplink.Trap, error) {
	return nil, nil
}

func (emptyTrapBook) KnownTraps(context.Context, string, string, string) ([]maplink.Trap, error) {
	return nil, nil
}

func (emptyTrapBook) Notice(context.Context, string, string, []maplink.Observer) error { return nil }

func (emptyTrapBook) SearchTraps(context.Context, pgx.Tx, string, string, maplink.Observer, string, []int, time.Time) (maplink.SearchResult, error) {
	return maplink.SearchResult{}, nil
}
func (emptyTrapBook) Told(context.Context, string, string, []string) {}
func (emptyTrapBook) TriggerTrap(context.Context, pgx.Tx, string, string, string, time.Time) (maplink.Trap, error) {
	return maplink.Trap{}, maplink.ErrTrapNotArmed
}

func (emptyTrapBook) RestoreTrap(context.Context, pgx.Tx, string, string, string, string, *time.Time, time.Time) error {
	return nil
}
func (emptyTrapBook) TrapChanged(context.Context, string, string, string) {}
func (emptyTrapBook) TrapNames(context.Context, string, []string) (map[string]string, error) {
	return nil, nil
}
