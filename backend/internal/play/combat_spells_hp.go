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
	"github.com/PuraFome/meuRPG/backend/internal/platform/conv"
	"github.com/PuraFome/meuRPG/backend/internal/platform/dice"
	"github.com/PuraFome/meuRPG/backend/internal/play/link"
	"github.com/PuraFome/meuRPG/backend/internal/play/playdb"
	"github.com/PuraFome/meuRPG/backend/internal/rules"
	"github.com/PuraFome/meuRPG/backend/internal/rules/combat"
)

// Spells that read hit points (MR-014, RN-20): Sono, Leque Cromático,
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
	// bonus is the part of max that Ajuda added: a player's character's, from its
	// vitals; an NPC's and a creature's is on the combatant (c.HpMaxBonus).
	bonus int
}

// readFxTarget reads a target's hit points: an NPC's or a creature's from its
// combatant row, a player's character's from its vitals. A druid in a beast form
// has the beast's: its separate pool, without temporary hit points, which is what
// damage and healing use (RN-02).
func (s *Service) readFxTarget(ctx context.Context, c *combatTx, t playdb.Combatant) (fxTarget, error) {
	if holdsHP(t) {
		return fxTarget{c: t, hp: int(conv.OrZero(t.HpCurrent)), max: int(conv.OrZero(t.HpMax)), temp: int(conv.OrZero(t.HpTemp))}, nil
	}
	v, err := s.vitals.GetVitalsTx(ctx, c.tx, c.session.CampaignID, t.CharacterID)
	if err != nil {
		return fxTarget{}, err
	}
	if w := v.GetWildShape(); w != nil {
		return fxTarget{c: t, hp: int(w.GetHitPointsCurrent()), max: int(w.GetHitPointsMax()), bonus: int(v.GetHitPointsMaxBonus())}, nil
	}
	return fxTarget{c: t, hp: int(v.GetHitPointsCurrent()), max: int(v.GetHitPointsMax()), temp: int(v.GetHitPointsTemporary()), bonus: int(v.GetHitPointsMaxBonus())}, nil
}

// healsHitPoints says whether the spell gives hit points back: a healing, and Aid
// on a target at 0, which wakes it with current hit points.
func healsHitPoints(sp link.Spell) bool {
	return sp.Heal != nil || (sp.HP != nil && (sp.HP.Kind == rules.SpellKindFlatHeal || sp.HP.Kind == rules.SpellKindMaxHP))
}

