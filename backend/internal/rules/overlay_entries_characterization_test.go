package rules

import (
	"bytes"
	"encoding/json"
	"strconv"
	"strings"
	"testing"
)

// overlayEntryOutcome is what With answers for o: the violations, or, when it
// accepts, what it wrote for key (by the given reader).
func overlayEntryOutcome(t *testing.T, srd *Content, name string, o Overlay, written func(n *content) any) string {
	t.Helper()
	c, err := srd.With(o)
	if (err == nil) != (c != nil) || strings.HasPrefix(name, "ok:") != (err == nil) {
		t.Errorf("%s: content %v, error %v", name, c != nil, err)
	}
	if err != nil {
		return violationText(err)
	}
	b, err := json.Marshal(written(c.c))
	if err != nil {
		t.Fatal(err)
	}
	return "accepted: " + string(b)
}

// overlayFeatureCases corrupt a feature, a trait, a background feature or an
// option list, one way each, to freeze what addFeature refuses.
func overlayFeatureCases() map[string]func(o *Overlay) {
	posturas := func(o *Overlay) *TableFeature { return &o.Subclasses[0].Levels[0].Features[0] }
	more := func(o *Overlay) *TableFeature { return &o.Subclasses[0].Levels[1].Features[0] }
	trait := func(o *Overlay) *TableFeature { return &o.Races[0].Traits[0] }
	subTrait := func(o *Overlay) *TableFeature { return &o.Subraces[0].Traits[0] }
	bgFeature := func(o *Overlay) *TableFeature { return &o.Backgrounds[0].Feature }
	options := func() []TableFeature {
		return []TableFeature{postureOption("feature:opcao-a"+tableSuffix, "Opção A"), postureOption("feature:opcao-b"+tableSuffix, "Opção B")}
	}
	srdChoice := Effect{Type: "choice", Choice: "feature", Count: 1, From: []string{"feature:fighter-fighting-style-defense", "feature:fighter-fighting-style-dueling"}}
	return map[string]func(o *Overlay){
		"ok: the posture overlay":                  func(_ *Overlay) {},
		"trait key of a feature":                   func(o *Overlay) { trait(o).Key = "feature:resistente" + tableSuffix },
		"background feature key of a trait":        func(o *Overlay) { bgFeature(o).Key = "trait:luz" + tableSuffix },
		"feature key of a trait":                   func(o *Overlay) { more(o).Key = "trait:mais" + tableSuffix },
		"feature key reserved":                     func(o *Overlay) { more(o).Key = "feature:mais-ability-score-improvement" + tableSuffix },
		"trait key taken by another trait":         func(o *Overlay) { subTrait(o).Key = trait(o).Key },
		"feature key taken by an option":           func(o *Overlay) { more(o).Key = postureHeron },
		"trait name empty":                         func(o *Overlay) { trait(o).NamePT = "" },
		"feature name two lines":                   func(o *Overlay) { more(o).NamePT = "Mais\nPosturas" },
		"feature text too long":                    func(o *Overlay) { more(o).DescPT = []string{"a", strings.Repeat("a", maxTextRunes+1)} },
		"feature text too many paragraphs":         func(o *Overlay) { more(o).DescPT = make([]string, maxTextParagraphs+1) },
		"key before name before text":              func(o *Overlay) { f := more(o); f.Key, f.NamePT, f.DescPT = "feature:x", "", make([]string, 99) },
		"name before text":                         func(o *Overlay) { f := more(o); f.NamePT, f.DescPT = "", make([]string, 99) },
		"options on a trait":                       func(o *Overlay) { trait(o).Options = options() },
		"options on a background feature":          func(o *Overlay) { bgFeature(o).Options = options() },
		"options on a subrace trait":               func(o *Overlay) { subTrait(o).Options = options() },
		"options without a choice":                 func(o *Overlay) { posturas(o).Effects = posturas(o).Effects[:1] },
		"options with a choice of skills only":     func(o *Overlay) { posturas(o).Effects[1].Choice = "skill" },
		"text checked before the options' kind":    func(o *Overlay) { trait(o).Options, trait(o).DescPT = options(), make([]string, 99) },
		"options with two choices of features":     func(o *Overlay) { posturas(o).Effects = append(posturas(o).Effects, posturas(o).Effects[1]) },
		"two choices of SRD options":               func(o *Overlay) { more(o).Effects = []Effect{srdChoice, srdChoice} },
		"ok: a choice of SRD options":              func(o *Overlay) { more(o).Effects = []Effect{srdChoice} },
		"ok: a choice of SRD options and a note":   func(o *Overlay) { more(o).Effects = []Effect{{Type: "note"}, srdChoice} },
		"ok: options listed by the choice":         func(o *Overlay) { posturas(o).Effects[1].From = []string{postureHeron, postureBull, postureWolf} },
		"option key bad":                           func(o *Overlay) { posturas(o).Options[2].Key = "feature:x" },
		"option name repeated":                     func(o *Overlay) { posturas(o).Options[2].NamePT = posturas(o).Options[0].NamePT },
		"option errors before the missing choice":  func(o *Overlay) { posturas(o).Effects, posturas(o).Options[1].NamePT = nil, "" },
		"option effects compiled with the entry's": func(o *Overlay) { posturas(o).Options[1].Effects = []Effect{{Type: "teleport"}} },
		"ok: a feature with no effects":            func(o *Overlay) { more(o).Effects = nil },
		"ok: a trait with a resource": func(o *Overlay) {
			trait(o).Effects = []Effect{{Type: "resource", Resource: "folego", Max: "1", Recharge: "long_rest"}}
		},
		"an empty resource name is refused later":   func(o *Overlay) { trait(o).Effects = []Effect{{Type: "note"}, {Type: "resource"}} },
		"strict owner refuses a stray option field": func(o *Overlay) { o.Strict = []string{postureSubclass}; posturas(o).Options[1].Effects[0].Count = 1 },
	}
}

