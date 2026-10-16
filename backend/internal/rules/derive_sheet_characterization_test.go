package rules

import (
	"encoding/json"
	"fmt"
	"maps"
	"testing"

	"github.com/PuraFome/meuRPG/backend/internal/rules/srd51"
)

// The corpus of the sheet: Derive over builds of every class at two levels and
// over builds broken one way at a time, for the numbers the deriver computes
// (abilities, proficiencies, skills, armor class, hit points, spellcasting,
// attacks, resources and actions, the picks' effects) and the issues it reports
// with the table keys they are tied to. WildShapeDerived has its own corpus.

// sheetDump writes a Derived as one line of JSON, shortened so the golden stays
// readable: the features as a count (collecteffects.json has them), the
// proficiencies as one string each, the spells as their
// keys with a mark for the prepared ones, the standard actions as a count (the
// hand-written list, the same on every sheet), the abilities, saves and skills
// as one string each, the attacks without the names and parsed dice (the
// content's names and the Damage string again), and no zero field anywhere.
func sheetDump(d Derived) (string, error) {
	type compact struct {
		Derived
		Abilities, SavingThrows, Skills []string
		Proficiencies                   []string
		Features                        int
		Spells                          []string
		Attacks                         []any
		StandardActions                 int
	}
	out := compact{Derived: d, StandardActions: len(d.StandardActions)}
	for _, a := range d.Abilities {
		out.Abilities = append(out.Abilities, fmt.Sprintf("%s %d %+d %+d %+d = %d (%+d)", a.Ability, a.Base, a.RaceBonus, a.ManualBonus, a.Bonus, a.Score, a.Modifier))
	}
	for _, s := range d.SavingThrows {
		out.SavingThrows = append(out.SavingThrows, fmt.Sprintf("%s %t %+d", s.Ability, s.Proficient, s.Bonus))
	}
	for _, s := range d.Skills {
		out.Skills = append(out.Skills, fmt.Sprintf("%s %s %d %+d", s.Key, s.Ability, s.Proficiency, s.Bonus))
	}
	out.Features = len(d.Features)
	for _, p := range d.Proficiencies {
		out.Proficiencies = append(out.Proficiencies, p.Kind+" "+p.Key+" "+p.NamePT)
	}
	for _, s := range d.Spells {
		k := s.Spell.Key
		if s.Prepared {
			k += "*"
		}
		out.Spells = append(out.Spells, k)
	}
	for _, a := range d.Attacks {
		a.Name, a.NamePT, a.DamageTypeNamePT, a.DamageDice, a.VersatileDice = "", "", "", DiceFormula{}, DiceFormula{}
		out.Attacks = append(out.Attacks, a)
	}
	raw, err := json.Marshal(out)
	if err != nil {
		return "", err
	}
	var tree any
	if err := json.Unmarshal(raw, &tree); err != nil {
		return "", err
	}
	raw, err = json.Marshal(pruneZero(tree))
	return string(raw), err
}

// pruneZero drops the zero fields of decoded JSON objects (false, 0, "", null
// and empty lists or objects), at every depth.
func pruneZero(v any) any {
	switch t := v.(type) {
	case map[string]any:
		for k, x := range t {
			t[k] = pruneZero(x)
			switch y := t[k].(type) {
			case nil:
				delete(t, k)
			case bool:
				if !y {
					delete(t, k)
				}
			case float64:
				if y == 0 {
					delete(t, k)
				}
			case string:
				if y == "" {
					delete(t, k)
				}
			case []any:
				if len(y) == 0 {
					delete(t, k)
				}
			case map[string]any:
				if len(y) == 0 {
					delete(t, k)
				}
			}
		}
	case []any:
		for i := range t {
			t[i] = pruneZero(t[i])
		}
	}
	return v
}

// sheetKit is a class's sheet for the corpus: the subclass, the weapons, the
// armor and shield, the cantrips and the options it takes.
type sheetKit struct {
	sub      string
	weapons  []string
	armor    string
	shield   bool
	cantrips []string
	picks    []string
}

