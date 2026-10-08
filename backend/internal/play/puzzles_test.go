package play

import (
	"encoding/json"
	"fmt"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"slices"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"connectrpc.com/connect"
	"google.golang.org/protobuf/encoding/protojson"
	"google.golang.org/protobuf/proto"

	mapsv1 "github.com/PuraFome/meuRPG/backend/gen/meurpg/maps/v1"
	"github.com/PuraFome/meuRPG/backend/gen/meurpg/maps/v1/mapsv1connect"
	playv1 "github.com/PuraFome/meuRPG/backend/gen/meurpg/play/v1"
	"github.com/PuraFome/meuRPG/backend/gen/meurpg/play/v1/playv1connect"
	"github.com/PuraFome/meuRPG/backend/internal/maps"
	"github.com/PuraFome/meuRPG/backend/internal/platform/dbtest"
	"github.com/PuraFome/meuRPG/backend/internal/platform/httpserver"
	"github.com/PuraFome/meuRPG/backend/internal/rules/grid"
)

// Puzzles (MR-038, RN-27, RN-10, Etapa 10, slice 10.7a). The fixture is the armed
// campaign (a master and three players with a character each, the session open,
// a map with a grid) with the real maps service behind it, so "Ao resolver" runs
// for real: the doors layer, the points and the clues. Every answer a player gets
// is read as the app's JSON (protojson), so a field that leaks shows up whatever
// its name.

type puzzleTable struct {
	*armed
	msvc   *maps.Service
	server *httptest.Server
}

func newPuzzleTable(t *testing.T) *puzzleTable {
	t.Helper()
	a := newArmed(t)
	content, err := testRules()
	if err != nil {
		t.Fatalf("rules.LoadSRD() error = %v", err)
	}
	msvc, err := maps.New(maps.Config{Pool: a.h.pool, Characters: a.h.chars, Live: a.h.svc, Rules: content, Combats: a.h.svc, Logger: slog.New(slog.DiscardHandler)})
	if err != nil {
		t.Fatalf("maps.New() error = %v", err)
	}
	a.h.svc.SetTerrain(msvc)
	a.h.svc.SetPuzzleMaps(msvc)
	a.h.svc.SetTraps(msvc) // a wrong move may fire a trap point ("Ao errar", slice 10.7b)
	srv := httpserver.New(httpserver.Config{Logger: slog.New(slog.DiscardHandler)})
	msvc.Mount(srv.Handle, mapsSessions{testSessions}, a.h.camps, connect.WithRequireConnectProtocolHeader())
	server := httptest.NewServer(srv.Handler())
	t.Cleanup(server.Close)
	return &puzzleTable{armed: a, msvc: msvc, server: server}
}

func (p *puzzleTable) pc(u *user) playv1connect.PuzzleServiceClient {
	return playv1connect.NewPuzzleServiceClient(&http.Client{Transport: userTransport{userID: u.id, next: p.h.server.Client().Transport}}, p.h.server.URL)
}

func (p *puzzleTable) mc(u *user) mapsv1connect.MapServiceClient {
	return mapsv1connect.NewMapServiceClient(&http.Client{Transport: userTransport{userID: u.id, next: p.server.Client().Transport}}, p.server.URL)
}

// --- the puzzles ---

func lightsConfig(n int32) *playv1.PuzzleConfig {
	return &playv1.PuzzleConfig{Kind: &playv1.PuzzleConfig_Lights{Lights: &playv1.LightsConfig{Size: n}}}
}

func lockConfig(wheels int32, a playv1.PuzzleAlphabet) *playv1.PuzzleConfig {
	return &playv1.PuzzleConfig{Kind: &playv1.PuzzleConfig_Lock{Lock: &playv1.LockConfig{Wheels: wheels, Alphabet: a}}}
}

func pillarsConfig(pillars, symbols int32, links ...[]int32) *playv1.PuzzleConfig {
	c := &playv1.PillarsConfig{Pillars: pillars, Symbols: symbols}
	for _, l := range links {
		c.Links = append(c.Links, &playv1.PillarLinks{AlsoTurns: l})
	}
	return &playv1.PuzzleConfig{Kind: &playv1.PuzzleConfig_Pillars{Pillars: c}}
}

func lockSolution(w ...int32) *playv1.PuzzleSolution {
	return &playv1.PuzzleSolution{Kind: &playv1.PuzzleSolution_Lock{Lock: &playv1.LockSolution{Wheels: w}}}
}

func lockStart(w ...int32) *playv1.PuzzleState {
	return &playv1.PuzzleState{Kind: &playv1.PuzzleState_Lock{Lock: &playv1.LockState{Wheels: w}}}
}

func pillarsSolution(p ...int32) *playv1.PuzzleSolution {
	return &playv1.PuzzleSolution{Kind: &playv1.PuzzleSolution_Pillars{Pillars: &playv1.PillarsSolution{Pillars: p}}}
}

func lockMove(wheel, delta int32) *playv1.PuzzleMove {
	return &playv1.PuzzleMove{Kind: &playv1.PuzzleMove_Lock{Lock: &playv1.LockMove{Wheel: wheel, Delta: delta}}}
}

func lightsMove(row, col int32) *playv1.PuzzleMove {
	return &playv1.PuzzleMove{Kind: &playv1.PuzzleMove_Lights{Lights: &playv1.LightsMove{Row: row, Col: col}}}
}

// create makes a puzzle as the master, or fails the test.
func (p *puzzleTable) create(t *testing.T, req *playv1.CreatePuzzleRequest) *playv1.Puzzle {
	t.Helper()
	req.CampaignId = p.campaignID
	res, err := p.pc(p.master).CreatePuzzle(t.Context(), connect.NewRequest(req))
	if err != nil {
		t.Fatalf("CreatePuzzle(%q) error = %v", req.GetName(), err)
	}
	return res.Msg.GetPuzzle()
}

func (p *puzzleTable) lights(t *testing.T, name string, n int32, edit ...func(*playv1.CreatePuzzleRequest)) *playv1.Puzzle {
	t.Helper()
	req := &playv1.CreatePuzzleRequest{Name: name, Config: lightsConfig(n), Clue: "Só o selo apagado abre o caminho."}
	for _, e := range edit {
		e(req)
	}
	return p.create(t, req)
}

// lock makes a four-wheel lock of digits, solution 7 3 5 1, starting at 0 0 0 0.
func (p *puzzleTable) lock(t *testing.T, name string, edit ...func(*playv1.CreatePuzzleRequest)) *playv1.Puzzle {
	t.Helper()
	req := &playv1.CreatePuzzleRequest{
		Name: name, Config: lockConfig(4, playv1.PuzzleAlphabet_PUZZLE_ALPHABET_DIGITS),
		Solution: lockSolution(7, 3, 5, 1), Start: lockStart(0, 0, 0, 0), Clue: "O fogo nasce antes da lua.",
		Hints: []string{"dica um: três coisas da natureza", "dica dois: a raiz vê tudo", "dica três: a lua vem depois"},
	}
	for _, e := range edit {
		e(req)
	}
	return p.create(t, req)
}

// pillars makes four pillars of four symbols, each turning with its neighbors.
func (p *puzzleTable) pillars(t *testing.T, name string, edit ...func(*playv1.CreatePuzzleRequest)) *playv1.Puzzle {
	t.Helper()
	req := &playv1.CreatePuzzleRequest{
		Name: name, Config: pillarsConfig(4, 4, []int32{1}, []int32{0, 2}, []int32{1, 3}, []int32{2}),
		Solution: pillarsSolution(1, 0, 3, 2), Clue: "Os pilares obedecem ao mural.",
	}
	for _, e := range edit {
		e(req)
	}
	return p.create(t, req)
}

func (p *puzzleTable) show(t *testing.T, id string) *playv1.MasterPuzzleRun {
	t.Helper()
	res, err := p.pc(p.master).ShowPuzzle(t.Context(), connect.NewRequest(&playv1.ShowPuzzleRequest{CampaignId: p.campaignID, PuzzleId: id}))
	if err != nil {
		t.Fatalf("ShowPuzzle() error = %v", err)
	}
	return res.Msg.GetRun()
}

func (p *puzzleTable) masterRun(t *testing.T, id string) *playv1.MasterPuzzleRun {
	t.Helper()
	res, err := p.pc(p.master).GetMasterPuzzleRun(t.Context(), connect.NewRequest(&playv1.GetMasterPuzzleRunRequest{CampaignId: p.campaignID, PuzzleId: id}))
	if err != nil {
		t.Fatalf("GetMasterPuzzleRun() error = %v", err)
	}
	return res.Msg.GetRun()
}

func (p *puzzleTable) read(t *testing.T, u *user, id string) *playv1.PuzzleRun {
	t.Helper()
	res, err := p.pc(u).GetPuzzleRun(t.Context(), connect.NewRequest(&playv1.GetPuzzleRunRequest{CampaignId: p.campaignID, PuzzleId: id}))
	if err != nil {
		t.Fatalf("GetPuzzleRun() error = %v", err)
	}
	return res.Msg.GetRun()
}

// move makes a move as the player with a new key.
func (p *puzzleTable) move(t *testing.T, u *user, id string, mv *playv1.PuzzleMove) (*playv1.MakePuzzleMoveResponse, error) {
	t.Helper()
	return p.moveKey(t, u, id, mv, newKey())
}

func (p *puzzleTable) moveKey(t *testing.T, u *user, id string, mv *playv1.PuzzleMove, key string) (*playv1.MakePuzzleMoveResponse, error) {
	t.Helper()
	res, err := p.pc(u).MakePuzzleMove(t.Context(), connect.NewRequest(&playv1.MakePuzzleMoveRequest{CampaignId: p.campaignID, PuzzleId: id, Move: mv, IdempotencyKey: key}))
	if err != nil {
		return nil, err
	}
	return res.Msg, nil
}

func (p *puzzleTable) mustMove(t *testing.T, u *user, id string, mv *playv1.PuzzleMove) *playv1.MakePuzzleMoveResponse {
	t.Helper()
	res, err := p.move(t, u, id, mv)
	if err != nil {
		t.Fatalf("MakePuzzleMove() error = %v", err)
	}
	return res
}

// solve plays the master's shortest way as the player, to the last move.
func (p *puzzleTable) solve(t *testing.T, u *user, id string) *playv1.MakePuzzleMoveResponse {
	t.Helper()
	path := p.masterRun(t, id).GetMinimum().GetPath()
	if len(path) == 0 {
		t.Fatal("the puzzle has no moves left to solve it")
	}
	var last *playv1.MakePuzzleMoveResponse
	for _, mv := range path {
		last = p.mustMove(t, u, id, mv)
	}
	return last
}

func blockedReason(err error) playv1.PuzzleBlockedReason {
	if ce, ok := err.(*connect.Error); ok { //nolint:errorlint // the handler's error is a *connect.Error as it is
		for _, d := range ce.Details() {
			if v, err := d.Value(); err == nil {
				if b, ok := v.(*playv1.PuzzleBlocked); ok {
					return b.GetReason()
				}
			}
		}
	}
	return playv1.PuzzleBlockedReason_PUZZLE_BLOCKED_REASON_UNSPECIFIED
}

func invalidReason(err error) playv1.PuzzleInvalidReason {
	if ce, ok := err.(*connect.Error); ok { //nolint:errorlint // the handler's error is a *connect.Error as it is
		for _, d := range ce.Details() {
			if v, err := d.Value(); err == nil {
				if b, ok := v.(*playv1.PuzzleInvalid); ok {
					return b.GetReason()
				}
			}
		}
	}
	return playv1.PuzzleInvalidReason_PUZZLE_INVALID_REASON_UNSPECIFIED
}

func wantPuzzleBlocked(t *testing.T, call string, err error, want playv1.PuzzleBlockedReason) {
	t.Helper()
	wantCode(t, call, err, connect.CodeFailedPrecondition)
	if got := blockedReason(err); got != want {
		t.Fatalf("%s reason = %v, want %v", call, got, want)
	}
}

func wantPuzzleInvalid(t *testing.T, call string, err error, want playv1.PuzzleInvalidReason) {
	t.Helper()
	wantCode(t, call, err, connect.CodeInvalidArgument)
	if got := invalidReason(err); got != want {
		t.Fatalf("%s reason = %v, want %v", call, got, want)
	}
}

func litCount(s *playv1.PuzzleState) int {
	n := 0
	for _, l := range s.GetLights().GetLit() {
		if l {
			n++
		}
	}
	return n
}

// jsonOf is a message as the app receives it.
func jsonOf(m proto.Message) string { return protojson.Format(m) }

// --- the tests ---