// featureWrites is what addFeature wrote for the posture overlay and the race.
func featureWrites(n *content) any {
	keys := []string{postureFeature, postureMore, postureHeron, "feature:opcao-a" + tableSuffix, "trait:resistente" + tableSuffix, "trait:olhar-firme" + tableSuffix, "background-feature:luz-guia" + tableSuffix}
	out := map[string]any{}
	for _, k := range keys {
		out[k] = map[string]any{"feature": n.features[k], "trait": n.traits[k], "effects": n.effects[k], "name": n.namesPT[k]}
	}
	out["offeredBy"] = map[string][]string{
		postureHeron: n.offeredBy[postureHeron], "feature:fighter-fighting-style-defense": n.offeredBy["feature:fighter-fighting-style-defense"],
	}
	return out
}

// TestOverlayFeatureCharacterization freezes what addFeature refuses and writes.
func TestOverlayFeatureCharacterization(t *testing.T) {
	t.Parallel()
	srd := loadForTest(t)
	got := map[string]string{}
	for name, edit := range overlayFeatureCases() {
		o := postureOverlay()
		race, sub, bg := tableMisc()
		o.Races, o.Subraces, o.Backgrounds = []TableRace{race}, []TableSubrace{sub}, []TableBackground{bg}
		edit(&o)
		got[name] = overlayEntryOutcome(t, srd, name, o, featureWrites)
	}
	// More options than a feature may list never reach addFeature through With
	// (checkBudgets refuses them first), so its own check is called directly.
	b := &overlayBuilder{base: srd.c, entries: map[string]bool{}, used: map[string]bool{}, strict: map[string]bool{}}
	b.n = srd.c.cloneForOverlay()
	f := TableFeature{Key: "feature:muitas" + tableSuffix, NamePT: "Muitas", Options: make([]TableFeature, MaxFeatureOptions+1)}
	got["61 options, called directly"] = violationText(b.addFeature(&f, featureKind{prefix: "feature:", owner: "class:x" + tableSuffix}, "classes[0].levels[0].features[0]"))
	goldenText(t, "overlay-feature-refusals", got)
}

