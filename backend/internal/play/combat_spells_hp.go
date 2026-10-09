package play

import (
	"context"
	"errors"
	"fmt"
	"math"
	"slices"

	"connectrpc.com/connect"
	"github.com/jackc/pgx/v5"

	playv1 "github.com/PuraFome/meuRPG/backend/gen/meurpg/play/v1"
	"github.com/PuraFome/meuRPG/backend/internal/authz"
	"github.com/PuraFome/meuRPG/backend/internal/platform/dice"
	"github.com/PuraFome/meuRPG/backend/internal/play/link"
	"github.com/PuraFome/meuRPG/backend/internal/play/playdb"
	"github.com/PuraFome/meuRPG/backend/internal/rules"
	"github.com/PuraFome/meuRPG/backend/internal/rules/combat"
)

// Spells that read hit points (MR-014, RN-20, Etapa 8): Sono, Leque Cromático,
// the Palavras de Poder, Estabilizar and Cura Completa. The rules say
// what each one does (effects/spells.json, rules.SpellEffect) and do the
// arithmetic (combat.ResolvePool and the others, pure); this file reads the
// real hit points (an NPC's from the combat, a player's character's from its
// vitals), applies the result in the cast's transaction and records what it did,
// with what an undo needs, in the cast event. The conditions go through the same
// column the master's SetCombatantConditions writes.

// unconscious is the condition a pool spell skips.
const unconscious = "condition:unconscious"

// fxTarget is a target of the spell with the hit points it has now.
type fxTarget struct {
	c       playdb.Combatant
	hp, max int
	temp    int
}

// readFxTarget reads a target's hit points: an NPC's or a creature's from its
// combatant row, a player's character's from its vitals. A druid in a beast form
// has the beast's: its separate pool, without temporary hit points, which is what
// damage and healing use (RN-02).
func (s *Service) readFxTarget(ctx context.Context, c *combatTx, t playdb.Combatant) (fxTarget, error) {
	if holdsHP(t) {
		return fxTarget{c: t, hp: int(num(t.HpCurrent)), max: int(num(t.HpMax)), temp: int(num(t.HpTemp))}, nil
	}
	v, err := s.vitals.GetVitalsTx(ctx, c.tx, c.session.CampaignID, t.CharacterID)
	if err != nil {
		return fxTarget{}, err
	}
	if w := v.GetWildShape(); w != nil {
		return fxTarget{c: t, hp: int(w.GetHitPointsCurrent()), max: int(w.GetHitPointsMax())}, nil
	}
	return fxTarget{c: t, hp: int(v.GetHitPointsCurrent()), max: int(v.GetHitPointsMax()), temp: int(v.GetHitPointsTemporary())}, nil
}

// poolRoll rolls the dice of a pool: the server rolls them, or the player types
// the sum of the physical dice (RN-18, checked by the caller).
func (s *Service) poolRoll(d link.Dice, in rollInput) (dice.Result, error) {
	expr := dice.Expr{Count: d.Count, Sides: d.Sides}
	if in.inApp {
		roll, err := dice.Roll(s.roller, expr)
		if err != nil {
			return dice.Result{}, fmt.Errorf("roll the pool: %w", err)
		}
		return roll, nil
	}
	roll, err := dice.Physical(expr, in.typed)
	if err != nil {
		return dice.Result{}, connect.NewError(connect.CodeInvalidArgument, fmt.Errorf("pool_sum must be %d to %d", expr.Count, expr.Count*expr.Sides))
	}
	return roll, nil
}

// mustRollPool checks the roll of a pool spell: it needs the app's roll or the
// sum of physical dice (not a d20 face), and a player's way of rolling must
// agree with the campaign's dice setting (RN-18); the master rolls either way.
func (s *Service) mustRollPool(ctx context.Context, tx pgx.Tx, m authz.Membership, in rollInput, rolled bool) error {
	if !rolled || (!in.inApp && !in.pool) {
		return connect.NewError(connect.CodeInvalidArgument, errors.New("set roll_in_app or pool_sum: this spell rolls a pool of dice"))
	}
	return s.mustRollThisWay(ctx, tx, m, in)
}

