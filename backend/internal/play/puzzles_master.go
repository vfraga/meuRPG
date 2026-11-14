package play

import (
	"context"
	"errors"
	"fmt"
	"slices"
	"time"

	"connectrpc.com/connect"
	"github.com/jackc/pgx/v5"
	"google.golang.org/protobuf/proto"

	playv1 "github.com/PuraFome/meuRPG/backend/gen/meurpg/play/v1"
	"github.com/PuraFome/meuRPG/backend/internal/platform/idem"
	"github.com/PuraFome/meuRPG/backend/internal/play/link"
	"github.com/PuraFome/meuRPG/backend/internal/play/playdb"
	"github.com/PuraFome/meuRPG/backend/internal/rules/puzzle"
)

// The master's calls on the campaign's puzzles: make, edit, list, archive. None of
// them needs an open session, and none touches a run but to know whether the puzzle
// was ever shown.

// buildPuzzle checks everything the master wrote and works out the stored forms. A
// seed of 0 draws one (the kinds that generate their start use it).
func (s *Service) buildPuzzle(ctx context.Context, tx pgx.Tx, campaignID string, in puzzleInput) (puzzleBuilt, string, *playv1.PuzzleOnSolve, error) {
	if err := in.checkTexts(); err != nil {
		return puzzleBuilt{}, "", nil, err
	}
	kind := kindOf(in.config)
	if kind == nil {
		return puzzleBuilt{}, "", nil, puzzleInvalid(playv1.PuzzleInvalidReason_PUZZLE_INVALID_REASON_KIND, "config", "config must have one kind")
	}
	seed := in.seed
	if kind.generates() && seed <= 0 {
		seed = s.newSeed()
	}
	built, err := kind.build(in, seed)
	if err != nil {
		return puzzleBuilt{}, "", nil, err
	}
	action, target, err := s.checkOnSolve(ctx, tx, campaignID, in.onSolve)
	if err != nil {
		return puzzleBuilt{}, "", nil, err
	}
	if err := s.checkCipherKeyClue(ctx, tx, campaignID, built.config); err != nil {
		return puzzleBuilt{}, "", nil, err
	}
	if built.hintCheck, err = s.checkHintCheck(in.hintCheck, in.hints); err != nil {
		return puzzleBuilt{}, "", nil, err
	}
	if built.parts, err = s.checkParts(ctx, tx, campaignID, in.parts); err != nil {
		return puzzleBuilt{}, "", nil, err
	}
	if built.onWrong, err = s.checkOnWrong(ctx, tx, campaignID, kind, in.onWrong); err != nil {
		return puzzleBuilt{}, "", nil, err
	}
	return built, action, target, nil
}

// fromBuilt is a puzzle row's worth of a checked puzzle, for an insert or an update.
func fromBuilt(kind puzzleKind, in puzzleInput, b puzzleBuilt, action string, target *playv1.PuzzleOnSolve) (puzzleDef, error) {
	d := puzzleDef{
		kind: kind, config: b.config, solution: b.solution, start: b.start, hints: in.hints, onSolve: target,
		hintCheck: b.hintCheck, parts: b.parts, onWrong: b.onWrong,
	}
	least, err := kind.minimum(d, b.start)
	if err != nil {
		return puzzleDef{}, err
	}
	if !least.GetSolvable() {
		return puzzleDef{}, puzzleInvalid(playv1.PuzzleInvalidReason_PUZZLE_INVALID_REASON_UNSOLVABLE, "solution", "the puzzle cannot be solved from its start")
	}
	m := least.GetMoves()
	d.row = playdb.Puzzle{Kind: kind.stored(), Name: in.name, Clue: in.clue, Seed: b.seed, MinimumMoves: &m, SolveAction: action}
	return d, nil
}

// puzzleColumns is the JSON of the columns of a built puzzle.
type puzzleColumns struct {
	config, solution, start, hints, target, parts, onWrong []byte
	hintSkill                                              *string
	hintDC                                                 *int32
}