var sheetKits = map[string]sheetKit{
	"class:barbarian": {sub: "subclass:berserker", weapons: []string{"equipment:greataxe", "equipment:handaxe", "equipment:javelin"}},
	"class:bard":      {sub: "subclass:lore", weapons: []string{"equipment:rapier", "equipment:dagger"}, armor: "equipment:leather-armor", cantrips: []string{"spell:vicious-mockery", "spell:light"}},
	"class:cleric":    {sub: "subclass:life", weapons: []string{"equipment:club"}, armor: "equipment:chain-mail", shield: true, cantrips: []string{"spell:sacred-flame", "spell:light"}},
	"class:druid":     {sub: "subclass:land", weapons: []string{"equipment:quarterstaff"}, armor: "equipment:leather-armor", shield: true, cantrips: []string{"spell:produce-flame"}},
	"class:fighter":   {sub: "subclass:champion", weapons: []string{"equipment:longsword", "equipment:longbow"}, armor: "equipment:chain-mail", shield: true, picks: []string{"feature:fighter-fighting-style-dueling"}},
	"class:monk":      {sub: "subclass:open-hand", weapons: []string{"equipment:quarterstaff", "equipment:shortsword", "equipment:dagger"}},
	"class:paladin":   {sub: "subclass:devotion", weapons: []string{"equipment:longsword", "equipment:javelin"}, armor: "equipment:chain-mail", shield: true, picks: []string{"feature:fighting-style-defense"}},
	"class:ranger":    {sub: "subclass:hunter", weapons: []string{"equipment:longbow", "equipment:shortsword"}, armor: "equipment:leather-armor", picks: []string{"feature:ranger-fighting-style-archery"}},
	"class:rogue":     {sub: "subclass:thief", weapons: []string{"equipment:rapier", "equipment:shortsword", "equipment:longbow"}, armor: "equipment:leather-armor"},
	"class:sorcerer":  {sub: "subclass:draconic", weapons: []string{"equipment:dagger"}, cantrips: []string{"spell:fire-bolt", "spell:shocking-grasp"}, picks: []string{"feature:dragon-ancestor-red---fire-damage"}},
	"class:warlock":   {sub: "subclass:fiend", weapons: []string{"equipment:dagger"}, armor: "equipment:leather-armor", cantrips: []string{"spell:eldritch-blast"}},
	"class:wizard":    {sub: "subclass:evocation", weapons: []string{"equipment:quarterstaff"}, cantrips: []string{"spell:fire-bolt", "spell:chill-touch"}},
}

// kitBuild is the standard human of a class at a level with its kit.
func kitBuild(class string, level int) Build {
	k := sheetKits[class]
	b := standard(class, level)
	b.Classes[0].Subclass = k.sub
	b.Weapons, b.Armor, b.Shield = k.weapons, k.armor, k.shield
	b.Cantrips, b.FeatureChoices = k.cantrips, k.picks
	return b
}

