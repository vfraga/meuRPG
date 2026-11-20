package rules

import "slices"

// Casting outside a fight (SRD 5.1, "Spellcasting"): how long a spell takes to
// cast, the ritual version, how long it lasts and what a character's sheet lets
// them cast as a ritual. Everything here is pure; the play module records the
// casts and decides when they start and end.

// RitualExtraMinutes is what the ritual version of a spell adds to its casting
// time (SRD 5.1, "Rituals": "10 minutes longer to cast than normal").
const RitualExtraMinutes = 10

// Minutes per casting time unit.
const (
	minutesPerHour = 60
	// SecondsPerRound is the six seconds of a round, in combat and outside it (SRD 5.1,
	// "The Order of Combat"): the one conversion between rounds and game time.
	SecondsPerRound  = 6
	secondsPerMinute = 60
	secondsPerHour   = 3600
	secondsPerDay    = 86400
)

// The rests that end a spell by the time they take (SRD 5.1, "Resting": a short
// rest is at least 1 hour long and a long rest at least 8 hours).
const (
	// ShortRestMinutes and LongRestMinutes are the shortest time of each rest.
	ShortRestMinutes = 60
	LongRestMinutes  = 480
)

// What ends a lasting spell, as EndedByRest says.
const (
	// RestNone: no rest ends it by itself (it outlasts a long rest, or it has no
	// duration the table can count).
	RestNone = ""
	// RestShort: its whole duration fits a short rest.
	RestShort = "short"
	// RestLong: its whole duration fits a long rest.
	RestLong = "long"
)

// Minutes is how long casting takes when it is longer than an action: a spell
// with a casting time of 1 action, 1 bonus action or 1 reaction takes 0 (it is
// cast at once), "10 minutes" takes 10 and "1 hour" takes 60. A casting time the
// content could not read takes 0 as well.
func (ct CastingTime) Minutes() int {
	switch ct.Unit {
	case CastMinute:
		return ct.Amount
	case CastHour:
		return ct.Amount * minutesPerHour
	}
	return 0
}

// CastMinutes is the minutes casting takes, the ritual version's 10 included:
// a ritual that takes an action as usual takes 10 minutes as a ritual, and one
// that takes 1 minute takes 11.
func CastMinutes(ct CastingTime, ritual bool) int {
	m := ct.Minutes()
	if ritual {
		m += RitualExtraMinutes
	}
	return m
}

// IsLongCast says casting takes minutes or hours, not an action, a bonus action
// or a reaction: the caster spends the casting time concentrating, and the slot
// is spent when the casting is done (SRD 5.1, "Casting Time": "Longer Casting
// Times").
func IsLongCast(ct CastingTime, ritual bool) bool { return CastMinutes(ct, ritual) > 0 }

// Seconds is how long a timed duration lasts: rounds are 6 seconds. False for a
// duration that is instantaneous, until dispelled or special.
func (d SpellDuration) Seconds() (int, bool) {
	if d.Kind != DurationTimed || d.Amount <= 0 {
		return 0, false
	}
	switch d.Unit {
	case DurationRound:
		return d.Amount * SecondsPerRound, true
	case DurationMinute:
		return d.Amount * secondsPerMinute, true
	case DurationHour:
		return d.Amount * secondsPerHour, true
	case DurationDay:
		return d.Amount * secondsPerDay, true
	}
	return 0, false
}

// Lasts says the spell stays after the cast is done: it has a timed duration or
// lasts until dispelled. An instantaneous spell does not, and a special duration
// is left to the table.
func (d SpellDuration) Lasts() bool {
	_, timed := d.Seconds()
	return timed || d.Kind == DurationUntilDispelled
}

// EndedByRest says which rest is long enough for the whole duration of the spell
// to pass: RestShort when it lasts an hour or less, RestLong when it lasts 8
// hours or less, RestNone otherwise. The SRD gives the length of each rest, not a
// list of spells they end: the spell ends because its time ran out while the
// character rested.
func EndedByRest(d SpellDuration) string {
	secs, timed := d.Seconds()
	switch {
	case !timed:
		return RestNone
	case secs <= ShortRestMinutes*secondsPerMinute:
		return RestShort
	case secs <= LongRestMinutes*secondsPerMinute:
		return RestLong
	}
	return RestNone
}

