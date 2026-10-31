package play

import (
	"context"
	"slices"

	"connectrpc.com/connect"

	"github.com/PuraFome/meuRPG/backend/internal/play/link"
	"github.com/PuraFome/meuRPG/backend/internal/play/playdb"
)

// A character's sheet can make it immune to conditions (a table's condition_immunity effect): no
// spell, rider, trap or hold puts the condition on it. A creature's or an NPC's immunities come from
// its stat block and ride on its traits the same way (rules.Content.ConditionImmunities); the immunity
// a lasting effect gives (Heroism) is read from the states (combat.ImmuneTo).

// sheetImmuneTo says the traits make the sheet immune to the condition.
func sheetImmuneTo(t link.Traits, condition string) bool {
	return slices.ContainsFunc(t.ConditionImmunities, func(i link.ConditionImmunity) bool {
		return slices.Contains(i.Conditions, condition)
	})
}

// immuneToCondition says the combatant's sheet, or its stat block, makes it immune to the condition.
func (c *combatTx) immuneToCondition(ctx context.Context, who playdb.Combatant, condition string) (bool, error) {
	if condition == "" {
		return false, nil
	}
	sheet, err := c.svc.sheetOf(ctx, c.tx, c.session.CampaignID, who)
	if err != nil {
		return false, err
	}
	return sheetImmuneTo(sheet.Traits, condition), nil
}

// sheetImmunities are the conditions each of the combatants is immune to by its sheet, by combatant.
func (c *combatTx) sheetImmunities(ctx context.Context, targets []playdb.Combatant) (map[string]link.Traits, error) {
	out := make(map[string]link.Traits, len(targets))
	for _, t := range targets {
		sheet, err := c.svc.sheetOf(ctx, c.tx, c.session.CampaignID, t)
		if isCreature(t) && connect.CodeOf(err) == connect.CodeNotFound {
			continue // a creature the content does not know has no stat block to be immune by
		}
		if err != nil {
			return nil, err
		}
		out[t.ID] = sheet.Traits
	}
	return out, nil
}
