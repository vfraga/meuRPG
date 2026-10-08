package play

import (
	"context"
	"crypto/rand"
	"encoding/binary"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"strings"
	"sync"
	"time"
	"unicode/utf8"

	"connectrpc.com/connect"
	"github.com/jackc/pgx/v5"
	"google.golang.org/protobuf/encoding/protojson"
	"google.golang.org/protobuf/proto"

	playv1 "github.com/PuraFome/meuRPG/backend/gen/meurpg/play/v1"
	"github.com/PuraFome/meuRPG/backend/internal/authz"
	"github.com/PuraFome/meuRPG/backend/internal/platform/safego"
	"github.com/PuraFome/meuRPG/backend/internal/play/live"
	"github.com/PuraFome/meuRPG/backend/internal/play/playdb"
)

// Puzzles (MR-038, RN-27, Etapa 10, slice 10.7a): the master makes them in the
// campaign, shows them in the open session, and the players solve them live. The
// files:
//
//   - puzzles.go: this file, the pieces every kind and every call share: the
//     stored forms, the errors, the checks on what the master writes, the stream
//     hint.
//   - puzzles_kinds.go: one puzzleKind per kind (lights, lock, pillars, riddle,
//     sequence, cipher), the only place that knows what a kind is.
//   - puzzles_rules.go: what is the same for every kind and is the master's to set:
//     "Ao errar" (a trap, the attempts, the limits), the hints won by a skill check
//     and the split information; and the limits worked out from the run.
//   - puzzles_master.go: the campaign's puzzles (create, edit, list, archive).
//   - puzzles_session.go: the master's session calls (show, reset, new start,
//     close, hints, the live view).
//   - puzzles_play.go: what a player reads, and a move (with "Ao resolver" and "Ao
//     errar").
//   - puzzles_hints.go: a player's try for a hint by a skill check.
//
// The rules are package rules/puzzle (pure); the three tables are puzzles (the
// master's, never read by a player: RN-10), puzzle_runs (a session's live state)
// and puzzle_moves (every move, with its idempotency key). The session's events
// keep only shown, solved, reset and closed (ADR-0007); a hint won by a skill check
// has its own table, puzzle_hint_tries, and a trap that a wrong move fires writes the
// trap's own event (`trap_triggered`).

// PuzzleMaps is what a solved puzzle asks of the maps module ("Ao resolver"): the
// doors, the points and the clues are its own. maps.Service implements it
// (puzzleseam.go there); cmd/api connects it with SetPuzzleMaps. Every method
// takes the transaction of the call (nil: the pool) and no caller: they run after
// this package's own authorization. A `not_found` Connect error means the target is
// not the campaign's, or not there any more.
type PuzzleMaps interface {
	// PuzzleCheckDoor says whether the square of the campaign's map has a door.
	PuzzleCheckDoor(ctx context.Context, tx pgx.Tx, campaignID, mapID string, col, row int) error
	// PuzzleOpenDoor opens that door whatever its state was, and says whether it
	// changed one. DoorsChanged tells the watchers after the commit.
	PuzzleOpenDoor(ctx context.Context, tx pgx.Tx, campaignID, mapID string, col, row int) (bool, error)
	// DoorsChanged tells the watchers, after the commit, that doors of the map
	// were opened.
	DoorsChanged(ctx context.Context, campaignID, mapID string)
	// PuzzleCheckPoint says whether the point is on the campaign's map and is one a
	// puzzle can reveal (not a light).
	PuzzleCheckPoint(ctx context.Context, tx pgx.Tx, campaignID, mapID, pointID string) error
	// PuzzleRevealPoint reveals the point to the players and says whether it
	// changed, with its name when the players see the point (the session shows
	// currentMapID, or the map was revealed), "" otherwise; the function it returns
	// tells the watchers after the commit.
	PuzzleRevealPoint(ctx context.Context, tx pgx.Tx, campaignID, mapID, pointID, currentMapID string) (changed bool, name string, after func(context.Context), err error)
	// PuzzleCheckClue says whether the clue is on a scene point of the campaign.
	PuzzleCheckClue(ctx context.Context, tx pgx.Tx, campaignID, clueID string) error
	// PuzzleRevealClue gives the clue to the character's player and says whether
	// they did not have it; the function it returns tells them after the commit.
	PuzzleRevealClue(ctx context.Context, tx pgx.Tx, campaignID, clueID, characterID, userID string, at time.Time) (bool, func(context.Context), error)
	// PuzzleClueFoundBy says whether the player has found the clue: a cipher shows
	// where its key is only once they have. False, not an error, for a clue that is gone.
	PuzzleClueFoundBy(ctx context.Context, tx pgx.Tx, campaignID, clueID, userID string) (bool, error)
	// PuzzleTrapSeenBy says whether the caller sees the trap point as the maps do: its map
	// is on their screen and, on a fog map, they see or remember its square. A trap that
	// fired is public to who sees it. It reads through the pool: never call it inside a
	// transaction.
	PuzzleTrapSeenBy(ctx context.Context, m authz.Membership, mapID, pointID string) (bool, error)
}

