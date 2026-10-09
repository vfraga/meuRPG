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
	levels := x.skillLevel
	for _, a := range x.active {
		e := a.effect
		if !x.applies(a) {
			continue
		}
		switch {
		case e.Type == "roll_mode":
			targets := make([]string, len(e.Targets))
			for i, t := range e.Targets {
				targets[i] = hintTarget(t)
			}
			x.hint(a, targets, e.Roll, 0)
		case e.Type == "proficiency" && len(e.Tags) > 0 && strings.HasPrefix(e.Proficiency, "skill:"):
			// Such as Artificer's Lore: History with expertise, only about
			// magic items. The hint carries the whole bonus.
			s, ok := x.c.skills[e.Proficiency]
			if !ok {
				continue
			}
			lvl := max(levels[e.Proficiency], proficiencyLevelOf(e.Level))
			v := x.mods[Ability(s.Ability)] + x.profBonus(lvl)
			x.hint(a, []string{e.Proficiency}, "bonus", v)
		case e.Type == "modifier" && len(e.Tags) > 0:
			if x.appliedTagged[e] {
				continue
			}
			v, ok := x.value(a)
			if ok {
				x.hint(a, []string{hintTarget(e.Target)}, "bonus", v)
			}
		case e.Type == "note" && e.TextPT != "":
			v := 0
			if e.value != nil {
				var ok bool
				if v, ok = x.value(a); !ok {
					continue
				}
			}
			x.hint(a, nil, "note", v)
		}
	}
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
	c := x.c

	// Skills: the player chooses the starting class's skills, one more from
	// some multiclass classes, and those that effects or traits offer
	// (Skill Versatility, the bard's Bonus Proficiencies). Race and
	// background skills come automatically and do not count.
	automatic := x.automaticSkills
	allowed := 0
	for i, oc := range x.classes {
		switch {
		case i == 0:
			allowed += oc.class.SkillChoices.Choose
		case oc.class.Multiclass.SkillChoices != nil:
			allowed += oc.class.Multiclass.SkillChoices.Choose
		}
	}
	for _, t := range x.ownedTraits() {
		if t.ProficiencyChoices > 0 && len(t.ProficiencyOptions) > 0 && strings.HasPrefix(t.ProficiencyOptions[0], "proficiency:skill-") {
			allowed += t.ProficiencyChoices
		}
	}
	expertiseAllowed := 0
	for _, a := range x.active {
		if a.effect.Type == "choice" {
			switch a.effect.Choice {
			case "skill":
				allowed += a.effect.Count
			case "expertise":
				expertiseAllowed += a.effect.Count
			}
		}
	}
	chosen := map[string]bool{}
	for i, s := range x.b.SkillProficiencies {
		if _, ok := c.skills[s]; !ok {
			x.issue(IssueUnknownKey, fmt.Sprintf("full.skill_proficiency_keys[%d]", i), "A perícia escolhida não existe.")
			continue
		}
		if !automatic[s] {
			chosen[s] = true
		}
	}
	if len(x.classes) > 0 {
		switch n := len(chosen); {
		case n > allowed:
			subject, change := x.skillChange(allowed, n)
			x.issueChange(IssueSkillCount, "full.skill_proficiency_keys", subject, change,
				fmt.Sprintf("Há %d perícias escolhidas; o personagem escolhe %d.", n, allowed))
		case n < allowed:
			x.d.OpenChoices = append(x.d.OpenChoices, OpenChoice{Kind: OpenChoiceSkills, Missing: allowed - n})
			subject, change := x.skillChange(allowed, n)
			x.issueChange(IssueSkillCount, "full.skill_proficiency_keys", subject, change,
				fmt.Sprintf("Faltam %d perícias para escolher.", allowed-n))
		}
	}

	// Expertise comes from features such as the rogue's Expertise.
	for _, f := range x.d.Features {
		if feat, ok := c.features[f.Key]; ok {
			expertiseAllowed += feat.ExpertiseChoices
		}
	}
	if n := len(x.b.Expertise); n > expertiseAllowed {
		x.issue(IssueExpertise, "full.expertise_skill_keys", "Há %d perícias com especialização; o personagem tem %d.", n, expertiseAllowed)
	}

	x.checkOptionCounts()

	// A custom background grants two skills, like every SRD background.
	if x.customBackground() {
		if n := len(x.b.CustomBackgroundSkills); n < CustomBackgroundSkillCount {
			x.issue(IssueSkillCount, "full.custom_background.skill_keys", "O antecedente personalizado concede %d perícias; faltam %d.", CustomBackgroundSkillCount, CustomBackgroundSkillCount-n)
		}
		// SRD 5.1 "Customizing a Background": two tools or languages, a feature and
		// the equipment too (question 82). A sheet written before these fields
		// existed shows nothing new: the three warnings come only once the sheet has
		// any of them, that is, once the editor that writes them saved it.
		if x.customBackgroundStarted() {
			if n := len(x.b.CustomBackgroundProficiencies); n < CustomBackgroundProficiencyCount {
				x.issue(IssueMissing, "full.custom_background.proficiency_keys", "O antecedente personalizado concede %d ferramentas ou idiomas; faltam %d.", CustomBackgroundProficiencyCount, CustomBackgroundProficiencyCount-n)
			}
			if x.b.CustomBackgroundFeatureName == "" || x.b.CustomBackgroundFeature == "" {
				x.issue(IssueMissing, "full.custom_background.feature_name", "O antecedente personalizado tem uma característica: falta o nome ou o texto dela.")
			}
			if x.b.CustomBackgroundEquipment == "" {
				x.issue(IssueMissing, "full.custom_background.equipment", "O antecedente personalizado traz equipamento: falta descrevê-lo.")
			}
		}
		// A language the race already gives uses one of the two picks for nothing.
		if x.race != nil {
			for i, k := range x.b.CustomBackgroundProficiencies {
				if slices.Contains(x.race.Languages, k) {
					x.issue(IssueMissing, fmt.Sprintf("full.custom_background.proficiency_keys[%d]", i), "O idioma %s já vem da raça: ele gasta uma das duas escolhas do antecedente sem acrescentar nada.", x.c.namePT(k))
				}
			}
		}
	}

	// Multiclassing needs the prerequisites of every class, the first one
	// included.
	if len(x.classes) > 1 {
		for _, oc := range x.classes {
			if !x.meetsMulticlass(oc) {
				x.issueChange(IssueMulticlass, fmt.Sprintf("full.classes[%d].class_key", oc.index), oc.key,
					"agora pede outras habilidades para multiclasse; os desta ficha não cumprem.",
					fmt.Sprintf("As habilidades não cumprem o pré-requisito de multiclasse de %s.", c.namePT(oc.key)))
			}
		}
	}
}