// RestEnds says a rest of restMinutes (restMinutes: a short rest's 60 or a
// long rest's 480) is long enough for the spell's whole duration to pass.
func RestEnds(d SpellDuration, restMinutes int) bool {
	secs, timed := d.Seconds()
	return timed && secs <= restMinutes*secondsPerMinute
}

// SpellStanding is what a character's sheet says about one spell: whether the
// spell is on it, whether it can be cast with a slot today, and whether it can
// be cast as a ritual.
type SpellStanding struct {
	// Known: the spell is on the sheet (a wizard's spellbook counts).
	Known bool
	// Prepared: it can be cast with a slot today (every cantrip, every spell of a
	// class that knows its spells, the prepared list).
	Prepared bool
	// CanRitual: the spell has the ritual tag and the character has a feature
	// that casts it as a ritual.
	CanRitual bool
}

// wizardClass is the class whose Ritual Casting reads the spellbook: SRD 5.1,
// "Rituals": the caster must have the spell prepared or on the list of spells
// known, unless the character's ritual feature says otherwise, as the wizard's does.
const wizardClass = "class:wizard"

// findFamiliar and pactOfTheChain are the one case that lets a character cast a
// ritual spell it does not have: the warlock's Pact of the Chain (SRD 5.1) learns
// Find Familiar and "can cast it as a ritual".
const (
	findFamiliar   = "spell:find-familiar"
	pactOfTheChain = "feature:pact-of-the-chain"
)

// bookInvocationKey is the warlock invocation that casts two spells of the Book of
// Shadows, and the warlock spells it knows, as rituals (SRD 5.1, Warlock, Eldritch
// Invocations: Book of Ancient Secrets). bookRitualsKey is the choice of its two spells.
const (
	bookInvocationKey = "feature:eldritch-invocation-book-of-ancient-secrets"
	bookRitualsKey    = bookInvocationKey + textSeparator + "rituals"
)

// SpellStanding works out what the derived sheet says about the spell.
// ritualTag is whether the spell carries the ritual tag (SpellEntry.Ritual).
//
// The rituals follow the SRD classes that have a ritual feature (Derived
// Spellcasting.Ritual): the bard from the spells known, the cleric and the druid
// from the prepared spells, the wizard from the spellbook. The warlock's Book of
// Ancient Secrets adds its two spells (known, not prepared) and the warlock spells known.
func (d Derived) SpellStanding(spellKey string, ritualTag bool) SpellStanding {
	var out SpellStanding
	book := d.holdsBook()
	for _, cs := range d.Spells {
		if cs.Spell.Key != spellKey {
			continue
		}
		out.Known, out.Prepared = true, cs.Prepared
		if ritualTag && d.castsAsRitual(cs, book) {
			out.CanRitual = true
		}
	}
	if spellKey == findFamiliar && ritualTag && slices.ContainsFunc(d.Features, func(f Feature) bool { return f.Key == pactOfTheChain }) {
		out.Known, out.CanRitual = true, true
	}
	return out
}

// holdsBook says the sheet has the invocation Book of Ancient Secrets.
func (d *Derived) holdsBook() bool {
	return slices.ContainsFunc(d.Features, func(f Feature) bool { return f.Key == bookInvocationKey })
}

// castsAsRitual says a ritual feature of the sheet casts the spell of the sheet as a ritual:
// the class's Ritual Casting, or, with the Book of Ancient Secrets, the book's two spells and
// the warlock spells the character knows (the ones on the character's warlock list, the
// patron's expanded list included: CharacterSpell.WarlockList). The sheet does not say which
// class knows a spell, so on a multiclass sheet a spell on the warlock list counts whoever
// knows it.
func (d Derived) castsAsRitual(cs CharacterSpell, book bool) bool {
	for _, sc := range d.Spellcasting {
		if sc.Ritual && (cs.Prepared || sc.Class == wizardClass) {
			return true
		}
	}
	return book && (cs.FromBook || (cs.Prepared && cs.WarlockList))
}

// MageArmorSpell is Mage Armor's key, and mageArmorBase the 13 in its "base AC
// becomes 13 + its Dexterity modifier".
const (
	MageArmorSpell = "spell:mage-armor"
	mageArmorBase  = 13
)
