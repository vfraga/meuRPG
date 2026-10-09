package rules

import (
	"fmt"
	"slices"
	"strings"

	"github.com/PuraFome/meuRPG/backend/internal/rules/formula"
	"github.com/PuraFome/meuRPG/backend/internal/rules/srd51"
)

// deriver holds the work of one Derive call. It is used once and thrown
// away; nothing in it outlives the call, and nothing is shared between
// calls except the read-only content.
type deriver struct {
	b Build
	c *content
	d *Derived

	// classes are the Build's classes that exist in the content, in Build
	// order; the first is the starting class.
	classes []ownedClass
	race    *srd51.Race
	subrace *srd51.Subrace

	scores map[Ability]int
	mods   map[Ability]int
	prof   int

	// armor is the worn body armor, or nil; armorCategory is "none" when
	// there is none.
	armor         *srd51.Armor
	armorCategory string

	// items are the equipped inventory lines, resolved; armorItem and shieldItem
	// are the worn armor and the shield among them, and itemSpellAttack and
	// itemSpellDC what the items add to the spell attack and the save DC.
	items                        []*itemRuntime
	armorItem, shieldItem        *itemRuntime
	itemSpellAttack, itemSpellDC int

	// active are the effects of everything the character has, in a stable
	// order.
	active []activeEffect
	// proficient holds every proficiency key the character has, such as
	// "proficiency:light-armor".
	proficient map[string]bool
	// skillLevel is each skill's proficiency, and automaticSkills the
	// skills the race, background or effects gave (not the player's
	// choice). Both are set by skills().
	skillLevel      map[string]ProficiencyLevel
	automaticSkills map[string]bool

	env *formula.Env
	// conditions caches each effect's `when` result: the character does
	// not change during one Derive, and a broken condition must report one
	// Issue, not one per use.
	conditions map[*Effect]bool
}

type ownedClass struct {
	key      string
	class    *srd51.Class
	level    int
	subclass *srd51.Subclass
	// customSubclass is the typed name of a subclass outside the content.
	customSubclass string
	index          int // position in Build.Classes, for Issue fields
}

type activeEffect struct {
	owner  string // the feature, trait or background key
	effect *Effect
}

func derive(b Build, c *content) Derived {
	d := &Derived{ContentVersion: c.version}
	x := &deriver{b: b, c: c, d: d, proficient: map[string]bool{}, conditions: map[*Effect]bool{}}

	x.resolve()
	x.resolveItems()
	x.resolveArmor()
	x.adjustItemArmor()
	x.names()
	x.abilities()
	x.levelAndProficiency()
	x.collectEffects()
	x.addItemEffects()
	x.buildEnv()
	x.abilityEffects()
	x.proficiencies()
	x.savingThrows()
	x.skills()
	x.armorClass()
	x.hitPoints()
	x.speedAndSenses()
	x.languages()
	x.spellcasting()
	x.attacks()
	x.resourcesAndActions()
	x.effectHints()
	x.checkChoices()
	// Only an issue tied to a table entry is ever blamed on a change.
	for i := range d.Issues {
		if len(d.Issues[i].Keys) == 0 {
			d.Issues[i].ChangeMessage, d.Issues[i].ChangeSubject = "", ""
		}
	}
	return *d
}

// issue records a problem on the sheet.
func (x *deriver) issue(code, field, format string, args ...any) {
	x.d.Issues = append(x.d.Issues, Issue{Code: code, Field: field, Message: fmt.Sprintf(format, args...), Keys: x.issueKeys(code, field)})
}

// issueChange records an issue and, when it is tied to a table entry, the
// sentence "A classe mudou" tells it with (Issue.ChangeMessage).
func (x *deriver) issueChange(code, field, subject, change, message string) {
	x.d.Issues = append(x.d.Issues, Issue{Code: code, Field: field, Message: message, Keys: x.issueKeys(code, field), ChangeMessage: change, ChangeSubject: subject})
}

// countPT writes a count with its noun in Portuguese: "1 perícia", "2 perícias".
func countPT(n int, one, many string) string {
	if n == 1 {
		return "1 " + one
	}
	return fmt.Sprintf("%d %s", n, many)
}

