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

// The light a character carries (MR-036): a light preset's key on
// the character's token, set by the player for their own character and by the
// master for anyone. It moves with the token. The master and the character's
// own player read it (MapToken.carried_light); what the other players see of it
// is its light, which the fog's view (fog.go) works out from the key.

// SetCarriedLight implements mapsv1connect.MapServiceHandler.
func (s *Service) SetCarriedLight(
	ctx context.Context,
	req *connect.Request[mapsv1.SetCarriedLightRequest],
) (*connect.Response[mapsv1.SetCarriedLightResponse], error) {
	m, err := authz.RequireCampaignMember(ctx, req.Msg.GetCampaignId())
	if err != nil {
		return nil, err
	}
	mapID, ok := uid.Canonical(req.Msg.GetMapId())
	if !ok {
		return nil, errMapNotFound()
	}
	carried, err := s.carriedKey(req.Msg.GetLightKey())
	if err != nil {
		return nil, err
	}
	v, err := s.viewerOf(ctx, m)
	if err != nil {
		return nil, err
	}
	character, err := s.livingCharacter(ctx, m.CampaignID, req.Msg.GetCharacterId())
	if err != nil {
		return nil, err
	}
	if err := checkCarrier(v, m, character); err != nil {
		return nil, err
	}

	var mapRow mapsdb.Map
	var token mapsdb.MapToken
	err = s.inSession(ctx, m.CampaignID, func(tx pgx.Tx, t sessiontell.Tell) error {
		var err error
		mapRow, token, err = s.writeCarriedLight(ctx, s.queries.WithTx(tx), v, m, mapID, character.GetId(), carried)
		if err != nil {
			return err
		}
		s.announce(t, func(ctx context.Context) { s.announceCarriedLight(ctx, m.CampaignID, mapRow, v, character) })
		return nil
	})
	if err != nil {
		return nil, s.dbError(ctx, "set a carried light", err)
	}
	return connect.NewResponse(&mapsv1.SetCarriedLightResponse{Token: tokenToProto(token, character, v)}), nil
}

// carriedKey is the light preset's key to store: nil (NULL) for none, and a refusal for
// a key that is no preset.
func (s *Service) carriedKey(key string) (*string, error) {
	if key == "" {
		return nil, nil
	}
	if _, ok := s.rules.LightPreset(key); !ok {
		return nil, badSpec("light_key is not a light preset")
	}
	return &key, nil
}

// checkCarrier checks that the caller may set the character's light: the master for
// anyone, a player for their own character only.
func checkCarrier(v viewer, m authz.Membership, character *charactersv1.CharacterSummary) error {
	if v.master {
		return nil
	}
	// Another player's character is known to the party, an NPC is not: the answer
	// for an NPC is the one for a character that does not exist.
	switch {
	case character.GetKind() != charactersv1.CharacterKind_CHARACTER_KIND_PLAYER:
		return errCharacterNotFound()
	case character.GetPlayerUserId() != m.UserID:
		return connect.NewError(connect.CodePermissionDenied, errors.New("only the character's player or the master may do this"))
	}
	return nil
}

// writeCarriedLight sets the light on the character's token inside the transaction of q
// and returns the map's row and the token as written.
func (s *Service) writeCarriedLight(ctx context.Context, q *mapsdb.Queries, v viewer, m authz.Membership, mapID, characterID string, carried *string) (mapsdb.Map, mapsdb.MapToken, error) {
	mapRow, err := s.campaignMap(ctx, q, m.CampaignID, mapID)
	if err != nil {
		return mapRow, mapsdb.MapToken{}, err
	}
	if !v.seesMap(mapRow.ID, mapRow.RevealedAt) {
		return mapRow, mapsdb.MapToken{}, errMapNotFound() // a hidden map is not found to a player (RN-10)
	}
	// A hidden token is not there for a player: the answer is the one for no
	// token at all, so the call never tells them where their character is hidden.
	current, err := q.GetMapTokenForUpdate(ctx, mapsdb.GetMapTokenForUpdateParams{MapID: mapID, CharacterID: characterID})
	// On a fog map a player's own character is never hidden from them (D6), so
	// the hidden flag only hides the token on a map without fog.
	if errors.Is(err, pgx.ErrNoRows) || (err == nil && !v.master && current.Hidden && !fogged(mapRow)) {
		return mapRow, mapsdb.MapToken{}, errTokenNotFound()
	}
	if err != nil {
		return mapRow, mapsdb.MapToken{}, fmt.Errorf("find token: %w", err)
	}
	token, err := q.SetMapTokenCarriedLight(ctx, mapsdb.SetMapTokenCarriedLightParams{
		MapID: mapID, CharacterID: characterID, CarriedLight: carried, Now: s.now(),
	})
	if errors.Is(err, pgx.ErrNoRows) {
		return mapRow, token, errTokenNotFound()
	}
	if err != nil {
		return mapRow, token, fmt.Errorf("set the carried light: %w", err)
	}
	return mapRow, token, nil
}

// announceCarriedLight tells the watchers after the commit. The master reads the key,
// and so does the character's own player; nobody else is told of it. What the others
// see of the light is what the fog's view works out: on a fog map the light is
// remembered and the players whose view changed are told (`vision_changed`).
func (s *Service) announceCarriedLight(ctx context.Context, campaignID string, mapRow mapsdb.Map, v viewer, character *charactersv1.CharacterSummary) {
	s.publishMapChanged(ctx, campaignID, mapRow.ID, false)
	if owner := character.GetPlayerUserId(); owner != "" && playersSee(mapRow.ID, mapRow.RevealedAt, v.currentMap) {
		s.publishUsers(ctx, campaignID, []string{owner}, mapChangedEvent(mapRow.ID))
	}
	if fogged(mapRow) {
		s.refreshVision(ctx, campaignID, mapRow.ID)
	}
}
