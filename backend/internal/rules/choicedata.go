package rules

import (
	"fmt"
	"io/fs"
	"slices"
	"strings"
)

// This file reads effects/choices.json, the hand-written data behind the
// choices a character makes at creation, on a locked sheet and at level-up
// (choicegroups.go): the Draconic Ancestry table, the prerequisites of the
// Eldritch Invocations, the Ranger's Favored Enemy types and Natural Explorer
// terrains, and the damage resistances some traits give. Every row names its
// SRD 5.1 source, and the loader refuses a row without one or a key the content
// does not have.

// The shapes of a breath weapon.
const (
	BreathLine = "line"
	BreathCone = "cone"
)

// The kinds of a prerequisite an option can have.
const (
	PrerequisiteLevel   = "level"
	PrerequisiteSpell   = "spell"
	PrerequisiteFeature = "feature"
)

// choiceData is the content of effects/choices.json, indexed.
type choiceData struct {
	// ancestry is the Draconic Ancestry table by the dragonborn's trait key and by
	// the sorcerer's Dragon Ancestor option key.
	ancestryByTrait    map[string]*draconicAncestry
	ancestryByFeature  map[string]*draconicAncestry
	breathTrait        string
	breathDice         []breathDice
	breathSource       string
	prerequisites      map[string]invocationPrerequisite
	enemyTypes         []string
	enemyHumanoid      string
	enemyHumanoidRaces int
	terrains           []string
	resistances        map[string]racialResistance
	// summaries are the one-line rules of the options (OptionSummaries).
	summaries map[string]string
}

type draconicAncestry struct {
	Color      string `json:"color"`
	Trait      string `json:"trait"`
	Feature    string `json:"dragon_ancestor"`
	DamageType string `json:"damage_type"`
	Shape      string `json:"shape"`
	SizeFt     int    `json:"size_ft"`
	WidthFt    int    `json:"width_ft"`
	Save       string `json:"save"`
	Source     string `json:"source"`
}

type breathDice struct {
	Level int    `json:"level"`
	Dice  string `json:"dice"`
}

// invocationPrerequisite is what an Eldritch Invocation asks of the warlock:
// any of a warlock level, a known cantrip and a Pact Boon.
type invocationPrerequisite struct {
	Invocation string `json:"invocation"`
	Level      int    `json:"level,omitempty"`
	Spell      string `json:"spell,omitempty"`
	Feature    string `json:"feature,omitempty"`
	Source     string `json:"source"`
}

type racialResistance struct {
	Trait      string `json:"trait"`
	DamageType string `json:"damage_type"`
	Source     string `json:"source"`
}

// choiceDataFile is the file effects/choices.json.
type choiceDataFile struct {
	Comment          string             `json:"_comment"`
	DraconicAncestry []draconicAncestry `json:"draconic_ancestry"`
	BreathWeapon     struct {
		Trait  string       `json:"trait"`
		Dice   []breathDice `json:"dice"`
		Source string       `json:"source"`
	} `json:"breath_weapon"`
	Invocations  []invocationPrerequisite `json:"invocation_prerequisites"`
	FavoredEnemy struct {
		Types         []string `json:"types"`
		Humanoid      string   `json:"humanoid"`
		HumanoidRaces int      `json:"humanoid_races"`
		Source        string   `json:"source"`
	} `json:"favored_enemy"`
	NaturalExplorer struct {
		Terrains []string `json:"terrains"`
		Source   string   `json:"source"`
	} `json:"natural_explorer"`
	Resistances []racialResistance `json:"racial_resistances"`
	Summaries   map[string]string  `json:"option_summaries"`
}

// choiceDataName is the file loadChoiceData reads, the prefix of its errors.
const choiceDataName = "effects/choices.json"

func choiceDataFail(format string, args ...any) error {
	return fmt.Errorf("%s: %s", choiceDataName, fmt.Sprintf(format, args...))
}

// loadChoiceData reads effects/choices.json and checks every key against the
// content. It must run after the effects (the invocations' keys are features).
// The tables are checked one after the other, in the order of the file, and the
// first problem is returned; the content keeps nothing of a file it refused.
func (c *content) loadChoiceData(fsys fs.FS) error {
	var f choiceDataFile
	if err := readJSON(fsys, choiceDataName, &f); err != nil {
		return err
	}
	d := &choiceData{
		ancestryByTrait:   map[string]*draconicAncestry{},
		ancestryByFeature: map[string]*draconicAncestry{},
		prerequisites:     map[string]invocationPrerequisite{},
		resistances:       map[string]racialResistance{},
	}
	for _, load := range [...]func(*choiceDataFile, *choiceData) error{
		c.loadAncestries, c.loadBreathWeapon, c.loadInvocations, c.loadFavoredEnemy,
		c.loadNaturalExplorer, c.loadResistances, c.loadSummaries, c.checkChoiceNames,
	} {
		if err := load(&f, d); err != nil {
			return err
		}
	}
	c.choices = d
	return nil
}

