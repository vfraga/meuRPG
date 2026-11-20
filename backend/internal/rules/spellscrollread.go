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

// ScrollSpellGonePT is what the character's player reads for a scroll whose spell the master switched off or archived after it was
// set (RN-23), in place of the spell's key and name, and as the words of a refused read.
const ScrollSpellGonePT = "A magia deste pergaminho não está mais na mesa."

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

// ScrollSpell is what the spell stored on an item of a sheet makes of it as a Spell Scroll: the one place that says whether a scroll
// gives a spell, for the sheet's read and for the reading of the scroll alike.
type ScrollSpell struct {
	// IsScroll says the item is a variant of the Spell Scroll family; false for any other item, whose stored key says nothing.
	IsScroll bool
	// Key is the stored spell key ("" for a scroll with none).
	Key string
	// Level is the spell's own level, 0 when the key is no spell.
	Level int
	// Off says the key is no spell of the content or the table has switched it off or archived it.
	Off bool
	// Moved says the spell exists and its level is not the scroll's.
	Moved bool
}

// Gives says the scroll gives the spell: it holds one that exists, is on at the table and is of the scroll's level.
func (s ScrollSpell) Gives() bool { return s.IsScroll && s.Key != "" && !s.Off && !s.Moved }

// ScrollSpellOf reads the spell the item holds, for the spell key stored on its sheet entry.
func (c *Content) ScrollSpellOf(itemKey, spellKey string) ScrollSpell {
	scroll, ok := c.SpellScrollNumbers(itemKey)
	if !ok {
		return ScrollSpell{}
	}
	out := ScrollSpell{IsScroll: true, Key: spellKey}
	if spellKey == "" {
		return out
	}
	det, exists := c.SpellDetails(spellKey)
	out.Off = !exists || c.Hidden(spellKey)
	if exists {
		out.Level, out.Moved = det.Spell.Level, det.Spell.Level != scroll.Level
	}
	return out
}

// ScrollCheck is the ability check that reading a spell above the reader's level asks for.
type ScrollCheck struct {
	D20, Modifier, Total, DC int
	// Passed says the total reaches the DC; always true for a read that asks no check.
	Passed bool
}

// Check is the check of the reading for the d20 rolled or typed and the modifier of the reader's spellcasting ability: the total
// (d20 plus modifier) against the DC, a tie passing, with no critical on a 1 or a 20 (an ability check has none). A read that
// needs no check passes with nothing rolled.
func (r ScrollRead) Check(d20, modifier int) ScrollCheck {
	if !r.CheckNeeded {
		return ScrollCheck{Passed: true}
	}
	total := d20 + modifier
	return ScrollCheck{D20: d20, Modifier: modifier, Total: total, DC: r.CheckDC, Passed: total >= r.CheckDC}
}

// AbilityModifier is the modifier of the character's ability, 0 for an ability the sheet does not carry (none).
func (d Derived) AbilityModifier(a Ability) int {
	for _, s := range d.Abilities {
		if s.Ability == a {
			return s.Modifier
		}
	}
	return 0
}