// sheetCases are the SRD builds of the corpus.
func sheetCases() map[string]func() Build {
	with := func(base func() Build, edit func(b *Build)) func() Build {
		return func() Build {
			b := base()
			edit(&b)
			return b
		}
	}
	kit := func(class string, level int) func() Build { return func() Build { return kitBuild(class, level) } }
	multi := func(levels ...ClassLevel) func() Build {
		return func() Build {
			b := standard(levels[0].Class, levels[0].Level)
			b.Classes = levels
			return b
		}
	}
	halfElf := func(edit func(b *Build)) func() Build {
		return with(kit("class:bard", 1), func(b *Build) {
			b.Race = "race:half-elf"
			edit(b)
		})
	}
	pick := func(a Ability) string {
		return ScopedChoice(AbilityChoiceKey("race:half-elf"), abilityPrefix+string(a))
	}
	out := map[string]func() Build{
		"pensantus": pensantus,

		// abilities
		"scores missing or out of range": with(pensantus, func(b *Build) {
			b.BaseScores = map[Ability]int{DEX: 0, CON: 31, INT: 16, WIS: 13, CHA: 12}
			b.ExtraAbilityBonuses = map[Ability]int{INT: 15, WIS: -15}
		}),
		"half-elf without picks":              halfElf(func(*Build) {}),
		"half-elf with one pick":              halfElf(func(b *Build) { b.FeatureChoices = []string{pick(DEX)} }),
		"half-elf with two picks":             halfElf(func(b *Build) { b.FeatureChoices = []string{pick(DEX), pick(CON)} }),
		"half-elf with a manual increase":     halfElf(func(b *Build) { b.ExtraAbilityBonuses = map[Ability]int{DEX: 1} }),
		"half-elf with a manual decrease":     halfElf(func(b *Build) { b.ExtraAbilityBonuses = map[Ability]int{DEX: -1} }),
		"half-elf with picks off the list":    halfElf(func(b *Build) { b.FeatureChoices = []string{pick(CHA), pick(DEX), pick(DEX), pick(WIS), pick(STR)} }),
		"a score above 20 without an item":    with(kit("class:fighter", 1), func(b *Build) { b.BaseScores[STR] = 20 }),
		"a score above 20 with an item":       with(kit("class:fighter", 1), func(b *Build) { b.BaseScores[STR] = 20; b.ExtraAbilityBonuses = map[Ability]int{STR: 2} }),
		"primal champion over 20":             with(kit("class:barbarian", 20), func(b *Build) { b.BaseScores[STR] = 20 }),
		"a rock gnome with a +1 manual bonus": with(pensantus, func(b *Build) { b.ExtraAbilityBonuses = map[Ability]int{CHA: 1} }),

		// resolve
		"no race":                   with(pensantus, func(b *Build) { b.Race, b.Subrace = "", "" }),
		"unknown race":              with(pensantus, func(b *Build) { b.Race = "race:nope" }),
		"unknown subrace":           with(pensantus, func(b *Build) { b.Subrace = "subrace:nope" }),
		"subrace of another race":   with(pensantus, func(b *Build) { b.Subrace = "subrace:high-elf" }),
		"no classes":                with(pensantus, func(b *Build) { b.Classes = nil }),
		"unknown class":             with(pensantus, func(b *Build) { b.Classes = append(b.Classes, ClassLevel{Class: "class:nope", Level: 2}) }),
		"class twice":               with(pensantus, func(b *Build) { b.Classes = append(b.Classes, b.Classes[0]) }),
		"subclass of another class": with(pensantus, func(b *Build) { b.Classes[0].Subclass = "subclass:life" }),
		"unknown subclass":          with(pensantus, func(b *Build) { b.Classes[0].Subclass = "subclass:nope" }),
		"subclass too early":        with(pensantus, func(b *Build) { b.Classes[0].Level = 1 }),
		"custom subclass":           with(pensantus, func(b *Build) { b.Classes[0].Subclass, b.Classes[0].CustomSubclassName = "", "Cronurgia" }),
		"level 0 and level 25":      multi(ClassLevel{Class: "class:wizard", Level: 0}, ClassLevel{Class: "class:fighter", Level: 25}),
		"total above 20":            multi(ClassLevel{Class: "class:wizard", Level: 12}, ClassLevel{Class: "class:fighter", Level: 12}),

		// proficiencies and skills
		"fighter then wizard": multi(ClassLevel{Class: "class:fighter", Level: 1}, ClassLevel{Class: "class:wizard", Level: 1}),
		"wizard then fighter": multi(ClassLevel{Class: "class:wizard", Level: 1}, ClassLevel{Class: "class:fighter", Level: 1}),
		"custom background tools and an unknown skill": with(pensantus, func(b *Build) {
			b.CustomBackgroundProficiencies = []string{"proficiency:thieves-tools", "language:elvish", "language:nope"}
			b.CustomBackgroundSkills = []string{"skill:arcana", "skill:nope"}
		}),
		"unknown background with custom parts": with(pensantus, func(b *Build) { b.Background = "background:nope" }),
		"skills, expertise and their issues": with(kit("class:rogue", 6), func(b *Build) {
			b.SkillProficiencies = []string{"skill:stealth", "skill:nope", "skill:acrobatics", "skill:perception"}
			b.Expertise = []string{"skill:stealth", "skill:nope", "skill:athletics", "skill:perception"}
		}),
		"an elf's keen senses": with(kit("class:ranger", 2), func(b *Build) { b.Race, b.Subrace = "race:elf", "subrace:high-elf" }),
		"a half-orc's menace":  with(kit("class:barbarian", 6), func(b *Build) { b.Race = "race:half-orc" }),
		"a bard of 2 with jack of all trades": with(kit("class:bard", 2), func(b *Build) {
			b.SkillProficiencies = []string{"skill:performance"}
		}),
		"a lore bard of 3 with expertise": with(kit("class:bard", 3), func(b *Build) {
			b.SkillProficiencies = []string{"skill:performance", "skill:persuasion"}
			b.Expertise = []string{"skill:performance", "skill:persuasion"}
		}),

		// armor class
		"a wizard in plate with a shield": with(kit("class:wizard", 3), func(b *Build) { b.Armor, b.Shield = "equipment:plate-armor", true }),
		"a dwarf in plate, too weak": with(kit("class:cleric", 3), func(b *Build) {
			b.Race, b.Subrace, b.Armor = "race:dwarf", "subrace:hill-dwarf", "equipment:plate-armor"
			b.BaseScores[STR] = 8
		}),
		"a human in plate, too weak": with(kit("class:fighter", 3), func(b *Build) { b.Armor = "equipment:plate-armor"; b.BaseScores[STR] = 8 }),
		"scale mail with a high dexterity": with(kit("class:cleric", 3), func(b *Build) {
			b.Armor = "equipment:scale-mail"
			b.BaseScores[DEX] = 18
		}),
		"mage armor without armor":    with(pensantus, func(b *Build) { b.MageArmor = true }),
		"mage armor over leather":     with(pensantus, func(b *Build) { b.MageArmor, b.Armor = true, "equipment:leather-armor" }),
		"mage armor on a wise monk":   with(kit("class:monk", 3), func(b *Build) { b.MageArmor = true; b.BaseScores[WIS] = 18 }),
		"mage armor on a clumsy monk": with(kit("class:monk", 3), func(b *Build) { b.MageArmor = true; b.BaseScores[WIS] = 6 }),
		"barbarian then monk":         multi(ClassLevel{Class: "class:barbarian", Level: 2}, ClassLevel{Class: "class:monk", Level: 2}),
		"monk then barbarian":         multi(ClassLevel{Class: "class:monk", Level: 2}, ClassLevel{Class: "class:barbarian", Level: 2}),
		"unknown armor":               with(kit("class:fighter", 1), func(b *Build) { b.Armor = "equipment:nope" }),
		"a shield as armor":           with(kit("class:fighter", 1), func(b *Build) { b.Armor = "equipment:shield" }),
		"a weapon as armor":           with(kit("class:fighter", 1), func(b *Build) { b.Armor = "equipment:dagger" }),
		"draconic resilience":         with(kit("class:sorcerer", 3), func(b *Build) { b.BaseScores[DEX] = 18 }),

		// hit points
		"rolled hit points": with(kit("class:fighter", 4), func(b *Build) {
			b.HitPoints = HitPoints{Method: HitPointsRolled, Rolls: []int{10, 1, 7}}
		}),
		"rolls out of the die": with(kit("class:fighter", 4), func(b *Build) {
			b.HitPoints = HitPoints{Method: HitPointsRolled, Rolls: []int{0, 11, -3}}
		}),
		"too few rolls": with(kit("class:fighter", 4), func(b *Build) {
			b.HitPoints = HitPoints{Method: HitPointsRolled, Rolls: []int{6}}
		}),
		"too many rolls": with(kit("class:fighter", 2), func(b *Build) {
			b.HitPoints = HitPoints{Method: HitPointsRolled, Rolls: []int{6, 5, 4}}
		}),
		"rolls over three classes": func() Build {
			b := multi(ClassLevel{Class: "class:fighter", Level: 2}, ClassLevel{Class: "class:wizard", Level: 2}, ClassLevel{Class: "class:cleric", Level: 1})()
			b.HitPoints = HitPoints{Method: HitPointsRolled, Rolls: []int{9, 6, 7, 8}}
			return b
		},
		"a frail wizard": with(pensantus, func(b *Build) { b.BaseScores[CON] = 3; b.Classes[0].Level = 5 }),
		"a hill dwarf and a draconic sorcerer": with(kit("class:sorcerer", 5), func(b *Build) {
			b.Race, b.Subrace = "race:dwarf", "subrace:hill-dwarf"
		}),

		// attacks
		"a halfling with a maul": with(kit("class:fighter", 1), func(b *Build) {
			b.Race, b.Subrace = "race:halfling", "subrace:lightfoot"
			b.Weapons = []string{"equipment:maul", "equipment:longbow", "equipment:dagger"}
		}),
		"dueling with one, two and versatile weapons": with(kit("class:fighter", 1), func(b *Build) {
			b.Weapons = []string{"equipment:longsword", "equipment:dagger", "equipment:greataxe", "equipment:javelin", "equipment:net"}
		}),
		"unknown weapons": with(kit("class:fighter", 1), func(b *Build) {
			b.Weapons = []string{"equipment:nope", "equipment:plate-armor", "equipment:dagger"}
		}),
		"a monk in armor":       with(kit("class:monk", 5), func(b *Build) { b.Armor = "equipment:leather-armor" }),
		"a monk with a shield":  with(kit("class:monk", 5), func(b *Build) { b.Shield = true }),
		"a strong monk":         with(kit("class:monk", 5), func(b *Build) { b.BaseScores[STR], b.BaseScores[DEX] = 17, 10 }),
		"a weak unarmed wizard": with(pensantus, func(b *Build) { b.BaseScores[STR] = 3; b.Weapons = nil }),
		"cantrips nobody casts": with(kit("class:fighter", 1), func(b *Build) {
			b.Cantrips = []string{"spell:fire-bolt"}
		}),
		"cantrips of every shape": with(pensantus, func(b *Build) {
			b.Cantrips = []string{"spell:nope", "spell:light", "spell:fireball", "spell:fire-bolt", "spell:sacred-flame", "spell:shocking-grasp"}
		}),
		"a cleric and wizard casting each cantrip": func() Build {
			b := multi(ClassLevel{Class: "class:wizard", Level: 3}, ClassLevel{Class: "class:cleric", Level: 3})()
			b.Cantrips = []string{"spell:sacred-flame", "spell:fire-bolt", "spell:vicious-mockery"}
			return b
		},
		"eldritch blast at 17": kit("class:warlock", 17),

		// resources and actions
		"cleric 2 and paladin 3": multi(ClassLevel{Class: "class:cleric", Level: 2}, ClassLevel{Class: "class:paladin", Level: 3}),
		"paladin 3 and cleric 6": multi(ClassLevel{Class: "class:paladin", Level: 3}, ClassLevel{Class: "class:cleric", Level: 6}),
		"cleric 6 and paladin 3": multi(ClassLevel{Class: "class:cleric", Level: 6}, ClassLevel{Class: "class:paladin", Level: 3}),
		"a monk of 1":            kit("class:monk", 1),
		"a fighter of 15":        kit("class:fighter", 15),
		"a fighter of 2 and a monk of 2, two weapons": func() Build {
			b := multi(ClassLevel{Class: "class:fighter", Level: 2}, ClassLevel{Class: "class:monk", Level: 2})()
			b.FeatureChoices = []string{"feature:fighter-fighting-style-two-weapon-fighting"}
			b.Weapons = []string{"equipment:shortsword", "equipment:dagger"}
			return b
		},
		"a half-orc barbarian of 17": with(kit("class:barbarian", 17), func(b *Build) { b.Race = "race:half-orc" }),
		"a rogue of 11":              kit("class:rogue", 11),

		// what the picks give
		"a red dragonborn": with(kit("class:fighter", 5), func(b *Build) {
			b.Race, b.FeatureChoices = "race:dragonborn", []string{"trait:draconic-ancestry-red"}
		}),
		"a dragonborn with red twice and gold": with(kit("class:fighter", 17), func(b *Build) {
			b.Race, b.FeatureChoices = "race:dragonborn", []string{"trait:draconic-ancestry-red", "trait:draconic-ancestry-red", "trait:draconic-ancestry-gold", "trait:draconic-ancestry-blue"}
		}),
		"a dragonborn without an ancestry": with(kit("class:fighter", 1), func(b *Build) { b.Race = "race:dragonborn" }),
		"an ancestry on a human":           with(kit("class:fighter", 1), func(b *Build) { b.FeatureChoices = []string{"trait:draconic-ancestry-red"} }),
		"a tiefling":                       with(kit("class:warlock", 3), func(b *Build) { b.Race = "race:tiefling" }),
		"a draconic sorcerer of 6": with(kit("class:sorcerer", 6), func(b *Build) {
			b.FeatureChoices = []string{"feature:dragon-ancestor-red---fire-damage", "feature:dragon-ancestor-gold---fire-damage", "trait:draconic-ancestry-red"}
		}),
		"a dragonborn draconic sorcerer of 6": with(kit("class:sorcerer", 6), func(b *Build) {
			b.Race = "race:dragonborn"
			b.FeatureChoices = []string{"trait:draconic-ancestry-blue", "feature:dragon-ancestor-blue---lightning-damage"}
		}),

		// spellcasting
		"a paladin of 1":                   kit("class:paladin", 1),
		"three casters and a pact":         multi(ClassLevel{Class: "class:wizard", Level: 3}, ClassLevel{Class: "class:cleric", Level: 2}, ClassLevel{Class: "class:paladin", Level: 2}, ClassLevel{Class: "class:warlock", Level: 3}),
		"a ranger of 2 and a paladin of 2": multi(ClassLevel{Class: "class:ranger", Level: 2}, ClassLevel{Class: "class:paladin", Level: 2}),
	}
	for class := range sheetKits {
		out[class[len("class:"):]+" at 5"] = kit(class, 5)
	}
	return out
}