// isLiveDamageType says the key is a damage type the content has a name for.
func (c *content) isLiveDamageType(k string) bool {
	n, ok := c.named[k]
	return ok && n != nil && strings.HasPrefix(k, "damage-type:")
}

// loadAncestries reads the Draconic Ancestry table.
func (c *content) loadAncestries(f *choiceDataFile, d *choiceData) error {
	for i := range f.DraconicAncestry {
		a := &f.DraconicAncestry[i]
		if err := c.checkAncestry(a, d); err != nil {
			return err
		}
		d.ancestryByTrait[a.Trait], d.ancestryByFeature[a.Feature] = a, a
	}
	return nil
}

// checkAncestry checks one row of the Draconic Ancestry table against the content
// and the rows before it (d).
func (c *content) checkAncestry(a *draconicAncestry, d *choiceData) error {
	if a.Source == "" {
		return choiceDataFail("draconic ancestry %q needs a source", a.Color)
	}
	if _, ok := c.traits[a.Trait]; !ok {
		return choiceDataFail("draconic ancestry %q: unknown trait %q", a.Color, a.Trait)
	}
	if _, ok := c.features[a.Feature]; !ok {
		return choiceDataFail("draconic ancestry %q: unknown feature %q", a.Color, a.Feature)
	}
	if !c.isLiveDamageType(a.DamageType) {
		return choiceDataFail("draconic ancestry %q: %q is not a damage type", a.Color, a.DamageType)
	}
	if a.Shape != BreathLine && a.Shape != BreathCone {
		return choiceDataFail("draconic ancestry %q: shape %q is not line or cone", a.Color, a.Shape)
	}
	if a.SizeFt <= 0 || (a.Shape == BreathLine) != (a.WidthFt > 0) {
		return choiceDataFail("draconic ancestry %q: a line has a size and a width, a cone only a size", a.Color)
	}
	if _, ok := ability(a.Save); !ok {
		return choiceDataFail("draconic ancestry %q: %q is not an ability", a.Color, a.Save)
	}
	if d.ancestryByTrait[a.Trait] != nil || d.ancestryByFeature[a.Feature] != nil {
		return choiceDataFail("draconic ancestry %q is listed twice", a.Color)
	}
	return nil
}

// loadBreathWeapon reads the breath weapon's damage by level.
func (c *content) loadBreathWeapon(f *choiceDataFile, d *choiceData) error {
	bw := &f.BreathWeapon
	if _, ok := c.traits[bw.Trait]; !ok || bw.Source == "" || len(bw.Dice) == 0 {
		return choiceDataFail("breath_weapon needs a known trait, dice and a source")
	}
	d.breathTrait, d.breathSource = bw.Trait, bw.Source
	for i, row := range bw.Dice {
		if _, ok := ParseDice(row.Dice); !ok || row.Level < 1 || row.Level > MaxLevel || (i > 0 && row.Level <= bw.Dice[i-1].Level) {
			return choiceDataFail("breath_weapon dice %q at level %d: dice and ascending levels expected", row.Dice, row.Level)
		}
	}
	d.breathDice = bw.Dice
	return nil
}

// loadInvocations reads the prerequisites of the Eldritch Invocations; every
// invocation the content has needs a row.
func (c *content) loadInvocations(f *choiceDataFile, d *choiceData) error {
	for _, p := range f.Invocations {
		if err := c.checkInvocation(p, d); err != nil {
			return err
		}
		d.prerequisites[p.Invocation] = p
	}
	for _, key := range c.features[invocationsFeature].Options {
		if _, ok := d.prerequisites[key]; !ok {
			return choiceDataFail("invocation %q has no row: list it even when it asks for nothing", key)
		}
	}
	return nil
}

