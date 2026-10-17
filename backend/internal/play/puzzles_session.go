package play

import (
	"context"
	"errors"
	"fmt"
	"time"

	"connectrpc.com/connect"
	"github.com/jackc/pgx/v5"

	playv1 "github.com/PuraFome/meuRPG/backend/gen/meurpg/play/v1"
	"github.com/PuraFome/meuRPG/backend/internal/authz"
	"github.com/PuraFome/meuRPG/backend/internal/platform/db"
	"github.com/PuraFome/meuRPG/backend/internal/play/playdb"
	"github.com/PuraFome/meuRPG/backend/internal/rules/puzzle"
)

// The master's calls on a puzzle in the open session: show it, put it back at its
// start, draw another start, close it, release a hint, and read it live. Every
// change takes the same steps, in the same order: lock the session's row (so the
// events get their numbers in order, and a move waits for the change in progress),
// lock the puzzle, lock the run. Taking the locks in one order everywhere is what
// keeps two changes from waiting on each other.

// runTx is one change to a puzzle's run, inside its transaction.
type runTx struct {
	m       authz.Membership
	q       *playdb.Queries
	session playdb.GameSession
	def     puzzleDef
	run     *playdb.PuzzleRun // nil: the puzzle has no run in this session yet
	now     time.Time
	tell    bool // send puzzle_changed to the session once the transaction commits
	// later are the moments, from now, when the session needs one more puzzle_changed
	// (the next step of a sequence being played, the end of a time limit): nothing
	// changes in the database then, but each app has to read the run again.
	later []time.Duration
	// roundBegan says the change began a round: the time limit starts to run.
	roundBegan bool
}

// params is the run as a SavePuzzleRun that changes nothing.
func (c *runTx) params() playdb.SavePuzzleRunParams { return paramsOf(c.run) }

// paramsOf is a run as a SavePuzzleRun that changes nothing.
func paramsOf(r *playdb.PuzzleRun) playdb.SavePuzzleRunParams {
	return playdb.SavePuzzleRunParams{
		ID: r.ID, Seed: r.Seed, Start: r.Start, State: r.State, ReleasedHints: r.ReleasedHints, ShownAt: r.ShownAt,
		ClosedAt: r.ClosedAt, SolvedAt: r.SolvedAt, SolvedByCharacterID: r.SolvedByCharacterID, SolveOutcome: r.SolveOutcome,
		LastMoverCharacterID: r.LastMoverCharacterID, LastMove: r.LastMove, LastMovedAt: r.LastMovedAt,
		MovesMade: r.MovesMade, Revision: r.Revision, SolveMessage: r.SolveMessage,
		Plays: r.Plays, PlayStartedAt: r.PlayStartedAt, RoundStartSeq: r.RoundStartSeq, RoundStartedAt: r.RoundStartedAt,
	}
}

// save writes the changed run, one revision up.
func (c *runTx) save(ctx context.Context, p playdb.SavePuzzleRunParams) error {
	p.Revision = c.run.Revision + 1
	p.UpdatedAt = c.now
	row, err := c.q.SavePuzzleRun(ctx, p)
	if err != nil {
		return fmt.Errorf("save the puzzle's run: %w", err)
	}
	c.run = &row
	return nil
}

// toStart puts the pieces back where the run started and forgets that it was
// solved and who moved last; the hints stay released and the moves stay in the
// history. It begins a new round (what "Ao errar" counts in: the attempts, the limit
// of moves, the time limit, which counts from clock), which is also how a stopped puzzle starts again, and ends
// a sequence's play that is running; the plays already made are still counted.
func toStart(p *playdb.SavePuzzleRunParams, clock *time.Time) {
	p.State = p.Start
	p.SolvedAt, p.SolvedByCharacterID, p.SolveOutcome, p.SolveMessage = nil, nil, nil, nil
	p.LastMoverCharacterID, p.LastMove, p.LastMovedAt = nil, nil, nil
	p.RoundStartSeq, p.RoundStartedAt, p.PlayStartedAt = p.MovesMade, clock, nil
}

// clock is when the round's time limit starts to count if the round begins now: at once,
// except for a sequence, whose clock starts at its first play in the round (nil here), so
// the time never runs out before anyone may play.
func (c *runTx) clock() *time.Time {
	if _, ok := c.def.kind.(sequenceKind); ok {
		return nil
	}
	return &c.now
}

