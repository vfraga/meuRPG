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
	"github.com/PuraFome/meuRPG/backend/internal/play/link"
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
	sources  []*playv1.AdvantageSource
	// offer is the question about a Bardic Inspiration die the try waits on: the d20 was rolled and
	// kept, and nothing else of the result exists. bonus is the die the answer added. lucky is the
	// halfling's Lucky question the try waits on instead (it comes first).
	offer *playv1.OutsideInspirationOffer
	bonus *playv1.BonusDie
	lucky *playv1.LuckyOffer
	held  *playv1.HeldRollOffer
}

// TryPuzzleHint implements playv1connect.PuzzleServiceHandler.
func (s *Service) TryPuzzleHint(
	ctx context.Context,
	req *connect.Request[playv1.TryPuzzleHintRequest],
) (*connect.Response[playv1.TryPuzzleHintResponse], error) {
	m, err := heldRole(ctx, req.Msg.GetCampaignId(), authz.RolePlayer)
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
	in, err := hintRollInput(req.Msg)
	if err != nil {
		return nil, err
	}

	res := &hintTryResult{}
	var names runNames
	err = db.InTx(ctx, s.pool, func(tx pgx.Tx) error {
		*res = hintTryResult{}
		var err error
		names, err = s.applyHintTry(ctx, tx, m, id, key, in, req.Msg, res)
		return err
	})
	if err != nil {
		return nil, s.dbError(ctx, "try for a puzzle hint", err)
	}
	if res.offer != nil || res.lucky != nil { // nothing was written: the question is the whole answer
		return connect.NewResponse(&playv1.TryPuzzleHintResponse{InspirationOffer: res.offer, LuckyOffer: res.lucky, HeldRollOffer: res.held}), nil
	}
	if !res.replayed {
		s.publishPuzzleChanged(m.CampaignID, id) // the master reads the try; the revision moved
	}
	return s.hintTryAnswer(ctx, m, res, names)
}

// hintRollInput reads how the player rolls: in the app, one typed d20, or the typed dice of a mode.
func hintRollInput(msg *playv1.TryPuzzleHintRequest) (rollInput, error) {
	var in rollInput
	in.extraFaces = msg.GetExtraDieFaces()
	switch roll := msg.GetRoll().(type) {
	case *playv1.TryPuzzleHintRequest_RollInApp:
		if !roll.RollInApp {
			return rollInput{}, connect.NewError(connect.CodeInvalidArgument, errors.New("roll_in_app must be true"))
		}
		in.inApp = true
	case *playv1.TryPuzzleHintRequest_D20Face:
		in.typed = int(roll.D20Face)
		if in.typed < 1 || in.typed > 20 {
			return rollInput{}, connect.NewError(connect.CodeInvalidArgument, errors.New("d20_face must be 1 to 20"))
		}
	default:
		faces, err := typedD20Faces(msg.GetD20Faces())
		if err != nil {
			return rollInput{}, err
		}
		in.typedFaces = faces
	}
	return in, nil
}

// hintTryAnswer is the answer of a try that was written (or replayed): the player's run as it is
// now, the roll, and the die that was added.
func (s *Service) hintTryAnswer(ctx context.Context, m authz.Membership, res *hintTryResult, names runNames) (*connect.Response[playv1.TryPuzzleHintResponse], error) {
	view, err := s.viewerOf(ctx, nil, heldViewer(ctx, m), res.d, res.run)
	if err != nil {
		return nil, s.dbError(ctx, "read the puzzle after a try", err)
	}
	out, err := playerRunProto(res.d, res.run, names, view)
	if err != nil {
		return nil, s.dbError(ctx, "read the puzzle after a try", err)
	}
	resp := &playv1.TryPuzzleHintResponse{
		Run:      out,
		Roll:     hintRollProto(res.try),
		Mode:     modeToProto[modeOfKey(res.try.RollMode)],
		Sources:  res.sources,
		Passed:   res.try.Passed,
		Replayed: res.replayed,
	}
	if res.bonus != nil {
		resp.BonusDice = []*playv1.BonusDie{res.bonus}
	}
	return connect.NewResponse(resp), nil
}