// MR-038: the master makes a puzzle of each kind, in the campaign, and only the master reads it.
func TestMR038_CreateEachKindAndReadItAsTheMaster(t *testing.T) {
	t.Parallel()
	p := newPuzzleTable(t)
	ctx := t.Context()

	lights := p.lights(t, "O selo da Capela", 5)
	if lights.GetKind() != playv1.PuzzleKind_PUZZLE_KIND_LIGHTS || lights.GetName() != "O selo da Capela" {
		t.Errorf("lights puzzle = %v", lights)
	}
	// "Dado um Apagar as luzes de 5 por 5, quando o crio, então o servidor gera um começo que tem solução e nunca já resolvido, e eu vejo quantos toques bastam."
	if n := litCount(lights.GetStart()); n == 0 || len(lights.GetStart().GetLights().GetLit()) != 25 {
		t.Errorf("the start has %d lit of %d, want a start that is not solved", n, len(lights.GetStart().GetLights().GetLit()))
	}
	if !lights.GetMinimum().GetSolvable() || lights.GetMinimum().GetMoves() == 0 || int(lights.GetMinimum().GetMoves()) != len(lights.GetMinimum().GetPath()) {
		t.Errorf("the minimum = %v, want a solvable one with its path", lights.GetMinimum())
	}
	lock := p.lock(t, "O cofre do Refeitório", func(r *playv1.CreatePuzzleRequest) {
		r.Config = lockConfig(4, playv1.PuzzleAlphabet_PUZZLE_ALPHABET_RUNES)
		r.Solution, r.Start = lockSolution(0, 1, 2, 7), lockStart(1, 1, 2, 5)
	})
	if lock.GetMinimum().GetMoves() != 1+0+0+2 || len(lock.GetSymbols()) != 8 || lock.GetSymbols()[0].GetNamePt() != "Lua" {
		t.Errorf("lock minimum = %d, symbols = %v; want 3 turns and the eight runes named in Portuguese", lock.GetMinimum().GetMoves(), lock.GetSymbols())
	}
	pillars := p.pillars(t, "Os pilares da Galeria")
	if pillars.GetMinimum().GetMoves() < 3 || len(pillars.GetSymbols()) != 4 || len(pillars.GetConfig().GetPillars().GetLinks()) != 4 {
		t.Errorf("pillars = %v", pillars)
	}

	list, err := p.pc(p.master).ListPuzzles(ctx, connect.NewRequest(&playv1.ListPuzzlesRequest{CampaignId: p.campaignID}))
	if err != nil || len(list.Msg.GetPuzzles()) != 3 || list.Msg.GetPuzzles()[0].GetName() != "Os pilares da Galeria" {
		t.Fatalf("ListPuzzles() = %v, %v; want the three, newest first", list, err)
	}
	got, err := p.pc(p.master).GetPuzzle(ctx, connect.NewRequest(&playv1.GetPuzzleRequest{CampaignId: p.campaignID, PuzzleId: lock.GetId()}))
	if err != nil || !proto.Equal(got.Msg.GetPuzzle().GetSolution(), lock.GetSolution()) {
		t.Fatalf("GetPuzzle() = %v, %v", got, err)
	}

	// Nobody but the master reads or makes them.
	for _, u := range []*user{p.caio, p.ana} {
		_, err := p.pc(u).ListPuzzles(ctx, connect.NewRequest(&playv1.ListPuzzlesRequest{CampaignId: p.campaignID}))
		wantCode(t, "ListPuzzles as a player", err, connect.CodePermissionDenied)
		_, err = p.pc(u).GetPuzzle(ctx, connect.NewRequest(&playv1.GetPuzzleRequest{CampaignId: p.campaignID, PuzzleId: lock.GetId()}))
		wantCode(t, "GetPuzzle as a player", err, connect.CodePermissionDenied)
		_, err = p.pc(u).CreatePuzzle(ctx, connect.NewRequest(&playv1.CreatePuzzleRequest{CampaignId: p.campaignID, Name: "x", Config: lightsConfig(3)}))
		wantCode(t, "CreatePuzzle as a player", err, connect.CodePermissionDenied)
	}
	stranger := p.h.newUser("Intrusa")
	_, err = p.pc(stranger).ListPuzzles(ctx, connect.NewRequest(&playv1.ListPuzzlesRequest{CampaignId: p.campaignID}))
	wantCode(t, "ListPuzzles as a stranger", err, connect.CodeNotFound)
	_, err = p.pc(p.h.anonymous()).ListPuzzles(ctx, connect.NewRequest(&playv1.ListPuzzlesRequest{CampaignId: p.campaignID}))
	wantCode(t, "ListPuzzles signed out", err, connect.CodeUnauthenticated)
	_, err = p.pc(p.master).GetPuzzle(ctx, connect.NewRequest(&playv1.GetPuzzleRequest{CampaignId: p.campaignID, PuzzleId: newKey()}))
	wantCode(t, "GetPuzzle of another ID", err, connect.CodeNotFound)
}

// The master's input is checked, with the typed reason the form shows.
func TestMR038_CreateChecksWhatTheMasterWrote(t *testing.T) {
	t.Parallel()
	p := newPuzzleTable(t)
	ctx := t.Context()
	create := func(edit func(*playv1.CreatePuzzleRequest)) error {
		req := &playv1.CreatePuzzleRequest{CampaignId: p.campaignID, Name: "Um", Config: lockConfig(3, playv1.PuzzleAlphabet_PUZZLE_ALPHABET_LETTERS), Solution: lockSolution(1, 2, 3), Start: lockStart(0, 0, 0)}
		edit(req)
		_, err := p.pc(p.master).CreatePuzzle(ctx, connect.NewRequest(req))
		return err
	}
	lx, ly := sq(6, 6)
	light, err := p.mc(p.master).CreateMapPoint(ctx, connect.NewRequest(&mapsv1.CreateMapPointRequest{
		CampaignId: p.campaignID, MapId: p.mapID, Kind: mapsv1.MapPointKind_MAP_POINT_KIND_LIGHT, Name: "Tocha", XBp: lx, YBp: ly,
		Light: &mapsv1.LightSpec{PresetKey: "light:torch"},
	}))
	if err != nil {
		t.Fatal(err)
	}
	for _, tc := range []struct {
		name string
		edit func(*playv1.CreatePuzzleRequest)
		want playv1.PuzzleInvalidReason
	}{
		{"a light, which the players never see", func(r *playv1.CreatePuzzleRequest) {
			r.OnSolve = &playv1.PuzzleOnSolve{Action: playv1.PuzzleSolveAction_PUZZLE_SOLVE_ACTION_REVEAL_POINT, Target: &playv1.PuzzleOnSolve_Point{Point: &playv1.PuzzlePointTarget{MapId: p.mapID, PointId: light.Msg.GetPoint().GetId()}}}
		}, playv1.PuzzleInvalidReason_PUZZLE_INVALID_REASON_TARGET},
		{"no name", func(r *playv1.CreatePuzzleRequest) { r.Name = "  " }, playv1.PuzzleInvalidReason_PUZZLE_INVALID_REASON_NAME},
		{"a long name", func(r *playv1.CreatePuzzleRequest) { r.Name = strings.Repeat("a", 81) }, playv1.PuzzleInvalidReason_PUZZLE_INVALID_REASON_NAME},
		{"a long clue", func(r *playv1.CreatePuzzleRequest) { r.Clue = strings.Repeat("a", 501) }, playv1.PuzzleInvalidReason_PUZZLE_INVALID_REASON_TEXT},
		{"an empty hint", func(r *playv1.CreatePuzzleRequest) { r.Hints = []string{"ok", " "} }, playv1.PuzzleInvalidReason_PUZZLE_INVALID_REASON_TEXT},
		{"eleven hints", func(r *playv1.CreatePuzzleRequest) { r.Hints = make([]string, 11) }, playv1.PuzzleInvalidReason_PUZZLE_INVALID_REASON_TEXT},
		{"no kind", func(r *playv1.CreatePuzzleRequest) { r.Config = &playv1.PuzzleConfig{} }, playv1.PuzzleInvalidReason_PUZZLE_INVALID_REASON_KIND},
		{"one wheel", func(r *playv1.CreatePuzzleRequest) {
			r.Config, r.Solution, r.Start = lockConfig(1, playv1.PuzzleAlphabet_PUZZLE_ALPHABET_DIGITS), lockSolution(1), lockStart(0)
		}, playv1.PuzzleInvalidReason_PUZZLE_INVALID_REASON_SIZE},
		{"a solution of another kind", func(r *playv1.CreatePuzzleRequest) { r.Solution = pillarsSolution(1, 2, 3) }, playv1.PuzzleInvalidReason_PUZZLE_INVALID_REASON_KIND},
		{"no start", func(r *playv1.CreatePuzzleRequest) { r.Start = nil }, playv1.PuzzleInvalidReason_PUZZLE_INVALID_REASON_KIND},
		{"a position out of the alphabet", func(r *playv1.CreatePuzzleRequest) { r.Solution = lockSolution(1, 2, 26) }, playv1.PuzzleInvalidReason_PUZZLE_INVALID_REASON_SYMBOLS},
		{"a start that is the solution", func(r *playv1.CreatePuzzleRequest) { r.Start = lockStart(1, 2, 3) }, playv1.PuzzleInvalidReason_PUZZLE_INVALID_REASON_START_SOLVED},
		{"a board of 8", func(r *playv1.CreatePuzzleRequest) { r.Config, r.Solution, r.Start = lightsConfig(8), nil, nil }, playv1.PuzzleInvalidReason_PUZZLE_INVALID_REASON_SIZE},
		{"a board of 2", func(r *playv1.CreatePuzzleRequest) { r.Config, r.Solution, r.Start = lightsConfig(2), nil, nil }, playv1.PuzzleInvalidReason_PUZZLE_INVALID_REASON_SIZE},
		{"a pillar linked to itself", func(r *playv1.CreatePuzzleRequest) {
			r.Config, r.Solution, r.Start = pillarsConfig(3, 3, []int32{0}), pillarsSolution(0, 1, 2), nil
		}, playv1.PuzzleInvalidReason_PUZZLE_INVALID_REASON_LINKS},
		{"a mural out of the symbols", func(r *playv1.CreatePuzzleRequest) {
			r.Config, r.Solution, r.Start = pillarsConfig(3, 3), pillarsSolution(0, 1, 3), nil
		}, playv1.PuzzleInvalidReason_PUZZLE_INVALID_REASON_SYMBOLS},
		{"seven symbols", func(r *playv1.CreatePuzzleRequest) {
			r.Config, r.Solution, r.Start = pillarsConfig(3, 7), pillarsSolution(0, 1, 2), nil
		}, playv1.PuzzleInvalidReason_PUZZLE_INVALID_REASON_SIZE},
		{"a door target on a notify", func(r *playv1.CreatePuzzleRequest) {
			r.OnSolve = &playv1.PuzzleOnSolve{Action: playv1.PuzzleSolveAction_PUZZLE_SOLVE_ACTION_NOTIFY, Target: &playv1.PuzzleOnSolve_Clue{Clue: &playv1.PuzzleClueTarget{ClueId: newKey()}}}
		}, playv1.PuzzleInvalidReason_PUZZLE_INVALID_REASON_TARGET},
		{"a door action with no door", func(r *playv1.CreatePuzzleRequest) {
			r.OnSolve = &playv1.PuzzleOnSolve{Action: playv1.PuzzleSolveAction_PUZZLE_SOLVE_ACTION_OPEN_DOOR}
		}, playv1.PuzzleInvalidReason_PUZZLE_INVALID_REASON_TARGET},
		{"a door of no map", func(r *playv1.CreatePuzzleRequest) {
			r.OnSolve = &playv1.PuzzleOnSolve{Action: playv1.PuzzleSolveAction_PUZZLE_SOLVE_ACTION_OPEN_DOOR, Target: &playv1.PuzzleOnSolve_Door{Door: &playv1.PuzzleDoorTarget{MapId: newKey(), Col: 1, Row: 1}}}
		}, playv1.PuzzleInvalidReason_PUZZLE_INVALID_REASON_TARGET},
		{"a door on a square without one", func(r *playv1.CreatePuzzleRequest) {
			r.OnSolve = &playv1.PuzzleOnSolve{Action: playv1.PuzzleSolveAction_PUZZLE_SOLVE_ACTION_OPEN_DOOR, Target: &playv1.PuzzleOnSolve_Door{Door: &playv1.PuzzleDoorTarget{MapId: p.mapID, Col: 1, Row: 1}}}
		}, playv1.PuzzleInvalidReason_PUZZLE_INVALID_REASON_TARGET},
		{"a clue that is not there", func(r *playv1.CreatePuzzleRequest) {
			r.OnSolve = &playv1.PuzzleOnSolve{Action: playv1.PuzzleSolveAction_PUZZLE_SOLVE_ACTION_REVEAL_CLUE, Target: &playv1.PuzzleOnSolve_Clue{Clue: &playv1.PuzzleClueTarget{ClueId: newKey()}}}
		}, playv1.PuzzleInvalidReason_PUZZLE_INVALID_REASON_TARGET},
		{"a point that is not there", func(r *playv1.CreatePuzzleRequest) {
			r.OnSolve = &playv1.PuzzleOnSolve{Action: playv1.PuzzleSolveAction_PUZZLE_SOLVE_ACTION_REVEAL_POINT, Target: &playv1.PuzzleOnSolve_Point{Point: &playv1.PuzzlePointTarget{MapId: p.mapID, PointId: newKey()}}}
		}, playv1.PuzzleInvalidReason_PUZZLE_INVALID_REASON_TARGET},
	} {
		wantPuzzleInvalid(t, tc.name, create(tc.edit), tc.want)
	}
	if list, err := p.pc(p.master).ListPuzzles(ctx, connect.NewRequest(&playv1.ListPuzzlesRequest{CampaignId: p.campaignID})); err != nil || len(list.Msg.GetPuzzles()) != 0 {
		t.Errorf("a refused puzzle was stored: %v, %v", list, err)
	}
}

// The preview and the create agree: the same seed gives the same start.
func TestMR038_ASeedGivesTheSameStart(t *testing.T) {
	t.Parallel()
	p := newPuzzleTable(t)
	ctx := t.Context()
	pre, err := p.pc(p.master).PreviewPuzzleStart(ctx, connect.NewRequest(&playv1.PreviewPuzzleStartRequest{CampaignId: p.campaignID, Config: lightsConfig(6)}))
	if err != nil || pre.Msg.GetSeed() <= 0 || litCount(pre.Msg.GetStart()) == 0 || !pre.Msg.GetMinimum().GetSolvable() {
		t.Fatalf("PreviewPuzzleStart() = %v, %v", pre, err)
	}
	again, _ := p.pc(p.master).PreviewPuzzleStart(ctx, connect.NewRequest(&playv1.PreviewPuzzleStartRequest{CampaignId: p.campaignID, Config: lightsConfig(6), Seed: pre.Msg.GetSeed()}))
	if !proto.Equal(again.Msg.GetStart(), pre.Msg.GetStart()) {
		t.Errorf("the same seed gave two starts")
	}
	made := p.lights(t, "Seis", 6, func(r *playv1.CreatePuzzleRequest) { r.Seed = pre.Msg.GetSeed() })
	if !proto.Equal(made.GetStart(), pre.Msg.GetStart()) || made.GetMinimum().GetMoves() != pre.Msg.GetMinimum().GetMoves() {
		t.Errorf("CreatePuzzle with the seed gave another start")
	}
	_, err = p.pc(p.master).PreviewPuzzleStart(ctx, connect.NewRequest(&playv1.PreviewPuzzleStartRequest{CampaignId: p.campaignID, Config: lockConfig(3, playv1.PuzzleAlphabet_PUZZLE_ALPHABET_DIGITS)}))
	wantPuzzleBlocked(t, "PreviewPuzzleStart of a lock", err, playv1.PuzzleBlockedReason_PUZZLE_BLOCKED_REASON_NO_GENERATED_START)
}

