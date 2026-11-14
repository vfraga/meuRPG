package maps

import (
	"context"
	"errors"
	"fmt"

	"connectrpc.com/connect"
	"github.com/jackc/pgx/v5"

	charactersv1 "github.com/PuraFome/meuRPG/backend/gen/meurpg/characters/v1"
	mapsv1 "github.com/PuraFome/meuRPG/backend/gen/meurpg/maps/v1"
	"github.com/PuraFome/meuRPG/backend/internal/authz"
	"github.com/PuraFome/meuRPG/backend/internal/maps/mapsdb"
	"github.com/PuraFome/meuRPG/backend/internal/platform/sessiontell"
	"github.com/PuraFome/meuRPG/backend/internal/platform/uid"
)

// PlaceMapToken implements mapsv1connect.MapServiceHandler.
func (s *Service) PlaceMapToken(
	ctx context.Context,
	req *connect.Request[mapsv1.PlaceMapTokenRequest],
) (*connect.Response[mapsv1.PlaceMapTokenResponse], error) {
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
	if err := tokenSubject(req.Msg.GetCharacterId(), req.Msg.GetCreatureId()); err != nil {
		return nil, err
	}
	if req.Msg.GetCreatureId() != "" {
		res, err := s.placeCreatureToken(ctx, m, req.Msg, mapID)
		if err != nil {
			return nil, err
		}
		return connect.NewResponse(res), nil
	}
	character, err := s.livingCharacter(ctx, m.CampaignID, req.Msg.GetCharacterId())
	if err != nil {
		return nil, err
	}
	current, err := s.currentMap(ctx, m.CampaignID)
	if err != nil {
		return nil, err
	}
	pl, err := s.placeTokenTx(ctx, m, mapID, current, req.Msg.GetXBp(), req.Msg.GetYBp(), character)
	if err != nil {
		return nil, s.dbError(ctx, "place a token", err)
	}
	return connect.NewResponse(&mapsv1.PlaceMapTokenResponse{Token: tokenToProto(pl.token, character, newViewer(m, current))}), nil
}

// tokenPlacement is a token after PlaceMapToken: the row, the row it replaced
// and whether it is new.
type tokenPlacement struct {
	mapRow     mapsdb.Map
	token, was mapsdb.MapToken
	placed     bool // a new token, not a move
}

// putToken inserts the character's token, or moves the one it has. It returns the token as it
// was (empty for a new one), as it is, and whether it is new.
func (s *Service) putToken(ctx context.Context, q *mapsdb.Queries, mapID string, x, y int32, character *charactersv1.CharacterSummary) (was, token mapsdb.MapToken, placed bool, err error) {
	key := mapsdb.GetMapTokenForUpdateParams{MapID: mapID, CharacterID: character.GetId()}
	was, err = q.GetMapTokenForUpdate(ctx, key)
	switch {
	case errors.Is(err, pgx.ErrNoRows):
		// A player's character starts visible, an NPC hidden (question
		// 31 for Samuel): the master reveals an NPC when the group meets
		// it.
		token, err = q.InsertMapToken(ctx, mapsdb.InsertMapTokenParams{
			MapID: mapID, CharacterID: character.GetId(), XBp: x, YBp: y,
			Hidden:    character.GetKind() != charactersv1.CharacterKind_CHARACTER_KIND_PLAYER,
			UpdatedAt: s.now(),
		})
		placed = true
	case err == nil:
		token, err = q.MoveMapToken(ctx, mapsdb.MoveMapTokenParams{
			MapID: mapID, CharacterID: character.GetId(), XBp: x, YBp: y, UpdatedAt: s.now(),
		})
	}
	if err != nil {
		return was, token, placed, fmt.Errorf("place token: %w", err)
	}
	return was, token, placed, nil
}