func columnsOf(d puzzleDef) (puzzleColumns, error) {
	hints, err := jsonMarshal(append([]string{}, d.hints...))
	if err != nil {
		return puzzleColumns{}, fmt.Errorf("encode the hints: %w", err)
	}
	c := puzzleColumns{config: toJSON(d.config), solution: toJSON(d.solution), start: toJSON(d.start), hints: hints}
	if d.row.SolveAction != solveNotify || d.onSolve.GetMessage() != "" {
		c.target = toJSON(d.onSolve)
	}
	parts := make([]storedPart, len(d.parts))
	for i, p := range d.parts {
		parts[i] = storedPart{CharacterID: p.GetCharacterId(), Text: p.GetText()}
	}
	if c.parts, err = jsonMarshal(parts); err != nil {
		return puzzleColumns{}, fmt.Errorf("encode the parts: %w", err)
	}
	if d.hintCheck != nil {
		skill, dc := d.hintCheck.GetSkillKey(), d.hintCheck.GetDc()
		c.hintSkill, c.hintDC = &skill, &dc
	}
	if d.onWrong != nil {
		c.onWrong = toJSON(d.onWrong)
	}
	return c, nil
}

// puzzleInputOf reads a create or update request.
func puzzleInputOf(name string, cfg *playv1.PuzzleConfig, sol *playv1.PuzzleSolution, start *playv1.PuzzleState, seed int64, clue string, hints []string, on *playv1.PuzzleOnSolve, extras puzzleExtras) puzzleInput {
	return puzzleInput{
		name: name, config: cfg, solution: sol, start: start, seed: seed, clue: clue, hints: hints, onSolve: on,
		hintCheck: extras.hintCheck, parts: extras.parts, onWrong: extras.onWrong,
	}
}

// puzzleExtras is what a create or an edit carries besides the first ten fields:
// the hint check, the split information and "Ao errar".
type puzzleExtras struct {
	hintCheck *playv1.PuzzleHintCheck
	parts     []*playv1.PuzzlePart
	onWrong   *playv1.PuzzleOnWrong
}

// CreatePuzzle implements playv1connect.PuzzleServiceHandler.
func (s *Service) CreatePuzzle(
	ctx context.Context,
	req *connect.Request[playv1.CreatePuzzleRequest],
) (*connect.Response[playv1.CreatePuzzleResponse], error) {
	m, err := requireMaster(ctx, req.Msg.GetCampaignId())
	if err != nil {
		return nil, err
	}
	in := puzzleInputOf(req.Msg.GetName(), req.Msg.GetConfig(), req.Msg.GetSolution(), req.Msg.GetStart(), req.Msg.GetSeed(), req.Msg.GetClue(), req.Msg.GetHints(), req.Msg.GetOnSolve(),
		puzzleExtras{hintCheck: req.Msg.GetHintCheck(), parts: req.Msg.GetParts(), onWrong: req.Msg.GetOnWrong()})
	key, err := idem.Clean(req.Msg.GetIdempotencyKey())
	if err != nil {
		return nil, err
	}
	// The key is unique in the campaign, and kept with a hash of the whole request: a retry
	// returns the first puzzle (with the start it was drawn) and makes no other.
	scopedKey, requestHash := idem.Scope(m.CampaignID, key), idem.Hash(req.Msg)
	var row playdb.Puzzle
	err = s.txOrNone(ctx, m.CampaignID, func(tx pgx.Tx, h *hints, _ string) error {
		q := s.queriesIn(tx)
		var replayed bool
		var err error
		row, replayed, err = idem.Create(ctx, scopedKey, requestHash, q.GetPuzzleByCreateKey,
			func(p playdb.Puzzle) *string { return p.CreateHash },
			func() (playdb.Puzzle, error) {
				return s.insertPuzzle(ctx, tx, m.CampaignID, in, scopedKey, requestHash)
			})
		if err != nil {
			return err
		}
		// A new puzzle is in the master's list of the session's puzzles: a screen that shows it reads again. The retry of the key
		// wrote nothing and has nothing to tell again but that.
		if replayed {
			h.replay()
		}
		h.touch()
		return nil
	})
	if err != nil {
		return nil, s.dbError(ctx, "create a puzzle", err)
	}
	out, err := s.masterPuzzle(ctx, row, false)
	if err != nil {
		return nil, s.dbError(ctx, "read the new puzzle", err)
	}
	return connect.NewResponse(&playv1.CreatePuzzleResponse{Puzzle: out}), nil
}

