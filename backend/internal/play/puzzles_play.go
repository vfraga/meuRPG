package play

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"slices"
	"time"

	"connectrpc.com/connect"
	"github.com/jackc/pgx/v5"
	"google.golang.org/protobuf/proto"

	playv1 "github.com/PuraFome/meuRPG/backend/gen/meurpg/play/v1"
	"github.com/PuraFome/meuRPG/backend/internal/authz"
	maplink "github.com/PuraFome/meuRPG/backend/internal/maps/link"
	"github.com/PuraFome/meuRPG/backend/internal/platform/db"
	"github.com/PuraFome/meuRPG/backend/internal/platform/logging"
	"github.com/PuraFome/meuRPG/backend/internal/play/link"
	"github.com/PuraFome/meuRPG/backend/internal/play/playdb"
)

// What the players do with a puzzle: see which are shown, read one, and move.
// Everything a player gets comes out of playerRunProto (puzzles_view.go), which
// holds only what RN-10 and RN-27 allow. A puzzle that is not shown, was closed,
// is archived or does not exist is the same answer: `not_found`.

// shownRun finds the run of a puzzle that the session shows, with its puzzle, or
// answers `not_found`.
func (s *Service) shownRun(ctx context.Context, q *playdb.Queries, session playdb.GameSession, campaignID, puzzleID string) (puzzleDef, playdb.PuzzleRun, error) {
	row, err := q.GetPuzzle(ctx, playdb.GetPuzzleParams{CampaignID: campaignID, ID: puzzleID})
	if errors.Is(err, pgx.ErrNoRows) {
		return puzzleDef{}, playdb.PuzzleRun{}, errPuzzleNotFound()
	}
	if err != nil {
		return puzzleDef{}, playdb.PuzzleRun{}, fmt.Errorf("find puzzle: %w", err)
	}
	run, err := q.GetPuzzleRun(ctx, playdb.GetPuzzleRunParams{GameSessionID: session.ID, PuzzleID: puzzleID})
	if errors.Is(err, pgx.ErrNoRows) {
		return puzzleDef{}, playdb.PuzzleRun{}, errPuzzleNotFound()
	}
	if err != nil {
		return puzzleDef{}, playdb.PuzzleRun{}, fmt.Errorf("find the puzzle's run: %w", err)
	}
	if row.ArchivedAt != nil || !visible(&run) {
		return puzzleDef{}, playdb.PuzzleRun{}, errPuzzleNotFound()
	}
	d, err := decodePuzzle(row)
	return d, run, err
}

// ListShownPuzzles implements playv1connect.PuzzleServiceHandler.
func (s *Service) ListShownPuzzles(
	ctx context.Context,
	req *connect.Request[playv1.ListShownPuzzlesRequest],
) (*connect.Response[playv1.ListShownPuzzlesResponse], error) {
	m, err := authz.RequireCampaignMember(ctx, req.Msg.GetCampaignId())
	if err != nil {
		return nil, err
	}
	session, err := s.puzzleSession(ctx, m.CampaignID)
	if err != nil {
		return nil, err
	}
	rows, err := s.queries.ListShownPuzzleRuns(ctx, session.ID)
	if err != nil {
		return nil, s.dbError(ctx, "list the shown puzzles", err)
	}
	res := &playv1.ListShownPuzzlesResponse{}
	for _, r := range rows {
		kind := kindByStored(r.PuzzleKind)
		if kind == nil {
			continue // a kind this server does not know: nothing to play
		}
		stopped := false
		if len(r.PuzzleOnWrong) > 0 {
			on := &playv1.PuzzleOnWrong{}
			if err := fromJSON(r.PuzzleOnWrong, on); err != nil {
				return nil, s.dbError(ctx, "list the shown puzzles", err)
			}
			stopped = stopOf(puzzleDef{onWrong: on}, playdb.PuzzleRun{SolvedAt: r.SolvedAt, MovesMade: r.MovesMade, RoundStartSeq: r.RoundStartSeq, RoundStartedAt: r.RoundStartedAt}, s.now()) != playv1.PuzzleStopReason_PUZZLE_STOP_REASON_UNSPECIFIED
		}
		res.Puzzles = append(res.Puzzles, &playv1.PuzzleSummary{PuzzleId: r.PuzzleID, Name: r.PuzzleName, Kind: kind.id(), Solved: r.SolvedAt != nil, Stopped: stopped})
	}
	return connect.NewResponse(res), nil
}

