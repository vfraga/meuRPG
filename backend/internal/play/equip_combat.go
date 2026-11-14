package play

import (
	"context"
	"fmt"

	"connectrpc.com/connect"
	"github.com/jackc/pgx/v5"

	playv1 "github.com/PuraFome/meuRPG/backend/gen/meurpg/play/v1"
	"github.com/PuraFome/meuRPG/backend/internal/platform/conv"
	"github.com/PuraFome/meuRPG/backend/internal/platform/sessiontell"
	"github.com/PuraFome/meuRPG/backend/internal/play/link"
	"github.com/PuraFome/meuRPG/backend/internal/play/playdb"
)

// Equipping and stowing gear in a combat (SRD 5.1, "Other Activity on Your Turn" and
// "Armor"): the character drawing or stowing one weapon uses the turn's free object
// interaction; a second one in the same turn, and donning or doffing a shield, costs the
// action; body armor takes minutes, so a player cannot change it in a fight. The master has
// the final word: no turn, no economy, armor included. Outside a running combat it is free.

// EquipInCombat implements characters.ReviewHost: it asks the combat that holds the
// character, inside the transaction that writes the sheet and before it writes. It writes the
// swap as an event of the combat (gear_swapped) with the turn's economy from before, so the master's
// undo takes the swap back first. It runs inside characters' InSession, with the session's row already
// held: what it tells (the combat, the swap's line) goes out through t after the commit, with the
// write's number, and outside a combat it tells nothing.
func (s *Service) EquipInCombat(ctx context.Context, tx pgx.Tx, campaignID, characterID, actorUserID string, change link.EquipChange, t sessiontell.Tell) error {
	q := s.queries.WithTx(tx)
	session, enc, found, err := openCombatOf(ctx, q, campaignID)
	if err != nil || !found || enc.Status != statusActive {
		return err // no running combat: gear is free to change
	}
	cs, err := q.ListCombatants(ctx, enc.ID)
	if err != nil {
		return fmt.Errorf("list the combatants: %w", err)
	}
	who := wearerIn(cs, characterID)
	if who == nil {
		return nil
	}
	c, err := s.openTx(ctx, combatTx{tx: tx, q: q, session: session, enc: enc, now: s.now(), actorUserID: actorUserID, svc: s})
	if err != nil {
		return err
	}
	swap := gearSwapOf(*who, change, c.enc.Round)
	if conv.OrZero(who.UserID) == actorUserID { // the owner is bound by the turn; the master has the last word
		if swap.Cost, err = s.mayEquip(ctx, c, *who, change); err != nil {
			return err
		}
	}
	if err := s.writeGearSwap(ctx, c, actorUserID, characterID, swap); err != nil {
		return err
	}
	if err := touch(ctx, c); err != nil { // as every change of a combat: the stream carries the new revision
		return err
	}
	owner := conv.OrZero(who.UserID)
	st := tellOf(t)
	st.touchSteps()
	st.later(func(ctx context.Context, say sayFn) {
		s.tellEncounterChanged(ctx, say, campaignID, c.enc)
		s.tellGearLog(say, campaignID, c.enc.ID, owner) // the swap's line is the master's and the owner's
	})
	return nil
}

// mayEquip checks a player's change against the combat and spends what it costs; it says what it cost.
func (s *Service) mayEquip(ctx context.Context, c *combatTx, who playdb.Combatant, change link.EquipChange) (string, error) {
	if change.Kind == link.EquipArmor {
		return "", errEncounter(playv1.EncounterBlockedReason_ENCOUNTER_BLOCKED_REASON_ARMOR_IN_COMBAT, "body armor takes minutes to don or doff: not in a combat")
	}
	if err := s.mustActNow(ctx, c, who); err != nil {
		return "", err
	}
	return s.payEquip(ctx, c, who, change)
}

// wearerIn is the combatant of the player's character in cs, nil when it is not in the combat.
func wearerIn(cs []playdb.Combatant, characterID string) *playdb.Combatant {
	for i := range cs {
		if cs[i].Kind == kindPlayer && cs[i].CharacterID == characterID && !cs[i].Dismissed {
			return &cs[i]
		}
	}
	return nil
}

// payEquip spends what the change costs: the first weapon of the turn the free object
// interaction, every other change the action.
func (s *Service) payEquip(ctx context.Context, c *combatTx, who playdb.Combatant, change link.EquipChange) (string, error) {
	if change.Kind == link.EquipWeapon && !who.ObjectInteractionUsed {
		if err := c.q.SetCombatantObjectInteractionUsed(ctx, playdb.SetCombatantObjectInteractionUsedParams{ID: who.ID, ObjectInteractionUsed: true}); err != nil {
			return "", fmt.Errorf("spend the free object interaction: %w", err)
		}
		return gearCostFree, nil
	}
	return s.payEquipWithAction(ctx, c, who)
}

// payEquipWithAction spends the action of the turn, or the extra action an effect gives for
// using an object (Haste) when the action is used; with neither it refuses. It says which it spent.
func (s *Service) payEquipWithAction(ctx context.Context, c *combatTx, who playdb.Combatant) (string, error) {
	if !who.ActionUsed {
		if err := c.q.SetCombatantEconomy(ctx, playdb.SetCombatantEconomyParams{
			ID: who.ID, ActionUsed: true, BonusActionUsed: who.BonusActionUsed, ReactionUsed: who.ReactionUsed, Dashed: who.Dashed,
		}); err != nil {
			return "", fmt.Errorf("spend the action: %w", err)
		}
		return gearCostAction, nil
	}
	err := s.mustHaveExtraAction(ctx, c, who, "standard:use-an-object")
	switch {
	case err == nil:
		if err := c.q.SetCombatantExtraActionUsed(ctx, playdb.SetCombatantExtraActionUsedParams{ID: who.ID, ExtraActionUsed: true}); err != nil {
			return "", fmt.Errorf("spend the extra action: %w", err)
		}
		return gearCostExtra, nil
	case connect.CodeOf(err) == connect.CodeFailedPrecondition:
		return "", errEncounter(playv1.EncounterBlockedReason_ENCOUNTER_BLOCKED_REASON_ACTION_USED, "the action of this turn is used")
	}
	return "", err
}