// overlaySpellCases corrupt the table's ranged bolt one field at a time, and
// a few at once, or give it each shape of casting time, range, duration and
// target, to freeze what addSpell, checkTarget and castingTimeText answer.
func overlaySpellCases() map[string]func(s *TableSpell) {
	ct := func(unit string, amount int, trigger string) func(s *TableSpell) {
		return func(s *TableSpell) { s.CastingTime = TableCastingTime{Unit: unit, Amount: amount, TriggerPT: trigger} }
	}
	rng := func(kind string, ft int) func(s *TableSpell) {
		return func(s *TableSpell) { s.Range = TableRange{Kind: kind, DistanceFt: ft} }
	}
	dur := func(d TableDuration, conc bool) func(s *TableSpell) {
		return func(s *TableSpell) { s.Duration, s.Concentration = d, conc }
	}
	target := func(t SpellTarget) func(s *TableSpell) {
		return func(s *TableSpell) { s.Target, s.Attack = t, "" }
	}
	selfTarget := func(t SpellTarget) func(s *TableSpell) {
		return func(s *TableSpell) { s.Target, s.Attack, s.Range = t, "", TableRange{Kind: RangeSelf} }
	}
	timed := func(amount int, unit string) TableDuration {
		return TableDuration{Kind: DurationTimed, Amount: amount, Unit: unit}
	}
	return map[string]func(s *TableSpell){
		"ok: the bolt":   func(_ *TableSpell) {},
		"level -1":       func(s *TableSpell) { s.Level = -1 },
		"level 10":       func(s *TableSpell) { s.Level = 10 },
		"ok: level 9":    func(s *TableSpell) { s.Level = 9 },
		"school unknown": func(s *TableSpell) { s.School = "school:cooking" },
		"school of another kind": func(s *TableSpell) {
			s.School = "damage-type:fire"
		},

		// Casting time.
		"ok: action 0":                         ct(CastAction, 0, ""),
		"action 2":                             ct(CastAction, 2, ""),
		"ok: bonus action":                     ct(CastBonusAction, 1, ""),
		"ok: reaction with a trigger":          ct(CastReaction, 1, "quando você sofre dano"),
		"ok: reaction without a trigger":       ct(CastReaction, 1, ""),
		"trigger on an action":                 ct(CastAction, 1, "quando quiser"),
		"trigger on minutes":                   ct(CastMinute, 0, "quando quiser"),
		"trigger too long":                     ct(CastReaction, 1, strings.Repeat("a", 201)),
		"ok: trigger of 200":                   ct(CastReaction, 1, strings.Repeat("é", 200)),
		"trigger of two lines":                 ct(CastReaction, 1, "quando\nquiser"),
		"trigger untrimmed":                    ct(CastReaction, 1, "quando quiser "),
		"reaction 3 with a bad trigger":        ct(CastReaction, 3, " x"),
		"ok: 1 minute":                         ct(CastMinute, 1, ""),
		"ok: 10 minutes":                       ct(CastMinute, 10, ""),
		"ok: 60 hours":                         ct(CastHour, 60, ""),
		"0 minutes":                            ct(CastMinute, 0, ""),
		"61 hours":                             ct(CastHour, 61, ""),
		"casting time unit unknown":            ct("round", 1, ""),
		"casting time unit empty with trigger": ct("", 1, "x"),
		"ok: range self":                       func(s *TableSpell) { s.Range, s.Target = TableRange{Kind: RangeSelf}, SpellTarget{Kind: TargetSelf} },
		"ok: range touch":                      rng(RangeTouch, 0),
		"ok: range sight":                      rng(RangeSight, 0),
		"ok: range unlimited":                  rng(RangeUnlimited, 0),
		"ok: range 5280":                       rng(RangeRanged, 5280),
		"range 0":                              rng(RangeRanged, 0),
		"range 5285":                           rng(RangeRanged, 5285),
		"range 32":                             rng(RangeRanged, 32),
		"range kind unknown":                   rng("far", 60),
		"ok: until dispelled":                  dur(TableDuration{Kind: DurationUntilDispelled}, false),
		"until dispelled with concentration":   dur(TableDuration{Kind: DurationUntilDispelled}, true),
		"instantaneous with concentration":     dur(TableDuration{Kind: DurationInstantaneous}, true),
		"ok: 1 round":                          dur(timed(1, DurationRound), false),
		"ok: up to 10 minutes":                 dur(TableDuration{Kind: DurationTimed, Amount: 10, Unit: DurationMinute, UpTo: true}, false),
		"ok: concentration, 8 hours":           dur(timed(8, DurationHour), true),
		"ok: 999 days":                         dur(timed(999, DurationDay), false),
		"timed 0":                              dur(timed(0, DurationRound), false),
		"timed 1000 with a bad unit":           dur(timed(1000, "week"), false),
		"duration kind unknown":                dur(TableDuration{Kind: "forever"}, false),
		"ritual cantrip":                       func(s *TableSpell) { s.Level, s.Ritual, s.Damage = 0, true, nil },
		"ok: ritual":                           func(s *TableSpell) { s.Ritual = true },
		"ok: material with text":               func(s *TableSpell) { s.Components = TableComponents{Material: true, MaterialPT: "pó"} },
		"ok: no components":                    func(s *TableSpell) { s.Components = TableComponents{} },
		"material without text":                func(s *TableSpell) { s.Components = TableComponents{Verbal: true, Material: true} },
		"material text without material":       func(s *TableSpell) { s.Components = TableComponents{MaterialPT: "pó"} },
		"material text too long": func(s *TableSpell) {
			s.Components = TableComponents{Material: true, MaterialPT: strings.Repeat("a", maxTextRunes+1)}
		},
		"desc too long":                          func(s *TableSpell) { s.DescPT = []string{"a", strings.Repeat("a", maxTextRunes+1)} },
		"desc too many paragraphs":               func(s *TableSpell) { s.DescPT = make([]string, maxTextParagraphs+1) },
		"higher level too long":                  func(s *TableSpell) { s.HigherLevelPT = []string{strings.Repeat("a", maxTextRunes+1)} },
		"class unknown":                          func(s *TableSpell) { s.Classes = []string{"class:wizard", "class:nope"} },
		"class twice":                            func(s *TableSpell) { s.Classes = []string{"class:wizard", "class:cleric", "class:wizard"} },
		"class unknown twice":                    func(s *TableSpell) { s.Classes = []string{"class:nope", "class:nope"} },
		"ok: no classes":                         func(s *TableSpell) { s.Classes = nil },
		"class of the table that does not exist": func(s *TableSpell) { s.Classes = []string{"class:nope" + tableSuffix} },

		// Target.
		"target label":                       func(s *TableSpell) { s.Target.Label = "uma criatura" },
		"ok: self":                           selfTarget(SpellTarget{Kind: TargetSelf}),
		"self with a count":                  selfTarget(SpellTarget{Kind: TargetSelf, Count: 1}),
		"self with per slot":                 selfTarget(SpellTarget{Kind: TargetSelf, PerSlotLevel: 1}),
		"self with a shape":                  selfTarget(SpellTarget{Kind: TargetSelf, Shape: ShapeCone}),
		"self with a size":                   selfTarget(SpellTarget{Kind: TargetSelf, SizeFt: 5}),
		"self at a distance":                 target(SpellTarget{Kind: TargetSelf}),
		"ok: creature, 10 more per slot":     target(SpellTarget{Kind: TargetCreature, PerSlotLevel: 10}),
		"creature, 11 more per slot":         target(SpellTarget{Kind: TargetCreature, PerSlotLevel: 11}),
		"creature, -1 per slot":              target(SpellTarget{Kind: TargetCreature, PerSlotLevel: -1}),
		"creature with a count":              target(SpellTarget{Kind: TargetCreature, Count: 2}),
		"creature with a shape":              target(SpellTarget{Kind: TargetCreature, Shape: ShapeCone}),
		"creature with a size":               target(SpellTarget{Kind: TargetCreature, SizeFt: 10}),
		"creature, per slot and count":       target(SpellTarget{Kind: TargetCreature, PerSlotLevel: 11, Count: 2}),
		"creature with the range self":       selfTarget(SpellTarget{Kind: TargetCreature}),
		"ok: creatures 2 to 20":              target(SpellTarget{Kind: TargetCreatures, Count: 20, PerSlotLevel: 1}),
		"creatures 1":                        target(SpellTarget{Kind: TargetCreatures, Count: 1}),
		"creatures 21, per slot 11, a shape": target(SpellTarget{Kind: TargetCreatures, Count: 21, PerSlotLevel: 11, Shape: ShapeCube}),
		"creatures with a size":              target(SpellTarget{Kind: TargetCreatures, Count: 3, SizeFt: 5}),
		"creatures with the range self":      selfTarget(SpellTarget{Kind: TargetCreatures, Count: 3}),
		"ok: area":                           target(SpellTarget{Kind: TargetArea, Shape: ShapeSphere, SizeFt: 300}),
		"ok: area from self":                 selfTarget(SpellTarget{Kind: TargetArea, Shape: ShapeLine, SizeFt: 5}),
		"area shape unknown":                 target(SpellTarget{Kind: TargetArea, Shape: "star", SizeFt: 10}),
		"area size 0":                        target(SpellTarget{Kind: TargetArea, Shape: ShapeCube}),
		"area size 305":                      target(SpellTarget{Kind: TargetArea, Shape: ShapeCube, SizeFt: 305}),
		"area size 12":                       target(SpellTarget{Kind: TargetArea, Shape: ShapeCube, SizeFt: 12}),
		"area with a count":                  target(SpellTarget{Kind: TargetArea, Shape: ShapeCube, SizeFt: 10, Count: 2}),
		"area with per slot":                 target(SpellTarget{Kind: TargetArea, Shape: ShapeCube, SizeFt: 10, PerSlotLevel: 2}),
		"area with an attack":                func(s *TableSpell) { s.Target = SpellTarget{Kind: TargetArea, Shape: ShapeCube, SizeFt: 10} },
		"target kind unknown":                target(SpellTarget{Kind: "everyone"}),
		"target kind empty":                  target(SpellTarget{}),

		// Attack, save, damage and healing.
		"attack unknown":                 func(s *TableSpell) { s.Attack = "thrown" },
		"ok: melee":                      func(s *TableSpell) { s.Attack = "melee" },
		"ok: no attack":                  func(s *TableSpell) { s.Attack = "" },
		"save with an attack":            func(s *TableSpell) { s.Save = &SpellSave{Ability: DEX, OnSuccess: "half"} },
		"save of an unknown ability":     func(s *TableSpell) { s.Attack, s.Save = "", &SpellSave{Ability: "luck", OnSuccess: "none"} },
		"save on success unknown":        func(s *TableSpell) { s.Attack, s.Save = "", &SpellSave{Ability: DEX, OnSuccess: "all"} },
		"ok: save for half":              func(s *TableSpell) { s.Attack, s.Save = "", &SpellSave{Ability: DEX, OnSuccess: "half"} },
		"save for half without damage":   func(s *TableSpell) { s.Attack, s.Save, s.Damage = "", &SpellSave{Ability: DEX, OnSuccess: "half"}, nil },
		"save with a bad attack":         func(s *TableSpell) { s.Attack, s.Save = "thrown", &SpellSave{Ability: DEX, OnSuccess: "none"} },
		"5 damage types":                 func(s *TableSpell) { s.Damage = append(s.Damage, s.Damage[0], s.Damage[0], s.Damage[0], s.Damage[0]) },
		"damage type unknown":            func(s *TableSpell) { s.Damage[0].Type = "damage-type:boredom" },
		"damage type of another kind":    func(s *TableSpell) { s.Damage[0].Type = "school:evocation" },
		"damage dice with a bonus":       func(s *TableSpell) { s.Damage[0].Dice = "3d6+1" },
		"damage dice of a d7":            func(s *TableSpell) { s.Damage[0].Dice = "3d7" },
		"damage per slot of another die": func(s *TableSpell) { s.Damage[0].PerSlotLevel = "1d8" },
		"damage per tier on a leveled":   func(s *TableSpell) { s.Damage[0].PerTier = "1d6" },
		"ok: damage without per slot":    func(s *TableSpell) { s.Damage[0].PerSlotLevel = "" },
		"ok: two damage types": func(s *TableSpell) {
			s.Damage = append(s.Damage, TableSpellDamage{Type: "damage-type:fire", Dice: "1d4"})
		},
		"ok: cantrip damage by tier": func(s *TableSpell) {
			s.Level, s.Damage = 0, []TableSpellDamage{{Type: "damage-type:fire", Dice: "1d10", PerTier: "1d10"}}
		},
		"ok: cantrip damage without tiers": func(s *TableSpell) {
			s.Level, s.Damage = 0, []TableSpellDamage{{Type: "damage-type:fire", Dice: "2d6"}}
		},
		"cantrip damage per slot": func(s *TableSpell) {
			s.Level, s.Damage = 0, []TableSpellDamage{{Type: "damage-type:fire", Dice: "1d10", PerSlotLevel: "1d10"}}
		},
		"cantrip damage per tier of another die": func(s *TableSpell) {
			s.Level, s.Damage = 0, []TableSpellDamage{{Type: "damage-type:nope", Dice: "1d10", PerTier: "1d6"}}
		},
		"ok: healing": func(s *TableSpell) {
			s.Attack, s.Damage, s.Heal = "", nil, &TableSpellHeal{Dice: "1d8", PerSlotLevel: "1d8", AddsModifier: true}
		},
		"ok: healing without per slot": func(s *TableSpell) { s.Attack, s.Damage, s.Heal = "", nil, &TableSpellHeal{Dice: "2d4"} },
		"healing on a cantrip":         func(s *TableSpell) { s.Level, s.Damage, s.Heal = 0, nil, &TableSpellHeal{Dice: "1d8"} },
		"healing dice bad":             func(s *TableSpell) { s.Heal = &TableSpellHeal{Dice: "1d8+2"} },
		"healing per slot of another":  func(s *TableSpell) { s.Heal = &TableSpellHeal{Dice: "1d8", PerSlotLevel: "1d4"} },
		"everything wrong at once": func(s *TableSpell) {
			s.Level, s.School, s.Ritual = -1, "school:nope", true
			s.CastingTime = TableCastingTime{Unit: CastHour, Amount: 0, TriggerPT: "x"}
			s.Range = TableRange{Kind: RangeRanged, DistanceFt: 1}
			s.Duration, s.Concentration = TableDuration{Kind: DurationTimed, Amount: 0, Unit: "week"}, true
			s.Components = TableComponents{MaterialPT: "pó"}
			s.DescPT = []string{strings.Repeat("a", maxTextRunes+1)}
			s.Classes = []string{"class:nope", "class:wizard", "class:wizard"}
			s.Target = SpellTarget{Kind: TargetArea, Label: "x"}
			s.Save = &SpellSave{Ability: "luck", OnSuccess: "x"}
			s.Damage = []TableSpellDamage{{Type: "x", Dice: "x"}, {Type: "damage-type:fire", Dice: "1d6", PerTier: "1d6", PerSlotLevel: "1d4"}}
			s.Heal = &TableSpellHeal{Dice: "x"}
		},
	}
}