// puzzleDeps is what the puzzle calls need besides the rest of the Service. The
// zero value is usable but for maps: without it, a puzzle whose "Ao resolver" is
// not "only notify" cannot be made.
type puzzleDeps struct {
	maps PuzzleMaps
	// seed draws the seed of a generated start. Nil is crypto/rand; tests set it.
	seed func() uint64
	// hints sends puzzle_changed at most once every hintEvery per puzzle; the changes
	// in between merge into one hint sent when the interval ends (zero: the default).
	hints     puzzleGate
	hintEvery time.Duration
}

// defaultHintEvery is how often a puzzle's hint may go out, as map_changed's is
// throttled: a burst of moves is one read, and the last change is never left
// unannounced.
const defaultHintEvery = 250 * time.Millisecond

// puzzleGate lets a key send at most one hint every interval: the first goes at
// once, and the ones that follow within the interval merge into one sent when it ends.
type puzzleGate struct {
	mu   sync.Mutex
	keys map[string]*puzzleGateState
	// now and after are the clock; nil means the real one (time.Now, time.AfterFunc).
	// Tests set them to drive the gate without sleeping.
	now    func() time.Time
	after  func(time.Duration, func())
	logger *slog.Logger // for a panic in a send that runs on the timer's own goroutine
}

type puzzleGateState struct {
	last    time.Time
	waiting bool // a hint is scheduled for the end of the interval
}

func (g *puzzleGate) clock() time.Time {
	if g.now != nil {
		return g.now()
	}
	return time.Now()
}

func (g *puzzleGate) schedule(d time.Duration, f func()) {
	if g.after != nil {
		g.after(d, f)
		return
	}
	time.AfterFunc(d, func() {
		defer safego.Recover(g.logger, "puzzle hint")
		f()
	})
}

func (g *puzzleGate) fire(key string, every time.Duration, send func()) {
	g.mu.Lock()
	if g.keys == nil {
		g.keys = map[string]*puzzleGateState{}
	}
	now := g.clock()
	if len(g.keys) > 256 { // the table stays small: idle keys go
		for k, st := range g.keys {
			if k != key && !st.waiting && now.Sub(st.last) > time.Minute {
				delete(g.keys, k)
			}
		}
	}
	st := g.keys[key]
	if st == nil {
		st = &puzzleGateState{}
		g.keys[key] = st
	}
	if st.waiting { // a hint is already waiting: this change merges into it
		g.mu.Unlock()
		return
	}
	if wait := every - now.Sub(st.last); wait > 0 {
		st.waiting = true
		g.schedule(wait, func() {
			g.mu.Lock()
			st.waiting, st.last = false, g.clock()
			g.mu.Unlock()
			send()
		})
		g.mu.Unlock()
		return
	}
	st.last = now
	g.mu.Unlock()
	send()
}