// tieToOfferers ties the last issue (an SRD option the sheet no longer has the
// feature for) to the table entries whose features offer options of the same SRD
// set: the entry where that option set comes from. An entry that has nothing to do
// with the option is never blamed (a table subclass of another class), and the
// returned key is the first of them, for the sentence.
func (x *deriver) tieToOfferers(parent string) string {
	if len(x.c.entryRevision) == 0 || parent == "" {
		return ""
	}
	last := &x.d.Issues[len(x.d.Issues)-1]
	first := ""
	for _, a := range x.active {
		e := a.effect
		if e.Type != "choice" || e.Choice != "feature" || !isTableKey(a.owner) {
			continue
		}
		f := x.c.features[a.owner]
		if f == nil {
			continue
		}
		entry := f.Class
		if f.Subclass != "" {
			entry = f.Subclass
		}
		if !isTableKey(entry) {
			continue
		}
		for _, from := range e.From {
			if of := x.c.features[from]; of != nil && x.optionParent(from, of.Parent) == parent {
				if !slices.Contains(last.Keys, entry) {
					last.Keys = append(last.Keys, entry)
				}
				if first == "" {
					first = entry
				}
				break
			}
		}
	}
	slices.Sort(last.Keys)
	return first
}

// issueKeys are the table keys an issue depends on (Issue.Keys): the key at the
// field it points at, and, by what the code is about, the keys whose content the
// problem is computed from. A formula that failed at run time may come from any
// feature of the build, so it depends on all the table keys it has.
func (x *deriver) issueKeys(code, field string) []string {
	if len(x.c.entryRevision) == 0 {
		return nil // the SRD content or a table with nothing: no key to blame
	}
	all := BuildKeys(x.b)
	seen := map[string]bool{}
	var out []string
	add := func(key string) {
		if isTableKey(key) && !seen[key] {
			seen[key] = true
			out = append(out, key)
		}
	}
	for _, kf := range all {
		if kf.Field == field {
			add(kf.Key)
		}
	}
	// groups: which fields of the sheet the numbers behind a code come from.
	var classes, race, background, spells bool
	switch code {
	case IssueSkillCount, IssueExpertise, IssueMulticlass, IssueSubclassLevel, IssueArmorProficiency:
		classes, race, background = true, true, true
	case IssueSpellCount, IssueSpellLevel, IssueSpellNotOnList:
		classes = true
	case IssueRaceBonus:
		race = true
	case IssueHitPointRolls, IssueLevel:
		classes = true
	case IssueFormula:
		classes, race, background, spells = true, true, true, true
	}
	for _, kf := range all {
		switch {
		case classes && (strings.Contains(kf.Field, ".classes[") || kf.Field == "full.classes"):
			add(kf.Key)
		case race && (kf.Field == "full.race_key" || kf.Field == "full.subrace_key"):
			add(kf.Key)
		case background && kf.Field == "full.background_key":
			add(kf.Key)
		case spells && (strings.Contains(kf.Field, "_spell_keys[") || strings.Contains(kf.Field, "cantrip_keys[")):
			add(kf.Key)
		}
	}
	slices.Sort(out)
	return out
}

// resolve looks up the race, subrace and classes, turning unknown keys into
// Issues so the rest of Derive can rely on what it finds.
func (x *deriver) resolve() {
	c := x.c
	switch r, ok := c.races[x.b.Race]; {
	case x.b.Race == "":
		x.issue(IssueMissing, "full.race_key", "Escolha uma raça.")
	case !ok:
		x.issue(IssueUnknownKey, "full.race_key", "A raça escolhida não existe no conteúdo %s.", c.version)
	default:
		x.race = r
	}
	if x.b.Subrace != "" {
		s, ok := c.subraces[x.b.Subrace]
		switch {
		case !ok:
			x.issue(IssueUnknownKey, "full.subrace_key", "A sub-raça escolhida não existe no conteúdo %s.", c.version)
		case x.race == nil || s.Race != x.race.Key:
			x.issue(IssueUnknownKey, "full.subrace_key", "A sub-raça não é da raça escolhida.")
		default:
			x.subrace = s
		}
	}
	if len(x.b.Classes) == 0 {
		x.issue(IssueMissing, "full.classes", "Escolha uma classe.")
	}
	seen := map[string]bool{}
	for i, cl := range x.b.Classes {
		field := fmt.Sprintf("full.classes[%d]", i)
		class, ok := c.classes[cl.Class]
		if !ok {
			x.issue(IssueUnknownKey, field+".class_key", "A classe escolhida não existe no conteúdo %s.", c.version)
			continue
		}
		if seen[cl.Class] {
			x.issue(IssueUnknownKey, field+".class_key", "A classe %s aparece duas vezes.", c.namePT(cl.Class))
			continue
		}
		seen[cl.Class] = true
		level := min(max(cl.Level, 1), MaxLevel)
		oc := ownedClass{key: cl.Class, class: class, level: level, index: i, customSubclass: cl.CustomSubclassName}
		if cl.Subclass != "" {
			sub, ok := c.subclasses[cl.Subclass]
			switch {
			case !ok || sub.Class != cl.Class:
				x.issue(IssueUnknownKey, field+".subclass_key", "A subclasse escolhida não é de %s.", c.namePT(cl.Class))
			case level < class.SubclassLevel:
				x.issueChange(IssueSubclassLevel, field+".subclass_key", cl.Class,
					fmt.Sprintf("agora escolhe a subclasse no nível %d; esta ficha tem nível %d nela.", class.SubclassLevel, level),
					fmt.Sprintf("%s escolhe a subclasse no nível %d.", c.namePT(cl.Class), class.SubclassLevel))
				oc.subclass = sub
			default:
				oc.subclass = sub
			}
		}
		x.classes = append(x.classes, oc)
	}
}

