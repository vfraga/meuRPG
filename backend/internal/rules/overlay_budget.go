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
// allocated, and refuses the one that is over a budget, naming the limit.
func checkBudgets(o *Overlay) error {
	bg := &budget{formulas: map[string]bool{}}
	for ci := range o.Classes {
		tc := &o.Classes[ci]
		path := fmt.Sprintf("classes[%d]", ci)
		if err := bg.casting(path, tc.Key, &tc.Casting); err != nil {
			return err
		}
		n := 0
		for li := range tc.Levels {
			for fi := range tc.Levels[li].Features {
				n++
				if n > MaxTableFeatures {
					return ovErr(tc.Key, "more than %d features; the limit is %d per class", MaxTableFeatures, MaxTableFeatures).at(fmt.Sprintf("%s.levels[%d].features[%d]", path, li, fi), ReasonLimit)
				}
				if err := bg.feature(fmt.Sprintf("%s.levels[%d].features[%d]", path, li, fi), &tc.Levels[li].Features[fi]); err != nil {
					return err
				}
			}
		}
	}
	for si := range o.Subclasses {
		ts := &o.Subclasses[si]
		path := fmt.Sprintf("subclasses[%d]", si)
		if err := bg.casting(path, ts.Key, ts.Casting); err != nil {
			return err
		}
		n := 0
		for li := range ts.Levels {
			for fi := range ts.Levels[li].Features {
				n++
				if n > MaxTableFeatures {
					return ovErr(ts.Key, "more than %d features; the limit is %d per subclass", MaxTableFeatures, MaxTableFeatures).at(fmt.Sprintf("%s.levels[%d].features[%d]", path, li, fi), ReasonLimit)
				}
				if err := bg.feature(fmt.Sprintf("%s.levels[%d].features[%d]", path, li, fi), &ts.Levels[li].Features[fi]); err != nil {
					return err
				}
			}
		}
	}
	traits := func(path, key string, ts []TableFeature) error {
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
	for ri := range o.Races {
		tr := &o.Races[ri]
		path := fmt.Sprintf("races[%d]", ri)
		if tr.DarkvisionFt > 0 {
			if err := bg.add(path, 1); err != nil {
				return err
			}
		}
		if err := traits(path, tr.Key, tr.Traits); err != nil {
			return err
		}
	}
	for si := range o.Subraces {
		if err := traits(fmt.Sprintf("subraces[%d]", si), o.Subraces[si].Key, o.Subraces[si].Traits); err != nil {
			return err
		}
	}
	for bi := range o.Backgrounds {
		if err := bg.feature(fmt.Sprintf("backgrounds[%d].feature", bi), &o.Backgrounds[bi].Feature); err != nil {
			return err
		}
	}
	return nil
}
