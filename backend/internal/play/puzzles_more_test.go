package play

import (
	"encoding/json"
	"regexp"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"connectrpc.com/connect"

	campaignsv1 "github.com/PuraFome/meuRPG/backend/gen/meurpg/campaigns/v1"
	mapsv1 "github.com/PuraFome/meuRPG/backend/gen/meurpg/maps/v1"
	playv1 "github.com/PuraFome/meuRPG/backend/gen/meurpg/play/v1"
	rulesv1 "github.com/PuraFome/meuRPG/backend/gen/meurpg/rules/v1"
	"github.com/PuraFome/meuRPG/backend/internal/platform/dbtest"
	"github.com/PuraFome/meuRPG/backend/internal/rules/puzzle"
)

// More puzzles (MR-038, RN-27, RN-10, RN-18, Etapa 10, slice 10.7b): the riddle, the
// sequence and the cipher; the hints won by a skill check; the split information; and the
// consequences of a wrong move ("Ao errar"). They use the fixture of puzzles_test.go:
// Toren's player is caio, Pensantus's ana and Brisa's bia. Every answer a player gets is
// read as the app's JSON, so a field that leaks shows up whatever its name.

// --- builders ---

const (
	riddleText = "Moro embaixo de cada passo seu, mas nunca peso nada. O que sou?"
	letter     = "O tesouro está sob o altar"
	letterC    = "R WHVRXUR HVWD VRE R DOWDU"
)

func riddleConfig() *playv1.PuzzleConfig {
	return &playv1.PuzzleConfig{Kind: &playv1.PuzzleConfig_Riddle{Riddle: &playv1.RiddleConfig{Text: riddleText}}}
}

func riddleSolution(answers ...string) *playv1.PuzzleSolution {
	return &playv1.PuzzleSolution{Kind: &playv1.PuzzleSolution_Riddle{Riddle: &playv1.RiddleSolution{Answers: answers}}}
}

func riddleMove(answer string) *playv1.PuzzleMove {
	return &playv1.PuzzleMove{Kind: &playv1.PuzzleMove_Riddle{Riddle: &playv1.RiddleMove{Answer: answer}}}
}

func sequenceConfig(bells int32) *playv1.PuzzleConfig {
	return &playv1.PuzzleConfig{Kind: &playv1.PuzzleConfig_Sequence{Sequence: &playv1.SequenceConfig{Bells: bells}}}
}

func sequenceSolution(steps ...int32) *playv1.PuzzleSolution {
	return &playv1.PuzzleSolution{Kind: &playv1.PuzzleSolution_Sequence{Sequence: &playv1.SequenceSolution{Steps: steps}}}
}

func bellMove(bell int32) *playv1.PuzzleMove {
	return &playv1.PuzzleMove{Kind: &playv1.PuzzleMove_Sequence{Sequence: &playv1.SequenceMove{Bell: bell}}}
}

func cipherConfig(keyClueID string) *playv1.PuzzleConfig {
	return &playv1.PuzzleConfig{Kind: &playv1.PuzzleConfig_Cipher{Cipher: &playv1.CipherConfig{KeyClueId: keyClueID}}}
}

func cipherSolution(message string, shift int32) *playv1.PuzzleSolution {
	return &playv1.PuzzleSolution{Kind: &playv1.PuzzleSolution_Cipher{Cipher: &playv1.CipherSolution{Message: message, Method: &playv1.CipherSolution_Shift{Shift: shift}}}}
}

func cipherMove(text string) *playv1.PuzzleMove {
	return &playv1.PuzzleMove{Kind: &playv1.PuzzleMove_Cipher{Cipher: &playv1.CipherMove{Text: text}}}
}

// riddle makes the riddle whose answer is "sombra" (or "a sombra").
func (p *puzzleTable) riddle(t *testing.T, name string, edit ...func(*playv1.CreatePuzzleRequest)) *playv1.Puzzle {
	t.Helper()
	req := &playv1.CreatePuzzleRequest{Name: name, Config: riddleConfig(), Solution: riddleSolution("sombra", "a sombra"), Clue: "Procure no chão da Cripta."}
	for _, e := range edit {
		e(req)
	}
	return p.create(t, req)
}

// sequence makes four bells and the steps 2 0 3 1.
func (p *puzzleTable) sequence(t *testing.T, name string, edit ...func(*playv1.CreatePuzzleRequest)) *playv1.Puzzle {
	t.Helper()
	req := &playv1.CreatePuzzleRequest{Name: name, Config: sequenceConfig(4), Solution: sequenceSolution(2, 0, 3, 1), Clue: "Quem toca os sinos escuta o trono."}
	for _, e := range edit {
		e(req)
	}
	return p.create(t, req)
}

// cipher makes the letter of the artboard (a shift of three).
func (p *puzzleTable) cipher(t *testing.T, name string, edit ...func(*playv1.CreatePuzzleRequest)) *playv1.Puzzle {
	t.Helper()
	req := &playv1.CreatePuzzleRequest{Name: name, Config: cipherConfig(""), Solution: cipherSolution(letter, 3)}
	for _, e := range edit {
		e(req)
	}
	return p.create(t, req)
}

func onWrong(edit func(*playv1.PuzzleOnWrong)) func(*playv1.CreatePuzzleRequest) {
	return func(r *playv1.CreatePuzzleRequest) {
		r.OnWrong = &playv1.PuzzleOnWrong{}
		edit(r.OnWrong)
	}
}

// trapPoint makes a trap point on the table's map.
func (p *puzzleTable) trapPoint(t *testing.T, name string, col, row int) *mapsv1.MapPoint {
	t.Helper()
	x, y := sq(col, row)
	res, err := p.mc(p.master).CreateMapPoint(t.Context(), connect.NewRequest(&mapsv1.CreateMapPointRequest{
		CampaignId: p.campaignID, MapId: p.mapID, Kind: mapsv1.MapPointKind_MAP_POINT_KIND_TRAP, Name: name, XBp: x, YBp: y, Trap: aTrap(),
	}))
	if err != nil {
		t.Fatalf("CreateMapPoint(%s) error = %v", name, err)
	}
	return res.Msg.GetPoint()
}

func (p *puzzleTable) trapState(t *testing.T, pointID string) string {
	t.Helper()
	var state *string
	if err := p.h.pool.QueryRow(t.Context(), `SELECT trap_state FROM map_points WHERE id = $1`, pointID).Scan(&state); err != nil {
		t.Fatal(err)
	}
	return deref(state)
}

func (p *puzzleTable) eventCount(t *testing.T, kind string) int {
	t.Helper()
	var n int
	if err := p.h.pool.QueryRow(t.Context(), `SELECT count(*) FROM session_events WHERE kind = $1`, kind).Scan(&n); err != nil {
		t.Fatal(err)
	}
	return n
}

func onWrongTrap(p *playv1.PuzzleTrapTarget) func(*playv1.PuzzleOnWrong) {
	return func(o *playv1.PuzzleOnWrong) { o.Trap = p }
}

// movableClock is a clock the test moves: the time limit and the sequence's play are
// measured against the service's own clock.
type movableClock struct {
	mu sync.Mutex
	t  time.Time
}

func (c *movableClock) now() time.Time {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.t = c.t.Add(time.Microsecond)
	return c.t
}

func (c *movableClock) advance(d time.Duration) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.t = c.t.Add(d)
}

func (p *puzzleTable) movableClock() *movableClock {
	c := &movableClock{t: time.Now().Truncate(time.Microsecond)}
	p.h.svc.now = c.now
	return c
}

func (p *puzzleTable) reset(t *testing.T, id string) *playv1.MasterPuzzleRun {
	t.Helper()
	res, err := p.pc(p.master).ResetPuzzle(t.Context(), connect.NewRequest(&playv1.ResetPuzzleRequest{CampaignId: p.campaignID, PuzzleId: id}))
	if err != nil {
		t.Fatalf("ResetPuzzle() error = %v", err)
	}
	return res.Msg.GetRun()
}

func (p *puzzleTable) playSequence(t *testing.T, id string) *playv1.MasterPuzzleRun {
	t.Helper()
	res, err := p.pc(p.master).PlayPuzzleSequence(t.Context(), connect.NewRequest(&playv1.PlayPuzzleSequenceRequest{CampaignId: p.campaignID, PuzzleId: id}))
	if err != nil {
		t.Fatalf("PlayPuzzleSequence() error = %v", err)
	}
	return res.Msg.GetRun()
}

func (p *puzzleTable) release(t *testing.T, id string) {
	t.Helper()
	if _, err := p.pc(p.master).ReleaseNextPuzzleHint(t.Context(), connect.NewRequest(&playv1.ReleaseNextPuzzleHintRequest{CampaignId: p.campaignID, PuzzleId: id})); err != nil {
		t.Fatalf("ReleaseNextPuzzleHint() error = %v", err)
	}
}

// tryHint rolls for a hint as the player with a typed d20 (or in the app when face is 0).
func (p *puzzleTable) tryHint(t *testing.T, u *user, id string, face int32) (*playv1.TryPuzzleHintResponse, error) {
	t.Helper()
	return p.tryHintKey(t, u, id, face, newKey())
}

func (p *puzzleTable) tryHintKey(t *testing.T, u *user, id string, face int32, key string) (*playv1.TryPuzzleHintResponse, error) {
	t.Helper()
	req := &playv1.TryPuzzleHintRequest{CampaignId: p.campaignID, PuzzleId: id, IdempotencyKey: key}
	if face == 0 {
		req.Roll = &playv1.TryPuzzleHintRequest_RollInApp{RollInApp: true}
	} else {
		req.Roll = &playv1.TryPuzzleHintRequest_D20Face{D20Face: face}
	}
	res, err := p.pc(u).TryPuzzleHint(t.Context(), connect.NewRequest(req))
	if err != nil {
		return nil, err
	}
	return res.Msg, nil
}

func hintCheck(skill string, dc int32) func(*playv1.CreatePuzzleRequest) {
	return func(r *playv1.CreatePuzzleRequest) {
		r.Hints = []string{"dica um: o que acompanha você ao meio-dia", "dica dois: some quando a tocha apaga", "dica três: é escura"}
		r.HintCheck = &playv1.PuzzleHintCheck{SkillKey: skill, Dc: dc}
	}
}

// --- the riddle ---

func TestMR038_RiddleCreateAnswerAndSolve(t *testing.T) {
	t.Parallel()
	p := newPuzzleTable(t)
	puz := p.riddle(t, "A porta da Cripta pergunta")
	if puz.GetKind() != playv1.PuzzleKind_PUZZLE_KIND_RIDDLE || len(puz.GetSolution().GetRiddle().GetAnswers()) != 2 || puz.GetConfig().GetRiddle().GetText() != riddleText {
		t.Fatalf("the master reads %v", puz)
	}
	// A player never receives the puzzle before it is shown, nor the answers after.
	_, err := p.pc(p.caio).GetPuzzleRun(t.Context(), connect.NewRequest(&playv1.GetPuzzleRunRequest{CampaignId: p.campaignID, PuzzleId: puz.GetId()}))
	wantCode(t, "GetPuzzleRun before it is shown", err, connect.CodeNotFound)
	p.show(t, puz.GetId())
	read := p.read(t, p.caio, puz.GetId())
	if read.GetKind() != playv1.PuzzleKind_PUZZLE_KIND_RIDDLE || read.GetConfig().GetRiddle().GetText() != riddleText || read.GetClue() == "" {
		t.Fatalf("the player reads %v", read)
	}
	if strings.Contains(jsonOf(read), "sombra") {
		t.Errorf("the player's riddle has an answer: %s", jsonOf(read))
	}

	// A wrong answer: nothing moves but the move itself, which is marked wrong.
	res := p.mustMove(t, p.caio, puz.GetId(), riddleMove("escuridão"))
	if res.GetSolvedByThisMove() || res.GetRun().GetSolved() || !res.GetRun().GetLastMove().GetWrong() || res.GetRun().GetLastMove().GetCharacterName() != "Toren" {
		t.Fatalf("a wrong answer left %v", res)
	}
	if res.GetRun().GetLastMove().GetMove() != nil || strings.Contains(jsonOf(res), "escuridão") {
		t.Errorf("a typed answer reached the players: %s", jsonOf(res))
	}
	m := p.masterRun(t, puz.GetId())
	if got := m.GetLastMove().GetMove().GetRiddle().GetAnswer(); got != "escuridão" || !m.GetLastMove().GetWrong() || m.GetMovesMade() != 1 {
		t.Errorf("the master reads the last move %v (moves %d), want the typed answer", m.GetLastMove(), m.GetMovesMade())
	}
	// A right answer, in the form nobody typed in the master's list: case, accents, the
	// spaces around and the punctuation do not count.
	res = p.mustMove(t, p.ana, puz.GetId(), riddleMove("  A SOMBRA! "))
	if !res.GetSolvedByThisMove() || !res.GetRun().GetSolved() || res.GetRun().GetSolvedByName() != "Pensantus" {
		t.Fatalf("the right answer left %v", res)
	}
	if m := p.masterRun(t, puz.GetId()); m.GetOutcome() != playv1.PuzzleSolveOutcome_PUZZLE_SOLVE_OUTCOME_NOTIFIED || m.GetStatus() != playv1.PuzzleRunStatus_PUZZLE_RUN_STATUS_SOLVED {
		t.Errorf("the master reads %v", m)
	}
	_, err = p.move(t, p.bia, puz.GetId(), riddleMove("sombra"))
	wantPuzzleBlocked(t, "an answer to a solved riddle", err, playv1.PuzzleBlockedReason_PUZZLE_BLOCKED_REASON_SOLVED)
	if n := p.eventCount(t, "puzzle_solved"); n != 1 {
		t.Errorf("%d puzzle_solved events, want 1", n)
	}
	// "Recomeçar" gives the riddle back.
	p.reset(t, puz.GetId())
	if r := p.read(t, p.bia, puz.GetId()); r.GetSolved() || r.GetLastMove() != nil {
		t.Errorf("the reset riddle reads %v", r)
	}
	if res := p.mustMove(t, p.bia, puz.GetId(), riddleMove("Sombra")); !res.GetSolvedByThisMove() {
		t.Errorf("the second round was not solved: %v", res)
	}
}

