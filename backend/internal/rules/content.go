package rules

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io/fs"
	"path"
	"regexp"
	"slices"
	"strconv"
	"strings"
	"time"

	"github.com/PuraFome/meuRPG/backend/internal/rules/encounter"
	"github.com/PuraFome/meuRPG/backend/internal/rules/formula"
	"github.com/PuraFome/meuRPG/backend/internal/rules/srd51"
)

// content is the loaded rules content, indexed by key. It is built once by
// load and never changed afterwards, so Derive can read it from many
// requests at once.
type content struct {
	version  string
	manifest srd51.Manifest

	abilities     map[Ability]srd51.AbilityScore
	skills        map[string]*srd51.Skill
	skillOrder    []string
	races         map[string]*srd51.Race
	subraces      map[string]*srd51.Subrace
	traits        map[string]*srd51.Trait
	classes       map[string]*srd51.Class
	subclasses    map[string]*srd51.Subclass
	features      map[string]*srd51.Feature
	backgrounds   map[string]*srd51.Background
	feats         map[string]*srd51.Feat
	proficiencies map[string]*srd51.Proficiency
	equipment     map[string]*srd51.Equipment
	spells        map[string]*srd51.Spell
	monsters      map[string]*srd51.Monster
	// attacksPerAction holds the creature corrections of effects/corrections.json:
	// the Multiattack count the snapshot has wrong, by creature key.
	attacksPerAction map[string]int
	// monsterPlans are the plans of the creatures' actions (monsterplan.go), by
	// creature key.
	monsterPlans map[string]*MonsterPlan
	magicItems   map[string]*srd51.MagicItem
	languages    map[string]*srd51.Language
	named        map[string]*srd51.Named

	// classLevels[class][n-1] is row n of the class table, and
	// subclassLevels[subclass][n] the subclass row at class level n.
	classLevels    map[string][]*srd51.Level
	subclassLevels map[string]map[int]*srd51.Level

	// effects are the hand-written effects, by the key they belong to.
	effects map[string][]*Effect
	// spellEffects are the spells that read hit points, by spell key
	// (effects/spells.json).
	spellEffects map[string]spellEffectDef
	// coverIgnoring are the spells whose saving throw gets no benefit from cover
	// (the "ignores_cover" kind of effects/spells.json), by spell key.
	coverIgnoring map[string]bool
	// missHalving are the spell attacks that deal half their damage on a miss (the
	// "half_on_miss" kind of effects/spells.json), by spell key.
	missHalving map[string]bool
	// revives are the spells that bring the dead back (the "revive" kind), by spell key.
	revives map[string]ReviveSpec
	// summons are the spells that summon a creature (the "summon" kind of
	// effects/spells.json), by spell key.
	summons map[string]*summonDef
	// monsterEntries are the creatures as the lists show them, sorted by
	// Portuguese name.
	monsterEntries []CreatureEntry
	// magicItemEntries are the magic items as the lists show them, sorted by
	// Portuguese name.
	magicItemEntries []MagicItemEntry
	// magicUnits are the rolling units of each rarity (MagicItemUnits).
	magicUnits map[string][]MagicItemUnit
	// consumables are the keys of the single-use items (effects/consumables.json).
	consumables map[string]bool
	// treasure is the generator's content: the SRD 5.2.1 values of the magic
	// items and our tables of coins, gems and art (effects/magic_item_values.json
	// and effects/treasure.json).
	treasure treasureTables
	// traps are the trap presets and the SRD's severity tables
	// (effects/traps.json), and lights the light presets (effects/lights.json).
	traps  traps
	lights []LightPreset
	// combatEffects are the effects that last in a combat (effects/combat_effects.json).
	combatEffects *combatEffects
	// encounterBudget is the XP per character of each level, band by band, index 0
	// being level 1 (effects/encounter_budget.json, SRD 5.2.1).
	encounterBudget []encounter.Budget
	// standardActions are the actions every character has.
	standardActions []Action
	// levelXP[n-1] is the XP to reach level n, and ratings the SRD's challenge
	// ratings with their XP (effects/advancement.json).
	levelXP []int
	ratings []ChallengeRating
	// casting is each casting class's spellcasting effect, and the class
	// level it starts at. subCasting is the same for a subclass that casts on
	// its own (a third caster, table content only), by subclass key.
	casting    map[string]classCasting
	subCasting map[string]classCasting
	// multiclassTable is the SRD full caster whose class table is the
	// multiclass spellcaster table (see multiclassSlots).
	multiclassTable string

	namesPT map[string]string
	namesEN map[string]string

	// spellTextsPT and itemTextsPT are the Portuguese texts of the SRD spells and
	// magic items (texts_pt.go), by key; an entry the files lack falls back to the English.
	spellTextsPT map[string]*SpellTextPT
	itemTextsPT  map[string][]string
	// ruleTextsPT are the Portuguese texts of the SRD's features, traits, background
	// features and feats (rule_texts_pt.go), by key.
	ruleTextsPT map[string][]string

	// choices is effects/choices.json (choicedata.go), shared by a table layer.
	choices *choiceData

	compiler *formula.Compiler
	catalog  Catalog
	// spellEntries are the Catalog's spells by key.
	spellEntries map[string]SpellEntry
	// spellDetails are the structured details of each spell, by key.
	spellDetails map[string]*SpellDetails
	// optionParents maps an option that 5e-database lists only in its
	// parent's options (no "parent" field) to that parent.
	optionParents map[string]string
	// optionSet says, for each table option (a key "option:<set>:<slug>@mesa"), the
	// set it joins, and optionPrereq what it asks of the character (overlay_option.go).
	optionSet map[string]string
	// classFeatureOf says, for each table class feature, the class and the level it is at.
	classFeatureOf map[string]ClassFeatureEntry
	optionPrereq   map[string]TableOptionPrerequisite
	// creatureOf says, for each table creature, the SRD creature it is made from (overlay_creature.go).
	creatureOf map[string]CreatureTableEntry

	// The table's layer (overlay.go); every one of these is empty in the
	// SRD content.
	//
	// table says this content has a table layer, and tableRevision is its
	// revision. listFrom maps a table class that reuses another class's spell
	// list to that class. offeredBy maps an SRD option a table feature offers
	// to the table features that do. archived are the table keys that no
	// longer show as a new choice. spellTargets are the targets of the table's
	// spells, raceChoice the ability bonuses a table race lets the player
	// place, and bgEquipment a table background's equipment text.
	// programs memoizes compiled formulas by text while a table layer is added.
	programs      map[programKey]*formula.Program
	table         bool
	tableRevision int
	listFrom      map[string]string
	offeredBy     map[string][]string
	archived      map[string]bool
	// off are the keys (the SRD's and the table's) the master switched off for the
	// players ("Opções para os jogadores", RN-23); empty without a table layer.
	off          map[string]bool
	spellTargets map[string]SpellTarget
	// spellZones are the areas the table's spells keep on the map while they last.
	spellZones map[string]SpellZoneRule
	// srdTargets are the hand-written targets of some SRD spells
	// (effects/spell_targets.json), by spell key.
	srdTargets map[string]SpellTarget
	// resistances are the damage resistances of features and traits
	// (effects/damage_resistances.json), by key.
	resistances map[string]Resistance
	raceChoice  map[string][]int
	// raceSets are the alternative sets of ability bonuses a table race lets the player place
	// and raceSizes the sizes it lets the player pick; absent for the races that have none.
	raceSets    map[string][][]int
	raceSizes   map[string][]string
	bgEquipment map[string]string
	// entryRevision is the revision of each table entry at its last change.
	entryRevision  map[string]int
	entryChangedAt map[string]time.Time
}