// hintTryByKey is the try a retry made before with the same key (found), or nothing. The key
// stands for that try, of that player, rolled that way (in the app, or with that typed die);
// a key spent on anything else, a move of the run included, is refused.
func hintTryByKey(ctx context.Context, q *playdb.Queries, m authz.Membership, runID, key string, in rollInput) (playdb.PuzzleHintTry, bool, error) {
	done, err := q.GetPuzzleHintTryByKey(ctx, playdb.GetPuzzleHintTryByKeyParams{RunID: runID, IdempotencyKey: key})
	switch {
	case err == nil:
		sameRoll := done.Physical != in.inApp && (in.inApp || sameTypedFaces(done, in))
		if done.UserID == nil || *done.UserID != m.UserID || !sameRoll {
			return playdb.PuzzleHintTry{}, false, errKeyReused()
		}
		return done, true, nil
	case !errors.Is(err, pgx.ErrNoRows):
		return playdb.PuzzleHintTry{}, false, fmt.Errorf("find the try of this idempotency key: %w", err)
	}
	if _, err := q.GetPuzzleMoveByKey(ctx, playdb.GetPuzzleMoveByKeyParams{RunID: runID, IdempotencyKey: key}); err == nil {
		return playdb.PuzzleHintTry{}, false, errKeyReused()
	} else if !errors.Is(err, pgx.ErrNoRows) {
		return playdb.PuzzleHintTry{}, false, fmt.Errorf("find the move of this idempotency key: %w", err)
	}
	return playdb.PuzzleHintTry{}, false, nil
}

// hintTryOpen refuses a try on a solved or stopped puzzle, or on one without a hint check.
func hintTryOpen(d puzzleDef, run playdb.PuzzleRun, now time.Time) error {
	switch {
	case run.SolvedAt != nil:
		return puzzleBlocked(playv1.PuzzleBlockedReason_PUZZLE_BLOCKED_REASON_SOLVED, "the puzzle is solved")
	case stopOf(d, run, now) != playv1.PuzzleStopReason_PUZZLE_STOP_REASON_UNSPECIFIED:
		return puzzleBlocked(playv1.PuzzleBlockedReason_PUZZLE_BLOCKED_REASON_STOPPED, "a limit stopped the puzzle")
	case d.hintCheck == nil:
		return puzzleBlocked(playv1.PuzzleBlockedReason_PUZZLE_BLOCKED_REASON_NO_HINT_CHECK, "this puzzle has no skill check for hints")
	}
	return nil
}

// nextHintToTry is the hint this player would read next, after the master's released ones
// and the ones they won; refused when none is left or they tried for it already.
func nextHintToTry(ctx context.Context, q *playdb.Queries, m authz.Membership, d puzzleDef, run playdb.PuzzleRun) (int, error) {
	won, err := q.GetPuzzleHintCursor(ctx, playdb.GetPuzzleHintCursorParams{RunID: run.ID, UserID: &m.UserID})
	if err != nil {
		return 0, fmt.Errorf("read the hints the player won: %w", err)
	}
	next := max(int(run.ReleasedHints), int(won))
	if next >= len(d.hints) {
		return 0, puzzleBlocked(playv1.PuzzleBlockedReason_PUZZLE_BLOCKED_REASON_NO_MORE_HINTS, "there is no hint left for you")
	}
	tried, err := q.HasTriedPuzzleHint(ctx, playdb.HasTriedPuzzleHintParams{RunID: run.ID, UserID: &m.UserID, HintIndex: clamp32(next, 0, maxHints)})
	if err != nil {
		return 0, fmt.Errorf("read whether the player tried: %w", err)
	}
	if tried {
		return 0, puzzleBlocked(playv1.PuzzleBlockedReason_PUZZLE_BLOCKED_REASON_HINT_ALREADY_TRIED, "you tried for this hint already")
	}
	return next, nil
}

