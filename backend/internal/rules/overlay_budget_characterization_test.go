package rules

import (
	"strconv"
	"strings"
	"testing"
)

// budgetFiller is the classes that use up a budget before the site under test:
// effects effects with no formula, then formulas effects each with a formula of
// its own, 20 to a feature and at most MaxTableFeatures features to a class.
func budgetFiller(effects, formulas int) []TableClass {
	var out []TableClass
	var cur *TableClass
	feature := func() *TableFeature {
		if cur == nil || featureCount(cur) == MaxTableFeatures {
			out = append(out, TableClass{TableEntry: TableEntry{Key: "class:enchimento-" + strconv.Itoa(len(out)) + tableSuffix}, Levels: []TableClassLevel{{}}})
			cur = &out[len(out)-1]
		}
		row := &cur.Levels[0]
		if len(row.Features) == 0 || len(row.Features[len(row.Features)-1].Effects) == MaxFeatureEffects {
			row.Features = append(row.Features, TableFeature{Key: "feature:enchimento-" + strconv.Itoa(featureCount(cur)) + tableSuffix})
		}
		return &row.Features[len(row.Features)-1]
	}
	for range effects {
		f := feature()
		f.Effects = append(f.Effects, Effect{Type: "note"})
	}
	for i := range formulas {
		f := feature()
		f.Effects = append(f.Effects, Effect{Type: "modifier", Value: "prof() + " + strconv.Itoa(i)})
	}
	return out
}

func featureCount(tc *TableClass) int {
	n := 0
	for _, row := range tc.Levels {
		n += len(row.Features)
	}
	return n
}