// event writes one of the puzzle's session events about the run.
func (c *runTx) event(ctx context.Context, kind string) error {
	return puzzleSessionEvent(ctx, c.q, c.session, kind, c.m.UserID, nil, puzzleEvent{PuzzleID: c.def.row.ID, RunID: c.run.ID}, c.now)
}

// changeRun runs the master's change in one transaction, and answers with the
// puzzle as the master now sees it.
func (s *Service) changeRun(ctx context.Context, m authz.Membership, rawID string, change func(c *runTx) error) (*playv1.MasterPuzzleRun, error) {
	id, err := parsePuzzleID(rawID)
	if err != nil {
		return nil, err
	}
	var c *runTx
	err = db.InTx(ctx, s.pool, func(tx pgx.Tx) error {
		var err error
		if c, err = s.lockRunTx(ctx, s.queriesIn(tx), m, id); err != nil {
			return err
		}
		return change(c)
	})
	if err != nil {
		return nil, s.dbError(ctx, "change a puzzle", err)
	}
	if c.tell {
		s.tellRunChange(c, id)
	}
	return s.masterView(ctx, nil, m.CampaignID, c.def, c.run)
}

// lockRunTx takes the locks of a change in their one order: the session's row, the puzzle, the run
// (none yet: the puzzle was never shown in this session).
func (s *Service) lockRunTx(ctx context.Context, q *playdb.Queries, m authz.Membership, id string) (*runTx, error) {
	session, err := lockOpenSessionOr(ctx, q, m.CampaignID, errNoOpenSessionForPuzzle)
	if err != nil {
		return nil, err
	}
	row, err := q.GetPuzzleForUpdate(ctx, playdb.GetPuzzleForUpdateParams{CampaignID: m.CampaignID, ID: id})
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, errPuzzleNotFound()
	}
	if err != nil {
		return nil, fmt.Errorf("find puzzle: %w", err)
	}
	def, err := decodePuzzle(row)
	if err != nil {
		return nil, err
	}
	c := &runTx{m: m, q: q, session: session, def: def, now: s.now()}
	run, err := q.GetPuzzleRunForUpdate(ctx, playdb.GetPuzzleRunForUpdateParams{GameSessionID: session.ID, PuzzleID: id})
	switch {
	case err == nil:
		c.run = &run
	case !errors.Is(err, pgx.ErrNoRows):
		return nil, fmt.Errorf("find the puzzle's run: %w", err)
	}
	return c, nil
}

// tellRunChange sends puzzle_changed to the session once the change is committed, and again at each
// moment the change asked for.
func (s *Service) tellRunChange(c *runTx, id string) {
	campaignID := c.m.CampaignID
	s.publishPuzzleChanged(campaignID, id)
	if c.roundBegan {
		if dl := deadlineOf(c.def, runOrZero(c.run)); dl != nil {
			c.later = append(c.later, dl.Sub(c.now)) // the apps read the run again when the time is up
		}
	}
	for _, after := range c.later {
		if after > 0 {
			s.puzzles.hints.schedule(after, func() { s.sendPuzzleChanged(campaignID, id) })
		}
	}
}

func runOrZero(r *playdb.PuzzleRun) playdb.PuzzleRun {
	if r == nil {
		return playdb.PuzzleRun{}
	}
	return *r
}

// masterView is the master's message for a puzzle and its run (nil: none).
func (s *Service) masterView(ctx context.Context, tx pgx.Tx, campaignID string, d puzzleDef, run *playdb.PuzzleRun) (*playv1.MasterPuzzleRun, error) {
	var runs []playdb.PuzzleRun
	if run != nil {
		runs = append(runs, *run)
	}
	names, err := s.namesOfRuns(ctx, tx, campaignID, runs...)
	if err != nil {
		return nil, s.dbError(ctx, "read the puzzle's run", err)
	}
	shown, err := s.shownSet(ctx, campaignID)
	if err != nil {
		return nil, s.dbError(ctx, "read the puzzle's run", err)
	}
	var ex masterExtra
	if run != nil {
		if ex, err = s.masterExtras(ctx, tx, campaignID, d, *run); err != nil {
			return nil, s.dbError(ctx, "read the puzzle's run", err)
		}
	}
	out, err := masterRunProto(d, run, names, shown[d.row.ID], ex)
	if err != nil {
		return nil, s.dbError(ctx, "read the puzzle's run", err)
	}
	if err := s.flagParts(ctx, campaignID, out.GetPuzzle()); err != nil {
		return nil, s.dbError(ctx, "read the puzzle's run", err)
	}
	return out, nil
}