// kindByStored finds a kind by its stored name.
func kindByStored(stored string) puzzleKind {
	for _, k := range allKinds {
		if k.stored() == stored {
			return k
		}
	}
	return nil
}

// GetPuzzleRun implements playv1connect.PuzzleServiceHandler.
func (s *Service) GetPuzzleRun(
	ctx context.Context,
	req *connect.Request[playv1.GetPuzzleRunRequest],
) (*connect.Response[playv1.GetPuzzleRunResponse], error) {
	m, err := authz.RequireCampaignMember(ctx, req.Msg.GetCampaignId())
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
	d, run, err := s.shownRun(ctx, s.queries, session, m.CampaignID, id)
	if err != nil {
		return nil, s.dbError(ctx, "read a puzzle", err)
	}
	names, err := s.namesOfRuns(ctx, nil, m.CampaignID, run)
	if err != nil {
		return nil, s.dbError(ctx, "read a puzzle", err)
	}
	view, err := s.viewerOf(ctx, nil, m, d, run)
	if err != nil {
		return nil, s.dbError(ctx, "read a puzzle", err)
	}
	out, err := playerRunProto(d, run, names, view)
	if err != nil {
		return nil, s.dbError(ctx, "read a puzzle", err)
	}
	return connect.NewResponse(&playv1.GetPuzzleRunResponse{Run: out}), nil
}

// moveResult is what a move's transaction leaves for after the commit.
type moveResult struct {
	d        puzzleDef
	run      playdb.PuzzleRun
	who      link.Character
	replayed bool
	solved   bool // this move solved it
	wrong    bool // this move was a wrong one (it counts toward the limits)
	after    []func(context.Context)
	doorMap  string           // a map whose door the solve opened: tell its watchers
	fired    *trapFireEvent   // the trap a wrong move fired: tell the watchers
	firedEnc playdb.Encounter // the combat it fired in, zero outside a combat
}

// logPuzzleMove writes the DEBUG events of a move that was committed: the move
// itself with its outcome (`solved`, `wrong` or `ok`), and `puzzle.solved` when
// it was the one that solved the puzzle.
func logPuzzleMove(ctx context.Context, l *slog.Logger, puzzleID string, res *moveResult) {
	outcome := "ok"
	switch {
	case res.solved:
		outcome = "solved"
	case res.wrong:
		outcome = "wrong"
	}
	ids := []slog.Attr{slog.String("puzzle_id", puzzleID), slog.String("run_id", res.run.ID), slog.String("character_id", res.who.ID)}
	logging.Event(ctx, l, "puzzle.move_made", append(ids, slog.String("outcome", outcome))...)
	switch {
	case res.solved:
		logging.Event(ctx, l, "puzzle.solved", ids...)
	case res.wrong:
		logging.Event(ctx, l, "puzzle.failed", ids...)
	}
}

// MakePuzzleMove implements playv1connect.PuzzleServiceHandler.
func (s *Service) MakePuzzleMove(
	ctx context.Context,
	req *connect.Request[playv1.MakePuzzleMoveRequest],
) (*connect.Response[playv1.MakePuzzleMoveResponse], error) {
	m, err := authz.RequireCampaignRole(ctx, req.Msg.GetCampaignId(), authz.RolePlayer)
	if err != nil {
		return nil, err
	}
	key, err := parseKey(req.Msg.GetIdempotencyKey())
	if err != nil {
		return nil, err
	}
	id, err := parsePuzzleID(req.Msg.GetPuzzleId())
	if err != nil {
		return nil, err
	}
	if req.Msg.GetMove().GetKind() == nil {
		return nil, puzzleInvalid(playv1.PuzzleInvalidReason_PUZZLE_INVALID_REASON_KIND, "move", "move must have one kind")
	}

	var res *moveResult
	var names runNames
	err = db.InTx(ctx, s.pool, func(tx pgx.Tx) error {
		res = &moveResult{}
		var err error
		if names, err = s.applyMove(ctx, tx, m, id, key, req.Msg.GetMove(), res); err != nil {
			return err
		}
		return nil
	})
	if err != nil {
		return nil, s.dbError(ctx, "make a puzzle move", err)
	}
	if !res.replayed {
		logPuzzleMove(ctx, s.logger, id, res)
		s.publishPuzzleChanged(m.CampaignID, id)
		for _, f := range res.after {
			f(ctx)
		}
		if res.doorMap != "" && s.puzzles.maps != nil {
			s.puzzles.maps.DoorsChanged(ctx, m.CampaignID, res.doorMap)
		}
		if res.fired != nil {
			s.afterFiring(ctx, m.CampaignID, res.fired, res.firedEnc)
		}
	}
	view, err := s.viewerOf(ctx, nil, m, res.d, res.run)
	if err != nil {
		return nil, s.dbError(ctx, "read the puzzle after a move", err)
	}
	out, err := playerRunProto(res.d, res.run, names, view)
	if err != nil {
		return nil, s.dbError(ctx, "read the puzzle after a move", err)
	}
	return connect.NewResponse(&playv1.MakePuzzleMoveResponse{Run: out, Replayed: res.replayed, SolvedByThisMove: res.solved, Wrong: res.wrong}), nil
}