// A puzzle is edited until it is first shown, and archived, never deleted.
func TestMR038_EditUntilShownAndArchive(t *testing.T) {
	t.Parallel()
	p := newPuzzleTable(t)
	ctx := t.Context()
	lock := p.lock(t, "O cofre")
	update := func(edit func(*playv1.UpdatePuzzleRequest)) (*playv1.UpdatePuzzleResponse, error) {
		req := &playv1.UpdatePuzzleRequest{
			CampaignId: p.campaignID, PuzzleId: lock.GetId(), Name: "O cofre velho", Config: lock.GetConfig(), Solution: lockSolution(1, 1, 1, 1),
			Start: lockStart(0, 0, 0, 0), Clue: "outra pista", Hints: []string{"uma"},
		}
		edit(req)
		res, err := p.pc(p.master).UpdatePuzzle(ctx, connect.NewRequest(req))
		if err != nil {
			return nil, err
		}
		return res.Msg, nil
	}
	res, err := update(func(*playv1.UpdatePuzzleRequest) {})
	if err != nil || res.GetPuzzle().GetName() != "O cofre velho" || res.GetPuzzle().GetClue() != "outra pista" || len(res.GetPuzzle().GetHints()) != 1 {
		t.Fatalf("UpdatePuzzle() = %v, %v", res, err)
	}
	_, err = update(func(r *playv1.UpdatePuzzleRequest) {
		r.Config = lightsConfig(4) // another kind
	})
	wantPuzzleInvalid(t, "UpdatePuzzle to another kind", err, playv1.PuzzleInvalidReason_PUZZLE_INVALID_REASON_KIND)
	_, err = update(func(r *playv1.UpdatePuzzleRequest) { r.Name = "" })
	wantPuzzleInvalid(t, "UpdatePuzzle with no name", err, playv1.PuzzleInvalidReason_PUZZLE_INVALID_REASON_NAME)

	// A lights puzzle keeps its start on an edit of the texts (seed 0), and draws
	// another when the board changes.
	lights := p.lights(t, "Luzes", 5)
	edited, err := p.pc(p.master).UpdatePuzzle(ctx, connect.NewRequest(&playv1.UpdatePuzzleRequest{CampaignId: p.campaignID, PuzzleId: lights.GetId(), Name: "Luzes 2", Config: lights.GetConfig(), Clue: "x"}))
	if err != nil || !proto.Equal(edited.Msg.GetPuzzle().GetStart(), lights.GetStart()) {
		t.Fatalf("an edit of the texts changed the start: %v, %v", edited, err)
	}
	bigger, err := p.pc(p.master).UpdatePuzzle(ctx, connect.NewRequest(&playv1.UpdatePuzzleRequest{CampaignId: p.campaignID, PuzzleId: lights.GetId(), Name: "Luzes 2", Config: lightsConfig(7)}))
	if err != nil || len(bigger.Msg.GetPuzzle().GetStart().GetLights().GetLit()) != 49 {
		t.Fatalf("a new board gave %v, %v; want a 7 x 7 start", bigger, err)
	}

	// Archived: out of the list, cannot be shown, comes back.
	arch, err := p.pc(p.master).ArchivePuzzle(ctx, connect.NewRequest(&playv1.ArchivePuzzleRequest{CampaignId: p.campaignID, PuzzleId: lock.GetId()}))
	if err != nil || !arch.Msg.GetPuzzle().GetArchived() {
		t.Fatalf("ArchivePuzzle() = %v, %v", arch, err)
	}
	if again, err := p.pc(p.master).ArchivePuzzle(ctx, connect.NewRequest(&playv1.ArchivePuzzleRequest{CampaignId: p.campaignID, PuzzleId: lock.GetId()})); err != nil || !again.Msg.GetPuzzle().GetArchived() {
		t.Errorf("archiving twice = %v, %v; want it to stay archived", again, err)
	}
	list, _ := p.pc(p.master).ListPuzzles(ctx, connect.NewRequest(&playv1.ListPuzzlesRequest{CampaignId: p.campaignID}))
	if len(list.Msg.GetPuzzles()) != 1 {
		t.Errorf("the list has %d puzzles, want 1 (the archived one is out)", len(list.Msg.GetPuzzles()))
	}
	list, _ = p.pc(p.master).ListPuzzles(ctx, connect.NewRequest(&playv1.ListPuzzlesRequest{CampaignId: p.campaignID, IncludeArchived: true}))
	if len(list.Msg.GetPuzzles()) != 2 {
		t.Errorf("the list with the archived has %d puzzles, want 2", len(list.Msg.GetPuzzles()))
	}
	_, err = p.pc(p.master).ShowPuzzle(ctx, connect.NewRequest(&playv1.ShowPuzzleRequest{CampaignId: p.campaignID, PuzzleId: lock.GetId()}))
	wantPuzzleBlocked(t, "ShowPuzzle of an archived puzzle", err, playv1.PuzzleBlockedReason_PUZZLE_BLOCKED_REASON_ARCHIVED)
	back, err := p.pc(p.master).UnarchivePuzzle(ctx, connect.NewRequest(&playv1.UnarchivePuzzleRequest{CampaignId: p.campaignID, PuzzleId: lock.GetId()}))
	if err != nil || back.Msg.GetPuzzle().GetArchived() {
		t.Fatalf("UnarchivePuzzle() = %v, %v", back, err)
	}

	// Shown once, never edited again.
	p.show(t, lock.GetId())
	_, err = update(func(*playv1.UpdatePuzzleRequest) {})
	wantPuzzleBlocked(t, "UpdatePuzzle of a shown puzzle", err, playv1.PuzzleBlockedReason_PUZZLE_BLOCKED_REASON_ALREADY_SHOWN)
	got, _ := p.pc(p.master).GetPuzzle(ctx, connect.NewRequest(&playv1.GetPuzzleRequest{CampaignId: p.campaignID, PuzzleId: lock.GetId()}))
	if !got.Msg.GetPuzzle().GetShown() {
		t.Errorf("the puzzle does not say it was shown")
	}
}

// MR-038: shown to every player, solved live by the shortest way, frozen when solved.
func TestMR038_ShowPlayAndSolveEachKind(t *testing.T) {
	t.Parallel()
	p := newPuzzleTable(t)
	ctx := t.Context()
	lights := p.lights(t, "O selo da Capela", 5)
	lock := p.lock(t, "O cofre do Refeitório")
	pillars := p.pillars(t, "Os pilares da Galeria")

	// Nothing is shown yet: the players see nothing.
	for _, u := range []*user{p.caio, p.ana, p.bia} {
		res, err := p.pc(u).ListShownPuzzles(ctx, connect.NewRequest(&playv1.ListShownPuzzlesRequest{CampaignId: p.campaignID}))
		if err != nil || len(res.Msg.GetPuzzles()) != 0 {
			t.Fatalf("ListShownPuzzles() = %v, %v; want none", res, err)
		}
		_, err = p.pc(u).GetPuzzleRun(ctx, connect.NewRequest(&playv1.GetPuzzleRunRequest{CampaignId: p.campaignID, PuzzleId: lights.GetId()}))
		wantCode(t, "GetPuzzleRun of a puzzle not shown", err, connect.CodeNotFound)
		_, err = p.move(t, u, lights.GetId(), lightsMove(0, 0))
		wantCode(t, "MakePuzzleMove of a puzzle not shown", err, connect.CodeNotFound)
	}
	menu, err := p.pc(p.master).ListSessionPuzzles(ctx, connect.NewRequest(&playv1.ListSessionPuzzlesRequest{CampaignId: p.campaignID}))
	if err != nil || len(menu.Msg.GetPuzzles()) != 3 || menu.Msg.GetPuzzles()[0].GetStatus() != playv1.PuzzleRunStatus_PUZZLE_RUN_STATUS_NOT_SHOWN {
		t.Fatalf("ListSessionPuzzles() = %v, %v", menu, err)
	}

	for _, tc := range []struct {
		name string
		puz  *playv1.Puzzle
		who  *user
	}{{"lights", lights, p.caio}, {"lock", lock, p.ana}, {"pillars", pillars, p.bia}} {
		shown := p.show(t, tc.puz.GetId())
		if shown.GetStatus() != playv1.PuzzleRunStatus_PUZZLE_RUN_STATUS_SHOWN || !proto.Equal(shown.GetRun().GetState(), tc.puz.GetStart()) {
			t.Fatalf("%s: shown = %v; want shown at the puzzle's start", tc.name, shown)
		}
		// Every player sees it, in the same state.
		for _, u := range []*user{p.caio, p.ana, p.bia} {
			if r := p.read(t, u, tc.puz.GetId()); !proto.Equal(r.GetState(), tc.puz.GetStart()) || r.GetClue() != tc.puz.GetClue() || r.GetSolved() {
				t.Errorf("%s: a player reads %v", tc.name, r)
			}
		}
		// Showing a shown puzzle changes nothing.
		before := p.masterRun(t, tc.puz.GetId()).GetRun().GetRevision()
		if again := p.show(t, tc.puz.GetId()); again.GetRun().GetRevision() != before {
			t.Errorf("%s: showing it again moved the revision %d -> %d", tc.name, before, again.GetRun().GetRevision())
		}

		// One move, then the shortest way from there.
		first := p.masterRun(t, tc.puz.GetId()).GetMinimum().GetPath()[0]
		res := p.mustMove(t, tc.who, tc.puz.GetId(), first)
		who := map[*user]string{p.caio: "Toren", p.ana: "Pensantus", p.bia: "Brisa"}[tc.who]
		if res.GetRun().GetLastMove().GetCharacterName() != who || res.GetRun().GetSolved() == (tc.puz.GetMinimum().GetMoves() > 1) {
			t.Errorf("%s: after one move: %v; want %s as the last mover", tc.name, res.GetRun(), who)
		}
		m := p.masterRun(t, tc.puz.GetId())
		if m.GetMovesMade() != 1 || m.GetMinimum().GetMoves() != tc.puz.GetMinimum().GetMoves()-1 && tc.name != "lights" {
			t.Errorf("%s: moves made %d, minimum now %d (from %d)", tc.name, m.GetMovesMade(), m.GetMinimum().GetMoves(), tc.puz.GetMinimum().GetMoves())
		}
		var last *playv1.MakePuzzleMoveResponse
		if !res.GetRun().GetSolved() {
			last = p.solve(t, tc.who, tc.puz.GetId())
		} else {
			last = res
		}
		if !last.GetRun().GetSolved() || !last.GetSolvedByThisMove() || last.GetRun().GetSolvedByName() != who {
			t.Fatalf("%s: the last move = %v; want it solved by %s", tc.name, last, who)
		}
		if tc.name == "lights" && litCount(last.GetRun().GetState()) != 0 {
			t.Errorf("lights: %d lights still lit", litCount(last.GetRun().GetState()))
		}
		// The master is told: solved, with who solved it.
		m = p.masterRun(t, tc.puz.GetId())
		if m.GetStatus() != playv1.PuzzleRunStatus_PUZZLE_RUN_STATUS_SOLVED || m.GetRun().GetSolvedByName() != who || m.GetOutcome() != playv1.PuzzleSolveOutcome_PUZZLE_SOLVE_OUTCOME_NOTIFIED {
			t.Errorf("%s: the master reads %v", tc.name, m)
		}
		// Solved freezes the run: a later move, by anyone, is refused with its reason.
		for _, u := range []*user{tc.who, p.caio} {
			_, err := p.move(t, u, tc.puz.GetId(), tc.puz.GetMinimum().GetPath()[0])
			wantPuzzleBlocked(t, tc.name+": a move after solved", err, playv1.PuzzleBlockedReason_PUZZLE_BLOCKED_REASON_SOLVED)
		}
		if after := p.read(t, p.bia, tc.puz.GetId()); !proto.Equal(after.GetState(), last.GetRun().GetState()) || !after.GetSolved() {
			t.Errorf("%s: the state moved after it was solved", tc.name)
		}
	}
	// The players' list says which are solved.
	shown, _ := p.pc(p.caio).ListShownPuzzles(ctx, connect.NewRequest(&playv1.ListShownPuzzlesRequest{CampaignId: p.campaignID}))
	if len(shown.Msg.GetPuzzles()) != 3 || !shown.Msg.GetPuzzles()[0].GetSolved() {
		t.Errorf("ListShownPuzzles() = %v", shown.Msg)
	}
	// The master cannot play.
	_, err = p.pc(p.master).MakePuzzleMove(ctx, connect.NewRequest(&playv1.MakePuzzleMoveRequest{CampaignId: p.campaignID, PuzzleId: lock.GetId(), Move: lockMove(0, 1), IdempotencyKey: newKey()}))
	wantCode(t, "MakePuzzleMove as the master", err, connect.CodePermissionDenied)
}

// Moves that are not valid, and a player with no character.
func TestMR038_BadMoves(t *testing.T) {
	t.Parallel()
	p := newPuzzleTable(t)
	lock := p.lock(t, "Cofre")
	lights := p.lights(t, "Luzes", 4)
	pillars := p.pillars(t, "Pilares")
	for _, id := range []string{lock.GetId(), lights.GetId(), pillars.GetId()} {
		p.show(t, id)
	}
	before := p.read(t, p.caio, lock.GetId())
	for _, tc := range []struct {
		name string
		id   string
		mv   *playv1.PuzzleMove
		want playv1.PuzzleInvalidReason
	}{
		{"a wheel that is not there", lock.GetId(), lockMove(4, 1), playv1.PuzzleInvalidReason_PUZZLE_INVALID_REASON_MOVE},
		{"a turn by two", lock.GetId(), lockMove(0, 2), playv1.PuzzleInvalidReason_PUZZLE_INVALID_REASON_MOVE},
		{"a turn by nothing", lock.GetId(), lockMove(0, 0), playv1.PuzzleInvalidReason_PUZZLE_INVALID_REASON_MOVE},
		{"a move of another kind", lock.GetId(), lightsMove(0, 0), playv1.PuzzleInvalidReason_PUZZLE_INVALID_REASON_KIND},
		{"no move at all", lock.GetId(), &playv1.PuzzleMove{}, playv1.PuzzleInvalidReason_PUZZLE_INVALID_REASON_KIND},
		{"a cell off the board", lights.GetId(), lightsMove(4, 0), playv1.PuzzleInvalidReason_PUZZLE_INVALID_REASON_MOVE},
		{"a negative cell", lights.GetId(), lightsMove(0, -1), playv1.PuzzleInvalidReason_PUZZLE_INVALID_REASON_MOVE},
		{"a pillar that is not there", pillars.GetId(), &playv1.PuzzleMove{Kind: &playv1.PuzzleMove_Pillars{Pillars: &playv1.PillarsMove{Pillar: 4, Delta: 1}}}, playv1.PuzzleInvalidReason_PUZZLE_INVALID_REASON_MOVE},
	} {
		_, err := p.move(t, p.caio, tc.id, tc.mv)
		wantPuzzleInvalid(t, tc.name, err, tc.want)
	}
	if after := p.read(t, p.caio, lock.GetId()); after.GetRevision() != before.GetRevision() {
		t.Errorf("a refused move changed the run")
	}
	_, err := p.moveKey(t, p.caio, lock.GetId(), lockMove(0, 1), "not-a-uuid")
	wantCode(t, "MakePuzzleMove with a bad key", err, connect.CodeInvalidArgument)
	_, err = p.move(t, p.caio, "nope", lockMove(0, 1))
	wantCode(t, "MakePuzzleMove of a bad ID", err, connect.CodeNotFound)

	// A player with no living character has nothing to move as.
	late := p.h.newUser("Lia")
	p.h.join(p.master, p.campaignID, late)
	_, err = p.move(t, late, lock.GetId(), lockMove(0, 1))
	wantPuzzleBlocked(t, "a move with no character", err, playv1.PuzzleBlockedReason_PUZZLE_BLOCKED_REASON_NO_CHARACTER)
	// ... though they read it like the others.
	if r := p.read(t, late, lock.GetId()); r.GetName() != "Cofre" {
		t.Errorf("a member without a character reads %v", r)
	}
}

