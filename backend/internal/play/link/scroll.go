package link

// ScrollEntry is a Spell Scroll entry of a character's sheet as its reader reads it: the spell it gives and what reading the spell
// comes to for the character (the characters module works out every number with the rules; package play decides what to do with
// them).
type ScrollEntry struct {
	// EntryID is the entry's id on the sheet, ItemKey the scroll's catalog key and ItemNamePT its name.
	EntryID, ItemKey, ItemNamePT string
	// NoSpell says the scroll gives no spell: none is stored, the stored one is not of the scroll's level, or the table has
	// switched it off or archived it (Off says that is the reason). Every field below is empty then.
	NoSpell, Off bool
	// SpellKey and SpellNamePT are the spell the scroll gives.
	SpellKey, SpellNamePT string
	// Level is the spell's level, 0 for a cantrip.
	Level int
	// Reads says the spell is on the list of a class the character casts from, and UnreadablePT says why it is not when it is not.
	Reads        bool
	UnreadablePT string
	// CheckNeeded says the spell's level is above the highest the character casts, and CheckDC is the check's DC; Modifier is the
	// modifier of the character's spellcasting ability, which the check adds to the d20.
	CheckNeeded bool
	CheckDC     int
	Modifier    int
	// Economy is the part of the turn the spell's casting time spends: "action", "bonus_action" or "reaction", and "" for a
	// casting of a minute or longer, which spends none.
	Economy string
}