// insertPuzzle checks what the master wrote and stores the puzzle under the request's key.
func (s *Service) insertPuzzle(ctx context.Context, tx pgx.Tx, campaignID string, in puzzleInput, scopedKey, requestHash *string) (playdb.Puzzle, error) {
	d, action, cols, err := s.builtColumns(ctx, tx, campaignID, kindOf(in.config), in)
	if err != nil {
		return playdb.Puzzle{}, err
	}
	p, err := s.queriesIn(tx).InsertPuzzle(ctx, playdb.InsertPuzzleParams{
		CampaignID: campaignID, Kind: d.row.Kind, Name: d.row.Name, Config: cols.config, Solution: cols.solution,
		Seed: d.row.Seed, Start: cols.start, MinimumMoves: d.row.MinimumMoves, Clue: d.row.Clue, Hints: cols.hints,
		SolveAction: action, SolveTarget: cols.target, HintSkill: cols.hintSkill, HintDc: cols.hintDC, Parts: cols.parts, OnWrong: cols.onWrong,
		CreateKey: scopedKey, CreateHash: requestHash, CreatedAt: s.now(),
	})
	if err != nil && !errors.Is(err, pgx.ErrNoRows) {
		return p, fmt.Errorf("insert puzzle: %w", err)
	}
	return p, err
}

// builtColumns checks everything the master wrote and gives the puzzle's definition, its solve
// action and the JSON of its columns.
func (s *Service) builtColumns(ctx context.Context, tx pgx.Tx, campaignID string, kind puzzleKind, in puzzleInput) (puzzleDef, string, puzzleColumns, error) {
	built, action, target, err := s.buildPuzzle(ctx, tx, campaignID, in)
	if err != nil {
		return puzzleDef{}, "", puzzleColumns{}, err
	}
	d, err := fromBuilt(kind, in, built, action, target)
	if err != nil {
		return puzzleDef{}, "", puzzleColumns{}, err
	}
	cols, err := columnsOf(d)
	if err != nil {
		return puzzleDef{}, "", puzzleColumns{}, err
	}
	return d, action, cols, nil
}

// masterPuzzle is the master's message for a row.
func (s *Service) masterPuzzle(ctx context.Context, row playdb.Puzzle, shown bool) (*playv1.Puzzle, error) {
	d, err := decodePuzzle(row)
	if err != nil {
		return nil, err
	}
	out, err := puzzleProto(d, shown)
	if err != nil {
		return nil, err
	}
	return out, s.flagParts(ctx, row.CampaignID, out)
}

// flagParts marks, on the master's copy, the parts whose owner is not a living player
// character of the active party any more (a character that died or left): nobody reads
// them, and the master should know. It reads through the pool.
func (s *Service) flagParts(ctx context.Context, campaignID string, p *playv1.Puzzle) error {
	if !slices.ContainsFunc(p.GetParts(), func(pt *playv1.PuzzlePart) bool { return pt.GetCharacterId() != "" }) {
		return nil
	}
	party, err := s.roster.CombatParty(ctx, nil, campaignID)
	if err != nil {
		return err
	}
	for _, pt := range p.GetParts() {
		if pt.GetCharacterId() != "" {
			pt.OwnerUnavailable = !slices.ContainsFunc(party, func(c link.Character) bool { return c.ID == pt.GetCharacterId() })
		}
	}
	return nil
}

// UpdatePuzzle implements playv1connect.PuzzleServiceHandler.
func (s *Service) UpdatePuzzle(
	ctx context.Context,
	req *connect.Request[playv1.UpdatePuzzleRequest],
) (*connect.Response[playv1.UpdatePuzzleResponse], error) {
	m, err := requireMaster(ctx, req.Msg.GetCampaignId())
	if err != nil {
		return nil, err
	}
	id, err := parsePuzzleID(req.Msg.GetPuzzleId())
	if err != nil {
		return nil, err
	}
	in := puzzleInputOf(req.Msg.GetName(), req.Msg.GetConfig(), req.Msg.GetSolution(), req.Msg.GetStart(), req.Msg.GetSeed(), req.Msg.GetClue(), req.Msg.GetHints(), req.Msg.GetOnSolve(),
		puzzleExtras{hintCheck: req.Msg.GetHintCheck(), parts: req.Msg.GetParts(), onWrong: req.Msg.GetOnWrong()})
	var row playdb.Puzzle
	err = s.txOrNone(ctx, m.CampaignID, func(tx pgx.Tx, h *hints, _ string) error {
		var err error
		if row, err = s.updatePuzzle(ctx, tx, m.CampaignID, id, in); err != nil {
			return err
		}
		h.touch() // the master's list of the session's puzzles shows the edited puzzle (and no longer a run prepared at the old start)
		return nil
	})
	if err != nil {
		return nil, s.dbError(ctx, "update a puzzle", err)
	}
	out, err := s.masterPuzzle(ctx, row, false)
	if err != nil {
		return nil, s.dbError(ctx, "read the edited puzzle", err)
	}
	return connect.NewResponse(&playv1.UpdatePuzzleResponse{Puzzle: out}), nil
}

