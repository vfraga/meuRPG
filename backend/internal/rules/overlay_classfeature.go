package rules

import (
	"fmt"
	"slices"
	"strings"

	"github.com/PuraFome/meuRPG/backend/internal/rules/srd51"
)

// The table's features for a class (MR-025): an entry "classfeature:<slug>@mesa" of a class
// (the SRD's or the table's), at a class level. The feature goes into the class's row of that
// level, as a feature of the class's own table does, so Derive, the choices, the level-up and
// play read it where they read the class's features. It may take the place of another
// feature of the class, which then leaves the rows. Switched off by the master, the entry
// changes nothing: the class reads as it did without it.

// ClassFeatureKeyPrefix starts the key of a table class feature.
const ClassFeatureKeyPrefix = "classfeature:"

// TableClassFeature is a feature added to a class at a class level. Replaces, when set, is a
// feature of the class that it takes the place of: one that comes at the same level or later.
type TableClassFeature struct {
	TableEntry
	DescPT   []string
	Class    string
	Level    int
	Replaces string
	Effects  []Effect
	// Summon says the class feature brings the table's creatures to every character that has it (see TableSummon).
	Summon *TableSummon
}

// addClassFeature registers a class feature: its name, its text and its effects (compiled with
// the others), and queues it for applyClassFeatures, which puts it in the class's rows once every
// class has its rows.
func (b *overlayBuilder) addClassFeature(o *TableClassFeature, path string) error {
	var c entryErrors
	key := o.Key
	if !b.isClass(o.Class) {
		c.at(key, path, ".class_key", ReasonReference, "the class %q does not exist", o.Class)
	}
	if o.Level < 1 || o.Level > MaxLevel {
		c.at(key, path, ".level", ReasonValue, "the level is 1 to %d", MaxLevel)
	}
	checkTextAt(&c, key, path, ".desc_pt", o.DescPT)
	if len(o.Effects) > MaxFeatureEffects {
		c.at(key, path, ".effects", ReasonLimit, "%d effects; the limit is %d per feature", len(o.Effects), MaxFeatureEffects)
	}
	if o.Replaces != "" && (o.Replaces == key || !strings.HasPrefix(o.Replaces, featureKeyPrefix) && !strings.HasPrefix(o.Replaces, ClassFeatureKeyPrefix)) {
		c.at(key, path, ".replaces_feature_key", ReasonReference, "%q is not a feature of a class", o.Replaces)
	}
	tf := &TableFeature{Key: key, NamePT: o.NamePT, DescPT: o.DescPT, Effects: o.Effects, Summon: o.Summon}
	k := featureKind{prefix: featureKeyPrefix, owner: key, class: o.Class, level: o.Level}
	b.know(key, k)
	if err := c.err(); err != nil {
		return err
	}
	// A summon lowers to a granted_spell effect (and a hidden spell) of the entry, as a class feature's own does.
	effects, summon, err := b.lowerFeatureSummon(tf, key, path, o.Effects)
	if err != nil {
		return err
	}
	b.declareResources(k, effects)
	b.declareStates(key, key, effects)
	if len(effects) > 0 {
		b.pending = append(b.pending, pendingEffects{owner: key, entry: key, effects: effects, path: path, strict: b.strict[key], summon: summon})
	}
	b.register(o.TableEntry)
	b.n.namesEN[key], b.n.namesPT[key] = o.NamePT, o.NamePT
	if err := b.writeFeature(tf, k, effects, path); err != nil {
		return err
	}
	b.n.classFeatureOf[key] = ClassFeatureEntry{Key: key, Class: o.Class, Level: o.Level}
	b.classFeatures = append(b.classFeatures, pendingClassFeature{entry: o, path: path})
	return nil
}

// pendingClassFeature is a class feature waiting to go into the rows, with its path for refusals.
type pendingClassFeature struct {
	entry *TableClassFeature
	path  string
}