// ShowPuzzle implements playv1connect.PuzzleServiceHandler.
func (s *Service) ShowPuzzle(
	ctx context.Context,
	req *connect.Request[playv1.ShowPuzzleRequest],
) (*connect.Response[playv1.ShowPuzzleResponse], error) {
	m, err := requireMaster(ctx, req.Msg.GetCampaignId())
	if err != nil {
		return nil, err
	}
	run, err := s.changeRun(ctx, m, req.Msg.GetPuzzleId(), func(c *runTx) error { return c.show(ctx) })
	if err != nil {
		return nil, err
	}
	return connect.NewResponse(&playv1.ShowPuzzleResponse{Run: run}), nil
}

// show makes the puzzle visible to the players: the run is made, or the prepared or closed one is shown
// at its start. A puzzle that is shown already stays as it is.
func (c *runTx) show(ctx context.Context) error {
	if c.def.row.ArchivedAt != nil {
		return puzzleBlocked(playv1.PuzzleBlockedReason_PUZZLE_BLOCKED_REASON_ARCHIVED, "the puzzle is archived")
	}
	switch {
	case c.run == nil:
		if err := c.insertShownRun(ctx); err != nil {
			return err
		}
	case visible(c.run):
		return nil // shown already: nothing changes
	default:
		if err := c.showAgain(ctx); err != nil {
			return err
		}
	}
	c.tell = true
	return c.event(ctx, eventPuzzleShown)
}

// insertShownRun makes the run of a puzzle never shown in this session, shown now.
func (c *runTx) insertShownRun(ctx context.Context) error {
	row, err := c.q.InsertPuzzleRun(ctx, playdb.InsertPuzzleRunParams{
		GameSessionID: c.session.ID, PuzzleID: c.def.row.ID, Seed: c.def.row.Seed, Start: c.def.row.Start, ShownAt: &c.now, RoundStartedAt: c.clock(), CreatedAt: c.now,
	})
	if err != nil {
		return fmt.Errorf("make the puzzle's run: %w", err)
	}
	c.run = &row
	c.roundBegan = true
	return nil
}

// showAgain shows a run prepared with a new start, or closed, at its start.
func (c *runTx) showAgain(ctx context.Context) error {
	p := c.params()
	p.ShownAt, p.ClosedAt = &c.now, nil
	if c.run.ClosedAt != nil {
		toStart(&p, c.clock())
	} else {
		p.RoundStartSeq, p.RoundStartedAt = p.MovesMade, c.clock() // the first round begins now
	}
	c.roundBegan = true
	return c.save(ctx, p)
}

// needsVisibleRun is the refusal of a change that needs the players to see the
// puzzle.
func needsVisibleRun(c *runTx) error {
	if !visible(c.run) {
		return puzzleBlocked(playv1.PuzzleBlockedReason_PUZZLE_BLOCKED_REASON_NOT_SHOWN, "the puzzle is not shown in this session")
	}
	return nil
}

// notStale is the refusal of a change whose caller read an older run than the one there is
// (expected is the revision they read, 0 for no check): the change would act on a state the
// master did not see, or repeat one that a call whose answer was lost already made.
func notStale(c *runTx, expected int32) error {
	if expected != 0 && (c.run == nil || c.run.Revision != expected) {
		return puzzleBlocked(playv1.PuzzleBlockedReason_PUZZLE_BLOCKED_REASON_STALE_REVISION, "the puzzle changed since you read it: read it again")
	}
	return nil
}