// placeTokenTx places or moves the character's token in one transaction, under the session's lock, and tells the watchers once it
// committed.
func (s *Service) placeTokenTx(ctx context.Context, m authz.Membership, mapID, current string, x, y int32, character *charactersv1.CharacterSummary) (tokenPlacement, error) {
	var pl tokenPlacement
	err := s.inSession(ctx, m.CampaignID, func(tx pgx.Tx, t sessiontell.Tell) error {
		q := s.queries.WithTx(tx)
		var err error
		pl = tokenPlacement{}
		if pl.mapRow, err = s.campaignMap(ctx, q, m.CampaignID, mapID); err != nil {
			return err
		}
		if pl.was, pl.token, pl.placed, err = s.putToken(ctx, q, mapID, x, y, character); err != nil {
			return err
		}
		s.announce(t, func(ctx context.Context) { s.announceTokenPlaced(ctx, m, mapID, current, pl, character) })
		// What the character sees from where it landed is part of this write: the landing moves the number once.
		return s.noticeLanded(ctx, tx, t, m.CampaignID, mapID, pl.mapRow, []landedToken{{token: pl.token, character: character}})
	})
	return pl, err
}

// announceTokenPlaced tells the watchers of a placed or moved token, and lets a
// trap react to it.
func (s *Service) announceTokenPlaced(ctx context.Context, m authz.Membership, mapID, current string, pl tokenPlacement, character *charactersv1.CharacterSummary) {
	// A new token needs its name: the app reads the map again. A move is a
	// `token_moved`, except for an NPC on a fog map, which reaches a player only as
	// what they see (publishTokenWritten).
	var before *mapsdb.MapToken
	if !pl.placed {
		before = &pl.was
	}
	s.publishTokenWritten(ctx, m.CampaignID, pl.mapRow, current, before, &pl.token, isPlayerCharacter(character), !pl.placed)
	s.tokenLanded(ctx, m.CampaignID, m.UserID, mapID, pl.token, character) // a trap may fire, or be noticed (MR-035)
}

// SetMapTokenHidden implements mapsv1connect.MapServiceHandler.
func (s *Service) SetMapTokenHidden(
	ctx context.Context,
	req *connect.Request[mapsv1.SetMapTokenHiddenRequest],
) (*connect.Response[mapsv1.SetMapTokenHiddenResponse], error) {
	m, err := authz.RequireCampaignRole(ctx, req.Msg.GetCampaignId(), authz.RoleMaster)
	if err != nil {
		return nil, err
	}
	mapID, ok := uid.Canonical(req.Msg.GetMapId())
	if !ok {
		return nil, errMapNotFound()
	}
	character, err := s.livingCharacter(ctx, m.CampaignID, req.Msg.GetCharacterId())
	if err != nil {
		return nil, err // a dead character's token is not listed either
	}
	current, err := s.currentMap(ctx, m.CampaignID)
	if err != nil {
		return nil, err
	}

	var mapRow mapsdb.Map
	var before, after mapsdb.MapToken
	err = s.inSession(ctx, m.CampaignID, func(tx pgx.Tx, t sessiontell.Tell) error {
		var err error
		if mapRow, before, after, err = s.setTokenHiddenTx(ctx, tx, m.CampaignID, mapID, character.GetId(), req.Msg.GetHidden()); err != nil {
			return err
		}
		s.announce(t, func(ctx context.Context) {
			s.publishTokenWritten(ctx, m.CampaignID, mapRow, current, &before, &after, isPlayerCharacter(character), false)
		})
		return nil
	})
	if err != nil {
		return nil, s.dbError(ctx, "hide or show a token", err)
	}
	return connect.NewResponse(&mapsv1.SetMapTokenHiddenResponse{Token: tokenToProto(after, character, newViewer(m, current))}), nil
}

// setTokenHiddenTx is SetMapTokenHidden's transaction: it checks the map is the campaign's,
// locks the character's token and sets whether it is hidden. It returns the map, and the
// token as it was and as it is.
func (s *Service) setTokenHiddenTx(ctx context.Context, tx pgx.Tx, campaignID, mapID, characterID string, hidden bool) (mapRow mapsdb.Map, before, after mapsdb.MapToken, err error) {
	q := s.queries.WithTx(tx)
	if mapRow, err = s.campaignMap(ctx, q, campaignID, mapID); err != nil {
		return mapRow, before, after, err
	}
	before, err = q.GetMapTokenForUpdate(ctx, mapsdb.GetMapTokenForUpdateParams{MapID: mapID, CharacterID: characterID})
	if errors.Is(err, pgx.ErrNoRows) {
		return mapRow, before, after, errTokenNotFound()
	}
	if err != nil {
		return mapRow, before, after, fmt.Errorf("find token: %w", err)
	}
	after, err = q.SetMapTokenHidden(ctx, mapsdb.SetMapTokenHiddenParams{MapID: mapID, CharacterID: characterID, Hidden: hidden, Now: s.now()})
	if err != nil {
		return mapRow, before, after, fmt.Errorf("hide or show token: %w", err)
	}
	return mapRow, before, after, nil
}

