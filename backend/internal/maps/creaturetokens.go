package maps

import (
	"context"
	"errors"
	"fmt"

	"connectrpc.com/connect"
	"github.com/jackc/pgx/v5"
	"google.golang.org/protobuf/types/known/timestamppb"

	mapsv1 "github.com/PuraFome/meuRPG/backend/gen/meurpg/maps/v1"
	"github.com/PuraFome/meuRPG/backend/internal/authz"
	"github.com/PuraFome/meuRPG/backend/internal/maps/link"
	"github.com/PuraFome/meuRPG/backend/internal/maps/mapsdb"
	"github.com/PuraFome/meuRPG/backend/internal/platform/sessiontell"
	"github.com/PuraFome/meuRPG/backend/internal/platform/uid"
)

// The tokens of a character's creatures (MR-037). The master places
// one like any token (PlaceMapToken with creature_id); it lives in a table of its
// own (map_creature_tokens) because a character's creatures are many and map_tokens
// holds one token for each character.
//
// A creature's token is a party token (D6), like a player character's: there is no
// hidden flag, no player is kept from it on a map with the fog of war on, and its
// move reaches every player who sees the map. What a creature SEES for its owner
// is a separate matter, and a narrower one: only the familiar, and only while the
// player looks through its eyes (fog.go, newSight).

// errCreatureNotFound is not_found for a creature that is not a live creature of a
// living character of the campaign.
func errCreatureNotFound() error {
	return connect.NewError(connect.CodeNotFound, errors.New("creature not found"))
}

// tokenSubject checks that a place or remove request names exactly one of a
// character and a creature.
func tokenSubject(characterID, creatureID string) error {
	if (characterID == "") == (creatureID == "") {
		return connect.NewError(connect.CodeInvalidArgument, errors.New("set exactly one of character_id and creature_id"))
	}
	return nil
}

// liveCreature finds a live creature of the campaign.
func (s *Service) liveCreature(ctx context.Context, campaignID, rawID string) (link.MapCreature, error) {
	id, ok := uid.Canonical(rawID)
	if !ok {
		return link.MapCreature{}, errCreatureNotFound()
	}
	found, err := s.characters.MapCreatures(ctx, campaignID, []string{id})
	if err != nil {
		return link.MapCreature{}, s.dbError(ctx, "find a creature", err)
	}
	if len(found) != 1 {
		return link.MapCreature{}, errCreatureNotFound()
	}
	return found[0], nil
}

// creatureTokenToProto builds the MapToken of a creature as the viewer sees it.
func creatureTokenToProto(t mapsdb.MapCreatureToken, c link.MapCreature, v viewer) *mapsv1.MapToken {
	return &mapsv1.MapToken{
		MapId: t.MapID, CharacterId: c.OwnerCharacterID, CreatureId: t.CreatureID, Name: c.Name,
		Mine: c.OwnerUserID != "" && c.OwnerUserID == v.userID,
		XBp:  t.XBp, YBp: t.YBp, UpdatedAt: timestamppb.New(t.UpdatedAt),
	}
}

// creatureTokensOf lists the creature tokens of a map as the viewer sees them:
// every one for everyone who sees the map (a party token), with the dismissed
// creatures and the dead owners' left out. The caller checked the viewer sees the
// map.
func (s *Service) creatureTokensOf(ctx context.Context, campaignID, mapID string, v viewer) ([]*mapsv1.MapToken, error) {
	rows, err := s.queries.ListMapCreatureTokens(ctx, mapID)
	if err != nil {
		return nil, fmt.Errorf("list a map's creature tokens: %w", err)
	}
	if len(rows) == 0 {
		return nil, nil
	}
	ids := make([]string, len(rows))
	byID := make(map[string]mapsdb.MapCreatureToken, len(rows))
	for i, t := range rows {
		ids[i] = t.CreatureID
		byID[t.CreatureID] = t
	}
	creatures, err := s.characters.MapCreatures(ctx, campaignID, ids)
	if err != nil {
		return nil, fmt.Errorf("read the creatures on a map: %w", err)
	}
	out := make([]*mapsv1.MapToken, 0, len(creatures))
	for _, c := range creatures {
		if t, ok := byID[c.ID]; ok {
			out = append(out, creatureTokenToProto(t, c, v))
		}
	}
	return out, nil
}

