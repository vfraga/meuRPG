package play

import (
	"context"
	"errors"
	"fmt"

	"connectrpc.com/connect"

	"github.com/PuraFome/meuRPG/backend/internal/play/link"
	"github.com/PuraFome/meuRPG/backend/internal/play/playdb"
	"github.com/PuraFome/meuRPG/backend/internal/rules"
	"github.com/PuraFome/meuRPG/backend/internal/rules/combat"
)

// monsterAttack is what the attack roll of a monster adds to RollAttack: the limit of its
// action (a recharging web), the Multiattack routine it counts down, and the legendary action
// it may be taken as. RollAttack calls start before the roll and finish after it.
type monsterAttack struct {
	mon       *monsterRef
	action    rules.ActionPlan
	state     combat.MonsterState
	before    combat.MonsterState
	legendary bool
	cost      int
}

// startMonsterAttack checks and spends what a monster's attack costs besides the action: nil
// when the attacker is not a monster or the attack is not an action of its stat block (a
// cantrip). A legendary attack (legendaryKey) is taken off turn and spends legendary actions
// instead of the action; any other attack uses up its limit (Recharge, x/day) and counts as one
// attack of the Multiattack routine of the turn.
func (s *Service) startMonsterAttack(ctx context.Context, c *combatTx, attacker playdb.Combatant, sheet link.Sheet, attackKey, legendaryKey string) (*monsterAttack, error) {
	if sheet.MonsterKey == "" {
		if legendaryKey != "" {
			return nil, errNotAMonster()
		}
		return nil, nil
	}
	mon, err := s.monsterOf(ctx, c.tx, c.session.CampaignID, attacker)
	if err != nil || mon == nil {
		return nil, err
	}
	ap, ok := mon.plan.Action(attackKey)
	if !ok || ap.Attack == nil {
		return nil, nil
	}
	st, err := monsterStateOf(attacker)
	if err != nil {
		return nil, err
	}
	ma := &monsterAttack{mon: mon, action: ap, state: st, before: st.Clone()}
	if legendaryKey == "" {
		if err := ma.state.Spend(ap.Usage, ap.Key); err != nil {
			return nil, limitError(err)
		}
		ma.state.CountAttack(mon.plan, ap.Key)
		return ma, nil
	}
	if mon.plan.Legendary == nil {
		return nil, connect.NewError(connect.CodeInvalidArgument, errors.New("the creature has no legendary actions"))
	}
	for _, o := range mon.plan.Legendary.Options {
		if o.Key != legendaryKey {
			continue
		}
		if o.ActionKey != attackKey {
			return nil, connect.NewError(connect.CodeInvalidArgument, errors.New("legendary_option_key does not make this attack"))
		}
		if err := s.legendaryGate(c, attacker, st, mon.plan, o.Cost); err != nil {
			return nil, err
		}
		if err := ma.state.SpendLegendary(mon.plan, o.Cost); err != nil {
			return nil, err
		}
		ma.state.Offer = nil // one action for each offer
		ma.legendary, ma.cost = true, o.Cost
		return ma, nil
	}
	return nil, connect.NewError(connect.CodeInvalidArgument, errors.New("legendary_option_key is not a legendary action of the creature"))
}

// finishMonsterAttack saves what the attack spent and writes it into the attack's event.
func (s *Service) finishMonsterAttack(ctx context.Context, c *combatTx, attacker playdb.Combatant, ma *monsterAttack, hit bool, critical bool, target playdb.Combatant, hasDamage bool, made *actionEvent) error {
	if ma == nil {
		return nil
	}
	if !statesEqual(ma.before, ma.state) {
		if err := saveMonsterState(ctx, c, attacker, ma.state); err != nil {
			return err
		}
		if made.Monster == nil {
			made.Monster = &monsterEvent{}
		}
		made.Monster.Before = &ma.before
	}
	if ma.legendary {
		if made.Monster == nil {
			made.Monster = &monsterEvent{}
		}
		made.Monster.Cost = clamp32(ma.cost, 0, maxLegendaryCost)
	}
	// A hit with no damage to roll has nothing to wait for: its grapple or saving throw is now.
	if hit && !hasDamage {
		var effects actionEvent
		if err := s.monsterHitEffects(ctx, c, ma.action.Key, critical, attacker, target, &effects); err != nil {
			return fmt.Errorf("give the effects of a hit: %w", err)
		}
		if effects.Monster != nil {
			if made.Monster == nil {
				made.Monster = &monsterEvent{}
			}
			made.Monster.Rider, made.Monster.Conds, made.Monster.Extras = effects.Monster.Rider, effects.Monster.Conds, effects.Monster.Extras
		}
	}
	return nil
}