// classCasting is how a class (or a third-caster subclass) casts: its
// spellcasting effect, the class level it starts at, and the class whose
// spell list it reads (list). sub is set for a third caster's subclass.
type classCasting struct {
	effect *Effect
	level  int
	list   string
	sub    string
}

// abilityIndex maps each ability to its position on the sheet.
var abilityIndex = map[Ability]int{STR: 0, DEX: 1, CON: 2, INT: 3, WIS: 4, CHA: 5}

// effectsRevision is effects/revision.json: the n of "fx.<n>" and the hash
// of every other file in effects/ when n was set.
type effectsRevision struct {
	Revision int    `json:"revision"`
	SHA256   string `json:"sha256"`
}

func loadSRD() (*Content, error) {
	c, err := load(srd51.Files)
	if err != nil {
		return nil, fmt.Errorf("rules: loading the SRD 5.1 content: %w", err)
	}
	return &Content{c: c}, nil
}

// load reads a snapshot (data/*.json) and its effects (effects/*.json) from
// fsys, indexes them and compiles every formula.
func load(fsys fs.FS) (*content, error) {
	c := newContent()
	if err := c.loadSnapshot(fsys); err != nil {
		return nil, err
	}
	if err := c.loadRevision(fsys); err != nil {
		return nil, err
	}
	if err := c.loadNamesPT(fsys); err != nil {
		return nil, err
	}
	c.compiler = formula.NewCompiler(c.classIndexes())
	if err := c.loadEffectFiles(fsys); err != nil {
		return nil, err
	}
	c.multiclassTable = c.findMulticlassTable()
	c.buildCatalog(nil)
	c.buildCreatures()
	if err := c.buildPlans(); err != nil {
		return nil, err
	}
	c.buildMagicItems()
	if err := c.loadTreasure(fsys); err != nil {
		return nil, err
	}
	return c, nil
}

// newContent is a content with every map made.
func newContent() *content {
	return &content{
		abilities:      map[Ability]srd51.AbilityScore{},
		skills:         map[string]*srd51.Skill{},
		races:          map[string]*srd51.Race{},
		subraces:       map[string]*srd51.Subrace{},
		traits:         map[string]*srd51.Trait{},
		classes:        map[string]*srd51.Class{},
		subclasses:     map[string]*srd51.Subclass{},
		features:       map[string]*srd51.Feature{},
		backgrounds:    map[string]*srd51.Background{},
		feats:          map[string]*srd51.Feat{},
		proficiencies:  map[string]*srd51.Proficiency{},
		equipment:      map[string]*srd51.Equipment{},
		spells:         map[string]*srd51.Spell{},
		monsters:       map[string]*srd51.Monster{},
		magicItems:     map[string]*srd51.MagicItem{},
		languages:      map[string]*srd51.Language{},
		named:          map[string]*srd51.Named{},
		classLevels:    map[string][]*srd51.Level{},
		subclassLevels: map[string]map[int]*srd51.Level{},
		effects:        map[string][]*Effect{},
		casting:        map[string]classCasting{},
		subCasting:     map[string]classCasting{},
		namesPT:        map[string]string{},
		namesEN:        map[string]string{},
		optionParents:  map[string]string{},
	}
}

// contentStep is one step of load that reads files.
type contentStep func(fsys fs.FS) error

// runContentSteps runs the steps in order and stops at the first error.
func runContentSteps(fsys fs.FS, steps []contentStep) error {
	for _, step := range steps {
		if err := step(fsys); err != nil {
			return err
		}
	}
	return nil
}

// noFiles makes a step of a check that reads no file.
func noFiles(check func() error) contentStep {
	return func(fs.FS) error { return check() }
}

// loadSnapshot reads the manifest and the data files, checks them, reads the
// translations and the level rows, and applies the corrections.
func (c *content) loadSnapshot(fsys fs.FS) error {
	return runContentSteps(fsys, []contentStep{
		c.loadManifest, c.loadData, noFiles(c.checkMonsters), noFiles(c.checkMagicItems),
		c.loadTextsPT, c.loadRuleTextsPT, c.indexLevels, c.applyCorrections,
	})
}

// loadManifest reads data/manifest.json, which must name its snapshot.
func (c *content) loadManifest(fsys fs.FS) error {
	if err := readJSON(fsys, "data/manifest.json", &c.manifest); err != nil {
		return err
	}
	if c.manifest.SnapshotVersion == "" {
		return fmt.Errorf("data/manifest.json has no snapshot_version")
	}
	return nil
}

// loadRevision reads effects/revision.json into the content's version.
func (c *content) loadRevision(fsys fs.FS) error {
	var rev effectsRevision
	if err := readJSON(fsys, "effects/revision.json", &rev); err != nil {
		return err
	}
	if rev.Revision < 1 {
		return fmt.Errorf("effects/revision.json: revision must be at least 1")
	}
	c.version = c.manifest.SnapshotVersion + "+fx." + strconv.Itoa(rev.Revision)
	return nil
}

// freeNamePrefixes are the key prefixes names_pt.json may name that are not
// keys of the data: the effects' own names, checked by their loaders.
var freeNamePrefixes = []string{"sense:", "resource:", "trap:", "light:", "effect:", "attunement:", "creature-type:", "terrain:"}

// loadNamesPT reads effects/names_pt.json, refusing a key the content does
// not know.
func (c *content) loadNamesPT(fsys fs.FS) error {
	var names struct {
		Names map[string]string `json:"names"`
	}
	if err := readJSON(fsys, "effects/names_pt.json", &names); err != nil {
		return err
	}
	for k, v := range names.Names {
		if !c.exists(k) && !hasFreeNamePrefix(k) && !c.isActionName(k) {
			return fmt.Errorf("effects/names_pt.json: unknown key %q", k)
		}
		c.namesPT[k] = v
	}
	return nil
}

// hasFreeNamePrefix says whether key starts with one of freeNamePrefixes.
func hasFreeNamePrefix(key string) bool {
	return slices.ContainsFunc(freeNamePrefixes, func(p string) bool { return strings.HasPrefix(key, p) })
}

// classIndexes are the classes' indexes ("wizard"), sorted: what a formula's
// classLevel may name.
func (c *content) classIndexes() []string {
	classIndexes := make([]string, 0, len(c.classes))
	for k := range c.classes {
		classIndexes = append(classIndexes, strings.TrimPrefix(k, "class:"))
	}
	slices.Sort(classIndexes)
	return classIndexes
}

// loadEffectFiles reads and checks every effects file, then indexes how
// each class casts.
func (c *content) loadEffectFiles(fsys fs.FS) error {
	return runContentSteps(fsys, []contentStep{
		c.loadEffects, c.loadChoiceData, c.loadStandardActions, c.loadAdvancement, c.loadSpellEffects,
		c.loadSpellTargets, c.loadDamageResistances, c.loadTraps, c.loadLights, c.loadCombatEffects,
		c.loadEncounterBudget, c.loadMagicItemEffects, noFiles(c.indexCasting),
	})
}