// TestOverlaySpellCharacterization freezes what addSpell refuses, field by
// field, and the SRD strings and target it writes for each shape it accepts.
func TestOverlaySpellCharacterization(t *testing.T) {
	t.Parallel()
	srd := loadForTest(t)
	got := map[string]string{}
	for name, edit := range overlaySpellCases() {
		s := tableTestSpells()[0]
		edit(&s)
		key := s.Key
		got[name] = overlayEntryOutcome(t, srd, name, Overlay{Spells: []TableSpell{s}}, func(n *content) any {
			return map[string]any{"spell": n.spells[key], "target": n.spellTargets[key]}
		})
	}
	goldenText(t, "overlay-spell-refusals", got)
}

// overlayRaceCases corrupt the table's race, background and feat, to freeze
// what addRace, addBackground and checkFeatPrerequisite refuse and write.
func overlayRaceCases() map[string]func(o *Overlay) {
	race := func(o *Overlay) *TableRace { return &o.Races[0] }
	bg := func(o *Overlay) *TableBackground { return &o.Backgrounds[0] }
	feat := func(o *Overlay) *FeatPrerequisite { return &o.Feats[0].Prerequisite }
	return map[string]func(o *Overlay){
		"ok: the race, background and feat": func(_ *Overlay) {},

		// Race.
		"race size huge":                 func(o *Overlay) { race(o).Size = "Huge" },
		"ok: race size tiny":             func(o *Overlay) { race(o).Size = "Tiny" },
		"race speed 0":                   func(o *Overlay) { race(o).SpeedFt = 0 },
		"race speed 125":                 func(o *Overlay) { race(o).SpeedFt = 125 },
		"race speed 32":                  func(o *Overlay) { race(o).SpeedFt = 32 },
		"ok: race speed 120":             func(o *Overlay) { race(o).SpeedFt = 120 },
		"race darkvision -5":             func(o *Overlay) { race(o).DarkvisionFt = -5 },
		"race darkvision 125":            func(o *Overlay) { race(o).DarkvisionFt = 125 },
		"race darkvision 7":              func(o *Overlay) { race(o).DarkvisionFt = 7 },
		"ok: race without darkvision":    func(o *Overlay) { race(o).DarkvisionFt = 0 },
		"race bonus 5":                   func(o *Overlay) { race(o).AbilityBonuses = map[Ability]int{STR: 5, DEX: -5, CHA: -4} },
		"race bonus of an unknown":       func(o *Overlay) { race(o).AbilityBonuses = map[Ability]int{"luck": 1} },
		"race choice bonuses 7":          func(o *Overlay) { race(o).ChoiceBonuses = []int{1, 1, 1, 1, 1, 1, 1} },
		"race choice bonus 0 and 5":      func(o *Overlay) { race(o).ChoiceBonuses = []int{0, 2, 5} },
		"ok: race choice bonuses sorted": func(o *Overlay) { race(o).ChoiceBonuses = []int{1, 2, 1} },
		"ok: race without choice":        func(o *Overlay) { race(o).ChoiceBonuses = nil },
		"race language choices -1":       func(o *Overlay) { race(o).LanguageChoices = -1 },
		"race language choices 5":        func(o *Overlay) { race(o).LanguageChoices = 5 },
		"race language unknown":          func(o *Overlay) { race(o).Languages = []string{"language:common", "language:nope"} },
		"race trait bad":                 func(o *Overlay) { race(o).Traits[0].NamePT = "" },
		"race two traits bad": func(o *Overlay) {
			r := race(o)
			r.Traits = append(r.Traits, TableFeature{Key: "trait:x"}, TableFeature{Key: "trait:ok" + tableSuffix, NamePT: "Ok"})
			r.Traits[0].Key = "trait:y"
		},
		"race everything wrong at once": func(o *Overlay) {
			r := race(o)
			r.Size, r.SpeedFt, r.DarkvisionFt = "", 3, 3
			r.AbilityBonuses, r.ChoiceBonuses = map[Ability]int{CON: 9, "luck": 1}, []int{1, 1, 1, 1, 1, 1, 9}
			r.LanguageChoices, r.Languages = 9, []string{"x"}
			r.Traits[0].DescPT = make([]string, 99)
		},

		// Subrace.
		"subrace bonus 5": func(o *Overlay) { o.Subraces[0].AbilityBonuses = map[Ability]int{WIS: 5} },

		// Background.
		"background one skill":         func(o *Overlay) { bg(o).Skills = bg(o).Skills[:1] },
		"background three skills":      func(o *Overlay) { bg(o).Skills = append(bg(o).Skills, "skill:arcana") },
		"background skill unknown":     func(o *Overlay) { bg(o).Skills[1] = "skill:cooking" },
		"background skill twice":       func(o *Overlay) { bg(o).Skills[1] = bg(o).Skills[0] },
		"background unknown twice":     func(o *Overlay) { bg(o).Skills = []string{"skill:x", "skill:x"} },
		"background five tools":        func(o *Overlay) { bg(o).Tools = make([]string, 5) },
		"background tool not a tool":   func(o *Overlay) { bg(o).Tools = []string{"proficiency:thieves-tools", "proficiency:light-armor"} },
		"ok: background other tool":    func(o *Overlay) { bg(o).Tools = []string{"proficiency:land-vehicles"} },
		"background language -1":       func(o *Overlay) { bg(o).LanguageChoices = -1 },
		"background language 5":        func(o *Overlay) { bg(o).LanguageChoices = 5 },
		"background equipment long":    func(o *Overlay) { bg(o).EquipmentPT = strings.Repeat("a", maxTextRunes+1) },
		"ok: background no equipment":  func(o *Overlay) { bg(o).EquipmentPT = "" },
		"background feature bad":       func(o *Overlay) { bg(o).Feature.Key = "feature:x" + tableSuffix },
		"background feature text long": func(o *Overlay) { bg(o).Feature.DescPT = []string{strings.Repeat("a", maxTextRunes+1)} },
		"background everything wrong at once": func(o *Overlay) {
			b := bg(o)
			b.Skills = []string{"skill:x", "skill:x", "skill:insight"}
			b.Tools, b.LanguageChoices = []string{"a", "b", "c", "d", "e"}, 9
			b.EquipmentPT = strings.Repeat("a", maxTextRunes+1)
			b.Feature.NamePT = ""
		},

		// Feat.
		"feat minimum of an unknown":    func(o *Overlay) { feat(o).Minimums = map[Ability]int{"luck": 13, STR: 13} },
		"feat minimum 31":               func(o *Overlay) { feat(o).Minimums = map[Ability]int{STR: MaxScore + 1, DEX: -1} },
		"ok: feat minimum 0 not asked":  func(o *Overlay) { feat(o).Minimums = map[Ability]int{STR: 0} },
		"feat any_of 31":                func(o *Overlay) { feat(o).AnyOf = map[Ability]int{WIS: 31, "x": 3} },
		"feat proficiency unknown":      func(o *Overlay) { feat(o).Proficiency = "proficiency:nope" },
		"ok: feat proficiency":          func(o *Overlay) { feat(o).Proficiency = "proficiency:light-armor" },
		"feat race unknown":             func(o *Overlay) { feat(o).Race = "race:nope" },
		"ok: feat race of the SRD":      func(o *Overlay) { feat(o).Race = "race:dwarf" },
		"ok: feat subrace of the SRD":   func(o *Overlay) { feat(o).Race = "subrace:hill-dwarf" },
		"ok: feat race of the table":    func(o *Overlay) { feat(o).Race = o.Races[0].Key },
		"ok: feat subrace of the table": func(o *Overlay) { feat(o).Race = o.Subraces[0].Key },
		"feat a class as race":          func(o *Overlay) { feat(o).Race = "class:wizard" },
		"feat level -1":                 func(o *Overlay) { feat(o).Level = -1 },
		"feat level 21":                 func(o *Overlay) { feat(o).Level = 21 },
		"ok: feat level 20, casting":    func(o *Overlay) { feat(o).Level, feat(o).Spellcasting = 20, true },
		"feat two ability increases": func(o *Overlay) {
			e := o.Feats[0].Effects[0]
			o.Feats[0].Effects = append(o.Feats[0].Effects, Effect{Type: "note"}, e, e)
		},
		"feat text too long": func(o *Overlay) { o.Feats[0].DescPT = []string{strings.Repeat("a", maxTextRunes+1)} },
		"feat everything wrong at once": func(o *Overlay) {
			p := feat(o)
			p.Minimums, p.AnyOf = map[Ability]int{"b": 1, "a": 1, CON: 0, STR: 40}, map[Ability]int{DEX: 99}
			p.Proficiency, p.Race, p.Level = "x", "y", 99
			o.Feats[0].DescPT = make([]string, 99)
		},
	}
}