// placeCreatureToken is PlaceMapToken for a creature.
func (s *Service) placeCreatureToken(ctx context.Context, m authz.Membership, req *mapsv1.PlaceMapTokenRequest, mapID string) (*mapsv1.PlaceMapTokenResponse, error) {
	creature, err := s.liveCreature(ctx, m.CampaignID, req.GetCreatureId())
	if err != nil {
		return nil, err
	}
	current, err := s.currentMap(ctx, m.CampaignID)
	if err != nil {
		return nil, err
	}
	var mapRow mapsdb.Map
	var token mapsdb.MapCreatureToken
	err = s.inSession(ctx, m.CampaignID, func(tx pgx.Tx, t sessiontell.Tell) error {
		q := s.queries.WithTx(tx)
		var err error
		if mapRow, err = s.campaignMap(ctx, q, m.CampaignID, mapID); err != nil {
			return err
		}
		token, err = q.UpsertMapCreatureToken(ctx, mapsdb.UpsertMapCreatureTokenParams{
			MapID: mapID, CreatureID: creature.ID, XBp: req.GetXBp(), YBp: req.GetYBp(), UpdatedAt: s.now(),
		})
		if err != nil {
			return fmt.Errorf("place creature token: %w", err)
		}
		s.announce(t, func(ctx context.Context) {
			s.publishCreatureToken(ctx, m.CampaignID, mapRow, current)
			s.creatureLanded(ctx, m.CampaignID, m.UserID, mapID, token, creature) // a trap may fire (MR-035)
		})
		return nil
	})
	if err != nil {
		return nil, s.dbError(ctx, "place a creature's token", err)
	}
	return &mapsv1.PlaceMapTokenResponse{Token: creatureTokenToProto(token, creature, newViewer(m, current))}, nil
}

// removeCreatureToken is RemoveMapToken for a creature. It does not ask whether the
// creature still lives: a dismissed creature's token can be taken off too.
func (s *Service) removeCreatureToken(ctx context.Context, m authz.Membership, mapID, rawCreatureID string) error {
	creatureID, ok := uid.Canonical(rawCreatureID)
	if !ok {
		return errTokenNotFound()
	}
	current, err := s.currentMap(ctx, m.CampaignID)
	if err != nil {
		return err
	}
	var mapRow mapsdb.Map
	err = s.inSession(ctx, m.CampaignID, func(tx pgx.Tx, t sessiontell.Tell) error {
		q := s.queries.WithTx(tx)
		var err error
		if mapRow, err = s.campaignMap(ctx, q, m.CampaignID, mapID); err != nil {
			return err
		}
		if _, err = q.DeleteMapCreatureToken(ctx, mapsdb.DeleteMapCreatureTokenParams{MapID: mapID, CreatureID: creatureID}); errors.Is(err, pgx.ErrNoRows) {
			return errTokenNotFound()
		} else if err != nil {
			return fmt.Errorf("remove creature token: %w", err)
		}
		s.announce(t, func(ctx context.Context) { s.publishCreatureToken(ctx, m.CampaignID, mapRow, current) })
		return nil
	})
	if err != nil {
		return s.dbError(ctx, "remove a creature's token", err)
	}
	return nil
}

// publishCreatureToken tells the watching members a creature's token was written:
// a party token reaches every player who sees the map, and on a fog map what the
// familiar's eyes see may have changed too.
func (s *Service) publishCreatureToken(ctx context.Context, campaignID string, mapRow mapsdb.Map, current string) {
	s.publishMapChanged(ctx, campaignID, mapRow.ID, playersSee(mapRow.ID, mapRow.RevealedAt, current))
	if fogged(mapRow) {
		s.refreshVision(ctx, campaignID, mapRow.ID)
	}
}
