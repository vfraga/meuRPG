package rules

import (
	"fmt"
	"io/fs"
	"strings"
)

// Spell scrolls (SRD 5.1, "Spell Scroll"): the level of the spell on a scroll sets the scroll's own saving throw DC and
// spell attack bonus, not the reader's. effects/spell_scrolls.json holds them for each variant of the family.

// spellScrollFamily is the catalog key of the Spell Scroll family, whose variants are the scrolls a sheet holds.
const spellScrollFamily = "item:spell-scroll"

// Bounds of the numbers of a scroll: the saving throw DC runs from 10 up to 30, and the attack bonus is the DC less 8.
const (
	spellScrollMinDC       = 10
	spellScrollMaxDC       = 30
	spellScrollDCOverBonus = 8
)

// SpellScrollNumbers is what a scroll of one spell level carries.
type SpellScrollNumbers struct {
	// Level is the level of the spell on the scroll: 0 for a cantrip, 1 to 9 otherwise.
	Level int
	// SaveDC is the saving throw DC of the spell cast from the scroll.
	SaveDC int
	// AttackBonus is the spell attack bonus of the spell cast from the scroll.
	AttackBonus int
}

// spellScrollFile is one entry of effects/spell_scrolls.json, which maps a scroll variant's key to it.
type spellScrollFile struct {
	Level       int `json:"level"`
	SaveDC      int `json:"save_dc"`
	AttackBonus int `json:"attack_bonus"`
}

// loadSpellScrolls reads effects/spell_scrolls.json and refuses a key that is not a variant of the Spell Scroll family,
// a level that is not the variant's, a level, DC or bonus out of range, a bonus that is not the DC less 8, and a variant
// of the family with no row.
func (c *content) loadSpellScrolls(fsys fs.FS) error {
	const name = "effects/spell_scrolls.json"
	var f map[string]spellScrollFile
	if err := readJSON(fsys, name, &f); err != nil {
		return err
	}
	c.spellScrolls = map[string]SpellScrollNumbers{}
	for _, k := range sortedKeys(f) {
		n, err := c.spellScrollOf(k, f[k])
		if err != nil {
			return fmt.Errorf("%s: %s: %w", name, k, err)
		}
		c.spellScrolls[k] = n
	}
	if fam := c.magicItems[spellScrollFamily]; fam != nil {
		for _, v := range fam.Variants {
			if _, ok := c.spellScrolls[v]; !ok {
				return fmt.Errorf("%s: %s has no row", name, v)
			}
		}
	}
	return nil
}

func (c *content) spellScrollOf(key string, e spellScrollFile) (SpellScrollNumbers, error) {
	m, ok := c.magicItems[key]
	if !ok || m.VariantOf != spellScrollFamily {
		return SpellScrollNumbers{}, fmt.Errorf("is not a variant of the Spell Scroll")
	}
	switch level, known := spellScrollLevelOf(key); {
	case !known || e.Level != level:
		return SpellScrollNumbers{}, fmt.Errorf("level is the level of the variant (a cantrip is 0)")
	case e.SaveDC < spellScrollMinDC || e.SaveDC > spellScrollMaxDC:
		return SpellScrollNumbers{}, fmt.Errorf("save_dc is %d to %d", spellScrollMinDC, spellScrollMaxDC)
	case e.AttackBonus != e.SaveDC-spellScrollDCOverBonus:
		return SpellScrollNumbers{}, fmt.Errorf("attack_bonus is save_dc less %d", spellScrollDCOverBonus)
	}
	return SpellScrollNumbers(e), nil
}

// spellScrollSuffixes are the suffixes of the variants' keys, at the index of the spell level each names (a cantrip is 0).
var spellScrollSuffixes = [...]string{"cantrip", "1st", "2nd", "3rd", "4th", "5th", "6th", "7th", "8th", "9th"}

// spellScrollLevelOf is the spell level a variant's key names ("item:spell-scroll-cantrip" is 0, "item:spell-scroll-3rd" is 3).
func spellScrollLevelOf(key string) (int, bool) {
	suffix, ok := strings.CutPrefix(key, spellScrollFamily+"-")
	if !ok {
		return 0, false
	}
	for level, s := range spellScrollSuffixes {
		if suffix == s {
			return level, true
		}
	}
	return 0, false
}

// SpellScrollNumbers is the spell level, saving throw DC and spell attack bonus of the scroll with that item key, and
// false for a key that is no variant of the Spell Scroll (the family included).
func (c *Content) SpellScrollNumbers(key string) (SpellScrollNumbers, bool) {
	n, ok := c.c.spellScrolls[key]
	return n, ok
}