// levelAndProficiency sets the total level and the proficiency bonus.
func (x *deriver) levelAndProficiency() {
	total := 0
	for _, oc := range x.classes {
		total += oc.level
	}
	if total > MaxLevel {
		x.issue(IssueLevel, "full.classes", "O nível total passa de %d; os cálculos usam %d.", MaxLevel, MaxLevel)
		total = MaxLevel
	}
	x.d.TotalLevel = total
	x.d.NextLevelXP, _ = x.c.nextLevelXP(total) // 0 at level 20
	// The proficiency bonus follows the total character level: +2 at levels
	// 1-4, +3 at 5-8, and so on (the same column in every class table).
	x.prof = 2 + (max(total, 1)-1)/4
	if len(x.classes) > 0 {
		if row := x.c.classLevels[x.classes[0].key][max(total, 1)-1]; row.ProfBonus > 0 {
			x.prof = row.ProfBonus
		}
	}
	x.d.ProficiencyBonus = x.prof
}

// collectEffects gathers everything the character has (race and subrace
// traits, background, class and subclass features up to each class level,
// and the chosen options), in a stable order. It fills Derived.Features
// on the way.
func (x *deriver) collectEffects() {
	c := x.c
	owned := map[string]bool{}
	add := func(key string) {
		if owned[key] {
			return
		}
		owned[key] = true
		for _, e := range c.effects[key] {
			x.active = append(x.active, activeEffect{owner: key, effect: e})
		}
	}
	feature := func(key, name, source string, level int, desc []string) {
		x.d.Features = append(x.d.Features, Feature{
			Key: key, Name: name, NamePT: c.namePT(key), Source: source, Level: level,
			SourcePT: x.sourcePT(source, level), Description: desc,
		})
	}

	if x.race != nil {
		add(x.race.Key)
		for _, t := range x.race.Traits {
			if tr, ok := c.traits[t]; ok {
				add(t)
				feature(t, tr.Name, x.race.Key, 0, tr.Desc)
			}
		}
	}
	if x.subrace != nil {
		add(x.subrace.Key)
		for _, t := range x.subrace.Traits {
			if tr, ok := c.traits[t]; ok {
				add(t)
				feature(t, tr.Name, x.subrace.Key, 0, tr.Desc)
			}
		}
	}
	if bg, ok := c.backgrounds[x.b.Background]; ok {
		add(bg.Key)
		add(bg.Feature.Key)
		feature(bg.Feature.Key, bg.Feature.Name, bg.Key, 0, bg.Feature.Desc)
	} else if x.b.Background != "" {
		x.issue(IssueUnknownKey, "full.background_key", "O antecedente escolhido não existe no conteúdo %s.", c.version)
	} else if x.customBackground() {
		// The player's own background (SRD 5.1 "Customizing a Background"): its
		// feature is the player's text, like a table background's text feature.
		if x.b.CustomBackgroundFeatureName != "" || x.b.CustomBackgroundFeature != "" {
			var desc []string
			if x.b.CustomBackgroundFeature != "" {
				desc = []string{x.b.CustomBackgroundFeature}
			}
			x.d.Features = append(x.d.Features, Feature{
				Key: CustomBackgroundFeatureKey, Name: x.b.CustomBackgroundFeatureName, NamePT: x.b.CustomBackgroundFeatureName,
				Source: CustomBackgroundKey, SourcePT: x.b.CustomBackgroundName, Description: desc,
			})
		}
	} else if x.b.Background == "" {
		x.issue(IssueMissing, "full.background_key", "Escolha um antecedente.")
	}

	for _, oc := range x.classes {
		add(oc.key)
		if oc.subclass != nil {
			add(oc.subclass.Key)
		}
		for lvl := 1; lvl <= oc.level; lvl++ {
			for _, fk := range c.classLevels[oc.key][lvl-1].Features {
				if f, ok := c.features[fk]; ok {
					add(fk)
					feature(fk, f.Name, oc.key, lvl, f.Desc)
				}
			}
			if oc.subclass == nil {
				continue
			}
			row := c.subclassLevels[oc.subclass.Key][lvl]
			if row == nil {
				continue
			}
			for _, fk := range row.Features {
				if f, ok := c.features[fk]; ok {
					add(fk)
					feature(fk, f.Name, oc.subclass.Key, lvl, f.Desc)
				}
			}
		}
	}

	// Chosen options count only while their parent feature or trait is
	// owned, so a fighting style disappears with the fighter levels.
	pickedNames := map[string]bool{}
	for i, key := range x.b.FeatureChoices {
		field := fmt.Sprintf("full.feature_choice_keys[%d]", i)
		switch {
		case strings.HasPrefix(key, "feature:"):
			f, ok := c.features[key]
			if !ok {
				x.issue(IssueUnknownKey, field, "A opção escolhida não existe no conteúdo %s.", c.version)
				continue
			}
			parent := x.optionParent(key, f.Parent)
			if parent == "" || !owned[parent] {
				// A table feature may offer an SRD option (a fighting style).
				parent = x.offeredParent(key, owned)
			}
			if parent == "" || !owned[parent] {
				x.issueChange(IssueUnknownKey, field, "", fmt.Sprintf("a escolha %s não vale mais: nada na ficha oferece essa opção.", c.namePT(key)), fmt.Sprintf("%s não vale para este personagem.", c.namePT(key)))
				if entry := x.tieToOfferers(x.optionParent(key, f.Parent)); entry != "" {
					last := &x.d.Issues[len(x.d.Issues)-1]
					last.ChangeSubject = entry
					last.ChangeMessage = fmt.Sprintf("agora não oferece a escolha %s; ela não vale mais nesta ficha.", c.namePT(key))
				}
				continue
			}
			// An option can be taken once: the same style from a second class
			// (Defense from the fighter and the paladin) is not a second one.
			if pickedNames[f.Name] {
				x.issue(IssueChoiceCount, field, "%s já foi escolhida: ela vale uma vez só.", c.namePT(key))
				continue
			}
			pickedNames[f.Name] = true
			add(key)
			feature(key, f.Name, parent, x.featureLevel(parent), f.Desc)
		case strings.HasPrefix(key, "trait:"):
			t, ok := c.traits[key]
			if !ok || t.Parent == "" || !owned[t.Parent] {
				x.issue(IssueUnknownKey, field, "A opção escolhida não vale para este personagem.")
				continue
			}
			add(key)
			feature(key, t.Name, t.Parent, 0, t.Desc)
		default:
			x.issue(IssueUnknownKey, field, "A opção escolhida não existe no conteúdo %s.", c.version)
		}
	}
}

