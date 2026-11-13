package rules

import (
	"errors"
	"fmt"
	"slices"
	"unicode/utf8"
)

// maxSpellEffectItems is the most modifiers, and the most conditions, a table spell's effect has.
const maxSpellEffectItems = 8

// checkSpellEffect checks the lasting effect of a table spell and returns it as the combat reads it, nil when
// the spell has none. Every field that is wrong is reported at the effect, one sentence for each. It is a
// method of the builder because the conditions and the damage types are the SRD content's.
func (b *overlayBuilder) checkSpellEffect(c *entryErrors, key, path string, ts *TableSpell, target SpellTarget) *EffectDef {
	e := ts.Effect
	if e == nil {
		return nil
	}
	checkEffectHead(c, key, path, ts, target)
	b.checkEffectConditions(c, key, path, e.Conditions)
	b.checkEffectModifiers(c, key, path, e.Modifiers)
	b.checkEffectAura(c, key, path, ts, target)
	if len(e.Conditions) == 0 && len(e.Modifiers) == 0 {
		c.at(key, path, ".effect", ReasonValue, "an effect needs a condition or a modifier")
	}
	if ts.Duration.Kind == DurationInstantaneous {
		c.at(key, path, ".effect", ReasonValue, "an effect lasts as the spell does: an instantaneous spell has nothing to last")
	}
	if utf8.RuneCountInString(e.PlayerLabelPT) > maxPlayerLabel {
		c.at(key, path, ".effect.player_label_pt", ReasonLimit, "the label is over %d characters", maxPlayerLabel)
	}
	if e.Visibility != "" && e.Visibility != EffectVisibilityPublic && e.Visibility != EffectVisibilityOwner {
		c.at(key, path, ".effect.visibility", ReasonValue, "the visibility is public or owner")
	}
	return tableEffectDef(ts)
}

// auraRefusedKinds are the modifiers an aura cannot give: they are written on the combatant when the effect
// lands (the armor class, the speed, the temporary hit points), so they would stay behind when it moves away. An
// aura gives what a roll or a damage reads where the creature stands then.
var auraRefusedKinds = []string{ModifierACBonus, ModifierSpeedAdd, ModifierSpeedMultiplier, ModifierTempHP}

// checkEffectAura checks the aura of a spell's effect: its radius and side, that the spell is one the caster
// keeps on themself and that lasts, and that it gives only what is read at the roll.
func (b *overlayBuilder) checkEffectAura(c *entryErrors, key, path string, ts *TableSpell, target SpellTarget) {
	a := ts.Effect.Aura
	if a == nil {
		return
	}
	if err := checkAuraModifier(EffectModifier{Kind: ModifierAura, RadiusFt: a.RadiusFt, Affects: a.Affects}); err != nil {
		c.at(key, path, ".effect.aura"+modifierFieldSuffix(err), ReasonValue, "%v", err)
	}
	if ts.Duration.Kind == DurationInstantaneous {
		c.at(key, path, ".effect.aura", ReasonValue, "an aura lasts as the spell does: an instantaneous spell has none")
	}
	if target.Kind != TargetSelf {
		c.at(key, path, ".effect.aura", ReasonValue, "an aura is around the caster: the spell's target is the caster")
	}
	if len(ts.Effect.Conditions) > 0 {
		c.at(key, path, ".effect.conditions", ReasonValue, "an aura gives no condition: it is worked out where each creature stands at the roll")
	}
	for i, m := range ts.Effect.Modifiers {
		if slices.Contains(auraRefusedKinds, m.Kind) {
			c.at(key, path, fmt.Sprintf(".effect.modifiers[%d].kind", i), ReasonValue, "%s is written on the creature when the effect lands, so an aura cannot give it", m.Kind)
		}
	}
}

// checkEffectHead checks whom the effect reaches against what the spell asks of its targets.
func checkEffectHead(c *entryErrors, key, path string, ts *TableSpell, target SpellTarget) {
	switch applies := ts.Effect.Applies; {
	case applies != "" && !slices.Contains(effectApplies, applies):
		c.at(key, path, ".effect.applies", ReasonValue, "the effect applies to all, to the ones that fail the save, or to the ones the attack hits")
	case applies != "" && applies != "all" && target.Kind == TargetSelf:
		c.at(key, path, ".effect.applies", ReasonValue, "a spell that only affects the caster has no save and no attack: the effect applies to all")
	case applies == "failed_save" && ts.Save == nil:
		c.at(key, path, ".effect.applies", ReasonValue, "failed_save needs a saving throw on the spell")
	case applies == "hit" && ts.Attack == "":
		c.at(key, path, ".effect.applies", ReasonValue, "hit needs a spell attack on the spell")
	}
}