func TestMR038_RiddleChecksWhatTheMasterWrote(t *testing.T) {
	t.Parallel()
	p := newPuzzleTable(t)
	many := make([]string, 11)
	for i := range many {
		many[i] = strings.Repeat("x", i+1)
	}
	tests := []struct {
		name string
		edit func(*playv1.CreatePuzzleRequest)
		want playv1.PuzzleInvalidReason
	}{
		{"no answers", func(r *playv1.CreatePuzzleRequest) { r.Solution = riddleSolution() }, playv1.PuzzleInvalidReason_PUZZLE_INVALID_REASON_ANSWERS},
		{"eleven answers", func(r *playv1.CreatePuzzleRequest) { r.Solution = riddleSolution(many...) }, playv1.PuzzleInvalidReason_PUZZLE_INVALID_REASON_ANSWERS},
		{"an empty answer", func(r *playv1.CreatePuzzleRequest) { r.Solution = riddleSolution("sombra", "  ") }, playv1.PuzzleInvalidReason_PUZZLE_INVALID_REASON_ANSWERS},
		{"an answer of 81 characters", func(r *playv1.CreatePuzzleRequest) { r.Solution = riddleSolution(strings.Repeat("a", 81)) }, playv1.PuzzleInvalidReason_PUZZLE_INVALID_REASON_ANSWERS},
		{"the same answer twice once folded", func(r *playv1.CreatePuzzleRequest) { r.Solution = riddleSolution("Sombra", "sombra!") }, playv1.PuzzleInvalidReason_PUZZLE_INVALID_REASON_ANSWERS},
		{"no solution", func(r *playv1.CreatePuzzleRequest) { r.Solution = nil }, playv1.PuzzleInvalidReason_PUZZLE_INVALID_REASON_KIND},
		{"a lock's solution", func(r *playv1.CreatePuzzleRequest) { r.Solution = lockSolution(1, 2) }, playv1.PuzzleInvalidReason_PUZZLE_INVALID_REASON_KIND},
		{"an empty riddle", func(r *playv1.CreatePuzzleRequest) {
			r.Config = &playv1.PuzzleConfig{Kind: &playv1.PuzzleConfig_Riddle{Riddle: &playv1.RiddleConfig{Text: " "}}}
		}, playv1.PuzzleInvalidReason_PUZZLE_INVALID_REASON_TEXT},
		{"a riddle of 501 characters", func(r *playv1.CreatePuzzleRequest) {
			r.Config = &playv1.PuzzleConfig{Kind: &playv1.PuzzleConfig_Riddle{Riddle: &playv1.RiddleConfig{Text: strings.Repeat("a", 501)}}}
		}, playv1.PuzzleInvalidReason_PUZZLE_INVALID_REASON_TEXT},
	}
	for _, tt := range tests {
		req := &playv1.CreatePuzzleRequest{CampaignId: p.campaignID, Name: "Enigma", Config: riddleConfig(), Solution: riddleSolution("sombra")}
		tt.edit(req)
		_, err := p.pc(p.master).CreatePuzzle(t.Context(), connect.NewRequest(req))
		wantPuzzleInvalid(t, tt.name, err, tt.want)
	}
	// Ten answers of 80 characters are the most.
	best := make([]string, 10)
	for i := range best {
		best[i] = strings.Repeat(string(rune('a'+i)), 80)
	}
	p.riddle(t, "O enigma mais longo", func(r *playv1.CreatePuzzleRequest) { r.Solution = riddleSolution(best...) })
	puz := p.riddle(t, "O enigma")
	p.show(t, puz.GetId())
	for _, bad := range []string{"", "   ", strings.Repeat("a", 601)} {
		_, err := p.move(t, p.caio, puz.GetId(), riddleMove(bad))
		wantPuzzleInvalid(t, "a typed answer of the wrong length", err, playv1.PuzzleInvalidReason_PUZZLE_INVALID_REASON_MOVE)
	}
	_, err := p.move(t, p.caio, puz.GetId(), cipherMove("sombra"))
	wantPuzzleInvalid(t, "a cipher's move on a riddle", err, playv1.PuzzleInvalidReason_PUZZLE_INVALID_REASON_KIND)
	_, err = p.pc(p.master).MakePuzzleMove(t.Context(), connect.NewRequest(&playv1.MakePuzzleMoveRequest{CampaignId: p.campaignID, PuzzleId: puz.GetId(), Move: riddleMove("sombra"), IdempotencyKey: newKey()}))
	wantCode(t, "an answer by the master", err, connect.CodePermissionDenied)
	// Nothing was made of the refused answers.
	if m := p.masterRun(t, puz.GetId()); m.GetMovesMade() != 0 {
		t.Errorf("%d moves were made by refused answers", m.GetMovesMade())
	}
}

// --- the sequence ---

func TestMR038_SequencePlayedThenRepeatedWithAWrongStep(t *testing.T) {
	t.Parallel()
	p := newPuzzleTable(t)
	clock := p.movableClock()
	puz := p.sequence(t, "Os sinos do Salão do trono")
	if got := puz.GetConfig().GetSequence(); got.GetBells() != 4 || got.GetSteps() != 4 || len(puz.GetSymbols()) != 4 || len(puz.GetSolution().GetSequence().GetSteps()) != 4 {
		t.Fatalf("the master reads %v", puz)
	}
	p.show(t, puz.GetId())

	// Before it is played the players have the shape and no step at all, and cannot strike.
	r := p.read(t, p.caio, puz.GetId())
	if pb := r.GetSequence(); pb.GetTotalSteps() != 4 || pb.GetPlays() != 0 || pb.GetPlaying() || len(pb.GetShown()) != 0 || len(r.GetSymbols()) != 4 {
		t.Fatalf("before the play the player reads %v", r)
	}
	_, err := p.move(t, p.caio, puz.GetId(), bellMove(2))
	wantPuzzleBlocked(t, "a bell before the play", err, playv1.PuzzleBlockedReason_PUZZLE_BLOCKED_REASON_SEQUENCE_NOT_PLAYED)

	// The master plays it: step by step, never a step before its time.
	p.playSequence(t, puz.GetId())
	want := []int32{2, 0, 3, 1}
	for i := range want {
		got := p.read(t, p.ana, puz.GetId()).GetSequence()
		if !got.GetPlaying() || got.GetPlays() != 1 || !equalInt32(got.GetShown(), want[:i+1]) || got.GetNextInMs() <= 0 || got.GetNextInMs() > 1201 || got.GetStepMs() != 1200 {
			t.Fatalf("at step %d the player reads %v, want the first %d steps %v", i+1, got, i+1, want[:i+1])
		}
		_, err = p.move(t, p.ana, puz.GetId(), bellMove(2))
		wantPuzzleBlocked(t, "a bell while it plays", err, playv1.PuzzleBlockedReason_PUZZLE_BLOCKED_REASON_SEQUENCE_PLAYING)
		clock.advance(puzzle.SequenceStep)
	}
	got := p.read(t, p.ana, puz.GetId()).GetSequence()
	if got.GetPlaying() || len(got.GetShown()) != 0 || got.GetNextInMs() != 0 || got.GetPlays() != 1 {
		t.Fatalf("after the play the player reads %v: the sequence is never sent again", got)
	}

	// Repeat it: two right steps, then a wrong one, which sends the attempt back.
	for i, b := range []int32{2, 0} {
		res := p.mustMove(t, p.caio, puz.GetId(), bellMove(b))
		if res.GetRun().GetState().GetSequence().GetProgress() != int32(i+1) || res.GetRun().GetLastMove().GetWrong() {
			t.Fatalf("a right bell left %v", res.GetRun())
		}
	}
	res := p.mustMove(t, p.bia, puz.GetId(), bellMove(0)) // the third step is bell 3
	lm := res.GetRun().GetLastMove()
	if res.GetRun().GetState().GetSequence().GetProgress() != 0 || !lm.GetWrong() || lm.GetStep() != 3 || lm.GetCharacterName() != "Brisa" || lm.GetMove() != nil {
		t.Fatalf("a wrong bell left %v", res.GetRun())
	}
	if m := p.masterRun(t, puz.GetId()); m.GetLastMove().GetMove().GetSequence().GetBell() != 0 || m.GetMinimum().GetMoves() != 4 {
		t.Errorf("the master reads %v", m)
	}
	// The master plays it again; then the whole sequence in order solves it.
	p.playSequence(t, puz.GetId())
	if pb := p.read(t, p.caio, puz.GetId()).GetSequence(); pb.GetPlays() != 2 || !pb.GetPlaying() || len(pb.GetShown()) != 1 {
		t.Errorf("a second play reads %v", pb)
	}
	clock.advance(10 * puzzle.SequenceStep)
	var last *playv1.MakePuzzleMoveResponse
	for _, b := range want {
		last = p.mustMove(t, p.ana, puz.GetId(), bellMove(b))
	}
	if !last.GetSolvedByThisMove() || !last.GetRun().GetSolved() {
		t.Fatalf("the whole sequence did not solve it: %v", last)
	}
	_, err = p.move(t, p.caio, puz.GetId(), bellMove(2))
	wantPuzzleBlocked(t, "a bell after it is solved", err, playv1.PuzzleBlockedReason_PUZZLE_BLOCKED_REASON_SOLVED)
	// Not a sequence, not shown, a player: refused.
	other := p.riddle(t, "Outro")
	p.show(t, other.GetId())
	_, err = p.pc(p.master).PlayPuzzleSequence(t.Context(), connect.NewRequest(&playv1.PlayPuzzleSequenceRequest{CampaignId: p.campaignID, PuzzleId: other.GetId()}))
	wantPuzzleBlocked(t, "playing a riddle", err, playv1.PuzzleBlockedReason_PUZZLE_BLOCKED_REASON_NOT_A_SEQUENCE)
	_, err = p.pc(p.caio).PlayPuzzleSequence(t.Context(), connect.NewRequest(&playv1.PlayPuzzleSequenceRequest{CampaignId: p.campaignID, PuzzleId: puz.GetId()}))
	wantCode(t, "PlayPuzzleSequence by a player", err, connect.CodePermissionDenied)
	_, err = p.pc(p.master).PlayPuzzleSequence(t.Context(), connect.NewRequest(&playv1.PlayPuzzleSequenceRequest{CampaignId: p.campaignID, PuzzleId: puz.GetId()}))
	wantPuzzleBlocked(t, "playing a solved sequence", err, playv1.PuzzleBlockedReason_PUZZLE_BLOCKED_REASON_SOLVED)
}

func equalInt32(a, b []int32) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}

func TestMR038_SequenceChecksWhatTheMasterWrote(t *testing.T) {
	t.Parallel()
	p := newPuzzleTable(t)
	tests := []struct {
		name string
		cfg  *playv1.PuzzleConfig
		sol  *playv1.PuzzleSolution
		want playv1.PuzzleInvalidReason
	}{
		{"two bells", sequenceConfig(2), sequenceSolution(0, 1, 0), playv1.PuzzleInvalidReason_PUZZLE_INVALID_REASON_SIZE},
		{"nine bells", sequenceConfig(9), sequenceSolution(0, 1, 0), playv1.PuzzleInvalidReason_PUZZLE_INVALID_REASON_SIZE},
		{"two steps", sequenceConfig(4), sequenceSolution(0, 1), playv1.PuzzleInvalidReason_PUZZLE_INVALID_REASON_SIZE},
		{"thirteen steps", sequenceConfig(4), sequenceSolution(0, 1, 0, 1, 0, 1, 0, 1, 0, 1, 0, 1, 0), playv1.PuzzleInvalidReason_PUZZLE_INVALID_REASON_SIZE},
		{"a bell that is not there", sequenceConfig(4), sequenceSolution(0, 1, 4), playv1.PuzzleInvalidReason_PUZZLE_INVALID_REASON_SYMBOLS},
		{"one bell over and over", sequenceConfig(4), sequenceSolution(1, 1, 1), playv1.PuzzleInvalidReason_PUZZLE_INVALID_REASON_SYMBOLS},
		{"no steps", sequenceConfig(4), nil, playv1.PuzzleInvalidReason_PUZZLE_INVALID_REASON_KIND},
	}
	for _, tt := range tests {
		_, err := p.pc(p.master).CreatePuzzle(t.Context(), connect.NewRequest(&playv1.CreatePuzzleRequest{CampaignId: p.campaignID, Name: "Sinos", Config: tt.cfg, Solution: tt.sol}))
		wantPuzzleInvalid(t, tt.name, err, tt.want)
	}
	// Three to eight bells and three to twelve steps are fine, and the number of steps
	// is the solution's whatever the request said.
	big := p.create(t, &playv1.CreatePuzzleRequest{
		Name: "Sinos", Config: &playv1.PuzzleConfig{Kind: &playv1.PuzzleConfig_Sequence{Sequence: &playv1.SequenceConfig{Bells: 8, Steps: 99}}},
		Solution: sequenceSolution(7, 0, 1, 2, 3, 4, 5, 6, 7, 0, 1, 2),
	})
	if big.GetConfig().GetSequence().GetSteps() != 12 || len(big.GetSymbols()) != 8 {
		t.Errorf("the biggest sequence reads %v", big)
	}
	puz := p.sequence(t, "Os sinos")
	p.show(t, puz.GetId())
	p.playSequence(t, puz.GetId())
	p.movableClock().advance(time.Minute)
	_, err := p.move(t, p.caio, puz.GetId(), bellMove(4))
	wantPuzzleInvalid(t, "a bell that is not there", err, playv1.PuzzleInvalidReason_PUZZLE_INVALID_REASON_MOVE)
	_, err = p.move(t, p.caio, puz.GetId(), bellMove(-1))
	wantPuzzleInvalid(t, "a negative bell", err, playv1.PuzzleInvalidReason_PUZZLE_INVALID_REASON_MOVE)
}

// A play schedules the reads of its steps: the stream says puzzle_changed now and at the
// end of each step, so the apps read the next one on time.
func TestMR038_APlayScheduleItsSteps(t *testing.T) {
	t.Parallel()
	p := newPuzzleTable(t)
	gc := &gateClock{t: time.Now()}
	p.h.svc.puzzles.hints.now, p.h.svc.puzzles.hints.after = gc.now, gc.after
	puz := p.sequence(t, "Os sinos")
	p.show(t, puz.GetId())
	gc.timers = nil
	p.playSequence(t, puz.GetId())
	var at []time.Duration
	for _, tm := range gc.timers {
		at = append(at, tm.at.Sub(gc.t))
	}
	for i := 1; i <= 4; i++ {
		found := false
		for _, d := range at {
			found = found || d == time.Duration(i)*puzzle.SequenceStep
		}
		if !found {
			t.Errorf("no hint scheduled at step %d: %v", i, at)
		}
	}
}