func readJSON(fsys fs.FS, name string, v any) error {
	b, err := fs.ReadFile(fsys, name)
	if err != nil {
		return err
	}
	dec := json.NewDecoder(bytes.NewReader(b))
	dec.DisallowUnknownFields()
	if err := dec.Decode(v); err != nil {
		return fmt.Errorf("%s: %w", name, err)
	}
	return nil
}

// indexStep is the step that reads a data file into a map by key, refusing
// duplicate keys, and records each row's English name; keyName gives a row's key
// and name.
func indexStep[T any](c *content, name string, into map[string]*T, keyName func(*T) (key, name string)) contentStep {
	return func(fsys fs.FS) error {
		var rows []T
		if err := readJSON(fsys, "data/"+name, &rows); err != nil {
			return err
		}
		for i := range rows {
			r := &rows[i]
			k, en := keyName(r)
			if _, dup := into[k]; dup {
				return fmt.Errorf("data/%s: duplicate key %q", name, k)
			}
			into[k] = r
			c.namesEN[k] = en
		}
		return nil
	}
}

// loadData reads the data files, checks the shapes the code relies on and
// indexes the options' parents.
func (c *content) loadData(fsys fs.FS) error {
	if err := c.loadAbilities(fsys); err != nil {
		return err
	}
	if err := c.indexDataFiles(fsys); err != nil {
		return err
	}
	for _, b := range c.backgrounds {
		c.namesEN[b.Feature.Key] = b.Feature.Name
	}
	if err := c.checkDataShapes(); err != nil {
		return err
	}
	for k := range c.skills {
		c.skillOrder = append(c.skillOrder, k)
	}
	slices.Sort(c.skillOrder)
	c.indexOptionParents()
	return nil
}

// loadAbilities reads data/abilities.json: the six abilities, no other.
func (c *content) loadAbilities(fsys fs.FS) error {
	var abilities []srd51.AbilityScore
	if err := readJSON(fsys, "data/abilities.json", &abilities); err != nil {
		return err
	}
	for _, a := range abilities {
		if _, ok := abilityIndex[Ability(a.Key)]; !ok {
			return fmt.Errorf("data/abilities.json: unknown ability %q", a.Key)
		}
		c.abilities[Ability(a.Key)] = a
		c.namesEN[a.Key] = a.FullName
	}
	if len(c.abilities) != len(abilityIndex) {
		return fmt.Errorf("data/abilities.json: want %d abilities, got %d", len(abilityIndex), len(c.abilities))
	}
	return nil
}

// indexDataFiles reads every keyed data file, in a fixed order: the first
// one that fails is the error.
func (c *content) indexDataFiles(fsys fs.FS) error {
	steps := []contentStep{
		indexStep(c, "skills.json", c.skills, func(r *srd51.Skill) (string, string) { return r.Key, r.Name }),
		indexStep(c, "races.json", c.races, func(r *srd51.Race) (string, string) { return r.Key, r.Name }),
		indexStep(c, "subraces.json", c.subraces, func(r *srd51.Subrace) (string, string) { return r.Key, r.Name }),
		indexStep(c, "traits.json", c.traits, func(r *srd51.Trait) (string, string) { return r.Key, r.Name }),
		indexStep(c, "classes.json", c.classes, func(r *srd51.Class) (string, string) { return r.Key, r.Name }),
		indexStep(c, "subclasses.json", c.subclasses, func(r *srd51.Subclass) (string, string) { return r.Key, r.Name }),
		indexStep(c, "features.json", c.features, func(r *srd51.Feature) (string, string) { return r.Key, r.Name }),
		indexStep(c, "backgrounds.json", c.backgrounds, func(r *srd51.Background) (string, string) { return r.Key, r.Name }),
		indexStep(c, "feats.json", c.feats, func(r *srd51.Feat) (string, string) { return r.Key, r.Name }),
		indexStep(c, "proficiencies.json", c.proficiencies, func(r *srd51.Proficiency) (string, string) { return r.Key, r.Name }),
		indexStep(c, "equipment.json", c.equipment, func(r *srd51.Equipment) (string, string) { return r.Key, r.Name }),
		indexStep(c, "spells.json", c.spells, func(r *srd51.Spell) (string, string) { return r.Key, r.Name }),
		indexStep(c, "monsters.json", c.monsters, func(r *srd51.Monster) (string, string) { return r.Key, r.Name }),
		indexStep(c, "magic-items.json", c.magicItems, func(r *srd51.MagicItem) (string, string) { return r.Key, r.Name }),
		indexStep(c, "languages.json", c.languages, func(r *srd51.Language) (string, string) { return r.Key, r.Name }),
	}
	for _, name := range []string{"damage-types.json", "magic-schools.json", "weapon-properties.json", "conditions.json"} {
		steps = append(steps, indexStep(c, name, c.named, func(r *srd51.Named) (string, string) { return r.Key, r.Name }))
	}
	return runContentSteps(fsys, steps)
}

// spellAreaStepFt is the step of a spell's area size: whole squares of 5 ft.
const spellAreaStepFt = 5

// checkDataShapes refuses a feat whose prerequisites the code cannot read,
// and a spell whose area is not a shape of whole squares.
func (c *content) checkDataShapes() error {
	for _, k := range sortedKeys(c.feats) {
		if field, msg := c.checkFeatShape(c.feats[k]); field != "" {
			return fmt.Errorf("data/feats.json: %s: %s: %s", k, field, msg)
		}
	}
	for _, k := range sortedKeys(c.spells) {
		if s := c.spells[k]; !validSpellArea(s) {
			return fmt.Errorf("data/spells.json: %s has the area %q of %d ft", k, s.AreaType, s.AreaSizeFt)
		}
	}
	return nil
}

// validSpellArea says whether a spell has no area, or a shape with a size
// of whole squares.
func validSpellArea(s *srd51.Spell) bool {
	if (s.AreaType == "") != (s.AreaSizeFt == 0) {
		return false
	}
	return s.AreaType == "" || slices.Contains(targetShapesOfData, s.AreaType) && s.AreaSizeFt >= spellAreaStepFt && s.AreaSizeFt%spellAreaStepFt == 0
}

// targetShapesOfData are the area shapes data/spells.json may name.
var targetShapesOfData = []string{ShapeCone, ShapeCube, ShapeCylinder, ShapeLine, ShapeSphere}

// indexOptionParents gives each option feature its parent: the first one in
// key order that offers it, which is the lowest level one (a feature listed
// by metamagic-1, 2 and 3 belongs to metamagic-1, the one a level 3 sorcerer
// has).
func (c *content) indexOptionParents() {
	for _, k := range sortedKeys(c.features) {
		for _, o := range c.features[k].Options {
			if _, set := c.optionParents[o]; set {
				continue
			}
			if f, ok := c.features[o]; ok && f.Parent == "" {
				c.optionParents[o] = k
			}
		}
	}
}

// indexLevels reads data/levels.json into the class and subclass rows, and
// refuses a class without all its levels.
func (c *content) indexLevels(fsys fs.FS) error {
	var rows []srd51.Level
	if err := readJSON(fsys, "data/levels.json", &rows); err != nil {
		return err
	}
	for i := range rows {
		if err := c.placeLevelRow(&rows[i]); err != nil {
			return err
		}
	}
	for key := range c.classes {
		rows := c.classLevels[key]
		if len(rows) != MaxLevel || slices.Contains(rows, nil) {
			return fmt.Errorf("data/levels.json: class %s does not have all %d levels", key, MaxLevel)
		}
	}
	return nil
}

