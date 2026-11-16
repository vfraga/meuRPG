package play

import (
	"context"
	"fmt"

	"github.com/google/uuid"

	playv1 "github.com/PuraFome/meuRPG/backend/gen/meurpg/play/v1"
	"github.com/PuraFome/meuRPG/backend/internal/authz"
	"github.com/PuraFome/meuRPG/backend/internal/platform/conv"
	"github.com/PuraFome/meuRPG/backend/internal/play/playdb"
	"github.com/PuraFome/meuRPG/backend/internal/rules"
)

// The rider of a creature's attack (SRD 5.1, Monsters): a ghoul's claws and a giant spider's bite ask a
// saving throw of the creature they hit (a condition, or extra damage with half on a success), and a
// constrictor's attack grapples with no save (an escape DC). The stat block's plan says which attack has
// one (rules.ActionPlan.Save.OnHit, HitCondition). Once the hit has landed (the damage applied,
// afterDamage) the attack offers the rider to the master as a hit rider of the kind "creature", open until
// answered or the round ends: Apply has the server roll the target's save as a spell's save is rolled,
// open the damage the save governs as pending damage and give the condition to a target that fails; for a
// grapple it gives the condition and notes the escape DC in the line. Decline skips it. The offer and
// its line are the master's: the DC is a number of the stat block.

// creatureRiderOf says whether the action has a rider: a save asked of the target of a hit, or a condition
// a hit gives.
func creatureRiderOf(plan rules.ActionPlan) bool {
	return plan.Attack != nil && ((plan.Save != nil && plan.Save.OnHit) || plan.HitCondition != nil)
}

// offerCreatureRider opens the offer of the rider an NPC's attack has once its damage landed. Only an
// attack's own damage offers one: the damage a rider or a cast opened (it has a cast) does not.
func (s *Service) offerCreatureRider(ctx context.Context, c *combatTx, target playdb.Combatant, d damageLanded) error {
	if d.pending.CastID != nil || d.attacker == "" || d.key == "" {
		return nil
	}
	if d.hit.After != nil && (d.hit.After.HP == 0 || d.hit.After.Defeated) { // a target the hit downed or defeated has no save to ask
		return nil
	}
	cs, err := listCombatants(ctx, c)
	if err != nil {
		return err
	}
	attacker, ok := combatantByID(cs, d.attacker)
	if !ok || attacker.Kind != kindNPC || attacker.Side == target.Side {
		return nil
	}
	plan, ok, err := s.planOfAttack(ctx, c, attacker, d.key, false)
	if err != nil || !ok || !creatureRiderOf(plan) {
		return err
	}
	return s.insertCreatureRider(ctx, c, attacker, target, plan, d.pending.ID)
}

// insertCreatureRider writes the offer of a rider, once for the damage that opened it.
func (s *Service) insertCreatureRider(ctx context.Context, c *combatTx, attacker, target playdb.Combatant, plan rules.ActionPlan, pending string) error {
	if n, err := c.q.CountHitRidersOfPending(ctx, &pending); err != nil || n > 0 {
		return err
	}
	if _, err := c.q.InsertHitRider(ctx, playdb.InsertHitRiderParams{
		EncounterID: c.enc.ID, AttackerID: attacker.ID, TargetID: target.ID, Kind: riderCreature, Round: c.enc.Round, CreatedAt: c.now, ActionKey: &plan.Key, PendingID: &pending,
	}); err != nil {
		return fmt.Errorf("offer the creature's rider: %w", err)
	}
	return nil
}

// answerCreatureRider answers a creature's rider: a decline writes the line; an apply gives the grapple's
// condition and rolls the save the attack asks.
func (s *Service) answerCreatureRider(ctx context.Context, c *combatTx, m authz.Membership, row playdb.HitRider, attacker, target playdb.Combatant, choice string, re *riderEvent) error {
	plan, ok, err := s.planOfAction(ctx, c, attacker, conv.OrZero(row.ActionKey))
	if err != nil {
		return err
	}
	if !ok {
		return errEncounter(playv1.EncounterBlockedReason_ENCOUNTER_BLOCKED_REASON_CREATURE_ACTION_UNAVAILABLE, "the attack is no longer in the stat block")
	}
	re.ActionKey, re.ActionName = plan.Key, nameOfPlan(plan)
	if choice == choiceDecline {
		return nil
	}
	re.Result = riderApplied
	if hc := plan.HitCondition; hc != nil {
		if err := s.grapplingRider(ctx, c, target, hc, re); err != nil {
			return err
		}
	}
	if sv := plan.Save; sv != nil && sv.OnHit {
		return s.savingRider(ctx, c, m, plan, attacker, target, re)
	}
	return nil
}