// TestDeriveSheetCharacterization freezes the whole sheet of every build of the
// corpus, over the SRD, over contents the SRD never has (no choice data, a
// caster row without casting columns) and over a table content whose entries
// have revisions, so the issues name the table keys they depend on.
func TestDeriveSheetCharacterization(t *testing.T) {
	t.Parallel()
	srd := loadForTest(t)
	got := map[string]string{}
	put := func(name string, d Derived) {
		t.Helper()
		text, err := sheetDump(d)
		if err != nil {
			t.Fatal(err)
		}
		got[name] = text
	}
	for name, build := range sheetCases() {
		put("srd: "+name, Derive(build(), srd))
	}

	// A content without the choice data gives no breath weapon and no resistance.
	noChoices := *srd.c
	noChoices.choices = nil
	put("no choice data: a red dragonborn", derive(sheetCases()["a red dragonborn"](), &noChoices))

	// A caster whose class row has no casting columns: no numbers from the row,
	// and no slots for a single caster.
	bare := *srd.c
	bare.classLevels = maps.Clone(srd.c.classLevels)
	rows := make([]*srd51.Level, len(srd.c.classLevels["class:wizard"]))
	for i, r := range srd.c.classLevels["class:wizard"] {
		row := *r
		row.Spellcasting = nil
		rows[i] = &row
	}
	bare.classLevels["class:wizard"] = rows
	put("wizard rows without casting: pensantus", derive(pensantus(), &bare))
	put("wizard rows without casting: wizard and cleric", derive(sheetCases()["a cleric and wizard casting each cantrip"](), &bare))

	for name, d := range tableSheets(t, srd) {
		put("table: "+name, d)
	}
	goldenText(t, "derivesheets", got)
}