// No session open: the session's calls say so.
func TestMR038_NoOpenSession(t *testing.T) {
	t.Parallel()
	p := newPuzzleTable(t)
	lock := p.lock(t, "Cofre")
	session := p.show(t, lock.GetId())
	_ = session
	sessions, err := p.master.play.ListGameSessions(t.Context(), connect.NewRequest(&playv1.ListGameSessionsRequest{CampaignId: p.campaignID}))
	if err != nil || len(sessions.Msg.GetGameSessions()) != 1 {
		t.Fatalf("ListGameSessions() = %v, %v", sessions, err)
	}
	p.master.end(t, sessions.Msg.GetGameSessions()[0])
	ctx := t.Context()
	_, err = p.pc(p.master).ShowPuzzle(ctx, connect.NewRequest(&playv1.ShowPuzzleRequest{CampaignId: p.campaignID, PuzzleId: lock.GetId()}))
	wantPuzzleBlocked(t, "ShowPuzzle", err, playv1.PuzzleBlockedReason_PUZZLE_BLOCKED_REASON_NO_OPEN_SESSION)
	_, err = p.pc(p.master).GetMasterPuzzleRun(ctx, connect.NewRequest(&playv1.GetMasterPuzzleRunRequest{CampaignId: p.campaignID, PuzzleId: lock.GetId()}))
	wantPuzzleBlocked(t, "GetMasterPuzzleRun", err, playv1.PuzzleBlockedReason_PUZZLE_BLOCKED_REASON_NO_OPEN_SESSION)
	_, err = p.pc(p.caio).GetPuzzleRun(ctx, connect.NewRequest(&playv1.GetPuzzleRunRequest{CampaignId: p.campaignID, PuzzleId: lock.GetId()}))
	wantPuzzleBlocked(t, "GetPuzzleRun", err, playv1.PuzzleBlockedReason_PUZZLE_BLOCKED_REASON_NO_OPEN_SESSION)
	_, err = p.move(t, p.caio, lock.GetId(), lockMove(0, 1))
	wantPuzzleBlocked(t, "MakePuzzleMove", err, playv1.PuzzleBlockedReason_PUZZLE_BLOCKED_REASON_NO_OPEN_SESSION)
	// The puzzle itself stays: the next session starts it again from its start.
	p.master.start(t, p.campaignID)
	if m := p.masterRun(t, lock.GetId()); m.GetStatus() != playv1.PuzzleRunStatus_PUZZLE_RUN_STATUS_NOT_SHOWN {
		t.Errorf("in the next session the puzzle is %v, want not shown", m.GetStatus())
	}
}

// MR-038: two players move at once and both moves count.
func TestMR038_TwoPlayersMovingAtOnce(t *testing.T) {
	t.Parallel()
	dbtest.PoolSize(t, 4)
	p := newPuzzleTable(t)
	lock := p.lock(t, "Cofre", func(r *playv1.CreatePuzzleRequest) {
		r.Config = lockConfig(6, playv1.PuzzleAlphabet_PUZZLE_ALPHABET_DIGITS)
		r.Solution, r.Start = lockSolution(9, 9, 9, 9, 9, 9), lockStart(0, 0, 0, 0, 0, 0)
	})
	p.show(t, lock.GetId())

	// Toren turns wheels 0 and 1 up three times each; Pensantus wheel 0 up three
	// times and wheel 2 down three times. Wheel 0 gets six turns from the two.
	moves := map[*user][]*playv1.PuzzleMove{
		p.caio: {lockMove(0, 1), lockMove(0, 1), lockMove(0, 1), lockMove(1, 1), lockMove(1, 1), lockMove(1, 1)},
		p.ana:  {lockMove(0, 1), lockMove(0, 1), lockMove(0, 1), lockMove(2, -1), lockMove(2, -1), lockMove(2, -1)},
	}
	var wg sync.WaitGroup
	var failed atomic.Int32
	for u, list := range moves {
		wg.Go(func() {
			for _, mv := range list {
				if _, err := p.move(t, u, lock.GetId(), mv); err != nil {
					t.Errorf("MakePuzzleMove() error = %v", err)
					failed.Add(1)
				}
			}
		})
	}
	wg.Wait()
	if failed.Load() != 0 {
		t.FailNow()
	}
	got := p.read(t, p.bia, lock.GetId())
	want := []int32{6, 3, 7, 0, 0, 0}
	if w := got.GetState().GetLock().GetWheels(); len(w) != 6 || w[0] != want[0] || w[1] != want[1] || w[2] != want[2] || w[3] != 0 {
		t.Errorf("the wheels are %v, want %v: every move counts", w, want)
	}
	if m := p.masterRun(t, lock.GetId()); m.GetMovesMade() != 12 || m.GetRun().GetRevision() != 13 {
		t.Errorf("moves made %d, revision %d; want 12 and 13", m.GetMovesMade(), m.GetRun().GetRevision())
	}
	var rows, distinct int
	if err := p.h.pool.QueryRow(t.Context(), `SELECT count(*), count(DISTINCT seq) FROM puzzle_moves`).Scan(&rows, &distinct); err != nil || rows != 12 || distinct != 12 {
		t.Errorf("puzzle_moves has %d rows with %d distinct numbers (%v), want 12 and 12", rows, distinct, err)
	}
}

// Two moves that solve the puzzle at once: one wins, and the other is refused as
// "solved", never applied.
func TestMR038_TwoMovesThatSolveAtOnce(t *testing.T) {
	t.Parallel()
	dbtest.PoolSize(t, 4)
	p := newPuzzleTable(t)
	lock := p.lock(t, "Cofre", func(r *playv1.CreatePuzzleRequest) {
		r.Solution, r.Start = lockSolution(1, 0, 0, 0), lockStart(0, 0, 0, 0)
	})
	p.show(t, lock.GetId())
	var wg sync.WaitGroup
	results := make([]error, 2)
	solvers := make([]bool, 2)
	for i, u := range []*user{p.caio, p.ana} {
		wg.Go(func() {
			res, err := p.move(t, u, lock.GetId(), lockMove(0, 1))
			results[i] = err
			solvers[i] = err == nil && res.GetSolvedByThisMove()
		})
	}
	wg.Wait()
	wins := 0
	for i := range results {
		switch {
		case results[i] == nil && solvers[i]:
			wins++
		case results[i] == nil:
			// The second applied before the first? Not possible: the first solved it.
			t.Errorf("a move was applied without solving: %d", i)
		default:
			wantPuzzleBlocked(t, "the losing move", results[i], playv1.PuzzleBlockedReason_PUZZLE_BLOCKED_REASON_SOLVED)
		}
	}
	if wins != 1 {
		t.Errorf("%d moves solved it, want exactly 1", wins)
	}
	if m := p.masterRun(t, lock.GetId()); m.GetMovesMade() != 1 {
		t.Errorf("%d moves were made, want 1", m.GetMovesMade())
	}
}

// A move is made once however many times it is sent.
func TestMR038_AMoveWithTheSameKeyIsMadeOnce(t *testing.T) {
	t.Parallel()
	p := newPuzzleTable(t)
	lock := p.lock(t, "Cofre")
	p.show(t, lock.GetId())
	key := newKey()
	first, err := p.moveKey(t, p.caio, lock.GetId(), lockMove(0, 1), key)
	if err != nil || first.GetReplayed() {
		t.Fatalf("first move = %v, %v", first, err)
	}
	// Another player moves in between; the replay does not undo or redo anything.
	p.mustMove(t, p.ana, lock.GetId(), lockMove(1, 1))
	again, err := p.moveKey(t, p.caio, lock.GetId(), lockMove(0, 1), key)
	if err != nil || !again.GetReplayed() {
		t.Fatalf("replay = %v, %v; want replayed", again, err)
	}
	if w := again.GetRun().GetState().GetLock().GetWheels(); w[0] != 1 || w[1] != 1 {
		t.Errorf("the replay changed the wheels: %v", w)
	}
	if m := p.masterRun(t, lock.GetId()); m.GetMovesMade() != 2 {
		t.Errorf("moves made = %d, want 2", m.GetMovesMade())
	}
	// Another player's key is another change: the key cannot be borrowed.
	_, err = p.moveKey(t, p.ana, lock.GetId(), lockMove(0, 1), key)
	wantCode(t, "a key used by another player", err, connect.CodeInvalidArgument)
}

// A move's key stands for that move: the same key with another move is refused, and changes nothing.
func TestMR038_AMoveKeyReusedForAnotherMoveIsRefused(t *testing.T) {
	t.Parallel()
	p := newPuzzleTable(t)
	lights := p.lights(t, "O selo da Capela", 5)
	p.show(t, lights.GetId())
	key := newKey()
	if _, err := p.moveKey(t, p.caio, lights.GetId(), lightsMove(2, 2), key); err != nil {
		t.Fatalf("MakePuzzleMove() error = %v", err)
	}
	if again, err := p.moveKey(t, p.caio, lights.GetId(), lightsMove(2, 2), key); err != nil || !again.GetReplayed() {
		t.Fatalf("the retry = %v, %v; want the first answer", again, err)
	}
	_, err := p.moveKey(t, p.caio, lights.GetId(), lightsMove(0, 0), key)
	wantCode(t, "the key with another move", err, connect.CodeInvalidArgument)
	if m := p.masterRun(t, lights.GetId()); m.GetMovesMade() != 1 {
		t.Errorf("moves made = %d, want 1", m.GetMovesMade())
	}
}

// The replay of the move that solved the puzzle still answers "solved by this move".
func TestMR038_AReplayOfTheWinningMove(t *testing.T) {
	t.Parallel()
	p := newPuzzleTable(t)
	lock := p.lock(t, "Cofre", func(r *playv1.CreatePuzzleRequest) {
		r.Solution, r.Start = lockSolution(1, 0, 0, 0), lockStart(0, 0, 0, 0)
	})
	p.show(t, lock.GetId())
	key := newKey()
	first, err := p.moveKey(t, p.caio, lock.GetId(), lockMove(0, 1), key)
	if err != nil || !first.GetSolvedByThisMove() {
		t.Fatalf("winning move = %v, %v", first, err)
	}
	again, err := p.moveKey(t, p.caio, lock.GetId(), lockMove(0, 1), key)
	if err != nil || !again.GetReplayed() || !again.GetSolvedByThisMove() || !again.GetRun().GetSolved() {
		t.Fatalf("replay of the winning move = %v, %v", again, err)
	}
	var events int
	if err := p.h.pool.QueryRow(t.Context(), `SELECT count(*) FROM session_events WHERE kind = 'puzzle_solved'`).Scan(&events); err != nil || events != 1 {
		t.Errorf("%d puzzle_solved events (%v), want 1", events, err)
	}
}

// "Recomeçar" returns to the same start; "Gerar outro começo" draws another; the hints stay.
func TestMR038_ResetReseedAndClose(t *testing.T) {
	t.Parallel()
	p := newPuzzleTable(t)
	var seeds atomic.Int64
	p.h.svc.puzzles.seed = func() uint64 { return uint64(seeds.Add(1)) * 7919 } //nolint:gosec // a counter
	ctx := t.Context()
	lights := p.lights(t, "O selo da Capela", 5, func(r *playv1.CreatePuzzleRequest) { r.Hints = []string{"a", "b"} })
	lock := p.lock(t, "Cofre")
	p.show(t, lights.GetId())
	p.show(t, lock.GetId())

	p.mustMove(t, p.caio, lights.GetId(), lightsMove(2, 2))
	p.mustMove(t, p.ana, lights.GetId(), lightsMove(0, 0))
	if _, err := p.pc(p.master).ReleaseNextPuzzleHint(ctx, connect.NewRequest(&playv1.ReleaseNextPuzzleHintRequest{CampaignId: p.campaignID, PuzzleId: lights.GetId()})); err != nil {
		t.Fatalf("ReleaseNextPuzzleHint() error = %v", err)
	}
	// Recomeçar: the same start, for all; the last move is forgotten, the hint stays, the moves stay in the history.
	res, err := p.pc(p.master).ResetPuzzle(ctx, connect.NewRequest(&playv1.ResetPuzzleRequest{CampaignId: p.campaignID, PuzzleId: lights.GetId()}))
	if err != nil {
		t.Fatalf("ResetPuzzle() error = %v", err)
	}
	r := res.Msg.GetRun()
	if !proto.Equal(r.GetRun().GetState(), lights.GetStart()) || r.GetRun().GetLastMove() != nil || len(r.GetRun().GetHints()) != 1 || r.GetMovesMade() != 2 {
		t.Errorf("after Recomeçar: %v", r)
	}
	if got := p.read(t, p.bia, lights.GetId()); !proto.Equal(got.GetState(), lights.GetStart()) {
		t.Errorf("a player does not see the same start again")
	}

	// Gerar outro começo: a new seed and start, not solved, with the fewest moves worked out again.
	re, err := p.pc(p.master).ReseedPuzzle(ctx, connect.NewRequest(&playv1.ReseedPuzzleRequest{CampaignId: p.campaignID, PuzzleId: lights.GetId()}))
	if err != nil {
		t.Fatalf("ReseedPuzzle() error = %v", err)
	}
	other := re.Msg.GetRun()
	if proto.Equal(other.GetRun().GetState(), lights.GetStart()) || litCount(other.GetRun().GetState()) == 0 || !other.GetMinimum().GetSolvable() || !proto.Equal(other.GetStart(), other.GetRun().GetState()) {
		t.Errorf("after Gerar outro começo: %v", other)
	}
	if !proto.Equal(other.GetPuzzle().GetStart(), lights.GetStart()) {
		t.Errorf("a new start for the run changed the puzzle's own")
	}
	// Recomeçar now returns to the new start.
	p.mustMove(t, p.caio, lights.GetId(), lightsMove(1, 1))
	res, _ = p.pc(p.master).ResetPuzzle(ctx, connect.NewRequest(&playv1.ResetPuzzleRequest{CampaignId: p.campaignID, PuzzleId: lights.GetId()}))
	if !proto.Equal(res.Msg.GetRun().GetRun().GetState(), other.GetRun().GetState()) {
		t.Errorf("Recomeçar did not return to the run's own start")
	}
	// A lock has no generated start.
	_, err = p.pc(p.master).ReseedPuzzle(ctx, connect.NewRequest(&playv1.ReseedPuzzleRequest{CampaignId: p.campaignID, PuzzleId: lock.GetId()}))
	wantPuzzleBlocked(t, "ReseedPuzzle of a lock", err, playv1.PuzzleBlockedReason_PUZZLE_BLOCKED_REASON_NO_GENERATED_START)

	// Reset after solved: playable again.
	p.solve(t, p.caio, lights.GetId())
	if m := p.masterRun(t, lights.GetId()); m.GetStatus() != playv1.PuzzleRunStatus_PUZZLE_RUN_STATUS_SOLVED {
		t.Fatalf("not solved: %v", m.GetStatus())
	}
	res, _ = p.pc(p.master).ResetPuzzle(ctx, connect.NewRequest(&playv1.ResetPuzzleRequest{CampaignId: p.campaignID, PuzzleId: lights.GetId()}))
	if res.Msg.GetRun().GetStatus() != playv1.PuzzleRunStatus_PUZZLE_RUN_STATUS_SHOWN || res.Msg.GetRun().GetRun().GetSolved() {
		t.Errorf("a reset puzzle is still solved: %v", res.Msg.GetRun())
	}
	p.mustMove(t, p.ana, lights.GetId(), lightsMove(0, 1))

	// Fechar: the players stop seeing it; showing it again starts it over.
	closed, err := p.pc(p.master).ClosePuzzle(ctx, connect.NewRequest(&playv1.ClosePuzzleRequest{CampaignId: p.campaignID, PuzzleId: lights.GetId()}))
	if err != nil || closed.Msg.GetRun().GetStatus() != playv1.PuzzleRunStatus_PUZZLE_RUN_STATUS_CLOSED {
		t.Fatalf("ClosePuzzle() = %v, %v", closed, err)
	}
	_, err = p.pc(p.ana).GetPuzzleRun(ctx, connect.NewRequest(&playv1.GetPuzzleRunRequest{CampaignId: p.campaignID, PuzzleId: lights.GetId()}))
	wantCode(t, "GetPuzzleRun of a closed puzzle", err, connect.CodeNotFound)
	_, err = p.move(t, p.ana, lights.GetId(), lightsMove(0, 1))
	wantCode(t, "MakePuzzleMove of a closed puzzle", err, connect.CodeNotFound)
	shown, _ := p.pc(p.ana).ListShownPuzzles(ctx, connect.NewRequest(&playv1.ListShownPuzzlesRequest{CampaignId: p.campaignID}))
	if len(shown.Msg.GetPuzzles()) != 1 || shown.Msg.GetPuzzles()[0].GetPuzzleId() != lock.GetId() {
		t.Errorf("the players' list after Fechar = %v, want only the lock", shown.Msg)
	}
	if again, err := p.pc(p.master).ClosePuzzle(ctx, connect.NewRequest(&playv1.ClosePuzzleRequest{CampaignId: p.campaignID, PuzzleId: lights.GetId()})); err != nil || again.Msg.GetRun().GetRun().GetRevision() != closed.Msg.GetRun().GetRun().GetRevision() {
		t.Errorf("closing a closed puzzle = %v, %v; want no change", again, err)
	}
	_, err = p.pc(p.master).ResetPuzzle(ctx, connect.NewRequest(&playv1.ResetPuzzleRequest{CampaignId: p.campaignID, PuzzleId: lights.GetId()}))
	wantPuzzleBlocked(t, "ResetPuzzle of a closed puzzle", err, playv1.PuzzleBlockedReason_PUZZLE_BLOCKED_REASON_NOT_SHOWN)
	back := p.show(t, lights.GetId())
	if back.GetStatus() != playv1.PuzzleRunStatus_PUZZLE_RUN_STATUS_SHOWN || !proto.Equal(back.GetRun().GetState(), other.GetRun().GetState()) {
		t.Errorf("shown again = %v; want it at its start", back)
	}
	// A puzzle never shown cannot be closed, reset or given a hint.
	fresh := p.lock(t, "Outro")
	for name, call := range map[string]func() error{
		"close": func() error {
			_, err := p.pc(p.master).ClosePuzzle(ctx, connect.NewRequest(&playv1.ClosePuzzleRequest{CampaignId: p.campaignID, PuzzleId: fresh.GetId()}))
			return err
		},
		"reset": func() error {
			_, err := p.pc(p.master).ResetPuzzle(ctx, connect.NewRequest(&playv1.ResetPuzzleRequest{CampaignId: p.campaignID, PuzzleId: fresh.GetId()}))
			return err
		},
		"hint": func() error {
			_, err := p.pc(p.master).ReleaseNextPuzzleHint(ctx, connect.NewRequest(&playv1.ReleaseNextPuzzleHintRequest{CampaignId: p.campaignID, PuzzleId: fresh.GetId()}))
			return err
		},
	} {
		wantPuzzleBlocked(t, name+" of a puzzle never shown", call(), playv1.PuzzleBlockedReason_PUZZLE_BLOCKED_REASON_NOT_SHOWN)
	}
}

