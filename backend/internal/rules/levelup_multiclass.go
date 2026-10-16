package rules

import (
	"slices"
	"strings"
)

// Multiclassing at level-up (SRD 5.1, "Multiclassing"). A character that can
// level up may take its next level in a class it has, or in a class it does not
// have yet, which then joins the sheet at level 1:
//
//   - the prerequisites ("Prerequisites"): the main ability of every class the
//     character has and of the new one, 13 or more in the final score;
//   - experience points, hit points and the proficiency bonus follow the total
//     level, and the new class's hit die gives a level after the first, never
//     the maximum ("Experience Points", "Hit Points and Hit Dice", "Proficiency
//     Bonus");
//   - the new class gives only the reduced proficiencies of its multiclass
//     table, once, and never starting equipment ("Proficiencies");
//   - the class features of the new class's level 1 come with four exceptions
//     ("Class Features"): Channel Divinity, Extra Attack and Unarmored Defense do
//     not stack, and Spellcasting follows its own rule;
//   - spells, cantrips and preparation are those of each class, and the spell
//     slots come from the multiclass spellcaster table, with Pact Magic apart
//     ("Spellcasting").
//
// Derive already follows the rules above for a sheet that has several classes:
// this file adds the way into a new class, and what the screens need to draw it.

// minClassesForMulticlass is how many classes make a sheet multiclass.
const minClassesForMulticlass = 2

// wizardStartingSpells is the number of 1st-level spells the Wizard's spellbook
// has when the class is taken (SRD 5.1, Wizard, "Spellbook").
const wizardStartingSpells = 6

// MulticlassRequirement is one ability a class asks for, with the character's
// final score in it.
type MulticlassRequirement struct {
	Ability Ability
	Minimum int
	Have    int
	Met     bool
}

// MulticlassPrerequisite is what a class asks of a character that has several
// classes (SRD 5.1, "Multiclassing", "Prerequisites"), judged against the final
// scores.
type MulticlassPrerequisite struct {
	Class       string
	ClassNamePT string
	// AnyOf says one requirement is enough (the Fighter's Strength or Dexterity);
	// otherwise every one must be met.
	AnyOf        bool
	Requirements []MulticlassRequirement
	Met          bool
}

// refusal is the LevelUpError of a prerequisite that is not met: the missing
// ability, or for an "or" prerequisite the closest one.
func (p MulticlassPrerequisite) refusal(reason string) *LevelUpError {
	miss := MulticlassRequirement{}
	for _, r := range p.Requirements {
		switch {
		case r.Met:
		case miss.Minimum == 0, p.AnyOf && r.Have > miss.Have:
			miss = r
		}
	}
	e := refuse("class_key", reason, "%s asks for %s %d, the character has %d", p.Class, miss.Ability, miss.Minimum, miss.Have)
	e.ClassKey, e.Ability, e.Minimum, e.Have = p.Class, miss.Ability, miss.Minimum, miss.Have
	return e
}

// scoresOf are the final ability scores of a Build (race, increases and effects
// included), the values the prerequisites are judged on.
func scoresOf(b Build, c *content) map[Ability]int {
	scores := map[Ability]int{}
	for _, s := range derive(b, c).Abilities {
		scores[s.Ability] = s.Score
	}
	return scores
}

// prerequisite judges one class's multiclass prerequisite against scores.
func (c *content) prerequisite(classKey string, scores map[Ability]int) MulticlassPrerequisite {
	p := MulticlassPrerequisite{Class: classKey, ClassNamePT: c.namePT(classKey)}
	class := c.classes[classKey]
	if class == nil {
		return p
	}
	mc := class.Multiclass
	table := mc.Minimums
	if len(mc.AnyOf) > 0 {
		table, p.AnyOf = mc.AnyOf, true
	}
	for _, a := range AllAbilities() {
		if minimum, ok := table[string(a)]; ok {
			have := scores[a]
			p.Requirements = append(p.Requirements, MulticlassRequirement{Ability: a, Minimum: minimum, Have: have, Met: have >= minimum})
		}
	}
	if p.AnyOf {
		p.Met = slices.ContainsFunc(p.Requirements, func(r MulticlassRequirement) bool { return r.Met })
	} else {
		p.Met = !slices.ContainsFunc(p.Requirements, func(r MulticlassRequirement) bool { return !r.Met })
	}
	return p
}

