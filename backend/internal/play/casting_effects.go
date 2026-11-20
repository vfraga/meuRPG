package play

import (
	"context"
	"encoding/json"
	"fmt"
	"math"
	"time"
	"uuid"

	"github.com/PuraFome/meuRPG/backend/internal/authz"
	"github.com/PuraFome/meuRPG/backend/internal/play/link"
	"github.com/PuraFome/meuRPG/backend/internal/play/playdb"
	"github.com/PuraFome/meuRPG/backend/internal/rules"
	"github.com/PuraFome/meuRPG/backend/internal/rules/combat"
)

// What a cast does when it takes effect (SRD 5.1, each spell's own text). The server
// applies what the engine knows: the slot, the concentration, the hit points of a
// healing spell (Cure Wounds, Healing Word, Prayer of Healing, Heal and the others) and
// of False Life, the armor class of Mage Armor and the creatures of a summoning
// spell. Anything else is a cast the log records and the master narrates.

// stub is the combatant a character that is not in a combat stands for in the
// helpers of the combat's hit points (healCombatant, readFxTarget).
func stub(ch link.Character) playdb.Combatant {
	return playdb.Combatant{Kind: kindPlayer, CharacterID: ch.ID}
}

// castRolled is the dice a cast rolled.
type castRolled struct {
	count, sides int32
	faces        []int32
	total        int32
	physical     bool
}

// takeEffect makes the cast take effect: it spends the slot, ends the caster's other
// concentration, applies the spell's effect and closes the row (it lasts, or it is
// over). The row exists already, in the casting state.
func (s *Service) takeEffect(ctx context.Context, c *combatTx, m authz.Membership, plan castPlan) (castOutcome, error) {
	out := castOutcome{guard: plan.g}
	if err := s.spendCastSlot(ctx, c, plan, &out); err != nil {
		return out, err
	}
	if err := s.endCastConcentration(ctx, c, plan, &out); err != nil {
		return out, err
	}
	rolled, err := s.applyCastEffect(ctx, c, m, plan, &out)
	if err != nil {
		return out, err
	}
	// It lasts, or it is over (SRD 5.1, "Duration").
	lasts := plan.osp.Lasts || plan.osp.Spell.Concentration
	if err := s.finishCastRow(ctx, c, plan, rolled, lasts, &out); err != nil {
		return out, err
	}
	plan.row = out.row
	if lasts {
		if err := s.castCharacterEffects(ctx, c, plan); err != nil {
			return out, err
		}
	}
	out.ev = out.event()
	return out, nil
}

// spendCastSlot spends the slot now, not at the start of a long casting (SRD 5.1, "Longer
// Casting Times": a casting that fails spends none). An NPC keeps no slots.
func (s *Service) spendCastSlot(ctx context.Context, c *combatTx, plan castPlan, out *castOutcome) error {
	if plan.slot == nil || !plan.g.caster.Player {
		return nil
	}
	v, err := s.spendSlot(ctx, c, plan.g.caster.ID, *plan.slot, 1)
	if err != nil {
		return err
	}
	out.vitals = append(out.vitals, v)
	return nil
}

// endCastConcentration ends the caster's other concentration when the spell asks for its own
// (SRD 5.1, "Duration": a creature can't concentrate on two spells at once).
func (s *Service) endCastConcentration(ctx context.Context, c *combatTx, plan castPlan, out *castOutcome) error {
	if !plan.osp.Spell.Concentration {
		return nil
	}
	ended, dismissed, vitals, err := s.endOtherConcentration(ctx, c, plan.g.caster, plan.row.ID)
	if err != nil {
		return err
	}
	out.ended, out.dismissed = append(out.ended, ended...), append(out.dismissed, dismissed...)
	out.vitals = append(out.vitals, vitals...)
	return nil
}

// applyCastEffect applies what the spell does to its targets and returns the dice it rolled.
func (s *Service) applyCastEffect(ctx context.Context, c *combatTx, m authz.Membership, plan castPlan, out *castOutcome) (castRolled, error) {
	var rolled castRolled
	var err error
	switch plan.osp.Effect {
	case link.EffectHeal:
		rolled, out.targets, err = s.healTargets(ctx, c, plan, out)
	case link.EffectTempHP:
		rolled, out.targets, err = s.tempHPTargets(ctx, c, plan, out)
	case link.EffectMaxHP:
		out.targets, err = s.maxHPTargets(ctx, c, plan, out)
	case link.EffectArmorClass:
		out.targets, err = s.armorTargets(ctx, c, m, plan)
	case link.EffectSummon:
		out.targets = narratedTargets(plan)
		if !plan.osp.NPC {
			err = s.summonFor(ctx, c, m, plan, out)
		}
	default:
		out.targets = narratedTargets(plan)
	}
	return rolled, err
}