// refuseTheDead refuses a healing aimed at a player's character that is dead: the
// master confirmed its death, it failed three death saves (SRD 5.1, "Dropping to 0
// Hit Points": three failures and the creature dies), or its character was marked
// dead. A dead creature can't regain hit points, so nothing is spent.
func (s *Service) refuseTheDead(ctx context.Context, c *combatTx, targs []playdb.Combatant) error {
	for _, t := range targs {
		if t.Kind != kindPlayer {
			continue
		}
		dead := t.Defeated || t.DeathFailures >= 3
		if !dead {
			_, err := s.vitals.GetVitalsTx(ctx, c.tx, c.session.CampaignID, t.CharacterID)
			dead = connect.CodeOf(err) == connect.CodeNotFound // marked dead: no longer an active character
		}
		if dead {
			return errEncounter(playv1.EncounterBlockedReason_ENCOUNTER_BLOCKED_REASON_TARGET_DEAD, "the target is dead and regains no hit points")
		}
	}
	return nil
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

// hpCast is what one hit point spell works on: its effect, the targets with the hit
// points they have now, the hit each one gets in the cast event, and the caster's roll.
type hpCast struct {
	fx      *link.HPEffect
	targets []fxTarget
	hits    []castHit
	in      rollInput
	made    *actionEvent
}

// hpKinds maps each kind of hit point effect to the function that applies it. The
// functions return the vitals of the player's characters they changed, to publish.
var hpKinds = map[string]func(*Service, context.Context, *combatTx, *hpCast) ([]*playv1.CharacterVitals, error){
	rules.SpellKindHPPool:      (*Service).hpPool,
	rules.SpellKindHPThreshold: (*Service).hpThreshold,
	rules.SpellKindZeroHP:      (*Service).hpZero,
	rules.SpellKindTempHP:      (*Service).hpTemp,
	rules.SpellKindMaxHP:       (*Service).hpMax,
	rules.SpellKindFlatHeal:    (*Service).hpFlatHeal,
}

// castHPSpell applies a spell that reads hit points to its targets, in the order
// the caster listed them, and fills the cast event with the kind, the pool roll
// and what it did to each. It returns the vitals of the player's characters it
// changed, to publish.
func (s *Service) castHPSpell(ctx context.Context, c *combatTx, sp link.Spell, targs []playdb.Combatant, in rollInput, made *actionEvent) ([]castHit, []*playv1.CharacterVitals, error) {
	fx := sp.HP
	made.FxKind, made.FxCondition = fx.Kind, fx.Condition
	targets, hits, err := s.readFxTargets(ctx, c, targs)
	if err != nil {
		return nil, nil, err
	}
	apply, ok := hpKinds[fx.Kind]
	if !ok {
		return nil, nil, fmt.Errorf("spell %s: unknown hit point effect %q", sp.Key, fx.Kind)
	}
	vitals, err := apply(s, ctx, c, &hpCast{fx: fx, targets: targets, hits: hits, in: in, made: made})
	if err != nil {
		return nil, nil, err
	}
	return hits, vitals, nil
}

// readFxTargets reads the hit points of every target and starts a hit for each: unaffected
// until the effect says otherwise.
func (s *Service) readFxTargets(ctx context.Context, c *combatTx, targs []playdb.Combatant) ([]fxTarget, []castHit, error) {
	targets := make([]fxTarget, len(targs))
	hits := make([]castHit, len(targs))
	for i, t := range targs {
		var err error
		if targets[i], err = s.readFxTarget(ctx, c, t); err != nil {
			return nil, nil, err
		}
		hits[i] = castHit{Target: t.ID, HPBefore: clamp32(targets[i].hp, 0, math.MaxInt32), Fx: fxUnaffected}
	}
	return targets, hits, nil
}

// hpPool is Sono and Leque Cromático: a pool of dice affects the creatures in ascending
// order of hit points, as many as it covers (RN-18).
func (s *Service) hpPool(ctx context.Context, c *combatTx, k *hpCast) ([]*playv1.CharacterVitals, error) {
	fx, made := k.fx, k.made
	roll, err := s.poolRoll(fx.Pool, k.in)
	if err != nil {
		return nil, err
	}
	made.DiceCount, made.DiceSides, made.Faces = clamp32(fx.Pool.Count, 0, 100), clamp32(fx.Pool.Sides, 0, 100), faces32(roll.Faces)
	made.Total, made.Physical = clamp32(roll.Total, 0, math.MaxInt32), roll.Physical
	creatures := make([]combat.HPCreature, len(k.targets))
	for i, t := range k.targets {
		creatures[i] = combat.HPCreature{HP: t.hp, Unconscious: slices.Contains(t.c.Conditions, unconscious)}
	}
	for place, step := range combat.ResolvePool(roll.Total, creatures) {
		h := &k.hits[step.Index]
		h.Order, h.Left = clamp32(place+1, 0, 100), clamp32(step.Left, 0, math.MaxInt32)
		if !step.Affected {
			h.FxReason = step.Reason
			continue
		}
		h.Fx = fxAffected
		if err := s.giveCondition(ctx, c, k.targets[step.Index].c, fx.Condition, h); err != nil {
			return nil, err
		}
	}
	return nil, nil
}

// hpThreshold is Palavra de Poder Atordoar and Matar: a creature with no more hit points than
// the limit is stunned or dies.
func (s *Service) hpThreshold(ctx context.Context, c *combatTx, k *hpCast) ([]*playv1.CharacterVitals, error) {
	fx := k.fx
	k.made.FxLimit = clamp32(fx.Threshold, 0, math.MaxInt32)
	var vitals []*playv1.CharacterVitals
	for i, t := range k.targets {
		h := &k.hits[i]
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
			return nil, err
		}
	}
	return vitals, nil
}