// MulticlassPrerequisites judges these classes' prerequisites against the final
// scores of b, in the order given. A class the content does not have is judged
// as having none.
func MulticlassPrerequisites(b Build, classKeys []string, c *Content) []MulticlassPrerequisite {
	scores := scoresOf(b, c.c)
	out := make([]MulticlassPrerequisite, 0, len(classKeys))
	for _, k := range classKeys {
		out = append(out, c.c.prerequisite(k, scores))
	}
	return out
}

// multiclassAllowed refuses taking class classKey as a new class: the character
// is at level 20, a class it has asks for an ability it lacks (it comes first,
// because it closes every new class), or the new class does. The prerequisites
// are judged on the final scores of b.
func (c *content) multiclassAllowed(b Build, classKey string) *LevelUpError {
	if b.totalLevel() >= MaxLevel {
		return refuse("full.classes", LevelUpReasonMaxLevel, "the character is already level %d", MaxLevel)
	}
	scores := scoresOf(b, c)
	for _, cl := range b.Classes {
		if p := c.prerequisite(cl.Class, scores); !p.Met {
			return p.refusal(LevelUpReasonMulticlassPrerequisiteCurrent)
		}
	}
	if p := c.prerequisite(classKey, scores); !p.Met {
		return p.refusal(LevelUpReasonMulticlassPrerequisite)
	}
	return nil
}

// LevelUpClassChoice is one way the next level can go, for the class step: a
// class the character has (its next level) or a class it does not have (its
// first).
type LevelUpClassChoice struct {
	Class       string
	ClassNamePT string
	// New says the character does not have the class.
	New                bool
	FromLevel, ToLevel int
	// SubclassNamePT is the subclass the character has in the class, and
	// SubclassDue says the class picks its subclass at ToLevel.
	SubclassNamePT string
	SubclassDue    bool
	// Prerequisite is what the class asks for as one of several (for a class the
	// character has, what a new class asks of it).
	Prerequisite MulticlassPrerequisite
	// Available says the level can go to this class now; when it cannot,
	// Unavailable is why: "max_level" (the character or the class is at level 20),
	// "prerequisite" (the new class asks for more) or "prerequisite_current" (a
	// class the character has asks for more, which closes every new class).
	Available   bool
	Unavailable string
}

// Reasons of LevelUpClassChoice.Unavailable.
const (
	UnavailableMaxLevel            = "max_level"
	UnavailablePrerequisite        = "prerequisite"
	UnavailablePrerequisiteCurrent = "prerequisite_current"
)