// finishCastRow closes the cast's row: it stays active when it lasts, and is over at once when not.
func (s *Service) finishCastRow(ctx context.Context, c *combatTx, plan castPlan, rolled castRolled, lasts bool, out *castOutcome) error {
	now := c.now
	status, reason, endedAt := castActive, (*string)(nil), (*time.Time)(nil)
	if !lasts {
		status, reason, endedAt = castEnded, new(endInstant), &now
	}
	targets, err := json.Marshal(nonNilTargets(out.targets))
	if err != nil {
		return fmt.Errorf("encode the cast's targets: %w", err)
	}
	row, err := c.q.FinishSpellCast(ctx, playdb.FinishSpellCastParams{
		ID: plan.row.ID, Status: status, EndReason: reason, EndedAt: endedAt,
		Concentrating: lasts && plan.osp.Spell.Concentration, Targets: targets,
		DiceCount: rolled.count, DiceSides: rolled.sides, RollFaces: nonNilFaces(rolled.faces), RollTotal: rolled.total,
		Physical: rolled.physical, CreatureIds: nonNil(out.created), CastAt: &now,
		SlotLevel: castSlotLevel(plan.slot), SlotPact: plan.slot != nil && plan.slot.Pact,
	})
	if err != nil {
		return fmt.Errorf("finish the cast: %w", err)
	}
	out.row = row
	return nil
}

func nonNilTargets(t []castTarget) []castTarget {
	if t == nil {
		return []castTarget{}
	}
	return t
}

func nonNilFaces(f []int32) []int32 {
	if f == nil {
		return []int32{}
	}
	return f
}

// narratedTargets lists the targets of a cast the server applies nothing to: the log
// still says who the master was told it reached.
func narratedTargets(plan castPlan) []castTarget {
	var out []castTarget
	for _, t := range plan.g.targets {
		out = append(out, castTarget{ID: t.ID, Effect: castNarrated})
	}
	if len(out) == 0 && selfOnly(plan.osp.Spell) {
		out = append(out, castTarget{ID: plan.g.caster.ID, Effect: castNarrated})
	}
	return out
}

// levelOf is the level the spell has in this casting: the slot's, the cast level of its grant or its own
// (SRD 5.1, "Casting a Spell at a Higher Level").
func levelOf(plan castPlan) int {
	if plan.slot != nil {
		return int(plan.slot.Level)
	}
	return plan.grant.LevelOr(plan.osp.Spell.Level)
}

// healTargets heals each player character among the targets. The spell's dice are one
// roll for the whole cast (as a combat's heal is); the Life Domain adds to it (SRD
// 5.1, Cleric: Life Domain): Disciple of Life to each target, Supreme Healing makes
// the dice their highest number, and Blessed Healer heals the caster once when someone
// else regained hit points. An NPC target changes nothing: it has no hit points
// outside a combat, and the master narrates.
func (s *Service) healTargets(ctx context.Context, c *combatTx, plan castPlan, out *castOutcome) (castRolled, []castTarget, error) {
	osp := plan.osp
	h := combat.HealFeatures{Disciple: osp.Healing.Disciple, Blessed: osp.Healing.Blessed, Supreme: osp.Healing.Supreme}
	rolled, amount, err := s.rollHealing(plan, h)
	if err != nil {
		return rolled, nil, err
	}
	level := levelOf(plan)
	extra := h.TargetExtra(level)

	var targets []castTarget
	healedOther := false
	for _, t := range plan.g.targets {
		if !t.Player {
			targets = append(targets, castTarget{ID: t.ID, Effect: castNarrated})
			continue
		}
		ct, err := s.healCastTarget(ctx, c, t, amount, extra, out)
		if err != nil {
			return rolled, nil, err
		}
		targets = append(targets, ct)
		healedOther = healedOther || (t.ID != plan.g.caster.ID && ct.Amount > 0)
	}
	if len(plan.g.targets) == 0 && selfOnly(osp.Spell) && plan.g.caster.Player {
		ct, err := s.healCastTarget(ctx, c, plan.g.caster, amount, extra, out)
		if err != nil {
			return rolled, nil, err
		}
		targets = append(targets, ct)
	}
	targets, err = s.blessedHealer(ctx, c, plan, h.SelfExtra(level, healedOther), targets, out)
	return rolled, targets, err
}