func (b *overlayBuilder) checkEffectConditions(c *entryErrors, key, path string, conds []string) {
	if len(conds) > maxSpellEffectItems {
		c.at(key, path, ".effect.conditions", ReasonLimit, "an effect has at most %d conditions", maxSpellEffectItems)
	}
	for i, k := range conds {
		at := fmt.Sprintf(".effect.conditions[%d]", i)
		known := b.base.combatEffects != nil && b.base.combatEffects.hasCondition(k)
		switch {
		case !known:
			c.at(key, path, at, ReasonReference, "%q is not an SRD condition", k)
		case slices.Contains(conds[:i], k):
			c.at(key, path, at, ReasonValue, "the condition %q is listed twice", k)
		}
	}
}

func (f *combatEffects) hasCondition(k string) bool {
	_, ok := f.conditions[k]
	return ok
}

func (b *overlayBuilder) checkEffectModifiers(c *entryErrors, key, path string, mods []EffectModifier) {
	if len(mods) > maxSpellEffectItems {
		c.at(key, path, ".effect.modifiers", ReasonLimit, "an effect has at most %d modifiers", maxSpellEffectItems)
	}
	for i, m := range mods {
		at := fmt.Sprintf(".effect.modifiers[%d]", i)
		err := checkModifier(m)
		if err == nil {
			err = b.base.checkModifierContent(m)
		}
		if err == nil {
			err = spellModifierRefusal(m)
		}
		if err != nil {
			c.at(key, path, at+modifierFieldSuffix(err), ReasonValue, "%v", err)
		}
	}
}

// modifierFieldSuffix is ".field" of the modifier's field a refusal is about, or "".
func modifierFieldSuffix(err error) string {
	if fe, ok := errors.AsType[*modifierFieldError](err); ok {
		return "." + fe.field
	}
	return ""
}

// refusedForTableSpell are the kinds of modifier a table spell's effect does not offer.
var refusedForTableSpell = []string{ModifierAura, ModifierBaseAC, ModifierExtraAction, ModifierNoMove, ModifierNoAction, ModifierNoReaction, ModifierConditionImmunity, ModifierTurnTempHP}

// spellModifierRefusal refuses what a modifier may be in the SRD's file but makes no sense for a table
// spell: the base armor class and the extra action need the engine's own working out, and the rest are
// worked out when the spell is cast, which a table spell has no way to say. A table effect's modifiers are a
// closed set: conditions go in the effect's `conditions`, and what stops a creature from moving or acting is a
// condition (paralyzed, restrained), not a modifier.
func spellModifierRefusal(m EffectModifier) error {
	switch {
	case slices.Contains(refusedForTableSpell, m.Kind):
		return modifierField("kind", "%s is not for a table spell", m.Kind)
	case m.FromCaster:
		return modifierField("from_caster", "a table spell cannot take its value from the caster")
	case m.Choose:
		return modifierField("choose", "a table spell cannot ask the caster to choose an ability")
	case m.Kind == ModifierAttackDisadvantage && m.Once:
		return modifierField("once", "attack_disadvantage of a table spell lasts as the effect does, not for one roll")
	case m.Kind == ModifierAttackDisadvantage && m.ExceptSource:
		return modifierField("except_source", "attack_disadvantage of a table spell has no source to spare")
	}
	return nil
}

// tableEffectDef is the effect as the combat reads the SRD's: a copy, so the entry the caller keeps is never
// shared with the content.
func tableEffectDef(ts *TableSpell) *EffectDef {
	e := ts.Effect
	mods := make([]EffectModifier, len(e.Modifiers))
	for i, m := range e.Modifiers {
		m.AppliesTo, m.Abilities = slices.Clone(m.AppliesTo), slices.Clone(m.Abilities)
		m.Conditions, m.Allowed, m.DamageTypes = slices.Clone(m.Conditions), slices.Clone(m.Allowed), slices.Clone(m.DamageTypes)
		mods[i] = m
	}
	if a := e.Aura; a != nil {
		mods = append([]EffectModifier{{Kind: ModifierAura, RadiusFt: a.RadiusFt, Affects: a.Affects}}, mods...)
	}
	d := &EffectDef{
		Key: ts.Key, NamePT: ts.NamePT, Kind: "spell", Concentration: ts.Concentration,
		Conditions: slices.Clone(e.Conditions), Modifiers: mods, Applies: e.Applies,
		PlayerLabelPT: e.PlayerLabelPT, Visibility: e.Visibility, TagsPT: EffectTagsPT(mods),
	}
	if d.Applies == "" {
		d.Applies = "all"
	}
	if d.Visibility == "" {
		d.Visibility = EffectVisibilityOwner
	}
	return d
}