// budgetSites are an overlay of one thing at each place checkBudgets counts, so
// a case can put the budget's last straw on any of them.
func budgetSites() map[string]func(o *Overlay) {
	casting := func(prep string) TableCasting {
		return TableCasting{Kind: CastingFull, Ability: INT, Preparation: prep}
	}
	one := []TableFeature{{Key: "feature:um" + tableSuffix, Effects: []Effect{{Type: "note"}}}}
	oneFormula := func(e Effect) []TableFeature {
		return []TableFeature{{Key: "feature:um" + tableSuffix, Effects: []Effect{e}}}
	}
	return map[string]func(o *Overlay){
		"class casting known": func(o *Overlay) {
			o.Classes = append(o.Classes, TableClass{TableEntry: TableEntry{Key: "class:x" + tableSuffix}, Casting: casting(PreparationKnown)})
		},
		"class casting prepared": func(o *Overlay) {
			o.Classes = append(o.Classes, TableClass{TableEntry: TableEntry{Key: "class:x" + tableSuffix}, Casting: casting(PreparationPrepared)})
		},
		"class casting prepared_max": func(o *Overlay) {
			c := casting(PreparationPrepared)
			c.PreparedMax = "prof() + 999"
			o.Classes = append(o.Classes, TableClass{TableEntry: TableEntry{Key: "class:x" + tableSuffix}, Casting: c})
		},
		"class casting prepared_max repeated": func(o *Overlay) {
			c := casting(PreparationPrepared)
			c.PreparedMax = "prof() + 0"
			o.Classes = append(o.Classes, TableClass{TableEntry: TableEntry{Key: "class:x" + tableSuffix}, Casting: c})
		},
		"class casting none": func(o *Overlay) {
			o.Classes = append(o.Classes, TableClass{TableEntry: TableEntry{Key: "class:x" + tableSuffix}})
		},
		"class feature": func(o *Overlay) {
			o.Classes = append(o.Classes, TableClass{TableEntry: TableEntry{Key: "class:x" + tableSuffix}, Levels: []TableClassLevel{{}, {Features: one}}})
		},
		"class feature when": func(o *Overlay) {
			o.Classes = append(o.Classes, TableClass{TableEntry: TableEntry{Key: "class:x" + tableSuffix}, Levels: []TableClassLevel{{Features: oneFormula(Effect{Type: "note", When: "level() > 3"})}}})
		},
		"class feature max": func(o *Overlay) {
			o.Classes = append(o.Classes, TableClass{TableEntry: TableEntry{Key: "class:x" + tableSuffix}, Levels: []TableClassLevel{{Features: oneFormula(Effect{Type: "resource", Max: "prof() * 2"})}}})
		},
		"subclass without casting": func(o *Overlay) {
			o.Subclasses = append(o.Subclasses, TableSubclass{TableEntry: TableEntry{Key: "subclass:x" + tableSuffix}})
		},
		"subclass casting": func(o *Overlay) {
			o.Subclasses = append(o.Subclasses, TableSubclass{TableEntry: TableEntry{Key: "subclass:x" + tableSuffix}, Casting: &TableCasting{Kind: CastingThird, Preparation: PreparationPrepared}})
		},
		"subclass feature": func(o *Overlay) {
			o.Subclasses = append(o.Subclasses, TableSubclass{TableEntry: TableEntry{Key: "subclass:x" + tableSuffix}, Levels: []TableSubclassLevel{{}, {Features: one}}})
		},
		"race darkvision": func(o *Overlay) {
			o.Races = append(o.Races, TableRace{TableEntry: TableEntry{Key: "race:x" + tableSuffix}, DarkvisionFt: 60})
		},
		"race without darkvision": func(o *Overlay) {
			o.Races = append(o.Races, TableRace{TableEntry: TableEntry{Key: "race:x" + tableSuffix}})
		},
		"race trait": func(o *Overlay) {
			o.Races = append(o.Races, TableRace{TableEntry: TableEntry{Key: "race:x" + tableSuffix}, Traits: one})
		},
		"subrace trait": func(o *Overlay) {
			o.Subraces = append(o.Subraces, TableSubrace{TableEntry: TableEntry{Key: "subrace:x" + tableSuffix}, Traits: []TableFeature{{}, one[0]}})
		},
		"background feature": func(o *Overlay) {
			o.Backgrounds = append(o.Backgrounds, TableBackground{TableEntry: TableEntry{Key: "background:x" + tableSuffix}, Feature: one[0]})
		},
		"feat effect": func(o *Overlay) {
			o.Feats = append(o.Feats, TableFeat{TableEntry: TableEntry{Key: "feat:x" + tableSuffix}, Effects: []Effect{{Type: "note"}, {Type: "note"}}})
		},
		"option effect": func(o *Overlay) {
			f := one[0]
			f.Options = []TableFeature{{Key: "feature:opcao" + tableSuffix, Effects: []Effect{{Type: "note"}}}}
			f.Effects = nil
			o.Classes = append(o.Classes, TableClass{TableEntry: TableEntry{Key: "class:x" + tableSuffix}, Levels: []TableClassLevel{{Features: []TableFeature{f}}}})
		},
	}
}

