package rules

import (
	"fmt"
	"slices"
)

// The typed data of an infusion (TableFeature.Infusion, TableFeature.InfusionRule): an option of a host feature
// that a class puts on a piece of gear, and the host's limit of live infusions. The rules only read and check it
// here; the sheet's infusion list and the attunement and bonus rules read it through Content.Infusion,
// Content.InfusionHost and Content.InfusionLimit.

// The kinds of piece an infusion goes on (TableInfusion.Accepts).
const (
	InfusionAcceptsWeapon            = "weapon"
	InfusionAcceptsArmor             = "armor"
	InfusionAcceptsShield            = "shield"
	InfusionAcceptsArmorOrShield     = "armor_or_shield"
	InfusionAcceptsThrownWeapon      = "thrown_weapon"
	InfusionAcceptsAmmunitionWeapon  = "ammunition_weapon"
	InfusionAcceptsSpellcastingFocus = "spellcasting_focus"
	InfusionAcceptsAnyObject         = "any_object"
)

// The parts of an infusion the app computes (TableInfusion.Effect).
const (
	// InfusionEffectNone is an infusion the app only counts: its text is the table's.
	InfusionEffectNone                = "none"
	InfusionEffectItemBonus           = "item_bonus"
	InfusionEffectSpellFocusBonus     = "spell_focus_bonus"
	InfusionEffectRepeatingAmmunition = "repeating_ammunition"
	InfusionEffectReturning           = "returning"
	InfusionEffectResistanceLabel     = "resistance_label"
)

// MaxInfusionBonus is the highest bonus a step of an infusion gives.
const MaxInfusionBonus = 3

// TableInfusion is what an infusion does to the piece it goes on. It is on an option of a feature that has a
// TableInfusionRule.
type TableInfusion struct {
	// Accepts is the kind of piece it goes on (InfusionAccepts*), required.
	Accepts string
	// RequiresAttunement says the holder must attune to the piece for the infusion to count.
	RequiresAttunement bool
	// Effect is what the app computes (InfusionEffect*), required; none means text only.
	Effect string
	// BonusSteps is the bonus by class level, for the item bonus and the spell focus bonus: levels ascending,
	// each bonus holding from its level up.
	BonusSteps []TableInfusionStep
	// DamageTypes are the damage types the holder picks one of, for the resistance label.
	DamageTypes []string
}

// TableInfusionStep is a bonus that holds from a class level up.
type TableInfusionStep struct {
	ClassLevel int
	Bonus      int
}

// TableInfusionRule is the limit of live infusions a host feature allows, by class level.
type TableInfusionRule struct {
	LimitSteps []TableInfusionLimitStep
}

// TableInfusionLimitStep is the number of live infusions that holds from a class level up.
type TableInfusionLimitStep struct {
	ClassLevel int
	Active     int
}

// BonusAt is the bonus the infusion gives at the class level: the last step at or below it, 0 when there is none.
func (in TableInfusion) BonusAt(classLevel int) int {
	bonus := 0
	for _, s := range in.BonusSteps {
		if s.ClassLevel <= classLevel {
			bonus = s.Bonus
		}
	}
	return bonus
}

// ActiveAt is how many live infusions the rule allows at the class level: the last step at or below it, 0 when none.
func (r TableInfusionRule) ActiveAt(classLevel int) int {
	active := 0
	for _, s := range r.LimitSteps {
		if s.ClassLevel <= classLevel {
			active = s.Active
		}
	}
	return active
}

func (in TableInfusion) clone() TableInfusion {
	in.BonusSteps = slices.Clone(in.BonusSteps)
	in.DamageTypes = slices.Clone(in.DamageTypes)
	return in
}

// infusionBook is what the table layer knows of infusions: the data of each infusion option, the host feature of
// each option and the rule of each host.
type infusionBook struct {
	data  map[string]TableInfusion
	host  map[string]string
	rules map[string]TableInfusionRule
}

func newInfusionBook() infusionBook {
	return infusionBook{data: map[string]TableInfusion{}, host: map[string]string{}, rules: map[string]TableInfusionRule{}}
}

// Infusion is the infusion data of an option key, a copy the caller may change; false when the key is not an infusion.
func (c *Content) Infusion(optionKey string) (TableInfusion, bool) {
	in, ok := c.c.infusions.data[optionKey]
	return in.clone(), ok
}