// sourcePT says where a feature comes from, in Portuguese: "Mago 1",
// "Evocação (Mago 2)", "Gnomo".
func (x *deriver) sourcePT(source string, level int) string {
	c := x.c
	switch {
	case strings.HasPrefix(source, "class:"):
		return fmt.Sprintf("%s %d", c.namePT(source), level)
	case strings.HasPrefix(source, "subclass:"):
		if sub, ok := c.subclasses[source]; ok {
			return fmt.Sprintf("%s (%s %d)", c.namePT(source), c.namePT(sub.Class), level)
		}
	case strings.HasPrefix(source, "feature:"), strings.HasPrefix(source, "trait:"):
		// An option: say where its parent comes from.
		for _, f := range x.d.Features {
			if f.Key == source {
				return f.SourcePT
			}
		}
	}
	return c.namePT(source)
}

// featureLevel is the level at which the character got an owned feature,
// or 0 for traits.
func (x *deriver) featureLevel(key string) int {
	for _, f := range x.d.Features {
		if f.Key == key {
			return f.Level
		}
	}
	return 0
}

// names fills the display names of the race, background and classes.
func (x *deriver) names() {
	c := x.c
	if x.race != nil {
		x.d.RaceNamePT = c.namePT(x.race.Key)
	}
	if x.subrace != nil {
		x.d.SubraceNamePT = c.namePT(x.subrace.Key)
	}
	if _, ok := c.backgrounds[x.b.Background]; ok {
		x.d.BackgroundNamePT = c.namePT(x.b.Background)
		x.d.BackgroundEquipmentPT = c.bgEquipment[x.b.Background]
	} else if x.b.Background == "" {
		x.d.BackgroundNamePT = x.b.CustomBackgroundName
		x.d.BackgroundEquipmentPT = x.b.CustomBackgroundEquipment
	}
	for _, oc := range x.classes {
		dc := DerivedClass{ClassKey: oc.key, NamePT: c.namePT(oc.key), Level: oc.level, SubclassNamePT: oc.customSubclass}
		if oc.subclass != nil {
			dc.SubclassNamePT = c.namePT(oc.subclass.Key)
		}
		x.d.Classes = append(x.d.Classes, dc)
	}
}