// "Gerar outro começo" before the puzzle is shown prepares the run; the players see nothing.
func TestMR038_ReseedBeforeShowing(t *testing.T) {
	t.Parallel()
	p := newPuzzleTable(t)
	var seeds atomic.Int64
	p.h.svc.puzzles.seed = func() uint64 { return uint64(seeds.Add(1)) * 104729 } //nolint:gosec // a counter
	lights := p.lights(t, "Luzes", 4)
	re, err := p.pc(p.master).ReseedPuzzle(t.Context(), connect.NewRequest(&playv1.ReseedPuzzleRequest{CampaignId: p.campaignID, PuzzleId: lights.GetId()}))
	if err != nil || re.Msg.GetRun().GetStatus() != playv1.PuzzleRunStatus_PUZZLE_RUN_STATUS_NOT_SHOWN || re.Msg.GetRun().GetRun() != nil {
		t.Fatalf("ReseedPuzzle before showing = %v, %v; want a prepared run the players do not see", re, err)
	}
	_, err = p.pc(p.caio).GetPuzzleRun(t.Context(), connect.NewRequest(&playv1.GetPuzzleRunRequest{CampaignId: p.campaignID, PuzzleId: lights.GetId()}))
	wantCode(t, "GetPuzzleRun of a prepared puzzle", err, connect.CodeNotFound)
	shown := p.show(t, lights.GetId())
	if !proto.Equal(shown.GetRun().GetState(), re.Msg.GetRun().GetStart()) {
		t.Errorf("shown at %v, want the prepared start", shown.GetRun().GetState())
	}
}

// Hints are released one by one; a player never reads the ones not released.
func TestMR038_HintsAreReleasedOneByOne(t *testing.T) {
	t.Parallel()
	p := newPuzzleTable(t)
	lock := p.lock(t, "Cofre")
	p.show(t, lock.GetId())
	ctx := t.Context()
	release := func() (*playv1.MasterPuzzleRun, error) {
		res, err := p.pc(p.master).ReleaseNextPuzzleHint(ctx, connect.NewRequest(&playv1.ReleaseNextPuzzleHintRequest{CampaignId: p.campaignID, PuzzleId: lock.GetId()}))
		if err != nil {
			return nil, err
		}
		return res.Msg.GetRun(), nil
	}
	if r := p.read(t, p.caio, lock.GetId()); len(r.GetHints()) != 0 {
		t.Fatalf("a player reads %v before any is released", r.GetHints())
	}
	for i := 1; i <= 3; i++ {
		m, err := release()
		if err != nil || int(m.GetReleasedHints()) != i || len(m.GetPuzzle().GetHints()) != 3 {
			t.Fatalf("release %d = %v, %v", i, m, err)
		}
		r := p.read(t, p.ana, lock.GetId())
		if len(r.GetHints()) != i || r.GetHints()[i-1] != lock.GetHints()[i-1] {
			t.Errorf("after %d releases a player reads %v", i, r.GetHints())
		}
		// The ones to come are not in what the player gets, in any form.
		for _, later := range lock.GetHints()[i:] {
			if strings.Contains(jsonOf(r), later) {
				t.Errorf("after %d releases the player's JSON has the hint %q", i, later)
			}
		}
	}
	_, err := release()
	wantPuzzleBlocked(t, "a fourth hint", err, playv1.PuzzleBlockedReason_PUZZLE_BLOCKED_REASON_NO_MORE_HINTS)
	_, err = p.pc(p.caio).ReleaseNextPuzzleHint(ctx, connect.NewRequest(&playv1.ReleaseNextPuzzleHintRequest{CampaignId: p.campaignID, PuzzleId: lock.GetId()}))
	wantCode(t, "ReleaseNextPuzzleHint as a player", err, connect.CodePermissionDenied)
}

// The session's history keeps only shown, solved, reset and closed, with IDs and no content.
func TestMR038_TheSessionHistoryKeepsTheFourEvents(t *testing.T) {
	t.Parallel()
	p := newPuzzleTable(t)
	ctx := t.Context()
	lock := p.lock(t, "Cofre", func(r *playv1.CreatePuzzleRequest) {
		r.Solution, r.Start = lockSolution(1, 0, 0, 0), lockStart(0, 0, 0, 0)
	})
	p.show(t, lock.GetId())
	p.mustMove(t, p.caio, lock.GetId(), lockMove(1, 1)) // a move is not an event
	if _, err := p.pc(p.master).ResetPuzzle(ctx, connect.NewRequest(&playv1.ResetPuzzleRequest{CampaignId: p.campaignID, PuzzleId: lock.GetId()})); err != nil {
		t.Fatal(err)
	}
	p.mustMove(t, p.ana, lock.GetId(), lockMove(0, 1)) // solves
	if _, err := p.pc(p.master).ClosePuzzle(ctx, connect.NewRequest(&playv1.ClosePuzzleRequest{CampaignId: p.campaignID, PuzzleId: lock.GetId()})); err != nil {
		t.Fatal(err)
	}
	rows, err := p.h.pool.Query(ctx, `SELECT kind, payload::TEXT, character_id::TEXT FROM session_events WHERE kind LIKE 'puzzle_%' ORDER BY seq`)
	if err != nil {
		t.Fatal(err)
	}
	defer rows.Close()
	var kinds []string
	for rows.Next() {
		var kind, payload string
		var character *string
		if err := rows.Scan(&kind, &payload, &character); err != nil {
			t.Fatal(err)
		}
		kinds = append(kinds, kind)
		var body map[string]string
		if err := json.Unmarshal([]byte(payload), &body); err != nil {
			t.Fatalf("payload %q: %v", payload, err)
		}
		for k := range body {
			if k != "puzzle_id" && k != "run_id" && k != "outcome" {
				t.Errorf("the %s payload has %q: only IDs and what the action did are kept", kind, k)
			}
		}
		if body["puzzle_id"] != lock.GetId() {
			t.Errorf("the %s payload names puzzle %q", kind, body["puzzle_id"])
		}
		if kind == eventPuzzleSolved && (character == nil || body["outcome"] != "notified") {
			t.Errorf("puzzle_solved = %v, %q; want the character and the outcome", character, body["outcome"])
		}
	}
	if want := []string{eventPuzzleShown, eventPuzzleReset, eventPuzzleSolved, eventPuzzleClosed}; strings.Join(kinds, ",") != strings.Join(want, ",") {
		t.Errorf("the session's events = %v, want %v", kinds, want)
	}
	var moves int
	if err := p.h.pool.QueryRow(ctx, `SELECT count(*) FROM puzzle_moves`).Scan(&moves); err != nil || moves != 2 {
		t.Errorf("puzzle_moves has %d rows (%v), want 2: the moves are kept in their own table", moves, err)
	}
}

// --- "Ao resolver" ---

// paintDoor paints a door on the cave's map as the master.
func (p *puzzleTable) paintDoor(t *testing.T, state mapsv1.DoorState, col, row int32) {
	t.Helper()
	if _, err := p.mc(p.master).PaintMapCells(t.Context(), connect.NewRequest(&mapsv1.PaintMapCellsRequest{
		CampaignId: p.campaignID, MapId: p.mapID, Layer: mapsv1.MapLayer_MAP_LAYER_DOORS, Value: int32(state), Squares: []*mapsv1.MapSquare{{Col: col, Row: row}},
	})); err != nil {
		t.Fatalf("PaintMapCells(door) error = %v", err)
	}
}

func (p *puzzleTable) doorAt(t *testing.T, col, row int) grid.Door {
	t.Helper()
	terrain, err := p.msvc.Terrain(t.Context(), nil, p.campaignID, p.mapID)
	if err != nil {
		t.Fatalf("Terrain() error = %v", err)
	}
	return terrain.Doors.Get(col, row)
}

func (p *puzzleTable) layersRevision(t *testing.T) int32 {
	t.Helper()
	res, err := p.mc(p.master).GetMapLayers(t.Context(), connect.NewRequest(&mapsv1.GetMapLayersRequest{CampaignId: p.campaignID, MapId: p.mapID}))
	if err != nil {
		t.Fatalf("GetMapLayers() error = %v", err)
	}
	return res.Msg.GetLayersRevision()
}

// oneMoveLock is a lock one turn from solved: the puzzle of the "Ao resolver" tests.
func (p *puzzleTable) oneMoveLock(t *testing.T, on *playv1.PuzzleOnSolve) *playv1.Puzzle {
	t.Helper()
	puz := p.lock(t, "A porta da Capela", func(r *playv1.CreatePuzzleRequest) {
		r.Solution, r.Start, r.OnSolve = lockSolution(1, 0, 0, 0), lockStart(0, 0, 0, 0), on
	})
	p.show(t, puz.GetId())
	return puz
}

func doorTarget(mapID string, col, row int32) *playv1.PuzzleOnSolve {
	return &playv1.PuzzleOnSolve{
		Action: playv1.PuzzleSolveAction_PUZZLE_SOLVE_ACTION_OPEN_DOOR,
		Target: &playv1.PuzzleOnSolve_Door{Door: &playv1.PuzzleDoorTarget{MapId: mapID, Col: col, Row: row}},
	}
}

// MR-038, RN-26: "Ao resolver: abrir uma porta" opens the door in the winning move's
// transaction, whatever state it was in, and the layers' revision goes up.
func TestMR038_SolvingOpensADoor(t *testing.T) {
	t.Parallel()
	for _, tc := range []struct {
		name  string
		state mapsv1.DoorState
	}{{"closed", mapsv1.DoorState_DOOR_STATE_CLOSED}, {"locked", mapsv1.DoorState_DOOR_STATE_LOCKED}, {"barred", mapsv1.DoorState_DOOR_STATE_BARRED}, {"secret", mapsv1.DoorState_DOOR_STATE_SECRET}} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			p := newPuzzleTable(t)
			p.paintDoor(t, tc.state, 7, 4)
			puz := p.oneMoveLock(t, doorTarget(p.mapID, 7, 4))
			rev := p.layersRevision(t)
			if got := p.doorAt(t, 7, 4); got == grid.DoorOpen {
				t.Fatalf("the door starts open")
			}
			watch := p.master.watch(t, p.campaignID)
			watch.ready(t)
			res := p.mustMove(t, p.caio, puz.GetId(), lockMove(0, 1))
			if !res.GetSolvedByThisMove() {
				t.Fatalf("the move did not solve it: %v", res)
			}
			if got := p.doorAt(t, 7, 4); got != grid.DoorOpen {
				t.Errorf("the door is %d after the solve, want open", got)
			}
			if got := p.layersRevision(t); got != rev+1 {
				t.Errorf("layers_revision = %d, want %d", got, rev+1)
			}
			if m := p.masterRun(t, puz.GetId()); m.GetOutcome() != playv1.PuzzleSolveOutcome_PUZZLE_SOLVE_OUTCOME_DOOR_OPENED {
				t.Errorf("outcome = %v, want DOOR_OPENED", m.GetOutcome())
			}
			// The players read a generic line (the master wrote none).
			if got := p.read(t, p.bia, puz.GetId()).GetSolvedMessage(); got != "Uma porta se abriu." {
				t.Errorf("the players read %q, want the generic door line", got)
			}
			// The master hears of it: the puzzle, and the map's layers.
			var gotPuzzle, gotMap bool
			for !gotPuzzle || !gotMap {
				ev := watch.nextChange(t)
				gotPuzzle = gotPuzzle || ev.GetPuzzleChanged().GetPuzzleId() == puz.GetId()
				gotMap = gotMap || ev.GetMapChanged().GetMapId() == p.mapID
			}
		})
	}
}