// placeLevelRow puts a row in its subclass's map or its class's table.
func (c *content) placeLevelRow(r *srd51.Level) error {
	if r.Level < 1 || r.Level > MaxLevel {
		return fmt.Errorf("data/levels.json: %s level %d out of range", r.Class, r.Level)
	}
	if r.Subclass != "" {
		if c.subclassLevels[r.Subclass] == nil {
			c.subclassLevels[r.Subclass] = map[int]*srd51.Level{}
		}
		c.subclassLevels[r.Subclass][r.Level] = r
		return nil
	}
	if c.classLevels[r.Class] == nil {
		c.classLevels[r.Class] = make([]*srd51.Level, MaxLevel)
	}
	c.classLevels[r.Class][r.Level-1] = r
	return nil
}

// loadEffects reads every effects file except names_pt.json, revision.json,
// standard_actions.json, advancement.json, spells.json, traps.json, lights.json, encounter_budget.json, consumables.json, magic_item_values.json, treasure.json, spells_pt.json, magic_items_pt.json, features_pt.json, traits_pt.json, backgrounds_pt.json and feats_pt.json (tables and texts, not effects), checks
// and compiles each effect.
func (c *content) loadEffects(fsys fs.FS) error {
	files, err := fs.Glob(fsys, "effects/*.json")
	if err != nil {
		return err
	}
	for _, name := range files {
		if notEffectFiles[path.Base(name)] {
			continue
		}
		if err := c.loadEffectFile(fsys, name); err != nil {
			return err
		}
	}
	return nil
}

// notEffectFiles are the files of effects/ that hold tables and texts, not
// effects: loadEffects leaves them to their own loaders.
var notEffectFiles = map[string]bool{
	"names_pt.json": true, "revision.json": true, "standard_actions.json": true, "advancement.json": true,
	"spells.json": true, "spell_targets.json": true, "corrections.json": true, "damage_resistances.json": true,
	"traps.json": true, "lights.json": true, "combat_effects.json": true, "consumables.json": true,
	"encounter_budget.json": true, "magic_item_values.json": true, "treasure.json": true, "choices.json": true,
	"spells_pt.json": true, "magic_items_pt.json": true, "features_pt.json": true, "traits_pt.json": true,
	"backgrounds_pt.json": true, "feats_pt.json": true,
}

// loadEffectFile reads one effects file, and checks and compiles its effects
// in key order.
func (c *content) loadEffectFile(fsys fs.FS, name string) error {
	var f struct {
		Effects map[string][]*Effect `json:"effects"`
	}
	if err := readJSON(fsys, name, &f); err != nil {
		return err
	}
	for _, key := range sortedKeys(f.Effects) {
		if _, dup := c.effects[key]; dup {
			return fmt.Errorf("%s: effects for %q are also in another file", name, key)
		}
		if !c.effectOwnerExists(key) {
			return fmt.Errorf("%s: effects for unknown key %q", name, key)
		}
		for _, e := range f.Effects[key] {
			if err := c.compileEffect(key, e); err != nil {
				return fmt.Errorf("%s: %w", name, err)
			}
		}
		c.effects[key] = f.Effects[key]
	}
	return nil
}

// effectOwnerExists says whether effects may hang on key: a feature, a
// trait, a background or its feature, a race, subrace, class or subclass.
func (c *content) effectOwnerExists(key string) bool {
	switch {
	case strings.HasPrefix(key, "background-feature:"):
		for _, b := range c.backgrounds {
			if b.Feature.Key == key {
				return true
			}
		}
		return false
	case strings.HasPrefix(key, featureKeyPrefix), strings.HasPrefix(key, "trait:"),
		strings.HasPrefix(key, "background:"), strings.HasPrefix(key, "race:"), strings.HasPrefix(key, "feat:"),
		strings.HasPrefix(key, "subrace:"), strings.HasPrefix(key, "class:"),
		strings.HasPrefix(key, "subclass:"), strings.HasPrefix(key, OptionKeyPrefix), strings.HasPrefix(key, ClassFeatureKeyPrefix):
		return c.exists(key)
	}
	return false
}

// indexCasting finds, for each class, the feature with the spellcasting
// effect and the class level it comes at.
func (c *content) indexCasting() error {
	for classKey := range c.classLevels {
		if err := c.indexClassCasting(classKey); err != nil {
			return err
		}
	}
	return nil
}

// indexClassCasting is indexCasting for one class.
func (c *content) indexClassCasting(classKey string) error {
	for _, row := range c.classLevels[classKey] {
		for _, fk := range row.Features {
			for _, e := range c.effects[fk] {
				if e.Type != "spellcasting" {
					continue
				}
				if _, dup := c.casting[classKey]; dup {
					return fmt.Errorf("class %s has two spellcasting effects", classKey)
				}
				c.casting[classKey] = classCasting{effect: e, level: row.Level, list: classKey}
			}
		}
	}
	return nil
}

// findMulticlassTable picks the class whose table the multiclass spellcaster
// rule reads: the first full caster of the SRD, never a table class (its
// table is its own).
func (c *content) findMulticlassTable() string {
	for _, key := range sortedKeys(c.casting) {
		if c.casting[key].effect.Progression == "full" && !isTableKey(key) {
			return key
		}
	}
	return ""
}

// castingFor says how a class casts, with the subclass the character chose
// (a third caster casts through its subclass).
func (c *content) castingFor(classKey string, sub *srd51.Subclass) (classCasting, bool) {
	if cast, ok := c.casting[classKey]; ok {
		return cast, true
	}
	if sub != nil {
		cast, ok := c.subCasting[sub.Key]
		return cast, ok
	}
	return classCasting{}, false
}

// castingRow is the table row a caster reads at a class level: the class's,
// or the subclass's for a third caster. It is nil when the subclass table has
// no row there.
func (c *content) castingRow(classKey string, level int, cast classCasting) *srd51.Level {
	if cast.sub != "" {
		return c.subclassLevels[cast.sub][level]
	}
	return c.classLevels[classKey][level-1]
}

// onList says whether spell s is on the spell list of class: the spell names
// the class, or the class reuses (listFrom) a list that names it. It is the one
// membership test Derive, the level-up and the catalog share.
func (c *content) onList(s *srd51.Spell, class string) bool {
	if slices.Contains(s.Classes, class) {
		return true
	}
	from, ok := c.listFrom[class]
	return ok && slices.Contains(s.Classes, from)
}

// exists says whether key is any known content key.
func (c *content) exists(key string) bool {
	if _, ok := abilityIndex[Ability(key)]; ok {
		return true
	}
	_, ok := c.namesEN[key]
	return ok
}

// namePT is the Portuguese name of key, or its English name, or "".
func (c *content) namePT(key string) string {
	if n, ok := c.namesPT[key]; ok {
		return n
	}
	return c.namesEN[key]
}

// namesWithPrefix lists the keys of the Portuguese names that start with prefix, sorted, so a
// loader that refuses a stray name always names the same one (a map's order is random).
func (c *content) namesWithPrefix(prefix string) []string {
	var keys []string
	for k := range c.namesPT {
		if strings.HasPrefix(k, prefix) {
			keys = append(keys, k)
		}
	}
	slices.Sort(keys)
	return keys
}