// ResetPuzzle implements playv1connect.PuzzleServiceHandler.
func (s *Service) ResetPuzzle(
	ctx context.Context,
	req *connect.Request[playv1.ResetPuzzleRequest],
) (*connect.Response[playv1.ResetPuzzleResponse], error) {
	m, err := requireMaster(ctx, req.Msg.GetCampaignId())
	if err != nil {
		return nil, err
	}
	run, err := s.changeRun(ctx, m, req.Msg.GetPuzzleId(), func(c *runTx) error {
		if err := notStale(c, req.Msg.GetExpectedRevision()); err != nil {
			return err
		}
		if err := needsVisibleRun(c); err != nil {
			return err
		}
		p := c.params()
		toStart(&p, c.clock())
		if err := c.save(ctx, p); err != nil {
			return err
		}
		c.tell, c.roundBegan = true, true
		return c.event(ctx, eventPuzzleReset)
	})
	if err != nil {
		return nil, err
	}
	return connect.NewResponse(&playv1.ResetPuzzleResponse{Run: run}), nil
}

// ReseedPuzzle implements playv1connect.PuzzleServiceHandler.
func (s *Service) ReseedPuzzle(
	ctx context.Context,
	req *connect.Request[playv1.ReseedPuzzleRequest],
) (*connect.Response[playv1.ReseedPuzzleResponse], error) {
	m, err := requireMaster(ctx, req.Msg.GetCampaignId())
	if err != nil {
		return nil, err
	}
	run, err := s.changeRun(ctx, m, req.Msg.GetPuzzleId(), func(c *runTx) error {
		if err := notStale(c, req.Msg.GetExpectedRevision()); err != nil {
			return err
		}
		return s.reseed(ctx, c)
	})
	if err != nil {
		return nil, err
	}
	return connect.NewResponse(&playv1.ReseedPuzzleResponse{Run: run}), nil
}

// reseed draws another start for the puzzle and puts its run at it.
func (s *Service) reseed(ctx context.Context, c *runTx) error {
	if !c.def.kind.generates() {
		return puzzleBlocked(playv1.PuzzleBlockedReason_PUZZLE_BLOCKED_REASON_NO_GENERATED_START, "this kind has no generated start")
	}
	seed := s.newSeed()
	start, err := c.def.kind.generate(c.def, seed)
	if err != nil {
		return err
	}
	startJSON := toJSON(start)
	if c.run == nil {
		return c.insertPreparedRun(ctx, seed, startJSON)
	}
	return c.restartAt(ctx, seed, startJSON)
}

// insertPreparedRun prepares the run at the new start of a puzzle not shown yet. The players see
// nothing until ShowPuzzle.
func (c *runTx) insertPreparedRun(ctx context.Context, seed int64, start []byte) error {
	row, err := c.q.InsertPuzzleRun(ctx, playdb.InsertPuzzleRunParams{
		GameSessionID: c.session.ID, PuzzleID: c.def.row.ID, Seed: seed, Start: start, CreatedAt: c.now,
	})
	if err != nil {
		return fmt.Errorf("make the puzzle's run: %w", err)
	}
	c.run = &row
	return nil
}

// restartAt puts the existing run at the new start; the players are told when they see the puzzle.
func (c *runTx) restartAt(ctx context.Context, seed int64, start []byte) error {
	p := c.params()
	p.Seed, p.Start = seed, start
	toStart(&p, c.clock())
	if c.run.ShownAt == nil {
		p.RoundStartedAt = nil // prepared, not shown: the round begins when it is shown
	}
	if err := c.save(ctx, p); err != nil {
		return err
	}
	if !visible(c.run) {
		return nil
	}
	c.tell, c.roundBegan = true, true
	return c.event(ctx, eventPuzzleReset)
}

// ClosePuzzle implements playv1connect.PuzzleServiceHandler.
func (s *Service) ClosePuzzle(
	ctx context.Context,
	req *connect.Request[playv1.ClosePuzzleRequest],
) (*connect.Response[playv1.ClosePuzzleResponse], error) {
	m, err := requireMaster(ctx, req.Msg.GetCampaignId())
	if err != nil {
		return nil, err
	}
	run, err := s.changeRun(ctx, m, req.Msg.GetPuzzleId(), func(c *runTx) error {
		if c.run != nil && c.run.ShownAt != nil && c.run.ClosedAt != nil {
			return nil // closed already: nothing changes
		}
		if err := needsVisibleRun(c); err != nil {
			return err
		}
		p := c.params()
		p.ClosedAt = &c.now
		if err := c.save(ctx, p); err != nil {
			return err
		}
		c.tell = true
		return c.event(ctx, eventPuzzleClosed)
	})
	if err != nil {
		return nil, err
	}
	return connect.NewResponse(&playv1.ClosePuzzleResponse{Run: run}), nil
}

