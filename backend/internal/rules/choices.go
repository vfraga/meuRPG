package rules

import (
	"fmt"
	"slices"
	"strconv"
	"strings"
)

// effectHints turns situational effects into Hints: advantage and
// disadvantage (roll_mode), bonuses that only count in some situations
// (tagged proficiency and modifier effects) and notes with a text.
func (x *deriver) effectHints() {
	for _, a := range x.active {
		if !x.applies(a) {
			continue
		}
		x.effectHint(a)
	}
}

// effectHint dispatches one applying effect to the hint of its kind; the other
// effects give none.
func (x *deriver) effectHint(a activeEffect) {
	e := a.effect
	switch {
	case e.Type == "roll_mode":
		x.rollModeHint(a)
	case e.Type == "proficiency" && len(e.Tags) > 0 && strings.HasPrefix(e.Proficiency, "skill:"):
		x.taggedSkillHint(a)
	case e.Type == "modifier" && len(e.Tags) > 0:
		x.taggedModifierHint(a)
	case e.Type == "note" && e.TextPT != "":
		x.noteHint(a)
	case e.Type == "resistance":
		x.resistanceHint(a)
	case e.Type == "condition_immunity":
		x.conditionImmunityHint(a)
	}
}

// rollModeHint is advantage or disadvantage on the effect's targets.
func (x *deriver) rollModeHint(a activeEffect) {
	targets := make([]string, len(a.effect.Targets))
	for i, t := range a.effect.Targets {
		targets[i] = hintTarget(t)
	}
	x.hint(a, targets, a.effect.Roll, 0)
	if a.effect.AgainstCondition != "" {
		h := &x.d.Hints[len(x.d.Hints)-1]
		h.AgainstCondition = a.effect.AgainstCondition
		if a.effect.TextPT == "" {
			h.TextPT = x.c.rollAgainstText(a.effect)
		}
	}
}

// rollAgainstText is the sentence of a roll mode against a condition, in Portuguese.
func (c *content) rollAgainstText(e *Effect) string {
	mode := map[string]string{"advantage": "Vantagem", "disadvantage": "Desvantagem"}[e.Roll]
	return fmt.Sprintf("%s em testes de resistência para não ficar com a condição %s.", mode, strings.ToLower(c.namePT(e.AgainstCondition)))
}

// conditionImmunityHint is the sheet's note of the conditions the character takes none of.
func (x *deriver) conditionImmunityHint(a activeEffect) {
	e := a.effect
	if e.TextPT == "" {
		names := make([]string, len(e.Conditions))
		for i, k := range e.Conditions {
			names[i] = strings.ToLower(x.c.namePT(k))
		}
		e = &Effect{TextPT: "Imune às condições: " + strings.Join(names, ", ") + "."}
		a = activeEffect{owner: a.owner, effect: e}
	}
	x.hint(a, nil, "note", 0)
}

// taggedSkillHint is a skill proficiency that only counts in some situations,
// such as Artificer's Lore: History with expertise, only about magic items. The
// hint carries the whole bonus.
func (x *deriver) taggedSkillHint(a activeEffect) {
	e := a.effect
	s, ok := x.c.skills[e.Proficiency]
	if !ok {
		return
	}
	lvl := max(x.skillLevel[e.Proficiency], proficiencyLevelOf(e.Level))
	v := x.mods[Ability(s.Ability)] + x.profBonus(lvl)
	x.hint(a, []string{e.Proficiency}, "bonus", v)
}

// taggedModifierHint is a situational modifier no step applied already.
func (x *deriver) taggedModifierHint(a activeEffect) {
	if x.appliedTagged[a.effect] {
		return
	}
	if v, ok := x.value(a); ok {
		x.hint(a, []string{hintTarget(a.effect.Target)}, "bonus", v)
	}
}

// noteHint is a note with a text, and its value when it has one; a value that
// fails drops the note.
func (x *deriver) noteHint(a activeEffect) {
	v := 0
	if a.effect.value != nil {
		var ok bool
		if v, ok = x.value(a); !ok {
			return
		}
	}
	x.hint(a, nil, "note", v)
}

// hintTarget writes skill targets as skill keys ("skill.history" becomes
// "skill:history"), and leaves the others ("save.int") as they are.
func hintTarget(t string) string {
	if s, ok := strings.CutPrefix(t, "skill."); ok {
		return "skill:" + s
	}
	return t
}

