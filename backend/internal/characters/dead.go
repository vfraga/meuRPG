package characters

import (
	"context"
	"errors"

	"github.com/jackc/pgx/v5"

	charactersv1 "github.com/PuraFome/meuRPG/backend/gen/meurpg/characters/v1"
	"github.com/PuraFome/meuRPG/backend/internal/characters/charactersdb"
	"github.com/PuraFome/meuRPG/backend/internal/platform/conv"
	"github.com/PuraFome/meuRPG/backend/internal/platform/uid"
	"github.com/PuraFome/meuRPG/backend/internal/play/link"
)

// The dead, for Revivify (play.CombatRoster). The methods take no caller: they run after
// package play's own authorization check.

// DeadCharacters implements play.CombatRoster: the campaign's dead player characters, newest
// death first.
func (s *Service) DeadCharacters(ctx context.Context, tx pgx.Tx, campaignID string) ([]link.DeadCharacter, error) {
	rows, err := s.queriesIn(tx).ListDeadPlayerCharacters(ctx, campaignID)
	if err != nil {
		return nil, wrap("list the dead", err)
	}
	out := make([]link.DeadCharacter, len(rows))
	for i, r := range rows {
		out[i] = link.DeadCharacter{ID: r.ID, Name: r.Name, PlayerUserID: conv.OrZero(r.PlayerUserID), RevivifyBlocked: r.RevivifyBlocked}
	}
	return out, nil
}

// MarkRevivifyBlocked implements play.CombatRoster: the master's switch on a dead character.
func (s *Service) MarkRevivifyBlocked(ctx context.Context, tx pgx.Tx, campaignID, characterID string, blocked bool) error {
	id, ok := uid.Canonical(characterID)
	if !ok {
		return errCharacterNotFound()
	}
	_, err := s.queries.WithTx(tx).SetCharacterRevivifyBlocked(ctx, charactersdb.SetCharacterRevivifyBlockedParams{CampaignID: campaignID, ID: id, Blocked: blocked})
	if errors.Is(err, pgx.ErrNoRows) {
		return errBlocked(charactersv1.CharacterBlockedReason_CHARACTER_BLOCKED_REASON_NOT_DEAD, id)
	}
	if err != nil {
		return wrap("set the revivify switch", err)
	}
	return nil
}