// raceWrites is what addRace, addSubrace, addBackground and addFeat wrote.
func raceWrites(n *content) any {
	r, s, b := tableMisc()
	return map[string]any{
		"race": n.races[r.Key], "raceChoice": n.raceChoice[r.Key], "raceEffects": n.effects[r.Key],
		"subrace": n.subraces[s.Key], "background": n.backgrounds[b.Key], "equipment": n.bgEquipment[b.Key],
		"feat": n.feats["feat:talento"+tableSuffix], "featEffects": n.effects["feat:talento"+tableSuffix],
	}
}

// TestOverlayRaceCharacterization freezes what addRace, addBackground and
// addFeat refuse, field by field, and what they write.
func TestOverlayRaceCharacterization(t *testing.T) {
	t.Parallel()
	srd := loadForTest(t)
	got := map[string]string{}
	for name, edit := range overlayRaceCases() {
		race, sub, bg := tableMisc()
		o := Overlay{Races: []TableRace{race}, Subraces: []TableSubrace{sub}, Backgrounds: []TableBackground{bg}, Feats: []TableFeat{{
			TableEntry: TableEntry{Key: "feat:talento" + tableSuffix, NamePT: "Talento"},
			Effects:    []Effect{{Type: "ability_increase", From: []string{"str", "dex"}, Count: 1, Value: "1"}},
		}}}
		edit(&o)
		got[name] = overlayEntryOutcome(t, srd, name, o, raceWrites)
	}
	goldenText(t, "overlay-race-refusals", got)
}

