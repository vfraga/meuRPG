package maps

import (
	"context"
	"errors"
	"fmt"
	"slices"

	"connectrpc.com/connect"
	"github.com/jackc/pgx/v5"

	charactersv1 "github.com/PuraFome/meuRPG/backend/gen/meurpg/characters/v1"
	mapsv1 "github.com/PuraFome/meuRPG/backend/gen/meurpg/maps/v1"
	"github.com/PuraFome/meuRPG/backend/internal/authz"
	"github.com/PuraFome/meuRPG/backend/internal/maps/mapsdb"
	"github.com/PuraFome/meuRPG/backend/internal/platform/conv"
	"github.com/PuraFome/meuRPG/backend/internal/platform/sessiontell"
	"github.com/PuraFome/meuRPG/backend/internal/platform/uid"
	"github.com/PuraFome/meuRPG/backend/internal/rules/grid"
)

// Tokens outside combat (plan B, B2): the master moves tokens freely, alone or as a group, and puts
// the party on the map; a player moves their own character's token when the master allows it on the
// map. A move walks the straight line and stops before a wall, a locked door or a square the master
// marked "can't pass"; the answer says how far each token got and why it stopped. While a combat is
// on the map the tokens move through the combat instead.

// maxMovedTokens bounds a move or a placement of a group: a combat has at most 40 combatants.
const maxMovedTokens = 40

// tokenAsk is a move of tokens the caller asked for, read and checked.
type tokenAsk struct {
	m   authz.Membership
	v   viewer
	id  string // the map
	ids []string
	x   int32
	y   int32
}

// characterIDs checks a list of characters: 1 to 40 UUIDs with no repeat.
func characterIDs(raw []string) ([]string, error) {
	if len(raw) == 0 || len(raw) > maxMovedTokens {
		return nil, connect.NewError(connect.CodeInvalidArgument, fmt.Errorf("character_ids must have 1 to %d characters", maxMovedTokens))
	}
	ids := make([]string, 0, len(raw))
	for _, r := range raw {
		id, ok := uid.Canonical(r)
		if !ok || slices.Contains(ids, id) {
			return nil, connect.NewError(connect.CodeInvalidArgument, errors.New("character_ids must be different UUIDs"))
		}
		ids = append(ids, id)
	}
	return ids, nil
}

// livingCharacters finds the living characters of the campaign in the order asked, or answers
// `not_found` when any is missing.
func (s *Service) livingCharacters(ctx context.Context, campaignID string, ids []string) ([]*charactersv1.CharacterSummary, error) {
	found, err := s.characters.MapCharacters(ctx, nil, campaignID, ids)
	if err != nil {
		return nil, s.dbError(ctx, "find the characters", err)
	}
	out := make([]*charactersv1.CharacterSummary, 0, len(ids))
	for _, id := range ids {
		i := slices.IndexFunc(found, func(c *charactersv1.CharacterSummary) bool { return c.GetId() == id })
		if i < 0 {
			return nil, errCharacterNotFound()
		}
		out = append(out, found[i])
	}
	return out, nil
}

// SetMapPlayersMove implements mapsv1connect.MapServiceHandler.
func (s *Service) SetMapPlayersMove(
	ctx context.Context,
	req *connect.Request[mapsv1.SetMapPlayersMoveRequest],
) (*connect.Response[mapsv1.SetMapPlayersMoveResponse], error) {
	m, err := authz.RequireCampaignRole(ctx, req.Msg.GetCampaignId(), authz.RoleMaster)
	if err != nil {
		return nil, err
	}
	mapID, ok := uid.Canonical(req.Msg.GetMapId())
	if !ok {
		return nil, errMapNotFound()
	}
	current, err := s.currentMap(ctx, m.CampaignID)
	if err != nil {
		return nil, err
	}
	var row mapsdb.Map
	err = s.inSession(ctx, m.CampaignID, func(tx pgx.Tx, t sessiontell.Tell) error {
		q := s.queries.WithTx(tx)
		var err error
		if row, err = lockMap(ctx, q, m.CampaignID, mapID); err != nil {
			return err
		}
		if row.PlayersMoveTokens == req.Msg.GetEnabled() {
			t.Replay() // nothing to write: the notice goes out again with the number as it stands
		} else {
			if row, err = q.SetMapPlayersMove(ctx, mapsdb.SetMapPlayersMoveParams{CampaignID: m.CampaignID, ID: mapID, Enabled: req.Msg.GetEnabled()}); err != nil {
				return fmt.Errorf("set whether the players move: %w", err)
			}
			t.Touch()
		}
		// The players read the setting in the map: a notice, to the ones who see the map.
		t.After(func(ctx context.Context, out sessiontell.Out) {
			s.publishMapChanged(withOut(ctx, out), m.CampaignID, mapID, playersSee(mapID, row.RevealedAt, current))
		})
		return nil
	})
	if err != nil {
		return nil, s.dbError(ctx, "set whether the players move their tokens", err)
	}
	res, err := s.masterMap(ctx, m, mapID)
	if err != nil {
		return nil, err
	}
	return connect.NewResponse(&mapsv1.SetMapPlayersMoveResponse{Map: res}), nil
}

