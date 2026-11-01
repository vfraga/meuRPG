package rules

import (
	"fmt"
	"maps"
	"slices"
	"strconv"
	"strings"
	"unicode/utf8"

	"github.com/PuraFome/meuRPG/backend/internal/rules/srd51"
)

// The table's creatures (issue #578): an entry "creature:<slug>@mesa" is an SRD creature with a few
// adjustments: its hit points, its armor class, a damage bonus on its attacks and extra traits (text
// only). The entry becomes a creature of the clone, by its own key, so the creature list and sheet,
// giving it to a character, combat and "Criar NPC" read it where they read an SRD creature
// (MonsterDerived, CreatureByKey). Summoning spells, Wild Shape and the encounter tools offer SRD
// creatures only. The base is never written: the adjusted stat block is a copy.

// CreatureKeyPrefix starts the key of a table creature.
const CreatureKeyPrefix = "creature:"

// The limits of a table creature's adjustments.
const (
	// MaxCreatureHitPoints and MaxCreatureArmorClass bound the values that replace the base's.
	MaxCreatureHitPoints  = 9999
	MaxCreatureArmorClass = 40
	// MinCreatureDamageBonus and MaxCreatureDamageBonus bound the bonus added to each attack's damage.
	MinCreatureDamageBonus = -10
	MaxCreatureDamageBonus = 30
	// MaxCreatureTraits is the most traits one creature adds.
	MaxCreatureTraits = 10
)

// TableCreature is an SRD creature (BaseKey) with adjustments. A zero HitPoints or ArmorClass keeps
// the base's; DamageBonus 0 adds nothing.
type TableCreature struct {
	TableEntry
	BaseKey     string
	HitPoints   int
	ArmorClass  int
	DamageBonus int
	Traits      []TableCreatureTrait
}

// TableCreatureTrait is a trait added to a table creature: a name and a text, no effects.
type TableCreatureTrait struct {
	NamePT string
	TextPT string
}

// addCreature registers a table creature: it checks the adjustments, derives the stat block from the
// base's and puts it in the clone's creatures.
func (b *overlayBuilder) addCreature(o *TableCreature, path string) error {
	var c entryErrors
	key := o.Key
	base, ok := b.base.monsters[o.BaseKey]
	if !ok {
		c.at(key, path, ".base_key", ReasonReference, "the creature %q does not exist in the SRD", o.BaseKey)
	}
	if o.HitPoints != 0 && (o.HitPoints < 1 || o.HitPoints > MaxCreatureHitPoints) {
		c.at(key, path, ".hit_points", ReasonValue, "the hit points are 0 (the base's) or 1 to %d", MaxCreatureHitPoints)
	}
	if o.ArmorClass != 0 && (o.ArmorClass < 1 || o.ArmorClass > MaxCreatureArmorClass) {
		c.at(key, path, ".armor_class", ReasonValue, "the armor class is 0 (the base's) or 1 to %d", MaxCreatureArmorClass)
	}
	if o.DamageBonus < MinCreatureDamageBonus || o.DamageBonus > MaxCreatureDamageBonus {
		c.at(key, path, ".damage_bonus", ReasonValue, "the damage bonus is %d to %d", MinCreatureDamageBonus, MaxCreatureDamageBonus)
	}
	checkCreatureTraits(&c, o, base, path)
	if err := c.err(); err != nil {
		return err
	}
	b.register(o.TableEntry)
	m := b.deriveCreature(base, o)
	plan, err := b.n.planOf(m)
	if err != nil {
		return ovErr(key, "%v", err).at(path, ReasonValue)
	}
	b.ownCreatures()
	n := b.n
	n.monsters[key] = m
	n.monsterPlans[key] = plan
	if v, ok := n.attacksPerAction[o.BaseKey]; ok {
		n.attacksPerAction[key] = v
	}
	n.monsterEntries = append(n.monsterEntries, n.creatureEntry(key, m))
	n.creatureOf[key] = CreatureTableEntry{Key: key, NamePT: o.NamePT, BaseKey: o.BaseKey, Archived: o.Archived}
	return nil
}

// readTraitNames are the names of the traits the engine reads by name (creatures.go, creaturecaster.go,
// creaturecasting.go): a table trait with one would not be text only, so the names are refused.
var readTraitNames = []string{magicWeaponsTrait, "Spellcasting", "Innate Spellcasting"}