// updatePuzzle is the edit's transaction: the puzzle is locked, must be one nobody saw yet, and keeps its kind.
func (s *Service) updatePuzzle(ctx context.Context, tx pgx.Tx, campaignID, id string, in puzzleInput) (playdb.Puzzle, error) {
	q := s.queriesIn(tx)
	old, oldDef, err := lockUnshownPuzzle(ctx, q, campaignID, id)
	if err != nil {
		return playdb.Puzzle{}, err
	}
	kind := kindOf(in.config)
	if kind == nil || kind.id() != oldDef.kind.id() {
		return playdb.Puzzle{}, puzzleInvalid(playv1.PuzzleInvalidReason_PUZZLE_INVALID_REASON_KIND, "config", "the kind of a puzzle cannot change")
	}
	// A seed of 0 keeps the start the puzzle has, as long as what makes it (the
	// config and the answer) did not change.
	if kind.generates() && in.seed <= 0 && sameAnswer(oldDef, in) {
		in.seed = old.Seed
	}
	d, action, cols, err := s.builtColumns(ctx, tx, campaignID, kind, in)
	if err != nil {
		return playdb.Puzzle{}, err
	}
	// A run prepared with a new start before the edit would carry a start that no
	// longer fits (another board, another mural): drop it, the master draws again.
	if err := q.DeletePreparedPuzzleRuns(ctx, id); err != nil {
		return playdb.Puzzle{}, fmt.Errorf("drop the prepared runs: %w", err)
	}
	row, err := q.UpdatePuzzle(ctx, playdb.UpdatePuzzleParams{
		CampaignID: campaignID, ID: id, Name: d.row.Name, Config: cols.config, Solution: cols.solution, Seed: d.row.Seed,
		Start: cols.start, MinimumMoves: d.row.MinimumMoves, Clue: d.row.Clue, Hints: cols.hints,
		SolveAction: action, SolveTarget: cols.target, HintSkill: cols.hintSkill, HintDc: cols.hintDC, Parts: cols.parts, OnWrong: cols.onWrong,
		UpdatedAt: s.now(),
	})
	if err != nil {
		return playdb.Puzzle{}, fmt.Errorf("update puzzle: %w", err)
	}
	return row, nil
}

// lockUnshownPuzzle locks the campaign's puzzle for an edit and decodes it; a puzzle that was shown in
// a session cannot be edited any more.
func lockUnshownPuzzle(ctx context.Context, q *playdb.Queries, campaignID, id string) (playdb.Puzzle, puzzleDef, error) {
	old, err := q.GetPuzzleForUpdate(ctx, playdb.GetPuzzleForUpdateParams{CampaignID: campaignID, ID: id})
	if errors.Is(err, pgx.ErrNoRows) {
		return playdb.Puzzle{}, puzzleDef{}, errPuzzleNotFound()
	}
	if err != nil {
		return playdb.Puzzle{}, puzzleDef{}, fmt.Errorf("find puzzle: %w", err)
	}
	shown, err := q.ListShownPuzzleIDs(ctx, campaignID)
	if err != nil {
		return playdb.Puzzle{}, puzzleDef{}, fmt.Errorf("read which puzzles were shown: %w", err)
	}
	if slices.Contains(shown, id) {
		return playdb.Puzzle{}, puzzleDef{}, puzzleBlocked(playv1.PuzzleBlockedReason_PUZZLE_BLOCKED_REASON_ALREADY_SHOWN, "the puzzle was shown in a session: it cannot be edited any more")
	}
	oldDef, err := decodePuzzle(old)
	if err != nil {
		return playdb.Puzzle{}, puzzleDef{}, err
	}
	return old, oldDef, nil
}

// sameAnswer says whether the request's config and answer are the puzzle's own, so
// its generated start still fits.
func sameAnswer(old puzzleDef, in puzzleInput) bool {
	return proto.Equal(old.config, in.config) && (old.solution.GetKind() == nil && in.solution.GetKind() == nil || proto.Equal(old.solution, in.solution))
}