// hintTryTx is one try inside its transaction: the session, the puzzle and the run it locked.
type hintTryTx struct {
	s       *Service
	tx      pgx.Tx
	q       *playdb.Queries
	m       authz.Membership
	session playdb.GameSession
	d       puzzleDef
	run     playdb.PuzzleRun
	key     string
	in      rollInput
	raw     *playv1.TryPuzzleHintRequest
	res     *hintTryResult
}

// hintTryPlan is what the try works out before it rolls: who rolls, the hint they try for, what their
// sheet adds to the skill, how the check is rolled and the modifier.
type hintTryPlan struct {
	who    link.Character
	next   int
	option link.SceneOption
	cm     checkMode
	bonus  int
}

// applyHintTry is the try's transaction. The session's row is locked first and then
// the run's, as every change to a run does, so a try, a move and the master's release
// of the same hint take turns.
func (s *Service) applyHintTry(ctx context.Context, tx pgx.Tx, m authz.Membership, puzzleID, key string, in rollInput, raw *playv1.TryPuzzleHintRequest, res *hintTryResult) (runNames, error) {
	h := &hintTryTx{s: s, tx: tx, q: s.queriesIn(tx), m: m, key: key, in: in, raw: raw, res: res}
	if err := h.lock(ctx, puzzleID); err != nil {
		return nil, err
	}
	// A retry of a try already made answers as the first call did, and rolls nothing.
	done, replayed, err := hintTryByKey(ctx, h.q, m, h.run.ID, key, in)
	if err != nil {
		return nil, err
	}
	if replayed {
		res.try, res.replayed = done, true
		if done.BonusSides != nil && done.BonusFace != nil { // the die of the first answer
			res.bonus = &playv1.BonusDie{SourceKey: bardicInspirationFt, Sides: *done.BonusSides, Face: *done.BonusFace, Used: true}
		}
		return s.namesOfRuns(ctx, tx, m.CampaignID, h.run)
	}

	now := s.now()
	plan, err := h.plan(ctx, now)
	if err != nil {
		return nil, err
	}
	step, err := h.roll(ctx, plan)
	if err != nil {
		return nil, err
	}
	if step.Held != nil {
		res.offer, res.lucky, res.held = s.outsideOffers(ctx, tx, step.Held)
		return nil, nil
	}
	return h.write(ctx, plan, step, now)
}

// lock locks the session's row, reads the shown puzzle and locks its run.
func (h *hintTryTx) lock(ctx context.Context, puzzleID string) error {
	var err error
	if h.session, err = lockOpenSession(ctx, h.q, h.m.CampaignID); err != nil {
		return err
	}
	if h.d, _, err = h.s.shownRun(ctx, h.q, h.session, h.m.CampaignID, puzzleID); err != nil {
		return err
	}
	if h.run, err = h.q.GetPuzzleRunForUpdate(ctx, playdb.GetPuzzleRunForUpdateParams{GameSessionID: h.session.ID, PuzzleID: puzzleID}); err != nil {
		return fmt.Errorf("lock the puzzle's run: %w", err)
	}
	h.res.d, h.res.run = h.d, h.run
	return nil
}

// plan refuses a try that cannot be made and works out the one that can, before anything is rolled.
func (h *hintTryTx) plan(ctx context.Context, now time.Time) (hintTryPlan, error) {
	if err := hintTryOpen(h.d, h.run, now); err != nil {
		return hintTryPlan{}, err
	}
	who, has, err := h.s.myCharacter(ctx, h.tx, h.m)
	if err != nil {
		return hintTryPlan{}, err
	}
	if !has {
		return hintTryPlan{}, puzzleBlocked(playv1.PuzzleBlockedReason_PUZZLE_BLOCKED_REASON_NO_CHARACTER, "you have no living character to roll for")
	}
	next, err := nextHintToTry(ctx, h.q, h.m, h.d, h.run)
	if err != nil {
		return hintTryPlan{}, err
	}
	force, err := h.s.dice.ForcedDice(ctx, h.tx, h.m.CampaignID, h.m.UserID)
	if err != nil {
		return hintTryPlan{}, err
	}
	if force.refuses(h.in.inApp) {
		return hintTryPlan{}, puzzleBlocked(playv1.PuzzleBlockedReason_PUZZLE_BLOCKED_REASON_WRONG_DICE_MODE, "this is not how the campaign has you roll your dice")
	}
	plan := hintTryPlan{who: who, next: next}
	if err := h.skill(ctx, &plan); err != nil {
		return hintTryPlan{}, err
	}
	return plan, nil
}

