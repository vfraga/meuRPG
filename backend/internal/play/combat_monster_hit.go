package play

import (
	"context"
	"fmt"
	"math"
	"slices"
	"strings"
	"uuid"

	playv1 "github.com/PuraFome/meuRPG/backend/gen/meurpg/play/v1"
	"github.com/PuraFome/meuRPG/backend/internal/play/playdb"
	"github.com/PuraFome/meuRPG/backend/internal/rules"
	"github.com/PuraFome/meuRPG/backend/internal/rules/combat"
)

// What a monster's hit gives besides the damage of the attack's first part (SRD 5.1,
// "Monsters": actions; the data's attack actions): the other damage parts (a dragon's bite
// plus fire), a saving throw asked of the target (the ghoul's claws paralyze, a spider's poison
// deals more damage), and a condition with no save (a grapple). The hit is settled when its
// first damage is rolled, not when the attack roll lands: a Shield that stops the hit
// stops all of it, and an undo of the attack roll takes none of this away because none of it
// had happened.

// afterMonsterHit runs inside RollDamage's transaction, once the first damage part of a
// monster's attack is rolled. It opens the pending damage of every other part, rolls the
// rider's saving throw for the target and gives the condition a failed save or a grapple gives.
// It writes what it did into the damage roll's event (made.Monster) for the log and the undo.
func (s *Service) afterMonsterHit(ctx context.Context, c *combatTx, p playdb.PendingDamage, attacker, target playdb.Combatant, made *actionEvent) error {
	if p.CastID != nil || p.Healing {
		return nil
	}
	return s.monsterHitEffects(ctx, c, p.AttackKey, p.Critical, attacker, target, made)
}

// monsterHitEffects is afterMonsterHit for the hit of the attack with the key: what RollDamage runs
// once the first part is rolled, and RollAttack for an attack that has no damage (a grapple's
// tendril), whose hit has no damage roll to wait for.
func (s *Service) monsterHitEffects(ctx context.Context, c *combatTx, attackKey string, critical bool, attacker, target playdb.Combatant, made *actionEvent) error {
	if attacker.Kind != kindNPC || !strings.Contains(attackKey, "#") {
		return nil
	}
	mon, err := s.monsterOf(ctx, c.tx, c.session.CampaignID, attacker)
	if err != nil || mon == nil {
		return err
	}
	ap, ok := mon.plan.Action(attackKey)
	if !ok || ap.Attack == nil {
		return nil
	}
	ev := &monsterEvent{}
	for _, part := range ap.Damage[min(1, len(ap.Damage)):] {
		extra, err := s.openMonsterPart(ctx, c, attacker, target, attackKey, part, critical, false)
		if err != nil {
			return err
		}
		ev.Extras = append(ev.Extras, extra.ID)
	}
	if sv := ap.Save; sv != nil && sv.OnHit {
		rider, err := s.rollRider(ctx, c, attacker, target, attackKey, sv, ev)
		if err != nil {
			return err
		}
		ev.Rider = rider
	}
	if hc := ap.HitCondition; hc != nil && sizeAllows(target.Size, hc.MaxSize) {
		added, immune, before, err := s.addCondition(ctx, c, target, hc.ConditionKey)
		if err != nil {
			return err
		}
		if ev.Rider == nil {
			ev.Rider = &riderEv{}
		}
		ev.Rider.EscapeDC = clamp32(hc.EscapeDC, 0, 100)
		switch {
		case added:
			ev.Rider.Condition = hc.ConditionKey
			ev.Conds = append(ev.Conds, condChange{Target: target.ID, Before: before})
		case immune:
			ev.Rider.Immune = true
		}
	}
	if len(ev.Extras) > 0 || ev.Rider != nil {
		made.Monster = ev
	}
	return nil
}

// sizeAllows says whether a target of the size is not above the biggest the attack affects
// ("Large or smaller"); an empty maximum allows any.
func sizeAllows(size, largest string) bool {
	if largest == "" {
		return true
	}
	word := strings.ToUpper(size[:min(1, len(size))]) + strings.ToLower(size[min(1, len(size)):])
	if word == "" {
		word = "Medium"
	}
	return rules.SizeAtMost(word, largest)
}