// optionParent finds the feature that offers option key. 5e-database marks
// most options with a parent; a few (such as metamagic-twinned-spell) are
// only listed in their parent's options.
func (x *deriver) optionParent(key, parent string) string {
	if parent != "" {
		return parent
	}
	return x.c.optionParents[key]
}

// offeredParent finds the table feature the character owns that offers option
// key, or "".
func (x *deriver) offeredParent(key string, owned map[string]bool) string {
	for _, p := range x.c.offeredBy[key] {
		if owned[p] {
			return p
		}
	}
	return ""
}

// buildEnv gives formulas their view of the character.
func (x *deriver) buildEnv() {
	levels := map[string]int{}
	for _, oc := range x.classes {
		levels[strings.TrimPrefix(oc.key, "class:")] = oc.level
	}
	x.env = &formula.Env{
		Level:      func() int { return x.d.TotalLevel },
		ClassLevel: func(class string) int { return levels[class] },
		Mod:        func(a string) int { return x.mods[Ability(a)] },
		Score:      func(a string) int { return x.scores[Ability(a)] },
		Prof:       func() int { return x.prof },
		Armor:      func() string { return x.armorCategory },
		Shield:     func() bool { return x.b.Shield },
	}
}

// applies says whether an active effect's condition holds. A broken
// condition becomes an Issue and the effect is skipped.
func (x *deriver) applies(a activeEffect) bool {
	if a.effect.when == nil {
		return true
	}
	if ok, cached := x.conditions[a.effect]; cached {
		return ok
	}
	ok, err := a.effect.when.Bool(x.env)
	if err != nil {
		x.issue(IssueFormula, "", "Um efeito de %s foi ignorado: a condição falhou.", x.c.namePT(a.owner))
		ok = false
	}
	x.conditions[a.effect] = ok
	return ok
}

// value runs a modifier's formula. ok is false when it fails (an Issue is
// recorded).
func (x *deriver) value(a activeEffect) (int, bool) {
	n, err := a.effect.value.Int(x.env)
	if err != nil {
		x.issue(IssueFormula, "", "Um efeito de %s foi ignorado: a fórmula falhou.", x.c.namePT(a.owner))
		return 0, false
	}
	return n, true
}

// modifiers applies every untagged modifier for target to base, in order:
// set replaces, max keeps the larger, add adds.
func (x *deriver) modifiers(target string, base int) int {
	for _, a := range x.active {
		e := a.effect
		if e.Type != "modifier" || e.Target != target || len(e.Tags) > 0 || !x.applies(a) {
			continue
		}
		v, ok := x.value(a)
		if !ok {
			continue
		}
		switch e.Mode {
		case "set":
			base = v
		case "max":
			base = max(base, v)
		case "add":
			base += v
		}
	}
	return base
}

// hasHandler says whether an active effect names this handler.
func (x *deriver) hasHandler(name string) bool {
	for _, a := range x.active {
		if a.effect.Type == "handler" && a.effect.Handler == name {
			return true
		}
	}
	return false
}

// customBackground says the sheet has a background of the player's own: no
// background key, and any of its parts filled in.
func (x *deriver) customBackground() bool {
	b := x.b
	return b.Background == "" && (b.CustomBackgroundName != "" || len(b.CustomBackgroundSkills) > 0 || x.customBackgroundStarted())
}

// customBackgroundStarted says any of the parts SRD 5.1 "Customizing a
// Background" adds to the name and the skills (the tools or languages, the
// feature, the equipment) is filled in: the sheet was written by an editor that
// knows them.
func (x *deriver) customBackgroundStarted() bool {
	b := x.b
	return len(b.CustomBackgroundProficiencies) > 0 || b.CustomBackgroundFeatureName != "" || b.CustomBackgroundFeature != "" || b.CustomBackgroundEquipment != ""
}