// hpZero is Poupar os Moribundos: a character at 0 hit points becomes stable.
func (s *Service) hpZero(ctx context.Context, c *combatTx, k *hpCast) ([]*playv1.CharacterVitals, error) {
	for i, t := range k.targets {
		h := &k.hits[i]
		if !combat.ResolveZeroHP(t.hp) {
			h.FxReason = fxNotAtZero
			continue
		}
		h.Fx = fxAffected
		if t.c.Kind == kindPlayer { // an NPC at 0 is defeated and never a target
			h.DeathBefore = deathOf(t.c)
			if err := c.q.SetCombatantDeathSaves(ctx, playdb.SetCombatantDeathSavesParams{
				ID: t.c.ID, DeathSuccesses: deathSaveCount, DeathFailures: 0, DeathSaveRolled: t.c.DeathSaveRolled, Defeated: t.c.Defeated,
			}); err != nil {
				return nil, fmt.Errorf("make the character stable: %w", err)
			}
		}
	}
	return nil, nil
}

// hpTemp is Vitalidade Falsa: the roll plus the fixed amount, as temporary hit points.
func (s *Service) hpTemp(ctx context.Context, c *combatTx, k *hpCast) ([]*playv1.CharacterVitals, error) {
	fx, made := k.fx, k.made
	roll, err := s.poolRoll(fx.Pool, k.in)
	if err != nil {
		return nil, err
	}
	gained := roll.Total + fx.Amount
	made.DiceCount, made.DiceSides, made.Faces, made.Modifier = clamp32(fx.Pool.Count, 0, 100), clamp32(fx.Pool.Sides, 0, 100), faces32(roll.Faces), clamp32(fx.Amount, 0, math.MaxInt32)
	made.Total, made.Physical = clamp32(gained, 0, math.MaxInt32), roll.Physical
	var vitals []*playv1.CharacterVitals
	for i, t := range k.targets {
		h := &k.hits[i]
		h.Fx = fxAffected
		v, err := s.giveTempHP(ctx, c, t, gained, h)
		if err != nil {
			return nil, err
		}
		if v != nil {
			vitals = append(vitals, v)
		}
	}
	return vitals, nil
}

// hpMax is Ajuda: it raises the maximum and the current hit points of each target.
func (s *Service) hpMax(ctx context.Context, c *combatTx, k *hpCast) ([]*playv1.CharacterVitals, error) {
	var vitals []*playv1.CharacterVitals
	for i, t := range k.targets {
		h := &k.hits[i]
		h.Fx = fxAffected
		v, err := s.raiseMaxHP(ctx, c, t, k.fx.Amount, h)
		if err != nil {
			return nil, err
		}
		if v != nil {
			vitals = append(vitals, v)
		}
	}
	return vitals, nil
}

// hpFlatHeal is Cura Completa: a fixed healing that also ends the conditions the spell names.
func (s *Service) hpFlatHeal(ctx context.Context, c *combatTx, k *hpCast) ([]*playv1.CharacterVitals, error) {
	fx := k.fx
	var vitals []*playv1.CharacterVitals
	for i, t := range k.targets {
		h := &k.hits[i]
		h.Fx = fxAffected
		r := combat.ResolveFlatHeal(fx.Heal, t.hp, t.max, t.c.Conditions, fx.Ends)
		hit, v, err := s.healCombatant(ctx, c, t.c, clamp32(fx.Heal, 0, math.MaxInt32))
		if err != nil {
			return nil, err
		}
		healed := hit.Amount
		h.Healed, h.Restore, h.DeathBefore = &healed, hit.Before, hit.DeathBefore
		if v != nil {
			vitals = append(vitals, v)
		}
		if len(r.Ended) > 0 {
			if err := s.setCondition(ctx, c, t.c, r.Conditions, h); err != nil {
				return nil, err
			}
		}
	}
	return vitals, nil
}

// Why a target was not affected, as a cast event stores it: a pool's reasons are
// combat.PoolSkipped and combat.PoolTooHigh.
const (
	fxAboveLimit = "above_limit"
	fxNotAtZero  = "not_at_zero"
)

// What Aid gave a target, as a cast event stores it: the maximum (the target stood)
// or the current hit points too (it was at 0 and woke up).
const (
	gainMaximum = "maximum"
	gainCurrent = "current"
)

