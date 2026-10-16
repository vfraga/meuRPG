package rules

import (
	"fmt"
	"unicode/utf8"
)

// Budgets of one overlay, checked in the first pass, before anything compiles.
// The per-entry limits alone let an overlay cost seconds and hundreds of
// megabytes (300 classes of 60 features of 20 effects, each with its own formula
// to compile and keep), so the whole overlay has a budget too.
const (
	// MaxOverlayEffects is the most effects (the features', the race's
	// darkvision, and the Spellcasting effect of each casting class and subclass)
	// in one overlay.
	MaxOverlayEffects = 2000
	// MaxOverlayFormulas is the most distinct formula texts in one overlay. A
	// formula repeated in many effects is compiled once, so only the distinct
	// texts count (the prepared-spells formula of each class that prepares
	// included).
	MaxOverlayFormulas = 500
	// MaxTraits is the most traits one race has, and the most one subrace has.
	MaxTraits = 60
	// MaxFeatureEffects is the most effects one feature has.
	MaxFeatureEffects = 20
	// maxEffectList bounds the lists inside an effect (tags, targets, options,
	// granted spells).
	maxEffectList = 20

	// MaxChoiceCount bounds how many a choice effect asks the player to pick:
	// more than the 18 skills there are would never be answerable.
	MaxChoiceCount = 18
)

// budget counts what an overlay will cost while it is walked in the first pass.
type budget struct {
	effects  int
	formulas map[string]bool
}

func (bg *budget) add(path string, n int, formulas ...string) error {
	bg.effects += n
	for _, f := range formulas {
		if f != "" {
			bg.formulas[f] = true
		}
	}
	if bg.effects > MaxOverlayEffects {
		return ovErr("", "more than %d effects in the table (the limit)", MaxOverlayEffects).at(path, ReasonLimit)
	}
	if len(bg.formulas) > MaxOverlayFormulas {
		return ovErr("", "more than %d distinct formulas in the table (the limit)", MaxOverlayFormulas).at(path, ReasonLimit)
	}
	return nil
}

// feature counts one feature's effects and checks their shape.
func (bg *budget) feature(path string, f *TableFeature) error {
	if len(f.Options) > MaxFeatureOptions {
		return ovErr(f.Key, "%d options; the limit is %d per feature", len(f.Options), MaxFeatureOptions).at(path+".options", ReasonLimit)
	}
	for i := range f.Options {
		at := fmt.Sprintf("%s.options[%d]", path, i)
		if len(f.Options[i].Options) > 0 {
			return ovErr(f.Key, "an option does not have options of its own").at(at+".options", ReasonValue)
		}
		if err := bg.feature(at, &f.Options[i]); err != nil {
			return err
		}
	}
	if len(f.Effects) > MaxFeatureEffects {
		return ovErr(f.Key, "%d effects; the limit is %d per feature", len(f.Effects), MaxFeatureEffects).at(path+".effects", ReasonLimit)
	}
	for i := range f.Effects {
		e := &f.Effects[i]
		at := fmt.Sprintf("%s.effects[%d]", path, i)
		if len(e.Tags) > maxEffectList || len(e.Targets) > maxEffectList || len(e.From) > maxEffectList || len(e.Spells) > maxEffectList {
			return ovErr(f.Key, "an effect lists at most %d tags, targets, options or spells (the limit)", maxEffectList).at(at, ReasonLimit)
		}
		if utf8.RuneCountInString(e.TextPT) > maxTextRunes {
			return ovErr(f.Key, "an effect text has more than %d characters", maxTextRunes).at(at+".text_pt", ReasonText)
		}
		if err := bg.add(at, 1, e.Value, e.When, e.Max); err != nil {
			return err
		}
	}
	return nil
}

// casting counts the Spellcasting effect the engine writes for a casting class
// or subclass, and its prepared formula.
func (bg *budget) casting(path string, key string, c *TableCasting) error {
	if c == nil || c.Kind == CastingNone {
		return nil
	}
	formula := c.PreparedMax
	if formula == "" && c.Preparation == PreparationPrepared {
		formula = "synthesized:" + key
	}
	return bg.add(path+".casting", 1, formula)
}