// hint adds one Hint. The effect's TextPT may carry {value}, replaced by
// the signed value ("+8").
func (x *deriver) hint(a activeEffect, targets []string, mode string, value int) {
	e := a.effect
	text := e.TextPT
	if text == "" {
		text = x.c.namePT(a.owner)
	}
	text = strings.ReplaceAll(text, "{value}", signed(value))
	text = strings.ReplaceAll(text, "{n}", strconv.Itoa(value))
	h := Hint{Source: a.owner, Targets: targets, Mode: mode, Tags: e.Tags, TextPT: text}
	if len(targets) > 0 {
		h.Target = targets[0]
	}
	if mode == "bonus" {
		h.Value = value
	}
	x.d.Hints = append(x.d.Hints, h)
}

func signed(n int) string {
	if n >= 0 {
		return "+" + strconv.Itoa(n)
	}
	return strconv.Itoa(n)
}

// checkChoices reports choices the rules would not allow: the number of
// chosen skills and expertise, the number of options picked for each feature
// that offers some (fighting styles, metamagic, invocations, a pact boon...),
// the custom background's skills and the multiclass prerequisites. Spell
// choices are checked in spellcasting.
func (x *deriver) checkChoices() {
	allowed, expertiseAllowed := x.skillAllowance()
	chosen := x.chosenSkills()
	x.checkSkillCount(allowed, len(chosen))
	x.checkExpertiseCount(expertiseAllowed)
	x.checkProficiencyPicks()
	x.checkOptionCounts()
	x.checkCustomBackground()
	x.checkMulticlass()
}

// skillAllowance is how many skills and how many expertise picks the effects
// give. Skills: the player chooses the starting class's skills, one more from
// some multiclass classes, and those that effects or traits offer (Skill
// Versatility, the bard's Bonus Proficiencies). Race and background skills come
// automatically and do not count.
func (x *deriver) skillAllowance() (skills, expertise int) {
	for i, oc := range x.classes {
		switch {
		case i == 0:
			skills += oc.class.SkillChoices.Choose
		case oc.class.Multiclass.SkillChoices != nil:
			skills += oc.class.Multiclass.SkillChoices.Choose
		}
	}
	for _, t := range x.ownedTraits() {
		if t.ProficiencyChoices > 0 && len(t.ProficiencyOptions) > 0 && strings.HasPrefix(t.ProficiencyOptions[0], "proficiency:skill-") {
			skills += t.ProficiencyChoices
		}
	}
	for _, a := range x.active {
		if a.effect.Type != "choice" || a.effect.DailyChoice {
			continue // a daily choice is made again each day: it never adds to the skills the player keeps
		}
		switch a.effect.Choice {
		case "skill":
			skills += a.effect.Count
		case "expertise":
			expertise += a.effect.Count
		}
	}
	return skills, expertise
}

// chosenSkills are the skills the player chose, without the automatic ones; an
// unknown key is an issue and counts for nothing.
func (x *deriver) chosenSkills() map[string]bool {
	chosen := map[string]bool{}
	for i, s := range x.b.SkillProficiencies {
		if _, ok := x.c.skills[s]; !ok {
			x.issue(IssueUnknownKey, fmt.Sprintf("full.skill_proficiency_keys[%d]", i), "A perícia escolhida não existe.")
			continue
		}
		if !x.automaticSkills[s] {
			chosen[s] = true
		}
	}
	return chosen
}

// checkSkillCount compares the chosen skills with the allowance; a sheet without
// a class has none to compare.
func (x *deriver) checkSkillCount(allowed, n int) {
	if len(x.classes) == 0 {
		return
	}
	switch {
	case n > allowed:
		subject, change := x.skillChange(allowed, n)
		x.issueChange(IssueSkillCount, "full.skill_proficiency_keys", subject, change,
			fmt.Sprintf("Há %d perícias escolhidas; o personagem escolhe %d.", n, allowed))
	case n < allowed:
		x.d.OpenChoices = append(x.d.OpenChoices, OpenChoice{Kind: OpenChoiceSkills, Missing: allowed - n})
		subject, change := x.skillChange(allowed, n)
		x.issueChange(IssueSkillCount, "full.skill_proficiency_keys", subject, change,
			missingSkills(allowed-n))
	}
}

// checkExpertiseCount adds the expertise of features such as the rogue's
// Expertise to what the effects give, and compares the sheet's with it.
func (x *deriver) checkExpertiseCount(allowed int) {
	for _, f := range x.d.Features {
		if feat, ok := x.c.features[f.Key]; ok {
			allowed += feat.ExpertiseChoices
		}
	}
	switch n := len(x.b.Expertise); {
	case n > allowed:
		x.issue(IssueExpertise, "full.expertise_skill_keys", "Há %d perícias com especialização; o personagem tem %d.", n, allowed)
	case n < allowed:
		x.openPick(OpenChoiceExpertise, allowed-n, "full.expertise_skill_keys", "especialização", "especializações")
	}
}