// ability returns the Ability for an index such as "int", and whether it
// is one.
func ability(index string) (Ability, bool) {
	a := Ability(index)
	_, ok := abilityIndex[a]
	return a, ok
}

func abilityMap(m map[string]int) map[Ability]int {
	out := make(map[Ability]int, len(m))
	for k, v := range m {
		if a, ok := ability(k); ok {
			out[a] = v
		}
	}
	return out
}

// traitSkills are the skills the traits give, as skill keys: the trait's
// "proficiency:skill-<name>" proficiencies.
func (c *content) traitSkills(traits []string) []string {
	var out []string
	for _, t := range traits {
		tr, ok := c.traits[t]
		if !ok {
			continue
		}
		for _, p := range tr.Proficiencies {
			if skill, ok := strings.CutPrefix(p, "proficiency:skill-"); ok {
				out = append(out, "skill:"+skill)
			}
		}
	}
	return out
}

// buildCatalog fills catalog, spellEntries and spellDetails. reuse, when not nil,
// are the details of another content with the same SRD spells (the base of a
// With): the ones that did not change are shared instead of parsed again.
// Every list but the abilities is sorted by Portuguese name, as the editor
// shows it.
func (c *content) buildCatalog(reuse map[string]*SpellDetails) {
	cat := Catalog{ContentVersion: c.version, Attribution: srd51.Attribution}
	cat.Races = c.raceEntries()
	cat.Subraces = c.subraceEntries()
	cat.Classes = c.classEntries()
	cat.Subclasses = c.subclassEntries()
	cat.Backgrounds = c.backgroundEntries()
	cat.Skills = c.skillEntries()
	cat.Armor, cat.Weapons = c.gearEntries()
	cat.Spells = c.spellCatalog(reuse)
	for _, a := range AllAbilities() {
		cat.Abilities = append(cat.Abilities, AbilityEntry{
			Ability: a, Name: c.abilities[a].FullName, NamePT: c.namePT(string(a)), AbbreviationPT: abbreviationPT[a],
		})
	}
	cat.ChallengeRatings = slices.Clone(c.ratings)
	cat.LevelXP = slices.Clone(c.levelXP)
	cat.Languages, cat.Proficiencies, cat.DamageTypes = c.namedEntries()
	c.catalog = cat
}

func (c *content) raceEntries() []RaceEntry {
	var out []RaceEntry
	for _, k := range sortedKeys(c.races) {
		r := c.races[k]
		out = append(out, RaceEntry{
			Key: k, Name: r.Name, NamePT: c.namePT(k), SpeedFt: r.SpeedFt, Size: r.Size,
			AbilityBonuses: abilityMap(r.AbilityBonuses), Subraces: r.Subraces,
			ChoiceBonuses: c.raceChoice[k], ChoiceBonusSets: c.raceSets[k], Sizes: slices.Clone(c.raceSizes[k]), SkillProficiencies: c.traitSkills(r.Traits), Archived: c.archived[k], Off: c.off[k],
		})
	}
	sortPT(out, func(e RaceEntry) string { return e.NamePT })
	return out
}

func (c *content) subraceEntries() []SubraceEntry {
	var out []SubraceEntry
	for _, k := range sortedKeys(c.subraces) {
		s := c.subraces[k]
		out = append(out, SubraceEntry{
			Key: k, Name: s.Name, NamePT: c.namePT(k), Race: s.Race, AbilityBonuses: abilityMap(s.AbilityBonuses),
			SkillProficiencies: c.traitSkills(s.Traits), Archived: c.archived[k], Off: c.off[k],
		})
	}
	sortPT(out, func(e SubraceEntry) string { return e.NamePT })
	return out
}

func (c *content) classEntries() []ClassEntry {
	var out []ClassEntry
	for _, k := range sortedKeys(c.classes) {
		out = append(out, c.classEntry(k, c.classes[k]))
	}
	sortPT(out, func(e ClassEntry) string { return e.NamePT })
	return out
}

// classEntry is a class as the catalog lists it, with its casting when it casts.
func (c *content) classEntry(k string, cl *srd51.Class) ClassEntry {
	e := ClassEntry{
		Key: k, Name: cl.Name, NamePT: c.namePT(k), HitDie: cl.HitDie,
		SkillChoices: cl.SkillChoices.Choose, SkillOptions: cl.SkillChoices.From,
		SubclassLevel: cl.SubclassLevel, Subclasses: cl.Subclasses, Archived: c.archived[k], Off: c.off[k],
		SpellListFrom: c.listFrom[k],
	}
	for _, s := range cl.SavingThrows {
		if a, ok := ability(s); ok {
			e.SavingThrows = append(e.SavingThrows, a)
		}
	}
	cast, ok := c.casting[k]
	if !ok {
		return e
	}
	e.SpellcastingAbility = Ability(cast.effect.Ability)
	e.PreparesSpells = cast.effect.Prepares
	e.SpellPreparation = preparation(cast.effect)
	e.SpellcastingLevel = cast.level
	for _, row := range c.classLevels[k] {
		highest := 0
		if row != nil && row.Spellcasting != nil {
			highest = MaxSpellLevelFromSlots(row.Spellcasting.Slots)
		}
		e.MaxSpellLevelByLevel = append(e.MaxSpellLevelByLevel, highest)
	}
	return e
}

func (c *content) subclassEntries() []SubclassEntry {
	var out []SubclassEntry
	for _, k := range sortedKeys(c.subclasses) {
		out = append(out, c.subclassEntry(k, c.subclasses[k]))
	}
	sortPT(out, func(e SubclassEntry) string { return e.NamePT })
	return out
}

// subclassEntry is a subclass as the catalog lists it, with the spells it always
// prepares and its casting when it is a third caster.
func (c *content) subclassEntry(k string, s *srd51.Subclass) SubclassEntry {
	e := SubclassEntry{Key: k, Name: s.Name, NamePT: c.namePT(k), Class: s.Class, Archived: c.archived[k], Off: c.off[k]}
	for _, ss := range s.Spells {
		// A spell that needs a feature's choice is not always prepared for everyone.
		if len(ss.WithFeatures) == 0 && !s.ExpandedList {
			e.AlwaysPrepared = append(e.AlwaysPrepared, SubclassSpellRef{Spell: ss.Spell, ClassLevel: ss.ClassLevel})
		}
	}
	cast, ok := c.subCasting[k]
	if !ok {
		return e
	}
	sc := &SubclassCasting{
		Kind: cast.effect.Progression, Ability: Ability(cast.effect.Ability), Preparation: preparation(cast.effect),
		SpellList: cast.list, StartLevel: cast.level,
	}
	for lvl := 1; lvl <= MaxLevel; lvl++ {
		highest := 0
		if row := c.subclassLevels[k][lvl]; row != nil && row.Spellcasting != nil {
			highest = MaxSpellLevelFromSlots(row.Spellcasting.Slots)
		}
		sc.MaxSpellLevelByLevel = append(sc.MaxSpellLevelByLevel, highest)
	}
	e.Casting = sc
	return e
}

