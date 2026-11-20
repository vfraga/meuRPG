package rules

import (
	"fmt"
	"strings"
)

// How a pick is written in Build.FeatureChoices.
//
// An option of a feature or trait (a fighting style, a pact boon, an
// invocation, a dragon ancestry) is written as its own key, which already says
// where it belongs. A pick that is not a feature (a favored enemy's type and
// language, a terrain, the half-elf's abilities, a spell a feature grants) is
// written "<choice key>=<value key>", so the choice it answers is never a
// guess: two favored enemies that both speak Giant are two different strings,
// and a spell is never taken for another feature's.

const (
	scopedSeparator = "="
	// LanguageNone is the language pick of a favored enemy that speaks none.
	LanguageNone = "language:none"
	// abilityPrefix starts the key of an ability pick ("ability:dex").
	abilityPrefix = "ability:"
	// textSeparator joins a choice key and the number of its free text.
	textSeparator = "#"
)

// ScopedChoice writes the pick of value for the choice with this key.
func ScopedChoice(choiceKey, value string) string {
	return choiceKey + scopedSeparator + value
}

// SplitScopedChoice reads a pick written by ScopedChoice.
func SplitScopedChoice(stored string) (choiceKey, value string, ok bool) {
	choiceKey, value, ok = strings.Cut(stored, scopedSeparator)
	if !ok || choiceKey == "" || value == "" {
		return "", "", false
	}
	return choiceKey, value, true
}

// ChoiceTextKey is the key, in Build.FeatureChoiceText, of the n-th text (from 1)
// a choice takes.
func ChoiceTextKey(choiceKey string, n int) string {
	return fmt.Sprintf("%s%s%d", choiceKey, textSeparator, n)
}

// MaxChoiceTextLength bounds each free text of a choice, in characters.
const MaxChoiceTextLength = 40

// MaxChoiceTexts bounds Build.FeatureChoiceText.
const MaxChoiceTexts = 12

// abilityValue writes an ability as a pick value ("ability:dex").
func abilityValue(a Ability) string { return abilityPrefix + string(a) }

// featPick says the value is a feat picked through a `choice` effect of kind feat
// that the choice key's owner has.
func (c *content) featPick(choiceKey, value string) bool {
	return strings.HasPrefix(value, "feat:") && len(c.effectChoices(strings.TrimSuffix(choiceKey, featChoiceSuffix), "feat")) > 0
}

// choiceValueOK says whether a stored pick has the shape of one: a scoped pick
// whose choice key is a feature, trait, race or spell-granting feature (with an
// optional "#part"), and whose value is a key the content has (or the "no
// language" mark). Whether the choice exists for this character, and whether the
// value is one of its options, is the choice engine's to say.
func (c *content) choiceValueOK(stored string) bool {
	choiceKey, value, ok := SplitScopedChoice(stored)
	if !ok {
		return false
	}
	base, _, _ := strings.Cut(choiceKey, textSeparator)
	if strings.Contains(choiceKey, groupKeyPart) {
		return c.groupValueOK(choiceKey, value)
	}
	if strings.Contains(choiceKey, altKeyPart) {
		return c.alternativeValueOK(choiceKey, value)
	}
	if !c.choiceOwnerKnown(base) && !c.featPick(choiceKey, value) {
		return false
	}
	return c.choiceValueKnown(value)
}

// choiceOwnerKnown says the key of a choice (without its "#part") is a feature,
// trait, race, subrace, background or feat the content has.
func (c *content) choiceOwnerKnown(base string) bool {
	return c.features[base] != nil || c.traits[base] != nil || c.races[base] != nil || c.subraces[base] != nil ||
		c.backgrounds[base] != nil || c.feats[base] != nil
}

// choiceValueKnown says the value of a pick is a key the content has, by its
// kind, or the "no language" mark.
func (c *content) choiceValueKnown(value string) bool {
	switch {
	case value == LanguageNone:
		return true
	case strings.HasPrefix(value, bonusSetPrefix):
		return bonusSetPick(value) >= 0
	case strings.HasPrefix(value, abilityPrefix):
		_, ok := ability(strings.TrimPrefix(value, abilityPrefix))
		return ok
	case strings.HasPrefix(value, "damage-type:"):
		return c.isLiveDamageType(value)
	case strings.HasPrefix(value, "creature-type:"):
		return c.hasCreatureType(value)
	case strings.HasPrefix(value, "terrain:"):
		return c.hasTerrain(value)
	case strings.HasPrefix(value, "proficiency:"):
		_, ok := c.proficiencies[value]
		return ok
	case strings.HasPrefix(value, "language:"):
		_, ok := c.languages[value]
		return ok
	case strings.HasPrefix(value, "feat:"):
		_, ok := c.feats[value]
		return ok
	case strings.HasPrefix(value, "spell:"):
		_, ok := c.spells[value]
		_, hidden := c.featureSummons[value] // the spell of a feature's summon is no pick: it reads as an unknown key
		return ok && !hidden
	}
	return false
}

func (c *content) hasCreatureType(key string) bool {
	if c.choices == nil {
		return false
	}
	return key == c.choices.enemyHumanoid || containsKey(c.choices.enemyTypes, key)
}

func (c *content) hasTerrain(key string) bool {
	return c.choices != nil && containsKey(c.choices.terrains, key)
}

func containsKey(keys []string, key string) bool {
	for _, k := range keys {
		if k == key {
			return true
		}
	}
	return false
}
