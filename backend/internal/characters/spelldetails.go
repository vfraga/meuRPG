package characters

import (
	"context"
	"errors"
	"slices"

	"connectrpc.com/connect"

	rulesv1 "github.com/PuraFome/meuRPG/backend/gen/meurpg/rules/v1"
	"github.com/PuraFome/meuRPG/backend/internal/authz"
	"github.com/PuraFome/meuRPG/backend/internal/rules"
)

// GetSpellDetails implements rulesv1connect.ContentServiceHandler. It is the
// long, per-spell counterpart of ListContent, which stays light.
func (s *Service) GetSpellDetails(
	ctx context.Context,
	req *connect.Request[rulesv1.GetSpellDetailsRequest],
) (*connect.Response[rulesv1.GetSpellDetailsResponse], error) {
	// The same access as ListContent: a pending member may look too (RN-15).
	m, err := authz.RequireCampaignMemberOrPending(ctx, req.Msg.GetCampaignId())
	if err != nil {
		return nil, err
	}
	content, err := s.contentFor(ctx, nil, m.CampaignID)
	if err != nil {
		return nil, s.dbError(ctx, "read rules content", err)
	}
	d, ok := content.SpellDetails(req.Msg.GetSpellKey())
	if !ok {
		return nil, connect.NewError(connect.CodeNotFound, errUnknownSpell)
	}
	visible, err := s.spellVisible(ctx, m, content, req.Msg.GetSpellKey())
	if err != nil {
		return nil, s.dbError(ctx, "read the caller's sheets", err)
	}
	if !visible {
		return nil, connect.NewError(connect.CodeNotFound, errUnknownSpell)
	}
	out := spellDetailsToProto(d)
	if !isMaster(m) {
		// The classes whose list has the spell: a hidden one is not named to a player.
		out.Spell.ClassKeys = slices.DeleteFunc(slices.Clone(out.GetSpell().GetClassKeys()), content.Hidden)
	}
	out.HitPointEffect = hitPointEffect(content, req.Msg.GetSpellKey(), d.Spell.Level)
	return connect.NewResponse(&rulesv1.GetSpellDetailsResponse{Spell: out}), nil
}

// spellVisible says whether the caller reads the spell. The spell of a feature's summon is the feature's: the master and
// a player whose character uses the entry of the feature read it. Any other spell the master archived or switched off is
// not shown to a player (RN-23), unless one of their own sheets in the campaign uses it.
func (s *Service) spellVisible(ctx context.Context, m authz.Membership, content *rules.Content, key string) (bool, error) {
	if isMaster(m) {
		return true, nil
	}
	if owner, ok := content.FeatureSummonOwner(key); ok {
		return s.callerUses(ctx, m, content, owner)
	}
	if content.Hidden(key) {
		return s.callerUses(ctx, m, content, key)
	}
	return true, nil
}

var hpEffectKindToProto = map[string]rulesv1.SpellHitPointEffectKind{
	rules.SpellKindHPPool:      rulesv1.SpellHitPointEffectKind_SPELL_HIT_POINT_EFFECT_KIND_POOL,
	rules.SpellKindHPThreshold: rulesv1.SpellHitPointEffectKind_SPELL_HIT_POINT_EFFECT_KIND_THRESHOLD,
	rules.SpellKindZeroHP:      rulesv1.SpellHitPointEffectKind_SPELL_HIT_POINT_EFFECT_KIND_ZERO_HP,
	rules.SpellKindFlatHeal:    rulesv1.SpellHitPointEffectKind_SPELL_HIT_POINT_EFFECT_KIND_FLAT_HEAL,
	rules.SpellKindTempHP:      rulesv1.SpellHitPointEffectKind_SPELL_HIT_POINT_EFFECT_KIND_TEMP_HP,
	rules.SpellKindMaxHP:       rulesv1.SpellHitPointEffectKind_SPELL_HIT_POINT_EFFECT_KIND_MAX_HP,
}

// hitPointEffect is the rule of a spell that reads hit points, at its own circle
// and what one circle more adds (the difference of two reads of the effects).
// Nil for any other spell.
func hitPointEffect(content *rules.Content, key string, level int) *rulesv1.SpellHitPointEffect {
	base, ok := content.SpellEffect(key, level)
	if !ok {
		return nil
	}
	next, _ := content.SpellEffect(key, level+1)
	return &rulesv1.SpellHitPointEffect{
		Kind:             hpEffectKindToProto[base.Kind],
		PoolDiceCount:    i32(base.Dice.Count),
		PoolDiceSides:    i32(base.Dice.Sides),
		PoolDicePerLevel: i32(next.Dice.Count - base.Dice.Count),
		ConditionKey:     base.Condition,
		Threshold:        i32(base.Threshold),
		Dies:             base.Dies,
		HealAmount:       i32(base.Heal),
		HealPerLevel:     i32(next.Heal - base.Heal),
		Amount:           i32(base.Amount),
		AmountPerLevel:   i32(next.Amount - base.Amount),
	}
}