func (c *content) backgroundEntries() []BackgroundEntry {
	var out []BackgroundEntry
	for _, k := range sortedKeys(c.backgrounds) {
		b := c.backgrounds[k]
		out = append(out, BackgroundEntry{Key: k, Name: b.Name, NamePT: c.namePT(k), SkillProficiencies: b.Skills, EquipmentPT: c.bgEquipment[k], Archived: c.archived[k], Off: c.off[k]})
	}
	sortPT(out, func(e BackgroundEntry) string { return e.NamePT })
	return out
}

// skillEntries are the skills in the order of the SRD's list, sorted by name.
func (c *content) skillEntries() []SkillEntry {
	var out []SkillEntry
	for _, k := range c.skillOrder {
		s := c.skills[k]
		out = append(out, SkillEntry{Key: k, Name: s.Name, NamePT: c.namePT(k), Ability: Ability(s.Ability)})
	}
	sortPT(out, func(e SkillEntry) string { return e.NamePT })
	return out
}

// gearEntries are the body armors (a shield is not one) and the weapons.
func (c *content) gearEntries() ([]ArmorEntry, []WeaponEntry) {
	var armor []ArmorEntry
	var weapons []WeaponEntry
	for _, k := range sortedKeys(c.equipment) {
		e := c.equipment[k]
		switch {
		case e.Armor != nil && e.Armor.Category != "shield":
			armor = append(armor, c.armorEntry(k, e))
		case e.Weapon != nil:
			weapons = append(weapons, c.weaponEntry(k, e))
		}
	}
	sortPT(armor, func(e ArmorEntry) string { return e.NamePT })
	sortPT(weapons, func(e WeaponEntry) string { return e.NamePT })
	return armor, weapons
}

func (c *content) armorEntry(k string, e *srd51.Equipment) ArmorEntry {
	a := e.Armor
	return ArmorEntry{
		Key: k, Name: e.Name, NamePT: c.namePT(k), Category: a.Category, BaseAC: a.BaseAC,
		DexBonus: a.DexBonus, MaxDexBonus: a.MaxDexBonus, StrMinimum: a.StrMinimum,
		StealthDisadvantage: a.StealthDisadvantage,
	}
}

// weaponEntry is a weapon with the range it is used at: the thrown range of a
// melee weapon that can be thrown, none for one that cannot.
func (c *content) weaponEntry(k string, e *srd51.Equipment) WeaponEntry {
	w := e.Weapon
	normal, long := w.NormalRangeFt, w.LongRangeFt
	if w.ThrowNormalFt > 0 {
		normal, long = w.ThrowNormalFt, w.ThrowLongFt
	}
	if w.Range == "melee" && w.ThrowNormalFt == 0 {
		normal, long = 0, 0
	}
	return WeaponEntry{
		Key: k, Name: e.Name, NamePT: c.namePT(k), Category: w.Category, Range: w.Range,
		Damage: w.Damage, DamageType: w.DamageType, TwoHandedDamage: w.TwoHandedDamage,
		Properties: w.Properties, NormalRangeFt: normal, LongRangeFt: long,
	}
}

// spellCatalog lists the spells sorted by name and fills spellEntries and
// spellDetails, sharing the details of reuse that did not change.
func (c *content) spellCatalog(reuse map[string]*SpellDetails) []SpellEntry {
	var out []SpellEntry
	c.spellEntries = make(map[string]SpellEntry, len(c.spells))
	c.spellDetails = make(map[string]*SpellDetails, len(c.spells))
	for _, k := range sortedKeys(c.spells) {
		s := c.spells[k]
		e := SpellEntry{
			Key: k, Name: s.Name, NamePT: c.namePT(k), Level: s.Level,
			School: s.School, SchoolNamePT: c.namePT(s.School),
			Classes: c.spellClasses(s), Ritual: s.Ritual, Concentration: s.Concentration,
			CastingTime: parseCastingTime(s.CastingTime), Archived: c.archived[k], Off: c.off[k],
		}
		out = append(out, e)
		c.spellEntries[k] = e
		if d, ok := reuse[k]; ok && d.Spell.Archived == e.Archived && d.Spell.Off == e.Off && slices.Equal(d.Spell.Classes, e.Classes) {
			c.spellDetails[k] = d
			continue
		}
		d := c.buildSpellDetails(s, e)
		// The table's spell says whom it reaches itself; an SRD spell is worked out.
		if t, ok := c.spellTargets[k]; ok {
			d.Target = t
		} else {
			d.Target = c.srdTarget(s)
		}
		c.spellDetails[k] = d
	}
	sortPT(out, func(e SpellEntry) string { return e.NamePT })
	return out
}

// namedEntries are the languages, the proficiencies and the damage types, sorted
// by Portuguese name.
func (c *content) namedEntries() (languages, proficiencies, damageTypes []NamedEntry) {
	for _, k := range sortedKeys(c.languages) {
		languages = append(languages, NamedEntry{Key: k, NamePT: c.namePT(k), Kind: "language"})
	}
	for _, k := range sortedKeys(c.proficiencies) {
		proficiencies = append(proficiencies, NamedEntry{Key: k, NamePT: c.proficiencyNamePT(k), Kind: c.proficiencies[k].Kind})
	}
	for _, k := range sortedKeys(c.named) {
		if strings.HasPrefix(k, "damage-type:") {
			damageTypes = append(damageTypes, NamedEntry{Key: k, NamePT: c.namePT(k)})
		}
	}
	for _, list := range [][]NamedEntry{languages, proficiencies, damageTypes} {
		sortPT(list, func(e NamedEntry) string { return e.NamePT })
	}
	return languages, proficiencies, damageTypes
}

// spellClasses are the classes whose list a spell is on: the spell's own, and
// the table classes that reuse a list it is on. Without a table layer it is the
// SRD's slice, shared.
func (c *content) spellClasses(s *srd51.Spell) []string {
	if len(c.listFrom) == 0 {
		return s.Classes
	}
	out := s.Classes
	cloned := false
	for _, class := range sortedKeys(c.listFrom) {
		if slices.Contains(s.Classes, c.listFrom[class]) && !slices.Contains(out, class) {
			if !cloned {
				out, cloned = slices.Clone(out), true
			}
			out = append(out, class)
		}
	}
	return out
}

// isTableKey says whether a content key belongs to the table's layer: it ends
// in "@mesa" (ADR-0018).
func isTableKey(key string) bool { return strings.HasSuffix(key, tableSuffix) }

func sortPT[T any](s []T, name func(T) string) {
	slices.SortStableFunc(s, func(a, b T) int { return comparePT(name(a), name(b)) })
}

func sortedKeys[T any](m map[string]T) []string {
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	slices.Sort(keys)
	return keys
}

// creatureCorrectionFields are the creature fields effects/corrections.json may
// correct. The set is closed.
var creatureCorrectionFields = []string{
	"attacks_per_action", "armor_class", "hit_points", "hit_points_roll",
	"speed_walk", "speed_fly", "speed_swim", "speed_climb", "speed_burrow",
	"darkvision", "blindsight", "tremorsense", "truesight", "passive_perception",
	"skills", "damage_immunities", "condition_immunities", fieldActionSaveSuccess, // the last is handled before the field switch: it corrects an action
}

// correctionFields are the class table columns effects/corrections.json may
// correct. The set is closed.
var correctionFields = []string{"invocations_known"}

