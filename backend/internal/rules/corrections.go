package rules

import (
	"encoding/json"
	"fmt"
	"io/fs"
	"slices"
	"strconv"
	"strings"

	"github.com/PuraFome/meuRPG/backend/internal/rules/srd51"
)

// correctionsFileName is the file applyCorrections reads.
const correctionsFileName = "effects/corrections.json"

// correctionsFile is the shape of effects/corrections.json: one list per kind
// of correction.
type correctionsFile struct {
	Comment       string                  `json:"_comment"`
	Corrections   []classRowCorrection    `json:"corrections"`
	Creatures     []creatureCorrection    `json:"creature_corrections"`
	Spells        []spellCorrection       `json:"spell_corrections"`
	Subclasses    []subclassCorrection    `json:"subclass_corrections"`
	ExpandedLists []expandedCorrection    `json:"expanded_list_corrections"`
	Multiclass    []multiclassCorrection  `json:"multiclass_corrections"`
	SpellLists    []spellListCorrection   `json:"spell_list_corrections"`
	SubclassRows  []subclassRowCorrection `json:"subclass_feature_corrections"`
}

// classRowCorrection sets a column of a class table at some levels.
type classRowCorrection struct {
	Class   string         `json:"class"`
	Field   string         `json:"field"`
	Source  string         `json:"source"`
	ByLevel map[string]int `json:"by_level"`
}

// creatureCorrection sets one field of a creature's stat block.
type creatureCorrection struct {
	Creature string          `json:"creature"`
	Field    string          `json:"field"`
	Value    json.RawMessage `json:"value"`
	Source   string          `json:"source"`
}

// spellCorrection sets the attack, the saving throw, the damage and the damage
// choice of a spell.
type spellCorrection struct {
	Spell       string `json:"spell"`
	Source      string `json:"source"`
	AttackType  string `json:"attack_type"`
	SaveAbility string `json:"save_ability"`
	SaveSuccess string `json:"save_success"`
	// DamageChoice is "scale" or "alternative" (see the DamageChoice constants).
	DamageChoice string                  `json:"damage_choice"`
	Damage       []spellDamageCorrection `json:"damage"`
}

type spellDamageCorrection struct {
	DamageType string            `json:"damage_type"`
	AtSlot     map[string]string `json:"at_slot_level"`
}

// subclassCorrection adds the spells a subclass always has.
type subclassCorrection struct {
	Subclass string                `json:"subclass"`
	Source   string                `json:"source"`
	Spells   []srd51.SubclassSpell `json:"add_spells"`
}

// expandedCorrection marks a subclass whose spells only join the class list.
type expandedCorrection struct {
	Subclass string `json:"subclass"`
	Source   string `json:"source"`
}

// multiclassCorrection sets what a class gives as a second class.
type multiclassCorrection struct {
	Class   string   `json:"class"`
	Field   string   `json:"field"`
	Source  string   `json:"source"`
	Choose  int      `json:"choose"`
	Options []string `json:"options"`
}

// applyCorrections reads effects/corrections.json and writes its numbers over
// the snapshot (data/ is never edited by hand, so a value the snapshot has
// wrong against the SRD 5.1 is fixed here): class table rows, creature stat
// blocks, spells and the spells a subclass always has. It refuses an unknown
// class, creature, spell, subclass, field or level, a value that does not fit
// the field, and a correction without a source.
func (c *content) applyCorrections(fsys fs.FS) error {
	const name = correctionsFileName
	var f correctionsFile
	if err := readJSON(fsys, name, &f); err != nil {
		return err
	}
	c.attacksPerAction = map[string]int{}
	// The sections apply in this order, and the first fault is the one reported.
	sections := [...]func() error{
		func() error { return c.correctSpellLists(name, f.SpellLists) },
		func() error { return c.correctSubclassFeatures(name, f.SubclassRows) },
		func() error { return c.correctCreatures(name, f.Creatures) },
		func() error { return c.correctSpells(name, f.Spells) },
		func() error { return c.correctMulticlass(name, f.Multiclass) },
		func() error { return c.correctExpandedLists(name, f.ExpandedLists) },
		func() error { return c.correctSubclassSpells(name, f.Subclasses) },
		func() error { return c.correctClassRows(name, f.Corrections) },
	}
	for _, section := range sections {
		if err := section(); err != nil {
			return err
		}
	}
	return nil
}

