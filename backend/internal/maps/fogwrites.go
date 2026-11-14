package maps

import (
	"context"
	"slices"

	charactersv1 "github.com/PuraFome/meuRPG/backend/gen/meurpg/characters/v1"
	mapsv1 "github.com/PuraFome/meuRPG/backend/gen/meurpg/maps/v1"
	"github.com/PuraFome/meuRPG/backend/internal/maps/mapsdb"
	"github.com/PuraFome/meuRPG/backend/internal/rules/grid"
)

// Who hears of a write on a map with the fog of war on (MR-036, RN-10, D6). On a
// map without fog nothing here changes: the existing rules (visibility.go) say
// who hears. With the fog on, a hint that says "a point changed" or "a token
// moved" is itself something a player must not learn about a place they do not
// see, so:
//
//   - a change to a point reaches (`map_changed`) only the players who see or
//     remember one of the point's squares;
//   - an NPC token's placing, moving, hiding or removal reaches a player only as
//     `vision_changed`, and only when a square the token was on or is on is seen
//     by them now; never as `token_moved`, which carries the square;
//   - a player character's token is no secret from the party: its move goes to
//     everyone as before;
//   - after a change to what someone sees (a token, a carried light, a Luz point,
//     the walls and the light), refreshVision remembers what the players see and
//     tells the ones whose view changed.

// playerViews is what each player of a fog map knows now, by user: for the
// hints that go only to the players who know a place.
func (s *Service) playerViews(ctx context.Context, campaignID string, mapRow mapsdb.Map) (map[string]*playerView, error) {
	size, err := s.queries.GetMapGrid(ctx, mapsdb.GetMapGridParams{CampaignID: campaignID, ID: mapRow.ID})
	if err != nil {
		return nil, err
	}
	in := fogInputOfRow(mapRow, size.ImageWidth, size.ImageHeight)
	points, err := s.queries.ListMapPoints(ctx, mapRow.ID)
	if err != nil {
		return nil, err
	}
	tokens, err := s.queries.ListMapTokens(ctx, mapRow.ID)
	if err != nil {
		return nil, err
	}
	sg, err := s.newSight(ctx, nil, in, points, tokens)
	if err != nil {
		return nil, err
	}
	stored, err := s.queries.ListMapVisionMemory(ctx, mapRow.ID)
	if err != nil {
		return nil, err
	}
	memories := make(map[string][]byte, len(stored))
	var rememberers []string
	for _, m := range stored {
		rememberers = append(rememberers, m.UserID)
		if m.Epoch == in.epoch {
			memories[m.UserID] = m.Seen
		}
	}
	out := map[string]*playerView{}
	for _, userID := range sg.users(rememberers...) {
		memory := grid.NewLayer(in.g)
		if seen, ok := memories[userID]; ok {
			memory = decodeMemory(in.g, seen)
		}
		out[userID] = sg.newView(userID, memory)
	}
	return out, nil
}

// publishPointsChanged tells the watching members that points of a map changed:
// the master always, and, when players is true, the players who see the points
// (the ones that gives: a point's change reaches only a player who sees or
// remembers a square of it on a fog map). The points are the rows before and
// after the change. A change to a Luz point also changes what is seen, so the
// view is worked out again.
func (s *Service) publishPointsChanged(ctx context.Context, campaignID string, mapRow mapsdb.Map, players bool, points ...mapsdb.MapPoint) {
	if !fogged(mapRow) {
		s.publishMapChanged(ctx, campaignID, mapRow.ID, players)
		return
	}
	s.publishMapChanged(ctx, campaignID, mapRow.ID, false)
	if players {
		s.tellWhoKnows(ctx, campaignID, mapRow, func(pv *playerView) bool {
			return slices.ContainsFunc(points, func(p mapsdb.MapPoint) bool { return s.pointKnown(pv, p) })
		})
	}
	light := kindToDB[mapsv1.MapPointKind_MAP_POINT_KIND_LIGHT]
	if slices.ContainsFunc(points, func(p mapsdb.MapPoint) bool { return p.Kind == light }) {
		s.refreshVision(ctx, campaignID, mapRow.ID)
	}
}

