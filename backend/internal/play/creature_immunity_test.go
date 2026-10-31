package play

import (
	"context"
	"testing"

	playv1 "github.com/PuraFome/meuRPG/backend/gen/meurpg/play/v1"
	"github.com/PuraFome/meuRPG/backend/internal/play/playdb"
)

// TestMR042_ACreatureTakesNoConditionItsStatBlockIsImmuneTo: the immunities of a creature's stat
// block (SRD 5.1: the Black Pudding is immune to prone, not to poisoned) stop a condition where it is
// given, and the hit says why (the view test checks who reads it).
func TestMR042_ACreatureTakesNoConditionItsStatBlockIsImmuneTo(t *testing.T) {
	t.Parallel()
	a := newCasters(t)
	e := a.castersFight(t, 2)
	added := a.mustAddMonsters(t, e, func(r *playv1.AddMonstersRequest) {
		r.CreatureKey, r.Count, r.Hidden = "monster:black-pudding", 1, new(false)
	})
	id := added.GetCombatantIds()[0]
	a.coreInTx(t, e.GetId(), func(ctx context.Context, c *combatTx) error {
		cs, err := c.q.ListCombatants(ctx, c.enc.ID)
		if err != nil {
			return err
		}
		var pudding playdb.Combatant
		for _, x := range cs {
			if x.ID == id {
				pudding = x
			}
		}
		for cond, want := range map[string]bool{"condition:prone": true, "condition:charmed": true, "condition:poisoned": false} {
			got, err := c.immuneToCondition(ctx, pudding, cond)
			if err != nil || got != want {
				t.Errorf("immuneToCondition(black pudding, %s) = %v, %v; want %v", cond, got, err, want)
			}
		}
		refused := &castHit{}
		if err := a.h.svc.giveCondition(ctx, c, pudding, "condition:prone", refused); err != nil {
			return err
		}
		if refused.CondSet || refused.NoEffectWhy != whyImmune {
			t.Errorf("prone on the pudding: set %v, why %q; want refused with %q", refused.CondSet, refused.NoEffectWhy, whyImmune)
		}
		given := &castHit{}
		if err := a.h.svc.giveCondition(ctx, c, pudding, "condition:poisoned", given); err != nil {
			return err
		}
		if !given.CondSet || given.NoEffectWhy != "" { // the positive control
			t.Errorf("poisoned on the pudding: set %v, why %q; want given", given.CondSet, given.NoEffectWhy)
		}
		return nil
	})
}