// applyMove is the move's transaction. The session's row is locked first (so the
// solve's event gets its number in order, and a master's reset waits), then the
// run's: two players' moves take turns, each applied to the state the other left,
// which is why both count. A move is relative, so the order they take does not
// change where the pieces end.
func (s *Service) applyMove(ctx context.Context, tx pgx.Tx, m authz.Membership, puzzleID, key string, mv *playv1.PuzzleMove, res *moveResult) (runNames, error) {
	q := s.queriesIn(tx)
	session, err := lockOpenSession(ctx, q, m.CampaignID)
	if err != nil {
		return nil, err
	}
	d, _, err := s.shownRun(ctx, q, session, m.CampaignID, puzzleID)
	if err != nil {
		return nil, err
	}
	// The run is read again with its lock: the read above only found it.
	run, err := q.GetPuzzleRunForUpdate(ctx, playdb.GetPuzzleRunForUpdateParams{GameSessionID: session.ID, PuzzleID: puzzleID})
	if err != nil {
		return nil, fmt.Errorf("lock the puzzle's run: %w", err)
	}
	res.d, res.run = d, run

	// A retry of a move already made answers as the first call did, and changes nothing.
	done, err := q.GetPuzzleMoveByKey(ctx, playdb.GetPuzzleMoveByKeyParams{RunID: run.ID, IdempotencyKey: key})
	switch {
	case err == nil:
		// The key stands for that move, of that player: another move, or another player, is
		// not a retry.
		made := &playv1.PuzzleMove{}
		if done.UserID == nil || *done.UserID != m.UserID || fromJSON(done.Move, made) != nil || !proto.Equal(made, mv) {
			return nil, errKeyReused()
		}
		res.replayed, res.solved, res.wrong = true, done.Solved, done.Wrong
		return s.namesOfRuns(ctx, tx, m.CampaignID, run)
	case !errors.Is(err, pgx.ErrNoRows):
		return nil, fmt.Errorf("find the move of this idempotency key: %w", err)
	}
	// A key spent on a hint try of the run is spent: a move is another change.
	if _, err := q.GetPuzzleHintTryByKey(ctx, playdb.GetPuzzleHintTryByKeyParams{RunID: run.ID, IdempotencyKey: key}); err == nil {
		return nil, errKeyReused()
	} else if !errors.Is(err, pgx.ErrNoRows) {
		return nil, fmt.Errorf("find the try of this idempotency key: %w", err)
	}

	if run.SolvedAt != nil {
		return nil, puzzleBlocked(playv1.PuzzleBlockedReason_PUZZLE_BLOCKED_REASON_SOLVED, "the puzzle is solved: nothing moves any more")
	}
	now := s.now()
	if stopOf(d, run, now) != playv1.PuzzleStopReason_PUZZLE_STOP_REASON_UNSPECIFIED {
		return nil, puzzleBlocked(playv1.PuzzleBlockedReason_PUZZLE_BLOCKED_REASON_STOPPED, "a limit stopped the puzzle: the master restarts or closes it")
	}
	who, has, err := s.myCharacter(ctx, tx, m)
	if err != nil {
		return nil, err
	}
	if !has {
		return nil, puzzleBlocked(playv1.PuzzleBlockedReason_PUZZLE_BLOCKED_REASON_NO_CHARACTER, "you have no living character to play with")
	}
	res.who = who
	if g, ok := d.kind.(gatedKind); ok {
		if err := g.gate(d, run, now); err != nil {
			return nil, err
		}
	}
	// The player's attempts: every wrong move of theirs in this round spent one. The
	// run is locked, so the count is exact when moves come at once.
	if limit := int(d.onWrong.GetAttemptsPerPlayer()); limit > 0 {
		rows, err := q.CountWrongPuzzleMoves(ctx, playdb.CountWrongPuzzleMovesParams{RunID: run.ID, Seq: run.RoundStartSeq})
		if err != nil {
			return nil, fmt.Errorf("count the player's wrong moves: %w", err)
		}
		for _, r := range rows {
			if r.UserID != nil && *r.UserID == m.UserID && int(r.Wrong) >= limit {
				return nil, puzzleBlocked(playv1.PuzzleBlockedReason_PUZZLE_BLOCKED_REASON_NO_ATTEMPTS_LEFT, "you have no attempts left in this round")
			}
		}
	}

	state := &playv1.PuzzleState{}
	if err := fromJSON(run.State, state); err != nil {
		return nil, err
	}
	out, err := d.kind.move(d, state, mv)
	if err != nil {
		return nil, err
	}
	solved := out.solved
	if !solved {
		if solved, err = d.kind.solved(d, out.state); err != nil {
			return nil, err
		}
	}

	last := &playv1.PuzzleLastMove{Move: mv, Changed: out.changed, Wrong: out.wrong, Step: out.step}
	if out.wrong {
		// "Ao errar": the trap fires in this transaction, with the move; the attempt is
		// the move itself, marked wrong below, and so is its count toward the limits.
		if last.TrapName, err = s.fireWrongTrap(ctx, tx, q, session, m, who, d, now, res); err != nil {
			return nil, err
		}
	}
	p := playdb.SavePuzzleRunParams{
		ID: run.ID, Seed: run.Seed, Start: run.Start, State: toJSON(out.state), ReleasedHints: run.ReleasedHints, ShownAt: run.ShownAt,
		ClosedAt: run.ClosedAt, SolvedAt: run.SolvedAt, SolvedByCharacterID: run.SolvedByCharacterID, SolveOutcome: run.SolveOutcome,
		LastMoverCharacterID: &who.ID, LastMove: toJSON(last), LastMovedAt: &now,
		MovesMade: run.MovesMade + 1, Revision: run.Revision + 1, UpdatedAt: now,
		Plays: run.Plays, PlayStartedAt: run.PlayStartedAt, RoundStartSeq: run.RoundStartSeq, RoundStartedAt: run.RoundStartedAt,
	}
	// A solved run is never moved, so the message is empty here: it is written only by the solve below.
	if solved {
		// "Ao resolver" runs here, in the transaction of the winning move: the door,
		// the point or the clue changes if and only if the puzzle is solved.
		outcome, line, err := s.solvePuzzle(ctx, tx, m, d, who, now, res)
		if err != nil {
			return nil, err
		}
		name := outcomeName(outcome)
		p.SolvedAt, p.SolvedByCharacterID, p.SolveOutcome = &now, &who.ID, &name
		if line != "" {
			p.SolveMessage = &line
		}
	}
	saved, err := q.SavePuzzleRun(ctx, p)
	if err != nil {
		return nil, fmt.Errorf("save the puzzle's run: %w", err)
	}
	if _, err := q.InsertPuzzleMove(ctx, playdb.InsertPuzzleMoveParams{
		RunID: run.ID, Seq: saved.MovesMade, UserID: &m.UserID, CharacterID: &who.ID, IdempotencyKey: key,
		Move: toJSON(mv), Revision: saved.Revision, Solved: solved, Wrong: out.wrong, CreatedAt: now,
	}); err != nil {
		return nil, fmt.Errorf("record the move: %w", err)
	}
	if solved {
		ev := puzzleEvent{PuzzleID: puzzleID, RunID: run.ID, Outcome: deref(saved.SolveOutcome)}
		if err := puzzleSessionEvent(ctx, q, session, eventPuzzleSolved, m.UserID, &who.ID, ev, now); err != nil {
			return nil, err
		}
	}
	res.run, res.solved, res.wrong = saved, solved, out.wrong
	return runNames{who.ID: who.Name}, nil
}