// skill reads what the character's sheet adds to the hint's skill and how the check is rolled.
func (h *hintTryTx) skill(ctx context.Context, plan *hintTryPlan) error {
	key := h.d.hintCheck.GetSkillKey()
	options, err := h.s.roster.SceneOptions(ctx, h.tx, h.m.CampaignID, plan.who.ID, []string{key})
	if err != nil {
		return err
	}
	if len(options) != 1 || !options[0].Known {
		return puzzleBlocked(playv1.PuzzleBlockedReason_PUZZLE_BLOCKED_REASON_NO_SKILL, "your character has no numbers for this skill")
	}
	plan.option = options[0]
	ability, isCheck := checkKey(key)
	if plan.cm, err = h.s.checkModeOf(ctx, h.tx, h.m.CampaignID, h.session.ID, plan.who.ID, key, ability, isCheck); err != nil {
		return err
	}
	plan.bonus, err = h.s.withEffectDice(h.in, h.in.extraFaces, &plan.cm, plan.option.Bonus)
	return err
}

// roll rolls the d20. A character that holds a Bardic Inspiration die has the d20 kept until the player answers,
// before anything is spent or written, and so has a halfling's d20 that shows a 1, for the
// Lucky question; the answer brings it back (with the die added, or rolled again).
func (h *hintTryTx) roll(ctx context.Context, plan hintTryPlan) (gateResult, error) {
	return h.s.heldGate(ctx, h.s.outsideGate(h.tx, h.q), rollSpec{
		Kind: heldHintTry, Key: h.key, Request: h.raw, Roller: heldRoller{CampaignID: h.m.CampaignID, CharacterID: plan.who.ID, UserID: h.m.UserID},
		In: h.in, Modifier: plan.bonus, Mode: plan.cm.Mode, Reliable: plan.option.ReliableTalent, Lucky: plan.option.Lucky, MayAsk: true,
	})
}

// write spends what the roll used, records the try and moves the run's revision with it, so the
// app applies the read that follows.
func (h *hintTryTx) write(ctx context.Context, plan hintTryPlan, step gateResult, now time.Time) (runNames, error) {
	if err := h.spend(ctx, plan.cm); err != nil {
		return nil, err
	}
	try, err := h.record(ctx, plan, step, now)
	if err != nil {
		return nil, err
	}
	p := paramsOf(&h.run)
	p.Revision, p.UpdatedAt = h.run.Revision+1, now
	saved, err := h.q.SavePuzzleRun(ctx, p)
	if err != nil {
		return nil, fmt.Errorf("save the puzzle's run: %w", err)
	}
	h.res.try, h.res.run = try, saved
	names, err := h.s.namerFor(ctx, h.tx, h.m.CampaignID)
	if err != nil {
		return nil, err
	}
	h.res.sources = plan.cm.shownCheck(names)
	return h.s.namesOfRuns(ctx, h.tx, h.m.CampaignID, saved)
}

// spend uses the once-only effects and the helps the check counted.
func (h *hintTryTx) spend(ctx context.Context, cm checkMode) error {
	spendTx, err := h.s.openTx(ctx, combatTx{tx: h.tx, q: h.q, session: h.session, now: h.s.now(), svc: h.s, master: true})
	if err != nil {
		return err
	}
	if err := h.s.spendOnceEffects(ctx, spendTx, cm.Rolled); err != nil {
		return err
	}
	return h.s.spendCheckHelps(ctx, spendTx, cm.Helps)
}

