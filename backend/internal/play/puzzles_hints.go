package play

import (
	"context"
	"errors"
	"fmt"

	"connectrpc.com/connect"
	"github.com/jackc/pgx/v5"

	playv1 "github.com/PuraFome/meuRPG/backend/gen/meurpg/play/v1"
	"github.com/PuraFome/meuRPG/backend/internal/authz"
	"github.com/PuraFome/meuRPG/backend/internal/platform/db"
	"github.com/PuraFome/meuRPG/backend/internal/play/playdb"
)

// A hint won by a skill check (MR-038, RN-27, RN-18): a player rolls their character's
// skill against the DC the master set; a pass gives that player, and only them, the next
// hint they do not read yet. The DC never goes to a player: the answer carries the roll
// and whether it passed. A player tries once for each hint (the unique key of
// puzzle_hint_tries), so a failed try cannot be repeated until the master releases that
// hint (another player passing wins that player their own hint, not this one back); and each try is one revision of the run, so the app
// applies the read that follows.

// hintTryResult is what a try's transaction leaves for after the commit.
type hintTryResult struct {
	d        puzzleDef
	run      playdb.PuzzleRun
	try      playdb.PuzzleHintTry
	replayed bool
}

// TryPuzzleHint implements playv1connect.PuzzleServiceHandler.
func (s *Service) TryPuzzleHint(
	ctx context.Context,
	req *connect.Request[playv1.TryPuzzleHintRequest],
) (*connect.Response[playv1.TryPuzzleHintResponse], error) {
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
	var in rollInput
	switch roll := req.Msg.GetRoll().(type) {
	case *playv1.TryPuzzleHintRequest_RollInApp:
		if !roll.RollInApp {
			return nil, connect.NewError(connect.CodeInvalidArgument, errors.New("roll_in_app must be true"))
		}
		in.inApp = true
	case *playv1.TryPuzzleHintRequest_D20Face:
		in.typed = int(roll.D20Face)
		if in.typed < 1 || in.typed > 20 {
			return nil, connect.NewError(connect.CodeInvalidArgument, errors.New("d20_face must be 1 to 20"))
		}
	default:
		return nil, connect.NewError(connect.CodeInvalidArgument, errors.New("set roll_in_app or d20_face"))
	}

	res := &hintTryResult{}
	var names runNames
	err = db.InTx(ctx, s.pool, func(tx pgx.Tx) error {
		*res = hintTryResult{}
		var err error
		names, err = s.applyHintTry(ctx, tx, m, id, key, in, res)
		return err
	})
	if err != nil {
		return nil, s.dbError(ctx, "try for a puzzle hint", err)
	}
	if !res.replayed {
		s.publishPuzzleChanged(m.CampaignID, id) // the master reads the try; the revision moved
	}
	view, err := s.viewerOf(ctx, nil, m, res.d, res.run)
	if err != nil {
		return nil, s.dbError(ctx, "read the puzzle after a try", err)
	}
	out, err := playerRunProto(res.d, res.run, names, view)
	if err != nil {
		return nil, s.dbError(ctx, "read the puzzle after a try", err)
	}
	return connect.NewResponse(&playv1.TryPuzzleHintResponse{
		Run:      out,
		Roll:     diceRoll(1, 20, []int32{res.try.D20}, res.try.Modifier, res.try.Total, res.try.Physical),
		Passed:   res.try.Passed,
		Replayed: res.replayed,
	}), nil
}