// fireWrongTrap fires the trap point of "Ao errar" for a wrong move, inside the move's
// transaction (nothing reads through the pool here), the way the master firing a trap by
// hand does, with nobody caught: the master decides what the trap does (MR-035).
//   - While a combat without a map runs (RN-25) nothing fires: that combat has no traps.
//     The wrong move still counts.
//   - While a combat runs on the trap's map, the firing is that combat's, as FireTrap's in
//     a combat: the event carries the combat, the encounter is touched and the log hears
//     of it, so the master may extend the firing to the creatures it catches.
//   - Otherwise it is the firing outside a combat.
//
// The point fires once: one that already fired, was disarmed or is gone fires nothing, and
// the move goes on. It returns the name the master gave the point, which the players who
// see it read once it fired, and nothing for a trap that did not fire. res keeps the firing,
// so the watchers hear of it after the commit.
func (s *Service) fireWrongTrap(ctx context.Context, tx pgx.Tx, q *playdb.Queries, session playdb.GameSession, m authz.Membership, who link.Character, d puzzleDef, now time.Time, res *moveResult) (string, error) {
	target := d.onWrong.GetTrap()
	if target == nil || s.traps == nil {
		return "", nil
	}
	if theatre, err := theatreRunning(ctx, q, session.ID); err != nil || theatre {
		return "", err
	}
	traps, err := s.traps.Traps(ctx, tx, m.CampaignID, target.GetMapId())
	if connect.CodeOf(err) == connect.CodeNotFound {
		return "", nil // the map is gone
	}
	if err != nil {
		return "", err
	}
	i := slices.IndexFunc(traps, func(t maplink.Trap) bool { return t.PointID == target.GetPointId() })
	if i < 0 || traps[i].State == "triggered" || traps[i].State == "disarmed" {
		return "", nil
	}
	trap := traps[i]
	// openTx reads the table's rules in this transaction, so the trap's critical follows them (RN-24).
	c, err := s.openTx(ctx, combatTx{tx: tx, q: q, session: session, now: now, characterID: &who.ID, kind: eventTrapTriggered, actorUserID: m.UserID, svc: s})
	if err != nil {
		return "", err
	}
	// A combat running on the trap's map (read in this transaction, never through the pool).
	enc, err := q.GetLatestEncounter(ctx, session.ID)
	if err != nil && !errors.Is(err, pgx.ErrNoRows) {
		return "", fmt.Errorf("find the session's combat: %w", err)
	}
	inCombat := err == nil && enc.Status != statusEnded && enc.MapID != nil && *enc.MapID == trap.MapID
	var fired *trapFireEvent
	if inCombat {
		c.enc = enc
		fired, err = s.fireInCombat(ctx, c, trap, nil, true, "")
	} else {
		fired, err = s.fireOutside(ctx, c, trap, nil, nil, true, "")
	}
	if errors.Is(err, maplink.ErrTrapNotArmed) {
		return "", nil
	}
	if err != nil {
		return "", err
	}
	ev := actionEvent{Trap: fired}
	if inCombat {
		if c.enc, err = q.TouchEncounter(ctx, c.enc.ID); err != nil {
			return "", fmt.Errorf("touch the encounter: %w", err)
		}
		ev.Round = c.enc.Round
		res.firedEnc = c.enc
	}
	if err := insertEvent(ctx, c, eventTrapTriggered, &m.UserID, nil, ev); err != nil {
		return "", err
	}
	res.fired = fired
	return trap.Name, nil
}