// skillChange is the sentence for "A classe mudou" when the number of skills a
// sheet chose no longer matches what it is entitled to, and the entry it is
// about. With the starting class the only source it is the class ("agora dá 2
// perícias no nível 1; esta ficha tem 3."); with other sources (a race's trait)
// it is only the count, and no entry is named.
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
	c := x.c
	var pools []*optionPool
	join := func(feature string, choose int, options []string) {
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
	}
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
			join(d.feature, d.choose, d.options)
		}
	}
	if inv := c.features[invocationsFeature]; inv != nil && seen[invocationsFeature] {
		known := 0
		for _, oc := range x.classes {
			if rows := c.classLevels[oc.key]; oc.level >= 1 && oc.level <= len(rows) {
				known += invocationsKnown(rows[oc.level-1])
			}
		}
		join(invocationsFeature, known, inv.Options)
	}

	for _, p := range pools {
		names := map[string]bool{}
		for _, key := range x.b.FeatureChoices {
			if f, ok := c.features[key]; ok && p.options[key] {
				names[f.Name] = true
			}
		}
		if n := len(names); n > p.allowed {
			x.issue(IssueChoiceCount, "full.feature_choice_keys", "Há %d escolhas em %s; o personagem tem %d.", n, p.name, p.allowed)
		}
	}
}