// tellWhoKnows sends `map_changed` to the players of a fog map whose view
// satisfies the test. A failure to work it out is logged and nobody is told: the
// app reads the map again after any reconnection.
func (s *Service) tellWhoKnows(ctx context.Context, campaignID string, mapRow mapsdb.Map, test func(*playerView) bool) {
	views, err := s.playerViews(ctx, campaignID, mapRow)
	if err != nil {
		s.logger.ErrorContext(ctx, "maps: cannot tell which players know a place", "error", err)
		return
	}
	var users []string
	for userID, pv := range views {
		if test(pv) {
			users = append(users, userID)
		}
	}
	slices.Sort(users)
	if len(users) > 0 {
		s.publishUsers(ctx, campaignID, users, mapChangedEvent(mapRow.ID))
	}
}

// publishTokenWritten tells the watching members about a token written (placed,
// moved, hidden, shown or removed): before and after are the rows (nil for none),
// player says whether the character is a player's (an NPC's token on a fog map
// reaches a player only as `vision_changed`), moved is true for a plain move (the
// stream's `token_moved`; any other change is a `map_changed`).
func (s *Service) publishTokenWritten(ctx context.Context, campaignID string, mapRow mapsdb.Map, current string, before, after *mapsdb.MapToken, player, moved bool) {
	token := after
	if token == nil {
		token = before
	}
	seen := playersSee(mapRow.ID, mapRow.RevealedAt, current)
	if !fogged(mapRow) {
		players := seen && ((before != nil && !before.Hidden) || (after != nil && !after.Hidden))
		s.publishTokenEvent(ctx, campaignID, mapRow.ID, *token, moved, players)
		return
	}
	// A player's character is never hidden from the party on a fog map.
	if player {
		s.publishTokenEvent(ctx, campaignID, mapRow.ID, *token, moved, seen)
		s.refreshVision(ctx, campaignID, mapRow.ID)
		return
	}
	// The master hears as ever. The players hear only through the view.
	s.publishTokenEvent(ctx, campaignID, mapRow.ID, *token, moved, false)
	s.refreshAfterNPCToken(ctx, campaignID, mapRow, before, after)
}

// publishTokenEvent tells the watching members of a token written: the stream's
// `token_moved` for a plain move, else a `map_changed`.
func (s *Service) publishTokenEvent(ctx context.Context, campaignID, mapID string, token mapsdb.MapToken, moved, players bool) {
	if moved {
		s.publishTokenMoved(ctx, campaignID, token, players)
		return
	}
	s.publishMapChanged(ctx, campaignID, mapID, players)
}

// refreshAfterNPCToken works out again what the players see after an NPC's token was
// written on a fog map, and tells the ones who see a square the token was on or is on.
func (s *Service) refreshAfterNPCToken(ctx context.Context, campaignID string, mapRow mapsdb.Map, before, after *mapsdb.MapToken) {
	// A token the master keeps hidden is no news to a player, wherever it is. It may
	// carry a light, though, and that changes what they see: the view says so.
	if (before == nil || before.Hidden) && (after == nil || after.Hidden) {
		s.refreshVision(ctx, campaignID, mapRow.ID)
		return
	}
	// While a combat runs on the map the NPC tokens are out of a player's reads
	// (fogcombat.go reads the combatants), so a token's move is no news to anyone.
	if combat, err := s.combats.CombatPositions(ctx, nil, campaignID, mapRow.ID); err == nil && combat.Running {
		s.refreshVision(ctx, campaignID, mapRow.ID)
		return
	}
	size, err := s.queries.GetMapGrid(ctx, mapsdb.GetMapGridParams{CampaignID: campaignID, ID: mapRow.ID})
	if err != nil {
		s.logger.ErrorContext(ctx, "maps: cannot read the map's grid", "error", err)
		return
	}
	g := gridOf(mapRow.GridColumns, mapRow.GridFactor, size.ImageWidth, size.ImageHeight)
	var watch []grid.Square
	for _, t := range []*mapsdb.MapToken{before, after} {
		if t != nil && !t.Hidden {
			watch = append(watch, g.SquareOf(int(t.XBp), int(t.YBp)))
		}
	}
	s.refreshVision(ctx, campaignID, mapRow.ID, watch...)
}

// isPlayerCharacter says whether a character summary is a player's.
func isPlayerCharacter(c *charactersv1.CharacterSummary) bool {
	return c.GetKind() == charactersv1.CharacterKind_CHARACTER_KIND_PLAYER
}
