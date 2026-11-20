package characters

import (
	"slices"

	rulesv1 "github.com/PuraFome/meuRPG/backend/gen/meurpg/rules/v1"
	"github.com/PuraFome/meuRPG/backend/internal/rules"
)

// The infusion data of a table feature (TableFeature.infusion and infusion_rule): read from the stored body into the
// engine's types, and left out of a player's copy of the entry.

var (
	infusionAcceptsOf = map[rulesv1.InfusionAccepts]string{
		rulesv1.InfusionAccepts_INFUSION_ACCEPTS_WEAPON:             rules.InfusionAcceptsWeapon,
		rulesv1.InfusionAccepts_INFUSION_ACCEPTS_ARMOR:              rules.InfusionAcceptsArmor,
		rulesv1.InfusionAccepts_INFUSION_ACCEPTS_SHIELD:             rules.InfusionAcceptsShield,
		rulesv1.InfusionAccepts_INFUSION_ACCEPTS_ARMOR_OR_SHIELD:    rules.InfusionAcceptsArmorOrShield,
		rulesv1.InfusionAccepts_INFUSION_ACCEPTS_THROWN_WEAPON:      rules.InfusionAcceptsThrownWeapon,
		rulesv1.InfusionAccepts_INFUSION_ACCEPTS_AMMUNITION_WEAPON:  rules.InfusionAcceptsAmmunitionWeapon,
		rulesv1.InfusionAccepts_INFUSION_ACCEPTS_SPELLCASTING_FOCUS: rules.InfusionAcceptsSpellcastingFocus,
		rulesv1.InfusionAccepts_INFUSION_ACCEPTS_ANY_OBJECT:         rules.InfusionAcceptsAnyObject,
	}
	infusionEffectOf = map[rulesv1.InfusionEffect]string{
		rulesv1.InfusionEffect_INFUSION_EFFECT_NONE:                 rules.InfusionEffectNone,
		rulesv1.InfusionEffect_INFUSION_EFFECT_ITEM_BONUS:           rules.InfusionEffectItemBonus,
		rulesv1.InfusionEffect_INFUSION_EFFECT_SPELL_FOCUS_BONUS:    rules.InfusionEffectSpellFocusBonus,
		rulesv1.InfusionEffect_INFUSION_EFFECT_REPEATING_AMMUNITION: rules.InfusionEffectRepeatingAmmunition,
		rulesv1.InfusionEffect_INFUSION_EFFECT_RETURNING:            rules.InfusionEffectReturning,
		rulesv1.InfusionEffect_INFUSION_EFFECT_RESISTANCE_LABEL:     rules.InfusionEffectResistanceLabel,
	}
)

// tableInfusionOf is the engine's infusion of a stored one, or nil when none is set. A kind the engine does not know
// (unspecified, or a value of a newer client) reads as empty, which the engine refuses.
func tableInfusionOf(in *rulesv1.TableInfusion) *rules.TableInfusion {
	if in == nil {
		return nil
	}
	out := &rules.TableInfusion{
		Accepts: infusionAcceptsOf[in.GetAccepts()], RequiresAttunement: in.GetRequiresAttunement(), Effect: infusionEffectOf[in.GetEffect()],
		DamageTypes: slices.Clone(in.GetDamageTypes()),
	}
	for _, s := range in.GetBonusSteps() {
		out.BonusSteps = append(out.BonusSteps, rules.TableInfusionStep{ClassLevel: int(s.GetClassLevel()), Bonus: int(s.GetBonus())})
	}
	return out
}

// tableInfusionRuleOf is the engine's infusion rule of a stored one, or nil when none is set.
func tableInfusionRuleOf(in *rulesv1.TableInfusionRule) *rules.TableInfusionRule {
	if in == nil {
		return nil
	}
	out := &rules.TableInfusionRule{}
	for _, s := range in.GetLimitSteps() {
		out.LimitSteps = append(out.LimitSteps, rules.TableInfusionLimitStep{ClassLevel: int(s.GetClassLevel()), Active: int(s.GetActive())})
	}
	return out
}

// carriesInfusions says the body is an entry with a feature, or an option of a feature, that has infusion data, or a
// table option that is an infusion. Only a class or a subclass can hold valid data; the other kinds are covered so a
// body that carries some anyway never reaches a player.
func carriesInfusions(body tableBody) bool {
	has := func(fs []*rulesv1.TableFeature) bool { return slices.ContainsFunc(fs, featureCarriesInfusion) }
	switch b := body.(type) {
	case *rulesv1.TableClass:
		return slices.ContainsFunc(b.GetLevels(), func(l *rulesv1.TableClassLevel) bool { return has(l.GetFeatures()) })
	case *rulesv1.TableSubclass:
		return slices.ContainsFunc(b.GetLevels(), func(l *rulesv1.TableSubclassLevel) bool { return has(l.GetFeatures()) })
	case *rulesv1.TableRace:
		return has(b.GetTraits())
	case *rulesv1.TableSubrace:
		return has(b.GetTraits())
	case *rulesv1.TableBackground:
		return b.GetFeature() != nil && has([]*rulesv1.TableFeature{b.GetFeature()})
	case *rulesv1.TableFeat:
		return has(b.GetOptions())
	case *rulesv1.TableOption:
		return b.GetInfusion() != nil
	}
	return false
}

func featureCarriesInfusion(f *rulesv1.TableFeature) bool {
	return f.GetInfusion() != nil || f.GetInfusionRule() != nil || slices.ContainsFunc(f.GetOptions(), featureCarriesInfusion)
}