// SetPuzzleMaps connects the maps module, for "Ao resolver". cmd/api calls it once
// maps exists, before the server starts.
func (s *Service) SetPuzzleMaps(m PuzzleMaps) { s.puzzles.maps = m }

// newSeed draws a seed that fits a positive int64.
func (s *Service) newSeed() int64 {
	var n uint64
	if s.puzzles.seed != nil {
		n = s.puzzles.seed()
	} else {
		var b [8]byte
		if _, err := rand.Read(b[:]); err != nil {
			panic(fmt.Sprintf("play: no random source: %v", err)) // crypto/rand does not fail on a working system
		}
		n = binary.BigEndian.Uint64(b[:])
	}
	return int64(n >> 1)
}

// The limits of what the master writes.
const (
	maxPuzzleName   = 80
	maxPuzzleClue   = 500
	maxPuzzleHint   = 300
	maxHints        = 10
	maxSolveMessage = 200
)

// The stored names of the "Ao resolver" actions (puzzles.solve_action).
const (
	solveNotify      = "notify"
	solveOpenDoor    = "open_door"
	solveRevealPoint = "reveal_point"
	solveRevealClue  = "reveal_clue"
)

// The session events of a puzzle (migration 00136): IDs only.
const (
	eventPuzzleShown  = "puzzle_shown"
	eventPuzzleSolved = "puzzle_solved"
	eventPuzzleReset  = "puzzle_reset"
	eventPuzzleClosed = "puzzle_closed"
)

// puzzleEvent is the payload of the four events: which puzzle and run, and for a
// solve what the "Ao resolver" action did.
type puzzleEvent struct {
	PuzzleID string `json:"puzzle_id"`
	RunID    string `json:"run_id"`
	Outcome  string `json:"outcome,omitempty"`
}

// --- errors ---

// errKeyReused is the refusal of an idempotency key that was used for another change.
func errKeyReused() error {
	return connect.NewError(connect.CodeInvalidArgument, errors.New("idempotency_key was already used for another change"))
}

// puzzleBlocked is PuzzleService's failed_precondition, with the PuzzleBlocked
// detail the app reads.
func puzzleBlocked(reason playv1.PuzzleBlockedReason, msg string) error {
	err := connect.NewError(connect.CodeFailedPrecondition, errors.New(msg))
	if detail, detailErr := connect.NewErrorDetail(&playv1.PuzzleBlocked{Reason: reason}); detailErr == nil {
		err.AddDetail(detail)
	}
	return err
}

// puzzleInvalid is PuzzleService's invalid_argument, with the PuzzleInvalid detail:
// the reason and the request field that is wrong, never its value.
func puzzleInvalid(reason playv1.PuzzleInvalidReason, field, msg string) error {
	err := connect.NewError(connect.CodeInvalidArgument, errors.New(msg))
	if detail, detailErr := connect.NewErrorDetail(&playv1.PuzzleInvalid{Reason: reason, Field: field}); detailErr == nil {
		err.AddDetail(detail)
	}
	return err
}

func errNoOpenSessionForPuzzle() error {
	return puzzleBlocked(playv1.PuzzleBlockedReason_PUZZLE_BLOCKED_REASON_NO_OPEN_SESSION, "the campaign has no open game session")
}

func errPuzzleNotFound() error {
	return connect.NewError(connect.CodeNotFound, errors.New("puzzle not found"))
}

// parsePuzzleID reads a puzzle ID: not a UUID names nothing.
func parsePuzzleID(raw string) (string, error) {
	id, err := parseCombatID(raw, "puzzle")
	if err != nil {
		return "", errPuzzleNotFound()
	}
	return id, nil
}

// --- stored forms ---

var (
	protoOut = protojson.MarshalOptions{}
	protoIn  = protojson.UnmarshalOptions{DiscardUnknown: true} // slice 10.7b adds fields: older rows still read
)