// castHPSpell applies a spell that reads hit points to its targets, in the order
// the caster listed them, and fills the cast event with the kind, the pool roll
// and what it did to each. It returns the vitals of the player's characters it
// changed, to publish.
func (s *Service) castHPSpell(ctx context.Context, c *combatTx, sp link.Spell, targs []playdb.Combatant, in rollInput, made *actionEvent) ([]castHit, []*playv1.CharacterVitals, error) {
	fx := sp.HP
	made.FxKind, made.FxCondition = fx.Kind, fx.Condition
	var vitals []*playv1.CharacterVitals
	targets := make([]fxTarget, len(targs))
	hits := make([]castHit, len(targs))
	for i, t := range targs {
		var err error
		if targets[i], err = s.readFxTarget(ctx, c, t); err != nil {
			return nil, nil, err
		}
		hits[i] = castHit{Target: t.ID, HPBefore: clamp32(targets[i].hp, 0, math.MaxInt32), Fx: fxUnaffected}
	}

	switch fx.Kind {
	case rules.SpellKindHPPool:
		roll, err := s.poolRoll(fx.Pool, in)
		if err != nil {
			return nil, nil, err
		}
		made.DiceCount, made.DiceSides, made.Faces = clamp32(fx.Pool.Count, 0, 100), clamp32(fx.Pool.Sides, 0, 100), faces32(roll.Faces)
		made.Total, made.Physical = clamp32(roll.Total, 0, math.MaxInt32), roll.Physical
		creatures := make([]combat.HPCreature, len(targets))
		for i, t := range targets {
			creatures[i] = combat.HPCreature{HP: t.hp, Unconscious: slices.Contains(t.c.Conditions, unconscious)}
		}
		for place, step := range combat.ResolvePool(roll.Total, creatures) {
			h := &hits[step.Index]
			h.Order, h.Left = clamp32(place+1, 0, 100), clamp32(step.Left, 0, math.MaxInt32)
			if !step.Affected {
				h.FxReason = step.Reason
				continue
			}
			h.Fx = fxAffected
			if err := s.giveCondition(ctx, c, targets[step.Index].c, fx.Condition, h); err != nil {
				return nil, nil, err
			}
		}

	case rules.SpellKindHPThreshold:
		made.FxLimit = clamp32(fx.Threshold, 0, math.MaxInt32)
		for i, t := range targets {
			h := &hits[i]
			if !combat.ResolveThreshold(fx.Threshold, t.hp) {
				h.FxReason = fxAboveLimit
				continue
			}
			h.Fx = fxAffected
			var err error
			if fx.Dies {
				var v *playv1.CharacterVitals
				if v, err = s.dropToZero(ctx, c, t, h); v != nil {
					vitals = append(vitals, v)
				}
			} else {
				err = s.giveCondition(ctx, c, t.c, fx.Condition, h)
			}
			if err != nil {
				return nil, nil, err
			}
		}

	case rules.SpellKindZeroHP:
		for i, t := range targets {
			h := &hits[i]
			if !combat.ResolveZeroHP(t.hp) {
				h.FxReason = fxNotAtZero
				continue
			}
			h.Fx = fxAffected
			if t.c.Kind == kindPlayer { // an NPC at 0 is defeated and never a target
				h.DeathBefore = deathOf(t.c)
				if err := c.q.SetCombatantDeathSaves(ctx, playdb.SetCombatantDeathSavesParams{
					ID: t.c.ID, DeathSuccesses: 3, DeathFailures: 0, DeathSaveRolled: t.c.DeathSaveRolled, Defeated: t.c.Defeated,
				}); err != nil {
					return nil, nil, fmt.Errorf("make the character stable: %w", err)
				}
			}
		}

	case rules.SpellKindTempHP:
		roll, err := s.poolRoll(fx.Pool, in)
		if err != nil {
			return nil, nil, err
		}
		gained := roll.Total + fx.Amount
		made.DiceCount, made.DiceSides, made.Faces, made.Modifier = clamp32(fx.Pool.Count, 0, 100), clamp32(fx.Pool.Sides, 0, 100), faces32(roll.Faces), clamp32(fx.Amount, 0, math.MaxInt32)
		made.Total, made.Physical = clamp32(gained, 0, math.MaxInt32), roll.Physical
		for i, t := range targets {
			h := &hits[i]
			h.Fx = fxAffected
			v, err := s.giveTempHP(ctx, c, t, gained, h)
			if err != nil {
				return nil, nil, err
			}
			if v != nil {
				vitals = append(vitals, v)
			}
		}

	case rules.SpellKindMaxHP:
		for i, t := range targets {
			h := &hits[i]
			h.Fx = fxAffected
			v, err := s.raiseMaxHP(ctx, c, t, fx.Amount, h)
			if err != nil {
				return nil, nil, err
			}
			if v != nil {
				vitals = append(vitals, v)
			}
		}

	case rules.SpellKindFlatHeal:
		for i, t := range targets {
			h := &hits[i]
			h.Fx = fxAffected
			r := combat.ResolveFlatHeal(fx.Heal, t.hp, t.max, t.c.Conditions, fx.Ends)
			hit, v, err := s.healCombatant(ctx, c, t.c, clamp32(fx.Heal, 0, math.MaxInt32))
			if err != nil {
				return nil, nil, err
			}
			healed := hit.Amount
			h.Healed, h.Restore, h.DeathBefore = &healed, hit.Before, hit.DeathBefore
			if v != nil {
				vitals = append(vitals, v)
			}
			if len(r.Ended) > 0 {
				if err := s.setCondition(ctx, c, t.c, r.Conditions, h); err != nil {
					return nil, nil, err
				}
			}
		}

	default:
		return nil, nil, fmt.Errorf("spell %s: unknown hit point effect %q", sp.Key, fx.Kind)
	}
	return hits, vitals, nil
}