// openPick records a pick the sheet has not made: an open choice, and an issue on the field the player fills.
func (x *deriver) openPick(kind string, missing int, field, one, many string) {
	x.d.OpenChoices = append(x.d.OpenChoices, OpenChoice{Kind: kind, Missing: missing})
	noun := many
	if missing == 1 {
		noun = one
	}
	x.issue(IssueMissing, field, "Falta escolher %d %s.", missing, noun)
}

// checkCustomBackground checks a custom background: it grants two skills, like
// every SRD background.
func (x *deriver) checkCustomBackground() {
	if !x.customBackground() {
		return
	}
	if n := len(x.b.CustomBackgroundSkills); n < CustomBackgroundSkillCount {
		x.issue(IssueSkillCount, "full.custom_background.skill_keys", "O antecedente personalizado concede %d perícias; faltam %d.", CustomBackgroundSkillCount, CustomBackgroundSkillCount-n)
	}
	x.checkCustomBackgroundParts()
	x.checkCustomBackgroundLanguages()
}

// checkCustomBackgroundParts: SRD 5.1 "Customizing a Background": two tools or
// languages, a feature and the equipment too. A sheet written
// before these fields existed shows nothing new: the three warnings come only
// once the sheet has any of them, that is, once the editor that writes them
// saved it.
func (x *deriver) checkCustomBackgroundParts() {
	if !x.customBackgroundStarted() {
		return
	}
	b := x.b
	if n := len(b.CustomBackgroundProficiencies); n < CustomBackgroundProficiencyCount {
		x.issue(IssueMissing, "full.custom_background.proficiency_keys", "O antecedente personalizado concede %d ferramentas ou idiomas; faltam %d.", CustomBackgroundProficiencyCount, CustomBackgroundProficiencyCount-n)
	}
	if b.CustomBackgroundFeatureName == "" || b.CustomBackgroundFeature == "" {
		x.issue(IssueMissing, "full.custom_background.feature_name", "O antecedente personalizado tem uma característica: falta o nome ou o texto dela.")
	}
	if b.CustomBackgroundEquipment == "" {
		x.issue(IssueMissing, "full.custom_background.equipment", "O antecedente personalizado traz equipamento: falta descrevê-lo.")
	}
}

// checkCustomBackgroundLanguages: a language the race already gives uses one of
// the two picks for nothing.
func (x *deriver) checkCustomBackgroundLanguages() {
	if x.race == nil {
		return
	}
	for i, k := range x.b.CustomBackgroundProficiencies {
		if slices.Contains(x.race.Languages, k) {
			x.issue(IssueMissing, fmt.Sprintf("full.custom_background.proficiency_keys[%d]", i), "O idioma %s já vem da raça: ele gasta uma das duas escolhas do antecedente sem acrescentar nada.", x.c.namePT(k))
		}
	}
}

// checkMulticlass: multiclassing needs the prerequisites of every class, the
// first one included.
func (x *deriver) checkMulticlass() {
	if len(x.classes) <= 1 {
		return
	}
	for _, oc := range x.classes {
		if !x.meetsMulticlass(oc) {
			x.issueChange(IssueMulticlass, fmt.Sprintf("full.classes[%d].class_key", oc.index), oc.key,
				"agora pede outras habilidades para multiclasse; os desta ficha não cumprem.",
				fmt.Sprintf("As habilidades não cumprem o pré-requisito de multiclasse de %s.", x.c.namePT(oc.key)))
		}
	}
}

// skillChange is the sentence for "A classe mudou" when the number of skills a
// sheet chose no longer matches what it is entitled to, and the entry it is
// about. With the starting class the only source it is the class ("agora dá 2
// perícias no nível 1; esta ficha tem 3."); with other sources (a race's trait)
// it is only the count, and no entry is named.
// missingSkills: "Falta 1 perícia para escolher." / "Faltam 2 perícias para escolher.".
func missingSkills(n int) string {
	if n == 1 {
		return "Falta 1 perícia para escolher."
	}
	return fmt.Sprintf("Faltam %d perícias para escolher.", n)
}

func (x *deriver) skillChange(allowed, chosen int) (subject, change string) {
	if len(x.classes) > 0 && allowed == x.classes[0].class.SkillChoices.Choose {
		return x.classes[0].key, fmt.Sprintf("agora dá %s no nível 1; esta ficha tem %d.", countPT(allowed, "perícia", "perícias"), chosen)
	}
	return "", fmt.Sprintf("o total de perícias para escolher agora é %d; esta ficha tem %d.", allowed, chosen)
}

