package rules

import (
	"fmt"
	"strings"
	"unicode/utf8"
)

// validate checks what cannot make sense at all, before a sheet is written:
// ranges, list sizes, keys that exist and belong together. Everything that
// is only unusual is left to Derive's Issues. The checks run in the order of
// the Build's fields and the first refusal is the one returned.
func validate(b Build, c *content) error {
	for _, check := range []func(*Build, *content) error{
		validateAbilities, validateRace, validateClasses, validateBackground,
		validateCustomBackground, validateSkills, validateHitPoints, validateGear,
		validateSpells, validateChoices,
	} {
		if err := check(&b, c); err != nil {
			return err
		}
	}
	return nil
}

// invalid is the refusal of one field.
func invalid(field, format string, args ...any) error {
	return &ValidationError{Field: field, Message: fmt.Sprintf(format, args...)}
}

// validateAbilities checks the six scores and the manual bonuses.
func validateAbilities(b *Build, _ *content) error {
	for _, a := range AllAbilities() {
		field := "full.base_scores." + protoAbility[a]
		v, ok := b.BaseScores[a]
		if !ok {
			return invalid(field, "is required")
		}
		if v < MinScore || v > MaxScore {
			return invalid(field, "must be %d to %d", MinScore, MaxScore)
		}
	}
	for a := range b.BaseScores {
		if _, ok := abilityIndex[a]; !ok {
			return invalid("full.base_scores", "has an unknown ability")
		}
	}
	for a, v := range b.ExtraAbilityBonuses {
		if _, ok := abilityIndex[a]; !ok {
			return invalid("full.extra_ability_bonuses", "has an unknown ability")
		}
		if v < -MaxManualBonus || v > MaxManualBonus {
			return invalid("full.extra_ability_bonuses."+protoAbility[a], "must be %d to %d", -MaxManualBonus, MaxManualBonus)
		}
	}
	return nil
}

// validateRace checks the race and the subrace.
func validateRace(b *Build, c *content) error {
	if b.Race == "" {
		return invalid("full.race_key", "is required")
	}
	if _, ok := c.races[b.Race]; !ok {
		return invalid("full.race_key", "is not a known race")
	}
	if b.Subrace != "" {
		s, ok := c.subraces[b.Subrace]
		if !ok || s.Race != b.Race {
			return invalid("full.subrace_key", "is not a subrace of the race")
		}
	}
	return nil
}

// validateClasses checks the list of classes, each one and the level total.
func validateClasses(b *Build, c *content) error {
	if len(b.Classes) == 0 {
		return invalid("full.classes", "must have at least one class")
	}
	if len(b.Classes) > MaxClasses {
		return invalid("full.classes", "must have at most %d classes", MaxClasses)
	}
	total := 0
	seen := map[string]bool{}
	for i, cl := range b.Classes {
		if err := validateClass(c, cl, fmt.Sprintf("full.classes[%d]", i), seen); err != nil {
			return err
		}
		total += cl.Level
	}
	if total > MaxLevel {
		return invalid("full.classes", "levels must add up to at most %d", MaxLevel)
	}
	return nil
}

// validateClass checks one entry of the class list; seen holds the classes
// before it.
func validateClass(c *content, cl ClassLevel, field string, seen map[string]bool) error {
	if _, ok := c.classes[cl.Class]; !ok {
		return invalid(field+".class_key", "is not a known class")
	}
	if seen[cl.Class] {
		return invalid(field+".class_key", "repeats a class")
	}
	seen[cl.Class] = true
	if cl.Level < 1 || cl.Level > MaxLevel {
		return invalid(field+".level", "must be 1 to %d", MaxLevel)
	}
	if cl.Subclass != "" && cl.CustomSubclassName != "" {
		return invalid(field, "has both a subclass and a custom subclass")
	}
	if cl.Subclass != "" {
		s, ok := c.subclasses[cl.Subclass]
		if !ok || s.Class != cl.Class {
			return invalid(field+".subclass_key", "is not a subclass of the class")
		}
	}
	if err := checkName(cl.CustomSubclassName); err != "" {
		return invalid(field+".custom_subclass_name", "%s", err)
	}
	return nil
}