// grapplingRider gives the condition a hit gives with no save, and notes the escape DC.
func (s *Service) grapplingRider(ctx context.Context, c *combatTx, target playdb.Combatant, hc *rules.ActionHitCondition, re *riderEvent) error {
	var given castHit
	if err := s.giveCondition(ctx, c, target, hc.ConditionKey, &given); err != nil {
		return err
	}
	re.ConditionKey, re.ConditionName, re.Given, re.EscapeDC = hc.ConditionKey, hc.ConditionNamePT, given.CondSet, conv.Int32(hc.EscapeDC)
	return nil
}

// savingRider has the server roll the target's save against the DC of the attack, as a spell's, opens the
// damage the save governs and gives the condition to a target that fails.
func (s *Service) savingRider(ctx context.Context, c *combatTx, m authz.Membership, plan rules.ActionPlan, attacker, target playdb.Combatant, re *riderEvent) error {
	cs, err := listCombatants(ctx, c)
	if err != nil {
		return err
	}
	sv, sp := plan.Save, spellOfPlan(plan)
	modes, err := s.spellModes(ctx, c, m, c.viewer(m, cs), cs, attacker, &playv1.CastSpellRequest{}, "")
	if err != nil {
		return err
	}
	c.castID = uuid.New().String()
	var hit castHit
	if err := s.spellSave(ctx, c, m, sp, attacker, target, coverView{}, &hit, modes); err != nil {
		return err
	}
	re.Duration, re.RepeatSave, re.Pending, re.More = sv.Duration, sv.RepeatSave, hit.Pending, hit.More
	if hit.Save == nil {
		return nil
	}
	re.save = hit.Save
	re.D20, re.Total, re.DC, re.countedRoll = hit.Save.D20, hit.Save.Total, hit.Save.DC, hit.Save.countedRoll
	re.Resisted, re.ResistLeft = hit.Save.Resisted, hit.Save.ResistedLeft
	if hit.Save.Saved {
		re.Result = riderSaved
		return nil
	}
	re.Result = riderFailed
	if sv.ConditionKey != "" {
		if err := s.giveCondition(ctx, c, target, sv.ConditionKey, &hit); err != nil {
			return err
		}
		re.ConditionKey, re.ConditionName, re.Given = sv.ConditionKey, sv.ConditionNamePT, hit.CondSet
	}
	return nil
}

// decorateCreatureRiders fills in the creature offers the master reads with what the stat block says: the
// attack's name, the condition and the DC. A player's copy has no creature offer.
func (s *Service) decorateCreatureRiders(ctx context.Context, m authz.Membership, out *playv1.Encounter) error {
	if m.Role != authz.RoleMaster {
		return nil
	}
	var content *rules.Content
	for _, o := range out.GetHitRiders() {
		if o.GetKind() != playv1.HitRiderKind_HIT_RIDER_KIND_CREATURE {
			continue
		}
		if content == nil {
			var err error
			if content, err = s.roster.RulesContent(ctx, nil, m.CampaignID); err != nil {
				return fmt.Errorf("read the content for the creature riders: %w", err)
			}
		}
		plan, ok := content.MonsterActionPlan(o.GetActionKey())
		if !ok {
			continue
		}
		o.ActionNamePt = nameOfPlan(plan)
		if sv := plan.Save; sv != nil {
			o.SaveDc, o.ConditionKey, o.ConditionNamePt = conv.Int32(sv.DC), sv.ConditionKey, sv.ConditionNamePT
		}
		if hc := plan.HitCondition; hc != nil {
			o.EscapeDc, o.ConditionKey, o.ConditionNamePt = conv.Int32(hc.EscapeDC), hc.ConditionKey, hc.ConditionNamePT
		}
	}
	return nil
}