// rollHealing works out the healing of the cast: a fixed number, or the spell's dice rolled once (or
// at their highest number with Supreme Healing), and notes the dice for the cast's row.
func (s *Service) rollHealing(plan castPlan, h combat.HealFeatures) (castRolled, int, error) {
	osp := plan.osp
	var rolled castRolled
	amount := 0
	switch {
	case osp.Spell.HP != nil && osp.Spell.HP.Kind == rules.SpellKindFlatHeal:
		amount = osp.Spell.HP.Heal // Heal restores a fixed number
	case osp.Spell.Heal != nil:
		var err error
		if amount, err = s.rollHealDice(plan, h, &rolled); err != nil {
			return rolled, 0, err
		}
	}
	amount = max(amount, 0)
	rolled.total = clamp32(amount, 0, math.MaxInt32)
	return rolled, amount, nil
}

// rollHealDice rolls the dice of a healing spell (or takes their highest number) and returns the
// healing they give with the Life Domain's features.
func (s *Service) rollHealDice(plan castPlan, h combat.HealFeatures, rolled *castRolled) (int, error) {
	d := plan.osp.Spell.Heal
	sum := 0
	if d.Count > 0 && !h.Supreme {
		r, err := s.poolRoll(link.Dice{Count: d.Count, Sides: d.Sides}, plan.in)
		if err != nil {
			return 0, err
		}
		sum, rolled.faces, rolled.physical = r.Total, faces32(r.Faces), r.Physical
	}
	if d.Count > 0 && h.Supreme {
		rolled.faces = make([]int32, d.Count)
		for i := range rolled.faces {
			rolled.faces[i] = clamp32(d.Sides, 0, math.MaxInt32)
		}
	}
	amount := h.HealTotal(d.Count, d.Sides, d.Bonus, sum)
	rolled.count, rolled.sides = clamp32(d.Count, 0, 100), clamp32(d.Sides, 0, 100)
	return amount, nil
}

// healCastTarget heals one player character by the cast's healing plus Disciple of Life's extra
// and returns what the cast did to it.
func (s *Service) healCastTarget(ctx context.Context, c *combatTx, t link.Character, amount, extra int, out *castOutcome) (castTarget, error) {
	hit, v, err := s.healCombatant(ctx, c, stub(t), clamp32(amount+extra, 0, math.MaxInt32))
	if err != nil {
		return castTarget{}, err
	}
	out.vitals = append(out.vitals, v)
	ct := castTarget{ID: t.ID, Effect: castHeal, Amount: hit.Amount, Extra: clamp32(extra, 0, math.MaxInt32)}
	if hit.Before != nil && hit.After != nil {
		ct.Before, ct.After = hit.Before.HP, hit.After.HP
	}
	return ct, nil
}

// blessedHealer heals the caster once for the casting, when a creature other than the caster
// regained hit points (self is zero otherwise). It adds to the caster's own line when the caster is a
// target too.
func (s *Service) blessedHealer(ctx context.Context, c *combatTx, plan castPlan, self int, targets []castTarget, out *castOutcome) ([]castTarget, error) {
	if self <= 0 || !plan.g.caster.Player {
		return targets, nil
	}
	hit, v, err := s.healCombatant(ctx, c, stub(plan.g.caster), clamp32(self, 0, math.MaxInt32))
	if err != nil {
		return nil, err
	}
	out.vitals = append(out.vitals, v)
	if i := indexOfTarget(targets, plan.g.caster.ID); i >= 0 {
		targets[i].Amount += hit.Amount
		if hit.After != nil {
			targets[i].After = hit.After.HP
		}
		return targets, nil
	}
	ct := castTarget{ID: plan.g.caster.ID, Effect: castHeal, Amount: hit.Amount, Blessed: true}
	if hit.Before != nil && hit.After != nil {
		ct.Before, ct.After = hit.Before.HP, hit.After.HP
	}
	return append(targets, ct), nil
}

func indexOfTarget(targets []castTarget, id string) int {
	for i, t := range targets {
		if t.ID == id {
			return i
		}
	}
	return -1
}