// Why a target was not affected, as a cast event stores it: a pool's reasons are
// combat.PoolSkipped and combat.PoolTooHigh.
const (
	fxAboveLimit = "above_limit"
	fxNotAtZero  = "not_at_zero"
)

// What Aid gave a target, as a cast event stores it.
const (
	gainMaximum   = "maximum"
	gainTemporary = "temporary"
	gainCurrent   = "current"
)

// giveCondition adds the condition to a target's conditions, when it does not
// have it and has room for it, and notes what was there for an undo.
func (s *Service) giveCondition(ctx context.Context, c *combatTx, t playdb.Combatant, key string, h *castHit) error {
	if key == "" || slices.Contains(t.Conditions, key) || len(t.Conditions) >= maxConditions {
		return nil
	}
	// A creature immune to a condition cannot suffer it (SRD 5.1, Monsters: condition immunities).
	if immune, err := s.conditionImmune(ctx, c.tx, c.session.CampaignID, t, key); err != nil || immune {
		return err
	}
	return s.setCondition(ctx, c, t, append(slices.Clone(t.Conditions), key), h)
}

// setCondition writes a target's conditions and notes the old ones for an undo.
func (s *Service) setCondition(ctx context.Context, c *combatTx, t playdb.Combatant, conditions []string, h *castHit) error {
	if err := c.q.SetCombatantConditions(ctx, playdb.SetCombatantConditionsParams{ID: t.ID, Conditions: nonNil(conditions)}); err != nil {
		return fmt.Errorf("set the conditions: %w", err)
	}
	h.CondSet, h.CondBefore = true, t.Conditions
	return s.endFormIfAsleep(ctx, c, t, conditions) // a druid put to sleep is itself again (SRD)
}

// dropToZero is Palavra de Poder Matar on a target: an NPC is defeated at 0 hit
// points; a player's character drops to 0 with three death save failures, and
// the master confirms the death with ConfirmDeath (RN-03: the engine never kills
// a character by itself). A druid in a beast form loses the beast's hit points
// instead: the form ends with nothing carried over and the druid keeps its own.
// It returns the character's vitals after.
func (s *Service) dropToZero(ctx context.Context, c *combatTx, t fxTarget, h *castHit) (*playv1.CharacterVitals, error) {
	zero := int32(0)
	if holdsHP(t.c) {
		before := hpOf(t.c)
		h.Restore = &before
		if err := c.q.SetCombatantHitPoints(ctx, playdb.SetCombatantHitPointsParams{ID: t.c.ID, HpCurrent: &zero, HpTemp: &zero, Defeated: true}); err != nil {
			return nil, fmt.Errorf("defeat the target: %w", err)
		}
		return nil, nil
	}
	now, err := s.vitals.GetVitalsTx(ctx, c.tx, c.session.CampaignID, t.c.CharacterID)
	if err != nil {
		return nil, err
	}
	if w := now.GetWildShape(); w != nil {
		before, after, err := s.damageBeast(ctx, c, t.c, now, w.GetHitPointsCurrent(), &actionEvent{})
		if err != nil {
			return nil, err
		}
		h.Restore = new(hpStateOf(before))
		return after, nil
	}
	before, after, err := s.vitalsOf(ctx, c, t.c.CharacterID, &playv1.AdjustCharacterVitalsRequest{HitPointsCurrent: &zero, HitPointsTemporary: &zero})
	if err != nil {
		return nil, err
	}
	h.Restore = new(hpStateOf(before))
	h.DeathBefore = deathOf(t.c)
	if err := c.q.SetCombatantDeathSaves(ctx, playdb.SetCombatantDeathSavesParams{
		ID: t.c.ID, DeathSuccesses: 0, DeathFailures: 3, DeathSaveRolled: t.c.DeathSaveRolled, Defeated: false,
	}); err != nil {
		return nil, fmt.Errorf("fail the death saves: %w", err)
	}
	return after, nil
}