// checkInvocation checks one row of the invocations' prerequisites against the
// content and the rows before it (d).
func (c *content) checkInvocation(p invocationPrerequisite, d *choiceData) error {
	if p.Source == "" {
		return choiceDataFail("invocation %q needs a source", p.Invocation)
	}
	inv, ok := c.features[p.Invocation]
	if !ok || inv.Parent != invocationsFeature {
		return choiceDataFail("%q is not an Eldritch Invocation", p.Invocation)
	}
	if _, dup := d.prerequisites[p.Invocation]; dup {
		return choiceDataFail("invocation %q is listed twice", p.Invocation)
	}
	if p.Spell != "" {
		if s, ok := c.spells[p.Spell]; !ok || s.Level != 0 {
			return choiceDataFail("invocation %q: %q is not a known cantrip", p.Invocation, p.Spell)
		}
	}
	if p.Feature != "" {
		if pf, ok := c.features[p.Feature]; !ok || pf.Parent != "feature:pact-boon" {
			return choiceDataFail("invocation %q: %q is not a Pact Boon", p.Invocation, p.Feature)
		}
	}
	if p.Level < 0 || p.Level > MaxLevel {
		return choiceDataFail("invocation %q: level %d is not 0 to %d", p.Invocation, p.Level, MaxLevel)
	}
	return nil
}

// loadFavoredEnemy reads the Ranger's Favored Enemy types.
func (c *content) loadFavoredEnemy(f *choiceDataFile, d *choiceData) error {
	fe := f.FavoredEnemy
	if fe.Source == "" || len(fe.Types) == 0 || fe.Humanoid == "" || fe.HumanoidRaces < 1 || hasDuplicate(fe.Types) {
		return choiceDataFail("favored_enemy needs types, the humanoid key, a race count and a source")
	}
	d.enemyTypes, d.enemyHumanoid, d.enemyHumanoidRaces = fe.Types, fe.Humanoid, fe.HumanoidRaces
	for _, k := range append(slices.Clone(fe.Types), fe.Humanoid) {
		if !strings.HasPrefix(k, "creature-type:") {
			return choiceDataFail("favored enemy %q is not a creature-type key", k)
		}
	}
	return nil
}

// loadNaturalExplorer reads the Ranger's Natural Explorer terrains.
func (c *content) loadNaturalExplorer(f *choiceDataFile, d *choiceData) error {
	ne := f.NaturalExplorer
	if ne.Source == "" || len(ne.Terrains) == 0 || hasDuplicate(ne.Terrains) {
		return choiceDataFail("natural_explorer needs terrains and a source")
	}
	for _, k := range ne.Terrains {
		if !strings.HasPrefix(k, "terrain:") {
			return choiceDataFail("terrain %q is not a terrain key", k)
		}
	}
	d.terrains = ne.Terrains
	return nil
}

// loadResistances reads the damage resistances some traits give.
func (c *content) loadResistances(f *choiceDataFile, d *choiceData) error {
	for _, r := range f.Resistances {
		if _, ok := c.traits[r.Trait]; !ok || !c.isLiveDamageType(r.DamageType) || r.Source == "" {
			return choiceDataFail("racial resistance of %q needs a known trait, a damage type and a source", r.Trait)
		}
		d.resistances[r.Trait] = r
	}
	return nil
}

// loadSummaries reads the one-line rules of the options. Every option a feature
// offers says its rule in a line, except the ones whose line the engine writes
// (the dragon ancestors and the terrains of the Land).
func (c *content) loadSummaries(f *choiceDataFile, d *choiceData) error {
	d.summaries = f.Summaries
	for k, text := range f.Summaries {
		if _, ok := c.features[k]; !ok || strings.TrimSpace(text) == "" {
			return choiceDataFail("option summary %q: a known feature and a text expected", k)
		}
	}
	for _, parent := range c.features {
		for _, key := range parent.Options {
			if _, isAncestor := d.ancestryByFeature[key]; isAncestor || strings.HasPrefix(key, "feature:circle-of-the-land-") {
				continue
			}
			if _, ok := f.Summaries[key]; !ok {
				return choiceDataFail("option %q has no summary", key)
			}
		}
	}
	return nil
}

// checkChoiceNames asks for a Portuguese name for each key this file introduces.
func (c *content) checkChoiceNames(_ *choiceDataFile, d *choiceData) error {
	for _, k := range append(slices.Clone(d.enemyTypes), append([]string{d.enemyHumanoid}, d.terrains...)...) {
		if _, ok := c.namesPT[k]; !ok {
			return choiceDataFail("%s has no Portuguese name in effects/names_pt.json", k)
		}
	}
	return nil
}

func hasDuplicate(keys []string) bool {
	for i, k := range keys {
		if slices.Contains(keys[:i], k) {
			return true
		}
	}
	return false
}