func (c *content) correctCreatures(name string, corrections []creatureCorrection) error {
	seen := map[string]bool{}
	for _, corr := range corrections {
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
		if seen[corr.Creature+"/"+corr.Field] {
			return fmt.Errorf("%s: %s: %s is corrected twice", name, corr.Creature, corr.Field)
		}
		seen[corr.Creature+"/"+corr.Field] = true
		if err := c.correctCreature(corr.Creature, m, corr.Field, corr.Value); err != nil {
			return fmt.Errorf("%s: %s: %s: %w", name, corr.Creature, corr.Field, err)
		}
	}
	return nil
}

func (c *content) correctSpells(name string, corrections []spellCorrection) error {
	seen := map[string]bool{}
	for _, corr := range corrections {
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
		if err := c.correctSpell(name, corr, s); err != nil {
			return err
		}
	}
	return nil
}

// correctSpell writes one spell correction over the spell: its attack, its
// saving throw, its damage table and its damage choice, in that order.
func (c *content) correctSpell(name string, corr spellCorrection, s *srd51.Spell) error {
	if corr.AttackType == "" && corr.SaveAbility == "" && corr.Damage == nil && corr.DamageChoice == "" {
		return fmt.Errorf("%s: %s corrects nothing", name, corr.Spell)
	}
	if err := correctSpellAttack(name, corr, s); err != nil {
		return err
	}
	if err := correctSpellSave(name, corr, s); err != nil {
		return err
	}
	if corr.Damage != nil {
		dmg, err := c.spellDamageTable(name, corr, s)
		if err != nil {
			return err
		}
		s.Damage = dmg
	}
	return correctSpellChoice(name, corr, s)
}

func correctSpellAttack(name string, corr spellCorrection, s *srd51.Spell) error {
	if corr.AttackType == "" {
		return nil
	}
	if corr.AttackType != "melee" && corr.AttackType != "ranged" {
		return fmt.Errorf("%s: %s: attack_type %q is not melee or ranged", name, corr.Spell, corr.AttackType)
	}
	s.AttackType = corr.AttackType
	return nil
}

func correctSpellSave(name string, corr spellCorrection, s *srd51.Spell) error {
	if corr.SaveAbility == "" {
		if corr.SaveSuccess != "" {
			return fmt.Errorf("%s: %s: save_success without save_ability", name, corr.Spell)
		}
		return nil
	}
	if _, ok := abilityIndex[Ability(corr.SaveAbility)]; !ok {
		return fmt.Errorf("%s: %s: %q is not an ability", name, corr.Spell, corr.SaveAbility)
	}
	if !slices.Contains(spellCorrectionSaveSuccess, corr.SaveSuccess) {
		return fmt.Errorf("%s: %s: save_success %q is not half, none or other", name, corr.Spell, corr.SaveSuccess)
	}
	s.SaveAbility, s.SaveSuccess = corr.SaveAbility, corr.SaveSuccess
	return nil
}

// spellDamageTable checks the corrected damage of a spell, one entry per damage
// type, and returns it.
func (c *content) spellDamageTable(name string, corr spellCorrection, s *srd51.Spell) ([]srd51.SpellDamage, error) {
	dmg := make([]srd51.SpellDamage, 0, len(corr.Damage))
	for _, d := range corr.Damage {
		if n, ok := c.named[d.DamageType]; !ok || n == nil || !strings.HasPrefix(d.DamageType, "damage-type:") {
			return nil, fmt.Errorf("%s: %s: %q is not a damage type", name, corr.Spell, d.DamageType)
		}
		if len(d.AtSlot) == 0 {
			return nil, fmt.Errorf("%s: %s: %s has no slot levels", name, corr.Spell, d.DamageType)
		}
		for slot, dice := range d.AtSlot {
			if err := checkSlotDice(name, corr.Spell, s.Level, slot, dice); err != nil {
				return nil, err
			}
		}
		dmg = append(dmg, srd51.SpellDamage{DamageType: d.DamageType, AtSlotLevel: d.AtSlot})
	}
	return dmg, nil
}

// checkSlotDice: the slot is a level the spell can be cast at, the dice are dice.
func checkSlotDice(name, spell string, spellLevel int, slot, dice string) error {
	n, err := strconv.Atoi(slot)
	if err != nil || n < max(spellLevel, 1) || n > maxCorrectedSlot {
		return fmt.Errorf("%s: %s: slot %q is not %d to %d", name, spell, slot, max(spellLevel, 1), maxCorrectedSlot)
	}
	if _, ok := ParseDice(dice); !ok {
		return fmt.Errorf("%s: %s: %q is not dice", name, spell, dice)
	}
	return nil
}