// effectFieldSetters set each field unusedField knows, one at a time.
var effectFieldSetters = map[string]func(e *Effect){
	"target": func(e *Effect) { e.Target = "ac" }, "mode": func(e *Effect) { e.Mode = "add" }, "value": func(e *Effect) { e.Value = "1" },
	"proficiency": func(e *Effect) { e.Proficiency = "skill:arcana" }, "level": func(e *Effect) { e.Level = "full" },
	"roll": func(e *Effect) { e.Roll = "advantage" }, "targets": func(e *Effect) { e.Targets = []string{"save.str"} },
	"sense": func(e *Effect) { e.Sense = "darkvision" }, "range_ft": func(e *Effect) { e.RangeFt = 60 },
	"resource": func(e *Effect) { e.Resource = "x" }, "max": func(e *Effect) { e.Max = "1" }, "recharge": func(e *Effect) { e.Recharge = "long_rest" },
	"choice": func(e *Effect) { e.Choice = "skill" }, "count": func(e *Effect) { e.Count = 1 }, "from": func(e *Effect) { e.From = []string{"x"} },
	"economy": func(e *Effect) { e.Economy = "action" }, "ability": func(e *Effect) { e.Ability = "str" }, "applies": func(e *Effect) { e.Applies = "damage" },
	"ignore_cover": func(e *Effect) { e.IgnoreCover = true }, "range_multiplier": func(e *Effect) { e.RangeMultiplier = 2 },
	"no_long_range_disadvantage": func(e *Effect) { e.NoLongRangeDisadvantage = true }, "progression": func(e *Effect) { e.Progression = "full" },
	"prepares": func(e *Effect) { e.Prepares = true }, "prepared_max": func(e *Effect) { e.PreparedMax = "1" },
	"spellbook": func(e *Effect) { e.Spellbook = true }, "ritual": func(e *Effect) { e.Ritual = true }, "handler": func(e *Effect) { e.Handler = "x" },
	"max_cr": func(e *Effect) { e.MaxCR = "1" }, "no_fly": func(e *Effect) { e.NoFly = true }, "no_swim": func(e *Effect) { e.NoSwim = true },
	// Fields unusedField does not look at: a stray one is never named.
	"when": func(e *Effect) { e.When = "true" }, "tags": func(e *Effect) { e.Tags = []string{"x"} }, "text_pt": func(e *Effect) { e.TextPT = "x" },
	"spells": func(e *Effect) { e.Spells = []string{"spell:light"} }, "cap": func(e *Effect) { e.Cap = 1 }, "sides": func(e *Effect) { e.Sides = 6 },
	"recharge_if": func(e *Effect) { e.RechargeIf = "true" }, "replaces": func(e *Effect) { e.Replaces = "x" },
}