func (x *deriver) meetsMulticlass(oc ownedClass) bool {
	mc := oc.class.Multiclass
	for k, v := range mc.Minimums {
		if x.scores[Ability(k)] < v {
			return false
		}
	}
	if len(mc.AnyOf) == 0 {
		return true
	}
	for k, v := range mc.AnyOf {
		if x.scores[Ability(k)] >= v {
			return true
		}
	}
	return false
}

// summarize names a Build's race and classes for a list row.
func summarize(b Build, c *content) Summary {
	var s Summary
	if sub, ok := c.subraces[b.Subrace]; ok && sub.Race == b.Race {
		s.RaceNamePT = c.namePT(b.Subrace)
	} else if _, ok := c.races[b.Race]; ok {
		s.RaceNamePT = c.namePT(b.Race)
	}
	var parts []string
	for _, cl := range b.Classes {
		if _, ok := c.classes[cl.Class]; !ok {
			continue
		}
		parts = append(parts, fmt.Sprintf("%s %d", c.namePT(cl.Class), cl.Level))
		s.TotalLevel += cl.Level
	}
	s.ClassSummaryPT = strings.Join(parts, " / ")
	return s
}

// optionPool is the options that a group of owned features offer and how many
// of them the character picks. Features that offer the same options (the
// fighter's Fighting Style and the champion's Additional Fighting Style, the
// three metamagic features) share one pool, whose allowance is their sum.
type optionPool struct {
	name    string
	allowed int
	options map[string]bool
}

// checkOptionCounts raises an Issue when a pool holds more distinct options
// than its features allow. A repeated option name counts once (the same
// style from two classes is the same style).
func (x *deriver) checkOptionCounts() {
	for _, p := range x.optionPools() {
		x.checkPoolCount(p)
	}
}

// optionPools are the pools of the features the sheet owns, each feature once,
// and the warlock's invocations last with the number the class tables give.
func (x *deriver) optionPools() []*optionPool {
	c := x.c
	var pools []*optionPool
	seen := map[string]bool{}
	for _, f := range x.d.Features {
		if seen[f.Key] {
			continue
		}
		seen[f.Key] = true
		if f.Key == invocationsFeature {
			continue
		}
		for _, d := range c.featureGains([]string{f.Key}, "").choices {
			pools = c.joinPool(pools, d.feature, d.choose, d.options)
		}
	}
	if inv := c.features[invocationsFeature]; inv != nil && seen[invocationsFeature] {
		pools = c.joinPool(pools, invocationsFeature, x.invocationsKnownTotal(), inv.Options)
	}
	return pools
}

// joinPool adds a feature's options to the pool that shares one of them, or to
// a new pool named after it. A feature whose options are in two pools bridges
// them: the first takes the second's options and allowance.
func (c *content) joinPool(pools []*optionPool, feature string, choose int, options []string) []*optionPool {
	var into *optionPool
	for _, p := range pools {
		if !slices.ContainsFunc(options, func(o string) bool { return p.options[o] }) {
			continue
		}
		if into == nil {
			into = p
			continue
		}
		into.allowed += p.allowed // the new feature bridges two pools
		for o := range p.options {
			into.options[o] = true
		}
		p.allowed, p.options = 0, map[string]bool{}
	}
	if into == nil {
		into = &optionPool{name: c.namePT(feature), options: map[string]bool{}}
		pools = append(pools, into)
	}
	into.allowed += choose
	for _, o := range options {
		into.options[o] = true
	}
	return pools
}

// invocationsKnownTotal is the invocations the class tables give the sheet's
// classes at their levels.
func (x *deriver) invocationsKnownTotal() int {
	known := 0
	for _, oc := range x.classes {
		if rows := x.c.classLevels[oc.key]; oc.level >= 1 && oc.level <= len(rows) {
			known += invocationsKnown(rows[oc.level-1])
		}
	}
	return known
}

// checkPoolCount raises the issue of a pool whose picks, by name, pass its
// allowance.
func (x *deriver) checkPoolCount(p *optionPool) {
	names := map[string]bool{}
	for _, key := range x.b.FeatureChoices {
		if f, ok := x.c.features[key]; ok && p.options[key] {
			names[f.Name] = true
		}
	}
	if n := len(names); n > p.allowed {
		x.issue(IssueChoiceCount, "full.feature_choice_keys", "Há %d escolhas em %s; o personagem tem %d.", n, p.name, p.allowed)
	}
}