// moveOutcome is what a committed move leaves for the answer and the stream.
type moveOutcome struct {
	row         mapsdb.Map
	g           grid.Grid
	chars       []*charactersv1.CharacterSummary
	before      []mapsdb.MapToken
	after       []mapsdb.MapToken
	plan        movePlan
	doorsOpened bool
}

// changed says whether the move wrote anything: a token left its square, or a door opened.
func (o moveOutcome) changed() bool {
	if o.doorsOpened {
		return true
	}
	for i := range o.after {
		if o.before[i].XBp != o.after[i].XBp || o.before[i].YBp != o.after[i].YBp {
			return true
		}
	}
	return false
}

// MoveMapTokens implements mapsv1connect.MapServiceHandler.
func (s *Service) MoveMapTokens(
	ctx context.Context,
	req *connect.Request[mapsv1.MoveMapTokensRequest],
) (*connect.Response[mapsv1.MoveMapTokensResponse], error) {
	ask, chars, err := s.readMoveAsk(ctx, req.Msg)
	if err != nil {
		return nil, err
	}
	var out moveOutcome
	err = s.inSession(ctx, ask.m.CampaignID, func(tx pgx.Tx, t sessiontell.Tell) error {
		var err error
		if out, err = s.moveTx(ctx, tx, ask, chars); err != nil {
			return err
		}
		if !out.changed() {
			return nil
		}
		s.announce(t, func(ctx context.Context) { s.announceMove(ctx, ask, out) })
		// What the characters that changed square see from where they stand is part of this write: the move moves the number once.
		var landed []landedToken
		for i, c := range out.chars {
			if out.before[i].XBp != out.after[i].XBp || out.before[i].YBp != out.after[i].YBp {
				landed = append(landed, landedToken{token: out.after[i], character: c})
			}
		}
		return s.noticeLanded(ctx, tx, t, ask.m.CampaignID, ask.id, out.row, landed)
	})
	if err != nil {
		return nil, s.dbError(ctx, "move tokens", err)
	}
	res, err := s.moveAnswer(ctx, ask, out)
	if err != nil {
		return nil, err
	}
	return connect.NewResponse(res), nil
}

// readMoveAsk reads who moves what: the master moves any living characters; a player only their own
// character, alone. A player who names anyone else is refused whatever exists (RN-10).
func (s *Service) readMoveAsk(ctx context.Context, msg *mapsv1.MoveMapTokensRequest) (tokenAsk, []*charactersv1.CharacterSummary, error) {
	m, err := authz.RequireCampaignMember(ctx, msg.GetCampaignId())
	if err != nil {
		return tokenAsk{}, nil, err
	}
	mapID, ok := uid.Canonical(msg.GetMapId())
	if !ok {
		return tokenAsk{}, nil, errMapNotFound()
	}
	if err := checkPosition(msg.GetXBp(), msg.GetYBp()); err != nil {
		return tokenAsk{}, nil, err
	}
	ids, err := characterIDs(msg.GetCharacterIds())
	if err != nil {
		return tokenAsk{}, nil, err
	}
	v, err := s.viewerOf(ctx, m)
	if err != nil {
		return tokenAsk{}, nil, err
	}
	ask := tokenAsk{m: m, v: v, id: mapID, ids: ids, x: msg.GetXBp(), y: msg.GetYBp()}
	if !v.master && len(ids) != 1 {
		return tokenAsk{}, nil, errNotYourToken()
	}
	chars, err := s.livingCharacters(ctx, m.CampaignID, ids)
	if err != nil {
		if !v.master {
			return tokenAsk{}, nil, errNotYourToken()
		}
		return tokenAsk{}, nil, err
	}
	if !v.master && chars[0].GetPlayerUserId() != m.UserID {
		return tokenAsk{}, nil, errNotYourToken()
	}
	return ask, chars, nil
}