// On a calibrated map the door a puzzle opens is the whole square of the drawing,
// as walking through it opens it (RN-25): every door square of the block opens.
func TestMR038_SolvingOpensTheWholeDoorOfACalibratedMap(t *testing.T) {
	t.Parallel()
	p := newPuzzleTable(t)
	m, err := p.mc(p.master).GetMap(t.Context(), connect.NewRequest(&mapsv1.GetMapRequest{CampaignId: p.campaignID, MapId: p.mapID}))
	if err != nil {
		t.Fatalf("GetMap() error = %v", err)
	}
	if _, err := p.mc(p.master).SetMapGrid(t.Context(), connect.NewRequest(&mapsv1.SetMapGridRequest{
		CampaignId: p.campaignID, MapId: p.mapID, Columns: m.Msg.GetMap().GetGridColumns(), SquareFactor: 2,
	})); err != nil {
		t.Fatalf("SetMapGrid(factor 2) error = %v", err)
	}
	p.paintDoor(t, mapsv1.DoorState_DOOR_STATE_LOCKED, 7, 4) // painting one square paints its block
	block := [][2]int{{6, 4}, {7, 4}, {6, 5}, {7, 5}}
	for _, sq := range block {
		if got := p.doorAt(t, sq[0], sq[1]); got != grid.DoorLocked {
			t.Fatalf("door square %v is %d before the solve, want locked", sq, got)
		}
	}
	puz := p.oneMoveLock(t, doorTarget(p.mapID, 7, 4))
	if res := p.mustMove(t, p.caio, puz.GetId(), lockMove(0, 1)); !res.GetSolvedByThisMove() {
		t.Fatalf("the move did not solve it: %v", res)
	}
	for _, sq := range block {
		if got := p.doorAt(t, sq[0], sq[1]); got != grid.DoorOpen {
			t.Errorf("door square %v is %d after the solve, want open: the whole door opens", sq, got)
		}
	}
}

// The door was opened already: the puzzle is solved all the same, and says so.
func TestMR038_SolvingADoorThatIsOpenOrGone(t *testing.T) {
	t.Parallel()
	p := newPuzzleTable(t)
	p.paintDoor(t, mapsv1.DoorState_DOOR_STATE_OPEN, 7, 4)
	puz := p.oneMoveLock(t, doorTarget(p.mapID, 7, 4))
	rev := p.layersRevision(t)
	if res := p.mustMove(t, p.caio, puz.GetId(), lockMove(0, 1)); !res.GetSolvedByThisMove() {
		t.Fatalf("not solved: %v", res)
	}
	if m := p.masterRun(t, puz.GetId()); m.GetOutcome() != playv1.PuzzleSolveOutcome_PUZZLE_SOLVE_OUTCOME_DOOR_NOT_CLOSED || p.layersRevision(t) != rev {
		t.Errorf("an open door: outcome %v, revision %d -> %d", m.GetOutcome(), rev, p.layersRevision(t))
	}

	// The door is gone (painted over with nothing) after the puzzle was made.
	p.paintDoor(t, mapsv1.DoorState_DOOR_STATE_CLOSED, 9, 4)
	gone := p.oneMoveLock(t, doorTarget(p.mapID, 9, 4))
	p.paintDoor(t, mapsv1.DoorState_DOOR_STATE_UNSPECIFIED, 9, 4)
	if res := p.mustMove(t, p.ana, gone.GetId(), lockMove(0, 1)); !res.GetSolvedByThisMove() {
		t.Fatalf("not solved: %v", res)
	}
	if m := p.masterRun(t, gone.GetId()); m.GetOutcome() != playv1.PuzzleSolveOutcome_PUZZLE_SOLVE_OUTCOME_DOOR_NOT_CLOSED {
		t.Errorf("a door painted away: outcome %v", m.GetOutcome())
	}
}

// "Ao resolver: revelar um ponto do mapa" reveals the hidden point to the players.
func TestMR038_SolvingRevealsAPoint(t *testing.T) {
	t.Parallel()
	p := newPuzzleTable(t)
	x, y := sq(5, 5)
	res, err := p.mc(p.master).CreateMapPoint(t.Context(), connect.NewRequest(&mapsv1.CreateMapPointRequest{
		CampaignId: p.campaignID, MapId: p.mapID, Kind: mapsv1.MapPointKind_MAP_POINT_KIND_SCENE, Name: "A câmara do selo", XBp: x, YBp: y,
	}))
	if err != nil {
		t.Fatalf("CreateMapPoint() error = %v", err)
	}
	point := res.Msg.GetPoint()
	revealed := func() bool {
		var at *string
		if err := p.h.pool.QueryRow(t.Context(), `SELECT revealed_at::TEXT FROM map_points WHERE id = $1`, point.GetId()).Scan(&at); err != nil {
			t.Fatalf("read the point: %v", err)
		}
		return at != nil
	}
	if revealed() {
		t.Fatal("the point starts revealed")
	}
	puz := p.oneMoveLock(t, &playv1.PuzzleOnSolve{
		Action: playv1.PuzzleSolveAction_PUZZLE_SOLVE_ACTION_REVEAL_POINT,
		Target: &playv1.PuzzleOnSolve_Point{Point: &playv1.PuzzlePointTarget{MapId: p.mapID, PointId: point.GetId()}},
	})
	if revealed() {
		t.Fatal("the point was revealed by showing the puzzle")
	}
	p.mustMove(t, p.caio, puz.GetId(), lockMove(1, 1)) // not solved yet
	if revealed() {
		t.Fatal("the point was revealed before the puzzle was solved")
	}
	p.mustMove(t, p.caio, puz.GetId(), lockMove(1, -1))
	p.mustMove(t, p.caio, puz.GetId(), lockMove(0, 1))
	if !revealed() {
		t.Errorf("the point is still hidden after the solve")
	}
	if m := p.masterRun(t, puz.GetId()); m.GetOutcome() != playv1.PuzzleSolveOutcome_PUZZLE_SOLVE_OUTCOME_POINT_REVEALED {
		t.Errorf("outcome = %v", m.GetOutcome())
	}
	if got := p.read(t, p.bia, puz.GetId()).GetSolvedMessage(); got != "A câmara do selo apareceu no mapa." {
		t.Errorf("the players read %q, want the point's name and the generic line", got)
	}
	// The players read the point now.
	got, err := p.mc(p.bia).GetMap(t.Context(), connect.NewRequest(&mapsv1.GetMapRequest{CampaignId: p.campaignID, MapId: p.mapID}))
	if err != nil {
		t.Fatalf("GetMap() as a player error = %v", err)
	}
	found := false
	for _, pt := range got.Msg.GetPoints() {
		found = found || pt.GetId() == point.GetId()
	}
	if !found {
		t.Errorf("a player does not read the revealed point")
	}
	// A second puzzle with the same point: already revealed.
	again := p.oneMoveLock(t, &playv1.PuzzleOnSolve{
		Action: playv1.PuzzleSolveAction_PUZZLE_SOLVE_ACTION_REVEAL_POINT,
		Target: &playv1.PuzzleOnSolve_Point{Point: &playv1.PuzzlePointTarget{MapId: p.mapID, PointId: point.GetId()}},
	})
	p.mustMove(t, p.ana, again.GetId(), lockMove(0, 1))
	if m := p.masterRun(t, again.GetId()); m.GetOutcome() != playv1.PuzzleSolveOutcome_PUZZLE_SOLVE_OUTCOME_POINT_ALREADY_REVEALED {
		t.Errorf("outcome = %v, want POINT_ALREADY_REVEALED", m.GetOutcome())
	}
}

// "Ao resolver: revelar uma pista" gives the clue to the player whose move solved
// it, and to nobody else.
func TestMR038_SolvingRevealsAClueToTheSolverOnly(t *testing.T) {
	t.Parallel()
	p := newPuzzleTable(t)
	x, y := sq(6, 6)
	pt, err := p.mc(p.master).CreateMapPoint(t.Context(), connect.NewRequest(&mapsv1.CreateMapPointRequest{
		CampaignId: p.campaignID, MapId: p.mapID, Kind: mapsv1.MapPointKind_MAP_POINT_KIND_SCENE, Name: "A cripta", XBp: x, YBp: y,
	}))
	if err != nil {
		t.Fatalf("CreateMapPoint() error = %v", err)
	}
	clue, err := p.mc(p.master).AddSceneClue(t.Context(), connect.NewRequest(&mapsv1.AddSceneClueRequest{
		CampaignId: p.campaignID, MapId: p.mapID, PointId: pt.Msg.GetPoint().GetId(), Text: "A chave está sob a pedra da lua.",
	}))
	if err != nil {
		t.Fatalf("AddSceneClue() error = %v", err)
	}
	puz := p.oneMoveLock(t, &playv1.PuzzleOnSolve{
		Action: playv1.PuzzleSolveAction_PUZZLE_SOLVE_ACTION_REVEAL_CLUE,
		Target: &playv1.PuzzleOnSolve_Clue{Clue: &playv1.PuzzleClueTarget{ClueId: clue.Msg.GetClue().GetId()}},
	})
	watchAna, watchCaio := p.ana.watch(t, p.campaignID), p.caio.watch(t, p.campaignID)
	watchAna.ready(t)
	watchCaio.ready(t)
	// Pensantus's player makes the winning move.
	if res := p.mustMove(t, p.ana, puz.GetId(), lockMove(0, 1)); !res.GetSolvedByThisMove() {
		t.Fatalf("not solved: %v", res)
	}
	rows, err := p.h.pool.Query(t.Context(), `SELECT user_id::TEXT, character_id::TEXT FROM scene_clue_reveals WHERE clue_id = $1`, clue.Msg.GetClue().GetId())
	if err != nil {
		t.Fatal(err)
	}
	defer rows.Close()
	n := 0
	for rows.Next() {
		var user, character string
		if err := rows.Scan(&user, &character); err != nil {
			t.Fatal(err)
		}
		n++
		if user != p.ana.id || character != p.pens.GetId() {
			t.Errorf("the clue went to user %s, character %s; want Pensantus's player", user, character)
		}
	}
	if n != 1 {
		t.Errorf("%d players have the clue, want only the solver", n)
	}
	if m := p.masterRun(t, puz.GetId()); m.GetOutcome() != playv1.PuzzleSolveOutcome_PUZZLE_SOLVE_OUTCOME_CLUE_REVEALED {
		t.Errorf("outcome = %v", m.GetOutcome())
	}
	if got := p.read(t, p.bia, puz.GetId()).GetSolvedMessage(); got != "Você ganhou uma pista." {
		t.Errorf("the players read %q, want the generic clue line", got)
	}
	// Only the solver hears "notes changed".
	sawNotes := func(w *watcher) bool {
		for {
			ev := w.nextChange(t)
			if ev.GetNotesChanged() != nil {
				return true
			}
			if ev.GetPuzzleChanged() != nil {
				// The puzzle hint comes with the commit; the notes' just after.
				continue
			}
		}
	}
	if !sawNotes(watchAna) {
		t.Errorf("the solver did not hear of the clue")
	}
	// The same player again: they had it already.
	again := p.oneMoveLock(t, &playv1.PuzzleOnSolve{
		Action: playv1.PuzzleSolveAction_PUZZLE_SOLVE_ACTION_REVEAL_CLUE,
		Target: &playv1.PuzzleOnSolve_Clue{Clue: &playv1.PuzzleClueTarget{ClueId: clue.Msg.GetClue().GetId()}},
	})
	p.mustMove(t, p.ana, again.GetId(), lockMove(0, 1))
	if m := p.masterRun(t, again.GetId()); m.GetOutcome() != playv1.PuzzleSolveOutcome_PUZZLE_SOLVE_OUTCOME_CLUE_ALREADY_HAD {
		t.Errorf("outcome = %v, want CLUE_ALREADY_HAD", m.GetOutcome())
	}
	// Another solver gets it too: each player gets it once.
	third := p.oneMoveLock(t, &playv1.PuzzleOnSolve{
		Action: playv1.PuzzleSolveAction_PUZZLE_SOLVE_ACTION_REVEAL_CLUE,
		Target: &playv1.PuzzleOnSolve_Clue{Clue: &playv1.PuzzleClueTarget{ClueId: clue.Msg.GetClue().GetId()}},
	})
	p.mustMove(t, p.caio, third.GetId(), lockMove(0, 1))
	var total int
	if err := p.h.pool.QueryRow(t.Context(), `SELECT count(*) FROM scene_clue_reveals WHERE clue_id = $1`, clue.Msg.GetClue().GetId()).Scan(&total); err != nil || total != 2 {
		t.Errorf("%d players have the clue (%v), want 2", total, err)
	}
}

// --- RN-10 ---

// What a player may receive, message by message: the exact JSON keys, by path (the
// index of an array is dropped). Anything else, however it is named, fails the
// test (RN-10, RN-27, the artboard E10-06): so does a new field, until someone reads
// this list and decides that a player may have it.
var playerRunKeys = map[string][]string{
	"":                        {"puzzleId", "name", "kind", "config", "symbols", "state", "clue", "hints", "revision", "solved", "solvedAt", "solvedByName", "lastMove", "mural", "solvedMessage", "limits", "stopped", "stoppedMessage", "myPart", "partHolders", "hintByCheck", "hintSkillKey", "canTryHint", "sharedHints", "sequence", "hasKeyClue", "keyClueId"},
	".config":                 {"lights", "lock", "pillars", "riddle", "sequence", "cipher"},
	".config.lights":          {"size"},
	".config.lock":            {"wheels", "alphabet"},
	".config.pillars":         {"pillars", "symbols", "links"},
	".config.pillars.links[]": {"alsoTurns"},
	".config.riddle":          {"text"},
	".config.sequence":        {"bells", "steps"},
	".config.cipher":          {"ciphertext"},
	".symbols[]":              {"key", "namePt"},
	".state":                  {"lights", "lock", "pillars", "riddle", "sequence", "cipher"},
	".state.lights":           {"lit"},
	".state.lock":             {"wheels"},
	".state.pillars":          {"pillars"},
	".state.riddle":           {},
	".state.sequence":         {"progress"},
	".state.cipher":           {},
	// A typed answer and a struck bell never reach the other players: the riddle, the
	// sequence and the cipher are not in lastMove.move, whatever they say.
	".lastMove":              {"characterName", "move", "changed", "at", "wrong", "step", "trapName"},
	".lastMove.move":         {"lights", "lock", "pillars"},
	".lastMove.move.lights":  {"row", "col"},
	".lastMove.move.lock":    {"wheel", "delta"},
	".lastMove.move.pillars": {"pillar", "delta"},
	".mural":                 {"pillars"},
	".limits":                {"attemptsPerPlayer", "attemptsLeft", "maxMoves", "movesMade", "timeLimitSeconds", "secondsLeft", "deadline"},
	".sequence":              {"totalSteps", "plays", "playing", "shown", "stepMs", "nextInMs"},
}