// InfusionHost is the key of the feature whose option list holds the infusion, or "" when the key is not an infusion.
func (c *Content) InfusionHost(optionKey string) string {
	return c.c.infusions.host[optionKey]
}

// InfusionLimit is how many infusions a character may keep live from the host feature at the class level; 0 when the
// feature has no rule or the level is below its first step.
func (c *Content) InfusionLimit(hostKey string, classLevel int) int {
	return c.c.infusions.rules[hostKey].ActiveAt(classLevel)
}

// infusionKinds says which kinds of piece each effect may go on; none goes on any kind.
var infusionKinds = map[string][]string{
	InfusionEffectNone:                nil,
	InfusionEffectRepeatingAmmunition: {InfusionAcceptsAmmunitionWeapon},
	InfusionEffectReturning:           {InfusionAcceptsThrownWeapon},
	InfusionEffectSpellFocusBonus:     {InfusionAcceptsSpellcastingFocus},
	InfusionEffectResistanceLabel:     {InfusionAcceptsArmor},
	InfusionEffectItemBonus: {
		InfusionAcceptsWeapon, InfusionAcceptsArmor, InfusionAcceptsShield, InfusionAcceptsArmorOrShield,
		InfusionAcceptsThrownWeapon, InfusionAcceptsAmmunitionWeapon,
	},
}

var infusionAccepts = []string{
	InfusionAcceptsWeapon, InfusionAcceptsArmor, InfusionAcceptsShield, InfusionAcceptsArmorOrShield,
	InfusionAcceptsThrownWeapon, InfusionAcceptsAmmunitionWeapon, InfusionAcceptsSpellcastingFocus, InfusionAcceptsAnyObject,
}

// stepLevelsText is what a step with a class level out of order is refused with.
const stepLevelsText = "the class levels of the steps go up, each from 1 to %d"

// levelStepOK says the class level of a step is 1 to MaxLevel and above the level of the step before (prev, 0 for the first).
func levelStepOK(level, prev int) bool { return level >= 1 && level <= MaxLevel && level > prev }

// checkInfusionRule refuses a rule that is not a limit by class level: no steps, levels outside 1 to 20 or not
// ascending, or a step that allows none. Only a class or subclass feature with options has one.
func checkInfusionRule(c *entryErrors, f *TableFeature, k featureKind, path string) {
	r := f.InfusionRule
	switch {
	case k.prefix != featureKeyPrefix:
		c.at(f.Key, path, ".infusion_rule", ReasonValue, "only a class or subclass feature has an infusion rule")
	case len(f.Options) == 0:
		c.at(f.Key, path, ".infusion_rule", ReasonValue, "an infusion rule needs the feature's options: they are the infusions")
	case len(r.LimitSteps) == 0:
		c.at(f.Key, path, ".infusion_rule.limit_steps", ReasonValue, "a rule has at least one step")
	}
	prev := 0
	for i, s := range r.LimitSteps {
		at := fmt.Sprintf(".infusion_rule.limit_steps[%d]", i)
		if !levelStepOK(s.ClassLevel, prev) {
			c.at(f.Key, path, at+".class_level", ReasonValue, stepLevelsText, MaxLevel)
		}
		if s.Active < 1 {
			c.at(f.Key, path, at+".active", ReasonValue, "a step allows at least 1 live infusion")
		}
		prev = s.ClassLevel
	}
}

// addInfusionRule checks the infusion rule of a feature, when it has one, and makes it the rule of the feature's
// infusions.
func (b *overlayBuilder) addInfusionRule(f *TableFeature, k featureKind, path string) error {
	if f.InfusionRule == nil {
		return nil
	}
	var c entryErrors
	checkInfusionRule(&c, f, k, path)
	if err := c.err(); err != nil {
		return err
	}
	b.n.infusions.rules[f.Key] = TableInfusionRule{LimitSteps: slices.Clone(f.InfusionRule.LimitSteps)}
	return nil
}

// addOptionInfusion makes an infusion of the inline option o of host f, when it has the data.
func (b *overlayBuilder) addOptionInfusion(c *entryErrors, f, o *TableFeature, at string) {
	b.addInfusion(c, o.Key, f.Key, at, o.Infusion)
}