// PreviewPuzzleStart implements playv1connect.PuzzleServiceHandler.
func (s *Service) PreviewPuzzleStart(
	ctx context.Context,
	req *connect.Request[playv1.PreviewPuzzleStartRequest],
) (*connect.Response[playv1.PreviewPuzzleStartResponse], error) {
	if _, err := requireMaster(ctx, req.Msg.GetCampaignId()); err != nil {
		return nil, err
	}
	kind := kindOf(req.Msg.GetConfig())
	if kind == nil {
		return nil, puzzleInvalid(playv1.PuzzleInvalidReason_PUZZLE_INVALID_REASON_KIND, "config", "config must have one kind")
	}
	if !kind.generates() {
		return nil, puzzleBlocked(playv1.PuzzleBlockedReason_PUZZLE_BLOCKED_REASON_NO_GENERATED_START, "this kind has no generated start")
	}
	seed := req.Msg.GetSeed()
	if seed <= 0 {
		seed = s.newSeed()
	}
	built, err := kind.build(puzzleInput{config: req.Msg.GetConfig(), solution: req.Msg.GetSolution()}, seed)
	if err != nil {
		return nil, err
	}
	d := puzzleDef{kind: kind, config: built.config, solution: built.solution, start: built.start}
	least, err := kind.minimum(d, built.start)
	if err != nil {
		return nil, s.dbError(ctx, "work out the fewest moves of a start", err)
	}
	return connect.NewResponse(&playv1.PreviewPuzzleStartResponse{Seed: seed, Start: built.start, Minimum: least}), nil
}

// PreviewPuzzleCipher implements playv1connect.PuzzleServiceHandler.
func (s *Service) PreviewPuzzleCipher(
	ctx context.Context,
	req *connect.Request[playv1.PreviewPuzzleCipherRequest],
) (*connect.Response[playv1.PreviewPuzzleCipherResponse], error) {
	if _, err := requireMaster(ctx, req.Msg.GetCampaignId()); err != nil {
		return nil, err
	}
	sol := req.Msg.GetSolution()
	if sol == nil {
		return nil, puzzleInvalid(playv1.PuzzleInvalidReason_PUZZLE_INVALID_REASON_KIND, "solution", "solution must be a cipher's")
	}
	text, err := puzzle.Encipher(sol.GetMessage(), cipherKey(sol))
	if err != nil {
		return nil, rulesError(err, "solution.cipher")
	}
	return connect.NewResponse(&playv1.PreviewPuzzleCipherResponse{Ciphertext: text}), nil
}

// shownSet is the set of the campaign's puzzle IDs that were shown in any session.
func (s *Service) shownSet(ctx context.Context, campaignID string) (map[string]bool, error) {
	return s.shownSetIn(ctx, s.queries, campaignID)
}

// shownSetIn is shownSet read through q (a transaction's, or the pool's).
func (s *Service) shownSetIn(ctx context.Context, q *playdb.Queries, campaignID string) (map[string]bool, error) {
	ids, err := q.ListShownPuzzleIDs(ctx, campaignID)
	if err != nil {
		return nil, fmt.Errorf("read which puzzles were shown: %w", err)
	}
	set := make(map[string]bool, len(ids))
	for _, id := range ids {
		set[id] = true
	}
	return set, nil
}

// ListPuzzles implements playv1connect.PuzzleServiceHandler.
func (s *Service) ListPuzzles(
	ctx context.Context,
	req *connect.Request[playv1.ListPuzzlesRequest],
) (*connect.Response[playv1.ListPuzzlesResponse], error) {
	m, err := requireMaster(ctx, req.Msg.GetCampaignId())
	if err != nil {
		return nil, err
	}
	rows, err := s.queries.ListPuzzles(ctx, playdb.ListPuzzlesParams{CampaignID: m.CampaignID, IncludeArchived: req.Msg.GetIncludeArchived()})
	if err != nil {
		return nil, s.dbError(ctx, "list puzzles", err)
	}
	shown, err := s.shownSet(ctx, m.CampaignID)
	if err != nil {
		return nil, s.dbError(ctx, "list puzzles", err)
	}
	res := &playv1.ListPuzzlesResponse{}
	for _, row := range rows {
		p, err := s.masterPuzzle(ctx, row, shown[row.ID])
		if err != nil {
			return nil, s.dbError(ctx, "read a puzzle", err)
		}
		res.Puzzles = append(res.Puzzles, p)
	}
	return connect.NewResponse(res), nil
}

// GetPuzzle implements playv1connect.PuzzleServiceHandler.
func (s *Service) GetPuzzle(
	ctx context.Context,
	req *connect.Request[playv1.GetPuzzleRequest],
) (*connect.Response[playv1.GetPuzzleResponse], error) {
	m, err := requireMaster(ctx, req.Msg.GetCampaignId())
	if err != nil {
		return nil, err
	}
	id, err := parsePuzzleID(req.Msg.GetPuzzleId())
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
	shown, err := s.shownSet(ctx, m.CampaignID)
	if err != nil {
		return nil, s.dbError(ctx, "find a puzzle", err)
	}
	p, err := s.masterPuzzle(ctx, row, shown[row.ID])
	if err != nil {
		return nil, s.dbError(ctx, "read a puzzle", err)
	}
	return connect.NewResponse(&playv1.GetPuzzleResponse{Puzzle: p}), nil
}