// checkKeys fails for every object in raw (JSON) whose path or keys are not in the allowlist.
func checkKeys(t *testing.T, what, raw, prefix string, allowed map[string][]string) {
	t.Helper()
	var v any
	if err := json.Unmarshal([]byte(raw), &v); err != nil {
		t.Fatalf("%s is not JSON: %v", what, err)
	}
	var walk func(path string, v any)
	walk = func(path string, v any) {
		switch x := v.(type) {
		case map[string]any:
			keys, ok := allowed[strings.TrimPrefix(path, prefix)]
			if !ok {
				t.Errorf("%s: an object at %q that a player may not receive: %v", what, path, x)
				return
			}
			for k, child := range x {
				if !slices.Contains(keys, k) {
					t.Errorf("%s: the key %q at %q is not on the list of what a player may receive", what, k, path)
					continue
				}
				walk(path+"."+k, child)
			}
		case []any:
			for _, e := range x {
				walk(path+"[]", e)
			}
		}
	}
	walk(prefix, v)
}

func checkPlayerRun(t *testing.T, what string, run *playv1.PuzzleRun) {
	t.Helper()
	checkKeys(t, what, jsonOf(run), "", playerRunKeys)
}

func checkPlayerMove(t *testing.T, what string, res *playv1.MakePuzzleMoveResponse) {
	t.Helper()
	var top map[string]json.RawMessage
	if err := json.Unmarshal([]byte(jsonOf(res)), &top); err != nil {
		t.Fatal(err)
	}
	for k := range top {
		if k != "run" && k != "replayed" && k != "solvedByThisMove" && k != "wrong" {
			t.Errorf("%s: the key %q is not on the list", what, k)
		}
	}
	checkPlayerRun(t, what, res.GetRun())
}

// RN-10, RN-27: what a player receives, as the app's JSON, is exactly the allowed
// keys: never the lock's solution, the minimum, the path, the hints not released,
// the "Ao resolver" or its targets; and a puzzle not shown (or archived while shown)
// is not found. The turning symbols' players do read the mural and the links.
func TestRN10_PlayersNeverReceiveTheAnswer(t *testing.T) {
	t.Parallel()
	p := newPuzzleTable(t)
	ctx := t.Context()
	p.paintDoor(t, mapsv1.DoorState_DOOR_STATE_LOCKED, 7, 4)
	x, y := sq(5, 5)
	pt, err := p.mc(p.master).CreateMapPoint(ctx, connect.NewRequest(&mapsv1.CreateMapPointRequest{
		CampaignId: p.campaignID, MapId: p.mapID, Kind: mapsv1.MapPointKind_MAP_POINT_KIND_SCENE, Name: "A câmara secreta", XBp: x, YBp: y,
	}))
	if err != nil {
		t.Fatal(err)
	}
	clue, err := p.mc(p.master).AddSceneClue(ctx, connect.NewRequest(&mapsv1.AddSceneClueRequest{CampaignId: p.campaignID, MapId: p.mapID, PointId: pt.Msg.GetPoint().GetId(), Text: "TEXTO-DA-PISTA-SECRETA"}))
	if err != nil {
		t.Fatal(err)
	}
	leakMarker := "SEGREDO-DICA-DOIS"
	hiddenText := "SEGREDO-MENSAGEM-ANTES"
	lock := p.lock(t, "Cofre", func(r *playv1.CreatePuzzleRequest) {
		r.Hints = []string{"dica solta", leakMarker}
		r.Solution, r.Start = lockSolution(1, 0, 0, 0), lockStart(0, 0, 0, 0)
		r.OnSolve = doorTarget(p.mapID, 7, 4)
		r.OnSolve.Message = hiddenText
	})
	pointPuz := p.lock(t, "Ponto", func(r *playv1.CreatePuzzleRequest) {
		r.Solution, r.Start = lockSolution(1, 0, 0, 0), lockStart(0, 0, 0, 0)
		r.OnSolve = &playv1.PuzzleOnSolve{Action: playv1.PuzzleSolveAction_PUZZLE_SOLVE_ACTION_REVEAL_POINT, Target: &playv1.PuzzleOnSolve_Point{Point: &playv1.PuzzlePointTarget{MapId: p.mapID, PointId: pt.Msg.GetPoint().GetId()}}}
	})
	cluePuz := p.lock(t, "Pista", func(r *playv1.CreatePuzzleRequest) {
		r.Solution, r.Start = lockSolution(1, 0, 0, 0), lockStart(0, 0, 0, 0)
		r.OnSolve = &playv1.PuzzleOnSolve{Action: playv1.PuzzleSolveAction_PUZZLE_SOLVE_ACTION_REVEAL_CLUE, Target: &playv1.PuzzleOnSolve_Clue{Clue: &playv1.PuzzleClueTarget{ClueId: clue.Msg.GetClue().GetId()}}}
	})
	pillars := p.pillars(t, "Pilares", func(r *playv1.CreatePuzzleRequest) { r.Hints = []string{leakMarker} })
	lights := p.lights(t, "Luzes", 3)
	archived := p.lights(t, "SEGREDO-NOME-ARQUIVADO", 4)
	hidden := p.lights(t, "SEGREDO-NOME-NAO-MOSTRADO", 3)
	shown := []*playv1.Puzzle{lock, pointPuz, cluePuz, pillars, lights, archived}
	for _, puz := range shown {
		p.show(t, puz.GetId())
	}
	if _, err := p.pc(p.master).ReleaseNextPuzzleHint(ctx, connect.NewRequest(&playv1.ReleaseNextPuzzleHintRequest{CampaignId: p.campaignID, PuzzleId: lock.GetId()})); err != nil {
		t.Fatal(err)
	}

	var pile []string
	add := func(m proto.Message) { pile = append(pile, jsonOf(m)) }
	for _, u := range []*user{p.caio, p.ana, p.bia} {
		list, err := p.pc(u).ListShownPuzzles(ctx, connect.NewRequest(&playv1.ListShownPuzzlesRequest{CampaignId: p.campaignID}))
		if err != nil || len(list.Msg.GetPuzzles()) != len(shown) {
			t.Fatalf("ListShownPuzzles() = %v, %v", list, err)
		}
		checkKeys(t, "ListShownPuzzles", jsonOf(list.Msg), "", map[string][]string{"": {"puzzles"}, ".puzzles[]": {"puzzleId", "name", "kind", "solved"}})
		add(list.Msg)
		for _, puz := range shown {
			r := p.read(t, u, puz.GetId())
			checkPlayerRun(t, "GetPuzzleRun "+puz.GetName(), r)
			add(r)
		}
	}
	// The turning symbols' players read the mural and the links, nothing of the rest.
	if r := p.read(t, p.caio, pillars.GetId()); len(r.GetMural().GetPillars()) != 4 || len(r.GetConfig().GetPillars().GetLinks()) != 4 {
		t.Errorf("the pillars' run has no mural or no links: %v", r)
	}
	// Moves, and the winning move of every kind of puzzle (its answer says what happened).
	for _, puz := range []*playv1.Puzzle{lock, pointPuz, cluePuz} {
		res := p.mustMove(t, p.caio, puz.GetId(), lockMove(0, 1))
		if !res.GetSolvedByThisMove() {
			t.Fatalf("%s was not solved", puz.GetName())
		}
		checkPlayerMove(t, "the winning move of "+puz.GetName(), res)
		add(res)
	}
	for _, puz := range []*playv1.Puzzle{pillars, lights} {
		path := p.masterRun(t, puz.GetId()).GetMinimum().GetPath()
		for i, mv := range path {
			res := p.mustMove(t, p.ana, puz.GetId(), mv)
			checkPlayerMove(t, fmt.Sprintf("move %d of %s", i, puz.GetName()), res)
			add(res)
		}
		if !p.read(t, p.bia, puz.GetId()).GetSolved() {
			t.Fatalf("%s was not solved", puz.GetName())
		}
	}
	streams := map[string]*watcher{"Toren": p.caio.watch(t, p.campaignID), "Pensantus": p.ana.watch(t, p.campaignID)}
	for _, w := range streams {
		w.ready(t)
	}
	// Three changes of three puzzles, so three hints (one per puzzle is not merged).
	for _, id := range []string{lock.GetId(), cluePuz.GetId()} {
		if _, err := p.pc(p.master).ReleaseNextPuzzleHint(ctx, connect.NewRequest(&playv1.ReleaseNextPuzzleHintRequest{CampaignId: p.campaignID, PuzzleId: id})); err != nil {
			t.Fatal(err)
		}
	}
	// A puzzle archived while shown vanishes for the players.
	if _, err := p.pc(p.master).ArchivePuzzle(ctx, connect.NewRequest(&playv1.ArchivePuzzleRequest{CampaignId: p.campaignID, PuzzleId: archived.GetId()})); err != nil {
		t.Fatal(err)
	}
	_, err = p.pc(p.caio).GetPuzzleRun(ctx, connect.NewRequest(&playv1.GetPuzzleRunRequest{CampaignId: p.campaignID, PuzzleId: archived.GetId()}))
	wantCode(t, "GetPuzzleRun of a puzzle archived while shown", err, connect.CodeNotFound)
	_, err = p.move(t, p.caio, archived.GetId(), lightsMove(0, 0))
	wantCode(t, "MakePuzzleMove of a puzzle archived while shown", err, connect.CodeNotFound)
	list, _ := p.pc(p.caio).ListShownPuzzles(ctx, connect.NewRequest(&playv1.ListShownPuzzlesRequest{CampaignId: p.campaignID}))
	for _, e := range list.Msg.GetPuzzles() {
		if e.GetPuzzleId() == archived.GetId() || e.GetName() == archived.GetName() {
			t.Errorf("the players' list still has the archived puzzle")
		}
	}
	if got := p.read(t, p.bia, lock.GetId()).GetSolvedMessage(); got != hiddenText {
		t.Errorf("the master's text reads %q after the solve, want %q", got, hiddenText)
	}

	for name, w := range streams {
		got := 0
		for got < 3 {
			ev := w.nextChange(t)
			if ev.GetPuzzleChanged() == nil {
				continue
			}
			got++
			checkKeys(t, name+"'s puzzle_changed", jsonOf(ev), "", map[string][]string{"": {"puzzleChanged"}, ".puzzleChanged": {"puzzleId"}})
			add(ev)
		}
	}

	// Strings that only the master may ever see, anywhere in the pile.
	all := strings.Join(pile, "\n")
	for _, forbidden := range []string{
		leakMarker, "TEXTO-DA-PISTA-SECRETA", hidden.GetName(), hidden.GetId(),
		p.mapID, pt.Msg.GetPoint().GetId(), clue.Msg.GetClue().GetId(),
		"solution", "minimum", "path", "onSolve", "movesMade", "outcome", "releasedHints", "doorTarget", "mapId", "pointId", "clueId",
	} {
		if strings.Contains(all, forbidden) {
			i := strings.Index(all, forbidden)
			t.Errorf("a player's JSON has %q: ...%s...", forbidden, all[max(0, i-60):min(len(all), i+80)])
		}
	}
	// The master's own view has them, so the test above is not empty.
	if mj := jsonOf(p.masterRun(t, lock.GetId())); !strings.Contains(mj, "solution") || !strings.Contains(mj, leakMarker) || !strings.Contains(mj, "minimum") || !strings.Contains(mj, hiddenText) {
		t.Errorf("the master's view lacks the answer: %s", mj)
	}
	// A puzzle not shown, and one that does not exist, answer the same.
	_, err = p.pc(p.caio).GetPuzzleRun(ctx, connect.NewRequest(&playv1.GetPuzzleRunRequest{CampaignId: p.campaignID, PuzzleId: hidden.GetId()}))
	wantCode(t, "GetPuzzleRun of a puzzle not shown", err, connect.CodeNotFound)
	_, err = p.pc(p.caio).GetPuzzleRun(ctx, connect.NewRequest(&playv1.GetPuzzleRunRequest{CampaignId: p.campaignID, PuzzleId: newKey()}))
	wantCode(t, "GetPuzzleRun of a puzzle that does not exist", err, connect.CodeNotFound)
	// And the master's calls refuse a player outright.
	_, err = p.pc(p.caio).GetMasterPuzzleRun(ctx, connect.NewRequest(&playv1.GetMasterPuzzleRunRequest{CampaignId: p.campaignID, PuzzleId: lock.GetId()}))
	wantCode(t, "GetMasterPuzzleRun as a player", err, connect.CodePermissionDenied)
	_, err = p.pc(p.caio).ListSessionPuzzles(ctx, connect.NewRequest(&playv1.ListSessionPuzzlesRequest{CampaignId: p.campaignID}))
	wantCode(t, "ListSessionPuzzles as a player", err, connect.CodePermissionDenied)
	_, err = p.pc(p.caio).ShowPuzzle(ctx, connect.NewRequest(&playv1.ShowPuzzleRequest{CampaignId: p.campaignID, PuzzleId: hidden.GetId()}))
	wantCode(t, "ShowPuzzle as a player", err, connect.CodePermissionDenied)
}

// The stream: puzzle_changed to the master and to every player, with the puzzle's ID alone.
func TestMR038_TheStreamHintReachesEveryone(t *testing.T) {
	t.Parallel()
	p := newPuzzleTable(t)
	lock := p.lock(t, "Cofre")
	streams := map[string]*watcher{"master": p.master.watch(t, p.campaignID), "Toren": p.caio.watch(t, p.campaignID), "Pensantus": p.ana.watch(t, p.campaignID), "Brisa": p.bia.watch(t, p.campaignID)}
	for _, w := range streams {
		w.ready(t)
	}
	waitHint := func(what string) {
		t.Helper()
		for name, w := range streams {
			for {
				ev := w.nextChange(t)
				if h := ev.GetPuzzleChanged(); h != nil {
					if h.GetPuzzleId() != lock.GetId() {
						t.Errorf("%s: %s's hint names puzzle %q", what, name, h.GetPuzzleId())
					}
					break
				}
			}
		}
	}
	p.show(t, lock.GetId())
	waitHint("show")
	p.mustMove(t, p.caio, lock.GetId(), lockMove(0, 1))
	waitHint("move")
	if _, err := p.pc(p.master).ReleaseNextPuzzleHint(t.Context(), connect.NewRequest(&playv1.ReleaseNextPuzzleHintRequest{CampaignId: p.campaignID, PuzzleId: lock.GetId()})); err != nil {
		t.Fatal(err)
	}
	waitHint("hint")
	if _, err := p.pc(p.master).ClosePuzzle(t.Context(), connect.NewRequest(&playv1.ClosePuzzleRequest{CampaignId: p.campaignID, PuzzleId: lock.GetId()})); err != nil {
		t.Fatal(err)
	}
	waitHint("close")
}

