package play

import (
	"context"
	"fmt"
	"slices"

	"github.com/jackc/pgx/v5"

	playv1 "github.com/PuraFome/meuRPG/backend/gen/meurpg/play/v1"
	"github.com/PuraFome/meuRPG/backend/internal/play/playdb"
)

// How a combat changes a player's character's vitals (RN-02): the hit points
// (damage, healing), the spell slots and the class resources go through the
// same VitalsKeeper the master's correction uses, inside the change's own
// transaction, so there is one source of truth for them. Everything here runs
// after the session's row is locked.

// changeVitals applies a change to a character's vitals and, when it takes the
// character from 0 hit points to above 0, resets the death save counts of its
// combatant in the session's combat (RN-03: any healing above 0 does it: a
// spell, Retomar o fôlego, a natural 20, the master's correction, an undo). A
// drop from above 0 to 0 on the character's own turn holds its death save to
// its next turn. It returns the vitals before and after.
func (s *Service) changeVitals(ctx context.Context, q *playdb.Queries, tx pgx.Tx, sessionID, campaignID, characterID string, req *playv1.AdjustCharacterVitalsRequest) (before, after *playv1.CharacterVitals, err error) {
	before, after, err = s.vitals.AdjustVitals(ctx, tx, campaignID, characterID, req)
	if err != nil {
		return nil, nil, err
	}
	// A druid whose own hit points reach 0 is itself again (SRD): the beast form
	// ends, whatever took them there (damage, the master's hand, a spell).
	if after.GetWildShape() != nil && after.GetHitPointsMax() > 0 && after.GetHitPointsCurrent() == 0 {
		if after, err = s.formEnds(ctx, q, tx, sessionID, campaignID, characterID, after.GetWildShape().GetBeastKey(), endedAtZero, 0); err != nil {
			return nil, nil, err
		}
	}
	if before.GetHitPointsCurrent() > 0 && after.GetHitPointsCurrent() == 0 {
		if err := q.MarkDeathSaveRolledOnTurn(ctx, playdb.MarkDeathSaveRolledOnTurnParams{CharacterID: characterID, GameSessionID: sessionID}); err != nil {
			return nil, nil, fmt.Errorf("hold the death save to the next turn: %w", err)
		}
	}
	if before.GetHitPointsCurrent() == 0 && after.GetHitPointsCurrent() > 0 {
		if err := q.ResetDeathSavesOfCharacter(ctx, playdb.ResetDeathSavesOfCharacterParams{CharacterID: characterID, GameSessionID: sessionID}); err != nil {
			return nil, nil, fmt.Errorf("reset the death saves: %w", err)
		}
	}
	return before, after, nil
}

// vitalsOf changes a character's vitals inside the combat's transaction.
func (s *Service) vitalsOf(ctx context.Context, c *combatTx, characterID string, req *playv1.AdjustCharacterVitalsRequest) (before, after *playv1.CharacterVitals, err error) {
	before, after, err = s.changeVitals(ctx, c.q, c.tx, c.session.ID, c.session.CampaignID, characterID, req)
	if err == nil && before.GetWildShape() != nil && after.GetWildShape() == nil {
		c.told = append(c.told, after) // the beast is gone: the streams and the fog hear of it
	}
	return before, after, err
}

// spendSlot spends one spell slot of a player's character (delta 1) or gives it
// back (delta -1: an undo), and returns its vitals after. NO_SLOT when none is
// free. Giving back never fails for what the sheet became meanwhile: a slot level
// the sheet no longer has is left alone, and a count above the new total is cut to it.
func (s *Service) spendSlot(ctx context.Context, c *combatTx, characterID string, slot slotRef, delta int32) (*playv1.CharacterVitals, error) {
	now, err := s.vitals.GetVitalsTx(ctx, c.tx, c.session.CampaignID, characterID)
	if err != nil {
		return nil, err
	}
	noSlot := func() error {
		return errEncounter(playv1.EncounterBlockedReason_ENCOUNTER_BLOCKED_REASON_NO_SLOT, "there is no free spell slot of that level",
			func(b *playv1.EncounterBlocked) { b.MinLevel = slot.Level })
	}
	req := &playv1.AdjustCharacterVitalsRequest{}
	if slot.Pact {
		p := now.GetPactSlots()
		if p == nil || p.GetSlotLevel() != slot.Level {
			return s.lostOnGiveBack(ctx, now, delta, "pact slot", noSlot())
		}
		used := p.GetUsed() + delta
		if used > p.GetTotal() {
			if delta < 0 {
				used = p.GetTotal()
			} else {
				return nil, noSlot()
			}
		}
		used = max(used, 0)
		req.PactSlotsUsed = &used
	} else {
		i := slices.IndexFunc(now.GetSpellSlots(), func(u *playv1.SpellSlotUsage) bool { return u.GetLevel() == slot.Level })
		if i < 0 {
			return s.lostOnGiveBack(ctx, now, delta, "spell slot", noSlot())
		}
		u := now.GetSpellSlots()[i]
		used := u.GetUsed() + delta
		if used > u.GetTotal() {
			if delta < 0 {
				used = u.GetTotal()
			} else {
				return nil, noSlot()
			}
		}
		req.SpellSlotsUsed = []*playv1.SpellSlotsUsed{{Level: slot.Level, Used: max(used, 0)}}
	}
	_, after, err := s.vitalsOf(ctx, c, characterID, req)
	return after, err
}

// lostOnGiveBack answers a spend that finds nothing to spend: for a spend, the
// refusal; for a give-back (an undo), the sheet no longer has the slot or the
// resource, so there is nothing to give back and the vitals stay as they are.
func (s *Service) lostOnGiveBack(ctx context.Context, now *playv1.CharacterVitals, delta int32, what string, refusal error) (*playv1.CharacterVitals, error) {
	if delta >= 0 {
		return nil, refusal
	}
	s.logger.WarnContext(ctx, "play: an undo found nothing to give back", "what", what)
	return now, nil
}

// spendResource spends one use of a class or race resource of a player's
// character (delta 1) or gives it back (delta -1), and returns its vitals
// after. NO_USES when none is left. Giving back never fails for what the sheet
// became meanwhile (see spendSlot).
func (s *Service) spendResource(ctx context.Context, c *combatTx, characterID, key string, delta int32) (*playv1.CharacterVitals, error) {
	now, err := s.vitals.GetVitalsTx(ctx, c.tx, c.session.CampaignID, characterID)
	if err != nil {
		return nil, err
	}
	i := slices.IndexFunc(now.GetResources(), func(r *playv1.ResourceUsage) bool { return r.GetKey() == key })
	if i < 0 {
		if delta < 0 {
			return s.lostOnGiveBack(ctx, now, delta, "resource", nil)
		}
		return nil, fmt.Errorf("character %s has no resource %q", characterID, key) // the options said it had
	}
	r := now.GetResources()[i]
	used := r.GetUsed() + delta
	if used > r.GetTotal() && delta < 0 {
		used = r.GetTotal()
	}
	if used > r.GetTotal() {
		return nil, errEncounter(playv1.EncounterBlockedReason_ENCOUNTER_BLOCKED_REASON_NO_USES, "no uses left",
			func(b *playv1.EncounterBlocked) { b.Recharge = r.GetRecharge() })
	}
	_, after, err := s.vitalsOf(ctx, c, characterID, &playv1.AdjustCharacterVitalsRequest{
		ResourcesUsed: []*playv1.ResourceUsed{{Key: key, Used: max(used, 0)}},
	})
	return after, err
}