// tableSheets derives sheets of a table content with revisions: a class whose
// features carry formulas that fail when the sheet is made and actions of table
// shapes, and builds broken one way at a time so that each issue code names its
// table keys.
func tableSheets(t *testing.T, srd *Content) map[string]Derived {
	t.Helper()
	o := stamped(t, srd)
	o.Spells = tableTestSpells()
	race, sub, bg := tableMisc()
	race.Revision, sub.Revision, bg.Revision = 1, 1, 1
	// A table trait that offers an SRD option: no feature, so never blamed.
	race.Traits[0].Effects = append(race.Traits[0].Effects, Effect{Type: "choice", Choice: "feature", Count: 1, From: []string{"feature:fighter-fighting-style-dueling"}})
	o.Races, o.Subraces, o.Backgrounds = []TableRace{race}, []TableSubrace{sub}, []TableBackground{bg}
	const none = "class:gen-none@mesa"
	const noneSub = "subclass:gen-none-a@mesa"
	const prepared = "class:gen-full-prepared@mesa"
	broken := "1 / (level() - level())"
	classOf(&o, none).Levels[0].Features = append(classOf(&o, none).Levels[0].Features,
		tf("formula-quebrada", "Fórmula quebrada", Effect{Type: "resource", Resource: "quebrado", Max: broken, Recharge: RechargeLongRest}),
		tf("recarga-quebrada", "Recarga quebrada", Effect{Type: "resource", Resource: "recarga", Max: "2", Recharge: RechargeLongRest, RechargeIf: "floor(" + broken + ") > 0", RechargeThen: RechargeShortRest}),
		tf("recarga-curta", "Recarga curta", Effect{Type: "resource", Resource: "curta", Max: "level()", Recharge: RechargeLongRest, RechargeIf: "level() >= 2", RechargeThen: RechargeShortRest}),
		tf("sem-usos", "Sem usos", Effect{Type: "resource", Resource: "nenhum", Max: "0", Recharge: RechargeLongRest}),
		tf("fuga-agil", "Fuga ágil", Effect{Type: "grant_action", Economy: EconomyBonusAction, TextPT: "Disparar ou Desengajar como ação bônus."}),
		tf("passo-escondido", "Passo escondido", Effect{Type: "grant_action", Economy: EconomyBonusAction, TextPT: "Esconder-se como ação bônus."}),
		tf("golpe-comum", "Golpe comum", Effect{Type: "grant_action", Economy: EconomyAction, TextPT: "Disparar."}),
		tf("gasta-o-que-nao-tem", "Gasta o que não tem", Effect{Type: "grant_action", Economy: EconomyAction, Resource: "nenhum"}),
		tf("gasta-curta", "Gasta curta", Effect{Type: "grant_action", Economy: EconomyReaction, Resource: "curta"}),
	)
	classOf(&o, none).Levels[1].Features = append(classOf(&o, none).Levels[1].Features,
		tf("pele-grossa", "Pele grossa", Effect{Type: "modifier", Target: "ac.base", Mode: "add", Value: "1"}),
		tf("pele-quebrada", "Pele quebrada", Effect{Type: "modifier", Target: "ac.base", Mode: "set", Value: broken}),
	)
	// A table subclass that offers an SRD fighting style: a pick of another style
	// of the same set is blamed on it.
	styles := subclassOf2(&o, noneSub)
	styles.Levels[0].Features = append(styles.Levels[0].Features,
		tf("estilo-da-mesa", "Estilo da mesa", Effect{Type: "choice", Choice: "feature", Count: 1, From: []string{"feature:fighter-fighting-style-dueling"}}))
	classOf(&o, prepared).Casting.PreparedMax = broken
	c := withOverlayOn(t, srd, o)

	base := func(class string, level int) Build {
		b := Build{
			BaseScores: map[Ability]int{STR: 10, DEX: 14, CON: 14, INT: 12, WIS: 16, CHA: 8},
			Race:       race.Key, Subrace: sub.Key, Background: bg.Key,
			Classes:             []ClassLevel{{Class: class, Level: level}},
			ExtraAbilityBonuses: map[Ability]int{STR: 2, DEX: 1},
		}
		return b
	}
	with := func(b Build, edit func(b *Build)) Build {
		edit(&b)
		return b
	}
	out := map[string]Derived{}
	for name, b := range map[string]Build{
		"the class of broken formulas": base(none, 2),
		"the class at level 1":         base(none, 1),
		"a prepared caster with a broken limit": with(base(prepared, 3), func(b *Build) {
			b.SpellsPrepared = []string{"spell:bless", "spell:cure-wounds"}
		}),
		"a subclass too early": with(base(none, 2), func(b *Build) { b.Classes[0].Subclass = noneSub }),
		"expertise without the skill": with(base(none, 3), func(b *Build) {
			b.Classes[0].Subclass = noneSub
			b.Expertise = []string{"skill:stealth"}
		}),
		"plate without proficiency": with(base(none, 3), func(b *Build) {
			b.Classes[0].Subclass = noneSub
			b.Armor, b.Shield = "equipment:plate-armor", true
		}),
		"the race bonuses not placed": with(base(none, 1), func(b *Build) { b.ExtraAbilityBonuses = nil }),
		"rolls out of the die": with(base(none, 3), func(b *Build) {
			b.Classes[0].Subclass = noneSub
			b.HitPoints = HitPoints{Method: HitPointsRolled, Rolls: []int{9}}
		}),
		"total above 20": with(base(none, 20), func(b *Build) {
			b.Classes[0].Subclass = noneSub
			b.Classes = append(b.Classes, ClassLevel{Class: "class:fighter", Level: 2})
		}),
		"too many known spells": with(base(prepared, 3), func(b *Build) {
			b.Cantrips = []string{"spell:light", "spell:sacred-flame", "spell:thaumaturgy", "spell:guidance", "spell:spare-the-dying", "spell:fire-bolt"}
			b.SpellsPrepared = []string{"spell:fireball", "spell:cone-de-teste@mesa"}
		}),
		"an unknown armor and weapon": with(base(none, 1), func(b *Build) {
			b.Armor, b.Weapons = "equipment:nope", []string{"equipment:nope"}
		}),
		"an SRD style the table subclass does not offer": with(base(none, 3), func(b *Build) {
			b.Classes[0].Subclass = noneSub
			b.FeatureChoices = []string{"feature:fighter-fighting-style-defense"}
		}),
		"an SRD class beside a table subclass": with(base(none, 3), func(b *Build) {
			b.Classes[0].Subclass = noneSub
			b.Classes = append(b.Classes, ClassLevel{Class: "class:wizard", Subclass: "subclass:evocation", Level: 1})
		}),
	} {
		out[name] = Derive(b, c)
	}
	// A maneuver option that also grants an action lists the die, not the action.
	both := postureOption(maneuverHeron, "Golpe da Garça",
		Effect{Type: "superiority_die", Applies: ManeuverDamage, Resource: maneuverDice, Value: "8"},
		Effect{Type: "grant_action", Economy: EconomyBonusAction, Resource: maneuverDice})
	mc := withOverlayOn(t, srd, maneuverOverlay(both, postureOption(maneuverPlain, "Golpe Simples", Effect{Type: "grant_action", Economy: EconomyBonusAction, Resource: maneuverDice})))
	out["a maneuver with an action"] = Derive(maneuverBuild(3, maneuverHeron, maneuverPlain), mc)

	// A third caster whose subclass table has no row at its level casts nothing.
	knight := "subclass:cavaleiro-runico@mesa"
	gaps := *c.c
	gaps.subclassLevels = maps.Clone(c.c.subclassLevels)
	gaps.subclassLevels[knight] = maps.Clone(c.c.subclassLevels[knight])
	delete(gaps.subclassLevels[knight], 4)
	knightBuild := sweepBase(t, c, "class:fighter", knight)
	knightBuild.Classes = []ClassLevel{{Class: "class:fighter", Subclass: knight, Level: 4}}
	out["a third caster without a row"] = derive(knightBuild, &gaps)
	out["a third caster with its row"] = Derive(knightBuild, c)
	return out
}