// toJSON is the stored form of a message (the columns are JSONB).
func toJSON(m proto.Message) []byte {
	b, err := protoOut.Marshal(m)
	if err != nil {
		panic(fmt.Sprintf("play: cannot encode %T: %v", m, err)) // a message of our own protos always encodes
	}
	return b
}

// fromJSON reads a stored message back into m.
func fromJSON(b []byte, m proto.Message) error {
	if len(b) == 0 {
		return nil
	}
	if err := protoIn.Unmarshal(b, m); err != nil {
		return fmt.Errorf("decode a stored %T: %w", m, err)
	}
	return nil
}

// puzzleDef is a puzzle row, decoded.
type puzzleDef struct {
	row      playdb.Puzzle
	kind     puzzleKind
	config   *playv1.PuzzleConfig
	solution *playv1.PuzzleSolution
	start    *playv1.PuzzleState
	hints    []string
	onSolve  *playv1.PuzzleOnSolve
	// The master's other settings (slice 10.7b): the skill check that wins a hint (nil:
	// none), the split information and "Ao errar" (nil: nothing). None of them reaches a
	// player but as the counters and the part that are theirs (puzzles_view.go).
	hintCheck *playv1.PuzzleHintCheck
	parts     []*playv1.PuzzlePart
	onWrong   *playv1.PuzzleOnWrong
}

// storedPart is a part of the split information as puzzles.parts keeps it.
type storedPart struct {
	CharacterID string `json:"character_id"`
	Text        string `json:"text"`
}

// decodePuzzle reads the stored JSON of a puzzle row.
func decodePuzzle(row playdb.Puzzle) (puzzleDef, error) {
	d := puzzleDef{row: row, config: &playv1.PuzzleConfig{}, solution: &playv1.PuzzleSolution{}, start: &playv1.PuzzleState{}, onSolve: &playv1.PuzzleOnSolve{}}
	var err error
	if err = fromJSON(row.Config, d.config); err == nil {
		err = fromJSON(row.Solution, d.solution)
	}
	if err == nil {
		err = fromJSON(row.Start, d.start)
	}
	if err == nil {
		err = fromJSON(row.SolveTarget, d.onSolve)
	}
	if err != nil {
		return puzzleDef{}, err
	}
	if len(row.Hints) > 0 {
		if err := jsonUnmarshalStrings(row.Hints, &d.hints); err != nil {
			return puzzleDef{}, err
		}
	}
	if d.kind = kindOf(d.config); d.kind == nil {
		return puzzleDef{}, fmt.Errorf("puzzle %s has a kind this server does not know", row.ID)
	}
	d.onSolve.Action = solveActionOf(row.SolveAction)
	if row.HintSkill != nil && row.HintDc != nil {
		d.hintCheck = &playv1.PuzzleHintCheck{SkillKey: *row.HintSkill, Dc: *row.HintDc}
	}
	if len(row.Parts) > 0 {
		var parts []storedPart
		if err := json.Unmarshal(row.Parts, &parts); err != nil {
			return puzzleDef{}, fmt.Errorf("decode the stored parts: %w", err)
		}
		for _, p := range parts {
			d.parts = append(d.parts, &playv1.PuzzlePart{CharacterId: p.CharacterID, Text: p.Text})
		}
	}
	if len(row.OnWrong) > 0 {
		d.onWrong = &playv1.PuzzleOnWrong{}
		if err := fromJSON(row.OnWrong, d.onWrong); err != nil {
			return puzzleDef{}, err
		}
	}
	return d, nil
}

func solveActionOf(stored string) playv1.PuzzleSolveAction {
	switch stored {
	case solveOpenDoor:
		return playv1.PuzzleSolveAction_PUZZLE_SOLVE_ACTION_OPEN_DOOR
	case solveRevealPoint:
		return playv1.PuzzleSolveAction_PUZZLE_SOLVE_ACTION_REVEAL_POINT
	case solveRevealClue:
		return playv1.PuzzleSolveAction_PUZZLE_SOLVE_ACTION_REVEAL_CLUE
	}
	return playv1.PuzzleSolveAction_PUZZLE_SOLVE_ACTION_NOTIFY
}