// --- the cipher ---

func TestMR038_CipherRoundTripAndTheKeyAsAClue(t *testing.T) {
	t.Parallel()
	p := newPuzzleTable(t)
	x, y := sq(6, 6)
	pt, err := p.mc(p.master).CreateMapPoint(t.Context(), connect.NewRequest(&mapsv1.CreateMapPointRequest{
		CampaignId: p.campaignID, MapId: p.mapID, Kind: mapsv1.MapPointKind_MAP_POINT_KIND_SCENE, Name: "A biblioteca", XBp: x, YBp: y,
	}))
	if err != nil {
		t.Fatal(err)
	}
	clue, err := p.mc(p.master).AddSceneClue(t.Context(), connect.NewRequest(&mapsv1.AddSceneClueRequest{
		CampaignId: p.campaignID, MapId: p.mapID, PointId: pt.Msg.GetPoint().GetId(), Text: "Cada letra anda três para trás.",
	}))
	if err != nil {
		t.Fatal(err)
	}
	clueID := clue.Msg.GetClue().GetId()
	puz := p.cipher(t, "A carta do Capitão", func(r *playv1.CreatePuzzleRequest) { r.Config = cipherConfig(clueID) })
	if puz.GetConfig().GetCipher().GetCiphertext() != letterC || puz.GetConfig().GetCipher().GetKeyClueId() != clueID || puz.GetSolution().GetCipher().GetMessage() != letter {
		t.Fatalf("the master reads %v", puz)
	}
	// What the master sees in the form is what the players will read.
	prev, err := p.pc(p.master).PreviewPuzzleCipher(t.Context(), connect.NewRequest(&playv1.PreviewPuzzleCipherRequest{CampaignId: p.campaignID, Solution: puz.GetSolution().GetCipher()}))
	if err != nil || prev.Msg.GetCiphertext() != letterC {
		t.Fatalf("PreviewPuzzleCipher() = %v, %v", prev, err)
	}
	p.show(t, puz.GetId())

	read := p.read(t, p.caio, puz.GetId())
	if read.GetConfig().GetCipher().GetCiphertext() != letterC || read.GetConfig().GetCipher().GetKeyClueId() != "" || !read.GetHasKeyClue() || read.GetKeyClueId() != "" {
		t.Fatalf("the player reads %v", read)
	}
	if j := jsonOf(read); strings.Contains(j, clueID) || strings.Contains(strings.ToLower(j), "tesouro") || strings.Contains(j, "shift") {
		t.Errorf("the player's cipher has the key or the plain message: %s", j)
	}
	// The group finds the key in the adventure: only the players that have the clue read where it is.
	if _, err := p.mc(p.master).RevealSceneClue(t.Context(), connect.NewRequest(&mapsv1.RevealSceneClueRequest{CampaignId: p.campaignID, ClueId: clueID, CharacterIds: []string{p.pens.GetId()}})); err != nil {
		t.Fatal(err)
	}
	if got := p.read(t, p.ana, puz.GetId()); got.GetKeyClueId() != clueID || got.GetConfig().GetCipher().GetKeyClueId() != "" {
		t.Errorf("the player that found the clue reads %v", got)
	}
	if got := p.read(t, p.caio, puz.GetId()); got.GetKeyClueId() != "" || !got.GetHasKeyClue() {
		t.Errorf("a player that did not find it reads %v", got)
	}
	// A near miss is wrong; the plain message in any case, accents or punctuation solves.
	res := p.mustMove(t, p.bia, puz.GetId(), cipherMove("o tesouro esta sobre o altar"))
	if res.GetSolvedByThisMove() || !res.GetRun().GetLastMove().GetWrong() || strings.Contains(jsonOf(res), "sobre") {
		t.Fatalf("a wrong message left %v", res)
	}
	res = p.mustMove(t, p.bia, puz.GetId(), cipherMove("O TESOURO ESTA SOB O ALTAR."))
	if !res.GetSolvedByThisMove() {
		t.Fatalf("the plain message did not solve it: %v", res)
	}
	// The key must be a clue of the campaign, and a keyword makes a keyed alphabet.
	_, err = p.pc(p.master).CreatePuzzle(t.Context(), connect.NewRequest(&playv1.CreatePuzzleRequest{
		CampaignId: p.campaignID, Name: "Carta", Config: cipherConfig(newKey()), Solution: cipherSolution(letter, 3),
	}))
	wantPuzzleInvalid(t, "a key clue that does not exist", err, playv1.PuzzleInvalidReason_PUZZLE_INVALID_REASON_CIPHER)
	_, err = p.pc(p.master).CreatePuzzle(t.Context(), connect.NewRequest(&playv1.CreatePuzzleRequest{
		CampaignId: p.campaignID, Name: "Carta", Config: cipherConfig("não é uuid"), Solution: cipherSolution(letter, 3),
	}))
	wantPuzzleInvalid(t, "a key clue that is not a UUID", err, playv1.PuzzleInvalidReason_PUZZLE_INVALID_REASON_CIPHER)
	keyed := p.create(t, &playv1.CreatePuzzleRequest{Name: "Carta com palavra", Config: cipherConfig(""), Solution: &playv1.PuzzleSolution{Kind: &playv1.PuzzleSolution_Cipher{
		Cipher: &playv1.CipherSolution{Message: "abcde", Method: &playv1.CipherSolution_Keyword{Keyword: "lua"}},
	}}})
	if keyed.GetConfig().GetCipher().GetCiphertext() != "LUABC" {
		t.Errorf("the keyed cipher reads %q, want LUABC", keyed.GetConfig().GetCipher().GetCiphertext())
	}
}

func TestMR038_CipherChecksWhatTheMasterWrote(t *testing.T) {
	t.Parallel()
	p := newPuzzleTable(t)
	invalid := playv1.PuzzleInvalidReason_PUZZLE_INVALID_REASON_CIPHER
	solutions := map[string]*playv1.PuzzleSolution{
		"shift 0":        {Kind: &playv1.PuzzleSolution_Cipher{Cipher: &playv1.CipherSolution{Message: letter, Method: &playv1.CipherSolution_Shift{Shift: 0}}}},
		"shift 26":       cipherSolution(letter, 26),
		"no key":         {Kind: &playv1.PuzzleSolution_Cipher{Cipher: &playv1.CipherSolution{Message: letter}}},
		"a short word":   {Kind: &playv1.PuzzleSolution_Cipher{Cipher: &playv1.CipherSolution{Message: letter, Method: &playv1.CipherSolution_Keyword{Keyword: "lu"}}}},
		"no letter":      cipherSolution("123 !?", 3),
		"an empty text":  cipherSolution("  ", 3),
		"301 characters": cipherSolution(strings.Repeat("a", 301), 3),
	}
	for name, sol := range solutions {
		_, err := p.pc(p.master).CreatePuzzle(t.Context(), connect.NewRequest(&playv1.CreatePuzzleRequest{CampaignId: p.campaignID, Name: "Carta", Config: cipherConfig(""), Solution: sol}))
		wantPuzzleInvalid(t, name, err, invalid)
	}
	_, err := p.pc(p.master).PreviewPuzzleCipher(t.Context(), connect.NewRequest(&playv1.PreviewPuzzleCipherRequest{CampaignId: p.campaignID, Solution: solutions["shift 0"].GetCipher()}))
	wantPuzzleInvalid(t, "preview of a bad key", err, invalid)
	_, err = p.pc(p.caio).PreviewPuzzleCipher(t.Context(), connect.NewRequest(&playv1.PreviewPuzzleCipherRequest{CampaignId: p.campaignID, Solution: cipherSolution(letter, 3).GetCipher()}))
	wantCode(t, "preview by a player", err, connect.CodePermissionDenied)
	puz := p.cipher(t, "Carta")
	p.show(t, puz.GetId())
	_, err = p.move(t, p.caio, puz.GetId(), cipherMove(""))
	wantPuzzleInvalid(t, "an empty message", err, playv1.PuzzleInvalidReason_PUZZLE_INVALID_REASON_MOVE)
}

// --- hints won by a skill check ---

func TestMR038_AHintWonBySkillCheckIsOnlyThePlayers(t *testing.T) {
	t.Parallel()
	p := newPuzzleTable(t)
	puz := p.riddle(t, "A porta da Cripta pergunta", hintCheck("skill:investigation", 15))
	if puz.GetHintCheck().GetDc() != 15 || puz.GetHintCheck().GetSkillKey() != "skill:investigation" {
		t.Fatalf("the master reads %v", puz.GetHintCheck())
	}
	// Before it is shown, a try finds nothing.
	_, err := p.tryHint(t, p.caio, puz.GetId(), 20)
	wantCode(t, "a try before the puzzle is shown", err, connect.CodeNotFound)
	p.show(t, puz.GetId())
	r := p.read(t, p.caio, puz.GetId())
	if !r.GetHintByCheck() || r.GetHintSkillKey() != "skill:investigation" || !r.GetCanTryHint() || len(r.GetHints()) != 0 {
		t.Fatalf("the player reads %v", r)
	}
	if dcKey.MatchString(jsonOf(r)) {
		t.Errorf("the player's JSON has the DC: %s", jsonOf(r))
	}

	// A pass (a real die: 20 plus the bonus reaches 15): the next hint, for that player only.
	key := newKey()
	res, err := p.tryHintKey(t, p.caio, puz.GetId(), 20, key)
	if err != nil {
		t.Fatal(err)
	}
	if !res.GetPassed() || res.GetReplayed() || !res.GetRoll().GetPhysical() || res.GetRoll().GetFaces()[0] != 20 || res.GetRun().GetSharedHints() != 0 ||
		len(res.GetRun().GetHints()) != 1 || res.GetRun().GetHints()[0] != puz.GetHints()[0] {
		t.Fatalf("a pass left %v", res)
	}
	if got := p.read(t, p.ana, puz.GetId()); len(got.GetHints()) != 0 || !got.GetCanTryHint() {
		t.Errorf("another player reads %v: the hint is only the winner's", got)
	}
	if got := p.read(t, p.caio, puz.GetId()); len(got.GetHints()) != 1 || !got.GetCanTryHint() {
		t.Errorf("the winner reads %v", got)
	}
	// The same key again: the first answer, no new try.
	again, err := p.tryHintKey(t, p.caio, puz.GetId(), 20, key)
	if err != nil || !again.GetReplayed() || !again.GetPassed() {
		t.Fatalf("a retry = %v, %v", again, err)
	}
	if m := p.masterRun(t, puz.GetId()); len(m.GetHintTries()) != 1 || m.GetHintTries()[0].GetCharacterName() != "Toren" || !m.GetHintTries()[0].GetPassed() ||
		m.GetHintTries()[0].GetHint() != 1 || m.GetReleasedHints() != 0 {
		t.Errorf("the master reads %v", m.GetHintTries())
	}
	// The next hint: a fail (a 1 never reaches 15 here), and no second try for the same hint.
	res, err = p.tryHint(t, p.caio, puz.GetId(), 1)
	if err != nil || res.GetPassed() || len(res.GetRun().GetHints()) != 1 || res.GetRun().GetCanTryHint() {
		t.Fatalf("a fail = %v, %v", res, err)
	}
	_, err = p.tryHint(t, p.caio, puz.GetId(), 20)
	wantPuzzleBlocked(t, "a second try for the same hint", err, playv1.PuzzleBlockedReason_PUZZLE_BLOCKED_REASON_HINT_ALREADY_TRIED)
	// Another player may try for it, and the master releasing the hint lets Toren try again.
	if res, err = p.tryHint(t, p.ana, puz.GetId(), 20); err != nil || !res.GetPassed() || len(res.GetRun().GetHints()) != 1 {
		t.Fatalf("Pensantus's try = %v, %v", res, err)
	}
	p.release(t, puz.GetId())
	p.release(t, puz.GetId()) // hints 1 and 2 are shared now: Toren read hint 1 already and hint 2 is shared
	if got := p.read(t, p.caio, puz.GetId()); len(got.GetHints()) != 2 || got.GetSharedHints() != 2 || !got.GetCanTryHint() {
		t.Errorf("after the master released two hints Toren reads %v", got)
	}
	if res, err = p.tryHint(t, p.caio, puz.GetId(), 20); err != nil || !res.GetPassed() || len(res.GetRun().GetHints()) != 3 {
		t.Fatalf("Toren's third try = %v, %v", res, err)
	}
	// No hint is left for him.
	_, err = p.tryHint(t, p.caio, puz.GetId(), 20)
	wantPuzzleBlocked(t, "a try with no hint left", err, playv1.PuzzleBlockedReason_PUZZLE_BLOCKED_REASON_NO_MORE_HINTS)
	if got := p.read(t, p.caio, puz.GetId()); got.GetCanTryHint() {
		t.Errorf("Toren can try with no hint left: %v", got)
	}
	// The master cannot try, and a puzzle without a check refuses.
	_, err = p.pc(p.master).TryPuzzleHint(t.Context(), connect.NewRequest(&playv1.TryPuzzleHintRequest{CampaignId: p.campaignID, PuzzleId: puz.GetId(), IdempotencyKey: newKey(), Roll: &playv1.TryPuzzleHintRequest_RollInApp{RollInApp: true}}))
	wantCode(t, "a try by the master", err, connect.CodePermissionDenied)
	plain := p.riddle(t, "Sem teste", func(r *playv1.CreatePuzzleRequest) { r.Hints = []string{"dica"} })
	p.show(t, plain.GetId())
	_, err = p.tryHint(t, p.caio, plain.GetId(), 20)
	wantPuzzleBlocked(t, "a try with no check", err, playv1.PuzzleBlockedReason_PUZZLE_BLOCKED_REASON_NO_HINT_CHECK)
	if r := p.read(t, p.caio, plain.GetId()); r.GetHintByCheck() || r.GetCanTryHint() {
		t.Errorf("a puzzle with no check reads %v", r)
	}
	// A solved puzzle gives no more hints.
	p.mustMove(t, p.bia, puz.GetId(), riddleMove("sombra"))
	_, err = p.tryHint(t, p.bia, puz.GetId(), 20)
	wantPuzzleBlocked(t, "a try at a solved puzzle", err, playv1.PuzzleBlockedReason_PUZZLE_BLOCKED_REASON_SOLVED)
}

