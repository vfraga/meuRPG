package rules

import (
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"sort"
	"testing"
)

// sectionDigest is "n=<entries> sha=<16 hex>" of a value's JSON, so a diff in the
// golden names the section that changed without a megabyte of catalog in git.
func sectionDigest(v any, n int) string {
	raw, err := json.Marshal(v)
	if err != nil {
		return "marshal: " + err.Error()
	}
	return fmt.Sprintf("n=%d sha=%x", n, sha256.Sum256(raw))[:len(fmt.Sprintf("n=%d sha=", n))+16]
}

// catalogDigests is everything buildCatalog fills, section by section.
func catalogDigests(c *content) map[string]string {
	cat := c.catalog
	out := map[string]string{
		"version":          cat.ContentVersion + "|" + cat.Attribution,
		"abilities":        sectionDigest(cat.Abilities, len(cat.Abilities)),
		"races":            sectionDigest(cat.Races, len(cat.Races)),
		"subraces":         sectionDigest(cat.Subraces, len(cat.Subraces)),
		"classes":          sectionDigest(cat.Classes, len(cat.Classes)),
		"subclasses":       sectionDigest(cat.Subclasses, len(cat.Subclasses)),
		"backgrounds":      sectionDigest(cat.Backgrounds, len(cat.Backgrounds)),
		"skills":           sectionDigest(cat.Skills, len(cat.Skills)),
		"armor":            sectionDigest(cat.Armor, len(cat.Armor)),
		"weapons":          sectionDigest(cat.Weapons, len(cat.Weapons)),
		"spells":           sectionDigest(cat.Spells, len(cat.Spells)),
		"challengeRatings": sectionDigest(cat.ChallengeRatings, len(cat.ChallengeRatings)),
		"levelXP":          sectionDigest(cat.LevelXP, len(cat.LevelXP)),
		"languages":        sectionDigest(cat.Languages, len(cat.Languages)),
		"proficiencies":    sectionDigest(cat.Proficiencies, len(cat.Proficiencies)),
		"damageTypes":      sectionDigest(cat.DamageTypes, len(cat.DamageTypes)),
		"spellEntries":     sectionDigest(c.spellEntries, len(c.spellEntries)),
		"spellDetails":     sectionDigest(c.spellDetails, len(c.spellDetails)),
	}
	return out
}

// sharedDetails lists the spells whose details are the very ones of base: the
// reuse buildCatalog does for a With.
func sharedDetails(c, base *content) string {
	var shared, fresh int
	var changed []string
	for k, d := range c.spellDetails {
		if b, ok := base.spellDetails[k]; ok {
			if b == d {
				shared++
			} else {
				changed = append(changed, k)
			}
			continue
		}
		fresh++
	}
	sort.Strings(changed)
	return fmt.Sprintf("shared=%d fresh=%d rebuilt: %s", shared, fresh, sectionDigest(changed, len(changed)))
}

// TestBuildCatalogCharacterization freezes the catalog, the spell entries and the
// spell details buildCatalog fills, for the SRD and for a table content with
// every kind of entry (a third caster, a class that reuses a spell list, an
// archived entry, an entry switched off), and the details it shares with the base.
func TestBuildCatalogCharacterization(t *testing.T) {
	t.Parallel()
	srd := loadForTest(t)
	got := map[string]string{}
	for k, v := range catalogDigests(srd.c) {
		got["srd: "+k] = v
	}

	o := fullOverlay(t, srd)
	o.Subclasses = append(o.Subclasses, thirdCaster(srd, "terco-catalogo", "class:fighter", "class:wizard", PreparationKnown))
	o.Classes[0].Archived = true
	o.Races[0].Archived = true
	o.Spells[0].Archived = true
	o.Off = []string{"class:barbarian", "spell:fireball", "race:elf"}
	c := withOverlayOn(t, srd, o)
	for k, v := range catalogDigests(c.c) {
		got["table: "+k] = v
	}
	got["table: shared details"] = sharedDetails(c.c, srd.c)

	// An overlay with nothing in it shares every detail of the base.
	empty := withOverlayOn(t, srd, Overlay{Revision: 1})
	for k, v := range catalogDigests(empty.c) {
		got["empty: "+k] = v
	}
	got["empty: shared details"] = sharedDetails(empty.c, srd.c)
	goldenText(t, "catalog", got)
}