// ReleaseNextPuzzleHint implements playv1connect.PuzzleServiceHandler.
func (s *Service) ReleaseNextPuzzleHint(
	ctx context.Context,
	req *connect.Request[playv1.ReleaseNextPuzzleHintRequest],
) (*connect.Response[playv1.ReleaseNextPuzzleHintResponse], error) {
	m, err := requireMaster(ctx, req.Msg.GetCampaignId())
	if err != nil {
		return nil, err
	}
	run, err := s.changeRun(ctx, m, req.Msg.GetPuzzleId(), func(c *runTx) error {
		if err := notStale(c, req.Msg.GetExpectedRevision()); err != nil {
			return err
		}
		if err := needsVisibleRun(c); err != nil {
			return err
		}
		if int(c.run.ReleasedHints) >= len(c.def.hints) {
			return puzzleBlocked(playv1.PuzzleBlockedReason_PUZZLE_BLOCKED_REASON_NO_MORE_HINTS, "every hint is released already")
		}
		p := c.params()
		p.ReleasedHints++
		if err := c.save(ctx, p); err != nil {
			return err
		}
		c.tell = true
		return nil // a hint is not an event: the session's history keeps only shown, solved, reset and closed
	})
	if err != nil {
		return nil, err
	}
	return connect.NewResponse(&playv1.ReleaseNextPuzzleHintResponse{Run: run}), nil
}

// PlayPuzzleSequence implements playv1connect.PuzzleServiceHandler.
func (s *Service) PlayPuzzleSequence(
	ctx context.Context,
	req *connect.Request[playv1.PlayPuzzleSequenceRequest],
) (*connect.Response[playv1.PlayPuzzleSequenceResponse], error) {
	m, err := requireMaster(ctx, req.Msg.GetCampaignId())
	if err != nil {
		return nil, err
	}
	run, err := s.changeRun(ctx, m, req.Msg.GetPuzzleId(), func(c *runTx) error {
		if err := notStale(c, req.Msg.GetExpectedRevision()); err != nil {
			return err
		}
		return c.playSequence(ctx)
	})
	if err != nil {
		return nil, err
	}
	return connect.NewResponse(&playv1.PlayPuzzleSequenceResponse{Run: run}), nil
}

// sequencePlayable refuses a play of a puzzle that is not a sequence, not shown, solved or stopped.
func (c *runTx) sequencePlayable() error {
	if c.def.row.Kind != (sequenceKind{}).stored() {
		return puzzleBlocked(playv1.PuzzleBlockedReason_PUZZLE_BLOCKED_REASON_NOT_A_SEQUENCE, "the puzzle is not a sequence")
	}
	if err := needsVisibleRun(c); err != nil {
		return err
	}
	if c.run.SolvedAt != nil {
		return puzzleBlocked(playv1.PuzzleBlockedReason_PUZZLE_BLOCKED_REASON_SOLVED, "the puzzle is solved")
	}
	if stopOf(c.def, *c.run, c.now) != playv1.PuzzleStopReason_PUZZLE_STOP_REASON_UNSPECIFIED {
		return puzzleBlocked(playv1.PuzzleBlockedReason_PUZZLE_BLOCKED_REASON_STOPPED, "a limit stopped the puzzle")
	}
	return nil
}

// playSequence is the master's own act: it counts one more play and starts the
// steps from the clock. The app reads the run again at every step (the hint
// below, one for each); the attempt's progress and the counters stay as they were.
func (c *runTx) playSequence(ctx context.Context) error {
	if err := c.sequencePlayable(); err != nil {
		return err
	}
	p := c.params()
	p.Plays++
	p.PlayStartedAt = &c.now
	if p.RoundStartedAt == nil {
		p.RoundStartedAt = &c.now // the time limit of a sequence starts at its first play of the round
		c.roundBegan = true
	}
	if err := c.save(ctx, p); err != nil {
		return err
	}
	c.tell = true
	for i := 1; i <= len(c.def.solution.GetSequence().GetSteps()); i++ {
		c.later = append(c.later, time.Duration(i)*puzzle.SequenceStep)
	}
	return nil // a play is not an event: the history keeps only shown, solved, reset and closed
}

