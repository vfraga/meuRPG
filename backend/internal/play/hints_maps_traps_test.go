package play

import (
	"net/http"
	"testing"

	"connectrpc.com/connect"
	"google.golang.org/protobuf/proto"

	mapsv1 "github.com/PuraFome/meuRPG/backend/gen/meurpg/maps/v1"
	"github.com/PuraFome/meuRPG/backend/gen/meurpg/maps/v1/mapsv1connect"
	playv1 "github.com/PuraFome/meuRPG/backend/gen/meurpg/play/v1"
)

// The writes of the maps module that change a trap, a treasure or a scene's clues run under play's funnel (Service.InSession,
// #735 part 2, 4d maps): one number for each write, carried by what it tells; a write that changes nothing moves nothing; a
// refused call tells nothing; a keyed one replays without a new number and tells again; with no open session nothing moves.
// A trap or a treasure the players do not see tells the master alone: the players' number moves the same.

func (r *trapRig) treasureClient(u *user) mapsv1connect.TreasureServiceClient {
	return mapsv1connect.NewTreasureServiceClient(&http.Client{Transport: userTransport{userID: u.id, next: r.server.Client().Transport}}, r.server.URL)
}

func (r *trapRig) placeTreasureCall(t *testing.T, key string, col int32) func() error {
	t.Helper()
	content, err := testRules()
	if err != nil {
		t.Fatalf("rules.LoadSRD() error = %v", err)
	}
	version := content.Version()
	return func() error {
		_, err := r.treasureClient(r.master).PlaceTreasure(t.Context(), connect.NewRequest(&mapsv1.PlaceTreasureRequest{
			CampaignId: r.campaignID, MapId: r.mapID, Mode: mapsv1.TreasureMode_TREASURE_MODE_HOARD, PartyLevel: 4,
			Seed: proto.Uint64(2209), Column: col, Row: 5, IdempotencyKey: key, ContentVersion: version,
		}))
		return err
	}
}

func TestPlaceTreasureHints(t *testing.T) {
	t.Parallel()
	r, g := mapRigHints(t, "Caio", "Ana", "Bia") // a new treasure is hidden: the master alone hears
	key := newKey()
	g.keyed(r.placeTreasureCall(t, key, 7), r.placeTreasureCall(t, key, 7), r.placeTreasureCall(t, key, 8))
	g.lost(r.placeTreasureCall(t, newKey(), 9))
}

func TestRevealTrapHints(t *testing.T) {
	t.Parallel()
	r := newTrapRig(t)
	p := r.trap(t, "Poço", 12, 7)
	g := newHintRig(t, r.h, r.campaignID, r.master, map[string]*user{"Caio": r.caio, "Ana": r.ana, "Bia": r.bia}, "Ana", "Bia")
	reveal := func(ids ...string) func() error {
		return func() error {
			_, err := r.mc(r.master).RevealTrap(t.Context(), connect.NewRequest(&mapsv1.RevealTrapRequest{
				CampaignId: r.campaignID, MapId: r.mapID, PointId: p.GetId(), CharacterIds: ids, All: len(ids) == 0,
			}))
			return err
		}
	}
	// Toren's player (Caio) and the master hear; the others hear nothing but their number moves the same.
	g.first(reveal(r.toren.GetId()), true)
	// What everyone already knew changes nothing.
	before := g.seq()
	if err := reveal(r.toren.GetId())(); err != nil {
		t.Fatalf("RevealTrap(again) error = %v", err)
	}
	if got := g.seq(); got != before {
		t.Errorf("hint_seq = %d after a reveal that changed nothing, want %d", got, before)
	}
	g.refused(func() error {
		_, err := r.mc(r.master).RevealTrap(t.Context(), connect.NewRequest(&mapsv1.RevealTrapRequest{CampaignId: r.campaignID, MapId: r.mapID, PointId: newKey(), All: true}))
		return err
	})
}

func TestDisarmTrapHints(t *testing.T) {
	t.Parallel()
	r := newTrapRig(t)
	p := r.trap(t, "Poço", 12, 7)
	g := newHintRig(t, r.h, r.campaignID, r.master, map[string]*user{"Caio": r.caio, "Ana": r.ana, "Bia": r.bia}, "Caio", "Ana", "Bia") // hidden: the master alone
	disarm := func() error {
		_, err := r.mc(r.master).DisarmTrap(t.Context(), connect.NewRequest(&mapsv1.DisarmTrapRequest{CampaignId: r.campaignID, MapId: r.mapID, PointId: p.GetId()}))
		return err
	}
	g.first(disarm, true)
	before := g.seq()
	if err := disarm(); err != nil {
		t.Fatalf("DisarmTrap(again) error = %v", err)
	}
	if got := g.seq(); got != before {
		t.Errorf("hint_seq = %d after a disarm that changed nothing, want %d", got, before)
	}
}

