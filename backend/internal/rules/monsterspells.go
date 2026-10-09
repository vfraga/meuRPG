package rules

import (
	"fmt"
	"regexp"
	"strconv"
	"strings"

	"github.com/PuraFome/meuRPG/backend/internal/rules/srd51"
)

// The Spellcasting and Innate Spellcasting traits of a stat block (SRD 5.1,
// "Monsters": Spellcasting, Innate Spellcasting): the data keeps them as text, in
// two regular layouts, which are read here and checked against the spells:
//
//	The mage is a 9th-level spellcaster. Its spellcasting ability is Intelligence
//	(spell save DC 14, +6 to hit with spell attacks). ... wizard spells prepared:
//	- Cantrips (at will): fire bolt, light
//	- 1st level (4 slots): detect magic, shield
//
//	The djinni's innate spellcasting ability is Charisma (spell save DC 17). ...
//	At will: detect magic
//	3/day each: tongues, wind walk
//	1/day each: creation

var (
	spellAbilityRe = regexp.MustCompile(`(?i)spell ?casting ability is (Strength|Dexterity|Constitution|Intelligence|Wisdom|Charisma)`)
	spellDCRe      = regexp.MustCompile(`spell save DC (\d+)`)
	spellAttackRe  = regexp.MustCompile(`\+(\d+) to hit with spell attacks`)
	casterLevelRe  = regexp.MustCompile(`is an? (\d+)(?:st|nd|rd|th)-level spellcaster`)
	spellClassRe   = regexp.MustCompile(`has (?:the )?following (\w+) spells prepared`)
	atWillProseRe  = regexp.MustCompile(`can cast ([a-z ,']+?) at will`)
	singleSpellRe  = regexp.MustCompile(`can innately cast ([a-z ]+?)(?: \(spell save DC \d+\))?, requiring`)
	// spellLabelRe finds the labels that start a group of spells, wherever they are in
	// the text (the lamia's trait has them on one line).
	spellLabelRe = regexp.MustCompile(`(?:Cantrips \(at will\)|(\d+)(?:st|nd|rd|th) level \((\d+) slots?\)|At will|(\d+)/day(?: each)?):`)
	spellNoteRe  = regexp.MustCompile(`\s*\(([^)]*)\)`)
)

