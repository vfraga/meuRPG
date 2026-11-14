package play

import (
	"context"
	"errors"
	"fmt"
	"math"
	"slices"

	"connectrpc.com/connect"

	playv1 "github.com/PuraFome/meuRPG/backend/gen/meurpg/play/v1"
	"github.com/PuraFome/meuRPG/backend/internal/platform/idem"
	"github.com/PuraFome/meuRPG/backend/internal/play/playdb"
	"github.com/PuraFome/meuRPG/backend/internal/rules/grid"
)

// The sunlight of a combat (RN-20, RN-29) is the master's switch: a creature with Sunlight Sensitivity or Weakness
// rolls at a disadvantage while it stands in it (sunlight.go). The state is the master's alone: a player reads
// none of it, only the roll's result, so the write goes to the master's screen and its event is secret.

// SetSunlight implements playv1connect.CombatServiceHandler.
func (s *Service) SetSunlight(
	ctx context.Context,
	req *connect.Request[playv1.SetSunlightRequest],
) (*connect.Response[playv1.SetSunlightResponse], error) {
	m, key, encID, err := masterCall(ctx, req.Msg.GetCampaignId(), req.Msg.GetIdempotencyKey(), req.Msg.GetEncounterId())
	if err != nil {
		return nil, err
	}
	res, err := s.write(ctx, combatWrite{m: m, key: key, hash: idem.Hash(req.Msg), kind: eventSunlightSet, encounterID: encID}, func(c *combatTx) (any, error) {
		return sunlightWrite(ctx, c, req.Msg)
	})
	if err != nil {
		return nil, s.dbError(ctx, "set the sunlight", err)
	}
	out, err := s.finish(ctx, m, res, func(_ context.Context, d *encounterData) { s.publishEncounterChangedToMaster(ctx, m.CampaignID, d.enc) })
	if err != nil {
		return nil, err
	}
	return connect.NewResponse(&playv1.SetSunlightResponse{Encounter: out}), nil
}

// sunlightWrite is the change of SetSunlight. Turning the sunlight off keeps the scope and the squares.
func sunlightWrite(ctx context.Context, c *combatTx, req *playv1.SetSunlightRequest) (any, error) {
	if err := notEnded(c.enc); err != nil {
		return nil, err
	}
	scope, squares, err := nextSunlight(c.enc, req)
	if err != nil {
		return nil, err
	}
	enc, err := c.q.SetEncounterSunlight(ctx, playdb.SetEncounterSunlightParams{
		ID: c.enc.ID, SunlightOn: req.GetOn(), SunlightScope: scope, SunlightSquares: squaresToJSON(squares),
	})
	if err != nil {
		return nil, fmt.Errorf("set the encounter's sunlight: %w", err)
	}
	c.enc = enc
	return actionEvent{Round: enc.Round, Secret: true, SunlightOn: enc.SunlightOn, SunlightScope: enc.SunlightScope, SunlightSquares: len(squares)}, nil
}

// nextSunlight is the scope and the sorted squares the combat keeps after the request, or the refusal.
func nextSunlight(enc playdb.Encounter, req *playv1.SetSunlightRequest) (string, []grid.Square, error) {
	scope, err := sunScopeOf(enc, req.GetScope())
	if err != nil {
		return "", nil, err
	}
	adds, removes := req.GetAddSquares(), req.GetRemoveSquares()
	if len(adds) > maxSunSquares || len(removes) > maxSunSquares {
		return "", nil, sunInvalid("too many squares")
	}
	if isTheatre(enc) || enc.GridColumns <= 0 || enc.GridRows <= 0 { // no grid, no squares to light: the sun reaches the whole combat (a grid whose map was deleted keeps its squares)
		if scope == sunScopeSquares && req.GetScope() != playv1.SunlightScope_SUNLIGHT_SCOPE_UNSPECIFIED {
			return "", nil, sunInvalid("a combat with no grid has no squares: the sunlight reaches all of it")
		}
		if len(adds)+len(removes) > 0 {
			return "", nil, sunInvalid("a combat with no grid has no squares")
		}
		return sunScopeMap, nil, nil
	}
	out, err := editedSunSquares(enc, adds, removes)
	return scope, out, err
}

func sunInvalid(msg string) error {
	return connect.NewError(connect.CodeInvalidArgument, errors.New(msg))
}

// sunScopeOf is the scope the request asks for; an unspecified one keeps the combat's.
func sunScopeOf(enc playdb.Encounter, ask playv1.SunlightScope) (string, error) {
	switch ask {
	case playv1.SunlightScope_SUNLIGHT_SCOPE_UNSPECIFIED:
		return enc.SunlightScope, nil
	case playv1.SunlightScope_SUNLIGHT_SCOPE_MAP:
		return sunScopeMap, nil
	case playv1.SunlightScope_SUNLIGHT_SCOPE_SQUARES:
		return sunScopeSquares, nil
	}
	return "", sunInvalid("unknown sunlight scope")
}

// editedSunSquares is the combat's sunlit squares with the removals and then the additions applied, sorted by row then column.
func editedSunSquares(enc playdb.Encounter, adds, removes []*playv1.SunlightSquare) ([]grid.Square, error) {
	set := squareSet(squaresOfJSON(enc.SunlightSquares))
	for _, sq := range removes {
		g, err := sunSquareOf(enc, sq)
		if err != nil {
			return nil, err
		}
		delete(set, g)
	}
	for _, sq := range adds {
		g, err := sunSquareOf(enc, sq)
		if err != nil {
			return nil, err
		}
		set[g] = true
	}
	if len(set) > maxSunSquares {
		return nil, sunInvalid("the sunlit squares pass the limit")
	}
	out := make([]grid.Square, 0, len(set))
	for sq := range set {
		out = append(out, sq)
	}
	slices.SortFunc(out, func(a, b grid.Square) int {
		if a.Row != b.Row {
			return a.Row - b.Row
		}
		return a.Col - b.Col
	})
	return out, nil
}

// sunSquareOf checks the square is inside the combat's grid.
func sunSquareOf(enc playdb.Encounter, sq *playv1.SunlightSquare) (grid.Square, error) {
	if sq.GetCol() < 0 || sq.GetRow() < 0 || sq.GetCol() >= enc.GridColumns || sq.GetRow() >= enc.GridRows {
		return grid.Square{}, connect.NewError(connect.CodeInvalidArgument, errors.New("a square is off the grid"))
	}
	return grid.Square{Col: int(sq.GetCol()), Row: int(sq.GetRow())}, nil
}

// sunlightView sets the sunlight on the master's view; a player reads none of it (RN-20, RN-29).
func (d *encounterData) sunlightView(v combatViewer, out *playv1.Encounter) {
	if !v.master {
		return
	}
	out.SunlightOn = d.enc.SunlightOn
	switch d.enc.SunlightScope {
	case sunScopeMap:
		out.SunlightScope = playv1.SunlightScope_SUNLIGHT_SCOPE_MAP
	case sunScopeSquares:
		out.SunlightScope = playv1.SunlightScope_SUNLIGHT_SCOPE_SQUARES
	}
	for _, sq := range squaresOfJSON(d.enc.SunlightSquares) {
		out.SunlightSquares = append(out.SunlightSquares, &playv1.SunlightSquare{Col: clamp32(sq.Col, 0, math.MaxInt32), Row: clamp32(sq.Row, 0, math.MaxInt32)})
	}
}