// addInfusion checks the infusion of the option key, an option of the feature host, and when it is sound makes it one
// the content knows. The host must have an infusion rule.
func (b *overlayBuilder) addInfusion(c *entryErrors, key, host, at string, in *TableInfusion) {
	if in == nil {
		return
	}
	if _, ok := b.n.infusions.rules[host]; !ok {
		c.at(key, at, ".infusion", ReasonValue, "an infusion is an option of a feature that has an infusion rule")
		return
	}
	before := len(c.list)
	b.checkInfusion(c, key, at+".infusion", in)
	if len(c.list) == before {
		b.n.infusions.data[key] = in.clone()
		b.n.infusions.host[key] = host
	}
}

// checkInfusion checks the data of one infusion, each problem at its own field of path.
func (b *overlayBuilder) checkInfusion(c *entryErrors, key, path string, in *TableInfusion) {
	checkInfusionKinds(c, key, path, in)
	checkInfusionBonus(c, key, path, in)
	b.checkInfusionDamage(c, key, path, in)
}

// checkInfusionKinds checks the piece the infusion goes on and the effect, and that they go together.
func checkInfusionKinds(c *entryErrors, key, path string, in *TableInfusion) {
	accepts := slices.Contains(infusionAccepts, in.Accepts)
	if !accepts {
		c.at(key, path, ".accepts", ReasonValue, "%s", unknownText(in.Accepts, "the kind of piece the infusion goes on is required", "is not a kind of piece an infusion goes on"))
	}
	kinds, known := infusionKinds[in.Effect]
	if !known {
		c.at(key, path, ".effect", ReasonValue, "%s", unknownText(in.Effect, "what the app computes is required: an effect, or none for a text only infusion", "is not an effect of an infusion"))
		return
	}
	if accepts && len(kinds) > 0 && !slices.Contains(kinds, in.Accepts) {
		c.at(key, path, ".accepts", ReasonValue, "the effect %q does not go on a piece of kind %q", in.Effect, in.Accepts)
	}
}

// unknownText is the sentence for a value that is not on a closed list: empty is missing, anything else is unknown.
func unknownText(value, missing, unknown string) string {
	if value == "" {
		return missing
	}
	return fmt.Sprintf("%q %s", value, unknown)
}

// checkInfusionBonus checks the bonus steps: the bonus effects need them and no other effect has them.
func checkInfusionBonus(c *entryErrors, key, path string, in *TableInfusion) {
	bonus := in.Effect == InfusionEffectItemBonus || in.Effect == InfusionEffectSpellFocusBonus
	if _, known := infusionKinds[in.Effect]; !known {
		return
	}
	switch {
	case bonus && len(in.BonusSteps) == 0:
		c.at(key, path, ".bonus_steps", ReasonValue, "a bonus infusion needs the bonus by class level")
	case !bonus && len(in.BonusSteps) > 0:
		c.at(key, path, ".bonus_steps", ReasonValue, "only a bonus infusion has bonus steps")
	}
	prev := 0
	for i, s := range in.BonusSteps {
		at := fmt.Sprintf(".bonus_steps[%d]", i)
		if !levelStepOK(s.ClassLevel, prev) {
			c.at(key, path, at+".class_level", ReasonValue, stepLevelsText, MaxLevel)
		}
		if s.Bonus < 1 || s.Bonus > MaxInfusionBonus {
			c.at(key, path, at+".bonus", ReasonValue, "the bonus is 1 to %d", MaxInfusionBonus)
		}
		prev = s.ClassLevel
	}
}

// checkInfusionDamage checks the damage types a resistance infusion offers: the resistance needs them, each a damage
// type of the app, once each, and no other effect has them.
func (b *overlayBuilder) checkInfusionDamage(c *entryErrors, key, path string, in *TableInfusion) {
	if _, known := infusionKinds[in.Effect]; !known {
		return
	}
	switch {
	case in.Effect == InfusionEffectResistanceLabel && len(in.DamageTypes) == 0:
		c.at(key, path, ".damage_types", ReasonValue, "a resistance infusion lists the damage types the holder picks from")
	case in.Effect != InfusionEffectResistanceLabel && len(in.DamageTypes) > 0:
		c.at(key, path, ".damage_types", ReasonValue, "only a resistance infusion has damage types")
	}
	seen := map[string]bool{}
	for i, d := range in.DamageTypes {
		at := fmt.Sprintf(".damage_types[%d]", i)
		switch {
		case !b.base.isDamageTypeKey(d):
			c.at(key, path, at, ReasonReference, "%q is not a damage type", d)
		case seen[d]:
			c.at(key, path, at, ReasonValue, "the damage types are listed once each")
		}
		seen[d] = true
	}
}