func TestMR038_AHintByAppDiceAndTheDiceMode(t *testing.T) {
	t.Parallel()
	p := newPuzzleTable(t)
	puz := p.lock(t, "Cofre", hintCheck("skill:arcana", 12))
	p.show(t, puz.GetId())
	// In the app the server rolls: 16 plus the bonus passes DC 12, a 2 fails.
	p.h.roller.queue(16, 2)
	res, err := p.tryHint(t, p.ana, puz.GetId(), 0)
	if err != nil || !res.GetPassed() || res.GetRoll().GetPhysical() || res.GetRoll().GetFaces()[0] != 16 || res.GetRoll().GetTotal() < 16 {
		t.Fatalf("an app roll of 16 = %v, %v", res, err)
	}
	res, err = p.tryHint(t, p.ana, puz.GetId(), 0)
	if err != nil || res.GetPassed() || res.GetRoll().GetFaces()[0] != 2 || len(res.GetRun().GetHints()) != 1 {
		t.Fatalf("an app roll of 2 = %v, %v", res, err)
	}
	// RN-18: a mode the master forced binds the player.
	p.forceDice(t, campaignsv1.DiceMode_DICE_MODE_PHYSICAL)
	_, err = p.tryHint(t, p.caio, puz.GetId(), 0)
	wantPuzzleBlocked(t, "an app roll when everybody rolls real dice", err, playv1.PuzzleBlockedReason_PUZZLE_BLOCKED_REASON_WRONG_DICE_MODE)
	if res, err = p.tryHint(t, p.caio, puz.GetId(), 19); err != nil || !res.GetRoll().GetPhysical() {
		t.Fatalf("a real roll when everybody rolls real dice = %v, %v", res, err)
	}
	p.forceDice(t, campaignsv1.DiceMode_DICE_MODE_APP)
	_, err = p.tryHint(t, p.bia, puz.GetId(), 19)
	wantPuzzleBlocked(t, "a typed roll when everybody rolls in the app", err, playv1.PuzzleBlockedReason_PUZZLE_BLOCKED_REASON_WRONG_DICE_MODE)
	// A bad die, and no roll at all.
	for _, bad := range []int32{-1, 21} {
		_, err = p.tryHint(t, p.bia, puz.GetId(), bad)
		wantCode(t, "a die out of 1 to 20", err, connect.CodeInvalidArgument)
	}
	_, err = p.pc(p.bia).TryPuzzleHint(t.Context(), connect.NewRequest(&playv1.TryPuzzleHintRequest{CampaignId: p.campaignID, PuzzleId: puz.GetId(), IdempotencyKey: newKey()}))
	wantCode(t, "a try with no roll", err, connect.CodeInvalidArgument)
	_, err = p.tryHintKey(t, p.bia, puz.GetId(), 0, "não é uuid")
	wantCode(t, "a try with a bad key", err, connect.CodeInvalidArgument)
}

func TestMR038_HintChecksWhatTheMasterWrote(t *testing.T) {
	t.Parallel()
	p := newPuzzleTable(t)
	bad := playv1.PuzzleInvalidReason_PUZZLE_INVALID_REASON_HINT_CHECK
	tests := []struct {
		name string
		edit func(*playv1.CreatePuzzleRequest)
	}{
		{"an unknown skill", hintCheck("skill:cooking", 12)},
		{"an ability, not a skill", hintCheck("ability:int", 12)},
		{"no skill", hintCheck("", 12)},
		{"DC 0", hintCheck("skill:arcana", 0)},
		{"DC 31", hintCheck("skill:arcana", 31)},
		{"no hints to win", func(r *playv1.CreatePuzzleRequest) {
			r.HintCheck = &playv1.PuzzleHintCheck{SkillKey: "skill:arcana", Dc: 12}
		}},
	}
	for _, tt := range tests {
		req := &playv1.CreatePuzzleRequest{CampaignId: p.campaignID, Name: "Enigma", Config: riddleConfig(), Solution: riddleSolution("sombra")}
		tt.edit(req)
		_, err := p.pc(p.master).CreatePuzzle(t.Context(), connect.NewRequest(req))
		wantPuzzleInvalid(t, tt.name, err, bad)
	}
	for _, dc := range []int32{1, 30} {
		p.riddle(t, "Enigma", hintCheck("skill:perception", dc))
	}
}

// --- the split information ---

func TestMR038_SplitInformationEachPlayerReadsOnlyTheirPart(t *testing.T) {
	t.Parallel()
	p := newPuzzleTable(t)
	parts := []*playv1.PuzzlePart{
		{CharacterId: p.toren.GetId(), Text: "A porta ouve o que o chão esconde…"},
		{CharacterId: p.pens.GetId(), Text: "…os tambores ecoam três vezes antes de a porta ceder."},
		{Text: "…só a resposta certa a faz calar."},
	}
	puz := p.riddle(t, "A porta da Cripta pergunta", func(r *playv1.CreatePuzzleRequest) { r.Parts = parts })
	if len(puz.GetParts()) != 3 || puz.GetParts()[2].GetCharacterId() != "" {
		t.Fatalf("the master reads %v", puz.GetParts())
	}
	p.show(t, puz.GetId())

	toren, pens, brisa := p.read(t, p.caio, puz.GetId()), p.read(t, p.ana, puz.GetId()), p.read(t, p.bia, puz.GetId())
	if toren.GetMyPart() != parts[0].GetText() || len(toren.GetPartHolders()) != 1 || toren.GetPartHolders()[0] != "Pensantus" {
		t.Errorf("Toren reads %v", toren)
	}
	if pens.GetMyPart() != parts[1].GetText() || len(pens.GetPartHolders()) != 1 || pens.GetPartHolders()[0] != "Toren" {
		t.Errorf("Pensantus reads %v", pens)
	}
	if brisa.GetMyPart() != "" || len(brisa.GetPartHolders()) != 2 {
		t.Errorf("Brisa, with no part, reads %v", brisa)
	}
	for who, run := range map[string]*playv1.PuzzleRun{"Toren": toren, "Pensantus": pens, "Brisa": brisa} {
		j := jsonOf(run)
		for i, part := range parts {
			own := (who == "Toren" && i == 0) || (who == "Pensantus" && i == 1)
			if strings.Contains(j, part.GetText()) != own {
				t.Errorf("%s's JSON has part %d = %v, want %v: %s", who, i, !own, own, j)
			}
		}
		if strings.Contains(j, "characterId") || strings.Contains(j, "SEM-DONO") {
			t.Errorf("%s's JSON has an owner: %s", who, j)
		}
	}
	// The move's answer reads the part as the player's own, too (the same builder).
	res := p.mustMove(t, p.ana, puz.GetId(), riddleMove("nada"))
	if res.GetRun().GetMyPart() != parts[1].GetText() {
		t.Errorf("the answer to a move reads %v", res.GetRun())
	}
	if m := p.masterRun(t, puz.GetId()); m.GetRun().GetMyPart() != "" || len(m.GetPuzzle().GetParts()) != 3 {
		t.Errorf("the master's players' copy has a part: %v", m.GetRun())
	}
	// The master's own read of GetPuzzleRun is a player's: no part.
	if got := p.read(t, p.master, puz.GetId()); got.GetMyPart() != "" || len(got.GetPartHolders()) != 0 {
		t.Errorf("the master reads as a player: %v", got)
	}
}

func TestMR038_SplitInformationChecksWhoGetsAPart(t *testing.T) {
	t.Parallel()
	p := newPuzzleTable(t)
	// A member still pending has a character that is not in the party: no part for it,
	// and the puzzle is not theirs to read.
	late := p.h.newUser("Lia")
	p.h.joinPending(p.master, p.campaignID, late)
	pending := late.hero(t, p.campaignID, "Lia", "class:wizard", "race:human", 1, &rulesv1.AbilityScores{Strength: 8, Dexterity: 14, Constitution: 12, Intelligence: 16, Wisdom: 10, Charisma: 10}, nil, []string{fireBolt})
	invalid := playv1.PuzzleInvalidReason_PUZZLE_INVALID_REASON_PARTS
	part := func(id, text string) *playv1.PuzzlePart { return &playv1.PuzzlePart{CharacterId: id, Text: text} }
	tests := []struct {
		name  string
		parts []*playv1.PuzzlePart
	}{
		{"a pending member's character", []*playv1.PuzzlePart{part(pending.GetId(), "parte")}},
		{"an NPC", []*playv1.PuzzlePart{part(p.goblin.GetId(), "parte")}},
		{"a character that is not there", []*playv1.PuzzlePart{part(newKey(), "parte")}},
		{"not a UUID", []*playv1.PuzzlePart{part("Toren", "parte")}},
		{"a character twice", []*playv1.PuzzlePart{part(p.toren.GetId(), "um"), part(p.toren.GetId(), "dois")}},
		{"an empty part", []*playv1.PuzzlePart{part(p.toren.GetId(), "  ")}},
		{"a part of 301 characters", []*playv1.PuzzlePart{part(p.toren.GetId(), strings.Repeat("a", 301))}},
		{"nine parts", []*playv1.PuzzlePart{part("", "1"), part("", "2"), part("", "3"), part("", "4"), part("", "5"), part("", "6"), part("", "7"), part("", "8"), part("", "9")}},
	}
	for _, tt := range tests {
		_, err := p.pc(p.master).CreatePuzzle(t.Context(), connect.NewRequest(&playv1.CreatePuzzleRequest{
			CampaignId: p.campaignID, Name: "Enigma", Config: riddleConfig(), Solution: riddleSolution("sombra"), Parts: tt.parts,
		}))
		wantPuzzleInvalid(t, tt.name, err, invalid)
	}
	// Eight parts are the most, and three with no owner are fine.
	eight := make([]*playv1.PuzzlePart, 8)
	for i := range eight {
		eight[i] = part("", strings.Repeat("a", 300))
	}
	puz := p.riddle(t, "Enigma", func(r *playv1.CreatePuzzleRequest) { r.Parts = eight })
	p.show(t, puz.GetId())
	// The pending member gets nothing: not the puzzle, not the list, not a move.
	_, err := p.pc(late).GetPuzzleRun(t.Context(), connect.NewRequest(&playv1.GetPuzzleRunRequest{CampaignId: p.campaignID, PuzzleId: puz.GetId()}))
	wantCode(t, "GetPuzzleRun by a pending member", err, connect.CodeNotFound)
	_, err = p.pc(late).ListShownPuzzles(t.Context(), connect.NewRequest(&playv1.ListShownPuzzlesRequest{CampaignId: p.campaignID}))
	wantCode(t, "ListShownPuzzles by a pending member", err, connect.CodeNotFound)
	_, err = p.pc(late).MakePuzzleMove(t.Context(), connect.NewRequest(&playv1.MakePuzzleMoveRequest{CampaignId: p.campaignID, PuzzleId: puz.GetId(), Move: riddleMove("sombra"), IdempotencyKey: newKey()}))
	wantCode(t, "MakePuzzleMove by a pending member", err, connect.CodeNotFound)
	_, err = p.pc(late).TryPuzzleHint(t.Context(), connect.NewRequest(&playv1.TryPuzzleHintRequest{CampaignId: p.campaignID, PuzzleId: puz.GetId(), IdempotencyKey: newKey(), Roll: &playv1.TryPuzzleHintRequest_D20Face{D20Face: 10}}))
	wantCode(t, "TryPuzzleHint by a pending member", err, connect.CodeNotFound)
	// The part's owner leaving the party (dying) leaves it with no reader, and no error.
	owned := p.riddle(t, "Dono", func(r *playv1.CreatePuzzleRequest) {
		r.Parts = []*playv1.PuzzlePart{part(p.toren.GetId(), "só do Toren")}
	})
	p.show(t, owned.GetId())
	if _, err := p.h.pool.Exec(t.Context(), `UPDATE characters SET status = 'dead', died_at = now() WHERE id = $1`, p.toren.GetId()); err != nil {
		t.Fatalf("kill the character: %v", err)
	}
	if got := p.read(t, p.ana, owned.GetId()); len(got.GetPartHolders()) != 0 || strings.Contains(jsonOf(got), "só do Toren") {
		t.Errorf("a part of a dead character reads %v", got)
	}
}

// --- "Ao errar": the trap ---