// giveTempHP gives a target temporary hit points: they do not stack, so the target
// keeps the larger of what it had and what the spell gives (SRD 5.1). It notes the
// hit points before for an undo and what the target gained in h.Healed.
func (s *Service) giveTempHP(ctx context.Context, c *combatTx, t fxTarget, amount int, h *castHit) (*playv1.CharacterVitals, error) {
	temp := clamp32(max(amount, t.temp), 0, math.MaxInt32)
	gained := temp - clamp32(t.temp, 0, math.MaxInt32)
	h.Healed = &gained
	if holdsHP(t.c) {
		before := hpOf(t.c)
		h.Restore = &before
		if err := c.q.SetCombatantHitPoints(ctx, playdb.SetCombatantHitPointsParams{ID: t.c.ID, HpCurrent: &before.HP, HpTemp: &temp, Defeated: before.Defeated}); err != nil {
			return nil, fmt.Errorf("give the temporary hit points: %w", err)
		}
		return nil, nil
	}
	before, after, err := s.vitalsOf(ctx, c, t.c.CharacterID, &playv1.AdjustCharacterVitalsRequest{HitPointsTemporary: &temp})
	if err != nil {
		return nil, err
	}
	h.Restore = new(hpStateOf(before))
	h.DeathBefore = deathOf(t.c)
	return after, nil
}

// raiseMaxHP raises a target's maximum and current hit points by amount (Ajuda). An
// NPC's or a creature's maximum is on its combatant, so it rises (a target at 0 stays
// at 0: the master decides whether a fallen NPC is dead). A player's character's
// maximum is worked out from its sheet and has no place for a bonus, so the character
// gets the amount as temporary hit points, which absorb damage first; a character at
// 0 gets it as current hit points instead, because the spell's current hit points
// wake them up (SRD 5.1), and temporary ones would leave them dying.
func (s *Service) raiseMaxHP(ctx context.Context, c *combatTx, t fxTarget, amount int, h *castHit) (*playv1.CharacterVitals, error) {
	if !holdsHP(t.c) {
		if t.hp > 0 {
			h.Gain = gainTemporary
			return s.giveTempHP(ctx, c, t, amount, h)
		}
		h.Gain = gainCurrent
		hit, v, err := s.healCombatant(ctx, c, t.c, clamp32(amount, 0, math.MaxInt32))
		if err != nil {
			return nil, err
		}
		woke := hit.Amount
		h.Healed, h.Restore, h.DeathBefore = &woke, hit.Before, hit.DeathBefore
		return v, nil
	}
	rose := clamp32(amount, 0, math.MaxInt32)
	h.Healed, h.Gain = &rose, gainMaximum
	before, maxBefore := hpOf(t.c), num(t.c.HpMax)
	h.Restore, h.MaxBefore = &before, &maxBefore
	if err := c.q.SetCombatantHitPointsMax(ctx, playdb.SetCombatantHitPointsMaxParams{ID: t.c.ID, HpMax: new(maxBefore + rose)}); err != nil {
		return nil, fmt.Errorf("raise the maximum hit points: %w", err)
	}
	if before.HP > 0 {
		if err := c.q.SetCombatantHitPoints(ctx, playdb.SetCombatantHitPointsParams{ID: t.c.ID, HpCurrent: new(before.HP + rose), HpTemp: &before.Temp, Defeated: before.Defeated}); err != nil {
			return nil, fmt.Errorf("raise the current hit points: %w", err)
		}
	}
	return nil, nil
}