// RemoveMapToken implements mapsv1connect.MapServiceHandler. It does not
// ask whether the character still lives: a dead character's token can be
// taken off too.
func (s *Service) RemoveMapToken(
	ctx context.Context,
	req *connect.Request[mapsv1.RemoveMapTokenRequest],
) (*connect.Response[mapsv1.RemoveMapTokenResponse], error) {
	m, err := authz.RequireCampaignRole(ctx, req.Msg.GetCampaignId(), authz.RoleMaster)
	if err != nil {
		return nil, err
	}
	mapID, ok := uid.Canonical(req.Msg.GetMapId())
	if !ok {
		return nil, errMapNotFound()
	}
	if err := tokenSubject(req.Msg.GetCharacterId(), req.Msg.GetCreatureId()); err != nil {
		return nil, err
	}
	if req.Msg.GetCreatureId() != "" {
		if err := s.removeCreatureToken(ctx, m, mapID, req.Msg.GetCreatureId()); err != nil {
			return nil, err
		}
		return connect.NewResponse(&mapsv1.RemoveMapTokenResponse{}), nil
	}
	characterID, ok := uid.Canonical(req.Msg.GetCharacterId())
	if !ok {
		return nil, errTokenNotFound()
	}
	if err := s.removeCharacterToken(ctx, m, mapID, characterID); err != nil {
		return nil, err
	}
	return connect.NewResponse(&mapsv1.RemoveMapTokenResponse{}), nil
}

// removeCharacterToken takes a character's token off the map and tells the
// watchers.
func (s *Service) removeCharacterToken(ctx context.Context, m authz.Membership, mapID, characterID string) error {
	current, err := s.currentMap(ctx, m.CampaignID)
	if err != nil {
		return err
	}
	err = s.deleteTokenTx(ctx, m.CampaignID, mapID, characterID, func(ctx context.Context, mapRow mapsdb.Map, deleted mapsdb.MapToken) {
		// Whose token it was: a dead character's is told as an NPC's, which reaches no
		// player on a fog map but through what they see.
		player := false
		if found, err := s.characters.MapCharacters(ctx, nil, m.CampaignID, []string{characterID}); err == nil && len(found) == 1 {
			player = isPlayerCharacter(found[0])
		}
		s.publishTokenWritten(ctx, m.CampaignID, mapRow, current, &deleted, nil, player, false)
	})
	if err != nil {
		return s.dbError(ctx, "remove a token", err)
	}
	return nil
}

// deleteTokenTx deletes the character's token in one transaction, under the session's lock, and runs tell once it committed.
func (s *Service) deleteTokenTx(ctx context.Context, campaignID, mapID, characterID string, tell func(ctx context.Context, mapRow mapsdb.Map, deleted mapsdb.MapToken)) error {
	var mapRow mapsdb.Map
	var deleted mapsdb.MapToken
	return s.inSession(ctx, campaignID, func(tx pgx.Tx, t sessiontell.Tell) error {
		q := s.queries.WithTx(tx)
		var err error
		if mapRow, err = s.campaignMap(ctx, q, campaignID, mapID); err != nil {
			return err
		}
		deleted, err = q.DeleteMapToken(ctx, mapsdb.DeleteMapTokenParams{MapID: mapID, CharacterID: characterID})
		if errors.Is(err, pgx.ErrNoRows) {
			return errTokenNotFound()
		}
		if err != nil {
			return fmt.Errorf("remove token: %w", err)
		}
		s.announce(t, func(ctx context.Context) { tell(ctx, mapRow, deleted) })
		return nil
	})
}