func TestTreasureFoundHints(t *testing.T) {
	t.Parallel()
	r, g := mapRigHints(t)
	if err := r.placeTreasureCall(t, newKey(), 7)(); err != nil {
		t.Fatalf("PlaceTreasure() error = %v", err)
	}
	var id string
	if err := r.h.pool.QueryRow(t.Context(), `SELECT id::TEXT FROM map_points WHERE map_id = $1 AND kind = 'treasure'`, r.mapID).Scan(&id); err != nil {
		t.Fatalf("find the treasure: %v", err)
	}
	g.drain()
	mc := r.mc(r.master)
	// A found treasure is visible to everyone who sees the map: all hear, with the number.
	g.first(func() error {
		_, err := mc.MarkTreasureFound(t.Context(), connect.NewRequest(&mapsv1.MarkTreasureFoundRequest{CampaignId: r.campaignID, MapId: r.mapID, PointId: id, CharacterIds: []string{r.toren.GetId()}}))
		return err
	}, true)
	g.first(func() error {
		_, err := mc.UnmarkTreasureFound(t.Context(), connect.NewRequest(&mapsv1.UnmarkTreasureFoundRequest{CampaignId: r.campaignID, MapId: r.mapID, PointId: id}))
		return err
	}, true)
	g.refused(func() error {
		_, err := mc.MarkTreasureFound(t.Context(), connect.NewRequest(&mapsv1.MarkTreasureFoundRequest{CampaignId: r.campaignID, MapId: r.mapID, PointId: newKey(), CharacterIds: []string{r.toren.GetId()}}))
		return err
	})
}

func TestRevealSceneClueHints(t *testing.T) {
	t.Parallel()
	r, g := mapRigHints(t, "Caio", "Bia")
	x, y := sq(3, 3)
	pt, err := r.mc(r.master).CreateMapPoint(t.Context(), connect.NewRequest(&mapsv1.CreateMapPointRequest{CampaignId: r.campaignID, MapId: r.mapID, Kind: mapsv1.MapPointKind_MAP_POINT_KIND_SCENE, Name: "Biblioteca", XBp: x, YBp: y}))
	if err != nil {
		t.Fatalf("CreateMapPoint() error = %v", err)
	}
	clue, err := r.mc(r.master).AddSceneClue(t.Context(), connect.NewRequest(&mapsv1.AddSceneClueRequest{CampaignId: r.campaignID, MapId: r.mapID, PointId: pt.Msg.GetPoint().GetId(), Text: "Uma chave"}))
	if err != nil {
		t.Fatalf("AddSceneClue() error = %v", err)
	}
	g.drain()
	reveal := func() error {
		_, err := r.mc(r.master).RevealSceneClue(t.Context(), connect.NewRequest(&mapsv1.RevealSceneClueRequest{CampaignId: r.campaignID, ClueId: clue.Msg.GetClue().GetId(), CharacterIds: []string{r.pens.GetId()}}))
		return err
	}
	// Pensantus's player (Ana) hears that her notes changed; nobody else does.
	g.first(reveal, true)
	before := g.seq()
	if err := reveal(); err != nil {
		t.Fatalf("RevealSceneClue(again) error = %v", err)
	}
	if got := g.seq(); got != before {
		t.Errorf("hint_seq = %d after a reveal of what she had, want %d", got, before)
	}
	g.refused(func() error {
		_, err := r.mc(r.master).RevealSceneClue(t.Context(), connect.NewRequest(&mapsv1.RevealSceneClueRequest{CampaignId: r.campaignID, ClueId: newKey(), CharacterIds: []string{r.pens.GetId()}}))
		return err
	})
}

// A token dropped within reach of a trap its character notices is one write: the token's, with the notice's reveal. It moves the
// number once; the notice is told to the finder alone (trap_noticed, and the map's change), and the others' number moves the same.
func TestNoticeTellsTheFinderAloneWithTheNumber(t *testing.T) {
	t.Parallel()
	r := newTrapRig(t)
	r.trap(t, "Fosso Dourado", 9, 7, func(s *mapsv1.TrapSpec) { s.NoticeDc = 10 })
	g := newHintRig(t, r.h, r.campaignID, r.master, map[string]*user{"Caio": r.caio, "Ana": r.ana, "Bia": r.bia})
	before := g.seq()
	r.place(t, r.toren.GetId(), 7, 7)
	if got := g.seq(); got != before+1 {
		t.Errorf("hint_seq = %d, want %d (the token with its notice)", got, before+1)
	}
	heard := g.drain()
	noticed := func(name string) bool {
		for _, ev := range heard[name] {
			if _, ok := ev.GetEvent().(*playv1.WatchGameSessionResponse_TrapNoticed_); ok {
				if ev.GetSessionSeq() == 0 {
					t.Errorf("%s heard the notice with no number", name)
				}
				return true
			}
		}
		return false
	}
	if !noticed("Caio") {
		t.Errorf("Toren's player did not hear that he noticed the trap")
	}
	for _, name := range []string{"Ana", "Bia", "master"} {
		if noticed(name) {
			t.Errorf("%s heard of a trap Toren noticed", name)
		}
	}
}

