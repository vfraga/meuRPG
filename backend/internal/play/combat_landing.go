package play

import (
	"context"
	"fmt"
	"math"
	"time"

	playv1 "github.com/PuraFome/meuRPG/backend/gen/meurpg/play/v1"
	"github.com/PuraFome/meuRPG/backend/internal/platform/conv"
	"github.com/PuraFome/meuRPG/backend/internal/play/live"
	"github.com/PuraFome/meuRPG/backend/internal/play/playdb"
	"github.com/PuraFome/meuRPG/backend/internal/rules/combat"
)

// Where a damage or a heal lands on its target, and the concentration save it owes.

// afterResistance is the damage that lands on the target once its resistances, vulnerabilities and immunities to the
// damage type are applied, after the bonuses and the half of a saving throw (SRD 5.1): settleDamage's amount.
func (s *Service) afterResistance(ctx context.Context, c *combatTx, target playdb.Combatant, damageType string, amount int32) (int32, error) {
	if !holdsHP(target) {
		return amount, nil // a character's damage waits for the master: nothing here changes what lands
	}
	_, after, err := s.settleDamage(ctx, c, target, damageType, amount)
	return after, err
}

// settleDamage takes a damage that is no attack (a zone's, a trap's, an effect's) through the same modifiers an attack's
// damage goes through (settleLanding: the creature's own, its lasting effects', a Silence's) and answers the steps and what
// lands. Only a target that takes the damage at once is adjusted; for a player's character the steps are the preview the
// master applies from, and the amount is the one given.
func (s *Service) settleDamage(ctx context.Context, c *combatTx, target playdb.Combatant, damageType string, amount int32) (landing, int32, error) {
	if damageType == "" || amount <= 0 {
		return landing{}, amount, nil
	}
	rows, err := c.q.ListCombatantStates(ctx, c.enc.ID)
	if err != nil {
		return landing{}, 0, fmt.Errorf("list the states: %w", err)
	}
	steps, err := s.settleLanding(ctx, c, statesOf(rows), target, map[string]int{damageType: int(amount)}, combat.Origin{})
	if err != nil {
		return landing{}, 0, err
	}
	if holdsHP(target) && steps.changed {
		return steps, steps.amount, nil
	}
	return steps, amount, nil
}

// landDamage puts a rolled damage or heal on its target, as far as the target's
// kind lets it land at once. An NPC takes a damage at once: temporary hit points
// first, then the hit points, defeated at 0 (and a heal puts it back above 0).
// A heal lands at once on a player's character too, up to its maximum, through
// the vitals (a character healed from 0 gets up and its death saves reset). A
// damage on a player's character waits for the master: nothing lands. The hit
// says what changed, with what an undo needs.
func (s *Service) landDamage(ctx context.Context, c *combatTx, p playdb.PendingDamage, target playdb.Combatant, amount int32) (damageHit, *playv1.CharacterVitals, error) {
	hit := damageHit{Pending: p.ID, Target: target.ID, Amount: amount, Half: p.Half}
	switch {
	case p.Healing:
		healed, vit, err := s.healCombatant(ctx, c, target, amount)
		healed.Pending, healed.Half = p.ID, p.Half
		return healed, vit, err
	case holdsHP(target): // an NPC and a creature take a damage at once
		dmg := combat.ApplyDamage(int(conv.OrZero(target.HpCurrent)), int(conv.OrZero(target.HpTemp)), int(amount))
		after := hpState{HP: clamp32(dmg.HP, 0, math.MaxInt32), Temp: clamp32(dmg.TempHP, 0, math.MaxInt32), Defeated: dmg.HP == 0}
		if err := c.q.SetCombatantHitPoints(ctx, playdb.SetCombatantHitPointsParams{ID: target.ID, HpCurrent: &after.HP, HpTemp: &after.Temp, Defeated: after.Defeated}); err != nil {
			return hit, nil, fmt.Errorf("apply the damage: %w", err)
		}
		before := hpOf(target)
		hit.Before, hit.After, hit.Applied = &before, &after, true
		hit.ConcentrationDC = concentrationDC(target, amount-clamp32(dmg.Absorbed, 0, math.MaxInt32))
	}
	return hit, nil, nil
}