func solveActionName(a playv1.PuzzleSolveAction) string {
	switch a {
	case playv1.PuzzleSolveAction_PUZZLE_SOLVE_ACTION_OPEN_DOOR:
		return solveOpenDoor
	case playv1.PuzzleSolveAction_PUZZLE_SOLVE_ACTION_REVEAL_POINT:
		return solveRevealPoint
	case playv1.PuzzleSolveAction_PUZZLE_SOLVE_ACTION_REVEAL_CLUE:
		return solveRevealClue
	}
	return solveNotify
}

// --- what the master writes ---

// puzzleInput is what CreatePuzzle and UpdatePuzzle carry.
type puzzleInput struct {
	name     string
	config   *playv1.PuzzleConfig
	solution *playv1.PuzzleSolution
	start    *playv1.PuzzleState
	seed     int64
	clue     string
	hints    []string
	onSolve  *playv1.PuzzleOnSolve
	// Slice 10.7b: the hint check, the split information and "Ao errar".
	hintCheck *playv1.PuzzleHintCheck
	parts     []*playv1.PuzzlePart
	onWrong   *playv1.PuzzleOnWrong
}

// checkedText trims the text and checks its length in characters.
func checkedText(raw string, lo, hi int, reason playv1.PuzzleInvalidReason, field string) (string, error) {
	text := strings.TrimSpace(raw)
	if n := utf8.RuneCountInString(text); n < lo || n > hi {
		return "", puzzleInvalid(reason, field, fmt.Sprintf("%s must have %d to %d characters", field, lo, hi))
	}
	return text, nil
}

// checkTexts checks the name, the clue and the hints.
func (in *puzzleInput) checkTexts() error {
	var err error
	if in.name, err = checkedText(in.name, 1, maxPuzzleName, playv1.PuzzleInvalidReason_PUZZLE_INVALID_REASON_NAME, "name"); err != nil {
		return err
	}
	if in.clue, err = checkedText(in.clue, 0, maxPuzzleClue, playv1.PuzzleInvalidReason_PUZZLE_INVALID_REASON_TEXT, "clue"); err != nil {
		return err
	}
	if len(in.hints) > maxHints {
		return puzzleInvalid(playv1.PuzzleInvalidReason_PUZZLE_INVALID_REASON_TEXT, "hints", fmt.Sprintf("a puzzle has at most %d hints", maxHints))
	}
	hints := make([]string, 0, len(in.hints))
	for _, h := range in.hints {
		text, err := checkedText(h, 1, maxPuzzleHint, playv1.PuzzleInvalidReason_PUZZLE_INVALID_REASON_TEXT, "hints")
		if err != nil {
			return err
		}
		hints = append(hints, text)
	}
	in.hints = hints
	return nil
}