// planSpellcasting reads one Spellcasting or Innate Spellcasting trait.
func (c *content) planSpellcasting(m *srd51.Monster, a srd51.MonsterAbility) (SpellcastingPlan, error) {
	fail := func(format string, args ...any) (SpellcastingPlan, error) {
		return SpellcastingPlan{}, fmt.Errorf("%s: %s", a.Name, fmt.Sprintf(format, args...))
	}
	sc := SpellcastingPlan{Innate: a.Name == "Innate Spellcasting", Name: a.Name, Text: a.Desc}
	am := spellAbilityRe.FindStringSubmatch(a.Desc)
	if am == nil {
		return fail("no spellcasting ability")
	}
	sc.Ability = abilityByWord[am[1]]
	mod := modifier(monsterScores(m)[sc.Ability])
	// The SRD states the DC and the attack bonus in the stat block; where it leaves one
	// out, the creature's own numbers give it (8 or 0, plus the proficiency bonus and the
	// ability's modifier).
	sc.SaveDC = 8 + m.ProficiencyBonus + mod
	if dm := spellDCRe.FindStringSubmatch(a.Desc); dm != nil {
		sc.SaveDC, _ = strconv.Atoi(dm[1])
	}
	sc.AttackBonus = m.ProficiencyBonus + mod
	if hm := spellAttackRe.FindStringSubmatch(a.Desc); hm != nil {
		sc.AttackBonus, _ = strconv.Atoi(hm[1])
	}
	if lm := casterLevelRe.FindStringSubmatch(a.Desc); lm != nil {
		sc.CasterLevel, _ = strconv.Atoi(lm[1])
	}
	if cm := spellClassRe.FindStringSubmatch(a.Desc); cm != nil {
		sc.ClassKey = "class:" + cm[1]
		if _, ok := c.classes[sc.ClassKey]; !ok {
			return fail("%q is not a class", cm[1])
		}
	}
	ref := func(name string) (SpellRef, error) {
		name = strings.TrimSpace(name)
		before := name
		note := ""
		if nm := spellNoteRe.FindStringSubmatch(name); nm != nil {
			note = nm[1]
		}
		name = strings.TrimSpace(strings.TrimSuffix(strings.TrimSpace(spellNoteRe.ReplaceAllString(name, "")), "*"))
		if strings.HasSuffix(before, "*") {
			note = "conjurada antes do combate"
		}
		key := "spell:" + slugOf(name)
		sp, ok := c.spells[key]
		if !ok {
			return SpellRef{}, fmt.Errorf("%s: %q is not an SRD spell", a.Name, name)
		}
		return SpellRef{Key: key, Name: sp.Name, NamePT: c.namePT(key), Level: sp.Level, Note: note}, nil
	}
	refs := func(list string) ([]SpellRef, error) {
		var out []SpellRef
		for _, part := range splitSpellList(list) {
			r, err := ref(part)
			if err != nil {
				return nil, err
			}
			out = append(out, r)
		}
		return out, nil
	}

	text := a.Desc
	if sm := singleSpellRe.FindStringSubmatch(text); sm != nil && sc.Innate && !strings.HasPrefix(sm[1], "the following") {
		uses, ok := c.innateUses[m.Key]
		if !ok {
			return fail("the single spell has no uses a day (innate_spell_uses)")
		}
		list, err := refs(sm[1])
		if err != nil {
			return SpellcastingPlan{}, err
		}
		sc.PerDay = append(sc.PerDay, InnateGroup{Uses: uses, Each: true, Spells: list})
		return sc, nil
	}
	// The footnote of the archmage ("* The archmage casts these spells on itself before
	// combat.") is not a spell list.
	if i := strings.Index(text, "\n* "); i >= 0 {
		text = text[:i]
	}
	labels := spellLabelRe.FindAllStringSubmatchIndex(text, -1)
	if len(labels) == 0 {
		return fail("no spell list")
	}
	if pm := atWillProseRe.FindStringSubmatch(text[:labels[0][0]]); pm != nil {
		list, err := refs(strings.ReplaceAll(pm[1], " and ", ", "))
		if err != nil {
			return SpellcastingPlan{}, err
		}
		sc.AtWill = append(sc.AtWill, list...)
	}
	for i, at := range labels {
		end := len(text)
		if i+1 < len(labels) {
			end = labels[i+1][0]
		}
		// The text between two labels: the spells, up to the end of the line.
		body := strings.TrimSpace(text[at[1]:end])
		if nl := strings.Index(body, "\n"); nl >= 0 {
			body = strings.TrimSpace(body[:nl])
		}
		list, err := refs(body)
		if err != nil {
			return SpellcastingPlan{}, err
		}
		label := text[at[0]:at[1]]
		switch {
		case at[2] >= 0: // "1st level (4 slots)"
			level, _ := strconv.Atoi(text[at[2]:at[3]])
			slots, _ := strconv.Atoi(text[at[4]:at[5]])
			sc.Levels = append(sc.Levels, SpellcastingLevel{Level: level, Slots: slots, Spells: list})
		case at[6] >= 0: // "3/day each"
			uses, _ := strconv.Atoi(text[at[6]:at[7]])
			sc.PerDay = append(sc.PerDay, InnateGroup{Uses: uses, Each: strings.Contains(label, "each"), Spells: list})
		default: // "Cantrips (at will)" and "At will"
			sc.AtWill = append(sc.AtWill, list...)
		}
	}
	return sc, nil
}

// splitSpellList splits a list of spell names at the commas outside parentheses
// ("disguise self (any humanoid form), major image").
func splitSpellList(list string) []string {
	var out []string
	depth, start := 0, 0
	for i, r := range list {
		switch r {
		case '(':
			depth++
		case ')':
			depth--
		case ',':
			if depth == 0 {
				out = append(out, strings.TrimSpace(list[start:i]))
				start = i + 1
			}
		}
	}
	if rest := strings.TrimSpace(strings.TrimRight(list[start:], ". ")); rest != "" {
		out = append(out, rest)
	}
	return out
}