// setArchived archives or brings back a puzzle, and tells the session's streams:
// an archived puzzle is not shown to the players any more.
func (s *Service) setArchived(ctx context.Context, campaignID, rawID string, archive bool) (*playv1.Puzzle, error) {
	id, err := parsePuzzleID(rawID)
	if err != nil {
		return nil, err
	}
	var row playdb.Puzzle
	err = s.txOrNone(ctx, campaignID, func(tx pgx.Tx, h *hints, _ string) error {
		var changed bool
		var err error
		if row, changed, err = s.flipArchived(ctx, s.queriesIn(tx), campaignID, id, archive); err != nil || !changed {
			return err
		}
		shown, err := s.shownSetIn(ctx, s.queriesIn(tx), campaignID)
		if err != nil {
			return err
		}
		if shown[id] {
			s.tellPuzzleChanged(h, campaignID, id) // a shown puzzle leaves, or comes back to, the players' list
		} else {
			h.touch() // the master's list of puzzles shows it archived, or back
		}
		return nil
	})
	if err != nil {
		return nil, s.dbError(ctx, "archive a puzzle", err)
	}
	shown, err := s.shownSet(ctx, campaignID)
	if err != nil {
		return nil, s.dbError(ctx, "archive a puzzle", err)
	}
	p, err := s.masterPuzzle(ctx, row, shown[row.ID])
	if err != nil {
		return nil, s.dbError(ctx, "read a puzzle", err)
	}
	return p, nil
}

// flipArchived archives or brings back the locked puzzle, and says whether the row changed: a puzzle
// that is so already stays as it is.
func (s *Service) flipArchived(ctx context.Context, q *playdb.Queries, campaignID, id string, archive bool) (playdb.Puzzle, bool, error) {
	cur, err := q.GetPuzzleForUpdate(ctx, playdb.GetPuzzleForUpdateParams{CampaignID: campaignID, ID: id})
	if errors.Is(err, pgx.ErrNoRows) {
		return playdb.Puzzle{}, false, errPuzzleNotFound()
	}
	if err != nil {
		return playdb.Puzzle{}, false, fmt.Errorf("find puzzle: %w", err)
	}
	if (cur.ArchivedAt != nil) == archive {
		return cur, false, nil // already so: nothing changes
	}
	var at *time.Time
	now := s.now()
	if archive {
		at = &now
	}
	row, err := q.SetPuzzleArchived(ctx, playdb.SetPuzzleArchivedParams{CampaignID: campaignID, ID: id, ArchivedAt: at, UpdatedAt: now})
	if err != nil {
		return playdb.Puzzle{}, false, fmt.Errorf("archive puzzle: %w", err)
	}
	return row, true, nil
}

// ArchivePuzzle implements playv1connect.PuzzleServiceHandler.
func (s *Service) ArchivePuzzle(
	ctx context.Context,
	req *connect.Request[playv1.ArchivePuzzleRequest],
) (*connect.Response[playv1.ArchivePuzzleResponse], error) {
	m, err := requireMaster(ctx, req.Msg.GetCampaignId())
	if err != nil {
		return nil, err
	}
	p, err := s.setArchived(ctx, m.CampaignID, req.Msg.GetPuzzleId(), true)
	if err != nil {
		return nil, err
	}
	return connect.NewResponse(&playv1.ArchivePuzzleResponse{Puzzle: p}), nil
}

// UnarchivePuzzle implements playv1connect.PuzzleServiceHandler.
func (s *Service) UnarchivePuzzle(
	ctx context.Context,
	req *connect.Request[playv1.UnarchivePuzzleRequest],
) (*connect.Response[playv1.UnarchivePuzzleResponse], error) {
	m, err := requireMaster(ctx, req.Msg.GetCampaignId())
	if err != nil {
		return nil, err
	}
	p, err := s.setArchived(ctx, m.CampaignID, req.Msg.GetPuzzleId(), false)
	if err != nil {
		return nil, err
	}
	return connect.NewResponse(&playv1.UnarchivePuzzleResponse{Puzzle: p}), nil
}
