package rules

import (
	"fmt"
	"regexp"
	"strconv"
	"strings"
)

// MonsterCaster is what the combat needs to let a stat block cast its own spells (MR-042): the
// Spellcasting trait (slots, prepared spells, save DC, attack bonus) and the Innate Spellcasting
// trait (at will, N/day), read from the SRD text. Innate spells are granted spells: at will, or
// with a resource that holds the uses a day.
type MonsterCaster struct {
	// Level is the caster level the cantrips scale with: the "Nth-level spellcaster" of the Spellcasting trait,
	// and 1 for a creature that only casts innately (the SRD data gives no caster level to a monster's innate cantrips).
	Level int
	// Spellcasting is the creature's casting; its Class is the creature's key.
	Spellcasting Spellcasting
	// Slots are the slots of each level, index 0 the 1st level; nil without a Spellcasting trait.
	Slots []int
	// Spells are the prepared spells of the Spellcasting trait, cantrips included.
	Spells []CharacterSpell
	// Granted are the innate spells, and Resources the uses a day they spend.
	Granted   []GrantedSpell
	Resources []Resource
}

var (
	casterAbilityRe = regexp.MustCompile(`(?i)spell ?casting ability (?:is )?(Intelligence|Wisdom|Charisma)`)
	casterLevelRe   = regexp.MustCompile(`(?i)(\d+)(?:st|nd|rd|th)-level spellcaster`)
	casterDCRe      = regexp.MustCompile(`spell save DC (\d+)`)
	casterAttackRe  = regexp.MustCompile(`\+(\d+) to hit with spell attacks`)
	innateBreakRe   = regexp.MustCompile(`[\s;]+(\d+/day(?: each)?:)`)
	innateLineRe    = regexp.MustCompile(`(?mi)^(?:[^\n]*\. )?(at will|(\d+)/day( each)?): (.+)$`)
	parenthesisRe   = regexp.MustCompile(`\s*\([^)]*\)`)
)

// CastingText is what the text of a Spellcasting or Innate Spellcasting trait writes: the caster level, the
// casting ability, the save DC and the spell attack bonus. A Has field says the text wrote that value, so a reader that
// prints the text (the NPC's note) does not invent what the rules would derive.
type CastingText struct {
	Level                                  int
	Ability                                Ability
	DC, Attack                             int
	HasLevel, HasAbility, HasDC, HasAttack bool
}

// SpellcastingNumbers reads the numbers a trait's text writes. It is the one reader of them: MonsterCaster and the
// NPC's note both use it.
func SpellcastingNumbers(desc string) CastingText {
	var t CastingText
	if m := casterLevelRe.FindStringSubmatch(desc); m != nil {
		t.Level, _ = strconv.Atoi(m[1])
		t.HasLevel = true
	}
	if m := casterAbilityRe.FindStringSubmatch(desc); m != nil {
		t.Ability, t.HasAbility = casterAbilities[strings.ToLower(m[1])], true
	}
	if m := casterDCRe.FindStringSubmatch(desc); m != nil {
		t.DC, _ = strconv.Atoi(m[1])
		t.HasDC = true
	}
	if m := casterAttackRe.FindStringSubmatch(desc); m != nil {
		t.Attack, _ = strconv.Atoi(m[1])
		t.HasAttack = true
	}
	return t
}

var casterAbilities = map[string]Ability{"intelligence": INT, "wisdom": WIS, "charisma": CHA}

// innateSourceSuffix ends the key of the pseudo source of a creature's innate spells.
const innateSourceSuffix = "#innate-spellcasting"

// MonsterCaster reads the casting of a creature; false when it has no Spellcasting or Innate
// Spellcasting trait with a spell the content knows.
func (c *Content) MonsterCaster(key string) (MonsterCaster, bool) {
	m, ok := c.c.monsters[key]
	if !ok {
		return MonsterCaster{}, false
	}
	scores := monsterScores(m)
	out := MonsterCaster{Level: 1}
	for _, a := range m.SpecialAbilities {
		switch a.Name {
		case "Spellcasting":
			c.c.addSlotCasting(&out, key, m.ProficiencyBonus, scores, a.Desc)
		case "Innate Spellcasting":
			c.c.addInnateCasting(&out, key, m.ProficiencyBonus, scores, a.Desc)
		}
	}
	if len(out.Spells) == 0 && len(out.Granted) == 0 {
		return MonsterCaster{}, false
	}
	return out, true
}