// checkBudgets walks the whole overlay once, before anything is compiled or
// allocated, and refuses the one that is over a budget, naming the limit. The
// parts are walked in a fixed order, so the first refusal is always the same.
func checkBudgets(o *Overlay) error {
	bg := &budget{formulas: map[string]bool{}}
	for _, part := range []func(*Overlay) error{bg.classes, bg.subclasses, bg.races, bg.subraces, bg.backgrounds, bg.feats} {
		if err := part(o); err != nil {
			return err
		}
	}
	return nil
}

// classes counts each class's casting and features.
func (bg *budget) classes(o *Overlay) error {
	for ci := range o.Classes {
		tc := &o.Classes[ci]
		path := fmt.Sprintf("classes[%d]", ci)
		if err := bg.casting(path, tc.Key, &tc.Casting); err != nil {
			return err
		}
		if err := budgetRows(bg, path, tc.Key, "class", tc.Levels, func(l *TableClassLevel) []TableFeature { return l.Features }); err != nil {
			return err
		}
	}
	return nil
}

// subclasses counts each subclass's casting and features.
func (bg *budget) subclasses(o *Overlay) error {
	for si := range o.Subclasses {
		ts := &o.Subclasses[si]
		path := fmt.Sprintf("subclasses[%d]", si)
		if err := bg.casting(path, ts.Key, ts.Casting); err != nil {
			return err
		}
		if err := budgetRows(bg, path, ts.Key, "subclass", ts.Levels, func(l *TableSubclassLevel) []TableFeature { return l.Features }); err != nil {
			return err
		}
	}
	return nil
}

// budgetRows counts the features of a class's or a subclass's rows, refusing
// the one past MaxTableFeatures. kind names the entry in the message.
func budgetRows[L any](bg *budget, path, key, kind string, levels []L, features func(*L) []TableFeature) error {
	n := 0
	for li := range levels {
		fs := features(&levels[li])
		for fi := range fs {
			n++
			at := fmt.Sprintf("%s.levels[%d].features[%d]", path, li, fi)
			if n > MaxTableFeatures {
				return ovErr(key, "more than %d features; the limit is %d per %s", MaxTableFeatures, MaxTableFeatures, kind).at(at, ReasonLimit)
			}
			if err := bg.feature(at, &fs[fi]); err != nil {
				return err
			}
		}
	}
	return nil
}

// traits counts the traits of a race or a subrace.
func (bg *budget) traits(path, key string, ts []TableFeature) error {
	if len(ts) > MaxTraits {
		return ovErr(key, "%d traits; the limit is %d per race or subrace", len(ts), MaxTraits).at(path+".traits", ReasonLimit)
	}
	for i := range ts {
		if err := bg.feature(fmt.Sprintf("%s.traits[%d]", path, i), &ts[i]); err != nil {
			return err
		}
	}
	return nil
}

// races counts each race's darkvision (an effect the engine writes) and traits.
func (bg *budget) races(o *Overlay) error {
	for ri := range o.Races {
		tr := &o.Races[ri]
		path := fmt.Sprintf("races[%d]", ri)
		if tr.DarkvisionFt > 0 {
			if err := bg.add(path, 1); err != nil {
				return err
			}
		}
		if err := bg.traits(path, tr.Key, tr.Traits); err != nil {
			return err
		}
	}
	return nil
}

// subraces counts each subrace's traits.
func (bg *budget) subraces(o *Overlay) error {
	for si := range o.Subraces {
		if err := bg.traits(fmt.Sprintf("subraces[%d]", si), o.Subraces[si].Key, o.Subraces[si].Traits); err != nil {
			return err
		}
	}
	return nil
}

// backgrounds counts each background's feature.
func (bg *budget) backgrounds(o *Overlay) error {
	for bi := range o.Backgrounds {
		if err := bg.feature(fmt.Sprintf("backgrounds[%d].feature", bi), &o.Backgrounds[bi].Feature); err != nil {
			return err
		}
	}
	return nil
}

// feats counts each feat's effects, as a feature's.
func (bg *budget) feats(o *Overlay) error {
	for fi := range o.Feats {
		tf := &o.Feats[fi]
		if err := bg.feature(fmt.Sprintf("feats[%d]", fi), &TableFeature{Key: tf.Key, Effects: tf.Effects}); err != nil {
			return err
		}
	}
	return nil
}