// giveCondition adds the condition to a target's conditions, when it does not
// have it and has room for it, and notes what was there for an undo.
func (s *Service) giveCondition(ctx context.Context, c *combatTx, t playdb.Combatant, key string, h *castHit) error {
	if key == "" || slices.Contains(t.Conditions, key) || len(t.Conditions) >= maxConditions {
		return nil
	}
	immune, err := c.immuneToCondition(ctx, t, key)
	if err != nil {
		return err
	}
	if immune {
		h.NoEffectWhy = whyImmune // the refusal and its reason, which only the master reads
		return nil
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
	given := combat.ApplyTempHP(t.temp, amount)
	temp := clamp32(given.TempHP, 0, math.MaxInt32)
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

// raiseMaxHP raises a target's hit point maximum and current hit points by amount
// for Ajuda (SRD 5.1, Aid): the bonus is kept on the target until the master ends it
// (EndCombatEffect). Two Ajudas never add up (SRD, "Combining Magical Effects": the
// effects of the same spell cast several times do not combine, the most potent
// one applies): the target keeps the stronger bonus, and the current hit points rise
// by what the maximum rose. A weaker Ajuda over a stronger one changes nothing. A
// player's character at 0 hit points wakes up with the new current hit points (the
// spell raises them, SRD "Dropping to 0 Hit Points": the unconsciousness ends when
// the character regains any); an NPC or a creature at 0 stays down, the master
// decides whether a fallen NPC is dead.
func (s *Service) raiseMaxHP(ctx context.Context, c *combatTx, t fxTarget, amount int, h *castHit) (*playv1.CharacterVitals, error) {
	bonus := clamp32(amount, 0, math.MaxInt32)
	if holdsHP(t.c) {
		return nil, s.raiseCombatantMaxHP(ctx, c, t, bonus, h)
	}
	return s.raiseCharacterMaxHP(ctx, c, t, bonus, h)
}

// raiseCombatantMaxHP is raiseMaxHP for an NPC or a creature, whose hit points are on the combatant.
func (s *Service) raiseCombatantMaxHP(ctx context.Context, c *combatTx, t fxTarget, bonus int32, h *castHit) error {
	bonusBefore := t.c.HpMaxBonus
	stronger, rose := aidRaise(bonusBefore, bonus)
	before, maxBefore := hpOf(t.c), conv.OrZero(t.c.HpMax)
	h.Healed, h.Gain, h.Restore, h.MaxBefore, h.BonusBefore = &rose, gainMaximum, &before, &maxBefore, &bonusBefore
	now := before
	if rose > 0 {
		if err := c.q.SetCombatantHitPointsMax(ctx, playdb.SetCombatantHitPointsMaxParams{ID: t.c.ID, HpMax: new(maxBefore + rose), HpMaxBonus: stronger}); err != nil {
			return fmt.Errorf("raise the maximum hit points: %w", err)
		}
		if before.HP > 0 {
			now.HP = before.HP + rose
			if err := c.q.SetCombatantHitPoints(ctx, playdb.SetCombatantHitPointsParams{ID: t.c.ID, HpCurrent: &now.HP, HpTemp: &before.Temp, Defeated: before.Defeated}); err != nil {
				return fmt.Errorf("raise the current hit points: %w", err)
			}
		}
	}
	h.HPAfter, h.MaxAfter = &now.HP, new(maxBefore+rose)
	return nil
}

// raiseCharacterMaxHP is raiseMaxHP for a player's character, whose bonus is kept on its vitals.
func (s *Service) raiseCharacterMaxHP(ctx context.Context, c *combatTx, t fxTarget, bonus int32, h *castHit) (*playv1.CharacterVitals, error) {
	stronger, _ := aidRaise(clamp32(t.bonus, 0, math.MaxInt32), bonus)
	b, a, err := s.vitals.SetHitPointsMaxBonus(ctx, c.tx, c.session.CampaignID, t.c.CharacterID, stronger)
	if err != nil {
		return nil, err
	}
	rose := a.GetHitPointsMaxBonus() - b.GetHitPointsMaxBonus()
	h.Healed, h.Gain = &rose, gainMaximum
	h.Restore, h.BonusBefore, h.DeathBefore = new(hpStateOf(b)), new(b.GetHitPointsMaxBonus()), deathOf(t.c)
	h.HPAfter, h.MaxAfter = new(a.GetHitPointsCurrent()), new(a.GetHitPointsMax())
	if b.GetHitPointsCurrent() == 0 && a.GetHitPointsCurrent() > 0 {
		h.Gain = gainCurrent
		if err := c.q.ResetDeathSavesOfCharacter(ctx, playdb.ResetDeathSavesOfCharacterParams{CharacterID: t.c.CharacterID, GameSessionID: c.session.ID}); err != nil {
			return nil, fmt.Errorf("reset the death saves: %w", err)
		}
	}
	return a, nil
}