func TestMR038_AWrongMoveFiresTheTrapOnce(t *testing.T) {
	t.Parallel()
	p := newPuzzleTable(t)
	trap := p.trapPoint(t, "Dardos envenenados", 12, 4)
	target := &playv1.PuzzleTrapTarget{MapId: p.mapID, PointId: trap.GetId()}
	seq := p.sequence(t, "Os sinos do Salão do trono", onWrong(onWrongTrap(target)))
	if seq.GetOnWrong().GetTrap().GetPointId() != trap.GetId() {
		t.Fatalf("the master reads %v", seq.GetOnWrong())
	}
	p.show(t, seq.GetId())
	p.playSequence(t, seq.GetId())
	p.movableClock().advance(time.Minute)

	// A right bell fires nothing.
	p.mustMove(t, p.caio, seq.GetId(), bellMove(2))
	if st := p.trapState(t, trap.GetId()); st == "triggered" || p.eventCount(t, "trap_triggered") != 0 {
		t.Fatalf("a right bell fired the trap: %s", st)
	}
	// The wrong one does, in its own transaction, with the move's key: a retry fires nothing again.
	key := newKey()
	res, err := p.moveKey(t, p.ana, seq.GetId(), bellMove(3), key)
	if err != nil {
		t.Fatal(err)
	}
	lm := res.GetRun().GetLastMove()
	if !lm.GetWrong() || lm.GetStep() != 2 || lm.GetTrapName() != "Dardos envenenados" {
		t.Fatalf("a wrong bell left %v", lm)
	}
	if st := p.trapState(t, trap.GetId()); st != "triggered" || p.eventCount(t, "trap_triggered") != 1 {
		t.Fatalf("the trap is %q after %d firings, want triggered once", st, p.eventCount(t, "trap_triggered"))
	}
	if again, err := p.moveKey(t, p.ana, seq.GetId(), bellMove(3), key); err != nil || !again.GetReplayed() {
		t.Fatalf("a retry = %v, %v", again, err)
	}
	// The point fires once: the next wrong move finds it fired, and the move goes on.
	res = p.mustMove(t, p.caio, seq.GetId(), bellMove(1))
	if !res.GetRun().GetLastMove().GetWrong() || res.GetRun().GetLastMove().GetTrapName() != "" {
		t.Errorf("a wrong bell at a trap that fired reads %v", res.GetRun().GetLastMove())
	}
	if n := p.eventCount(t, "trap_triggered"); n != 1 {
		t.Errorf("%d trap_triggered events, want 1", n)
	}
	// The trap is public once it fired, and nothing of it but the name reaches the players.
	if j := jsonOf(p.read(t, p.bia, seq.GetId())); strings.Contains(j, trap.GetId()) || strings.Contains(j, p.mapID) {
		t.Errorf("a player's run has the trap's ids: %s", j)
	}
	// The master arms it again, and it fires again.
	if _, err := p.mc(p.master).UpdateMapPoint(t.Context(), connect.NewRequest(&mapsv1.UpdateMapPointRequest{
		CampaignId: p.campaignID, MapId: p.mapID, PointId: trap.GetId(), Trap: func() *mapsv1.TrapSpec {
			spec := aTrap()
			spec.State = mapsv1.TrapState_TRAP_STATE_ARMED
			return spec
		}(),
	})); err != nil {
		t.Fatalf("UpdateMapPoint(re-arm) error = %v", err)
	}
	if st := p.trapState(t, trap.GetId()); st != "armed" {
		t.Fatalf("the trap is %q after the master armed it again", st)
	}
	if res = p.mustMove(t, p.bia, seq.GetId(), bellMove(1)); res.GetRun().GetLastMove().GetTrapName() == "" {
		t.Errorf("a re-armed trap did not fire: %v", res.GetRun().GetLastMove())
	}
	// A riddle works the same, and a wrong answer to it counts.
	trap2 := p.trapPoint(t, "Fosso", 14, 4)
	rid := p.riddle(t, "Enigma", onWrong(onWrongTrap(&playv1.PuzzleTrapTarget{MapId: p.mapID, PointId: trap2.GetId()})))
	p.show(t, rid.GetId())
	if res = p.mustMove(t, p.caio, rid.GetId(), riddleMove("luz")); res.GetRun().GetLastMove().GetTrapName() != "Fosso" {
		t.Errorf("a wrong answer left %v", res.GetRun().GetLastMove())
	}
	// A trap that was deleted meanwhile is not an error: the move goes on.
	trap3 := p.trapPoint(t, "Mola", 15, 4)
	gone := p.riddle(t, "Enigma 3", onWrong(onWrongTrap(&playv1.PuzzleTrapTarget{MapId: p.mapID, PointId: trap3.GetId()})))
	p.show(t, gone.GetId())
	if _, err := p.mc(p.master).DeleteMapPoint(t.Context(), connect.NewRequest(&mapsv1.DeleteMapPointRequest{CampaignId: p.campaignID, MapId: p.mapID, PointId: trap3.GetId()})); err != nil {
		t.Fatal(err)
	}
	if res = p.mustMove(t, p.caio, gone.GetId(), riddleMove("luz")); !res.GetRun().GetLastMove().GetWrong() || res.GetRun().GetLastMove().GetTrapName() != "" {
		t.Errorf("a wrong answer at a deleted trap left %v", res.GetRun().GetLastMove())
	}
}

func TestMR038_OnWrongChecksWhatTheMasterWrote(t *testing.T) {
	t.Parallel()
	p := newPuzzleTable(t)
	trap := p.trapPoint(t, "Dardos", 12, 4)
	scene, err := p.mc(p.master).CreateMapPoint(t.Context(), connect.NewRequest(&mapsv1.CreateMapPointRequest{
		CampaignId: p.campaignID, MapId: p.mapID, Kind: mapsv1.MapPointKind_MAP_POINT_KIND_SCENE, Name: "Cena", XBp: 100, YBp: 100,
	}))
	if err != nil {
		t.Fatal(err)
	}
	invalid := playv1.PuzzleInvalidReason_PUZZLE_INVALID_REASON_ON_WRONG
	good := &playv1.PuzzleTrapTarget{MapId: p.mapID, PointId: trap.GetId()}
	tests := []struct {
		name string
		make func(t *testing.T) error
	}{
		{"a scene point is not a trap", func(t *testing.T) error {
			t.Helper()
			_, err := p.pc(p.master).CreatePuzzle(t.Context(), connect.NewRequest(&playv1.CreatePuzzleRequest{
				CampaignId: p.campaignID, Name: "x", Config: riddleConfig(), Solution: riddleSolution("a"),
				OnWrong: &playv1.PuzzleOnWrong{Trap: &playv1.PuzzleTrapTarget{MapId: p.mapID, PointId: scene.Msg.GetPoint().GetId()}},
			}))
			return err
		}},
		{"a point that is not there", func(t *testing.T) error {
			t.Helper()
			_, err := p.pc(p.master).CreatePuzzle(t.Context(), connect.NewRequest(&playv1.CreatePuzzleRequest{
				CampaignId: p.campaignID, Name: "x", Config: riddleConfig(), Solution: riddleSolution("a"),
				OnWrong: &playv1.PuzzleOnWrong{Trap: &playv1.PuzzleTrapTarget{MapId: p.mapID, PointId: newKey()}},
			}))
			return err
		}},
		{"a map that is not there", func(t *testing.T) error {
			t.Helper()
			_, err := p.pc(p.master).CreatePuzzle(t.Context(), connect.NewRequest(&playv1.CreatePuzzleRequest{
				CampaignId: p.campaignID, Name: "x", Config: riddleConfig(), Solution: riddleSolution("a"),
				OnWrong: &playv1.PuzzleOnWrong{Trap: &playv1.PuzzleTrapTarget{MapId: newKey(), PointId: trap.GetId()}},
			}))
			return err
		}},
		{"a trap without ids", func(t *testing.T) error {
			t.Helper()
			_, err := p.pc(p.master).CreatePuzzle(t.Context(), connect.NewRequest(&playv1.CreatePuzzleRequest{
				CampaignId: p.campaignID, Name: "x", Config: riddleConfig(), Solution: riddleSolution("a"),
				OnWrong: &playv1.PuzzleOnWrong{Trap: &playv1.PuzzleTrapTarget{}},
			}))
			return err
		}},
		{"a trap on a lock", func(t *testing.T) error {
			t.Helper()
			_, err := p.pc(p.master).CreatePuzzle(t.Context(), connect.NewRequest(&playv1.CreatePuzzleRequest{
				CampaignId: p.campaignID, Name: "x", Config: lockConfig(2, playv1.PuzzleAlphabet_PUZZLE_ALPHABET_DIGITS),
				Solution: lockSolution(1, 1), Start: lockStart(0, 0), OnWrong: &playv1.PuzzleOnWrong{Trap: good},
			}))
			return err
		}},
		{"attempts on the lights", func(t *testing.T) error {
			t.Helper()
			_, err := p.pc(p.master).CreatePuzzle(t.Context(), connect.NewRequest(&playv1.CreatePuzzleRequest{
				CampaignId: p.campaignID, Name: "x", Config: lightsConfig(3),
				OnWrong: &playv1.PuzzleOnWrong{AttemptsPerPlayer: 3},
			}))
			return err
		}},
		{"11 attempts", func(t *testing.T) error {
			t.Helper()
			_, err := p.pc(p.master).CreatePuzzle(t.Context(), connect.NewRequest(&playv1.CreatePuzzleRequest{
				CampaignId: p.campaignID, Name: "x", Config: riddleConfig(), Solution: riddleSolution("a"),
				OnWrong: &playv1.PuzzleOnWrong{AttemptsPerPlayer: 11},
			}))
			return err
		}},
		{"negative attempts", func(t *testing.T) error {
			t.Helper()
			_, err := p.pc(p.master).CreatePuzzle(t.Context(), connect.NewRequest(&playv1.CreatePuzzleRequest{
				CampaignId: p.campaignID, Name: "x", Config: riddleConfig(), Solution: riddleSolution("a"),
				OnWrong: &playv1.PuzzleOnWrong{AttemptsPerPlayer: -1},
			}))
			return err
		}},
		{"201 moves", func(t *testing.T) error {
			t.Helper()
			_, err := p.pc(p.master).CreatePuzzle(t.Context(), connect.NewRequest(&playv1.CreatePuzzleRequest{
				CampaignId: p.campaignID, Name: "x", Config: riddleConfig(), Solution: riddleSolution("a"),
				OnWrong: &playv1.PuzzleOnWrong{MaxMoves: 201},
			}))
			return err
		}},
		{"9 seconds", func(t *testing.T) error {
			t.Helper()
			_, err := p.pc(p.master).CreatePuzzle(t.Context(), connect.NewRequest(&playv1.CreatePuzzleRequest{
				CampaignId: p.campaignID, Name: "x", Config: riddleConfig(), Solution: riddleSolution("a"),
				OnWrong: &playv1.PuzzleOnWrong{TimeLimitSeconds: 9},
			}))
			return err
		}},
		{"four hours and a second", func(t *testing.T) error {
			t.Helper()
			_, err := p.pc(p.master).CreatePuzzle(t.Context(), connect.NewRequest(&playv1.CreatePuzzleRequest{
				CampaignId: p.campaignID, Name: "x", Config: riddleConfig(), Solution: riddleSolution("a"),
				OnWrong: &playv1.PuzzleOnWrong{TimeLimitSeconds: 14401},
			}))
			return err
		}},
	}
	for _, tt := range tests {
		wantPuzzleInvalid(t, tt.name, tt.make(t), invalid)
	}
	// The limits apply to every kind; all of it together is fine; all zero is nothing.
	p.create(t, &playv1.CreatePuzzleRequest{Name: "Luzes", Config: lightsConfig(3), OnWrong: &playv1.PuzzleOnWrong{MaxMoves: 200, TimeLimitSeconds: 14400}})
	all := p.riddle(t, "Tudo", onWrong(func(o *playv1.PuzzleOnWrong) {
		o.Trap, o.AttemptsPerPlayer, o.MaxMoves, o.TimeLimitSeconds = good, 10, 1, 10
	}))
	if o := all.GetOnWrong(); o.GetAttemptsPerPlayer() != 10 || o.GetMaxMoves() != 1 || o.GetTimeLimitSeconds() != 10 || o.GetTrap() == nil {
		t.Errorf("the master reads %v", o)
	}
	if nothing := p.riddle(t, "Nada", onWrong(func(*playv1.PuzzleOnWrong) {})); nothing.GetOnWrong() != nil {
		t.Errorf("an empty \"Ao errar\" reads %v", nothing.GetOnWrong())
	}
	// An edit changes it, and takes it away.
	upd, err := p.pc(p.master).UpdatePuzzle(t.Context(), connect.NewRequest(&playv1.UpdatePuzzleRequest{
		CampaignId: p.campaignID, PuzzleId: all.GetId(), Name: "Tudo", Config: riddleConfig(), Solution: riddleSolution("sombra"), Hints: []string{"dica"},
		HintCheck: &playv1.PuzzleHintCheck{SkillKey: "skill:history", Dc: 9}, Parts: []*playv1.PuzzlePart{{Text: "parte"}},
		OnWrong: &playv1.PuzzleOnWrong{AttemptsPerPlayer: 2},
	}))
	if err != nil {
		t.Fatalf("UpdatePuzzle() error = %v", err)
	}
	if u := upd.Msg.GetPuzzle(); u.GetOnWrong().GetAttemptsPerPlayer() != 2 || u.GetOnWrong().GetTrap() != nil || u.GetHintCheck().GetSkillKey() != "skill:history" || len(u.GetParts()) != 1 {
		t.Errorf("the edited puzzle reads %v", u)
	}
	upd, err = p.pc(p.master).UpdatePuzzle(t.Context(), connect.NewRequest(&playv1.UpdatePuzzleRequest{CampaignId: p.campaignID, PuzzleId: all.GetId(), Name: "Tudo", Config: riddleConfig(), Solution: riddleSolution("sombra")}))
	if err != nil || upd.Msg.GetPuzzle().GetOnWrong() != nil || upd.Msg.GetPuzzle().GetHintCheck() != nil || len(upd.Msg.GetPuzzle().GetParts()) != 0 {
		t.Errorf("an edit that takes them away = %v, %v", upd, err)
	}
}

// --- "Ao errar": the attempts and the limits ---

func TestMR038_AnAttemptIsSpentPerPlayer(t *testing.T) {
	t.Parallel()
	p := newPuzzleTable(t)
	puz := p.riddle(t, "A porta da Cripta pergunta", onWrong(func(o *playv1.PuzzleOnWrong) { o.AttemptsPerPlayer = 3 }))
	p.show(t, puz.GetId())
	if l := p.read(t, p.caio, puz.GetId()).GetLimits(); l.GetAttemptsPerPlayer() != 3 || l.GetAttemptsLeft() != 3 || l.GetMaxMoves() != 0 || l.GetDeadline() != nil {
		t.Fatalf("the limits read %v", l)
	}
	for i := range 3 {
		res := p.mustMove(t, p.caio, puz.GetId(), riddleMove("luz"))
		if l := res.GetRun().GetLimits(); l.GetAttemptsLeft() != int32(2-i) || l.GetMovesMade() != int32(i+1) {
			t.Fatalf("after wrong answer %d the limits read %v", i+1, l)
		}
	}
	_, err := p.move(t, p.caio, puz.GetId(), riddleMove("sombra"))
	wantPuzzleBlocked(t, "a fourth answer", err, playv1.PuzzleBlockedReason_PUZZLE_BLOCKED_REASON_NO_ATTEMPTS_LEFT)
	if r := p.read(t, p.caio, puz.GetId()); r.GetLimits().GetAttemptsLeft() != 0 || r.GetSolved() || r.GetStopped() {
		t.Errorf("Toren reads %v", r)
	}
	// The others have all of theirs, and the master reads everyone's.
	if l := p.read(t, p.ana, puz.GetId()).GetLimits(); l.GetAttemptsLeft() != 3 {
		t.Errorf("Pensantus reads %v", l)
	}
	m := p.masterRun(t, puz.GetId())
	got := map[string]int32{}
	for _, a := range m.GetAttempts() {
		got[a.GetCharacterName()] = a.GetLeft()
		if a.GetCharacterName() == "Toren" && a.GetWrong() != 3 {
			t.Errorf("the master reads Toren's wrong moves as %d", a.GetWrong())
		}
	}
	if got["Toren"] != 0 || got["Pensantus"] != 3 || got["Brisa"] != 3 {
		t.Errorf("the master reads the attempts left %v", got)
	}
	// Another player may still answer, right: it solves, though Toren has no attempts.
	if res := p.mustMove(t, p.ana, puz.GetId(), riddleMove("sombra")); !res.GetSolvedByThisMove() {
		t.Errorf("Pensantus's answer = %v", res)
	}
	// "Recomeçar" gives them back.
	p.reset(t, puz.GetId())
	if l := p.read(t, p.caio, puz.GetId()).GetLimits(); l.GetAttemptsLeft() != 3 || l.GetMovesMade() != 0 {
		t.Errorf("after the restart Toren reads %v", l)
	}
	if res := p.mustMove(t, p.caio, puz.GetId(), riddleMove("luz")); res.GetRun().GetLimits().GetAttemptsLeft() != 2 {
		t.Errorf("a wrong answer of the new round = %v", res.GetRun().GetLimits())
	}
}