// multiclassCorrectionFields are the multiclass fields effects/corrections.json
// may add. The set is closed.
var multiclassCorrectionFields = []string{"instrument_choices"}

// spellCorrectionSaveSuccess are the outcomes of a saving throw a spell
// correction may name (the values of Spell.SaveSuccess).
var spellCorrectionSaveSuccess = []string{"half", "none", "other"}

// Spell damage choices a spell correction may name (srd51.Spell.DamageChoice).
const (
	// DamageChoiceScale: every listed type is dealt; the dice a higher slot adds
	// go to the type the caster picks (Flame Strike).
	DamageChoiceScale = "scale"
	// DamageChoiceAlternative: only the type the caster picks is dealt (Spirit
	// Guardians: radiant or necrotic).
	DamageChoiceAlternative = "alternative"
)

// minChoiceDamageTypes is how many damage types a spell needs for the caster to
// have one to pick.
const minChoiceDamageTypes = 2

// hitPointsRollPattern is a creature's hit point roll, "6d8+6" or "3d8".
var hitPointsRollPattern = regexp.MustCompile(`^(\d+d\d+)([+-]\d+)?$`)

// spellListCorrection moves spells on or off one class's spell list in
// effects/corrections.json.
type spellListCorrection struct {
	Class  string   `json:"class"`
	Add    []string `json:"add"`
	Remove []string `json:"remove"`
	Source string   `json:"source"`
}

// correctSpellLists writes the spell list corrections over the spells'
// classes. It refuses an unknown class or spell, a spell added that is already on the list or removed that is not,
// a spell named twice, a correction that changes nothing and one without a
// source.
func (c *content) correctSpellLists(name string, corrections []spellListCorrection) error {
	seen := map[string]bool{}
	for _, corr := range corrections {
		if err := c.checkSpellListCorrection(name, corr, seen); err != nil {
			return err
		}
		if err := c.applySpellListCorrection(name, corr); err != nil {
			return err
		}
	}
	return nil
}

// checkSpellListCorrection refuses an unknown class, a correction without a
// source, one of a class corrected before, one that changes nothing, and an
// unknown spell or one named twice.
func (c *content) checkSpellListCorrection(name string, corr spellListCorrection, seen map[string]bool) error {
	if _, ok := c.classes[corr.Class]; !ok {
		return fmt.Errorf("%s: unknown class %q", name, corr.Class)
	}
	if corr.Source == "" {
		return fmt.Errorf("%s: the spell list of %s needs a source", name, corr.Class)
	}
	if seen[corr.Class] {
		return fmt.Errorf("%s: the spell list of %s is corrected twice", name, corr.Class)
	}
	seen[corr.Class] = true
	if len(corr.Add)+len(corr.Remove) == 0 {
		return fmt.Errorf("%s: the spell list of %s corrects nothing", name, corr.Class)
	}
	named := map[string]bool{}
	for _, key := range slices.Concat(corr.Add, corr.Remove) {
		if _, ok := c.spells[key]; !ok {
			return fmt.Errorf("%s: %s: unknown spell %q", name, corr.Class, key)
		}
		if named[key] {
			return fmt.Errorf("%s: %s: %s is named twice", name, corr.Class, key)
		}
		named[key] = true
	}
	return nil
}

// applySpellListCorrection adds and removes the spells, refusing one added
// that is already on the list or removed that is not.
func (c *content) applySpellListCorrection(name string, corr spellListCorrection) error {
	for _, key := range corr.Add {
		s := c.spells[key]
		if slices.Contains(s.Classes, corr.Class) {
			return fmt.Errorf("%s: %s is already on the list of %s", name, key, corr.Class)
		}
		s.Classes = append(slices.Clone(s.Classes), corr.Class)
	}
	for _, key := range corr.Remove {
		s := c.spells[key]
		i := slices.Index(s.Classes, corr.Class)
		if i < 0 {
			return fmt.Errorf("%s: %s is not on the list of %s", name, key, corr.Class)
		}
		s.Classes = slices.Delete(slices.Clone(s.Classes), i, i+1)
	}
	return nil
}

// subclassRowCorrection adds features to the row of a subclass at a class
// level in effects/corrections.json, creating the row when the snapshot has
// none at that level.
type subclassRowCorrection struct {
	Subclass string   `json:"subclass"`
	Level    int      `json:"level"`
	Add      []string `json:"add_features"`
	Source   string   `json:"source"`
}

// correctSubclassFeatures writes the subclass row corrections. It refuses an
// unknown subclass or feature, a feature that belongs to another subclass, a
// level out of range, a feature the row already has and a correction without
// a source.
func (c *content) correctSubclassFeatures(name string, corrections []subclassRowCorrection) error {
	seen := map[string]bool{}
	for _, corr := range corrections {
		sub, err := c.checkSubclassRowCorrection(name, corr, seen)
		if err != nil {
			return err
		}
		if err := c.addSubclassRowFeatures(name, corr, c.subclassRow(sub, corr)); err != nil {
			return err
		}
	}
	return nil
}

// checkSubclassRowCorrection refuses an unknown subclass, a correction
// without features or a source, a level out of range and a row corrected
// before; it returns the subclass.
func (c *content) checkSubclassRowCorrection(name string, corr subclassRowCorrection, seen map[string]bool) (*srd51.Subclass, error) {
	sub, ok := c.subclasses[corr.Subclass]
	if !ok {
		return nil, fmt.Errorf("%s: unknown subclass %q", name, corr.Subclass)
	}
	if corr.Source == "" || len(corr.Add) == 0 {
		return nil, fmt.Errorf("%s: %s level %d needs features and a source", name, corr.Subclass, corr.Level)
	}
	if corr.Level < 1 || corr.Level > MaxLevel {
		return nil, fmt.Errorf("%s: %s: level %d is not 1 to %d", name, corr.Subclass, corr.Level, MaxLevel)
	}
	id := fmt.Sprintf("%s/%d", corr.Subclass, corr.Level)
	if seen[id] {
		return nil, fmt.Errorf("%s: %s level %d is corrected twice", name, corr.Subclass, corr.Level)
	}
	seen[id] = true
	return sub, nil
}

// subclassRow is the subclass's row at the correction's level, made when the
// snapshot has none.
func (c *content) subclassRow(sub *srd51.Subclass, corr subclassRowCorrection) *srd51.Level {
	row := c.subclassLevels[corr.Subclass][corr.Level]
	if row != nil {
		return row
	}
	row = &srd51.Level{Class: sub.Class, Subclass: corr.Subclass, Level: corr.Level}
	if c.subclassLevels[corr.Subclass] == nil {
		c.subclassLevels[corr.Subclass] = map[int]*srd51.Level{}
	}
	c.subclassLevels[corr.Subclass][corr.Level] = row
	return row
}

// addSubclassRowFeatures adds the features to the row, refusing an unknown
// one, one of another subclass and one the row has.
func (c *content) addSubclassRowFeatures(name string, corr subclassRowCorrection, row *srd51.Level) error {
	for _, key := range corr.Add {
		f, ok := c.features[key]
		if !ok {
			return fmt.Errorf("%s: %s: unknown feature %q", name, corr.Subclass, key)
		}
		if f.Subclass != corr.Subclass {
			return fmt.Errorf("%s: %s: %s is not a feature of this subclass", name, corr.Subclass, key)
		}
		if slices.Contains(row.Features, key) {
			return fmt.Errorf("%s: %s level %d already has %s", name, corr.Subclass, corr.Level, key)
		}
		row.Features = append(row.Features, key)
	}
	return nil
}