var errUnknownSpell = errors.New("spell not found")

var (
	castingUnitToProto = map[string]rulesv1.CastingTimeUnit{
		rules.CastAction:      rulesv1.CastingTimeUnit_CASTING_TIME_UNIT_ACTION,
		rules.CastBonusAction: rulesv1.CastingTimeUnit_CASTING_TIME_UNIT_BONUS_ACTION,
		rules.CastReaction:    rulesv1.CastingTimeUnit_CASTING_TIME_UNIT_REACTION,
		rules.CastMinute:      rulesv1.CastingTimeUnit_CASTING_TIME_UNIT_MINUTE,
		rules.CastHour:        rulesv1.CastingTimeUnit_CASTING_TIME_UNIT_HOUR,
	}
	rangeKindToProto = map[string]rulesv1.SpellRangeKind{
		rules.RangeSelf:      rulesv1.SpellRangeKind_SPELL_RANGE_KIND_SELF,
		rules.RangeTouch:     rulesv1.SpellRangeKind_SPELL_RANGE_KIND_TOUCH,
		rules.RangeRanged:    rulesv1.SpellRangeKind_SPELL_RANGE_KIND_RANGED,
		rules.RangeSight:     rulesv1.SpellRangeKind_SPELL_RANGE_KIND_SIGHT,
		rules.RangeUnlimited: rulesv1.SpellRangeKind_SPELL_RANGE_KIND_UNLIMITED,
		rules.RangeSpecial:   rulesv1.SpellRangeKind_SPELL_RANGE_KIND_SPECIAL,
	}
	durationKindToProto = map[string]rulesv1.SpellDurationKind{
		rules.DurationInstantaneous:  rulesv1.SpellDurationKind_SPELL_DURATION_KIND_INSTANTANEOUS,
		rules.DurationTimed:          rulesv1.SpellDurationKind_SPELL_DURATION_KIND_TIMED,
		rules.DurationUntilDispelled: rulesv1.SpellDurationKind_SPELL_DURATION_KIND_UNTIL_DISPELLED,
		rules.DurationSpecial:        rulesv1.SpellDurationKind_SPELL_DURATION_KIND_SPECIAL,
	}
	durationUnitToProto = map[string]rulesv1.SpellDurationUnit{
		rules.DurationRound:  rulesv1.SpellDurationUnit_SPELL_DURATION_UNIT_ROUND,
		rules.DurationMinute: rulesv1.SpellDurationUnit_SPELL_DURATION_UNIT_MINUTE,
		rules.DurationHour:   rulesv1.SpellDurationUnit_SPELL_DURATION_UNIT_HOUR,
		rules.DurationDay:    rulesv1.SpellDurationUnit_SPELL_DURATION_UNIT_DAY,
	}
	spellAttackToProto = map[string]rulesv1.SpellAttackType{
		"":       rulesv1.SpellAttackType_SPELL_ATTACK_TYPE_NONE,
		"melee":  rulesv1.SpellAttackType_SPELL_ATTACK_TYPE_MELEE,
		"ranged": rulesv1.SpellAttackType_SPELL_ATTACK_TYPE_RANGED,
	}
	saveSuccessToProto = map[string]rulesv1.SpellSaveSuccess{
		"none":  rulesv1.SpellSaveSuccess_SPELL_SAVE_SUCCESS_NONE,
		"half":  rulesv1.SpellSaveSuccess_SPELL_SAVE_SUCCESS_HALF,
		"other": rulesv1.SpellSaveSuccess_SPELL_SAVE_SUCCESS_OTHER,
	}
)

