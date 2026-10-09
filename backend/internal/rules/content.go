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
	proficiencies map[string]*srd51.Proficiency
	equipment     map[string]*srd51.Equipment
	spells        map[string]*srd51.Spell
	monsters      map[string]*srd51.Monster
	// attacksPerAction holds the creature corrections of effects/corrections.json:
	// the Multiattack count the snapshot has wrong, by creature key.
	attacksPerAction map[string]int
	// legendaryPerRound and innateUses are the other creature corrections that
	// are not a field of the snapshot: how many legendary actions a creature takes
	// between its turns, and the uses a day of the single spell of an innate
	// spellcaster (effects/corrections.json), by creature key. monsterPlans are the
	// stat blocks as a combat reads them (monsterplan.go), built after the corrections.
	legendaryPerRound map[string]int
	innateUses        map[string]int
	monsterPlans      map[string]*MonsterPlan
	// engineTraits are the traits of a stat block the engine applies, by the SRD's name
	// (effects/monster_traits.json).
	engineTraits map[string]bool
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

	compiler *formula.Compiler
	catalog  Catalog
	// spellEntries are the Catalog's spells by key.
	spellEntries map[string]SpellEntry
	// spellDetails are the structured details of each spell, by key.
	spellDetails map[string]*SpellDetails
	// optionParents maps an option that 5e-database lists only in its
	// parent's options (no "parent" field) to that parent.
	optionParents map[string]string

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
	// srdTargets are the hand-written targets of some SRD spells
	// (effects/spell_targets.json), by spell key.
	srdTargets  map[string]SpellTarget
	raceChoice  map[string][]int
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
	c := &content{
		abilities:      map[Ability]srd51.AbilityScore{},
		skills:         map[string]*srd51.Skill{},
		races:          map[string]*srd51.Race{},
		subraces:       map[string]*srd51.Subrace{},
		traits:         map[string]*srd51.Trait{},
		classes:        map[string]*srd51.Class{},
		subclasses:     map[string]*srd51.Subclass{},
		features:       map[string]*srd51.Feature{},
		backgrounds:    map[string]*srd51.Background{},
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
	if err := readJSON(fsys, "data/manifest.json", &c.manifest); err != nil {
		return nil, err
	}
	if c.manifest.SnapshotVersion == "" {
		return nil, fmt.Errorf("data/manifest.json has no snapshot_version")
	}
	if err := c.loadData(fsys); err != nil {
		return nil, err
	}
	if err := c.checkMonsters(); err != nil {
		return nil, err
	}
	if err := c.checkMagicItems(); err != nil {
		return nil, err
	}
	if err := c.indexLevels(fsys); err != nil {
		return nil, err
	}
	if err := c.applyCorrections(fsys); err != nil {
		return nil, err
	}

	var rev effectsRevision
	if err := readJSON(fsys, "effects/revision.json", &rev); err != nil {
		return nil, err
	}
	if rev.Revision < 1 {
		return nil, fmt.Errorf("effects/revision.json: revision must be at least 1")
	}
	c.version = c.manifest.SnapshotVersion + "+fx." + strconv.Itoa(rev.Revision)

	var names struct {
		Names map[string]string `json:"names"`
	}
	if err := readJSON(fsys, "effects/names_pt.json", &names); err != nil {
		return nil, err
	}
	for k, v := range names.Names {
		if !c.exists(k) && !strings.HasPrefix(k, "sense:") && !strings.HasPrefix(k, "resource:") && !strings.HasPrefix(k, "trap:") && !strings.HasPrefix(k, "light:") && !strings.HasPrefix(k, "attunement:") && !c.isAttackName(k) {
			return nil, fmt.Errorf("effects/names_pt.json: unknown key %q", k)
		}
		c.namesPT[k] = v
	}

	classIndexes := make([]string, 0, len(c.classes))
	for k := range c.classes {
		classIndexes = append(classIndexes, strings.TrimPrefix(k, "class:"))
	}
	slices.Sort(classIndexes)
	c.compiler = formula.NewCompiler(classIndexes)
	if err := c.loadEffects(fsys); err != nil {
		return nil, err
	}
	if err := c.loadStandardActions(fsys); err != nil {
		return nil, err
	}
	if err := c.loadAdvancement(fsys); err != nil {
		return nil, err
	}
	if err := c.loadSpellEffects(fsys); err != nil {
		return nil, err
	}
	if err := c.loadSpellTargets(fsys); err != nil {
		return nil, err
	}
	if err := c.loadTraps(fsys); err != nil {
		return nil, err
	}
	if err := c.loadLights(fsys); err != nil {
		return nil, err
	}
	if err := c.loadEncounterBudget(fsys); err != nil {
		return nil, err
	}
	if err := c.loadMagicItemEffects(fsys); err != nil {
		return nil, err
	}
	if err := c.indexCasting(); err != nil {
		return nil, err
	}
	c.multiclassTable = c.findMulticlassTable()
	c.buildCatalog(nil)
	c.buildCreatures()
	if err := c.loadMonsterTraits(fsys); err != nil {
		return nil, err
	}
	if err := c.buildPlans(); err != nil {
		return nil, err
	}
	c.buildMagicItems()
	if err := c.loadTreasure(fsys); err != nil {
		return nil, err
	}
	return c, nil
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

// index reads a data file into a map by key, refusing duplicate keys.
func index[T any](fsys fs.FS, name string, key func(*T) string, into map[string]*T, namesEN map[string]string, nameOf func(*T) string) error {
	var rows []T
	if err := readJSON(fsys, "data/"+name, &rows); err != nil {
		return err
	}
	for i := range rows {
		r := &rows[i]
		k := key(r)
		if _, dup := into[k]; dup {
			return fmt.Errorf("data/%s: duplicate key %q", name, k)
		}
		into[k] = r
		namesEN[k] = nameOf(r)
	}
	return nil
}

func (c *content) loadData(fsys fs.FS) error {
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

	errs := []error{
		index(fsys, "skills.json", func(s *srd51.Skill) string { return s.Key }, c.skills, c.namesEN, func(s *srd51.Skill) string { return s.Name }),
		index(fsys, "races.json", func(r *srd51.Race) string { return r.Key }, c.races, c.namesEN, func(r *srd51.Race) string { return r.Name }),
		index(fsys, "subraces.json", func(r *srd51.Subrace) string { return r.Key }, c.subraces, c.namesEN, func(r *srd51.Subrace) string { return r.Name }),
		index(fsys, "traits.json", func(r *srd51.Trait) string { return r.Key }, c.traits, c.namesEN, func(r *srd51.Trait) string { return r.Name }),
		index(fsys, "classes.json", func(r *srd51.Class) string { return r.Key }, c.classes, c.namesEN, func(r *srd51.Class) string { return r.Name }),
		index(fsys, "subclasses.json", func(r *srd51.Subclass) string { return r.Key }, c.subclasses, c.namesEN, func(r *srd51.Subclass) string { return r.Name }),
		index(fsys, "features.json", func(r *srd51.Feature) string { return r.Key }, c.features, c.namesEN, func(r *srd51.Feature) string { return r.Name }),
		index(fsys, "backgrounds.json", func(r *srd51.Background) string { return r.Key }, c.backgrounds, c.namesEN, func(r *srd51.Background) string { return r.Name }),
		index(fsys, "proficiencies.json", func(r *srd51.Proficiency) string { return r.Key }, c.proficiencies, c.namesEN, func(r *srd51.Proficiency) string { return r.Name }),
		index(fsys, "equipment.json", func(r *srd51.Equipment) string { return r.Key }, c.equipment, c.namesEN, func(r *srd51.Equipment) string { return r.Name }),
		index(fsys, "spells.json", func(r *srd51.Spell) string { return r.Key }, c.spells, c.namesEN, func(r *srd51.Spell) string { return r.Name }),
		index(fsys, "monsters.json", func(r *srd51.Monster) string { return r.Key }, c.monsters, c.namesEN, func(r *srd51.Monster) string { return r.Name }),
		index(fsys, "magic-items.json", func(r *srd51.MagicItem) string { return r.Key }, c.magicItems, c.namesEN, func(r *srd51.MagicItem) string { return r.Name }),
		index(fsys, "languages.json", func(r *srd51.Language) string { return r.Key }, c.languages, c.namesEN, func(r *srd51.Language) string { return r.Name }),
	}
	for _, name := range []string{"damage-types.json", "magic-schools.json", "weapon-properties.json", "conditions.json"} {
		errs = append(errs, index(fsys, name, func(r *srd51.Named) string { return r.Key }, c.named, c.namesEN, func(r *srd51.Named) string { return r.Name }))
	}
	for _, err := range errs {
		if err != nil {
			return err
		}
	}
	for _, b := range c.backgrounds {
		c.namesEN[b.Feature.Key] = b.Feature.Name
	}
	for _, k := range sortedKeys(c.spells) {
		s := c.spells[k]
		if (s.AreaType == "") != (s.AreaSizeFt == 0) ||
			(s.AreaType != "" && (!slices.Contains([]string{ShapeCone, ShapeCube, ShapeCylinder, ShapeLine, ShapeSphere}, s.AreaType) || s.AreaSizeFt < 5 || s.AreaSizeFt%5 != 0)) {
			return fmt.Errorf("data/spells.json: %s has the area %q of %d ft", k, s.AreaType, s.AreaSizeFt)
		}
	}
	for k := range c.skills {
		c.skillOrder = append(c.skillOrder, k)
	}
	slices.Sort(c.skillOrder)
	for _, k := range sortedKeys(c.features) {
		for _, o := range c.features[k].Options {
			if f, ok := c.features[o]; ok && f.Parent == "" {
				c.optionParents[o] = k
			}
		}
	}
	return nil
}

func (c *content) indexLevels(fsys fs.FS) error {
	var rows []srd51.Level
	if err := readJSON(fsys, "data/levels.json", &rows); err != nil {
		return err
	}
	for i := range rows {
		r := &rows[i]
		if r.Level < 1 || r.Level > MaxLevel {
			return fmt.Errorf("data/levels.json: %s level %d out of range", r.Class, r.Level)
		}
		if r.Subclass != "" {
			if c.subclassLevels[r.Subclass] == nil {
				c.subclassLevels[r.Subclass] = map[int]*srd51.Level{}
			}
			c.subclassLevels[r.Subclass][r.Level] = r
			continue
		}
		if c.classLevels[r.Class] == nil {
			c.classLevels[r.Class] = make([]*srd51.Level, MaxLevel)
		}
		c.classLevels[r.Class][r.Level-1] = r
	}
	for key := range c.classes {
		rows := c.classLevels[key]
		if len(rows) != MaxLevel || slices.Contains(rows, nil) {
			return fmt.Errorf("data/levels.json: class %s does not have all %d levels", key, MaxLevel)
		}
	}
	return nil
}

// loadEffects reads every effects file except names_pt.json, revision.json,
// standard_actions.json, advancement.json, spells.json, traps.json, lights.json, encounter_budget.json, consumables.json, magic_item_values.json treasure.json and monster_traits.json (tables, not effects), checks
// and compiles each effect.
func (c *content) loadEffects(fsys fs.FS) error {
	files, err := fs.Glob(fsys, "effects/*.json")
	if err != nil {
		return err
	}
	for _, name := range files {
		switch path.Base(name) {
		case "names_pt.json", "revision.json", "standard_actions.json", "advancement.json", "spells.json", "spell_targets.json", "corrections.json", "traps.json", "lights.json", "consumables.json", "encounter_budget.json", "magic_item_values.json", "treasure.json", "monster_traits.json":
			continue
		}
		var f struct {
			Effects map[string][]*Effect `json:"effects"`
		}
		if err := readJSON(fsys, name, &f); err != nil {
			return err
		}
		keys := make([]string, 0, len(f.Effects))
		for k := range f.Effects {
			keys = append(keys, k)
		}
		slices.Sort(keys)
		for _, key := range keys {
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
	case strings.HasPrefix(key, "feature:"), strings.HasPrefix(key, "trait:"),
		strings.HasPrefix(key, "background:"), strings.HasPrefix(key, "race:"),
		strings.HasPrefix(key, "subrace:"), strings.HasPrefix(key, "class:"),
		strings.HasPrefix(key, "subclass:"):
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
func (c *content) buildCatalog(reuse map[string]*SpellDetails) {
	cat := Catalog{ContentVersion: c.version, Attribution: srd51.Attribution}
	for _, k := range sortedKeys(c.races) {
		r := c.races[k]
		cat.Races = append(cat.Races, RaceEntry{
			Key: k, Name: r.Name, NamePT: c.namePT(k), SpeedFt: r.SpeedFt, Size: r.Size,
			AbilityBonuses: abilityMap(r.AbilityBonuses), Subraces: r.Subraces,
			ChoiceBonuses: c.raceChoice[k], SkillProficiencies: c.traitSkills(r.Traits), Archived: c.archived[k], Off: c.off[k],
		})
	}
	for _, k := range sortedKeys(c.subraces) {
		s := c.subraces[k]
		cat.Subraces = append(cat.Subraces, SubraceEntry{
			Key: k, Name: s.Name, NamePT: c.namePT(k), Race: s.Race, AbilityBonuses: abilityMap(s.AbilityBonuses),
			SkillProficiencies: c.traitSkills(s.Traits), Archived: c.archived[k], Off: c.off[k],
		})
	}
	for _, k := range sortedKeys(c.classes) {
		cl := c.classes[k]
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
		if cast, ok := c.casting[k]; ok {
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
		}
		cat.Classes = append(cat.Classes, e)
	}
	for _, k := range sortedKeys(c.subclasses) {
		s := c.subclasses[k]
		e := SubclassEntry{Key: k, Name: s.Name, NamePT: c.namePT(k), Class: s.Class, Archived: c.archived[k], Off: c.off[k]}
		for _, ss := range s.Spells {
			// A spell that needs a feature's choice is not always prepared for everyone.
			if len(ss.WithFeatures) == 0 && !s.ExpandedList {
				e.AlwaysPrepared = append(e.AlwaysPrepared, SubclassSpellRef{Spell: ss.Spell, ClassLevel: ss.ClassLevel})
			}
		}
		if cast, ok := c.subCasting[k]; ok {
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
		}
		cat.Subclasses = append(cat.Subclasses, e)
	}
	for _, k := range sortedKeys(c.backgrounds) {
		b := c.backgrounds[k]
		cat.Backgrounds = append(cat.Backgrounds, BackgroundEntry{Key: k, Name: b.Name, NamePT: c.namePT(k), SkillProficiencies: b.Skills, EquipmentPT: c.bgEquipment[k], Archived: c.archived[k], Off: c.off[k]})
	}
	for _, k := range c.skillOrder {
		s := c.skills[k]
		cat.Skills = append(cat.Skills, SkillEntry{Key: k, Name: s.Name, NamePT: c.namePT(k), Ability: Ability(s.Ability)})
	}
	for _, k := range sortedKeys(c.equipment) {
		e := c.equipment[k]
		switch {
		case e.Armor != nil && e.Armor.Category != "shield":
			a := e.Armor
			cat.Armor = append(cat.Armor, ArmorEntry{
				Key: k, Name: e.Name, NamePT: c.namePT(k), Category: a.Category, BaseAC: a.BaseAC,
				DexBonus: a.DexBonus, MaxDexBonus: a.MaxDexBonus, StrMinimum: a.StrMinimum,
				StealthDisadvantage: a.StealthDisadvantage,
			})
		case e.Weapon != nil:
			w := e.Weapon
			normal, long := w.NormalRangeFt, w.LongRangeFt
			if w.ThrowNormalFt > 0 {
				normal, long = w.ThrowNormalFt, w.ThrowLongFt
			}
			if w.Range == "melee" && w.ThrowNormalFt == 0 {
				normal, long = 0, 0
			}
			cat.Weapons = append(cat.Weapons, WeaponEntry{
				Key: k, Name: e.Name, NamePT: c.namePT(k), Category: w.Category, Range: w.Range,
				Damage: w.Damage, DamageType: w.DamageType, TwoHandedDamage: w.TwoHandedDamage,
				Properties: w.Properties, NormalRangeFt: normal, LongRangeFt: long,
			})
		}
	}
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
		cat.Spells = append(cat.Spells, e)
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
	for _, a := range AllAbilities() {
		cat.Abilities = append(cat.Abilities, AbilityEntry{
			Ability: a, Name: c.abilities[a].FullName, NamePT: c.namePT(string(a)), AbbreviationPT: abbreviationPT[a],
		})
	}

	// Every list but the abilities is sorted by Portuguese name, as the
	// editor shows it.
	sortPT(cat.Races, func(e RaceEntry) string { return e.NamePT })
	sortPT(cat.Subraces, func(e SubraceEntry) string { return e.NamePT })
	sortPT(cat.Classes, func(e ClassEntry) string { return e.NamePT })
	sortPT(cat.Subclasses, func(e SubclassEntry) string { return e.NamePT })
	sortPT(cat.Backgrounds, func(e BackgroundEntry) string { return e.NamePT })
	sortPT(cat.Skills, func(e SkillEntry) string { return e.NamePT })
	sortPT(cat.Armor, func(e ArmorEntry) string { return e.NamePT })
	sortPT(cat.Weapons, func(e WeaponEntry) string { return e.NamePT })
	sortPT(cat.Spells, func(e SpellEntry) string { return e.NamePT })
	cat.ChallengeRatings = slices.Clone(c.ratings)
	cat.LevelXP = slices.Clone(c.levelXP)
	for _, k := range sortedKeys(c.languages) {
		cat.Languages = append(cat.Languages, NamedEntry{Key: k, NamePT: c.namePT(k), Kind: "language"})
	}
	for _, k := range sortedKeys(c.proficiencies) {
		cat.Proficiencies = append(cat.Proficiencies, NamedEntry{Key: k, NamePT: c.proficiencyNamePT(k), Kind: c.proficiencies[k].Kind})
	}
	for _, k := range sortedKeys(c.named) {
		if strings.HasPrefix(k, "damage-type:") {
			cat.DamageTypes = append(cat.DamageTypes, NamedEntry{Key: k, NamePT: c.namePT(k)})
		}
	}
	sortPT(cat.Languages, func(e NamedEntry) string { return e.NamePT })
	sortPT(cat.Proficiencies, func(e NamedEntry) string { return e.NamePT })
	sortPT(cat.DamageTypes, func(e NamedEntry) string { return e.NamePT })
	c.catalog = cat
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
	"skills", "damage_immunities", "condition_immunities",
	"action_save_success", "legendary_actions", "innate_spell_uses",
}

// correctionFields are the class table columns effects/corrections.json may
// correct. The set is closed.
var correctionFields = []string{"invocations_known"}

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

// maxLegendaryPerRound and maxInnateUses bound the numbers a creature correction may
// give for the legendary actions of a round and the uses a day of an innate spell.
const (
	maxLegendaryPerRound = 5
	maxInnateUses        = 9
)

// minChoiceDamageTypes is how many damage types a spell needs for the caster to
// have one to pick.
const minChoiceDamageTypes = 2

// maxCorrectedSlot is the highest spell slot level a damage table may list.
const maxCorrectedSlot = 9

// hitPointsRollPattern is a creature's hit point roll, "6d8+6" or "3d8".
var hitPointsRollPattern = regexp.MustCompile(`^(\d+d\d+)([+-]\d+)?$`)

// applyCorrections reads effects/corrections.json and writes its numbers over
// the snapshot (data/ is never edited by hand, so a value the snapshot has
// wrong against the SRD 5.1 is fixed here): class table rows, creature stat
// blocks, spells and the spells a subclass always has. It refuses an unknown
// class, creature, spell, subclass, field or level, a value that does not fit
// the field, and a correction without a source.
func (c *content) applyCorrections(fsys fs.FS) error {
	const name = "effects/corrections.json"
	var f struct {
		Comment     string `json:"_comment"`
		Corrections []struct {
			Class   string         `json:"class"`
			Field   string         `json:"field"`
			Source  string         `json:"source"`
			ByLevel map[string]int `json:"by_level"`
		} `json:"corrections"`
		Creatures []struct {
			Creature string `json:"creature"`
			// Action names the action of the stat block an action_save_success
			// correction is about; no other field takes one.
			Action string          `json:"action"`
			Field  string          `json:"field"`
			Value  json.RawMessage `json:"value"`
			Source string          `json:"source"`
		} `json:"creature_corrections"`
		Spells []struct {
			Spell       string `json:"spell"`
			Source      string `json:"source"`
			AttackType  string `json:"attack_type"`
			SaveAbility string `json:"save_ability"`
			SaveSuccess string `json:"save_success"`
			// DamageChoice is "scale" or "alternative" (see the DamageChoice constants).
			DamageChoice string `json:"damage_choice"`
			Damage       []struct {
				DamageType string            `json:"damage_type"`
				AtSlot     map[string]string `json:"at_slot_level"`
			} `json:"damage"`
		} `json:"spell_corrections"`
		Subclasses []struct {
			Subclass string                `json:"subclass"`
			Source   string                `json:"source"`
			Spells   []srd51.SubclassSpell `json:"add_spells"`
		} `json:"subclass_corrections"`
		ExpandedLists []struct {
			Subclass string `json:"subclass"`
			Source   string `json:"source"`
		} `json:"expanded_list_corrections"`
		SpellLists   []spellListCorrection   `json:"spell_list_corrections"`
		SubclassRows []subclassRowCorrection `json:"subclass_feature_corrections"`
	}
	if err := readJSON(fsys, name, &f); err != nil {
		return err
	}
	c.attacksPerAction = map[string]int{}
	c.legendaryPerRound, c.innateUses = map[string]int{}, map[string]int{}
	if err := c.correctSpellLists(name, f.SpellLists); err != nil {
		return err
	}
	if err := c.correctSubclassFeatures(name, f.SubclassRows); err != nil {
		return err
	}
	seen := map[string]bool{}
	for _, corr := range f.Creatures {
		m, ok := c.monsters[corr.Creature]
		if !ok {
			return fmt.Errorf("%s: unknown creature %q", name, corr.Creature)
		}
		if !slices.Contains(creatureCorrectionFields, corr.Field) {
			return fmt.Errorf("%s: %s: field %q cannot be corrected", name, corr.Creature, corr.Field)
		}
		if corr.Source == "" {
			return fmt.Errorf("%s: %s: %s needs a source", name, corr.Creature, corr.Field)
		}
		if (corr.Action != "") != (corr.Field == "action_save_success") {
			return fmt.Errorf("%s: %s: %s needs an action exactly when it is action_save_success", name, corr.Creature, corr.Field)
		}
		if seen[corr.Creature+"/"+corr.Field+"/"+corr.Action] {
			return fmt.Errorf("%s: %s: %s is corrected twice", name, corr.Creature, corr.Field)
		}
		seen[corr.Creature+"/"+corr.Field+"/"+corr.Action] = true
		if err := c.correctCreature(corr.Creature, m, corr.Field, corr.Action, corr.Value); err != nil {
			return fmt.Errorf("%s: %s: %s: %w", name, corr.Creature, corr.Field, err)
		}
	}
	seen = map[string]bool{}
	for _, corr := range f.Spells {
		s, ok := c.spells[corr.Spell]
		if !ok {
			return fmt.Errorf("%s: unknown spell %q", name, corr.Spell)
		}
		if corr.Source == "" {
			return fmt.Errorf("%s: %s needs a source", name, corr.Spell)
		}
		if seen[corr.Spell] {
			return fmt.Errorf("%s: %s is corrected twice", name, corr.Spell)
		}
		seen[corr.Spell] = true
		if corr.AttackType == "" && corr.SaveAbility == "" && corr.Damage == nil && corr.DamageChoice == "" {
			return fmt.Errorf("%s: %s corrects nothing", name, corr.Spell)
		}
		if corr.AttackType != "" {
			if corr.AttackType != "melee" && corr.AttackType != "ranged" {
				return fmt.Errorf("%s: %s: attack_type %q is not melee or ranged", name, corr.Spell, corr.AttackType)
			}
			s.AttackType = corr.AttackType
		}
		if corr.SaveAbility != "" {
			if _, ok := abilityIndex[Ability(corr.SaveAbility)]; !ok {
				return fmt.Errorf("%s: %s: %q is not an ability", name, corr.Spell, corr.SaveAbility)
			}
			if !slices.Contains(spellCorrectionSaveSuccess, corr.SaveSuccess) {
				return fmt.Errorf("%s: %s: save_success %q is not half, none or other", name, corr.Spell, corr.SaveSuccess)
			}
			s.SaveAbility, s.SaveSuccess = corr.SaveAbility, corr.SaveSuccess
		} else if corr.SaveSuccess != "" {
			return fmt.Errorf("%s: %s: save_success without save_ability", name, corr.Spell)
		}
		if corr.Damage != nil {
			dmg := make([]srd51.SpellDamage, 0, len(corr.Damage))
			for _, d := range corr.Damage {
				if n, ok := c.named[d.DamageType]; !ok || n == nil || !strings.HasPrefix(d.DamageType, "damage-type:") {
					return fmt.Errorf("%s: %s: %q is not a damage type", name, corr.Spell, d.DamageType)
				}
				if len(d.AtSlot) == 0 {
					return fmt.Errorf("%s: %s: %s has no slot levels", name, corr.Spell, d.DamageType)
				}
				for slot, dice := range d.AtSlot {
					n, err := strconv.Atoi(slot)
					if err != nil || n < max(s.Level, 1) || n > maxCorrectedSlot {
						return fmt.Errorf("%s: %s: slot %q is not %d to %d", name, corr.Spell, slot, max(s.Level, 1), maxCorrectedSlot)
					}
					if _, ok := ParseDice(dice); !ok {
						return fmt.Errorf("%s: %s: %q is not dice", name, corr.Spell, dice)
					}
				}
				dmg = append(dmg, srd51.SpellDamage{DamageType: d.DamageType, AtSlotLevel: d.AtSlot})
			}
			s.Damage = dmg
		}
		if corr.DamageChoice != "" {
			if corr.DamageChoice != DamageChoiceScale && corr.DamageChoice != DamageChoiceAlternative {
				return fmt.Errorf("%s: %s: damage_choice %q is not %s or %s", name, corr.Spell, corr.DamageChoice, DamageChoiceScale, DamageChoiceAlternative)
			}
			if len(s.Damage) < minChoiceDamageTypes {
				return fmt.Errorf("%s: %s: damage_choice needs a spell with two or more damage types", name, corr.Spell)
			}
			s.DamageChoice = corr.DamageChoice
		}
	}
	seen = map[string]bool{}
	for _, corr := range f.ExpandedLists {
		sub, ok := c.subclasses[corr.Subclass]
		if !ok {
			return fmt.Errorf("%s: unknown subclass %q", name, corr.Subclass)
		}
		if corr.Source == "" || len(sub.Spells) == 0 {
			return fmt.Errorf("%s: %s needs spells and a source", name, corr.Subclass)
		}
		if seen[corr.Subclass] {
			return fmt.Errorf("%s: %s is corrected twice", name, corr.Subclass)
		}
		seen[corr.Subclass] = true
		sub.ExpandedList = true
	}
	seen = map[string]bool{}
	for _, corr := range f.Subclasses {
		sub, ok := c.subclasses[corr.Subclass]
		if !ok {
			return fmt.Errorf("%s: unknown subclass %q", name, corr.Subclass)
		}
		if corr.Source == "" || len(corr.Spells) == 0 {
			return fmt.Errorf("%s: %s needs spells and a source", name, corr.Subclass)
		}
		if seen[corr.Subclass] {
			return fmt.Errorf("%s: %s is corrected twice", name, corr.Subclass)
		}
		seen[corr.Subclass] = true
		for _, add := range corr.Spells {
			if _, ok := c.spells[add.Spell]; !ok {
				return fmt.Errorf("%s: %s: unknown spell %q", name, corr.Subclass, add.Spell)
			}
			if add.ClassLevel < 1 || add.ClassLevel > MaxLevel {
				return fmt.Errorf("%s: %s: level %d is not 1 to %d", name, corr.Subclass, add.ClassLevel, MaxLevel)
			}
			if slices.ContainsFunc(sub.Spells, func(s srd51.SubclassSpell) bool { return s.Spell == add.Spell && s.ClassLevel == add.ClassLevel }) {
				return fmt.Errorf("%s: %s already has %s at level %d", name, corr.Subclass, add.Spell, add.ClassLevel)
			}
			sub.Spells = append(sub.Spells, add)
		}
	}
	for _, corr := range f.Corrections {
		rows, ok := c.classLevels[corr.Class]
		if !ok {
			return fmt.Errorf("%s: unknown class %q", name, corr.Class)
		}
		if !slices.Contains(correctionFields, corr.Field) {
			return fmt.Errorf("%s: %s: field %q cannot be corrected", name, corr.Class, corr.Field)
		}
		for lvl, v := range corr.ByLevel {
			n, err := strconv.Atoi(lvl)
			if err != nil || n < 1 || n > MaxLevel {
				return fmt.Errorf("%s: %s: level %q is not 1 to %d", name, corr.Class, lvl, MaxLevel)
			}
			row := rows[n-1]
			cols := map[string]json.RawMessage{}
			if err := json.Unmarshal(row.ClassSpecific, &cols); err != nil {
				return fmt.Errorf("%s: %s level %d has no class_specific columns: %w", name, corr.Class, n, err)
			}
			if _, has := cols[corr.Field]; !has {
				return fmt.Errorf("%s: %s level %d has no column %q", name, corr.Class, n, corr.Field)
			}
			cols[corr.Field] = json.RawMessage(strconv.Itoa(v))
			raw, err := json.Marshal(cols)
			if err != nil {
				return err
			}
			row.ClassSpecific = raw
		}
	}
	return nil
}

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
		sub, ok := c.subclasses[corr.Subclass]
		if !ok {
			return fmt.Errorf("%s: unknown subclass %q", name, corr.Subclass)
		}
		if corr.Source == "" || len(corr.Add) == 0 {
			return fmt.Errorf("%s: %s level %d needs features and a source", name, corr.Subclass, corr.Level)
		}
		if corr.Level < 1 || corr.Level > MaxLevel {
			return fmt.Errorf("%s: %s: level %d is not 1 to %d", name, corr.Subclass, corr.Level, MaxLevel)
		}
		id := fmt.Sprintf("%s/%d", corr.Subclass, corr.Level)
		if seen[id] {
			return fmt.Errorf("%s: %s level %d is corrected twice", name, corr.Subclass, corr.Level)
		}
		seen[id] = true
		row := c.subclassLevels[corr.Subclass][corr.Level]
		if row == nil {
			row = &srd51.Level{Class: sub.Class, Subclass: corr.Subclass, Level: corr.Level}
			if c.subclassLevels[corr.Subclass] == nil {
				c.subclassLevels[corr.Subclass] = map[int]*srd51.Level{}
			}
			c.subclassLevels[corr.Subclass][corr.Level] = row
		}
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
	}
	return nil
}

// correctCreature writes one corrected field of a stat block. raw is the
// field's JSON value: a number, a hit point roll, a skill map or a key list.
func (c *content) correctCreature(key string, m *srd51.Monster, field, action string, raw json.RawMessage) error {
	number := func(lo, hi int) (int, error) {
		var v int
		if err := json.Unmarshal(raw, &v); err != nil || v < lo || v > hi {
			return 0, fmt.Errorf("needs a number of %d to %d", lo, hi)
		}
		return v, nil
	}
	const maxFeet, maxStat = 500, 100
	// numbers are the fields that hold one number, with the range it may take.
	numbers := map[string]struct {
		dst    *int
		lo, hi int
	}{
		"armor_class":        {&m.ArmorClass, 1, maxStat},
		"hit_points":         {&m.HitPoints, 1, 1 << 16},
		"speed_walk":         {&m.Speed.Walk, 0, maxFeet},
		"speed_fly":          {&m.Speed.Fly, 0, maxFeet},
		"speed_swim":         {&m.Speed.Swim, 0, maxFeet},
		"speed_climb":        {&m.Speed.Climb, 0, maxFeet},
		"speed_burrow":       {&m.Speed.Burrow, 0, maxFeet},
		"darkvision":         {&m.Darkvision, 0, maxFeet},
		"blindsight":         {&m.Blindsight, 0, maxFeet},
		"tremorsense":        {&m.Tremorsense, 0, maxFeet},
		"truesight":          {&m.Truesight, 0, maxFeet},
		"passive_perception": {&m.PassivePerception, 1, maxStat},
	}
	if n, ok := numbers[field]; ok {
		v, err := number(n.lo, n.hi)
		if err == nil {
			*n.dst = v
		}
		return err
	}
	switch field {
	case "legendary_actions":
		if len(m.LegendaryActions) == 0 {
			return fmt.Errorf("%s has no legendary actions", key)
		}
		v, err := number(1, maxLegendaryPerRound)
		if err != nil {
			return err
		}
		c.legendaryPerRound[key] = v
		return nil
	case "innate_spell_uses":
		if !slices.ContainsFunc(m.SpecialAbilities, func(a srd51.MonsterAbility) bool { return a.Name == "Innate Spellcasting" }) {
			return fmt.Errorf("%s has no Innate Spellcasting", key)
		}
		v, err := number(1, maxInnateUses)
		if err != nil {
			return err
		}
		c.innateUses[key] = v
		return nil
	case "action_save_success":
		var outcome string
		if err := json.Unmarshal(raw, &outcome); err != nil || !slices.Contains(spellCorrectionSaveSuccess, outcome) {
			return fmt.Errorf("needs half, none or other")
		}
		i := slices.IndexFunc(m.Actions, func(a srd51.MonsterAction) bool { return a.Name == action })
		if i < 0 || m.Actions[i].Save == nil {
			return fmt.Errorf("%s has no action %q with a saving throw", key, action)
		}
		m.Actions[i].Save.OnSuccess = outcome
		return nil
	case "attacks_per_action":
		if !slices.ContainsFunc(m.Actions, func(a srd51.MonsterAction) bool { return len(a.Multiattack) > 0 }) {
			return fmt.Errorf("%s has no Multiattack to correct", key)
		}
		v, err := number(1, 20)
		if err != nil {
			return err
		}
		c.attacksPerAction[key] = v
		return nil
	case "hit_points_roll":
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
	return c.correctCreatureLists(key, m, field, raw)
}

// correctCreatureLists writes the corrected fields of a stat block that hold a
// map or a list: the skills and the immunities.
func (c *content) correctCreatureLists(key string, m *srd51.Monster, field string, raw json.RawMessage) error {
	switch field {
	case "skills":
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
	case "damage_immunities", "condition_immunities":
		var keys []string
		if err := json.Unmarshal(raw, &keys); err != nil || len(keys) == 0 {
			return fmt.Errorf("needs a list of keys")
		}
		prefix := "condition:"
		if field == "damage_immunities" {
			prefix = "damage-type:"
		}
		for i, k := range keys {
			if n, ok := c.named[k]; !ok || n == nil || !strings.HasPrefix(k, prefix) {
				return fmt.Errorf("%q is not a %s key", k, strings.TrimSuffix(prefix, ":"))
			}
			if slices.Contains(keys[:i], k) {
				return fmt.Errorf("%q is listed twice", k)
			}
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
	return fmt.Errorf("field %q cannot be corrected", field)
}

// isAttackName reports whether key is "attack:<slug>" for the name of an action, a legendary
// action, a trait or a reaction of some SRD creature (the Portuguese names of what a stat block
// does; the prefix is from the attacks, the first to be named).
func (c *content) isAttackName(key string) bool {
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
		for _, list := range [][]srd51.MonsterAbility{m.SpecialAbilities, m.Reactions, m.LegendaryActions} {
			for _, a := range list {
				if slugOf(costRe.ReplaceAllString(a.Name, "")) == slug {
					return true
				}
			}
		}
	}
	return false
}