// validateBackground checks the background key, which excludes the custom one.
func validateBackground(b *Build, c *content) error {
	if b.Background == "" {
		return nil
	}
	if _, ok := c.backgrounds[b.Background]; !ok {
		return invalid("full.background_key", "is not a known background")
	}
	if b.hasCustomBackground() {
		return invalid("full.custom_background", "cannot be set with background_key")
	}
	return nil
}

// hasCustomBackground says any field of the player's own background is set.
func (b *Build) hasCustomBackground() bool {
	return b.CustomBackgroundName != "" || len(b.CustomBackgroundSkills) > 0 || len(b.CustomBackgroundProficiencies) > 0 ||
		b.CustomBackgroundFeatureName != "" || b.CustomBackgroundFeature != "" || b.CustomBackgroundEquipment != ""
}

// validateCustomBackground checks the fields of the player's own background.
func validateCustomBackground(b *Build, c *content) error {
	if err := checkName(b.CustomBackgroundName); err != "" {
		return invalid("full.custom_background.name", "%s", err)
	}
	if err := checkKeys(c, "full.custom_background.skill_keys", b.CustomBackgroundSkills, CustomBackgroundSkillCount, isSkill); err != nil {
		return err
	}
	// Two tools or languages in total, in any mix (SRD 5.1, "Customizing a
	// Background"); fewer shows an Issue, as for the skills.
	if err := checkKeys(c, "full.custom_background.proficiency_keys", b.CustomBackgroundProficiencies, CustomBackgroundProficiencyCount, isToolOrLanguage); err != nil {
		return err
	}
	if err := checkName(b.CustomBackgroundFeatureName); err != "" {
		return invalid("full.custom_background.feature_name", "%s", err)
	}
	if utf8.RuneCountInString(b.CustomBackgroundFeature) > MaxCustomFeatureTextLength {
		return invalid("full.custom_background.feature_text", "must be at most %d characters", MaxCustomFeatureTextLength)
	}
	if utf8.RuneCountInString(b.CustomBackgroundEquipment) > MaxCustomEquipmentLength {
		return invalid("full.custom_background.equipment", "must be at most %d characters", MaxCustomEquipmentLength)
	}
	return nil
}

// validateSkills checks the skill proficiencies and the expertise.
func validateSkills(b *Build, c *content) error {
	if err := checkKeys(c, "full.skill_proficiency_keys", b.SkillProficiencies, MaxSkillKeys, isSkill); err != nil {
		return err
	}
	return checkKeys(c, "full.expertise_skill_keys", b.Expertise, MaxSkillKeys, isSkill)
}

// validateHitPoints checks the method and the rolls.
func validateHitPoints(b *Build, _ *content) error {
	if b.HitPoints.Method != HitPointsFixed && b.HitPoints.Method != HitPointsRolled {
		return invalid("full.hit_points.method", "is not a known method")
	}
	if len(b.HitPoints.Rolls) > MaxHitPointRolls {
		return invalid("full.hit_points.rolls", "must have at most %d rolls", MaxHitPointRolls)
	}
	for i, r := range b.HitPoints.Rolls {
		if r < 1 || r > MaxRoll {
			return invalid(fmt.Sprintf("full.hit_points.rolls[%d]", i), "must be 1 to %d", MaxRoll)
		}
	}
	return nil
}

// validateGear checks the worn armor and the weapons.
func validateGear(b *Build, c *content) error {
	if b.Armor != "" {
		e, ok := c.equipment[b.Armor]
		if !ok || e.Armor == nil || e.Armor.Category == "shield" {
			return invalid("full.armor_key", "is not a known body armor")
		}
	}
	return checkKeys(c, "full.weapon_keys", b.Weapons, MaxWeapons, isWeapon)
}