// wildDump writes what WildShapeDerived sets on the sheet as one line of JSON;
// the rest is the character's own sheet, copied.
func wildDump(d Derived) (string, error) {
	out := struct {
		ArmorClass, HitPointsMax, HitPointsFromEffects, AttacksPerAction  int
		ArmorClassDescription                                             string
		HitDice                                                           []HitDice
		SpeedWalkFt, SpeedFlyFt, SpeedSwimFt, SpeedClimbFt, SpeedBurrowFt int
		Hover                                                             bool
		Attacks                                                           []Attack
		SaveActions                                                       []SaveAction
		Abilities                                                         []AbilityScore
		SavingThrows                                                      []SavingThrow
		Skills                                                            []Skill
		Passives                                                          [3]int
		Initiative                                                        int
		Senses                                                            []Sense
		Spellcasting                                                      []Spellcasting
		Spells, Features                                                  int
		PactMagic                                                         *PactMagic
		LastFeature                                                       string
		LastHint                                                          Hint
	}{
		d.ArmorClass, d.HitPointsMax, d.HitPointsFromEffects, d.AttacksPerAction, d.ArmorClassDescription, d.HitDice,
		d.SpeedWalkFt, d.SpeedFlyFt, d.SpeedSwimFt, d.SpeedClimbFt, d.SpeedBurrowFt, d.Hover,
		d.Attacks, d.SaveActions, d.Abilities, d.SavingThrows, d.Skills,
		[3]int{d.PassivePerception, d.PassiveInvestigation, d.PassiveInsight},
		d.Initiative, d.Senses,
		d.Spellcasting, len(d.Spells), len(d.Features), d.PactMagic, d.Features[len(d.Features)-1].Key, d.Hints[len(d.Hints)-1],
	}
	raw, err := json.Marshal(out)
	return string(raw), err
}