// record writes the try row for the roll.
func (h *hintTryTx) record(ctx context.Context, plan hintTryPlan, step gateResult, now time.Time) (playdb.PuzzleHintTry, error) {
	d20 := step.Roll
	dieFace := 0
	if step.Bonus != nil {
		dieFace, h.res.bonus = int(step.Bonus.Face), step.Bonus // the die the player added after rolling: the total holds it
	}
	passed := d20.Total >= int(h.d.hintCheck.GetDc())
	var granted *int32
	if passed {
		g := clamp32(plan.next+1, 1, maxHints)
		granted = &g
	}
	var bonusSides, bonusFace *int32 // the die the try keeps, for a retry to answer with it
	if step.Bonus != nil {
		bonusSides, bonusFace = new(clamp32(int(step.Bonus.Sides), 2, 100)), new(clamp32(int(step.Bonus.Face), 1, 100))
	}
	var luckyAt *int32 // the d20 Lucky rolled again: the try keeps where, its faces the new one
	if step.Lucky != nil {
		luckyAt = new(clamp32(int(step.Lucky.Index), 0, 1))
	}
	try, err := h.q.InsertPuzzleHintTry(ctx, playdb.InsertPuzzleHintTryParams{
		RunID: h.run.ID, UserID: &h.m.UserID, CharacterID: &plan.who.ID, IdempotencyKey: h.key, HintIndex: clamp32(plan.next, 0, maxHints),
		Passed: passed, GrantedCount: granted, D20: clamp32(d20.Face(), 1, 20), Modifier: clamp32(plan.bonus+dieFace, -1000, 1000),
		Total: clamp32(d20.Total, -1000, 1000), Physical: d20.Physical, CreatedAt: now,
		D20B: clamp32(d20.otherFace(), 0, 20), Counted: clamp32(d20.Index, 0, 1), RollMode: modeKey(plan.cm.Mode),
		LuckyIndex: luckyAt, BonusSides: bonusSides, BonusFace: bonusFace,
	})
	if err != nil {
		return playdb.PuzzleHintTry{}, fmt.Errorf("record the try: %w", err)
	}
	return try, nil
}

// hintRollProto is the try's d20: the pair when it was rolled with advantage or
// disadvantage, with the one that counts marked, and the die Lucky rolled again.
func hintRollProto(t playdb.PuzzleHintTry) *playv1.DiceRoll {
	if t.D20B == 0 {
		out := treatedRoll(t.D20, t.Modifier, t.Total, t.Physical)
		out.Rerolled = hintLucky(t).proto()
		return out
	}
	out := diceRoll(2, 20, pairOf(t.D20, t.D20B, t.Counted), t.Modifier, t.Total, t.Physical)
	out.CountedIndex = t.Counted
	out.Rerolled = hintLucky(t).proto()
	return markTreated(out, t.Modifier, t.Total)
}

// hintFaces are the try's d20 in the order rolled: the pair, or the one.
func hintFaces(t playdb.PuzzleHintTry) []int32 {
	if t.D20B == 0 {
		return []int32{t.D20}
	}
	return pairOf(t.D20, t.D20B, t.Counted)
}

// hintLucky is the d20 the halfling's Lucky rolled again on the try (it only ever rolls a 1
// again, and the try keeps the new face): nil when none was.
func hintLucky(t playdb.PuzzleHintTry) *luckyReroll {
	faces := hintFaces(t)
	if t.LuckyIndex == nil || int(*t.LuckyIndex) >= len(faces) {
		return nil
	}
	return &luckyReroll{Index: *t.LuckyIndex, From: 1, To: faces[*t.LuckyIndex]}
}

// sameTypedFaces says whether the dice a call types are the dice of the try made before: the
// faces it rolled, with the 1 Lucky rolled again put back (the request typed the 1).
func sameTypedFaces(done playdb.PuzzleHintTry, in rollInput) bool {
	want := hintFaces(done)
	if r := hintLucky(done); r != nil {
		want[r.Index] = r.From
	}
	if len(in.typedFaces) == 0 {
		return len(want) == 1 && int(want[0]) == in.typed
	}
	if len(want) != len(in.typedFaces) {
		return false
	}
	for i, f := range in.typedFaces {
		if f != int(want[i]) {
			return false
		}
	}
	return true
}