// LevelUpClasses lists the ways the next level can go: the classes the character
// has, in sheet order, then every other class of the content, sorted by
// Portuguese name. Hiding the table's archived or switched-off classes from a
// player is the caller's.
func LevelUpClasses(before Build, c *Content) []LevelUpClassChoice {
	x := c.c
	scores := scoresOf(before, x)
	atCap := before.totalLevel() >= MaxLevel
	currentOK := true
	var out []LevelUpClassChoice
	for _, cl := range before.Classes {
		class := x.classes[cl.Class]
		if class == nil {
			continue
		}
		p := x.prerequisite(cl.Class, scores)
		currentOK = currentOK && p.Met
		ch := LevelUpClassChoice{
			Class: cl.Class, ClassNamePT: x.namePT(cl.Class), FromLevel: cl.Level, ToLevel: cl.Level + 1, Prerequisite: p,
			SubclassNamePT: cl.CustomSubclassName, Available: !atCap && cl.Level < MaxLevel,
		}
		if cl.Subclass != "" {
			ch.SubclassNamePT = x.namePT(cl.Subclass)
		}
		ch.SubclassDue = cl.Subclass == "" && cl.CustomSubclassName == "" && class.SubclassLevel == cl.Level+1
		if !ch.Available {
			ch.Unavailable = UnavailableMaxLevel
		}
		out = append(out, ch)
	}
	var others []LevelUpClassChoice
	for _, key := range sortedKeys(x.classes) {
		if slices.ContainsFunc(before.Classes, func(cl ClassLevel) bool { return cl.Class == key }) {
			continue
		}
		class := x.classes[key]
		p := x.prerequisite(key, scores)
		ch := LevelUpClassChoice{
			Class: key, ClassNamePT: x.namePT(key), New: true, FromLevel: 0, ToLevel: 1, Prerequisite: p,
			SubclassDue: class.SubclassLevel == 1,
		}
		switch {
		case atCap:
			ch.Unavailable = UnavailableMaxLevel
		case !currentOK:
			ch.Unavailable = UnavailablePrerequisiteCurrent
		case !p.Met:
			ch.Unavailable = UnavailablePrerequisite
		}
		ch.Available = ch.Unavailable == ""
		others = append(others, ch)
	}
	slices.SortStableFunc(others, func(a, b LevelUpClassChoice) int { return comparePT(a.ClassNamePT, b.ClassNamePT) })
	return append(out, others...)
}

// MulticlassOffer is what taking a class as a later class gives and asks for
// (SRD 5.1, "Multiclassing", "Proficiencies").
type MulticlassOffer struct {
	// Proficiencies are the armor, weapon and tool proficiencies the class's
	// multiclass table gives, each marked when the character already has it.
	Proficiencies []MulticlassProficiency
	// Skills and Instruments are the picks the table asks for: one skill from a
	// list (the Bard's from all of them), one musical instrument; nil for none.
	Skills      *MulticlassPick
	Instruments *MulticlassPick
	// CasterLevelBefore and After are the character's caster level in the
	// multiclass spellcaster table, and SlotsByTable says that table gives the
	// slots after the level (more than one class has the Spellcasting feature);
	// with one, the slots are the class's own.
	CasterLevelBefore int
	CasterLevelAfter  int
	SlotsByTable      bool
}

// MulticlassProficiency is a proficiency the multiclass table gives.
type MulticlassProficiency struct {
	Key, NamePT string
	Kind        string
	AlreadyHave bool
}

// MulticlassPick is "choose Choose of From" among a multiclass table's options.
type MulticlassPick struct {
	Choose int
	From   []MulticlassOption
}

// MulticlassOption is one option of a pick, marked when the character already has it.
type MulticlassOption struct {
	Key, NamePT string
	AlreadyHave bool
}

// multiclassOffer is the offer of taking classKey as a later class. dBefore is
// the character before the level.
func (c *content) multiclassOffer(b Build, dBefore Derived, bare Build, classKey string) *MulticlassOffer {
	mc := c.classes[classKey].Multiclass
	o := &MulticlassOffer{}
	have := map[string]bool{}
	for _, p := range dBefore.Proficiencies {
		have[p.Key] = true
	}
	for _, key := range mc.Proficiencies {
		p := c.proficiencies[key]
		if p == nil {
			continue
		}
		o.Proficiencies = append(o.Proficiencies, MulticlassProficiency{Key: key, NamePT: c.proficiencyNamePT(key), Kind: p.Kind, AlreadyHave: c.covered(have, key)})
	}
	if mc.SkillChoices != nil {
		haveSkill := map[string]bool{}
		for _, s := range dBefore.Skills {
			haveSkill[s.Key] = s.Proficiency >= ProficiencyFull // half (Jack of All Trades) is not a proficiency
		}
		pick := &MulticlassPick{Choose: mc.SkillChoices.Choose}
		for _, k := range mc.SkillChoices.From {
			pick.From = append(pick.From, MulticlassOption{Key: k, NamePT: c.namePT(k), AlreadyHave: haveSkill[k]})
		}
		o.Skills = pick
	}
	if mc.InstrumentChoices != nil {
		pick := &MulticlassPick{Choose: mc.InstrumentChoices.Choose}
		for _, k := range mc.InstrumentChoices.From {
			pick.From = append(pick.From, MulticlassOption{Key: k, NamePT: c.proficiencyNamePT(k), AlreadyHave: hasTool(b.ToolProficiencies, c.proficiencyNamePT(k))})
		}
		o.Instruments = pick
	}
	var before, after int
	before, _ = c.casterLevel(b)
	var casters int
	after, casters = c.casterLevel(bare)
	o.CasterLevelBefore, o.CasterLevelAfter, o.SlotsByTable = before, after, casters > 1
	return o
}