func errNotYourToken() error {
	return connect.NewError(connect.CodePermissionDenied, errors.New("a player moves only their own character's token, when the master allows it"))
}

// moveTx runs the move in one transaction: it locks the map, checks that nothing forbids a move
// there (a combat, or a player's setting), plans the move on the terrain and writes the tokens, and
// opens the closed doors the tokens walked through.
func (s *Service) moveTx(ctx context.Context, tx pgx.Tx, ask tokenAsk, chars []*charactersv1.CharacterSummary) (moveOutcome, error) {
	q := s.queries.WithTx(tx)
	out := moveOutcome{chars: chars}
	var err error
	if out.row, err = lockMap(ctx, q, ask.m.CampaignID, ask.id); err != nil {
		return out, err
	}
	if err := s.checkOutOfCombat(ctx, tx, ask, out.row); err != nil {
		return out, err
	}
	if out.before, err = lockTokens(ctx, q, ask, chars); err != nil {
		return out, err
	}
	size, err := q.GetMapGrid(ctx, mapsdb.GetMapGridParams{CampaignID: ask.m.CampaignID, ID: ask.id})
	if err != nil {
		return out, fmt.Errorf("read the map's grid: %w", err)
	}
	out.g = gridOf(size.GridColumns, size.GridFactor, size.ImageWidth, size.ImageHeight)
	set, err := s.layersOf(ctx, q, ask.id, out.g)
	if err != nil {
		return out, err
	}
	if out.plan, err = planTokens(out.g, set, out.before, ask); err != nil {
		return out, err
	}
	if out.after, err = s.writeMoves(ctx, q, ask, out); err != nil {
		return out, err
	}
	out.doorsOpened, err = s.openWalkedDoors(ctx, q, ask.id, out.g, set, out.plan, max(int(size.GridFactor), 1))
	return out, err
}

// checkOutOfCombat refuses a move while a combat runs on the map, and a player's move that the
// master did not allow or on a map the player does not see.
func (s *Service) checkOutOfCombat(ctx context.Context, tx pgx.Tx, ask tokenAsk, row mapsdb.Map) error {
	if !ask.v.master {
		if !playersSee(row.ID, row.RevealedAt, ask.v.currentMap) {
			return errMapNotFound() // a hidden map is not found to a player (RN-10)
		}
		if !row.PlayersMoveTokens {
			return errNotYourToken()
		}
	}
	running, err := s.combats.CombatRunsOnMap(ctx, tx, ask.m.CampaignID, ask.id)
	if err != nil {
		return fmt.Errorf("find out whether a combat runs: %w", err)
	}
	if running {
		return errMapBlocked(mapsv1.MapBlockedReason_MAP_BLOCKED_REASON_COMBAT_RUNNING, "a combat is on the map: the tokens move through the combat")
	}
	return nil
}

// lockTokens locks the tokens of the characters in the order asked. A character with no token on the
// map is not found, and so is a hidden token for a player.
func lockTokens(ctx context.Context, q *mapsdb.Queries, ask tokenAsk, chars []*charactersv1.CharacterSummary) ([]mapsdb.MapToken, error) {
	out := make([]mapsdb.MapToken, 0, len(chars))
	for _, c := range chars {
		t, err := q.GetMapTokenForUpdate(ctx, mapsdb.GetMapTokenForUpdateParams{MapID: ask.id, CharacterID: c.GetId()})
		if errors.Is(err, pgx.ErrNoRows) || (err == nil && t.Hidden && !ask.v.master) {
			return nil, errTokenNotFound()
		}
		if err != nil {
			return nil, fmt.Errorf("find token: %w", err)
		}
		out = append(out, t)
	}
	return out, nil
}

