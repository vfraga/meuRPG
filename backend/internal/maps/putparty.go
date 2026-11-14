package maps

import (
	"context"
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

// formationOf is the shape of a request's formation; the default is the block.
func formationOf(f mapsv1.MapFormation) grid.Formation {
	switch f {
	case mapsv1.MapFormation_MAP_FORMATION_LINE:
		return grid.FormationLine
	case mapsv1.MapFormation_MAP_FORMATION_ARC:
		return grid.FormationArc
	}
	return grid.FormationBlock
}

// partyPut is what a committed PutPartyOnMap leaves for the answer and the stream.
type partyPut struct {
	row         mapsdb.Map
	chars       []*charactersv1.CharacterSummary
	was, tokens []mapsdb.MapToken
	placed      []bool
	forRoom     int
}

// PutPartyOnMap implements mapsv1connect.MapServiceHandler.
func (s *Service) PutPartyOnMap(
	ctx context.Context,
	req *connect.Request[mapsv1.PutPartyOnMapRequest],
) (*connect.Response[mapsv1.PutPartyOnMapResponse], error) {
	m, err := authz.RequireCampaignRole(ctx, req.Msg.GetCampaignId(), authz.RoleMaster)
	if err != nil {
		return nil, err
	}
	mapID, ok := uid.Canonical(req.Msg.GetMapId())
	if !ok {
		return nil, errMapNotFound()
	}
	if err := checkPosition(req.Msg.GetXBp(), req.Msg.GetYBp()); err != nil {
		return nil, err
	}
	ids, err := characterIDs(req.Msg.GetCharacterIds())
	if err != nil {
		return nil, err
	}
	chars, err := s.livingCharacters(ctx, m.CampaignID, ids)
	if err != nil {
		return nil, err
	}
	current, err := s.currentMap(ctx, m.CampaignID)
	if err != nil {
		return nil, err
	}
	var put partyPut
	err = s.inSession(ctx, m.CampaignID, func(tx pgx.Tx, t sessiontell.Tell) error {
		var err error
		if put, err = s.putPartyTx(ctx, tx, m, mapID, chars, req.Msg); err != nil {
			return err
		}
		s.announce(t, func(ctx context.Context) { s.announcePut(ctx, m, mapID, current, put) })
		landed := make([]landedToken, len(put.chars))
		for i, c := range put.chars {
			landed[i] = landedToken{token: put.tokens[i], character: c}
		}
		// What the characters see from where they landed is part of this write: the formation moves the number once.
		return s.noticeLanded(ctx, tx, t, m.CampaignID, mapID, put.row, landed)
	})
	if err != nil {
		return nil, s.dbError(ctx, "put the party on the map", err)
	}
	v := newViewer(m, current)
	res := &mapsv1.PutPartyOnMapResponse{MovedForRoom: conv.Int32(put.forRoom)}
	for i, c := range put.chars {
		res.Tokens = append(res.Tokens, tokenToProto(put.tokens[i], c, v))
	}
	return connect.NewResponse(res), nil
}

// announcePut tells the watchers of each token the party's formation wrote, and lets a trap react to it.
func (s *Service) announcePut(ctx context.Context, m authz.Membership, mapID, current string, put partyPut) {
	for i, c := range put.chars {
		before := &put.was[i]
		if put.placed[i] {
			before = nil
		}
		s.publishTokenWritten(ctx, m.CampaignID, put.row, current, before, &put.tokens[i], isPlayerCharacter(c), !put.placed[i])
		s.tokenLanded(ctx, m.CampaignID, m.UserID, mapID, put.tokens[i], c)
	}
}

// putPartyTx chooses the squares of the formation and writes the tokens, in one transaction.
func (s *Service) putPartyTx(ctx context.Context, tx pgx.Tx, m authz.Membership, mapID string, chars []*charactersv1.CharacterSummary, msg *mapsv1.PutPartyOnMapRequest) (partyPut, error) {
	q := s.queries.WithTx(tx)
	put := partyPut{chars: chars}
	var err error
	if put.row, err = lockMap(ctx, q, m.CampaignID, mapID); err != nil {
		return put, err
	}
	running, err := s.combats.CombatRunsOnMap(ctx, tx, m.CampaignID, mapID)
	if err != nil {
		return put, fmt.Errorf("find out whether a combat runs: %w", err)
	}
	if running {
		return put, errMapBlocked(mapsv1.MapBlockedReason_MAP_BLOCKED_REASON_COMBAT_RUNNING, "a combat is on the map: the tokens move through the combat")
	}
	size, err := q.GetMapGrid(ctx, mapsdb.GetMapGridParams{CampaignID: m.CampaignID, ID: mapID})
	if err != nil {
		return put, fmt.Errorf("read the map's grid: %w", err)
	}
	g := gridOf(size.GridColumns, size.GridFactor, size.ImageWidth, size.ImageHeight)
	if !g.Valid() {
		return put, errNoGrid()
	}
	squares, forRoom, err := s.partySquares(ctx, q, mapID, g, chars, msg)
	if err != nil {
		return put, err
	}
	put.forRoom = forRoom
	for i, c := range chars {
		x, y := g.CenterOf(squares[i])
		was, token, placed, err := s.putToken(ctx, q, mapID, conv.Int32(x), conv.Int32(y), c)
		if err != nil {
			return put, err
		}
		put.was, put.tokens, put.placed = append(put.was, was), append(put.tokens, token), append(put.placed, placed)
	}
	return put, nil
}

// partySquares chooses a square for each character: the formation's, or the nearest free one, never a
// wall, a can't-pass square or one another token (not of the party being put) holds. It says how many did
// not get their own place, and answers NO_ROOM when the map has too few free squares.
func (s *Service) partySquares(ctx context.Context, q *mapsdb.Queries, mapID string, g grid.Grid, chars []*charactersv1.CharacterSummary, msg *mapsv1.PutPartyOnMapRequest) ([]grid.Square, int, error) {
	set, err := s.layersOf(ctx, q, mapID, g)
	if err != nil {
		return nil, 0, err
	}
	t := grid.Terrain{Grid: g, Walls: set.walls, Difficult: set.terrain, Cover: set.cover, Doors: set.doors, NoPass: set.noPass}
	tokens, err := q.ListMapTokens(ctx, mapID)
	if err != nil {
		return nil, 0, fmt.Errorf("list the tokens: %w", err)
	}
	var taken grid.Squares
	for _, tk := range tokens {
		if !slices.ContainsFunc(chars, func(c *charactersv1.CharacterSummary) bool { return c.GetId() == tk.CharacterID }) {
			taken = append(taken, g.SquareOf(int(tk.XBp), int(tk.YBp)))
		}
	}
	anchor := g.SquareOf(int(msg.GetXBp()), int(msg.GetYBp()))
	f := formationOf(msg.GetFormation())
	squares := grid.Arrange(t, anchor, len(chars), f, taken)
	if len(squares) < len(chars) {
		return nil, 0, errMapBlocked(mapsv1.MapBlockedReason_MAP_BLOCKED_REASON_NO_ROOM, "the map has no room for the whole group")
	}
	return squares, countMoved(grid.FormationSquares(anchor, len(chars), f), squares), nil
}

// countMoved is how many squares differ from the formation's own.
func countMoved(ideal, got []grid.Square) int {
	n := 0
	for i := range got {
		if got[i] != ideal[i] {
			n++
		}
	}
	return n
}