// tempHPTargets gives the temporary hit points of False Life: the dice plus the fixed
// amount. They do not stack: a target keeps the larger of what it had and what the
// spell gives (SRD 5.1, "Temporary Hit Points").
func (s *Service) tempHPTargets(ctx context.Context, c *combatTx, plan castPlan, out *castOutcome) (castRolled, []castTarget, error) {
	fx := plan.osp.Spell.HP
	var rolled castRolled
	gained := fx.Amount
	if fx.Pool.Count > 0 {
		r, err := s.poolRoll(link.Dice{Count: fx.Pool.Count, Sides: fx.Pool.Sides}, plan.in)
		if err != nil {
			return rolled, nil, err
		}
		gained += r.Total
		rolled = castRolled{count: clamp32(fx.Pool.Count, 0, 100), sides: clamp32(fx.Pool.Sides, 0, 100), faces: faces32(r.Faces), physical: r.Physical}
	}
	rolled.total = clamp32(gained, 0, math.MaxInt32)
	var targets []castTarget
	for _, t := range s.effectTargetsAll(plan) {
		if !t.Player {
			targets = append(targets, castTarget{ID: t.ID, Effect: castNarrated})
			continue
		}
		ft, err := s.readFxTarget(ctx, c, stub(t))
		if err != nil {
			return rolled, nil, err
		}
		var hit castHit
		v, err := s.giveTempHP(ctx, c, ft, gained, &hit)
		if err != nil {
			return rolled, nil, err
		}
		out.vitals = append(out.vitals, v)
		ct := castTarget{ID: t.ID, Effect: castTempHP, Before: clamp32(ft.temp, 0, math.MaxInt32), After: clamp32(max(ft.temp, gained), 0, math.MaxInt32)}
		if hit.Healed != nil {
			ct.Amount = *hit.Healed
		}
		targets = append(targets, ct)
	}
	return rolled, targets, nil
}

// effectTargetsAll are the targets of an effect, the caster standing in for a spell that
// reaches only the caster: the NPCs too, which the effect passes over.
func (s *Service) effectTargetsAll(plan castPlan) []link.Character {
	if len(plan.g.targets) == 0 && selfOnly(plan.osp.Spell) {
		return []link.Character{plan.g.caster}
	}
	return plan.g.targets
}

// armorTargets puts Mage Armor on each player character among the targets (SRD 5.1:
// the base armor class becomes 13 + Dexterity for a creature that wears no armor).
// The sheet is not changed: a combat reads the armor class from the combatant, and
// the combatants of the character in a combat that is open take it now.
func (s *Service) armorTargets(ctx context.Context, c *combatTx, m authz.Membership, plan castPlan) ([]castTarget, error) {
	var targets []castTarget
	for _, t := range s.effectTargetsAll(plan) {
		if !t.Player {
			targets = append(targets, castTarget{ID: t.ID, Effect: castNarrated})
			continue
		}
		mage, err := s.roster.MageArmorAC(ctx, c.tx, m.CampaignID, t.ID)
		if err != nil {
			return nil, err
		}
		if !mage.Applies {
			targets = append(targets, castTarget{ID: t.ID, Effect: castNarrated})
			continue
		}
		ac := clamp32(mage.AC, 1, 60)
		targets = append(targets, castTarget{ID: t.ID, Effect: castArmor, AC: ac})
	}
	return targets, nil
}

// summonFor makes the creatures of a summoning spell, as PlayService.CastSummon does:
// the choice is checked against the slot and the sheet, the creatures are recorded
// for the caster, and a new familiar or a new concentration sends the old ones away.
func (s *Service) summonFor(ctx context.Context, c *combatTx, m authz.Membership, plan castPlan, out *castOutcome) error {
	circle := 0
	if plan.slot != nil {
		circle = int(plan.slot.Level)
	}
	chk, err := s.checkSummonChoice(ctx, c.tx, m.CampaignID, plan.g.caster.ID, plan.spell, circle, plan.pick, plan.grant != nil && plan.slot == nil)
	if err != nil {
		return err
	}
	if plan.ritual && (!chk.CanRitual || !chk.Ritual) {
		return badCast("the character cannot cast this spell as a ritual")
	}
	given, err := summonNames(plan.pick.GetNames(), len(chk.Creatures))
	if err != nil {
		return err
	}
	sum, err := s.roster.SummonCreatures(ctx, c.tx, summonOf(m.CampaignID, plan.g.caster.ID, plan.spell, uuid.New().String(), circle, chk, summonSpecs(chk.Creatures, given)))
	if err != nil {
		return err
	}
	for _, cr := range sum.Created {
		out.created = append(out.created, cr.ID)
	}
	was := c.characterID
	c.characterID = &plan.g.caster.ID
	defer func() { c.characterID = was }()
	for _, cr := range sum.Replaced {
		out.dismissed = append(out.dismissed, cr.ID)
		if err := insertEvent(ctx, c, eventCreatureDismissed, &m.UserID, nil, actionEvent{OwnerCharacter: plan.g.caster.ID, Created: []string{cr.ID}, Reason: cr.DismissedReason}); err != nil {
			return err
		}
	}
	return nil
}