// layersOf reads a map's layers for its grid: all empty when none was painted.
func (s *Service) layersOf(ctx context.Context, q *mapsdb.Queries, mapID string, g grid.Grid) (layerSet, error) {
	if !g.Valid() {
		return layerSet{}, nil
	}
	stored, err := q.GetMapLayers(ctx, mapID)
	if err != nil && !errors.Is(err, pgx.ErrNoRows) {
		return layerSet{}, fmt.Errorf("read the layers: %w", err)
	}
	return loadLayers(stored, g), nil
}

// planTokens plans the move of the locked tokens on the map's terrain. On a map with no grid there
// are no squares and no layers: the tokens keep their offset from the anchor and nothing stops them.
func planTokens(g grid.Grid, set layerSet, tokens []mapsdb.MapToken, ask tokenAsk) (movePlan, error) {
	if !g.Valid() {
		return planFree(tokens, ask), nil
	}
	t := grid.Terrain{Grid: g, Walls: set.walls, Difficult: set.terrain, Cover: set.cover, Doors: set.doors, NoPass: set.noPass}
	starts := make([]grid.Square, len(tokens))
	for i, tk := range tokens {
		starts[i] = g.SquareOf(int(tk.XBp), int(tk.YBp))
	}
	return planMove(t, starts, g.SquareOf(int(ask.x), int(ask.y))), nil
}

// planFree is the plan of a map with no grid: no square to count, nothing to stop a token.
func planFree(tokens []mapsdb.MapToken, ask tokenAsk) movePlan {
	plan := movePlan{steps: make([]tokenStep, len(tokens)), blocked: -1}
	dx, dy := ask.x-tokens[0].XBp, ask.y-tokens[0].YBp
	for i, tk := range tokens {
		plan.moved = plan.moved || dx != 0 || dy != 0
		plan.steps[i] = tokenStep{freeAt: &[2]int32{clampBp(tk.XBp + dx), clampBp(tk.YBp + dy)}}
	}
	return plan
}

// clampBp keeps a position in the image.
func clampBp(n int32) int32 { return min(max(n, 0), maxPosition) }

// writeMoves writes the tokens that changed square, each to the center of the square it reached (to the
// point itself on a map with no grid), and returns the tokens as they are.
func (s *Service) writeMoves(ctx context.Context, q *mapsdb.Queries, ask tokenAsk, out moveOutcome) ([]mapsdb.MapToken, error) {
	after := slices.Clone(out.before)
	for i, step := range out.plan.steps {
		x, y, moved := stepPosition(out.g, out.before[i], step)
		if !moved {
			continue
		}
		t, err := q.MoveMapToken(ctx, mapsdb.MoveMapTokenParams{MapID: ask.id, CharacterID: out.before[i].CharacterID, XBp: x, YBp: y, UpdatedAt: s.now()})
		if err != nil {
			return nil, fmt.Errorf("move token: %w", err)
		}
		after[i] = t
	}
	return after, nil
}

// stepPosition is where a token ends in base points, and whether that is another place.
func stepPosition(g grid.Grid, was mapsdb.MapToken, step tokenStep) (x, y int32, moved bool) {
	if step.freeAt != nil {
		x, y = step.freeAt[0], step.freeAt[1]
		return x, y, x != was.XBp || y != was.YBp
	}
	if g.SquareOf(int(was.XBp), int(was.YBp)) == step.reached {
		return was.XBp, was.YBp, false // it stayed in its square: keep where it stood in it
	}
	cx, cy := g.CenterOf(step.reached)
	return conv.Int32(cx), conv.Int32(cy), true
}