// TestOverlayEffectFieldsCharacterization freezes unusedField for every field
// on every type of the menu (and on an unknown one), what stripUnused leaves of
// an effect with every field set, and the field effectError names for each
// message of the engine.
func TestOverlayEffectFieldsCharacterization(t *testing.T) {
	t.Parallel()
	got := map[string]string{}
	types := append([]string{"teleport"}, overlayEffectTypes...)
	all := Effect{}
	for field, set := range effectFieldSetters {
		set(&all)
		var named []string
		for _, typ := range types {
			e := Effect{Type: typ}
			set(&e)
			named = append(named, typ+"="+unusedField(&e))
		}
		got["unused: "+field] = strings.Join(named, " ")
	}
	for _, typ := range types {
		e := all
		e.Type = typ
		var order []string
		for name := unusedField(&e); name != ""; name = unusedField(&e) {
			order = append(order, name)
			clearEffectField(&e, name)
		}
		b, err := json.Marshal(e)
		if err != nil {
			t.Fatal(err)
		}
		got["strip: "+typ] = strings.Join(order, ",") + " -> " + string(b)
		stripped := all
		stripped.Type = typ
		stripUnused(&stripped)
		if b2, _ := json.Marshal(stripped); !bytes.Equal(b2, b) {
			t.Errorf("stripUnused(%s) = %s, want %s", typ, b2, b)
		}
	}
	unknown := all
	clearEffectField(&unknown, "nope")
	b, err := json.Marshal(unknown)
	if err != nil {
		t.Fatal(err)
	}
	got["clear: an unknown name"] = string(b)
	for name, tc := range effectErrorCases() {
		attr, reason := effectError(&tc.e, tc.msg)
		got["error: "+name] = attr + " " + reason
	}
	goldenText(t, "overlay-effect-fields", got)
}