func TestMR038_AMovesLimitStopsThePuzzle(t *testing.T) {
	t.Parallel()
	p := newPuzzleTable(t)
	puz := p.cipher(t, "A carta do Capitão", onWrong(func(o *playv1.PuzzleOnWrong) { o.MaxMoves = 3 }))
	p.show(t, puz.GetId())
	p.mustMove(t, p.caio, puz.GetId(), cipherMove("um"))
	p.mustMove(t, p.ana, puz.GetId(), cipherMove("dois"))
	if r := p.read(t, p.bia, puz.GetId()); r.GetStopped() || r.GetLimits().GetMovesMade() != 2 || r.GetLimits().GetMaxMoves() != 3 {
		t.Fatalf("two moves in, the player reads %v", r)
	}
	res := p.mustMove(t, p.bia, puz.GetId(), cipherMove("três"))
	if !res.GetRun().GetStopped() || res.GetRun().GetStoppedMessage() != "O quebra-cabeça parou. O mestre decide o que acontece agora." || res.GetRun().GetSolved() || res.GetRun().GetLimits().GetMovesMade() != 3 {
		t.Fatalf("the move that reached the limit left %v", res.GetRun())
	}
	// The players read it neutrally (no reason); the master reads which limit, and is told.
	if j := jsonOf(res.GetRun()); strings.Contains(j, "stopReason") {
		t.Errorf("a player's run has the reason: %s", j)
	}
	if m := p.masterRun(t, puz.GetId()); m.GetStopReason() != playv1.PuzzleStopReason_PUZZLE_STOP_REASON_MOVES || m.GetStatus() != playv1.PuzzleRunStatus_PUZZLE_RUN_STATUS_SHOWN {
		t.Errorf("the master reads %v", m)
	}
	_, err := p.move(t, p.caio, puz.GetId(), cipherMove(letter))
	wantPuzzleBlocked(t, "a move after the limit", err, playv1.PuzzleBlockedReason_PUZZLE_BLOCKED_REASON_STOPPED)
	// "Recomeçar" begins a new round; the move that reaches the limit and solves it solves it.
	run := p.reset(t, puz.GetId())
	if run.GetStopReason() != playv1.PuzzleStopReason_PUZZLE_STOP_REASON_UNSPECIFIED {
		t.Errorf("a restarted puzzle reads %v", run.GetStopReason())
	}
	p.mustMove(t, p.caio, puz.GetId(), cipherMove("um"))
	p.mustMove(t, p.caio, puz.GetId(), cipherMove("dois"))
	if res := p.mustMove(t, p.caio, puz.GetId(), cipherMove(letter)); !res.GetSolvedByThisMove() || res.GetRun().GetStopped() {
		t.Errorf("the third move solved it and left %v", res.GetRun())
	}
	// A limit counts the moves of every kind: three presses of a lock.
	lock := p.lock(t, "Cofre", onWrong(func(o *playv1.PuzzleOnWrong) { o.MaxMoves = 2 }))
	p.show(t, lock.GetId())
	p.mustMove(t, p.caio, lock.GetId(), lockMove(0, 1))
	if res := p.mustMove(t, p.caio, lock.GetId(), lockMove(0, 1)); !res.GetRun().GetStopped() {
		t.Errorf("a lock at its limit reads %v", res.GetRun())
	}
	_, err = p.tryHint(t, p.caio, lock.GetId(), 20)
	wantPuzzleBlocked(t, "a try for a hint at a stopped puzzle", err, playv1.PuzzleBlockedReason_PUZZLE_BLOCKED_REASON_STOPPED)
	// Closing a stopped puzzle and showing it again begins a round too.
	p.close(t, lock.GetId())
	p.show(t, lock.GetId())
	if r := p.read(t, p.caio, lock.GetId()); r.GetStopped() || r.GetLimits().GetMovesMade() != 0 {
		t.Errorf("a puzzle shown again reads %v", r)
	}
}

func (p *puzzleTable) close(t *testing.T, id string) {
	t.Helper()
	if _, err := p.pc(p.master).ClosePuzzle(t.Context(), connect.NewRequest(&playv1.ClosePuzzleRequest{CampaignId: p.campaignID, PuzzleId: id})); err != nil {
		t.Fatalf("ClosePuzzle() error = %v", err)
	}
}

func TestMR038_ATimeLimitStopsThePuzzle(t *testing.T) {
	t.Parallel()
	p := newPuzzleTable(t)
	clock := p.movableClock()
	gc := &gateClock{t: time.Now()}
	p.h.svc.puzzles.hints.now, p.h.svc.puzzles.hints.after = gc.now, gc.after
	puz := p.sequence(t, "Os sinos", onWrong(func(o *playv1.PuzzleOnWrong) { o.TimeLimitSeconds = 300 }))
	p.show(t, puz.GetId())
	// A sequence's clock starts at its first play, not at "show": nobody may play before it.
	clock.advance(10 * time.Minute)
	if r := p.read(t, p.caio, puz.GetId()); r.GetStopped() || r.GetLimits().GetDeadline() != nil || r.GetLimits().GetSecondsLeft() != 0 {
		t.Fatalf("a sequence not played yet reads %v: its clock must not run", r.GetLimits())
	}
	p.playSequence(t, puz.GetId())
	// The first play starts the clock, and the apps are told to read again when it runs out.
	foundTimer := false
	for _, tm := range gc.timers {
		foundTimer = foundTimer || (tm.at.Sub(gc.t) > 299*time.Second && tm.at.Sub(gc.t) <= 300*time.Second)
	}
	if !foundTimer {
		t.Errorf("no hint scheduled for the end of the time limit")
	}
	clock.advance(time.Minute)
	r := p.read(t, p.caio, puz.GetId())
	if l := r.GetLimits(); l.GetTimeLimitSeconds() != 300 || l.GetSecondsLeft() < 238 || l.GetSecondsLeft() > 240 || l.GetDeadline() == nil || r.GetStopped() {
		t.Fatalf("a minute in, the player reads %v", r)
	}
	p.mustMove(t, p.caio, puz.GetId(), bellMove(2))
	clock.advance(4*time.Minute + 5*time.Second)
	r = p.read(t, p.caio, puz.GetId())
	if !r.GetStopped() || r.GetLimits().GetSecondsLeft() != 0 || r.GetStoppedMessage() == "" {
		t.Fatalf("after the time ran out the player reads %v", r)
	}
	if m := p.masterRun(t, puz.GetId()); m.GetStopReason() != playv1.PuzzleStopReason_PUZZLE_STOP_REASON_TIME {
		t.Errorf("the master reads %v", m.GetStopReason())
	}
	_, err := p.move(t, p.ana, puz.GetId(), bellMove(0))
	wantPuzzleBlocked(t, "a bell after the time ran out", err, playv1.PuzzleBlockedReason_PUZZLE_BLOCKED_REASON_STOPPED)
	_, err = p.pc(p.master).PlayPuzzleSequence(t.Context(), connect.NewRequest(&playv1.PlayPuzzleSequenceRequest{CampaignId: p.campaignID, PuzzleId: puz.GetId()}))
	wantPuzzleBlocked(t, "playing a stopped sequence", err, playv1.PuzzleBlockedReason_PUZZLE_BLOCKED_REASON_STOPPED)
	// "Recomeçar" begins a new round; the sequence's clock starts again at its first play.
	p.reset(t, puz.GetId())
	if r := p.read(t, p.caio, puz.GetId()); r.GetStopped() || r.GetLimits().GetDeadline() != nil {
		t.Errorf("a restarted sequence reads %v", r)
	}
	p.playSequence(t, puz.GetId())
	if r := p.read(t, p.caio, puz.GetId()); r.GetStopped() || r.GetLimits().GetSecondsLeft() < 299 {
		t.Errorf("a restarted and played sequence reads %v", r)
	}
	// A prepared run (a new start drawn before showing) starts its clock when it is shown.
	late := p.lights(t, "Luzes", 3, onWrong(func(o *playv1.PuzzleOnWrong) { o.TimeLimitSeconds = 60 }))
	if _, err := p.pc(p.master).ReseedPuzzle(t.Context(), connect.NewRequest(&playv1.ReseedPuzzleRequest{CampaignId: p.campaignID, PuzzleId: late.GetId()})); err != nil {
		t.Fatal(err)
	}
	clock.advance(10 * time.Minute)
	p.show(t, late.GetId())
	if r := p.read(t, p.caio, late.GetId()); r.GetStopped() || r.GetLimits().GetSecondsLeft() < 59 {
		t.Errorf("a prepared puzzle shown later reads %v", r)
	}
}

// Concurrent wrong answers: the limits are counted exactly, because every change to a
// run takes its lock.
func TestMR038_ConcurrentWrongAnswersAreCountedExactly(t *testing.T) {
	t.Parallel()
	dbtest.PoolSize(t, 8)
	p := newPuzzleTable(t)
	attempts := p.riddle(t, "Tentativas", onWrong(func(o *playv1.PuzzleOnWrong) { o.AttemptsPerPlayer = 3 }))
	limit := p.riddle(t, "Limite", onWrong(func(o *playv1.PuzzleOnWrong) { o.MaxMoves = 7 }))
	p.show(t, attempts.GetId())
	p.show(t, limit.GetId())

	var wg sync.WaitGroup
	var accepted, noAttempts, stopped, other atomic.Int32
	count := func(err error) {
		switch {
		case err == nil:
			accepted.Add(1)
		case blockedReason(err) == playv1.PuzzleBlockedReason_PUZZLE_BLOCKED_REASON_NO_ATTEMPTS_LEFT:
			noAttempts.Add(1)
		case blockedReason(err) == playv1.PuzzleBlockedReason_PUZZLE_BLOCKED_REASON_STOPPED:
			stopped.Add(1)
		default:
			other.Add(1)
			t.Errorf("MakePuzzleMove() error = %v", err)
		}
	}
	for _, u := range []*user{p.caio, p.ana, p.bia} {
		for range 6 {
			wg.Go(func() {
				_, err := p.move(t, u, attempts.GetId(), riddleMove("luz"))
				count(err)
			})
		}
	}
	wg.Wait()
	if accepted.Load() != 9 || noAttempts.Load() != 9 || other.Load() != 0 {
		t.Fatalf("attempts: %d accepted, %d out of attempts, want 9 and 9 (3 for each of 3 players)", accepted.Load(), noAttempts.Load())
	}
	for name, u := range map[string]*user{"Toren": p.caio, "Pensantus": p.ana, "Brisa": p.bia} {
		if l := p.read(t, u, attempts.GetId()).GetLimits(); l.GetAttemptsLeft() != 0 || l.GetMovesMade() != 9 {
			t.Errorf("%s reads %v", name, l)
		}
	}
	accepted.Store(0)
	for _, u := range []*user{p.caio, p.ana, p.bia} {
		for range 6 {
			wg.Go(func() {
				_, err := p.move(t, u, limit.GetId(), riddleMove("luz"))
				count(err)
			})
		}
	}
	wg.Wait()
	if accepted.Load() != 7 || stopped.Load() != 11 {
		t.Fatalf("limit: %d accepted and %d stopped, want 7 and 11", accepted.Load(), stopped.Load())
	}
	if m := p.masterRun(t, limit.GetId()); m.GetMovesMade() != 7 || m.GetStopReason() != playv1.PuzzleStopReason_PUZZLE_STOP_REASON_MOVES {
		t.Errorf("the master reads %d moves and %v", m.GetMovesMade(), m.GetStopReason())
	}
	var rows int
	if err := p.h.pool.QueryRow(t.Context(), `SELECT count(*) FROM puzzle_moves WHERE wrong`).Scan(&rows); err != nil || rows != 16 {
		t.Errorf("%d wrong moves are recorded (%v), want 16", rows, err)
	}
}

// --- RN-10 ---

