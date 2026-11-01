package play

import (
	"context"
	"errors"

	playv1 "github.com/PuraFome/meuRPG/backend/gen/meurpg/play/v1"
	"github.com/PuraFome/meuRPG/backend/internal/platform/conv"
	"github.com/PuraFome/meuRPG/backend/internal/play/playdb"
	"github.com/PuraFome/meuRPG/backend/internal/rules"
)

// The limits of a stat block's actions (rules.ActionUsage): a Recharge action, an action with uses a day and
// one that recharges after a rest. What a creature spent of them is kept in creature_spent (combat_spent.go),
// only when spent. Only a creature of a stat block has them.
//
// A Multiattack is no action that is spent: it is a routine, and each of its steps is made with its own call
// (RollAttack for an attack, UseCreatureAction for a step that asks a saving throw or rolls nothing). A step
// that is a limited action is spent and refused as the action is, so the routine needs nothing of its own.

// limitKeyOf is the key under which a limited action's spend is kept, "" for an action that is not limited.
func limitKeyOf(plan rules.ActionPlan) string {
	switch plan.Usage.Kind {
	case rules.UsageRecharge:
		return spentRechargePrefix + plan.Key
	case rules.UsagePerDay:
		return spentUsePrefix + plan.Key
	case rules.UsageRest:
		return spentRestPrefix + plan.Key
	}
	return ""
}

// limitUsed is how many uses of the action the creature spent (a Recharge action: 1 when spent).
func limitUsed(plan rules.ActionPlan, who playdb.Combatant) int {
	return parseSpent(who.CreatureSpent).count(limitKeyOf(plan))
}

// limitSpent says the creature cannot use the action now.
func limitSpent(plan rules.ActionPlan, who playdb.Combatant) bool {
	return plan.Usage.Limited() && limitUsed(plan, who) >= plan.Usage.UsesAllowed()
}

// checkActionLimit refuses a limited action the creature has spent. Nothing changes.
// The master and the player who controls the creature read why; no other player can try the creature's action.
func checkActionLimit(plan rules.ActionPlan, who playdb.Combatant) error {
	if !limitSpent(plan, who) {
		return nil
	}
	if plan.Usage.Kind == rules.UsageRecharge {
		return errEncounter(playv1.EncounterBlockedReason_ENCOUNTER_BLOCKED_REASON_CREATURE_ACTION_RECHARGING, "the action has not recharged")
	}
	return errEncounter(playv1.EncounterBlockedReason_ENCOUNTER_BLOCKED_REASON_CREATURE_ACTION_NO_USES_LEFT, "the action has no uses left")
}

// spendActionLimit spends one use of a limited action in the transaction of the use and returns the key the
// event keeps for its undo ("" for an action that is not limited). A spend that does not fit in the creature's
// column is refused as an action the creature cannot make; nothing changes.
func spendActionLimit(ctx context.Context, c *combatTx, plan rules.ActionPlan, who playdb.Combatant) (string, error) {
	key := limitKeyOf(plan)
	if key == "" {
		return "", nil
	}
	if _, err := bumpCreatureSpent(ctx, c.q, who.ID, key, 1); err != nil {
		if errors.Is(err, errCreatureSpentFull) {
			return "", unavailable("what the creature spent does not fit")
		}
		return "", err
	}
	return key, nil
}

// limitBack gives back the use an event spent (its undo): the action is available again, or has the use back.
func (u *undoer) limitBack(ctx context.Context, ev actionEvent) error {
	if ev.LimitKey == "" {
		return nil
	}
	if _, here := u.find(ev.Actor); !here {
		return nil // a creature that left the combat has nothing to put back
	}
	if _, err := bumpCreatureSpent(ctx, u.c.q, ev.Actor, ev.LimitKey, -1); err != nil {
		return err
	}
	return nil
}

// castLimitBack is limitBack as a step of the undo of a cast.
func (u *undoer) castLimitBack(ctx context.Context, _ playdb.Combatant, ev actionEvent) error {
	return u.limitBack(ctx, ev)
}

// limitedActionsOf are the limited actions of a stat block, in its order.
func limitedActionsOf(p *rules.MonsterPlan) []rules.ActionPlan {
	var out []rules.ActionPlan
	for _, a := range p.Actions {
		if a.Usage.Limited() {
			out = append(out, a)
		}
	}
	return out
}

// limitedActionsView is the copy of the limited actions of a creature with what it has spent of them, for the
// master and the player who controls the creature. No other player gets it (RN-20, RN-29).
func limitedActionsView(actions []rules.ActionPlan, who playdb.Combatant) []*playv1.LimitedAction {
	var out []*playv1.LimitedAction
	for _, a := range actions {
		allowed := a.Usage.UsesAllowed()
		left := usesLeft(allowed, limitUsed(a, who))
		out = append(out, &playv1.LimitedAction{
			ActionKey: a.Key, Name: a.Name, NamePt: a.NamePT, Kind: a.Usage.Kind, Usage: a.Usage.Text,
			Available: left > 0, UsesAllowed: conv.Int32(allowed), UsesLeft: conv.Int32(left),
		})
	}
	return out
}