// validateSpells checks the cantrips and the known and prepared spells.
func validateSpells(b *Build, c *content) error {
	if err := checkKeys(c, "full.cantrip_keys", b.Cantrips, MaxCantrips, isCantrip); err != nil {
		return err
	}
	if err := checkKeys(c, "full.known_spell_keys", b.SpellsKnown, MaxKnownSpells, isLeveledSpell); err != nil {
		return err
	}
	return checkKeys(c, "full.prepared_spell_keys", b.SpellsPrepared, MaxPreparedSpells, isLeveledSpell)
}

// validateChoices checks the texts of the choices, the picks and the feats.
func validateChoices(b *Build, c *content) error {
	if len(b.FeatureChoiceText) > MaxChoiceTexts {
		return invalid("full.feature_choice_text", "must have at most %d entries", MaxChoiceTexts)
	}
	for k, v := range b.FeatureChoiceText {
		if _, _, ok := strings.Cut(k, textSeparator); !ok || utf8.RuneCountInString(v) > MaxChoiceTextLength {
			return invalid("full.feature_choice_text", "has a key that is no choice's or a text over %d characters", MaxChoiceTextLength)
		}
	}
	if err := checkKeys(c, "full.feature_choice_keys", b.FeatureChoices, MaxListLength, isChoicePick); err != nil {
		return err
	}
	return checkKeys(c, "full.feat_keys", b.Feats, MaxListLength, isFeat)
}

// isChoicePick: an option of a feature or trait, or a pick of the choice engine.
func isChoicePick(c *content, k string) bool { return isOption(c, k) || c.choiceValueOK(k) }

// checkName checks a custom name's length; the characters module cleans
// the text itself (one line, no control characters).
func checkName(s string) string {
	if n := utf8.RuneCountInString(s); n > MaxCustomNameLength {
		return fmt.Sprintf("must be at most %d characters", MaxCustomNameLength)
	}
	if strings.TrimSpace(s) != s {
		return "must not start or end with spaces"
	}
	return ""
}

// checkKeys checks a list of keys: at most limit, no repeats, each of the
// right kind.
func checkKeys(c *content, field string, keys []string, limit int, valid func(*content, string) bool) error {
	if len(keys) > limit {
		return &ValidationError{Field: field, Message: fmt.Sprintf("must have at most %d entries", limit)}
	}
	seen := make(map[string]bool, len(keys))
	for i, k := range keys {
		f := fmt.Sprintf("%s[%d]", field, i)
		if seen[k] {
			return &ValidationError{Field: f, Message: "repeats an entry"}
		}
		seen[k] = true
		if !valid(c, k) {
			return &ValidationError{Field: f, Message: "is not a known key of the right kind"}
		}
	}
	return nil
}

func isSkill(c *content, k string) bool {
	_, ok := c.skills[k]
	return ok
}

// isToolOrLanguage: a tool proficiency (the kind a table background's tools
// are) or a language of the SRD.
func isToolOrLanguage(c *content, k string) bool {
	if p, ok := c.proficiencies[k]; ok {
		return p.Kind == "tool" || p.Kind == "other"
	}
	_, ok := c.languages[k]
	return ok
}

func isWeapon(c *content, k string) bool {
	e, ok := c.equipment[k]
	return ok && e.Weapon != nil
}

func isCantrip(c *content, k string) bool {
	s, ok := c.spells[k]
	return ok && s.Level == 0
}

func isLeveledSpell(c *content, k string) bool {
	s, ok := c.spells[k]
	return ok && s.Level > 0
}

// isFeat: a feat of the SRD or the table.
func isFeat(c *content, k string) bool {
	_, ok := c.feats[k]
	return ok
}

// isOption: a feature or trait that is an option of another one.
func isOption(c *content, k string) bool {
	if f, ok := c.features[k]; ok {
		return f.Parent != "" || c.optionParents[k] != ""
	}
	t, ok := c.traits[k]
	return ok && t.Parent != ""
}
