package play

import (
	"sync"
	"testing"

	"connectrpc.com/connect"
	"google.golang.org/protobuf/proto"

	playv1 "github.com/PuraFome/meuRPG/backend/gen/meurpg/play/v1"
	"github.com/PuraFome/meuRPG/backend/internal/platform/dbtest"
)

// The audit of 07/10/2026 (F7): a retried StartGameSession or CreatePuzzle must not start a second
// session or make a second puzzle.

func (u *user) startWithKey(t *testing.T, campaignID, key string) (*playv1.StartGameSessionResponse, error) {
	t.Helper()
	res, err := u.play.StartGameSession(t.Context(), connect.NewRequest(&playv1.StartGameSessionRequest{CampaignId: campaignID, IdempotencyKey: key}))
	if err != nil {
		return nil, err
	}
	return res.Msg, nil
}

func TestStartGameSessionIsIdempotent(t *testing.T) {
	t.Parallel()
	h := newHarness(t)
	master := h.newUser("Mestre")
	campaign := h.newCampaign(master, "Mirathel")

	first, err := master.startWithKey(t, campaign, "key-1")
	if err != nil {
		t.Fatalf("StartGameSession() error = %v", err)
	}
	// A retry while the session is open is the same session, not "a session is open".
	again, err := master.startWithKey(t, campaign, "key-1")
	if err != nil {
		t.Fatalf("StartGameSession() retry error = %v", err)
	}
	if again.GetGameSession().GetId() != first.GetGameSession().GetId() {
		t.Errorf("retry = session %q, want the first, %q", again.GetGameSession().GetId(), first.GetGameSession().GetId())
	}
	// A retry that arrives after the session ended is still the first session: it starts none.
	master.end(t, first.GetGameSession())
	late, err := master.startWithKey(t, campaign, "key-1")
	if err != nil || late.GetGameSession().GetId() != first.GetGameSession().GetId() || late.GetGameSession().GetEndedAt() == nil {
		t.Errorf("late retry = %v, %v; want the first session, ended", late, err)
	}
	// A new key is a new session; no key is not deduplicated, as before.
	second, err := master.startWithKey(t, campaign, "key-2")
	if err != nil || second.GetGameSession().GetSessionNumber() != 2 {
		t.Errorf("new key = %v, %v; want session 2", second, err)
	}
	master.end(t, second.GetGameSession())
	third, err := master.startWithKey(t, campaign, "")
	if err != nil || third.GetGameSession().GetSessionNumber() != 3 {
		t.Errorf("no key = %v, %v; want session 3", third, err)
	}
	// An open session still refuses a new key.
	_, err = master.startWithKey(t, campaign, "key-4")
	wantCode(t, "StartGameSession(key, a session open)", err, connect.CodeFailedPrecondition)
	_, err = master.startWithKey(t, campaign, string(make([]byte, 65)))
	wantCode(t, "StartGameSession(a key too long)", err, connect.CodeInvalidArgument)
}

func TestStartGameSessionWithTheSameKeyAtOnceStartsOne(t *testing.T) {
	t.Parallel()
	dbtest.PoolSize(t, 4) // the racers must overlap: one connection would run them one by one
	h := newHarness(t)
	master := h.newUser("Mestre")
	campaign := h.newCampaign(master, "Mirathel")

	ids := make([]string, 4)
	var wg sync.WaitGroup
	for i := range ids {
		wg.Go(func() {
			res, err := master.startWithKey(t, campaign, "racing")
			if err != nil {
				t.Errorf("StartGameSession() error = %v", err)
				return
			}
			ids[i] = res.GetGameSession().GetId()
		})
	}
	wg.Wait()
	for _, id := range ids {
		if id != ids[0] {
			t.Fatalf("ids = %v, want one session", ids)
		}
	}
	list, err := master.play.ListGameSessions(t.Context(), connect.NewRequest(&playv1.ListGameSessionsRequest{CampaignId: campaign}))
	if err != nil || len(list.Msg.GetGameSessions()) != 1 {
		t.Errorf("ListGameSessions() = %v, %v; want 1 session", list, err)
	}
}

