package rules

import (
	"slices"
	"strconv"
	"strings"
	"testing"
)

// classCases corrupts the generated classes and subclasses one invariant at a
// time, and a few at once, to freeze every violation addClass and addSubclass
// report, with its field and reason, and the order they come in.
func classCases(srd *Content) map[string]func(o *Overlay) {
	none := func(o *Overlay) *TableClass { return classOf(o, "class:gen-none"+tableSuffix) }
	known := func(o *Overlay) *TableClass { return classOf(o, "class:gen-full-known"+tableSuffix) }
	prepared := func(o *Overlay) *TableClass { return classOf(o, "class:gen-full-prepared"+tableSuffix) }
	half := func(o *Overlay) *TableClass { return classOf(o, "class:gen-half-prepared"+tableSuffix) }
	pact := func(o *Overlay) *TableClass { return classOf(o, "class:gen-pact-known"+tableSuffix) }
	sub := func(o *Overlay) *TableSubclass { return subclassOf2(o, "subclass:gen-none-a"+tableSuffix) }
	third := func(o *Overlay, slug, class, list string) {
		o.Subclasses = append(o.Subclasses, thirdCaster(srd, slug, class, list, PreparationKnown))
	}
	feature := func(o *Overlay) *TableFeature { return &none(o).Levels[0].Features[0] }
	return map[string]func(o *Overlay){
		"ok: the generated overlay": func(_ *Overlay) {},
		"ok: third caster":          func(o *Overlay) { third(o, "terco-bom", "class:fighter", "class:wizard") },

		// Classes.
		"class: hit die 7":                   func(o *Overlay) { none(o).HitDie = 7 },
		"class: hit die 0":                   func(o *Overlay) { none(o).HitDie = 0 },
		"class: one saving throw":            func(o *Overlay) { none(o).SavingThrows = []Ability{CON} },
		"class: three saving throws":         func(o *Overlay) { none(o).SavingThrows = []Ability{CON, WIS, DEX} },
		"class: saving throw unknown":        func(o *Overlay) { none(o).SavingThrows = []Ability{CON, "luck"} },
		"class: both saving throws unknown":  func(o *Overlay) { none(o).SavingThrows = []Ability{"x", "luck"} },
		"class: same saving throw twice":     func(o *Overlay) { none(o).SavingThrows = []Ability{CON, CON} },
		"class: skill choose negative":       func(o *Overlay) { none(o).SkillChoose = -1 },
		"class: skill choose too many":       func(o *Overlay) { none(o).SkillChoose = 9 },
		"class: skill outside the SRD":       func(o *Overlay) { none(o).SkillFrom[1] = "skill:cooking" },
		"class: proficiency is a skill":      func(o *Overlay) { none(o).Proficiencies = []string{"proficiency:skill-arcana"} },
		"class: proficiency unknown":         func(o *Overlay) { none(o).Proficiencies = []string{"proficiency:light-armor", "proficiency:nope"} },
		"class: multiclass proficiency":      func(o *Overlay) { none(o).MulticlassProficiencies = []string{"proficiency:nope"} },
		"class: multiclass skills":           func(o *Overlay) { none(o).MulticlassSkillChoose = 99 },
		"class: multiclass skills negative":  func(o *Overlay) { none(o).MulticlassSkillChoose = -1 },
		"ok: multiclass skills":              func(o *Overlay) { none(o).MulticlassSkillChoose = 2 },
		"class: minimum too high":            func(o *Overlay) { none(o).Minimums = map[Ability]int{STR: MaxScore + 1} },
		"class: minimum 0":                   func(o *Overlay) { none(o).Minimums = map[Ability]int{STR: 0} },
		"class: minimum unknown ability":     func(o *Overlay) { none(o).Minimums = map[Ability]int{"luck": 10} },
		"class: any_of too high":             func(o *Overlay) { none(o).AnyOf = map[Ability]int{DEX: 31, STR: 12} },
		"class: any_of unknown":              func(o *Overlay) { none(o).AnyOf = map[Ability]int{"luck": 10, CON: 0} },
		"ok: class without a subclass level": func(o *Overlay) { none(o).SubclassLevel = 0 },
		"sub: level against the default subclass level": func(o *Overlay) {
			none(o).SubclassLevel = 0
			sub(o).Level = 4
		},
		"class: subclass level 21":                  func(o *Overlay) { none(o).SubclassLevel = 21 },
		"class: subclass level negative":            func(o *Overlay) { none(o).SubclassLevel = -2 },
		"class: 19 rows":                            func(o *Overlay) { c := none(o); c.Levels = c.Levels[:19] },
		"class: 21 rows":                            func(o *Overlay) { c := none(o); c.Levels = append(c.Levels, TableClassLevel{}) },
		"class: no rows":                            func(o *Overlay) { none(o).Levels = nil },
		"class: ASI twice":                          func(o *Overlay) { none(o).ASILevels = []int{4, 4, 0, 25} },
		"ok: ASI sorted":                            func(o *Overlay) { none(o).ASILevels = []int{12, 4} },
		"class: ASI 21":                             func(o *Overlay) { none(o).ASILevels = []int{21} },
		"class: proficiency bonus 13":               func(o *Overlay) { none(o).Levels[3].ProfBonus = 13 },
		"class: proficiency bonus negative":         func(o *Overlay) { none(o).Levels[3].ProfBonus = -1 },
		"ok: proficiency bonus 12":                  func(o *Overlay) { none(o).Levels[3].ProfBonus = 12 },
		"class: list on a class that does not cast": func(o *Overlay) { none(o).Casting.ListFrom = "class:wizard" },
		"class: prepared_max without casting":       func(o *Overlay) { none(o).Casting.PreparedMax = "1" },
		"class: cantrips before casting":            func(o *Overlay) { none(o).Levels[2].CantripsKnown = 1 },
		"class: spells known before casting":        func(o *Overlay) { none(o).Levels[2].SpellsKnown = 1 },
		"class: slots before casting":               func(o *Overlay) { none(o).Levels[4].Slots[3] = 1 },
		"class: slots at 9 before casting":          func(o *Overlay) { none(o).Levels[4].Slots[8] = 1 },
		"class: casting kind third":                 func(o *Overlay) { known(o).Casting.Kind = CastingThird },
		"class: casting kind unknown":               func(o *Overlay) { known(o).Casting.Kind = "wild" },
		"class: casting ability":                    func(o *Overlay) { known(o).Casting.Ability = "luck" },
		"class: casting preparation":                func(o *Overlay) { known(o).Casting.Preparation = PreparationSpellbook },
		"class: prepared_max on a class that knows": func(o *Overlay) { known(o).Casting.PreparedMax = "1" },
		"class: bad prepared formula":               func(o *Overlay) { prepared(o).Casting.PreparedMax = "ghost()" },
		"class: list does not exist":                func(o *Overlay) { known(o).Casting.ListFrom = "class:fantasma" },
		"class: list chains":                        func(o *Overlay) { known(o).Casting.ListFrom = "class:gen-full-prepared" + tableSuffix },
		"class: list is its own":                    func(o *Overlay) { known(o).Casting.ListFrom = "class:gen-full-known" + tableSuffix },
		"class: list of a class that does not cast": func(o *Overlay) { known(o).Casting.ListFrom = "class:fighter" },
		"class: casting start 21":                   func(o *Overlay) { known(o).Casting.StartLevel = 21 },
		"class: a caster without slots":             func(o *Overlay) { known(o).Levels[4].Slots = [9]int{} },
		"class: pact slots of two levels":           func(o *Overlay) { pact(o).Levels[6].Slots = [9]int{1, 1} },
		"class: pact slots of two levels, first":    func(o *Overlay) { pact(o).Levels[6].Slots = [9]int{0, 1, 0, 1} },
		"class: pact without slots":                 func(o *Overlay) { pact(o).Levels[6].Slots = [9]int{} },
		"class: half caster slots before level 2":   func(o *Overlay) { half(o).Levels[0].Slots[0] = 1 },
		"class: casting columns out of range":       func(o *Overlay) { known(o).Levels[4].CantripsKnown = -1 },
		"class: casting columns too big":            func(o *Overlay) { known(o).Levels[4].SpellsKnown = 999 },
		"class: feature collides across classes":    func(o *Overlay) { known(o).Levels[0].Features[0].Key = feature(o).Key },
		"class: feature key without a suffix":       func(o *Overlay) { feature(o).Key = "feature:vigor" },
		"class: feature key reserved":               func(o *Overlay) { feature(o).Key = "feature:x-ability-score-improvement-9@mesa" },
		"class: feature name empty":                 func(o *Overlay) { feature(o).NamePT = "" },
		"class: feature effect handler":             func(o *Overlay) { feature(o).Effects = []Effect{{Type: "handler", Handler: "monk.martial_arts"}} },
		"class: casting key collides": func(o *Overlay) {
			feature(o).Key = "feature:class-gen-full-known-spellcasting" + tableSuffix
		},
		"class: ASI key collides": func(o *Overlay) {
			feature(o).Key = "feature:class-gen-none-ability-score-improvement-1" + tableSuffix
		},
		"class: 61 features": func(o *Overlay) {
			c := none(o)
			for n := range 61 {
				c.Levels[19].Features = append(c.Levels[19].Features, tf("extra-"+strconv.Itoa(n), "Extra"))
			}
		},
		"class: 62 features, one report": func(o *Overlay) {
			c := none(o)
			for n := range 62 {
				c.Levels[n%20].Features = append(c.Levels[n%20].Features, tf("extra-"+strconv.Itoa(n), "Extra"))
			}
		},
		"class: everything wrong at once": func(o *Overlay) {
			c := known(o)
			c.HitDie, c.SavingThrows, c.SkillChoose = 5, []Ability{CON, CON}, 99
			c.SkillFrom[0] = "skill:cooking"
			c.Proficiencies, c.MulticlassProficiencies = []string{"proficiency:nope"}, []string{"proficiency:nope"}
			c.MulticlassSkillChoose = -1
			c.Minimums, c.AnyOf = map[Ability]int{STR: 0}, map[Ability]int{"luck": 1}
			c.SubclassLevel, c.ASILevels = 40, []int{3, 3}
			c.Casting.Ability = "luck"
			c.Levels[1].ProfBonus = 99
			c.Levels[1].Slots = [9]int{}
			c.Levels[2].Features[0].Key = "feature:nope"
			c.Levels = c.Levels[:18]
		},
		"class: everything wrong at once, rows": func(o *Overlay) {
			c := none(o)
			c.Levels[0].ProfBonus = 99
			c.Levels[0].Slots[0] = 1
			c.Levels[1].CantripsKnown, c.Levels[1].SpellsKnown = 1, 1
			c.Levels[2].Features[0].Key = "feature:nope"
			c.Levels[4].Features[0].Effects = []Effect{{Type: "teleport"}}
		},
		"class: casting wrong, rows only range-checked": func(o *Overlay) {
			c := known(o)
			c.Casting.Kind = "wild"
			c.Levels[0].Slots[0] = 99
			c.Levels[1].CantripsKnown = -3
		},

		// Subclasses.
		"sub: parent does not exist":     func(o *Overlay) { sub(o).Class = "class:fantasma@mesa" },
		"sub: level is not the class's":  func(o *Overlay) { sub(o).Level = 5 },
		"ok: sub level 0 is the class's": func(o *Overlay) { sub(o).Level = 0 },
		"sub: description too long":      func(o *Overlay) { sub(o).DescPT = []string{strings.Repeat("a", 4001)} },
		"sub: levels out of order":       func(o *Overlay) { s := sub(o); s.Levels[0], s.Levels[1] = s.Levels[1], s.Levels[0] },
		"sub: level repeated":            func(o *Overlay) { s := sub(o); s.Levels[1].Level = s.Levels[0].Level },
		"sub: level 21":                  func(o *Overlay) { s := sub(o); s.Levels[1].Level = 21 },
		"sub: level 0":                   func(o *Overlay) { s := sub(o); s.Levels[0].Level = 0 },
		"sub: features before the level": func(o *Overlay) { sub(o).Levels[0].Level = 2 },
		"sub: feature key bad":           func(o *Overlay) { sub(o).Levels[0].Features[0].Key = "feature:nope" },
		"sub: casting on a class that casts": func(o *Overlay) {
			third(o, "mago-falso", "class:wizard", "class:cleric")
		},
		"sub: casting on a table class that casts": func(o *Overlay) {
			third(o, "clerigo-falso", "class:gen-full-known"+tableSuffix, "class:wizard")
		},
		"sub: third caster without a list": func(o *Overlay) { third(o, "sem-lista", "class:fighter", "") },
		"sub: third caster list does not exist": func(o *Overlay) {
			third(o, "lista-fantasma", "class:fighter", "class:fantasma")
		},
		"sub: third caster kind full": func(o *Overlay) {
			third(o, "terco-cheio", "class:fighter", "class:wizard")
			o.Subclasses[len(o.Subclasses)-1].Casting.Kind = CastingFull
		},
		"sub: third caster starts before the subclass": func(o *Overlay) {
			third(o, "terco-cedo", "class:fighter", "class:wizard")
			o.Subclasses[len(o.Subclasses)-1].Casting.StartLevel = 2
		},
		"sub: third caster starts at 21": func(o *Overlay) {
			third(o, "terco-tarde", "class:fighter", "class:wizard")
			o.Subclasses[len(o.Subclasses)-1].Casting.StartLevel = 21
		},
		"sub: third caster misses a row": func(o *Overlay) {
			third(o, "terco-curto", "class:fighter", "class:wizard")
			s := &o.Subclasses[len(o.Subclasses)-1]
			s.Levels = slices.Delete(s.Levels, 5, 6)
		},
		"sub: third caster without slots": func(o *Overlay) {
			third(o, "terco-seco", "class:fighter", "class:wizard")
			o.Subclasses[len(o.Subclasses)-1].Levels[8].Slots = [9]int{}
		},
		"sub: third caster columns before the start": func(o *Overlay) {
			third(o, "terco-adiantado", "class:fighter", "class:wizard")
			s := &o.Subclasses[len(o.Subclasses)-1]
			s.Casting.StartLevel = 7
			s.Levels[0].Slots[0] = 1
		},
		"sub: third caster bad prepared_max": func(o *Overlay) {
			third(o, "terco-formula", "class:fighter", "class:wizard")
			s := &o.Subclasses[len(o.Subclasses)-1]
			s.Casting.Preparation, s.Casting.PreparedMax = PreparationPrepared, "ghost()"
		},
		"sub: third caster casting key collides": func(o *Overlay) {
			third(o, "terco-colide", "class:fighter", "class:wizard")
			s := &o.Subclasses[len(o.Subclasses)-1]
			s.Levels[0].Features[0].Key = "feature:subclass-terco-colide-spellcasting" + tableSuffix
		},
		"sub: 61 features": func(o *Overlay) {
			s := sub(o)
			for n := range 61 {
				s.Levels[1].Features = append(s.Levels[1].Features, tf("sub-extra-"+strconv.Itoa(n), "Extra"))
			}
		},
		"sub: always-prepared on a subclass that never casts": func(o *Overlay) {
			sub(o).AlwaysPrepared = []TableAlwaysPrepared{{ClassLevel: 3, Spell: "spell:bless"}}
		},
		"sub: always-prepared spell does not exist": func(o *Overlay) {
			s := subclassOf2(o, "subclass:gen-full-prepared-a"+tableSuffix)
			s.AlwaysPrepared = []TableAlwaysPrepared{{ClassLevel: 3, Spell: "spell:fantasma@mesa"}}
		},
		"sub: always-prepared cantrip": func(o *Overlay) {
			s := subclassOf2(o, "subclass:gen-full-prepared-a"+tableSuffix)
			s.AlwaysPrepared = []TableAlwaysPrepared{{ClassLevel: 3, Spell: "spell:fire-bolt"}}
		},
		"sub: always-prepared class level": func(o *Overlay) {
			s := subclassOf2(o, "subclass:gen-full-prepared-a"+tableSuffix)
			s.AlwaysPrepared = []TableAlwaysPrepared{{ClassLevel: 0, Spell: "spell:bless"}, {ClassLevel: 21, Spell: "spell:bless"}}
		},
		"sub: always-prepared level and spell both wrong": func(o *Overlay) {
			s := subclassOf2(o, "subclass:gen-full-prepared-a"+tableSuffix)
			s.AlwaysPrepared = []TableAlwaysPrepared{{ClassLevel: 0, Spell: "spell:nope"}}
		},
		"sub: everything wrong at once": func(o *Overlay) {
			third(o, "terco-tudo", "class:wizard", "class:fantasma")
			s := &o.Subclasses[len(o.Subclasses)-1]
			s.Level = 9
			s.DescPT = []string{strings.Repeat("a", 4001)}
			s.Casting.Kind = CastingFull
			s.Levels[0].Level = 2
			s.Levels[1].Level = 2
			s.AlwaysPrepared = []TableAlwaysPrepared{{ClassLevel: 99, Spell: "spell:fire-bolt"}}
		},
		"sub: parent missing and casting": func(o *Overlay) {
			third(o, "terco-orfao", "class:fantasma@mesa", "class:wizard")
			s := &o.Subclasses[len(o.Subclasses)-1]
			s.Level = 9
			s.AlwaysPrepared = []TableAlwaysPrepared{{ClassLevel: 3, Spell: "spell:bless"}}
		},
	}
}

// TestOverlayClassCharacterization freezes what addClass and addSubclass
// report for a class or a subclass gone wrong: every violation, its field and
// its reason, in the order they come.
func TestOverlayClassCharacterization(t *testing.T) {
	t.Parallel()
	srd := loadForTest(t)
	got := map[string]string{}
	for name, edit := range classCases(srd) {
		o := fullOverlay(t, srd)
		edit(&o)
		c, err := srd.With(o)
		got[name] = violationText(err)
		if (err == nil) != (c != nil) || strings.HasPrefix(name, "ok:") != (err == nil) {
			t.Errorf("%s: content %v, error %v", name, c != nil, err)
		}
	}
	goldenText(t, "overlay-class-refusals", got)
}