// numbersOf are what a trait's text writes, with the save DC and the attack bonus it gives: the numbers
// it writes, and, for what it leaves out, 8 or nothing plus the proficiency bonus and the ability modifier.
// false when the text writes neither an ability nor a DC or an attack bonus: there is nothing to cast with.
func numbersOf(profBonus int, scores map[Ability]int, desc string) (t CastingText, dc, attack int, ok bool) {
	t = SpellcastingNumbers(desc)
	if !t.HasAbility && !t.HasDC && !t.HasAttack {
		return t, 0, 0, false
	}
	mod := modifier(scores[t.Ability])
	dc, attack = 8+profBonus+mod, profBonus+mod
	switch {
	case t.HasDC && t.HasAttack:
		dc, attack = t.DC, t.Attack
	case t.HasDC && !t.HasAbility:
		dc, attack = t.DC, t.DC-8
	case t.HasAttack && !t.HasAbility:
		dc, attack = t.Attack+8, t.Attack
	case t.HasDC:
		dc = t.DC
	case t.HasAttack:
		attack = t.Attack
	}
	return t, dc, attack, true
}

// addSlotCasting adds the Spellcasting trait: the slots, the prepared spells and the numbers.
func (c *content) addSlotCasting(out *MonsterCaster, key string, profBonus int, scores map[Ability]int, desc string) {
	t, dc, attack, ok := numbersOf(profBonus, scores, desc)
	slots, spells := c.castingTrait(desc)
	if !ok || (len(spells) == 0 && !t.HasDC) {
		return
	}
	if t.HasLevel {
		out.Level = t.Level
	}
	out.Spellcasting = Spellcasting{Class: key, ClassNamePT: c.namePT(key), Ability: t.Ability, SaveDC: dc, AttackBonus: attack}
	out.Slots = slots[:]
	for _, k := range spells {
		if e, ok := c.spellEntries[k]; ok {
			out.Spells = append(out.Spells, CharacterSpell{Spell: e, Prepared: true})
		}
	}
}

// addInnateCasting adds the innate spells of the trait as granted spells: "At will" ones at will, "N/day each"
// ones with a resource of N uses a spell, and "N/day" ones with one resource of N uses for the whole list.
func (c *content) addInnateCasting(out *MonsterCaster, key string, profBonus int, scores map[Ability]int, desc string) {
	t, dc, attack, ok := numbersOf(profBonus, scores, desc)
	if !ok {
		return
	}
	source := key + innateSourceSuffix
	// A text that runs its lists together on one line ("... major image 3/day each: ...") gets one list a line.
	desc = innateBreakRe.ReplaceAllString(desc, "\n$1")
	base := GrantedSpell{Source: source, SourcePT: "Conjuração inata", Ability: t.Ability, AttackBonus: attack, SaveDC: dc}
	for _, line := range innateLineRe.FindAllStringSubmatch(desc, -1) {
		perDay, _ := strconv.Atoi(line[2])
		for _, name := range listedSpellNames(line[4]) {
			if entry, ok := c.spellEntries["spell:"+slugOf(name)]; ok {
				g := base
				g.Spell, g.AtWill, g.CastLevel = entry, strings.EqualFold(line[1], "At will"), entry.Level
				grantInnate(out, g, perDay, line[3] != "")
			}
		}
	}
	if len(out.Granted) > 0 && out.Spellcasting.Class == "" {
		out.Spellcasting = Spellcasting{Class: key, ClassNamePT: c.namePT(key), Ability: t.Ability, SaveDC: dc, AttackBonus: attack}
	}
}

// grantInnate adds an innate spell, with the resource of its uses a day when it is not at will: its own for "N/day
// each", the list's shared one for "N/day".
func grantInnate(out *MonsterCaster, g GrantedSpell, perDay int, each bool) {
	if !g.AtWill {
		g.Resource = fmt.Sprintf("innate_%d_day", perDay)
		if each {
			g.Resource = fmt.Sprintf("innate_%d_day_%s", perDay, strings.TrimPrefix(g.Spell.Key, "spell:"))
		}
		if !hasCasterResource(out.Resources, g.Resource) {
			out.Resources = append(out.Resources, Resource{Key: g.Resource, NamePT: resourceName(g.Spell.NamePT, perDay, each), Max: perDay, Recharge: RechargeLongRest, Source: g.Source})
		}
	}
	out.Granted = append(out.Granted, g)
}

func hasCasterResource(rs []Resource, key string) bool {
	for _, r := range rs {
		if r.Key == key {
			return true
		}
	}
	return false
}

// resourceName names the uses of an innate spell: the spell's own, or the list's shared pool.
func resourceName(spellPT string, perDay int, each bool) string {
	if each {
		return spellPT
	}
	return fmt.Sprintf("Magias inatas (%d/dia)", perDay)
}

// innateListNames are the spell names an Innate Spellcasting trait's text lists, by frequency.
func innateListNames(desc string) []string {
	var names []string
	for _, line := range innateLineRe.FindAllStringSubmatch(innateBreakRe.ReplaceAllString(desc, "\n$1"), -1) {
		names = append(names, listedSpellNames(line[4])...)
	}
	return names
}