func TestCreatePuzzleIsIdempotent(t *testing.T) {
	t.Parallel()
	p := newPuzzleTable(t)
	ctx := t.Context()
	create := func(req *playv1.CreatePuzzleRequest) (*playv1.Puzzle, error) {
		res, err := p.pc(p.master).CreatePuzzle(ctx, connect.NewRequest(req))
		if err != nil {
			return nil, err
		}
		return res.Msg.GetPuzzle(), nil
	}
	req := &playv1.CreatePuzzleRequest{
		CampaignId: p.campaignID, Name: "Luzes", Config: lightsConfig(4), Clue: "Só o selo apagado abre o caminho.", IdempotencyKey: "key-1",
	}
	first, err := create(req)
	if err != nil {
		t.Fatalf("CreatePuzzle() error = %v", err)
	}
	// The start is drawn when it is not given: a retry returns the first one, not a new draw.
	again, err := create(req)
	if err != nil {
		t.Fatalf("CreatePuzzle() retry error = %v", err)
	}
	if again.GetId() != first.GetId() || !proto.Equal(again, first) {
		t.Errorf("retry = %v, want the first puzzle %v", again, first)
	}
	// The same key with another request is refused, and makes nothing.
	other := proto.Clone(req).(*playv1.CreatePuzzleRequest)
	other.Name = "Outro"
	_, err = create(other)
	wantCode(t, "CreatePuzzle(same key, other name)", err, connect.CodeInvalidArgument)
	list, err := p.pc(p.master).ListPuzzles(ctx, connect.NewRequest(&playv1.ListPuzzlesRequest{CampaignId: p.campaignID}))
	if err != nil || len(list.Msg.GetPuzzles()) != 1 {
		t.Errorf("ListPuzzles() = %v, %v; want 1 puzzle", list, err)
	}
	// A new key is a new puzzle; no key is not deduplicated, as before.
	next := proto.Clone(req).(*playv1.CreatePuzzleRequest)
	next.IdempotencyKey = "key-2"
	if made, err := create(next); err != nil || made.GetId() == first.GetId() {
		t.Errorf("new key = %v, %v; want another puzzle", made, err)
	}
	next.IdempotencyKey = ""
	a, errA := create(next)
	b, errB := create(next)
	if errA != nil || errB != nil || a.GetId() == b.GetId() {
		t.Errorf("no key made %v and %v (%v, %v), want two puzzles", a, b, errA, errB)
	}
}

// A combat change keeps the hash of its request with its key: the same key sent again with the
// same request replays the first answer, and sent with another request is refused instead of
// answering success and applying nothing.
func TestACombatKeyReusedForAnotherRequestIsRefused(t *testing.T) {
	t.Parallel()
	t.Run("SubmitInitiative", func(t *testing.T) {
		t.Parallel()
		f := newFight(t)
		e := f.startFight(t, 1)
		key := newKey()
		submit := func(label string, face int32) error {
			_, err := f.master.combat.SubmitInitiative(t.Context(), connect.NewRequest(&playv1.SubmitInitiativeRequest{
				CampaignId: f.campaignID, EncounterId: e.GetId(), CombatantId: byLabel(t, e, label).GetId(), IdempotencyKey: key,
				Roll: &playv1.SubmitInitiativeRequest_D20Face{D20Face: face},
			}))
			return err
		}
		if err := submit("Toren", 15); err != nil {
			t.Fatalf("first SubmitInitiative() error = %v", err)
		}
		if err := submit("Toren", 15); err != nil {
			t.Errorf("SubmitInitiative(same key, same request) error = %v, want the first answer", err)
		}
		wantCode(t, "SubmitInitiative(same key, another combatant)", submit("Pensantus", 9), connect.CodeInvalidArgument)
		wantCode(t, "SubmitInitiative(same key, another roll)", submit("Toren", 3), connect.CodeInvalidArgument)
	})
	t.Run("MoveCombatant", func(t *testing.T) {
		t.Parallel()
		c := newCave(t)
		e := c.fight(t)
		key := newKey()
		move := func(label string, col, row int32) error {
			_, err := c.master.combat.MoveCombatant(t.Context(), connect.NewRequest(&playv1.MoveCombatantRequest{
				CampaignId: c.campaignID, EncounterId: e.GetId(), CombatantId: c.id(t, label), IdempotencyKey: key, Col: col, Row: row, Forced: true,
			}))
			return err
		}
		if err := move("Toren", 7, 7); err != nil {
			t.Fatalf("first MoveCombatant() error = %v", err)
		}
		if err := move("Toren", 7, 7); err != nil {
			t.Errorf("MoveCombatant(same key, same square) error = %v, want the first answer", err)
		}
		wantCode(t, "MoveCombatant(same key, another square)", move("Toren", 9, 7), connect.CodeInvalidArgument)
	})
}