// solveLine is what the players read once the puzzle is solved: the master's own
// text, or a generic line for what the action did, and nothing when it did nothing.
func solveLine(master string, outcome playv1.PuzzleSolveOutcome, pointName string) string {
	if master != "" {
		return master
	}
	switch outcome {
	case playv1.PuzzleSolveOutcome_PUZZLE_SOLVE_OUTCOME_DOOR_OPENED:
		return "Uma porta se abriu."
	case playv1.PuzzleSolveOutcome_PUZZLE_SOLVE_OUTCOME_POINT_REVEALED:
		if pointName == "" { // the players do not see the point: its name is not theirs to read
			return "Algo apareceu no mapa."
		}
		return pointName + " apareceu no mapa."
	case playv1.PuzzleSolveOutcome_PUZZLE_SOLVE_OUTCOME_CLUE_REVEALED:
		return "Você ganhou uma pista." // the clue itself goes to the solver only
	}
	return ""
}

// solvePuzzle runs the puzzle's "Ao resolver" inside the winning move's
// transaction and says what it did, and the line the players read. A target that is
// gone (deleted since the puzzle was made) does not undo the solve: the master reads
// TARGET_GONE or DOOR_NOT_CLOSED. The notices for the watchers wait in res until the
// commit.
func (s *Service) solvePuzzle(ctx context.Context, tx pgx.Tx, m authz.Membership, d puzzleDef, who link.Character, now time.Time, res *moveResult) (playv1.PuzzleSolveOutcome, string, error) {
	outcome, pointName, err := s.runSolveAction(ctx, tx, m, d, who, now, res)
	if err != nil {
		return 0, "", err
	}
	return outcome, solveLine(d.onSolve.GetMessage(), outcome, pointName), nil
}