// openWalkedDoors opens, in the layers, the closed doors the tokens walked through, and says whether
// it opened any (the screens then read the layers again).
func (s *Service) openWalkedDoors(ctx context.Context, q *mapsdb.Queries, mapID string, g grid.Grid, set layerSet, plan movePlan, factor int) (bool, error) {
	var squares []grid.Square
	for _, st := range plan.steps {
		squares = append(squares, st.opens...)
	}
	if len(squares) == 0 || !g.Valid() {
		return false, nil
	}
	if len(openClosedDoors(set, squares, factor)) == 0 {
		return false, nil
	}
	if err := storeLayers(ctx, q, mapID, set, s.now()); err != nil {
		return false, err
	}
	if _, err := q.BumpMapLayersRevision(ctx, mapID); err != nil {
		return false, fmt.Errorf("bump the layers' revision: %w", err)
	}
	return true, nil
}

// announceMove tells the watchers of every token that moved, lets a trap react to it, and tells them
// of the doors it opened. It runs after the commit and never fails the move.
func (s *Service) announceMove(ctx context.Context, ask tokenAsk, out moveOutcome) {
	for i, c := range out.chars {
		if out.before[i].XBp == out.after[i].XBp && out.before[i].YBp == out.after[i].YBp {
			continue
		}
		before := out.before[i]
		s.publishTokenWritten(ctx, ask.m.CampaignID, out.row, ask.v.currentMap, &before, &out.after[i], isPlayerCharacter(c), true)
		s.tokenLanded(ctx, ask.m.CampaignID, ask.m.UserID, ask.id, out.after[i], c)
	}
	if out.doorsOpened {
		s.layersChanged(ctx, ask.m.CampaignID, out.row, true, true)
	}
}

// moveAnswer builds the answer as the caller reads it. A player's names only their own token and, on a
// fog map, says why it stopped only when the square in the way is one they see or remember (RN-10).
func (s *Service) moveAnswer(ctx context.Context, ask tokenAsk, out moveOutcome) (*mapsv1.MoveMapTokensResponse, error) {
	res := &mapsv1.MoveMapTokensResponse{Moved: out.plan.moved}
	if out.plan.blocked >= 0 {
		res.BlockedCharacterId = out.chars[out.plan.blocked].GetId()
	}
	knows, err := s.squareKnower(ctx, ask)
	if err != nil {
		return nil, err
	}
	for i, step := range out.plan.steps {
		res.Moves = append(res.Moves, &mapsv1.MapTokenMove{
			Token:        tokenToProto(out.after[i], out.chars[i], ask.v),
			SquaresAsked: conv.Int32(step.asked), SquaresMoved: conv.Int32(step.moved),
			StoppedBy: stopOf(step, out.g, ask, knows),
		})
	}
	return res, nil
}

// squareKnower says whether the caller knows a square: the master and a player of a map with no fog know
// every square, a player of a fog map the ones they see now or remember.
func (s *Service) squareKnower(ctx context.Context, ask tokenAsk) (func(grid.Square) bool, error) {
	all := func(grid.Square) bool { return true }
	if ask.v.master {
		return all, nil
	}
	_, _, pv, err := s.visionOf(ctx, ask.m, ask.id, "")
	if err != nil {
		var ce *connect.Error
		if errors.As(err, &ce) && connect.CodeOf(err) == connect.CodeFailedPrecondition {
			return all, nil // no grid, no fog
		}
		return nil, err
	}
	return pv.terrainKnown, nil
}

// stopOf is the reason a token stopped, as the caller may read it: the square in the way is the next one
// on the line past where the token ended, and a player who does not know it learns only that it stopped.
func stopOf(step tokenStep, _ grid.Grid, ask tokenAsk, knows func(grid.Square) bool) mapsv1.MapMoveStop {
	if step.stop == grid.StopNone {
		return mapsv1.MapMoveStop_MAP_MOVE_STOP_UNSPECIFIED
	}
	if !ask.v.master && !knows(step.inTheWay) {
		return mapsv1.MapMoveStop_MAP_MOVE_STOP_BLOCKED
	}
	switch step.stop {
	case grid.StopNoPass:
		return mapsv1.MapMoveStop_MAP_MOVE_STOP_NO_PASS
	case grid.StopLocked:
		return mapsv1.MapMoveStop_MAP_MOVE_STOP_LOCKED
	}
	return mapsv1.MapMoveStop_MAP_MOVE_STOP_WALL
}