func correctSpellChoice(name string, corr spellCorrection, s *srd51.Spell) error {
	if corr.DamageChoice == "" {
		return nil
	}
	if corr.DamageChoice != DamageChoiceScale && corr.DamageChoice != DamageChoiceAlternative {
		return fmt.Errorf("%s: %s: damage_choice %q is not %s or %s", name, corr.Spell, corr.DamageChoice, DamageChoiceScale, DamageChoiceAlternative)
	}
	if len(s.Damage) < minChoiceDamageTypes {
		return fmt.Errorf("%s: %s: damage_choice needs a spell with two or more damage types", name, corr.Spell)
	}
	s.DamageChoice = corr.DamageChoice
	return nil
}

func (c *content) correctMulticlass(name string, corrections []multiclassCorrection) error {
	seen := map[string]bool{}
	for _, corr := range corrections {
		class, ok := c.classes[corr.Class]
		if !ok {
			return fmt.Errorf("%s: unknown class %q", name, corr.Class)
		}
		if !slices.Contains(multiclassCorrectionFields, corr.Field) {
			return fmt.Errorf("%s: %s: field %q cannot be corrected", name, corr.Class, corr.Field)
		}
		if corr.Source == "" {
			return fmt.Errorf("%s: %s: %s needs a source", name, corr.Class, corr.Field)
		}
		if seen[corr.Class+"/"+corr.Field] {
			return fmt.Errorf("%s: %s: %s is corrected twice", name, corr.Class, corr.Field)
		}
		seen[corr.Class+"/"+corr.Field] = true
		if err := c.checkInstrumentChoice(name, corr); err != nil {
			return err
		}
		class.Multiclass.InstrumentChoices = &srd51.Choice{Choose: corr.Choose, From: slices.Clone(corr.Options)}
	}
	return nil
}

// checkInstrumentChoice: the number to choose fits the options, which are tool
// proficiencies, each listed once.
func (c *content) checkInstrumentChoice(name string, corr multiclassCorrection) error {
	if corr.Choose < 1 || corr.Choose > len(corr.Options) {
		return fmt.Errorf("%s: %s: %s chooses %d of %d", name, corr.Class, corr.Field, corr.Choose, len(corr.Options))
	}
	for i, o := range corr.Options {
		if p, ok := c.proficiencies[o]; !ok || p.Kind != "tool" {
			return fmt.Errorf("%s: %s: %q is not a tool proficiency", name, corr.Class, o)
		}
		if slices.Contains(corr.Options[:i], o) {
			return fmt.Errorf("%s: %s: %q is listed twice", name, corr.Class, o)
		}
	}
	return nil
}

func (c *content) correctExpandedLists(name string, corrections []expandedCorrection) error {
	seen := map[string]bool{}
	for _, corr := range corrections {
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
	return nil
}

func (c *content) correctSubclassSpells(name string, corrections []subclassCorrection) error {
	seen := map[string]bool{}
	for _, corr := range corrections {
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
			if err := c.addSubclassSpell(name, corr.Subclass, sub, add); err != nil {
				return err
			}
		}
	}
	return nil
}

// addSubclassSpell adds one always-prepared spell to a subclass.
func (c *content) addSubclassSpell(name, key string, sub *srd51.Subclass, add srd51.SubclassSpell) error {
	if _, ok := c.spells[add.Spell]; !ok {
		return fmt.Errorf("%s: %s: unknown spell %q", name, key, add.Spell)
	}
	if add.ClassLevel < 1 || add.ClassLevel > MaxLevel {
		return fmt.Errorf("%s: %s: level %d is not 1 to %d", name, key, add.ClassLevel, MaxLevel)
	}
	if slices.ContainsFunc(sub.Spells, func(s srd51.SubclassSpell) bool { return s.Spell == add.Spell && s.ClassLevel == add.ClassLevel }) {
		return fmt.Errorf("%s: %s already has %s at level %d", name, key, add.Spell, add.ClassLevel)
	}
	sub.Spells = append(sub.Spells, add)
	return nil
}

func (c *content) correctClassRows(name string, corrections []classRowCorrection) error {
	for _, corr := range corrections {
		rows, ok := c.classLevels[corr.Class]
		if !ok {
			return fmt.Errorf("%s: unknown class %q", name, corr.Class)
		}
		if !slices.Contains(correctionFields, corr.Field) {
			return fmt.Errorf("%s: %s: field %q cannot be corrected", name, corr.Class, corr.Field)
		}
		for lvl, v := range corr.ByLevel {
			if err := correctClassLevel(name, corr, rows, lvl, v); err != nil {
				return err
			}
		}
	}
	return nil
}

// correctClassLevel writes one value of a class table column at a level.
func correctClassLevel(name string, corr classRowCorrection, rows []*srd51.Level, lvl string, v int) error {
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
	return nil
}
