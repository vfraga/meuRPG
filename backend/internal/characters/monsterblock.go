package characters

import (
	"context"

	"github.com/jackc/pgx/v5"

	rulesv1 "github.com/PuraFome/meuRPG/backend/gen/meurpg/rules/v1"
)

// CreatureStatBlock implements play.CombatRoster: the SRD stat block of a creature, in the
// form the bestiary shows it (rules.v1.Creature), for the master's view of a monster in
// a combat. False for a key that is not an SRD creature.
func (s *Service) CreatureStatBlock(ctx context.Context, tx pgx.Tx, campaignID, monsterKey string) (*rulesv1.Creature, bool, error) {
	content, err := s.contentFor(ctx, tx, campaignID)
	if err != nil {
		return nil, false, s.dbError(ctx, "read rules content", err)
	}
	c, ok := content.CreatureByKey(monsterKey)
	if !ok {
		return nil, false, nil
	}
	return creatureToProto(c), true, nil
}