// beastWithSaves is the giant spider of the SRD with saving throws listed, which
// no SRD beast has, and a shorter darkvision: the content it is added to.
func beastWithSaves(srd *Content) *Content {
	cc := *srd.c
	cc.monsters = maps.Clone(srd.c.monsters)
	m := *srd.c.monsters["monster:giant-spider"]
	m.Key = "monster:test-spider"
	m.Saves = map[string]int{"dex": 5, "wis": 1}
	m.Darkvision = 10
	cc.monsters[m.Key] = &m
	cc.namesPT = maps.Clone(srd.c.namesPT)
	cc.namesPT[m.Key] = "Aranha de teste"
	return &Content{c: &cc}
}

// TestWildShapeDerivedCharacterization freezes the beast-form sheet of druids of
// several levels in several beasts: the scores, saves and skills the beast lends,
// the senses, the spells kept or dropped, and the refusals.
func TestWildShapeDerivedCharacterization(t *testing.T) {
	t.Parallel()
	srd := loadForTest(t)
	c := beastWithSaves(srd)
	got := map[string]string{}
	expert := salvia(8)
	expert.SkillProficiencies = []string{"skill:stealth", "skill:perception"}
	sheets := map[string]Derived{
		"salvia 2":                    Derive(salvia(2), srd),
		"salvia 8 in stealth":         Derive(expert, srd),
		"salvia 18":                   Derive(salvia(18), srd),
		"a human druid without sight": Derive(standard("class:druid", 4), srd),
	}
	for sheet, d := range sheets {
		for _, beast := range []string{"monster:wolf", "monster:badger", "monster:giant-owl", "monster:giant-spider", "monster:test-spider", "monster:goblin", "monster:nope"} {
			w, err := c.WildShapeDerived(d, beast)
			text := errText(err)
			if err == nil {
				var derr error
				text, derr = wildDump(w)
				if derr != nil {
					t.Fatal(derr)
				}
			}
			got[sheet+" as "+beast] = text
		}
	}
	goldenText(t, "wildshapederived", got)
}