func (s *Service) runSolveAction(ctx context.Context, tx pgx.Tx, m authz.Membership, d puzzleDef, who link.Character, now time.Time, res *moveResult) (playv1.PuzzleSolveOutcome, string, error) {
	gone := playv1.PuzzleSolveOutcome_PUZZLE_SOLVE_OUTCOME_TARGET_GONE
	on, maps := d.onSolve, s.puzzles.maps
	switch on.GetAction() {
	case playv1.PuzzleSolveAction_PUZZLE_SOLVE_ACTION_OPEN_DOOR:
		t := on.GetDoor()
		if maps == nil {
			return gone, "", nil
		}
		opened, err := maps.PuzzleOpenDoor(ctx, tx, m.CampaignID, t.GetMapId(), int(t.GetCol()), int(t.GetRow()))
		switch {
		case connect.CodeOf(err) == connect.CodeNotFound:
			return gone, "", nil
		case err != nil:
			return 0, "", err
		case !opened:
			return playv1.PuzzleSolveOutcome_PUZZLE_SOLVE_OUTCOME_DOOR_NOT_CLOSED, "", nil
		}
		res.doorMap = t.GetMapId()
		return playv1.PuzzleSolveOutcome_PUZZLE_SOLVE_OUTCOME_DOOR_OPENED, "", nil
	case playv1.PuzzleSolveAction_PUZZLE_SOLVE_ACTION_REVEAL_POINT:
		t := on.GetPoint()
		if maps == nil {
			return gone, "", nil
		}
		onScreen, err := s.queries.WithTx(tx).GetOnScreen(ctx, m.CampaignID)
		if err != nil && !errors.Is(err, pgx.ErrNoRows) {
			return 0, "", fmt.Errorf("read what the session shows: %w", err)
		}
		changed, name, after, err := maps.PuzzleRevealPoint(ctx, tx, m.CampaignID, t.GetMapId(), t.GetPointId(), deref(onScreen.CurrentMapID))
		switch {
		case connect.CodeOf(err) == connect.CodeNotFound:
			return gone, "", nil
		case err != nil:
			return 0, "", err
		case !changed:
			return playv1.PuzzleSolveOutcome_PUZZLE_SOLVE_OUTCOME_POINT_ALREADY_REVEALED, "", nil
		}
		res.after = append(res.after, after)
		return playv1.PuzzleSolveOutcome_PUZZLE_SOLVE_OUTCOME_POINT_REVEALED, name, nil
	case playv1.PuzzleSolveAction_PUZZLE_SOLVE_ACTION_REVEAL_CLUE:
		t := on.GetClue()
		if maps == nil || who.PlayerUserID == "" {
			return gone, "", nil
		}
		gave, after, err := maps.PuzzleRevealClue(ctx, tx, m.CampaignID, t.GetClueId(), who.ID, who.PlayerUserID, now)
		switch {
		case connect.CodeOf(err) == connect.CodeNotFound:
			return gone, "", nil
		case err != nil:
			return 0, "", err
		case !gave:
			return playv1.PuzzleSolveOutcome_PUZZLE_SOLVE_OUTCOME_CLUE_ALREADY_HAD, "", nil
		}
		res.after = append(res.after, after)
		return playv1.PuzzleSolveOutcome_PUZZLE_SOLVE_OUTCOME_CLUE_REVEALED, "", nil
	}
	return playv1.PuzzleSolveOutcome_PUZZLE_SOLVE_OUTCOME_NOTIFIED, "", nil
}