// covered says whether the proficiencies the character has (by key) already give
// proficiency key: the key itself, "all armor" for each armor category, and a
// weapon category for each weapon of it (the Monk's shortswords are martial).
func (c *content) covered(have map[string]bool, key string) bool {
	if have[key] {
		return true
	}
	p := c.proficiencies[key]
	if p == nil || len(p.Refs) == 0 {
		return false
	}
	for _, ref := range p.Refs {
		if !c.refCovered(have, ref) {
			return false
		}
	}
	return true
}

// refCovered says whether one piece of equipment, or a category of it, is covered
// by the proficiencies in have.
func (c *content) refCovered(have map[string]bool, ref string) bool {
	for h := range have {
		hp := c.proficiencies[h]
		if hp == nil {
			continue
		}
		for _, hr := range hp.Refs {
			switch {
			case hr == ref:
				return true
			case hr == "equipment-category:armor" && strings.HasPrefix(ref, "equipment-category:") && strings.HasSuffix(ref, "-armor"):
				return true
			case strings.HasPrefix(ref, "equipment:"):
				if eq := c.equipment[ref]; eq != nil && eq.Weapon != nil && hr == "equipment-category:"+eq.Weapon.Category+"-weapons" {
					return true
				}
			}
		}
	}
	return false
}

// hasTool says whether the free-text tool list has name, ignoring case.
func hasTool(tools []string, name string) bool {
	return slices.ContainsFunc(tools, func(t string) bool { return strings.EqualFold(strings.TrimSpace(t), name) })
}

// casterLevel is the character's level in the multiclass spellcaster table (all
// the levels of full casters, half of a half caster's and a third of a third
// caster's, each rounded down) and how many classes have the Spellcasting
// feature. Pact Magic is apart and counts for neither.
func (c *content) casterLevel(b Build) (level, casters int) {
	for _, cl := range b.Classes {
		if c.classes[cl.Class] == nil {
			continue
		}
		cast, ok := c.castingFor(cl.Class, c.subclasses[cl.Subclass])
		if !ok || cl.Level < cast.level || cast.effect.Progression == "pact" {
			continue
		}
		casters++
		level += casterLevelOf(cast.effect.Progression, cl.Level)
	}
	return level, casters
}

// CasterLevel is the character's level in the multiclass spellcaster table
// (SRD 5.1, "Multiclassing", "Spell Slots"), and how many of its classes have
// the Spellcasting feature; with more than one the table gives the slots.
func CasterLevel(b Build, c *Content) (level, casters int) {
	return c.c.casterLevel(b)
}

// checkMulticlassSkills checks the new skills of a level that takes a class as a later
// class: Skills.Choose of them come from the class's list and are not skills the
// character has. Anything else is a refusal of the level's skills.
func (c *content) checkMulticlassSkills(offer *MulticlassOffer, got []string, extra int) *LevelUpError {
	const field = "full.skill_proficiency_keys"
	want := extra
	if offer.Skills != nil {
		want += offer.Skills.Choose
	}
	if len(got) != want {
		reason := LevelUpReasonSkills
		if offer.Skills != nil {
			reason = LevelUpReasonProficiencyChoice
		}
		return refuse(field, reason, "the new level lets the player choose %d skills", want)
	}
	if offer.Skills == nil {
		return nil
	}
	inList := 0
	for _, k := range got {
		i := slices.IndexFunc(offer.Skills.From, func(o MulticlassOption) bool { return o.Key == k })
		if i >= 0 {
			if offer.Skills.From[i].AlreadyHave {
				return refuse(field, LevelUpReasonProficiencyChoice, "the character already has %s", k)
			}
			inList++
		}
	}
	if inList < offer.Skills.Choose || (extra == 0 && inList != len(got)) {
		return refuse(field, LevelUpReasonProficiencyChoice, "the multiclass skill comes from the class's list")
	}
	return nil
}