// GetMasterPuzzleRun implements playv1connect.PuzzleServiceHandler.
func (s *Service) GetMasterPuzzleRun(
	ctx context.Context,
	req *connect.Request[playv1.GetMasterPuzzleRunRequest],
) (*connect.Response[playv1.GetMasterPuzzleRunResponse], error) {
	m, err := requireMaster(ctx, req.Msg.GetCampaignId())
	if err != nil {
		return nil, err
	}
	id, err := parsePuzzleID(req.Msg.GetPuzzleId())
	if err != nil {
		return nil, err
	}
	session, err := s.puzzleSession(ctx, m.CampaignID)
	if err != nil {
		return nil, err
	}
	row, err := s.queries.GetPuzzle(ctx, playdb.GetPuzzleParams{CampaignID: m.CampaignID, ID: id})
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, errPuzzleNotFound()
	}
	if err != nil {
		return nil, s.dbError(ctx, "find a puzzle", err)
	}
	d, err := decodePuzzle(row)
	if err != nil {
		return nil, s.dbError(ctx, "read a puzzle", err)
	}
	var run *playdb.PuzzleRun
	r, err := s.queries.GetPuzzleRun(ctx, playdb.GetPuzzleRunParams{GameSessionID: session.ID, PuzzleID: id})
	switch {
	case err == nil:
		run = &r
	case !errors.Is(err, pgx.ErrNoRows):
		return nil, s.dbError(ctx, "find the puzzle's run", err)
	}
	out, err := s.masterView(ctx, nil, m.CampaignID, d, run)
	if err != nil {
		return nil, err
	}
	return connect.NewResponse(&playv1.GetMasterPuzzleRunResponse{Run: out}), nil
}

// ListSessionPuzzles implements playv1connect.PuzzleServiceHandler.
func (s *Service) ListSessionPuzzles(
	ctx context.Context,
	req *connect.Request[playv1.ListSessionPuzzlesRequest],
) (*connect.Response[playv1.ListSessionPuzzlesResponse], error) {
	m, err := requireMaster(ctx, req.Msg.GetCampaignId())
	if err != nil {
		return nil, err
	}
	session, err := s.puzzleSession(ctx, m.CampaignID)
	if err != nil {
		return nil, err
	}
	rows, err := s.queries.ListPuzzles(ctx, playdb.ListPuzzlesParams{CampaignID: m.CampaignID})
	if err != nil {
		return nil, s.dbError(ctx, "list puzzles", err)
	}
	runs, err := s.queries.ListPuzzleRuns(ctx, session.ID)
	if err != nil {
		return nil, s.dbError(ctx, "list the puzzles' runs", err)
	}
	byPuzzle := make(map[string]*playdb.PuzzleRun, len(runs))
	for i := range runs {
		byPuzzle[runs[i].PuzzleID] = &runs[i]
	}
	names, err := s.namesOfRuns(ctx, nil, m.CampaignID, runs...)
	if err != nil {
		return nil, s.dbError(ctx, "list the puzzles' runs", err)
	}
	shown, err := s.shownSet(ctx, m.CampaignID)
	if err != nil {
		return nil, s.dbError(ctx, "list the puzzles' runs", err)
	}
	res := &playv1.ListSessionPuzzlesResponse{}
	for _, row := range rows {
		out, err := s.sessionPuzzle(ctx, m.CampaignID, row, byPuzzle[row.ID], names, shown[row.ID])
		if err != nil {
			return nil, s.dbError(ctx, "read a puzzle", err)
		}
		res.Puzzles = append(res.Puzzles, out)
	}
	return connect.NewResponse(res), nil
}

// sessionPuzzle is the master's message for a puzzle row and its run in the session (nil: none).
func (s *Service) sessionPuzzle(ctx context.Context, campaignID string, row playdb.Puzzle, run *playdb.PuzzleRun, names runNames, shown bool) (*playv1.MasterPuzzleRun, error) {
	d, err := decodePuzzle(row)
	if err != nil {
		return nil, err
	}
	var ex masterExtra
	if run != nil {
		if ex, err = s.masterExtras(ctx, nil, campaignID, d, *run); err != nil {
			return nil, err
		}
	}
	out, err := masterRunProto(d, run, names, shown, ex)
	if err != nil {
		return nil, err
	}
	if err := s.flagParts(ctx, campaignID, out.GetPuzzle()); err != nil {
		return nil, err
	}
	return out, nil
}