type effectErrorCase struct {
	e   Effect
	msg string
}

// effectErrorCases are the engine's compile messages (as compileEffect words
// them) on the effects that get them.
func effectErrorCases() map[string]effectErrorCase {
	c := map[string]effectErrorCase{}
	add := func(name string, e Effect, msg string) { c[name] = effectErrorCase{e: e, msg: msg} }
	f := func(src string) string { return "formula " + strconv.Quote(src) + ": unexpected end" }
	add("formula of when", Effect{When: "a(", Value: "b(", Max: "c("}, f("a("))
	add("formula of value", Effect{When: "a(", Value: "b(", Max: "c("}, f("b("))
	add("formula of max", Effect{When: "a(", Value: "b(", Max: "c("}, f("c("))
	add("formula of none", Effect{When: "a("}, f("z("))
	add("formula with an empty when", Effect{Value: "b("}, f(""))
	add("unknown target", Effect{Type: "modifier"}, "unknown target \"x\"")
	add("unknown target of a roll mode", Effect{Type: "roll_mode"}, "unknown target \"x\"")
	add("unknown mode", Effect{}, "unknown mode \"x\"")
	add("value is required", Effect{}, "value is required")
	add("a value needs", Effect{}, "a value needs a text")
	add("value needs text_pt", Effect{}, "a value needs text_pt to show it")
	add("unknown proficiency", Effect{}, "unknown proficiency \"x\"")
	add("unknown level", Effect{}, "unknown level \"x\"")
	add("unknown roll", Effect{}, "unknown roll \"x\"")
	add("targets are required", Effect{}, "targets are required")
	add("sense needs, known sense", Effect{Sense: "darkvision"}, "sense needs a range")
	add("sense needs, unknown sense", Effect{Sense: "smell"}, "sense needs a range")
	add("resource needs a name", Effect{}, "resource needs a name, a max and a recharge")
	add("resource needs a max", Effect{Resource: "x"}, "resource needs a name, a max and a recharge")
	add("resource needs a recharge", Effect{Resource: "x", Max: "1"}, "resource needs a name, a max and a recharge")
	add("unknown choice", Effect{}, "unknown choice \"x\"")
	add("in from", Effect{}, "\"x\" in from is unknown")
	add("ability_increase with a shape problem", Effect{Type: "ability_increase"}, "ability_increase is wrong")
	add("ability_increase well shaped", Effect{Type: "ability_increase", From: []string{"str"}, Count: 1, Value: "1"}, "ability_increase is wrong")
	add("unknown economy", Effect{}, "unknown economy \"x\"")
	add("extra_attack needs", Effect{}, "extra_attack needs a count")
	add("empty tag", Effect{}, "empty tag")
	add("unknown spell", Effect{}, "unknown spell \"x\"")
	add("anything else", Effect{}, "something else went wrong")
	add("ranged fields on a modifier", Effect{Type: "modifier", IgnoreCover: true}, "ignore_cover and the like belong to ranged_attack")
	add("ranged attack refused", Effect{Type: "ranged_attack"}, "unknown target \"x\"")
	add("first match wins", Effect{Type: "modifier"}, "unknown target and unknown mode")
	add("formula wins over target", Effect{Value: "b("}, "unknown target: "+f("b("))
	return c
}