// The ranges of a creature's corrected numbers.
const (
	maxCreatureFeet     = 500
	maxCreatureStat     = 100
	maxCreatureHP       = 1 << 16
	maxAttacksPerAction = 20
)

// creatureNumber is a field of a stat block that holds one number, with the
// range it may take.
type creatureNumber struct {
	dst    *int
	lo, hi int
}

// creatureNumbers are the fields of m that hold one number.
func creatureNumbers(m *srd51.Monster) map[string]creatureNumber {
	return map[string]creatureNumber{
		"armor_class":        {&m.ArmorClass, 1, maxCreatureStat},
		"hit_points":         {&m.HitPoints, 1, maxCreatureHP},
		"speed_walk":         {&m.Speed.Walk, 0, maxCreatureFeet},
		"speed_fly":          {&m.Speed.Fly, 0, maxCreatureFeet},
		"speed_swim":         {&m.Speed.Swim, 0, maxCreatureFeet},
		"speed_climb":        {&m.Speed.Climb, 0, maxCreatureFeet},
		"speed_burrow":       {&m.Speed.Burrow, 0, maxCreatureFeet},
		"darkvision":         {&m.Darkvision, 0, maxCreatureFeet},
		"blindsight":         {&m.Blindsight, 0, maxCreatureFeet},
		"tremorsense":        {&m.Tremorsense, 0, maxCreatureFeet},
		"truesight":          {&m.Truesight, 0, maxCreatureFeet},
		"passive_perception": {&m.PassivePerception, 1, maxCreatureStat},
	}
}

// correctedNumber reads raw as a number from lo to hi.
func correctedNumber(raw json.RawMessage, lo, hi int) (int, error) {
	var v int
	if err := json.Unmarshal(raw, &v); err != nil || v < lo || v > hi {
		return 0, fmt.Errorf("needs a number of %d to %d", lo, hi)
	}
	return v, nil
}

// correctCreature writes one corrected field of a stat block. raw is the
// field's JSON value: a number, a hit point roll, a skill map or a key list.
func (c *content) correctCreature(key string, m *srd51.Monster, field string, raw json.RawMessage) error {
	if n, ok := creatureNumbers(m)[field]; ok {
		v, err := correctedNumber(raw, n.lo, n.hi)
		if err == nil {
			*n.dst = v
		}
		return err
	}
	switch field {
	case "attacks_per_action":
		return c.correctAttacksPerAction(key, m, raw)
	case "hit_points_roll":
		return correctHitPointsRoll(m, raw)
	}
	return c.correctCreatureLists(key, m, field, raw)
}

// correctAttacksPerAction sets how many attacks a creature's Multiattack makes.
func (c *content) correctAttacksPerAction(key string, m *srd51.Monster, raw json.RawMessage) error {
	if !slices.ContainsFunc(m.Actions, func(a srd51.MonsterAction) bool { return len(a.Multiattack) > 0 }) {
		return fmt.Errorf("%s has no Multiattack to correct", key)
	}
	v, err := correctedNumber(raw, 1, maxAttacksPerAction)
	if err != nil {
		return err
	}
	c.attacksPerAction[key] = v
	return nil
}

// correctHitPointsRoll sets the hit point roll, which must roll the
// creature's hit dice.
func correctHitPointsRoll(m *srd51.Monster, raw json.RawMessage) error {
	var roll string
	if err := json.Unmarshal(raw, &roll); err != nil {
		return fmt.Errorf("needs a roll such as 6d8+6")
	}
	if match := hitPointsRollPattern.FindStringSubmatch(roll); len(match) == 0 || match[1] != m.HitDice {
		return fmt.Errorf("%q is not a roll of %s", roll, m.HitDice)
	}
	m.HitPointsRoll = roll
	return nil
}

// correctCreatureLists writes the corrected fields of a stat block that hold a
// map or a list: the skills and the immunities.
func (c *content) correctCreatureLists(key string, m *srd51.Monster, field string, raw json.RawMessage) error {
	switch field {
	case "skills":
		return c.correctCreatureSkills(m, raw)
	case "damage_immunities", "condition_immunities":
		return c.correctCreatureImmunities(key, m, field, raw)
	}
	return fmt.Errorf("field %q cannot be corrected", field)
}

// correctCreatureSkills sets the bonuses of the skills the map names.
func (c *content) correctCreatureSkills(m *srd51.Monster, raw json.RawMessage) error {
	var skills map[string]int
	if err := json.Unmarshal(raw, &skills); err != nil || len(skills) == 0 {
		return fmt.Errorf("needs a map of skill keys to bonuses")
	}
	if m.Skills == nil {
		m.Skills = map[string]int{}
	}
	for k, bonus := range skills {
		if _, ok := c.skills[k]; !ok {
			return fmt.Errorf("%q is not a skill", k)
		}
		m.Skills[k] = bonus
	}
	return nil
}

// correctCreatureImmunities replaces the damage or the condition immunities.
func (c *content) correctCreatureImmunities(key string, m *srd51.Monster, field string, raw json.RawMessage) error {
	var keys []string
	if err := json.Unmarshal(raw, &keys); err != nil || len(keys) == 0 {
		return fmt.Errorf("needs a list of keys")
	}
	prefix := "condition:"
	if field == "damage_immunities" {
		prefix = "damage-type:"
	}
	if err := c.checkImmunityKeys(keys, prefix); err != nil {
		return err
	}
	if field == "condition_immunities" {
		m.ConditionImmunities = keys
		return nil
	}
	if slices.ContainsFunc(m.Immunities, func(d srd51.MonsterDamageMod) bool { return d.Note != "" }) {
		return fmt.Errorf("%s has an immunity with a note, which a corrected list would lose", key)
	}
	m.Immunities = nil
	for _, k := range keys {
		m.Immunities = append(m.Immunities, srd51.MonsterDamageMod{Types: []string{k}})
	}
	return nil
}

// checkImmunityKeys refuses a key that is not a named key of the prefix, and
// one listed twice.
func (c *content) checkImmunityKeys(keys []string, prefix string) error {
	for i, k := range keys {
		if n, ok := c.named[k]; !ok || n == nil || !strings.HasPrefix(k, prefix) {
			return fmt.Errorf("%q is not a %s key", k, strings.TrimSuffix(prefix, ":"))
		}
		if slices.Contains(keys[:i], k) {
			return fmt.Errorf("%q is listed twice", k)
		}
	}
	return nil
}

// isActionName reports whether key is "attack:<slug>" for the name of an action
// of some SRD creature, attack or not (the Portuguese names of creature actions).
func (c *content) isActionName(key string) bool {
	slug, ok := strings.CutPrefix(key, "attack:")
	if !ok {
		return false
	}
	for _, m := range c.monsters {
		for _, a := range m.Actions {
			if slugOf(a.Name) == slug {
				return true
			}
		}
	}
	return false
}

// SpellZone is the area a table spell keeps on the map while it lasts, and whether the spell has one.
func (c *Content) SpellZone(key string) (SpellZoneRule, bool) {
	z, ok := c.c.spellZones[key]
	return z, ok
}