// checkOnSolve normalizes "Ao resolver" and checks, inside tx, that its target is
// the campaign's: a door with a square, a point of a map, a clue. It returns the
// stored action and target (nil for NOTIFY).
func (s *Service) checkOnSolve(ctx context.Context, tx pgx.Tx, campaignID string, on *playv1.PuzzleOnSolve) (string, *playv1.PuzzleOnSolve, error) {
	bad := func(field string) error {
		return puzzleInvalid(playv1.PuzzleInvalidReason_PUZZLE_INVALID_REASON_TARGET, field, field+" is not valid for this action")
	}
	action := on.GetAction()
	if action == playv1.PuzzleSolveAction_PUZZLE_SOLVE_ACTION_UNSPECIFIED {
		action = playv1.PuzzleSolveAction_PUZZLE_SOLVE_ACTION_NOTIFY
	}
	message, err := checkedText(on.GetMessage(), 0, maxSolveMessage, playv1.PuzzleInvalidReason_PUZZLE_INVALID_REASON_TEXT, "on_solve.message")
	if err != nil {
		return "", nil, err
	}
	if on == nil {
		on = &playv1.PuzzleOnSolve{}
	}
	on = proto.Clone(on).(*playv1.PuzzleOnSolve) //nolint:forcetypeassert // Clone returns the same type
	on.Action, on.Message = action, message
	var check func() error
	switch action {
	case playv1.PuzzleSolveAction_PUZZLE_SOLVE_ACTION_NOTIFY:
		if on.GetTarget() != nil {
			return "", nil, bad("on_solve.target")
		}
		if message == "" {
			return solveNotify, nil, nil
		}
		return solveNotify, on, nil // only the text to keep
	case playv1.PuzzleSolveAction_PUZZLE_SOLVE_ACTION_OPEN_DOOR:
		t := on.GetDoor()
		if t == nil {
			return "", nil, bad("on_solve.door")
		}
		check = func() error {
			return s.puzzles.maps.PuzzleCheckDoor(ctx, tx, campaignID, t.GetMapId(), int(t.GetCol()), int(t.GetRow()))
		}
	case playv1.PuzzleSolveAction_PUZZLE_SOLVE_ACTION_REVEAL_POINT:
		t := on.GetPoint()
		if t == nil {
			return "", nil, bad("on_solve.point")
		}
		check = func() error {
			return s.puzzles.maps.PuzzleCheckPoint(ctx, tx, campaignID, t.GetMapId(), t.GetPointId())
		}
	case playv1.PuzzleSolveAction_PUZZLE_SOLVE_ACTION_REVEAL_CLUE:
		t := on.GetClue()
		if t == nil {
			return "", nil, bad("on_solve.clue")
		}
		check = func() error { return s.puzzles.maps.PuzzleCheckClue(ctx, tx, campaignID, t.GetClueId()) }
	default:
		return "", nil, bad("on_solve.action")
	}
	if s.puzzles.maps == nil {
		return "", nil, errors.New("play: the maps module is not connected to the puzzles")
	}
	if err := check(); err != nil {
		if connect.CodeOf(err) == connect.CodeNotFound {
			return "", nil, bad("on_solve.target")
		}
		return "", nil, err
	}
	return solveActionName(action), on, nil
}

// --- the stream ---

// publishPuzzleChanged tells everyone in the session that a puzzle changed. The
// hint names the puzzle and nothing else (RN-10): each app reads the puzzle again,
// as a player or as the master. A hint that waits in a stream's queue is not queued
// twice, and a time gate (puzzleGate) lets one hint out per puzzle every 250 ms.
func (s *Service) publishPuzzleChanged(campaignID, puzzleID string) {
	every := s.puzzles.hintEvery
	if every <= 0 {
		every = defaultHintEvery
	}
	s.puzzles.hints.fire(puzzleID, every, func() { s.sendPuzzleChanged(campaignID, puzzleID) })
}

// sendPuzzleChanged is the hint itself.
func (s *Service) sendPuzzleChanged(campaignID, puzzleID string) {
	s.hub.Publish(campaignID, live.Event{
		Audience: live.Audience{Everyone: true},
		Message: &playv1.WatchGameSessionResponse{Event: &playv1.WatchGameSessionResponse_PuzzleChanged_{
			PuzzleChanged: &playv1.WatchGameSessionResponse_PuzzleChanged{PuzzleId: puzzleID},
		}},
		Coalesce: "puzzle:" + puzzleID,
	})
}

// puzzleSessionEvent appends one of the four puzzle events inside tx. The caller
// holds the session's row lock, so the events get their numbers in order.
func puzzleSessionEvent(ctx context.Context, q *playdb.Queries, session playdb.GameSession, kind string, actorUserID string, characterID *string, ev puzzleEvent, now time.Time) error {
	body, err := jsonMarshal(ev)
	if err != nil {
		return fmt.Errorf("encode the event payload: %w", err)
	}
	seq, err := q.NextSessionEventSeq(ctx, session.ID)
	if err != nil {
		return fmt.Errorf("next event number: %w", err)
	}
	if _, err := q.InsertSessionEvent(ctx, playdb.InsertSessionEventParams{
		GameSessionID: session.ID, Seq: seq, Kind: kind, ActorUserID: &actorUserID, CharacterID: characterID, Payload: body, CreatedAt: now,
	}); err != nil {
		return fmt.Errorf("insert session event: %w", err)
	}
	return nil
}