// A run prepared with a new start ("Gerar outro começo") and then an edit of the puzzle
// (allowed: not shown yet) must not keep the old start: the edit drops the prepared run.
func TestMR038_AnEditDropsThePreparedRun(t *testing.T) {
	t.Parallel()
	p := newPuzzleTable(t)
	ctx := t.Context()
	lights := p.lights(t, "Luzes", 5)
	if _, err := p.pc(p.master).ReseedPuzzle(ctx, connect.NewRequest(&playv1.ReseedPuzzleRequest{CampaignId: p.campaignID, PuzzleId: lights.GetId()})); err != nil {
		t.Fatal(err)
	}
	if _, err := p.pc(p.master).UpdatePuzzle(ctx, connect.NewRequest(&playv1.UpdatePuzzleRequest{CampaignId: p.campaignID, PuzzleId: lights.GetId(), Name: "Luzes", Config: lightsConfig(7)})); err != nil {
		t.Fatalf("UpdatePuzzle() error = %v", err)
	}
	if m := p.masterRun(t, lights.GetId()); m.GetStatus() != playv1.PuzzleRunStatus_PUZZLE_RUN_STATUS_NOT_SHOWN || m.GetStart() != nil {
		t.Errorf("the prepared run survived the edit: %v", m)
	}
	shown := p.show(t, lights.GetId())
	if n := len(shown.GetRun().GetState().GetLights().GetLit()); n != 49 || shown.GetRun().GetConfig().GetLights().GetSize() != 7 {
		t.Fatalf("shown with %d cells on a board of %d, want 49 on 7", n, shown.GetRun().GetConfig().GetLights().GetSize())
	}
	p.mustMove(t, p.caio, lights.GetId(), lightsMove(0, 0))
	// The master can draw another start again after the edit.
	if _, err := p.pc(p.master).ReseedPuzzle(ctx, connect.NewRequest(&playv1.ReseedPuzzleRequest{CampaignId: p.campaignID, PuzzleId: lights.GetId()})); err != nil {
		t.Errorf("ReseedPuzzle() after the edit error = %v", err)
	}

	// The pillars: the mural edited to be the prepared start must not show it solved.
	pil := p.pillars(t, "Pilares")
	re, err := p.pc(p.master).ReseedPuzzle(ctx, connect.NewRequest(&playv1.ReseedPuzzleRequest{CampaignId: p.campaignID, PuzzleId: pil.GetId()}))
	if err != nil {
		t.Fatal(err)
	}
	prepared := re.Msg.GetRun().GetStart().GetPillars().GetPillars()
	if _, err := p.pc(p.master).UpdatePuzzle(ctx, connect.NewRequest(&playv1.UpdatePuzzleRequest{CampaignId: p.campaignID, PuzzleId: pil.GetId(), Name: "Pilares", Config: pil.GetConfig(), Solution: pillarsSolution(prepared...)})); err != nil {
		t.Fatalf("UpdatePuzzle(pillars) error = %v", err)
	}
	m := p.show(t, pil.GetId())
	if m.GetRun().GetSolved() || !m.GetMinimum().GetSolvable() || m.GetMinimum().GetMoves() < 3 {
		t.Errorf("the pillars show solved or too easy after the edit: %v", m)
	}
}

// "Ao resolver" has a text for the players: the master's own, kept as written.
func TestMR038_TheMasterSolveMessage(t *testing.T) {
	t.Parallel()
	p := newPuzzleTable(t)
	notify := p.lock(t, "Aviso", func(r *playv1.CreatePuzzleRequest) {
		r.Solution, r.Start = lockSolution(1, 0, 0, 0), lockStart(0, 0, 0, 0)
		r.OnSolve = &playv1.PuzzleOnSolve{Message: "  A porta da Capela se abriu.  "}
	})
	if notify.GetOnSolve().GetMessage() != "A porta da Capela se abriu." || notify.GetOnSolve().GetAction() != playv1.PuzzleSolveAction_PUZZLE_SOLVE_ACTION_NOTIFY {
		t.Fatalf("the notify with a text = %v", notify.GetOnSolve())
	}
	plain := p.lock(t, "Sem texto", func(r *playv1.CreatePuzzleRequest) {
		r.Solution, r.Start = lockSolution(1, 0, 0, 0), lockStart(0, 0, 0, 0)
	})
	p.show(t, notify.GetId())
	p.show(t, plain.GetId())
	if r := p.read(t, p.caio, notify.GetId()); r.GetSolvedMessage() != "" {
		t.Errorf("the text is read before it is solved: %q", r.GetSolvedMessage())
	}
	p.mustMove(t, p.caio, notify.GetId(), lockMove(0, 1))
	p.mustMove(t, p.caio, plain.GetId(), lockMove(0, 1))
	if got := p.read(t, p.ana, notify.GetId()).GetSolvedMessage(); got != "A porta da Capela se abriu." {
		t.Errorf("the players read %q", got)
	}
	if got := p.read(t, p.ana, plain.GetId()).GetSolvedMessage(); got != "" {
		t.Errorf("a notify with no text says %q to the players, want nothing", got)
	}
	// A door painted away: the master's text is still said, the outcome says why nothing opened.
	p.paintDoor(t, mapsv1.DoorState_DOOR_STATE_CLOSED, 3, 3)
	gone := p.lock(t, "Porta", func(r *playv1.CreatePuzzleRequest) {
		r.Solution, r.Start = lockSolution(1, 0, 0, 0), lockStart(0, 0, 0, 0)
		r.OnSolve = doorTarget(p.mapID, 3, 3)
		r.OnSolve.Message = "Algo estalou."
	})
	p.show(t, gone.GetId())
	p.paintDoor(t, mapsv1.DoorState_DOOR_STATE_UNSPECIFIED, 3, 3)
	p.mustMove(t, p.caio, gone.GetId(), lockMove(0, 1))
	if m := p.masterRun(t, gone.GetId()); m.GetOutcome() != playv1.PuzzleSolveOutcome_PUZZLE_SOLVE_OUTCOME_DOOR_NOT_CLOSED || m.GetRun().GetSolvedMessage() != "Algo estalou." {
		t.Errorf("a door painted away: outcome %v, text %q", m.GetOutcome(), m.GetRun().GetSolvedMessage())
	}
	// Too long.
	_, err := p.pc(p.master).CreatePuzzle(t.Context(), connect.NewRequest(&playv1.CreatePuzzleRequest{
		CampaignId: p.campaignID, Name: "x", Config: lightsConfig(3), OnSolve: &playv1.PuzzleOnSolve{Message: strings.Repeat("a", 201)},
	}))
	wantPuzzleInvalid(t, "a text of 201 characters", err, playv1.PuzzleInvalidReason_PUZZLE_INVALID_REASON_TEXT)
}

// puzzle_changed has a time gate: a burst of moves is a few hints, the last change is
// never left unannounced, and another puzzle has its own gate.
func TestMR038_TheHintIsThrottled(t *testing.T) {
	t.Parallel()
	p := newPuzzleTable(t)
	p.h.svc.puzzles.hintEvery = 400 * time.Millisecond
	lock := p.lock(t, "Cofre", func(r *playv1.CreatePuzzleRequest) {
		r.Config = lockConfig(6, playv1.PuzzleAlphabet_PUZZLE_ALPHABET_DIGITS)
		r.Solution, r.Start = lockSolution(9, 9, 9, 9, 9, 9), lockStart(0, 0, 0, 0, 0, 0)
	})
	w := p.ana.watch(t, p.campaignID)
	w.ready(t)
	start := time.Now()
	p.show(t, lock.GetId())
	var beforeLast time.Time
	for range 12 {
		beforeLast = time.Now()
		p.mustMove(t, p.caio, lock.GetId(), lockMove(0, 1))
	}
	lastMove := time.Now()
	var hints []time.Time
	deadline := time.After(3 * time.Second)
loop:
	for {
		select {
		case ev, ok := <-w.events:
			if !ok {
				break loop
			}
			if ev.GetPuzzleChanged() != nil {
				hints = append(hints, time.Now())
			}
		case <-deadline:
			break loop
		}
	}
	// 13 changes (the show and 12 moves). The gate's timing is TestPuzzleGate's (a fake
	// clock); here, through the stream, what holds on any machine: the changes were
	// merged (never more hints than one per interval of the time the moves took, plus
	// the trailing one), and the last change was announced after the last move.
	if len(hints) < 2 {
		t.Fatalf("%d hints for 13 changes, want the first at once and a trailing one", len(hints))
	}
	every := p.h.svc.puzzles.hintEvery
	if most := 2 + int(lastMove.Sub(start)/every); len(hints) > most {
		t.Errorf("%d hints for 13 changes in %v, want at most %d (one per %v): the changes were not merged", len(hints), lastMove.Sub(start), most, every)
	}
	if end := hints[len(hints)-1]; end.Before(beforeLast) {
		t.Errorf("the last hint came before the last move started: the last change was lost")
	}
}

// Showing or solving a puzzle mid-combat must not close the combat's undo chain, nor
// must the clue the solve gave.
func TestMR038_APuzzleNeverClosesTheUndoChain(t *testing.T) {
	t.Parallel()
	f := newDoorCave(t, mapsv1.DoorState_DOOR_STATE_UNSPECIFIED)
	f.h.svc.SetPuzzleMaps(f.msvc)
	p := &puzzleTable{armed: f.armed, msvc: f.msvc, server: f.server}
	x, y := atBP(grid.Square{Col: 5, Row: 5})
	pt, err := p.mc(p.master).CreateMapPoint(t.Context(), connect.NewRequest(&mapsv1.CreateMapPointRequest{
		CampaignId: p.campaignID, MapId: p.mapID, Kind: mapsv1.MapPointKind_MAP_POINT_KIND_SCENE, Name: "Cripta", XBp: x, YBp: y,
	}))
	if err != nil {
		t.Fatal(err)
	}
	clue, err := p.mc(p.master).AddSceneClue(t.Context(), connect.NewRequest(&mapsv1.AddSceneClueRequest{CampaignId: p.campaignID, MapId: p.mapID, PointId: pt.Msg.GetPoint().GetId(), Text: "pista"}))
	if err != nil {
		t.Fatal(err)
	}
	f.mustMove(t, f.master, "Goblin 1", 17, 5)
	puz := p.lock(t, "Cofre", func(r *playv1.CreatePuzzleRequest) {
		r.Solution, r.Start = lockSolution(1, 0, 0, 0), lockStart(0, 0, 0, 0)
		r.OnSolve = &playv1.PuzzleOnSolve{Action: playv1.PuzzleSolveAction_PUZZLE_SOLVE_ACTION_REVEAL_CLUE, Target: &playv1.PuzzleOnSolve_Clue{Clue: &playv1.PuzzleClueTarget{ClueId: clue.Msg.GetClue().GetId()}}}
	})
	p.show(t, puz.GetId())
	if res := p.mustMove(t, p.ana, puz.GetId(), lockMove(0, 1)); !res.GetSolvedByThisMove() {
		t.Fatalf("not solved: %v", res)
	}
	if _, err := p.pc(p.master).ClosePuzzle(t.Context(), connect.NewRequest(&playv1.ClosePuzzleRequest{CampaignId: p.campaignID, PuzzleId: puz.GetId()})); err != nil {
		t.Fatal(err)
	}
	f.undoLast(t) // still the master's forced move of Goblin 1
	if got := f.who(t, f.master, "Goblin 1"); got.GetCol() != 18 {
		t.Errorf("the undo left Goblin 1 at column %d, want 18: a puzzle closed the undo chain", got.GetCol())
	}
}

// A puzzle whose "Ao resolver" reveals a point of a map the players cannot open does not
// name it in the line they read, in the puzzle or in the session's events (RN-10).
func TestPuzzleRevealDoesNotNameAPointOfAHiddenMap(t *testing.T) {
	t.Parallel()
	p := newPuzzleTable(t)
	const needle = "LEAKCANARY-point-1"
	hiddenMap := p.h.newMap(p.campaignID, gridColumns) // not current, never revealed
	res, err := p.mc(p.master).CreateMapPoint(t.Context(), connect.NewRequest(&mapsv1.CreateMapPointRequest{
		CampaignId: p.campaignID, MapId: hiddenMap, Kind: mapsv1.MapPointKind_MAP_POINT_KIND_SCENE, Name: needle,
		XBp: 2500, YBp: 2500,
	}))
	if err != nil {
		t.Fatalf("CreateMapPoint() error = %v", err)
	}
	if got := res.Msg.GetPoint().GetName(); got != needle { // positive control: the master reads the name
		t.Fatalf("the master's point is named %q, want %q", got, needle)
	}
	puz := p.oneMoveLock(t, &playv1.PuzzleOnSolve{
		Action: playv1.PuzzleSolveAction_PUZZLE_SOLVE_ACTION_REVEAL_POINT,
		Target: &playv1.PuzzleOnSolve_Point{Point: &playv1.PuzzlePointTarget{MapId: hiddenMap, PointId: res.Msg.GetPoint().GetId()}},
	})
	var rev *string
	if err := p.h.pool.QueryRow(t.Context(), `SELECT revealed_at::TEXT FROM maps WHERE id = $1`, hiddenMap).Scan(&rev); err != nil || rev != nil {
		t.Fatalf("the map must be hidden: %v %v", err, rev)
	}
	move := p.mustMove(t, p.caio, puz.GetId(), lockMove(0, 1))
	var seen []string
	seen = append(seen, "move: "+jsonOf(move))
	for _, u := range []*user{p.caio, p.ana, p.bia} {
		seen = append(seen, "read: "+jsonOf(p.read(t, u, puz.GetId())))
	}
	var sid string
	if err := p.h.pool.QueryRow(t.Context(), `SELECT id::STRING FROM game_sessions WHERE campaign_id = $1 LIMIT 1`, p.campaignID).Scan(&sid); err != nil {
		t.Fatalf("read the session: %v", err)
	}
	rows, err := p.h.pool.Query(t.Context(), `SELECT kind, payload::STRING FROM session_events WHERE game_session_id = $1 ORDER BY seq`, sid)
	if err != nil {
		t.Fatalf("read session_events: %v", err)
	}
	defer rows.Close()
	for rows.Next() {
		var kind, payload string
		if err := rows.Scan(&kind, &payload); err != nil {
			t.Fatal(err)
		}
		seen = append(seen, "event "+kind+": "+payload)
	}
	for _, s := range seen {
		if strings.Contains(s, needle) {
			t.Errorf("a player can read the hidden point's name: %s", s)
		}
	}
}