// checkCreatureTraits checks the traits: their number, their names (one line, 1 to 80 characters, each
// with a slug of its own that the base's abilities and actions do not use, as the slug is part of the
// key of the feature on the sheet) and their texts (1 to 4000 characters).
func checkCreatureTraits(c *entryErrors, o *TableCreature, base *srd51.Monster, path string) {
	if len(o.Traits) > MaxCreatureTraits {
		c.at(o.Key, path, ".traits", ReasonLimit, "%d traits; the limit is %d per creature", len(o.Traits), MaxCreatureTraits)
		return
	}
	used := map[string]bool{}
	if base != nil {
		for _, a := range base.SpecialAbilities {
			used[slugOf(a.Name)] = true
		}
		for _, a := range base.Actions {
			used[slugOf(a.Name)] = true
		}
	}
	for i, t := range o.Traits {
		at := fmt.Sprintf(".traits[%d]", i)
		switch err := checkEntryName(o.Key, t.NamePT); {
		case err != nil:
			c.at(o.Key, path, at+".name_pt", ReasonName, "the name must be 1 to %d characters on one line, without spaces at the ends", maxNameRunes)
		case slugOf(t.NamePT) == "":
			c.at(o.Key, path, at+".name_pt", ReasonName, "the name must have a letter or a digit from a to z")
		case slices.ContainsFunc(readTraitNames, func(n string) bool { return strings.EqualFold(n, t.NamePT) }):
			c.at(o.Key, path, at+".name_pt", ReasonName, "the name %q is one the engine reads, not a text", t.NamePT)
		case used[slugOf(t.NamePT)]:
			c.at(o.Key, path, at+".name_pt", ReasonName, "the name is already a trait or an action of the creature")
		default:
			used[slugOf(t.NamePT)] = true
		}
		if n := utf8.RuneCountInString(t.TextPT); n < 1 || n > maxTextRunes {
			c.at(o.Key, path, at+".text_pt", ReasonText, "the text must be 1 to %d characters", maxTextRunes)
		}
	}
}

// deriveCreature is the stat block of o: a copy of the base with the key and name of the entry and its
// adjustments. What the copy shares with the base (slices of the speeds, of the damage lists) is only
// read; what it changes is copied first.
func (b *overlayBuilder) deriveCreature(base *srd51.Monster, o *TableCreature) *srd51.Monster {
	m := *base
	m.Key, m.Name = o.Key, o.NamePT
	if o.HitPoints > 0 {
		m.HitPoints, m.HitPointsRoll = o.HitPoints, ""
	}
	if o.ArmorClass > 0 {
		// The base's kind of armor class ("natural") and its notes no longer describe the value.
		m.ArmorClass, m.ArmorClassType, m.ArmorClassDesc, m.ArmorClassItems, m.ArmorClassAlts = o.ArmorClass, "", "", nil, nil
	}
	if o.DamageBonus != 0 {
		m.Actions = slices.Clone(base.Actions)
		for i := range m.Actions {
			a := &m.Actions[i]
			if !a.HasAttack || len(a.Damage) == 0 {
				continue
			}
			a.Damage = slices.Clone(a.Damage)
			a.Damage[0].Dice = addDiceBonus(a.Damage[0].Dice, o.DamageBonus)
		}
	}
	if len(o.Traits) > 0 {
		m.SpecialAbilities = slices.Clone(base.SpecialAbilities)
		for _, t := range o.Traits {
			m.SpecialAbilities = append(m.SpecialAbilities, srd51.MonsterAbility{Name: t.NamePT, Desc: t.TextPT})
		}
	}
	return &m
}

// addDiceBonus writes dice ("2d4+2", "1d6", "5") with n more added: "2d4+2" and 3 is "2d4+5". A flat
// damage stops at 0 ("1" and -10 is "0"; rolled dice are floored at 0 by combat.DamageTotal). Dice
// that cannot be read, or that end in "+MOD", are returned as they are.
func addDiceBonus(dice string, n int) string {
	f, ok := ParseDice(dice)
	if !ok || f.AddsModifier {
		return dice
	}
	f.Bonus += n
	if f.Count == 0 {
		return strconv.Itoa(max(f.Bonus, 0)) // a flat damage is never below 0
	}
	out := fmt.Sprintf("%dd%d", f.Count, f.Sides)
	switch {
	case f.Bonus > 0:
		out += "+" + strconv.Itoa(f.Bonus)
	case f.Bonus < 0:
		out += strconv.Itoa(f.Bonus)
	}
	return out
}

// ownCreatures copies the maps the creatures go into, once, so the base content's are never written.
func (b *overlayBuilder) ownCreatures() {
	if b.creaturesOwned {
		return
	}
	b.creaturesOwned = true
	n := b.n
	n.monsters = maps.Clone(n.monsters)
	n.monsterPlans = maps.Clone(n.monsterPlans)
	n.attacksPerAction = maps.Clone(n.attacksPerAction)
	n.monsterEntries = slices.Clone(n.monsterEntries)
}

// sortCreatures puts the creature list back in the order of the Portuguese names, after the table's
// creatures joined it.
func (b *overlayBuilder) sortCreatures() {
	if b.creaturesOwned {
		sortPT(b.n.monsterEntries, func(e CreatureEntry) string { return e.NamePT })
	}
}

// CreatureTableEntry is a table creature as the content lists it.
type CreatureTableEntry struct {
	Key, NamePT, BaseKey string
	Archived, Off        bool
}

// Creatures lists the table's creatures, sorted by Portuguese name.
func (c *Content) Creatures() []CreatureTableEntry {
	x := c.c
	out := make([]CreatureTableEntry, 0, len(x.creatureOf))
	for _, k := range sortedKeys(x.creatureOf) {
		e := x.creatureOf[k]
		e.Archived, e.Off = x.archived[k], x.off[k]
		out = append(out, e)
	}
	slices.SortStableFunc(out, func(a, b CreatureTableEntry) int { return strings.Compare(a.NamePT, b.NamePT) })
	return out
}
