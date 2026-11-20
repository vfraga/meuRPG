package rules

import "slices"

// Reading the spell of a scroll (SRD 5.1, "Spell Scroll"): a character reads it when the spell is on the list of a class they
// cast from, and when its level is above the highest they can cast the reader first passes an ability check with their
// spellcasting ability against DC 10 + the spell's level.

// ScrollRead is what a character's reading of the spell on a scroll comes to.
type ScrollRead struct {
	// Level is the spell's level, 0 for a cantrip.
	Level int
	// Reads says the spell is on the list of a class the character casts from.
	Reads bool
	// UnreadablePT is why the character does not read it ("Não é da lista de Mago."), empty when Reads.
	UnreadablePT string
	// CheckNeeded says the spell's level is above the highest the character can cast, pact slots included.
	CheckNeeded bool
	// CheckDC is 10 plus the spell's level when CheckNeeded, else 0.
	CheckDC int
	// Ability is the spellcasting ability of the class whose list has the spell when CheckNeeded, else empty.
	Ability Ability
	// Economy is the part of the turn the spell's casting time spends (an Economy* constant), or "" when it takes a minute or
	// longer, which spends none of it. Empty when the character does not read the spell.
	Economy string
}

// scrollCheckBaseDC is what the check's DC adds the spell's level to.
const scrollCheckBaseDC = 10

// ReadScrollSpell says what it comes to for the character of d to read the scroll's spell, and false when the key is no spell.
func (c *Content) ReadScrollSpell(d Derived, spellKey string) (ScrollRead, bool) {
	det, ok := c.SpellDetails(spellKey)
	if !ok {
		return ScrollRead{}, false
	}
	out := ScrollRead{Level: det.Spell.Level}
	var caster *Spellcasting
	for i := range d.Spellcasting {
		if slices.Contains(det.Spell.Classes, d.Spellcasting[i].castsFrom()) {
			caster = &d.Spellcasting[i]
			break
		}
	}
	if caster == nil {
		out.UnreadablePT = c.unreadableReasonPT(d)
		return out, true
	}
	out.Reads = true
	out.Economy = det.CastingTime.Economy()
	if det.Spell.Level > highestCastableLevel(d) {
		out.CheckNeeded, out.CheckDC, out.Ability = true, scrollCheckBaseDC+det.Spell.Level, caster.Ability
	}
	return out, true
}

// castsFrom is the class whose spell list the caster reads.
func (s Spellcasting) castsFrom() string {
	if s.SpellList != "" {
		return s.SpellList
	}
	return s.Class
}

// highestCastableLevel is the highest spell level the character has slots for: the slots of every class and the pact slots.
func highestCastableLevel(d Derived) int {
	highest := 0
	for i, n := range d.SpellSlots {
		if n > 0 {
			highest = i + 1
		}
	}
	if d.PactMagic != nil && d.PactMagic.Slots > 0 {
		highest = max(highest, d.PactMagic.SlotLevel)
	}
	return highest
}

// unreadableReasonPT is why a character does not read a spell that is on no list of theirs: the lists they do read, named by
// their classes, or that they cast nothing.
func (c *Content) unreadableReasonPT(d Derived) string {
	var names []string
	for _, s := range d.Spellcasting {
		if name := c.NamePT(s.castsFrom()); name != "" && !slices.Contains(names, name) {
			names = append(names, name)
		}
	}
	switch len(names) {
	case 0:
		return "Não conjura magias."
	case 1:
		return "Não é da lista de " + names[0] + "."
	}
	text := "Não é da lista de " + names[0]
	for i, name := range names[1:] {
		if i == len(names)-2 {
			text += " nem de " + name
		} else {
			text += ", de " + name
		}
	}
	return text + "."
}
