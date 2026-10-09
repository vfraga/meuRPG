package rules

import (
	"fmt"
	"io/fs"
	"strings"
)

// effects/monster_traits.json says which traits of a stat block the engine applies (a trait such
// as Pack Tactics that gives advantage): the rest are reminders the app shows to the master and
// never applies ("Lembrete: o app não aplica este texto"). A trait enters the file when the
// engine reads it, with the SRD source; loading refuses a trait no creature has, a repeated one
// and one without a source, and the file follows the revision rule of CONTRIBUTING.md.

// loadMonsterTraits reads the table of the traits the engine applies.
func (c *content) loadMonsterTraits(fsys fs.FS) error {
	const name = "effects/monster_traits.json"
	var f struct {
		Comment string `json:"_comment"` // the file explains itself
		Traits  []struct {
			Trait  string `json:"trait"`
			Reads  string `json:"reads"`
			Source string `json:"source"`
		} `json:"engine_reads"`
	}
	if err := readJSON(fsys, name, &f); err != nil {
		return err
	}
	c.engineTraits = map[string]bool{}
	for _, t := range f.Traits {
		switch {
		case t.Trait == "" || t.Source == "" || strings.TrimSpace(t.Reads) == "":
			return fmt.Errorf("%s: %q needs what the engine reads and a source", name, t.Trait)
		case c.engineTraits[t.Trait]:
			return fmt.Errorf("%s: %q is listed twice", name, t.Trait)
		case !c.hasTrait(t.Trait):
			return fmt.Errorf("%s: no creature has the trait %q", name, t.Trait)
		}
		c.engineTraits[t.Trait] = true
	}
	return nil
}

// hasTrait says whether some creature has a trait (a special ability) with this name.
func (c *content) hasTrait(name string) bool {
	for _, m := range c.monsters {
		for _, a := range m.SpecialAbilities {
			if a.Name == name {
				return true
			}
		}
	}
	return false
}