// budgetShapeCases break one limit of checkBudgets each, or several to show which
// comes first.
func budgetShapeCases() map[string]func(o *Overlay) {
	key := func(k string) TableEntry { return TableEntry{Key: k + tableSuffix} }
	features := func(n int, prefix string) []TableFeature {
		out := make([]TableFeature, n)
		for i := range out {
			out[i] = TableFeature{Key: "feature:" + prefix + strconv.Itoa(i) + tableSuffix}
		}
		return out
	}
	effects := func(n int) []Effect { return make([]Effect, n) }
	feature := func(e Effect) TableFeature { return TableFeature{Key: "feature:f" + tableSuffix, Effects: []Effect{e}} }
	inClass := func(o *Overlay, f TableFeature) {
		o.Classes = append(o.Classes, TableClass{TableEntry: key("class:c"), Levels: []TableClassLevel{{}, {}, {Features: []TableFeature{{}, f}}}})
	}
	list := make([]string, maxEffectList+1)
	return map[string]func(o *Overlay){
		"ok: empty": func(_ *Overlay) {},
		"ok: at every per-entry limit": func(o *Overlay) {
			o.Classes = []TableClass{{TableEntry: key("class:c"), Levels: []TableClassLevel{{Features: features(MaxTableFeatures, "c")}}}}
			o.Subclasses = []TableSubclass{{TableEntry: key("subclass:s"), Levels: []TableSubclassLevel{{Features: features(MaxTableFeatures, "s")}}}}
			o.Races = []TableRace{{TableEntry: key("race:r"), Traits: features(MaxTraits, "r")}}
			o.Subraces = []TableSubrace{{TableEntry: key("subrace:s"), Traits: features(MaxTraits, "sr")}}
			o.Feats = []TableFeat{{TableEntry: key("feat:f"), Effects: effects(MaxFeatureEffects)}}
		},
		"class: 61 features over two rows": func(o *Overlay) {
			o.Classes = []TableClass{{TableEntry: key("class:c"), Levels: []TableClassLevel{{Features: features(30, "a")}, {}, {Features: features(31, "b")}}}}
		},
		"subclass: 61 features over two rows": func(o *Overlay) {
			o.Subclasses = []TableSubclass{{TableEntry: key("subclass:s"), Levels: []TableSubclassLevel{{Features: features(1, "a")}, {Features: features(60, "b")}}}}
		},
		"subclass: second subclass": func(o *Overlay) {
			o.Subclasses = []TableSubclass{{TableEntry: key("subclass:a")}, {TableEntry: key("subclass:s"), Levels: []TableSubclassLevel{{Features: features(61, "b")}}}}
		},
		"race: 61 traits": func(o *Overlay) {
			o.Races = []TableRace{{TableEntry: key("race:a")}, {TableEntry: key("race:r"), Traits: features(MaxTraits+1, "r")}}
		},
		"subrace: 61 traits": func(o *Overlay) {
			o.Subraces = []TableSubrace{{TableEntry: key("subrace:a")}, {TableEntry: key("subrace:s"), Traits: features(MaxTraits+1, "r")}}
		},
		"feat: 21 effects": func(o *Overlay) {
			o.Feats = []TableFeat{{TableEntry: key("feat:a")}, {TableEntry: key("feat:f"), Effects: effects(MaxFeatureEffects + 1)}}
		},
		"background: 21 effects": func(o *Overlay) {
			o.Backgrounds = []TableBackground{{TableEntry: key("background:a")}, {TableEntry: key("background:b"), Feature: TableFeature{Key: "background-feature:b" + tableSuffix, Effects: effects(MaxFeatureEffects + 1)}}}
		},
		"feature: 61 options": func(o *Overlay) {
			inClass(o, TableFeature{Key: "feature:f" + tableSuffix, Options: features(MaxFeatureOptions+1, "o")})
		},
		"feature: 60 options": func(o *Overlay) {
			inClass(o, TableFeature{Key: "feature:f" + tableSuffix, Options: features(MaxFeatureOptions, "o")})
		},
		"feature: an option with options": func(o *Overlay) {
			opts := features(3, "o")
			opts[2].Options = features(1, "p")
			inClass(o, TableFeature{Key: "feature:f" + tableSuffix, Options: opts})
		},
		"feature: an option with 21 effects": func(o *Overlay) {
			opts := features(2, "o")
			opts[1].Effects = effects(MaxFeatureEffects + 1)
			inClass(o, TableFeature{Key: "feature:f" + tableSuffix, Options: opts})
		},
		"feature: an option with a long text": func(o *Overlay) {
			opts := features(2, "o")
			opts[1].Effects = []Effect{{}, {TextPT: strings.Repeat("a", maxTextRunes+1)}}
			inClass(o, TableFeature{Key: "feature:f" + tableSuffix, Options: opts})
		},
		"feature: options before effects": func(o *Overlay) {
			inClass(o, TableFeature{Key: "feature:f" + tableSuffix, Options: features(MaxFeatureOptions+1, "o"), Effects: effects(MaxFeatureEffects + 1)})
		},
		"feature: 21 effects": func(o *Overlay) {
			inClass(o, TableFeature{Key: "feature:f" + tableSuffix, Effects: effects(MaxFeatureEffects + 1)})
		},
		"effect: 21 tags":    func(o *Overlay) { inClass(o, feature(Effect{Tags: list})) },
		"effect: 21 targets": func(o *Overlay) { inClass(o, feature(Effect{Targets: list})) },
		"effect: 21 from":    func(o *Overlay) { inClass(o, feature(Effect{From: list})) },
		"effect: 21 spells":  func(o *Overlay) { inClass(o, feature(Effect{Spells: list})) },
		"ok: effect: 20 of each": func(o *Overlay) {
			inClass(o, feature(Effect{Tags: list[1:], Targets: list[1:], From: list[1:], Spells: list[1:]}))
		},
		"effect: long text": func(o *Overlay) { inClass(o, feature(Effect{TextPT: strings.Repeat("é", maxTextRunes+1)})) },
		"ok: effect: text at the limit": func(o *Overlay) {
			inClass(o, feature(Effect{TextPT: strings.Repeat("é", maxTextRunes)}))
		},
		"effect: list before text": func(o *Overlay) {
			inClass(o, feature(Effect{Tags: list, TextPT: strings.Repeat("a", maxTextRunes+1)}))
		},
		"order: classes before feats": func(o *Overlay) {
			o.Feats = []TableFeat{{TableEntry: key("feat:f"), Effects: effects(MaxFeatureEffects + 1)}}
			inClass(o, TableFeature{Key: "feature:f" + tableSuffix, Effects: effects(MaxFeatureEffects + 1)})
		},
		"order: subclasses before races": func(o *Overlay) {
			o.Races = []TableRace{{TableEntry: key("race:r"), Traits: features(MaxTraits+1, "r")}}
			o.Subclasses = []TableSubclass{{TableEntry: key("subclass:s"), Levels: []TableSubclassLevel{{Features: features(61, "b")}}}}
		},
		"order: races before subraces before backgrounds": func(o *Overlay) {
			o.Backgrounds = []TableBackground{{TableEntry: key("background:b"), Feature: TableFeature{Effects: effects(MaxFeatureEffects + 1)}}}
			o.Subraces = []TableSubrace{{TableEntry: key("subrace:s"), Traits: features(MaxTraits+1, "r")}}
			o.Races = []TableRace{{TableEntry: key("race:r"), Traits: []TableFeature{{Effects: effects(MaxFeatureEffects + 1)}}}}
		},
		"order: subraces before backgrounds": func(o *Overlay) {
			o.Backgrounds = []TableBackground{{TableEntry: key("background:b"), Feature: TableFeature{Effects: effects(MaxFeatureEffects + 1)}}}
			o.Subraces = []TableSubrace{{TableEntry: key("subrace:s"), Traits: features(MaxTraits+1, "r")}}
		},
		"order: backgrounds before feats": func(o *Overlay) {
			o.Feats = []TableFeat{{TableEntry: key("feat:f"), Effects: effects(MaxFeatureEffects + 1)}}
			o.Backgrounds = []TableBackground{{TableEntry: key("background:b"), Feature: TableFeature{Effects: effects(MaxFeatureEffects + 1)}}}
		},
		"order: a class's casting before its features": func(o *Overlay) {
			o.Classes = []TableClass{{TableEntry: key("class:c"), Levels: []TableClassLevel{{Features: features(61, "a")}}}}
			o.Classes = append(budgetFiller(MaxOverlayEffects, 0), o.Classes...)
			o.Classes[len(o.Classes)-1].Casting = TableCasting{Kind: CastingFull}
		},
	}
}

// TestOverlayBudgetCharacterization freezes what checkBudgets refuses: each limit
// with its message, field and reason, the place that tips each overall budget,
// and the order the parts of the overlay are walked in.
func TestOverlayBudgetCharacterization(t *testing.T) {
	t.Parallel()
	got := map[string]string{}
	for name, edit := range budgetShapeCases() {
		var o Overlay
		edit(&o)
		got[name] = violationText(checkBudgets(&o))
	}
	for site, add := range budgetSites() {
		// One effect (or formula) more than the filler leaves room for.
		effects := Overlay{Classes: budgetFiller(MaxOverlayEffects, 0)}
		add(&effects)
		got["effects: "+site] = violationText(checkBudgets(&effects))
		formulas := Overlay{Classes: budgetFiller(0, MaxOverlayFormulas)}
		add(&formulas)
		got["formulas: "+site] = violationText(checkBudgets(&formulas))
		room := Overlay{Classes: budgetFiller(MaxOverlayEffects-MaxOverlayFormulas-2, MaxOverlayFormulas-1)}
		add(&room)
		got["room: "+site] = violationText(checkBudgets(&room))
	}
	goldenText(t, "overlay-budget-refusals", got)
}