// applyHintTry is the try's transaction. The session's row is locked first and then
// the run's, as every change to a run does, so a try, a move and the master's release
// of the same hint take turns.
func (s *Service) applyHintTry(ctx context.Context, tx pgx.Tx, m authz.Membership, puzzleID, key string, in rollInput, res *hintTryResult) (runNames, error) {
	q := s.queriesIn(tx)
	session, err := lockOpenSession(ctx, q, m.CampaignID)
	if err != nil {
		return nil, err
	}
	d, _, err := s.shownRun(ctx, q, session, m.CampaignID, puzzleID)
	if err != nil {
		return nil, err
	}
	run, err := q.GetPuzzleRunForUpdate(ctx, playdb.GetPuzzleRunForUpdateParams{GameSessionID: session.ID, PuzzleID: puzzleID})
	if err != nil {
		return nil, fmt.Errorf("lock the puzzle's run: %w", err)
	}
	res.d, res.run = d, run

	// A retry of a try already made answers as the first call did, and rolls nothing.
	done, err := q.GetPuzzleHintTryByKey(ctx, playdb.GetPuzzleHintTryByKeyParams{RunID: run.ID, IdempotencyKey: key})
	switch {
	case err == nil:
		// The key stands for that try, of that player, rolled that way: in the app, or with
		// that typed die.
		sameRoll := done.Physical != in.inApp && (in.inApp || int(done.D20) == in.typed)
		if done.UserID == nil || *done.UserID != m.UserID || !sameRoll {
			return nil, errKeyReused()
		}
		res.try, res.replayed = done, true
		return s.namesOfRuns(ctx, tx, m.CampaignID, run)
	case !errors.Is(err, pgx.ErrNoRows):
		return nil, fmt.Errorf("find the try of this idempotency key: %w", err)
	}
	// A key spent on a move of the run is spent: a try is another change.
	if _, err := q.GetPuzzleMoveByKey(ctx, playdb.GetPuzzleMoveByKeyParams{RunID: run.ID, IdempotencyKey: key}); err == nil {
		return nil, errKeyReused()
	} else if !errors.Is(err, pgx.ErrNoRows) {
		return nil, fmt.Errorf("find the move of this idempotency key: %w", err)
	}

	now := s.now()
	switch {
	case run.SolvedAt != nil:
		return nil, puzzleBlocked(playv1.PuzzleBlockedReason_PUZZLE_BLOCKED_REASON_SOLVED, "the puzzle is solved")
	case stopOf(d, run, now) != playv1.PuzzleStopReason_PUZZLE_STOP_REASON_UNSPECIFIED:
		return nil, puzzleBlocked(playv1.PuzzleBlockedReason_PUZZLE_BLOCKED_REASON_STOPPED, "a limit stopped the puzzle")
	case d.hintCheck == nil:
		return nil, puzzleBlocked(playv1.PuzzleBlockedReason_PUZZLE_BLOCKED_REASON_NO_HINT_CHECK, "this puzzle has no skill check for hints")
	}
	who, has, err := s.myCharacter(ctx, tx, m)
	if err != nil {
		return nil, err
	}
	if !has {
		return nil, puzzleBlocked(playv1.PuzzleBlockedReason_PUZZLE_BLOCKED_REASON_NO_CHARACTER, "you have no living character to roll for")
	}
	won, err := q.GetPuzzleHintCursor(ctx, playdb.GetPuzzleHintCursorParams{RunID: run.ID, UserID: &m.UserID})
	if err != nil {
		return nil, fmt.Errorf("read the hints the player won: %w", err)
	}
	// The hint this player would read next: after the master's released ones and the
	// ones they won.
	next := max(int(run.ReleasedHints), int(won))
	if next >= len(d.hints) {
		return nil, puzzleBlocked(playv1.PuzzleBlockedReason_PUZZLE_BLOCKED_REASON_NO_MORE_HINTS, "there is no hint left for you")
	}
	tried, err := q.HasTriedPuzzleHint(ctx, playdb.HasTriedPuzzleHintParams{RunID: run.ID, UserID: &m.UserID, HintIndex: clamp32(next, 0, maxHints)})
	if err != nil {
		return nil, fmt.Errorf("read whether the player tried: %w", err)
	}
	if tried {
		return nil, puzzleBlocked(playv1.PuzzleBlockedReason_PUZZLE_BLOCKED_REASON_HINT_ALREADY_TRIED, "you tried for this hint already")
	}
	force, err := s.dice.ForcedDice(ctx, tx, m.CampaignID, m.UserID)
	if err != nil {
		return nil, err
	}
	if force.refuses(in.inApp) {
		return nil, puzzleBlocked(playv1.PuzzleBlockedReason_PUZZLE_BLOCKED_REASON_WRONG_DICE_MODE, "this is not how the campaign has you roll your dice")
	}
	options, err := s.roster.SceneOptions(ctx, tx, m.CampaignID, who.ID, []string{d.hintCheck.GetSkillKey()})
	if err != nil {
		return nil, err
	}
	if len(options) != 1 || !options[0].Known {
		return nil, puzzleBlocked(playv1.PuzzleBlockedReason_PUZZLE_BLOCKED_REASON_NO_SKILL, "your character has no numbers for this skill")
	}
	face, roll, err := s.d20(in, options[0].Bonus)
	if err != nil {
		return nil, err
	}
	passed := roll.Total >= int(d.hintCheck.GetDc())
	var granted *int32
	if passed {
		g := clamp32(next+1, 1, maxHints)
		granted = &g
	}
	try, err := q.InsertPuzzleHintTry(ctx, playdb.InsertPuzzleHintTryParams{
		RunID: run.ID, UserID: &m.UserID, CharacterID: &who.ID, IdempotencyKey: key, HintIndex: clamp32(next, 0, maxHints),
		Passed: passed, GrantedCount: granted, D20: clamp32(face, 1, 20), Modifier: clamp32(options[0].Bonus, -1000, 1000),
		Total: clamp32(roll.Total, -1000, 1000), Physical: roll.Physical, CreatedAt: now,
	})
	if err != nil {
		return nil, fmt.Errorf("record the try: %w", err)
	}
	// The run's revision moves with the try, so the app applies the read that follows.
	p := paramsOf(&run)
	p.Revision, p.UpdatedAt = run.Revision+1, now
	saved, err := q.SavePuzzleRun(ctx, p)
	if err != nil {
		return nil, fmt.Errorf("save the puzzle's run: %w", err)
	}
	res.try, res.run = try, saved
	return s.namesOfRuns(ctx, tx, m.CampaignID, saved)
}