// The allow-list of the players' JSON for every new path (RN-10, RN-27): the riddle's
// answers, the sequence before it plays and after, the cipher's message and key, the DC,
// the other players' parts, a typed answer and the trap's details.
func TestRN10_PlayersNeverReceiveWhatTheNewKindsHide(t *testing.T) {
	t.Parallel()
	p := newPuzzleTable(t)
	clock := p.movableClock()
	ctx := t.Context()
	trap := p.trapPoint(t, "NOME-DA-ARMADILHA", 12, 4)
	x, y := sq(6, 6)
	pt, err := p.mc(p.master).CreateMapPoint(ctx, connect.NewRequest(&mapsv1.CreateMapPointRequest{CampaignId: p.campaignID, MapId: p.mapID, Kind: mapsv1.MapPointKind_MAP_POINT_KIND_SCENE, Name: "Biblioteca", XBp: x, YBp: y}))
	if err != nil {
		t.Fatal(err)
	}
	clue, err := p.mc(p.master).AddSceneClue(ctx, connect.NewRequest(&mapsv1.AddSceneClueRequest{CampaignId: p.campaignID, MapId: p.mapID, PointId: pt.Msg.GetPoint().GetId(), Text: "TEXTO-DA-CHAVE"}))
	if err != nil {
		t.Fatal(err)
	}
	clueID := clue.Msg.GetClue().GetId()
	torenPart, pensPart := "SEGREDO-PARTE-DO-TOREN", "SEGREDO-PARTE-DA-PENSANTUS"
	riddle := p.riddle(t, "Enigma", func(r *playv1.CreatePuzzleRequest) {
		r.Solution = riddleSolution("RESPOSTA-SECRETA", "OUTRA-RESPOSTA")
		hintCheck("skill:investigation", 17)(r)
		r.Parts = []*playv1.PuzzlePart{{CharacterId: p.toren.GetId(), Text: torenPart}, {CharacterId: p.pens.GetId(), Text: pensPart}}
		r.OnWrong = &playv1.PuzzleOnWrong{Trap: &playv1.PuzzleTrapTarget{MapId: p.mapID, PointId: trap.GetId()}, AttemptsPerPlayer: 5, MaxMoves: 50, TimeLimitSeconds: 3600}
	})
	seq := p.sequence(t, "Sinos", func(r *playv1.CreatePuzzleRequest) {
		r.Solution = sequenceSolution(3, 3, 0, 2, 1, 1)
		r.OnWrong = &playv1.PuzzleOnWrong{MaxMoves: 20}
	})
	cph := p.cipher(t, "Cifra", func(r *playv1.CreatePuzzleRequest) {
		r.Config = cipherConfig(clueID)
		r.Solution = cipherSolution("MENSAGEM-SECRETA-AQUI", 5)
	})
	puzzles := []*playv1.Puzzle{riddle, seq, cph}
	for _, puz := range puzzles {
		p.show(t, puz.GetId())
	}

	var pile []string
	read := func(u *user, puz *playv1.Puzzle) *playv1.PuzzleRun {
		t.Helper()
		r := p.read(t, u, puz.GetId())
		checkPlayerRun(t, "GetPuzzleRun "+puz.GetName(), r)
		pile = append(pile, jsonOf(r))
		return r
	}
	everyone := func() {
		t.Helper()
		for _, u := range []*user{p.caio, p.ana, p.bia} {
			for _, puz := range puzzles {
				read(u, puz)
			}
		}
	}
	checkMove := func(what string, res *playv1.MakePuzzleMoveResponse, err error) {
		t.Helper()
		if err != nil {
			t.Fatalf("%s: %v", what, err)
		}
		checkPlayerMove(t, what, res)
		pile = append(pile, jsonOf(res))
	}
	everyone()
	// Before any wrong move the trap is not named anywhere a player reads.
	if strings.Contains(strings.Join(pile, "\n"), "NOME-DA-ARMADILHA") {
		t.Fatal("the trap's name reached the players before it fired")
	}
	// The sequence mid-play and after it: the steps only as the play reveals them.
	p.playSequence(t, seq.GetId())
	if mid := p.read(t, p.caio, seq.GetId()).GetSequence().GetShown(); len(mid) != 1 || mid[0] != 3 {
		t.Fatalf("a play shows %v, want only the first step", mid)
	}
	everyone()
	clock.advance(time.Minute)
	everyone()
	// Wrong answers, a wrong bell and a wrong message: each answer to a move is checked.
	res, err := p.move(t, p.caio, riddle.GetId(), riddleMove("PALPITE-DO-JOGADOR"))
	checkMove("a wrong answer", res, err)
	res, err = p.move(t, p.ana, seq.GetId(), bellMove(0))
	checkMove("a wrong bell", res, err)
	res, err = p.move(t, p.bia, cph.GetId(), cipherMove("PALPITE-DA-MENSAGEM"))
	checkMove("a wrong message", res, err)
	// Hints won by a check: a pass and a fail, each answer and the player's read.
	for _, face := range []int32{20, 1} {
		hint, err := p.tryHint(t, p.caio, riddle.GetId(), face)
		if err != nil {
			t.Fatal(err)
		}
		checkPlayerHint(t, "a try", hint)
		pile = append(pile, jsonOf(hint))
	}
	everyone()
	// The key clue found, the sequence solved, the riddle solved, the cipher restarted.
	if _, err := p.mc(p.master).RevealSceneClue(ctx, connect.NewRequest(&mapsv1.RevealSceneClueRequest{CampaignId: p.campaignID, ClueId: clueID, CharacterIds: []string{p.pens.GetId()}})); err != nil {
		t.Fatal(err)
	}
	if got := read(p.ana, cph); got.GetKeyClueId() != clueID {
		t.Errorf("the player that found the clue reads %v", got)
	}
	for _, b := range []int32{3, 3, 0, 2, 1, 1} {
		res, err = p.move(t, p.ana, seq.GetId(), bellMove(b))
		checkMove("a right bell", res, err)
	}
	res, err = p.move(t, p.ana, riddle.GetId(), riddleMove("outra resposta"))
	checkMove("the right answer", res, err)
	p.reset(t, cph.GetId())
	everyone()
	// The stream: the hint names the puzzle and nothing else.
	w := p.ana.watch(t, p.campaignID)
	w.ready(t)
	p.reset(t, seq.GetId())
	for {
		ev := w.nextChange(t)
		if ev.GetPuzzleChanged() == nil {
			continue
		}
		checkKeys(t, "puzzle_changed", jsonOf(ev), "", map[string][]string{"": {"puzzleChanged"}, ".puzzleChanged": {"puzzleId"}})
		break
	}

	all := strings.Join(pile, "\n")
	for _, forbidden := range []string{
		"RESPOSTA-SECRETA", "OUTRA-RESPOSTA", "MENSAGEM-SECRETA", "TEXTO-DA-CHAVE", "PALPITE-DO-JOGADOR", "PALPITE-DA-MENSAGEM",
		trap.GetId(), p.mapID, pt.Msg.GetPoint().GetId(),
		"solution", "answers", "keyword", "shift", "\"dc\"", "hintCheck", "onWrong", "minimum", "path", "characterId", "stopReason",
	} {
		if strings.Contains(all, forbidden) {
			i := strings.Index(all, forbidden)
			t.Errorf("a player's JSON has %q: ...%s...", forbidden, all[max(0, i-60):min(len(all), i+80)])
		}
	}
	// The trap's name is public once it fired, as the name of the last wrong move's trap.
	if !strings.Contains(all, "NOME-DA-ARMADILHA") {
		t.Errorf("the trap that fired is not named for the players")
	}
	// Each player reads their own part and nobody else's; the key clue is read by who found it.
	for who, u := range map[string]*user{"Toren": p.caio, "Pensantus": p.ana, "Brisa": p.bia} {
		j := jsonOf(p.read(t, u, riddle.GetId()))
		if got := strings.Contains(j, torenPart); got != (who == "Toren") {
			t.Errorf("%s reads Toren's part = %v", who, got)
		}
		if got := strings.Contains(j, pensPart); got != (who == "Pensantus") {
			t.Errorf("%s reads Pensantus's part = %v", who, got)
		}
		if got := strings.Contains(jsonOf(p.read(t, u, cph.GetId())), clueID); got != (who == "Pensantus") {
			t.Errorf("%s reads the key clue's id = %v", who, got)
		}
	}
	// What the master reads has all of it, so the checks above are not empty.
	mj := jsonOf(p.masterRun(t, riddle.GetId()))
	for _, want := range []string{"RESPOSTA-SECRETA", torenPart, pensPart, "\"dc\"", "attemptsPerPlayer", "outra resposta"} {
		if !strings.Contains(mj, want) {
			t.Errorf("the master's view lacks %q: %s", want, mj)
		}
	}
	if sj := jsonOf(p.masterRun(t, seq.GetId())); !strings.Contains(sj, "\"steps\"") {
		t.Errorf("the master's sequence lacks its steps: %s", sj)
	}
}

// checkPlayerHint checks the keys of a try's answer: the run is the player's own run, the
// roll a dice roll, and nothing else.
func checkPlayerHint(t *testing.T, what string, res *playv1.TryPuzzleHintResponse) {
	t.Helper()
	var top map[string]json.RawMessage
	if err := json.Unmarshal([]byte(jsonOf(res)), &top); err != nil {
		t.Fatal(err)
	}
	for k := range top {
		if k != "run" && k != "roll" && k != "passed" && k != "replayed" {
			t.Errorf("%s: the key %q is not on the list", what, k)
		}
	}
	checkKeys(t, what+"'s roll", jsonOf(res.GetRoll()), "", map[string][]string{"": {"diceCount", "diceSides", "faces", "modifier", "total", "physical"}})
	checkPlayerRun(t, what, res.GetRun())
}

// --- fix round 1 ---

// RN-25: a combat without a map has no traps. A wrong move still counts, and fires nothing.
func TestMR038_ATheatreCombatFiresNoPuzzleTrap(t *testing.T) {
	t.Parallel()
	p := newPuzzleTable(t)
	trap := p.trapPoint(t, "Dardos", 12, 4)
	rid := p.riddle(t, "Enigma", onWrong(func(o *playv1.PuzzleOnWrong) {
		o.Trap, o.MaxMoves = &playv1.PuzzleTrapTarget{MapId: p.mapID, PointId: trap.GetId()}, 5
	}))
	p.show(t, rid.GetId())
	p.theatreThree(t)
	res := p.mustMove(t, p.caio, rid.GetId(), riddleMove("luz"))
	lm := res.GetRun().GetLastMove()
	if !lm.GetWrong() || lm.GetTrapName() != "" || res.GetRun().GetLimits().GetMovesMade() != 1 {
		t.Fatalf("a wrong answer in a theatre combat left %v, %v", lm, res.GetRun().GetLimits())
	}
	if st := p.trapState(t, trap.GetId()); st != "armed" || p.eventCount(t, "trap_triggered") != 0 {
		t.Errorf("the trap is %q with %d firings, want armed and none", st, p.eventCount(t, "trap_triggered"))
	}
}

// While a combat runs on the trap's map the firing is the combat's, so the master can
// extend it to the creatures it catches.
func TestMR038_ACombatOnTheTrapsMapOwnsTheFiring(t *testing.T) {
	t.Parallel()
	p := newPuzzleTable(t)
	trap := p.trapPoint(t, "Dardos", 20, 12)
	rid := p.riddle(t, "Enigma", onWrong(func(o *playv1.PuzzleOnWrong) {
		o.Trap = &playv1.PuzzleTrapTarget{MapId: p.mapID, PointId: trap.GetId()}
	}))
	p.show(t, rid.GetId())
	enc := p.threeAndAGoblin(t)
	res := p.mustMove(t, p.ana, rid.GetId(), riddleMove("luz"))
	if res.GetRun().GetLastMove().GetTrapName() == "" {
		t.Fatalf("the trap did not fire: %v", res.GetRun().GetLastMove())
	}
	var firingID string
	var encID *string
	if err := p.h.pool.QueryRow(t.Context(), `SELECT id::TEXT, encounter_id::TEXT FROM session_events WHERE kind = 'trap_triggered'`).Scan(&firingID, &encID); err != nil {
		t.Fatal(err)
	}
	if encID == nil || *encID != enc.GetId() {
		t.Fatalf("the firing belongs to combat %v, want %s", encID, enc.GetId())
	}
	// The master extends it to a combatant: it works only because the firing is the combat's.
	if _, err := p.master.play.FireTrap(t.Context(), connect.NewRequest(&playv1.FireTrapRequest{
		CampaignId: p.campaignID, MapId: p.mapID, PointId: trap.GetId(), TargetIds: []string{byLabel(t, enc, "Brisa").GetId()},
		IdempotencyKey: newKey(), ExtendFiringId: firingID,
	})); err != nil {
		t.Fatalf("FireTrap(extend) error = %v", err)
	}
	if n := p.eventCount(t, "trap_triggered"); n != 2 {
		t.Errorf("%d firings, want the puzzle's and the extension", n)
	}
}

// A player that does not see the trap's point reads nothing of it: no name, not an empty
// one (RN-10).
func TestRN10_ATrapThePlayerDoesNotSeeIsNotNamed(t *testing.T) {
	t.Parallel()
	p := newPuzzleTable(t)
	other := p.h.newMap(p.campaignID, gridColumns)
	x, y := sq(3, 3)
	pt, err := p.mc(p.master).CreateMapPoint(t.Context(), connect.NewRequest(&mapsv1.CreateMapPointRequest{
		CampaignId: p.campaignID, MapId: other, Kind: mapsv1.MapPointKind_MAP_POINT_KIND_TRAP, Name: "NOME-DA-ARMADILHA-ESCONDIDA", XBp: x, YBp: y, Trap: aTrap(),
	}))
	if err != nil {
		t.Fatal(err)
	}
	rid := p.riddle(t, "Enigma", onWrong(func(o *playv1.PuzzleOnWrong) {
		o.Trap = &playv1.PuzzleTrapTarget{MapId: other, PointId: pt.Msg.GetPoint().GetId()}
	}))
	p.show(t, rid.GetId())
	res := p.mustMove(t, p.caio, rid.GetId(), riddleMove("luz"))
	if st := p.trapState(t, pt.Msg.GetPoint().GetId()); st != "triggered" {
		t.Fatalf("the trap is %q, want triggered", st)
	}
	for who, run := range map[string]*playv1.PuzzleRun{"the answer": res.GetRun(), "a read": p.read(t, p.ana, rid.GetId())} {
		j := jsonOf(run)
		if strings.Contains(j, "ARMADILHA") || strings.Contains(j, "trapName") {
			t.Errorf("%s names a trap on a map the player does not see: %s", who, j)
		}
	}
	// The master reads it, and the point is on the screen once the map is.
	if got := p.masterRun(t, rid.GetId()).GetLastMove().GetTrapName(); got != "NOME-DA-ARMADILHA-ESCONDIDA" {
		t.Errorf("the master reads %q", got)
	}
	if _, err := p.master.play.SetCurrentMap(t.Context(), connect.NewRequest(&playv1.SetCurrentMapRequest{CampaignId: p.campaignID, MapId: other})); err != nil {
		t.Fatal(err)
	}
	if got := p.read(t, p.ana, rid.GetId()).GetLastMove().GetTrapName(); got != "NOME-DA-ARMADILHA-ESCONDIDA" {
		t.Errorf("a player that sees the point reads %q", got)
	}
}

// A stopped puzzle does not look open in the list, and a player with no living character
// cannot try for a hint.
func TestMR038_TheListShowsAStoppedPuzzleAndNoHintWithoutACharacter(t *testing.T) {
	t.Parallel()
	p := newPuzzleTable(t)
	puz := p.riddle(t, "Enigma", func(r *playv1.CreatePuzzleRequest) {
		hintCheck("skill:investigation", 12)(r)
		onWrong(func(o *playv1.PuzzleOnWrong) { o.MaxMoves = 1 })(r)
	})
	p.show(t, puz.GetId())
	list := func() *playv1.PuzzleSummary {
		res, err := p.pc(p.caio).ListShownPuzzles(t.Context(), connect.NewRequest(&playv1.ListShownPuzzlesRequest{CampaignId: p.campaignID}))
		if err != nil || len(res.Msg.GetPuzzles()) != 1 {
			t.Fatalf("ListShownPuzzles() = %v, %v", res, err)
		}
		return res.Msg.GetPuzzles()[0]
	}
	if list().GetStopped() {
		t.Fatal("an open puzzle is listed as stopped")
	}
	// Brisa's player has a character; a member with none cannot try.
	late := p.h.newUser("Lia")
	p.h.join(p.master, p.campaignID, late)
	if r := p.read(t, late, puz.GetId()); r.GetCanTryHint() || !r.GetHintByCheck() {
		t.Errorf("a player with no character reads %v", r)
	}
	if r := p.read(t, p.bia, puz.GetId()); !r.GetCanTryHint() {
		t.Errorf("a player with a character reads %v", r)
	}
	p.mustMove(t, p.caio, puz.GetId(), riddleMove("luz"))
	if !list().GetStopped() {
		t.Errorf("a stopped puzzle is listed as open")
	}
	p.reset(t, puz.GetId())
	if list().GetStopped() {
		t.Errorf("a restarted puzzle is listed as stopped")
	}
}