func spellDetailsToProto(d *rules.SpellDetails) *rulesv1.SpellDetails {
	out := &rulesv1.SpellDetails{
		Spell: spellToProto(d.Spell),
		CastingTime: &rulesv1.SpellCastingTime{
			Amount: i32(d.CastingTime.Amount), Unit: castingUnitToProto[d.CastingTime.Unit],
			Trigger: d.CastingTime.Trigger, Raw: d.CastingTime.Raw,
		},
		Range: &rulesv1.SpellRange{Kind: rangeKindToProto[d.Range.Kind], DistanceFt: i32(d.Range.DistanceFt), Raw: d.Range.Raw},
		Components: &rulesv1.SpellComponents{
			Verbal: d.Components.Verbal, Somatic: d.Components.Somatic, Material: d.Components.Material,
			MaterialText: d.Components.MaterialText, MaterialTextPt: d.Components.MaterialTextPT,
		},
		Duration: &rulesv1.SpellDuration{
			Kind: durationKindToProto[d.Duration.Kind], Amount: i32(d.Duration.Amount), Unit: durationUnitToProto[d.Duration.Unit],
			UpTo: d.Duration.UpTo, Concentration: d.Duration.Concentration, Raw: d.Duration.Raw,
		},
		AttackType:      spellAttackToProto[d.AttackType],
		HealBySlotLevel: levelsToProto(d.HealBySlotLevel),
		Description:     d.Description,
		HigherLevel:     d.HigherLevel,
		DescriptionPt:   d.DescriptionPT,
		HigherLevelPt:   d.HigherLevelPT,
		TextPtMissing:   d.TextPTMissing,
		TextPtOnly:      d.TextPTOnly,
		Target:          spellTargetToProto(d.Target),
		DamageChoice:    damageChoiceToProto[d.DamageChoice],
	}
	if d.Save != nil {
		out.Save = &rulesv1.SpellSave{Ability: abilityToProto[d.Save.Ability], OnSuccess: saveSuccessToProto[d.Save.OnSuccess]}
	}
	for _, dm := range d.Damage {
		out.Damage = append(out.Damage, &rulesv1.SpellDamage{
			DamageTypeKey: dm.Type, DamageTypePt: dm.TypeNamePT,
			BySlotLevel: levelsToProto(dm.BySlotLevel), ByCharacterLevel: levelsToProto(dm.ByCharacterLevel),
		})
	}
	return out
}

var damageChoiceToProto = map[string]rulesv1.SpellDamageChoice{
	rules.DamageChoiceScale:       rulesv1.SpellDamageChoice_SPELL_DAMAGE_CHOICE_SCALE,
	rules.DamageChoiceAlternative: rulesv1.SpellDamageChoice_SPELL_DAMAGE_CHOICE_ALTERNATIVE,
}

var (
	spellTargetKindToProto = map[string]rulesv1.SpellTargetKind{
		rules.TargetCreature:  rulesv1.SpellTargetKind_SPELL_TARGET_KIND_CREATURE,
		rules.TargetCreatures: rulesv1.SpellTargetKind_SPELL_TARGET_KIND_CREATURES,
		rules.TargetArea:      rulesv1.SpellTargetKind_SPELL_TARGET_KIND_AREA,
		rules.TargetSelf:      rulesv1.SpellTargetKind_SPELL_TARGET_KIND_SELF,
		rules.TargetNone:      rulesv1.SpellTargetKind_SPELL_TARGET_KIND_NONE,
	}
	spellAreaShapeToProto = map[string]rulesv1.SpellAreaShape{
		rules.ShapeCone:     rulesv1.SpellAreaShape_SPELL_AREA_SHAPE_CONE,
		rules.ShapeCube:     rulesv1.SpellAreaShape_SPELL_AREA_SHAPE_CUBE,
		rules.ShapeCylinder: rulesv1.SpellAreaShape_SPELL_AREA_SHAPE_CYLINDER,
		rules.ShapeLine:     rulesv1.SpellAreaShape_SPELL_AREA_SHAPE_LINE,
		rules.ShapeSphere:   rulesv1.SpellAreaShape_SPELL_AREA_SHAPE_SPHERE,
	}
)

// spellTargetToProto is whom a spell reaches, with the text the "Magias" page shows.
func spellTargetToProto(t rules.SpellTarget) *rulesv1.SpellTarget {
	return &rulesv1.SpellTarget{
		Kind: spellTargetKindToProto[t.Kind], Count: i32(t.Count), PerSlotLevel: i32(t.PerSlotLevel),
		Shape: spellAreaShapeToProto[t.Shape], SizeFt: i32(t.SizeFt), LabelPt: t.LabelPT(),
	}
}

func levelsToProto(m map[int]string) map[int32]string {
	if len(m) == 0 {
		return nil
	}
	out := make(map[int32]string, len(m))
	for l, v := range m {
		out[i32(l)] = v
	}
	return out
}