// checkMulticlassTools checks the free-text tool list of a level that takes a
// class as a later class: the instrument the Bard's table asks for, as the
// instrument's Portuguese name, and nothing else.
func (c *content) checkMulticlassTools(offer *MulticlassOffer, before, after []string) *LevelUpError {
	const field = "full.tool_proficiencies"
	if len(after) < len(before) || !slices.Equal(before, after[:len(before)]) {
		return refuse(field, LevelUpReasonInstrumentChoice, "a tool proficiency was removed or changed")
	}
	got := after[len(before):]
	want := 0
	if offer.Instruments != nil {
		want = offer.Instruments.Choose
	}
	if len(got) != want {
		if want == 0 {
			return refuse(field, LevelUpReasonLocked, "the sheet is locked: only the master changes this")
		}
		return refuse(field, LevelUpReasonInstrumentChoice, "the new class lets the player choose %d instruments", want)
	}
	for i, name := range got {
		j := slices.IndexFunc(offer.Instruments.From, func(o MulticlassOption) bool { return strings.EqualFold(o.NamePT, strings.TrimSpace(name)) })
		if j < 0 || offer.Instruments.From[j].AlreadyHave || slices.ContainsFunc(got[:i], func(o string) bool { return strings.EqualFold(o, name) }) {
			return refuse(field, LevelUpReasonInstrumentChoice, "an instrument the character does not already have, from the class's list")
		}
	}
	return nil
}

// MulticlassSummary is what the summary of a level up on a multiclass sheet adds
// to the numbers Derive already gives: the caster level, what the level gives as
// proficiencies, and the exceptions that stopped a number from growing.
type MulticlassSummary struct {
	// NewClass is the class the level adds to the sheet, "" when the level goes to
	// a class the character already has.
	NewClass string
	// CasterLevelBefore and CasterLevelAfter are the levels in the multiclass
	// spellcaster table, and SlotsByTable says it gives the slots after the level.
	CasterLevelBefore int
	CasterLevelAfter  int
	SlotsByTable      bool
	// ProficienciesGained are the armor, weapon and tool proficiencies the sheet
	// has after the level and not before, then the new skills and the instrument
	// picked.
	ProficienciesGained []NamedKey
	// Exceptions says which of the SRD's multiclass exceptions the level meets:
	// ExceptionExtraAttack, ExceptionChannelDivinity or ExceptionUnarmoredDefense.
	Exceptions []string
}

// The multiclass exceptions of SRD 5.1, "Multiclassing", "Class Features", that a
// level can meet.
const (
	// ExceptionExtraAttack: the level's Extra Attack does not add to the attacks the
	// character already makes.
	ExceptionExtraAttack = "extra_attack"
	// ExceptionChannelDivinity: the level's Channel Divinity gives effects, not uses.
	ExceptionChannelDivinity = "channel_divinity"
	// ExceptionUnarmoredDefense: the level's Unarmored Defense is the second one, so
	// the character does not gain it.
	ExceptionUnarmoredDefense = "unarmored_defense"
)