// lockOpenSession locks the campaign's open session inside tx, the first step of
// every change to a run, or fails with NO_OPEN_SESSION.
func lockOpenSession(ctx context.Context, q *playdb.Queries, campaignID string) (playdb.GameSession, error) {
	session, err := q.GetOpenGameSessionForUpdate(ctx, campaignID)
	if errors.Is(err, pgx.ErrNoRows) {
		return playdb.GameSession{}, errNoOpenSessionForPuzzle()
	}
	if err != nil {
		return playdb.GameSession{}, fmt.Errorf("lock the open session: %w", err)
	}
	return session, nil
}

// requireMaster is the check at the top of the master's handlers.
func requireMaster(ctx context.Context, campaignID string) (authz.Membership, error) {
	return authz.RequireCampaignRole(ctx, campaignID, authz.RoleMaster)
}

// jsonMarshal and jsonUnmarshalStrings are encoding/json, named here so the
// puzzle files read the same.
func jsonMarshal(v any) ([]byte, error) { return json.Marshal(v) }

func jsonUnmarshalStrings(b []byte, out *[]string) error {
	if err := json.Unmarshal(b, out); err != nil {
		return fmt.Errorf("decode the stored hints: %w", err)
	}
	return nil
}

// The stored names of what "Ao resolver" did (puzzle_runs.solve_outcome).
var outcomeNames = map[playv1.PuzzleSolveOutcome]string{
	playv1.PuzzleSolveOutcome_PUZZLE_SOLVE_OUTCOME_NOTIFIED:               "notified",
	playv1.PuzzleSolveOutcome_PUZZLE_SOLVE_OUTCOME_DOOR_OPENED:            "door_opened",
	playv1.PuzzleSolveOutcome_PUZZLE_SOLVE_OUTCOME_DOOR_NOT_CLOSED:        "door_not_closed",
	playv1.PuzzleSolveOutcome_PUZZLE_SOLVE_OUTCOME_POINT_REVEALED:         "point_revealed",
	playv1.PuzzleSolveOutcome_PUZZLE_SOLVE_OUTCOME_POINT_ALREADY_REVEALED: "point_already_revealed",
	playv1.PuzzleSolveOutcome_PUZZLE_SOLVE_OUTCOME_CLUE_REVEALED:          "clue_revealed",
	playv1.PuzzleSolveOutcome_PUZZLE_SOLVE_OUTCOME_CLUE_ALREADY_HAD:       "clue_already_had",
	playv1.PuzzleSolveOutcome_PUZZLE_SOLVE_OUTCOME_TARGET_GONE:            "target_gone",
}

func outcomeName(o playv1.PuzzleSolveOutcome) string { return outcomeNames[o] }

func outcomeOf(stored string) playv1.PuzzleSolveOutcome {
	for o, name := range outcomeNames {
		if name == stored {
			return o
		}
	}
	return playv1.PuzzleSolveOutcome_PUZZLE_SOLVE_OUTCOME_UNSPECIFIED
}

// puzzleSession is the campaign's open session (not locked: for the reads), or
// NO_OPEN_SESSION.
func (s *Service) puzzleSession(ctx context.Context, campaignID string) (playdb.GameSession, error) {
	session, err := s.queries.GetOpenGameSession(ctx, campaignID)
	if errors.Is(err, pgx.ErrNoRows) {
		return playdb.GameSession{}, errNoOpenSessionForPuzzle()
	}
	if err != nil {
		return playdb.GameSession{}, s.dbError(ctx, "find the open session", err)
	}
	return session, nil
}