// rollRider rolls the saving throw a hit asks of its target and opens the damage and gives the
// condition it leaves, as an area spell's save does: all the damage for a failure, half of it
// when the stat block says a success takes half. The d20 is the app's.
func (s *Service) rollRider(ctx context.Context, c *combatTx, attacker, target playdb.Combatant, key string, sv *rules.ActionSave, ev *monsterEvent) (*riderEv, error) {
	save, err := s.saveOf(ctx, c.tx, c.session.CampaignID, target, string(sv.Ability))
	if err != nil {
		return nil, err
	}
	face, roll, err := s.d20(rollInput{inApp: true}, save.Bonus)
	if err != nil {
		return nil, err
	}
	saved := combat.SaveSucceeded(roll.Total, sv.DC)
	rider := &riderEv{Save: &saveRoll{
		D20: clamp32(face, 1, 20), Bonus: clamp32(save.Bonus, math.MinInt32, math.MaxInt32), Total: clamp32(roll.Total, math.MinInt32, math.MaxInt32),
		DC: clamp32(sv.DC, 0, math.MaxInt32), Saved: saved, Unknown: !save.Known, OnSuccess: sv.OnSuccess, Ability: string(sv.Ability),
	}}
	// A success takes none of the damage unless the stat block says half (or leaves it to the table).
	if takesNone := saved && sv.OnSuccess != "half" && sv.OnSuccess != "other"; !takesNone {
		for _, part := range sv.Damage {
			// The damage of a saving throw is not the attack's dice: a critical hit does not double it.
			pd, err := s.openMonsterPart(ctx, c, attacker, target, key, part, false, saved && sv.OnSuccess == "half")
			if err != nil {
				return nil, err
			}
			rider.Pending = append(rider.Pending, pd.ID)
		}
	}
	if !saved && sv.ConditionKey != "" {
		added, immune, before, err := s.addCondition(ctx, c, target, sv.ConditionKey)
		if err != nil {
			return nil, err
		}
		switch {
		case added:
			rider.Condition = sv.ConditionKey
			ev.Conds = append(ev.Conds, condChange{Target: target.ID, Before: before})
		case immune:
			rider.Immune = true
		}
	}
	return rider, nil
}

// openMonsterPart opens the pending damage of one damage part of a monster's hit, to be rolled
// by RollDamage: a critical hit rolls the dice of an attack's part twice (the table's rule).
// Each part is its own roll: a cast id of its own keeps RollDamage from settling two parts
// with one roll.
func (s *Service) openMonsterPart(ctx context.Context, c *combatTx, attacker, target playdb.Combatant, key string, part rules.ActionDamage, critical, half bool) (playdb.PendingDamage, error) {
	count, fixed := combat.CriticalDice(part.Dice, critical, criticalRuleOf(c.rules))
	castID := uuid.New().String()
	p, err := c.q.InsertPendingDamage(ctx, playdb.InsertPendingDamageParams{
		EncounterID: c.enc.ID, AttackerID: &attacker.ID, TargetID: target.ID, AttackKey: key, Status: pendingAwaitingRoll, Critical: critical,
		DiceCount: clamp32(count, 0, 100), CriticalMax: clamp32(fixed, 0, 10000), CriticalMaxRule: critical && c.rules.CriticalMaxPlusRoll,
		DiceSides: clamp32(part.Dice.Sides, 0, 100), DiceBonus: clamp32(part.Dice.Bonus, -1000, 1000),
		DamageType: part.TypeKey, CreatedAt: c.now, CastID: &castID, Half: half,
	})
	if err != nil {
		return playdb.PendingDamage{}, fmt.Errorf("open the pending damage: %w", err)
	}
	return p, nil
}

// monsterFollowUps are the pending damages a damage roll opened for the rest of a monster's hit,
// as the caller may see them.
func (s *Service) monsterFollowUps(ctx context.Context, res combatResult, ev actionEvent, v combatViewer) ([]*playv1.PendingDamage, error) {
	if ev.Monster == nil {
		return nil, nil
	}
	ids := slices.Clone(ev.Monster.Extras)
	if ev.Monster.Rider != nil {
		ids = append(ids, ev.Monster.Rider.Pending...)
	}
	var out []*playv1.PendingDamage
	for _, id := range ids {
		p, err := s.pendingFor(ctx, res, id, v)
		if err != nil {
			return nil, err
		}
		if p != nil {
			out = append(out, p)
		}
	}
	return out, nil
}