// MulticlassSummaryOf describes a level up from before to after, when the sheet
// has several classes after it; ok is false for a single-class sheet. newClass
// is the class the level adds, or "".
func MulticlassSummaryOf(before, after Build, newClass string, c *Content) (MulticlassSummary, bool) {
	if len(after.Classes) < minClassesForMulticlass {
		return MulticlassSummary{}, false
	}
	x := c.c
	s := MulticlassSummary{NewClass: newClass}
	var casters int
	s.CasterLevelBefore, _ = x.casterLevel(before)
	s.CasterLevelAfter, casters = x.casterLevel(after)
	s.SlotsByTable = casters > 1

	dBefore, dAfter := derive(before, x), derive(after, x)
	s.ProficienciesGained = x.proficienciesGained(before, after, dBefore, dAfter, newClass)
	s.Exceptions = x.multiclassExceptions(dBefore, dAfter)
	return s, true
}

// proficienciesGained are the proficiencies the sheet after has and the sheet
// before did not, then, for a new class, the new skills and tools picked.
func (c *content) proficienciesGained(before, after Build, dBefore, dAfter Derived, newClass string) []NamedKey {
	had := map[string]bool{}
	for _, p := range dBefore.Proficiencies {
		had[p.Key] = true
	}
	var out []NamedKey
	for _, p := range dAfter.Proficiencies {
		if !c.covered(had, p.Key) {
			out = append(out, NamedKey{Key: p.Key, NamePT: p.NamePT})
		}
	}
	if newClass == "" {
		return out
	}
	fresh, _ := added(before.SkillProficiencies, after.SkillProficiencies)
	for _, k := range fresh {
		out = append(out, NamedKey{Key: k, NamePT: c.namePT(k)})
	}
	tools, _ := added(before.ToolProficiencies, after.ToolProficiencies)
	for _, t := range tools {
		out = append(out, NamedKey{NamePT: t})
	}
	return out
}

// multiclassExceptions are the exceptions the level meets: a feature it gives
// whose effect the character already had in full from another feature.
func (c *content) multiclassExceptions(dBefore, dAfter Derived) []string {
	hadFeature := map[string]bool{}
	for _, f := range dBefore.Features {
		hadFeature[f.Key] = true
	}
	var out []string
	for _, f := range dAfter.Features {
		if !hadFeature[f.Key] {
			out = c.featureExceptions(out, f.Key, dBefore, dAfter)
		}
	}
	return out
}

// featureExceptions adds to out the exceptions a new feature (key) meets: its
// Extra Attack or resource adds nothing, or it is a second Unarmored Defense.
func (c *content) featureExceptions(out []string, key string, dBefore, dAfter Derived) []string {
	for _, e := range c.effects[key] {
		switch {
		case e.Type == "extra_attack" && dBefore.AttacksPerAction >= e.Count:
			out = appendOnce(out, ExceptionExtraAttack)
		case e.Type == "resource" && resourceMaxOf(dBefore, e.Resource) > 0 && resourceMaxOf(dBefore, e.Resource) >= resourceMaxOf(dAfter, e.Resource):
			out = appendOnce(out, ExceptionChannelDivinity)
		}
	}
	if unarmoredDefenseFeatures[key] && slices.ContainsFunc(dBefore.Features, func(o Feature) bool { return unarmoredDefenseFeatures[o.Key] }) {
		out = appendOnce(out, ExceptionUnarmoredDefense)
	}
	return out
}

// resourceMax is the maximum of a resource, 0 when the character has none.
func resourceMaxOf(d Derived, key string) int {
	for _, r := range d.Resources {
		if r.Key == key {
			return r.Max
		}
	}
	return 0
}

func appendOnce(list []string, s string) []string {
	if slices.Contains(list, s) {
		return list
	}
	return append(list, s)
}

// withNewClass is b with classKey added at level 0, last. The level-up treats
// the class as one the character has, at the level before its first.
func withNewClass(b Build, classKey string) Build {
	out := b.clone()
	out.Classes = append(out.Classes, ClassLevel{Class: classKey})
	return out
}

// newClassOf says the level the Builds differ by adds a class: after has one
// more class than before.
func newClassOf(before, after Build) (string, bool) {
	if len(after.Classes) != len(before.Classes)+1 {
		return "", false
	}
	return after.Classes[len(after.Classes)-1].Class, true
}