// A part whose owner died mid-session has no reader: the master's view flags it.
func TestMR038_APartOfADeadCharacterIsFlaggedForTheMaster(t *testing.T) {
	t.Parallel()
	p := newPuzzleTable(t)
	puz := p.riddle(t, "Enigma", func(r *playv1.CreatePuzzleRequest) {
		r.Parts = []*playv1.PuzzlePart{{CharacterId: p.toren.GetId(), Text: "do Toren"}, {CharacterId: p.pens.GetId(), Text: "da Pensantus"}, {Text: "sem dono"}}
	})
	p.show(t, puz.GetId())
	for _, pt := range p.masterRun(t, puz.GetId()).GetPuzzle().GetParts() {
		if pt.GetOwnerUnavailable() {
			t.Fatalf("a part of a living owner is flagged: %v", pt)
		}
	}
	if _, err := p.h.pool.Exec(t.Context(), `UPDATE characters SET status = 'dead', died_at = now() WHERE id = $1`, p.toren.GetId()); err != nil {
		t.Fatal(err)
	}
	flags := map[string]bool{}
	for _, pt := range p.masterRun(t, puz.GetId()).GetPuzzle().GetParts() {
		flags[pt.GetText()] = pt.GetOwnerUnavailable()
	}
	if !flags["do Toren"] || flags["da Pensantus"] || flags["sem dono"] {
		t.Errorf("the master's flags are %v, want only the dead character's part", flags)
	}
	got, err := p.pc(p.master).GetPuzzle(t.Context(), connect.NewRequest(&playv1.GetPuzzleRequest{CampaignId: p.campaignID, PuzzleId: puz.GetId()}))
	if err != nil || !got.Msg.GetPuzzle().GetParts()[0].GetOwnerUnavailable() {
		t.Errorf("GetPuzzle reads %v, %v", got, err)
	}
}

// Two tries at once: the same player with two keys makes one real try (the other finds
// the hint tried), and the same key twice rolls once.
func TestMR038_ConcurrentHintTriesRollOnce(t *testing.T) {
	t.Parallel()
	dbtest.PoolSize(t, 6)
	p := newPuzzleTable(t)
	puz := p.riddle(t, "Enigma", hintCheck("skill:investigation", 12))
	p.show(t, puz.GetId())
	var wg sync.WaitGroup
	errs := make([]error, 2)
	for i := range 2 {
		wg.Go(func() {
			_, errs[i] = p.tryHint(t, p.caio, puz.GetId(), 1) // a fail: the next try is for the same hint
		})
	}
	wg.Wait()
	ok, tried := 0, 0
	for _, err := range errs {
		switch {
		case err == nil:
			ok++
		case blockedReason(err) == playv1.PuzzleBlockedReason_PUZZLE_BLOCKED_REASON_HINT_ALREADY_TRIED:
			tried++
		default:
			t.Errorf("TryPuzzleHint() error = %v", err)
		}
	}
	if ok != 1 || tried != 1 {
		t.Fatalf("two keys at once: %d tries made and %d refused, want 1 and 1", ok, tried)
	}
	// The same key twice, at once: one roll, and the other answers the same.
	key := newKey()
	var replays atomic.Int32
	for i := range 2 {
		wg.Go(func() {
			res, err := p.tryHintKey(t, p.ana, puz.GetId(), 20, key)
			errs[i] = err
			if err == nil && res.GetReplayed() {
				replays.Add(1)
			}
		})
	}
	wg.Wait()
	if errs[0] != nil || errs[1] != nil || replays.Load() != 1 {
		t.Fatalf("the same key twice: errors %v, %d replays, want none and 1", errs, replays.Load())
	}
	var rows int
	if err := p.h.pool.QueryRow(t.Context(), `SELECT count(*) FROM puzzle_hint_tries`).Scan(&rows); err != nil || rows != 2 {
		t.Errorf("%d tries are recorded (%v), want 2", rows, err)
	}
}

// A try's key stands for that try, and a key spent on a move or on a try is spent for the
// other: the same key for another roll, or for the other kind of change, is refused and
// changes nothing.
func TestMR038_AKeyReusedForAnotherRollOrKindOfChangeIsRefused(t *testing.T) {
	t.Parallel()
	p := newPuzzleTable(t)
	puz := p.lights(t, "O selo da Capela", 5, hintCheck("skill:investigation", 15))
	p.show(t, puz.GetId())

	tried := newKey()
	if _, err := p.tryHintKey(t, p.caio, puz.GetId(), 1, tried); err != nil {
		t.Fatalf("TryPuzzleHint() error = %v", err)
	}
	if again, err := p.tryHintKey(t, p.caio, puz.GetId(), 1, tried); err != nil || !again.GetReplayed() {
		t.Fatalf("the retry = %v, %v; want the first answer", again, err)
	}
	_, err := p.tryHintKey(t, p.caio, puz.GetId(), 20, tried)
	wantCode(t, "the key with another typed die", err, connect.CodeInvalidArgument)
	_, err = p.tryHintKey(t, p.caio, puz.GetId(), 0, tried)
	wantCode(t, "the key with a roll in the app", err, connect.CodeInvalidArgument)
	_, err = p.moveKey(t, p.caio, puz.GetId(), lightsMove(2, 2), tried)
	wantCode(t, "the key of a try on a move", err, connect.CodeInvalidArgument)

	moved := newKey()
	if _, err := p.moveKey(t, p.ana, puz.GetId(), lightsMove(2, 2), moved); err != nil {
		t.Fatalf("MakePuzzleMove() error = %v", err)
	}
	_, err = p.tryHintKey(t, p.ana, puz.GetId(), 20, moved)
	wantCode(t, "the key of a move on a try", err, connect.CodeInvalidArgument)

	m := p.masterRun(t, puz.GetId())
	if len(m.GetHintTries()) != 1 || m.GetMovesMade() != 1 {
		t.Errorf("the master reads %d tries and %d moves, want 1 and 1: the refused calls change nothing", len(m.GetHintTries()), m.GetMovesMade())
	}
}

// A closed puzzle is `not_found` to the players, whatever they ask, so the retry of a move made
// before the master closed it is too: the answer to a retry is not a way to read what the master
// took away. Shown again, the puzzle answers the retry with the first answer.
func TestMR038_AMoveRetriedAfterThePuzzleWasClosedIsNotFound(t *testing.T) {
	t.Parallel()
	p := newPuzzleTable(t)
	puz := p.lights(t, "O selo da Capela", 5)
	p.show(t, puz.GetId())
	key := newKey()
	if _, err := p.moveKey(t, p.caio, puz.GetId(), lightsMove(2, 2), key); err != nil {
		t.Fatalf("MakePuzzleMove() error = %v", err)
	}
	p.close(t, puz.GetId())
	_, err := p.moveKey(t, p.caio, puz.GetId(), lightsMove(2, 2), key)
	wantCode(t, "the retry of a move on a closed puzzle", err, connect.CodeNotFound)
	p.show(t, puz.GetId())
	if again, err := p.moveKey(t, p.caio, puz.GetId(), lightsMove(2, 2), key); err != nil || !again.GetReplayed() {
		t.Errorf("the retry once shown again = %v, %v; want the first answer", again, err)
	}
}

// The master's changes to a run name the revision they read: a retry of a change whose answer was
// lost, or a change made after a player moved, finds the run on another revision and is refused,
// so a hint is not released twice, a restart does not wipe the moves made after the first, and a
// play of the sequence is not counted twice. Without a revision nothing is checked.
func TestMR038_AMasterChangeForARunThatMovedOnIsRefused(t *testing.T) {
	t.Parallel()
	p := newPuzzleTable(t)
	ctx := t.Context()
	pc := p.pc(p.master)
	stale := func(call string, err error) {
		t.Helper()
		wantCode(t, call, err, connect.CodeFailedPrecondition)
		if got := blockedReason(err); got != playv1.PuzzleBlockedReason_PUZZLE_BLOCKED_REASON_STALE_REVISION {
			t.Errorf("%s reason = %v, want STALE_REVISION", call, got)
		}
	}
	lights := p.lights(t, "O selo da Capela", 5, func(r *playv1.CreatePuzzleRequest) { r.Hints = []string{"a", "b", "c"} })
	p.show(t, lights.GetId())
	revision := func(id string) int32 { return p.masterRun(t, id).GetRun().GetRevision() }
	release := func(rev int32) error {
		_, err := pc.ReleaseNextPuzzleHint(ctx, connect.NewRequest(&playv1.ReleaseNextPuzzleHintRequest{CampaignId: p.campaignID, PuzzleId: lights.GetId(), ExpectedRevision: rev}))
		return err
	}
	reset := func(rev int32) error {
		_, err := pc.ResetPuzzle(ctx, connect.NewRequest(&playv1.ResetPuzzleRequest{CampaignId: p.campaignID, PuzzleId: lights.GetId(), ExpectedRevision: rev}))
		return err
	}
	reseed := func(rev int32) error {
		_, err := pc.ReseedPuzzle(ctx, connect.NewRequest(&playv1.ReseedPuzzleRequest{CampaignId: p.campaignID, PuzzleId: lights.GetId(), ExpectedRevision: rev}))
		return err
	}

	// A hint released once, the retry of the call refused: one hint, not two.
	read := revision(lights.GetId())
	if err := release(read); err != nil {
		t.Fatalf("ReleaseNextPuzzleHint() error = %v", err)
	}
	stale("the retry of a release", release(read))
	if n := p.masterRun(t, lights.GetId()).GetReleasedHints(); n != 1 {
		t.Errorf("released hints = %d, want 1", n)
	}
	// A move came in after the master read the run: a restart for the old revision is refused and
	// keeps the move; for the revision now, it restarts, and its retry is refused.
	p.mustMove(t, p.caio, lights.GetId(), lightsMove(2, 2))
	stale("a restart for a run a player moved in", reset(read))
	if n := p.masterRun(t, lights.GetId()).GetMovesMade(); n != 1 || p.masterRun(t, lights.GetId()).GetRun().GetLastMove() == nil {
		t.Fatalf("the refused restart changed the run (moves made %d)", n)
	}
	read = revision(lights.GetId())
	if err := reset(read); err != nil {
		t.Fatalf("ResetPuzzle() error = %v", err)
	}
	p.mustMove(t, p.ana, lights.GetId(), lightsMove(0, 0))
	stale("the retry of a restart", reset(read))
	if p.masterRun(t, lights.GetId()).GetRun().GetLastMove() == nil {
		t.Errorf("the refused retry of the restart wiped the move made after it")
	}
	stale("the retry of a new start", reseed(read))
	// Without a revision nothing is checked, and the current one is accepted.
	if err := reseed(0); err != nil {
		t.Errorf("ReseedPuzzle() with no revision error = %v", err)
	}
	if err := reset(revision(lights.GetId())); err != nil {
		t.Errorf("ResetPuzzle() for the current revision error = %v", err)
	}

	// A play of the sequence, once.
	seq := p.sequence(t, "Os sinos")
	p.show(t, seq.GetId())
	play := func(rev int32) error {
		_, err := pc.PlayPuzzleSequence(ctx, connect.NewRequest(&playv1.PlayPuzzleSequenceRequest{CampaignId: p.campaignID, PuzzleId: seq.GetId(), ExpectedRevision: rev}))
		return err
	}
	read = revision(seq.GetId())
	if err := play(read); err != nil {
		t.Fatalf("PlayPuzzleSequence() error = %v", err)
	}
	stale("the retry of a play", play(read))
	if n := p.masterRun(t, seq.GetId()).GetRun().GetSequence().GetPlays(); n != 1 {
		t.Errorf("plays = %d, want 1", n)
	}
}

// dcKey finds a field that holds a DC ("dc", "hintDc", "checkDc"...): a key made of
// letters only. Searching the whole JSON for "dc" and "15" also matched the random
// UUIDs in the answer, which carry both now and then.
var dcKey = regexp.MustCompile(`"[A-Za-z]*[Dd][Cc][A-Za-z]*"\s*:`)

// A wrong answer sent again answers wrong, as the first call did, even when another player's
// move is the run's last one by then: the verdict is the move's own, not read from the run.
func TestAReplayedWrongAnswerIsStillWrong(t *testing.T) {
	t.Parallel()
	p := newPuzzleTable(t)
	puz := p.riddle(t, "Enigma")
	p.show(t, puz.GetId())
	key := newKey()
	first, err := p.moveKey(t, p.caio, puz.GetId(), riddleMove("escuridão"), key)
	if err != nil || first.GetReplayed() || !first.GetWrong() {
		t.Fatalf("wrong answer = %v, %v; want wrong", first, err)
	}
	// Another player's wrong answer becomes the run's last move.
	if other := p.mustMove(t, p.ana, puz.GetId(), riddleMove("nada")); !other.GetWrong() {
		t.Fatalf("the other wrong answer = %v", other)
	}
	again, err := p.moveKey(t, p.caio, puz.GetId(), riddleMove("escuridão"), key)
	if err != nil || !again.GetReplayed() || !again.GetWrong() {
		t.Fatalf("replay = %v, %v; want replayed and wrong", again, err)
	}
	if again.GetRun().GetLastMove().GetCharacterName() == "Toren" {
		t.Errorf("the run's last move is the replayed one, so the test proves nothing: %v", again.GetRun().GetLastMove())
	}
	// A right answer is not wrong.
	right := p.mustMove(t, p.bia, puz.GetId(), riddleMove("sombra"))
	if right.GetWrong() {
		t.Errorf("a right answer came back wrong: %v", right)
	}
}