// applyClassFeatures puts each class feature (the ones not switched off) in the row of its level
// and takes out of the rows the feature it replaces. The rows are copied before they change: the
// SRD's are shared. The entries go in the order of their keys, so the answer does not depend on the
// order the caller listed them in; a feature that is replaced twice is refused at the second.
func (b *overlayBuilder) applyClassFeatures() error {
	n := b.n
	slices.SortStableFunc(b.classFeatures, func(x, y pendingClassFeature) int { return strings.Compare(x.entry.Key, y.entry.Key) })
	copied := map[string]bool{}
	row := func(class string, level int) *srd51.Level {
		rows := n.classLevels[class]
		if level < 1 || level > len(rows) {
			return nil
		}
		if !copied[class] {
			n.classLevels[class] = slices.Clone(rows)
			copied[class] = true
		}
		cp := *n.classLevels[class][level-1]
		cp.Features = slices.Clone(cp.Features)
		n.classLevels[class][level-1] = &cp
		return &cp
	}
	for _, cf := range b.classFeatures {
		o := cf.entry
		if b.offKeys[o.Key] {
			continue
		}
		if o.Replaces != "" {
			if err := b.replaceInRows(o, cf.path, row); err != nil {
				return err
			}
		}
		if r := row(o.Class, o.Level); r != nil {
			r.Features = append(r.Features, o.Key)
		}
	}
	return nil
}

// replaceInRows takes the feature o replaces out of the class's rows, at the level of o or later.
func (b *overlayBuilder) replaceInRows(o *TableClassFeature, path string, row func(string, int) *srd51.Level) error {
	rows := b.n.classLevels[o.Class]
	found := false
	for lvl := o.Level; lvl <= len(rows); lvl++ {
		if !slices.Contains(b.n.classLevels[o.Class][lvl-1].Features, o.Replaces) {
			continue
		}
		r := row(o.Class, lvl)
		r.Features = slices.DeleteFunc(r.Features, func(f string) bool { return f == o.Replaces })
		found = true
	}
	if found {
		return nil
	}
	reason := fmt.Sprintf("the class has no feature %q at level %d or later", o.Replaces, o.Level)
	for lvl := 1; lvl < o.Level && lvl <= len(rows); lvl++ {
		if slices.Contains(rows[lvl-1].Features, o.Replaces) {
			reason = fmt.Sprintf("%q comes before level %d: a character never loses what it already had", o.Replaces, o.Level)
		}
	}
	return ovErr(o.Key, "%s", reason).at(path+".replaces_feature_key", ReasonReference)
}

// ClassFeatureSource is the class key and the level of the table class feature with the key; false for any other key.
func (c *Content) ClassFeatureSource(key string) (class string, level int, ok bool) {
	cf, ok := c.c.classFeatureOf[key]
	if !ok {
		return "", 0, false
	}
	return cf.Class, cf.Level, true
}

// ClassFeatureEntry is a table class feature as the content lists it.
type ClassFeatureEntry struct {
	Key, NamePT, Class string
	Level              int
	Archived, Off      bool
}

// ClassFeatures lists the table's class features, sorted by class, level and Portuguese name.
func (c *Content) ClassFeatures() []ClassFeatureEntry {
	x := c.c
	out := make([]ClassFeatureEntry, 0, len(x.classFeatureOf))
	for _, k := range sortedKeys(x.classFeatureOf) {
		cf := x.classFeatureOf[k]
		out = append(out, ClassFeatureEntry{Key: k, NamePT: x.namePT(k), Class: cf.Class, Level: cf.Level, Archived: x.archived[k], Off: x.off[k]})
	}
	slices.SortStableFunc(out, func(a, b ClassFeatureEntry) int {
		if a.Class != b.Class {
			return strings.Compare(a.Class, b.Class)
		}
		if a.Level != b.Level {
			return a.Level - b.Level
		}
		return comparePT(a.NamePT, b.NamePT)
	})
	return out
}