// healCombatant heals a combatant: an NPC's hit points, up to its maximum (it
// comes back above 0), or a player's character's through the vitals, up to its
// maximum (healed from 0 it gets up and its death saves reset, RN-03). The hit
// says how much it regained, with what an undo needs; the vitals are the
// character's after.
func (s *Service) healCombatant(ctx context.Context, c *combatTx, target playdb.Combatant, amount int32) (damageHit, *playv1.CharacterVitals, error) {
	hit := damageHit{Target: target.ID, Applied: true}
	if holdsHP(target) {
		before := hpOf(target)
		r := combat.ApplyHeal(int(before.HP), int(conv.OrZero(target.HpMax)), int(amount))
		after := hpState{HP: clamp32(r.HP, 0, math.MaxInt32), Temp: before.Temp, Defeated: r.HP == 0}
		if err := c.q.SetCombatantHitPoints(ctx, playdb.SetCombatantHitPointsParams{ID: target.ID, HpCurrent: &after.HP, HpTemp: &after.Temp, Defeated: after.Defeated}); err != nil {
			return hit, nil, fmt.Errorf("apply the heal: %w", err)
		}
		hit.Before, hit.After, hit.Amount = &before, &after, clamp32(r.Healed, 0, math.MaxInt32)
		return hit, nil, nil
	}
	now, err := s.vitals.GetVitalsTx(ctx, c.tx, c.session.CampaignID, target.CharacterID)
	if err != nil {
		return hit, nil, err
	}
	if w := now.GetWildShape(); w != nil {
		// In a beast form the healing is the beast's (MR-037).
		r := combat.ApplyHeal(int(w.GetHitPointsCurrent()), int(w.GetHitPointsMax()), int(amount))
		hp := clamp32(r.HP, 0, math.MaxInt32)
		before, after, err := s.vitalsOf(ctx, c, target.CharacterID, &playv1.AdjustCharacterVitalsRequest{WildShapeHitPointsCurrent: &hp})
		if err != nil {
			return hit, nil, err
		}
		hit.DeathBefore = deathOf(target)
		hit.Before, hit.After = new(hpStateOf(before)), new(hpStateOf(after))
		hit.Amount = clamp32(r.Healed, 0, math.MaxInt32)
		return hit, after, nil
	}
	r := combat.ApplyHeal(int(now.GetHitPointsCurrent()), int(now.GetHitPointsMax()), int(amount))
	hp := clamp32(r.HP, 0, math.MaxInt32)
	before, after, err := s.vitalsOf(ctx, c, target.CharacterID, &playv1.AdjustCharacterVitalsRequest{HitPointsCurrent: &hp})
	if err != nil {
		return hit, nil, err
	}
	hit.DeathBefore = deathOf(target)
	hit.Before = new(hpStateOf(before))
	hit.After = new(hpStateOf(after))
	hit.Amount = clamp32(r.Healed, 0, math.MaxInt32)
	return hit, after, nil
}

// concentrationDC is the Constitution save DC to keep concentrating after a
// combatant took damage (RN-22): 0 when it does not concentrate or took none.
// The damage it takes is what temporary hit points did not absorb, even when it
// drops the combatant below 0.
func concentrationDC(target playdb.Combatant, taken int32) int32 {
	if target.ConcentrationSpell == nil || taken <= 0 {
		return 0
	}
	return clamp32(combat.ConcentrationDC(int(taken)), 0, math.MaxInt32)
}

// takenBy is what a damage of amount cost a combatant: what temporary hit points
// did not absorb, even when it drops the combatant below 0.
func takenBy(amount int32, before, after *hpState) int32 {
	if before == nil || after == nil {
		return max(amount, 0)
	}
	return max(amount-max(before.Temp-after.Temp, 0), 0)
}

// castConcentrationDC notes what a damage of a spell cost its target (taken) and
// gives the Constitution save DC the target owes for the whole cast (RN-22): the
// SRD asks one save for each source of damage, and a spell is one source however
// many damage types it deals. It is 0 while another pending damage of the cast is
// still to be settled for the target, and when the target does not concentrate;
// the pending damage that settles last carries the DC of the sum of what the
// cast's damages cost the target.
func (s *Service) castConcentrationDC(ctx context.Context, c *combatTx, p playdb.PendingDamage, target playdb.Combatant, taken int32) (int32, error) {
	if err := c.q.SetPendingDamageTaken(ctx, playdb.SetPendingDamageTakenParams{ID: p.ID, Taken: &taken}); err != nil {
		return 0, fmt.Errorf("note what the damage cost: %w", err)
	}
	all, err := c.q.ListCastPendingDamages(ctx, playdb.ListCastPendingDamagesParams{EncounterID: c.enc.ID, CastID: p.CastID})
	if err != nil {
		return 0, fmt.Errorf("list the cast's pending damage: %w", err)
	}
	var total int32
	for _, o := range all {
		if o.TargetID != p.TargetID || o.Healing {
			continue
		}
		switch o.Status {
		case pendingApplied:
			total = clamp32(int(total)+int(conv.OrZero(o.Taken)), 0, math.MaxInt32)
		case pendingDiscarded:
		default:
			return 0, nil // another damage type of the cast has still to land on it
		}
	}
	return concentrationDC(target, total), nil
}

// resolvedAt is when a pending damage was settled: now for an applied one, not
// yet for one that waits for the master.
func resolvedAt(status string, c *combatTx) *time.Time {
	if status == pendingApplied {
		return &c.now
	}
	return nil
}

// publishVitals tells the master and the character's player the vitals
// changed; nothing when the change did not touch any.
func (s *Service) publishVitals(campaignID string, v *playv1.CharacterVitals) {
	if v == nil {
		return
	}
	s.send(campaignID, vitalsHint(v))
}

// vitalsHint is the hint that v changed, for the master and the character's player.
func vitalsHint(v *playv1.CharacterVitals) live.Event {
	return live.Event{
		Audience: vitalsAudience(v),
		Message: &playv1.WatchGameSessionResponse{Event: &playv1.WatchGameSessionResponse_VitalsChanged_{
			VitalsChanged: &playv1.WatchGameSessionResponse_VitalsChanged{Vitals: v},
		}},
	}
}