// A token dropped on an armed trap is two writes: the token's and the trap's firing. The master's `map_changed` the firing tells
// (TrapChanged, called by play after its own commit) never carries the drop's number: a stale number is worse than none.
func TestATokenDropThatFiresATrapTellsTheFiringWithoutTheDropsNumber(t *testing.T) {
	t.Parallel()
	r := newTrapRig(t)
	r.trap(t, "Poço de entrada", 12, 7, pit("1d6"))
	g := newHintRig(t, r.h, r.campaignID, r.master, map[string]*user{"Caio": r.caio, "Ana": r.ana, "Bia": r.bia})
	before := g.seq()
	r.h.roller.queue(3)
	r.place(t, r.toren.GetId(), 12, 7)
	if got := g.seq(); got < before+2 {
		t.Fatalf("hint_seq = %d, want at least %d (the token, then the firing)", got, before+2)
	}
	stale := 0
	for _, ev := range g.drain()["master"] {
		if _, ok := ev.GetEvent().(*playv1.WatchGameSessionResponse_MapChanged_); ok && ev.GetSessionSeq() == before+1 {
			stale++
		}
	}
	if stale != 1 {
		t.Errorf("the master heard %d map_changed with the drop's number %d, want only the drop's own", stale, before+1)
	}
}

// A token dropped on an armed "Ao entrar na área" trap that the drop does not fire (no open session, a combat of the theatre of the
// mind) is noticed as any trap beside it: the finder knows it, and the landing is one write.
func TestALandingOnATrapThatFiresNothingStillNotices(t *testing.T) {
	t.Parallel()
	for name, tc := range map[string]struct {
		setup func(t *testing.T, r *trapRig)
		moves int64
	}{
		"no open session": {setup: func(t *testing.T, r *trapRig) {
			t.Helper()
			r.master.end(t, r.master.liveSession(t, r.campaignID).GetGameSession())
		}, moves: 0},
		"theatre of the mind": {setup: func(t *testing.T, r *trapRig) {
			t.Helper()
			r.start(t, plan{theatre: true, npcs: []*playv1.Participant{{CharacterId: r.goblins.GetId()}}, npcRolls: []int{2}, players: map[string]int32{"Toren": 18, "Pensantus": 10, "Brisa": 1}})
		}, moves: 1},
	} {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			r := newTrapRig(t)
			hole := r.trap(t, "Fosso Dourado", 9, 7, func(s *mapsv1.TrapSpec) { s.NoticeDc = 10; pit("1d6")(s) })
			if _, err := r.mc(r.master).SetMapRevealed(t.Context(), connect.NewRequest(&mapsv1.SetMapRevealedRequest{CampaignId: r.campaignID, MapId: r.mapID, Revealed: true})); err != nil {
				t.Fatalf("SetMapRevealed() error = %v", err)
			}
			g := newHintRig(t, r.h, r.campaignID, r.master, map[string]*user{"Caio": r.caio, "Ana": r.ana, "Bia": r.bia})
			tc.setup(t, r)
			before := g.seq()
			r.place(t, r.toren.GetId(), 9, 7)
			if got := g.seq(); got != before+tc.moves {
				t.Errorf("hint_seq = %d, want %d", got, before+tc.moves)
			}
			r.wantKnows(t, "Toren's player", r.caio, hole.GetId(), true)
			r.wantKnows(t, "Pensantus's player", r.ana, hole.GetId(), false)
			if got := r.stateOf(t, r.mapID, hole.GetId()); got != mapsv1.TrapState_TRAP_STATE_ARMED {
				t.Errorf("the pit is %v, want it armed: nothing fires it", got)
			}
		})
	}
}

// A token dropped on a trap that goes off with the drop is not also noticed: a trap that fires is known to all.
func TestALandingThatFiresATrapIsNotAlsoNoticed(t *testing.T) {
	t.Parallel()
	r := newTrapRig(t)
	r.trap(t, "Poço de entrada", 12, 7, func(s *mapsv1.TrapSpec) { s.NoticeDc = 10; pit("1d6")(s) })
	r.h.roller.queue(3)
	r.place(t, r.toren.GetId(), 12, 7)
	if n := r.eventCount(t, "trap_triggered"); n != 1 {
		t.Fatalf("trap_triggered events = %d, want 1", n)
	}
	if n := r.eventCount(t, "trap_noticed"); n != 0 {
		t.Errorf("trap_noticed events = %d, want 0: the trap that fired was not also noticed", n)
	}
}
